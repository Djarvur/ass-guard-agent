---
status: testing
phase: 24-documentation-ops-tails
source: [24-VERIFICATION.md]
started: 2026-09-10T18:20:00Z
updated: 2026-09-10T18:20:00Z
---

## Current Test

number: 1
name: Nightly parity CI post-merge activation (24-USER-SETUP.md)
expected: |
  After the branch merges to master: confirm scheduled runs fire at cron 41 3 * * *,
  workflow_dispatch smoke is green, the [self-hosted, zcode] runner is online,
  and issue-on-drift permissions work. All four items tracked in 24-USER-SETUP.md
  (dispatch was GitHub-404 pre-merge — the plan's documented DEFERRED-TO-MERGE contingency).
awaiting: user response

## Tests

### 1. Nightly parity CI post-merge activation (24-USER-SETUP.md)
expected: Post-merge: cron 41 3 * * * fires from master; dispatch smoke green; [self-hosted, zcode] runner online; issue-on-drift permissions verified. Local stand-in evidence (drift-core end-to-end, exit-7 report) already recorded by 24-04.
result: [pending]

### 2. LSP guide live editor leg (docs/lsp-setup.md step 7)
expected: In Zed with the guide's mcp-language-server + gopls setup: prompt an mcp__gopls__* tool call in an ass-guard session; it executes. The scripted dry-run (six mcp__gopls__* tools in the session catalog) is the automated witness.
result: [pending]

### 3. D-14 real-plugin fidelity (operator judgment)
expected: Operator confirms plugins/skills behave unchanged vs their own Claude Code setup (the superpowers 6.1.1 spot-check is the automated anchor; this is the judgment leg).
result: [pending]

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps
