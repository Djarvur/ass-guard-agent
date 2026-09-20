---
phase: 21-context-policy-parity-closures
plan: 06
subsystem: permissions
tags: [hooks, permissions, gate, pretooluse, policy, go]

requires:
  - phase: 21-01
    provides: the ecosys hook substrate — settings scopes, PreToolUseVerdict seam, four-valued Verdict, deny-wins ResolveVerdict
  - phase: 17-permissions-elicitation
    provides: the ONE gate pipeline — gateCall chokepoint, gateVerdict enum, perm RuleSet.Evaluate, the ask queue
  - phase: 21-04
    provides: the readRuleEvaluator @-mention consult seam (nil = implicit allow pre-join)
provides:
  - The joined gate head — settings.json PreToolUse verdicts consume at gateCall's head (deny → gateDeny with first reason; ask → gateSuspend even ungated; user-allow → gateExecute; none → rules)
  - The disposed executor PreToolUse leg — one consultation site, one result form per decision (grep-audited)
  - The perm-backed Read-rule consult for @-mentions (PAR-06 ↔ PAR-03 one rule authority)
  - Exported ecosys Verdict/HookScope constants (the consumed verdict contract)
affects: [phase-25-kit-extraction, permissions-policy, hooks-observability]

actuals:
  tokens: 21773   # chars/4 over the realized internal/+cmd/ diff (87093 chars)
  tasks: 2
  commits: 5

tech-stack:
  added: []
  patterns:
    - "Total verdict mapping at a pipeline head: every enum value has exactly one gate action; NO-DECISION is the only fall-through (T-21-20)"
    - "Disposal-not-stubbing for dead policy seams: the executor consultation deleted, the single-consumption invariant pinned by grep audit + once-only side-effect test"

key-files:
  created:
    - internal/runtime/hooks_join_wiring_test.go
  modified:
    - internal/session/gate.go
    - internal/session/session.go
    - internal/session/gate_test.go
    - internal/coreexec/register.go
    - internal/coreexec/bash.go
    - internal/coreexec/hooks_wiring_test.go
    - internal/ecosys/hooks.go
    - internal/ecosys/hookverdict.go
    - internal/ecosys/loader.go
    - internal/ecosys/hooks_test.go
    - internal/ecosys/hookverdict_test.go
    - internal/runtime/runtime.go

key-decisions:
  - "The head maps the four-valued verdict TOTALY and consults nothing else: ask → gateSuspend unconditionally (D-04's letter — even ungated, riding 17-02/17-03's existing queued ask; no new ask path)"
  - "gateCall regained its ctx parameter (the 17-02 locked-contract signature): hooks execute subprocesses; runTurn passes its ctx at BOTH consumption sites"
  - "ecosys Verdict + HookScope constants exported by necessity (rename-only) — STATE's 21-01 decision anticipated 21-06 exporting the verdict surface the gate consumes"
  - "readRuleEvaluator wired ONCE per Runner inside sessionFor (all sessions share the workDir; the once-guard keeps concurrent construction race-free against turn-time reads; rule-less sessions keep the nil implicit-allow fallback)"
  - "The executor leg was DELETED, not stubbed: coreexec.ToolHooks is PostToolUse-only, HookRunner.PreToolUse and errHookRefused are gone — Phase 25 re-homes the shape if the kit needs it"

patterns-established:
  - "Head-of-pipeline delegation: one injected verdict function, one total mapping, NO-DECISION as the sole fall-through"
  - "Single-consumption grep audit as a shipped acceptance gate (zero consultation sites in coreexec, the gate head the sole production consumer)"

requirements-completed: [PAR-03]

