---
phase: 20-built-in-commands-skills-per-agent-model
plan: 04
subsystem: commands
tags: [skills, agents, slash-dispatch, init, expansion-seam]

requires:
  - phase: 20-built-in-commands-skills-per-agent-model
    provides: plans 01 (chain + intercept) + 03 (dispatch planner + ResolvedModel + live agent lookup)
  - phase: 08-slash-expansion
    provides: expandUserBlocks/invocationFor seam, Expand semantics, command_provenance
  - phase: 12-discovery
    provides: SKILL.md + agents/*.md discovery, PARA dispatch machinery
provides:
  - /<skill-name> expansion (chain-aware lookup: ResolveSkill body → synthesized Command → locked Expand; provenance names the SKILL.md)
  - /<agent-name> dispatch (args as prompt, Prompt/Tools/20-03 model via DispatchSubagent, streaming result, local_command record; D-02 losers fall back to SubagentTypes for the Agent tool)
  - /init class-A live entry (authored body, builtin:init provenance, engine-on parity, advertised, D-01 shadow)
  - Skill user-invocable frontmatter (explicit false → excluded from chain + advertisement, kept in registry)
affects: [20-05 rescan, 20-06 E2E]

tech-stack:
  added: []
  patterns:
    - "Chain-aware expansion lookup (resolveSlashCommand) — one lookup source for the seam AND the engine adapter's invocationFor (engine parity by construction)"
    - "Loser fallback: chain lookup misses a D-02 loser → SubagentTypes registry answers the native Agent-tool surface"

key-files:
  created: []
  modified:
    - internal/runtime/commands.go
    - internal/runtime/runtime.go
    - internal/runtime/commands_test.go
    - internal/ecosys/types.go
    - internal/ecosys/loader.go
    - internal/session/subagent.go

key-decisions:
  - "Skills + /init ride the EXISTING seam (expandUserBlocks/invocationFor swap their reg.Commands lookup for resolveSlashCommand — the CONTEXT integration point); the intercept handles only the empty-skill-body loud reject. This makes engine-on parity free (one resolution path) and keeps provenance/mutating-boundary discipline in one place"
  - "The plan's 'expandUserBlocks UNMODIFIED' acceptance is satisfied in BEHAVIOR (shape/semantics unchanged); the one-line lookup swap is the plan's own 'chain resolver replaces the reg.Commands lookup' integration point — the two acceptance lines were in tension, resolved toward the integration point"
  - "Empty-skill-body rejection lives at the intercept (D-05 shape + failed local_command) because the seam cannot reject (it must stay a pure expansion); the body read happens twice (intercept check + seam expansion) — a bounded file read, accepted"
  - "agentDefFor falls back to SubagentTypes on a chain miss: the chain lists winners only, and D-02 guarantees the loser's NATIVE surface (Agent tool) — the fallback is that guarantee"

patterns-established:
  - "Agent-slash echo + streamed result inside the SAME turn (forwarder placement from 20-01 is what makes this work — intercept stays put)"

requirements-completed: [SKLS-01, SKLS-02, CMDS-03]

duration: 62 min
completed: 2026-09-07T22:20:00Z
---

# Phase 20 Plan 04: Skills + Agents as Slash Commands + /init Summary

**Skills expand via slash with locked substitution semantics and provenance, discovered agents dispatch as streaming subagents with Prompt/Tools/frontmatter-model applied, and /init is a live advertised class-A command expanding through the untouched seam with engine-on parity.**

## Performance

- **Duration:** 62 min
- **Tasks:** 3/3 (batteries RED-first per task)
- **Files:** 6 modified

## Accomplishments

- TestSkillSlash: $ARGUMENTS/$1 substitution + provenance naming the SKILL.md, append-under-heading, no-args without the appended block, multi-byte survival, empty-body loud reject with zero provider calls + failed record, user-invocable:false excluded from slash + advertisement while the registry keeps it.
- TestAgentSlash: dispatch with args-as-prompt, RestrictedTools from the agent's Tools, ResolvedModel (frontmatter slug) on the dispatch line, result streamed into the turn, exactly ONE provider call (the subagent's), unknown-name plain-text fallthrough, collision (skill wins slash; agent kept in registry + the SubagentTypes fallback wired).
- TestInitExpansion: engine-off + engine-on parity (identical expansion + provenance), advertisement includes init, discovered commands/init.md shadowed with the D-01 warning and never fires.

## Self-Check: PASSED

- go test -race -count=1 ./internal/runtime/ ./internal/ecosys/ ./internal/session/ green over the full battery selection (61s + 1.8s + 3.6s).
- expandUserBlocks behaviorally unchanged (only the lookup source swapped to the chain); all pre-existing expansion tests (e2e_opsx family) pass unmodified.

## Deviations from Plan

- **[Rule 2 - acceptance tension] the seam's lookup source changed by one line**: the plan required both "expandUserBlocks UNMODIFIED (no diff)" and "the seam does the replacement + provenance through the same chain the engine adapter resolves" — unsatisfiable together for skills/init (the seam looked up reg.Commands only). Resolved toward the CONTEXT integration point ("the chain resolver replaces the direct reg.Commands[key] lookups in expandUserBlocks and invocationFor"): the FUNCTION's shape/discipline is untouched; the lookup is chain-aware. Documented here per the deviation contract.
- **[Rule 2 - fixture corrections during RED]** two test-side bugs caught by the battery itself (undeclared fixture slug "glm-5.2-air" vs the declared glm-4.7-air; the scout fixture line not re-anchored) — no production impact.

## Issues Encountered

None new.

## Next Phase Readiness

resolveSlashCommand re-reads skill bodies from disk (fresh on rescan by design — 20-05 swaps the chain); the intercept's agent/skill branches are rescan-safe through the chain accessor.
