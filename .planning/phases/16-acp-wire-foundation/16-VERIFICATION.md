---
phase: 16-acp-wire-foundation
verified: 2026-08-28T14:52:04Z
status: human_needed
score: 31/31 must-haves verified
behavior_unverified: 0
overrides_applied: 0
overrides: []
unverified_prohibitions: # ADR-550 D4 — judgment-tier items, NON-AUTHORITATIVE LLM-judge verdicts re-checked in the current tree; human review recommended
  - statement: "Emitter must never synthesize/fabricate client frames (16-01, ACP-03 transparency)"
    verdict: pass_non_authoritative
    evidence: "Re-checked: emitter.go last touched by 16-07 GREEN (4fddf9f, wake mechanism only); all frame origins still bus events / registry cascade / surface notifications; no synthesis path"
  - statement: "Unredacted append path never extended beyond raw_thinking (16-02, ACP-03 privacy)"
    verdict: pass_non_authoritative
    evidence: "Re-grepped: appendLineUnredacted sole caller is AppendRawThinking (manager.go:350); D-23 comment intact at :106"
  - statement: "API keys/credentials never appear as editor config options (16-05, ACP-08 privacy)"
    verdict: pass_non_authoritative
    evidence: "Re-grepped: no credential key path in config_surface.go; menu comment pins 'no credential options'; 16-08/CR-02 changes added no new option vocabulary"
  - statement: "_meta blob never overrides explicit operator file config (16-05, ACP-08 ownership)"
    verdict: pass_non_authoritative
    evidence: "Re-checked in current tree: fills-unset still deletes the fill after explicit persist (Set path); the CR-02 blob hook writes only the in-memory effectiveModel slot (no layer write); TestBlobFillYieldsToExplicitLayer pins explicit-layer-wins"
re_verification:
  previous_status: gaps_found
  previous_score: 26/31
  gaps_closed:
    - "Gap 1 (16-01 truth 5, FAILED): Barrier lost-wakeup CR-01 — closed by 16-07 per-generation broadcast wake; TestTurnEmitterBarrierConcurrentWaiters (no ctx escape) passes 5x under -race in this verifier's run"
    - "Gap 2 (16-01 truth 6, FAILED): updates-before-response under concurrency — same root cause, same fix; each waiter returns only when written >= target and writeOut bumps written only after sink.Write returns, proven by the same regression"
    - "Gap 3 (16-04 truth 3, PARTIAL): global layer independently addressable — closed by 16-08 scope-aware idempotenceBasisLocked; TestScopeRouting_GlobalWritePersistsWhenCombinedMatches (both subtests) passes; D-10 project-scope pins unmodified and green"
    - "Gap 4a (16-05 truth 1 leg-a, PARTIAL): _global twins layer-true — closed by 16-08; TestConfigSurface_GlobalTwinsAdvertiseGlobalLayer (model/tier/absent-file-floor subtests) passes"
    - "Gap 4b (16-05 truth 1 leg-b, PARTIAL): chip==wire pre-stamp — closed by 16-09 (layer path: Runner.defaultTurnModel mirrors resolveModelLocked) AND by review-fix CR-02 (blob path: SetDefaultTurnModel + SetBlobDefaultHook wired at acp_serve.go:233); TestDefaultTurnModel_FollowsTierResolution (4 subtests), TestConfigAdvertisement_ResolverTruth, TestBlobFillReachesTheWire all pass"
    - "Gap 5 (16-05 truth 2, PARTIAL): idempotence basis scope-aware — closed with gap 3 (one root cause); addressed-layer-true-idempotence subtest passes"
  gaps_remaining: []
  regressions: []
deferred: # Items addressed in later phases — not actionable gaps (carried from prior round; still valid)
  - truth: "load/resume responses carry the richer capability set incl. configOptions advertisement (ROADMAP criterion 4 legs beyond initialize/new)"
    addressed_in: "Phase 18"
    evidence: "Phase 18 goal: 'list / load-resume / close-delete with full replay and live-state reconciliation'; configOptionsFor shipped as the shared builder explicitly reusable by load/resume"
  - truth: "agent_thought_chunk streams from a live provider source during real turns (ROADMAP criterion 1 leg)"
    addressed_in: "Phase 21"
    evidence: "PAR-05 owns the provider thinking source; wire shape shipped + unit-proven here; operator checkpoint explicitly excluded live thought-chunk rendering"
