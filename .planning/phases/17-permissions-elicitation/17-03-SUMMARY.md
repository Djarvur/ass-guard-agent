---
phase: 17-permissions-elicitation
plan: 03
subsystem: permissions
tags: [acp, ask-queue, dialog-serialization, priority-classes, turn-death-drain, cancellation, concurrency, race-proofs]

# Dependency graph
requires:
  - phase: 17-permissions-elicitation (17-02)
    provides: the AskQueue minimal core (one-outstanding fire loop, class/seq-carrying entries), the gate chokepoint's suspend path (suspendForPermission → Enqueue), and resumePermissionAsk with the cancelled-normal branch
  - phase: 16-acp-wire-foundation (16-03)
    provides: the outbound Registry with ResolveCancelled + the $/cancel_request cascade (16-D-19) and the HUMAN-ASK class
provides:
  - "internal/session/askqueue.go — AskQueue full D-11..D-13 semantics: priority classes (earliest foreground preempts the waiting head, FIFO within class via Seq, an OPEN entry is never preempted), deterministic synchronous promotion of the first entry, D-12 notes ('ask queued — N pending' through an injected subscriber-backed emitter) + the Enqueued counter, AskEntry.SetFire, queue-owned per-firing fire ctx, DrainTurn(turnID)/DrainAll (the ONE shared drain), Open()/Pending() accessors"
  - "internal/session — Session.DrainPermissionAsks/HasOpenAsk/HasQueuedAsks (the teardown drain accessors)"
  - "internal/acp/handlers.go — the AskDrainer capability (DrainAsks/DrainSessionAsks) wired into handleSessionCancel (drain before/with cancelTurn) and handleLogout (drain before the reap)"
  - "internal/runtime — Runner.DrainAsks/DrainSessionAsks/DrainAllAsks (the capability implementation over the sessions map) + the D-12 note-emitter composition wiring (bus publish via the session-lifetime forwarder path)"
  - "internal/acpserve/acp_serve.go — the serve-shutdown drain composition (runner.DrainAllAsks before CloseAllSessions in the ctx-done goroutine)"
affects: [17-04-elicitation, 17-05-permission-docs, 21-hooks, ACP-01]

# Actuals (#2632) — chars/4 over the realized diff (71,242 chars), same scale as the plan's estimate
actuals:
  tokens: 17800
  tasks: 2
  commits: 5

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Deterministic promotion: the enqueue that finds the queue idle promotes its entry to the open slot synchronously, so notes and Pending() can never race the pump goroutine's pop"
    - "Queue-owned fire ctx: every fire receives a cancellable ctx created at promotion; the drain cancels it to resolve an OPEN dialog through the surface's own cancelled family (a registry-backed fire cascades $/cancel_request) — no id plumbing crosses the session/acp layer boundary"
    - "One shared drain: DrainTurn(turnID) is the only drain primitive; DrainAll is its per-turn loop; all three teardown paths (cancel notification, logout, serve shutdown) reduce to it"
    - "Subscriber-backed note emission: the queue calls the injected emitter synchronously at the enqueue site; composition (runtime sessionFor) binds it to the bus path the session-lifetime forwarder always serves (13-03 timing hazard dead by construction)"

key-files:
  created:
    - internal/session/askqueue_test.go
    - internal/acpserve/ask_drain_test.go
  modified:
    - internal/session/askqueue.go
    - internal/session/gate.go
    - internal/session/gate_test.go
    - internal/session/session.go
    - internal/acp/handlers.go
    - internal/acp/handlers_test.go
    - internal/acpserve/acp_serve.go
    - internal/acpserve/ask_surface.go
    - internal/acpserve/ask_surface_test.go
    - internal/runtime/runtime.go

key-decisions:
  - "The fire seam became ctx-aware (GateDeps.Fire / PermissionAsk.Fire / SetPermissionAskFire take the queue's per-firing ctx) — the only way the drain can resolve an OPEN registry-backed dialog without the session package importing internal/acp; nil ctx falls back to the serve-lifetime ctx"
  - "Promotion moved into Enqueue: the first entry leaves the waiting slice synchronously, making the D-12 note count and Pending() exact (the async-pump-pop version produced race-dependent 'N pending' counts — caught by the battery)"
  - "Cancel-path drain scope = the whole session's queue (open + queued): the ACP cancel contract covers ALL pending session/request_permission requests, and one-outstanding means the user-visible dialog is the session's; the turn-SCOPED drain (other turns untouched) is pinned at the queue level where the scoping lives"
  - "Serve shutdown drains BEFORE CloseAllSessions in the existing ctx-done goroutine — cascades ride a still-open writer (no closed-writer writes), and the explicit drain complements the registry's own Stop backstop"
  - "queued-but-unfired asks drain cancelled-normal THROUGH the resolve callback — their results land via the same loud-append resume seam the dialog outcome uses (no second append mechanism)"

