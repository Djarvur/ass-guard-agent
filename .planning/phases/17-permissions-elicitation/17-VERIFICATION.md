---
phase: 17-permissions-elicitation
verified: 2026-09-02T21:53:17Z
status: human_needed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: human_needed
  previous_score: 5/5
  gaps_closed:
    - "G-17-1 (blocker): request_permission responses now decode the CANONICAL ACP v1 nested outcome object — PermissionOutcomeFrame reshaped (inner PermissionOutcome struct, internal/acp/types.go:342-356), Fire switches on the INNER discriminator and maps Selected from the inner optionId (internal/acpserve/ask_surface.go:164-177), the flat shape now fails the decode (errPermissionOutcomeBad), and all three flat-shape test sites corrected (commits 1538673 RED / f372db4 GREEN / d445ab3 docs)"
    - "WR-01 (re-review warning): the selected-without-optionId fail-safe path pinned by commit 2f56969 — TestGateOutcomeMatrix/unknown_option_id_declines_fail-safe/{empty_optionId,non-canonical_optionId} + TestPermissionAskDispatch/selected_without_optionId_passes_through_empty, all green under -race in this verification"
  gaps_remaining: []
  regressions: []
human_verification:
  - test: "Live-Zed re-run of UAT Test 1 (WINDOWS ledger #15, seven-step checklist): in a scratch project set permissions.mode: gated, prompt a mutating tool call, answer the dialog"
    expected: "Zed's native four-option dialog renders (four-option set already operator-confirmed 2026-09-02; A1 caveat closed); with the 17-06 canonical nested decode, an allow_always answer now EXECUTES the call and persists the rule (permissions.yaml gains the allow entry; second identical call runs with no dialog); reject_always denies with no further dialog; every leg persists instead of fail-safe-declining (the original G-17-1 symptom — 'malformed permission outcome: cannot unmarshal object' on every answer — must be gone)"
    why_human: "The machine-side decode is proven (dispatch 8/8 subtests + TestPermissionsE2E 5/5 green with the canonical-answer simulator), but the live Zed 1.18.0 round-trip is the gap's originating evidence and its re-run is pending with the operator — a deterministic simulator proves the wire, not the editor's dialog"
  - test: "Live-Zed re-run of UAT Test 2 (WINDOWS #15 steps 4-5, now UN-BLOCKED): in the same gated session, prompt an AskUserQuestion / trigger a learning-store or engine ask"
    expected: "The ask renders as Zed's native structured form (elicitation/create) — previously untestable because the AskUserQuestion tool call was itself permission-gated and every dialog answer fail-safe-declined before elicitation/create was ever sent; answering lands the answer as the tool result; on a pre-elicitation client the ask degrades to the plain-text path (that degrade leg already PASSED in UAT)"
    why_human: "Native form rendering and answer UX are visual/editor-side behaviors; the capable + degraded simulator scenarios prove the wire frames and byte-parity fallback only"
---

# Phase 17: Permissions + Elicitation — Verification Report (Re-Verification after Gap Closure 17-06)

**Phase Goal:** The agent's asks become clickable editor surfaces instead of plain text — permission asks via session/request_permission with allow/reject × once/always semantics, learning/engine asks via elicitation/create forms — riding the AskBroker suspension pattern so human-timescale waits never hold locks; the ONE gate pipeline (hook verdict → permission ask → execute) is locked and documented here so Phase 21's hooks join rather than bolt on.
**Verified:** 2026-09-02T21:53:17Z
**Status:** human_needed (all machine-verifiable must-haves VERIFIED; 2 live-Zed operator legs remain pending per the 15-07/16-06 WINDOWS-ledger precedent)
**Re-verification:** Yes — after gap-closure plan 17-06 (G-17-1) and the WR-01 pin. Previous report (2026-09-01, 5/5, human_needed) is in git history; this report regenerates it against the post-17-06 tree with all load-bearing checks re-run.

## Goal Achievement

### Observable Truths

Merged must-haves: the 5 ROADMAP success criteria (the contract) with plan-frontmatter truths (17-01..17-06) as supporting detail. Every truth was verified with a passing behavioral test re-run in THIS verification against the post-17-06 tree — not symbol presence, not prior-report text.

