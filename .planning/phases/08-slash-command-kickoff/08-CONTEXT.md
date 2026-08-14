# Phase 8: Slash-Command Kickoff - Context

**Gathered:** 2026-08-14
**Status:** Ready for planning

<domain>
## Phase Boundary

A user types `/opsx:*` (or any discovered `/namespace:name` command) in Zed and ass-guard discovers, expands, and executes it — driving a real OpenSpec change (`/opsx:explore → propose → apply → archive`) end-to-end with zero manual continues, verified against the real `openspec` binary. This closes v1.0's major known gap (04-UAT.md) and is v1.1's product proof. Scope grew two user-added necessities during discussion: working claude-code-compatible **skill invocation** (commands lean on skills) and **real WebSearch/WebFetch backends** (at minimum for `/opsx:explore`) — recorded as CMD-06/CMD-07.

</domain>

<decisions>
## Implementation Decisions

### Expansion visibility & transcript (CMD-01/02, CMD-05)
- **D-01:** The editor shows the **typed command only** (`/opsx:explore fix-the-thing`); the expansion into the command's markdown body happens invisibly — matches the Claude Code user-side feel. *[User-selected.]*
- **D-02:** The **durable transcript records the expanded body as the user message**; the typed command rides as provenance metadata (which command file drove the turn — satisfies CMD-05). Rationale: `session/load` replay fidelity — a replayed session shows the model exactly what it saw. *[User-selected.]*

### Command & skill breadth
- **D-03:** **Everything discovered is invocable** — all `.claude/commands/` (project + user) and `.ass-guard/commands/` under the existing precedence; no restriction/allow-deny machinery in v1.1. *[User-selected.]*
- **D-04:** **All discovered skills are exposed** (`.claude/skills/` + `.ass-guard/skills/`, existing precedence) — consistent with D-03. *[User-selected.]*
- **D-05:** Skills activate via the **model-invoked Skill tool** (Claude Code/zcode semantics): skill name + description surfaced in the model's context; the model calls `Skill(name)` when relevant; the skill's SKILL.md loads into the turn. The `/opsx` prompt content naturally triggers the matching `openspec-*` skill. No command-referenced auto-injection. *[User-selected — the "claude-code-compatible way" the user required.]*
- **D-06:** The skill listing reaches the model's context via **profile shape + dynamic merge**: the captured zcode profile dictates the listing's shape/placement/format; dynamically discovered project skills slot into that shape — the same pattern v1.0 used to merge dynamic MCP tools into the captured catalog (Phase-5 D-01). Never a synthetic ass-guard-invented listing. *[User-selected.]*

### Web tools (CMD-07)
- **D-07:** WebSearch ships a **real DDG-HTML default backend** (zero API key, predecessor-proven pattern) on the existing swappable-Backend seam; keyed APIs remain config-swappable. Zero-config first run keeps working. *[User-selected.]*
- **D-08:** WebFetch returns **fetch + html→markdown** (JohannesKaufmann/html-to-markdown — already in STACK research) — the model gets clean markdown, like Claude Code's WebFetch. *[User-selected.]*

### Frontmatter & failures
- **D-09:** Command frontmatter is **body-only for v1.1**: parse frontmatter (description matters for skill-matching/UX) but do NOT act on `allowed-tools`/`model` functionally — functional directives are a later enhancement. *[User-selected.]*
- **D-10:** `openspec:*` tool failures surface as a **structured error to the model** (exit code, stderr, classification) + diagnostics to the stderr log; the model adapts/retries/tells the user in prose. No user-visible session/update note, no fail-loud halt. *[User-selected.]*

### Context boundaries & chaining
- **D-11:** Slash-commands **mirror the tool-mutability table** (v1.0 D-15 discipline extended to commands): mutating commands (`/opsx:apply`, `/opsx:archive`) ALWAYS open a context boundary at expansion time; read-only ones never do. *[User-selected.]*
- **D-12:** Zero-continue chaining patterns are **seeded from REAL `/opsx` run output** — captured during this phase's real-binary E2E gate (patterns must match the model's actual stage-end output, not the command files' input text) — operator-overridable, with the existing learning mode extending for unfamiliar handoffs. *[User-selected — also the "no stub-only evidence" rule applied to patterns.]*

