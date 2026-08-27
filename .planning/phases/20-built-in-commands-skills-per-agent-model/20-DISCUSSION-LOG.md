# Phase 20: Built-in Commands + Skills + Per-Agent Model - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-27
**Phase:** 20 - Built-in Commands + Skills + Per-Agent Model
**Areas discussed:** Resolver chain & collisions, Class-B command surface, Live rescan mechanism, Per-agent model routing

**Context:** Discussed inline while Phase 15 executes and Phase 16 plans in background. Research-before-questions enabled.

---

## Resolver chain & collisions

Research basis: CC built-ins are non-overridable fixed logic; scope precedence enterprise > personal > project ([official](https://code.claude.com/docs/en/skills)); skill-vs-command same-name is an open CC bug — skill silently blocks the command ([#14945](https://github.com/anthropics/claude-code/issues/14945)).

| Option | Description | Selected |
|--------|-------------|----------|
| Reserved, unoverridable | Builtins always win; file/skill named /status never fires | ✓ |
| Discovered may shadow | User files override builtins | |
| Shadow + warn | Shadow allowed with per-session warning | |

**User's choice:** Reserved, unoverridable (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Chain order, silent | builtins → skills → agents → files; deterministic tiebreak | ✓ |
| Chain order + load warn | Same + one warning per collision | |
| Namespaced autocomplete | /foo, /foo@agent suffixes in autocomplete | |

**User's choice:** Chain order, silent (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Dispatch subagent | /agent-name runs the agent; body = its prompt | ✓ |
| Expand prompt | Agent system prompt injected as user turn | |
| Foreground turn | Agent prompt prepended to a regular turn | |

**User's choice:** Dispatch subagent (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Winners only | Autocomplete shows exactly what runs | ✓ |
| All entries | Shadowed names also listed | |
| Winners + shadow note | Disabled annotated entries for shadowed names | |

**User's choice:** Winners only (Recommended)

---

## Class-B command surface

Research basis: ACP slash-commands arrive as ordinary session/prompt text; agent recognizes prefix and answers per normal turn shape; closed stopReason enum ([slash-commands](https://agentclientprotocol.com/protocol/v1/slash-commands), [schema](https://agentclientprotocol.com/protocol/v1/schema)).

| Option | Description | Selected |
|--------|-------------|----------|
| agent_message + end_turn | Echo + output chunks + end_turn; local_command line on disk | ✓ |
| thought chunk | Output as agent_thought_chunk — quieter, maybe hidden | |
| Disk-only | No streamed output at all | |

**User's choice:** agent_message + end_turn (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Echo user_message | Typed command appears before output; replay matches | ✓ |
| No echo | Only output appears | |

**User's choice:** Echo user_message (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Context reset boundary | Same session; lean window empties; transcript survives | ✓ |
| New session | Close + reopen — surprises the editor | |
| Boundary + marker | Boundary plus explicit marker line | |

**User's choice:** Context reset boundary (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Transcript × cost table | Derive from transcript usage records | |
| In-memory counters | Live only; lies after resume | |
| Free-text | "provider url/queries when available; transcript × cost otherwise" | ✓ (operator directive) |

**User's choice:** free-text — provider endpoint preferred, transcript fallback.

### Follow-ups

| Option | Description | Selected |
|--------|-------------|----------|
| On-demand + fallback | Live fetch, FAST-CONTROL timeout, fallback with source note | ✓ |
| Live only | Error to user on failure | |
| Cached fetch | Session TTL cache | |

**User's choice:** On-demand + fallback (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Live snapshot + chain | /status live state; /help generated from resolver | ✓ |
| Minimal both | /help static table; /status model + id | |
| Claude decides | Contents pinned at planning | |

**User's choice:** Live snapshot + chain (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Read-only view | Lists loaded memory sources; grows with PAR-04 | ✓ |
| Open-for-edit | Inline editing — ACP lacks editor-open | |
| Defer to 21 | Not in this phase's set | |

**User's choice:** Read-only view (Recommended)

---

## Live rescan mechanism

Research basis: watch directories not files (atomic saves break file watches — [fsnotify#17](https://github.com/fsnotify/fsnotify/issues/17)); debounce editor multi-events ([practice](https://medium.com/@impactarchitecture/file-watchers-lie-debounce-throttle-and-coalescing-in-build-loops-8d91cb29f712)); inotify unreliable on network filesystems → polling fallback ([docs](https://pkg.go.dev/github.com/fsnotify/fsnotify)).

| Option | Description | Selected |
|--------|-------------|----------|
| Watch + invoke-time | fsnotify drives updates; every invocation re-checks freshness | ✓ |
| Watch only | Cheaper; missed event silently resolves stale | |
| Invoke-time only | Always correct; autocomplete drifts | |

**User's choice:** Watch + invoke-time (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Skip + warn | One structured warning per bad file; rest loads | ✓ |
| Fail rescan | One typo kills all new discovery | |

**User's choice:** Skip + warn (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Degrade to invoke-only | Loud warning; commands still work | ✓ |
| Poll fallback | Periodic mtime scan keeps autocomplete live | |

**User's choice:** Degrade to invoke-only (Recommended)

---

## Per-agent model routing

Research basis: CC `model: inherit` = unset → parent model; env var overrides frontmatter; CC's own precedence differs from SKLS-03's locked chain ([sub-agents](https://code.claude.com/docs/en/sub-agents), [model-config](https://code.claude.com/docs/en/model-config)). SKLS-03 chain treated as locked, not re-asked.

| Option | Description | Selected |
|--------|-------------|----------|
| Keep 14-05 light default | Unset agents → tiers.light; frontmatter/dispatch override | |
| Strict SKLS-03 | Unset agents → parent model; tier only in degraded sessions | ✓ |

**User's choice:** Strict SKLS-03 (Recommended) — reverses 14-05's default knowingly.

| Option | Description | Selected |
|--------|-------------|----------|
| Accept inherit | Parses as unset → parent (CC parity) | ✓ |
| Unknown-slug degrade | inherit unknown → degrade + warn | |

**User's choice:** Accept inherit (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Skip + loud warn | Keep parent; one warning (existing precedent) | |
| Route cross-provider | Second provider instance actually used | ✓ (operator override) |

**User's choice:** Route cross-provider — NOT the recommendation.

| Option | Description | Selected |
|--------|-------------|----------|
| Degrade to parent | Missing credentials → parent + one loud warning | ✓ |
| Fail dispatch | Typed error kills the turn | |

**User's choice:** Degrade to parent (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Transcript + note | Durable field on dispatch line + live session/update note | ✓ |
| Transcript only | Durable but invisible live | |
| Note only | Visible live but lost to replay | |

**User's choice:** Transcript + note (Recommended)

---

## Deferred Ideas

- Namespaced autocomplete (/foo@agent suffixes) — rejected for CC divergence; revisit if silent shadowing proves painful.
