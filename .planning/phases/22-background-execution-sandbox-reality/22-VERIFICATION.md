---
phase: 22-background-execution-sandbox-reality
verified: 2026-09-10T01:44:45Z
status: gaps_found
score: 34/39 must-haves verified
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
  - .planning/phases/22-background-execution-sandbox-reality/22-REVIEW.md
  - .planning/REQUIREMENTS.md
  - cmd/ass-guard/acp_serve.go
  - cmd/ass-guard/main.go
  - go.mod
  - internal/acpserve/acp_serve.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/options.go
  - internal/coreexec/ansistrip.go
  - internal/coreexec/background.go
  - internal/coreexec/bash.go
  - internal/coreexec/procopts_linux.go
  - internal/coreexec/procopts_other.go
  - internal/coreexec/ptty.go
  - internal/coreexec/register.go
  - internal/runtime/cron_wiring.go
  - internal/runtime/runtime.go
  - internal/sandbox/child_linux.go
  - internal/sandbox/doc.go
  - internal/sandbox/landlock_linux.go
  - internal/sandbox/policy.go
  - internal/sandbox/seatbelt_darwin.go
  - internal/session/session.go
  - internal/session/subagent.go
  - internal/tasks/notify.go
  - internal/tasks/subagent.go
  - internal/tasks/tracker.go
  - internal/toolcat/coretools.json
covered_digest: "v1:sha256:5cd9913f2e85f90ec542ef9cc92ba4dcc6535d0b79ea14d52fb5ee27de70d8ba"
behavior_unverified: 1 # 22-05 T5 darwin live seatbelt battery — not reproducible on this Linux host
overrides_applied: 0
gaps:
  - truth: "Background subagent registration is capped (default 8): over-cap registrations enter a FIFO queue with a visible queued note, DRAINED AS SLOTS FREE (D-10) — also 22-03 'Over-cap dispatches queue FIFO via the tracker'"
    status: failed
    reason: "CR-01 CONFIRMED by direct code reading: t.runningSubagents is only ever incremented (tracker.go:74 declaration, :164 admission, :205 waiter pop) — NO decrement path exists. Complete() calls startNextWaiter() but never releases the completing subagent's slot, so the counter counts lifetime admissions. After 8 TOTAL background-subagent starts in a session (sequential or concurrent), RegisterSubagent's admission check (runningSubagents < SubagentCap) is permanently false and every subsequent dispatch queues forever with a note promising 'it starts when a slot frees'. Existing queue tests pass only because they never re-register after completions."
    artifacts:
      - path: "internal/tasks/tracker.go"
        issue: "runningSubagents monotonic; Complete() must decrement (floored) for Kind==KindSubagent before startNextWaiter"
    missing:
      - "Decrement runningSubagents in Complete (or a releaseSubagentSlot helper) so completions actually free slots"
      - "Regression test: register cap tasks, complete all, register once more, assert queued==false (starts immediately)"
  - truth: "The ONE wake-drain infrastructure is itself lifecycle-sound: the drain chain goes idle when the pending queue is empty and respects serve shutdown (goal: 'converges on ONE lifecycle infrastructure')"
    status: failed
    reason: "CR-02 CONFIRMED: wakeDrainChain's defer (cron_wiring.go:393-400) unconditionally calls r.scheduleWakeDrain(sessionID) on EVERY exit path — including the empty-queue exit (:412-414) — and is not gated on ctx.Err(). The defer clears the in-flight flag then scheduleWakeDrain's CAS on the just-cleared flag always succeeds, spawning a successor that again finds the queue empty: a permanent one-goroutine spin loop per session after the first completion, continuing through and past serve shutdown. TestWakeTurn_EmptyPendingNoTurn passes only because the spin makes no provider calls and the test binary exits."
    artifacts:
      - path: "internal/runtime/cron_wiring.go"
        issue: "defer restarts the chain unconditionally; restart must be gated on (a) pending actually non-empty (a genuine race) and (b) serve ctx still live; scheduleWakeDrain should refuse to spawn when serveCtx is done"
    missing:
      - "Conditional restart in the wakeDrainChain defer (pending non-empty AND ctx.Err()==nil)"
      - "Guard in scheduleWakeDrain against spawning past serve shutdown"
  - truth: "Cancellation/robustness of background subagents: a panicking background subagent must never kill the ass-guard process (the D-13 invariant the foreground leg delivers, session/subagent.go:105-126)"
    status: failed
    reason: "CR-03 CONFIRMED: RunBackgroundSubagent's launcher goroutine (tasks/subagent.go:77-100) has defer cancelFn() but NO recover(). dep.Run is the session's nested turn loop (provider streaming, restricted tool execution); any panic there crashes the entire agent — ACP server, every session, every PTY shell — violating the project's own stated D-13 invariant, which the FOREGROUND dispatch site enforces and pins with TestSubagentPanicRecovery. The panic also bypasses w.finish (dangling output file) and Tracker.Complete (wedging the already-broken slot accounting of CR-01)."
    artifacts:
      - path: "internal/tasks/subagent.go"
        issue: "no recover() at the goroutine boundary; panic path skips marker write, notification, and slot release"
    missing:
      - "Mirror the foreground recover(): on panic, w.finish(\"error\", ...) + Tracker.Complete with ExitStatus error"
      - "Test: a dep.Run that panics does not crash the process and fires exactly one error notification"
  - truth: "Session close fully ends the session's background machinery — late task completions must never resurrect a closed session (OQ5 family; PAR-07/PAR-08 lifecycle convergence)"
    status: failed
    reason: "CR-04 CONFIRMED: CloseSession (runtime.go:2783-2810) evicts r.sessions only; grep shows NO trackers.Delete / wakeInFlight.Delete / ptyManagers.Delete anywhere (IN-03). OnClose cancels queued subagents (CancelQueued) but RUNNING background subagents are never cancelled, so their completions land after close; the wake chain then calls drainWakeNotifications whose first step is the CONSTRUCTING r.sessionFor(ctx, sessionID) (cron_wiring.go:441-448), rebuilding a full session (MCP host subprocesses, transcript writer, PTY manager, registry) for the closed id, holding the turn mutex, draining the batch, and running a real provider wake turn into a session the operator closed — ghost provider cost, transcript appends, spawned subprocesses, plus the resurrected session lingering in r.sessions."
    artifacts:
      - path: "internal/runtime/runtime.go"
        issue: "CloseSession does not delete trackers/wakeInFlight (or ptyManagers post-Drain); running background subagents not cancelled at close"
      - path: "internal/runtime/cron_wiring.go"
        issue: "drainWakeNotifications uses the constructing sessionFor; a missing session must be terminal (log once, drop batch), not reconstruct"
    missing:
      - "Delete per-session tracker/wakeInFlight entries at CloseSession; cancel running background subagents at close"
      - "Non-constructing session lookup in drainWakeNotifications; treat missing session as terminal (drop the batch loudly, exit the chain)"
