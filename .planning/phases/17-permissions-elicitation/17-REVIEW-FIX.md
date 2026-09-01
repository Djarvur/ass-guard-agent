---
phase: 17-permissions-elicitation
fixed_at: 2026-09-01T08:00:00Z
review_path: .planning/phases/17-permissions-elicitation/17-REVIEW.md
iteration: 1
findings_in_scope: 9
fixed: 9
skipped: 0
info_also_fixed: 3
status: all_fixed
---

# Phase 17: Code Review Fix Report

**Fixed at:** 2026-09-01T08:00:00Z
**Source review:** .planning/phases/17-permissions-elicitation/17-REVIEW.md
**Iteration:** 1
**Branch:** gsd/v1.2-claude-code-parity (main checkout — `workflow.use_worktrees=false`)

**Summary:**
- Findings in scope (Critical + Warning): 9
- Fixed: 9
- Skipped: 0
- Info findings also fixed (trivially-safe, encountered incidentally): 3

Every fix is committed atomically (`fix(17): …`) with a pinning test in the same
commit. Every concurrency/logic pin was RED-proven: the fix was temporarily
disabled and the test observed failing with the finding's exact symptom, then
re-enabled and observed passing. No false positives were found.

## Fixed Issues

### CR-01: Multi-call batch drops all but the last permission suspension

**Files modified:** `internal/session/session.go`, `internal/session/projector.go`, `internal/session/gate_test.go`, `internal/session/projector_test.go`
**Commit:** `a9d8ea4`
**Status:** fixed: requires human verification (logic + projection semantics)

`runTurn` now collects every gated suspension into `gateSuspensions []pendingPermission`
(was a single overwritten pointer) and suspends ALL of them in batch order; the
ask queue serializes their firing (D-11 one-outstanding). Because a partial
answer leaves a sibling call resultless mid-resume, the projector now drops
still-unanswered tool_use from the projected batch (`accumulateMidTurn`
hasResult filter) — an unpaired tool_use is what the Anthropic-protocol
provider rejects. Pins: `TestGatePermissionSuspend_MultiCallBatch` (two gated
calls in one response → two ask_suspended records, two sequential surface
firings, mid-suspension projection carries exactly [call1], both results land)
and `TestPairingInvariant_ProjectorWindow` (pin UPDATED: the old pin asserted
that a dangling unanswered batch IS projected — that state is only reachable
across a suspension resume, where projecting it is precisely the CR-01 bug).

### CR-02: Resume paths re-enter runTurn without the per-session turn mutex

**Files modified:** `internal/session/session.go`, `internal/session/ask.go`, `internal/session/gate.go`, `internal/runtime/runtime.go`, `internal/session/gate_test.go`, `internal/runtime/ask_wiring_test.go`
**Commit:** `a13054e`
**Status:** fixed: requires human verification (concurrency semantics)

`Session.SetResumeSerial(func(func()))` is injected at composition
(`runtime.sessionFor` → `r.sessionTurnMu(sessionID)` guarded execution — the
session cannot import the runtime, so the func-seam injection follows the
OnSuspended/ParkMu precedent). Every ASYNC resume driver is wrapped: the gate
queue's resolve callback (permission family), the elicitation queue's resolve
(question family accept/non-answer), and the D-01 timer (`SetOnTimeout` body).
The sync reply path (`routeAskReply` → `ResolveAsk` inside `Run`) deliberately
bypasses the wrapper — Run already holds the mutex and the wrapper is not
reentrant. Pins: `TestAskWiring_ResumeHoldsTurnMutex` (dialog answered while
turnMu held → NO result until release), `TestAskWiring_TimerResumeHoldsTurnMutex`
(D-01 leg), `TestGateResumeRunsUnderResumeSerial` (serializer entered exactly
once, zero overlap). All three RED-proven.

### CR-03: session/cancel drain runs full model resumes inline in the reader goroutine

**Files modified:** `internal/session/askqueue.go`, `internal/session/askqueue_test.go`
**Commit:** `e2afc1a`
**Status:** fixed

