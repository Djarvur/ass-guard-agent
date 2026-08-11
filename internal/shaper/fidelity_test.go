package shaper_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/shaper"
)

// loadProfileFromRoot loads a profile from a profiles root directory.
func loadProfileFromRoot(t *testing.T, root, name string) profile.Profile {
	t.Helper()
	p, err := profile.NewLoader(root).Load(name)
	if err != nil {
		t.Fatalf("load %s from %s: %v", name, root, err)
	}
	return p
}

// profilesRoot resolves the repo-root profiles directory.
func profilesRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestFidelity_SystemBlocksByteEqual asserts the shaped request's serialized
// system blocks are byte-equal to the profile's block-N.txt files (TIER-1).
func TestFidelity_SystemBlocksByteEqual(t *testing.T) {
	prof := loadProfileFromRoot(t, profilesRoot(t), "zcode")
	s := shaper.New()
	params, _, err := s.Shape(prof, []shaper.Message{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	// Marshal the shaped request and decode just the system text to compare
	// byte-for-byte against the profile's stored blocks.
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		System []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"system"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.System) != len(prof.System) {
		t.Fatalf("shaped system has %d blocks, profile has %d", len(got.System), len(prof.System))
	}
	for i, b := range got.System {
		if b.Text != prof.System[i].Text {
			t.Errorf("system block %d text differs from profile (byte-fidelity failure)", i)
		}
		if b.Type != "text" {
			t.Errorf("system block %d type = %q, want text", i, b.Type)
		}
	}
}

// TestFidelity_ToolCountAndThinking asserts the shaped request carries the full
// captured catalog + the thinking config.
func TestFidelity_ToolCountAndThinking(t *testing.T) {
	prof := loadProfileFromRoot(t, profilesRoot(t), "zcode")
	s := shaper.New()
	params, opts, err := s.Shape(prof, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(params.Tools) != len(prof.Tools) {
		t.Errorf("shaped tools = %d, profile = %d", len(params.Tools), len(prof.Tools))
	}
	if params.Thinking.OfEnabled == nil || params.Thinking.OfEnabled.BudgetTokens != 32000 {
		t.Error("thinking budget not reproduced byte-faithfully")
	}
	if len(opts) != 12 {
		t.Errorf("opts = %d header options, want 12", len(opts))
	}
	if !strings.EqualFold(string(params.Model), "GLM-5.2") {
		t.Errorf("Model = %q", params.Model)
	}
}

// TestPROF02_SyntheticProfileShapes is the structural PROF-02 enforcement: a
// non-zcode profile loads through the SAME Shaper code path and shapes to its
// own fields (not any zcode defaults). D-11.
func TestPROF02_SyntheticProfileShapes(t *testing.T) {
	prof := loadProfileFromRoot(t, profilesRoot(t), "synthetic")
	s := shaper.New()
	params, opts, err := s.Shape(prof, []shaper.Message{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(params.Model) != "synth-model" {
		t.Errorf("Model = %q, want synth-model (PROF-02: synthetic fields, not zcode)", params.Model)
	}
	if len(params.System) != 3 || !strings.Contains(params.System[0].Text, "synthetic test agent") {
		t.Error("shaped request did not carry the synthetic system blocks")
	}
	if len(params.Tools) != 2 || params.Tools[0].OfTool.Name != "synth_tool_a" {
		t.Error("shaped tools are not the synthetic pair")
	}
	if len(opts) != 2 {
		t.Errorf("opts = %d, want 2 synthetic headers", len(opts))
	}
}

// TestPROF02_NoProfileNameLiteralsInShaper is the D-11 grep lint: the shaper
// package contains no profile-name literals (e.g. "zcode") outside test files.
// PROF-02 is a structural guarantee, not a code-review hope.
func TestPROF02_NoProfileNameLiteralsInShaper(t *testing.T) {
	root, err := filepath.Abs(filepath.Join(".."))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "zcode") {
			t.Errorf("shaper source file %s contains the literal \"zcode\" (D-11 / PROF-02 violation)", name)
		}
	}
}
