# VERIFIED-FACTS — Phase 0 Re-verification

> **Post-spike source of truth (D-01).** This file is the dated, sanitized record
> of the five STACK Phase-0 verification items after the throwaway spikes ran.
> `STACK.md` is the **pre-spike recommendation** and is left **unchanged** — what
> we guessed (Aug 2026) never blurs into what we proved (here). Phase 1's
> researcher reads this file as the ground truth for MIMC-02 / PROF-03 / PROF-05.
>
> **Verification date:** 2026-08-09.
> **Author:** Plan 00-05 (the single writer of this file, per D-01/D-05; the four
> Wave-1 evidence files it folds in live under `spikes/0{1,2,3,5}-*/`, one per
> spike, so Wave-1 plans have zero `files_modified` overlap).
> **Completeness gate:** `bash .planning/phases/00-spike-re-verification/check-verified-facts.sh`
> (exits 0; codifies RESEARCH.md §8 + §9 — 5 sections, 5 Status, 5 Evidence, no
> forbidden tokens, #4 STRUCTURALLY-MOOT, §3 path correction recorded).
> **Sanitization:** every embedded sample is D-03-redacted (prompt/system text,
> all ids, token counts, API keys, file paths, tool arguments redacted; JSON keys,
> HTTP header **names**, enum values, and array lengths preserved — they ARE the
> mimicry target). This file is the phase's final secret-leak barrier (threat
> T-00-10, high).

---

## #1. zcode JSONL transcript path + per-line schema

- **Fact:** STACK.md §"Open Verification Items" item #1 (line 488) verbatim: *"zcode's exact JSONL transcript path + line schema. claude-code-compat runtimes use `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`, but recent Claude Code versions changed some session-file behavior. Confirm zcode's actual path and the exact JSONL line schema (which fields carry the system prompt, tool catalog, identity)."* The mimicry-capture row (~line 47) restates the same path: *"claude-code-compat runtimes (including zcode) write full message-level transcripts as JSONL at `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`."*

- **Source:** STACK.md §"Open Verification Items" item #1 (line 488); STACK.md mimicry-capture row (~line 47); secondary sources cited in STACK.md Sources (adityabawankule.io, claude-dev.tools, databunny medium — all describe the *Claude Code* layout, which STACK conflated with zcode).

- **Verified:** 2026-08-09

- **Verified against:** on-disk files at the **corrected** path `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl`. Inspected the largest main-session `model-io-sess_<session-id>.jsonl` file on this machine (48 lines, all `type: "model_io"`; the session-id uuid is redacted here per D-03). Cross-checked `~/.claude/projects/<munged-cwd>/*.jsonl` to confirm it is a different product's transcript. Inspection tools: `jq` 1.7.1, `python3` 3.14.6 (dev-time only; no Go program — item #1 is a filesystem capture per CONTEXT.md / RESEARCH.md §7).

