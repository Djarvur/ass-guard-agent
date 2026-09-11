---
status: partial
phase: 24-documentation-ops-tails
source: [24-VERIFICATION.md]
started: 2026-09-10T18:20:00Z
updated: 2026-09-11T20:15:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Nightly parity CI post-merge activation (24-USER-SETUP.md)
expected: Post-merge: cron 41 3 * * * fires from master; dispatch smoke green; [self-hosted, zcode] runner online; issue-on-drift permissions verified. Local stand-in evidence (drift-core end-to-end, exit-7 report) already recorded by 24-04.
result: blocked
blocked_by: other
reason: "Merge-gated by design (deterministic fact, auto-resolved by the orchestrator 2026-09-11): gsd/v1.2-claude-code-parity is not merged to master; the workflow file is absent from the default branch (dispatch GitHub-404, documented DEFERRED-TO-MERGE contingency). Post-merge checklist remains tracked in 24-USER-SETUP.md."

### 2. LSP guide live editor leg (docs/lsp-setup.md step 7)
expected: In Zed with the guide's mcp-language-server + gopls setup: prompt an mcp__gopls__* tool call in an ass-guard session; it executes. The scripted dry-run (six mcp__gopls__* tools in the session catalog) is the automated witness.
result: pass
resolution: "Live editor leg executed 2026-09-11: orchestrator configured the project .mcp.json (mcp-language-server + gopls, explicit env.PATH after catching the documented silent LSP-command-not-found hazard) + Zed context_servers; operator opened a fresh ass-guard thread and prompted an mcp__gopls__* call — executed as expected (operator: pass)."

### 3. D-14 real-plugin fidelity (operator judgment)
expected: Operator confirms plugins/skills behave unchanged vs their own Claude Code setup (the superpowers 6.1.1 spot-check is the automated anchor; this is the judgment leg).
result: pass
resolution: |
  First attempt in the MAIN repo failed ("скилл вроде gsd-status в списке доступных не значится")
  — root-caused to environment asymmetry, not a product gap: this repo's GSD skills live only
  in .zcode/skills/ (zcode runtime) and .claude/skills/ does not exist; ass-guard reads the
  Claude-Code layout by contract. Wire check on a scratch project WITH .claude/skills/
  (/tmp/ass-guard-skilltest, real binary over ACP stdio, stub provider): the dynamic
  user-invocable skills section IS injected into the model's system prompt (demo-skill +
  its phrase present, 6911 chars) and the Skill tool is in the catalog — the model-facing
  skills surface works as in Claude Code. Operator then ran the live editor leg on the
  scratch project (after fixing the cwd-relative profiles resolution via symlink) and
  confirmed the skill triggers and returns the expected phrase (operator: pass).
  Note for the environment: cwd-relative profile loading means a project without a
  profiles/ tree cannot start the agent (observed fatal); main-repo GSD skills could be
  installed to .claude/skills/ if wanted in ass-guard sessions here. Neither is a phase-24
  contract item.

## Summary

total: 3
passed: 2
issues: 0
pending: 0
skipped: 0
blocked: 1

## Gaps
