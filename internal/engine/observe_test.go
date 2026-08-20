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

var errProviderDown = errors.New("provider down")

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

func (c *capturingManager) AppendEngineDecision(
	turnID, action, signal, matchedSpan, configSource, reason string,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.decisions = append(c.decisions, engine.Decision{
		TurnID: turnID, Action: parseAction(action), Signal: signal,
		MatchedSpan: matchedSpan, ConfigSource: configSource, Reason: reason,
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

func (panickingTable) MatchText(string) engine.MatchDetail { panic("simulated MatchText panic") }
func (panickingTable) MatchTool(string) engine.MatchDetail { return engine.MatchDetail{} }

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

	_, err := eng.Observe(context.Background(), runner, table,
		[]session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
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
	runner := &errorRunner{err: errProviderDown}
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

// TestObserve_EmitsProvenance (09-02 T2 Tests 7-9, AUD-04): a matching turn
// publishes an EngineDecision event + transcript line carrying MatchedSpan +
// ConfigSource; an unmatched turn still writes exactly ONE nothing-decision
// with EMPTY provenance (the Phase-4 cadence invariant — enrich fields, never
// cadence).
func TestObserve_EmitsProvenance(t *testing.T) {
	t.Parallel()

	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: "p1", Text: "The change is ready to implement now."},
	}}
	bus := event.NewBus()
	capturing := &capturingManager{}
	eng := &engine.Engine{Bus: bus, Manager: capturing}

	events := captureEvents(t, bus)

	_, err := eng.Observe(context.Background(), runner, spanTable{},
		[]session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	got := events()

	if len(got) == 0 {
		t.Fatal("no EngineDecision event — the provenance leg is not wired")
	}

	ed := got[0]

	if ed.Action != "continue" || ed.Signal != signalImplComplete {
		t.Errorf("event action/signal = %q/%q; want continue/text:impl-complete", ed.Action, ed.Signal)
	}

	if ed.MatchedSpan != "ready to implement" {
		t.Errorf("event MatchedSpan = %q; want the matched span", ed.MatchedSpan)
	}

	if ed.ConfigSource != "openspec.toml patterns/impl-complete" {
		t.Errorf("event ConfigSource = %q; want the entry source", ed.ConfigSource)
	}

	if len(capturing.decisions) == 0 {
		t.Fatal("no decision written to the transcript seam")
	}

	// Unmatched turn: exactly one nothing-decision, empty provenance.
	runner2 := &scriptedRunner{outputs: []engine.TurnOutput{{TurnID: "p2", Text: "plain"}}}
	capturing2 := &capturingManager{}
	eng2 := &engine.Engine{Manager: capturing2}

	_, err = eng2.Observe(context.Background(), runner2, spanTable{},
		[]session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe(unmatched): %v", err)
	}

	if len(capturing2.decisions) != 1 {
		t.Fatalf("nothing-turn decisions = %d; want exactly 1 (cadence invariant)", len(capturing2.decisions))
	}

	d := capturing2.decisions[0]
	if d.Action != engine.ActionNothing || d.Signal != resultUnmatched || d.MatchedSpan != "" || d.ConfigSource != "" {
		t.Errorf("nothing decision = %+v; want unmatched with empty provenance", d)
	}
}

// --- 13-00 T3: the engine ask-wait (Observe stops exiting on stopAsk) ---

// askWaitRunner is a scripted runner whose Run returns per-call stop reasons
// (ask markers included) and whose LastTurnOutput slot for an ASKING turn
// flips ONE-SHOT to the fixed completed output once the settle channel closes
// — mirroring the real adapter's terminal-line precedence (a later
// assistant_message outranks ask_suspended exactly when the resume completed,
// and only for THAT suspension). AskSettle exposes the channel (nil = never
// settles).
type askWaitRunner struct {
	mu      sync.Mutex
	stops   []string
	before  []engine.TurnOutput
	after   engine.TurnOutput
	applied []bool // per-slot: the settle flip fired for this asking turn

	calls   atomic.Int32
	settle  chan struct{}
	settles atomic.Int32
}

func (a *askWaitRunner) Run(_ context.Context, _ []session.ContentBlock) (string, error) {
	a.calls.Add(1)

	a.mu.Lock()
	defer a.mu.Unlock()

	idx := int(a.calls.Load()) - 1
	if idx >= len(a.stops) {
		idx = len(a.stops) - 1
	}

	stop := stopEndTurn
	if idx >= 0 && a.stops[idx] != "" {
		stop = a.stops[idx]
	}

	return stop, nil
}

func (a *askWaitRunner) LastTurnOutput() engine.TurnOutput {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx := int(a.calls.Load()) - 1
	if idx < 0 {
		return engine.TurnOutput{}
	}

	if idx >= len(a.before) {
		idx = len(a.before) - 1
	}

	if a.applied == nil {
		a.applied = make([]bool, len(a.before))
	}

	// The suspended slot flips to the completed output once settle fired —
	// exactly once per suspension (the resume is one-shot).
	select {
	case <-a.settle:
		if a.stops[idx] == engine.StopAsk && !a.applied[idx] {
			a.before[idx] = a.after
			a.applied[idx] = true
		}
	default:
	}

	return a.before[idx]
}

// AskSettle implements the engine's optional AskSettler capability.
func (a *askWaitRunner) AskSettle() <-chan struct{} {
	a.settles.Add(1)

	return a.settle
}

func (a *askWaitRunner) runCalls() int { return int(a.calls.Load()) }

// askNoSettleRunner is the same scripted shape WITHOUT the AskSettler
// capability (a distinct type — embedding would promote the method back).
type askNoSettleRunner struct {
	mu      sync.Mutex
	stops   []string
	outputs []engine.TurnOutput
	calls   atomic.Int32
}

func (a *askNoSettleRunner) Run(_ context.Context, _ []session.ContentBlock) (string, error) {
	a.calls.Add(1)

	a.mu.Lock()
	defer a.mu.Unlock()

	idx := int(a.calls.Load()) - 1
	if idx >= len(a.stops) {
		idx = len(a.stops) - 1
	}

	stop := stopEndTurn
	if idx >= 0 && a.stops[idx] != "" {
		stop = a.stops[idx]
	}

	return stop, nil
}

func (a *askNoSettleRunner) LastTurnOutput() engine.TurnOutput {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx := int(a.calls.Load()) - 1
	if idx < 0 {
		return engine.TurnOutput{}
	}

	if idx >= len(a.outputs) {
		idx = len(a.outputs) - 1
	}

	return a.outputs[idx]
}

func (a *askNoSettleRunner) runCalls() int { return int(a.calls.Load()) }

// TestObserve_AskWait_InjectedTurnDecidesAfterSettle (13-00 T3 test 1): an
// INJECTED turn that suspends (stopAsk) WAITS for the settle signal; once the
// resumed turn completes, Observe RE-READS it, emits a continue decision for
// the asking turn, and performs the next injection. Today (RED): Observe exits
// at the post-injection `stop != "end_turn"` check with NO decision — the
// detached-resume blocker at the engine level.
func TestObserve_AskWait_InjectedTurnDecidesAfterSettle(t *testing.T) {
	t.Parallel()

	settle := make(chan struct{})

	runner := &askWaitRunner{
		stops: []string{stopEndTurn, engine.StopAsk, stopEndTurn},
		before: []engine.TurnOutput{
			{TurnID: turn001, Text: implementationCompleteMsg}, // user turn → continue
			{TurnID: turn002, AskSuspended: true},              // injected turn asks
			{TurnID: turn003, Text: resultUnmatched},           // final injection ends
		},
		after:  engine.TurnOutput{TurnID: turn002, Text: implementationCompleteMsg}, // resumed turn002
		settle: settle,
	}

	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus}
	collect := captureEvents(t, bus)

	// The resume completes 50ms in: the engine's wait must span it.
	time.AfterFunc(50*time.Millisecond, func() { close(settle) })

	stop, err := eng.Observe(context.Background(), runner, seededTable(),
		[]session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err: %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn (the chain ran to its natural terminus)", stop)
	}

	if got := runner.runCalls(); got != 3 {
		t.Errorf("runner.Run called %d times; want 3 (user + ask-suspended injection + settled next injection)", got)
	}

	events := collect()

	contForAsking := false

	for _, ev := range events {
		if ev.TurnID == turn002 && ev.Action == engine.ActionContinue.String() {
			contForAsking = true
		}
	}

	if !contForAsking {
		t.Errorf("no continue decision for the ASKING turn %q after settle (events: %+v) — "+
			"the completed turn never fed Decide (the engine-level blocker, pinned)", turn002, summarizeEvents(events))
	}
}

// TestObserve_AskWait_FirstTurnDecidesAfterSettle (13-00 T3 test 2): a FIRST
// (user) turn that suspends then settles takes the same normal-decide path —
// the completed turn's decision is a continue with an injection. Today (RED):
// ActionAsk on the unresolved suspended output.
func TestObserve_AskWait_FirstTurnDecidesAfterSettle(t *testing.T) {
	t.Parallel()

	settle := make(chan struct{})

	runner := &askWaitRunner{
		stops:  []string{engine.StopAsk, stopEndTurn},
		before: []engine.TurnOutput{{TurnID: turn001, AskSuspended: true}, {TurnID: turn002, Text: resultUnmatched}},
		after:  engine.TurnOutput{TurnID: turn001, Text: implementationCompleteMsg},
		settle: settle,
	}

	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus}
	collect := captureEvents(t, bus)

	time.AfterFunc(50*time.Millisecond, func() { close(settle) })

	stop, err := eng.Observe(context.Background(), runner, seededTable(),
		[]session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err: %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn", stop)
	}

	if got := runner.runCalls(); got != 2 {
		t.Errorf("runner.Run called %d times; want 2 (the suspended user turn + the settled injection)", got)
	}

	events := collect()

	// The suspension surfaces its ask decision FIRST (the 12-01 audit line),
	// then the settled turn gets its NORMAL continue decision.
	if len(events) < 2 {
		t.Fatalf("decisions = %d; want >= 2 (the ask surface + the continue for the settled turn)", len(events))
	}

	if events[0].TurnID != turn001 || events[0].Signal != engine.SignalAskSuspended {
		t.Errorf("first decision = %+v; want the ask-suspension surface for %q", events[0], turn001)
	}

	if events[1].TurnID != turn001 || events[1].Action != engine.ActionContinue.String() {
		t.Errorf("second decision = %+v; want continue for the settled user turn %q", events[1], turn001)
	}
}

