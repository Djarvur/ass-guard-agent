---
phase: 02-session-core-acp-interface
status: passed
verified: 2026-08-09
verifier: inline (zcode runtime, no Agent dispatch)
requirements: [SESS-01, SESS-02, SESS-03, SESS-04, SESS-05, SESS-06, ACP-01, ACP-02, ACP-03, ACP-04, ACP-05, LOG-02, LOG-03, LOG-04, PARA-01, PARA-02, PARA-03, PARA-04]
---

# Phase 2 Verification — Session Core + ACP Interface

## Phase Goal

> A developer can spawn ass-guard from Zed via ACP, send a prompt, watch the
> model's output stream token-by-token, restart Zed, and have the session replay
> correctly — all while the model sees a clean, lean context window at each
> command boundary.

**Verdict: PASSED** (with the documented D-09 scope reduction: NO replay in v1 —
the transcript persists for human investigation; `session/load` is a -32601 no-op).

The end-to-end ACP path is proven autonomously (in-process mock client +
mock streaming provider over io.Pipe — `cmd/ass-guard/integration_test.go` +
`internal/acp/server_test.go`). Live Zed interop is operator-gated (needs a live
Zed editor + ZAI_API_KEY) and is the natural first UAT after this phase ships.

## Requirement coverage

| REQ | Status | Evidence |
|-----|--------|----------|
| SESS-01 two-layer context | ✓ | `session.Manager` (durable transcript) + `session.Projector` (lean window); TestProjector_*; reconstruction test |
| SESS-02 mutating = always boundary | ✓ | `toolcat.IsBoundary` structural floor; TestMaybeAppendBoundary_ReadOnlyNoBoundary + config-adds; TestEffectiveMutability |
| SESS-03 effective-mutability formula | ✓ | `toolcat.EffectiveMutability` (more-mutating wins); TestEffectiveMutabilityFormula |
| SESS-04 boundary resets lean seed | ✓ | `Projector.Project` finds last boundary → zero carry-forward; TestProjector_LeanSeedAfterBoundary |
| SESS-05 boundary markers durable | ✓ | `TypeBoundary` JSONL line (D-08); rehydration moot per D-09 (durability live — TestMaybeAppendBoundary_OnMutatingTool) |
| SESS-06 Session Manager sole owner | ✓ | mutex-guarded Append*; TestConcurrency (100 goroutines → 100 clean lines) |
| ACP-01 stdio JSON-RPC server | ✓ | `acp.Server.Serve` (single reader + per-request goroutine); TestInitialize*, transport-discipline test |
| ACP-02 lifecycle methods | ✓ | initialize/session/new/session/prompt/session/cancel/session/load/logout/session/set_mode handlers |
| ACP-03 session/load replay | DROPPED (D-09) | `session/load` → -32601 no-op; loadSession:false advertised; TestIntegration_SessionLoadNoOp |
| ACP-04 end-to-end streaming | ✓ | `Provider.Stream` → bus → `session/update` (no full-turn buffering); TestIntegration_RealStreamingThroughACP, TestStreamWired |
| ACP-05 framing resolved | ✓ | hand-rolled newline framer (D-14, ~150 LOC, zero deps); TestWriteFrame*, grep go.mod = 0 jsonrpc2 |
| LOG-02 async bus consumer | ✓ | `session.TranscriptWriter.Run` (async, all 7 kinds); TestTranscriptWriterAsync |
| LOG-03 per-line redaction | ✓ | Manager redacts before write; TestRedactionOnRequestShaped (auth [REDACTED], names preserved); reconstruction secret-leak guard |
| LOG-04 reconstruction-sufficiency | ✓ | TestTranscriptReconstructsSession (8 investigation questions, no secret leak) |
| PARA-01 isolated goroutine turn-loops | ✓ | `session.DispatchSubagent`; TestDispatchSubagent_AppendsDispatchLine |
| PARA-02 bus-tagged results | ✓ | SubagentResult event + streamed chunks (parent-turn-id); TestSubagent_StreamsProgressWithParentTurnID |
| PARA-03 panic recovery | ✓ | goroutine-boundary recover → tool-error + error line; TestSubagentPanicRecovery, TestParentPanicRecovery |
| PARA-04 provider semaphore | ✓ | `provider.Semaphore` (default 6); TestSemaphore_AllowsMaxConcurrent, wired in turn loop + subagent |

## Decisions honored (from 02-CONTEXT.md)

- D-09 (NO replay in v1): `session/load` is -32601; transcript is for human investigation. ✓
- D-20 (transcript = audit log): one session-path writer (`TranscriptWriter`, all 7 kinds). ✓
- D-14 (hand-rolled framing ~150 LOC): zero JSON-RPC deps. ✓
- D-04/D-05 (typed channels + bounded buffer + block): expanded bus; TestBackpressure. ✓
- D-17 (Session Core owns transcript+projector+turn loop, one package): `internal/session`. ✓
- D-19 (per-tool Mutability + SESS-03 formula): `toolcat.EffectiveMutability`. ✓
- D-13 (goroutine-boundary recover): parent + subagent recover. ✓
- D-16 (session/cancel = ctx cancel + drain): end-to-end cancel. ✓

## Quality gates

- `go test ./... -race` — PASS (14 packages, full suite green).
- `go build ./...` + `go vet ./...` — clean.
- TDD: every plan committed RED (test) → GREEN (impl). 18 commits across the phase.
- Phase-1 regression: all Phase-1 packages (profile, shaper, provider, toolcat, event, audit, redact, parity, drift, loop) still pass — no regressions.

## Operator-gated follow-ups (NOT phase blockers)

- Live Zed interop UAT (the editor spawn + a real prompt round-trip).
- Live model streaming round-trip (needs ZAI_API_KEY; the streaming path is unit-tested via httptest mock SSE).

## Adaptations from plan assumptions (real interfaces)

- `toolcat.Mutability` was already a typed `int` enum in Phase 1 (not the string type the plan assumed) — Plan 02-03 adapted to the real enum; the `Mutability` field was already on `Tool` + seeded in `coretools.json`.
- `event.Bus.Subscribe(kind, handler)` was replaced by `Subscribe(kind, buffer) <-chan Event` (Plan 02-04); the Phase-1 AuditLogger + loop fakeProvider migrated to the new API.
- `Provider.Stream` returns `(<-chan StreamChunk, error)` (the plan's "plus an assembled Response" was a deadlock-prone signature; the terminal "done" chunk carries FinishReason + Raw).
- The Phase-1 AuditLogger is kept for the NON-SESSION tracer CLI path (main.go); D-20's one-writer property holds for the session path (the TranscriptWriter handles all kinds). Documented in the 02-07 SUMMARY + the TranscriptWriter package doc.
