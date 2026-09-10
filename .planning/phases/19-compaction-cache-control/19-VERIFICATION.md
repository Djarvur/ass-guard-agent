---
phase: 19-compaction-cache-control
verified: 2026-09-10T10:47:05Z
status: passed
score: 14/14 must-haves verified
covered_files:
  - .planning/REQUIREMENTS.md
  - internal/modelrouting/defaults/config.yaml
  - internal/session/compaction.go
  - internal/session/compaction_test.go
  - internal/session/projector.go
  - internal/session/projector_test.go
  - internal/session/session.go
  - internal/session/steering_test.go
  - internal/session/steerqueue.go
  - internal/session/transcript.go
  - internal/shaper/shaper.go
  - profiles/zcode/profile.yaml
covered_digest: "v1:sha256:50bf6c55bf5db9793efad2948cc2551ca1aff34fa9948a178fc4525a31ddb336"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: "passed (2026-09-07T18:14Z report: 13/14, frontmatter status: passed with 1 behavior-unverified item — the G-19-2 live leg; resolved 2026-09-10 by the auto-UAT harness before this run)"
  previous_score: 13/14
  stale_reason: "Post-verification commits touched phase-19 surface files in internal/session: 830d045 (20 review CR-01 — LAST-reset-point clear-vs-marker ordering in Project()), b397954/3061d84 (23-01 SteerQueue + projector fold + fold-hardening tests), 40b2bbc/348d30f (23-02 steering ingress + parked-ask fallback), 9372ee9 (22-03 discriminated background dispatch). This run re-establishes every phase-19 truth against the CURRENT tree (HEAD = d38f913)."
  gaps_closed:
    - "SC-1c live-coherence leg (the sole prior behavior-unverified item): closed by the operator-approved auto-UAT harness (uat-harness-run-20260910, PASS 20/20) — threshold compactions fire live, overflow retry projects post-marker (rejected 515664B → retry 95605B carrying the FORCED marker's summary), restore-80 persists. The harness binary was built 2026-09-10 02:22 +03:00, AFTER every steering commit, and internal/session + internal/shaper + internal/provider have zero commits since that timestamp — the live evidence is valid for the current tree"
  gaps_remaining: []
  regressions: [] # none — see Goal Achievement; every phase-19 pin passed unmodified on the current tree
advisory:
  - finding: "Clear-vs-marker reset ordering (830d045's resetIdx > mIdx rule) has no direct test pin — no test anywhere combines a context-reset boundary with a compaction marker (TestClassBClear in internal/runtime/commands_test.go covers markerless /clear only)"
    category: other
    reason: "Phase-20-scope observation, not a phase-19 must-have: /clear postdates phase 19. Code-read this session confirms the semantics (reset strictly NEWER than the marker wins; marker-newer → projectCompacted governs, so clear-then-compact keeps the compaction's durable seed); a marker+reset ordering regression test would close it"
    evidence_status: "none provided"
---

# Phase 19: Compaction + cache_control Verification Report

**Phase Goal:** Long sessions stop silently losing earlier turns: threshold-triggered light-tier compaction appends an additive typed marker the Projector treats as a durable reset-point class, and cache_control ephemeral breakpoints ship on every system block — the corpus-proven parity-faithful lever (zcode has no auto-compact; docs/compaction-decision.md settles design).
**Verified:** 2026-09-10T10:47:05Z
**Status:** passed
**Re-verification:** Yes — STALE re-run (third report). Supersedes the 2026-09-07T18:14Z report after phase-20/22/23 commits (steering: SteerQueue + projector fold; /clear full-reset ordering) touched internal/session files inside this phase's surface.

## Re-Verification Audit Trail (why this run exists, what changed since 2026-09-07)

Commits touching the phase-19 surface after the prior verification, and their audited impact:

