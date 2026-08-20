package parity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeFile is the test helper for seeding fixture trees.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
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

func (a *touchingArm) RunTurn(_ context.Context, _ string) ([]ToolCall, error) {
	return nil, errors.New("touchingArm requires a workspace (implement WorkspaceArm)")
}

func (a *touchingArm) RunTurnInWorkspace(_ context.Context, _ string, dir string) ([]ToolCall, error) {
	path := filepath.Join(dir, "state.txt")
	before := "MISSING"

	if raw, err := os.ReadFile(path); err == nil {
		before = string(raw)
	}

	if err := os.AppendFile(path, []byte(a.marker+"\n"), 0o600); err != nil {
		return nil, err
	}

	return []ToolCall{{Name: "Observed", Input: []byte("state=" + before)}}, nil
}

// TestRunSuite_PerTurnIsolation proves the 12-03 state-pollution fix: turn 1's
// tool calls mutate the scratch file, and turn 2 replays against the ORIGINAL
// fixture content — per-turn seeding, never a sibling's residue (the
// 2026-08-16 drift report's 4/13 artifact class).
func TestRunSuite_PerTurnIsolation(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	writeFile(t, filepath.Join(base, "state.txt"), "pristine")

	suite := []CapturedTurn{
		{TurnID: "t1", Prompt: "mutate", ExpectedToolCalls: []ToolCall{{Name: "Observed", Input: []byte("state=pristine")}}},
		{TurnID: "t2", Prompt: "depends on pristine state", ExpectedToolCalls: []ToolCall{{Name: "Observed", Input: []byte("state=pristine")}}},
	}

	h := NewHarness()
	h.BaseWorkspace = base

	results, err := runSuite(context.Background(), h, suite, &touchingArm{marker: "touched"})
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
// snapshot — and a turn with neither still gets a FRESH dir.
func TestRunSuite_FixtureSnapshotSource(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	writeFile(t, filepath.Join(base, "state.txt"), "from-base")

	snap := t.TempDir()
	writeFile(t, filepath.Join(snap, "state.txt"), "from-snapshot")

	suite := []CapturedTurn{
		{TurnID: "snap", Prompt: "own snapshot", FixtureSnapshot: snap},
		{TurnID: "base", Prompt: "base fallback"},
		{TurnID: "fresh", Prompt: "neither — fresh empty scratch"},
	}

	h := NewHarness()
	h.BaseWorkspace = base

	results, err := runSuite(context.Background(), h, suite, &readingArm{})
	if err != nil {
		t.Fatalf("runSuite: %v", err)
	}

	want := map[string]string{"snap": "from-snapshot", "base": "from-base", "fresh": "MISSING"}
	for _, r := range results {
		got := string(r.AssGuardCalls[0].Input)
		if wantStr := "state=" + want[r.TurnID]; got != wantStr {
			t.Errorf("turn %s workspace = %s, want %s", r.TurnID, got, wantStr)
		}
	}
}

// readingArm reads the scratch state file without mutating it.
type readingArm struct{}

func (a *readingArm) RunTurn(_ context.Context, _ string) ([]ToolCall, error) {
	return nil, errors.New("readingArm requires a workspace")
}

func (a *readingArm) RunTurnInWorkspace(_ context.Context, _ string, dir string) ([]ToolCall, error) {
	before := "MISSING"

	if raw, err := os.ReadFile(filepath.Join(dir, "state.txt")); err == nil {
		before = string(raw)
	}

	return []ToolCall{{Name: "Observed", Input: []byte("state=" + before)}}, nil
}

// TestRunSuite_DoubleRunDeterminism proves no cross-RUN residue: running the
// suite twice in one process against a mutating arm yields identical results
// (each pass seeds fresh scratches from the untouched base).
func TestRunSuite_DoubleRunDeterminism(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	writeFile(t, filepath.Join(base, "state.txt"), "pristine")

	suite := []CapturedTurn{
		{TurnID: "t1", Prompt: "mutate", ExpectedToolCalls: []ToolCall{{Name: "Observed", Input: []byte("state=pristine")}}},
	}

	h := NewHarness()
	h.BaseWorkspace = base

	runOnce := func() []string {
		t.Helper()

		results, err := runSuite(context.Background(), h, suite, &touchingArm{marker: "pass"})
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

	suite := []CapturedTurn{{TurnID: "t1", Prompt: "plain"}}

	h := NewHarness()

	results, err := runSuite(context.Background(), h, suite, &plainArm{})
	if err != nil {
		t.Fatalf("runSuite: %v", err)
	}

	if len(results) != 1 || results[0].TurnID != "t1" {
		t.Fatalf("legacy run = %+v, want the single t1 result", results)
	}
}

// plainArm consumes no workspace (the LiveArm shape: RunTurn only).
type plainArm struct{}

func (a *plainArm) RunTurn(_ context.Context, _ string) ([]ToolCall, error) {
	return []ToolCall{}, nil
}
