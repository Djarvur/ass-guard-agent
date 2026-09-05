---
status: testing
phase: 18-session-family
source: [18-VERIFICATION.md]
started: 2026-09-03T13:05:00Z
updated: 2026-09-03T13:05:00Z
---

## Current Test

number: 2
name: Interactive TTY/ssh picker check (18-06 D-11)
expected: |
  `ass-guard --resume` in a real terminal renders the numbered picker (pipe-safe: numbers + names, stderr), arrow/number selection lands in a fully replayed session; under a pipe (no TTY) it degrades safely (no garbled render).
awaiting: user response

## Tests

### 1. Zed-side session family UAT (18-04 D6)
expected: Editor picker enumerates; resume replays visibly; close mid-turn stops cleanly; delete vanishes from list but artifacts survive on disk.
result: pass
note: "Operator 2026-09-06: A1 list+resume+continue PASS, A2 stop-mid-turn PASS (no hang), A3 delete vanishes from list while transcripts+audit remain on disk PASS. (A4/B5 below cover the remaining items.)"

### 2. Interactive TTY/ssh picker check (18-06 D-11)
expected: `ass-guard --resume` in a real terminal renders the numbered picker (pipe-safe: numbers + names, stderr), arrow/number selection lands in a fully replayed session; under a pipe (no TTY) it degrades safely (no garbled render).
result: issue
reported: "`ass-guard --resume` in ~/tmp/perm-uat (two real transcripts on disk, no tombstones): Error: no sessions to resume. Root cause diagnosed from code+disk: Manager.AppendSessionStart (internal/session/manager.go:165) has ZERO production callers — real sessions never write the session_start opener line, so ListSessions' readHeaderOpener (internal/session/list.go:271-306, requires a conforming session_start first line) skips every real transcript as non-conforming. Fixtures hand-write the opener, so the whole list battery is green against a shape real sessions never produce (the G-17-1 class again: tests model a fiction)."
severity: blocker

### 3. Operator sign-off: 18-06 cwd-scoped --resume resolution deviation
expected: `--resume <id|name>` resolves against the cwd session store only (no cross-project registry search) — the plan's inherited deviation, carried verbatim in 18-06-SUMMARY pending operator acknowledgment.
result: pass
note: "Operator 2026-09-06: «устраивает»."

## Summary

total: 3
passed: 2
issues: 1
pending: 0
skipped: 0
blocked: 0

## Gaps

- gap_id: G-18-1
  truth: "ass-guard --resume (and any ListSessions consumer) enumerates the cwd store's real sessions"
  status: failed
  reason: "Real sessions never write the session_start opener (AppendSessionStart has zero production callers — manager.go:165), so readHeaderOpener (list.go:271+) skips every real transcript → 'no sessions to resume' with two live transcripts on disk. Verified: transcripts begin with user_message lines; fixtures (which hand-write session_start) made the battery green against a fictional shape."
  severity: blocker
  test: 2
  artifacts:
    - internal/session/manager.go:165-167
    - internal/session/list.go:244-306
    - ~/tmp/perm-uat/.ass-guard/transcript_15ceb322*.jsonl (first line = user_message)
  missing: []
  fix_direction: "(a) wire AppendSessionStart into real session creation (the transcript's first line, id in Text as the contract expects); (b) make readHeaderOpener tolerant for pre-fix transcripts — first line of ANY known type with a valid timestamp serves as the createdAt fallback (title scan unchanged) so legacy sessions stay enumerable; (c) RED pins: a real-shape transcript (user_message opener) must list, and a session created through the real serve path must produce a session_start first line."
