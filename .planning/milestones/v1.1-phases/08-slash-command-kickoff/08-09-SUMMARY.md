---
phase: 08-slash-command-kickoff
plan: 09
subsystem: session-boundary-resets
tags: [gap-closure, boundaries, between-turn-reset, mid-turn-carry, convergence, e2e, locked-invariant-revision]
status: complete   # T1-T2 RED→GREEN + guard green; T3 gated E2E CONVERGED on every turn (the 08-08 max-iterations signature is GONE) with real work on disk — the 08-08 T4 finding closed at its root. SESS-04 revision recorded in STATE.md. Chaining assertions deliberately left to 08-06 T3 (patterns unseeded). Three findings recorded (STATE.md): duplicate tool_use chunks (pre-existing), profile env-cwd static composition, D-10 probe route nondeterminism.

# Dependency graph
requires:
  - phase: 08-07 (mid-turn accumulation machinery + capture pinning)
  - phase: 08-08 (real core executors + plainContent + the T4 finding this plan closes)
provides:
  - "the between-turn boundary reset scoping: Project's reset point = the LAST boundary recorded STRICTLY BEFORE the projected turn's user message (splitAtResetBoundary); accumulateMidTurn anchors SOLELY on the turn's user_message — mid-turn boundary lines never wipe the producing turn's accumulation"
  - "seed stability within a turn: the no-reset-boundary summary scope is the PRE-turn lines (the seed is byte-identical across the turn's iterations — fixes the summary churn + duplicated intent)"
  - "the re-pinned boundary-discipline battery (both halves): TestProjector_MidTurnBoundaryCarry, TestProjector_BetweenTurnResetAfterMidTurnBoundary, TestProjector_SeedStabilityWithinTurn, TestProjector_RepeatedMidTurnBoundaries, TestCoreExec_BashThroughSession (wiring level, boundary-line + carry + next-turn lean seed)"
  - "the revised invariant stated at all four boundary sites (boundary.go, manager.go, session.go batch + subagent) — comment-only, revision-dated, evidence-pointed"
  - "CONVERGENCE proof: the gated E2E turns END with real closing output (explore 118.60s, fixable 83.21s + 151.02s; real archives on disk in both fixable runs)"
