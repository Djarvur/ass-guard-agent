---
phase: 02-session-core-acp-interface
plan: 01
status: complete
requirements: [ACP-01, ACP-02, ACP-04, ACP-05]
autonomous: true
---

# Plan 02-01 — Tracer: ACP v1 stdio server

## Outcome

Established the thinnest possible end-to-end ACP v1 server proving the transport
seam: a stdio JSON-RPC server with hand-rolled newline-delimited framing, the
canonical method set, and a streaming session/update path from a stub turn loop
to the client. `initialize → session/new → session/prompt` round-trips through an
in-process mock client over io.Pipe, observing ≥1 session/update (agent_message_chunk)
notification before the session/prompt response.

## What was built

- `internal/acp/types.go` — `Message` envelope (JSONRPC, ID *int, Method, Params,
  Result, Error), `RPCError` (implements `error` so handlers can return specific
  JSON-RPC codes, e.g. -32601 for session/load), `ContentBlock`, canonical codes.
- `internal/acp/framer.go` — `writeFrame`/`readFrame` (newline-delimited, rejects
  decoded + marshaled embedded newlines per transports.md) + `Writer` (ASYNC:
  bounded channel + drain goroutine, D-05 backpressure; decouples producers from
  a slow client so the reader goroutine never blocks on output).
- `internal/acp/server.go` — `Server.Serve`: single reader goroutine + per-request
  goroutine dispatch (notifications inline); `recoverDispatch` (per-dispatch
  recover, Pitfall 7); handler-WaitGroup + Writer-Close on shutdown.
- `internal/acp/handlers.go` — `initialize` (agentCapabilities.loadSession:false,
  protocolVersion:1 int, agentInfo, authMethods:[]), `session/new`, `session/prompt`
  (turn runner seam, adapter streams chunks), `session/cancel` (D-16 cancel), `session/load`
  (-32601 no-op per D-09), `logout`, `session/set_mode`.
- `internal/session/doc.go` — Session Core package doc (real code in Plan 02-02).
- `cmd/ass-guard/acp_serve.go` — `acp serve` cobra subcommand (the entrypoint Zed
  spawns); --profile default zcode, --max-concurrent default 6; log→stderr.

## Key design decisions

- **Async stdout Writer (deviation from "mutex-guarded" plan wording, honors D-05).**
  The plan described the Writer as mutex-guarded. A plain mutex around a blocking
  io.Pipe write DEADLOCKS the reader goroutine on a slow client (the parse-error
  response is written from the reader; io.Pipe writes block until read). The Writer
  is now async: a bounded channel (256) + single drain goroutine. This is the D-05
  "bounded buffer + block" boundary applied at the stdout seam. Producers enqueue
  without blocking unless the buffer fills (backpressure). `Close()` flushes.
- **`*RPCError` implements `error`.** Handlers return `*RPCError` for specific codes
  (session/load's -32601); `handleRequest` surfaces it verbatim. Other errors are
  scrubbed via `redact.ScrubError` and wrapped as -32603 (T-02-03).
- **Turn runner seam.** `TurnRunner.Run(ctx, emit, prompt)` is the interface 02-05
  implements with the real Session.Prompt. The tracer uses a stub.

## Self-Check

- [x] `go test ./internal/acp/ ./cmd/ass-guard/ -race` passes
- [x] `go build ./...` + `go vet ./...` clean
- [x] initialize → session/new → session/prompt round-trips over io.Pipe
- [x] ≥1 session/update (agent_message_chunk, no id) before the session/prompt response
- [x] `agentCapabilities` exact field name; integer protocolVersion:1; loadSession:false
- [x] session/cancel (notification) produces NO response frame
- [x] session/load returns -32601 (D-09 no-op; NO replay code path)
- [x] malformed frame → -32700, server keeps reading
- [x] stdout carries ONLY valid frames (transport discipline)
- [x] Phase-1 `profile check` still works (no regression)
- [x] TDD: RED commits 7352547, 11646d7, 9ac0501 → GREEN commits 47321f5, c4f5a8d, fa7faef

## Key files

- created: `internal/acp/types.go`, `internal/acp/framer.go`, `internal/acp/framer_test.go`
- created: `internal/acp/server.go`, `internal/acp/server_test.go`
- created: `internal/acp/handlers.go`
- created: `internal/session/doc.go`
- created: `cmd/ass-guard/acp_serve.go`, `cmd/ass-guard/acp_serve_test.go`
- modified: `cmd/ass-guard/main.go` (registers the `acp` command)
