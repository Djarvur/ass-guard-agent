---
phase: 19-compaction-cache-control
plan: "03"
subsystem: session
tags: [compaction, projector, transcript, context-window, tdd]

# Dependency graph
requires:
  - phase: 16-acp-wire-foundation
    provides: TypeCompaction transcript kind + AppendCompaction (16-02, D-21 rich boundary record)
provides:
  - Summary field on the compaction marker line (additive, redacted append path)
  - The Projector's compaction reset-point class — durable summary seed (D-06)
  - Budget-fill pair-safe post-marker tail cut with the injected CompactionTailBudget / SetCompactionTailBudget seam
affects: [19-04 (engine wiring), 19-05 (config keys), PAR-01 verification]

# Actuals (#2632) — pairs with the plan's estimate (30000) to calibrate future estimates.
actuals:
  tokens: 12700   # chars/4 over the realized diff (50919 diff chars)
  tasks: 3
  commits: 7

tech-stack:
  added: []   # stdlib only
  patterns:
    - "durable reset-point class: marker summary survives later boundaries; boundary between marker and turn drops only the TAIL (seed from marker, tail from last reset point)"
    - "budget-fill cut: newest-first group walk, stop-before-exceed, newest-group floor, boundMidTurn advance guard kept verbatim"
    - "foldExchanges: the accumulateMidTurn fold generalized with a filterTurn flag (no duplicated fold logic)"

key-files:
  created: []
  modified:
    - internal/session/transcript.go
    - internal/session/manager.go
    - internal/session/projector.go
    - internal/session/projector_test.go
    - internal/session/transcript_newkinds_test.go
    - internal/session/gate_test.go

key-decisions:
  - "Marker seed reuses the exact lean-seed wrapper (seedContent single-sourced); the text is sourced from the marker — the plan's 'reuse the shape, source it from the marker' letter"
  - "A TypeBoundary after the marker drops only the tail (existing discipline: current turn's own accumulation); the seed stays the marker's summary ALONE (D-06 + Open Question 3's pinned summary-only reading)"
  - "Budget math: message cost = folded JSON size / 4 (truncating); allowance = 60% of the injected limit − summary chars/4; zero/unset budget falls back to boundMidTurn's 64-message count"
  - "accumulateMidTurn generalized into foldExchanges (filterTurn) rather than duplicated; boundMidTurn itself untouched for the no-marker path"
  - "TestProjectorToleratesNewKinds updated per the 21-03 activation precedent: the marker is an ACTIVE reset point now, so its inertness interleave moved out (coverage lives in TestProjector_CompactionResetPoint)"

patterns-established:
  - "Reset-point classes with per-class seed semantics: the scan stays positional (strictly before the projected turn's user message), the seed source differs by winning kind"
  - "Injector seam naming for cross-plan wiring: CompactionTailBudget/SetCompactionTailBudget are exported under the exact names the dependent plan pins"

requirements-completed: [PAR-01]

coverage:
  - id: D1
    description: "Summary payload home on the compaction marker line (redacted append path, exact round-trip, field-tolerant pre-field parse)"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/transcript_newkinds_test.go#TestTranscriptNewKinds/compaction_summary_payload_rides_the_redacted_path_and_round-trips"
        status: pass
    human_judgment: false
  - id: D2
    description: "Durable reset-point class in the Projector: marker-wins seed, boundary-after-marker durability (summary-only), no-marker byte-identity, subagent in-flight safety, most-recent-marker selection"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/projector_test.go#TestProjector_CompactionResetPoint"
        status: pass
    human_judgment: false
  - id: D3
    description: "Budget-fill pair-safe tail cut: multi-turn post-marker tail, group-head cut, no orphaned tool result, 60% fill target, zero-budget 64-message fallback, thinking untouched in kept groups"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/projector_test.go#TestProjector_CompactionTailCut"
        status: pass
    human_judgment: false

# Metrics
duration: 47min
completed: 2026-09-06
status: complete
---

# Phase 19 Plan 03: Compaction Reset-Point + Tail-Cut Projection Semantics Summary

**Durable compaction reset-point class in the Projector (marker summary survives later boundaries, D-06) plus the budget-fill pair-safe tail cut, with the summary payload landed additively on the 16-02 marker line**

