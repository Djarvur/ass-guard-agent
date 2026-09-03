---
phase: 21-context-policy-parity-closures
plan: "03"
subsystem: api
tags: [thinking, sse, streaming, transcript, projector, shaper, anthropic-sdk, agent-thought-chunk, par-05]

# Dependency graph
requires:
  - phase: 16-acp-wire-foundation
    provides: AppendRawThinking redactor-exempt transcript endpoint (16-02), EmitterHandle.ThoughtChunk + agent_thought_chunk frame kind (16-01), ActivityEmitter embedding discipline (16-D-20)
  - phase: 21-context-policy-parity-closures
    provides: "21-01/21-02 landed before this plan (wave ordering); no code dependency"
provides:
  - SSE thinking accumulator in drainSSE — signed thinking_delta/signature_delta blocks and redacted_thinking blocks emerge as typed StreamChunks with raw assembled bytes (PAR-05 hop 1+2)
  - AgentThoughtChunk bus event + session leg — thinking lands in the transcript verbatim (redactor-exempt, AppendRawThinking's first production caller) and streams live to the editor
  - Runtime forwarder wiring — both subscribe sites (per-Run + session-lifetime) carry agent_thought_chunk through the ActivityEmitter type-assert idiom
  - Projector thinking fold — raw_thinking lines fold INTO their turn's assistant unit (batch or final text), orphan thinking drops with its turn, boundary cuts take the whole turn unit (Pitfall 5)
  - Shaper mapping — ThinkingBlockParam AND RedactedThinkingBlockParam in original order; zero-thinking rendering byte-identical
  - D-14 golden battery — committed wire-pair fixtures proving field-value identity at every hop, plus the D-13 replay pin
affects: [18-session-family replay, 19-compaction-cache-control, par-05 verification, future thinking-enabled captures]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
# Same estimateTokens scale (chars/4 over the realized diff), never a harness token count.
actuals:
  tokens: 21000   # 84,396 diff chars / 4 over internal/ (19 files, +1655/-50)
  tasks: 3
  commits: 7      # 6 task commits (RED+GREEN per task) + this docs commit

# Tech tracking
tech-stack:
  added: []       # stdlib encoding/json + pinned anthropic-sdk-go v1.63.0 param types only
  patterns:
    - "Sibling-state SSE accumulator: the thinking tracker mirrors the tool-use lifecycle shape without touching it (flush at the same EOF/error/[DONE] call sites)"
    - "Field-value identity chain: raw delta strings assembled once at the provider, RawMessage stored verbatim, values extracted ONLY at the projector, SDK re-serializes (D-12/D-14)"
    - "Fold-into-assistant-unit ownership: thinking lives and dies with its turn's assistant message; flushBatch materializes it, orphans drop with the turn (never standalone)"

key-files:
  created:
    - internal/session/thinking_test.go
    - internal/session/testdata/thinking-golden/sse-thinking.jsonl
  modified:
    - internal/provider/streaming.go
    - internal/provider/provider.go
    - internal/provider/streaming_test.go
    - internal/provider/goconst_constants.go
    - internal/provider/goconst_constants_test.go
    - internal/event/events.go
    - internal/event/events_test.go
    - internal/session/session.go
    - internal/session/projector.go
    - internal/session/projector_test.go
    - internal/session/transcript_newkinds_test.go
    - internal/session/goconst_constants.go
    - internal/shaper/shaper.go
    - internal/shaper/shaper_test.go
    - internal/runtime/runtime.go
    - internal/runtime/cron_wiring.go
    - internal/runtime/emitter_e2e_test.go

key-decisions:
  - "Field-value identity, not envelope identity: assembled block JSON is built from the decoded delta strings via a small marshal view (never a typed round-trip of provider bytes); the projector extracts values, the SDK re-serializes — the flagged precision row pinned by the goldens"
  - "Redacted blocks append to the transcript but publish no AgentThoughtChunk (no thinking field value exists — nothing displayable); filtering or synthesizing a signature was never an option"
  - "Thinking blocks lead the assistant message in the shaper (both SDK param types in original order); the Message struct's flat slices cannot represent text-interleaved thinking, matching the provider's thinking-first canonical order"
  - "parseThinkingBlock skips typeless/unknown payloads tolerantly — production blocks always carry type (the accumulator emits it); hand-written typeless fixtures (the 16-02 inertness battery) therefore never folded, which exposed the stale tripwire"

patterns-established:
  - "Sibling lifecycle state in drainSSE: new content-block types join as sibling trackers with their own flush registered at the same termination call sites — never a refactor of the existing machine"
  - "Activation of a reserved transcript kind: the 16-02 inertness tripwire's own comment named Phase 21 as activation owner; activating the fold means REMOVING the kind from the inert set and updating the pin (foreign-turn placement keeps the leak guard)"

requirements-completed: [PAR-05]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "SSE thinking accumulator: signed blocks accumulate thinking_delta/signature_delta and emit on stop; redacted_thinking emits immediately; interleaving additive-only; flush-at-termination exactly once"
    requirement: PAR-05
    verification:
      - kind: unit
        ref: "internal/provider/streaming_test.go#TestStreamThinking_SignedBlockAccumulatesDeltas"
        status: pass
      - kind: unit
        ref: "internal/provider/streaming_test.go#TestStreamThinking_RedactedEmitsImmediately"
        status: pass
      - kind: unit
        ref: "internal/provider/streaming_test.go#TestStreamThinking_InterleavedWithTextAndToolUse"
        status: pass
      - kind: unit
        ref: "internal/provider/streaming_test.go#TestStreamThinking_FlushOnTerminationWithoutStop"
        status: pass
      - kind: unit
        ref: "internal/provider/streaming_test.go#TestStreamThinking_MultipleBlocksEmitInOrder"
        status: pass
      - kind: unit
        ref: "internal/provider/streaming_test.go#TestStreamThinking_SingleFlushOnCleanEnd"
        status: pass
    human_judgment: false
  - id: D2
    description: "Session leg: thinking chunks append verbatim via AppendRawThinking (redactor-exempt, zero invocations pinned) with model attribution, and publish AgentThoughtChunk carrying the display text; redacted appends without publishing"
    requirement: PAR-05
    verification:
      - kind: unit
        ref: "internal/session/thinking_test.go#TestStreamAndEmit_ThinkingAppendsRawAndPublishesThought"
        status: pass
      - kind: unit
        ref: "internal/session/thinking_test.go#TestStreamAndEmit_ThinkingRedactedAppendsWithoutPublish"
        status: pass
      - kind: unit
        ref: "internal/session/thinking_test.go#TestStreamAndEmit_ThinkingRedactorZeroCalls"
        status: pass
      - kind: unit
        ref: "internal/event/events_test.go#TestAgentThoughtChunkKind"
        status: pass
    human_judgment: false
  - id: D3
    description: "Live editor stream: agent_thought_chunk frames reach stdout through the bus -> forwarder -> ActivityEmitter chain, ordered before the turn's message chunks, at BOTH subscribe sites (per-Run + session-lifetime)"
    requirement: PAR-05
    verification:
      - kind: e2e
        ref: "internal/runtime/emitter_e2e_test.go#TestThoughtForward"
        status: pass
    human_judgment: false
  - id: D4
    description: "Projector fold: raw_thinking folds INTO the assistant batch (with tool_use, original order) or rides with the final assistant text; orphan thinking drops with its turn; mid-turn boundaries never split the turn unit"
    requirement: PAR-05
    verification:
      - kind: unit
        ref: "internal/session/projector_test.go#TestProjector_ThinkingFoldsIntoAssistantBatch"
        status: pass
      - kind: unit
        ref: "internal/session/projector_test.go#TestProjector_ThinkingAttachesToAssistantText"
        status: pass
      - kind: unit
        ref: "internal/session/projector_test.go#TestProjector_ThinkingOrphanDroppedWithTurn"
        status: pass
      - kind: unit
        ref: "internal/session/projector_test.go#TestProjector_ThinkingBoundaryAdjacent"
        status: pass
    human_judgment: false
  - id: D5
    description: "Shaper mapping: both SDK param types (ThinkingBlockParam{Signature,Thinking} / RedactedThinkingBlockParam{Data}) in original order; zero-thinking rendering byte-identical to the pre-change form"
    requirement: PAR-05
    verification:
      - kind: unit
        ref: "internal/shaper/shaper_test.go#TestShaperThinking_MapsBothBlockTypesInOrder"
        status: pass
      - kind: unit
        ref: "internal/shaper/shaper_test.go#TestShaperThinking_ZeroThinkingByteIdentical"
        status: pass
    human_judgment: false
  - id: D6
    description: "D-14 golden identity chain: committed wire-pair fixtures replay SSE -> StreamChunk -> transcript -> projector -> shaper with field-value identity at every hop (signed + redacted + boundary-adjacent), plus the D-13 replay pin (appended == re-read bytes)"
    requirement: PAR-05
    verification:
      - kind: integration
        ref: "internal/session/projector_test.go#TestThinkingGolden"
        status: pass
    human_judgment: false

# Metrics
duration: 55 min
completed: 2026-09-03
status: complete
---

# Phase 21 Plan 03: Thinking Pipeline (PAR-05) Summary

**Provider thinking blocks stream end-to-end — SSE deltas accumulate into raw StreamChunks, land in the transcript redactor-exempt, stream live as agent_thought_chunk frames, and round-trip into outgoing requests field-value-identical through golden wire-pair fixtures (both signed and redacted blocks)**

## Performance

- **Duration:** 55 min
- **Started:** 2026-09-03T18:58:54Z
- **Completed:** 2026-09-03T19:54:23Z
- **Tasks:** 3 (each RED -> GREEN; 6 task commits)
- **Files modified:** 19 (2 created, 17 modified)

## Accomplishments
- SSE thinking accumulator in drainSSE as SIBLING state beside the tool-use tracker — signed blocks (thinking_delta concatenation + signature_delta) emit one StreamChunk per block on content_block_stop, redacted_thinking emits immediately from content_block_start, and both flush at the exact EOF/error/[DONE] call sites flushToolUse uses (no double-emit, no behavior change to text/tool paths)
- The session leg gives Phase 16's two zero-caller endpoints their production callers: AppendRawThinking (transcript, redactor-exempt, model-attributed) and EmitterHandle.ThoughtChunk (live wire), wired through the new AgentThoughtChunk bus event at both runtime forwarder sites
- The projector folds thinking INTO its turn's assistant unit — batch or final text — with the orphan-drop rule and the whole-turn boundary guarantee pinned (Pitfall 5); the shaper maps both SDK param types in original order with zero-thinking rendering byte-identical
- The D-14 golden battery proves field-value identity at every hop off committed fixtures (signed + redacted + boundary-adjacent), with the D-13 replay pin riding hop 2

## Task Commits

Each task was committed atomically (TDD: failing test first):

1. **Task 1: SSE thinking accumulator** — `7d7c982` (test) + `eaecab0` (feat)
2. **Task 2: Session leg + live forward** — `34501d5` (test) + `e548ea2` (feat)
3. **Task 3: Projector + shaper + goldens** — `91bb59a` (test + fixture) + `95dda05` (feat)

**Plan metadata:** this commit (docs)

## Files Created/Modified
- `internal/provider/streaming.go` — thinking accumulator state + flushThinking/emitThinking/marshalThinkingBlock (raw delta strings, never a typed round-trip)
- `internal/provider/provider.go` — ThinkingBlock alias (the Message/ToolCall one-directional pattern)
- `internal/event/events.go` — AgentThoughtChunk event + BufAgentThoughtChunk = 32
- `internal/session/session.go` — streamAndEmit case chunkTypeThinking + thinkingDisplayText (display-only extraction)
- `internal/runtime/runtime.go`, `internal/runtime/cron_wiring.go` — both forwarder sites + routeBusEvent case + forwardThoughtChunk
- `internal/session/projector.go` — TypeRawThinking fold, pending-thinking stash, parseThinkingBlock (the only replay-path extraction site)
- `internal/shaper/shaper.go` — ThinkingBlock type, additive Message.ThinkingBlocks, both SDK param mappings
- `internal/session/testdata/thinking-golden/sse-thinking.jsonl` — the golden fixtures (corpus_absent, see Deviations)
- Tests: streaming_test.go, thinking_test.go (NEW), events_test.go, emitter_e2e_test.go, projector_test.go, shaper_test.go, transcript_newkinds_test.go (inertness pin updated)

## Decisions Made
- Field-value identity over envelope identity for the assembled block JSON (the flagged `precision` row): the accumulator builds `{"type","thinking","signature"[,"data"]}` from the decoded delta strings via a small marshal view; the projector extracts values untouched; the SDK re-serializes. The goldens pin VALUES at every hop, not envelope bytes.
- Redacted blocks publish no AgentThoughtChunk (no display text exists) but always append to the transcript — the storage path is unconditional, the display path is value-gated.
- parseThinkingBlock tolerantly skips typeless/unknown payloads: production blocks always carry type (the accumulator emits it), so the skip only affects hand-written non-production fixtures — this is what exposed the stale 16-02 inertness tripwire (see Deviations).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 16-02 inertness tripwire required updating for the activated contract**
- **Found during:** Task 3 (GREEN)
- **Issue:** `TestProjectorToleratesNewKinds` (transcript_newkinds_test.go) pinned raw_thinking as projection-inert with a hand-written TYPELESS payload (`{"thinking":"scratch"}`), so the activation initially went unnoticed (the typeless block never folded). The test's own comment names Phase 21 (PAR-05) as the activation owner — the pin existed precisely to trip this change.
- **Fix:** The turn_t raw_thinking interleave left the inert set (thinking is active fold territory, covered by the new TestProjector_Thinking* battery); the foreign-turn (turn_0) line now carries the kind's inertness pin with a TYPED payload, proving foreign-turn thinking never leaks into the current turn's window. Comments updated on both the test and the fixture builder.
- **Files modified:** internal/session/transcript_newkinds_test.go
- **Verification:** `go test -race ./internal/session/ -count=1` green (TestProjectorToleratesNewKinds + TestTranscriptNewKinds + the new thinking battery together)
- **Committed in:** 95dda05 (Task 3 commit)

**2. [Rule 1 - Bug] flushBatch cleared the thinking stash before the assistant-text case could read it**
- **Found during:** Task 3 (GREEN — RED caught it)
- **Issue:** The first GREEN run failed `TestProjector_ThinkingAttachesToAssistantText` and the golden redacted case: the TypeAssistantMessage case called flushBatch() first, whose cleanup nil'd pendingThinking before the text message could attach it.
- **Fix:** Capture the stash (`th := pendingThinking`) BEFORE flushBatch, attach `th` to the text message, reset after.
- **Files modified:** internal/session/projector.go
- **Verification:** Task 3 verify commands green (`TestThinkingGolden|TestProjector` + `TestShaperThinking`)
- **Committed in:** 95dda05 (Task 3 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking test-contract activation, 1 bug)
**Impact on plan:** Both were Task 3 mechanics, not scope changes. The plan's contracts (D-12/D-13/D-14, Pitfall 5) are implemented as written.

## Known Limitations

- **The golden fixtures are corpus_absent synthetic (A4 fallback, LOUD by design):** the corpus hunt found ZERO thinking-bearing SSE responses anywhere (profiles/zcode/ thinking.json is the empty thinking-CONFIG file; internal/profile sample sessions carry only thinking config blocks; .ass-guard audit bodies contain prose "thinking" mentions only; repo-wide grep for thinking_delta/signature_delta/redacted_thinking in captured data: no hits). A fresh capture via BuildWithCapturer requires a live provider with thinking enabled (not configured on this machine). Per the plan's flagged assumption A4, the fixtures are synthetic with shapes grounded in platform.claude.com's thinking docs and anthropic-sdk-go v1.63.0's wire forms; the fixture's provenance header records the hunt, and WINDOWS ledger entry #17 tracks the replacement with captured wire pairs. Field-value identity through the chain is fully test-proven regardless of fixture provenance.

## Issues Encountered
- `mise ci` red on two PRE-EXISTING causes, both worktree-proven at the pre-plan commit e7affc4: (1) the ledgered exhaustruct_v5 lint drift (WINDOWS #16) — a filtered run shows ZERO lint findings in every file this plan touched; (2) load-timing flakes in internal/runtime (TestAskPark_PromptResponsePrecedesResolution, TestAskWiring_ServerLevelSurface, plan-mode wiring), internal/session (TestGateOutcomeMatrix), internal/checkpoint — all pass in isolation and repeatedly standalone. Logged in deferred-items.md under 21-03. All plan-level verify commands, the five-package -race suite, vet, and the CGO_ENABLED=0 build are green.

## TDD Gate Compliance

All three tasks executed RED -> GREEN with per-phase commits:
- RED: `7d7c982`, `34501d5`, `91bb59a` (test commits; each verified failing first — Tasks 2/3 as build failures on the undefined new symbols, Task 1 as six failing assertions)
- GREEN: `eaecab0`, `e548ea2`, `95dda05` (each verified passing after)
- No REFACTOR phase needed (the GREEN shapes landed clean; no post-green cleanup commits)

## Self-Check: PASSED

- Created files exist: internal/session/thinking_test.go, internal/session/testdata/thinking-golden/sse-thinking.jsonl — FOUND
- All task commits exist in git log: 7d7c982, eaecab0, 34501d5, e548ea2, 91bb59a, 95dda05 — FOUND
- Plan `<verification>`: `go test -race ./internal/provider/ ./internal/session/ ./internal/shaper/ ./internal/event/ ./internal/runtime/ -count=1` — PASS (final run after the last task commit)
- `mise ci` — vet/build green, test green in 4/6 standalone full passes with the pre-existing load flakes documented above, lint red = ledgered pre-existing drift (zero findings in this plan's files)

## Next Phase Readiness
- PAR-05's five hops are live end-to-end; both Phase 16 endpoints (AppendRawThinking, ThoughtChunk) consumed; the thinking chain respects 19-D-04 (thinking lines replay verbatim — the projector extracts values, never rewrites storage)
- Phase 18's session/load replay can re-emit raw_thinking lines as real history through the same AgentThoughtChunk path (D-13's replay half)
- Open follow-up: replace the synthetic goldens with captured wire pairs when a thinking-enabled capture run lands (WINDOWS #17)

---
*Phase: 21-context-policy-parity-closures*
*Completed: 2026-09-03*
