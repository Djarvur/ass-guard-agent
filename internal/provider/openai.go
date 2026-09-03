package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

var errOpenaiNoAPI = errors.New("openai provider: no API key (set OPENAI_API_KEY or pass WithOpenAIAPIKey)")
var errOpenaiResponseHas = errors.New("openai provider: response has no choices")

// OpenAIDefaultBaseURL is the canonical OpenAI Chat Completions endpoint. Z.ai,
// MiniMax, Groq, and OpenRouter all accept this shape with a base-URL swap
// (PROV-01 — configurable base URL).
const OpenAIDefaultBaseURL = "https://api.openai.com/v1"

// OpenAIProvider sends requests to an OpenAI-shape Chat Completions endpoint
// (PROV-03 secondary) and parses tool_calls into zcode-normalized ToolCalls.
// It targets Chat Completions (`/chat/completions`), NOT the Responses API
// (VERIFIED-FACTS.md item #2). It implements Provider.
type OpenAIProvider struct {
	apiKey  string
	baseURL string
	model   string // optional override; default = profile.Model
	capture RequestCapturer
}

// OpenAIOption configures an OpenAIProvider.
type OpenAIOption func(*OpenAIProvider)

// WithOpenAIAPIKey overrides the API key (default: $OPENAI_API_KEY).
func WithOpenAIAPIKey(key string) OpenAIOption {
	return func(p *OpenAIProvider) { p.apiKey = key }
}

// WithOpenAIBaseURL overrides the base URL (default: the OpenAI endpoint).
func WithOpenAIBaseURL(url string) OpenAIOption {
	return func(p *OpenAIProvider) { p.baseURL = url }
}

// WithOpenAIModel overrides the model (default: the profile's model).
func WithOpenAIModel(model string) OpenAIOption {
	return func(p *OpenAIProvider) { p.model = model }
}

// WithOpenAIRequestCapture installs a RequestCapturer (LOG-01 hook).
func WithOpenAIRequestCapture(c RequestCapturer) OpenAIOption {
	return func(p *OpenAIProvider) { p.capture = c }
}

// NewOpenAIProvider builds an OpenAIProvider from options.
func NewOpenAIProvider(opts ...OpenAIOption) *OpenAIProvider {
	p := &OpenAIProvider{baseURL: OpenAIDefaultBaseURL}
	for _, o := range opts {
		o(p)
	}

	return p
}

// Send builds a Chat Completions request from the profile + messages, posts it,
// and parses Choices[0].Message.ToolCalls into zcode-normalized ToolCalls.
func (p *OpenAIProvider) Send(ctx context.Context, prof *profile.Profile, messages []Message) (Response, error) {
	key := p.apiKey
	if key == "" {
		key = os.Getenv("OPENAI_API_KEY")
	}

	if key == "" {
		return Response{}, errOpenaiNoAPI
	}

	req := p.buildRequest(prof, messages)

	if p.capture != nil {
		body, err := json.Marshal(req)
		if err == nil {
			p.capture(body, nil)
		}
	}

	cfg := openai.DefaultConfig(key)
	if p.baseURL != "" {
		cfg.BaseURL = p.baseURL
	}

	client := openai.NewClientWithConfig(cfg)

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return Response{}, fmt.Errorf("openai provider send: %w", err)
	}

	return parseOpenAIResponse(&resp)
}

// buildRequest constructs the Chat Completions request: messages → ChatCompletionMessage,
// profile tools → openai.Tool{Type:function, Function:{Name,Description,Parameters}},
// tool_choice from the profile, model from the provider override or the profile.
//
//nolint:funcorder // ordering groups related logic
func (p *OpenAIProvider) buildRequest(prof *profile.Profile, messages []Message) openai.ChatCompletionRequest {
	model := p.model
	if model == "" {
		model = prof.Model
	}

	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: toOpenAIMessages(messages),
		Tools:    openAITools(prof),
	}
	// tool_choice: only "auto"/"none"/"required" map directly; object forms are
	// provider-specific and out of Phase-1 scope.
	if prof.ToolChoice != nil {
		var tc struct {
			Type string `json:"type"`
		}

		if json.Unmarshal(prof.ToolChoice, &tc) == nil {
			req.ToolChoice = tc.Type
		}
	}

	return req
}

