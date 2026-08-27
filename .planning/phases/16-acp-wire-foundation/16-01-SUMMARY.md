---
phase: 16-acp-wire-foundation
plan: 01
subsystem: acp-wire
tags: [acp, emitter, backpressure, ordering, tdd, tracer]
requires:
  - "Phase 15 carved runtime (emitFor seam, SetEmitter)"
  - "internal/acp Writer (framer.go) — the final serialization chokepoint"
provides:
  - "TurnEmitter: one ordered bounded emitter owning ALL session/update notifications (fg preempts at head, bg FIFO, block-never-drop)"
  - "ActivityEmitter interface (ChunkEmitter + ToolCall/ToolCallUpdate/PlanUpdate/ThoughtChunk) + EmitterHandle fg/bg classes"
  - "v1 wire frame types: ToolCallFrame, ToolCallUpdateFrame, ToolCallContent, DiffContent, ToolCallLocation, PlanFrame, PlanEntry, ThoughtChunkFrame"
  - "Turn-end Barrier (updates-before-response) + D-03 stall detector (log + counter) + Server.BackgroundEmitter"
affects:
  - "16-02 registry (attaches to the same Writer; emitter stays the only notification producer)"
  - "16-03/16-05 (background emitters, config surface) — attach to the emitter spine without touching the ordering invariant"
  - "Phases 17-23 (interactive asks, sessions, compaction ride this frame path)"
tech-stack:
  added: [] # stdlib only (sync, sync/atomic, time, context, encoding/json)
  patterns:
    - "nested-select priority drain (fg-first, equal chance when fg empty)"
    - "separate stall-sampler goroutine (drain wedges inside sink.Write on slow clients)"
    - "mutex-guarded enqueue/written counter pair driving a Barrier (no lock spans an enqueue)"
    - "presentation rule in the wire layer (TodoWrite → plan frame; runtime stays ACP-word-free)"
key-files:
  created:
    - internal/acp/emitter.go
    - internal/acp/emitter_test.go
    - internal/runtime/emitter_e2e_test.go
  modified:
    - internal/acp/types.go
    - internal/acp/server.go
    - internal/acp/handlers.go
    - internal/acpserve/acp_serve.go
    - internal/runtime/runtime.go
    - internal/runtime/cron_wiring.go
key-decisions:
  - "TurnEmitter is armed by NewServer itself; the composition root tunes knobs via WithTurnEmitter at the acp_serve junction — every notification path shares one drain from the first frame (single-producer invariant holds unconditionally)"
  - "ActivityEmitter growth via embedding, ChunkEmitter untouched — existing test fakes across the repo keep compiling (RESEARCH Open Question 1 resolution)"
  - "Lane capacities fg 128 / bg 256, stall threshold 5s (CONTEXT-discretion defaults; invariant tests are capacity- and threshold-independent)"
  - "Stall sampler lives in its own goroutine: the drain blocks INSIDE sink.Write on a slow client, so in-drain sampling would go silent exactly when a stall is real (anti-D-03)"
  - "TodoWrite→plan presentation rule applied in the acp layer (EmitterHandle.ToolCall); runtime.go gained zero plan/todo vocabulary (15-D-20)"
requirements-completed: [ACP-03]
duration: 59 min
completed: 2026-08-27
estimate:
  tokens: 48000
actuals:
  tokens: 26000
  tasks: 3
  commits: 5
