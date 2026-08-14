---
phase: 03-model-scheduling
status: passed
phase_goal: "An operator can configure ass-guard to use cheap models off-peak and heavy models at peak hours, override the tier→model table per project, and trust that a provider outage degrades gracefully instead of cascading — while the developer never thinks about which concrete model is running."
requirements: [SCHED-01, SCHED-02, SCHED-03, SCHED-04, SCHED-05, SCHED-06]
must_haves_verified: 4
must_haves_total: 4
verified_at: 2026-08-12
---

# Phase 3 Verification — Model Scheduling

**Status: PASSED** — all 4 success criteria verified against the actual codebase;
all 6 SCHED requirements accounted for; the full project test suite is green
under `-race`; `CGO_ENABLED=0 go build ./...` is clean (the single-static-binary
constraint holds with `time/tzdata` bundled).

## Phase goal achievement

The phase goal promised an operator-controllable, developer-invisible model
scheduling layer. Verification confirms:

- **Operator-controllable:** a declarative `scheduling.yaml` (loaded by
  `internal/scheduler/load.go`) configures tier→model mappings, time-windows,
  per-project overrides, fallback chains, capability profiles, circuit-breaker
  thresholds, and cost ceilings. The `ass-guard scheduling validate` + `resolve`
  CLI lets the operator inspect/validate without running the agent.
- **Developer-invisible:** the turn loop calls `Scheduler.Dispatch(ctx, tier, …)`
  with a tier label (heavy/good/light); everything else is config. The phase's
  integration test (`TestSchedulerEndToEnd`) drives this end-to-end.
- **Graceful degradation:** transient failures walk a configured fallback chain
  (emitting `ProviderFallback` events); circuit breakers skip cascading outages;
  the cost ceiling degrades then stops; structural failures report immediately
  (no retry storm).

## Success criteria × evidence

### 1. Tier abstraction (SCHED-01) — VERIFIED

A command/skill/subagent selects a tier; the resolver returns a concrete
(provider, model) at request time.

- `Resolver.Resolve(tier, project, now, capReq) (Target, []Target, error)` —
  `internal/scheduler/resolver.go`. `Target{Provider, Model, BaseURL, Shape,
  Capabilities, Pricing}` is the hand-off shape.
- `Scheduler.Dispatch(ctx, tier, project, capReq, prof, messages)` —
  `internal/scheduler/dispatch.go`; the turn-loop seam.
- `ass-guard scheduling resolve --tier heavy` — `cmd/ass-guard/scheduling.go`
  (operator surface; `TestSchedulingResolveJSON` proves the resolution).
- Tests: `TestResolveGlobalFallback`, `TestResolveFallbackChainEnriched`,
  `TestSchedulingResolveJSON`, `TestSchedulingResolvePeakWindow`.

### 2. Time-windowed substitution + per-project override (SCHED-02, SCHED-03) — VERIFIED

Time-windows are timezone-explicit (IANA zones, `time/tzdata` bundled); a
per-project override narrows within the active table; falls back to the global
default when unset.

- `activeWindow` + `TimeWindow.contains` + `parseHHMM` + `dayMatches` —
  `internal/scheduler/resolver.go`; handles same-day, overnight-wrap
  (22:00→06:00), weekday-filter, to-exclusive boundary.
