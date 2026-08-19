---
status: testing
phase: 14-adoption-readiness-analysis-dispositions
source: [14-VERIFICATION.md]
started: 2026-08-19T21:25:00Z
updated: 2026-08-19T21:25:00Z
---

## Current Test

number: 1
name: Run the gated live rollback E2E (or explicitly accept the offline-evidence disposition in WINDOWS.md #4)
expected: |
  TestCheckpointLiveRollback_Gated PASSES with recorded evidence (transcript + ref list),
  closing WINDOWS.md entry #4 and the phase gate's "live rollback demonstration" clause.
  Command: ASSGUARD_CHECKPOINT_E2E=1 ZAI_API_KEY=<GLM key> go test ./cmd/ass-guard/... -run TestCheckpointLiveRollback_Gated -count=1 -v
awaiting: user response

## Tests

### 1. Gated live rollback E2E (or accept offline disposition)
expected: TestCheckpointLiveRollback_Gated PASSES with recorded evidence, closing WINDOWS.md #4. Offline UserGitUntouched pair already proves the invariant; this leg is the live-serve demonstration.
result: [pending]

### 2. pi↔shaper audit row sign-off (docs/shaper-pi-audit.md, 29 rows)
expected: Each row's classification (match / divergence-justified / divergence-routed / absent-at-pin) judged correct against its dual file:line citations; zero-fix outcome accepted as legitimate.
result: [pending]

### 3. Compaction-decision routing sign-off (docs/compaction-decision.md §3)
expected: cache_control system-block emission → post-adoption queue and cross-turn window span → 12-05 confirmed as intended destinations before 12-05 consumes them.
result: [pending]

### 4. MVP goal-format discrepancy ruling
expected: Either accept the current phase-goal wording ("As the operator…" — semantic slots role/capability/outcome all present; verification proceeded on that basis) or normalize via /gsd mvp-phase 14.
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps
