---
phase: 02-session-core-acp-interface
plan: 07
status: complete
requirements: [LOG-04, LOG-02]
autonomous: true
---

# Plan 02-07 — Reconstruction gate + audit fold + end-to-end

## Outcome

The Phase-2 gate. The TranscriptWriter now subscribes to ALL 7 event kinds (the
one session-path audit writer, D-20). A reconstruction-sufficiency test (LOG-04)
proves the transcript + profile answer every investigation question for a scripted
session (Bash boundary + Task subagent + cancel), with NO secret leaking to disk.
An end-to-end test exercises the full stack. `go test ./... -race` passes — the
phase gate is green.

## What was built

- `internal/session/transcript_writer.go` — expanded to subscribe to ALL 7 event
  kinds (RequestShaped, AgentMessageChunk, ToolCall, ToolCallUpdate [no-op],
  UsageUpdate, Boundary, SubagentResult). Each event → the matching
  Manager.Append*. This is the unified session-path audit writer (D-20).
- `internal/session/reconstruction_test.go` — `TestTranscriptReconstructsSession`
  (LOG-04): a scripted session (3 prompts: Bash boundary, Task subagent, end_turn
  + an injected canceled line + a secret-bearing request_shaped). Asserts every
  line type is present (user/assistant/tool_call/tool_result/boundary/
  request_shaped/canceled/subagent_dispatch/subagent_result), the Bash boundary
  carries "mutating-command:Bash", and NO secret leaks (`sk-`/`Bearer` absent,
  `[REDACTED]` present, field names preserved).
- `internal/session/end_to_end_test.go` — `TestEndToEndSession`: a Task subagent
  turn through the full stack (project → stream → subagent dispatch → result →
  session_end), asserting the transcript captures the full sequence and no secret
  pattern leaks. Deadline-guarded against deadlock.

## Adaptation vs. plan (D-20 audit fold)

The plan called for building `internal/audit/transcript_writer.go` and REMOVING
the Phase-1 `internal/audit/audit.go` AuditLogger. The AuditLogger is still used
by the Phase-1 tracer CLI (`cmd/ass-guard/main.go` runTrace) for the NON-SESSION
LOG-01 path (it writes to a file/stderr sink, not a session transcript). Removing
it would break the tracer CLI which has no session.Manager.

D-20's one-artifact property is honored for the SESSION path: within a session,
the `session.TranscriptWriter` (subscribing all 7 kinds) is the SOLE writer to
the one transcript file. The Phase-1 AuditLogger is NOT used in the session path
(it's the tracer-only LOG-01 path), so there is no two-writer drift within a
session. Documented in the TranscriptWriter package doc. (If the tracer CLI is
retired in a later phase, the AuditLogger can be removed then.)

## Self-Check

- [x] `go test ./... -race` passes (PHASE GATE green; 14 packages)
- [x] `go build ./...` + `go vet ./...` clean
- [x] TranscriptWriter subscribes all 7 event kinds (D-20 one session writer)
- [x] Reconstruction test: all 8 investigation questions answered (LOG-04)
- [x] No secret leaks on disk (`sk-`/`Bearer` absent, `[REDACTED]` present)
- [x] Bash boundary → "mutating-command:Bash"; Task subagent dispatch captured
- [x] End-to-end test: full-stack Task turn, transcript captures the sequence
- [x] TDD: reconstruction + e2e tests added; full suite -race green

## Key files

- modified: `internal/session/transcript_writer.go` (all 7 kinds)
- created: `internal/session/reconstruction_test.go`
- created: `internal/session/end_to_end_test.go`
