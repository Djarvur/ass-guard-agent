---
phase: 22-background-execution-sandbox-reality
verified: 2026-09-10T04:06:06Z
status: human_needed
score: 38/39 must-haves verified
covered_files:
  - .planning/phases/22-background-execution-sandbox-reality/22-01-PLAN.md
  - .planning/phases/22-background-execution-sandbox-reality/22-01-SUMMARY.md
  - .planning/phases/22-background-execution-sandbox-reality/22-02-PLAN.md
  - .planning/phases/22-background-execution-sandbox-reality/22-02-SUMMARY.md
  - .planning/phases/22-background-execution-sandbox-reality/22-03-PLAN.md
  - .planning/phases/22-background-execution-sandbox-reality/22-03-SUMMARY.md
  - .planning/phases/22-background-execution-sandbox-reality/22-04-PLAN.md
  - .planning/phases/22-background-execution-sandbox-reality/22-04-SUMMARY.md
  - .planning/phases/22-background-execution-sandbox-reality/22-05-PLAN.md
  - .planning/phases/22-background-execution-sandbox-reality/22-05-SUMMARY.md
  - .planning/phases/22-background-execution-sandbox-reality/22-06-PLAN.md
  - .planning/phases/22-background-execution-sandbox-reality/22-06-SUMMARY.md
  - .planning/phases/22-background-execution-sandbox-reality/22-07-PLAN.md
  - .planning/phases/22-background-execution-sandbox-reality/22-07-SUMMARY.md
  - .planning/phases/22-background-execution-sandbox-reality/22-08-PLAN.md
  - .planning/phases/22-background-execution-sandbox-reality/22-08-SUMMARY.md
  - .planning/phases/22-background-execution-sandbox-reality/22-09-PLAN.md
  - .planning/phases/22-background-execution-sandbox-reality/22-09-SUMMARY.md
  - .planning/phases/22-background-execution-sandbox-reality/22-REVIEW.md
  - .planning/REQUIREMENTS.md
  - cmd/ass-guard/acp_serve.go
  - cmd/ass-guard/main.go
  - cmd/ass-guard/background_wiring_test.go
  - go.mod
  - internal/acpserve/acp_serve.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/options.go
  - internal/coreexec/ansistrip.go
  - internal/coreexec/background.go
  - internal/coreexec/background_test.go
  - internal/coreexec/bash.go
  - internal/coreexec/planmode.go
  - internal/coreexec/procopts_linux.go
  - internal/coreexec/procopts_other.go
  - internal/coreexec/ptty.go
  - internal/coreexec/register.go
  - internal/runtime/cron_wiring.go
  - internal/runtime/runtime.go
  - internal/runtime/wake_wiring_test.go
  - internal/sandbox/child_linux.go
  - internal/sandbox/doc.go
  - internal/sandbox/landlock_linux.go
  - internal/sandbox/policy.go
  - internal/sandbox/seatbelt_darwin.go
  - internal/session/session.go
  - internal/session/subagent.go
  - internal/tasks/notify.go
  - internal/tasks/subagent.go
  - internal/tasks/subagent_test.go
  - internal/tasks/tracker.go
  - internal/tasks/tracker_test.go
  - internal/toolcat/coretools.json
