# Roadmap: ass-guard-agent (working name)

**Project mode:** mvp (vertical slices — each phase delivers an end-to-end user capability)

## Milestones

- ✅ **v1.0 MVP** — Phases 0–7 (shipped 2026-08-14; full detail: `.planning/milestones/v1.0-ROADMAP.md`, artifacts in `.planning/milestones/v1.0-phases/`)
- ✅ **v1.1 ACP Early Adoption** — Phases 8, 9, 12–14 (shipped 2026-08-25; full detail: `.planning/milestones/v1.1-ROADMAP.md`, artifacts in `.planning/milestones/v1.1-phases/`)
- 🚧 **v1.2 Claude Code Parity** — Phases 15–25 (in progress; this roadmap)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 0–7) — SHIPPED 2026-08-14</summary>

- [x] Phase 0: Spike + Re-verification (5/5 plans) — completed 2026-08-09
- [x] Phase 1: Mimicry MVP (north-star proof) (6/6 plans) — completed 2026-08-13
- [x] Phase 2: Session Core + ACP Interface (7/7 plans) — completed 2026-08-11
- [x] Phase 3: Model Scheduling (4/4 plans) — completed 2026-08-11
- [x] Phase 4: Unified Engine + Hook-DAG + OpenSpec + Learning (7/7 plans) — completed 2026-08-12
- [x] Phase 5: Ecosystem Compatibility (3/3 plans) — completed 2026-08-13
- [x] Phase 6: Distribution + Polish (2/2 plans) — completed 2026-08-13
- [x] Phase 7: Multi-Provider Config & Credentials (2/2 plans) — completed 2026-08-14

</details>

<details>
<summary>✅ v1.1 ACP Early Adoption (Phases 8, 9, 12–14) — SHIPPED 2026-08-25</summary>

- [x] Phase 8: Slash-Command Kickoff (9/9 plans) — completed 2026-08-16; zero-continue product proof
- [x] Phase 9: Serve-Path Audit + zcode Parity Re-capture (6/6 plans) — completed 2026-08-18
- [x] Phase 12: Product Functional Completeness (11/11 plans incl. gap-closure 12-09..12-11) — completed 2026-08-20; re-verified 2026-08-25 after the live UAT round found and fixed three real gaps
- [x] Phase 13: OpenSpec Workflow Completion (5/5 plans) — completed 2026-08-21
- [x] Phase 14: Adoption Readiness (Analysis Dispositions) (6/6 plans) — completed 2026-08-19

Post-adoption UAT reopen/close (2026-08-22…25): live stdio driving found dead plan-mode wiring, silent tool-result loss, ReadSessionContext id mismatch, TaskOutput catalog omission — all fixed via 12-09/12-10/12-11, UAT 11/11 pass.

Operator decisions at close (2026-08-23…25): D-09 REVERSED (session resume = must-have); "scheduling" renamed to model-routing (commit 732a8a3); **mimicry abandoned as the product bar in favor of client-native ACP surfaces** (Zed-provided fs/permission/session UX); LSP routed to IDE-side MCP configuration (documented requirement, not own implementation).

</details>

<details>
<summary>✅ v1.2 Claude Code Parity (Phases 15–25) — SHIPPED 2026-09-20</summary>

- [x] Phase 15: internal/runtime Carve (7/7 plans) — completed 2026-09-10
- [x] Phase 16: ACP Wire Foundation (9/9 plans) — completed 2026-09-01
- [x] Phase 17: Permissions + Elicitation (6/6 plans) — completed 2026-09-03
- [x] Phase 18: Session Family (7/7 plans) — completed 2026-09-06
- [x] Phase 19: Compaction + cache_control (7/7 plans) — completed 2026-09-10
- [x] Phase 20: Built-in Commands + Skills + Per-Agent Model (6/6 plans) — completed 2026-09-10 (human-UAT closed 2026-09-20, 7/7 wire-level)
- [x] Phase 21: Context & Policy Parity Closures (6/6 plans) — completed 2026-09-06
- [x] Phase 22: Background Execution + Sandbox Reality (9/9 plans) — completed 2026-09-11 (Darwin seatbelt leg deferred to a future macOS host)
- [x] Phase 23: SEED Gaps Close-out (7/7 plans) — completed 2026-09-20 (operator UAT 2/2)
- [x] Phase 24: Documentation & Ops Tails (6/6 plans) — completed 2026-09-11 (nightly-CI cron activation merge-gated to master)
- [x] Phase 25: SEED-001 Kit Extraction (9/9 plans) — completed 2026-09-11

</details>


## Backlog

### Phase 999.1: Study ai-agent-book, extract useful patterns (BACKLOG)

**Goal:** [Captured for future planning] Study https://github.com/bojieli/ai-agent-book/ (Li Bojie, 《深入理解 AI Agent：设计原理与工程实践》, ~45k stars, Apache-2.0, 10 chapters + 109 companion experiments, English translation in book-en/) and extract patterns useful for ass-guard-agent. See NOTES.md in the phase directory for chapter-to-surface mapping.
**Requirements:** TBD
**Plans:** 0 plans

Plans:

- [ ] TBD (promote with /gsd:review-backlog when ready)

