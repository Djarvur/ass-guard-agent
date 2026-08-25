---
phase: 14-adoption-readiness-analysis-dispositions
verified: 2026-08-25T00:00:00Z
status: passed
score: 39/39 must-haves verified
behavior_unverified: 0 # the live leg resolved 2026-08-19T22:41Z (see Human verification — resolved)
overrides_applied: 0
human_verification:
  - test: "Run the gated live rollback E2E (or explicitly accept the offline-evidence disposition already recorded in WINDOWS.md #4)"
    expected: "TestCheckpointLiveRollback_Gated PASSES with recorded evidence (transcript + ref list), closing WINDOWS.md entry #4 and the phase gate's 'live rollback demonstration' clause"
    why_human: "Needs ZAI_API_KEY + ASSGUARD_CHECKPOINT_E2E=1 (operator credentials). Skip-loud behavior verified this run: ungated execution SKIPs naming both env gates; TestSessionFor_WiresCheckpointer PASSes"
  - test: "Sign off the pi↔shaper audit row classifications (docs/shaper-pi-audit.md, 29 rows)"
    expected: "Each row's classification (match / divergence-justified / divergence-routed / absent-at-pin) is judged correct against its dual file:line citations; zero-fix outcome accepted as legitimate"
    why_human: "Classification correctness is analytical judgment over cited evidence — grep proves the vocabulary and citations exist, not that each disposition is right (14-03's own summary flags this human_judgment: true)"
  - test: "Sign off the compaction-decision routing (docs/compaction-decision.md §3)"
    expected: "cache_control system-block emission → post-adoption queue and cross-turn window span → 12-05 confirmed as the intended destinations before 12-05 consumes them"
    why_human: "Routing is an operator judgment call; the census itself is mechanically reproducible, the routing is not (14-02's own summary flags D2 human_judgment: true)"
  - test: "Decide on the MVP goal-format discrepancy: `user-story.validate` rejects the phase goal on the literal article ('As the operator…' vs the canonical 'As a …')"
    expected: "Either accept the current wording (semantic slots role/capability/outcome are all present — this verification proceeded on that basis) or normalize via /gsd mvp-phase 14"
    why_human: "Format-guard ruling is a tooling-policy decision; the validator is strict while the goal is a user story in substance"
deferred:
  - truth: "Truncation marker's captured form is corpus-absent"
    addressed_in: "Phase 12-05"
    evidence: "truncate.go doc comment routes the marker shape to the 12-05 re-record; ROADMAP pre-adoption ordering (12-05 after Phase 14)"
  - truth: "Read-tracking is_error forms (165 live observations) not implemented"
    addressed_in: "Phase 12-05"
    evidence: "docs/tool-contract-inventory.md routes the class per the standing 08-08 operator disposition; 12-05's re-record carries the forms"
  - truth: "Cross-turn window-span divergence not re-observable (corpus single-turn)"
    addressed_in: "Phase 12-05"
    evidence: "docs/compaction-decision.md disposition #2: re-check rides the 12-05 long multi-turn capture"
  - truth: "Full nightly parity gate automation"
    addressed_in: "Phase 12-08"
    evidence: "ROADMAP Phase 14 added-at note: 'the full nightly CI automation lands with 12-08's EVAL gate (post-adoption)'"
  - truth: "cache_control system-block emission (profile TextBlock format change, TIER-1)"
    addressed_in: "post-adoption queue (12-03/12-08/12-07 era)"
    evidence: "14-02 disposition #1 + 14-03 CC-1; standing `cache probe: FAIL` verdict observed live this verification flips green when the fix lands"
  - truth: "Cross-provider light-tier routing (second provider instance through the subagent runner)"
    addressed_in: "post-adoption queue"
    evidence: "acp_serve.go resolveSubagentModel degrade comment + 14-05 summary routed residue"
  - truth: "Engine gating on isDestructive (chaining safety model unchanged)"
    addressed_in: "operator decision (v1.2 flags knob noted)"
    evidence: "docs/tool-contract-inventory.md flags map + 14-06 summary key-decisions"