behavior_unverified_items:
  - truth: "macOS confinement rides an embedded .sb template rendered IN MEMORY with targeted denies, never deny-default — the live darwin battery proves curl-connect deny and outside-write deny (22-05 T5)"
    test: "Run the darwin seatbelt battery on a macOS host (go test ./internal/sandbox/ with GOOS=darwin on darwin hardware)"
    expected: "sandbox-exec -p confinement denies curl connect and outside writes; profile is allow-default with targeted denies"
    why_human: "This verifier runs on Linux; seatbelt_darwin_test.go is build-gated darwin-only and cannot execute here. The linux-runnable evidence (TestPolicySymmetry_SeatbeltShapeNeverDenyDefault, TestPolicySymmetry_GoldenDenySet) pins the RENDERED shape, not live enforcement; the Linux landlock leg IS live-proven here (TestBackgroundSandbox_LiveDeniesNetwork PASS)."
---

# Phase 22: Background Execution + Sandbox Reality Verification Report

**Phase Goal:** All long-lived process work converges on ONE lifecycle infrastructure: full subagents (background dispatch, structured task-notifications by kind, output retrieval, cancellation), background Bash completion callbacks on the same task-notification subsystem, persistent-shell Bash via PTY, and the sandbox flag made real with landlock/sandbox-exec split — probe-and-degrade loudly, default OFF.
**Verified:** 2026-09-10T01:44:45Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

