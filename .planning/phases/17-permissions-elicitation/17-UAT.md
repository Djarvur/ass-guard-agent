---
status: testing
phase: 17-permissions-elicitation
source: [17-VERIFICATION.md]
started: 2026-09-01T05:05:00Z
updated: 2026-09-02T20:30:00Z
---

## Current Test

number: 1
name: Live-Zed permission dialog round-trip incl. persistence — RE-RUN after G-17-1 fix (WINDOWS #15)
expected: |
  With the canonical nested outcome decode landed (17-06, commits 1538673/f372db4), an allow_always answer executes the call and persists the rule (second identical call runs with NO dialog); reject_always persists the deny and re-denies with no dialog; the original "malformed permission outcome" symptom on every answer is GONE. Four-option rendering + clean cancel already operator-confirmed (2026-09-02).
awaiting: user response

## Tests

### 1. Live-Zed four-option permission dialog + always-persistence (WINDOWS #15)
expected: In a scratch project set permissions.mode: gated via Zed settings/configOptions, prompt a mutating tool call. Zed's native permission dialog renders with FOUR options (Allow once / Always allow / Reject once / Always reject — A1 caveat: confirm the four-option set, not just allow/reject); allow_always persists (second identical call runs with no dialog); reject_always denies with no dialog; cancel closes cleanly.
result: [pending]
re-run: true
note: "First run (2026-09-02): 4-option rendering + cancel CONFIRMED (operator screenshots); every answer failed on the flat-vs-nested outcome decode bug (issue, blocker). Gap G-17-1 fixed by 17-06 (canonical nested decode; simulator corrected; mise ci green) — re-running the persistence legs against the fixed binary."

### 2. Live-Zed native elicitation form + answer lands as tool result (WINDOWS #15 steps 4-5)
expected: Prompt an AskUserQuestion / trigger a learning-store or engine ask in the same gated session. The ask renders as Zed's native structured form (elicitation/create); answering lands the answer as the tool result; on a pre-elicitation client the ask degrades to the plain-text path.
result: [pending]
re-run: true
note: "First run (2026-09-02): blocked by G-17-1 — the AskUserQuestion call was itself permission-gated, its dialog answer hit the decode bug, and the model degraded to the plain-text question (that degrade leg PASSED coherently). G-17-1 fixed — the native form + answer-lands-as-tool-result legs are now testable."

### 3. Human review of the five concurrency fixes (17-REVIEW-FIX.md)
expected: An experienced reviewer eyeballs each fix diff for concurrency-semantics correctness beyond what the pins assert (no lost wakeups, no lock-order inversions, no scheduling-window leaks): CR-01 (a9d8ea4 batch gate-suspension collection + projector hasResult filter), CR-02 (a13054e SetResumeSerial turn-mutex serialization of async resumes), CR-04 (90f22ed SetTurnOriginAutomation bracket), CR-05 (f154e7d AskBroker.ArmSettle for permission suspensions), atomic DrainAll (a710b75). All six pins are RED-proven and green under -race; concurrency invariants can hold in tests and still hide races.
result: pass
note: "Orchestrator pre-reviewed all five diffs and surfaced two findings; operator reviewed them and ruled PASS (non-blocking). CR-01/CR-02/CR-05/DrainAll reviewed sound. Findings preserved as Deferred Follow-Ups below."

### 4. Judgment-tier prohibition review (4 items, NON-AUTHORITATIVE verdicts)
expected: Reviewer confirms the structural evidence is acceptable: 17-01 privacy — internal/perm has ZERO net/* deps (go list -deps); 17-02 safety — persist-then-execute, write failure downgrades to once-only with loud log (internal/session/ask.go:897-955); 17-02 values — neutral symmetric four-option labels (internal/acp/types.go:311-314); 17-04 privacy — BuildElicitationForm's only input is the AskQuestion slice (form discloses nothing the plain-text ask would not).
result: pass
note: "All four mechanical checks re-run by orchestrator live: go list -deps ./internal/perm | grep -c '^net' = 0; ask.go:929-956 persist-then-execute + loud-log downgrade both directions; types.go:311-314 canonical enum confirmed rendered in live dialog (Test 1 screenshot); BuildElicitationForm(qs []session.AskQuestion, ...) content sourced solely from the question slice (ask_surface.go:292). Operator confirmed."

## Summary

total: 4
passed: 2
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps

- gap_id: G-17-1
  truth: "A gated permission-dialog answer (allow/reject x once/always) executes or denies the call and persists the always-rule to permissions.yaml"
  status: resolved
  resolved_by: 17-06-PLAN.md
  resolved_at: 2026-09-02
  reason: "Live Zed 1.18.0 answers session/request_permission with the CANONICAL nested outcome object {\"result\":{\"outcome\":{\"outcome\":\"selected\",\"optionId\":\"…\"}}} (agentclientprotocol.com/protocol/v1/tool-calls), but PermissionOutcomeFrame (internal/acp/types.go:340) declared outcome as a flat string — json.Unmarshal failed at internal/acpserve/ask_surface.go:155-160 and every answer took the fail-safe decline. The E2E simulator answered the same flat shape (internal/acpserve/permissions_e2e_test.go:380 permAnswerSelected), codifying the bug: battery green against a nonconformant client, dead against the real one. Verified from operator transcript/audit in ~/tmp/perm-uat/.ass-guard/ (permissions.yaml empty after all legs)."
  severity: blocker
  test: 1
  artifacts:
    - internal/acp/types.go:335-349
    - internal/acpserve/ask_surface.go:96-160
    - internal/acpserve/permissions_e2e_test.go:377-382
    - https://agentclientprotocol.com/protocol/v1/tool-calls
    - ~/tmp/perm-uat/.ass-guard/transcript_15ceb322-bfe2-439f-814f-a13f1bde76f7.jsonl
  missing: []
  resolution: "17-06 executed (commits 1538673 RED pins, f372db4 GREEN canonical nested decode + simulator corrected, d445ab3 docs; WR-01 pin 2f56969; re-review fixed b4e7c81; re-verification d191fc2 5/5 machine must-haves). Live legs re-pending as Tests 1-2."

## Deferred Follow-Ups

```yaml
- test: 3
  idea: "CR-04 (90f22ed): SetTurnOriginAutomation(true) is set BEFORE mu.TryLock and stays true while the automation goroutine queues behind / runs a turn — any foreground turn overlapping it gets its gated ask-class calls D-07-declined instead of a dialog (fail-safe direction, UX divergence). The in-code comment claims the turn mutex is already held, which does not match the code position. Proper fix: per-turn origin (turn-scoped), not a session-global flag. Operator ruled non-blocking for Phase 17."
  deferred_at: 2026-09-02
- test: 3
  idea: "hasSubstitution rewrite in a710b75 (internal/perm/rules.go) dropped double-quote tracking: case '\\'' skips to the next single quote even when both are literal INSIDE double quotes, so git \"log 'x $(curl evil) y'\" under-detects live $()/backtick substitution (verified in live shell) and an allow rule (Bash(git *)) can now match a substitution-bearing command — WR-02's one-sided allow guard violated. Old pre-a710b75 code traced double quotes and caught this. Security-adjacent; deserves a fix ticket soon. Operator ruled non-blocking for Phase 17."
  deferred_at: 2026-09-02
```
