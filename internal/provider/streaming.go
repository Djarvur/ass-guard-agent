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

	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/shaper"
)

// httpClient is the streaming-path HTTP client. A shared default client is fine
// (Go's http.Transport pools connections). Tests override the endpoint via
// WithAnthropicBaseURL against an httptest server.
var httpClient = &http.Client{}

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
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("authorization", "Bearer "+key)
	req.Header.Set("anthropic-version", anthropicVersion)
	for _, h := range prof.Headers {
		req.Header.Set(h.Name, shaper.RenderHeaderValue(h.ValueTemplate))
	}
	for _, o := range p.extraOpts {
		// extraOpts are opaque option.RequestOption funcs applied to the SDK
		// client; for the raw-HTTP path we apply what we can (the profile headers
		// above cover the identity set). SDK-specific options are no-ops here.
		_ = o
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic provider stream send: %w", err)
	}

	ch := make(chan StreamChunk, 8)
	go func() {
		defer resp.Body.Close()
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
	var finishReason string
	var assembled bytes.Buffer
	assembled.WriteByte('[')
	first := true
	for {
		select {
		case <-ctx.Done():
			// Best-effort: emit a done chunk with whatever finish reason we have.
			sendDone(ch, finishReason, assembled.Bytes())
			return
		default:
		}
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				sendDone(ch, finishReason, finalizeAssembled(&assembled))
				return
			}
			// Network error mid-stream (often ctx abort): emit done + return.
			sendDone(ch, finishReason, finalizeAssembled(&assembled))
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			sendDone(ch, finishReason, finalizeAssembled(&assembled))
			return
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue // skip malformed frame
		}
		if !first {
			assembled.WriteByte(',')
		}
		first = false
		assembled.WriteString(payload)
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

// parseAnthropicSSEEvent turns one decoded SSE event into a StreamChunk (and/or
// captures the stop reason). Returns nil chunk if the event carries no chunk
// payload (e.g. content_block_start for text, content_block_stop, etc.).
func parseAnthropicSSEEvent(ev map[string]any) (*StreamChunk, string) {
	typ, _ := ev["type"].(string)
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
			if dt, _ := delta["type"].(string); dt == "text_delta" {
				if text, _ := delta["text"].(string); text != "" {
					return &StreamChunk{Type: "text", Text: text}, ""
				}
			}
		}
	case "content_block_start":
		if cb, _ := ev["content_block"].(map[string]any); cb != nil {
			if t, _ := cb["type"].(string); t == "tool_use" {
				name, _ := cb["name"].(string)
				id, _ := cb["id"].(string)
				// Input can arrive as json.RawMessage (if the JSON decoder kept
				// it raw) or as map[string]any (fully decoded). Handle both.
				var in json.RawMessage
				switch v := cb["input"].(type) {
				case json.RawMessage:
					in = v
				case nil:
					in = json.RawMessage("{}")
				default:
					b, _ := json.Marshal(v)
					in = b
				}
				return &StreamChunk{Type: "tool_use", ToolCall: &ToolCall{Name: name, Input: in}, ToolCallID: id}, ""
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
