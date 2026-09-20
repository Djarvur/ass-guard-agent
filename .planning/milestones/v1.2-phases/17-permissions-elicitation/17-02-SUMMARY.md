---
phase: 17-permissions-elicitation
plan: 02
subsystem: permissions
tags: [acp, request-permission, permission-gate, chokepoint, ask-queue, config-options, cc-parity, fail-safe]

# Dependency graph
requires:
  - phase: 17-permissions-elicitation (17-01)
    provides: internal/perm rule grammar (ParseRule/RuleSet.Evaluate/MCPName/SplitMCPName) + the permissions.yaml store (Rules/AllowTool/ForbidTool, 0600 atomic)
  - phase: 16-acp-wire-foundation (16-03/16-05)
    provides: the id'd outbound request Registry (Call/Deliver/ResolveCancelled, HUMAN-ASK class) and the ConfigSurface advertisement with the permissions.mode pending slot + `_global/` twins
provides:
  - "internal/session/gate.go — gateCall, THE one per-call permission pipeline (hook-verdict head for Phase 21 → perm rules in BOTH modes → D-07 automation decline → mode decision → degraded-client guard), gateExecute/gateDeny/gateSuspend/gateDeclineAutomation verdicts, GateDeps injection (Rules/Mode/Allow/Forbid/Fire/Queue/Subject), PermModeUngated/PermModeGated"
  - "internal/session/askqueue.go — AskQueue minimal core: one-outstanding fired head, FIFO completion chain, AskEntry{turnID, sessionID, callID, tool, title, kind, input, class, seq}, AskOutcome{Selected/Cancelled/Err/Unsupported}; priority classes, notes + counter, turn-death drain expand in 17-03"
  - "internal/session/ask.go — PendingAskKindPermission + resumePermissionAsk (persist-if-always BEFORE execute; cancelled → cancelled-NORMAL; reject_always persists deny BEFORE the denial result; failures decline fail-safe) + executeGatedCall riding the loop's own subagent/batch paths"
  - "internal/acp — RequestPermissionFrame/PermissionOption/PermissionOutcomeFrame v1-verbatim (the four canonical option kinds) + MethodRequestPermission + Server.Registry() accessor"
  - "internal/acpserve/ask_surface.go — BuildPermissionAsk (neutral symmetric four-option frame, ToolCallUpdate toolCall carrying the raw input as content) + PermissionAsk.Fire (registry Call under HUMAN-ASK; cancelled/-32800 → cancelled; -32601 → Err+Unsupported; timeout → Err)"
  - "internal/acpserve/config_surface.go — the REAL permissions.mode handler (typed two-value validation, scope-aware idempotence guard, WriteLayerOption persist, live apply hook, EffectivePermMode boot resolution) behind 16-05's advertised slot; compaction-threshold stays pending"
  - "internal/runtime — gate composition (perm store on the .ass-guard floor, live mode accessor read per call, queue + fire-callback injection, automation turn-origin marker around runAutomationTurn)"
affects: [17-03-ask-queue-expansion, 17-04-elicitation, 17-05-permission-docs, 21-hooks, ACP-01]

# Actuals (#2632) — chars/4 over the realized diff (~150k chars), same scale as the plan's estimate
actuals:
  tokens: 37500
  tasks: 3
  commits: 8

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "ONE chokepoint: a per-call gate seam beside planModeBlocks in runTurn's loop — and in the subagent branch before DispatchSubagent — with nil deps preserving v1.1 behavior for unwired sessions"
    - "Suspend-don't-block: the gate NEVER waits on a human inside the turn; suspension ends the turn with the ask marker and the ask queue's pump goroutine owns the fire round-trip + the resume (no turn lock held)"
    - "Persist-then-act trust discipline: allow_always writes the rule BEFORE the gated call executes; reject_always writes the deny rule BEFORE the denial result; a failed write downgrades the click, never silently widens or executes"
    - "Live accessor + boot-from-layer: the gate reads the mode per call through a runner-owned accessor the ConfigSurface hook flips; the accessor is seeded from the layer resolution at startup (chip==wire across restarts)"
    - "Typed fail-safe outcomes: AskOutcome.Err declines (never allow); Unsupported (-32601) marks a STICKY per-session degradation; transient errors stay non-sticky"