- **Status:** `FAILED` — **Tier B (favorable direction).** A load-bearing STACK premise (the transcript path) is wrong. The D-07 resolution is **(a) revise-and-continue**: the capability (extract the zcode profile from on-disk logs) is fully intact and in fact higher-fidelity than STACK projected. **The finding is recorded here before any continuation; the Tier-B disposition is recorded in Notes below** (it propagates to MIMC-02's wording, ROADMAP, and any downstream reference to `~/.claude/projects/`). Research predicted revise-and-continue; the on-disk evidence confirms that prediction — see Notes.

- **Evidence:** 1 representative `model_io` line, sanitized per D-03 (prompt/system text, all ids, token counts, tool arguments, and file paths redacted; JSON keys, HTTP header **names**, enum values, and array lengths preserved as they ARE the mimicry target):
```json
{
  "type": "model_io",
  "model": {
    "modelId": "GLM-5.2",
    "providerId": "builtin:zai-coding-plan",
    "role": "main",
    "source": "config",
    "variant": "max"
  },
  "sessionId": "[REDACTED: uuid]",
  "turnId": "[REDACTED: str]",
  "requestId": "[REDACTED: uuid]",
  "traceId": "[REDACTED: str]",
  "startedAt": "[REDACTED: ISO timestamp]",
  "completedAt": "[REDACTED: ISO timestamp]",
  "durationMs": "[REDACTED: int]",
  "attempt": 1,
  "querySource": "[REDACTED: str]",
  "request": {
    "body": {
      "model": "GLM-5.2",
      "max_tokens": "[REDACTED: int]",
      "metadata": { "user_id": "[REDACTED]" },
      "output_config": "[REDACTED: object]",
      "stream": "[REDACTED: bool]",
      "system": [
        { "type": "text", "text": "[REDACTED: system-prompt block, ~N chars]", "cache_control": "[REDACTED if present]" },
        { "type": "text", "text": "[REDACTED: system-prompt block]", "cache_control": "[REDACTED if present]" },
        { "type": "text", "text": "[REDACTED: system-prompt block]", "cache_control": "[REDACTED if present]" }
      ],
      "thinking": "[REDACTED: Anthropic extended-thinking object, e.g. {type, budget_tokens}]",
      "tool_choice": "[REDACTED: tool-choice directive]",
      "tools": [
        { "name": "[REDACTED: tool name]", "description": "[REDACTED]", "input_schema": { "$schema": "[REDACTED]", "type": "object", "properties": "[REDACTED: JSON-schema properties]", "required": "[REDACTED: required-field names]", "additionalProperties": "[REDACTED: bool]" } }
      ]
    },
    "headers": {
      "HTTP-Referer": "[REDACTED]",
      "User-Agent": "[REDACTED]",
      "X-Os-Category": "[REDACTED]",
      "X-Os-Version": "[REDACTED]",
      "X-Platform": "[REDACTED]",
      "X-Title": "[REDACTED]",
      "X-ZCode-Agent": "[REDACTED]",
      "X-ZCode-App-Version": "[REDACTED]",
      "x-query-id": "[REDACTED]",
      "x-request-id": "[REDACTED]",
      "x-session-id": "[REDACTED]",
      "x-zcode-trace-id": "[REDACTED]"
    },
    "messages": [
      { "role": "system", "content": "[REDACTED: prompt content]" },
      { "role": "user", "content": "[REDACTED: prompt content]" }
    ],
    "maxOutputTokens": "[REDACTED: int]",
    "providerOptions": { "anthropic": { "thinking": "[REDACTED]" } },
    "toolNames": ["[REDACTED: ...77 tool names, parallel to body.tools]"],
    "messageCount": "[REDACTED: int]",
    "messagesKind": "full",
    "messageOffset": "[REDACTED: int]"
  },
  "response": {
    "finishReason": "tool-calls",
    "text": "[REDACTED: assistant text output]",
    "toolCalls": [
      { "id": "[REDACTED: call id]", "name": "[REDACTED: tool name]", "input": { "[REDACTED: arg name]": "[REDACTED: arg value]" } }
    ],
    "usage": {
      "inputTokens": "[REDACTED: int]",
      "outputTokens": "[REDACTED: int]",
      "totalTokens": "[REDACTED: int]",
      "cacheReadTokens": "[REDACTED: int]",
      "cacheWriteTokens": "[REDACTED: int]"
    },
    "headers": { "[REDACTED: response header names/values]" },
    "modelId": "glm-5.2",
    "responseId": "[REDACTED: msg id]",
    "providerMetadata": { "anthropic": "[REDACTED]" }
  }
}
```
**Redaction note (D-03):** 1 representative `model_io` line; redacted all prompt/system text, all ids (session/trace/turn/request/call/message), all token counts, all tool arguments, all header *values*, and file paths. **Preserved** all JSON keys, the 12 HTTP header **names** (the identity fingerprint Phase 1 must mimic), enum values (`type`, `role`, `finishReason`, `messagesKind`, `providerId`), structural shape, and array lengths (`system` = 3 blocks, `tools` = 77 entries observed). Verified structural counts on the inspected line: `request.headers` has exactly 12 keys; `request.body.tools` has exactly 77 entries; `response.toolCalls[0]` has keys `{id, name, input}`; `response.usage` has 5 token-count fields.

- **Notes:**
  - **Corrected path:** `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl` (plus `model-io-no-session.jsonl` for pre-session calls; `model-io-sess_subagent_agent_<id>.jsonl` for subagent sessions — same schema). NOT `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`.
  - **Corrected schema:** each line is `type: "model_io"` carrying the **full wire-level request/response** (body + headers + system + tools + response.toolCalls + usage), strictly higher fidelity than STACK's "message-level transcript" description. Verified top-level keys (13), `request.*` keys (9), `request.body.*` keys (9: `max_tokens, metadata, model, output_config, stream, system, thinking, tool_choice, tools`), `request.headers` (exactly 12 identity header names), `response.*` keys (8). `request.body.system` = array of `{type, text, cache_control}` (Anthropic system-block shape). `request.body.tools` = array of `{name, description, input_schema}` (Anthropic tool-catalog shape; 77 tools observed). `request.body.thinking` + `request.body.tool_choice` present (Anthropic extended-thinking knobs not described by STACK). `response.toolCalls[]` = `{id, name, input}` (zcode-normalized, not raw Anthropic `tool_use` block shape).
  - **Obsolete convention:** the `<munged-cwd>` → `<-replaced-cwd>` path encoding is a Claude-Code artifact and does NOT apply to zcode. zcode keys transcripts by `session-id` (a UUID) under `~/.zcode/cli/{rollout,agents,exec,artifacts}/`.
  - **Different-product proof:** `~/.claude/projects/<munged-cwd>/*.jsonl` exists on this machine and uses `type: "queue-operation"` with keys `{operation, sessionId, timestamp, type}` — that is **Claude Code's** transcript schema, a different product. STACK conflated the two because both are "claude-code-compat runtimes"; their on-disk layouts and schemas are entirely different.
  - **Anthropic-shape body + provider identity:** `request.body` is Anthropic-shape (`system` array, `tools` with `input_schema`, `max_tokens`, `thinking`); `model.providerId: "builtin:zai-coding-plan"` confirms zcode talks to Z.ai's GLM via the Anthropic protocol (matches STACK's Z.ai finding). The zcode profile's `message_shape.provider` is **`anthropic`**, not `openai`.
  - **MIMC-02 wording impact:** any MIMC-02 / PROF-03 / ROADMAP text referencing `~/.claude/projects/<munged-cwd>/...` as the zcode capture path must be corrected to `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl`. This is the MIMC-02 ground-truth path correction, resolved per the Tier-B disposition recorded below.
  - **PROF-03 `target_capture_ref`:** = the `sessionId` + the corrected path `~/.zcode/cli/rollout/model-io-sess_<sessionId>.jsonl`. Phase 1 reads these files directly to extract the profile — **no MITM proxy needed for the request body** (the rollout captures the full body + the full 12-header set); residual MITM use is cross-check only, not discovery.
  - **Tier-B resolution (D-07): research-predicted default option-a (revise-and-continue) applied — confirmed by on-disk evidence (plans 00-01..00-04), documented as the prediction in 00-05-PLAN.md Task 2, and applied by default after the user declined to override the checkpoint. Reversible at Phase 1 planning.** Research (00-RESEARCH.md §3 / §9) predicted (a) revise-and-continue; the on-disk evidence confirms that prediction — the path is present, the schema is richer than STACK claimed, and the mimicry capability is intact and easier than projected. Three D-07 options for the record: (a) revise-and-continue (predicted, confirmed by evidence, **applied**), (b) halt-and-replan (not warranted — capability intact), (c) accept-and-document (subset of (a)).
  - **Consequence of option-a (verbatim from 00-05-PLAN.md):** → Phase 1 MIMC-02 path wording to be corrected to `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl` during Phase 1 planning; munged-cwd convention obsolete for zcode.

