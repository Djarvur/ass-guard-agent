---
phase: 24-documentation-ops-tails
plan: "02"
subsystem: modelrouting
tags: [outcome-store, circuit-breaker, replay, model-routing, cli, tail-01]

requires:
  - phase: 24-documentation-ops-tails (plan 24-01)
    provides: DispatchOutcome/OutcomeStore/ReadOutcomes/ReplayBreakers/FirstAllowed/Aggregate + exported ProviderModelKey
provides:
  - Live outcome recording at the REAL dispatch sites — every completed provider attempt on the parent turn path (runTurn → streamAndEmit) and the subagent path (defaultSubagentRunner → streamAndEmitTaggedProf) appends one mechanical record to .ass-guard/routing/outcomes.jsonl
  - Session injection seams Outcomes/ProviderName/SessionTier + an injectable outcomeNow clock (nil-allowed, the Hooks/Checkpointer pattern; nil store = byte-identical behavior)
  - `ass-guard model-routing stats` CLI (--store/--json/--config): aggregated per-(provider,model) counts + token/cost totals, human→stderr / --json→stdout only
  - Feedback consumption (D-06 observable): ConfigSurface.SetOutcomeBreakers + resolveModelLocked FirstAllowed demotion, resolveSubagentModel breakers parameter + Runner's lazy one-time replay load, acp_serve composition wiring
  - modelrouting.OutcomeStorePath read-side helper
affects: [24-05 (stats dump CLI family), routing operators, future breaker-aware surfaces]

actuals:
  tokens: 15773   # chars/4 over the realized diff (63095 chars, 13 code files)
  tasks: 3
  commits: 6      # measured: git rev-list --count 1ebdaab..HEAD

tech-stack:
  added: []        # stdlib + in-tree only (go.mod content unchanged)
  patterns:
    - "Live-path recording bracket: counters reset at attempt head, clock read before the attempt, one record the moment it completes (success/stream-error/cancel alike)"
    - "Replay demotion rides the EXISTING Breaker seam over the chain Resolve already returned — no new tier-preference layer (D-06)"
    - "Demotion observability window: a breaker denies only while its cooldown runs — test seeds must sit inside the cooldown of the resolution clock"

key-files:
  created:
    - internal/session/session_outcomes_test.go
    - internal/acpserve/config_surface_outcomes_test.go
  modified:
    - internal/session/session.go
    - internal/session/subagent.go
    - internal/runtime/runtime.go
    - internal/runtime/apply_model_test.go
    - internal/acpserve/config_surface.go
    - internal/acpserve/acp_serve.go
    - internal/modelrouting/outcomes.go
    - internal/modelroutingcmd/modelrouting.go
    - cmd/ass-guard/modelrouting.go
    - cmd/ass-guard/modelrouting_test.go
    - cmd/ass-guard/cli_contract_test.go

key-decisions:
  - "Recording sits at the REAL sites (OQ2 resolution): the turn loop's streamAndEmit bracket and the subagent runner's tagged-stream bracket — Scheduler.Dispatch stays unwired (zero production callers, Pitfall 1)"
  - "An unclassified stream error classifies transient (the conservative class) — unknown failures feed breaker evidence, never silently drop"
  - "All-denied chain keeps the primary with one loud note on both Resolve sites — a resolution never fails over evidence"
  - "Runner's replayed breakers load lazily ONCE (the effectiveCompaction precedent; NewRunner stays struct-fill-only) — acp_serve.go wires the surface's map at composition, exactly once"
  - "Test seeds must be stamped inside the breaker cooldown window of the resolution clock — records older than the cooldown admit the half-open probe and the demotion is unobservable"
  - "subagent records carry 0/0 tokens (unknown-on-path, never free) — the tagged subagent stream publishes no usage"

patterns-established:
  - "Outcome bracket: per-attempt summed usage counters (Add, not last-wins) alongside the UsageUpdate publish; injectable clock stamps At and measures latency"
  - "Store failure = exactly ONE stderr note + turn proceeds (T-24-02-01); store construction failure = recording disabled + one note"

requirements-completed: [TAIL-01]   # shared with 24-01 (which has its SUMMARY) — this is the last declaring plan; ready-ids flips the checkbox now

