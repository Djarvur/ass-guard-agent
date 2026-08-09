<!-- GSD:project-start source:PROJECT.md -->
## Project

**ass-guard-agent (working name)**

A Go-based AI coding agent that makes SDD (Spec-Driven Development) workflows run hands-off by automatically doing the routine work a developer keeps forgetting to ask for. It speaks to model providers with requests structured like another agent (the first mimicry target is zcode — a claude-code-compat runtime shipping its own `AGENTS.md`, skills, commands, MCP, and tool catalog), so the model behaves identically to how it behaves in the mimicked agent. The agent hosts unmodified SDD toolkits (OpenSpec in v1), drives them to completion without manual "continue" taps, and runs configurable post-stage routines (review / memory / tests / linters / improvement proposals) on its own. Primary interface is ACP (IDE-native, e.g. Zed); Telegram is a full peer surface (text + voice). Built for SDD-capable teams who want the toolkit to just run.

**Core Value:** Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first) — if the model can tell the requests apart, everything built on top is compromised, because model behavior diverges. Every other capability (autocontinue, hooks, scheduling, interfaces) is downstream of this.

### Constraints

- **Tech stack**: Go (single static binary; first-class concurrency for parallel subagents; mature streaming HTTP clients) — load-bearing, not stylistic
- **Transport discipline**: stdout reserved exclusively for ACP JSON-RPC frames; all logging/diagnostics to stderr (non-negotiable LSP-style discipline)
- **Compatibility**: Claude Code config layout drop-in (existing `.claude/` setups work unchanged)
- **Distribution**: Static binary via goreleaser; ACP registry manifest; no daemon, no network port (the editor owns process lifecycle)
- **Platform**: macOS + Linux, amd64 + arm64 — Windows deferred
- **Safety model**: No tool-execution confirmation tier; the pattern/hook table (no match → nothing runs) plus manual cancellation is the only safety mechanism
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## How To Read This Document
- **HIGH** — multiple independent current sources agree; the choice is uncontroversial for the stated use case; safe to build against.
- **MEDIUM** — solid primary source or strong precedent, but either the library is newer (less battle-testing) or there is one open verification item to close in a Phase-0 spike. Build against it; budget a half-day fallback.
- **LOW** — plausible default but genuine uncertainty; spike before committing.
## Recommended Stack
### Core Technologies
#### Inherited Core Technologies (verified current as of Aug 2026)
| Technology | Version | Purpose | Confidence | Why Recommended |
|------------|---------|---------|------------|-----------------|
| Go | 1.25.x (or 1.26.x if released) | Implementation language | **HIGH** | Single static binary distribution (the editor spawns the agent as a subprocess — zero runtime deps for the host); first-class concurrency (goroutines map to subagent fan-out); mature streaming HTTP for model inference. The predecessor pinned 1.23+; 1.23 is now end-of-support per Go's two-release policy — **bump the floor to 1.25**. Verified: 1.26 is referenced as the Feb 2026 release; 1.27 ~Aug 2026. Pin `go 1.25` in go.mod as the supported floor. |
| ACP (Agent Client Protocol) v1 | spec v1 (v2 is Draft, not stable) | The primary IDE-native interface | **HIGH** | JSON-RPC 2.0 over stdio; the editor (Zed, JetBrains) spawns the agent as a subprocess; stdout reserved for protocol frames, all logging to stderr (non-negotiable LSP-style discipline). Verified: v1 is the current stable spec at agentclientprotocol.com; **v2 is in Draft** (https://agentclientprotocol.com/announcements/acp-v2-draft) — pin to v1 for ass-guard's v1. Method names (`initialize`, `session/prompt`, `session/update`, `session/load`) confirmed against the canonical spec pages. |
| `anthropics/anthropic-sdk-go` | v1.62.0 (Jul 2026) | Anthropic-shape provider client (also covers GLM via Z.ai) | **HIGH** | First-party Go SDK; supports streaming (SSE), native `tool_use` blocks, configurable base URL, and a tool-loop helper. **Currency verified:** v1.62.0 released with `mid-conversation-tool-changes-2026-07-01` beta + session budgets (GitHub Releases page, Jul 2026). Point the base URL at `https://api.z.ai/api/anthropic` for GLM — Z.ai's "GLM Coding Plan" explicitly supports both Anthropic and OpenAI protocols (verified Z.ai dev docs). One SDK covers Claude + GLM with no separate GLM client needed. |
| `sashabaranov/go-openai` | latest (1.x; last pkg.go.dev publish Aug 2025, repo active 2026) | OpenAI-shape provider client (MiniMax M3, others) | **MEDIUM** | The de-facto Go client for OpenAI-compatible endpoints. ~10.7k stars, references GPT-5/5.5 in repo description (2026-active). Used for OpenAI-shape providers (MiniMax M3 "apply" route, OpenRouter passthrough). **Caveat:** publish cadence on pkg.go.dev looks ~1 year stale vs. GitHub repo activity — pin to the latest tag at build time and verify the tool-calling schema matches the target provider's expectation during Phase 0. If a specific provider misbehaves, the thin-adapter design (inherited) keeps blast-radius small. |
| JSON-RPC 2.0 framing | hand-rolled (primary); `go.lsp.dev/jsonrpc2` (optional) | ACP wire framing | **HIGH** (hand-rolled) / **MEDIUM** (`go.lsp.dev`) | The predecessor flagged `go.lsp.dev/jsonrpc2` as "effectively unmaintained." **2026 verification confirms:** upstream is quiet; a 2026 fork `github.com/kwo/jsonrpc2` (`v0.0.0-20260410…`) exists as a maintained fallback. **Recommendation: hand-roll the newline-delimited JSON-RPC reader/writer/dispatcher.** It is ~150 lines of trivial Go, gives full control over ACP's notification semantics (which `go.lsp.dev` was only ever a partial fit for), removes a stagnant dep, and matches the LSP-in-Go lineage the architecture follows. The framing protocol is small; a library buys little and costs maintenance risk. See "What NOT to Use" below. |
| `goreleaser` | v2.17 (2026) | Cross-platform static binary release | **HIGH** | The standard for Go release engineering. **Verified current:** v2.17 is the latest stable (goreleaser.com, 2026). Produces macOS+Linux amd64+arm64 tarballs/archives from one config. Also generates the ACP registry `agent.json` packaging step (the registry just names the spawn command + args; goreleaser ships the binary the registry points at). |
| Claude Code `.claude/` config layout | (convention, not versioned) | Drop-in ecosystem compat | **HIGH** | `.claude/settings.json` (user/project), `CLAUDE.md`/`AGENTS.md` hierarchy, `.claude/commands/`, `.claude/agents/`, `.claude/skills/`, `.mcp.json` (project-scoped MCP). ass-guard reuses this layout so existing Claude-Code setups work unchanged; ass-guard's additions namespace under `.claude/ass-guard/` (or a clearly-namespaced settings key) — never clobber Claude Code's files. Verified: multiple 2026 secondary sources confirm the layout; `~/.claude.json` is the global config (history, project settings, user-scoped MCP). |
#### New Core Technologies (the five ass-guard focus areas)
| Technology | Version | Purpose | Confidence | Why Recommended |
|------------|---------|---------|------------|-----------------|
| **Mimicry capture: JSONL transcript harvest + MITM proxy** | (no library — pattern) | Extract the zcode profile (system prompts, tool catalog, message shape, identity) from ground-truth logs | **HIGH** | The north star requires grounded mimicry (hand-written profiles are guesses). claude-code-compat runtimes (including zcode) write full message-level transcripts as JSONL at `~/.claude/projects/<munged-cwd>/<session-id>.jsonl` (one JSON object per line, capturing messages + tool calls + metadata). For deeper "what is actually on the wire" capture, MITM proxies exist purpose-built for AI agents (LLM Interceptor, Sherlock, Claude Inspector) — see Focus 1 below. |
| **Mimicry expression: profile = config bundle** | (no library — internal types) | `{system prompts, tool catalog, message shape, identity}` per profile | **HIGH** | A profile is a config-driven struct, not code. The Turn Loop reads the active profile and shapes every outgoing provider request from it. zcode is profile #1; the architecture supports N from day one (decided in PROJECT.md). See Focus 1. |
| **Model scheduling: internal config-table resolver (no routing library)** | (no library — internal) | Tier abstraction (heavy/good/light), time-windowed substitution, per-project override, fallback chains | **HIGH** | Surveyed LiteLLM router, OpenRouter routing, Portkey — all are **Python or hosted-proxy** products, not Go-embeddable libraries. There is **no mature Go library for multi-provider model routing** (verified Aug 2026). The right 2026 call: a small in-process resolver over a config table — see Focus 2. |
| **Telegram: `go-telegram/bot`** | latest (active 2026) | Telegram peer interface (text + voice) | **HIGH** | Among gotgbot / telebot / go-telegram-bot-api / go-telegram/bot, **`go-telegram/bot` wins for ass-guard**: zero-dependency, idiomatic `context.Context` throughout (load-bearing for clean cancellation/shutdown when the ACP server owns process lifecycle), modern Go conventions, handler-based API. gotgbot is a fine alternative (code-generated, dispatcher pattern) but its Python-inspired shape is a worse fit for a process that must coexist with an ACP stdio server. See Focus 3. |
| **STT (voice → text): pluggable backend, OpenAI Whisper API default** | (configurable) | Transcribe Telegram voice messages to ordinary user input | **MEDIUM** (default) / **HIGH** (architecture) | Three viable backends: (1) **OpenAI Whisper API** (default — drop-in via the OpenAI-shape provider client, no extra dep), (2) **whisper.cpp local** via the official Go binding `github.com/ggml-org/whisper.cpp/bindings/go` for offline/privacy, (3) **Groq STT** for low-latency. Backend is configurable; v1 ships the OpenAI default and the interface accommodates the others. See Focus 3. |
| **Hook-DAG engine: hand-rolled in-process executor** | (no library — internal) | Configurable post-stage routine DAG (run-command / send-prompt / fresh-context / wait) | **HIGH** | No mature, lightweight, in-process Go DAG library fits (Temporal/Argo are distributed heavy iron; see Focus 4). The step types are few and well-defined. A ~300-line executor over a config-driven DAG is the right call — matches GitHub-Actions-style YAML semantics in a Go-native shape. |
| **MCP hosting: `modelcontextprotocol/go-sdk`** | v1.0.0+ (official, Google-collaborated) | Host Claude-Code-installed MCP servers as subprocesses; bridge their tools into ass-guard's catalog | **HIGH** | The **official Go MCP SDK** reached v1.0.0 in 2026 (maintained in collaboration with Google, in the `modelcontextprotocol` org). It ships both client and server: `StdioMCPClient` launches an MCP server as a subprocess and speaks JSON-RPC over its stdin/stdout — exactly the host pattern ass-guard needs. Supersedes the community `mark3labs/mcp-go` for new builds (mark3labs influenced the official SDK and remains a fine library, but official now wins on spec-compliance + long-term support). Protocol version 2026-07-28. See Focus 5. |
### Supporting Libraries
#### Inherited Supporting Libraries (verified current)
| Library | Version | Purpose | Confidence | When to Use |
|---------|---------|---------|------------|-------------|
| `spf13/cobra` | latest (v1.x, active) | CLI surface (the `acp` subcommand for the registry; any future CLI flags) | **HIGH** | Always — the ACP registry spawns `ass-guard acp` (or similar); cobra structures that entrypoint. Idiomatic, uncontroversial. |
| `spf13/viper` | latest (v1.x, active) | Config loading (settings hierarchy: defaults → user `.claude/` → project `.claude/`) | **HIGH** | Always — ass-guard layers Claude-Code-compat config + its own namespaced keys. Viper handles the precedence stack cleanly. |
| `log/slog` (stdlib) | Go 1.21+ | Structured logging → stderr | **HIGH** | Always. Zero-dep, structured, stdlib. **Critical:** all log output goes to stderr; stdout is reserved for ACP frames. |
| `golangci-lint` | latest (v1.x+, active) | Linting | **HIGH** | Always — standard Go linting. |
| `stretchr/testify` | latest (v1.x, active) | Test assertions | **MEDIUM** | Optional — table-driven tests + assertions. The predecessor flagged this; stdlib `testing` alone is also fine. |
| `JohannesKaufmann/html-to-markdown` | latest (active) | `WebFetch` HTML→markdown conversion (ddg-search port) | **HIGH** | For the `WebFetch` built-in tool. Mature, used by scrapers widely. |
| `golang.org/x/net/html` | latest (golang.org/x) | HTML parsing for scrape/fetch | **HIGH** | Supporting the WebSearch/WebFetch port from ddg-search. |
#### New Supporting Libraries
| Library | Version | Purpose | Confidence | When to Use |
|---------|---------|---------|------------|-------------|
| `github.com/go-telegram/bot` | latest (active 2026) | Telegram bot framework | **HIGH** | Always for the Telegram peer. See Focus 3. |
| `github.com/ggml-org/whisper.cpp/bindings/go` | tracks whisper.cpp | Local whisper.cpp STT (offline/privacy backend) | **LOW** | Optional — only if the configured STT backend is `whisper-cpp-local`. Adds a C dependency to the build (breaks pure-Go static binary claim — verify goreleaser cross-compile impact before committing). The OpenAI Whisper API default avoids this entirely. |
| `github.com/modelcontextprotocol/go-sdk` | v1.0.0+ | MCP client + server | **HIGH** | Always — hosts Claude-Code-installed MCP servers. See Focus 5. |
| `github.com/kwo/jsonrpc2` | `v0.0.0-20260410…` | **Fallback only** — maintained 2026 fork of `go.lsp.dev/jsonrpc2` | **MEDIUM** | Only if a future need makes hand-rolled framing painful (e.g. complex bidirectional notification routing). Default is hand-rolled — see "What NOT to Use." |
| Internal: `internal/profile` | (project package) | Profile types + loader | **HIGH** | Always — the mimicry profile mechanism. |
| Internal: `internal/scheduler` | (project package) | Tier/time-window/fallback resolver | **HIGH** | Always — the model scheduling layer. |
| Internal: `internal/hookdag` | (project package) | In-process DAG executor | **HIGH** | Always — the "forgotten routine" engine. |
### Development Tools
| Tool | Purpose | Notes |
|------|---------|-------|
| `go` 1.25+ | Build/test | Pin `go 1.25` in go.mod as the floor (1.23 is EOL). |
| `goreleaser` v2.17 | Release | macOS+Linux, amd64+arm64; produces the binary the ACP registry manifest points at. |
| `golangci-lint` | Lint | Standard linters; enforce the stdout/stderr discipline via a custom check or review gate. |
| `mitmproxy` / LLM Interceptor / Sherlock | **Profile extraction** (dev-time, not shipped) | Run during Phase 1 to capture zcode's actual outgoing requests → ground the zcode profile. Not a runtime dependency. |
| ACP log viewer (Zed `dev: open acp logs`) | Debug ACP traffic | Essential during Phase 0 (ACP skeleton). |
| `claude-code-log` (Python CLI) | Read Claude-Code-compat JSONL transcripts during profile authoring | Converts JSONL → readable HTML/Markdown. Dev-time tool, not shipped. |
## Installation
# Initialize (once)
# Inherited core
# (stdlib log/slog, net/http, regexp, encoding/json — no go get needed)
# New for ass-guard scope
# Optional / backend-conditional
# (hand-rolled JSON-RPC framing + hook-DAG + profile + scheduler: no external dep)
# Dev
## The Five New Focus Areas (deep dives)
### Focus 1 — Mimicry / Profile Mechanism
#### A. Capture — where do agent request shapes come from?
#### B. Expression — how does ass-guard shape outgoing requests to match?
# profile: zcode (example shape — actual content harvested from logs)
### Focus 2 — Model Scheduling Layer
- Tiers (heavy/good/light ≈ opus/sonnet/haiku) abstract the concrete model.
- Tier→model mapping is **time-scheduled** (e.g. heavy = glm-5.2 normally, minimax-m3 in peak hours).
- **Per-project override** of the tier→model table.
- **Fallback chains** on provider error/limit (degrade tier, or walk a configured chain).
- **LiteLLM Router** — the canonical reference design (load balancing, retries, cooldowns, fallback chains across providers). **But it is Python and runs as a proxy server** — not embeddable in a Go binary. Its value to ass-guard is **as a design template**, not a dependency.
- **OpenRouter** — a hosted routing service (OpenAI-compatible endpoint). Useful as a *provider* ass-guard can route to, not as the routing logic itself.
- **Portkey** — hosted AI gateway. Same shape as OpenRouter: provider, not library.
- **Requesty** — hosted routing. Same.
### Focus 3 — Telegram Bot Stack in Go
#### A. Bot library
| Library | Shape | Context handling | Maintenance | Verdict |
|---|---|---|---|---|
| `github.com/go-telegram/bot` | Handler-based, zero-dep | **Idiomatic `context.Context` throughout** | Active 2026 | **Recommended** |
| `github.com/PaulSonOfLars/gotgbot/v2` | Updater/dispatcher (python-telegram-bot-inspired), code-generated | Good | Active 2026 (v2 published May 2026) | Strong alternative |
| `gopkg.in/telebot.v3` | Decorator-based, beginner-friendly | OK | Active | Easiest start; less idiomatic |
| `go-telegram-bot-api/telegram-bot-api` | Long-established | OK | **"A bit neglected" (maintainer's words)** | **Avoid** |
#### B. STT (voice → text)
| Backend | Library / path | Latency | Privacy | Notes |
|---|---|---|---|---|
| **OpenAI Whisper API** (default) | Reuse the OpenAI-shape provider client (`sashabaranov/go-openai`) — no extra dep | Low | Cloud | **Default for v1.** Drop-in; the same client handles chat + transcription. |
| **whisper.cpp local** | `github.com/ggml-org/whisper.cpp/bindings/go` (official Go binding) | Medium (hardware-dependent) | **Offline** | Optional. **Caveat:** adds a C build dependency — verify it does not break goreleaser cross-compilation for the static binary claim. If it does, ship whisper.cpp as an **out-of-process subprocess** (the agent shells out to the `whisper-cli` binary) rather than cgo-binding it. |
| **Groq STT** | OpenAI-compatible endpoint via `go-openai` (base URL swap) | **Very low** | Cloud | Optional; for latency-sensitive deployments. |
#### C. Architecture: Telegram peer + ACP stdio in one process
### Focus 4 — Hook-DAG / Configurable Pipeline Engine
- **Temporal** — the canonical durable-execution engine. **Confirmed too heavy:** it requires a server, persistence layer, and workers. Reddit r/golang consensus: Temporal is "awesome" for genuinely long-running complex workflows needing durability; "overkill" for embedded pipelines. **Violates ass-guard's "single static binary, no daemon" constraint.** Refute: do not use.
- **Argo Workflows** — Kubernetes-native. Wrong shape entirely (container orchestration).
- **Windmill** — fast self-hosted workflow engine, but it is a *product*, not an embeddable Go library.
- **go-task/task** — a Make-like build tool with task dependencies (DAG ordering). Single binary. **Useful as a design reference** for the YAML semantics + dependency ordering, but it is a CLI build tool, not an in-process library.
- **Flowpack/prunner** — the closest analog: an embeddable Go pipeline runner with an HTTP API, single binary, no DB. **Useful design reference**, but ass-guard's hook-DAG is simpler (4 step types, in-process, no HTTP API needed).
- **No lightweight, mature, in-process Go DAG library** is a clean fit. Verified Aug 2026.
- Topological execution with parallelism within a dependency-rank (test + lint run concurrently; review waits).
- Step failure → configurable (halt chain, or continue with error note). Mirrors GitHub Actions' `if: always()` / `if: failure()`.
- The whole DAG is triggered by the **unified engine** after turn-complete (PROJECT.md: autocontinue and hooks collapse into one decision engine; hooks are a special case).
- **Learning mode:** when an unfamiliar handoff appears, the engine asks ("fresh context? wait? how long?") and remembers → proposes a new hook DAG entry.
### Focus 5 — MCP Server Hosting in Go
- **`modelcontextprotocol/go-sdk`** — the **official Go SDK**, reached **v1.0.0** in 2026. Maintained in collaboration with **Google**, in the official `modelcontextprotocol` org. Supports protocol version **2026-07-28**. Ships both server and client, including **`StdioMCPClient`** — launches an MCP server as a subprocess, speaks JSON-RPC over its stdin/stdout. This is precisely the host pattern ass-guard needs.
- **`mark3labs/mcp-go`** — the community library (Ed Zynda) that **influenced the official SDK's design**. Still a fine, actively-maintained library with a high-level API; stronger for quick HTTP-transport server setup. But for a greenfield 2026 build where spec-compliance and long-term support matter, the official SDK wins.
- **`mcp package`** (pkg.go.dev) — the official reference; notes protocol 2026-07-28, with explicit guidance on consuming stderr for stdio servers (the same stdout/stderr discipline ass-guard already enforces for ACP).
## Alternatives Considered
| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Hand-rolled JSON-RPC framing | `github.com/kwo/jsonrpc2` (2026 fork of `go.lsp.dev/jsonrpc2`) | If bidirectional ACP notification routing grows complex enough that hand-rolling costs more than a dep. The fork is actively maintained (Apr 2026 timestamp). Default stays hand-rolled — the protocol is small. |
| Hand-rolled JSON-RPC framing | `sourcegraph/jsonrpc2` | Never for new builds — historically mature but updates have slowed; `go.lsp.dev` and its forks superseded it. |
| `go-telegram/bot` | `PaulSonOfLars/gotgbot/v2` | If you prefer a python-telegram-bot-style dispatcher/updater and want code-generated API-docs consistency. Active (May 2026 publish). Loses on idiomatic `context.Context`. |
| `go-telegram/bot` | `gopkg.in/telebot.v3` | For the fastest prototyping start (decorator syntax). Less idiomatic; not the best fit for a context-disciplined multi-frontend process. |
| OpenAI Whisper API (STT default) | whisper.cpp local (subprocess) | When privacy/offline is required. Run as a subprocess (`whisper-cli`), not cgo, to preserve the static binary. |
| OpenAI Whisper API (STT default) | Groq STT | When latency is critical (Groq is very fast). Same OpenAI-compatible client, base URL swap. |
| Internal scheduler resolver | LiteLLM Router (as a proxy sidecar) | **Never for v1** — violates "no daemon, single binary." LiteLLM's *design* (fallbacks, cooldowns) is worth studying as a template for the internal resolver. |
| Internal hook-DAG executor | Temporal / Argo / Windmill | **Never for ass-guard** — all are server/cluster products. Use go-task/task + prunner as design references only. |
| `modelcontextprotocol/go-sdk` (official) | `mark3labs/mcp-go` | If a specific feature in mark3labs is missing from the official SDK at build time. Otherwise the official SDK wins on spec-compliance + support. |
| `anthropics/anthropic-sdk-go` | hand-rolled Anthropic client | Never — the first-party SDK is actively maintained (v1.62.0, Jul 2026), supports streaming + tools + base URL swap. No reason to hand-roll. |
| `sashabaranov/go-openai` | `CherryHQ/openai-go` (community fork) | Only if the upstream stalls further. Currently go-openai is the standard (~10.7k stars, 2026-active). |
## What NOT to Use
| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `go.lsp.dev/jsonrpc2` (upstream) | Effectively unmaintained — the predecessor flagged this; 2026 verification confirms upstream is quiet. Scorecard security score 5.6/10. | Hand-rolled newline-delimited JSON-RPC framing (~150 lines). Fallback: `github.com/kwo/jsonrpc2` (2026 fork). |
| `sourcegraph/jsonrpc2` | Updates slowed; superseded by the `go.lsp.dev` line. | Hand-rolled framing (or `kwo/jsonrpc2` fork). |
| `go-telegram-bot-api/telegram-bot-api` (original) | Maintainer has publicly said it is "a bit neglected." | `github.com/go-telegram/bot` (idiomatic, context-aware, active). |
| Temporal (or any durable-workflow server) | Requires server + persistence + workers. Violates ass-guard's "single static binary, no daemon, no network port" constraint (PROJECT.md constraints). Confirmed too heavy by r/golang consensus. | Hand-rolled in-process DAG executor in `internal/hookdag`. |
| Argo Workflows | Kubernetes-container-native; completely wrong shape for an embedded agent pipeline. | Hand-rolled hook-DAG executor. |
| LiteLLM as a runtime dependency | It is a Python proxy server. Running it alongside a Go stdio agent breaks the static-binary + no-daemon constraints. | Internal scheduler resolver. **Do** study LiteLLM's router docs as a fallback-chain design template. |
| whisper.cpp via cgo binding (in the static binary) | Adds a C build dependency; risks breaking goreleaser cross-compilation for macOS+Linux amd64+arm64. | Ship whisper.cpp as an out-of-process subprocess (`whisper-cli`) if local STT is needed; default to OpenAI Whisper API (no extra dep). |
| `mark3labs/mcp-go` (for greenfield 2026 builds) | Not wrong — a fine library — but the official `modelcontextprotocol/go-sdk` (v1.0.0, Google-collaborated, protocol 2026-07-28) now wins on spec-compliance and long-term support. mark3labs influenced the official SDK. | `github.com/modelcontextprotocol/go-sdk` v1.0.0+. |
| Hand-written mimicry profiles (guessed content) | PROJECT.md key decision: profile content must be log-extracted, not hand-written — hand-written is a guess. | Harvest from JSONL transcripts (`~/.claude/projects/...`) + MITM proxy capture (LLM Interceptor / Sherlock / mitmproxy). |
| A standalone CLI surface | Out of scope (PROJECT.md): ACP (IDE) + Telegram are the only interfaces; no terminal REPL to maintain. | ACP stdio + Telegram peer, sharing one core. |
## Stack Patterns by Variant
- Load `profile: zcode` from config (harvested from zcode's JSONL transcripts + a MITM capture run).
- Outgoing requests assembled from the profile: system blocks, tool catalog, message shape, identity.
- The mimicry contract is the profile artifact; switching profiles switches the target.
- Run a one-shot MITM proxy capture (LLM Interceptor / Sherlock / mitmproxy with an Anthropic+OpenAI allowlist) against the target agent.
- Harvest the missing field, fold it into the profile.
- The runtime never depends on the proxy — it is dev-time extraction only.
- The scheduler resolver returns a primary target **plus a fallback chain** (degrade tier, or walk a configured provider chain).
- The turn loop walks the chain on error; the user sees an info `session/update` noting the degradation.
- The scheduler applies the time-window substitution before returning the target (e.g. heavy tier → minimax-m3 during 09:00–17:00 local, → glm-5.2 otherwise).
- Per-project override takes precedence over the global table.
- Telegram frontend runs as a goroutine inside the same process; both frontends share the core engine.
- On editor-initiated shutdown, `context.Context` cancellation drains the Telegram long-poll loop and any in-flight Telegram-driven turns.
- The process can be launched in Telegram-only mode (a cobra subcommand distinct from `acp`).
- Same core, only the frontend differs. (Note: this slightly tensions "no standalone CLI surface" in PROJECT.md — resolve at architecture time whether Telegram-only launch counts as a "CLI surface.")
- `modelcontextprotocol/go-sdk`'s `StdioMCPClient` launches it as a subprocess.
- Its tools register in ass-guard's catalog as `mcp__<server>__<tool>` (Claude-Code naming) — the profile's tool catalog already names them.
- The model invokes them like any built-in tool; the registry routes the call through the MCP client.
- The Telegram frontend shells out to `whisper-cli` (subprocess), reads the transcript, injects as text.
- **Do not** cgo-bind whisper.cpp into the static binary — preserve goreleaser cross-compilation.
- Per-step configurable: halt the chain (default for mutating steps) or continue-with-note (for advisory steps like lint after test).
- Mirrors GitHub Actions' `if: failure()` / `if: always()` semantics.
## Version Compatibility
| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| Go 1.25 floor | `anthropics/anthropic-sdk-go` v1.62.0 | SDK requires modern Go; 1.25 floor is safe. |
| Go 1.25 floor | `modelcontextprotocol/go-sdk` v1.0.0 | Official SDK tracks current Go; 1.25 is safe. |
| Go 1.25 floor | `go-telegram/bot` latest | Library follows modern Go conventions (context-first); 1.25 safe. |
| `anthropics/anthropic-sdk-go` v1.62.0 | Z.ai GLM endpoint (`https://api.z.ai/api/anthropic`) | Verified: Z.ai "GLM Coding Plan" explicitly supports the Anthropic protocol. Set base URL, use model slug `glm-4.6` / `glm-5`. |
| `sashabaranov/go-openai` latest | MiniMax M3, OpenRouter, Groq (STT) | All OpenAI-compatible; swap base URL + model slug. **Verify** the tool-calling schema matches each provider at Phase 0. |
| ACP v1 (stable) | v2 (Draft) | **v2 is draft** — do not target it. Pin v1 method names. Verify against canonical spec in Phase 0. |
| `modelcontextprotocol/go-sdk` v1.0.0 | MCP protocol 2026-07-28 | Official SDK tracks the spec; pin SDK version, be aware of protocol-version deprecations noted in pkg.go.dev. |
| whisper.cpp subprocess (if used) | ggml-format models | Follow the 2026 whisper.cpp setup guide for model download; do not cgo-bind. |
| goreleaser v2.17 | macOS+Linux amd64+arm64 | Verified; produces the binary the ACP registry manifest points at. |
| `go-telegram/bot` + ACP stdio in one process | Both share `log/slog` → stderr | **Critical:** Telegram frontend must never write to stdout (stdout is ACP's). Enforce via review/lint. |
## Open Verification Items (Phase-0 spike checklist)
## Sources
- https://agentclientprotocol.com/get-started/introduction — ACP overview
- https://agentclientprotocol.com/protocol/v1/overview, /transports, /prompt-turn, /tool-calls, /session-setup, /schema — v1 spec (stable)
- https://agentclientprotocol.com/announcements/acp-v2-draft — **v2 is Draft** (confirms v1 is the stable target)
- https://github.com/agentclientprotocol/agent-client-protocol — canonical repo
- https://agentclientprotocol.com/rfds/acp-agent-registry + https://github.com/agentclientprotocol/registry/blob/main/agent.schema.json — registry manifest format
- https://go.dev/doc/devel/release — release history (1.23 = Aug 2024; 1.26 = Feb 2026; floor bumped to 1.25)
- https://goreleaser.com/ — v2.17 current (2026)
- https://github.com/anthropics/anthropic-sdk-go/releases — v1.62.0 (Jul 2026), mid-conversation-tool-changes + session budgets
- https://github.com/sashabaranov/go-openai — ~10.7k stars, 2026-active (GPT-5/5.5 in repo desc)
- https://docs.z.ai/devpack/quick-start — **Z.ai Anthropic-compatible base URL `https://api.z.ai/api/anthropic` confirmed**
- https://pkg.go.dev/go.lsp.dev/jsonrpc2 — upstream stagnant (v0.10.0, scorecard 5.6/10)
- https://libraries.io/go/github.com%2Fkwo%2Fjsonrpc2 — 2026 fork (`v0.0.0-20260410…`) as fallback
- https://github.com/sourcegraph/jsonrpc2 — superseded; not for new builds
- https://www.adityabawankule.io/blog/claude-code-session-jsonl-format — JSONL path `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`
- https://claude-dev.tools/docs/jsonl-format — JSONL format reference
- https://databunny.medium.com/inside-claude-code-the-session-file-format-and-how-to-inspect-it-b9998e66d56b — full transcript on-disk
- https://github.com/chouzz/llm-interceptor — MITM proxy for AI coding assistants
- https://news.ycombinator.com/item?id=46799898 — Sherlock MITM proxy
- https://localai.io/features/mitm-proxy/index.print.html — allowlist-based LLM MITM
- https://www.c-sharpcorner.com/article/intercepting-and-decoding-claude-code-api-calls-using-mitm-proxy/ — Claude Code ~80 KB payload interception walkthrough
- https://github.com/daaain/claude-code-log — JSONL → HTML/Markdown converter (dev-time tool)
- https://docs.litellm.ai/docs/routing + /docs/proxy/reliability — LiteLLM router/fallback design (template only; Python proxy, not a Go dep)
- https://www.requesty.ai/blog/best-llm-routing-platforms-compared-2026-requesty-portkey-litellm-openrouter — 2026 comparison (all are hosted/Python)
- https://github.com/go-telegram/bot — recommended (zero-dep, context-first, active)
- https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2/ext — gotgbot alternative (May 2026 publish)
- https://community.latenode.com/t/which-library-offers-better-architecture-go-telegram-bot-or-go-telegram-bot-api/27111 — go-telegram/bot architecture praise
- https://developers.openai.com/api/docs/guides/speech-to-text — OpenAI Whisper API (default STT)
- https://github.com/ggml-org/whisper.cpp + https://pkg.go.dev/github.com/ggml-org/whisper.cpp/bindings/go — local STT (subprocess, not cgo)
- https://www.reddit.com/r/golang/comments/nsfjtq/for_those_running_go_in_production_at_scale_what/ — Temporal = overkill for embedded pipelines
- https://github.com/go-task/task — design reference (DAG-ordered Make-like)
- https://github.com/Flowpack/prunner — design reference (embeddable Go pipeline runner)
- https://github.com/modelcontextprotocol/go-sdk — **official Go SDK, v1.0.0, Google-collaborated** (recommended)
- https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp — protocol 2026-07-28, stdio + stderr guidance
- https://modelcontextprotocol.io/docs/2026-07-28/sdk — official SDK docs
- https://github.com/orgs/modelcontextprotocol/discussions/364 — Go SDK design discussion (vs mark3labs)
- https://github.com/mark3labs/mcp-go — community alternative (influenced official SDK)
- https://fast.io/resources/mcp-server-golang/ — 2026 comparison (official = spec compliance; mark3labs = quick setup)
- https://code.claude.com/docs/en/mcp-quickstart — `.mcp.json` config format + subprocess launch
- https://forum.cursor.com/t/cursor-3-4-20-kills-stdio-mcp-servers-1-5s-after-successful-initialize-sigkill-v2-fsm-race/160892 — stdio lifecycle gotcha
- https://code.claude.com/docs/en/hooks — hooks reference (config schema, JSON I/O, lifecycle events)
- https://www.adityabawankule.io/blog/claude-code-session-jsonl-format — `~/.claude/projects/` layout
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
