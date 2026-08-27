---
schema_version: 1
open_count: 4
waived_count: 0
fixed_count: 7
total_count: 11
last_updated: 2026-08-27T18:30:14.289Z
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
| 5 | 14 | deviation | cmd/ass-guard/parity.go |  | Standing cache probe FAIL on every parity run: placement-vs-pin reports the routed system-class cache_control emission gap (14-03 CC-1, post-adoption queue) — expected verdict, flips green when the TextBlock format fix lands | open |  | 2026-08-19T19:13:00.378Z |  |
| 6 | 12 | unrun-verify | internal/parity/replay.go |  | 12-05 Behavior-4 zero-target deferred to 12-03 (post-adoption): the extractor decomposition ran on the fresh capture with the CURRENT extractor (36 turns / 12 empty-expectation vs the drift baseline 6/13) — the delta-aware fix + the zero-empty acceptance re-runs over the committed capture when 12-03 executes | fixed |  | 2026-08-20T00:02:29.871Z | 2026-08-20T11:37:21.920Z |
| 7 | 12 | deviation | tools/zcode-recapture/harvest-deferred-forms.mjs |  | 12-05: the primary capture rollout rotated off ~/.zcode/cli/rollout/ mid-harvest (D-04 loss class, live repeat) — unique families pinned verbatim in the committed fixture; supplementary session snapshot was /tmp-only. Future harvests must snapshot the rollout file IMMEDIATELY after each live pass | open |  | 2026-08-20T00:02:30.135Z |  |
| 8 | 12 | unrun-verify | cmd/ass-guard/evalsuite_bridge_test.go |  | 12-08: the eval gate's FIRST GREEN is blocked on pattern-table drift — three live runs, three chain shapes (run1 stall after explore; run2 archive-stage ask suspension; run3 apply self-injection x8 to the budget cap; evidence /tmp/eval-net-evidence/12-08-first-runs/). The NET itself is complete and its opening catches are real findings. Run: mise eval-gate. Fix route: capture-informed re-tuning of the 08-06 stage-transition patterns (operator decision) | fixed | 260821-close: re-tuning landed as seeded.toml row edits ONLY (8c56b29, RE-TUNED 2026-08-20 provenance in-file; internal/evalsuite byte-untouched across 13-00); flagship green ×3 — eval-20260820-161520 / 163220 / 205550-k1.json (13-00 gate, 13-01 re-verify, manager certification); 13-VERIFICATION 3/3 PASS | 2026-08-20T13:29:34.742Z | 2026-08-20T21:20:00.000Z |
| 9 | 12 | deviation | cmd/ass-guard/e2e_opsx_test.go |  | 12-08 finding: the Phase-8 flagship proof predates REAL asks (12-01) — a mid-chain AskUserQuestion now suspends the chain at the no-chain-suspension pin; the E2E runners carry askTimeout=45s (D-01's documented hands-off mode: the bounded timeout returns the capture-shaped non-answer and the model proceeds) | fixed | 260821-close: root closed by 13-00's engine-visible ask resume (Rule-4 route 1, commits c528236..bc07dab) — chains survive mid-chain asks, pinned by TestAskWiring_ChainSurvivesAskTimerResume + the park battery; flagship green THROUGH asks ×3 (eval-20260820-161520 / 163220 / 205550-k1.json); 13-VERIFICATION 3/3 PASS | 2026-08-20T13:29:34.929Z | 2026-08-20T21:20:00.000Z |
| 10 | 15 | unmet-truth | .planning/phases/15-internal-runtime-carve-step-0/15-07-SUMMARY.md |  | PENDING-OPERATOR-CONFIRMATION: live-Zed editor-session identity check (ROADMAP criterion 2) not yet executed by operator | open |  | 2026-08-26T13:57:30.775Z |  |
| 11 | 16 | unmet-truth | .planning/phases/16-acp-wire-foundation/16-06-SUMMARY.md |  | PENDING-OPERATOR-CONFIRMATION: live-Zed operator confirmation of ROADMAP criteria 1 and 4 (native tool cards/plan panel/streaming tokens + config options in Zed's settings UI; editor model switch changes the next request) not yet executed; the 16-06 simulator + soak prove the wire, the five-item checklist awaits the operator | open |  | 2026-08-27T18:30:14.289Z |  |

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
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-19T19:13:00.378Z",
    "resolved_at": null
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
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-27T18:30:14.289Z",
    "resolved_at": null
  }
]
````
