---
phase: 22-background-execution-sandbox-reality
plan: 01
subsystem: background-execution
tags: [task-notifications, wake-turn, background-bash, concurrency, automation-rails]

requires:
  - phase: 16-acp-wire-foundation
    provides: TurnEmitter rails + EngineDecision transcript vocabulary the wake turn rides
  - phase: 12 (coreexec background)
    provides: TaskRegistry (start/output/stop/reap) the CompletionHook extends
provides:
  - internal/tasks package — the ONE task-notification subsystem (Kind enum, D-02 Notification, Tracker with pending queue, coalescing, subagent cap/FIFO)
  - TaskRegistry.CompletionHook — primitive-arg terminal-transition callback (no coreexec→tasks import)
  - drainWakeNotifications + wakeDrainChain — the wake-turn drain riding sessionTurnMu TryLock + runOneTurn with wake:tasks provenance
  - Session-close queued-cancel with counted stderr note (OQ5)
affects: [22-02, 22-03, 22-04, 22-06, background-execution, sandbox]

tech-stack:
  added: []
  patterns:
    - "wake drain chain: per-session CAS-deduplicated retry loop — busy turn leaves pending (D-01 fallback), one coalesced batch per drain (D-03)"
    - "completion-time-ordered pending queue with task-id dedupe (first terminal wins)"

key-files:
  created:
    - internal/tasks/tracker.go
    - internal/tasks/notify.go
    - internal/tasks/tracker_test.go
    - internal/tasks/notify_test.go
    - internal/runtime/wake_wiring_test.go
  modified:
    - internal/coreexec/background.go
    - internal/coreexec/background_test.go
    - internal/runtime/runtime.go
    - internal/runtime/cron_wiring.go
    - internal/redact/redact.go
    - internal/redact/redact_test.go

key-decisions:
  - "SetDrain(fn func(pending []Notification)) is the drain-ATTEMPT callback: pending is a non-destructive peek; the runner's chain owns TryLock and consumes via Drain() (the only destructive consumer) — the busy-turn path can therefore leave notifications pending by construction"
  - "wakeDrainChain: one CAS-deduplicated retry chain per session (500ms busy-retry default, wakeRetryInterval field) — closes the D-01-fallback delivery gap when no scheduler tick exists (schedule-less serves); scheduler tick also retries"
  - "skRe redaction gained a leading \\b (word boundary) — without it the wake block's <task-notification> markup was mangled to <ta[REDACTED]> by the mid-word sk- token match (Rule-2 deviation fix, pinned by TestRedact_SkTokenWordBoundary)"
  - "Notification block markup: one <task-notification> element per notification with D-02 fields as labeled lines + <tail> section; wake provenance vocabulary wake:tasks (CONTEXT discretion documented in cron_wiring.go)"

patterns-established:
  - "Kind-detection discipline: notifications discriminate by the Kind struct field, never output text-matching (PAR-07)"
  - "Tracker.Enqueue truncates tails at TailBytes on a UTF-8 rune boundary (straddling rune dropped whole)"

requirements-completed: [PAR-07, PAR-08]

duration: 95 min
completed: 2026-09-07T21:10:00Z
---

# Phase 22 Plan 01: Task-Notification Subsystem + Wake-Turn Drain Summary

**Background Bash completions now wake the model through one kind-tagged notification subsystem: coalesced wake turns ride the existing automation rails (TryLock + runOneTurn), a busy client turn is never interrupted, and subagent registrations are capped at 8 with FIFO queueing.**

## Performance

- **Duration:** 95 min
- **Started:** 2026-09-07T19:35:00Z
- **Completed:** 2026-09-07T21:10:00Z
- **Tasks:** 3
- **Files modified:** 11

## Accomplishments

