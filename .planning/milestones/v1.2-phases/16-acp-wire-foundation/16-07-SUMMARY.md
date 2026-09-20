---
phase: 16-acp-wire-foundation
plan: 07
subsystem: acp
tags: [acp, emitter, barrier, concurrency, lost-wakeup, broadcast, tdd]

# Dependency graph
requires:
  - phase: 16-acp-wire-foundation (16-01)
    provides: TurnEmitter primitive (lanes, drain, Barrier updates-before-response) that CR-01 flagged
  - phase: 16-acp-wire-foundation (16-VERIFICATION)
    provides: code-confirmed gap 1+2 definitions and the verifier-named broadcast fix
provides:
  - Per-generation broadcast wake in TurnEmitter (close + fresh-channel swap under mu) — every parked Barrier waiter re-checks after each written frame
  - TestTurnEmitterBarrierConcurrentWaiters regression: two connection-lifetime-ctx Barriers satisfied by the same final frame both return; lone-waiter wake preserved
  - Closed 16-VERIFICATION gaps 1 and 2 (16-01 truths 5 and 6, both FAILED → proven)
affects: [17-interactive-asks, 18-sessions, 19-compaction, emitter-soak, server-liveness]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
# Same estimateTokens scale (chars/4 over the realized diff), never a harness token count.
actuals:
  tokens: 2293
  tasks: 2
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Per-generation broadcast channel: close + swap under the guarding mutex turns a capacity-1 token wake into a broadcast without sync.Cond; serialized by the single drain caller, so never double-closed"
    - "Snapshot-under-the-same-lock discipline: waiter reads written AND wake in one critical section, then parks on the snapshot — either sees both pre-state (gets closed-channel wake) or both post-state (returns/parks fresh); no lost wakeup, no spin"

key-files:
  created: []
  modified:
    - internal/acp/emitter.go
    - internal/acp/emitter_test.go

key-decisions:
  - "CR-01 fixed as the verifier named: per-generation broadcast (close + fresh channel swapped under mu in writeOut) over sync.Cond — keeps the wake inside the existing mu discipline, the drain stays writeOut's only caller so closes are serialized and never double-close, and Pitfall 3 (no mu across lane send / sink.Write) holds untouched"
  - "Adjacency predicate (unresolved probe) decided and pinned: two Barrier waiters adjacent to the same written frame stay SEPARATE — each re-checks written independently; no waiter is absorbed by another's wake (the both-waiters subtest fails pre-fix precisely by stranding one waiter)"
  - "Empty-queue and ordering contracts unchanged by construction: only the wake mechanism changed — lanes, drain, single-writer rule untouched; TestTurnEmitterEmptyTurn byte-identical and green; full package + soak + cross-package e2e green under -race"

patterns-established:
  - "Broadcast-wake pattern: close+swap the wake channel per generation under mu; waiters snapshot under the same lock and park on the snapshot"
  - "Lost-wakeup regression shape: waiters on deliberately never-cancelled contexts (context.Background()) so no ctx.Done escape can mask a broken wake; bounded select-timeout join fails with a clear message instead of hanging the suite"

requirements-completed: [ACP-03]

# Coverage metadata (#1602)
coverage:
  - "TestTurnEmitterBarrierConcurrentWaiters/same_final_frame_satisfies_both_waiters → CR-01 broadcast wake (writeOut close+swap, Barrier generation snapshot)"

deviations:
  - "[Rule 1/3 - lint] Task 1's new test tripped 11 golangci-lint findings (intrange, noinlineerr, paralleltest/tparallel, wsl_v5); fixed in a separate style commit (20333dd) before GREEN so the fix commit stays purely production code"

---

# Phase 16 Plan 07: Gap Closure 16-07 — Barrier broadcast wake (CR-01) Summary

**One-liner:** TurnEmitter.Barrier's capacity-1 token wake replaced with a per-generation broadcast (close + fresh-channel swap under mu), so two concurrent session/prompt turns on one connection both see their turn-end Barrier return — pinned by a no-ctx-escape regression that fails on the old code.

## What Was Built

