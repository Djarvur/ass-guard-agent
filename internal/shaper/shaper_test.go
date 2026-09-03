package shaper_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
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

// --- PAR-06 image-block mapping battery (21-05, Task 2 RED) ---

// Image-mapping fixture markers.
const (
	imgMediaPNG   = "image/png"
	imgRefName    = "ingress.png"
	imgNoteMarker = "ass-guard: image content block dropped"
)

// plantImageFile writes a tiny PNG into a temp dir and returns its path +
// its base64 (the Ref'd-bytes shape ingress persists).
func plantImageFile(t *testing.T) (ref string, b64 string) {
	t.Helper()

	var buf bytes.Buffer

	img := image.NewRGBA(image.Rect(0, 0, 4, 4)) //nolint:mnd // fixture dims
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	ref = filepath.Join(t.TempDir(), imgRefName)

	if err := os.WriteFile(ref, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("plant image: %v", err)
	}

	return ref, base64.StdEncoding.EncodeToString(buf.Bytes())
}

// TestImageCapability_MapsRefToBase64Param pins the shaper mapping: a Message
// carrying an image block's Ref renders an anthropic.ImageBlockParam whose
// Source is Base64ImageSourceParam{Data: base64 of the Ref'd file, MediaType:
// the block's media type} — the file is read at SHAPE TIME; dims/provenance
// fields never reach the wire (the SDK param carries only the source).
func TestImageCapability_MapsRefToBase64Param(t *testing.T) {
	t.Parallel()

	ref, b64 := plantImageFile(t)

	p := loadFixture(t, "minimal")

	messages := []shaper.Message{{
		Role: roleUser,
		Blocks: []shaper.Block{
			{Text: "describe this"},
			{Image: &shaper.ImageBlock{Ref: ref, MediaType: imgMediaPNG}},
		},
	}}

	params, _, err := shaper.New().Shape(&p, messages)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	if len(params.Messages) != 1 {
		t.Fatalf("len(Messages) = %d; want 1", len(params.Messages))
	}

	blocks := params.Messages[0].Content
	if len(blocks) != 2 {
		t.Fatalf("len(content blocks) = %d; want 2 (text + image)", len(blocks))
	}

	if b := blocks[0]; b.OfImage != nil || b.OfText == nil || b.OfText.Text != "describe this" {
		t.Errorf("block 0 = %+v; want the leading text block", b)
	}

	img := blocks[1].OfImage
	if img == nil {
		t.Fatalf("block 1 carries no ImageBlockParam: %+v", blocks[1])
	}

	src := img.Source.OfBase64
	if src == nil {
		t.Fatalf("image block source is not base64: %+v", img.Source)
	}

	if src.Data != b64 {
		t.Errorf("source data = %.40s…; want the base64 of the Ref'd file (%.40s…)", src.Data, b64)
	}

	if src.MediaType != imgMediaPNG {
		t.Errorf("source media type = %q; want %q", src.MediaType, imgMediaPNG)
	}
}

// TestImageCapability_OrderPreserved pins content-block order parity: an
// image block BETWEEN two text blocks renders in position.
func TestImageCapability_OrderPreserved(t *testing.T) {
	t.Parallel()

	ref, _ := plantImageFile(t)

	p := loadFixture(t, "minimal")

	messages := []shaper.Message{{
		Role: roleUser,
		Blocks: []shaper.Block{
			{Text: "before"},
			{Image: &shaper.ImageBlock{Ref: ref, MediaType: imgMediaPNG}},
			{Text: "after"},
		},
	}}

	params, _, err := shaper.New().Shape(&p, messages)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	blocks := params.Messages[0].Content
	if len(blocks) != 3 {
		t.Fatalf("len(content blocks) = %d; want 3", len(blocks))
	}

	if blocks[0].OfImage != nil || blocks[0].OfText.Text != "before" {
		t.Errorf("block 0 = %+v; want text 'before'", blocks[0])
	}

	if blocks[1].OfImage == nil {
		t.Errorf("block 1 = %+v; want the image IN POSITION", blocks[1])
	}

	if blocks[2].OfImage != nil || blocks[2].OfText.Text != "after" {
		t.Errorf("block 2 = %+v; want text 'after'", blocks[2])
	}
}

// TestImageCapability_MissingRefDegrades pins the degrade-softly rule: a
// missing Ref file at shape time (disk loss between ingress and shape) drops
// the block with ONE loud error-class note — Shape still succeeds with the
// remaining blocks (never a 400-spamming empty block, never a dead turn).
func TestImageCapability_MissingRefDegrades(t *testing.T) { //nolint:paralleltest // swaps the global log writer
	var notes bytes.Buffer

	log.SetOutput(&notes)

	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	ref := filepath.Join(t.TempDir(), "missing.png")

	p := loadFixture(t, "minimal")

	messages := []shaper.Message{{
		Role: roleUser,
		Blocks: []shaper.Block{
			{Text: "the text survives"},
			{Image: &shaper.ImageBlock{Ref: ref, MediaType: imgMediaPNG}},
		},
	}}

	params, _, err := shaper.New().Shape(&p, messages)
	if err != nil {
		t.Fatalf("Shape failed on a missing Ref: %v (want degrade-softly)", err)
	}

	blocks := params.Messages[0].Content
	if len(blocks) != 1 || blocks[0].OfText == nil || blocks[0].OfText.Text != "the text survives" {
		t.Errorf("blocks = %+v; want the surviving text block only", blocks)
	}

	if got := strings.Count(notes.String(), imgNoteMarker); got != 1 {
		t.Errorf("loud notes = %d; want exactly 1 (%q)", got, notes.String())
	}
}

// TestImageCapability_ZeroBlocksByteIdentical pins the additive-only
// guarantee: a Message with NO Blocks renders byte-identically to the
// pre-change form (one text block from Content).
func TestImageCapability_ZeroBlocksByteIdentical(t *testing.T) {
	t.Parallel()

	p := loadFixture(t, "minimal")

	messages := []shaper.Message{
		{Role: roleUser, Content: "go"},
		{Role: roleAssistant, Content: "plain answer"},
	}

	params, _, err := shaper.New().Shape(&p, messages)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	want0 := []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("go")}
	if !reflect.DeepEqual(params.Messages[0].Content, want0) {
		t.Errorf("user message drifted: %+v; want %+v", params.Messages[0].Content, want0)
	}

	want1 := []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("plain answer")}
	if !reflect.DeepEqual(params.Messages[1].Content, want1) {
		t.Errorf("assistant message drifted: %+v; want %+v", params.Messages[1].Content, want1)
	}
}
