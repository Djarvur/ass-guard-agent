---
phase: 20-built-in-commands-skills-per-agent-model
plan: 02
subsystem: commands
tags: [slash-commands, class-b, cost, clear, model-switch, delegation-seams]

requires:
  - phase: 20-built-in-commands-skills-per-agent-model
    provides: plan 01 — chain + class-B intercept + D-05 shape + builtin table
  - phase: 16-acp-wire-foundation
    provides: ApplyTurnModel/SetTurnModel live-apply seam (16-05), 16-D-22 local_command record
  - phase: 18-session-family
    provides: ListSessions listing engine (18-03/18-04) — /resume's real machinery
  - phase: 19-compaction-cache-control
    provides: Session.CompactNow (19-D-11 immediate trigger) — /compact's real machinery
provides:
  - All twelve CMDS-02 class-B commands live at the runner seam (zero model turns, durable local_command records)
  - /help self-describing from the chain; /memory read-only; /permissions mode; /mcp config list; /doctor credential-safe checks; /config resolved view
  - /model session-scope live-apply (declared-slug validation, loud unknown-slug degrade, zero layer writes)
  - /clear same-session FULL context reset (session.BoundaryCauseContextReset + projectFullReset — empty-seed projection that also beats the durable compaction summary)
  - /resume + /compact delegation seams (resumeListHook/compactNowHook) armed with the REAL Phase 18/19 machinery in NewRunner; nil seams degrade loudly with "unavailable:" outcomes
  - /cost per D-07: config-declared provider usage_endpoint capability (default none), bounded live fetch, transcript×Pricing fallback reusing the cost.go Account formula, mandatory source note (P-20-02)
affects: [20-06 E2E battery, roadmap criterion 3]

tech-stack:
  added: []
  patterns:
    - "Handler contract (output, outcomeOverride) — delegation degrades record 'unavailable: ...' durably in the local_command line"
    - "Cause-aware full-reset boundary: the projector recognizes BoundaryCauseContextReset and projects an empty seed (intent alone), overriding both the mechanical summary and the durable compaction seed"
    - "Test-injectable transport + budget seams for the one network-capable handler"

key-files:
  created: []
  modified:
    - internal/runtime/commands.go
    - internal/runtime/commands_test.go
    - internal/runtime/runtime.go
    - internal/runtime/acp_engine_e2e_test.go
    - internal/session/projector.go
    - internal/modelrouting/config.go

key-decisions:
  - "Delegation seams armed in NewRunner with the REAL machinery (Phase 18 listing + 19 CompactNow exist in this build) — the plan's never-leave-a-seam-unregistered rule; tests nil them to pin the loud-degrade posture"
  - "/clear needed a projector rule, not just a boundary: a plain TypeBoundary seed retains a mechanical summary of the cleared span (extractSummary over pre-boundary lines), so D-06's 'starts empty' is delivered by a dedicated context-reset cause the projector projects with an EMPTY seed — and it also drops the compaction marker's durable summary (a cleared context starts empty, period)"
  - "/model does NOT call Runner.ApplyTurnModel (it locks every session's turn mutex — deadlock while ours is held); it rides the per-session leg SetTurnModel directly, matching 16-D-12's session-scope semantics"
  - "/cost live response rendering is deliberately best-effort generic JSON (provider-specific shapes unverifiable without operator credentials — A1); the SOURCE NOTE is the contract (P-20-02), never the shape"
  - "/compact passes no focus args to CompactNow (its signature admits none — 19-RESEARCH OQ3): args recorded verbatim in the local_command line + fixed-form note in output"

patterns-established:
  - "countingRoundTripper with ctx-honoring fakes (a sleeping custom RoundTripper that ignores ctx bypasses Go's post-roundtrip cancellation check — the fake must select on req.Context().Done())"
  - "dirSnapshot workDir freeze assertion (excluding .ass-guard transcript) for read-only/no-layer-write contracts"

requirements-completed: [CMDS-02]

duration: 118 min
completed: 2026-09-07T20:35:00Z
---

# Phase 20 Plan 02: Class-B Command Family Summary

**All twelve control-plane slash commands now execute at the runner seam with zero model turns, durable local_command records, and the three hard behaviors live: same-session /clear full reset, source-noted /cost, session-scope /model — with /resume and /compact routing through seams armed with the real Phase 18/19 machinery.**

## Performance

- **Duration:** 118 min
- **Tasks:** 3/3 (TDD: RED batteries first per task)
- **Commits:** bacd0e4 (pure-local family), b9734d3 (mutating + delegating), e8c83a9 (/cost), 8e15ca6 (D-04 assertion update)
- **Files:** 6 modified

## Accomplishments

- Task 1: /help (chain-generated, fixture-proven self-describing), /memory (read-only, workDir-freeze-asserted), /permissions, /mcp, /doctor (credential env NAMES only — the fixture secret asserted absent), /config — all zero-network, battery-pinned D-05 shape.
- Task 2: /model (next-request model asserted via captured provider requests; unknown slug leaves state untouched; layer files byte-identical), /clear (boundary + empty-seed projection proven at the projector level — the pre-clear codeword never reaches the post-clear request; session id + transcript survive), /resume + /compact delegation (nil-seam loud degrade with "unavailable:" outcome; registered fake invoked exactly once with typed args verbatim).
- Task 3: /cost three-case matrix (capability none → zero network attempts; declared+working → note names the endpoint; timeout → fallback under the bounded budget), fallback math hand-computed ($4.00 from seeded usage lines × fixture pricing), credential never rendered.

## Self-Check: PASSED

- go test -race -count=1 ./internal/runtime/ green (TestCommandChainReservedShadowing updated once: model/compact became live builtin winners when this plan wired their handlers — the D-01 pin is now winner-kind==builtin, which is the honest assertion).
- Known pre-existing baseline flake (TestAskPark_PromptResponsePrecedesResolution under full-suite parallel load) did not recur in this plan's verification runs; documented in 20-01-SUMMARY.

## Deviations from Plan

- **[Rule 2 - missing detail] /clear required a projector rule, not just AppendBoundary**: the plan said "append the same context-reset boundary class the Projector already treats as a hard reset" — no such class existed (a plain boundary retains a mechanical summary; D-06 demands empty). Added session.BoundaryCauseContextReset + fullResetBoundaryIdx + projectFullReset (empty seed, also overrides the compaction marker's durable summary). 9 lines of projector logic, projector_test coverage via the runtime battery; internal/session untouched by ACP vocabulary (15-D-20 holds).
- **[Rule 2 - deadlock hazard] /model calls the per-session leg directly** (Session.SetTurnModel) instead of Runner.ApplyTurnModel — ApplyTurnModel takes every session's turn mutex, and the intercept already holds ours. Same 16-D-12 semantics (this session only, nothing persisted).
- **[Rule 3 - environment] delegation seams registered at construction with the real machinery** (Phase 18 listing + Phase 19 CompactNow both exist in this build per the plan's own conditional), not left nil: NewRunner arms realResumeListing(workDir) + realCompactNow; the degrade posture stays pinned by nil-seam tests.

## Issues Encountered

None new (the lint-baseline and AskPark flake observations carry over from 20-01-SUMMARY).

## Next Phase Readiness

Twelve-command surface complete for 20-06's E2E battery; /help's chain generation and the delegation outcomes are stable contracts.
