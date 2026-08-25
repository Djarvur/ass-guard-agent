---
phase: 13-openspec-workflow-completion
plan: "00"
subsystem: engine-ask-resume
tags: [ask-suspension, chaining, engine, serve-park, eval-gate]
requires:
  - "12-01 ask suspension wire contract (live-proven)"
  - "13-01-eval-first-runs/iterations-20260820 diagnosis (code-verified, in-repo)"
provides:
  - "engine-visible ask resume — a turn completing after ask resolution gets its chaining decision (manager Rule-4 route 1)"
  - "Session.AskSettleChan — the per-suspension one-shot settle signal (both resume drivers close it)"
  - "engine.AskSettler optional TurnRunner capability + Observe's ask-wait"
  - "the serve park — prompt response returns at suspension while the chain parks (turnMu-serialized post-settle injections, cancel/close drains)"
  - "sessionTurnRunner.WaitChainIdle — the harness seam's wait-through-suspension"
affects:
  - "13-01 Task 1 (gate precondition discharged — the same flagship green)"
  - "13-02/13-03/13-04 (downstream-unblocked)"
  - "Phase-12 ACP-08 deferred exit evidence (DISCHARGED — first green)"
tech-stack:
  added: []   # zero new dependencies
  patterns:
    - "optional-capability interface (AskSettler, the ContinuePopulator precedent)"
    - "per-suspension one-shot channel armed at Surface, closed at the shared resume path"
    - "parked-chain ctx serveCtx-derived + request-ctx-death watchdog (ENG-03)"
key-files:
  created: []
  modified:
    - internal/session/ask.go
    - internal/session/ask_test.go
    - internal/engine/observe.go
    - internal/engine/types.go
    - internal/engine/observe_test.go
    - internal/engine/goconst_constants_test.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/ask_wiring_test.go
    - cmd/ass-guard/e2e_opsx_test.go
    - cmd/ass-guard/integration_test.go
    - cmd/ass-guard/mcp_tracer_test.go
    - cmd/ass-guard/acp_serve_test.go
decisions:
  - "Route 1 (engine-visible resume) executed per the manager Rule-4 disposition — FLAGGED retroactive; overturn = revert 13-00 commits + route-3 re-disposition"
  - "The first turn's ActionAsk audit line surfaces AT suspension (before the wait) — preserves the 12-01 reply-routing pin"
  - "The park signal fires from AskSettle (after the ask decision is on disk) — no response/audit race"
metrics:
  duration: 255min
  tasks: 5
  commits: 10
status: complete
actuals:
  tokens: 495000   # chars/4 over the realized diff (13-00 range, docs+tests+code)
  tasks: 5
  commits: 10
---

# Phase 13 Plan 00: Gap closure — engine-visible ask resume Summary

**One-liner:** A mid-chain AskUserQuestion no longer kills the zero-continue chain: the session
exposes a per-suspension settle signal, Observe waits on it and decides on the COMPLETED turn, and
the serve layer parks the chain (wire byte-compat preserved) — the flagship eval gate is GREEN
through asks (first green; ACP-08's deferred exit evidence).

## What was built (T1–T5)

| Task | Commit | Content |
|------|--------|---------|
| Step 0 | c528236 | checker fixes applied (B1 split secluded verify with baseline hashes; W1–W5) |
| T1 RED | 1564028 | TestAskWiring_ChainSurvivesAskTimerResume — the diagnosis pinned (both assertions failed: no decision for the asking turn; no apply continuation) |
| T2 | 2ffbea1 → de4ee03 | session settle seam: PendingAsk.settle (json:"-") armed fresh in AskBroker.Surface, mirrored as broker.settleCh (survives Claim); AskSettleChan accessors; resumeAskClaimed closes it AFTER the resumed runTurn returns. 5-test battery: timer settles, reply settles, turn-completion-not-claim (delayed provider), fresh-per-sequential-suspension, block-forever never settles without a driver |
| T3 | a5643df → 766a8ce | engine.StopAsk + optional AskSettler (types.go); Observe's ask-wait — settled ⇒ re-read + decide normally (nested asks loop back into the wait); ctx death ⇒ (cancelled, nil); unresolved ⇒ today's semantics (first turn ActionAsk no-lookup / injection silent exit); no-capability byte-identical; waits consume no budget |
| T3.5 | d07b35a | restructure: the first turn's ActionAsk surfaces BEFORE the wait (the 12-01 audit line — ReplyRouting's pin); injected turns never pre-decide |
| T4 | 77b3128 | the serve park: Observe on a goroutine under a per-session parked ctx (serveCtx-derived + ENG-03 watchdog folding request-ctx death); the adapter implements AskSettler + fires the park signal from AskSettle; runOneTurn returns stopAsk at the suspension (mapAskStop wire byte-compat); post-settle injections hold the session turnMu per turn + stream via the session-lifetime forwarder (WINDOWS #3); cancelParkedChains drains on CloseSession + closeAllSessions; chainEnter/Exit + WaitChainIdle. Pins: server-level response-precedes-resolution (1h timer), cancel+close drains (both routes, no post-drain decision, goroutine released), reply-during-park queues the injection after the resumed turn's closing. **T1's pin GREEN unmodified** |
| T5 | e2295ae → bc07dab | harness mirror (RunPrompt/runStageTyped WaitChainIdle — assertions/scenarios untouched) + lint conformance (awaitAskSettlement extraction, emitBudgetCap, test consts) |

## The exit gate

- `mise run ci` GREEN (vet + lint + build + race test).
- `mise run eval-gate` **GREEN — opsx-flagship pass@1 TRUE**: 3 continues, 4 assistant messages,
  781.8s. Artifact: `.ass-guard/eval/eval-20260820-161520-k1.json` (archived at
  `/tmp/eval-net-evidence/13-00-gate/` with the run log). The flagship zero-continue chain
  completed THROUGH asks — the same exit evidence 13-01 Task 1 consumes; the Phase-12 ACP-08
  deferred evidence is discharged; the 12-08 blocker + the Phase-13 blocker are closed in STATE.md.

## Safety re-pins (all green UNMODIFIED)

TestDecide_AskSuspendedNeverChains, TestDecide_UnmatchedIsNothing, TestObserve_ReFireBudget,
TestObserve_CancelDrain, TestAskWiring_ReplyRouting, TestAskWiring_ServerLevelSurface,
TestAskWiring_ConfigKnob, TestEngine_CommandProvenanceNotInjectable, TestCancelDrainsInjections.

## Secluded packages (checker B1)

- `internal/evalsuite/`: zero delta across the whole plan (`git diff --stat HEAD -- internal/evalsuite/` = 0).
- `internal/evalharness/` at the plan-start draft baseline, hash-verified at T1/T5:
  - `git diff -- internal/evalharness/harness.go | shasum -a 256` = `b07888569929487f91b30a73d0d5c094051c294365ebaa959b8f31b948584029`
  - `shasum -a 256 internal/evalharness/profile_guard_test.go` = `ee655672dd25e9d5ef97cbe4dc965b843afaadec4ad42a8a7af52f72e7684a71`
  - The uncommitted 13-01 Task 2 draft is present, untouched, and was never staged.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] user_message bodies ride Content, not Text — pin (b) assertion**
