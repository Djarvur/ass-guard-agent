package openspec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/openspec"
)

// TestNextPrompt_ConfigParsesNextField (08-06 Test 1): a [[patterns]] entry
// with a stage-bearing id and a next command parses with both preserved.
func TestNextPrompt_ConfigParsesNextField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "openspec.toml")

	err := os.WriteFile(path, []byte(
		"[[patterns]]\nid = \"post-propose-handoff\"\nregex = \"ready for .opsx:apply\"\naction = \"continue\"\nnext = \"/opsx:apply\"\n"), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := openspec.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.Patterns) != 1 {
		t.Fatalf("patterns = %d; want 1", len(cfg.Patterns))
	}

	p := cfg.Patterns[0]
	if p.ID != "post-propose-handoff" {
		t.Errorf("id = %q; want post-propose-handoff", p.ID)
	}

	if p.Next != "/opsx:apply" {
		t.Errorf("next = %q; want /opsx:apply", p.Next)
	}
}

// TestNextPrompt_TableAnswersNextPromptFor (08-06 Test 2): the table returns
// the entry's next text for its id; unknown ids (and entries without next)
// yield "" (the engine's generic fallback still applies).
func TestNextPrompt_TableAnswersNextPromptFor(t *testing.T) {
	t.Parallel()

	cfg := &openspec.OpenSpecConfig{Patterns: []openspec.PatternEntry{
		{ID: "post-propose-handoff", Regex: "ready", Action: "continue", Next: "/opsx:apply"},
		{ID: "no-next-handoff", Regex: "plain", Action: "continue"},
	}}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	if got := pt.NextPromptFor("post-propose-handoff"); got != "/opsx:apply" {
		t.Errorf("NextPromptFor(post-propose-handoff) = %q; want /opsx:apply", got)
	}

	if got := pt.NextPromptFor("no-next-handoff"); got != "" {
		t.Errorf("NextPromptFor(no-next) = %q; want empty (fallback applies)", got)
	}

	if got := pt.NextPromptFor("unknown-id"); got != "" {
		t.Errorf("NextPromptFor(unknown) = %q; want empty", got)
	}
}
