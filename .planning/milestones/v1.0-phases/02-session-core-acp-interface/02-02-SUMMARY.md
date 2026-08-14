---
phase: 02-session-core-acp-interface
plan: 02
status: complete
requirements: [SESS-01, SESS-04, SESS-05, SESS-06, LOG-02, LOG-03]
autonomous: true
---

# Plan 02-02 — Session Core: transcript + projector + turn loop

## Outcome

Built the Session Core (D-17): a sole-owner transcript Manager (the one audit
artifact per D-20), a lean-window Projector (mechanical extraction, D-01/D-02),
and the D-18 turn loop. The Phase-1 test-harness loop (`internal/loop`) is now
superseded by `Session.Prompt`. Tool execution stays stubbed (D-15); Plan 02-05
wires Provider.Stream + boundaries; Plan 02-06 wires subagent dispatch.

## What was built

- `internal/session/transcript.go` — `Line` (flat JSONL struct, 15 type
  constants), `ContentBlock`, `openTranscript` (creates `.ass-guard/` + the
  D-07 self-gitignore `*\n!.gitignore\n`).
- `internal/session/manager.go` — `Manager` (sole transcript owner, mutex-guarded
  Append* + Read*). Every Append* redacts via the injected Redactor BEFORE the
  write (LOG-03). Methods for all 15 line types + `ReadAll` / `ReadLastBoundary`
  / `ReadSince`.
- `internal/session/projector.go` — `Projector.Project(turnID)`: builds the lean
  window by reading the transcript, finding the last boundary, extracting a
  mechanical summary (last user/assistant excerpts + deduped files-touched),
  and assembling ONE user message (summary + current intent). ZERO carry-forward
  of prior assistant/tool turns as separate messages (D-01).
- `internal/session/session.go` — `Session.Prompt`: the D-18 6-step cycle
  (project → send → emit → tool-loop → end_turn). Parent-turn panic recovery
  (named returns + deferred recover → investigate-and-fix-ready error line + error
  return, D-13). ctx cancel → `canceled` line + stopReason "cancelled" (D-16).
  Semaphore-acquire around Send (PARA-04). Stub tool execution (D-15).
- `internal/session/transcript_writer.go` — async `TranscriptWriter.Run(ctx)`
  (LOG-02): subscribes to RequestShaped / AgentMessageChunk / UsageUpdate on the
  bus and appends each via the Manager. A slow writer never blocks the turn.

## Key design decisions

- **Lean window = one user message.** D-01 says "system + summary + current
  message". The Shaper adds system from the profile separately, so the projector
  returns ONLY the dynamic conversation: one user message combining the
  mechanical summary (bridge context from before the boundary) + the current
  intent. No assistant-role messages are carried forward (zero carry-forward of
  message structure, D-01). A truncated excerpt of the last assistant may appear
  IN the summary text (D-02).
- **Direct writes for read-back lines, async for audit lines.** The Session
  appends user/tool/assistant/canceled/boundary lines directly (synchronous —
  the projector reads them back). The async TranscriptWriter handles
  request_shaped / chunks / usage (audit-only, LOG-02). The Manager serializes
  both via its mutex.
- **Manager is the SOLE file writer.** Both the Session and the TranscriptWriter
  call Manager.Append*; the mutex guarantees no interleaved/corrupted lines
  (SESS-06, TestConcurrency with 100 goroutines).

## Self-Check

- [x] `go test ./... -race` passes (full suite, no regressions)
- [x] D-02 lint clean (`grep .Send|.Stream|Provider projector.go` empty)
- [x] `grep os.Stdout internal/session/` empty (transport discipline)
- [x] Manager concurrency: 100 goroutines → 100 clean JSONL lines
- [x] Redaction: auth value [REDACTED], field name preserved (LOG-03)
- [x] `.ass-guard/.gitignore` = exactly `*\n!.gitignore\n` (D-07)
- [x] Projector: zero carry-forward of assistant-role messages; mechanical summary
- [x] Turn loop: appends user/assistant/tool lines; RequestShaped published
- [x] Cancel → canceled line + stopReason "cancelled" (D-16)
- [x] Parent panic → investigate-and-fix-ready error line + error return (D-13)
- [x] TranscriptWriter async: slow writer does not block the turn (LOG-02)
- [x] TDD: RED d9c1a8e, 08aceca, 17126fe → GREEN 82e4b09, 5c8c7df, feb94d7

## Key files

- created: `internal/session/transcript.go`, `internal/session/manager.go`, `internal/session/manager_test.go`
- created: `internal/session/projector.go`, `internal/session/projector_test.go`
- created: `internal/session/session.go`, `internal/session/session_test.go`
- created: `internal/session/transcript_writer.go`