- Found during: T4 (T1's pin re-run)
- The apply-turn assertion read `lines[i].Text`; user messages carry Content. Switched to the
  existing `lastUserMessageText` helper. Files: ask_wiring_test.go.

**2. [Rule 3 - Blocking] Observe's complexity/lint conformance (gocognit/funlen/goconst cascade)**
- Found during: T5 (`mise ci` lint stage RED)
- Extracted `awaitAskSettlement` (+ observeStep) and `emitBudgetCap` from Observe; hoisted repeated
  test literals to const blocks (including threshold-tripped occurrences in pre-existing test
  files: integration_test.go, mcp_tracer_test.go, acp_serve_test.go — mechanical const swaps,
  zero behavior change). Commit bc07dab.

**3. [Rule 1 - Environmental] TestBash_ProcessGroupKill flake (coreexec, untouched by this plan)**
- Found during: T5's `mise ci`. The first run failed with "orphaned grandchild"; investigation
  showed PIDs 73478/73479 were stale survivors FROM that first run (re-parented to PID 1), poisoning
  every re-run's pgrep. Killed the stale orphans → clean pass, no new orphans, source untouched
  (coreexec last touched by dc17900, pre-Phase-13). D-06: single re-run at zero delta, both shapes
  documented here.

**4. [T4-internal refinement] The park signal fires from AskSettle, not adapter.Run**
- The plan sketched the onSuspended hook "when sess.Prompt returns the ask marker"; firing there
  races the first-turn ask-decision emit against the response return (ReplyRouting's pin could read
  a transcript missing the line). Firing from AskSettle (inside waitAskSettled, after the pre-decide
  emit) makes the ordering structural. Same commit 77b3128.

**5. [T4-internal refinement] ENG-03 watchdog on the parked ctx**
- The plan's parked ctx (serveCtx-derived) alone would miss request-ctx death MID-turn
  (session/cancel's cancelTurn during the first turn — TestCancelDrainsInjections's exact shape).
  A watchdog goroutine folds request-ctx death into the parked cancel; the response's return does
  NOT cancel the request ctx (verified in handleSessionPrompt), so the park survives it. Commit 77b3128.

## TDD Gate Compliance

- T1: test commit (1564028) RED verified (both assertions failing) before every implementation commit.
- T2: RED (2ffbea1, compile-failing battery) → GREEN (de4ee03).
- T3: RED (a5643df) → GREEN (766a8ce); two tests updated at d07b35a for the restructured contract
  (ask-surface first, continue second — the STRONGER pin; documented in the commit).
- T4: pins RED-demonstrated via file-swap against pre-park HEAD (build-RED) + T1's committed
  diagnosis pin as the driver that went green unmodified; committed together (77b3128) — Go's
  compile coupling makes a pins-only commit bisect-hostile.
- T5: seam + gate; no RED owed (verification-only task).

## Known Stubs

None — no placeholder code in this plan's surface.

## Threat Flags

None — no security-relevant surface beyond the plan's threat_model (all six register rows mitigated
as planned; see the safety re-pins + the drain pins).

## Self-Check: PASSED

All 10 task commits found in git log; 13-00-SUMMARY.md exists on disk; gate artifact archived.
