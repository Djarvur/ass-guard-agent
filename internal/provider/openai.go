package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/sashabaranov/go-openai"

	"github.com/djarvur/ass-guard-agent/internal/profile"
)

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
func (p *OpenAIProvider) Send(ctx context.Context, prof profile.Profile, messages []Message) (Response, error) {
	key := p.apiKey
	if key == "" {
		key = os.Getenv("OPENAI_API_KEY")
	}
	if key == "" {
		return Response{}, errors.New("openai provider: no API key (set OPENAI_API_KEY or pass WithOpenAIAPIKey)")
	}

	req := p.buildRequest(prof, messages)

	if p.capture != nil {
		if body, err := json.Marshal(req); err == nil {
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
	return parseOpenAIResponse(resp)
}

// buildRequest constructs the Chat Completions request: messages → ChatCompletionMessage,
// profile tools → openai.Tool{Type:function, Function:{Name,Description,Parameters}},
// tool_choice from the profile, model from the provider override or the profile.
func (p *OpenAIProvider) buildRequest(prof profile.Profile, messages []Message) openai.ChatCompletionRequest {
	model := p.model
	if model == "" {
		model = prof.Model
	}
	msgs := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, m := range messages {
		msgs = append(msgs, openai.ChatCompletionMessage{Role: m.Role, Content: m.Content})
	}
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
	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: msgs,
		Tools:    tools,
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

// parseOpenAIResponse turns the Chat Completions response into the
// zcode-normalized Response. Each ToolCall.Function.Arguments (a JSON-encoded
// STRING per the OpenAI spec — VERIFIED-FACTS.md item #2) is parsed into a
// json.RawMessage ToolCall.Input.
func parseOpenAIResponse(resp openai.ChatCompletionResponse) (Response, error) {
	out := Response{FinishReason: string(resp.Choices[0].FinishReason)}
	if len(resp.Choices) == 0 {
		return out, errors.New("openai provider: response has no choices")
	}
	for _, tc := range resp.Choices[0].Message.ToolCalls {
		var input json.RawMessage
		args := []byte(tc.Function.Arguments)
		if len(args) == 0 {
			input = json.RawMessage("{}")
		} else if json.Valid(args) {
			input = json.RawMessage(args)
		} else {
			// The spec says Arguments is a JSON-encoded string; defensively wrap
			// a non-JSON value as a JSON string so the consumer always sees valid JSON.
			wrapped, _ := json.Marshal(tc.Function.Arguments)
			input = json.RawMessage(wrapped)
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{Name: tc.Function.Name, Input: input})
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
	return json.Marshal(msg)
}

// Stream is not implemented for the OpenAI-shape adapter in Phase 2 (the
// streaming path is Anthropic-shape only — ACP-04 is exercised against the Z.ai
// GLM Anthropic endpoint). It returns a clear error so callers do not silently
// fall back to a non-streaming shape. OpenAI-shape streaming lands in a later
// phase if a non-Anthropic streaming provider becomes a target.
func (p *OpenAIProvider) Stream(ctx context.Context, prof profile.Profile, messages []Message) (<-chan StreamChunk, error) {
	return nil, errOpenAIStreamNotImplemented
}

// errOpenAIStreamNotImplemented is the sentinel returned by the OpenAI adapter's
// Stream (Phase 2 scope: Anthropic-shape streaming only).
var errOpenAIStreamNotImplemented = errors.New("openai provider: streaming not implemented in Phase 2 (ACP-04 is Anthropic-shape only)")

// compile-time interface check.
var _ Provider = (*OpenAIProvider)(nil)