patterns-established:
  - "Pattern: AskDrainer capability interfaces on TurnRunner (SessionCloser precedent) — optional teardown seams stub runners need not implement"
  - "Pattern: baseline+settle goroutine-leak checks for queue/serve teardown (the emitter-soak idiom)"

requirements-completed: []

# Coverage metadata (#1602) — one entry per shipped deliverable
coverage:
  - id: D1
    description: "D-11 serialization + priority: one outstanding fired ask (second enqueue queues, never stacks), foreground preempts the waiting head, FIFO within class, an OPEN entry never preempted"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/askqueue_test.go#TestAskQueueSerialization"
        status: pass
      - kind: unit
        ref: "internal/session/askqueue_test.go#TestAskQueuePriority"
        status: pass
      - kind: other
        ref: "go test -race ./internal/session/ -run TestAskQueue -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: "D-12 visible queue: immediate-fire emits no note; queued emits exactly 'ask queued — N pending' (N incl. itself) through the injected subscriber-backed emitter (no-subscriber case proven); the Enqueued counter counts every enqueue"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/askqueue_test.go#TestAskQueueNotes"
        status: pass
      - kind: unit
        ref: "internal/session/askqueue_test.go#TestAskQueueNoteNoSubscriber"
        status: pass
    human_judgment: false
  - id: D3
    description: "Concurrency invariants: 64 interleaved enqueues+resolves under -race hold one-outstanding (max 1 in flight), FIFO-within-class, no double-fire, no lost entry"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/askqueue_test.go#TestAskQueueConcurrency"
        status: pass
      - kind: other
        ref: "mise ci (go test -race ./...)"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-13 turn-scoped drain: the dead turn's OPEN entry resolves cancelled via the fire-ctx cancellation, queued entries drain cancelled-normal with zero fires, other turns' entries untouched"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/askqueue_test.go#TestAskQueueDrainTurn"
        status: pass
      - kind: unit
        ref: "internal/session/askqueue_test.go#TestAskQueueDrainAll"
        status: pass
    human_judgment: false
  - id: D5
    description: "Criterion 2 session-level: dialog held open, second prompt's ask queues, teardown drain appends cancelled-NORMAL results and resumes, exactly one surface fire through the whole lifecycle (firing monopoly / zero ask-method calls outside the queue fire step)"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/askqueue_test.go#TestGateTurnDeath"
        status: pass
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateChokepoint_SecondPromptWhileDialogOpen"
        status: pass
    human_judgment: false
  - id: D6
    description: "The ONE drain reachable from all three teardown paths, each wired: session/cancel (drain before/with cancelTurn), logout (drain before the reap), serve shutdown (drain before CloseAllSessions); cancel/logout wiring pinned by event-order tests, shutdown by the acpserve behavior battery (exactly one $/cancel_request with the ask's request id, no post-close writes, zero leaked goroutines)"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/acp/handlers_test.go#TestSessionCancelDrainOnCancel"
        status: pass
      - kind: unit
        ref: "internal/acp/handlers_test.go#TestSessionCancelDrainOnLogout"
        status: pass
      - kind: unit
        ref: "internal/acpserve/ask_drain_test.go#TestPermissionAskDrainCascade"
        status: pass
      - kind: unit
        ref: "internal/acpserve/ask_drain_test.go#TestAskQueueShutdownDrain"
        status: pass
      - kind: other
        ref: "acp_serve.go ctx-done composition (runner.DrainAllAsks before CloseAllSessions) — reviewed wiring; behavior covered by the acpserve battery"
        status: pass
    human_judgment: true
    rationale: "The literal Run()-level shutdown wiring is composition code (the runner is constructed inside Run and not injectable); its behavior is proven at the unit battery and the one-line wiring is review-verified. A full-Run gated-ask scripting pass would duplicate the 16-06 simulator's scope."

# Metrics
duration: 48 min
completed: 2026-09-01
status: complete
---

# Phase 17 Plan 03: Ask Queue Full Semantics + Turn-Death Drain Summary

**The D-11..D-13 dialog-serialization discipline: one outstanding ask with foreground priority and FIFO classes, a visible queue (notes + counter through a subscriber-backed emitter), and a turn-scoped drain wired to all three teardown paths — with the criterion-2 liveness proofs under -race**

## Performance

- **Duration:** 48 min
- **Started:** 2026-09-01T00:44:38Z
- **Completed:** 2026-09-01T01:32:36Z
- **Tasks:** 2
- **Files modified:** 12 (2 created, 10 modified)

