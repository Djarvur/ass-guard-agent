---
status: testing
phase: 09-serve-path-audit-zcode-parity-re-capture
source: [09-VERIFICATION.md]
started: 2026-08-16T08:45:09Z
updated: 2026-08-16T08:45:09Z
---

## Current Test

number: 1
name: Parity re-baseline disposition (AUD-05 clause)
expected: |
  Read profiles/zcode/drift-reports/2026-08-16-recapture.md §"Parity re-baseline" and decide:
  accept the fresh numbers as the new recorded reference (curated FAIL 1/8 identically on OLD
  and NEW bundles — control-proven stale v1.0-era expectations, profile-independent;
  from-rollout 1/13 with documented harness artifacts; Phase-1 baseline SUPERSEDED, no silent
  threshold movement) and schedule the two follow-ups (re-record curated expectations from
  current live zcode; ExtractTurnsFromRollout delta-record handling + per-turn workspace
  fixtures), OR reject and re-open the leg.
awaiting: user response

## Tests

### 1. Parity re-baseline disposition (AUD-05 clause)
expected: Explicit operator acceptance or rejection of the recorded re-baseline (see above); acceptance includes scheduling the two follow-ups.
result: [pending]

### 2. Live-serve redacted-audit witness (needs ZAI_API_KEY)
expected: Run a real `ass-guard acp serve` against the live provider; observe a redacted `request_shaped` line + an `engine_decision` line in `.ass-guard/audit/<session>.jsonl`; no credential material anywhere in the artifact tree.
result: [pending]

### 3. Capture-method deviation acceptance (optional)
expected: Confirm the operator-directed app-server stdio driver capture (drift report §"Capture method") stands in place of the runbook's interactive procedure, or formalize via an `overrides:` entry.
result: [pending]

### 4. User-story format note (informational)
expected: Operator reformats the ROADMAP goal via `/gsd mvp-phase 9` or accepts as-is (role/capability/outcome all present substantively; only the canonical regex disagrees).
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps
