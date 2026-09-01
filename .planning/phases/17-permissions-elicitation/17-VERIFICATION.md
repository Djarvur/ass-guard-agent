---
phase: 17-permissions-elicitation
verified: 2026-09-01T05:02:34Z
status: human_needed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
unverified-prohibition: "4 judgment-tier prohibitions carry NON-AUTHORITATIVE LLM-judge PASS verdicts with structural evidence — human review recommended (see Human Verification item 4)"
human_verification:
  - test: "Live-Zed leg (WINDOWS ledger #15, seven-step checklist): in a scratch project set permissions.mode: gated via Zed settings/configOptions, prompt a mutating tool call"
    expected: "Zed's native permission dialog renders with FOUR options (Allow once / Always allow / Reject once / Always reject — A1 caveat: confirm the four-option set, not just allow/reject); allow_always persists (second identical call runs with no dialog); reject_always denies with no dialog; cancel closes cleanly"
    why_human: "Zed's native dialog rendering and the always-persistence UX are editor-side behaviors no deterministic in-repo test can observe; the simulator proves the wire, not the pixels"
  - test: "Live-Zed leg (WINDOWS ledger #15, checklist steps 4-5): prompt an AskUserQuestion / trigger a learning-store or engine ask in the same gated session"
    expected: "The ask renders as Zed's native structured form (elicitation/create); answering lands the answer as the tool result; on a pre-elicitation client the ask degrades to the plain-text path"
    why_human: "Native form rendering and answer UX are visual/editor-side; the capable + degraded simulator scenarios prove the wire frames and byte-parity fallback only"
  - test: "Human review of the five concurrency fixes 17-REVIEW-FIX.md marked 'requires human verification': CR-01 (a9d8ea4 batch gate-suspension collection + projector hasResult filter), CR-02 (a13054e SetResumeSerial turn-mutex serialization of async resumes), CR-04 (90f22ed SetTurnOriginAutomation bracket), CR-05 (f154e7d AskBroker.ArmSettle for permission suspensions), and the atomic DrainAll (a710b75)"
    expected: "An experienced reviewer eyeballs each fix diff for concurrency-semantics correctness beyond what the pins assert (no lost wakeups, no lock-order inversions, no scheduling-window leaks)"
    why_human: "Each fix has a RED-proven pin that passes under -race (TestGatePermissionSuspend_MultiCallBatch, TestAskWiring_ResumeHoldsTurnMutex, TestGateResumeRunsUnderResumeSerial, TestCronWiring_AutomationTurnDeclinesGatedAsk, TestGatePermissionSuspensionArmsSettle, TestAskQueueDrainAll — all green in this verification), but concurrency invariants can hold in tests and still hide races; the fixer explicitly flagged these for human review"
  - test: "Unverified-prohibition review (human review recommended — NON-AUTHORITATIVE LLM-judge verdicts): 17-01 privacy (no network path in internal/perm), 17-02 safety (persist-then-execute, write failure downgrades to once-only with loud log), 17-02 values (neutral symmetric four-option labels), 17-04 privacy (form discloses nothing the plain-text ask would not)"
    expected: "Reviewer confirms the structural evidence is acceptable: internal/perm has ZERO net/* deps (go list -deps), the persist-failure downgrade path is at internal/session/ask.go:897-955 (slog.Error + once-only), labels at internal/acp/types.go:311-314 are neutral/symmetric, and BuildElicitationForm's only input is the AskQuestion slice"
    why_human: "Judgment-tier prohibitions cannot be closed by an autonomous verifier per ADR-550 D4; the evidence here is structural (greps/dep-graph) and the values/non-disclosure judgments are inherently human"
---

# Phase 17: Permissions + Elicitation — Verification Report

**Phase Goal:** The agent's asks become clickable editor surfaces instead of plain text — permission asks via session/request_permission with allow/reject × once/always semantics, learning/engine asks via elicitation/create forms — riding the AskBroker suspension pattern so human-timescale waits never hold locks; the ONE gate pipeline (hook verdict → permission ask → execute) is locked and documented here so Phase 21's hooks join rather than bolt on.
**Verified:** 2026-09-01T05:02:34Z
**Status:** human_needed (all machine-verifiable must-haves VERIFIED; 4 operator/review legs recorded per the 15-07/16-06 WINDOWS-ledger precedent)
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

Merged must-haves: the 5 ROADMAP success criteria (the contract) with plan-frontmatter truths as supporting detail. Every truth below was verified with a passing behavioral test (not symbol presence alone).

