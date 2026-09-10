package acpserve //nolint:testpackage // internal package test

// 24-02 (TAIL-01, D-06 observable): the replayed outcome-store breakers bend
// the LIVE config-surface model resolution. A seeded store with enough
// transient evidence for the resolved primary's (provider, model) opens the
// replayed breaker; resolveModelLocked then returns the first ALLOWED
// fallback's model — the advertisement reflects it, and ONE loud stderr note
// names the demoted primary and its replacement. An EMPTY breaker map is
// byte-identical to the pre-change behavior: no demotion, no note.

import (
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
)

// outcomeDemotionFixture seeds a real outcome store (24-01's append-only
// JSONL) with enough consecutive transient records to OPEN the primary's
// replayed breaker under the embedded floor's thresholds, then replays the
// store through modelrouting.ReplayBreakers — exactly the composition
// acp_serve.Run wires through SetOutcomeBreakers.
func outcomeDemotionBreakers(t *testing.T, model string) map[modelrouting.ProviderModelKey]modelrouting.Breaker {
	t.Helper()

	store, err := modelrouting.NewOutcomeStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewOutcomeStore: %v", err)
	}

	cfg, err := modelrouting.Load() // the embedded floor's breaker thresholds
	if err != nil {
		t.Fatalf("Load floor: %v", err)
	}

	// Seed with RECENT timestamps: the surface resolves at time.Now(), and a
	// breaker only denies while its cooldown has NOT elapsed since the last
	// transient — records stamped minutes ago would admit the half-open probe.
	base := time.Now().UTC().Add(-time.Duration(cfg.CircuitBreaker.ConsecutiveFailures+2) * time.Second)

	for i := range cfg.CircuitBreaker.ConsecutiveFailures {
		aerr := store.Append(modelrouting.DispatchOutcome{
			At:       base.Add(time.Duration(i) * time.Second),
			Provider: "anthropic",
			Model:    model,
			Outcome:  modelrouting.OutcomeTransient,
			Origin:   modelrouting.OutcomeOriginTurn,
		})
		if aerr != nil {
			t.Fatalf("Append seed record %d: %v", i, aerr)
		}
	}

	records, skipped, rerr := modelrouting.ReadOutcomes(store.Path())
	if rerr != nil {
		t.Fatalf("ReadOutcomes: %v", rerr)
	}

	if skipped != 0 {
		t.Fatalf("skipped = %d; want 0", skipped)
	}

	return modelrouting.ReplayBreakers(records, cfg.CircuitBreaker, nil)
}

// TestOutcomeBreakersDemoteSessionModel: with the primary's replayed breaker
// OPEN, the advertised session model resolves to the first allowed fallback —
// routing decisions provably change after enough evidence accumulates (D-06).
func TestOutcomeBreakersDemoteSessionModel(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	// Sanity: before evidence, the floor resolves the heavy primary.
	if got := optionByID(t, f.surface.Options(), optModel).CurrentValue; got != testModelPrimary {
		t.Fatalf("pre-seed model = %q; want the primary %q", got, testModelPrimary)
	}

	f.surface.SetOutcomeBreakers(outcomeDemotionBreakers(t, testModelPrimary))

	got := optionByID(t, f.surface.Options(), optModel).CurrentValue
	if got != testModelFallback {
		t.Fatalf("post-seed model = %q; want the first allowed fallback %q (breaker-open primary demoted)", got, testModelFallback)
	}

	if !strings.Contains(f.stderr.String(), testModelPrimary) || !strings.Contains(f.stderr.String(), testModelFallback) {
		t.Errorf("stderr does not name the demoted primary + replacement: %q", f.stderr.String())
	}

	if !strings.Contains(f.stderr.String(), "breaker") {
		t.Errorf("the demotion note is missing the breaker cause: %q", f.stderr.String())
	}
}

// TestOutcomeBreakersEmptyMapKeepsPrimary: an EMPTY breaker map (no store, no
// evidence) leaves resolution byte-identical to the pre-change behavior — the
// primary stays, and no demotion note fires.
func TestOutcomeBreakersEmptyMapKeepsPrimary(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	f.surface.SetOutcomeBreakers(map[modelrouting.ProviderModelKey]modelrouting.Breaker{})

	got := optionByID(t, f.surface.Options(), optModel).CurrentValue
	if got != testModelPrimary {
		t.Fatalf("model = %q; want the primary %q (empty evidence never demotes)", got, testModelPrimary)
	}

	if strings.Contains(f.stderr.String(), "breaker") {
		t.Errorf("demotion note fired with an empty breaker map: %q", f.stderr.String())
	}
}