key-files:
  created:
    - internal/session/gate.go
    - internal/session/askqueue.go
    - internal/session/gate_test.go
    - internal/acpserve/ask_surface.go
    - internal/acpserve/ask_surface_test.go
  modified:
    - internal/session/ask.go
    - internal/session/session.go
    - internal/session/goconst_constants.go
    - internal/session/projector.go
    - internal/session/planmode_test.go
    - internal/acp/types.go
    - internal/acp/server.go
    - internal/acp/request_registry_test.go
    - internal/acpserve/config_surface.go
    - internal/acpserve/config_test.go
    - internal/acpserve/acp_serve.go
    - internal/runtime/runtime.go

key-decisions:
  - "THE chokepoint shape: gateCall invoked beside planModeBlocks AND inside the subagent branch (D-05 — no second permission path); a suspension defers to after the batch's survivors dispatch, then ends the turn with the SAME stopAsk marker as the question family"
  - "D-07 automation decline precedes the mode decision — ask-class calls decline on automation turns in BOTH modes (never a dialog nobody answers, never a silent allow); deny/allow rules are evaluated first and stay enforced"
  - "Permission asks do NOT occupy the AskBroker: the queue entry carries fire (surface round-trip) + resolve (the resume variant); the broker's single slot stays question-family — the queue is the single firing path for ask surfaces (the 17-02 assumption-delta promotion)"
  - "The mode accessor is runner-owned and read PER CALL (Pitfall 8); the ConfigSurface advertisement resolves currentValue through it (chip==wire with the gate), and EffectivePermMode boots it from the layer files so a hand-edited gated layer gates after restart"
  - "MCP rule subjects resolve through the injected GateDeps.Subject built on perm.MCPName/SplitMCPName — the catalog already registers mcp__<server>__<tool> names, so the mapping is canonicalize + identity (Pitfall 7, both directions table-pinned)"
  - "Session-side sticky degradation: the surface flags -32601 as AskOutcome.Unsupported; the resume marks the session degraded so later gated asks decline without a new registry round-trip (16-D-18, no retry storm); transient failures are not sticky"

patterns-established:
  - "Pattern: gateVerdict pipeline (hook head → rules → automation → mode → degraded → suspend) — Phase 21's hook runner joins at the documented Step-1 head; 17-05's grep audit can pin ONE rule-evaluation call site family"
  - "Pattern: AskQueue Enqueue(entry, resolve) — producers never block on humans; the pump's fire→resolve chain gives FIFO serialization of dialog FIRING, never of turn execution"
  - "Pattern: executeGatedCall reuses the loop's own execution paths (subagent dispatch / deadline-wrapped DispatchBatch) so an allowed call keeps its normal deadline and boundary semantics"

requirements-completed: [ACP-01]

