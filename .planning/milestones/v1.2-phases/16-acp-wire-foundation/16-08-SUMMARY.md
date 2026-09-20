---
phase: 16-acp-wire-foundation
plan: 08
subsystem: acp
tags: [acp, acpserve, config-surface, modelrouting, scope-routing, idempotence, tdd]

# Dependency graph
requires:
  - phase: 16-acp-wire-foundation (16-04, 16-05)
    provides: the ConfigSurface (menu/Set/persist/apply seams), the two operator layer files (D-08), WriteLayerOption (D-07), the D-10 blob fills-unset boundary, and the WR-05 defect this plan closes
provides:
  - scope-aware Set idempotence: global-scoped writes compare against the ADDRESSED layer (globalOnlyResolvedLocked), never silently swallowed when the combined value matches (gaps 3+5)
  - layer-true _global twins: _global/model and _global/tier advertise the global layer's own resolved values, with embedded-floor fallback and a fills-unset boundary against blobFills (gap 4a)
  - floorResolvedLocked helper (embedded-floor degradation, reused by the optionsLocked error arm)
affects: [16-09 (gap 4b chip truthfulness), 18 (load/resume reuse configOptionsFor), phase 20 per-agent-model]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 3935
  tasks: 2
  commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "addressed-layer resolution: any scope-aware comparison/display resolves the addressed layer alone (globalOnlyResolvedLocked), never the combined project-won view"
    - "typed result struct to escape the nonamedreturns × gocritic unnamedResult conflict"

key-files:
  created: []
  modified:
    - internal/acpserve/config_surface.go
    - internal/acpserve/config_test.go

key-decisions:
  - "Set's idempotence basis is scope-aware: project scope keeps the combined comparison (D-10 anti-promotion guard untouched); global scope compares against the global layer ALONE via globalOnlyResolvedLocked — no project layer, no blob overlay"
  - "The _global twins resolve as one coherent global-layer view: tier twin = the global layer's session_tier, model twin = that tier's binding in the global layer; blobFills never reach the twins (they describe a layer FILE)"
  - "Degradation ladder for the twins: global layer → embedded floor (loud log) → combined values (last resort so the menu stays whole); floor fallback extracted to floorResolvedLocked and reused by the existing optionsLocked error arm"

patterns-established:
  - "Addressed-layer basis: scope-aware guards/ads must load the addressed layer alone (modelrouting.Load over the single path, existence-filtered; embedded floor when absent)"
  - "Typed result struct over multi-same-type returns — the repo's nonamedreturns and gocritic unnamedResult rules conflict; a small struct satisfies both"

requirements-completed: [ACP-08]

coverage:
  - id: D1
    description: "A _global/-scoped set whose value equals the combined effective value but differs from the global layer's own value persists into the global layer; a global-scope re-push of a value the addressed layer already holds stays a logged no-op with zero file churn; project-scope D-10 pins pass unmodified"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "internal/acpserve/config_test.go#TestScopeRouting_GlobalWritePersistsWhenCombinedMatches"
        status: pass
      - kind: unit
        ref: "internal/acpserve/config_test.go#TestSetIdempotent_BlobDerivedEffective"
        status: pass
      - kind: unit
        ref: "internal/acpserve/config_test.go#TestSetIdempotent_ExplicitFileEffective"
        status: pass
    human_judgment: false
  - id: D2
    description: "The _global/model and _global/tier twins advertise the global layer's own resolved values (combined values only on the bare options; embedded floor when no global file exists; never a blob fill)"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "internal/acpserve/config_test.go#TestConfigSurface_GlobalTwinsAdvertiseGlobalLayer"
        status: pass
      - kind: integration
        ref: "internal/acpserve/simulator_e2e_test.go#TestZedSimulatorE2E"
        status: pass
    human_judgment: false

# Metrics
duration: 12min
completed: 2026-08-28
status: complete
---

# Phase 16 Plan 08: Gap Closure — Scope-aware config surface semantics (WR-05) Summary

**ConfigSurface scope semantics fixed: global-scoped sets compare against the addressed global layer (silent-swallow closed), and the _global model/tier twins advertise the global layer's own values instead of the project-won combined view — closing 16-VERIFICATION gaps 3, 4a, and 5 at one root cause.**

## Performance

- **Duration:** 12 min
- **Started:** 2026-08-28T12:47:11Z
- **Completed:** 2026-08-28T12:59:55Z
- **Tasks:** 2 (both TDD RED→GREEN)
- **Files modified:** 2

## Accomplishments

- Set's idempotence guard is scope-aware (gaps 3+5): a `_global/-scoped` write whose value equals the COMBINED (project-won) effective value but differs from the GLOBAL layer's own value now persists into the global layer; the guard fires only when the ADDRESSED layer genuinely holds the value
- The `_global/model` and `_global/tier` twins are layer-true (gap 4a): built from `globalOnlyResolvedLocked()` (global layer alone, no blobFills), with embedded-floor fallback for the absent-global-file case and a loud log on unresolvable layers
- D-10's anti-promotion guard on the default (Zed re-push) scope is preserved unmodified — a project-scoped set equal to the combined effective value remains a logged no-op with zero file churn (both existing `TestSetIdempotent_*` pins pass untouched)

