---
title: Per-provider capability substitution (skills/MCP/tools)
resolves_phase:
created: 2026-09-06
source: operator request (verify-work session, 2026-09-06)
priority: normal
---

# Per-provider capability substitution (skills/MCP/tools)

## What

Different providers grant the agent different levels of support. Example
(operator's): Claude Code and z.ai plans ship **web search and web fetch**
support; opencode-go does not. The agent must **automatically substitute**
equivalent skills / MCP servers / built-in tools per provider, so the
effective capability set stays equivalent regardless of which provider a
turn routes to.

## Requirements

- Per-provider **capability declaration** in the provider config (which
  native capabilities the provider/plan exposes: web_search, web_fetch, …).
- A **substitution table**: capability → ordered fallback chain
  (provider-native → agent built-in tool → MCP server → skill →
  absent-with-loud-note). First concrete rows: web_search, web_fetch.
- Substitution happens automatically at session/turn construction based on
  the routed provider; explicit operator config overrides the automatic
  choice.
- When NOTHING can substitute, degrade loudly in the documented style
  (the D-11/D-14 family) — never a silently missing tool.

## Design considerations (for whoever picks it up)

- The profile's tool catalog is the mimicry surface: substitution must keep
  the outgoing tool catalog consistent with what is actually executable —
  stable tool names with swappable backing implementations (provider-native
  vs local WebSearch/WebFetch port vs MCP `mcp__<server>__<tool>`).
- Cross-links: the modelrouting resolver already picks the provider per
  turn (tiers/time windows/fallbacks) — substitution must re-resolve when a
  fallback chain lands on a different provider mid-conversation.
- Candidate seams: `internal/modelrouting` (provider choice),
  `internal/providerfactory` (capability flags — see the 21-05
  `SupportsImages` precedent for per-provider capability), the profile's
  tool-catalog builder.
- Tests: table-driven substitution matrix (provider × capability → expected
  effective tool source); a pin that a provider lacking web_search gets the
  local implementation under the SAME tool name; a pin for loud degradation
  when the chain is empty.

## Non-goals (first cut)

- Auto-discovery of arbitrary replacement MCPs from the network.
- Mid-turn hot-swapping (resolve at construction; a mid-conversation
  provider fallback re-resolves at the next turn boundary).
- User-facing per-skill toggles beyond the explicit config override.
