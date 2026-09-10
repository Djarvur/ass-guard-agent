package modelrouting

import (
	"log/slog"
	"slices"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// AggregateStats is the per-(provider, model) tally Aggregate produces.
type AggregateStats struct {
	OK         int     `json:"ok"`
	Transient  int     `json:"transient"`
	Structural int     `json:"structural"`
	Exhausted  int     `json:"exhausted"`
	InTokens   int64   `json:"in_tokens"`
	OutTokens  int64   `json:"out_tokens"`
	CostUSD    float64 `json:"cost_usd"`
}

// OutcomeWindow bounds aggregation to the last N recorded outcomes per key
// (Pitfall 8: the window is expressed in recorded outcomes, not wall-clock
// alone — a restart must not erase evidence, and record count is the only unit
// the append-only store guarantees). OutcomeWindowAll disables truncation.
type OutcomeWindow int

// OutcomeWindowAll aggregates over every record.
const OutcomeWindowAll OutcomeWindow = 0

// Aggregate tallies per-(provider, model) outcome counts and token/cost totals
// over each key's trailing window of records, processed chronologically (At,
// stable — store order breaks ties). Zero token/cost fields mean
// unknown-on-that-path and contribute nothing to the totals.
func Aggregate(records []DispatchOutcome, window OutcomeWindow) map[ProviderModelKey]AggregateStats {
	ordered := chronological(records)

	byKey := make(map[ProviderModelKey][]DispatchOutcome)
	for i := range ordered {
		rec := ordered[i]
		key := ProviderModelKey{Provider: rec.Provider, Model: rec.Model}
		if window > 0 && len(byKey[key]) >= int(window) {
			byKey[key] = byKey[key][1:] // slide: the oldest record leaves the window
		}

		byKey[key] = append(byKey[key], rec)
	}

	out := make(map[ProviderModelKey]AggregateStats, len(byKey))
	for key, recs := range byKey {
		out[key] = tally(recs)
	}

	return out
}

// ReplayBreakers chronologically replays outcome records through the EXISTING
// breaker seam (D-06: no new routing layer — the map is consumable via
// Scheduler.SetBreakers). One CircuitBreaker is constructed per key with at
// least one ok-or-transient record: ok records take the RecordSuccess path and
// transient records the RecordTransient path (a synthetic KindTransient
// ProviderError carrying the record's provider/model), mirroring the Dispatch
// outcome-learning points exactly. Structural and exhausted records are
// skipped entirely — they never feed breakers live, so they never do on
// replay either. Keys absent from the returned map are allowed by the
// Scheduler's no-op-breaker convention.
func ReplayBreakers(
	records []DispatchOutcome, breakerCfg CircuitBreakerConfig, log *slog.Logger,
) map[ProviderModelKey]Breaker {
	out := make(map[ProviderModelKey]Breaker)

	ordered := chronological(records)
	for i := range ordered {
		rec := &ordered[i]
		if rec.Outcome != OutcomeOK && rec.Outcome != OutcomeTransient {
			continue // structural/exhausted: terminal outcomes never feed the breaker
		}

		key := ProviderModelKey{Provider: rec.Provider, Model: rec.Model}
		b, ok := out[key]
		if !ok {
			b = NewCircuitBreaker(key, breakerCfg, log)
		}

		if rec.Outcome == OutcomeOK {
			b.RecordSuccess()
		} else {
			b.RecordTransient(rec.At, &provider.ProviderError{
				Kind:     provider.KindTransient,
				Provider: rec.Provider,
				Model:    rec.Model,
			})
		}

		out[key] = b
	}

	return out
}

// ReplayCostTracker replays token-bearing records through the EXISTING
// CostCeilingTracker seam (D-06), so windowed spend accumulated in past
// processes still degrades routing after a restart. Account is called per
// record in chronological order; outcome class is irrelevant to spend (a
// timed-out stream can still have consumed tokens), and records with unknown
// tokens (the zero fields) are skipped — Account's own (0,0) no-op, without
// its per-record warn noise over a large store. The int narrowing is lossless:
// token counts come from int64 usage fields and int is 64-bit on every
// supported platform.
//
//nolint:ireturn // the seam's own interface type is the contract (SetCostTracker takes it)
func ReplayCostTracker(
	records []DispatchOutcome, costCfg CostCeilingConfig, pricing map[string]Pricing,
	bus *event.Bus, log *slog.Logger,
) CostTracker {
	tr := NewCostCeilingTracker(costCfg, pricing, bus, log)

	ordered := chronological(records)
	for i := range ordered {
		rec := &ordered[i]
		if rec.InTokens == 0 && rec.OutTokens == 0 {
			continue
		}

		tr.Account(rec.Model, int(rec.InTokens), int(rec.OutTokens))
	}

	return tr
}

// FirstAllowed walks the candidate chain in order and returns the first
// candidate whose breaker admits now. A key ABSENT from the breakers map is
// allowed (the SetBreakers no-op-breaker convention); only a present, denying
// breaker skips its candidate. The second return reports whether the walk
// demoted past the primary (the first candidate). When every candidate is
// denied, the zero Target and false are returned — callers treat a zero Model
// as "no candidate admitted".
func FirstAllowed(candidates []Target, breakers map[ProviderModelKey]Breaker, now time.Time) (Target, bool) {
	for i := range candidates {
		cand := &candidates[i]
		b, ok := breakers[ProviderModelKey{Provider: cand.Provider, Model: cand.Model}]
		if ok && !b.Allow(now) {
			continue
		}

		return *cand, i > 0
	}

	return Target{}, false
}

// chronological returns the records ordered by At (stable — store order breaks
// ties), so replay and aggregation process outcomes in the order they happened
// regardless of how the caller assembled the slice.
func chronological(records []DispatchOutcome) []DispatchOutcome {
	ordered := slices.Clone(records)
	slices.SortStableFunc(ordered, func(a, b DispatchOutcome) int {
		return a.At.Compare(b.At)
	})

	return ordered
}

// tally reduces one key's (already windowed) records to AggregateStats.
func tally(records []DispatchOutcome) AggregateStats {
	var stats AggregateStats
	for i := range records {
		rec := &records[i]
		switch rec.Outcome {
		case OutcomeOK:
			stats.OK++
		case OutcomeTransient:
			stats.Transient++
		case OutcomeStructural:
			stats.Structural++
		case OutcomeExhausted:
			stats.Exhausted++
		}

		stats.InTokens += rec.InTokens
		stats.OutTokens += rec.OutTokens
		stats.CostUSD += rec.CostUSD
	}

	return stats
}
