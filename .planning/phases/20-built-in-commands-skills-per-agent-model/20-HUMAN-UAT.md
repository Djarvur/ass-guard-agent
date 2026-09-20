---
status: complete
phase: 20-built-in-commands-skills-per-agent-model
source: [20-06-PLAN.md Task 3, 20-06-VERIFICATION.md]
started: 2026-09-08T00:20:00Z
updated: 2026-09-20T18:30:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Autocomplete lists the full winner set
expected: Typing "/" in Zed lists the builtins (help status cost mcp memory permissions doctor config model clear resume compact init) AND discovered commands/skills/agents — no duplicates, no shadowed entries (D-04).
result: pass
resolution: |
  Wire-verified 2026-09-20 (operator-elected automated run): the available_commands_update
  on session/new carries EXACTLY the 14 builtins (D-04's 13 + undo, added by Phase 23 by
  design) + the discovered agent demo-agent — 15 entries, zero duplicates, 15/15 with
  descriptions. Discovered-file coverage: command files exercised in test 3, agents in
  tests 1/4; no skills present in the scratch project (skills-kind discovery rides the
  same chain mechanics; model-facing injection proven by 24-UAT's D-14 leg). Method: the
  real binary rebuilt from HEAD (ed8ac66), driven over ACP stdio
  (initialize/session/new/session/prompt) on a scratch project against a scripted SSE
  provider stub. Note: the live set is a strict superset of the D-04-era list via Phase
  23's /undo — expected evolution, not drift. The Zed "/" popup renders this wire payload
  (visual leg available to the operator on request).

### 2. /status answers instantly with no model turn
expected: /status returns the live snapshot immediately (no model-turn spinner); the echo bubble and the output render as two messages; /help's inventory matches the autocomplete set (D-05/D-08).
result: pass
resolution: |
  Wire-verified 2026-09-20: /status answers in 143-150ms with echo (user_message_chunk)
  + output (agent_message_chunk) as two messages, stopReason end_turn, ZERO provider
  requests (stub call counter = 0) — control-plane-fast, no model turn. /help renders
  "Commands (chain winners ...)" and its inventory lists every one of the 15 advertised
  names (bidirectional match), also with zero provider calls.

### 3. Mid-session pickup without restart
expected: With a session open, creating .claude/commands/extra.md makes "extra" appear in the "/" menu within ~1-2s (debounce 300ms + rescan) and /extra fires; removal reverses both (CMDS-04/D-10).
result: pass
resolution: |
  Wire-verified 2026-09-20: creating .claude/commands/extra.md re-fires
  available_commands_update including "extra" in 0.30s (inside the 300ms-debounce +
  rescan window); /extra focus fires ONE provider turn whose request body carries the
  expansion "Extra body: focus" ($ARGUMENTS substituted). Removing the file reverses the
  advertisement in 0.30s (set identical to pre-create); /extra gone then reaches the
  model RAW in the request's current-request section (no expansion) — resolution
  reversal proven on the wire.

### 4. /agent dispatch streams with the resolved-model note
expected: /<agent-name> <task> dispatches the subagent; its output streams into the conversation; a note names the resolved model (frontmatter slug when declared) (D-03/D-16).
result: pass
resolution: |
  Wire-verified 2026-09-20: /demo-agent find the entrypoint streams the subagent's
  chunks into the conversation (agent_message_chunk frames), the live note reads
  "subagent routed to GLM-5.3 (frontmatter)", the stubbed provider received the
  subagent request with model GLM-5.3 + the agent's system prompt + typed args, and the
  transcript gains subagent_dispatch (resolvedModel GLM-5.3) + local_command
  (demo-agent) lines. Zero parent-model turns.

### 5. /cost shows a source note
expected: /cost prints a number with its source note (transcript×cost-table derivation unless a provider usage_endpoint is declared) — never a bare number (D-07/P-20-02).
result: pass
resolution: |
  Wire-verified 2026-09-20: /cost prints "cost (source: transcript usage × modelrouting
  cost table — derived, not provider-live):" with usage 3000000 in + 1500000 out tokens
  and estimated $6.00 — exactly (3M × $1.0 + 1.5M × $2.0)/1e6 against the scratch
  project's pricing override. Never a bare number; zero provider calls (class-B). The
  live usage_endpoint leg was intentionally not configured in this rig (derived path is
  the shipped default; live-leg shape code-pinned at kit/runtime/commands.go:978).

### 6. /compact at this milestone
expected: /compact invokes the registered Phase 19 CompactNow machinery (compaction completes; typed focus args recorded with the not-yet-passed note); with the seam nil'd it degrades loudly — never silence (19-D-11).
result: pass
resolution: |
  Wire-verified 2026-09-20: /compact focus-on-api-args returns "compaction complete —
  the next request projects from the fresh window" plus the fixed note "focus
  instructions are recorded but not yet passed to the summarizer"; the summarizer rode
  the real provider pipeline (extractive prompt observed at the stub, model GLM-5.3);
  the transcript gains the typed compaction marker (summary + preRef/postRef line
  pointers). Failure leg (stub scripted 500): exactly ONE stderr warning "compaction:
  summarizer failed; proceeding un-compacted", no new marker, turn never fails (D-09).
  Seam-nil'd leg: unit-covered (kit/runtime/commands_test.go:965 → "/compact
  unavailable: compaction not registered"); not config-nil-able by design.
  Observation (non-blocking): on summarizer failure the client-visible text still says
  "compaction complete" while the loudness lands on stderr per the D-09 contract —
  recorded for the operator. Incidental bonus: Phase 19's THRESHOLD auto-compaction
  fired live mid-run (stub-reported usage crossed 80% of the window) and appended its
  own marker — the automatic path proven end-to-end in passing.

### 7. Reserved-name shadowing visible
expected: A discovered commands/model.md (or skill named like a builtin) never fires via slash and logs one shadow-check warning naming the file (D-01).
result: pass
resolution: |
  Wire-verified 2026-09-20: creating .claude/commands/model.md (name shadowing builtin
  /model) re-fires the advertisement with "model" present EXACTLY once (the builtin
  wins; set otherwise unchanged; 0.31s pickup) and emits exactly ONE stderr warning:
  'ass-guard: discovered file ".../.claude/commands/model.md" shadows reserved builtin
  name "model" — the builtin wins /model'. /model still fires the builtin class-B
  handler (session-model output, zero provider calls, no SHADOW BODY expansion).

## Summary

total: 7
passed: 7
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none]
