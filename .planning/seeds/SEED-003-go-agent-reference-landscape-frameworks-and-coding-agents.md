---
id: SEED-003
status: referenced-in-v1.2
planted: 2026-08-17
planted_during: v1.1 Kickoff & Peers / Phase 9 (legs complete, awaiting verification)
trigger_when: alongside SEED-002's triggers — SEED-001 kit extraction (API-surface design); engine interrupt/resume + checkpoint design; session/memory package evolution; provider-testing cassettes (parity); scheduler error classification; post-stage auto-debug routines (hook-DAG); headless/non-interactive serve-mode design; STACK.md currency passes
scope: small
audit_acknowledged:
  milestone: v1.1
  at: 2026-08-25
  status: referenced-in-v1.2
---

# SEED-003: Go agent-framework & coding-agent reference landscape — the projects worth studying alongside fantasy (SEED-002)

## Why This Matters

SEED-002 captured one reference (charmbracelet/fantasy) in depth. This seed is the surrounding landscape, surveyed and repo-verified 2026-08-17: fantasy is **not** alone — there are two whole categories of prior art, and several projects solve problems ass-guard will hit sooner than the kit extraction does (interrupt/resume checkpoints, session/memory shape, sentinel error classes, plan→execute→auto-debug loops, headless JSON mode). Standing verdict from SEED-002 applies ecosystem-wide: **none of these enter the runtime dependency graph** — they are reading material and pattern sources, with per-project license rules for any code lifting.

## The Landscape (repo-verified 2026-08-17)

### A. Go agent frameworks / kits (fantasy's peers)

