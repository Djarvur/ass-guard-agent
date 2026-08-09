# Feature Research

**Domain:** SDD-hosting AI coding agent (Go; ACP + Telegram; OpenSpec host; profile-based mimicry)
**Researched:** 2026-08-09
**Confidence:** HIGH (2026 web-verified across Claude Code, zcode/claude-code-compat ecosystem, OpenCode, Codex CLI, Cursor, Aider, the SDD toolkit family, and the ACP standard)

> Scope note: this document inventories the 2026 AI-coding-agent / SDD-automation feature landscape against ass-guard's stated scope. Each feature is tagged **INHERITED** (predecessor `sdd-acp-agent` had it — 5 epics / 27 stories) or **NEW** (one of ass-guard's six deltas). Complexity is implementation complexity for ass-guard specifically (Go, single binary, ACP-stdio-Telegram), not generic difficulty. Where a 2026 source informs the call, it's cited inline.

---

## Feature Landscape

The 2026 AI-coding-agent market has converged on a recognizable core: multi-provider model routing with fallback, a built-in file/shell tool catalog, hooks/lifecycle events, subagents, an AGENTS.md/CLAUDE.md instruction layer, skills/slash-commands, and some form of memory. Claude Code is the de-facto reference everyone targets ("claude-code-compat" is now a product category, not a marketing phrase). ACP ("LSP for coding agents," JSON-RPC 2.0 over stdio, backed by Zed + Google since Aug 2025) is the editor-native integration standard. SDD toolkits (OpenSpec, spec-kit, BMad, GSD) have proliferated and are explicitly designed to be driven *unmodified* by a coding agent — making "host and auto-drive a toolkit" a real, well-defined surface rather than a custom integration each time.

