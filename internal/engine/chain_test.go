package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// chainPatternID is the stage-bearing fixture pattern id (goconst).
const chainPatternID = "post-explore-handoff"

// continueTable matches any text containing "handoff" and reports a
// stage-bearing pattern id; everything else is unmatched.
type continueTable struct{}

func (continueTable) MatchText(text string) engine.MatchDetail {
	if strings.Contains(text, "handoff") {
		return engine.MatchDetail{
			ID:           chainPatternID,
			Action:       engine.ActionContinue,
			Span:         "handoff",
			ConfigSource: "fake patterns/" + chainPatternID,
		}
	}

	return engine.MatchDetail{}
}

func (continueTable) MatchTool(string) engine.MatchDetail {
	return engine.MatchDetail{}
}

// populatingDispatcher is a fakeDispatcher extended with ContinuePopulator:
// it fills Decision.NextPrompt from a signal→command map (the 08-06 chaining
// seam).
type populatingDispatcher struct {
	fakeDispatcher

	// next maps pattern id → the next command text to inject.
	next map[string]string
}

func (p *populatingDispatcher) PopulateContinue(dec *engine.Decision) {
	// Strip any signal prefix ("text:" / "tool:" / "command:") to the bare
	// pattern id — the same contract the real acpDispatcher.PopulateContinue
	// implements (08-06 chaining + the hybrid provenance signal).
	id := dec.Signal
	for _, prefix := range []string{"text:", "tool:", "command:"} {
		id = strings.TrimPrefix(id, prefix)
	}

	if next, ok := p.next[id]; ok {
		dec.NextPrompt = []session.ContentBlock{{Type: blockText, Text: next}}
	}
}

// TestChain_PopulatorFillsNextPrompt (08-06 Test 3): when a continue decision
// carries no NextPrompt, the dispatcher's ContinuePopulator fills it BEFORE
// injection — the injected runner prompt is the populator's command text, not
// the generic "continue" fallback.
func TestChain_PopulatorFillsNextPrompt(t *testing.T) {
	t.Parallel()

	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: "t1", Text: "stage done, handoff present"},
		{TurnID: "t2", Text: "final stage, nothing more"},
	}}

	d := &populatingDispatcher{next: map[string]string{
		chainPatternID: "/opsx:propose add-login",
	}}

	eng := &engine.Engine{Bus: event.NewBus(), Dispatcher: d}

	_, err := eng.Observe(context.Background(), runner, continueTable{},
		[]session.ContentBlock{{Type: blockText, Text: "/opsx:explore add-login"}})
	if err != nil {
		t.Fatalf("Observe err = %v", err)
	}

	if got := runner.calls.Load(); got != 2 {
		t.Fatalf("runner Run calls = %d; want 2 (user turn + 1 injection)", got)
	}

	if len(runner.prompts) < 2 {
		t.Fatal("runner recorded fewer than 2 prompts")
	}

	injected := runner.prompts[1]
	if len(injected) != 1 || injected[0].Text != "/opsx:propose add-login" {
		t.Errorf("injected prompt = %+v; want the populator's /opsx:propose text", injected)
	}
}

// TestChain_BudgetFourStageChain (08-06 Test 6): the full
// explore→propose→apply→archive chain is 4 turns with 3 continuations — well
// within MaxContinueInjections=8; the engine stops cleanly at archive's
// non-continue decision.
func TestChain_BudgetFourStageChain(t *testing.T) {
	t.Parallel()

	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: "t1", Text: "explored; handoff"},  // explore → continue
		{TurnID: "t2", Text: "proposed; handoff"},  // propose → continue
		{TurnID: "t3", Text: "applied; handoff"},   // apply → continue
		{TurnID: "t4", Text: "archived; all done"}, // archive → nothing
	}}

	d := &populatingDispatcher{next: map[string]string{
		"post-explore-handoff": "/opsx:propose",
	}}

	eng := &engine.Engine{Bus: event.NewBus(), Dispatcher: d}

	stop, err := eng.Observe(context.Background(), runner, continueTable{},
		[]session.ContentBlock{{Type: blockText, Text: "/opsx:explore"}})
	if err != nil {
		t.Fatalf("Observe err = %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn (clean stop at the archive decision)", stop)
	}

	if got := runner.calls.Load(); got != 4 {
		t.Errorf("runner Run calls = %d; want 4 (user + 3 continuations, within budget 8)", got)
	}
}
