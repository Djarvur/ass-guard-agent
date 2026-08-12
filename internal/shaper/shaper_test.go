package shaper_test

import (
	"path/filepath"
	"strings"
	"testing"

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
	p := loadFixture(t, "minimal")
	s := shaper.New()

	params, opts, err := s.Shape(p, []shaper.Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	if got := string(params.Model); got != "synth-model" {
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
	root, _ := filepath.Abs(filepath.Join("..", "..", "profiles"))
	l := profile.NewLoader(root)

	p, err := l.Load("zcode")
	if err != nil {
		t.Skipf("zcode profile not available: %v", err)
	}

	s := shaper.New()

	params, opts, err := s.Shape(p, []shaper.Message{{Role: "user", Content: "read go.mod"}})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	if len(params.System) != 3 {
		t.Errorf("len(System) = %d, want 3", len(params.System))
	}

	if len(params.Tools) == 0 {
		t.Error("Tools empty; expected the captured catalog")
	}
	// D-16: tool count is source-declared (103 for the 016eee8a main session).
	if len(params.Tools) != len(p.Tools) {
		t.Errorf("len(Tools) = %d, want %d (all profile tools shaped)", len(params.Tools), len(p.Tools))
	}

	if params.Thinking.OfEnabled == nil || params.Thinking.OfEnabled.BudgetTokens != 32000 {
		t.Error("thinking budget not reproduced")
	}

	if len(opts) != 12 {
		t.Errorf("len(opts) = %d, want 12 identity headers", len(opts))
	}

	if !strings.EqualFold(string(params.Model), "GLM-5.2") {
		t.Errorf("Model = %q, want GLM-5.2", params.Model)
	}
}
