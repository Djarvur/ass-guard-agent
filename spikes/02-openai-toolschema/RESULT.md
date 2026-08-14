# #2. sashabaranov/go-openai tool-calling schema (OpenAI-shape providers)

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

*Evidence file for STACK Phase-0 item #2. Plan 00-05 (Wave 3) is the single writer of `.planning/research/VERIFIED-FACTS.md` and folds this RESULT.md in verbatim-ish. This file does NOT touch VERIFIED-FACTS.md. Throwaway spike per D-04/D-05.*