// toOpenAIMessages maps Messages to Chat Completions messages. Structured
// mid-turn messages render OpenAI-natively (08-07): an assistant batch becomes
// tool_calls[{id,type:function,function:{name,arguments}}] (arguments = the
// Input JSON verbatim) and a tool-role message becomes
// {role:"tool", tool_call_id, content}.
func toOpenAIMessages(messages []Message) []openai.ChatCompletionMessage {
	msgs := make([]openai.ChatCompletionMessage, 0, len(messages))
	for i := range messages {
		m := &messages[i]
		cm := openai.ChatCompletionMessage{Role: m.Role, Content: m.Content}

		for _, tc := range m.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Name,
					Arguments: string(tc.Input),
				},
			})
		}

		if strings.EqualFold(m.Role, "tool") {
			cm.ToolCallID = m.ToolCallID
		}

		msgs = append(msgs, cm)
	}

	return msgs
}

// openAITools maps the profile's tool declarations to the Chat Completions
// tools wrapper.
func openAITools(prof *profile.Profile) []openai.Tool {
	tools := make([]openai.Tool, 0, len(prof.Tools))

	for _, d := range prof.Tools {
		params := d.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object"}`)
		}

		tools = append(tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        d.Name,
				Description: d.Description,
				Parameters:  params,
			},
		})
	}

	return tools
}

// parseOpenAIResponse turns the Chat Completions response into the
// zcode-normalized Response. Each ToolCall.Function.Arguments (a JSON-encoded
// STRING per the OpenAI spec — VERIFIED-FACTS.md item #2) is parsed into a
// json.RawMessage ToolCall.Input.
func parseOpenAIResponse(resp *openai.ChatCompletionResponse) (Response, error) {
	out := Response{FinishReason: string(resp.Choices[0].FinishReason)}
	if len(resp.Choices) == 0 {
		return out, errOpenaiResponseHas
	}

	for _, tc := range resp.Choices[0].Message.ToolCalls {
		var input json.RawMessage

		args := []byte(tc.Function.Arguments)
		switch {
		case len(args) == 0:
			input = json.RawMessage("{}")
		case json.Valid(args):
			input = json.RawMessage(args)
		default:
			// The spec says Arguments is a JSON-encoded string; defensively wrap
			// a non-JSON value as a JSON string so the consumer always sees valid JSON.
			wrapped, mErr := json.Marshal(tc.Function.Arguments)
			if mErr != nil {
				return Response{}, fmt.Errorf("openai: marshal tool arguments: %w", mErr)
			}

			input = json.RawMessage(wrapped)
		}

		out.ToolCalls = append(out.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Input: input})
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		return out, fmt.Errorf("openai provider marshal response: %w", err)
	}

	out.Raw = raw

	return out, nil
}

// ToolResultMessage builds the OpenAI-shape follow-up: role "tool",
// tool_call_id, JSON-string content (PROV-02 TranslateFromInternal;
// VERIFIED-FACTS.md item #2).
func (p *OpenAIProvider) ToolResultMessage(toolCallID string, result json.RawMessage) (json.RawMessage, error) {
	content := string(orEmpty(result))
	if content == `""` {
		content = ""
	}

	msg := openai.ChatCompletionMessage{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolCallID,
	}

	data, mErr := json.Marshal(msg)
	if mErr != nil {
		return nil, fmt.Errorf("marshal: %w", mErr)
	}

	return data, nil
}

// SupportsImages reports the OpenAI-shape adapter's TEXT-ONLY status today
// (21-05, D-11): this adapter maps to Chat Completions string content only —
// the go-openai MultiContent surface is explicitly OUT of PAR-06's letter
// (21-RESEARCH's Alternatives row blesses drop+loud-note instead). Image
// blocks are dropped at the turn path with one loud note naming this
// provider; the turn proceeds with the text.
func (p *OpenAIProvider) SupportsImages() bool { return false }

// Stream is not implemented for the OpenAI-shape adapter in Phase 2 (the
// streaming path is Anthropic-shape only — ACP-04 is exercised against the Z.ai
// GLM Anthropic endpoint). It returns a clear error so callers do not silently
// fall back to a non-streaming shape. OpenAI-shape streaming lands in a later
// phase if a non-Anthropic streaming provider becomes a target.
func (p *OpenAIProvider) Stream(
	ctx context.Context, prof *profile.Profile, messages []Message,
) (<-chan StreamChunk, error) {
	return nil, errOpenAIStreamNotImplemented
}

// errOpenAIStreamNotImplemented is the sentinel returned by the OpenAI adapter's
// Stream (Phase 2 scope: Anthropic-shape streaming only).
var errOpenAIStreamNotImplemented = errors.New(
	"openai provider: streaming not implemented in Phase 2 (ACP-04 is Anthropic-shape only)")

// compile-time interface check.
var _ Provider = (*OpenAIProvider)(nil)
