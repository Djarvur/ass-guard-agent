---
schema_version: 1
open_count: 13
waived_count: 0
fixed_count: 10
total_count: 23
last_updated: 2026-09-10T11:43:49.324Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 12 | deviation | internal/acp/framer.go | 34 | Pre-existing, surfaced by the 12-01 live witness: the Writer's decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023; 450 transcript vs 440 wire chunks reconciled). Relaxing it reverses a documented v1.0 transport decision — operator disposition required | fixed | 260819-nlg: guard relaxed to the raw-byte check (decoded newlines are legal JSON content; spec rule is wire-bytes framing); model chunks now reach the wire | 2026-08-19T00:22:26.401Z | 2026-08-19T07:15:30.000Z |
| 2 | 12 | deviation | internal/acp/framer.go | 34 | Pre-existing, surfaced by the 12-01 live witness: the Writer decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023) - operator disposition required | fixed | 260819-nlg: guard relaxed to the raw-byte check (duplicate of #1) | 2026-08-19T00:22:32.786Z | 2026-08-19T07:15:30.000Z |
| 3 | 12 | stub | internal/session/ask.go |  | D-01 timer-driven resume: the resumed turn runs server-side and is transcript-recorded, but its chunks are not mirrored to the ACP client (no active Run subscription at fire time); the reply path IS fully client-visible | fixed |  | 2026-08-19T00:22:32.985Z | 2026-08-20T12:37:47.286Z |
| 4 | 14 | unrun-verify | cmd/ass-guard/checkpoint_test.go |  | Gated live rollback leg TestCheckpointLiveRollback_Gated not executed in plan 14-01 (ZAI_API_KEY absent at run time; test skips loud naming both env gates). Offline battery proves byte-identical restore + user-git-untouched; live run command: ASSGUARD_CHECKPOINT_E2E=1 ZAI_API_KEY=<key> go test ./cmd/ass-guard/... -run TestCheckpointLiveRollback_Gated -count=1 -v | fixed | 260820-live: gated E2E executed with operator ZAI_API_KEY — PASS (15.54s, exit 0): real GLM turn mutated the scratch repo, Store.Restore returned byte-identical workspace, user .git HEAD/index/porcelain unchanged; evidence restoredRef=refs/checkpoints/sess-ckpt-live-turn-001 + transcript_sess-ckpt-live.jsonl (test temp dir); log /tmp/ckpt-live-e2e.log | 2026-08-19T17:58:28.367Z | 2026-08-19T22:55:00.000Z |
| 5 | 14 | deviation | cmd/ass-guard/parity.go |  | Standing cache probe FAIL on every parity run: placement-vs-pin reports the routed system-class cache_control emission gap (14-03 CC-1, post-adoption queue) — expected verdict, flips green when the TextBlock format fix lands | fixed | 260906-19-01: cache_control emission landed — composition derives from the profile, placement-vs-pin green on the committed pin fixture | 2026-08-19T19:13:00.378Z | 2026-09-06T20:24:05.615Z |
| 6 | 12 | unrun-verify | internal/parity/replay.go |  | 12-05 Behavior-4 zero-target deferred to 12-03 (post-adoption): the extractor decomposition ran on the fresh capture with the CURRENT extractor (36 turns / 12 empty-expectation vs the drift baseline 6/13) — the delta-aware fix + the zero-empty acceptance re-runs over the committed capture when 12-03 executes | fixed |  | 2026-08-20T00:02:29.871Z | 2026-08-20T11:37:21.920Z |
| 7 | 12 | deviation | tools/zcode-recapture/harvest-deferred-forms.mjs |  | 12-05: the primary capture rollout rotated off ~/.zcode/cli/rollout/ mid-harvest (D-04 loss class, live repeat) — unique families pinned verbatim in the committed fixture; supplementary session snapshot was /tmp-only. Future harvests must snapshot the rollout file IMMEDIATELY after each live pass | open |  | 2026-08-20T00:02:30.135Z |  |
| 8 | 12 | unrun-verify | cmd/ass-guard/evalsuite_bridge_test.go |  | 12-08: the eval gate's FIRST GREEN is blocked on pattern-table drift — three live runs, three chain shapes (run1 stall after explore; run2 archive-stage ask suspension; run3 apply self-injection x8 to the budget cap; evidence /tmp/eval-net-evidence/12-08-first-runs/). The NET itself is complete and its opening catches are real findings. Run: mise eval-gate. Fix route: capture-informed re-tuning of the 08-06 stage-transition patterns (operator decision) | fixed | 260821-close: re-tuning landed as seeded.toml row edits ONLY (8c56b29, RE-TUNED 2026-08-20 provenance in-file; internal/evalsuite byte-untouched across 13-00); flagship green ×3 — eval-20260820-161520 / 163220 / 205550-k1.json (13-00 gate, 13-01 re-verify, manager certification); 13-VERIFICATION 3/3 PASS | 2026-08-20T13:29:34.742Z | 2026-08-20T21:20:00.000Z |
| 9 | 12 | deviation | cmd/ass-guard/e2e_opsx_test.go |  | 12-08 finding: the Phase-8 flagship proof predates REAL asks (12-01) — a mid-chain AskUserQuestion now suspends the chain at the no-chain-suspension pin; the E2E runners carry askTimeout=45s (D-01's documented hands-off mode: the bounded timeout returns the capture-shaped non-answer and the model proceeds) | fixed | 260821-close: root closed by 13-00's engine-visible ask resume (Rule-4 route 1, commits c528236..bc07dab) — chains survive mid-chain asks, pinned by TestAskWiring_ChainSurvivesAskTimerResume + the park battery; flagship green THROUGH asks ×3 (eval-20260820-161520 / 163220 / 205550-k1.json); 13-VERIFICATION 3/3 PASS | 2026-08-20T13:29:34.929Z | 2026-08-20T21:20:00.000Z |
| 10 | 15 | unmet-truth | .planning/phases/15-internal-runtime-carve-step-0/15-07-SUMMARY.md |  | PENDING-OPERATOR-CONFIRMATION: live-Zed editor-session identity check (ROADMAP criterion 2) not yet executed by operator | open |  | 2026-08-26T13:57:30.775Z |  |
| 11 | 16 | unmet-truth | .planning/phases/16-acp-wire-foundation/16-06-SUMMARY.md |  | PENDING-OPERATOR-CONFIRMATION: live-Zed operator confirmation of ROADMAP criteria 1 and 4 (native tool cards/plan panel/streaming tokens + config options in Zed's settings UI; editor model switch changes the next request) not yet executed; the 16-06 simulator + soak prove the wire, the five-item checklist awaits the operator | fixed | 260827-confirm: operator-delegated live-Zed exercise, machine-verified by the orchestrator (2026-08-27) — all five items PASS: native tool cards with live diffs, TodoWrite plan panel, streamed message tokens, config chips in Zed's message bar with layer-matched values, and editor model switch changing the next request (session chip glm-5.2→GLM-5.3; project layer persisted tiers.heavy.model GLM-5.3; transcript_b7fd0737 turn-002 all 3 requests GLM-5.3). One recorded finding (pre-stamp Model chip untruthful vs profile-default wire model) handed to the Phase 16 verifier via deferred-items.md; 16-06 SUMMARY marker flipped OPERATOR-CONFIRMED | 2026-08-27T18:30:14.289Z | 2026-08-27T19:57:39.950Z |
| 12 | 17 | deviation | internal/acpserve/acp_serve.go |  | 17-04 Rule-3 wiring deviation: ElicitationAsk composition + SetAskFire injection live in acp_serve.go (beyond the plan's file list; the key_links' wiring seams) | open |  | 2026-09-01T02:47:24.814Z |  |
| 13 | 17 | deviation | internal/runtime/ask_wiring_test.go |  | 17-04 Rule-3 wiring deviation: the Run-level AskUserQuestion round-trip test lives in ask_wiring_test.go (beyond the plan's file list; the real harness exists only there) | open |  | 2026-09-01T02:47:33.475Z |  |
| 14 | 17 | deviation | internal/session/askqueue.go |  | 17-04 Rule-3 contract deviation: AskOutcome elicitation fields (Elicit/Content/Violation/Fallback) + AskEntry.Note/PlainTextFallback live beside the queue types (beyond the plan's file list; the dispatcher's input/outcome contract) | open |  | 2026-09-01T02:47:33.676Z |  |
| 15 | 17 | unmet-truth | .planning/phases/17-permissions-elicitation/17-05-SUMMARY.md |  | PENDING-OPERATOR-CONFIRMATION: live-Zed operator confirmation of ROADMAP criteria 1 and 3 (native four-option permission dialog with persistence + native elicitation form rendering, seven-step checklist) not yet executed; the 17-05 simulator battery + docs prove the wire and the pipeline, the live legs await the operator | fixed |  | 2026-09-01T03:34:40.899Z | 2026-09-03T10:24:28.543Z |
| 16 | 18 | deviation | .mise.toml |  | golangci-lint 2.12.2 panics on go1.27-requiring deps (pre-existing module-cache drift); 2.13.2 runs but exhaustruct_v5 flags 2222 pre-existing issues — mise ci's lint step red until a standalone pin-bump + exhaustruct-config migration lands (18-06 files verified clean; logged in 18-session-family/deferred-items.md) | open |  | 2026-09-03T16:26:32.677Z |  |
| 17 | 21 | deviation | internal/session/testdata/thinking-golden/sse-thinking.jsonl |  | D-14 golden fixture is corpus_absent synthetic (A4 fallback): no thinking-bearing SSE captures exist in the corpus; provenance header records the hunt; replace with captured wire pairs when a thinking-enabled capture run lands | open |  | 2026-09-03T19:54:40.279Z |  |
| 18 | 21 | deviation | internal/runtime/imgscale_test.go |  | plan-literal 9000x6000 over-dims fixture env-gated (ASSGUARD_IMG_HEAVY=1); cheap default row proves the same path (suite-health tuning) | open |  | 2026-09-03T22:27:52.112Z |  |
| 19 | 21 | lint-warning | .golangci.yml |  | golangci-lint v2.12.2 (built with go1.26) panics on a go1.27-requiring dependency — mise ci lint leg red before and after 21-06 (environmental; vet+build+test green) | open |  | 2026-09-03T22:57:28.131Z |  |
| 20 | 19 | deviation | .golangci.yml |  | mise lint gate fails repo-wide: golangci-lint 2.13.2 exhaustruct_v5 rename defeats the 2.12.x-tuned wildcard exclusion (pre-existing env drift, details in 19 deferred-items.md) | open |  | 2026-09-06T21:32:25.796Z |  |
| 21 | 22 | deviation | internal/runtime/rescan_test.go |  | TestRescanConcurrency data race (spawnMCP vs installRegistry) pre-existing before 22-04 — blocks the ./internal/runtime/ green gate; logged in 22-04 deferred-items.md | open |  | 2026-09-10T00:24:14.411Z |  |
| 22 | 22 | unrun-verify | internal/acpserve |  | TestPermissionsE2E (acpserve) skipped under -race in 22-06's phase-quick verify — pre-existing cross-workstream regression from Phase 23 commit 40b2bbc (STATE.md blocker, operator-bisected); passes at 721c7bc | open |  | 2026-09-10T01:06:35.602Z |  |
| 23 | 23 | unmet-truth | .planning/phases/23-seed-gaps-close-out/23-05-SUMMARY.md |  | PENDING-OPERATOR-CONFIRMATION: live-Zed operator UAT of the phase's two interactive surfaces (23-01/23-02 boundary steering mid-turn + 23-05 /undo incl. the auto-cancel-then-restore path against a running turn) not yet executed — plan 23-05 Task 3 checkpoint; offline batteries prove the core (steering boundary delivery, undo walk/cancel ordering), the Zed-client behavior leg (RESEARCH A2 note: Zed may queue mid-turn prompts client-side) awaits the operator and documents client behavior for TG-02 planning | open |  | 2026-09-10T11:43:49.324Z |  |

````json
[
  {
    "id": 1,
    "kind": "deviation",
    "phase": "12",
    "file": "internal/acp/framer.go",
    "line": 34,
    "description": "Pre-existing, surfaced by the 12-01 live witness: the Writer's decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023; 450 transcript vs 440 wire chunks reconciled). Relaxing it reverses a documented v1.0 transport decision — operator disposition required",
    "status": "fixed",
    "reason": "260819-nlg: guard relaxed to the raw-byte check (decoded newlines are legal JSON content; spec rule is wire-bytes framing); model chunks now reach the wire",
    "recorded_at": "2026-08-19T00:22:26.401Z",
    "resolved_at": "2026-08-19T07:15:30.000Z"
  },
  {
    "id": 2,
    "kind": "deviation",
    "phase": "12",
    "file": "internal/acp/framer.go",
    "line": 34,
    "description": "Pre-existing, surfaced by the 12-01 live witness: the Writer decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023) - operator disposition required",
    "status": "fixed",
    "reason": "260819-nlg: guard relaxed to the raw-byte check (duplicate of #1)",
    "recorded_at": "2026-08-19T00:22:32.786Z",
    "resolved_at": "2026-08-19T07:15:30.000Z"
  },
  {
    "id": 3,
    "kind": "stub",
    "phase": "12",
    "file": "internal/session/ask.go",
    "line": null,
    "description": "D-01 timer-driven resume: the resumed turn runs server-side and is transcript-recorded, but its chunks are not mirrored to the ACP client (no active Run subscription at fire time); the reply path IS fully client-visible",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-19T00:22:32.985Z",
    "resolved_at": "2026-08-20T12:37:47.286Z"
  },
  {
    "id": 4,
    "kind": "unrun-verify",
    "phase": "14",
    "file": "cmd/ass-guard/checkpoint_test.go",
    "line": null,
    "description": "Gated live rollback leg TestCheckpointLiveRollback_Gated not executed in plan 14-01 (ZAI_API_KEY absent at run time; test skips loud naming both env gates). Offline battery proves byte-identical restore + user-git-untouched; live run command: ASSGUARD_CHECKPOINT_E2E=1 ZAI_API_KEY=<key> go test ./cmd/ass-guard/... -run TestCheckpointLiveRollback_Gated -count=1 -v",
    "status": "fixed",
    "reason": "260820-live: gated E2E executed with operator ZAI_API_KEY — PASS (15.54s, exit 0): real GLM turn mutated the scratch repo, Store.Restore returned byte-identical workspace, user .git HEAD/index/porcelain unchanged; evidence restoredRef=refs/checkpoints/sess-ckpt-live-turn-001 + transcript_sess-ckpt-live.jsonl (test temp dir); log /tmp/ckpt-live-e2e.log",
    "recorded_at": "2026-08-19T17:58:28.367Z",
    "resolved_at": "2026-08-19T22:55:00.000Z"
  },
  {
    "id": 5,
    "kind": "deviation",
    "phase": "14",
    "file": "cmd/ass-guard/parity.go",
    "line": null,
    "description": "Standing cache probe FAIL on every parity run: placement-vs-pin reports the routed system-class cache_control emission gap (14-03 CC-1, post-adoption queue) — expected verdict, flips green when the TextBlock format fix lands",
    "status": "fixed",
    "reason": "260906-19-01: cache_control emission landed — composition derives from the profile, placement-vs-pin green on the committed pin fixture",
    "recorded_at": "2026-08-19T19:13:00.378Z",
    "resolved_at": "2026-09-06T20:24:05.615Z"
  },
  {
    "id": 6,
    "kind": "unrun-verify",
    "phase": "12",
    "file": "internal/parity/replay.go",
    "line": null,
    "description": "12-05 Behavior-4 zero-target deferred to 12-03 (post-adoption): the extractor decomposition ran on the fresh capture with the CURRENT extractor (36 turns / 12 empty-expectation vs the drift baseline 6/13) — the delta-aware fix + the zero-empty acceptance re-runs over the committed capture when 12-03 executes",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-20T00:02:29.871Z",
    "resolved_at": "2026-08-20T11:37:21.920Z"
  },
  {
    "id": 7,
    "kind": "deviation",
    "phase": "12",
    "file": "tools/zcode-recapture/harvest-deferred-forms.mjs",
    "line": null,
    "description": "12-05: the primary capture rollout rotated off ~/.zcode/cli/rollout/ mid-harvest (D-04 loss class, live repeat) — unique families pinned verbatim in the committed fixture; supplementary session snapshot was /tmp-only. Future harvests must snapshot the rollout file IMMEDIATELY after each live pass",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-20T00:02:30.135Z",
    "resolved_at": null
  },
  {
    "id": 8,
    "kind": "unrun-verify",
    "phase": "12",
    "file": "cmd/ass-guard/evalsuite_bridge_test.go",
    "line": null,
    "description": "12-08: the eval gate's FIRST GREEN is blocked on pattern-table drift — three live runs, three chain shapes (run1 stall after explore; run2 archive-stage ask suspension; run3 apply self-injection x8 to the budget cap; evidence /tmp/eval-net-evidence/12-08-first-runs/). The NET itself is complete and its opening catches are real findings. Run: mise eval-gate. Fix route: capture-informed re-tuning of the 08-06 stage-transition patterns (operator decision)",
    "status": "fixed",
    "reason": "260821-close: re-tuning landed as seeded.toml row edits ONLY (8c56b29, RE-TUNED 2026-08-20 provenance in-file; internal/evalsuite byte-untouched across 13-00); flagship green ×3 — eval-20260820-161520 / 163220 / 205550-k1.json (13-00 gate, 13-01 re-verify, manager certification); 13-VERIFICATION 3/3 PASS",
    "recorded_at": "2026-08-20T13:29:34.742Z",
    "resolved_at": "2026-08-20T21:20:00.000Z"
  },
  {
    "id": 9,
    "kind": "deviation",
    "phase": "12",
    "file": "cmd/ass-guard/e2e_opsx_test.go",
    "line": null,
    "description": "12-08 finding: the Phase-8 flagship proof predates REAL asks (12-01) — a mid-chain AskUserQuestion now suspends the chain at the no-chain-suspension pin; the E2E runners carry askTimeout=45s (D-01's documented hands-off mode: the bounded timeout returns the capture-shaped non-answer and the model proceeds)",
    "status": "fixed",
    "reason": "260821-close: root closed by 13-00's engine-visible ask resume (Rule-4 route 1, commits c528236..bc07dab) — chains survive mid-chain asks, pinned by TestAskWiring_ChainSurvivesAskTimerResume + the park battery; flagship green THROUGH asks ×3 (eval-20260820-161520 / 163220 / 205550-k1.json); 13-VERIFICATION 3/3 PASS",
    "recorded_at": "2026-08-20T13:29:34.929Z",
    "resolved_at": "2026-08-20T21:20:00.000Z"
  },
  {
    "id": 10,
    "kind": "unmet-truth",
    "phase": "15",
    "file": ".planning/phases/15-internal-runtime-carve-step-0/15-07-SUMMARY.md",
    "line": null,
    "description": "PENDING-OPERATOR-CONFIRMATION: live-Zed editor-session identity check (ROADMAP criterion 2) not yet executed by operator",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-26T13:57:30.775Z",
    "resolved_at": null
  },
  {
    "id": 11,
    "kind": "unmet-truth",
    "phase": "16",
    "file": ".planning/phases/16-acp-wire-foundation/16-06-SUMMARY.md",
    "line": null,
    "description": "PENDING-OPERATOR-CONFIRMATION: live-Zed operator confirmation of ROADMAP criteria 1 and 4 (native tool cards/plan panel/streaming tokens + config options in Zed's settings UI; editor model switch changes the next request) not yet executed; the 16-06 simulator + soak prove the wire, the five-item checklist awaits the operator",
    "status": "fixed",
    "reason": "260827-confirm: operator-delegated live-Zed exercise, machine-verified by the orchestrator (2026-08-27) — all five items PASS: native tool cards with live diffs, TodoWrite plan panel, streamed message tokens, config chips in Zed's message bar with layer-matched values, and editor model switch changing the next request (session chip glm-5.2→GLM-5.3; project layer persisted tiers.heavy.model GLM-5.3; transcript_b7fd0737 turn-002 all 3 requests GLM-5.3). One recorded finding (pre-stamp Model chip untruthful vs profile-default wire model) handed to the Phase 16 verifier via deferred-items.md; 16-06 SUMMARY marker flipped OPERATOR-CONFIRMED",
    "recorded_at": "2026-08-27T18:30:14.289Z",
    "resolved_at": "2026-08-27T19:57:39.950Z"
  },
  {
    "id": 12,
    "kind": "deviation",
    "phase": "17",
    "file": "internal/acpserve/acp_serve.go",
    "line": null,
    "description": "17-04 Rule-3 wiring deviation: ElicitationAsk composition + SetAskFire injection live in acp_serve.go (beyond the plan's file list; the key_links' wiring seams)",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-01T02:47:24.814Z",
    "resolved_at": null
  },
  {
    "id": 13,
    "kind": "deviation",
    "phase": "17",
    "file": "internal/runtime/ask_wiring_test.go",
    "line": null,
    "description": "17-04 Rule-3 wiring deviation: the Run-level AskUserQuestion round-trip test lives in ask_wiring_test.go (beyond the plan's file list; the real harness exists only there)",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-01T02:47:33.475Z",
    "resolved_at": null
  },
  {
    "id": 14,
    "kind": "deviation",
    "phase": "17",
    "file": "internal/session/askqueue.go",
    "line": null,
    "description": "17-04 Rule-3 contract deviation: AskOutcome elicitation fields (Elicit/Content/Violation/Fallback) + AskEntry.Note/PlainTextFallback live beside the queue types (beyond the plan's file list; the dispatcher's input/outcome contract)",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-01T02:47:33.676Z",
    "resolved_at": null
  },
  {
    "id": 15,
    "kind": "unmet-truth",
    "phase": "17",
    "file": ".planning/phases/17-permissions-elicitation/17-05-SUMMARY.md",
    "line": null,
    "description": "PENDING-OPERATOR-CONFIRMATION: live-Zed operator confirmation of ROADMAP criteria 1 and 3 (native four-option permission dialog with persistence + native elicitation form rendering, seven-step checklist) not yet executed; the 17-05 simulator battery + docs prove the wire and the pipeline, the live legs await the operator",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-09-01T03:34:40.899Z",
    "resolved_at": "2026-09-03T10:24:28.543Z"
  },
  {
    "id": 16,
    "kind": "deviation",
    "phase": "18",
    "file": ".mise.toml",
    "line": null,
    "description": "golangci-lint 2.12.2 panics on go1.27-requiring deps (pre-existing module-cache drift); 2.13.2 runs but exhaustruct_v5 flags 2222 pre-existing issues — mise ci's lint step red until a standalone pin-bump + exhaustruct-config migration lands (18-06 files verified clean; logged in 18-session-family/deferred-items.md)",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T16:26:32.677Z",
    "resolved_at": null
  },
  {
    "id": 17,
    "kind": "deviation",
    "phase": "21",
    "file": "internal/session/testdata/thinking-golden/sse-thinking.jsonl",
    "line": null,
    "description": "D-14 golden fixture is corpus_absent synthetic (A4 fallback): no thinking-bearing SSE captures exist in the corpus; provenance header records the hunt; replace with captured wire pairs when a thinking-enabled capture run lands",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T19:54:40.279Z",
    "resolved_at": null
  },
  {
    "id": 18,
    "kind": "deviation",
    "phase": "21",
    "file": "internal/runtime/imgscale_test.go",
    "line": null,
    "description": "plan-literal 9000x6000 over-dims fixture env-gated (ASSGUARD_IMG_HEAVY=1); cheap default row proves the same path (suite-health tuning)",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T22:27:52.112Z",
    "resolved_at": null
  },
  {
    "id": 19,
    "kind": "lint-warning",
    "phase": "21",
    "file": ".golangci.yml",
    "line": null,
    "description": "golangci-lint v2.12.2 (built with go1.26) panics on a go1.27-requiring dependency — mise ci lint leg red before and after 21-06 (environmental; vet+build+test green)",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T22:57:28.131Z",
    "resolved_at": null
  },
  {
    "id": 20,
    "kind": "deviation",
    "phase": "19",
    "file": ".golangci.yml",
    "line": null,
    "description": "mise lint gate fails repo-wide: golangci-lint 2.13.2 exhaustruct_v5 rename defeats the 2.12.x-tuned wildcard exclusion (pre-existing env drift, details in 19 deferred-items.md)",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-06T21:32:25.796Z",
    "resolved_at": null
  },
  {
    "id": 21,
    "kind": "deviation",
    "phase": "22",
    "file": "internal/runtime/rescan_test.go",
    "line": null,
    "description": "TestRescanConcurrency data race (spawnMCP vs installRegistry) pre-existing before 22-04 — blocks the ./internal/runtime/ green gate; logged in 22-04 deferred-items.md",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-10T00:24:14.411Z",
    "resolved_at": null
  },
  {
    "id": 22,
    "kind": "unrun-verify",
    "phase": "22",
    "file": "internal/acpserve",
    "line": null,
    "description": "TestPermissionsE2E (acpserve) skipped under -race in 22-06's phase-quick verify — pre-existing cross-workstream regression from Phase 23 commit 40b2bbc (STATE.md blocker, operator-bisected); passes at 721c7bc",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-10T01:06:35.602Z",
    "resolved_at": null
  },
  {
    "id": 23,
    "kind": "unmet-truth",
    "phase": "23",
    "file": ".planning/phases/23-seed-gaps-close-out/23-05-SUMMARY.md",
    "line": null,
    "description": "PENDING-OPERATOR-CONFIRMATION: live-Zed operator UAT of the phase's two interactive surfaces (23-01/23-02 boundary steering mid-turn + 23-05 /undo incl. the auto-cancel-then-restore path against a running turn) not yet executed — plan 23-05 Task 3 checkpoint; offline batteries prove the core (steering boundary delivery, undo walk/cancel ordering), the Zed-client behavior leg (RESEARCH A2 note: Zed may queue mid-turn prompts client-side) awaits the operator and documents client behavior for TG-02 planning",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-10T11:43:49.324Z",
    "resolved_at": null
  }
]
````
