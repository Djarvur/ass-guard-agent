---
phase: 22-background-execution-sandbox-reality
plan: 07
subsystem: tasks
tags: [background-execution, tracker, subagent, panic-recovery, concurrency, tdd]

# Dependency graph
requires:
  - phase: 22-background-execution-sandbox-reality (22-01..22-06)
    provides: the tasks Tracker (D-10 cap/queue, PAR-07/PAR-08 notifications), RunBackgroundSubagent (progressive output file, registration), and the foreground D-13 recover precedent in internal/session/subagent.go
provides:
  - D-10 slots actually free on completion — the over-cap FIFO queue drains as promised (G-22-1 / CR-01)
  - Cancel-entry retirement on Complete — finished subagents stop looking running; CancelTask on a completed id is an idempotent no-op (the substrate 22-09's TaskStop/TaskOutput classifier consumes)
  - D-13 panic invariant on the background leg — a panicking background subagent is a contained, notified, slot-releasing error; the process survives (G-22-3 / CR-03)
affects: [22-background-execution-sandbox-reality (22-08, 22-09), internal/tasks consumers]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
# Same estimateTokens scale (chars/4 over the realized diff), never a harness token count.
actuals:
  tokens: 3952   # chars/4 over the realized diff (15,810 chars across 4 files)
  tasks: 2
  commits: 4     # MEASURED: git rev-list --count f655662..HEAD

# Tech tracking
tech-stack:
  added: []      # runtime/debug is stdlib; zero new deps
  patterns:
    - "release-then-admit slot ordering: Complete releases the finishing subagent's slot (floored decrement + cancel-entry delete) under t.mu BEFORE startNextWaiter admits exactly one waiter outside the lock"
    - "goroutine-boundary recover mirroring: the background launcher defer mirrors the foreground D-13 leg — w.finish(error marker + panic text + debug.Stack) then exactly one Tracker.Complete(ExitStatus=error)"
    - "subprocess-guard RED for background-goroutine panics: the outer test leg re-execs the test binary so an escaping panic surfaces as a clean outer assertion failure (a raw background-goroutine panic kills the process with no FAIL line)"

key-files:
  created: []
  modified:
    - internal/tasks/tracker.go
    - internal/tasks/tracker_test.go
    - internal/tasks/subagent.go
    - internal/tasks/subagent_test.go

key-decisions:
  - "releaseSubagentSlot is a callers-hold-t.mu helper invoked inside Complete's critical section (the plan's refinement of CR-01's self-locking sketch — Complete already holds the lock; startNextWaiter re-locks outside)"
  - "Task 2's RED used the plan-authorized subprocess guard: a panic escaping a background goroutine produces no --- FAIL line (verified empirically), so the guard converts the process crash into a clean outer assertion — also the only shape that yields valid TDD RED evidence (RED_EVIDENCE_OK via check tdd-red-evidence)"
  - "The recover defer sits directly after defer cancelFn(): LIFO runs the recover first during unwind, the panicked-past normal Complete is REPLACED by exactly one error notification, and cancelFn still runs on both paths"
  - "coreexec's CompletionHook fires bgKindBash only — subagent completions arrive solely via internal/tasks/subagent.go (normal + recover), so no double-release path exists (grep-verified)"

patterns-established:
  - "Floored-decrement release helpers guard defensive/ghost completions against negative counters"
  - "Go-test RED evidence records carry a faithful TAP translation of the --- FAIL lines so check tdd-red-evidence can classify them (target_test_failed)"

requirements-completed: [PAR-07]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "G-22-1 (CR-01): background-subagent slots are RELEASED on completion — after SubagentCap total starts in a session, RegisterSubagent still admits directly; the over-cap FIFO queue drains as slots free; Complete retires the finishing id's cancel registration (CancelTask on a completed id reports false)"
    requirement: PAR-07
    verification:
      - kind: unit
        ref: internal/tasks/tracker_test.go#TestTrackerCap_SlotsFreeAfterCompletion
        status: pass
      - kind: unit
        ref: internal/tasks/tracker_test.go#TestTrackerCap_SequentialReuseNeverQueues
        status: pass
      - kind: unit
        ref: internal/tasks/tracker_test.go#TestTrackerComplete_RetiresCancelEntry
        status: pass
      - kind: command
        ref: "go test -race -count=1 ./internal/tasks/ (full package, pre-existing battery included)"
        status: pass
    human_judgment: false
  - id: D2
    description: "G-22-3 (CR-03): a panicking background subagent NEVER kills the ass-guard process — the launcher goroutine recovers at the boundary, writes the terminal error marker (panic text + stack) to the output file, fires exactly ONE error notification, and releases its slot + cancel entry (D-13 invariant, background leg)"
    requirement: PAR-07
    verification:
      - kind: unit
        ref: internal/tasks/subagent_test.go#TestBackgroundSubagent_PanicRecovered
        status: pass
      - kind: command
        ref: "go test -race -count=1 ./internal/session/ -run 'TestDispatchBackground|TestSubagentPanicRecovery'"
        status: pass
    human_judgment: false

# Metrics
duration: 17 min
completed: 2026-09-10
status: complete
---

# Phase 22 Plan 07: Gap closure — internal/tasks cluster (G-22-1 slot release + G-22-3 panic recovery) Summary

**Tracker completions now free D-10 slots (floored decrement + cancel-entry retirement, release-then-admit) and the background-subagent goroutine recovers panics at the boundary (one error notification + marker + stack, process survives) — both TDD, red-verified then green**

## Performance

- **Duration:** 17 min
- **Started:** 2026-09-10T02:41:47Z
- **Completed:** 2026-09-10T02:59:04Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- **G-22-1 (CR-01) closed:** `Tracker.Complete` for `Kind==KindSubagent` now calls `releaseSubagentSlot(id)` under `t.mu` BEFORE `startNextWaiter()` — a floored decrement of `runningSubagents` plus `delete(t.subagentCancels, id)`. The monotonic counter that permanently queued every session past its first cap-full of dispatches is gone: the queue drains as promised, the count never exceeds the cap during handoff, and sequential reuse never queues.
- **Cancel-entry retirement (the 22-09 substrate):** finished subagents stop looking running — the map had no delete path (populated only at RegisterSubagent :172 and startNextWaiter :212), so `CancelTask` on a completed id now truthfully reports false.
- **G-22-3 (CR-03) closed:** the background launcher goroutine in `RunBackgroundSubagent` mirrors the foreground D-13 recover — `w.finish("error", panic text + debug.Stack())` then exactly ONE `Tracker.Complete` (Kind=KindSubagent, ExitStatus "error"). A panicking subagent is contained, notified, and slot-releasing instead of killing the ACP server, every session, every PTY.
- **Strictly additive:** FIFO order, cap notes, cancel-by-id, queued-over-cap dispatch all byte-stable — the full pre-existing internal/tasks battery and the session `TestDispatchBackground|TestSubagentPanicRecovery` rows stay green.

## Task Commits

Each task was committed atomically (TDD: RED test commit → GREEN implementation commit):

1. **Task 1 RED: tracker battery for slot release + cancel-entry retirement** - `6bed452` (test)
2. **Task 1 GREEN: releaseSubagentSlot in Complete (release-then-admit)** - `9e38160` (feat)
3. **Task 2 RED: background-subagent panic-recovery test (subprocess guard)** - `3201cf0` (test)
4. **Task 2 GREEN: recover at the launcher goroutine boundary** - `6477366` (feat)

**Plan metadata:** see the docs(22-07) commit following this summary.

## TDD Gate Compliance

Both tasks executed full RED→GREEN cycles under `tdd_mode: true`:

| Task | RED commit | RED evidence | GREEN commit | Gate |
|------|-----------|--------------|--------------|------|
| 1 (G-22-1) | `6bed452` test(22-07) | `RED_EVIDENCE_OK` (target_test_failed; exit 1, all 3 target tests failing on assertions: count monotonic 3≠2/≠0, re-register queued=true, cancels retained, CancelTask=true) | `9e38160` feat(22-07) | PASS |
| 2 (G-22-3) | `3201cf0` test(22-07) | `RED_EVIDENCE_OK` (target_test_failed; exit 1, outer guard leg failed cleanly with the inner crash captured: panic escaping subagent.go:80 dep.Run) | `6477366` feat(22-07) | PASS |

REFACTOR: not needed — both GREEN implementations are minimal and the batteries pass as written.

## Files Created/Modified

- `internal/tasks/tracker.go` — Complete calls releaseSubagentSlot (floored decrement + cancel-entry delete) before startNextWaiter; D-10 doc updated with the release-then-admit ordering
- `internal/tasks/tracker_test.go` — TestTrackerCap_SlotsFreeAfterCompletion, TestTrackerCap_SequentialReuseNeverQueues, TestTrackerComplete_RetiresCancelEntry + countRunning/cancelKnown same-package seams
- `internal/tasks/subagent.go` — recover defer at the launcher goroutine boundary (panic text + debug.Stack via w.finish, exactly one error Complete); runtime/debug import
- `internal/tasks/subagent_test.go` — TestBackgroundSubagent_PanicRecovered with the subprocess guard (panicGuardEnv)

## Decisions Made

- **releaseSubagentSlot shape:** callers-hold-`t.mu` helper invoked inside Complete's critical section — the plan's refinement of CR-01's sketch (which had the helper self-lock and call startNextWaiter itself); Complete already holds the lock, and startNextWaiter must run outside it.
- **Subprocess guard for Task 2's RED (plan-authorized option):** an escaping background-goroutine panic kills the test process with no `--- FAIL:` line (verified empirically), which would classify as INVALID_RED; the guard's outer leg re-execs the test binary and converts the crash into a clean assertion failure — valid RED evidence AND a better long-term test (the crash is captured, not just suffered).
- **No extra slot/cancel bookkeeping in the panic path:** the recover-path `Complete` lands in Task 1's releaseSubagentSlot automatically (plan's design confirmed by the green count/cancel assertions in the panic test).
- **Verified no double-release path:** coreexec's CompletionHook fires `bgKindBash` only; subagent completions arrive solely from internal/tasks/subagent.go (normal + recover legs).

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None. (Pre-existing, out of scope, not touched: `gofmt -l` flags comment-alignment quirks in internal/tasks/{notify.go,notify_test.go,tracker.go} — the flagged regions predate this plan; the known TestRescanConcurrency race and TestPermissionsE2E regression are tracked in deferred-items.md/STATE.md as Phase-22-not-ours.)

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- G-22-1 and G-22-3 are closed with named regression batteries that were red against the pre-fix code (re-verification mapping: G-22-1 → the three tracker tests; G-22-3 → TestBackgroundSubagent_PanicRecovered).
- 22-08 (cron_wiring/runtime lifecycle cluster, G-22-2 + G-22-4) and 22-09 (coreexec TaskStop/TaskOutput seam, G-22-5) can proceed; 22-09's classifier consumes the finished-vs-running signal this plan landed (cancel-entry retirement).
- PAR-07 stays open until 22-08/22-09 (its co-declaring siblings) complete — handled by the requirements.ready-ids shared-ID gate, not marked complete by this plan.

## Self-Check: PASSED

All 4 modified source/test files exist on disk; all 4 task commits (6bed452, 9e38160, 3201cf0, 6477366) exist in git log; TDD gate commits verified (`test(22-07)` × 2 precede `feat(22-07)` × 2); every acceptance criterion re-run green in one consolidated battery (new rows, pre-existing tracker battery, grep counts 1/1/1, panic test, full package, session rows, vet, static build).

---
*Phase: 22-background-execution-sandbox-reality*
*Completed: 2026-09-10*
