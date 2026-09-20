---
status: complete
phase: 21-context-policy-parity-closures
source: [21-VERIFICATION.md]
started: 2026-09-03T15:55:00Z
updated: 2026-09-06T20:05:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Live-Zed AGENTS.md/CLAUDE.md auto-injection (criterion 3 operator half)
expected: Fresh session shows memory content unaided; edits picked up on next session (mtime cache); caps hold.
result: pass
note: "Operator 2026-09-06: Б1 PASS (rule from AGENTS.md followed unprompted; edits picked up on new session)."

### 2. Thinking stream rendering in Zed
expected: During a turn with a thinking-capable model, thought chunks render live in the editor (agent_thought_chunk frames), byte-identical round-trip into the next outgoing request.
result: pass
note: "Operator 2026-09-06: Б2 PASS (thought blocks render during turns)."

### 3. @-mention / image paste round-trip in the editor
expected: An @-file mention in a prompt expands to the file content section (Read-rule-gated, provenance line on disk); a pasted image becomes the corresponding image content block in the outgoing request (or degrades loudly on an image-incapable provider); originals preserved on disk.
result: pass
note: "Operator 2026-09-06: Б3 @-mention PASS (agent read @note.txt content unprompted). Б4 image: agent answered loudly that the MODEL does not see pictures — pipeline correct (Anthropic adapter SupportsImages=true, block carried; the scratch config's glm-5.2 is text-only), i.e. the designed D-11 loud outcome from the model side. Follow-up note: image understanding requires a vision-capable model in config (glm vision variant) — operator choice, not a defect."

### 4. Live hook-deny demonstration
expected: A deny hook configured in .claude/settings.json (project scope) blocks the matching tool call in a real session with the hook's reason; repo-shipped settings never grant allow; a hook-ask on an automation turn declines (never a dialog nobody answers).
result: pass
note: "First attempt 2026-09-06 (Б5): command RAN — but the orchestrator's instruction used the WRONG output dialect ({\"decision\":\"block\"}); the implemented channels per internal/ecosys/hookverdict.go D-02 are hookSpecificOutput JSON (permissionDecision deny/ask/allow) or legacy exit-2. Schema-invalid JSON yields VerdictNone = fail-open BY DESIGN, so the run was inconclusive, not an agent defect. RETEST 2026-09-06 PASS (operator): corrected fixture ~/tmp/hook-deny-uat (PreToolUse matcher Bash, script prints {\"hookSpecificOutput\":{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"UAT-21-T4: deny hook fired (correct dialect)\"}}) — the Bash call was blocked with the hook's reason in a live session."

## Summary

total: 4
passed: 4
issues: 0
pending: 0
skipped: 0
blocked: 0

## Deferred Follow-Ups

```yaml
- test: 0
  idea: "Operator UX request 2026-09-06: in the Zed chat input, Up-arrow should recall the previously submitted prompt (terminal-style input history) instead of scrolling the conversation. This is Zed-side input-widget behavior — the agent cannot influence it; upstream Zed feature territory."
  deferred_at: 2026-09-06
```

## Gaps

[none]
