package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

const mnd8 = 8

var errAnthropicNoAPI = errors.New("anthropic provider: no API key (set ZAI_API_KEY or pass WithAnthropicAPIKey)")
var errAnthropicNilShaper = errors.New("anthropic provider: nil Shaper")

// httpClient is the streaming-path HTTP client. A shared default client is fine
// (Go's http.Transport pools connections). Tests override the endpoint via
// WithAnthropicBaseURL against an httptest server.
var httpClient = &http.Client{} //nolint:gochecknoglobals // shared HTTP client (connection pooling)

// sseIdleTimeoutDefault bounds server-side mid-stream silence between two body
// reads (the 08-09 SSE-stall finding: the connection stays open, zero chunks,
// and ctx cancellation does not reach the blocked ReadString — two capture runs
// hung 6+ minutes past the turn ctx's 20m deadline). Five minutes is generous:
// propose-stage turns emit chunks steadily; the observed stalls were open-ended
// silences. On expiry the watchdog force-closes the body and the stream
// terminates with a retryable (KindTransient) error instead of wedging.
const sseIdleTimeoutDefault = 5 * time.Minute

// errSSEIdleTimeout and errSSECancelled are the watchdog's static abort causes
// (wrapped with per-abort detail at the abort site).
var errSSEIdleTimeout = errors.New("sse idle timeout: server stalled mid-stream")
var errSSECancelled = errors.New("sse stream cancelled")

// Watchdog tick bound derivation: tick = idle/divisor clamped to
// [watchdogTickLowerBound, watchdogTickUpperBound] so short test idles check
// often enough and production idles do not spin.
const watchdogTickDivisor = 10
const watchdogTickLowerBound = 10 * time.Millisecond
const watchdogTickUpperBound = time.Second

// sseBody wraps the response body with idle tracking. Every successful Read
// refreshes lastActivity; the watchdog closes the underlying body (unblocking a
// wedged Read) when the ctx cancels or the idle deadline passes, recording the
// abort cause so drainSSE can surface it.
type sseBody struct {
	body io.ReadCloser

	mu       sync.Mutex
	last     time.Time
	abortErr error
}

func newSSEBody(body io.ReadCloser) *sseBody {
	return &sseBody{body: body, last: time.Now()}
}

// Read implements io.Reader (bufio wraps this). A successful read refreshes
// the idle clock. After the watchdog closes the body, Read returns the
// transport's closed-body error; drainSSE asks abortCause() for the semantic.
func (b *sseBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)

	if n > 0 {
		b.mu.Lock()
		b.last = time.Now()
		b.mu.Unlock()
	}

	if err != nil {
		return n, fmt.Errorf("sse body read: %w", err)
	}

	return n, nil
}

// idleFor returns the time since the last successful read.
func (b *sseBody) idleFor() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	return time.Since(b.last)
}

// abort closes the underlying body with cause recorded, force-unblocking any
// Read wedged on a silent connection (Close is the documented unblock for an
// in-flight body read; ctx propagation alone demonstrably does not reach it).
func (b *sseBody) abort(cause error) {
	b.mu.Lock()
	if b.abortErr == nil {
		b.abortErr = cause
	}
	b.mu.Unlock()

	_ = b.body.Close()
}

// abortCause returns the recorded abort cause (nil when the body was not
// aborted by the watchdog — e.g. a natural transport error).
func (b *sseBody) abortCause() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.abortErr
}

// sseWatchdog runs alongside drainSSE: it force-closes the monitored body when
// the request ctx cancels (belt-and-braces — the live h2 wedge showed ctx
// cancellation alone does not unblock the body read) or when no byte has
// arrived for idle. It exits when stop closes (drainSSE returned) or after the
// first abort.
func sseWatchdog(ctx context.Context, b *sseBody, idle time.Duration, stop <-chan struct{}) {
	if idle <= 0 {
		idle = sseIdleTimeoutDefault
	}

	tick := min(max(idle/watchdogTickDivisor, watchdogTickLowerBound), watchdogTickUpperBound)

	t := time.NewTicker(tick)
	defer t.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			b.abort(fmt.Errorf("%w: %w", errSSECancelled, ctx.Err()))

			return
		case <-t.C:
			if b.idleFor() >= idle {
				b.abort(fmt.Errorf("%w: no data for %s", errSSEIdleTimeout, idle))

				return
			}
		}
	}
}