What is **not** yet table stakes, and where ass-guard's differentiators land:
- **Mimicry** (structural indistinguishability from a named target agent's outgoing requests) is essentially unoccupied as a product category. "claude-code-compat" exists, but it means "accepts the same config/plugins/skills" — not "the model cannot tell our requests apart from the mimicked agent's." That is ass-guard's north star and its sharpest differentiator.
- **Hands-off SDD autocontinue** (zero "continue" taps, toolkit drives to completion) is the predecessor's reason-to-exist and remains rare — most agents are manual-continue, with hooks filling specific lifecycle slots rather than driving an entire multi-stage workflow.
- **Telegram-as-full-peer + voice** for a *coding* agent is unusual. Claude Code Channels (shipped March 2026) added Telegram/Discord *messaging into a running session*, but as an adjunct, not a peer that can itself drive a full SDD scenario.
- **Multi-tier model scheduling with time-windows + fallback chains** is partially covered by routers (OpenCode's 75+ providers, model-router-hook, Omniroute), but time-windowed substitution and per-project overrides are not standard.

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete. Most of these are **INHERITED** from the predecessor or are baseline 2026 agent expectations.

| Feature | Why Expected | Complexity | Inherited/New | Notes |
|---------|--------------|------------|---------------|-------|
| Built-in tool catalog (Bash, Read, Edit, Write, Glob, Grep) | 2026's stock agent toolset is bash/read/write/edit + search; Cloudflare's managed agents, Anthropic's optimization notes, and the Nova Bridge (24 tools as of SDK 0.17.0, May 2026) all confirm this core. Claude Code ships ~25 JSON-schema tools (~52 KB inline payload before Anthropic's 2026 payload optimization). | MEDIUM | INHERITED (FR6, E2) | Must match the active profile's catalog name-for-name, schema-for-schema. zcode profile = claude-code-compat catalog. Source of truth = extracted from target agent logs, not hand-written. |
| ACP v1 server (stdio JSON-RPC 2.0) | ACP is the editor-native standard ("LSP for coding agents"), backed by Zed + Google since Aug 2025; Zed, JetBrains, Obsidian are clients. An SDD agent without ACP is not IDE-native. | MEDIUM | INHERITED (FR1–FR3, E1) | stdout reserved for protocol frames, all logs to stderr (non-negotiable LSP-style discipline). `session/prompt`, streamed `session/update`, `session/load` replay are the minimum lifecycle. |
| Multi-provider model layer (Anthropic-shape + OpenAI-shape) | Every serious 2026 agent supports multiple providers. OpenCode advertises "75+ providers out of the box"; Codex CLI's `config.toml` lets you add any custom provider. | MEDIUM | INHERITED (FR5, E1) | Two API shapes cover the field: Anthropic-shape (Z.ai/GLM via compat endpoint) and OpenAI-shape (MiniMax M3, others). Configurable base URL is table stakes. |
| Model routing per task + fallback on error | Per-task routing (explore vs apply, different models) is a 2026 table-stakes pattern — Augment Code, Duet, MindStudio all publish routing guides (Opus/Sonnet/Haiku/GPT-5.2 by task type). Single-model fallback on provider error is baseline. | MEDIUM | INHERITED (FR5, E1 Story 1.5) — but the *tier abstraction* + *time-windows* + *chains* layers are NEW (see differentiators). | ass-guard's per-command routing is the predecessor's baseline; the upgrade is tier abstraction + time-windowed substitution + multi-step fallback chains. |
| AGENTS.md / CLAUDE.md instruction hierarchy | AGENTS.md is the 2026 vendor-neutral standard for always-on project context (in the system prompt). Claude Code, Codex, Copilot, Cursor all read it. | LOW | INHERITED (FR7/FR8, E2) | Drop-in Claude Code `.claude/` layout (settings.json + CLAUDE.md hierarchy); ass-guard namespaces under `.claude/sdd/` and never clobbers. Content always flows through config — never hardcoded in Go. |
| Skills + slash-commands drop-in | Claude Code skills/commands are the de-facto extension format; "claude-code-compat skills" under `.claude/skills/*/SKILL.md` and `.agents/skills/` are a recognized open standard. SDD toolkits (OpenSpec) ship as slash-commands (`/opsx:propose`, etc.). | MEDIUM | INHERITED (FR7, E2) | Claude Code plugins/skills/MCP/slash-commands installed for Claude Code must work unchanged in ass-guard. |
| Lifecycle hooks (PreToolUse, PostToolUse, Stop, SubagentStop, etc.) | Claude Code hooks are JSON-configured shell commands firing on lifecycle events; the set grew from ~12 to ~30 events by 2026. Auto-format/lint/test-on-edit and stop-hook code review are documented patterns. | HIGH | PARTIAL — the predecessor had a *dual-signal autocontinue engine* and *per-toolkit pattern tables*; ass-guard's NEW contribution is the **unified engine** collapsing autocontinue + hooks into one post-turn-complete decision point (hooks are a special case). | The unified engine is NEW (delta 5); hook-DAG execution as a concept is informed by 2026's hook-rich ecosystem but the DAG generalization is ass-guard's. |
| Subagents / parallel exploration | Subagents spawning isolated workers for parallel tasks are mainstream (Claude Code, Codex multi-agent with Agents SDK + MCP, Cursor background agents). | HIGH | INHERITED (FR17/FR18, E4) | `Task`/`Agent` tool → goroutine turn-loops with scoped context + restricted tool subset; panic-recovered → tool-error; provider concurrency bounded by semaphore. |
| Context hygiene (window reset at command boundaries) | Long-running SDD workflows fill context; 2026 agents handle this variously (Cursor Composer, Claude Code compaction). Two-layer (durable replayable transcript + lean projected window) is more disciplined than most. | HIGH | INHERITED (FR13/FR14, E4) | ass-guard's specific design: mutating toolkit commands are ALWAYS boundaries (cannot be removed by config); config may only ADD boundaries. Satisfies ACP's replay-on-load contract. |
| Session durability + replay on restart | ACP `session/load` replay is a contract, not optional. Transcript-as-replay is standard (TraceTrail, Gryph build businesses on JSONL transcript replay in 2026). | MEDIUM | INHERITED (FR3, E1 Story 1.6) | Durable transcript is the single source rehydrated on restart; boundary markers authoritative on rehydration. |
| Audit logging (requests, tool calls, actions) | 2026 enterprise/compliance discourse (Northflank, LoginRadius, Collibra, Nylas) treats structured per-action audit logs as baseline; Gryph/TraceTrail show replay-from-logs is a real capability. | MEDIUM | INHERITED (PROJECT.md Logging requirement) | ass-guard logs user input + model requests + tool calls + everything needed to reconstruct the exact action sequence. |
| Web tools (search + fetch) | Web grounding is table stakes; OpenCode, Codex, Cursor all have it. | LOW | INHERITED (FR9, E5) — but the *configurable backend* is NEW (delta 3). | Predecessor hardcoded DDG; ass-guard generalizes to a configurable backend (no hardcoded provider; no paid-backend commitment in v1). |
| Static-binary distribution via goreleaser | Single-binary, no daemon, editor-owned lifecycle is the ACP-spawned-agent norm. | LOW | INHERITED (NFR1/NFR10, E5) | macOS + Linux, amd64 + arm64; ACP registry `agent.json` manifest for one-shot install. |

### Differentiators (Competitive Advantage)

Features that set ass-guard apart. Not required by the market, but valuable — and they map directly to the six deltas in PROJECT.md. ass-guard should compete here, not on table stakes.

| Feature | Value Proposition | Complexity | Inherited/New | Notes |
|---------|-------------------|------------|---------------|-------|
| **Agent mimicry via profiles** (north star) | Outgoing model requests are structurally indistinguishable from the configured target agent (zcode first) — message hierarchy, tool catalog (names + schemas), identity/system prompt structure, significant fields/headers. The model behaves identically to how it behaves in the mimicked agent. No 2026 product occupies this: "claude-code-compat" means accepts-the-same-config, not indistinguishable-to-the-model. | HIGH | NEW (delta 1) | This is the load-bearing differentiator — everything downstream is meaningless if mimicry fails. Profile = configurable bundle {system prompts, tool catalog, message shape, identity}. Architecture supports N profiles from day one (zcode is first). **Profile content is log-extracted, not hand-written** — the only honest source. Requires: target-agent log capture, a profile extraction tool, a request-shape conformance test. |
| **Hands-off SDD autocontinue** (zero continue-taps) | Run an unmodified OpenSpec workflow end-to-end and never tap "continue" unless the toolkit explicitly requests input. Most 2026 agents are manual-continue; hooks fill specific slots rather than driving an entire multi-stage workflow. This is the predecessor's reason-to-exist and ass-guard carries it forward. | HIGH | INHERITED (FR10–FR12/FR20, E3) — the *unified-engine* framing is NEW (delta 5). | Dual-signal detection: text-pattern OR known-handoff-tool-call (whichever first). Grounded in the ccr-log finding that OpenSpec handoffs happen via Skill tool-calls, not literal text markers. Structural safety: unmatched output triggers nothing; only off-switch is manual cancellation (drains queued injections). Validated first on the thinnest possible stack. |
| **Multi-tier model scheduling with time-windows + fallback chains** | heavy/good/light (≈opus/sonnet/haiku) abstracts the concrete model; command/skill/subagent selects a tier; tier→model mapping is *time-scheduled* (e.g. heavy = glm-5.2 normally, minimax-m3 in peak hours); per-project override; fallback *chains* (degrade tier or walk a configured chain, not a single default). Time-windowed substitution and per-project overrides are not standard in 2026 routers. | HIGH | NEW (delta 2) | 2026 routers (model-router-hook, Omniroute, OpenCode) do per-task routing + single fallback. ass-guard adds: tier abstraction (decouple command intent from concrete model), time-windowed substitution (cost/capacity steering), multi-step fallback chains, per-project override of the tier→model table. Depends on the two-shape provider layer. |
| **Unified post-turn engine** (autocontinue is upper mechanism; hooks are special case) | One decision point after turn-complete decides: continue the SDD scenario, trigger a hook-DAG, ask the user, or wait. Collapses the predecessor's two-mechanism design and unifies learning under one engine. Simpler, more observable, one place to extend. | HIGH | NEW (delta 5) | Generalizes Claude Code's lifecycle hooks (which fire at fixed events) into a single post-turn-complete decision engine where hooks are a special case. Enables the learning mode to plug into the same engine. |
| **Hook-DAG (the "forgotten routine")** with seeded set | Configurable DAG of arbitrary steps (run command / send prompt / fresh context / wait) in any order, per stage. Seeded out of the box (post-implement: test + lint + review + memory; post-phase: improvement proposals) for zero-config first run. Addresses "things the user forgets to ask for." | HIGH | NEW (delta 4/5) | 2026 has rich hook ecosystems (Claude Code ~30 events; community stop-hook code reviewers; Copilot auto-PR-review), but they are flat event→action mappings, not a configurable ordered DAG per SDD stage. The seeded set is what makes it zero-config. This is the project's reason to exist for the author personally. |
| **Learning mode** (ask-and-remember + propose hooks) | When the engine doesn't know how to launch what should be launched, it asks (fresh context? wait for confirmation? how long?) and remembers the answer. Proposes new hooks based on the work log and asks the user whether to add them. | HIGH | NEW (delta 4) | 2026 "adaptive agent" memory (Cortex, Mem0, AgeMem's store/retrieve/update/summarize/discard) is broader but focused on *recall*. ass-guard's learning mode is narrower and sharper: it learns *launch policies* (when to inject what) and *hook proposals*, not general facts. Closer in spirit to a memlog of engine decisions than a vector store. Depends on the unified engine + hook-DAG. |
| **Configurable tool backends** | Complex tools (e.g. WebSearch) use a configurable backend, not a hardcoded one. Generalizes the predecessor's hardcoded DDG. | MEDIUM | NEW (delta 3) | 2026 agents either hardcode a provider (Claude Code's web tool, Aider's scraper) or route through a gateway (OpenRouter). ass-guard's per-tool backend config lets a team pick their search/fetch backend without code changes. |
| **Telegram peer interface (text + voice)** | Telegram is a secondary but full peer: can drive an entire SDD scenario (text and voice); voice messages transcribed to text as ordinary user input via a configurable STT backend. | HIGH | NEW (delta 6) | Claude Code Channels (March 2026) added Telegram/Discord *messaging into a running session* — but as an adjunct surface, not a peer that itself drives a full workflow. ass-guard's Telegram is a full-capability front to one core (drives an entire SDD scenario). Voice-as-ordinary-input via configurable STT (Whisper-v3-class; many 2026 STT options) is uncommon for coding agents. ACP remains primary (IDE-native for code); Telegram extends reach (mobile, voice, async). |
| **Tool read-only parallelism / mutating serialization** | Read-only tools parallelize within a turn; mutating tools serialize relative to each other. | MEDIUM | INHERITED (FR18, E2 Story 2.3) | Most 2026 agents parallelize reads; few document the mutating-serialization discipline explicitly. ass-guard makes it a structural invariant (each tool carries a `Mutability`; effective mutability = more-mutating of adapter-class and tool-Mutability). |
| **Profile content log-extracted, not hand-written** | Profile source of truth = the target agent's on-disk request logs (grounded mimicry, not guesses). | MEDIUM | NEW (delta 1, operational) | This is a property of the mimicry feature but worth calling out: it is the *method* that makes mimicry honest. Requires a log-capture/extraction step per profile. No 2026 competitor does log-grounded profile extraction because no competitor does mimicry. |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but would compromise ass-guard's core value or scope. Documented to prevent scope creep. Several are already explicitly out-of-scope in PROJECT.md.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Byte-for-byte request identity with the mimicked agent | "If structural indistinguishability is the bar, why not exact equality?" | Binary-diff equality is brittle (cosmetic field ordering, optional fields, timestamp/nonce fields) and gains nothing — the *model* can't tell them apart structurally, which is the actual success criterion. Chasing byte equality wastes effort and breaks on harmless upstream variance. | "Structurally indistinguishable to the model" is the bar. A request-shape conformance test (structural, not byte) is the verification. (PROJECT.md Out of Scope.) |
| A standalone CLI / terminal REPL surface | "Every coding agent has a CLI." | A terminal REPL is a third interface to maintain alongside ACP + Telegram. It duplicates the IDE experience without adding reach (unlike Telegram's mobile/voice) and splits testing surface. The ACP-stdio server is already the headless core; a REPL wraps it for no net gain. | ACP (IDE) + Telegram (mobile/voice/async) are the only interfaces. The ACP server is the headless core; no separate REPL. (PROJECT.md Out of Scope.) |
| A confirmation/permission tier for tool execution | "Tools should ask before running (write gating, bash approval)." | A confirmation tier breaks the hands-off success bar (zero continue-taps becomes impossible) and duplicates the safety mechanism. 2026 agents disagree here (Codex/Command Code have deny/ask/allow modes; Claude Code leans permissive). For ass-guard's hands-off SDD goal, gating is anti-functional. | No tool-execution confirmation tier. The pattern/hook table (no match → nothing runs) + manual cancellation (drains queued injections) is the only safety mechanism. (PROJECT.md Out of Scope, NFR12.) |
| Windows platform support in v1 | "Windows is a big developer market." | Windows is not the first target (win-developer teams are not the author's SDD-practicing customer first). macOS + Linux cover the primary audience; adding Windows doubles platform-testing surface for a v1 that needs to validate mimicry first. | v1 = macOS + Linux, amd64 + arm64. Windows deferred. Interface accommodates later addition. (PROJECT.md Out of Scope.) |
| GSD / spec-kit / BMad toolkit adapters in v1 | "The adapter interface exists; ship all four toolkits." | OpenSpec has the deepest reference (predecessor's ccr-log, handoff examples, pattern research). Shipping four half-validated adapters dilutes v1 focus from the mimicry north star. The adapter *interface* must accommodate all four; the *implementations* are deferred. | v1 hosts OpenSpec only. Adapter interface accommodates GSD/spec-kit/BMad; implementations deferred. (PROJECT.md Out of Scope.) |
| Perplexity or other paid search backends committed in v1 | "Perplexity gives better grounding." | Committing a paid backend couples v1 to a vendor and a billing surface. The configurable-backend design (delta 3) makes paid backends *possible* without committing to one. | Configurable tool backends; no specific paid integration committed. A team can wire Perplexity themselves via config. (PROJECT.md Out of Scope.) |
| Byte-level telemetry / product analytics | "Track usage patterns to improve the product." | ass-guard is a single-binary local agent for SDD teams; a telemetry pipeline adds operational surface and a privacy posture that conflicts with the audit-log-only discipline. The audit log already captures everything needed to reconstruct runs. | Full local audit log is the observability surface. No remote telemetry. |
| Porting code from `sdd-acp-agent` | "The predecessor built this already; reuse it." | The predecessor's scope was narrower (Claude Code *compatibility*, flat per-command routing, no mimicry, no Telegram, two-mechanism autocontinue). Its code carries decisions that don't fit ass-guard's scope; retrofitting is costlier than building fresh. | Fresh build against the new scope. Predecessor is reference-only (research, architecture spine, epic breakdown, log analysis survive as reference). (PROJECT.md Out of Scope.) |
| General-purpose vector memory / RAG | "2026 agents have memory; add a vector store." | A general vector memory is a broad recall system (Cortex, Mem0 pattern). ass-guard's learning mode is narrower and sharper (launch policies + hook proposals), not a knowledge base. A vector store adds infra, embedding costs, and a recall-quality surface that doesn't serve the SDD-driving goal. | Learning mode = memlog of engine decisions + proposed hooks. Narrow, file-based, replayable. Not a vector store. |
| Multi-environment / remote-HTTP transport | "Run ass-guard as a remote service." | The ACP model is editor-spawned subprocess, stdio only. A remote/HTTP transport adds auth, networking, and lifecycle complexity that breaks the single-binary, editor-owned, no-daemon operational envelope. | v1 = single-binary stdio only (ACP) + Telegram (outbound-polling bot). No remote/HTTP transport, no observability pipelines. (PROJECT.md Additional Requirements: operational envelope.) |

## Feature Dependencies

```
[ACP v1 server (stdio)]
    └──requires──> [Built-in tool catalog]
    └──requires──> [Two-shape provider layer]
    └──requires──> [AGENTS.md / .claude/ config hierarchy]
    └──requires──> [Audit logging]

[Two-shape provider layer]
    └──enables──> [Model routing per task + fallback]  (table-stakes baseline)
                      └──enhanced-by──> [Multi-tier scheduling + time-windows + fallback chains]  (DIFFERENTIATOR)

[Agent mimicry via profiles]  (NORTH STAR)
    └──requires──> [Built-in tool catalog]  (must match profile's catalog name-for-name)
    └──requires──> [Two-shape provider layer]  (must send the right request shape)
    └──requires──> [AGENTS.md / system-prompt config]  (identity)
    └──requires──> [Profile log-extraction]  (grounded, not guessed)
    └──validated-by──> [Request-shape conformance test]  (structural, not byte)

[Hands-off SDD autocontinue]  (DIFFERENTIATOR)
    └──requires──> [ACP v1 server]
    └──requires──> [Per-toolkit pattern/handoff config]
    └──requires──> [Event bus (global-total-order broadcaster)]
    └──enables──> [Success bar: zero continue-taps]

[Unified post-turn engine]  (DIFFERENTIATOR)
    └──subsumes──> [Hands-off SDD autocontinue]
    └──subsumes──> [Hook-DAG execution]
    └──enables──> [Learning mode]

[Hook-DAG + seeded set]  (DIFFERENTIATOR)
    └──requires──> [Unified post-turn engine]
    └──requires──> [Context hygiene]  (fresh-context step needs boundary semantics)

[Learning mode]  (DIFFERENTIATOR)
    └──requires──> [Unified post-turn engine]
    └──requires──> [Hook-DAG]  (to propose new hooks)
    └──requires──> [Audit log / memlog]  (decision recall)

[Telegram peer + voice]  (DIFFERENTIATOR)
    └──requires──> [ACP v1 server / shared core]  (Telegram is a front to the same core)
    └──requires──> [Configurable STT backend]  (voice → text)

[Context hygiene (two-layer)]
    └──requires──> [Session durability / replay]  (boundary markers must survive restart)
    └──requires──> [Built-in tool catalog Mutability classification]

[Subagents]
    └──requires──> [Context hygiene]  (scoped, isolated windows)
    └──requires──> [Two-shape provider layer]  (bounded concurrency semaphore)

[Configurable tool backends]  (DIFFERENTIATOR)
    └──requires──> [Built-in tool catalog]  (backends plug into catalog tools)
```

### Dependency Notes

- **Mimicry requires the catalog + provider layer + config + log-extraction.** It is the north star and sits on top of the table-stakes stack; it cannot be validated until the baseline turn loop sends real requests. PROJECT.md mandates it is validated first, on the thinnest possible stack.
- **Multi-tier scheduling enhances (does not replace) baseline per-task routing.** The predecessor's per-command routing (FR5) is the substrate; tiers/time-windows/chains layer on top. Do not build the differentiator before the baseline.
- **Unified engine subsumes autocontinue and hooks.** Build autocontinue first (the predecessor's validated dual-signal design), then generalize into the unified engine with hooks-as-special-case. Building the unified engine abstractly first risks an unvalidated abstraction.
- **Learning mode requires the unified engine + hook-DAG.** It is the last differentiator — it depends on the engine existing to plug into and the hook-DAG to propose additions. Do not build learning mode before its host mechanisms.
- **Telegram is a front to the shared core, not a separate agent.** It depends on the ACP-driven core existing; it adds an outbound-polling bot + STT, not a parallel execution path. Build after the core is hands-off-capable so Telegram inherits the autocontinue.
- **Context hygiene is a hard dependency for subagents and for the hook-DAG's fresh-context step.** Subagents need scoped isolated windows (AD-9a); the fresh-context hook step needs boundary semantics. Build context hygiene before subagents and before the full hook-DAG.
- **Conflict: confirmation tier vs hands-off success bar.** A tool-execution confirmation tier directly breaks the zero-continue-taps goal. These are mutually exclusive — hence confirmation is an anti-feature for ass-guard.

## MVP Definition

### Launch With (v1)

Minimum viable product — what's needed to validate the concept. Ordered: mimicry first (it's the north star and gates everything), then the hands-off loop, then the differentiators that make it the author's daily driver.

- [ ] **ACP v1 stdio server** (initialize, session/prompt, streamed session/update, session/load replay) — the IDE-native entry point; without it the agent doesn't exist in an editor. INHERITED. [table stakes]
- [ ] **Two-shape provider layer** (Anthropic-shape + OpenAI-shape, configurable base URL) — must send requests in the target shape to make mimicry possible. INHERITED. [table stakes]
- [ ] **Built-in tool catalog matching the active profile** (zcode = claude-code-compat catalog; names + schemas verbatim from extracted logs) — the model expects this catalog; mimicry fails without it. INHERITED (catalog) + NEW (profile-driven). [table stakes + mimicry substrate]
- [ ] **Agent mimicry via profiles (zcode first), log-extracted** — the north star; validates the core thesis that requests can be made structurally indistinguishable. NEW (delta 1). [DIFFERENTIATOR — must work first]
- [ ] **AGENTS.md / `.claude/` config drop-in** (settings.json + CLAUDE.md hierarchy, namespaced under `.claude/sdd/`) — identity layer for mimicry; Claude Code users feel at home. INHERITED. [table stakes]
- [ ] **Per-task model routing + single fallback** (the predecessor's FR5 baseline) — survive provider hiccups; substrate for scheduling. INHERITED. [table stakes]
- [ ] **Hands-off SDD autocontinue (dual-signal, zero continue-taps) for OpenSpec** — the product's reason to exist; the success bar. INHERITED (FR10–FR12/FR20). [DIFFERENTIATOR]
- [ ] **Per-toolkit pattern/handoff config (OpenSpec seeded from ccr-log)** — makes autocontinue shareable/configurable, not hardcoded. INHERITED (FR15). [DIFFERENTIATOR substrate]
- [ ] **Event bus (global-total-order broadcaster)** — structural spine for autocontinue + later the unified engine. INHERITED (FR19). [DIFFERENTIATOR substrate]
- [ ] **Context hygiene (two-layer, mutating-always-boundary)** — keeps multi-step SDD clean; required for replay-on-load contract. INHERITED (FR13/FR14). [table stakes]
- [ ] **Subagents (Task/Agent goroutine turn-loops, panic-recovered)** — parallel exploration; the model expects this tool. INHERITED (FR17). [table stakes]
- [ ] **Audit logging (full action sequence, replayable)** — non-negotiable for a tool that runs ungated. INHERITED. [table stakes]
- [ ] **Web tools with configurable backend** (DDG as default, no hardcode) — model grounding; backend-config replaces predecessor hardcode. INHERITED (tool) + NEW (config backend, delta 3). [table stakes + NEW substrate]
- [ ] **Static binary + ACP registry manifest** (goreleaser, macOS+Linux amd64+arm64) — one-shot team install. INHERITED (NFR1/NFR10). [table stakes]
- [ ] **OpenSpec hosted unmodified** (per-toolkit config; shell adapter runs `openspec ...` as subprocess) — v1 toolkit scope. INHERITED (FR15/FR16). [DIFFERENTIATOR scope]

### Add After Validation (v1.x)

Features to add once the mimicry + hands-off core is working and validated against real OpenSpec workflows.

- [ ] **Multi-tier model scheduling + time-windows + fallback chains** — once per-task routing is proven, add tier abstraction, time-windowed substitution, per-project overrides, multi-step chains. NEW (delta 2). Trigger: per-task routing validated + cost/capacity steering needed.
- [ ] **Unified post-turn engine (autocontinue + hooks collapse)** — once dual-signal autocontinue is proven, generalize into the unified engine with hooks-as-special-case. NEW (delta 5). Trigger: autocontinue validated + a second hook consumer is wanted.
- [ ] **Hook-DAG + seeded set (post-implement: test+lint+review+memory; post-phase: improvement proposals)** — the "forgotten routine." NEW (delta 4/5). Trigger: unified engine lands + author wants the routine automated.
- [ ] **Telegram peer (text)** — once the core is hands-off-capable, add Telegram as a second front. NEW (delta 6). Trigger: core validated + mobile/async reach wanted.
- [ ] **Voice via configurable STT backend** — after Telegram text works, add voice-message transcription. NEW (delta 6). Trigger: Telegram text validated + voice input wanted.
- [ ] **Learning mode (ask-and-remember launch policies)** — once the unified engine exists, let it learn unknown launch situations. NEW (delta 4). Trigger: unified engine + a recurring "engine doesn't know what to do" case appears.
- [ ] **Learning mode (propose new hooks from work log)** — once hook-DAG + learning mode exist, let the engine propose hook additions. NEW (delta 4). Trigger: hook-DAG stable + author sees a routine the engine should have proposed.

### Future Consideration (v2+)

Features to defer until mimicry + hands-off + the differentiator stack are validated and the product has a real user (the author's own workflow first).

- [ ] **Additional profiles beyond zcode** — the architecture supports N profiles from day one, but only zcode is built in v1. Trigger: a second mimicry target is identified (e.g. a Cursor or Codex profile). Defer: each profile needs log-capture + extraction + conformance work.
- [ ] **GSD / spec-kit / BMad toolkit adapters** — interface accommodates them in v1; implementations deferred. Trigger: author's GSD/BMad/spec-kit workflows want hosting. Defer: OpenSpec has the deepest reference; broaden once OpenSpec hosting is proven.
- [ ] **Windows platform support** — macOS+Linux in v1. Trigger: a Windows-developer SDD team adopts. Defer: not the first customer.
- [ ] **Paid search backends (Perplexity, etc.)** — configurable backend makes this possible; no commitment. Trigger: a team needs higher-quality grounding than the default backend. Defer: DDG/default suffices for v1.
- [ ] **Replay-from-audit-log UI** — the audit log captures everything; a replay UI (TraceTrail/Gryph-style) is a later convenience. Trigger: debugging complex multi-stage runs gets painful. Defer: stderr logs + JSONL transcript suffice for v1.
- [ ] **Additional peer interfaces (Discord, Slack)** — Telegram is v1's peer; Discord/Slack follow the same front-to-core pattern. Trigger: a team's chat surface is Discord/Slack not Telegram. Defer: Telegram covers the mobile/voice/async reach first.

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority | Inherited/New |
|---------|------------|---------------------|----------|---------------|
| Agent mimicry via profiles (zcode, log-extracted) | HIGH | HIGH | P1 | NEW (delta 1) |
| ACP v1 stdio server | HIGH | MEDIUM | P1 | INHERITED |
| Two-shape provider layer | HIGH | MEDIUM | P1 | INHERITED |
| Built-in tool catalog (profile-driven) | HIGH | MEDIUM | P1 | INHERITED + NEW |
| AGENTS.md / `.claude/` config drop-in | HIGH | LOW | P1 | INHERITED |
| Per-task model routing + single fallback | HIGH | MEDIUM | P1 | INHERITED |
| Hands-off SDD autocontinue (dual-signal) | HIGH | HIGH | P1 | INHERITED |
| Per-toolkit pattern/handoff config (OpenSpec) | HIGH | MEDIUM | P1 | INHERITED |
| Event bus (global-total-order) | HIGH | MEDIUM | P1 | INHERITED |
| Context hygiene (two-layer, mutating-boundary) | HIGH | HIGH | P1 | INHERITED |
| Subagents (goroutine turn-loops) | MEDIUM | HIGH | P1 | INHERITED |
| Audit logging (full action sequence) | HIGH | MEDIUM | P1 | INHERITED |
| OpenSpec hosted unmodified (shell adapter) | HIGH | MEDIUM | P1 | INHERITED |
| Static binary + ACP registry manifest | MEDIUM | LOW | P1 | INHERITED |
| Web tools with configurable backend | MEDIUM | LOW | P1 | INHERITED + NEW (delta 3) |
| Multi-tier scheduling + time-windows + chains | MEDIUM | HIGH | P2 | NEW (delta 2) |
| Unified post-turn engine | HIGH | HIGH | P2 | NEW (delta 5) |
| Hook-DAG + seeded set | HIGH | HIGH | P2 | NEW (delta 4/5) |
| Telegram peer (text) | MEDIUM | HIGH | P2 | NEW (delta 6) |
| Voice via configurable STT | MEDIUM | MEDIUM | P2 | NEW (delta 6) |
| Learning mode (ask-and-remember) | MEDIUM | HIGH | P2/P3 | NEW (delta 4) |
| Learning mode (propose hooks) | MEDIUM | HIGH | P3 | NEW (delta 4) |
| Additional profiles (beyond zcode) | LOW | HIGH | P3 | NEW (delta 1 extension) |
| GSD / spec-kit / BMad adapters | LOW | MEDIUM | P3 | INHERITED interface / NEW impl |
| Windows support | LOW | MEDIUM | P3 | (deferred) |
| Paid search backends | LOW | LOW | P3 | (configurable, deferred) |

**Priority key:**
- P1: Must have for launch — the mimicry north star + the hands-off SDD core + the table-stakes baseline that makes them real.
- P2: Should have, add when the P1 core is validated — the differentiator stack (scheduling, unified engine, hook-DAG, Telegram, learning mode) that makes ass-guard the author's daily driver.
- P3: Nice to have, future consideration — breadth (more profiles, more toolkits, more platforms, more backends).

## Competitor Feature Analysis

| Feature | Claude Code (2026) | OpenCode (2026) | Codex CLI (2026) | Cursor (2026) | Aider (2026) | ass-guard (planned) |
|---------|--------------------|-----------------|-------------------|----------------|--------------|---------------------|
| **Mimicry (indistinguishable to model)** | N/A (is the reference) | No (own shape) | No (own shape) | No (IDE-internal) | No | **Yes — north star, profile-driven, log-extracted** |
| **Multi-provider** | Anthropic (+ compat) | 75+ providers out of the box | OpenAI (+ config.toml custom) | Multi-model picker | Any via LiteLLM | Two-shape (Anthropic + OpenAI) + configurable base URL |
| **Model routing** | Hooks (model-router-hook community) + multi-model | Router across 4 LLM tiers (community) | Profiles in config.toml | Auto + manual picker (bug-reported) | Per-task model choice | Per-task routing (baseline) → tier abstraction + time-windows + chains (differentiator) |
| **Lifecycle hooks** | ~30 events (PreToolUse/PostToolUse/Stop/SubagentStop/...) JSON-configured shell commands | Plugin system | Skills + config | `.cursorrules` + Composer rules | Limited | **Unified post-turn engine** (hooks = special case) + hook-DAG (differentiator) |
| **Autocontinue (hands-off multi-stage)** | Manual-continue + hooks at slots | Manual + Plan/Build modes | Manual + cloud tasks | Background agents (async) | Manual | **Dual-signal autocontinue, zero continue-taps for OpenSpec** (differentiator) |
| **Subagents** | Yes (agent hooks, Task tool) | Yes | Multi-agent (Agents SDK + MCP) | Background agents + adaptive-subagent proposals | No | Yes (goroutine turn-loops, panic-recovered, scoped context) |
| **Memory / learning** | CLAUDE.md (always-on), Cortex (community), skills | Config + skills | AGENTS.md + profiles | `.cursorrules` + Cortex (community) | Repo map | **Learning mode: launch-policy memlog + hook proposals** (differentiator) — narrower than vector-store memory |
| **Config / identity standard** | `.claude/` (settings.json, CLAUDE.md, skills, commands, plugins) | OpenCode-native dirs | AGENTS.md + config.toml + profiles | `.cursorrules` | conventions | **AGENTS.md + `.claude/` drop-in** (Claude Code compat) + `.claude/sdd/` namespaced additions |
| **IDE-native protocol** | (is the CLI) | Terminal-native | Terminal + JetBrains agent provider (July 2026) | Own IDE | Terminal | **ACP v1 (Zed/JetBrains/Obsidian) stdio JSON-RPC** |
| **Chat/mobile/voice peer** | Claude Code Channels (Telegram/Discord, March 2026) — adjunct to running session | No | No | No | No | **Telegram as full peer (text + voice), drives full SDD scenario** (differentiator) |
| **SDD-toolkit hosting (unmodified)** | Drives toolkits via skills/commands | Drives via commands | Drives via commands | Drives via commands | Drives via commands | **Host + auto-drive OpenSpec unmodified (v1); adapter interface for GSD/spec-kit/BMad** (differentiator focus) |
| **Audit logging** | JSONL transcripts (TraceTrail/Gryph replay ecosystem) | Logs | Logs | Diff history | Git-based | Full audit log (requests + tool calls + actions), replayable |
| **Distribution** | npm/CLI | Open source | npm/CLI + cloud | IDE installer | pip | Static Go binary (goreleaser) + ACP registry manifest |
| **Tool execution gating** | Permissive + hooks | Configurable | Sandbox + approval modes | Composer rules | Git-based | **No gating** (pattern/hook table + manual cancel is the only safety mechanism) — anti-feature to add gating |

### Competitor Notes

- **Claude Code is the reference, not a competitor.** ass-guard *mimics* it (via the zcode profile). Claude Code's config layout (`.claude/`), tool catalog, skills/commands/plugins, and lifecycle hooks are all things ass-guard is drop-in compatible with. The relationship is "indistinguishable-from," not "competes-with."
- **zcode is the mimicry target, also not a competitor.** It is a claude-code-compat runtime that ships its own AGENTS.md, skills, commands, MCP, and tool catalog. Its on-disk logs are the source of truth for the zcode profile.
- **The "claude-code-compat" category** (x-code-cli, multica wrapper runtimes, oh-my-openagent's `claude-code-compat-core`, Command Code's compat permission modes) confirms the ecosystem recognizes compat as a product class — but compat means *accepts the same config/plugins*, not *indistinguishable to the model*. ass-guard's mimicry is stricter and unoccupied.
- **OpenCode is the closest open-source analog** for provider flexibility (75+ providers, routing across tiers) but does not do mimicry, autocontinue, or SDD-toolkit hosting as first-class.
- **Codex CLI's `config.toml` profiles** (named profiles with distinct models/approval-modes/instructions) and **Cursor's adaptive-subagent proposals** are the nearest analogs to ass-guard's profile + learning-mode concepts — but neither makes outgoing requests structurally indistinguishable, and neither auto-drives an SDD toolkit hands-off.
- **Claude Code Channels (March 2026)** is the nearest analog to ass-guard's Telegram peer — but Channels messages *into a running session* as an adjunct, whereas ass-guard's Telegram is a full peer that can itself drive an entire SDD scenario. ass-guard's voice-as-ordinary-input via configurable STT is additionally uncommon for coding agents.
- **The SDD toolkit family** (OpenSpec, spec-kit, BMad, GSD) is explicitly designed to be driven *unmodified* by a coding agent — OpenSpec's `AGENTS.md` + `/opsx:*` slash-commands + `.agents/skills/` integration + "paste the setup prompt and the agent installs + runs `openspec init`" workflow means "host and auto-drive" is a well-defined surface, not a custom integration per toolkit. This validates ass-guard's hosting focus.

## Sources

**ACP / IDE-native integration:**
- [Agent Client Protocol — official site](https://agentclientprotocol.com/get-started/introduction)
- [Google, Zed fight VS Code lock-in with ACP — The Register](https://www.theregister.com/software/2025/08/28/google-zed-fight-vs-code-lock-in-with-agent-client-protocol/1500705)
- [Bring Your Own Agent to Zed — Zed Blog](https://zed.dev/blog/bring-your-own-agent-to-zed)
- [ACP Overview — Phil Schmid](https://www.philschmid.de/acp-overview)

**Claude Code ecosystem (the mimicry reference):**
- [Claude Code Hooks Reference — official docs](https://code.claude.com/docs/en/hooks)
- [Claude Code Settings — official docs](https://code.claude.com/docs/en/settings)
- [Claude Code Skills — official docs](https://code.claude.com/docs/en/skills)
- [Claude Code: Hooks, Subagents & Skills Complete Guide 2026 — ofox.ai](https://ofox.ai/blog/claude-code-hooks-subagents-skills-complete-guide-2026/)
- [Claude Code 2026: The Daily Operating System — Towards AI](https://pub.towardsai.net/claude-code-2026-the-daily-operating-system-top-developers-actually-use-d393a2a5186d)
- [Anthropic ships tool payload optimization (25 JSON schemas, ~52KB) — LinkedIn](https://www.linkedin.com/posts/alxsuv_llmops-promptengineering-claudecode-activity-7475197128353398785-ZaAL)

**claude-code-compat category (validates mimicry-adjacent ecosystem):**
- [Feature: Claude Code-compatible wrapper runtimes — multica-ai/multica #2288](https://github.com/multica-ai/multica/issues/2288)
- [woai3c/x-code-cli](https://github.com/woai3c/x-code-cli)
- [Command Code v1 — Claude Code-compatible permission modes](https://commandcode.ai/docs/whats-new)
- [cersei-types — Claude Code-compatible JSONL session persistence](https://lib.rs/crates/cersei-types)

**AGENTS.md standard (identity/config):**
- [A Complete Guide To AGENTS.md — AI Hero](https://www.aihero.dev/a-complete-guide-to-agents-md)
- [The Agent-Native Repo: Why AGENTS.md is the New Standard — Harness](https://www.harness.io/blog/the-agent-native-repo-why-agents-md-is-the-new-standard)
- [AGENTS.md, Skills, and the Full Workflow — Blake Niemyjski](https://blakeniemyjski.com/blog/agentic-driven-development/)

**Model routing / scheduling:**
- [Best AI Model for Coding Agents in 2026: A Routing Guide — Augment Code](https://www.augmentcode.com/guides/ai-model-routing-guide)
- [Claude Opus vs Sonnet vs Haiku: Model Routing Guide — Duet](https://duet.so/guides/claude-opus-vs-sonnet-model-routing)
- [AI Agent Model Routing and Dynamic Model Selection — Zylos AI Research](https://zylos.ai/research/2026-03-02-ai-agent-model-routing/)
- [tzachbon/claude-model-router-hook](https://github.com/tzachbon/claude-model-router-hook)
- [Routing — Spacebot Docs (fallback chains)](https://docs.spacebot.sh/routing)

**Comparable agents:**
- [OpenCode — official site (75+ providers)](https://opencode.ai/)
- [The Complete Guide to OpenCode — Fastino](https://fastino.ai/blog/the-complete-guide-to-opencode-open-source-ai-coding-agents)
- [Codex CLI Guide 2026 — blakecrosley.com](https://blakecrosley.com/guides/codex)
- [Codex Configuration Reference — official](https://openai-codex.mintlify.app/configuration/reference)
- [Codex as agent provider in JetBrains IDEs — GitHub Blog (July 2026)](https://github.blog/changelog/2026-07-07-codex-as-agent-provider-and-agentic-enhancements-in-jetbrains-ides/)
- [Claude Code vs Cursor vs Aider: 2026 Battle — dev.to](https://dev.to/sameer_saleem/claude-code-vs-cursor-vs-aider-the-2026-battle-for-your-terminal-and-ide-3cb4)
- [Cursor AI IDE 2026 Setup Guide — PetronellaTech](https://petronellatech.com/blog/cursor-ai-ide-setup-guide/)

**SDD toolkits (the hosting surface):**
- [Fission-AI/OpenSpec — GitHub](https://github.com/Fission-AI/openspec)
- [github/spec-kit — GitHub](https://github.com/github/spec-kit)
- [6 Best Spec-Driven Development Tools for AI Coding in 2026 — Augment Code](https://www.augmentcode.com/tools/best-spec-driven-development-tools)
- [9 Best AI Tools for Spec-Driven Development in 2026 — MarkTechPost](https://www.marktechpost.com/2026/05/08/9-best-ai-tools-for-spec-driven-development-in-2026-kiro-bmad-gsd-and-more-compare/)
- [I Tested Three Spec-Driven AI Tools — ranthebuilder.cloud](https://ranthebuilder.cloud/blog/i-tested-three-spec-driven-ai-tools-here-s-my-honest-take/)
- [fulgidus/pi-gsd (GSD framework) — GitHub](https://github.com/fulgidus/pi-gsd)
- [What Is GSD? Spec-Driven Development Without the Ceremony — Medium](https://medium.com/@richardhightower/what-is-gsd-spec-driven-development-without-the-ceremony-570216956a84)

**Multi-interface / chat peers / voice:**
- [Claude Code Channels: Telegram and Discord — Towards AI](https://pub.towardsai.net/claude-code-channels-message-your-ai-coding-agent-from-telegram-and-discord-2026-5f263ccc4b9c)
- [Claude Code Channels: Telegram and Discord Messaging — DigitalApplied](https://www.digitalapplied.com/blog/claude-code-channels-message-ai-coder-telegram-discord)
- [Telegram vs Slack vs Discord: Which Platform Is Best for AI Bots — getclaw.sh](https://getclaw.sh/blog/telegram-slack-discord-ai-bot-comparison)
- [Best Speech-to-Text Models 2026 — Telnyx](https://telnyx.com/resources/best-speech-to-text-engine)
- [Best Open Source STT 2026 with Benchmarks — Northflank](https://northflank.com/blog/best-open-source-speech-to-text-stt-model-in-2026-benchmarks)

**Post-stage automation / hooks for quality:**
- [Claude Code Hooks for Automated Quality Checks — dev.to](https://dev.to/letanure/claude-code-part-8-hooks-for-automated-quality-checks-20il)
- [What are test hooks in AI-native development — CircleCI](https://circleci.com/blog/test-hooks-ai-development/)
- [A Smart and Automated Code Review Stop Hook — r/ClaudeCode](https://www.reddit.com/r/ClaudeCode/comments/1qapiw2/a_smart_and_automated_code_review_stop_hook/)
- [Automate actions with hooks — Claude Code official](https://code.claude.com/docs/en/hooks-guide)

**Learning / memory:**
- [What Is AI Agent Memory — IBM](https://www.ibm.com/think/topics/ai-agent-memory)
- [AI Agent Memory: The Complete Guide — Mem0](https://mem0.ai/blog/memory-in-agents-what-why-and-how)
- [Cortex: Production-Ready Memory for AI Agents — Cursor Forum](https://forum.cursor.com/t/cortex-production-ready-memory-for-ai-agents-built-with-cursor/148436)
- [Memory for Autonomous LLM Agents — arXiv (AgeMem: store/retrieve/update/summarize/discard)](https://arxiv.org/html/2603.07670v1)

**Audit logging / replay:**
- [Gryph: Audit Trail for AI Coding Agents — safedep.io](https://safedep.io/gryph-ai-agent-audit-trail)
- [Agent Replays with TraceTrail — DevelopersDigest](https://www.developersdigest.tech/blog/agent-replays-with-tracetrail)
- [Audit AI Agent Activity (Claude, Copilot, MCP) — Nylas](https://cli.nylas.com/guides/audit-ai-agent-activity)
- [Enterprise AI coding agent deployment 2026 — Northflank](https://northflank.com/blog/enterprise-ai-coding-agent-deployment)
- [Analyzing coding agent transcripts — METR (Feb 2026)](https://metr.org/notes/2026-02-17-exploratory-transcript-analysis-for-estimating-time-savings-from-coding-agents/)

**Predecessor (inherited baseline):**
- `sdd-acp-agent` epics.md (5 epics / 27 stories / 20 FRs / 13 NFRs) — the INHERITED feature baseline.

---
*Feature research for: SDD-hosting AI coding agent (ass-guard)*
*Researched: 2026-08-09*
