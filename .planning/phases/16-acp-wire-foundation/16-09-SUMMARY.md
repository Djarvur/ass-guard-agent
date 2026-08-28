---
phase: 16-acp-wire-foundation
plan: 09
subsystem: acp
tags: [acp, runtime, modelrouting, config-surface, chip-truth, resolver, tdd]

# Dependency graph
requires:
  - phase: 16-acp-wire-foundation (16-05, 16-08)
    provides: the ACP-08 advertisement (ConfigSurface.optionsLocked / resolveModelLocked), the effectiveModel live-apply seam (ApplyTurnModel + sessionFor stamp), and the WR-05 gap register this plan's 4b belongs to
  - phase: 14 (model scheduling)
    provides: schedCfg on the Runner, modelrouting.Resolver.Resolve, the 14-05 heavy-tier provider pick the default must stay consistent with
provides:
  - chip==wire invariant, pinned from BOTH sides: the runner's default turn model and the advertisement's bare-model currentValue resolve through the SAME chain (tier + time window + static-binding fallback)
  - Runner.defaultTurnModel — unexported resolver-then-static-binding ladder over schedCfg (nil schedCfg preserves the documented profile-slug default)
  - TestDefaultTurnModel_FollowsTierResolution (wire side: four subtests) and TestConfigAdvertisement_ResolverTruth (advertisement side: independent resolver evaluation)
affects: [18 (session reconciliation builds on one model source of truth), 20 (/model per-agent routing), phase-16 close]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 3700
  tasks: 2
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "defaultTurnModel ladder: resolver primary → static tier binding → empty, mirroring the advertisement's resolveModelLocked exactly (chip parity by construction)"

key-files:
  created:
    - internal/acpserve/chip_truth_test.go
  modified:
    - internal/runtime/runtime.go
    - internal/runtime/apply_model_test.go

key-decisions:
  - "CHIP-TRUTHFULNESS DECISION (recorded, implemented): the RUNNER's default follows the tier resolution — the D-11/D-12-consistent route. 'Chip follows runner' rejected (it would stamp runner state into entries advertising LAYER values — the _global twins, the pending options — incoherent with D-11); 'operator override' rejected (no operator disposition exists; the locked decisions entail the precedence chain as the single source of truth). Phases 18/20 build on this."
  - "The profile slug is OUT of the precedence chain: it sat outside D-11/D-12 (a leftover of the abandoned mimicry bar, pivoted 2026-08-25) and only governs when schedCfg is nil (the documented test-runner default)"
  - "The empty-session-tier fallback uses the string literal \"heavy\" in internal/runtime — modelrouting's and providerfactory's tierHeavy constants are unexported and not importable there; loader-produced schedCfg never has an empty tier (applyDefaults), so the literal only covers hand-built configs"

patterns-established:
  - "One source of truth for the wire model: any code that needs 'the model the next request carries' resolves through modelrouting (tier + time window + static-binding fallback), never reads a profile slug or a cached string"

requirements-completed: [ACP-08]

coverage:
  - id: D1
    description: "With no editor stamp, the model on the wire equals the tier-resolved config model; an explicit editor stamp keeps absolute precedence (D-12); nil schedCfg keeps the profile slug; a resolver decline falls back to the static binding exactly like the advertisement's resolveModelLocked"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "internal/runtime/apply_model_test.go#TestDefaultTurnModel_FollowsTierResolution"
        status: pass
      - kind: unit
        ref: "internal/runtime/apply_model_test.go#TestApplyTurnModel (existing pins pass unmodified)"
        status: pass
      - kind: unit
        ref: "internal/runtime/apply_model_test.go#TestApplyTurnModel_StampsFutureSessions (existing pin passes unmodified)"
        status: pass
    human_judgment: false
  - id: D2
    description: "The bare model advertisement entry is resolver-true by construction: its currentValue equals an independent modelrouting resolver evaluation over the same layer files, computed inside the test — and the phase's four wire packages stay fully green under -race with the new default"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "internal/acpserve/chip_truth_test.go#TestConfigAdvertisement_ResolverTruth"
        status: pass
      - kind: integration
        ref: "internal/acpserve/simulator_e2e_test.go#TestZedSimulatorE2E"
        status: pass
    human_judgment: false

# Metrics
duration: 9min
completed: 2026-08-28
status: complete
---

# Phase 16 Plan 09: Gap Closure — Chip truthfulness, one source of truth for the wire model (gap 4b) Summary

**The runner's default turn model now resolves through the tier chain (resolver → static binding), making the wire model equal the ACP-08 advertisement's currentValue by construction — the operator-observed pre-stamp divergence (turn-001 GLM-5.3 on the wire while the chip showed glm-5.2) cannot recur, and both sides are pinned by tests.**

## Performance

- **Duration:** 9 min
- **Started:** 2026-08-28T13:07:32Z
- **Completed:** 2026-08-28T13:16:22Z
- **Tasks:** 2 (Task 1 TDD RED→GREEN)
- **Files modified:** 3 (1 created)

## Accomplishments

- Gap 4b closed by implementation: `Runner.defaultTurnModel()` mirrors the advertisement's `resolveModelLocked` ladder exactly (resolver primary → static tier binding → empty), and `sessionFor` falls through to it whenever no editor stamp is set — the pre-stamp request model and the advertised currentValue now flow from ONE resolution
- Explicit editor stamps keep absolute precedence (D-12): the `effectiveModelFor()` read stays first in the stamp block; `ApplyTurnModel`/`SetTurnModel` and the turn-mutex serialization are untouched (all existing live-apply pins pass unmodified)
- The chip==wire invariant is enforced from both ends: the wire side fails if the profile slug ever rides out again with a schedCfg present; the advertisement side fails if the bare model entry ever drifts from an independent resolver evaluation over the same layer files

