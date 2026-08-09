# Project Research Summary

**Project:** ass-guard-agent (working name)
**Domain:** Go-based SDD-hosting AI coding agent with model-request mimicry
**Researched:** 2026-08-09
**Confidence:** HIGH

## Executive Summary

ass-guard-agent is a Go-based AI coding agent whose north star is **mimicry**: outgoing model requests must be structurally indistinguishable from a configured target agent (zcode first), so the model behaves identically to how it behaves in the mimicked agent. Around that core it hosts unmodified SDD toolkits (OpenSpec in v1), drives them hands-off through multi-stage scenarios (zero "continue" taps), runs configurable "forgotten routine" hook-DAGs (review/tests/lint/memory), and exposes a primary ACP interface (IDE-native) plus a full Telegram peer (text + voice). v1 scope is deliberately disciplined: OpenSpec only (GSD/spec-kit/BMad accommodated by the adapter interface, deferred), ACP primary + Telegram peer, macOS + Linux (Windows deferred), fresh build (the `sdd-acp-agent` predecessor is reference-only — no code port).

The recommended approach is anchored by a **mimicry-first build order**: prove the thesis (outgoing requests are structurally indistinguishable from zcode's, verified by a behavioral A/B parity test against recorded ground-truth logs) on the thinnest possible stack before building anything downstream. The stack is Go 1.25 with ACP v1 (the stable editor-native standard, JSON-RPC over stdio), two provider-shape adapters (Anthropic-shape via the first-party SDK covering Claude + GLM via Z.ai; OpenAI-shape covering MiniMax M3), the official `modelcontextprotocol/go-sdk` for MCP hosting, and `go-telegram/bot` for the Telegram peer. Three subsystems have **no mature Go library** and are hand-rolled by design: the in-process model scheduler (tier/time-window/fallback resolver), the hook-DAG executor (4 step types, ~300 lines), and the newline-delimited JSON-RPC framing (~150 lines). Hand-written mimicry profiles are explicitly rejected — profile content must be log-extracted (JSONL transcripts + a MITM proxy capture run), making mimicry grounded rather than guessed.

Key risks are dominated by the north star. **Profile drift** (zcode updates silently, the captured profile goes stale), **incomplete capture** (logs don't exercise every code path, mimicry breaks only on un-exercised tools), and **catalog drift** (ass-guard's built-in tool implementation diverges from the profile's declared schema) all compromise mimicry silently and must be engineered out from Phase M with a versioned profile artifact, a coverage manifest, a drift detector, and a schema-adapter layer. A second cross-cutting risk class is the **single-process operational envelope** (stdout reserved for ACP frames, MCP subprocess lifecycle, audit-log volume) — every subsystem must respect it. The roadmap must serialize the six deltas rather than slice them in parallel; a thin-slice-of-everything plan is risk-multiplication because when (not if) it breaks, you cannot tell which delta is at fault.

## Key Findings

### Recommended Stack

The stack is split into **inherited** core (verified current as of Aug 2026 — ACP v1 stable, ACP v2 is Draft and must NOT be targeted; Go floor bumped to 1.25 as 1.23 is end-of-support) and **five new focus areas** (mimicry mechanism, model scheduling, Telegram peer, hook-DAG engine, MCP hosting). Notably, three of the new areas deliberately carry **no external library** — the survey found no mature Go library for multi-provider model routing, no lightweight in-process Go DAG library that fits, and the JSON-RPC framing library line (`go.lsp.dev/jsonrpc2`) is stagnant. The right 2026 call in each case is hand-rolled (~150-300 lines), preserving the "single static binary, no daemon" constraint.

**Core technologies:**
- **Go 1.25** — implementation language — single static binary (editor spawns it as a subprocess, zero runtime deps), goroutines map to subagent fan-out, mature streaming HTTP. Pin `go 1.25` in go.mod; 1.23 is end-of-support.
- **ACP v1** (spec v1; **v2 is Draft, do not target**) — primary IDE-native interface — JSON-RPC 2.0 over stdio; Zed/JetBrains/Obsidian spawn the agent as a subprocess; stdout reserved for frames, all logging to stderr (non-negotiable LSP-style discipline).
- **`anthropics/anthropic-sdk-go` v1.62.0** — Anthropic-shape provider client — first-party, streaming + native `tool_use` + configurable base URL; one SDK covers Claude + GLM via `https://api.z.ai/api/anthropic`.
- **`sashabaranov/go-openai`** — OpenAI-shape provider client — covers MiniMax M3, OpenRouter, Groq STT; verify tool-calling schema per provider in Phase 0.
- **`modelcontextprotocol/go-sdk` v1.0.0+** (official, Google-collaborated) — MCP server hosting — `StdioMCPClient` launches Claude-Code-installed MCP servers as subprocesses; supersedes the community `mark3labs/mcp-go` on spec-compliance and long-term support.
- **`go-telegram/bot`** — Telegram peer framework — zero-dependency, idiomatic `context.Context` throughout (load-bearing for clean shutdown when ACP owns process lifecycle), handler-based.
- **Hand-rolled internal packages** (`internal/profile`, `internal/scheduler`, `internal/hookdag`, hand-rolled JSON-RPC framing) — mimicry profiles, tier/time-window/fallback scheduling, configurable hook-DAG executor, ACP wire framing — each is small, project-specific, and removes a stagnant/heavy dependency.
- **Claude Code `.claude/` config layout** (convention) — drop-in ecosystem compat — existing Claude-Code setups work unchanged; ass-guard namespaces additions cleanly under its own key, never clobbering.
- **STT: pluggable backend, OpenAI Whisper API default** — voice → text — drop-in via the OpenAI-shape client; interface accommodates whisper.cpp (subprocess, not cgo) and Groq.
- **`goreleaser` v2.17** — distribution — macOS + Linux, amd64 + arm64; produces the binary the ACP registry `agent.json` manifest points at.

See [STACK.md](./STACK.md) for the full per-area confidence levels, the "What NOT to Use" rationale (Temporal/LiteLLM/Argo/`go.lsp.dev` upstream/whisper.cpp-via-cgo), and the Phase-0 spike checklist.

### Expected Features

The 2026 AI-coding-agent market has converged on a recognizable table-stakes core (multi-provider routing, built-in tool catalog, hooks/lifecycle events, subagents, AGENTS.md/CLAUDE.md instruction layer, skills/slash-commands). "claude-code-compat" is now a product category, but it means *accepts-the-same-config*, NOT *indistinguishable-to-the-model* — ass-guard's mimicry is stricter and unoccupied. See [FEATURES.md](./FEATURES.md).

**Must have (table stakes):**
- ACP v1 stdio server (initialize, session/prompt, streamed session/update, session/load replay) — IDE-native entry point; without it the agent doesn't exist in an editor.
- Two-shape provider layer (Anthropic-shape + OpenAI-shape, configurable base URL) — must send the target shape to make mimicry possible.
- Built-in tool catalog (Bash, Read, Edit, Write, Glob, Grep, WebSearch, WebFetch, Task, TodoWrite) — matching the active profile name-for-name, schema-for-schema (log-extracted, not hand-written).
- AGENTS.md / `.claude/` config drop-in — identity layer; Claude Code users feel at home.
- Per-task model routing + single fallback — predecessor baseline; substrate for the scheduling differentiator.
- Context hygiene (two-layer: durable replayable transcript + lean projected window; mutating-toolkit-commands are always boundaries) — required for the ACP replay-on-load contract.
- Session durability + replay on restart; audit logging (full action sequence, replayable); subagents (goroutine turn-loops, panic-recovered); static-binary distribution via goreleaser.

**Should have (differentiators — the six deltas):**
- **Agent mimicry via profiles** (NORTH STAR — delta 1): profile = configurable bundle {system prompts, tool catalog, message shape, identity}; zcode first, N supported by design; **profile content is log-extracted, not hand-written**. Validated first on the thinnest stack.
- **Hands-off SDD autocontinue** for OpenSpec (zero continue-taps; dual-signal: text-pattern OR known handoff tool-call) — predecessor's reason-to-exist, carried forward.
- **Multi-tier model scheduling** (delta 2): heavy/good/light tiers, time-windowed substitution, per-project override, multi-step fallback chains.
- **Unified post-turn engine** (delta 5): one decision point (continue / trigger hook-DAG / ask user / wait / learn / stop); autocontinue is the upper mechanism, hooks are a special case.
- **Hook-DAG + seeded set** (delta 4/5): configurable DAG (run-command / send-prompt / fresh-context / wait); seeded post-implement (test+lint+review+memory) and post-phase (improvement proposals) for zero-config first run.
- **Configurable tool backends** (delta 3): generalizes the predecessor's hardcoded DDG; no paid backend committed in v1.
- **Telegram peer (text + voice)** (delta 6): a full-capability second front to one core; voice transcribed to ordinary text input via configurable STT at the edge.
- **Learning mode** (delta 4): ask-and-remember launch policies; propose new hooks from the work log.

**Defer (v2+):**
- Additional profiles beyond zcode (interface supports N from day one; each profile needs log-capture + extraction + conformance work).
- GSD / spec-kit / BMad toolkit adapters (interface accommodates them in v1; implementations deferred — OpenSpec has the deepest reference).
- Windows platform support (macOS + Linux in v1).
- Paid search backends (Perplexity, etc.) — configurable backend makes this possible; no commitment.
- Additional peer interfaces (Discord, Slack) — Telegram first; same front-to-core pattern.
- Standalone CLI / terminal REPL surface, byte-for-byte request identity, a tool-execution confirmation tier, general-purpose vector memory, remote/HTTP transport — all **explicitly out of scope** (see PROJECT.md); they would compromise the core value or operational envelope.

### Architecture Approach

ass-guard composes cleanly with the predecessor's 6-component runtime (ACP Frontend, Session Manager, Turn Loop, Tool Registry, Subagent Manager, Autocontinue Engine) — **no inherited component is removed, no inherited boundary is violated, the Event Bus remains the integration spine, the two-layer context model is reused.** The eight new architecture questions resolve to **5 new components + 1 promotion**: the Autocontinue Engine is promoted to the **Unified Engine** (broader decision mandate, same siting and safety property). The Event Bus pattern is broadened: the dangerous/complex things (Unified Engine, Audit Log, Profile Shaper's tool-catalog projection) are observers/projections, never in a turn's critical path. The mimicry mechanism is isolated at exactly one boundary (Profile Shaper → Provider Adapter), with one source of evidence (Audit Log's verbatim `shaped_request` records) — this makes mimicry locally verifiable and profile-swappable. See [ARCHITECTURE.md](./ARCHITECTURE.md).