## Task Commits

Each task was committed atomically (TDD RED→GREEN):

1. **Task 1: Scope-aware idempotence** — `0ddbf2e` (test, RED) + `8c7929f` (feat, GREEN)
2. **Task 2: Global twins advertise the global layer** — `e064df3` (test, RED) + `9006708` (feat, GREEN)

## Files Created/Modified

- `internal/acpserve/config_surface.go` — `globalOnlyResolvedLocked` (global-layer-alone resolution), `idempotenceBasisLocked` (typed `idempotenceBasis` result: value + log name), scope-aware Set guard, layer-true twins in `optionsLocked`, `floorResolvedLocked` extraction (reused by the existing error arm)
- `internal/acpserve/config_test.go` — `TestScopeRouting_GlobalWritePersistsWhenCombinedMatches` (persist-when-different + true-idempotence-no-churn subtests), `TestConfigSurface_GlobalTwinsAdvertiseGlobalLayer` (model twin, tier twin, absent-global-file floor + fills-unset boundary subtests); subtest bodies extracted to helpers for funlen

## Decisions Made

- The twins resolve as ONE coherent global-layer view: tier twin = the global layer's `session_tier`, model twin = that tier's binding in the global layer (matches how a real layer read behaves; pinned by the tier-twin subtest)
- Degradation ladder for the twins on an unresolvable global layer: log loudly → embedded floor → combined values as last resort so the eight-entry menu stays whole (a floor failure would otherwise return an empty advertisement)
- Blob fills never reach the twins: the twins describe a layer FILE; the blob channel stays on the bare options (pinned by the absent-global-file subtest's blob assertion)
- D-09 typed rejection unchanged: `validateSettableLocked` still runs against the COMBINED config before any scope routing (T-16-08-03 — the scope-aware basis opens no new write surface)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Lint-gate compliance restructurings**
- **Found during:** Task 1 and Task 2 GREEN
- **Issue:** the repo's strict golangci-lint v2 gate (funlen ≤60, lll ≤120, noinlineerr, nonamedreturns × gocritic unnamedResult conflict) rejected the first-pass implementations
- **Fix:** Set's basis computation extracted to `idempotenceBasisLocked` returning a typed `idempotenceBasis` struct; `os.Stat` moved to plain-assignment style; test subtest bodies extracted to named helpers; one long Errorf line wrapped
- **Files modified:** internal/acpserve/config_surface.go, internal/acpserve/config_test.go
- **Verification:** `golangci-lint run internal/acpserve/...` → 0 issues; full behavior pinned by the unchanged RED tests
- **Committed in:** 9006708 (with Task 2 GREEN)

---

**Total deviations:** 1 auto-fixed (lint-gate compliance, behavior-preserving)
**Impact on plan:** None on behavior — every RED test asserted the same behavior before and after the restructuring.

## Issues Encountered

- None blocking. The RED runs reproduced both predicted defects exactly: the silent swallow (global file kept glm-5.2, one "idempotent" log line) and the combined-value twins (subtest 3 additionally showed the blob tier fill leaking `""` into the twin's combined resolution — now impossible by construction).

## ACP-08 Concurrency Assumption (resolved by construction, per plan)

Concurrent/interleaved Set calls serialize through `s.mu` (unchanged); the new addressed-layer reads happen under that same lock (no torn read); a failed persist leaves both layer files byte-identical, returns the typed `acp.ConfigPersistError`, and touches no live state (D-07) — re-pinned by the full-package `-race` run (ok) including `TestConfigSurface_ConcurrentMutation`.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Gaps 3, 4a, 5 closed at the surface: the set channel and the display channel now share one scope-aware resolution basis
- Remaining from 16-VERIFICATION: gap 4b (pre-stamp Model chip truthfulness: runner default vs advertisement) — DIFFERENT artifact (`internal/runtime/runtime.go`), owned by plan 16-09 per this plan's scope discipline
- Phase 18 can reuse `configOptionsFor`/`optionsLocked` for load/resume with layer-true global twins for free

---
*Phase: 16-acp-wire-foundation*
*Completed: 2026-08-28*

## Self-Check: PASSED

- Commits verified in git log: 0ddbf2e, 8c7929f, e064df3, 9006708
- Files verified on disk: internal/acpserve/config_surface.go, internal/acpserve/config_test.go
- Full verification re-run green after the final task: `go test -race -count=1 ./internal/acpserve/` (ok), `go test -race ./internal/acpserve/ -run TestZedSimulatorE2E -count=1` (ok), `go test ./internal/providerfactory/ ./internal/modelrouting/ -run 'TestConfigWrite|TestSessionTier'` (ok), `golangci-lint run internal/acpserve/...` (0 issues)
- TDD gate sequence: test commit precedes feat commit for both tasks (0ddbf2e→8c7929f, e064df3→9006708)
