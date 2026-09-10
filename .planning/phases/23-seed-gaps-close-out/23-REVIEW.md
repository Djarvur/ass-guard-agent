---
phase: 23-seed-gaps-close-out
reviewed: 2026-09-10T12:00:55Z
depth: standard
files_reviewed: 16
files_reviewed_list:
  - internal/session/steerqueue.go
  - internal/session/steerqueue_test.go
  - internal/session/steering_test.go
  - internal/session/transcript.go
  - internal/session/manager.go
  - internal/session/session.go
  - internal/session/projector.go
  - internal/runtime/steering_ingress_test.go
  - internal/runtime/runtime.go
  - internal/session/ask.go
  - internal/checkpoint/store.go
  - internal/checkpoint/store_test.go
  - internal/runtime/restore_guard_test.go
  - internal/acpserve/config_surface.go
  - internal/runtime/commands.go
  - internal/runtime/commands_test.go
findings:
  critical: 1
  warning: 3
  info: 5
  total: 9
status: issues_found
---

# Phase 23: Code Review Report

**Reviewed:** 2026-09-10T12:00:55Z
**Depth:** standard
**Files Reviewed:** 16
**Status:** issues_found

## Summary

Phase 23's deliverables were reviewed at standard depth: the SteerQueue + boundary-drain steering pipeline (session/runtime/projector), the checkpoint hardening (Runner-owned store, restore guard, session-start GC, configOptions), and `/undo` (D-11 recency walk, D-12 auto-cancel-then-restore). The core happy paths are solid and unusually well-tested (ticket conservation under `-race`, pair-safety/anchor-safety projector pins, the fail-closed ordering battery); `go build`, `go vet`, and every phase-23 test pass under `-race` locally.

The adversarial pass nonetheless found one Critical defect and three Warnings. The Critical: the restore guard enforces the "restore never happens under an active turn/chain" invariant **per session only**, while the checkpoint store and the workspace it force-checkouts are **shared across all sessions of the process** — a `/undo` in one session can run `checkout -f` + `clean -fd` underneath another session's live turn, destroying that turn's in-flight work. The Warnings: a genuine race hole in the anti-zombie steering invariant at the turn-death boundary (non-atomic `CancelAll` + pre-mutex enqueue), a retention backstop (`prune`) that evicts by session-name grammar order rather than recency, and a blob-fill inconsistency in the new checkpoint/background config options.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: Restore guard is session-scoped, but the workspace and store it mutates are process-shared — restore can run under ANOTHER session's live turn

**File:** `internal/runtime/runtime.go:1995-2005` (`restoreBlockers`), composed at `internal/runtime/runtime.go:1182-1219` (`restoreUndoActive`) and `internal/runtime/commands.go:1283-1304` (`restoreUndo`); mutating half at `internal/checkpoint/store.go:556-583` (`Store.Restore`)

**Issue:** `restoreBlockers` refuses only while `clientTurnActive(sessionID)` or `chainCount(sessionID)` is set — the **calling** session's state. But all sessions of one Runner share `r.workDir` (fixed at startup; `sessionFor` opens every Manager and attaches the Checkpointer to the same workspace store), and turn serialization is per-session (`turnMus sync.Map`, `runtime.go:278`). Two ACP sessions run turns concurrently on different mutexes; a `/undo` in idle session B passes the guard while session A's turn is mid-flight, then `Store.Restore` runs `git checkout --no-overlay -f` plus `git clean -fd` over the shared worktree — deleting files session A's turn created since B's snapshot and clobbering files it is actively writing. The store lock (`withLock`) serializes only store-vs-store operations; tool execution (coreexec writes) never takes it. The active turn then snapshots the torn tree at its next boundary, laundering the corruption into a "valid" checkpoint.

The phase invariant — "restore never happens under an active turn/chain" — is stated unscoped, and the consequence is the exact data-loss class the guard exists to prevent (mid-turn work destroyed while the operator is told "undo complete"). The doc comment's scope note ("The v1.1 CLI restore path is a separate process…") covers cross-process detection only; this hole is entirely in-process, where detection is trivially available.

