---
phase: 02-session-core-acp-interface
plan: 04
status: complete
requirements: [ACP-04, PARA-04, LOG-02]
autonomous: true
---

# Plan 02-04 — Streaming infrastructure: typed-channel bus + Provider.Stream + semaphore

## Outcome

Expanded the Phase-1 event bus into typed Go channels (D-04) with bounded buffers
+ block (D-05), added the full 7-event catalog, added `Provider.Stream` (SSE →
chunks) for token-by-token streaming, and added the provider semaphore (PARA-04).
The Phase-1 AuditLogger was migrated to the new channel API (it is folded into
the TranscriptWriter in Plan 02-07).

## What was built

- `internal/event/bus.go` — `Bus.Subscribe(kind, buffer) <-chan Event` (typed
  receive channel per subscriber) + `Publish(e)` (fan-out, BLOCKS on full per D-05)
  + `Close()` (closes all channels at shutdown). No-subscriber events drop + log.
- `internal/event/events.go` — full catalog: `RequestShaped` (+TurnID),
  `AgentMessageChunk`, `ToolCall`, `ToolCallUpdate`, `UsageUpdate`,
  `SubagentResult`, `Boundary` + per-kind buffer-size constants (RESEARCH §2.3).
- `internal/provider/streaming.go` — `AnthropicProvider.Stream`: shapes via the
  same Shaper as Send (body is mimicry-faithful), adds `stream:true`, POSTs via
  raw HTTP (so ctx cancellation aborts the in-flight request), parses SSE deltas
  into text / tool_use / usage chunks, emits a terminal "done" chunk carrying the
  FinishReason, then closes the channel. Testable via httptest (no live key).
- `internal/provider/semaphore.go` — `Semaphore` (buffered-channel) +
  `Acquire(ctx)` (respects ctx cancel; a cancelled Acquire does not consume a
  slot) + `Release()` + `DefaultMaxConcurrent = 6`.
- `internal/provider/provider.go` — added `StreamChunk` / `Usage` types and the
  `Stream` method to the `Provider` interface.
- Migration: `internal/audit/audit.go` AuditLogger drains its channel in a
  background goroutine (keeps LOG-01 working against the Phase-2 bus until 02-07
  folds it); OpenAI adapter + loop fakeProvider got Stream stubs.

## Key design decisions

- **Bus Publish blocks sequentially per subscriber.** A full buffer blocks the
  producer (D-05 backpressure) — a stuck consumer stalls the turn rather than
  dropping tokens. This is the intended semantics ("a stuck client stalls the
  turn rather than dropping user-visible tokens or growing memory unbounded").
- **Stream returns `(<-chan StreamChunk, error)`.** The plan's wording "plus an
  assembled Response" was slightly imprecise — returning both a channel and a
  Response would deadlock (the caller must drain the channel while Stream blocks
  to return). Instead the channel emits chunks and a terminal "done" chunk
  carries the FinishReason + assembled Raw bytes, then closes. Idiomatic Go.
- **Stream via raw HTTP, not the SDK streaming client.** Raw HTTP gives full
  control over ctx-cancellation aborting the in-flight request (and is testable
  via httptest without a live key). The body is still shaped via the same Shaper
  as Send (mimicry-faithful); identity headers are set from the profile directly.

## Self-Check

- [x] `go test ./... -race` passes (full suite, no regressions)
- [x] `go build ./...` + `go vet ./...` clean
- [x] Typed channels: Subscribe returns `<-chan Event`; Publish delivers (D-04)
- [x] Backpressure: full buffer blocks Publish (D-05, TestBackpressure)
- [x] Fan-out: 2 subscribers both receive (TestFanOut)
- [x] 7-event catalog with correct Kind() + fields (TestEventCatalog)
- [x] Stream emits text/tool_use/usage chunks + done; respects cancel
- [x] Semaphore: max concurrent respected; Acquire respects cancel (PARA-04)
- [x] Phase-1 Send unchanged (regression); AuditLogger migrated to channel API
- [x] TDD: RED 89f6153, e47ee04 → GREEN 05c8f9f

## Key files

- modified: `internal/event/bus.go` (channel API), `internal/event/bus_test.go`
- created: `internal/event/events.go`
- created: `internal/provider/streaming.go`, `internal/provider/streaming_test.go`
- created: `internal/provider/semaphore.go`, `internal/provider/semaphore_test.go`
- modified: `internal/provider/provider.go` (StreamChunk/Usage/Stream), `internal/provider/openai.go` (Stream stub)
- modified: `internal/audit/audit.go` (channel drain), `internal/loop/loop_test.go` (fakeProvider Stream stub)
