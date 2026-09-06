---
status: testing
phase: 19-compaction-cache-control
source: [19-VERIFICATION.md]
started: 2026-09-06T20:30:00Z
updated: 2026-09-06T20:30:00Z
---

## Current Test

number: 1
name: Architect decision — same-turn marker carve-out vs next-turn semantics (CR-01)
expected: |
  One of: (a) accept next-turn recovery semantics (criterion 3's retry rescues the session, not the producing turn) and re-pin the criterion wording; or (b) rule the producing turn must be rescuable and schedule the same-turn carve-out (project the retry turn as post-marker via a per-turn override, per 19-REVIEW's fix sketch) with a content-sensitive regression test.
awaiting: user response

## Tests

### 1. Architect decision — same-turn marker carve-out vs next-turn semantics (CR-01 / deferred-items.md)
expected: One of: (a) accept next-turn recovery semantics (criterion 3's retry rescues the session, not the producing turn) and re-pin the criterion wording; or (b) rule the producing turn must be rescuable and schedule the same-turn carve-out (per-turn override projecting the retry turn as post-marker) with a content-sensitive regression test.
result: [pending]

### 2. Operator live-LLM compaction check — run a real long session past the 80% threshold (or force a low threshold via set_config_option), let it compact, continue working
expected: Conversation continues coherently with earlier-turn context preserved via the summary (where v1.1 silently lost turns); note that compaction is currently INVISIBLE to the user (the compacting note degraded to stderr+counter — no session/update status frame exists in the landed v1 vocabulary), which itself may need a UX decision.
result: [pending]

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps

[none yet]