// anthropicVersion is the Anthropic API version header the streaming path sends
// (matches the SDK's default). The non-streaming Send path uses the SDK, which
// sets this internally; the raw-HTTP streaming path sets it explicitly.
const anthropicVersion = "2023-06-01"

// Stream opens a streaming SSE request to the Anthropic endpoint and returns a
// channel of StreamChunks (ACP-04). It shapes the request via the same Shaper as
// Send (so the body is mimicry-faithful), adds `stream:true`, POSTs via raw HTTP
// (so ctx cancellation aborts the in-flight request), and parses the SSE deltas
// into text / tool_use / usage chunks, then a terminal "done" chunk carrying the
// FinishReason. ctx cancellation closes the channel promptly.
//
// An idle watchdog bounds server-side mid-stream silence: when the connection
// freezes with the body read blocked (the 08-09 SSE-stall finding), the
// watchdog force-closes the body and the channel carries an "error" chunk with
// a retryable ProviderError, then closes — a wedged stream never hangs the turn.
//
// The channel is buffered small (8); the turn loop drains it. A slow consumer
// is fine — the HTTP body read blocks until the consumer drains (backpressure
// propagates to the provider via the TCP window).
//
//nolint:funlen // domain complexity is inherent
func (p *AnthropicProvider) Stream(
	ctx context.Context, prof *profile.Profile, messages []Message,
) (<-chan StreamChunk, error) {
	key := p.apiKey
	if key == "" {
		key = os.Getenv("ZAI_API_KEY")
	}

	if key == "" {
		return nil, errAnthropicNoAPI
	}

	if p.shaper == nil {
		return nil, errAnthropicNilShaper
	}

	params, _, err := p.shaper.Shape(prof, messages)
	if err != nil {
		return nil, fmt.Errorf("anthropic provider shape: %w", err)
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("anthropic provider marshal stream body: %w", err)
	}
	// Inject stream:true so the endpoint returns SSE. We patch the marshaled
	// body rather than the SDK type to keep this provider-agnostic.
	body = injectStreamTrue(body)

	// Capture the verbatim shaped request (LOG-01) — same hook as Send.
	if p.capture != nil {
		p.capture(body, headersFromProfile(prof))
	}

	url := strings.TrimRight(p.baseURL, "/") + "/v1/messages"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic provider stream request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", key)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Anthropic-Version", anthropicVersion)

	for _, h := range prof.Headers {
		req.Header.Set(h.Name, shaper.RenderHeaderValue(h.ValueTemplate))
	}

	for _, o := range p.extraOpts {
		// extraOpts are opaque option.RequestOption funcs applied to the SDK
		// client; for the raw-HTTP path we apply what we can (the profile headers
		// above cover the identity set). SDK-specific options are no-ops here.
		_ = o
	}

	// bodyclose cannot track closes inside goroutines; the 2xx body is closed
	// via defer in the drain goroutine below (must stay open for SSE streaming);
	// the non-2xx body is closed synchronously at the status check below.
	resp, err := httpClient.Do(req) //nolint:bodyclose // closed at the status check / in drain goroutine
	if err != nil {
		return nil, fmt.Errorf("anthropic provider stream send: %w", err)
	}

	// Non-2xx rejection (Pitfall 1, 19-RESEARCH): the error body is plain JSON,
	// not SSE — draining it would skip every non-`data:` line and sendDone would
	// fabricate an empty end_turn. Read a bounded slice, close the body
	// synchronously, classify through ClassifyHTTP (the surfaced error carries a
	// Kind), and emit an error chunk the turn loop already consumes (the
	// chunkErrorType case). The drain goroutine never starts for a rejected
	// request.
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		ech := make(chan StreamChunk, 1)
		ech <- StreamChunk{Type: chunkError, Error: rejectStreamError(resp, prof)}
		close(ech)

		return ech, nil
	}

	ch := make(chan StreamChunk, mnd8)
	mon := newSSEBody(resp.Body)
	idle := p.idleTimeout

	go func() {
		defer func() { _ = resp.Body.Close() }()

		stop := make(chan struct{})
		defer close(stop)

		go sseWatchdog(ctx, mon, idle, stop)

		defer close(ch)

		p.drainSSE(ctx, mon, ch)
	}()

	return ch, nil
}

