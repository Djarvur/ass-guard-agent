---
phase: 19-compaction-cache-control
verified: 2026-09-07T20:45:00Z
status: gaps_found
score: 12/14 must-haves verified
behavior_unverified: 1 # SC-1c live-LLM coherence leg (G-19-2 retest pending on the fresh binary) — present + wired, operator-gated by design
overrides_applied: 0
re_verification:
  previous_status: human_needed
  previous_score: 7/9
  gaps_closed:
    - "G-19-1 producing-turn rescue (operator ruling (b)): the 19-06 same-turn carve-out landed and is content-sensitively pinned for never-compacted sessions — the previous Human Item 1 (architect decision) is resolved and implemented"
  gaps_remaining:
    - "NEW residual (19-REVIEW CR-01, code-confirmed by this verifier): the carve-out is bypassed once any earlier compaction marker precedes the producing turn's user message — the retry is byte-identical and the turn fails; recovery works exactly once per session"
    - "Previous Human Item 2 (live-LLM threshold retest, G-19-2) still pending on the fresh binary installed 2026-09-07"
  regressions: []
gaps:
  - truth: "SC-3 / G-19-1: on a provider overflow error the producing turn completes after compaction + the single retry — the retry request projects post-marker (carries the summary), is measurably smaller, and a content-sensitive provider proves it"
    status: partial
    reason: "True and test-pinned ONLY for sessions with no pre-existing compaction marker before the producing turn's user message. Project() consults the pre-user scan FIRST (projector.go:153) and returns projectCompacted whenever ANY marker precedes the turn's user message; the G-19-1 carve-out (projector.go:161-165) runs only when that scan finds nothing. With an earlier marker M1 present, the forced compact's new marker M2 (TurnID-keyed, after the turn's user message) is invisible to the projection: foldExchanges' switch (projector.go:617-653) handles only tool/thinking/assistant line types, so M2 and the summarizer's lines change nothing and the retry is byte-identical to the rejected request — it deterministically overflows again and the turn fails through appendError after burning one summarizer call. Recovery therefore works exactly once per session (the never-compacted case the battery pins). Realistic doors to the residual: (1) compaction disabled after a marker exists (manual CompactNow earlier, or the same disabled-check backstop shape the G-19-1 battery itself uses — one config toggle away from the tested scenario); (2) WR-06 stale context limit after a per-session model switch to a smaller window (threshold fires past the real window, overflow arrives with M1 present). The battery cannot detect this: newSizeRejectSession plants no prior marker (zero AppendCompaction calls in compaction_test.go). 19-REVIEW CR-01 (Critical) independently reached the same conclusion."
    artifacts:
      - path: internal/session/projector.go
        issue: "Project branch order (:153 pre-user scan wins over the armed carve-out at :161); projectCompacted projects through the OLD marker; foldExchanges skips TypeCompaction/TypeUsage/TypeRequestShaped lines"
      - path: internal/session/session.go
        issue: "the retry branch (:573-585) arms SetRetryCompactedTurn unconditionally, but the arming is a no-op whenever a pre-user marker exists"
      - path: internal/session/compaction_test.go
        issue: "TestCompaction_OverflowRetryCarriesSummary has no leg planting a prior AppendCompaction marker before the producing turn's user message"
    missing:
      - "precedence fix: let the armed override take precedence over the pre-user scan (or have the carve-out accept the MOST RECENT marker regardless of position when retryCompactedTurn == turnID), keeping the TurnID key and the not-armed byte-identity pin"
      - "regression leg: plant a prior AppendCompaction marker before the producing turn's user message, drive the overflow, assert the retry is still strictly smaller and seeded with the NEW summary and the turn completes (19-REVIEW CR-01's fix sketch)"