`DrainTurn` invokes each drained entry's resolve on its own goroutine
(`go qa.resolve(entry, AskOutcome{Cancelled: true})`); the cancel-func
invocation stays outside the mutex as before. The drained resolves inherit
CR-02's turn-mutex serialization, so ordering with live turns is preserved.
Pin: `TestAskQueueDrainResolvesAsync` — the drain returns while a drained
resolve is still blocked mid-model-loop; RED-proven (synchronous resolve
hangs the drain).

Follow-up (`a710b75`): `DrainAll` now claims the whole drain state (all queued
entries + the open cancel) under ONE mutex pass instead of looping `DrainTurn`
per turn id. The per-id loop left a scheduling window in which the pump, woken
by the first cancel, promoted and fired a not-yet-drained queued entry — the
zombie-ask ban violated by luck (`TestAskQueueDrainAll` caught it under the
re-worked timing).

### CR-04: D-07 automation-decline unwired — SetTurnOriginAutomation has zero production callers

**Files modified:** `internal/runtime/cron_wiring.go`, `internal/runtime/cron_wiring_test.go`
**Commit:** `90f22ed`
**Status:** fixed: requires human verification (lifecycle bracket semantics)

`runAutomationTurn` brackets the firing with
`sess.SetTurnOriginAutomation(true)` / `defer sess.SetTurnOriginAutomation(false)`,
mirroring the provenance set/reset bracket. Automation turns now DECLINE
ask-class calls fail-safe in both modes — never a dialog nobody would answer,
and the documented decline note lands as the audit trail. Pin:
`TestCronWiring_AutomationTurnDeclinesGatedAsk` — a gated automation firing
against a mutating tool call produces the "Permission declined" tool result,
zero surface firings, zero ask_suspended records; RED-proven (without the
bracket the turn suspends and fires the surface).

### CR-05: Permission suspensions invisible to the engine ask-wait

**Files modified:** `internal/session/ask.go`, `internal/session/gate.go`, `internal/session/gate_test.go`, `internal/runtime/ask_wiring_test.go`
**Commit:** `f154e7d`
**Status:** fixed: requires human verification (engine park/resume semantics)