The surface area of this phase is genuinely delivered and heavily tested: every plan's artifacts exist, are substantive, and are wired; the named behavioral batteries pass (including LIVE landlock confinement on this Linux host); no debt markers; no orphaned requirements. However, all four Critical findings from 22-REVIEW.md were independently re-confirmed by direct code reading in this verification — each is a deterministic correctness defect in a delivered must-have that the existing test batteries do not exercise. Per the escalation-gate contract, a Critical correctness bug in a delivered must-have is a gap, not a style note.

### Observable Truths (roadmap contract level)

| # | Truth (Success Criterion) | Status | Evidence |
|---|---------------------------|--------|----------|
| 1 | SC-1: Full subagents — background dispatch returns immediately, kind-detected task-notifications, output retrieval, clean cancellation | ✗ FAILED | Dispatch/notifications/retrieval verified live (tests pass), but CR-01 jams the cap queue after 8 total starts ("drained as slots free" is false), CR-03 lets one panicking background subagent kill the whole process, and WR-01 leaves model-facing cancellation unreachable (TaskStop/TaskOutput consult only the TaskRegistry; `Tracker.CancelTask` has ZERO production callers) |
| 2 | SC-2: Background Bash — same notification subsystem, TERM-before-KILL, Pdeathsig, stale-log sweep, no orphans at shutdown | ✓ VERIFIED | terminateGroup funnel (background.go:541-548) on all kill paths; setPdeathsig (procopts_linux.go:18, no-op other); sweep before scheduler (acp_serve.go:444); ReapAll ladder; TestEscalation_* / TestWakeTurn_BackgroundBashCompletion PASS |
| 3 | SC-3: Persistent-shell PTY — state persists, ANSI stripped, EIO-as-EOF, no fd leak | ✓ VERIFIED | TestPersistentShell_CdPersists / ExportedEnvPersists / SentinelLookalike / NonPersistentCallsStayStateless, TestPTYDrain PASS; OnClose chain drains PTY beside ReapAll (runtime.go:2350-2351) |
| 4 | SC-4: Sandbox real — landlock/sandbox-exec split, loud degrade, default OFF, --sandbox=off escape | ✓ VERIFIED | Flag default "off" + startup vocab validation (cmd/acp_serve.go:392, :357; ValidateSandboxMode acp_serve.go:135); LIVE linux proof: TestBackgroundSandbox_LiveDeniesNetwork, QueuedStartWrapsIdentically, DefaultOffArgvIdentity, TestPTYSandbox_LiveShellConfined all PASS; darwin live leg not reproducible here (see behavior_unverified_items) |
| 5 | Goal-level: the ONE wake/lifecycle infrastructure is itself lifecycle-sound (chain idles when empty, respects shutdown; session close ends the machinery) | ✗ FAILED | CR-02: wakeDrainChain defer restarts unconditionally (permanent spin per session, past serve ctx); CR-04: late completions reconstruct closed sessions via constructing sessionFor and run ghost provider turns |

### Per-Plan Truth Detail (37 plan truths + 2 goal-derived)

| Plan | Truths | Verdicts | Failed / Unverified |
|------|--------|----------|---------------------|
| 22-01 (tracker/notify/wake, PAR-07/08) | 6 | 5 VERIFIED | T5 (D-10 queue drains as slots free) FAILED — CR-01 |
| 22-02 (Bash lifecycle hardening, PAR-08) | 6 | 6 VERIFIED | — |
| 22-03 (background subagents, PAR-07) | 7 | 6 VERIFIED | T5 (over-cap FIFO queue) FAILED — CR-01 |
| 22-04 (persistent PTY, PAR-09) | 7 | 7 VERIFIED | — (WR-03/WR-08 edge warnings noted) |
| 22-05 (sandbox policy core, SAND-01) | 5 | 4 VERIFIED | T5 (live darwin battery) PRESENT_BEHAVIOR_UNVERIFIED — darwin-only, host is linux |
| 22-06 (sandbox enforcement, SAND-01) | 6 | 6 VERIFIED | — (live linux confinement proven) |
| Derived (goal lifecycle soundness) | 2 | 0 VERIFIED | D1 FAILED — CR-02; D2 FAILED — CR-04 |

**Score:** 34/39 truths verified (1 present, behavior-unverified; 4 failed)

### Required Artifacts