behavior_unverified_items:
  - truth: "SC-1c: the user observes the conversation continuing coherently where v1.1 lost earlier turns (live threshold fire + summary-coherent continuation)"
    test: "On the fresh binary installed 2026-09-07 (v0.0.0-20260907153836-a8f63f31b1b0, engine string verified present per 19-UAT G-19-2 resolution), set compaction-threshold low in a live session, confirm compaction fires (marker on disk; coherent continuation across further turns), then restore 80"
    expected: "Earlier-turn facts survive in the summary seed; turns stay coherent; subsequent turns stay under the threshold. Note compaction is invisible in the editor UI (pre-authorized stderr+counter degrade; observability deferred to Phase 20's /status vehicle per 19-UAT Deferred Follow-Ups)"
    why_human: "The original live test failed on a stale pre-phase-19 binary (G-19-2 root cause: 16-05 pending no-op advertised the menu entry); the retest needs a real model and a real editor session — offline tests prove the machinery with a scripted summarizer, not live summary fidelity"
---

# Phase 19: Compaction + cache_control Verification Report

**Phase Goal:** Long sessions stop silently losing earlier turns: threshold-triggered light-tier compaction appends an additive typed marker the Projector treats as a durable reset-point class, and cache_control ephemeral breakpoints ship on every system block — the corpus-proven parity-faithful lever (zcode has no auto-compact; docs/compaction-decision.md settles design).
**Verified:** 2026-09-07T20:45:00Z
**Status:** gaps_found
**Re-verification:** Yes — supersedes the 2026-09-06 human_needed report (written before plan 19-06 landed); this run reflects the complete six-plan state including 19-06

## Goal Achievement