- internal/tasks package: Kind enum (bash/subagent), D-02 Notification struct, Tracker with completion-time-ordered pending queue (task-id dedupe, first-terminal-wins), rune-boundary tail truncation (8192 default), subagent cap 8 + FIFO queue (bound 64) with queued-note form, CancelTask, CancelQueued (OQ5), SetDrain/Drain/PendingPeek.
- TaskRegistry.CompletionHook: primitive-arg callback (taskID, kind, exitStatus, duration, tail, outputFile) fired exactly once per task from the Wait goroutine's terminal transition; exit status derived from waitErr ("0"/decimal/128+signal/"killed"/"error"); nil hook is a safe no-op.
- Wake drain (cron_wiring.go): drainWakeNotifications copies runAutomationTurn's skeleton (sessionFor nil-check → TryLock → provenance bracket → runOneTurn → AppendEngineDecision with wake:tasks) with the D-01 difference: failed TryLock LEAVES PENDING (never queues); the per-session CAS-deduplicated retry chain (500ms cadence) plus the scheduler tick form the retry path; SetTurnOriginAutomation brackets the wake turn (17-D-07 ask-decline parity).
- runtime.go sessionFor: tracker constructed beside the TaskRegistry, CompletionHook adapter (kind maps to tasks.Kind), SetDrain → scheduleWakeDrain, OnClose gains CancelQueued with a counted stderr note.
- E2E tracer (TestWakeTurn_BackgroundBashCompletion): client turn starts run_in_background Bash → exactly ONE wake turn with all D-02 fields (exec_ id, kind: bash, exit_status: 0, .ass-guard/outputs/ pointer, tail marker), one wake-provenance EngineDecision, provider calls exactly 3.

## Task Execution Detail

| Task | Commit | Files |
|-------|--------|-------|
| 1 — tracer: bash completion → notification → wake turn | test(22-01) RED + feat(22-01) GREEN | internal/tasks/*, coreexec/background*, runtime/{runtime,cron_wiring,wake_wiring_test} |
| 2 — coalescing, ordering, busy fallback, truncation | feat(22-01) | internal/tasks/notify_test.go, runtime/wake_wiring_test.go |
| 3 — subagent cap + FIFO + close-cancel | feat(22-01) | internal/tasks/tracker_test.go |

## Verification Evidence

- `go test -race -count=1 ./internal/runtime/ -run 'TestWakeTurn'` — PASS (tracer + busy + empty batteries).
- `go test -race -count=1 ./internal/tasks/` — PASS (9 batteries: coalescing, clock ordering, dedupe, empty no-op, rune-boundary truncation table, enqueue truncation, busy-leaves-pending, cap, FIFO, close, bound, default-cap, CancelTask).
- `go test -race -count=1 ./internal/coreexec/` — PASS (six pre-existing TestBackground_* + CompletionHook pair).
- `go vet ./internal/tasks/ ./internal/runtime/ ./internal/coreexec/ ./internal/redact/` — clean.
- `grep -c TryLock internal/runtime/cron_wiring.go` = 4 (≥ 2); `grep -c snapshotAndClear internal/tasks/notify.go` = 4 (≥ 1); `grep -c subagentQueueBound internal/tasks/tracker.go` = 4 (≥ 1).
- Full-package regression: `go test -race -count=1 ./internal/runtime/` PASS (193s under race; the package needs ≥200s timeouts).

## Deviations from Plan

- **[Rule 2 — missing critical] skRe redaction mid-word false positive.** Found during: Task 1 GREEN (tracer asserted the wake block; the transcript showed `<ta[REDACTED]>`). Issue: `sk-[A-Za-z0-9_\-]{6,}` matched inside "task-notification" (no word boundary) — any hyphenated sk- word (disk-usage, flask-session) was mangled in transcripts. Fix: leading `\b` in internal/redact/redact.go; TestRedact_SkTokenWordBoundary pins both directions (mid-word intact, boundary tokens still scrub). Files: internal/redact/redact.go, redact_test.go. Verification: redact package + tracer green.
- **[Discretion refinement] SetDrain/Drain signature interpretation.** The plan's artifact `SetDrain(fn func(pending []Notification))` is implemented as the drain-ATTEMPT callback (pending = non-destructive peek), with `Drain() []Notification` as the destructive snapshot-and-clear the runner calls under the turn mutex — the only shape where the busy-turn path can leave notifications pending by construction. Documented in tracker.go doc comments.
- **[Discretion refinement] busy-retry chain.** Beyond completion-scheduled attempts + scheduler-tick retry, a CAS-deduplicated per-session retry chain (500ms) covers schedule-less serves (test runners, no schedule store) where a wake that loses the TryLock race would otherwise strand until the next completion. Runner field wakeRetryInterval.

**Total deviations:** 1 auto-fixed (Rule 2), 2 discretion refinements. **Impact:** none on adjacent plans; 22-02/22-03 build directly on the tracker + CompletionHook.

## Issues Encountered

None.

## Self-Check: PASSED

## Next Phase Readiness

Ready for 22-02 (bash hardening rides TaskRegistry) and 22-03 (subagent leg rides the tracker).
