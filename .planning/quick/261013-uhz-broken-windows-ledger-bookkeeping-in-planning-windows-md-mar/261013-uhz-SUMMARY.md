---
phase: 261013-uhz
plan: 01
subsystem: planning-ledger
tags: [broken-windows, bookkeeping, windows-ledger, uat-discharge]
requires: ["Phase-23 UAT evidence (23-UAT-evidence-2026-09-20.md)", "commit ed83044 (TestGitRun_PipesBothChildStdioFds regression pin)"]
provides: ["WINDOWS.md rows #23/#24 discharged fixed-with-reason", "WINDOWS.md row #30 (stdout-discipline observation, fixed-with-reason)", "three-way ledger agreement (frontmatter / fenced JSON / rendered table)"]
affects: ["/gsd-ship window gate (workflow.windows_enforce)", "v1.3 milestone planning"]
tech-stack:
  added: []
  patterns: ["gsd-tools windows verbs for all structural ledger mutations", "hand-filled reasons mirrored byte-exact in JSON + table surfaces"]
key-files:
  created: []
  modified: [".planning/WINDOWS.md"]
decisions:
  - "Row #30 resolved_at pinned to the git-verified ed83044 closure instant 2026-09-27T18:15:27Z (author date per batch plan-checker), not the bookkeeping write time — the closure event is the commit"
  - "Rows #23/#24 keep tool-set write-time resolved_at (row #29 precedent); the 2026-09-20 discharge date lives in the dated 260920-uat reasons"
  - "[Rule 3] Pre-existing row #29 surface drift repaired by syncing the deliberate 260910-24-06 narrative INTO the JSON entry (table stays byte-identical to its committed form) — additive, restores the module's table-equals-renderTable(entries) invariant"
metrics:
  duration: ~8 min
  completed: 2026-10-13
  tasks: 3
  commits: 0 (coordinator-owned per quick-batch constraints)
status: complete
---

# Quick Item 261013-uhz: Broken Windows ledger bookkeeping Summary

Discharged Phase-23 UAT windows #23/#24 as fixed-with-reason (operator UAT 2/2 PASS, 2026-09-20), recorded the 23-UAT stdout-discipline observation as fixed row #30 citing commit ed83044 and TestGitRun_PipesBothChildStdioFds, and brought the ledger to three-way agreement at open 16 / waived 0 / fixed 14 / total 30.

## What Was Done

- **Task 1 — tool-side mutations:** `windows fixed 23`, `windows fixed 24`, `windows append` (kind deviation, phase 23, file kit/checkpoint/store.go, description verbatim from the plan; tool minted id 30), `windows fixed 30`. Counts recomputed by the tool: 16/0/14/30 as predicted.
- **Task 2 — resolution narratives:** filled dated fixed-resolution reasons in BOTH surfaces (JSON entry field + identical table cell), avoiding pipe/backslash/backtick characters so the byte-exact table invariant holds by construction:
  - #23 `260920-uat:` operator Phase-23 UAT 2/2 PASS (2026-09-20), Test 1 — live-Zed boundary steering + idle/mid-turn /undo incl. auto-cancel-then-restore, real Zed 1.20.2 + Zed-shaped ACP driver; cites 23-UAT-evidence-2026-09-20.md + STATE.md Phase 23 entry.
  - #24 `260920-uat:` same UAT, Test 2 — two-live-session cross-session restore check PASS (/undo from idle refuses naming the busy session); same evidence pointers.
  - #30 `260927-close:` non-reproducing on v1.2 release code (fresh-store acp serve probe stdout byte-clean), gitRun seam piped both child fds since the store's first commit; closed by ed83044 adding TestGitRun_PipesBothChildStdioFds at kit/checkpoint/store_test.go:1475. resolved_at pinned to `2026-09-27T18:15:27Z` in both surfaces.
  - Frontmatter last_updated refreshed to `2026-10-13T21:11:42.071Z`.
