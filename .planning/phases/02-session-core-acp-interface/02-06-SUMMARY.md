---
phase: 02-session-core-acp-interface
plan: 06
status: complete
requirements: [PARA-01, PARA-02, PARA-03]
autonomous: true
---

# Plan 02-06 — Subagent dispatch: isolated goroutine turn-loops + streamed progress + panic recovery

## Outcome

Added the Task/Agent subagent dispatch to the Session Core. When the model
invokes Task (or Agent), an isolated goroutine runs a nested turn loop with a
restricted tool subset (D-10). Streamed progress is published with parent-turn-id
tagging (PARA-02). A panicking subagent is recovered at the goroutine boundary →
the parent receives an error result + an investigate-and-fix-ready error line;
the process NEVER crashes (PARA-03, D-13).

## What was built

- `internal/session/subagent.go` —
  - `DispatchSubagent(ctx, parentTurnID, toolCallID, prompt, restricted)`: appends
    a `subagent_dispatch` line, spawns an isolated goroutine, waits for the
    result, appends a `subagent_result` line, publishes a `SubagentResult` event.
  - `defaultSubagentRunner.Run`: the nested turn loop — same Manager (subagent-
    tagged lines), same Provider via the semaphore (D-12), RestrictedExecutor over
    the session toolExec. Streamed chunks published with the subagent turnID.
  - `executeRestricted`: runs a tool via `toolcat.NewRestrictedExecutor` (D-10).
  - Goroutine-boundary `recover()` → `SubagentResult` error + investigate-and-fix-
    ready `error` line with stack (D-13).
- `internal/session/session.go` — turn loop detects Task/Agent tool calls and
  dispatches via `DispatchSubagent`; other tools stub-execute. Added `subagentRunner`
  seam (test-injectable) + `toolExec` field.

## Key design decisions

- **DispatchSubagent is synchronous from the parent's view.** The parent's turn
  loop blocks until the subagent completes (it needs the tool_result to continue).
  Streamed progress flows concurrently via the bus (the ACP adapter forwards
  subagent chunks for visibility, PARA-02). The parent's lean window receives
  only the final `subagent_result` (D-11 — intermediates are subagent-tagged and
  excluded from the parent's projection).
- **Goroutine-boundary recover, not in-loop.** The recover wraps the goroutine
  (D-13); a panic becomes a SubagentResult error + an error transcript line with
  a stack. The parent continues (no crash). This is the PROJECT.md investigate-
  and-fix-ready property.
- **RestrictedExecutor enforces the subset at runtime (D-10).** The model sees
  the full catalog (parent mimicry); the subagent's executor enforces the allowed
  subset. Default: Read/Glob/Grep/WebFetch/WebSearch.
- **`subagentRunner` seam.** Production uses `defaultSubagentRunner`; tests inject
  a panicking fake to verify recovery without coupling to the real provider.

## Self-Check

- [x] `go test ./... -race` passes (full suite, no regressions)
- [x] Task tool_call → subagent_dispatch line (PARA-01)
- [x] Subagent streams AgentMessageChunk during dispatch (PARA-02)
- [x] subagent_result line present after dispatch (final result)
- [x] subagent_dispatch records the restricted tool set (D-10)
- [x] Panicking subagent recovered → error line + subagent_result error (PARA-03, D-13)
- [x] Parent continues after subagent panic (no crash)
- [x] TDD: RED 51f0dca → GREEN 322acc6

## Key files

- created: `internal/session/subagent.go`, `internal/session/subagent_test.go`
- modified: `internal/session/session.go` (Task/Agent dispatch branch + subagentRunner seam)
