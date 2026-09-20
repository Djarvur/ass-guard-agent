# Phase 21: Context & Policy Parity Closures - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-27
**Phase:** 21 - Context & Policy Parity Closures
**Areas discussed:** Hook authority & merge, AGENTS.md discovery set, Rich content rules, Thinking verification

**Context:** Discussed inline while Phase 16 plans in background. Research-before-questions enabled.

---

## Hook authority & merge

Research basis: CC PreToolUse hooks return hookSpecificOutput.permissionDecision (allow/deny/ask) via stdout JSON, exit-2 legacy deny, hooks fire before permission checks ([guide](https://scalably.io/blog/claude-code-hooks-guide), [explainer](https://blakecrosby.com/blog/claude-code-hooks-explained), [Developers Digest](https://www.developersdigest.tech/blog/claude-code-hooks-explained)).

| Option | Description | Selected |
|--------|-------------|----------|
| User-allow / project-deny | User-scope hooks may allow; repo-shipped deny-only (PAR-03 letter) | ✓ |
| Deny-only everywhere | Stricter than CC and than the letter | |

**User's choice:** User-allow / project-deny (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| JSON + exit-2 both | CC verdict JSON and legacy deny both parse | ✓ |
| JSON only | Breaks existing exit-2 plugin hooks | |

**User's choice:** JSON + exit-2 both (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| All run, deny wins | Deterministic scope order; any deny beats any allow | ✓ |
| First-verdict short-circuit | Cheaper; hides lower hooks' verdicts | |

**User's choice:** All run, deny wins (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Full allow/deny/ask | 'ask' opens a dialog even ungated — escalation lever | ✓ |
| ask = no-op note | Documented divergence; no surprise dialogs | |

**User's choice:** Full allow/deny/ask (Recommended)

---

## AGENTS.md discovery set

Research basis: CC hierarchy — enterprise → user → project root, parent-chain walk, subdirectory on demand ([memory docs](https://code.claude.com/docs/en/memory), [.claude directory](https://code.claude.com/docs/en/claude-directory), [hierarchy](https://vineetagarwal-code-claude-code.mintlify.app/concepts/memory-context)).

| Option | Description | Selected |
|--------|-------------|----------|
| Both, CLAUDE.md same-dir | Both names; CLAUDE.md wins same-dir collisions | ✓ |
| Merge both | No winner; duplication risk | |
| AGENTS.md only | CC's own name unrecognized | |

**User's choice:** Both, CLAUDE.md same-dir (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| cwd→git-root + user | Walk up to repo boundary; no $HOME noise | ✓ |
| Root + user only | Minimal; misses subdir starts | |
| Full CC span | Walk-to-home + subdir-on-demand | |

**User's choice:** cwd→git-root + user (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| ~/.ass-guard only | Own convention | |
| ~/.claude only | Reads CC's file; other tool's home | |
| Both, home first | ~/.ass-guard wins on conflict | ✓ (operator choice) |

**User's choice:** Both, home first

| Option | Description | Selected |
|--------|-------------|----------|
| Cap + truncation note | Per-file + total budget; loud truncation | ✓ |
| No cap | Operator-authored trust content | |

**User's choice:** Cap + truncation note (Recommended)

---

## Rich content rules

Research basis: image limits 8000×8000, 10 MB direct API / 5 MB effective, ~1568 px recommendation ([vision docs](https://platform.claude.com/docs/en/build-with-claude/vision), [#19701](https://github.com/anthropics/claude-code/issues/19701)).

| Option | Description | Selected |
|--------|-------------|----------|
| Reject + note | Typed note naming limit; no image dep | |
| Auto-downscale | Fit the limit; original preserved; new dependency | ✓ (operator choice) |

**User's choice:** Auto-downscale — NOT reject; UX priority.

| Option | Description | Selected |
|--------|-------------|----------|
| Files + dir listing | @file content; @dir one-level listing | ✓ |
| Files only | @dir ordinary text | |

**User's choice:** Files + dir listing (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Drop + loud note | Block dropped, turn proceeds | ✓ |
| Reject prompt | Typed error; user removes image | |

**User's choice:** Drop + loud note (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Pure-Go, gate stands | CGO_ENABLED=0 unchanged; planner picks lib | ✓ |
| Allow cgo | Weakens the build gate | |

**User's choice:** Pure-Go, gate stands (Recommended)

---

## Thinking verification

Research basis: signature = tamper checksum; redacted_thinking byte-for-byte round-trip; thinking blocks preserved unmodified across tool turns ([thinking docs](https://platform.claude.com/docs/en/build-with-claude/thinking), [Bedrock](https://docs.aws.amazon.com/bedrock/latest/userguide/claude-messages-thinking-encryption.html)).

| Option | Description | Selected |
|--------|-------------|----------|
| Raw passthrough store | json.RawMessage exact bytes; identical by construction | ✓ |
| Typed block | Re-serialization drift risk | |

**User's choice:** Raw passthrough store (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Stream live + replay | agent_thought_chunk live and on session/load | ✓ |
| Wire-only | API round-trip but invisible in Zed | |

**User's choice:** Stream live + replay (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Captured goldens | Real wire pairs; drift fails loudly | ✓ |
| Synthetic only | No corpus dependency; misses shape drift | |

**User's choice:** Captured goldens (Recommended)

---

## Deferred Ideas

- Full CC memory span (walk-to-home + subdirectory-on-demand) — narrowed deliberately; revisit on monorepo pain.
- CLAUDE.local.md + import syntax — CC legacy surfaces; not carried.