Machine-verified against the codebase and named tests this verifier executed itself: full session battery under `-race -count=1` (2.7s ok), each 19-06 battery member individually `-v` PASS, regression packages (`shaper`, `profile`, `paritycli`, `provider`, `modelrouting`, `providerfactory`) all ok, `acpserve` compaction surface tests (3) PASS, `go vet ./internal/session/` clean, `CGO_ENABLED=0 go build ./...` clean. All 19-06 commits (c4288fa..eb96dde) verified in git log. SUMMARY claims were cross-checked against code; the one place narrative and generality diverge (the G-19-1 residual) was confirmed from code independently of 19-REVIEW.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC-1a: over-threshold session compacts automatically (blocking compact at loop head, summary-bearing marker on disk, next turn seeded, headroom restored, disabled = zero delta) | ✓ VERIFIED | Regression green this run: `TestCompaction_Threshold`, `TestCompaction_Chaining`, `TestCompaction_SummarizerFailure`, `TestCompaction_LoopHead`, `TestCompactNow`, `TestCompaction_EndToEnd` all pass under `-race`; engine code previously read line-by-line (compaction.go blocking compact, D-09 degrade, session.go:514 loop head) — unchanged by 19-06 except the additive guard (truth 9) |
| 2 | SC-1b: threshold ~80% default and configurable end-to-end (config keys, menu advertise + effective values, persist-then-apply, live apply, clamps, no context-limit key) | ✓ VERIFIED | Re-run this run: `TestCompactionLive_BootDefaults`, `TestCompactionLive_MenuThresholdRoundTrip`, `TestCompactionOptions` PASS; `defaults/config.yaml:45-46` (threshold_pct 80 / enabled true) and `config_surface.go:51-52` both ids present |
| 3 | SC-1c: the user observes the conversation continuing coherently where v1.1 lost earlier turns | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Machinery fully proven offline (truth 1); the original live test failed on a stale pre-phase-19 binary (G-19-2 diagnosed: 16-05 pending no-op advertised the menu). Fresh binary installed 2026-09-07, retest required — see behavior_unverified_items |
| 4 | SC-2: tool_use/result pairs atomic, thinking never rewritten mid-chain, pinned Projector tests prove boundary-survival and pair-atomicity | ✓ VERIFIED | `TestProjector_CompactionResetPoint` + `TestProjector_CompactionTailCut` green under `-race` this run, bodies unmodified since 19-03 (git diff 06d92d1 over the three test files: 1006 insertions, 0 deletions) |
| 5 | SC-3 / G-19-1: on a provider overflow error the producing turn completes after compaction + the single retry — retry post-marker, measurably smaller, content-sensitively proven | ✗ PARTIAL (gap) | The carve-out exists, is wired, and works: `TestCompaction_OverflowRetryCarriesSummary` PASS (3 subtests: regression — retry strictly smaller, summary-seeded, turn completes, exactly 3 calls; arming scope; degraded fail-through byte-identical). BUT the truth holds only when NO earlier marker precedes the producing turn's user message: `projector.go:153` returns `projectCompacted` from the pre-user scan before the carve-out at `:161-165` is consulted, and `foldExchanges` skips marker/usage lines, so with a prior marker the armed retry is byte-identical and the turn fails. Recovery works exactly once per session. Zero `AppendCompaction` calls in compaction_test.go — the prior-marker leg is absent. Matches 19-REVIEW CR-01 (Critical); see Gaps Summary |
| 6 | 19-06 T2: the carve-out is ENGINE-GATED — transcript content alone never reshapes a projection (kill-9 replay + tamper safety) | ✓ VERIFIED | `Projector` code: carve-out requires non-empty exact `retryCompactedTurn == turnID` (in-memory, engine-set at session.go:582) AND the pre-user scan empty AND a TurnID-matched marker; pinned by the "not armed: marker-present projects byte-identically to marker-free" subtest (reflect.DeepEqual, projector_test.go:2244) — green |
| 7 | 19-06 T3: 19-03's position rule survives untouched; every existing 19-03/19-04 fixture passes UNMODIFIED | ✓ VERIFIED | "pre-user marker wins; the armed override cannot displace 19-03's scan" pin green (projector_test.go:2279); git diff 06d92d1 over projector_test.go / compaction_test.go / reconcile_test.go = 1006 insertions, 0 deletions (additions only) |
| 8 | WR-03a: summarizer input bounded — span capped by a budget derived from ContextLimit (fallback cap when unset) | ✓ VERIFIED | `spanBudgetChars` (compaction.go:515) derives from `compaction.ContextLimit` × fill-target share minus prevSummary/instruction/slack; `compactionSpanFallbackChars = 400_000` (:53), `compactionSpanMinChars = 2_000` (:59); `compact` renders `boundSpanMessages(fold, …)` (:290); span subtests green |
| 9 | WR-03b: at most ONE threshold-class compaction attempt per turn; forced compact stamps the guard; a new turn re-attempts | ✓ VERIFIED | `maybeCompact` guard (compaction.go:219-223: after check-counter + threshold test, before compact); session.go:580 stamps on the forced path; guard subtests of `TestCompaction_BoundedSpanAndReFireGuard` green |
| 10 | 19-06 T6: Reconcile classifies a mid-turn same-turn marker as inert bookkeeping; Project is a pure function of (lines, turnID, override) | ✓ VERIFIED | `TestReconcileSameTurnMarker` green (reconcile_test.go:610); determinism pinned inside `TestProjector_SameTurnCarveOut` |
| 11 | SC-4a: every outgoing request carries cache_control {"type":"ephemeral"} on each system block (keep-last-4 at the API cap), visible in the marshaled body | ✓ VERIFIED | Regression green: shaper + profile packages ok; `profiles/zcode/profile.yaml` `system_cache_control: true`; `shaper.go:167-183` gated emission via `NewCacheControlEphemeralParam` confined to the systemBlocks loop (grep: no tools/messages placement) |
| 12 | SC-4b: the parity cache-discipline probe flips green on placement against the committed pin fixture; baseline untouched | ✓ VERIFIED | `TestCacheProbe_PlacementFlip` green (paritycli package ok this run); `git diff --name-only 73440bd -- internal/parity/cacheprobe.go` EMPTY — prohibition held across the whole phase including 19-06 |
| 13 | PAR-01 substrate (19-02): non-2xx rejections surface as ClassifyHTTP-typed error chunks (never a defaulted end_turn done chunk); IsOverflow message-class matcher, no new ErrorKind; happy SSE path unchanged | ✓ VERIFIED | provider package ok this run (`TestStream_Non2xxError`, `TestIsOverflow` included). Note: 19-REVIEW CR-02 (in-band SSE `error` events swallowed → fabricated end_turn) is PRE-EXISTING, in the 200-status mid-stream path — outside this truth's non-2xx scope; carried as review debt below |
| 14 | 19-03: summary payload additive on the marker line (redacted path, round-trip, field-tolerant); no-marker transcripts project byte-identically | ✓ VERIFIED | `TestTranscriptNewKinds` + no-marker identity pins green under `-race` this run |

