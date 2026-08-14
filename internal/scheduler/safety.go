package scheduler

import (
	"log/slog"
	"os"

	"github.com/Djarvur/ass-guard-agent/internal/event"
)

// NewBreakersMap builds one CircuitBreaker per distinct (provider, model)
// referenced anywhere in the config — the global tier table + every fallback,
// every time-window tier, and every per-project override. One breaker per
// binding isolates outages (one bad provider does not kill the others, D-07).
// The map is keyed by (provider, model); Dispatch looks up the breaker for each
// candidate and falls back to a no-op when a key is absent.
func NewBreakersMap(cfg *Config, log *slog.Logger) map[providerModelKey]*CircuitBreaker {
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	out := map[providerModelKey]*CircuitBreaker{}
	add := func(modelSlug string) {
		m, ok := cfg.Models[modelSlug]
		if !ok {
			return
		}

		key := providerModelKey{Provider: m.Provider, Model: modelSlug}
		if _, exists := out[key]; !exists {
			out[key] = NewCircuitBreaker(key, cfg.CircuitBreaker, log)
		}
	}
	addBinding := func(b TierBinding) {
		add(b.Model)

		for _, fb := range b.Fallback {
			add(fb)
		}
	}

	for _, b := range cfg.Tiers {
		addBinding(b)
	}

	for _, w := range cfg.TimeWindows {
		for _, b := range w.Tiers {
			addBinding(b)
		}
	}

	for _, po := range cfg.Projects {
		for _, b := range po.Tiers {
			addBinding(b)
		}
	}

	return out
}

// NewCostTrackerFromConfig builds the pricing map from cfg.Models and constructs
// a CostCeilingTracker over the config's ceiling + window + degrade_to.
func NewCostTrackerFromConfig(cfg *Config, bus *event.Bus, log *slog.Logger) *CostCeilingTracker {
	pricing := make(map[string]Pricing, len(cfg.Models))
	for slug, m := range cfg.Models {
		pricing[slug] = m.Pricing
	}

	return NewCostCeilingTracker(cfg.CostCeiling, pricing, bus, log)
}

// InstallSafety wires real CircuitBreaker + CostCeilingTracker implementations
// into the Scheduler, replacing the Plan-03-02 no-op stubs. It is the single
// call site that turns Dispatch from "structure only" into "actually enforced".
func (s *Scheduler) InstallSafety(cfg *Config, bus *event.Bus) {
	concrete := NewBreakersMap(cfg, s.log)

	iface := make(map[providerModelKey]Breaker, len(concrete))

	for k, v := range concrete {
		iface[k] = v
	}

	s.breakers = iface
	s.cost = NewCostTrackerFromConfig(cfg, bus, s.log)
}

// Breaker returns the concrete CircuitBreaker for a (provider, model) key, or
// nil if none is registered (test/diagnostic accessor).
func (s *Scheduler) Breaker(providerSlug, model string) *CircuitBreaker {
	b, ok := s.breakers[providerModelKey{providerSlug, model}]
	if !ok {
		return nil
	}

	if cb, ok := b.(*CircuitBreaker); ok {
		return cb
	}

	return nil
}