**Major components (11 total = 6 inherited + 5 new, with the Autocontinue Engine promoted within the 6):**
1. **ACP Adapter** (inherited, renamed from ACP Frontend — an Interface Adapter impl) — stdio JSON-RPC frontend; stdout=frames, stderr=logs.
2. **Telegram Adapter** (NEW) — Telegram Bot API frontend; STT at the edge (voice → text before entering the core).
3. **Session Manager** (inherited, gains per-session lock for multi-interface serialization) — durable transcript + context-window projection; owns per-session input queue.
4. **Turn Loop** (inherited) — one model↔tool turn; now receives shaped request + model spec.
5. **Tool Registry** (inherited, gains configurable backends) — built-in catalog; read-only tools parallelize, mutating serialize.
6. **Subagent Manager** (inherited, unchanged) — `Task` tool → goroutine Turn Loops with isolated context.
7. **Profile Shaper** (NEW) — applies the active profile (system prompts, tool catalog projection, message shape, identity) to the outgoing provider request, before the adapter. Single mimicry chokepoint.
8. **Model Scheduler** (NEW) — resolves tier→model at request time (time-windows, per-project override, fallback chains); separate from the adapter (policy vs mechanism).
9. **Unified Engine** (PROMOTED from Autocontinue Engine) — post-turn-complete decision point emitting typed Actions (ContinueNextTurn / TriggerHookDAG / AskUser / Wait / Learn / Stop).
10. **Hook-DAG Executor** (NEW) — runs a configurable DAG of steps; `SendPrompt` re-enters the Turn Loop; owns its own context windows via `FreshContext`.
11. **Learned Config (Memory)** (NEW) + **Audit Log** (NEW) — persistent launch-decisions + proposed-hooks store; reconstruction-grade Event Bus tap recording verbatim shaped requests.