covered_digest: "v1:sha256:d3733336c13d4ae7ba9c77d19ba544fd629f270d739814e90e80ed1ef03224ce"
behavior_unverified: 1 # 22-05 T5 darwin live seatbelt battery — not reproducible on this Linux host
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 34/39
  gaps_closed:
    - "G-22-1 (CR-01): tracker slot release — Complete decrements runningSubagents (floored) and retires the cancel entry for Kind==KindSubagent before startNextWaiter (internal/tasks/tracker.go:120-157); pinned green by TestTrackerCap_SlotsFreeAfterCompletion, TestTrackerCap_SequentialReuseNeverQueues, TestTrackerComplete_RetiresCancelEntry (run by this verifier)"
    - "G-22-2 (CR-02): wake-drain chain idles on empty and dies with the serve ctx — scheduleWakeDrain refuses to spawn past shutdown (cron_wiring.go:377-379) and the chain's exit defer restarts only when ctx live AND tracker present AND pending non-empty (:404-419); pinned green by TestWakeChain_IdlesWhenEmpty/NoSpawnPastServeShutdown/CtxCancelStopsChain/RacingCompletionStillWakes (run by this verifier)"
    - "G-22-3 (CR-03): panic recovery at the background-subagent goroutine boundary — recover defer writes the terminal error marker (panic text + debug.Stack) and fires exactly ONE error Complete (internal/tasks/subagent.go:87-100); pinned green by TestBackgroundSubagent_PanicRecovered (run by this verifier)"
    - "G-22-4 (CR-04): close-then-complete is terminal — OnClose cancels RUNNING subagents (runtime.go:2445), CloseSession prunes trackers/wakeInFlight/ptyManagers after s.Close() (:2908-2910), and drainWakeNotifications resolves the session NON-constructingly, dropping a closed session's batch loudly (cron_wiring.go:465-478); pinned green by TestWakeChain_ClosedSessionDropsBatch (both subtests) + TestCloseSession_PrunesWakeState (run by this verifier)"
    - "G-22-5 (WR-01, folded into PAR-07's prior NEEDS-GAP-CLOSURE evidence): TaskStop/TaskOutput fall through to tracker-backed seams on registry-unknown ids — InteractiveConfig.TaskStopFallback/TaskOutputFallback (planmode.go:122-123), errUnknownTask fallthrough (background.go:731, :825), sessionFor binds tracker.CancelTask + subagentOutputFallback (runtime.go:2192, :2197), Tracker.SubagentState classifier (tracker.go:329-342); pinned green by the 9 coreexec seam rows + TestTrackerSubagentState + TestTaskStopFallbackWiring + TestTaskOutputFallbackWiring (run by this verifier)"
  gaps_remaining: []
  regressions: []
behavior_unverified_items:
  - truth: "macOS confinement rides an embedded .sb template rendered IN MEMORY with targeted denies, never deny-default — the live darwin battery proves curl-connect deny and outside-write deny (22-05 T5)"
    test: "Run the darwin seatbelt battery on a macOS host (go test ./internal/sandbox/ with GOOS=darwin on darwin hardware)"
    expected: "sandbox-exec -p confinement denies curl connect and outside writes; profile is allow-default with targeted denies"
    why_human: "This verifier runs on Linux; seatbelt_darwin_test.go is build-gated darwin-only and cannot execute here. The linux-runnable evidence (TestPolicySymmetry_SeatbeltShapeNeverDenyDefault, TestPolicySymmetry_GoldenDenySet — both re-run green by this verifier) pins the RENDERED shape, not live enforcement; the Linux landlock leg IS live-proven here (included green in the full ./internal/coreexec/ package gate)."
human_verification:
  - test: "On a macOS host (amd64 or arm64), run GOOS=darwin go test ./internal/sandbox/ -run 'TestSeatbelt|TestPolicySymmetry_SeatbeltShape' -v"
    expected: "Live seatbelt battery passes: sandbox-exec -p confinement denies curl connect and outside writes; the rendered profile is allow-default with targeted denies (never deny-default)"
    why_human: "seatbelt_darwin_test.go is build-gated darwin-only; this verification host is Linux and cannot execute the live enforcement leg. Only the rendered-shape symmetry (golden deny set) is machine-verified here."
  - test: "Human resolution of the 8 judgment-tier prohibitions recorded in the Prohibition Disposition section below (P1-P9)"
    expected: "A human confirms the non-authoritative HELD verdicts for the must-NOT invariants (ids never model-controlled, client turn never preempted, sweep never silently deletes, ask-class decline never bypassed, PTY output never to stdout, never fail open silently, never confine ass-guard itself, green result never implies confinement, no exec site skips wrap while on)"
    why_human: "ADR-550 D4 autonomous verify: judgment-tier prohibitions carry a NON-AUTHORITATIVE LLM-judge verdict plus the unverified-prohibition flag; they are never a silent pass and require explicit human resolution at the end-of-phase checkpoint."
