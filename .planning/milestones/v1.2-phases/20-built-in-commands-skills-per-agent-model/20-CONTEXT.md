# Phase 20: Built-in Commands + Skills + Per-Agent Model - Context

**Gathered:** 2026-08-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Slash invocation becomes one coherent resolver chain — builtins → skills → agents → file-discovered commands — with CC's useful internal commands executing control-plane-fast (no model turn) as class-B or prompt-expanding as class-A (/init), skills addressable as /skill-name, discovered AGENTS dispatchable as subagents via /agent-name, and per-agent `model:` frontmatter actually routing subagent dispatch (with cross-provider routing). Live rescan keeps discovery current without restart; available_commands_update re-fires so editor autocomplete always reflects what runs.

</domain>

<decisions>
## Implementation Decisions

### Resolver Chain & Collisions
- **D-01:** Builtin command names (CMDS-02's class-B list + /init) are RESERVED and unoverridable — a discovered file, skill, or agent sharing a builtin name never fires via slash. Matches CC (builtins are fixed logic). Autocomplete cannot lie. — **Reversibility:** costly — the reserved-name list becomes a published contract; adding a builtin later can silently (from the user's view) shadow an existing discovered command, so growth of the list needs a shadow-check warning.
- **D-02:** Collisions among skills/agents/file-commands break by chain order (CMDS-01 letter: builtins → skills → agents → file commands), silently and deterministically. The loser remains reachable through its native surface (agent dispatch via the Agent tool, skills via the Skill tool) — only the slash name is exclusive.
- **D-03:** `/agent-name` (SKLS-02) DISPATCHES the subagent: the invocation body becomes the subagent's prompt, the discovered agent's Prompt/Tools apply (existing PARA machinery), and the result returns into the conversation as a subagent turn — not a prompt expansion.
- **D-04:** available_commands_update lists WINNERS ONLY per name — what autocomplete shows is exactly what runs. One name, one truth; shadowed entries are absent, not annotated.

### Class-B Command Surface
- **D-05:** Class-B output shape: session/update user_message echoing the typed /command, then agent_message chunks carrying the output, then stopReason end_turn. Zero model turns; the local_command transcript line (16-D-22 full record: key, args, source, outcome) is the durable record. ACP has no special local-command wire semantics — commands arrive as ordinary prompt text and answer per normal turn shape.
- **D-06:** `/clear` writes a full context-reset boundary in the SAME session — the Projector's lean window starts empty; transcript and session id survive; resume is unaffected (CC parity: clears context, not the session).
- **D-07:** `/cost` numbers: use the provider's usage/billing endpoint live when one exists (operator directive) — on-demand fetch at invocation under the FAST-CONTROL timeout class (~10s, 16-D-17); on timeout or error fall back to transcript usage × modelrouting cost table WITH a source note naming which produced the number. Transcript-derived numbers survive resume (18-D-01 transcript-as-truth).
- **D-08:** `/status` = live session snapshot (resolved model + tier, provider, session id, turn count, context-usage estimate, degraded-capability flags from the counters family). `/help` = inventory generated FROM the resolver chain itself — self-describing, never drifts from reality.
- **D-09:** `/memory` = read-only view of loaded memory sources (agent-md files, learning-store summary); grows richer when PAR-04 lands in Phase 21. No inline editing — ACP has no editor-open mechanism.

### Live Rescan
- **D-10:** Live rescan (CMDS-04) = fsnotify watches on the discovery directories (debounced) driving rescan + available_commands_update re-fire, AND an invoke-time freshness re-check before every slash resolution. Watch gives autocomplete immediacy; the invoke-time backstop guarantees resolution never runs against a stale chain when the watch misses (editor quirks, races).
- **D-11:** Malformed discovered files (bad frontmatter, unreadable) are skipped with ONE structured warning naming file + reason; the rest of the registry loads — graceful-degradation family, MCP-host precedent.
- **D-12:** Watcher failure (unsupported filesystem, resource limits): ONE loud degrade warning + fallback to invoke-time-only rescan. Commands still resolve correctly; autocomplete just updates later. A session never dies over discovery.

### Per-Agent Model Routing
- **D-13:** Strict SKLS-03 precedence: frontmatter `model:` > dispatch-time model > session (parent) model > tier. An agent file with NO model frontmatter runs on the PARENT model — tiers.light is consulted only when no session model exists at all (degraded session). — **Reversibility:** costly — this REVERSES 14-05's unconditional light-tier subagent default; operators relying on light-tier economics must now write `model:` into agent frontmatter explicitly. Token spend changes silently for existing unset agents.
- **D-14:** `model: inherit` parses as unset → parent model (CC keyword parity). Any other slug routes to that model subject to D-15.
- **D-15:** Cross-provider frontmatter models ROUTE (operator override of the existing skip precedent): the dispatch builds/reaches the second provider through the modelrouting provider factory. If that provider's credentials/config are missing, the dispatch DEGRADES to the parent model + exactly ONE loud warning naming the intended model — a turn never fails over routing. — **Reversibility:** costly — a second live provider instance in one process is a new lifecycle surface (key loading, breaker state) that Phase 22's background work builds on.
- **D-16:** resolvedModel is reported BOTH ways: a field on the subagent_dispatch transcript line (durable — replay and audit see what actually ran) AND a session/update note at dispatch time (live editor visibility). Criterion 6's "reported back" holds for both live and post-hoc inspection.

