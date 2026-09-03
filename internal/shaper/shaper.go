package shaper

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
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
const typeThinking = "thinking"
const typeRedactedThinking = "redacted_thinking"

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
	// Blocks is the ORDERED rich-content rendering (PAR-06, 21-05): text and
	// image blocks in their original content-block order. When non-empty it
	// IS the wire rendering — the Content string stays the text-concatenation
	// view for consumers like the projector's extractText. Additive: nil for
	// every pre-image message, so legacy rendering stays byte-identical.
	Blocks []Block
	// ToolCalls is the assistant mid-turn batch (empty for plain text turns).
	ToolCalls []ToolCall
	// ThinkingBlocks carries the assistant message's provider thinking blocks
	// (PAR-05, 21-03) — additive: empty for every pre-thinking message, so
	// text/tool-only rendering stays byte-identical.
	ThinkingBlocks []ThinkingBlock
	// ToolCallID/ToolName/IsError describe one tool-role result message.
	ToolCallID string
	ToolName   string
	IsError    bool
}

// Block is one entry of a Message's ordered rich rendering: exactly one of
// Text or Image is set (content-block order is parity-relevant — an image
// between two text blocks renders in position).
type Block struct {
	Text  string
	Image *ImageBlock
}

// ImageBlock is one image content block carried on a user Message (PAR-06,
// 21-05): the Ref is the ingress-persisted path (the lean transcript line's
// dataRef) whose bytes are read AT SHAPE TIME and mapped onto the SDK's
// Base64ImageSourceParam. Only the Ref + canonical media type travel here —
// dims/resize provenance never reach the wire (the SDK param carries only the
// source).
type ImageBlock struct {
	Ref       string
	MediaType string
}

// ThinkingBlock is one provider thinking block, extracted ONCE by the
// projector (PAR-05, 21-03). Type is "thinking" (Text+Signature carry the
// signed block's field values) or "redacted_thinking" (Data carries the
// encrypted payload). Field VALUES pass through UNTOUCHED from the provider's
// bytes to the SDK param (D-14) — the SDK re-serializes; nothing here is ever
// recomputed, reordered, or filtered by type.
type ThinkingBlock struct {
	Type      string
	Text      string
	Signature string
	Data      string
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

// ComposeRuntimeWorkDir rewrites the profile's system blocks so every
// occurrence of the CAPTURED working directory (Profile.CaptureWorkDir — the
// value the runtime-composed env/system fields carried at capture time) is
// replaced by the session's actual working directory.
//
// The mimicry target composes its env block ("Primary working directory: …")
// from the RUNTIME cwd per session; replaying the captured value statically
// tells the model it stands in the capture's repo when it actually stands in
// the session's (the 08-09 E2E explore leg read the REAL repo instead of the
// scratch). The substitution is form-identical to the capture — only the cwd
// VALUE changes. No-op when the profile declares no capture_work_dir, the
// session dir is empty, or the two are equal (the TIER-1 byte-fidelity default
// stands; the parity/fidelity guards compare exactly that uncomposed form).
//
// Callers own the profile COPY they compose into (the sessionFor per-session
// copy discipline, D-16 — the shared loaded profile is never mutated).
func ComposeRuntimeWorkDir(p *profile.Profile, sessionWorkDir string) {
	if p == nil || p.CaptureWorkDir == "" || sessionWorkDir == "" || p.CaptureWorkDir == sessionWorkDir {
		return
	}

	for i := range p.System {
		p.System[i].Text = strings.ReplaceAll(p.System[i].Text, p.CaptureWorkDir, sessionWorkDir)
	}
}

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
//
//nolint:funlen // one per-protocol rendering ladder; flagged pre-21-05 too
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

		blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(m.ToolCalls)+len(m.ThinkingBlocks))

		// Thinking blocks lead the assistant message (PAR-05, 21-03): BOTH SDK
		// param types, original order — filtering by type=="thinking" only
		// would drop redacted blocks and the provider would 400. The guard
		// keeps zero-thinking rendering byte-identical (additive-only).
		for _, tb := range m.ThinkingBlocks {
			switch tb.Type {
			case typeThinking:
				blocks = append(blocks, anthropic.NewThinkingBlock(tb.Signature, tb.Text))
			case typeRedactedThinking:
				blocks = append(blocks, anthropic.NewRedactedThinkingBlock(tb.Data))
			}
		}

		switch {
		case len(m.Blocks) > 0:
			// Rich rendering (PAR-06, 21-05): the ordered block list IS the
			// wire content — Content is the text-concatenation view only.
			blocks = append(blocks, richBlocks(m.Blocks)...)
		case len(m.ToolCalls) == 0:
			// Text-only: exactly the pre-08-07 rendering (byte-compat).
			blocks = append(blocks, anthropic.NewTextBlock(m.Content))
		default:
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

// richBlocks renders a Message's ordered rich block list (PAR-06, 21-05):
// text blocks in position; image blocks via the shape-time Ref read (a
// missing Ref drops that block loudly and continues).
func richBlocks(in []Block) []anthropic.ContentBlockParamUnion {
	out := make([]anthropic.ContentBlockParamUnion, 0, len(in))

	for i := range in {
		blk := &in[i]

		if blk.Image != nil {
			if imgParam, ok := imageBlockParam(blk.Image); ok {
				out = append(out, imgParam)
			}

			continue
		}

		if blk.Text != "" {
			out = append(out, anthropic.NewTextBlock(blk.Text))
		}
	}

	return out
}

// imgDropNotePrefix is the shaper's loud degrade-note prefix (the D-10/D-11
// note family): a missing Ref at shape time (disk loss between ingress and
// shape) drops the block + ONE note — Shape still succeeds with the remaining
// blocks (degrade-softly, never a 400-spamming empty block, never a dead
// turn). stderr only (transport discipline).
const imgDropNotePrefix = "ass-guard: image content block dropped"

// LoadImageBytes reads a Ref's bytes back for shape-time mapping (21-05,
// PAR-06): the Ref is an ingress-persisted path under the session .ass-guard
// images dir (the runtime's imgscale persists them; the shaper — the layer
// that reads — owns the reader so the dependency edge stays one-directional).
func LoadImageBytes(ref string) ([]byte, error) {
	data, err := os.ReadFile(ref)
	if err != nil {
		return nil, fmt.Errorf("load image ref: %w", err)
	}

	return data, nil
}

// imageBlockParam renders one image Block onto the SDK param (PAR-06, 21-05):
// the Ref'd bytes are read AT SHAPE TIME via LoadImageBytes and base64-mapped
// onto Base64ImageSourceParam — SDK param structs only, never hand-marshaled
// JSON (RESEARCH Don't-Hand-Roll). ok=false is the degrade path: the block is
// DROPPED (contributes no param — never a phantom empty block that would
// 400), one loud note lands on stderr, and shaping continues.
func imageBlockParam(b *ImageBlock) (block anthropic.ContentBlockParamUnion, ok bool) {
	data, err := LoadImageBytes(b.Ref)
	if err != nil {
		log.Printf("%s at shape time: ref unreadable (%s) — remaining blocks continue", imgDropNotePrefix, b.Ref)

		return anthropic.ContentBlockParamUnion{}, false
	}

	return anthropic.NewImageBlockBase64(b.MediaType, base64.StdEncoding.EncodeToString(data)), true
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