**Task 1 — RED (`dc02474`):** `TestTurnEmitterBarrierConcurrentWaiters` in `internal/acp/emitter_test.go`. Subtest "same final frame satisfies both waiters": sink blocks its first Write, two foreground frames enqueued, two `Barrier(context.Background())` goroutines started after both enqueues (both target=2), sink released — on the pre-fix code exactly one waiter strands at terminal state and the subtest fails with "lost wakeup (CR-01)" after its bounded 2s join. Subtest "a lone waiter still wakes per frame" passed pre-fix, pinning that the fix must not over-correct into a lost single-waiter wake. Waiter contexts are never-cancelled — no `context.WithCancel`/`WithTimeout` anywhere on a Barrier argument (grep-verified, count 0).

**Task 2 — GREEN (`4fddf9f`):** `internal/acp/emitter.go` wake mechanism rewritten as the verifier named:
- `writeOut`: `written++` and `close(t.wake)` + `t.wake = make(chan struct{})` share ONE critical section (the drain is writeOut's only caller → closes serialized, never double-close; no mu held across sink.Write or lane sends — Pitfall 3 intact).
- `Barrier`: each iteration checks `written >= target` AND snapshots `wake := t.wake` under the same lock, then selects on the snapshot / caller ctx / emitter root ctx. A pre-close snapshot receives the closed channel and re-checks; a post-swap snapshot parks on the fresh generation. No spin, no lost wakeup, waiters never merge.
- `wake` initialized unbuffered; doc comments updated to describe the broadcast (single-token handoff description removed).

`20333dd` is a test-only lint cleanup (11 findings in the new test), kept separate from the GREEN gate commit.

## Verification

- `go test -race ./internal/acp/ -run TestTurnEmitterBarrierConcurrentWaiters -count=1` — RED pre-fix (both-waiters subtest deadline, lone-waiter pass); GREEN post-fix; `-count=5 -race` shake clean
- `go test -race -count=1 ./internal/acp/` — full package ok (Priority, Stall, ProducerCtxAbort, Barrier, EmptyTurn, frame-surface, ConcurrentWaiters)
- `go test -race -count=1 ./internal/runtime/ -run TestTurnEmitterEndToEnd` — cross-package single-order e2e ok
- `ASSGUARD_EMITTER_SOAK=1 go test ./internal/acp/ -run TestTurnEmitterSoak -count=1 -timeout 10m` — full 153s chaos run ok: no-drop, FIFO, no-leak, stall-fired, clean-close all hold with the broadcast wake
- `go vet ./internal/acp/` clean; `golangci-lint run internal/acp/...` — 0 issues
- Grep proof of mechanism: `close(t.wake)` ×1 in emitter.go (the writeOut broadcast site)
- Empty-queue truth preserved: TestTurnEmitterEmptyTurn byte-identical (untouched) and green — zero-activity turns still emit zero frames and return immediately

## Success Criteria

16-VERIFICATION gaps 1 and 2 closed: the second concurrent turn's session/prompt response can no longer hang — a wake from one written frame reaches EVERY parked Barrier waiter, and updates-before-response holds for every concurrent waiter. Threat T-16-07-01 (high, mitigate) mitigated and pinned by the regression; T-16-07-02 (waiters block on channel receive, never spin) holds by construction and -race.

## Deviations from Plan

**1. [Rule 1/3 - lint] New regression test tripped 11 golangci-lint findings**
- **Found during:** Task 2 verification
- **Issue:** intrange (2), noinlineerr (4), paralleltest/tparallel (3), wsl_v5 (2) in `TestTurnEmitterBarrierConcurrentWaiters`
- **Fix:** `for range waiters`, plain-assignment error handling, `t.Parallel()` in subtests, whitespace above channel sends — semantics unchanged, regression re-run green
- **Files modified:** internal/acp/emitter_test.go
- **Commit:** 20333dd

No other deviations — plan executed as written. No stubs, no skipped tests, no unrun verifies; nothing to add to the broken-windows ledger.

## TDD Gate Compliance

RED gate (`test(16-07)` dc02474, failing both-waiters subtest on pre-fix code) → `style(16-07)` 20333dd (test-only lint) → GREEN gate (`fix(16-07)` 4fddf9f). Plan type is `execute` with per-task tdd="true"; gate sequence test-then-fix intact.

## Self-Check: PASSED

- internal/acp/emitter.go modified (broadcast wake) — FOUND
- internal/acp/emitter_test.go modified (regression) — FOUND
- Commits dc02474, 20333dd, 4fddf9f — FOUND in git log
