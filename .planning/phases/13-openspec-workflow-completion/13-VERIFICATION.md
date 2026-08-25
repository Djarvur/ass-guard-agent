---
phase: 13-openspec-workflow-completion
verified: 2026-08-25T00:00:00Z
status: passed
score: 3/3 must-haves verified
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "Retroactively confirm (or reverse) the two standing manager flags this phase executed under: (a) the EVAL-NET DISPOSITION 2026-08-20 (eval-gate first green = Phase 13 exit evidence; re-tuning routed to 13-01) and (b) the Rule-4 route-1 ruling (engine-visible ask resume, executed as 13-00)"
    expected: "Explicit operator confirmation recorded in STATE.md/ROADMAP, OR a reversal — the recorded overturn routes are cheap (re-label as 12-09 residual; revert 13-00 commits c528236..bc07dab + route-3 re-disposition)"
    why_human: "Both rulings amend written phase gates and were made under standing operator autonomy, FLAGGED for retroactive confirmation — operator-scope decisions; the verifier confirms they are on paper and the work under them delivered, but cannot ratify them"
  - test: "Flip WINDOWS #8/#9 to fixed (or record their disposition) — STATE.md's 12-08 closure says 'WINDOWS #8/#9 discharge with this closure' but both rows still read status=open in WINDOWS.md (open_count=4)"
    expected: "WINDOWS.md rows #8 (eval first green) and #9 (askTimeout=45s deviation) marked fixed with the flagship-green evidence, or an explicit reason they stay open; /gsd-ship blocks while open_count > 0"
    why_human: "Close bookkeeping is the manager's by instruction; the discharge is evidenced (3 flagship-green artifacts + today's certification) but the ledger rows were not updated"
---

# Phase 13: OpenSpec Workflow Completion Verification Report

**Phase Goal:** As a developer practicing SDD with OpenSpec, I want the toolkit's full command matrix — beyond the proven `explore → propose → apply → archive` loop — to run end-to-end through ass-guard with zero-continue chaining, so that the unmodified toolkit works hands-off, not just its flagship workflow.
**Verified:** 2026-08-20T21:10:25Z (HEAD cfd1cd7)
**Status:** human_needed — 3/3 criteria PASS; two operator ratification items carried (the standing manager flags), no technical gaps
**Re-verification:** No — initial verification

## MVP Mode Note

ROADMAP marks this phase `Mode: mvp`. The goal carries role/capability/outcome unambiguously, and verification proceeded on the three ROADMAP Success Criteria as the contract, per-plan evidence beneath each. **Format discrepancy recorded:** the canonical `user-story.validate` regex returns `false` — the goal uses the "I want *the toolkit's full command matrix* … to run" shape ("I want ⟨noun⟩ to ⟨verb⟩") rather than strict "I want to ⟨verb⟩". Phase 12's accepted goal ("I want every tool in the captured catalog to execute for real…") has the identical shape and was verified as well-formed (12-VERIFICATION MVP note) — this verification follows that house precedent. If the operator wants strict conformance, reword at close; no semantic ambiguity results.

## User Flow Coverage

