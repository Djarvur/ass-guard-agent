---
phase: 19-compaction-cache-control
reviewed: 2026-09-06T00:00:00Z
depth: deep
files_reviewed: 36
files_reviewed_list:
  - cmd/extract-profile/main.go
  - internal/acpserve/acp_serve.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/config_surface_lock_test.go
  - internal/acpserve/config_test.go
  - internal/defaults/seed/config.yaml
  - internal/modelrouting/config.go
  - internal/modelrouting/defaults/config.yaml
  - internal/modelrouting/load_test.go
  - internal/paritycli/parity.go
  - internal/paritycli/parity_test.go
  - internal/profile/extract.go
  - internal/profile/extract_test.go
  - internal/profile/goconst_constants.go
  - internal/profile/loader.go
  - internal/profile/testdata/sample-sessions/cache-control.jsonl
  - internal/profile/types.go
  - internal/provider/errors.go
  - internal/provider/errors_test.go
  - internal/provider/streaming.go
  - internal/provider/streaming_test.go
  - internal/providerfactory/config_write_test.go
  - internal/runtime/runtime.go
  - internal/session/compaction.go
  - internal/session/compaction_test.go
  - internal/session/gate_test.go
  - internal/session/manager.go
  - internal/session/projector.go
  - internal/session/projector_test.go
  - internal/session/session.go
  - internal/session/transcript.go
  - internal/session/transcript_newkinds_test.go
  - internal/shaper/midturn_test.go
  - internal/shaper/shaper.go
  - internal/shaper/shaper_test.go
  - profiles/zcode/profile.yaml
findings:
  critical: 1
  warning: 6
  info: 6
  total: 13
status: issues_found
---

# Phase 19: Code Review Report

**Reviewed:** 2026-09-06
**Depth:** deep
**Files Reviewed:** 36
**Status:** issues_found

## Summary

Phase 19 shipped five sub-features: cache_control emission (19-01), non-2xx stream rejection + IsOverflow (19-02), the compaction reset point + budget-fill tail (19-03), the compaction engine (19-04), and the compaction config keys + menu wiring (19-05).

The review traced the full call chains: profile loader → shaper (cache_control keep-last-4), provider.Stream → streamAndEmit → runTurn loop-head → maybeCompact/compact → Projector, and ConfigSurface.Set → WriteLayerOption → runner.ApplyCompactionSettings → per-session turn-mutex swap. The concurrency discipline holds where it is claimed: `ApplyCompactionSettings`/`ApplyTurnModel` take each session's turn mutex one at a time (no hold-and-wait across sessions, so no lock inversion), hooks fire outside the surface mutex (WR-03 pins), and the buffered-1 closed-channel error-chunk path in `Stream` is correct.

One structural defect breaks the phase's headline recovery feature, and several robustness/validation gaps remain:

1. **The overflow retry-once is a no-op resend** (CR-01). The forced compaction's marker can never apply to its own producing turn (by the same Pitfall-5 rule the projector correctly enforces), so the retried request is projected byte-identically to the one that just overflowed and deterministically overflows again. The green "recovery" test passes only because the scripted fake provider succeeds regardless of message content.
2. The compaction blob fills advertise values the engine never enforces (chip != wire, WR-01); the threshold estimate's anchor is async-appended and timing-dependent (WR-02); the summarizer input is unbounded and can itself overflow (WR-03); a client-supplied session id reaches the transcript path unsanitized (WR-04); the marker's postRef arithmetic is racy (WR-05); and the context limit ignores per-session model switches (WR-06).

Cache_control emission (19-01) and the non-2xx rejection path (19-02) were verified correct against their tests, including the keep-last-4 degrade arithmetic (`flagged-seenFlagged <= 4` keeps exactly the last four flagged blocks), the bounded 8 KiB error-body read with synchronous close, and the message-class (not status-class) overflow matcher.

## Critical Issues

### CR-01: Overflow retry-once re-sends a byte-identical request — recovery is structurally impossible