affects: [every agentic turn's mid-turn window, 08-06 T3 + UAT + phase gate (resumed on this handback), Phase 9 AUD-05 (cross-turn windowing divergence stays routed there)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "reset-point anchoring on the consuming turn's user message — a boundary is meaningful to a projection only if it PRECEDES that turn's start (between-turn semantics)"
    - "re-pin, never delete: a locked-invariant revision rewrites its tests to assert BOTH halves (the changed half + the preserved half) and documents the revision at every code site + STATE.md"

key-files:
  created: []
  modified:
    - internal/session/projector.go (splitAtResetBoundary + the accumulateMidTurn anchor)
    - internal/session/projector_test.go (the re-pinned battery + three new tests)
    - cmd/ass-guard/coreexec_wiring_test.go (the wiring-level re-pin)
    - internal/session/boundary.go (comment-only)
    - internal/session/manager.go (comment-only)
    - internal/session/session.go (comment-only, two sites)

key-decisions:
  - "SESS-04's reset TIMING re-scoped to between turns under the 08-09 plan's approval authority; D-11's expansion-time boundary VERBATIM (zero code change at acp_serve.go:583 — it precedes the user message, so the re-scoped reader honors it identically)"
  - "writers byte-identical: boundary lines keep schema/cause/turnID; only the Projector's two READERS changed — the minimal diff the plan's scope_note demanded (verified: 6 files, one executable)"
  - "the wiring-level RED ran through the real sessionFor + scripted-provider path (TestCoreExec_BashThroughSession (b)) — the convergence fix proven at the wiring level, not just the unit level"
  - "lint conformance split into its own behavior-identical commit (splitAtResetBoundary extraction for funlen; fixture-id constants; house nolints) so the comment-only claim of T2 stays checkable per-commit"

patterns-established:
  - "between-turn reset + within-turn carry asserted as ONE invariant in one battery (carry half + lean-reset half + writer half)"

requirements-completed: [CMD-04-partial]   # the convergence half; chaining/scenario completion is 08-06 T3's leg (resumed)

# Metrics
duration: 140min
completed: 2026-08-15   # T1-T3 under the standing autonomy; flagged for retroactive confirmation
---

# Phase 8 Plan 09: Gap closure Summary

**The 08-08 T4 finding is closed at its root: real agentic turns CONVERGE even when built from mutating tool calls — the model sees its own prior exchanges on every tool-loop iteration, and the "tool loop exceeded max iterations" signature is GONE from every gated run. The locked-invariant revision is honest and attributable: tests re-pinned (both halves), doc comments revised at every boundary site, the STATE.md decisions entry carries the corpus citations, and the safety model is untouched and re-proven.**

## Performance

- **Duration:** ~140 min (T1 RED→GREEN, T2 guard + lint conformance, T3 the gated E2E + evidence + documentation)
- **Commits:** `1fed319` (RED) → `27b3bb3` (GREEN) → `2dd1f32` (lint conformance, behavior-identical) → `5519f3b` (comment-only invariant docs) → this SUMMARY/STATE commit

## Task Commits

1. **T1 RED** — `1fed319`: the re-pinned battery FAILS on today's reset behavior (MidTurnBoundaryCarry 3-msgs-not-5; SeedStabilityWithinTurn churn; RepeatedMidTurnBoundaries 1-msg; TestCoreExec_BashThroughSession (b) no-carry). The between-turn half (TestProjector_BetweenTurnResetAfterMidTurnBoundary) passed pre-fix — the preserved half pinned alongside.
2. **T1 GREEN** — `27b3bb3`: `Project`'s reset point = last boundary STRICTLY BEFORE the turn's user message (`turnUserIdx` = last user_message with TurnID match, fallback last overall; no-reset-boundary scope = pre-turn lines); `accumulateMidTurn`'s anchor = the turn's user_message ONLY (the TypeBoundary arm deleted). All projector + wiring tests green; the between-turn-layout v1.0 battery green UNMODIFIED (verified by diff: zero mentions in the hunk set).
3. **Lint conformance** — `2dd1f32`: splitAtResetBoundary extraction (funlen), fixture-id constants (goconst), wiringRoleTool, perfsprint/lll/wsl fixes, house nolints on the flat battery. TestCoreExec_ReadOnlyProjection verified byte-identical.
4. **T2** — `5519f3b`: comment-only invariant docs at boundary.go MaybeAppendBoundary, manager.go AppendBoundary, session.go batch-result + subagent-dispatch sites (git diff -U0 non-comment line count: 0). Full guard: `go test ./... -race -count=1` (25 pkgs ok, 0 FAIL) + `mise run ci` green + `TestEngine_ToolResultContentIgnored` (08-07 T3 assistant-role-only regression) green.

## The Corpus Citation (the revision's ground truth)

Verified mechanically 2026-08-15 against the live rollout, session `4440f5a7` (50 request records: 46 `tail` + 4 `delta`, 15 turns): every request carries the ROLLING last-64 messages (`messageOffset` advances 455→459→461→… exactly as messages append); within-turn tool results PERSIST into later requests of the same turn (579 toolCallId persistences across same-turnId request pairs); ZERO resets on tool results. The `delta` records' offset arithmetic (`off_b = off_a + len_a`) proves stream continuation, not reset. Committed fixtures: `internal/shaper/testdata/zcode-midturn-messages.json`, `internal/coreexec/testdata/zcode-core-results.json`.

## The Re-Pinned Tests (before → after)

| Test | Before (mid-turn reset era) | After (between-turn era) |
|---|---|---|
| TestProjector_MidTurnBoundaryReset → **…BoundaryCarry** | boundary wipes the turn's accumulation (seed + post-boundary only, 3 msgs) | seed + BOTH exchanges carried (5 msgs); orphaned result still dropped (pair-safety, boundary-independent); boundary line still recorded with cause + toolCallID |
| **TestProjector_BetweenTurnResetAfterMidTurnBoundary** (new) | (covered only by between-turn-layout tests) | the NEXT turn's projection after a mid-turn boundary = lean seed ONLY (≤3 msgs, no assistant/tool), summary names the prior turn's last user text |
| **TestProjector_SeedStabilityWithinTurn** (new) | (no equivalent — the all-lines fallback churned the seed) | byte-identical seed across the turn's iterations while the accumulation grows 3→7 msgs; no current-turn exchanges in the summary |
| **TestProjector_RepeatedMidTurnBoundaries** (new) | only the post-last-boundary tail survived (the 42/42 E2E signature) | all three mutating exchanges carried past their boundaries (7 msgs) |
| TestCoreExec_BashThroughSession (wiring) | asserted NO tool message past the Bash boundary | (a) the `mutating-command:Bash` boundary line EXISTS (writer half); (b) Project CARRIES the Bash tool message with plainContent `hi`; (c) after a next-turn user message, Project(next) = lean seed (no tool/assistant) |
| TestProjector_LeanSeedAfterBoundary / SummaryExtraction / Truncation / FirstTurn | between-turn layouts | UNMODIFIED, green (git diff shows no edits) |

## T3 — The Gated E2E (acceptance evidence)

Command: `ASSGUARD_OPENSPEC_BIN=1 ASSGUARD_E2E_LLM=1 go test ./cmd/ass-guard/ -run 'TestOpsxEndToEnd_Gated|TestOpsxFixableRecovery_Gated' -v -count=1 -timeout 40m` (+ a fixable-only re-run). Evidence preserved to **/tmp/e2e-08-09-evidence/** (preserver started BEFORE the run — run.log, preserve.log, live scratches; the tail-miss note below).

| Acceptance criterion | Result | Evidence |
|---|---|---|
| (1) CONVERGENCE — both gated turns END, max-iterations gone | **PASS** | EndToEnd 118.60s: turn ended with a real final assistant message (captured to `cmd/ass-guard/testdata/opsx-e2e/stage-1-output.txt`, 3.9KB — the in-process "captured 1 stage outputs" log). Fixable 83.21s + re-run 151.02s: Run returned nil (test reached its assertion, no `stage prompt … tool loop exceeded max iterations` fatal). The 08-08 signature appears NOWHERE in either run.log. |
| (2) ≥1 FULL STAGE of REAL work, zero no-implementation | **PASS** | Explore: 7 requests, 9 unique tool calls all answered, 8 `mutating-command:Bash` boundaries (writer preserved), real `openspec list --json` stdout (root.path = the scratch), real closing report. Fixable ×2: real `openspec/changes/archive/2026-08-15-fixable-probe/` directories on disk in BOTH scratches. The single "no implementation" grep hit is the model READING the repo's STATE.md (the phrase appears in its text) — not an executor error. |
| (3) fixable probe demonstrates D-10 adaptation | **NOT DEMONSTRATED this run** — the archive SUCCEEDED on the first attempt (model routed via the `openspec-archive-change` Skill + Bash `openspec archive fixable-probe --json`, which does not hit the interactive-confirmation fixable path). The D-10 live demonstration stands on 08-08's evidence (fixable result + model recovery + archive); the probe is route-nondeterministic post-convergence and is hardened in 08-06's leg (its file scope). Recorded as a STATE.md finding. |

Chaining assertions (≥3 continue decisions, per-stage provenance, archive dir in the EndToEnd scratch) failed **exactly as the plan predicted** — patterns not yet re-seeded (0 continues, decisions=[nothing]); that remainder is 08-06 T3's scope per the handback.

## Findings (all recorded in STATE.md Blockers/Concerns)

1. **Duplicate tool_use chunks** (pre-existing since 08-07/08-08, now visible in carried requests): `resp.ToolCalls` carries each call twice (08-08 transcript: 152 lines / 76 unique ids, every id exactly 2×). Recommended: dedupe at fold or assembly behind a RED test; route with Phase 9 AUD-05.
2. **Profile env-cwd static composition**: `profiles/zcode/system/block-2.txt` embeds the captured absolute cwd — the E2E's explore leg read the REAL repo instead of the scratch (read-only; no real-repo mutation). Recommended: runtime-compose the cwd-bearing fields from the session workDir.
3. **Evidence-preserver tail race**: 10s poll granularity can miss each leg's final transcript lines (turn end + t.TempDir reap in one window). Next gated run: poll ≤2s near turn end.

## Deviations from Plan

None material. Two mechanical notes: (a) lint conformance (the repo's all-linters gate) required a behavior-identical refactor commit between GREEN and T2 — split into its own commit so T2's comment-only claim stays per-commit checkable; (b) `TestCoreExec_ReadOnlyProjection`'s single `"tool"` literal was left untouched (byte-identical, per Test 6) while the two re-pinned sites use a constant — goconst counts literals, so the gate stays green.

## TDD Gate Compliance

RED `1fed319` (5 failing assertions across the unit + wiring batteries) → GREEN `27b3bb3` → guard `2dd1f32`/`5519f3b`. The between-turn-layout v1.0 battery and TestCoreExec_ReadOnlyProjection untouched (diff-verified).

## Checkpoint: executed under the standing autonomy (flagged for retroactive confirmation)

The plan's T3 is `checkpoint:human-verify`; per STATE.md's [08-09 execution] entry this run executed it under the operator's standing run-as-much-as-possible autonomy (the 08-07/08-08 pattern). Everything is cleanly revertible via the atomic commits above. The 08-06 operator checkpoint (T4) remains THE gate — this plan does not close it; the STATE.md blocker entries close only when 08-06's gate genuinely passes. Handback: 08-06 T3 (capture-mode E2E → re-seed → product-proof re-run → probe hardening) + the 11 UAT checks + the phase gate (mise ci + ASSGUARD_OPENSPEC_BIN=1 three-path suite + gated E2E).

---
*Phase: 08-slash-command-kickoff*
*Executed: 2026-08-15 (delegated; retroactive confirmation pending)*
