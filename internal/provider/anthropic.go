package provider

import (
	"fmt"
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// AnthropicDefaultBaseURL is the Z.ai GLM Anthropic-protocol endpoint
// (VERIFIED-FACTS.md item #1: providerId "builtin:zai-coding-plan" confirms zcode
// talks to Z.ai's GLM via the Anthropic protocol at this base URL).
const AnthropicDefaultBaseURL = "https://api.z.ai/api/anthropic"

// AnthropicProvider sends shaped requests to an Anthropic-protocol endpoint
// (primary: Z.ai GLM) and parses tool_use blocks into zcode-normalized
// ToolCalls. It implements Provider (PROV-03 primary adapter).
type AnthropicProvider struct {
	shaper    *shaper.Shaper
	apiKey    string
	baseURL   string
	extraOpts []option.RequestOption
	capture   RequestCapturer
}

// AnthropicOption configures an AnthropicProvider.
type AnthropicOption func(*AnthropicProvider)

// WithAnthropicAPIKey overrides the API key (default: $ZAI_API_KEY).
func WithAnthropicAPIKey(key string) AnthropicOption {
	return func(p *AnthropicProvider) { p.apiKey = key }
}

// WithAnthropicBaseURL overrides the base URL (default: the Z.ai endpoint).
func WithAnthropicBaseURL(url string) AnthropicOption {
	return func(p *AnthropicProvider) { p.baseURL = url }
}

// WithAnthropicExtraOption appends a raw SDK request option (used for tests and
// custom HTTP clients/transports).
func WithAnthropicExtraOption(o option.RequestOption) AnthropicOption {
	return func(p *AnthropicProvider) { p.extraOpts = append(p.extraOpts, o) }
}

// WithAnthropicRequestCapture installs a RequestCapturer (LOG-01 hook).
func WithAnthropicRequestCapture(c RequestCapturer) AnthropicOption {
	return func(p *AnthropicProvider) { p.capture = c }
}

// NewAnthropicProvider builds an AnthropicProvider from the given Shaper and
// options. A zero Shaper is rejected.
func NewAnthropicProvider(s *shaper.Shaper, opts ...AnthropicOption) *AnthropicProvider {
	p := &AnthropicProvider{
		shaper:  s,
		baseURL: AnthropicDefaultBaseURL,
	}
	for _, o := range opts {
		o(p)
	}

	return p
}

// Send shapes the profile + messages, posts to the Anthropic endpoint, and
// parses tool_use blocks. Returns a clear error if no API key is configured.
//
// Implementation note: Send delegates to Stream (SSE) because Z.ai requires
// streaming for operations that may take longer than 10 minutes. The SSE
// response is drained synchronously and accumulated into a Response — callers
// see the same interface as a non-streaming call, but the wire uses stream:true.
func (p *AnthropicProvider) Send(ctx context.Context, prof *profile.Profile, messages []Message) (Response, error) {
	ch, err := p.Stream(ctx, prof, messages)
	if err != nil {
		return Response{}, err
	}

	var out Response

	for chunk := range ch {
		switch chunk.Type {
		case blockToolUse:
			if chunk.ToolCall != nil {
				out.ToolCalls = append(out.ToolCalls, *chunk.ToolCall)
			}
		case "done":
			out.FinishReason = chunk.FinishReason
			out.Raw = chunk.Raw
		}
	}

	if out.FinishReason == "" {
		out.FinishReason = "end_turn"
	}

	return out, nil
}

// ToolResultMessage builds the Anthropic-shape follow-up: a user-role message
// whose content is a tool_result block referencing the tool-call id (PROV-02
// TranslateFromInternal). The result bytes are JSON the SDK would accept as a
// user MessageParam content block.
func (p *AnthropicProvider) ToolResultMessage(toolCallID string, result json.RawMessage) (json.RawMessage, error) {
	msg := map[string]any{
		"role": "user",
		"content": []map[string]any{{
			keyType:       "tool_result",
			"tool_use_id": toolCallID,
			"content":     orEmpty(result),
		}},
	}

	data, mErr := json.Marshal(msg); if mErr != nil { return nil, fmt.Errorf("marshal: %w", mErr) }; return data, nil
}

// orEmpty returns b as-is, or a single space if empty, so JSON object fields
// never carry a null where a string is expected.
func orEmpty(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage(`""`)
	}

	return b
}

// compile-time interface check.
var _ Provider = (*AnthropicProvider)(nil)
