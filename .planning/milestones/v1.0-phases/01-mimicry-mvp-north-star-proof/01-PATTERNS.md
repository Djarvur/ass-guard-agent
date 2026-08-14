# Phase 1: Mimicry MVP — Pattern Map

**Mapped:** 2026-08-09
**Phase:** 01 — Mimicry MVP (north-star proof)
**Source:** `01-CONTEXT.md`, `01-RESEARCH.md`, `spikes/` (throwaway references), VERIFIED-FACTS.md

> **Greenfield caveat.** This is a fresh Go module (no production code exists yet — only the throwaway `spikes/` module). PATTERNS.md here does NOT map "new file → existing production analog." It maps each Phase-1 file to (a) the closest spike reference to study (NOT import — spikes are an isolated module, Phase-0 D-05), (b) the SDK type/convention to follow, and (c) the project convention (transport discipline, redaction, structured footer) established by Phase 0. Every pattern below is grounded in dated, inspectable source.

---

## Conventions established by Phase 0 (apply to ALL Phase-1 files)

### C1. Transport discipline — stdout is ACP-only; all diagnostics to stderr
**Source:** PROJECT.md "Transport discipline"; AGENTS.md; `spikes/02-openai-toolschema/main.go:51-53`; `spikes/05-stdout-collision/` (VERIFIED).
**Pattern:** every Phase-1 file that emits human-readable output writes to `os.Stderr` via a `log/slog`-backed helper or `fmt.Fprintf(os.Stderr, ...)`. `os.Stdout` is reserved for ACP JSON-RPC frames (Phase 2's server; Phase 1 has no ACP server, but the discipline holds so Phase 2 doesn't refactor).
```go
// spikes/02-openai-toolschema/main.go:51-53 — the spike's sole diagnostic sink
func stdlog(format string, args ...any) {
    fmt.Fprintf(os.Stderr, format+"\n", args...)
}
```
**Apply in:** `internal/loop`, `internal/audit`, `cmd/ass-guard`, `cmd/extract-profile`, all tests. A test asserting "stdout is clean" (capture `os.Stdout`, assert byte-empty or byte-equal to canned frames) is the spike-05 pattern — replicate it for the Phase-1 CLI entrypoint.

### C2. Secret redaction — LOG-03 discipline, applied at every output boundary
**Source:** `spikes/02-openai-toolschema/main.go:377-445` (`redact`, `walkRedact`, `isSecretKey`, `scrubError`, `redactRaw`).
**Pattern:** a JSON-tree walker that replaces secret-carrier VALUES while preserving field NAMES, plus regex belt-and-suspenders for non-JSON bodies and error strings. The `isSecretKey` allowlist (`authorization, api_key, api-key, apikey, key, bearer, token, x-api-key`, case-insensitive) is the canonical secret-key set.
```go
// spikes/02-openai-toolschema/main.go:406-422 — reusable LOG-03 shape
func walkRedact(node any) {
    switch v := node.(type) {
    case map[string]any:
        for k, val := range v {
            if isSecretKey(k) { v[k] = "[REDACTED]"; continue }
            walkRedact(val)
        }
    case []any:
        for i := range v { walkRedact(v[i]) }
    }
}
```
**Apply in:** `internal/audit` (redact the verbatim shaped request before writing to the audit log — the 12 identity header NAMES are kept, the auth/x-api-key VALUES are redacted). Also apply in the parity harness's results-file writer and the extractor's provenance output. **Promote this to a shared `internal/redact` package** — Phase 1 has ≥3 callers; do not copy-paste.

### C3. Provider config from env (Tier-A fallback on missing key)
**Source:** `spikes/02-openai-toolschema/main.go:64-110` (`providersFromEnv`).
**Pattern:** resolve provider targets from env vars (`ZAI_API_KEY`, `OPENAI_API_KEY`, base URLs); a provider whose key is unset is SKIPPED, not fatal. If NO key is set, the harness exits non-zero with a clear stderr message and records `Status: PARTIAL` (the Tier-A fallback from Phase 0 — operator must provision a key).
**Apply in:** `internal/loop` / the parity harness — the live ass-guard arm needs `ZAI_API_KEY` (Z.ai GLM, Anthropic-shape). Mark the live parity run `autonomous: false` (operator-gated on key presence) while the metric/harness/replay logic stays unit-tested and autonomous.

