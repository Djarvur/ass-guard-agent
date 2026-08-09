# Spike 01 — zcode JSONL transcript capture (item #1, no Go)

> **This is a filesystem capture, not a Go program.** Per CONTEXT.md per-item
> assignment and RESEARCH.md §7, item #1 has **no `main.go`** — `jq` and
> `python3 -m json.tool` suffice to inspect the on-disk transcripts. Keeping #1
> dependency-free is deliberate: the discovery is "where does zcode write
> transcripts and what is their shape," which is a read-only filesystem question.

## HEADLINE: STACK's path is wrong for zcode (Tier-B-favorable, revise-and-continue)

STACK.md §"Open Verification Items" item #1 (line 488) and the mimicry-capture
row (~line 47) claim zcode's transcripts live at:

```
~/.claude/projects/<munged-cwd>/<session-id>.jsonl
```

**That path exists on this machine, but it is a DIFFERENT PRODUCT's transcript
(Claude Code / claude-cli), not zcode's.** Its JSONL files are keyed by
munged-cwd and use a `queue-operation` schema. zcode writes its model I/O
elsewhere.

**The correct path (verified 2026-08-09 on this machine):**

```
~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl
```

This is a **Tier-B finding by category** (a load-bearing STACK premise — the path
— is wrong), but its substance is **favorable** (D-07 option (a)
revise-and-continue): the capability (extract the zcode profile from on-disk
logs) is fully intact and in fact **easier** than STACK projected — no MITM proxy
is needed for the request body, because zcode logs the **full wire-level
request/response** (body + headers + system + tools + response.toolCalls +
usage), strictly higher fidelity than the message-level transcripts STACK
described. Explicit user sign-off on this path correction lands in Plan 00-05
(it propagates to MIMC-02's wording, ROADMAP, and any downstream reference to
`~/.claude/projects/`).

## Proof that `~/.claude/projects/` is a different product

```
~/.claude/projects/                     # PRESENT on this machine
├── -Users-nil--claude-code-router/      # keyed by munged-cwd (the OBSOLETE convention)
├── -Users-nil-DiskD-W-Djarvur-AI/
├── -Users-nil-DiskD-W-Djarvur-repofix/
└── -Users-nil-DiskD-W-Djarvur-yafar/
    └── <session-id>.jsonl
        first line: {"type": "queue-operation", "operation": ..., "sessionId": ..., "timestamp": ...}
        top-level keys: {operation, sessionId, timestamp, type}
```

That is **Claude Code's** transcript schema (`queue-operation` discriminator,
operation/timestamp/sessionId fields). It is NOT zcode's model I/O capture.
STACK conflated the two because both products are "claude-code-compat
runtimes" — but their on-disk layouts and schemas are entirely different.

## The zcode rollout layout (verified 2026-08-09)

```
~/.zcode/cli/
├── rollout/
│   ├── model-io-sess_<session-id>.jsonl      # per-session model I/O (THE GOLD — request/response payloads)
│   ├── model-io-sess_subagent_agent_<id>.jsonl  # subagent sessions (same schema)
│   └── model-io-no-session.jsonl             # pre-session / no-session calls
├── log/
│   └── zcode-<YYYY-MM-DD>.jsonl              # daily structured logs (NOT transcripts)
├── agents/sess_<session-id>/                 # per-session agent state (NOT model I/O)
├── exec/sess_<session-id>/                   # per-session exec/tool-call state
└── artifacts/sess_<session-id>/              # per-session artifacts
```

**Only `rollout/model-io-sess_*.jsonl` carries the request/response payloads**
Phase 1 needs. The `agents/`, `exec/`, and `artifacts/` trees are per-session
side state (tool-call dispatch, exec transcripts, file artifacts) — useful for
PROF-05 coverage but not the mimicry ground truth.

**Munged-cwd encoding rule is OBSOLETE for zcode.** zcode does not key
transcripts by cwd at all — it keys by `session-id` (a UUID). The
`<munged-cwd>` → `<-replaced-cwd>` convention is a Claude-Code artifact and does
not apply to zcode's layout.

On this machine: 3 `model-io-sess_*.jsonl` files (2 main sessions + 1
subagent session), inspected the largest main-session file (48 lines, all
`type: "model_io"`).

## Per-line schema (the load-bearing finding)

Each line in `rollout/model-io-sess_*.jsonl` is one JSON object with
`type: "model_io"` — **one line per model round-trip.** Top-level keys (13,
verified):

| Key | Type | Carries |
|---|---|---|
| `type` | `"model_io"` | discriminator |
| `model` | object | `{modelId, providerId, role, source, variant}` — e.g. `{modelId: "GLM-5.2", providerId: "builtin:zai-coding-plan", role: "main", source: "config", variant: "max"}`. **The identity/tier metadata.** |
| `request` | object | **The full outgoing wire request** (see below) — this is the mimicry target. |
| `response` | object | **The full model response** (see below). |
| `sessionId` | string (uuid) | session id |
| `turnId` | string | turn id |
| `requestId`, `traceId` | string | correlation ids |
| `startedAt`, `completedAt` | ISO timestamp | timing |
| `durationMs` | int | latency |
| `attempt` | int | retry attempt number |
| `querySource` | string | origin of the query |

### `request` sub-object (the mimicry payload — Anthropic-shape)

Verified keys (9): `body`, `headers`, `maxOutputTokens`, `messageCount`,
`messageOffset`, `messages`, `messagesKind`, `providerOptions`, `toolNames`.

