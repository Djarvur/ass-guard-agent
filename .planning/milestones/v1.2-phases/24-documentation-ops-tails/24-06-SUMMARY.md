---
phase: 24-documentation-ops-tails
plan: "06"
subsystem: testing
tags: [ecos-04, wake-turn, modes-matrix, gap-closure, modelrouting, breaker-demotion, provider-filter, tail-01, tail-03]

requires:
  - phase: 24-documentation-ops-tails (plans 24-02, 24-05)
    provides: the replayed-breaker demotion walks this plan provider-filters, and the ECOS-04 modes-matrix harness whose wake row this plan makes real
  - phase: 22-background-execution-sandbox-reality
    provides: the live wake machinery the three drivers exercise (backgroundLaunch seam, scheduleWakeDrain, wakeDrainChain, drainWakeNotifications)
provides:
  - Three REAL ECOS-04 wake drivers (wake x commands/skills/hooks) through the live phase-22 wake chain — ECOS-04 is 12/12 cells exercised with zero precondition marks
  - The false-premise correction set: 24-05-SUMMARY wake claims amended + dated correction paragraph, WINDOWS #29 resolved, corrected test-file comment
  - Provider-filtered breaker demotion at BOTH resolve sites (resolveSubagentModel + demoteIfDeniedLocked) with the cross-provider-PRIMARY guard — the no-silent-wrong-wire contract enforced, pinned by cross-provider rows that were red against the defect