// TestObserve_AskWait_CancelDuringWaitDrains (13-00 T3 test 3a): ctx death
// MID-WAIT ⇒ cancel-drain semantics — ("cancelled", nil), no decision, no
// injection (D-03 extended to the ask-wait).
func TestObserve_AskWait_CancelDuringWaitDrains(t *testing.T) {
	t.Parallel()

	runner := &askWaitRunner{
		stops:  []string{engine.StopAsk},
		before: []engine.TurnOutput{{TurnID: turn001, AskSuspended: true}},
		settle: make(chan struct{}), // non-nil, NEVER closes — the ctx is the only exit
	}

	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus}
	collect := captureEvents(t, bus)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	time.AfterFunc(50*time.Millisecond, cancel)

	stop, err := eng.Observe(ctx, runner, seededTable(),
		[]session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err: %v; want nil (cancel-drain)", err)
	}

	if stop != "cancelled" {
		t.Errorf("stop = %q; want cancelled (ctx death mid-wait drains)", stop)
	}

	if got := runner.runCalls(); got != 1 {
		t.Errorf("runner.Run called %d times; want 1 (no injection after the drain)", got)
	}

	// The ask-suspension decision MAY have surfaced before the drain (it is
	// true + harmless); the pin is that NO completion decision and NO
	// injection followed the cancelled wait.
	for _, ev := range collect() {
		if ev.Action == engine.ActionContinue.String() {
			t.Errorf("a continue decision fired during the drain: %+v (the cancelled chain must hold)", ev)
		}
	}
}

