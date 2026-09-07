---
phase: 19-compaction-cache-control
plan: "07"
subsystem: compaction
tags: [compaction, projector, overflow-retry, tdd, regression, parity]

requires:
  - phase: 19-compaction-cache-control plan 06
    provides: the engine-armed same-turn carve-out seam (SetRetryCompactedTurn + projectSameTurnCompacted) this plan re-prioritizes
provides:
  - Armed carve-out takes precedence over the pre-user marker scan in Project() — PAR-01 retry-once recovery works on EVERY overflow, not just a session's first
  - Prior-marker regression coverage at both the projector level (inverted precedence pin + fallback pin) and the session level (prior-marker overflow leg)
affects: [phase-19 verification, PAR-01, WR-06 stale-limit door, future plans touching internal/session/projector_test.go]

actuals:
  tokens: 3738   # chars/4 over the realized code diff (14953 chars across the two task commits)
  tasks: 3
  commits: 3     # 2 production + 1 metadata

tech-stack:
  added: []
  patterns:
    - "Precedence flip as a pure branch reorder: bodies moved verbatim, provable by diff scope (the only behavior change is the order)"
    - "RED pin inversion: a subtest whose old expectation encoded the defect gets its assertions flipped in place (transcript verbatim) instead of being deleted"

key-files:
  created: []
  modified:
    - internal/session/projector.go
    - internal/session/projector_test.go
    - internal/session/compaction_test.go
    - internal/session/session.go

key-decisions:
  - "Project() evaluation order is now (1) armed carve-out, (2) pre-user scan, (3) pre-phase — the armed override defeats any pre-user marker for the matching turnID"
  - "Armed with no same-turn marker, the pre-user scan stays the fallback (degraded-compact fail-through byte-identical to 19-06's behavior)"
  - "The prior-marker leg's AppendBoundary is load-bearing: without it the pre-fix projection takes projectCompacted's budget path (~2.4K chars, under the 4000-byte bound) and the leg passes vacuously"

patterns-established:
  - "Prior-marker CR-01 fixture shape: prior turn user message + AppendCompaction + AppendBoundary before the producing turn's user message, driven through runTurn behind the content-sensitive size-rejecting provider"

requirements-completed: [PAR-01]

coverage:
  - id: D1
    description: "Armed carve-out precedence fix: with a prior marker M1 before the producing turn's user message and an armed same-turn marker M2, Project() resolves through projectSameTurnCompacted — the retry seed carries the NEW summary, never M1's"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: internal/session/projector_test.go#TestProjector_SameTurnCarveOut/armed_override_wins_over_the_pre-user_marker_scan_(CR-01)
        status: pass
      - kind: unit
        ref: internal/session/projector_test.go#TestProjector_SameTurnCarveOut/armed_with_no_same-turn_marker:_the_pre-user_scan_governs_unchanged
        status: pass
    human_judgment: false
  - id: D2
    description: "Prior-marker overflow recovery end-to-end: rejected fat send, bounded summarize, new marker M2 (markersOf == 2), exactly one STRICTLY smaller retry seeded with the NEW summary, turn ends stopEndTurn, no provider error line"
    requirement: PAR-01
    verification:
      - kind: integration
        ref: internal/session/compaction_test.go#TestCompaction_OverflowRetryCarriesSummary/prior_marker:_the_overflow_recovery_still_reaches_the_producing_turn_(CR-01)
        status: pass
    human_judgment: false
  - id: D3
    description: "Tamper safety + fixture integrity survive the reorder: not-armed byte-identity DeepEqual pin, degraded fail-through, subagent-safe pins, and every 19-03..19-06 fixture pass unmodified; deletions vs 06d92d1 confined to (in fact zero — net insertions across) the inverted subtest"
    verification:
      - kind: unit
        ref: "go test -race ./internal/session/ -run '<13-fn battery>' -count=1 (GATE-OK, this run)"
        status: pass
      - kind: other
        ref: "git diff 06d92d1 -- projector_test.go compaction_test.go => 1103 insertions, 0 deletions; git diff --name-only 73440bd -- internal/parity/cacheprobe.go => empty"
        status: pass
    human_judgment: false