// maxErrorBodyBytes bounds the error-envelope read at the non-2xx check site
// (T-19-03): the body is externally-controlled input — a bounded read, never a
// scan of the whole body.
const maxErrorBodyBytes = 8 << 10

// anthropicErrorEnvelope is the Anthropic error body shape:
// {"type":"error","error":{"type":"invalid_request_error","message":"..."}}.
type anthropicErrorEnvelope struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// rejectStreamError reads a bounded slice of a non-2xx response body, CLOSES it
// at the status-check site, and classifies the failure through ClassifyHTTP
// (400 lands in KindStructural via structuralStatuses — D-04: the surfaced
// error carries a Kind). The provider's error type + message ride the Cause so
// message-class predicates (IsOverflow) can match over them.
//
// A malformed, truncated, or empty body degrades to the generic classified
// ProviderError carrying the status code — the parse never panics (T-19-03
// mitigation; the endpoint is the already-trusted TLS provider channel).
func rejectStreamError(resp *http.Response, prof *profile.Profile) *ProviderError {
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	_ = resp.Body.Close()

	var cause error

	if envErr := errorEnvelopeCause(raw); envErr != nil {
		cause = envErr
	} else if readErr != nil {
		cause = fmt.Errorf("error body unreadable: %w", readErr)
	}

	return ClassifyHTTP(providerAnthropic, prof.Model, resp.StatusCode, cause)
}

// errorEnvelopeCause extracts the provider's error type + message as a plain
// error, or nil when the body is empty, malformed, or carries no message (the
// caller degrades to the generic structural classification).
func errorEnvelopeCause(raw []byte) error {
	if len(raw) == 0 {
		return nil
	}

	var env anthropicErrorEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Error.Message == "" {
		return nil
	}

	if env.Error.Type != "" {
		return fmt.Errorf("%s: %s", env.Error.Type, env.Error.Message)
	}

	return errors.New(env.Error.Message)
}

