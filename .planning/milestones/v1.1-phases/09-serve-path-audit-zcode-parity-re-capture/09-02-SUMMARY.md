---
phase: 09-serve-path-audit-zcode-parity-re-capture
plan: 02
subsystem: engine-decision-provenance
tags: [audit, aud-04, provenance, engine, pattern-table, transcript]
status: complete

# Dependency graph
requires:
  - phase: Phase 4 (engine observe/decide) + 08-06 (chaining vocab)
    provides: Decide, EngineDecisionWriter, the OpenSpec table
provides:
  - "engine.MatchDetail{ID,Action,Span,ConfigSource} — PatternTable matches carry full provenance"
  - "Decision.MatchedSpan + Decision.ConfigSource → event.EngineDecision + engine_decision transcript line"
  - "the AUD-04 differentiator: one engine_decision line answers 'why did it continue' (D-03)"
affects: [09-06 (audit mirror formats the same fields)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "table-supplied ConfigSource keeps internal/engine free of openspec knowledge (dependency direction openspec → engine preserved)"
    - "tool-signal span = the tool-call NAME (TurnOutput carries names) — documented in the field comment, not accidental"

key-files:
  created: []
  modified:
    - internal/engine/types.go
    - internal/engine/decide.go
    - internal/engine/decide_test.go
    - internal/engine/dispatch_test.go
    - internal/engine/observe.go
    - internal/engine/observe_test.go
    - internal/event/events.go
    - internal/session/manager.go
    - internal/session/manager_test.go
    - internal/session/transcript.go
    - internal/openspec/patterntable.go
    - internal/openspec/register_test.go

key-decisions:
  - "PatternTable interface changed to return MatchDetail (zero value = no match) — a breaking interface change, taken in one step with all fakes adapted; no non-test implementers existed beyond OpenSpecPatternTable"
  - "flat 6-param AppendEngineDecision (plan-specified) — same mechanical style as the prior 4-param seam"

patterns-established:
  - "FindString span extraction at the table layer — the ONLY place the span exists"

requirements-completed: [AUD-04]

# Metrics
duration: 70min
completed: 2026-08-15
---

# Phase 9 Plan 02: Engine-decision provenance Summary

**Every engine_decision line now answers "why did it continue" from the line alone: action + signal + matched span + config source + source turn (AUD-04's differentiator, locked D-03) — threaded from the pattern-table match site (where the span exists) through Decide → Decision → bus event → transcript line, with zero engine-behavior change.**

## Performance

- **Duration:** ~70 min
- **Tasks:** T1 complete (RED `496d692` → GREEN `a696c4e`), T2 complete (RED `a880d78` → GREEN `853f58c`)

## Accomplishments

- **T1** — `engine.MatchDetail{ID, Action, Span, ConfigSource}`; `PatternTable.MatchText/MatchTool` return it (zero value = no match); `Decision` gains `MatchedSpan` + `ConfigSource`; `Decide` rewritten to consume MatchDetail with signal construction, dual-signal attribution (text wins), and reasons byte-identical — pure (D-01) unchanged. `OpenSpecPatternTable.MatchText` extracts the LITERAL substring via `FindString` and names its entries (`openspec.toml patterns/<id>` / `handoff_tools/<id>`); tool span = the tool NAME. Six behavior tests: text-span, tool-span-is-name, unmatched-lean, dual-signal-text-span, FindString literal, config-source naming.
- **T2** — `event.EngineDecision` + `Line` gain the two fields (additive); `EngineDecisionWriter.AppendEngineDecision` extended to 6 params; `emit` passes span + source to BOTH the bus event and the Manager write in lockstep; the engine_decision line payload carries them (`matchedSpan`, `configSource`). Tests: the event carries provenance (via the buffered capture helper); the transcript line alone answers "why" (turn + action + signal + span + source all asserted on ONE line); nothing-decisions keep exactly-one-per-turn cadence with EMPTY provenance; write failures stay logged-not-fatal (existing degradation suite green).

## Task Commits

1. **T1 RED** `496d692` → **GREEN** `a696c4e`
2. **T2 RED** `a880d78` → **GREEN** `853f58c`

**Plan metadata:** this commit

## Deviations from Plan

None. (The first T2 test draft deadlocked on an undrained bus subscription — D-05 backpressure working as designed; fixed with the existing buffered capture helper before commit.)

## Issues Encountered

- `go test` package timeout on the first T2 run (the deadlock above) — diagnosed from the goroutine dump (`Publish` blocked on chan send), not worked around.

## TDD Gate Compliance

RED→GREEN both tasks; `go test ./internal/engine/ ./internal/session/ ./internal/event/ ./internal/openspec/ -race -count=1` green; `mise run ci` green (exit 0); lint 0 issues.

## Verification (re-runnable)

- `go test ./internal/engine/ -race -run 'TestDecide_.*Span|TestDecide_UnmatchedStaysLean' -v`
- `go test ./internal/openspec/ -race -run 'TestPatternTable_MatchDetail' -v`
- `go test ./internal/engine/ -race -run TestObserve_EmitsProvenance -v` (event + cadence)
- `go test ./internal/session/ -race -run TestAppendEngineDecisionProvenance -v` (the AUD-04 acceptance)

---
*Phase: 09-serve-path-audit-zcode-parity-re-capture*
*Completed: 2026-08-15*
