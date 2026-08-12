package engine_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// scriptedRunner is a fake TurnRunner: it returns a scripted slice of
// TurnOutputs in order (one per Run call). It records the prompts it received +
// a count of Run calls. The last scripted output is repeated once exhausted so
// an always-matching fake loops to the budget deterministically.
type scriptedRunner struct {
	mu          sync.Mutex
	outputs     []engine.TurnOutput
	prompts     [][]session.ContentBlock
	calls       atomic.Int32
	lastPanicOn int // if >0, the Nth LastTurnOutput call panics (1-indexed)
	lastCalls   atomic.Int32
}

func (s *scriptedRunner) Run(ctx context.Context, prompt []session.ContentBlock) (string, error) {
	s.calls.Add(1)
	s.mu.Lock()
	s.prompts = append(s.prompts, prompt)

	idx := int(s.calls.Load()) - 1

	if idx >= len(s.outputs) {
		idx = len(s.outputs) - 1
	}

	out := engine.TurnOutput{}
	if idx >= 0 {
		out = s.outputs[idx]
	}
	s.mu.Unlock()

	_ = out // stop reason is always end_turn for the tracer fakes

	return stopEndTurn, nil
}

func (s *scriptedRunner) LastTurnOutput() engine.TurnOutput {
	n := s.lastCalls.Add(1)
	if s.lastPanicOn > 0 && int(n) == s.lastPanicOn {
		panic("simulated LastTurnOutput panic")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// LastTurnOutput reflects the most recent Run's scripted output.
	idx := int(s.calls.Load()) - 1
	if idx < 0 {
		return engine.TurnOutput{}
	}

	if idx >= len(s.outputs) {
		idx = len(s.outputs) - 1
	}

	return s.outputs[idx]
}

func (s *scriptedRunner) runCalls() int { return int(s.calls.Load()) }

// capturingManager is a fake EngineDecisionWriter that records every decision
// written to the transcript seam.
type capturingManager struct {
	mu        sync.Mutex
	decisions []engine.Decision
}

func (c *capturingManager) AppendEngineDecision(turnID, action, signal, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.decisions = append(c.decisions, engine.Decision{
		TurnID: turnID, Action: parseAction(action), Signal: signal, Reason: reason,
	})

	return nil
}

func parseAction(s string) engine.Action {
	for a := engine.ActionNothing; a <= engine.ActionWait; a++ {
		if a.String() == s {
			return a
		}
	}

	return engine.ActionNothing
}

// captureEvents subscribes to EngineDecision on bus. Because Bus.Publish is
// synchronous (it blocks on `ch <- e` until the event is buffered or received)
// and Observe runs in the caller's goroutine, by the time Observe returns every
// event is already in the channel buffer. collect() drains the buffer
// non-blocking — no reader goroutine, so there is no done-channel race.
func captureEvents(t *testing.T, bus *event.Bus) func() []event.EngineDecision {
	t.Helper()
	// Buffer generously (>= the re-fire budget + budget-cap event) so Publish
	// never blocks mid-Observe.
	ch := bus.Subscribe("EngineDecision", 64)

	return func() []event.EngineDecision {
		var got []event.EngineDecision

		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return got
				}

				if d, ok := e.(event.EngineDecision); ok {
					got = append(got, d)
				}
			default:
				return got
			}
		}
	}
}

// TestObserve_ZeroContinueHappyPath is the TRACER assertion: a handoff on the
// first turn, end_turn (unmatched) on the second → runner.Run called TWICE (the
// user prompt + one continue-injection) and ("end_turn", nil) returned.
func TestObserve_ZeroContinueHappyPath(t *testing.T) {
	t.Parallel()

	table := seededTable()
	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: turn001, Text: implementationCompleteMsg},
		{TurnID: turn002, Text: "final answer, nothing more"}, // unmatched
	}}
	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus}
	collect := captureEvents(t, bus)

	stop, err := eng.Observe(context.Background(), runner, table, []session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err = %v; want nil", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn", stop)
	}

	if got := runner.runCalls(); got != 2 {
		t.Errorf("runner.Run called %d times; want 2 (user prompt + 1 continue-injection)", got)
	}
	// The 2nd Run's prompt is the engine's continue-injection.
	runner.mu.Lock()
	secondPrompt := runner.prompts[1]
	runner.mu.Unlock()

	if len(secondPrompt) != 1 || secondPrompt[0].Text != stopContinue {
		t.Errorf("2nd Run prompt = %+v; want the continue-injection [{text continue}]", secondPrompt)
	}
	// Provenance stream: exactly two EngineDecision events (continue, then nothing).
	events := collect()
	if len(events) != 2 {
		t.Fatalf("got %d EngineDecision events; want 2", len(events))
	}

	if events[0].Action != stopContinue || events[0].TurnID != turn001 {
		t.Errorf("event[0] = %+v; want continue/turn-001", events[0])
	}

	if events[1].Action != fixtureNothing || events[1].TurnID != turn002 {
		t.Errorf("event[1] = %+v; want nothing/turn-002 (provenance advances)", events[1])
	}
}

