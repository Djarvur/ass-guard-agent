---
phase: 23-seed-gaps-close-out
plan: 05
subsystem: runtime
tags: [undo, class-b-commands, checkpoint, auto-cancel, steering, seedg, tdd]
# Dependency graph
requires:
  - phase: 20-built-in-commands-skills-per-agent-model
    provides: the class-B intercept + RESERVED name set + D-05 output shape + TestClassB battery
  - phase: 23-02
    provides: the pre-mutex ingress classifier's class-B resolve slot (the locked position this plan fills)
  - phase: 23-03
    provides: SnapshotPreRestore (the -pre- id family), RestoreGuard (nested-repo refusal)
  - phase: 23-04
    provides: the Runner-owned checkpoint store + restoreBlockers guard the undo composes
provides:
  - /undo as the fourteenth RESERVED class-B command — the D-11 stack walk over the checkpoint store with the D-05 output shape, verbatim local_command records, and loud degradation (SEEDG-03)
  - The D-12 auto-cancel-then-restore path in the locked ordering (snapshot -> cancel -> drain under the mutex -> restore), classified pre-mutex so it never steers and never self-deadlocks
  - The Runner active-turn cancel registry — the in-process half of the existing cancel contract (the ACP half untouched)
  - Teardown-side steering-queue resolution at parked-chain cancel and turn exit (the panic-recover residual closed; nothing zombie-delivers into a later turn's window)
affects: [phase-24 (tails — ECOS-04 mode matrix), TG-02 (Telegram steering consumer — the UAT's Zed-client-behavior note feeds its planning)]
actuals:
  tokens: 15000    # chars/4 over the realized diff (59,126 diff chars across 3 files, 4 commits)
  tasks: 3
  commits: 4       # MEASURED: git rev-list --count 12b5e39..HEAD
plan_head_before: 12b5e39183051730aadb0689b4b41f068c2b5dd0
tech-stack:
  added: []
  patterns:
    - "recency-ordered stack walk over both checkpoint id families (CommittedAt desc with pre-family/TurnNum tie-breaks reproducing mint order) — the grammar-order List() is deliberately NOT the walk order"
    - "registry-cancel-gated-on-chain-liveness: a derived turn ctx may only be cancelled at Run exit when no engine chain still counts — the 13-00 park must survive its suspending Run's return"
    - "pre-mutex class-B completion: the one class-B command that finishes mid-turn does classification, snapshot, and cancel ALL before the turn-mutex acquisition (Pitfall 4's -race-invisible deadlock class, pinned behaviorally)"
key-files:
  created: []
  modified:
    - internal/runtime/commands.go
    - internal/runtime/runtime.go
    - internal/runtime/commands_test.go
key-decisions:
  - "The D-11 walk orders entries by RECENCY (CommittedAt desc; same-second ties break pre-family-above-turn then TurnNum desc) — not List()'s grammar order — because the minted pre-restore snapshot must be the NEXT /undo's target for the walk to be reversible (undo-of-undo); the grammar order would strand every -pre- entry below the turn stack"
  - "The acceptance line '/undo 2 from fresh lands the same state' cannot coexist with the behavior bullet 'the second /undo restores the state before the first /undo' under ANY walk semantics (the two impose different targets for the second invocation given the same entry set); the behavior block + must_haves reversibility contract wins, and the battery pins /undo N as an N-entry jump on a fresh fixture (second-newest for N=2)"
  - "The active-turn registry's ctx release is GATED on chainCount==0 at Run exit: an unconditional cancel killed the 13-00 park (three ask-park regressions caught by the full suite BEFORE commit) because the engine path's request-ctx watchdog folds turnCtx death into the parked cancel"
  - "/undo store ops ride the serve ctx (the parked-chain precedent) — a restore must not die with the requesting request halfway through its git plumbing"
  - "The turn-exit and parked-chain-cancel queue resolutions are unconditional (the plan's rider letter): undelivered steering resolves cancelled-normal at every turn exit class, closing the panic-recover residual; idempotent with recordCanceled's mid-turn funnel"
patterns-established:
  - "Same-package func-var seams for store-adjacent injection (undoSnapshotPreRestore — the gitRun precedent), with tests that swap them kept non-parallel (Go runs sequential tests outside the parallel batch, so the swap never races)"
  - "Class-B batteries drive Run end-to-end over a seeded checkpoint store (seedUndoSnap + undoRun + ckptLiveTree byte-identity) rather than unit-calling handlers"
requirements-completed: [SEEDG-03]
coverage:
  - id: D1
    description: "/undo idle path: 14th RESERVED class-B name, D-11 walk over both checkpoint families, D-05 shape with zero provider calls, verbatim local_command records, edge table, cross-session scoping, loud nil-store degradation"
    requirement: SEEDG-03
    verification:
      - kind: unit
        ref: "internal/runtime/commands_test.go#TestClassBUndoIdle (incl. undo-of-undo)"
        status: pass
      - kind: unit
        ref: "internal/runtime/commands_test.go#TestUndoWalk (treeMap byte-identity, /undo 2 N-jump)"
        status: pass
      - kind: unit
        ref: "internal/runtime/commands_test.go#TestClassBUndoEdges (0/negative/non-numeric/beyond-depth/empty-store)"
        status: pass
      - kind: unit
        ref: "internal/runtime/commands_test.go#TestClassBUndoCrossSession + TestClassBUndoDegradedStore"
        status: pass
    human_judgment: false
  - id: D2
    description: "D-12 auto-cancel-then-restore: /undo mid-turn completes without the provider releasing (Pitfall-4 deadlock guard), the turn ends cancelled, snapshot-first fail-closed semantics, nested-repo refusal outranking the auto path, no /undo text in any request"
    requirement: SEEDG-03
    verification:
      - kind: integration
        ref: "internal/runtime/commands_test.go#TestUndoAutoCancel (5s deadlock guards, queued-steering resolution, fresh-turn cleanliness)"
        status: pass
      - kind: integration
        ref: "internal/runtime/commands_test.go#TestUndoAutoCancelParkedChain (parked chain, no client turn)"
        status: pass
      - kind: integration
        ref: "internal/runtime/commands_test.go#TestUndoFailClosed (injected SnapshotPreRestore failure; turn untouched)"
        status: pass
      - kind: integration
        ref: "internal/runtime/commands_test.go#TestUndoNestedRefusal (paths named; never cancelled)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Teardown-side steering resolution: cancelParkedChains and the turn-exit unregister resolve the session's SteerQueue (cancelled-normal), closing the parked-teardown and panic-recover exit classes"
    verification:
      - kind: unit
        ref: "internal/runtime/commands_test.go#TestParkedChainCancelSteering (fresh turn's request clean, queue empty, no steering_delivery line)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Live-Zed operator UAT of the phase's two interactive surfaces: boundary steering mid-turn (23-01/23-02) and /undo incl. the auto-cancel-then-restore path against a running turn"
    verification: []
    human_judgment: true
    rationale: "Requires a human editor session in Zed against a scratch git repo (steering visible mid-turn, cancelled-stop rendering, restore summary, ACP log inspection); the offline batteries prove the transport-neutral core, and the RESEARCH A2 open question (whether Zed queues mid-turn prompts client-side) can only be answered by observing the client — the outcome feeds TG-02 planning"
# Metrics
duration: 34 min
completed: 2026-09-10
status: complete
---

# Phase 23 Plan 05: /undo Class-B Command + Live-Zed UAT Record Summary

**/undo as the fourteenth RESERVED class-B command with the D-11 recency-ordered checkpoint walk, the D-12 auto-cancel-then-restore sequence in locked pre-mutex ordering, and teardown-side steering resolution — SEEDG-03 closed (live-Zed UAT recorded PENDING-OPERATOR-CONFIRMATION).**

## Performance

- **Duration:** 34 min
- **Started:** 2026-09-10T11:12:06Z
- **Completed:** 2026-09-10T11:45:50Z
- **Tasks:** 3 (2 TDD auto + 1 human-verify checkpoint)
- **Files modified:** 3

## Accomplishments

- /undo is live as the fourteenth RESERVED class-B name: idle sessions restore the newest checkpoint of THEIR session with zero provider calls, the D-05 echo/output shape naming the restored id and the pre-restore snapshot id, and a durable local_command record with args verbatim; repeated /undo walks the stack reversibly (undo-of-undo via the minted D-09 snapshot) and /undo N jumps N entries.
- The D-12 active path composes the existing cancel contract: classification (snapshot, cancel) ALL before the turn-mutex acquisition — pinned by behavioral timeout guards for the -race-invisible deadlock class — with snapshot-first fail-closed semantics and the nested-repo refusal outranking the auto path.
- The Runner gained the active-turn cancel registry (the parkedCancels structural analog; the ACP half st.cancelTurn untouched), and both teardown sites (parked-chain cancel, turn exit) now resolve undelivered steering cancelled-normal — the panic-recover residual closed.
- The live-Zed operator UAT (steering + /undo, Task 3) is recorded as WINDOWS #23 PENDING-OPERATOR-CONFIRMATION per the 15-07/16-06 precedent — never automated.

## Task Commits

Each task was committed atomically (TDD: RED then GREEN per task):

1. **Task 1 RED: /undo battery** - `3c64e82` (test)
2. **Task 1 GREEN: 14th RESERVED name + D-11 walk + idle path** - `36de582` (feat)
3. **Task 2 RED: D-12 battery** - `a6940e0` (test)
4. **Task 2 GREEN: cancel registry + locked ordering + riders** - `0a57399` (feat)
5. **Task 3: live-Zed UAT** - recorded in this SUMMARY + WINDOWS #23 (no code)

**TDD gate compliance:** both tdd="true" tasks verified via `check tdd-red-evidence` (verdict RED_EVIDENCE_OK, targets TestClassBUndoIdle / TestUndoAutoCancel) before their GREEN commits; no feat precedes its test commit.

## Files Created/Modified

- `internal/runtime/commands.go` - undo in reservedNames + builtinTable; parseUndoDepth; undoWalkTarget (recency walk); prepareUndo/restoreUndo; the SnapshotPreRestore seam
- `internal/runtime/runtime.go` - active-turn cancel registry + turnCtx wiring in Run; routeUndoActive filling the 23-02 class-B slot; restoreUndoActive (locked D-12 sequence); cancelParkedChains + turn-exit steering-resolution riders
- `internal/runtime/commands_test.go` - the full /undo battery (10 tests: idle, walk, edges x5, cross-session, degraded, auto-cancel, parked-chain, fail-closed, nested-refusal, chain-cancel steering)

## Decisions Made

- **Walk order = recency, not grammar order.** List() sorts by (TurnNum, Kind) — the grammar order — under which every minted -pre- entry (counter restarts at 1) sorts at the BOTTOM of the stack and the walk could never progress or reverse. The walk therefore re-sorts by CommittedAt desc with same-second tie-breaks (pre-family above turn, then TurnNum desc) that reproduce mint order; undo-of-undo and the N-jump are pinned by the battery.
- **The acceptance-line reconciliation (documented).** "three checkpoints + two /undo calls land two steps back; /undo 2 from fresh lands the same state" cannot hold together with the behavior bullet "the second /undo restores the state before the first /undo" under any implementable walk (they impose different second-invocation targets over the same entry set — the equivalence would require the walk to skip the very pre-restore entry the reversibility contract depends on). The behavior block + must_haves win; the battery pins two /undo calls as two stack steps (t3 then the minted pre) and /undo N as an N-entry jump.
- **Chain-liveness gate on the registry's ctx release.** The first GREEN draft cancelled turnCtx unconditionally at Run exit; the full suite's three ask-park regressions (TestAskPark_*, TestAskWiring_ChainSurvives*) showed the 13-00 park dying — the engine path's request-ctx watchdog folds turnCtx death into parkedCancel, and a chain parked on an ask OUTLIVES its suspending Run's return. The cancel now fires only when chainCount==0; the parent request ctx bounds the suspended case (the ACP half's own accommodation).
- **Store ops on the serve ctx** (the parked-chain precedent): a restore must not die with the requesting request mid-plumbing.
- **Queue resolution unconditional at teardown** (the plan's rider letter): every turn-exit class resolves undelivered steering cancelled-normal — idempotent with recordCanceled's mid-turn funnel; no existing steering test regressed (the full suite is green).

## Deviations from Plan

None — the plan executed as written. One Rule-1-class defect introduced and fixed WITHIN Task 2's GREEN (before its commit):

### Auto-fixed Issues

**1. [Rule 1 - Bug] Registry cancel killed the 13-00 park**
- **Found during:** Task 2 GREEN (full-suite regression sweep)
- **Issue:** unregisterActiveTurn's unconditional turnCancel() cancelled the derived turn ctx at Run exit; the engine path's request-ctx watchdog folds that death into the parked-chain cancel, so every chain parked on an ask died the moment its suspending Run responded (three ask-park tests failed: the queued apply injection never ran).
- **Fix:** the ctx release is gated on chainCount(sessionID)==0 — a live (parked or running) chain means the park must survive; the parent request ctx bounds the ctx instead.
- **Files modified:** internal/runtime/runtime.go
- **Verification:** the three regressions green again; full runtime/session/checkpoint/acpserve suites green under -race.
- **Committed in:** 0a57399 (part of the Task 2 GREEN commit — the defect never landed on the branch)

---

**Total deviations:** 1 auto-fixed (1 bug, caught and fixed within the introducing task's commit)
**Impact on plan:** No scope creep; the fix preserves the 13-00 park contract the plan builds on.

## Issues Encountered

- TestUndoAutoCancel's first draft expected the walk to land on a pre-seeded checkpoint, but the running turn's OWN entry snapshot (minted by Prompt at turn start) is the newest checkpoint — which is the correct product semantics (/undo of a running turn returns the workspace to the turn's start state). The test was remodeled to the product story: mutate the canary mid-flight, assert the restore lands the pre-turn state. No production change was needed.
- TestUndoNestedRefusal's nested-repo fixture needed one commit inside the nested repo (a bare `git init` leaves no HEAD for the shadow store's `git add -A` to record a gitlink against) — fixture fix in the RED commit.

## Verification

- Plan gate: `go test -race -count=1 ./internal/session/ ./internal/runtime/ ./internal/checkpoint/ ./internal/acpserve/` — green with the two DOCUMENTED pre-existing skips (`TestRescanConcurrency` — deferred-items.md race; `TestPermissionsE2E` — Phase 23 commit 40b2bbc cross-workstream regression, STATE.md). Both skips verified green-or-documented in isolation.
- `go vet` + `go build ./...` green.
- `mise ci` could not run in this environment: the mise shim and a golangci-lint binary are absent (only a source checkout exists), and `mise run lint`'s full-repo baseline is the documented pre-existing broken state (~1.6k findings, STATE.md LINT BASELINE blocker — out of scope per the repo convention). Vet covers the static-analysis leg that is runnable.
- TDD gates: `check tdd-red-evidence` RED_EVIDENCE_OK for both tdd="true" tasks (records: /tmp/undo-red-t1-record.json, /tmp/undo-red-t2-record.json; raw logs /tmp/undo-red-t{1,2}.log).
- Threat register: T-23-15 (never steers — battery asserts /undo in no request), T-23-16 (deadlock — behavioral timeout guards), T-23-17 (unsnapshotted concurrent work — snapshot-first + fail-closed + treeMap byte-identity), T-23-18 (durable record — every outcome class writes local_command with args verbatim) all mitigated and pinned.

## Authentication Gates

None — no credential-gated surfaces were touched.

## Known Stubs

None — every path is wired; no placeholder output, no unwired data sources.

## Threat Flags

None — no security-relevant surface beyond the plan's own threat model.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Phase 23 is COMPLETE (5/5 plans). Ready for `/gsd:verify-work 23` — the verifier should route D4 (the live-Zed UAT, WINDOWS #23) to the operator, whose Zed-client observations (RESEARCH A2) feed TG-02 planning. No blockers from this plan; the two documented pre-existing failures (TestRescanConcurrency, TestPermissionsE2E) remain the phase's known deferred items.

## Self-Check: PASSED

All key-files exist on disk; all four production commits (3c64e82, 36de582, a6940e0, 0a57399) present in git log; measured commit count (4) matches the frontmatter.

---
*Phase: 23-seed-gaps-close-out*
*Completed: 2026-09-10*
