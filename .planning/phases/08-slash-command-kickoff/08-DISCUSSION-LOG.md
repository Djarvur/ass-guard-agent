# Phase 8: Slash-Command Kickoff - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-14
**Phase:** 8-slash-command-kickoff
**Areas discussed:** Expansion visibility, Command breadth, Skills invocation (user-added), WebSearch/WebFetch backends (user-added), Frontmatter directives, Tool-failure surfacing, Skill-listing surface & mimicry, Command-level boundaries, Chaining pattern source

---

## Scope additions (user free-text at area selection)

**User's input:** "to make the commands working we also need skills working, again in claude code compatible way. and we need websearch and webfetch tools at least for /opsx:explore"

**Outcome:** Two requirements added — CMD-06 (skills invocation) and CMD-07 (real web-tool backends). REQUIREMENTS.md and ROADMAP.md extended (21 → 23 REQ-IDs). Treated as in-phase prerequisites (the phase goal fails without them), not scope creep.

---

## Expansion visibility

| Option | Description | Selected |
|--------|-------------|----------|
| Typed command only | Editor shows what you typed; expansion invisible; matches Claude Code feel; provenance in transcript/audit | ✓ |
| Expanded body visible | Full markdown appears as your message — transparent but noisy | |
| Command + marker | Typed command plus one summary line ("expanded from opsx/explore.md, 2.1 KB") | |

**User's choice:** Typed command only
**Notes:** Transcript follow-up (separate question): the durable transcript records the **expanded body** as the user message with the typed command as provenance metadata — session/load replays exactly what the model saw.

---

## Command breadth

| Option | Description | Selected |
|--------|-------------|----------|
| Everything discovered | All `.claude/commands` (project+user) + `.ass-guard/commands`, existing precedence; no restriction code | ✓ |
| opsx-only | Narrowest path to the phase goal; others fall through as text | |
| All + allow/deny config | Everything plus a namespace allow/deny list | |

**User's choice:** Everything discovered

---

## Skills invocation (user-added area)

| Option | Description | Selected |
|--------|-------------|----------|
| Model-invoked Skill tool | Name+description in context; model calls Skill(name) when relevant; SKILL.md loads into the turn — Claude Code/zcode semantics | ✓ |
| Command-referenced auto-injection | Referenced skills injected up front when a command expands | |
| Both mechanisms | Skill tool + frontmatter skill references (heavier) | |

**User's choice:** Model-invoked Skill tool
**Notes:** Skill scope follow-up: **all discovered skills** exposed (consistent with command breadth). Listing-surface follow-up (second round): **profile shape + dynamic merge** — the captured zcode profile dictates the listing's shape; dynamic project skills slot in (the v1.0 dynamic-MCP-tools pattern). Never a synthetic listing.

---

## WebSearch/WebFetch backends (user-added area)

| Option | Description | Selected |
|--------|-------------|----------|
| DDG default | DDG-HTML scraping as shipped default (zero key, predecessor-proven) on the swappable seam | ✓ |
| Keyed API default + DDG fallback | Brave/Tavily-class default when configured | |
| Operator-configured only | No shipped default — breaks zero-config first run | |

**User's choice:** DDG default
**Notes:** WebFetch follow-up: **fetch + html→markdown** (JohannesKaufmann, in STACK research) — model gets clean markdown.

---

## Frontmatter directives

| Option | Description | Selected |
|--------|-------------|----------|
| Body-only | Parse frontmatter (description for matching/UX); don't act on allowed-tools/model in v1.1 | ✓ |
| Implement functional directives | allowed-tools toolset restriction + model/tier override now | |
| Strict zcode keys + warn | Recognize exactly zcode's 6 keys, warn on unknown | |

**User's choice:** Body-only

---

## Tool-failure surfacing

| Option | Description | Selected |
|--------|-------------|----------|
| Model-only | Structured error (exit code, stderr, classification) to the model + stderr diagnostics; model adapts/tells user | ✓ |
| Model + user-visible note | Plus session/update note for missing-binary/timeout classes | |
| Fail-loud halt | Missing-binary-class failures halt the turn | |

**User's choice:** Model-only

---

## Command-level boundaries (second-round area)

| Option | Description | Selected |
|--------|-------------|----------|
| Mirror tool mutability | Mutating commands (/opsx:apply, /opsx:archive) ALWAYS open a boundary at expansion; read-only never — D-15 discipline extended | ✓ |
| Tools only, not commands | Boundaries only on mutating TOOL calls; commands run in current context | |
| Config-declared | Per-command mutability in config with a sensible default | |

**User's choice:** Mirror tool mutability

---

## Chaining pattern source (second-round area)

| Option | Description | Selected |
|--------|-------------|----------|
| Seed from real output + learning extends | Default table seeded from REAL /opsx run output (captured at the phase's E2E gate); operator-overridable; learning handles unfamiliar handoffs | ✓ |
| Derive from command files | Parse opsx markdown for handoff phrasing — but patterns match model OUTPUT, not command input | |
| Learning-only | No seed; engine asks once per unfamiliar handoff (first runs interrupt hands-off promise) | |

**User's choice:** Seed from real output + learning extends

---

## Claude's Discretion

- ecosys loader mechanics (colon-join scheme, one-level scan implementation)
- Expand substitution engine internals; edge-case contract test design
- Adapter probe/reconciliation mechanics, timeout/exit-code classification details
- Where the skill listing slots into the profile's system blocks (decided by captured zcode shape, not invented)

## Deferred Ideas

- Functional frontmatter directives (allowed-tools, model/tier) — later enhancement
- Claude-Code command≡skill merge semantics (context: fork, stacking) — Future Requirements
- Keyed search APIs as configured backends — seam-supported, not shipped defaults
- Expanded OpenSpec profile E2E (new/continue/ff/verify/bulk-archive/onboard) — Future Requirements
