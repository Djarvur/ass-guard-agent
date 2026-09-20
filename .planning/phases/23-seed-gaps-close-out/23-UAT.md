---
status: complete
phase: 23-seed-gaps-close-out
source: [23-VERIFICATION.md]
started: 2026-09-10T05:30:00Z
updated: 2026-09-20T18:40:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Live-Zed steering + /undo operator UAT (WINDOWS #23)
expected: In a live Zed session: mid-turn steering note + boundary delivery (marker in transcript, turn not cancelled); idle /undo instant restore; mid-turn /undo cancel-then-restore with the fail-closed ordering. The automated witnesses (TestSteeringDeliveryEndToEnd, TestSteeringAntiZombie, TestClassBUndoIdle, TestUndoAutoCancel, TestUndoFailClosed) are green — this leg confirms the operator-visible editor surface.
result: pass
evidence: |
  Automated operator run 2026-09-20 (see 23-UAT-evidence-2026-09-20.md). Steering leg
  driven through live Zed 1.20.2 (XWayland synthetic input): note delivered at a model-
  request boundary, turn not cancelled (6 tool calls completed), reply ended "DONE
  STEERED" with the requested directory count; transcript marker verified on disk.
  Idle /undo: class-B resolution, instant restore, file reverted ("undo complete —
  restored checkpoint …-turn-001"). Mid-turn /undo: 0.1s response, interrupted turn
  stopReason=cancelled, file reverted. G-23-1 wire evidence with the real client:
  available_commands_update (15 commands incl. undo) arrives AFTER the session/new
  response — the fixed ordering; session accepted client-side (immediate
  session/set_config_option).
  Method note: /undo legs executed via a Zed-shaped ACP driver (same binary, workspace,
  wire shape); the Zed-side catalog acceptance is proven on the wire with the real client.
reported: "Operator: pass (2026-09-20)."
severity: none

### 2. Two-live-Zed-session cross-session restore check (WINDOWS #24)
expected: Two Zed sessions on one workspace: session A's turn live, /undo from idle session B refuses with a refusal naming A (workspace busy); after A's turn ends, B's retry restores. The offline battery TestUndoCrossSessionRefusal is the automated witness — this leg confirms the operator-visible surface.
result: pass
evidence: |
  Two sessions (A 64c1127c…, B d2250c19…), one workspace: with A's turn live, B's /undo
  refused in 0.1s naming A ("a client turn is active for session 64c1127c… — the
  worktree and checkpoint store are shared by every session of this process; cancel
  that session's turn or wait for it to end"); B's file untouched. After A ended, B's
  retry restored ("undo complete — restored checkpoint d2250c19…-turn-001") and the
  file reverted. Driver-executed leg (see evidence file + driver2-frames.jsonl).
reported: "Operator: pass (2026-09-20)."
severity: none

## Summary

total: 2
passed: 2
issues: 0
pending: 0
skipped: 0
blocked: 0

## Observations (non-blocking)

- stdout discipline violation: at startup the agent writes a git-checkpoint init log
  line to stdout (reserved for ACP frames) — `[refs/checkpoints/last (root-commit)
  e5f44b9] ass-guard checkpoint store init`. Route to stderr. (strace frames 0–1.)
- Zed 1.20.2 renders no slash-command menu on "/" even with the catalog accepted;
  command validation happens at send time. Send-time UI forwarding of /undo was not
  exercised through the live editor (dock/panel automation blocked by workspace-scoped
  dock state); covered on the wire instead.
- Zed does not re-spawn a dead ACP agent server via panel interaction (Restart banner
  unresponsive to synthetic input); a full Zed restart re-spawns at startup.

## Gaps

- gap_id: G-23-1
  truth: "In a live editor session, /undo (mid-turn and idle) reaches the agent's class-B command resolver — Zed offers it and executes cancel-then-restore / instant restore"
  status: resolved        # was: failed
  resolved_by: 23-07-PLAN
  resolved_at: 2026-09-16
  verified_at: 2026-09-20
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
  verification_note: "2026-09-20: fix verified post-response ordering on the wire with the real Zed client (23-UAT-evidence-2026-09-20.md); operator passed both affected tests."