| Commit | Change | Impact on phase-19 truths |
|--------|--------|--------------------------|
| 830d045 (20 review CR-01) | `Project()` gained a full-reset branch: `fullResetBoundaryIdx` + `projectFullReset`, LAST-reset-point-wins vs the marker | NONE on the marker path by construction: the branch fires only when `resetIdx >= 0 && (mIdx < 0 \|\| resetIdx > mIdx)` — a marker NEWER than the reset still routes to `projectCompacted` (clear-then-compact keeps the durable seed). Ordinary mutating boundaries (the phase-19 durability scope) never carry the `context-reset` cause, so the D-06 durability guarantee is untouched. Unmodified `TestProjector_CompactionResetPoint`/`TailCut` PASS on the current tree. See Advisory for the unpinned-ordering note |
| b397954 (23-01 steering) | `foldExchanges` gained a `TypeSteeringDelivery` case; `steerqueue.go` (new); `manager.go`/`transcript.go`/`session.go` steering seams | Pair atomicity preserved: the steering case calls `flushBatch()` BEFORE appending the user-role message (a user message can never land inside a tool_use batch). Anchor safety preserved: `turnAnchorOf` matches `TypeUserMessage` ONLY — steering never moves the accumulation anchor. Anti-spoofing: `AppendSteeringDelivery` is the sole writer of `steering_delivery` lines (agent-appended only; nothing parses model/tool output into steering). Drain sits at runTurn's iteration top BEFORE `maybeCompact`/`Project` (session.go:631-642), so steering lands in arrival position relative to the marker and never displaces the summary seed. `TestSteeringProjectPairSafety`/`AnchorSafety`/`IntentSummaryUnchanged`/`ReplayParity` all PASS this session |
| 3061d84 (23-01 tests) | +260 lines steering fold-hardening tests | Additive only; all PASS this session |
| 40b2bbc / 348d30f (23-02) | Pre-mutex steering ingress; parked-note moved to the ask-queue parked branch (session.go -26/+26) | Runtime ingress layer; the overflow-retry arming branch (session.go:664-703) byte-identical in shape — arming still at :700 between the forced compact and the `continue` |
| 9372ee9 (22-03) | Discriminated background dispatch (subagent.go) | Subagent surface; the `subagent_turn_safe` carve-out pin PASSes unmodified |

**Frozen-surface proof (content, not just presence):**

- `internal/session/compaction.go`: ZERO diff since 35d1740 (the 19-07 CR-01 fix commit) — the compaction engine is untouched by everything after phase 19.
- `internal/session/projector_test.go` + `compaction_test.go`: ZERO content diff since 35d1740 (absent from `git diff --numstat 35d1740..HEAD -- internal/session/`) — **every phase-19 pinned fixture survives unmodified**, so the green battery below is a genuine regression result, not re-authored expectations.
- `projectSameTurnCompacted` / `sameTurnMarkerIdx` / `SetRetryCompactedTurn`: zero diff since 35d1740.
- `internal/shaper/`, `internal/profile/`, `internal/paritycli/`, `internal/provider/errors.go`, `internal/parity/cacheprobe.go`: ZERO diff since 41a5c63 (phase-19 completion) — the entire PAR-02 substrate is frozen.
- Working-tree note: 471 files under `internal/` show as modified — ALL are permission-bit flips (0644→0755), `git diff --stat` = 0 insertions/0 deletions. Working-tree content equals HEAD (d38f913); all verdicts are against the committed state.
- Live-evidence validity: the auto-UAT harness ran 2026-09-10T02:22:20+03:00 on a HEAD build; every steering commit predates it (2026-09-08), and `git log --since` shows ZERO commits to `internal/session`/`internal/shaper`/`internal/provider` after the harness timestamp — the harness's live PASS is evidence against the current tree's content.

## Goal Achievement

Machine-verified against the CURRENT tree (HEAD d38f913) by commands this verifier executed itself: the 13-function phase-19 session battery under `-race -count=1` — ok 3.632s with the CR-01/tamper/fallback/anchor subtests individually `--- PASS` in a `-v` re-run; the steering fold battery ok; `TestTranscriptNewKinds` ok; acpserve compaction surface 3/3 PASS; shaper/profile/paritycli/provider/modelrouting all ok; `go vet ./internal/session/` clean; the frozen-surface diffs above re-run this session.

### Observable Truths