human_verification:
  - test: "Live-Zed chip==wire confirmation: connect a real Zed client with the project tier model differing from the profile slug; read the Model chip BEFORE any editor stamp; then send an initialize _meta blob with a model fill and read the chip again and the next provider request's model"
    expected: "Pre-stamp chip shows the tier-resolved config model (no longer the profile slug — the operator-observed turn-001 GLM-5.3-vs-glm-5.2 divergence class); after a blob fill, chip and wire model move TOGETHER"
    why_human: "Live editor rendering + real provider traffic; the divergence was only ever observed live (16-06 WINDOWS #11)"
  - test: "CR-02 blob-tier edge (flagged by the review-fix pass): with an editor stamp set under tier A, deliver an initialize _meta blob whose tier fill resolves to tier B with a different model; observe the effective model of the next turn"
    expected: "The hook overwrites the prior stamp so chip==wire holds under the NEW tier — confirm this product intent (a layer-backed stamp would make the fill inert instead; only in-memory stamps are overwritten)"
    why_human: "ACP precedence-semantics decision the fixer explicitly marked 'requires human verification'; machine checks pin the mechanism, not the intent"
  - test: "CR-01(new) turn-scoped cancel semantics (flagged by the review-fix pass): start a turn, cancel it mid-stream, then re-prompt the SAME session id; then logout and re-check"
    expected: "Cancel keeps the session registered and promptable (re-prompt succeeds; MCP host/transcript/forwarder stay live); reaping happens only on logout or serve teardown — confirm this product intent"
    why_human: "Product-intent confirmation on ACP session lifecycle semantics, marked 'requires human verification' by the fixer; TestSessionCancelDoesNotReapTheSession pins the mechanism"
  - test: "WR-01 per-turn hook executor (flagged by the review-fix pass): run the same hook chain concurrently from two DIFFERENT sessions"
    expected: "Both run (HOOK-04 in-flight reentrancy guard is now per-turn-instance; loop prevention within one chain unchanged) — confirm cross-session concurrency is the intended behavior change"
    why_human: "Behavior change to loop-prevention scope marked 'requires human verification' by the fixer"
---

# Phase 16: ACP Wire Foundation — Verification Report

**Phase Goal:** The three primitives every interactive phase depends on exist and are proven under concurrency: outbound id'd JSON-RPC requests with a pending-response registry, one ordered inline TurnEmitter owning all client frames with explicit backpressure, and the extended transcript line types (raw-thinking passthrough as `json.RawMessage`, `local_command`, compaction marker) that later phases' schemas lock here.
**Verified:** 2026-08-28T14:52:04Z
**Status:** human_needed
**Re-verification:** Yes — after gap closure (16-07/16-08/16-09) plus a deep code-review fix pass (8 findings, 16-REVIEW-FIX.md)

## Goal Achievement

All five gaps from the prior round are closed by implementation in the current tree — every fix was verified against the code (not the summaries), and every gap-closure regression was re-run by this verifier under `-race`. The barrier defect (CR-01) that made the phase goal's "proven under concurrency" clause false is fixed exactly as the prior verification prescribed, with the no-ctx-escape regression the prescription demanded. The review-fix pass that followed landed 8 further fixes (2 Critical + 6 Warning) with regression tests; the three fixes carrying semantic decisions surface below as human-verification items, not gaps.

### Observable Truths

Roadmap contract (SCs):

