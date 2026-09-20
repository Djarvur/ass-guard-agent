---
phase: 22-background-execution-sandbox-reality
plan: 03
subsystem: background-execution
tags: [background-subagents, async-dispatch, par-07, cancellation, output-retrieval]

requires:
  - phase: 22-01
    provides: the tracker (RegisterSubagent/Complete/CancelTask), notification spine, wake drain
  - phase: 20-03 (landed)
    provides: SubagentModelPlanner — the single resolution call site both dispatch modes share
provides:
  - Discriminated background Agent dispatch: async_launched JSON result, serve-lifetime detach, queued form (D-10)
  - internal/tasks/subagent.go — RunBackgroundSubagent (id mint, tracker registration, progressive output file with killed/completed markers, rune-boundary buffering, bounded tail)
  - Background ask-decline (OQ3): ctx-scoped marker, ask-class tools decline with the 17-D-07 note
  - Pattern-7 seam pin: planSubagent documented as the ONE resolution site for both modes
affects: [22-04, 22-06, session-dispatch, tasks]

tech-stack:
  added: []
  patterns:
    - "ctx-marker scoping (ContextWithBackgroundSubagent): background-only behavior rides the loop's ctx — never session-global state (a concurrent client turn is unaffected)"
    - "bus-backed progress tee: the launcher subscribes the subagent's turn-tagged events and streams them into the task's output file"

key-files:
  created:
    - internal/tasks/subagent.go
    - internal/tasks/subagent_test.go
  modified:
    - internal/session/subagent.go
    - internal/session/session.go
    - internal/session/subagent_test.go
    - internal/runtime/runtime.go

key-decisions:
  - "The session stays tasks-free: BackgroundDispatch is a func seam on Session; the runtime adapter owns the tasks.RunBackgroundSubagent call (the SetPermissionAskFire precedent)"
  - "Ask-decline at executeRestricted's head via the ctx marker — the default restricted set already excludes ask tools; custom agent defs reaching them now decline with the note instead of firing a dialog nobody answers"
  - "Exactly one terminal marker per output file: the writer's finish() is the single marker call site; the cancel-vs-complete race resolves by the cancelled flag at the goroutine's terminal point"
  - "subagentWriter holds back trailing incomplete runes (writeRuneBuffered) — the file only ever receives whole runes; utf8.Valid pinned by a split-at-every-byte battery"

patterns-established:
  - "Discriminated result vocabulary: {status: async_launched, task_id, output_file} (+ queued/note for the D-10 form) — the discrimination is the status field"

requirements-completed: [PAR-07]

duration: 80 min
completed: 2026-09-08T00:15:00Z
---

# Phase 22 Plan 03: Background Subagents Summary

**run_in_background Agent dispatches now return a discriminated async_launched result immediately, run detached under the serve-lifetime ctx with tracker-mediated cancellation, stream progress to a resolvable output file, and inherit the Phase-20 model routing through the one shared call site.**

## Performance

- **Duration:** 80 min
- **Started:** 2026-09-07T22:55:00Z
- **Completed:** 2026-09-08T00:15:00Z
- **Tasks:** 3
- **Files modified:** 7

## Accomplishments

- tasks/subagent.go: RunBackgroundSubagent — crypto/rand exec_ id, output file with header from t=0, tracker registration (queued-or-launch with the D-10 note), the loop under ServeCtx with a per-task cancel func, terminal markers (completed/error/killed) BEFORE the KindSubagent notification (the D-02 tail includes the marker), rune-boundary-buffered appends, bounded in-memory tail window.
- session: wantsBackgroundDispatch parsing (the schema-advertised field), DispatchSubagentBackground (same planSubagent resolution + AppendSubagentDispatch discipline + the discriminated JSON payload), the dispatch-site branch ahead of the synchronous path, the BackgroundDispatch func seam on Session.
- runtime: the launcher adapter wiring tracker + serveCtx + the bus-backed progress tee (AgentMessageChunk/ToolCall events tagged with the subagent's turn id → file lines), the ask-decline ctx marker armed around the background loop.
- OQ3: executeRestricted declines ask-class tools under the background ctx marker with the "no human present" note — ctx-scoped, never session-global.

## Verification Evidence

- `go test -race -count=1 ./internal/session/ -run 'TestDispatchBackground'` — PASS (async_launched before completion; turn-ctx death leaves the loop alive; foreground parity; queued form).
- `go test -race -count=1 ./internal/tasks/ -run 'TestBackgroundOutput|TestBackgroundCancel'` — PASS (mid-run retrieval; killed marker + exactly-one killed notification; zero-output probe; split-rune utf8.Valid).
- `go test -race -count=1 ./internal/tasks/ ./internal/session/` — PASS whole packages (foreground routing untouched).
- `grep -c 'async_launched' session/{subagent,session}.go` ≥ 2; `grep -c '20-03\|Phase-20' internal/session/subagent.go` = 9; zero resolver symbols in internal/tasks.

## Deviations from Plan

- **[Rule 1 — test-infra race] gatedSubagentRunner** required atomic counters + buffered started channel — the fg+bg routing battery shares one fake across goroutines (race detector catch; production code unaffected).
- **[Discretion] TestDispatchBackground_ForegroundParity** asserts the negative (no async_launched on the foreground path) + the seam equality lives in TestDispatchBackgroundRouting — the plan's "same tool-result shape" assertion rides the pre-existing TestSubagent_FinalResultToParent battery unchanged.

**Total deviations:** 1 auto-fixed, 1 discretion. **Impact:** none.

## Issues Encountered

None (beyond the Phase-23 cross-workstream items already recorded in STATE.md).

## Self-Check: PASSED

## Next Phase Readiness

Ready for 22-04 (persistent shell) — independent of this plan's surfaces except runtime.go ordering.