---

## #2. sashabaranov/go-openai tool-calling schema (OpenAI-shape providers)

**Status: PARTIAL** — schema verified offline; live round-trip deferred pending operator key provisioning.

- **Fact:** STACK item #2 — the `sashabaranov/go-openai` row: "*latest (1.x; last pkg.go.dev publish Aug 2025, repo active 2026)* — The de-facto Go client for OpenAI-compatible endpoints. ~10.7k stars, references GPT-5/5.5 in repo description (2026-active). Used for OpenAI-shape providers (MiniMax M3 'apply' route, OpenRouter passthrough). **Caveat:** publish cadence on pkg.go.dev looks ~1 year stale vs. GitHub repo activity — pin to the latest tag at build time and **verify the tool-calling schema matches the target provider's expectation during Phase 0.**" (research/STACK.md, Inherited Core Technologies table row).
- **Source:** `.planning/research/STACK.md` §"Inherited Core Technologies" `sashabaranov/go-openai` row; `.planning/research/STACK.md` §"Open Verification Items (Phase-0 spike checklist)" item #2; `.planning/research/STACK.md` §"Version Compatibility" (`sashabaranov/go-openai` latest ↔ MiniMax M3, OpenRouter, Groq: "*Verify the tool-calling schema matches each provider at Phase 0.*"). Secondary: https://github.com/sashabaranov/go-openai , https://pkg.go.dev/github.com/sashabaranov/go-openai .
- **Verified:** 2026-08-09.
- **Verified against:** `github.com/sashabaranov/go-openai@v1.42.0` (released 2026-08-02; the verified-as-of-2026-08-09 latest tag, pinned in `spikes/go.mod`), compiled with `go1.26.5` against the spikes module `github.com/djarvur/ass-guard-spikes`. **Provider round-trip status:** none round-tripped — `MINIMAX_API_KEY`, `GROQ_API_KEY`, and `OPENAI_API_KEY` were all unset at execution time, so all three target providers (MiniMax M3, Groq, generic OpenAI-shape triple) were SKIPPED. The schema-fidelity assertions ran offline and PASS. VERIFIED requires re-running with at least one provider key provisioned.
- **Status:** `PARTIAL`. Reason: operator had no provider key provisioned; the live tool-call round-trip against {MiniMax M3, Groq} could not be executed. This is the Tier-A fallback (CONTEXT.md D-07 / RESEARCH.md §9): a missing key is a doc/minor condition — record PARTIAL with the corrected/deferred state and continue. The schema shape (the load-bearing #2 question) is verified offline; only the provider-specific live quirk observations remain open.
- **Evidence:** request body the spike serialized via go-openai's `Tool` / `FunctionDefinition` / `jsonschema.Definition` types (D-03 redacted; this IS the Chat Completions schema being verified — `tools[].function` wrapper, string-only tool `type`, `tool_choice:"required"`, the `tool`-role message shape asserted structurally by the test suite):
```json
  {
    "messages": [
      { "content": "What is the weather in Paris right now? Use the tool.", "role": "user" }
    ],
    "model": "MODEL_PLACEHOLDER",
    "stream": false,
    "tool_choice": "required",
    "tools": [
      {
        "function": {
          "description": "Retrieves current weather for the given location.",
          "name": "get_weather",
          "parameters": {
            "properties": {
              "location": { "description": "City name, e.g. Paris", "type": "string" }
            },
            "required": ["location"],
            "type": "object"
          }
        },
        "type": "function"
      }
    ]
  }
```

  Per-assertion result lines from the spike's structured `=== SPIKE 02 RESULT ===` footer (stderr):
```
  offline.schema_request_shape: PASS          # tools[].type=="function" + tools[].function.{name,description,parameters}
  offline.arguments_is_string_type: PASS      # ToolCall.Function.Arguments is string-typed (Go field), parses into {location:string}
  offline.tool_result_message_shape: PASS     # role:"tool" + tool_call_id + JSON-string content closes the loop
  live.providers_round_tripped: 0
  live.providers_skipped: MiniMax(no key), Groq(no key), OpenAI-shape(no key)
  overall_status: PARTIAL
  partial_reason: operator had no provider key provisioned; live round-trip deferred
  ```

  Reference response shape (OpenAI Chat Completions spec, asserted by the offline test `TestToolCallArgumentsIsJSONStringIntoSchema` using go-openai's own `ToolCall`/`FunctionCall` types — `arguments` is a JSON-encoded STRING, the load-bearing divergence point from providers that return a parsed object):
```json
  {
    "role": "assistant",
    "tool_calls": [
      {
        "id": "call_abc123",
        "type": "function",
        "function": { "name": "get_weather", "arguments": "{\"location\":\"Paris\"}" }
      }
    ]
  }
  ```

  Reference tool-result follow-up message (the shape the second turn sends back; asserted by `TestToolResultMessageSerializesChatCompletionsShape`):
```json
  { "role": "tool", "tool_call_id": "call_abc123", "content": "{\"temperature\":25}" }
  ```

  Redaction note: request body, response body, and tool-result message excerpted above; `Authorization` header and any API key value would be replaced with `[REDACTED]` (the spike's `redact()` / `scrubError()` — verified clean against a synthetic `sk-...` key run). Tool-call schema shape (JSON keys, the `function` wrapper, `type`/`role` enum values, the string-typed `arguments`) preserved — these ARE the schema Phase 1's OpenAI-shape adapter conforms to.
- **Notes:**
  - **Chat Completions vs Responses API (RESEARCH.md §4).** The OpenAI docs function-calling page now leads with the **Responses API** shape (`output[].function_call`, `function_call_output`). The `sashabaranov/go-openai` client targets **Chat Completions** (`tools[].function`, assistant `tool_calls[]`, `role:"tool"` result message) — verified by reading the v1.42.0 source (`chat.go`: `chatCompletionsSuffix = "/chat/completions"`; `Tool.MarshalJSON` emits the `{"type","function"}` wrapper; `ToolCall.Function.Arguments string`). The spike correctly asserts the Chat Completions shape; Phase 1's OpenAI-shape provider adapter builds on this and MUST NOT drift to the Responses API shape.
  - **pkg.go.dev cadence nuance (RESEARCH.md §1).** STACK's caveat that "publish cadence on pkg.go.dev looks ~1 year stale vs. GitHub repo activity" is **correct but not stagnation**: the GitHub repo actively released v1.40.0 → v1.42.0 over May–Aug 2026 (v1.42.0 on 2026-08-02). pkg.go.dev indexing lags the source; the library is healthy. Phase 1 should pin an exact tag (`@v1.42.0` or later) and not re-litigate the pkg.go.dev lag.
  - **Provider quirks NOT yet observed (open under PARTIAL).** The three MiniMax/Groq divergence points RESEARCH.md §4 flagged — (a) does MiniMax accept the same `tools[]` shape? (b) does `tool_choice:"required"` work? (c) is `arguments` returned as a JSON string (OpenAI spec) or a parsed object (some clones)? — could not be tested without a provider key. The spike is built to answer exactly these once a key is provisioned: `runRoundTrip()` asserts (a)+(b) via the live response and (c) via `assertArgumentsIsJSONStringIntoSchema`. Groq is predicted to be a close OpenAI clone (RESEARCH.md §4); MiniMax M3 is the higher-divergence-risk provider.
  - **Tier disposition: Tier A (CONTEXT.md D-07).** PARTIAL with a missing-key reason is the doc/minor fallback — record + continue. **Not Tier B**: the failure is not a schema divergence go-openai cannot represent (the library serializes the correct Chat Completions shape, proven offline); it is an operator-key provisioning gap. No halt-and-replan, no three-option user decision required. Plan 00-05 records item #2 as "schema VERIFIED offline; live round-trip PARTIAL — deferred pending operator key provisioning."
  - **What unblocks VERIFIED (the single operator action):** export `MINIMAX_API_KEY` (MiniMax platform console → API keys) and/or `GROQ_API_KEY` (Groq Console → API Keys), then re-run `cd spikes && go run ./02-openai-toolschema/`. The spike auto-detects whichever key(s) are set, round-trips each, and the footer flips to `overall_status: VERIFIED` once at least one provider passes all three live assertions (request shape + tool_calls string arguments + follow-up references result). Until then the schema fidelity (the load-bearing #2 question) stands verified offline.

---

## #3. ACP v1 method names + wire shape

**Status: VERIFIED**

- **Fact:** "ACP (Agent Client Protocol) v1 — spec v1 (v2 is Draft, not stable). The primary IDE-native interface. JSON-RPC 2.0 over stdio; the editor (Zed, JetBrains) spawns the agent as a subprocess; stdout reserved for protocol frames, all logging to stderr (non-negotiable LSP-style discipline). Method names (`initialize`, `session/prompt`, `session/update`, `session/load`) confirmed against the canonical spec pages." *(STACK.md §"Open Verification Items" item #3 + the ACP table row, paraphrased; the item #3 question was "ACP v1 method names against the canonical spec repo — pin the spec version; some community impls lag.")*
- **Source:** STACK.md §"Open Verification Items (Phase-0 spike checklist)" item #3 (line 490); STACK.md ACP table row (~line 33, "Inherited Core Technologies").
- **Verified:** 2026-08-09
- **Verified against:** the canonical ACP v1 spec at `https://agentclientprotocol.com/protocol/v1/` (pages: `transports.md`, `overview.md`, `initialization.md`, `session-setup.md`, `prompt-turn.md` — fetched 2026-08-09 via the site's `.md` content endpoint), exercised empirically through a real `initialize → session/new → session/prompt` exchange that observed a `session/update` notification before the prompt response. The peer exercised was an **in-process mock ACP server** (a goroutine speaking the spec framing over `io.Pipe`, grounded in the canonical spec — not a free-form echo) inside the throwaway spike `spikes/03-acp-handshake/main.go` (module `github.com/djarvur/ass-guard-spikes`). No external ACP server was reachable/reachable without operator setup, so the in-process mock was chosen per the plan's peer-selection discretion (option c); the mock is spec-grounded so the round-trip checks the wire shape, not the client's own guesses.
- **Status:** VERIFIED — all four methods round-tripped against spec-shaped frames; `overall_status: VERIFIED` in the spike's structured footer; exit code 0.
- **Evidence:** one representative frame each for the four methods, hand-extracted from the spike's exchange (the redacted transcript in the footer confirms each was actually sent/received). All frames are newline-delimited single-line JSON objects — **no `Content-Length` header**, no embedded newlines — exactly the canonical framing `transports.md` mandates.
  <!-- spike-03-evidence: each frame is one JSON object terminated by '\n' on the wire -->
```
  {"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":false,"writeTextFile":false},"terminal":false},"clientInfo":{"name":"ass-guard-spike","version":"0"}}}
  {"jsonrpc":"2.0","id":0,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"sessionCapabilities":{"resume":{},"close":{}},"mcpCapabilities":{"http":true,"sse":true}},"agentInfo":{"name":"ass-guard-mock-agent","version":"0"},"authMethods":[]}}
  {"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"[REDACTED: path]","mcpServers":[]}}
  {"jsonrpc":"2.0","id":1,"result":{"sessionId":"[REDACTED: uuid]"}}
  {"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"[REDACTED: uuid]","prompt":[{"type":"text","text":"[REDACTED: content]"}]}}
  {"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"[REDACTED: uuid]","update":{"sessionUpdate":"agent_message_chunk","messageId":"[REDACTED: str]","content":{"type":"text","text":"[REDACTED: content]"}}}}
  {"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}
  ```

  Per-line: (1) initialize request; (2) initialize response — note the field name `agentCapabilities` and the echoed integer `protocolVersion`; (3) session/new request — `cwd` + `mcpServers:[]`; (4) session/new response — `sessionId`; (5) session/prompt request — `sessionId` + a `prompt` content array (NO `tools`/`messages`); (6) session/update **notification** — note: `method` + `params` but NO `id`, and `params.update` carries the `sessionUpdate` discriminator; (7) session/prompt response — `stopReason`.

  Structured footer (stderr) emitted by `go run ./03-acp-handshake/`:
```
  === SPIKE 03 RESULT ===
  protocol: ACP v1
  framing: newline-delimited JSON-RPC (NO Content-Length; no embedded newlines)
  peer: in-process mock ACP server (spec-grounded, no model call, no secrets)
  lifecycle exercised: initialize -> session/new -> session/prompt (observed session/update)
    PASS | initialize (protocolVersion + agentCapabilities)
    PASS | session/new (returns sessionId)
    PASS | session/prompt (>=1 session/update w/ discriminator before response)
  overall_status: VERIFIED
  methods_confirmed: initialize, session/new, session/prompt, session/update
  result_field_confirmed: agentCapabilities (NOT capabilities/serverInfo)
  protocolVersion_sent: integer 1 (canonical spec example form)
  === END ===
  ```

  **Redaction note (D-03):** the `cwd` tempdir path, the `sessionId` value, the `messageId`, and all prompt/response `text` content were redacted to `[REDACTED: path|uuid|str|content]`. **Preserved** (intentionally — they are the mimicry/implementation target, not secrets): all JSON keys, all method names (`initialize`/`session/new`/`session/prompt`/`session/update`), all result field names (`protocolVersion`/`agentCapabilities`/`agentInfo`/`authMethods`/`sessionId`/`stopReason`/`sessionUpdate`), enum values (`end_turn`/`agent_message_chunk`), the capability flag names (`loadSession`/`readTextFile`/`writeTextFile`/`terminal`/`http`/`sse`/`resume`/`close`), the integer `protocolVersion: 1`, and the JSON-RPC 2.0 envelope structure. No tokens, API keys, or live session data arise (the peer is a local mock — critical_constraint #7).
- **Notes:**
  1. **Tier-A lifecycle correction (the load-bearing finding):** STACK's lifecycle `initialize / session/prompt / session/update / session/load` **omits the required `session/new` step**. The canonical spec (`overview.md` "Message Flow" + `session-setup.md`) fixes the lifecycle as `initialize → (authenticate, if required) → session/new` (or `session/load`, gated on `agentCapabilities.loadSession`) `→ session/prompt`. You **cannot** send `session/prompt` without first obtaining a `sessionId` from `session/new`. The full confirmed v1 method set (server-side, what ass-guard implements): `initialize`, `authenticate`, `session/new`, `session/prompt`, `session/load`, `logout`, `session/set_mode`, `session/cancel`. Client-side callbacks (server → editor): `session/update`, `session/request_permission`, `fs/read_text_file`, `fs/write_text_file`, `terminal/*`, `elicitation/*`. This is a Tier A doc/minor correction (D-07) — a missing step in STACK, not a wrong method. Recorded + continued (no halt).
  2. **Framing rule (load-bearing for Phase 2's ACP adapter AND sibling spike #5):** the wire is **newline-delimited JSON-RPC** (`\n`), **NOT** LSP-style `Content-Length` headers. Verbatim from `transports.md`: *"Messages are delimited by newlines (`\n`), and **MUST NOT** contain embedded newlines. The agent **MUST NOT** write anything to its `stdout` that is not a valid ACP message. The client **MUST NOT** write anything to the agent's `stdin` that is not a valid ACP message."* This matches the project's transport discipline (PROJECT.md "Transport discipline"; AGENTS.md) exactly. STACK's framing description ("JSON-RPC 2.0 over stdio; stdout reserved for protocol frames") is **consistent** with the spec — no discrepancy on framing. The `session/load` step (a STACK-mentioned method) is real but optional (requires `loadSession: true`); the spike did not exercise it because the in-process mock always returns `loadSession: true` and `session/new` already proves the sessionId mechanism.
  3. **Result field name is `agentCapabilities` — NOT `capabilities` or `serverInfo` (differs from LSP/MCP).** `initialize.md` confirms: the result object carries `protocolVersion`, `agentCapabilities` (the capability object), `agentInfo` (a `{name, title?, version}` object — NOT `serverInfo`), and `authMethods`. Phase 2's ACP adapter must use the exact field name `agentCapabilities` or a spec-conformant client will not find the agent's capabilities. The spike's offline test `TestInitializeResultUsesAgentCapabilitiesFieldName` asserts both that `agentCapabilities` is present AND that `capabilities`/`serverInfo` are absent.
  4. **`protocolVersion` is an integer `1`, not a string.** `initialization.md` ("Protocol version") states: *"the protocol versions … are a single integer that identifies a MAJOR protocol version."* All canonical examples send the integer `1` (`"protocolVersion": 1`). The spike sends the integer `1`; the mock echoes the integer `1`. (RESEARCH.md §2 flagged a schema-string-vs-example-integer ambiguity; the canonical spec resolves it unambiguously in favor of the integer. Phase 2's `InitializeRequest` struct should type `protocolVersion` as `int`. If a future spec revision moves to a string, the negotiation rule ("Client sends latest it supports; Agent echoes same or its latest") still holds.)
  5. **`session/update` is a JSON-RPC notification (no `id`), carrying a `sessionUpdate` discriminator.** `prompt-turn.md` confirms the agent streams `session/update` notifications during a prompt turn, with `params.update.sessionUpdate` being one of `plan`, `agent_message_chunk`, `tool_call`, `tool_call_update`, `usage_update` (and `user_message_chunk` during `session/load` replay). The spike asserts ≥1 `session/update` arrives before the matching `session/prompt` response and that the notification carries the discriminator. Phase 2's streaming path (provider SSE → event bus → `session/update`) maps directly onto this shape.
  6. **Which peer was exercised:** an in-process mock ACP server (`serveMockAgent` in `spikes/03-acp-handshake/main.go`) — deterministic, secret-free, no model call. The mock's responses are spec-grounded (the `initialize` result advertises the canonical capability set; `session/new` returns a `sessionId`; `session/prompt` streams one `agent_message_chunk` notification then responds with `stopReason: end_turn`). This proves the wire shape + method names + field names against the canonical spec; it does NOT prove interop with a specific third-party ACP server (Zed's, etc.), which is out of Phase 0's scope — Phase 2's ACP adapter will validate against Zed directly. No discrepancy vs STACK on framing; the only correction is the omitted `session/new` step (Note 1).
  7. **No discrepancy vs STACK beyond Note 1.** STACK's framing, transport discipline, and stable-v1/Draft-v2 characterization all hold against the canonical spec. The version pin question ("some community impls lag") is moot for ass-guard, which implements the spec directly (Phase 2) rather than consuming a third-party ACP library.

---

## #4. whisper.cpp / STT cross-compile (structurally moot)

**Status: STRUCTURALLY-MOOT** (D-06 — STT is always an external utility; closed structurally, no spike). Full D-02 field block below.

- **Fact:** STACK.md §"Open Verification Items" item #4 (line 491) verbatim: *"whisper.cpp cross-compile impact — if local STT is in scope, confirm the subprocess approach (`whisper-cli`) preserves goreleaser's clean macOS+Linux amd64+arm64 matrix (it should, since it is out-of-process, but verify)."* The supporting `ggml-org/whisper.cpp/bindings/go` row (~line 74) is the LOW-confidence row: *"Optional — only if the configured STT backend is `whisper-cpp-local`. Adds a C dependency to the build (breaks pure-Go static binary claim — verify goreleaser cross-compile impact before committing). The OpenAI Whisper API default avoids this entirely."*

- **Source:** STACK.md §"Open Verification Items" item #4 (line 491); STACK.md supporting-library row `github.com/ggml-org/whisper.cpp/bindings/go` (~line 74, LOW confidence); STACK.md "What NOT to Use" whisper.cpp-via-cgo row; STACK.md Focus 3 §B (STT backends).

- **Verified:** 2026-08-09

- **Verified against:** CONTEXT.md decision D-06 ("STT is ALWAYS an external utility — ass-guard NEVER bundles STT via cgo") + RESEARCH.md §6 reasoning chain (the architectural rule that generalizes STACK's "do not cgo-bind whisper.cpp" to ALL STT backends).

- **Status:** `STRUCTURALLY-MOOT`.

- **Evidence:** Reasoning chain (D-06 closes this structurally; no code sample is possible or needed). D-06 mandates STT is **always** an external utility — ass-guard NEVER bundles STT via cgo. This holds for every backend: (1) **OpenAI Whisper API** (v1 default) = an HTTPS call via `go-openai` — zero cgo, zero cross-compile impact; (2) **whisper.cpp local** = an out-of-process `whisper-cli` subprocess the user installs (`exec.Command`, never the `ggml-org/whisper.cpp/bindings/go` cgo binding) — the binary is built separately with its own C toolchain and is NOT part of ass-guard's goreleaser matrix; (3) **Groq STT** = an HTTPS call (same OpenAI-compatible client, base URL swap) — zero cgo. Therefore ass-guard's goreleaser matrix (pure Go, `CGO_ENABLED=0`, macOS+Linux amd64+arm64) is **unaffected by STT choice** under D-06. STACK item #4's premise (cgo cross-compile risk from bundling whisper.cpp) **does not arise** under this architecture — the cgo binding is structurally forbidden, so the cross-compile question it raises has no object to apply to.
- **Notes:**
  - This rule **propagates to v2 STT work**: the external-utility rule is settled and will not be relitigated. Should local STT be pulled into scope in v2+, the subprocess model (`whisper-cli`) is the mandated shape; the cgo cross-compile concern remains closed because the cgo binding remains forbidden.
  - **No spike produced** (item #4 has no spike directory; CONTEXT.md per-item assignment, RESEARCH.md §7). The closure is the one-paragraph reasoning above, citing D-06.
  - **Optional micro-confirmation NOT performed** (RESEARCH.md §6 offered a 5-line `exec.LookPath("whisper-cli")` program as belt-and-suspenders): omitted as the structural closure does not depend on whether the binary happens to be installed on this dev machine — the architecture forbids the cgo binding regardless.
  - **Cross-references:** RESEARCH.md §6 (D-06 reasoning chain); CONTEXT.md D-06 (the architectural rule); STACK.md ~line 74 (the LOW-confidence row being closed) and the "What NOT to Use" whisper.cpp-via-cgo row (which already pointed at the subprocess model — D-06 generalizes it from whisper.cpp to all STT backends).
  - Phase 0 closes **5/5 items** (not 4/5 + a defer): #4 is closed structurally here, not deferred.

---

## #5. go-telegram/bot + ACP stdout collision

**Status: VERIFIED**

- **Fact:** STACK item #5 (verbatim): "`go-telegram/bot` + ACP stdout-collision test — integration test that both frontends running in one process never write a non-ACP frame to stdout." Reinforced by the go-telegram/bot Version-Compatibility line (STACK ~line 480): "`go-telegram/bot` + ACP stdio in one process | Both share `log/slog` → stderr | **Critical:** Telegram frontend must never write to stdout (stdout is ACP's). Enforce via review/lint." And the Stack-Pattern line (STACK ~line 291): "Telegram logging MUST go to stderr (stdout is ACP's)."

- **Source:** `.planning/research/STACK.md` §"Open Verification Items (Phase-0 spike checklist)" item #5 (line 492); STACK table rows `go-telegram/bot` (~lines 50, 63, 248, 480, 192); `.planning/ROADMAP.md` Phase 0 Success Criterion #3 ("the `go-telegram/bot` + ACP stdout-collision integration test demonstrates the two transports don't fight over stdout"); AGENTS.md "Transport discipline" (stdout reserved exclusively for ACP JSON-RPC frames; all logging/diagnostics to stderr).

- **Verified:** 2026-08-09

- **Verified against:** `github.com/go-telegram/bot@v1.23.0` (released 2026-08-03), via the integration spike at `spikes/05-stdout-collision/main.go` — a real `go-telegram/bot` long-poll loop running in a goroutine alongside canned ACP frames written to a captured `os.Stdout`, with a byte-equality assertion proving zero non-ACP bytes. Also verified by direct inspection of the `go-telegram/bot@v1.23.0` package source in the module cache (the load-bearing library-behavior claim — see Evidence + Notes (a)).

- **Status:** `VERIFIED`

- **Evidence:**
  The spike runs the two transports concurrently in ONE PROCESS and asserts the captured stdout is byte-equal to the canned ACP frames the spike wrote (zero extra bytes from the bot). Run output (`go run ./05-stdout-collision/` from `spikes/`, stderr footer):
```
  === SPIKE 05 RESULT ===
  claim: go-telegram/bot@v1.23.0 + ACP coexist in one process; stdout stays byte-clean
  library: github.com/go-telegram/bot@v1.23.0
  topology: main goroutine writes canned ACP frames to os.Stdout; bot goroutine runs b.Start(ctx)
  token: [REDACTED: dummy token] (no real Telegram connection; WithSkipGetMe keeps New() offline)
  canned_frame_count: 3
  canonical_byte_count: 415  (the bytes the spike wrote to captured stdout)
  captured_byte_count: 415  (the bytes read back from captured stdout; must == canonical)
  stdout_extra_byte_count: 0  (0 = byte-clean; the load-bearing number)
  stderr_sink_byte_count: 132  (handler-routing evidence sink; marker "SPIKE05_HANDLER_PROBE_MARKER" present=true)
    PASS | clean stdout (captured == canned ACP frames; byte-equal, 0 extra) — captured 415 bytes == canonical 415 bytes; extra-byte count 0 (byte-equal)
    PASS | default silence (WithDebug NOT set → no stdout writes) — captured stdout byte-equal to canned frames — debug path silent by default
    PASS | handler routing (errors → stderr-bound sink, never stdout) — synthetic error marker found in stderr sink and absent from captured stdout
    PASS | shutdown (bot goroutine exits within 2s of context cancel) — bot goroutine exited within 2s of cancel (context-first contract holds)
  overall_status: VERIFIED
  === END ===
  ```

  The canned ACP frames written to stdout (D-03 redacted; 3 frames, newline-delimited JSON-RPC 2.0 per Plan 00-03 — an initialize response, a session/update notification, a session/prompt response):
```
  {"id":1,"jsonrpc":"2.0","result":{"agentCapabilities":{"loadSession":true},"agentInfo":{"name":"ass-guard-spike-acp","version":"0"},"protocolVersion":1}}
  {"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"[REDACTED: id]","update":{"content":{"text":"[REDACTED: content]","type":"text"},"messageId":"[REDACTED: id]","sessionUpdate":"agent_message_chunk"}}}
  {"id":2,"jsonrpc":"2.0","result":{"stopReason":"end_turn"}}
  ```

  Load-bearing library-behavior verification (direct source inspection of `go-telegram/bot@v1.23.0`, module cache path `~/go/pkg/mod/github.com/go-telegram/bot@v1.23.0/`): a recursive grep for `os.Stdout` / `fmt.Print` / `fmt.Fprint(os.Stdout…` across the package's non-test, non-`examples/` Go files returns **ZERO matches**. The only output surfaces are three default handlers in `bot.go`, all using stdlib `log.Printf` (which writes to `log.Default()` → `os.Stderr`):
```
  // bot.go:147-157  (go-telegram/bot@v1.23.0)
  func defaultErrorsHandler(err error)  { log.Printf("[TGBOT] [ERROR] %v", err) }
  func defaultDebugHandler(format string, args ...any) { log.Printf("[TGBOT] [DEBUG] "+format, args...) }
  func defaultHandler(_ context.Context, _ *Bot, update *models.Update) { log.Printf("[TGBOT] [UPDATE] %+v", update) }
  ```

  The `defaultDebugHandler` is gated behind `b.isDebug` — it only fires when `WithDebug()` is set (`get_updates.go:40`, `raw_request.go:61,135`). With `WithDebug()` NOT set (the spike's configuration), the debug path is silent by default. The `defaultErrorsHandler`/`defaultHandler` are overridable via `WithErrorsHandler`/`WithDefaultHandler`. There is **no `WithLogger`, no `io.Writer` option, no unconditional stdout write** anywhere in the v1.23.0 API surface.

  Redaction note (D-03): the dummy bot token (`dummyTokenForSpike()`) is replaced with `[REDACTED: dummy token]` throughout; the canned frames redact `sessionId`/`messageId` → `[REDACTED: id]` and `text` → `[REDACTED: content]`, preserving all JSON keys, method names, field names (`agentCapabilities`, `protocolVersion`, `sessionUpdate`, `stopReason`), enums (`end_turn`, `agent_message_chunk`), and framing. Module-cache file paths in this Evidence field are reduced to the generic `~/go/pkg/mod/...` form. Plan 00-05 re-runs the grep over the merged VERIFIED-FACTS.md as a second barrier.

- **Notes:**

  (a) **The load-bearing finding — `go-telegram/bot@v1.23.0` is SILENT BY DEFAULT and never writes to stdout.** Direct source inspection confirms zero `os.Stdout`/`fmt.Print` writes in the package (the only matches are in `examples/` — separate sample programs, not the library). All library output funnels through three default handlers in `bot.go` (`defaultErrorsHandler`, `defaultDebugHandler`, `defaultHandler`), each using stdlib `log.Printf` → `log.Default()` → `os.Stderr`. The library has **no `WithLogger` and no `io.Writer` option** — all output routing is via the `WithDebugHandler`/`WithErrorsHandler`/`WithDefaultHandler` callbacks. The debug path is gated behind `b.isDebug` (set only by `WithDebug()`); without `WithDebug()`, `defaultDebugHandler` never fires. This is exactly what RESEARCH.md §5 predicted; the spike confirms it empirically (assertion B: captured stdout byte-equal to the canned frames with `WithDebug` NOT set).

  (b) **Tier-A doc refinement vs RESEARCH.md §5 (handler signatures).** RESEARCH.md §5 documented the callback signatures as `ErrorsHandler func(ctx context.Context, err error)` and `DebugHandler func(ctx context.Context, message string, err error, additionalData any)`. The **actual** `go-telegram/bot@v1.23.0` signatures (bot.go:27-28) are simpler: `type ErrorsHandler func(err error)` and `type DebugHandler func(format string, args ...any)`. This is Tier A (cosmetic doc drift — a slightly over-described signature; the load-bearing "callbacks, no stdout, no io.Writer" claim is unchanged). The spike uses the REAL v1.23.0 signatures; Phase 5's Telegram frontend must use them too.

  (c) **The mitigation recipe (propagation rule for the real Phase-5 implementation).** ass-guard's v2 Telegram frontend follows this exact pattern to keep stdout ACP-only:
    1. Construct the bot with `bot.New(token, bot.WithErrorsHandler(makeStderrErrorsHandler(os.Stderr)))` — route errors to `slog` → `os.Stderr`.
    2. Do **NOT** register `WithDebug()` or a stdout-writing `WithDebugHandler` (default silence).
    3. Belt-and-suspenders: call `log.SetOutput(os.Stderr)` at process start so even the library's default handlers (if one is accidentally left un-overridden) stay off stdout.
    4. Run the long-poll loop as `go b.Start(telegramCtx)` in a goroutine; cancel `telegramCtx` on editor-initiated shutdown to drain the loop.

  (d) **Context-first contract confirmed (assertion D).** The bot goroutine exits within 2 seconds of `telegramCtx` cancellation — the context-first shutdown contract STACK picked the library for (Focus 3: "idiomatic `context.Context` throughout — load-bearing for clean cancellation/shutdown when the ACP server owns process lifecycle"). `b.Start(ctx)` blocks on the long-poll loop and returns cleanly on `ctx.Done()`.

  (e) **Empirical proof shape.** The spike captures the process's real `os.Stdout` via `os.Pipe()`, writes 3 canned ACP frames (415 bytes) to the pipe's write-end, runs the bot goroutine concurrently, then reads the bytes back from the read-end and asserts `captured (415) == canonical (415)` with `stdout_extra_byte_count: 0`. Assertion C additionally determinizes the handler-routing path: the spike directly invokes the bot's configured `WithErrorsHandler` with a synthetic error carrying a unique marker, and proves the marker lands in the stderr-bound sink (132 bytes, `present=true`) and is absent from the captured stdout. This is observable behavior, not structural reasoning.

  (f) **Tier disposition: VERIFIED (Tier A — the favorable outcome).** The library is silent by default and routes all output through stderr-bound callbacks; the transport discipline (stdout = ACP only) holds with `go-telegram/bot` in-process. No Tier-B (architectural) finding: no stdout-write default that can't be silenced, no library contract bug on shutdown. The only correction is the Tier-A handler-signature doc refinement in (b). ass-guard's Phase-5 Telegram frontend can proceed against `go-telegram/bot@v1.23.0` with the recipe in (c).

---

*End of VERIFIED-FACTS.md — Phase 0 re-verification complete. All 5 STACK Phase-0 items closed with dated, sanitized evidence (#1 FAILED→revise-and-continue [Tier-B resolved], #2 PARTIAL [schema VERIFIED offline, live round-trip deferred pending operator keys], #3 VERIFIED [Tier-A session/new correction recorded], #4 STRUCTURALLY-MOOT [D-06], #5 VERIFIED [Tier-A handler-signature refinement recorded]). STACK.md is unchanged (D-01); this file is the post-spike source of truth.*
