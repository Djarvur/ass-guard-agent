package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// Sem is the outbound-concurrency gate Dispatch acquires around every provider
// call (primary AND each fallback attempt — a fallback is still an outbound
// request and does not bypass the bound). provider.Semaphore satisfies this;
// tests inject a recording stub. RESEARCH §0, Phase-2 D-12 (default 6).
type Sem interface {
	Acquire(ctx context.Context) error
	Release()
}

// Breaker is the per-(provider,model) circuit-breaker seam. Plan 03-02 ships a
// no-op default (every candidate allowed); Plan 03-03 swaps in CircuitBreaker
// (Closed/Open/HalfOpen, D-07). The mutex inside a real breaker is held ONLY
// across these methods — never across the provider call (pitfall 6).
type Breaker interface {
	Allow(now time.Time) bool
	RecordSuccess()
	RecordTransient(now time.Time, err *provider.ProviderError)
}

// CostTracker is the cost-ceiling seam. Plan 03-02 ships a no-op default; Plan
// 03-03 swaps in CostCeilingTracker (dollars per fixed window, D-08).
type CostTracker interface {
	Check(now time.Time) CostAction
	Account(model string, inTokens, outTokens int)
}

// CostAction is what Check returns for the next request.
type CostAction int

const (
	// CostAllow means within budget — proceed.
	CostAllow CostAction = iota
	// CostDegrade means ceiling breached — degrade to the cheaper tier (D-08 first
	// breach). Plan 03-02 logs the seam; Plan 03-03 re-resolves to degrade_to.
	CostDegrade
	// CostHardStop means the degraded tier also hit its ceiling — Dispatch returns
	// KindExhausted (D-08 second breach).
	CostHardStop
)

// providerModelKey identifies one (provider, model) breaker (D-07 per-binding).
type providerModelKey struct {
	Provider string
	Model    string
}

// The Scheduler orchestrates resolve → capability gate → breaker → cost →
// semaphore → provider.Send → fallback walk. The developer-facing surface is a
// tier (heavy/good/light); everything else is operator config (the phase-goal
// promise: "the developer never thinks about which concrete model is running").
type Scheduler struct {
	resolver  *Resolver
	bus       *event.Bus
	sem       Sem
	providers map[string]provider.Provider
	breakers  map[providerModelKey]Breaker
	cost      CostTracker
	now       func() time.Time
	log       *slog.Logger
}

// NewScheduler constructs a Scheduler over the (validated) config. The breakers
// map defaults to empty (BreakerFor returns a no-op when a key is absent) and
// the cost tracker defaults to a no-op — Plan 03-03's safety.go swaps in real
// implementations. sem may be nil (a default Phase-2 semaphore is installed).
func NewScheduler(
	cfg *Config, bus *event.Bus, sem Sem,
	providers map[string]provider.Provider, log *slog.Logger,
) *Scheduler {
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	s := &Scheduler{
		resolver:  NewResolver(cfg),
		bus:       bus,
		providers: providers,
		breakers:  map[providerModelKey]Breaker{},
		cost:      noopCostTracker{},
		now:       time.Now,
		log:       log,
	}
	if sem != nil {
		s.sem = sem
	} else {
		s.sem = asDefaultSem()
	}

	return s
}

// SetBreakers injects a real breakers map (Plan 03-03). The map is keyed by
// (provider, model); candidates whose key is absent fall back to a no-op
// breaker (allowed).
func (s *Scheduler) SetBreakers(b map[providerModelKey]Breaker) {
	if b == nil {
		s.breakers = map[providerModelKey]Breaker{}

		return
	}

	s.breakers = b
}

// SetCostTracker injects a real cost tracker (Plan 03-03).
func (s *Scheduler) SetCostTracker(c CostTracker) {
	if c == nil {
		c = noopCostTracker{}
	}

	s.cost = c
}

// SetNow injects a fixed clock (for deterministic tests).
func (s *Scheduler) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// breakerFor returns the breaker for a candidate, or a no-op when none is
// registered for that (provider, model) key.
func (s *Scheduler) breakerFor(cand Target) Breaker {
	if b, ok := s.breakers[providerModelKey{cand.Provider, cand.Model}]; ok {
		return b
	}

	return noopBreaker{}
}