// TestObserve_UnmatchedZeroInjections is the structural-safety end-to-end cell:
// the first turn is unmatched → runner.Run called ONCE, no continue-injection,
// one EngineDecision{nothing} event.
func TestObserve_UnmatchedZeroInjections(t *testing.T) {
	t.Parallel()

	table := seededTable()
	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: turn001, Text: "totally unrelated output"},
	}}
	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus}
	collect := captureEvents(t, bus)

	stop, err := eng.Observe(context.Background(), runner, table, []session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil || stop != stopEndTurn {
		t.Fatalf("Observe = (%q, %v); want (end_turn, nil)", stop, err)
	}

	if got := runner.runCalls(); got != 1 {
		t.Errorf("runner.Run called %d times; want 1 (no continue-injection)", got)
	}

	events := collect()
	if len(events) != 1 || events[0].Action != fixtureNothing {
		t.Fatalf("events = %+v; want exactly one {nothing}", events)
	}
}

// TestObserve_LastTurnOutputPanicDegradation verifies a panicking LastTurnOutput
// ⇒ the ORIGINAL (stop, err) from the first Run are returned (graceful
// degradation, D-04 — the turn already completed).
func TestObserve_LastTurnOutputPanicDegradation(t *testing.T) {
	t.Parallel()

	table := seededTable()
	runner := &scriptedRunner{
		outputs: []engine.TurnOutput{
			{TurnID: turn001, Text: implementationCompleteMsg},
		},
		lastPanicOn: 1, // the first LastTurnOutput call panics
	}
	eng := &engine.Engine{}

	stop, err := eng.Observe(context.Background(), runner, table, []session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err = %v; want nil (panic recovered)", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn (original first-turn result)", stop)
	}

	if got := runner.runCalls(); got != 1 {
		t.Errorf("runner.Run called %d times; want 1 (no injection after panic)", got)
	}
}

// TestObserve_DecidePanicDegradation verifies a panicking PatternTable ⇒ the
// original (stop, err) are returned, no crash.
func TestObserve_DecidePanicDegradation(t *testing.T) {
	t.Parallel()

	panickingTable := panickingTable{}
	runner := &scriptedRunner{outputs: []engine.TurnOutput{{TurnID: turn001, Text: "anything"}}}
	eng := &engine.Engine{}

	stop, err := eng.Observe(context.Background(), runner, panickingTable,
		[]session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err = %v; want nil (panic recovered)", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn (original)", stop)
	}
}

type panickingTable struct{}

func (panickingTable) MatchText(string) (string, engine.Action) { panic("simulated MatchText panic") }
func (panickingTable) MatchTool(string) (string, engine.Action) { return "", engine.ActionNothing }

// TestObserve_ReFireBudget verifies an always-matching fake ⇒ exactly
// MaxContinueInjections+1 Run calls, then stops with a budget Reason (not an
// error — the budget is a safety stop). This is the second infinite-loop bar.
func TestObserve_ReFireBudget(t *testing.T) {
	t.Parallel()

	table := seededTable()
	// Every scripted turn matches the handoff → the loop would run forever
	// without the budget.
	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: turn001, Text: implementationCompleteMsg},
	}}
	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus}
	collect := captureEvents(t, bus)

	stop, err := eng.Observe(context.Background(), runner, table, []session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err = %v; want nil (budget is not an error)", err)
	}

	wantCalls := engine.MaxContinueInjections + 1
	if got := runner.runCalls(); got != wantCalls {
		t.Errorf("runner.Run called %d times; want %d (user prompt + %d injections)",
			got, wantCalls, engine.MaxContinueInjections)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn", stop)
	}
	// The last event is the budget-cap Nothing with a "budget" signal.
	events := collect()
	if len(events) == 0 {
		t.Fatal("no EngineDecision events captured")
	}

	last := events[len(events)-1]
	if last.Signal != "budget" {
		t.Errorf("last event signal = %q; want budget", last.Signal)
	}
}