**File:** `internal/session/session.go:548-553`, `internal/session/projector.go:308-320`, `internal/session/compaction.go:215-322`
**Issue:** On an overflow-classified stream error, `runTurn` sets `overflowRetried`, calls `s.compact(ctx, turnID)`, and `continue`s to the next loop head, which re-projects and resends. But the compaction marker is appended *during* turn T — after T's `user_message` line — and `compactionMarkerIdx` only accepts markers **strictly before** the projected turn's user message (`i < userIdx`, projector.go:314). `splitAtResetBoundary`, `accumulateMidTurn`, and `foldExchanges` are likewise unaffected by a `TypeCompaction` line. The retried projection is therefore identical to the overflowing one:

- Mid-turn overflow (the realistic case — the window grows via the turn's own accumulation): the marker cannot shrink the accumulation; `boundMidTurn` caps by message count, not size, so the resent body has the same bytes.
- First-iteration overflow: the lean seed is small; overflow there is implausible, and even then the marker still postdates the turn's user message.

Consequence: the second overflow always fires, the guard sends the turn to `appendError`, and the turn fails. The compaction only helps the *next* turn (which does start after the marker). `TestCompaction_OverflowRetryOnce`'s "recovery" subtest masks this because `compactionProvider` script #3 returns success regardless of the message batch it receives; the fail-twice subtest is what production always does. Criterion 3 ("fail overflow, compact, resend, turn completes") is not actually implemented — the resend carries the same payload.

**Fix:** Make the retry project through the marker. Minimal shape: have `compact` return the marker's transcript index (or set a per-turn `compactedForRetry bool`), and on the retry iteration pass an override to the projector so the producing turn projects as a *post-marker* turn — seed = marker summary, tail = the budget-fill post-marker tail (the `projectCompacted` path with `mIdx` allowed to be ≥ the turn's user message for this turn only). Alternative: on the retry, project with `foldExchanges` bounded by the budget tail from the marker forward instead of the full turn accumulation. Either way, add a regression test where the fake provider rejects any request whose marshaled size exceeds a bound, so a byte-identical resend fails the test (the current fake cannot detect it).

## Warnings

### WR-01: Initialize-blob compaction fills advertise values the engine never enforces (chip != wire)

**File:** `internal/acpserve/config_surface.go:422-440` (`applyFillsLocked`), `:385-405` (`ApplyBlobDefaults`)
**Issue:** `applyFillsLocked` accepts `compaction-threshold`, `compaction-enabled`, and `tombstoneGraceDays` as `_meta` blob fills and `optionsLocked()` advertises them as `currentValue` (effectiveCompactionThresholdLocked falls through to `blobOrDefaultLocked`). But the blob path fires only `blobHook` (the model stamp, 16-REVIEW CR-02); nothing relays a compaction fill to `runner.ApplyCompactionSettings`. Result: an editor whose initialize `_meta` carries `compaction-threshold: 65` sees the menu advertise 65 while the running session compares against the floor's 80 — the exact chip-shows-X/wire-sends-Y divergence class CR-02 was fixed for on the model path. The code comment declares this deliberate ("the operator's live lever for compaction is set_config_option"), but the advertisement is then knowingly wrong until the operator acts.
**Fix:** Either exclude compaction (and grace) ids from `applyFillsLocked`'s recognized set so the advertisement shows the enforced floor value, or fire `compactionHook` with the post-fill effective pair when a compaction fill moved the effective value (mirroring the CR-02 blob-model fix).

### WR-02: Threshold estimate anchors on an async-appended line — timing-dependent double count

**File:** `internal/session/compaction.go:141-162` (`estimateLinesSinceLastRequest`), `internal/session/transcript_writer.go:85-92`
**Issue:** `request_shaped` lines are appended by the async `TranscriptWriter` (bus subscriber), while `estimateSinceLastRequest` reads the transcript at the loop head looking for the *last* `request_shaped` line. If the just-shaped request's line has not landed yet, the anchor slides back to the previous request and the estimate counts the entire intervening exchange round (assistant text + tool_call/tool_result lines) that is already inside `lastInputTokens` (that round was part of the last reported input). One exchange round gets double-counted whenever the writer lags one request. Additionally, after a compaction the marker line itself (whose `Summary` can be thousands of tokens) plus the usage line sit after the anchor and count toward the next estimate until the next request_shaped line lands. Effect: premature firing (never late — consistent with the "fires early" policy, but the drift is uncontrolled and grows with writer lag).
**Fix:** Record the last-request anchor in memory at shape time (e.g., a monotonic append counter or byte offset captured in the capturer, the same in-memory-read-model discipline `lastInputTokens` uses per Pitfall 3), instead of re-deriving it from the async transcript; or exclude `TypeCompaction`/`TypeUsage` lines from the estimate sum.

