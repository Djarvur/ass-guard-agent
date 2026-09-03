---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Claude Code Parity
current_phase: 21
current_phase_name: Context & Policy Parity Closures
current_plan: 3
status: executing
stopped_at: Completed 21-02-PLAN.md
last_updated: "2026-09-03T18:52:51.673Z"
last_activity: 2026-09-03
last_activity_desc: Phase 21 execution started
state_head: 4cddcc34d046318848ff2147b0c98060b5ff8c21
progress:
  total_phases: 11
  completed_phases: 3
  total_plans: 70
  completed_plans: 30
  percent: 27
---

# State: ass-guard-agent (working name)

## Project Reference

See: .planning/PROJECT.md (updated 2026-09-03)
**Core value:** A hands-off coding agent that feels native in the editor: full SDD workflows run end-to-end without manual continues, and the agent surfaces through the client's own UX — clickable permission prompts, native file diffs, session management. (Pivoted 2026-08-25 from the mimicry bar.)
**Current focus:** Phase 21 — Context & Policy Parity Closures

## Current Position

Phase: 21 (Context & Policy Parity Closures) — EXECUTING
Current Plan: 3
Total Plans in Phase: 6
Status: Ready to execute
Last activity: 2026-09-03 — Phase 21 execution started

Progress: [██████░░░░░░░░░░░░░░░░] 22/70 plans ([███░░░░░░░] 27%)

## Performance Metrics

**Velocity (v1.0 history, for calibration):** 36 plans / 8 phases in 6 days; `mise ci` gate green at every phase close. **v1.1:** 51 plans / 5 phases over ~7 active days.

