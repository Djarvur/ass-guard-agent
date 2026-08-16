---
id: SEED-002
status: dormant
planted: 2026-08-17
planted_during: v1.1 Kickoff & Peers / Phase 9 (legs complete, awaiting verification + parity disposition)
trigger_when: (a) when hardening tool-call validation/repair in toolexec; (b) when designing the scheduler's retry/fallback-chain policy; (c) when building cassette/replay fixtures for parity+drift; (d) when SEED-001's kit extraction starts (fantasy is its direct prior art); (e) if a Crush mimicry profile is ever considered; (f) at the next STACK.md review (official openai-go SDK question)
scope: medium
---

# SEED-002: Mine charmbracelet/fantasy (Charm's Go agent kit, the engine powering Crush) — patterns, stealable packages, and prior art for the kit extraction

## Why This Matters

`charmbracelet/fantasy` (module `charm.land/fantasy`, Apache-2.0, ~950★, active WIP, text-only for now) is the most direct prior art that exists for two things ass-guard cares about: **a production Go agent's wire-level Anthropic shaping** and **SEED-001's agent-creation-kit idea** (fantasy IS an extracted agent kit with the app — Crush — built on top of it).

**Standing verdict (decided 2026-08-17, pre-seed): NOT a runtime dependency.** Its pitch — "multiple providers, multiple models, one API" — is a normalization layer, and normalization is the enemy of mimicry: any request field zcode sends that fantasy's `Call`/`Response` model doesn't represent is un-sendable, and its agent loop would displace our own engine IP (profile shaping, scheduler chains, hook-DAG triggers, ACP streaming). Same "What NOT to Use" bucket as LiteLLM/Temporal: fine library, wrong abstraction for our load-bearing constraint.

The value is in **selective adoption**: two self-contained packages worth vendoring/porting, ~six design patterns worth copying, one testing-infrastructure idea worth replicating, and ecosystem intel (including that fantasy's anthropic provider is, effectively, Crush's request shaper — ground truth for any future Crush profile).

## Extracted Intel (what to adopt, where)

### 1. Stealable as code (Apache-2.0, vendor with NOTICE)

- **`jsonrepair` package** (self-contained, `jsonrepair/jsonrepair.go`): repairs malformed LLM JSON. Handles: truncated output (missing brackets), unquoted strings/keys, doubled/misplaced quotes, smart quotes, ```json fences, `#`/`//`/`/* */` comments, bad escapes, duplicate keys, bare matrix rows, truncated numbers, multiple top-level values. API: `RepairJSON`, `Loads`, `RepairJSONWithLog`; options `WithStrict` (error instead of heuristics), `WithStreamStable` (conservative repair for partial/streamed input — the mode that matters for streaming tool-input deltas), `WithSkipJSONLoads`, `WithEnsureASCII`. **Adopt when:** `internal/toolexec` adds tool-call input validation/repair for ass-guard-native tools (zcode-profile tools must keep zcode's exact behavior — this is for OUR tools, or dev-time).
- **`schema` package** (reflection-based JSON-schema-from-Go-types, ~one file): `Generate(reflect.Type)`, `ToParameters(Schema)`. json tags (name override, `-` skip, `omitempty` → optional), `description` and `enum` struct tags, snake_case fallback, recursion guard for self-referential types. **Adopt when:** any ass-guard-native tool needs a schema without hand-writing JSON (zcode-profile tools take their schemas from the harvested profile, never generated).

### 2. Anthropic wire-shaping intel — they wrap the SAME SDK we do (`anthropics/anthropic-sdk-go`; we're both on v1.63.0)

- **Raw-JSON tool injection for unmodeled/beta tools:** `option.WithJSONSet("tools", rawTools)` — bypasses SDK type gaps (they use it for computer use). Relevant to `internal/shaper`/`internal/provider` when the zcode catalog carries beta-shaped tools the SDK doesn't model.
- **Beta routing:** query param `beta=true` + `anthropic-beta` header per flag.
- **tool_use ↔ tool_result pairing survival:** `decodeToolCallInputMap` falls back to `{}` on malformed tool input rather than dropping the call — the Anthropic API hard-errors on orphaned tool_use/tool_result. A protocol invariant our loop must respect.
- **Incomplete-stream detection:** a stream is complete only with BOTH a stop reason AND the `message_stop` SSE event; otherwise `NewIncompleteStreamError()`. Cheap correctness check for `internal/provider` streaming.
- **System shape:** system prompt = `[]anthropic.TextBlockParam` (multi-block); prompt history grouped by role; tool messages merged into user blocks; later system blocks skipped after the first.
- **Finish-reason map:** `end_turn`/`pause_turn`/`stop_sequence` → Stop; `tool_use` → ToolCalls. Default `MaxTokens` 4096 (profile-driven for us, but a useful baseline data point).
- **User-agent mechanics:** per-call UA override via `option.WithHeader("User-Agent", ua)`; resolution precedence explicit-UA > headers-map > default; case-insensitive UA dedup. Shows exactly where UA interception lives in the SDK — the seam our profile identity fields use.

