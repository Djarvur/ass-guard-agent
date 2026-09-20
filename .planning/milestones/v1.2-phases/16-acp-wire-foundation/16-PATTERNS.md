# Phase 16: ACP Wire Foundation - Pattern Map

**Mapped:** 2026-08-27
**Files analyzed:** 10 (new/modified)
**Analogs found:** 9 / 10

**Carve status note:** the Phase-15 carve is LANDED in the working tree — `internal/runtime/runtime.go` holds `emitFor` (line 182, `SetEmitter` at 1327-1329) and `internal/acpserve/acp_serve.go` is the composition root. All analogs below are against target-state paths.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/acp/emitter.go` (NEW) | component (concurrency) | streaming | `internal/acp/framer.go` Writer (lines 90-149) | exact (same package, composed chokepoint) |
| `internal/acp/request_registry.go` (NEW) | service (state registry) | request-response | `internal/acp/server.go` sessionState + handlers map (lines 58-91) | role-match (new concurrent-resolution concern) |
| `internal/acp/server.go` (MOD: response interception) | controller | request-response | itself — `Serve` dispatch (lines 180-196) | exact (edit in place) |
| `internal/acp/handlers.go` (MOD: caps + set_config_option) | controller | request-response | `handleInitialize` (lines 36-58) + `handleSessionSetMode` (218-222) | exact |
| `internal/acp/types.go` (MOD: wire frames) | model | transform | `Message` struct + RPCError (lines 33-53) | exact |
| `internal/acpserve/acp_serve.go` (MOD: emitter wiring) | config/composition | event-driven | `Run` lines 200-215 (srv/emitFor junction) | exact |
| `internal/session/transcript.go` (MOD: 3 new kinds) | model | file-I/O (append) | existing kind block (lines 18-49) + Line envelope (61-115) | exact |
| `internal/session/manager.go` (MOD: unredacted path + 2 appenders) | service | file-I/O | `appendLine` (lines 76-103) | exact (explicit divergence point) |
| `internal/modelrouting/config_write.go` or `providerfactory/` (NEW: persist-then-apply writer) | service | file-I/O | `modelrouting/load.go` Load/deepMerge (43-64, 97) + `session` 0600 constants (transcript.go:11-13) | role-match (writer does not exist) |
| `internal/acp/emitter_test.go` + `request_registry_test.go` + session tests (NEW) | test | — | `internal/acp/serve_test.go` / `internal/acpserve/serve_test.go` | exact (package conventions) |

## Pattern Assignments

### `internal/acp/emitter.go` — TurnEmitter (NEW; streaming)

**Analog:** `internal/acp/framer.go` (Writer). Same package, same stdlib-only concurrency style, same doc-comment discipline (decision tags like "(D-05 backpressure boundary)").

**Core Writer pattern to compose, not duplicate** (framer.go:90-131):
```go
type Writer struct {
	w      io.Writer
	ch     chan *Message
	wg     sync.WaitGroup
	mu     sync.Mutex // guards Close once
	closed bool
}
func newWriter(w io.Writer) *Writer {
	wtr := &Writer{w: w, ch: make(chan *Message, writeBuffer)}
	wtr.wg.Add(1)
	go wtr.drain()
	return wtr
}
const writeBuffer = 256
```
- The emitter's drain goroutine becomes the ONLY producer of session/update frames into `Writer.Write` (one frame in flight max — Pitfall 2's priority-inversion guard).
- Copy the bounded-channel-blocks-producer comment style verbatim (framer.go:114-121) for both lanes; add the D-01 "nothing ever dropped" invariant to the doc comment.
- Copy the Close discipline (framer.go:133-149): after Close, Write blocks forever — the emitter must stop before server close (Pitfall 8).
- Use the RESEARCH Pattern-1 nested-select sketch for fg-preempts/bg-FIFO; stall detector = time.Ticker watermark + stderr log + counter, never disconnect (D-03).

**Emitter session handle** — extend, do not replace, `Server.Emitter` (server.go:143-145) and `adapter` (server.go:308-329); new frame methods mirror `adapter.AgentMessageChunk`'s map-based update construction with `//nolint:tagliatelle` camelCase wire names.

