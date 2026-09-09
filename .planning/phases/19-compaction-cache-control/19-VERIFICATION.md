---
phase: 19-compaction-cache-control
verified: 2026-09-07T18:14:00Z
status: passed
score: 13/14 must-haves verified
behavior_unverified: 1 # SC-1c live-LLM coherence leg (G-19-2 retest pending on the fresh binary) — present + wired, operator-gated by design
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 12/14
  gaps_closed:
    - "CR-01 residual (gap 1 / truth 5): the armed carve-out now takes precedence over the pre-user compactionMarkerIdx scan in Project() (projector.go:170-174 before :176-178); the prior-marker regression legs exist at both the projector level (armed-override-wins inverted pin + fallback pin, projector_test.go:2279/:2318) and the session level (prior-marker overflow leg with planted AppendCompaction M1 + AppendBoundary, compaction_test.go:1912) — this verifier ran both legs GREEN on HEAD and independently reproduced the RED proof on commit b301267 (exactly the two new legs fail there, every other subtest passes)"
  gaps_remaining: [] # no code gaps remain; the G-19-2 operator live retest is a human leg (below), not a code gap
  regressions: []
behavior_unverified_items:

  - truth: "SC-1c: the user observes the conversation continuing coherently where v1.1 lost earlier turns (live threshold fire + summary-coherent continuation)"
    test: "On the fresh binary installed 2026-09-07 (v0.0.0-20260907153836-a8f63f31b1b0, engine string verified present per 19-UAT G-19-2 resolution), set compaction-threshold low in a live session, confirm compaction fires (marker on disk; coherent continuation across further turns), then restore 80"
    expected: "Earlier-turn facts survive in the summary seed; turns stay coherent; subsequent turns stay under the threshold. Note compaction is invisible in the editor UI (pre-authorized stderr+counter degrade; observability deferred to Phase 20's /status vehicle per 19-UAT Deferred Follow-Ups)"
    why_human: "The original live test failed on a stale pre-phase-19 binary (G-19-2 root cause: 16-05 pending no-op advertised the menu entry); the retest needs a real model and a real editor session — offline tests prove the machinery with a scripted summarizer, not live summary fidelity"
human_verification:

  - test: "Operator live-threshold retest on the fresh binary (G-19-2 / SC-1c): set compaction-threshold low (e.g. 5) in a live editor session against the 2026-09-07 binary (v0.0.0-20260907153836-a8f63f31b1b0), keep working, then restore 80"
    expected: "Compaction observably fires (marker line on disk under .ass-guard/; continuation stays coherent; later turns stay under threshold). Expect NO visible indicator in the editor — the pre-authorized stderr+counter degrade; the observability UX decision is deferred to Phase 20's /status vehicle"
    why_human: "The first live attempt ran a stale pre-phase-19 binary whose menu entry was 16-05's pending no-op; summary fidelity under a real model is the research validation table's operator-gated leg — offline tests prove the machinery with a scripted summarizer, not live summary fidelity"
---

# Phase 19: Compaction + cache_control Verification Report

**Phase Goal:** Long sessions stop silently losing earlier turns: threshold-triggered light-tier compaction appends an additive typed marker the Projector treats as a durable reset-point class, and cache_control ephemeral breakpoints ship on every system block — the corpus-proven parity-faithful lever (zcode has no auto-compact; docs/compaction-decision.md settles design).
**Verified:** 2026-09-07T18:14:00Z
**Status:** human_needed
**Re-verification:** Yes — supersedes the 2026-09-07 gaps_found report (CR-01); this run reflects the complete seven-plan state including the 19-07 gap closure

## Verification Method Note (prior spot-check table vacuous — corrected here)

The prior report's Behavioral Spot-Checks table quoted the battery command with `'\|'` separators. In Go's regexp, `\|` is a LITERAL pipe character, so that pattern matched ZERO test functions — the prior "ok 2.748s" line proved only that zero selected tests cannot fail. This report's evidence comes from commands this verifier executed itself with CORRECT alternation (single-quoted `-run 'A|B|C'`, plain `|`), and from `-v` output proving each named function actually ran. The conclusions of the prior report survive this correction because every claim it made has now been re-established with real runs (below) — but the prior table's evidence line was vacuous and is superseded.

## Goal Achievement