### 3. Agent-loop design patterns (`agent.go`, 56 KB)

- **Composable `StopCondition`s** instead of a max-steps knob: `StepCountIs(n)`, `HasToolCall(name)`, `HasContent(type)`, `FinishReasonIs(r)`, `MaxTokensUsed(budget)`. Maps cleanly onto our engine/hook-DAG continuation policy.
- **`PrepareStep` middleware seam:** per-step override of model, messages, system prompt, tool choice, active tools. Conceptually our shaper; theirs proves the pattern at scale.
- **Parallel tool execution:** tools opt in via a `Parallel` flag; parallel ones run behind a **semaphore of 5**; non-parallel serialize under a mutex; results collected under a shared mutex. Direct input for `internal/toolexec` concurrency.
- **`runToolSafely`:** recovers tool panics → error tool-response including stack trace. Cheap robustness we should mirror.
- **`StopTurn` flag on tool responses:** results still delivered to the transcript, but no further completion requested. Interesting semantic for hook-DAG "halt chain but record" behavior.
- **Pluggable `RepairToolCallFunction`** in `validateAndRepairToolCall` (default = jsonrepair). The validation-pipeline shape toolexec wants.
- **No session object** — multi-turn by passing accumulated `Messages`. (Ours is richer; noted as contrast, not adoption.)

### 4. Retry design (`retry.go`) — a Go-native fallback-chain template

Honors `retry-after-ms` first (OpenAI-style, more precise), then `retry-after` (seconds or RFC1123 date); header override accepted only if `0 ≤ ms < 60s` or < exponential backoff; floor 5s, factor 2.0, default 3 retries. Never retries context cancellation. Distinguishes: provider-retryable errors, `net.Error` connection failures, HTTP/2 transport errors. `OnRetry` callback documented as "reset accumulated stream state" (retries replay stream callbacks from scratch — a real trap). **`OnAuthRefresh`: one-shot credential refresh on 401; on success the entire retry pass re-runs with a fresh budget.** Exhausted retries → `RetryError` aggregating all attempts. → Feed into `internal/scheduler`'s fallback-chain design alongside the LiteLLM template; fantasy's is Go-native and header-aware.

### 5. Testing infrastructure pattern (`providertests/` + `charm.land/x/vcr`)

One shared behavioral suite (`common_test.go`, `object_test.go`) executed against EVERY provider (anthropic, openai + responses, azure ×2, bedrock, google, openrouter, vercel, openaicompat) over **VCR cassette record/replay** (`charm.land/x/vcr`), with `.env.sample` for live-key runs. The cross-provider shared-suite pattern maps 1:1 onto "same behavioral suite across N profiles" for `internal/parity`/`internal/drift` — and cassettes are exactly the golden-fixture shape the parity gate needs for its curated suite.

### 6. Ecosystem intel

