---
phase: 14-adoption-readiness-analysis-dispositions
reviewed: 2026-08-19T20:55:18Z
depth: deep
files_reviewed: 43
files_reviewed_list:
  - .gitignore
  - cmd/ass-guard/acp_serve.go
  - cmd/ass-guard/acp_serve_test.go
  - cmd/ass-guard/checkpoint.go
  - cmd/ass-guard/checkpoint_test.go
  - cmd/ass-guard/goconst_constants.go
  - cmd/ass-guard/main.go
  - cmd/ass-guard/parity.go
  - cmd/ass-guard/parity_test.go
  - cmd/ass-guard/provider_factory.go
  - cmd/ass-guard/subagent_tier_wiring_test.go
  - cmd/ass-guard/testdata/checkpoint-e2e/README.md
  - docs/compaction-decision.md
  - docs/shaper-pi-audit.md
  - docs/tool-contract-inventory.md
  - internal/checkpoint/store.go
  - internal/checkpoint/store_test.go
  - internal/coreexec/bash_test.go
  - internal/coreexec/files_test.go
  - internal/parity/cacheprobe.go
  - internal/parity/cacheprobe_test.go
  - internal/parity/run.go
  - internal/profile/corpus_scan.go
  - internal/profile/corpus_scan_test.go
  - internal/profile/testdata/context-behavior/cache-control.jsonl
  - internal/profile/testdata/context-behavior/eviction-window.jsonl
  - internal/provider/errors_test.go
  - internal/scheduler/dispatch_test.go
  - internal/session/checkpoint_seam_test.go
  - internal/session/session.go
  - internal/session/subagent.go
  - internal/session/subagent_test.go
  - internal/session/truncate.go
  - internal/session/truncate_test.go
  - internal/toolcat/catalog.go
  - internal/toolcat/catalog_test.go
  - internal/toolcat/coretools.json
  - internal/toolcat/types.go
  - internal/toolexec/batch.go
  - internal/toolexec/batch_test.go
  - internal/toolexec/iserror_corpus_test.go
  - internal/toolexec/testdata/iserror-rollout-fixture.jsonl
  - internal/toolexec/testdata/occ-census.json
findings:
  critical: 2
  warning: 7
  info: 6
  total: 15
status: issues_found
---

# Phase 14: Code Review Report

**Reviewed:** 2026-08-19T20:55:18Z
**Depth:** deep
**Files Reviewed:** 43
**Status:** issues_found

## Summary

Phase 14 lands the shadow-git checkpoint store (`internal/checkpoint`), the compaction corpus scan (`internal/profile/corpus_scan.go`), the cache probe + version-drift warning (`internal/parity`, `cmd/ass-guard/parity.go`), light-tier subagent routing + tool-output truncation (`internal/session`), and the uniform tool contract (`internal/toolcat`, `internal/toolexec`). Build, `go vet`, and `-race` tests pass for every changed package, and the corpus evidence discipline (provenance headers, redacted fixtures, documented corpus-absent forms) is genuinely strong.

Two defects block the phase's two headline promises, both verified empirically against the real code paths (not inferred):

1. **Checkpoint restore of any non-latest checkpoint silently leaves files behind** — the overlay `checkout -f <ref> -- .` + `clean -fd` pair cannot delete files that a LATER snapshot staged into the shadow index. The "undo this turn" recovery story (EARLY-01) is incomplete for every restore except the newest checkpoint, which is the only case the tests exercise.
2. **The truncation chokepoint (14-05/EARLY-05) is bypassed by any subagent result containing a newline, quote, or backslash** — the naive quote-concat produces invalid JSON, `boundedToolResult` fails to unmarshal it, and returns it unbounded (verified: a 200,002-byte multi-line result passed through with cap 131,072). Real subagent reports are multi-line, so the token-flood lever does not fire for exactly the payloads it exists to bound.

