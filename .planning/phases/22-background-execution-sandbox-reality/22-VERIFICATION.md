---
phase: 22-background-execution-sandbox-reality
verified: 2026-09-10T13:41:06Z
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
  - internal/runtime/commands.go
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
covered_digest: "v1:sha256:0890d9df4a24b278303d49cd0e3a6c376b32ae95b72625655e29a54b73cb53b4"
behavior_unverified: 1 # 22-05 T5 darwin live seatbelt battery — not reproducible on this Linux host
overrides_applied: 0
re_verification:
  previous_status: human_needed
  previous_score: 38/39
  gaps_closed: [] # stale re-verification, not a gap-closure round — prior pass (2026-09-10T04:06:06Z) left zero open gaps
  gaps_remaining: []
  regressions: [] # phase-23 (3c64e82..0c17c02) touched only runtime.go + commands.go in this phase's surface; all seams intact, all batteries re-run green
behavior_unverified_items:
  - truth: "macOS confinement rides an embedded .sb template rendered IN MEMORY with targeted denies, never deny-default — the live darwin battery proves curl-connect deny and outside-write deny (22-05 T5)"
    test: "Run the darwin seatbelt battery on a macOS host (go test ./internal/sandbox/ with GOOS=darwin on darwin hardware)"
    expected: "sandbox-exec -p confinement denies curl connect and outside writes; profile is allow-default with targeted denies"
    why_human: "This verifier runs on Linux; seatbelt_darwin_test.go is build-gated darwin-only and cannot execute here. The linux-runnable evidence (TestPolicySymmetry_SeatbeltShapeNeverDenyDefault, TestPolicySymmetry_GoldenDenySet — both re-run green by this verifier in THIS pass) pins the RENDERED shape, not live enforcement; the Linux landlock leg IS live-proven here (included green in the full ./internal/coreexec/ package gate)."
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
**Verified:** 2026-09-10T13:41:06Z
**Status:** human_needed
**Re-verification:** Yes — STALE re-verification (post-phase-23 code movement). Prior chain: initial 2026-09-10T01:44:45Z (gaps_found, 34/39) → gap closure 22-07/08/09 (commits 6bed452..dc3ab12) → re-verify 2026-09-10T04:06:06Z (human_needed, 38/39, zero gaps) → THIS pass re-ran the goal-backward analysis after phase-23 commits touched `internal/runtime`.

## Goal Achievement

### Stale re-verification scope (this pass)

The prior pass went stale because phase-23 commits (3c64e82..0c17c02: 23-05 `/undo` class-B command, 23-06 workspace-scoped restore guard) modified files phase 22's surface covers. Bounding the delta precisely:

- **Committed changes since 2026-09-10T04:06:06Z in this phase's surface:** exactly `internal/runtime/runtime.go` (+404/-26 across 12 hunks) and `internal/runtime/commands.go` (+226), plus two NEW phase-23 test files (`commands_test.go`, `restore_guard_test.go` — phase-23 scope, not phase-22 must-haves). Every other phase-22 covered impl file (tasks/tracker.go, tasks/subagent.go, tasks/notify.go, coreexec/background.go, coreexec/planmode.go, coreexec/ptty.go, coreexec/bash.go, runtime/cron_wiring.go, sandbox/*, session/*, acpserve/*, cmd/*) has **zero commits** since the prior green pass.
- **Working tree:** the many ` M` entries are file-mode changes only (0644→0755, 0 content lines — `git diff --stat` confirms 0 insertions/deletions on every phase-22 covered file); runtime.go/commands.go/background.go have no worktree diff at all.
- **Hunk-level review of the two changed files:** all runtime.go hunks sit in the /undo/checkpoint/restore-guard/parked-chain areas (Runner struct fields, Run-pipeline command resolution at :878-1075, resolvesAsCommand, unregisterParkedChain/cancelParkedChains, checkpointStore/restoreBlockedError/restoreBlockers). None touch the phase-22 seams. commands.go's /undo additions reference no tracker/task machinery (only a CostCeilingTracker comment).

**Phase-22 seams re-confirmed present and wired in the CURRENT tree:**