// drainSSE reads SSE `data: <json>\n\n` frames from body, parses each into a
// StreamChunk, and sends it on ch. On EOF it sends the terminal "done" chunk
// carrying the captured FinishReason + the assembled raw response, then returns
// (the caller closes ch). ctx cancellation stops the drain. A non-EOF read
// error that is NOT a cancellation emits an "error" chunk carrying a retryable
// ProviderError (the idle-watchdog abort path) — a wedged stream terminates
// loudly instead of fabricating a clean end_turn.
//
//nolint:gocognit,gocyclo,cyclop,funlen // SSE parsing is inherently complex
func (p *AnthropicProvider) drainSSE(ctx context.Context, body *sseBody, ch chan<- StreamChunk) {
	br := bufio.NewReader(body)

	var (
		finishReason string
		assembled    bytes.Buffer
	)
	assembled.WriteByte('[')

	first := true
	// Tool-use lifecycle state: content_block_start → content_block_delta
	// (input_json_delta fragments) → content_block_stop. We accumulate the
	// input JSON across deltas and emit the complete chunk on block stop.
	// emittedTools dedupes REPLAYED blocks (the Z.ai endpoint delivers each
	// tool_use block twice — the 08-09 finding: every call id appeared exactly
	// 2x in live transcripts): one emission per provider id per response.
	var (
		tuName, tuID string
		tuInput      strings.Builder
		inToolUse    bool
		emittedTools = map[string]struct{}{}
	)

	// Thinking-block lifecycle state (PAR-05, 21-03): a SIBLING of the
	// tool-use tracker, never a refactor of it. content_block_start of type
	// "thinking" opens the accumulator; thinking_delta strings concatenate
	// into thText, the signature_delta into thSignature; content_block_stop
	// emits ONE thinking chunk whose Raw carries the assembled block JSON
	// (field values assembled from the raw delta strings — D-12: never a
	// typed round-trip of provider bytes). redacted_thinking never opens the
	// accumulator — it arrives fully-formed in content_block_start and emits
	// immediately.
	var (
		thText      strings.Builder
		thSignature strings.Builder
		inThinking  bool
	)

	for {
		select {
		case <-ctx.Done():
			sendDone(ch, finishReason, assembled.Bytes())

			return
		default:
		}

		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, emittedTools, ch)
				flushThinking(ctx, &thText, &thSignature, &inThinking, ch)
				sendDone(ch, finishReason, finalizeAssembled(&assembled))

				return
			}

			if ctx.Err() != nil {
				// Cancellation (the watchdog force-closed the body; the transport
				// surfaces it as a read error) — existing cancel semantics.
				sendDone(ch, finishReason, finalizeAssembled(&assembled))

				return
			}

			flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, emittedTools, ch)
			flushThinking(ctx, &thText, &thSignature, &inThinking, ch)
			sendAbortError(ctx, ch, body.abortCause(), err)

			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, emittedTools, ch)
			flushThinking(ctx, &thText, &thSignature, &inThinking, ch)
			sendDone(ch, finishReason, finalizeAssembled(&assembled))

			return
		}

		var ev map[string]any

		err = json.Unmarshal([]byte(payload), &ev)
		if err != nil {
			continue
		}

		if !first {
			assembled.WriteByte(',')
		}

		first = false

		assembled.WriteString(payload)

		// Handle tool-use + thinking lifecycle events (multi-event state
		// machines; the thinking tracker is sibling state beside the tool-use
		// one — neither touches the other's behavior).
		evType, _ := ev[keyType].(string)
		switch evType {
		case "content_block_start":
			cb, _ := ev["content_block"].(map[string]any)
			if cb != nil {
				t, _ := cb[keyType].(string)
				if t == blockToolUse {
					// flush previous if unclosed
					flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, emittedTools, ch)
					tuName, _ = cb["name"].(string)
					tuID, _ = cb["id"].(string)

					tuInput.Reset()

					inToolUse = true

					continue // don't emit yet — wait for deltas
				}
				if t == chunkTypeThinking {
					// flush previous if unclosed
					flushThinking(ctx, &thText, &thSignature, &inThinking, ch)
					thText.Reset()
					thSignature.Reset()

					inThinking = true

					continue // don't emit yet — wait for deltas
				}
				if t == blockRedactedThinking {
					// Fully-formed in the start event: flush any open block, then
					// emit immediately — no deltas follow (RESEARCH Pattern 4).
					flushThinking(ctx, &thText, &thSignature, &inThinking, ch)

					data, _ := cb["data"].(string)
					emitThinking(ctx, ch, marshalThinkingBlock(blockRedactedThinking, "", "", data))

					continue
				}
			}
		case "content_block_delta":
			delta, _ := ev["delta"].(map[string]any)
			if delta != nil && inToolUse {
				if dt, _ := delta[keyType].(string); dt == "input_json_delta" {
					if pj, _ := delta["partial_json"].(string); pj != "" {
						tuInput.WriteString(pj)
					}
				}
			}
			if delta != nil && inThinking {
				dt, _ := delta[keyType].(string)
				switch dt {
				case deltaThinking:
					if s, _ := delta[chunkTypeThinking].(string); s != "" {
						thText.WriteString(s)
					}
				case deltaSignature:
					if s, _ := delta["signature"].(string); s != "" {
						thSignature.WriteString(s)
					}
				}
			}
		case "content_block_stop":
			if inToolUse {
				flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, emittedTools, ch)

				continue
			}
			if inThinking {
				flushThinking(ctx, &thText, &thSignature, &inThinking, ch)

				continue
			}
		}

		chunk, fr := parseAnthropicSSEEvent(ev)
		if fr != "" {
			finishReason = fr
		}

		if chunk != nil {
			select {
			case ch <- *chunk:
			case <-ctx.Done():
				sendDone(ch, finishReason, finalizeAssembled(&assembled))

				return
			}
		}
	}
}