- **Task 3 — consistency gate:** `windows status` exits 0; byte-exact check `extractTableRegion(raw, total) === renderTable(entries)` passes; row assertions pass (30 entries, 16/0/14; #23/#24/#30 fixed-with-reason; #30 reason cites ed83044 + TestGitRun_PipesBothChildStdioFds; #21/#22/#27 still open). Grep counts exactly 4/2/2/2 as the plan's verification requires.

## Ledger Deltas

| Row | Before | After |
|-----|--------|-------|
| #23 | open, unmet-truth, no reason | fixed, 260920-uat reason, resolved_at 2026-10-13T21:09:45.478Z (write time) |
| #24 | open, unmet-truth, no reason | fixed, 260920-uat reason, resolved_at 2026-10-13T21:09:45.699Z (write time) |
| #30 | did not exist | deviation, phase 23, kit/checkpoint/store.go, fixed, 260927-close reason, resolved_at 2026-09-27T18:15:27Z |
| #29 | JSON reason "" vs table narrative (latent drift) | JSON reason synced to the committed 260910-24-06 narrative (see Deviations) |
| frontmatter | open 18 / waived 0 / fixed 11 / total 29 | open 16 / waived 0 / fixed 14 / total 30, last_updated refreshed |

## Deviations from Plan

**[Rule 3 - Blocking issue] Pre-existing row #29 table/JSON surface drift repaired (JSON entry #29 reason synced)**
- **Found during:** Task 1 pre-flight
- **Issue:** Commit cba127a (24-06, 2026-09-10) hand-edited row #29 and added the resolution narrative to the TABLE row only; JSON entry #29 kept `"reason": ""`. The ledger has carried this latent drift since then: `extractTableRegion(raw) !== renderTable(entries)`. The write path drift-guards (`WINDOWS_LEDGER_TABLE_DRIFT`, broken-windows.cjs:862), so every `windows fixed`/`windows append` invocation would have aborted — Task 1 was impossible without repair, and Task 3's byte-exact gate could never pass.
- **Fix:** Inserted the exact table-cell narrative (`260910-24-06: the premise was FALSE — phase 22 executed 2026-09-08..09-10 … see 24-06-SUMMARY`) into JSON entry #29's `reason` field. Direction chosen: sync INTO the JSON (not blank the table) because the 24-06 commit message ("WINDOWS entry #29 resolved") and the dated narrative format prove the reason was deliberate; the sync is additive, restores the module's "table = rendering of JSON" invariant, changes no row's meaning, and leaves the table byte-identical to its committed form. No counts affected. Scope-fence rows #21/#22/#27 verified byte-identical to HEAD in both surfaces after all changes.
- **Files modified:** .planning/WINDOWS.md (JSON entry #29 reason field only)
- **Commit:** none yet — coordinator-owned per quick-batch constraints

## Notes for the Coordinator (out-of-scope observations, not fixed)

- **gsd-core quirk:** `--pick` fails for ALL `windows` verbs with "command output was not JSON" — `broken-windows.cjs` emits via `process.stdout.write` while the `--pick` capture path only patches `fs.writeSync`, so the capture is empty. The plan's Task 1 verify snippet and one Task 3 leg used `--pick ledger.*_count`; both were adapted to parse the plain JSON output (same assertions, identical strictness). Worth an upstream gsd-core ticket.
- The pinned #30 resolved_at `2026-09-27T18:15:27Z` is ed83044's AUTHOR date per the batch plan-checker; used verbatim without re-deriving from %ci.

## Verification

- `node gsd-tools.cjs windows status --cwd <root>` exits 0 (ship-gate read validator green).
- Byte-exact: `extractTableRegion(raw, 30) === renderTable(entries)` → true.
- Counts: 30 entries, open 16 / waived 0 / fixed 14.
- Rows #23/#24/#30 fixed with non-empty reason + resolved_at; #30 reason cites ed83044 and TestGitRun_PipesBothChildStdioFds.
- Rows #21/#22/#27: open, empty reasons, table rows AND JSON entries byte-identical to HEAD.
- Grep counts: `260920-uat:` ×4, `260927-close:` ×2, `ed83044` ×2, `TestGitRun_PipesBothChildStdioFds` ×2.
- `"resolved_at": "2026-09-27T18:15:27Z"` present in JSON; row 30 table cell carries the same instant.

## Self-Check: PASSED

- .planning/WINDOWS.md modified with all Task 1-3 changes: FOUND (git diff shows frontmatter + rows 23/24/29/30 hunks only).
- All three task verify gates re-run green after every mutation; final TASK3-OK gate passed after the last edit.