| Seam | Current location | Status |
|------|-----------------|--------|
| 22-09 G-22-5: `TaskStopFallback: tracker.CancelTask` / `TaskOutputFallback: subagentOutputFallback(dir, tracker)` bound in `sessionFor`'s `RegisterInteractive` | runtime.go:2549-2563 (fields at :2556/:2561) | ✓ intact, unwired-by-nothing — `tracker.CancelTask` remains a live production binding |
| 22-09: `subagentOutputFallback` (SubagentState classify → bounded 64 KiB `readTail` of `.ass-guard/outputs/<id>.log`; stat-miss = not-handled) | runtime.go:2139-2167 | ✓ intact, byte-identical region (shifted line numbers only) |
| 22-08 G-22-4: OnClose cancels RUNNING subagents (beside CancelQueued) | runtime.go:2791-2815 (`CancelQueued` :2799, `CancelRunning` :2809, `ptyMgr.Drain()` :2794) | ✓ intact |
| 22-08 G-22-4: CloseSession prunes trackers/wakeInFlight/ptyManagers AFTER `s.Close()` | runtime.go:3236-3277 (pruning :3272-3274) | ✓ intact — phase-23's `cancelParkedChains` call (:3240) sits ahead of it without disturbing the ordering |
| 22-07 G-22-2: wake chain (shutdown guard, gated exit defer, non-constructing drain) | cron_wiring.go:372-390/:404-419/:465-478 — git-unchanged | ✓ intact |
| 22-07 G-22-1/G-22-3: slot release + panic recover | tracker.go:117-145+, subagent.go:88-89 — git-unchanged | ✓ intact |
| Sandbox default OFF (`Mode "off"` = default and explicit refusal) + wrap seam at bash/pty/background | bash.go:74-80, ptty.go:67, background.go:136 — git-unchanged | ✓ intact |

**Every battery re-run green by this verifier in its own process** (see Behavioral Spot-Checks) — including the full three-package gate, which compiles and exercises the post-phase-23 runtime.go. **No regressions.** The phase goal still holds on the current tree; the status remains `human_needed` solely for the two carried human legs (darwin live seatbelt + ADR-550 prohibitions).

### Observable Truths (roadmap contract level)

| # | Truth (Success Criterion) | Status | Evidence |
|---|---------------------------|--------|----------|
| 1 | SC-1: Full subagents — background dispatch returns immediately, kind-detected task-notifications, output retrieval, clean cancellation | ✓ VERIFIED | Dispatch/notifications/retrieval/retrieval seams intact (git-unchanged core + intact runtime bindings); cancellation whole: slot release (tracker.go:117-145), panic recovery (subagent.go:88-89), TaskStop/TaskOutput reach exec_ ids (runtime.go:2556/:2561 + :2139-2167) — batteries re-run green this pass |
| 2 | SC-2: Background Bash — same notification subsystem, TERM-before-KILL, Pdeathsig, stale-log sweep, no orphans at shutdown | ✓ VERIFIED | coreexec git-unchanged; full ./internal/coreexec/ package gate re-run green this pass (escalation + wake-completion rows inside) |
| 3 | SC-3: Persistent-shell PTY — state persists, ANSI stripped, EIO-as-EOF, no fd leak | ✓ VERIFIED | git-unchanged; persistent-shell + PTY rows green inside the full package gate re-run this pass; `ptyMgr.Drain()` at OnClose confirmed at runtime.go:2794 |
| 4 | SC-4: Sandbox real — landlock/sandbox-exec split, loud degrade, default OFF, --sandbox=off escape | ✓ VERIFIED | Linux leg unchanged-green (live landlock batteries inside the coreexec gate re-run this pass; symmetry goldens re-run green); default-OFF refusal logic confirmed at bash.go:74-80; darwin LIVE leg ⚠ behavior-unverified (host is Linux — see behavior_unverified_items / Human Verification) |
| 5 | Goal-level: the ONE wake/lifecycle infrastructure is itself lifecycle-sound (chain idles when empty, respects shutdown; session close ends the machinery) | ✓ VERIFIED | Wake chain git-unchanged (cron_wiring.go); OnClose/CloseSession lifecycle intact after phase-23 edits (runtime.go:2791-2815, :3263-3274); 8-test runtime battery re-run green under `-race` this pass |

### Gap Truths (from the 2026-09-10T04:06:06Z gap-closure round — still closed)