### C4. SDK-driven request building (not hand-marshaled JSON)
**Source:** `spikes/02-openai-toolschema/main.go:114-141` (`buildWeatherToolRequest` uses `openai.Tool` / `FunctionDefinition` / `jsonschema.Definition` so the library's serialization is exercised, not hand-built JSON); anthropic-sdk-go native types confirmed in RESEARCH §5.1.
**Pattern:** populate the SDK's native param types (`MessageParam`, `ContentBlockParamUnion`, `TextBlockParam`, `ToolParam{InputSchema}`, `ThinkingConfigEnabledParam`, `ToolChoiceUnionParam` for Anthropic; `ChatCompletionRequest` with `[]openai.Tool` for OpenAI). Let the SDK serialize. Capture the serialized body (via a teeing `http.RoundTripper` — `teeTransport`, spike 02:159-178 — or by marshaling the param struct) for the audit log + assertions.
**Apply in:** `internal/shaper` (Anthropic native types + `option.WithHeader` escape hatch per D-09), `internal/provider/anthropic.go`, `internal/provider/openai.go`.

### C5. Capturing round-trip bytes (for audit + assertions)
**Source:** `spikes/02-openai-toolschema/main.go:148-178` (`capturedRoundTrip`, `teeTransport`).
**Pattern:** wrap the HTTP client's transport to tee response body + capture request body; assert on the captured bytes (redacted before printing).
**Apply in:** the parity harness's conformance tests and `internal/audit` — the verbatim shaped request IS the audit payload (LOG-01). For Anthropic-sdk-go, the SDK exposes `option.WithResponseInto(&raw)` (seen in `betaagent.go:98`) to capture raw response bytes; use that or a teeing transport.

### C6. Structured stderr footer (deterministic test output)
**Source:** `spikes/02-openai-toolschema/main.go` `=== SPIKE 02 RESULT ===` footer; `spikes/03-acp-handshake/main.go` `=== SPIKE 03 RESULT ===` footer.
**Pattern:** emit a structured `=== RESULT ===` ... `=== END ===` block to stderr with one `key: value` per line and per-assertion `PASS|FAIL | description` lines. Machine-parseable, greppable, and the verify step can grep for `overall_status: PASS`.
**Apply in:** the parity harness — emit a `=== PARITY RESULT ===` footer with `suite_size`, `layer1_pass_rate`, `layer2_pass_rate`, `overall_status: PASS|FAIL`, per-prompt lines. The plan's `<verify>` greps this.

---

## File-by-file pattern map

### `go.mod` (repo root — Phase 1 starts it)
- **Closest analog:** `spikes/go.mod` (module `github.com/djarvur/ass-guard-spikes`, `go 1.25`, requires `go-telegram/bot v1.23.0` + `sashabaranov/go-openai v1.42.0`).
- **Pattern:** `module github.com/djarvur/ass-guard-agent`, `go 1.25` (STACK floor), `require ( anthropic-sdk-go; sashabaranov/go-openai v1.42.0; spf13/cobra; spf13/viper; stretchr/testify )`. Do NOT import `go-telegram/bot` (Phase 5) or `modelcontextprotocol/go-sdk` (Phase 5). Keep `CGO_ENABLED=0` clean.

### `internal/profile/` (PROF-01..05 — profile types, loader, coverage)
- **Closest analog:** none in production; convention is STACK's "internal/profile" recommendation + RESEARCH §4.1 directory layout.
- **Pattern:** `profile.Profile` struct mirrors the §4.1 directory (`System []TextBlock`, `Tools []Decl`, `Headers []Header{Name, ValueTemplate}`, `Thinking`, `ToolChoice`, `Model`). Loader reads the directory; `coverage.yaml` is the PROF-05 manifest. Use `gopkg.in/yaml.v3` (or viper) for YAML. **The loader reads profiles BY NAME (PROF-01) — the string "zcode" appears here as a data-driven name lookup, which is allowed (D-11 lint scope is `internal/shaper/` only).**

### `internal/shaper/` (MIMC-01, D-09, D-10 — the mimicry chokepoint)
- **Closest analog:** `spikes/02-openai-toolschema/main.go:114-141` (`buildWeatherToolRequest`) — same SDK-driven build pattern, but with `anthropic-sdk-go` types and profile-driven content.
- **SDK types (RESEARCH §5.1, verified in module cache `anthropic-sdk-go@v1.61.0`):**
  - `MessageParam` / `ContentBlockParamUnion` for messages
  - `TextBlockParam` for the 3 system blocks (TIER-1 byte-faithful)
  - `ToolParam{InputSchema ToolInputSchemaParam}` + `ToolUnionParam` for the 77 tools
  - `ThinkingConfigEnabledParam{BudgetTokens int64}` for thinking
  - `ToolChoiceUnionParam` (with `ToolChoiceParamOfTool(name)`) for tool_choice
  - `option.WithBaseURL`, `option.WithAPIKey`, `option.WithHeader(key, value)` / `WithHeaderAdd` (escape hatch for the 12 identity headers, `option/requestoption.go:148/231/240`)
- **Pattern:** the Shaper takes a `profile.Profile` + messages, returns the populated SDK params + the per-request `option.RequestOption` slice (one `option.WithHeader` per identity header, generated from `profile.Headers` in a loop — DATA-DRIVEN, not hardcoded zcode). **The string "zcode" MUST NOT appear in `internal/shaper/` outside test fixtures (D-11 lint).**

### `internal/provider/` (PROV-01..03 — the common interface + 2 adapters)
- **Closest analog (OpenAI adapter):** `spikes/02-openai-toolschema/main.go` wholesale — the request-build (`buildWeatherToolRequest`), the round-trip (`teeTransport`), and the parse (string-typed `function.arguments` → `ToolCall.Input`) are the reference. **Chat Completions shape, NOT Responses API** (VERIFIED-FACTS.md item #2).
- **Closest analog (Anthropic adapter):** `anthropic-sdk-go` native (`client.Messages.New` with shaped params + header options; base URL `https://api.z.ai/api/anthropic`).
- **Pattern:** `provider.Provider` interface (`Send(ctx, profile, messages, tools) (Response, error)`, RESEARCH §6.1). `Response.ToolCalls` is `[]{Name string, Input json.RawMessage}` — the zcode-normalized shape (`response.toolCalls[] = {id, name, input}`, VERIFIED-FACTS.md item #1). Each adapter's `TranslateToInternal` maps the SDK's native tool-call representation to this. Conformance tests round-trip both adapters (PROV-02).

### `internal/toolcat/` (TOOL-01..03 — built-in catalog + schema adapter)
- **Closest analog:** none in production; the ~20 built-in tools' schemas come from the zcode transcripts (`request.body.tools[].input_schema`).
- **Pattern:** `Catalog.Get(name) (Tool, bool)` + `Catalog.Satisfies(profileDecls) (unsatisfied []string, ok bool)` (RESEARCH §5.2). The built-in tools are `Agent, AskUserQuestion, Bash, CronCreate, CronDelete, CronList, Edit, EnterPlanMode, ExitPlanMode, Read, ReadSessionContext, SendMessage, Skill, TaskStop, TodoRead, TodoWrite, WebFetch, WebSearch, Write` (RESEARCH §1.2). `Tool.Execute` is a stub in Phase 1 (D-15). `Satisfies` feeds TOOL-03 CI.

### `internal/loop/` (D-12 — test-harness Turn Loop)
- **Closest analog:** `spikes/02-openai-toolschema/main.go`'s send-parse flow, narrowed to a single turn.
- **Pattern:** `Run(ctx, profile, provider, prompt, toolStubs) ([]provider.ToolCall, error)` (RESEARCH §7.1). Single-turn (D-15). Publishes `RequestShaped` to the event bus before sending (D-13). The loop is replaced by the Session Core in Phase 2 — keep the interface narrow.

### `internal/event/` (D-13 — minimal event bus)
- **Closest analog:** none; convention is a typed publish/subscribe.
- **Pattern:** `Bus.Subscribe(kind, handler)`, `Bus.Publish(e)`. One event type in Phase 1: `RequestShaped{VerbatimRequest, Profile, Timestamp}`. Recommend goroutine-per-subscriber dispatch from day one (RESEARCH §7.2 — matches LOG-02's eventual contract).

### `internal/audit/` (LOG-01 — audit-log subscriber)
- **Closest analog:** none; combines C2 (redaction) + C5 (verbatim capture).
- **Pattern:** subscribe to `RequestShaped`, redact the verbatim request via `internal/redact` (C2), write to a file (or stderr). NEVER stdout (C1). Records `user input, model requests (verbatim shaped), tool calls, engine decisions` (LOG-01 wording) — Phase 1 records the verbatim shaped request (the mimicry evidence source); the other event kinds arrive in later phases.

### `internal/parity/` (MIMC-03/04, D-01..05 — A/B parity harness + metric)
- **Closest analog:** the spike footer pattern (C6); the replay concept is novel.
- **Pattern:** two arms — ass-guard arm (`internal/loop.Run` live), zcode arm (replay `response.toolCalls[].name` from `eea3dc48` JSONL per D-05). Metric: Layer 1 (tool-name sequence equality) + Layer 2 (per-tool arg structural equality with the normalization rules in RESEARCH §3.2). Curated suite: 5–15 prompts from `eea3dc48` turns (RESEARCH §3.3). Emit the `=== PARITY RESULT ===` footer (C6).

### `internal/drift/` (PROF-04 — profile-check drift detector)
- **Closest analog:** `redact()`'s tree-walk pattern (C2) — drift detection is a field-by-field diff against `coverage.yaml`, tiered by D-06.
- **Pattern:** fresh-capture diff: load the profile's `coverage.yaml` (expected fields per tier), compare a freshly-captured request field-by-field, report TIER-1/2 drift (TIER-3 audit-only). Unit-testable with fixtures (a known-drift "after" fixture → assert the diff reports exactly the drifted TIER-1/2 fields). The live `profile check zcode` command spawns zcode + captures + diffs (D-08, operator-gated).

### `cmd/ass-guard/` (cobra root + `profile` subcommand)
- **Closest analog:** STACK `spf13/cobra` recommendation.
- **Pattern:** root command `ass-guard`; subcommand `profile` with children `check <name>` (PROF-04, D-08) and optionally `extract <session-id>` (the extractor). The `acp` subcommand is Phase 2 — do NOT add it. Transport discipline: cobra commands write to stderr for human output (C1).

### `cmd/extract-profile/` (MIMC-02 extractor — dev-time tool)
- **Closest analog:** `spikes/01-jsonl-capture/` was a `jq`/`python3` capture; the Go extractor formalizes it.
- **Pattern:** stream `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl` (corrected path, RESEARCH §0/§9), parse with `encoding/json` into a struct mirroring VERIFIED-FACTS.md item #1, extract TIER-1/2 fields, assert shape stability across lines (RESEARCH §1.3), write `profiles/zcode/`. Filter the `null`-named-tool edge case (RESEARCH §1.2). Emit a `=== EXTRACT RESULT ===` footer (C6).

### `profiles/zcode/` (the shipped profile artifact — RESEARCH §4.1)
- **Closest analog:** none; content is extracted, not authored.
- **Pattern:** `profile.yaml`, `system/block-{0,1,2}.txt` (byte-faithful TIER-1), `tools.json` (77 declarations, TIER-1), `identity.yaml` (12 header names + value templates, TIER-2), `thinking.json`, `tool_choice.json`, `coverage.yaml` (PROF-05), `meta.yaml` (`target_capture_ref`, PROF-03). **Check it in** (RESEARCH §13.3 — makes the parity test reproducible without the operator's transcripts).

### `profiles/synthetic/` (D-11 — PROF-02 conformance fixture)
- **Closest analog:** none; minimal non-zcode fixture.
- **Pattern:** different system blocks ("You are a synthetic test agent"), different tool names (`synth_tool_a/b`), different headers (`X-Synth-Test`). Loaded through the SAME Shaper code path; asserts the outgoing request carries synthetic fields, not zcode. Catches any `if profile.Name == "zcode"` branch.

---

## PATTERN MAPPING COMPLETE

Phase 1's files map cleanly to (a) Phase-0 spike references (studied, not imported), (b) anthropic-sdk-go / go-openai native types, and (c) six reusable conventions (transport discipline, redaction, env-driven provider config, SDK-driven building, round-trip capture, structured footers). The greenfield caveat is the only nuance: there is no production analog to copy, so the patterns are conventions + SDK idioms + spike references. The `internal/redact` package is the one shared utility with ≥3 callers — extract it early.
