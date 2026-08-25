---
phase: 13-openspec-workflow-completion
plan: "03"
subsystem: unmatched-ending-advisory
tags: [advisory, d02-dead-ends, d05-dedupe, replay-fidelity]
requires:
  - "12-01 (the AskUserQuestion surface)"
  - "13-02 (the seeds the advisory must not interfere with)"
provides:
  - "engine.ClassifyQuestionEnding (2 capture-anchored classes) + SignalAdvisory + the unmatched-cell augmentation"
  - "the serve wiring: advisorySeen dedupe + the EngineDecision collector + the post-done direct note emit"
  - "the dead-end advisory scan folded into the onboard matrix leg"
affects:
  - "future matrix legs (the advisory line asserts on question-shaped closings)"
tech-stack:
  added: []
  patterns:
    - "collector-mirroring-drain (the PATTERNS timing hazard designed around)"
    - "wrapper-held dedupe state (engine statelessness preserved)"
key-files:
  created:
    - internal/engine/advisory.go
    - internal/engine/advisory_test.go
    - cmd/ass-guard/advisory_wiring_test.go
  modified:
    - internal/engine/types.go
    - internal/engine/decide.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/e2e_opsx_matrix_test.go
decisions:
  - "The note emits from inside Run — it reaches the client just BEFORE the response frame (the ask-surface wire position); 'after the turn' is the turn, not the response frame"
  - "The lever NOT applied (the corpus shows AskUserQuestion usage); the gated re-proof not owed"
metrics:
  duration: 105min
  tasks: 2
  commits: 3
status: complete
actuals:
  tokens: 172000
  tasks: 2
  commits: 3
---

# Phase 13 Plan 03: The unmatched-ending advisory Summary

**One-liner:** Interactive dead-ends now surface instead of stalling: an unmatched question-shaped ending gets an
audit-always advisory decision (ActionNothing — never holds) plus ONE client-visible note per class per session,
with the chain provably not held and replay fidelity intact.

## Task results

| Task | Commits | Result |
|------|---------|--------|
| 1 (tracer, TDD) | 352d90b RED → c65faa9 GREEN | ClassifyQuestionEnding (the choice class, capture-anchored) + SignalAdvisory + the unmatched-cell augmentation; the serve wiring (advisorySeen on the wrapper, the EngineDecision collector, the post-done direct emit of the fixed single-line note). Server-level pin: the note as its own session/update + the advisory engine_decision line + ZERO new user-message lines. |
| 2 (full battery) | 9639d20 | The open-question class (harvest-informed); multi-class + first-match-wins; seeded non-interference + ask-precedence; the D-05 dedupe (one note/class/session, audit-always); the wording-elements pin (question + AskUserQuestion named; no hold-language); the dead-end scan folded into the onboard leg. |

## The D-02 model-initiated-half evidence record (both conditionals, honestly)

- **(a) Dead-end advisory scan:** the 13-01 captures identify onboard's closing as the question-shaped exemplar
  ("Which task interests you? (Pick a number or describe your own)"). The PRESERVED matrix transcripts predate
  13-03 (the advisory did not exist at 13-01's runs) — no post-13-03 preserved transcript exists yet, so the scan's
  assertion is FOLDED INTO the onboard leg (a classifying closing MUST carry the advisory line on every future
  gated run) + the wiring pins stand as today's evidence. Explicitly recorded, never silently skipped.
- **(b) Lever decision: NOT APPLIED** — the corpus shows the model already invokes AskUserQuestion at interactive
  points (the onboard legs' tutorial asks rode the broker through the D-01 timeouts + 13-00's resume; the
  command's own template mandates the tool). No seeded-row text edit; the captured catalog untouched.
- **(c) Gated re-proof: NOT OWED** (the lever was not applied) — recorded per the plan's conditional.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1] The collector subscribed after the events** — the RED wiring test hung (ReadBytes blocked waiting
for a note that never came): the EngineDecision events publish DURING runOneTurn and the bus DROPS them without
subscribers; the collector now subscribes BEFORE the turn. Found in RED debugging — exactly what RED is for.

**2. [Plan-mechanics refinement] The note's wire position** — the plan sketched the note "after the turn's
response"; emitting from inside Run (the in-hand emitter) puts it just BEFORE the response frame — the SAME wire
position the 12-01 ask surface uses. The pin asserts existence + transcript invariants; the ordering note is
documented in the test.

## Known Stubs

None.

## Threat Flags

None (T-13-03-01/02 mitigated: fixed class constants + fixed note template, wording-elements pinned; the
Action-is-Nothing invariant asserted in the battery).
