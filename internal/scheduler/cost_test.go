package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/event"
)

// newCostTracker builds a tracker with a fresh bus + the given pricing.
func newCostTracker(t *testing.T, cfg CostCeilingConfig, pricing map[string]Pricing) (*CostCeilingTracker, *event.Bus) {
	t.Helper()

	bus := event.NewBus()

	t.Cleanup(func() { bus.Close() })

	return NewCostCeilingTracker(cfg, pricing, bus, nil), bus
}

// TestCostArithmetic: Account applies the per-model pricing correctly.
// glm-5.2 @ {0.60 in, 2.20 out} for 1M in + 0.5M out = $1.70.
func TestCostArithmetic(t *testing.T) {
	cfg := CostCeilingConfig{AmountUSD: 50.0, Window: 24 * time.Hour, DegradeTo: "light"}
	tr, _ := newCostTracker(t, cfg, map[string]Pricing{
		"glm-5.2": {InputPerMToken: 0.60, OutputPerMToken: 2.20},
	})
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)

	tr.Account("glm-5.2", 1_000_000, 500_000)
	require.InDelta(t, 1.70, tr.Spent(), 1e-9, "(1M*0.60 + 0.5M*2.20)/1e6 = $1.70")
	// Check initializes the window but does not trip (1.70 < 50).
	require.Equal(t, CostAllow, tr.Check(now))
}

// TestCostFirstBreachDegrade: crossing amount_usd returns CostDegrade + emits
// exactly ONE CostCeilingWarn (HardStop=false). Subsequent Checks are
// idempotently CostDegrade.
func TestCostFirstBreachDegrade(t *testing.T) {
	cfg := CostCeilingConfig{AmountUSD: 50.0, Window: 24 * time.Hour, DegradeTo: "light"}
	tr, bus := newCostTracker(t, cfg, map[string]Pricing{
		"glm-5.2": {InputPerMToken: 60.0, OutputPerMToken: 0}, // $60 per 1M in
	})
	ch := bus.Subscribe("CostCeilingWarn", 4)
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	require.Equal(t, CostAllow, tr.Check(now))
	// 1M input tokens × $60/MTok = $60 >= $50 → first breach on the NEXT Check.
	tr.Account("glm-5.2", 1_000_000, 0)

	got := tr.Check(now)
	require.Equal(t, CostDegrade, got, "first breach returns CostDegrade")
	require.True(t, tr.IsDegraded())
	// Subsequent Check is idempotent (still CostDegrade, no second event).
	require.Equal(t, CostDegrade, tr.Check(now))

	// Exactly one warn event with HardStop=false + DegradedTo set.
	var warns []CostCeilingWarn

drain:
	for {
		select {
		case e := <-ch:
			if w, ok := e.(CostCeilingWarn); ok {
				warns = append(warns, w)
			}
		default:
			break drain
		}
	}

	require.Len(t, warns, 1, "exactly ONE CostCeilingWarn on first breach (idempotent thereafter)")
	require.False(t, warns[0].HardStop)
	require.Equal(t, "light", warns[0].DegradedTo)
	require.InDelta(t, 60.0, warns[0].Spent, 1e-9)
}

// TestCostSecondBreachHardStop: once degraded, accumulating past the (same)
// ceiling on the degraded budget returns CostHardStop + a warn(HardStop=true).
func TestCostSecondBreachHardStop(t *testing.T) {
	cfg := CostCeilingConfig{AmountUSD: 50.0, Window: 24 * time.Hour, DegradeTo: "light"}
	tr, bus := newCostTracker(t, cfg, map[string]Pricing{
		"minimax-m3": {InputPerMToken: 60.0, OutputPerMToken: 0},
	})
	ch := bus.Subscribe("CostCeilingWarn", 4)
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	tr.Check(now) // initialize window
	// First breach.
	tr.Account("minimax-m3", 1_000_000, 0) // $60 on primary budget
	require.Equal(t, CostDegrade, tr.Check(now))
	require.True(t, tr.IsDegraded())
	// Drain the first warn so we can isolate the hard-stop warn.
drain1:
	for {
		select {
		case <-ch:
		default:
			break drain1
		}
	}
	// Second breach: accumulate against the degraded budget past the ceiling.
	tr.Account("minimax-m3", 1_000_000, 0) // another $60 on degraded budget
	got := tr.Check(now)
	require.Equal(t, CostHardStop, got, "second breach returns CostHardStop")

	// The hard-stop warn arrived.
	var hs *CostCeilingWarn

drain2:
	for {
		select {
		case e := <-ch:
			if w, ok := e.(CostCeilingWarn); ok {
				hs = &w

				break drain2
			}
		default:
			break drain2
		}
	}

	require.NotNil(t, hs, "hard-stop warn must be published")
	require.True(t, hs.HardStop)
}

