---
phase: 03-model-scheduling
plan: 03
status: complete
requirements: [SCHED-05]
key-files:
  created:
    - internal/scheduler/breaker.go
    - internal/scheduler/breaker_test.go
    - internal/scheduler/cost.go
    - internal/scheduler/cost_test.go
    - internal/scheduler/safety.go
    - internal/scheduler/safety_test.go
  modified:
    - internal/scheduler/dispatch.go
---

# Plan 03-03 SUMMARY — Circuit breakers + cost ceiling (the safety nets)

## What was built

The two safety nets that stop cascading outages and cost surprises (SCHED-05):

- **`internal/scheduler/breaker.go`** — `CircuitBreaker` (Closed/Open/HalfOpen
  per-(provider,model) state machine, D-07). Both trip mechanisms OR:
  consecutive-failure (N=5) AND error-rate (>50% over M=20, consulted only once
  the M-window is full so a single failure on a near-empty window cannot trip).
  Cooldown (60s) → half-open probe → close on success / re-open on failure.
- **`internal/scheduler/cost.go`** — `CostCeilingTracker` (D-08): dollars per
  fixed calendar-aligned window, degrade-then-stop. First breach → CostDegrade +
  one warn event; second breach → CostHardStop (Dispatch returns KindExhausted).
- **`internal/scheduler/safety.go`** — `NewBreakersMap`, `NewCostTrackerFromConfig`,
  `Scheduler.InstallSafety` — the wiring that swaps the Plan-03-02 no-op stubs
  for real implementations.
- **`internal/scheduler/dispatch.go`** modified — Dispatch's cost-Degrade branch
  now re-resolves to the `degrade_to` tier (one re-resolution, guarded by a local
  flag so the degraded-tier walk does not loop); the breaker branch is now backed
  by the real `Allow()`.

## Decisions honored

- **D-07** both consecutive + error-rate (the most-robust option); per-(provider,
  model) isolation; cooldown + half-open probe; only Transient feeds the breaker
  (no RecordStructural — pitfall 4).
- **D-08** dollars per fixed window; degrade-then-stop (NOT hard-stop-on-first-
  breach — the ROADMAP goal's graceful-degradation language).

## Notable findings / investigate-and-fix-ready

1. **Error-rate mechanism requires a full M-window.** The first cut checked rate
   on every Transient, which tripped instantly (rate=1.0 on a 1-sample window),
   defeating "slow on degraded performance" and making the consecutive mechanism
   pointless. Added a `window.len() >= M` guard (RESEARCH §6.1 "over the last M
   requests" = full window). This is a load-bearing semantic clarification the
   tests caught.

2. **A breaker-skip is NOT a ProviderFallback event.** D-06 events are for
   transient-failure-driven walks ("provider X failed (429), falling back"). A
   breaker skip is a separate candidate-skip reason (logged at Info "skip
   breaker open"), not an event — the provider was never called, so no failure
   to report. TestSafetyBreakerSkipsOpenCandidate asserts no event fires.

3. **Dispatch cost-Degrade uses an outer tier-resolution loop.** The candidate
   walk is wrapped in an outer `for` that re-resolves with `degrade_to` on the
   first Degrade signal (one re-resolution, guarded). The 10 Plan-03-02 dispatch
   tests pass unchanged (noop cost → CostAllow → the cost switch is a no-op).

## Self-Check: PASSED

- 5 safety + 10 breaker + 7 cost tests pass -race.
- The full scheduler suite (Plan-03-01/02/03) is green; provider suite green
  (no Phase-1/2 regression).
- `grep -c "RecordStructural" internal/scheduler/breaker.go` = 0.

## Evidence

- Consecutive trip: `TestBreakerTripViaConsecutive` — 5 transients → Open.
- Error-rate trip: `TestBreakerTripViaErrorRate` — full M-window with 0.55 rate
  → Open despite consecutive=2.
- Concurrency: `TestBreakerConcurrency` — 100 goroutines × 50 calls, `-race`
  clean, consistent final state.
- Breaker-skip in Dispatch: `TestSafetyBreakerSkipsOpenCandidate` — tripped
  primary skipped, fallback called.
- Cost HardStop: `TestSafetyCostHardStop` — KindExhausted returned, provider not
  called.
- Cost Degrade re-resolve: `TestSafetyCostDegradeReResolves` — original-tier
  primary NOT called, degrade_to tier's model called once (no loop).
- State persists: `TestSafetyStatePersistsAcrossDispatch` — breaker counter
  advances across calls.