# Coverage metadata (#1602) — one entry per shipped deliverable
coverage:
  - id: D1
    description: "THE one per-call gate chokepoint in runTurn's loop for BOTH dispatch branches and BOTH modes: ungated default executes ask-class calls with zero dialogs, deny rules still deny ungated, subagent Task calls gate per their catalog class before DispatchSubagent"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateChokepoint_UngatedDefaultNoDialog"
        status: pass
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateChokepoint_UngatedDenyStillDenies"
        status: pass
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateChokepoint_SubagentBranchGated"
        status: pass
      - kind: other
        ref: "mise ci (vet + golangci-lint v2 + CGO_ENABLED=0 build + go test -race ./...)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Gated ask-class suspension end-to-end: turn ends with the ask marker (no executor call), the queue fires the surface exactly once with a v1 RequestPermissionFrame (four neutral option kinds), allow_always persists the rule BEFORE the gated call executes, the result lands loud, the SAME turn resumes, and the follow-up call hits the allow rule with no second dialog; a second prompt completes while the dialog is open (-race)"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/gate_test.go#TestGatePermissionSuspend"
        status: pass
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateChokepoint_SecondPromptWhileDialogOpen"
        status: pass
      - kind: unit
        ref: "internal/acpserve/ask_surface_test.go#TestPermissionAskFrame"
        status: pass
      - kind: unit
        ref: "internal/acpserve/ask_surface_test.go#TestPermissionAskDispatch"
        status: pass
    human_judgment: false
  - id: D3
    description: "Complete dialog outcome matrix through the one path: allow_once executes without persisting, reject_once denies and asks again, reject_always persists the deny rule BEFORE the denial (next matching call denies with no dialog), cancelled appends a cancelled-NORMAL (non-error) result and resumes"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateOutcomeMatrix"
        status: pass
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateChokepoint_CancelledNormalResume"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-07 automation fail-safe: on automation-origin turns ask-class calls decline with a client-visible transcript note + structured log in BOTH modes, deny and allow rules stay enforced, and the human control case still suspends"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateAutomationDecline"
        status: pass
    human_judgment: false
  - id: D5
    description: "Degraded-client safety: a -32601 answer maps to decline (never allow) and is STICKY for the session — later gated asks decline without a new registry round-trip; transient failures ask again"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateDegradedClient"
        status: pass
    human_judgment: false
  - id: D6
    description: "MCP rule-namespace mapping: the gate's rule subject canonicalizes through perm.MCPName/SplitMCPName (both directions pinned); namespace-form rules match fake-registered MCP tools and bare-server rules select the whole namespace"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/gate_test.go#TestGateMCPNamespace"
        status: pass
    human_judgment: false
  - id: D7
    description: "permissions.mode is real end-to-end: set_config_option persists via WriteLayerOption (0600 atomic) BEFORE the live apply, the flip reaches the running session's very next tool call (no recreation), invalid values are typed rejects, re-pushes are idempotent no-ops, _global/ routes to the global layer, and the boot default (or a hand-edited layer) resolves ungated/gated truth"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/acpserve/config_test.go#TestPermissionsModeFlip"
        status: pass
      - kind: unit
        ref: "internal/session/gate_test.go#TestGatePermissionModeFlip"
        status: pass
    human_judgment: false
  - id: D8
    description: "Live-Zed native dialog rendering (criterion 1's operator leg): the agent-supplied four-option list drives Zed's dialog buttons"
    requirement: ACP-01
    verification: []
    human_judgment: true
    rationale: "Live Zed rendering is 17-05's operator checkpoint per the plan's verification section (deterministic wire-level coverage shipped here); needs a human in a real editor session"

# Metrics
duration: 52 min
completed: 2026-09-01
status: complete
---

# Phase 17 Plan 02: Permission Gate Chokepoint + request_permission Surface Summary

**THE one per-call permission gate (hook head → rules → automation → mode → suspend/decline) with a native four-option request_permission dialog riding a new minimal ask queue, persist-then-execute trust semantics, and the real live-flip permissions.mode handler**

## Performance

- **Duration:** 52 min
- **Started:** 2026-08-31T23:33:28Z
- **Completed:** 2026-09-01T00:25:07Z
- **Tasks:** 3
- **Files modified:** 17 (5 created, 12 modified)

## Accomplishments
- The ONE gate chokepoint exists in code: `gateCall` runs per call beside `planModeBlocks` AND inside the subagent branch — rule evaluation in both modes, D-07 automation decline, mode/human/degraded decisions — with the hook-verdict head documented as Phase 21's join seam (D-04/D-05)
- Criterion 1's spine works end-to-end at the wire/test level: gated ask-class call → suspend (no executor call, no lock held) → single HUMAN-ASK registry fire with the v1 four-option frame → allow_always persisted BEFORE execution → loud result, same-turn resume, zero second dialog
- The full outcome matrix (D-01/D-03 both directions, cancelled-normal), the D-07 automation decline in both modes, sticky -32601 degradation (16-D-18), and the MCP namespace mapping (Pitfall 7) all ride the same path
- permissions.mode is a real first-class ConfigSurface option: persist-then-apply (layer content at apply time is test-pinned), live per-call mode accessor, typed rejects, idempotent re-push, `_global/` routing, and layer-truth boot — compaction-threshold stays pending for Phase 19

## Task Commits

Each task was committed atomically (TDD RED→GREEN):

1. **Task 1: Tracer — gated permission ask end-to-end through the ONE chokepoint**
   - `cba2e89` test(17-02): add failing permission-gate chokepoint battery (RED)
   - `aec080a` feat(17-02): implement the permission-gate chokepoint — suspend, native ask, persist-then-execute (GREEN)