## Accomplishments
- D-11 complete: one outstanding fired ask at a time; the head selection picks the earliest foreground entry (a later fg enqueue preempts the waiting bg head), FIFO within class by Seq, and an OPEN entry is never preempted regardless of class
- D-12 complete: the queue is user-visibly MOVING — immediate fires emit no note (spam dead by construction), queued enqueues emit exactly "ask queued — N pending" through an injected subscriber-backed emitter (composition binds it to the bus path the session-lifetime forwarder always serves — the 13-03 timing hazard cannot drop it), and the Enqueued counter (stall-metric family) counts every enqueue
- D-13 complete: `DrainTurn(turnID)` — the ONE shared drain — resolves the dead turn's OPEN dialog cancelled through the queue-owned fire-ctx cancellation (the registry-backed surface cascades $/cancel_request via 16-D-19) and drains queued-but-unfired asks cancelled-normal with zero fires; other turns untouched; reachable from all three teardown paths (session/cancel, logout, serve shutdown)
- Criterion 2 proven under -race: alive-while-asking (second prompt completes during an open dialog), turn death mid-ask delivers cancelled-NORMAL (never an error, never a hang), nothing holds a lock across the human wait, zero leaked goroutines and no post-close writes at shutdown, and the firing monopoly (zero ask-method calls outside the queue's fire step)

## Task Commits

Each task was committed atomically (TDD RED→GREEN), plus one blocking pre-existing repair:

0. **Pre-existing blocker:** `af9a4d6` fix(17-03): land 17-02's uncommitted ask-surface test repair (compile blocker)
1. **Task 1: Queue semantics — priority classes, visible notes, counter**
   - `bcc0941` test(17-03): add failing ask-queue semantics battery (RED)
   - `8092210` feat(17-03): expand the ask queue — priority classes, visible notes + counter (GREEN)
2. **Task 2: Turn-death drain on all teardown paths + criterion-2 proofs**
   - `4b3516b` test(17-03): add failing turn-death drain, firing-monopoly, and teardown-wiring battery (RED)
   - `f8a285d` feat(17-03): implement turn-scoped drain on all teardown paths + criterion-2 proofs (GREEN)

## Files Created/Modified
- `internal/session/askqueue.go` — the full D-11..D-13 queue: priority pop, synchronous promotion, openAsk{ctx,cancel}, SetNoteEmitter/Enqueued/Pending/Open, DrainTurn/DrainAll, SetFire, Session drain accessors
- `internal/session/askqueue_test.go` — the 8-test battery: serialization, priority (3 subtests), notes, no-subscriber, concurrency (64 ops), drain-turn, drain-all+leak, gate turn-death
- `internal/session/gate.go` — GateDeps.Fire threads the queue's per-firing ctx (the drain seam)
- `internal/session/gate_test.go` — fakeGateSurface Fire signature + hold mode; TestGatePermissionSuspend result-based wait (flake-family fix)
- `internal/session/session.go` — nolint-only net change (contextcheck resolved by the newOpenAsk refactor)
- `internal/acp/handlers.go` — the AskDrainer capability + drain calls in handleSessionCancel (before/with cancelTurn) and handleLogout (before the reap)
- `internal/acp/handlers_test.go` — TestSessionCancelDrainOnCancel/OnLogout (event-order wiring pins)
- `internal/acpserve/acp_serve.go` — the serve-shutdown drain composition (runner.DrainAllAsks before CloseAllSessions)
- `internal/acpserve/ask_surface.go` — PermissionAsk.Fire takes the queue's ctx (nil → serve ctx)
- `internal/acpserve/ask_drain_test.go` — the cascade + shutdown battery over a REAL registry
- `internal/acpserve/ask_surface_test.go` — fireAsync ctx threading (pre-existing uncommitted repair landed in af9a4d6)
- `internal/runtime/runtime.go` — the note-emitter composition wiring (subscriber-backed bus path), fire-callback ctx threading, Runner.DrainAsks/DrainSessionAsks/DrainAllAsks

## Decisions Made
- **Ctx through the fire seam:** the only way the drain can cancel an OPEN registry-backed round-trip without crossing the session/acp import boundary — GateDeps.Fire/PermissionAsk.Fire/SetPermissionAskFire all take the queue's per-firing ctx; the registry surfaces the cancellation as the cancelled family AND the D-19 cascade
- **Synchronous promotion:** the enqueue that finds the queue idle promotes its entry out of the waiting slice under the queue mutex, making note counts and Pending() deterministic (the async-pump-pop first draft produced race-dependent "N pending" counts — the RED battery caught it)
- **Cancel-path drain scope:** the whole session's queue — the ACP cancel contract covers ALL pending permission requests; the turn-SCOPED semantics (other turns untouched) live and are pinned at the queue level (DrainTurn), and DrainAll is literally its per-turn loop (one shared function)
- **Shutdown ordering:** the ctx-done goroutine drains BEFORE CloseAllSessions so cascades ride a still-open writer; the registry's Stop remains the backstop

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 17-02's uncommitted ask-surface test repair**
- **Found during:** execution start (working-tree inspection)
- **Issue:** HEAD's `internal/acpserve/ask_surface_test.go` called `NewPermissionAsk(reg, ctx, w)` against the production `(serveCtx, registry, stderr)` signature — the package could not compile at HEAD, blocking every `mise ci` gate
- **Fix:** landed the pending working-tree repair as its own commit (signature fix + its lint residue) before Task 1
- **Files modified:** internal/acpserve/ask_surface_test.go
- **Verification:** go build/vet + full battery green
- **Commit:** af9a4d6

**2. [Rule 3 - Blocking] Wiring seams omitted from the plan's file list**
- **Found during:** Task 1/2 GREEN
- **Issue:** the queue is created in `runtime.go sessionFor` (not the plan's file list) — the D-12 emitter composition and the Runner-side drain methods had no home in the listed files; `gate.go`/`ask_surface.go` needed the ctx-aware Fire signature; the acpserve cascade test demanded a new test file beside the REAL registry
- **Fix:** minimal touches — runtime.go (note wiring + drain methods), gate.go (Fire ctx param), ask_surface.go (Fire ctx param + nil fallback), new internal/acpserve/ask_drain_test.go, AskEntry.SetFire
- **Files modified:** internal/runtime/runtime.go, internal/session/gate.go, internal/acpserve/ask_surface.go, internal/acpserve/ask_drain_test.go (new)
- **Verification:** full battery + mise ci green
- **Committed in:** 8092210, f8a285d

**3. [Rule 1 - Bug] TestGatePermissionSuspend racy assertion (the 17-01/17-02 flake family, third recurrence)**
- **Found during:** first `mise ci` run (passed standalone, failed under full-suite load)
- **Issue:** the test asserted call2's transcript result immediately after observing the exec-order slice — noteExec lands INSIDE the tool's Execute, before the result append; the millisecond window widened under the heavier full-suite parallel load
- **Fix:** the gateWaitFor condition now includes both calls' appended results (not just the order slice); captured here per the deferred-items watchlist
- **Files modified:** internal/session/gate_test.go
- **Verification:** 5x standalone green + full `mise ci` green (0 FAIL lines)
- **Committed in:** f8a285d

---

**Total deviations:** 3 auto-fixed (1 pre-existing compile blocker, 2 wiring/lint-family). **Impact on plan:** all within the plan's architecture; no scope creep.

## Issues Encountered
- The load-sensitive millisecond-window flake family (17-01/17-02 deferred-items watchlist) recurred once in `mise ci` under the now-heavier full-suite load and was fixed at the assertion level (results-based wait). The final `mise ci` is green with zero FAIL lines.

## TDD Gate Compliance
- Both tasks followed RED→GREEN with `test(17-03)` commits preceding `feat(17-03)` commits (bcc0941→8092210, 4b3516b→f8a285d); no REFACTOR commits needed (lint-clean at each GREEN; the flake fix rode the Task 2 GREEN commit as a test-robustness repair).

## Known Stubs

None — every deliverable is wired end-to-end; no placeholder paths.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- 17-04 (elicitation) joins the same queue as its single firing path: enqueue with `AskClass` per origin, `SetFire` the elicitation surface, and the D-12 notes/counter come for free; degraded-client (no form capability) asks can decline or fall back per 16-D-13/D-18 before ever enqueueing
- The queue's `DrainTurn` scoping is ready for per-turn elicitation entries; engine/learning asks enqueue `AskClassBackground` and inherit priority + notes without new machinery
- 17-05's doc task can pin the queue contract (one outstanding, fg priority, notes+counter, turn-scoped drain) alongside the chokepoint audit
- Watchlist: the window-flake family has now recurred three times (17-01, 17-02, 17-03) — consider a shared polling helper with generous windows as a small cross-phase cleanup

---
*Phase: 17-permissions-elicitation*
*Completed: 2026-09-01*

## Self-Check: PASSED

- Created files verified on disk: internal/session/askqueue_test.go, internal/acpserve/ask_drain_test.go
- All 5 commits verified in git log: af9a4d6, bcc0941, 8092210, 4b3516b, f8a285d
- Plan-level verification re-run green: `go test -race ./internal/session/ ./internal/acp/ ./internal/acpserve/ -count=1`; final `mise ci` green with zero FAIL lines (vet + golangci-lint v2 0 issues + CGO_ENABLED=0 build + go test -race ./...)
- All Task 1/2 acceptance criteria individually re-verified (dedicated cases, no-subscriber note proof, 64-op concurrency, cascade exactly-once, three-path wiring, leak checks, monopoly, mise ci)