// TestObserve_AskWait_UnresolvedKeepsTodaySemantics (13-00 T3 test 3b): a
// settler that never settles with a LIVE ctx keeps today's semantics exactly —
// the FIRST turn decides ActionAsk on the suspended output (no table lookup);
// the INJECTED turn exits WITHOUT deciding.
func TestObserve_AskWait_UnresolvedKeepsTodaySemantics(t *testing.T) { //nolint:funlen // two-path semantics battery
	t.Parallel()

	t.Run("first turn unresolved", func(t *testing.T) {
		t.Parallel()

		runner := &askWaitRunner{
			stops:  []string{engine.StopAsk},
			before: []engine.TurnOutput{{TurnID: turn001, AskSuspended: true}},
			settle: nil,
		}

		bus := event.NewBus()
		eng := &engine.Engine{Bus: bus}
		collect := captureEvents(t, bus)

		stop, err := eng.Observe(context.Background(), runner, seededTable(),
			[]session.ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Observe err: %v", err)
		}

		if stop != engine.StopAsk {
			t.Errorf("stop = %q; want the ask marker returned unchanged", stop)
		}

		if got := runner.runCalls(); got != 1 {
			t.Errorf("runner.Run called %d times; want 1 (a suspended turn never chains)", got)
		}

		events := collect()
		if len(events) != 1 {
			t.Fatalf("decisions = %d; want exactly the ask decision (events: %+v)",
				len(events), summarizeEvents(events))
		}

		// The nil-dispatcher engine degrades ActionAsk to nothing; the PIN is
		// the ask:suspended signal with NO table lookup — never a continue.
		if events[0].Signal != engine.SignalAskSuspended || events[0].Action == engine.ActionContinue.String() {
			t.Errorf("decision = %+v; want the %s decision, never a continue (no table lookup)",
				events[0], engine.SignalAskSuspended)
		}
	})

	t.Run("injected turn unresolved exits silently", func(t *testing.T) {
		t.Parallel()

		runner := &askWaitRunner{
			stops: []string{stopEndTurn, engine.StopAsk},
			before: []engine.TurnOutput{
				{TurnID: turn001, Text: implementationCompleteMsg},
				{TurnID: turn002, AskSuspended: true},
			},
			settle: nil,
		}

		bus := event.NewBus()
		eng := &engine.Engine{Bus: bus}
		collect := captureEvents(t, bus)

		stop, err := eng.Observe(context.Background(), runner, seededTable(),
			[]session.ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Observe err: %v", err)
		}

		if stop != engine.StopAsk {
			t.Errorf("stop = %q; want the ask marker returned unchanged", stop)
		}

		if got := runner.runCalls(); got != 2 {
			t.Errorf("runner.Run called %d times; want 2 (user + the one injection; nothing after)", got)
		}

		// Exactly ONE decision (the user turn's continue); the asking turn
		// exits WITHOUT a decision — today's post-injection behavior.
		events := collect()
		if len(events) != 1 {
			t.Fatalf("decisions = %+v; want exactly 1 (the user turn's continue — the ask exits silently)",
				summarizeEvents(events))
		}
	})
}

