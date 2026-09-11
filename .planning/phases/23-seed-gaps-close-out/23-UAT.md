---
status: complete
phase: 23-seed-gaps-close-out
source: [23-VERIFICATION.md]
started: 2026-09-10T05:30:00Z
updated: 2026-09-11T19:25:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Live-Zed steering + /undo operator UAT (WINDOWS #23)
expected: In a live Zed session: mid-turn steering note + boundary delivery (marker in transcript, turn not cancelled); idle /undo instant restore; mid-turn /undo cancel-then-restore with the fail-closed ordering. The automated witnesses (TestSteeringDeliveryEndToEnd, TestSteeringAntiZombie, TestClassBUndoIdle, TestUndoAutoCancel, TestUndoFailClosed) are green — this leg confirms the operator-visible editor surface.
result: issue
reported: "steering - работает (live note, turn not cancelled); undo на работающем прогоне - НЕ работает: Zed shows 'An Error Happened: /undo is not a recognized command in ass-guard … Available commands for ass-guard: none'. Screenshots 2026-09-11 22-17/22-18."
severity: major
evidence: |
  Binary is FRESH (~/go/bin/ass-guard built 2026-09-11 22:11, test ran 22:17 — not a
  G-19-2 stale-binary repeat). The error text is NOT in the ass-guard binary (strings
  grep "not a recognized command" = 0 hits) — it is Zed's CLIENT-SIDE rejection: Zed
  blocks any /command absent from the agent's available_commands_update advertisement,
  and the advertisement was empty/never sent ("Available commands: none" — not even the
  phase-20 builtins). Tools and turns work in the same session; only the command
  advertisement surface is dead. Binary contains the reserved-builtin strings and the
  "ass-guard: available_commands_update enqueue failed (continuing)" silent-degrade path.

### 2. Two-live-Zed-session cross-session restore check (WINDOWS #24)
expected: Two Zed sessions on one workspace: session A's turn live, /undo from idle session B refuses with a refusal naming A (workspace busy); after A's turn ends, B's retry restores. The offline battery TestUndoCrossSessionRefusal is the automated witness — this leg confirms the operator-visible surface.
result: issue
reported: "то же — the same Zed client-side rejection (empty available_commands advertisement); cross-session surface unreachable until G-23-1 is fixed"
severity: major

## Summary

total: 2
passed: 0
issues: 2
pending: 0
skipped: 0
blocked: 0

## Gaps

- gap_id: G-23-1
  truth: "In a live editor session, /undo (mid-turn and idle) reaches the agent's class-B command resolver — Zed offers it and executes cancel-then-restore / instant restore"
  status: failed
  reason: "Operator UAT 2026-09-11 22:17: Zed client-side rejects /undo — 'Available commands for ass-guard: none'. The agent's available_commands_update advertisement is empty/absent in the shipped fresh binary, so Zed never forwards any slash command. Steering leg PASSED (live note, no cancellation) — the failure is isolated to the command advertisement surface. Test 2 (cross-session) confirmed blocked by the same root cause (operator: «то же»)."
  severity: major
  test: 1
  related_tests: [2]
  artifacts: []
  missing: []
