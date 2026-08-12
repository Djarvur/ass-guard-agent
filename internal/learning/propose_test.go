package learning_test

import (
	"reflect"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/learning"
)

// wl builds a worklog of tool names quickly.
func wl(turn string, tools ...string) []learning.WorklogEntry {
	out := make([]learning.WorklogEntry, len(tools))
	for i, t := range tools {
		out[i] = learning.WorklogEntry{TurnID: turn, Tool: t}
	}

	return out
}

// TestProposeHooks_RepeatedSequence verifies a 3-step sequence appearing 3
// times yields a Proposal with Occurrences=3 + the sequence's steps.
func TestProposeHooks_RepeatedSequence(t *testing.T) {
	worklog := append(wl("t1", "Read", "Grep", "Bash"), wl("t2", "Read", "Grep", "Bash")...)
	worklog = append(worklog, wl("t3", "Read", "Grep", "Bash")...)

	proposals := learning.ProposeHooks(worklog)
	if len(proposals) == 0 {
		t.Fatal("no proposals; want at least one for [Read Grep Bash] x3")
	}

	var best learning.Proposal
	for _, p := range proposals {
		if p.Occurrences > best.Occurrences {
			best = p
		}
	}

	if best.Occurrences < 3 {
		t.Errorf("best Occurrences = %d; want >= 3", best.Occurrences)
	}

	if !reflect.DeepEqual(best.Steps, []string{"Read", "Grep", "Bash"}) {
		t.Errorf("best Steps = %v; want [Read Grep Bash]", best.Steps)
	}
}

// catWL concatenates worklog slices.
func catWL(parts ...[]learning.WorklogEntry) []learning.WorklogEntry {
	var out []learning.WorklogEntry
	for _, p := range parts {
		out = append(out, p...)
	}

	return out
}

// TestProposeHooks_BelowThreshold verifies a sequence appearing only 2 times
// yields no Proposal.
func TestProposeHooks_BelowThreshold(t *testing.T) {
	worklog := catWL(wl("t1", "Read", "Grep"), wl("t2", "Read", "Grep"))
	if proposals := learning.ProposeHooks(worklog); len(proposals) != 0 {
		t.Errorf("got %d proposals; want 0 (< 3 occurrences)", len(proposals))
	}
}

// TestProposeHooks_DistinctSequences verifies two distinct repeated sequences
// each yield a Proposal.
func TestProposeHooks_DistinctSequences(t *testing.T) {
	worklog := catWL(
		wl("t1", "Read", "Grep"), wl("t2", "Read", "Grep"), wl("t3", "Read", "Grep"),
		wl("t4", "Write", "Edit"), wl("t5", "Write", "Edit"), wl("t6", "Write", "Edit"),
	)

	proposals := learning.ProposeHooks(worklog)
	if len(proposals) < 2 {
		t.Errorf("got %d proposals; want >= 2 (two distinct sequences)", len(proposals))
	}
}

// TestProposeHooks_Deterministic asserts determinism: the same input yields the
// same output across calls (propose.go is a pure function).
func TestProposeHooks_Deterministic(t *testing.T) {
	worklog := catWL(wl("t1", "Read", "Grep"), wl("t2", "Read", "Grep"), wl("t3", "Read", "Grep"))
	first := learning.ProposeHooks(worklog)

	second := learning.ProposeHooks(worklog)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("ProposeHooks not deterministic: %v vs %v", first, second)
	}
}
