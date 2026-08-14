---
phase: 02-session-core-acp-interface
plan: 05
status: complete
requirements: [ACP-03, ACP-04, PARA-04]
autonomous: true
---

# Plan 02-05 — Integration: real Session Core in ACP + streaming + boundaries + cancel

## Outcome

Wired the real Session Core (02-02), streaming provider + semaphore + expanded bus
(02-04), and mutability boundary (02-03) into the ACP server (02-01). The stub
turn loop is replaced: the ACP server now drives a real `Session.Prompt` that
streams token-by-token. session/load is a no-op (-32601, D-09 — NO replay).
session/cancel cancels the active turn end-to-end (D-16). Mutating tool calls
write boundary lines and reset the lean window.

## What was built

- `internal/session/boundary.go` — `Session.MaybeAppendBoundary`: calls
  `toolcat.IsBoundary` + appends a boundary line with cause
  "mutating-command:<Tool>" or "config-added:<Tool>" (D-08, SESS-02/03).
- `internal/session/session.go` — turn loop switched from Send to `Provider.Stream`
  (`streamAndEmit`): reads chunks, emits AgentMessageChunk/ToolCall/UsageUpdate
  to the bus (D-18 step 4, ACP-04 — NO full-turn buffering), assembles the
  response. After each mutating tool_result, calls MaybeAppendBoundary. ctx
  cancel mid-stream → canceled line + "cancelled" (D-16).
- `internal/acp/server.go` + `handlers.go` — `TurnRunner.Run` now takes a
  `sessionID` (the runner creates/looks up the Session by id).
- `cmd/ass-guard/acp_serve.go` — `sessionTurnRunner`: the production adapter from
  the ACP TurnRunner interface to the real Session Core. It creates a Session per
  sessionId, subscribes a chunk-forwarder (bus AgentMessageChunk → session/update),
  and calls Session.Prompt. Wired into `runACPServe` + the `acp serve` cobra
  command (new `--work-dir` flag for `.ass-guard/` location).
- `cmd/ass-guard/integration_test.go` — full-stack tests: real streaming through
  ACP (initialize → session/new → session/prompt emits ≥1 session/update before
  the response, ACP-04); session/load no-op (-32601, D-09); driven by a
  `mockStreamProvider` (autonomous — no live model call).

## Key design decisions

- **Turn loop uses Stream; panic must be synchronous.** A provider that panics
  inside its Stream GOROUTINE cannot be recovered by Session.Prompt's defer (the
  goroutine is separate). The AnthropicProvider.Stream does its work in the
  caller's goroutine (returns the channel; the drain goroutine only reads). The
  fake/test providers panic in the synchronous part. Production AnthropicProvider
  panics (e.g. nil Shaper) happen before the goroutine spawns → recoverable.
- **sessionTurnRunner forwarder uses a local `promptDone` signal.** After
  Prompt returns, the forwarder drains any buffered chunks then exits — it
  cannot rely on ctx.Done() (ctx is not cancelled on a successful turn) and
  cannot close the shared bus channel.
- **No ACP→session package coupling.** The acp package stays decoupled (takes a
  TurnRunner interface). The sessionTurnRunner adapter lives in cmd/ass-guard,
  which imports both. Integration tests live in cmd/ass-guard.

## Self-Check

- [x] `go test ./... -race` passes (full suite, no regressions)
- [x] Turn loop uses Provider.Stream (D-18 step 4); chunks emitted to the bus
- [x] Mutating tool calls → boundary line; read-only → no boundary (SESS-02/03)
- [x] Config-added boundaries work (WebFetch); catalog floor respected
- [x] session/prompt streams ≥1 session/update before the response (ACP-04)
- [x] session/load returns -32601 (D-09 — NO replay code path)
- [x] session/cancel aborts the turn end-to-end (D-16, verified in 02-01 + here)
- [x] Phase-1 `profile check` still works; `acp serve` is the real entrypoint
- [x] TDD: boundary RED → GREEN; integration tests added

## Key files

- created: `internal/session/boundary.go`, `internal/session/boundary_test.go`
- modified: `internal/session/session.go` (Stream turn loop + MaybeAppendBoundary), `internal/session/session_test.go` (fakeProvider.Stream)
- modified: `internal/acp/server.go` (TurnRunner.Run + sessionID), `internal/acp/handlers.go`, `internal/acp/server_test.go`
- modified: `cmd/ass-guard/acp_serve.go` (sessionTurnRunner + runACPServe wiring), `cmd/ass-guard/acp_serve_test.go`
- created: `cmd/ass-guard/integration_test.go`
