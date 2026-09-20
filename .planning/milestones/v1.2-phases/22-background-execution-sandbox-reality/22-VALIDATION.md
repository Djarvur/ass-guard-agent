---
phase: 22
slug: background-execution-sandbox-reality
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-08-28
---

# Phase 22 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Populated (revision r1) from 22-RESEARCH.md "Validation Architecture" + the per-task `<verify>` blocks in 22-01..22-06-PLAN.md.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `stretchr/testify` v1.11.1 (in go.mod), `-race` always |
| **Config file** | `.mise.toml` (tasks), golangci-lint v2 config — existing, no Wave 0 install |
| **Quick run command** | `go test -race -count=1 ./internal/coreexec/ ./internal/runtime/ ./internal/session/ ./internal/tasks/ ./internal/sandbox/ ./internal/acpserve/ ./internal/toolcat/` |
| **Full suite command** | `mise ci` (vet + lint + CGO_ENABLED=0 build + `go test -race -count=1 ./...`) |
| **Linux compile gate** | `GOOS=linux CGO_ENABLED=0 go build ./... && GOOS=linux CGO_ENABLED=0 go vet ./internal/sandbox/ ./internal/coreexec/` |
| **Estimated runtime** | quick ~30s; linux gate ~10s; `mise ci` several minutes (phase gate + wave merges only) |

---

## Sampling Rate

