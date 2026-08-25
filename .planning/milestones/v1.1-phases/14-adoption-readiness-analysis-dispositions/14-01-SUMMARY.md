---
phase: 14-adoption-readiness-analysis-dispositions
plan: 01
subsystem: infra
tags: [shadow-git, checkpoints, undo, git-subprocess, earli-01, reversibility]

# Dependency graph
requires:
  - phase: 08-09-between-turn-boundaries
    provides: the turn-boundary machinery (Prompt lifecycle, nextTurnID) the snapshot seam hooks without redefining
provides:
  - internal/checkpoint.Store — the EARLY-01 shadow-git checkpoint store (Open/Snapshot/List/Restore, DefaultKeep=50 retention, O_EXCL whole-store lock, strict id validation before any exec)
  - The Session.Checkpointer seam (snapshot at parent-turn START; failure loud but never turn-fatal; nil = disabled)
  - `ass-guard checkpoint list|restore [--work-dir]` CLI (stderr-only output)
  - Serve wiring at sessionFor — DEFAULT ON, loud-degrade on store failure (AUD-03 pattern)
  - The gated live rollback demonstration + operator runbook (testdata/checkpoint-e2e/README.md)
affects: [14-02 compaction verify, 14-04 cache probe, adoption line, v1.2 Telegram replan]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
actuals:
  tokens: 78000    # chars/4 over the 10 changed files (store+tests+seam+CLI+wiring+README)
  tasks: 3
  commits: 5

# Tech tracking
tech-stack:
  added: []   # zero new dependencies — system git subprocess only (go.mod/go.sum untouched, verified)
  patterns:
    - "Shadow-git store recipe: bare-layout init + core.bare=false + symbolic-ref HEAD refs/checkpoints/last (a plain `git init <dir>.git` nests .git inside on current git)"
    - "Git subprocess isolation env: --git-dir/--work-tree pair + own GIT_INDEX_FILE + GIT_CONFIG_GLOBAL/SYSTEM=/dev/null + GIT_TERMINAL_PROMPT=0 + -c core.hooksPath=/dev/null"
    - "Commit-then-update-ref split: a checkpoint ref exists only after its commit object (partial-ref guarantee, testable via the unexported commitSnapshot step)"
    - "Typed-nil interface guard at wiring: assign the adapter only when the store is non-nil"

key-files:
  created:
    - internal/checkpoint/store.go
    - internal/checkpoint/store_test.go
    - internal/session/checkpoint_seam_test.go
    - cmd/ass-guard/checkpoint.go
    - cmd/ass-guard/checkpoint_test.go
    - cmd/ass-guard/testdata/checkpoint-e2e/README.md
  modified:
    - internal/session/session.go
    - cmd/ass-guard/main.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/acp_serve_test.go

key-decisions:
  - "Bare-layout git init + core.bare=false for the shadow store (plain `git init shadow.git` creates shadow.git/.git on git 2.50 — discovered and fixed in GREEN)"
  - "Snapshot at Prompt entry right after nextTurnID, BEFORE hooks and the user message — restore is exactly 'undo this turn'; subagent turns never snapshot (they run inside the parent's recovery window)"
  - "Serve wiring DEFAULT ON with no v1 flag; store-open failure degrades loud to a session WITHOUT checkpointing (never a serve refusal)"
  - "checkpointerAdapter at the cmd wiring site bridges Store.Snapshot to the seam's SnapshotTurn — internal/session keeps no import of internal/checkpoint (OnClose func-seam pattern)"
  - "TestServeMirror_Override exit condition moved to the turn's terminal transcript line (see Deviations — the turn-entry snapshot lengthens turns and exposed a pre-existing early-return race)"

patterns-established:
  - "Shadow-git checkpoint discipline: strict id grammar validated BEFORE any git exec; refs confined to refs/checkpoints/; the gitRun package-var seam counts invocations in tests"
  - "Gated live E2E: ASSGUARD_* env gates with loud t.Skipf naming every gate, runbook committed beside the test"