`AskBroker.ArmSettle(ch)` (mutex-guarded, Surface's exact seam semantics)
publishes the permission suspension's settle channel;
`suspendForPermission` arms it immediately after creating `p.settle`. The
engine's `waitAskSettled` now blocks on a LIVE channel during a gated dialog —
no silent exit on nil, no hot-spin on a stale closed channel. Pins:
`TestGatePermissionSuspensionArmsSettle` (seam live while the dialog is open;
closes only after the resumed turn returns) and
`TestAskWiring_ChainSurvivesPermissionDialogResume` (the gold pin: engine-on
chain parks while the dialog is open — `WaitChainIdle` does NOT fire — then
after the answer resumes, decides continue, and injects the apply stage);
RED-proven (without ArmSettle the chain exits at the dialog).

### WR-01: Corrupt permissions.yaml disables all rules (fail-open)

**Files modified:** `internal/perm/store.go`, `internal/perm/store_test.go`, `internal/runtime/runtime.go`
**Commit:** `4dcd299`
**Status:** fixed

Adopted the review's option (b): typed `perm.ErrCorrupt` (document-level parse
failures, wrapping the YAML error from `load`) and `perm.OpenRepaired` — the
unreadable file is quarantined byte-intact to `permissions.yaml.corrupt`
(stale quarantine replaced), the floor 0600 file is recreated so dialog writes
still work, and the degradation is LOUD (structured error: "DENY/ALLOW RULES
ARE NOT ENFORCED until the file is restored"). `runtime.sessionFor` opens
through `OpenRepaired`; environmental failures (mkdir/stat/create) and
line-level malformations pass through unchanged. Pins:
`TestOpenCorruptDocumentTypedError`, `TestOpenRepairedQuarantinesCorruptFile`
(bytes-intact quarantine + 0600 recreate + working store),
`TestOpenRepairedHealthyFileUnchanged` (scoping).

### WR-02: Compound splitting ignores quoting and command substitution

**Files modified:** `internal/perm/rules.go`, `internal/perm/rules_test.go`
**Commit:** `bbbf71a`
**Status:** fixed

`SplitCompound` is now quote-aware: separators inside single or double quotes
split nothing (`git commit -m "fix a && b"` is one subcommand); unterminated
quotes fail safe to a single segment. `evaluateSingle` refuses to let an ALLOW
match a substitution-bearing command (`hasSubstitution`: `$(…)`/backtick, LIVE
inside double quotes per shell semantics, literal inside single quotes, `\`
escapes honored) — deny and ask still match the raw subject, so deny stays
strong and the compound discipline cannot be defeated by payload hidden in a
substitution. Pins: 8 new TestRules rows (substitution-never-allows,
backtick, double-quoted-live, single-quoted-literal, escaped-literal,
plain-prefix-still-allows, deny-still-matches, compound-fails-safe) +
`TestSplitCompoundQuotes` (5 rows). RED-proven.

### WR-03: Mid-pattern star in a tool selector is a silently dead rule

**Files modified:** `internal/perm/rules.go`, `internal/perm/rules_test.go`
**Commit:** `a4c6760`
**Status:** fixed

`parseList` warn-and-skips any tool selector whose star is not trailing, in
EVERY list (the inert-allow-glob precedent) with reason "mid-pattern star in
tool selector never matches (rule skipped)". Trailing-star globs, bare stars,
and literal names are untouched. Pin: TestRulesWarnings mid-star rows across
deny + allow; RED-proven.

### WR-04: Store load never tightens loose permissions

**Files modified:** `internal/perm/store.go`, `internal/perm/store_test.go`
**Commit:** `b107220`
**Status:** fixed

`Open` now tightens the store directory to 0750 (after `MkdirAll`) and the
existing permissions.yaml to 0600 (after a successful load): stat → compare →
chmod, loud structured warning on every correction, non-fatal on chmod error.
Pin: `TestOpenTightensLoosePerms` (0644 file + 0777 dir → 0600/0750, rules
intact); RED-proven.

## Info findings fixed (out of scope, trivially safe, encountered incidentally)

### IN-01: Import-keep hack in handleSessionSetMode — `012961a`
Dropped `_ = redact.ScrubError(nil)` and the now-unused redact import from
`internal/acp/handlers.go` (server.go's redact users keep the package live).

### IN-02: Fixed sleep stands in for the pump — `19815cd`
`internal/session/elicitation_reply_test.go` fallback subtest now waits on
`gateWaitFor(t, func() bool { return fired() == 1 })` instead of a 50ms sleep.

### IN-03: AppendAskSuspended error silently discarded — `f91e5c5`
The suspension site logs the marker-write failure (slog.Warn with
turn/call/tool) while still proceeding with the suspension.

## Verification

- Per-fix: targeted `go test` on the touched packages; pins RED-proven by
  temporarily disabling each fix and observing the failure.
- Full gate: `mise ci` (go vet + golangci-lint v2 all-linters + CGO_ENABLED=0
  build + `go test -race -count=1 ./...`) — run in the MAIN checkout (no
  worktree: `workflow.use_worktrees=false`). First run failed lint (37 issues
  introduced by the fixes: cyclop/funlen/gocyclo on the new test batteries,
  goconst on repeated literals, noinlineerr/wsl/lll house rules); all resolved
  (test-table complexity per house nolint style, production hasSubstitution
  restructured, projector pre-scans extracted, DrainAll made atomic) and the
  gate re-run clean.

## Commit index

| Finding | Commit |
|---|---|
| CR-01 | `a9d8ea4` |
| CR-02 | `a13054e` |
| CR-03 | `e2afc1a` (+ atomic DrainAll `a710b75`) |
| CR-04 | `90f22ed` |
| CR-05 | `f154e7d` |
| WR-01 | `4dcd299` |
| WR-02 | `bbbf71a` |
| WR-03 | `a4c6760` |
| WR-04 | `b107220` |
| IN-01 | `012961a` |
| IN-02 | `19815cd` |
| IN-03 | `f91e5c5` |

---

_Fixed: 2026-09-01T08:00:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
