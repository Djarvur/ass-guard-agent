# ass-guard-agent (working name)

## What This Is

A Go-based AI coding agent that makes SDD (Spec-Driven Development) workflows run hands-off by automatically doing the routine work a developer keeps forgetting to ask for. It speaks to model providers with requests structured like another agent (the first mimicry target is zcode — a claude-code-compat runtime shipping its own `AGENTS.md`, skills, commands, MCP, and tool catalog), so the model behaves identically to how it behaves in the mimicked agent. The agent hosts unmodified SDD toolkits (OpenSpec in v1), drives them to completion without manual "continue" taps, and runs configurable post-stage routines (review / memory / tests / linters / improvement proposals) on its own. Primary interface is ACP (IDE-native, e.g. Zed); Telegram is a full peer surface (text + voice). Built for SDD-capable teams who want the toolkit to just run.

## Core Value

Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first) — if the model can tell the requests apart, everything built on top is compromised, because model behavior diverges. Every other capability (autocontinue, hooks, scheduling, interfaces) is downstream of this.

## Business Context

- **Customer**: SDD-practicing engineering teams (and the author's own SDD workflow as the first instance)
- **Revenue model**: Undecided (open-source team tooling; possible hosted/managed later)
- **Success metric**: A team can install ass-guard via ACP registry, run an unmodified OpenSpec workflow, and never tap "continue" or remember to ask for review/tests/lint/memory — the agent does the forgotten routine automatically
- **Strategy notes**: Successor to `sdd-acp-agent` (closed in favor of this project). The predecessor's research, architecture spine, and specs survive as reference; its Go code does not carry over — ass-guard is built fresh against its own scope.

## Requirements

### Validated

- Model scheduling layer (Phase 3 — Model Scheduling): tier abstraction (heavy/good/light → concrete provider+model), time-windowed substitution with IANA-zone support + bundled tzdata, per-project override (D-02 precedence: time-window → project → global), typed ProviderError classification (Transient/Structural) driving an explicit fallback chain, circuit breakers (consecutive + error-rate, D-07) and a dollars-per-window cost ceiling with degrade-then-stop (D-08), and structured capability profiles with load-time + request-time mismatch enforcement (D-09/D-10). Validated by `internal/scheduler` (config/resolver/dispatch/breaker/cost/capability) + `internal/provider/errors.go` + the `ass-guard scheduling validate|resolve` CLI.

### Active

**Mimicry (north star — must work first)**

- [ ] Outgoing model requests are structurally indistinguishable from the active profile's target agent (zcode first): message hierarchy, tool catalog (names + schemas), identity/system prompt structure, and significant fields/headers all match
- [ ] Profile = a configurable bundle of {system prompts, tool catalog, message shape, identity}; zcode is the first profile, architecture supports N profiles from day one
- [ ] Profile content is extracted from the target agent's on-disk logs (grounded, not guessed)

**Model provider layer**

- [ ] Supports both Anthropic-shape and OpenAI-shape protocols, with any compatible provider via configurable base URL
- [x] Model tiers (heavy/good/light ≈ opus/sonnet/haiku) abstract the concrete model; command/skill/subagent selects a tier — *Validated in Phase 3 (SCHED-01)*
- [x] Tier→model mapping is time-scheduled (e.g. heavy is glm-5.2 normally, minimax-m3 in peak hours) — *Validated in Phase 3 (SCHED-02)*
- [x] Per-project override of the tier→model table (falls back to config default if unset) — *Validated in Phase 3 (SCHED-03)*
- [x] Fallback chains on provider error/limit (degrade tier, or walk a configured chain — not just a single default) — *Validated in Phase 3 (SCHED-04/05)*

**Agent ecosystem compatibility**

- [ ] Claude Code drop-in: plugins, skills, MCP servers, and slash-commands installed for Claude Code work unchanged in ass-guard
- [ ] Reuses Claude Code's `.claude/` config layout (settings, CLAUDE.md/AGENTS.md hierarchy); ass-guard's additions namespace cleanly, never clobbering Claude Code's files

**Tooling**

- [ ] Built-in tool catalog matching the active profile's tool set (claude-code-compat catalog as the baseline), each tool's call/result shape faithful to the reference
- [ ] Complex tools (e.g. WebSearch) use a configurable backend, not a hardcoded one
- [ ] Read-only tools parallelize within a turn; mutating tools serialize relative to each other

**Unified engine (autocontinue is the upper mechanism; hooks are a special case)**

- [ ] After turn-complete, a single engine decides: continue the SDD scenario (dual-signal — text-pattern OR known handoff tool-call), trigger a hook-DAG, ask the user, or wait
- [ ] SDD scenarios run to completion with zero manual "continue" taps, except where the toolkit explicitly requests user input
- [ ] Unmatched output triggers nothing — the structural safety property; the only off-switch is manual cancellation, which drains queued injections
- [ ] Learning mode: when the engine doesn't know how to launch what should be launched, it asks (fresh context? wait for confirmation? how long?) and remembers the answer
- [ ] Learning mode proposes new hooks based on the work log and asks the user whether to add them

**Hook-DAG (the "forgotten routine")**

- [ ] Configurable DAG of arbitrary steps (run command / send prompt / fresh context / wait) in any order — fully customizable per stage
- [ ] Seeded hook set out of the box (post-implement: test + lint + review + memory; post-phase: improvement proposals) for zero-config first run

**Context hygiene**

- [ ] Two-layer model: durable replayable transcript (ACP-visible) + lean projected window (model-visible), reset at command boundaries
- [ ] Mutating toolkit commands are always boundaries (cannot be removed by config); config may only add boundaries

**Logging**

- [ ] Full audit log: user input, model requests, tool calls, and everything needed to reconstruct the exact sequence of actions

**Parallelism**

- [ ] `Task`/`Agent` tool dispatches subagents as isolated goroutine turn-loops with scoped context and a restricted tool subset

**Interfaces**

- [ ] ACP v1 server over stdio JSON-RPC is the primary interface (IDE-native; spawned by the editor as a subprocess)
- [ ] Telegram is a secondary but full peer: can drive an entire SDD scenario (text and voice); voice messages are transcribed to text as ordinary user input via a configurable STT backend

**Toolkits**

- [ ] v1 hosts OpenSpec, consumed unmodified (per-toolkit pattern/handoff config; GSD / spec-kit / BMad accommodated by the adapter interface but deferred)

**Distribution**

- [ ] Single static Go binary via goreleaser (macOS + Linux, amd64 + arm64); ACP registry manifest for one-shot install

### Out of Scope

- Windows platform support — v1 macOS+Linux only; Windows deferred (win-developer teams are not the first target)
- GSD / spec-kit / BMad toolkit adapters in v1 — OpenSpec only; interface accommodates them, implementation deferred
- Perplexity or other paid search backends — configurable backend design replaces the hardcode, but no specific paid integrations are committed
- Byte-for-byte request identity with the mimicked agent — "structurally indistinguishable to the model" is the bar, not binary diff equality (cosmetic field ordering/optional fields allowed)
- A standalone CLI surface — ACP (IDE) and Telegram are the only interfaces; no terminal REPL to maintain
- A confirmation/permission tier for tool execution — tools run ungated; the pattern/hook table + manual cancellation is the safety mechanism (inherited from predecessor)
- Porting code from `sdd-acp-agent` — fresh build; predecessor is reference-only

## Context

**Predecessor — `sdd-acp-agent`.** A Go-based SDD-toolkit host with ACP UI, Claude-Code-compatible tooling, and pattern-matching autocontinue. Closed in favor of ass-guard. Its planning artifacts (technical research, architecture spine with 11 architectural decisions, epic breakdown with 5 epics/27 stories, log analysis of tool catalog and system prompts) are first-class reference material and live at `/Users/nil/DiskD/W/Djarvur/sdd-acp-agent`. The predecessor validated several load-bearing facts that carry forward as given:

- ACP is JSON-RPC 2.0 over stdio; the editor spawns the agent as a subprocess; stdout is reserved for protocol frames, all logging goes to stderr
- The model-provider layer collapses to two API shapes (Anthropic-shape covering GLM via Z.ai's compatible endpoint; OpenAI-shape covering MiniMax M3 and others) — per-command routing is a config concern, not an architecture
- Context-drop is resolved by a two-layer model (durable replayable transcript vs. lean projected window) — satisfies ACP's replay-on-load contract while letting each command start clean
- Parallel subagents map cleanly to goroutine turn-loops with isolated context
- The autocontinue engine is an observer on an internal event bus, not in a turn's critical path — graceful degradation is structural

**What ass-guard adds beyond the predecessor.** Six deltas, each load-bearing for the new scope:

1. **Mimicry as the north star** — the predecessor had Claude Code *compatibility*; ass-guard makes structural indistinguishability from a configurable target agent (zcode first) the primary success criterion
2. **Multi-tier model scheduling** — heavy/good/light abstraction with time-windowed substitution and per-project overrides, broader than the predecessor's flat per-command routing
3. **Configurable tool backends** — generalizing the predecessor's hardcoded DDG search into configurable backends
4. **Learning mode** — the autocontinue engine asks and remembers how to handle unfamiliar launch situations (fresh context? wait? how long?), and proposes new hooks from the work log
5. **Unified engine** — autocontinue and hooks collapse into one post-turn-complete decision engine (hooks are a special case), simplifying the predecessor's two-mechanism design
6. **Telegram peer** — a second full interface (text + voice) the predecessor explicitly did not have

**Mimicry reference — zcode.** zcode is a claude-code-compat runtime that ships its own `AGENTS.md`, skills, slash-commands, MCP integrations, and a built-in tool catalog. Its on-disk logs (to be located during Phase 1) are the source of truth for the zcode profile: system prompts, tool catalog (names + schemas), message shape, and identity fields. The profile is then expressed as ass-guard config.

**Author's SDD practice.** The author runs OpenSpec, GSD, BMad, and spec-kit workflows and routinely forgets to invoke the routine work between stages (review, memory updates, tests, linters, improvement proposals). The hook-DAG + seeded set directly addresses this — it is the project's reason to exist for the author personally.

## Constraints

- **Tech stack**: Go (single static binary; first-class concurrency for parallel subagents; mature streaming HTTP clients) — load-bearing, not stylistic
- **Transport discipline**: stdout reserved exclusively for ACP JSON-RPC frames; all logging/diagnostics to stderr (non-negotiable LSP-style discipline)
- **Compatibility**: Claude Code config layout drop-in (existing `.claude/` setups work unchanged)
- **Distribution**: Static binary via goreleaser; ACP registry manifest; no daemon, no network port (the editor owns process lifecycle)
- **Platform**: macOS + Linux, amd64 + arm64 — Windows deferred
- **Safety model**: No tool-execution confirmation tier; the pattern/hook table (no match → nothing runs) plus manual cancellation is the only safety mechanism
- **Investigate-and-fix-ready logging (must-have)**: ass-guard must log every problem it encounters — errors, panics, failed tool calls, provider failures, engine misfires, unexpected states — in a form detailed enough that a developer reading the transcript can diagnose the root cause and fix it. Not just "an error occurred" but the context, the inputs, the failure point, and the recoverable/non-recoverable classification. The transcript is the primary diagnostic surface (corollary of Phase 2 D-03/D-20: transcript = human-investigation artifact); every problem must be investigate-able from the transcript alone.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Fresh build; `sdd-acp-agent` is reference-only (no code port) | Predecessor's scope was narrower; its code carries decisions that don't fit the mimicry + scheduling + hooks + Telegram scope. Building fresh against the new scope is cleaner than retrofitting. | — Pending |
| Mimicry is the north star, not a feature | Model behavior diverges if requests aren't structurally identical; everything downstream (forgotten-routine hooks, autocontinue, scheduling) is meaningless if the model doesn't behave as in the target agent. Validated first, on the thinnest possible stack. | — Pending |
| Autocontinue is the upper mechanism; hooks are a special case | Collapses two predecessor ideas into one decision point after turn-complete, avoiding a two-mechanism design and unifying learning under one engine. | — Pending |
| Profiles are config (zcode first, architecture for N) | "Mimic under any agent" is an explicit goal; baking zcode as the only profile would force a rewrite later. | — Pending |
| Profile content is log-extracted, not hand-written | Grounded mimicry: the only honest source of "what does zcode actually send" is zcode's own request logs. Hand-written profiles are guesses. | — Pending |
| ACP primary, Telegram secondary-but-peer | ACP is the IDE-native working surface for code; Telegram extends reach (mobile, voice, async) without duplicating the IDE experience. Both are full-capability fronts to one core. | — Pending |
| v1 toolkits = OpenSpec only | OpenSpec has the deepest reference (predecessor's ccr-log, handoff examples, pattern research); GSD/spec-kit/BMad accommodated by the adapter interface, implemented later. | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-08-11 after Phase 3 (Model Scheduling) completion*
