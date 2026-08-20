# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.1 — ACP Early Adoption

**Complete:** 2026-08-21 (adoption line crossed 2026-08-20 — the operator began daily ACP use, the milestone's completion bar; the post-adoption continuation finished in use)
**Phases:** 5 (8, 9, 12, 14, 13 — Phases 10/11 re-scoped to the v1.2 pool mid-flight) | **Plans:** 34 | **Timeline:** 8 days (2026-08-14 → 2026-08-21), 343 commits (`v1.0` tag → `506b7c6`)
**Shape:** re-scoped twice mid-flight — 2026-08-16 (Telegram + dsh → v1.2 pool; Phase 12 added, split same day into 12 machinery + 13 OpenSpec halves) and 2026-08-18 (adoption re-order: Phase 14 created, Phase 12's waves split around an explicit ADOPTION LINE; milestone renamed 4×: Kickoff & Peers → ACP Completion → Product Completion → ACP Early Adoption)

### What Was Built
- **Phase 8 (2026-08-16, witness-accepted):** the zero-continue product proof — `/opsx:explore → propose → apply → archive` chained by the engine against the real openspec binary via command-provenance chaining (`StartedBy` + `CommandMatcher` + seeded rows; provenance unspoofable from model-visible content). Reached through three stacked gap-closure plans: within-turn tool-result carry (08-07), capture-grounded core tool execution (08-08), and the between-turn boundary re-scope that made real agentic turns convergent (08-09 — the D-11 mid-turn reset was itself a mimicry divergence, proven from the corpus: 46/46 tails = rolling 64-window, zero tool-result resets).
- **Phase 9 (2026-08-18):** redacted, bounded, decision-explaining audit on `acp serve` (factory capturer seam, body_ref hash-store volume bound, `engine_decision` lines with matched-span provenance, `.ass-guard/audit/` mirror) + the zcode parity re-capture with the driver kit archived in-repo; the stale curated-suite expectations re-baselined by control run (fresh numbers = the recorded reference, no silent threshold movement).
- **Phase 12 (2026-08-20, 8/9 + disposition):** every catalog tool executes for real — AskUserQuestion suspension/resume (live operator witness over a real ACP client), plan mode resolved BY CAPTURE as the target's own runtime mutating-tool gate, SendMessage/ReadSessionContext, the cron quartet with engine-driven firing (no daemon, no port), the background TaskRegistry + TaskStop, Bash background flags in captured forms, corpus-absent forms re-pinned via a live zcode 0.16.3 re-record (committed fixture), Claude-Code plugin installs consumed native (all five contribution kinds, live-proven on the operator's cache), the behavioral-eval net, and the permanent catalog-completeness gate — zero `no implementation yet` dead ends.
- **Phase 14 (2026-08-19, 39/39):** the adoption backstops — shadow-git checkpoints/undo (live rollback E2E: byte-identical restore, user `.git` untouched), the compaction decision from corpus evidence (verify-first: no auto-compact, no eviction; rolling 64-window + `cache_control` on every system block; the one emission gap routed post-adoption), the cache probe + installed-zcode drift warning in the parity run, the pi↔shaper cross-validation audit (shaper already agrees with the captured wire form), token economics (light-tier subagent routing + 128 KiB tool-output truncation), the uniform tool contract (timeouts/is_error/retry- transient/flags).
- **Phase 13 (2026-08-21, 3/3):** the full OpenSpec command matrix hands-off — the engine-visible ask resume (13-00), the 12-leg matrix E2E (`new/continue/ff/verify/bulk-archive/onboard` × happy/fixable) against the real binary with 12 committed captures, harvest-derived chaining rows (verify→continue, new→continue; termini dispositioned in-table), the unmatched-ending advisory (question-shaped endings surface, never silently stall), and 12 per-command eval suites green k=1.

### The Product Proof
The unmodified OpenSpec toolkit runs hands-off end-to-end: the flagship chain green at k=1 THROUGH real mid-chain model asks (flagship green ×3: eval-20260820-161520 / -163220 / -205550-k1.json — the third manager-certified at 173.51s), the 6-command matrix on 12 E2E legs vs the real binary + real model, 12 per-command eval suites (full batch 12/12 pass@1, 1450s), and catalog completeness as a permanent regression test. Verified milestone gates: `mise ci` green (28/28 packages, race battery) + eval-gate flagship green k=1.

### What Worked
- **The adoption line held.** The 2026-08-18 re-order made "the operator begins daily ACP use" the completion bar and split everything into P1 (pre-adoption minimal set: 12-01/02 → 14 → 12-05/04/06), P2 (readiness items), P3 (post-adoption queue: 12-03/08/07, 13). The 2026-08-19 operator priority ruling confirmed the structure ("no re-order needed — already encoded exactly"). Everything above the line landed before daily use began; the post-adoption queue finished while the agent was in use.
- **Capture as the argument-ender.** 12-04's "model self-restraint" premise for plan mode was disproved by the target's own captured refusals — resolved by mirroring the runtime gate, not by debate. 12-05's re-record replaced rotated-off ground truth with live-captured forms.
- **The eval net's first live failures WERE the net working** (see Lessons 3) — and the disposition routed the fix without touching a single assertion.
- **13-00's discipline:** a mid-flight architectural fix (engine-visible ask resume) went through the full plan→checker→execute cycle — the checker caught a self-defeating verify gate pre-execution (the original secluded-packages zero-diff would have failed on the evalharness draft it legitimately touched; split into zero-diff evalsuite + hash-baseline-pinned evalharness), and five safety invariants were pinned by named tests before any code landed.
- **Verifier independence:** 13-VERIFICATION re-ran both zero-continue chain legs itself (57.3s / 162.1s green) and read — not trusted — every gate artifact and certification. The false self-check (below) is exactly why.
- **Zero new dependencies held** across the milestone (go.mod/go.sum untouched in Phases 12–13, git-diff-proven) despite two scope expansions.
- **Velocity through background executor dispatches:** Phase 14's six plans in one day (2026-08-19, 169 min recorded total); 2026-08-20 alone carried 11 plans across two dispatches (Phase 12's post-adoption six: 724 min recorded; Phase 13's five: 970 min recorded).

### What Was Inefficient
- **Two background executors died of transcript bloat ("Invalid string length") AFTER completing their missions** — only their final reports were lost. The plans' commits, STATE updates, and summaries were all on disk; the manager reconstructed the reports from the artifacts.
- **One executor's "mise ci green" self-check was false** — a timing-flaky test failed under full-battery load while the piped exit code said green. Caught by the manager's independent certification run, not by the executor.
- **Bookkeeping staleness recurred** (v1.0 Lesson 3, unlearned): REQUIREMENTS status cells and ROADMAP checkboxes lagged verified work at both the Phase-12 and Phase-13 closes; the WINDOWS #8/#9 discharge was recorded in STATE but the ledger rows were never flipped — both verifiers flagged it as the milestone's friction point.
- **The ask-suspension × chaining defect surfaced only at the net's first live runs (12-08, post-adoption).** 2 of 3 flagship gate iterations died on mid-chain model questions. An earlier live firing of the net against real asks might have caught it pre-adoption — the net was deliberately scheduled last to pin settled behavior, which also meant the interaction stayed latent until then.
- **The 13-01 telemetry incident:** an executor overwrote the global openspec config without a byte-copy (a hash was taken — hashes are not reversible); the anonymousId is irrecoverable, the recovery command is recorded in STATE. The guard primitive existed and was bypassed; the process fix (guard-before-probe, byte-exact save/restore) is now pinned in code.
- **Scope churn cost re-checking:** the milestone was renamed 4×; 12-02's operator scope widening outgrew its checker-passed shape and needed a re-check at dispatch time.

### Patterns Established
- The **adoption line** — a milestone whose completion bar is the user starting daily use, with work explicitly partitioned pre-line/readiness/post-line.
- **Standing autonomy + retroactive flags with recorded overturn routes** — three architectural/product rulings executed under it (adoption re-order structure, eval-net disposition, route-1 ask resume), all later ratified (2026-08-19 confirmation; 2026-08-21 for the two standing flags).
- **Wave-0 gap-closure plans**: mid-flight defects get a plan→checker→execute cycle with invariant pins (13-00), not an in-task hotfix.
- **Secluded-packages discipline**: a fix plan must not touch its own judge — zero-diff or hash-baseline pins on the eval suites its gate runs through.
- **Guard-before-probe** on operator-global state; evidence dirs preserved in-repo or archived before /tmp reaping.
- **D-06 flake policy**: single zero-delta re-run allowed, both logs preserved, never a forced green; timing-sensitive pins poll load-immune signals (WaitChainIdle), not wall-clock deadlines.

### Key Lessons
1. **Cap background-executor dispatches at ~3 plans per agent.** Transcript bloat killed two executors after mission completion — and the crashes were recoverable ONLY because the work discipline (atomic commits, STATE updates per plan, summaries on disk) never depended on the executor surviving. Size the dispatch for the report, not the mission.
2. **Never trust a piped self-check on timing-sensitive suites.** An executor's "mise ci green" was false under full-battery load. Independent gate certification by the manager and full-battery runs are the only counts for timing-sensitive suites; a piped exit code can lie.
3. **A regression net whose first live runs go red on real drift is a WORKING net.** The eval net's opening failures caught genuine pattern-table drift (3 runs, 3 chain shapes; safety pins held everywhere). The disposition routed the fix (re-tuning, seeded.toml-only) rather than weakening assertions — assertions never moved all milestone (Pitfall 18 held).
4. **Standing autonomy + retroactive flags + overturn routes recorded at ruling time kept every architectural call reversible** — and all three rulings were eventually ratified. The flag pattern ("FLAGGED for retroactive operator confirmation — CONFIRMED by the operator <date>") makes autonomy auditable without stopping work.
5. **An uncommitted draft carried across dispatches is a pattern with guardrails, not a recommendation.** It worked once (13-01 Task 2's matrix bootstrap) only because 13-00's checker pinned hash baselines of the draft state — without that, the secluded-packages verify gate could not have distinguished legitimate draft evolution from gate tampering.

### Cost Observations
- Recorded per-plan durations (STATE.md table): 2,086 min across the 19 Phase-12/13/14 plans — Phase 14: 169 min (6 plans, one day); Phase 12: 947 min (8 plans); Phase 13: 970 min (5 plans, incl. 13-00's 255-min gap closure).
- Commit density peaked at adoption: 73 commits on 2026-08-19 (Phase 14 + replan), 82 on 2026-08-20 (11 plans + the adoption line).
- All E2E/eval/chain legs ran the real model (GLM via the operator's credentials) and the real openspec 1.5.0 binary; flagship gate legs cost 173s–782s each, the full matrix batch 1450s.
- Sessions: single continuous arc, 2026-08-14 → 2026-08-21, with the adoption line inside it — the milestone's second half executed while the agent was already in daily use.

---

## Milestone: v1.0 — MVP

**Shipped:** 2026-08-14
**Phases:** 8 | **Plans:** 36 | **Timeline:** 6 days (2026-08-09 → 2026-08-14), 210 commits

### What Was Built
- Mimicry core: zcode profile from real rollout logs (3 system blocks, 103 tools, identity), A/B parity thesis proven
- ACP v1 session core (Zed-native stdio JSON-RPC, streaming, replay, two-layer context) + parallel subagents
- Model scheduling: tiers, time-windowed substitution, per-project override, fallback chains, circuit breakers, cost ceiling
- Unified engine + hook-DAG (≤300 LOC hand-rolled) + learning store + OpenSpec hosting (subprocess adapter)
- Ecosystem: MCP subprocess hosting (zombie-proof lifecycle), `.claude/` discovery read-only; Distribution: goreleaser 4-target static binary, ACP registry manifest, zero-config first-run seed
- Multi-provider config & credentials: two-shape ProviderFactory, flag>env>config credential precedence, live-proven zero-env editor-spawned turn

### What Worked
- **Phase 0 re-verification spike paid for itself** — caught a wrong JSONL path, a missing ACP `session/new` lifecycle step, and confirmed library-level stdout discipline before any load-bearing code existed. Cheap insurance against 5-week-old inherited "facts".
- **The mise ci phase gate** (vet + all-linters + CGO build + `-race`) held every phase to zero issues — no lint debt accumulated across 33.5k LOC.
- **Serialized deltas** (mimicry → scheduling → engine → ecosystem → distribution) kept risk multiplication out; each phase built on a verified foundation.
- **TDD with recorded evidence** (VERIFICATION-PREP mapping success criteria → exact test commands) made milestone acceptance mostly mechanical.
- Honest attribution in the decision log (e.g. "option-a applied by default, NOT user chose") kept the audit trail trustworthy.

### What Was Inefficient
- **Stub-binary E2E hid a model mismatch**: Phase 4's OpenSpec adapter was proven against a stub, so the real toolkit's command-file-driven workflow (and the seeded command set mismatching openspec v1.5.0's surface) was only caught at post-ship UAT. The operator-gated real-binary test existed but was never run.
- **`internal/ecosys` shipped unwired**: discovery was implemented and tested in isolation, but nothing imported it — an integration gap invisible to per-package gates.
- **Traceability table went stale**: 63 of 71 requirement statuses were never updated as phases closed; milestone close required reconciliation against STATE.md evidence.
- **SUMMARY.md files degenerated to stubs** mid-milestone, weakening the auto-extracted milestone record (manual rewrite needed).

### Patterns Established
- Hand-rolled JSON-RPC framing, hook-DAG executor, and scheduler resolver — zero-dep internal packages where libraries would have added maintenance risk (per STACK.md discipline)
- Tier-A/Tier-B evidence classification for research corrections (spec-verified vs revise-and-continue)
- Verbatim operator quotes recorded in decision entries; deferred items tracked with explicit disposition

### Key Lessons
1. **A gated-but-never-run integration test is a false signal.** If the real-dependency gate exists (ASSGUARD_OPENSPEC_BIN=1), running it belongs in the phase's definition of done — otherwise the stub's assumptions ship.
2. **Per-package green ≠ integrated.** Discovery without an importer passed every gate. Cross-package wiring needs an explicit acceptance check ("grep for the importer" or an E2E that exercises the seam).
3. **Bookkeeping that isn't automated goes stale.** Traceability status updates should be part of phase transition, not deferred to milestone close.
4. **Post-ship UAT with the operator is where model-of-reality errors surface.** The openspec kickoff correction came from the user describing their actual workflow, not from any test.

### Cost Observations
- Model mix: predominantly opus-tier planning with haiku-tier checking (per GSD defaults); execution largely autonomous (background agents, sequential per worktree-disabled config)
- Sessions: single continuous milestone arc, 2026-08-09 → 2026-08-14
- Notable: 6 days from empty repo to shipped, gated milestone — the spec-driven discipline (phases 0–7, 36 plans, every phase mise-ci green) kept rework near zero except the UAT-surfaced integration gap

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.0 | continuous arc | 8 | Baseline established: Phase-0 verification spike + serialized deltas + mise ci gate |
| v1.1 | continuous arc w/ adoption line | 5 | Mid-milestone re-scoping as routine (2 re-orders, 4 renames); explicit adoption-line partitioning (P1/P2/P3); standing autonomy + retroactive flags; background-executor dispatch batches |

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v1.0 | 24 packages `-race` green | not tracked (gate = full-suite -race) | jsonrpc framing, hookdag executor, scheduler resolver, profile loader |
| v1.1 | 28 packages `-race` green (mise ci, certified at close) | not tracked (gate = full-suite -race) | none — zero new dependencies held across the milestone (git-diff-proven) |

### Top Lessons (Verified Across Milestones)

1. **Bookkeeping that isn't automated goes stale** (v1.0 L3) — **re-verified in v1.1**: REQUIREMENTS/ROADMAP/WINDOWS rows lagged verified work at both the Phase-12 and Phase-13 closes; both verifiers flagged it. Still unsolved mechanically.
2. **A gated-but-never-run integration test is a false signal** (v1.0 L1) — **countermeasure held in v1.1**: every external surface carried a live gate that actually ran (operator ask-witness, live rollback E2E, verifier-re-run chain legs, live model in every eval leg).
3. **Per-package green ≠ integrated** (v1.0 L2) — **countermeasure held in v1.1**: wiring/completeness checks became permanent regression tests (catalog completeness, key-link verification in every phase report).
4. **Dispatch size and self-check trust** (v1.1 L1/L2, new) — re-verify in v1.2: ~3-plan dispatch caps and manager-certified gates are policy now, not tooling; observe whether they hold under v1.2's Telegram/runtime-extraction work.
