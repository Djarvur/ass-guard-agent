// Package shaper implements the Profile Shaper — the mimicry chokepoint
// (MIMC-01). It populates anthropic-sdk-go native request types from a loaded
// profile and emits per-request identity headers via option.WithHeader (the
// D-09 escape hatch).
//
// The Shaper is profile-agnostic (PROF-02): the same code path shapes any
// profile. There are no profile-name branches, no hardcoded header names, and
// no hardcoded tool names — everything is driven by the Profile struct handed
// to Shape. The D-11 lint (grep for any profile's literal name in this file,
// excluding tests) is belt-and-suspenders.
package shaper

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// Message is one conversational turn shaped into the outgoing request. It is
// defined here (not in the provider package) to keep the dependency edge
// one-directional: provider imports the Shaper; the Shaper never imports the
// provider, so the message type lives with the layer that consumes it.
type Message struct {
	Role    string
	Content string
}

// Shaper turns a loaded Profile + conversational messages into the native SDK
// request type plus a data-driven set of per-request identity-header options.
// A zero Shaper is ready to use.
type Shaper struct{}

// New returns a ready Shaper.
func New() *Shaper { return &Shaper{} }

// Shape builds an anthropic.MessageNewParams from the profile and messages, and
// returns one option.WithHeader per profile.Headers entry (the D-09 escape
// hatch for the identity headers the SDK types do not model). Every populated
// field is read from the profile in a loop — no profile-specific literals.
func (s *Shaper) Shape(
	p *profile.Profile, messages []Message,
) (anthropic.MessageNewParams, []option.RequestOption, error) {
	systemBlocks := make([]anthropic.TextBlockParam, 0, len(p.System))
	for _, b := range p.System {
		systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: b.Text})
	}

	msgParams, err := toMessageParams(messages)
	if err != nil {
		return anthropic.MessageNewParams{}, nil, fmt.Errorf("shape messages: %w", err)
	}

	tools, err := toToolUnions(p.Tools)
	if err != nil {
		return anthropic.MessageNewParams{}, nil, fmt.Errorf("shape tools: %w", err)
	}

	params := anthropic.MessageNewParams{
		Model:      p.Model,
		MaxTokens:  int64(p.MaxTokens),
		System:     systemBlocks,
		Messages:   msgParams,
		Tools:      tools,
		Thinking:   toThinking(p.Thinking),
		ToolChoice: toToolChoice(p.ToolChoice),
	}

	opts := make([]option.RequestOption, 0, len(p.Headers))
	for _, h := range p.Headers {
		opts = append(opts, option.WithHeader(h.Name, RenderHeaderValue(h.ValueTemplate)))
	}

	return params, opts, nil
}

// RenderHeaderValue renders a profile header value template into the per-request
// value. Id-like placeholders get a fresh UUID (per-request ephemera); an auth
// placeholder reads the operator key from the environment; anything else uses
// the template body as a stable stand-in. Values are TIER-3 (per-session); the
// load-bearing property is presence (TIER-2), not the exact value.
func RenderHeaderValue(template string) string {
	if template == "" {
		return ""
	}

	low := strings.ToLower(template)

	switch {
	case strings.Contains(low, "auth"):
		if key := os.Getenv("ZAI_API_KEY"); key != "" {
			return "Bearer " + key
		}

		return ""
	case strings.Contains(low, "id"):
		return uuidV4()
	default:
		return strings.Trim(template, "<>")
	}
}

func toMessageParams(messages []Message) ([]anthropic.MessageParam, error) {
	out := make([]anthropic.MessageParam, 0, len(messages))

	for _, m := range messages {
		role, err := toMessageParamRole(m.Role)
		if err != nil {
			return nil, err
		}

		out = append(out, anthropic.MessageParam{
			Role:    role,
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(m.Content)},
		})
	}

	return out, nil
}

func toMessageParamRole(role string) (anthropic.MessageParamRole, error) {
	switch strings.ToLower(role) {
	case "user":
		return anthropic.MessageParamRoleUser, nil
	case "assistant":
		return anthropic.MessageParamRoleAssistant, nil
	default:
		return "", fmt.Errorf("unsupported message role %q (want user|assistant)", role)
	}
}