| # | Truth | Status | Evidence (current tree, this session) |
|---|-------|--------|----------|
| 1 | SC-1a: over-threshold session compacts automatically (blocking compact at loop head, summary-bearing marker on disk, next turn seeded, headroom restored, disabled = zero delta) | ✓ VERIFIED | Battery green (`TestCompaction_Threshold`, `TestCompaction_Chaining`, `TestCompaction_SummarizerFailure`, `TestCompaction_LoopHead`, `TestCompactNow`, `TestCompaction_EndToEnd` all inside the ok 3.632s run); `compaction.go` zero-diff since 35d1740; `maybeCompact` still at runTurn head (session.go:640) |
| 2 | SC-1b: threshold ~80% default and configurable end-to-end (config keys, menu advertise + effective values, persist-then-apply, live apply, clamps, no context-limit key) | ✓ VERIFIED | `TestCompactionLive_BootDefaults`, `TestCompactionLive_MenuThresholdRoundTrip`, `TestCompactionOptions` 3/3 PASS this session; `config.yaml:45-46` carries `threshold_pct: 80` / `enabled: true`; harness persisted restore-80 to the project layer live |
| 3 | SC-1c: the user observes the conversation continuing coherently where v1.1 lost earlier turns (live threshold fire + summary-coherent continuation) | ✓ VERIFIED (upgraded from PRESENT_BEHAVIOR_UNVERIFIED) | Auto-UAT harness 2026-09-10 (operator-approved automated stand-in for the live leg, G-19-2 resolution in 19-UAT.md): PASS 20/20 — threshold-50 live-set fires first + second compactions (markers on disk, stderr notes, summary-seeded continuations across turns 3-5), wire order exact, seeding follows the pinned position rule (req3=[1] req4=[2] retry=[4]), 4 marker lines with non-empty durable summaries. Directly observed behavior on a binary whose session/shaper/provider content equals the current tree |
| 4 | SC-2: tool_use/result pairs atomic, thinking never rewritten mid-chain, pinned Projector tests prove boundary-survival and pair-atomicity | ✓ VERIFIED | `TestProjector_CompactionResetPoint` + `TestProjector_CompactionTailCut` PASS inside this session's battery — UNMODIFIED (zero test diff since 35d1740); steering fold preserves pair safety (`flushBatch()` before the steering user message; `TestSteeringProjectPairSafety` PASS) |
| 5 | SC-3 / G-19-1 + CR-01 closure: on a provider overflow error the producing turn completes after compaction + the single retry — the retry projects post-marker (carries the NEW summary), is measurably smaller, on EVERY overflow including sessions with an earlier marker | ✓ VERIFIED | Code: the armed carve-out is STILL FIRST in `Project()` (projector.go:170-174, read this session) — ahead of both the phase-20 full-reset branch (:185-187) and the pre-user marker scan (:189-191). Tests: `-v` run this session shows `armed_override_wins_over_the_pre-user_marker_scan_(CR-01)` and `prior_marker:…_(CR-01)` `--- PASS` plus the tamper byte-identity and fallback pins. Live: harness leg "retry window shrank (CR-01 post-marker projection) — rejected=515664B retry=95605B", turn completes end_turn (requests.jsonl seq 5-7) |
| 6 | 19-06 T2 + 19-07: the carve-out is ENGINE-GATED — transcript content alone never reshapes a projection (kill-9 replay + tamper safety) | ✓ VERIFIED | Arming is still the in-memory, engine-set-only `SetRetryCompactedTurn(turnID)` at session.go:700 inside the `provider.IsOverflow && !overflowRetried` branch; `not_armed:_marker-present_projects_byte-identically_to_marker-free` PASS this session; steering cannot spoof the anchor (`turnAnchorOf` matches TypeUserMessage only) or inject steering lines from model output (AppendSteeringDelivery sole writer) |
| 7 | 19-03 position rule survives; every 19-03/19-04 fixture passes UNMODIFIED | ✓ VERIFIED | `git diff --numstat 35d1740..HEAD -- internal/session/` lists NEITHER projector_test.go NOR compaction_test.go — zero content change since the 19-07-verified state; the full battery (which includes every 19-03/19-04 pin) is green on the current tree |
| 8 | WR-03a: summarizer input bounded — span capped by a budget derived from ContextLimit (fallback cap when unset) | ✓ VERIFIED | `TestCompaction_BoundedSpanAndReFireGuard` PASS in this session's battery; `compaction.go` (spanBudgetChars/boundSpanMessages) zero-diff |
| 9 | WR-03b: at most ONE threshold-class compaction attempt per turn; forced compact stamps the guard; a new turn re-attempts | ✓ VERIFIED | Guard stamp intact (session.go:698 `s.compactionAttemptTurn = turnID`); battery green; harness live leg "turn-4 compaction: one threshold (drift) + one forced" consistent with the once-per-class rule |
| 10 | 19-06 T6: Reconcile classifies a mid-turn same-turn marker as inert bookkeeping; Project is a pure function of (lines, turnID, override) | ✓ VERIFIED | `TestReconcileSameTurnMarker` PASS in this session's battery; `TestSteeringReplayParity` PASS (steering replays identically — the purity discipline extended to the new line kind) |
| 11 | SC-4a: every outgoing request carries cache_control {"type":"ephemeral"} on each system block (keep-last-4 at the API cap), visible in the marshaled body | ✓ VERIFIED | shaper.go ZERO content diff since 41a5c63; emission still confined to the systemBlocks loop (shaper.go:167-180, `NewCacheControlEphemeralParam` only); `profiles/zcode/profile.yaml:5` `system_cache_control: true`; shaper package ok this session |
| 12 | SC-4b: the parity cache-discipline probe flips green on placement against the committed pin fixture; baseline untouched | ✓ VERIFIED | paritycli package ok this session (TestCacheProbe_PlacementFlip at parity_test.go:417); `internal/parity/cacheprobe.go` zero diff since 41a5c63 |
| 13 | PAR-01 substrate (19-02): non-2xx rejections surface as ClassifyHTTP-typed error chunks (never a defaulted end_turn done chunk); IsOverflow message-class matcher, no new ErrorKind; happy SSE path unchanged | ✓ VERIFIED | provider package ok this session; `internal/provider/errors.go` zero diff since 41a5c63. CR-02 (in-band SSE error events swallowed) remains PRE-EXISTING review debt outside this truth's non-2xx scope (carried below) |
| 14 | 19-03: summary payload additive on the marker line (redacted path, round-trip, field-tolerant); no-marker transcripts project byte-identically | ✓ VERIFIED | `TestTranscriptNewKinds` PASS under `-race` this session; the 23-01 steering additions to transcript.go are a NEW additive line kind (`steering_delivery`, redacted path, sole-writer discipline) — they did not alter the marker payload or the no-marker identity pins (which PASS inside the battery) |

