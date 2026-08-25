# Roadmap: ass-guard-agent (working name)

**Project mode:** mvp (vertical slices — each phase delivers an end-to-end user capability)

## Milestones

- ✅ **v1.0 MVP** — Phases 0–7 (shipped 2026-08-14; full detail: `.planning/milestones/v1.0-ROADMAP.md`, artifacts in `.planning/milestones/v1.0-phases/`)
- ✅ **v1.1 ACP Early Adoption** — Phases 8, 9, 12, 13, 14 (shipped 2026-08-25; full detail: `.planning/milestones/v1.1-ROADMAP.md`, artifacts in `.planning/milestones/v1.1-phases/`)

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

## Next Milestone: v1.2 (pool — not yet planned)

Staged pool (order TBD at planning):

1. **ACP completeness** — session/request_permission (clickable asks), elicitation/create, tool_call+plan update streaming, available_commands_update, session/list/resume/close/delete family (session resume is operator-must-have, D-09 reversal); plus **editor-driven configuration**: read Zed `settings` payload at initialize + advertise `configOptions` / handle `session/set_config_option` so tier/model defaults are switchable from the editor UI (operator ask 2026-08-25; api keys stay env/file, never editor settings)
2. **Telegram Peer** (ex-Phase 10, replan) — text + voice STT, shared turn core (`internal/runtime` extraction), go-telegram/bot dependency lands here
3. **dsh profile #2** (ex-Phase 11, replan vs source-analysis) — scope re-evaluated under the mimicry-pivot decision
4. **LSP support** — documented requirement for IDE-side MCP configuration (operator decision 2026-08-25: no agent-side LSP implementation)
5. SEED-001 agent-creation kit library; SEED-002 charmbracelet/fantasy mining; SEED-003 Go agent reference landscape; SEED-004 IDEA-LANDSCAPE borrow list (acknowledged dormant at v1.1 close)
6. Scheduler outcome store + feedback loop; nightly-parity CI automation tail

Deferred from v1.1 requirements: TG-01..06 (Telegram), DSH-01..05 (dsh profile).