### Claude's Discretion
- Debounce window for watch events (100–500ms per research; pick and document).
- Which directories get watches (project + user discovery roots; plugin caches as forced).
- /doctor's concrete check list; /mcp's output shape.
- Invalid (non-inherit, unknown) model slug in frontmatter: degrade-to-parent + loud warning follows the D-15 family — exact wording and counter name.
- Echo user_message formatting (command + args verbatim vs redacted).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §Built-in Chat Commands — CMDS-01 (one chain), CMDS-02 (class-B list + local_command lines), CMDS-03 (class-A /init via expandUserBlocks), CMDS-04 (live rescan) verbatim
- `.planning/REQUIREMENTS.md` §Slash-invocable Skills — SKLS-01 (SKILL.md body expands with args), SKLS-02 (AGENTS as slash commands, BMad layout), SKLS-03 (model precedence chain) verbatim
- `.planning/REQUIREMENTS.md` §ACP completeness — ACP-04 (available_commands_update on start + discovery change) verbatim
- `.planning/ROADMAP.md` §Phase 20 — goal, 6 success criteria (autocomplete, live pickup, instant class-B set, /init provenance, /skill + /agent invocation, per-agent routing)

### Prior Phase Contracts (hard dependencies)
- `.planning/phases/16-acp-wire-foundation/16-CONTEXT.md` — D-05 (full menu advertised day-1), D-17 (FAST-CONTROL timeout class), D-22 (full local_command transcript record), available_commands_update re-fire contract
- `.planning/phases/18-session-family/18-CONTEXT.md` — D-10/D-11 (/resume rides the picker machinery — class-B /resume delegates there)
- `.planning/phases/19-*` — /compact delegates to Phase 19's compaction machinery (PAR-01); this phase only routes the invocation

### External References
- `agentclientprotocol.com/protocol/v1/slash-commands` — commands arrive as ordinary session/prompt text; available_commands_update shape (name, description, input hint)
- `agentclientprotocol.com/protocol/v1/schema` — stopReason closed enum; session/update content kinds for D-05's shape
- `code.claude.com/docs/en/sub-agents` — model: inherit semantics (D-14's parity target)
- `code.claude.com/docs/en/skills` — builtins-are-fixed-logic, scope-precedence reference for D-01

### Code Anchors
- `internal/runtime/runtime.go` expandUserBlocks/invocationFor (377–470) — the existing expansion seam class-A rides unchanged (CMDS-03)
- `internal/ecosys` loader/registry — Agent type already carries Model; one-shot Load becomes the rescan target
- `internal/runtime/runtime.go` resolveSubagentModel (~1160) — the 14-05 light-tier resolution D-13 modifies
- `internal/modelrouting` — resolver/dispatch/factory for D-15's cross-provider routing
- fsnotify upstream guidance: watch directories not files; debounce editor multi-events; polling fallback for network filesystems

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- expandUserBlocks/invocationFor seam (runtime.go:377–470): single-parse resolution + expansion with provenance lines and mutating-boundary discipline — class-A /init rides it verbatim; the resolver chain generalizes the single `reg.Commands` map lookup at its heart.
- internal/ecosys Registry: Skills/Commands/Plugins/Agents maps with deterministic AllX() accessors; Agent struct already has Name/Description/Tools/Model/Prompt/Path; discoverAgents walks .claude/agents (BMad layout already first-class).
- PARA subagent machinery: SubagentTypes wired from reg.Agents at sessionFor; per-dispatch profile copy (subagentProfile) is the natural carrier for D-13's routing.
- modelrouting provider factory: multi-provider config, resolver, cost table — D-07's fallback numbers and D-15's second provider both come from here.
- Engine advisory-note machinery: D-16's resolvedModel note reuses the session/update note shape + dedupe discipline.

### Established Patterns
- Graceful degradation + loud counters — D-11/D-12/D-15 warnings and D-07's source note join the family.
- Transcript-as-truth (18-D-01) — D-07's fallback derivation and D-16's durable field both reconstruct from the transcript.
- One-parse resolution feeding multiple consumers (invocationFor pattern) — the chain resolver keeps that shape.

### Integration Points
- available_commands_update emission points: session start (existing seam from Phase 16) + rescan completion (new).
- The chain resolver replaces the direct `reg.Commands[key]` lookups in expandUserBlocks and invocationFor.
- fsnotify watcher lifecycle ties to the serve-lifetime context (per-session? serve-wide — planner decides; sessions share one registry per serve today).
- /model and /config class-B handlers call Phase 16's set-config internals (persist-then-apply, 16-D-07).

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- /cost must prefer the provider's own usage/billing endpoint when one exists — "there is a url or queries for some providers returning such data. it must be used when available. use transcript × cost if no such url exists" — with on-demand fetch + fallback pinned.
- Cross-provider routing chosen deliberately over the skip precedent: a frontmatter model on another provider actually routes there; only missing credentials degrade to parent. "Mis-routed background launches become impossible" (criterion 6) read as routing-intent-is-honored.
- Strict SKLS-03 accepted with eyes open: unset agents get the parent model, reversing 14-05's light-tier default — explicit frontmatter becomes the token-economics lever.

</specifics>

<deferred>
## Deferred Ideas

- Namespaced autocomplete (/foo@agent suffixes for colliding names) — rejected for CC divergence; revisit only if silent shadowing proves painful in practice.

</deferred>

---

*Phase: 20-built-in-commands-skills-per-agent-model*
*Context gathered: 2026-08-27*
