---
phase: 24-documentation-ops-tails
plan: "01"
subsystem: modelrouting
tags: [jsonl, circuit-breaker, cost-ceiling, model-routing, replay, tail-01]

requires:
  - phase: 03-scheduling-safety
    provides: Breaker/CostTracker seams, CircuitBreaker, CostCeilingTracker, SetBreakers/SetCostTracker injection
provides:
  - DispatchOutcome record schema (mechanical D-04 signals only) + append-only JSONL OutcomeStore under .ass-guard/routing/outcomes.jsonl
  - Tolerant ReadOutcomes (skip-and-count malformed/torn/unknown-shape lines; missing file = empty store)
  - ReplayBreakers / ReplayCostTracker — durable outcome history re-entering routing through the EXISTING seams (D-06)
  - FirstAllowed — the demotion-aware fallback walk over a breakers map
  - Aggregate/AggregateStats/OutcomeWindow — per-key counts + token/cost totals over a trailing record-count window
  - Exported ProviderModelKey (renamed from providerModelKey) so the replay map is consumable across packages
affects: [24-02 (recording sites + live-path wiring), 24-05 (stats dump CLI family), modelrouting seam consumers]

actuals:
  tokens: 11261   # chars/4 over the realized diff (45043 chars, 10 files)
  tasks: 3
  commits: 4      # measured: git rev-list --count 59952f8..HEAD

tech-stack:
  added: []        # stdlib + in-tree only (per RESEARCH Standard Stack — verified: go.mod content unchanged)
  patterns:
    - "Compiling-RED scaffolding: test file + zero-value stubs so the RED suite runs and fails on assertions (tdd-red-evidence RED_EVIDENCE_OK)"
    - "Durable-store replay = chronological sort by record.At (stable, store order breaks ties) before feeding any seam"
    - "Rolling window in RECORDED OUTCOMES per key, never wall-clock alone (Pitfall 8)"

key-files:
  created:
    - internal/modelrouting/outcomes.go
    - internal/modelrouting/outcomes_agg.go
    - internal/modelrouting/outcomes_test.go
  modified:
    - internal/modelrouting/dispatch.go
    - internal/modelrouting/breaker.go
    - internal/modelrouting/safety.go
    - internal/modelrouting/dispatch_test.go
    - internal/modelrouting/breaker_test.go
    - internal/modelrouting/safety_test.go
    - .golangci.yml

key-decisions:
  - "Compiling RED (stubs + failing suite) over fail-to-compile RED — tdd_mode's #3770 gate requires target_test_failed; the plan pre-authorized this route"
  - "ReadOutcomes treats a missing file as an empty store (not an error) — 24-02's startup seeding reads before any append exists"
  - "Replay skips zero-token records before Account (the (0,0) no-op) so a large store replay does not spam per-record warns; zero = unknown-on-path, never free"
  - "FirstAllowed on all-denied returns the zero Target + false (callers treat zero Model as no-candidate) — absent key stays allowed by the SetBreakers convention"
  - "golangci baseline repaired per the STATE blocker's prescription: exhaustruct_v5 exclusion rename + wsl_v5/gomodguard_v2/noinlineerr/modernize disables (2.13 renames/new linters silently re-enabled under default:all)"

patterns-established:
  - "Outcome record schema: 11 mechanical json-tagged fields; the round-trip test pins the exact field set so a content-bearing field fails the suite"
  - "Seam replay: durable history re-enters routing ONLY through the existing Breaker/CostTracker seams — structural/exhausted never feed breakers on replay, exactly as live"

requirements-completed: [TAIL-01]   # shared with 24-02 — ready-ids gate defers the checkbox until both SUMMARYs exist