duration: 6min
completed: 2026-09-07
status: complete
---

# Phase 19 Plan 07: CR-01 Gap Closure Summary

**Armed same-turn carve-out now takes precedence over the pre-user marker scan in Project() — overflow retry-once recovery works on every overflow (not just a session's first), proven by a prior-marker regression leg the prior battery could not detect.**

## Performance

- **Duration:** 6 min
- **Started:** 2026-09-07T17:58:42Z
- **Completed:** 2026-09-07T18:06:30Z
- **Tasks:** 3 (RED, GREEN, gate)
- **Files modified:** 4

## RED → GREEN Evidence

**RED (Task 1, pre-fix, commit b301267):** `go test ./internal/session/ -run 'TestProjector_SameTurnCarveOut|TestCompaction_OverflowRetryCarriesSummary' -count=1 -v` exited 1 with EXACTLY the two new legs failing, every other subtest passing:

- `TestProjector_SameTurnCarveOut/armed_override_wins_over_the_pre-user_marker_scan_(CR-01)` — FAIL at projector_test.go:2304/:2309 (armed seed carried the pre-user marker's summary; the same-turn summary was displaced)
- `TestCompaction_OverflowRetryCarriesSummary/prior_marker:_the_overflow_recovery_still_reaches_the_producing_turn_(CR-01)` — FAIL at compaction_test.go:1929: `runTurn: stop="" err=session turn stream: ... prompt is too long` — the byte-identical resend overflowed again and the turn failed through appendError, exactly the predicted pre-fix mode
- Fallback pin, not-armed byte-identity subtest, degraded fail-through subtest, and all other pins PASSED

**GREEN (Task 2, post-fix, commit 35d1740):** the same battery passes under `-race`; `go vet ./internal/session/` clean. The reorder (carve-out branch moved above the pre-user scan, bodies verbatim) is the only behavior change — `git show 35d1740` touches exactly projector.go + session.go (session.go comment-only, +5/-1).

**Gate (Task 3):** the chained verifier command set printed `GATE-OK` — 13-function session battery under `-race -count=1` (incl. the TestCompaction_OverflowRetryOnce canary, passing UNMODIFIED on its fresh single-turn fixtures), six regression packages (shaper, profile, paritycli, provider, modelrouting, providerfactory) all ok, acpserve `TestCompaction` surface ok, `go vet` clean, `CGO_ENABLED=0 go build ./...` clean.

**Audits:** `git diff 06d92d1 -- internal/session/projector_test.go internal/session/compaction_test.go` = 1103 insertions, **0 deletions** (the inverted subtest replaced lines that were themselves post-06d92d1 insertions, so the pinned zero-deletion prohibition holds everywhere); `git diff --name-only 73440bd -- internal/parity/cacheprobe.go` = empty (probe baseline untouched).

## Accomplishments

- Closed the CR-01 residual (19-VERIFICATION gap 1, 19-REVIEW CR-01 Critical): Project() now evaluates the engine-armed carve-out FIRST, so an overflow retry always projects post-marker even when an earlier compaction marker precedes the producing turn's user message
- Added the regression detector the battery lacked (zero AppendCompaction plants before this plan): the prior-marker session leg fails on the pre-fix tree and passes post-fix
- Preserved every pinned behavior: not-armed byte-identity (tamper safety), degraded-compact fail-through (fallback pin + existing subtest), retry budget (exactly 3 stream calls), subagent safety, determinism
- Updated all four doc sites (Project doc, inline branch comment, SetRetryCompactedTurn doc, session.go retry-branch comment) to state the precedence and the fallback

## Task Commits

1. **Task 1: RED — prior-marker regression leg + inverted precedence pin** — `b301267` (test)
2. **Task 2: GREEN — armed carve-out takes precedence over pre-user marker scan** — `35d1740` (fix)
3. **Task 3: gate + audits** — no code changes (GATE-OK evidence above)

**Plan metadata:** see final docs commit.

## Files Created/Modified

- `internal/session/projector.go` — branch reorder in Project() (carve-out above pre-user scan, bodies verbatim) + three doc-site updates
- `internal/session/projector_test.go` — inverted precedence subtest (transcript verbatim, assertions flipped) + new fallback pin sibling
- `internal/session/compaction_test.go` — newPriorMarkerSizeRejectSession helper (near-copy, Pitfall-5 discipline; load-bearing AppendBoundary documented) + prior-marker subtest
- `internal/session/session.go` — comment-only: one appended sentence on the overflow-retry branch noting the precedence

## Decisions Made

- Pure branch reorder chosen over the "most recent marker regardless of position" variant — sameTurnMarkerIdx already returns the most recent TurnID-matched marker, so the reorder IS that variant with the smallest possible diff (per the plan's read of 19-VERIFICATION gap 1 missing[0])
- The prior-marker fixture plants a boundary after the prior marker: without it the pre-fix projection routes through projectCompacted's budget path and fits under the rejection bound, making the leg vacuously green (documented in the helper's comment)

## Deviations from Plan

None - plan executed exactly as written. (Doc-comment wording was adjusted within Task 2 to satisfy the plan's own `grep -c 'takes precedence'` acceptance criterion; no behavior impact.)

## Issues Encountered

- **Concurrent Phase 20 execution (shared working tree):** `internal/session/session.go` carried the Phase 20 executor's uncommitted `MintLocalCommandTurnID` hunk when Task 2 needed to commit a comment change in the same file. Resolved by selective hunk staging (`git apply --cached` of only the 19-07 hunk); commit 35d1740 contains exactly the two intended files and the concurrent work remains untouched in the working tree. Same discipline applied to the metadata commit for STATE.md.
- **Discovered residual (doc-only, out of scope by the plan's own audit):** the battery-level doc comment on `TestProjector_SameTurnCarveOut` (projector_test.go ~:2177) still describes the OLD precedence ("...AND no pre-user marker exists"; "the pre-user marker keeps 19-03's winning scan"). Editing it would introduce deletions outside the inverted subtest and violate the pinned zero-deletions audit vs 06d92d1, so it was left as-is and recorded in the phase's deferred-items.md.

## Known Stubs

None — no stubs, no skipped tests, no unrun verifies.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- CR-01 residual closed; PAR-01 retry-once recovery holds on every overflow. The remaining 19-VERIFICATION items stay in their own domains: G-19-2 live-threshold operator retest (UAT domain), CR-02 SSE error swallow, WR-01..06 (review debt) — all explicitly out of this plan's scope.
- Phase 19 now has 7/7 plan summaries; the phase can proceed to its re-verification cycle.

## Self-Check: PASSED

- Files exist: projector.go, projector_test.go, compaction_test.go, session.go (all FOUND)
- Commits exist: b301267, 35d1740 (both FOUND in git log)
- Task 1 acceptance greps: OLD-PRIOR-SUMMARY=4 (>=2), SHRUNK-PRIOR-SUMMARY=2 (>=2), 'armed override wins'=1 (==1), t-prior=4 (>=3); RED run exited 1 with exactly the two new legs failing
- Task 2 acceptance greps: carve-out guard at projector.go:170 < compactionMarkerIdx call at :176; 'takes precedence|takes PRECEDENCE'=3 (>=2); setter doc contains precedence+fallback, position clause gone; battery -race + vet exit 0
- Task 3 gate: GATE-OK printed (chained command exit 0); both diff audits clean

---
*Phase: 19-compaction-cache-control*
*Completed: 2026-09-07*
