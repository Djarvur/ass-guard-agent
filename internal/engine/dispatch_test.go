package engine_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// fakeDispatcher is an ActionDispatcher test double recording its calls +
// returning scripted Hook status / Ask answer.
type fakeDispatcher struct {
	mu         sync.Mutex
	hookCalls  []string
	askCalls   []string
	hookStatus string
	askAnswer  string
	askErr     error
}

func (f *fakeDispatcher) Hook(_ context.Context, signal, turnID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.hookCalls = append(f.hookCalls, signal+"@"+turnID)
	if f.hookStatus == "" {
		return "completed"
	}

	return f.hookStatus
}

func (f *fakeDispatcher) Ask(_ context.Context, situation string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.askCalls = append(f.askCalls, situation)

	return f.askAnswer, f.askErr
}

// hookTable is a PatternTable whose text match yields ActionHook (so applyDispatcher
// routes through Dispatcher.Hook).
type hookTable struct{}

func (hookTable) MatchText(string) (string, engine.Action) {
	return "proposal-ready", engine.ActionHook
}
func (hookTable) MatchTool(string) (string, engine.Action) { return "", engine.ActionNothing }

// askTable yields ActionAsk on every text match.
type askTable struct{}

func (askTable) MatchText(string) (string, engine.Action) {
	return "unmatched-launch", engine.ActionAsk
}
func (askTable) MatchTool(string) (string, engine.Action) { return "", engine.ActionNothing }

// TestDispatch_HookCallsDispatcher verifies ActionHook dispatches through
// Dispatcher.Hook + the EngineDecision carries the hook status (T1).
func TestDispatch_HookCallsDispatcher(t *testing.T) {
	runner := &scriptedRunner{outputs: []engine.TurnOutput{{TurnID: "h1", Text: "trigger"}}}
	bus := event.NewBus()
	d := &fakeDispatcher{hookStatus: "completed"}
	eng := &engine.Engine{Bus: bus, Dispatcher: d}
	collect := captureEvents(t, bus)

	stop, err := eng.Observe(context.Background(), runner, hookTable{}, []session.ContentBlock{{Type: "text", Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err = %v", err)
	}

	_ = stop

	if len(d.hookCalls) != 1 {
		t.Fatalf("Hook calls = %v; want exactly 1", d.hookCalls)
	}

	events := collect()
	if len(events) == 0 {
		t.Fatal("no EngineDecision events emitted")
	}

	last := events[len(events)-1]
	if !strings.Contains(last.Reason, "completed") {
		t.Errorf("last event Reason = %q; want it to carry the hook status", last.Reason)
	}
}

// TestDispatch_AskWithStoredAnswerContinues verifies ActionAsk with a stored
// "continue" answer ⇒ the engine continues (re-enters the runner).
func TestDispatch_AskWithStoredAnswerContinues(t *testing.T) {
	// Turn 1 matches the ask signal; the dispatcher returns "continue" so the
	// engine re-enters the runner. Turn 2's text does NOT match askTable...
	// except askTable.MatchText ALWAYS returns ActionAsk. So we'd loop to the
	// budget. To test the "continue" path deterministically, use a table that
	// asks once then stops: turn 1 asks, turn 2 yields nothing.
	table := &askOnceTable{}
	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: "a1", Text: "first"},
		{TurnID: "a2", Text: "second"},
	}}
	bus := event.NewBus()
	d := &fakeDispatcher{askAnswer: "continue"}
	eng := &engine.Engine{Bus: bus, Dispatcher: d}
	collect := captureEvents(t, bus)

	stop, err := eng.Observe(context.Background(), runner, table, []session.ContentBlock{{Type: "text", Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err = %v", err)
	}

	_ = stop

	if got := runner.runCalls(); got != 2 {
		t.Errorf("Run calls = %d; want 2 (ask returned continue ⇒ re-entered)", got)
	}

	if len(d.askCalls) != 1 {
		t.Errorf("Ask calls = %d; want 1 (only the first turn asked)", len(d.askCalls))
	}

	events := collect()
	if len(events) == 0 {
		t.Fatal("no events emitted")
	}
}

// askOnceTable yields ActionAsk on the FIRST call, then ActionNothing (so the
// engine continues after the stored answer, then stops on the second turn).
type askOnceTable struct {
	called bool
}

func (a *askOnceTable) MatchText(_ string) (string, engine.Action) {
	if a.called {
		return "", engine.ActionNothing
	}

	a.called = true

	return "unmatched-launch", engine.ActionAsk
}
func (a *askOnceTable) MatchTool(string) (string, engine.Action) { return "", engine.ActionNothing }

// TestDispatch_AskPendingBreaksLoop verifies ActionAsk with no stored answer
// (ErrAskPending) ⇒ an ask EngineDecision is emitted + the loop breaks (Run
// called once, no continue-injection).
func TestDispatch_AskPendingBreaksLoop(t *testing.T) {
	runner := &scriptedRunner{outputs: []engine.TurnOutput{{TurnID: "a1", Text: "first"}}}
	bus := event.NewBus()
	d := &fakeDispatcher{askErr: engine.ErrAskPending}
	eng := &engine.Engine{Bus: bus, Dispatcher: d}
	collect := captureEvents(t, bus)

	stop, err := eng.Observe(context.Background(), runner, askTable{}, []session.ContentBlock{{Type: "text", Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err = %v", err)
	}

	_ = stop

	if got := runner.runCalls(); got != 1 {
		t.Errorf("Run calls = %d; want 1 (loop breaks on pending ask)", got)
	}

	events := collect()
	if len(events) != 1 {
		t.Fatalf("events = %d; want 1 (the ask decision)", len(events))
	}

	if events[0].Action != "ask" {
		t.Errorf("event Action = %q; want ask", events[0].Action)
	}
}

// TestDispatch_NilDispatcherDegradesToNothing verifies a nil Dispatcher makes
// hook/ask degrade to nothing (the tracer / 04-01 contract preserved).
func TestDispatch_NilDispatcherDegradesToNothing(t *testing.T) {
	runner := &scriptedRunner{outputs: []engine.TurnOutput{{TurnID: "h1", Text: "trigger"}}}
	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus} // no Dispatcher
	collect := captureEvents(t, bus)
	_, _ = eng.Observe(context.Background(), runner, hookTable{}, []session.ContentBlock{{Type: "text", Text: "go"}})

	events := collect()
	if len(events) == 0 {
		t.Fatal("no events")
	}

	if events[0].Action != "nothing" {
		t.Errorf("nil-dispatcher hook degraded to %q; want nothing", events[0].Action)
	}
}

// guard against unused imports.
var _ = errors.New
