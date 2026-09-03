package shaper_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// loadFixture loads a profile from the internal/profile testdata (the synthetic
// fixture), used to prove the Shaper is profile-agnostic (PROF-02 seed).
func loadFixture(t *testing.T, name string) profile.Profile {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "profile", "testdata"))
	if err != nil {
		t.Fatal(err)
	}

	l := profile.NewLoader(root)

	p, err := l.Load(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}

	return p
}

// TestShape_SyntheticFixture proves the Shaper shapes a non-zcode profile
// through the same code path (PROF-02 structural enforcement seed). The shaped
// request must carry the SYNTHETIC fields, not any profile-specific defaults.
func TestShape_SyntheticFixture(t *testing.T) {
	t.Parallel()
	p := loadFixture(t, "minimal")
	s := shaper.New()

	params, opts, err := s.Shape(&p, []shaper.Message{{Role: roleUser, Content: "hello"}})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	if got := params.Model; got != synthModel {
		t.Errorf("Model = %q, want synth-model", got)
	}

	if params.MaxTokens != 1024 {
		t.Errorf("MaxTokens = %d, want 1024", params.MaxTokens)
	}

	if len(params.System) != 2 {
		t.Fatalf("len(System) = %d, want 2", len(params.System))
	}

	if params.System[0].Text != "You are a synthetic test agent." {
		t.Errorf("System[0].Text = %q", params.System[0].Text)
	}

	if len(params.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2", len(params.Tools))
	}

	if params.Tools[0].OfTool == nil || params.Tools[0].OfTool.Name != "synth_tool_a" {
		got := "(nil)"
		if params.Tools[0].OfTool != nil {
			got = params.Tools[0].OfTool.Name
		}

		t.Errorf("Tools[0].OfTool.Name = %q, want synth_tool_a", got)
	}

	if params.Thinking.OfEnabled == nil || params.Thinking.OfEnabled.BudgetTokens != 1024 {
		t.Error("Thinking.OfEnabled.BudgetTokens missing or != 1024")
	}

	if params.ToolChoice.OfAuto == nil {
		t.Error("ToolChoice.OfAuto is nil, want non-nil for {type:auto}")
	}

	if len(params.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(params.Messages))
	}
	// One option.WithHeader per profile.Headers entry — data-driven (D-11).
	if len(opts) != len(p.Headers) {
		t.Errorf("len(opts) = %d, want %d (one per header)", len(opts), len(p.Headers))
	}
}

// TestShape_RenderedHeadersNonEmpty asserts the data-driven header loop emits a
// non-empty rendered value per header (the value is TIER-3 per-session, but it
// must be present so drift detection sees the field populated — TIER-2 presence).
func TestShape_RenderedHeadersNonEmpty(t *testing.T) {
	t.Parallel()
	p := loadFixture(t, "minimal")
	// Render each header template and assert non-empty.
	for _, h := range p.Headers {
		v := shaper.RenderHeaderValue(h.ValueTemplate)
		if v == "" {
			t.Errorf("rendered value for %q is empty", h.Name)
		}
	}
}

// TestShape_ZcodeProfile is the integration check against the real extracted
// artifact. Skipped if the profile is absent.
func TestShape_ZcodeProfile(t *testing.T) {
	t.Parallel()

	root, _ := filepath.Abs(filepath.Join("..", "..", "profiles"))
	l := profile.NewLoader(root)

	p, err := l.Load(profileZcode)
	if err != nil {
		t.Skipf("zcode profile not available: %v", err)
	}

	s := shaper.New()

	params, opts, err := s.Shape(&p, []shaper.Message{{Role: roleUser, Content: "read go.mod"}})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	if len(params.System) != 3 {
		t.Errorf("len(System) = %d, want 3", len(params.System))
	}

	if len(params.Tools) == 0 {
		t.Error("Tools empty; expected the captured catalog")
	}
	// D-16: tool count is source-declared (79 for the 3cee56ae pin, zcode 0.16.3).
	if len(params.Tools) != len(p.Tools) {
		t.Errorf("len(Tools) = %d, want %d (all profile tools shaped)", len(params.Tools), len(p.Tools))
	}

	// The 2026-08-16 re-pin carries NO thinking config — none may be emitted.
	if params.Thinking.OfEnabled != nil {
		t.Error("thinking emitted but the pinned capture carries no thinking config")
	}

	if len(opts) != 12 {
		t.Errorf("len(opts) = %d, want 12 identity headers", len(opts))
	}

	if !strings.EqualFold(params.Model, "GLM-5.3") {
		t.Errorf("Model = %q, want GLM-5.3", params.Model)
	}
}

