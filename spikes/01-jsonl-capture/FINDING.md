# Spike 01 FINDING — DRAFT VERIFIED-FACTS.md content (items #1 and #4)

> **This file is the DRAFT evidence Plan 00-05 folds into
> `.planning/research/VERIFIED-FACTS.md`.** Plan 00-05 is the single writer of
> VERIFIED-FACTS.md; this file is the per-spike evidence producer (D-05
> per-spike-file convention — zero `files_modified` overlap with sibling Wave-1
> plans). Plan 00-05 copies the `# #1.` and `# #4.` sections below verbatim-ish
> into VERIFIED-FACTS.md after the user signs off on the Tier-B resolution.
>
> Both items live in this one file because both are no-Go deliverables: #1 is a
> filesystem capture, #4 is a structural closure (D-06). Items #2/#3/#5 each
> write their own `RESULT.md` in their own spike subdir.

---

# #1. zcode JSONL transcript path + per-line schema

- **Fact:** STACK.md §"Open Verification Items" item #1 (line 488) verbatim: *"zcode's exact JSONL transcript path + line schema. claude-code-compat runtimes use `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`, but recent Claude Code versions changed some session-file behavior. Confirm zcode's actual path and the exact JSONL line schema (which fields carry the system prompt, tool catalog, identity)."* The mimicry-capture row (~line 47) restates the same path: *"claude-code-compat runtimes (including zcode) write full message-level transcripts as JSONL at `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`."*

- **Source:** STACK.md §"Open Verification Items" item #1 (line 488); STACK.md mimicry-capture row (~line 47); secondary sources cited in STACK.md Sources (adityabawankule.io, claude-dev.tools, databunny medium — all describe the *Claude Code* layout, which STACK conflated with zcode).

- **Verified:** 2026-08-09

- **Verified against:** on-disk files at the **corrected** path `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl`. Inspected the largest main-session `model-io-sess_<session-id>.jsonl` file on this machine (48 lines, all `type: "model_io"`; the session-id uuid is redacted here per D-03). Cross-checked `~/.claude/projects/<munged-cwd>/*.jsonl` to confirm it is a different product's transcript. Inspection tools: `jq` 1.7.1, `python3` 3.14.6 (dev-time only; no Go program — item #1 is a filesystem capture per CONTEXT.md / RESEARCH.md §7).

- **Status:** `FAILED` — **Tier B (favorable direction).** A load-bearing STACK premise (the transcript path) is wrong. The D-07 resolution is **(a) revise-and-continue**: the capability (extract the zcode profile from on-disk logs) is fully intact and in fact higher-fidelity than STACK projected. **The finding is recorded here before any continuation; explicit user sign-off on the path correction lands in Plan 00-05** (it propagates to MIMC-02's wording, ROADMAP, and any downstream reference to `~/.claude/projects/`). Research predicted revise-and-continue; the on-disk evidence confirms that prediction — see Notes.

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
  - **MIMC-02 wording impact:** any MIMC-02 / PROF-03 / ROADMAP text referencing `~/.claude/projects/<munged-cwd>/...` as the zcode capture path must be corrected to `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl`. This is the MIMC-02 ground-truth path correction awaiting user sign-off in Plan 00-05.
  - **PROF-03 `target_capture_ref`:** = the `sessionId` + the corrected path `~/.zcode/cli/rollout/model-io-sess_<sessionId>.jsonl`. Phase 1 reads these files directly to extract the profile — **no MITM proxy needed for the request body** (the rollout captures the full body + the full 12-header set); residual MITM use is cross-check only, not discovery.
  - **Tier-B resolution pointer:** awaiting user sign-off in Plan 00-05. Research (00-RESEARCH.md §3 / §9) predicted **(a) revise-and-continue** — confirmed by the on-disk evidence: the path is present, the schema is richer than STACK claimed, and the mimicry capability is intact and easier than projected. Three D-07 options for the record: (a) revise-and-continue (predicted, confirmed by evidence), (b) halt-and-replan (not warranted — capability intact), (c) accept-and-document (subset of (a)).

---

# #4. whisper.cpp / STT cross-compile (structurally moot)

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

*End of FINDING.md — Plan 00-05 folds both sections above into `.planning/research/VERIFIED-FACTS.md` after the user signs off on the #1 Tier-B (revise-and-continue) resolution.*