All 25 declared artifacts exist, are substantive (line counts: tracker 273, notify 120, subagent 242, background 820, ptty 613, bash 574, policy 241, landlock 181, child 139, seatbelt 137, cron_wiring 531, session/subagent 559, config_surface 1911, runtime 3894), and are wired. Spot checks:

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/tasks/tracker.go` | ONE notification subsystem, Kind enum, cap/queue | ✓ VERIFIED (artifact) | `func NewTracker` present; D-02 fields; per-kind queue — but see CR-01 gap in the counter |
| `internal/tasks/notify.go` | pending queue, coalesce, dedupe, rune-boundary tail | ✓ VERIFIED | all four behaviors test-pinned |
| `internal/coreexec/background.go` | CompletionHook, terminateGroup, FIFO waiters, sandbox wrap | ✓ VERIFIED | hook :135/:161, funnel :541, waiters :126, wrap :385 |
| `internal/runtime/cron_wiring.go` | drainWakeNotifications, TryLock, runOneTurn | ✓ VERIFIED (artifact) | :434+ TryLock discipline — but see CR-02/CR-04 gaps in the chain |
| `internal/coreexec/procopts_linux.go` / `_other.go` | Pdeathsig split | ✓ VERIFIED | :18 SIGKILL with go.dev/issue/27505 caveat; no-op leg |
| `internal/tasks/subagent.go` | RunBackgroundSubagent, output file, killed marker | ✓ VERIFIED (artifact) | :54; but see CR-03 gap (no recover) |
| `internal/session/subagent.go` + `session.go` | async_launched branch, shared loop core | ✓ VERIFIED | :543 `"status": "async_launched"`; background branch session.go:771 |
| `internal/coreexec/ansistrip.go` / `ptty.go` / `bash.go` | StripANSI, PTYManager, Persistent branch | ✓ VERIFIED | :13, :108/:372/:554, :153/:374 |
| `internal/toolcat/coretools.json` | additive `persistent` property | ✓ VERIFIED | :187 |
| `internal/sandbox/*` (5 files) | Policy/DefaultPolicy/WrapCmd, ProbeABI, RunSandboxChild, seatbelt, doc | ✓ VERIFIED | all markers present; symmetry golden PASS |
| `cmd/ass-guard/acp_serve.go` / `main.go` | --sandbox flag, ApplySandboxChildHook | ✓ VERIFIED | :392 flag, main.go:47 unconditional hook before cobra |
| `internal/acpserve/options.go` / `acp_serve.go` | SandboxMode, resolveSandboxAvailability, sweep | ✓ VERIFIED | :40 SandboxMode, :148/:155 probe, :444 sweep before scheduler |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| background.go Wait goroutine | Tracker.Complete | CompletionHook wired runtime.go:2024, kind KindBash | ✓ WIRED | TestWakeTurn_BackgroundBashCompletion PASS |
| tracker completion | drainWakeNotifications | scheduleWakeDrain + sessionTurnMu TryLock | ✓ WIRED (letter) | delivery works for live sessions; chain lifecycle broken (CR-02/CR-04) |
| session.go dispatch site | RunBackgroundSubagent | runtime.go:2051 backgroundLaunch | ✓ WIRED | TestDispatchBackground_AsyncLaunchedImmediately PASS |
| bash.go persistent branch | PTYManager.Run | coreexec.Config PTY injection (register.go) | ✓ WIRED | TestPersistentShell_BashToolCdPersistsAcrossCalls PASS |
| ptty.go Drain | OnClose chain | runtime.go:2351 beside ReapAll | ✓ WIRED | TestPTYDrain PASS |
| Policy | LandlockRules + SeatbeltProfile | one source, symmetry golden | ✓ WIRED | TestPolicySymmetry_GoldenDenySet PASS |
| WrapCmd (portable) | 3 exec sites | single `wrapSandboxCmd` seam (bash.go:62); background.go:385, ptty.go:201 | ✓ WIRED | live tests PASS at all three sites |
| --sandbox flag | startup probe | flag → Options.SandboxMode → resolveSandboxAvailability before scheduler | ✓ WIRED | ValidateSandboxMode at cmd start |
| main.go | ApplySandboxChildHook | unconditional before root dispatch | ✓ WIRED | live re-exec child proven by LiveDeniesNetwork PASS |
| config_surface Set | injected caps | background.bash → registry, background.subagents → Tracker | ✓ WIRED | optBackgroundBash/Subs in menu (:55, :1383) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| Notification payload | exit status / duration / tail / output path | waitErr + start timestamp + bounded buffer snapshot + taskLogPath (background.go hook) | Yes | ✓ FLOWING |
| Wake turn input | pending batch | tracker Drain() snapshot-and-clear under turn mutex | Yes | ✓ FLOWING |
| Subagent output file | progressive chunks | dep.Run progress → subagentWriter.append | Yes | ✓ FLOWING |
| PTY capture | sentinel-delimited read | PTY master read loop, ANSI-stripped | Yes | ✓ FLOWING |
| Sandbox availability | Availability reason | ProbeABI / probeSeatbelt at startup, stored in Handle | Yes | ✓ FLOWING |

### Behavioral Spot-Checks (single named tests, run by this verifier)

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| D-03 coalescing (3→1 batch, ordering, dedupe) | `go test ./internal/tasks/ -run '^(TestNotify_CoalesceThreeIntoOneBatch|TestNotify_CompletionTimeOrderingNotArrival|TestNotify_DedupeByTaskID)$'` | all PASS | ✓ PASS |
| PAR-08 escalation ladder (TERM respected / TERM-immune killed after grace) | `go test ./internal/coreexec/ -run '^(TestEscalation_TermRespectingChildDiesByTerm|TestEscalation_TermImmuneChildKilledAfterGrace)$'` | both PASS | ✓ PASS |
| Mid-flight output retrieval + killed marker + exactly-one notification | `go test ./internal/tasks/ -run '^(TestBackgroundOutput_MidRunRetrieval|TestBackgroundCancel_KilledMarkerAndNotification)$'` | both PASS | ✓ PASS |
| async_launched returns immediately; queue-over-cap reports queued; foreground panic recovery | `go test ./internal/session/ -run '^(TestDispatchBackground_AsyncLaunchedImmediately|TestDispatchBackground_QueuedOverCap|TestSubagentPanicRecovery)$'` | all PASS | ✓ PASS |
| Background ask-decline (OQ3) | `go test ./internal/session/ -run 'TestBackgroundAskDecline'` | PASS | ✓ PASS |
| Persistent shell state (cd/export persist, sentinel anti-spoof, non-persistent stateless) | `go test ./internal/coreexec/ -run '^(TestPersistentShell_CdPersists|TestPersistentShell_SentinelLookalikeOutputDoesNotTerminateEarly|TestPersistentShell_NonPersistentCallsStayStateless)$'` | all PASS | ✓ PASS |
| LIVE landlock: background network deny, queued-start wraps identically, argv identity off, PTY live confinement | `go test ./internal/coreexec/ -run '^(TestBackgroundSandbox_LiveDeniesNetwork|TestBackgroundSandbox_QueuedStartWrapsIdentically|TestBackgroundSandbox_DefaultOffArgvIdentity|TestPTYSandbox_LiveShellConfined)$'` | all PASS | ✓ PASS |
| Sandbox policy symmetry (never deny-default) | `go test ./internal/sandbox/ -run '^(TestPolicySymmetry_GoldenDenySet|TestPolicySymmetry_SeatbeltShapeNeverDenyDefault)$'` | both PASS | ✓ PASS |
| Wake turn wiring (completion wakes, busy accumulates, empty no-turn) | `go test ./internal/runtime/ -run '^(TestWakeTurn_BackgroundBashCompletion|TestWakeTurn_BusyClientTurnAccumulates|TestWakeTurn_EmptyPendingNoTurn)$'` | all PASS | ✓ PASS |
| D-10 queue drain order | `go test ./internal/tasks/ -run 'TestTrackerQueue_FIFODrainOrder'` | PASS (but never re-registers after completion — blind to CR-01) | ✓ PASS |

Note: all named tests pass. The four gaps are on paths these batteries do not exercise (re-registration after completions, chain idle termination past test-binary exit, panics in the background goroutine, close-then-complete ordering) — presence of green tests is not behavioral coverage of the broken invariants.

### Probe Execution

No probe scripts declared or discovered (`find` for `scripts/*/tests/probe-*.sh` returns nothing; the plans' "probe" language refers to in-code sandbox probe taxonomy, not shell probes). Step 7c: none to run.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| PAR-07 | 22-01, 22-03 | Full subagents: dispatch, kind-detected notifications, output retrieval, cancellation | ✗ NEEDS GAP CLOSURE | Dispatch/notifications/retrieval verified (tests PASS); cancellation broken at three points: CR-01 (queue jams after 8 total starts), CR-03 (panic kills process), WR-01 (TaskStop/TaskOutput cannot address subagent task ids — `Tracker.CancelTask` dead code, zero production callers) |
| PAR-08 | 22-01, 22-02 | Background Bash: same subsystem, process-group lifecycle, Pdeathsig, stale sweep | ✓ SATISFIED | Full ladder + sweep + ReapAll verified; WR-04 caveat (sweep tombstones `bash-*.log` oversize persists, breaking `<persisted-output>` pointers after restart) |
| PAR-09 | 22-04 | Persistent-shell Bash via PTY | ✓ SATISFIED | All seven truths test-pinned; WR-03 (sentinel swallowed by stdin-readers → 120s wedge) and WR-08 (OSC bare-backslash leak) edge warnings |
| SAND-01 | 22-05, 22-06 | Sandbox real: landlock/seatbelt split, loud degrade, default OFF | ✓ SATISFIED (linux-live) | Default OFF + startup validation + live linux confinement at all three sites; darwin live leg execution-claimed (host was darwin during 22-05), not reproducible here; WR-05 (DefaultPolicy RW on entire shared /tmp) and WR-06 (seatbelt unescaped quote in paths) warnings |

Orphaned requirements: none — all four IDs mapped to Phase 22 in REQUIREMENTS.md are claimed by plans (PAR-07: 22-01/22-03; PAR-08: 22-01/22-02; PAR-09: 22-04; SAND-01: 22-05/22-06).

### Anti-Patterns Found

No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER markers in any phase-modified file. Debt-marker gate: clean.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/tasks/tracker.go | 74, 164, 205 | CR-01 monotonic counter | 🛑 Blocker | gap 1 |
| internal/runtime/cron_wiring.go | 393-400 | CR-02 unconditional chain restart | 🛑 Blocker | gap 2 |
| internal/tasks/subagent.go | 77-100 | CR-03 missing recover() | 🛑 Blocker | gap 3 |
| internal/runtime/runtime.go:2783 / cron_wiring.go:441 | — | CR-04 session resurrection | 🛑 Blocker | gap 4 |
| internal/coreexec/background.go | 700-792 | WR-01 TaskStop/TaskOutput cannot address subagent tasks | ⚠️ Warning | model-facing cancellation of background subagents unreachable; `Tracker.CancelTask` dead code |
| internal/coreexec/background.go | 585-597 | WR-02 Stop on already-reaped task signals a possibly recycled pgid | ⚠️ Warning | stale task id stopped late can TERM/KILL an unrelated process group |
| internal/coreexec/ptty.go | 407-417 | WR-03 sentinel swallowed by stdin-consuming commands | ⚠️ Warning | 120s wedge + shell-state loss on `cat`/`read`/password prompts |
| internal/acpserve/acp_serve.go | 524-571 | WR-04 sweep tombstones oversize `bash-*.log` persists | ⚠️ Warning | persisted-output pointers 404 after restart; deleted after 7 days |
| internal/acpserve/acp_serve.go / sandbox/policy.go | 176-178 / 45-54 | WR-05 RW grant on the entire shared system tmp dir | ⚠️ Warning | confinement weakness on multi-user hosts; tests use private tmp, production does not |
| internal/sandbox/policy.go | 100-108 | WR-06 seatbelt paths interpolated unescaped | ⚠️ Warning | a `"` in a workdir can widen/alter the darwin profile; landlock leg safe (JSON) |
| internal/tasks/subagent.go | 56-110 | WR-07 queued-then-dropped tasks leak the output-file fd | ⚠️ Warning | one fd per dropped waiter for process lifetime |
| internal/coreexec/ansistrip.go | 63-67 | WR-08 OSC terminated at bare backslash | ⚠️ Warning | OSC tail + control byte can reach tool results |
| internal/acpserve/config_surface.go | 306, 1292 | IN-01 stale menu-size comments | ℹ️ Info | docs say 8/12 entries; menu is 20 |
| internal/tasks/subagent_test.go | 216-220 | IN-02 dead no-op conditional | ℹ️ Info | always-taken empty `if`, asserts nothing |
| internal/runtime/runtime.go | 295-301 | IN-03 per-session maps never pruned | ℹ️ Info | unbounded growth; substrate of CR-04 |
| internal/tasks/subagent.go | 205-214 | IN-04 output-file write errors ignored | ℹ️ Info | notification tail can diverge from the durable log |
| internal/runtime/cron_wiring.go | 442-448 | IN-05 unbounded retry stderr spam | ℹ️ Info | masks diagnostics when session cannot be constructed |