- **After every task commit:** Run the quick command (or the touched-package subset from the task's `<verify>` — always faster than the quick command)
- **After every plan wave:** Run `mise ci` (full suite)
- **Before `/gsd-verify-work`:** Full suite green + the linux compile gate
- **Max feedback latency:** < 30s per task commit (touched-package subset), < 60s full quick command

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 22-01-01 | 01 | 1 | PAR-07, PAR-08 | T-22-01, T-22-02 | Background bash completion → ONE kind-tagged D-02 wake turn through the automation rails; task ids crypto/rand minted, never model-controlled (tracer, RED-first) | integration | `go test -race -count=1 ./internal/runtime/ -run 'TestWakeTurn' && go test -race -count=1 ./internal/tasks/ ./internal/coreexec/` | ❌ W0 | ⬜ pending |
| 22-01-02 | 01 | 1 | PAR-07 | T-22-02 | Coalescing: one wake per drain window, ordered + deduped; busy client turn never preempted (D-01 fallback, D-03) | unit | `go test -race -count=1 ./internal/tasks/ ./internal/runtime/ -run 'TestNotify|TestWakeTurn'` | ❌ W0 | ⬜ pending |
| 22-01-03 | 01 | 1 | PAR-07 | T-22-02 | Subagent cap bounded (default 8) with FIFO queue + bound breach error; close drops queued-unstarted with counted note, never kills running work (D-10, OQ5) | unit | `go test -race -count=1 ./internal/tasks/ -run 'TestTrackerCap|TestTrackerQueue|TestTrackerClose' && go test -race -count=1 ./internal/tasks/ ./internal/runtime/` | ❌ W0 | ⬜ pending |
| 22-02-01 | 02 | 2 | PAR-08 | T-22-05 | TERM-before-KILL escalation on every termination path, one terminateGroup funnel; ESRCH-clean; Pdeathsig linux-only via build tag (compile gate) | unit + compile gate | `go test -race -count=1 ./internal/coreexec/ -run 'TestEscalation' && GOOS=linux CGO_ENABLED=0 go build ./... && GOOS=linux CGO_ENABLED=0 go vet ./internal/coreexec/` | ❌ W0 | ⬜ pending |
| 22-02-02 | 02 | 2 | PAR-08 | T-22-06, T-22-08 | Over-cap bash queues FIFO (bounded 64) with visible note, no silent fail; caps operator-tunable apply-as-landed (D-11, D-12) | unit | `go test -race -count=1 ./internal/coreexec/ -run 'TestBackgroundCap' && go test -race -count=1 ./internal/acpserve/ -run 'TestConfig' && go test -race -count=1 ./internal/coreexec/ ./internal/acpserve/ ./internal/runtime/` | ❌ W0 | ⬜ pending |
| 22-02-03 | 02 | 2 | PAR-08 | T-22-07 | Startup sweep tombstone-marks orphan logs, never silent rm, never removes the directory; runs before scheduler start (OQ4) | unit | `go test -race -count=1 ./internal/acpserve/ -run 'TestStaleSweep' && go test -race -count=1 ./internal/acpserve/` | ❌ W0 | ⬜ pending |
| 22-05-01 | 05 | 1 | SAND-01 | T-22-19, T-22-21 | ONE policy → symmetric landlock + seatbelt renderings (golden); profile never deny-default; LIVE darwin denies (curl connect fail, outside-touch EPERM, inside-tmp ok); no profile artifacts on disk (D-04, D-06) | unit + integration (live darwin) | `go test -race -count=1 ./internal/sandbox/ -run 'TestPolicySymmetry|TestSeatbelt' && go vet ./internal/sandbox/` | ❌ W0 | ⬜ pending |
| 22-05-02 | 05 | 1 | SAND-01 | T-22-20, T-22-22, T-22-23 | Strict probe taxonomy (ENOSYS/EOPNOTSUPP/ABI<4 → distinct reasons), never BestEffort (grep gate 0); child-only ruleset from shared LandlockRules(); cgo-free compile gate | unit (linux-tagged) + compile gate | `GOOS=linux CGO_ENABLED=0 go build ./... && GOOS=linux CGO_ENABLED=0 go vet ./internal/sandbox/ && go build ./...` | ❌ W0 | ⬜ pending |
| 22-03-01 | 03 | 3 | PAR-07 | T-22-12 | async_launched returns before subagent completion; serve-lifetime ctx survives dispatching-turn cancellation; foreground path byte-identical (anti-pattern regression test) | unit | `go test -race -count=1 ./internal/session/ -run 'TestDispatchBackground' && go test -race -count=1 ./internal/tasks/ ./internal/session/` | ❌ W0 | ⬜ pending |
| 22-03-02 | 03 | 3 | PAR-07 | T-22-10, T-22-13 | Output file retrievable mid-run; cancel → killed marker + exactly ONE terminal notification; files always valid UTF-8 | unit | `go test -race -count=1 ./internal/tasks/ -run 'TestBackgroundOutput|TestBackgroundCancel' && go test -race -count=1 ./internal/tasks/ ./internal/session/` | ❌ W0 | ⬜ pending |
| 22-03-03 | 03 | 3 | PAR-07 | T-22-09 | Ask-class tools declined inside background subagents (no silent permission escalation, 17-D-07); one model-resolution call site for both modes (equality battery; no second resolver — grep gate 0) | unit | `go test -race -count=1 ./internal/session/ -run 'TestDispatchBackgroundRouting|TestBackgroundAskDecline'` | ❌ W0 | ⬜ pending |
| 22-04-01 | 04 | 4 | PAR-09 | T-22-15 | One PTY persistence (cd/export); nonce-sentinel completion (lookalike output never ends early); ANSI-stripped capture — zero 0x1b reaches tool result; schema property additive with additionalProperties still false (OQ1) | unit (real PTY on darwin) | `go test -race -count=1 ./internal/coreexec/ -run 'TestAnsiStrip|TestPersistentShell' && go test -race -count=1 ./internal/toolcat/ && go test -race -count=1 ./internal/coreexec/` | ❌ W0 | ⬜ pending |
| 22-04-02 | 04 | 4 | PAR-09 | T-22-16 | Dead shell detected → lazy restart with visible state-loss note (once); prompt interruption return; arrival-order serialization; empty command moves nothing (D-08) | unit | `go test -race -count=1 ./internal/coreexec/ -run 'TestPTYDeadRestart|TestPTYSerialize|TestPTYEmpty' && go test -race -count=1 ./internal/coreexec/` | ❌ W0 | ⬜ pending |
| 22-04-03 | 04 | 4 | PAR-09, PAR-08 | T-22-17 | Close drains shell group TERM→KILL + closes master fd; fd count returns to baseline after 5 cycles (no leak across session/load cycles); OnClose link fires | unit | `go test -race -count=1 ./internal/coreexec/ -run 'TestPTYDrain' && go test -race -count=1 ./internal/runtime/ -run 'TestSessionClosePTY|TestOnClose' && go test -race -count=1 ./internal/coreexec/ ./internal/runtime/` | ❌ W0 | ⬜ pending |
| 22-06-01 | 06 | 5 | SAND-01 | T-22-20, T-22-26 | Default OFF (zero probing when off); flag validated at startup; probe step before scheduler start; faked probe failure → exactly ONE loud warning + serve continues; sentinel main hook before root dispatch | unit | `go test -race -count=1 ./internal/acpserve/ -run 'TestSandboxFlag|TestStaleSweep' && go build ./... && GOOS=linux CGO_ENABLED=0 go build ./...` | ❌ W0 | ⬜ pending |
| 22-06-02 | 06 | 5 | SAND-01 | T-22-19, T-22-26 | Foreground wrap through FULL flag path (live darwin denies); default-OFF argv byte-identity; --sandbox=off always escapes; dangerouslyDisableSandbox honored loudly both arms (OQ2); availability-false → per-run note (never silent fail-open); group-kill reaches wrapped child | unit + integration (live darwin) | `go test -race -count=1 ./internal/coreexec/ -run 'TestBashSandbox' && go test -race -count=1 ./internal/coreexec/` | ❌ W0 | ⬜ pending |
| 22-06-03 | 06 | 5 | SAND-01 | T-22-26, T-22-27 | Background bash confined under --sandbox=on via TaskRegistry.Start's own exec construction (network deny proves it — the checker BLOCKER battery); queued-start wraps identically; persistent shell confined at spawn + re-wrapped on restart (D-09 orthogonality); terminateGroup/Drain kill confined children; default-OFF byte-identical | unit + integration (live darwin) | `go test -race -count=1 ./internal/coreexec/ -run 'TestBackgroundSandbox|TestPTYSandbox' && go test -race -count=1 ./internal/coreexec/ ./internal/runtime/ && GOOS=linux CGO_ENABLED=0 go build ./...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*
*File Exists "❌ W0": every plan is TDD-style — the named test file is created RED-first inside the task itself, so the Wave 0 gap closes at the task's first commit, not before the wave.*

---

## Wave 0 Requirements

All satisfied in-plan (TDD-style tasks create their test files RED-first); nothing blocks before Wave 1:

- [ ] `internal/tasks/tracker_test.go`, `notify_test.go` — created by 22-01-01/02 (RED-first; covers PAR-07/PAR-08 notification half)
- [ ] `internal/sandbox/policy_test.go` (+ darwin/linux-tagged test files) — created by 22-05-01/02 (covers SAND-01 symmetry)
- [ ] `internal/coreexec/ptty_test.go`, `ansistrip_test.go` — created by 22-04-01/02 (covers PAR-09; darwin host executes the real-PTY batteries)
- [ ] `GOOS=linux` compile gate wired into per-task verify + the phase gate (runtime landlock/Pdeathsig tests stay linux-host-gated; skipped-by-tag on darwin)
- [ ] Rewrite `internal/coreexec/background_test.go` cap cases to the D-11 queue contract — 22-02-02 (existing coverage of errBgCap is zero; new TestBackgroundCap_* replaces)

*Existing infrastructure — TaskRegistry tests, cron/automation tests, config_surface tests, cmd/ass-guard test files — covers the surrounding rails.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Landlock runtime enforcement (real network + write deny through a confined child) | SAND-01 | Needs a Linux kernel ≥5.13 host (≥6.7 for TCP v4); dev host is darwin, no Docker | On a Linux host/CI: run `go test -race -count=1 ./internal/sandbox/ -run 'TestLandlock'` (tagged `//go:build linux`) + the linux arms of 22-06-03's batteries |
| Pdeathsig actual delivery on thread/process death (residual A4 race) | PAR-08 | Linux-only syscall; darwin cannot exercise it | On a Linux host: kill the ass-guard parent mid-background-task; assert no orphaned children remain |
| MPTCP blind spot (confined Go ≥1.24 child binding via MPTCP) | SAND-01 (D-05 note) | Documented accepted caveat (Pitfall 2); non-Go children unaffected | Informational: confirm tool children in the operator's environment are non-Go or accept the documented caveat |
| Live Zed ACP session smoke: wake turn arrives in a real editor session | PAR-07/PAR-08 (D-01) | Requires an interactive editor session; the runner battery (22-01-01) covers the rails | Optional operator check: spawn ass-guard from Zed, run a background command, observe the wake turn stream into the session |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify (every task has an automated command)
- [x] Wave 0 covers all MISSING references (all in-plan RED-first; listed above)
- [x] No watch-mode flags
- [x] Feedback latency < 30s per task commit (touched-package subset)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** planned (revision r1) 2026-08-28
