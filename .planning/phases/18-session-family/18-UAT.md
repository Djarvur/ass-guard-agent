---
status: testing
phase: 18-session-family
source: [18-VERIFICATION.md]
started: 2026-09-03T13:05:00Z
updated: 2026-09-03T13:05:00Z
---

## Current Test

number: 1
name: Zed-side session family UAT (18-04 D6)
expected: |
  In Zed with the scratch project (fresh binary): the editor's session list/picker enumerates past sessions; opening/resuming an old session replays the conversation through the same ordered frames as live turns; closing a session mid-turn stops work cleanly (no hang, later prompts get the typed not-found error); deleting a session makes it vanish from the list while the transcript + audit stay on disk (tombstone, never removed).
awaiting: user response

## Tests

### 1. Zed-side session family UAT (18-04 D6)
expected: Editor picker enumerates; resume replays visibly; close mid-turn stops cleanly; delete vanishes from list but artifacts survive on disk.
result: [pending]

### 2. Interactive TTY/ssh picker check (18-06 D-11)
expected: `ass-guard --resume` in a real terminal renders the numbered picker (pipe-safe: numbers + names, stderr), arrow/number selection lands in a fully replayed session; under a pipe (no TTY) it degrades safely (no garbled render).
result: [pending]

### 3. Operator sign-off: 18-06 cwd-scoped --resume resolution deviation
expected: `--resume <id|name>` resolves against the cwd session store only (no cross-project registry search) — the plan's inherited deviation, carried verbatim in 18-06-SUMMARY pending operator acknowledgment.
result: [pending]

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps

[none]