// TestCostWindowRollover: crossing the calendar boundary freezes the prior
// window + resets spent/degraded; Check returns CostAllow again.
func TestCostWindowRollover(t *testing.T) {
	cfg := CostCeilingConfig{AmountUSD: 50.0, Window: 24 * time.Hour, DegradeTo: "light"}
	tr, _ := newCostTracker(t, cfg, map[string]Pricing{
		"glm-5.2": {InputPerMToken: 60.0, OutputPerMToken: 0},
	})
	t0 := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	tr.Check(t0)
	tr.Account("glm-5.2", 1_000_000, 0) // $60
	require.Equal(t, CostDegrade, tr.Check(t0))
	require.True(t, tr.IsDegraded())

	// Cross into the next window (>24h later).
	t1 := t0.Add(25 * time.Hour)
	require.Equal(t, CostAllow, tr.Check(t1), "window rollover resets the budget")
	require.False(t, tr.IsDegraded(), "degraded flag resets on rollover")
	require.InDelta(t, 0.0, tr.Spent(), 1e-9)
}

// TestCostNoTokenCountsGraceful: Account(0,0) is a no-op that logs a Warn (the
// Send path carries no token counts, pitfall 5) — the ceiling cannot trip
// without tokens.
func TestCostNoTokenCountsGraceful(t *testing.T) {
	cfg := CostCeilingConfig{AmountUSD: 50.0, Window: 24 * time.Hour, DegradeTo: "light"}
	tr, _ := newCostTracker(t, cfg, map[string]Pricing{
		"glm-5.2": {InputPerMToken: 60.0, OutputPerMToken: 0},
	})
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	tr.Check(now)

	for range 100 {
		tr.Account("glm-5.2", 0, 0)
	}

	require.InDelta(t, 0.0, tr.Spent(), 1e-9, "no token counts → $0 accounted")
	require.False(t, tr.IsDegraded(), "ceiling cannot trip without tokens")
	require.Equal(t, CostAllow, tr.Check(now))
}

// TestCostConcurrentAccount: 100 goroutines calling Account concurrently under
// -race → consistent window total (no lost updates).
func TestCostConcurrentAccount(t *testing.T) {
	cfg := CostCeilingConfig{AmountUSD: 1e9, Window: 24 * time.Hour, DegradeTo: "light"} // huge ceiling, no trip
	tr, _ := newCostTracker(t, cfg, map[string]Pricing{
		"glm-5.2": {InputPerMToken: 1.0, OutputPerMToken: 0},
	})
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	tr.Check(now)

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			for range 100 {
				tr.Account("glm-5.2", 1_000_000, 0) // $1 each
			}
		})
	}

	wg.Wait()
	// 100 goroutines × 100 calls × $1 = $10000.
	require.InDelta(t, 10000.0, tr.Spent(), 1e-6, "no lost updates under concurrency")
}

// TestCostDisabled: AmountUSD=0 disables the ceiling (always CostAllow).
func TestCostDisabled(t *testing.T) {
	tr, _ := newCostTracker(t, CostCeilingConfig{AmountUSD: 0, Window: 24 * time.Hour}, map[string]Pricing{
		"m": {InputPerMToken: 1e6, OutputPerMToken: 0},
	})
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	tr.Check(now)
	tr.Account("m", 1_000_000, 0) // would be $1M, but ceiling disabled
	require.Equal(t, CostAllow, tr.Check(now))
	require.False(t, tr.IsDegraded())
}
