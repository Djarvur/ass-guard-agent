package openspec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
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

// TestSeeded_ChainingRowsFromRealCapture (08-06 T3, D-12): the EMBEDDED seeded
// table carries the four-stage chaining rows re-seeded from the REAL capture —
// cmd/ass-guard/testdata/opsx-e2e/stage-{1,2,3}-output-capture.txt (the
// 2026-08-15T13:29Z gated run: real openspec 1.5.0 binary + real GLM-5.2 model
// through the full explore→propose→apply→archive scenario, 505s, all four
// stages' closing outputs captured). Each stage's REAL closing text selects its
// own handoff row + next command; the archive closing selects NOTHING (the
// chain ends naturally). Excerpts are verbatim from the capture files.
func TestSeeded_ChainingRowsFromRealCapture(t *testing.T) {
	t.Parallel()

	cfg, err := openspec.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	cases := []struct {
		stage   string
		excerpt string
		wantID  string
		wantNxt string
	}{
		{
			stage:   "explore",
			excerpt: "When something crystallizes, I can spin up an OpenSpec change proposal for it. But no rush",
			wantID:  "post-explore-handoff",
			wantNxt: "/opsx:propose",
		},
		{
			stage:   "propose",
			excerpt: "Run `/opsx:apply` (or just ask me to implement) to start working on the tasks.",
			wantID:  "post-propose-handoff",
			wantNxt: "/opsx:apply",
		},
		{
			stage:   "apply",
			excerpt: "All tasks complete! You can archive this change with `/opsx:archive`.",
			wantID:  "post-apply-handoff",
			wantNxt: "/opsx:archive",
		},
	}

	for _, tc := range cases {
		m := pt.MatchText(tc.excerpt)
		if m.ID != tc.wantID {
			t.Errorf("%s closing: matched %q (span %q); want %q", tc.stage, m.ID, m.Span, tc.wantID)

			continue
		}

		if m.Action != engine.ActionContinue {
			t.Errorf("%s: action = %q; want continue", tc.stage, m.Action)
		}

		if got := pt.NextPromptFor(m.ID); got != tc.wantNxt {
			t.Errorf("%s: NextPromptFor = %q; want %q", tc.stage, got, tc.wantNxt)
		}
	}

	// The archive closing ends the chain: no row matches (verbatim excerpt).
	archiveClosing := "## Archive Complete\n\n**Change:** `add-a-tiny-feature`\n" +
		"**Archived to:** `openspec/changes/archive/2026-08-15-add-a-tiny-feature/`\n" +
		"no active changes remain under `openspec/changes/`."

	if m := pt.MatchText(archiveClosing); m.ID != "" {
		t.Errorf("archive closing matched %q; want NO match (the chain ends naturally)", m.ID)
	}
}
