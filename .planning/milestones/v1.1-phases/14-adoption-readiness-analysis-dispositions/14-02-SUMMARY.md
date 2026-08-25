---
phase: 14-adoption-readiness-analysis-dispositions
plan: 02
subsystem: profile
tags: [corpus-scan, cache_control, compaction, rollout-jsonl, context-window, evidence]

requires:
  - phase: 09-profile-drift-audit
    provides: the rollout parsing conventions (extract.go ModelIO), the coverage-manifest pin discipline, the 08-09 window prior art
provides:
  - profile.ScanContextBehavior(io.Reader) — mechanical context-behavior census over zcode rollout JSONL
  - profile.ContextBehaviorReport — cache_control placement classes (incl. the exposed other bucket), literal compaction-marker shape keys, WindowShape arithmetic
  - docs/compaction-decision.md — the EARLY-02 decision artifact (provenance, census, three-way dispositions, consequences) consumed by 14-04/12-05/12-08
  - committed redacted fixtures pinning the observed cache_control and window forms
affects: [14-04-cache-probe, 12-05-recapture, 12-08-eval-net]

actuals:
  tokens: 12048   # chars/4 over the realized diff (estimate was 48000 at factor 1 / confidence low)
  tasks: 2
  commits: 3

tech-stack:
  added: []          # stdlib only — encoding/json + bufio, per the zero-new-deps discipline
  patterns:
    - "generic JSON-path walk for wire-field censuses (count-everything, classify-by-path, expose the other bucket — never silently drop)"
    - "literal shape-key extraction: structural prefixes, never keyword interpretation of conversation text"

key-files:
  created:
    - internal/profile/corpus_scan.go
    - internal/profile/corpus_scan_test.go
    - internal/profile/testdata/context-behavior/cache-control.jsonl
    - internal/profile/testdata/context-behavior/eviction-window.jsonl
    - docs/compaction-decision.md
  modified:
    - internal/profile/extract.go   # ModelIO.Request extended with messages/messagesKind/messageOffset/messageCount

key-decisions:
  - "cache_control is the one routed gap: the target places {"type":"ephemeral"} on EVERY system block (910/910 placements, other bucket empty across 228 records); ass-guard emits none — emission ROUTED post-adoption, 14-04's probe consumes the placement facts only"
  - "window behavior is already delivered by mimicry: rolling last-64-message tail + zero mid-turn resets = Projector MidTurnWindowMessages=64 (08-09 re-scope); the cross-turn window-span divergence stays routed to the 12-05 re-record (today's corpus is single-turn per session — span not re-observable)"
  - "auto-compact and eviction markers are absent-in-target with the honesty clause: 'not observed in analyzed corpus', re-check rides 12-05 on a long multi-turn capture"
  - "execution-time corpus reality: the pinned sess_3cee56ae AND the planning-time 18/22MB mains all rotated off; ladder step 2 analyzed the live 2026-08-19 wave sessions (self-referential — they include this very execution), snapshot-frozen before scanning"
  - "messagesKind/messageOffset/messageCount are request-level rollout bookkeeping outside request.body; the wire-observable window is the message array itself (full to 64, then rolling 64-tail)"

patterns-established:
  - "Corpus-census artifacts quote scanner output verbatim and state the reproduction driver in-artifact (numbers reproducible by re-running the committed scan)"
  - "Fixtures mirror observed forms with provenance headers; synthetic classifier probes are explicitly marked as probes in the header"

requirements-completed: [EARLY-02]

coverage:
  - id: D1
    description: "Mechanical context-behavior corpus scan with committed tests and redacted fixtures (placement classes incl. exposed other bucket, literal marker shape keys, window arithmetic)"
    requirement: EARLY-02
    verification:
      - kind: unit
        ref: internal/profile/corpus_scan_test.go#TestScanContextBehavior_CacheControlFixture
        status: pass
      - kind: unit
        ref: internal/profile/corpus_scan_test.go#TestScanContextBehavior_WindowShape
        status: pass
      - kind: unit
        ref: internal/profile/corpus_scan_test.go#TestScanContextBehavior_CompactionMarkers
        status: pass
      - kind: unit
        ref: internal/profile/corpus_scan_test.go#TestScanContextBehavior_EmptyInput
        status: pass
      - kind: command
        ref: "go test ./internal/profile/... -count=1 (battery unbroken, mise ci exit 0)"
        status: pass
    human_judgment: false
  - id: D2
    description: "docs/compaction-decision.md — the EARLY-02 decision artifact: corpus provenance, complete census with not-observed rows, three-way dispositions (4 delivered / 2 gaps routed / 2 absent), consequences for 12-05/14-04"
    requirement: EARLY-02
    verification:
      - kind: command
        ref: "plan <verify> chain: go test ./internal/profile/... -count=1 && grep counts for cache_control/compact/evict/sess_3cee56ae all non-zero"
        status: pass
      - kind: command
        ref: "acceptance greps: 4 section headers, 12 'not observed in analyzed corpus' rows, disposition literals 4/2/2, zero production 'compact' matches outside corpus_scan.go"
        status: pass
    human_judgment: true
    rationale: "the disposition ROUTING decisions (cache_control emission post-adoption, cross-turn span to 12-05) feed 14-04's and 12-05's planning inputs; the operator should eyeball the routing before those plans consume it — mechanical verifies prove the census, not the routing judgment"

duration: 16min
completed: 2026-08-19
status: complete
---

# Phase 14 Plan 02: Compaction verify-first — decision artifact from corpus evidence Summary