Machine-verified against the committed codebase (HEAD) and named tests this verifier executed itself: full 13-function session battery under `-race -count=1` — ok 2.394s with all 13 functions individually `--- PASS`; the 12 subtests of the two CR-01 battery functions individually PASS (including both new legs); RED proof independently reproduced on commit b301267 (exactly the two new CR-01 legs FAIL pre-fix, all sibling subtests pass); six regression packages all ok; acpserve compaction surface 3/3 PASS (on the committed tree — see Concurrent-Execution Note); `go vet ./internal/session/` clean; `CGO_ENABLED=0 go build ./...` clean (committed tree); diff audits vs 06d92d1 (1103 insertions, 0 deletions) and vs 73440bd (cacheprobe.go untouched) re-run this session.

### Concurrent-Execution Note (read before judging any build failure)

Phase 20 is executing concurrently in this working tree. Untracked files `internal/runtime/commands.go` / `commands_test.go` (git status `??`, never committed) currently break `go build` of `internal/runtime` and its dependents (including `internal/acpserve`) IN THE WORKING TREE. This is not Phase 19 state: `internal/session/` is clean vs HEAD (zero diff), and on the committed tree (`git archive HEAD` extract) `CGO_ENABLED=0 go build ./...` is clean and the acpserve compaction surface passes 3/3. All Phase 19 verdicts below are given against the committed state.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC-1a: over-threshold session compacts automatically (blocking compact at loop head, summary-bearing marker on disk, next turn seeded, headroom restored, disabled = zero delta) | ✓ VERIFIED | Re-run this session (correct alternation): `TestCompaction_Threshold`, `TestCompaction_Chaining`, `TestCompaction_SummarizerFailure`, `TestCompaction_LoopHead`, `TestCompactNow`, `TestCompaction_EndToEnd` all `--- PASS` under `-race`; engine code re-read (unchanged by 19-07 except the additive precedence guard) |
| 2 | SC-1b: threshold ~80% default and configurable end-to-end (config keys, menu advertise + effective values, persist-then-apply, live apply, clamps, no context-limit key) | ✓ VERIFIED | Committed-tree run this session: `TestCompactionLive_BootDefaults`, `TestCompactionLive_MenuThresholdRoundTrip`, `TestCompactionOptions` all PASS; `internal/modelrouting/defaults/config.yaml` carries `threshold_pct: 80` / `enabled: true` |
| 3 | SC-1c: the user observes the conversation continuing coherently where v1.1 lost earlier turns | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Machinery fully proven offline (truth 1 + the CR-01-hardened recovery of truth 5); the original live test failed on a stale pre-phase-19 binary (G-19-2 diagnosed). Fresh binary installed 2026-09-07, retest required — see behavior_unverified_items / Human Verification |
| 4 | SC-2: tool_use/result pairs atomic, thinking never rewritten mid-chain, pinned Projector tests prove boundary-survival and pair-atomicity | ✓ VERIFIED | `TestProjector_CompactionResetPoint` + `TestProjector_CompactionTailCut` `--- PASS` under `-race` this session; bodies untouched by 19-07 (0 deletions vs 06d92d1) |
| 5 | SC-3 / G-19-1 + CR-01 closure: on a provider overflow error the producing turn completes after compaction + the single retry — the retry projects post-marker (carries the NEW summary), is measurably smaller, on EVERY overflow including sessions with an earlier marker | ✓ VERIFIED | **Code:** `Project()` now evaluates the armed carve-out FIRST (projector.go:170-174: `retryCompactedTurn != "" && == turnID` + `sameTurnMarkerIdx >= 0` → `projectSameTurnCompacted`) BEFORE the pre-user scan (:176-178 `compactionMarkerIdx` → `projectCompacted`). **Tests (this session's runs):** `TestCompaction_OverflowRetryCarriesSummary` all 4 subtests PASS — the new `prior_marker:` leg (compaction_test.go:1912) drives a planted `t-prior` marker + boundary through `runTurn` behind the content-sensitive size-rejecting provider and asserts stop==stopEndTurn, exactly 3 stream calls, markersOf==2, retry strictly smaller, seed carries `SHRUNK-PRIOR-SUMMARY` + the turn's intent, excludes `OLD-PRIOR-SUMMARY`, no provider error line. **Projector level:** `TestProjector_SameTurnCarveOut/armed_override_wins_over_the_pre-user_marker_scan_(CR-01)` PASS (asserts same-turn summary present / pre-user summary absent / intent present). **RED proof reproduced by this verifier:** on commit b301267 (tests-without-fix) exactly these two legs FAIL and every sibling subtest passes — the regression detector genuinely detects the defect, and commit 35d1740 turned exactly them green |
| 6 | 19-06 T2 + 19-07: the carve-out is ENGINE-GATED — transcript content alone never reshapes a projection (kill-9 replay + tamper safety) | ✓ VERIFIED | Carve-out still requires the non-empty, exact in-memory `retryCompactedTurn == turnID` (engine-set only at session.go's overflow-retry branch); pinned by the UNMODIFIED `not_armed:_marker-present_projects_byte-identically_to_marker-free` DeepEqual subtest — PASS this session; `subagent_turn_safe` pin PASS |
| 7 | 19-03 position rule survives; every 19-03/19-04 fixture passes UNMODIFIED (the one edited pin is the 19-06-era precedence subtest, deliberately inverted by the sanctioned 19-07 plan — its old expectation encoded the CR-01 defect) | ✓ VERIFIED | `git diff 06d92d1` over projector_test.go + compaction_test.go = 1103 insertions, **0 deletions** (re-run this session); the 19-03 pins (ResetPoint/TailCut) pass untouched; the fallback pin (`armed_with_no_same-turn_marker:_the_pre-user_scan_governs_unchanged`) PASSes both pre- and post-fix, preserving the not-armed/degraded behavior verbatim |
| 8 | WR-03a: summarizer input bounded — span capped by a budget derived from ContextLimit (fallback cap when unset) | ✓ VERIFIED | `TestCompaction_BoundedSpanAndReFireGuard` `--- PASS` under `-race` this session; `spanBudgetChars`/`boundSpanMessages` unchanged by 19-07 |
| 9 | WR-03b: at most ONE threshold-class compaction attempt per turn; forced compact stamps the guard; a new turn re-attempts | ✓ VERIFIED | Same battery PASS this session; guard stamps unchanged by 19-07 |
| 10 | 19-06 T6: Reconcile classifies a mid-turn same-turn marker as inert bookkeeping; Project is a pure function of (lines, turnID, override) | ✓ VERIFIED | `TestReconcileSameTurnMarker` `--- PASS` under `-race` this session; determinism subtest of the carve-out battery PASS |
| 11 | SC-4a: every outgoing request carries cache_control {"type":"ephemeral"} on each system block (keep-last-4 at the API cap), visible in the marshaled body | ✓ VERIFIED | shaper + profile packages ok this session; `profiles/zcode/profile.yaml:5` `system_cache_control: true`; emission via `NewCacheControlEphemeralParam` confined to the systemBlocks loop (shaper.go:167-180) |
| 12 | SC-4b: the parity cache-discipline probe flips green on placement against the committed pin fixture; baseline untouched | ✓ VERIFIED | paritycli package ok this session (`TestCacheProbe_PlacementFlip` at parity_test.go:417); `git diff --name-only 73440bd -- internal/parity/cacheprobe.go` EMPTY — re-checked this session |
| 13 | PAR-01 substrate (19-02): non-2xx rejections surface as ClassifyHTTP-typed error chunks (never a defaulted end_turn done chunk); IsOverflow message-class matcher, no new ErrorKind; happy SSE path unchanged | ✓ VERIFIED | provider package ok this session; `IsOverflow` at errors.go:158. CR-02 (in-band SSE `error` events swallowed, streaming.go:443-531/:651-678) remains PRE-EXISTING review debt — outside this truth's non-2xx scope (carried below) |
| 14 | 19-03: summary payload additive on the marker line (redacted path, round-trip, field-tolerant); no-marker transcripts project byte-identically | ✓ VERIFIED | `TestTranscriptNewKinds` `--- PASS` under `-race` this session; no-marker identity pins inside the battery PASS |

**Score:** 13/14 truths verified (1 present-but-behavior-unverified — the G-19-2 live leg)

### 19-07 Gap-Closure Plan must_haves (all verified)

| 19-07 Truth | Status | Evidence |
|-------------|--------|----------|
| Armed retry with a prior marker resolves through projectSameTurnCompacted; seed carries the SAME-TURN summary, never the earlier marker's | ✓ VERIFIED | projector.go:170-174 branch order (read this session); armed-override-wins pin PASS; RED on b301267 |
| Prior-marker overflow leg completes end-to-end (rejected → bounded summarize → M2 → one strictly-smaller NEW-summary-seeded retry, stopEndTurn, no error line, exactly 3 calls) | ✓ VERIFIED | prior_marker subtest PASS with every assertion (compaction_test.go:1912-1974); RED on b301267 |
| Tamper safety unchanged (not-armed byte-identity DeepEqual pin unmodified and passing; engine gate intact) | ✓ VERIFIED | Not-armed subtest PASS; 0 deletions vs 06d92d1 |
| Degraded compact keeps the fail-through (armed + no same-turn marker → pre-user scan governs) | ✓ VERIFIED | Fallback pin PASS post-fix AND pre-fix (b301267 run); existing degraded fail-through subtest PASS |
| Every other pinned fixture passes unmodified; only the inverted precedence subtest edited | ✓ VERIFIED | 1103 insertions / 0 deletions vs 06d92d1; full 13-fn battery green |

### Prohibition Checks

| Prohibition | Verdict | Evidence |
|-------------|---------|----------|
| Tamper safety: projector never accepts a same-turn marker from transcript content alone | VERIFIED | Engine gate (non-empty exact turnID, in-memory only); not-armed DeepEqual pin green this session |
| Test integrity: existing pinned fixtures MUST NOT be edited | VERIFIED | 0 deletions vs 06d92d1 across both test files (the inverted subtest replaced lines that were themselves post-06d92d1 insertions) |
| Retry budget: retry stays EXACTLY ONCE | VERIFIED | prior-marker leg asserts exactly 3 stream calls (rejected, summarize, retry); fail-twice canary green |
| cache_control never on tools/message blocks | VERIFIED | shaper emission confined to systemBlocks loop; shaper package green |
| Probe baseline never edited | VERIFIED | Empty diff vs 73440bd (this session) |
| No field beyond type ephemeral | VERIFIED | `NewCacheControlEphemeralParam` only; golden pins green |
| No new ErrorKind; no generic-400 overflow misclassification | VERIFIED | provider package green |
| Error-envelope parsing never panics | VERIFIED | malformed-body subtest green |
| Transcript never mutated by compaction | VERIFIED | append-only paths; replay-identity pins green |
| Durable seed never displaced; tail never orphaned | VERIFIED | ResetPoint/TailCut pins green, unmodified |
| Bus isolation; estimate never request bytes | VERIFIED | zero-bus-publishes + estimate subtests green |
| No context-limit key; out-of-range never persisted | VERIFIED | `TestCompactionOptions` green |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/session/projector.go` | armed carve-out PRECEDENCE over pre-user scan + doc sites | ✓ VERIFIED | :170-174 carve-out first, :176-178 pre-user scan second (line-order check this session); docs at :103-115 (setter), :140-154 (Project), :161-169 (inline) all state precedence + fallback |
| `internal/session/projector_test.go` | inverted precedence pin + fallback pin | ✓ VERIFIED | :2279 armed-override-wins, :2318 fallback pin — both substantive, both PASS |
| `internal/session/compaction_test.go` | prior-marker regression leg + near-copy helper | ✓ VERIFIED | :1698-1754 `newPriorMarkerSizeRejectSession` (load-bearing AppendBoundary documented), :1912-1974 the leg — full assertion set, PASS |
| `internal/session/session.go` | comment-only precedence note | ✓ VERIFIED | Commit 35d1740's session.go hunk is a 4-line comment extension, zero code change (diff inspected) |
| 19-01..19-06 artifacts (profile/loader/shaper/paritycli/provider/modelrouting/config_surface/runtime/transcript) | per plans | ✓ VERIFIED | Regression packages green this session; symbols spot-checked (truths 1, 2, 11, 12, 13, 14) |

All artifacts: exists + substantive + wired. The size-reject provider family is test-only by design.

### Key Link Verification

| From | To | Via | Status |
|------|----|----|--------|
| session.go overflow-retry branch | projector.go SetRetryCompactedTurn | arming between forced compact and continue | ✓ WIRED (unchanged by 19-07; the retry now benefits from the precedence flip) |
| Project() armed branch | projectSameTurnCompacted | `retryCompactedTurn == turnID` + `sameTurnMarkerIdx >= 0` → returns before the pre-user scan | ✓ WIRED — this is the CR-01 fix itself, RED→GREEN proven |
| Armed-with-no-marker fallback | projectCompacted via compactionMarkerIdx | branch falls through to :176 | ✓ WIRED (fallback pin green both pre- and post-fix) |
| compaction.go span budget | compactionSettings.ContextLimit | spanBudgetChars → boundSpanMessages | ✓ WIRED |
| profiles/zcode/profile.yaml | shaper.go | system_cache_control → NewCacheControlEphemeralParam | ✓ WIRED |
| provider/errors.go IsOverflow | session.go retry branch | intercept before appendError | ✓ WIRED |

### Behavioral Spot-Checks

All commands executed by this verifier with CORRECT Go regexp alternation (plain `|` inside single quotes). The prior report's `'\|'` form matched zero tests and is superseded.

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full session battery (13 functions) | `go test -race ./internal/session/ -run 'TestProjector_SameTurnCarveOut|TestCompaction_OverflowRetryCarriesSummary|TestCompaction_BoundedSpanAndReFireGuard|TestReconcileSameTurnMarker|TestCompaction_OverflowRetryOnce|TestProjector_CompactionResetPoint|TestProjector_CompactionTailCut|TestCompaction_EndToEnd|TestCompactNow|TestCompaction_Threshold|TestCompaction_Chaining|TestCompaction_SummarizerFailure|TestCompaction_LoopHead' -count=1` | ok 2.394s; `-v` run lists all 13 functions `--- PASS` individually (13/13) | ✓ PASS |
| Both CR-01 battery functions, subtest level | same two-name filter, `-v` | 12/12 subtests `--- PASS`, incl. `armed_override_wins…(CR-01)` and `prior_marker…(CR-01)` | ✓ PASS |
| RED proof (pre-fix tree, commit b301267 via `git archive` extract — no working-tree mutation) | `go test ./internal/session/ -run 'TestProjector_SameTurnCarveOut|TestCompaction_OverflowRetryCarriesSummary' -count=1 -v` | exactly the two new CR-01 legs `--- FAIL`; all sibling subtests PASS | ✓ PASS (detector proven) |
| Regression packages | `go test ./internal/shaper/ ./internal/profile/ ./internal/paritycli/ ./internal/provider/ ./internal/modelrouting/ ./internal/providerfactory/ -count=1` | all 6 ok (0.7s–3.7s) | ✓ PASS |
| acpserve compaction surface (committed tree) | `git archive HEAD` extract → `go test ./internal/acpserve/ -run 'TestCompaction' -count=1 -v` | 3/3 PASS (BootDefaults, MenuThresholdRoundTrip, Options); working-tree run blocked by Phase 20's untracked `internal/runtime/commands.go` — not Phase 19 state | ✓ PASS |
| Transcript additivity | `go test -race ./internal/session/ -run 'TestTranscriptNewKinds' -count=1` | `--- PASS` | ✓ PASS |
| Probe baseline untouched | `git diff --name-only 73440bd -- internal/parity/cacheprobe.go` | empty | ✓ PASS |
| Pinned fixtures unmodified | `git diff 06d92d1 --numstat -- internal/session/projector_test.go internal/session/compaction_test.go` | 734/0 + 369/0 (1103 insertions, 0 deletions) | ✓ PASS |
| Vet + static build (committed tree) | `go vet ./internal/session/` / `CGO_ENABLED=0 go build ./...` on `git archive HEAD` extract | VET-OK / BUILD-OK | ✓ PASS |
| Gap-closure commits exist | `git log -1 b301267 / 35d1740 / 41a5c63` | all three present; 35d1740 touches exactly projector.go + session.go (session.go comment-only) | ✓ PASS |

### Probe Execution

Step 7c: SKIPPED — no probe scripts declared by this phase (`scripts/*/tests/probe-*.sh` convention not used by this repo's phase plans; the parity A/B probe is operator-gated live-model work by design).

### Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| PAR-01 | 19-02, 19-03, 19-04, 19-05, 19-06, 19-07 | Compaction on context overflow risk: threshold-triggered configurable light-tier summarizer, additive typed marker, durable reset-point class, pair atomicity, thinking never rewritten, retry-once recovery | ✓ SATISFIED | Truths 1, 2, 4-10, 13, 14. The CR-01 gap is closed: retry-once recovery now holds on every overflow (prior marker or not), RED→GREEN proven independently. The live-coherence leg (SC-1c) is the one remaining item — an operator retest on live infra (behavior-unverified), not an implementation gap |
| PAR-02 | 19-01 | cache_control {"type":"ephemeral"} on every system block via the Shaper | ✓ SATISFIED | Truths 11, 12 — untouched by 19-07, re-confirmed green this session |

No orphaned requirements: REQUIREMENTS.md maps exactly PAR-01 and PAR-02 to Phase 19; all seven plans declare them (19-01: PAR-02; 19-02..07: PAR-01). REQUIREMENTS.md rows updated per this verdict: PAR-01 `[x]` Complete (set by 41a5c63, endorsed here); PAR-02 restored to `[x]` Complete — its "Gaps Found" was collateral from the a2752b8 blanket revert, and PAR-02 has been verified SATISFIED in both this and the prior report.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/session/projector_test.go | ~2177 | Stale battery-level doc comment still describes the OLD precedence ("ONLY when … AND no pre-user marker exists") | ⚠️ Warning (doc-only) | Deliberately deferred (deferred-items.md 19-07 entry): fixing it inside 19-07 would have produced deletions outside the inverted subtest and violated the zero-deletions audit; fix opportunistically in the next plan touching the file |
| (19-07 modified files) | - | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER; no stdout writes — ACP frame discipline holds | - | - |

Review findings carried (19-REVIEW.md, status issues_found — factored per coordinator instruction; none blocks a phase must-have):

- **CR-01 (Critical)** — CLOSED by 19-07 (this verification's core finding).
- **CR-02 (Critical, code health)** — in-band SSE `error` events swallowed, fabricated end_turn (`streaming.go:443-531`, `:651-678`). PRE-EXISTING and outside 19-02's pinned non-2xx scope; schedule as engineering debt (fix sketch + test shape in the review).
- ⚠️ WR-01 (blob-fill advertisement divergence), WR-02 (estimate anchor async drift, bounded by the once-per-turn guard), WR-04 (session-id path traversal, pre-existing), WR-05 (marker pre/post refs race the async writer, write-only today), WR-06 (stale context limit after per-session model switch — the second door to the overflow backstop; the CR-01 fix means the backstop now actually recues these sessions instead of failing them). ℹ️ IN-01 fixed by 19-06; IN-02 still broken (always-empty `firstFile` in extract.go's error, re-confirmed this session); IN-03..08 cosmetic/pre-existing.

### Human Verification Required

### 1. Operator live-threshold retest on the fresh binary (G-19-2 / SC-1c)

**Test:** On the binary installed 2026-09-07 (v0.0.0-20260907153836-a8f63f31b1b0 — engine string verified present per the UAT diagnosis), set compaction-threshold low (e.g. 5) in a live editor session, keep working, then restore 80.
**Expected:** Compaction observably fires (marker line on disk under `.ass-guard/`; continuation stays coherent; later turns stay under threshold). Expect NO visible indicator in the editor — the pre-authorized stderr+counter degrade; the observability UX decision is deferred to Phase 20's /status vehicle.
**Why human:** The first live attempt ran a stale pre-phase-19 binary whose menu entry was 16-05's pending no-op; summary fidelity under a real model is the research validation table's operator-gated leg — offline tests prove the machinery with a scripted summarizer, not live summary fidelity.

(Human-item lineage: the 2026-09-06 report's Item 1 — the architect decision on producing-turn semantics — was closed by operator ruling (b), implemented by 19-06; its CR-01 residue was an engineering gap, now closed by 19-07 and verified above. This G-19-2 retest is the sole remaining human leg.)

### Gaps Summary

No code gaps remain. The single blocking gap of the prior report (CR-01: recovery worked exactly once per session because the pre-user marker scan preceded the armed carve-out) is closed in code at projector.go:170-178, is pinned by regression legs at both the projector and session level, and this verifier independently reproduced the RED proof on the pre-fix commit (exactly the two new legs fail there) before confirming GREEN on HEAD. Tamper safety, the degraded fail-through, the retry budget, and every pre-existing pin survive (1103 insertions / 0 deletions vs 06d92d1; probe baseline untouched). The phase's status is human_needed solely because the G-19-2 operator live retest (SC-1c live coherence on the fresh binary) cannot be executed offline. Carried review debt (CR-02, WR-01..06, IN-02..08) and the stale battery doc comment remain documented above and in deferred-items.md; none blocks a phase must-have.

---

_Verified: 2026-09-07T18:14:00Z_
_Verifier: ZCode (gsd-verifier)_
