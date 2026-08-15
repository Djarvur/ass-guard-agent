---
phase: 08-slash-command-kickoff
plan: 06
subsystem: opsx-e2e-gate
tags: [e2e, real-binary, real-model, chaining, pattern-reseed, uat, blocker]
status: checkpoint-blocked   # overnight addendum below: carry blocker closed by 08-07; gate re-blocked on core tool execution

# Dependency graph
requires:
  - phase: 08-02/08-03/08-04/08-05
    provides: web tools, real openspec tools, expansion seam, skills — the full kickoff chain
provides:
  - chaining plumbing: [[patterns]] next field + NextPromptFor + Decision.NextPrompt population (engine optional ContinuePopulator) + stage vocabulary (post-explore/propose/apply/archive)
  - the gated real-/opsx E2E harness (double env gate, scratch bootstrap with the real binary, zero-continue assertion set, capture mode, fixable-recovery probe)
  - the real-model loop-bound finding (16→64) and the WITHIN-TURN TOOL-RESULT CARRY blocker diagnosis
affects: [the phase gate (blocked), gap-closure planning for within-turn conversation assembly, Phase 9 re-capture]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "double env gate (binary + live model) with FAIL-LOUD blockers on missing prerequisites — never silent skip"
    - "optional dispatcher interface (engine.ContinuePopulator) — additive capability without an API break"
    - "dynamic NextPromptFor resolution through the pattern-table interface — the table stays the single source of truth"

key-files:
  created:
    - cmd/ass-guard/e2e_opsx_test.go
    - internal/engine/chain_test.go
    - internal/openspec/patterntable_test.go
  modified:
    - internal/openspec/config.go
    - internal/openspec/patterntable.go
    - internal/openspec/seeded.toml
    - internal/engine/observe.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/acp_serve_test.go
    - internal/session/session.go

key-decisions:
  - "engine.ContinuePopulator as an OPTIONAL dispatcher extension (the dispatcher interface only intercepted hook/ask; the plan assumed otherwise — minimal additive engine change, documented deviation)"
  - "E2E two modes: capture drive (manual four stages, produces D-12 raw material pre-re-seed) + product-proof mode (one prompt, zero-continue assertions) — the committed test carries the full assertion set"
  - "the real-model gate surfaced TWO product findings: the stub-era 16-call tool-loop bound (fixed, 64) and the missing within-turn tool-result carry (BLOCKING — see below)"

patterns-established:
  - "stage-bearing pattern ids (post-<stage>-…) select their own hook stage; un-staged ids keep post-implement (backward compatible)"

requirements-completed: []   # CMD-04 remains OPEN — blocked at the gate

# Metrics
duration: 95min
completed: 2026-08-15   # T1 + T2-harness; the gate itself is checkpoint-blocked
---

# Phase 8 Plan 06: The gate Summary

**The chaining plumbing is real and green (RED→GREEN), the gated E2E harness is committed and FAIL-LOUD — and the real-model gate did its job: it surfaced a BLOCKING architectural gap (within-turn tool-result carry) that stub-only v1.0 testing never could. The phase gate stops at the operator checkpoint with that finding on the table.**

## Performance