---

# Phase 22: Background Execution + Sandbox Reality Verification Report

**Phase Goal:** All long-lived process work converges on ONE lifecycle infrastructure: full subagents (background dispatch, structured task-notifications by kind, output retrieval, cancellation), background Bash completion callbacks on the same task-notification subsystem, persistent-shell Bash via PTY, and the sandbox flag made real with landlock/sandbox-exec split — probe-and-degrade loudly, default OFF.
**Verified:** 2026-09-10T04:06:06Z
**Status:** human_needed
**Re-verification:** Yes — after gap closure (22-07 / 22-08 / 22-09, commits 6bed452..dc3ab12)

## Goal Achievement

Re-verification of the 2026-09-10T01:44:45Z report (status gaps_found, 34/39). All four Critical-finding gaps were re-verified against the CURRENT codebase by direct code reading AND by running each gap's named regression battery in this verifier's own process — every one is genuinely closed, not narratively closed. The bonus closure round also fixed the prior PAR-07 blocker WR-01 (model-facing TaskStop/TaskOutput for subagent ids — `Tracker.CancelTask` had zero production callers; it now has its first ones). No regressions: the full three-package gate (coreexec + tasks + runtime, `-skip TestRescanConcurrency` — the documented pre-existing deferred race) is green, as are the session dispatch/panic/ask-decline rows, the sandbox symmetry goldens, and the cmd wiring battery.

The single remaining non-verified item is environmental, not code: the live darwin seatbelt battery cannot execute on this Linux host. It routes to human verification (status human_needed per the decision tree — rule 2), together with the 8 judgment-tier prohibition flags carried from the initial verification (ADR-550 autonomous mode).

### Observable Truths (roadmap contract level)

| # | Truth (Success Criterion) | Status | Evidence |
|---|---------------------------|--------|----------|
| 1 | SC-1: Full subagents — background dispatch returns immediately, kind-detected task-notifications, output retrieval, clean cancellation | ✓ VERIFIED | Dispatch/notifications/retrieval unchanged-green; cancellation now whole: G-22-1 slot release (tracker.go:120-157), G-22-3 panic recovery (subagent.go:87-100), G-22-5 TaskStop/TaskOutput reach exec_ ids (planmode.go:122-123/:141/:146, background.go:731/:825, runtime.go:2192/:2197) — 13 named tests run green by this verifier |
| 2 | SC-2: Background Bash — same notification subsystem, TERM-before-KILL, Pdeathsig, stale-log sweep, no orphans at shutdown | ✓ VERIFIED | Unchanged from initial verification; full ./internal/coreexec/ package gate (includes escalation + wake-completion rows) re-run green |
| 3 | SC-3: Persistent-shell PTY — state persists, ANSI stripped, EIO-as-EOF, no fd leak | ✓ VERIFIED | Unchanged; persistent-shell + TestPTYDrain rows green inside the full package gate |
| 4 | SC-4: Sandbox real — landlock/sandbox-exec split, loud degrade, default OFF, --sandbox=off escape | ✓ VERIFIED | Linux leg unchanged-green (live landlock batteries inside the coreexec gate; symmetry goldens re-run green); darwin LIVE leg ⚠ behavior-unverified (host is Linux — see behavior_unverified_items / Human Verification) |
| 5 | Goal-level: the ONE wake/lifecycle infrastructure is itself lifecycle-sound (chain idles when empty, respects shutdown; session close ends the machinery) | ✓ VERIFIED | G-22-2: scheduleWakeDrain shutdown guard (cron_wiring.go:377-379) + gated exit defer (:404-419, restart only on ctx-live AND tracker AND pending-non-empty); G-22-4: OnClose CancelRunning (runtime.go:2445), CloseSession prunes trackers/wakeInFlight/ptyManagers (:2908-2910), non-constructing drain lookup with loud terminal drop (cron_wiring.go:465-478) — 6 named tests run green by this verifier |

### Re-Verified Gap Truths (full 3-level verification — the previously failed items)

