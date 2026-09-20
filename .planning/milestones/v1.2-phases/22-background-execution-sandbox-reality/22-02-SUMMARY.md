---
phase: 22-background-execution-sandbox-reality
plan: 02
subsystem: background-execution
tags: [background-bash, escalation, pdeathsig, queue-on-cap, configoptions, stale-sweep]

requires:
  - phase: 22-01
    provides: the task-notification subsystem (CompletionHook, tracker) this plan's registry extensions feed
provides:
  - TERM-before-KILL escalation ladder (terminateGroup) on every background termination path + Linux Pdeathsig
  - D-11 queue-on-cap TaskRegistry (FIFO waiters, bounded 64, queued-note form, queued Stop/Output semantics, ReapAll waiter drop)
  - D-12 configOptions menu entries background.subagents (8) / background.bash (16) with apply-as-landed wiring (SetBackgroundCaps seam)
  - PAR-08/OQ4 startup stale-log sweep (tombstone .stale rename, 7-day windowed GC, counted note)
affects: [22-03, 22-04, 22-06, coreexec, acpserve]

tech-stack:
  added: []
  patterns:
    - "terminateGroup funnel: every kill path escalates TERM → grace → KILL + reap through one helper — no drift between call sites"
    - "apply-as-landed caps: config surface resolves, next sessionFor construction consumes — no mid-session mutation"

key-files:
  created:
    - internal/coreexec/procopts_linux.go
    - internal/coreexec/procopts_other.go
    - internal/coreexec/procopts_linux_test.go
  modified:
    - internal/coreexec/background.go
    - internal/coreexec/background_test.go
    - internal/coreexec/bash.go
    - internal/acpserve/config_surface.go
    - internal/acpserve/config_test.go
    - internal/acpserve/config_surface_lock_test.go
    - internal/acpserve/acp_serve.go
    - internal/acpserve/serve_test.go
    - internal/runtime/runtime.go

key-decisions:
  - "exitStatusFor refinement: a TERM-trapped child's CHOSEN exit code (trap \"exit 7\" TERM) reports \"7\" even in the stopped state — only unchosen deaths (signaled) report \"killed\""
  - "Start's signature grew to (id, queued, error): the queued id is minted at dispatch so Stop/Output work immediately; bashBackgroundStart renders the queued-note form (renderBackgroundQueued, the renderBackgroundStart prose family)"
  - "Caps are injected (TaskRegistry.Cap / TrackerOpts.SubagentCap) via Runner.SetBackgroundCaps(surface.EffectiveBackgroundCaps) — the runner field is nil-safe with 8/16 defaults"
  - "Escalation tests need ready-gates: a Stop racing the child's exec finds the TRAP uninstalled (default TERM death, no observation) — the batteries wait for a ready marker before stopping"
  - "Sweep counts returned as a struct; the Run pipeline step logs one counted stderr note and never refuses serve"

patterns-established:
  - "queued task = full registry citizen: bgQueued state, not_ready TaskOutput envelope with <status>queued</status>, Stop removes from FIFO with the stillQueued guard in startNextWaiter"

requirements-completed: [PAR-08]

duration: 105 min
completed: 2026-09-07T23:20:00Z
---

# Phase 22 Plan 02: Background Bash Hardening Summary

**Background Bash now escalates TERM→grace→KILL on every path (Linux children carry Pdeathsig), over-cap starts queue FIFO with a visible note under operator-tunable caps in the configOptions menu, and serve start tombstone-sweeps orphaned task logs.**

## Performance

- **Duration:** 105 min
- **Started:** 2026-09-07T21:35:00Z
- **Completed:** 2026-09-07T23:20:00Z
- **Tasks:** 3
- **Files modified:** 12

## Accomplishments

- terminateGroup (PAR-08 Pattern 6): SIGTERM group → grace (5s default, test-injectable termGrace) → SIGKILL group + bounded reapGroup; ESRCH-clean at every step; reap on both exits (TERM deaths can leave fork-race stragglers). Stop, ReapAll all funnel through it.
- procopts_linux.go/procopts_other.go: setPdeathsig (SIGKILL, go.dev/issue/27505 caveat documented) applied at Start's SysProcAttr; linux-tagged attr test.
- Queue-on-cap (D-11): Start returns (id, queued, err); over-cap joins FIFO waiters (bgQueueBound 64 keeps the errBgCap path for pathological growth); completion drains head-first; queued Stop/Output semantics; ReapAll drops waiters with count (OQ5).
- D-12 menu: background.subagents/background.bash (+ global twins) with offered sets 4/8/16/32 and 8/16/32/64; setBackgroundCapLocked (validate ≥1 → WR-05 idempotence guard → persist int); EffectiveBackgroundCaps for the runtime seam; Runner.SetBackgroundCaps + sessionFor injection.
- Stale sweep (OQ4): sweepStaleTaskLogs tombstones orphaned *.log (rename .stale), GCs .stale past the 7-day window, skips foreign entries, never removes the directory; Run pipeline step before StartScheduler.

## Verification Evidence

- `go test -race -count=1 ./internal/coreexec/` — PASS (escalation + cap batteries + the pre-existing six).
- `go test -race -count=1 ./internal/acpserve/ -run 'TestStaleSweep|TestConfigSurface_BackgroundCapsMenu'` — PASS.
- `GOOS=linux CGO_ENABLED=0 go build ./... && GOOS=linux CGO_ENABLED=0 go vet ./internal/coreexec/` — PASS.
- `grep -c terminateGroup internal/coreexec/background.go` = 7; `grep -c errBgCap` ≥ 2; `grep -c Pdeathsig procopts_linux.go` = 4.

## Deviations from Plan

- **[Rule 1 — test correctness] Escalation ready-gates.** The plan's ladder tests assumed Stop-after-Start is race-free; in reality Stop can beat the child's exec (trap uninstalled → default TERM death, no observation possible). Fix: ready-marker gates before Stop in the TERM-immune/TERM-respecting/ReapAll batteries.
- **[Concurrent-workstream note, not a deviation] Phase 23 entanglement.** The concurrent Phase 23 executor's commit 40b2bbc (steering ingress) swept this plan's then-uncommitted runtime.go hunks (SetBackgroundCaps + cap wiring) into their commit — my Task 2 commit therefore carries only the coreexec/acpserve files. Additionally: (a) their in-flight config_surface.go edits add 4 more menu rows (checkpoint options — 20 total), temporarily failing the committed menu-count tests until they land; (b) their committed steering change REGRESSES TestPermissionsE2E (verified in an isolated baseline worktree: passes at 721c7bc, fails at dbbb5d0 without any of my acpserve changes). Recorded in STATE.md for the operator.

**Total deviations:** 1 auto-fixed (Rule 1) + 1 environmental note. **Impact:** none on plan contracts.

## Issues Encountered

- TestPermissionsE2E (+ several serve E2E tests under parallel load) fail from the CONCURRENT Phase 23 workstream's committed steering change — verified not caused by this plan (isolated-worktree bisection). Surfaced to STATE.md.

## Self-Check: PASSED

## Next Phase Readiness

Ready for 22-03 (subagent leg) — the tracker cap injection and notification spine are in place.