| Key | Type | Carries |
|---|---|---|
| `body` | object | **The HTTP request body** (Anthropic-shape). Verified keys (9): `max_tokens`, `metadata`, `model`, `output_config`, `stream`, `system`, `thinking`, `tool_choice`, `tools`. |
| `body.system` | array | `{type, text, cache_control}` blocks — the **system prompt composition** (3 blocks observed on the inspected line). |
| `body.tools` | array | `{name, description, input_schema}` — **Anthropic-shape tool catalog** (`input_schema` is a full JSON schema: `$schema, additionalProperties, properties, required, type`). **77 tools observed.** |
| `body.messages` | array | role/content messages (may be absent when `request.messages` carries them). |
| `body.thinking` | object | Anthropic extended-thinking knob (e.g. `{type, budget_tokens}`). |
| `body.tool_choice` | varies | Anthropic tool-choice directive. |
| `headers` | object | **The HTTP headers** — 12 identity header names (see table below). **This is the identity/header field set PROF-05 needs to cover.** |
| `messages` | array | the conversation messages (role/content); `messages[0].role` can be `system` too (39 observed on the inspected line). |
| `maxOutputTokens` | int | e.g. 32000 |
| `providerOptions` | object | provider-specific knobs (e.g. `{anthropic: {thinking: ...}}`). |
| `toolNames` | array | names of tools declared (parallel to `body.tools`). |
| `messageCount`, `messagesKind`, `messageOffset` | int/str | projection-window metadata (`messagesKind: "full"` vs presumably `"windowed"`). |

### `request.headers` — the 12 identity header names (the mimicry target, safe to name)

Verified exactly 12:

```
HTTP-Referer, User-Agent, X-Os-Category, X-Os-Version, X-Platform, X-Title,
X-ZCode-Agent, X-ZCode-App-Version, x-query-id, x-request-id, x-session-id,
x-zcode-trace-id
```

The **set of header names** is itself the identity fingerprint Phase 1 must
mimic (PROF-05). Header *values* are redacted in evidence (D-03).

### `response` sub-object

Verified keys (8): `finishReason`, `headers`, `modelId`, `providerMetadata`,
`responseId`, `text`, `toolCalls`, `usage`.

| Key | Type | Carries |
|---|---|---|
| `finishReason` | string | `"stop"` / `"tool-calls"` / `"max_tokens"` etc. |
| `text` | string | the assistant's text output |
| `toolCalls` | array | **Anthropic-normalized** `{id, name, input}` (NOT the raw `tool_use` block shape — this is zcode's internal representation; all three keys verified present). |
| `usage` | object | `{inputTokens, outputTokens, totalTokens, cacheReadTokens, cacheWriteTokens}` (5 token-count fields). |
| `headers` | object | response HTTP headers (incl. `x-log-id`, `x-process-time`). |
| `modelId`, `responseId`, `providerMetadata` | string/object | model/response metadata. |

## Which fields carry what (the Phase-1 lookup table)

| Phase 1 needs | Look in |
|---|---|
| **System prompt composition** (PROF-03) | `request.body.system[]` (ordered array of `{type, text, cache_control}` blocks) |
| **Tool catalog declaration** (TOOL-01/02) | `request.body.tools[]` (`{name, description, input_schema}` — Anthropic-shape, 77 tools observed) |
| **Identity / significant headers** (PROF-05) | `request.headers` (the 12 header names above) |
| **Message block shape** | `request.body.messages[]` and `request.messages[]` |
| **Model / tier mapping** | `model.{modelId, providerId, role, variant}` |
| **Provider-specific options** | `request.providerOptions` and `request.body.thinking` (Anthropic extended-thinking) |
| **Tool-call response shape** (PROV-02 conformance) | `response.toolCalls[]` (`{id, name, input}`) |
| **Usage / token accounting** | `response.usage` (5 fields) |
| **`target_capture_ref`** (PROF-03) | the `sessionId` + `~/.zcode/cli/rollout/model-io-sess_<sessionId>.jsonl` path |

### Crucial implication for the mimicry thesis

The `request.body` is **Anthropic-shape** (`system` array, `tools` with
`input_schema`, `max_tokens`, `thinking`, `tool_choice`). zcode's
`providerId: "builtin:zai-coding-plan"` confirms it talks to Z.ai's GLM via
the Anthropic protocol (matches STACK's Z.ai finding). So the zcode profile's
`message_shape.provider` is **`anthropic`**, not `openai`. Phase 1's zcode
profile extraction reads these rollout files directly — **no MITM proxy needed
for the request body** (the rollout captures the full body + the full header
set); the only residual MITM use would be to cross-check, not to discover.

## How the capture was done (reproducible, dev-time only)

```bash
ROLLDIR="$HOME/.zcode/cli/rollout"
TARGET=$(ls -S "$ROLLDIR"/model-io-sess_*.jsonl | grep -v subagent | head -1)

# top-level keys
head -1 "$TARGET" | jq -r 'keys[]'

# request.body + request.headers + response.toolCalls shape
head -1 "$TARGET" | jq -r '.request.body | keys[]'
head -1 "$TARGET" | jq -r '.request.headers | keys[]'      # the 12 identity header NAMES
head -1 "$TARGET" | jq -r '.response.toolCalls[0] | keys[]' # {id, name, input}
head -1 "$TARGET" | jq -r '.request.body.tools | length'    # 77

# confirm ~/.claude/projects is a different product
head -1 "$(find ~/.claude/projects/ -name '*.jsonl' | head -1)" | jq -r '.type'  # "queue-operation"
```

All diagnostic output above is **dev-time only** and is NOT committed. The only
thing committed is the redacted excerpt in `FINDING.md` (sanitized per D-03).
