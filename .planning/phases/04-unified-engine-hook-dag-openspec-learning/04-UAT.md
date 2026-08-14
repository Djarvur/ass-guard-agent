---
status: partial
phase: 04-unified-engine-hook-dag-openspec-learning
source: [04-01-SUMMARY.md, 04-02-SUMMARY.md, 04-03-SUMMARY.md, 04-04-SUMMARY.md, 04-05-SUMMARY.md, 04-06-SUMMARY.md, 04-07-SUMMARY.md, 04-VERIFICATION-PREP.md]
started: 2026-08-14T14:07:34Z
updated: 2026-08-14T14:45:00Z
---

## Current Test

[testing paused — 11 items outstanding]

## Tests

### 1. Kick off an unmodified OpenSpec workflow
expected: In a project with an openspec.toml workflow, start ass-guard and send the kickoff prompt. The agent picks up the current stage and begins working it — the toolkit runs unmodified.
result: issue
reported: "to kick of the openspec scenario I need claude code compatible commands support in ass-guard. I'll have to run something like '/opsx:explore', and the corresponding command have to run. there is a binary named openspec, but it is the agent commands and skills support, not the real openspec toolkit"
severity: major

### 2. Stages advance with zero "continue" taps
expected: When a stage completes, the next stage starts by itself. You never type "continue" — transitions fire on text-pattern handoffs AND known handoff tool-calls.
result: [pending]

### 3. Forgotten routine runs after the implement stage
expected: After the implement stage, the seeded hooks (tests, lint, review, memory) run automatically, visible in the transcript. You did not ask for them.
result: [pending]

### 4. Unfamiliar launch: asked once, remembered
expected: On an unfamiliar handoff the agent asks once ("fresh context? wait? how long?"). Answer it. When the same situation recurs, it does not ask again — the answer is remembered.
result: [pending]

### 5. Outcome — the workflow completes unattended
expected: The whole OpenSpec workflow runs to completion without babysitting. The toolkit just ran.
result: [pending]

### 6. Structural safety — unmatched output triggers nothing
expected: Output that matches no pattern and no known handoff tool-call triggers no injection. No match means nothing runs.
result: [pending]

### 7. Engine failure degrades, never blocks
expected: If the engine fails mid-run, the current turn still completes and the agent degrades to manual-continue. An engine failure never prevents a turn from finishing.
result: [pending]

### 8. Hook on-failure semantics + loop prevention
expected: Every hook failure declares halt/continue/ask (never silently blocks). A hook cannot re-trigger its own stage without explicit opt-in — no infinite hook loops.
result: [pending]

### 9. Cancel-drain — manual cancellation stops queued injections
expected: Cancelling a turn (ESC in Zed) drains any queued continue-injections immediately. Cancellation is the off-switch that always works.
result: [pending]

### 10. Concurrency discipline + swappable backends
expected: Read-only tools (Glob, Grep, Read) run concurrently within a turn; mutating tools serialize relative to each other. WebSearch/WebFetch backends swap by config, no code changes.
result: [pending]

### 11. Learning store is versioned and revertible
expected: Learned settings are inspectable (`learning list`) and revertible (`learning revert`). Nothing learned is permanent or opaque.
result: [pending]

### 12. Coverage — outcome clause observably true
expected: Goal-backward: "the toolkit just runs without my babysitting" is observably true in the codebase — the engine's observe/decide loop, seeded hook set, and learning store exist and are exercised by the E2E suite.
result: [pending]

## Summary

total: 12
passed: 0
issues: 1
pending: 11
skipped: 0
blocked: 0

## Gaps

- truth: "A developer can kick off an unmodified OpenSpec workflow through ass-guard by invoking the toolkit's agent commands (e.g. /opsx:explore), and the corresponding command runs"
  status: failed
  reason: "User reported: to kick of the openspec scenario I need claude code compatible commands support in ass-guard. I'll have to run something like '/opsx:explore', and the corresponding command have to run. there is a binary named openspec, but it is the agent commands and skills support, not the real openspec toolkit"
  severity: major
  test: 1
  root_cause: "Entry-surface gap, twofold. (1) internal/ecosys (Phase 5, ECOS-04) implements discovery+merge of .claude/ commands/skills/plugins but is UNWIRED — zero non-test importers in the repo; no ACP/session surface exposes loaded commands and no prompt path expands '/opsx:explore'-style invocations into the command's markdown prompt. (2) Phase 4's hosting model (internal/openspec, D-13) treats the openspec BINARY as the stage driver via subprocess tools openspec:list/show/validate/apply/implement — but the real toolkit (openspec v1.5.0, verified via 'openspec init --tools claude' probe) drives workflows through agent-executed command files it installs (.claude/commands/opsx/{explore,apply,propose,sync,archive}.md + .claude/skills/openspec-*/SKILL.md); the binary is supporting tooling (list/view/change/spec/archive/doctor/context). The seeded command set doesn't match the real binary surface either (no show/validate/apply/implement top-level subcommands). E2E proof used a STUB binary so the model mismatch was never caught (the real-openspec gate ASSGUARD_OPENSPEC_BIN=1 is operator-run and was not run)."
  artifacts:
    - path: "internal/ecosys/"
      issue: "discovery implemented but unwired — no importer wires AllSkills/AllCommands into session/ACP; /opsx:* invocation impossible"
    - path: "internal/openspec/seeded.toml"
      issue: "declares commands list/show/validate/apply/implement; real openspec v1.5.0 surface is list/view/change/spec/archive/config/schema/store/doctor/context/workset"
    - path: "cmd/ass-guard/acp_serve.go"
      issue: "no slash-command surface — session/prompt has no /command expansion path"
  missing:
    - "Wire internal/ecosys discovery into the session/ACP layer: expose loaded slash-commands and expand '/<namespace>:<name>' (e.g. /opsx:explore) prompt text into the command's markdown prompt before the provider turn"
    - "Reconcile internal/openspec adapter command set with the real openspec v1.5.0 binary surface (or derive it from openspec/config.yaml)"
    - "Run the operator-gated real-openspec test (ASSGUARD_OPENSPEC_BIN=1) against the real binary as part of the fix gate"
  debug_session: ""