User story: «As a developer practicing SDD with OpenSpec, I want the toolkit's full command matrix — beyond the proven `explore → propose → apply → archive` loop — to run end-to-end through ass-guard with zero-continue chaining, so that the unmodified toolkit works hands-off, not just its flagship workflow.»

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Operator types one expanded command (`/opsx:new/continue/ff/verify/bulk-archive/onboard`) | The command drives real E2E through ass-guard against the real openspec 1.5.0 binary + real model; command artifacts land on disk | `newOpsxMatrixRunner` (e2e_opsx_matrix_test.go:64-170): guard-before-init, FAIL-LOUD on missing commands/skills, real provider creds REQUIRED (t.Fatalf without them), on-disk assertions per leg; 12 committed captures under testdata/opsx-e2e-matrix/ with provenance headers (verified real model output); the per-command eval suites re-proved each command E2E at k=1 — 12/12 PASS (artifact eval-20260820-202612-k1.json) | ✓ |
| A fixable failure occurs mid-command | The model observes the failure, adapts, and the command's goal is achieved | 6 fixable legs with PROBED deterministic triggers (name collision / bogus sidecar schema / scenario-less delta / incomplete-tasks batch / D-08 idempotent re-run); `scanMatrixFixable` + `assertFixableShape` assert fail-idx-before-recover-idx + goal artifact; 6 fixable captures committed; `AssertFixableRecovery` (harness.go:595) scans tool results AND closings; 6 fixable eval scenarios PASS k=1 | ✓ |
| A stage closes with a handoff phrase | The engine chains the next command — zero manual continues — with audit-trail evidence | BOTH chain legs RE-RUN GREEN BY THIS VERIFIER: `TestOpsxMatrixVerifyChain_Gated` ok 57.3s (D-07 handoff: ≥1 continue decision + opsx:continue provenance + design.md on disk + natural terminal) and `TestOpsxMatrixNewChain_Gated` ok 162.1s (≥2 continues + provenance + tasks.md live-or-archived + natural terminal); seeded rows `post-verify-handoff` + `post-new-continue-handoff` in seeded.toml with per-capture provenance comments; termini dispositions documented in-table (ff shield / bulk unmatched / onboard question class) | ✓ |
| The command hits an interactive dead-end | It surfaces (ask or advisory), never silently stalls | 13-00's engine-visible ask resume: flagship green THROUGH asks (3 artifacts; today's manager certification PASS 173.51s); `TestAskWiring_ChainSurvivesAskTimerResume` + the four `TestAskPark_*` pins green in this verifier's run; 13-03 advisory: classifier → Decide's unmatched cell (ActionNothing — never holds) → one client note per class/session through the real acp.Server — `TestAdvisoryWiring_*` battery green in this verifier's run; onboard leg carries the folded dead-end assertion (e2e_opsx_matrix_test.go:509-519) | ✓ |
| Outcome: the toolkit works hands-off beyond the flagship | The behavioral-eval net covers the expanded matrix on the same re-run gate | 12 per-command scenarios (happy+fixable ×6) + flagship = 13 under internal/evalsuite/scenarios/; full batch pass@1 = TRUE (12/12, 1450s); mise eval-gate flagship green (eval-20260820-205550-k1.json, certified today); failure attribution by scenario id PROVEN by the preserved RED run (eval-20260820-195714 names exactly its 5 failing scenarios) | ✓ |

## Goal Achievement

