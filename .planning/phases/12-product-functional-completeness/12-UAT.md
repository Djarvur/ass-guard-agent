---
status: testing
phase: 12-product-functional-completeness
source: [12-01-SUMMARY.md, 12-02-SUMMARY.md, 12-03-SUMMARY.md, 12-04-SUMMARY.md, 12-05-SUMMARY.md, 12-06-SUMMARY.md, 12-07-SUMMARY.md, 12-08-SUMMARY.md]
started: 2026-08-22T09:00:00Z
updated: 2026-08-22T09:00:00Z
---

## Current Test
<!-- OVERWRITE each test - shows where we are -->

number: 4
name: Agent messaging + session-context reads
expected: |
  SendMessage delivers to a spawned agent's mailbox with an honest ack; unknown agent ids error structurally. ReadSessionContext returns relevant/handoff context from the product's own transcript with documented truncation tail.
awaiting: user response

## Tests

### Section A — user-flow walk-through (phase goal: drive ass-guard from an ACP editor, no dead ends)

### 1. Model asks you a question mid-task (live leg 1)
expected: Question surfaces readable in your editor; your reply lands as the tool result in the captured answered form; model continues the same task with your answer.
result: pass
note: "Operator saw the question but options were NOT clickable — answered by typing the number. Verified by design: ass-guard surfaces asks as plain session/update text chunks (single-line render); ACP session/request_permission is not implemented (captured zcode client form corpus_absent, documented default). Core suspension→reply→resume loop worked."
tested_at: 2026-08-22

### 2. Ask times out when you don't answer (D-01 hands-off leg 2)
expected: With --ask-timeout set (default 10m) and no reply, the timer fires at exactly the configured delay; the model receives the non-answer form ("ask timed out after Ns; proceed or decline on your own") and proceeds or declines autonomously.
result: pass
tested_at: 2026-08-23

### 3. Plan-mode gate + exit approval
expected: When plan mode is ON, mutating tool calls are refused without executing ("Plan mode only allows read-only, non-destructive tools"). ExitPlanMode surfaces the plan as an approval question and suspends the turn; your approval resumes the same turn and execution proceeds.
result: pass
retest_note: "Initial live run FAILED (gap G-12-3 — SetPlanMode never wired in sessionFor). Fixed by 12-09 (commits 00ccc2f RED, e704789 GREEN); the server-level battery TestPlanModeWiring_* now proves enter→marker→gate-refusal→approval→same-turn-resume through the real acp.Server. Automated re-test PASS 2026-08-23; operator live confirmation optional. Ordinary-turn file saves without approval remain correct by design (no-confirmation-tier model)."
tested_at: 2026-08-23

### 4. Agent messaging + session-context reads
expected: SendMessage delivers to a spawned agent's mailbox with an honest ack; unknown agent ids error structurally (no broadcast). ReadSessionContext returns relevant/handoff context from the product's own transcript with documented truncation tail.
result: pass
retest_note: "Live stdio re-test 2026-08-25 PASS after 12-11 fix: ReadSessionContext on the real UUID session id returns the lite-context head + turn excerpts. SendMessage unknown-id structured error unchanged (PASS)."
original_reported: "ReadSessionContext errored on every real session id — UUID vs sess_* vocabulary mismatch."

### 5. Background bash trio
expected: Bash with run_in_background returns immediately with an exec_<uuid> id, the real log path, and read guidance. TaskOutput retrieves running tasks as not_ready (and finished ones ready). TaskStop kills the process group. Over-cap and unknown-id cases error structurally.
result: issue
reported: "stdio-driven live test 2026-08-23. Bash run_in_background: PASS (exec_id + log path + guidance). TaskStop: PASS (group kill + honest ack). TaskOutput: FAILS — model-visible catalog carries it (zcode profile) but the EXECUTION catalog (coretools.json) lacks the entry, so RegisterInteractive silently skips it and every call errors 'tool TaskOutput not in catalog' with a NULL payload line (isError, no output text). The phase's own completeness gate misses it because the gate walks coretools, not the profile decls."
severity: major
root_cause: "Catalog asymmetry: TaskOutput exists in internal/defaults/seed/profiles/zcode/tools.json but not internal/toolcat/coretools.json; RegisterInteractive's skip-if-missing discipline (forward-compat) turned the omission into a guaranteed dead end — the exact 'no implementation yet' class Phase 12 was scoped to kill."