**Corpus scan + decision artifact proving zcode runs a rolling 64-message window with system-block cache_control breakpoints and NO compaction/eviction forms; mimicry already delivers the window, cache_control emission is the one routed gap**

## Performance

- **Duration:** 16 min
- **Started:** 2026-08-19T18:06:57Z
- **Completed:** 2026-08-19T18:23:00Z
- **Tasks:** 2
- **Files modified:** 6 (4 created, 2 modified incl. extract.go extension)

## Accomplishments

- Committed `profile.ScanContextBehavior` — a mechanical census over zcode rollout JSONL counting every cache_control occurrence by placement class (with an EXPOSED other bucket), compact-shaped forms by literal shape key, and the full window arithmetic (messagesKind census, message-length distribution, monotonic-offset vs reset classification). TDD RED `d981add` → GREEN `7931d30`; lint 0 issues.
- Committed two redacted corpus-mirroring fixtures with `# provenance:` headers (header values templated, payload placeholders, synthetic classifier probes marked as probes).
- Ran the scan over the execution-time corpus (the pinned session and the planning-time mains had ALL rotated off — ladder step 2, live 2026-08-19 sessions, snapshot-frozen) and committed docs/compaction-decision.md `a0789e1` with the four required sections and a fully mechanical census: 910/910 cache_control placements on system blocks (`{"type":"ephemeral"}` exclusively), 176/176 tail records at len 64 with advancing offsets, zero resets/regressions, zero compact-continuation headers across 228 records.
- Dispositioned every behavior: 4 already-delivered-by-mimicry (Projector 64-tail, no-mid-turn-reset, system-role→user wire mapping, user-message structure), 2 gaps ROUTED post-adoption (cache_control emission — 14-04 consumes the facts only; cross-turn window span — 12-05 re-check), 2 absent-in-target with the honesty clause. ZERO implementation in-phase — `git diff` over internal/session + internal/shaper is empty.
- `mise ci` exit 0 (full battery green, zero failures).

## Task Commits

1. **Task 1: corpus scan + fixtures (tracer, TDD)** - `d981add` (test: RED tests + fixtures), `7931d30` (feat: scanner implementation)
2. **Task 2: run the scan, commit the decision artifact** - `a0789e1` (docs)

**Plan metadata:** (final commit below)

## Files Created/Modified

- `internal/profile/corpus_scan.go` — the scanner (315 lines): ScanContextBehavior, ContextBehaviorReport/WindowShape, generic cache_control path-walker, windowTracker
- `internal/profile/corpus_scan_test.go` — the four pinning tests (RED-first)
- `internal/profile/testdata/context-behavior/cache-control.jsonl` — redacted cache_control placement forms + marked probes
- `internal/profile/testdata/context-behavior/eviction-window.jsonl` — redacted window forms incl. a marked fabricated eviction probe
- `internal/profile/extract.go` — ModelIO.Request extended (messages/messagesKind/messageOffset/messageCount); extractor behavior unchanged
- `docs/compaction-decision.md` — the EARLY-02 decision artifact

## Decisions Made

- See key-decisions in frontmatter (cache_control routing; window delivered; absent-forms honesty clause; corpus snapshot discipline; request-level bookkeeping semantics).
- ModelIO extended rather than forked (plan mandate): the four request-level fields live on the shared struct; ExtractFromRollout ignores them.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Walker role/block context was map-iteration-order dependent**
- **Found during:** Task 1 (GREEN iteration)
- **Issue:** message-content cache_control classification read the role/type context during key iteration; Go map iteration order is random, so the placement key was nondeterministic (test failed ~intermittently).
- **Fix:** hoisted context extraction to the map level before iterating keys.
- **Files modified:** internal/profile/corpus_scan.go
- **Verification:** TestScanContextBehavior_CacheControlFixture green repeatedly; full battery green.
- **Committed in:** 7931d30 (part of the Task 1 GREEN commit, pre-commit fix)

**2. [Rule 3 - Blocking] Reproduction driver could not import the internal package**
- **Found during:** Task 2 (corpus run)
- **Issue:** `go run` of a driver placed outside the module fails on the internal-package restriction.
- **Fix:** ran the driver from a scratch dir inside the module; documented the in-module placement requirement in the artifact's reproduction section.
- **Files modified:** docs/compaction-decision.md (reproduction snippet notes the constraint)
- **Verification:** scan ran clean over all three sessions (exit 0).
- **Committed in:** a0789e1

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocker)
**Impact on plan:** None on scope — both were execution mechanics, fixed inline before the respective commits. Verify-first discipline intact (the corpus finding was never bent to the expected shape).

## Issues Encountered

- Corpus reality shifted between planning and execution (both planning-time main sessions rotated off; the dir holds only the live 2026-08-19 wave). Handled per the plan's own fallback ladder (step 2) with the snapshot + honesty notes in the artifact — not a blocker.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Ready for 14-03 (pi↔shaper audit) and then wave 2; 14-04's cache probe has its expected-placement baseline waiting in docs/compaction-decision.md §3 row 1.
- 12-05 inherits three named re-checks (long-session compaction markers, cross-turn span, cross-version cache_control placement stability).

---
*Phase: 14-adoption-readiness-analysis-dispositions*
*Completed: 2026-08-19*

## Self-Check: PASSED

All 6 created files exist on disk; all 4 plan commits (d981add RED, 7931d30 GREEN, a0789e1 artifact, 32f1a80 summary) verified in git log; mise ci exit 0.