| # | Truth (ROADMAP SC) | Status | Evidence |
|---|--------------------|--------|----------|
| 1 | With `permissions.mode: gated`, a mutating tool call pops the native permission dialog with allow/reject × once/always options; the chosen option persists across subsequent calls (always = no further ask) | ✓ VERIFIED (deterministic half; live-Zed rendering → Human Verification 1) | Four canonical option kinds at `internal/acp/types.go:311-314`; `session/request_permission` surface under HUMAN-ASK in `internal/acpserve/ask_surface.go`; persist-then-execute at `internal/session/ask.go:897-955` (allow_always persists BEFORE execute; reject_always persists BEFORE the denial result); `TestPermissionsE2E` happy path green under -race: one request_permission frame → allow_always → rule on disk → execution → second identical call with zero further frames. Store restart survival: `TestStoreAllowToolPersistsAndSurvivesRestart`. |
| 2 | While a permission dialog is open the session stays alive and responsive — other prompts queue, cancellation works, turn death mid-ask delivers cancelled as a NORMAL response, nothing holds the turn mutex | ✓ VERIFIED | Pins all green under -race in this verification: `TestGateChokepoint_SecondPromptWhileDialogOpen`, `TestGateTurnDeath`, `TestGateChokepoint_CancelledNormalResume`, `TestAskWiring_ResumeHoldsTurnMutex` (dialog answered while turnMu held → NO result until release), `TestGatePermissionSuspensionArmsSettle` (engine chain parks at the dialog, resumes after), `TestAskQueueDrainResolvesAsync` (cancel path never stalls the reader). Drain wired on all three teardown paths: `internal/acp/handlers.go:48` (session/cancel, before cancelTurn), `:58` (logout), `internal/acpserve/acp_serve.go:302` (serve shutdown). |
| 3 | Learning-store and engine asks render as structured elicitation/create forms on current clients; on older clients the -32601 probe degrades to the plain-text AskBroker path automatically; the answer lands as the tool result | ✓ VERIFIED (deterministic half; live-Zed rendering → Human Verification 2) | `BuildElicitationForm` D-08 mapping pinned by `TestElicitationMapping` (oneOf titled consts / array enum-or-anyOf / free-text / empty-Options / boolean capability branches); `TestPermissionsE2E` capable scenario (form → accept → answered tool result) and degraded scenario (-32601 → plain-text chunk byte-parity vs `coreexec.RenderAskSurface`) green under -race; sticky degradation pinned by `TestGateDegradedClient`; byte-identity golden `TestElicitationParity` + `TestElicitationRevalidation` green. |
| 4 | Default remains ungated (zero new dialogs vs v1.1); mode flips live via editor configOptions on the running session | ✓ VERIFIED | `TestGateChokepoint_UngatedDefaultNoDialog` + `TestGateChokepoint_UngatedDenyStillDenies` green; `TestPermissionsE2E` ungated-default scenario (zero request_permission frames, zero trust writes) green; live flip: `TestGatePermissionModeFlip` green — the gate reads the mode through a per-call accessor (`internal/session/gate.go` `gateMode()`), `SetPermModeHook`/`EffectivePermMode` boot resolution in `internal/acpserve/config_surface.go:172-187`; real `permissions.mode` handler behind 16-05's advertised slot with WriteLayerOption persist-then-apply. |
| 5 | The gate pipeline precedence (hook verdict → permission ask → execute) is implemented at ONE chokepoint, documented, with permissions.yaml choices surviving restarts | ✓ VERIFIED | ONE chokepoint: `gateCall` at `internal/session/gate.go:153`, invoked in BOTH runTurn branches (`internal/session/session.go:465` batch-eligible, `:533` subagent); no-second-gate region check re-run in this verification: zero `Evaluate(` call sites in internal/session non-test sources outside gate.go; verdict chain in code matches the documented 5-step precedence exactly (hook head → rules in BOTH modes → automation decline D-07 → mode decision → degraded guard) — read side-by-side; `docs/permissions-gate.md` names gateCall 7×, documents the Phase-21 deny-only join contract, permissions.yaml grammar, and the deliberate CC divergences. Restart survival: `TestStoreAllowToolPersistsAndSurvivesRestart`; WR-01 fail-safe quarantine (`perm.OpenRepaired` at `internal/runtime/runtime.go:1375`) replaces the review's fail-open. |

**Score:** 5/5 truths verified (0 present, behavior-unverified — every behavior-dependent truth above has a passing -race test)

