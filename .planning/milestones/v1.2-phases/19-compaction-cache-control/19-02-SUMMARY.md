---
phase: 19-compaction-cache-control
plan: 02
subsystem: provider
tags: [anthropic, sse, error-handling, provider-errors, overflow, tdd]

# Dependency graph
requires:
  - phase: 08-09 provider streaming
    provides: the raw-HTTP Stream path, the SSE idle watchdog, and the error-chunk emission shape (chunkError) the turn loop already consumes
provides:
  - Non-2xx stream rejections surfaced as ClassifyHTTP-typed error chunks (RESEARCH Pitfall 1 closed) — any provider rejection is now observable at the turn loop
  - IsOverflow(err) message-class predicate over *ProviderError — PAR-01's retry-once trigger, consumed by 19-04's runTurn branch
affects: [19-04 (retry-once recovery consumes IsOverflow), 19-03 (compaction trigger rides the same error path), PAR-01]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
# Same estimateTokens scale (chars/4 over the realized diff), never a harness token count.
actuals:
  tokens: 3530    # chars/4 over the realized diff (14123 chars / 4)
  tasks: 2        # tasks completed
  commits: 4      # commits made (2 RED/GREEN TDD pairs)

# Tech tracking
tech-stack:
  added: []       # stdlib only — no new dependencies
  patterns:
    - "status-check-at-send-site: rejected bodies never reach the SSE drain — bounded envelope read + synchronous close + ClassifyHTTP + error chunk on a 1-buffered channel"
    - "message-class matcher predicate: IsOverflow follows the isNetOrContextError family style instead of adding an ErrorKind (typed-Kind discipline D-04 preserved)"

key-files:
  created: []
  modified:
    - internal/provider/streaming.go
    - internal/provider/streaming_test.go
    - internal/provider/errors.go
    - internal/provider/errors_test.go

key-decisions:
  - "Non-2xx check sits between the httpClient.Do error return and the drain-path channel construction; the rejected body is read bounded (8 KiB, T-19-03), closed synchronously, classified through ClassifyHTTP(anthropic, prof.Model, status, cause), and emitted as one error chunk — drainSSE never sees it"
  - "Malformed/empty/truncated error bodies degrade to the generic structural ProviderError carrying the status code — the envelope parser can never panic (prohibition)"
  - "IsOverflow is a matcher, not a Kind: errors.As to *ProviderError + case-insensitive contains of the prompt-is-too-long class over Error() (which carries the envelope cause text); the exact wording is community-sourced (A1), live-endpoint re-verification is 19-04's E2E concern"

patterns-established:
  - "Provider rejection surfacing: the error envelope's type+message ride the ProviderError Cause so downstream message-class predicates match without re-parsing bodies"
  - "TDD pair per task in one plan: RED commit pins the swallow (done chunk with defaulted end_turn) before the fix exists"

requirements-completed: [PAR-01]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "Non-2xx stream responses surface as classified error chunks (never a defaulted end_turn done chunk); malformed bodies degrade safely"
    requirement: "PAR-01"
    verification:
      - kind: unit
        ref: "internal/provider/streaming_test.go#TestStream_Non2xxError"
        status: pass
      - kind: unit
        ref: "internal/provider/streaming_test.go#TestStream_Non2xxError_MalformedBody"
        status: pass
      - kind: command
        ref: "go test -race ./internal/provider/ -count=1 (full suite unchanged)"
        status: pass
    human_judgment: false
  - id: D2
    description: "IsOverflow message-class predicate — true only for the prompt-is-too-long class over *ProviderError, no new ErrorKind"
    requirement: "PAR-01"
    verification:
      - kind: unit
        ref: "internal/provider/errors_test.go#TestIsOverflow"
        status: pass
      - kind: command
        ref: "grep kind constants — ErrorKind value set unchanged (Transient/Structural/Exhausted only)"
        status: pass
    human_judgment: false

# Metrics
duration: 3 min
completed: 2026-09-06
status: complete
---

# Phase 19 Plan 02: Overflow Surfacing (Non-2xx Error Chunks + IsOverflow) Summary

