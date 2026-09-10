---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: Claude Code Parity
current_phase: 24
current_phase_name: Documentation & Ops Tails
current_plan: 3
status: executing
stopped_at: "Completed 24-03-PLAN.md (DOC-01: LSP guide + executed dry-run)"
last_updated: "2026-09-10T14:54:47.876Z"
last_activity: 2026-09-10
last_activity_desc: Phase 24 execution started
state_head: 1388c28cc5605eade0c768407325ba3f08e281cb
progress:
  total_phases: 11
  completed_phases: 7
  total_plans: 77
  completed_plans: 65
  percent: 64
---

# State: ass-guard-agent (working name)

## Project Reference

See: .planning/PROJECT.md (updated 2026-09-10)
**Core value:** A hands-off coding agent that feels native in the editor: full SDD workflows run end-to-end without manual continues, and the agent surfaces through the client's own UX — clickable permission prompts, native file diffs, session management. (Pivoted 2026-08-25 from the mimicry bar.)
**Current focus:** Phase 24 — Documentation & Ops Tails

## Current Position

Phase: 24 (Documentation & Ops Tails) — EXECUTING
Plan: 3 of 5
Current Plan: 3
Total Plans in Phase: 5
Status: Ready to execute
Last activity: 2026-09-10 — Phase 24 execution started

Progress: [███████████████░░░░░░] 55/73 plans ([██████░░░░] 64%)

## Performance Metrics

**Velocity (v1.0 history, for calibration):** 36 plans / 8 phases in 6 days; `mise ci` gate green at every phase close. **v1.1:** 51 plans / 5 phases over ~7 active days.