| # | Truth (ROADMAP SC) | Status | Evidence |
|---|--------------------|--------|----------|
| 1 | With `permissions.mode: gated`, a mutating tool call pops the native permission dialog with allow/reject × once/always options; the chosen option persists across subsequent calls (always = no further ask) | ✓ VERIFIED (deterministic half incl. the canonical wire decode; live-Zed rendering → Human Verification 1) | **G-17-1 closed:** `PermissionOutcomeFrame` now declares the canonical NESTED union (`PermissionOutcome{Outcome, OptionID}` at internal/acp/types.go:342-356, doc comments cite the schema defs + Zed 1.18.0); `Fire` switches on `of.Outcome.Outcome` and maps `Selected: of.Outcome.OptionID` (internal/acpserve/ask_surface.go:164-177). Re-run green in this verification: `TestPermissionAskDispatch` 8/8 subtests (selected, selected_without_optionId, cancelled, flat_outcome_nonconformance, unknown_inner_discriminator, -32800, -32601, timeout) and `TestPermissionsE2E` 5/5 under -race (11.5s) with the simulator answering canonical nested (permissions_e2e_test.go:385). Persist-then-execute + reject_always-persist matrix at internal/session/ask.go:894-965; restart survival `TestStoreAllowToolPersistsAndSurvivesRestart` green in the full perm-package run. |
| 2 | While a permission dialog is open the session stays alive and responsive — other prompts queue, cancellation works, turn death mid-ask delivers cancelled as a NORMAL response, nothing holds the turn mutex | ✓ VERIFIED | Re-run green in this verification: `go test -race ./internal/session/ -run 'TestAskQueue\|TestGate\|TestElicitation'` ok 1.7s — includes `TestGateChokepoint_SecondPromptWhileDialogOpen`, `TestGateTurnDeath`, `TestGateChokepoint_CancelledNormalResume`, `TestAskWiring_ResumeHoldsTurnMutex` (runtime sweep, ok 9.3s), `TestGatePermissionSuspensionArmsSettle`, `TestAskQueueDrainResolvesAsync`, `TestAskQueueDrainAll`. Drain wired on all three teardown paths (re-confirmed via the green full acpserve sweep, ok 51.8s). |
| 3 | Learning-store and engine asks render as structured elicitation/create forms on current clients; on older clients the -32601 probe degrades to the plain-text AskBroker path automatically; the answer lands as the tool result | ✓ VERIFIED (deterministic half; live-Zed native form now UN-BLOCKED → Human Verification 2) | Re-run green: `TestElicitationMapping` (D-08 table) + `TestPermissionsE2E` capable scenario (form → accept → answered tool result) and degraded scenario (-32601 → plain-text byte-parity vs `coreexec.RenderAskSurface`) under -race. The elicitation response shape (flat action + content) deliberately untouched by 17-06 — verified canonical against CreateElicitationResponse; permAnswerAccept carries the canonical-flat comment (permissions_e2e_test.go:389-399). UAT plain-text-degrade leg already operator-PASSED; the native-form leg was blocked by G-17-1 and is now re-testable. |
| 4 | Default remains ungated (zero new dialogs vs v1.1); mode flips live via editor configOptions on the running session | ✓ VERIFIED | Re-run green: `TestPermissionsE2E` ungated-default scenario (zero request_permission frames) + `TestGatePermissionModeFlip` (gate_test.go:1545) in the green session sweep; gate reads the mode per call through the accessor (`gateMode()` in gate.go), `SetPermModeHook`/`EffectivePermMode` in config_surface.go. |
| 5 | The gate pipeline precedence (hook verdict → permission ask → execute) is implemented at ONE chokepoint, documented, with permissions.yaml choices surviving restarts | ✓ VERIFIED | Re-run in this verification: `grep -rn 'Evaluate(' internal/session/ --include='*.go'` non-test outside gate.go → ZERO matches (no second gate); `gateCall` invoked in BOTH runTurn branches (session.go:465 batch-eligible, :533 subagent — re-read); `grep -c gateCall docs/permissions-gate.md` = 7; restart survival `TestStoreAllowToolPersistsAndSurvivesRestart` green in the full `go test -race ./internal/perm/` run (ok 1.6s). WR-01 fail-safe quarantine (`perm.OpenRepaired`) wired at runtime.go. |

