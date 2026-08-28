package runtime //nolint:testpackage // internal package test (accesses unexported fields)

// 16-REVIEW WR-06 regression pin: the automation firing target
// (currentSessionID) is the most-recently-ACTIVE session, not the
// most-recently-CREATED one. The only pre-fix writer was sessionFor at
// CREATION, so with two sessions on one connection every due automation fired
// into the newest session even while the operator was actively driving the
// older one — turns (and their tool side effects) landed in a session nobody
// was watching. The fix updates the marker on EVERY sessionFor resolve
// (Run's and the automation firing's turn-start step).

import (
	"context"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

func TestCurrentSessionIDFollowsActivity(t *testing.T) {
	t.Parallel()

	runner := &Runner{
		bus:     event.NewBus(),
		profile: profile.Profile{Name: testProfileName, Model: testModelBefore},
		workDir: t.TempDir(),
		maxConc: 2,
		makeProvider: func(provider.RequestCapturer) provider.Provider {
			return newGatedStreamProvider()
		},
	}

	ctx := context.Background()

	_ = runner.sessionFor(ctx, "s-1") // created — the target
	_ = runner.sessionFor(ctx, "s-2") // created — the target moves

	if got := runner.currentSessionID(); got != "s-2" {
		t.Fatalf("current firing target = %q; want the newest session %q", got, "s-2")
	}

	// The operator re-prompts the OLDER session: the firing target must follow
	// the ACTIVITY. Pre-fix the marker stayed on s-2 and every due automation
	// fired into the unwatched session.
	_ = runner.sessionFor(ctx, "s-1")

	if got := runner.currentSessionID(); got != "s-1" {
		t.Errorf("current firing target = %q; want %q — the marker tracks most-recently-ACTIVE, "+
			"not most-recently-created (WR-06)", got, "s-1")
	}
}