All five closures (G-22-1..G-22-5, commits 6bed452..dc3ab12) remain in history with their batteries; their batteries were ALL re-executed green by this verifier against the post-phase-23 tree (see Behavioral Spot-Checks — the tasks battery, the runtime wake/close battery under `-race`, and the 9 coreexec seam rows). The seams themselves were re-read in the current code (table above).

**Score:** 38/39 truths verified (1 present, behavior-unverified — darwin live seatbelt, environmental)

### Advisory (New Scope, Unevidenced)

Re-verification ran; per the evidence gate (#3304) this section is reported even when empty.

| # | Finding | Category | Why Advisory |
|---|---------|----------|--------------|
| — | None — no unevidenced new-scope findings in the phase-23 delta | — | The two modified files (runtime.go, commands.go) are the only in-surface regressions per the evidence gate; both were hunk-reviewed (all hunks in /undo/checkpoint/parked-chain areas) and every phase-22 battery re-ran green on them. Debt-marker scan clean on all 14 impl files re-scanned. |

### Required Artifacts

All previously verified artifacts remain present and substantive. The stale-delta round modified two of them (`runtime.go`, `commands.go`) without phase-22 regressions; all other covered impl files are git-unchanged since the prior green pass.

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/runtime/runtime.go` | 22-08/22-09 seams survive phase-23 /undo+restore-guard work | ✓ VERIFIED | Seams re-read this pass: fallback bindings :2549-2563, subagentOutputFallback :2139-2167, OnClose cancel legs :2791-2815, CloseSession pruning :3272-3274; phase-23 hunks confined to /undo/checkpoint/parked-chain areas |
| `internal/runtime/commands.go` | no interference with task seams | ✓ VERIFIED | /undo additions use checkpoint/cancel-registry machinery only; zero tracker/task-seam references (grep) |
| `internal/runtime/cron_wiring.go` | wake chain lifecycle | ✓ VERIFIED | git-unchanged since prior pass; 5 wake-chain tests green under `-race` this pass |
| `internal/tasks/tracker.go` + `subagent.go` | slot release, cancel retirement, CancelRunning, SubagentState, panic recover | ✓ VERIFIED | git-unchanged; 5-test battery green this pass |
| `internal/coreexec/background.go` + `planmode.go` | fallback seams + errUnknownTask fallthrough + renderTaskOutput | ✓ VERIFIED | git-unchanged; 9 seam rows green this pass |
| `internal/coreexec/bash.go`/`ptty.go` + `internal/sandbox/*` | sandbox wrap seam, default OFF, landlock/seatbelt split | ✓ VERIFIED | git-unchanged; default-OFF refusal at bash.go:74-80; full coreexec gate (live landlock) + symmetry goldens green this pass |
| Test artifacts (tracker_test, subagent_test, wake_wiring_test, background_test, background_wiring_test) | named regression batteries | ✓ VERIFIED | all exist and re-ran green this pass (verbose-confirmed they RAN) |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| TaskStop/TaskOutput executors → tracker seams | errUnknownTask fallthrough → CancelTask / SubagentState+readTail | planmode stubs binding + sessionFor binding | ✓ WIRED | Re-read at runtime.go:2549-2563 post-phase-23; TestTaskStopFallbackWiring + TestTaskOutputFallbackWiring green under `-race` this pass; coreexec stays tasks-free |
| Tracker.Complete (KindSubagent) | releaseSubagentSlot → startNextWaiter | release-then-admit under/outside t.mu | ✓ WIRED | git-unchanged; slot/cancel battery green this pass |
| background goroutine panic | w.finish + one error Complete | recover defer, LIFO after cancelFn | ✓ WIRED | git-unchanged; TestBackgroundSubagent_PanicRecovered green this pass |
| completion → scheduleWakeDrain → wakeDrainChain | gated exit defer + shutdown guard | flag.Store(false) then conditional restart | ✓ WIRED | git-unchanged; 4 lifecycle rows green under `-race` this pass |
| CloseSession → OnClose (CancelQueued+CancelRunning+PTY Drain) → map pruning | s.Close() then trackers/wakeInFlight/ptyManagers.Delete | single unconditional site | ✓ WIRED | Re-read at runtime.go:3263-3274 post-phase-23; TestCloseSession_PrunesWakeState + TestWakeChain_ClosedSessionDropsBatch (both subtests) green under `-race` this pass |
| late completion → drainWakeNotifications | non-constructing r.sessions lookup → terminal drop | sessMu-held read, miss = loud drop + Drain | ✓ WIRED | git-unchanged (cron_wiring.go); covered by the same close battery |

All key links from the initial verification remain wired (re-checked via the full package gates: bash completion hook, PTY drain, sandbox wrap at 3 sites, flag → probe chain).

### Data-Flow Trace (Level 4)

Unchanged from the prior pass — all five flows still FLOWING (notification payload ← waitErr+timestamps+tail+path; wake turn input ← tracker Drain under turn mutex; subagent output file ← dep.Run progress; PTY capture ← master read loop; availability ← ProbeABI), plus the G-22-5 flow: TaskOutput fallback envelope ← SubagentState classification, else bounded 64 KiB readTail of `.ass-guard/outputs/<id>.log` (runtime.go:2160) — re-read this pass, still a real file read; stat-miss reports not-handled.

### Behavioral Spot-Checks (single named tests, run by this verifier THIS pass, post-phase-23 tree)

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Wake/close/fallback battery (8 tests: chain idle / no-spawn-past-shutdown / ctx-cancel / racing carry / closed-session-drops-batch ×2 subtests / CloseSession prunes / TaskStop+TaskOutput wiring) | `go test -race -count=1 -v -run '^(TestWakeChain_IdlesWhenEmpty\|TestWakeChain_NoSpawnPastServeShutdown\|TestWakeChain_CtxCancelStopsChain\|TestWakeChain_RacingCompletionStillWakes\|TestWakeChain_ClosedSessionDropsBatch\|TestCloseSession_PrunesWakeState\|TestTaskStopFallbackWiring\|TestTaskOutputFallbackWiring)$' ./internal/runtime/` | all PASS (verbose-confirmed, incl. both subtests) | ✓ PASS |
| Tasks battery (slot release / sequential reuse / cancel retirement / panic recovered / SubagentState) | `go test -count=1 -v -run '^(TestTrackerCap_SlotsFreeAfterCompletion\|TestTrackerCap_SequentialReuseNeverQueues\|TestTrackerComplete_RetiresCancelEntry\|TestBackgroundSubagent_PanicRecovered\|TestTrackerSubagentState)$' ./internal/tasks/` | all 5 PASS (verbose-confirmed) | ✓ PASS |
| Coreexec seam (9 rows: FallbackAck/Declined/Nil/RegistryOwned × both tools + NotReadyStates + FinishedOutput) | `go test -count=1 -v -run '^TestTask(Stop\|Output)_' ./internal/coreexec/` | all 9 PASS (verbose-confirmed) | ✓ PASS |
| Closure regression gate (3 packages, incl. live landlock + PTY + escalation + wake-turn rows, post-phase-23 runtime) | `go test -count=1 -skip 'TestRescanConcurrency' ./internal/coreexec/ ./internal/tasks/ ./internal/runtime/` | ok ×3 (11.2s / 5.1s / 29.6s) | ✓ PASS |
| Session dispatch regression (DispatchBackground family ×5 + SubagentPanicRecovery + BackgroundAskDecline) | `go test -count=1 ./internal/session/ -run 'TestDispatchBackground' -v` + `-run '^(TestSubagentPanicRecovery\|TestBackgroundAskDecline)$' -v` | all PASS (verbose-confirmed; the family prefix is required — `TestDispatchBackground` is a 5-test family, not one test) | ✓ PASS |
| Sandbox symmetry regression | `go test -count=1 ./internal/sandbox/ -run '^(TestPolicySymmetry_GoldenDenySet\|TestPolicySymmetry_SeatbeltShapeNeverDenyDefault)$' -v` | both PASS (verbose-confirmed) | ✓ PASS |
| cmd wiring regression (6 rows incl. CrossSessionIsolation, StopKillsGroup, ReapAll) | `go test -count=1 ./cmd/ass-guard/ -run 'TestBackgroundWiring' -v` | all 6 PASS (verbose-confirmed) | ✓ PASS |
| darwin live seatbelt | (cannot run — Linux host) | — | ? SKIP → human verification |

Note: every named test was verbose-confirmed to RUN (a silent `-run` mismatch would also print "ok" — this pass caught and corrected one such anchor mistake on the DispatchBackground family). Pre-existing exclusions held: TestRescanConcurrency skipped per deferred-items.md (open, pre-existing, fix owner outside phase 22); the environmental `t.Skipf` at background_test.go:937 (sandbox-enforcement-unavailable host gate — the documented probe-and-degrade design) did not mask the live landlock rows on this host, which ran green inside the coreexec package gate.

### Probe Execution

No probe scripts declared or discovered (unchanged — the plans' "probe" language refers to in-code sandbox probe taxonomy, not shell probes). Step 7c: none to run.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| PAR-07 | 22-01, 22-03, 22-07, 22-08, 22-09 | Full subagents: dispatch, kind-detected notifications, output retrieval, cancellation | ✓ SATISFIED | All legs re-confirmed on the post-phase-23 tree; 22-09 seam intact (runtime.go:2549-2563) and its wiring tests green under `-race` this pass; REQUIREMENTS.md marks Complete |
| PAR-08 | 22-01, 22-02, 22-08 | Background Bash: same subsystem, process-group lifecycle, Pdeathsig, stale sweep | ✓ SATISFIED | git-unchanged; full coreexec gate green this pass; close-time running-cancel + pruning re-read intact (runtime.go:2791-2815/:3272-3274); WR-04 caveat remains documented review debt |
| PAR-09 | 22-04 | Persistent-shell Bash via PTY | ✓ SATISFIED | git-unchanged; PTY rows green inside the full package gate; WR-03/WR-08 edge warnings remain documented review debt |
| SAND-01 | 22-05, 22-06 | Sandbox real: landlock/seatbelt split, loud degrade, default OFF | ✓ SATISFIED (linux-live) | Default OFF refusal confirmed (bash.go:74-80); live linux confinement green inside the coreexec gate; darwin LIVE leg still routed to human verification (Linux host); WR-05/WR-06 warnings remain documented review debt |

Orphaned requirements: none — all four IDs mapped to Phase 22 in REQUIREMENTS.md are claimed by plans (22-01..22-09) and marked Complete.

### Decision Coverage

Gate run this pass (`check.decision-coverage-verify`): 12/12 trackable CONTEXT.md decisions honored by shipped artifacts, 0 not honored (non-blocking gate, informational).

### Anti-Patterns Found

Debt-marker gate re-run this pass on the 14 phase-22 impl files (including the two phase-23-modified ones): CLEAN — no TBD/FIXME/XXX. Disabled-test scan on the phase-22 battery files: one environmental `t.Skipf` (background_test.go:937 — sandbox-enforcement host gate, by design; live landlock rows ran on this host).

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/coreexec/background.go | 585-597 | WR-02 Stop on already-reaped task signals possibly recycled pgid | ⚠️ Warning | unchanged review debt |
| internal/coreexec/ptty.go | 407-417 | WR-03 sentinel swallowed by stdin-readers | ⚠️ Warning | unchanged review debt |
| internal/acpserve/acp_serve.go | 524-571 | WR-04 sweep tombstones oversize bash-*.log | ⚠️ Warning | unchanged review debt |
| internal/acpserve/acp_serve.go / sandbox/policy.go | 176-178 / 45-54 | WR-05 RW grant on entire shared tmp | ⚠️ Warning | unchanged review debt |
| internal/sandbox/policy.go | 100-108 | WR-06 seatbelt paths unescaped | ⚠️ Warning | unchanged review debt (darwin leg — fold into the human macOS check) |
| internal/tasks/subagent.go | 56-110 | WR-07 queued-then-dropped fd leak | ⚠️ Warning | unchanged review debt |
| internal/coreexec/ansistrip.go | 63-67 | WR-08 OSC bare-backslash leak | ⚠️ Warning | unchanged review debt |
| internal/acpserve/config_surface.go | 306, 1292 | IN-01 stale menu-size comments | ℹ️ Info | unchanged |
| internal/tasks/subagent_test.go | 216-220 | IN-02 dead no-op conditional | ℹ️ Info | unchanged |
| internal/runtime/runtime.go | 295-301 (prior numbering) | IN-03 turnMus/turnActive growth (half remains) | ℹ️ Info | trackers/wakeInFlight/ptyManagers half fixed (G-22-4); turn mutexes intentionally kept; re-confirmed present this pass (pruning at :3272-3274) |
| internal/tasks/subagent.go | 205-214 | IN-04 output-file write errors ignored | ℹ️ Info | unchanged |

Retired earlier: WR-01 (G-22-5) and IN-05 (unbounded stay-pending retry). No new debt markers, stubs, or unwired seams introduced by the phase-23 delta.

### Prohibition Disposition (ADR-550 D4 — autonomous verify: NON-AUTHORITATIVE verdicts)

**unverified-prohibition — human review recommended** for all 8 judgment-tier prohibitions (re-judged this pass against the post-phase-23 code; the phase-23 delta touches none of these surfaces except P2/P4's shared file, where the wake-chain paths are git-unchanged and the new /undo paths add no turn-preemption — they route through the same command-resolution gate before any turn starts):

| Prohibition | Verdict | Basis |
|-------------|---------|-------|
| 22-01/P1 ids/paths never model-controlled (PAR-07) | HELD | crypto/rand minting unchanged (subagent.go git-unchanged); TaskOutput fallback adds no id-guessing surface (stat-miss = not-handled) |
| 22-01/P2 client turn never preempted (PAR-08) | HELD | TryLock discipline preserved (cron_wiring.go:174/:481 re-confirmed); phase-23's /undo resolves as a command BEFORE turn dispatch — no preemption added |
| 22-02/P3 sweep never silently deletes | HELD (letter) | sweep unchanged; WR-04 caveat unchanged |
| 22-03/P4 ask-class decline never bypassed | HELD | SetTurnOriginAutomation bracket unchanged (cron_wiring.go:168-170); stop/read wiring rows assert zero provider calls (re-run green) |
| 22-04/P5 PTY output never to stdout | HELD | unchanged |
| 22-05/P6 never fail open silently | HELD | unchanged |
| 22-05/P7 never confine ass-guard itself | HELD | unchanged |
| 22-06/P8 green result never implies confinement; P9 no exec site skips wrap while on | HELD | unchanged |

### Human Verification Required

### 1. Darwin live seatbelt battery (the one behavior-unverified truth)

**Test:** On a macOS host, run `GOOS=darwin go test ./internal/sandbox/ -run 'TestSeatbelt|TestPolicySymmetry_SeatbeltShape' -v`
**Expected:** Live sandbox-exec confinement denies curl connect and outside writes; the rendered profile is allow-default with targeted denies (never deny-default). While there, consider WR-06 (quote in a workdir can widen the darwin profile).
**Why human:** This verifier runs on Linux; the darwin battery is build-gated and cannot execute here. The Linux landlock leg IS live-proven on this host (re-run green this pass); only the darwin live leg is unexercised.

### 2. Judgment-tier prohibitions (ADR-550 autonomous mode)

**Test:** Human review of the 8 must-NOT invariants in the Prohibition Disposition table above.
**Expected:** Explicit human resolution of each flagged prohibition at the end-of-phase checkpoint (the recorded HELD verdicts are non-authoritative LLM judgments, not human sign-off).
**Why human:** Judgment-tier prohibitions cannot be machine-verified; autonomous verify must never silently pass them.

### Gaps Summary

No gaps. This stale re-verification confirms the phase-22 goal still holds on the current tree after phase-23's `/undo` + restore-guard work: the delta was confined to runtime.go and commands.go, every phase-22 seam in those files was re-read intact (22-09 fallback bindings, OnClose cancel legs, CloseSession pruning), every other covered file is git-unchanged, and the full regression surface — 8-test runtime battery under `-race`, 5-test tasks battery, 9 coreexec seam rows, the full three-package gate, session dispatch family, sandbox symmetry goldens, and the 6-row cmd wiring battery — re-ran green in this verifier's own process, verbose-confirmed.

The status is human_needed (not passed) solely because of the two carried human-review items: the darwin live seatbelt battery (environmental — needs macOS hardware) and the 8 judgment-tier prohibition flags (ADR-550 autonomous-verify contract). Neither is a code gap; both block a `passed` verdict by the decision tree's rule 2.

---

_Verified: 2026-09-10T13:41:06Z_
_Verifier: Claude (gsd-verifier)_
_Stale re-verification of: 2026-09-10T04:06:06Z report (human_needed, 38/39) — triggered by phase-23 commits 3c64e82..0c17c02 touching internal/runtime_