coverage:
  - id: D1
    description: "Live record path: one completed parent-turn provider attempt appends exactly one DispatchOutcome with real provider/effective model/tier/outcome class/latency and summed streamed usage; transient ProviderError records transient; broken store = one stderr note + turn completes; nil store records nothing"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "internal/session/session_outcomes_test.go#TestSessionOutcomeRecording"
        status: pass
    human_judgment: false
  - id: D2
    description: "Subagent dispatch records origin subagent with the effective (planner-overridden) subagentProfile model — proven through the REAL nested runner"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "internal/session/session_outcomes_test.go#TestSessionOutcomeRecording/subagent_dispatch_records_origin_subagent_with_the_override_model"
        status: pass
    human_judgment: false
  - id: D3
    description: "ass-guard model-routing stats CLI: per-(provider,model) counts + token/cost totals to STDERR (stdout byte-clean), --json round-trips seeded counts to STDOUT only, missing store = empty-aggregate note at exit 0, --store routes; registered in the model-routing group; CLI contract golden regenerated"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "cmd/ass-guard/modelrouting_test.go#TestSchedulingStatsHuman"
        status: pass
      - kind: unit
        ref: "cmd/ass-guard/modelrouting_test.go#TestSchedulingStatsJSON"
        status: pass
      - kind: unit
        ref: "cmd/ass-guard/modelrouting_test.go#TestSchedulingStatsEmptyStore"
        status: pass
      - kind: unit
        ref: "cmd/ass-guard/modelrouting_test.go#TestSchedulingStatsStoreFlagRoutes"
        status: pass
      - kind: other
        ref: "command: built binary over a seeded JSONL store — human dump to stderr only (stdout 0 bytes), --json to stdout only (stderr 0 bytes), aggregates exact"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-06 observable at the config-surface Resolve site: a seeded store opening the primary's replayed breaker demotes resolveModelLocked to the first allowed fallback (advertisement reflects it, one loud note); empty breaker map = byte-identical primary resolution, no note"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "internal/acpserve/config_surface_outcomes_test.go#TestOutcomeBreakersDemoteSessionModel"
        status: pass
      - kind: unit
        ref: "internal/acpserve/config_surface_outcomes_test.go#TestOutcomeBreakersEmptyMapKeepsPrimary"
        status: pass
    human_judgment: false
  - id: D5
    description: "D-06 observable at the subagent light-tier Resolve site: open light primary returns the fallback slug via FirstAllowed (one note); empty breakers and all-denied chains keep the primary; the single production caller passes the Runner's replayed map; acp_serve composition calls SetOutcomeBreakers exactly once"
    requirement: TAIL-01
    verification:
      - kind: unit
        ref: "internal/runtime/apply_model_test.go#TestResolveSubagentModelBreakerDemotion"
        status: pass
      - kind: other
        ref: "command: grep — SetOutcomeBreakers exactly once in acp_serve.go; resolveSubagentModel's only production call site passes r.outcomeBreakersFor()"
        status: pass
    human_judgment: false

duration: 51 min
completed: 2026-09-10
status: complete
plan_head_before: 1ebdaab01b404a48d017cc6c40a7e20a22d325c8
---

# Phase 24 Plan 02: TAIL-01 live wiring Summary

**The 24-01 outcome store wired into the live turn path (real dispatch sites → JSONL records with usage/latency/class), an `ass-guard model-routing stats` CLI, and replayed breaker evidence bending BOTH live Resolve sites — built tracer-first then test-first (2x RED_EVIDENCE_OK → GREEN → REFACTOR)**

## Performance

- **Duration:** 51 min
- **Started:** 2026-09-10T15:37:56Z
- **Completed:** 2026-09-10T16:29:21Z
- **Tasks:** 3 (tracer + 2 TDD)
- **Files modified:** 13 (2 created, 11 modified)

## TDD Gate Compliance

- **Task 1 (tracer, not tdd-marked):** `9e976c6` feat — TestSessionOutcomeRecording (5 subtests) green under `-race` from the introducing commit; tracer feedback gate re-ran the verify end-to-end (auto mode) before expansion.
- **Task 2 RED:** `c6ab947` test — 4 TestSchedulingStats* tests fail on assertions vs compiling stubs; `check tdd-red-evidence` → `RED_EVIDENCE_OK / target_test_failed` (record: 24-02-task2-red-evidence.json).
- **Task 2 GREEN:** `2f74d75` feat — all 4 pass; full cmd suite green.
- **Task 3 RED:** `d2d9091` test — TestOutcomeBreakersDemoteSessionModel + TestResolveSubagentModelBreakerDemotion fail on assertions (the empty-map guard passes by design — it pins pre-change behavior); `RED_EVIDENCE_OK / target_test_failed` (record: 24-02-task3-red-evidence.json).
- **Task 3 GREEN:** `b85de05` feat — demotion suite green; all four touched packages green.
- **REFACTOR:** `8169832` refactor — lint conformance (exhaustive switch, lll wraps, funlen/funcorder nolints, test-fixture restructure); behavior unchanged.
- Note: Task 1 (tracer, no `tdd` mark) legitimately committed `feat(24-02)` before Task 2's RED — it implements the record path, not the stats-CLI behavior the gate guards. Both TDD tasks have their own test-before-feat ordering.

