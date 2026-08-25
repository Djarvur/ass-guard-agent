# Phase 11: dsh Mimicry Profile #2 - Research

**Researched:** 2026-08-14
**Domain:** Mimicry profile capture for a second target agent (deepseek-harness, "dsh") — recording-proxy wire capture, zstd session-log harvest, DeepSeek OpenAI-dialect wire mapping, N-profile generalization
**Confidence:** HIGH overall — the four flagged unknowns are now grounded: DeepSeek dialect verified against official API docs, klauspost/compress retractions verified against upstream releases, dsh custom-provider baseURL config verified against dsh's official docs, and the two capture-only questions (wire truth, system-message form) are resolved into **decision procedures the capture itself answers**, per the roadmap's "answerable only from the capture" framing.

## Summary

Phase 11 adds mimicry profile #2 (dsh) as **data plus generalization, not a new engine**. Three facts drive every plan. First: dsh's session JSONL stores *session events*, not outgoing HTTP requests — so wire truth MUST come from a **recording proxy at the configured baseURL** (dsh supports arbitrary custom-provider baseURLs via `$DSH_HOME/settings.yaml`, including plain-HTTP localhost endpoints — no TLS MITM needed because we own the URL dsh posts to). Second: the OpenAI-shape provider in this repo (`internal/provider/openai.go`) is currently a MiniMax-validated shell: `buildRequest` never reads `profile.System`, `Stream` returns "not implemented", profile headers are never sent, and `tools` is never omitted-when-empty — dsh's wire (always `stream:true` + `include_usage`, identity headers, omit-when-empty `tools`, `strict:true` schemas) maps onto it only after those gaps close. Third: "N profiles, no target-specific code paths" is currently false in detail — 69 non-test `zcode` references sit under `internal/` + `cmd/`, of which the load-bearing ones are the redaction doc-contract, the parity harness's zcode-shaped suite extraction, `profile check`'s hardcoded zcode rollout loader, and the extractor's zcode-only CLI — the DSH-01 audit genericizes or parameterizes each.

Web-verified this session (August 2026): DeepSeek's official docs REQUIRE `strict: true` on every tool function with server-side JSON-Schema validation; thinking-mode models silently IGNORE `temperature`/`top_p`/`presence_penalty`/`frequency_penalty`; JSON-mode output can truncate mid-string when `max_tokens` is too low (`finish_reason:"length"`). klauspost/compress v1.18.1 AND v1.19.0/v1.19.1 are RETRACTED upstream (bad flate encoding; arm64 crash under profiling/GC) — v1.19.2 is the fix and the only acceptable pin. dsh's custom-provider schema is `llm-pi-ai.providers.<id>` with `api: openai-completions`, `baseURL`, `apiKeyEnv`, `models` — credentials live in `$DSH_HOME/.credentials.yaml`.

**Primary recommendation:** Build the capture tooling first (recording proxy + zstd spike) against a real dsh install pinned at one commit, decide the system-message mapping form from the capture via the decision procedure below, then close the OpenAI-provider wire gaps, then extract and prove parity — mirroring Phase 9's provenance discipline (plans 09-03/09-04) generalized to a second target.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** The live DeepSeek probe targets the **opencode subscription's gateway** — the operator's actual access path (MiniMax + DeepSeek via the opencode subscription, recorded at v1.0 close). Prerequisite verified during the phase: the gateway must expose an OpenAI-compatible endpoint (base URL + credential wired through the Phase-7 `ProviderConfig` machinery; scheduling.yaml's DeepSeek provider entry points there). DeepSeek's official API remains a config-swappable alternative — nothing hardcodes the gateway. *[User-selected.]*
- **D-02:** **Pin + drift guard**: one capture against a pinned dsh commit (recorded in profile meta); `profile check dsh` guards against silent divergence; re-capture is a documented manual procedure run when dsh moves — NOT a scheduled ritual. *[User-selected.]*
- **D-03:** **Full behavioral bar** — same standard as zcode: tool-call sequences through ass-guard-with-dsh-profile statistically indistinguishable from live dsh (the A/B parity harness, `internal/parity`). The cross-harness thesis gets proven, not assumed. *[User-selected.]*
- **D-04:** **The profile is specified per-model in config** — model entries in `scheduling.yaml` declare their profile (DeepSeek model entries specify `dsh`; GLM entries keep `zcode`). The explicit `--profile` flag remains as an override; per-turn switching stays a v1.2 non-goal. Requires a small schema addition (per-model `profile` field) riding the Phase-7 provider/model declaration machinery. *[User-selected — freeform: "profile is cpecified per-model in config".]*

### Claude's Discretion
- Recording-proxy implementation shape (local reverse proxy at a configured baseURL; dev-time tooling, not shipped runtime)
- zstd harvest tooling details (after the decode spike)
- The dsh capture workload's exact prompt sequence (mirrors the Phase-9 runbook discipline: divergence-prone)
- Redaction preserved-header genericization mechanics (profile-supplied lists)
- `strict: true` and DeepSeek dialect handling keyed by capability profile, never `if provider == "deepseek"`

