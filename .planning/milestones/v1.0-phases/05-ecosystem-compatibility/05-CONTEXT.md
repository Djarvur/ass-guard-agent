# Phase 5: Ecosystem Compatibility - Context

**Gathered:** 2026-08-12
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 5 makes ass-guard a drop-in member of the Claude-Code ecosystem. A Claude Code user's existing `.claude/` setup — MCP servers, skills, slash-commands, plugins — works unchanged inside ass-guard. ass-guard's own additions live separately under `.ass-guard/` and never clobber Claude Code's files.

**In scope (5 REQ-IDs):**
- **MCP hosting (ECOS-01/02/03):** Claude-Code-installed MCP servers run in ass-guard as subprocesses via `modelcontextprotocol/go-sdk` `StdioMCPClient`. Process-group spawn, group-signal shutdown, reaper goroutine (no zombies). `tools/list` re-fetched on every connection (no schema drift).
- **Skills/commands/plugins (ECOS-04):** Claude Code skills, slash-commands, and plugins load from the `.claude/` layout and work unchanged.
- **Namespacing (ECOS-05):** ass-guard's own additions live under `.ass-guard/`, never clobbering Claude Code's `.claude/` files. User→project precedence respected.

**Out of scope:**
- Distribution + zero-config first run (Phase 6)
- Additional profiles beyond zcode (v2, PROF-06/07)

**Mode:** mvp (ROADMAP.md Phase 5). Smallest phase (5 REQ-IDs). STACK pins `modelcontextprotocol/go-sdk` v1.0.0+.

</domain>

<decisions>
## Implementation Decisions

### MCP hosting & tool bridging (ECOS-01/02/03)

