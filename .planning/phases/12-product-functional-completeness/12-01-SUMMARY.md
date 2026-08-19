---
phase: 12-product-functional-completeness
plan: "01"
subsystem: acp
tags: [acp, askuserquestion, suspension, d01-timeout, tool-executors, capture-grounded, engine-ask]

requires:
  - phase: 08-slash-command-kickoff
    provides: "the core-executor template (RegisterCore, fixture discipline, plainContent seam) + the deferred-tools table quoting the captured answered form"
provides:
  - "AskUserQuestion executes for real end-to-end: capture-shaped question surface to the ACP client, turn suspension on the engine ask path, operator reply lands as the tool result, D-01 timeout resumes autonomously"
  - "internal/session AskBroker — the reusable suspension/resume seam every later interactive feature rides (plan-mode approval, cron engine-driven turns, eval ask scenarios)"
  - "internal/coreexec/testdata/zcode-interactive-results.json — the interactive-tool forms fixture (answered form capture-quoted via the 08-08 note; non-answer corpus-absent w/ D-01 default + 12-05 upgrade pointer)"
  - "engine TurnOutput.AskSuspended -> ActionAsk with NO table lookup (the no-chain pin) + the learning store bypass for tool-suspension asks"
  - "--ask-timeout serve flag (D-01: default 10m, 0 = block forever)"
affects: [12-03, 12-05, 12-07, 12-08, 13-openspec-workflow-completion]

actuals:
  tokens: 34946   # chars/4 over the 12-01 production commits (estimate was 88000 — the planner over-estimated ~2.5x; witness-wait wall time inflates duration, not tokens)
  tasks: 3
  commits: 8

tech-stack:
  added: []
  patterns:
    - "Interactive-tool executor class: parse -> questions ride the result Output -> ErrSuspended sentinel; the session loop (sole holder of turnID+callID) binds the PendingAsk, records ask_suspended, ends the turn with the internal 'ask' marker"
    - "First-claimant-wins broker Claim: the operator reply and the D-01 timer race; exactly one drives the shared resume path"
    - "Client surface via the bus: the broker's onSurface publishes an AgentMessageChunk that the ACTIVE Run's forwarder turns into session/update just before the suspended turn's response"

key-files:
  created:
    - internal/session/ask.go
    - internal/coreexec/ask.go
    - internal/coreexec/testdata/zcode-interactive-results.json
    - cmd/ass-guard/ask_wiring_test.go
    - internal/session/ask_test.go
    - internal/coreexec/ask_test.go
    - internal/engine/ask_test.go
  modified:
    - internal/session/session.go
    - internal/session/transcript.go
    - internal/session/manager.go
    - internal/engine/types.go
    - internal/engine/decide.go
    - internal/engine/observe.go
    - cmd/ass-guard/acp_serve.go

key-decisions:
  - "Suspension over blocking (ROADMAP SC-1 + D-01): the executor returns ErrSuspended; the turn ENDS with the internal 'ask' stop marker mapped to a completed ACP turn; the reply arrives as the NEXT session/prompt and is routed as the pending call's tool result — the reply IS the result, no new user message"
  - "The executor's parsed questions ride the suspended result's Output (the Stub seam carries no call identity); the session loop completes the PendingAsk at the exact point it knows turnID+callID — T-12-01-01 keying preserved"
  - "The answered/non-answer renderers live in internal/session (single definition) with coreexec's conformance test pinning them to the fixture — coreexec imports session (AskBroker), so the forms cannot live in coreexec without a cycle"
  - "A suspended turn never chains: Decide returns ActionAsk with NO table lookup (even against an all-continue table + provenance rows), and applyDispatcher bypasses the learning store for the ask:suspended signal"
  - "The client surface renders SINGLE-LINE: the ACP Writer's decoded-newline transport guard drops newline-carrying frames (the live-witness finding) — corpus-absent client form, documented default, test-pinned"

patterns-established:
  - "Executor + fixture + registration + wiring test — the interactive-tool class of the 08-08 executor template (re-proven by this plan)"
  - "Server-level wiring test: drive the REAL acp.Server over stdio pipes with a scripted suspending provider — the runner-level emitter test alone could not see the live-path gap"

requirements-completed: [ACP-01]

