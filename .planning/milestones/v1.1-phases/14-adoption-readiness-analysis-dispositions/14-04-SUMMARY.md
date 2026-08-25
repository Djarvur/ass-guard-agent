---
phase: 14-adoption-readiness-analysis-dispositions
plan: "04"
subsystem: testing
tags: [parity, cache-control, drift-warning, probe, tdd]

requires:
  - phase: 14-02
    provides: ScanContextBehavior corpus census + the committed cache-control.jsonl fixture (the placement pin source)
  - phase: 14-03
    provides: the pi↔shaper audit's cache_control dispositions (CC-1 routed emission, CC-2/3/4 corpus-wins)
provides:
  - parity.RunCacheProbe / CacheProbeReport / ProbeCheck — stable→volatile merge-ordering assertions
  - parity.AssertPlacementAgainstPin + PinClasses — bidirectional placement-vs-corpus-pin check
  - cmd zcodeInstalledVersion seam (fixed-argv `zcode --version`, 3s timeout) + non-blocking drift/provenance/skip line in every parity run
  - `cache probe: PASS/FAIL — fact` footer line + additive RunResult.CacheProbe
  - `--cache-pin` flag (repo-relative default to 14-02's fixture; empty/unreadable → explicit skipped check)
affects: [12-08 nightly gate, parity harness operators, post-adoption cache_control emission fix]

actuals:
  tokens: 12245   # chars/4 over the realized 6-commit diff (5 files, +1265/-4)
  tasks: 3
  commits: 6

tech-stack:
  added: []   # stdlib only (os/exec, context timeout); zero new dependencies
  patterns:
    - "Seam-var injection at the cmd layer (zcodeInstalledVersion / parityRun / composeCacheProbeInput) for offline runParity testing"
    - "Corpus-pin derivation in production (PinClasses over the committed fixture), never a hand-written expectation"

key-files:
  created:
    - internal/parity/cacheprobe.go
    - internal/parity/cacheprobe_test.go
    - cmd/ass-guard/parity_test.go
  modified:
    - cmd/ass-guard/parity.go
    - internal/parity/run.go

key-decisions:
  - "Pin derivation is mechanical: a placement class is pinned iff its census placements >= scanned records (the 'every system block, 910/910' corpus fact); the fixture's self-documented SYNTHETIC classifier probes sit below the bar and never contaminate the pin — corpus wins on conflict, as the locked prohibition requires"
  - "The default-composition probe verdict at wiring time is FAIL naming the system class — 14-03 CC-1's routed emission gap IS the probe line's fact (the plan's anticipated finding), not a defect of this plan"
  - "The probe is report-only: footer line + additive RunResult.CacheProbe field; it never feeds Summary, the gate, or exit semantics (drift warning and probe both non-blocking by design)"
  - "MCP tools are labeled absent in the offline composition (live host needed); skills/agents listings merge for real via ecosys.Discover — the live offline run shows stable prefix 3 + dynamic 2, ordering ok"

patterns-established:
  - "Non-blocking observability line pattern: one stderr line per run stating version relationship (drift/provenance/skip), never touching exit semantics"
  - "Corpus-derived expectation loading at runtime from committed fixtures (--cache-pin), mirroring the --suite repo-relative default contract"

requirements-completed: [EARLY-03]

coverage:
  - id: D1
    description: "Non-blocking zcode version drift warning in the parity run (loud on mismatch naming both versions, provenance line on match, skip note on unresolvable/missing manifest/pin)"
    requirement: EARLY-03
    verification:
      - kind: unit
        ref: cmd/ass-guard/parity_test.go#TestParityDriftWarning_Mismatch
        status: pass
      - kind: unit
        ref: cmd/ass-guard/parity_test.go#TestParityDriftWarning_Match
        status: pass
      - kind: unit
        ref: cmd/ass-guard/parity_test.go#TestParityDriftWarning_Unresolvable
        status: pass
    human_judgment: false
  - id: D2
    description: "Cache probe stable→volatile ordering assertions (violations name offending index pairs; green reports enumerate stable prefix + dynamic count; empty merges pass)"
    requirement: EARLY-03
    verification:
      - kind: unit
        ref: internal/parity/cacheprobe_test.go#TestCacheProbe_OrderingViolationFails
        status: pass
      - kind: unit
        ref: internal/parity/cacheprobe_test.go#TestCacheProbe_ToolArraySpliceFails
        status: pass
      - kind: unit
        ref: internal/parity/cacheprobe_test.go#TestCacheProbe_GreenCase
        status: pass
      - kind: unit
        ref: internal/parity/cacheprobe_test.go#TestCacheProbe_EmptyMerges
        status: pass
    human_judgment: false
  - id: D3
    description: "Placement-vs-pin assertion + parity-run wiring (footer `cache probe:` line in both states; probe verdict never changes A/B summary semantics; default run reports the routed system-class gap)"
    requirement: EARLY-03
    verification:
      - kind: unit
        ref: internal/parity/cacheprobe_test.go#TestCacheProbe_PlacementAgainstPin
        status: pass
      - kind: unit
        ref: cmd/ass-guard/parity_test.go#TestParityRun_CacheProbeWired
        status: pass
      - kind: unit
        ref: cmd/ass-guard/parity_test.go#TestParityRun_CacheProbeDefaultGap
        status: pass
      - kind: manual_procedural
        ref: "offline `go run ./cmd/ass-guard parity` from repo root: drift line + cache probe footer line observed (evidence in SUMMARY Verification section)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Full CI gate green with the new probe + warning (vet, lint, build, race tests across all 26 packages)"
    verification:
      - kind: command
        ref: mise run ci
        status: pass
    human_judgment: false

duration: 24min
completed: 2026-08-19
status: complete
---

# Phase 14 Plan 04: Cache-discipline probe + target-version drift warning Summary

**Cache-discipline probe (stable→volatile merge ordering + placement-vs-corpus-pin) wired into `ass-guard parity` beside a loud, non-blocking installed-vs-pinned zcode version drift warning — one run now answers structure, cache discipline, and target-version drift together (EARLY-03).**

## Performance

- **Duration:** 24 min
- **Started:** 2026-08-19T18:47:14Z
- **Completed:** 2026-08-19T19:11:17Z
- **Tasks:** 3/3
- **Files modified:** 5

## Accomplishments

- Every parity run states its target-version relationship to the pinned capture (09-03 provenance in coverage.yaml vs a fixed-argv, 3s-bounded `zcode --version` exec): loud WARNING naming both versions on drift, explicit provenance line on match, skip note on unresolvable — never blocking (test-pinned summary/error equality between match and mismatch runs).
- Merge-order regressions now fail loudly: any future merge site splicing a volatile block or mcp__* tool into the captured stable prefix trips the probe with the offending index pair named, instead of silently busting cache economics.
- Placement discipline is asserted against the corpus pin, derived mechanically from 14-02's committed fixture (corpus-wins rule in code; no pi-derived expectations anywhere in the test file).
- Live offline evidence (repo-root run): ordering GREEN on the real composition (stable prefix 3 + dynamic 2 — the operator's actual skills/agents listings appended after the captured blocks), placement reporting the routed system-class emission gap — exactly the finding the plan told us to expect and record.

## Task Commits

Each task was committed atomically (TDD RED → GREEN per task):

1. **Task 1: drift-warning slice (tracer)** — RED `4a22b33`, GREEN `ce392be`
2. **Task 2: cache probe ordering** — RED `0cccbe0`, GREEN `831c181`
3. **Task 3: placement-vs-pin + wiring** — RED `69a5343`, GREEN `042e83c`

**Plan metadata:** (recorded at final commit)

## TDD Gate Compliance

| Gate | Task 1 | Task 2 | Task 3 |
|------|--------|--------|--------|
| RED (test commit first) | ✓ 4a22b33 | ✓ 0cccbe0 | ✓ 69a5343 |
| GREEN (feat commit) | ✓ ce392be | ✓ 831c181 | ✓ 042e83c |
| REFACTOR | — (not needed) | — (not needed) | — (lint conformance folded into GREEN) |

Tracer feedback gate (auto mode): Task 1's verify re-run end-to-end green before expansion tasks started.

## Files Created/Modified

- `internal/parity/cacheprobe.go` — RunCacheProbe (system/tools ordering checks), AssertPlacementAgainstPin (bidirectional class delta), PinClasses (corpus-pin derivation), CacheProbeReport.Fact; corpus-wins authority rule in the file doc comment
- `internal/parity/cacheprobe_test.go` — Tests 4-8 (ordering violations, green case, empty merges, placement-vs-pin from the committed fixture)
- `cmd/ass-guard/parity.go` — zcodeInstalledVersion seam (T-14-11/12 mitigations), emitVersionDriftWarning, parityRun/composeCacheProbeInput seams, assembleCacheProbe + placementCheck, --cache-pin flag, footer probe line
- `cmd/ass-guard/parity_test.go` — Tests 1-3 + 9 (drift warning trio; wiring pass/fail/summary-equality; default-gap leg)
- `internal/parity/run.go` — RunResult.CacheProbe additive field

## Decisions Made

- Pin derivation rule (placements ≥ scanned records) chosen to be mechanically computable from 14-02's ContextBehaviorReport while excluding the fixture's self-documented synthetic classifier probes — the corpus fact "every system block" is encoded without hand-writing expectations.
- The wiring-time placement FAIL (missing system class) is left as the probe's standing verdict rather than special-cased away: it is the routed 14-03 CC-1 emission gap made visible on every run, which is precisely the daily-use drift detection EARLY-03 asked for.
- `--cache-pin` flag added (mirroring `--suite`): the repo-relative default preserves operator ergonomics; tests pass the fixture path explicitly.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Plan referenced "the existing parity cmd tests" — none existed**
- **Found during:** Task 1 (RED)
- **Issue:** Test 9's "offline, suite with a fake arm/provider per the existing parity cmd tests" assumed prior cmd-level parity tests; `cmd/ass-guard/parity_test.go` did not exist.
- **Fix:** Created it with a `parityRun` package seam over `parity.Run` (fake A/B arms offline) + fixture builders (minimal profile bundle pinning a coverage version, one-turn suite).
- **Files modified:** cmd/ass-guard/parity_test.go, cmd/ass-guard/parity.go
- **Verification:** all 7 tests green; provider path still exercised live by the real default.
- **Committed in:** 4a22b33 / ce392be

**2. [Rule 1 - Bug] Wiring test's "default composition" leg passed for the wrong reason**
- **Found during:** Task 3 (GREEN verification, pre-commit)
- **Issue:** `t.Cleanup` restores the composition seam only at test end, so the default leg ran with the violation fake still active — its assertions were satisfied by the violation's own detail text.
- **Fix:** Split the default leg into `TestParityRun_CacheProbeDefaultGap` (seam never faked there) and tightened the assertion to the literal `pin-has-composed-lacks [system]` delta (the word "system" alone also matches the check name `system-ordering`).
- **Files modified:** cmd/ass-guard/parity_test.go
- **Verification:** default leg now exercises the real ecosys composition path.
- **Committed in:** 042e83c

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug)
**Impact on plan:** Both necessary for honest test semantics; no scope creep. Lint-conformance refactors (external test package, struct returns, goconst constants) rode the GREEN commits per the `mise ci` gate.

## Issues Encountered

None beyond the deviations above. `zcode` is not on the executor shell's PATH, which conveniently demonstrated the unresolvable-version skip path live.

## Verification

- `go test ./internal/parity/... ./cmd/ass-guard/... -count=1` — green.
- `mise run ci` — green (vet + lint 0 issues + build + race tests, all 26 packages).
- Offline live run from repo root (`go run ./cmd/ass-guard parity --results ""`):
  - `zcode version check skipped: installed zcode unresolvable (exec zcode --version: ... not found); pinned capture zcode 0.16.3`
  - `cache probe: FAIL — system-ordering: stable prefix 3, dynamic 2, ordering ok; tools-ordering: stable prefix 79, dynamic 0, ordering ok; placement-vs-pin: pin-has-composed-lacks [system] (corpus pin: 14-02 fixture)`
  - Exit semantics unchanged (tests 1 vs 2 summary-equality).

## Known Stubs

None. The standing placement FAIL is not a stub — it is the probe faithfully reporting the routed 14-03 CC-1 emission gap (post-adoption queue); the emission fix will flip the line green without probe changes (the seam derives flags from the profile once TextBlock carries the captured value).

## Threat Flags

None beyond the plan's threat model: T-14-11/12 mitigated as specified (fixed argv, 3s timeout, error→skip; seam keeps prod path testable), T-14-13 mitigated (placement expectations read only the committed corpus-derived fixture; corpus-wins stated in cacheprobe.go's doc comment).

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- EARLY-03 closed: the parity harness carries the cache probe + drift warning; wave 2 continues with 14-05 (token economics) and 14-06 (tool contract).
- The standing `cache probe: FAIL` (system-class emission gap) is expected until the post-adoption emission fix lands (routed per 14-03 CC-1; needs the profile-bundle TextBlock format change).
- 12-08's nightly gate can consume the probe line/report as-is when it lands.

## Self-Check: PASSED

All 5 key files exist on disk; all 7 commits (3 RED + 3 GREEN + SUMMARY docs) verified in git log; mise ci green; live offline run evidence recorded above.

---
*Phase: 14-adoption-readiness-analysis-dispositions*
*Completed: 2026-08-19*
