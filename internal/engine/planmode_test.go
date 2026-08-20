package engine_test // engine-level provenance test (12-04 T1 Test 3)

import (
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
)

// TestDecide_PlanModeProvenanceOnly (12-04 T1 Test 3): a turn that ended with
// plan mode ON carries the state in its decision REASON (audit provenance) —
// and NOTHING ELSE keys on it: the action/signal are exactly what the same
// turn without the flag would produce (the gate lives at the tool-exec layer,
// not the engine; no new chaining behavior).
func TestDecide_PlanModeProvenanceOnly(t *testing.T) {
	t.Parallel()

	base := engine.TurnOutput{
		TurnID: "sess-pm-turn-001",
		Text:   "I explored the codebase and have a plan ready",
	}
	on := base
	on.PlanMode = true

	decOff := engine.Decide(base, fakeTable{})
	decOn := engine.Decide(on, fakeTable{})

	if decOn.Action != decOff.Action || decOn.Signal != decOff.Signal {
		t.Errorf("plan mode changed the decision: on=%+v off=%+v (provenance ONLY)", decOn, decOff)
	}

	if !strings.Contains(decOn.Reason, "plan-mode ON") {
		t.Errorf("reason = %q; want the plan-mode provenance note", decOn.Reason)
	}

	if strings.Contains(decOff.Reason, "plan-mode") {
		t.Errorf("off-state reason = %q; must not carry the note", decOff.Reason)
	}
}
