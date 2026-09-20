---
phase: 23-seed-gaps-close-out
plan: 03
subsystem: checkpoint
tags: [checkpoint, gc, git, restore-guard, nested-repos, exclude]

requires:
  - phase: 14 (EARLY-01 shadow-git store)
    provides: Store/Snapshot/Restore/prune/withLock, validate-before-refspec gate
provides:
  - Store.SnapshotPreRestore — pre-restore snapshot family (<sessionID>-pre-<NNN>), deterministic counter, fail-closed error surface
  - Three-site id grammar accepting BOTH families (idPattern/validateTurnID/parseRefLine), Entry.Kind
  - findNestedRepos (dir + .git-file variants) + NestedRepoError + Store.RestoreGuard
  - Store.Sweep (age+count dual axis, ref deletion + reflog expire + gc --prune=now in one withLock) + pure expireByAge/expireByCount
  - Store.EnsureUserRepoExclude — idempotent .ass-guard/ append to the user repo's .git/info/exclude with typed skips
  - De-chained snapshot commits (commit-tree parented on the init anchor) making object reclamation possible
affects: [23-04, 23-05]

actuals:
  tokens: 14800
  tasks: 3
  commits: 7

tech-stack:
  added: []
  patterns:
    - "three coupled grammar sites move in one commit (Pitfall 6 discipline)"
    - "dual-axis GC victim selection as pure table-testable helpers + git side effects in one withLock"

key-files:
  created: []
  modified:
    - internal/checkpoint/store.go
    - internal/checkpoint/store_test.go

key-decisions:
  - "De-chained snapshot commits: commitSnapshot now uses write-tree + commit-tree parented DIRECTLY on the init anchor (refs/checkpoints/last, never advanced). The old git-commit-on-HEAD chaining anchored ALL history at last, so deleting a checkpoint ref could never make its objects unreachable — gc reclaimed nothing (verified empirically: 315 loose objects became 315 packed, zero pruned). D-08's 'the store stops growing' is impossible under chaining; the de-chain makes Sweep's object expiry real. Rule-1 deviation, fully documented at commitSnapshot."
  - "The exactly-maxAge boundary survives (expireByAge uses strictly-Before); age-axis fixture carries a 5s scheduling-skew allowance because the snapshot git calls cost seconds against Sweep's fresh now"
  - "expireByCount counts BOTH families per session (pre-restore entries share the lifecycle — D-09 one store, one lifecycle); recency = CommittedAt with TurnNum/Ref tie-breaks"
  - "prune stays as the between-sweeps global backstop; Sweep is the retention authority — relationship documented in prune's comment (no double-delete: both remove only existing refs, idempotently)"
  - "Top-level .git (the user's own repo or a worktree workspace) is never flagged by findNestedRepos — only entries BELOW the top level count; a .git symlink counts as the file variant (conservative refusal)"
  - "Typed skips for the exclude append (ErrExcludeSkippedNotRepo / ErrExcludeSkippedWorktree) are caller-log notes, never failures (AUD-03)"

patterns-established:
  - "NestedRepoError typed refusal: the store reports and refuses; the composition layer (23-04/23-05) owns the policy decision"
  - "read-check-append exclude discipline: MkdirAll info 0700, O_APPEND|O_CREATE 0600, never truncate, exact-line idempotence check"

requirements-completed: [SEEDG-02]

coverage:
  - id: D1
    description: "Pre-restore snapshots are first-class checkpoint objects: three-site grammar, restart-deterministic counter, byte-identical restore round trip"
    requirement: SEEDG-02
    verification:
      - kind: unit
        ref: "internal/checkpoint/store_test.go#TestPreRestoreSnapshotFamily"
        status: pass
      - kind: unit
        ref: "internal/checkpoint/store_test.go#TestIDGrammarTable"
        status: pass
    human_judgment: false
  - id: D2
    description: "Nested repos (dir and .git-file variants) detected and refused with named paths; clean never descends; symlinks bounded"
    requirement: SEEDG-02
    verification:
      - kind: unit
        ref: "internal/checkpoint/store_test.go#TestNestedRepoDetectedAndRefused"
        status: pass
      - kind: unit
        ref: "internal/checkpoint/store_test.go#TestRestoreCleanNeverDescendsIntoNestedRepos"
        status: pass
    human_judgment: false
  - id: D3
    description: "Age+count GC with real object expiry under one lock; no cross-session eviction"
    requirement: SEEDG-02
    verification:
      - kind: unit
        ref: "internal/checkpoint/store_test.go#TestCheckpointGCAgeAxis"
        status: pass
      - kind: unit
        ref: "internal/checkpoint/store_test.go#TestCheckpointGCCountAxis"
        status: pass
      - kind: unit
        ref: "internal/checkpoint/store_test.go#TestCheckpointGCObjectExpiry"
        status: pass
    human_judgment: false
  - id: D4
    description: "User repo's .git/info/exclude carries .ass-guard/ idempotently with existing lines preserved; check-ignore fires; non-repo/worktree workdirs skip loudly"
    requirement: SEEDG-02
    verification:
      - kind: unit
        ref: "internal/checkpoint/store_test.go#TestUserRepoExclude"
        status: pass
    human_judgment: false