coverage:
  - deliverable: "One frame path end-to-end: bus ToolCall → runtime forwarder → TurnEmitter fg lane → single drain → Writer → stdout, in emission order, written-count == observed frames"
    verification:
      - kind: test
        ref: "tests/internal/runtime/emitter_e2e_test.go#TestTurnEmitterEndToEnd"
        status: pass
    human_judgment: false
  - deliverable: "Backpressure invariants (D-01/D-02): fg preempts the queue head over pre-filled bg lane, bg is FIFO, nothing dropped under a fake slow writer (-race)"
    verification:
      - kind: test
        ref: "tests/internal/acp/emitter_test.go#TestTurnEmitterPriority"
        status: pass
    human_judgment: false
  - deliverable: "Loud stall detection (D-03): one structured stderr line + one counter increment per sustained-full episode; blocked producers unblock on resume; ctx-aware producer abort"
    verification:
      - kind: test
        ref: "tests/internal/acp/emitter_test.go#TestTurnEmitterStall"
        status: pass
      - kind: test
        ref: "tests/internal/acp/emitter_test.go#TestTurnEmitterProducerCtxAbort"
        status: pass
    human_judgment: false
  - deliverable: "Turn-end barrier (updates-before-response) incl. zero-activity turns"
    verification:
      - kind: test
        ref: "tests/internal/acp/emitter_test.go#TestTurnEmitterBarrier"
        status: pass
      - kind: test
        ref: "tests/internal/acp/emitter_test.go#TestTurnEmitterEmptyTurn"
        status: pass
    human_judgment: false
  - deliverable: "Frame surface: plan-from-TodoWrite (no card beside the plan), agent_thought_chunk shape, tool_call_update diff/locations/kind/status under verbatim v1 camelCase names"
    verification:
      - kind: test
        ref: "tests/internal/acp/emitter_test.go#TestPlanFrameFromTodoWrite"
        status: pass
      - kind: test
        ref: "tests/internal/acp/emitter_test.go#TestThoughtChunkFrame"
        status: pass
      - kind: test
        ref: "tests/internal/acp/emitter_test.go#TestToolCallUpdateFrame"
        status: pass
    human_judgment: false
  - deliverable: "Live Zed rendering of tool_call/plan/thought frames (ACP-03 criterion leg)"
    human_judgment: true
    rationale: "Live-editor verification is operator-side per the 15-07 live-check pattern (WINDOWS ledger); the deterministic wire-shape proof is the in-repo test set above."
---

# Phase 16 Plan 01: ACP Wire Foundation — Ordered TurnEmitter Tracer Summary

One ordered TurnEmitter now owns every client-visible session/update frame — fg preempts at the head, bg stays FIFO, bounded lanes block-never-drop, stalls are loud, and the prompt response cannot overtake its turn's frames — proven end-to-end from a bus ToolCall event to stdout under `-race`.

## Accomplishments

- **Tracer (Task 1):** ONE frame path live end-to-end — bus ToolCall event published during a real Run turn flows through the runtime forwarder (ToolCall/ToolCallUpdate subscriptions beside AgentMessageChunk) into the TurnEmitter's foreground lane, out the single nested-select drain (fg-first always; equal chance when fg empty), into the existing Writer, and reaches stdout as a v1 `tool_call` frame — with `agent_message_chunk` frames around it in exact emission order and the emitter's written-count equal to the observed notification frames (sole-producer invariant).
- **Invariants (Task 2):** bounded lanes (fg 128 / bg 256) block producers with ctx awareness — nothing ever dropped; a late foreground frame is written BEFORE 100 queued background frames under a fake slow writer; bg is exactly FIFO; a lane held full past the threshold logs EXACTLY ONE structured stderr line + one counter increment per episode (sampler in its own goroutine — the drain wedges inside sink.Write on slow clients); the prompt handler barriers before returning, so the response lands strictly after every queued turn frame; empty turns respond immediately with zero notification frames.
- **Frame surface (Task 3):** TodoWrite calls render as v1 full-replacement `plan` frames (entries re-typed verbatim from the captured {todos:[…]} shape — no card beside the plan; malformed input falls back to the card); `agent_thought_chunk` (messageId + ContentBlock, v1 ContentChunk shape) unit-proven ahead of Phase 21's PAR-05 source; `tool_call_update` carries kind/status + diff (path/oldText/newText) + locations (absolute path + optional line) under verbatim camelCase v1 names; `grep -c 'plan_update|configId' types.go` = 0.
- **D-04 gate:** the -race stress rides the standing `mise ci` (37 packages green at close; whole file adds ~2s).
- **TDD gates:** RED commits `0c90200` (tracer e2e, build-failure RED — the emitter API did not exist) and `c3da730` (plan-from-TodoWrite, behavioral RED: `sessionUpdate = tool_call; want plan`) each precede their GREEN commits. Two Task-3 tests (ThoughtChunk/UpdateFrame) passed at RED by design — they pin surface that landed early as the ActivityEmitter compile surface; noted here for the gate review.

## TDD Gate Compliance

| Plan | RED | GREEN | REFACTOR | Status |
|------|-----|-------|----------|--------|
| 16-01 | ✓ `0c90200`, `c3da730` | ✓ `cf41c63`, `5424720`, `11a86c3` | — (no refactor needed) | Pass |

Note: the tracer RED fails as a Go build failure (new API absent) — the idiomatic expression of "emitter does not exist"; the Task-3 RED fails behaviorally. Task-3's two passing-at-RED tests are shape-pins on intentionally early-landed surface, documented above.