// TestObserve_AskWait_NoCapabilityByteIdentical (13-00 T3 test 4): a runner
// WITHOUT the AskSettler capability keeps today's behavior byte-identical on
// both paths (first turn → ActionAsk decision; injection → exit without
// deciding).
func TestObserve_AskWait_NoCapabilityByteIdentical(t *testing.T) { //nolint:funlen // two-path byte-identity battery
	t.Parallel()

	t.Run("first turn", func(t *testing.T) {
		t.Parallel()

		runner := &askNoSettleRunner{
			stops:   []string{engine.StopAsk},
			outputs: []engine.TurnOutput{{TurnID: turn001, AskSuspended: true}},
		}

		bus := event.NewBus()
		eng := &engine.Engine{Bus: bus}
		collect := captureEvents(t, bus)

		stop, err := eng.Observe(context.Background(), runner, seededTable(),
			[]session.ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Observe err: %v", err)
		}

		if stop != engine.StopAsk {
			t.Errorf("stop = %q; want the ask marker unchanged", stop)
		}

		if got := runner.runCalls(); got != 1 {
			t.Errorf("runner.Run called %d times; want 1 (a suspended turn never chains)", got)
		}

		events := collect()
		if len(events) != 1 || events[0].Signal != engine.SignalAskSuspended ||
			events[0].Action == engine.ActionContinue.String() {
			t.Errorf("decisions = %+v; want the ask-suspended decision, never a continue "+
				"(today's first-turn semantics)",
				summarizeEvents(events))
		}
	})

	t.Run("injection", func(t *testing.T) {
		t.Parallel()

		runner := &askNoSettleRunner{
			stops: []string{stopEndTurn, engine.StopAsk},
			outputs: []engine.TurnOutput{
				{TurnID: turn001, Text: implementationCompleteMsg},
				{TurnID: turn002, AskSuspended: true},
			},
		}

		bus := event.NewBus()
		eng := &engine.Engine{Bus: bus}
		collect := captureEvents(t, bus)

		stop, err := eng.Observe(context.Background(), runner, seededTable(),
			[]session.ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Observe err: %v", err)
		}

		if stop != engine.StopAsk {
			t.Errorf("stop = %q; want the ask marker unchanged", stop)
		}

		if events := collect(); len(events) != 1 {
			t.Errorf("decisions = %+v; want exactly 1 (today's silent exit)", summarizeEvents(events))
		}
	})
}

