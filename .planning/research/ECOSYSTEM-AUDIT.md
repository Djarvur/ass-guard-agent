# Ecosystem Audit — Ideal-Agent Sources, Borrow Verdicts, and Proposed Requirements

**Domain:** cross-agent ecosystem audit feeding v1.2+ planning: which agent architectures were surveyed, what they validate in our plan, what we should borrow, what we reject, and three new requirement clusters (plugins, memory, evals) proposed in v1.1 requirements style.
**Researched:** 2026-08-14 → 2026-08-15 (context session; all web surfaces verified Aug 2026, local ground truth from `~/.zcode/cli/plugins/`, shallow clones of `earendil-works/pi` and `oraios/serena`); **amended 2026-08-19** (OpenClaude + Open Claude Code deep-dives; GitHub API + README verified live, not source-read)
**Confidence:** HIGH for audit rows grounded in our own v1.0/v1.1 requirement docs and local artifacts; HIGH for plugin/marketplace formats (official docs + zcode's live registry files); MEDIUM for sizing estimates (line counts exact, port multipliers judgment); MEDIUM for Hindsight internals (README-level, not build-verified); **HIGH for the 2026-08-19 OpenClaude/occ deep-dives (both shallow-cloned and source-read; key claims file-anchored in §1.1; OpenClaude legal status quoted from its LICENSE/NOTICE)**.

> Scope note: this doc does NOT reopen v1.1 scope. It (a) validates the v1.0/v1.1 plan against external sources, (b) lists borrow candidates with effort/trigger conditions, (c) proposes requirement clusters for the NEXT milestone, and (d) records redlines that should survive all future planning. Consumption: read during the next `/gsd:new-milestone` requirements discussion; Sections 4–5 are drafted to be lift-able into REQUIREMENTS.md.

---

## 1. Sources Audited

| Source | What it is | Why audited |
|---|---|---|
| **ass-guard v1.0/v1.1 plans** | `.planning/milestones/v1.0-REQUIREMENTS.md` (shipped), `.planning/REQUIREMENTS.md` (v1.1, executing Phase 8) | the plan under test |
| **Claude Code internals course** (`justxor/Claudecourse`) | 36-point RU-language deep-dive on Claude Code's harness: loop, permissions, hooks, compaction, sub-agents, evals, token/caching economics, fault tolerance | the richest single production-harness checklist |
| **OpenCode** (`anomalyco/opencode`) | OSS TS/Bun coding-agent server + Go TUI; ACP-native; embeddable via generated SDKs; provider layer = Vercel AI SDK | closest open claude-code-shaped runtime; fork/subprocess candidate |
| **pi** (`earendil-works/pi`, ex badlogic/pi-mono) | minimal TS "anti-framework": `ai`/`agent`/`protocol` layered libs, extension host, session tree; powers OpenClaw | closest library floor; custom-provider seam |
| **OpenClaw** (`openclaw/openclaw`) | agent gateway daemon: Telegram/WhatsApp/Discord/… adapters, heartbeat, cron, on pi | the Telegram-peer half of our product, in production |
| **Serena** (`oraios/serena`) | MCP server exposing LSP (40+ langs, via bundled `solidlsp`) + markdown memory system | LSP + memory via our existing MCP hosting |
| **Hindsight** (`vectorize-io/hindsight`) | memory system: World/Experiences/Mental-Models banks; Retain/Recall/Reflect; hybrid retrieval | memory design ideas |
| Framework survey (context) | Google ADK Go 2.0, Microsoft Agent Framework Go (preview), Claude Agent SDK (TS/Py), AWS Strands, OpenAI Agents SDK, LangGraph/pydantic-ai, WASM-bridge option | substrate decisions, §2 |
| **OpenClaude** (`Gitlawb/openclaude`, added 2026-08-19, source-read) | ~30.8k★ TS/Bun coding-agent CLI (Node ≥22, npm `@gitlawb/openclaude`; 684k lines); 20+ cloud/local providers; **contains unlicensed Anthropic Claude Code source (per its own LICENSE/NOTICE)** | largest open Claude-Code-shaped runtime; provider-surface + UX reference; provenance: read freely, no mechanical porting (§1.1) |
| **Open Claude Code** (`ruvnet/open-claude-code`, `occ`, added 2026-08-19, source-read) | Node clean-room rebuild of the Claude Code CLI from decompiled npm intelligence (ruDevolution); 9.8k-line core: 25 tools, 6 permission-mode names, 4 MCP transports, ~100 `CLAUDE_CODE_*` env vars; nightly upstream-tracking releases (tracks Claude Code 2.1.235) | enumerated Claude Code surface = toolcat/profile/config coverage checklist; nightly parity pipeline + deterministic cost-cascade outcome store (§1.1) |

---

## 1.1 Deep Dives — OpenClaude & Open Claude Code (amended 2026-08-19)

### OpenClaude (`Gitlawb/openclaude`) — the unlicensed Claude-Code fork turned multi-provider

**What it is (source-read, shallow clone 2026-08-19).** TS/Bun CLI (Node ≥22, npm `@gitlawb/openclaude`), ~30.8k★ / 8.9k forks since creation 2026-04-01; 2,591 TS files / **684,830 lines** under `src/`. One terminal-first workflow across 20+ providers: OpenAI-compatible (OpenAI/OpenRouter/DeepSeek/Groq/Mistral/LM Studio), Gemini, GitHub Models, Codex OAuth/Codex, Ollama, Atomic Chat, Z.AI GLM Coding Plan (`glm-5.2`), Bedrock/Vertex/Foundry, + partner gateways. Own config root `~/.openclaude` + `~/.openclaude.json` + `.openclaude-profile.json`; **deliberately does not read `~/.claude` or `CLAUDE_CONFIG_DIR`** — the opposite of our drop-in-compat decision, though their migration docs map what users actually create.

**Provenance (factual finding; reading allowed — operator ruling 2026-08-19).** The repo LICENSE/NOTICE states it verbatim: *"This repository contains code derived from Anthropic's Claude Code CLI. The original Claude Code source is proprietary software… This project does not have Anthropic's authorization to distribute their proprietary source. Users and contributors should evaluate their own legal position."* The source confirms the lineage beyond doubt: upstream Claude Code internals are everywhere (`tengu_*` internal names, `getCacheControl` cache-breakpoint logic, `feature('KAIROS')` gates, Zod v4 settings schema, `getClaudeConfigHomeDir`). Repo creation (2026-04-01) is the day after the March 31, 2026 source-map leak. **Consequences, scoped:** (a) no *mechanical line-by-line porting* from this tree — a mechanical translation of copyrighted code into Go is still a derivative work, and this tree's own NOTICE (knowing unauthorized redistribution, post-leak) is the worst provenance to mechanically build from; (b) its wire behavior is evidence about *Claude Code*, not about zcode — our target is zcode, so profile content still comes from zcode's own transcripts, a fidelity rule rather than a reading ban; (c) studying this codebase for behavior, architecture, and ideas is fine and encouraged — that is what this deep-dive did.

**Alignment (validates our plan).**
- Multi-provider behind one agent core is proven at scale (our §2 substrate bet; supports the scheduler's 2-shape provider model).
- Provider profiles + `/provider` guided setup ≈ our scheduler per-project override + provider-config UX (phase 07).
- Background sessions are child processes (`--bg`, `openclaude ps/logs/kill`; no daemon, no network service) — independent confirmation of our no-port process-lifecycle invariant.
- `--resume/--continue/--fork-session` = our TREE branch-at-boundary borrow, shipping in fork-shaped form (transcript-level only; no filesystem/worktree isolation — same boundary we drew).

**Borrow (code-confirmed, all provider-surface or UX-shaped, none shaping-shaped).**
- **OLLAMA-CTX**: Ollama native chat API with explicit 32768-token context request per call (avoids silent truncation via the OpenAI-compatible shim) — concrete recipe for a local tier in our scheduler.
- **SEARCH-FALLBACK**: DuckDuckGo scrape as free `WebSearch` default + optional Firecrawl upgrade — validates the exact fallback pattern our WebSearch port (ddg-search, STACK.md) already chose.
- **AGENT-ROUTING** (`services/api/agentRouting.ts` + `agentRouteSettings.ts`): per-agent `agentModels`/`agentRouting` overrides + `maxSteps` caps — recipe confirmation for scheduler tier→agent mapping.
- **REPOMAP** (`context/repoMap/`: parser, graph, pagerank, renderer, cache): tree-sitter + IDF + PageRank; only the *callable-tool* shape is compatible with mimicry — auto-injection (even env-gated) stays rejected (request-parity redline; Aider verdict).
- **VCR** (`services/vcr.ts`): HTTP record/replay layer — a third independent validation of the parity-cassette direction (adk `httprr`, fantasy VCR).
- **TOKEN-ECON** (`services/tokenEstimation.ts`, `cost-tracker.ts`, `cacheMetrics.ts`): char-weight token estimation + custom pricing + cache-hit stats — reference UX for our ECON/CACHE surfaces.

**Steal (long-term, format-level only).** session memory / team memory / memdir layout as MEM tier-2 format reference; `compaction` + `contextCollapse` as a *concept catalog* for the compaction verification question (🧲-2), never as code.

**Reject.** mechanical porting from this tree (unauthorized-redistribution lineage — the one scoped guard in redline 8); headless gRPC server (`src/grpc/`; network-port violation); `/buddy` gimmick; `~/.openclaude` config divergence; VS Code extension / web / Docker / self-hosted-runner surfaces (out of scope). Reading the source and borrowing ideas from it is explicitly fine (operator ruling 2026-08-19).

### Open Claude Code (`ruvnet/open-claude-code`, `occ`) — the clean-room surface census (source-read)

**What it is (source-read, shallow clone 2026-08-19).** Node (`.mjs`) rebuild of the Claude Code CLI, "reverse engineered & rebuilt" from analysis of the *published npm package* via ruDevolution (34,759+ functions tracked, 95.7% name accuracy); MIT; ~477★; package `@ruvnet/open-claude-code`; tracks upstream to Claude Code 2.1.235 (2026-08-18). **v2/ is small enough to read end-to-end: 144 `.mjs` files, 9,772 total lines** (core 1,959; tools 2,293; ui 1,691; optimize 649; mcp 648; config 547; permissions 521). Test suite: 991/991 passing (README copy still claims 1,581 tests / 61 files / 8,314 lines — stale badge corrected in commits).

**What the source actually shows (verified, with fidelity caveats).**
- **Agent loop** (`core/agent-loop.mjs`, 518 lines): async generator yielding 13 event types; `MAX_TOOL_RECURSION_DEPTH=50`; pre/post tool hooks; permission check per tool; tool results appended as one user message; per-task cost-cascade hook on the initial prompt only.
- **Provider layer** (`core/providers.mjs`): unified anthropic/openai/google transforms + **bedrock/vertex are stubs** ("would go here"). Anthropic body is functional-minimal: `model/max_tokens/messages/system/tools/stream/thinking` — **no `cache_control`**, no attempt at wire fidelity. OpenAI conversion keeps only `tool_result` blocks in history; Google conversion drops tool blocks entirely. **Conclusion: occ proves the enumerable surface, not the wire. A useful census to study — but not a shaping reference, because its own requests are deliberately non-faithful (fidelity verdict, not a legal one).**
- **Permissions** (`permissions/checker.mjs`): 6 mode *names* from decompilation (default/auto/plan/acceptEdits/bypassPermissions/dontAsk) with simplified behavior — no readline → default mode allows (headless); `auto` = allow; `dontAsk` = deny-all-unpreapproved; plan = read-only trio. Plus always-on Bash injection check + file-path validation. Names match the census; semantics are weaker than the real thing.
- **MCP** (`mcp/client.mjs` + 3 transports): all 4 transports (stdio newline-JSON RS, SSE, WebSocket, Streamable HTTP) with a pending-map request/response pump; **protocol pinned to 2024-11-05** — two generations behind the 2026-07-28 our go-sdk targets; stdio framing is the same newline-delimited shape we hand-rolled.
- **Compaction** (`core/context-manager.mjs`): 4-char/token estimate, 80%-of-180k threshold, micro-compaction (truncate stale tool results >200→100 chars), then **deterministic non-LLM** compaction (first-200-char summary, keep 6 recent messages). Deliberately simpler than Claude Code's real LLM auto-compact — another census=fidelity gap.
- **Session** (`core/session.mjs`): writes **`~/.claude/projects/<sha256(cwd)>/session.json`** — a single JSON file with full messages (not JSONL append); save/resume/teleport (base64). Notes a compat project that *writes into `~/.claude`* (we never will — redline 4).
- **Env census** (`config/env.mjs`): **~100 `CLAUDE_CODE_*` environment variables enumerated with defaults** — the single most reusable artifact in the repo: a free coverage checklist for our ecosys/config surface (zcode consumes the same families). This is borrow #14's core.
- **Scheduler ≠ router (correction to the README-level draft):** `core/scheduler.mjs` is a **cron** task scheduler (`~/.claude/scheduled_tasks.json`, interval shorthand + cron fallback) — it is not the model router. The cost-cascade router lives in `optimize/`.
- **Cost-cascade** (`optimize/router.mjs` + `store.mjs`): **zero-dependency, deterministic, makes no model calls.** Ladder haiku ($1/MTok)/sonnet ($9)/opus ($45); heuristic complexity prior (length + signal words); recorded-outcome blending after ≥3 samples (weight cap 0.9) vs quality bar 0.7; `escalate()` exists but the loop never calls it mid-task — it learns per task, cheapest-first, records cost/latency/success to a JSONL store, re-routes the *next* task. **This is structurally our scheduler's tier/fallback idea plus a feedback loop — and it is invariant-clean (no LLM between loop and provider), so redline 2 is not implicated the way the README first suggested.**
- **Hooks** (`hooks/engine.mjs`, 170 lines): PreToolUse/PostToolUse/Stop with allow/deny — same lifecycle names as our hookdag input surface.
- **OAuth** (`auth/oauth.mjs`): PKCE auth-code + device flow + refresh + credentials in `~/.claude/credentials`, presets for anthropic/codex/openai — reference for our credential UX (phase 07).
- **Tools**: 25 built-in tools registered (`tools/registry.mjs`), names matching Claude Code exactly; several are partial (**LSP is an explicit stub**; multi-edit/web-fetch are thin). Census caveat: name presence ≠ behavior depth.

**Borrow (highest-value finds in the whole audit for our parity/EVAL and scheduler work).**
- **NIGHTLY-PARITY** (`.github/workflows/nightly.yml`, ADR-001): daily 03:00 UTC → detect upstream npm release (`scripts/check-claude-release.sh` + `last-known-claude-version.txt`) → full gate (991 tests + npm audit + lint + smoke) → AI discovery analysis (Claude Sonnet 4.6) → publish verified nightly tag only on green; failure blocks and opens issues. **This is the missing operational half of our parity harness**: today ours is one-shot capture/compare; a re-run gate that fires whenever the tracked target (zcode) ships makes mimicry honest over time. Directly portable to Phase 8/12 EVAL-01..03.
- **SURFACE-INVENTORY** (`env.mjs` ~100 vars; 25 tool names; 6 permission-mode names; 4 MCP transports; 40 slash commands): free coverage checklist for toolcat/profile/config — companion to Claudecourse's 34-section checklist (borrow #11).
- **COST-CASCADE OUTCOME STORE** (`optimize/`): deterministic cost-aware ladder + recorded per-task outcomes + self-tuning predictions — validate our scheduler design and add the one genuinely new idea: **outcome store + feedback loop** (ECON extension). Their loop records (agent-loop `recordOptOutcome`) exactly the events our audit bus already emits — implementation is a store + one engine hook.
- **MCP-4-TRANSPORT**: stdio + SSE + Streamable HTTP + WebSocket client breadth as a future `internal/mcp` extension (we start stdio-only per PLUG scope; same `mcp__` catalog shape; adopt our newer protocol version, not theirs).
- **REGEN-WIRE-INS** (`scripts/auto-update-capabilities.mjs`): LLM-driven upstream sync guarded by PROTECTED_PATHS, required wire-in markers, fail-closed revert — a mature pattern for future auto-update of profile bundles against a tracked target version. (**Their use is LLM-generating code; our analogue would be profile/manifest regeneration** — same guardrails, different payload.)

**Reject.** the stack (Node CLI; no ACP; no IDE surface — ours is Go + ACP); the 6 permission modes as a feature (our safety model unchanged); copying its request-shaping — a fidelity verdict, not a legal one: no `cache_control`, lossy OpenAI/Google conversions, stub providers, census-grade only; writing into `~/.claude` (redline 4); their stale protocol version and stub transports.

---

## 2. Substrate Verdicts (all reaffirmed, none reopened)

**The architecture stands: hand-rolled Go, provider-level SDKs, shaper owns the wire.** Survey findings that support it:

- **ADK Go** — Gemini-coupled (`model.LLMRequest` is `genai.*` types; backends = gemini/apigee only; Anthropic/OpenAI = open issue #596; tool packer internal, issue #1054). Even relaxed to Python (LiteLLM IR) the chokepoint problem persists.
- **MAF Go** — best-mannered framework (wraps our own `anthropic-sdk-go`, has a `MessageNewParams` raw-params valve) but: MAF content-parts IR is lossy by design, `Instructions` is one string, TypeBox-equivalent schema generation, preview-status drift. Public preview Jul 2026.
- **Claude Agent SDK / headless supervision / OpenCode fork / TS composite** — the relaxed-Go option map (own-the-wire ↔ inherit-the-wire) was mapped; **all rejected for now**, with recorded triggers (§6). No framework beats owning the chokepoint (`internal/shaper`, MIMC-01).
- **Extraction verdicts:** OpenCode = process boundary only (internals not importable; ACP bridge at `packages/opencode/src/acp/` is TS); pi = genuinely extractable as dependency libs (`ai`/`agent`/`protocol` published, `createProvider({api})` seam, `transformHeaders`, `onPayload`) with the Context-IR fidelity question as the gating spike; **WASM-bridging TS libs into Go = rejected** (Javy/QuickJS shims Node surface = fork-in-disguise; import value ≈ 0 since we already own loop+provider+headers).
- **Serena Go port = rejected** (sizing: 62k Python lines; `solidlsp` alone 41.8k incl. 74 per-language server managers at 26.5k; scoped 3-lang port ≈ 6–10 weeks but removes only the Python runtime — Node remains for pyright/tsserver; and native-named tools would diverge the catalog from any zcode configuration). Host as MCP instead (§4.2).

---

## 3. Idea Audit (amended 2026-08-15: plugin marketplace REVERSED to needed; memory DESIGNED)

### 3.1 Ideas we already implemented (sources validate)

| Idea | Source | Ours |
|---|---|---|
| Agent = model+tools+loop; harness around it | Claudecourse #1–3 | `internal/session` loop; engine/hookdag/audit/session layers |
| MCP hosting, `mcp__<server>__<tool>` naming, never cache `tools/list` | Claudecourse #16; opencode; pi | ECOS-01..03 |
| Subagents: restricted tools, results-only, bounded concurrency, panic recovery | Claudecourse #21; opencode agents | PARA-01..04 |
| Read-only parallel / mutating serialized | Claudecourse #7 | toolcat Mutability + PARA dispatch |
| Append-only transcript, resume | Claudecourse #24 | SESS-05/06, `session/load` |
| SSE streaming; tool_use assembled before exec | Claudecourse #31 | provider streaming |
| Provider fallback chains | Claudecourse #30 | scheduler |
| Verbatim-request observability | Claudecourse #26 (variant) | `internal/audit` (redacted shaped requests — stronger than OTel for the mimicry question) |

### 3.2 Ideas we planned (v1.1) — sources confirm priority

Skills on demand (CMD-06) ← Claudecourse #20, pi cache-aware skills; WebSearch/WebFetch (CMD-07); command semantics (CMD-02); correlated observability (AUD-02..04); chat peer + voice (TG-01..06) ← OpenClaw; resume across restarts (TG-02); post-turn autonomy (ENG) — **ours is the strongest design of any source: one decision engine, not bolt-on scripts**.

### 3.3 Ideas we should borrow (ranked; → §4 clusters)

1. **Snapshots + undo** (opencode) — pairs with no-confirmation safety: no gate before, reversible after. Today a bad mutating turn is unrecoverable.
2. **Behavioral evals pyramid** (Claudecourse #29; pi ships an `evals` package) — we prove fidelity (parity) but zero capability. Biggest methodological hole: Phase 8 changes turn behavior with no regression net beyond UAT.
3. **Prompt-cache discipline** (Claudecourse #28; pi) — parity proves structure, not cache-hit behavior; dynamic merges (skills, MCP tools) can be structurally perfect and still bust zcode's cache economics.
4. **LSP via Serena-as-MCP** (amended from "native LSP") — zero code through `internal/mcp`; catalog grows `mcp__serena__*` = zcode-with-serena; AUD-05's MCP-attach capture covers parity.
5. **Token economics** (Claudecourse #27) — explicit light-tier routing for subagent turns; tool-output truncation policy.
6. **Uniform tool contract** (Claudecourse #5/#30) — `is_error` in tool_result, per-tool timeouts, retry-only-transient, `isConcurrencySafe`/`isDestructive` flags feeding engine + parallel dispatch.
7. **Memory system** (Serena lightweight tier + Hindsight design) — see §4.3.
8. **Session tree / branch-at-boundary** (pi) — boundaries become fork points; SDD side-quests without context pollution.
9. **OTel span export** (Claudecourse #26; pi telemetry) — event bus is 80% there; session→turn→api_call→tool_use.
10. **Background heartbeat turns** (OpenClaw) — cron-driven runs motorize LRN improvement proposals.
11. **Plugin/marketplace compatibility** (user-directed reversal of the original "not needed" verdict) — see §4.1.
12. **Nightly upstream-parity gate** (occ ADR-001) — re-run the EVAL/parity suite whenever the tracked target (zcode) ships; one-shot capture becomes a living gate. §1.1.
13. **Scheduler outcome feedback loop** (occ `optimize/` — deterministic, zero LLM calls, invariant-clean) — record real per-task cost/latency/success; ECON grows a self-tuning loop. §1.1.
14. **Claude-Code surface census** (occ `env.mjs` ~100 vars, 25 tool names, 6 permission-mode names, 4 MCP transports; OpenClaude provider matrix) — free coverage checklist for toolcat + profile + config surface; companion to #11. §1.1.

### 3.4 Ideas we do not need (redlines in §6)

Permission gates / 4 modes / HITL prompts (structural safety-model decision); HITL injection defenses (keep the hygiene half — `internal/redact`); autoCompact summarization (replaced by boundary resets; watch long-single-turn edge); TUI/REPL (no terminal surface); 15-provider breadth (2 shapes suffice); SDK-as-product (ACP is our client surface); team protocols/remote subagents/worktree isolation (revisit only for parallel SDD stages); hierarchical CLAUDE.md machinery (arrives via mimicry; HOOK-02 memory covers ours).

---

## 4. Proposed Requirement Clusters (drafted for next milestone; v1.1-style)

### 4.1 PLUG — Claude-Code-compatible plugin & marketplace ecosystem

Ground truth: zcode itself consumes `claude-plugins-official` (git, 254 plugins) AND `zcode-plugins-official` (URL marketplace.json) through one mechanism — the format is proven runtime-portable; ass-guard becomes the next consumer, not an innovator.

- **PLUG-01 (consume)**: Installed plugins load: `installed_plugins.json` (v1 registry: `name@marketplace`, installPath, scope) + `<root>/plugins/cache/<marketplace>/<plugin>/<version>/` layout; `.claude-plugin/plugin.json` manifests parsed; bundled `skills/`, `commands/`, `agents/`, `hooks/hooks.json`, `.mcp.json` merge into existing discovery (ecosys) with documented precedence (user/project `.claude/agents/` override plugin agents; plugin skills namespaced `plugin:skill`).
- **PLUG-02 (marketplace format)**: `marketplace.json` parsed (`name/description/owner/renames/plugins[]` with `source`: relative path | `git-subdir{url,path}` | github ref); marketplaces clone/fetch to `<root>/plugins/marketplaces/<id>/`; `known_marketplaces.json` maintained (v1 schema, github|url sources).
- **PLUG-03 (lifecycle)**: `plugin marketplace add/list/remove`, `plugin install/uninstall/update` (version-bump detection; git-subdir resolution) surfaced as slash commands on ACP and Telegram.
- **PLUG-04 (write boundary)**: reads span `~/.claude/plugins/` (Claude Code installs); writes land ONLY under the ass-guard root (zcode's own `~/.zcode/` pattern — same format, separate root, D-05/D-06 extended).
- **PLUG-05 (v1.1 carve-out, cheap)**: CMD-01/CMD-06 discovery widened to read `installed_plugins.json` and merge plugin skills/commands — the 80% case lands inside Phase 8; full lifecycle (PLUG-03) waits for the next milestone.
- **PLUG-06 (late surfaces)**: `.lsp.json` + `monitors/` support tracked here (the plugin ecosystem's delivery vehicle for audit-borrow items; interact with §4.2/§4.4 before scheduling).
- Out of scope: Anthropic community pipeline, SHA-pinning CI, third-party MCP registries (Smithery/Docker/PulseMCP), the `/plugin` TUI.
- Note: Serena's README warns marketplace install configs are outdated vs canonical per-project config — prefer canonical config when they diverge.

### 4.2 LSP — Serena hosted as MCP (no port; triggers recorded)

- **LSP-01**: Serena runs as a user-configured MCP server (`uv tool install serena-agent`; stdio) through existing `internal/mcp`; its basic utilities (shell/file tools) disabled in-harness; symbol tools (`find_symbol`, `find_referencing_symbols`, symbol body edit, rename, diagnostics) bridge as `mcp__serena__*` — catalog shape identical to zcode-with-serena.
- **LSP-02**: AUD-05 parity runbook includes a serena-attached capture scenario (MCP-attach workload already planned — pin it).
- **Re-port triggers (any one fires → revisit scoped Go port, gopls-first, MCP-shaped names, ~3–4 weeks Go-only / 6–10 weeks +TS/Py)**: operator-visible Python/uv pain in real deployments; engine integration genuinely blocked by MCP round-trips.
- Redline: never expose semantic tools under native (non-`mcp__`) names — no zcode configuration produces that catalog.

### 4.3 MEM — two-tier memory (Serena lightweight; Hindsight-informed design)

Hindsight's taxonomy maps onto machinery we already have — Experiences = audit transcript (verbatim turns + AUD-04 engine decisions, richer than chat logs); Retain = HOOK-02 post-implement memory step; Reflect = HOOK-02 post-phase improvement-proposals; Recall = engine input; Mental Models = LRN store.

- **MEM-01 (tier 1, built-in)**: Serena-style per-project markdown memories, tool-gated (progressive disclosure, cache-friendly, no system-prompt mutation — MCP/tools only), with an onboarding-mode equivalent driven by the engine.
- **MEM-02 (tier 2, optional)**: external memory service (Hindsight-shaped: World/Experiences/Mental-Models banks, temporal queries) integrated ONLY via hook-DAG steps (HTTP) or a thin MCP shim — operator-hosted, never in-process.
- **MEM-03**: Retain-extraction LLM calls route through scheduler `light` tier.
- **MEM-04 (design borrow)**: memory entries carry turn/session correlation IDs (audit linkage) and timestamps (temporal recall).
- **Redline**: Hindsight's LLM-wrapper auto-capture (wrap the client, intercept every call) is the one surveyed integration that structurally violates mimicry — never adopt.

### 4.4 Small borrow items (individual requirements or fold-ins)

- **SNAP**: file-state snapshots at turn/boundary start + undo surface (ACP + Telegram command). Pairs with no-confirmation safety model.
- **EVAL-01..03**: behavioral eval pyramid — deterministic tool unit tests → scenario suites (`/opsx` pass@k against a scratch project with the real binary) → re-run gate on profile/model/turn-behavior changes. Land alongside Phase 8 if possible (regression net for command kickoff).
- **CACHE**: cache-discipline checks — dynamic merges respect stable→volatile ordering; add a cache-hit-behavior probe (not just structural parity) to the parity harness.
- **ECON**: explicit `light`-tier routing for subagent turns; tool-output truncation policy (tail/grep).
- **TOOLCON**: uniform tool contract — `is_error` in tool_result, per-tool timeouts, retry-only-transient (429/5xx/timeout), `isConcurrencySafe`/`isDestructive` flags.
- **TREE**: boundaries as fork points (session tree, pi-style) — design note for Session Core evolution.
- **OTEL**: span export from the event bus (session→turn→api_call→tool_use).
- **HEART**: cron/heartbeat engine runs (motorize LRN improvement proposals; OpenClaw precedent).

---

## 5. Near-term tasks (fold into current v1.1 where cheap)

1. **Shaper ↔ pi cross-validation** (days, zero commitment): diff our `internal/shaper` behaviors against pi's `packages/ai/src/api/anthropic-messages.ts` (1,370 lines) + `transform-messages.ts` — cache_control placement, thinking-config mapping, header merge order, compat-flag catalog. pi's 43k-line test suite is a free behavioral spec. This hardens exactly the component the north star depends on.
2. **CMD-06 plugin discovery carve-out** (PLUG-05 above).
3. **AUD-05 runbook**: pin the MCP-attach capture to include serena (LSP-02) — one scenario, reused forever.

---

## 6. Redlines (survive all future planning)

1. The wire chokepoint stays in `internal/shaper` — no framework IR (genai, MAF content-parts, Vercel AI SDK, LiteLLM) between profile and bytes.
2. No LLM-wrapper/memory layer ever sits between the turn loop and the provider.
3. Semantic/added tools register only under MCP-shaped names the mimicry target can also produce (`mcp__server__tool`).
4. ass-guard never writes inside `~/.claude/` — reads yes, writes to our own root only.
5. No WASM/JS-engine bridges for TS libraries in the Go build (fork-in-disguise shims; import value ≈ 0 given what we own).
6. No full Serena/framework ports without a fired trigger condition (§4.2, §2).
7. No confirmation tier (unchanged v1.0 safety model — pattern/hook table + manual cancellation; borrow reversibility, not gates).
8. Mimicry profile content derives only from the target's own wire traffic (zcode JSONL/MITM) — PROJECT.md's log-extraction principle restated. **This is a fidelity rule, not a reading ban: studying any third-party code is allowed (operator ruling 2026-08-19)** — foreign trees are the wrong target (OpenClaude ≈ Claude Code, not zcode) or deliberately non-faithful (occ), so they cannot substitute for the target's own traffic as profile ground truth. One scoped legal guard: no mechanical line-by-line porting from OpenClaude's tree specifically — its NOTICE declares unauthorized redistribution of Anthropic's proprietary source, and derivative-work risk survives translation to another language. Idea-level study of it (and everything else) is unaffected.

---

## 7. Sizing Evidence (measured 2026-08-15)

| Artifact | Measurement |
|---|---|
| pi interesting subset | `anthropic-messages.ts` 1,370 + `transform-messages.ts` 223 + types/protocol ≈ 2k lines TS to read; full `ai`+`agent`+`protocol` = 37k src / 43k test |
| pi loop | `agent-loop.ts` = 796 lines (ours: `internal/session` 1,527) |
| serena | 19.5k agent layer + 41.8k `solidlsp` (74 lang servers = 26.5k; core 14.9k) ≈ 62k Python |
| opencode | TS/Bun server + Go TUI; ACP at `packages/opencode/src/acp/` (TS, on Zed's official SDK); internals not importable |
| openclaude (added 2026-08-19) | 2,591 TS files / 684,830 lines `src/` (shallow-clone measured 2026-08-19); docs + targeted source reading fine (ruling 2026-08-19); no mechanical porting from this tree |
| open-claude-code (added 2026-08-19) | 144 `.mjs` / 9,772 lines total (core 1,959, tools 2,293, ui 1,691, optimize 649, mcp 648, config 547, permissions 521); 991/991 tests; readable end-to-end |
| zcode plugin ground truth | `~/.zcode/cli/plugins/{installed_plugins.json, known_marketplaces.json, cache/<mkt>/<plugin>/<ver>/, marketplaces/<id>/}`; both github and URL marketplace sources live |

## 8. Sources

- Claudecourse: github.com/justxor/Claudecourse (36-point harness checklist)
- OpenCode: opencode.ai/docs (ACP), github.com/anomalyco/opencode, cefboud.com deep-dive, PRs #2422/#2947
- pi: pi.dev, github.com/earendil-works/pi (+ packages/ai provider README), lucumr.pocoo.org/2026/1/31/pi/
- OpenClaw: openclaw.ai, docs.openclaw.ai, github.com/openclaw/openclaw, github.com/openclaw/mcporter
- Serena: github.com/oraios/serena (README + shallow clone measurement)
- Hindsight: github.com/vectorize-io/hindsight (README)
- Frameworks: adk.dev/2.0 + adk-go issues #596/#1054/#619/#480; pkg.go.dev/google.golang.org/adk/model; learn.microsoft.com Agent Framework (Anthropic provider, Go tab); devblogs.microsoft.com Go preview; strandsagents.com; code.claude.com/docs (plugins, agent-sdk)
- Ecosystem: morphllm.com ACP overview (native agents: Gemini CLI, Goose, OpenCode, Qwen Code, Kimi CLI, OpenHands, Augment)
- OpenClaude (2026-08-19, shallow clone): github.com/Gitlawb/openclaude (README, LICENSE/NOTICE — explicit unlicensed Anthropic code; docs/agent-routing.md, docs/repo-map.md; GitHub API: ~30.8k★, TS, created 2026-04-01)
- Open Claude Code (2026-08-19, shallow clone): github.com/ruvnet/open-claude-code (v2/src: core/agent-loop.mjs, core/providers.mjs, core/context-manager.mjs, core/session.mjs, mcp/client.mjs, config/env.mjs, optimize/router.mjs + store.mjs, permissions/checker.mjs, .github/workflows/nightly.yml); github.com/ruvnet/rudevolution (decompiler, 34,759 fns @ 95.7%)