## Performance

- **Duration:** 47 min
- **Started:** 2026-09-06T20:44:39Z
- **Completed:** 2026-09-06T21:31:30Z
- **Tasks:** 3 (each RED→GREEN under TDD mode)
- **Files modified:** 6

## Accomplishments
- Summary payload on the compaction line: `Line.Summary` (json `summary`, omitempty, field-additive under D-20) + `AppendCompaction` extended with the summary parameter; the marker rides the REDACTED append path (counting-fake proof, exactly 1 call) and pre-field markers parse with an empty Summary
- The Projector's third reset-point class: a marker before the projected turn's user message wins the scan — the seed is the marker's Summary in the reused lean-seed wrapper, DURABLE across later TypeBoundary lines (summary-only after a boundary, per the planner-pinned Open Question 3 reading), while transcripts without a marker project byte-identically (explicit hand-derived pin over a boundary-only transcript)
- Subagent safety pinned (Pitfall 5): a marker landing mid-subagent-turn never touches that turn's own window; the next parent turn gets the summary seed; a mid-flight marker of the projected turn itself is not its reset point
- Budget-fill tail cut (D-04/D-05): multi-turn post-marker fold through the generalized `foldExchanges`, newest-first group walk with stop-before-exceed + newest-group floor, `boundMidTurn`'s orphan-advance guard kept verbatim, message cost = folded JSON/4, fill target = 60% of the injected limit minus the summary estimate, zero-budget fallback to the 64-message count
- The injector seam landed under the exact pinned names `CompactionTailBudget` / `SetCompactionTailBudget` (19-04's session wiring target); thinking blocks inside kept groups survive untouched and dropped groups take their thinking with them

## Task Commits

Each task was committed atomically (TDD: RED then GREEN):

1. **Task 1: Summary payload on the compaction line** — `08f333a` (test, RED) + `f0ad329` (feat, GREEN)
2. **Task 2: Durable reset-point semantics in the Projector** — `88c856e` (test, RED) + `22fee29` (feat, GREEN)
3. **Task 3: Budget-fill pair-safe tail cut** — `0441ffe` (test, RED) + `46e38ca` (feat, GREEN)
4. **Race fix surfaced by Task 3 verification** — `a5a46a1` (fix)

**Plan metadata:** (docs commit follows)

## Files Created/Modified
- `internal/session/transcript.go` — `Summary` field on the compaction group of `Line`
- `internal/session/manager.go` — `AppendCompaction` extended with the summary parameter (redacted path, doc'd T-19-05/06)
- `internal/session/projector.go` — compaction reset-point path (`compactionMarkerIdx`, `projectCompacted`, `boundaryAfterMarker`), `seedContent` single-sourcing the wrapper, `foldExchanges` generalization, `boundCompactionTail`/`boundCompactionTailByBudget`/`messageCost`, the `CompactionTailBudget` injector
- `internal/session/projector_test.go` — `TestProjector_CompactionResetPoint` (5 subtests) + `TestProjector_CompactionTailCut` (3 subtests) + shared helpers
- `internal/session/transcript_newkinds_test.go` — `testCompactionSummaryPayload` battery; tolerance-pin activation update
- `internal/session/gate_test.go` — the allow_once race fix (deviation 1)

## Decisions Made
- Marker seed wrapper reused verbatim from the mechanical path (`seedContent` now single-sources it) — the plan's "reuse the shape, source it from the marker" letter; one wrapper, two summary sources
- D-06 tail discipline implemented as: a boundary between marker and turn drops the post-marker tail (window = seed + the current turn's own accumulation, the pre-phase between-turn shape); only the summary is durable
- Budget floor: the newest group is always kept when a budget is set (a tiny allowance must not produce an empty window); an oversized summary shrinks the tail to that floor, never the window (T-19-07)
- `foldExchanges(lines, from, turnID, filterTurn)` generalization instead of a duplicated post-marker fold — the folding rules stay single-sourced; `boundMidTurn` untouched (its existing tests pass unmodified)
- 16-02 tolerance pin updated per the 21-03 activation precedent (planned, not a deviation): the compaction interleave left the inertness fixture, activation coverage lives in the new battery

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] gate_test allow_once subtest raced the transcript append**
- **Found during:** Task 3 verification (full-suite runs)
- **Issue:** `TestGateOutcomeMatrix/allow_once_executes_without_persisting` read `toolResultsFor` without the `gateWaitFor` its two sibling subtests use; the result line trails the exec record under parallel load, and 19-03's new parallel batteries widened the window to a ~50% full-suite flake (verified: pre-19-03 passes 4/4, my tree flaked before the fix, 10/10 after)
- **Fix:** Added the missing `gateWaitFor` before the result read (the sibling pattern, lines 991/1046)
- **Files modified:** internal/session/gate_test.go
- **Verification:** 10 consecutive `go test -race ./internal/session/ -count=1` runs green
- **Committed in:** a5a46a1

---

**Total deviations:** 1 auto-fixed (Rule 1 bug)
**Impact on plan:** No scope creep — a one-line test-robustness fix matching its siblings' established pattern.

## Issues Encountered

- **mise lint gate broken repo-wide by golangci-lint version drift (pre-existing, out of scope):** mise resolves golangci-lint to 2.13.2 (built go1.27.0) while `.golangci.yml` was tuned for 2.12.x — the `exhaustruct`→`exhaustruct_v5` rename defeats the config's wildcard exclusion and new/renamed linters flag pre-existing code across every package (reproduced on `internal/shaper`, untouched since Phase 15; the stale `~/go/bin` 2.12.2 binary also panics against the go1.27.1 toolchain). Logged to `deferred-items.md`; all NEW code in this plan was kept clean of findings under 2.13.2 beyond the renamed-linter noise.
- **internal/runtime full-package flakes (pre-existing, out of scope):** `TestIntegration_RealStreamingThroughACP` and `TestAskPark_PromptResponsePrecedesResolution` fail intermittently under machine load — reproduced identically at the pre-19-03 commit `00c9da6`; both pass in isolation and the full `go test -race -count=1 ./...` sweep passed on 2026-09-07. Logged to `deferred-items.md`.

## TDD Gate Compliance

All three behavior-adding tasks followed RED→GREEN with per-phase commits:
`test(19-03)` RED commits (08f333a, 88c856e, 0441ffe) each precede their `feat(19-03)` GREEN commits (f0ad329, 22fee29, 46e38ca). RED failures verified for the right reasons (missing field/signature compile errors; four marker-activation subtests failing while the no-marker pins passed).

## Verification

- `go test -race ./internal/session/ -count=1` — green (10 consecutive runs post gate-fix), including `TestProjector_CompactionResetPoint` and `TestProjector_CompactionTailCut`
- `go test -race ./internal/session/ ./internal/runtime/ -count=1` — session green; runtime intermittently flaky per Issues (pre-existing, reproduced at pre-19-03)
- `go vet ./...` — clean; `CGO_ENABLED=0 go build ./...` — clean
- `go test -race -count=1 ./...` (full repo) — green on the closing run
- No-marker byte-identity: full pre-phase projector suite passes (only the planned tolerance-pin activation update); the explicit boundary-only hand-derived pin holds
- 16-02 tolerance pins: kinds inert without markers (local_command, foreign-turn thinking, unknown kinds), marker active as a reset point (new battery)

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- PAR-01's projection semantics are pinned; 19-04 (the compaction engine) lands against them: the summarizer writes markers via the extended `AppendCompaction`, and the session wires the modelrouting-resolved context window through `SetCompactionTailBudget` (the exact-name seam this plan exported)
- The lint-gate reconciliation (golangci-lint 2.12↔2.13 config drift) should land before the next phase close that relies on `mise ci`

## Self-Check: PASSED

- Files exist: transcript.go, manager.go, projector.go, projector_test.go, transcript_newkinds_test.go (modified), gate_test.go (modified) — all present in the 19-03 commit range
- Commits 08f333a, f0ad329, 88c856e, 22fee29, 0441ffe, 46e38ca, a5a46a1 all present on gsd/v1.2-claude-code-parity

---
*Phase: 19-compaction-cache-control*
*Completed: 2026-09-06*
