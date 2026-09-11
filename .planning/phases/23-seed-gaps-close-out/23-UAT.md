---
status: diagnosed
phase: 23-seed-gaps-close-out
source: [23-VERIFICATION.md]
started: 2026-09-10T05:30:00Z
updated: 2026-09-11T19:45:00Z
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
  root_cause: "AND-gated interop mismatch. Agent-side: internal/acp/handlers.go handleSessionNew (:353-366, introduced 89bcc6f Phase 20-01) emits available_commands_update BEFORE writing the session/new response — deliberately mirroring the 16-01/18-05 session/load ordering. Client-side: Zed drops every session/update for an unregistered session (acp.rs handle_session_notification → 'unknown session' warn → return); on session/new Zed cannot pre-register (learns the sessionId only from the response), while on session/load it pre-registers before awaiting — which is why mirroring the load order onto session/new was invalid. Wire-reproduced against the shipped fresh binary: the full 14-command advertisement (incl. /undo) IS emitted, correct shape, but arrives before the response by construction and is discarded client-side. No later re-fire rescues the session (startup SetCatalog notify fires with zero live sessions; 20-05 rescan needs .claude/ fs changes). Matches open upstream zed-industries/zed#60199 (identical symptom with CodeBuddy)."
  artifacts:
    - path: internal/acp/handlers.go
      issue: "handleSessionNew emits the advertisement pre-response (:353-366) — the agent-side half of the mismatch"
    - path: internal/acp/server.go
      issue: "NotifyAvailableCommands (:538-560) — correct in isolation (foreground lane), only its call timing is wrong"
  missing:
    - "Emit the session/new advertisement AFTER the session/new response frame is written (post-response emission / deferred enqueue ordered behind the response write)"
    - "Leave the session/load pre-response order unchanged (Zed pre-registers there — that order is correct)"
    - "20-05 rescan re-fire already covers later discovery changes"
  debug_session: .planning/debug/zed-empty-commands-advertisement.md