// TestObserve_AskWait_WaitsConsumeNoBudget (13-00 T3 test 5): a chain of
// ask→settle→ask→settle… counts INJECTIONS only against the re-fire budget —
// waits are free. The cap still bounds the chain (no infinite ask-settle
// loop).
func TestObserve_AskWait_WaitsConsumeNoBudget(t *testing.T) {
	t.Parallel()

	settle := make(chan struct{})

	runner := &askWaitRunner{
		stops:  []string{engine.StopAsk, engine.StopAsk, engine.StopAsk},
		before: []engine.TurnOutput{{TurnID: turn001, AskSuspended: true}},
		after:  engine.TurnOutput{TurnID: turn001, Text: implementationCompleteMsg}, // every settle → continue
		settle: settle,
	}

	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus}
	collect := captureEvents(t, bus)

	// Keep the settle channel cycling: close-and-replace is not observable by
	// a closed channel, so leave it OPEN-closed from 25ms — every wait after
	// that sees it closed (settled instantly).
	time.AfterFunc(25*time.Millisecond, func() { close(settle) })

	stop, err := eng.Observe(context.Background(), runner, seededTable(),
		[]session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Observe err: %v", err)
	}

	_ = stop

	// The budget caps at MaxContinueInjections injections; the user prompt is
	// the +1. The waits never counted.
	if got := runner.runCalls(); got != engine.MaxContinueInjections+1 {
		t.Errorf("runner.Run called %d times; want %d (waits consumed no budget — injections only)",
			got, engine.MaxContinueInjections+1)
	}

	events := collect()

	last := events[len(events)-1]
	if last.Signal != "budget" {
		t.Errorf("last signal = %q; want budget (the cap, not the waits, stopped the chain)", last.Signal)
	}
}

// summarizeEvents renders the captured decisions compactly for failure
// messages.
func summarizeEvents(events []event.EngineDecision) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.TurnID+"/"+ev.Action+"/"+ev.Signal)
	}

	return out
}
