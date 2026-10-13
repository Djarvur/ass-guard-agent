---
phase: quick-261013-uhy
plan: "01"
subsystem: kit/runtime
tags: [data-race, test-fakes, concurrency, nightly-gate, windows-ledger]
requires: [261013-uhx]
provides:
  - "fakeCatalog fully mutex-swept — every canned-table accessor holds t.mu (Hooks closed the family)"
  - "nightly -race gate runs TestRescanConcurrency again (skip entry discharged)"
affects: [.github/workflows/nightly.yml skip list]
tech-stack:
  added: []
  patterns: ["ab7956b test-fake mutex-guard shape (mutex on the fake, no production change)"]
key-files:
  created: []
  modified:
    - kit/runtime/fakes_test.go
    - .github/workflows/nightly.yml
decisions:
  - "Hooks guarded per the ab7956b fakeLearned precedent — t.mu at the accessor head, defer unlock, nothing else changed; the original production race pair was already structurally removed by the Phase-25 catalog injection, so the fix stays test-side"
  - "MemoryInjection/MemoryFiles deliberately left lock-free — FS-only lenses, no canned-table access (contract recorded in the fakeCatalog doc comment)"
  - "Comment reflow keeps the REMOVE-entries-as-failures-are-fixed instruction verbatim; the three remaining documented entries (acpserve simulator pair, TestRunSuite_* openspec-binary battery, escalation flake) survive byte-identical"
metrics:
  duration: ~4 min
  completed: 2026-10-13
  tasks: 2
  files: 2
status: complete
commits: 2
plan_head_before: 79e3eb26349a86e5464a17b7abbcd71d940dfe99
actuals:
  tokens: 3800
  tasks: 2
  commits: 2
---

# Quick Batch 261013-uhy: Fix the TestRescanConcurrency data race in kit/runtime Summary

**One-liner:** Closed the last unsynchronized fakeCatalog accessor (Hooks reader) per the ab7956b mutex-guard shape, proved the rescan concurrency stress test sustained-green under -race (-count=3), and discharged its entry from the nightly -skip list so the hosted drift gate runs the Pitfall-1 scenario again.

## What Was Built

**Task 1 — fakeCatalog mutex sweep closure (kit/runtime/fakes_test.go @ 01ba3a4).** Execution-time audit confirmed the planning-time result: of the 27 fakeCatalog receiver methods in the package (all in fakes_test.go — grep-verified no receivers elsewhere), exactly one touched shared state without t.mu — `Hooks` read the `hooks` field while `setHooks` wrote it under the mutex (a locked writer against an unlocked reader is still a data race when concurrent, the same family as the nightly race). Guarded it exactly like the ab7956b fakeLearned precedent: `t.mu.Lock()` + `defer t.mu.Unlock()` at the method head, nothing else changed. The guard contract is recorded in the fakeCatalog doc comment (accessors touching the canned tables hold mu; MemoryInjection/MemoryFiles are FS-only by design and stay lock-free). No production file touched — the original spawnMCP-vs-installRegistry pair was structurally removed by the Phase-25 catalog injection (read half: t.mu-guarded MCPLayers; write half: t.mu-guarded Rediscover + atomic chainPtr.Store).

**Task 2 — sustained proof + nightly unskip (.github/workflows/nightly.yml @ 40bd483).** Proof first: `go test -race -count=3 -run '^TestRescanConcurrency$' ./kit/runtime/` — 3/3 PASS (1.36–1.39s each, no race reports). Then removed the `^TestRescanConcurrency$` token from the -skip regex (line 68) and dropped the pre-existing-race clause from the reasons comment (reflowed: "The remaining 2026-09-10 entry keeps its reason: the TestRunSuite_* battery…"; the trailing REMOVE-entries instruction intact). Worked against the file's post-uhx text — sibling 261013-uhx had already removed its own TestPermissionsE2E entry; the diff touches only this item's entry.

## Verification

- Task 1 gates: `gofmt -l kit/runtime/` empty; `go vet ./kit/runtime/` clean; `go test -race -count=1 ./kit/runtime/` green (42.8s).
- Sustained proof: `go test -race -count=3 -run '^TestRescanConcurrency$' ./kit/runtime/` green (run twice: pre-edit proof and post-edit verify chain — 5.2s / 5.3s).
- Skip-list gates: comment-filtered `grep -c RescanConcurrency` on nightly.yml = 0; `TestRunSuite_` survivor grep passes; `ruby -ryaml` parse passes (NIGHTLY-UNSKIP-OK).
- Scope: `git diff --name-only` at plan end shows only the two declared files; zero production files touched.

## Deviations from Plan

None — plan executed exactly as written. (Environment note, not a deviation: commits landed on master per this quick batch's orchestrator dispatch — isolation=none sequential on the primary checkout, `use_worktrees: false` in project config, consistent with sibling items uhw/uhx.)

## Known Stubs

None.

## WINDOWS Ledger Note

Per dispatch constraints, .planning/WINDOWS.md was NOT edited — row discharge for WINDOWS #21/#27 belongs to sibling item 261013-uhz / the orchestrator. This item's evidence: the race family is closed in the test fake and the skip entry is gone from the hosted gate.

## Self-Check: PASSED

- SUMMARY.md on disk; commits 01ba3a4 (Task 1) and 40bd483 (Task 2) both resolvable from HEAD.