| Gap | Truth | Code (exists + substantive + wired) | Behavioral test (this verifier ran it) | Status |
|-----|-------|--------------------------------------|----------------------------------------|--------|
| G-22-1 (CR-01) | D-10 slots free on completion; queue drains as slots free; sequential reuse never queues | `Complete` calls `releaseSubagentSlot(id)` under `t.mu` for Kind==KindSubagent — floored decrement of `runningSubagents` + `delete(t.subagentCancels, id)` — BEFORE `startNextWaiter()` outside the lock (tracker.go:120-157, :218-238). The former monotonic counter has a real release path; release-then-admit keeps count ≤ cap during handoff | TestTrackerCap_SlotsFreeAfterCompletion, TestTrackerCap_SequentialReuseNeverQueues, TestTrackerComplete_RetiresCancelEntry — all PASS (verbose-confirmed) | ✓ VERIFIED |
| G-22-2 (CR-02) | Chain idles on empty queue, refuses to spawn past serve shutdown, no successor spin | `scheduleWakeDrain` returns early once `serveCtxOrBackground().Err() != nil` (cron_wiring.go:377-379); the chain's exit defer stores the flag false FIRST, then restarts only when `ctx.Err() == nil && tr != nil && len(tr.PendingPeek()) > 0` (:404-419) — exactly the review's prescribed gate; the peek→store race window is carried by the completion's own CAS (pinned by the racing row) | TestWakeChain_IdlesWhenEmpty, TestWakeChain_NoSpawnPastServeShutdown (sampled-window witness), TestWakeChain_CtxCancelStopsChain, TestWakeChain_RacingCompletionStillWakes — all PASS | ✓ VERIFIED |
| G-22-3 (CR-03) | A panicking background subagent never kills the process; one error notification; slot released | Launcher goroutine has a recover defer directly after `defer cancelFn()` (LIFO: recover runs first during unwind, replacing the normal Complete with exactly ONE error notification; cancelFn still runs on both paths): `w.finish("error", panic text + debug.Stack())` then `Tracker.Complete(ExitStatus "error")` (subagent.go:78-100) — the error Complete carries Kind=KindSubagent, so G-22-1's slot release fires on the panic path too | TestBackgroundSubagent_PanicRecovered (subprocess-guard shape: escaping panic → clean outer assertion) — PASS | ✓ VERIFIED |
| G-22-4 (CR-04) | Session close fully ends the background machinery; late completions never resurrect a closed session | OnClose cancels RUNNING subagents with counted stderr note (runtime.go:2445-2448, beside CancelQueued :2435); CloseSession deletes trackers/wakeInFlight/ptyManagers AFTER `s.Close()` so OnClose's cancels and PTY Drain run while state is discoverable (runtime.go:2899-2910); `drainWakeNotifications` resolves the session via a NON-constructing sessMu-held `r.sessions` read — missing session = one stderr line naming session + dropped count, `tr.Drain()` consumed, chain exits (cron_wiring.go:465-478). No constructing sessionFor remains in the drain (its only cron_wiring call site is runAutomationTurn); IN-05's unbounded retry is retired with it | TestWakeChain_ClosedSessionDropsBatch (missing_session_is_terminal + late_completion_never_resurrects subtests), TestCloseSession_PrunesWakeState — all PASS; TestSessionClosePTY and TestInProcessReloadAfterCloseRebuildsSession green inside the full runtime gate (close-then-restore preserved) | ✓ VERIFIED |
| G-22-5 (WR-01 — prior PAR-07 blocker) | TaskStop/TaskOutput address background-subagent exec_ ids; CancelTask gains production callers; finished classifies finished | `InteractiveConfig.TaskStopFallback func(id) bool` / `TaskOutputFallback func(id) (out, state string, running, handled bool)` bound at the stubs map (planmode.go:122-123, :141, :146); both executors fall through ONLY on `errors.Is(oerr, errUnknownTask)` and render through the ONE shared `renderTaskOutput` (background.go:731-743, :825+); `sessionFor` binds `TaskStopFallback: tracker.CancelTask` (first production callers) and `TaskOutputFallback: subagentOutputFallback(dir, tracker)` — SubagentState classify, else bounded 64 KiB `readTail`, stat-miss = not-handled (runtime.go:1775-1808, :2192, :2197); `SubagentState` reads waiters + subagentCancels under one lock (tracker.go:329-342), truthful because G-22-1's release deletes completed cancel entries | 9 coreexec seam rows (FallbackAck/Declined/Nil/RegistryOwned × both tools + NotReadyStates queued+running + FinishedOutput) + TestTrackerSubagentState + TestTaskStopFallbackWiring (live cancel fires, finished id = truthful error, zero provider calls) + TestTaskOutputFallbackWiring — all PASS | ✓ VERIFIED |

