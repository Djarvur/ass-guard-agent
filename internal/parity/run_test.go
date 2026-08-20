package parity_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
)

// Test constants (goconst) + the workspace-required sentinel (err113).
const (
	stateMissing = "MISSING"
	toolObserved = "Observed"
)

var errWorkspaceRequired = errors.New("arm requires a workspace (implement WorkspaceArm)")

// writeFile is the test helper for seeding fixture trees.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}

	err = os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// readFile is the test helper for asserting scratch content.
func readFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(raw)
}

// touchingArm is a workspace-consuming arm: it appends a marker line to the
// turn's scratch file and reports the file's pre-write content as its observed
// call input (a fake whose output DEPENDS on workspace state — the pollution
// detector).
type touchingArm struct {
	marker string
}

func (a *touchingArm) RunTurn(_ context.Context, _ string) ([]parity.ToolCall, error) {
	return nil, errWorkspaceRequired
}

func (a *touchingArm) RunTurnInWorkspace(_ context.Context, _, dir string) ([]parity.ToolCall, error) {
	path := filepath.Join(dir, "state.txt")
	before := stateMissing

	raw, readErr := os.ReadFile(path)
	if readErr == nil {
		before = string(raw)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	_, err = f.WriteString(a.marker + "\n")
	if err != nil {
		_ = f.Close()

		return nil, fmt.Errorf("append %s: %w", path, err)
	}

	err = f.Close()
	if err != nil {
		return nil, fmt.Errorf("close %s: %w", path, err)
	}

	return []parity.ToolCall{{Name: toolObserved, Input: []byte("state=" + before)}}, nil
}

// TestRunSuite_PerTurnIsolation proves the 12-03 state-pollution fix: turn 1's
// tool calls mutate the scratch file, and turn 2 replays against the ORIGINAL
// fixture content — per-turn seeding, never a sibling's residue (the
// 2026-08-16 drift report's 4/13 artifact class).
func TestRunSuite_PerTurnIsolation(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	writeFile(t, filepath.Join(base, "state.txt"), "pristine")

	expectPristine := []parity.ToolCall{{Name: toolObserved, Input: []byte("state=pristine")}}
	suite := []parity.CapturedTurn{
		{TurnID: "t1", Prompt: "mutate", ExpectedToolCalls: expectPristine},
		{TurnID: "t2", Prompt: "depends on pristine state", ExpectedToolCalls: expectPristine},
	}

	h := parity.NewHarness()
	h.BaseWorkspace = base

	results, err := parity.RunSuite(context.Background(), h, suite, &touchingArm{marker: "touched"})
	if err != nil {
		t.Fatalf("runSuite: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	for _, r := range results {
		if !r.Comparison.Layer1SequenceMatch {
			t.Errorf("turn %s: layer1 mismatch — observed %+v (workspace state leaked between turns)",
				r.TurnID, r.AssGuardCalls)
		}
	}

	// The base snapshot itself is never mutated (a corrupted base would poison
	// every later turn AND every later suite run).
	if got := readFile(t, filepath.Join(base, "state.txt")); got != "pristine" {
		t.Errorf("base snapshot mutated: %q", got)
	}
}

// TestRunSuite_FixtureSnapshotSource pins the snapshot precedence: a turn's
// own recorded snapshot seeds its scratch when present, else the suite's base
// snapshot; with no base anywhere, a snapshot-less sibling gets a FRESH dir.
func TestRunSuite_FixtureSnapshotSource(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	writeFile(t, filepath.Join(base, "state.txt"), "from-base")

	snap := t.TempDir()
	writeFile(t, filepath.Join(snap, "state.txt"), "from-snapshot")

	suite := []parity.CapturedTurn{
		{TurnID: "snap", Prompt: "own snapshot", FixtureSnapshot: snap},
		{TurnID: "base", Prompt: "base fallback (the documented fallback)"},
	}

	h := parity.NewHarness()
	h.BaseWorkspace = base

	results, err := parity.RunSuite(context.Background(), h, suite, &readingArm{})
	if err != nil {
		t.Fatalf("runSuite: %v", err)
	}

	want := map[string]string{"snap": "from-snapshot", "base": "from-base"}

	for _, r := range results {
		got := string(r.AssGuardCalls[0].Input)
		if wantStr := "state=" + want[r.TurnID]; got != wantStr {
			t.Errorf("turn %s workspace = %s, want %s", r.TurnID, got, wantStr)
		}
	}

	// No base configured anywhere + one turn carrying a snapshot: the
	// snapshot-less sibling still gets a FRESH dir (never residue).
	noBase := []parity.CapturedTurn{
		{TurnID: "carries", Prompt: "has snapshot", FixtureSnapshot: snap},
		{TurnID: "bare", Prompt: "no snapshot, no base"},
	}

	results, err = parity.RunSuite(context.Background(), parity.NewHarness(), noBase, &readingArm{})
	if err != nil {
		t.Fatalf("runSuite (no base): %v", err)
	}

	if got := string(results[1].AssGuardCalls[0].Input); got != "state="+stateMissing {
		t.Errorf("bare turn workspace = %s, want fresh empty (%s)", got, stateMissing)
	}
}

// readingArm reads the scratch state file without mutating it.
type readingArm struct{}

func (a *readingArm) RunTurn(_ context.Context, _ string) ([]parity.ToolCall, error) {
	return nil, errWorkspaceRequired
}

func (a *readingArm) RunTurnInWorkspace(_ context.Context, _, dir string) ([]parity.ToolCall, error) {
	before := stateMissing

	raw, readErr := os.ReadFile(filepath.Join(dir, "state.txt"))
	if readErr == nil {
		before = string(raw)
	}

	return []parity.ToolCall{{Name: toolObserved, Input: []byte("state=" + before)}}, nil
}

// TestRunSuite_DoubleRunDeterminism proves no cross-RUN residue: running the
// suite twice in one process against a mutating arm yields identical results
// (each pass seeds fresh scratches from the untouched base).
func TestRunSuite_DoubleRunDeterminism(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	writeFile(t, filepath.Join(base, "state.txt"), "pristine")

	expectPristine := []parity.ToolCall{{Name: toolObserved, Input: []byte("state=pristine")}}
	suite := []parity.CapturedTurn{
		{TurnID: "t1", Prompt: "mutate", ExpectedToolCalls: expectPristine},
	}

	h := parity.NewHarness()
	h.BaseWorkspace = base

	runOnce := func() []string {
		t.Helper()

		results, err := parity.RunSuite(context.Background(), h, suite, &touchingArm{marker: "pass"})
		if err != nil {
			t.Fatalf("runSuite: %v", err)
		}

		out := make([]string, 0, len(results))
		for _, r := range results {
			out = append(out, string(r.AssGuardCalls[0].Input))
		}

		return out
	}

	first, second := runOnce(), runOnce()
	if !slices.Equal(first, second) {
		t.Errorf("double-run divergence: pass1=%v pass2=%v (scratch residue)", first, second)
	}
}

// TestRunSuite_LegacyPathUnchanged pins the no-config behavior: with no base
// snapshot and no per-turn snapshots the run path never materializes a scratch
// (the curated suite's existing shared behavior — Pitfall 18: no silent
// threshold movement).
func TestRunSuite_LegacyPathUnchanged(t *testing.T) {
	t.Parallel()

	suite := []parity.CapturedTurn{{TurnID: "t1", Prompt: "plain"}}

	h := parity.NewHarness()

	results, err := parity.RunSuite(context.Background(), h, suite, &plainArm{})
	if err != nil {
		t.Fatalf("runSuite: %v", err)
	}

	if len(results) != 1 || results[0].TurnID != "t1" {
		t.Fatalf("legacy run = %+v, want the single t1 result", results)
	}
}

// plainArm consumes no workspace (the LiveArm shape: RunTurn only).
type plainArm struct{}

func (a *plainArm) RunTurn(_ context.Context, _ string) ([]parity.ToolCall, error) {
	return []parity.ToolCall{}, nil
}
