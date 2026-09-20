---
phase: 23-seed-gaps-close-out
plan: 01
subsystem: session
tags: [steering, concurrency, transcript, projector, ticket-queue]

requires:
  - phase: 16-acp-wire-foundation
    provides: additive transcript-kind discipline (16-D-20), REDACTED append path
  - phase: 18-session-family
    provides: transcript-as-truth replay parity (18-D-01), recordCanceled funnel
provides:
  - transport-neutral SteerQueue (Enqueue/Drain/CancelThrough/CancelAll/Pending) in internal/session
  - steering_delivery transcript kind + Manager.AppendSteeringDelivery (sole writer, REDACTED path)
  - Session.SetSteerQueue + runTurn iteration-top boundary drain (one coalesced marker-wrapped user-role message)
  - live note "steering applied: N inputs" via the AgentMessageChunk bus path
  - cancelled-exit queue resolution (anti-zombie) at the recordCanceled funnel
  - Projector steering fold (arrival position, after flushBatch, anchor untouched)
affects: [23-02, 23-05, TG-02 (v1.3 Telegram peer)]

actuals:
  tokens: 12400
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "ticket/cutoff queue on the AskBroker structural shell (single mutex, no channels)"
    - "boundary drain at the runTurn iteration top — after results appended, before Project (pair-safety structural)"

key-files:
  created:
    - internal/session/steerqueue.go
    - internal/session/steerqueue_test.go
    - internal/session/steering_test.go
  modified:
    - internal/session/transcript.go
    - internal/session/manager.go
    - internal/session/session.go
    - internal/session/projector.go

key-decisions:
  - "The steering marker renders as one system-reminder-shaped block wrapping each input in <user-input> tags — ONE block, ONE user-role message, inputs in arrival order (CONTEXT discretion, corpus_scan convention)"
  - "Cancelled inputs acknowledge via CancelThrough/CancelAll return count + stderr slog note — NEVER a steering_delivery line, so a cancelled input cannot reach a later model window live or on replay"
  - "The cancelled-exit resolution lives in recordCanceled (all three runTurn cancelled returns funnel there); panic-recovered exits and parked-chain teardowns are 23-02/23-05's explicit calls (the key link's no-live-funnel half)"
  - "The fold case lives in foldExchanges (shared by mid-turn and compaction-tail paths) so replay and post-marker tails carry steering identically; the anchor loop stays TypeUserMessage-only by construction"
  - "A failed steering_delivery append is loud (stderr) and the input is NOT delivered to the model — never turn-fatal (AUD-03)"

patterns-established:
  - "Boundary-drain seam: drain between the ctx.Err() check and Project at the runTurn iteration top — the only point where pairs are closed and no request is in flight"
  - "Anti-zombie funnel: queue resolution composes into recordCanceled so turn death deterministically resolves undelivered steering"

requirements-completed: [SEEDG-01]

coverage:
  - id: D1
    description: "Steering enqueued mid-turn reaches the model as a marker-wrapped user-role message at the NEXT request boundary with the iteration-1 tool pair intact"
    requirement: SEEDG-01
    verification:
      - kind: unit
        ref: "internal/session/steering_test.go#TestSteeringDeliveryEndToEnd"
        status: pass
    human_judgment: false
  - id: D2
    description: "Ticket/cutoff concurrency contract: monotonic tickets, arrival-order coalesced drain, exact-cutoff cancel, delivered+cancelled==assigned under -race, zero ACP-wire imports"
    requirement: SEEDG-01
    verification:
      - kind: unit
        ref: "internal/session/steerqueue_test.go#TestSteerQueueHammerConservation"
        status: pass
      - kind: unit
        ref: "internal/session/steerqueue_test.go#TestSteerQueueNoACP"
        status: pass
    human_judgment: false
  - id: D3
    description: "Projector folds steering anchor-safely with live/replay parity and legacy tolerance"
    requirement: SEEDG-01
    verification:
      - kind: unit
        ref: "internal/session/steering_test.go#TestSteeringProjectAnchorSafety"
        status: pass
      - kind: unit
        ref: "internal/session/steering_test.go#TestSteeringReplayParity"
        status: pass
    human_judgment: false
  - id: D4
    description: "Cancelled turns resolve undelivered steering (anti-zombie): nothing reaches a later turn's window"
    requirement: SEEDG-01
    verification:
      - kind: unit
        ref: "internal/session/steering_test.go#TestSteeringAntiZombie"
        status: pass
    human_judgment: false