### Claude's Discretion
- Exact ecosys loader changes (colon-join key scheme, one-level scan implementation), the Expand substitution engine internals, adapter probe/reconciliation mechanics, timeout/exit-code classification details — all planner/researcher territory per the research docs.
- Where the skill listing slots into the profile's system blocks — decided by the captured zcode shape (ground truth in `profiles/zcode/`), not invented.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### v1.1 research (ground truth, 2026-08-14 — verified against source + live binary)
- `.planning/research/SUMMARY.md` — synthesized findings, phase structure, cross-cutting invariants
- `.planning/research/STACK.md` — installed-binary surface table (probe-pinned), zcode substitution semantics, frontmatter sufficiency (yaml.v3), DDG/STT backend details
- `.planning/research/FEATURES.md` — Claude-Code-vs-zcode command semantics divergence (zcode wins), OpenSpec command/skill inventory, comparables
- `.planning/research/ARCHITECTURE.md` — integration points (turn-runner hook, no-Execute gap, factory seam), build order
- `.planning/research/PITFALLS.md` — 18 phase-mapped pitfalls (flat-scan blocker, injection guards, substitution contract, version-drift trap)

### The gap this phase closes
- `.planning/milestones/v1.0-phases/04-unified-engine-hook-dag-openspec-learning/04-UAT.md` — full root-cause diagnosis (ecosys unwired, adapter model mismatch, 11 deferred checks)

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` §"Slash-Command Kickoff" — CMD-01..07
- `.planning/ROADMAP.md` §"Phase 8" — goal, success criteria, phase gate

### Prior-phase decisions that bind
- `.planning/milestones/v1.0-phases/05-ecosystem-compatibility/05-CONTEXT.md` — D-05 (`.claude/` read-only, `.ass-guard/` additions), D-06 (precedence chain), D-01 (dynamic-merge pattern D-06 here generalizes)
- `.planning/milestones/v1.0-phases/04-unified-engine-hook-dag-openspec-learning/04-CONTEXT.md` — D-02 (dual-signal), D-03 (structural safety), D-14 (openspec TOML), D-15 (mutability table)

### Code ground truth
- `internal/ecosys/loader.go` — the flat-scan `discoverCommands` (first task: subdirectory + colon-join)
- `internal/openspec/{adapter.go,register.go,seeded.toml}` — Adapter (subprocess Run), RegisterTools (no-Execute gap), stale command set
- `cmd/ass-guard/acp_serve.go` — `sessionTurnRunner.Run` (the expansion hook point, before `runOneTurn`)
- `profiles/zcode/` — captured profile (system blocks shape for D-06's listing merge)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/ecosys` — discovery + precedence chain already implemented (skills AND commands); needs subdirectory scan + richer frontmatter + pure `Expand`; this phase becomes its first non-test importer
- `internal/openspec.Adapter` — subprocess Run with stdout/stderr capture; needs Execute wiring + probe-pinned command set
- `internal/toolexec` — swappable Backend seam for WebSearch/WebFetch (stub-proven); DDG backend drops in
- `internal/engine` + pattern table — dual-signal chaining already proven; re-seed patterns from real output
- `internal/toolcat` — catalog registry; Skill tool exists in the 103-tool set

### Established Patterns
- Dynamic-merge-into-captured-shape (v1.0 MCP tools → v1.1 skills listing, D-06)
- `.claude/` strictly read-only; `.ass-guard/` writes only
- Declarative config (TOML for openspec, YAML for hooks/scheduling) — config-not-code
- Phase gate = `mise ci` + real-dependency evidence; no stub-only closure

### Integration Points
- Expansion hooks at `sessionTurnRunner.Run` (session layer) — NOT `internal/acp.handleSessionPrompt`; surface-agnosticity is why Telegram (Phase 10) inherits `/opsx:*` free
- Boundary engine: command mutability rides the existing toolcat boundary path (mutating command → boundary at expansion, D-11)
- Transcripts: expanded-body-as-user-message per D-02 (two-layer context model unchanged)

</code_context>

<specifics>
## Specific Ideas

- User's framing for the two scope additions: "to make the commands working we also need skills working, again in claude code compatible way. and we need websearch and webfetch tools at least for /opsx:explore" — skills and web tools are prerequisites, not enhancements; `/opsx:explore` is the minimum bar the phase must clear live.
- zcode semantics win over Claude Code wherever they differ (`` !`cmd` `` dynamic shell rejected, `${ARGUMENTS}` braces unrecognized, out-of-range `$N` → empty) — the mimicry target is the contract (research: zcode-guide/diagnosing-commands skill is the ground truth).

</specifics>

<deferred>
## Deferred Ideas

- Functional frontmatter directives (`allowed-tools` toolset restriction, `model`/tier override) — later enhancement beyond v1.1 (D-09)
- Claude-Code command≡skill merge semantics (`context: fork`, stacking) — Future Requirements (REQUIREMENTS.md)
- Keyed search APIs (Brave/Tavily) as configured backends — supported by the seam, not shipped as defaults
- Expanded OpenSpec profile E2E (new/continue/ff/verify/bulk-archive/onboard) — Future Requirements

</deferred>

---

*Phase: 8-Slash-Command Kickoff*
*Context gathered: 2026-08-14*
