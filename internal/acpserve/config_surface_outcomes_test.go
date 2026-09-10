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

	"github.com/Djarvur/ass-guard-agent/kit/modelrouting"
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

// --- 24-06 (G-24-2, WR-01): the surface demotion is provider-filtered too ---

// crossProviderOpenAI is the openai provider + minimax-m3 declaration the
// cross-provider layer fixtures share (the valid.yaml vocabulary, deep-merged
// over the embedded floor's anthropic-only table).
const crossProviderOpenAI = `providers:
  openai:
    base_url: "https://api.openai.com/v1"
    shape: openai
models:
  minimax-m3:
    provider: openai
    pricing: { input_per_mtoken: 0.10, output_per_mtoken: 0.30 }
    capabilities: { context_window: 200000, max_output_tokens: 32000, tool_calling: true, streaming: true, extended_thinking: true }
`

// crossProviderHeavyLayer declares the heavy chain with a CROSS-PROVIDER
// fallback in the middle: primary GLM-5.3 on anthropic (the serve provider),
// fallbacks [minimax-m3 on openai, glm-5.2 on anthropic] — the wrong-wire
// fixture (an unfiltered walk advertises minimax-m3, a model applyTargetLocked
// refuses to apply).
const crossProviderHeavyLayer = crossProviderOpenAI + `tiers:
  heavy:
    model: GLM-5.3
    fallback: [minimax-m3, glm-5.2]
`

// excludedPrimaryLayer binds the heavy PRIMARY on the NON-serve provider
// (minimax-m3 on openai) with the same-provider glm-5.2 fallback — the
// excluded-primary corner: the primary's breaker is never consulted and must
// never be claimed.
const excludedPrimaryLayer = crossProviderOpenAI + `tiers:
  heavy:
    model: minimax-m3
    fallback: [glm-5.2]
`

// crossProviderBreakers seeds breakers open for every named (anthropic-hosted)
// model — the multi-model variant of outcomeDemotionBreakers the
// cross-provider rows need (primary and same-provider fallback denied
// together). Recent timestamps, same as the single-model helper: the surface
// resolves at time.Now().
func crossProviderBreakers(
	t *testing.T, models ...string,
) map[modelrouting.ProviderModelKey]modelrouting.Breaker {
	t.Helper()

	store, err := modelrouting.NewOutcomeStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewOutcomeStore: %v", err)
	}

	cfg, err := modelrouting.Load() // the embedded floor's breaker thresholds
	if err != nil {
		t.Fatalf("Load floor: %v", err)
	}

	base := time.Now().UTC().Add(-time.Duration(cfg.CircuitBreaker.ConsecutiveFailures+2) * time.Second)

	for _, model := range models {
		for i := range cfg.CircuitBreaker.ConsecutiveFailures {
			aerr := store.Append(modelrouting.DispatchOutcome{
				At:       base.Add(time.Duration(i) * time.Second),
				Provider: "anthropic",
				Model:    model,
				Outcome:  modelrouting.OutcomeTransient,
				Origin:   modelrouting.OutcomeOriginTurn,
			})
			if aerr != nil {
				t.Fatalf("Append seed record %s/%d: %v", model, i, aerr)
			}
		}
	}

	records, _, rerr := modelrouting.ReadOutcomes(store.Path())
	if rerr != nil {
		t.Fatalf("ReadOutcomes: %v", rerr)
	}

	return modelrouting.ReplayBreakers(records, cfg.CircuitBreaker, nil)
}

// TestOutcomeBreakersCrossProviderDemotion (24-06, G-24-2/WR-01): the
// advertisement's demotion walk is provider-filtered — a breaker-open primary
// resolves to the first ALLOWED fallback ON THE SERVE PROVIDER, and a
// fully-denied same-provider chain keeps the primary with exactly ONE
// truthful note naming the provider constraint (the advertised value must
// always be one applyTargetLocked's provider guard could actually apply).
func TestOutcomeBreakersCrossProviderDemotion(t *testing.T) {
	t.Parallel()

	t.Run("open primary skips the allowed cross-provider candidate", func(t *testing.T) {
		t.Parallel()

		f := newSurfaceFixture(t)
		writeLayer(t, f.projectPath, crossProviderHeavyLayer)

		f.surface.SetOutcomeBreakers(crossProviderBreakers(t, testModelPrimary))

		got := optionByID(t, f.surface.Options(), optModel).CurrentValue
		if got != testModelFallback {
			t.Fatalf("model = %q; want the same-provider fallback %q (never the cross-provider minimax-m3)", got, testModelFallback)
		}

		if !strings.Contains(f.stderr.String(), testModelPrimary) || !strings.Contains(f.stderr.String(), testModelFallback) {
			t.Errorf("stderr does not name the demoted primary + replacement: %q", f.stderr.String())
		}
	})

	t.Run("same-provider chain fully denied keeps the primary with the provider-constraint note", func(t *testing.T) {
		t.Parallel()

		f := newSurfaceFixture(t)
		writeLayer(t, f.projectPath, crossProviderHeavyLayer)

		f.surface.SetOutcomeBreakers(crossProviderBreakers(t, testModelPrimary, testModelFallback))

		got := optionByID(t, f.surface.Options(), optModel).CurrentValue
		if got != testModelPrimary {
			t.Fatalf("model = %q; want the primary %q kept (the only admitted fallback is cross-provider)", got, testModelPrimary)
		}

		if !strings.Contains(f.stderr.String(), "anthropic") {
			t.Errorf("the keep-primary note must name the serve provider constraint: %q", f.stderr.String())
		}
	})
}

// TestOutcomeBreakersExcludedPrimaryCorner (24-06): a layer whose heavy-tier
// PRIMARY is the cross-provider one, with a NON-empty breaker map seeding
// ONLY the same-provider fallback's breaker open — the advertisement keeps
// the primary byte-identically with ZERO breaker notes (the primary's
// breaker was never consulted and must never be claimed). Passes at HEAD by
// design: the regression pin the provider filter must not regress.
func TestOutcomeBreakersExcludedPrimaryCorner(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, excludedPrimaryLayer)

	// NON-empty map, ONLY the fallback's breaker seeded.
	f.surface.SetOutcomeBreakers(crossProviderBreakers(t, testModelFallback))

	got := optionByID(t, f.surface.Options(), optModel).CurrentValue
	if got != "minimax-m3" {
		t.Fatalf("model = %q; want the primary minimax-m3 kept byte-identically (its breaker was never consulted)", got)
	}

	if strings.Contains(f.stderr.String(), "breaker-open") {
		t.Errorf("a breaker-open note fired for a primary whose breaker was never consulted: %q", f.stderr.String())
	}
}