2. **Task 2: Outcome matrix, automation decline, MCP namespace, degraded clients**
   - `552d626` test(17-02): add failing outcome-matrix, automation-decline, degraded-client, MCP-namespace battery (RED)
   - `a1e72fd` feat(17-02): complete the chokepoint decision surface — outcome matrix, automation decline, MCP namespace, degraded clients (GREEN)
3. **Task 3: permissions.mode real handler — persist-then-apply live flip**
   - `a3c4cf9` test(17-02): add failing permissions.mode real-handler battery (RED)
   - `f54469e` feat(17-02): register the real permissions.mode handler — persist-then-apply live flip (GREEN)
4. **Gate repair:** `151b433` fix(17-02): settle-based flip assertion + test window margin for load-sensitive ladder tests

## Files Created/Modified
- `internal/session/gate.go` — gateCall pipeline, gateVerdict/GateDeps, ask-class mirror of isAloneInSlot, deny/decline/cancelled result forms
- `internal/session/askqueue.go` — AskQueue minimal core (one-outstanding fire loop, FIFO completion, class/seq-carrying entries)
- `internal/session/ask.go` — PendingAskKindPermission, resumePermissionAsk + resolvePermissionOutcome matrix, executeGatedCall (subagent/batch paths)
- `internal/session/session.go` — gate invocation in both runTurn branches, gate/automationTurn/permDegradedFlag fields
- `internal/session/gate_test.go` — the full gate battery (12 tests: suspend chain, chokepoint matrix, automation, degraded, namespace, mode flip)
- `internal/acp/types.go` — RequestPermissionFrame/PermissionOption/PermissionOutcomeFrame v1-verbatim + option-kind enum
- `internal/acp/server.go` — Server.Registry() accessor (deviation Rule 3)
- `internal/acpserve/ask_surface.go` — BuildPermissionAsk + PermissionAsk.Fire (HUMAN-ASK dispatch, fail-safe outcome mapping)
- `internal/acpserve/config_surface.go` — the real permissions.mode handler + perm mode read/apply seams + EffectivePermMode boot
- `internal/acpserve/acp_serve.go` — fire-callback + mode-seam composition wiring (deviation Rule 3)
- `internal/runtime/runtime.go` — gate composition in sessionFor (perm store, queue, live accessor, subject resolver), SetPermMode/PermMode/SetPermissionAskFire, automation turn-origin marker
- `internal/acp/request_registry_test.go` — test-window margin fix (see Deviations)

## Decisions Made
- **Chokepoint suspension defers to the batch:** a gateSuspend mid-batch lets the already-accumulated batch calls dispatch + record first, then ends the turn with the same stopAsk marker as the question family — the loop's established suspend shape, no second mechanism
- **Permission asks bypass the AskBroker:** the queue entry carries fire (surface round-trip) + resolve (the resume); the broker keeps serving questions — the 17-02 assumption-delta decision (queue = the single firing path) honored without touching the question machinery
- **Automation decline before mode:** D-07's letter ("in BOTH modes") puts the human-present check ahead of the ungated short-circuit; the RED test caught the Task-1 ordering and GREEN fixed it
- **Sticky vs transient degradation:** only -32601 (Unsupported) marks the sticky session flag; timeouts/transient failures re-ask on the next call — the plan pins stickiness for -32601 only, and the test family proves both polarities
- **Boot-from-layer:** the runner's mode accessor is seeded at composition from EffectivePermMode (project > global > ungated) so the advertisement, the gate, and a hand-edited layer agree after restart (chip==wire)
- **reject_always persist failure still denies:** a failed deny-rule write logs loudly but never turns a rejection into an execution

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `acp/server.go` Registry accessor + `acpserve/acp_serve.go` fire injection**
- **Found during:** Task 1 (GREEN wiring)
- **Issue:** the plan's 11-file footprint omits both wiring seams: the ask surface needs the 16-03 registry handle (private on Server) and the runner-side fire-callback injection point (the runtime cannot import acpserve, and acpserve must bind post-NewServer)
- **Fix:** one exported getter (`Server.Registry()`) and one composition block in Run (`NewPermissionAsk` + `SetPermissionAskFire`, later plus the Task-3 mode seams and boot seeding)
- **Files modified:** internal/acp/server.go, internal/acpserve/acp_serve.go
- **Verification:** full gate battery + mise ci green
- **Committed in:** aec080a, f54469e