Both blockers have verified one-line fixes. The remaining findings are retention-order and locking weaknesses in the checkpoint store, evidence-artifact gaps in the parity path, and pre-existing subagent-loop defects that phase 14's changes touch but do not repair.

Verification performed: `go build ./...`, `go vet` on all changed packages, `go test -race` on all changed packages (all green), plus two standalone reproductions (git shadow-store restore semantics; `boundedToolResult` multi-line bypass) and an env-dedup experiment (Go keeps the last duplicate env value — the checkpoint store's `gitEnv` append pattern is safe).

## Critical Issues

### CR-01: Restore of a non-latest checkpoint does not remove files created in later turns

**File:** `internal/checkpoint/store.go:259-267`
**Issue:** `Restore` runs `git checkout -f <ref> -- .` followed by `git clean -fd -e .ass-guard/`. Pathspec checkout is an *overlay* operation: it writes paths present in the target tree but never deletes index entries absent from it. Files added after the target snapshot were staged into the shadow index by the next snapshot's `git add -A`, so they are still *tracked* at restore time — and `git clean -fd` only removes *untracked* files. They survive the restore silently.

Empirically reproduced with the store's exact invocation pattern (`--git-dir`/`--work-tree`, `GIT_INDEX_FILE`, `checkout -f` + `clean -fd -e .ass-guard/`):

```
snapshot turn-001 (a.txt) -> create b.txt -> snapshot turn-002 -> restore turn-001
=> b.txt SURVIVES; shadow status shows "D  b.txt" (tracked, staged-deleted)
```

The CLI explicitly supports restoring ANY listed checkpoint (`checkpoint list` + `checkpoint restore <sessionID-turn-NNN>`), and `Restore`'s contract — "returns the workspace to the checkpoint's recorded pre-turn state" — is what the no-confirmation-tier safety model sells as the reversibility backstop. Every multi-turn rollback (undo turns 2..N by restoring turn 2's pre-state) leaves all files created in the intervening turns in place, silently. No test covers it: `TestSnapshotRestore_ByteIdentical` restores the only/latest snapshot (post-snapshot files were never re-snapshotted, hence untracked), `TestCheckpointLiveRollback_Gated` restores `entries[len(entries)-1]`, and `TestCheckpointRestoreRoundtripCLI` restores the single snapshot.

**Fix** (verified to remove later-added files while keeping everything else identical):

```go
// Restore: --no-overlay makes checkout DELETE index+worktree entries that are
// in the shadow index but absent from the target tree — files staged by a
// LATER snapshot. clean -fd then only has to sweep genuinely untracked files.
_, err := s.git(ctx, "checkout", "--no-overlay", "-f", refPrefix+id, "--", ".")
```

Alternative: `git read-tree -u --reset <ref>` (resets index+worktree to the tree without moving HEAD; `reset --hard` would move `refs/checkpoints/last`). Add a test: snapshot A, create a file, snapshot B, restore A, assert the file is gone.

### CR-02: Subagent Task results with newlines/quotes bypass the truncation cap entirely and write invalid JSON to the transcript

**File:** `internal/session/session.go:419-420`, `internal/session/truncate.go:61-81`
**Issue:** The Task-result path builds its payload as `` json.RawMessage("`" + result + `"") `` — quoting the raw string with NO JSON escaping. Any result containing a raw newline, `"`, `\`, or a control character — i.e., every multi-line model report, the normal shape for a subagent's final answer — is invalid JSON. `boundedToolResult` then fails `json.Unmarshal` and returns the raw bytes untouched. Consequences, both verified by driving the real function:

1. The 128 KiB cap is not applied. A 200,002-byte multi-line result passed through `boundedToolResult` unmodified (cap = 131,072). The phase's stated goal (T-14-14, EARLY-05: "the pathological case is bounded before it can flood the model's context window") is defeated for exactly the payloads most likely to overflow — real subagent reports.
2. The transcript's `tool_result.output` is invalid JSON whenever the result needs escaping, so every downstream consumer must tolerate a malformed payload.

The phase comment added at this site claims "over-cap is bounded + properly re-encoded", which is only true for the escape-free single-line fixtures the tests use (`overCapResult` = `"x"×200KiB + "TAIL-SENTINEL"`); a multi-line over-cap result is untested and fails. The naive concat predates phase 14, but phase 14 built the chokepoint on top of it and declared the over-cap path fixed.

**Fix:**

```go
// Encode the subagent result as PROPER JSON before the chokepoint: json.Marshal
// of an escape-free string is byte-identical to the naive concat, and every
// other case previously produced invalid JSON anyway.
if out, mErr := json.Marshal(result); mErr == nil {
    _ = s.Manager.AppendToolResult(turnID, callID, boundedToolResult(out), false)
} else {
    errJSON, _ := json.Marshal(map[string]string{mapKeyError: mErr.Error()})
    _ = s.Manager.AppendToolResult(turnID, callID, errJSON, true)
}
```

Add a test: an over-cap result containing newlines and quotes must land in the transcript bounded (marker prefix + tail sentinel retained) AND as valid JSON.

## Warnings

### WR-01: prune evicts by (sessionID, turn number), not by commit age — documented behavior not implemented

**File:** `internal/checkpoint/store.go:486-503` (with `listRefs` sort at `432-438`)
**Issue:** `prune`'s doc comment promises "the oldest refs beyond DefaultKeep are deleted. Ties on commit timestamp … break by (sessionID, turn number)". The implementation sorts purely by (SessionID, TurnNum) and pops from the front — `CommittedAt` is never compared. With one session per workspace the two orders coincide; with multiple sessions (one store per workspace, sessions share it) the alphabetically-first session's checkpoints are evicted first regardless of age: an active session `zzz-…` keeps its full recovery history while an older session `aaa-…` loses its recovery points (or the reverse — the active session dies first if it sorts first). `TestRetentionPrune` covers a single session, so the deviation is invisible.
**Fix:** In `prune`, sort a copy of the entries by `CommittedAt` first, with (SessionID, TurnNum) as the documented tie-break, and delete from the front of that order. Keep `List`'s display sort unchanged.

### WR-02: Store lock can be stolen from a live holder after 30s; release then deletes the successor's lock

**File:** `internal/checkpoint/store.go:528-555`
**Issue:** `lockStaleAfter = 30s` doubles as the steal threshold for a *slow but alive* holder. A snapshot of a large workspace (`git add -A` + commit) can legitimately exceed 30s; a concurrent `ass-guard checkpoint restore` then steals the lock, and both processes interleave writes against the SAME `GIT_INDEX_FILE` (the exact interleaving `withLock` exists to prevent). Additionally, the original holder's release func (`os.Remove(lockPath)`) carries no owner identity, so it deletes whichever lock file exists at release time — including the thief's fresh lock, un-serializing a third process.
**Fix:** Write the holder's PID into the lock file and only steal when the PID is not alive (`syscall.Kill(pid, 0)`); or replace the O_EXCL file with `syscall.Flock` on an fd (kernel-released on process death), which removes the stale timer entirely. At minimum, size `lockStaleAfter` against a documented worst-case snapshot time and note the ceiling.

### WR-03: parity-results.json is written before `CacheProbe` is attached — the evidence artifact never contains the probe verdict

**File:** `cmd/ass-guard/parity.go:250-263`
**Issue:** `parityRun` (= `parity.Run`) writes the results file inside the call, from `RunOptions.ResultsPath`. `res.CacheProbe = &probeRep` is assigned only AFTER the call returns. The `RunResult.CacheProbe` field exists to be serialized (`json:"cache_probe,omitempty"`) and the stderr footer prints it — but the JSON evidence artifact (the reproducibility record, D-04) silently omits it.
**Fix:** Attach the probe before the write — e.g. add a `CacheProbe *CacheProbeReport` input field on `parity.RunOptions` that `Run` stamps onto the result before `writeResults`, or re-write the results file after assigning `res.CacheProbe`.

### WR-04: Subagent tool_call/tool_result pairing keyed by tool NAME, not the provider call id

**File:** `internal/session/subagent.go:173,181,183`
**Issue:** The nested loop records `AppendToolCall(subagentTurnID, tc.Name, tc.Name, …)` and `AppendToolResult(subagentTurnID, tc.Name, …)` — the NAME is the pairing id. Two calls to the same tool in one subagent iteration produce duplicate ids (ambiguous tool_call↔tool_result pairing for replay/projection). The parent loop fixed exactly this pattern in 08-06 ("recording the tool NAME here was the 08-06 blocker's root cause 2" — `session.go:382-384`); the subagent path was never aligned. Predates phase 14 but the phase modified this file and the inconsistency with the corrected parent path remains.
**Fix:** Use `toolCallIDOf(tc)` at both sites — it falls back to the name for id-less legacy fakes, so existing tests stay green.

### WR-05: Subagent nested loop re-sends the identical user prompt every iteration — tool results never reach the model

**File:** `internal/session/subagent.go:146-191`
**Issue:** `messages := []provider.Message{{Role: "user", Content: prompt}}` is built once and never appended with the assistant turn or the tool results; each of the up to 8 iterations sends a byte-identical request. The comment says "(simplified: re-send prompt)" and the stale note above it says "Phase-2 stubs tools execution; a full subagent tool-loop is Phase 4" — but Phase 4 shipped, real tool execution is wired through `executeRestricted`, and the subagent's model receives zero feedback from its own tool calls. It will plausibly repeat the same calls until `maxIter`, wasting provider round-trips (on the light tier, but still 8× a full prompt+catalog payload). Predates phase 14.
**Fix:** After executing each iteration's tool calls, append the assistant content and the tool results to `messages` before looping (mirror the parent loop's projection, or build the array manually: assistant message with `tool_use` blocks, then one `tool` message per result keyed by call id).

### WR-06: Semaphore tokens leak on the panic path — a session can wedge after maxConc recovered panics

**File:** `internal/session/session.go:347-359`, `internal/session/subagent.go:154-164`
**Issue:** `Semaphore.Release()` is called after `streamAndEmit` returns normally, not via `defer`. Both call sites sit inside functions whose panics are deliberately recovered one frame up (`runTurn`'s and `DispatchSubagent`'s recover-defers exist precisely because panics are anticipated). Every panic between `Acquire` and `Release` permanently consumes one of the (default 6) tokens; after 6 recovered panics, every later turn of that session blocks in `Acquire` until its ctx is cancelled. The token channel makes the loss silent.
**Fix:** Immediately after a successful `Acquire`, `defer s.Semaphore.Release()` (both sites), so the token returns as the frame unwinds through the recover.

### WR-07: Structured tool outputs bypass the truncation cap entirely

**File:** `internal/session/truncate.go:61-81`
**Issue:** `boundedToolResult` bounds only the JSON-string payload form; every structured object passes through untouched — the file's own comment names the openspec `{stdout,stderr,exit_code,classification}` convention as a pass-through. An openspec command (or any structured-output tool) emitting a multi-megabyte `stdout` therefore floods the transcript and the model's projected window with no bound at all, defeating the stated purpose ("the pathological case is bounded before it can flood the model's context window", T-14-14). The comment frames this as deliberate, but the invariant that makes it safe (structured outputs are bounded upstream) is neither implemented nor documented anywhere at the chokepoint.
**Fix:** Either document the upstream bound that guarantees structured payloads cannot overflow (per-command caps in openspec/coreexec, with a test pinning it), or extend `boundedToolResult` to walk known text-bearing fields of the structured forms (`stdout`, `stderr`, `text`) and bound each.

## Info

### IN-01: Cache-probe placement "skip note" is constructed then discarded

**File:** `cmd/ass-guard/parity.go:157-165,170-177`
**Issue:** `CheckPlacementVsPin`'s doc (`internal/parity/cacheprobe.go:39-42`) promises the wiring site appends the placement check "or its explicit skip note". `assembleCacheProbe` drops the skip `ProbeCheck` entirely when `ok=false` — with an empty/unreadable `--cache-pin`, the report carries only the two ordering checks and no trace that placement was skipped or why.
**Fix:** Append the skipped check to `rep.Checks` (OK:true with its "skipped: …" detail) instead of discarding it; `rep.OK` is unaffected.

### IN-02: Stale doc comment for the removed `executeStub`

**File:** `internal/session/session.go:564-566`
**Issue:** "executeStub is retained for Phase-2 callers/tests that drive one tool call" — no such function exists; the sentence dangles directly above `streamAndEmit`'s doc comment and now reads as its first (wrong) line.
**Fix:** Delete the orphaned comment lines 564-565.

### IN-03: `DispatchBatch`'s error return is always nil; the caller's error branch is dead

**File:** `internal/toolexec/batch.go:91-167`, `internal/session/session.go:437-443`
**Issue:** Every return path of `DispatchBatch` returns a nil error (cancellation surfaces per-call in `results`). `runTurn`'s comment claims "a non-nil top-level error is a cancelled-ctx path — record + continue", and its `appendError(turnID, "toolexec", batchErr, true)` branch can never execute — a misleading contract for future callers. (The `([]ToolResult, error)` shape is fine if the error stays reserved; the comment is the defect.)
**Fix:** Correct the session.go comment to state that errors surface per-call via `IsError`/`Err` and the top-level error is currently always nil — or actually return `ctx.Err()` when the context is cancelled.

### IN-04: PARA-02 "tagged with parent-turn-id" claim is not implemented

**File:** `internal/session/subagent.go:35-37,196-200,245`
**Issue:** `DispatchSubagent`'s doc says "streamed progress flows via the bus tagged with parent-turn-id (PARA-02)", but `streamAndEmitTaggedProf` publishes chunks with `TurnID: subagentTurnID` and explicitly discards the parent id (`_ = parentTurnID // events are tagged via the SubagentResult`). A client cannot attribute streamed subagent output to the parent turn it belongs to.
**Fix:** Either carry `ParentTurnID` on the chunk events (additive field on `event.AgentMessageChunk`) or fix the two doc comments to describe the SubagentResult-only tagging.

### IN-05: `Store.List()` ignores contexts entirely

**File:** `internal/checkpoint/store.go:242-244`
**Issue:** `List` runs its `for-each-ref` under `context.Background()`; the CLI surface (`checkpoint list`) has no cancellation path. Contrast with `Snapshot`/`Restore`, which thread ctx.
**Fix:** `func (s *Store) List(ctx context.Context) ([]Entry, error)`; the cmd plumbing (`cmd/ass-guard/checkpoint.go:68`) already has `cmd.Context()` available.

### IN-06: `executeBounded` attributes a parent-deadline expiry to the per-tool timeout

**File:** `internal/toolexec/batch.go:215-231`
**Issue:** The deadline-expiry mapping checks `errors.Is(callCtx.Err(), context.DeadlineExceeded)`. If the PARENT batch ctx carries a deadline that expires during a call, `callCtx.Err()` is also `DeadlineExceeded` and the result's message reads "tool X timed out after <timeout_ms>ms" — naming the tool's own bound as the cause when the parent cancellation was. Misleading diagnostics only (the IsError handling is correct either way).
**Fix:** Distinguish the sources before rewriting the output, e.g. only apply the timeout form when `ctx.Err() == nil` (parent not expired) and surface the parent error verbatim otherwise.

---

_Reviewed: 2026-08-19T20:55:18Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
