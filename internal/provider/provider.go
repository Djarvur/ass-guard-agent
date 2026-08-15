package provider

import (
	"context"
	"encoding/json"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// Message is one conversational turn. It is an alias of shaper.Message so the
// Shaper and Provider share one type without a circular import (the provider
// imports the Shaper; the Shaper never imports the provider).
type Message = shaper.Message

// Provider is the protocol-agnostic model interface (PROV-01/PROV-02). Both
// Anthropic-shape and OpenAI-shape adapters implement it; the Turn Loop and the
// parity harness depend only on this interface.
type Provider interface {
	// Send shapes the profile + messages into an outgoing request, sends it to
	// the provider, and returns the parsed tool-calls. The shaped outgoing
	// request is observable via the RequestCapturer hook when set (LOG-01).
	Send(ctx context.Context, prof *profile.Profile, messages []Message) (Response, error)
	// Stream shapes the profile + messages, opens a streaming SSE request, and
	// returns a channel of StreamChunks (ACP-04 — provider SSE → bus →
	// session/update, NO full-turn buffering). The channel emits text/tool_use/
	// usage chunks as they arrive, then a single "done" chunk carrying the
	// FinishReason, then closes. ctx cancellation aborts the request and closes
	// the channel promptly.
	Stream(ctx context.Context, prof *profile.Profile, messages []Message) (<-chan StreamChunk, error)
	// ToolResultMessage builds the provider-native follow-up message that closes
	// a tool-call loop (PROV-02 TranslateFromInternal). The returned bytes are
	// the marshaled native message — each protocol's shape (Anthropic user+
	// tool_result; OpenAI role:tool + tool_call_id).
	ToolResultMessage(toolCallID string, result json.RawMessage) (json.RawMessage, error)
}

// StreamChunk is one streamed fragment of a model response. Type discriminates:
//   - "text": a text delta (Text is the fragment)
//   - "tool_use": a tool invocation (ToolCall is set)
//   - "usage": a token-usage update (Usage is set)
//   - "error": the stream aborted mid-flight (Error carries the cause — e.g.
//     the idle watchdog's retryable timeout). No "done" follows an "error".
//   - "done": the terminal chunk; FinishReason carries the provider's stop reason
//     and Raw carries the assembled raw response bytes for the audit log.
type StreamChunk struct {
	Type         string
	Text         string
	ToolCall     *ToolCall
	ToolCallID   string // set on tool_use chunks (the provider's tool-call id)
	Usage        *Usage
	FinishReason string
	Raw          json.RawMessage
	// Error is set only on "error" chunks (mid-stream abort — the SSE-stall
	// finding's surface: a wedged stream terminates loudly with a retryable
	// error instead of fabricating a clean end_turn).
	Error error
}

// Usage is the token accounting for a turn (or partial).
type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// Response is the zcode-normalized model response.
type Response struct {
	// ToolCalls is the ordered list of tool calls the model selected.
	ToolCalls []ToolCall
	// FinishReason is the provider's stop reason (e.g. "tool_use", "stop").
	FinishReason string
	// Raw is the verbatim response body, captured for the audit log.
	Raw json.RawMessage
}

// ToolCall is one zcode-normalized tool invocation (VERIFIED-FACTS.md item #1:
// the captured response.toolCalls[] shape is {id, name, input}). It is an alias
// of shaper.ToolCall — the same one-directional pattern as Message (provider
// imports the Shaper; the Shaper never imports the provider) — so the REAL
// provider tool-call id carries end-to-end: streaming parses it, the
// transcript records it, and the Projector pairs tool_use with tool_result
// (08-07; the 08-06 gate's root cause 1 was a local no-ID struct here).
type ToolCall = shaper.ToolCall

// RequestCapturer is invoked by an adapter with the verbatim outgoing request
// body + header set after the Shaper produces them and before the provider
// sends. The audit log (LOG-01, Plan 01-05) subscribes via this hook.
type RequestCapturer func(body []byte, headers map[string]string)