### Critical Pitfalls

Top risks from [PITFALLS.md](./PITFALLS.md) (priority-ordered for the mimicry north star). All five below must be engineered out from Phase M — they are not retrofit-safe.

1. **Profile drift (N1, CRITICAL — north-star killer):** zcode updates silently; the captured profile goes stale; mimicry degrades with no error. *Mitigation:* treat the profile as a versioned artifact `{profile_version, target_capture_ref (zcode build hash), captured_at}`; ship a profile-drift detector (`ass-guard profile check zcode`) that structurally diffs the profile against a fresh re-capture; pin a zcode version range per profile and warn loudly when out of range; distinguish cosmetic drift (acceptable) from structural drift (hard fail).
2. **Incomplete capture (N3, HIGH):** logs don't exercise every code path; mimicry breaks only on un-exercised tools — harder to detect than drift. *Mitigation:* the extractor must emit a **coverage manifest** from day one (`{observed, observation_count, source_log_entries}` per section); cross-check the extracted catalog against a non-log source (zcode's declared tool-definitions); build a capture-completeness script that exercises every declared tool.
3. **Tool-catalog drift (N4, HIGH):** ass-guard's built-in tool implementation drifts from the profile's declared schema; the model calls a tool expecting one shape, the executor runs another. *Mitigation:* the profile's declared catalog is the authoritative schema at runtime; a **schema-adapter layer** translates between profile-declared and built-in-implementation schemas; a catalog-consistency check runs in CI.
4. **Over-investing in byte-identical mimicry (N2, HIGH — wastes scarce attention on the wrong axis):** chasing `diff`-empty equality instead of behavioral parity. *Mitigation:* define mimicry parity as an **empirical property** — a fixed prompt suite through both ass-guard (zcode profile) and live zcode must produce statistically indistinguishable tool-call sequences; maintain a parity-impact triage (cosmetic / structural-low / structural-high); make PROJECT.md's "byte-identical is out of scope" a load-bearing comment at the top of the profile-serialization module.
5. **Hook-DAG infinite loops (N8, HIGH — runaway agent, cost + state corruption):** a hook's output matches the trigger for the next stage. *Mitigation:* tag every turn with provenance `{user, model, hook, autocontinue}`; the engine only fires autocontinue/hooks on `user`/`model`-provenance turns, never on `hook`/`autocontinue`; maintain an injection-depth counter per scenario with a small bound; add a loop-detector test fixture to the engine's acceptance test.