coverage:
  - id: D1
    description: Hook deny blocks at the chokepoint in BOTH runTurn branches with the FIRST denying hook's reason as the structured transcript denial
    requirement: PAR-03
    verification:
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/script_deny_blocks_end-to-end_(batch_branch)
        status: pass
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/script_deny_blocks_end-to-end_(subagent_branch)
        status: pass
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/the_FIRST_denying_hook's_reason_wins_(D-03)
        status: pass
    human_judgment: false
  - id: D2
    description: Hook ask opens the permission dialog EVEN UNGATED (allow_once executes, reject_once denies); the hookless ungated control stays dialog-free
    requirement: PAR-03
    verification:
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/hook_ask_in_ungated_mode_opens_the_dialog_(D-04)
        status: pass
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/hook_ask_answered_reject_once_denies_without_a_rule_write
        status: pass
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/hookless_ungated_control_stays_dialog-free_(criterion_4_coexists)
        status: pass
    human_judgment: false
  - id: D3
    description: USER-scope hook allow executes without consulting rules or opening a dialog (hook-scoped, never persisted); without the hook the same call asks
    requirement: PAR-03
    verification:
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/user-scope_allow_executes_without_rules_or_dialog_(D-01)
        status: pass
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/the_same_call_without_the_hook_asks_(the_allow_was_hook-scoped)
        status: pass
    human_judgment: false
  - id: D4
    description: NO-DECISION (silent, timed-out hooks) falls through to rule evaluation identical to the pre-join tree
    requirement: PAR-03
    verification:
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/silent_hook_falls_through_to_the_rules_(no_decision)
        status: pass
      - kind: unit
        ref: internal/session/gate_test.go#TestGateHookVerdict/timed-out_hook_falls_through_to_the_rules_(fail-open,_PAR-03)
        status: pass
      - kind: unit
        ref: 17-02 battery green unmodified under -race (go test -race ./internal/session/)
        status: pass
    human_judgment: false
  - id: D5
    description: The runtime composition wiring — a project settings.json deny hook blocks a REAL turn through discovery + sessionFor with the structured gate denial
    requirement: PAR-03
    verification:
      - kind: integration
        ref: internal/runtime/hooks_join_wiring_test.go#TestHookJoin_ProjectDenyThroughComposition
        status: pass
    human_judgment: false
  - id: D6
    description: Executor-leg disposal — hook side effects fire EXACTLY ONCE per call, PostToolUse observation survives, zero executor consultation sites (grep audit)
    requirement: PAR-03
    verification:
      - kind: unit
        ref: internal/coreexec/hooks_wiring_test.go#TestHooksOnceOnly
        status: pass
      - kind: unit
        ref: internal/coreexec/hooks_wiring_test.go#TestRegisterCorePreToolUseDisposal
        status: pass
      - kind: other
        ref: grep audit — `grep -rn "hooks\.PreToolUse(" internal/coreexec/` == 0 AND `grep -c "PreToolUseVerdict(" internal/session/gate.go` >= 1
        status: pass
    human_judgment: false
  - id: D7
    description: The @-mention Read-rule consult is backed by the perm rule set (deny denies the mention; ask does not) — mentions and tool calls answer to one rule authority
    verification:
      - kind: integration
        ref: internal/runtime/hooks_join_wiring_test.go#TestHookJoin_ReadRuleEvaluatorBackedByPerm
        status: pass
    human_judgment: false

duration: 20 min
completed: 2026-09-03
status: complete
---

# Phase 21 Plan 06: The Hook→Gate Join Summary

**Hook PreToolUse verdicts joined Phase 17's single gateCall head (deny→gateDeny with first reason, ask→gateSuspend even ungated, user-allow→gateExecute) with the executor leg disposed — one gate, one consultation site, one result form per decision, proven by grep and by a once-only side-effect test**

## Performance

- **Duration:** 20 min
- **Started:** 2026-09-03T22:34:02Z
- **Completed:** 2026-09-03T22:54:59Z
- **Tasks:** 2 (both TDD: RED → GREEN)
- **Files modified:** 13 (+1135/−266 over internal/)

## Accomplishments