- **Duration:** ~95 min (T1 + T2 harness + two gated real-model runs + diagnosis)
- **Tasks:** T1 complete; T2 harness complete, gated run BLOCKED by a product finding; T3 blocked (depends on T2's completed capture); T4 = this checkpoint

## Accomplishments
- T1 (RED `a4ce2d1` → GREEN `d2c4677`): `[[patterns]]` gains the `next` field (row format documented in seeded.toml's header; v1.0 rows untouched); `OpenSpecPatternTable.NextPromptFor`; the dispatcher populates `Decision.NextPrompt` for empty-prompt continues via the new OPTIONAL `engine.ContinuePopulator` interface (no engine API break; resolution is dynamic through the pattern-table interface); `triggerFromSignal` learns post-explore/post-propose/post-apply/post-archive; all six T1 behavior tests green under `-race` — including the full-stack injection test (matched handoff → injected `/opsx:apply` arrives EXPANDED with provenance + the mutating-command boundary, through the real adapter)
- T2 harness (`f017e25`-era commit): `TestOpsxEndToEnd_Gated` with the double env gate, REAL-binary scratch bootstrap (`openspec init --tools claude --force` + seeded codebase), REAL zcode profile + REAL provider factory (creds from the repo's `.ass-guard/scheduling.yaml`, FAIL-LOUD when absent), zero-continue assertion set (>= 3 continue decisions, per-stage provenance, archive dir present), capture mode for D-12's raw material, and the fixable-recovery probe (D-10). Clean-skips without the gates (CI-safe)
- Real finding #1 (fixed, `0710d41`-era commit): the stub-era 16-call inner tool-loop bound burned in ~2 min by a real explore turn → raised to 64
- Real finding #2 (BLOCKING, STATE.md): at 64 the turn STILL loops — diagnosis below

## The Blocking Finding (the gate working as designed)

`TestOpsxEndToEnd_Gated` failed twice (`tool loop exceeded max iterations`) with the real model + real binary. Root cause: **the Projector rebuilds an identical lean window every tool-loop iteration** (summary + current intent only) — the model never sees its own tool calls or their results within a turn. Every iteration is a fresh conversation; the model re-explores indefinitely. Pipeline gaps: `provider.ToolCall` carries no ID (pairing impossible), `session.Prompt` records the tool NAME as the call ID, `shaper.Message` is text-only (no tool_use/tool_result blocks), and `provider.ToolResultMessage` (PROV-02) is wired nowhere. The captured zcode sessions DO carry within-conversation tool results (e.g. `model-io-sess_fb066d52…jsonl` messages[7], role `tool`) — the mimicry target's real shape includes them; ass-guard never implemented that half.

Consequences: the real /opsx scenario cannot complete (T2's assertion set cannot go green); T3's re-seed needs a COMPLETED capture (blocked); the phase gate (mise ci + real-binary suites + E2E) cannot close. Disposition required: a gap-closure plan/phase for within-turn conversation assembly (recommendation recorded in STATE.md: unique tool-call IDs through the provider seam → transcript records real IDs → Projector emits the current turn's assistant/tool_use/tool_result lines after the lean seed → Shaper maps structured blocks to provider-native params → parity-check against a captured tool-carrying zcode request).

## Task Commits

1. **T1 chaining plumbing** — `a4ce2d1` (test RED) → `d2c4677` (feat GREEN)
2. **T2 harness** — commit carrying `cmd/ass-guard/e2e_opsx_test.go` (double-gated, FAIL-LOUD)
3. **Loop-bound fix** — 16 → 64 (`internal/session/session.go`, real-gate finding)
4. **Blocker recorded** — STATE.md Blockers/Concerns (this commit)

**Plan metadata:** (this commit)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2] engine.ContinuePopulator added (minimal engine change)**
- **Issue:** the plan assumed the dispatcher could intercept continues from cmd/ass-guard alone; the engine's dispatcher interface only routes hook/ask
- **Fix:** optional `ContinuePopulator` interface — purely additive, existing dispatchers unaffected; full-suite green
- **Committed in:** `d2c4677`

**2. [Rule 2] E2E capture mode added**
- **Issue:** T2's text says the engine chains the first run, but the chained patterns don't exist until T3's re-seed (chicken-and-egg)
- **Fix:** `ASSGUARD_E2E_CAPTURE=1` drives the four stages manually to produce D-12's raw material; the default mode carries the zero-continue product assertions and goes green post-re-seed
- **Committed in:** the T2 harness commit

**3. [Rule 3 - Blocking] The gate cannot pass — within-turn tool-result carry missing**
- **Found during:** T2's gated real-model runs (both failed: 16 and 64 iterations)
- **Status:** BLOCKER recorded in STATE.md; the FAIL-LOUD harness stays committed; T3 + the phase gate await the operator's disposition (gap-closure plan vs. replan)
- **NOT worked around** — per the no-stub-only-evidence rule, a fake-provider "pass" of this gate would be exactly the hollow green the phase exists to prevent

---

**Total deviations:** 3 (2 auto-fixed, 1 blocking — the phase's terminal state)

## Issues Encountered
- The blocking finding above. Everything else green: full `go test ./... -race` suite; `go vet` + `golangci-lint run ./...` (0 issues) + `CGO_ENABLED=0 go build ./...` clean; the three-path real-binary adapter suite green (08-03).

## TDD Gate Compliance
RED `a4ce2d1` → GREEN `d2c4677` (T1, all six tests). T2 is an execute-type harness task; its gated assertion set is deliberately RED until the blocker closes — the gate's honesty property.

## Checkpoint: AWAITING OPERATOR (T4)

**Plan:** 08-06 — the gate. **Progress:** T1 complete (green), T2 harness complete (gated run blocked by the finding), T3/T4 blocked on disposition.

**What the operator is asked to witness/decide:**

1. T1's chaining plumbing is verifiable now: `go test ./internal/openspec/ ./cmd/ass-guard/ ./internal/engine/ -race -run 'TestNextPrompt|TestStageVocab|TestChain'` — green.
2. The blocker: re-run the gated E2E yourself to witness the loop (`ASSGUARD_OPENSPEC_BIN=1 ASSGUARD_E2E_LLM=1 go test ./cmd/ass-guard/ -run TestOpsxEndToEnd_Gated -v -count=1 -timeout 25m`) — it fails loudly with `tool loop exceeded max iterations` after ~8 minutes of real tool calls; the diagnosis + recommended fix path are in STATE.md's Blockers section.
3. Decide the disposition: authorize a gap-closure plan for within-turn conversation assembly (recommended — it is prerequisite infrastructure for CMD-04, Phase 10's Telegram flows, and honest E2E anywhere), or replan.
4. The 11 UAT checks and the pattern re-seed (T3) ride on that decision — they cannot be evidenced until real turns converge.

**Resume signal:** "approved" (accept T1 + the harness as the plan's executable state and disposition the blocker separately), or describe the issue(s)/disposition.

---
*Phase: 08-slash-command-kickoff*
*Checkpoint: 2026-08-15*

---

# Overnight Addendum (2026-08-15, delegated run)

Per the overnight delegation (STATE.md 5f27c6e): 08-07 executed and completed (its blocker's carry half — see 08-07-SUMMARY), then this plan's remainder was attempted autonomously.

## T3 (pattern re-seed): BLOCKED — no capture possible

The re-seed needs COMPLETED stage outputs (D-12's raw material). The gated E2E re-run with the carry fixed still cannot complete a stage: **core tool execution is missing** (Bash/Read/Write/Edit have no Execute implementations — v1.0 shipped the catalog schema-only; only openspec:*/Skill/WebSearch/WebFetch are real). The model sees its tool results now (08-07 proven) but every file-read/CLI path fails structurally, so no stage produces closing output to capture. Recorded as the NEW blocking finding in STATE.md; T3 stays open pending the operator's disposition (recommended gap-closure plan for capture-grounded core tool execution).

## The 11 UAT checks — walked with honest evidence (2026-08-15)

Mechanism-level evidence re-verified green this session; scenario-level legs marked blocked. Per the no-stub-only-evidence rule, blocked checks stay pending rather than passing on mechanism alone where the check's wording demands the real run.

| # | Check | Evidence | Status |
|---|-------|----------|--------|
| 2 | Zero "continue" taps | Plumbing green (TestNextPrompt_*, TestChain_*, engine TestDecide_*); the REAL chain blocked by the core-tool gap | partial |
| 3 | Forgotten routine post-implement | hookdag suite green (OnFailure semantics, seeded steps); the E2E's stage-end hook firing blocked | partial |
| 4 | Unfamiliar → asked once, remembered | learning suite green (TestProposeHooks_*, TestStore_*); real-run ask/remember cycle blocked | partial |
| 5 | Completes unattended | THE gated E2E — blocked (core-tool gap) | blocked |
| 6 | Unmatched output triggers nothing | **PASS**: TestDecide_UnmatchedIsNothing, TestDecide_QuickUnmatchedIsNothing, TestObserve_UnmatchedZeroInjections, + 08-07's TestEngine_ToolResultContentIgnored (assistant-role-only pinned structurally) | pass |
| 7 | Engine failure degrades | **PASS**: TestObserve_LastTurnOutputPanicDegradation, TestObserve_DecidePanicDegradation, TestObserve_RealErrorStopsLoop | pass |
| 8 | Hook on-failure + loop prevention | **PASS**: hookdag config_test (OnFailureHalt pinned), engine TestObserve_ReFireBudget (loop prevention) | pass |
| 9 | Cancel-drain | **PASS**: TestObserve_CancelDrain, TestCancelDrainsInjections (cmd) | pass |
| 10 | Concurrency + swappable backends | **PASS**: TestDispatchBatch_ReadOnlyParallelism, TestDispatchBatch_MutatingSerialization, TestBackend_SwappableWithoutCodeChange, TestBackendsFromConfig_Select | pass |
| 11 | Learning inspectable + revertible | **PASS**: TestStore_ListDeterministicOrder, TestStore_RevertRemovesEntry, TestStore_RevertAtomicReadOnlyDir | pass |
| 12 | Coverage — outcome observably true | Engine observe/decide + hookdag + learning exercised by the suites; the E2E leg pending | partial |

Score: 6 pass, 4 partial (mechanism green, real-run leg blocked), 1 blocked — the phase gate CANNOT close tonight.

## Phase gate status

- `mise run ci` — GREEN (exit 0; one transient flake of the documented internal/profile stability test while a live zcode session writes rollout — re-run clean)
- `ASSGUARD_OPENSPEC_BIN=1 go test ./internal/openspec/ ./internal/ecosys/ -count=1` — GREEN (real-binary three-path adapter + surface suites)
- Gated E2E — **BLOCKED** (core tool execution; STATE.md)

**The gate stays open; the blocker is superseded by the core-tool-execution finding.** Morning disposition needed on that gap; this plan's T3 + gate legs resume after it closes.