Two cross-cutting operational pitfalls also rank high because every phase depends on them being usable: **Audit log volume + sensitive-data leakage (N16)** (async/bounded writes, redaction layer, per-session rotation, reconstruction-sufficiency test) and **Greenfield-from-reference drift (N18)** (hard no-copy rule, re-verify every inherited fact in Phase 0 — the predecessor's verification is 2026-07-02 and 5 weeks is enough for an ACP spec revision).

## Implications for Roadmap

The single most important roadmap decision is **prove mimicry before anything else.** The north star is a thesis that must be validated empirically before code accumulates on top of it; if it fails, the project stops and re-plans. This dictates a strict dependency-ordered build, not a thin-slice-of-everything plan. The architecture's build sequence (ARCHITECTURE.md item #1) and the pitfalls' phase mapping (PITFALLS.md Phase M) agree: the Profile Shaper + Audit Log + thinnest possible Turn Loop (one adapter, one tier) is item #1, and every later item assumes it is proven.

Based on the research, suggested phase structure:

### Phase 0: Spike + Re-verification
**Rationale:** PITFALLS N18 flags that every load-bearing inherited fact is 5 weeks old and must be re-verified before building; STACK flags 5 Phase-0 verification items that do not block the recommendation but should be closed. The predecessor's research is *input*, not *oracle*.
**Delivers:** Closed verification items: zcode's exact JSONL transcript path + line schema; `go-openai` latest tag + tool-calling schema per OpenAI-shape provider; ACP v1 method names against the canonical spec; whisper.cpp cross-compile impact (only if local STT is in scope); `go-telegram/bot` + ACP stdout-collision integration test. Re-verified facts recorded with `{fact, source, verified_date, verified_against_version}`.
**Avoids:** Pitfall N18 (greenfield-from-reference drift — carrying forward a stale fact), I1 (ACP spec drift).

### Phase 1 (Phase M): Mimicry MVP — the north-star proof
**Rationale:** ARCHITECTURE.md build item #1; FEATURES dependency graph puts mimicry at the top of the table-stakes stack; PITFALLS marks N1/N2/N3/N4 as Phase M. This is the deliberate de-risking of the project's reason to exist. If it fails, stop and re-plan — do not build downstream on an unvalidated thesis.
**Delivers:** Profile Shaper + Audit Log + thinnest possible Turn Loop (one provider adapter, one tier); the zcode profile extracted from real logs (JSONL + a one-shot MITM proxy capture); a profile-drift detector (`ass-guard profile check zcode`); a coverage manifest; a catalog schema-adapter; and a **behavioral mimicry A/B parity test** (fixed prompt suite through both ass-guard-with-zcode-profile and live zcode → statistically indistinguishable tool-call sequences).
**Addresses:** Differentiator "Agent mimicry via profiles" (the north star); table-stakes catalog + provider layer as its substrate.
**Avoids:** N1 (drift), N2 (byte-identical waste — set the empirical bar early), N3 (incomplete capture), N4 (catalog drift), N18 (copy-ban enforced from project start).
**Cannot parallelize with:** anything. This phase is the gate.

### Phase 2 (Phase A + Session Core): Audit log, Session Manager, ACP Adapter
**Rationale:** ARCHITECTURE.md build items #2-#3. The Audit Log must exist alongside the Profile Shaper (it records the verbatim shaped request — that's the mimicry-evidence source). The two-layer Session Manager + the ACP Adapter (renamed frontend) wire the proven mimicry core to an IDE surface. The Interface Adapter abstraction validates cleanly with one implementation before Telegram is added.
**Delivers:** Two-layer context (transcript + projection) with replay-on-load; ACP surface working end-to-end with mimicry-faithful requests; audit log as a first-class async Event Bus consumer with redaction + rotation + reconstruction-sufficiency test.
**Uses:** Go 1.25, ACP v1, hand-rolled JSON-RPC framing, internal/audit, internal/session.
**Implements:** Session Manager, ACP Adapter, Audit Log components.
**Avoids:** N16 (audit-log volume/explosion — async from day one), I5 (context-drop breaking ACP replay — two-layer model present from day one).

### Phase 3 (Phase S): Model scheduling
**Rationale:** ARCHITECTURE.md build item #4. Depends on Phase 1 (the Shaper, for re-shaping when a fallback chain crosses providers) and Phase 2 (the wired ACP surface). Per-task routing (predecessor's FR5 baseline) is the substrate; tiers/time-windows/chains layer on top — do not build the differentiator before the baseline.
**Delivers:** Tier abstraction (heavy/good/light), time-windowed substitution (timezone-explicit, IANA zones), per-project override, multi-step fallback chains with transient-vs-structural failure classification + circuit breakers + cost ceilings.
**Addresses:** Differentiator "Multi-tier model scheduling" (delta 2).
**Avoids:** N5 (fallback-chain cascading failure), N6 (time-window boundary bugs — bundle `time/tzdata`), N7 (tier mismatch across providers — document per-(provider,tier) capability profile).

### Phase 4 (Phase H, part 1): Unified engine + Hook-DAG + learning mode
**Rationale:** ARCHITECTURE.md build items #6-#8. Autocontinue (the predecessor's validated dual-signal design) is built first *inside* the new decision-point shape, then generalized to the Unified Engine with hooks-as-special-case. Building the unified engine abstractly first risks an unvalidated abstraction. The Hook-DAG depends on the engine + context hygiene (FreshContext step needs boundary semantics). Learning mode comes last — it depends on the engine + hook-DAG to plug into and propose additions to.
**Delivers:** Unified Engine with provenance-tagged turns; Hook-DAG executor with seeded set (post-implement, post-phase); learned-config store + learning mode (confidence-threshold ≥3, expiry/review dates, conflict-checked at accept time).
**Addresses:** Differentiators "Unified post-turn engine" (delta 5), "Hook-DAG + seeded set" (delta 4/5), "Hands-off SDD autocontinue" for OpenSpec, "Learning mode" (delta 4).
**Avoids:** N8 (hook loops — provenance tagging is structural), N9 (hook failure semantics — every hook declares `on-failure: halt|continue|ask`), N17 (learning proposes bad hooks — confidence threshold, counter-examples, versioned+revertible).

### Phase 5 (Phase I): Telegram peer (text first, then voice) + MCP hosting (Phase C)
**Rationale:** ARCHITECTURE.md build items #9. Telegram is a front to the shared core, not a separate agent — build it after the core is hands-off-capable so it inherits autocontinue. STT sits at the Telegram Adapter boundary; the core sees text only. MCP hosting can proceed in parallel with Telegram once the tool registry is stable (Phase 2/3), since it bridges MCP tools into the catalog.
**Delivers:** Telegram Adapter (text first), Interface Adapter abstraction validated with two impls; STT at the edge (OpenAI Whisper default); MCP hosting via `StdioMCPClient` with process-group spawn + group-signal shutdown + reaper goroutine.
**Uses:** `go-telegram/bot`, OpenAI Whisper API, `modelcontextprotocol/go-sdk`.
**Implements:** Telegram Adapter, MCP client hosting; per-interface threat model (ACP ungated, Telegram-gated allowlist for mutating MCP tools).
**Avoids:** N10 (concurrent input race — single serialized session mailbox from the start), N11 (responsiveness asymmetry — status queries + cancel-via-either-interface), N12 (STT errors compounding — echo-back-before-acting + domain-term dictionary), N13 (MCP subprocess zombies — process groups + transport-coupled kill), N14 (MCP schema drift — never cache `tools/list`; re-fetch every connection), N15 (MCP security — per-interface threat model, Telegram mutating-MCP allowlist).

### Phase 6: Polish + distribution
**Rationale:** ARCHITECTURE.md build item #10. Ship readiness after all functional deltas are validated.
**Delivers:** goreleaser config (macOS + Linux, amd64 + arm64); ACP registry `agent.json` manifest; full audit-log replay tooling; documented threat model; profile-drift detector in CI.
**Addresses:** Static-binary distribution (NFR1/NFR10); one-shot team install via ACP registry.

### Phase Ordering Rationale
- **Mimicry-first is non-negotiable** (Phase 1 gates everything). The north star is a thesis; validating it on the thinnest stack is the deliberate de-risking. ARCHITECTURE.md's build item #1, FEATURES' dependency graph, and PITFALLS' Phase M mapping all agree. Skipping this to build "real features" first is the explicit Anti-Pattern 5 in ARCHITECTURE.md.
- **The dependency chain is strict through Phase 3.** Session Manager needs the Turn Loop; ACP Adapter needs the Session Manager; Scheduling needs the Shaper (fallback re-shaping). These cannot usefully parallelize.
- **From Phase 4 onward, limited parallelism is possible** *within* a phase (e.g. seeded-hook authoring alongside engine internals) but not across phases — Phase 4 (engine) must precede Phase 5 (Telegram inherits autocontinue).
- **MCP hosting (Phase C) can proceed in parallel with Telegram (Phase I)** once the tool registry is stable, because MCP bridges into the catalog without depending on the engine or Telegram.
- **Serialize the six deltas; do not thin-slice them in parallel.** PITFALLS N18 is explicit: a "thin slice of all six in Phase 1" plan feels like risk-reduction but is risk-multiplication — when it breaks, you cannot tell which delta is at fault.
- **The v1 cut-line is OpenSpec only, ACP primary + Telegram peer, macOS + Linux.** Do NOT add GSD/spec-kit/BMad implementations, Windows, paid search backends, additional profiles, or a standalone CLI surface in v1 — each is an explicit anti-feature or deferral per FEATURES.md and PROJECT.md.

### Research Flags
Phases likely needing deeper research during planning:
- **Phase 1 (Phase M — Mimicry):** The single highest-research phase. Must close zcode's exact JSONL schema (STACK Phase-0 item #1), design the profile artifact format, the coverage manifest, and the behavioral parity test methodology. The mimicry A/B parity test is itself a research question (what prompt suite, what tolerance band, what statistical comparison).
- **Phase 3 (Phase S — Scheduling):** Fallback-chain semantics under correlated failure (N5) and time-window timezone handling (N6) need explicit design. LiteLLM's router is a design template worth studying (NOT a dependency — it's a Python proxy).
- **Phase 5 (Phase I — Telegram/MCP):** Per-interface threat model (N15) is a design decision PROJECT.md must resolve: ACP ungated vs Telegram-gated allowlist for mutating MCP tools. This tension with PROJECT.md's "no tool-execution confirmation tier" must be resolved explicitly.

