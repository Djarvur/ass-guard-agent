package runtime //nolint:testpackage // internal package test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// PAR-04 wiring tests (21-02 Task 2): the FOURTH trailing-TextBlock merge in
// sessionFor — a fresh session in a memory-bearing repo carries the injected
// block automatically (criterion 3), the empty tree adds nothing, the shared
// r.profile is never mutated, and truncation notes flow through end-to-end.

// memInjHeader is the injection body's framing line (pinned in
// internal/ecosys/memory_test.go; duplicated here so a header drift fails at
// the wiring level too).
const memInjHeader = "The following project memory files apply to this session:"

// memRunner builds a Runner over a temp project carrying a .git boundary,
// HOME pinned to an empty temp dir (hermetic user global), with plant
// decorating the tree before the real Loader runs. Mirrors newSkillRunner.
func memRunner(t *testing.T, plant func(dir string)) (*Runner, string) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("mkdir .git boundary: %v", err)
	}

	if plant != nil {
		plant(dir)
	}

	prov := &scriptedACPProvider{}

	r := &Runner{
		bus:          event.NewBus(),
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      4,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}

	if err := r.SetupEngine(); err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	r.LoadCommandRegistry() // the real Loader over the temp tree

	return r, dir
}

// memPlantListingsAndMemory plants a skill, an agent definition, and the
// repo-root AGENTS.md — every dynamic merge fires, memory last.
func memPlantListingsAndMemory(t *testing.T, dir string) {
	t.Helper()

	writeSkillFixtures(t, dir)

	agentPath := filepath.Join(dir, ".claude", "agents", "reader.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o750); err != nil {
		t.Fatalf("mkdir agents fixture dir: %v", err)
	}

	if err := os.WriteFile(agentPath,
		[]byte("---\ndescription: Reads things carefully\n---\nYou read things.\n"), 0o600); err != nil {
		t.Fatalf("write agent fixture: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"),
		[]byte("repo-memory-body-marker\n"), 0o600); err != nil {
		t.Fatalf("write AGENTS.md fixture: %v", err)
	}
}

// memBlockIndex returns the index of the block starting with prefix, or -1.
func memBlockIndex(system []profile.TextBlock, prefix string) int {
	for i, b := range system {
		if strings.HasPrefix(b.Text, prefix) {
			return i
		}
	}

	return -1
}

// TestMemoryInjection_MemoryBlockIsLast (criterion 3): a session constructed
// in a repo with an AGENTS.md shows the memory body as the LAST system
// TextBlock of the per-session profile copy — after cwd composition, the
// skills listing, and the agent listing.
func TestMemoryInjection_MemoryBlockIsLast(t *testing.T) { //nolint:paralleltest // HOME pin
	r, _ := memRunner(t, func(dir string) { memPlantListingsAndMemory(t, dir) })

	sess := r.sessionFor(context.Background(), "sess-mem-1")
	if sess == nil {
		t.Fatal("sessionFor returned nil")
	}

	system := sess.Profile.System
	if len(system) == 0 {
		t.Fatal("session profile carries no system blocks")
	}

	last := system[len(system)-1]
	if !strings.HasPrefix(last.Text, memInjHeader) {
		t.Fatalf("last system block is not the memory injection:\n%q", last.Text)
	}

	if !strings.Contains(last.Text, "repo-memory-body-marker") {
		t.Errorf("memory block missing the AGENTS.md body:\n%s", last.Text)
	}

	if !strings.Contains(last.Text, filepath.Join(r.workDir, "AGENTS.md")) {
		t.Errorf("memory block missing the source path:\n%s", last.Text)
	}

	skillsIdx := memBlockIndex(system, skillListingHeaderCaptured)
	agentsIdx := memBlockIndex(system, "The following specialized agent types are available")
	if skillsIdx < 0 || agentsIdx < 0 || agentsIdx < skillsIdx {
		t.Errorf("listings absent or misordered (skills=%d agents=%d)", skillsIdx, agentsIdx)
	}

	if agentsIdx >= len(system)-1 {
		t.Errorf("agent listing at %d is not before the trailing memory block (%d)", agentsIdx, len(system)-1)
	}
}

