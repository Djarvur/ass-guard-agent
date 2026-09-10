package modelrouting

import (
	"log/slog"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
)

// AggregateStats is the per-(provider, model) tally Aggregate produces.
type AggregateStats struct {
	OK          int     `json:"ok"`
	Transient   int     `json:"transient"`
	Structural  int     `json:"structural"`
	Exhausted   int     `json:"exhausted"`
	InTokens    int64   `json:"in_tokens"`
	OutTokens   int64   `json:"out_tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

// OutcomeWindow bounds aggregation to the last N recorded outcomes per key
// (Pitfall 8: the window is expressed in recorded outcomes, not wall-clock
// alone — restarts must not erase evidence). OutcomeWindowAll disables
// truncation.
type OutcomeWindow int

// OutcomeWindowAll aggregates over every record.
const OutcomeWindowAll OutcomeWindow = 0

// Aggregate tallies per-(provider, model) outcome counts and token/cost totals
// over the trailing window.
//
// RED stub (plan 24-01 Task 1): returns nil.
func Aggregate(_ []DispatchOutcome, _ OutcomeWindow) map[providerModelKey]AggregateStats {
	return nil
}

// ReplayBreakers chronologically replays outcome records through the EXISTING
// breaker seam, constructing one CircuitBreaker per seen key: ok records
// RecordSuccess, transient records RecordTransient with a synthetic
// KindTransient ProviderError, and structural/exhausted records are skipped
// entirely (they never feed breakers on the live path either).
//
// RED stub (plan 24-01 Task 1): returns nil.
func ReplayBreakers(_ []DispatchOutcome, _ CircuitBreakerConfig, _ *slog.Logger) map[providerModelKey]Breaker {
	return nil
}

// ReplayCostTracker replays token-bearing records through the EXISTING
// CostCeilingTracker seam (Account per record), so windowed spend accumulated
// in past processes still degrades routing after a restart.
//
// RED stub (plan 24-01 Task 1): returns nil.
func ReplayCostTracker(
	_ []DispatchOutcome, _ CostCeilingConfig, _ map[string]Pricing, _ *event.Bus, _ *slog.Logger,
) CostTracker {
	return nil
}

// FirstAllowed walks the candidate chain and returns the first candidate whose
// breaker (absent key = allowed by convention) admits now; the second return
// reports whether the walk demoted past the primary candidate.
//
// RED stub (plan 24-01 Task 1): zero Target, false.
func FirstAllowed(_ []Target, _ map[providerModelKey]Breaker, _ time.Time) (Target, bool) {
	return Target{}, false
}
