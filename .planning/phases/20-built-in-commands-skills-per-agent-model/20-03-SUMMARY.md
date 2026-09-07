---
phase: 20-built-in-commands-skills-per-agent-model
plan: 03
subsystem: subagents
tags: [per-agent-model, model-routing, cross-provider, subagents, transcript]

requires:
  - phase: 20-built-in-commands-skills-per-agent-model
    provides: plan 01 — chain accessor (live agent lookup)
  - phase: 14-scheduling (EARLY-05)
    provides: resolveSubagentModel light-tier machinery (reversed to the model-less arm) + modelrouting resolver/factory/credential surfaces
  - phase: 12-discovery
    provides: Agent.Model frontmatter (parsed since 12-02), PARA dispatch machinery
provides:
  - Runner.planSubagentDispatch — D-13 strict precedence (frontmatter > dispatch-time > session/parent > tier), D-14 inherit normalization, D-15 cross-provider routing with per-(provider, session) cached second provider, one-warning degrades with counter
  - session.SubagentDispatchPlan + SubagentModelPlanner seam (runtime hands routing in; internal/session stays modelrouting-free)
  - session.AgentLookup live-chain agent registry (rescan-safe dispatch: post-construction agents dispatchable)
  - Line.ResolvedModel on subagent_dispatch lines (D-20-tolerant) + deduped live dispatch note (advisoryNoteDue classes)
affects: [20-04 agent slash dispatch, 20-06 E2E, Phase 22 background subagents]

tech-stack:
  added: []
  patterns:
    - "Planner seam: runtime resolves routing, session executes — the OnClose/func-seam convention extended to dispatch"
    - "Credential check BEFORE factory construction (ResolveCredential export) — the uncredentialed degrade fires without building the lazy-failing provider"

key-files:
  created: []
  modified:
    - internal/runtime/runtime.go
    - internal/session/subagent.go
    - internal/session/session.go
    - internal/session/transcript.go
    - internal/session/manager.go
    - internal/session/subagent_test.go
    - internal/runtime/subagent_tier_wiring_test.go

key-decisions:
  - "SubagentModelPlanner takes the SESSION as first arg (the parent model + advisory dedupe are session-scoped; the original agentDef-only signature forced a fragile 'current session' lookup)"
  - "D-15 credentials resolved via modelrouting.ResolveCredential BEFORE BuildWithCapturer — cleaner than detecting the lazy-failing noCredentialProvider after construction"
  - "The cross-provider provider is built with a factory over schedCfg (nil capturer) inside the runner — BuildWithCapturer stays the single construction seam (AUD-01); acpserve needs no new wiring"
  - "subagentProfile now stamps plan.Model only — Session.SubagentModel stays as a field (compat) but is never stamped at sessionFor; the 14-05 tier tests rewritten to the reversal (NoLightTierStamp) + dispatch-time tier arms"
  - "The D-16 live note rides a bus AgentMessageChunk published from DispatchSubagent (the parent turn's forwarder delivers it) rather than the post-turn advisory collector — dispatch notes are mid-turn by nature"

patterns-established:
  - "TestDispatchModel battery shape: planner-seam unit tables (arms, degrades, dedupe, cache identity via require.Same) + session-level line assertions"

requirements-completed: [SKLS-03]

duration: 74 min
completed: 2026-09-07T21:55:00Z
---

# Phase 20 Plan 03: Per-Agent Model Routing Summary

**Subagent dispatch now honors agent `model:` frontmatter with the strict SKLS-03 precedence (14-05's unconditional light-tier default reversed), routes cross-provider models through a cached factory-built second provider, degrades every failure to parent with exactly one warning, and reports the resolved model both live and durably.**

## Performance

- **Duration:** 74 min
- **Tasks:** 3/3 (single coherent commit — the three tasks share one seam; batteries written per task before each implementation slice)
- **Files:** 7 modified

## Accomplishments

- D-13/D-14: TestDispatchModel_Precedence pins frontmatter/inherit/dispatch-time/parent arms; TestDispatchModel_TierArms pins the reversal (session model present → parent despite a light binding; model-less session → tiers.light; cross-provider light binding on a model-less session → the 14-05 Test-4 warning, moved to its dispatch-time home).
- D-15: cross-provider gpt-air ROUTES via the cached second provider (require.Same cache identity); unknown slug and uncredentialed providers degrade to parent with one stderr warning naming the slug/reason + counter; notes dedupe per session.
- D-16: every subagent_dispatch line carries ResolvedModel (routed slug / parent on inherit or degrade); legacy lines without the field load empty (D-20); the live note dedupes via the advisory machinery.
- Rescan-safe dispatch: TestDispatchModel_LiveAgentLookup registers an agent after session construction and dispatches it through the chain accessor.

## Self-Check: PASSED

- go test -race -count=1 ./internal/runtime/ ./internal/session/ green (106s + 3.7s).

## Deviations from Plan

- **[Rule 2 - signature correction] the planner seam takes the session** (sess, agentDef, dispatchModel) instead of (agentDef, dispatchModel): the parent model and the advisory dedupe are session-scoped, and the plan's suggested signature would have forced a fragile "current session" heuristic. The plan's artifacts list said "runtime resolves and passes results in" — held exactly.
- **[Rule 3 - task merge] one commit for the three tasks**: the seam, the precedence, and the durable field are one type/threading change (SubagentDispatchPlan flows through dispatch → profile → line writer); splitting the commits would leave intermediate commits that don't compile or tests that can't assert the line field. Batteries were still written RED-first per task.

## Issues Encountered

None new.

## Next Phase Readiness

20-04's /<agent-name> dispatch rides DispatchSubagent with the planner already wired; Phase 22's background subagents inherit the resolution and the provider cache.
