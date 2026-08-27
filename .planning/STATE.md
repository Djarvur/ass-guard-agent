---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Claude Code Parity
current_phase: 16
current_phase_name: ACP Wire Foundation
status: planning
stopped_at: Phase 15 complete, ready to plan Phase 16
last_updated: "2026-08-27T13:26:48.516Z"
last_activity: 2026-08-27
last_activity_desc: Phase 15 complete, transitioned to Phase 16
state_head: 6a5fe68f1661a1f4efcbedacaaed65b5a394d285
progress:
  total_phases: 11
  completed_phases: 1
  total_plans: 29
  completed_plans: 7
  percent: 9
---

# State: ass-guard-agent (working name)

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-27)
**Core value:** A hands-off coding agent that feels native in the editor: full SDD workflows run end-to-end without manual continues, and the agent surfaces through the client's own UX — clickable permission prompts, native file diffs, session management. (Pivoted 2026-08-25 from the mimicry bar.)
**Current focus:** Phase 16 — ACP Wire Foundation

## Current Position

Phase: 16 — ACP Wire Foundation
Total Plans in Phase: 6
Status: Ready to plan
Last activity: 2026-08-27 — Phase 15 complete, transitioned to Phase 16

Progress: [█████░░░░░░░░░░░░░░] 7/29 plans (24%)

## Performance Metrics

**Velocity (v1.0 history, for calibration):** 36 plans / 8 phases in 6 days; `mise ci` gate green at every phase close. **v1.1:** 51 plans / 5 phases over ~7 active days.

**By Phase (v1.2):** no plans executed yet.
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 15 P01 | 13 min | 2 tasks | 2 files |
| Phase 15 P02 | 16 min | 2 tasks | 8 files |
| Phase 15 P03 | 14 min | 2 tasks | 10 files |
| Phase 15 P04 | 18 min | 2 tasks | 14 files |
| Phase 15 P05 | 22 min | 2 tasks | 6 files |
| Phase 15 P06 | 75 min | 3 tasks | 20 files |
| Phase 15 P07 | 20 min | 2 tasks | 2 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table. Decisions shaping the v1.2 roadmap:

- **[ROADMAP STRUCTURE, 2026-08-26]:** 11 phases derived from research/SUMMARY.md's 10-cluster suggestion plus a small tails phase — carve (15) → wire primitives (16) → interactive asks (17) → sessions (18) → compaction (19) → commands/skills/per-agent-model (20) → content/policy closures (21) → background+sandbox (22) → SEED gaps (23) → tails (24) → kit extraction (25, strictly last). Numbering continues from v1.1's Phase 14. ACP-04 (available_commands_update) maps to Phase 20 with the commands it advertises; ACP-03 + ACP-08 map to Phase 16 as wire primitives.
- **[MILESTONE SCOPE, operator 2026-08-26]:** Telegram peer deferred to the v1.3 pool (LOWEST priority); steering queue SEEDG-01 is built transport-neutral in Phase 23 as its prerequisite — do not descope to queue-behind silently. dsh profile #2 dropped entirely (mimicry bar abandoned 2026-08-25). Safety-model amendment: permission tier AVAILABLE but NOT default (`permissions.mode` default ungated).
- **[GATE PIPELINE LOCK, Phase 17]:** ONE permission/gate pipeline with documented precedence (hook verdict → permission ask → execute) locks in Phase 17; Phase 21's hooks join it rather than bolting a second gate. Hooks carry deny-only authority from project scope (repo-shipped files never grant allow).
- [Phase 15]: provider-factory extracted to internal/providerfactory behind 5-wrapper cmd bridge; dead unparam directive dropped (exported funcs skipped by unparam)
- [Phase 15]: CLI-support split: cobra shells stay in cmd, run-logic verbatim to internal/{checkpointcmd,learningcmd,modelroutingcmd}; tests follow subject (modelrouting cobra-tree tests stay in cmd)
- [Phase 15]: providerfactory wrapper bridge deleted at 15-04: all five consumers qualified (acp_serve :339/:349/:351/:354 + 4 test files); parity seams stay unexported vars (same-package test injection)
- [Phase 15]: acpserve extracted with PrepareServe/FinishServe callback-hook seam (4 hooks incl. CloseSessions); stdout tests moved with stub runner; 3 runner-subject tests stay in cmd until 15-06
- [Phase 15]: 15-06: exported sextet (quartet + SetupEngine/LoadCommandRegistry) — D-19 export-by-necessity for the acpserve.Run composition
- [Phase 15]: 15-06: pointer configs for NewRunner/NewEngineTurnAdapter/NewACPDispatcher (hugeParam, 15-05 precedent)
- [Phase 15]: 15-07 live-Zed criterion closed by operator UAT 2026-08-27: parity with v1.1 PROVEN (internal/acp/ byte-identical; loadSession:false + session/load -32601 is v1.1's shipped behavior — Zed aborts client-side); streaming/tool-diff legs operator-confirmed. Resume UX gap deferred to Phase 18 (18-01/18-04); tracked in UAT Deferred Follow-Ups

### Pending Todos

None yet.

### Blockers/Concerns

- [RESEARCH FLAGS / planning-time]: Phases 16/18/19/22/23 flagged for `--research-phase` (ACP schema LOW-confidence details; resume reconciliation inventory; compaction × projector pins; platform drift; pi/strands steering references unverified). Full list in ROADMAP.md Research Flags section.
- [Phase 15 UAT, deferred 2026-08-27]: Zed history-resume shows "Failed to Launch" — by-design v1 scope (loadSession:false); fix owned by Phase 18 Session Family (18-01/18-04), already planned. Do not re-diagnose as a Phase 15/16 regression.

## Deferred Items

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| peer | Telegram peer (TG-01..02) — full text+voice interface consuming the steering core | Deferred to v1.3 pool (lowest priority, operator) | 2026-08-26 | v1.3 |
| profile | dsh profile #2 (DSH-01..05) | Dropped entirely (operator — mimicry bar abandoned) | 2026-08-26 | none |

## Session Continuity

Last session: 2026-08-27T13:27:00Z
Stopped at: Phase 15 complete (verified + transitioned), Phase 16 ready to execute
Resume file: None
