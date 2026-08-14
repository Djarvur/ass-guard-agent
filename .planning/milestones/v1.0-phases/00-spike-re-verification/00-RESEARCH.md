# Phase 0 Research: Spike + Re-verification

**Researched:** 2026-08-09
**Researcher:** Claude (research subagent)
**Scope:** Gather planner-ready findings to PLAN Phase 0 (verification of 5 STACK Phase-0 items). No code written; no planning artifacts modified except this file.

> **Read alongside:** `00-CONTEXT.md` (decisions D-01..D-07 — the locked contract), `research/STACK.md` §"Open Verification Items" (the 5 facts being verified), `REQUIREMENTS.md` (MIMC-02/PROF-03/PROF-05 — what item #1 unblocks downstream).

---

## HEADLINE FINDING — read this first

**Item #1 (zcode JSONL path + schema) is a Tier B architectural finding, but in the *favorable* direction.** STACK's claimed path `~/.claude/projects/<munged-cwd>/<session-id>.jsonl` does **NOT exist** on this machine. zcode writes its transcripts in a **completely different location** with a **completely different (and higher-fidelity) schema**:

- **Actual path:** `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl` (plus `model-io-no-session.jsonl` for pre-session calls)
- **Organization:** by `session-id` under `~/.zcode/cli/{rollout,agents,exec,artifacts}/sess_<id>/` — NOT by munged-cwd
- **Schema:** each line is `{"type": "model_io", "model": {...}, "request": {body, headers, messages, ...}, "response": {text, toolCalls, usage, headers, ...}, ...}` — i.e. **full wire-level request/response capture**, including HTTP headers, the Anthropic-shape `body.system`/`body.tools`/`body.messages`, and the response's `toolCalls[]`/`usage`/`finishReason`.
- **Fidelity:** strictly **higher** than STACK claimed. STACK described "message-level" transcripts; zcode actually logs the **wire payload** (the exact thing Phase 1's mimicry profile needs). The `body.tools` array (77 Anthropic-shape tool declarations observed) and the `headers` block (`User-Agent`, `X-ZCode-*`, `x-session-id`, etc.) are exactly the identity/system-prompt fields MIMC-02/PROF-03/PROF-05 need.

This is Tier B because the **path is wrong** (a load-bearing STACK premise) — but it surfaces as **option (a) revise-and-continue** under D-07, not (b) halt-and-replan: the *capability* (extract zcode profile from on-disk logs) is fully intact and in fact easier than STACK projected (no MITM proxy needed for the request body — it's already on disk). **The planner must flag this for explicit user acknowledgment** in PLAN.md (the path correction propagates to MIMC-02's wording, ROADMAP, and any downstream reference to `~/.claude/projects/`). Details in §3.

All other items (#2–#5) are on track to verify cleanly; their version pins are confirmed current as of 2026-08-09.

---

## 1. Current Version Pins (for the spike's `spikes/go.mod`)

All confirmed via the GitHub Releases pages on 2026-08-09.

| Package | STACK-claimed | Verified latest (2026-08-09) | Pin in `spikes/go.mod` | Drift |
|---|---|---|---|---|
| `github.com/sashabaranov/go-openai` | "latest (1.x; last pkg.go.dev publish Aug 2025)" | **`v1.42.0`** (released 2026-08-02) | `github.com/sashabaranov/go-openai@v1.42.0` | **Tier A** — STACK said "pkg.go.dev looks ~1 year stale"; pkg.go.dev lag is real but GitHub repo is active. v1.42.0 is current. Pin explicitly. |
| `github.com/go-telegram/bot` | "latest (active 2026)" | **`v1.23.0`** (released 2026-08-03) | `github.com/go-telegram/bot@v1.23.0` | None (STACK said "latest"; confirm the concrete tag). |
| `github.com/anthropics/anthropic-sdk-go` | "v1.62.0 (Jul 2026)" | **`v1.62.0`** (released 2026-08-06) | (only if a spike needs it — see §7) | None — STACK's "Jul 2026" is slightly off (actual release date 2026-08-06) but the version number is exact. |
| ACP spec | "v1 stable; v2 Draft" | **v1 stable confirmed; v2 Draft confirmed** | n/a (spec, not a Go dep) | None. Quote: "v2 is a Draft… various pieces can, and will, change before stabilization. Don't ship it by default in production until we are closer to stabilization." |

**Pin guidance for the planner:** the spike `go.mod` should pin **exact tags** (`@v1.42.0`, `@v1.23.0`), not `@latest` — D-02's `Verified against` field demands a concrete version string, and `@latest` would make the spike non-reproducible. STACK's own caveat ("pin to the latest tag at build time") agrees.

> Note on `go-openai` cadence: STACK flagged "publish cadence on pkg.go.dev looks ~1 year stale vs. GitHub repo activity." That observation is **correct** — pkg.go.dev indexing lags — but it does **not** indicate library stagnation. The GitHub repo is actively releasing (v1.40.0 → v1.42.0 over May–Aug 2026). The VERIFIED-FACTS.md entry for #2 should record this nuance so Phase 1 doesn't re-litigate it.

---

## 2. ACP v1 Wire Specifics (for the #3 handshake spike)

**Framing (load-bearing for the spike):** **newline-delimited JSON** (`\n`), NOT Content-Length headers. Verbatim from `/protocol/v1/transports`:
> "Messages are delimited by newlines (`\n`), and **MUST NOT** contain embedded newlines."
> "The agent **MUST NOT** write anything to its `stdout` that is not a valid ACP message."
> "The client **MUST NOT** write anything to the agent's `stdin` that is not a valid ACP message."

This matches the project's transport discipline (stdout = ACP only) and confirms hand-rolled framing is trivial (~line scanner + `encoding/json`). Each unit is a JSON-RPC 2.0 `request` (has `id` + `method`), `notification` (has `method`, no `id`), or `response` (has `id` + `result`/`error`).

**Method names (confirmed against `/protocol/v1/overview`):**
- Agent methods (server-side, what ass-guard implements): `initialize`, `authenticate`, `session/new`, `session/prompt`, `session/load`, `logout`, `session/set_mode`, `session/cancel`.
- Client methods (server calls back to the editor): `session/update` (the token stream), `session/request_permission`, `fs/read_text_file`, `fs/write_text_file`, `terminal/*`, `elicitation/*`.

> **Correction to the phase brief / STACK:** STACK lists the v1 lifecycle as `initialize / session/prompt / session/update / session/load`. The canonical spec adds **`session/new`** as a required step between `initialize` and `session/prompt` — you cannot send `session/prompt` without first obtaining a `sessionId` from `session/new`. The #3 spike must therefore exercise `initialize` → `session/new` → `session/prompt` → observe `session/update`. This is a **Tier A doc/minor correction** (a missing step, not a wrong method). The planner should instruct the executor to spike all four (and optionally `session/load`).

### `initialize`
- **Request params** (`InitializeRequest`): `protocolVersion` (**string** per schema definition — but the documented examples send the integer `1`; the spike should send `"1"` per the schema and confirm the server accepts both), `clientInfo` (optional object `{name, version}` — noted as "will be required in future versions"), `clientCapabilities` (defaults to `{"fs":{"readTextFile":false,"writeTextFile":false},"terminal":false}`), `_meta`.
- **Result** (`InitializeResult`): `protocolVersion`, `agentCapabilities` (object — note: field name is `agentCapabilities`, NOT `capabilities`/`serverInfo`; this differs from LSP/MCP). `agentCapabilities` advertises things like `loadSession: true`, `sessionCapabilities: {resume: {}, close: {}}`, `mcpCapabilities: {http: true, sse: true}`.

### `session/new`
- **Request params** (`NewSessionRequest`): `cwd` (**required**, absolute path), `mcpServers` (**required** array — may be empty `[]`), `additionalDirectories` (optional), `_meta`.
- **Result:** contains the `sessionId` needed for subsequent calls.

### `session/prompt`
- **Request params:** `sessionId` (from `session/new`), `prompt` (array of content parts: `{type: "text", text}`, `{type: "resource", resource: {uri, mimeType, text}}`, etc.). **No `messages` or `tools` field** — the agent owns its own tool catalog and conversation state.
- **Result** (returned at turn end): `{stopReason: "end_turn" | "max_tokens" | "max_turn_requests" | "refusal" | "cancelled"}`.

### `session/update` (notification — agent → client, streamed)
- **Params:** `sessionId`, `update` object with a `sessionUpdate` discriminator: `plan`, `agent_message_chunk` (carries `messageId` + `content`), `tool_call`, `tool_call_update`, `usage_update`.
- This is the token stream. The spike should send a prompt and read `session/update` frames from the agent's stdout until the `session/prompt` response arrives.

### `session/load`
- **Request params:** `sessionId` (of an existing session). **Requires** `agentCapabilities.loadSession: true` from `initialize`.
- Documented as "Load an existing session"; the #3 spike may exercise it as a 5th step if a second handshake is cheap.

**What the #3 spike needs (planner-ready):**
1. Read JSON-RPC frames from stdin (`bufio.Scanner` + `json.Decoder`), write to stdout (`json.Encoder` + explicit `\n`). No Content-Length.
2. Send `initialize` → read response → assert `protocolVersion` and that `agentCapabilities` is present.
3. Send `session/new` with `cwd` = tempdir and `mcpServers: []` → read response → capture `sessionId`.
4. Send `session/prompt` with a trivial text prompt → read `session/update` frames until the `session/prompt` response arrives → assert at least one `agent_message_chunk`.
5. Print all assertions + a redacted frame dump to **stderr** (D-03: capture into VERIFIED-FACTS.md).

The spike can talk to **any** ACP v1 server (e.g. an existing agent like zcode itself if it speaks ACP, or a minimal reference server). The planner should let the executor pick whatever is reachable; the goal is confirming method names + wire shape, not a specific peer.

---

## 3. zcode JSONL Specifics for #1 (filesystem investigation on THIS machine)

### The path is NOT what STACK claimed

STACK item #1 says: `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`. **That path exists on this machine, but it is for *Claude Code* (claude-cli), not zcode.** The directories under `~/.claude/projects/` are keyed by munged-cwd (e.g. `-Users-nil-DiskD-W-Djarvur-yafar`), and their JSONL files use a `queue-operation` / claude-code transcript schema (first line observed: `{"type": "queue-operation", ...}`). That is a **different product's** transcript.

**zcode writes its transcripts here:**

```
~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl    # per-session model I/O (the gold)
~/.zcode/cli/rollout/model-io-no-session.jsonl           # pre-session / no-session calls
~/.zcode/cli/log/zcode-<YYYY-MM-DD>.jsonl                # daily structured logs (NOT transcripts)
~/.zcode/cli/agents/sess_<session-id>/                   # per-session agent state (NOT model I/O)
~/.zcode/cli/exec/sess_<session-id>/                     # per-session exec/tool-call state
~/.zcode/cli/artifacts/sess_<session-id>/                # per-session artifacts
```

**Only `rollout/model-io-sess_*.jsonl` carries the request/response payloads** Phase 1 needs. The `agents/`, `exec/`, and `artifacts/` trees are per-session side state (tool-call dispatch, exec transcripts, file artifacts) — useful for PROF-05 coverage but not the mimicry ground truth.

**Munged-cwd encoding rule is OBSOLETE for zcode.** zcode does not key transcripts by cwd at all — it keys by `session-id` (a UUID). The `<munged-cwd>` → `<-replaced-cwd>` convention is a Claude-Code artifact and does not apply to zcode's layout.

### Per-line schema (the load-bearing finding)

Each line in `rollout/model-io-sess_*.jsonl` is one JSON object with `type: "model_io"` — one line per model round-trip. Top-level keys:

| Key | Type | Carries |
|---|---|---|
| `type` | `"model_io"` | discriminator |
| `model` | object | `{modelId, providerId, role, source, variant}` — e.g. `{modelId: "GLM-5.2", providerId: "builtin:zai-coding-plan", role: "lite", source: "config", variant: "max"}`. **This is the identity/tier metadata.** |
| `request` | object | **The full outgoing wire request** (see below) — this is the mimicry target. |
| `response` | object | **The full model response** (see below). |
| `sessionId` | string (uuid) | session id |
| `turnId` | string | turn id |
| `requestId`, `traceId` | string | correlation ids |
| `startedAt`, `completedAt` | ISO timestamp | timing |
| `durationMs` | int | latency |
| `attempt` | int | retry attempt number |
| `querySource` | string | origin of the query |

**`request` sub-object (the mimicry payload — Anthropic-shape):**

| Key | Type | Carries |
|---|---|---|
| `body` | object | **The HTTP request body.** Keys: `model`, `max_tokens`, `metadata`, `system` (array of `{type, text, cache_control?}` — the **system prompt**), and (when tools are declared) `tools` (array of `{name, description, input_schema}` — **Anthropic-shape tool catalog**, 77 tools observed) and `messages`. |
| `headers` | object | **The HTTP headers** — identity fields: `User-Agent`, `HTTP-Referer`, `X-Title`, `X-ZCode-App-Version`, `X-ZCode-Agent`, `X-Platform`, `X-Os-Category`, `X-Os-Version`, `x-request-id`, `x-zcode-trace-id`, `x-query-id`, `x-session-id`. **This is the identity/header field set PROF-05 needs to cover.** |
| `messages` | array | the conversation messages (role/content) — note `messages[0].role` can be `system` too |
| `maxOutputTokens` | int | e.g. 32000 |
| `providerOptions` | object | provider-specific knobs, e.g. `{anthropic: {thinking: ...}}` |
| `toolNames` | array | names of tools declared (parallel to `body.tools`) |
| `messageCount`, `messagesKind`, `messageOffset` | int/str | projection-window metadata (`messagesKind: "full"` vs presumably `"windowed"`) |

**`response` sub-object:**

| Key | Type | Carries |
|---|---|---|
| `finishReason` | string | `"stop"` / `"tool-calls"` / `"max_tokens"` etc. |
| `text` | string | the assistant's text output |
| `toolCalls` | array | **Anthropic-shape** `{id, name, input}` — e.g. `{id: "<str>", name: "Skill", input: {skill, args}}`. (Note: zcode normalizes to `{id, name, input}`, not the raw `tool_use` block shape — this is zcode's internal representation.) |
| `usage` | object | `{inputTokens, outputTokens, totalTokens, cacheReadTokens, cacheWriteTokens}` |
| `headers` | object | response HTTP headers (incl. `x-log-id`, `x-process-time`) |
| `modelId`, `responseId`, `providerMetadata` | string/object | model/response metadata |

### Redacted sample (D-03 compliant) — what the executor embeds in VERIFIED-FACTS.md

```json
{
  "type": "model_io",
  "model": {
    "modelId": "GLM-5.2",
    "providerId": "builtin:zai-coding-plan",
    "role": "lite",
    "source": "config",
    "variant": "max"
  },
  "sessionId": "[REDACTED: uuid]",
  "turnId": "[REDACTED: str]",
  "request": {
    "body": {
      "model": "GLM-5.2",
      "max_tokens": 32000,
      "metadata": { "user_id": "[REDACTED]" },
      "system": [
        { "type": "text", "text": "[REDACTED: system prompt, ~997 chars]", "cache_control": "[REDACTED if present]" }
      ],
      "tools": [
        { "name": "[REDACTED: tool name]", "description": "[REDACTED]", "input_schema": "[REDACTED: JSON schema]" }
      ],
      "messages": [
        { "role": "user", "content": "[REDACTED: prompt content]" }
      ]
    },
    "headers": {
      "User-Agent": "[REDACTED]",
      "HTTP-Referer": "[REDACTED]",
      "X-Title": "[REDACTED]",
      "X-ZCode-App-Version": "[REDACTED]",
      "X-ZCode-Agent": "[REDACTED]",
      "X-Platform": "[REDACTED]",
      "X-Os-Category": "[REDACTED]",
      "X-Os-Version": "[REDACTED]",
      "x-request-id": "[REDACTED]",
      "x-zcode-trace-id": "[REDACTED]",
      "x-query-id": "[REDACTED]",
      "x-session-id": "[REDACTED]"
    },
    "maxOutputTokens": 32000,
    "providerOptions": { "anthropic": { "thinking": "[REDACTED]" } },
    "toolNames": ["[REDACTED: ...77 names]"],
    "messageCount": 2,
    "messagesKind": "full",
    "messageOffset": 0
  },
  "response": {
    "finishReason": "tool-calls",
    "text": "[REDACTED]",
    "toolCalls": [
      { "id": "[REDACTED]", "name": "Skill", "input": { "skill": "[REDACTED]", "args": "[REDACTED]" } }
    ],
    "usage": {
      "inputTokens": "[REDACTED: int]",
      "outputTokens": "[REDACTED: int]",
      "totalTokens": "[REDACTED: int]",
      "cacheReadTokens": "[REDACTED: int]",
      "cacheWriteTokens": "[REDACTED: int]"
    },
    "headers": { "[REDACTED: response headers]" },
    "modelId": "glm-5.2",
    "responseId": "[REDACTED: msg_id]",
    "providerMetadata": { "anthropic": "[REDACTED]" }
  }
}
```

**Sanitization checklist the executor applies (D-03):**
- Replace every prompt content / system prompt text → `[REDACTED: ...]` with a length hint.
- Replace every `*_id`, `traceId`, `sessionId`, `turnId` value → `[REDACTED: uuid]` / `[REDACTED: str]`.
- Replace every token count value → `[REDACTED: int]` (keep the key — Phase 1 needs to know the field exists).
- Replace every header value → `[REDACTED]` (keep the header **name** — the set of header names is itself the identity fingerprint Phase 1 must mimic).
- Replace every tool name/description/schema → `[REDACTED]` (keep the count, e.g. "77 tools declared").
- Note in `Evidence`: "1 representative line; prompt content, ids, token counts, and tool schemas redacted; header names and structural keys preserved as they ARE the mimicry target."

### Which fields carry what (the Phase 1 lookup table)

| Phase 1 needs | Look in |
|---|---|
| **System prompt composition** (PROF-03) | `request.body.system[]` (ordered array of `{type, text, cache_control}` blocks) |
| **Tool catalog declaration** (TOOL-01/02) | `request.body.tools[]` (`{name, description, input_schema}` — Anthropic-shape) |
| **Identity / significant headers** (PROF-05) | `request.headers` (the 12 header names above) |
| **Message block shape** | `request.body.messages[]` and `request.messages[]` |
| **Model / tier mapping** | `model.{modelId, providerId, role, variant}` |
| **Provider-specific options** | `request.providerOptions` (e.g. `anthropic.thinking`) |
| **Tool-call response shape** (PROV-02 conformance) | `response.toolCalls[]` (`{id, name, input}`) |
| **Usage / token accounting** | `response.usage` |
| **`target_capture_ref`** (PROF-03) | the `sessionId` + `~/.zcode/cli/rollout/model-io-sess_<sessionId>.jsonl` path |

> **Crucial implication for the mimicry thesis:** the `request.body` is **Anthropic-shape** (`system`, `tools` with `input_schema`, `max_tokens`). zcode's `providerId: "builtin:zai-coding-plan"` confirms it talks to Z.ai's GLM via the Anthropic protocol (matches STACK's Z.ai finding). So the zcode profile's `message_shape.provider` is **`anthropic`**, not `openai`. Phase 1's zcode profile extraction reads these rollout files directly — no MITM proxy needed for the body (only for any header NOT already captured here, of which there appear to be none — the rollout captures the full header set).

---

## 4. OpenAI-shape Tool-calling Schema (for the #2 spike)

**Important clarification for the executor:** the OpenAI docs page (`/api/docs/guides/function-calling`) now leads with the **Responses API** shape (`output[].function_call`, `function_call_output`), which differs from the **Chat Completions** shape. The `sashabaranov/go-openai` client targets **Chat Completions**, so the spike must assert the Chat Completions schema. The two shapes:

### Chat Completions (what `go-openai` uses — load-bearing for the spike)

**Request `tools[]` element:**
```json
{
  "type": "function",
  "function": {
    "name": "get_weather",
    "description": "Retrieves current weather for the given location.",
    "parameters": {
      "type": "object",
      "properties": { "location": { "type": "string" } },
      "required": ["location"]
    }
  }
}
```
(Note the `function` wrapper — distinct from Anthropic's flat `{name, description, input_schema}`. The spike should confirm `go-openai`'s `Tool`/`FunctionDefinition` structs serialize to exactly this.)

**Assistant response `tool_calls[]`:**
```json
{
  "role": "assistant",
  "tool_calls": [
    {
      "id": "call_abc123",
      "type": "function",
      "function": {
        "name": "get_weather",
        "arguments": "{\"location\":\"Paris\"}"
      }
    }
  ]
}
```
(`arguments` is a **JSON-encoded string**, not an object — a common provider divergence point. The spike should assert it can round-trip a non-trivial nested-object argument.)

**`tool_choice`:**
- `"auto"` (default) — model decides
- `"none"` — never call
- `"required"` — must call at least one
- `{"type": "function", "function": {"name": "get_weather"}}` — force a specific function

**Tool-result message (role: `tool`):**
```json
{ "role": "tool", "tool_call_id": "call_abc123", "content": "{\"temperature\": 25}" }
```
(`content` is a string, typically JSON-encoded. `tool_call_id` correlates to the call.)

### Responses API (newer — DO NOT use for the spike, but recognize it)

The docs show `output[].function_call` (`{id, call_id, type, name, arguments}`) and the return item `function_call_output` (`{type, call_id, output}`). This is the **Responses API**, not Chat Completions. **The spike must NOT assert this shape** against `go-openai` (which is Chat-Completions-only). If a provider's docs show this shape, the spike is hitting the wrong endpoint.

### Provider-specific quirks to exercise in the spike

- **MiniMax M3** (OpenAI-compatible): base URL per MiniMax docs; model slug per their catalog. Known divergence points to assert: (a) does it accept `tools` in the same shape? (b) does `tool_choice: "required"` work? (c) is `arguments` returned as a JSON string (OpenAI-spec) or as a parsed object (some clones)?
- **Groq** (OpenAI-compatible): base URL `https://api.groq.com/openai/v1`. Groq is generally a close OpenAI clone; assert the same three points.

**What the #2 spike does (planner-ready):**
1. Build a `go-openai` client with a swapped base URL (configurable via env var: `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL`).
2. Declare a minimal tool (`get_weather` with one string param).
3. Send a chat request that forces a tool call (`tool_choice: "required"` or a prompt that obviously needs the tool).
4. Assert the response's `tool_calls[0].function.arguments` is a JSON string parseable into the declared schema.
5. Send a follow-up turn with the `tool`-role result message.
6. Assert the model uses the tool result in its reply.
7. Print the raw request JSON, raw response JSON, and pass/fail per assertion to **stderr**.
8. Repeat for MiniMax M3 and Groq (skipping any provider whose creds aren't available — record as PARTIAL with reason).

The spike asserts the **round-trip works** and the **schema matches OpenAI Chat Completions spec**. Provider quirks get recorded in `Notes`.

---

## 5. `go-telegram/bot` API Surface (for the #5 stdout-collision spike)

**Construction + handler + start (confirmed against README):**
```go
b, err := bot.New("BOT_TOKEN")  // or bot.New(token, opts...)
b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, myHandler)
b.Start(ctx)  // blocks; long-poll loop runs here. ctx cancellation stops it.
```
`b.Start(ctx)` blocks on the long-poll loop. To run it in a goroutine alongside an ACP server: `go b.Start(ctx)` — and the ctx cancel drains the loop (the library is `context.Context`-first, which is exactly why STACK picked it). **The planner should have the spike launch `go b.Start(telegramCtx)` in a goroutine and run the ACP handshake on the main goroutine, then cancel `telegramCtx` on shutdown and assert the goroutine exits.**

**Logging / stdout behavior — the load-bearing question for spike #5:**
- **`WithDebug()`** — enables debug mode. Off by default.
- **`WithDebugHandler(func(ctx, message, err, data))`** — routes debug events to your callback. **You** control where (stdout/stderr/file). Type: `type DebugHandler func(ctx context.Context, message string, err error, additionalData any)`.
- **`WithErrorsHandler(func(ctx, err))`** — routes internal errors to your callback. Type: `type ErrorsHandler func(ctx context.Context, err error)`.
- **There is no `WithLogger` and no `io.Writer` option** — all output is via callbacks.
- **No unconditional `fmt.Println` / `os.Stdout` write was found** in the API surface; the library routes everything through the two handlers above. **When `WithDebug()` is NOT set (the default), the library is silent.** This is exactly what spike #5 needs: by default the bot writes nothing to stdout, so the spike can assert "no non-ACP frame on stdout" trivially, then additionally assert that even with `WithDebug()` on, routing the handler to stderr keeps stdout clean.

**Spike #5 discipline:** the spike MUST register `WithErrorsHandler` routing to `slog` (→ stderr) and must NOT register `WithDebugHandler` writing to stdout. The assertion: a tee of stdout is byte-equal to the ACP frames the test itself emitted, with zero extraneous bytes. The Telegram goroutine must not leak any write.

**Cancellation/shutdown:** `signal.NotifyContext(context.Background(), os.Interrupt)` is the README pattern. For the spike, build a `context.WithCancel`, start the bot goroutine, run a canned ACP handshake on stdout, cancel, assert the goroutine exited (`sync.WaitGroup` or a done channel) and stdout stayed clean.

---

## 6. whisper.cpp / STT Cross-compile (confirming D-06)

**D-06 is structurally sound. Reasoning holds:**

- If STT is the **OpenAI Whisper API** (v1 default) → it's an HTTPS call via `go-openai`. Zero cgo. Zero cross-compile impact.
- If STT is **whisper.cpp local** → D-06 mandates it runs as an **out-of-process subprocess** (`whisper-cli`), not via the `ggml-org/whisper.cpp/bindings/go` cgo binding. The Go binary `exec.Command`s the `whisper-cli` binary the user installs; the binary itself is built separately (with its own C toolchain) and is **not** part of ass-guard's goreleaser matrix.
- If STT is **Groq STT** → same as OpenAI API shape, an HTTPS call. Zero cgo.

**Consequence:** ass-guard's goreleaser matrix (macOS+Linux amd64+arm64, pure Go, CGO_ENABLED=0) is **unaffected** by STT choice under D-06. STACK item #4's premise (cgo cross-compile risk) does not arise. **Status: `STRUCTURALLY-MOOT`.** No spike needed; the closure is a one-paragraph reasoning confirmation written directly into VERIFIED-FACTS.md (the executor cites D-06 + this section).

**Optional micro-confirmation for the executor (cheap, increases confidence):** write a 5-line Go program that calls `exec.LookPath("whisper-cli")` and prints whether the binary is discoverable — proves the subprocess model is viable without bundling. Not required by D-04 (item #4 = no spike); the planner can include it as optional belt-and-suspenders or omit.

---

## 7. Spike Structure Guidance (for the planner)

### Recommended directory layout

```
spikes/
├── README.md                      # how to run each spike; the canonical entrypoint doc
├── go.mod                         # module github.com/djarvur/ass-guard-spikes (isolated, D-05)
├── go.sum                         # GITIGNORED (D-05)
├── 01-jsonl-capture/              # item #1 — NOT a Go program; a capture/notes subpackage
│   └── README.md                  # documents the path + schema discovery (findings feed VERIFIED-FACTS directly)
├── 02-openai-toolschema/          # item #2 — main.go: tool-call round-trip vs MiniMax M3 + Groq
│   └── main.go
├── 03-acp-handshake/              # item #3 — main.go: initialize/session/new/session/prompt + observe session/update
│   └── main.go
└── 05-stdout-collision/           # item #5 — main.go: go-telegram/bot goroutine + ACP frames on stdout, assert clean
    └── main.go
```

**Why subdirectories with one `main.go` each (NOT a single multi-command binary):** each spike proves a different fact, runs independently, has different env-var requirements (API keys, bot tokens, ACP peer), and dies on its own. A single binary with subcommands would couple them and complicate the assertion that "stdout was clean" for spike #5. Each subdirectory is its own `package main` — Go supports multiple `main` packages in one module.

**Item #1 has NO `main.go`.** Per CONTEXT.md, #1 is a **filesystem capture**, not Go code. The `01-jsonl-capture/` dir holds a `README.md` documenting the path discovery + schema (the §3 findings above), and the executor copies a sanitized sample line into VERIFIED-FACTS.md. (Optional: a small `inspect.go` helper that prints the schema of a line — but it's not required; `python3 -m json.tool` / `jq` suffices and keeps #1 dependency-free.)

**Item #4 has NO spike directory.** It's `STRUCTURALLY-MOOT` (D-06). Optionally a note in `spikes/README.md` pointing at this research doc's §6.

### `spikes/go.mod` (exact content the executor creates)

```
module github.com/djarvur/ass-guard-spikes

go 1.25

require (
	github.com/go-telegram/bot v1.23.0
	github.com/sashabaranov/go-openai v1.42.0
)
```
- `anthropic-sdk-go` is **NOT needed** for any spike (item #2 is OpenAI-shape only; item #3 is raw JSON-RPC, hand-rolled; item #5 is telegram+stdout). Keep the dep surface minimal — D-05's intent is "throwaway, not the codebase."
- If the #3 spike wants to talk to an Anthropic-protocol ACP peer and the executor finds raw `net/http` painful, they may add `anthropics/anthropic-sdk-go@v1.62.0` — but the planner should default to hand-rolled HTTP (matches the project's framing decision and keeps the spike honest about ACP wire shape).

### `spikes/README.md` content (committed, per D-05)

For each spike: one-time setup (env vars), how to run (`cd spikes/02-openai-toolschema && go run .`), what it asserts, where the output goes (stderr), and how to capture the evidence into VERIFIED-FACTS.md. Explicit warning: "These spikes are THROWAWAY. They are not the ass-guard codebase. Do not import from them in Phase 1."

### Per-spike behavior

| Spike | Does | Asserts | Prints to stderr | Captures to VERIFIED-FACTS # |
|---|---|---|---|---|
| **01-jsonl-capture** (no Go) | Locates `~/.zcode/cli/rollout/*.jsonl`, extracts one `model_io` line, redacts | Path exists; schema has `request.body.{system,tools}`, `request.headers`, `response.toolCalls` | n/a (filesystem only) | #1 |
| **02-openai-toolschema** | Tool-call round-trip vs MiniMax M3 + Groq via `go-openai` | (a) `tools[]` serialized to OpenAI shape; (b) response `tool_calls[].function.arguments` is a JSON string; (c) follow-up `tool`-role message consumed | raw request JSON, raw response JSON, per-assertion PASS/FAIL, provider quirks | #2 |
| **03-acp-handshake** | `initialize` → `session/new` → `session/prompt`; read `session/update` frames | (a) `initialize` returns `protocolVersion` + `agentCapabilities`; (b) `session/new` returns a `sessionId`; (c) at least one `session/update` frame arrives before the prompt response | each frame sent (method + redacted params), each frame received, PASS/FAIL | #3 |
| **05-stdout-collision** | `go-telegram/bot` goroutine + canned ACP frames on stdout; ctx cancel drains bot | (a) stdout byte-equals the canned ACP frames (zero extra bytes from the bot); (b) bot goroutine exits within e.g. 2s of ctx cancel | the canned frames, the bot's exit timing, PASS/FAIL | #5 |

**Output discipline (load-bearing):** every spike prints ALL diagnostic output to **stderr** (`log.Printf`, `fmt.Fprintln(os.Stderr, ...)`). stdout is reserved for the thing being tested (ACP frames in #3 and #5; nothing in #2). This models the project's transport discipline from day one and is itself part of what #5 asserts.

---

## 8. VERIFIED-FACTS.md Authoring Guidance

### Section template (per D-02 field schema) — the executor fills one per item

```markdown
## #N. <Fact title>

- **Fact:** <the claim being verified — quote STACK.md verbatim>
- **Source:** STACK.md §"Open Verification Items" item #N (line <NNNN>); <secondary source URL if any>
- **Verified:** 2026-08-09
- **Verified against:** <exact version / commit / tag / "on-disk file at <path>">
- **Status:** VERIFIED | FAILED | PARTIAL | STRUCTURALLY-MOOT
- **Evidence:** <5–10 representative lines as a fenced code block, sanitized per D-03; OR a one-paragraph reasoning chain for STRUCTURALLY-MOOT items; include the redaction note: "N representative lines; <what was redacted>">
- **Notes:** <drift from STACK, provider quirks, downstream implications, anything Phase 1's researcher must know>
```

### Sanitization checklist (D-03) — executor applies before commit

For every embedded sample (JSONL line, ACP frame, HTTP request/response):
1. **Prompt content / system prompt text** → `[REDACTED: <length hint>]`.
2. **IDs** (session, trace, request, turn, message, call) → `[REDACTED: uuid]` / `[REDACTED: str]`.
3. **Token counts / usage numbers** → `[REDACTED: int]` (keep the key).
4. **API keys / auth tokens / bearer values** → `[REDACTED]` (never embed, even redacted-looking).
5. **File paths that reveal user/project structure** → `[REDACTED: <path>]`.
6. **Tool argument contents** → `[REDACTED]`.
7. **Preserve:** all JSON keys, all HTTP header **names**, all enum values (`finishReason`, `role`, `type`), structural shape, array lengths (as `[N items]`). These are the mimicry target and must survive redaction.
8. **Note in `Evidence`:** exactly what was redacted and why (e.g. "1 model_io line; prompt text, ids, token counts redacted; header names and tool catalog shape preserved as they ARE the mimicry target").

### Completeness gate (the planner's "phase is done" check)

VERIFIED-FACTS.md is complete iff:
- All 5 items have a section.
- Every section has all 7 D-02 fields populated (no `TBD`, no empty `Evidence`).
- Every `Status` is one of the 4 enum values.
- Item #4 is `STRUCTURALLY-MOOT` with the D-06 reasoning.
- At least items #1, #2, #3, #5 have either `VERIFIED`/`PARTIAL`/`FAILED` with concrete evidence (a spike ran OR a file was inspected).
- Any `FAILED` item has a corrected fact in `Notes` and (if Tier B) a flagged user-decision pointer.

---

## 9. Validation Architecture

This phase is a **verification phase**, so validation = "did we honestly close each fact," not "does a feature work." The validation architecture is severity-tiered (D-07) and binary per item.

### Per-spike binary pass/fail assertions (observable, repeatable)

| # | Assertion (binary) | PASS evidence | FAIL evidence |
|---|---|---|---|
| 1 | The on-disk transcript path exists AND a `model_io` line yields `request.body.system`, `request.body.tools`, `request.headers`, `response.toolCalls` | Path + redacted schema in VERIFIED-FACTS.md | Path absent → Tier B (architectural; surface to user). Schema fields absent → Tier B (the mimicry thesis needs them). |
| 2 | `go-openai` round-trips a tool call against at least one of {MiniMax M3, Groq}: `tool_choice:"required"` returns a `tool_calls[]` whose `function.arguments` is a JSON string parseable into the declared schema, AND the follow-up `tool`-role turn is consumed | Raw request + response JSON in VERIFIED-FACTS.md (redacted), per-provider PASS | Round-trip fails → Tier A (try the other provider; record PARTIAL). Both fail + schema diverges from OpenAI spec → Tier B (go-openai cannot represent it). |
| 3 | An ACP v1 `initialize` → `session/new` → `session/prompt` exchange returns: (a) `initialize` result with `protocolVersion` + `agentCapabilities`; (b) `session/new` result with a `sessionId`; (c) ≥1 `session/update` frame before the prompt response | Redacted frame dump in VERIFIED-FACTS.md | A method name is wrong/missing → Tier A (record corrected name, continue). A load-bearing method absent from v1 → Tier B (architecture assumed it). |
| 4 | (Structural) D-06 reasoning is sound: STT is never cgo-bundled | One-paragraph reasoning in VERIFIED-FACTS.md citing D-06 | n/a — closed structurally; no runtime assertion |
| 5 | With `go-telegram/bot` in a goroutine and canned ACP frames written to stdout, stdout is byte-equal to the canned frames (zero non-ACP bytes); bot goroutine exits within 2s of ctx cancel | stdout hexdiff / byte count in VERIFIED-FACTS.md | stdout has extra bytes → Tier A (route the bot's debug/errors handler to stderr; the spike's whole point is to prove this is fixable). Bot doesn't exit on cancel → Tier A (library contract bug; record and pick mitigation). |

### VERIFIED-FACTS.md completeness check (testable)

A tiny grep-based check the executor runs before declaring the phase done:
- `grep -c '^## #' VERIFIED-FACTS.md` ≥ 5 (five sections).
- `grep -c '^\- \*\*Status:\*\*' VERIFIED-FACTS.md` ≥ 5, each matching `VERIFIED|FAILED|PARTIAL|STRUCTURALLY-MOOT`.
- `grep -c '^\- \*\*Evidence:\*\*' VERIFIED-FACTS.md` ≥ 5, none followed by an empty line.
- No occurrence of `TBD`, `TODO`, `<empty>`, or `[fill in]`.

(These are illustrative; the planner can codify them as a single shell one-liner the executor runs.)

### Negative-finding protocol (D-07) — testable behavior

- **Tier A (doc/minor):** the executor records `Status: FAILED` **+ a corrected `Fact`** in `Notes` **+ continues** to the next item. The phase does not stop. (Example: `protocolVersion` is a string not an int; record, continue.)
- **Tier B (architectural):** the executor records `Status: FAILED` + the finding, **halts**, and uses the `AskUserQuestion`-equivalent to surface three options: (a) revise-and-continue, (b) halt-and-replan, (c) accept-and-document. (Example: zcode's rollout schema lacked `request.body.system` — would mean the system prompt isn't captured; that would break the mimicry thesis.)

> **Current research predicts ZERO Tier B failures.** The §3 finding (path wrong) is Tier B by *category* (a load-bearing STACK premise is wrong), but its *substance* is favorable (better data available than expected) → resolution is **(a) revise-and-continue**: record the corrected path/schema, flag the MIMC-02 wording change, continue. The planner should still surface it explicitly so the user signs off on the MIMC-02 path correction before Phase 1.

### How spikes print results (so the executor can capture into VERIFIED-FACTS.md)

Every spike prints a **structured, parseable footer to stderr** at exit:
```
=== SPIKE 0N RESULT ===
assertion_1: PASS
assertion_2: PASS
...
evidence_path: /tmp/spike-0N-output.json   # optional, if a dump was written
raw_redacted_excerpt: |
  <5-10 line sanitized sample for VERIFIED-FACTS.md>
=== END ===
```
The executor copies the `raw_redacted_excerpt` block into VERIFIED-FACTS.md's `Evidence` field (after re-checking the sanitization checklist). The PASS/FAIL lines drive the `Status` field. Structured format = the planner can later add a script that scrapes spike output and auto-checks the VERIFIED-FACTS.md entries match.

---

## RESEARCH COMPLETE

**Summary of findings:**

1. **Version pins confirmed** (2026-08-09): `go-openai@v1.42.0`, `go-telegram/bot@v1.23.0`, `anthropic-sdk-go@v1.62.0`. ACP v1 stable / v2 Draft confirmed. All Tier A or none.

2. **ACP v1 wire specifics captured:** newline-delimited JSON framing; `initialize`/`session/new`/`session/prompt`/`session/update`/`session/load` confirmed; the brief's missing `session/new` step is a Tier A correction; `protocolVersion` is schema-string but example-integer (spike should test both); result field is `agentCapabilities` (not `capabilities`/`serverInfo`).

3. **MAJOR FINDING (item #1, Tier B → revise-and-continue):** STACK's zcode JSONL path `~/.claude/projects/<munged-cwd>/<session-id>.jsonl` is **wrong for zcode**. The actual path is `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl` with a `type: "model_io"` schema capturing **full wire-level request/response** (body + headers + system + tools + response.toolCalls + usage). This is **higher fidelity** than STACK projected — Phase 1 needs no MITM proxy for the request body. The munged-cwd convention does not apply (zcode keys by session-id). MIMC-02's path wording must be corrected; the planner must surface this for user sign-off. Schema fully documented in §3 with a redacted sample.

4. **OpenAI tool-calling schema captured** (Chat Completions shape, distinct from the newer Responses API the docs now lead with — the spike must assert Chat Completions). Provider-quirk checklist (MiniMax M3, Groq) defined.

5. **`go-telegram/bot` logging behavior confirmed:** silent by default; debug/errors route through `WithDebugHandler`/`WithErrorsHandler` callbacks (no `WithLogger`, no `io.Writer`, no unconditional stdout writes). Spike #5's assertion ("stdout stays clean") is achievable by routing handlers to `slog`→stderr. Context-first API confirmed for clean shutdown.

6. **D-06 confirmed sound:** STT always external (API HTTPS call or `whisper-cli` subprocess) → zero cgo → goreleaser matrix unaffected. Item #4 = `STRUCTURALLY-MOOT`, no spike.

7. **Spike structure recommended:** `spikes/{01-jsonl-capture,02-openai-toolschema,03-acp-handshake,05-stdout-collision}/` with isolated `go.mod` (`github.com/djarvur/ass-guard-spikes`, `go 1.25`, deps `go-telegram/bot@v1.23.0` + `go-openai@v1.42.0`). Item #1 = filesystem capture (no Go); item #4 = no spike.

8. **VERIFIED-FACTS.md template + sanitization checklist + completeness gate** defined for the planner.

9. **Validation Architecture defined:** binary per-spike assertions, VERIFIED-FACTS.md completeness grep-checks, D-07 tiered negative-finding protocol (Tier A → continue, Tier B → halt + 3 options). Predicted Tier B failures: zero (the §3 finding is favorable-direction revise-and-continue).

**Drift from STACK.md discovered:**
- **Item #1:** path AND schema wrong (Tier B-favorable; detailed in §3 + HEADLINE FINDING). Requires MIMC-02 wording update + user sign-off.
- **Item #2:** `go-openai` latest tag is concretely `v1.42.0` (STACK said "latest 1.x" — Tier A, just needs pinning).
- **Item #3:** ACP lifecycle includes `session/new` between `initialize` and `session/prompt` (STACK omitted it — Tier A doc/minor).
- **Item #4:** unchanged (D-06 closes it).
- **Item #5:** unchanged; library contract confirmed suitable.

**No blockers. Phase 0 is ready to plan.**