requirements-completed: [EARLY-01]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "Shadow-git checkpoint store: snapshot at turn boundaries into <workDir>/.ass-guard/checkpoints/shadow.git, byte-identical restore, retention, lock, perms, ref validation"
    requirement: EARLY-01
    verification:
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestSnapshotRestore_ByteIdentical
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestSnapshot_UserGitUntouched
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestRestore_UserGitUntouched
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestCheckpointListOrdering
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestSnapshot_NoChangesStillCommits
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestSnapshot_IdempotentSameTurn
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestSnapshot_InterruptedNeverExposesPartialRef
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestConcurrentSnapshotRestoreSerialize
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestRetentionPrune
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestStorePerms
        status: pass
      - kind: unit
        ref: internal/checkpoint/store_test.go#TestRestore_RejectsBadIds
        status: pass
    human_judgment: false
  - id: D2
    description: "Session seam: every parent turn snapshots at entry BEFORE any turn work; snapshot failure is loud but never turn-fatal; nil checkpointer = disabled"
    requirement: EARLY-01
    verification:
      - kind: unit
        ref: internal/session/checkpoint_seam_test.go#TestPrompt_SnapshotsAtTurnEntry
        status: pass
      - kind: unit
        ref: internal/session/checkpoint_seam_test.go#TestPrompt_CheckpointFailureNonFatal
        status: pass
    human_judgment: false
  - id: D3
    description: "`ass-guard checkpoint list|restore` CLI with --work-dir resolution; all output on stderr (stdout stays reserved for ACP frames)"
    requirement: EARLY-01
    verification:
      - kind: unit
        ref: cmd/ass-guard/checkpoint_test.go#TestCheckpointListEmptyStore
        status: pass
      - kind: unit
        ref: cmd/ass-guard/checkpoint_test.go#TestCheckpointListShowsSnapshots
        status: pass
      - kind: unit
        ref: cmd/ass-guard/checkpoint_test.go#TestCheckpointRestoreRoundtripCLI
        status: pass
      - kind: unit
        ref: cmd/ass-guard/checkpoint_test.go#TestCheckpointRestoreRejectsBadID
        status: pass
    human_judgment: false
  - id: D4
    description: "Serve wiring: sessionFor opens the store against the resolved WorkDir and wires Session.Checkpointer — DEFAULT ON, loud-degrade on failure"
    requirement: EARLY-01
    verification:
      - kind: integration
        ref: cmd/ass-guard/checkpoint_test.go#TestSessionFor_WiresCheckpointer
        status: pass
    human_judgment: false
  - id: D5
    description: "Gated live rollback demonstration: a real mutating model turn in a scratch git repo, restored byte-identically with the user repo's .git untouched"
    requirement: EARLY-01
    verification: []
    human_judgment: true
    rationale: "The live leg requires operator credentials (ZAI_API_KEY) absent at execution time; the test skips loud naming both gates. The offline battery (D1) proves the guarantee directly; the live run remains an operator-executable backstop (WINDOWS.md unrun-verify entry + testdata/checkpoint-e2e/README.md command)."

# Metrics
duration: 47min
completed: 2026-08-19
status: complete
---

# Phase 14 Plan 01: Shadow-git checkpoints + undo Summary

**Shadow-git checkpoint store (external git subprocess, isolated env) + turn-entry Session seam + `ass-guard checkpoint list|restore` CLI + default-ON serve wiring — a mutating turn is now recoverable to byte-identical pre-turn state with the user's `.git` untouched**

## Performance

- **Duration:** 47 min
- **Started:** 2026-08-19T17:09:53Z
- **Completed:** 2026-08-19T17:57:10Z
- **Tasks:** 3
- **Files modified:** 10

## Accomplishments

