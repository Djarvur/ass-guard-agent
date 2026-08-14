# Phase 2: Session Core + ACP Interface — Pattern Map

**Mapped:** 2026-08-09
**Phase:** 02 — Session Core + ACP Interface
**Source:** `02-CONTEXT.md`, `02-RESEARCH.md`, `spikes/03-acp-handshake/main.go` (the ACP framer reference), Phase-1 plans (the interface contracts Phase 2 wraps), Phase-1 `01-PATTERNS.md` (conventions C1-C4 carry forward)
**Files analyzed:** 23 new/modified files across `internal/acp`, `internal/session`, `internal/provider`, `internal/event`, `internal/toolcat`, `internal/audit`, `cmd/ass-guard`, `.ass-guard/`
**Analogs found:** 14 / 23 (the rest are greenfield with spike references only)

> **Greenfield caveat (carries from Phase 1).** The production module has only `doc.go` skeletons today (Phase 1 execution is in progress in parallel). PATTERNS.md maps each Phase-2 file to (a) the closest spike reference to study (NOT import — spikes are an isolated module, Phase-0 D-05), (b) the Phase-1 interface contract it wraps, and (c) the project conventions (C1 transport discipline, C2 redaction, C3 env-config, C4 SDK-driven) established by Phase 0. Every pattern is grounded in dated, inspectable source.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/acp/framer.go` | transport | streaming (newline-delimited) | `spikes/03-acp-handshake/main.go:77-149` (`writeFrame`/`readFrame`/`containsDecodedNewline`) | exact |
| `internal/acp/server.go` | server | request-response + notifications | `spikes/03-acp-handshake/main.go` (the mock server goroutine `serveMockAgent`) | role-match |
| `internal/acp/adapter.go` | adapter | event-driven (bus → stdout) | (greenfield — no analog; RESEARCH §8.2) | no analog |
| `internal/acp/handlers.go` | handler | request-response | `spikes/03-acp-handshake/main.go:156-260` (`buildInitializeResult` etc.) | role-match |
| `internal/acp/types.go` | model | — | `spikes/03-acp-handshake/main.go:73-110` (frame shape) + VERIFIED-FACTS #3 | exact |
| `internal/session/manager.go` | service | file-I/O (append) + event-driven | (greenfield; RESEARCH §3.4) | no analog |
| `internal/session/transcript.go` | model | file-I/O | (greenfield; RESEARCH §3) | no analog |
| `internal/session/projector.go` | service | transform (transcript → lean window) | (greenfield; RESEARCH §4) | no analog |
| `internal/session/turn.go` | service | streaming (turn loop) | Phase-1 `internal/loop/loop.go` (`Run` — REPLACED per D-17) | role-match (supersedes) |
| `internal/session/subagent.go` | service | event-driven (goroutine) | (greenfield; RESEARCH §10) | no analog |
| `internal/session/mutability.go` | utility | transform | (greenfield; RESEARCH §6) | no analog |
| `internal/session/reconstruction_test.go` | test | integration | (greenfield; RESEARCH §9) | no analog |
| `internal/event/bus.go` | service | pub-sub (channels) | Phase-1 `internal/event/bus.go` (`Bus.Subscribe`/`Publish` — EXPANDED per D-04) | role-match (supersedes) |
| `internal/provider/semaphore.go` | utility | concurrency | (greenfield; RESEARCH §11.1) | no analog |
| `internal/provider/streaming.go` | service | streaming (SSE → chunks) | Phase-1 `internal/provider/anthropic.go` (`Send` — additive `Stream` method) | role-match (extends) |
| `internal/toolcat/mutability.go` | model | — | Phase-1 `internal/toolcat/types.go` (EXTENDED with `Mutability` field) | role-match (extends) |
| `internal/toolcat/restricted.go` | wrapper | request-response | (greenfield; RESEARCH §10.2 `RestrictedExecutor`) | no analog |
| `internal/audit/transcript_writer.go` | service | event-driven (bus → file) | Phase-1 `internal/audit/audit.go` (`AuditLogger` — FOLDED per D-20) | role-match (supersedes) |
| `cmd/ass-guard/main.go` | entrypoint | CLI | Phase-1 `cmd/ass-guard/main.go` (EXTENDED with `acp serve` subcommand) | role-match (extends) |
| `cmd/ass-guard/acp_serve.go` | command | CLI | Phase-1 `cmd/ass-guard/profile_check.go` (cobra subcommand pattern) | role-match |
| `.ass-guard/.gitignore` | config | — | `.claude/` convention (D-07) | role-match |
| `.ass-guard/<session>.jsonl` | artifact | file-I/O | zcode rollout JSONL (`~/.zcode/cli/rollout/model-io-sess_*.jsonl`, VERIFIED-FACTS #1) | role-match (format analog) |
| `internal/redact/` (Phase-1 package) | utility | transform | (Phase 1 — UNCHANGED, called by transcript writer) | reused |

---

## Pattern Assignments

### `internal/acp/framer.go` (transport, newline-delimited streaming)

**Analog:** `spikes/03-acp-handshake/main.go:77-149` — the spike's `writeFrame`/`readFrame`/`containsDecodedNewline` are the exact pattern. **Study, do NOT import** (spikes are an isolated module, Phase-0 D-05).

**Framing rule (from spike + spec `transports.md`):**
```go
// spikes/03-acp-handshake/main.go:77-105 — writeFrame
func writeFrame(w io.Writer, v any) error {
    if containsDecodedNewline(v) {
        return errors.New("frame value contains an embedded newline — spec forbids it")
    }
    raw, err := json.Marshal(v)
    if err != nil { return fmt.Errorf("marshal frame: %w", err) }
    if bytes.Contains(raw, []byte{'\n'}) {
        return errors.New("marshaled frame contains a raw newline byte — invariant violated")
    }
    if _, err := w.Write(raw); err != nil { return fmt.Errorf("write frame: %w", err) }
    if _, err := w.Write([]byte{'\n'}); err != nil { return fmt.Errorf("write frame newline: %w", err) }
    return nil
}