### Deferred Ideas (OUT OF SCOPE)
- Per-turn profile switching — v1.2 non-goal (locked)
- dsh Responses-API wire shape — only if verification demands it
- Scheduled re-capture automation — manual procedure per D-02; revisit if dsh stabilizes
- UI/plugin-runtime mimicry of dsh (only the model-wire surface is mimicked) — permanent non-goal
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DSH-01 | Shared mimicry code contains no zcode-specific paths — a zcode-ism audit genericizes or parameterizes them (e.g. redaction's preserved-header list becomes profile-supplied) so "N profiles, no target-specific code paths" holds for two profiles | §6 Zcode-ism Audit — the grep census (69 non-test refs, load-bearing list), the genericization pattern (profile-supplied identity-header list), and the accept/reject rule from Pitfall 16 |
| DSH-02 | The OpenAI-shape provider maps `profile.System` onto the wire exactly as captured dsh traffic does (form decided from the capture, not assumption), and the profile loader tolerates profiles without `thinking.json` / `tool_choice.json` | §3 System-Message Mapping Form (decision procedure from capture) + §5 OpenAI Provider Gap Census (buildRequest/Stream/headers/tools-omission sites, loader lines 70-77) |
| DSH-03 | The dsh profile's content is captured, not hand-written — recording-proxy wire-truth capture at the configured baseURL plus zstd harvest of dsh session event logs (decode spike against a real `session.jsonl.zstd` first), with the pinned dsh commit recorded in profile meta | §1 Recording-Proxy Capture Runbook + §2 zstd Decode Approach & Spike Design + §7 Capture-Provenance Contract (traceability: every profile entry points at a captured line) |
| DSH-04 | Profile selection is declared per-model in config — model entries in `scheduling.yaml` carry a `profile` field (DeepSeek models → `dsh`, GLM → `zcode`; the explicit `--profile` flag remains an override) — and `scheduling.yaml` carries a credentialed DeepSeek provider entry (the opencode gateway's OpenAI-compatible endpoint per D-01, base-URL-swappable), `profile check dsh` detects drift against the per-profile capture, and a sanitized dsh seed ships in the embedded defaults | §5 (ModelConfig.Profile schema site + factory flow), §1 (gateway baseURL verification step), §8 Drift Extension (per-profile capture loader), §9 Seeding (defaults embed reaches seed/profiles/*) |
| DSH-05 | The A/B parity harness is green for dsh against a DeepSeek endpoint, one live DeepSeek tool-calling round-trip succeeds per routed model, and every profile entry is traceable to a captured request | §10 Parity + Live-Probe Design (suite from the proxy capture, LiveArm is provider-agnostic, dialect guards from §4) |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Wire-truth capture (recording proxy) | Dev-time CLI tool (repo, not shipped runtime) | — | The proxy is positioned at the *configured* baseURL — no interception of third-party traffic; it is capture tooling like `cmd/extract-profile`, never linked into `ass-guard` runtime |
| zstd session-log harvest | `internal/profile` (extraction) + `cmd/extract-profile` (CLI) | — | Same home as the zcode extractor; profile bundles are produced by extractors, never hand-written |
| dsh wire mapping (system msg, stream, headers, tools omission, strict) | `internal/provider` (OpenAI adapter) driven by `internal/profile` data | `internal/shaper` (Anthropic-only today; NOT extended for dsh) | dsh is chat-completions wire — the OpenAI adapter owns it; the shaper stays Anthropic-shape |
| Profile selection per model | `internal/scheduler` (ModelConfig.Profile) → serve-path profile load | `cmd/ass-guard` (`--profile` override) | Config-driven per D-04; the scheduler already owns per-(provider,model) declaration |
| Drift detection per profile | `cmd/ass-guard profile check` + `internal/drift` + per-profile capture loader | `internal/profile` coverage manifest | `drift.Detect` is already generic; only the capture-line source is zcode-hardcoded |
| Dialect handling (strict, param quirks) | `internal/scheduler` CapabilityProfile + `internal/provider` keyed by capability | — | Never `if provider == "deepseek"` (CONTEXT discretion + Pitfall 15) |
| A/B parity | `internal/parity` (harness is arm-agnostic) + dsh suite extractor | `cmd/ass-guard parity --profile dsh` | LiveArm takes any `provider.Provider`; the suite comes from the capture |
| Redaction identity semantics | `internal/redact` (generic walker) + profile-supplied identity-header list | — | The walker is already generic; the zcode-ism is the baked-in "12 headers" contract |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/klauspost/compress/zstd` | **v1.19.2** (pinned; NOT latest-blind) | Decode dsh `session.jsonl.zstd` (concatenated per-batch frames) for session-event harvest | Pure Go (CGO_ENABLED=0 survives); multistream concatenated-frame decode and XXH64 checksum verification are the DEFAULT decode path; de-facto standard. **[VERIFIED: GitHub releases + Go module proxy + pkg.go.dev, 2026-08-14]** |
| `github.com/sashabaranov/go-openai` | v1.42.0 (already pinned) | dsh wire = chat-completions dialect on the existing OpenAI adapter | `FunctionDefinition.Strict bool` (`json:"strict,omitempty"`) and `*StreamOptions` ALREADY exist in the pinned version — verified via `go doc` this session. No SDK change needed for `strict:true` or `include_usage`. **[VERIFIED: go doc against go.mod pin]** |
| `net/http/httputil` (stdlib) | Go 1.25+ | The recording proxy (dev-time tool) | `httputil.ReverseProxy` is the entire proxy; no TLS MITM needed because dsh's custom-provider baseURL is ours to point at plain-HTTP localhost. **[VERIFIED: dsh docs — custom providers accept arbitrary baseURL]** |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Node 22.19+/24 + pnpm 11.7.0 (Corepack) | dsh's pinned toolchain | Clone/install dsh at a pinned commit on the CAPTURE machine only | Capture setup + re-capture procedure; never a runtime or CI dep **[VERIFIED: dsh docs/development.md, research STACK.md]** |
| `gopkg.in/yaml.v3` | existing pin | dsh settings.yaml editing (runbook) + scheduling.yaml schema | Already a project dep |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| klauspost/compress/zstd (in-process) | Shell out to system `zstd` binary | Only if the spike hits a decoder edge — unlikely (concatenated frames + XXH64 are the default decode path). A subprocess adds a runtime external; avoid **[VERIFIED: pkg.go.dev decode semantics]** |
| In-repo Go recording proxy (httputil.ReverseProxy) | mitmproxy / LLM Interceptor / Sherlock | The external proxies shine when you CANNOT change the client's baseURL. dsh's baseURL is configurable, so a plain positioned reverse proxy is smaller, committed, scriptable, and records exactly the dsh wire without TLS cert games. Discretion resolved: in-repo Go tool |
| Recording proxy as wire truth | dsh session events alone | Events are the *session's* view (packed chunk rows, reconstructed seq/time); the proxy sees the *wire* (exact serialized request incl. headers + the runtime-composed system prompt). Profile content (system blocks, tools, headers, message shape) comes from the WIRE; events corroborate behavioral parity (tool-call sequences). Use both, wire wins conflicts |
| New dsh provider adapter | Parameterize the existing OpenAI adapter | A second adapter would fork wire logic — the exact "divergent copies" anti-pattern Phase 9 banned for audit. dsh's dialect differences are DATA (profile + capability fields), not code paths |

**Installation:**
```bash
go get github.com/klauspost/compress@v1.19.2   # the ONLY new module dependency this phase
```

## Package Legitimacy Audit

> Only one package install this phase. slopcheck (pip) targets npm/PyPI ecosystems; Go module verification done against the upstream release history + proxy instead.

| Package | Registry | Age | Downloads | Source Repo | Legitimacy check | Disposition |
|---------|----------|-----|-----------|-------------|------------------|-------------|
| github.com/klauspost/compress | Go module proxy | ~10 yrs | ecosystem-wide (used by Docker distribution, k8s deps) | github.com/klauspost/compress | Official GitHub releases confirm v1.19.2 is the fix release AFTER two public retractions (v1.18.1: invalid flate/zip/gzip encoding; v1.19.0/v1.19.1: arm64 crash during profiling/GC while decompressing — OpenTelemetry Collector issue #15660) **[VERIFIED: github.com/klauspost/compress/releases + otel-collector#15660]** | Approved — pin EXACTLY v1.19.2; do not resolve to v1.18.x/early v1.19.x |

**Packages removed due to legitimacy failure:** none.
**Packages flagged:** none.

## §1 Recording-Proxy Capture Runbook (Flagged Unknown #1 — RESOLVED as a procedure)

**The question:** how to stand up a local reverse proxy at the configured baseURL against a real dsh install.

**Verified configuration surface [CITED: deepseek-harness.github.io/deepseek-harness/en/guide/providers + github.com/deepseek-ai/deepseek-harness/blob/master/docs/config-catalog.md]:**

dsh adds custom model providers under `$DSH_HOME/settings.yaml` (Settings → Models in the UI writes the same YAML):

```yaml
llm-pi-ai:
  providers:
    dsh-capture:                 # provider id — lowercase, permanent
      apiKeyEnv: GATEWAY_API_KEY # or a literal credential in $DSH_HOME/.credentials.yaml
      api: openai-completions    # the OpenAI-compatible protocol key
      baseURL: http://127.0.0.1:8317/v1
      models:
        - id: deepseek-chat      # enter models MANUALLY (see GET /models note)
        - id: deepseek-reasoner
```

Load-bearing facts, all web-verified:
- The docs' stated use for custom providers is "a company gateway, self-hosted server, or provider absent from the installed catalog" — **arbitrary baseURLs, including plain-HTTP localhost, are first-class. No TLS MITM is needed**: the proxy is positioned, not intercepting. `[VERIFIED]`
- Credentials are write-only, stored in `$DSH_HOME/.credentials.yaml`; settings keep a reference or `apiKeyEnv`. The proxy forwards Authorization headers verbatim and redacts only at RECORD time. `[VERIFIED]`
- The UI's "Fetch available models" hits an OpenAI-compatible `GET /models` — the proxy MUST proxy that too (or the runbook enters models manually). `[CITED]`
- **Adapter nuance (capture-decides, again):** custom providers route through `llm-pi-ai` (the multi-provider package) with `api: openai-completions` — NOT necessarily the native `llm-deepseek` adapter whose `serialize.ts` the v1.1 research quoted. These two adapters may serialize differently. **The runbook therefore records WHICH route the captured turns flowed through** (the configured provider entry + model), and the wire shape is read from the capture, never assumed from either adapter's source. This is Pitfall 15's "wire protocol from capture, not docs" applied one level deeper. `[VERIFIED: dsh providers doc names llm-pi-ai as the custom-provider host]`
- dsh's llm adapters apply base URL / catalog / request defaults **per operation without restart** (dynamic config) — baseURL swaps are cheap mid-capture-session. `[CITED: packages/llm/llm-deepseek/README.md]`

**The runbook (operator procedure, mirrors Phase-9's 09-03/09-04 discipline):**

1. **Pin dsh.** Clone `github.com/deepseek-ai/deepseek-harness`, `git checkout <commit>`, record the full hash (feeds profile meta per D-02). `corepack enable && pnpm install && pnpm run typecheck` (Node 22.19+/24, pnpm 11.7.0).
2. **Start the recording proxy** (the in-repo Go tool this phase builds): listens on 127.0.0.1:8317, forwards to the REAL upstream (D-01: the opencode subscription gateway; DeepSeek official API as the base-URL-swappable alternative), and appends one JSON line per request AND response to a capture JSONL: `{ts, dir: "request"|"response", method, path, status, headers, body}`. Redaction applies at record time (Authorization value → `[REDACTED]`; the identity headers stay — they ARE the fingerprint).
3. **Configure dsh** with the settings.yaml block above (baseURL → proxy). Run one trivial turn to confirm end-to-end.
4. **Run the divergence-prone workload ONCE** (one session; thresholds mirror Phase-9's rationale — the capture must be able to FAIL checks): at least 5 user turns, at least 8 distinct dsh tools (dsh's catalog: shell, terminal, fs, web, todo, subagent, jobs, workflow, lsp, skill, code-runtime — hit read tools, mutation tools, a subagent dispatch, a web tool), at least 1 multi-tool-callback turn, and at least 1 tool-result-with-empty-output turn (exercises the `'(no output)'` substitution). Below-threshold captures are re-run, never relaxed.
5. **Also capture the zstd session log** for the SAME session (dsh writes `<root>/<normalized-cwd>/<encoded-session-id>/session.jsonl.zstd`; root is deployment-configured — record where it landed). The session log corroborates tool-call sequences for parity; the proxy capture is wire truth.
6. **Freeze + record provenance:** pinned dsh commit, capture date, provider route (which adapter/model), proxy tool commit, the capture file hash. All of it goes into `profiles/dsh/meta.yaml` — every entry in the profile must be traceable to a captured line (§7).

**Proxy implementation shape (discretion, resolved):** a `cmd/dsh-proxy`-style dev tool (~200 lines): `httputil.ReverseProxy` with a `ModifyResponse` + request-roundtrip hook writing the JSONL (0600), stdlib only + `internal/redact` for scrubbing. NOT shipped in the ass-guard binary's serve path — it is a separate main package, exactly like `cmd/extract-profile`. Dev-time tooling, never a runtime dependency.

## §2 zstd Decode Approach & Spike Design (Flagged Unknown #2 — RESOLVED)

**Verified library facts [VERIFIED: pkg.go.dev/github.com/klauspost/compress/zstd + github.com/klauspost/compress/releases, 2026-08-14]:**
- **Version discipline:** v1.18.1 RETRACTED (invalid flate/zip/gzip encoding); v1.19.0 + v1.19.1 RETRACTED (arm64: process crash possible while profiling/GC-scanning during zstd decompression — flagged via OpenTelemetry Collector #15660); **v1.19.2 is the fix**. Pin exactly; `go get github.com/klauspost/compress@v1.19.2`. The Go toolchain itself will warn on retracted versions — treat that warning as a build breaker.
- **Concatenated frames:** the Decoder is multistream by default — "encoded blocks can be concatenated and the result will be the combined input stream" — matching dsh's layout (one independent checksummed frame per append batch). `DecodeAll(input, dst)` decodes the whole file non-streaming; a `NewReader(r)` stream also drains frame-after-frame. Either works; `DecodeAll` is simpler for a bounded file.
- **Checksums:** frame checksums (XXH64) are verified BY DEFAULT; `ErrCRCMismatch` is the sentinel. `IgnoreChecksum` exists but must NOT be used — checksum failure on a real dsh artifact is a stop-and-reinvestigate signal, not noise.
- **Hostile-input guards:** `WithDecoderMaxMemory` (default 64GiB) bounds decoded size; descriptive sentinels exist (`ErrMagicMismatch`, `ErrFrameSizeExceeded`, `ErrDecoderSizeExceeded`); the decoder is fuzz-hardened to never crash. A session file that trips these is corrupt/captured-mid-write — error out loudly.

**dsh artifact format [CITED: research STACK.md, verified against packages/session/session-persistence-jsonl README at v1.1 research time]:**
- Path: `<root>/<normalized-cwd>/<encoded-session-id>/session.jsonl.zstd` (or `session.jsonl` when compression:'none'). Root is deployment-configured — there is NO default home like `~/.dsh`; the harvest tool takes an explicit root flag.
- Content: line 1 = immutable session header object; every subsequent line = a `SessionEvent` JSON object. Runs of >=3 assistant streaming chunks are packed into `text-chunks`/`reasoning-chunks`/`tool-call-chunks` rows whose per-event seq/time reconstruct as `seq0/time0 + dt`. The spike must expect BOTH shapes (plain events and packed rows).

**Spike design (DSH-03's explicit gate: decode a REAL `session.jsonl.zstd` before building the harvest on it):**
- Input: one real `session.jsonl.zstd` from the capture run (§1 step 5). CI without the artifact: a synthetic round-trip fixture — the test ENCODES a small known event stream as multiple concatenated `EncodeAll` outputs (simulating per-batch frames) and decodes it back, asserting byte equality + frame-boundary tolerance; the real-artifact test skip-cleans with a message naming the runbook step (the exact 09-03 pattern).
- Assertions on the real artifact: (a) `DecodeAll` succeeds with checksums on; (b) line 1 parses as a JSON object (session header); (c) EVERY line parses as JSON; (d) the event-type census is non-empty and includes at least one tool-call-bearing event; (e) packed `*-chunks` rows decode with `seq0/time0 + dt` reconstruction; (f) re-encoding the decoded stream and re-decoding round-trips.
- Failure disposition: checksum/magic errors on the real artifact → the file was captured mid-append or the root config is wrong — re-capture, never decode with checks disabled.

## §3 System-Message Mapping Form (Flagged Unknown #3 — RESOLVED as a decision procedure)

**The question:** does dsh put its system prompt on the wire as ONE joined system message or MULTIPLE system messages (and in what content form)?

**Why it is capture-only:** dsh's system prompt is a **runtime-composed registry** (ordered sections at priorities like `-100` harness identity, `0` deployment persona, plugin sections; `{{variable}}` interpolation; an expert-waterfall event) — static source reading CANNOT produce the final assembly `[CITED: research STACK.md, packages/core/system-prompt]`. And since the wire may be serialized by `llm-pi-ai` rather than `llm-deepseek` (§1 adapter nuance), even reading one adapter's serializer only bounds the answer. The capture is the only ground truth.

**What the capture MUST record (this is the deliverable of this section — the recording contract):**
For EVERY captured request line, record verbatim:
1. The full `messages` array as sent — role sequence, the COUNT of system-role messages, each message's position index, and the content form (plain string vs array-of-parts).
2. Byte-faithful system text(s) (pre-redaction; redaction never touches system blocks — they are the profile).
3. The full `tools` array (or its absence) — including whether each function object carries `strict` and its value.
4. Top-level request fields census: `stream`, `stream_options`, `tool_choice` (present/absent — dsh's native adapter NEVER serializes it), `thinking`, `reasoning_effort`, `temperature`, `max_tokens`, `stop`, and any unknown keys (unknown keys are profile content too — they go into the profile's message-shape data, not into code).
5. All request headers (identity/telemetry headers like `x-deepseek-harness-user-id` on the native route; whatever the actual route sends).
6. Tool-RESULT message shape: `role:"tool"` entries, `tool_call_id`, and the literal `'(no output)'` substitution for empty outputs.

**The decision procedure (executed during extraction, not planning):**
- Count system-role messages per request across ALL requests in the pinned capture session.
- **Exactly one** in every request → profile.System = [one TextBlock]; `buildRequest` emits ONE system message, content form copied from the capture (string vs parts).
- **N > 1 consistently** → profile.System = [N TextBlocks in captured order]; `buildRequest` emits N system messages in order — the OpenAI chat-completions wire allows leading system messages; mimicry means preserving the captured count and order exactly.
- **Mixed/inconsistent within the session** → STOP; that is intra-session divergence (the Phase-9 stability discipline): classify before shipping, re-capture if needed. Never pick the "nicest" form.
- The profile records the decision (a `system_form` note in meta or the blocks themselves carry it implicitly via count) so `profile check dsh` can assert the form still holds against fresh captures.

**The mapping implementation site (DSH-02):** `internal/provider/openai.go` `buildRequest` (line 108) currently builds messages ONLY from the turn's `messages` parameter — `profile.System` is silently dropped (verified this session). The fix: emit the profile's system blocks as leading message(s) in exactly the captured form BEFORE the turn messages, driven purely by profile data. Plus loader tolerance: `internal/profile/loader.go` lines 70-77 ERROR when `thinking.json` / `tool_choice.json` are absent — dsh's profile ships neither (dsh never sends `tool_choice`; `thinking` rides request fields). Loader change: absent file → nil RawMessage (absence is data: "never send this field"), present file → current behavior. A regression test pins that the zcode profile still requires its files (they exist) and dsh loads cleanly without them.

## §4 DeepSeek Dialect Specifics (Flagged Unknown #4 — RESOLVED, web-verified August 2026)

All against official DeepSeek API docs:

| Dialect fact | Verified behavior | Mimicry/engineering consequence |
|---|---|---|
| **`strict: true` on tools** | "In the tools parameter, all functions need to set the `strict` property to true; the server will validate the JSON Schema of the Function" `[CITED: api-docs.deepseek.com/guides/tool_calls/]` | Whether the captured dsh wire carries `strict` is CAPTURE DATA. If present → the profile/tools data carries it and ass-guard MUST send it or the request is distinguishable AND invalid. The pinned go-openai v1.42.0 already has `FunctionDefinition.Strict` (`json:"strict,omitempty"`) — verified via `go doc`; no SDK change. Note `omitempty`: `strict:false` will NOT serialize — matches omit-when-absent wire discipline; only `true` ever goes on the wire. Community bug reports exist (malformed JSON in strict-beta edge cases) — client-side argument parsing already tolerates malformed JSON (existing loop behavior) |
| **Thinking-mode param rejection** | Thinking mode "does not support temperature, top_p, presence_penalty, frequency_penalty. Setting these parameters will not trigger an error" — they are IGNORED (official API); some third-party gateways/SDKs ERROR instead (Vercel AI SDK #4455) `[CITED: api-docs.deepseek.com/guides/thinking_mode/]` | The capability profile (D-09 machinery) declares which params each model tolerates; the OpenAI adapter consults capability fields, never provider names. For the opencode GATEWAY (D-01), observed behavior wins: if the capture shows dsh sending `temperature` to a reasoner model, mimicry sends it too (ignored upstream); the capability `Limitations` list records "ignores sampling params" for cost/behavior modeling |
| **JSON-mode truncation** | "Set the max_tokens parameter reasonably to prevent the JSON string from being truncated midway"; the API "may occasionally fail to output valid JSON" even in JSON mode; `finish_reason:"length"` marks truncation `[CITED: api-docs.deepseek.com/guides/json_mode/ + /api/create-chat-completion]` | Tool-call ARGUMENTS are model JSON — the turn loop's argument parsing must treat `finish_reason:"length"` + malformed-arguments as a retry/re-ask path, not a crash. Reasoning tokens count against `max_tokens` — dsh's defaults (max_tokens 256k-class on the native route) are capture data; the profile's MaxTokens field carries the captured default |
| **Reasoner content split** | Reasoning arrives as a separate `reasoning_content` field on the assistant message (dsh's native adapter preserves it on tool-call turns; never null content) `[CITED: research STACK.md serialize.ts quote-level reading]` | The OpenAI adapter's response parsing must surface `reasoning_content` when present and reproduce dsh's exact assistant-turn serialization (content never null on tool-call turns) — all capture-driven |

**The rule that binds all four (CONTEXT discretion + Pitfall 15):** dialect behavior keys off the CAPTURED WIRE (what dsh actually sends) and the CAPABILITY PROFILE (what the model/endpoint tolerates). Zero `if provider == "deepseek"` branches — a grep gate in the plan enforces it.

## §5 OpenAI Provider Gap Census (code-verified this session)

What `internal/provider/openai.go` must gain for dsh's wire, in capture-driven priority (DSH-02 + DSH-05):

| # | Gap (verified in code) | dsh wire fact (STACK.md §Feature 5, quote-level) | Fix shape |
|---|---|---|---|
| 1 | `buildRequest` (openai.go:108) never reads `profile.System` | System prompt(s) lead the messages array (form per §3) | Emit profile.System blocks as leading message(s), captured form |
| 2 | `Stream` returns `errOpenAIStreamNotImplemented` (openai.go:228) — while the SERVE path calls `Provider.Stream` (session.go:276, subagent.go:193) | `stream: true` ALWAYS + `stream_options: {include_usage: true}` | Implement SSE chat-completions streaming (go-openai CreateChatCompletionStream); always-on per the wire |
| 3 | Profile `Headers` never sent (Send builds no header set) | Identity/telemetry headers (`x-deepseek-harness-user-id` + session/compaction flags on the native route) | Header roundtrip via the client's HTTP transport (config-level default headers), values rendered through the existing header value_template mechanism |
| 4 | `tools` array always built, even when `prof.Tools` is empty | `tools` OMITTED ENTIRELY when empty | Omit-when-empty (nil slice) — never send `"tools": []` |
| 5 | No `strict` threading | (per capture) `strict: true` on function defs | Data-driven: `Decl` gains optional strict flag set by extraction from the captured tools array |
| 6 | `tool_choice` maps only string forms | Native dsh NEVER serializes `tool_choice` | Absent `tool_choice.json` → field never set (loader tolerance, §3) — the existing string-form mapping stays for hypothetical other profiles |
| 7 | No assistant-turn serialization control | Assistant tool-call turns carry `tool_calls[{id,type,function}]` + `reasoning_content`, content never null; tool results `{role:"tool", tool_call_id, content}` with `'(no output)'` for empty | Message-shape handling in the adapter's message construction, keyed by profile data (message-shape fields from the capture), including the literal `'(no output)'` substitution |
| 8 | No `thinking`/`reasoning_effort` request fields | `thinking: {type}` + `reasoning_effort` per capture | Sent only when the captured wire carries them (profile data; absent file = never send) |

**Scheduler wiring (D-04):** `ModelConfig` (scheduler/config.go:39) gains `Profile string \`yaml:"profile"\`` — riding the existing Phase-7 machinery end-to-end: the serve path resolves model → ModelConfig.Profile → loads that profile bundle; `--profile` flag remains the explicit override; empty field → default `zcode` (back-compat: today's scheduling.yaml has no profile keys). The scheduling.yaml DeepSeek provider entry (D-01): `providers: { opencode-gw: { base_url: <gateway>, shape: openai, api_key_env: OPENCODE_GATEWAY_KEY } }` + `models: { deepseek-chat: { provider: opencode-gw, profile: dsh, ... } }` — base-URL-swappable to `https://api.deepseek.com` with zero code change. The gateway's OpenAI-compatibility is a PREREQUISITE VERIFIED step (one live probe before committing the entry, per D-01's wording).

## §6 Zcode-ism Audit (DSH-01) — census and rule

`grep -ri zcode internal/ cmd/ --include="*.go" | grep -v _test` = **69 references** today. Classification (Pitfall 16's rule: after this phase, nothing under `internal/` outside the profile package hardcodes a target-agent name):

- **Load-bearing (genericize or parameterize):**
  - `cmd/ass-guard/profile_check.go` — `loadCaptureLine`'s live path hardcodes `~/.zcode/cli/rollout` (line ~103) and `extractCaptureCounts` parses zcode's `ModelIO` (Anthropic-body) shape. Fix: per-profile capture loader (zcode → rollout freshest line + ModelIO counter; dsh → freshest recording-proxy request line + chat-completions counter); `drift.Detect` itself is already generic (manifest + map).
  - `cmd/extract-profile/main.go` — zcode-only CLI (`-rollout-dir`, default out `profiles/zcode`, name `zcode`). Fix: dsh mode (capture JSONL + zstd session root as inputs) alongside the zcode path; shared bundle-writer.
  - `internal/redact/redact.go` — the walker/secretKeys are generic, but the DOC CONTRACT bakes "the 12 zcode identity header names" as the preservation rule, and any preserved-header list elsewhere must become profile-supplied (CONTEXT discretion names exactly this). Fix: identity-header preservation list moves into profile data (the profile's `headers`/identity bundle IS the list); the package doc rewords to the generic contract.
  - `cmd/ass-guard/parity.go` + `internal/parity/{harness,run,replay,metric}.go` — the suite extractor (`ExtractTurnsFromRollout`) is zcode-JSONL-shaped; LiveArm itself is provider-agnostic. Fix: a dsh suite extractor (from the proxy capture / session events); arm + metrics untouched.
  - `cmd/ass-guard/acp_serve.go` — `--profile` default constant; harmless but gets the per-model resolution from the scheduler (D-04) with the flag as override.
  - `internal/defaults/defaults.go` + firstrun — doc strings name the zcode seed; the embed already reaches `seed/profiles` (a `seed/profiles/dsh` subtree is embedded automatically — DSH-04's sanitized seed rides this).
- **Legitimate (document as zcode-scoped, no change):** `internal/ecosys` (the `.claude/` compat surface IS zcode/Claude-Code layout by definition — that's config compatibility, not mimicry-target leakage), `internal/profile/{extract,loader,types,doc}.go` (the profile package legitimately knows profile #1; extraction tiers are generic), comments in `internal/provider/*` ("zcode-normalized ToolCalls" — reword to "profile-normalized" opportunistically while touching those files), `internal/toolcat` docstrings, `scheduler/load.go` dotted-slug comment.

**Acceptance rule (goes in the plan as a gate):** after Phase 11, `grep -ri "zcode" internal/ cmd/ --include="*.go" | grep -v _test | grep -v "internal/profile\|internal/ecosys"` returns only comments explicitly labeled zcode-scoped (config-compat rationale), zero mimicry-path logic — and NO `provider == "deepseek"` / `profile.Name == "dsh"` switch anywhere outside profile data loading.

## §7 Capture-Provenance Contract (generalizing Phase 9's 09-03/09-04 discipline)

Every entry in `profiles/dsh/` must be traceable to a captured line — the DSH-05 traceability bar:

- `meta.yaml` records: pinned dsh commit (full hash), capture date, provider route (adapter + model + upstream baseURL sans credential), proxy tool commit, capture-file hash, zstd session-file hash, dsh preset/profile in use (Standard/Code/Minimal differ in toolsets — Pitfall 14), and the system-message form decision (§3).
- The extraction writes `coverage.yaml` with per-field `source` entries pointing at capture line numbers (the existing CoverageEntry.Source mechanism, extended to the dsh capture format).
- Drift check (`profile check dsh`): loads the dsh manifest, pulls a fresh capture line (proxy capture file), runs the chat-completions counter, reports TIER-1/2 drifts — same UX as zcode. Re-capture is the documented manual procedure (§1 runbook committed in-repo, `docs/dsh-recapture-runbook.md`), triggered when dsh moves — NO scheduled ritual (D-02).
- Sanitized seed for `internal/defaults`: the profile bundle minus any operator-identifying header VALUES (value_templates keep placeholders); committed only after the leak-guard test (defaults_test.go's /Users//home/ check) passes; the seed's meta says "sanitized" and points at the runbook for the real capture.

## §8 Parity + Live-Probe Design (DSH-05, D-03)

- **Suite:** `parity.ExtractTurnsFromRollout` is zcode-shaped; a dsh extractor turns the recording-proxy capture (request+response pairs) into `CapturedTurn`s (prompt + expected tool-call sequence). The zstd session events corroborate (assistant tool-call events in order).
- **Arm:** `parity.LiveArm{Profile, Provider}` is provider-agnostic (verified: run.go:24) — the dsh parity run loads the dsh profile and builds the OpenAI-shape provider through the scheduler factory (`factory.Build` switches on `prov.Shape` — parity.go already builds via the factory). `--profile dsh` flag exists on the parity cmd already (default zcode).
- **The bar (D-03):** tool-call sequences statistically indistinguishable from live dsh — same methodology as zcode (v1.0 Phase 1): same prompts to both arms, tool-call sequence comparison, surprise check. Thresholds are FRESHLY baselined for dsh (never carry zcode numbers over).
- **Live probe:** one live DeepSeek tool-calling round-trip per ROUTED model (deepseek-chat, deepseek-reasoner if routed) against the D-01 gateway — gated on the gateway credential env var, loud-skip-with-note when absent (the 09-04 pattern; extraction legs never block on the key, the live leg reports pending).
- **Sequencing note:** Phase 10's `internal/runtime` extraction will have moved the turn core before Phase 11 executes (strict order 8→9→10→11). dsh plans touch `internal/provider`, `internal/profile`, `internal/scheduler`, `cmd/*` — none of which Phase 10 relocates (it moves the turn runner/session wiring). Where a dsh plan wires the serve path, reference the post-Phase-10 home (`internal/runtime`) with a read-first on the actual file.

## Architecture Patterns

### System Architecture Diagram (capture → profile → wire → parity)

```
[dsh pinned @ commit]                [recording proxy (dev tool)]         [DeepSeek upstream]
      |  settings.yaml:                    |  forwards + records               ^  D-01 gateway
      |  baseURL=127.0.0.1:8317/v1  -----> |  request/response JSONL           |  (or api.deepseek.com)
      |                                    v                                  |
      |                            capture.jsonl  +  session.jsonl.zstd       |
      |                                    |                                  |
      v                                    v                                  |
 [operator workload ONCE] ------> [cmd/extract-profile dsh mode]              |
        (>=5 turns, >=8 tools,        |  wire-truth extraction (proxy)        |
         >=1 subagent, >=1            |  + zstd event harvest (klauspost)     |
         empty-output turn)           |  system-form decision (§3 procedure)  |
                                       v                                      |
                              profiles/dsh/ bundle                            |
                                (meta: pinned commit, hashes)                 |
                                       |                                      |
        scheduling.yaml:                |  ModelConfig.Profile = dsh           |
        models.deepseek-chat.profile    v                                      |
        = dsh  ----------------> [ass-guard serve path]                        |
                                       |  OpenAI adapter: system blocks,       |
                                       |  always-stream + include_usage,       |
                                       |  headers, omit-when-empty tools,      |
                                       |  strict per capture                   |
                                       +------------ A/B parity ------------->+
                                          suite from capture | ass-guard arm
                                          tool-call sequences compared (D-03)
```

### Pattern 1: Profile = data, adapter = generic machinery
**What:** every dsh-specific wire fact (system form, strict, headers, thinking fields, tool-result substitution) lives in `profiles/dsh/*` + capability fields; `internal/provider/openai.go` reads profile data and emits the wire. **When to use:** everywhere in this phase. **Anti-pattern:** `switch prof.Name { case "dsh": ... }` — banned by DSH-01.

### Pattern 2: Pinned-capture provenance (Phase-9 generalization)
**What:** one operator capture against a pinned target commit; versions + hashes in meta; drift check against the capture bundle; re-capture as a documented manual runbook. **Source:** plans 09-03/09-04 — this phase runs the same shape for dsh with `docs/dsh-recapture-runbook.md`.

### Pattern 3: Skip-clean real-artifact tests
**What:** tests over real capture artifacts (zstd file, proxy capture) skip with a runbook-pointing message when the artifact is absent; synthetic fixtures prove the assertion can fail (canary). **Source:** 09-03's pinned-session test pattern.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| zstd decode | Custom frame parser | `klauspost/compress/zstd` v1.19.2 DecodeAll | Concatenated checksummed frames are the library's default path; hand-rolling frame/XXH64 handling is weeks of fuzz-hardening to redo |
| HTTP reverse proxy | Manual TCP relay | stdlib `httputil.ReverseProxy` | Hop-by-hop headers, chunked bodies, SSE pass-through handled correctly; ~200-line dev tool |
| SSE streaming client | Manual line-reader over raw bodies | go-openai `CreateChatCompletionStream` | Already pinned; handles event parsing + usage chunks; matches the wire the SDK serializes for `stream_options` |
| Dialect detection | Provider-name switches | CapabilityProfile fields (existing D-09 machinery) | The scheduler already declares per-model capabilities/limitations; the adapter consults data |

## Common Pitfalls

### Pitfall A: Source-reading instead of capture (Pitfall 15, inherited)
**What goes wrong:** hand-writing the dsh wire from `serialize.ts`. **Why:** runtime-composed system prompt + adapter ambiguity (llm-pi-ai vs llm-deepseek). **How to avoid:** every profile entry cites a capture line (§7); the plan greps for hand-authored content. **Warning signs:** profile entries with no capture-line source; parity diffs attributed to "model noise" without ruling out wire mismatch.

### Pitfall B: dsh churn invalidating the profile (Pitfall 14, inherited)
**What goes wrong:** dsh (developer preview) ships breaking changes; the profile silently diverges. **How to avoid:** pinned commit in meta; `profile check dsh` drift guard; manual re-capture runbook (D-02 — explicitly NOT scheduled). **Warning signs:** meta without commit/preset fields.

### Pitfall C: zcode-isms surviving the audit (Pitfall 16, inherited)
**What goes wrong:** "shared" code quietly asserts zcode shapes (12 headers, ModelIO parsing, rollout paths) and breaks or mis-redacts dsh. **How to avoid:** the §6 census drives the audit task; the grep gate runs in CI-shaped verification. **Warning signs:** dsh captures failing with "12 header" errors; redaction preserving zcode headers on dsh wire.

### Pitfall D: Retracted zstd versions sneaking in
**What goes wrong:** `go get ...@latest`-ish resolution lands v1.18.1/v1.19.0/v1.19.1 (retracted) or a future bad release; arm64 crashes under profiling. **How to avoid:** pin exactly v1.19.2 in go.mod; treat go's retraction warning as a build failure. **Warning signs:** go.mod showing any other klauspost/compress version.

### Pitfall E: Gating extraction on the API key
**What goes wrong:** the zstd/harvest legs blocked on gateway credentials. **How to avoid:** only the live-probe/parity legs are credential-gated (loud-skip-with-note) — extraction reads local files (09-04's rule, verbatim).

### Pitfall F: Divergent provider-construction copies
**What goes wrong:** a dsh-specific provider construction path beside the factory. **How to avoid:** every provider construction goes through the Phase-9 factory seam — including the parity arm and the live probe (parity.go already does).

## Code Examples

### dsh capture settings.yaml (runbook artifact)
```yaml
# Source: deepseek-harness.github.io/deepseek-harness/en/guide/providers (2026-08-14)
llm-pi-ai:
  providers:
    dsh-capture:
      apiKeyEnv: GATEWAY_API_KEY
      api: openai-completions
      baseURL: http://127.0.0.1:8317/v1
      models:
        - id: deepseek-chat
        - id: deepseek-reasoner
```

### zstd decode (spike core)
```go
// Source: pkg.go.dev/github.com/klauspost/compress/zstd (2026-08-14)
dec, _ := zstd.NewReader(nil,
    zstd.WithDecoderMaxMemory(1<<30)) // bound hostile/corrupt input; checksums stay ON (default)
defer dec.Close()
plain, err := dec.DecodeAll(raw, nil)  // multistream: all concatenated frames, XXH64 verified
// err == zstd.ErrCRCMismatch → corrupt/mid-write artifact: stop, re-capture. Never IgnoreChecksum.
```

### Per-model profile selection (D-04 schema)
```yaml
# scheduling.yaml — rides Phase-7 machinery; base-URL-swappable per D-01
providers:
  opencode-gw:
    base_url: "https://<opencode-gateway>/v1"   # D-01 default; swap to https://api.deepseek.com freely
    shape: openai
    api_key_env: OPENCODE_GATEWAY_KEY
models:
  deepseek-chat:
    provider: opencode-gw
    profile: dsh                                # D-04 — the per-model declaration
    pricing: { input_per_mtoken: 0.27, output_per_mtoken: 1.10 }
    capabilities: { context_window: 128000, max_output_tokens: 8192, tool_calling: true, streaming: true, extended_thinking: false, limitations: ["ignores sampling params in thinking mode", "json-mode truncation on low max_tokens"] }
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| zcode-only profile assumption in shared code | N-profile data-driven architecture | v1.0 design; proven for 1 profile | Phase 11 makes it true for 2 — the thesis test |
| OpenAI adapter as MiniMax-validated Send-only shell | Full dsh wire mapping (system, stream, headers, strict) | this phase | The adapter becomes dialect-complete via data |
| klauspost "latest" | v1.19.2 exact pin (v1.18.1 + v1.19.0/.1 retracted) | 2026 releases | go.mod pin + retraction-warning-as-failure |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The opencode subscription gateway exposes a plain OpenAI-compatible chat-completions endpoint (baseURL + bearer) usable by dsh AND ass-guard | §1, D-01 | LOW-MEDIUM — D-01 already words this as a "prerequisite verified during the phase": the plan makes the one-probe verification the FIRST gated step; official api.deepseek.com is the fallback (base-URL swap, zero code) |
| A2 | dsh custom-provider `baseURL` accepts plain `http://127.0.0.1:<port>/v1` (no https requirement) | §1 | LOW — docs describe arbitrary self-hosted gateways as the use case; the runbook's step-3 trivial turn verifies immediately |
| A3 | Pricing numbers in the §5 example are illustrative placeholders | Code Examples | NONE — capture/config fills real numbers; the plan does not ship the example verbatim |
| A4 | dsh's tool catalog names (shell/terminal/fs/web/todo/subagent/jobs/workflow/lsp/skill/code-runtime) match the pinned commit's surface | §1 workload | LOW — the workload says "at least 8 distinct tools" against whatever the pinned install exposes; the capture census records the real names |

**No other claims are [ASSUMED]; the rest carry [VERIFIED]/[CITED] tags inline.**

## Open Questions (RESOLVED)

1. **Recording-proxy capture runbook** — RESOLVED (§1): positioned plain-HTTP reverse proxy at the configured `llm-pi-ai.providers.<id>.baseURL`; no TLS MITM; full runbook specified; adapter-route ambiguity handled by recording which route served the capture.
2. **zstd decode approach** — RESOLVED (§2): klauspost v1.19.2 exact pin (retractions verified), DecodeAll multistream + default checksums, spike design with real-artifact + synthetic-roundtrip + skip-clean paths.
3. **System-message mapping form** — RESOLVED as a decision procedure (§3): capture-only by nature; the recording contract (what every captured line must carry) and the one-vs-N decision rule are specified; the plan implements the procedure, the capture answers the question.
4. **DeepSeek dialect specifics** — RESOLVED (§4): strict/param-ignore/JSON-truncation/reasoner-split verified against official docs August 2026; keyed by capability profile, capture decides what goes on the wire.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | everything | ✓ | 1.25+ (repo pin) | — |
| klauspost/compress v1.19.2 | zstd harvest | ✓ (Go module proxy) | v1.19.2 | none — do NOT substitute versions |
| dsh @ pinned commit (Node 22.19+/24, pnpm 11.7.0) | capture run (operator machine) | ✗ not yet cloned | master (pin at capture) | no capture without it — plan 11-0N gates on it |
| DSH capture credential (gateway key) | live probe + parity | ✗ operator-held | — | loud-skip-with-note (09-04 pattern); extraction legs unkeyed |
| Real session.jsonl.zstd | zstd spike | ✗ produced by the capture run | — | synthetic round-trip fixture in CI; real-artifact test skip-cleans |

**Missing dependencies with no fallback:** none that block planning — the operator-gated items (dsh clone, credential) are checkpoint tasks by design (Phase-9 precedent: 09-04's `autonomous: false`).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (+ `testify` where already used) |
| Config file | none needed (go.mod) |
| Quick run command | `go test ./internal/profile/ ./internal/provider/ -race` |
| Full suite command | `go test ./... -race && mise run vet && mise run lint` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| DSH-01 | no zcode-specific mimicry paths; no provider-name switches | unit + grep gate | `go test ./internal/redact/ -race && <grep-gate script>` | partially — redact_test.go exists; grep gate is Wave 0 |
| DSH-02 | profile.System maps per capture form; loader tolerates absent thinking/tool_choice | unit (TDD) | `go test ./internal/provider/ -race -run TestOpenAI && go test ./internal/profile/ -race -run TestLoader` | openai_test.go exists (extends) |
| DSH-03 | zstd decode of real artifact; extraction from proxy capture | unit + real-artifact (skip-clean) | `go test ./internal/profile/ -race -run TestZstd` | Wave 0 |
| DSH-04 | per-model profile field; drift check dsh; embedded seed | unit + config | `go test ./internal/scheduler/ -race -run TestProfile && go test ./cmd/ass-guard/ -race -run TestProfileCheck` | config tests exist (extends) |
| DSH-05 | parity green (credential-gated); live round-trip; traceability | integration (operator-gated) | `go test ./internal/parity/ -race` + `parity --profile dsh` run | parity tests exist (extends) |

### Sampling Rate
- **Per task commit:** quick run command (profile + provider packages)
- **Per wave merge:** full suite command
- **Phase gate:** `mise ci` + zstd spike on real artifact + A/B parity green + live round-trip per routed model + traceability audit — per ROADMAP §Phase 11 gate

### Wave 0 Gaps
- [ ] `internal/profile/zstd_test.go` — decode tests (synthetic round-trip + real-artifact skip-clean) — covers DSH-03
- [ ] The zcode-ism grep gate (script/test) — covers DSH-01
- [ ] `cmd/extract-profile` dsh-mode test fixtures (synthetic capture line) — covers DSH-03/DSH-04

## Security Domain

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | Gateway credential via env (`api_key_env`) + `ResolveCredential` (Phase-7 machinery); never in logs/artifacts |
| V3 Session Management | no | no new session surfaces |
| V4 Access Control | no | dev-time tooling binds 127.0.0.1 only |
| V5 Input Validation | yes | capture JSONL parsed with strict JSON decoding; zstd guarded by `WithDecoderMaxMemory`; unknown request fields preserved-as-data, never executed |
| V6 Cryptography | no | no new crypto; checksums are integrity, not security |

### Known Threat Patterns for Go + capture tooling
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Credential capture in the recording proxy | Information disclosure | Redact-at-record-time (Authorization → `[REDACTED]`); capture file 0600; leak-guard test over committed artifacts (defaults_test pattern) |
| Hostile/corrupt zstd artifact crashing the harvester | DoS | klauspost fuzz-hardened decoder + MaxMemory bound; never IgnoreChecksum |
| Prompt-injection via captured dsh content riding into profiles | Tampering | Profile content is STRUCTURE (system blocks, schemas, headers) extracted mechanically; the leak-guard + sanitization step strips operator-identifying values before any seed ships |
| Operator-identifying header values committed (x-deepseek-harness-user-id VALUE) | Information disclosure | Seed sanitization: names preserved, values → placeholders; real capture stays out of the repo (only hashes + counts recorded) |

## Sources

### Primary (HIGH confidence)
- This repo's source, read this session: `internal/provider/openai.go` (buildRequest:108 never reads System; Stream:228 not-implemented), `internal/profile/{types,loader}.go` (loader:70-77 hard-requires thinking/tool_choice), `internal/scheduler/{config,factory}.go` (ModelConfig:39, Build shape-switch), `internal/redact/redact.go`, `cmd/extract-profile/main.go`, `cmd/ass-guard/{profile_check,parity,acp_serve}.go`, `internal/parity/run.go` (LiveArm:24 provider-agnostic), `internal/loop/loop.go` + `internal/session/session.go:276` (serve path uses Stream), `internal/defaults/defaults.go` (embed reaches seed/profiles), `go.mod` (no klauspost yet; go-openai v1.42.0 with Strict/StreamOptions verified via `go doc`)
- dsh official docs: deepseek-harness.github.io/deepseek-harness/en/guide/providers (custom-provider schema, credentials, GET /models, arbitrary baseURL) + github.com/deepseek-ai/deepseek-harness docs/config-catalog.md, examples/headless-agent/cordis.yml, packages/llm/llm-deepseek/README.md (dynamic config)
- DeepSeek API docs: api-docs.deepseek.com/guides/tool_calls/ (strict:true + schema validation), /guides/thinking_mode/ (ignored params), /guides/json_mode/ + /api/create-chat-completion (truncation, finish_reason)
- klauspost/compress: github.com/klauspost/compress/releases (v1.18.1 + v1.19.0/.1 retractions, v1.19.2 fix), pkg.go.dev/github.com/klauspost/compress/zstd (multistream default, ErrCRCMismatch, WithDecoderMaxMemory, sentinel errors), OpenTelemetry Collector issue #15660 (arm64 crash)
- v1.1 project research: `.planning/research/STACK.md` (dsh repo/wire/serial facts, quote-level), `.planning/research/PITFALLS.md` (Pitfalls 14-16), `.planning/research/SUMMARY.md` §Gaps
- Phase 9 plans 09-03/09-04 (committed) — the capture-provenance pattern this phase generalizes

### Secondary (MEDIUM confidence)
- Community corroboration of dialect quirks: Vercel AI SDK issue #4455 (reasoner param errors on some stacks), deepseek-ai/DeepSeek-V3 issue #1069 (strict-beta malformed JSON reports), chat-deep.ai + promptfoo parameter lists (agree with official docs)

### Tertiary (LOW confidence)
- None load-bearing.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — one new dep, retractions + decode semantics verified against upstream; SDK field support verified via go doc against the pin
- Architecture: HIGH — every integration point verified against this repo's source this session; capture design verified against dsh's official config docs
- Pitfalls: HIGH — 14-16 inherited from v1.1 research (grounded in dsh sources); D/E/F added from this session's verification

**Research date:** 2026-08-14
**Valid until:** 2026-09-14 (dsh is developer-preview — the pinned-commit discipline exists precisely because this expires; DeepSeek API facts are doc-stable but re-verify `strict`/reasoner behavior against the CAPTURE, which this plan does by construction)
