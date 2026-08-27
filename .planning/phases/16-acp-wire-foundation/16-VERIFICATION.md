---
phase: 16-acp-wire-foundation
verified: 2026-08-27T23:59:00Z
status: gaps_found
score: 26/31 must-haves verified
behavior_unverified: 0
overrides_applied: 0
unverified_prohibitions: # ADR-550 D4 — judgment-tier items, NON-AUTHORITATIVE LLM-judge verdicts; human review recommended
  - statement: "Emitter must never synthesize/fabricate client frames (16-01, ACP-03 transparency)"
    verdict: pass_non_authoritative
    evidence: "All frame origins traced to bus events / registry cascade / surface notifications; TodoWrite plan derives from captured input; no synthesis path found in emitter.go/handlers.go"
  - statement: "Unredacted append path never extended beyond raw_thinking (16-02, ACP-03 privacy)"
    verdict: pass_non_authoritative
    evidence: "grep: appendLineUnredacted sole caller is AppendRawThinking (manager.go:350); counting-fake test pins zero Redact calls"
  - statement: "API keys/credentials never appear as editor config options (16-05, ACP-08 privacy)"
    verdict: pass_non_authoritative
    evidence: "Menu fixed to model/tier/permissions.mode/compaction-threshold + _global/ twins (config_surface.go optionsLocked); no credential key path exists in the surface"
  - statement: "_meta blob never overrides explicit operator file config (16-05, ACP-08 ownership)"
    verdict: pass_non_authoritative
    evidence: "fills-unset is in-memory overlay only; TestMetaBlob_ExplicitFileWins pins explicit-file-wins in both directions; blobFills never persisted"