---

# Phase 14: Adoption Readiness (Analysis Dispositions) Verification Report

**Phase Goal:** As the operator about to daily-drive ass-guard from an ACP editor, I want the one unrecoverable-risk backstop (workspace undo) and the two cheap evidence checks (compaction behavior, cache discipline) that the cross-agent analyses flagged, so that early adoption starts on a controllable, cost-predictable agent — not a leap of faith.
**Verified:** 2026-08-19T21:21:36Z (human items resolved 2026-08-19T22:50Z — see the resolution addendum at the end)
**Status:** pass (39/39) — human_needed items resolved by the operator; see "Human verification — resolved"
**Re-verification:** No — initial verification

## MVP Mode Note

ROADMAP marks this phase `Mode: mvp`. The literal user-story validator (`user-story.validate`) rejects the goal on the article ("As the operator…" vs canonical "As a …") — a format technicality, not a structural failure: the role/capability/outcome slots are all present and unambiguous. Verification proceeded on the semantic story; the discrepancy is routed to human decision (item 4 below), not treated as a goal failure.

## User Flow Coverage

User story: «As the operator about to daily-drive ass-guard from an ACP editor, I want the one unrecoverable-risk backstop (workspace undo) and the two cheap evidence checks (compaction behavior, cache discipline)… so that early adoption starts on a controllable, cost-predictable agent.»

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Bad turn happens | Every parent turn snapshots the workspace at turn entry BEFORE any turn work | internal/session/session.go:191-197 (Checkpointer.SnapshotTurn after nextTurnID, before hooks); TestPrompt_SnapshotsAtTurnEntry green | ✓ |
| Undo from terminal | `ass-guard checkpoint list` shows the turn, `checkpoint restore <id>` returns byte-identical pre-turn state — including non-latest checkpoints — with the user's `.git` untouched | cmd/ass-guard/checkpoint.go (0 os.Stdout refs); store.go:267 `checkout --no-overlay -f` (CR-01 fix); TestSnapshotRestore_ByteIdentical, TestRestoreNonLatest_RemovesLaterTurnFiles, TestSnapshot/TestRestore_UserGitUntouched all ran green | ✓ |
| Compaction question answered | A committed decision artifact answers from mechanical corpus evidence, no guesswork | docs/compaction-decision.md (182 lines): census, 3-way dispositions, 12 honesty rows, provenance table; scanner internal/profile/corpus_scan.go committed + tests green | ✓ |
| Cache discipline observable | Every parity run reports merge ordering + placement-vs-pin + target-version drift | Live offline run this verification: `zcode version check skipped: … pinned capture zcode 0.16.3` + `cache probe: FAIL — … pin-has-composed-lacks [system]` footer lines both observed | ✓ |
| Outcome: controllable, cost-predictable | Backstop + evidence checks + economics (light-tier, truncation) + uniform tool contract all in place before daily use | All six EARLY requirements satisfied (table below); post-review criticals fixed in code with RED→GREEN tests | ✓ (live E2E leg pending — Human Verification 1) |

## Goal Achievement

### Observable Truths

39 truths total across the six plans (14-01: 8, 14-02: 6, 14-03: 6, 14-04: 6, 14-05: 6, 14-06: 7).

