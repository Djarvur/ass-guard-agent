# Phase 2: Session Core + ACP Interface — Research

**Researched:** 2026-08-09
**Status:** Ready for planning
**Researcher model:** sonnet (inline; gsd-phase-researcher contract)
**Primary inputs:** `02-CONTEXT.md` (D-01..D-20, zero discretion), `.planning/research/VERIFIED-FACTS.md` (#1 zcode JSONL, #3 ACP v1 wire), `.planning/phases/01-mimicry-mvp-north-star-proof/01-{CONTEXT,RESEARCH,PATTERNS}.md` + the 6 plans, `.planning/REQUIREMENTS.md`, `.planning/ROADMAP.md`, `.planning/PROJECT.md`, canonical ACP v1 spec pages (fetched 2026-08-09), Phase-1 codebase (only `doc.go` skeletons exist today — Phase 2 plans against Phase-1's *intended* outputs since Phase 2 cannot execute until Phase 1 ships).

> This research answers "what do I need to know to PLAN Phase 2 well?" It does NOT relitigate CONTEXT.md locked decisions (D-01..D-20 — every one user-selected). It resolves the open RESEARCH-NOTE items the decisions explicitly deferred (channel buffer sizes D-05; task-summary extraction rules D-02; subagent event-vs-window boundary D-11; provider semaphore default D-12), gives the planner concrete identifiers, type names, file layouts, and the canonical ACP v1 wire shapes. Every claim is grounded in a dated, inspectable source.

---

<user_constraints>

## User Constraints (from CONTEXT.md — copied verbatim, NON-NEGOTIABLE)

### Locked Decisions (D-01..D-20 — every one user-selected; zero Claude's-discretion)

**Context projection & lean seed (SESS-01/04/06):**
- **D-01:** The lean seed (post-boundary-reset model view) = system prompt (profile TIER-1 faithful) + one-paragraph task summary + active workspace context the new command references + ZERO carry-forward of prior turn messages. Hard cut; summary is the bridge.
- **D-02:** Task summary = MECHANICAL extraction from the durable transcript (NOT a model call). Extraction rule deferred to research: last user message + files touched (from tool calls) + last assistant message truncated to N chars. No model call → no latency, no mimicry divergence.
- **D-03:** Durable transcript = append-only JSONL; every turn (user msg, assistant response, tool call, tool result, boundary marker) appended as a structured JSON line. Single source of truth; projector reads it to build the lean window. PRIMARY PURPOSE: human investigation.

**Streaming path & backpressure (ACP-04):**
- **D-04:** Event bus = typed Go channels — one channel per event type (`RequestShaped`, `AgentMessageChunk`, `ToolCall`, `ToolCallUpdate`, `UsageUpdate`, `SubagentResult`). Producers send; consumers select. Spine of streaming + audit + future subagents.
- **D-05:** Backpressure = bounded buffer + block. Each channel buffered to N (64–128); if full, producer blocks → slows provider SSE read. Research picks exact N per channel type.

**Session rehydration & boundaries (SESS-05):**
- **D-06:** Transcript lives per-project under `.ass-guard/` in cwd. Named by session-id + timestamp. Part of workspace.
- **D-07:** `.ass-guard/` is self-gitignoring: ass-guard creates `.ass-guard/.gitignore` (`*` + `!.gitignore`) on first run. Matches `.claude/` convention.
- **D-08:** Boundary markers = explicit JSONL lines: `{"type": "boundary", "cause": "mutating-command:Bash", ...}`. First-class transcript entries, written by Session Manager when a mutating command completes.
- **D-09 (MAJOR SCOPE REDUCTION):** NO REPLAY IN v1. ACP-03 (session/load replay) DROPPED. Transcript primary purpose = human investigation. `session/load` is no-op or new-session fallback. SESS-05 rehydration part MOOT; boundary-marker durability (D-08) still needed for live projection.

**Subagent isolation & concurrency (PARA-01..04):**
- **D-10:** Profile declares full tool catalog; subagent restricts execution subset at RUNTIME (tool gets "not available" error, model adapts). Restriction is runtime, not catalog-shape difference.
- **D-11:** Subagent results = streamed intermediate progress. Subagent streams tool calls + partial outputs to event bus tagged with parent-turn-id. Parent observes + can forward to ACP. NOTE: "only results, not accumulated context" still applies to parent's lean window — research defines event-vs-window boundary.
- **D-12:** Outbound concurrency bounded by single provider-layer semaphore, combined parent + subagents. Configurable max (default 4–8). Research defines default + config surface.
- **D-13:** Panicking subagent = goroutine `recover()` → tool-error result. Each subagent goroutine wraps turn loop in recover. Panic → structured tool-error published to bus (parent-turn-id tagged). Process NEVER crashes.

**ACP server skeleton & framing (ACP-01/02/05):**
- **D-14 (ACP-05 resolved):** JSON-RPC framing = hand-rolled (~150 LOC). `bufio.Scanner` over stdin (newline-delimited); mutex-guarded writer to stdout; method→handler map. Notifications carry no `id`, no response. Zero deps.
- **D-15:** ACP server = single reader goroutine + per-prompt turn goroutine. `session/prompt` spawns concurrent turn goroutine (so `session/cancel` works mid-turn). Methods: `initialize`, `session/new`, `session/prompt`, `session/cancel`, `session/load` (no-op D-09), `logout`, `session/set_mode`.
- **D-16:** `session/cancel` = context cancellation + drain queued events. (1) abort in-flight provider HTTP request, (2) drain queued bus events (discard + "canceled" marker), (3) turn goroutine exits cleanly, (4) transcript records "canceled" boundary marker. Phase 2 provides MECHANISM; Phase 4 engine decides WHEN.

**Turn loop → Session Core integration (SESS-06):**
- **D-17:** Session Core owns transcript + projector + turn loop (ONE package, `internal/session`). Session Manager = sole transcript owner (append/read API). Turn loop is a method on Session Core. Shaper + adapters from Phase 1 passed in.
- **D-18:** Turn cycle: (1) project lean window, (2) shape via Shaper (emit `RequestShaped`), (3) send to provider, (4) stream chunks → emit `AgentMessageChunk`/`ToolCall`/`UsageUpdate`, (5) if tool_calls, execute them (STUBBED Phase 2; real exec Phase 4), append results, loop, (6) on `end_turn`, append assistant response, return.

**Mutability declaration interface (SESS-02/03):**
- **D-19:** Mutability = per-tool field in catalog (`Mutability: 'mutating' | 'read-only'`), resolved by SESS-03 formula at runtime: `mutating if (adapter-class(command) == mutating OR tool.Mutability == mutating) else read-only`. Built-in catalog seeds: Bash/Write/Edit = mutating; Read/Glob/Grep = read-only. Phase 4 OpenSpec adapter registers with own Mutability, NO engine change. Config can ADD boundaries, CANNOT flip declared-mutating → read-only.

**Audit log = transcript (LOG-01..04):**
- **D-20:** Audit log = transcript (ONE artifact). Per-session JSONL under `.ass-guard/` captures everything (user input, verbatim shaped requests, tool calls, results, engine decisions, boundary markers) with LOG-03 redaction per-line. Verbatim-request capture (LOG-01) = another line type. Rotation = per-session files. Reconstruction-sufficiency = transcript + profile. LOG-02 satisfied (transcript writer subscribes to bus async). One writer, one schema.

### Claude's Discretion
None — every Phase-2 decision was explicitly user-selected.

### Deferred Ideas (OUT OF SCOPE — Phase 4+)
- **ACP-03 session/load replay** (D-09) — dropped from v1.
- **Real tool execution** (TOOL-04/05) — Phase 4. Phase 2 keeps tools stubbed.
- **Unified engine / hooks / autocontinue** (ENG/HOOK/LRN) — Phase 4. Phase 2 provides `session/cancel` mechanism only.
- **OpenSpec hosting** (OPEN-01..03) — Phase 4. Phase 2 forward-designs the mutability interface.

</user_constraints>

<phase_requirements>

## Phase Requirements (18 REQ-IDs — ACP-03 noted DROPPED per D-09)

| ID | Description (from REQUIREMENTS.md) | Research Support |
|----|------------------------------------|------------------|
| SESS-01 | Two-layer context model: durable replayable transcript + lean projected window, reset at command boundaries | §3 transcript schema + projector; §4 lean window; D-03/D-20 (one artifact) |
| SESS-02 | Mutating toolkit commands are ALWAYS boundaries (config may only ADD) | §6 mutability formula; D-19 structural enforcement |
| SESS-03 | Effective mutability = more-mutating wins formula | §6 the formula + per-tool field |
| SESS-04 | Boundary resets window to lean seed (system + intent + zero carry-forward) | §4 lean seed composition; D-01/D-02 extraction rule |
| SESS-05 | Boundary markers durable in transcript; projector resets at most recent pre-rehydration boundary | §3.3 boundary line type; §5 (rehydration part MOOT per D-09; boundary durability LIVE) |
| SESS-06 | Session Manager sole transcript owner; append/read API | §3.4 the Manager API; D-17 one-package rule |
| ACP-01 | ACP v1 server over stdio JSON-RPC; stdout=frames, stderr=logs | §7 the framer; §8 server skeleton; D-14/D-15 |
| ACP-02 | Lifecycle methods (initialize, session/prompt, session/load) | §7/§8 method handlers; VERIFIED-FACTS #3 ground truth |
| ACP-03 | (DROPPED per D-09) session/load replays transcript | §5 — no-op/fallback; NOT implemented |
| ACP-04 | End-to-end streaming: provider SSE → bus → ACP session/update; no buffering | §2 typed channels; §8.3 the streaming adapter; D-04/D-05 |
| ACP-05 | JSON-RPC framing resolved | §7 hand-rolled ~150 LOC; D-14 |
| LOG-02 | Audit log = async bus consumer | §2.3 bus → transcript writer subscription; D-20 |
| LOG-03 | Sensitive data redacted from log | §3.5 per-line redaction; `internal/redact` from Phase 1 |
| LOG-04 | Rotation + reconstruction-sufficiency test | §3.6 per-session rotation; §9 reconstruction test design |
| PARA-01 | Task/Agent dispatches subagents as isolated goroutine turn-loops, restricted tools | §10.1 subagent goroutine; D-10 runtime restriction |
| PARA-02 | Subagent results via bus tagged parent-turn-id; only results to parent window | §10.2/§10.3 event-vs-window boundary; D-11 |
| PARA-03 | Panicking subagent recovered → tool-error, never crash | §10.4 recover at goroutine boundary; D-13 |
| PARA-04 | Provider-layer semaphore bounds parent+subagent concurrency | §11 the semaphore; D-12 |

</phase_requirements>

---

## 1. Carry-forward from Phase 1 (the machinery Phase 2 wraps — Phase-1 D-09..D-15 survive)

Phase 2 wraps the Phase-1 mimicry machinery. The Phase-1 plans (01-01..01-06) define the package structure; Phase 2 takes these as DEPENDENCIES (passed in or constructed once per D-17). Phase 2 does NOT rewrite them.

**Reusable Phase-1 assets (intended post-Phase-1 state):**

| Asset | Package | Interface (from Phase-1 plan signatures) | Phase-2 use |
|-------|---------|------------------------------------------|-------------|
| Profile artifact + loader | `internal/profile` | `profile.Profile` struct (System, Tools, Headers, Thinking, ToolChoice, Model, MaxTokens, Name); `loader.Load(name) (Profile, error)` | Session Core constructs Shaper with loaded profile; transcript references profile by name |
| Profile Shaper | `internal/shaper` | `Shaper.Shape(profile, messages) (anthropic.MessageRequest, []option.RequestOption, error)` | Session Core calls Shape per turn; emits `RequestShaped` to bus |
| Provider interface + adapters | `internal/provider` | `Provider interface { Send(ctx, profile, messages) (Response, error) }`; `Response{ToolCalls []ToolCall, FinishReason string, Raw json.RawMessage}`; `ToolCall{Name string, Input json.RawMessage}`; `AnthropicProvider`, `OpenAIProvider` | Session Core holds a `Provider`; **Phase 2 adds streaming + semaphore here** (see §11) |
| Test-harness Turn Loop | `internal/loop` | `Run(ctx, profile, Provider, prompt) ([]provider.ToolCall, error)` — SINGLE-TURN | **REPLACED** by Session Core turn loop per D-17. The package is either deleted or its content moves into `internal/session` (planner decides; recommend keeping `internal/loop` as a thin wrapper that calls `session.Session.Run` for back-compat during the transition, then removing in Phase 4) |
| Event bus seed | `internal/event` | `Event interface { Kind() string }`; `RequestShaped struct`; `Bus.Subscribe(kind, handler)`; `Bus.Publish(e)` (goroutine-per-subscriber) | **EXPANDED** into typed channels per D-04. The Phase-1 `Bus` shape changes (see §2) — the seed grows, not replaced, but the API surface evolves |
| Audit subscriber (LOG-01) | `internal/audit` | `AuditLogger` subscribes to "RequestShaped", writes redacted verbatim to `io.Writer` | **FOLDED INTO** transcript writer per D-20 (one artifact). The Phase-1 AuditLogger becomes the transcript writer's `RequestShaped` handler — same redaction, same async, new sink (`.ass-guard/<session>.jsonl`) |
| Redaction | `internal/redact` | `Redact([]byte) ([]byte, error)`; `IsSecretKey(string) bool`; `ScrubError(err) string` | Transcript writer calls per-line (LOG-03); ACP error responses scrubbed |
| Drift detector | `internal/drift` | PROF-04 live capture + diff | Unchanged in Phase 2 (PROF-04 is Phase 1) |
| Built-in tool catalog | `internal/toolcat` | catalog + schema adapter + consistency CI | **EXTENDED**: add `Mutability` field per tool (D-19); runtime restriction API for subagents (D-10) |
| Parity harness | `internal/parity` | A/B metric + replay + harness | Unchanged (Phase 1) |

**Critical: the `Provider.Send` signature is non-streaming.** Phase 1's `Send` returns a complete `Response`. Phase 2's ACP-04 streaming requirement (provider SSE → bus → session/update) needs a STREAMING provider method. Research recommendation (§11.2): add `Provider.Stream(ctx, profile, messages) (<-chan StreamChunk, Response, error)` alongside `Send`; the ACP turn loop uses `Stream`, the parity harness keeps using `Send`. This is an additive change to `internal/provider`, not a rewrite.

**`cmd/ass-guard/main.go` evolution:** Phase 1's main is a cobra root with `profile check` + a tracer prompt flag. Phase 2's main adds the `acp serve` subcommand (the stdio JSON-RPC server) — the ACP entry point Zed spawns. The tracer prompt flag is retained for dev/debug.

---

## 2. Event bus expansion (D-04 — typed Go channels, the streaming spine)

### 2.1 The channel-per-type shape (D-04 resolution)

Phase 1's `event.Bus` used a `map[string][]handler` with goroutine-per-subscriber dispatch (Phase-1 D-13). D-04 expands this into **typed Go channels — one channel per event type**. The expansion preserves Phase-1's `Event` interface and `RequestShaped` type (they migrate to the new shape), and the goroutine-per-subscriber contract migrates to "one consumer goroutine per channel, select-looping."

**Recommended shape (`internal/event/bus.go`):**

```go
// Event is the sealed interface all bus events implement (Phase-1 seed preserved).
type Event interface {
    Kind() string
}

// Bus holds one buffered channel per event kind. Producers send; consumers select.
// Backpressure (D-05): channels are buffered; full channel blocks the producer.
type Bus struct {
    chans map[string]chan Event
    mu    sync.RWMutex // only guards chans map creation, not sends
}

// Subscribe returns a receive-only channel for a kind. Buffer size is set at
// subscription registration (or bus construction) per D-05.
func (b *Bus) Subscribe(kind string, buffer int) <-chan Event

// Publish sends to the channel for the event's kind. Blocks if buffer full (D-05).
// If no subscriber exists for the kind, the event is dropped (logged to stderr).
func (b *Bus) Publish(e Event) error
```

**Why channels over handler-slices (D-04 rationale):**
- Type-safe at the call site (`ev := (<-ch).(AgentMessageChunk)` — the consumer knows the type).
- Backpressure is natural (D-05 — a full `chan` blocks the producer, propagating slowdown end-to-end without a separate mechanism).
- Multiple consumers of the same kind fan out by subscribing multiple channels (or the bus internally fans out one Publish to N channels — see §2.2).
- Zero external deps; idiomatic Go.

**Fan-out question (D-04 implicit):** does one Publish reach multiple subscribers of the same kind? YES — the ACP adapter and the transcript writer both consume `AgentMessageChunk`. **Recommended:** the Bus maintains a slice of channels per kind; `Publish` sends to each (blocking per-channel, so a slow consumer stalls only its own copy — OR use a per-subscriber goroutine that drains into the channel; recommend the slice-of-channels + blocking-send for simplicity, accepting that a stuck ACP client stalls the turn per D-05's explicit design). The planner should make the multi-subscriber blocking semantics explicit in the task action.

### 2.2 The event type catalog (D-04 — one channel per type)

| Event type | `Kind()` | Producer | Consumer(s) | Payload fields |
|------------|----------|----------|-------------|----------------|
| `RequestShaped` | `"RequestShaped"` | Session Core (pre-send, D-18 step 2) | Transcript writer (LOG-01 verbatim) | `VerbatimRequest json.RawMessage`, `Profile string`, `Timestamp time.Time`, `TurnID string` (NEW for session scoping) |
| `AgentMessageChunk` | `"AgentMessageChunk"` | Provider stream → Session Core | ACP adapter (→ session/update), Transcript writer | `Content string`, `MessageID string`, `TurnID string`, `IsFinal bool` |
| `ToolCall` | `"ToolCall"` | Session Core (parsed from response) | ACP adapter, Transcript writer | `ToolCallID string`, `Name string`, `Input json.RawMessage`, `TurnID string`, `ParentTurnID string` (for subagents) |
| `ToolCallUpdate` | `"ToolCallUpdate"` | Tool executor (stubbed Phase 2) | ACP adapter, Transcript writer | `ToolCallID string`, `Status string` (`running`/`completed`/`failed`/`cancelled`), `Content []ContentBlock`, `TurnID string` |
| `UsageUpdate` | `"UsageUpdate"` | Provider stream | ACP adapter, Transcript writer | `UsedInputTokens int`, `UsedOutputTokens int`, `TurnID string` |
| `SubagentResult` | `"SubagentResult"` | Subagent goroutine (D-11) | Parent turn loop, Transcript writer | `ParentTurnID string`, `Result string`, `Error error`, `ToolCallID string` (the dispatching Task call) |
| `Boundary` | `"Boundary"` | Session Manager (D-08) | Transcript writer (always), Projector (for next projection) | `Cause string`, `CommandRef string`, `Timestamp time.Time` |

**`RequestShaped` migration note:** Phase 1's `RequestShaped` had no `TurnID`. Phase 2 adds `TurnID` (and `ParentTurnID` for subagent-shape requests) so the transcript can correlate the verbatim request to its turn. This is additive — Phase-1 tests that construct `RequestShaped{VerbatimRequest, Profile, Timestamp}` still compile (struct literal with named fields).

### 2.3 Buffer sizes per channel type (D-05 resolution — "64–128" picked)

D-05 defers the exact N per channel to research. Recommendation:

| Channel kind | Buffer N | Rationale |
|--------------|----------|-----------|
| `RequestShaped` | 16 | One per turn; the transcript writer drains immediately. Small buffer is plenty; blocking here is fine (turn-scoped). |
| `AgentMessageChunk` | 128 | High-frequency (token-by-token from provider SSE). Large buffer absorbs burstiness; the ACP adapter drains at stdout-write speed. 128 = upper bound of D-05's range. |
| `ToolCall` | 16 | Few per turn (1–5 typical). |
| `ToolCallUpdate` | 32 | Multiple updates per tool call (progress). |
| `UsageUpdate` | 8 | ~1 per chunk-batch from provider. |
| `SubagentResult` | 8 | Few subagents per turn. |
| `Boundary` | 4 | One per mutating command; rare. |

**The backpressure contract (D-05 explicit):** if a channel fills, `Publish` BLOCKS. This propagates: a stuck ACP client (stdout pipe full) → ACP adapter stops draining `AgentMessageChunk` → channel fills → provider SSE reader blocks → provider HTTP request stalls (does not consume more tokens). The turn stalls rather than dropping user-visible tokens or growing memory unbounded. This is the intended behavior — a stuck client stalls the turn, per D-05. **`session/cancel` (D-16) is the escape hatch** — it drains the queued events and aborts.

**Config surface:** buffer sizes are package-level `const` (not config) in v1. If Phase 3+ needs tuning, they move to config then. Recommend NOT making them config in Phase 2 (YAGNI; the values above are well-reasoned defaults).

### 2.4 The async contract (LOG-02 — transcript writer subscribes async)

D-20 satisfies LOG-02 because the transcript writer is a consumer goroutine that selects on the channels it cares about (`RequestShaped`, `AgentMessageChunk`, `ToolCall`, `ToolCallUpdate`, `UsageUpdate`, `SubagentResult`, `Boundary`). It runs for the lifetime of the session, draining into `.ass-guard/<session>.jsonl`. It is NEVER in the critical path of the turn (it's behind the channel buffer). The Phase-1 `AuditLogger` constructor (`NewAuditLogger(bus, sink)`) migrates to `NewTranscriptWriter(bus, sink)` that subscribes to all 7 kinds.

---

## 3. Transcript schema (D-03/D-20 — one append-only JSONL artifact)

### 3.1 The line-type discriminator

Every line in `.ass-guard/<session-id>_<timestamp>.jsonl` is one JSON object with a `type` field. The schema is the union of all line types (D-20 — one artifact captures everything):

| `type` | When written | Key fields | Source REQ |
|--------|--------------|------------|------------|
| `session_start` | On session/new | `sessionID`, `cwd`, `profile`, `startedAt`, `agentVersion` | SESS-06, LOG-04 |
| `user_message` | On session/prompt | `turnID`, `content []ContentBlock`, `timestamp` | SESS-01, LOG-01 |
| `request_shaped` | Pre-send, every turn (LOG-01 verbatim) | `turnID`, `verbatim json.RawMessage` (REDACTED per LOG-03), `profile` | LOG-01, D-20 |
| `agent_message_chunk` | Streamed from provider | `turnID`, `messageID`, `content`, `isFinal` | ACP-04, LOG-01 |
| `assistant_message` | On end_turn (the assembled message) | `turnID`, `content`, `finishReason` | SESS-01 |
| `tool_call` | Parsed from response | `turnID`, `toolCallID`, `name`, `input json.RawMessage` | SESS-01, LOG-01 |
| `tool_result` | After tool execution (stubbed Phase 2) | `turnID`, `toolCallID`, `output`, `isError`, `mutability` | SESS-03, LOG-01 |
| `boundary` | Mutating command completes (D-08) | `cause`, `commandRef`, `timestamp`, `turnID` | SESS-02/05, D-08 |
| `subagent_dispatch` | Task/Agent tool invoked | `parentTurnID`, `subagentTurnID`, `restrictedTools []string` | PARA-01 |
| `subagent_result` | Subagent goroutine returns | `parentTurnID`, `subagentTurnID`, `result`, `error` | PARA-02/03 |
| `usage` | Per chunk-batch from provider | `turnID`, `inputTokens`, `outputTokens` | ACP-04 |
| `canceled` | session/cancel received (D-16) | `turnID`, `timestamp`, `reason` | D-16 |
| `engine_decision` | (Phase 4 mostly; Phase 2 may log cancel decisions) | `turnID`, `decision`, `rationale` | (Phase 4) |
| `error` | ANY error encountered (investigate-and-fix-ready, PROJECT.md) | `turnID`, `component`, `message`, `inputs`, `recoverable bool`, `stack string` (on panic) | PROJECT.md must-have |
| `session_end` | On logout/process exit | `sessionID`, `endedAt`, `turnCount` | LOG-04 |

**Investigate-and-fix-ready corollary (PROJECT.md must-have):** the `error` line type is FIRST-CLASS. Every error — provider failure, tool failure, panic-recovered subagent, unexpected state — is written as an `error` line with: the component that failed, the error message (scrubbed via `redact.ScrubError`), the inputs that triggered it (redacted), a `recoverable` classification (can the turn continue, or is it fatal), and on panic, the stack trace. The transcript is the primary diagnostic surface; a developer must be able to diagnose root cause from the transcript alone.

### 3.2 Content blocks (shared shape)

`ContentBlock` is the shared content shape (matches ACP v1 `session/update` content blocks):
```go
type ContentBlock struct {
    Type string `json:"type"`           // "text", "resource", "image", "audio"
    Text string `json:"text,omitempty"` // for type="text"
    // resource/image/audio fields per ACP spec (Phase 2 implements text only; others stubbed)
}
```

### 3.3 Boundary line (D-08 — explicit, first-class)

```json
{"type":"boundary","cause":"mutating-command:Bash","commandRef":"toolCallId_abc123","timestamp":"2026-08-10T12:34:56Z","turnID":"turn_042"}
```
- `cause`: the mutability source — `mutating-command:<ToolName>` (a tool flagged mutating by D-19 formula), or `config-added:<RuleName>` (a config-added boundary, SESS-02).
- `commandRef`: the `toolCallID` of the mutating command that triggered the boundary.
- Written by the Session Manager AFTER the mutating tool's result is appended (so the transcript order is: `tool_result` → `boundary`).

### 3.4 Session Manager API (SESS-06 — sole transcript owner, D-17 one package)

```go
// internal/session/manager.go
package session

// Manager is the SOLE owner of the transcript (SESS-06). All reads/writes go through here.
type Manager struct {
    transcript *Transcript  // the open JSONL file
    bus        *event.Bus
    // ...
}

// Append-only API (SESS-06). No mutation of prior lines.
func (m *Manager) AppendUserMessage(turnID string, content []ContentBlock) error
func (m *Manager) AppendAssistantMessage(turnID string, content []ContentBlock, finishReason string) error
func (m *Manager) AppendToolCall(turnID, toolCallID, name string, input json.RawMessage) error
func (m *Manager) AppendToolResult(turnID, toolCallID string, output json.RawMessage, isError bool, mutability string) error
func (m *Manager) AppendBoundary(cause, commandRef, turnID string) error
func (m *Manager) AppendError(turnID, component, message string, inputs json.RawMessage, recoverable bool, stack string) error
// ... one Append per line type

// Read API (for the projector, SESS-01).
func (m *Manager) ReadSince(turnID string) ([]TranscriptLine, error)  // lines after a turn
func (m *Manager) ReadLastBoundary() (TranscriptLine, error)          // for projector reset point
func (m *Manager) ReadAll() ([]TranscriptLine, error)                 // for reconstruction test (LOG-04)
```

**Concurrency:** the Manager serializes all appends through a mutex (one writer, D-20). Reads are lock-free (append-only file → readers see consistent prefixes). The transcript file is opened `O_APPEND` — each `Append*` call is one `Write` of one JSON line + `\n` (atomic on POSIX for lines < PIPE_BUF = 4096 bytes; for larger lines, the mutex serializes).

### 3.5 Per-line redaction (LOG-03)

Every `Append*` call redacts the line BEFORE writing, using `internal/redact.Redact` (Phase 1). For `request_shaped` lines (LOG-01 verbatim), the FULL verbatim request is redacted (secret values → `[REDACTED]`, field names + the 12 identity header names preserved — Phase 1's redact package already does this). For `tool_call`/`tool_result` lines, `input`/`output` are scrubbed for secret-carrier keys. `error` lines: `message` via `redact.ScrubError`, `inputs` via `redact.Redact`.

### 3.6 Rotation (LOG-04 — per-session files)

Rotation = per-session files. A new `session/new` → a new `.ass-guard/<session-id>_<timestamp>.jsonl`. No within-file rotation needed in v1 (a single session's transcript is bounded by session length; a long session produces a large file, but that's the human-investigation artifact — greppable). If a session exceeds a threshold (recommend 100MB, configurable later), Phase 4 can add rotation; Phase 2 does NOT.

**Self-gitignore (D-07):** on first run (session/new with no existing `.ass-guard/`), ass-guard creates `.ass-guard/.gitignore` with:
```
*
!.gitignore
```
This makes the dir exist in the repo but its contents never committed. Matches `.claude/` convention.

---

## 4. Lean seed + projector (SESS-01/04 — D-01/D-02)

### 4.1 Projector (builds the lean window from the transcript)

```go
// internal/session/projector.go
package session

// Projector builds the model-visible lean window from the transcript at each turn boundary.
type Projector struct {
    profile profile.Profile
    manager *Manager  // reads the transcript
}

// Project builds the lean window for the NEXT turn:
// 1. The profile system prompt (TIER-1 faithful)
// 2. A one-paragraph task summary (D-02 mechanical extraction)
// 3. The current user message (the new command's intent)
// 4. ZERO carry-forward of prior turn assistant/tool messages
func (p *Projector) Project(turnID string) ([]provider.Message, error)
```

### 4.2 Lean seed composition (D-01 — hard cut, summary is the bridge)

The lean window the model sees after a boundary reset:
1. **System prompt** — from the profile (TIER-1 faithful, byte-for-byte from `profile.System`). Re-shaped by the Shaper on every turn.
2. **Task summary** — one paragraph, MECHANICALLY extracted (D-02 — NOT a model call).
3. **Current user message** — the new command being processed (from the `session/prompt` params).
4. **ZERO prior turn messages** — no carry-forward of assistant responses, tool calls, or tool results from before the boundary.

### 4.3 Task-summary extraction rule (D-02 resolution — the deferred detail)

D-02 defers the exact extraction rule and the N-char truncation to research. Recommendation:

**The extraction reads the transcript back to the most recent `boundary` line (or session_start if none) and synthesizes:**

```
Task summary (mechanical, no model call):
- Current intent: {last user_message.content text, truncated to 500 chars}
- Files touched since last boundary: {deduplicated list of file_path values extracted from
  tool_call.input and tool_result.output for tools Read/Write/Edit/Glob/Grep/Bash, joined as
  a comma-separated list, max 20 files, truncated with "... and N more" beyond 20}
- Last assistant position: {last assistant_message.content text, truncated to 300 chars}
```

**N-char truncation policy:**
- Last user message: 500 chars (current intent is the most important; 500 chars covers a substantial prompt).
- Files touched: 20 files max (deduplicated; beyond 20, "... and N more").
- Last assistant message: 300 chars (position context; 300 chars is enough to know what was being done).
- Total summary target: ≤ 1500 chars (keeps the lean window genuinely lean; the model's job is the NEW command, the summary is orientation only).

**Why these values:** they are large enough to orient the model (it knows what it was doing and where) but small enough that the lean window is dramatically smaller than a full conversation history (a 50-turn session's full history could be 50K+ tokens; the summary is ~400 tokens). The values are `const` in v1 (not config) — if Phase 4's engine needs to tune them, they move to config then.

**Extraction is from the transcript only** (no model call → no latency, no mimicry divergence, D-02's core point). The `Projector.Project` method reads `manager.ReadSince(lastBoundary)` and applies the extraction rules above mechanically.

**Edge cases:**
- No prior boundary (first turn of session): the summary is empty (or just "Session start"); the lean window is system prompt + first user message.
- No files touched (pure-conversation turn): the "Files touched" line is omitted.
- User message exceeds 500 chars: truncated to 497 chars + "...".
- The current user message IS the "last user message" (the new command): so the summary's "current intent" and the lean window's "current user message" are the same content — the summary includes it for orientation, the window includes it as the actual message. This is intentional (the summary bridges; the message is what the model acts on).

---

## 5. session/load (D-09 — DROPPED, no-op/fallback)

Per D-09, `session/load` replay is OUT of v1. The transcript's purpose is human investigation, not editor replay. The `session/load` method:

**Recommended implementation (no-op/new-session fallback):**
- ass-guard advertises `agentCapabilities.loadSession: false` in the `initialize` response. This signals to Zed that `session/load` is unavailable — a spec-conformant client will not call it (per ACP v1 `session-setup.md`: "Clients MUST NOT attempt to call session/load" if loadSession is false/absent).
- If a client sends `session/load` anyway (non-conformant or future Zed behavior), ass-guard responds with a JSON-RPC error (method not supported / `loadSession: false`), OR — the fallback option — treats it as `session/new` (creates a fresh session, ignoring the requested sessionId). Recommend the error response (cleaner; matches the advertised capability).

**SESS-05 disposition:** the requirement's *rehydration-on-session/load* part is MOOT (no replay). The *boundary-marker durability* part (D-08) is STILL NEEDED for live projection — the projector reads boundary markers to know where to reset the lean window during a live session. So boundary markers are written and read, but never replayed to the editor.

**Planner note:** ACP-03 stays in the `requirements:` field of whatever plan owns the `session/load` handler, but the task action explicitly implements the no-op/error per D-09. The plan must NOT implement replay. The success criterion #2 in ROADMAP.md references replay — this is out of v1 per D-09; the planner should note this in the plan's `<objective>` (the transcript persistence for human investigation remains; the replay portion is deferred).

---

## 6. Mutability + boundaries (SESS-02/03 — D-19 formula, structural enforcement)

### 6.1 The per-tool Mutability field (D-19)

Extend the Phase-1 tool catalog (`internal/toolcat`) with a `Mutability` field on each tool declaration:

```go
// internal/toolcat/types.go (Phase-2 extension)
type Mutability string
const (
    Mutating  Mutability = "mutating"
    ReadOnly  Mutability = "read-only"
)

type Tool struct {
    Name       string          `json:"name"`
    InputSchema json.RawMessage `json:"input_schema"`
    Mutability Mutability      `json:"mutability"`  // NEW (D-19)
    // ... other fields
}
```

**Built-in catalog defaults (D-19):**
| Tool | Mutability |
|------|-----------|
| `Bash` | `mutating` |
| `Write` | `mutating` |
| `Edit` | `mutating` |
| `Read` | `read-only` |
| `Glob` | `read-only` |
| `Grep` | `read-only` |
| `Task`/`Agent` | `read-only` (the dispatch itself doesn't mutate; the subagent's tools do) |
| `TodoRead`/`TodoWrite` | `TodoWrite` = `mutating`, `TodoRead` = `read-only` |
| (all other built-ins) | per-tool judgment; default `read-only` unless the tool writes |

### 6.2 The SESS-03 formula (more-mutating wins)

```go
// internal/session/mutability.go (or internal/toolcat/mutability.go)
// EffectiveMutability resolves the SESS-03 formula:
//   mutating if (adapterClass(command) == mutating OR tool.Mutability == mutating) else read-only
// adapterClass is the adapter's classification (e.g. OpenSpec adapter declares a command mutating);
// tool.Mutability is the catalog field (D-19).
func EffectiveMutability(tool Tool, adapterClass Mutability) Mutability {
    if tool.Mutability == Mutating || adapterClass == Mutating {
        return Mutating
    }
    return ReadOnly
}
```

### 6.3 Structural enforcement (SESS-02 — config can ADD, never remove)

SESS-02 ("mutating commands are ALWAYS boundaries; config may only ADD") is enforced STRUCTURALLY:
- The built-in catalog's `Mutability` field is the floor. A tool declared `mutating` in the catalog is ALWAYS a boundary.
- Config (a future `.ass-guard/config.yaml` or profile overlay) can declare ADDITIONAL boundaries (e.g. "treat `WebFetch` as a boundary" — adding a boundary to a read-only tool).
- Config CANNOT flip a `mutating` tool to `read-only`. The `EffectiveMutability` function does not consult config for downgrading — only the catalog field + adapter class (both one-way-mutating). Config-added boundaries are a separate mechanism (a `ConfigAddedBoundaries []string` list checked after the formula).

**Phase-2 scope:** the config surface is minimal — a `[]string` of tool names that are ADDITIONAL boundaries. Full config schema is Phase 4. Phase 2 implements the formula + the structural floor + the config-adds-only rule.

### 6.4 Forward-design for OpenSpec (D-19 — Phase 4 no engine change)

Phase 4's OpenSpec adapter registers commands with their own `Mutability` field. The `EffectiveMutability(tool, adapterClass)` function takes the adapter's classification as the second arg — Phase 4 passes the OpenSpec-declared mutability; no engine change needed. This is D-19's forward-design: the interface is `EffectiveMutability(tool Tool, adapterClass Mutability)`, and Phase 4 just passes a different `adapterClass`.

---

## 7. ACP v1 wire shape (ACP-01/02/05 — VERIFIED-FACTS #3 ground truth + canonical spec)

This section is the load-bearing wire-shape reference. It is grounded in VERIFIED-FACTS.md item #3 (Tier-A verified, post-spike) + the canonical ACP v1 spec pages fetched 2026-08-09 (`transports.md`, `overview.md`, `initialization.md`, `session-setup.md`, `prompt-turn.md`).

### 7.1 Framing (D-14 — hand-rolled ~150 LOC)

The wire is **newline-delimited JSON-RPC** (`\n`), NOT LSP-style `Content-Length` headers. Verbatim from `transports.md`: *"Messages are delimited by newlines (`\n`), and MUST NOT contain embedded newlines. The agent MUST NOT write anything to its stdout that is not a valid ACP message."*

**The framer (`internal/acp/framer.go`):**
```go
// Reader: bufio.Scanner over stdin (newline-delimited).
scanner := bufio.NewScanner(os.Stdin)
scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line (requests are small)
for scanner.Scan() {
    line := scanner.Bytes()
    var msg Message
    if err := json.Unmarshal(line, &msg); err != nil { /* log + respond parse-error */ continue }
    dispatch(msg)
}

// Writer: mutex-guarded writes to stdout.
type Writer struct {
    mu sync.Mutex
    w  io.Writer  // os.Stdout
}
func (w *Writer) Write(msg Message) error {
    w.mu.Lock()
    defer w.mu.Unlock()
    b, _ := json.Marshal(msg)
    b = append(b, '\n')
    _, err := w.w.Write(b)
    return err
}
```

**Message envelope (JSON-RPC 2.0):**
```go
type Message struct {
    JSONRPC string          `json:"jsonrpc"`           // always "2.0"
    ID      *int            `json:"id,omitempty"`      // present for requests/responses; ABSENT for notifications
    Method  string          `json:"method,omitempty"`  // present for requests/notifications
    Params  json.RawMessage `json:"params,omitempty"`  // present for requests/notifications
    Result  json.RawMessage `json:"result,omitempty"`  // present for responses (success)
    Error   *RPCError       `json:"error,omitempty"`   // present for responses (error)
}
type RPCError struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

**Critical rule (D-14):** notifications carry NO `id` and get NO response. Requests carry an `id` and MUST get a response (success `result` or `error`). The framer distinguishes by presence of `id`.

**~150 LOC budget (D-14):** the framer (reader + writer + envelope + dispatch map) is ~150 LOC of Go. It does NOT pull `kwo/jsonrpc2` or `sourcegraph/jsonrpc2` (rejected per D-14 — adds a dep for a small protocol with unverified fit). Hand-rolled, zero deps, matches LSP-in-Go lineage.

### 7.2 The method set (D-15 — single reader goroutine + per-prompt turn goroutine)

| Method | Direction | `id`? | ass-guard v1 implementation |
|--------|-----------|-------|------------------------------|
| `initialize` | Client → Agent (request) | yes | Returns `protocolVersion: 1`, `agentCapabilities` (loadSession: false per D-09, promptCapabilities, mcpCapabilities, sessionCapabilities, auth), `agentInfo`, `authMethods: []` |
| `session/new` | Client → Agent (request) | yes | Creates a session; returns `{sessionId}`. Creates `.ass-guard/` if missing (D-07), opens transcript (D-20), constructs Session Core (D-17) |
| `session/prompt` | Client → Agent (request) | yes | Spawns a turn goroutine (D-15); streams `session/update` notifications; responds with `{stopReason}` when turn ends |
| `session/cancel` | Client → Agent (NOTIFICATION) | **no** | Cancels the active turn's context (D-16); the turn goroutine drains + exits; the `session/prompt` request gets `stopReason: "cancelled"` |
| `session/load` | Client → Agent (request) | yes | NO-OP per D-09 (loadSession: false advertised). Responds with method-not-supported error, OR new-session fallback |
| `logout` | Client → Agent (request) | yes | Closes the session; writes `session_end` to transcript; flushes; exits cleanly |
| `session/set_mode` | Client → Agent (request) | yes | Sets session mode (e.g. "plan" vs "act"); Phase 2 stubs this (accepts, stores, no behavioral change yet; Phase 4 engine uses it) |

### 7.3 The initialize response (canonical, from spec — `agentCapabilities` field name is load-bearing)

```json
{
  "jsonrpc": "2.0",
  "id": 0,
  "result": {
    "protocolVersion": 1,
    "agentCapabilities": {
      "loadSession": false,
      "promptCapabilities": {"image": false, "audio": false, "embeddedContext": false},
      "mcpCapabilities": {"http": false, "sse": false},
      "sessionCapabilities": {},
      "auth": {"logout": {}}
    },
    "agentInfo": {"name": "ass-guard", "title": "ass-guard", "version": "0.1.0"},
    "authMethods": []
  }
}
```

**Load-bearing field names (VERIFIED-FACTS #3 Notes 3-5):**
- `agentCapabilities` — NOT `capabilities` or `serverInfo` (differs from LSP/MCP). A spec-conformant client looks for this exact key.
- `protocolVersion` is integer `1`, not a string.
- `loadSession: false` per D-09 (no replay).

### 7.4 The session/update notification (ACP-04 — the streaming output)

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "<uuid>",
    "update": {
      "sessionUpdate": "agent_message_chunk",
      "messageId": "msg_abc",
      "content": {"type": "text", "text": "..."}
    }
  }
}
```
**No `id`** (notification). The `sessionUpdate` discriminator (VERIFIED-FACTS #3 Note 5 + canonical `prompt-turn.md`):

| `sessionUpdate` | Maps from event bus kind | Fields in `update` |
|-----------------|--------------------------|--------------------|
| `agent_message_chunk` | `AgentMessageChunk` | `messageId`, `content` (`{type, text}`) |
| `tool_call` | `ToolCall` | `toolCallId`, `title`, `kind`, `status` |
| `tool_call_update` | `ToolCallUpdate` | `toolCallId`, `status`, `content` (on completion) |
| `usage_update` | `UsageUpdate` | `used` (`{inputTokens, outputTokens}`), `size` (optional context window size) |
| `plan` | (Phase 4 — TodoWrite maps to plan; Phase 2 stubs/omits) | `entries` |

### 7.5 The session/prompt response (stopReason)

When the turn ends, the agent responds to the original `session/prompt` request:
```json
{"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}
```
`stopReason` values (canonical `prompt-turn.md`): `end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled` (D-16 cancel → `cancelled`).

---

## 8. ACP server skeleton (ACP-01/02 — D-15 single reader + per-prompt turn goroutine)

### 8.1 The dispatch architecture (D-15)

```go
// internal/acp/server.go
package acp

type Server struct {
    framer  *Framer
    handler map[string]Handler  // method → handler
    sessions map[string]*session.Manager  // sessionId → Session Core
    shaper  *shaper.Shaper
    provider provider.Provider
    profile profile.Profile
    bus     *event.Bus
}

type Handler func(ctx context.Context, params json.RawMessage) (result interface{}, err error)

func (s *Server) Serve(ctx context.Context) error {
    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() {
        var msg Message
        json.Unmarshal(scanner.Bytes(), &msg)
        if msg.ID == nil {
            // notification — dispatch, no response (session/cancel)
            go s.dispatch(ctx, msg)  // or inline; cancel must abort the active turn
        } else {
            // request — dispatch, MUST respond
            go s.dispatch(ctx, msg)  // per-request goroutine so reader keeps reading
        }
    }
}
```

**The single-reader rule (D-15):** ONE goroutine reads from stdin and dispatches via the method→handler map. Each `session/prompt` spawns a CONCURRENT turn goroutine (so `session/cancel` can be processed mid-turn while the prompt is running). The reader never blocks on a turn — it dispatches and immediately reads the next frame.

**Per-prompt turn goroutine lifecycle:**
1. `session/prompt` handler spawns `go s.runTurn(ctx, sessionID, prompt, id)`.
2. `runTurn` calls `session.Manager.Prompt(ctx, prompt)` — the Session Core turn loop (D-18).
3. The turn loop streams events to the bus; the ACP adapter subscribes and writes `session/update` notifications to stdout.
4. On turn end, `runTurn` writes the `session/prompt` response (`{stopReason}`).
5. On `session/cancel` (D-16), the ctx is cancelled → the turn aborts → `runTurn` writes `{stopReason: "cancelled"}`.

### 8.2 The ACP adapter (bus → session/update)

```go
// internal/acp/adapter.go
// Adapter subscribes to the event bus and writes session/update notifications.
type Adapter struct {
    framer   *Framer
    bus      *event.Bus
    sessionID string
}

func (a *Adapter) Run(ctx context.Context) {
    chunks := a.bus.Subscribe("AgentMessageChunk", 128)
    toolCalls := a.bus.Subscribe("ToolCall", 16)
    // ... one subscription per kind the adapter forwards
    for {
        select {
        case <-ctx.Done():
            return
        case ev := <-chunks:
            c := ev.(event.AgentMessageChunk)
            a.framer.Write(sessionUpdateNotification(a.sessionID, map[string]interface{}{
                "sessionUpdate": "agent_message_chunk",
                "messageId": c.MessageID,
                "content": map[string]string{"type": "text", "text": c.Content},
            }))
        case ev := <-toolCalls:
            // ... map to tool_call session/update
        // ... other kinds
        }
    }
}
```

### 8.3 End-to-end streaming (ACP-04 — no full-turn buffering)

The streaming path (ACP-04): provider SSE → `provider.Stream` → Session Core emits `AgentMessageChunk` to bus → ACP adapter drains bus → writes `session/update` to stdout → Zed renders token-by-token. **No full-turn buffering** — each chunk flows through as it arrives. The channel buffer (D-05, 128 for chunks) absorbs micro-bursts but does not hold a full turn.

---

## 9. Reconstruction-sufficiency test (LOG-04)

D-20/LOG-04: a reconstruction-sufficiency test verifies the transcript + profile let you reconstruct what happened. Test design:

```go
// internal/session/reconstruction_test.go
func TestTranscriptReconstructsSession(t *testing.T) {
    // 1. Run a scripted session: session/new + 3 prompts (one triggering a boundary) + logout.
    // 2. Read the transcript file (.ass-guard/<id>.jsonl).
    // 3. Assert the transcript + profile can answer:
    //    a. What prompts did the user send? (user_message lines, in order)
    //    b. What did the model say? (assistant_message lines, in order; agent_message_chunk lines for the streaming evidence)
    //    c. What tools were called, with what args? (tool_call lines)
    //    d. What were the tool results? (tool_result lines)
    //    e. Where were the boundaries? (boundary lines, with cause + commandRef)
    //    f. What was the verbatim shaped request for each turn? (request_shaped lines, redacted)
    //    g. What errors occurred? (error lines, with component + recoverable)
    //    h. What was the cancel state? (canceled lines if any)
    // 4. Assert every line type in §3.1 that occurred is present and well-formed.
    // 5. Assert secrets are redacted (no "sk-" or "Bearer " in the file).
}
```

This test runs as part of the session package test suite (Wave 0 / task-level TDD per Nyquist).

---

## 10. Subagent dispatch (PARA-01..04 — D-10/D-11/D-13)

### 10.1 The Task/Agent tool (PARA-01 — isolated goroutine turn-loop)

When the model invokes the `Task` (or `Agent`) tool, the Session Core's tool executor (stubbed in Phase 2 per D-18 step 5, BUT the subagent dispatch skeleton is real — PARA-01 is in Phase 2 scope) spawns a subagent:

```go
// internal/session/subagent.go
func (m *Manager) DispatchSubagent(ctx context.Context, parentTurnID, toolCallID string, prompt string, restrictedTools []string) {
    subagentTurnID := generateTurnID()
    m.Append(TranscriptLine{Type: "subagent_dispatch", ParentTurnID: parentTurnID, SubagentTurnID: subagentTurnID, RestrictedTools: restrictedTools})
    
    go func() {
        defer m.recoverSubagent(parentTurnID, toolCallID, subagentTurnID)  // D-13
        
        // The subagent is a NESTED Session Core turn loop with:
        // - Its own transcript lines (tagged with subagentTurnID + parentTurnID)
        // - A restricted tool executor (D-10: runtime subset enforcement)
        // - The SAME provider (via the semaphore, D-12)
        // - Its own lean window (projected from its own sub-transcript)
        result, err := m.runSubagentLoop(ctx, subagentTurnID, parentTurnID, prompt, restrictedTools)
        
        m.bus.Publish(event.SubagentResult{ParentTurnID: parentTurnID, Result: result, Error: err, ToolCallID: toolCallID})
        m.Append(TranscriptLine{Type: "subagent_result", ParentTurnID: parentTurnID, SubagentTurnID: subagentTurnID, Result: result, Error: errString(err)})
    }()
}
```

### 10.2 Runtime tool restriction (D-10 — full catalog declared, subset enforced)

The profile declares the FULL tool catalog to the model (parent-side mimicry preserved, Phase-1 D-14). The subagent's tool executor restricts the EXECUTION subset at runtime:

```go
// internal/session/subagent_tools.go
type RestrictedExecutor struct {
    inner      ToolExecutor         // the full executor
    allowed    map[string]bool      // the subset this subagent can run
}
func (r *RestrictedExecutor) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
    if !r.allowed[name] {
        return nil, fmt.Errorf("tool %q is not available in this subagent context", name)
    }
    return r.inner.Execute(ctx, name, input)
}
```

If the subagent's model invokes a restricted tool, it gets the "not available" error and adapts (D-10). The model SEES all tools; the executor ENFORCES which can run.

### 10.3 Event-vs-context-window boundary (D-11 resolution — the deferred detail)

D-11 defers the precise boundary between "streamed events for visibility" and "the parent's lean window." Recommendation:

- **Streamed events (visibility, PARA-02):** `ToolCall`, `ToolCallUpdate`, `AgentMessageChunk` events from the subagent flow to the bus tagged with `ParentTurnID`. The ACP adapter forwards them to Zed as `session/update` notifications — the user SEES the subagent working (tool calls, partial output). These events are ALSO written to the transcript (full investigation record).
- **Parent's lean window (context hygiene):** the parent's lean window receives ONLY the `SubagentResult` — the final result string + error status. The streamed intermediates do NOT enter the parent's projector. When the parent next projects its lean window, the subagent's work appears as a single `tool_result` line (the final result), not as the full stream.

**The mechanism:** the parent's projector reads its OWN transcript lines (tagged with the parent's turnID). The subagent's streamed events are tagged with the subagentTurnID — they're in the transcript (for investigation) but the parent's projector skips them (it reads only parent-turnID lines + the final `tool_result` that the `SubagentResult` handler appends to the parent's transcript). This keeps the parent's lean window lean (D-11's "only results, not accumulated context") while the events stream for visibility.

### 10.4 Panic recovery (D-13 — goroutine-boundary recover)

```go
func (m *Manager) recoverSubagent(parentTurnID, toolCallID, subagentTurnID string) {
    if r := recover(); r != nil {
        stack := debug.Stack()
        err := fmt.Errorf("subagent panic: %v", r)
        // Publish error result to bus (parent gets a tool-error)
        m.bus.Publish(event.SubagentResult{
            ParentTurnID: parentTurnID,
            ToolCallID:   toolCallID,
            Result:       "",
            Error:        err,
        })
        // Investigate-and-fix-ready: write the error + stack to the transcript
        m.Append(TranscriptLine{
            Type: "error",
            TurnID: subagentTurnID,
            Component: "subagent",
            Message: redact.ScrubError(err),
            Inputs: nil,
            Recoverable: false,  // the subagent is dead, but the parent continues
            Stack: string(stack),
        })
        m.Append(TranscriptLine{Type: "subagent_result", ParentTurnID: parentTurnID, SubagentTurnID: subagentTurnID, Error: err.Error()})
    }
}
```

**The recover is at the goroutine boundary** (the `defer` in the goroutine), NOT inside the turn loop. The turn loop itself does not recover — a panic propagates up to the goroutine's defer, is converted to a tool-error result, and the parent adapts. Process NEVER crashes (D-13).

---

## 11. Provider streaming + semaphore (ACP-04 + PARA-04)

### 11.1 The provider semaphore (D-12 — bounds parent + subagent concurrency)

```go
// internal/provider/semaphore.go (or internal/session/semaphore.go)
type Semaphore struct {
    ch chan struct{}
}
func NewSemaphore(max int) *Semaphore {
    return &Semaphore{ch: make(chan struct{}, max)}
}
func (s *Semaphore) Acquire(ctx context.Context) error {
    select {
    case s.ch <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
func (s *Semaphore) Release() { <-s.ch }
```

**Combined parent + subagent (D-12 verbatim):** the Session Core holds ONE semaphore, constructed once. Both the parent turn loop and every subagent goroutine call `Acquire` before calling `provider.Stream`/`Send`. When full, new dispatches wait. This bounds total outbound concurrency regardless of fan-out depth.

**Default max (D-12 resolution — "4–8"):** recommend **default 6** (middle of the 4–8 range; allows a parent + a few subagents concurrent without exhausting typical provider rate limits). Config surface: `--max-concurrent` flag on `acp serve` (or env `ASSGUARD_MAX_CONCURRENT`), defaulting to 6. The semaphore is constructed from this value at server startup.

### 11.2 The streaming provider method (ACP-04 — additive to Phase 1)

Phase 1's `Provider.Send` is non-streaming (returns complete `Response`). Phase 2 adds `Stream`:

```go
// internal/provider/provider.go (Phase-2 extension)
type StreamChunk struct {
    Type       string          // "text", "tool_use", "usage"
    Text       string          // for type="text"
    ToolCall   *ToolCall       // for type="tool_use"
    Usage      *Usage          // for type="usage"
}

type Provider interface {
    Send(ctx context.Context, prof profile.Profile, messages []Message) (Response, error)         // Phase 1 (parity harness)
    Stream(ctx context.Context, prof profile.Profile, messages []Message) (<-chan StreamChunk, Response, error)  // Phase 2 (ACP turn loop)
}
```

**`AnthropicProvider.Stream` implementation:** uses `anthropic-sdk-go`'s streaming API (`client.Messages.New(ctx, request, headerOptions...)` with a streaming option, OR the SDK's streaming helper if available). The SDK returns an iterator/chunk-reader; the adapter reads chunks, emits each to the returned channel, and assembles the final `Response`. When the stream ends, the channel is closed and the assembled `Response` is returned. The Session Core turn loop selects on the channel, emitting `AgentMessageChunk`/`ToolCall`/`UsageUpdate` to the bus per chunk.

**Backpressure end-to-end (D-05):** if the ACP adapter stops draining the bus, the bus channel fills, `Publish` blocks, the Session Core stops reading from the provider's `<-chan StreamChunk`, the SDK's HTTP reader blocks, the HTTP request stalls. Natural end-to-end backpressure.

---

## 12. Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON-RPC framing | A framing library (`kwo/jsonrpc2`, `sourcegraph/jsonrpc2`) | Hand-rolled `bufio.Scanner` + mutex-guarded writer (~150 LOC) | D-14 — small protocol, unverified library fit, zero deps preferred |
| SSE parsing | Custom SSE parser | `anthropic-sdk-go` native streaming | The SDK handles SSE; ass-guard reads chunks from the SDK |
| UUID generation | Custom ID format | `github.com/google/uuid` (already a transitive dep via anthropic-sdk-go) | Standard, no new dep |
| JSON encoding | Custom serializer | `encoding/json` (stdlib) | Standard |
| Secret redaction | New redactor | `internal/redact` (Phase 1) | Already built, already tested, ≥3 callers |
| Backpressure | Custom rate-limiter / drop policy | Bounded channels (D-05 — block on full) | Idiomatic Go; natural propagation |
| Tool dispatch | A new executor framework | Phase-1 `internal/toolcat` + a thin `ToolExecutor` interface | Phase 1 already has the catalog; Phase 2 adds the dispatch skeleton + restriction wrapper |

---

## 13. Common Pitfalls

### Pitfall 1: Stdout pollution (transport discipline violation)
**What goes wrong:** any byte written to stdout that is not a valid ACP frame breaks the protocol — Zed cannot parse the stream, the session dies.
**Why it happens:** a `log.Printf`, a panic stack trace, a deferred debug print goes to stdout instead of stderr.
**How to avoid:** the `cmd/ass-guard/main.go` entry point redirects stdout to the framer exclusively; ALL logging goes to stderr (`log.SetOutput(os.Stderr)`). The ACP `Writer` is the ONLY thing that writes to stdout. Unit tests assert stdout is clean after a session (transport-discipline test, per Phase 0 #5).
**Warning signs:** Zed shows "protocol error" or hangs; a `grep -c` on stdout output shows non-JSON lines.

### Pitfall 2: Notification vs request confusion (D-14)
**What goes wrong:** the framer sends a response to a notification (which has no `id`), or fails to send a response to a request (which has an `id`).
**Why it happens:** the dispatch logic doesn't distinguish the two; a notification is treated like a request and a response is attempted (writing a malformed frame with no `id`).
**How to avoid:** the `Message.ID` is a `*int` (nil for notifications). The framer checks `if msg.ID == nil` → notification (no response); else → request (MUST respond). Unit tests cover both paths.
**Warning signs:** Zed logs "unexpected response" or "missing response for request id=N".

### Pitfall 3: Leaky goroutines on cancel (D-16)
**What goes wrong:** `session/cancel` cancels the context but the turn goroutine doesn't exit cleanly — it leaks, continues writing to the bus, or panics on a closed channel.
**Why it happens:** the turn loop doesn't select on `ctx.Done()`, or the provider's stream reader doesn't respect context cancellation.
**How to avoid:** every channel select in the turn loop includes `case <-ctx.Done(): return`. The provider's `Stream` takes the ctx and aborts the HTTP request on cancellation (`http.NewRequestWithContext`). The turn goroutine's defer drains queued events + writes the cancel boundary marker.
**Warning signs:** goroutine count climbs after cancels (`runtime.NumGoroutine()` in tests); leaked goroutines write to a closed bus.

### Pitfall 4: Transcript corruption on concurrent append (SESS-06)
**What goes wrong:** two goroutines append simultaneously → interleaved bytes in the JSONL file → unparseable lines.
**Why it happens:** the Session Manager is supposed to be the sole writer, but a subagent goroutine or the adapter calls `Append` directly.
**How to avoid:** the Manager's `Append*` methods all take a mutex (D-20 — one writer). NOTHING else writes to the transcript file. The bus → transcript-writer goroutine is the ONLY path (even subagent results flow through the bus → writer, not direct append).
**Warning signs:** `jq` fails to parse the JSONL file; lines are truncated or merged.

### Pitfall 5: Mimicry divergence from summary model call (D-02 violation)
**What goes wrong:** a "convenience" optimization uses a model call to summarize the transcript → the shaped request differs from zcode's → mimicry breaks.
**Why it happens:** someone adds an LLM call for "better summaries" without realizing it violates D-02.
**How to avoid:** the `Projector.Project` method is PURE GO code (mechanical extraction, no provider call). Unit tests assert no `Provider` is constructed in the projector's dependency graph. The D-02 lint: `grep -rn "Provider\|Send\|Stream" internal/session/projector.go` returns empty.
**Warning signs:** projector test imports `internal/provider`; latency on boundary reset (a model call adds seconds).

### Pitfall 6: Scope creep into replay (D-09 violation)
**What goes wrong:** the planner/executor implements `session/load` replay because it's in the requirements, despite D-09 dropping it.
**Why it happens:** ACP-03 is in the requirements list; the planner doesn't internalize D-09.
**How to avoid:** the plan's `<objective>` explicitly states "ACP-03 DROPPED per D-09; session/load is a no-op/error." The success criterion #2 in ROADMAP references replay — the plan notes this is out of v1. The `requirements:` field includes ACP-03 (for traceability) but the task action implements the no-op.
**Warning signs:** a task implements "replay session/update notifications from transcript."

### Pitfall 7: Panic in parent turn crashes the process (D-13 scope)
**What goes wrong:** D-13 recovers SUBAGENT panics, but a panic in the PARENT turn loop (or the ACP reader goroutine) crashes the process.
**Why it happens:** D-13 is scoped to subagent goroutines; the parent turn loop and the ACP server goroutines need their own recovery.
**How to avoid:** the parent turn goroutine ALSO wraps in `recover()` (writing an `error` line + responding to the `session/prompt` request with an error result, NOT crashing). The ACP reader goroutine wraps in `recover()` (logs + continues reading). Process NEVER crashes is the PROJECT.md investigate-and-fix-ready principle — every panic is caught and written to the transcript.
**Warning signs:** process exits non-zero mid-session; transcript has no `error` line for the crash.

---

## 14. Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| Go toolchain | All code | ✓ | 1.25 (go.mod) | — |
| `anthropic-sdk-go` | Provider streaming | ✓ (Phase 1 dep) | v1.62.0+ | — |
| `go-openai` | OpenAI-shape provider (Phase 2 keeps; not primary for ACP turn loop) | ✓ (Phase 1 dep) | v1.42.0 | — |
| `github.com/google/uuid` | Session/turn IDs | ✓ (transitive via anthropic-sdk-go) | — | — |
| Zed editor (with ACP support) | End-to-end ACP verification | Operator-dependent | — | In-process mock ACP client (test harness) |
| `ZAI_API_KEY` env | Live provider calls (verify step only) | Operator-provided | — | Unit tests use httptest mocks |

**Missing dependencies with no fallback:** none — all code deps are Phase-1-established or stdlib.

**Missing dependencies with fallback:** Zed editor for end-to-end verification. The Phase-2 ACP tests use an in-process mock ACP client (a goroutine speaking the spec framing over `io.Pipe`, grounded in the canonical spec — same pattern as the Phase-0 spike `spikes/03-acp-handshake/`). Live Zed verification is an operator-gated checkpoint (like Phase 1's live Z.ai round-trip).

---

## 15. Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| ACP framing (stdin/stdout JSON-RPC) | `internal/acp` (transport) | — | The framer is the sole stdout writer; transport discipline |
| Method dispatch (initialize/session/* ) | `internal/acp` (server) | — | The method→handler map; per-request goroutine spawning |
| Session lifecycle (new/load/cancel) | `internal/session` (Manager) | `internal/acp` (handler delegates) | Session Core owns the transcript + turn loop (D-17); ACP handler is a thin delegate |
| Transcript append/read | `internal/session` (Manager) | — | Sole owner (SESS-06); one writer, mutex-guarded |
| Lean window projection | `internal/session` (Projector) | — | Reads transcript; mechanical extraction (D-02) |
| Event bus (typed channels) | `internal/event` | — | The streaming spine; producers publish, consumers select |
| Provider streaming (SSE → chunks) | `internal/provider` | — | Additive `Stream` method; respects ctx for cancel |
| Mutability resolution | `internal/toolcat` (formula) + `internal/session` (boundary write) | — | Catalog owns the field; Session Manager writes the boundary |
| Subagent dispatch + recover | `internal/session` (subagent goroutine) | — | PARA-01/03; goroutine-boundary recover |
| Concurrency bounding (semaphore) | `internal/provider` (or `internal/session`) | — | D-12; combined parent + subagent |
| Secret redaction | `internal/redact` (Phase 1) | — | LOG-03; called by transcript writer |
| Tool execution (stubbed) | `internal/session` (ToolExecutor interface) | — | Phase 2 stubs; real exec Phase 4 |

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib `testing`) + `github.com/stretchr/testify` (Phase 1 dep) |
| Config file | none (Go convention; per-package `_test.go`) |
| Quick run command | `go test ./internal/...` |
| Full suite command | `go test ./... -race` (the `-race` flag is load-bearing for the channel/bus/concurrency code) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SESS-01 | Two-layer context: transcript + lean window | unit | `go test ./internal/session -run TestProjector -race` | Wave 0 |
| SESS-02 | Mutating commands always boundaries (config adds only) | unit | `go test ./internal/session -run TestBoundaryEnforcement -race` | Wave 0 |
| SESS-03 | More-mutating-wins formula | unit (TDD) | `go test ./internal/toolcat -run TestEffectiveMutability` | Wave 0 |
| SESS-04 | Boundary resets to lean seed | unit | `go test ./internal/session -run TestLeanSeedAfterBoundary` | Wave 0 |
| SESS-05 | Boundary markers durable (replay moot) | unit | `go test ./internal/session -run TestBoundaryLineDurability` | Wave 0 |
| SESS-06 | Session Manager sole owner | unit | `go test ./internal/session -run TestManagerAppendRead` | Wave 0 |
| ACP-01 | Stdio JSON-RPC server; stdout=frames | integration | `go test ./internal/acp -run TestServerStdoutClean -race` | Wave 0 |
| ACP-02 | Lifecycle methods | integration | `go test ./internal/acp -run TestLifecycle -race` | Wave 0 |
| ACP-03 | (DROPPED) session/load no-op | unit | `go test ./internal/acp -run TestSessionLoadNoOp` | Wave 0 |
| ACP-04 | End-to-end streaming | integration | `go test ./internal/acp -run TestStreamingEndToEnd -race` | Wave 0 |
| ACP-05 | Hand-rolled framing | unit | `go test ./internal/acp -run TestFramer -race` | Wave 0 |
| LOG-02 | Async bus consumer | unit | `go test ./internal/session -run TestTranscriptWriterAsync` | Wave 0 |
| LOG-03 | Secrets redacted | unit | `go test ./internal/session -run TestTranscriptRedaction` | Wave 0 |
| LOG-04 | Reconstruction sufficiency | integration | `go test ./internal/session -run TestTranscriptReconstructsSession` | Wave 0 |
| PARA-01 | Subagent isolated goroutine, restricted tools | unit | `go test ./internal/session -run TestSubagentDispatch -race` | Wave 0 |
| PARA-02 | Results via bus tagged parent-turn-id | unit | `go test ./internal/session -run TestSubagentResultBusTagged` | Wave 0 |
| PARA-03 | Panic recovered → tool-error | unit | `go test ./internal/session -run TestSubagentPanicRecovery -race` | Wave 0 |
| PARA-04 | Provider semaphore bounds concurrency | unit | `go test ./internal/provider -run TestSemaphore -race` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/... -race` (the affected packages)
- **Per wave merge:** `go test ./... -race`
- **Phase gate:** `go test ./... -race` green before `/gsd:verify-work` + the reconstruction-sufficiency test passing.

### Wave 0 Gaps
- [ ] `internal/acp/server_test.go` — uses an in-process mock ACP client over `io.Pipe` (pattern from `spikes/03-acp-handshake/`)
- [ ] `internal/session/manager_test.go` — transcript append/read concurrency
- [ ] `internal/session/projector_test.go` — lean window extraction rules
- [ ] `internal/session/reconstruction_test.go` — LOG-04 end-to-end
- [ ] `internal/session/subagent_test.go` — PARA-01..03 with `-race`
- [ ] `internal/provider/semaphore_test.go` — PARA-04
- [ ] `internal/provider/streaming_test.go` — ACP-04 with httptest mock SSE

*(No framework install needed — Go testing + testify are Phase-1 deps.)*

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (provider auth is env-var API key, Phase 1) | — |
| V3 Session Management | yes (session lifecycle) | Session IDs are UUIDs (`google/uuid`); transcript per-session; cancel drains |
| V4 Access Control | yes (subagent tool restriction, D-10) | Runtime tool-subset enforcement (`RestrictedExecutor`) |
| V5 Input Validation | yes (ACP params parsing) | `json.Unmarshal` into typed structs; reject malformed frames |
| V6 Cryptography | no | — |
| V7 Error Handling | yes (investigate-and-fix-ready, PROJECT.md) | Every error → transcript `error` line; panics recovered (D-13) |
| V8 Data Protection | yes (LOG-03 redaction) | `internal/redact` per-line on transcript |

### Known Threat Patterns for Go stdio JSON-RPC + ACP

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Stdout pollution (malformed frames) | Tampering | Frayer is sole stdout writer; all logs to stderr; transport-discipline test |
| Unbounded memory from slow client | DoS | Bounded channels (D-05) — slow client stalls the turn, not OOM |
| Secret leakage to transcript | Information disclosure | Per-line redaction (LOG-03, `internal/redact`); reconstruction test asserts no `sk-`/`Bearer` |
| Subagent tool escape | Elevation of privilege | Runtime tool restriction (D-10, `RestrictedExecutor`); model sees catalog but executor enforces subset |
| Panic crash | DoS | Goroutine-boundary recover (D-13) + parent/recover; process never crashes |
| Malformed ACP input | Tampering | `json.Unmarshal` errors → JSON-RPC parse-error response; reader continues |

---

## Open Questions (RESOLVED)

1. **Channel buffer sizes (D-05)** — RESOLVED: §2.3. `AgentMessageChunk`=128, others 4–32 per kind. Package-level const in v1.
2. **Task-summary extraction rule + N-char (D-02)** — RESOLVED: §4.3. Last user msg (500 chars) + files touched (20 max) + last assistant msg (300 chars). Mechanical, no model call.
3. **Subagent event-vs-window boundary (D-11)** — RESOLVED: §10.3. Events stream for visibility (bus + transcript); parent's lean window gets only the final `SubagentResult`.
4. **Provider semaphore default (D-12)** — RESOLVED: §11.1. Default 6 (middle of 4–8); `--max-concurrent` flag.
5. **`session/load` behavior (D-09)** — RESOLVED: §5. `loadSession: false` advertised; no-op/error response. No replay.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `anthropic-sdk-go` exposes a streaming API usable for chunk-by-chunk reading | §11.2 | Medium — if the SDK's streaming is awkward, the adapter wraps a custom SSE reader over the raw HTTP body (the SDK's `option.WithResponseInto` gives raw bytes). Phase-1 RESEARCH §6.2 confirmed the SDK is the driver; streaming specifics verified at implementation. |
| A2 | Zed's ACP client is spec-conformant (respects `loadSession: false`) | §5 | Low — Zed is the canonical ACP consumer; if it diverges, the fallback is the new-session behavior. Live verification is operator-gated. |
| A3 | Go's `bufio.Scanner` 1MB buffer is enough for ACP frames | §7.1 | Low — ACP requests are small (prompt text + params); 1MB is generous. If a prompt exceeds 1MB, the buffer size increases. |
| A4 | The Phase-1 event bus API will evolve from handler-slice to channels cleanly | §2 | Low — the `Event` interface + `RequestShaped` migrate; Phase-1 tests that use `Subscribe(kind, handler)` need updating to `Subscribe(kind, buffer)` returning a channel. This is a Phase-2 refactor of `internal/event` (the seed grows). |

---

## Sources

### Primary (HIGH confidence)
- `.planning/research/VERIFIED-FACTS.md` — item #1 (zcode JSONL schema), item #3 (ACP v1 wire shape, Tier-A verified post-spike), item #5 (stdout/stderr transport discipline). Authoritative post-spike source of truth.
- Canonical ACP v1 spec (`https://agentclientprotocol.com/protocol/v1/`) — `transports.md` (newline-delimited framing), `initialization.md` (agentCapabilities field name, protocolVersion integer), `session-setup.md` (session/new, session/load, loadSession gate), `prompt-turn.md` (session/prompt, session/update discriminator, stopReason, session/cancel). Fetched 2026-08-09.
- `.planning/phases/01-mimicry-mvp-north-star-proof/01-{CONTEXT,RESEARCH}.md` + the 6 plans — Phase-1 interface signatures (Provider, Shaper, Bus, loop.Run, redact).
- `.planning/phases/02-session-core-acp-interface/02-CONTEXT.md` — D-01..D-20 (all locked decisions, zero discretion).

### Secondary (MEDIUM confidence)
- `spikes/03-acp-handshake/main.go` — the Phase-0 spike that empirically validated the ACP wire shape (reference for the framer, NOT code to import).

### Tertiary (LOW confidence)
- ACPex overview (`hexdocs.pm/acpex/protocol_overview.html`) — community doc confirming the streaming flow; cross-referenced but not load-bearing (canonical spec is authoritative).

---

## Metadata

**Confidence breakdown:**
- ACP wire shape: HIGH — VERIFIED-FACTS #3 (post-spike) + canonical spec fetched 2026-08-09.
- Event bus + streaming: HIGH — idiomatic Go channels; D-04/D-05 locked.
- Transcript schema: HIGH — D-03/D-20 locked; schema is the planner's synthesis of the locked decisions.
- Subagent mechanics: MEDIUM-HIGH — D-10/D-11/D-13 locked; the event-vs-window boundary (§10.3) is research's resolution of D-11's deferred detail.
- Provider streaming: MEDIUM — A1 assumption; verified at implementation.

**Research date:** 2026-08-09
**Valid until:** 2026-09-08 (30 days; ACP v1 spec is stable, so the wire-shape claims hold indefinitely — the 30-day window is for the Go-ecosystem claims like SDK streaming behavior)
