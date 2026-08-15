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

// Seeded-row literals shared by the capture-pinning test (goconst).
const (
	idPostProposeHandoff = "post-propose-handoff"
	idPostExploreHandoff = "post-explore-handoff"
	nextProposeCmd       = "/opsx:propose"
	stageApply           = "apply"
	keyOpsxExplore       = "opsx:explore"
	keyOpsxPropose       = "opsx:propose"
)

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
// stages' closing outputs captured). The propose/apply stages' REAL closing
// text selects its own handoff row + next command; the archive closing selects
// NOTHING (the chain ends naturally); the EXPLORE closings match NO text row —
// that boundary chains on command provenance (the hybrid mechanism, findings-6
// disposition — see TestSeeded_ExploreChainsOnProvenance). Excerpts are
// verbatim from the capture files.
func TestSeeded_ChainingRowsFromRealCapture(t *testing.T) { //nolint:funlen // four-stage excerpt battery
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
			stage:   "propose",
			excerpt: "Run `/opsx:apply` (or just ask me to implement) to start working on the tasks.",
			wantID:  idPostProposeHandoff,
			wantNxt: "/opsx:apply",
		},
		{
			stage:   stageApply,
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

	// The archive closing ends the chain via the TERMINAL SHIELD.
	archiveClosing := "## Archive Complete\n\n**Change:** `add-a-tiny-feature`\n" +
		"**Archived to:** `openspec/changes/archive/2026-08-15-add-a-tiny-feature/`\n" +
		"All artifacts complete (proposal, design, specs, tasks). " +
		"no active changes remain under `openspec/changes/`."

	m := pt.MatchText(archiveClosing)
	if m.ID != "post-archive-terminal" {
		t.Fatalf("archive closing matched %q; want post-archive-terminal (the shield)", m.ID)
	}

	if m.Action != engine.ActionWait {
		t.Errorf("archive closing action = %q; want wait (no injection — the chain ends)", m.Action)
	}
}

// The FOUR live explore closings (2026-08-15 gated runs — the 6th finding's
// evidence): structurally different free-form texts, two with NO propos* stem.
// They are the reason the explore boundary chains on COMMAND PROVENANCE, never
// on a text regex.
var exploreClosingExcerpts = []string{
	// Run 1 (13:29Z capture).
	"When something crystallizes, I can spin up an OpenSpec change proposal for it. But no rush",
	// Run 2 (13:40Z product run).
	"what does the smallest meaningful spec-driven change look like here? " +
		"A calibration run — proposal → spec → tasks — implement → archive.",
	// Run 3 (16:49Z product run) — open question, only the "Proposed" stem mid-text.
	"since `openspec/specs` is empty, whatever we pick becomes the first " +
		"capability spec in the project. So — what did you have in mind by " +
		"\"tiny feature\"? One option Proposed earlier was the CLI-args thread.",
	// Run 4 (17:0x product run) — pure Socratic close, zero propos* occurrences.
	"Which thread pulls at you?",
}

// TestSeeded_ExploreChainsOnProvenance (hybrid chaining, findings-6
// disposition): the embedded table's explore row is a COMMAND-PROVENANCE row —
// MatchCommand("opsx:explore") yields post-explore-handoff with next
// /opsx:propose — while EVERY observed explore closing matches NO text row
// (the free-form closing cannot be regex-chained honestly; four live forms
// prove it). The other stage keys carry NO command rows: propose/apply/archive
// stay chained by the capture-seeded TEXT rows (regex authoritative there).
func TestSeeded_ExploreChainsOnProvenance(t *testing.T) {
	t.Parallel()

	cfg, err := openspec.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	// The provenance row: explore → propose.
	cm := pt.MatchCommand(keyOpsxExplore)
	if cm.ID != idPostExploreHandoff {
		t.Fatalf("MatchCommand(%s) = %q; want %q (the seeded provenance row)", keyOpsxExplore, cm.ID, idPostExploreHandoff)
	}

	if cm.Action != engine.ActionContinue {
		t.Errorf("explore provenance action = %q; want continue", cm.Action)
	}

	if cm.Span != keyOpsxExplore {
		t.Errorf("explore provenance span = %q; want the command key %q", cm.Span, keyOpsxExplore)
	}

	if got := pt.NextPromptFor(cm.ID); got != nextProposeCmd {
		t.Errorf("NextPromptFor(%s) = %q; want %q", cm.ID, got, nextProposeCmd)
	}

	if cm.ConfigSource != "openspec.toml command_patterns/"+idPostExploreHandoff {
		t.Errorf("ConfigSource = %q; want the command_patterns entry source", cm.ConfigSource)
	}

	// The regex boundaries stay authoritative: no command rows for the other
	// stages — their closings chain on the capture-seeded TEXT rows.
	for _, key := range []string{keyOpsxPropose, "opsx:apply", "opsx:archive"} {
		if d := pt.MatchCommand(key); d.Action != engine.ActionNothing {
			t.Errorf("MatchCommand(%s) = %+v; want the zero value (text rows own that boundary)", key, d)
		}
	}

	// Every observed explore closing matches NO text row.
	for i, excerpt := range exploreClosingExcerpts {
		if d := pt.MatchText(excerpt); d.Action != engine.ActionNothing {
			t.Errorf("explore closing %d matched text row %q (%q); want NO text match — "+
				"the boundary chains on provenance, not regex", i+1, d.ID, d.Span)
		}
	}
}