affects: [Phase 25 executors (kit extraction inherits the closed gaps), verify-work re-verification of 24-VERIFICATION gaps 1-2, ship gate (WINDOWS #29 resolved)]

actuals:
  tokens: 13589   # chars/4 over the realized diff (54358 chars, 9 files)
  tasks: 2
  commits: 4      # measured: git rev-list --count c822290..HEAD

tech-stack:
  added: []
  patterns:
    - "Caller-side provider filtering before FirstAllowed: the provider constraint is a CALLER property (which wire the result rides) and both call sites already hold the provider slug — chosen over a provider-aware FirstAllowed variant that would change modelrouting's exported contract for two callers"
    - "Excluded-primary pin: a test row that PASSES at HEAD by design and pins the corner a fix must not regress (cross-provider primary + non-empty breaker map = byte-identical, zero notes) — the regression-pin twin of a RED row"

key-files:
  created:
    - .planning/phases/24-documentation-ops-tails/24-06-task1-red-evidence.json
    - .planning/phases/24-documentation-ops-tails/24-06-task2-red-evidence.json
  modified:
    - internal/runtime/modesmatrix_wake_test.go
    - internal/runtime/runtime.go
    - internal/runtime/apply_model_test.go
    - internal/acpserve/config_surface.go
    - internal/acpserve/config_surface_outcomes_test.go
    - .planning/phases/24-documentation-ops-tails/24-05-SUMMARY.md
    - .planning/WINDOWS.md

key-decisions:
  - "Task 1's RED is the committed three-loud-skip state captured as evidence (skip lines + count 3) — the plan's own design; the skip premise IS the defect, so the RED record documents the loudness the task deletes rather than a target_test_failed shape"
  - "Assertion-shape substitutions (both stated in the corrected file comment AND the 24-05 correction paragraph): commands/skills prove the invocation rides the wake-triggering dispatch verbatim (renderWakeBlocks' machine-composed input + first-text-block start-anchored expansion makes in-wake expansion structurally impossible), one real wake turn, woken model saw the notification; hooks proves PreToolUse inside the wake turn + the notification's kind/task-id linkage — never a skip, never a fabricated green"
  - "SubagentStop-on-background is a deliberate descope: it fires only inside the foreground DispatchSubagent wrapper (subagent.go:150); the background leg calls Runner.Run directly — firing it there would be a production change (a NEW gap), so the cell asserts the exhibited completing-dispatch observable instead"
  - "The minted task id is observed through its exhibited path: the client turn's async_launched tool-result payload (BackgroundDispatchResult.TaskID) joined against the wake notification block's task_id line"
  - "demoteIfDeniedLocked gains the cross-provider-PRIMARY guard BEFORE the walk (the surface twin of runtime's own :3156 guard): a primary whose breaker was never consulted is never claimed — the corner stays byte-identical with zero notes, pinned by surface row 3"

requirements-completed: [TAIL-03, TAIL-01]

coverage:
  - id: D1
    description: "Three real ECOS-04 wake drivers (G-24-1): wake x commands/skills/hooks run through the live phase-22 machinery — a completing background subagent (Task run_in_background, the backgroundLaunch seam) drives scheduleWakeDrain -> wakeDrainChain -> drainWakeNotifications -> runOneTurn; exactly one wake turn, wake:tasks provenance, typed prompt preserved, zero PRECONDITION-UNMET lines"
    requirement: TAIL-03
    verification:
      - kind: unit
        ref: "internal/runtime/modesmatrix_wake_test.go#TestModesMatrixWakeCommands"
        status: pass
      - kind: unit
        ref: "internal/runtime/modesmatrix_wake_test.go#TestModesMatrixWakeSkills"
        status: pass
      - kind: unit
        ref: "internal/runtime/modesmatrix_wake_test.go#TestModesMatrixWakeHooks"
        status: pass
      - kind: other
        ref: "command: go test -race ./internal/runtime/ -run TestModesMatrixWake -v | grep -c PRECONDITION-UNMET == 0; per-surface spacing-tolerant existence greps (MODES-MATRIX wake x commands/skills/hooks pass) -> THREE-WAKE-PASSES; full TestModesMatrix trio green across acpserve/session/runtime"
        status: pass
    human_judgment: false
  - id: D2
    description: "False premise corrected everywhere it lived (G-24-1 honesty): the corrected test-file comment, 24-05-SUMMARY wake-claim amendments (artifact bullet, honesty bullet, 4x3 table wake row, closing WINDOWS note, readiness bullets) plus the dated 24-06 correction paragraph restating both assertion-shape substitutions, and WINDOWS entry #29 resolved with a resolution note naming 24-06"
    requirement: TAIL-03
    verification:
      - kind: other
        ref: "command: grep -c '^| 29 .*| open |' .planning/WINDOWS.md == 0 (entry fixed, resolution names 24-06); comment-stripped greps on the wake file return 0 for SkipPrecondition and the PLANNED-BUT-UNEXECUTED token; 24-05-SUMMARY carries the '## Correction (2026-09-10, plan 24-06)' section with both substitution restatements"
        status: pass
    human_judgment: false
  - id: D3
    description: "Provider-filtered breaker demotion at BOTH resolve sites (G-24-2, CR-01+WR-01): an open session-provider primary demotes only onto a same-provider fallback; a fully-denied same-provider chain keeps the primary with one truthful note naming the provider constraint; the excluded-primary corner (cross-provider primary + non-empty map) keeps the primary byte-identically with zero notes; empty breaker maps and all pre-existing subtests unchanged"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "internal/runtime/apply_model_test.go#TestResolveSubagentModelCrossProviderDemotion"
        status: pass
      - kind: unit
        ref: "internal/acpserve/config_surface_outcomes_test.go#TestOutcomeBreakersCrossProviderDemotion"
        status: pass
      - kind: unit
        ref: "internal/acpserve/config_surface_outcomes_test.go#TestOutcomeBreakersExcludedPrimaryCorner"
        status: pass
      - kind: other
        ref: "command: go test -race ./internal/runtime/ -skip TestRescanConcurrency && go test -race ./internal/acpserve/ -skip TestPermissionsE2E && go vet both packages && GOOS=linux CGO_ENABLED=0 go build ./... -> ALL-SIX-GREEN (r1..r6=0)"
        status: pass
    human_judgment: false

duration: 19 min
completed: 2026-09-10
status: complete
plan_head_before: c8222904a9f701c3d357c7fc3da4f2159515508b
---

# Phase 24 Plan 06: Gap Closure Summary

**Gap closure G-24-1 + G-24-2: three real ECOS-04 wake drivers over the live phase-22 machinery (zero precondition marks, false premise corrected in every artifact) and provider-filtered breaker demotion at both resolve sites (the no-silent-wrong-wire contract enforced, pinned by rows that were red against the defect)**

## Performance

- **Duration:** 19 min
- **Started:** 2026-09-10T18:46:57Z
- **Completed:** 2026-09-10T19:06:32Z
- **Tasks:** 2 (both tdd-marked; Task 2 RED -> GREEN)
- **Files modified:** 9 (2 created, 7 modified)

## TDD Gate Compliance

- **Task 1 (G-24-1):** the committed RED is the pre-existing three-loud-skip state (24-05 `f678e8b`) — captured as `24-06-task1-red-evidence.json` (three `--- SKIP` subtests, three PRECONDITION-UNMET lines, count 3; the skip premise IS the defect). GREEN: `cba127a` (feat) — the three real drivers, all green under `-race` from the introducing commit; the plan designates this skip-shaped RED explicitly (an assertion-failure RED is impossible against stubs that skip).
- **Task 2 (G-24-2) RED:** `3d95dc5` (test) — 4 target rows fail at HEAD returning/advertising `minimax-m3` (the cross-provider slug — the wrong-wire demonstrated); evidence `24-06-task2-red-evidence.json`. `TestOutcomeBreakersExcludedPrimaryCorner` passes at RED BY DESIGN (the excluded-primary regression pin, documented in the record).
- **Task 2 (G-24-2) GREEN:** `f926747` (feat) — provider-filtered walks at both sites; every RED row green, every pre-existing row green unmodified.
- **REFACTOR:** not needed — the GREEN diff is minimal by design (two walk constructions + two note rewords + doc comments).

## Accomplishments

- **G-24-1 (TAIL-03):** the three wake cells now drive the REAL chain — a client turn's scripted Task tool call with `run_in_background` enters the backgroundLaunch seam, the nested loop completes, `Tracker.Complete` schedules the drain, and exactly ONE wake turn fires through `runOneTurn` with the `wake:tasks` provenance. The runtime registry records all three wake cells as pass; the wake file's run output carries ZERO precondition lines (the old count gate inverted).
- **G-24-1 honesty:** every artifact that repeated the false "phase 22 unexecuted" premise now states the true one (43a8378 / 5ad02a0 / a59baf9 are ancestors of phase-24 base 5552942; 24-VERIFICATION gap 1 cited) — corrected test-file comment, amended 24-05-SUMMARY claims + dated correction paragraph restating both assertion-shape substitutions, WINDOWS #29 fixed.
- **G-24-2 (TAIL-01, CR-01+WR-01):** replayed breaker evidence can no longer demote a subagent or an advertised session model onto a provider it does not ride — the runtime walk filters to `sessionProvider`, the surface walk filters to `s.providerName` behind a cross-provider-PRIMARY guard that never claims an unconsulted breaker (byte-identical corner, pinned by surface row 3).

## Task Commits

1. **Task 1 (G-24-1): wake row runs for real + premise corrections** — `cba127a` (feat)
2. **Task 2 (G-24-2) RED: cross-provider rows failing at both sites** — `3d95dc5` (test)
3. **Deviation fix: wake EngineDecision write-order race** — `cde0b80` (fix)
4. **Task 2 (G-24-2) GREEN: provider-filtered demotion walks** — `f926747` (feat)

**Plan metadata:** (final docs commit follows)

## Files Created/Modified

- `internal/runtime/modesmatrix_wake_test.go` — the three real wake drivers + shared harness (wakeMatrixDrive, wakeDispatchedTaskID, assertWakeTurnFiredOnce) over the existing newMatrixRunner/MountFixture/lenses
- `internal/runtime/runtime.go` — resolveSubagentModel's demotion walk provider-filtered + keep-note reworded to name the session provider; doc comment extended
- `internal/acpserve/config_surface.go` — demoteIfDeniedLocked: cross-provider-PRIMARY guard + s.providerName-filtered walk + keep-note reword; doc comment extended
- `internal/runtime/apply_model_test.go` — crossProviderLightConfig fixture + TestResolveSubagentModelCrossProviderDemotion (rows 1-2)
- `internal/acpserve/config_surface_outcomes_test.go` — crossProviderHeavyLayer/excludedPrimaryLayer fixtures + crossProviderBreakers multi-model seeder + TestOutcomeBreakersCrossProviderDemotion (rows 1-2) + TestOutcomeBreakersExcludedPrimaryCorner (row 3 pin)
- `.planning/phases/24-documentation-ops-tails/24-06-task1-red-evidence.json` — the three-skip RED record
- `.planning/phases/24-documentation-ops-tails/24-06-task2-red-evidence.json` — the wrong-wire RED record
- `.planning/phases/24-documentation-ops-tails/24-05-SUMMARY.md` — wake claims corrected + dated correction paragraph
- `.planning/WINDOWS.md` — entry #29 fixed with the 24-06 resolution note

## Decisions Made

See key-decisions in the frontmatter. All five follow the plan's letter (checker-verified designs: notification-observable for the hooks cell, spacing-tolerant greps, cross-provider-PRIMARY guard, exit-code-captured gates).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Wake EngineDecision write-order race in the Task 1 driver**
- **Found during:** Task 2 (the full-package `-race` battery surfaced it; the targeted wake runs had passed)
- **Issue:** `drainWakeNotifications` appends the wake turn's EngineDecision line AFTER the turn's user message; the driver asserted the count on first sight of the user text and could read 0 — a genuine write-order race in MY Task 1 test, not a production defect (flaked once as `EngineDecision lines with wake provenance = 0`).
- **Fix:** bounded `waitFor` poll on the EngineDecision count before asserting (the wake_wiring battery's own discipline); notification assertions unaffected.
- **Files modified:** internal/runtime/modesmatrix_wake_test.go
- **Verification:** the full battery re-ran green (`r3=0`); the wake battery re-ran green 3x after the fix; the final plan-level verify re-ran WAKE-FINAL-GREEN.
- **Committed in:** cde0b80

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** A test-only race fix in this plan's own driver; no production change, no scope creep. No other deviations — the plan executed exactly as written.

## Issues Encountered

None beyond the deviation above. The full-package gates carry only the two documented pre-existing skips (`TestRescanConcurrency` — phase-22 deferred-items.md; `TestPermissionsE2E` — STATE Blockers, Phase-23 commit 40b2bbc), both excluded per the plan's verify block.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Both Phase 24 verification gaps are closed: G-24-1 (the verifier's spot-check commands re-run: the wake substrate battery green; the wake-cell PRECONDITION-UNMET grep now returns ZERO, not three) and G-24-2 (cross-provider demotion pinned red-then-green at both resolve sites). The re-verification mapping: G-24-1 -> truths 1-3, G-24-2 -> truths 4-5.
- ECOS-04 is 12/12 cells functional with an honest evidence record; TAIL-03's "every interaction mode" claim now rests on drivers, not skips.
- No warning-classified review item was touched (WR-02/03/04/05/06/07, IN-01..06 stay deferred review debt per the plan's scope guard); production code changed only in the two demotion walks.

## Self-Check: PASSED

- All key files exist on disk (9/9).
- All four commits verified in git log (cba127a, 3d95dc5, cde0b80, f926747); measured commit count from the ledger base c822290 = 4.
- Plan-level verification re-run: THREE-WAKE-PASSES (zero precondition lines, three per-surface pass rows), matrix trio green, ALL-SIX-GREEN (both targeted batteries, both package gates past the documented skips, vet, CGO_ENABLED=0 build).
- Filter greps: `Provider == sessionProvider` exactly once at runtime.go:3189 (inside the demotion block); `Provider == s.providerName` exactly once at config_surface.go:794 (inside demoteIfDeniedLocked); the pre-existing :3161/:1348 guards and the new :787 guard are separate negative guards.

---
*Phase: 24-documentation-ops-tails*
*Completed: 2026-09-10*