- D-02 precedence (time-window → project → global, the user's explicit reversal)
  in `resolveBinding`.
- `_ "time/tzdata"` blank import — `internal/scheduler/init.go`;
  `TestTzdataBundled` proves a non-host zone loads.
- Tests: `TestResolveWindowWinsOutright` (D-02 reversal), `TestResolveOffPeakOvernight`,
  `TestActiveWindowOvernightBoundary` (5 boundary subtests), `TestActiveWindowWeekdayFilter`,
  `TestResolveWindowZoneOverride`, `TestLoadLayering` (per-project overlay).

### 3. Transient vs structural fallback (SCHED-04) — VERIFIED

On transient failure (429/5xx/network), ass-guard walks a configured fallback
chain; on structural failure (401/403), it reports instead of silently retrying
(N5 engineered out).

- `ProviderError` + `ErrorKind` (Transient/Structural/Exhausted) +
  `ClassifyHTTP` — `internal/provider/errors.go`. The Exhausted invariant
  (`ClassifyHTTP` never returns it) is asserted by `TestClassifyExhaustedInvariant`
  looping every status 100-599.
- `Scheduler.Dispatch` walks [primary, ...fallback] on Transient, emits a
  `ProviderFallback` event per step (D-06), and stops immediately on Structural.
- Tests: `TestDispatchTransientWalkSuccess`, `TestDispatchChainTwoFailuresThenSuccess`,
  `TestDispatchChainExhausted`, `TestDispatchStructuralStopsWalk` (zero events on
  Structural), `TestProviderErrorRedactsCause` (C2 — sk-/Bearer scrubbed).

### 4. Circuit breakers + cost ceilings + capability profiles (SCHED-05, SCHED-06) — VERIFIED

- **Circuit breakers (D-07):** `CircuitBreaker` — `internal/scheduler/breaker.go`;
  Closed/Open/HalfOpen; trips via consecutive (N=5) OR error-rate (>50% over full
  M=20 window); cooldown + half-open probe; per-(provider,model). Tests:
  `TestBreakerTripViaConsecutive`, `TestBreakerTripViaErrorRate`,
  `TestBreakerConcurrency` (100 goroutines, `-race`), `TestSafetyBreakerSkipsOpenCandidate`.
- **Cost ceiling (D-08):** `CostCeilingTracker` — `internal/scheduler/cost.go`;
  dollars per fixed window; degrade-then-stop. Tests: `TestCostFirstBreachDegrade`,
  `TestCostSecondBreachHardStop`, `TestCostWindowRollover`, `TestSafetyCostHardStop`,
  `TestSafetyCostDegradeReResolves`.
- **Capability profiles (D-09/D-10):** structured `CapabilityProfile` per model.
  Load-time: `Validate` rejects inconsistent configs (D-10) — `TestLoadInvalidCapMismatch`.
  Request-time: `applyCapabilityGate` skips incapable candidates —
  `TestCapabilityNeedsToolsSkipsToolLessPrimary`, `TestCapabilityNoCandidateReturnsError`.

## Requirement traceability

| REQ-ID | Plan(s) | Status | Primary evidence |
|--------|---------|--------|------------------|
| SCHED-01 | 03-01, 03-04 | ✓ | `Resolver.Resolve`, `Scheduler.Dispatch`, `scheduling resolve` CLI |
| SCHED-02 | 03-01 | ✓ | `activeWindow`/`contains` + overnight-wrap + tzdata bundling |
| SCHED-03 | 03-01 | ✓ | `resolveBinding` (project narrows within D-02) + `TestLoadLayering` |
| SCHED-04 | 03-02 | ✓ | `ProviderError`/`ClassifyHTTP` + fallback walker + Structural stop |
| SCHED-05 | 03-03 | ✓ | `CircuitBreaker` (D-07) + `CostCeilingTracker` (D-08) |
| SCHED-06 | 03-01, 03-04 | ✓ | `Validate` (D-10 load-time) + `applyCapabilityGate` (D-09 request-time) |

## Threat-model mitigations (all in place)

T-03-01 (tzdata) · T-03-02 (inconsistent config rejected) · T-03-03 (D-02 reversal)
· T-03-04 (provider/model in errors — accept) · T-03-05 (classification +
Exhausted invariant) · T-03-06 (secret scrub) · T-03-07 (Structural stops walk)
· T-03-08 (fallback observable) · T-03-09 (cascade breaker) · T-03-10 (cost
degrade-then-stop) · T-03-11 (mutex scope, `-race` clean) · T-03-12 (Warn logs)
· T-03-13 (runtime capability gate) · T-03-14 (transport discipline) · T-03-15
(resolve output — accept, no secrets). All verified by the cited tests.

## Notable deviations (investigate-and-fix-ready, documented in plan SUMMARYs)

1. **Loader uses `gopkg.in/yaml.v3`, not `spf13/viper`** (03-01). Viper's flatten
   splits dotted map keys, mangling model slugs (`glm-5.2` → `glm-5`). yaml.v3
   preserves them. Operator-facing D-01 contract unchanged.
2. **Real-interface adaptations** (03-02): `Provider.Send` takes `profile.Profile`
   (model stamped via `callProf.Model = cand.Model`); `Response` has no token
   fields (cost Account is (0,0) on the Send path — pitfall 5; full coupling with
   Stream in Phase 4); `Sem` is an interface for test injection.
3. **Error-rate mechanism requires a full M-window** (03-03). Without the guard,
   a single transient on a near-empty window trips instantly (rate=1.0), defeating
   "slow on degraded performance."
4. **A breaker-skip is NOT a `ProviderFallback` event** (03-03). D-06 events are
   for transient-failure-driven walks; breaker skips are logged at Info.

## Manual / operator-gated items (not exercised autonomously)

- A live 429 from a rate-limited endpoint producing a `ProviderFallback`
  session/update in the ACP stream (needs Phase-1 adapters + a live key — the
  VALIDATION Manual-Only path; the fake-provider dispatch tests prove the logic).
- A live cost-ceiling breach against a real endpoint (needs tokens from Stream —
  Phase 4 wires Dispatch to Stream where `StreamChunk.Usage` supplies counts; the
  cost tracker's arithmetic is unit-tested directly with real token counts).

These are out of Phase-3's autonomous scope (the plans are `autonomous: true`:
the scheduler uses fake/mock providers for tests, not live calls) and surface
naturally when Phase 1's adapters and Phase 4's Stream-driven dispatch land.
