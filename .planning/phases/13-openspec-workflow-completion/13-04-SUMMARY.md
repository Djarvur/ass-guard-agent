---
phase: 13-openspec-workflow-completion
plan: "04"
subsystem: per-command-eval-suites
tags: [os-03, d04-exact-attribution, eval-net, expanded-profile]
requires:
  - "13-01 (the guard primitive + the matrix legs' probed fixtures)"
  - "13-02 (the seeds the chained suites assert against)"
provides:
  - "12 per-command scenario files (6 happy + 6 fixable, D-04) under internal/evalsuite/scenarios/"
  - "the scenario schema's openspec_profile + fixture keys (validated; unknown keys still rejected)"
  - "the named keys change_dir + fixable_recovery (offline unit-tested) + the 5 fixture seeders"
  - "TestEvalSuite_Matrix_Gated — the matrix gate (per-suite via MATRIX_SUITE_ID; mise eval-gate stays flagship-scoped)"
affects:
  - "future profile/model changes (the expanded matrix is eval-covered on the same re-run gate)"
tech-stack:
  added: []
  patterns:
    - "fixture-key seeding (probed deterministic triggers as scenario data)"
    - "schema-compatible optional keys (the 12-08 contract extended, not broken)"
key-files:
  created:
    - internal/evalsuite/scenarios/opsx-{new,continue,ff,verify,bulk-archive,onboard}{,-fixable}.json
  modified:
    - internal/evalsuite/suite.go
    - internal/evalsuite/suite_test.go
    - internal/evalharness/harness.go
    - cmd/ass-guard/evalsuite_bridge_test.go
decisions:
  - "The matrix suites gate via TestEvalSuite_Matrix_Gated (on demand + verification); mise eval-gate keeps its 15m flagship scope"
  - "The fixable scan reads tool results AND closings (the report-driven commands)"
metrics:
  duration: 150min
  tasks: 2
  commits: 5
status: complete
actuals:
  tokens: 310000
  tasks: 2
  commits: 5
---

# Phase 13 Plan 04: Per-command eval suites extending the Phase-12 net Summary

**One-liner:** The eval net now covers the whole expanded matrix — 12 per-command scenarios (happy + fixable each)
running pass@1 over guarded expanded-profile scratches with deterministic fixtures, exact failure attribution by
scenario id, ALL GREEN in one gated batch run (1450s).

## Task results

| Task | Commits | Result |
|------|---------|--------|
| 1 (tracer) | RED (13-04 test commit) → 6406f65 | The schema keys (openspec_profile + fixture, validated), the 5 fixture seeders, the named keys (change_dir + fixable_recovery), BootstrapExpandedScratch (guard + FAIL-LOUD). opsx-new happy GREEN at k=1 (239s, artifact eval-20260820-194858) + opsx-new-fixable GREEN (169s). |
| 2 (full set) | 6406f65 → 313e32e → eff1848 | The 10 remaining scenario files; the bridge keeps mise eval-gate flagship-scoped + TestEvalSuite_Matrix_Gated for the matrix; the offline attribution/discovery batteries; the selftest green; **the FULL matrix suite GREEN at k=1** (1450s; artifact eval-20260820-202612-k1.json + the run log /tmp/eval-matrix-evidence-full2.log). |

## The gate + evidence

- The full gated set: flagship (13-00/13-01's green artifacts) + the 11-scenario matrix batch — ALL pass@1.
- mise ci green at every close; the offline universe (schema, attribution, discovery, named keys, guard battery) green.
- eval-check-changed selftest green ("class matches (4) + docs-only no-match verified"); the seeded-config path
  falls inside the existing turn-behavior class (internal/openspec coverage) — no script change needed; mise ci's
  dependency set UNCHANGED (no eval task joined ci).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1] The first full batch was RED on two fixture classes** — (a) the happy continue/verify/onboard suites
assumed the commands CREATE their named change, but continue/verify operate on EXISTING changes (onboard's tutorial
names its own) — fixtured (existing_change/completed_change); (b) the fixable scan read only tool results, but the
report-driven commands (verify/bulk) surface findings in the closing — widened to read both. Source fixes followed
by a full green re-run (D-06 honestly: fixes, not flake re-runs; both runs' logs preserved).

**2. [Rule 3] The 13-00 chain pin's settle heuristic was load-sensitive** — deterministic-under-load failures in the
full suite (3/3 green isolated); replaced with WaitChainIdle polling (the chain count is the terminal condition).
eff1848.

**3. [Plan-shape] The gated matrix runs live in cmd/ass-guard** — the plan's verify paths named
`./internal/evalsuite/` runs, but the real RunnerSeam driver is owned by package main (the 12-08 bridge shape);
the matrix gate is TestEvalSuite_Matrix_Gated at the bridge, per-suite selectable via MATRIX_SUITE_ID.

## D-06 record

The matrix batch ran twice: run 1 RED (the two fixture classes above — SOURCE fixes followed), run 2 GREEN.
No best-of-N at zero delta.

## Known Stubs

None.

## Threat Flags

None (T-13-04-01/02 mitigated: named keys only, unknown keys still rejected; the guard is the ONE restore
primitive; T-13-04-03: the matrix gates on demand, k=1, ci untouched).