// TestObserve_CancelDrain verifies ctx cancellation after the first Run ⇒
// ("cancelled", nil) + NO further runner.Run calls (ENG-03 — the only
// off-switch drains queued injections).
func TestObserve_CancelDrain(t *testing.T) {
	t.Parallel()

	table := seededTable()
	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: turn001, Text: implementationCompleteMsg},
	}}
	eng := &engine.Engine{}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel right after the first Run returns. We wrap the runner to detect
	// the first call.
	wrapped := &cancelAfterFirst{inner: runner, cancel: cancel}

	stop, err := eng.Observe(ctx, wrapped, table, []session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err = %v; want nil", err)
	}

	if stop != "cancelled" {
		t.Errorf("stop = %q; want cancelled", stop)
	}

	if got := runner.runCalls(); got != 1 {
		t.Errorf("runner.Run called %d times; want 1 (queued injection drained)", got)
	}
}

// cancelAfterFirst cancels the context after the first Run call, then delegates.
type cancelAfterFirst struct {
	inner  *scriptedRunner
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelAfterFirst) Run(ctx context.Context, prompt []session.ContentBlock) (string, error) {
	c.once.Do(func() { c.cancel() })

	return c.inner.Run(ctx, prompt)
}

func (c *cancelAfterFirst) LastTurnOutput() engine.TurnOutput { return c.inner.LastTurnOutput() }

// TestObserve_EmitsPerTurnWithManager verifies every turn writes exactly one
// engine_decision transcript line via the EngineDecisionWriter (D-20 — the
// audit log proves unmatched-output-triggers-nothing).
func TestObserve_EmitsPerTurnWithManager(t *testing.T) {
	t.Parallel()

	table := seededTable()
	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: turn001, Text: implementationCompleteMsg},
		{TurnID: turn002, Text: resultUnmatched},
	}}
	mgr := &capturingManager{}

	eng := &engine.Engine{Manager: mgr}
	if _, err := eng.Observe(context.Background(), runner, table,
		[]session.ContentBlock{{Type: blockText, Text: "go"}}); err != nil {
		t.Fatalf("Observe err = %v", err)
	}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if len(mgr.decisions) != 2 {
		t.Fatalf("wrote %d decisions; want 2 (one per turn)", len(mgr.decisions))
	}

	if mgr.decisions[0].Action != engine.ActionContinue || mgr.decisions[0].TurnID != turn001 {
		t.Errorf("decision[0] = %+v; want continue/turn-001", mgr.decisions[0])
	}

	if mgr.decisions[1].Action != engine.ActionNothing || mgr.decisions[1].TurnID != turn002 {
		t.Errorf("decision[1] = %+v; want nothing/turn-002", mgr.decisions[1])
	}
}

// TestObserve_RealErrorStopsLoop verifies a Run returning a real error stops the
// loop (the error is surfaced, not swallowed).
func TestObserve_RealErrorStopsLoop(t *testing.T) {
	t.Parallel()

	table := seededTable()
	runner := &errorRunner{err: errors.New("provider down")}
	eng := &engine.Engine{}

	_, err := eng.Observe(context.Background(), runner, table, []session.ContentBlock{{Type: blockText, Text: "go"}})
	if err == nil || err.Error() != "provider down" {
		t.Fatalf("Observe err = %v; want provider down (surfaced)", err)
	}
}

type errorRunner struct{ err error }

func (e *errorRunner) Run(context.Context, []session.ContentBlock) (string, error) {
	return "", e.err
}
func (e *errorRunner) LastTurnOutput() engine.TurnOutput { return engine.TurnOutput{} }

// ensure unused imports are referenced (time used in cancel-after-first path).
var _ = time.Second