- The gate head is LIVE: `gateCall` consults the injected `PreToolUseVerdict` (21-01's seam, the deny-wins resolver) before rules, maps the four-valued verdict totally — deny → `gateDeny` carrying the first denying hook's reason (`permissionHookDenyForm`), ask → `gateSuspend` UNCONDITIONALLY (D-04: even ungated, riding the existing queued permission ask; no new ask path), USER-scope allow → `gateExecute`, NO-DECISION → the untouched 17-02 rule tree. Precedence documented verbatim at the chokepoint: **hook verdict → permission ask → execute**.
- The executor PreToolUse leg is DISPOSED: `coreexec.ToolHooks` reduced to PostToolUse observation, `HookRunner.PreToolUse` and `errHookRefused` deleted. The single-consumption invariant holds by grep (zero consultation sites in coreexec; the gate head is the sole production `PreToolUseVerdict` consumer) and by the once-only marker test.
- The runtime composition wires the join: the session HookRunner's verdict into the gate deps, and the 21-04 `readRuleEvaluator` seam backed by the perm rule-set provider (`VerdictDeny → deny, else allow`) — @-mentions and tool calls answer to ONE rule authority.
- Both runTurn branches proven: hook deny blocks the batch-eligible path AND the subagent dispatch branch before `DispatchSubagent`.

## Task Commits

Each task was committed atomically (TDD):

1. **Task 1 RED: failing tests for the hook-verdict gate head join** - `1a2ded4` (test)
2. **Task 1 GREEN: hook verdicts join the gateCall head end-to-end** - `da0bef3` (feat)
3. **Tracer flake fix: hook-ask subtest waits on the transcript result** - `de82fee` (fix, Rule 1)
4. **Task 2 RED: failing tests for the executor PreToolUse leg disposal** - `db7c075` (test)
5. **Task 2 GREEN: dispose the executor PreToolUse leg — one consultation site** - `57cd111` (feat)

**Plan metadata:** (docs commit below)

## Files Created/Modified

- `internal/session/gate.go` - the live head (`gateHookVerdict` + total mapping), `permissionHookDenyForm`, `GateDeps.PreToolUseVerdict`, gateCall's ctx, the precedence contract in the package/step-1 docs
- `internal/session/session.go` - ctx passed at both gateCall consumption sites
- `internal/session/gate_test.go` - `TestGateHookVerdict` (10 subtests: both branches, first-reason, ask-ungated matrix, user-allow + hookless controls, silent/timeout fall-through)
- `internal/coreexec/register.go` - `ToolHooks` reduced to PostToolUse; `withHooks` observes only; disposal recorded in the seam comments
- `internal/coreexec/bash.go` - `errHookRefused` deleted
- `internal/coreexec/hooks_wiring_test.go` - `TestHooksOnceOnly` (real script marker), `TestRegisterCorePreToolUseDisposal`
- `internal/ecosys/hooks.go` / `hookverdict.go` / `loader.go` - Verdict + HookScope constants exported (rename-only); `HookRunner.PreToolUse` deleted
- `internal/ecosys/hooks_test.go` / `hookverdict_test.go` - boolean-seam tests retired/ported to `PreToolUseVerdict`
- `internal/runtime/runtime.go` - the join composition (gate deps + readRuleEvaluator) and the seam comment updates
- `internal/runtime/hooks_join_wiring_test.go` (NEW) - the real-settings-tree composition deny test + the perm-backed Read-rule join test

## Decisions Made

- **ask → gateSuspend unconditionally** (the plan's letter): the head consults NOTHING else — one delegation, one total mapping; a hook ask is the operator's explicit escalation lever, so it suspends even ungated rather than consulting mode/automation checks (T-21-20's total-mapping mitigation).
- **gateCall regained ctx** (the 17-02 locked-contract signature `gateCall(ctx, turnID, callID, tool, input)`): the landed code had dropped ctx because nothing needed it; hook execution does. Chokepoint placement, verdict vocabulary, and the head seam matched the locked contract verbatim — no divergence, no halt.
- **Constant exports are rename-only** (`verdictNone`→`VerdictNone`, `scopeProject`→`ScopeProject`, …): no aliasing, one source of truth; 21-01's STATE decision ("Verdict constants unexported until 21-06 exports by necessity") anticipated exactly this.
- **readRuleEvaluator wired once per Runner**: every session in a Runner shares the workDir, so the first successfully-opened perm store is the standing provider (the gate itself keeps each session's live store); the once-guard makes concurrent sessionFor construction race-free against turn-time reads.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Hook-ask subtest raced the transcript append**
- **Found during:** Task 1 tracer feedback gate (re-running the tracer `<verify>` under `-race` exposed it)
- **Issue:** The allow_once assertion waited on the store's order slice (`noteExec` fires inside the tool) and then read the transcript result, which lands a millisecond later under load — the documented 17-01/17-02 flake family
- **Fix:** Wait on BOTH the order and the transcript result, mirroring `TestGatePermissionSuspend`'s discipline
- **Files modified:** internal/session/gate_test.go
- **Verification:** 3 consecutive `-race` runs of the tracer verify command green
- **Committed in:** de82fee

**2. [Rule 3 - Blocking] ecosys verdict/scope constants had to be exported for the join**
- **Found during:** Task 1 RED (tests could not construct scoped hooks or consume the verdict enum cross-package)
- **Issue:** `verdictNone`…`verdictAllow` and `scopeProject`/`scopeUser`/`scopePlugin` were unexported; the gate (and its battery) is an external consumer of the enum by design
- **Fix:** Rename-only export (no aliases, no behavior change), with export-rationale comments at both const blocks; rode in the RED commit
- **Files modified:** internal/ecosys/{hooks,hookverdict,loader,hooks_test,hookverdict_test}.go
- **Verification:** `go test ./internal/ecosys/ -count=1` green pre- and post-rename; full suite green
- **Committed in:** 1a2ded4

**3. [Rule 3 - Blocking] The disposal's blast radius included three unplanned files**
- **Found during:** Task 2 GREEN (the deletion would not compile)
- **Issue:** The plan listed register.go + hooks_wiring_test.go, but the executor consultation's implementation lived in `ecosys.HookRunner.PreToolUse` (whose doc said "until 21-06 disposes it"), its test `TestPreToolUseDelegation`, and `errHookRefused` in bash.go — all dead policy seams by the plan's own prohibition ("MUST NOT leave any second PreToolUse consumption path alive")
- **Fix:** Deleted all three with the leg (deletion, not stubbing); refusal semantics re-pinned via `PreToolUseVerdict` in `TestPreToolUseStdinAndExitRouting`
- **Files modified:** internal/ecosys/hooks.go, internal/ecosys/hooks_test.go, internal/coreexec/bash.go
- **Verification:** build + vet + full race suite green; the single-consultation grep audit passes
- **Committed in:** 57cd111

---

**Total deviations:** 3 auto-fixed (1 bug, 2 blocking)
**Impact on plan:** All three were required for the join to compile, stay deterministic under `-race`, and honor the no-second-consumption-path prohibition. No scope creep — the ecosys exports were pre-authorized by the 21-01 STATE decision.

## TDD Gate Compliance

Both tasks carry `tdd="true"`; the gate sequence is present and ordered:

| Task | RED (test) | GREEN (feat) | Status |
|------|------------|--------------|--------|
| 1 | 1a2ded4 | da0bef3 (+ de82fee fix) | Pass |
| 2 | db7c075 | 57cd111 | Pass |

RED commits failed for the right behavioral reasons (deny didn't block; ask didn't suspend; allow didn't shortcut; marker fired 4× for 2 calls; the executor still refused), verified before each GREEN.

## Issues Encountered

- **`mise ci` lint leg is pre-existing red (environmental):** `golangci-lint run` panics with `file requires newer Go version go1.27 (application built with go1.26)` — captured byte-identically BEFORE any change (baseline at /tmp/lint_before.txt) and after. The installed golangci-lint v2.12.2 cannot load a go1.27-requiring dependency. The other `ci` legs — `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test -race -count=1 ./...` — are all green. This is the known drift called out in the execution brief, not a regression of this plan.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 21 is COMPLETE (6/6 plans). PAR-03 closed end-to-end: settings.json hooks (project + user scopes) resolve through deny-wins verdicts and consume at Phase 17's single gateCall head.
- The no-second-gate grep (`perm.RuleSet.Evaluate` in internal/session non-test sources == gate.go only) still holds; docs/permissions-gate.md §1.3's contract is now live code.
- Out-of-scope leftovers (unchanged, tracked in STATE): CR-04 per-turn origin and the hasSubstitution WR-02 fix in internal/perm remain deferred to the next phase touching that code.

## Self-Check: PASSED

All 13 key-files exist on disk; all 5 task commits exist in history (1a2ded4, da0bef3, de82fee, db7c075, 57cd111). Plan `<verification>` re-run green: `go test -race ./internal/session/ ./internal/coreexec/ ./internal/ecosys/ -count=1` ok; the single-consultation grep audit passes; the no-second-gate grep clean; vet + CGO_ENABLED=0 build + full `go test -race ./...` green (lint leg pre-existing red, environmental — see Issues Encountered).

---
*Phase: 21-context-policy-parity-closures*
*Completed: 2026-09-03*
