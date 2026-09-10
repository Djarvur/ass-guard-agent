---
status: testing
phase: 23-seed-gaps-close-out
source: [23-VERIFICATION.md]
started: 2026-09-10T05:30:00Z
updated: 2026-09-10T05:30:00Z
---

## Current Test

number: 1
name: Live-Zed steering + /undo operator UAT (WINDOWS #23)
expected: |
  In a live Zed session against a fresh binary: steering sent mid-turn surfaces the live note
  and is delivered at the next model-request boundary as a steering marker (never cancels the
  running turn); /undo on an idle session restores the last checkpoint instantly; /undo
  mid-turn auto-cancels then restores (fail-closed ordering).
awaiting: user response

## Tests

### 1. Live-Zed steering + /undo operator UAT (WINDOWS #23)
expected: In a live Zed session: mid-turn steering note + boundary delivery (marker in transcript, turn not cancelled); idle /undo instant restore; mid-turn /undo cancel-then-restore with the fail-closed ordering. The automated witnesses (TestSteeringDeliveryEndToEnd, TestSteeringAntiZombie, TestClassBUndoIdle, TestUndoAutoCancel, TestUndoFailClosed) are green — this leg confirms the operator-visible editor surface.
result: [pending]

### 2. Two-live-Zed-session cross-session restore check (WINDOWS #24)
expected: Two Zed sessions on one workspace: session A's turn live, /undo from idle session B refuses with a refusal naming A (workspace busy); after A's turn ends, B's retry restores. The offline battery TestUndoCrossSessionRefusal is the automated witness — this leg confirms the operator-visible surface.
result: [pending]

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps
