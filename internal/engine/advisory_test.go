package engine_test

import (
	"context"
	"strings"
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

// TestClassifyQuestionEnding_MultiClass (13-03 T2 Test 5): the second class
// (open-question, harvest-informed) matches its own closings and NOT the
// choice class's; first-match-wins order is pinned (a closing matching both
// yields the table's earlier class).
func TestClassifyQuestionEnding_MultiClass(t *testing.T) {
	t.Parallel()

	got, ok := engine.ClassifyQuestionEnding("If you'd like to continue, just say the word and I'll draft it.")
	if !ok || got != engine.ClassOpenQuestionEnding {
		t.Errorf("open-question classify = (%q, %v); want (%q, true)", got, ok, engine.ClassOpenQuestionEnding)
	}

	// Its own closings do not cross-match the choice class.
	got2, ok2 := engine.ClassifyQuestionEnding("let me know which store you meant")
	if !ok2 || got2 != engine.ClassOpenQuestionEnding {
		t.Errorf("open-question phrase classify = (%q, %v)", got2, ok2)
	}

	// The choice class still matches its shape.
	if got, ok := engine.ClassifyQuestionEnding("Which would you like? (1/2)"); !ok || got != engine.ClassChoiceEnding {
		t.Errorf("choice classify = (%q, %v)", got, ok)
	}
}

// TestDecide_AdvisorySeededNonInterference (13-03 T2 Test 6, the D-07 note):
// with a table whose text row MATCHES the closing, Decide returns the
// continue decision and NO advisory signal — the advisory lives only in the
// unmatched cell.
func TestDecide_AdvisorySeededNonInterference(t *testing.T) {
	t.Parallel()

	table := fakeTable{textPatterns: map[string]engine.Action{
		"say the word": engine.ActionContinue,
	}}

	dec := engine.Decide(engine.TurnOutput{
		TurnID: turn001, Text: "Ready when you are — just say the word.",
	}, table)

	if dec.Action != engine.ActionContinue {
		t.Errorf("Action = %v; want Continue (the seed matched — no advisory interference)", dec.Action)
	}

	if strings.HasPrefix(dec.Signal, engine.SignalAdvisory) {
		t.Errorf("Signal = %q; want the seeded signal, never an advisory on a matched closing", dec.Signal)
	}
}

// TestDecide_AdvisoryAskPrecedence (13-03 T2 Test 7): an AskSuspended turn
// yields ActionAsk + SignalAskSuspended, never an advisory (the suspension
// check precedes everything).
func TestDecide_AdvisoryAskPrecedence(t *testing.T) {
	t.Parallel()

	dec := engine.Decide(engine.TurnOutput{
		TurnID: turn001, AskSuspended: true, Text: "Which would you like? (1/2)",
	}, fakeTable{})

	if dec.Action != engine.ActionAsk || dec.Signal != engine.SignalAskSuspended {
		t.Errorf("decision = (%v, %q); want ActionAsk + %s (suspension first)",
			dec.Action, dec.Signal, engine.SignalAskSuspended)
	}
}