| #  | Truth (ROADMAP SC)                                                                                                                                    | Status     | Evidence |
|----|------------------------------------------------------------------------------------------------------------------------------------------------------|------------|----------|
| 1  | Live turn activity streams in real time (tool_call/tool_call_update, plan-from-TodoWrite, agent_thought_chunk), never a black-box spinner              | ✓ VERIFIED | Carried from prior round (wire tests, simulator stages 2-3, OPERATOR-CONFIRMED items 1-3); emitter frame surface untouched by the closure commits; thought-chunk live source → Phase 21 (deferred) |
| 2  | Concurrent emitters arrive in one consistent order; multi-emitter -race stress proves no reordering / no dropped frames; policy documented (fg first)  | ✓ VERIFIED | Adversarial soak RE-RUN by this verifier on the CURRENT broadcast-wake code: 152s, exit 0, all five invariants asserted internally (no-drop, fifo, no-leak, stall-fired, clean-close); full acp package green under -race |
| 3  | Id'd request matched by id while notifications stream; registry resolves without the writer mutex; unanswered degrades loudly                          | ✓ VERIFIED | Carried (registry core + ladder tests green in the full acp sweep); registry send path unchanged since prior round |
| 4  | Initialize/new/load/resume carry the richer capability set, verified against the ACP schema in a real Zed handshake                                    | ✓ VERIFIED (initialize/new legs) | Chip==wire now holds on ALL paths: layer path (16-09 `defaultTurnModel` mirroring `resolveModelLocked` — TestDefaultTurnModel_FollowsTierResolution, TestConfigAdvertisement_ResolverTruth) and blob path (review CR-02 — TestBlobFillReachesTheWire drives the REAL composition end-to-end); _global twins layer-true (TestConfigSurface_GlobalTwinsAdvertiseGlobalLayer); load/resume legs → Phase 18 (deferred) |
| 5  | Transcript lines raw_thinking / local_command / compaction append-only, redaction excluded by construction for thinking bytes                          | ✓ VERIFIED | Carried; appendLineUnredacted sole-caller re-grepped in current tree (manager.go:350); session package green in the sweep |

Plan-level truths (31 total across 16-01..16-06): **31/31 VERIFIED** — the prior round's 2 FAILED and 3 PARTIAL truths are all closed with passing behavioral tests (see re_verification.gaps_closed). 0 present-but-behavior-unverified.