// spikes/03-acp-handshake/main.go:131-149 — readFrame (use bufio.Reader, NOT Scanner,
// so the 1MB buffer limit is explicit; the spike uses bufio.Reader)
func readFrame(r *bufio.Reader) (map[string]any, error) {
    line, err := r.ReadBytes('\n')
    if err != nil { return nil, err }
    if len(bytes.TrimSpace(line)) == 0 { return nil, errors.New("empty frame line") }
    var f map[string]any
    if err := json.Unmarshal(line, &f); err != nil {
        return nil, fmt.Errorf("unmarshal frame: %w (line=%q)", err, string(line))
    }
    return f, nil
}
```

**Phase-2 adaptation:** the spike's framer uses `map[string]any`; Phase-2 types it into `Message` (RESEARCH §7.1). The mutex guard for concurrent writes (RESEARCH §7.1 `Writer` struct) is NEW — the spike is single-goroutine; Phase-2's server has the reader + per-prompt turn goroutines writing concurrently. Wrap `writeFrame` in a `sync.Mutex`.

**Imports pattern:** `bufio`, `bytes`, `encoding/json`, `errors`, `fmt`, `io`, `sync`.

---

### `internal/acp/server.go` + `handlers.go` (server, request-response + notifications)

**Analog:** `spikes/03-acp-handshake/main.go:156-260` — the spike's `buildInitializeResult`, `buildSessionNewResult`, `buildSessionPromptResponse` factories are the exact field shapes. The spike's `serveMockAgent` goroutine is the dispatch-shape reference.

**Initialize result shape (load-bearing field names — VERIFIED-FACTS #3 Note 3):**
```go
// spikes/03-acp-handshake/main.go:183-198 — buildInitializeResult
func buildInitializeResult(id int, protocolVersion int, agentCapabilities map[string]any) map[string]any {
    return map[string]any{
        "jsonrpc": "2.0",
        "id":      id,
        "result": map[string]any{
            "protocolVersion":   protocolVersion,        // integer 1, NOT string
            "agentCapabilities": agentCapabilities,       // NOT "capabilities"/"serverInfo"
            "agentInfo":         map[string]any{"name": "ass-guard", "title": "ass-guard", "version": "0.1.0"},
            "authMethods":       []any{},
        },
    }
}
```

**Phase-2 differences from the spike:**
- `loadSession: false` (D-09 — NOT `true` as the spike's mock advertised).
- Real dispatch (not a mock) — the `session/prompt` handler spawns a turn goroutine that runs the Session Core.
- `session/cancel` is a NOTIFICATION (no `id`, no response) — the spike didn't exercise this; Phase-2 must handle the `msg.ID == nil` branch (RESEARCH §7.1, Pitfall 2).

**Dispatch shape (RESEARCH §8.1):** single reader goroutine + per-request handler goroutine. The method→handler map is `map[string]Handler`.

---

### `internal/event/bus.go` (service, pub-sub channels — EXPANDED from Phase 1)

**Analog:** Phase-1 `internal/event/bus.go` (the seed from Phase-1 plan 01-05 T1 — `Event` interface, `RequestShaped`, `Bus.Subscribe`/`Publish`). Phase 2 EXPANDS the seed per D-04.

**Phase-1 seed (the contract that migrates):**
```go
// Phase-1 internal/event/bus.go (intended post-Phase-1 state)
type Event interface { Kind() string }
type RequestShaped struct {
    VerbatimRequest json.RawMessage
    Profile         string
    Timestamp       time.Time
}
type Bus struct { /* map[string][]handler */ }
func (b *Bus) Subscribe(kind string, handler func(Event))
func (b *Bus) Publish(e Event)  // goroutine-per-subscriber dispatch
```

**Phase-2 expansion (D-04 — typed channels):**
```go
// Phase-2 internal/event/bus.go
type Bus struct {
    chans map[string][]chan Event  // slice of channels per kind (fan-out)
    mu    sync.RWMutex
}
func (b *Bus) Subscribe(kind string, buffer int) <-chan Event  // returns receive-only channel
func (b *Bus) Publish(e Event) error  // sends to each channel for e.Kind(); blocks if full (D-05)
```

**Migration:** the `RequestShaped` struct gains a `TurnID string` field (additive — RESEARCH §2.2). New event types: `AgentMessageChunk`, `ToolCall`, `ToolCallUpdate`, `UsageUpdate`, `SubagentResult`, `Boundary` (RESEARCH §2.2 catalog). The Phase-1 `AuditLogger` subscriber migrates from `Subscribe(kind, handler)` to `Subscribe(kind, buffer)` returning a channel that it select-loops on.

---

### `internal/session/transcript.go` + `manager.go` (file-I/O, append-only)

**Analog (format):** zcode rollout JSONL (`~/.zcode/cli/rollout/model-io-sess_*.jsonl`, VERIFIED-FACTS #1). The zcode transcript is one `model_io` JSON object per line — Phase-2's transcript follows the same append-only JSONL discipline but with a richer line-type discriminator (RESEARCH §3.1).

**Append pattern (POSIX atomic append):**
```go
// Open with O_APPEND; each Append* call is ONE Write of one JSON line + '\n'.
// The mutex in Manager serializes appends (RESEARCH §3.4).
f, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
line, _ := json.Marshal(entry)
line = append(line, '\n')
m.mu.Lock()
defer m.mu.Unlock()
f.Write(line)
```

**Line-type discriminator (RESEARCH §3.1 — the schema):** every line is `{"type": "<line-type>", ...}`. The 15 line types are enumerated in RESEARCH §3.1. The `error` line type is FIRST-CLASS (investigate-and-fix-ready, PROJECT.md) — every error/panic gets a line with component, message, inputs, recoverable, stack.

**Self-gitignore (D-07):** on first run, create `.ass-guard/.gitignore`:
```
*
!.gitignore
```

---

### `internal/session/projector.go` (transform, transcript → lean window)

**Analog:** NONE in the codebase (greenfield). The pattern is RESEARCH §4 — mechanical extraction (D-02).

**Critical constraint (D-02 lint):** the projector MUST NOT import `internal/provider` or call any model. It is PURE GO mechanical extraction. Test:
```bash
grep -rn "Provider\|Send\|Stream" internal/session/projector.go  # MUST return empty
```

**Extraction rule (RESEARCH §4.3):** last user message (500 chars) + files touched (20 max, from tool_call/tool_result `file_path` args) + last assistant message (300 chars).

---

### `internal/session/turn.go` (streaming turn loop — REPLACES Phase-1 loop)

**Analog:** Phase-1 `internal/loop/loop.go` (`Run(ctx, profile, Provider, prompt) ([]provider.ToolCall, error)` — single-turn). Phase-2's turn loop is the multi-turn streaming replacement per D-17/D-18.

**Phase-1 loop (the contract being replaced):**
```go
// Phase-1 internal/loop/loop.go
func Run(ctx context.Context, prof profile.Profile, p Provider, prompt string) ([]provider.ToolCall, error)
// SINGLE-TURN: builds messages, calls provider.Send, returns tool-calls. No session state.
```

**Phase-2 turn loop (D-18 — the 6-step cycle):**
```go
// Phase-2 internal/session/turn.go
func (s *Session) runTurn(ctx context.Context, userPrompt []ContentBlock) (string, error) {
    turnID := generateTurnID()
    // D-18 step 1: project lean window
    messages, _ := s.projector.Project(turnID)
    // D-18 step 2: shape via Shaper, emit RequestShaped
    // D-18 step 3: send to provider (via semaphore, D-12)
    // D-18 step 4: stream chunks → emit AgentMessageChunk/ToolCall/UsageUpdate
    // D-18 step 5: if tool_calls, execute (STUBBED Phase 2), append, loop
    // D-18 step 6: on end_turn, append assistant response, return stopReason
}
```

**The `context.Context` is load-bearing (D-16):** every select in the turn loop includes `case <-ctx.Done(): return`. `session/cancel` cancels this ctx → turn aborts → response is `{stopReason: "cancelled"}`.

---

### `internal/session/subagent.go` (event-driven goroutine + recover)

**Analog:** NONE (greenfield). The pattern is RESEARCH §10.

**Goroutine-boundary recover (D-13 — Pitfall 7):**
```go
// RESEARCH §10.4 — recover is at the goroutine boundary, NOT inside the turn loop
go func() {
    defer func() {
        if r := recover(); r != nil {
            stack := debug.Stack()
            // publish error result to bus (parent gets tool-error)
            // write error + stack to transcript (investigate-and-fix-ready)
        }
    }()
    // subagent turn loop here
}()
```

**The parent turn loop ALSO wraps in recover** (Pitfall 7 — D-13 scopes to subagents but the parent must not crash either). The ACP reader goroutine wraps in recover too.

---

### `internal/provider/streaming.go` (streaming SSE → chunks — additive to Phase 1)

**Analog:** Phase-1 `internal/provider/anthropic.go` (`Send` — the non-streaming method). Phase-2 ADDS `Stream` (RESEARCH §11.2).

**Phase-1 AnthropicProvider (the contract being extended):**
```go
// Phase-1 internal/provider/anthropic.go (intended post-Phase-1 state)
type AnthropicProvider struct { client anthropic.Client }
func (p *AnthropicProvider) Send(ctx context.Context, prof profile.Profile, messages []Message) (Response, error)
```

**Phase-2 additive method:**
```go
// Phase-2 internal/provider/streaming.go (or anthropic.go)
func (p *AnthropicProvider) Stream(ctx context.Context, prof profile.Profile, messages []Message) (<-chan StreamChunk, Response, error)
```

**SDK pattern (C4 — SDK-driven, not hand-marshaled):** use `anthropic-sdk-go`'s streaming API. The SDK handles SSE parsing; ass-guard reads chunks. Capture raw bytes for the audit log via the same teeing-transport pattern as Phase 1 (spike 02:159-178 `teeTransport`).

---

### `internal/provider/semaphore.go` (concurrency bounding)

**Analog:** NONE (greenfield). The pattern is RESEARCH §11.1 — a buffered channel as a semaphore.

```go
// RESEARCH §11.1
type Semaphore struct { ch chan struct{} }
func (s *Semaphore) Acquire(ctx context.Context) error {
    select {
    case s.ch <- struct{}{}: return nil
    case <-ctx.Done(): return ctx.Err()  // respects cancel (D-16)
    }
}
func (s *Semaphore) Release() { <-s.ch }
```

**Combined parent + subagent (D-12):** ONE semaphore, held by the Session Core, used by both the parent turn loop and every subagent goroutine before calling `provider.Stream`/`Send`.

---

### `internal/toolcat/mutability.go` + `restricted.go` (model + wrapper)

**Analog:** Phase-1 `internal/toolcat/types.go` (`Tool` struct — EXTENDED with `Mutability` field, D-19).

**Phase-1 Tool struct (being extended):**
```go
// Phase-1 internal/toolcat/types.go (intended post-Phase-1 state)
type Tool struct {
    Name        string
    InputSchema json.RawMessage
    // ... other fields
}
```

**Phase-2 extension (D-19):**
```go
type Mutability string
const ( Mutating Mutability = "mutating"; ReadOnly Mutability = "read-only" )
type Tool struct {
    Name        string
    InputSchema json.RawMessage
    Mutability  Mutability  // NEW (D-19)
}
```

**RestrictedExecutor (D-10 — RESEARCH §10.2):** wraps the full executor; checks an allow-list before dispatching. Returns "tool not available" error for restricted tools.

---

### `internal/audit/transcript_writer.go` (event-driven bus → file — FOLDS Phase-1 AuditLogger)

**Analog:** Phase-1 `internal/audit/audit.go` (`AuditLogger` — subscribes to "RequestShaped", writes redacted verbatim to `io.Writer`). Phase-2 FOLDS this into the transcript writer per D-20 (one artifact).

**Phase-1 AuditLogger (the contract being folded):**
```go
// Phase-1 internal/audit/audit.go (intended post-Phase-1 state)
type AuditLogger struct { /* ... */ }
func NewAuditLogger(bus *event.Bus, sink io.Writer) *AuditLogger  // subscribes to "RequestShaped"
// writes {timestamp, profile, request: <redacted>} per line; NEVER stdout
```

**Phase-2 transcript writer (D-20):**
```go
// Phase-2 internal/audit/transcript_writer.go (or internal/session/transcript_writer.go)
type TranscriptWriter struct {
    manager *session.Manager  // the sole transcript owner (SESS-06)
    bus     *event.Bus
}
func NewTranscriptWriter(manager *session.Manager, bus *event.Bus) *TranscriptWriter
// subscribes to ALL 7 event kinds; each handler calls manager.Append* (which redacts per LOG-03)
```

**Redaction pattern (C2 — from spike 02:377-445, promoted to `internal/redact` in Phase 1):** every `Append*` call in the Manager redacts via `internal/redact.Redact` BEFORE writing. The transcript writer does NOT re-redact (the Manager owns redaction at the write boundary).

---

### `cmd/ass-guard/main.go` + `acp_serve.go` (CLI entrypoint — EXTENDS Phase 1)

**Analog:** Phase-1 `cmd/ass-guard/main.go` (cobra root with `profile check` + tracer prompt flag) + `cmd/ass-guard/profile_check.go` (cobra subcommand pattern). Phase-2 ADDS the `acp serve` subcommand.

**Phase-1 cobra pattern (being extended):**
```go
// Phase-1 cmd/ass-guard/main.go + profile_check.go (intended post-Phase-1 state)
// root command + subcommands via cobra
rootCmd := &cobra.Command{Use: "ass-guard"}
rootCmd.AddCommand(profileCheckCmd)  // Phase 1
rootCmd.AddCommand(acpServeCmd)       // Phase 2 NEW
```

**Transport discipline (C1 — load-bearing):** the `acp serve` command redirects `log` output to stderr (`log.SetOutput(os.Stderr)`); stdout is wired to the ACP framer exclusively. The Pitfall 1 test asserts stdout is clean.

---

## Shared Patterns

### C1. Transport discipline — stdout is ACP-only; all diagnostics to stderr
**Source:** PROJECT.md "Transport discipline"; `spikes/02-openai-toolschema/main.go:51-53`; `spikes/05-stdout-collision/` (VERIFIED).
**Apply to:** `internal/acp/server.go`, `cmd/ass-guard/acp_serve.go`, ALL Phase-2 code. `log.SetOutput(os.Stderr)` in `acp serve`. The `internal/acp/framer.go` `Writer` is the ONLY stdout writer.
**Test:** capture `os.Stdout` during a test session; assert byte-empty or byte-equal to canned ACP frames (the spike-05 pattern).

### C2. Secret redaction at every output boundary (LOG-03)
**Source:** `spikes/02-openai-toolschema/main.go:377-445`; `internal/redact` (Phase 1, promoted).
**Apply to:** `internal/session/manager.go` (every `Append*` redacts via `internal/redact.Redact`), `internal/acp/handlers.go` (error responses scrubbed via `redact.ScrubError`).
**Pattern:** JSON-tree walker replacing secret VALUES, preserving field NAMES + the 12 identity header names.

### C4. SDK-driven request/response (not hand-marshaled)
**Source:** `spikes/02-openai-toolschema/main.go:114-141`; Phase-1 Shaper.
**Apply to:** `internal/provider/streaming.go` — use `anthropic-sdk-go` streaming types; do not hand-build SSE parsing. The SDK serializes; ass-guard reads chunks.

### C5. Goroutine-boundary recover (investigate-and-fix-ready)
**Source:** PROJECT.md "Investigate-and-fix-ready logging (must-have)"; D-13.
**Apply to:** `internal/session/subagent.go` (subagent goroutine), `internal/session/turn.go` (parent turn goroutine), `internal/acp/server.go` (reader goroutine).
**Pattern:** `defer func() { if r := recover(); r != nil { ... write error+stack to transcript ... } }()`. Process NEVER crashes; every panic is an `error` transcript line.

### C6. context.Context propagation (D-16 cancel)
**Source:** D-16.
**Apply to:** every select in the turn loop, the provider `Stream` call, the semaphore `Acquire`. `case <-ctx.Done(): return` everywhere. `session/cancel` cancels the active turn's ctx.

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/acp/adapter.go` | adapter | event-driven (bus → stdout) | No existing event-bus-to-protocol adapter; greenfield per RESEARCH §8.2 |
| `internal/session/manager.go` | service | file-I/O + event-driven | No existing session/transcript manager; greenfield per RESEARCH §3.4 |
| `internal/session/transcript.go` | model | file-I/O | Greenfield schema (RESEARCH §3); zcode JSONL is a format analog, not a code analog |
| `internal/session/projector.go` | service | transform | Greenfield (RESEARCH §4); no existing context-projection logic |
| `internal/session/mutability.go` | utility | transform | Greenfield (RESEARCH §6); the SESS-03 formula is new |
| `internal/session/subagent.go` | service | event-driven | Greenfield (RESEARCH §10); no existing goroutine-isolation pattern |
| `internal/session/reconstruction_test.go` | test | integration | Greenfield (RESEARCH §9); the LOG-04 test is new |
| `internal/provider/semaphore.go` | utility | concurrency | Greenfield (RESEARCH §11.1); trivial buffered-channel pattern |
| `internal/toolcat/restricted.go` | wrapper | request-response | Greenfield (RESEARCH §10.2 `RestrictedExecutor`) |

**Planner guidance:** for these greenfield files, use RESEARCH.md patterns (the code sketches in §3, §4, §6, §8.2, §9, §10, §11.1) as the reference — they are prescriptive, not exploratory.

---

## Metadata

**Analog search scope:** `spikes/03-acp-handshake/`, `spikes/02-openai-toolschema/`, `spikes/05-stdout-collision/`, Phase-1 plan files (`01-01-PLAN.md` through `01-06-PLAN.md`), Phase-1 `01-PATTERNS.md`, VERIFIED-FACTS.md.
**Files scanned:** 11 source files (4 spikes + 6 Phase-1 plans + Phase-1 PATTERNS + VERIFIED-FACTS).
**Pattern extraction date:** 2026-08-09.