---

### `internal/acp/request_registry.go` (NEW; request-response)

**Analog:** `internal/acp/server.go` — sessionState concurrency style (own mutex per entity, lines 71-91) and `internal/session/ask.go:36` for the HUMAN-ASK timeout constant:

```go
// internal/session/ask.go:36
const DefaultAskTimeout = 10 * time.Minute
```

**Registry keys:** D-15 UUID strings — REUSE `newSessionID` (handlers.go:227-239), the in-repo crypto/rand v4 pattern (generalize or rename to `uuidV4`; do not add google/uuid).

**Resolution channel discipline** (from Server's handlerWG style): `ch chan resolution` buffered(1) so `Deliver` never blocks — registry mutex is separate from the Writer, never held across enqueue (Pitfall 3). Timeout classes: FAST-CONTROL ~10s / HUMAN-ASK mirroring DefaultAskTimeout. -32800 responses resolve immediately as cancelled, no retry.

---

### `internal/acp/server.go` — response interception (MOD)

**Analog:** itself. Insert in `Serve` (server.go:180-196) BEFORE the `msg.ID == nil` branch:

```go
// server.go:180-187 — current dispatch, edit point
if msg.ID == nil {
	s.safeDispatchInline(ctx, msg)
	continue
}
```
Add RESEARCH Pattern-2 shim: `if msg.ID != nil && msg.Method == "" && (msg.Result != nil || msg.Error != nil) { s.registry.Deliver(...); continue }` — a response gets NO response. `Message` (types.go:33-40) already models this (`Method` has omitempty, `ID` is `json.RawMessage`).

**Error scrubbing stays:** `redact.ScrubError` on all error responses (server.go:238, 245).

---

### `internal/acp/handlers.go` — capabilities + set_config_option (MOD)

**Analog:** `handleInitialize` (handlers.go:36-58) — the initializeResponse struct with `//nolint:tagliatelle` per ACP wire field is the template for every new response shape (loadSession, sessionCapabilities, configOptions). `handleSessionSetMode` (218-222) is the no-op-with-empty-result template for advertised-but-unhandled config keys (D-05 pending-handler no-ops get a log line added).

**Params parsing pattern** (handlers.go:66-74): anonymous struct + tolerant `_ = json.Unmarshal` — reuse for set_config_option with D-09 typed rejection via `&RPCError{Code: CodeInvalidParams, ...}` (typed-error convention, server.go:231-236).

---

### `internal/acp/types.go` — wire frames (MOD)

**Analog:** `Message`/`RPCError`/`ContentBlock` (types.go:33-61). Every new frame struct (ToolCallFrame, PlanFrame, ThoughtChunkFrame, ConfigOptionFrame, DiffContent) uses RESEARCH's verbatim schema-v1 field names with `//nolint:tagliatelle` per field. Pin v1 names (`id` not `configId`, `plan` not `plan_update` — Pitfall 7).

---

### `internal/acpserve/acp_serve.go` — wiring (MOD)

**Analog:** itself, `Run` lines 200-215 — the exact junction:
```go
srv := acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner))   // :200
runner.SetEmitter(srv.Emitter) // WINDOWS #3                          // :214
```
Construct the TurnEmitter between these (or swap `srv.Emitter` for an emitter-backed view) so both the runner's emitFor and background emitters go through the lanes. Degrade-loudly composition style: failures log to stderr and continue (see startAuditMirror, lines 32-58).

---

### `internal/session/transcript.go` — 3 new kinds (MOD)

**Analog:** the existing kind block (transcript.go:18-49) — append `TypeRawThinking`/`TypeLocalCommand`/`TypeCompaction` in the same const block with the same doc-comment style (decision + payload contract per kind). New Line fields (boundary id, pre/post pointers, resolution chain) follow the field-grouping-with-comment convention of `Line` (transcript.go:61-115), `json.RawMessage` for unparsed payloads, on-disk camelCase with `//nolint:tagliatelle`.

**0600 constants already exist:** `filePermOwner = 0o600` (transcript.go:11) — reference, don't redefine.

---

### `internal/session/manager.go` — unredacted append path (MOD)

**Analog:** `appendLine` (manager.go:76-103). D-23's `appendLineUnredacted` is a deliberate near-copy MINUS lines 82-89 (the Redact call):
```go
// manager.go:82-89 — the block the thinking path must NOT share
red, err := m.redactor.Redact(raw)
if err != nil {
	rawErr := errors.New(string(raw))
	red = []byte(m.redactor.ScrubError(rawErr))
}
```
No shared newline helper (Pitfall 5's warning sign). Same mutex/file-nil-check tail. `AppendRawThinking`/`AppendLocalCommand`/`AppendCompaction` follow the `AppendSessionStart` one-liner style (manager.go:108-110). `json.Marshal` of `json.RawMessage` emits original bytes verbatim — byte identity survives.

---

### Config writer (NEW: `internal/modelrouting/config_write.go` or providerfactory)

**Analog:** `modelrouting/load.go` (`Load` at :43, `deepMerge` at :97 — the writer must round-trip what Load reads) + `providerfactory.GlobalConfigPath()`/`ProjectConfigPath(workDir)` (provider_factory.go:20/:34) for layer addressing. 0600 + atomic temp+rename per session artifact conventions (`filePermOwner`, transcript.go:11). **No analog exists for the write half** — persist-then-apply ordering (D-07) is new design; YAML marshal must be inverse-compatible with deepMerge.

---

### Tests (NEW)

**Analog:** `internal/acpserve/serve_test.go` / package-local `_test.go` convention (per-package, stdlib testing, table tests, `-race` in `mise ci`). Fake-slow-writer pattern: replace the io.Writer under `newWriter` with a controllable blocker (Writer takes any io.Writer — framer.go:100). Zero-redactor-call assertion: inject a counting Redactor fake into `NewManager` (manager.go:39).

## Shared Patterns

### Transport discipline (stdout = frames only, stderr = diagnostics)
**Source:** types.go package doc (lines 1-15) + server.go:104-106 (`WithLogger`, "must NEVER be stdout"). Apply to: emitter stall logs, registry fallback logs, all D-16 telemetry counters.

### Typed JSON-RPC errors
**Source:** `RPCError` implementing error (types.go:42-53) + `errors.As` dispatch in `handleRequest` (server.go:228-240). Apply to: D-09 config rejection (CodeInvalidParams + violation detail in Data), D-07 write failure. Handler errors are scrubbed via `redact.ScrubError` before the wire.

### UUID v4 generation (no deps)
**Source:** `newSessionID` (handlers.go:227-239) — crypto/rand, version/variant bits, panics on entropy failure. Apply to: request ids (D-15), messageId, toolCallId, boundary ids.

### Degrade loudly, never crash/refuse
**Source:** acp_serve.go startup cascade (engine failure :191-198, schedule store :206-212, audit mirror :32-58 — every failure logs + continues). Apply to: stall detector (log + counter, no disconnect), capability degradation (sticky, logged), probe fallback.

### CSPRNG-id session state with own mutex
**Source:** `sessionState` + `setCancel`/`cancelTurn` (server.go:69-91). Apply to: registry pending entries, per-session capability cache (D-13/D-18).

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/modelrouting/config_write.go` (persist-then-apply writer) | service | file-I/O | Only the READ side exists (Load/deepMerge); the writer is new design per D-07 — use RESEARCH.md Pitfall 6 + atomic temp+rename |

## Metadata

**Analog search scope:** internal/acp, internal/acpserve, internal/runtime, internal/session, internal/modelrouting, internal/providerfactory
**Files read:** framer.go, server.go, types.go, handlers.go, acp_serve.go, transcript.go, manager.go (+ targeted greps: runtime.go emitFor, load.go, provider_factory.go, ask.go)
**Pattern extraction date:** 2026-08-27