**Fix:** Make `restoreBlockers` refuse when ANY session of the Runner has activity (naming the calling session's own state in the message for operator clarity):

```go
func (r *Runner) restoreBlockers(sessionID string) error {
	if r.clientTurnActive(sessionID) {
		return &restoreBlockedError{turn: true}
	}
	// Workspace-wide: another session's live turn is equally unsafe to
	// restore under — the store force-checkouts the SHARED worktree.
	otherBusy := false
	r.turnActive.Range(func(k, v any) bool {
		if k != sessionID && v.(*atomic.Bool).Load() {
			otherBusy = true
			return false
		}
		return true
	})
	if otherBusy {
		return &restoreBlockedError{turn: true}
	}
	if r.chainCount(sessionID) > 0 {
		return &restoreBlockedError{chain: true}
	}
	if n := r.anyChainCount(sessionID); n > 0 { // same walk over activeChains, excluding sessionID
		return &restoreBlockedError{chain: true}
	}
	return nil
}
```

At minimum, if the session-scoped behavior is a deliberate pinned decision, it must be documented at the guard AND surfaced in `/undo`'s output ("another session of this workspace is active — restore may race it"), because today it silently violates the stated invariant.

## Warnings

### WR-01: Anti-zombie steering invariant has a turn-death race — non-atomic `CancelAll` plus pre-mutex enqueue can land steering in an unrelated later turn

**File:** `internal/session/steerqueue.go:133-143` (`CancelAll` two-phase locking); `internal/runtime/runtime.go:1046-1081` (`routeSteering` enqueue), `internal/session/session.go:1222-1233` (`recordCanceled`), `internal/runtime/runtime.go:1674-1686` (`unregisterActiveTurn`)

**Issue:** `CancelAll` reads `q.next` under one lock acquisition, releases, then delegates to `CancelThrough` which re-acquires — an enqueue between the two critical sections mints `next+1` and survives a cancel that already decided it covered "every remaining item". This composes with the pre-mutex classifier into a concrete anti-zombie violation:

1. Turn T is active (`turnActive=true`).
2. Steering Run S enters `routeSteering`, reads `clientTurnActive == true`, passes the gate.
3. T exits (normally or cancelled): `markClientTurn(false)` → `unregisterActiveTurn` → `resolveSessionSteering` → `CancelAll` resolves an empty queue.
4. S now calls `q.Enqueue(text)` — nothing is active, but no rider will ever resolve it.
5. The NEXT, unrelated prompt's `runTurn` drains at its first iteration top and delivers the stale input as a mid-turn steering marker — exactly the "zombie-deliver into the NEXT turn's window" outcome `recordCanceled`/`unregisterActiveTurn`/`cancelParkedChains` exist to make impossible (and which `TestParkedChainCancelSteering` / `TestUndoAutoCancel` pin for the non-racy paths).

The same window exists inside a cancelled exit: `recordCanceled`'s `CancelAll` reads `next`, an enqueue lands in the gap, `CancelThrough(next)` misses it.

**Fix:** Make `CancelAll` atomic under a single critical section:

```go
func (q *SteerQueue) CancelAll() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	t := q.next
	if t > q.cutoff {
		q.cutoff = t
	}
	// ... in-place filter over q.items exactly as CancelThrough does ...
}
```

and/or close the classifier side: after `Enqueue`, re-check `r.clientTurnActive(sessionID)` and `r.chainCount(sessionID)`; if neither is active anymore, immediately `q.CancelThrough(ticket)` and emit a "turn ended before delivery — input cancelled" note instead of "steering queued".

### WR-02: `Store.prune` retention backstop evicts by (sessionID, turnNum) grammar order, not recency — a session's NEWEST checkpoints can be deleted while another session's OLDEST survive

**File:** `internal/checkpoint/store.go:912-937` (`prune`, consuming `listRefs`' sort at `internal/checkpoint/store.go:852-865`)

**Issue:** The prune docstring claims ties "break by (sessionID, turn number), which tracks snapshot sequence deterministically", but `listRefs` sorts by `(SessionID, TurnNum, Kind)` with **no timestamp component at all** — so with more than `DefaultKeep` (50) total refs between sweeps, prune deletes from the front of the grammar order: session `aaa`'s newest snapshots are evicted while session `zzz`'s oldest are kept, regardless of when either was committed. A `/undo` walk can then lose its walk targets (silent "nothing to restore"-adjacent behavior or a shallower stack) purely due to session naming. Phase 23's `Sweep` documentation explicitly keeps prune as the "between-sweeps backstop" and asserts both are safe together — that assertion holds for double-deletion, but not for recency. (The sort itself predates phase 23; the phase's GC work re-baselined prune's retention story, so it is in scope.)

**Fix:** Select prune victims by recency — the `expireByCount` discipline (`internal/checkpoint/store.go:443-474`) already implements newest-first per-session ordering; reuse it globally:

```go
func (s *Store) prune(ctx context.Context) error {
	entries, err := s.listRefs(ctx)
	if err != nil {
		return err
	}
	victims := expireByCount(entries, DefaultKeep) // global, recency-ordered
	for _, v := range victims {
		if _, derr := s.git(ctx, "update-ref", "-d", v.Ref); derr != nil {
			return fmt.Errorf("checkpoint: prune %s: %w", v.Ref, derr)
		}
	}
	return nil
}
```

(or keep per-session counting globally: `kept[s.SessionID] > DefaultKeep` evicts).

### WR-03: ConfigSurface accepts `_meta` blob fills for the four numeric options but never consults them — recognized-yet-inert fills, plus missing fill-supersede on explicit writes

**File:** `internal/acpserve/config_surface.go:453-471` (`applyFillsLocked` stores fills for every `isMenuOption` id), `internal/acpserve/config_surface.go:1730-1758` + `1828-1856` (`effectiveCheckpointBoundLocked` / `effectiveBackgroundCapLocked` read layers+defaults only, never `blobFills`), `internal/acpserve/config_surface.go:610-623` (`snapshotLocked`'s `effectiveState` omits both option families), `internal/acpserve/config_surface.go:1660-1668` + `1717-1725` (`setBackgroundCapLocked` / `setCheckpointBoundLocked` skip `delete(s.blobFills, bare)`)

**Issue:** `ApplyBlobDefaults` documents "recognized option keys fill UNSET slots" and `applyFillsLocked` stores fills for `background.subagents`, `background.bash`, `checkpoint.expiry_days`, `checkpoint.max_per_session` (all pass `isMenuOption`). But no resolver ever reads those fills: the effective-value ladders for these four go project-layer → global-layer → fixed default, and the blob change-detection snapshot ignores them. An initialize `_meta` carrying `checkpoint.expiry_days: "14"` is silently retained and changes nothing — inconsistent with every other menu id (tier/model/permMode/compaction/grace all resolve fills). Additionally, unlike the other six Set handlers, the two numeric handlers don't drop the superseded fill after an explicit persist; today that is masked by the fills being dead, but it becomes a stale-overlay bug the moment fills are wired.

**Fix:** Either exclude the four numeric ids from fill recognition (`isMenuOption` gains a second, resolution-aware predicate), or wire them consistently: resolve via `blobOrDefaultLocked`, include them in `effectiveState` for change detection, and `delete(s.blobFills, bare)` after a successful persist in both handlers.

## Info

### IN-01: Misleading `"call: %w"` error prefixes across the transcript/store paths

**File:** `internal/session/manager.go:69,79,99,116,130,499`; `internal/session/transcript.go:245,254,263`; `internal/runtime/runtime.go:1108` et al.

**Issue:** Wrap errors carry the prefix `"call: "` regardless of what failed (mkdir, gitignore write, file open, marshal, stream) — a mechanical artifact that actively misleads debugging ("call:" names no operation). It predates phase 23 but pervades the reviewed files.

**Fix:** Replace with operation-naming prefixes (`"open transcript: %w"`, `"write transcript: %w"`, `"marshal line: %w"`, …) opportunistically, or in one sweep.

### IN-02: Stale menu/table count comments vs actual surface sizes

**File:** `internal/acpserve/config_surface.go:306` ("full eight-entry menu"), `internal/acpserve/config_surface.go:1294` ("twelve-entry menu"), `internal/runtime/commands.go:113-115` ("20-01 ships exactly two live builtins")

**Issue:** `menuEntriesLocked` now builds 20 rows (10 bare + 10 `_global/` twins) and `builtinTable` ships 13 live handlers; the count comments drifted across phases and now understate the surface by 8 and 11 respectively.

**Fix:** Update the counts or drop the number from the sentence ("builds the full menu", "the live builtin set").

### IN-03: `checkpointGCBoundsLocked` silently converts hand-written non-positive bounds to defaults

**File:** `internal/acpserve/config_surface.go:1806-1824`

**Issue:** Only parse failures log; a parseable-but-non-positive persisted value (e.g. a hand-edited `expiry_days: 0`, which `Sweep`'s own contract defines as "disable the age axis") silently becomes the 7-day default — the opposite of the operator's intent, with no diagnostic. The strict-membership Set path makes 0 unreachable through the menu, so hand-edits are the only route and they diverge silently.

**Fix:** Log the non-positive fallback the same way the unparseable one logs ("stored value %q is not a positive number of days — falling back to the %dd default").

### IN-04: `SteerQueue.cutoff` is write-only state; `CancelThrough` has no production caller

**File:** `internal/session/steerqueue.go:41-45,97-126`

**Issue:** `cutoff` is only ever self-compared inside `CancelThrough` — the "ack protocol" inspection surface it documents does not exist yet, and production resolves exclusively through `CancelAll`. Dead state invites divergence between the documented and actual protocol.

**Fix:** Either expose an accessor (`Cutoff() uint64`) for the transport-neutral ack consumers the doc anticipates (TG-02), or delete the field until a reader lands.

### IN-05: Restore-guard coverage notes — symlinked directory contents are outside the nested-repo refusal

**File:** `internal/checkpoint/store.go:623-662` (`findNestedRepos`)

**Issue:** The guard refuses nested git repos (dir and `.git`-file variants) because their contents are "silently unprotected" by restore, but a symlink pointing at a directory (pinned as a cost bound in `TestNestedRepoSymlinkBounded`) has exactly the same property: never snapshotted, never cleaned, invisible to the refusal. Restore reports success while the symlinked tree's changes are unrecoverable. Also, `restore_guard_test.go:213` (`var _ = errors.Is`) is a dead import-keeper and `TestRestoreGuardRefusalMatrix` double-exits its chain counter (harmless under the `> 0` guard).

**Fix:** Consider flagging symlink-to-directory entries (resolvable via `os.Stat` on the walked entry, still no descent) in `findNestedRepos`, or document the symlink exclusion beside the gitlink one; drop the dead keeper.

---

_Reviewed: 2026-09-10T12:00:55Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