// Dispatch is the turn-loop seam. It resolves the tier, then walks
// [primary, ...fallback] applying per-candidate checks (capability → breaker →
// cost), calling the provider through the semaphore. On a Transient failure it
// emits a ProviderFallback event and tries the next candidate; on Structural it
// stops immediately and reports (D-04 — N5's silent retry-storm engineered
// out); on a cost HardStop it returns KindExhausted; on the first cost Degrade
// signal it re-resolves to the configured degrade_to tier (one re-resolution,
// guarded by a local flag so the degraded-tier walk does not loop — Plan 03-03).
//
// The resolved Target's Model is stamped onto a copy of prof before the call —
// the real Provider.Send takes a profile.Profile (the model lives in the
// profile), so the scheduler bridges tier→(provider,model)→profile.Model here.
// Response has no token-count fields on the Send path (Phase 1/2 design), so
// the cost Account on this path is (0,0) — pitfall 5's graceful "no token
// counts" path; full token-cost coupling lands when Dispatch drives Stream
// (Phase 4) where StreamChunk.Usage supplies the counts.
func (s *Scheduler) Dispatch(ctx context.Context, tier, project string, capReq CapabilityReq,
	prof *profile.Profile, messages []provider.Message) (provider.Response, error) {
	degradeTo := s.resolver.cfg.CostCeiling.DegradeTo
	turnID := turnIDFromCtx(ctx)
	currentTier := tier
	degraded := false // local: has Dispatch already performed the tier switch?

	var lastPerr *provider.ProviderError

	anyConsidered := false

	// Outer loop runs once for the requested tier, and one extra time if a cost
	// Degrade triggers a switch to the degrade_to tier (Plan 03-03).
	for {
		primary, fallbacks, err := s.resolver.Resolve(currentTier, project, s.now(), capReq)
		if err != nil {
			return provider.Response{}, err
		}

		candidates := append([]Target{primary}, fallbacks...)
		switchedTier := false

	candidateLoop:
		for i := range candidates {
			cand := candidates[i]
			// Capability gate (D-09 runtime seam). A zero-valued capReq (no
			// specific needs) skips the filter so the tracer behaves as 03-01.
			if !capReq.isZero() && !satisfies(cand.Capabilities, capReq) {
				s.log.Info("scheduler: skip candidate (capability mismatch)",
					"provider", cand.Provider, keyModel, cand.Model, "tier", currentTier)

				continue
			}

			anyConsidered = true

			// Breaker (D-07).
			b := s.breakerFor(cand)
			if !b.Allow(s.now()) {
				s.log.Info("scheduler: skip candidate (breaker open)",
					"provider", cand.Provider, keyModel, cand.Model)

				continue
			}

			// Cost (D-08). HardStop short-circuits with KindExhausted; the first
			// Degrade switches to the degrade_to tier (one re-resolution).
			switch s.cost.Check(s.now()) {
			case CostAllow:
				// within budget — proceed to acquire the semaphore and call.
			case CostHardStop:
				perr := &provider.ProviderError{
					Kind: provider.KindExhausted, Provider: cand.Provider, Model: cand.Model,
					Reason: "cost ceiling exhausted",
				}
				s.log.Warn("scheduler: cost ceiling exhausted (hard stop)",
					"provider", cand.Provider, keyModel, cand.Model)

				return provider.Response{}, perr
			case CostDegrade:
				if !degraded && degradeTo != "" && degradeTo != currentTier {
					degraded = true
					currentTier = degradeTo
					switchedTier = true

					s.log.Warn("scheduler: cost ceiling breached — degrading tier",
						"from_tier", tier, "to_tier", degradeTo, "provider", cand.Provider, keyModel, cand.Model)

					break candidateLoop
				}
				// Already degraded (or no degrade target): fall through and attempt
				// this candidate on the degraded tier.
			}

			// Semaphore + provider call.
			err := s.sem.Acquire(ctx)
			if err != nil {
				return provider.Response{}, fmt.Errorf("scheduler: acquire semaphore: %w", err)
			}

			callProf := prof
			callProf.Model = cand.Model
			resp, err := s.providers[cand.Provider].Send(ctx, callProf, messages)
			s.sem.Release()

			if err == nil {
				b.RecordSuccess()
				s.cost.Account(cand.Model, 0, 0) // Response has no token fields on the Send path

				return resp, nil
			}

			perr := asProviderError(err, &cand)
			if perr.Kind == provider.KindStructural {
				// Report, no retry (D-04). Structural errors do not feed the breaker
				// (pitfall 4: a 401 is a config bug, not an outage).
				return provider.Response{}, perr
			}

			if perr.Kind == provider.KindExhausted {
				return provider.Response{}, perr
			}

			// Transient: feed the breaker, emit fallback event, walk on.
			b.RecordTransient(s.now(), perr)
			s.cost.Account(cand.Model, 0, 0)

			lastPerr = perr

			if i < len(candidates)-1 {
				next := candidates[i+1]
				s.bus.Publish(ProviderFallback{
					TurnID:       turnID,
					FromProvider: cand.Provider, FromModel: cand.Model,
					ToProvider: next.Provider, ToModel: next.Model,
					Reason:    fmt.Sprintf("%s: HTTP %d: %s", perr.Kind, perr.StatusCode, perr.Reason),
					ErrorKind: perr.Kind, Attempt: i + 1,
				})
				s.log.Info("scheduler: transient failure — falling back",
					"from_provider", cand.Provider, "from_model", cand.Model,
					"to_provider", next.Provider, "to_model", next.Model,
					"reason", perr.Reason, "attempt", i+1)
			}
		} // end candidateLoop

		// If a cost Degrade triggered a tier switch, re-resolve with the
		// degrade_to tier and walk again; otherwise the dispatch is done.
		if switchedTier {
			continue
		}

		break
	} // end outer resolution loop

	if lastPerr != nil {
		return provider.Response{}, lastPerr
	}

	if !anyConsidered {
		// Every candidate was filtered by the capability gate.
		return provider.Response{}, fmt.Errorf(
			"scheduler: no candidate for tier %q satisfies capability requirement %+v", tier, capReq)
	}
	// All candidates were skipped by the breaker.
	return provider.Response{}, fmt.Errorf(
		"scheduler: all candidates for tier %q skipped (breaker open or cost)", tier)
}

