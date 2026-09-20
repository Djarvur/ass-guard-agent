---
phase: 23-seed-gaps-close-out
plan: 04
subsystem: runtime
tags: [checkpoint, restore-guard, gc, config-options, seedg, tdd]
# Dependency graph
requires:
  - phase: 18-session-family
    provides: the checkpoint store + tombstone-grace GC precedent (D-09 family)
  - phase: 23-02
    provides: the parked-ask queue state the restore guard's refusal matrix reads
  - phase: 23-03
    provides: the session-scoped checkpoint plumbing this plan lifts to the Runner
provides:
  - Runner-owned checkpoint store (one per workspace) — /undo, the restore guard, and the GC sweep never depend on a session-scoped handle; nil-store sessions report /undo unavailable loudly (Pitfall 9, AUD-03)
  - The restore guard's full refusal matrix — active client turn / running chain / parked chain (chainCount>0, no mutex) each refuse with a refusal naming the active state; only idle restores (SEEDG-02)
  - The session-start age+count GC sweep (D-08) — exclude-append → sweep → session-ready ordering, failures loud but never session-fatal
  - checkpoint.expiry_days (default 7) + checkpoint.max_per_session (default 50) as advertised, accept-validated, persisted configOptions selects under the checkpoint: layer key, read back by the sweep via the generic layer map (D-10)
affects: [23-05 (the /undo class-B command rides the Runner store), phase-24 ECOS-04 mode matrix]
actuals:
  tokens: 14000    # chars/4 over the realized diff (1417+13 diff lines ≈ 56K chars across runtime/acpserve)
  tasks: 3
  commits: 5      # 2 RED + 2 GREEN + 1 integration battery
tech-stack:
  added: []
  patterns:
    - "runner-owned store over session-scoped handles (the subagentProfile/PTY-manager precedent: lifecycle owners live on the Runner, sessions borrow)"
    - "refusal-matrix batteries enumerate the full active-state × store-presence cross product before any GREEN"
key-files:
  created:
    - internal/runtime/restore_guard_test.go
  modified:
    - internal/runtime/runtime.go
    - internal/acpserve/config_surface.go
key-decisions:
  - "The store is Runner-owned (one per workspace): /undo availability, restore guarding, and GC share one handle, so a degraded nil-store session degrades loudly at /undo instead of silently at restore time"
  - "Parked chains refuse restore via chainCount>0 with no mutex held — the 23-02 parked-ask state closes the guard's blind spot for engine chains that are neither running nor idle"
  - "GC bounds ride the same configOptions surface as every other operator lever (enumerated selects, layer-persisted, generic-map read-back) — no bespoke checkpoint config path"
requirements-completed: [SEEDG-02]
coverage:
  - description: "Runner-owned checkpoint store + nil-store loud degrade"
    verification:
      kind: tests
      ref: "internal/runtime/restore_guard_test.go#nil-store matrix axis"
      status: pass
    human_judgment: false
  - description: "Restore refusal matrix (turn / running chain / parked chain / idle × store presence)"
    verification:
      kind: tests
      ref: "internal/runtime/restore_guard_test.go#refusal matrix"
      status: pass
    human_judgment: false
  - description: "Session-start GC sweep ordering + config-driven bounds reaching the sweep"
    verification:
      kind: tests
      ref: "internal/runtime/restore_guard_test.go#session-start ordering + config-to-GC wiring"
      status: pass
    human_judgment: false
  - description: "checkpoint.expiry_days / checkpoint.max_per_session configOptions (advertise, validate, persist, read-back)"
    verification:
      kind: tests
      ref: "internal/acpserve/config_test.go + internal/runtime/restore_guard_test.go#config-to-GC"
      status: pass
    human_judgment: false
duration: interrupted-then-closed  # executed 2026-09-07/08 (commits 7e229bd..10088c5), closed out 2026-09-10 by the manager resume gate
completed: 2026-09-10
---

# Phase 23 Plan 04: Checkpoint Store, Restore Guard, GC ConfigOptions Summary

Runner-owned checkpoint store with the full restore-refusal matrix (turn / running chain / parked chain), the session-start age+count GC sweep, and both checkpoint GC bounds as persisted configOptions — SEEDG-02 closed.

## Manual Close-Out Note

The executor's run was interrupted after the final production commit (10088c5) and before SUMMARY: the manager's safe-resume gate detected five production commits with no SUMMARY and recovered via close-out (commits verified, all plan verification gates re-run green) rather than re-dispatch — no work was duplicated.

## Accomplishments

- The Runner holds the checkpoint store once per workspace; sessions borrow it, and a nil-store session reports /undo unavailable loudly (the matrix's nil axis pins it).
- Restore refuses under ANY active state — client turn, running engine chain, and the parked-chain blind spot (chainCount>0 without the mutex) — each refusal names the active state; only idle restores.
- The session-start sweep excludes the live session's appends before GC, orders exclude→sweep→ready, and degrades loudly without failing the session.
- checkpoint.expiry_days (7) and checkpoint.max_per_session (50) advertise as enumerated selects, persist under the checkpoint: layer key, and the sweep reads them through the generic layer map — pinned end-to-end by the config-to-GC test (persisted perSession=10 honored by the sweep).

## Commits (ledger, oldest first)

- 7e229bd test(23-04): RED guard battery — refusal matrix, session-start sweep, nil-store degradation, store-held-once
- e79eb0f feat(23-04): Runner-owned checkpoint store, restore guard, session-start GC sweep
- 9c5ab7e test(23-04): RED battery for the checkpoint GC config options
- ec4bedc feat(23-04): checkpoint GC configOptions — expiry_days + max_per_session
- 10088c5 test(23-04): integration battery — nil-store matrix axis, session-start ordering, config-to-GC wiring

## Deviations from Plan

None — commits match the three tasks' RED/GREEN/integration sequence as written.

## Verification (re-run at close-out, 2026-09-10)

- `go test -race -count=1 -skip 'TestRescanConcurrency' ./internal/session/ ./internal/runtime/ ./internal/checkpoint/` — green (scoped past the documented pre-existing deferred race, deferred-items.md).
- `./internal/acpserve/` green except TestPermissionsE2E — the documented pre-existing cross-workstream failure (Phase 23 commit 40b2bbc, operator-bisected, STATE.md) — every checkpoint/config test passes.
- The 16-05 config-surface battery green (no pending-option regressions).

## Self-Check: PASSED
