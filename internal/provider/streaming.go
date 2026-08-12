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

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// httpClient is the streaming-path HTTP client. A shared default client is fine
// (Go's http.Transport pools connections). Tests override the endpoint via
// WithAnthropicBaseURL against an httptest server.
var httpClient = &http.Client{} //nolint:gochecknoglobals // shared HTTP client (connection pooling)

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
// The channel is buffered small (8); the turn loop drains it. A slow consumer
// is fine — the HTTP body read blocks until the consumer drains (backpressure
// propagates to the provider via the TCP window).
func (p *AnthropicProvider) Stream(ctx context.Context, prof profile.Profile, messages []Message) (<-chan StreamChunk, error) {
	key := p.apiKey
	if key == "" {
		key = os.Getenv("ZAI_API_KEY")
	}

	if key == "" {
		return nil, errors.New("anthropic provider: no API key (set ZAI_API_KEY or pass WithAnthropicAPIKey)")
	}

	if p.shaper == nil {
		return nil, errors.New("anthropic provider: nil Shaper")
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

	// bodyclose cannot track closes inside goroutines; the body is closed via
	// defer in the drain goroutine below (must stay open for SSE streaming).
	resp, err := httpClient.Do(req) //nolint:bodyclose // closed in drain goroutine
	if err != nil {
		return nil, fmt.Errorf("anthropic provider stream send: %w", err)
	}

	ch := make(chan StreamChunk, 8)

	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(ch)

		p.drainSSE(ctx, resp.Body, ch)
	}()

	return ch, nil
}

// drainSSE reads SSE `data: <json>\n\n` frames from body, parses each into a
// StreamChunk, and sends it on ch. On EOF it sends the terminal "done" chunk
// carrying the captured FinishReason + the assembled raw response, then returns
// (the caller closes ch). ctx cancellation stops the drain.
func (p *AnthropicProvider) drainSSE(ctx context.Context, body io.Reader, ch chan<- StreamChunk) {
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
	var (
		tuName, tuID string
		tuInput      strings.Builder
		inToolUse    bool
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
			if err == io.EOF {
				flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, ch)
				sendDone(ch, finishReason, finalizeAssembled(&assembled))

				return
			}

			flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, ch)
			sendDone(ch, finishReason, finalizeAssembled(&assembled))

			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, ch)
			sendDone(ch, finishReason, finalizeAssembled(&assembled))

			return
		}

		var ev map[string]any
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}

		if !first {
			assembled.WriteByte(',')
		}

		first = false

		assembled.WriteString(payload)

		// Handle tool-use lifecycle events (multi-event state machine).
		evType, _ := ev[keyType].(string)
		switch evType {
		case "content_block_start":
			cb, _ := ev["content_block"].(map[string]any)
			if cb != nil {
				if t, _ := cb[keyType].(string); t == blockToolUse {
					flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, ch) // flush previous if unclosed
					tuName, _ = cb["name"].(string)
					tuID, _ = cb["id"].(string)

					tuInput.Reset()

					inToolUse = true

					continue // don't emit yet — wait for deltas
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
		case "content_block_stop":
			if inToolUse {
				flushToolUse(ctx, &tuName, &tuID, &tuInput, &inToolUse, ch)

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

// flushToolUse emits the accumulated tool-use chunk if one is active, then
// resets the state. Called on content_block_stop, EOF, error, and [DONE].
func flushToolUse(ctx context.Context, name, id *string, input *strings.Builder, inUse *bool, ch chan<- StreamChunk) {
	if !*inUse {
		return
	}

	*inUse = false

	in := tuInputBytes(input)
	if len(in) == 0 {
		in = json.RawMessage("{}")
	}

	chunk := StreamChunk{Type: blockToolUse, ToolCall: &ToolCall{Name: *name, Input: in}, ToolCallID: *id}
	select {
	case ch <- chunk:
	case <-ctx.Done():
	}

	*name = ""
	*id = ""

	input.Reset()
}

// tuInputBytes returns the accumulated input JSON, or nil if empty.
func tuInputBytes(b *strings.Builder) json.RawMessage {
	s := b.String()
	if s == "" {
		return nil
	}

	return json.RawMessage(s)
}

// parseAnthropicSSEEvent turns one decoded SSE event into a StreamChunk (and/or
// captures the stop reason). Tool-use events are handled by the lifecycle state
// machine in drainSSE (content_block_start → input_json_delta → content_block_stop);
// this function handles only the simple single-event types.
func parseAnthropicSSEEvent(ev map[string]any) (*StreamChunk, string) {
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

// finalizeAssembled closes the JSON array bracket on the assembled raw payload.
func finalizeAssembled(b *bytes.Buffer) json.RawMessage {
	b.WriteByte(']')

	return json.RawMessage(b.Bytes())
}

// injectStreamTrue patches a marshaled Anthropic request body to set stream:true.
// It re-decodes/marshals so the result is valid JSON.
func injectStreamTrue(body []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
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
func headersFromProfile(prof profile.Profile) map[string]string {
	out := map[string]string{}
	for _, h := range prof.Headers {
		out[h.Name] = shaper.RenderHeaderValue(h.ValueTemplate)
	}

	return out
}
