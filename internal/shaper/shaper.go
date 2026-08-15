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

const asciiDelete = 0x40
const uuidVariantSet = 0x80
const variantMask = 0x3F
const versionMask = 0x0F
const roleTool = "tool"

// Message is one conversational turn shaped into the outgoing request. It is
// defined here (not in the provider package) to keep the dependency edge
// one-directional: provider imports the Shaper; the Shaper never imports the
// provider, so the message type lives with the layer that consumes it.
//
// The structured mid-turn fields (ToolCalls, ToolCallID, ToolName, IsError)
// follow the CAPTURED zcode-normalized forms (08-07, VERIFIED-FACTS.md item #1
// and the rollout request.messages capture): an assistant response batch is
// {"role":"assistant","content":"","toolCalls":[{"id","name","input"}]} and a
// tool result is {"role":"tool","content":"<text>","toolCallId":…,
// "toolName":…,"isError":false}. Text-only user/assistant messages shape
// exactly as before (backward compatibility).
type Message struct {
	Role    string
	Content string
	// ToolCalls is the assistant mid-turn batch (empty for plain text turns).
	ToolCalls []ToolCall
	// ToolCallID/ToolName/IsError describe one tool-role result message.
	ToolCallID string
	ToolName   string
	IsError    bool
}

// ToolCall is one zcode-normalized tool invocation (VERIFIED-FACTS.md item #1:
// the captured response.toolCalls[] shape is {id, name, input}). It is defined
// here (like Message) so the provider seam can alias it — the REAL provider id
// is the join key that pairs a tool_use block with its tool_result block
// (08-07: the 08-06 gate's root cause 1 was this id being dropped). Input is
// the raw JSON arguments.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
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

// toMessageParams maps Messages to Anthropic MessageParams. Text-only messages
// render exactly one text block (byte-identical to the pre-08-07 shaping).
// Mid-turn structured messages render natively per protocol: an assistant
// batch becomes text-block-first + one tool_use block per ToolCall; consecutive
// tool-role messages GROUP into one user-role param carrying one tool_result
// block per message (the canonical Anthropic batch form — the capture's
// per-result message granularity is preserved by block order).
func toMessageParams(messages []Message) ([]anthropic.MessageParam, error) {
	out := make([]anthropic.MessageParam, 0, len(messages))

	for i := 0; i < len(messages); {
		m := messages[i]

		if strings.EqualFold(m.Role, roleTool) {
			// Group this tool-role message with every consecutive tool-role
			// message into ONE user param (never emitted as an Anthropic role).
			blocks := []anthropic.ContentBlockParamUnion{toolResultBlock(&m)}

			j := i + 1
			for j < len(messages) && strings.EqualFold(messages[j].Role, roleTool) {
				blocks = append(blocks, toolResultBlock(&messages[j]))
				j++
			}

			out = append(out, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleUser,
				Content: blocks,
			})
			i = j

			continue
		}

		role, err := toMessageParamRole(m.Role)
		if err != nil {
			return nil, err
		}

		blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(m.ToolCalls))
		if len(m.ToolCalls) == 0 {
			// Text-only: exactly the pre-08-07 rendering (byte-compat).
			blocks = append(blocks, anthropic.NewTextBlock(m.Content))
		} else {
			// Assistant batch: optional text block FIRST, then tool_use blocks.
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}

			for _, tc := range m.ToolCalls {
				input, perr := parseToolCallInput(tc.Input)
				if perr != nil {
					return nil, fmt.Errorf("tool %q input: %w", tc.Name, perr)
				}

				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
			}
		}

		out = append(out, anthropic.MessageParam{Role: role, Content: blocks})
		i++
	}

	return out, nil
}

// toolResultBlock renders one tool-role Message as a native tool_result block.
func toolResultBlock(m *Message) anthropic.ContentBlockParamUnion {
	return anthropic.NewToolResultBlock(m.ToolCallID, m.Content, m.IsError)
}

// RenderToolResultParam renders ONE tool-role Message to the Anthropic-native
// MessageParam (user role + tool_result block). It is the single block
// construction shared by the Shaper path and the provider's ToolResultMessage
// (08-07: the PROV-02 surface and the turn-loop rendering cannot diverge).
func RenderToolResultParam(m *Message) anthropic.MessageParam {
	return anthropic.MessageParam{
		Role:    anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{toolResultBlock(m)},
	}
}

// parseToolCallInput decodes a ToolCall's raw JSON Input into the any the SDK's
// NewToolUseBlock takes. Empty input shapes as an empty object.
func parseToolCallInput(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}

	var v any

	err := json.Unmarshal(raw, &v)
	if err != nil {
		return nil, fmt.Errorf("parse tool-call input: %w", err)
	}

	return v, nil
}

func toMessageParamRole(role string) (anthropic.MessageParamRole, error) {
	switch strings.ToLower(role) {
	case "user":
		return anthropic.MessageParamRoleUser, nil
	case "assistant":
		return anthropic.MessageParamRoleAssistant, nil
	case roleTool:
		// "tool" never appears as an Anthropic role — toMessageParams maps it
		// to a user-role param carrying tool_result blocks at the grouping
		// site. The case exists so the accepted-role set is explicit.
		return anthropic.MessageParamRoleUser, nil
	case "system":
		// Mid-conversation system messages appear in the CAPTURED zcode
		// request.messages (08-07 fixture). The Anthropic Messages API has no
		// mid-conversation system role — the claude-code-compat convention
		// rides the content as a user-role text block on the wire.
		return anthropic.MessageParamRoleUser, nil
	default:
		//nolint:err113 // dynamic error message
		return "", fmt.Errorf("unsupported message role %q (want user|assistant|tool|system)", role)
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
		return anthropic.ToolInputSchemaParam{}, fmt.Errorf("call: %w", err)
	}

	var props any
	if len(s.Properties) > 0 {
		err := json.Unmarshal(s.Properties, &props)
		if err != nil {
			return anthropic.ToolInputSchemaParam{}, fmt.Errorf("call: %w", err)
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

	b[6] = (b[6] & versionMask) | asciiDelete
	b[8] = (b[8] & variantMask) | uuidVariantSet

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
