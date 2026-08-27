---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Claude Code Parity
current_phase: 16
current_phase_name: ACP Wire Foundation
status: executing
stopped_at: Completed 16-01-PLAN.md (TurnEmitter tracer)
last_updated: "2026-08-27T14:55:00.000Z"
last_activity: 2026-08-27
last_activity_desc: 16-01 executed — ordered TurnEmitter landed and proven
state_head: 11a86c3529f677ae22d160dae53ca78f362e3bbb
progress:
  total_phases: 11
  completed_phases: 1
  total_plans: 29
  completed_plans: 8
  percent: 28
---

# State: ass-guard-agent (working name)

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-27)
**Core value:** A hands-off coding agent that feels native in the editor: full SDD workflows run end-to-end without manual continues, and the agent surfaces through the client's own UX — clickable permission prompts, native file diffs, session management. (Pivoted 2026-08-25 from the mimicry bar.)
**Current focus:** Phase 16 — ACP Wire Foundation

## Current Position

Phase: 16 (ACP Wire Foundation) — EXECUTING
Current Plan: 2 (16-02 next — outbound id'd requests + registry)
Total Plans in Phase: 6
Status: Executing Phase 16
Last activity: 2026-08-27 — 16-01 executed (ordered TurnEmitter, 3 tasks, 5 commits)

Progress: [████████░░░░░░░░░░░] 8/29 plans (28%)

## Performance Metrics

**Velocity (v1.0 history, for calibration):** 36 plans / 8 phases in 6 days; `mise ci` gate green at every phase close. **v1.1:** 51 plans / 5 phases over ~7 active days.

**By Phase (v1.2):** Phase 16: 1/6 plans executed (16-01 ✓ 2026-08-27).
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 16 P01 | 59 min | 3 tasks | 9 files |
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
- [Phase 16]: 16-01 TurnEmitter armed by NewServer (knobs via WithTurnEmitter at the acp_serve junction) — one drain owns every session/update notification from the first frame; lanes fg 128 / bg 256, stall threshold 5s (CONTEXT-discretion defaults)
- [Phase 16]: 16-01 ActivityEmitter added via embedding (ChunkEmitter untouched — repo test fakes keep compiling; RESEARCH Open Question 1 resolution); runtime forwarders type-assert emit
- [Phase 16]: 16-01 stall sampler runs in its OWN goroutine — the drain wedges inside sink.Write on slow clients, so in-drain sampling would go silent exactly when a stall is real (anti-D-03)
- [Phase 16]: 16-01 TodoWrite→plan presentation rule lives in internal/acp (EmitterHandle.ToolCall); runtime.go gained zero plan/todo vocabulary (15-D-20)

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

Last session: 2026-08-27T14:55:00Z
Stopped at: Completed 16-01-PLAN.md — ready for 16-02
Resume file: None
