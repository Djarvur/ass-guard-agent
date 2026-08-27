# Phase 22: Background Execution + Sandbox Reality - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-27
**Phase:** 22 - Background Execution + Sandbox Reality
**Areas discussed:** Notification delivery, Sandbox profile content, Persistent-shell lifecycle, Background caps

**Context:** Discussed inline while Phase 16 plans in background. Research-before-questions enabled.

---

## Notification delivery

Research basis: CC's harness injects `<task-notification>` on completion and re-invokes the model; TaskOutput deprecated in favor of output-file Read ([#21048](https://github.com/anthropics/claude-code/issues/21048), [tools reference](https://code.claude.com/docs/en/tools-reference), [from-source ch10](https://claude-code-from-source.com/ch10-coordination/)).

| Option | Description | Selected |
|--------|-------------|----------|
| Wake-turn + fallback | Agent-initiated turn via automation machinery; inject-next-turn while busy | ✓ |
| Inject-on-next-turn | Passive queue only; model waits for user | |

**User's choice:** Wake-turn + fallback (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Status + tail + pointer | id, kind, status, duration, tail, file pointer | ✓ |
| Status + pointer only | Minimal; forces a Read before reacting | |

**User's choice:** Status + tail + pointer (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Coalesce one turn | All pending notifications in one wake turn | ✓ |
| One turn each | N tasks = N turns and N model calls | |

**User's choice:** Coalesce one turn (Recommended)

---

## Sandbox profile content

Research basis: go-landlock RODirs/RWDirs canonical shape; ABI v4+ TCP restrictions ([pkg.go.dev](https://pkg.go.dev/github.com/landlock-lsm/go-landlock/landlock), [kernel docs](https://docs.kernel.org/userspace-api/landlock.html)). Landlock restricts process-wide → confinement targets spawned children.

| Option | Description | Selected |
|--------|-------------|----------|
| FS + network deny | rw workdir/tmp/.ass-guard, ro system, network denied | ✓ |
| FS only | Simpler; confined Bash can still phone out | |

**User's choice:** FS + network deny (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Spawned processes only | Children confined; in-process + MCP unconfined, documented | ✓ |
| Whole process | Max containment; breaks provider/MCP networking | |

**User's choice:** Spawned processes only (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Templates + one policy | Embedded templates, runtime path substitution, shared policy struct | ✓ |
| Runtime generation | Full .sb files written per session; injection surface | |

**User's choice:** Templates + one policy (Recommended)

---

## Persistent-shell lifecycle

| Option | Description | Selected |
|--------|-------------|----------|
| One per session | CC model; shared PTY; serialized persistent calls | ✓ |
| Per-workdir pool | Parallel-safe; state splits confusingly | |

**User's choice:** One per session (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Session-owned + lazy restart | Close drains; dead shell restarts with visible note | ✓ |
| Idle timeout | Resource-conserving; state vanishes mid-session | |

**User's choice:** Session-owned + lazy restart (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Per-call opt-in | Bash tool argument; default stays stateless | ✓ |
| Session-wide toggle | All Bash through PTY; bad state poisons everything | |

**User's choice:** Per-call opt-in (Recommended)

---

## Background caps

| Option | Description | Selected |
|--------|-------------|----------|
| Concurrent cap + queue | Default 8; FIFO queue with visible note | ✓ |
| Uncapped | Trust the model; runaway possible | |
| Cap + reject | Typed error on over-cap | |

**User's choice:** Concurrent cap + queue (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Same policy, own cap | Bash default 16; one discipline, two numbers | ✓ |
| Bash uncapped | Cheaper processes; fork-bomb risk | |

**User's choice:** Same policy, own cap (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| configOptions + defaults | Operator-tunable; joins 16's menu | ✓ |
| Constants | Compile-time; not tunable | |

**User's choice:** configOptions + defaults (Recommended)

---

## Deferred Ideas

None — discussion stayed within phase scope.
