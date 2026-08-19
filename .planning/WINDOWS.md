---
schema_version: 1
open_count: 3
waived_count: 0
fixed_count: 0
total_count: 3
last_updated: 2026-08-19T00:22:32.985Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 12 | deviation | internal/acp/framer.go | 34 | Pre-existing, surfaced by the 12-01 live witness: the Writer's decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023; 450 transcript vs 440 wire chunks reconciled). Relaxing it reverses a documented v1.0 transport decision — operator disposition required | open |  | 2026-08-19T00:22:26.401Z |  |
| 2 | 12 | deviation | internal/acp/framer.go | 34 | Pre-existing, surfaced by the 12-01 live witness: the Writer decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023) - operator disposition required | open |  | 2026-08-19T00:22:32.786Z |  |
| 3 | 12 | stub | internal/session/ask.go |  | D-01 timer-driven resume: the resumed turn runs server-side and is transcript-recorded, but its chunks are not mirrored to the ACP client (no active Run subscription at fire time); the reply path IS fully client-visible | open |  | 2026-08-19T00:22:32.985Z |  |

````json
[
  {
    "id": 1,
    "kind": "deviation",
    "phase": "12",
    "file": "internal/acp/framer.go",
    "line": 34,
    "description": "Pre-existing, surfaced by the 12-01 live witness: the Writer's decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023; 450 transcript vs 440 wire chunks reconciled). Relaxing it reverses a documented v1.0 transport decision — operator disposition required",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-19T00:22:26.401Z",
    "resolved_at": null
  },
  {
    "id": 2,
    "kind": "deviation",
    "phase": "12",
    "file": "internal/acp/framer.go",
    "line": 34,
    "description": "Pre-existing, surfaced by the 12-01 live witness: the Writer decoded-newline transport guard silently DROPS the model's own newline-carrying text chunks from the live wire (9 dropped in witness session d9f98023) - operator disposition required",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-19T00:22:32.786Z",
    "resolved_at": null
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
  }
]
````