### Prohibition Disposition (ADR-550 D4 — autonomous verify: NON-AUTHORITATIVE verdicts)

**unverified-prohibition — human review recommended** for all 8 judgment-tier prohibitions (`status: unresolved`, no test tier declared). LLM-judge verdicts from this verification (not a substitute for human resolution):

| Prohibition | Verdict | Basis |
|-------------|---------|-------|
| 22-01/P1 ids/paths never model-controlled (PAR-07) | HELD | crypto/rand minting (background.go:173, subagent.go:121); paths derive from session workdir |
| 22-01/P2 client turn never preempted (PAR-08) | HELD | TryLock + coalescing retry; TestWakeTurn_BusyClientTurnAccumulates PASS |
| 22-02/P3 sweep never silently deletes | HELD (letter) | tombstone rename + counted note; deletes only past-window .stale — but WR-04 sweeps files it should not |
| 22-03/P4 ask-class decline never bypassed | HELD | executeRestricted ctx marker + gateAskClass; TestBackgroundAskDecline PASS |
| 22-04/P5 PTY output never to stdout | HELD | all sinks stderr; stdout-clean serve batteries pin it |
| 22-05/P6 never fail open silently | HELD | strict taxonomy, distinct reasons, per-run notes + counter |
| 22-05/P7 never confine ass-guard itself | HELD | sentinel-child-only ruleset; hook fires only on sentinel env |
| 22-06/P8 green result never implies confinement | HELD | per-run unconfined notes + process-wide counter at all sites |
| 22-06/P9 no exec site skips wrap while on | HELD (three sites) | live batteries pin all three; "fourth site" clause is a future review gate |

