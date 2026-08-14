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
	t.Parallel()

	worklog := append(wl("t1", toolRead, toolGrep, toolBash), wl("t2", toolRead, toolGrep, toolBash)...)
	worklog = append(worklog, wl("t3", toolRead, toolGrep, toolBash)...)

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

	if !reflect.DeepEqual(best.Steps, []string{toolRead, toolGrep, toolBash}) {
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
	t.Parallel()

	worklog := catWL(wl("t1", toolRead, toolGrep), wl("t2", toolRead, toolGrep))
	if proposals := learning.ProposeHooks(worklog); len(proposals) != 0 {
		t.Errorf("got %d proposals; want 0 (< 3 occurrences)", len(proposals))
	}
}

// TestProposeHooks_DistinctSequences verifies two distinct repeated sequences
// each yield a Proposal.
func TestProposeHooks_DistinctSequences(t *testing.T) {
	t.Parallel()

	worklog := catWL(
		wl("t1", toolRead, toolGrep), wl("t2", toolRead, toolGrep), wl("t3", toolRead, toolGrep),
		wl("t4", toolWrite, toolEdit), wl("t5", toolWrite, toolEdit), wl("t6", toolWrite, toolEdit),
	)

	proposals := learning.ProposeHooks(worklog)
	if len(proposals) < 2 {
		t.Errorf("got %d proposals; want >= 2 (two distinct sequences)", len(proposals))
	}
}

// TestProposeHooks_Deterministic asserts determinism: the same input yields the
// same output across calls (propose.go is a pure function).
func TestProposeHooks_Deterministic(t *testing.T) {
	t.Parallel()

	worklog := catWL(wl("t1", toolRead, toolGrep), wl("t2", toolRead, toolGrep), wl("t3", toolRead, toolGrep))
	first := learning.ProposeHooks(worklog)

	second := learning.ProposeHooks(worklog)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("ProposeHooks not deterministic: %v vs %v", first, second)
	}
}