duration: 42 min
completed: 2026-09-08
status: complete
---

# Phase 23 Plan 03: Checkpoint Store Guards Summary

**The four store-owned SEEDG-02 guards: pre-restore snapshot family, nested-repo refusal, dual-axis GC with real object expiry, and the idempotent user-repo exclude append.**

## Performance

- **Duration:** 42 min
- **Tasks:** 3/3 (each RED→GREEN under TDD)
- **Files:** 2 modified (store.go +1240 lines region, store_test.go +860)

## Accomplishments

- **Task 1 (grammar + pre-restore family):** idPattern/validateTurnID/parseRefLine now accept both `-turn-` and `-pre-` families (all three sites in one commit — Pitfall 6); Entry.Kind carries the family with a deterministic same-number tie-break; `SnapshotPreRestore(ctx, sessionID)` mints `<sessionID>-pre-<NNN>` (counter seeded by max+1 ref scan — restart-deterministic), commits through the same commitSnapshot/updateRef/prune path, and returns the id for the caller's restore composition (fail-closed).
- **Task 2 (nested-repo refusal):** `findNestedRepos` walks with lstat semantics (symlinks never followed — no cycles, bounded cost), flags .git as dir AND file below the top level, skips the store root; `RestoreGuard` returns the typed `*NestedRepoError` naming paths + reason (gitlink contents silently unprotected); the clean-vocabulary test pins that restore's clean never carries `-ff`.
- **Task 3 (GC + exclude + de-chain):** `Sweep(ctx, maxAge, perSession)` with pure `expireByAge` (strictly-older) / `expireByCount` (per-session newest-N) victim selection; ref deletion + `reflog expire --expire=now --all` + `gc --prune=now` inside one withLock; `EnsureUserRepoExclude` appends `.ass-guard/` to the user repo's `.git/info/exclude` idempotently with typed skips for non-repo/worktree workdirs.

## Task Log

| Task | Commit | Verification |
|------|--------|--------------|
| T1 pre-restore family + grammar | RED 817ca1c → GREEN 624533b | TestPreRestoreSnapshotFamily + TestIDGrammarTable green |
| T2 nested-repo detection + refusal | RED ccffb8a → GREEN 78d5b92 | TestNestedRepo* green (dir + .git-file + symlink + clean vocabulary) |
| T3 GC sweep + exclude + de-chain | RED 1c61078 → GREEN fb73a4a (+doc 09aa992) | TestCheckpointGC* + TestUserRepoExclude green; whole package -race green |

## Deviations from Plan

- **[Rule 1 - bug] De-chained snapshot commits (commitSnapshot rewrite)** — Found during: Task 3 | Issue: the plan's object-expiry criterion assumed `gc --prune=now` reclaims deleted refs' objects; empirically the store's `git commit`-on-HEAD→last chaining anchored ALL history at refs/checkpoints/last, so victims stayed reachable — 315 loose objects became 315 packed, zero pruned, and D-08's "store stops growing" was unachievable. | Fix: commitSnapshot now parents every snapshot DIRECTLY on the init anchor via write-tree + commit-tree (10-line plumbing change; no pinned test depended on chaining; idempotent re-snapshot and zero-change commits preserved). TestCheckpointGCObjectExpiry now proves the total object count DROPS. | Files: store.go | Verification: whole package -race green | Commit: fb73a4a
- **[Note - fixture skew] The exactly-maxAge fixture carries a +5s allowance** — the snapshot git calls cost seconds and Sweep stamps a fresh now; a hard now-maxAge stamp is strictly older than the cutoff by exactly the drift. Documented inline; the strictly-older-evicts property is pinned by the 30d/20d refs.

**Total deviations:** 1 auto-fixed (Rule 1), 1 fixture note. **Impact:** the de-chain is load-bearing for D-08 — without it every future checkpoint ever taken stays in the pack forever.

## Issues Encountered

None beyond the deviations above.

- `go vet ./internal/checkpoint/` clean; `go test -race -count=1 ./internal/checkpoint/` fully green (existing battery included).
- golangci-lint remains environmentally broken (typecheck panics on Go 1.27.1 stdlib — pre-existing, STATE.md LINT BASELINE).

## Self-Check: PASSED
