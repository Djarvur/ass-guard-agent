package engine //nolint:testpackage // internal test: reaches the unexported applyDispatcher

import (
	"context"
	"testing"
)

// TestDecide_AskSuspendedNeverChains (12-01 T1 Test 3, the chained-stage
// hazard): a TurnOutput carrying the ask-suspension marker decides ActionAsk —
// NEVER ActionContinue — even when the table's text/tool rows WOULD match (the
// Phase-8 chaining machinery would otherwise auto-continue a suspended stage)
// and even when the turn was started by a command with provenance rows. No
// table lookup happens: a suspended turn matches nothing.
func TestDecide_AskSuspendedNeverChains(t *testing.T) {
	t.Parallel()

	// A table whose EVERY signal matches with ActionContinue.
	table := allContinueTable{}

	out := TurnOutput{
		TurnID:       askTestTurnID,
		Text:         "done. ## Implementation Complete — ready for review",
		ToolCalls:    []string{"OpenSpecHandoff"},
		StartedBy:    "opsx:explore", // provenance rows would also fire
		AskSuspended: true,
	}

	dec := Decide(out, table)

	if dec.Action != ActionAsk {
		t.Fatalf("Action = %v; want ActionAsk (a suspended turn must never chain)", dec.Action)
	}

	if dec.Signal != SignalAskSuspended {
		t.Errorf("Signal = %q; want %q", dec.Signal, SignalAskSuspended)
	}

	if dec.TurnID != askTestTurnID {
		t.Errorf("TurnID = %q; want t-ask (provenance)", dec.TurnID)
	}
}

// askTestTurnID is the canned suspended-turn id (goconst).
const askTestTurnID = "t-ask"

// askTestAlways is the canned matched-span text (goconst).
const askTestAlways = "always"

// allContinueTable is a PatternTable + CommandMatcher whose every lookup
// returns ActionContinue — the adversarial table for the no-chain pin.
type allContinueTable struct{}

func (allContinueTable) MatchText(string) MatchDetail {
	return MatchDetail{ID: askTestAlways, Action: ActionContinue, Span: askTestAlways}
}

func (allContinueTable) MatchTool(string) MatchDetail {
	return MatchDetail{ID: "always-tool", Action: ActionContinue, Span: askTestAlways}
}

func (allContinueTable) MatchCommand(string) MatchDetail {
	return MatchDetail{ID: "always-cmd", Action: ActionContinue, Span: askTestAlways}
}

// knowEverythingDispatcher is an ActionDispatcher whose Ask ALWAYS returns a
// learned "continue" answer — the adversarial dispatcher for the
// no-chain-through-learning pin (applyDispatcher must not consult the learning
// store for a tool-suspension ask).
type knowEverythingDispatcher struct {
	ActionDispatcher

	askCalls int
}

func (d *knowEverythingDispatcher) Hook(context.Context, string, string) string { return "done" }

func (d *knowEverythingDispatcher) Ask(_ context.Context, _ string) (string, error) {
	d.askCalls++

	return "continue", nil
}

// TestDispatcher_SuspendedAskSkipsLearningStore (12-01): the ask-suspension
// decision is a LIVE user question with a pending tool call — the learning
// store is not consulted (a stored answer must not convert a suspension into a
// continue; the hazard Test 3 pins at Decide level, here at dispatcher level).
func TestDispatcher_SuspendedAskSkipsLearningStore(t *testing.T) {
	t.Parallel()

	d := &knowEverythingDispatcher{}

	e := &Engine{Dispatcher: d}

	dec := Decision{
		TurnID: askTestTurnID,
		Action: ActionAsk,
		Signal: SignalAskSuspended,
		Reason: "turn suspended on AskUserQuestion",
	}

	got := e.applyDispatcher(context.Background(), &dec)

	if got.Action != ActionAsk {
		t.Fatalf("Action = %v; want ActionAsk (a learned answer must not resume-chain a suspended turn)", got.Action)
	}

	if d.askCalls != 0 {
		t.Errorf("learning store consulted %d times; want 0 for a tool-suspension ask", d.askCalls)
	}
}
