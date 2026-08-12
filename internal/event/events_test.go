package event_test

import (
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/event"
)

// TestEngineDecisionKind verifies the Phase-4 EngineDecision event reports its
// kind discriminator and carries the provenance-tagged verdict fields (ENG-02).
func TestEngineDecisionKind(t *testing.T) {
	t.Parallel()

	e := event.EngineDecision{TurnID: "t1", Action: "continue", Signal: "text:impl-complete", Reason: "matched"}
	if got := e.Kind(); got != "EngineDecision" {
		t.Errorf("EngineDecision.Kind() = %q; want EngineDecision", got)
	}
}

// TestHookProgressKind verifies the Phase-4 HookProgress event reports its kind
// discriminator (consumed by internal/hookdag in Plan 04-03).
func TestHookProgressKind(t *testing.T) {
	t.Parallel()

	e := event.HookProgress{
		TurnID: "t1", HookName: "post-implement",
		StepIndex: 0, StepKind: "run-command", Status: "start",
	}
	if got := e.Kind(); got != "HookProgress" {
		t.Errorf("HookProgress.Kind() = %q; want HookProgress", got)
	}
}

// TestPhase4EventsImplementInterface is a compile-time assertion that both new
// Phase-4 event types satisfy the event.Event interface (Kind() string).
func TestPhase4EventsImplementInterface(t *testing.T) {
	t.Parallel()

	var (
		_ event.Event = event.EngineDecision{}
		_ event.Event = event.HookProgress{}
	)
	// Value-receiver kinds: a zero value reports its kind (guards against an
	// accidental pointer-receiver change that would break the bus fan-out).
	dec := event.EngineDecision{}
	hook := event.HookProgress{}

	if dec.Kind() != "EngineDecision" {
		t.Fatalf("zero EngineDecision.Kind() = %q; want EngineDecision", dec.Kind())
	}

	if hook.Kind() != "HookProgress" {
		t.Fatalf("zero HookProgress.Kind() = %q; want HookProgress", hook.Kind())
	}
}

// TestPhase4BufferConstsPositive verifies the new Buf* consts are positive so a
// Subscribe call never hands back a nil (unbuffered-but-valid) channel by
// accident — the engine/hook publisher relies on a real buffer for backpressure.
func TestPhase4BufferConstsPositive(t *testing.T) {
	t.Parallel()

	if event.BufEngineDecision <= 0 {
		t.Errorf("BufEngineDecision = %d; want > 0", event.BufEngineDecision)
	}

	if event.BufHookProgress <= 0 {
		t.Errorf("BufHookProgress = %d; want > 0", event.BufHookProgress)
	}
}