## Accomplishments

- **Live record path (Task 1):** Session gains nil-allowed `Outcomes`/`ProviderName`/`SessionTier` + injectable `outcomeNow` clock; runTurn brackets every provider attempt (usage counters summed alongside the UsageUpdate publish, latency measured, one `DispatchOutcome` appended the moment the attempt completes — ok/transient/structural/exhausted from `errors.As` on the stream error, unclassified → transient). The subagent runner mirrors one record per stream attempt with origin subagent and the effective (possibly overridden) profile model. Store Append failure = exactly ONE stderr note, never a turn failure (T-24-02-01); nil store = byte-identical. `sessionFor` wires store + provider slug + session tier (construction failure → recording disabled + one note).
- **Stats CLI (Task 2):** `ass-guard model-routing stats [--store] [--json] [--config]` — `LoadAndAggregate` (tolerant read + `Aggregate`), `BuildStatsRows` (deterministic sort + tier-binding enrichment), `EmitStatsHuman` (stderr, write errors ignored) / `EmitStatsJSON` (the only stdout path) mirroring the EmitResolve* discipline. Missing store = empty aggregate at exit 0. CLI contract golden regenerated from the binary (18-06 transcription rule).
- **Feedback consumption (Task 3):** `ConfigSurface.SetOutcomeBreakers` + `demoteIfDeniedLocked` (FirstAllowed walk over primary+fallbacks, one loud note; empty map = zero change; all-denied keeps the primary); `resolveSubagentModel` takes the breakers parameter and demotes the same way; `Runner.outcomeBreakersFor()` lazily loads the replay once (ReadOutcomes + ReplayBreakers with schedCfg's thresholds); the acp_serve Run composition wires `SetOutcomeBreakers` exactly once (absent file → empty map, no note).

## Task Commits

1. **Task 1 (tracer): live record path** — `9e976c6` (feat)
2. **Task 2 (RED): failing stats CLI suite** — `c6ab947` (test)
3. **Task 2 (GREEN): stats dump implementation** — `2f74d75` (feat)
4. **Task 3 (RED): failing demotion suite** — `d2d9091` (test)
5. **Task 3 (GREEN): both Resolve sites demote** — `b85de05` (feat)
6. **REFACTOR: lint conformance** — `8169832` (refactor)

## Files Created/Modified

- `internal/session/session.go` — Outcomes/ProviderName/SessionTier/outcomeNow seams, attempt counters, recordDispatchOutcome + classifyStreamOutcome
- `internal/session/subagent.go` — the subagent attempt bracket (origin subagent, effective model)
- `internal/session/session_outcomes_test.go` — TestSessionOutcomeRecording battery (created)
- `internal/runtime/runtime.go` — sessionFor wiring, sessionTierOrHeavy, outcomeBreakersFor lazy load, resolveSubagentModel breakers param + demotion
- `internal/runtime/apply_model_test.go` — TestResolveSubagentModelBreakerDemotion battery + light-tier fixtures
- `internal/acpserve/config_surface.go` — outcomeBreakers field, SetOutcomeBreakers, demoteIfDeniedLocked
- `internal/acpserve/config_surface_outcomes_test.go` — surface demotion tests (created)
- `internal/acpserve/acp_serve.go` — replayOutcomeBreakers composition wiring
- `internal/modelrouting/outcomes.go` — OutcomeStorePath read-side helper
- `internal/modelroutingcmd/modelrouting.go` — LoadAndAggregate/BuildStatsRows/StatsRow/EmitStatsHuman/EmitStatsJSON
- `cmd/ass-guard/modelrouting.go` — newModelRoutingStatsCmd + flags
- `cmd/ass-guard/modelrouting_test.go` — TestSchedulingStats* battery + seedStatsStore fixture
- `cmd/ass-guard/cli_contract_test.go` — golden: stats joins the Available Commands rows

## Decisions Made

See key-decisions in the frontmatter; all six follow the plan's letter (OQ2 recording-at-Send/Stream-sites resolution, no new tier-preference layer, graceful-degradation-plus-loud-note house pattern).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] CLI contract golden regenerated**
- **Found during:** Task 2 GREEN (`go test ./cmd/ass-guard/`)
- **Issue:** TestCLIBinaryContractSupportCommands pins `model-routing --help` byte-exact; the new stats subcommand changes the Available Commands rows.
- **Fix:** Golden regenerated FROM the built binary's output (the 18-06 transcription rule, STATE precedent) — not in the plan's file list but the required collateral of registering the command.
- **Files modified:** cmd/ass-guard/cli_contract_test.go
- **Verification:** full cmd suite green
- **Committed in:** 2f74d75

**2. [Rule 1 - Bug] Test seeds must sit inside the breaker cooldown window**
- **Found during:** Task 3 GREEN (first run: demotion tests still red)
- **Issue:** Seeded records stamped with fixed past timestamps let the 60s cooldown elapse — `Allow(now)` transitioned the breaker to HalfOpen and admitted the probe, so the demotion was unobservable through no fault of the production change.
- **Fix:** Seeds stamped relative to the resolution clock (surface: `time.Now()`-relative; runtime: `resolveAt`-relative via the new `resolveAt` seed parameter). Documented as a pattern — the demotion is only observable while the cooldown runs.
- **Files modified:** internal/acpserve/config_surface_outcomes_test.go, internal/runtime/apply_model_test.go
- **Verification:** demotion suite green under -race
- **Committed in:** b85de05

**3. [Rule 2 - Missing Critical] modelrouting.OutcomeStorePath helper**
- **Found during:** Task 3 implementation
- **Issue:** The store-path convention lives on unexported constants; the acp_serve composition and the Runner's lazy load both need the path WITHOUT constructing a store (which mkdirs).
- **Fix:** Exported `OutcomeStorePath(root)` beside the constants it reuses — one source of truth, no triplicated literals.
- **Files modified:** internal/modelrouting/outcomes.go
- **Verification:** go vet/build green; both consumers wired
- **Committed in:** b85de05

**4. [Rule 3 - Blocking] `mise` absent — gate legs run directly; lint conformance pass**
- **Found during:** plan-level verification
- **Issue:** `mise test`/`mise ci` cannot run (binary absent on this host — the documented 24-01/23-phase environmental condition). First lint pass showed findings on the plan's files (gocognit/goconst/lll/exhaustive/thelper/funlen/funcorder).
- **Fix:** Legs run directly — `go vet ./...` green, `CGO_ENABLED=0 go build ./...` green, `go test -race -count=1 ./...` (37 packages ok; the four FAILs are the DOCUMENTED pre-existing set: TestPermissionsE2E, TestRescanConcurrency, evalsuite openspec-not-on-PATH, coreexec load-flake — coreexec passes isolated and does not import the touched packages), lint via golangci-lint 2.13.2 with a conformance refactor commit; verified against a scratch-worktree base that the only funcorder/cyclop delta across acpserve is the one new method (now nolinted beside its caller, the file's established grouping).
- **Files modified:** the six refactor-commit files
- **Verification:** lint clean on every file this plan touches (base comparison for the pre-existing flood)
- **Committed in:** 8169832

---

**Total deviations:** 4 auto-fixed (1 bug, 1 missing critical, 2 blocking)
**Impact on plan:** All gate-enabling or required collateral; no architectural change, no scope creep. D-06's "no new tier-preference layer" holds — demotion rides the existing Breaker seam over chains Resolve already returned.

## Issues Encountered

None beyond the deviations above. The RED evidence checker lives in the .zcode-scoped gsd-tools (the .claude copy predates the verb) — an environment note, not a code issue.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- TAIL-01 is fully live: record (real dispatch sites) → replay (existing seams) → consume (both live Resolve sites bend) → observe (stats CLI). Plan 24-05 (TAIL-03) builds its CLI family adjacent to `model-routing stats`.
- Residual baseline (pre-existing, out of scope): repo-wide lint under 2.13.2 still reports ~572 findings on pre-Phase-24 files; TestPermissionsE2E (cross-workstream), TestRescanConcurrency (phase-22 deferred race), evalsuite env-gating, coreexec load-flake; go.mod/go.sum mode churn.

## Self-Check: PASSED

- Both created key files exist on disk (session_outcomes_test.go, config_surface_outcomes_test.go).
- All 6 task commits verified in git log (9e976c6, c6ab947, 2f74d75, d2d9091, b85de05, 8169832).
- Plan-level verification re-run: vet green, build green, race suite green-modulo-documented, E2E CLI demo exact (human stderr-only / json stdout-only / missing-store note).
- go.mod content unchanged (mode-churn only) — zero new dependencies.