### WR-03: Summarizer input is unbounded — an overflowing context produces an overflowing summarize request, then a per-iteration degrade loop

**File:** `internal/session/compaction.go:241` (`compactionPrompt(prevSummary, renderSpan(...))`), `:215-322`, `:171-184`
**Issue:** `compact` renders the *entire* post-marker span (`foldExchanges(lines, from, "", false)` — potentially the whole transcript at first compaction) into one user message with no size bound, and the only cap is on output (`CompactionSummaryMaxTokens`). When the context is over the provider's limit (the overflow-retry trigger, or heavy estimation drift), the summarize request itself is over the limit: the non-2xx path (19-02) classifies it, `degradeCompaction` swallows it, no marker lands — and `maybeCompact` runs again at the *top of every one of the up-to-64 iterations*, each time re-sending the same oversized body. That is up to 64 guaranteed-400 near-limit provider calls per turn (cost + latency), and on the overflow-retry path it means the backstop's compact also cannot succeed (compounding CR-01).
**Fix:** Bound the summarizer input (render the span up to a byte/token budget derived from `ContextLimit`, newest-first pair-safe like `boundCompactionTailByBudget`), and/or add a per-turn compaction-attempt cap (analogous to `overflowRetried`) so a degrading summarizer stops re-firing within the same turn.

### WR-04: Client-supplied session id reaches the transcript path unsanitized (path traversal)

