package engine_test

import (
	"context"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// TestClassifyQuestionEnding_ChoiceClass (13-03 T1 Test 1, RED first): the
// captured 08-06 stage-4 choice-ending shape classifies; a plain declarative
// closing does not; the class id is a FIXED table constant, never derived
// from the matched text.
func TestClassifyQuestionEnding_ChoiceClass(t *testing.T) {
	t.Parallel()

	got, ok := engine.ClassifyQuestionEnding("Which would you like to do next? (1/2)")
	if !ok {
		t.Fatal("the captured choice-ending shape did not classify")
	}

	if got != engine.ClassChoiceEnding {
		t.Errorf("class = %q; want the fixed constant %q", got, engine.ClassChoiceEnding)
	}

	if _, ok := engine.ClassifyQuestionEnding("The implementation is complete."); ok {
		t.Error("a plain declarative closing classified as question-shaped")
	}
}

// TestDecide_AdvisoryOnUnmatchedQuestionEnding (13-03 T1 Test 2): with a
// table matching nothing, a question-shaped ending yields ActionNothing with
// Signal advisory:<class>; a NON-question unmatched ending keeps Signal
// resultUnmatched exactly as today.
func TestDecide_AdvisoryOnUnmatchedQuestionEnding(t *testing.T) {
	t.Parallel()

	table := fakeTable{} // matches nothing

	dec := engine.Decide(engine.TurnOutput{
		TurnID: turn001, Text: "Which would you like to do next? (1/2)",
	}, table)

	if dec.Action != engine.ActionNothing {
		t.Errorf("advisory decision Action = %v; want ActionNothing (the advisory NEVER holds)", dec.Action)
	}

	if dec.Signal != engine.SignalAdvisory+engine.ClassChoiceEnding {
		t.Errorf("advisory Signal = %q; want %q", dec.Signal, engine.SignalAdvisory+engine.ClassChoiceEnding)
	}

	dec2 := engine.Decide(engine.TurnOutput{
		TurnID: turn002, Text: "an honest unmatched closing with no anchor",
	}, table)

	if dec2.Signal != resultUnmatched {
		t.Errorf("non-question unmatched Signal = %q; want today's \"unmatched\"", dec2.Signal)
	}
}

// TestDecide_AdvisoryNeverHolds (13-03 T1 Test 3): the advisory decision's
// Action is ActionNothing — Observe does NOT re-enter the runner on it (no
// continue injection; the Phase-8 question-ending auto-continue rejection
// stays untouched).
func TestDecide_AdvisoryNeverHolds(t *testing.T) {
	t.Parallel()

	table := fakeTable{}

	dec := engine.Decide(engine.TurnOutput{
		TurnID: turn001, Text: "Which option? (1/2)",
	}, table)

	if dec.Action != engine.ActionNothing {
		t.Fatalf("Action = %v; want Nothing", dec.Action)
	}

	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: turn001, Text: "Which option? (1/2)"},
	}}

	eng := &engine.Engine{}

	stop, err := eng.Observe(context.Background(), runner, table,
		[]session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	if got := runner.runCalls(); got != 1 {
		t.Errorf("runner.Run called %d times; want 1 (the advisory never re-enters the runner)", got)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn", stop)
	}
}