**Score:** 38/39 truths verified (1 present, behavior-unverified — darwin live seatbelt, environmental)

### Advisory (New Scope, Unevidenced)

Re-verification ran; per the evidence gate (#3304) this section is reported even when empty.

| # | Finding | Category | Why Advisory |
|---|---------|----------|--------------|
| — | None — no unevidenced new-scope findings in the closure round's diffs | — | Debt-marker scan clean on all 11 gap-closure-modified files; the `gofmt -l` quirks in internal/tasks are pre-existing regions documented in 22-07..09 (verified predating the closure hunks per the summaries' `gofmt -d` checks) |

### Required Artifacts

All previously verified artifacts remain present and substantive; the closure round modified six of them without regressions (full-package gates green). Closure-round artifact status:

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/tasks/tracker.go` | slot release, cancel-entry retirement, CancelRunning, SubagentState | ✓ VERIFIED | :120-157 release-then-admit; :300-315 CancelRunning (close-time running-cancel leg, idempotent with release); :329-342 classifier |
| `internal/tasks/subagent.go` | recover at the goroutine boundary | ✓ VERIFIED | :87-100 recover defer (panic text + debug.Stack → one error Complete); runtime/debug import :10 |
| `internal/runtime/cron_wiring.go` | shutdown-guarded spawn, gated exit defer, non-constructing drain | ✓ VERIFIED | :372-390, :404-419, :465-478 |
| `internal/runtime/runtime.go` | CancelRunning link, close pruning, seam binding | ✓ VERIFIED | :2445-2448, :2899-2910, :2192/:2197 + :1775-1808 |
| `internal/coreexec/background.go` + `planmode.go` | fallback seams + fallthrough | ✓ VERIFIED | planmode fields :122-123 bound :141/:146; background.go fallthrough :731-743, :825+; renderTaskOutput one-form renderer |
| Test artifacts (tracker_test, subagent_test, wake_wiring_test, background_test, background_wiring_test) | named regression batteries | ✓ VERIFIED | all exist; 22 named closure tests executed green by this verifier (verbose-confirmed they RAN) |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| Tracker.Complete (KindSubagent) | releaseSubagentSlot → startNextWaiter | release-then-admit under/outside t.mu | ✓ WIRED | G-22-1; count/cancel assertions green in the battery |
| background goroutine panic | w.finish + one error Complete | recover defer, LIFO after cancelFn | ✓ WIRED | G-22-3; TestBackgroundSubagent_PanicRecovered green |
| completion → scheduleWakeDrain → wakeDrainChain | gated exit defer + shutdown guard | flag.Store(false) then conditional restart | ✓ WIRED | G-22-2; 4 lifecycle rows green incl. the racing-completion carry arm |
| CloseSession → OnClose (CancelQueued+CancelRunning) → map pruning | s.Close() then trackers/wakeInFlight/ptyManagers.Delete | single unconditional site | ✓ WIRED | G-22-4; grep: exactly one Delete per map in runtime.go (:2908-2910) |
| late completion → drainWakeNotifications | non-constructing r.sessions lookup → terminal drop | sessMu-held read, miss = loud drop + Drain | ✓ WIRED | G-22-4; ClosedSessionDropsBatch both subtests green |
| TaskStop/TaskOutput executors → tracker seams | errUnknownTask fallthrough → CancelTask / SubagentState+readTail | planmode stubs binding + sessionFor binding | ✓ WIRED | G-22-5; 9 seam rows + both wiring rows green; coreexec stays free of any tasks-package import (primitive-arg seam) |

All key links from the initial verification remain wired (re-checked via the full package gates: bash completion hook, PTY drain, sandbox wrap at 3 sites, flag → probe chain).

### Data-Flow Trace (Level 4)

Unchanged from the initial verification — all five flows still FLOWING (notification payload ← waitErr+timestamps+tail+path; wake turn input ← tracker Drain under turn mutex; subagent output file ← dep.Run progress; PTY capture ← master read loop; availability ← ProbeABI). The closure round adds one flow, verified: TaskOutput fallback envelope ← SubagentState classification, else bounded 64 KiB readTail of `.ass-guard/outputs/<id>.log` (runtime.go:1786-1808) — a real file read, not a static return; stat-miss reports not-handled (truthful error).

### Behavioral Spot-Checks (single named tests, run by this verifier)

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| G-22-1 slot release + sequential reuse + cancel retirement | `go test ./internal/tasks/ -run '^(TestTrackerCap_SlotsFreeAfterCompletion\|TestTrackerCap_SequentialReuseNeverQueues\|TestTrackerComplete_RetiresCancelEntry)$'` | all PASS (verbose-confirmed) | ✓ PASS |
| G-22-3 panic containment (process survives, one error notification) | `go test ./internal/tasks/ -run '^TestBackgroundSubagent_PanicRecovered$'` | PASS | ✓ PASS |
| G-22-2 chain lifecycle (idle / no-spawn-past-shutdown / ctx-cancel / racing carry) | `go test -race ./internal/runtime/ -run '^(TestWakeChain_IdlesWhenEmpty\|TestWakeChain_NoSpawnPastServeShutdown\|TestWakeChain_CtxCancelStopsChain\|TestWakeChain_RacingCompletionStillWakes)$'` | all PASS (verbose-confirmed) | ✓ PASS |
| G-22-4 close terminal (drop batch, no resurrection, state pruned) | `go test -race ./internal/runtime/ -run '^(TestWakeChain_ClosedSessionDropsBatch\|TestCloseSession_PrunesWakeState)$'` | PASS incl. both subtests | ✓ PASS |
| G-22-5 classifier | `go test ./internal/tasks/ -run '^TestTrackerSubagentState$'` | PASS | ✓ PASS |
| G-22-5 session wiring (stop live id / read finished id; zero provider calls) | `go test -race ./internal/runtime/ -run '^(TestTaskStopFallbackWiring\|TestTaskOutputFallbackWiring)$'` | both PASS | ✓ PASS |
| G-22-5 coreexec seam (9 rows) | `go test ./internal/coreexec/ -run '^TestTask(Stop\|Output)_'` | all 9 PASS (verbose-confirmed) | ✓ PASS |
| Closure regression gate (3 packages, incl. live landlock + PTY + escalation + wake-turn + close-then-restore rows) | `go test -count=1 -skip 'TestRescanConcurrency' ./internal/coreexec/ ./internal/tasks/ ./internal/runtime/` | ok ×3 | ✓ PASS |
| Session dispatch regression | `go test ./internal/session/ -run '^(TestDispatchBackground\|TestSubagentPanicRecovery\|TestBackgroundAskDecline)'` | ok | ✓ PASS |
| Sandbox symmetry regression | `go test ./internal/sandbox/ -run '^(TestPolicySymmetry_GoldenDenySet\|TestPolicySymmetry_SeatbeltShapeNeverDenyDefault)$'` | ok | ✓ PASS |
| cmd wiring regression (4 nil-fallback sites) | `go test ./cmd/ass-guard/ -run 'TestBackgroundWiring'` | ok | ✓ PASS |
| darwin live seatbelt | (cannot run — Linux host) | — | ? SKIP → human verification |

Note: every named test was verbose-confirmed to RUN (a silent `-run` mismatch would also print "ok"). The pre-existing exclusions held: TestRescanConcurrency skipped per deferred-items.md (open, pre-existing, fix owner outside phase 22); TestPermissionsE2E not in any gate package here (Phase 23 commit 40b2bbc, operator-bisected); internal/evalsuite openspec env gating untouched.

### Probe Execution

No probe scripts declared or discovered (unchanged from initial verification — the plans' "probe" language refers to in-code sandbox probe taxonomy, not shell probes). Step 7c: none to run.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| PAR-07 | 22-01, 22-03, 22-07, 22-08, 22-09 | Full subagents: dispatch, kind-detected notifications, output retrieval, cancellation | ✓ SATISFIED | Dispatch/notifications/retrieval green (unchanged); cancellation now whole: CR-01 slot release, CR-03 panic containment, WR-01 TaskStop/TaskOutput reach exec_ ids (CancelTask has production callers) — all behaviorally pinned by tests this verifier ran |
| PAR-08 | 22-01, 22-02, 22-08 | Background Bash: same subsystem, process-group lifecycle, Pdeathsig, stale sweep | ✓ SATISFIED | Unchanged-green; 22-08 adds close-time running-cancel + state pruning (G-22-4) on the same subsystem; WR-04 caveat remains documented review debt |
| PAR-09 | 22-04 | Persistent-shell Bash via PTY | ✓ SATISFIED | Unchanged-green (full coreexec gate); WR-03/WR-08 edge warnings remain documented review debt |
| SAND-01 | 22-05, 22-06 | Sandbox real: landlock/seatbelt split, loud degrade, default OFF | ✓ SATISFIED (linux-live) | Default OFF + validation + live linux confinement green (inside the coreexec gate); darwin LIVE leg routed to human verification (Linux host) — rendered-shape symmetry machine-verified; WR-05/WR-06 warnings remain documented review debt |

Orphaned requirements: none — all four IDs mapped to Phase 22 in REQUIREMENTS.md are claimed by plans (now 9 plans: 22-01..22-09), and REQUIREMENTS.md marks all four Complete.

### Anti-Patterns Found

Debt-marker gate on the 11 gap-closure-modified files (tracker.go, tracker_test.go, subagent.go, subagent_test.go, cron_wiring.go, runtime.go, wake_wiring_test.go, background.go, background_test.go, planmode.go, background_wiring_test.go): CLEAN — no TBD/FIXME/XXX.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/coreexec/background.go | 585-597 | WR-02 Stop on already-reaped task signals possibly recycled pgid | ⚠️ Warning | unchanged review debt (out of closure scope by 22-07's objective) |
| internal/coreexec/ptty.go | 407-417 | WR-03 sentinel swallowed by stdin-readers | ⚠️ Warning | unchanged review debt |
| internal/acpserve/acp_serve.go | 524-571 | WR-04 sweep tombstones oversize bash-*.log | ⚠️ Warning | unchanged review debt |
| internal/acpserve/acp_serve.go / sandbox/policy.go | 176-178 / 45-54 | WR-05 RW grant on entire shared tmp | ⚠️ Warning | unchanged review debt |
| internal/sandbox/policy.go | 100-108 | WR-06 seatbelt paths unescaped | ⚠️ Warning | unchanged review debt (darwin leg — fold into the human macOS check) |
| internal/tasks/subagent.go | 56-110 | WR-07 queued-then-dropped fd leak | ⚠️ Warning | unchanged review debt |
| internal/coreexec/ansistrip.go | 63-67 | WR-08 OSC bare-backslash leak | ⚠️ Warning | unchanged review debt |
| internal/acpserve/config_surface.go | 306, 1292 | IN-01 stale menu-size comments | ℹ️ Info | unchanged |
| internal/tasks/subagent_test.go | 216-220 | IN-02 dead no-op conditional | ℹ️ Info | unchanged |
| internal/runtime/runtime.go | 295-301 | IN-03 turnMus/turnActive growth (half remains) | ℹ️ Info | trackers/wakeInFlight/ptyManagers half FIXED by G-22-4; turn mutexes intentionally kept (in-flight holders) |
| internal/tasks/subagent.go | 205-214 | IN-04 output-file write errors ignored | ℹ️ Info | unchanged |

Retired by the closure round: WR-01 (G-22-5) and IN-05 (unbounded stay-pending retry — its only trigger, drain-side sessionFor construction failure, no longer exists).

### Prohibition Disposition (ADR-550 D4 — autonomous verify: NON-AUTHORITATIVE verdicts)

**unverified-prohibition — human review recommended** for all 8 judgment-tier prohibitions (carried from initial verification; verdicts re-judged against the post-closure code, P2/P4 against the changed wake-chain paths specifically):

| Prohibition | Verdict | Basis |
|-------------|---------|-------|
| 22-01/P1 ids/paths never model-controlled (PAR-07) | HELD | crypto/rand minting unchanged (subagent.go:143-151); TaskOutput fallback adds no id-guessing surface (stat-miss = not-handled) |
| 22-01/P2 client turn never preempted (PAR-08) | HELD | TryLock discipline preserved in the reworked drain (cron_wiring.go:481); the gated restart spawns chains, never preempts a turn |
| 22-02/P3 sweep never silently deletes | HELD (letter) | sweep unchanged; WR-04 caveat unchanged |
| 22-03/P4 ask-class decline never bypassed | HELD | SetTurnOriginAutomation bracket unchanged in the drain (:495-497); stop/read wiring rows assert zero provider calls |
| 22-04/P5 PTY output never to stdout | HELD | unchanged |
| 22-05/P6 never fail open silently | HELD | unchanged |
| 22-05/P7 never confine ass-guard itself | HELD | unchanged |
| 22-06/P8 green result never implies confinement; P9 no exec site skips wrap while on | HELD | unchanged |

### Human Verification Required

### 1. Darwin live seatbelt battery (the one behavior-unverified truth)

**Test:** On a macOS host, run `GOOS=darwin go test ./internal/sandbox/ -run 'TestSeatbelt|TestPolicySymmetry_SeatbeltShape' -v`
**Expected:** Live sandbox-exec confinement denies curl connect and outside writes; the rendered profile is allow-default with targeted denies (never deny-default). While there, consider WR-06 (quote in a workdir can widen the darwin profile).
**Why human:** This verifier runs on Linux; the darwin battery is build-gated and cannot execute here. The Linux landlock leg IS live-proven on this host; only the darwin live leg is unexercised.

### 2. Judgment-tier prohibitions (ADR-550 autonomous mode)

**Test:** Human review of the 8 must-NOT invariants in the Prohibition Disposition table above.
**Expected:** Explicit human resolution of each flagged prohibition at the end-of-phase checkpoint (the recorded HELD verdicts are non-authoritative LLM judgments, not human sign-off).
**Why human:** Judgment-tier prohibitions cannot be machine-verified; autonomous verify must never silently pass them.

### Gaps Summary

No gaps remain. All four Critical findings from 22-REVIEW.md (CR-01 slot jam, CR-02 wake-chain spin, CR-03 panic kill, CR-04 session resurrection) are closed in code and pinned by named regression batteries this verifier executed and verbose-confirmed in its own process; the prior PAR-07 blocker WR-01 (unreachable cancellation) is closed by the same round (G-22-5). The full three-package gate plus the session/sandbox/cmd regression legs are green with only the documented pre-existing exclusions (TestRescanConcurrency per deferred-items.md). The phase goal — ONE lifecycle infrastructure, lifecycle-sound end to end — is achieved and behaviorally evidenced on every leg executable from this host.

The status is human_needed (not passed) solely because of the two human-review items above: the darwin live seatbelt battery (environmental — needs macOS hardware) and the 8 judgment-tier prohibition flags (ADR-550 autonomous-verify contract). Neither is a code gap; both block a `passed` verdict by the decision tree's rule 2.

---

_Verified: 2026-09-10T04:06:06Z_
_Verifier: Claude (gsd-verifier)_
_Re-verification of: 2026-09-10T01:44:45Z report (gaps_found, 34/39)_
