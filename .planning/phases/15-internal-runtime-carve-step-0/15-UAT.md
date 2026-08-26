---
status: testing
phase: 15-internal-runtime-carve-step-0
source: [15-VERIFICATION.md]
started: 2026-08-26T00:00:00Z
updated: 2026-08-26T00:00:00Z
---

# Phase 15 UAT — Human Verification Tests

## Current Test

number: 1
name: Live-Zed editor-session identity (spawn, streaming, tool diffs, restart replay vs v1.1 close)
expected: |
  Token streaming renders natively (session/update chunks — the chunk-forwarder
  path); tool call diffs render natively and the tool result returns (catalog
  executor path); restart of Zed mid-session replays matching v1.1-close
  behavior (session/load no-op D-09 — a fresh session starts; transcripts
  remain on disk under .ass-guard/). Full checklist: phase-review.md
  "Operator live-Zed checklist".
awaiting: user response

## Tests

### 1. Live-Zed editor-session identity
expected: Spawn ass-guard acp serve from Zed exactly as in daily use; send a prompt; exercise a tool call (file read/edit); restart Zed mid-session. Native streaming, native tool diffs, restart replay matches v1.1-close behavior.
result: [pending]

## Summary

total: 1
passed: 0
issues: 0
pending: 1
skipped: 0
blocked: 0

## Gaps