**By Phase (v1.2):** Phase 15: 7/7 ✓ closed 2026-08-27. Phase 16: 9/9 ✓ closed 2026-09-01 (UAT 4/4 passed — operator confirmed blob-tier override, cancel-keeps-session, per-turn hook concurrency; SECURITY verified threats_open: 0). Phase 17: 6/6 ✓ closed 2026-09-03 (UAT 4/4 after gap closure: permission round-trip + always-persistence both directions live-verified, native elicitation form + answer-landing; G-17-1 canonical-nested-outcome fix found by UAT and re-verified live; WINDOWS #15 operator-confirmed). Phase 18: 7/7 ✓ closed 2026-09-06 (UAT 3/3 after gap closure G-18-1: real sessions write the session_start opener + tolerant legacy listing; Zed picker/resume/stop-mid-turn/delete operator-confirmed; TTY picker four legs re-run green on the fresh binary; cwd-scoped --resume deviation operator-accepted). Phase 21: 6/6 ✓ closed 2026-09-06 (UAT 4/4: AGENTS.md auto-inject + mtime pickup, thinking rendering, @-mention/image round-trip with D-11 loud model-side outcome, hook-deny live demo on the corrected dialect after an inconclusive first attempt — fail-open on schema-invalid hook JSON is by design).
Phase 19: 7/7 ✓ closed 2026-09-10 (verification completed by the auto-UAT live harness — spikes/19-compaction-live-uat, PASS 20/20: threshold compactions 1st+2nd, overflow retry post-marker 515KB→95KB per CR-01, restore-80 persisted; G-19-1 closed via 19-06/19-07, G-19-2 closed by retest; SECURITY threats_open: 0).

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
| Phase 21 P03 | 55 min | 3 tasks | 19 files |
| Phase 21 P04 | 25 min | 2 tasks | 6 files |
| 21-context-policy-parity-closures/05 | 109 min | 2 tasks | 38 files |
| Phase 21 P21-06 | 20 min | 2 tasks | 13 files |
| Phase 18 P07 | 21 min | 2 tasks | 4 files |
| Phase 19 P01 | 21 min | 3 tasks | 14 files |
| Phase 19 P02 | 3 min | 2 tasks | 4 files |
| Phase 19 P03 | 47 min | 3 tasks | 6 files |
| Phase 19 P04 | 37 min | 3 tasks | 3 files |
| Phase 19 P04 | 37 min | 3 tasks | 3 files |
| Phase 19 P05 | 36 min | 2 tasks | 12 files |
| Phase 19 P19-06 | 23 min | 3 tasks | 7 files |
| Phase 19 P07 | 6 min | 3 tasks | 4 files |
| Phase 20 P01 | 65 min | 3 tasks | 19 files |
| Phase 20 P02 | 118 min | 3 tasks | 6 files |
| Phase 20 P03 | 74 min | 3 tasks | 7 files |
| Phase 20 P04 | 62 min | 3 tasks | 6 files |
| Phase 20 P05 | 88 min | 3 tasks | 8 files |
| Phase 20 P06 | 46 min | 3 tasks | 1 files |
| Phase 23 P01 | 38 min | 3 tasks | 7 files |
| Phase 23 P03 | 42 min | 3 tasks | 2 files |
| Phase 23 P02 | 55 min | 3 tasks | 7 files |
| Phase 22 P04 | 35 min | 3 tasks | 15 files |
| Phase 22 P06 | 32 min | 3 tasks | 18 files |
| Phase 22 P07 | 17 min | 2 tasks | 4 files |
| Phase 22 P08 | 25 min | 2 tasks | 4 files |
| Phase 22 P09 | 22 min | 2 tasks | 8 files |
| Phase 23 P05 | 34 min | 3 tasks | 3 files |
| Phase 23 P06 | 18 min | 2 tasks | 3 files |
| Phase 24 P01 | 37 min | 3 tasks | 10 files |
| Phase 24 P03 | 26 min | 2 tasks | 2 files |

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
- [Phase 21]: PAR-05 thinking identity contract is FIELD-VALUE identity, not envelope identity: delta strings assembled once at the provider, RawMessage stored verbatim, values extracted only at the projector, SDK re-serializes (21-03)
- [Phase 21]: Redacted thinking blocks append to the transcript unconditionally but publish no agent_thought_chunk (no display text exists); thinking folds into its turn's assistant unit and drops with it when the unit never forms (21-03, Pitfall 5)
- [Phase 21]: Absolute @paths are admitted ONLY by an explicit evaluator ruling (nil resolves nothing outside the workspace root) — ingress can never serve as a whole-FS existence oracle; the 21-06 join wires internal/perm's rule set into the readRuleEvaluator seam
- [Phase 21]: Mention provenance is per attempt, not per success: forms file|dir|denied|unresolved record every @token's outcome, making the Read-rule gate observable in the transcript
- [Phase 21]: 21-05 D-11 seam: the turn path consults Provider.SupportsImages and strips+notes before ingress; the shaper stays pure
- [Phase 21]: 21-05 pixel-bomb law is two-tier: provider limits trigger D-09 downscale; a 100 Mpx decode-safety ceiling refuses at the config stage
- [Phase 21]: The 21-06 hook-gate join maps the four-valued PreToolUse verdict TOTALLY at gateCall's head (ask suspends even ungated, D-04); the executor consultation leg is deleted, not stubbed — one gate, one consultation site, one result form per decision
- [Phase 21]: Precedence hook verdict -> permission ask -> execute is documented AT the chokepoint (gateCall doc) and enforced by grep audits: zero executor consultation sites, the gate head the sole PreToolUseVerdict consumer
- [Phase 18]: 18-07 (G-18-1): two-sided opener fix — sessionFor writes AppendSessionStart behind a size-0 gate (creation-only by append-only discipline; the same gate IS resume-safety and kill-9 self-heal), and listing accepts any knownOpenerType first line with a valid timestamp as the legacy fallback (bounded whitelist; unknown/zero-timestamp/corrupt still skip); legacy title consequence documented in code, not fixed
- [Phase 19]: 19-01: cache_control storage is a profile-level presence flag (system_cache_control in profile.yaml) applied to every block at Load — the corpus value is 910/910 uniform so a bool carries full fidelity; no per-block sidecar channel exists — profile TextBlock carries the captured flag; the loader fans it out; the shaper emits — flag-off output provably byte-identical
- [Phase 19]: 19-01: over the Anthropic 4-breakpoint cap the shaper keeps the LAST four flagged system blocks (deepest cache prefixes) — degrade pinned 3/4/5/6; the parity probe compares placement classes so it stays green under either policy — a 5th breakpoint would 400 every request once dynamic merges stack system blocks; keep-last-4 is the corpus-faithful accommodation
- [Phase 19]: 19-01: the extractor derives the declaration by reusing ScanContextBehavior's system: placement class and extract-profile writes the yaml key — re-captures cannot lose the emission; probe baseline stayed byte-stable (D-12) — one detection implementation grounds both the decision census and the extraction; WINDOWS #5 closed (probe flip green on the committed pin fixture)
- [Phase ?]: 19-02: non-2xx stream rejections surface as error chunks — bounded 8 KiB envelope read + close at the status-check site, ClassifyHTTP(anthropic, prof.Model, status, cause); malformed/empty bodies degrade to the generic structural error; drainSSE never sees a rejected body (Pitfall 1 closed)
- [Phase ?]: 19-02: IsOverflow is a message-class matcher over *ProviderError (case-insensitive prompt-is-too-long contains over Error()), never a new ErrorKind — 400 stays KindStructural and the D-04 typed-Kind discipline holds; wording is A1 community-sourced, live re-verification is 19-04's E2E
- [Phase ?]: 19-04: the overflow retry and CompactNow bypass both the threshold and the enabled gate (the manual-intent class, D-11); only the pre-request check honors enabled — the disabled path is structurally inert, proven by zero checks/notes/extra provider calls
- [Phase ?]: 19-04: compacting note degraded to the warning-counter family (one stderr line + one counter per compaction start) — the landed session/update vocabulary has no status frame and agent_message_chunk is barred by PAR-01 bus isolation; gap in deferred-items.md — never invent a new wire frame; the plan pre-authorized this degrade
- [Phase ?]: 19-04: E2E shaped to 19-03s pinned position rule — the marker serves turns that START after it; the producing-turn re-seed carve-out (same-turn TurnID-keyed marker) was analyzed and NOT taken (Rule 4 architectural); analysis in deferred-items.md — changing a test-pinned cross-plan reset-point semantic needs architect sign-off
- [Phase ?]: 19-05: the compaction keys' absent-key 80/true defaults live in the EMBEDDED FLOOR yaml, not applyDefaults — a bool cannot distinguish absent from explicit-false, and an int backstop would eat the plan's own clamp case (a hand-edited 0 must reach the engine and compare as 1); out-of-range values pass through Load untouched (the set path rejects at the wire, the session-side comparison clamps 1..100)
- [Phase ?]: 19-05: the surface's live-apply target is the POST-WRITE effective PAIR (the written id plus the other resolved through project > global > blob > default) riding SetCompactionHook to Runner.ApplyCompactionSettings (the ApplyTurnModel discipline); the context limit resolves INSIDE the relay from the capability table — no context-limit menu entry or config key exists
- [Phase ?]: 19-05: compaction blob fills are advertisement-only D-10 in-memory defaults (no wire stamp, unlike tier/model's CR-02 blobHook) — a client default push never changes what the running session enforces; set_config_option is the operator's live lever, and the offered threshold set dropped 'off' (disabling is compaction-enabled's job)
- [Phase 19]: 19-06 (G-19-1): the same-turn compaction carve-out is ENGINE-GATED — SetRetryCompactedTurn arms the projector only on the overflow-retry path; transcript content alone never reshapes a live turn (tamper safety, kill-9 replay determinism); the override stays armed for the producing turn's remaining iterations and self-expires when a different turnID projects
- [Phase 19]: 19-06 (WR-03a): the summarize span budget derives from the SAME resolved context window as the threshold (fill-target share minus prevSummary/instruction/slack; ~400K-char fallback cap when unset; 2K-char floor) — the summarize call can never itself be an overflowing request; a cut span carries the one-line truncation notice
- [Phase 19]: 19-06 (WR-03b): at most ONE threshold-class compaction attempt per turn (compactionAttemptTurn keyed by turnID, stamped by maybeCompact AND the overflow-forced path); D-09's degraded-summarizer cadence amended — retry at the NEXT TURN's check, never one per loop head (operator-sanctioned via G-19-1's missing list)
- [Phase 19]: 19-06: the G-19-1 regression fixture drives runTurn over a pre-built mid-turn transcript (the ask-resume re-entry seam) behind a content-sensitive size-rejecting provider — a fresh Prompt's first projection is lean by construction (D-01), and a byte-identical resend now fails again (the content-blind fixture's blind spot closed)
- [Phase Phase 19]: 19-07 (CR-01): the armed same-turn carve-out takes PRECEDENCE over the pre-user marker scan in Project() — the overflow retry always projects post-marker (retry-once recovery on EVERY overflow, not just a session's first); armed with no same-turn marker the pre-user scan stays the fallback (degraded fail-through preserved)
- [Phase 22]: 22-04: stdin-pipe + pty-slave persistent shell (interactive sh ignores SIGTERM and tty-echo pollutes captures; the output-side PTY keeps colors + EIO)
- [Phase 22]: 22-04: per-generation reader goroutine + reaper dead-channel — pty-master read deadlines silently no-op (probe-verified); orphaned children mask EIO past shell death
- [Phase 22]: 22-04: additive Bash persistent catalog property per D-09/OQ1 — the documented waiver of the 08-05 byte-identical discipline (required stays [command], additionalProperties false)
- [Phase 22]: 22-06: one WrapCmd entry at all three Bash-class exec sites (foreground, registry launch funnel covering both start routes, PTY shell) — a fourth exec site without the wrap is a named review-gate violation (SAND-01)
- [Phase 22]: 22-06: the persistent OQ2 arm falls through to the foreground machinery — a per-call dangerouslyDisableSandbox can never unconfine the SHARED shell; off-arm stays the documented no-op; unconfined runs are individually noted with a process-wide counter at every site
- [Phase 22]: 22-06: zero-value Availability Mode reads as OFF at every exec-site gate — confined only when the operator asked (asking resolves landlock|seatbelt); default OFF leaves argv byte-identical with zero notes
- [Phase 22]: 22-06 (deviation): 22-05's linux leg was silently broken — ruleset rows for missing paths (/System on linux) failed the WHOLE ruleset and the re-exec child exec'd a bare argv[0] (no PATH lookup); fixed + live-proven on this host (landlock ABI 10), darwin arms compile-gated
- [Phase 22]: 22-07 (G-22-1): releaseSubagentSlot is a callers-hold-t.mu helper invoked inside Complete's critical section (floored decrement + cancel-entry delete) BEFORE startNextWaiter — release-then-admit; the plan's refinement of CR-01's self-locking sketch — Complete already holds the lock; startNextWaiter must run outside it. The release-then-admit ordering keeps the count at the cap during handoff and retires the finished id's cancel entry in the same critical section
- [Phase 22]: 22-07 (G-22-3): the panic-recovery RED used the plan-authorized subprocess guard — a background-goroutine panic produces no FAIL line (process dies), so the outer leg re-execs the test binary and converts the crash into a clean assertion (also the only valid TDD RED evidence shape) — Validated empirically: raw background-goroutine panic yields nonzero exit with no test-failure line (INVALID_RED); the guard yields target_test_failed
- [Phase 22]: [Phase 22]: 22-08 (G-22-2): scheduleWakeDrain refuses to spawn past serve shutdown (serveCtxOrBackground().Err() guard — Background-fallback test runners structurally unaffected); wakeDrainChain's exit defer stores the flag BEFORE the gated restart (ctx live AND tracker AND pending non-empty — the ordering is the race contract; completion after the peek carries itself via its own CAS on the cleared flag)
- [Phase 22]: [Phase 22]: 22-08 (G-22-4): drainWakeNotifications resolves sessions NON-constructingly (sessMu-held lookup; miss = loud terminal drop) even though CloseSession now prunes the tracker — tracker-registered-but-session-absent stays reachable (sessionFor stores tracker before session; completions racing close); CloseSession deletes trackers/wakeInFlight/ptyManagers AFTER s.Close() and cancels RUNNING subagents (tracker.CancelRunning beside CancelQueued); IN-05's unbounded retry retired incidentally
- [Phase 22]: 22-09 (G-22-5): TaskStop/TaskOutput reach background-subagent ids through two primitive-arg fallback seams (InteractiveConfig.TaskStopFallback/TaskOutputFallback — the CompletionHook precedent, coreexec stays tasks-free); the fallthrough arms ONLY on the registry's own unknown-task error, declined/nil keep the structured error byte-stable, and renderTaskOutput is the ONE envelope renderer for both id families (byte-identity by construction)
- [Phase 22]: 22-09: Tracker.SubagentState's finished row is truthful BECAUSE 22-07's releaseSubagentSlot deleted completed cancel entries — classifier-over-map-absence; the runtime binding (TaskStopFallback=tracker.CancelTask, TaskOutputFallback=SubagentState classify + bounded 64 KiB output-file tail; stat-miss=not-handled) gives CancelTask its first production callers, and block/timeout are documented as ignored for fallback ids
- [Phase 23]: 23-06 (G-23-1/CR-01): the idle /undo half consults the SELF-EXCLUDING workspaceBlockers walk, never the full restoreBlockers guard — Run marks the calling session's own turnActive before the class-B intercept, so the full guard would refuse every idle /undo (checker-verified design)
- [Phase 23]: 23-06: cross-session-before-own ordering + the refusal OUTRANKS the D-12 auto-cancel for both own-state variants (parked chain, own client turn) — nothing cancelled, nothing minted; the discriminator (restoreBlockedError.other) keeps the own-state path byte-stable
- [Phase 23]: 23-06: the workspace consult precedes SnapshotPreRestore on BOTH /undo halves — a pre-restore snapshot minted under a busy workspace is a torn D-11 walk target (shared tree committed mid-write under the other session's turn)
- [Phase 24]: Compiling-RED (stubs + failing suite) over fail-to-compile RED for 24-01 — tdd_mode #3770 requires target_test_failed — The gate must authorize GREEN on an intentional assertion failure; the plan pre-authorized the stub route
- [Phase 24]: golangci 2.13 baseline repaired per STATE prescription (exhaustruct_v5 exclusion + wsl_v5/gomodguard_v2/noinlineerr/modernize disables) — 24-01 files lint-clean under the only Go-1.27-runnable linter — 2.12.x panics under Go 1.27.1; without the repair the lint gate is unrunnable (STATE LINT BASELINE blocker)
- [Phase 24]: 24-03: worked-example LSP server auto-selected (auto_advance, gate=blocking) — isaacphi/mcp-language-server + gopls; legitimacy verified (1,590 stars, BSD-3, active); go install path, outside the npm/pip/cargo gate
- [Phase 24]: 24-03: doc cites the ACP mcpServers parse-but-ignore site by function name (handleSessionNew) — research line numbers had drifted; function-name citations survive drift
- [Phase 24]: 24-03: mcp.Start's failed-server skip is SILENT (comment claims slog logging that does not exist) — doc corrected to reality, observability gap logged to phase deferred-items.md (agent-side fix out of scope)

### Pending Todos

None yet.

### Blockers/Concerns

- [RESEARCH FLAGS / planning-time]: Phases 16/18/19/22/23 flagged for `--research-phase` (ACP schema LOW-confidence details; resume reconciliation inventory; compaction × projector pins; platform drift; pi/strands steering references unverified). Full list in ROADMAP.md Research Flags section.
- [Phase 17 review, deferred 2026-09-03, operator non-blocking]: CR-04 — SetTurnOriginAutomation set before TryLock stays true while an automation turn queues/runs; overlapping foreground turns get ask-class calls D-07-declined instead of dialogs (fail-safe direction). Proper fix: per-turn origin. Evidence in 17-UAT.md Deferred Follow-Ups.
- [Phase 17 review, deferred 2026-09-03, operator non-blocking, security-adjacent]: hasSubstitution (a710b75 rewrite, internal/perm/rules.go) dropped double-quote tracking — `git "log 'x $(cmd) y'"` under-detects live substitution and an allow rule can match a substitution-bearing command (WR-02 violation). Verified in live shell. Deserves a fix ticket in the next phase touching internal/perm.
- [25-01 Task 3 precondition, updated 2026-09-10]: 'strictly last' ordering still binds Phase 25 — 19 verified complete today, but 22/23 are partial and 24 unexecuted. Executor halted BEFORE the rank-0 move in 25-01; no kit/ paths created. Resolve by executing phases 19-24 first (then re-capture the test-ledger baseline) or by explicit operator override of the ROADMAP ordering.
- [LINT BASELINE / environmental, pre-existing before Phase 20]: mise-managed golangci-lint auto-updated to 2.13.2 which renamed linters (exhaustruct->exhaustruct_v5, wsl->wsl_v5); .golangci.yml exclusions reference pre-rename names so `mise run lint` reports ~1.6k pre-existing findings repo-wide. Verified at stash-baseline before any Phase 20 commit; every Phase 20 file passes the still-matching linters. Fix (one-line class, next touching commit): update .golangci.yml exclusion linter names to _v5 or pin golangci-lint = "2.12" in .mise.toml.
- CROSS-WORKSTREAM: Phase 23 commit 40b2bbc (steering ingress) regresses TestPermissionsE2E — verified via isolated-worktree bisection (passes at 721c7bc, fails at dbbb5d0 with zero Phase-22 acpserve changes). It also swept Phase-22 runtime.go cap-wiring hunks into their commit. Their in-flight config_surface.go menu rows (20 total) temporarily break the committed menu-count tests.

## Deferred Items

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| peer | Telegram peer (TG-01..02) — full text+voice interface consuming the steering core | Deferred to v1.3 pool (lowest priority, operator) | 2026-08-26 | v1.3 |
| provider | Per-provider HTTP(S) proxy support — proxy field in the provider config (http/https schemes, basic auth first; see todos/pending/provider-proxy-support.md) | Future work (operator request 2026-09-06) | 2026-09-06 | unscheduled |
| provider | Per-provider capability substitution — auto-substitute equivalent skills/MCP/built-in tools when a provider lacks a capability (first rows: web_search, web_fetch; e.g. z.ai/Claude-Code have them, opencode-go does not; see todos/pending/provider-capability-substitution.md) | Future work (operator request 2026-09-06) | 2026-09-06 | unscheduled |
| profile | dsh profile #2 (DSH-01..05) | Dropped entirely (operator — mimicry bar abandoned) | 2026-08-26 | none |

## Session Continuity

Last session: 2026-09-10T14:54:46.333Z
Stopped at: Completed 24-03-PLAN.md (DOC-01: LSP guide + executed dry-run)
Resume file: None
