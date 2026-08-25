---
phase: 12-product-functional-completeness
plan: "09"
subsystem: acp
tags: [acp, planmode, wiring, gap-closure, engine-provenance, server-level-testing]

requires:
  - phase: 12
    provides: "the 12-04 interactive family + the UAT gap G-12-3 diagnosis"
provides:
  - "Plan mode works end-to-end in the serve path: EnterPlanMode flips state (marker lands), the mutating-tool gate refuses while ON, ExitPlanMode suspends on the approval question, an approving reply ungates + resumes"
  - "TurnOutput.PlanMode provenance is actually reachable — the LastTurnOutput scan honors markers inside the terminal turn and treats a resolved suspension as non-terminal"
affects: [13-openspec-workflow-completion]

actuals:
  tokens: 0        # inline execution by the orchestrator (the spawned executor stalled twice on context-loading; work taken over per gap-closure discipline)
  tasks: 2
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Server-level wiring battery for state-wiring gaps: runner-level tests cannot see a sessionFor omission; drive the REAL acp.Server over stdio pipes"

key-files:
  created:
    - cmd/ass-guard/planmode_wiring_test.go
  modified:
    - cmd/ass-guard/acp_serve.go

key-decisions:
  - "The fix is one line (s.SetPlanMode(planMode) beside s.SetAskBroker) — but the scan defect it exposed is not: a backward terminal-line scan must keep walking to its turn's user_message boundary or mid-turn markers are invisible"
  - "A resolved suspension (its tool_result landed below) is NOT terminal — treating it as terminal made LastTurnOutput report AskSuspended forever after a reply, hanging WaitChainIdle (TestAskPark_ReplyDuringParkResumesAndQueuesInjection caught it)"

patterns-established:
  - "RED-first server-level scenario batteries as the standard for any 'wired at sessionFor' claim"

requirements-completed: [ACP-02]

duration: 95min (including executor-stall diagnosis + two probe detours)
completed: 2026-08-23
status: complete
---

# Phase 12 Plan 09: Plan-mode serve-path wiring Summary

**Plan mode now works end-to-end through the real serve path: the one-line SetPlanMode wiring closes G-12-3's root cause, and the terminal-line scan rewrite makes PlanMode provenance reachable AND un-hangs the parked-chain idle wait after an operator reply (a second live bug the new tests caught).**

## Performance

- **Duration:** ~95 min inline (spawned gsd-executor stalled twice during context loading with zero commits; orchestrator took over per the no-silent-workaround rule)
- **Tasks:** 2 (RED battery → GREEN wiring + scan fix)
- **Files:** 2 (1 created test battery, 1 modified)

## Accomplishments

- `sessionFor` wires the per-session plan-mode state onto the Session (`s.SetPlanMode(planMode)` beside `SetAskBroker`) — the Enter flip at the tool-result site now fires, the mutating-tool gate refuses while ON, ExitPlanMode surfaces the approval question and suspends.
- `LastTurnOutput`'s backward scan fixed twice over: (1) it walks to the terminal turn's own `user_message` boundary so mid-turn `plan_mode` markers are seen (`planModeOn` was previously unreachable for assistant-terminated turns); (2) an ask_suspended whose tool_result already landed below is treated as RESOLVED — previously a completed resume reported `AskSuspended` forever, which hung `WaitChainIdle` after an operator reply (caught by the existing park pins).
- Server-level RED→GREEN battery (`TestPlanModeWiring_*`) drives the REAL acp.Server over stdio pipes: enter → marker assertion; Edit refused while ON; approval question on the wire; approving reply resumes the same turn.

## Task Commits

1. **Task 1 RED** — `00ccc2f` (test): the plan-mode wiring battery, both tests failing exactly as the gap predicts
2. **Task 2 GREEN** — `e704789` (fix): SetPlanMode wiring + the scan rewrite; full `mise ci` green

## Files Created/Modified

- `cmd/ass-guard/acp_serve.go` — SetPlanMode call in sessionFor; LastTurnOutput scan rewrite (terminal-turn boundary walk + resolvedBelow map)
- `cmd/ass-guard/planmode_wiring_test.go` — scripted provider + server-level harness + two scenario tests

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The provenance leg surfaced a second live defect**
- **Found during:** GREEN run — the pre-existing park/timer pins failed
- **Issue:** the stalled executor's prediction confirmed empirically: the scan broke before reading markers; fixing that alone made resolved suspensions look permanently suspended (9-call re-fire loops in ZeroContinue-class tests)
- **Fix:** boundary-walk + resolved-below semantics in one pass; probe-verified against both failure shapes before committing
- **Verification:** TestAskPark_ReplyDuringParkResumesAndQueuesInjection, TestAskWiring_ChainSurvivesAskTimerResume, TestEndToEnd_ZeroContinue all green; mise ci green
- **Committed in:** e704789

---

**Total deviations:** 1 auto-fixed. No scope creep.

## Verification Evidence

- RED: `go test ./cmd/ass-guard/ -run TestPlanModeWiring` — both fail ("no plan_mode_enter marker"; "approval question never reached the client wire")
- GREEN: same command ok; full package battery ok; `mise ci` green (vet + lint + build + race tests)

## Next Phase Readiness

- G-12-3 closed at the code level; UAT re-test pending (operator or stdio-driven).
- The scan fix touches engine provenance shared with Phase-13 chains — their suites stayed green.
