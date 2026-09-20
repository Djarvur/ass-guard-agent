---
phase: 23-seed-gaps-close-out
plan: 02
subsystem: runtime
tags: [steering, ingress, classifier, parked-ask, concurrency]

requires:
  - phase: 23-01
    provides: SteerQueue, SetSteerQueue/SteerQueue, boundary drain, steering_delivery kind
  - phase: 20 (class-B machinery)
    provides: tryLocalCommand + the command chain resolve (the class-B slot's locked position)
provides:
  - routeSteering: the pre-mutex ingress classifier in Runner.Run (Pattern 6's locked order)
  - resolvesAsCommand: side-effect-free class-B predicate (chain-resolved invocations never steer)
  - SteerQueue wiring in sessionFor (per session)
  - parked_ask transcript kind + Manager.AppendParkedAsk + D-05 note at suspendForAsk
  - parked-cancel grammar (exact-phrase table) + Session.CancelPendingAsk (cancelled-normal via the D-01 settle path)
affects: [23-05, TG-02 (v1.3)]

actuals:
  tokens: 15200
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "pre-mutex classification over the race-tested primitives only (clientTurnActive + chainCount — no new flags)"
    - "exact-phrase cancel grammar table parsed before ordinary reply interpretation"

key-files:
  created:
    - internal/runtime/steering_ingress_test.go
  modified:
    - internal/runtime/runtime.go
    - internal/session/transcript.go
    - internal/session/manager.go
    - internal/session/session.go
    - internal/session/ask.go
    - internal/session/steering_test.go

key-decisions:
  - "Open Question 1 resolved per research recommendation: a steered prompt returns promptly (queued note + end_turn) — return-after-enqueue; the running turn's chunks continue via the session-lifetime forwarder (WINDOWS #3 split untouched: the steered Run returns before markClientTurn)"
  - "Queued-note wording: 'steering queued — N pending' (the D-12 ask-queue note family; CONTEXT discretion, documented at routeSteering)"
  - "The pending-ask route stays UNDER the mutex (ResolveAsk must hold turnMu — CR-02's non-reentrant serialization); this never blocks in practice because a pending broker ask implies the suspending turn already released the mutex — the classifier only needs the ask-first FALL-THROUGH decision pre-lock"
  - "Class-B slot: resolvesAsCommand (parse + chain resolve, zero side effects — no echo, no handler, no resolve-counter bump); only RESOLVED names fall through — an unknown /word is prose and steers"
  - "Parked-cancel grammar is exact-phrase after trim+casefold (never substring): cancel/cancel the ask/question, dismiss/dismiss the ask, never mind/mindmind variants — one table, parkedCancelPhrases"
  - "CancelPendingAsk resolves cancelled-normal through resumeAskClaimed(ctx, p, nil) — the D-01 timer's own non-answer path (17-D-13 drain semantics proactively); settle/exactly-once discipline untouched"
  - "Image-only inputs mid-turn fall through to the ordinary path (steering is text-only in v1) — documented at the classifier"

patterns-established:
  - "Pre-mutex classifier slot order: nothing-active -> ask route -> class-B slot -> steering -> ordinary (Pitfall 11's matrix, one code site)"
  - "Parked visibility pair: live note on the chunk family + parked_ask audit marker, both fire-and-forget at suspendForAsk"

requirements-completed: [SEEDG-01]

coverage:
  - id: D1
    description: "Mid-turn inputs classify pre-mutex: steered prompts return promptly with a queued note while the turn's provider call is still blocked (anti-queue-behind)"
    requirement: SEEDG-01
    verification:
      - kind: unit
        ref: "internal/runtime/steering_ingress_test.go#TestSteerIngressMidTurnPreMutex"
        status: pass
    human_judgment: false
  - id: D2
    description: "Classifier order: ordinary path byte-identical, ask outranks steering, class-B falls through, chain branch enqueues"
    requirement: SEEDG-01
    verification:
      - kind: unit
        ref: "internal/runtime/steering_ingress_test.go#TestSteerIngressNoActiveTurnOrdinary"
        status: pass
      - kind: unit
        ref: "internal/runtime/steering_ingress_test.go#TestSteerIngressPendingAskOutranks"
        status: pass
      - kind: unit
        ref: "internal/runtime/steering_ingress_test.go#TestSteerIngressClassBNotSteered"
        status: pass
      - kind: unit
        ref: "internal/runtime/steering_ingress_test.go#TestSteerIngressEngineChain"
        status: pass
    human_judgment: false
  - id: D3
    description: "Parked asks visible (D-05 note + parked_ask record) and cancelable by grammar (D-06) without killing anything; answers unchanged; replay tolerance"
    requirement: SEEDG-01
    verification:
      - kind: unit
        ref: "internal/session/steering_test.go#TestParkedAskNoteAndRecord"
        status: pass
      - kind: unit
        ref: "internal/runtime/steering_ingress_test.go#TestParkedAskCancelGrammar"
        status: pass
      - kind: unit
        ref: "internal/session/steering_test.go#TestParkedAskReplayTolerance"
        status: pass
    human_judgment: false
  - id: D4
    description: "Combined scenario (steering x2 coalesced + parked ask + cancel) and engine-chain steering with zero enginebridge changes"
    requirement: SEEDG-01
    verification:
      - kind: integration
        ref: "internal/runtime/steering_ingress_test.go#TestCombinedSteerScenario"
        status: pass
      - kind: integration
        ref: "internal/runtime/steering_ingress_test.go#TestEngineChainSteered"
        status: pass
    human_judgment: false
  - id: D5
    description: "Live-Zed confirmation of steering ingress (Zed may queue client-side — RESEARCH A2 flagged)"
    requirement: SEEDG-01
    verification: []
    human_judgment: true
    rationale: "RESEARCH A2 flagged that Zed may never deliver a mid-turn session/prompt; live-editor behavior is unassertable by tests — carried as a 23-05 UAT checkpoint"

duration: 55 min
completed: 2026-09-08
status: complete
---

# Phase 23 Plan 02: Steering Ingress Summary

**Pre-mutex input classification in Runner.Run kills queue-behind for steerable input: mid-turn texts enqueue on the SteerQueue and return promptly, parked asks are visible and cancelable by grammar, and engine chains steer with zero engine changes.**

## Performance

- **Duration:** 55 min
- **Tasks:** 3/3
- **Files:** 7 (1 created, 6 modified)

## Accomplishments

- **Task 1 (classifier + wiring + response semantics):** `routeSteering` at the Run head BEFORE turnMu.Lock, over clientTurnActive + chainCount only; locked order nothing-active → ask route → class-B slot (documented 20-01 position, 23-05 fills /undo) → steering enqueue; steered prompts return end_turn immediately with the "steering queued — N pending" note through the in-hand emit; sessionFor wires `SetSteerQueue` per session; `Session.SteerQueue()` accessor exported (TG-02's surface).
- **Task 2 (parked asks):** `TypeParkedAsk` kind + `AppendParkedAsk` (REDACTED); suspendForAsk emits the D-05 note + record; `isParkedCancel` exact-phrase grammar parsed before ResolveAsk in routeAskReply; `Session.CancelPendingAsk` resolves cancelled-normal via the D-01 non-answer settle path.
- **Task 3 (E2E):** TestCombinedSteerScenario (two steers coalesce into one count-2 delivery, scripted AskUserQuestion parks, cancel resolves and resumes the suspended turn, no cancelled content in any request) + TestEngineChainSteered (chainCount>0 mid-turn, steering delivered at the chain turn's boundary, chain completes and unwinds — zero enginebridge changes).

## Task Log

| Task | Commit | Verification |
|------|--------|--------------|
| T1 ingress classifier + wiring | RED (in battery commit) + 40b2bbc | TestSteerIngress* battery green under -race |
| T2 parked asks + cancel grammar | 40b2bbc | TestParkedAsk* green (session + runtime) |
| T3 combined + engine-chain E2E | d220988 | TestCombinedSteerScenario + TestEngineChainSteered green |

Whole-package verification: `go test -race -count=1 ./internal/session/ ./internal/runtime/` green (session 3.4s, runtime 194.8s).

## Deviations from Plan

- **[Note - sequencing] The combined scenario's four legs run as steer, steer, park, cancel** — the plan's literal order (steer, park, cancel, steer) is not constructible through the public API: a turn's own ask suspension ENDS the turn (stopAsk), so a second steering after the park would find nothing active. The executed order exercises every input class in one scenario with coalescing — the milestone's actual prescription ("tests never combining the classes" is the warning sign).
- **[Note - anchor drift] Plan anchors reference pre-Phase-22 line numbers** (Run at 479, routeAskReply at 1611, AskBroker wiring at 1120); all were re-located on the current tree (804/2933/1749).

**Total deviations:** 2 documented notes. **Impact:** none.

## Issues Encountered

None.

- Transient co-existence with the Phase 22 executor: one ~10-minute window where internal/coreexec was mid-edit (undefined bgConcurrentCap) blocked runtime builds; it self-resolved (their 22-01/22-02 work committed); no workaround applied, no files swept.
- go vet clean on both packages; golangci-lint remains environmentally broken (STATE.md LINT BASELINE).

## Self-Check: PASSED
