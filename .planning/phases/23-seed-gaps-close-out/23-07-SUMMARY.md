---
phase: 23-seed-gaps-close-out
plan: 07
subsystem: acp
tags: [acp, json-rpc, session-new, available-commands, ordering, zed]

# Dependency graph
requires:
  - phase: 23-seed-gaps-close-out (plans 23-01..23-06)
    provides: the SEED-gap surfaces this closes over — steering queue, checkpoint restore, /undo class-B command table
provides:
  - session/new available_commands_update advertisement emitted strictly AFTER the response frame (G-23-1 fix)
  - per-request post-response emission slot in internal/acp dispatch (context-scoped, drained on every exit path)
  - two-level deterministic ordering battery pinning response-before-advertisement (dispatch level + real acpserve composition)
affects: [phase-25-kit-extraction, acp-wire-foundation, session-family]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 7375   # 29502 diff chars / 4 over acdb233~1..fa370f2
  tasks: 2        # tasks 1-2 executed; task 3 is the operator UAT leg (routed to 23-UAT.md via verify-work)
  commits: 2

# Tech tracking
tech-stack:
  added: []
  patterns:
    - per-request post-response emission slot (context key + defer-drained callbacks) — enqueue order is wire order on the FIFO Writer

key-files:
  created:
    - .planning/phases/23-seed-gaps-close-out/23-07-task1-red-evidence.json
  modified:
    - internal/acp/handlers.go
    - internal/acp/server.go
    - internal/acp/server_test.go
    - internal/acp/emitter_test.go
    - internal/acp/available_commands_test.go
    - internal/acp/handlers_test.go
    - internal/acpserve/serve_test.go
    - internal/acpserve/commands_seam_test.go
    - internal/acpserve/kit_emitter_e2e_test.go
    - .planning/phases/23-seed-gaps-close-out/deferred-items.md

key-decisions:
  - "Post-response emission via a per-request context-scoped slot drained by defer on every handleRequest exit path (writeResult and each writeError) — per-request isolation by construction, no server-wide shared queue, exported Handler signature unchanged"
  - "handleSessionLoad (:819-827) left byte-unchanged: Zed pre-registers the session on session/load (acp.rs:1258-1267) so its pre-response advertisement + Barrier ordering (16-01/18-05) is deliberate — the two paths are intentionally NOT unified"
  - "TestTurnEmitterEmptyTurn baseline extended to settle the empty-set advertisement frame that now lands after the session/new response (test intent unchanged)"

patterns-established:
  - "Ordering-by-enqueue: properties that must hold on the wire are pinned by enqueue order on the FIFO Writer, never by timers/sleeps/goroutine-race deferral"

requirements-completed: [SEEDG-03]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "session/new wire order is response-frame-first, advertisement-second, exactly one advertisement with the complete winner set — proven at both the internal/acp dispatch level and the real acpserve composition level"
    requirement: SEEDG-03
    verification:
      - kind: unit
        ref: "internal/acp/available_commands_test.go#TestSessionNewAdvertisesAfterResponse"
        status: pass
      - kind: integration
        ref: "internal/acpserve/serve_test.go#TestServeSessionNewCommandOrderAfterResponse"
        status: pass
    human_judgment: false
  - id: D2
    description: "Live-Zed operator confirmation: / command autocomplete lists ass-guard commands (incl. /undo); idle /undo instant restore; mid-turn /undo cancel-then-restore; cross-session refusal leg reaches the agent's resolver; 'unknown session' drop warnings gone from ACP logs"
    requirement: SEEDG-03
    verification: []
    human_judgment: true
    rationale: "The G-23-1 failure mode is client-side (Zed drops pre-response session/updates for unregistered sessions, zed#60199) — only a live editor session can prove the operator-visible surface; this is plan Task 3 (blocking gate) and mirrors 23-UAT.md tests 1-2, discharged via /gsd-verify-work 23"

# Metrics
duration: 35min
completed: 2026-09-12
status: complete
---

# Plan 23-07: G-23-1 session/new advertisement ordering Summary

**session/new available_commands_update moved behind the response frame via a per-request post-response slot, with a red-green two-level ordering battery proving Zed's registration window is respected**

## Performance

- **Duration:** ~35 min (RED battery 00:09, GREEN fix 00:36 local)
- **Started:** 2026-09-11T21:05Z
- **Completed:** 2026-09-11T21:40Z (SUMMARY close-out 2026-09-14 — execution session was interrupted after the fix commit; close-out verified GREEN at HEAD before writing this file)
- **Tasks:** 2 of 3 (Task 3 is the operator UAT leg — see coverage D2)
- **Files modified:** 11

