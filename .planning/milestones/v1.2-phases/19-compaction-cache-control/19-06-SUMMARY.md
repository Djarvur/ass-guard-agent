---
phase: 19-compaction-cache-control
plan: 06
subsystem: compaction
tags: [compaction, projector, overflow-retry, context-window, same-turn-carve-out, tdd]

requires:
  - phase: 19-compaction-cache-control/03
    provides: the pinned compaction-marker reset-point rule, projectCompacted seed/tail rules, and the SetCompactionTailBudget seam discipline this plan's carve-out mirrors
  - phase: 19-compaction-cache-control/04
    provides: the compaction engine (maybeCompact/compact/renderSpan) and the overflow retry-once branch this plan reshapes
  - phase: 19-compaction-cache-control/02
    provides: provider.IsOverflow (the 400 prompt-is-too-long classification the content-sensitive fixture reuses)
provides:
  - SetRetryCompactedTurn arming seam + the engine-armed same-turn marker carve-out in the Projector (G-19-1, operator ruling (b))
  - WR-03a bounded summarizer span (Context-limit-derived budget, ~400K-char fallback cap, 2K-char floor, truncation notice)
  - WR-03b once-per-turn threshold-compaction attempt guard (Session.compactionAttemptTurn, stamped by maybeCompact AND the forced overflow path)
  - The content-sensitive size-rejecting provider fixture (a byte-identical resend fails again — closes the content-blind fixture's blind spot)
  - Reconcile inertness pin for mid-turn same-turn markers (kill-9 replay safety)
affects: [19-UAT retest (Test 1 G-19-1), Phase 20 /compact handler, future context-policy work]

actuals:
  tokens: 15200   # chars/4 over the realized code diff (60,860 chars across internal/session)
  tasks: 3
  commits: 6      # +1 metadata commit

tech-stack:
  added: []      # stdlib + existing deps only (T-19-SC: no package installs)
  patterns:
    - "Engine-gated transcript carve-out: a transcript line reshapes a live turn's projection ONLY under an in-memory per-turn override armed by the engine (tamper safety by construction — plain field under the caller-holds-turn-serialization discipline)"
    - "Content-sensitive test provider: payload-size-rejecting wrapper over the scripted fixture records marshaled size + first message per call; a byte-identical resend fails again"

key-files:
  created: []
  modified:
    - internal/session/projector.go   # SetRetryCompactedTurn seam, sameTurnMarkerIdx, projectSameTurnCompacted, findIntentLine TurnID-preference pass
    - internal/session/projector_test.go   # TestProjector_SameTurnCarveOut (8 pins, additions only)
    - internal/session/compaction.go   # spanBudgetChars, boundSpanMessages, renderSpanMessage, the maybeCompact guard, degrade log cadence
    - internal/session/compaction_test.go   # TestCompaction_BoundedSpanAndReFireGuard, sizeRejectProvider + TestCompaction_OverflowRetryCarriesSummary
    - internal/session/session.go   # compactionAttemptTurn field, retry-branch stamp + projector arming
    - internal/session/reconcile_test.go   # TestReconcileSameTurnMarker
    - internal/session/transcript.go   # TypeCompaction doc rule update (plan-directed, :68-76 stale IN-01 note)

key-decisions:
  - "The same-turn carve-out is ENGINE-GATED: SetRetryCompactedTurn arms it only on the overflow-retry path; transcript content alone never reshapes a live turn (kill-9 replay + tamper safety, T-19-15); the override stays armed for the producing turn's remaining iterations and self-expires when a different turnID projects"
  - "WR-03a: the summarize span budget derives from the SAME resolved context window as the threshold (fill-target share minus prevSummary/instruction/slack; ~400K-char fallback when unset — the overflow backstop's disabled state; 2K-char floor); a cut span carries the one-line [earlier conversation truncated] notice (T-19-18 accept disposition)"
  - "WR-03b supersedes CONTEXT.md D-09's degraded-summarizer cadence (operator-sanctioned via G-19-1's missing list): one threshold-class ATTEMPT per turn, retry at the NEXT TURN's check — never one guaranteed-failing near-limit call per loop head (up to 64/turn)"
  - "The G-19-1 regression drives runTurn over a pre-built mid-turn transcript (the ask-resume re-entry seam): a fresh Prompt's first projection is lean by construction (D-01 caps pre-turn carry at ~800 summary chars), so only a turn with accumulated exchanges can produce the fat rejected window the regression needs"

patterns-established:
  - "Armed-seam discipline: plain projector field + setter documented as the engine gate (the SetCompactionTailBudget precedent), zero value = not-armed, non-empty exact turnID match required"
  - "Span-bound budget unit = rendered chars via renderSpanMessage (the bound counts the SAME text the render emits — a separate cost model would drift from what is sent)"

requirements-completed: [PAR-01]

coverage:
  - id: D1
    description: "Projector same-turn carve-out under the engine-armed override (summary seed + own intent, empty/budget-fill tail; byte-identity when not armed; pre-user marker wins; subagent shadow; newest marker wins; subagent turn safe; deterministic)"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: internal/session/projector_test.go#TestProjector_SameTurnCarveOut
        status: pass
    human_judgment: false
  - id: D2
    description: "WR-03a bounded summarizer span: fat span cut with truncation notice (oldest dropped, newest kept), under-budget span byte-identical, pathological limit floored, zero-limit fallback caps an enormous span"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: internal/session/compaction_test.go#TestCompaction_BoundedSpanAndReFireGuard (span subtests)
        status: pass
    human_judgment: false
  - id: D3
    description: "WR-03b once-per-turn re-fire guard: degraded summarizer costs one call per turn with checks still counted per loop head; a new turn attempts again; the forced overflow compact stamps the turn"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: internal/session/compaction_test.go#TestCompaction_BoundedSpanAndReFireGuard (guard subtests)
        status: pass
    human_judgment: false
  - id: D4
    description: "G-19-1 engine wiring + content-sensitive regression: overflow -> bounded summarize -> marker -> retry strictly smaller and summary-seeded -> the producing turn completes with exactly one retry; arming spans the turn's iterations; degraded compact keeps the byte-identical-resend fail-through"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: internal/session/compaction_test.go#TestCompaction_OverflowRetryCarriesSummary
        status: pass
    human_judgment: false
  - id: D5
    description: "Replay/reconcile safety: a mid-turn same-turn marker is inert bookkeeping — same closures as the marker-free equivalent (row-1 failed tool_result + row-2 canceled terminal + session_end), Seed unaffected"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: internal/session/reconcile_test.go#TestReconcileSameTurnMarker
        status: pass
    human_judgment: false

duration: 23min
completed: 2026-09-07
status: complete
---

# Phase 19 Plan 06: Same-Turn Compaction Carve-Out (G-19-1) Summary

**Engine-armed same-turn marker carve-out plus the WR-03 rider: the overflow retry now provably carries the compaction summary (a content-sensitive size-rejecting provider pins it), the summarizer input is char-bounded from the context limit, and a degraded compaction costs one call per turn — not one per loop head.**

## Performance

- **Duration:** 23 min
- **Started:** 2026-09-07T16:25:11Z
- **Completed:** 2026-09-07T16:47:53Z
- **Tasks:** 3
- **Files modified:** 7 (all within the plan's declared set + the plan-directed transcript.go doc rule)

## Accomplishments

- G-19-1 closed per operator ruling (b): on a provider overflow error the PRODUCING turn completes after compaction + the single retry — the retry request is measurably smaller and carries the summarizer's summary seed plus the turn's own prompt as the current intent, proven by a content-sensitive provider that rejects by payload size (a byte-identical resend fails again; the old content-blind green "recovery" leg can no longer lie).
- Tamper/replay safety is structural: the carve-out requires the in-memory engine-armed override (`SetRetryCompactedTurn`) — a crafted or replayed transcript alone projects byte-identically to the marker-free transcript; Reconcile classifies the mid-turn marker as inert bookkeeping.
- WR-03a: the summarizer input is bounded from the same resolved context window that governs the threshold (fallback cap when unset, named floor when pathological) — an overflowing context can never produce an overflowing summarize request; dropped content is announced by a one-line truncation notice.
- WR-03b: at most one threshold-class compaction attempt per turn (the overflow-forced compact stamps the same guard); a degraded summarizer retries at the NEXT turn's check.
- Every 19-03/19-04 pinned fixture (TestProjector_CompactionResetPoint, TestProjector_CompactionTailCut, TestCompaction_EndToEnd, TestCompaction_OverflowRetryOnce) stays green with UNMODIFIED bodies — git diff over the plan base shows additions only in all three test files.

## Task Commits

Each task followed the full TDD cycle (RED battery committed failing, then the surgical GREEN):

1. **Task 1: Projector same-turn carve-out under an explicit per-turn override** — RED `c4288fa` (test) / GREEN `dff4ddc` (feat)
2. **Task 2: WR-03 rider — bounded summarizer span + once-per-turn re-fire guard** — RED `3669a58` (test) / GREEN `f05e71d` (feat)
3. **Task 3: Engine retry wiring + content-sensitive regression + reconcile/replay pins** — RED `bf4a19c` (test) / GREEN `1c6896e` (feat)

**TDD gate compliance:** all three plans-in-miniature carry a `test(19-06)` RED commit that failed for the right reason before its `feat(19-06)` GREEN commit (Task 1/Task 3 RED at the missing seam / the reproduced production failure; Task 2 RED behaviorally — `got == unbounded` on every span pin and callCount 4 vs 3 on the re-fire pin). No REFACTOR commit needed — the GREEN implementations landed in the target shape.

**Plan metadata:** (see final docs commit)

## Files Created/Modified

- `internal/session/projector.go` — `SetRetryCompactedTurn` arming seam (G-19-1 engine gate), `sameTurnMarkerIdx` (TurnID-keyed, position-blind), `projectSameTurnCompacted` (reuses seedMessage/seedContent/boundCompactionTail/foldExchanges verbatim; intent from `projectedUserIdx` — the subagent-shadow rule), findIntentLine before-scan TurnID-preference pass
- `internal/session/session.go` — `compactionAttemptTurn` field (turn-serialization discipline), the retry-branch guard stamp, and the projector arming between the forced compact and the continue
- `internal/session/compaction.go` — `spanBudgetChars` + named constants (fallback cap, floor, slack), `boundSpanMessages` (newest-first, message granularity, newest always kept), `renderSpanMessage` extraction, the maybeCompact guard, degrade-log cadence wording
- `internal/session/transcript.go` — TypeCompaction doc comment states the full rule (resets turns that START after it + the engine-armed same-turn carve-out)
- `internal/session/projector_test.go` — TestProjector_SameTurnCarveOut (8 pins)
- `internal/session/compaction_test.go` — TestCompaction_BoundedSpanAndReFireGuard (7 subtests), sizeRejectProvider fixture + TestCompaction_OverflowRetryCarvesSummary (3 subtests) [name on disk: TestCompaction_OverflowRetryCarriesSummary]
- `internal/session/reconcile_test.go` — TestReconcileSameTurnMarker

## Decisions Made

- **Engine gating, not transcript trust (T-19-15):** the carve-out branch requires `retryCompactedTurn != "" && == turnID` AND the pinned pre-user `compactionMarkerIdx` scan to have found nothing — 19-03's winning scan is untouchable by the override, and transcript content alone never reshapes a projection.
- **Unconditional arming is safe because it is transcript-gated:** the degraded-compact case (no marker landed) leaves `sameTurnMarkerIdx` at -1, so the retry projects identically to the rejected request — today's fail-through is preserved exactly, pinned by the degraded subtest (`sizes[2] == sizes[0]`).
- **D-09 cadence supersession (plan-checker W2 amendment, operator-sanctioned):** the degraded-summarizer retry moves from "the next check" to "the next TURN's check"; `degradeCompaction`'s log message updated to say so.
- **Span budget unit is rendered chars** (renderSpanMessage): the bound counts the same text the render emits — a separate cost model would drift from what is actually sent; the under-budget path is pinned byte-identical to the old render.
- **Message-granularity span bound:** the span is plain text inside one user prompt, so the tool_use/tool_result pair-safety constraint that governs the projected window's tail does not apply (plan-noted); the newest message is always kept (never summarize nothing).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - test-expectation bug] Forced-path pin's loop-head count**
- **Found during:** Task 2 GREEN (first verify run)
- **Issue:** the pin asserted `compactionChecks == 2`, but the retry's `continue` re-enters the loop head — the forced-path turn has THREE heads (original, retry, post-tool), so the counter reads 3.
- **Fix:** corrected the pin to 3 with the head enumeration in the message; the behavior itself (4 calls, 1 marker) was already correct.
- **Files modified:** internal/session/compaction_test.go
- **Verification:** the full Task 2 verify command green under -race.
- **Committed in:** f05e71d (Task 2 GREEN commit)

**2. [Rule 1 - fixture-narrative bug] The G-19-1 regression drives runTurn, not Prompt**
- **Found during:** Task 3 RED design
- **Issue:** the plan's behavior says "Drive Prompt", but a Prompt-minted turn's FIRST projection is lean by construction (D-01's mechanical summary caps pre-turn carry at ~800 chars and the fresh turn has no accumulated exchanges) — no Prompt-driven first projection can exceed any meaningful rejection bound, so assertions (a)-(d) were untestable as written.
- **Fix:** the fixture pre-builds the producing turn (user message + 40 fat exchanges) and drives `runTurn(ctx, turnID)` directly — the same re-entry seam the production ask-resume path uses; the overflow branch under test is exercised identically. Rationale documented at the fixture.
- **Files modified:** internal/session/compaction_test.go (newSizeRejectSession)
- **Verification:** RED reproduced the exact production failure ("prompt is too long" on the byte-identical retry, turn errors); GREEN completes the turn with the shrunken summary-seeded retry.
- **Committed in:** bf4a19c / 1c6896e

**3. [Rule 3 - settings shape] The regression uses disabled-check-with-resolved-limit settings**
- **Found during:** Task 3 RED design
- **Issue:** enabled settings with the small context limit the span bound needs would fire the threshold compaction at head 1 (the fat transcript's estimate alone crosses any clamped threshold), consuming the summarizer script entry and landing a pre-send marker — scrambling the plan's 3-call narrative.
- **Fix:** `SetCompactionSettings(false, 80, 1000)` — the threshold check skips (the backstop path, matching the existing recovery fixture per the plan's "settings may stay disabled") while the resolved limit feeds the WR-03a span budget and the projector tail budget.
- **Files modified:** internal/session/compaction_test.go
- **Verification:** exactly 3 stream calls; the summarize call under the bound; one marker.
- **Committed in:** bf4a19c

---

**Total deviations:** 3 auto-fixed (2 test-design fixes, 1 fixture-settings blocker)
**Impact on plan:** All fixes keep the plan's assertions intact and its prohibitions honored (pinned fixtures untouched — git-proven additions only). No scope creep; transcript.go's doc-rule update was plan-directed.

## Issues Encountered

None beyond the deviations above. The pre-existing `internal/runtime` load-sensitive flake family did NOT fire — the full-repo `go test -race -count=1 ./...` gate passed clean on the first run.

## Verification

- `go test -race ./internal/session/ -count=1` — green (new batteries + every existing pin).
- `go test ./internal/session/ -run 'TestProjector_CompactionResetPoint|TestProjector_CompactionTailCut|TestCompaction_OverflowRetryOnce|TestCompaction_EndToEnd' -count=1` — green; `git diff 06d92d1` over the three test files shows ZERO deletions (additions only).
- mise ci legs: build (`CGO_ENABLED=0 go build ./...`) and test (`go test -race -count=1 ./...`) green; lint leg exempt per plan (pre-existing repo-wide golangci 2.12↔2.13 drift, documented in deferred-items.md).
- RED proven in history: three `test(19-06)` commits, each failing for the right reason before its `feat(19-06)` successor.

## User Setup Required

None - no external service configuration required.

## Known Stubs

None — all production paths are real implementations; no placeholder data sources.

## Next Phase Readiness

- Phase 19 execution is complete pending the 19-UAT retest: Test 1's expectation (G-19-1) is now re-testable with a content-honest fixture; G-19-2 (stale-binary retest) remains the operator's pending item and is untouched by this plan, per its out-of-scope note.
- The carve-out's machinery (engine-gated override + TurnID-keyed markers) is available for Phase 20's /compact handler and any future context-policy work.

## Self-Check: PASSED

All 7 key files exist on disk; all 6 task commits (c4288fa, dff4ddc, 3669a58, f05e71d, bf4a19c, 1c6896e) verified in git log.

---
*Phase: 19-compaction-cache-control*
*Completed: 2026-09-07*