gaps:
  - truth: "A turn with zero streamed activity emits zero session/update frames and its session/prompt response still arrives — the turn-end barrier returns immediately on an empty queue (16-01 truth 5)"
    status: failed
    reason: "CR-01 (confirmed in current code, not fixed post-review): TurnEmitter.Barrier uses a capacity-1 wake channel with single-token handoff (emitter.go:181,278,304-307). With two concurrent session/prompt turns on one connection (supported: per-request goroutine dispatch, server.go:378; one shared emitter per Server, server.go:182; Barrier receives the connection-lifetime Serve ctx, handlers.go:400), the final written frame's token wakes only ONE waiter; the other never receives again, its ctx never fires in production, and its prompt response hangs forever. No test covers two concurrent long-lived-ctx Barriers completing on the same final frame (TestTurnEmitterBarrier is single-waiter; the soak's barrier hammer uses short-lived ctxs that escape via ctx.Done — emitter_soak_test.go:371-388)."
    artifacts:
      - path: "internal/acp/emitter.go"
        issue: "Barrier lost-wakeup: capacity-1 wake channel drops the second concurrent waiter at terminal state (lines 181, 261-285, 297-308)"
    missing:
      - "Broadcast wake: swap-and-close the wake channel per generation under mu (or sync.Cond / registered waiter channels) so every concurrent Barrier re-checks after each written frame"
      - "Regression test: two Barriers whose targets are satisfied by the same final frame, both with connection-lifetime ctxs (no ctx.Done escape), both must return"
  - truth: "The session/prompt response is written only after all that turn's queued notification frames have been written (updates-before-response, the verified cancel contract) (16-01 truth 6)"
    status: failed
    reason: "Same root cause as CR-01: for the hung second concurrent turn, the response is never written at all — the ordering contract degrades into a liveness failure exactly on the concurrency axis the phase goal claims ('proven under concurrency')."
    artifacts:
      - path: "internal/acp/emitter.go"
        issue: "Barrier wake mechanism cannot progress two simultaneous waiters at queue-drain"
    missing:
      - "The CR-01 fix above; the existing single-turn ordering tests already pin the single-waiter case"
  - truth: "The global layer and project layer are independently addressable write targets (16-04 truth 3, D-08)"
    status: partial
    reason: "WR-05 (confirmed in current code): the idempotence guard in ConfigSurface.Set compares against the COMBINED (project-won) effective value regardless of scope (config_surface.go:182, effectiveFor :398-404). Set(\"_global/model\", X) where X equals the combined effective value returns 'idempotent re-push, no layer write' — the operator's explicitly global-scoped mutation is silently swallowed and never reaches the global layer. Differing-value writes work (TestScopeRouting_GlobalPrefixWritesGlobalLayer passes)."
    artifacts:
      - path: "internal/acpserve/config_surface.go"
        issue: "Scope-blind idempotence guard intercepts global-scope writes whose value equals the combined effective value (lines 182-191)"
    missing:
      - "Scope-aware idempotence: compare against the ADDRESSED layer's current value (load the global layer through modelrouting.Load(s.globalPath)) before declaring a re-push idempotent"
      - "Test: _global/ write whose value equals the combined effective but differs from the global layer's value must persist to the global layer"
  - truth: "Advertisement carries an EFFECTIVE currentValue resolved through the precedence chain for every menu entry (16-05 truth 1; ROADMAP criterion 4 'true current values')"
    status: partial
    reason: "Two confirmed divergences. (a) WR-05 part 1: the _global/model and _global/tier twins are built with res.model/res.tier — the PROJECT-won combined values (config_surface.go:518-521) — so the editor renders the project's value under 'Model (global default)'. (b) Pre-stamp Model chip finding (operator-confirmed 2026-08-27, routed to this verifier): the runner's wire model defaults to the profile slug (runtime.go effectiveModel \"\" = profile model, mimicry parity) while the chip advertises the tier-resolved config value (config_surface.go resolveModelLocked); live evidence: turn-001 went out GLM-5.3 while the chip showed glm-5.2. Checked later phases (Step 9b): Phase 20's /model covers session-scope command routing, NOT the chip's default-state truth — no later-phase owner exists. Post-switch the values converge; divergence appears only when project tier model ≠ profile slug."
    artifacts:
      - path: "internal/acpserve/config_surface.go"
        issue: "_global twins advertise combined values (lines 518-521); model advertisement resolves config-layer value, not the runner's actual request model"
      - path: "internal/runtime/runtime.go"
        issue: "effectiveModel \"\" default sends the profile slug while the ACP-08 menu advertises the tier-resolved model (mimicry-parity vs chip-truth tension)"
    missing:
      - "Resolve the global twins' currentValue from the global layer alone"
      - "Decide and implement chip truthfulness: either stamp the advertisement with the runner's effective request model, or make the runner's default follow the tier resolution (one of the two sources of truth must win) — OR accept via a recorded override with the operator's disposition as rationale"
  - truth: "session/set_config_option persists first then applies live; an inbound value equal to the option's currently-effective value is a logged idempotent no-op (16-05 truth 2)"
    status: partial
    reason: "The persist-then-apply ordering, typed rejections, and idempotence guard all exist and are tested — but the idempotence rule is implemented with a scope-blind effective-value basis (see 16-04 truth 3 gap): for _global/-scoped ids, 'currently-effective' resolves to the combined value, so legitimate global-layer writes can be classified as redundant re-pushes and silently dropped. The must-have clause is implemented with the wrong comparison basis for the global scope."
    artifacts:
      - path: "internal/acpserve/config_surface.go"
        issue: "Idempotence guard basis does not distinguish addressed layer vs combined resolution (lines 182-191)"
    missing:
      - "Same scope-aware comparison as the 16-04 truth 3 gap (single fix, one root cause)"
deferred: # Items addressed in later phases — not actionable gaps
  - truth: "load/resume responses carry the richer capability set incl. configOptions advertisement (ROADMAP criterion 4 legs beyond initialize/new)"
    addressed_in: "Phase 18"
    evidence: "Phase 18 goal: 'list / load-resume / close-delete with full replay and live-state reconciliation'; 16-05 shipped configOptionsFor as the shared builder explicitly reusable by load/resume (16-05-SUMMARY: 'Phase 18 (load/resume reuse configOptionsFor)'); loadSession honestly false until replay lands"
  - truth: "agent_thought_chunk streams from a live provider source during real turns (ROADMAP criterion 1 leg)"
    addressed_in: "Phase 21"
    evidence: "PAR-05 owns the provider thinking source; 16-01 shipped + unit-proved the wire shape ahead of it; the operator checkpoint explicitly excluded live thought-chunk rendering ('its absence is not failure')"