**Score:** 14/14 truths verified (0 present-but-behavior-unverified)

### Prohibition Checks (re-checked on the current tree)

| Prohibition | Verdict | Evidence |
|-------------|---------|----------|
| Tamper safety: projector never accepts a same-turn marker from transcript content alone | VERIFIED | Engine gate intact (session.go:700 arming; non-empty exact turnID); not-armed byte-identity DeepEqual pin PASS this session |
| Test integrity: phase-19 pinned fixtures not edited by the later phases | VERIFIED | Zero content diff on projector_test.go/compaction_test.go since 35d1740 (numstat re-run this session) |
| Retry budget: retry stays EXACTLY ONCE | VERIFIED | Prior-marker leg (asserting exactly 3 stream calls) PASS this session; harness wire-order leg exact |
| cache_control never on tools/message blocks | VERIFIED | shaper.go frozen since 41a5c63; emission confined to systemBlocks loop |
| Probe baseline never edited | VERIFIED | `internal/parity/cacheprobe.go` zero diff since 41a5c63 |
| No field beyond type ephemeral | VERIFIED | `NewCacheControlEphemeralParam` only; shaper golden pins green |
| No new ErrorKind; no generic-400 overflow misclassification | VERIFIED | provider package green; errors.go frozen |
| Error-envelope parsing never panics | VERIFIED | malformed-body subtest green (provider ok) |
| Transcript never mutated by compaction | VERIFIED | compaction.go zero-diff; append-only paths; replay-identity pins green |
| Durable seed never displaced; tail never orphaned | VERIFIED | ResetPoint/TailCut pins PASS unmodified; steering fold never displaces the seed (flushBatch + arrival position; `TestSteeringProjectIntentSummaryUnchanged` PASS) |
| Bus isolation; estimate never request bytes | VERIFIED | Battery green (zero-bus-publishes + estimate subtests) |
| No context-limit key; out-of-range never persisted | VERIFIED | `TestCompactionOptions` PASS this session |
| Debt markers in phase surface files | VERIFIED (none) | TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER grep over projector.go/compaction.go/session.go/steerqueue.go/shaper.go — zero hits |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/session/projector.go` | armed carve-out precedence + compaction reset-point class + fold rules | ✓ VERIFIED | :170-174 carve-out first (read this session); `projectCompacted`/`projectSameTurnCompacted`/`sameTurnMarkerIdx` zero-diff since 35d1740; steering fold + full-reset branch are additive neighbors that route around the marker path (see Audit Trail) |
| `internal/session/compaction.go` | threshold engine, bounded span, re-fire guard | ✓ VERIFIED | ZERO diff since 35d1740 |
| `internal/session/session.go` | loop-head compact + overflow retry-once + arming | ✓ VERIFIED | :631 drain → :640 maybeCompact → :642 Project; :664-703 overflow branch with :700 arming — shape identical to the 19-07-verified state |
| `internal/session/projector_test.go` / `compaction_test.go` | every phase-19 pin | ✓ VERIFIED | Unmodified since 35d1740; full battery green on current tree |
| `internal/session/steerqueue.go` (new, phase 23) | must not regress the phase-19 surface | ✓ VERIFIED | Stdlib-only queue; nil-safe; delivery via the projector's pair-safe fold; steering battery green |
| `internal/shaper/shaper.go` + `profiles/zcode/profile.yaml` | PAR-02 cache_control emission | ✓ VERIFIED | Both zero-diff since 41a5c63; shaper package ok |
| `internal/modelrouting/defaults/config.yaml` | threshold_pct 80 / enabled | ✓ VERIFIED | :45-46 present |

### Key Link Verification

| From | To | Via | Status |
|------|----|----|--------|
| session.go overflow-retry branch | projector.go SetRetryCompactedTurn | arming between forced compact and continue (:700) | ✓ WIRED (re-read this session; unchanged) |
| Project() armed branch | projectSameTurnCompacted | `retryCompactedTurn == turnID` + `sameTurnMarkerIdx >= 0`, FIRST in Project() | ✓ WIRED — precedence holds ahead of both later-added branches |
| Armed-with-no-marker fallback | projectCompacted via compactionMarkerIdx | falls through to the pre-user scan | ✓ WIRED (fallback pin PASS) |
| runTurn iteration top | maybeCompact → Project | drain(23-01) → compact(19-04) → project ordering | ✓ WIRED — steering lands before Project, never displaces the marker/seed |
| SteerQueue.Drain | AppendSteeringDelivery → projector fold | sole-writer transcript line, pair-safe fold | ✓ WIRED (steering battery green) |
| compaction.go span budget | compactionSettings.ContextLimit | spanBudgetChars → boundSpanMessages | ✓ WIRED (zero-diff; battery green) |
| profiles/zcode/profile.yaml | shaper.go | system_cache_control → NewCacheControlEphemeralParam | ✓ WIRED (frozen; package green) |
| provider IsOverflow | session.go retry branch | intercept before appendError | ✓ WIRED (frozen; package green) |

### Behavioral Spot-Checks

All commands executed by this verifier on the current tree (working-tree content == HEAD; the 471 modified files are permission-bit-only). Notation guard (the 2026-09-07 report's lesson): the `\|` inside the -run patterns below is MARKDOWN TABLE escaping only — each command was executed with plain `|` alternation in single quotes (Go regexp), and the quoted ok/PASS lines are the actual output of those runs.

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Phase-19 13-function battery | `go test -race ./internal/session/ -run 'TestProjector_SameTurnCarveOut\|TestCompaction_OverflowRetryCarriesSummary\|TestCompaction_BoundedSpanAndReFireGuard\|TestReconcileSameTurnMarker\|TestCompaction_OverflowRetryOnce\|TestProjector_CompactionResetPoint\|TestProjector_CompactionTailCut\|TestCompaction_EndToEnd\|TestCompactNow\|TestCompaction_Threshold\|TestCompaction_Chaining\|TestCompaction_SummarizerFailure\|TestCompaction_LoopHead' -count=1` | ok 3.632s | ✓ PASS |
| CR-01 battery, subtest level | same two-name filter, `-v` | both functions `--- PASS`; subtests include `armed_override_wins…(CR-01)`, `prior_marker…(CR-01)`, `not_armed:…byte-identically`, `armed_with_no_same-turn_marker:…governs_unchanged` | ✓ PASS |
| Steering fold battery (post-stale-change surface) | `go test -race ./internal/session/ -run 'TestSteeringDeliveryEndToEnd\|TestSteeringProjectAnchorSafety\|TestSteeringProjectPairSafety\|TestSteeringProjectIntentSummaryUnchanged\|TestSteeringReplayParity\|TestSteeringProjectLegacyTolerance\|TestSteeringAntiZombie' -count=1` | ok 1.131s | ✓ PASS |
| Transcript additivity | `go test -race ./internal/session/ -run 'TestTranscriptNewKinds' -count=1` | ok 1.070s | ✓ PASS |
| acpserve compaction surface | `go test ./internal/acpserve/ -run 'TestCompaction' -count=1 -v` | 3/3 PASS (BootDefaults, MenuThresholdRoundTrip, Options) | ✓ PASS |
| PAR-02 regression packages | `go test ./internal/shaper/ ./internal/profile/ ./internal/paritycli/ ./internal/provider/ ./internal/modelrouting/ -count=1` | all 5 ok | ✓ PASS |
| Vet | `go vet ./internal/session/` | clean | ✓ PASS |
| Frozen-surface audits | `git diff --numstat 35d1740..HEAD -- internal/session/` (tests absent); `git diff 41a5c63..HEAD --numstat` over shaper/profile/paritycli/errors/cacheprobe (empty) | confirmed | ✓ PASS |
| Live harness validity | `git log --since=2026-09-10T02:22:20+03:00 -- internal/session/ internal/shaper/ internal/provider/` | empty — harness binary's phase-19 surface == current tree | ✓ PASS |

### Probe Execution

Step 7c: SKIPPED — no probe scripts declared by this phase (unchanged from prior reports; the parity A/B probe is operator-gated live-model work by design).

### Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| PAR-01 | 19-02..19-07 | Compaction on context overflow risk: threshold-triggered configurable light-tier summarizer, additive typed marker, durable reset-point class, pair atomicity, thinking never rewritten, retry-once recovery | ✓ SATISFIED | Truths 1-10, 13, 14. On the CURRENT tree: engine frozen, projector precedence intact ahead of the steering/full-reset additions, every pin green unmodified, and the live harness proves the end-to-end behavior (threshold fire, coherent continuation, post-marker retry) on this content |
| PAR-02 | 19-01 | cache_control {"type":"ephemeral"} on every system block via the Shaper | ✓ SATISFIED | Truths 11, 12 — substrate frozen since 41a5c63, packages green this session |

No orphaned requirements: REQUIREMENTS.md maps exactly PAR-01 and PAR-02 to Phase 19 (rows `[x]` Complete; verified against the current tree by this report).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER in projector.go, compaction.go, session.go, steerqueue.go, shaper.go; no stdout writes (ACP frame discipline holds) | - | - |

Carried (unchanged from prior reports; none blocks a phase-19 must-have): CR-02 (in-band SSE error events swallowed — PRE-EXISTING, outside 19-02's non-2xx scope), WR-01/02/04/05/06, IN-02..08 (see 19-REVIEW.md), the stale battery-level doc comment in projector_test.go (~2177, deferred), and the compaction-observability UX deferral to the /status vehicle (19-UAT.md Deferred Follow-Ups).

### Advisory (New Scope, Unevidenced)

| # | Finding | Category | Why Advisory |
|---|---------|----------|--------------|
| 1 | The clear-vs-marker ordering rule added by phase-20's 830d045 (`resetIdx > mIdx` — LAST reset point wins) has no direct regression pin: no test combines a `context-reset` boundary with a compaction marker; `TestClassBClear` covers markerless /clear only | other | Phase-20 scope (/clear postdates phase 19); code-read this session confirms the intended semantics and every phase-19 pin passes — no deterministic evidence of any defect, so reported, not blocking. A marker+reset ordering test in the next plan touching internal/session would close it |

### Human Verification Required

None. The sole prior human leg (G-19-2 / SC-1c operator live-threshold retest) was resolved 2026-09-10 by the operator-approved automated live harness (19-UAT.md: both tests pass, gaps resolved; evidence uat-harness-run-20260910/). No behavior-unverified truths remain.

### Gaps Summary

No gaps. The stale-trigger commits (steering fold, full-reset ordering, parked-ask fallback, phase-22/23 dispatch work) are additive neighbors of the phase-19 machinery: the armed carve-out still takes precedence in `Project()`, the compaction engine and both projector compaction paths are byte-identical to the 19-07-verified state, every phase-19 pinned test passed UNMODIFIED on the current tree under `-race`, the PAR-02 substrate is frozen, and the 2026-09-10 live harness (whose binary's phase-19 surface is content-identical to this tree) proves the end-to-end goal behavior live — threshold compactions fire, continuation stays coherent and summary-seeded, the overflow retry projects post-marker and shrinks 515664B→95605B, restore-80 persists. Phase goal achieved and still holding.

---

_Verified: 2026-09-10T10:47:05Z_
_Verifier: ZCode (gsd-verifier) — stale re-verification after phase-20/22/23 internal/session changes_