// --- PAR-05 thinking-block mapping battery (21-03, Task 3 RED) ---

// TestShaperThinking_MapsBothBlockTypesInOrder pins hop 5b: the
// assistant-batch branch maps ThinkingBlocks to BOTH SDK param types —
// thinking → ThinkingBlockParam{Signature, Thinking}, redacted_thinking →
// RedactedThinkingBlockParam{Data} — in ORIGINAL order, before text+tool_use.
// Filtering by type=="thinking" only would drop redacted blocks and 400.
func TestShaperThinking_MapsBothBlockTypesInOrder(t *testing.T) {
	t.Parallel()

	p := loadFixture(t, "minimal")

	messages := []shaper.Message{{
		Role: "assistant",
		ThinkingBlocks: []shaper.ThinkingBlock{
			{Type: "thinking", Text: "first thought", Signature: "sig-1"},
			{Type: "redacted_thinking", Data: "enc-opaque"},
			{Type: "thinking", Text: "second thought", Signature: "sig-2"},
		},
		ToolCalls: []shaper.ToolCall{{ID: "tc-1", Name: "Bash", Input: []byte(`{"command":"ls"}`)}},
	}}

	params, _, err := shaper.New().Shape(&p, messages)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	if len(params.Messages) != 1 {
		t.Fatalf("len(Messages) = %d; want 1", len(params.Messages))
	}

	blocks := params.Messages[0].Content
	if len(blocks) != 4 {
		t.Fatalf("len(content blocks) = %d; want 4 (thinking, redacted, thinking, tool_use)", len(blocks))
	}

	if b := blocks[0]; b.OfThinking == nil || b.OfThinking.Thinking != "first thought" || b.OfThinking.Signature != "sig-1" {
		t.Errorf("block 0 = %+v; want ThinkingBlockParam{first thought, sig-1}", b)
	}

	if b := blocks[1]; b.OfRedactedThinking == nil || b.OfRedactedThinking.Data != "enc-opaque" {
		t.Errorf("block 1 = %+v; want RedactedThinkingBlockParam{enc-opaque}", b)
	}

	if b := blocks[2]; b.OfThinking == nil || b.OfThinking.Thinking != "second thought" || b.OfThinking.Signature != "sig-2" {
		t.Errorf("block 2 = %+v; want ThinkingBlockParam{second thought, sig-2}", b)
	}

	if b := blocks[3]; b.OfToolUse == nil || b.OfToolUse.ID != "tc-1" {
		t.Errorf("block 3 = %+v; want the tool_use block AFTER the thinking blocks", b)
	}
}

// TestShaperThinking_ZeroThinkingByteIdentical pins the additive-only
// guarantee: a message with ZERO thinking blocks renders byte-identically to
// the pre-change form (text block first, then tool_use blocks — the exact
// append sequence the 08-07 shaping established).
func TestShaperThinking_ZeroThinkingByteIdentical(t *testing.T) {
	t.Parallel()

	p := loadFixture(t, "minimal")

	messages := []shaper.Message{
		{Role: "user", Content: "go"},
		{
			Role:      "assistant",
			Content:   "running it",
			ToolCalls: []shaper.ToolCall{{ID: "tc-z", Name: "Read", Input: []byte(`{"file_path":"a"}`)}},
		},
		{Role: "assistant", Content: "plain answer"},
	}

	params, _, err := shaper.New().Shape(&p, messages)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	if len(params.Messages) != 3 {
		t.Fatalf("len(Messages) = %d; want 3", len(params.Messages))
	}

	// The expected constructions are EXACTLY the pre-change forms.
	want0 := []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("go")}
	if !reflect.DeepEqual(params.Messages[0].Content, want0) {
		t.Errorf("text-only message drifted: %+v; want %+v", params.Messages[0].Content, want0)
	}

	want1 := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock("running it"),
		anthropic.NewToolUseBlock("tc-z", any(map[string]any{"file_path": "a"}), "Read"),
	}
	if !reflect.DeepEqual(params.Messages[1].Content, want1) {
		t.Errorf("text+tool message drifted: %+v; want %+v", params.Messages[1].Content, want1)
	}

	want2 := []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("plain answer")}
	if !reflect.DeepEqual(params.Messages[2].Content, want2) {
		t.Errorf("plain assistant message drifted: %+v; want %+v", params.Messages[2].Content, want2)
	}
}