| Project | Verified stats | Reference value for ass-guard |
|---|---|---|
| **cloudwego/eino** (ByteDance, Apache-2.0) | 12.7k★, active, battle-tested at ByteDance; components (ChatModel/Tool/Retriever/ChatTemplate) + ADK agent/Runner + `compose.Graph` orchestration + `eino-ext` provider repo | (1) **Interrupt/resume with checkpoints** — any agent/tool can pause for human input and resume from persisted state; directly comparable to our engine checkpoint patterns. (2) **Automatic stream handling through orchestration** — framework concatenates/merges/copies streams between nodes; input for our streaming plumbing. (3) Callbacks (OnStart/OnEnd/OnError) vs our `internal/event`. (4) **Core/ext two-repo split** — the strongest existing answer to SEED-001's "where does the kit API surface end" question. (5) `DeepAgent` multi-agent delegation. |
| **google/adk-go** (`google.golang.org/adk/v2`, Apache-2.0) | 8.7k★, active, v2 in progress; packages: agent, agentregistry, model, tool, runner, **session, memory**, artifact, auth, workflow, plugin, server, telemetry, platform | (1) **First-class `session` + `memory` packages** — design comparison for our `internal/session` and `internal/learning`. (2) `workflow` package vs our `internal/hookdag`. (3) **`internal/httprr`** — built-in HTTP record/replay for tests; second independent data point (after fantasy's `charm.land/x/vcr`) that cassette testing is the standard for provider clients — validates the parity-cassette direction. (4) OTel-native telemetry. (5) Google's own kit API shape = SEED-001 reference. |
| **zendev-sh/goai** (GoAI SDK, MIT) | 25+ providers; **zero third-party deps** (core needs only x/oauth2); claims 569µs cold start | (1) Zero-dep ethos matches our single-static-binary constraint — proof it's achievable with 25 providers. (2) Generics API: `GenerateObject[T]`, `NewTool[In]`, `StreamText` via channels, `WithMaxSteps` auto tool loop. (3) **Built-in MCP client (stdio/HTTP/SSE) converting MCP tools to native tools** — comparison for `internal/mcp`. NOTE: "inspired by" Vercel AI SDK, NOT wire-compatible (corrected during verification; the HN "Vercel-wire-compat Go SDK" is a different project). |
| **mozilla-ai/any-llm-go** (Apache-2.0) | 153★, Go 1.26+, sibling of Python any-llm; uses official OpenAI/Anthropic Go SDKs internally; providers incl. **z.ai**, llama.cpp, Llamafile, Ollama | (1) **Sentinel error normalization**: `ErrRateLimit`, `ErrAuthentication`, `ErrContextLength` — exactly the error-classification seam our scheduler fallback chains need to trigger on. (2) Minimal-unified-interface done thin (swap provider = swap import+constructor) — the "normalization with least loss" reference. |
| **tmc/langchaingo** (MIT) | the classic LangChain Go port; chains/agents/RAG, community-driven | Known quantity; abstraction-heavy LangChain shape. Low priority — consult only if we need a specific chain pattern. |
| **firebase/genkit** (Go support) | Google's app framework; flows + dev UI | Low priority; JS-first, Go is a sibling surface. |
| *(charmbracelet/fantasy)* | — | → SEED-002 (in-depth); Apache-2.0, vendorable. |

### B. Go coding agents (apps — engine & wire-behavior references)

| Project | Verified stats | Reference value for ass-guard |
|---|---|---|
| **charmbracelet/crush** (FSL-1.1-MIT) | 27.4k★, ~4k commits, very active; TUI, LSP, MCP (http/stdio/sse + OAuth), **skills system (Agent Skills standard)**, permissions/hooks, `crushrc`, mid-session model switching | The app whose engine is fantasy (SEED-002 §6). Skills/permissions/hooks are ecosystem-sibling features to study for zcode-compat parity. **⚠ LICENSE TRAP: FSL-1.1-MIT is source-available, NOT permissive — read/study freely, never copy code** (unlike fantasy/opencode/plandex). A future crush-profile mimicry target would be behavior-level, not code-level. |
| **opencode-ai/opencode** (MIT, **ARCHIVED Sep 2025**) | 13.7k★ frozen; **lineage verified: "continued under the name Crush by the original author and the Charm team"** — Crush is its continuation | A **frozen, complete, MIT Go coding agent** — the stable reading target Crush can never be (it moves too fast). Study: `internal/llm` provider layer (2nd Go wire-shaping reference after fantasy), SQLite session storage, LSP integration design, MCP stdio+SSE, **auto-compact at 95% context window** (input for our session compaction), custom commands with `$PLACEHOLDER` args, **non-interactive mode with text/JSON output** (prior art for our headless ACP serve path). Vendorable if ever needed. |
| **plandex-ai/plandex** (MIT) | 15.6k★, moderate activity (cloud wound down Oct 2025 → self-host Docker); Go | The **plan-first** coding agent: (1) **plan→execute→auto-debug loop that verifies via build/test/lint/and even Chrome for browser apps** — the closest prior art to our hook-DAG post-stage routines (review/tests/linters) and to SDD autocontinue; (2) **cumulative-diff sandbox: AI changes stay out of the project until approved, versioned as git branches per attempt** — safety-model reference (our model is no-confirmation + pattern-table, but the isolation technique is worth knowing); (3) 2M-token context management + tree-sitter project maps (30+ languages) — context-engineering reference; (4) context caching across providers. |

### C. Adjacent (one-liners, don't expand)

- **Non-Go coding agents** (codex/Rust, gemini-cli+qwen-code/TS, goose/Rust, OpenHands & aider/Python): not API references for us — but each is a **potential mimicry-target ecosystem** the way zcode is; keep on the radar for profile-horizon decisions, not engineering references.
- **Multi-provider gateways** (LiteLLM, one-api, agentgateway, llmgateway): already dispositioned in STACK.md research — hosted/Python, template-only.

## License Rules for Code Lifting (summary)

- **Vendorable** (with attribution/NOTICE): fantasy (Apache-2.0), eino (Apache-2.0), adk-go (Apache-2.0), any-llm-go (Apache-2.0), opencode (MIT), goai (MIT), plandex (MIT).
- **Read-only — never copy**: crush (FSL-1.1-MIT, converts to MIT per-release after 2 years; treat as behavior reference only).

## When to Surface

Same triggers as SEED-002, plus: engine checkpoint/interrupt-resume design (→ eino); `internal/session`/`internal/learning` evolution (→ adk-go session/memory); scheduler error-class enum design (→ any-llm-go sentinels); hook-DAG auto-debug step semantics (→ plandex); headless serve-mode hardening (→ opencode non-interactive mode); SEED-001 kit API-surface decisions (→ eino core/ext split, adk-go package layout, fantasy §7).

## Scope Estimate

**Small** — this seed is a map, not work. Each adoption unit is a bounded task (study pass or pattern port); no milestone-sized items. Stats are point-in-time (2026-08-17) — re-verify before citing externally.

## Breadcrumbs

- `.planning/seeds/SEED-002-mine-charmbracelet-fantasy-for-patterns-and-prior-art.md` — the in-depth fantasy analysis this landscape surrounds; SEED-002 §6 (Crush wire format) gains the opencode→crush lineage fact from here
- `.planning/seeds/SEED-001-agent-creation-kit-library-with-ass-guard-as-first-app.md` — kit extraction; eino/adk-go/fantasy are its three API-shape references
- `internal/engine/`, `internal/session/`, `internal/event/` — checkpoint/stream/callback comparison targets (eino, adk-go)
- `internal/scheduler/` — sentinel-error classes (any-llm-go), retry semantics (SEED-002 §4)
- `internal/hookdag/` — auto-debug step semantics (plandex)
- `internal/parity/`, `internal/drift/` — cassette validation (adk-go httprr, fantasy vcr)
- `internal/mcp/` — MCP-client-as-tool-converter comparison (goai)
- Upstream: github.com/{cloudwego/eino, google/adk-go, zendev-sh/goai, mozilla-ai/any-llm-go, tmc/langchaingo, charmbracelet/crush, opencode-ai/opencode, plandex-ai/plandex}
- **EXPANDED 2026-08-17:** the full 34-source landscape (this set + pi, Claudecourse, sst/anomalyco opencode, codex, cline, goose, OpenHands, gemini-cli, qwen-code, gh-aw, aider, SWE-agent, 5 vendor SDKs, 7 independent frameworks) with the implemented/planned/borrow/don't-need analysis lives in `.planning/research/IDEA-LANDSCAPE.md` — read that first; its ranked borrow list (checkpoints/undo, compaction verification, real sandboxing, steering queue) supersedes this seed as the adoption index

## Notes

Captured 2026-08-17 from the operator: "are even more projects like this outside? can you search for them, to use them as reference."

Method: 4 broad web searches → repo-level verification fetches for every load-bearing claim (language, stars, license, activity, lineage, and the GoAI "Vercel-compat" correction; any-llm-go path is mozilla-ai/, not mozilla/). langchaingo/genkit entries are qualitative (household names, no repo fetch — stats unverified). The opencode→crush lineage came from opencode's own archived repo banner, not third-party posts.

Survey sources: [cloudwego/eino](https://github.com/cloudwego/eino) + [Eino in practice (ByteDance)](https://www.cloudwego.io/docs/eino/overview/bytedance_eino_practice/), [google/adk-go](https://github.com/google/adk-go) + [ADK Go 1.0 announcement](https://developers.googleblog.com/adk-go-10-arrives/), [GoAI SDK](https://goai.sh) ([repo](https://github.com/zendev-sh/goai), [build story](https://blog.anh.sh/why-and-how-i-built-a-go-ai-sdk)), [any-llm-go (Mozilla AI)](https://github.com/mozilla-ai/any-llm-go) + [launch post](https://blog.mozilla.ai/run-openai-claude-mistral-llamafile-and-more-from-one-interface-now-in-go/), [charmbracelet/crush](https://github.com/charmbracelet/crush), [opencode-ai/opencode (archived)](https://github.com/opencode-ai/opencode), [plandex-ai/plandex](https://github.com/plandex-ai/plandex), plus landscape context from [Zep's agentic-Go survey](https://blog.getzep.com/agentic-development-in-go/), [awesome-cli-coding-agents](https://github.com/bradagi/awesome-cli-coding-agents), and [OpenHands' 2026 agent roundup](https://www.openhands.dev/blog/open-source-ai-coding-agents).