Phases with standard patterns (skip research-phase):
- **Phase 0 (Spike):** Closing STACK's 5 verification items is mechanical.
- **Phase 6 (Polish/Distribution):** goreleaser + ACP registry manifest are well-documented standard patterns.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Every load-bearing recommendation verified against current (Aug 2026) public sources. 5 Phase-0 verification items identified, none architectural. Inherited choices (ACP v1, two provider SDKs, goreleaser) re-verified current. |
| Features | HIGH | 2026 landscape verified across Claude Code, zcode/claude-code-compat ecosystem, OpenCode, Codex CLI, Cursor, Aider, SDD toolkit family, ACP standard. Mimicry-as-product-category confirmed unoccupied. |
| Architecture | HIGH | Inherited spine high-confidence (predecessor research); 5 new components medium-high (interfaces fixed, internals validated during build). Composes cleanly with predecessor's 6 components — no removals, no boundary violations. |
| Pitfalls | HIGH | New-scope pitfalls grounded in external sources where available (MCP zombies, Anthropic rate limits) and project-specific reasoning where not. Priority-ordered for the mimicry north star. |

**Overall confidence:** HIGH. The only genuine uncertainty is the Phase-1 mimicry proof itself — which the roadmap is designed to surface and resolve first.

### Gaps to Address