## Accomplishments
- G-23-1 root cause closed at the wire level: `handleSessionNew` no longer emits the advertisement before the response — emission registers on a context-scoped per-request slot that `handleRequest` drains (defer, all exit paths) only after the response frame's enqueue; the FIFO Writer makes enqueue order wire order deterministically.
- RED battery landed first and failed for the ordering reason at both levels (evidence: `23-07-task1-red-evidence.json`; load-path pins `TestReplay|TestSessionLoad` stayed green, isolating the RED to session/new).
- Stale comments rewritten truthfully (`handleSessionNew` emission-site block, `server_test.go` `readResultFrame` doc); `TestTurnEmitterEmptyTurn` baseline extended for the post-response advertisement.
- Load path untouched: `handleSessionLoad`, `ReplayTranscript`, `NotifyAvailableCommands`, `NotifyAllAvailableCommands`, 20-05 rescan wiring all byte-stable.

## Task Commits

1. **Task 1: RED battery — pin response-BEFORE-advertisement** - `acdb233` (test)
2. **Task 2: GREEN — post-response emission via per-request slot** - `fa370f2` (fix)
3. **Task 3: Live-Zed operator UAT** — pending operator; routed to `23-UAT.md` re-run via `/gsd-verify-work 23` (blocking gate, by design not agent-executable)

## Files Created/Modified
- `internal/acp/handlers.go` - handleSessionNew: pre-response Notify + Barrier removed; emission registered on the post-response slot; comment states the new order + zed#60199 reason
- `internal/acp/server.go` - per-request post-response slot: created in handleRequest, ctx-derived, defer-drained on every exit path
- `internal/acp/server_test.go` - readResultFrame doc rewritten (load path still pre-response; helper stays correct)
- `internal/acp/emitter_test.go` - TestTurnEmitterEmptyTurn baseline settles the empty-set advertisement
- `internal/acp/available_commands_test.go` - dispatch-level ordering battery (TestSessionNewAdvertisesAfterResponse)
- `internal/acp/handlers_test.go` - handler-shape witnesses for the new emission site
- `internal/acpserve/serve_test.go` - composition-level ordering battery (TestServeSessionNewCommandOrderAfterResponse; undo advertised)
- `internal/acpserve/commands_seam_test.go`, `internal/acpserve/kit_emitter_e2e_test.go` - seam/e2e witnesses adjusted to the new order
- `.planning/phases/23-seed-gaps-close-out/deferred-items.md` - D-23-07-1 (pre-existing simulator triple), D-23-07-2 (modesmatrix flake)

## Decisions Made
- Per-request context-scoped slot over a server-wide queue (second session/new could otherwise gate on the first's response) and over special-casing the method after writeResult (~10-line alternative rejected to keep the dispatch uniform).
- The two ordering batteries assert on the settled decoded frame sequence, not poll races.

## Deviations from Plan

None - plan executed exactly as written for Tasks 1-2. (Close-out note: the execution session was interrupted between the Task 2 commit and SUMMARY creation; this SUMMARY was written by the resumed orchestrator after verifying at HEAD that both ordering tests pass, `internal/acp` full `-race` is green, and `internal/acpserve` full `-race` fails only the documented pre-existing triple — see Issues.)

**Total deviations:** 0
**Impact on plan:** None.

## Issues Encountered
- Pre-existing environment-dependent failures documented in `deferred-items.md` (D-23-07-1: `TestZedSimulatorE2E`, `TestSimulatorCommandSurface`, `TestPermissionsE2E` under `-race`; verified byte-identical on clean RED-commit HEAD in a detached worktree — zero new failures from 23-07; suspected `ANTHROPIC_BASE_URL`-family env override reaching the simulator harnesses). Re-confirmed at close-out: failure set at HEAD is exactly this triple, nothing else.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Wire-level fix is landed, committed, and test-proven; the only outstanding item for Phase 23 completion is the operator UAT re-run (`/gsd-verify-work 23`) against a freshly built binary — it doubles as plan Task 3's blocking gate.
- After UAT passes, Phase 23 closes and the milestone's final steps (`/gsd-verify-work` acceptance pass + `/gsd-complete-milestone`) unblock.

---
*Phase: 23-seed-gaps-close-out*
*Completed: 2026-09-12*
