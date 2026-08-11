---
phase: 03-model-scheduling
plan: 02
status: complete
requirements: [SCHED-04]
key-files:
  created:
    - internal/provider/errors.go
    - internal/provider/errors_test.go
    - internal/scheduler/events.go
    - internal/scheduler/events_test.go
    - internal/scheduler/dispatch.go
    - internal/scheduler/dispatch_test.go
  modified:
    - internal/provider/doc.go
---

# Plan 03-02 SUMMARY — Runtime dispatch + typed ProviderError + fallback chain

## What was built

The runtime half of the scheduling seam (the dispatch + fallback-walk half;
Plan 03-01 delivered the resolution half):

- **`internal/provider/errors.go`** — `ErrorKind` (Transient/Structural/
  Exhausted), `ProviderError` (typed error with `.Kind`), `ClassifyHTTP`
  (adapter-side HTTP-status classifier), and the `Exhausted`-from-adapter
  invariant. `ProviderError.Error()` is investigate-and-fix-ready (C5) and
  routes `Cause` through `redact.ScrubError` (C2).
- **`internal/scheduler/events.go`** — `ProviderFallback` + `CostCeilingWarn`
  event kinds extending the Phase-2 bus catalog additively (both implement
  `event.Event`).
- **`internal/scheduler/dispatch.go`** — `Scheduler.Dispatch` orchestrating
  resolve → capability gate → breaker → cost → semaphore → provider.Send →
  fallback walk. Defines the `Breaker` / `CostTracker` / `Sem` seam interfaces
  (no-op defaults; Plan 03-03 swaps in real implementations) and
  `providerModelKey` + `asProviderError`.

## Decisions honored

- **D-04** typed `ProviderError` with `.Kind`; `ClassifyHTTP` (408/425/429/5xx/
  net → Transient; 400/401/403/404/405/411/413/422 → Structural; unknown →
  Transient safe-side; NEVER Exhausted — invariant tested across status 100-599).
- **D-05** explicit per-tier ordered fallback list walked in order; on chain
  exhaustion the last Transient error is surfaced.
- **D-06** `ProviderFallback` event published per fallback attempt → bus → ACP
  `session/update`.

## Notable adaptations to the REAL Phase-1/2 interfaces (investigate-and-fix-ready)

The plans were written against INTENDED interfaces; the on-disk Phase 1/2 code
differs in two load-bearing places, both adapted cleanly:

1. **`Provider.Send(ctx, prof profile.Profile, messages)` — not
   `Send(ctx, model string, messages)`.** The model slug lives inside the
   profile. Dispatch bridges tier→(provider,model)→profile by stamping the
   resolved `Target.Model` onto a copy of `prof` (`callProf.Model = cand.Model`)
   before each provider call. Dispatch's signature is therefore
   `Dispatch(ctx, tier, project, capReq, prof profile.Profile, messages)` — it
   accepts the profile the turn loop already holds. The fake provider reads
   `prof.Model` to pick its canned outcome, exercising the real interface.

2. **`Response` has no token-count fields** (Phase 1/2 design — tokens arrive
   via `StreamChunk.Usage` on the Stream path). The Send-based Dispatch path
   therefore calls `cost.Account(model, 0, 0)` — pitfall 5's documented
   "no token counts" graceful path. The cost tracker's real dollar arithmetic
   is unit-tested directly in Plan 03-03 (calls `Account` with real token
   counts); full token-cost coupling lands when Dispatch drives Stream in
   Phase 4.

3. **`event.Bus.Publish(e Event)` is void** (not `(e Event) error` as the plan's
   intended interface assumed). Dispatch calls `bus.Publish(...)` directly.

4. **`Sem` is an interface, not `*provider.Semaphore`** (the concrete type) — so
   tests inject a recording stub. `*provider.Semaphore` satisfies the interface;
   production passes it unchanged.

5. **The capability gate is a per-candidate skip in Dispatch's walk** (the seam).
   Plan 03-04 additionally wires the filter into Resolve (pre-filtering the
   primary). The two layer cleanly.

## Self-Check: PASSED

- 10 dispatch tests + 4 events tests + 8 provider-error tests pass -race.
- Transport discipline: `grep -cE "os\.Stdout|fmt\.Print" dispatch.go` = 0
  (all diagnostics via `log/slog` → stderr, C1).
- Exhausted invariant: `grep -c "KindExhausted" internal/provider/errors.go` = 1
  (declaration only; ClassifyHTTP never references it).

## Evidence

- Structural stops the walk (D-04/N5): `TestDispatchStructuralStopsWalk` — a 401
  returns immediately, ZERO events, only the primary attempted.
- Fallback walk: `TestDispatchChainTwoFailuresThenSuccess` — 2 Transient failures
  → fallback-2 success, TWO events with correct From/To/Attempt.
- Capability seam: `TestDispatchCapabilitySeam` — a tool-less primary is skipped,
  only the capable fallback is called.
- Exhausted invariant: `TestClassifyExhaustedInvariant` loops status 100-599.
- Redaction: `TestProviderErrorRedactsCause` — `sk-`/`Bearer` tokens scrubbed.