**2. [Rule 3 - Blocking] Pre-existing `TestRegistryRequestCancelledResponse` timing fragility**
- **Found during:** post-Task-3 `mise ci` (second run; the first run's failure was this plan's own test race, fixed in the same commit)
- **Issue:** the 16-03 test shrank the FAST-CONTROL window to 4ms; under full-suite parallel load the test goroutine's Deliver lost the race to the 8ms D-14 ladder — the 17-01 deferred-flake family RECURRING, this time captured (full log + root cause in deferred-items.md per the watchlist's capture-first plan)
- **Fix:** named `testFastWindow = 25ms` with the no-retry proof scaled to 3 windows — the test's assertions (immediate -32800 resolution, zero retries) preserved and strengthened; `mise ci` then fully green (zero FAIL lines)
- **Files modified:** internal/acp/request_registry_test.go, .planning/phases/17-permissions-elicitation/deferred-items.md
- **Verification:** `mise ci` green end-to-end; acp package 10x standalone green
- **Committed in:** 151b433

**3. [Rule 1 - Lint] goconst extraction of the `Write`/`Edit` literals**
- **Found during:** Task 1 (lint gate)
- **Issue:** adding the gate's primary-arg extraction made the `"Write"` literal cross the package's goconst threshold (projector.go/planmode_test.go literals newly flagged as needing the shared constant)
- **Fix:** `toolWrite`/`toolEdit` consts in goconst_constants.go, adopted at the flagged sites
- **Files modified:** internal/session/goconst_constants.go, internal/session/projector.go, internal/session/planmode_test.go
- **Verification:** golangci-lint 0 issues
- **Committed in:** aec080a

---

**Total deviations:** 3 auto-fixed (2 blocking wiring/fragility, 1 lint) + 1 in-plan test-race repair (151b433, the tracer's own battery)
**Impact on plan:** all fixes are within the plan's architecture; no scope creep. The wiring seams were implied by the plan's own must-have key_links but missing from its file list.

## Issues Encountered
- The transient `-race` gate flake family from 17-01 recurred during `mise ci` — twice, each time now fully captured (the first run also exposed a genuine race in this plan's TestGatePermissionModeFlip eager assertion, fixed to a settled-state wait). Root cause for the recurring family: load-sensitive millisecond-scale test windows, not product code. Margin fix applied, deferred-items watchlist updated with the capture, and the final `mise ci` is green with zero FAIL lines.

## TDD Gate Compliance
- All three tasks followed RED→GREEN with `test(17-02)` commits preceding `feat(17-02)` commits (cba2e89→aec080a, 552d626→a1e72fd, a3c4cf9→f54469e); no REFACTOR commits needed (lint-clean at each GREEN; the post-gate fix commit is a test-robustness fix, not a refactor gate).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- 17-03 expands the queue: priority classes (fg before bg), D-12 enqueue notes + counter, D-13 `DrainTurn(turnID)` wired to all three teardown paths — the entries already carry turnID/class/seq and the counting-fake registry test can pin zero firings outside the pump
- 17-04's elicitation surface joins the same queue (one firing path for every ask surface); the surface's capability-probe/dispatch branch (16-D-13/D-18) slots beside PermissionAsk
- 17-05's doc task owns the chokepoint audit grep — `gateCall` is the only rule-evaluation call site family in the session package (both branches verified by tests)
- Live-Zed native-dialog rendering remains the operator checkpoint (D8 above, 17-05's verification leg); known edge for 17-03: a client prompt arriving while a permission dialog is open projects the suspended call's unpaired tool_use — the same bounded shape as the existing question suspension, owned by the queue/drain work

---
*Phase: 17-permissions-elicitation*
*Completed: 2026-09-01*

## Self-Check: PASSED

- Created files verified on disk: internal/session/{gate,askqueue,gate_test}.go, internal/acpserve/{ask_surface,ask_surface_test}.go
- All 7 task/gate commits verified in git log: cba2e89, aec080a, 552d626, a1e72fd, a3c4cf9, f54469e, 151b433
- Plan-level verification re-run green: `go test -race` over the session gate battery and the acpserve ask/mode batteries; final `mise ci` green with zero FAIL lines (captured at /tmp/17-02-mise-ci-3.log)
- ACP-01 left unmarked in REQUIREMENTS.md per the shared-ID gate (later phase-17 plans also declare it; the last declaring sibling marks it)
