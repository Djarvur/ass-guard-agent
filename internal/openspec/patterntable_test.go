package openspec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/openspec"
)

// nextApplyCmd is the chaining fixture's next command (goconst).
const nextApplyCmd = "/opsx:apply"

// TestNextPrompt_ConfigParsesNextField (08-06 Test 1): a [[patterns]] entry
// with a stage-bearing id and a next command parses with both preserved.
func TestNextPrompt_ConfigParsesNextField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "openspec.toml")

	toml := "[[patterns]]\nid = \"post-propose-handoff\"\n" +
		"regex = \"ready for .opsx:apply\"\naction = \"continue\"\nnext = \"/opsx:apply\"\n"

	err := os.WriteFile(path, []byte(toml), 0o600)
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

	if p.Next != nextApplyCmd {
		t.Errorf("next = %q; want %s", p.Next, nextApplyCmd)
	}
}

// TestNextPrompt_TableAnswersNextPromptFor (08-06 Test 2): the table returns
// the entry's next text for its id; unknown ids (and entries without next)
// yield "" (the engine's generic fallback still applies).
func TestNextPrompt_TableAnswersNextPromptFor(t *testing.T) {
	t.Parallel()

	cfg := &openspec.OpenSpecConfig{Patterns: []openspec.PatternEntry{
		{ID: "post-propose-handoff", Regex: "ready", Action: "continue", Next: nextApplyCmd},
		{ID: "no-next-handoff", Regex: "plain", Action: "continue"},
	}}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	if got := pt.NextPromptFor("post-propose-handoff"); got != nextApplyCmd {
		t.Errorf("NextPromptFor(post-propose-handoff) = %q; want %s", got, nextApplyCmd)
	}

	if got := pt.NextPromptFor("no-next-handoff"); got != "" {
		t.Errorf("NextPromptFor(no-next) = %q; want empty (fallback applies)", got)
	}

	if got := pt.NextPromptFor("unknown-id"); got != "" {
		t.Errorf("NextPromptFor(unknown) = %q; want empty", got)
	}
}