- **D-01:** The profile declares the **built-in tools** (the stable core ~20). MCP tools are discovered **dynamically at session start**: ass-guard spawns each configured MCP server (from `.claude/` settings or `.mcp.json`), calls `tools/list`, and merges the discovered tools into the catalog alongside the built-ins. The profile does NOT hardcode MCP tools (they're session-dependent — Phase 1 found 77/97/103 variation because MCP tools change per session config). The Shaper includes both built-in + dynamic MCP tools in the outgoing request's tool catalog. This is WHY the tool count varies per session. *[User-selected.]*
- **D-02:** MCP server subprocess lifecycle (ECOS-02): spawned via `go-sdk`'s `StdioMCPClient` with **process-group isolation** (setsid/setpgid so signaling the group reaches all children), **group-signal shutdown** (SIGTERM to the process group on session end), and a **reaper goroutine** (Wait4 in a loop to prevent zombies). The lifecycle is: session/new → spawn all configured MCP servers → session/close/cancel → signal shutdown → reap.
- **D-03:** `tools/list` is re-fetched on **every connection** (ECOS-03): ass-guard NEVER caches MCP tool schemas across sessions. Each session start re-discovers tools. This prevents schema drift (if an MCP server is updated between sessions, ass-guard sees the new schemas — no stale cache). N13 and N14 (schema-drift pitfalls) engineered out.
- **D-04:** MCP tools register in the catalog as `mcp__<server>__<tool>` (Claude-Code naming convention per STACK). The model invokes them like any built-in tool; the catalog routes the call through the MCP client to the server subprocess.

### Skills/commands/plugins + namespacing (ECOS-04/05)

- **D-05:** ass-guard's own additions live in a **separate `.ass-guard/` directory** (consistent with Phase 2 D-06 transcripts + Phase 4 D-16 learned settings — all ass-guard state under `.ass-guard/`). Claude Code's `.claude/` directory is READ-ONLY from ass-guard's perspective: ass-guard loads CC's skills/commands/plugins from `.claude/` but never writes there. ass-guard's own skills/commands/plugins live under `.ass-guard/`. *[User-selected — chose separate dir over namespaced .claude/ass-guard/ subdir.]*
- **D-06:** ass-guard loads from BOTH directories: `.claude/` (Claude Code compat — user's existing setup) AND `.ass-guard/` (ass-guard's own additions). Precedence: `.claude/` wins on conflict (CC's files are the user's existing setup; ass-guard's additions must never override them — ECOS-05 "never clobbering"). User-scope → project-scope precedence within each directory (same as Claude Code).
- **D-07:** Skills and slash-commands follow Claude Code's discovery rules: skill `SKILL.md` files, command markdown files, loaded from the standard directory layout. Plugins follow the plugin manifest format. ass-guard implements the same loading logic as Claude Code (read-only from `.claude/` + own from `.ass-guard/`).

### Claude's Discretion
D-02/D-03/D-04/D-07 follow directly from the ECOS requirements + STACK. Not gray areas.

</decisions>

<canonical_refs>
## Canonical References

### Prior-phase outputs
- `.planning/phases/01-mimicry-mvp-north-star-proof/01-CONTEXT.md` — D-14 (tool catalog scope), D-16 (data-source: tool count varies per session because MCP tools are dynamic — this phase implements the dynamic discovery)
- `.planning/phases/02-session-core-acp-interface/02-CONTEXT.md` — D-06 (`.ass-guard/` directory — Phase 5 puts skills/commands/plugins alongside transcripts), D-07 (self-gitignoring dir)

### The 5 Phase-5 requirements
- `.planning/REQUIREMENTS.md` §"Ecosystem compatibility (Phase 5)" (ECOS-01..05)

### STACK references
- `.planning/research/STACK.md` §Focus 5 (MCP Server Hosting) — `modelcontextprotocol/go-sdk` v1.0.0+ (official, Google-collaborated, protocol 2026-07-28). `StdioMCPClient` launches MCP server as subprocess, speaks JSON-RPC over stdin/stdout. `mark3labs/mcp-go` as alternative (official wins on spec-compliance). `mcp__<server>__<tool>` naming convention.
- `.planning/research/STACK.md` §"Claude Code .claude/ config layout" — drop-in ecosystem compat. `settings.json`, `CLAUDE.md`/`AGENTS.md` hierarchy, `.claude/commands/`, `.claude/agents/`, `.claude/skills/`, `.mcp.json`.

### Phase goal + success criteria
- `.planning/ROADMAP.md` §"Phase 5" — 4 success criteria (MCP server runs; lifecycle robust; skills/commands/plugins load; namespacing clean).

### Codebase (Phases 1-4)
- `internal/toolcat` — the tool catalog where MCP tools merge with built-ins (D-01)
- `internal/session` — the Session Core that spawns MCP servers at session/new and shuts them down at session/close
- Phase 5 adds: `internal/mcp` (the MCP host: subprocess lifecycle + tool bridging), expands `internal/toolcat` (dynamic MCP tool registration), adds `.ass-guard/` skills/commands/plugins loader

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Tool catalog** (Phase 1/2 `internal/toolcat`): built-in tools are registered at startup. Phase 5 adds dynamic MCP tool registration at session start — the catalog grows per-session.
- **Session Core** (Phase 2 `internal/session`): the `session/new` handler is where MCP servers spawn; `session/close`/cancel is where they shut down.
- **`.ass-guard/` directory** (Phase 2 D-06/D-07): already exists for transcripts + learned settings. Phase 5 adds skills/commands/plugins subdirectories.

### Integration Points
- **session/new → MCP host → tool catalog:** at session start, the MCP host spawns all configured servers, fetches tools/list, and registers discovered tools in the catalog. The Shaper then includes them in the outgoing request.
- **Model → MCP tool call → MCP client → MCP server:** when the model invokes `mcp__<server>__<tool>`, the catalog routes the call through the MCP client to the server subprocess, gets the result, and returns it as a tool result.

</code_context>

<specifics>
## Specific Ideas

- The user chose **separate `.ass-guard/` directory** (D-05) over the recommended `.claude/ass-guard/` namespaced subdir. This keeps all ass-guard state in one place (transcripts, learned settings, skills/commands/plugins) and maintains a clean separation from Claude Code's `.claude/` — ass-guard never writes to `.claude/`, only reads from it. The trade-off: skills/commands/plugins exist in two places (`.claude/` for CC compat, `.ass-guard/` for ass-guard's own), but the precedence rule (`.claude/` wins) keeps it clean.
- The Phase 1 finding that tool count varies per session (77/97/103) is NOW explained: MCP tools are dynamic, discovered at session start (D-01). The profile declares the stable built-in core; the variable tail comes from whatever MCP servers are configured for the current session. This is not drift — it's expected session-dependence.

</specifics>

<deferred>
## Deferred Ideas

None raised. Phase 5 is tightly scoped to drop-in ecosystem compat.

</deferred>

---

*Phase: 5-Ecosystem Compatibility*
*Context gathered: 2026-08-12*