// flushToolUse emits the accumulated tool-use chunk if one is active AND its id
// has not been emitted for this response yet, then resets the state. Called on
// content_block_stop, EOF, error, and [DONE]. The dedupe collapses the Z.ai
// endpoint's replayed tool_use blocks to exactly-once per provider id.
func flushToolUse(
	ctx context.Context, name, id *string, input *strings.Builder, inUse *bool,
	emitted map[string]struct{}, ch chan<- StreamChunk,
) {
	if !*inUse {
		return
	}

	*inUse = false

	defer func() {
		*name = ""
		*id = ""

		input.Reset()
	}()

	if _, dup := emitted[*id]; dup {
		// Replayed block (same provider id delivered twice): the first emission
		// already carried this call; a second tool_use chunk would duplicate the
		// call into every consumer batch (request-shape divergence).
		return
	}

	emitted[*id] = struct{}{}

	in := tuInputBytes(input)
	if len(in) == 0 {
		in = json.RawMessage("{}")
	}

	// The ToolCall carries the REAL provider id (08-07): pairing of a
	// subsequent tool_result with this tool_use depends on it. ToolCallID stays
	// on the chunk for the bus event consumers.
	chunk := StreamChunk{
		Type: blockToolUse, ToolCall: &ToolCall{ID: *id, Name: *name, Input: in}, ToolCallID: *id,
	}
	select {
	case ch <- chunk:
	case <-ctx.Done():
	}
}

// tuInputBytes returns the accumulated input JSON, or nil if empty.
func tuInputBytes(b *strings.Builder) json.RawMessage {
	s := b.String()
	if s == "" {
		return nil
	}

	return json.RawMessage(s)
}

// flushThinking emits the accumulated thinking chunk if one is active, then
// resets the state (PAR-05, 21-03). Called on content_block_stop, EOF, error,
// and [DONE] — the SAME termination call sites flushToolUse uses — so a block
// whose stop never arrives still flushes exactly once, and never twice.
func flushThinking(
	ctx context.Context, text, sig *strings.Builder, inUse *bool, ch chan<- StreamChunk,
) {
	if !*inUse {
		return
	}

	*inUse = false

	defer func() {
		text.Reset()
		sig.Reset()
	}()

	emitThinking(ctx, ch, marshalThinkingBlock(chunkTypeThinking, text.String(), sig.String(), ""))
}

// emitThinking sends one thinking chunk (bounded by ctx, like flushToolUse).
func emitThinking(ctx context.Context, ch chan<- StreamChunk, raw json.RawMessage) {
	select {
	case ch <- StreamChunk{Type: chunkTypeThinking, Raw: raw}:
	case <-ctx.Done():
	}
}

// thinkingBlockJSON is the assembly view for the stored thinking-block bytes.
// The PAR-05 precision contract is FIELD-VALUE identity, not envelope
// identity: the transcript stores these assembled bytes whose field values
// equal the provider's delta-concatenated values (built from the raw decoded
// delta STRINGS, never by round-tripping provider JSON through a typed
// unmarshal/remarshal — D-12).
type thinkingBlockJSON struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"`
}

// marshalThinkingBlock assembles one thinking-block payload from the raw
// accumulated strings. A missing signature stores verbatim (the provider's own
// bytes are the truth — an empty signature field is omitted, never invented).
func marshalThinkingBlock(typ, thinking, signature, data string) json.RawMessage {
	raw, err := json.Marshal(thinkingBlockJSON{
		Type: typ, Thinking: thinking, Signature: signature, Data: data,
	})
	if err != nil { // all-string fields: unreachable, kept defensive
		return json.RawMessage(`{"type":"` + typ + `"}`)
	}

	return raw
}