**By Phase (v1.2):** Phase 15: 7/7 ✓ closed 2026-08-27. Phase 16: 9/9 ✓ closed 2026-09-01 (UAT 4/4 passed — operator confirmed blob-tier override, cancel-keeps-session, per-turn hook concurrency; SECURITY verified threats_open: 0). Phase 17: 6/6 ✓ closed 2026-09-03 (UAT 4/4 after gap closure: permission round-trip + always-persistence both directions live-verified, native elicitation form + answer-landing; G-17-1 canonical-nested-outcome fix found by UAT and re-verified live; WINDOWS #15 operator-confirmed).
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
| Phase 16 P09 | 9 min | 2 tasks | 3 files |
| Phase 17 P01 | 36 min | 2 tasks | 4 files |
| Phase 17 P02 | 52 min | 3 tasks | 17 files |
| Phase 17 P03 | 48 min | 2 tasks | 12 files |
| Phase 17 P04 | 51 min | 3 tasks | 12 files |
| Phase 17 P05 | 29 min | 3 tasks | 2 files |
| Phase 17 P06 | 12 min | 2 tasks | 4 files |
| Phase 18 P01 | 48 min | 2 tasks | 18 files |
| Phase 18 P02 | 23 min | 2 tasks | 12 files |
| Phase 18 P03 | 19min | 2 tasks | 2 files |
| Phase 18 P04 | 64 min | 3 tasks | 15 files |
| Phase 18-05 P05 | 72min | 2 tasks | 12 files |
| Phase 18 P06 | 60 min | 3 tasks | 8 files |
| Phase 21 P01 | 46 min | 3 tasks | 7 files |
| Phase 21 P02 | 34 min | 2 tasks | 4 files |

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
- [Phase ?]: 16-09 CHIP-TRUTHFULNESS DECISION (implemented, Phases 18/20 build on it): the RUNNER's default turn model follows the tier resolution via Runner.defaultTurnModel (resolver → static binding → "", mirroring resolveModelLocked) — the profile slug is OUT of the precedence chain (mimicry-bar leftover) and only governs with nil schedCfg; explicit editor stamps keep absolute precedence (D-12); chip==wire pinned from both sides (TestDefaultTurnModel_FollowsTierResolution + TestConfigAdvertisement_ResolverTruth)
- [Phase 17]: Compound fail-safe semantics pinned in perm.RuleSet: unmatched subcommand dominates allows (deny=any/ask=any/allow=ALL subcommands) — an allow rule must cover every subcommand
- [Phase 17]: D-02 grammar leaves effect assignment to the owning list (NewRuleSet); ParseRule stays single-string with EffectNone placeholder
- [Phase 17]: perm.Store commits in-memory state only after a successful atomic save — failed writes leave file AND rule set unchanged; warning sink = returned []Warning diagnostics
- [Phase 17]: 17-03: the ask queue's fire seam is ctx-aware — the drain cancels an OPEN dialog through the queue-owned per-firing ctx (registry cascades $//cancel_request) with no id plumbing across the session/acp boundary — The only way D-13 can resolve a registry-backed dialog without internal/session importing internal/acp; nil falls back to the serve ctx
- [Phase 17]: 17-03: cancel-path drain scope is the whole session's queue (ACP cancel contract covers ALL pending permission requests); turn-scoped drain lives at the queue level (DrainTurn), DrainAll is its per-turn loop — one shared function for all three teardown paths — Matches the ACP cancelled-outcome normative text verbatim and keeps the scoping proof where the scoping lives
- [Phase 17]: 17-03: the enqueue that finds the queue idle promotes its entry to the open slot synchronously — D-12 note counts and Pending() are deterministic by construction (the async-pump-pop draft raced the count) — Notes are user-visible; a race-dependent pending count would be a visible lie
- [Phase 17]: 17-04: the structured-reply seam anchors byte-identity — a single string-valued accept field renders through RenderAskAnswered verbatim (the captured answered form survives the structured upgrade by construction); multi-field renders property-order title=value lines — The model view of an answered ask must not drift (Pitfall 6); the zcode golden enforces it
- [Phase 17]: 17-04: elicitation dispatch degrades fail-safe — every degraded/malformed case (probe-degraded, -32601, transport failure) lands on the plain-text fallback verbatim; only -32601 is sticky (16-D-18); unknown accept content keys are ignored (never execute nor render) — The ask must never dead-end and forged content must never execute (T-17-11/12)
- [Phase 17]: 17-04: engine asks are not reply-answerable — PlainTextFallback=false suppresses the dead-end question publish, the degraded surface is the advisory note; accept persists through the learning store existing RecordCandidate API (persistence unchanged, only the surface) — A9 answers-persist-as-today plus the D-07 advisory shape; a dead-end question nobody can answer serves no one
- [Phase 17]: The chokepoint contract is a doc+grep pair: docs/permissions-gate.md names gateCall verbatim and the no-second-gate property is a re-runnable region check (perm.RuleSet.Evaluate call sites in internal/session non-test sources appear in gate.go only)
- [Phase 17]: Whole-Run E2E batteries asserting on-disk effects need Options.EngineEnabled=true — the zero value wires the canned stub executor and every non-mcp call lands "stubbed (engine disabled)" without failing the turn
- [Phase 17]: 17-06 (G-17-1): PermissionOutcomeFrame is the canonical v1 NESTED outcome object — one wire field typed as the new inner PermissionOutcome struct (outcome discriminator + optionId); the old flat shape now fails the decode (errPermissionOutcomeBad) instead of being silently accepted — wire truth re-verified at the tool-calls example page + schema page during execution — Live Zed 1.18.0 answers the nested union; the flat type made every dialog answer fail-safe-decline. Hard-rejecting the flat dialect keeps nonconformance fail-safe by construction
- [Phase 17]: 17-06: elicitation response shape deliberately NOT nested — CreateElicitationResponse is canonical FLAT action + optional content (permAnswerAccept/ElicitationOutcomeFrame untouched, comment added citing the 17-06 schema verification) — One protocol, two union encodings; each decode site is pinned to its own schema def so nobody unifies them by mistake
- [Phase 18]: 18-01: D-03 gate = loading-set + ready-flag double gate — sessions-map insert stays load's LAST step; mid-load prompts typed-rejected via s.loading; session/new states born ready
- [Phase 18]: 18-01: replay error lines — toolCallID-carrying closes the call as terminal failed tool_call_update; bare renders as agent chunk 'component: message'
- [Phase 18]: 18-01: symlink guard strengthened to Lstat link-bit rejection + Stat regular-file check (Stat alone resolved symlink→regular cleanly — RED battery caught it)
- [Phase 18]: 18-01: idempotent re-load returns the v1 response without re-replaying; concurrent second load typed-busy; load rejections side-effect-free by ordering (checks precede ResumeSession)
- [Phase 18]: Reconciliation engine built transcript-pure (18-02): Reconcile classifies all ten kill -9 inventory rows with keyed pair matching, engine-authored closure payloads, and Seed{MaxTurns,PlanMode}; Manager.AppendSynthetic is the 18-05 append seam through the redaction-disciplined appendLine
- [Phase 18]: Tombstone spelling locked: <sessionID>.deleted sibling (matches 18-04 SweepTombstones suffix-with-validating-id); open failures on structurally valid transcripts propagate per SessionReader.read precedent
- [Phase 18]: HasCheckpoints probes checkpoint loose-ref layout via one ReadDir (never checkpoint.Open, which creates the store); transcript reads capped at 64 KiB total via io.LimitReader
- [Phase 18]: 18-06: one resolver (resolveResumeTarget in cmd), two entrypoints — root RunE delegates via the runServeWithResumeTarget seam, serve RunE resolves inherited cmd.Flags(); id-form passes without existence check (the load engine owns that error), names match titles with traversal pre-scan rejection (T-18-13)
- [Phase 18]: 18-06: CLI-contract goldens are regenerated FROM the binary's output (15-01 transcription rule) — root-persistent flags shift every help surface's alignment, so regeneration beat hand-editing
- [Phase 21]: 21-01: plugin-scope allow verdicts demote alongside project-scope (only user scope widens trust — one loud warning per demoted result)
- [Phase 21]: 21-01: exit-2 reason in the composed path is classifyHookRun's stderr-first extraction; parseHookVerdict direct calls fall back to capped stdout
- [Phase 21]: 21-01: scope partition (D-03 project-user-plugin) resolves at NewHookRunner construction via scopeRank — loader merge untouched; Verdict constants unexported until 21-06 exports by necessity
- [Phase 21]: PAR-04 memory budget is whole-file fits-or-skips (over-budget files skipped with per-file notes, never partially cut); per-file 24 KB cap at discovery, 64 KB budget at injection — Deterministic and observably loud; partial budget cuts would entangle note length with accounting
- [Phase 21]: ~/.ass-guard existence-gate: ANY present memory candidate there (even unreadable) blocks ~/.claude/CLAUDE.md (D-07 conservative reading); memory framing header is operator text pending a re-capture pin — D-07 win-on-conflict read conservatively; CC's native framing is not captured on this machine

### Pending Todos

None yet.

### Blockers/Concerns

- [RESEARCH FLAGS / planning-time]: Phases 16/18/19/22/23 flagged for `--research-phase` (ACP schema LOW-confidence details; resume reconciliation inventory; compaction × projector pins; platform drift; pi/strands steering references unverified). Full list in ROADMAP.md Research Flags section.
- [Phase 15 UAT, deferred 2026-08-27]: Zed history-resume shows "Failed to Launch" — by-design v1 scope (loadSession:false); fix owned by Phase 18 Session Family (18-01/18-04), already planned. Do not re-diagnose as a Phase 15/16 regression.
- [Phase 17 review, deferred 2026-09-03, operator non-blocking]: CR-04 — SetTurnOriginAutomation set before TryLock stays true while an automation turn queues/runs; overlapping foreground turns get ask-class calls D-07-declined instead of dialogs (fail-safe direction). Proper fix: per-turn origin. Evidence in 17-UAT.md Deferred Follow-Ups.
- [Phase 17 review, deferred 2026-09-03, operator non-blocking, security-adjacent]: hasSubstitution (a710b75 rewrite, internal/perm/rules.go) dropped double-quote tracking — `git "log 'x $(cmd) y'"` under-detects live substitution and an allow rule can match a substitution-bearing command (WR-02 violation). Verified in live shell. Deserves a fix ticket in the next phase touching internal/perm.
- [25-01 Task 3 precondition, 2026-08-28]: Phases 17-24 NOT executed (internal/perm, internal/tasks, internal/sandbox, internal/modesmatrix absent; only 15-16 landed) — 'strictly last' ordering violated, move inventory would be wrong. Executor halted BEFORE the rank-0 move; no kit/ paths created. Resolve by executing phases 17-24 first (then re-capture the test-ledger baseline) or by explicit operator override of the ROADMAP ordering.

## Deferred Items

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| peer | Telegram peer (TG-01..02) — full text+voice interface consuming the steering core | Deferred to v1.3 pool (lowest priority, operator) | 2026-08-26 | v1.3 |
| profile | dsh profile #2 (DSH-01..05) | Dropped entirely (operator — mimicry bar abandoned) | 2026-08-26 | none |

## Session Continuity

Last session: 2026-09-03T18:52:50.969Z
Stopped at: Completed 21-02-PLAN.md
Resume file: None
