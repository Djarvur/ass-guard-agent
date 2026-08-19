---
schema_version: 1
open_count: 3
waived_count: 0
fixed_count: 2
total_count: 5
last_updated: 2026-08-19T19:13:00.378Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 12 | deviation | internal/acp/framer.go | 34 | Pre-existing, surfaced by the 12-01 live witness: the Writer's decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023; 450 transcript vs 440 wire chunks reconciled). Relaxing it reverses a documented v1.0 transport decision — operator disposition required | fixed | 260819-nlg: guard relaxed to the raw-byte check (decoded newlines are legal JSON content; spec rule is wire-bytes framing); model chunks now reach the wire | 2026-08-19T00:22:26.401Z | 2026-08-19T07:15:30.000Z |
| 2 | 12 | deviation | internal/acp/framer.go | 34 | Pre-existing, surfaced by the 12-01 live witness: the Writer decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023) - operator disposition required | fixed | 260819-nlg: guard relaxed to the raw-byte check (duplicate of #1) | 2026-08-19T00:22:32.786Z | 2026-08-19T07:15:30.000Z |
| 3 | 12 | stub | internal/session/ask.go |  | D-01 timer-driven resume: the resumed turn runs server-side and is transcript-recorded, but its chunks are not mirrored to the ACP client (no active Run subscription at fire time); the reply path IS fully client-visible | open |  | 2026-08-19T00:22:32.985Z |  |
| 4 | 14 | unrun-verify | cmd/ass-guard/checkpoint_test.go |  | Gated live rollback leg TestCheckpointLiveRollback_Gated not executed in plan 14-01 (ZAI_API_KEY absent at run time; test skips loud naming both env gates). Offline battery proves byte-identical restore + user-git-untouched; live run command: ASSGUARD_CHECKPOINT_E2E=1 ZAI_API_KEY=<key> go test ./cmd/ass-guard/... -run TestCheckpointLiveRollback_Gated -count=1 -v | open |  | 2026-08-19T17:58:28.367Z |  |
| 5 | 14 | deviation | cmd/ass-guard/parity.go |  | Standing cache probe FAIL on every parity run: placement-vs-pin reports the routed system-class cache_control emission gap (14-03 CC-1, post-adoption queue) — expected verdict, flips green when the TextBlock format fix lands | open |  | 2026-08-19T19:13:00.378Z |  |

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
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-19T00:22:32.985Z",
    "resolved_at": null
  },
  {
    "id": 4,
    "kind": "unrun-verify",
    "phase": "14",
    "file": "cmd/ass-guard/checkpoint_test.go",
    "line": null,
    "description": "Gated live rollback leg TestCheckpointLiveRollback_Gated not executed in plan 14-01 (ZAI_API_KEY absent at run time; test skips loud naming both env gates). Offline battery proves byte-identical restore + user-git-untouched; live run command: ASSGUARD_CHECKPOINT_E2E=1 ZAI_API_KEY=<key> go test ./cmd/ass-guard/... -run TestCheckpointLiveRollback_Gated -count=1 -v",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-19T17:58:28.367Z",
    "resolved_at": null
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
  }
]
````