Areas where research was inconclusive or needs validation during implementation:

- **zcode's exact JSONL transcript path + line schema** (STACK Phase-0 item #1; closes Focus 1 confidence from MEDIUM → HIGH on zcode specifics). Verify during Phase 0; do not assume the claude-code-compat path is exactly zcode's path.
- **Mimicry A/B parity test methodology** (PITFALLS N2): defining "statistically indistinguishable tool-call sequences" with a tolerance band is itself a design question. Resolve during Phase 1 planning — the acceptance test shape must be agreed before Phase 1 implementation.
- **Per-interface threat model for MCP + Telegram** (PITFALLS N15): PROJECT.md's "no tool-execution confirmation tier" tension with Telegram-as-remote/voice-driven surface must be resolved. Recommended: ACP ungated (matches Claude Code), Telegram mutating-MCP-tool allowlist defaults empty. PROJECT.md decision pending.
- **`sashabaranov/go-openai` tool-calling schema per provider** (STACK Phase-0 item #2): pkg.go.dev publish cadence looks ~1 year stale vs GitHub; pin to latest tag at build time and verify schema fidelity per OpenAI-shape provider.
- **Telegram-only launch vs PROJECT.md's "no standalone CLI surface"** (STACK tension flagged in Focus 3): whether a Telegram-only cobra subcommand counts as a "CLI surface" must be resolved at architecture time. Minor, but unresolved.
- **Can a Telegram message interrupt an in-flight ACP turn?** (PITFALLS N10): serialization semantics must be decided. Recommended: no — queue for the next turn boundary; surface "your message will be applied after the current stage."

## Sources

Consolidated and deduplicated from the four research documents. Full per-document source lists in [STACK.md](./STACK.md), [FEATURES.md](./FEATURES.md), [ARCHITECTURE.md](./ARCHITECTURE.md), and [PITFALLS.md](./PITFALLS.md).

### Primary (HIGH confidence)

**Project scope:**
- `/Users/nil/DiskD/W/Djarvur/ass-guard-agent/.planning/PROJECT.md` — north star, six deltas, requirements, key decisions, constraints, v1 cut-line.

**Predecessor research (inherited spine — reference, not oracle; re-verify in Phase 0 per N18):**
- `/Users/nil/DiskD/W/Djarvur/sdd-acp-agent/_bmad-output/planning-artifacts/research/technical-sdd-acp-agent-research-2026-07-02.md` — 6-component decomposition, event bus, two-layer context, provider-shape isolation, ACP wire protocol, integration patterns, phased build.

**ACP (inherited, re-verified Aug 2026):**
- https://agentclientprotocol.com/get-started/introduction — ACP overview.
- https://agentclientprotocol.com/protocol/v1/overview, /transports, /prompt-turn, /tool-calls, /session-setup, /schema — v1 spec (stable).
- https://agentclientprotocol.com/announcements/acp-v2-draft — v2 is Draft; confirms v1 is the stable target.
- https://github.com/agentclientprotocol/agent-client-protocol — canonical repo.
- https://agentclientprotocol.com/rfds/acp-agent-registry + https://github.com/agentclientprotocol/registry/blob/main/agent.schema.json — registry manifest format.

**Go & build:**
- https://go.dev/doc/devel/release — release history (1.23 EOL; floor bumped to 1.25; 1.26 = Feb 2026).
- https://goreleaser.com/ — v2.17 current (2026).

**Model providers (verified Aug 2026):**
- https://github.com/anthropics/anthropic-sdk-go/releases — v1.62.0 (Jul 2026).
- https://github.com/sashabaranov/go-openai — ~10.7k stars, 2026-active.
- https://docs.z.ai/devpack/quick-start — Z.ai Anthropic-compatible base URL confirmed.
- https://platform.claude.com/docs/en/api/rate-limits — Anthropic rate limits (transient-vs-structural failure classification for N5).

**MCP hosting (official SDK):**
- https://github.com/modelcontextprotocol/go-sdk — official Go SDK, v1.0.0, Google-collaborated.
- https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp — protocol 2026-07-28, stdio + stderr guidance.
- https://modelcontextprotocol.io/docs/2026-07-28/sdk — official SDK docs.
- https://github.com/mark3labs/mcp-go — community alternative (influenced official SDK; not the default for greenfield 2026).

**Mimicry capture (Focus 1):**
- https://www.adityabawankule.io/blog/claude-code-session-jsonl-format — JSONL path `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`.
- https://github.com/chouzz/llm-interceptor — MITM proxy for AI coding assistants (LLI).
- Sherlock MITM proxy — https://news.ycombinator.com/item?id=46799898.
- https://github.com/daaain/claude-code-log — JSONL → HTML/Markdown converter (dev-time tool).

**Telegram + STT:**
- https://github.com/go-telegram/bot — recommended (zero-dep, context-first, active).
- https://developers.openai.com/api/docs/guides/speech-to-text — OpenAI Whisper API (default STT).
- https://github.com/ggml-org/whisper.cpp — local STT (subprocess, not cgo).

### Secondary (MEDIUM confidence)

**claude-code-compat category (validates the mimicry-adjacent ecosystem):**
- https://github.com/woai3c/x-code-cli ; https://github.com/multica-ai/multica/issues/2288 ; https://commandcode.ai/docs/whats-new ; https://lib.rs/crates/cersei-types — compat-as-config, not indistinguishable-to-model.

**Comparable agents & routing:**
- https://opencode.ai/ (75+ providers); https://openai-codex.mintlify.app/configuration/reference ; https://github.blog/changelog/2026-07-07-codex-as-agent-provider-and-agentic-enhancements-in-jetbrains-ides/ ; https://docs.litellm.ai/docs/routing (design template only — Python proxy, NOT a Go dep).
- https://www.augmentcode.com/guides/ai-model-routing-guide ; https://duet.so/guides/claude-opus-vs-sonnet-model-routing — 2026 routing guides.

**SDD toolkit family (the hosting surface):**
- https://github.com/Fission-AI/openspec (v1 target); https://github.com/github/spec-kit ; https://github.com/fulgidus/pi-gsd — deferred; interface accommodates.

**Multi-interface / chat peers / voice:**
- https://pub.towardsai.net/claude-code-channels-message-your-ai-coding-agent-from-telegram-and-discord-2026-5f263ccc4b9c — Claude Code Channels (adjunct, not full peer — validates ass-guard's sharper differentiator).

**MCP subprocess lifecycle (PITFALLS N13):**
- https://dev.to/thestack_ai/i-built-a-zombie-process-killer-because-claude-code-ate-14gb-of-my-ram-1deg — orphaned MCP-server resource accumulation.
- https://github.com/NousResearch/hermes-agent/issues/15012 — gateway zombie-process bug.
- https://forum.cursor.com/t/cursor-3-4-20-kills-stdio-mcp-servers-1-5s-after-successful-initialize-sigkill-v2-fsm-race/160892 — stdio lifecycle gotcha.

**Hook-DAG design references (NOT dependencies):**
- https://github.com/go-task/task (DAG-ordered Make-like); https://github.com/Flowpack/prunner (embeddable Go pipeline runner) — design references only.
- https://www.reddit.com/r/golang/comments/nsfjtq/ — Temporal = overkill for embedded pipelines.

### Tertiary (LOW confidence — needs validation)

- **zcode's exact JSONL path + line schema for ass-guard's mimicry capture** — claude-code-compat path is verified but zcode's specific behavior may have diverged; verify in Phase 0 (STACK item #1, closes Focus 1 from MEDIUM → HIGH).
- **`sashabaranov/go-openai` latest-tag tool-calling fidelity per OpenAI-shape provider** (MiniMax M3, Groq) — pin the tag and verify schema during Phase 0 (STACK item #2).
- **whisper.cpp cross-compile impact via goreleaser** — only relevant if local STT is in scope; subprocess approach should preserve the static binary but verify (STACK item #4).

---
*Research completed: 2026-08-09*
*Ready for roadmap: yes*