coverage:
  - id: D1
    description: "Append-only JSONL outcome store under .ass-guard/routing/outcomes.jsonl: exact-field-set round-trip, no-dedup probe, tolerant read (malformed + unknown-field lines), interrupted-tail skip, 0600/0750 perms + self-gitignore, 10x10 concurrent append"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "internal/modelrouting/outcomes_test.go#TestOutcomeStoreAppendReadRoundtrip"
        status: pass
      - kind: unit
        ref: "internal/modelrouting/outcomes_test.go#TestOutcomeStoreTolerantRead"
        status: pass
      - kind: unit
        ref: "internal/modelrouting/outcomes_test.go#TestOutcomeStoreInterruptedTail"
        status: pass
      - kind: unit
        ref: "internal/modelrouting/outcomes_test.go#TestOutcomeStorePermsAndGitignore"
        status: pass
      - kind: unit
        ref: "internal/modelrouting/outcomes_test.go#TestOutcomeStoreConcurrentAppend"
        status: pass
    human_judgment: false
  - id: D2
    description: "Seam-replay feedback (D-06/D-07 deterministic half): seeded transients open the replayed breaker, an ok recovers it, structural/exhausted never create or feed a breaker, token-bearing records drive CostCeilingTracker to CostDegrade — all through the EXISTING seams"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "internal/modelrouting/outcomes_test.go#TestOutcomeFeedbackBreakerOpens"
        status: pass
      - kind: unit
        ref: "internal/modelrouting/outcomes_test.go#TestOutcomeFeedbackBreakerRecoversAndSkipsStructural"
        status: pass
      - kind: unit
        ref: "internal/modelrouting/outcomes_test.go#TestOutcomeFeedbackCostDegrade"
        status: pass
    human_judgment: false
  - id: D3
    description: "FirstAllowed demotion walk: primary-open demotes to the fallback (demoted true); all-allowing chain returns the primary (demoted false); absent key = allowed"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "internal/modelrouting/outcomes_test.go#TestFirstAllowedDemotes"
        status: pass
    human_judgment: false
  - id: D4
    description: "Exported ProviderModelKey (renamed from providerModelKey) with the replay map consumable via Scheduler.SetBreakers across packages — mechanical rename, no behavior change; full package + repo build green"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "command: go build ./... && go test -race -count=1 ./internal/modelrouting/"
        status: pass
    human_judgment: false

duration: 37 min
completed: 2026-09-10
status: complete
plan_head_before: 59952f84843ac82beb69f49599f2746fc2656501
---

# Phase 24 Plan 01: TAIL-01 core Summary

**Deterministic append-only JSONL outcome store (`.ass-guard/routing/outcomes.jsonl`) plus seam-replay aggregation that provably opens breakers and fires cost-degrade through the EXISTING modelrouting seams — built test-first (RED_EVIDENCE_OK → GREEN → REFACTOR)**

## Performance

- **Duration:** 37 min
- **Started:** 2026-09-10T13:44:36Z
- **Completed:** 2026-09-10T14:21:11Z
- **Tasks:** 3 (RED / GREEN / REFACTOR+gate)
- **Files modified:** 10 (3 created, 7 modified)

## TDD Gate Compliance