### Observable Truths (the 3 ROADMAP Success Criteria)

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | SC-1 expanded matrix E2E, happy + fixable, real binary (OS-01) | ✓ PASS | 14 gated test functions in cmd/ass-guard/e2e_opsx_matrix_test.go (6 happy + 6 fixable + 2 chain legs); every leg `e2eGates`-gated with a LOUD skip naming both env flags (e2e_opsx_test.go:55-68 — no silent absence); the runner bootstrap FAILS LOUD when the expanded profile does not install all 6 commands + skills (matrix test :121-137); operator's global openspec config byte-guarded by MEASUREMENT (LIFO cleanup pre/post compare :83-95 + GuardOpenSpecGlobalConfig) — the 13-01 anonymousId incident's process fix holds in code; 12 captures committed with RFC3339/binary-version/model provenance headers; per-command E2E re-proven by the 13-04 suite batch (real runs, fresh scratches, 12/12 pass). Documented divergences are LOUD, not silent: verify is report-driven (no CLI validate step — capture-rescoped, probed-but-unused trigger recorded in the leg comment), wrong-name candidates produce the D-02 ask class per the command files' own mandate |
| 2 | SC-2 zero-continue chaining over the expanded matrix, safety unchanged, dead-ends surfaced (OS-02) | ✓ PASS | (a) 13-00 engine-visible resume: session settle seam (ask.go:77-109,176-181,407-412 — armed at Surface, closed after the resumed runTurn returns), engine ask-wait (observe.go:207-300 — first-turn ActionAsk surfaces BEFORE the wait; settled ⇒ re-read + decide; unresolved ⇒ today's semantics; waits consume no budget), serve park (acp_serve.go:925-1053 — serveCtx-derived parked ctx + ENG-03 watchdog, turnMu-serialized post-settle injections, both drain routes). (b) 13-02 seeds: `post-verify-handoff` (D-07) + `post-new-continue-handoff` with D-12 capture-provenance comments; termini dispositioned in-table. (c) BOTH chain legs green under THIS verifier's own runs (57.3s / 162.1s) — continue decisions + command_provenance + on-disk artifacts asserted from real transcripts (the phase-gate audit-trail requirement). (d) Safety re-pins green with the expanded rows loaded (engine/openspec/session batteries ok, this run): unmatched⇒nothing, re-fire budget, provenance-not-injectable, ask-suspended-never-chains, cancel-drain. (e) 13-03 advisory: ClassifyQuestionEnding (2 classes) → SignalAdvisory in Decide's unmatched cell ONLY → collector-subscribed-before-turn + dedupe + post-done direct emit (acp_serve.go:810-828,1581-1640); the awareness lever honestly NOT applied (corpus shows AskUserQuestion usage — recorded, not skipped silently) |
| 3 | SC-3 per-command eval suites on the same re-run gate (OS-03) | ✓ PASS | 12 scenario files + flagship; schema keys `openspec_profile` + `fixture` validated with unknown-key rejection intact (suite.go:174,194,203-209); named keys `change_dir` + `fixable_recovery` (13-04 additions, offline-unit-tested); BootstrapExpandedScratch wires 13-01's guard into the scenario path (suite.go:391); FULL batch pass@1 TRUE — 12/12 scenarios (artifact eval-20260820-202612-k1.json, 1450s, /tmp/eval-matrix-evidence-full2.log "ok 1450.035s"); gate surface: `TestEvalSuite_Matrix_Gated` behind ASSGUARD_EVAL_GATE with per-suite MATRIX_SUITE_ID; mise eval-gate stays flagship-scoped (documented decision — the 15m budget) and mise ci gained NO dependency (tasks.ci untouched); detector selftest green this run; honest D-06: the first batch RED (5 named failures) preserved, source fixes followed, then green |

**Score:** 3/3 criteria PASS

### The ACP-08 Deferred Evidence (Phase-12 PARTIAL closure)

CLOSED as dispositioned. The flagship suite's first green exists — repeatedly and independently:
- eval-20260820-161520-k1.json (13-00's gate, 781.8s, pass_at_k=true; archived + run log at /tmp/eval-net-evidence/13-00-gate/ — read by this verifier)
- eval-20260820-163220-k1.json (13-01 Task 1 re-verify on the 13-00 tree; /tmp/eval-net-evidence/13-01-task1/)
- eval-20260820-205550-k1.json (today's manager certification: TestEvalSuite_Flagship_Gated PASS 173.51s, log /tmp/evalgate-final.log — read by this verifier)
- The re-tuning landed as seeded.toml row edits ONLY (RE-TUNED 2026-08-20 provenance comment in-file; the apply re-fire loop's cause documented); internal/evalsuite was byte-untouched across 13-00 (git diff 0d3c3ab..839e1a6 -- internal/evalsuite/ = empty, re-proven by this verifier).

The structural premise of the disposition (Phase 13's own gate could not close without the flagship green) proved true; nothing observed that should make the operator decline ratification.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| cmd/ass-guard/e2e_opsx_matrix_test.go (1038 lines) | 12-leg matrix + 2 chain legs, guard-before-init runner | ✓ VERIFIED | All 14 test functions present; assertions real (disk + transcript + audit trail); loud gating |
| cmd/ass-guard/testdata/opsx-e2e-matrix/ (12 captures + README) | D-03 pass-1 corpus, provenance headers | ✓ VERIFIED | Headers carry RFC3339 date, openspec 1.5.0, "real binary + real model"; content is genuine model output |
| internal/evalharness/harness.go + profile_guard_test.go | GuardOpenSpecGlobalConfig + ExpandedMatrixProfileJSON + BootstrapExpandedScratch + assertion keys | ✓ VERIFIED | Guard battery green; AssertChangeDir/AssertFixableRecovery read real disk + transcripts |
| internal/session/ask.go | Per-suspension settle signal (13-00) | ✓ VERIFIED | Armed fresh at Surface; closed after resumed runTurn; AskSettleChan accessor; 5-test battery green |
| internal/engine/observe.go + types.go | AskSettler capability + ask-wait (13-00) | ✓ VERIFIED | Full contract implemented incl. nested-ask loop + cancel-drain; no-capability path byte-identical |
| cmd/ass-guard/acp_serve.go (park + advisory wiring) | Parked chains, drains, advisory collector/dedupe/emit | ✓ VERIFIED | serveCtx-derived ctx + watchdog; both drain routes pinned; collector subscribes before the turn |
| internal/engine/advisory.go + decide.go | Classifier + unmatched-cell advisory (13-03) | ✓ VERIFIED | ActionNothing preserved; advisory lives ONLY in the unmatched cell; battery green |
| internal/openspec/seeded.toml | Re-tuned flagship rows + 2 handoff rows + 6 mutability rows | ✓ VERIFIED | Capture-provenance comments per D-12; termini dispositioned in-table |
| internal/evalsuite/scenarios/opsx-* (12 files) | Per-command happy + fixable suites | ✓ VERIFIED | Schema-valid; fixture keys seeded; all green k=1 in the batch artifact |
| internal/evalsuite/suite.go + suite_test.go | Named keys + schema validation + attribution | ✓ VERIFIED | Offline battery green this run (TestLoadScenarios_MatrixKeys, TestRunSuite_MatrixAssertKeys, TestRunSuite_FailureAttribution) |
| cmd/ass-guard/evalsuite_bridge_test.go | Flagship gate + TestEvalSuite_Matrix_Gated | ✓ VERIFIED | Flagship PASS today (certified); matrix gate per-suite selectable |
| .mise.toml | eval-gate/eval-deep/eval-check-changed; ci untouched | ✓ VERIFIED | tasks.ci = vet/lint/build/test only |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| matrix runner bootstrap | evalharness guard | GuardOpenSpecGlobalConfig before `openspec init` + command-file existence assertions | ✓ WIRED | The assertion IS the wiring proof (matrix test :97, :121-137) |
| seeded.toml [command_mutability] | expansion-time boundary | 6 new keys classify the expanded commands | ✓ WIRED | Rows present; openspec battery green loading them |
| session resume path | engine ask-wait | resumeAskClaimed closes settle → adapter AskSettler → Observe re-reads + decides | ✓ WIRED | TestAskWiring_ChainSurvivesAskTimerResume green this run (the T1 pin) |
| serve prompt path | parked chain | response returns at suspension; chain parks; post-settle injections turnMu-serialized | ✓ WIRED | TestAskPark_PromptResponsePrecedesResolution + drains green this run |
| Decide unmatched cell | advisory note on the client | collector → advisoryNoteDue dedupe → in-hand emit | ✓ WIRED | Server-level TestAdvisoryWiring_QuestionEndingNote green this run |
| chain legs | audit trail | TypeEngineDecision/TypeCommandProvenance assertions on real transcripts | ✓ WIRED | Both legs green under this verifier's own runs |
| scenario files | gate surface | LoadScenarios + TestEvalSuite_Matrix_Gated + mise eval-gate (flagship) | ✓ WIRED | 12/12 batch green; flagship green certified today |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| Matrix legs | closings, artifacts | real openspec 1.5.0 + real model runs | Yes — committed captures + fresh scratches per leg | ✓ FLOWING |
| Chain legs | continue decisions, provenance | engine over real session transcripts | Yes — asserted in this verifier's green runs | ✓ FLOWING |
| Eval suites | per-scenario outcomes | real gated runs → .ass-guard/eval/*.json (12 artifacts on disk, Aug 20) | Yes | ✓ FLOWING |
| Advisory | classification + note | real turn endings → classifier → ACP emitter | Yes — server-level battery through real acp.Server | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Offline batteries: engine + session + openspec + evalsuite | go test ./internal/engine/ ./internal/session/ ./internal/openspec/ ./internal/evalsuite/ -count=1 | all ok (0.99s/2.2s/7.3s/9.2s) | ✓ PASS |
| 13-00 pins: ask-resume chain survival, park promptness, both drains, reply-during-park, provenance unspoofable, cancel-drains-injections | go test ./cmd/ass-guard/ -run 'TestAskWiring\|TestAskPark\|TestEngine_CommandProvenanceNotInjectable\|TestCancelDrainsInjections' -count=1 | ok 6.199s (all named pins PASS) | ✓ PASS |
| 13-03 advisory wiring battery (note through real server, dedupe, wording elements) | go test ./cmd/ass-guard/ -run 'TestAdvisoryWiring' -count=1 (in same run) | ok | ✓ PASS |
| D-07 verify→continue chain, real binary + model | ASSGUARD_OPENSPEC_BIN=1 ASSGUARD_E2E_LLM=1 go test ./cmd/ass-guard/ -run TestOpsxMatrixVerifyChain_Gated | ok 57.285s | ✓ PASS (this verifier's run) |
| new→continue artifact-walk chain, real binary + model | ASSGUARD_OPENSPEC_BIN=1 ASSGUARD_E2E_LLM=1 go test ./cmd/ass-guard/ -run TestOpsxMatrixNewChain_Gated | ok 162.113s | ✓ PASS (this verifier's run) |
| Change-class detector selftest | ./scripts/eval-change-class.sh --selftest | selftest ok, exit 0 | ✓ PASS |
| 13-00 secluded-packages (evalsuite byte-untouched) | git diff --stat 0d3c3ab..839e1a6 -- internal/evalsuite/ | empty | ✓ PASS |
| Zero new dependencies (cross-phase invariant) | git log -- go.mod go.sum | last touched Phase-8 era; zero Phase-13 commits | ✓ PASS |
| Full matrix batch green | read .ass-guard/eval/eval-20260820-202612-k1.json + /tmp/eval-matrix-evidence-full2.log | 12/12 pass_at_k=true; "ok 1450.035s" | ✓ PASS |
| Flagship gate (manager-certified today) | read /tmp/evalgate-final.log + artifact eval-20260820-205550-k1.json | PASS 173.51s, pass_at_k=true | ✓ PASS (external certification, logs read) |
| mise ci (manager-certified today) | read /tmp/ci-final-p13.log | 28/28 packages ok, race battery, lint 0 issues | ✓ PASS (external certification, log read) |

### Probe Execution

No `scripts/*/tests/probe-*.sh` declared; this phase's probe equivalents are the gated Go legs above + the detector selftest. Step 7c: N/A.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| OS-01 | 13-01 | Expanded matrix E2E, happy + fixable, real binary | ✓ SATISFIED | 12 legs + captures + per-command eval re-proof; documented divergences loud |
| OS-02 | 13-00 + 13-02 + 13-03 | Zero-continue chaining incl. ask-survival + safety unchanged + dead-end surfacing | ✓ SATISFIED | Engine-visible resume landed + pinned; both chain legs green under this verifier's runs; advisory battery green |
| OS-03 | 13-04 | Per-command eval suites on the same re-run gate | ✓ SATISFIED | 12 scenarios green k=1; same gate surface; ci unchanged; attribution proven by preserved RED run |

Orphaned requirements: none — REQUIREMENTS maps exactly OS-01/02/03 to Phase 13, each claimed by plans. Note: REQUIREMENTS.md status cells still read "Pending" and ROADMAP Phase-13 plan checkboxes are unchecked — stale close-bookkeeping for the manager (all five summaries exist and the work verified on disk).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (all 13 phase production/test files scanned) | — | TBD/FIXME/XXX/TODO/placeholder | — | NONE FOUND (clean) |
| WINDOWS.md ledger | — | #8/#9 still `open` while STATE.md records their discharge | ⚠️ Warning | Bookkeeping only — the discharge evidence exists (flagship green ×3); the rows were never flipped; blocks /gsd-ship at open_count=4. Manager close item (human item 2) |
| internal/evalsuite scenarios opsx-new/opsx-verify | — | Assert `change_dir` only; the descriptions mention the chained handoffs but the key cannot fail on a chaining regression for these two (the change dir exists regardless) | ℹ️ Info | Chaining depth for the matrix is carried by 13-02's dedicated chain legs (which assert decisions + provenance and were re-run green by this verifier); the plan's must-haves required named keys, not chain assertions — recorded as a coverage-depth note for v1.2 suite hardening |
| user-story.validate | — | Strict canonical regex false ("I want ⟨noun⟩ to ⟨verb⟩" shape) | ℹ️ Info | Same shape as Phase 12's accepted goal; proceeded per house precedent (see MVP Mode Note) |
| STATE.md Concerns | — | 13-01 telemetry anonymousId incident (irreversible; recovery command recorded) | ℹ️ Info | Operator-visible; process fix (guard/bytes-copy) verified in code and held for the rest of the phase |

### Ledger Integrity (the three retroactive flags)

1. **EVAL-NET DISPOSITION (2026-08-20)** — on paper (STATE.md Decisions, FLAGGED for retroactive operator confirmation). Work delivered: re-tuning landed (8c56b29, provenance in seeded.toml), flagship green ×3 on disk. Overturn route recorded (re-label 12-09, zero work lost). Nothing observed that should cause the operator to decline — the disposition's structural premise proved true.
2. **Route-1 ruling / 13-00 (manager Rule-4, 2026-08-20)** — on paper (STATE.md Decisions `[13-00 EXECUTED…]` + the Phase-13 blocker entry with FIX ROUTE DECIDED and the EXECUTED closure; 13-00-PLAN.md committed 0d3c3ab naming the ruling provenance). Work delivered: all five invariants pinned by named tests green under this verifier's runs; secluded-packages discipline held (evalsuite zero-delta proven); wire byte-compat pinned server-level. Overturn route recorded (revert c528236..bc07dab + route-3).
3. **13-00 plan itself** — committed, checker-fixed (c528236), and executed wave-0 ahead of 13-01; the plan text carries the binding ruling citation. On paper.
4. **WINDOWS this phase**: #8/#9 were OPENED 2026-08-20T13:29 (pre-green) and their discharge is recorded in STATE's 12-08 closure — but the ledger rows were never flipped (see Warning above). No other windows opened this phase; the 13-01 config incident is recorded in STATE Concerns with the recovery path (not a WINDOWS-class defect: functional state restored, loss is an anonymous telemetry id).
5. **Blockers section consistent**: the Phase-13 ask-suspension blocker and the 12-08 drift blocker both carry EXECUTED/CLOSED closures whose claims this verification independently corroborated (code + tests + artifacts + own runs).

### Human Verification Required

### 1. Retroactive confirmation of the two standing manager flags

**Test:** Confirm or reverse (a) the EVAL-NET DISPOSITION (first green = Phase 13 exit evidence; re-tuning routed to 13-01) and (b) the Rule-4 route-1 ruling (engine-visible ask resume, executed as 13-00).
**Expected:** Explicit operator confirmation in STATE.md/ROADMAP, or a reversal via the recorded overturn routes (cheap: re-label as 12-09; revert c528236..bc07dab + route-3 re-disposition).
**Why human:** Both amend written phase gates under standing autonomy and are FLAGGED for retroactive confirmation. This verifier confirms they are on paper, that the work under them delivered, and found nothing that should cause decline — but ratification is the operator's, and it is also the item 12-VERIFICATION carried as its close condition.

### 2. Flip WINDOWS #8/#9 to fixed

**Test:** Update WINDOWS.md rows #8/#9 (or record why they stay open).
**Expected:** status=fixed with the flagship-green evidence (3 artifacts + certification); open_count drops to 2 (#5/#7 are pre-existing, routed post-adoption/v1.2 dispositions, not Phase-13's).
**Why human:** Close bookkeeping is the manager's by instruction; /gsd-ship blocks while open_count > 0, which is the immediate milestone-completion friction point.

### Gaps Summary

No missing artifacts, no stubs, no unwired links, no orphaned requirements, no debt markers, zero new dependencies. All three ROADMAP success criteria verified with behavioral evidence beyond the executor's claims: this verifier independently re-ran both zero-continue chain legs against the real binary + real model (both green), the full 13-00/13-03 pin batteries, all offline suites, the detector selftest, and read (not trusted) the gate artifacts, the archived flagship logs, the manager's same-day ci/eval-gate certifications, and the preserved RED diagnostics — including /tmp/vchain-debug.log, which corroborates the honest RED-before-green D-06 discipline (the first verify-anchor run genuinely failed and a source fix, not a re-run, followed).

The ACP-08 deferred exit evidence (Phase-12 PARTIAL) is discharged exactly as dispositioned: the flagship suite is green through real mid-chain asks, three times over, with the assertions and the evalsuite package untouched. The ask-survival fix (13-00) is the manager's route-1 ruling executed to its letter, with the wire contract, no-decision-on-unresolved-ask, unmatched⇒nothing, re-fire-budget, and provenance invariants all re-pinned by named tests that pass under this verifier's own runs.

**Overall verdict: PHASE 13 MAY CLOSE — 3/3 criteria PASS, no technical gaps** — carrying the two operator items above (the standing retroactive confirmations, which are the same item Phase 12's close condition reserved for the operator, plus the WINDOWS ledger flip). Subject to those, the v1.1 milestone's remaining completion bar is unchanged: the operator begins daily ACP use.

---

_Verified: 2026-08-20T21:10:25Z_
_Verifier: ZCode (gsd-verifier)_