**Score:** 12/14 truths verified (1 partial gap, 1 present-but-behavior-unverified)

### Prohibition Checks

| Prohibition | Verdict | Evidence |
|-------------|---------|----------|
| 19-06 tamper-safety: projector never accepts a same-turn marker from transcript content alone | VERIFIED | Engine gate (non-empty exact turnID match, in-memory only); not-armed DeepEqual pin green |
| 19-06 test-integrity: existing pinned fixtures MUST NOT be edited | VERIFIED | git diff 06d92d1 over the three test files: 1006 insertions, 0 deletions |
| 19-06 retry-budget: retry stays EXACTLY ONCE | VERIFIED | `overflowRetried` local (session.go:508), guard at :573, never reset; fail-twice leg green (exactly 3 stream calls) |
| 19-01 cache_control never on tools/message blocks | VERIFIED | shaper grep: emission only in the systemBlocks loop; regression green |
| 19-01 probe baseline never edited | VERIFIED | `git diff 73440bd -- internal/parity/cacheprobe.go` empty (re-checked this run) |
| 19-01 no field beyond type ephemeral | VERIFIED | `NewCacheControlEphemeralParam` only; golden pin green |
| 19-02 no new ErrorKind; no generic-400 overflow misclassification | VERIFIED | provider package green incl. false-table |
| 19-02 error-envelope parsing never panics | VERIFIED | malformed-body subtest green |
| 19-03 transcript never mutated by compaction | VERIFIED | append-only paths; replay-identity pins green |
| 19-03 durable seed never displaced; tail never orphaned | VERIFIED | ResetPoint/TailCut pins green, unmodified |
| 19-04 bus isolation; estimate never request bytes | VERIFIED | zero-bus-publishes + estimate subtests green |
| 19-05 no context-limit key; out-of-range never persisted | VERIFIED | no key in config.go/defaults; `TestCompactionOptions` green |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/session/projector.go` | carve-out seam + sameTurnMarkerIdx + projectSameTurnCompacted + position rule intact | ✓ VERIFIED | :95 retryCompactedTurn, :112 setter, :161-165 branch, :236-268 projectSameTurnCompacted, :384/:407 both scans — substantive and wired |
| `internal/session/session.go` | retry branch arms override + stamps guard; compactionAttemptTurn field | ✓ VERIFIED | :508, :573-585, :188-197 |
| `internal/session/compaction.go` | spanBudgetChars + boundSpanMessages + once-per-turn guard | ✓ VERIFIED | :219-223, :290, :507-551, named constants :49-62 |
| `internal/session/compaction_test.go` | sizeReject fixture + G-19-1 battery | ✓ VERIFIED | :1603 sizeRejectProvider (content-sensitive by construction), :1704 battery — all green; caveat: no prior-marker leg (gap 1) |
| `internal/session/projector_test.go` | TestProjector_SameTurnCarveOut (8 pins) | ✓ VERIFIED | :2187, incl. not-armed byte-identity and pre-user-marker-wins |
| `internal/session/reconcile_test.go` | TestReconcileSameTurnMarker | ✓ VERIFIED | :610, green |
| `internal/session/transcript.go` | TypeCompaction doc rule update | ✓ VERIFIED | full rule stated (resets turns that START after it + engine-armed carve-out) |
| 19-01..19-05 artifacts (profile/loader/shaper/paritycli/provider/modelrouting/config_surface/runtime) | per plans | ✓ VERIFIED | regression packages green this run; symbols spot-checked (see truths 1, 2, 11, 12, 13, 14) |

All artifacts: exists + substantive + wired (Level 3) + real data sources (Level 4). No static/hollow data paths; the sizeReject provider is test-only by design.

### Key Link Verification

| From | To | Via | Status |
|------|----|----|--------|
| session.go overflow retry branch | projector.go SetRetryCompactedTurn | arming between forced compact and continue (session.go:582) | ✓ WIRED |
| compaction.go span budget | compactionSettings.ContextLimit | spanBudgetChars derives from the same resolved window (:515, used :290) | ✓ WIRED |
| profiles/zcode/profile.yaml | shaper.go | system_cache_control → loader fan-out → NewCacheControlEphemeralParam | ✓ WIRED |
| config_surface.go | session SetCompactionSettings | SetCompactionHook → runner.ApplyCompactionSettings (live tests green) | ✓ WIRED |
| provider/errors.go IsOverflow | session.go retry branch | intercept before appendError (:573) | ✓ WIRED |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full session battery (13 test fns) | `go test -race ./internal/session/ -run 'TestProjector_SameTurnCarveOut\|TestCompaction_OverflowRetryCarriesSummary\|TestCompaction_BoundedSpanAndReFireGuard\|TestReconcileSameTurnMarker\|TestCompaction_OverflowRetryOnce\|TestProjector_CompactionResetPoint\|TestProjector_CompactionTailCut\|TestCompaction_EndToEnd\|TestCompactNow\|TestCompaction_Threshold\|TestCompaction_Chaining\|TestCompaction_SummarizerFailure\|TestCompaction_LoopHead' -count=1` | ok 2.748s | ✓ PASS |
| Each 19-06 battery member | same filter, `-v` | 5/5 --- PASS | ✓ PASS |
| Regression packages | `go test ./internal/shaper/ ./internal/profile/ ./internal/paritycli/ ./internal/provider/ ./internal/modelrouting/ ./internal/providerfactory/ -count=1` | all ok | ✓ PASS |
| acpserve compaction surface | `go test ./internal/acpserve/ -run 'TestCompaction' -count=1 -v` | 3 PASS | ✓ PASS |
| Probe baseline untouched | `git diff --name-only 73440bd -- internal/parity/cacheprobe.go` | empty | ✓ PASS |
| Pinned fixtures unmodified | `git diff 06d92d1 -- <three test files>` | 1006 insertions, 0 deletions | ✓ PASS |
| Vet + static build | `go vet ./internal/session/` / `CGO_ENABLED=0 go build ./...` | clean / BUILD-OK | ✓ PASS |

### Probe Execution

Step 7c: SKIPPED — no probe scripts declared by this phase (`scripts/*/tests/probe-*.sh` convention not used by this repo's phase plans; the parity A/B probe is operator-gated live-model work by design).

### Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| PAR-01 | 19-02, 19-03, 19-04, 19-05, 19-06 | Compaction on context overflow risk: threshold-triggered configurable light-tier summarizer, additive typed marker, durable reset-point class, pair atomicity, thinking never rewritten, retry-once recovery | ⚠️ PARTIAL | Truths 1, 2, 4, 6-10, 13, 14 verified. The retry-once recovery letter is once-per-session (gap 1: carve-out bypassed after the session's first compaction marker); the live-coherence leg awaits the G-19-2 retest (behavior-unverified item) |
| PAR-02 | 19-01 | cache_control {"type":"ephemeral"} on every system block via the Shaper | ✓ SATISFIED | Truths 11, 12 |

No orphaned requirements: REQUIREMENTS.md maps exactly PAR-01 and PAR-02 to Phase 19; all six plans declare them (19-01: PAR-02; 19-02..06: PAR-01).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER in any of the 7 files 19-06 modified; no stdout writes — ACP frame discipline holds | - | - |

Review findings carried (19-REVIEW.md, status issues_found — factored per coordinator instruction):
- **CR-01 residual (Critical)** — mapped to truth 5 / gap 1 (the one finding that touches a roadmap criterion). Not deferred to any later phase (Phases 20-25 goals checked — none covers compaction retry hardening).
- **CR-02 (Critical, code health)** — in-band SSE `error` events swallowed, fabricated end_turn (`streaming.go:443-531`, `:651-678`). PRE-EXISTING (untouched by this phase) and outside 19-02's pinned non-2xx scope, so it fails no phase must-have; it is the same swallow-class 19-02 eliminated for non-2xx and directly adjacent to PAR-01's error-surfacing contract — schedule as engineering debt (the review supplies the fix sketch and test shape).
- ⚠️ WR-01 (blob-fill advertisement can diverge from enforcement), WR-02 (estimate anchor async drift, bounded by the once-per-turn guard), WR-04 (session-id path traversal, pre-existing), WR-05 (marker pre/post refs race the async writer, write-only today), WR-06 (stale context limit after per-session model switch — compounds gap 1's second door). ℹ️ IN-01 fixed by 19-06; IN-02 still broken (always-empty path in an error message); IN-03..08 cosmetic/pre-existing.

### Human Verification Required

### 1. Operator live-threshold retest on the fresh binary (G-19-2 / SC-1c)

**Test:** On the binary installed 2026-09-07 (v0.0.0-20260907153836-a8f63f31b1b0 — engine string verified present per the UAT diagnosis), set compaction-threshold low in a live editor session, keep working, then restore 80.
**Expected:** Compaction observably fires (marker line on disk under `.ass-guard/`; continuation stays coherent; later turns stay under threshold). Expect NO visible indicator in the editor — the pre-authorized stderr+counter degrade; the observability UX decision is deferred to Phase 20's /status vehicle.
**Why human:** The first live attempt ran a stale pre-phase-19 binary whose menu entry was 16-05's pending no-op; summary fidelity under a real model is the research validation table's operator-gated leg.

(The previous report's Human Item 1 — the architect decision on producing-turn semantics — is CLOSED: ruling (b) recorded 2026-09-07, implemented by 19-06, content-sensitively pinned. Its residue is the CR-01 gap below, which is an engineering fix, not a decision.)

### Gaps Summary

One gap blocks clean closure. G-19-1's producing-turn rescue (operator ruling (b)) is implemented and content-sensitively pinned — but only for a session's FIRST compaction. `Project` consults the pre-user marker scan before the armed carve-out, and `foldExchanges` is marker-blind, so once any earlier marker precedes the producing turn's user message the retried request is byte-identical to the rejected one: it overflows again, the turn fails through `appendError`, and the forced compact's summarizer call was burned for nothing. This verifier confirmed the branch order and the fold skip directly in code (independently of 19-REVIEW), and confirmed the battery has no prior-marker leg (zero `AppendCompaction` calls in compaction_test.go). The main-line goal path — threshold-triggered compaction with durable seed, pair-safe tail, and coherent continuation — is unaffected: the threshold engine fires BEFORE overflow in the enabled/configuration-correct case, so the residual is the backstop degrading to once-per-session (reachable via compaction-disabled-with-prior-marker, or WR-06's stale-limit door). Fix per the review: give the armed override precedence (or accept the most recent marker regardless of position for the matching turnID) and add the prior-marker regression leg. CR-02 (pre-existing in-band SSE error swallow) and WR-01..06 are carried as review debt; none blocks a phase must-have. The G-19-2 live retest remains the operator's pending human leg.

---

_Verified: 2026-09-07T20:45:00Z_
_Verifier: ZCode (gsd-verifier)_