- **RED:** `5552942` test(24-01) — 9-test suite ran and failed on real assertions (9 fail, 0 pass, zero panics) against zero-value stubs; evidence record validated `RED_EVIDENCE_OK / target_test_failed` via `check tdd-red-evidence` (compiling-RED route the plan pre-authorizes; required under tdd_mode #3770 — a fail-to-compile RED classifies INVALID_RED).
- **GREEN:** `25b977b` feat(24-01) — all 9 tests pass under `-race`; full package suite green.
- **REFACTOR:** `25d9e13` refactor(24-01) — lint-conformance pass; behavior unchanged, suite re-verified green.
- Plus `ce424ea` chore(24-01) — golangci config baseline repair (gate enabler, see Deviations #3).

## Accomplishments

- **Store (D-04/D-05):** `DispatchOutcome` (11 mechanical json-tagged fields — at/provider/model/tier/outcome/fallback_used/latency_ms/in_tokens/out_tokens/cost_usd/origin; zero tokens = unknown-on-path, never free) + `OutcomeStore`: mutex-serialized `O_APPEND|O_CREATE|O_WRONLY` 0600 open-per-append (one-line interrupted-write window), 0750 dirs, self-written `.ass-guard/.gitignore` (`*\n!.gitignore\n`), no dedup (append-only law pinned by test).
- **Tolerant read:** `ReadOutcomes(path)` skips-and-counts malformed lines, a torn trailing line, and unknown-shape lines (missing provider/model/timestamp or unknown outcome class); unknown extra JSON fields kept (16-D-20 additive discipline); missing file = empty store; I/O errors returned, never panicked.
- **Seam replay (D-06/D-07):** `ReplayBreakers` (chronological; ok→RecordSuccess, transient→RecordTransient with synthetic KindTransient error; structural/exhausted skipped — mirrors dispatch.go:250-272 exactly), `ReplayCostTracker` (Account per token-bearing record through CostCeilingTracker), `FirstAllowed` (absent-key-allowed demotion walk), `Aggregate`/`AggregateStats`/`OutcomeWindow` (trailing record-count window per key, Pitfall 8).
- **Rename:** `providerModelKey` → exported `ProviderModelKey` (dispatch.go/breaker.go/safety.go + 4 test files) — the replay map is now consumable across packages; zero behavior change.

## Task Commits

1. **Task 1 (RED): failing outcome-store + seam-replay test suite** — `5552942` (test)
2. **Task 2 (GREEN): outcome store + seam-replay aggregation** — `25b977b` (feat)
3. **Task 3 (REFACTOR + gate): lint conformance** — `25d9e13` (refactor) + `ce424ea` (chore: golangci baseline repair)

## Files Created/Modified

- `internal/modelrouting/outcomes.go` — record schema, append-only store, tolerant reader (created)
- `internal/modelrouting/outcomes_agg.go` — Aggregate/ReplayBreakers/ReplayCostTracker/FirstAllowed (created)
- `internal/modelrouting/outcomes_test.go` — 9-test RED→GREEN suite, stdlib testing, fixed injected clocks (created)
- `internal/modelrouting/dispatch.go` — ProviderModelKey export + doc rationale
- `internal/modelrouting/breaker.go`, `safety.go`, `dispatch_test.go`, `breaker_test.go`, `safety_test.go` — mechanical rename collateral
- `.golangci.yml` — 2.13 linter-rename baseline repair

## Decisions Made

- Compiling-RED over fail-to-compile RED (see TDD Gate Compliance).
- `ReadOutcomes`: missing file = empty store; unknown-shape line = skipped+counted.
- Replay skips (0,0)-token records pre-Account (no per-record warn spam over a large store).
- `FirstAllowed` all-denied → zero Target + false; callers treat zero Model as no-candidate.
- Aggregation window = trailing N records per key (`OutcomeWindowAll` disables).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Compiling-RED scaffolding committed in Task 1**
- **Found during:** Task 1
- **Issue:** Task 1's `<files>` lists only `outcomes_test.go`, but `tdd_mode: true` requires RED evidence classified `target_test_failed` — a fail-to-compile run classifies INVALID_RED and blocks GREEN.
- **Fix:** Took the plan's explicitly pre-authorized alternative ("write the test file plus empty type/function skeletons returning zero values so tests RUN and FAIL"): `outcomes.go`/`outcomes_agg.go` were created in Task 1 with the schema + zero-value stubs. Verified: 9 fail / 0 pass on assertions, `RED_EVIDENCE_OK`.
- **Files modified:** internal/modelrouting/outcomes.go, internal/modelrouting/outcomes_agg.go (as stubs)
- **Verification:** check tdd-red-evidence verdict RED_EVIDENCE_OK, reason target_test_failed
- **Committed in:** 5552942

**2. [Rule 3 - Blocking] Rename collateral beyond the plan's file list**
- **Found during:** Task 2
- **Issue:** Plan said providerModelKey uses live in "dispatch.go + breaker.go only" — stale: `safety.go` and three test files also reference it.
- **Fix:** Mechanical rename covered safety.go + dispatch_test.go/breaker_test.go/safety_test.go (and the new outcomes_test.go); acceptance criterion "no unexported providerModelKey remains in internal/modelrouting" verified by grep (0 left).
- **Files modified:** internal/modelrouting/safety.go, internal/modelrouting/{dispatch,breaker,safety}_test.go
- **Verification:** grep -rn providerModelKey → 0 hits; go build ./... green
- **Committed in:** 25b977b

**3. [Rule 3 - Blocking] golangci-lint baseline repair (.golangci.yml)**
- **Found during:** Task 3 (the `mise ci` gate)
- **Issue:** golangci 2.12.x (config-matching) panics under Go 1.27.1 (pre-existing environmental, STATE LINT BASELINE note + 23-01 precedent); the only runnable linter (2.13.2) renamed exhaustruct→exhaustruct_v5 / wsl→wsl_v5 and added new linters (noinlineerr, modernize) that `default: all` silently enabled — 273 findings on this package, 212 of them the stale exhaustruct exclusion firing on pre-existing files.
- **Fix:** The STATE blocker's prescribed one-line-class fix: exclusion names exhaustruct_v5; disables wsl_v5/gomodguard_v2 (rename re-enables) + noinlineerr/modernize (new-in-2.13, fire on the pre-Phase-24 baseline, never vetted by this config; noinlineerr contradicts the house inline-err style). Result: ZERO findings on the three 24-01 files under 2.13.2.
- **Files modified:** .golangci.yml
- **Verification:** package run shows only the two pre-existing factory.go hugeParam findings (file untouched by this plan)
- **Committed in:** ce424ea

**4. [Rule 3 - Blocking] `mise` absent on this host — gate legs run directly**
- **Found during:** Task 3
- **Issue:** `mise ci` cannot run (binary absent; also absent for phases 23+ per their summaries). The gate's semantics are vet+lint+build+test.
- **Fix:** Ran the four legs directly: `go vet ./...` green; `CGO_ENABLED=0 go build ./...` green; lint via `go run golangci-lint@v2.13.2` (see #3); `go test -race -count=1 ./...` — modelrouting green; four packages fail with the DOCUMENTED pre-existing set, verified byte-identical at the pre-plan base commit in a scratch worktree (TestRescanConcurrency: phase-22 deferred race; TestPermissionsE2E: STATE cross-workstream note; TestRunSuite_*: openspec binary not on PATH). One extra failure (TestEscalation_ReapAllUsesLadder, coreexec) proved load-flaky: passes 5/5 isolated in both trees and in the base full-suite run — coreexec does not import modelrouting. Logged to deferred-items.md.
- **Verification:** base-commit full-suite comparison in a scratch worktree (removed after)
- **Committed in:** n/a (environmental; evidence in this summary + deferred-items.md)

---

**Total deviations:** 4 auto-fixed (4 blocking-class, 0 bugs)
**Impact on plan:** All four were gate-enabling, not scope creep. No architectural change; D-06's "no new tier-preference layer" holds — the diff touches no routing-decision logic, only the store + replay feeding the existing seams.

## Issues Encountered

None beyond the deviations above. The RED evidence gate (tdd-red-evidence) needed `go test -json` output converted to node-TAP for its parser — a one-off evidence-format conversion, not a code issue.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 24-02 (recording sites + live-path wiring) has the full tested core: `NewOutcomeStore`/`Append`/`ReadOutcomes`/`ReplayBreakers`/`ReplayCostTracker`/`FirstAllowed` + exported `ProviderModelKey` for `SetBreakers` consumption. Zero-token semantics documented on the type (unknown-on-path).
- Residual baseline (pre-existing, out of scope): two factory.go gocritic hugeParam findings; repo-wide lint under 2.13.2 still reports ~572 findings on pre-Phase-24 files; TestEscalation_ReapAllUsesLadder flakes under full-suite load; go.mod/go.sum (and ~1000 other files) carry environment-induced 100644→100755 mode churn — unstaged, not committed by this plan (except the six files whose content changes required staging).

## Self-Check: PASSED

- All 4 created/modified key files exist on disk (outcomes.go, outcomes_agg.go, outcomes_test.go, SUMMARY).
- All 4 task commits verified in git log (5552942, 25b977b, 25d9e13, ce424ea).
- Plan-level verification re-run green: `go test -race -count=1 ./internal/modelrouting/` ok (incl. all TestOutcome*/TestFirstAllowed); `go vet` ok.
- go.mod content unchanged (mode-churn only) — zero new dependencies.

