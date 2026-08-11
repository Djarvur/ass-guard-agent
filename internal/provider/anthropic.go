package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/shaper"
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
func (p *AnthropicProvider) Send(ctx context.Context, prof profile.Profile, messages []Message) (Response, error) {
	key := p.apiKey
	if key == "" {
		key = os.Getenv("ZAI_API_KEY")
	}
	if key == "" {
		return Response{}, errors.New("anthropic provider: no API key (set ZAI_API_KEY or pass WithAnthropicAPIKey)")
	}
	if p.shaper == nil {
		return Response{}, errors.New("anthropic provider: nil Shaper")
	}

	params, headerOpts, err := p.shaper.Shape(prof, messages)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic provider shape: %w", err)
	}

	clientOpts := []option.RequestOption{
		option.WithAPIKey(key),
		option.WithBaseURL(p.baseURL),
	}
	clientOpts = append(clientOpts, p.extraOpts...)
	clientOpts = append(clientOpts, headerOpts...)

	if p.capture != nil {
		body, _ := json.Marshal(params)
		p.capture(body, headersFromOpts(headerOpts))
	}

	client := anthropic.NewClient(clientOpts...)
	resp, err := client.Messages.New(ctx, params)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic provider send: %w", err)
	}
	return parseAnthropicResponse(resp)
}

// ToolResultMessage builds the Anthropic-shape follow-up: a user-role message
// whose content is a tool_result block referencing the tool-call id (PROV-02
// TranslateFromInternal). The result bytes are JSON the SDK would accept as a
// user MessageParam content block.
func (p *AnthropicProvider) ToolResultMessage(toolCallID string, result json.RawMessage) (json.RawMessage, error) {
	msg := map[string]any{
		"role": "user",
		"content": []map[string]any{{
			"type":        "tool_result",
			"tool_use_id": toolCallID,
			"content":     json.RawMessage(orEmpty(result)),
		}},
	}
	return json.Marshal(msg)
}

// parseAnthropicResponse turns an *anthropic.Message into the zcode-normalized
// Response: tool_use blocks become ToolCall{Name, Input json.RawMessage}; the
// stop reason is preserved; the raw response is re-marshaled for the audit log.
func parseAnthropicResponse(resp *anthropic.Message) (Response, error) {
	out := Response{
		FinishReason: string(resp.StopReason),
	}
	for _, block := range resp.Content {
		if block.Type != "tool_use" {
			continue
		}
		tc := ToolCall{Name: block.Name, Input: block.Input}
		if len(tc.Input) == 0 {
			tc.Input = json.RawMessage("{}")
		}
		out.ToolCalls = append(out.ToolCalls, tc)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		return out, fmt.Errorf("anthropic provider marshal response: %w", err)
	}
	out.Raw = raw
	return out, nil
}

// headersFromOpts is a best-effort extraction of header name→value pairs from
// the Shaper's option slice for the RequestCapturer hook. option.RequestOption
// is an opaque func; the Shaper emits exactly one WithHeader per profile header
// in profile.Headers order, so we surface the names from a parallel capture in
// production code paths that need them. For now the hook receives the body plus
// an empty header map (the audit log redacts/persists the body; full header
// capture is wired in Plan 01-05).
func headersFromOpts(_ []option.RequestOption) map[string]string {
	return map[string]string{}
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
