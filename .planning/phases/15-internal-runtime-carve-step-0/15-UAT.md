---
status: complete
phase: 15-internal-runtime-carve-step-0
source: [15-VERIFICATION.md]
started: 2026-08-26T00:00:00Z
updated: 2026-08-27T12:18:55Z
---

# Phase 15 UAT — Human Verification Tests

## Current Test

[testing complete]

## Tests

### 1. Live-Zed editor-session identity
expected: Spawn ass-guard acp serve from Zed exactly as in daily use; send a prompt; exercise a tool call (file read/edit); restart Zed mid-session. Native streaming, native tool diffs, restart replay matches v1.1-close behavior.
result: issue
reported: "Attempting to resume the previous session after restart: Zed shows 'Failed to Launch — Loading or resuming sessions is not supported by this agent.' instead of the v1.1-close behavior (fresh session starts, transcripts remain on disk under .ass-guard/)."
severity: blocker

## Summary

total: 1
passed: 0
issues: 1
pending: 0
skipped: 0
blocked: 0

## Gaps

- gap_id: G-15-1
  truth: "Restart of Zed mid-session replays matching v1.1-close behavior (session/load no-op — a fresh session starts; transcripts remain on disk under .ass-guard/)"
  status: failed
  reason: "User reported: Zed shows 'Failed to Launch — Loading or resuming sessions is not supported by this agent.' when resuming a session after restart."
  severity: blocker
  test: 1
  artifacts: []
  missing: []
