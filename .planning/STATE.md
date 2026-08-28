---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Claude Code Parity
current_phase: 25
current_phase_name: seed-001-kit-extraction-strictly-last
current_plan: 1
status: executing
stopped_at: 16-08 complete (WR-05 scope-aware config surface, gaps 3+4a+5 closed; commits 0ddbf2e/8c7929f/e064df3/9006708); 25-01 still halted at Task 3 phase-ordering precondition
last_updated: "2026-08-28T13:01:50.073Z"
last_activity: 2026-08-28
last_activity_desc: Phase 25 execution started
progress:
  total_phases: 11
  completed_phases: 1
  total_plans: 69
  completed_plans: 15
  percent: 9
state_head: 9ba2baa14bf99d55b37abcf41ae713e522306276
---

# State: ass-guard-agent (working name)

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-27)
**Core value:** A hands-off coding agent that feels native in the editor: full SDD workflows run end-to-end without manual continues, and the agent surfaces through the client's own UX — clickable permission prompts, native file diffs, session management. (Pivoted 2026-08-25 from the mimicry bar.)
**Current focus:** Phase 25 — seed-001-kit-extraction-strictly-last

## Current Position

Phase: 25 (seed-001-kit-extraction-strictly-last) — EXECUTING
Current Plan: 1
Total Plans in Phase: 9
Status: Executing Phase 25
Last activity: 2026-08-28 — Phase 25 execution started

Progress: [████████░░░░░░░░░░░] 8/29 plans ([█░░░░░░░░░] 9%)

## Performance Metrics

**Velocity (v1.0 history, for calibration):** 36 plans / 8 phases in 6 days; `mise ci` gate green at every phase close. **v1.1:** 51 plans / 5 phases over ~7 active days.

