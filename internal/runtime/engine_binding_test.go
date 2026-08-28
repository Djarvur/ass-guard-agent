package runtime //nolint:testpackage // internal package test (accesses unexported fields)

// 16-REVIEW WR-01 regression pin: the engine's per-turn wiring is
// PER-INVOCATION. The old runOneTurn rebound SHARED mutable state —
// r.hookExec.Commands/Turns/Boundaries and r.eng.Manager — to the current
// session on every engine-enabled turn. Client turns serialize per session,
// but two DIFFERENT sessions turn concurrently, and a parked 13-00 chain
// resumes without holding any of those mutexes after a later session's
// rebind: hook DAGs and engine decisions then landed in the wrong session's
// transcript. The fix builds a session-bound executor + dispatcher + engine
// per turn; this test pins the structural property that the shared templates
// never carry session state.

import (
	"context"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
)

// TestEngineWiringStaysSessionFree drives engine-enabled turns for two
// sessions and asserts the shared wiring is untouched: no Manager on the
// Engine template, no session-bound seams on the hook executor template. A
// rebinding regression fails here on the very first turn.
func TestEngineWiringStaysSessionFree(t *testing.T) {
	t.Parallel()

	gated := newGatedStreamProvider()
	runner := newModelTestRunner(t, gated, testModelBefore)

	serr := runner.SetupEngine()
	if serr != nil {
		t.Fatalf("SetupEngine: %v", serr)
	}

	ctx := context.Background()

	// Session A turns through the engine path (the gated provider holds only
	// the FIRST stream open).
	turnDone := startTestTurn(t, runner, "s-bind-a", ctx)

	awaitSeenModel(t, gated, testModelBefore)

	close(gated.releaseFirst)
	<-turnDone

	// Session B turns through the engine path — under the old code THIS is the
	// rebind that stole session A's parked wiring.
	_, _ = runner.Run(ctx, "s-bind-b", nopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "plain text, no invocation"}})

	// Structural pin: the SHARED wiring never carries session state.
	if runner.eng.Manager != nil {
		t.Error("the shared Engine template carries a session Manager — " +
			"per-turn decisions would land in the last-turning session's transcript (WR-01)")
	}

	if runner.hookExec.Turns != nil || runner.hookExec.Boundaries != nil || runner.hookExec.Commands != nil {
		t.Error("the shared hook executor's seams were rebound to a session — " +
			"a parked chain resuming after another session's turn would run against the wrong session (WR-01)")
	}
}
