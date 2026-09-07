---
status: partial
phase: 20-built-in-commands-skills-per-agent-model
source: [20-06-PLAN.md Task 3, 20-06-VERIFICATION.md]
started: 2026-09-08T00:20:00Z
updated: 2026-09-08T00:20:00Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. Autocomplete lists the full winner set
expected: Typing "/" in Zed lists the builtins (help status cost mcp memory permissions doctor config model clear resume compact init) AND discovered commands/skills/agents — no duplicates, no shadowed entries (D-04).
result: [pending]

### 2. /status answers instantly with no model turn
expected: /status returns the live snapshot immediately (no model-turn spinner); the echo bubble and the output render as two messages; /help's inventory matches the autocomplete set (D-05/D-08).
result: [pending]

### 3. Mid-session pickup without restart
expected: With a session open, creating .claude/commands/extra.md makes "extra" appear in the "/" menu within ~1-2s (debounce 300ms + rescan) and /extra fires; removal reverses both (CMDS-04/D-10).
result: [pending]

### 4. /agent dispatch streams with the resolved-model note
expected: /<agent-name> <task> dispatches the subagent; its output streams into the conversation; a note names the resolved model (frontmatter slug when declared) (D-03/D-16).
result: [pending]

### 5. /cost shows a source note
expected: /cost prints a number with its source note (transcript×cost-table derivation unless a provider usage_endpoint is declared) — never a bare number (D-07/P-20-02).
result: [pending]

### 6. /compact at this milestone
expected: /compact invokes the registered Phase 19 CompactNow machinery (compaction completes; typed focus args recorded with the not-yet-passed note); with the seam nil'd it degrades loudly — never silence (19-D-11).
result: [pending]

### 7. Reserved-name shadowing visible
expected: A discovered commands/model.md (or skill named like a builtin) never fires via slash and logs one shadow-check warning naming the file (D-01).
result: [pending]

## Summary

total: 7
passed: 0
issues: 0
pending: 7
skipped: 0
blocked: 0

## Gaps
