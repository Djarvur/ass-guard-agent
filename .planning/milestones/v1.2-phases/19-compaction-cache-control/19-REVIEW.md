---
phase: 19-compaction-cache-control
reviewed: 2026-09-07T00:00:00Z
depth: deep
files_reviewed: 37
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
  - internal/session/reconcile_test.go
  - internal/session/session.go
  - internal/session/transcript.go
  - internal/session/transcript_newkinds_test.go
  - internal/shaper/midturn_test.go
  - internal/shaper/shaper.go
  - internal/shaper/shaper_test.go
  - profiles/zcode/profile.yaml
findings:
  critical: 2
  warning: 5
  info: 8
  total: 15
status: issues_found
---

# Phase 19: Code Review Report

**Reviewed:** 2026-09-07
**Depth:** deep
**Files Reviewed:** 37
**Status:** issues_found

## Summary

This run supersedes the pre-19-06 review and covers the phase's complete execution (19-01 through 19-06). The call chains were re-traced end to end: profile loader → `TextBlock.CacheControl` → shaper emission (keep-last-4 cap), `ConfigSurface.Set` → `WriteLayerOption` → `runner.ApplyCompactionSettings` → per-session turn-mutex swap → `SetCompactionSettings` → projector tail budget, and the new 19-06 overflow path: `runTurn` forced compact → `SetRetryCompactedTurn` → `Project` same-turn carve-out → content-sensitive retry (verified against the `sizeRejectProvider` battery).

Disposition of the previous review's findings:

- **Previous CR-01 (byte-identical overflow resend) is closed for its pinned scope**: the engine-armed same-turn carve-out (`projector.go:161-165`, `projectSameTurnCompacted`) makes the retry carry the marker's summary seed plus a budget-fill tail, and the new content-sensitive battery (`TestCompaction_OverflowRetryCarriesSummary`) proves the retry is measurably smaller. However, the fix only engages when `compactionMarkerIdx` finds **no** pre-user marker — after a session's first successful compaction, every later overflow-retry still re-sends a byte-identical request. That residual is the new CR-01.
- **Previous WR-03 (unbounded summarizer span, per-iteration degrade loop) is closed**: `spanBudgetChars` + `boundSpanMessages` (WR-03a) bound the summarize input from the same resolved limit, and `compactionAttemptTurn` (WR-03b) caps threshold-class attempts at one per turn — both pinned by `TestCompaction_BoundedSpanAndReFireGuard`.
- Previous WR-01, WR-02, WR-04, WR-05, WR-06 persist unchanged (re-flagged below with current line numbers). Previous IN-01 (stale `TypeCompaction` comment) is fixed; IN-02/IN-03/IN-04/IN-05/IN-06 persist (IN-02 verified still broken — the error still formats the always-empty `firstFile`).

New this run: a Critical in the streaming SSE reader (in-band `error` events are silently swallowed and fabricated as a clean `end_turn` — the same Pitfall-1 class 19-02 fixed for non-2xx), plus two Info items in `streaming.go`.