### Human Verification Required

Not emitted as a routing section (status is gaps_found, which takes precedence). One behavior-unverified truth is recorded in `behavior_unverified_items` (darwin live seatbelt battery — needs a macOS host) and should be folded into the human checkpoint or UAT when the gaps are re-verified.

### Gaps Summary

Phase 22 delivered its full surface — artifacts, wiring, and green batteries at every layer, including live landlock confinement — but the review's four Critical findings are all real, independently re-confirmed in code by this verifier, and each falsifies a delivered must-have on a path no existing test exercises:

1. **CR-01 (tracker.go)** — the D-10 subagent cap queue permanently jams after 8 total starts in a session (counter never decremented); falsifies 22-01 T5 and 22-03 T5.
2. **CR-02 (cron_wiring.go)** — the wake-drain chain restarts itself unconditionally on every exit, a permanent per-session goroutine spin that also outlives serve shutdown; falsifies the goal's "ONE lifecycle infrastructure" soundness.
3. **CR-03 (tasks/subagent.go)** — no panic recovery in the background-subagent goroutine; one panicking subagent kills the entire agent, violating the D-13 invariant the foreground leg delivers.
4. **CR-04 (runtime.go + cron_wiring.go)** — late completions resurrect closed sessions (constructing sessionFor, never-deleted trackers, running subagents not cancelled at close), producing ghost provider turns into closed sessions.

Grouped root cause: three of the four (CR-01, CR-03, CR-04) are in the NEW background-notification machinery's terminal/lifecycle paths — the batteries prove the happy paths (launch, notify, coalesce, cancel-by-id) but none re-register after completions, panic, or close-then-complete. A focused gap-closure plan should fix the tracker slot release + panic recovery + close hygiene (CancelQueued-adjacent) together, and gate the drain chain's restart. WR-01 (model-facing TaskStop/TaskOutput for subagent ids) belongs in the same closure round: the cancel mechanism exists and is tested, but the model cannot reach it.

Not gaps (deferred check, Step 9b): none of the four defects is covered by Phases 23-25 (23 = SEED gaps close-out: steering queue/checkpoint/undo; 24 = docs/ops tails; 25 = kit extraction).

---

_Verified: 2026-09-10T01:44:45Z_
_Verifier: Claude (gsd-verifier)_