coverage:
  - id: D1
    description: "AskUserQuestion end-to-end offline battery: suspension, D-01 timeout resume with the non-answer form, reply-as-tool-result in the captured answered form, D-01 defaults (10m/0), no-chain engine pin, fixture conformance, schema-never-rewritten"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/session/ask_test.go#TestAsk_SuspendsAndTimesOutEndToEnd"
        status: pass
      - kind: unit
        ref: "internal/session/ask_test.go#TestAsk_ReplyResumesSameTurn"
        status: pass
      - kind: unit
        ref: "internal/session/ask_test.go#TestAsk_TimeoutDefaults"
        status: pass
      - kind: unit
        ref: "internal/coreexec/ask_test.go#TestAskForms_ConformToFixture"
        status: pass
      - kind: unit
        ref: "internal/coreexec/ask_test.go#TestRegisterAsk_SchemaNeverRewritten"
        status: pass
      - kind: unit
        ref: "internal/engine/ask_test.go#TestDecide_AskSuspendedNeverChains"
        status: pass
      - kind: integration
        ref: "cmd/ass-guard/ask_wiring_test.go#TestAskWiring_ReplyRouting"
        status: pass
    human_judgment: false
  - id: D2
    description: "The live phase-gate leg: a real serve session where the real model asks, the operator answers from a real ACP client, the reply lands as the tool result; plus the D-01 hands-off leg (30s timeout, no reply, model proceeds/declines autonomously)"
    requirement: ACP-01
    verification:
      - kind: e2e
        ref: "operator witness 2026-08-19, sessions d9f98023 (leg 1) + e253bfbd (leg 2) — verdict approved; evidence quoted verbatim below"
        status: pass
    human_judgment: true
    rationale: "Live-operator witness by design (the milestone rule: no feature closes with stub-only evidence); attested by the operator with one finding, fixed and re-proven at test level in this plan"

duration: 183min (includes the operator-witness wait)
completed: 2026-08-19
status: complete
---

# Phase 12 Plan 01: AskUserQuestion end-to-end Summary

**AskUserQuestion executes for real: capture-shaped question surface to the ACP client, turn suspension on the engine ask path (never chains), operator reply lands as the tool result in the captured answered form, and the D-01 timeout (10m default, 0 = block forever) resumes autonomously — live-proven by the operator witness, whose one finding (the surface vanishing on the live wire) was root-caused to the Writer's newline transport guard, fixed, and pinned by a server-level regression test.**

## Performance

- **Duration:** 183 min (includes the operator-witness wait + the post-verdict fix)
- **Started:** 2026-08-18T21:19:23Z
- **Completed:** 2026-08-19T00:22:50Z
- **Tasks:** 3 (2 code + the live-witness checkpoint)
- **Files modified:** 13 (7 created, 6 modified)

## Accomplishments