**By Phase (v1.2):** Phase 16: 6/6 plans executed (16-06 ✓ 2026-08-27) — phase closes pending the operator live-Zed confirmation (WINDOWS #11).
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
| Phase 16 P02 | 16 min | 2 tasks | 3 files |
| Phase 16 P04 | 13 min | 2 tasks | 5 files |
| Phase 16 P03 | 38 min | 3 tasks | 10 files |
| Phase 16 P05 | 49 min | 3 tasks | 11 files |
| Phase 16 P06 | 49 min | 3 tasks | 4 files |
| Phase 16 P07 | 11 min | 2 tasks | 2 files |
| Phase 16 P08 | 12 min | 2 tasks | 2 files |

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
- [Phase 16]: 16-02 raw_thinking reuses Line Content+Model (payload json.RawMessage verbatim + provider attribution); compaction pointers opaque PreRef/PostRef strings until Phase 19 types them (D-20 weak schema)
- [Phase 16]: 16-02 appendLineUnredacted is a deliberate near-copy of appendLine minus the redact block — no shared helper (Pitfall 5); sole caller AppendRawThinking, grep-verified (T-16-04 type-scoped exemption)
- [Phase 16]: 16-02 local-command args verbatim string + ordered SourceChain + free-string Expansion (vocabularies owned by Phase 20); compaction boundary ids via local uuidV4 crypto/rand replication
- [Phase 16]: 16-04 modelrouting.DeepMerge exported (was unexported deepMerge) — the config writer reuses Load's overlay semantics verbatim so a written layer is inverse-compatible with the loader that reads it back; no forked merge — Round-trip fidelity is the D-07 write-half contract; duplicating the merge rules would drift silently
- [Phase 16]: 16-04 WriteLayerOption atomic sibling-temp+rename at hard 0600 (dirs 0750 max), no fsync — parity with the session append path convention; renameFunc unexported seam proves the crash-between-marshal-and-rename window — Typed LayerReadError/LayerWriteError give 16-05 distinct JSON-RPC error classes; every failure leaves the target byte-identical with no temp leftovers (T-16-10/11)
- [Phase 16]: 16-04 session_tier is parse/default/round-trip only — Validate does NOT cross-reference it; tier consumption and D-09 typed rejection land in 16-05's apply seam — Keeps the config package ACP-free and value whitelisting at the wire where the menu ids live (T-16-12)
- [Phase 16]: 16-05: set REQUEST field is configId (v1 SetSessionConfigOptionRequest) while the advertisement key is id — both verbatim from the fetched schema; plan prose's optionId normalized to wire truth — Pitfall-7 discipline: wire shapes come from the canonical schema, never plan prose
- [Phase 16]: 16-05: D-10 explicitness boundary drawn at the LAYER FILES — the _meta blob beats the embedded floor in-memory only, never persists, and an idempotent re-push of a blob-derived value neither churns the layer nor promotes it into persisted config — Files are operator config (D-10/D-12); the floor is not — a redundant Zed default re-push must not become explicit config
- [Phase 16]: 16-05: live apply rides Runner.ApplyTurnModel under the per-session turn mutex — mid-turn Sets land between turns; model writes go through tiers.<tier>.model, _global/ prefix addresses the global layer; cross-provider targets degrade loudly with model unchanged — Reuses 12-07 queue semantics as the no-torn-stamp gate and the resolveSubagentModel loud-degrade precedent
- [Phase 16]: 16-06: the simulator's fake provider enters through the REAL seam (temp project config aiming the factory-built provider at a scripted SSE stub) — Run's only injection point; zero production changes, the Run-over-pipes key link holds
- [Phase 16]: 16-06: SSE tool blocks flush at the NEXT block's stop (real transport behavior) — scripted turns put tool phases before text phases so the [tool_call, plan, chunk] emission story is deterministic
- [Phase 16]: 16-06: soak chaos mapping — per-producer mid-block ctx cancel is the unit contract (TestTurnEmitterCtxAbort); the soak's equivalent is abrupt producer exits + barrier-ctx cancels; flooders + long-stall tormentor make stall-detector-fired deterministic (2min run: 4,056,534 frames, 160 episodes)
- [Phase 16]: 16-06: operator live-Zed checkpoint surfaced and recorded PENDING-OPERATOR-CONFIRMATION (WINDOWS #11) — criteria 1/4 await the operator; ACP-03 + ACP-08 marked complete as the last declaring sibling
- [Phase ?]: [Phase 25 P01 Task 1, AUTO-SELECTED 2026-08-28]: D-02 one-way gate — option-a 'Proceed' (kit/ tree inside the single module per D-01/D-02/D-07) auto-selected under auto_advance; takes effect at the tracer commit. Tracer itself NOT yet executed — blocked by the phase-ordering precondition.
- [Phase ?]: 16-07: CR-01 fixed as verifier-named per-generation broadcast (writeOut closes+swaps wake under mu), not sync.Cond — drain stays the only writer/closer, single-writer total order and D-01 untouched
- [Phase ?]: 16-07: adjacency probe decided — concurrent Barrier waiters stay separate, each re-checks written independently; pinned by TestTurnEmitterBarrierConcurrentWaiters with never-cancelled ctxs (no Done escape)
- [Phase ?]: 16-08: WR-05 closed at one root cause — Set's idempotence basis is scope-aware (global scope compares against the global layer ALONE via globalOnlyResolvedLocked; project scope keeps the combined D-10 guard) and the _global twins resolve the global layer's own tier/model (no blobFills, embedded-floor fallback); gaps 3+4a+5 closed, 4b (chip truthfulness) remains for 16-09

### Pending Todos

None yet.

### Blockers/Concerns

- [RESEARCH FLAGS / planning-time]: Phases 16/18/19/22/23 flagged for `--research-phase` (ACP schema LOW-confidence details; resume reconciliation inventory; compaction × projector pins; platform drift; pi/strands steering references unverified). Full list in ROADMAP.md Research Flags section.
- [Phase 15 UAT, deferred 2026-08-27]: Zed history-resume shows "Failed to Launch" — by-design v1 scope (loadSession:false); fix owned by Phase 18 Session Family (18-01/18-04), already planned. Do not re-diagnose as a Phase 15/16 regression.
- [25-01 Task 3 precondition, 2026-08-28]: Phases 17-24 NOT executed (internal/perm, internal/tasks, internal/sandbox, internal/modesmatrix absent; only 15-16 landed) — 'strictly last' ordering violated, move inventory would be wrong. Executor halted BEFORE the rank-0 move; no kit/ paths created. Resolve by executing phases 17-24 first (then re-capture the test-ledger baseline) or by explicit operator override of the ROADMAP ordering.

## Deferred Items

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| peer | Telegram peer (TG-01..02) — full text+voice interface consuming the steering core | Deferred to v1.3 pool (lowest priority, operator) | 2026-08-26 | v1.3 |
| profile | dsh profile #2 (DSH-01..05) | Dropped entirely (operator — mimicry bar abandoned) | 2026-08-26 | none |

## Session Continuity

Last session: 2026-08-28T13:01:50.044Z
Stopped at: 16-08 complete (WR-05 scope-aware config surface, gaps 3+4a+5 closed; commits 0ddbf2e/8c7929f/e064df3/9006708); 25-01 still halted at Task 3 phase-ordering precondition
Resume file: None