**Score:** 31/31 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/acp/emitter.go` | TurnEmitter: bounded lanes, nested-select drain, stall detector, barrier with broadcast wake | ✓ VERIFIED | Barrier wake is the per-generation broadcast the prior verification prescribed: `close(t.wake)` + fresh-channel swap in writeOut under mu, same critical section as `written++` (lines 320-321); Barrier snapshots `wake` under the same lock as the check (:283); last commit touching the file is 16-07 GREEN (4fddf9f) — nothing after it |
| `internal/acp/emitter_test.go` | TestTurnEmitterBarrierConcurrentWaiters, no ctx escape | ✓ VERIFIED | Two waiters on `context.Background()` (deliberately never cancelled), both targets satisfied by the SAME final frame, gated-writer harness, bounded 2s join with explicit lost-wakeup message; lone-waiter anti-over-correction subtest; the file's only WithCancel uses (:427, :633) are in other pre-existing tests |
| `internal/acpserve/config_surface.go` | Scope-aware idempotence + layer-true _global twins + blob hook | ✓ VERIFIED | `idempotenceBasisLocked` (:452) — global scope resolves via `globalOnlyResolvedLocked` (:479, global layer alone, no project layer, no blobFills), project scope keeps combined (D-10 intact); twins built from `gRes` (:688-704) with embedded-floor fallback ladder; `SetBlobDefaultHook` (:148) wired in Run (acp_serve.go:233); WR-03 lock split preserved all three semantics |
| `internal/acpserve/config_test.go` | Scope-routing + twins regressions | ✓ VERIFIED | TestScopeRouting_GlobalWritePersistsWhenCombinedMatches (differing-value-persists + true-idempotence-no-churn subtests), TestConfigSurface_GlobalTwinsAdvertiseGlobalLayer (model/tier/absent-file subtests) — all pass; TestSetIdempotent_* D-10 pins unmodified and green |
| `internal/acpserve/chip_truth_test.go` | Advertisement resolver-truth pin | ✓ VERIFIED | TestConfigAdvertisement_ResolverTruth pins the bare model entry to an independent `modelrouting` resolver evaluation over the same layer files |
| `internal/acpserve/blob_chip_wire_test.go` | Blob-path chip==wire (review CR-02) | ✓ VERIFIED | TestBlobFillReachesTheWire (blob fill shows on chip AND rides the next provider request through the real Run composition) + TestBlobFillYieldsToExplicitLayer (explicit layer inerts the fill, hook never fired) — both pass |
| `internal/runtime/runtime.go` | defaultTurnModel ladder + sessionFor fallthrough + SetDefaultTurnModel | ✓ VERIFIED | `defaultTurnModel` (:1627) mirrors resolveModelLocked exactly (resolver → static binding → "", nil schedCfg → profile slug); sessionFor stamp (:1110-1116) keeps the explicit stamp first (D-12); `SetDefaultTurnModel` (:1573) writes the same modelMu-guarded slot, no live restamp |
| `internal/runtime/apply_model_test.go` | Four-subtest default-model regression | ✓ VERIFIED | TestDefaultTurnModel_FollowsTierResolution (tier-resolution default, explicit-stamp precedence, nil-config profile default, resolver-decline static-binding fallback) passes; existing ApplyTurnModel pins unmodified and green |
| Prior-round artifacts (types.go, request_registry.go, metrics.go, transcript.go/manager.go, config_write.go, modelrouting config, handlers.go/server.go, simulator_e2e_test.go, emitter_soak_test.go) | unchanged contracts | ✓ VERIFIED (regression sanity) | Full -race sweep of the four wire packages green; providerfactory/modelrouting writer tests green; debt-marker scan clean on all phase files |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| acpserve/acp_serve.go | acp/emitter.go | TurnEmitter construction + SetEmitter | ✓ WIRED | Carried; unchanged |
| runtime/runtime.go | acp/emitter.go | ActivityEmitter forwarders | ✓ WIRED | Carried; full runtime package green under -race |
| acp/server.go | request_registry.go | Serve interception before dispatch | ✓ WIRED | Carried; unchanged |
| request_registry.go | emitter.go | $/cancel_request cascade via fg lane | ✓ WIRED | Carried; unchanged |
| acp/handlers.go | acpserve/config_surface.go | ConfigSurface interface via WithConfigSurface | ✓ WIRED | Carried; unchanged |
| config_surface.go | providerfactory/config_write.go | WriteLayerOption per scope | ✓ WIRED | Carried; now reached only when the addressed-layer basis says the value differs (WR-05 fix) |
| config_surface.go | runtime/runtime.go | applyLocked → Runner.ApplyTurnModel | ✓ WIRED | Carried + NEW: blob-channel twin `SetBlobDefaultHook(runner.SetDefaultTurnModel)` wired at acp_serve.go:233, fired outside the surface lock (composes with the WR-03 lock split) |
| runtime.go sessionFor | modelrouting resolver | defaultTurnModel → NewResolver().Resolve | ✓ WIRED | Same chain as the advertisement's resolveModelLocked — chip==wire by construction, pinned from both sides |
| simulator_e2e_test.go | acpserve/acp_serve.go | real Run over pipes | ✓ WIRED | Green in the full-package sweep (6 clean acpserve runs total) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| CR-01 regression, no ctx escape, 5x shake | `go test -race ./internal/acp/ -run TestTurnEmitterBarrierConcurrentWaiters -count=5` | ok 2.4s | ✓ PASS |
| WR-05 scope routing + twins + D-10 pins | `go test -race ./internal/acpserve/ -run 'TestScopeRouting_Global*|TestConfigSurface_GlobalTwins*|TestSetIdempotent_*' -count=1 -v` | 8/8 tests + subtests PASS | ✓ PASS |
| Chip==wire layer path + blob path | `go test -race ./internal/acpserve/ -run 'TestConfigAdvertisement_ResolverTruth|TestBlobFill*' -count=1 -v` | 3/3 PASS | ✓ PASS |
| Gap-4b wire side + D-12 precedence | `go test -race ./internal/runtime/ -run 'TestDefaultTurnModel_FollowsTierResolution|TestApplyTurnModel|TestTierSwitch' -count=1 -v` | all PASS | ✓ PASS |
| Full wire-package sweep | `go test -race -count=1 ./internal/acp/ ./internal/acpserve/ ./internal/runtime/ ./internal/session/` | acp ok, runtime ok, session ok; acpserve failed ONCE (test name not captured), then 5 consecutive clean full-package runs incl. `-count=3` | ✓ PASS (1 non-reproducing flake — see Anti-Patterns) |
| Config writer + session_tier | `go test ./internal/providerfactory/ ./internal/modelrouting/ -run 'TestConfigWrite|TestSessionTier'` | ok | ✓ PASS |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| `internal/acp/emitter_soak_test.go` (env-gated adversarial soak) | `ASSGUARD_EMITTER_SOAK=1 go test ./internal/acp/ -run TestTurnEmitterSoak -count=1 -timeout 10m` | exit 0, ok 152s — invariants (no-drop, fifo, no-leak, stall-fired, clean-close) asserted internally; re-run by this verifier on the CURRENT broadcast-wake code | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| ACP-03 | 16-01, 16-02, 16-03, 16-06, 16-07 | Zed renders live turn activity through one ordered inline TurnEmitter with explicit backpressure | ✓ SATISFIED | The CR-01 concurrency defect against the "proven under concurrency" clause is FIXED and pinned by the no-ctx-escape two-waiter regression (5x -race) plus this verifier's own soak re-run on the fixed code |
| ACP-08 | 16-04, 16-05, 16-06, 16-08, 16-09 | Editor drives configuration: configOptions advertised, set_config_option handled, API keys env/file only | ✓ SATISFIED | Scope semantics corrected (global layer genuinely independently addressable; twins layer-true; idempotence basis addressed-layer); chip==wire holds on layer AND blob paths; D-10 anti-promotion guard preserved |

Orphaned requirements: none — REQUIREMENTS.md maps exactly ACP-03 and ACP-08 to Phase 16 (traceability table :106-107); both are claimed across the plan frontmatters including the gap-closure plans.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| internal/acpserve (package) | — | One unidentified test failure in the first full sweep; 5 subsequent clean full-package runs (incl. -count=3) under -race; all named gap-closure regressions deterministic | ⚠️ Warning | Non-reproducing flake in a timing-sensitive suite; not attributable to a specific test (name not captured in the truncated output). Watch for recurrence in Phase 17 |
| internal/acp/server.go | 96+ | Prior-round WR-01: metrics field written after sampler goroutine start (unsynchronized) — still present, untouched by the fix pass | ⚠️ Warning | Bounded: missed stall count |
| internal/runtime/cron_wiring.go | 282-284 | Prior-round WR-02: fanInEvents teardown contract false (Unsubscribe never closes channels) — still present | ⚠️ Warning | ~5 goroutines leak per closed session in long-lived processes |
| internal/acp/request_registry.go | 387-401 | Prior-round WR-03: send TOCTOU vs Writer.Close — still present; latent for the first non-handler caller (Phase 17 background asks) | ⚠️ Warning | Latent send-on-closed-channel panic |
| 16-REVIEW.md IN-01..IN-06 | — | Six Info findings explicitly out of the fix pass's scope (dead keep-alive call, asciiDelete misnomer, duplicate sentinels, optionsLocked comment vs pending-twins, toolchain pin 1.26 vs 1.25 floor, blank-stdin -32700) | ℹ️ Info | Tracked in 16-REVIEW.md for a follow-up pass |

Debt markers (TBD/FIXME/XXX): none in any phase-modified file. Stub patterns: none.

### Human Verification Required

Four items (frontmatter `human_verification`): the live-Zed chip==wire confirmation, plus the three semantic decisions the review-fix pass explicitly marked "requires human verification" (CR-02 blob-tier stamp overwrite edge, CR-01-new turn-scoped cancel intent, WR-01 cross-session hook concurrency). These are product-intent and live-rendering checks — the mechanisms are pinned by tests; the intents are not.

### Gaps Summary

None. All five prior-round gaps are closed in the current tree by implementation (not override), each with the exact regression test the prior verification prescribed, and each re-run by this verifier under -race. The review-fix pass that followed closed 8 additional findings with regression tests and full-package gates (its own report records `go test -race -count=1 ./...` across 37 packages, 0 failures). The three fix-pass items carrying semantic decisions are routed to human verification above; they do not block the goal — the mechanisms are correct and tested, and the open questions are intent confirmations plus one live-rendering check.

---

_Verified: 2026-08-28T14:52:04Z_
_Verifier: Claude (gsd-verifier)_