- The phase's flagship tool is REAL: the model's mid-turn question surfaces to the ACP client in a readable rendered form, the turn suspends on the engine's ask path (ActionAsk, no table lookup, learning store bypassed), and the operator's reply lands as the pending call's tool result — the reply IS the result, verbatim in the captured `User has answered your questions: "…"` form.
- D-01 proven live both ways: leg 1 (reply wins), leg 2 (30s timeout fires autonomously, the model received `User has not answered your questions (ask timed out after 30s); proceed or decline on your own.` and correctly declined to touch code — presenting a recommendation instead, honoring the original "ask me before touching code" instruction).
- The suspension/resume seam every later interactive feature rides (12-03 plan-mode, 12-07 cron firing, the eval net's ask scenarios): AskBroker with first-claimant-wins Claim (reply vs timer race), the shared resume path re-entering the SAME turn's model loop, session-close disarm.
- The interactive-tool executor template re-proven: executor + fixture + Execute-only registration + wiring test (fixture `zcode-interactive-results.json` — answered form quoted via the 08-08 harvest note; non-answer flagged corpus_absent with the D-01 default + the 12-05 re-record upgrade pointer).
- The witness finding root-caused and fixed: 450 transcript chunks vs 440 wire frames reconciled EXACTLY to the 10 newline-carrying frames the Writer's transport guard silently drops — the ask surface now renders single-line, pinned at server level.

## Task Commits

1. **Task 1 RED** — `99663e0` (test): the three-package failing battery (compile-absent seam = the dead end proven live)
2. **Task 1 GREEN** — `8a909f2` (feat): AskBroker + runTurn extraction + suspension branch + coreexec executor + engine ActionAsk
3. **Task 2 RED** — `8ffe40a` (test): wiring battery (reply routing, client surface, knob, schema discipline)
4. **Task 2 GREEN** — `4edd91a` (feat): sessionFor wiring + reply routing + `--ask-timeout` + engine adapter terminal-line detection
5. **Lint conformance** — `e45d2ae` (refactor): `mise ci` green
6. **Checkpoint staged** — `bc3cc95` (docs)
7. **Witness finding fix** — `1b38e3d` (fix): single-line surface + the server-level live-path regression pin
8. **Plan metadata:** this commit (docs)

_12-01 followed the house TDD gates: RED commit before every behavior-adding GREEN commit._

## Files Created/Modified

- `internal/session/ask.go` — AskBroker (PendingAsk keyed turnID+callID, Surface/Claim/ArmTimeout/Disarm, D-01 timeout), answered + non-answer renderers, SetAskBroker/ResolveAsk/resumeAskClaimed
- `internal/session/session.go` — Prompt split into Prompt + runTurn (the resume re-enters the SAME turn); ErrSuspended branch (no tool result appended, ask_suspended record, 'ask' marker); Close disarms the timer
- `internal/session/transcript.go` + `manager.go` — the `ask_suspended` line type + AppendAskSuspended
- `internal/coreexec/ask.go` — AskUserQuestionExecute (best-effort parse, questions on the Output), RegisterAsk (Execute-only), RenderAskSurface (single-line, structure not judgment)
- `internal/coreexec/testdata/zcode-interactive-results.json` — the interactive-forms fixture
- `internal/engine/types.go` + `decide.go` + `observe.go` — TurnOutput.AskSuspended, askSuspendedDecision (no table lookup), the learning-store bypass
- `cmd/ass-guard/acp_serve.go` — broker at sessionFor (surface via the bus), RegisterAsk beside RegisterCore, routeAskReply, mapAskStop, `--ask-timeout` flag, adapter ask_suspended terminal detection
- Test files: `internal/session/ask_test.go`, `internal/coreexec/ask_test.go`, `internal/engine/ask_test.go`, `cmd/ass-guard/ask_wiring_test.go` (incl. the server-level TestAskWiring_ServerLevelSurface)

## Live Witness Evidence (verbatim; /tmp originals will be reaped)

Leg 1 — session `d9f98023-93d8-478d-97da-7c2e604b416d` (operator verdict: PASS):

```
ask_suspended   turn-001  input=[{header:"Cache lib", options:[…ristretto/v2 (Recommended)…]}]
engine_decision turn-001  23:39:38.286  input="ask:suspended"  name="ask"
tool_result     turn-001  2026-08-18T23:42:20.897Z  callID=call_a58679437c4c4be69a9d4d4f
                output="User has answered your questions: \"ristretto\""
assistant_message (final) "Done — ristretto is integrated and verified. The program pri…"
```

The model acknowledged the answer and did real work on it: `go get ristretto/v2`, main.go rewrite, `go run .` → "hello, world".

Leg 2 — session `e253bfbd-0403-4c35-bda7-d9c6a251de86`, `--ask-timeout 30s`, NO reply (operator verdict: PASS):

```
ask_suspended   turn-001  23:58:15
tool_result     turn-001  2026-08-18T23:58:45.262Z (+30.000s exactly)
                output="User has not answered your questions (ask timed out after 30s); proceed or decline on your own."
assistant_message (resumed turn, 308 chunks): "I asked but the question timed out, and since you
                explicitly wanted to be consulted before any code changes, I haven't touched
                anything. Here's what I found and my recommendation: …"
```

Witness finding (fixed in `1b38e3d`): the rendered ask surface never reached the live client wire — the ACP Writer's decoded-newline transport guard (`internal/acp/framer.go` `containsDecodedNewline`) drops newline-carrying frames with the error swallowed; the multi-line render vanished between the bus and the wire (450 transcript chunks vs 440 wire frames = the 10 newline-carrying frames, reconciled exactly). Fixed by the single-line render; re-proven at test level by `TestAskWiring_ServerLevelSurface` (the REAL acp.Server over stdio pipes) — the operator's attestation stands, per the verdict, as the leg-1 surface observation evidence.

## Decisions Made

See key-decisions in the frontmatter — all five are capture/safety-relevant and recorded for downstream plans.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Wiring site located by symbol, landed in cmd/ass-guard**
- **Found during:** Task 2
- **Issue:** the plan's `files_modified` names `internal/runtime/runtime.go` + `internal/runtime/ask_wiring_test.go`; that layout exists only on the parked Phase-10 side branch (47f10b4) — master's wiring lives in `cmd/ass-guard/acp_serve.go`
- **Fix:** located `sessionFor(` by grep and edited it wherever it lived, exactly as the plan's own Truths instruct; the wiring test landed as `cmd/ass-guard/ask_wiring_test.go`
- **Files modified:** cmd/ass-guard/acp_serve.go, cmd/ass-guard/ask_wiring_test.go
- **Verification:** the full wiring battery green
- **Committed in:** 4edd91a

**2. [Rule 1 - Bug] "Executor calls Surface" realized as Output-carried questions + loop-bound PendingAsk**
- **Found during:** Task 1
- **Issue:** the plan's key_link says the executor calls Surface; the Stub seam (`func(ctx, args)`) carries no turnID/callID, so the executor CANNOT key the PendingAsk — and a broker-side stash would collapse two AskUserQuestion calls in one batch
- **Fix:** the executor parses and returns the questions on the result Output alongside ErrSuspended; the session loop (the one place that knows both ids) calls broker.Surface — functionally identical, race-free for N calls per batch, T-12-01-01 keying preserved
- **Files modified:** internal/coreexec/ask.go, internal/session/session.go
- **Verification:** TestAsk_SuspendsAndTimesOutEndToEnd + TestAskExecute_ParsesAndSuspends
- **Committed in:** 8a909f2

**3. [Rule 3 - Blocking] Form renderers live in internal/session (not beside the executor)**
- **Found during:** Task 1
- **Issue:** the plan places the resume renderer beside the executor, but coreexec imports session (AskBroker) — the renderers cannot live in coreexec without an import cycle
- **Fix:** single definition in `internal/session/ask.go`; coreexec's conformance test pins them to the fixture (the "defined once" requirement holds)
- **Verification:** TestAskForms_ConformToFixture
- **Committed in:** 8a909f2

**4. [Rule 1 - Bug] The live-witness surface finding (single-line render)**
- **Found during:** Task 3 (the operator's live witness — the exact failure mode the phase gate exists to catch)
- **Issue:** the multi-line RenderAskSurface block was dropped by the Writer's decoded-newline transport guard; the client never saw the question (the operator relayed it from the transcript tool_call input)
- **Fix:** single-line render (segments joined with " · ", questions with " ··· "), pinned by a no-newline assertion + the server-level TestAskWiring_ServerLevelSurface; the captured client-side form is corpus-absent, so the convention is the documented default
- **Files modified:** internal/coreexec/ask.go, internal/coreexec/ask_test.go, cmd/ass-guard/ask_wiring_test.go
- **Verification:** TestAskWiring_ServerLevelSurface green through the REAL acp.Server; `mise ci` green
- **Committed in:** 1b38e3d

---

**Total deviations:** 4 auto-fixed (1 blocking-site, 2 seam-layering, 1 live-witness bug) + 2 documented residuals (below).
**Impact on plan:** No scope creep; every truth intact. Deviations 2-3 are layering realizations of the plan's intent, not semantic changes.

## Known Stubs / Residuals (recorded in WINDOWS.md + deferred-items.md)

1. **Timer-resume client mirroring:** the D-01 timer-driven resumed turn is fully transcript- and engine-recorded (live-proven leg 2) but its chunks are not mirrored to the ACP client (no active Run subscription at fire time); the reply path IS fully client-visible (live-proven leg 1). Natural home: 12-07's engine-driven-turn machinery.
2. **Pre-existing newline-chunk drop (NOT caused by this plan, surfaced by the witness):** the same transport guard silently drops the MODEL'S OWN newline-carrying text chunks on live streams (9 in the leg-1 session). Relaxing it reverses a documented v1.0 transport decision — operator disposition recorded in `.planning/phases/12-product-functional-completeness/deferred-items.md` + `.planning/WINDOWS.md`.

## Issues Encountered

The live witness caught finding #4 above — diagnosed by reproducing at server level (acp.Server over stdio pipes), bisecting bus-side vs wire-side with temporary instrumentation, and reconciling the 450-vs-440 frame counts to the exact drop set. The pre-existing model-chunk drop fell out of the same root-cause analysis.

## User Setup Required

None beyond the witness (consumed): ZAI_API_KEY resolved via the operator's global config.yaml per Phase 7 design; the ACP client was the operator's driver.

## Next Phase Readiness

- 12-01's suspension/resume seam is the substrate for 12-03 (plan-mode approval), 12-07 (cron firing reuses ArmTimeout + the engine-driven resume), 12-08 (eval ask scenarios), and 13's OS-02 (interactive dead-ends routed through AskUserQuestion).
- The D-01 timer machinery (armed at suspension, first-claimant-wins vs the reply) is the seed 12-07 reuses.
- Residuals are recorded and routed (WINDOWS.md); neither blocks the adoption line.

---
*Phase: 12-product-functional-completeness*
*Completed: 2026-08-19*

## Self-Check: PASS

