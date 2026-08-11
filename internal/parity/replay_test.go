package parity_test

import (
	"path/filepath"
	"testing"

	"github.com/djarvur/ass-guard-agent/internal/parity"
)

// TestExtractTurnsFromRollout parses a fixture rollout and confirms prompt→tool-
// call pairing. A turn with zero tool-calls produces an empty ExpectedToolCalls.
func TestExtractTurnsFromRollout(t *testing.T) {
	path := filepath.Join(".", "testdata", "sample-rollout.jsonl")
	turns, err := parity.ExtractTurnsFromRollout(path)
	if err != nil {
		t.Fatalf("ExtractTurnsFromRollout: %v", err)
	}
	if len(turns) < 2 {
		t.Fatalf("len(turns) = %d, want >= 2", len(turns))
	}
	// First turn: a "read go.mod" prompt → [Read].
	if turns[0].Prompt == "" {
		t.Error("turn 0 prompt is empty")
	}
	if len(turns[0].ExpectedToolCalls) != 1 || turns[0].ExpectedToolCalls[0].Name != "Read" {
		t.Errorf("turn 0 calls = %+v, want [Read]", turns[0].ExpectedToolCalls)
	}
	// Find the multi-tool turn ([Read, Read, Bash]).
	var multi *parity.CapturedTurn
	for i := range turns {
		if len(turns[i].ExpectedToolCalls) == 3 {
			multi = &turns[i]
			break
		}
	}
	if multi == nil {
		t.Fatal("no 3-tool turn found in fixture")
	}
	names := []string{multi.ExpectedToolCalls[0].Name, multi.ExpectedToolCalls[1].Name, multi.ExpectedToolCalls[2].Name}
	if names[0] != "Read" || names[1] != "Read" || names[2] != "Bash" {
		t.Errorf("multi-tool turn names = %v, want [Read Read Bash]", names)
	}
}

// TestExtractTurnsFromRollout_EmptyToolCalls confirms a zero-tool turn is valid.
func TestExtractTurnsFromRollout_EmptyToolCalls(t *testing.T) {
	path := filepath.Join(".", "testdata", "sample-rollout.jsonl")
	turns, _ := parity.ExtractTurnsFromRollout(path)
	for _, trn := range turns {
		if trn.ExpectedToolCalls == nil {
			t.Errorf("turn %q: ExpectedToolCalls is nil (must be a non-nil empty slice)", trn.TurnID)
		}
	}
}
