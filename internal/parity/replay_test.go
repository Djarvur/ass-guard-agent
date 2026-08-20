package parity_test

import (
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
)

// TestExtractTurnsFromRollout parses a fixture rollout and confirms prompt→tool-
// call pairing. A turn with zero tool-calls produces an empty ExpectedToolCalls.
func TestExtractTurnsFromRollout(t *testing.T) {
	t.Parallel()

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

	if len(turns[0].ExpectedToolCalls) != 1 || turns[0].ExpectedToolCalls[0].Name != toolRead {
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
	if names[0] != toolRead || names[1] != toolRead || names[2] != toolBash {
		t.Errorf("multi-tool turn names = %v, want [Read Read Bash]", names)
	}
}

// TestExtractTurnsFromRollout_EmptyToolCalls confirms a zero-tool turn is valid.
func TestExtractTurnsFromRollout_EmptyToolCalls(t *testing.T) {
	t.Parallel()

	path := filepath.Join(".", "testdata", "sample-rollout.jsonl")

	turns, _ := parity.ExtractTurnsFromRollout(path)
	for _, trn := range turns {
		if trn.ExpectedToolCalls == nil {
			t.Errorf("turn %q: ExpectedToolCalls is nil (must be a non-nil empty slice)", trn.TurnID)
		}
	}
}

// deltaFixturePath is the committed synthetic delta-record fixture (12-03): a
// model-io-sess-shaped line file whose request records carry messagesKind=delta
// entries with messageOffset fields — structure real, payloads synthetic (the
// D-03 sanitization convention; real rollouts rotate off disk, D-04).
func deltaFixturePath(t *testing.T) string {
	t.Helper()

	return filepath.Join(".", "testdata", "delta-records.jsonl")
}

// TestExtractTurnsFromRollout_DeltaRecords pins the 12-03 zero-empty fix: every
// tool-bearing turn extracts NON-empty ExpectedToolCalls, including turns whose
// tool_use blocks arrive only as delta-streamed records split across multiple
// lines (the 2026-08-16 drift report's 6/13 empty-expectation artifact class).
func TestExtractTurnsFromRollout_DeltaRecords(t *testing.T) {
	t.Parallel()

	turns, err := parity.ExtractTurnsFromRollout(deltaFixturePath(t))
	if err != nil {
		t.Fatalf("ExtractTurnsFromRollout: %v", err)
	}

	byID := map[string]parity.CapturedTurn{}
	for _, trn := range turns {
		byID[trn.TurnID] = trn
	}

	for _, id := range []string{"turn_synth_full_1", "turn_synth_delta_1", "turn_synth_inter_1", "turn_synth_mixed_1"} {
		trn, ok := byID[id]
		if !ok {
			t.Fatalf("turn %s missing from extraction (got %d turns)", id, len(turns))
		}

		if len(trn.ExpectedToolCalls) == 0 {
			t.Errorf("turn %s: ExpectedToolCalls is EMPTY (the delta artifact class)", id)
		}

		if trn.Prompt == "" {
			t.Errorf("turn %s: prompt is empty", id)
		}
	}

	// Single-tool delta turn split across 3 lines: the streamed tool_use
	// reconstructs with exact name + input.
	if trn := byID["turn_synth_delta_1"]; len(trn.ExpectedToolCalls) != 1 ||
		trn.ExpectedToolCalls[0].Name != "Grep" ||
		string(trn.ExpectedToolCalls[0].Input) == "" {
		t.Errorf("delta turn calls = %+v, want exactly [Grep] with input", trn.ExpectedToolCalls)
	}

	// Two-tool interleaved delta turn: both tools, exact names + inputs.
	if trn := byID["turn_synth_inter_1"]; len(trn.ExpectedToolCalls) != 2 ||
		trn.ExpectedToolCalls[0].Name != "Edit" ||
		trn.ExpectedToolCalls[1].Name != "Write" {
		t.Errorf("interleaved turn calls = %+v, want [Edit Write]", trn.ExpectedToolCalls)
	}

	// Plain no-tool turn stays a VALID empty-sequence entry (never nil).
	if trn := byID["turn_synth_notool_1"]; trn.ExpectedToolCalls == nil {
		t.Error("no-tool turn ExpectedToolCalls is nil (must be non-nil empty)")
	} else if len(trn.ExpectedToolCalls) != 0 {
		t.Errorf("no-tool turn calls = %+v, want empty", trn.ExpectedToolCalls)
	}
}

// TestExtractTurnsFromRollout_DeltaOffsetArithmetic proves the messageOffset
// assembly: interleaved delta records streaming two assistant messages
// alternately (out-of-order arrival) reconstruct both messages without
// cross-contamination — the 08-09 live-verified offset logic, pinned offline.
func TestExtractTurnsFromRollout_DeltaOffsetArithmetic(t *testing.T) {
	t.Parallel()

	turns, err := parity.ExtractTurnsFromRollout(deltaFixturePath(t))
	if err != nil {
		t.Fatalf("ExtractTurnsFromRollout: %v", err)
	}

	for _, trn := range turns {
		if trn.TurnID != "turn_synth_inter_1" {
			continue
		}

		if len(trn.ExpectedToolCalls) != 2 {
			t.Fatalf("interleaved turn calls = %+v, want 2", trn.ExpectedToolCalls)
		}

		if got := string(trn.ExpectedToolCalls[0].Input); got != `{"file_path":"a.go"}` {
			t.Errorf("tool 0 input = %s, want the a.go payload (no cross-contamination)", got)
		}

		if got := string(trn.ExpectedToolCalls[1].Input); got != `{"file_path":"b.go"}` {
			t.Errorf("tool 1 input = %s, want the b.go payload (no cross-contamination)", got)
		}

		return
	}

	t.Fatal("interleaved turn not found")
}

// TestExtractTurnsFromRollout_MixedCorpus pins the real capture shape: a file
// mixing full-message records and delta records extracts uniformly from both
// kinds — response-pinned calls (full records), streamed tool_use (deltas), and
// the merged order (assembled backbone + response-only tail).
func TestExtractTurnsFromRollout_MixedCorpus(t *testing.T) {
	t.Parallel()

	turns, err := parity.ExtractTurnsFromRollout(deltaFixturePath(t))
	if err != nil {
		t.Fatalf("ExtractTurnsFromRollout: %v", err)
	}

	var full, mixed parity.CapturedTurn

	for _, trn := range turns {
		switch trn.TurnID {
		case "turn_synth_full_1":
			full = trn
		case "turn_synth_mixed_1":
			mixed = trn
		}
	}

	if len(full.ExpectedToolCalls) != 1 || full.ExpectedToolCalls[0].Name != toolRead {
		t.Errorf("full-record turn calls = %+v, want [Read]", full.ExpectedToolCalls)
	}

	if len(mixed.ExpectedToolCalls) != 2 ||
		mixed.ExpectedToolCalls[0].Name != "Grep" ||
		mixed.ExpectedToolCalls[1].Name != "Bash" {
		t.Errorf("mixed turn calls = %+v, want [Grep Bash] (streamed + response tail)",
			mixed.ExpectedToolCalls)
	}
}