### 6. Cron schedules fire as engine-driven turns
expected: Created schedules persist per-project atomically; due prompts fire serialized behind active turns (queue, no double-fire); missed windows catch up once with a note; fired turns MIRROR to your editor client (WINDOWS #3 fix). No daemon, no port.
result: [pending]

### 7. No dead ends anywhere in the catalog (outcome clause)
expected: Across a real driving session, the model never hits a "no implementation yet" dead end — every non-mcp catalog tool executes for real (completeness gate as permanent regression test proves zero non-mcp gaps).
result: [pending]

### Section B — technical checks (deferred until Section A passes)

### 8. Eval regression net + first-green disposition
expected: The three-layer behavioral-eval net exists (harness, scenario suite w/ pass@k + artifacts, mise gate surface + change-class detector); its opening live runs CAUGHT the flagship pattern-table drift (assertions untouched, evidence preserved), and the first green was discharged via Phase 13 (flagship green ×3: eval-20260820-161520 / 163220 / 205550-k1.json). Includes retroactive operator ratification of the 2026-08-20 manager ruling (first green = Phase 13 exit evidence).
result: [pending]

### 9. Ecosystem discovery + precedence matrix (12-02, all-auto-covered confirm)
expected: installed_plugins.json + cache-layout discovery, five-tier precedence with shadow warnings, agents/hooks/mcp wiring, live operator-plugins evidence, mise ci green. Automated battery passed — confirming summary only.
result: [pending]

### 10. Parity extractor zero-empty re-run (12-03)
expected: Delta-window assembly yields zero empty-expectation turns over live captures (7 turns / 0 empty / 1 counted skip vs BEFORE 53-turns/6-empty); replays run per-turn isolated. Evidence: /tmp/eval-net-evidence/12-03-extract-decomp.txt.
result: [pending]

### 11. Re-capture freshness (12-05)
expected: Capture kit re-proven against current zcode (0.16.3, qualifying primary session sess_6e4b5cc7: 38 request records, 81 tools); config restore diff-verified EMPTY every pass; committed fixture zcode-recaptured-2026-08.json re-pins ACP-07 families (Bash timeout form, persisted-output envelope, answered-ask pairing) or documents corpus-absent hunts honestly.
result: [pending]

### 3b. WebFetch error surfacing (found during operator live session, go-err113 project)
expected: A failing tool call (e.g. WebFetch on an unreachable URL) returns a structured is_error result the model can see and adapt to — retry differently or give up — never silently vanishing.
result: pass
retest_note: "Root cause REPRODUCED and fixed by 12-10 (ddg.go passthrough now returns valid {content} JSON + loud appendToolResultLoud fallback at all bare sites; end-to-end regression pin lands a text/plain fetch transcript-visible). mise ci green."
original_reported: "33 consecutive identical WebFetch calls to https://raw.githubusercontent.com/golangci/example-plugin-module-linter/main/example.go over ~2min10s (21:42:53→21:45:05), each followed by a fresh model round-trip (4–11s each) — zero tool_result lines landed for any of them (56 tool_calls vs 22 tool_results in transcript; 34 unmatched). Model was flying blind, hence the storm. URL reachable from same machine (curl 200 in 0.6s)."
severity: major
root_cause: "Suspected: executor errors return (nil, err) → DispatchBatch marks IsError but the session batch-append path drops or never receives them for backend-delegated tools; needs code-level diagnosis (batch.go executeBounded vs session.go:519 append loop). Second-order effect: no dedup/backoff means blind retries are individually legal but collectively expensive."

## Summary

total: 11
passed: 2
issues: 2
pending: 8
skipped: 0
blocked: 0

## Gaps

- gap_id: G-12-4c
  truth: "ReadSessionContext reads ass-guard's own persisted transcripts for any real session id"
  status: failed
  reason: "Id-vocabulary mismatch — real ids are UUIDs; reader demands sess_* prefix and enumerates only transcript_sess_* files, so no real session is ever readable"
  severity: major
  test: 4
  artifacts:
    - path: "internal/coreexec/messaging.go"
      issue: "sessIDPattern (^sess_…) at :177 vs transcript_<uuid> writes (session/transcript.go:141); ListSessions glob transcript_sess_* at :242"
  missing:
    - "Accept the product's own id vocabulary: validate against UUID form OR drop the prefix check in favor of path-traversal-safe existence check over the enumerated family; add a live-shaped fixture (transcript_<uuid>.jsonl) to the offline tests"

- gap_id: G-12-5a
  truth: "TaskOutput executes for real when the model calls it"
  status: failed
  reason: "Executor + registry exist but the catalog entry does not (coretools.json lacks TaskOutput while the zcode profile declares it); RegisterInteractive silently skips → guaranteed dead end with null-payload error"
  severity: major
  test: 5
  artifacts:
    - path: "internal/toolcat/coretools.json"
      issue: "missing TaskOutput entry (TaskStop present — asymmetry)"
    - path: "internal/coreexec/planmode.go"
      issue: "RegisterInteractive skip-if-missing hid the omission; consider loud stderr warn on skipped stubs"
  missing:
    - "Add the TaskOutput entry to coretools.json matching the captured zcode schema verbatim; extend the completeness gate to diff profile decls vs execution catalog so model-visible-but-unexecutable tools fail CI"

- gap_id: G-12-3
  truth: "EnterPlanMode flips session state ON (plan_mode_enter marker in transcript); mutating calls refused with captured form while ON; ExitPlanMode surfaces plan approval and suspends"
  status: resolved
  resolved_by: 12-09-PLAN.md
  resolved_at: 2026-08-23
  reason: "State never flips ON — s.SetPlanMode never called on serve path; gate dead; ExitPlanMode errors 'not in plan mode'"
  severity: major
  test: 3
  artifacts:
    - path: "cmd/ass-guard/acp_serve.go"
      issue: "sessionFor builds planMode state, passes to RegisterInteractive, never calls s.SetPlanMode(planMode) on the Session literal/setter block (~line 1270-1345)"
  missing:
    - "Call s.SetPlanMode(planMode) beside s.SetAskBroker at the wiring site; add a server-level wiring test asserting Enter→gate-refusal→Exit approval flow through the real acp.Server"
- gap_id: G-12-3b
  truth: "A tool call's result always lands in the transcript and reaches the model, whether success or error"
  status: resolved
  resolved_by: 12-10-PLAN.md
  resolved_at: 2026-08-23
  reason: "33 identical WebFetch calls to one raw.githubusercontent URL over ~2min10s (21:42:53→21:45:05), zero tool_result lines for any of them; model retried blind each round-trip (~280B request growth per step = accumulated unpaired tool_use blocks)"
  severity: major
  test: 6
  root_cause: "REPRODUCED 2026-08-23: DefaultBackend.Fetch non-HTML passthrough returns raw body bytes as json.RawMessage WITHOUT JSON-wrapping (internal/toolexec/ddg.go:138-142 'return body, nil') — for text/plain responses that is INVALID JSON. json.Marshal(Line) then fails ('invalid character p looking for beginning of value' — reproduced standalone), appendLine returns the error, and the caller swallows it via '_ =' at internal/session/session.go:519. Same defect class as 14-05 CR-02 (subagent payloads; fixed there but the web-passthrough path was missed). Successes only landed when content happened to be valid JSON or wrapped."
  artifacts:
    - path: "internal/toolexec/ddg.go"
      issue: "non-HTML Fetch passthrough returns unwrapped raw bytes (line ~141); must marshal as {\"content\": string} like the HTML branch"
    - path: "internal/session/session.go"
      issue: "AppendToolResult errors swallowed by '_ =' (line ~519); silent transcript loss must be loud (stderr log + structured fallback payload)"
    - path: "internal/toolexec/batch.go"
      issue: "no defense-in-depth: executeOne should validate/coerce Output to valid JSON before returning (single chokepoint for all executors)"
  missing:
    - "Fix ddg.go passthrough to return valid JSON ({content: ...}); add Marshal-error fallback in appendLine callers so a bad payload degrades to an error form instead of vanishing; regression test: real DefaultBackend.Fetch on a text/plain URL produces a transcript-visible tool_result"
    - "Optional hardening: repeated-identical-call detection (same tool+input N times in one turn → inject a hint to the model) to bound retry storms"

## Deferred Follow-Ups

- test: 3
  idea: "Implement ACP session/load resume (replay transcript into editor on session/load) — operator overturns v1.0 D-09 cut-line ('looks like I did not understand the question at the time I postponed it; this is must-have'). Zed currently blocks thread resume with 'Loading or resuming sessions is not supported by this agent.' Route: v1.2 pool top priority."
  deferred_at: 2026-08-23