// asProviderError extracts a *provider.ProviderError from err; if err is not one,
// it is wrapped via ClassifyHTTP (status 0) so the scheduler always pattern-
// matches on a typed Kind.
func asProviderError(err error, cand *Target) *provider.ProviderError {
	var perr *provider.ProviderError
	if errors.As(err, &perr) {
		if perr.Provider == "" {
			perr.Provider = cand.Provider
		}

		if perr.Model == "" {
			perr.Model = cand.Model
		}

		return perr
	}

	return provider.ClassifyHTTP(cand.Provider, cand.Model, 0, err)
}

// isZero reports whether a capability requirement is the zero value (no
// specific needs → no filtering).
func (r CapabilityReq) isZero() bool { return r == CapabilityReq{} }

// noopBreaker admits every candidate and records nothing (Plan 03-02 default).
type noopBreaker struct{}

func (noopBreaker) Allow(time.Time) bool                               { return true }
func (noopBreaker) RecordSuccess()                                     {}
func (noopBreaker) RecordTransient(time.Time, *provider.ProviderError) {}

// noopCostTracker always allows and accounts nothing (Plan 03-02 default).
type noopCostTracker struct{}

func (noopCostTracker) Check(time.Time) CostAction { return CostAllow }
func (noopCostTracker) Account(string, int, int)   {}

// asDefaultSem installs a default Phase-2 semaphore when NewScheduler is given
// nil (kept lazy so the scheduler package does not import provider's concrete
// constructor at load time — it does, but the indirection makes test injection
// of a fake Sem clean).
func asDefaultSem() Sem { //nolint:ireturn // Sem abstraction over concrete type
	return provider.NewDefaultSemaphore()
}

// turnIDKey carries the turn id through context for event tagging.
type turnIDKey struct{}

// WithTurnID returns ctx annotated with the turn id (the turn loop sets this).
func WithTurnID(ctx context.Context, turnID string) context.Context {
	return context.WithValue(ctx, turnIDKey{}, turnID)
}

func turnIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(turnIDKey{}).(string); ok {
		return v
	}

	return ""
}