The 19-06 concurrency discipline holds where claimed: `ApplyCompactionSettings` releases `compactionMu` before taking `sessMu` (no lock-order inversion with `sessionFor`'s sessMu→compactionMu order), the carve-out override is engine-gated and turnID-keyed (transcript content alone cannot reshape a window), and the projector precedence is deterministic across re-projections.

## Critical Issues

### CR-01: Same-turn compaction carve-out is bypassed after the session's first compaction — overflow retry still re-sends a byte-identical request

**File:** `internal/session/projector.go:153-165`, `internal/session/session.go:573-585`
**Issue:** `Project` checks `compactionMarkerIdx` FIRST and returns `projectCompacted` whenever ANY marker precedes the projected turn's user message; the G-19-1 carve-out (`retryCompactedTurn`) runs only when that scan finds nothing (`projector.go:153-165`, pinned deliberately by `TestProjector_SameTurnCarveOut`'s "pre-user marker wins" subtest). Consider a session that compacted once earlier (marker M1 from a previous turn) and whose turn T9 now overflows: the forced compact appends M2 (after T9's user message), `SetRetryCompactedTurn(T9)` arms the override — but `Project(T9)` still resolves through the M1 path. `foldExchanges(lines, m1+1, …)` skips `TypeCompaction`/`TypeUsage`/`TypeRequestShaped` lines, so M2 and the summarizer's lines change nothing; the seed (M1's summary + current intent) and the budget-fill tail are inputs-identical to the overflowing projection. The retried request is byte-identical, deterministically overflows again, and the turn fails through `appendError` — the exact defect class the previous CR-01 described, now narrowed to "any session that has compacted at least once." The recovery works exactly once per session (the never-compacted case the G-19-1 battery tests with a fresh `newSizeRejectSession` transcript, which carries no prior marker). The forced compact also burns a summarizer call that cannot benefit the producing turn.
**Fix:** Let the armed override take precedence over the pre-user scan (or have the carve-out accept the MOST RECENT marker regardless of position when `retryCompactedTurn == turnID`), so the retry always projects post-M2. Extend `TestCompaction_OverflowRetryCarriesSummary` with a leg that plants a prior `AppendCompaction` marker before the producing turn's user message and asserts the retry is still strictly smaller and seeded with the NEW summary — the current battery cannot detect this residual.

### CR-02: In-band SSE `error` events are silently swallowed — mid-stream provider failures fabricate a clean `end_turn`

**File:** `internal/provider/streaming.go:443-531` (`drainSSE` switch), `:651-678` (`parseAnthropicSSEEvent`)
**Issue:** The Anthropic streaming protocol delivers mid-stream failures as `event: error` / `data: {"type":"error","error":{…}}` (e.g. `overloaded_error`). `drainSSE` handles only `content_block_start`/`content_block_delta`/`content_block_stop`, and `parseAnthropicSSEEvent` handles only `message_start`/`content_block_delta`/`message_delta` — an event with `type:"error"` falls through both switches, emits no chunk, is appended to the assembled raw buffer, and the loop continues to EOF/`[DONE]`, where `sendDone` defaults the empty `finishReason` to `"end_turn"`. A genuinely failed response is therefore surfaced as a successfully completed turn carrying truncated text — precisely the "silent swallow" class 19-02's Pitfall-1 work eliminated for the non-2xx path (which this same file now handles loudly via `rejectStreamError`). Pre-existing (untouched by this phase) but in a reviewed file and directly adjacent to the phase's error-surfacing contract; `sendAbortError`'s own doc states "a dropped abort chunk would let a truncated stream masquerade as complete."
**Fix:** Add a `case "error":` arm to the `drainSSE` switch (or `parseAnthropicSSEEvent`) that decodes the `error` envelope (reuse `errorEnvelopeCause`) and routes it through `sendAbortError`-style classification as a `chunkError` chunk, then returns without a `done`. Add a test feeding `data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}` mid-stream and asserting an error chunk (no fabricated `end_turn`).

## Warnings

### WR-01: Initialize-blob compaction fills advertise values the engine never enforces (chip != wire) — persists

**File:** `internal/acpserve/config_surface.go:422-440` (`applyFillsLocked`), `:356-414` (`ApplyBlobDefaults`)
**Issue:** `applyFillsLocked` still accepts `compaction-threshold`/`compaction-enabled` (and `tombstoneGraceDays`) as `_meta` fills and `optionsLocked` advertises them via `blobOrDefaultLocked`, but `ApplyBlobDefaults` fires only `blobHook` (the model stamp); nothing relays a compaction fill to `runner.ApplyCompactionSettings` (the `compactionHook` seam fires only after a persisted `Set`). An editor whose initialize `_meta` carries `compaction-threshold: 65` sees the menu advertise 65 while every live session compares against the floor's 80. The code comment rationalizes this ("the operator's live lever … is set_config_option"), but the advertisement is knowingly wrong until the operator acts — the same divergence class 16-REVIEW CR-02 fixed on the model path.
**Fix:** Either exclude the compaction/grace ids from the recognized fill set (advertisement then shows the enforced value), or fire `compactionHook` with the post-fill effective pair when a compaction fill moved the effective value.

### WR-02: Threshold estimate anchors on an async-appended line — timing-dependent double count — persists (mitigated)

**File:** `internal/session/compaction.go:168-189` (`estimateLinesSinceLastRequest`), `:207-226` (`maybeCompact`)
**Issue:** `request_shaped` lines are appended by the async TranscriptWriter while the estimate re-derives its anchor from the transcript at each loop head. If the just-shaped request's line has not landed, the anchor slides back one request and the estimate double-counts the intervening exchange round already inside `lastInputTokens`; the compaction marker line (whose `Summary` can be thousands of tokens) and usage lines also sit after the anchor and count toward the next estimate (the sum has no line-type filter). The 19-06 once-per-turn guard bounds the damage to one attempt per turn, but the drift itself is uncontrolled and grows with writer lag.
**Fix:** Capture the last-request anchor in memory at shape time (the `lastInputTokens` discipline), or at minimum exclude `TypeCompaction`/`TypeUsage` lines from the estimate sum.

### WR-04: Client-supplied session id reaches the transcript path unsanitized (path traversal) — persists

**File:** `internal/session/transcript.go:228-229` (`openTranscript`), `internal/session/manager.go:40-51`
**Issue:** `openTranscript` builds `filepath.Join(storeDir, "transcript_"+sessionID+".jsonl")` with no id-shape validation; a sessionID containing separators (e.g. `a/../../../tmp/evil`) escapes `.ass-guard/` and opens an O_APPEND|O_CREATE 0600 handle (plus the read paths) outside the workspace. The project applies id whitelisting to config option ids (T-16-13/14/15) but not to session ids. Pre-existing; the attacker must control the ACP stdin channel (normally the trusted editor), hence Warning.
**Fix:** Validate the session id shape in `NewManager` (reject anything outside `[A-Za-z0-9._-]`, or at minimum `/`, `\`, and `..` segments) and return a typed error.

### WR-05: Compaction marker preRef/postRef arithmetic races the async TranscriptWriter — persists

**File:** `internal/session/compaction.go:362-369`
**Issue:** `preRef = line:{max(len(lines)-1,0)}` / `postRef = line:{len(lines)+1}` assumes nothing lands between the opening `ReadAll` and the synchronous `AppendUsage`/`AppendCompaction`. The async writer can interleave lines (including the summarizer's own `request_shaped`, which the D-10 comment says is attributed to the current turn); with k interleaved lines the marker's real index is `len(lines)+k+1` and both pointers are wrong. No consumer reads PreRef/PostRef today (write-only), so impact is audit-bookkeeping accuracy — but they are the D-21 record's contract.
**Fix:** Derive the refs after the appends (re-read the length, or have the Append* methods return the landed index under the manager's mutex), or document the refs as best-effort snapshots.

### WR-06: Compaction context limit ignores per-session model switches — persists

**File:** `internal/runtime/runtime.go:2674-2690` (`compactionContextLimit`), `:2345-2373` (`ApplyTurnModel`), `:2412-2444` (`ApplyCompactionSettings`)
**Issue:** `compactionContextLimit()` resolves the window for the runner-level ladder (`effectiveModelFor`/`defaultTurnModel`) and `ApplyCompactionSettings` stamps the same limit onto every live session. A session switched via `set_config_option` to a model with a smaller `context_window` keeps the stale larger limit: the threshold fires past the real window, the session overflows before compaction triggers, and then hits CR-01. `ApplyTurnModel` does not re-resolve compaction limits.
**Fix:** In `ApplyTurnModel`, after `SetTurnModel`, re-stamp `SetCompactionSettings` with the limit resolved for the new model (or resolve the limit inside `SetCompactionSettings` from the session's own profile model via an injected resolver).

## Info

### IN-01: Stale doc — `Options()` advertises an "eight-entry" menu; it is twelve

**File:** `internal/acpserve/config_surface.go:275-277`
**Issue:** "Options returns the full eight-entry menu" — the menu has been twelve entries since 19-05 added the compaction pair and the grace options (`menuEntriesLocked` builds 12; `assertFullMenu` pins 12).
**Fix:** Update the comment to "twelve-entry" (or drop the count and name the option groups).

### IN-02: Error message names an always-empty variable — persists (previous IN-02 not fixed)

**File:** `internal/profile/extract.go:178-181`
**Issue:** The "no full-request lines" error formats `firstFile`, which is assigned only inside the `firstFull == nil` branch (`:154`) and is therefore always `""` at the error site. The message reads `found in ` with no path.
**Fix:** `fmt.Errorf("no full-request lines (system+tools) found in %s", path)` and delete `firstFile`.

### IN-03: extract-profile summary line prints the same count twice — persists

**File:** `cmd/extract-profile/main.go:97-100`
**Issue:** `"%d tools (%d after null-filter)"` is fed `res.ToolCount, res.ToolCount`; `ToolCount` is the post-filter count, so both numbers are identical and the parenthetical is misleading (the raw count is not retained on `ExtractResult`).
**Fix:** Carry the pre-filter count on `ExtractResult` and print both real numbers, or drop the parenthetical.

### IN-04: Byte-identical embedded config duplicated across two packages — persists

**File:** `internal/defaults/seed/config.yaml` vs `internal/modelrouting/defaults/config.yaml`
**Issue:** Verified byte-identical today (including the compaction block). No generation link exists, so future edits must be manually mirrored — a silent drift risk that would fork the zero-config baseline.
**Fix:** Generate one from the other (go:generate or a sameness test comparing both embedded assets), or have the seed embed `modelrouting.EmbeddedDefaultScheduling()`.

### IN-05: ConfigSurface setter locking discipline is inconsistent — persists

**File:** `internal/acpserve/config_surface.go:180-197`
**Issue:** `SetNotify`, `SetApplyHook`, and `SetBlobDefaultHook` write hook fields without the mutex while `SetPermModeRead`/`SetPermModeHook`/`SetCompactionHook` take `s.mu` (all five fields are read under the mutex in `Set`/`ApplyBlobDefaults`). Safe today only because all wiring happens in the composition before `Serve` starts traffic.
**Fix:** Take `s.mu` in the three unlocked setters (or store all hooks in one locked assignment).

### IN-06: `sendDone`'s non-blocking send can silently drop the terminal chunk — persists

**File:** `internal/provider/streaming.go:695-704`
**Issue:** `select { case ch <- …: default: }` drops the done chunk (FinishReason + assembled Raw) when the 8-slot buffer is full at EOF; the channel then closes and `mapStopReason("")` fabricates `end_turn`. Inconsistent with the phase's blocking `sendAbortError`/`flushToolUse` discipline. Pre-existing.
**Fix:** Make `sendDone` ctx-bounded like its siblings, or document the drop as the accepted degrade.

### IN-07: `sendAbortError` hardcodes the model in the classified error

**File:** `internal/provider/streaming.go:711-723`
**Issue:** `ClassifyHTTP(providerAnthropic, modelGLM52, 0, cause)` stamps `Model: "glm-5.2"` regardless of the session's actual model — the abort error's investigate-and-fix-ready message names the wrong model for every non-glm-5.2 session (`rejectStreamError` correctly uses `prof.Model`; the abort path has no profile at hand but the provider struct could carry it).
**Fix:** Thread the session model into the drain path (e.g. capture `p.model`/profile slug at Stream start) or drop the Model field rather than misreporting it.

### IN-08: `injectStreamTrue` re-marshals the request body through `map[string]any`

**File:** `internal/provider/streaming.go:734-750`
**Issue:** The streaming body is decoded into a generic map and re-marshaled: key order becomes Go's sorted-map order (diverging from the SDK-shaped non-streaming form) and large numeric literals (≥1e21) switch to scientific notation via the float64 round-trip. For a project whose core value is structural indistinguishability from the mimicked agent, the production turn path's wire bytes are rebuilt through a lossy generic representation. Pre-existing.
**Fix:** Patch the stream flag without a full re-marshal (e.g. inject `"stream":true` into the SDK params before marshaling, or splice the key textually at the top level), keeping the SDK's field ordering.

---

_Reviewed: 2026-09-07_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
