---
phase: 23-seed-gaps-close-out
verified: 2026-09-10T13:26:08Z
status: human_needed
score: 12/12 must-haves verified
covered_files:
  - .planning/REQUIREMENTS.md
  - .planning/phases/23-seed-gaps-close-out/23-01-PLAN.md
  - .planning/phases/23-seed-gaps-close-out/23-01-SUMMARY.md
  - .planning/phases/23-seed-gaps-close-out/23-02-PLAN.md
  - .planning/phases/23-seed-gaps-close-out/23-02-SUMMARY.md
  - .planning/phases/23-seed-gaps-close-out/23-03-PLAN.md
  - .planning/phases/23-seed-gaps-close-out/23-03-SUMMARY.md
  - .planning/phases/23-seed-gaps-close-out/23-04-PLAN.md
  - .planning/phases/23-seed-gaps-close-out/23-04-SUMMARY.md
  - .planning/phases/23-seed-gaps-close-out/23-05-PLAN.md
  - .planning/phases/23-seed-gaps-close-out/23-05-SUMMARY.md
  - .planning/phases/23-seed-gaps-close-out/23-06-PLAN.md
  - .planning/phases/23-seed-gaps-close-out/23-06-SUMMARY.md
  - .planning/phases/23-seed-gaps-close-out/23-REVIEW.md
  - internal/acpserve/checkpoint_options_test.go
  - internal/acpserve/config_surface.go
  - internal/checkpoint/store.go
  - internal/checkpoint/store_test.go
  - internal/runtime/commands.go
  - internal/runtime/commands_test.go
  - internal/runtime/restore_guard_test.go
  - internal/runtime/runtime.go
  - internal/runtime/steering_ingress_test.go
  - internal/session/ask.go
  - internal/session/manager.go
  - internal/session/projector.go
  - internal/session/session.go
  - internal/session/steering_test.go
  - internal/session/steerqueue.go
  - internal/session/steerqueue_test.go
  - internal/session/transcript.go
covered_digest: "v1:sha256:c27ca345566e7efb90c9a6ce54ec029235b750b546a245aa5023c7a08120bf1a"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 11/12
  gaps_closed:
    - "Checkpoint restore refuses safely when a turn or engine chain is active — now enforced for ANY session of the Runner, not only the calling session (G-23-1/CR-01, SEEDG-02 truth 8; closed by plan 23-06, commits 0bd4942..346d4be)"
  gaps_remaining: []
  regressions: []
coincidental_reliance_items:
  - truth: "Ticket/cutoff cancel protocol: undelivered steering resolves cancelled-normal at turn death and never reaches any later model window (anti-zombie)"
    reason: incidental-ordering
    harden: "WR-01: CancelAll (steerqueue.go:133-143) reads q.next under one lock, releases, then delegates to CancelThrough — an Enqueue landing in the gap mints next+1 and survives a cancel that already decided it covered everything. Combined with routeSteering's activity-check-THEN-enqueue (runtime.go:1046-1081), an input that passes the activity check can land after the turn's exit resolution and zombie-deliver into the next unrelated turn. Make CancelAll atomic under ONE critical section and/or re-check clientTurnActive/chainCount after Enqueue and self-cancel with a 'turn ended before delivery' note when nothing is active."
human_verification:
  - test: "Live-Zed operator UAT — boundary steering + /undo (WINDOWS #23, PENDING-OPERATOR-CONFIRMATION): launch ass-guard acp in Zed on a scratch git repo; start a multi-tool-call turn; mid-turn type a short steering instruction; after the turn run /undo; then start another turn and mid-turn run /undo; inspect Zed's ACP logs (dev: open acp logs). Full script: 23-05-PLAN.md Task 3."
    expected: "Immediate queued note + 'steering applied: 1 inputs' at the boundary + visible course change without restart; instant restore summary (restored id + pre-restore snapshot id), no model spin, clean git status (no .ass-guard/); mid-turn /undo visibly cancels the turn then restores; no malformed ACP frames. Record what Zed actually does with mid-turn prompts (RESEARCH A2 — feeds TG-02 planning)."
    why_human: "Requires a human editor session in a live Zed client; Zed's client-side prompt queuing is unobservable by tests."
  - test: "Two-live-Zed-session cross-session restore check (WINDOWS #24, PENDING-OPERATOR-CONFIRMATION — confirmatory leg of the closed G-23-1 gap, 23-06 coverage item D4): two Zed sessions on the same workspace; hold a turn open in session A; run /undo from idle session B."
    expected: "B receives the cross-session refusal naming session A and its state (worktree and checkpoint store shared by every session of this process), nothing restored under A's live turn; after A's turn ends, the same /undo from B restores normally."
    why_human: "Requires two live editor sessions against one workspace; the offline battery (TestUndoCrossSessionRefusal) is the gap's automated witness — this leg confirms the operator-visible surface in a real editor."