- `internal/checkpoint.Store` — the EARLY-01 backstop: Snapshot at turn boundaries into one shadow git repo per workspace (`refs/checkpoints/<sessionID>-turn-<NNN>`), byte-identical Restore, `DefaultKeep=50` retention prune, O_EXCL whole-store lock (30s stale steal), 0700 store dirs, strict id grammar validated BEFORE any git exec (T-14-01)
- The core invariant is pinned twice: `TestSnapshot_UserGitUntouched` + `TestRestore_UserGitUntouched` prove the USER repo's HEAD ref bytes, index bytes, porcelain status, and recursive `.git` tree are byte-identical across snapshot AND restore (T-14-04)
- `Session.Checkpointer` seam: snapshot fires at Prompt entry BEFORE hooks/user message/provider (sequenced by test), failure degrades loud (slog + investigate-and-fix transcript line) and never kills the turn (AUD-03 discipline)
- `ass-guard checkpoint list|restore` CLI — stderr-only (zero `os.Stdout` references in the file); serve wiring DEFAULT ON at sessionFor with loud degrade to no-checkpointing on store failure
- The full invariant battery green under `-race`: empty-turn commits, idempotent re-snapshot, interruption never exposes a partial ref, concurrent snapshot+restore serialize, retention, perms, bad-id rejection with ZERO git spawns

## Task Commits

Each task was committed atomically (TDD RED→GREEN per task):

1. **Task 1: End-to-end snapshot→list→restore (tracer)** - `670a167` (test, RED) + `3afdd8e` (feat, GREEN)
2. **Task 2: The invariant battery** - `5a1989f` (test; no fixes surfaced — Task 1's action already carried the threat-model mitigations, honestly recorded in the commit body)
3. **Task 3: Serve wiring + gated live demonstration** - `425825a` (test, RED) + `d154d49` (feat, GREEN)

**Plan metadata:** (final docs commit below)

## Files Created/Modified

- `internal/checkpoint/store.go` — the Store: Open (bare-layout init + core.bare=false, 0700), Snapshot/List/Restore, isolation env, retention, file lock
- `internal/checkpoint/store_test.go` — the 11-test battery (byte-identity, user-git-untouched ×2, ordering, empties, idempotency, interruption, concurrency, retention, perms, ref validation)
- `internal/session/session.go` — `Checkpointer` interface + field + the turn-entry call site
- `internal/session/checkpoint_seam_test.go` — seam ordering + non-fatal failure
- `cmd/ass-guard/checkpoint.go` — the CLI subcommands + `checkpointerAdapter`
- `cmd/ass-guard/checkpoint_test.go` — CLI tests + serve wiring test + the gated live leg
- `cmd/ass-guard/main.go` — `newCheckpointCmd()` registration
- `cmd/ass-guard/acp_serve.go` — sessionFor wiring (default ON, loud degrade)
- `cmd/ass-guard/acp_serve_test.go` — TestServeMirror_Override exit-condition fix (see Deviations)
- `cmd/ass-guard/testdata/checkpoint-e2e/README.md` — the gated live rollback runbook

## Decisions Made

- **Bare-layout init + `core.bare=false`**: `git init <dir>.git` on git ≥2.50 nests `.git` INSIDE the named dir; the bare-layout init immediately flipped non-bare is the classic shadow-repo recipe and works with the external `--git-dir`/`--work-tree` pair
- **`refs/checkpoints/last` symbolic HEAD**: every snapshot commit chains off the previous one; `last` is excluded from List/prune (not a turn checkpoint)
- **Typed-nil guard at wiring**: `s.Checkpointer = checkpointerAdapter{...}` assigned only when the store opened — a typed-nil `*Store` in the interface would satisfy `!= nil` and panic on SnapshotTurn
- **Task 2 committed test-only**: the battery passed on arrival because Task 1's plan text already prescribed the lock/validation/prune/perms implementation; recorded honestly rather than manufacturing a fake RED

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] TestServeMirror_Override flake introduced by the turn-entry snapshot**
- **Found during:** Task 3 (full cmd suite)
- **Issue:** The pre-existing test returned at the mid-turn `request_shaped` mirror line while the turn's async writers (audit body-store shard dirs) were still active; the checkpoint snapshot lengthens every turn, widening that window — measured 2/10 failures with the wiring vs 0/10 on baseline `ce46cba` (verified in a detached worktree, since removed)
- **Fix:** The test now also waits for the turn's terminal transcript line (`"type":"assistant_message"` on the session transcript — the mirror never carries it, it subscribes request_shaped/engine_decision/usage only) plus a 150ms async-writer drain (the TestEndToEndSession flush pattern); its original assertions are unchanged
- **Files modified:** cmd/ass-guard/acp_serve_test.go
- **Verification:** 0/15 failures after the fix; full `go test ./cmd/ass-guard/... -count=1` green; `mise run ci` green
- **Committed in:** d154d49 (Task 3 commit)

