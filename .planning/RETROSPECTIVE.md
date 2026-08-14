# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

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

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v1.0 | 24 packages `-race` green | not tracked (gate = full-suite -race) | jsonrpc framing, hookdag executor, scheduler resolver, profile loader |

### Top Lessons (Verified Across Milestones)

1. (single milestone so far — see Key Lessons above; re-verify in v1.1)