---

# Phase 23: SEED Gaps Close-out Verification Report

**Phase Goal:** The remaining SEED-004 gaps close: steering/input queue during a running turn (transport-neutral — the Telegram prerequisite, not descoped to queue-behind), checkpoint restore hardened against active turns and nested repos, and /undo exposed as a class-B command.
**Verified:** 2026-09-10T13:26:08Z
**Status:** human_needed
**Re-verification:** Yes — after gap closure (plan 23-06, commits 0bd4942..346d4be)

## Goal Achievement

All three phase pillars (SEEDG-01 steering, SEEDG-02 restore hardening, SEEDG-03 /undo) are delivered, wired, and behaviorally test-pinned. The one gap found by the initial verification — G-23-1/CR-01, the restore guard's session-scoped enforcement over a process-shared workspace — is CLOSED: the guard is now workspace-scoped (`workspaceBlockers` self-excluding walk consulted before any snapshot/cancel/restore at BOTH /undo mutation halves), the busy session is named in the operator-visible refusal and the durable local_command record, and three red-at-HEAD regression tests pin it. This re-verification ran every named battery in its own processes under `-race` — all green, including the full `internal/runtime` package gate with only the documented pre-existing `TestRescanConcurrency` skip. What remains are the two operator UAT legs riding the WINDOWS ledger (#23, #24) — not code gaps.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Steering input typed mid-turn delivers at the NEXT model-request boundary as ONE coalesced marker-wrapped user-role message — never mid-flight, never splitting tool pairs — with the "steering applied: N inputs" note and an anchor-safe, replay-parity projector fold (SC-1, D-01..D-04) | ✓ VERIFIED | Drain seam at session.go:631 sits exactly between the ctx.Err() check and maybeCompact/Project (pairs closed, no request in flight); AppendSteeringDelivery sole-writer via REDACTED path (manager.go:409); projector fold case projector.go:693. Initially verified green under -race; sanity re-run green (TestSteeringDeliveryEndToEnd). 23-06 touched no session files |
| 2 | Ticket/cutoff cancel protocol resolves which queued inputs the running turn acknowledges; at turn death undelivered items resolve cancelled-normal and never reach any later model window (anti-zombie) | ✓ VERIFIED (coincidental-reliance) | recordCanceled funnels all cancelled exits to CancelAll with a stderr note (session.go:1222-1233); teardown riders (turn exit + cancelParkedChains) per 23-05. Sanity re-run green: TestSteeringAntiZombie. Holds incidentally on the racy interleaving — see WR-01 in coincidental_reliance_items (CancelAll two-phase lock, steerqueue.go:133-143) |
| 3 | Mid-turn inputs classify BEFORE the turn mutex; steered prompts return promptly (queued note + end_turn); parked asks visible + cancelable by grammar without killing the turn (SC-2, D-05..D-07) | ✓ VERIFIED | routeSteering at Run head pre-lock (runtime.go:1007-1081) over clientTurnActive/chainCount only; exact-phrase cancel grammar; parked_ask kind + note. Battery green at initial verification; unchanged by 23-06 (routeUndoActive slot widened, classifier untouched) and covered by the full-package green below |
| 4 | The steering queue API is transport-neutral — consumable by a non-ACP frontend (SC-5, TG-02 prerequisite) | ✓ VERIFIED | steerqueue.go imports only sync + time (read in full); TestSteerQueueNoACP (go/parser import-block gate) sanity re-run green |
| 5 | A pre-restore snapshot is a first-class checkpoint object (three-site grammar, deterministic counter, restore parity); a restore NEVER proceeds without it (fail-closed, D-09/D-12) | ✓ VERIFIED | SnapshotPreRestore (store.go:260), both id families validated at all grammar sites; Tests green at initial verification (TestPreRestoreSnapshotFamily, TestIDGrammarTable); fail-closed end-to-end TestUndoFailClosed re-run green in the single-session battery |
| 6 | Nested-repo restores refused outright — .git as dir OR file (gitlink contents silently unprotected) | ✓ VERIFIED | findNestedRepos (store.go:623), typed NestedRepoError, RestoreGuard (store.go:607); prepareUndo refuses before any mutation (commands.go:1257). TestNestedRepoDetectedAndRefused green at initial verification; TestUndoNestedRefusal re-run green |
| 7 | Checkpoint GC is age+count with real object expiry; .ass-guard/ excluded from the user's git via .git/info/exclude (D-08) | ✓ VERIFIED | Sweep + expireByAge/expireByCount (store.go:364-500), de-chained commits make gc --prune=now real; EnsureUserRepoExclude (store.go:502). TestCheckpointGCObjectExpiry, TestUserRepoExclude green at initial verification. WR-02 Warning: the retained prune backstop evicts by (SessionID,TurnNum) grammar order, not recency (store.go:852-865, 920-937) |
| 8 | Checkpoint restore refuses safely when a turn or engine chain is active — for ANY session of the Runner over the shared workspace (SC-3, SEEDG-02) | ✓ VERIFIED (gap closed) | G-23-1/CR-01 closed by 23-06. workspaceBlockers walk (runtime.go:2052-2084): turnActive Range + one chainMu-held activeChains snapshot, self-excluding, state-only (never a turn mutex — promptness pinned). restoreBlockers consults it FIRST, own legs unchanged (:2106-2120); restoreBlockedError.other names the busy session in the operator-visible wording (:2004-2026). Both /undo mutation halves refuse EARLY: restoreUndoActive step 0 before undoSnapshotPreRestore (:1202 < :1206) + post-snapshot discriminator branch (:1218); restoreUndo idle-half consult before its snapshot (commands.go:1294 < :1298) — deliberately the self-excluding walk (the full guard's own-turn leg fires on the caller's own command-carrying turn). Batteries RE-RUN BY THIS VERIFIER under -race: TestRestoreGuardCrossSessionMatrix PASS (promptness 2s row, naming, own-wording byte-stable, idle-after-exit, chain cell); TestUndoCrossSessionRefusal PASS (sentinel survives under A's live turn, durable 'refused: workspace busy', transient retry restores, zero provider calls); TestUndoActivePathCrossSessionRefusal PASS both variants (parked chain NOT cancelled; own client turn NOT cancelled — refusal outranks auto-cancel). Pre-existing cells green unmodified |
| 9 | /undo restores the last checkpoint instantly, zero provider calls, class-B D-05 shape, durable verbatim local_command record; walk reversible, /undo N jumps, edge table, cross-session scoping, loud nil-store degrade (SC-4, SEEDG-03) | ✓ VERIFIED | undo in reservedNames (commands.go:108) + builtinTable (:183); parseUndoDepth edge rules (:1150); recency-ordered undoWalkTarget (:1182); prepareUndo/restoreUndo (:1243-1315). Single-session battery re-run green unmodified: TestClassBUndoIdle/Edges/CrossSession/DegradedStore, TestUndoWalk |
| 10 | /undo with an active turn/chain auto-cancels via the existing cancel contract THEN restores — snapshot precedes cancel (fail-closed), nested refusal outranks the auto path, classified pre-mutex so it never self-deadlocks (D-12); 23-06: a CROSS-SESSION hit outranks the auto-cancel for both own-state variants (nothing cancelled, nothing restored) | ✓ VERIFIED | restoreUndoActive locked sequence verified at runtime.go:1196-1246 (step 0 workspace consult -> snapshot -> guard/discriminator -> cancel -> turnMu -> restore; numbered doc comment updated with the outranking notes). Re-run green: TestUndoAutoCancel, TestUndoFailClosed, TestUndoNestedRefusal, TestParkedChainCancelSteering, plus the new TestUndoActivePathCrossSessionRefusal both variants |
| 11 | A class-B invocation typed mid-turn NEVER reaches the model as steering text (Pitfall 11) | ✓ VERIFIED | resolvesAsCommand predicate + routeUndoActive fill the reserved slot between the ask route and steering enqueue (runtime.go:1046-1081). Covered by the full-package green (-race, skip TestRescanConcurrency only) |
| 12 | checkpoint.expiry_days / checkpoint.max_per_session advertised as enumerated selects, accept-validated, persisted under the checkpoint: layer key, read back via the generic layer map (D-10) | ✓ VERIFIED | config_surface.go:122-123, menu entries :1386-1405, read-back wired to the sweep (runtime.go via checkpointGCBounds). TestCheckpointOptions green at initial verification. WR-03 Warning: `_meta` blob fills for these four numeric ids are recognized-but-inert |

**Score:** 12/12 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| internal/session/steerqueue.go | Transport-neutral SteerQueue (Enqueue/Drain/CancelThrough/CancelAll/Pending) | ✓ VERIFIED | Read in full; single-mutex, nil-safe, stdlib-only imports |
| internal/session/steerqueue_test.go | Ticket/cutoff battery incl. -race hammer + import-block gate | ✓ VERIFIED | All tests enumerated; green at initial verification; sanity re-run green |
| internal/session/steering_test.go | E2E delivery, anti-zombie, projector anchor/replay, parked-ask | ✓ VERIFIED | Named tests enumerated and green at initial verification; sanity re-run green |
| internal/session/transcript.go | TypeSteeringDelivery + TypeParkedAsk kinds with doc discipline | ✓ VERIFIED | :90-103 steering_delivery; parked_ask present (fold-tolerant by omission) |
| internal/session/manager.go | AppendSteeringDelivery + AppendParkedAsk (REDACTED path) | ✓ VERIFIED | :401-416 over appendLine; sole-writer discipline |
| internal/session/session.go | SetSteerQueue + boundary drain + cancelled-exit resolution | ✓ VERIFIED | :452-456, drain at :631 (correct seam), recordCanceled :1222-1233 |
| internal/session/projector.go | Steering fold after flushBatch, anchor untouched | ✓ VERIFIED | Fold case :693 in shared foldExchanges; anchor loop TypeUserMessage-only |
| internal/runtime/runtime.go | Pre-mutex classifier, SteerQueue wiring, Runner store, guard, sweep, cancel registry — guard now WORKSPACE-scoped | ✓ VERIFIED | All present and wired; 23-06 additions verified in code: workspaceBlockers walk (:2052), restoreBlockedError.other + cross-session Error() (:1998-2036), widened restoreBlockers (:2106), crossSessionBlocked (:2126), restoreUndoActive step 0 + discriminator branch (:1200-1230), doc truth incl. residual check-then-act window and cross-PROCESS caveat |
| internal/runtime/steering_ingress_test.go | Ingress battery (pre-mutex, order, combined, engine-chain) | ✓ VERIFIED | Enumerated; green at initial verification; covered by full-package green |
| internal/runtime/restore_guard_test.go | Refusal matrix + session-start sweep + nil-store axis + CROSS-SESSION battery | ✓ VERIFIED | 23-06 adds TestRestoreGuardCrossSessionMatrix (:106), TestUndoCrossSessionRefusal (:650), TestUndoActivePathCrossSessionRefusal (:734, both own-state variants) + fixtures (undoSentinel, runUndoPrompt 2s-timeout, startBlockedTurn, twoBlockProvider) — read in full, substantive; all re-run green under -race by this verifier |
| internal/checkpoint/store.go | SnapshotPreRestore, three-site grammar, RestoreGuard/findNestedRepos, Sweep, EnsureUserRepoExclude | ✓ VERIFIED | All functions present and substantive (read at cited lines) |
| internal/checkpoint/store_test.go | Pre-restore family, grammar table, nested-repo, GC, exclude | ✓ VERIFIED | Enumerated; green at initial verification |
| internal/acpserve/config_surface.go | Two checkpoint selects + persistence + read-back | ✓ VERIFIED | Entries, accept, layer persistence, read-back all present (WR-03 inert-fill warning) |
| internal/runtime/commands.go | undo 14th RESERVED name, parseUndoDepth, walk, prepare/restore — restoreUndo now consults the self-excluding workspace walk pre-snapshot | ✓ VERIFIED | Read :1139-1333; 23-06 consult at :1294 above undoSnapshotPreRestore :1298 with the why-not-full-guard doc comment; zero-provider path structural |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| runTurn iteration top | SteerQueue drain -> AppendSteeringDelivery -> Projector fold | session.go:631 (between ctx.Err() and Project) | ✓ WIRED | Correct seam; pair-safety structural |
| cancelled exits (3x) + teardown riders | SteerQueue CancelAll | recordCanceled (session.go:1229) + unregisterActiveTurn/cancelParkedChains (runtime.go) | ✓ WIRED | Racy residual WR-01 flagged (advisory) |
| Runner.Run head | classifier BEFORE turnMu -> Enqueue -> queued note + end_turn | runtime.go:1007-1081 routeSteering | ✓ WIRED | Pre-mutex pinned behaviorally (TestSteerIngressMidTurnPreMutex) |
| routeAskReply | parked-cancel grammar BEFORE ordinary reply | runtime.go (routeAskReply entry) | ✓ WIRED | Exact-phrase table; TestParkedAskCancelGrammar green |
| sessionFor store construction | Runner-level field -> guard + /undo + sweep reach it | runtime.go:362, 1927-1963, 2110+ | ✓ WIRED | Runner-owned; sessions attach adapter from shared instance |
| readLayerMap -> Sweep arguments | generic layer map read-back | checkpointGCBounds -> sessionFor sweep call | ✓ WIRED | TestCheckpointOptions + TestSessionStartSweep green |
| workspaceBlockers walk -> BOTH /undo mutation halves | idle half: pre-snapshot consult (commands.go:1294); active half: step 0 + post-snapshot discriminator (runtime.go:1202, 1218) | self-excluding walk — break either leg and one interleaving re-opens the race | ✓ WIRED | The two full-path cells are the witnesses (TestUndoCrossSessionRefusal idle leg; TestUndoActivePathCrossSessionRefusal both variants); comment-filtered `restoreBlockers(` in commands.go = 0 (idle half consults ONLY the walk); `workspaceBlockers(sess.SessionID)` count in commands.go = 1 |
| restoreBlockedError.other -> D-05 refusal output + local_command outcome | operator-visible naming ('refused: workspace busy') | Error() cross-session branch (runtime.go:2004-2026) -> undo output + transcript | ✓ WIRED | Asserted behaviorally in all three new tests (strings.Contains(session-id) on emitted output + record.Expansion) |
| Store.List ordering -> D-11 walk target | recency re-sort over both families | undoWalkTarget (commands.go:1182) | ✓ WIRED | Recency + tie-breaks; TestUndoWalk green |

### Data-Flow Trace (Level 4)

Not a data-rendering phase; the equivalent checks are (a) the transcript-as-truth chain for steering — enqueue (SteerQueue) -> drain -> steering_delivery line (sole writer) -> Projector fold live AND on replay (TestSteeringReplayParity re-reads from disk); no static/mock sources — and (b) the refusal-surface chain for 23-06: workspaceBlockers state read -> typed restoreBlockedError.other -> D-05 output + durable local_command record, asserted end-to-end through r.Run (real classifier + intercept) in TestUndoCrossSessionRefusal/TestUndoActivePathCrossSessionRefusal.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Cross-session guard matrix (naming, promptness under held turn mutex, own-wording byte-stable, chain cell, idle-after-exit) | `go test -race -count=1 -run 'TestRestoreGuardCrossSessionMatrix' ./internal/runtime/` | PASS 0.32s | ✓ PASS |
| Cross-session idle full path (refusal naming A, sentinel survives, A alive, durable record, transient retry restores, zero provider calls) | `go test -race -count=1 -run 'TestUndoCrossSessionRefusal' ./internal/runtime/` | PASS 0.49s | ✓ PASS |
| Cross-session active-path outranking, BOTH own-state variants (parked chain not cancelled; own client turn not cancelled) | `go test -race -count=1 -run 'TestUndoActivePathCrossSessionRefusal' ./internal/runtime/` | PASS (2/2 subtests) | ✓ PASS |
| Pre-existing guard cells unmodified | `go test -race -count=1 -run 'TestRestoreGuardRefusalMatrix\|TestRestoreGuardNilStoreAxis' ./internal/runtime/` | PASS | ✓ PASS |
| Single-session /undo battery unmodified | `go test -race -count=1 -run 'TestClassBUndoIdle\|TestClassBUndoEdges\|TestClassBUndoCrossSession\|TestClassBUndoDegradedStore\|TestUndoWalk\|TestUndoAutoCancel\|TestUndoFailClosed\|TestUndoNestedRefusal\|TestParkedChainCancelSteering' ./internal/runtime/` | ok 4.02s | ✓ PASS |
| Full runtime package gate | `go test -race -count=1 -skip 'TestRescanConcurrency' ./internal/runtime/` | ok 42.80s | ✓ PASS |
| Steering sanity (boundary E2E, anti-zombie, transport-neutral import gate) | `go test -race -count=1 -run 'TestSteeringDeliveryEndToEnd\|TestSteeringAntiZombie\|TestSteerQueueNoACP' ./internal/session/` | ok 1.12s | ✓ PASS |
| go vet | `go vet ./internal/runtime/` | clean | ✓ PASS |
| Gap-closure commits exist | `git log` 0bd4942, 2ebf0b7, 09415d3, 0c17c02, 346d4be | all present; diff 5e29d08..HEAD touches exactly runtime.go + commands.go + restore_guard_test.go (+ docs) | ✓ PASS |

(Initial-verification spot-checks for the untouched session/checkpoint/acpserve batteries — TestSteeringProjectAnchorSafety, TestSteeringReplayParity, TestSteerQueueHammerConservation, TestPreRestoreSnapshotFamily, TestIDGrammarTable, TestNestedRepoDetectedAndRefused, TestRestoreCleanNeverDescendsIntoNestedRepos, TestCheckpointGCObjectExpiry, TestUserRepoExclude, TestCheckpointOptions — stand as recorded; those packages' files are byte-identical since, per the bounded 23-06 diff, and were re-run green then.)

### Probe Execution

None declared by the plans (test-gated phase; `scripts/*/tests/probe-*.sh` scan found no phase-declared probes).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|---------------------|----------|----------|
| SEEDG-01 | 23-01, 23-02 | Steering queue during a running turn: boundary drain, ticket/cutoff protocol, parked-ask disambiguation, transport-neutral — not descoped to queue-behind | ✓ SATISFIED | Truths 1-4, 11; queue-behind dead for steerable input (pre-mutex classifier, behaviorally pinned); WR-01 race residual flagged (advisory) |
| SEEDG-02 | 23-03, 23-04, 23-06 | Checkpoint restore guard: refuse with active turn/chains (ANY session of the Runner); pre-restore snapshot; nested-repo refusal; object-expiry GC; .ass-guard/ exclude | ✓ SATISFIED | Truths 5-8, 12 — the initial verification's SEEDG-02 PARTIAL is resolved: the cross-session restore hole closed by 23-06 (truth 8), verified by this re-verification's own battery runs |
| SEEDG-03 | 23-05, 23-06 | /undo command (class-B) restoring last checkpoint | ✓ SATISFIED | Truths 9-10; live-Zed operator confirmation pending (WINDOWS #23) — human item |

Orphaned requirements: none — REQUIREMENTS.md maps exactly SEEDG-01/02/03 to Phase 23 (all marked Complete), all claimed by plans.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/session/steerqueue.go | 133-143 | WR-01: CancelAll two-phase locking (read next, unlock, CancelThrough) + pre-mutex enqueue leaves a race window where a stale steering item survives and zombie-delivers into a later turn | ⚠️ Warning | Narrow timing window; deterministic paths test-pinned; fix is a single-critical-section CancelAll and/or post-enqueue activity re-check; carried as coincidental-reliance on truth 2 |
| internal/checkpoint/store.go | 852-865, 920-937 | WR-02: prune backstop evicts by (SessionID, TurnNum) grammar order, not recency — >50 refs between sweeps can delete a session's newest while keeping another's oldest; doc comment claims tie-breaks "track snapshot sequence" | ⚠️ Warning | /undo walk targets can be lost to session naming; Sweep (the retention authority) is correct; fix = recency-ordered victims (expireByCount discipline) |
| internal/acpserve/config_surface.go | 469, 1728-1758 | WR-03: `_meta` blob fills for checkpoint.*/background.* numeric options recognized but never consulted; explicit Set handlers skip the fill-supersede delete | ⚠️ Warning | Initialize-time defaults silently inert for these four ids; inconsistent with the six resolved menu ids |
| internal/acpserve/config_surface.go | 1806-1824 | IN-03: hand-edited non-positive persisted bounds silently convert to defaults (only parse failures log) | ℹ️ Info | Menu path can't reach 0; hand-edits diverge silently |
| internal/session/steerqueue.go | 41-45, 97-126 | IN-04: cutoff write-only; CancelThrough has no production caller (resolution is exclusively CancelAll) | ℹ️ Info | Dead state invites protocol divergence; expose Cutoff() for TG-02 or delete |
| internal/checkpoint/store.go | 623-662 | IN-05: symlinked directory contents share the gitlink property (unprotected by restore) but are not flagged by findNestedRepos | ℹ️ Info | Same silent-loss shape as gitlinks; flag or document beside the gitlink exclusion |
| multiple | — | IN-01/IN-02: `"call: %w"` error prefixes; stale menu/table count comments (pre-existing, pervades reviewed files) | ℹ️ Info | Debuggability/comment drift only |

The initial verification's CR-01 Blocker row is RESOLVED (23-06) and removed from this table. WR-01..WR-03 and IN-01..IN-05 remain documented review debt, explicitly out of the 23-06 scope guard — none flips a truth. Debt-marker scan (TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER) over the three 23-06-modified files: clean.

### Human Verification Required

### 1. Live-Zed operator UAT — boundary steering + /undo (WINDOWS #23 PENDING-OPERATOR-CONFIRMATION)

**Test:** Launch ass-guard acp in Zed on a scratch git repo; start a multi-tool-call turn; mid-turn type a short steering instruction; after the turn, run /undo; then start another turn and mid-turn run /undo; inspect Zed's ACP logs (`dev: open acp logs`). Full script: 23-05-PLAN.md Task 3.
**Expected:** Immediate queued note + "steering applied: 1 inputs" at the boundary + visible course change without a restart; instant restore summary (restored id + pre-restore snapshot id), no model spin, clean git status (no .ass-guard/); mid-turn /undo visibly cancels the turn then restores; no malformed ACP frames. Record what Zed actually does with mid-turn prompts (RESEARCH A2 — feeds TG-02 planning).
**Why human:** Requires a human editor session in a live Zed client; Zed's client-side prompt queuing is unobservable by tests.

### 2. Two-live-Zed-session cross-session restore check (WINDOWS #24 PENDING-OPERATOR-CONFIRMATION — confirmatory leg of the closed G-23-1 gap)

**Test:** Two Zed sessions on the same workspace; hold a turn open in session A; run /undo from idle session B.
**Expected:** B receives the cross-session refusal naming session A and its state (worktree and checkpoint store shared by every session of this process), nothing restored under A's live turn; after A's turn ends, the same /undo from B restores normally.
**Why human:** Requires two live editor sessions against one workspace; the offline battery (TestUndoCrossSessionRefusal, re-run green by this verifier) is the gap's automated witness — this leg confirms the operator-visible surface in a real editor.

### Gaps Summary

No gaps remain. The single gap from the initial verification — **G-23-1/CR-01: the restore guard was session-scoped while the workspace it mutates is process-shared** — is closed and re-verified independently by this verifier (code read + own battery runs, not SUMMARY claims): the `workspaceBlockers` self-excluding walk (state-only reads, one chainMu snapshot, never a turn mutex) is consulted before any snapshot/cancel/restore at BOTH /undo mutation halves; cross-session refusals name the busy session in the D-05 output and the durable local_command record (`refused: workspace busy`); the refusal outranks the D-12 auto-cancel for both own-state variants; the pre-existing same-session wording and auto-cancel paths are byte-stable (all pre-existing batteries green unmodified); the refusal is transient (retry after the busy session ends restores normally). The residual check-then-act window (activity starting after the consult) is documented at the guard — the same class as the same-session guard, not a gap.

The 23-06 diff is bounded to exactly its three declared files (runtime.go, commands.go, restore_guard_test.go — verified via `git diff --stat 5e29d08..HEAD`), so no other-pillar artifact could regress; the full `internal/runtime` package is green under `-race` with only the documented pre-existing `TestRescanConcurrency` skip, and `go vet` is clean. The two known failing tests elsewhere (TestPermissionsE2E — the STATE cross-workstream note from phase 23's own 40b2bbc per operator bisection; TestRescanConcurrency — phase-22 deferred items) and the evalsuite env-gating keep their documented pre-existing classifications; neither was re-litigated.

Status is `human_needed` solely for the two operator UAT legs riding the WINDOWS ledger (#23 steering + /undo live-Zed UAT; #24 the two-live-Zed-session cross-session refusal check) — both PENDING-OPERATOR-CONFIRMATION, neither a code gap.

---

_Verified: 2026-09-10T13:26:08Z (re-verification after 23-06 gap closure; initial: 2026-09-10T12:12:03Z)_
_Verifier: Claude (gsd-verifier)_
