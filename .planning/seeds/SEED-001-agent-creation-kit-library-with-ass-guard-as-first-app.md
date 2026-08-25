---
id: SEED-001
status: dormant
planted: 2026-08-16
planted_during: v1.1 Kickoff & Peers / Phase 8 (gate-ready, awaiting operator witness)
trigger_when: when planning the milestone after v1.1 (v1.2+), especially once Phase 10's internal/runtime extraction and Phase 11's DSH-01 genericization have landed
scope: large
audit_acknowledged:
  milestone: v1.1
  at: 2026-08-25
  status: dormant
---

# SEED-001: Extract the agent-creation kit from ass-guard-agent as a library (in this repo); ass-guard-agent itself becomes an app built on the kit

## Why This Matters

**The main kit value: an ability to build your own agent with the desired shape, not tied to any ecosystem at all.**

ass-guard-agent's engine is already broader than its own product skin. The mimicry/profile mechanism, provider clients (Anthropic + OpenAI shapes), session/turn loop, projector, unified engine + hook-DAG, tool catalog/execution, scheduling, and redaction are all general agent-building machinery — the zcode profile, `.claude/` compat, OpenSpec hosting, ACP/Telegram frontends, and SDD autocontinue are one particular composition of them. Extracting the machinery as a library (in this repo) turns that composition into the reference app instead of the only shape: anyone can build an agent with exactly the shape they want — different profile, different frontend, different ecosystem coupling, or none — on the same proven core.

This also hardens the existing thesis rather than competing with it: "N profiles, no target-specific code paths" (DSH-01) and the Phase-10 `internal/runtime` extraction are both steps toward a front-end-agnostic, target-agnostic core; the kit is the same trajectory carried to its library conclusion.

## When to Surface

**Trigger:** when planning the milestone after v1.1 (v1.2+), especially once Phase 10's `internal/runtime` extraction and Phase 11's DSH-01 genericization have landed — those two deliver most of the decoupling the kit needs as a side effect.

Natural moments to re-read this seed:

- v1.1 close / v1.2 milestone scoping (`/gsd:new-milestone` scan)
- after Phase 10 lands `internal/runtime` as the shared turn core
- after Phase 11 proves the second profile with zero target-specific code paths
- if external interest appears in embedding ass-guard's engine outside the ACP+Telegram surfaces

## Scope Estimate

**Large** — a full architectural restructure: define the library's public API surface over the existing `internal/*` packages, decide module boundaries (single module with `pkg/` vs. separate library module in-repo), invert the remaining app-specific dependencies (`.claude/` layout, profile defaults, ecosystem adapters) behind kit seams, and re-home `cmd/ass-guard` as the first app composed from the kit. Phase 10's extraction and Phase 11's genericization should be treated as deliberate down-payments — sequencing the kit work right after them avoids re-doing the decoupling twice.

## Breadcrumbs

- `.planning/ROADMAP.md` — Phase 10 performs "the milestone's one structural refactor: extracting the turn core (runner + engine/hook/MCP wiring) from `cmd/ass-guard/acp_serve.go` into `internal/runtime`" — the first concrete step of the app/kit split (frontends become clients of a shared core)
- `.planning/ROADMAP.md` — Phase 11 DSH-01: "Shared mimicry code contains no zcode-specific paths … 'N profiles, no target-specific code paths'" — the ecosystem-decoupling half
- `internal/` (24 packages) — the kit's raw material: `engine`, `session`, `loop`, `shaper`, `provider`, `profile`, `scheduler`, `toolcat`, `toolexec`, `coreexec`, `hookdag`, `mcp`, `redact`, `parity`, `drift`, `ecosys`, `openspec`, `acp`, `audit`, `event`, `learning`, `firstrun`, `defaults`, `version`
- `cmd/ass-guard/` — the app composition layer that would become the first kit consumer (`acp_serve.go`, `provider_factory.go`, `scheduling.go`, …)
- `.planning/PROJECT.md` — "the architecture supports N [mimicry] targets from day one (decided in PROJECT.md)" — the kit generalizes this from N profiles to N agents

## Notes

Captured 2026-08-16 from the operator: "extract the agent creation kit from the ass-guard-agent and provide it as a library (in this repo) make as-guard-agent itself the app based on the kit — the main kit value is an ability to build your own agent with the desired shape, not tied to any ecosystem at all."

Tension to resolve at milestone planning: v1.1's cross-phase invariants (single static binary, stdout discipline, `.claude/` read-only) are app-level constraints — the kit must expose them as choices, not bake them in; conversely the app must keep enforcing them. Also note the ROADMAP's v1.1 scope line explicitly caps structural refactors at one (`internal/runtime`); the kit restructure is v1.2+ material by that same discipline.
