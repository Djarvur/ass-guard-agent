---
status: testing
phase: 17-permissions-elicitation
source: [17-VERIFICATION.md]
started: 2026-09-01T05:05:00Z
updated: 2026-09-01T05:05:00Z
---

## Current Test

number: 1
name: Live-Zed four-option permission dialog + always-persistence (WINDOWS #15)
expected: |
  In a scratch project set permissions.mode: gated via Zed settings/configOptions, prompt a mutating tool call. Zed's native permission dialog renders with FOUR options (Allow once / Always allow / Reject once / Always reject — A1 caveat: confirm the four-option set, not just allow/reject); allow_always persists (second identical call runs with no dialog); reject_always denies with no dialog; cancel closes cleanly.
awaiting: user response

## Tests

### 1. Live-Zed four-option permission dialog + always-persistence (WINDOWS #15)
expected: In a scratch project set permissions.mode: gated via Zed settings/configOptions, prompt a mutating tool call. Zed's native permission dialog renders with FOUR options (Allow once / Always allow / Reject once / Always reject — A1 caveat: confirm the four-option set, not just allow/reject); allow_always persists (second identical call runs with no dialog); reject_always denies with no dialog; cancel closes cleanly.
result: [pending]

### 2. Live-Zed native elicitation form + answer lands as tool result (WINDOWS #15 steps 4-5)
expected: Prompt an AskUserQuestion / trigger a learning-store or engine ask in the same gated session. The ask renders as Zed's native structured form (elicitation/create); answering lands the answer as the tool result; on a pre-elicitation client the ask degrades to the plain-text path.
result: [pending]

### 3. Human review of the five concurrency fixes (17-REVIEW-FIX.md)
expected: An experienced reviewer eyeballs each fix diff for concurrency-semantics correctness beyond what the pins assert (no lost wakeups, no lock-order inversions, no scheduling-window leaks): CR-01 (a9d8ea4 batch gate-suspension collection + projector hasResult filter), CR-02 (a13054e SetResumeSerial turn-mutex serialization of async resumes), CR-04 (90f22ed SetTurnOriginAutomation bracket), CR-05 (f154e7d AskBroker.ArmSettle for permission suspensions), atomic DrainAll (a710b75). All six pins are RED-proven and green under -race; concurrency invariants can hold in tests and still hide races.
result: [pending]

### 4. Judgment-tier prohibition review (4 items, NON-AUTHORITATIVE verdicts)
expected: Reviewer confirms the structural evidence is acceptable: 17-01 privacy — internal/perm has ZERO net/* deps (go list -deps); 17-02 safety — persist-then-execute, write failure downgrades to once-only with loud log (internal/session/ask.go:897-955); 17-02 values — neutral symmetric four-option labels (internal/acp/types.go:311-314); 17-04 privacy — BuildElicitationForm's only input is the AskQuestion slice (form discloses nothing the plain-text ask would not).
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps

[none]