**Non-2xx provider rejections now surface as ClassifyHTTP-typed error chunks (RESEARCH Pitfall 1 closed) plus the IsOverflow message-class predicate — the substrate 19-04's retry-once recovery consumes.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-09-06T20:35:38Z
- **Completed:** 2026-09-06T20:38:45Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- `Stream` now checks `resp.StatusCode` immediately after `httpClient.Do`: a non-2xx body is read bounded (8 KiB), closed at the site, classified via `ClassifyHTTP`, and emitted as one error chunk on a closed 1-buffered channel — `drainSSE` never runs for a rejected request, so the Pitfall 1 silent swallow (empty `end_turn`) is gone
- Malformed, truncated, and empty error bodies degrade to the generic structural `ProviderError` carrying the status code — the envelope parser never panics (T-19-03 mitigation, prohibition pinned by test)
- `IsOverflow(err) bool` added to errors.go: a message-class matcher in the `isNetOrContextError` family style — `errors.As` to `*ProviderError`, case-insensitive contains of "prompt is too long" over `Error()`; true only for the overflow class, false for nil / plain errors / network kinds / generic 400 / 429; no new `ErrorKind`
- The full provider suite passes unmodified (happy SSE path observably unchanged); `go vet` clean

## Task Commits

Each task was committed atomically (TDD RED/GREEN pairs):

1. **Task 1: Non-2xx responses surface as error chunks**
   - `8edd75a` (test, RED) — TestStream_Non2xxError + malformed-body sub-case; RED evidence: done chunk "end_turn" + no error chunk
   - `be04be7` (feat, GREEN) — status check + rejectStreamError + bounded envelope parse in streaming.go
2. **Task 2: IsOverflow — the message-class predicate**
   - `169915d` (test, RED) — TestIsOverflow table; RED evidence: `undefined: IsOverflow`
   - `424fe18` (feat, GREEN) — IsOverflow + overflowMessageClass const in errors.go

**Plan metadata:** committed with this SUMMARY (docs).

## TDD Gate Compliance

Both tasks followed RED → GREEN with the gate sequence validated in git log: `test(19-02)` commits precede their `feat(19-02)` counterparts (8edd75a → be04be7, 169915d → 424fe18). No REFACTOR commit needed — the GREEN implementations landed in their final shape. No violations.

## Files Created/Modified

- `internal/provider/streaming.go` — non-2xx status check at the Stream send site; `rejectStreamError` (bounded read + synchronous close + ClassifyHTTP); `anthropicErrorEnvelope` parse with safe degrade; bodyclose nolint comment kept accurate
- `internal/provider/streaming_test.go` — `errorEnvelopeHandler`/`streamAgainstHandler` harness; `TestStream_Non2xxError`; `TestStream_Non2xxError_MalformedBody`
- `internal/provider/errors.go` — `IsOverflow` predicate + `overflowMessageClass` const, documented as A1 community-sourced with 19-04 E2E re-verification routing
- `internal/provider/errors_test.go` — `TestIsOverflow` true/false table (exact A1 wording, case-varied, turn-loop-wrapped; nil, plain, network, generic-400, 429)

## Decisions Made

- The rejected request returns a 1-buffered channel carrying exactly one error chunk then closed (never `nil, err`): the turn loop's existing `chunkErrorType` case is the consumer — no new turn-loop vocabulary
- The envelope's `error.type` + `error.message` ride the `ProviderError` Cause so `IsOverflow` matches over `Error()` without re-parsing bodies at the consumer
- `ClassifyHTTP(providerAnthropic, prof.Model, resp.StatusCode, cause)` uses the model already in scope (`prof.Model`) rather than the `modelGLM52` constant `sendAbortError` hardcodes — the rejection is attributed to the profile's actual model

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- PAR-01's error substrate is real: rejected requests are typed, classified, and observable at the turn loop; `IsOverflow` is ready for 19-04's retry-once branch
- PAR-01 remains unchecked in REQUIREMENTS.md under the shared-ID gate (19-03/19-04/19-05 also declare it and have not finished)
- Live-endpoint wording verification of the A1 message class is routed to 19-04's E2E concern, as planned
- Ready for 19-03

## Self-Check: PASSED

- Created/modified files exist on disk: streaming.go, streaming_test.go, errors.go, errors_test.go (all under internal/provider/) — FOUND
- Commits exist: 8edd75a, be04be7, 169915d, 424fe18 — FOUND in git log
- Plan-level verification re-run: `go test -race ./internal/provider/ -count=1` green including both new tests; ErrorKind value set unchanged (grep: Transient/Structural/Exhausted only); ClassifyHTTP and structuralStatuses untouched (diff shows additions only)

---
*Phase: 19-compaction-cache-control*
*Completed: 2026-09-06*