re_verification: null
---

# Phase 16: ACP Wire Foundation — Verification Report

**Phase Goal:** The three primitives every interactive phase depends on exist and are proven under concurrency: outbound id'd JSON-RPC requests with a pending-response registry, one ordered inline TurnEmitter owning all client frames with explicit backpressure, and the extended transcript line types (raw-thinking passthrough as `json.RawMessage`, `local_command`, compaction marker) that later phases' schemas lock here.
**Verified:** 2026-08-27T23:59:00Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

Two of the three primitives (registry, transcript kinds) are verified end to end including concurrency proofs. The TurnEmitter's ordering/backpressure invariants are real and heavily tested — but the turn-end Barrier contains a code-confirmed lost-wakeup defect (CR-01 from 16-REVIEW.md, still present in current code) that hangs the second of two concurrent `session/prompt` turns forever. The phase goal's "proven under concurrency" clause is therefore not met for the emitter primitive: every shipped test covers either a single Barrier waiter or Barriers that escape via short-lived ctxs, and the concurrent-waiter terminal state is broken.

### Observable Truths

Roadmap contract (SCs):

| #  | Truth (ROADMAP SC)                                                                                                                                    | Status     | Evidence |
|----|------------------------------------------------------------------------------------------------------------------------------------------------------|------------|----------|
| 1  | Live turn activity streams in real time (tool_call/tool_call_update, plan-from-TodoWrite, agent_thought_chunk), never a black-box spinner              | ✓ VERIFIED | Wire tests (TestPlanFrameFromTodoWrite, TestToolCallUpdateFrame, TestThoughtChunkFrame), simulator stages 2-3, OPERATOR-CONFIRMED items 1-3; thought-chunk live source deferred to Phase 21 (PAR-05) by plan contract, shape shipped + unit-proven |
| 2  | Concurrent emitters arrive in one consistent order; multi-emitter -race stress proves no reordering / no dropped frames; policy documented (fg first)  | ✓ VERIFIED | TestTurnEmitterPriority PASS (ran); adversarial soak re-run by verifier: 2m0s, 4,051,238 frames, 155 stall episodes, all five invariants PASS; fg-priority documented at emitter.go:310-335 |
| 3  | Id'd request matched by id while notifications stream; registry resolves without the writer mutex; unanswered degrades loudly                          | ✓ VERIFIED | TestRegistryConcurrentResolve / TestRegistryTimeoutFallback / TestRegistrySyntheticCancel PASS (ran); resolution under registry's own mutex code-verified (request_registry.go:134-270) |
| 4  | Initialize/new/load/resume carry the richer capability set, verified against the ACP schema in a real Zed handshake                                    | ⚠️ partial | initialize/new verified (simulator stage 1, TestInitializeProbe/TestCapabilityStickiness exist+green, OPERATOR-CONFIRMED item 4) — but "true current values" diverges pre-stamp (chip vs wire model) and the _global twins display combined values → gap 4; load/resume legs → Phase 18 (deferred) |
| 5  | Transcript lines raw_thinking / local_command / compaction append-only, redaction excluded by construction for thinking bytes                          | ✓ VERIFIED | TestTranscriptNewKinds + TestProjectorToleratesNewKinds PASS (ran); appendLineUnredacted sole caller grep-verified (manager.go:350) |

