package profile_test

import (
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// TestLoader_MinimalFixture verifies the loader reads a synthetic fixture
// profile faithfully. This also seeds PROF-02 enforcement: the loader must be
// profile-agnostic (the same code loads the zcode and the synthetic profile).
func TestLoader_MinimalFixture(t *testing.T) { //nolint:cyclop,funlen // comprehensive test scenario
	t.Parallel()

	l := profile.NewLoader(filepath.Join(".", "testdata"))

	p, err := l.Load("minimal")
	if err != nil {
		t.Fatalf("Load(minimal): %v", err)
	}

	if p.Name != "minimal" {
		t.Errorf("Name = %q, want %q", p.Name, "minimal")
	}

	if p.Model != "synth-model" {
		t.Errorf("Model = %q, want %q", p.Model, "synth-model")
	}

	if p.MaxTokens != 1024 {
		t.Errorf("MaxTokens = %d, want %d", p.MaxTokens, 1024)
	}

	if len(p.System) != 2 {
		t.Fatalf("len(System) = %d, want 2", len(p.System))
	}

	if p.System[0].Text != "You are a synthetic test agent." {
		t.Errorf("System[0].Text = %q", p.System[0].Text)
	}

	if p.System[0].Type != "text" {
		t.Errorf("System[0].Type = %q, want %q", p.System[0].Type, "text")
	}

	if len(p.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2", len(p.Tools))
	}

	if p.Tools[0].Name != "synth_tool_a" {
		t.Errorf("Tools[0].Name = %q, want synth_tool_a", p.Tools[0].Name)
	}

	if len(p.Tools[0].InputSchema) == 0 {
		t.Error("Tools[0].InputSchema is empty")
	}

	if len(p.Headers) != 2 {
		t.Fatalf("len(Headers) = %d, want 2", len(p.Headers))
	}

	if p.Headers[0].Name != "X-Synth-Test" {
		t.Errorf("Headers[0].Name = %q, want X-Synth-Test", p.Headers[0].Name)
	}

	if p.Headers[0].ValueTemplate != "<synth-test>" {
		t.Errorf("Headers[0].ValueTemplate = %q", p.Headers[0].ValueTemplate)
	}

	if string(p.Thinking) == "" {
		t.Error("Thinking is empty")
	}

	if string(p.ToolChoice) == "" {
		t.Error("ToolChoice is empty")
	}
}

// TestLoader_MissingProfile returns a non-nil error for an unknown profile.
func TestLoader_MissingProfile(t *testing.T) {
	t.Parallel()

	l := profile.NewLoader(filepath.Join(".", "testdata"))

	_, err := l.Load("does-not-exist")
	if err == nil {
		t.Fatal("Load(nonexistent) returned nil error, want non-nil")
	}
}

// TestLoader_ZcodeProfile is the integration check that the real, extracted
// zcode profile loads and carries the captured shape. Skipped if the artifact
// is not present (e.g. before extraction runs).
func TestLoader_ZcodeProfile(t *testing.T) {
	t.Parallel()

	root, _ := filepath.Abs(filepath.Join("..", "..", "profiles"))
	l := profile.NewLoader(root)

	p, err := l.Load("zcode")
	if err != nil {
		t.Skipf("zcode profile not available yet: %v", err)
	}

	if p.Name != "zcode" {
		t.Errorf("Name = %q, want zcode", p.Name)
	}

	if p.Model != "GLM-5.3" {
		t.Errorf("Model = %q, want GLM-5.3", p.Model)
	}

	if len(p.System) != 3 {
		t.Errorf("len(System) = %d, want 3 (main session)", len(p.System))
	}

	if len(p.Tools) == 0 {
		t.Error("Tools is empty; expected the captured catalog")
	}
	// D-16: tool count is whatever the source session declares (observed 103
	// for the 016eee8a main session). Assert non-empty + built-in core present.
	want := map[string]bool{"Read": false, "Bash": false, "Edit": false, "Write": false}
	for _, d := range p.Tools {
		if _, ok := want[d.Name]; ok {
			want[d.Name] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("core tool %q missing from profile", name)
		}
	}

	if len(p.Headers) != 12 {
		t.Errorf("len(Headers) = %d, want 12 identity headers", len(p.Headers))
	}
}