- **Official `github.com/openai/openai-go` v3 exists at v3.50.0** and fantasy uses it — STACK.md currently recommends `sashabaranov/go-openai` (MEDIUM confidence, "verify at Phase 0"). The official SDK deserves a line in that verification item.
- `anthropic-sdk-go` v1.63.0 — already our pin (coincidence confirmed; STACK's v1.62.0 note is stale).
- Providers that exist if we ever need them: `vercel` (AI Gateway), `kronk` (ardanlabs search), native `openrouter`, `openaicompat` generic.
- **Crush's wire format IS fantasy's anthropic provider.** If a Crush (or any fantasy-based runtime) mimicry profile is ever considered, `providers/anthropic/anthropic.go` is the ground-truth request shaper to read — no MITM needed for the structural parts.
- Default UA construction: `Charm-Fantasy/<version>` — trivia now, identity-field intel later.

### 7. SEED-001 connection (prior art)

Fantasy validates the kit-extraction market: same shape (library + first app on top), same Go-native positioning. **ass-guard-kit's differentiator is exactly what fantasy doesn't do:** wire-level profile fidelity per mimicked agent (fantasy normalizes; we preserve). When SEED-001's kit work starts, fantasy's public API surface (`LanguageModel` interface with `Generate`/`Stream`/`*Object` variants, `iter.Seq[StreamPart]` streaming, `AgentTool` interface, provider registry, `CallWarning` capability-degradation model) is the reference landscape to design against — and to difference ourselves from in the kit's docs.

## When to Surface

**Triggers (any of):**

1. `internal/toolexec` tool-call validation/repair work → §1 jsonrepair, §3 validation pipeline
2. `internal/scheduler` fallback-chain/retry policy design → §4
3. `internal/parity`/`internal/drift` cassette/golden-fixture work → §5 (also relevant to the open parity-recalibration disposition: re-recording curated-suite expectations)
4. SEED-001 kit extraction kicks off → §7 + full-repo API reference pass
5. A Crush / fantasy-based-runtime mimicry profile is proposed → §2, §6
6. Next STACK.md review / Phase-0 provider-client verification → §6 (official openai-go vs sashabaranov)

Natural moments: `/gsd:new-milestone` scans (v1.2 pool contains Phases 10/11 — adjacent to kit concerns), Phase-9 parity disposition (trigger 3), STACK.md currency passes.

## Scope Estimate

**Medium** — nothing here is a milestone. The adoptable units are small and independent: vendor/port jsonrepair (small), schema generator (small), copy retry semantics into scheduler policy (small–medium), replicate VCR cross-suite in parity (medium, and partly blocked on the parity-expectations disposition), fantasy API reference pass at kit time (reading, not code). The seed's value is having the map ready so each adoption is a task, not a research project.

## Breadcrumbs

- `internal/toolexec/`, `internal/toolcat/` — tool execution + catalog (jsonrepair + schema adoption targets; `Parallel`/semaphore pattern)
- `internal/scheduler/` (resolver.go, dispatch.go) — fallback-chain/retry policy target (§4)
- `internal/parity/` (harness.go, replay.go, suite/) + `internal/drift/` — cassette/shared-suite pattern target (§5); open curated-expectations disposition
- `internal/provider/`, `internal/shaper/` — Anthropic wire-shaping intel target (§2: WithJSONSet raw tools, pairing fallback, incomplete-stream check)
- `internal/loop/`, `internal/engine/`, `internal/session/` — stop-condition/PrepareStep/StopTurn pattern comparison (§3)
- `go.mod` — `anthropics/anthropic-sdk-go v1.63.0` (matches fantasy; STACK.md's v1.62.0 already stale)
- `.planning/seeds/SEED-001-agent-creation-kit-library-with-ass-guard-as-first-app.md` — kit extraction; fantasy is its prior art (§7)
- `.planning/seeds/SEED-003-go-agent-reference-landscape-frameworks-and-coding-agents.md` — the surrounding landscape (eino, adk-go, goai, any-llm-go, opencode→crush lineage, plandex); enriches §6 with the verified opencode→crush continuation and crush's FSL-1.1-MIT license trap
- `.planning/research/STACK.md` — provider-client decisions this seed amends (openai-go official SDK, anthropic-sdk version)
- Upstream: `github.com/charmbracelet/fantasy` @ main (analysed 2026-08-17): `agent.go`, `model.go`, `tool.go`, `retry.go`, `jsonrepair/`, `schema/`, `providers/anthropic/{anthropic,sanitize,call_useragent}.go`, `providers/internal/httpheaders/`, `providertests/` + `testdata/`, `go.mod`

## Notes

Captured 2026-08-17 from the operator: "analyse https://github.com/charmbracelet/fantasy carefully and extract all the useful things for the future adoption."

Analysis method: README + repo tree + source-level reads of agent.go, model.go, tool.go, retry.go, schema/schema.go, jsonrepair/jsonrepair.go, providers/anthropic/{anthropic,sanitize,call_useragent}.go, providers/internal/httpheaders/httpheaders.go, go.mod, providertests/ listing (2026-08-17 snapshot; fantasy is WIP — re-verify specifics at adoption time).

Constraint to keep front-of-mind (from the pre-seed verdict): fantasy must never enter the runtime dependency graph while mimicry-by-structural-indistinguishability is the core value — adoption is vendoring isolated packages or copying patterns, not importing the kit. One nuance for the kit era: if SEED-001's kit ever wants a "normalized quick-start mode" (build an agent without a mimicry profile), fantasy becomes the API to imitate for that mode — an optional bridge, not a dependency.