Plan-level truths (31 total across 16-01..16-06): 26 VERIFIED (incl. the soak truth, re-proven by this verifier's own run, and the operator-disposition truth: anchored OPERATOR-CONFIRMED marker grep = 1), 2 FAILED, 3 PARTIAL — the failures/partials are the five gaps in the frontmatter above.

**Score:** 26/31 truths verified (0 present-but-behavior-unverified; the two behavior-dependent barrier truths FAILED on code-provable defects, the rest carry passing behavioral tests)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/acp/emitter.go` | TurnEmitter: bounded lanes, nested-select drain, stall detector, barrier | ✓ VERIFIED (defect: Barrier wake) | 619 lines; all symbols present; Barrier wake mechanism defective under 2 waiters (CR-01) |
| `internal/acp/types.go` | v1 frame structs, verbatim camelCase | ✓ VERIFIED | ToolCallFrame :97, ToolCallUpdateFrame :112, DiffContent :139, ToolCallLocation :153, PlanFrame :160, ThoughtChunkFrame :176, ConfigOptionFrame :226; no `plan_update`; `configId` is v1-verbatim for the SET REQUEST (schema-pinned) |
| `internal/acp/request_registry.go` | Registry: Call/Deliver/ResolveCancelled/Stop, ladder, cascade | ✓ VERIFIED | 487 lines; WIRED into Serve (interception server.go:361-365) + cascade via emitter fg lane |
| `internal/acp/metrics.go` | Counter family + Snapshot() | ✓ VERIFIED | Metrics :13, Snapshot :32; adopted stall counter (WR-01 race noted below) |
| `internal/session/transcript.go` + `manager.go` | 3 kinds + unredacted path + appenders | ✓ VERIFIED | TypeRawThinking :58, TypeLocalCommand :66, TypeCompaction :76; appendLineUnredacted :113, sole caller AppendRawThinking :350 |
| `internal/session/transcript_newkinds_test.go` | byte-identity, zero-redactor, tolerance tests | ✓ VERIFIED | TestTranscriptNewKinds/TestProjectorToleratesNewKinds PASS |
| `internal/providerfactory/config_write.go` | WriteLayerOption, typed errors, atomic 0600 | ✓ VERIFIED | LayerReadError/LayerWriteError, DeepMerge reuse; TestConfigWrite family PASS |
| `internal/modelrouting/config.go` | SessionTier yaml key defaulted heavy | ✓ VERIFIED | `session_tier` :23; TestSessionTier PASS |
| `internal/acp/handlers.go` + `server.go` | handleSetConfigOption, advertisement, WithConfigSurface | ✓ VERIFIED | Handler registered under v1 name; WithConfigSurface option; no-surface degrade |
| `internal/acpserve/config_surface.go` | menu, effective values, scope routing, blob, live apply | ✓ VERIFIED (defects: WR-05) | 681 lines; all seams wired; _global twins + idempotence basis defective (gaps 3-5) |
| `internal/runtime/runtime.go` + `session/session.go` | ApplyTurnModel + SetTurnModel seam | ✓ VERIFIED | Subscriptions :509-514; effectiveModel + turn-mutex serialization; TestLiveModelApply/TestTierSwitch/TestApplyTurnModel PASS |
| `internal/acpserve/simulator_e2e_test.go` | TestZedSimulatorE2E whole-surface story | ✓ VERIFIED | PASS under -race in verifier run (6.2s) |
| `internal/acp/emitter_soak_test.go` + `.mise.toml` | env-gated soak + mise task outside ci | ✓ VERIFIED | PASS in verifier run; `emitter-soak` task at .mise.toml:49-51, absent from ci chain |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| acpserve/acp_serve.go | acp/emitter.go | TurnEmitter construction + SetEmitter (:204-239) | ✓ WIRED | runner.SetEmitter(srv.Emitter); emitter armed in NewServer |
| runtime/runtime.go | acp/emitter.go | ActivityEmitter forwarders | ✓ WIRED | ToolCall/ToolCallUpdate subscriptions (:509-514), routeBusEvent dispatch |
| acp/server.go | request_registry.go | Serve interception before dispatch | ✓ WIRED | id+no-method+result-or-error → Deliver (:361-365) |
| request_registry.go | emitter.go | $/cancel_request cascade via fg lane | ✓ WIRED | WithRegistryCascade(s.emitter...Notify) (server.go:191) |
| acp/handlers.go | acpserve/config_surface.go | ConfigSurface interface via WithConfigSurface | ✓ WIRED | acpserve :211-217 injects; acp stays config-package-free |
| config_surface.go | providerfactory/config_write.go | WriteLayerOption per scope | ✓ WIRED | persistLocked → WriteLayerOption (defective basis upstream: WR-05) |
| config_surface.go | runtime/runtime.go | applyLocked → Runner.ApplyTurnModel | ✓ WIRED | Live apply proven by TestLiveModelApply (WR-04 lock-scope caveat) |
| simulator_e2e_test.go | acpserve/acp_serve.go | real Run over pipes | ✓ WIRED | Verifier-run PASS; five stages green |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Whole-surface story (5 stages) | `go test -race ./internal/acpserve/ -run TestZedSimulatorE2E -count=1` | ok 6.2s | ✓ PASS |
| Priority/no-drop + barrier + registry core | `go test -race ./internal/acp/ -run 'TestTurnEmitterPriority\|TestTurnEmitterBarrier\|TestRegistryConcurrentResolve\|TestRegistryTimeoutFallback\|TestRegistrySyntheticCancel'` | ok 2.0s | ✓ PASS |
| Transcript kinds + projector tolerance | `go test ./internal/session/ -run 'TestTranscriptNewKinds\|TestProjectorToleratesNewKinds'` | ok 0.8s | ✓ PASS |
| Config writer round-trip + session_tier | `go test ./internal/providerfactory/ ./internal/modelrouting/ -run 'TestConfigWrite\|TestSessionTier'` | ok | ✓ PASS |
| Live apply + tier switch (incl. mid-turn) | `go test -race ./internal/acpserve/ -run 'TestLiveModelApply\|TestTierSwitch'` | ok 14.6s | ✓ PASS |
| Two concurrent long-lived-ctx Barriers | (no such test exists — CR-01 hole) | code analysis: second waiter hangs at terminal state | ✗ FAIL (gap) |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| `internal/acp/emitter_soak_test.go` (declared in 16-06 PLAN) | `ASSGUARD_EMITTER_SOAK=1 go test ./internal/acp/ -run TestTurnEmitterSoak -count=1 -timeout 10m` | `SOAK RESULT: duration=2m0s producers=8 frames=4051238 stall_episodes=155 invariants=[no-drop,fifo,no-leak,stall-fired,clean-close] all=PASS` (153s) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| ACP-03 | 16-01, 16-02, 16-03, 16-06 | Zed renders live turn activity through one ordered inline TurnEmitter with explicit backpressure | ✓ SATISFIED (with critical defect) | Wire tests + simulator + OPERATOR-CONFIRMED items 1-3; CR-01 barrier hang is a concurrency defect against the "one ordered TurnEmitter... proven under concurrency" phase clause — recorded as gap 1-2 |
| ACP-08 | 16-04, 16-05, 16-06 | Editor drives configuration: configOptions advertised, set_config_option handled, API keys env/file only | ✓ SATISFIED (with partial defects) | Advertisement + set + live apply operator-confirmed (items 4-5, PASS); _global twin display + idempotence scope + pre-stamp chip truthfulness recorded as gaps 3-5 |

Orphaned requirements: none — REQUIREMENTS.md maps exactly ACP-03 and ACP-08 to Phase 16 (traceability table :106-107), both claimed by plan frontmatter.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| internal/acp/emitter.go | 181, 261-285, 304-308 | CR-01 Barrier lost-wakeup (capacity-1 wake, single-token handoff) | 🛑 Blocker | Second concurrent prompt response hangs forever |
| internal/acpserve/config_surface.go | 182-191, 398-404, 518-521 | WR-05 scope-blind idempotence + combined-value global twins | ⚠️ Warning | Global-scoped writes silently swallowed; mislabeled display |
| internal/acp/server.go vs emitter.go | 183 / 145,384 | WR-01 metrics field written after sampler goroutine start (unsynchronized) | ⚠️ Warning | Data race per Go memory model; bounded impact (missed stall count) |
| internal/runtime/cron_wiring.go | 249, 269-320 | WR-02 fanInEvents teardown contract false (Unsubscribe never closes channels; Bus.Close never called) | ⚠️ Warning | ~5 goroutines leak per closed session in long-lived processes; comment documents a mechanism that does not exist |
| internal/acp/request_registry.go | 387-401 vs 319-348 | WR-03 send TOCTOU vs Writer.Close (guarded today only by handlerWG.Wait-before-Stop convention) | ⚠️ Warning | Latent send-on-closed-channel panic for the first non-handler caller (Phase 17 background asks) |
| internal/acpserve/config_surface.go | 149-207, 435-464 | WR-04 Set holds s.mu across the live-apply hook | ⚠️ Warning | One mid-turn Set blocks all config operations (Options/Sets/blob) until the turn ends; serialization correctness itself holds |
| internal/acp/handlers.go | 489, 13,530 | IN-01 dead redact call pinning an import; IN-02 `asciiDelete` misnames UUID version bits | ℹ️ Info | Cosmetic/robustness |
| internal/session/manager.go | 106-128 | IN-03 "byte-identical / never re-serialized" comment overclaims (json.Marshal compacts RawMessage, HTML-escapes) | ℹ️ Info | On-disk contract wording; D-20 is one-way — reword before later phases lock against it |
| internal/acp/server.go | 361-365 | IN-04 malformed response (neither result nor error) falls through to a spurious -32601 | ℹ️ Info | Low likelihood (Zed-controlled side) |
| internal/acpserve/config_surface.go | 126-134 | IN-05 SetNotify/SetApplyHook write fields without s.mu | ℹ️ Info | Safe only under wired-before-serve convention |

Debt markers (TBD/FIXME/XXX): none in any phase-modified file. Stub patterns: none. No orphaned or placeholder artifacts.

### Human Verification Required

The phase's designated human gate (16-06 Task 3, blocking) was dispositioned **OPERATOR-CONFIRMED** (2026-08-27, operator-delegated, machine-verified; anchored marker grep = 1): all five live-Zed items PASS, with the pre-stamp Model-chip finding recorded and routed to this verifier (now gap 4). Residual human decisions ride the gaps, not a UAT list: the CR-01 fix (must-fix vs override with a documented single-concurrent-prompt limitation), and the chip truthfulness direction (advertisement follows runner vs runner follows tier resolution).

### Gaps Summary

Five gaps, three root causes, all confirmed by direct code reading in the current tree (the 16-REVIEW.md findings were re-verified against the code, not accepted on the reviewer's word):

1. **CR-01 Barrier lost-wakeup (BLOCKER, failed truths 16-01 #5 and #6).** Every test that passes today avoids the broken state by construction: single Barrier waiter, or short-lived ctxs escaping via ctx.Done. Production passes the connection-lifetime Serve ctx, so the second of two concurrent session/prompt turns hangs with no escape. This directly contradicts the phase goal's "proven under concurrency" for the emitter primitive. Fix is small (generation close/cond broadcast) and must ship with the two-waiter regression test.
2. **WR-05 scope semantics in the ACP-08 surface (partial, truths 16-04 #3, 16-05 #1, 16-05 #2).** One root cause: the effective-value basis used for both the _global twin display and the idempotence guard is the combined project-won resolution rather than the addressed layer's value.
3. **Pre-stamp Model chip truthfulness (partial, against ROADMAP criterion 4).** Code-confirmed, operator-observed live (turn-001 GLM-5.3 on wire vs glm-5.2 chip), and — per the Step 9b scan — owned by no later phase (Phase 20's /model is session-scope command routing). Route: gap-closure candidate; alternatively accept via a recorded override carrying the operator's OPERATOR-CONFIRMED disposition as rationale.

Everything else the phase claims holds under independent re-run: registry concurrency and degradation ladder, transcript schema lock with provable redaction exemption, config writer atomicity and round-trip, live model/tier apply with turn serialization, the whole-surface simulator story, and the 2-minute adversarial soak (4.05M frames, all invariants PASS in this verifier's own process).

---

_Verified: 2026-08-27T23:59:00Z_
_Verifier: Claude (gsd-verifier)_
