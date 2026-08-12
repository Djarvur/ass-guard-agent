package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// newSafetyScheduler builds a Scheduler over valid.yaml with REAL breakers +
// cost tracker installed (via InstallSafety), plus the fake provider on every
// declared provider key.
func newSafetyScheduler(t *testing.T, fp *fakeProvider) (*Scheduler, *event.Bus, *Config) {
	t.Helper()
	cfg := loadValid(t)
	bus := event.NewBus()

	t.Cleanup(func() { bus.Close() })

	providers := map[string]provider.Provider{
		providerAnthropic: fp, providerOpenAI: fp, providerGroq: fp,
	}
	s := NewScheduler(cfg, bus, nil, providers, nil)
	s.InstallSafety(cfg, bus)
	s.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) }) // Sunday noon → global table

	return s, bus, cfg
}

// TestSafetyBreakerSkipsOpenCandidate: after the primary's breaker is tripped
// (5 consecutive Transients), a Dispatch SKIPS the primary (Allow=false, logged)
// and goes straight to a fallback candidate that succeeds.
func TestSafetyBreakerSkipsOpenCandidate(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().
		set(modelGLM52, fakeOutcome{err: transientErr(modelGLM52, 429)}).
		set(modelMinimaxM3, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s, bus, cfg := newSafetyScheduler(t, fp)
	_ = cfg

	// Trip the (anthropic, glm-5.2) breaker directly via 5 transient records.
	cb := s.Breaker(providerAnthropic, modelGLM52)
	require.NotNil(t, cb, "heavy primary breaker must be registered")

	now := s.now()
	for range 5 {
		cb.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
	}

	require.Equal(t, breakerOpen, cb.State(), "breaker tripped")

	// Reset the fake call log so we observe only this Dispatch's calls.
	fp.mu.Lock()
	fp.calls = nil
	fp.mu.Unlock()

	ch := bus.Subscribe("ProviderFallback", 8)
	resp, err := s.Dispatch(context.Background(), tierHeavy, "myproj", CapabilityReq{}, profile.Profile{}, nil)
	require.NoError(t, err)
	require.Equal(t, stopReasonStop, resp.FinishReason)

	require.NotContains(t, fp.calledModels(), modelGLM52, "tripped primary must be SKIPPED")
	require.Contains(t, fp.calledModels(), modelMinimaxM3, "fallback must be attempted")
	// A breaker-skip is NOT a ProviderFallback event (D-06 events are for
	// transient-failure-driven walks, not breaker skips — the skip is logged at
	// Info "skip breaker open"). So no event is expected here.
	select {
	case <-ch:
		t.Fatal("a breaker skip must NOT emit a ProviderFallback event")
	default:
		// good — no event
	}
}

// TestSafetyCostHardStop: with the cost tracker pre-loaded to HardStop, Dispatch
// returns KindExhausted WITHOUT calling the provider (fake call count == 0).
func TestSafetyCostHardStop(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().set(modelGLM52, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s, _, _ := newSafetyScheduler(t, fp)
	// Pre-load a tracker into the HardStop state (degraded + the degraded-tier
	// budget also breached), so the very first Check() in Dispatch returns
	// CostHardStop.
	hard := NewCostCeilingTracker(CostCeilingConfig{AmountUSD: 1.0, Window: 24 * time.Hour, DegradeTo: tierLight}, map[string]Pricing{
		modelMinimaxM3: {InputPerMToken: 100.0, OutputPerMToken: 0},
	}, nil, nil)
	now := s.now()
	hard.Check(now)                            // init window
	hard.Account(modelMinimaxM3, 1_000_000, 0) // $100 on primary budget → degrade
	require.Equal(t, CostDegrade, hard.Check(now))
	hard.Account(modelMinimaxM3, 1_000_000, 0) // $100 on degraded budget → hard-stop
	require.Equal(t, CostHardStop, hard.Check(now), "precondition: tracker in HardStop state")
	s.SetCostTracker(hard)

	_, err := s.Dispatch(context.Background(), tierHeavy, "myproj", CapabilityReq{}, profile.Profile{}, nil)
	require.Error(t, err)

	var perr *provider.ProviderError

	require.ErrorAs(t, err, &perr)
	require.Equal(t, provider.KindExhausted, perr.Kind, "HardStop surfaces as KindExhausted")
	require.Equal(t, 0, fp.callCount(), "provider must NOT be called on HardStop")
}

// TestSafetyCostDegradeReResolves: on the first Degrade signal, Dispatch
// re-resolves to the degrade_to tier's model and calls it (one re-resolution,
// no infinite loop). valid.yaml: cost_ceiling.degrade_to = light → minimax-m3.
func TestSafetyCostDegradeReResolves(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().
		set(modelGLM52, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}}).
		set(modelMinimaxM3, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s, _, _ := newSafetyScheduler(t, fp)

	// Pre-load the tracker into the degraded state (so the first Check in
	// Dispatch returns CostDegrade and triggers the tier switch to "light").
	degrading := NewCostCeilingTracker(CostCeilingConfig{AmountUSD: 1.0, Window: 24 * time.Hour, DegradeTo: tierLight}, map[string]Pricing{
		modelGLM52:     {InputPerMToken: 100.0, OutputPerMToken: 0},
		modelMinimaxM3: {InputPerMToken: 0.0, OutputPerMToken: 0},
	}, nil, nil)
	now := s.now()
	degrading.Check(now)
	degrading.Account(modelGLM52, 1_000_000, 0) // $100 >= $1 ceiling → degrade
	require.Equal(t, CostDegrade, degrading.Check(now))
	s.SetCostTracker(degrading)

	resp, err := s.Dispatch(context.Background(), tierHeavy, "myproj", CapabilityReq{}, profile.Profile{}, nil)
	require.NoError(t, err)
	require.Equal(t, stopReasonStop, resp.FinishReason)

	called := fp.calledModels()
	require.NotContains(t, called, modelGLM52, "the original heavy primary must NOT be called (tier switched before any send)")
	require.Contains(t, called, modelMinimaxM3, "the degrade_to (light) tier's model must be called")
	// Exactly one re-resolution (no infinite loop): minimax-m3 called once.
	require.Len(t, called, 1, "one candidate call — no loop")
}

// TestSafetyStatePersistsAcrossDispatch: breaker + cost state is shared across
// Dispatch calls (a breaker tripped in call 1 stays tripped in call 2).
func TestSafetyStatePersistsAcrossDispatch(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().
		set(modelGLM52, fakeOutcome{err: transientErr(modelGLM52, 429)}).
		set(modelMinimaxM3, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s, _, _ := newSafetyScheduler(t, fp)

	// Dispatch 1: glm-5.2 transient → fallback minimax-m3 success.
	_, err := s.Dispatch(context.Background(), tierHeavy, "myproj", CapabilityReq{}, profile.Profile{}, nil)
	require.NoError(t, err)

	cb := s.Breaker(providerAnthropic, modelGLM52)
	require.NotNil(t, cb)
	consecAfter1 := consecutiveOf(cb)
	require.Equal(t, 1, consecAfter1, "one transient recorded on call 1")

	// Dispatch 2: another transient on glm-5.2 — counter advances (state persisted).
	fp.mu.Lock()
	fp.calls = nil
	fp.mu.Unlock()

	_, err = s.Dispatch(context.Background(), tierHeavy, "myproj", CapabilityReq{}, profile.Profile{}, nil)
	require.NoError(t, err)
	require.Equal(t, 2, consecutiveOf(cb), "breaker state persists — counter advanced on call 2")
}

func consecutiveOf(cb *CircuitBreaker) int {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.consecutive
}

// TestSafetyNewBreakersMapConstructsPerKey: NewBreakersMap returns one breaker
// per distinct (provider, model) referenced in tiers + fallbacks + windows +
// projects; NewCostTrackerFromConfig carries the config's ceiling.
func TestSafetyNewBreakersMapConstructsPerKey(t *testing.T) {
	t.Parallel()
	cfg := loadValid(t)
	bm := NewBreakersMap(cfg, nil)
	// Global heavy: glm-5.2 [anthropic], fallbacks minimax-m3 [openai], glm-4.6 [anthropic].
	require.Contains(t, bm, providerModelKey{providerAnthropic, modelGLM52})
	require.Contains(t, bm, providerModelKey{providerOpenAI, modelMinimaxM3})
	require.Contains(t, bm, providerModelKey{providerAnthropic, modelGLM46})
	// Each key has exactly one breaker (distinct *CircuitBreaker pointers).
	seen := map[*CircuitBreaker]struct{}{}
	for _, cb := range bm {
		seen[cb] = struct{}{}
	}

	require.Len(t, seen, len(bm), "one distinct breaker per key")

	ctr := NewCostTrackerFromConfig(cfg, nil, nil)
	require.InDelta(t, 50.0, ctr.cfg.AmountUSD, 1e-9)
	require.Equal(t, tierLight, ctr.cfg.DegradeTo)
	require.Equal(t, 24*time.Hour, ctr.cfg.Window)
	require.Contains(t, ctr.pricing, modelGLM52)
}