**2. [Rule 3 - Blocking] git ≥2.50 nested-init behavior**
- **Found during:** Task 1 GREEN
- **Issue:** `git init shadow.git` creates `shadow.git/.git/` on current git (2.50.1) — the shadow repo was never where the code looked
- **Fix:** `git init --bare` + `git config core.bare false` + `symbolic-ref HEAD refs/checkpoints/last` (verified end-to-end manually before re-running the suite)
- **Files modified:** internal/checkpoint/store.go
- **Verification:** All store tests green
- **Committed in:** 3afdd8e (Task 1 GREEN)

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** Both fixes necessary for correctness on current toolchains; no scope creep.

## TDD Gate Compliance

All three tasks carry `tdd="true"`; RED commits precede GREEN commits for every behavior-adding task:

| Task | RED | GREEN | Notes |
|------|-----|-------|-------|
| 1 (tracer) | `670a167` (10 failing tests, evidence in body) | `3afdd8e` | tracer feedback gate re-run GREEN before expansion |
| 2 (battery) | `5a1989f` (first-run-green, honestly recorded — no implementation followed) | — | the battery PINS existing Task-1 behavior per the plan's "(fixes folded in)" wording |
| 3 (wiring) | `425825a` (wiring FAIL + gated SKIP, evidence in body) | `d154d49` | gated leg cannot RED (skips without credentials) — the wiring test drives the cycle |

## Issues Encountered

- **Gated live rollback NOT executed this session** (plan-anticipated path): `ZAI_API_KEY` is absent from the environment and the repo carries no `.ass-guard/config.yaml`, so `TestCheckpointLiveRollback_Gated` skips loud (naming `ASSGUARD_CHECKPOINT_E2E` and `ZAI_API_KEY`); `ASSGUARD_CHECKPOINT_E2E=0` run verified exit-0-with-skip. The exact operator command is recorded in `cmd/ass-guard/testdata/checkpoint-e2e/README.md` and in WINDOWS.md (unrun-verify). The offline `UserGitUntouched` tests prove the invariant directly (the must_haves backstop arrangement).

## User Setup Required

None — no external service configuration required. (The optional live demonstration needs `ASSGUARD_CHECKPOINT_E2E=1` + `ZAI_API_KEY` per the runbook.)

## Next Phase Readiness

- EARLY-01's store, seam, CLI, and default-ON wiring are complete and test-pinned; `mise run ci` green; go.mod/go.sum untouched (zero-dep invariant verified over the plan's full commit range)
- Ready for 14-02 (compaction verification — reads the untouched 08-09 between-turn machinery, which this plan did not modify) and the rest of wave 1
- Open item: the gated live rollback run (operator, one command, runbook-committed)

## Self-Check: PASSED

- Files exist: internal/checkpoint/store.go, internal/checkpoint/store_test.go, internal/session/checkpoint_seam_test.go, cmd/ass-guard/checkpoint.go, cmd/ass-guard/checkpoint_test.go, cmd/ass-guard/testdata/checkpoint-e2e/README.md — all FOUND
- Commits found on branch: 670a167, 3afdd8e, 5a1989f, 425825a, d154d49 — all FOUND via git log
- Verification re-run at close: `go test ./internal/checkpoint/... -count=1 -race` green; `go test ./internal/session/... ./cmd/ass-guard/... -count=1` green; `git diff <base>..HEAD -- go.mod go.sum` EMPTY

---
*Phase: 14-adoption-readiness-analysis-dispositions*
*Completed: 2026-08-19*