**Score:** 5/5 truths verified (0 present, behavior-unverified — every behavior-dependent truth has a passing -race test re-run in this verification)

Supporting plan-level truths (17-01..17-06 frontmatter) map to pins all green in this verification's runs: `TestRules*`/`TestStore*`/`TestOpen*` (full perm package), `TestAskQueue*` (8 pins), `TestGate*` (16 pins incl. the new `TestGateOutcomeMatrix/unknown_option_id_declines_fail-safe` rows), `TestElicitation*`/`TestPermissionAskDispatch` (8 subtests), `TestAskWiring*`/`TestCronWiring*` (runtime sweep).

### Gap Closure Verification (17-06 + WR-01)

| Item | Claim | Status | Evidence (re-verified) |
| ---- | ----- | ------ | --------------------- |
| G-17-1 canonical nested decode | Production decodes `{"outcome":{"outcome":"selected","optionId":"…"}}` | ✓ VERIFIED | Commits 1538673 (RED) / f372db4 (GREEN) / d445ab3 (docs) present in git log; nested struct + inner-discriminator switch read in code (types.go:342-356, ask_surface.go:164-177); RED proof recorded verbatim in 17-06-SUMMARY (the live-Zed failure string reproduced); dispatch + E2E green under -race here |
| G-17-1 simulator fidelity | E2E answers the canonical shape | ✓ VERIFIED | `grep '"outcome":{"outcome"'` → 4 sites in ask_surface_test.go, 2 in permissions_e2e_test.go (incl. the wire send at :385); TestPermissionsE2E PASS 11.46s |
| G-17-1 fail-safe preservation | Flat/unknown/empty answers can never produce an allow | ✓ VERIFIED | Sentinels live: errPermissionOutcomeBad (unmarshal wrap), errPermissionOutcomeUnknown (switch default); flat_outcome_nonconformance + unknown_inner_discriminator subtests PASS via errors.Is; empty optionId → session gate default decline — pinned by WR-01 |
| WR-01 pin | Empty/non-canonical optionId declines fail-safe | ✓ VERIFIED | Commit 2f56969 in git log; `TestGateOutcomeMatrix/unknown_option_id_declines_fail-safe/{empty_optionId_(selected_without_optionId), non-canonical_optionId}` PASS + `TestPermissionAskDispatch/selected_without_optionId_passes_through_empty` PASS (re-run -race, verbose) |
| Review re-review | 17-06 diff reviewed | ✓ VERIFIED | 17-REVIEW.md (2026-09-02): status fixed, 0 critical, WR-01 fixed; canonical sources fetched live during review |

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/perm/rules.go` | ParseRule + RuleSet deny→ask→allow evaluator, compound split, MCP helpers | ✓ VERIFIED | Full package suite green under -race (this verification); zero net/* deps (prohibition) |
| `internal/perm/store.go` | 0600-atomic store, both-direction writers, OpenRepaired | ✓ VERIFIED | Green in perm run incl. restart survival |
| `internal/session/gate.go` | gateCall — THE per-call chokepoint | ✓ VERIFIED | Region check re-run: zero `Evaluate(` outside gate.go; both branches wired |
| `internal/session/askqueue.go` | AskQueue full D-11..D-13 semantics incl. DrainTurn/DrainAll | ✓ VERIFIED | All TestAskQueue* green in session sweep |
| `internal/session/ask.go` | Permission resume + outcome matrix (persist-then-execute, unknown-option decline) | ✓ VERIFIED | resolvePermissionOutcome re-read (ask.go:894-965); matrix incl. WR-01 rows green |
| `internal/acp/types.go` | RequestPermissionFrame + ElicitationFormFrame + CANONICAL nested PermissionOutcomeFrame | ✓ VERIFIED | Nested struct re-read; v1 spellings; discriminator constants unchanged |
| `internal/acpserve/ask_surface.go` | PermissionAsk + ElicitationAsk + BuildElicitationForm + nested outcome decode | ✓ VERIFIED | Fire decode re-read; sentinels preserved verbatim |
| `internal/acpserve/config_surface.go` | Real permissions.mode handler + live apply | ✓ VERIFIED | TestGatePermissionModeFlip + E2E gated-via-wire green |
| `internal/runtime/runtime.go` + `cron_wiring.go` | Gate composition, resume-serial, automation bracket | ✓ VERIFIED | Runtime sweep green (ok 9.3s) |
| `docs/permissions-gate.md` | The ONE-chokepoint contract document | ✓ VERIFIED | gateCall named 7× (re-run); Phase-21 join contract present |
| `internal/acpserve/permissions_e2e_test.go` | Five-scenario battery, canonical-answer client | ✓ VERIFIED | All five scenario functions present (HappyPath/ElicitationCapable/ElicitationDegraded/TurnDeath/UngatedDefault); PASS under -race |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| Zed 1.18.0 wire answer | `PermissionOutcomeFrame` → `Fire` switch → session persist/execute legs | the exact link that was dead in UAT Test 1 | ✓ WIRED | Nested decode + optionId mapping re-read; E2E happy path asserts rule on disk + execution + no second dialog; WR-01 rows pin the empty-id fall-through |
| `internal/session/session.go` runTurn (both branches) | `internal/session/gate.go` | `s.gateCall(...)` per call | ✓ WIRED | session.go:465 and :533 re-read; only rule-evaluation call sites (region check) |
| gate suspension / asks | `internal/session/askqueue.go` | Enqueue + fire monopoly | ✓ WIRED | Green across session/acpserve/runtime sweeps |
| `internal/acpserve/ask_surface.go` | outbound Registry | session/request_permission / elicitation/create under HUMAN-ASK | ✓ WIRED | Method constants at types.go:290-297; dispatch tests green |
| teardown paths | the one drain | DrainAsks/DrainSessionAsks/DrainAllAsks → DrainTurn | ✓ WIRED | Full acpserve sweep green |
| `docs/permissions-gate.md` | `internal/session/gate.go` | both name `gateCall`; verdict chain identical | ✓ WIRED | 7 mentions; precedence chain read side-by-side in the prior verification, doc unchanged since (17-06 touched no doc semantics beyond the wire comments) |

### Behavioral Spot-Checks (all re-run in this verification, post-17-06 tree)

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Grammar + store pins | `go test -race ./internal/perm/ -count=1` | ok 1.617s | ✓ PASS |
| Queue/gate/elicitation pins | `go test -race ./internal/session/ -run 'TestAskQueue\|TestGate\|TestElicitation' -count=1` | ok 1.688s | ✓ PASS |
| E2E battery + mapping + dispatch | `go test -race ./internal/acpserve/ -run 'TestPermissionsE2E\|TestElicitation\|TestPermissionAskDispatch' -count=1 -v` | PASS 11.46s; all subtests listed PASS | ✓ PASS |
| Runtime wiring pins | `go test -race ./internal/runtime/ -run 'TestAskWiring\|TestCronWiring' -count=1` | ok 9.276s | ✓ PASS |
| WR-01 gate rows (single named test) | `go test -race ./internal/session/ -run 'TestGateOutcomeMatrix' -count=1 -v` | unknown_option_id_declines_fail-safe/{empty_optionId, non-canonical_optionId} PASS | ✓ PASS |
| Full three-package sweep (17-06 verify block) | `go test -race ./internal/acp/ ./internal/acpserve/ ./internal/session/ -count=1` | ok 2.168s / ok 51.814s / ok 4.139s | ✓ PASS |
| One-chokepoint region check | `grep -rn 'Evaluate(' internal/session/` non-test, excl. gate.go | NONE | ✓ PASS |
| Doc-code identity | `grep -c gateCall docs/permissions-gate.md` | 7 | ✓ PASS |
| No-network prohibition | `go list -deps ./internal/perm \| grep -c '^net'` | 0 | ✓ PASS |
| Canonical shape gates | `grep -c '"outcome":{"outcome"'` on both test files; `PermissionOutcome` in types.go | 4 / 2 / 10 (≥3 / ≥1 / ≥2) | ✓ PASS |

Full-suite note: `mise ci` was reported green at 17-06 execution (log /tmp/17-06-mise-ci.log) and the re-review re-ran vet/gofmt/test green; this verification re-ran the complete acp + acpserve + session race sweep (the packages 17-06 touched) plus perm and runtime targeted batteries — a redundant full `mise ci` adds no new evidence for the phase-critical invariants.

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| (none declared) | `find scripts -path '*/tests/probe-*.sh'` + PLAN/SUMMARY grep | no probe scripts exist or are declared for this phase | N/A — TestPermissionsE2E is the phase's runnable proof (re-executed green above) |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| ACP-01 | 17-01, 17-02, 17-03, 17-05, 17-06 | Clickable permission asks via session/request_permission (allow/reject × once/always; cancelled normal on turn death); permissions.mode ungated\|gated switchable via configOptions — available, not default | ✓ SATISFIED | Truths 1, 2, 4, 5 + G-17-1 closure (REQUIREMENTS.md marks ACP-01 Complete; live-Zed round-trip re-run → Human Verification 1) |
| ACP-02 | 17-04, 17-05 | Learning-store/engine asks as elicitation/create forms with plain-text fallback and -32601 probe-and-degrade; url mode deferred | ✓ SATISFIED | Truth 3; url mode deferred per the requirement text itself (not a gap); native-form live leg now un-blocked → Human Verification 2 |

No orphaned requirements: REQUIREMENTS.md maps exactly ACP-01/ACP-02 to Phase 17 (both Complete).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | Debt-marker scan (TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER/not-implemented) across all phase-modified files incl. the four 17-06 files | — | Clean scan (zero matches) |

### Deferred Follow-Ups (recorded, operator-ruled non-blocking — not gaps)

Both were surfaced during the UAT human review (17-UAT.md §Deferred Follow-Ups), ruled non-blocking for Phase 17 by the operator, and are re-confirmed to exist in the current tree by this verification:

1. **CR-04 automation-flag window** — `SetTurnOriginAutomation(true)` is set before `mu.TryLock` (runtime), so a foreground turn overlapping a queued automation turn gets gated asks D-07-declined instead of a dialog (fail-safe direction; UX divergence). Proper fix is per-turn origin. No later-phase roadmap mapping; tracked in 17-UAT.md.
2. **hasSubstitution double-quote regression** (internal/perm/rules.go:288-311, introduced by a710b75's CR-03 rewrite) — the scanner traces single quotes and backslash escapes but has NO double-quote case, so `'` inside `"…"` skips to the next single quote and under-detects live `$(...)`/backtick substitution inside double quotes (e.g. `git "log 'x $(curl evil) y'"`); an allow rule can match a substitution-bearing command, weakening WR-02's one-sided allow guard. Re-read and confirmed real in this verification. Security-adjacent; operator ruled non-blocking, "deserves a fix ticket soon". Recorded in 17-UAT.md.

Neither maps to a later milestone phase's goal/success criteria, so neither is a Step-9b deferred item; both stay visible via the UAT record and this report.

### Human Verification Required

Two items remain — exactly the two live-Zed legs the WINDOWS ledger #15 still carries as PENDING-OPERATOR-CONFIRMATION (see frontmatter `human_verification` for full detail):

1. **Permission dialog round-trip incl. persistence (UAT Test 1 re-run)** — the original G-17-1 evidence; with the canonical nested decode landed, every dialog answer must execute/deny and persist instead of fail-safe-declining. The four-option rendering and clean cancel legs already operator-confirmed (2026-09-02); the answer-decode + persistence legs await re-run.
2. **Native elicitation form (UAT Test 2 re-run)** — un-blocked by G-17-1's fix (the AskUserQuestion gate dialog no longer declines before elicitation/create is sent); native form rendering + answer-lands-as-tool-result legs await re-run. The plain-text degrade leg already PASSED.

Previously listed human items now CLOSED by UAT: the five-fix concurrency review (UAT Test 3: pass — orchestrator pre-review + operator ruling, findings preserved as the deferred follow-ups above) and the four judgment-tier prohibition reviews (UAT Test 4: pass — mechanical checks re-run live, operator confirmed; the prior NON-AUTHORITATIVE flag is lifted).

### Gaps Summary

No gaps. All five ROADMAP success criteria are machine-verified with behavioral tests re-run green in this verification against the post-17-06 tree; gap G-17-1 is closed at the wire with RED-proven pins, a canonical-answer simulator, and the re-review's WR-01 warning pinned (2f56969). The phase closes machine-side; what remains is exactly the two live-Zed operator legs WINDOWS ledger #15 already tracks — the established 15-07/16-06 precedent, not unfinished work.

---

_Verified: 2026-09-02T21:53:17Z_
_Verifier: Claude (gsd-verifier)_
