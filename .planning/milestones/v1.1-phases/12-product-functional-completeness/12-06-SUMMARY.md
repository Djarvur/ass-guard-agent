---
phase: 12-product-functional-completeness
plan: "06"
subsystem: acp
tags: [acp-05, acp-06, background-tasks, taskregistry, run-in-background, captured-forms]

requires:
  - phase: 12-product-functional-completeness
    provides: "12-05's re-record fixture (the captured background_start + TaskOutput not_ready forms — pinned BEFORE this plan executed, as the plan's truths anticipated)"
provides:
  - "Bash run_in_background executes: immediate return with the CAPTURED start form (exec_<uuid> + the real progressive log under .ass-guard/outputs/), the registry owning the process group from the start"
  - "TaskOutput executes: block/timeout per the schema, the CAPTURED not_ready XML form for running tasks, the documented ready form for finished ones, structured unknown-id errors"
  - "TaskStop executes: whole-group SIGKILL + the reap loop, deprecated shell_id alias, per-session scoping"
  - "TaskRegistry: the per-session owner (16-task cap, bounded 1MB accumulation buffers, ReapAll chained into OnClose — no orphan groups outlive the session)"
  - "TestBackgroundWiring_CoreCompleteness — the landed-so-far dead-end-removal gate (non-mcp core tools outside the cron quartet + the documented by-design routes)"
affects: [12-07, 12-08]

actuals:
  tokens: 46000   # chars/4 over the plan's production commits (estimate was 68000)
  tasks: 2
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Registry-owned lifecycle: the background cmd carries NO ctx Cancel (the Start/Kill pair owns it) — the exec.Cmd Cancel field requires CommandContext, so plain Start + explicit group kill is the correct split"
    - "Stream-tagged accumulation buffers (stdout/stderr separate) combined stdout-before-stderr at retrieval — the captured combined discipline applied to the accumulated read"
    - "The tee'd progressive log: the start form's path is live from the first byte (the form's 'use Read on that file path' guidance is real)"

key-files:
  created:
    - internal/coreexec/background.go
    - internal/coreexec/background_test.go
    - cmd/ass-guard/background_wiring_test.go
  modified:
    - internal/coreexec/bash.go
    - internal/coreexec/bash_test.go
    - internal/coreexec/register.go
    - internal/coreexec/planmode.go (InteractiveConfig.Tasks)
    - cmd/ass-guard/acp_serve.go

key-decisions:
  - "The background forms were ALREADY PINNED by 12-05's fixture (this plan's truths anticipated the ordering): background_start (32 obs) + TaskOutput not_ready (30 obs) render from the capture; the ready + stop-ack forms are the documented corpus-absent defaults"
  - "No cmd.Cancel on background tasks: the REGISTRY owns the lifecycle (Stop/ReapAll kill the group); a Cancel func on a plain-Start cmd is rejected by os/exec — the discipline reuses Setpgid + explicit kill"
  - "The 16-task cap + 1MB accumulation bounds are documented corpus-absent defaults (flagged in the 12-05 fixture's corpus_absent list)"
  - "The completeness gate's exceptions are BY-DESIGN routes, not dead ends: WebSearch/WebFetch (RealExecutor backend routing — intercepted before the catalog), Agent (pre-batch subagent dispatch), Skill (the wiring-site closure proven by its own battery)"

requirements-completed: [ACP-05, ACP-06]

duration: 88min
completed: 2026-08-20
status: complete
---

# Phase 12 Plan 06: Background work — Bash run_in_background + TaskOutput + TaskStop Summary

**Background work is a first-class citizen: the per-session TaskRegistry (capped, bounded, reaped-on-close process groups with progressive logs) behind the CAPTURED immediate-return start form, the captured not_ready retrieval form, and honest structured errors — the background trio executes, and the completeness gate proves every core tool outside the cron quartet + the documented by-design routes is live. The pre-adoption minimal set is COMPLETE.**

## Performance

- **Duration:** 88 min
- **Tasks:** 2 (TDD: RED c647fa5 → GREEN 37e9aef; wiring battery 0b63732)
- **Files:** 8 (3 created, 5 modified)

## Accomplishments

- **The registry:** Start (own process group, no ctx Cancel — the registry owns the lifecycle; progressive tee into `.ass-guard/outputs/exec_<uuid>.log`; the 16-task cap with structured over-cap errors), Output (bounded blocking per the schema's timeout; snapshot non-blocking; separate stdout/stderr buffers combined stdout-before-stderr at retrieval), Stop (group SIGKILL + the reap loop), ReapAll (chained into OnClose beside the MCP reaper — T-12-06-02 proven at the wiring level).
- **Bash run_in_background:** the immediate-return branch with the CAPTURED form (32 obs — exec_<uuid> id + the real log path + the Read-guidance line); the foreground path byte-unchanged; dangerouslyDisableSandbox's doc upgraded from 'accepted-and-ignored' to the by-design no-op.
- **TaskOutput/TaskStop executors:** the CAPTURED `<retrieval_status>not_ready</retrieval_status>` XML form (30 obs) for running tasks; the documented ready form for finished ones; unknown ids error structurally (per-session scoping proven cross-session); the deprecated shell_id alias resolves.
- **The completeness gate:** every non-mcp core tool outside the cron quartet + the documented by-design routes carries Execute — the growing phase-gate grep equivalent, naming 12-07 as the remaining scope.

## Deviations from Plan

**1. [Scope refinement] The completeness test's exception set**
- The plan's Test 5 expected Execute != nil for everything outside the cron quartet; four tools execute through BY-DESIGN non-catalog routes established in v1.0/Phase-8 (WebSearch/WebFetch backend routing, Agent pre-batch dispatch, Skill wiring closure). The test documents each exception with its seam — the "no implementation yet" fallback provably cannot fire for them.

**2. [Scope note] Wiring-site location** — `internal/runtime/runtime.go` in the plan's files; the real site is `cmd/ass-guard/acp_serve.go` (the same anticipated deviation as 12-04).

## Known Stubs

None — the ready/stop-ack forms are documented corpus-absent defaults (flagged in the 12-05 fixture with the hunts).

## Self-Check: PASSED

- internal/coreexec/background.go, cmd/ass-guard/background_wiring_test.go — FOUND
- Commits c647fa5 / 37e9aef / 0b63732 — FOUND
- `mise ci` green (lint 0 + the full battery incl. the new wiring tests)