## Deviations from Plan

**1. [Rule 1 - Bug] Turn-end Barrier pulled forward into Task 1's commit**
- **Found during:** Task 1 GREEN (existing test `TestSessionPromptStreamsUpdate` failed)
- **Issue:** the old adapter wrote synchronously during Run, so the response could never overtake a chunk; the async emitter lane made response-overtakes-frames possible — Pitfall 4, caught red-handed by the existing suite's green gate (Task 1 acceptance criterion 6).
- **Fix:** `handleSessionPrompt` calls `s.emitter.Barrier(ctx)` immediately after `turnRunner.Run` returns (the Task-2 contract point, one commit early).
- **Files modified:** internal/acp/handlers.go
- **Verification:** acp suite ×3 + targeted -race ×5 green
- **Commit:** cf41c63

**2. [Rule 1 - Bug] Stall sampler moved out of the drain goroutine**
- **Found during:** Task 2 test design
- **Issue:** sampling inside the drain's select stops exactly when a stall is real — the drain blocks INSIDE sink.Write on a wedged client, so lanes could fill with no observation (silent blocking, the anti-D-03).
- **Fix:** dedicated `sampleLoop` goroutine (own ticker, root-ctx lifetime, covered by the emitter WaitGroup).
- **Files modified:** internal/acp/emitter.go
- **Verification:** TestTurnEmitterStall proves one-line-per-episode while the drain is wedged mid-write
- **Commit:** 5424720

**3. [Rule 1 - Bug] Tracer fixture re-served tool_use on every stream call**
- **Found during:** Task 2's `mise ci` run
- **Issue:** the session tool loop re-streams while a response carries tool calls, so the mock drove the 64-iteration runaway bound — squeaking under the read deadline standalone, over the line under full-CI load (emission pattern repeated ~64×, response timeout).
- **Fix:** the provider serves the tool phase exactly once, then an empty stream so the turn terminates deterministically after the tool pass.
- **Files modified:** internal/runtime/emitter_e2e_test.go
- **Verification:** e2e test green standalone + full `mise ci` green
- **Commit:** 5424720

**4. [Plan-structure note] Frame types landed in Task 1's commit**
PlanFrame/PlanEntry/ThoughtChunkFrame/DiffContent are compile-time dependencies of the Task-1 ActivityEmitter interface, so their struct definitions landed with cf41c63; Task 3 then delivered the behavior (mapping + methods) and the shape-pinning tests. Same file set as planned; ordering within the plan adjusted.

**Total deviations:** 3 auto-fixed (all Rule 1) + 1 structural note. **Impact:** low — all within the plan's own file set; the barrier pull-forward was required by the plan's own no-regression acceptance gate.

## Issues Encountered

- **Pre-existing flake logged, NOT fixed (scope boundary):** `TestAskPark_PromptResponsePrecedesResolution` (internal/runtime, Phase 12) tripped its 10s "near-instant" bound at 11.6s during one fully parallel `mise ci` run (scheduling delay under load; the distinguishing alternative — waiting for the ask resolution — is a 1-hour timer away). Passes standalone and in subsequent full-CI runs. Recorded in the phase deferred-items ledger for the verifier's awareness.

## Authentication Gates

None.

## Known Stubs

- `EmitterHandle.ThoughtChunk` is implemented and wire-proven at the unit level, but no live provider thinking source feeds it until Phase 21 (PAR-05) — intentional per the plan's own behavior contract; nothing is fabricated in the meantime (ACP-03 transparency prohibition).

## Verification Results

- `go test -race ./internal/acp/ ./internal/runtime/ ./internal/acpserve/ -count=1` — green (final run: 2.05s / 31.3s / 6.25s)
- `mise ci` (vet + golangci-lint strict + build + `go test -race ./...`) — green, 37 packages, exit 0
- `grep -c 'plan_update\|configId' internal/acp/types.go` — 0
- runtime.go gained no plan/todo vocabulary (diff-audited)

## Self-Check: PASSED

- internal/acp/emitter.go, internal/acp/emitter_test.go, internal/runtime/emitter_e2e_test.go exist on disk ✓
- Commits 0c90200, cf41c63, 5424720, c3da730, 11a86c3 present in git log ✓
- All task acceptance criteria re-run and passing; plan-level `<verification>` commands green ✓

## Next

Ready for 16-02 (outbound id'd requests + pending-response registry — attaches to the same Writer; the emitter's ordering invariant is untouched).
