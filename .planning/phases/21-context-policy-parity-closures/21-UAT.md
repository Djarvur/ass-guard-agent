---
status: testing
phase: 21-context-policy-parity-closures
source: [21-VERIFICATION.md]
started: 2026-09-03T15:55:00Z
updated: 2026-09-03T15:55:00Z
---

## Current Test

number: 1
name: Live-Zed AGENTS.md/CLAUDE.md auto-injection (criterion 3 operator half)
expected: |
  With a fresh binary: a fresh session in a repo containing AGENTS.md (and/or CLAUDE.md) shows the memory content in the agent's context with no manual step; editing the file changes what the next session injects (mtime cache), and the D-08 caps hold for large files.
awaiting: user response

## Tests

### 1. Live-Zed AGENTS.md/CLAUDE.md auto-injection (criterion 3 operator half)
expected: Fresh session shows memory content unaided; edits picked up on next session (mtime cache); caps hold.
result: [pending]

### 2. Thinking stream rendering in Zed
expected: During a turn with a thinking-capable model, thought chunks render live in the editor (agent_thought_chunk frames), byte-identical round-trip into the next outgoing request.
result: [pending]

### 3. @-mention / image paste round-trip in the editor
expected: An @-file mention in a prompt expands to the file content section (Read-rule-gated, provenance line on disk); a pasted image becomes the corresponding image content block in the outgoing request (or degrades loudly on an image-incapable provider); originals preserved on disk.
result: [pending]

### 4. Live hook-deny demonstration
expected: A deny hook configured in .claude/settings.json (project scope) blocks the matching tool call in a real session with the hook's reason; repo-shipped settings never grant allow; a hook-ask on an automation turn declines (never a dialog nobody answers).
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps

[none]