duration: 38 min
completed: 2026-09-08
status: complete
---

# Phase 23 Plan 01: Steering Queue Core Summary

**Transport-neutral SteerQueue delivering queued steering into the next model request as one marker-wrapped user-role message, folded anchor-safely by the Projector with anti-zombie cancellation.**

## Performance

- **Duration:** 38 min
- **Tasks:** 3/3
- **Files:** 7 (3 created, 4 modified)

## Accomplishments

- SteerQueue (`internal/session/steerqueue.go`): single-mutex ticket queue modeled on AskBroker — Enqueue (strictly increasing tickets from 1), Drain (arrival-order coalesced batch, watermark advances by removal), CancelThrough/CancelAll (cancelled-normal resolution), Pending (inspection). Nil-receiver safe; zero non-stdlib imports (transport-neutral by construction, TG-02).
- Transcript kind `steering_delivery` + `Manager.AppendSteeringDelivery(turnID, text, count)` through the REDACTED path, count as `{"count":N}` in Input; sole-writer anti-spoofing (agent-appended only).
- `Session.SetSteerQueue` + `drainSteering` at the runTurn iteration top (between ctx.Err() check and maybeCompact/Project): renders ONE marker block (`renderSteeringMarker` — system-reminder-shaped, one block, one message), appends the transcript line, emits the live note "steering applied: N inputs" through the AgentMessageChunk bus path.
- Cancelled exits: `recordCanceled` resolves the queue via CancelAll with a stderr note — undelivered steering can never zombie-deliver into the next turn's window.
- Projector: `TypeSteeringDelivery` case in `foldExchanges` folds as user-role in arrival position after `flushBatch()`; anchor loop (`turnAnchorOf`) untouched.

## Task Log

| Task | Commit | Verification |
|------|--------|--------------|
| T1 tracer: end-to-end steering delivery | b397954 | TestSteeringDeliveryEndToEnd + TestSteeringAntiZombie green under -race; whole package green |
| T2 tdd: ticket/cutoff battery | 1b90b51 | TestSteerQueue* battery green under -race (8-producer hammer conservation) |
| T3 tdd: projector fold hardening | 3061d84 | TestSteeringProject* + TestSteeringReplayParity green; whole package -race green |

## Deviations from Plan

- **[Rule 2 - task split] Pending() landed with the Task-1 tracer commit** — Found during: Task 1 | The accessor is three lines on the type the tracer already builds; deferring it would have meant a commit that adds an untested method. | Fix: included in b397954; the Task-2 battery pins it (TestSteerQueuePending) | Files: steerqueue.go | Verification: battery green | Commit: b397954
- **[Note - plan line drift] Plan's read_first line anchors (session.go:349-394, projector.go:170-227) reference an older snapshot** — the loop top and fold logic moved (fold now lives in shared `foldExchanges`, used by both mid-turn and compaction-tail paths). The fold case was added in `foldExchanges` so both paths carry steering identically (replay-consistent); `accumulateMidTurn` inherits it. No behavioral difference vs. the plan's intent.
- **[Note - TDD shape] Tasks 2/3 batteries pin rather than drive** — the Task-1 tracer's end-to-end test required the full queue + fold, so the batteries landed green on first run instead of RED→GREEN. Their regression value is the point (anchor-safety goes red under a user_message-kind regression; conservation goes red under a queue race).

**Total deviations:** 1 auto-fixed (task split), 2 documented notes. **Impact:** none — all acceptance criteria green.

## Issues Encountered

None.

- `go vet ./internal/session/` clean; `go build ./internal/session/ ./internal/checkpoint/` clean; `go test -race -count=1 ./internal/session/` fully green.
- golangci-lint could not run: the installed 2.12.x typecheck panics against Go 1.27.1 stdlib (pre-existing environmental issue, STATE.md LINT BASELINE note) — gates rest on vet/build/tests as prior phases did.
- `go build ./...` currently fails in `internal/runtime/cron_wiring.go` — the concurrent Phase 22 executor's in-flight uncommitted work (not this plan's surface; this plan touches only internal/session).

## Self-Check: PASSED