Supporting plan-level truths (17-01..17-05 frontmatter, 27 truths total) each map to a passing named pin enumerated in this verification: `TestRules*`/`TestStore*`/`TestOpen*` (23 pins, grammar + store discipline incl. idempotency, 0600-atomic rename-failure, quote-aware compound split, mid-pattern-star skip, loose-perm tightening), `TestAskQueue*` (8 pins: serialization, priority, notes/no-subscriber, concurrency, DrainTurn/DrainAll/async-resolve), `TestGate*` (16 pins incl. multi-call batch suspension, resume-serial, settle arming, automation decline, MCP namespace, outcome matrix), `TestElicitation*`/`TestStructuredReplyRender` (mapping, revalidation, byte-parity). All green.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/perm/rules.go` | ParseRule + RuleSet deny→ask→allow evaluator, compound split, MCP helpers | ✓ VERIFIED | 496 lines; consumed by gate.go (`perm.RuleSet.Evaluate`, `perm.SplitMCPName`/`MCPName`); zero net/* deps (prohibition-enforcing) |
| `internal/perm/store.go` | 0600-atomic store, both-direction dialog writers, OpenRepaired | ✓ VERIFIED | 364 lines; `0o600/0o750` consts, temp+rename `renameFunc` seam, `validSimpleEntry` API boundary; wired via `runtime.go:1375` |
| `internal/session/gate.go` | gateCall — THE per-call chokepoint | ✓ VERIFIED | 490 lines; called from both runTurn branches; verdict chain matches doc |
| `internal/session/askqueue.go` | AskQueue full D-11..D-13 semantics incl. DrainTurn/DrainAll | ✓ VERIFIED | 472 lines; `DrainTurn` is the one shared drain; DrainAll atomic (a710b75); note const `"ask queued — %d pending"` |
| `internal/session/ask.go` | PendingAskKindPermission + permission/elicitation resume variants | ✓ VERIFIED | 1034 lines; persist-then-execute, D-10 bounded loop, RenderStructuredReply |
| `internal/acp/types.go` | RequestPermissionFrame + ElicitationFormFrame, four option kinds | ✓ VERIFIED | Methods `session/request_permission` / `elicitation/create`; camelCase wire fields |
| `internal/acpserve/ask_surface.go` | PermissionAsk + ElicitationAsk + BuildElicitationForm + D-10 validator | ✓ VERIFIED | 795 lines; registry Call under HUMAN-ASK; -32601 sticky degrade |
| `internal/acpserve/config_surface.go` | Real permissions.mode handler + live apply + boot resolution | ✓ VERIFIED | 1080 lines; `optPermissionsMode`, WriteLayerOption, EffectivePermMode |
| `internal/runtime/runtime.go` + `cron_wiring.go` | Gate composition, fire-callback injection, resume-serial, automation bracket | ✓ VERIFIED | GateDeps at :1347, SetResumeSerial :1316, SetPermissionAskFire/SetAskFire, SetTurnOriginAutomation bracket cron_wiring.go:163-165 |
| `docs/permissions-gate.md` | The ONE-chokepoint contract document | ✓ VERIFIED | 299 lines; names gateCall 7×, precedence chain, Phase-21 join contract, grammar, CC divergences, degradation family — matches code as read |
| `internal/acpserve/permissions_e2e_test.go` | TestPermissionsE2E five-scenario battery | ✓ VERIFIED | 937 lines; all five scenarios green under -race (11.9s) in this verification |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `internal/session/session.go` runTurn (both branches) | `internal/session/gate.go` | `s.gateCall(...)` per call | ✓ WIRED | session.go:465 and :533; only rule-evaluation call sites |
| `internal/session/gate.go` | `internal/perm` | `perm.RuleSet.Evaluate` + MCP helpers | ✓ WIRED | gate.go:178, 270-279; no Evaluate outside gate.go (region check re-run) |
| gate suspension | `internal/session/askqueue.go` | `deps.Queue.Enqueue(entry, resolve)` | ✓ WIRED | gate.go:406; elicitation ask.go:646/802/808; engine runtime.go:1755 |
| `internal/acpserve/ask_surface.go` | outbound Registry | `session/request_permission` / `elicitation/create` under HUMAN-ASK | ✓ WIRED | ask_surface.go:6-11, 105-146, 598-693 |
| `internal/acpserve/config_surface.go` | gate mode accessor | `SetPermModeHook` live-apply + `EffectivePermMode` boot | ✓ WIRED | config_surface.go:110-187, 367; gate reads per call |
| teardown paths (cancel / logout / shutdown) | the one drain | `DrainAsks`/`DrainSessionAsks`/`DrainAllAsks` → `DrainTurn` | ✓ WIRED | handlers.go:48,58; acp_serve.go:302; askqueue.go:275,331,468 |
| `docs/permissions-gate.md` | `internal/session/gate.go` | both name `gateCall`; verdict chain identical | ✓ WIRED | doc §1.1-1.2 ↔ gate.go:151-244 (read side-by-side) |
| `internal/acpserve/permissions_e2e_test.go` | 16-06 simulator | drives the real serve composition over pipes | ✓ WIRED | E2E green; 17-05 records a scratch mutation check (unwiring fire seams fails the battery) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Permission dialog | outcome (optionId) | real client via Registry (HUMAN-ASK) | yes — E2E answers allow_always and asserts the rule on disk | ✓ FLOWING |
| permissions.yaml | rule entries | real dialog writes / hand edits, 0600 atomic | yes — E2E asserts the on-disk entry pre-execution | ✓ FLOWING |
| Elicitation form | accept.content | real client answer, D-10-validated | yes — E2E capable scenario asserts the answered tool result | ✓ FLOWING |
| Mode accessor | permissions.mode | config layer via EffectivePermMode / live hook | yes — E2E sets gated through the wire, ungated scenario asserts zero frames | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Full build | `go build ./...` | exit 0 | ✓ PASS |
| Grammar + store pins (23) | `go test -race ./internal/perm/ -count=1` | ok 1.8s | ✓ PASS |
| Queue/gate/elicitation pins (26 named) | `go test -race ./internal/session/ -run 'TestAskQueue\|TestGate\|TestElicitation' -count=1` | ok 1.7s | ✓ PASS |
| Five-scenario E2E battery + mapping | `go test -race ./internal/acpserve/ -run 'TestPermissionsE2E\|TestElicitation' -count=1` | ok 11.9s | ✓ PASS |
| Resume-serial / automation-bracket / engine-chain pins | `go test -race ./internal/runtime/ -run 'TestAskWiring\|TestCronWiring' -count=1` | ok 16.5s | ✓ PASS |
| One-chokepoint region check | `grep -rn "Evaluate(" internal/session/ --include='*.go'` non-test, excl. gate.go | NONE | ✓ PASS |
| Doc-code identity | `grep -c gateCall docs/permissions-gate.md` | 7 (≥1) | ✓ PASS |
| No-network prohibition | `go list -deps ./internal/perm \| grep -c '^net'` | 0 | ✓ PASS |

Full-suite note: the `mise ci` gate (38 packages, -race) was reported green at last execution including the review-fix commits; targeted batteries above re-prove the phase-critical invariants in this verification without a redundant full run.

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| (none declared) | `find scripts -path '*/tests/probe-*.sh'` + PLAN/SUMMARY grep | no probe scripts exist or are declared for this phase | N/A — the TestPermissionsE2E battery is the phase's runnable proof (executed green above) |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| ACP-01 | 17-01, 17-02, 17-03, 17-05 | Clickable permission asks via session/request_permission (allow/reject × once/always; cancelled normal on turn death); permissions.mode ungated|gated switchable via configOptions — available, not default | ✓ SATISFIED | Truths 1, 2, 4, 5 above (REQUIREMENTS.md marks ACP-01 Complete; live-Zed rendering leg → Human Verification 1) |
| ACP-02 | 17-04, 17-05 | Learning-store/engine asks as elicitation/create forms with plain-text fallback and -32601 probe-and-degrade; url mode deferred | ✓ SATISFIED | Truth 3 above; url mode deferred per the requirement text itself (not a gap) |

No orphaned requirements: REQUIREMENTS.md maps exactly ACP-01/ACP-02 to Phase 17. (PAR-03 *references* this phase's pipeline as Phase 21's join point — that is Phase 21's requirement, satisfied here only in its documentation role.)

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER/not-implemented markers in any phase-modified file | — | Clean scan |

WINDOWS ledger entries #12-#14 (open, informational): 17-04 Rule-3 wiring deviations — composition/test/contract code living one file beyond the plan's file list (acp_serve.go, ask_wiring_test.go, askqueue.go). Verified intentional scope-record, not gaps; all three artifacts are wired and pinned.

### Human Verification Required

See the four items in the frontmatter `human_verification` list: (1) live-Zed four-option permission dialog + persistence (WINDOWS #15, seven-step checklist); (2) live-Zed native elicitation form rendering; (3) human review of the five concurrency fixes flagged in 17-REVIEW-FIX.md (all pins RED-proven and green under -race here); (4) judgment-tier prohibition review (4 items, structural evidence recorded, non-authoritative verdicts).

### Gaps Summary

No gaps. All five ROADMAP success criteria are machine-verified with passing behavioral tests; the ONE gate pipeline exists at a single grep-provable chokepoint, is documented in a doc that names the code identically, and the review's 12 findings are fixed with commits that all exist and pins that all pass under -race in this verification. The phase closes machine-side; what remains is exactly the two live-Zed operator legs the plan itself routed to the WINDOWS ledger (#15) plus the fixer's five human-review flags — the established 15-07/16-06 precedent, not unfinished work.

---

_Verified: 2026-09-01T05:02:34Z_
_Verifier: Claude (gsd-verifier)_