// parseAnthropicSSEEvent turns one decoded SSE event into a StreamChunk (and/or
// captures the stop reason). Tool-use events are handled by the lifecycle state
// machine in drainSSE (content_block_start → input_json_delta → content_block_stop);
// this function handles only the simple single-event types.
func parseAnthropicSSEEvent(ev map[string]any) (*StreamChunk, string) { //nolint:gocritic // conflicts w/ nonamedreturns
	typ, _ := ev[keyType].(string)
	switch typ {
	case "message_start":
		if msg, ok := ev["message"].(map[string]any); ok {
			if u, ok := msg["usage"].(map[string]any); ok {
				return &StreamChunk{Type: "usage", Usage: usageFromMap(u)}, ""
			}
		}
	case "content_block_delta":
		delta, _ := ev["delta"].(map[string]any)
		if delta != nil {
			if dt, _ := delta[keyType].(string); dt == "text_delta" {
				if text, _ := delta["text"].(string); text != "" {
					return &StreamChunk{Type: "text", Text: text}, ""
				}
			}
		}
	case "message_delta":
		if delta, _ := ev["delta"].(map[string]any); delta != nil {
			if sr, _ := delta["stop_reason"].(string); sr != "" {
				return nil, sr
			}
		}
	}

	return nil, ""
}

// usageFromMap extracts token counts from a decoded usage object.
func usageFromMap(u map[string]any) *Usage {
	out := &Usage{}
	if v, ok := u["input_tokens"].(float64); ok {
		out.InputTokens = int64(v)
	}

	if v, ok := u["output_tokens"].(float64); ok {
		out.OutputTokens = int64(v)
	}

	return out
}

// sendDone emits the terminal "done" chunk carrying the FinishReason + raw bytes.
func sendDone(ch chan<- StreamChunk, finishReason string, raw json.RawMessage) {
	if finishReason == "" {
		finishReason = "end_turn"
	}

	select {
	case ch <- StreamChunk{Type: "done", FinishReason: finishReason, Raw: raw}:
	default:
	}
}

// sendAbortError emits the mid-stream abort chunk: a retryable ProviderError
// wrapping the abort cause (idle-watchdog timeout or transport failure). No
// "done" follows — the consumers must not mistake an aborted stream for a
// completed response. Unlike sendDone this send BLOCKS (bounded by ctx): a
// dropped abort chunk would let a truncated stream masquerade as complete.
func sendAbortError(ctx context.Context, ch chan<- StreamChunk, cause, readErr error) {
	if cause == nil {
		cause = readErr
	}

	perr := ClassifyHTTP(providerAnthropic, modelGLM52, 0, cause)
	perr.Reason = "sse stream aborted: " + cause.Error()

	select {
	case ch <- StreamChunk{Type: chunkError, Error: perr}:
	case <-ctx.Done():
	}
}

// finalizeAssembled closes the JSON array bracket on the assembled raw payload.
func finalizeAssembled(b *bytes.Buffer) json.RawMessage {
	b.WriteByte(']')

	return json.RawMessage(b.Bytes())
}

// injectStreamTrue patches a marshaled Anthropic request body to set stream:true.
// It re-decodes/marshals so the result is valid JSON.
func injectStreamTrue(body []byte) []byte {
	var m map[string]any

	err := json.Unmarshal(body, &m)
	if err != nil {
		return body
	}

	m["stream"] = true

	out, err := json.Marshal(m)
	if err != nil {
		return body
	}

	return out
}

// headersFromProfile builds the header map the RequestCapturer sees for the
// streaming path (the identity header NAMES from the profile + auth). Mirrors
// the non-streaming Send's capture hook.
func headersFromProfile(prof *profile.Profile) map[string]string {
	out := map[string]string{}
	for _, h := range prof.Headers {
		out[h.Name] = shaper.RenderHeaderValue(h.ValueTemplate)
	}

	return out
}
