---
status: complete
phase: 19-compaction-cache-control
source: [19-VERIFICATION.md]
started: 2026-09-07T21:30:00Z
updated: 2026-09-10T00:00:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Operator live-threshold retest (G-19-2 / SC-1c)
expected: On the binary installed 2026-09-07 (v0.0.0-20260907153836-a8f63f31b1b0 — engine string verified present), set compaction-threshold low in a live editor session, keep working, then restore 80. Compaction observably fires (marker line on disk under `.ass-guard/`; continuation stays coherent; later turns stay under threshold). Expect NO visible indicator in the editor — the pre-authorized stderr+counter degrade; observability UX deferred to Phase 20's /status vehicle. With the CR-01 fix landed, the backstop must also survive a session's SECOND and later compactions (retry not silently once-per-session).
result: pass
resolution: |
  Automated stand-in for the operator leg (operator ruling 2026-09-10: no manual
  retest — the harness performs the identical steps). spikes/19-compaction-live-uat
  builds the binary from HEAD (superset of the pinned 2026-09-07 install: 19-04
  engine + 19-06/19-07 CR-01 fixes), runs a local Anthropic-protocol mock, drives
  `acp serve` over stdio as a fake editor, and executes the full scenario:
  live set_config_option compaction-threshold 50 → first threshold compaction
  (marker + summarize + stderr note) → second compaction next turn (backstop not
  once-per-session) → restore 80 (wire + persisted project layer) → overflow
  rejection → drift threshold-compact + FORCED compact → retry projects
  post-marker (515664B → 95605B, carries the FORCED marker's fresh summary —
  the CR-01 ruling live) → quiet turn at 80 (no further compaction).
  VERDICT: PASS 20/20. Evidence: uat-harness-run-20260910/ (REPORT.txt,
  requests.jsonl wire log, agent-stderr.log, persisted-config.yaml).
source: automated

## Summary

total: 1
passed: 1
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