**File:** `internal/session/transcript.go:225-228` (`openTranscript`), `internal/session/manager.go:40-51` (`NewManager`), `internal/acp/handlers.go:452-456`
**Issue:** `session/prompt` checks only `p.SessionID == ""`, then `sessionFor` → `NewManager(dir, sessionID, …)` → `openTranscript` builds `filepath.Join(storeDir, "transcript_"+sessionID+".jsonl")`. A sessionID containing path separators (e.g. `a/../../../tmp/evil`) makes `filepath.Join` clean the path out of `.ass-guard/`, opening an append-handle (O_APPEND|O_CREATE, 0600) — and the load/list paths read by the same construction — on files outside the workspace. The project applies id whitelisting discipline to config option ids (T-16-13/14/15) but has no equivalent guard on session ids. Pre-existing (not introduced by this phase) but present in a reviewed file; the practical attacker must control the ACP stdin channel (normally the trusted editor), hence Warning rather than Critical.
**Fix:** Validate the session id shape at `NewManager` (reject anything outside `[A-Za-z0-9._-]`, or at minimum any `/`, `\`, or `..` segment) and return the typed error; optionally assert the cleaned path stays inside `storeDir`.

### WR-05: Compaction marker preRef/postRef arithmetic races the async TranscriptWriter

**File:** `internal/session/compaction.go:309-313`
**Issue:** `preRef = line:{max(len(lines)-1,0)}`, `postRef = line:{len(lines)+1}` assumes nothing is appended between the opening `ReadAll` and the synchronous `AppendUsage`/`AppendCompaction`. But the async TranscriptWriter can interleave `request_shaped` (and chunk/usage) lines at any moment — including the summarizer's own `request_shaped` line, which the code's own D-10 comment says lands "attributed to the current turn". When k async lines land in the window, the marker's real index is `len(lines)+k+1`, so `postRef` points at the wrong line (and the in-code comment "the usage line landed at len(lines), the marker at len(lines)+1" is only true when k=0). No consumer reads PreRef/PostRef today (grep confirms they are write-only), so impact is confined to audit-bookkeeping accuracy — but the pointers are the D-21 record's "what survived / where the window resets" contract.
**Fix:** Derive the refs after the appends (re-read the transcript length, or have `AppendUsage`/`AppendCompaction` return the landed line index under the manager's mutex), or document the refs as best-effort snapshots.

### WR-06: Compaction context limit ignores per-session model switches (wrong window for switched sessions)

**File:** `internal/runtime/runtime.go:2674-2690` (`compactionContextLimit`), `:2412-2444` (`ApplyCompactionSettings`), `:2345-2373` (`ApplyTurnModel`)
**Issue:** `compactionContextLimit()` resolves the context window for `r.effectiveModelFor()`/`defaultTurnModel()` — the runner-level ladder — and `ApplyCompactionSettings` stamps the *same* limit onto every live session. A session whose model was switched via `set_config_option` to a declared model with a different `context_window` (e.g. 100K vs the floor's 200K) keeps the stale 200K limit: the threshold fires at 160K, past the real 100K window, so the session overflows before compaction ever triggers (and then hits CR-01). `ApplyTurnModel` does not re-resolve compaction limits.
**Fix:** In `ApplyTurnModel`, after `SetTurnModel`, also `SetCompactionSettings` with the limit re-resolved for the new model (or resolve the limit inside `SetCompactionSettings` from the session's own profile model via an injected resolver).

## Info

### IN-01: Stale comment contradicts shipped behavior — TypeCompaction "is NOT a reset point today"

**File:** `internal/session/transcript.go:68-76`
**Issue:** The `TypeCompaction` doc still says "until then the Projector treats the kind as inert (D-20 additive-only — it is NOT a TypeBoundary reset point today)". Phase 19 made it exactly that (projector.go `compactionMarkerIdx`/`projectCompacted`); manager.go's sibling comment was updated in this phase but this one was not.
**Fix:** Update the comment to describe the Phase-19 durable reset-point semantics.

### IN-02: Error message names an always-empty variable

**File:** `internal/profile/extract.go:178-181`
**Issue:** The "no full-request lines" error formats `firstFile`, which is only assigned inside the `firstFull != nil` branch — it is always `""` here. The message should name `path` (the rollout file actually scanned).
**Fix:** `fmt.Errorf("no full-request lines (system+tools) found in %s", path)`.

### IN-03: extract-profile summary line prints the same count twice

**File:** `cmd/extract-profile/main.go:97-100`
**Issue:** `"%d tools (%d after null-filter)"` is fed `res.ToolCount, res.ToolCount` — `ToolCount` is the *post*-filter count, so both numbers are identical and the "after null-filter" phrasing is misleading (the raw count is not retained on `ExtractResult`).
**Fix:** Either carry the pre-filter count on `ExtractResult` and print both real numbers, or drop the parenthetical.

### IN-04: Byte-identical embedded config duplicated across two packages

**File:** `internal/defaults/seed/config.yaml` vs `internal/modelrouting/defaults/config.yaml`
**Issue:** The two embedded YAML files are byte-identical (including the new compaction block, which had to be added to both in this phase). There is no generation link, so future edits must be manually mirrored — a silent drift risk (a seed that disagrees with the floor would fork the zero-config baseline).
**Fix:** Generate one from the other (go:generate or a sameness test comparing both embedded assets), or have the seed embed `modelrouting.EmbeddedDefaultScheduling()`.

### IN-05: ConfigSurface setter locking discipline is inconsistent

**File:** `internal/acpserve/config_surface.go:180-197`
**Issue:** `SetNotify`, `SetApplyHook`, and `SetBlobDefaultHook` write hook fields with no lock, while the later-added `SetPermModeRead`/`SetPermModeHook`/`SetCompactionHook` take `s.mu` (the fields are read under the mutex in `Set`/`ApplyBlobDefaults`). Currently safe only because all setters run in the composition before `Serve` starts, but the mixed pattern invites a future race.
**Fix:** Take `s.mu` in the three unlocked setters (or store all hooks atomically in one locked assignment).

### IN-06: sendDone's non-blocking send can silently drop the terminal chunk under backpressure

**File:** `internal/provider/streaming.go:695-704`
**Issue:** `sendDone` uses `select { case ch <- …: default: }` — if the buffer (8) is full at EOF (consumer momentarily slow in `Bus.Publish`), the done chunk carrying `FinishReason` and the assembled `Raw` is dropped and the channel just closes; `mapStopReason("")` then fabricates `end_turn`. Pre-existing (untouched by this phase; the new error-chunk path correctly *blocks* via `sendAbortError`), but it sits beside the phase's new discipline and the two are now inconsistent.
**Fix:** Make `sendDone` ctx-bounded like `sendAbortError`/`flushToolUse`, or document the drop as intentional and the `end_turn` default as the accepted degrade.

---

_Reviewed: 2026-09-06_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