func toToolUnions(decls []profile.Decl) ([]anthropic.ToolUnionParam, error) {
	out := make([]anthropic.ToolUnionParam, 0, len(decls))

	for _, d := range decls {
		schema, err := toToolInputSchema(d.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q input_schema: %w", d.Name, err)
		}

		tp := &anthropic.ToolParam{
			Name:        d.Name,
			InputSchema: schema,
		}
		if d.Description != "" {
			tp.Description = param.NewOpt(d.Description)
		}

		out = append(out, anthropic.ToolUnionParam{OfTool: tp})
	}

	return out, nil
}

// toToolInputSchema maps a captured JSON schema onto the SDK's
// ToolInputSchemaParam. Properties + Required are first-class SDK fields; any
// other schema keys (e.g. additionalProperties, $schema) ride in ExtraFields so
// the schema round-trips without loss.
func toToolInputSchema(raw json.RawMessage) (anthropic.ToolInputSchemaParam, error) {
	var s struct {
		Type       string          `json:"type"`
		Properties json.RawMessage `json:"properties"`
		Required   []string        `json:"required"`
	}

	err := json.Unmarshal(raw, &s)
	if err != nil {
		return anthropic.ToolInputSchemaParam{}, err
	}

	var props any
	if len(s.Properties) > 0 {
		err := json.Unmarshal(s.Properties, &props)
		if err != nil {
			return anthropic.ToolInputSchemaParam{}, err
		}
	}

	extras := schemaExtras(raw)

	return anthropic.ToolInputSchemaParam{
		Properties:  props,
		Required:    s.Required,
		ExtraFields: extras,
	}, nil
}

// schemaExtras returns the non-{type,properties,required} keys of a captured
// schema so the SDK request carries them verbatim.
func schemaExtras(raw json.RawMessage) map[string]any {
	var m map[string]json.RawMessage

	err := json.Unmarshal(raw, &m)
	if err != nil {
		return nil
	}

	extras := make(map[string]any, len(m))

	for k, v := range m {
		if k == "type" || k == "properties" || k == "required" {
			continue
		}

		var val any

		_ = json.Unmarshal(v, &val)
		extras[k] = val
	}

	if len(extras) == 0 {
		return nil
	}

	return extras
}

func toThinking(raw json.RawMessage) anthropic.ThinkingConfigParamUnion {
	if len(raw) == 0 {
		return anthropic.ThinkingConfigParamUnion{}
	}

	var t struct {
		Type         string `json:"type"`
		BudgetTokens int64  `json:"budget_tokens"`
	}

	err := json.Unmarshal(raw, &t)
	if err != nil {
		return anthropic.ThinkingConfigParamUnion{}
	}

	switch strings.ToLower(t.Type) {
	case "enabled":
		return anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{BudgetTokens: t.BudgetTokens},
		}
	case "disabled":
		d := anthropic.NewThinkingConfigDisabledParam()

		return anthropic.ThinkingConfigParamUnion{OfDisabled: &d}
	default:
		return anthropic.ThinkingConfigParamUnion{}
	}
}

func toToolChoice(raw json.RawMessage) anthropic.ToolChoiceUnionParam {
	if len(raw) == 0 {
		return anthropic.ToolChoiceUnionParam{}
	}

	var tc struct {
		Type string `json:"type"`
	}

	err := json.Unmarshal(raw, &tc)
	if err != nil {
		return anthropic.ToolChoiceUnionParam{}
	}

	switch strings.ToLower(tc.Type) {
	case "auto":
		return anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}}
	case "any":
		return anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{}}
	case "none":
		return anthropic.ToolChoiceUnionParam{OfNone: &anthropic.ToolChoiceNoneParam{}}
	default:
		return anthropic.ToolChoiceUnionParam{}
	}
}

// uuidV4 returns an RFC 4122 v4 UUID string using crypto/rand. It panics on the
// (non-existent in practice) CSPRNG failure — ass-guard cannot run without a
// working entropy source.
func uuidV4() string {
	var b [16]byte

	_, err := rand.Read(b[:])
	if err != nil {
		panic("crypto/rand failed: " + err.Error())
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