// TestMemoryInjection_EmptyTreeAddsNothing (the skip contract end-to-end): a
// tree with listings but ZERO memory files anywhere (incl. HOME) leaves the
// per-session profile length at the no-memory baseline.
func TestMemoryInjection_EmptyTreeAddsNothing(t *testing.T) { //nolint:paralleltest // HOME pin
	baselineRunner, _ := memRunner(t, func(dir string) { writeSkillFixtures(t, dir) })

	baselineSess := baselineRunner.sessionFor(context.Background(), "sess-mem-empty-base")
	if baselineSess == nil {
		t.Fatal("baseline sessionFor returned nil")
	}

	baselineLen := len(baselineSess.Profile.System)

	emptyRunner, _ := memRunner(t, func(dir string) { writeSkillFixtures(t, dir) }) // listings fire; no memory

	emptySess := emptyRunner.sessionFor(context.Background(), "sess-mem-empty")
	if emptySess == nil {
		t.Fatal("empty sessionFor returned nil")
	}

	if got := len(emptySess.Profile.System); got != baselineLen {
		t.Errorf("empty-tree system len = %d; want the no-memory baseline %d", got, baselineLen)
	}

	if memBlockIndex(emptySess.Profile.System, memInjHeader) >= 0 {
		t.Error("zero memory files still produced an injection block")
	}
}

// TestMemoryInjection_SharedProfileUntouched (the double-append copy
// discipline): with a memory body present, the shared r.profile is
// byte-identical after session construction.
func TestMemoryInjection_SharedProfileUntouched(t *testing.T) { //nolint:paralleltest // HOME pin
	r, _ := memRunner(t, func(dir string) { memPlantListingsAndMemory(t, dir) })

	sharedBefore := append([]profile.TextBlock(nil), r.profile.System...)

	sess := r.sessionFor(context.Background(), "sess-mem-copy")
	if sess == nil {
		t.Fatal("sessionFor returned nil")
	}

	if len(r.profile.System) != len(sharedBefore) {
		t.Errorf("shared r.profile.System grew %d → %d (per-session copy discipline violated)",
			len(sharedBefore), len(r.profile.System))
	}

	for i := range sharedBefore {
		if r.profile.System[i] != sharedBefore[i] {
			t.Errorf("shared r.profile.System[%d] mutated", i)
		}
	}
}

// TestMemoryInjection_TruncationFlowsThrough (D-08 end-to-end): a >24 KB
// memory file in the temp project produces the truncated body + the loud
// note inside the session's trailing block.
func TestMemoryInjection_TruncationFlowsThrough(t *testing.T) { //nolint:paralleltest // HOME pin
	big := strings.Repeat("t", 30000)

	r, _ := memRunner(t, func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(big), 0o600); err != nil {
			t.Fatalf("write big AGENTS.md: %v", err)
		}
	})

	sess := r.sessionFor(context.Background(), "sess-mem-cap")
	if sess == nil {
		t.Fatal("sessionFor returned nil")
	}

	system := sess.Profile.System
	if len(system) == 0 || !strings.HasPrefix(system[len(system)-1].Text, memInjHeader) {
		t.Fatal("last system block is not the memory injection")
	}

	block := system[len(system)-1].Text
	if !strings.Contains(block, "truncated from 30000 bytes to the 24576-byte per-file cap") {
		t.Errorf("memory block missing the truncation note:\n%s", block[:min(400, len(block))])
	}

	if strings.Contains(block, strings.Repeat("t", 24577)) {
		t.Error("memory block carries untruncated content past the cap")
	}

	if !strings.Contains(block, strings.Repeat("t", 24576)) {
		t.Error("memory block missing the capped 24576-byte body")
	}
}
