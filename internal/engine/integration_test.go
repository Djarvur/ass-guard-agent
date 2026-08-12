package engine_test

import (
	"context"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/redact"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// testRedactor adapts internal/redact to session.Redactor for the integration
// test (the real Manager scrubs each transcript line — LOG-03).
type testRedactor struct{}

func (testRedactor) Redact(b []byte) ([]byte, error) { return redact.Redact(b) }
func (testRedactor) ScrubError(err error) string     { return redact.ScrubError(err) }

// newIntegrationManager opens a real session.Manager against a temp dir so the
// engine_decision audit lines are real artifacts (D-20), not fakes.
func newIntegrationManager(t *testing.T, sessionID string) *session.Manager {
	t.Helper()

	m, err := session.NewManager(t.TempDir(), sessionID, testRedactor{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	t.Cleanup(func() { _ = m.Close() })

	return m
}

// TestIntegration_TextSignalZeroContinue is the TRACER assertion: a fake
// provider returns a turn whose assistant text matches the seeded handoff
// literal → the engine decides continue → injects "continue" → the fake second
// turn returns end_turn unmatched → loop exits. Asserts 2 Run calls, the 2nd
// prompt is the continue-injection, ("end_turn", nil) returned, 2 EngineDecision
// events (continue then nothing), and 2 REAL engine_decision transcript lines.
func TestIntegration_TextSignalZeroContinue(t *testing.T) {
	t.Parallel()

	table := seededTable()
	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: "int-text-1", Text: "done. ## Implementation Complete — ready for review"},
		{TurnID: "int-text-2", Text: "final answer, nothing matches here"},
	}}
	mgr := newIntegrationManager(t, "sess-int-text")
	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus, Manager: mgr}
	collect := captureEvents(t, bus)

	stop, err := eng.Observe(context.Background(), runner, table, []session.ContentBlock{{Type: blockText, Text: "implement the spec"}})
	if err != nil {
		t.Fatalf("Observe err = %v; want nil", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn", stop)
	}

	if got := runner.runCalls(); got != 2 {
		t.Errorf("Run calls = %d; want 2", got)
	}

	runner.mu.Lock()
	secondPrompt := runner.prompts[1]
	runner.mu.Unlock()

	if len(secondPrompt) != 1 || secondPrompt[0].Text != stopContinue {
		t.Errorf("2nd prompt = %+v; want the continue-injection", secondPrompt)
	}

	events := collect()
	if len(events) != 2 {
		t.Fatalf("events = %d; want 2 (continue then nothing)", len(events))
	}

	if events[0].Action != stopContinue || events[1].Action != fixtureNothing {
		t.Errorf("event actions = %q,%q; want continue,nothing", events[0].Action, events[1].Action)
	}
	// REAL audit-log assertion: the transcript carries the two engine_decision
	// lines (D-20 — the audit log proves the structural-safety property).
	lines, err := mgr.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var engineLines int

	for _, l := range lines {
		if l.Type == session.TypeEngineDecision {
			engineLines++
		}
	}

	if engineLines != 2 {
		t.Errorf("engine_decision transcript lines = %d; want 2 (real audit artifacts)", engineLines)
	}
}

// TestIntegration_UnmatchedStructuralSafety: an unmatched user prompt produces
// ONE Run call, ZERO continue-injections, ONE EngineDecision{nothing} event, and
// ("end_turn", nil) returned. End-to-end structural safety.
func TestIntegration_UnmatchedStructuralSafety(t *testing.T) {
	t.Parallel()

	table := seededTable()
	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: "int-none-1", Text: "the agent did something with no handoff signal"},
	}}
	mgr := newIntegrationManager(t, "sess-int-none")
	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus, Manager: mgr}
	collect := captureEvents(t, bus)

	stop, err := eng.Observe(context.Background(), runner, table, []session.ContentBlock{{Type: blockText, Text: "hello"}})
	if err != nil || stop != stopEndTurn {
		t.Fatalf("Observe = (%q, %v); want (end_turn, nil)", stop, err)
	}

	if got := runner.runCalls(); got != 1 {
		t.Errorf("Run calls = %d; want 1 (zero injections)", got)
	}

	events := collect()
	if len(events) != 1 || events[0].Action != fixtureNothing {
		t.Fatalf("events = %+v; want exactly one {nothing}", events)
	}
}

// TestIntegration_ToolSignalContinue: the fake first turn carries a handoff
// TOOL-call (not text) → the engine continues on the tool signal (D-02 second
// signal). Asserts 2 Run calls + a continue-then-nothing event pair.
func TestIntegration_ToolSignalContinue(t *testing.T) {
	t.Parallel()

	table := seededTable()
	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: "int-tool-1", Text: "advancing via tool", ToolCalls: []string{toolRead, openSpecHandoffKind}},
		{TurnID: "int-tool-2", Text: "completed, no further signal"},
	}}
	mgr := newIntegrationManager(t, "sess-int-tool")
	bus := event.NewBus()
	eng := &engine.Engine{Bus: bus, Manager: mgr}
	collect := captureEvents(t, bus)

	stop, err := eng.Observe(context.Background(), runner, table, []session.ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil || stop != stopEndTurn {
		t.Fatalf("Observe = (%q, %v); want (end_turn, nil)", stop, err)
	}

	if got := runner.runCalls(); got != 2 {
		t.Errorf("Run calls = %d; want 2 (continue on tool signal)", got)
	}

	events := collect()
	if len(events) != 2 {
		t.Fatalf("events = %d; want 2", len(events))
	}

	if events[0].Action != stopContinue || !startsWith(events[0].Signal, "tool:") {
		t.Errorf("event[0] = %+v; want continue with tool:* signal", events[0])
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