| # | Truth (plan) | Status | Evidence |
|---|--------------|--------|----------|
| 1 | Byte-identical pre-turn restore, recursive comparison (14-01) | ✓ VERIFIED | TestSnapshotRestore_ByteIdentical + TestRestoreNonLatest_RemovesLaterTurnFiles ran green; CR-01 `--no-overlay` fix confirmed at store.go:259-267 |
| 2 | User repo git state never touched (14-01) | ✓ VERIFIED | TestSnapshot_UserGitUntouched + TestRestore_UserGitUntouched ran green (HEAD/index/status/.git-tree recursive) |
| 3 | Snapshot failure loud, never turn-fatal (14-01) | ✓ VERIFIED | TestPrompt_CheckpointFailureNonFatal green; seam call site degrades via slog, turn proceeds |
| 4 | checkpoint list ordered, stable, empty-store exits 0 (14-01) | ✓ VERIFIED | TestCheckpointListOrdering green in full package run (19.7s, -count=1) |
| 5 | Zero-change turn still checkpoints (14-01) | ✓ VERIFIED | TestSnapshot_NoChangesStillCommits green (`--allow-empty` in Snapshot path) |
| 6 | Re-snapshot same turn updates SAME ref (14-01) | ✓ VERIFIED | TestSnapshot_IdempotentSameTurn green |
| 7 | Concurrent ops serialize; no partial ref (14-01) | ✓ VERIFIED | TestConcurrentSnapshotRestoreSerialize + TestSnapshot_InterruptedNeverExposesPartialRef green; commit-then-update-ref ordering |
| 8 | BACKSTOP: gated live demo leg — user .git untouched across a real model turn (14-01) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Unrun by design-disposition: ZAI_API_KEY absent; test SKIPs loud naming both gates (observed this run); WINDOWS.md #4 open with exact operator command; offline guarantee proven by truths 1-2 → Human Verification 1 |
| 9 | Mechanical corpus scan committed, not a one-off (14-02) | ✓ VERIFIED | corpus_scan.go (315 lines): ScanContextBehavior, CacheControlPlacements + exposed other bucket, CompactionMarkers, WindowShape, ParseErrors/ScannedLines; 4 tests green |
| 10 | Decision artifact with 3-way disposition (14-02) | ✓ VERIFIED | docs/compaction-decision.md: delivered-by-mimicry / gap-routed / absent-in-target classes present |
| 11 | Provenance recorded incl. pinned-session-rotated statement (14-02) | ✓ VERIFIED | Provenance §1: file names, bytes, mtimes, zcode version (3.8.1 header + 0.16.3 manifest caveat), scan date, ladder step 2 named |
| 12 | Census complete, not sampled (14-02) | ✓ VERIFIED | 12 "not observed in analyzed corpus" rows; completeness guarantee stated in §2 |
| 13 | Implementation ONLY if evidence demands — zero in-phase (14-02) | ✓ VERIFIED | Commits d981add/7931d30/a0789e1 touch only internal/profile + docs (git show --stat) |
| 14 | Fixtures pin observed forms w/ provenance headers (14-02) | ✓ VERIFIED | cache-control.jsonl + eviction-window.jsonl both carry `# provenance:` headers naming source sessions |
| 15 | Cross-validated against pi source at pinned commit (14-03) | ✓ VERIFIED | Pin 496185f… recorded with re-derivation command; tools/pi-audit/ gitignored (.gitignore:65) |
| 16 | Fixed-table diff, file:line citations both sides (14-03) | ✓ VERIFIED | docs/shaper-pi-audit.md (143 lines), 5 dimensions, dual citations |
| 17 | Every row exactly one locked classification (14-03) | ✓ VERIFIED | All five literals present (match/divergence-fixed/divergence-justified/divergence-routed/absent-at-pin); rows IDed CC-/TH-/HD-/CF-/TM- |
| 18 | Fixes minimal + capture-faithful (14-03) | ✓ VERIFIED | Zero-fix walk: commits 165ac43/a8224e9/805800f are docs + .gitignore only — shaper byte-identical |
| 19 | pi test suite consulted as spec oracle (14-03) | ✓ VERIFIED | 7 test.ts citations in the audit rows |
| 20 | Deterministic re-derivation at pin (14-03) | ✓ VERIFIED | Fetch+checkout-at-pin command recorded in the audit header |
| 21 | Probe asserts stable→volatile ordering (14-04) | ✓ VERIFIED | RunCacheProbe ordering checks; TestCacheProbe_OrderingViolationFails/ToolArraySpliceFails/GreenCase/EmptyMerges green |
| 22 | Placement asserted against corpus pin, bidirectional (14-04) | ✓ VERIFIED | AssertPlacementAgainstPin doc + implementation bidirectional; TestCacheProbe_PlacementAgainstPin green |
| 23 | Probe wired into parity run w/ footer line (14-04) | ✓ VERIFIED | Observed live: `cache probe: FAIL — …` footer; TestParityRun_CacheProbeWired/DefaultGap green |
| 24 | Version drift loudly warned, non-blocking (14-04) | ✓ VERIFIED | Skip line observed live; Mismatch/Match/Unresolvable trio green |
| 25 | Exit semantics unchanged (14-04) | ✓ VERIFIED | TestParityDriftWarning_Mismatch asserts summary-equality (non-blocking lock test-pinned) |
| 26 | Version resolver seam-injected (14-04) | ✓ VERIFIED | zcodeInstalledVersion func-var seam (parity.go:30, fixed argv + timeout) |
| 27 | Light-tier routing, explicit both-ways (14-05) | ✓ VERIFIED | TestSessionFor_ResolvesLightTier green (binding → slug; no binding → parent model) |
| 28 | Existing config surface only (14-05) | ✓ VERIFIED | scheduler.NewResolver(cfg).Resolve(tierLight, …) — the tiers table; no new schema |
| 29 | Cross-provider binding degrades loudly, routed (14-05) | ✓ VERIFIED | TestSessionFor_LightTierCrossProviderWarns green; warning at acp_serve.go names both providers + post-adoption routing |
| 30 | 128 KiB tail-truncation; under-cap byte-unmodified (14-05) | ✓ VERIFIED | DefaultToolResultCapBytes=131072; TestTruncateToolResult_Cap/UnderCapUnmodified/Edges green |
| 31 | Marker corpus-absent flagged + routed to 12-05 (14-05) | ✓ VERIFIED | truncate.go CORPUS-ABSENT doc comment routes the captured form to the 12-05 re-record |
| 32 | Single chokepoint, both append paths (14-05) | ✓ VERIFIED | boundedToolResult at session.go:446 (subagent) + :501 (parent loop); CR-02 json.Marshal fix makes multi-line results boundable — TestSubagentMultiline* green |
| 33 | Per-tool timeouts catalog-wide; siblings isolated; inner deadlines win (14-06) | ✓ VERIFIED | 19 timeout_ms annotations; executeBounded per-call wrap; TestDispatchBatch_PerToolTimeout_Bounds/DefaultBackstop green |
| 34 | is_error corpus-grounded; Bash anchor (14-06) | ✓ VERIFIED | TestIsErrorCorpusFixture + TestIsErrorCorpusForm_BashExitCode green; inventory quotes the scan |
| 35 | Retry-only-transient pinned; census of every retry site (14-06) | ✓ VERIFIED | TestRetryOnlyTransient_ClassificationTable + TestRetrySites_NeverRetryStructural green; inventory §c lists 4 sites w/ file:line |
| 36 | Flags declared/parsed/consumed; engine gating routed w/ rationale (14-06) | ✓ VERIFIED | 19 destructive + 16 concurrency_safe annotations; isAloneInSlot consumes IsConcurrencySafe; TestDispatchBatch_UsesConcurrencySafeFlag green; routing in inventory |
| 37 | occ census INPUT ONLY; every name accounted (14-06) | ✓ VERIFIED | TestOccCensusCrossCheck green (13 in-catalog + 0 MCP + 12 documented-absent = 25); fixture carries provenance |
| 38 | Census deterministic + idempotent (14-06) | ✓ VERIFIED | TestOccCensusDeterministic green |
| 39 | BACKSTOP: routed engine-gating residual recoverable via checkpoint restore (14-06) | ✓ VERIFIED | Both links test-proven: turn-entry snapshot seam (TestPrompt_SnapshotsAtTurnEntry) + restore-to-pre-turn incl. non-latest (TestRestoreNonLatest_RemovesLaterTurnFiles); routing documented in inventory |