## Task Commits

Each task was committed atomically (Task 1 TDD RED→GREEN):

1. **Task 1: Runner default follows the tier resolution** — `1f21829` (test, RED) + `f4f0b69` (feat, GREEN)
2. **Task 2: Advertisement-side resolver truth + regression sweep** — `16c269e` (test)

## Files Created/Modified

- `internal/runtime/runtime.go` — `Runner.defaultTurnModel()` (resolver-then-static-binding ladder; nil schedCfg → ""; empty tier → the `"heavy"` load-floor literal); sessionFor's stamp block falls through to it; `effectiveModel` field doc now says "" = the tier-resolved config default
- `internal/runtime/apply_model_test.go` — `TestDefaultTurnModel_FollowsTierResolution` (four subtests: tier-resolution default, explicit-stamp precedence, nil-config profile default, resolver-decline static-binding fallback with a deterministically never-active malformed window); existing tests byte-identical (RED commit was pure insertion)
- `internal/acpserve/chip_truth_test.go` (NEW — deliberate: no files_modified overlap with 16-08) — `TestConfigAdvertisement_ResolverTruth`: project+global temp layers, the bare model entry's currentValue asserted equal to `modelrouting.Load` + `NewResolver.Resolve` computed in the test, plus a non-vacuity anchor on the concrete project-layer value

## Decisions Made

- **Chip-truthfulness decision (restated from the plan's recorded decision, as implemented — Phases 18/20 build on this):** the RUNNER's default follows the tier resolution. Grounding: D-11 locks that advertised options carry the EFFECTIVE value resolved through the precedence chain (the editor can never display stale truth); D-12 locks ONE chain (session override > time-window > project > global). The tier-resolved model IS the chain's answer; the profile slug sat outside the chain (a leftover of the abandoned mimicry bar). "Chip follows runner" was rejected — it would stamp runner state into entries that advertise LAYER values (the `_global` twins, the pending options), incoherent with D-11. "Operator override" was rejected — no operator disposition exists and the locked decisions entail the chain as the single source of truth.
- The default never rewires providers (T-16-09-01 mitigation): it rides the same heavy-tier resolution that picked the session provider (14-05); cross-provider targets keep the loud-degrade guards (the `resolveSubagentModel` precedent)
- Every decline path degrades down the documented ladder (T-16-09-03): a malformed time window or undeclared binding model yields the static binding, then the profile slug — never a failed turn (pinned by the malformed-window subtest)
- Task 2's simulator check found NO fixture alignment needed: the simulator's profile slug and the floor-resolved default coincide at GLM-5.3 and the SSE stub is turn-keyed, so `TestZedSimulatorE2E` passed unchanged

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Lint-gate compliance restructuring (Task 1)**
- **Found during:** Task 1 GREEN
- **Issue:** the strict golangci-lint v2 gate (funlen ≤60, noinlineerr) rejected the first-pass test layout
- **Fix:** the four subtest bodies extracted to named helper functions (the config_test.go precedent); `os.WriteFile`/`ApplyTurnModel` error checks moved to plain-assignment style
- **Files modified:** internal/runtime/apply_model_test.go
- **Verification:** `golangci-lint run internal/runtime/...` → 0 issues; the RED-asserted behavior unchanged
- **Committed in:** f4f0b69 (with Task 1 GREEN)

---

**Total deviations:** 1 auto-fixed (lint-gate compliance, behavior-preserving)
**Impact on plan:** None on behavior — every subtest asserted the same behavior before and after the restructuring.

## Issues Encountered

- None blocking. The RED run reproduced the gap exactly: both default-model subtests failed with the profile slug on the wire (`model-profile-slug` instead of `model-from-config` / `model-static-binding`), while the explicit-stamp and nil-config subtests passed — confirming the change touches only the unstamped default path.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- ALL five 16-VERIFICATION gaps are now closed (1-2: CR-01 in 16-07; 3+4a+5: WR-05 in 16-08; 4b: this plan) — Phase 16's code work is complete; the phase close still awaits the operator live-Zed confirmation (WINDOWS #11)
- Phases 18 (session reconciliation) and 20 (/model per-agent routing) inherit one documented source of truth for the wire model: resolve through the chain, never read a profile slug
- ROADMAP criterion 4's "true current values" now holds pre-stamp: the advertised value is the value the next request carries

---
*Phase: 16-acp-wire-foundation*
*Completed: 2026-08-28*

## Self-Check: PASSED

- Commits verified in git log: 1f21829 (RED), f4f0b69 (GREEN), 16c269e (Task 2)
- Files verified on disk: internal/runtime/runtime.go, internal/runtime/apply_model_test.go, internal/acpserve/chip_truth_test.go
- Existing 16-05 tests unmodified: the RED commit was pure insertion (170 insertions, 0 deletions in apply_model_test.go)
- Full verification re-run green: `go test -race -count=1 ./internal/runtime/ ./internal/acpserve/` (ok), `go test -race ./internal/acpserve/ -run TestZedSimulatorE2E -count=1` (ok), `go test -race -count=1 ./internal/acp/ ./internal/session/` (ok), `go vet ./internal/runtime/ ./internal/acpserve/` clean, `golangci-lint run internal/runtime/... internal/acpserve/...` → 0 issues
- TDD gate sequence: test commit (1f21829) precedes feat commit (f4f0b69); `grep -c defaultTurnModel internal/runtime/runtime.go` → 5 (≥2: definition + sessionFor call site)