**Score:** 38/39 truths verified (1 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| internal/checkpoint/store.go (563 lines) | Store: Open/Snapshot/List/Restore, DefaultKeep=50, isolation env, lock, prune | ✓ VERIFIED | All symbols present; GIT_CONFIG_GLOBAL isolation; refs/checkpoints/ namespace; CR-01 fix in place |
| cmd/ass-guard/checkpoint.go (146 lines) | list/restore CLI, stderr-only | ✓ VERIFIED | 0 os.Stdout references (grep) |
| internal/session/checkpoint_seam_test.go | Seam proof tests | ✓ VERIFIED | TestPrompt_SnapshotsAtTurnEntry + TestPrompt_CheckpointFailureNonFatal, ran green |
| cmd/ass-guard/testdata/checkpoint-e2e/README.md | Gated live runbook | ✓ VERIFIED | Env gates + exact operator command documented |
| internal/profile/corpus_scan.go (315 lines) | ScanContextBehavior + report | ✓ VERIFIED | ≥120 min_lines met; exposed other bucket |
| docs/compaction-decision.md (182 lines) | EARLY-02 decision artifact | ✓ VERIFIED | ≥60 min_lines met; 4 sections; honesty rows |
| docs/shaper-pi-audit.md (143 lines) | EARLY-04 audit | ✓ VERIFIED | ≥80 min_lines met; pin; 5 dimensions |
| internal/parity/cacheprobe.go (351 lines) | RunCacheProbe + AssertPlacementAgainstPin | ✓ VERIFIED | ≥100 min_lines met; corpus-wins rule in doc comment |
| cmd/ass-guard/parity.go wiring | drift seam + probe + footer | ✓ VERIFIED | zcodeInstalledVersion, assembleCacheProbe, --cache-pin, footer line (observed live) |
| internal/session/truncate.go + truncate_test.go | Cap + marker + 9 tests | ✓ VERIFIED | All present; CR-02 regression tests included |
| internal/toolcat annotations | 19 tools × 3 annotations | ✓ VERIFIED | 19 timeout_ms / 16 concurrency_safe / 19 destructive in coretools.json; EffectiveTimeoutMS/IsConcurrencySafe/IsDestructive accessors |
| internal/toolexec/batch.go | executeBounded per-call wrap | ✓ VERIFIED | isAloneInSlot pool membership consumes the flags |
| internal/toolexec/testdata/occ-census.json | Pinned census fixture | ✓ VERIFIED | Provenance block (repo, commit 5d007f09, extracted_at) |
| docs/tool-contract-inventory.md (164 lines) | EARLY-06 evidence base | ✓ VERIFIED | Existing-partials census, is_error table, retry census, cross-check accounting |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| session.Prompt (post-nextTurnID) | checkpoint Store.Snapshot | Checkpointer seam | ✓ WIRED | session.go:191-197 calls SnapshotTurn before hooks/user message |
| acp_serve.go sessionFor | checkpoint Store | serve wiring, default ON | ✓ WIRED | checkpoint.Open + checkpointerAdapter (nil-guarded); TestSessionFor_WiresCheckpointer green |
| checkpoint restore CLI | Store.Restore | git checkout + clean, refs/checkpoints/ | ✓ WIRED | store.go:267 (--no-overlay); id validated before any exec |
| corpus_scan.go | compaction-decision.md census | report → table | ✓ WIRED | Doc quotes scanner output; in-module reproduction driver documented |
| compaction-decision cache facts | 14-04 probe assertions | CacheControlPlacements → pin | ✓ WIRED | PinClasses derives from the committed fixture at runtime (live run cites it) |
| parity.go runParity | RunCacheProbe + AssertPlacementAgainstPin | footer line | ✓ WIRED | Observed live; WR-03 (results-file omits probe) is a partial evidence gap — warning |
| sessionFor | Session.SubagentModel | tierLight resolution | ✓ WIRED | resolveSubagentModel (acp_serve.go:1130) |
| AppendToolResult sites | truncation helper | boundedToolResult chokepoint | ✓ WIRED | Both call sites (session.go:446, :501) |
| toolcat annotations | DispatchBatch | accessors → deadlines/pool | ✓ WIRED | executeBounded + isAloneInSlot |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| parity cache probe | PinClasses | internal/profile/testdata/context-behavior/cache-control.jsonl (committed corpus fixture) | Yes | ✓ FLOWING |
| parity drift warning | installed version | zcodeInstalledVersion exec seam → real `zcode --version` (absent → skip note observed live) | Yes | ✓ FLOWING |
| compaction-decision census | counts | ScanContextBehavior over snapshot-frozen rollout corpus (names/sizes/mtimes in provenance) | Yes | ✓ FLOWING |
| checkpoint CLI | entries | Store.List over refs/checkpoints/ (real shadow git dir) | Yes | ✓ FLOWING |
| is_error inventory | counts | profile scan over live rollout dir (283 obs; live-scan test present) | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| CR-01 no-overlay restore removes later-turn files | go test ./internal/checkpoint/ -run 'TestRestoreNonLatest_RemovesLaterTurnFiles\|…UserGitUntouched…\|TestSnapshotRestore_ByteIdentical' | ok 3.4s | ✓ PASS |
| Full checkpoint invariant battery | go test ./internal/checkpoint/ -count=1 | ok 19.7s | ✓ PASS |
| CR-02 multiline subagent results bounded + valid JSON | go test ./internal/session/ -run 'TestSubagentMultiline…\|TestJSONMarshalEscapeFree…' | ok 1.7s | ✓ PASS |
| Gated E2E skips loud ungated; wiring works | ASSGUARD_CHECKPOINT_E2E=0 go test ./cmd/ass-guard/ -run 'TestCheckpointLiveRollback_Gated\|TestSessionFor_WiresCheckpointer' -v | SKIP (names both gates) + PASS | ✓ PASS |
| Light-tier wiring both-ways + loud degrade | go test ./cmd/ass-guard/ -run 'TestSessionFor_ResolvesLightTier\|TestSessionFor_LightTierCrossProviderWarns' | ok 3.3s | ✓ PASS |
| Tool contract: timeouts/backstop/census/is_error | go test ./internal/toolexec/ -run 'TestDispatchBatch_PerToolTimeout_Bounds\|TestDispatchBatch_DefaultBackstop\|TestOccCensusCrossCheck\|TestIsErrorCorpusFixture' | ok 1.3s | ✓ PASS |
| Corpus scanner battery | go test ./internal/profile/ -run TestScanContextBehavior | ok 1.0s | ✓ PASS |
| Parity drift trio + probe wiring | go test ./cmd/ass-guard/ -run 'TestParityDriftWarning\|TestParityRun_CacheProbe' | ok 1.5s | ✓ PASS |
| Retry-only-transient both ends | go test ./internal/provider/ -run TestRetryOnlyTransient_ClassificationTable; go test ./internal/scheduler/ -run TestRetrySites_NeverRetryStructural | ok / ok | ✓ PASS |
| Session seam + subagent model override | go test ./internal/session/ -run 'TestPrompt_Snapshots…\|TestPrompt_Checkpoint…\|TestSubagentModel_…' | ok 1.1s | ✓ PASS |
| Offline parity run (footer + drift line) | go run ./cmd/ass-guard parity --results "" | skip line + `cache probe: FAIL — … pin-has-composed-lacks [system]` | ✓ PASS (standing FAIL is the routed gap, by design) |
| Zero-dep prohibition | git diff 670a167~1..HEAD -- go.mod go.sum | empty | ✓ PASS |

### Probe Execution

No phase-declared `scripts/*/tests/probe-*.sh` probes exist; this phase's probe equivalents are the Go gated/wired tests executed in Behavioral Spot-Checks above. Step 7c: conventional-probe discovery returns none — N/A.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| EARLY-01 | 14-01 | Shadow-git checkpoint backstop + terminal list/restore | ✓ SATISFIED | Store + seam + CLI + default-ON wiring; CR-01 fixed; full battery green. Live-E2E leg pending operator run (WINDOWS.md #4) |
| EARLY-02 | 14-02 | Compaction answered from evidence, decision artifact committed | ✓ SATISFIED | docs/compaction-decision.md; window delivered-by-mimicry documented; cache_control gap routed; zero implementation |
| EARLY-03 | 14-04 | Cache probe + drift warning in parity run | ✓ SATISFIED | Probe + warning wired; both observed in live offline run; non-blocking pinned |
| EARLY-04 | 14-03 | pi↔shaper cross-validation audit, every divergence dispositioned | ✓ SATISFIED | 29-row audit at pin 496185f; zero unclassified; zero-fix walk legitimate (docs-only commits verified) |
| EARLY-05 | 14-05 | Light-tier subagent routing + tool-output truncation | ✓ SATISFIED | Both mechanisms test-proven; CR-02 fixed (multiline bypass closed) |
| EARLY-06 | 14-06 | Uniform tool contract, corpus-grounded | ✓ SATISFIED | Timeouts catalog-wide, is_error corpus-grounded, retry pins, flags consumed, occ census input-only |

Orphaned requirements: none — REQUIREMENTS.md maps exactly EARLY-01..06 to Phase 14; each claimed by exactly one plan.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (all 25 phase production/test files scanned) | — | TBD/FIXME/XXX/TODO/placeholder | — | NONE FOUND (clean) |
| internal/checkpoint/store.go | 486-510 | WR-01 open: prune evicts by (sessionID, turn), doc promises commit-age ordering (multi-session edge only) | ⚠️ Warning | Old-session checkpoints can evict before new-session ones across sessions; recorded in committed 14-REVIEW.md, scoped out of the critical-only fix round |
| internal/checkpoint/store.go | 528-555 | WR-02 open: lock stealable from a live holder after 30s; release deletes successor's lock | ⚠️ Warning | Large-workspace snapshot >30s + concurrent restore can interleave; recorded in review |
| cmd/ass-guard/parity.go | 250-263 | WR-03 open: parity-results.json written before CacheProbe attached — evidence file omits probe verdict (stderr footer carries it) | ⚠️ Warning | Reproducibility record incomplete; confirmed still open this verification |
| internal/session/truncate.go | 61-81 | WR-07 open: structured (object) payloads bypass the cap — deliberate per plan, but the upstream-bound invariant is undocumented | ⚠️ Warning | Multi-MB structured stdout unbounded; recorded in review |
| internal/session/subagent.go | 146-191, 173-183 | WR-04/05 open: subagent pairing by tool NAME; nested loop re-sends identical prompt (pre-phase-14 defects) | ℹ️ Info | Pre-existing; phase touched the file but did not repair; recorded in review |

The standing `cache probe: FAIL` is NOT a stub — it is the probe faithfully reporting the routed cache_control emission gap (the designed verdict; flips green when the routed fix lands).

### Human Verification Required

### 1. Gated live rollback E2E (closes the phase gate's "live rollback demonstration")

**Test:** Run `ASSGUARD_CHECKPOINT_E2E=1 ZAI_API_KEY=<GLM key> go test ./cmd/ass-guard/... -run TestCheckpointLiveRollback_Gated -count=1 -v` — or explicitly accept the offline-evidence disposition recorded in WINDOWS.md #4.
**Expected:** Real provider turn mutates a scratch git repo → Store.Restore → workspace byte-identical, user repo HEAD/index/status untouched, evidence paths logged; WINDOWS.md #4 closes.
**Why human:** Requires operator credentials no verifier holds. Ungated behavior verified: SKIP names both env gates; the offline UserGitUntouched pair proves the invariant directly (ran green).

### 2. pi↔shaper audit disposition sign-off

**Test:** Eyeball the 29 row classifications in docs/shaper-pi-audit.md against their dual file:line citations.
**Expected:** Each match/justified/routed/absent-at-pin classification is judged correct; the zero-fix outcome accepted.
**Why human:** Classification correctness is analytical judgment; grep proves citations exist, not that dispositions are right (14-03 flagged this human_judgment itself).

### 3. Compaction-decision routing sign-off

**Test:** Eyeball docs/compaction-decision.md §3 dispositions (cache_control emission → post-adoption; cross-turn span → 12-05).
**Expected:** Routing destinations confirmed before 12-05 consumes them.
**Why human:** Routing is an operator judgment; the census is mechanically reproducible, the routing is not (14-02 flagged this human_judgment itself).

### 4. MVP goal-format discrepancy

**Test:** Decide whether to normalize the phase goal's wording ("As the operator…" fails the strict "As a …" validator) via /gsd mvp-phase 14, or accept it.
**Expected:** Either a canonical-format goal or an explicit acceptance.
**Why human:** Tooling-policy decision; the semantic story slots are all present (this verification proceeded on that basis).

### Gaps Summary

No gaps found. All 39 must-have truths are either VERIFIED (38) or present-but-behavior-unverified (1 — the gated live demo leg, dispositioned skip-loud per the plan's own design). All artifacts exist, are substantive, wired, and data-flowing. Both post-review critical defects (CR-01 overlay-restore, CR-02 JSON-encoding bypass) are fixed in code — fixes independently re-verified this run by executing their RED→GREEN tests, not by trusting the fix report. Zero-dep prohibition holds (go.mod/go.sum byte-identical across the whole phase window). All four known residuals are properly recorded (WINDOWS.md #4/#5, truncate.go corpus-absent comment, resolveSubagentModel degrade comment), none silently missing. The seven review warnings (WR-01..07) remain open by the critical-only fix-round scope decision — recorded in the committed 14-REVIEW.md, surfaced above as warnings.

The phase goal — a controllable, cost-predictable agent, not a leap of faith — is achieved on offline evidence; the single outstanding item is the operator-executed live rollback demonstration (or explicit acceptance of the offline disposition), which is why this verification routes to human_needed rather than passed.

---

_Verified: 2026-08-19T21:21:36Z_
_Verifier: ZCode (gsd-verifier)_

## Human verification — resolved (2026-08-19T22:50Z, operator session)

All four human items resolved; the report's `human_verification` list above is retained
verbatim as the record of what was asked. Resolutions (detail in 14-UAT.md):

1. **Gated live rollback E2E — RUN, PASS** (15.54s, exit 0). Real GLM turn mutated the
   scratch repo; restore byte-identical; user `.git` HEAD/index/porcelain unchanged.
   Evidence: `restoredRef=refs/checkpoints/sess-ckpt-live-turn-001`,
   `transcript_sess-ckpt-live.jsonl` (entries=1), log `/tmp/ckpt-live-e2e.log`.
   WINDOWS.md #4 → fixed. The 39th must-have is now behavior-verified.
   (Environment note: the gated test stat-gates a project-layer `.ass-guard/config.yaml`;
   an empty deep-merge-neutral marker was created — credentials still resolve from the
   global layer + `$ZAI_API_KEY`, precedence unchanged.)
2. **pi↔shaper audit** — ACCEPTED: classifications correct, zero-fix outcome legitimate.
3. **Compaction-decision routing** — ACCEPTED: cache_control emission → post-adoption
   queue (TIER-1); cross-turn span → 12-05; already-delivered dispositions stand.
4. **MVP goal format** — ACCEPTED as-is (semantic slots present; validator over-literal).

**Status:** pass (39/39) — Phase 14 closed 2026-08-19.
