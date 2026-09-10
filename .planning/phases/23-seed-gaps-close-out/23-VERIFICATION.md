---
phase: 23-seed-gaps-close-out
verified: 2026-09-10T12:12:03Z
status: gaps_found
score: 11/12 must-haves verified
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
covered_digest: "v1:sha256:a31af16ba288338a0d1b88311b2e97245ea2cc1dc69809c456bdf24b97119f95"
behavior_unverified: 0
overrides_applied: 0
gaps:
  - truth: "Checkpoint restore refuses safely when a turn or engine chain is active (ROADMAP SC-3 / SEEDG-02)"
    status: failed
    reason: >-
      CR-01 (23-REVIEW, independently re-confirmed in code): restoreBlockers enforces the
      active-state refusal for the CALLING session only, while the workspace and the
      checkpoint store it mutates are process-shared. runtime.go:1995-2005 checks only
      clientTurnActive(sessionID) and chainCount(sessionID); workDir is a Runner-level
      field shared by every session (runtime.go:113, comment at :245); the store is
      workspace-scoped and shared (runtime.go:274, :362, checkpointStore() at :1927);
      turn serialization is per-session (turnMus sync.Map at :278) so two sessions run
      turns concurrently on different mutexes. A /undo from an idle session passes the
      guard while ANOTHER session's turn is mid-flight, then Store.Restore (store.go:556-583)
      runs checkout --no-overlay -f + clean -fd over the SHARED worktree — destroying the
      live turn's in-flight work, which the turn then launders into its next snapshot.
      The guard's doc comment (:1985-1994) documents only the cross-PROCESS CLI caveat;
      this hole is entirely in-process where detection is trivially available. No test
      covers it: TestRestoreGuardRefusalMatrix (restore_guard_test.go:28) uses one
      session id ("sess-guard") for both the live turn and the guard call.
    artifacts:
      - path: internal/runtime/runtime.go
        issue: "restoreBlockers (:1995-2005) session-scoped; no workspace-wide activity walk (turnActive.Range / anyChainCount) and no documentation of the session-scoped limitation"
      - path: internal/runtime/restore_guard_test.go
        issue: "refusal matrix exercises same-session activity only; no cross-session cell"
    missing:
      - "Make restoreBlockers refuse when ANY session of the Runner has activity (Range over turnActive excluding the calling session + an anyChainCount walk over activeChains), naming the calling session's own state in the message; OR document the session-scoped limitation at the guard AND surface it in /undo's output ('another session of this workspace is active')"
      - "Add the cross-session refusal-matrix test cell: session A turn in flight (blocked fake provider), /undo from idle session B, expect refusal (or the documented warning) — never a restore under A's live turn"
coincidental_reliance_items:
  - truth: "Ticket/cutoff cancel protocol: undelivered steering resolves cancelled-normal at turn death and never reaches any later model window (anti-zombie)"
    reason: incidental-ordering
    harden: "WR-01: CancelAll (steerqueue.go:133-143) reads q.next under one lock, releases, then delegates to CancelThrough — an Enqueue landing in the gap mints next+1 and survives a cancel that already decided it covered everything. Combined with routeSteering's activity-check-THEN-enqueue (runtime.go:1046-1081), an input that passes the activity check can land after the turn's exit resolution and zombie-deliver into the next unrelated turn. Make CancelAll atomic under ONE critical section and/or re-check clientTurnActive/chainCount after Enqueue and self-cancel with a 'turn ended before delivery' note when nothing is active."
deferred: []
---

# Phase 23: SEED Gaps Close-out Verification Report

**Phase Goal:** The remaining SEED-004 gaps close: steering/input queue during a running turn (transport-neutral — the Telegram prerequisite, not descoped to queue-behind), checkpoint restore hardened against active turns and nested repos, and /undo exposed as a class-B command.
**Verified:** 2026-09-10T12:12:03Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

Two of the three phase pillars (SEEDG-01 steering, SEEDG-03 /undo) are delivered, wired, and behaviorally test-pinned — every named battery re-run by this verifier passed under `-race`. The third pillar (SEEDG-02 restore hardening) is delivered at the store level and for the same-session case, but its central invariant — restore never happens under an active turn/chain — is enforced **per session only** while the workspace it mutates is **process-shared**: a `/undo` from an idle session can run `checkout -f` + `clean -fd` underneath another session's live turn (CR-01, re-confirmed in code by this verifier, not just taken from 23-REVIEW.md).

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Steering input typed mid-turn delivers at the NEXT model-request boundary as ONE coalesced marker-wrapped user-role message — never mid-flight, never splitting tool pairs — with the "steering applied: N inputs" note and an anchor-safe, replay-parity projector fold (SC-1, D-01..D-04) | ✓ VERIFIED | Drain seam at session.go:631 sits exactly between the ctx.Err() check and maybeCompact/Project (pairs closed, no request in flight); AppendSteeringDelivery sole-writer via REDACTED path (manager.go:409); projector fold case projector.go:693. Tests re-run green under -race: TestSteeringDeliveryEndToEnd, TestSteeringProjectAnchorSafety, TestSteeringReplayParity |
| 2 | Ticket/cutoff cancel protocol resolves which queued inputs the running turn acknowledges; at turn death undelivered items resolve cancelled-normal and never reach any later model window (anti-zombie) | ✓ VERIFIED (coincidental-reliance) | recordCanceled funnels all cancelled exits to CancelAll with a stderr note (session.go:1222-1233); teardown riders (turn exit + cancelParkedChains) per 23-05. Tests re-run green: TestSteeringAntiZombie, TestSteerQueueHammerConservation (-race), TestParkedChainCancelSteering. Holds incidentally on the racy interleaving — see WR-01 in coincidental_reliance_items (CancelAll two-phase lock, steerqueue.go:133-143) |
| 3 | Mid-turn inputs classify BEFORE the turn mutex; steered prompts return promptly (queued note + end_turn); parked asks visible + cancelable by grammar without killing the turn (SC-2, D-05..D-07) | ✓ VERIFIED | routeSteering at Run head pre-lock (runtime.go:1007-1081) over clientTurnActive/chainCount only; exact-phrase cancel grammar; parked_ask kind + note. Tests re-run green: TestSteerIngressMidTurnPreMutex, TestCombinedSteerScenario, TestParkedAskRecord; TestParkedAskReplayTolerance enumerated |
| 4 | The steering queue API is transport-neutral — consumable by a non-ACP frontend (SC-5, TG-02 prerequisite) | ✓ VERIFIED | steerqueue.go imports only sync + time (read in full); TestSteerQueueNoACP (go/parser import-block gate) re-run green |
| 5 | A pre-restore snapshot is a first-class checkpoint object (three-site grammar, deterministic counter, restore parity); a restore NEVER proceeds without it (fail-closed, D-09/D-12) | ✓ VERIFIED | SnapshotPreRestore (store.go:260), both id families validated at all grammar sites; Tests re-run green: TestPreRestoreSnapshotFamily, TestIDGrammarTable; fail-closed end-to-end TestUndoFailClosed |
| 6 | Nested-repo restores refused outright — .git as dir OR file (gitlink contents silently unprotected) | ✓ VERIFIED | findNestedRepos (store.go:623), typed NestedRepoError, RestoreGuard (store.go:607); prepareUndo refuses before any mutation (commands.go:1257). Tests re-run green: TestNestedRepoDetectedAndRefused, TestRestoreCleanNeverDescendsIntoNestedRepos |
| 7 | Checkpoint GC is age+count with real object expiry; .ass-guard/ excluded from the user's git via .git/info/exclude (D-08) | ✓ VERIFIED | Sweep + expireByAge/expireByCount (store.go:364-500), de-chained commits make gc --prune=now real; EnsureUserRepoExclude (store.go:502). Tests re-run green: TestCheckpointGCObjectExpiry, TestUserRepoExclude. WR-02 Warning: the retained prune backstop evicts by (SessionID,TurnNum) grammar order, not recency (store.go:852-865, 920-937) |
| 8 | Checkpoint restore refuses safely when a turn or engine chain is active (SC-3, SEEDG-02) | ✗ FAILED | CR-01 re-confirmed: restoreBlockers (runtime.go:1995-2005) checks ONLY the calling session while workDir (runtime.go:113), the checkpoint store (runtime.go:274,1927), and the worktree Store.Restore mutates (store.go:556-583: checkout --no-overlay -f + clean -fd) are shared process-wide; turn mutexes are per-session (runtime.go:278). Cross-session restore under a live turn is unguarded and untested (TestRestoreGuardRefusalMatrix is same-session only). Same-session matrix, prompt-return, and nil-store axes ARE green (TestRestoreGuardRefusalMatrix, TestRestoreGuardNilStoreAxis) — the hole is the cross-session cell only |
| 9 | /undo restores the last checkpoint instantly, zero provider calls, class-B D-05 shape, durable verbatim local_command record; walk reversible, /undo N jumps, edge table, cross-session scoping, loud nil-store degrade (SC-4, SEEDG-03) | ✓ VERIFIED | undo in reservedNames (commands.go:108) + builtinTable (:183); parseUndoDepth edge rules (:1150); recency-ordered undoWalkTarget (:1182); prepareUndo/restoreUndo (:1243-1304). Tests re-run green: TestClassBUndoIdle, TestUndoWalk, TestClassBUndoEdges (+ TestClassBUndoCrossSession/DegradedStore enumerated) |
| 10 | /undo with an active turn/chain auto-cancels via the existing cancel contract THEN restores — snapshot precedes cancel (fail-closed), nested refusal outranks the auto path, classified pre-mutex so it never self-deadlocks (D-12) | ✓ VERIFIED | restoreUndoActive locked sequence verified at runtime.go:1182-1219 (snapshot -> guard consultation -> cancel -> turnMu -> restore). Tests re-run green: TestUndoAutoCancel, TestUndoFailClosed, TestUndoNestedRefusal, TestParkedChainCancelSteering (5s deadlock guards) |
| 11 | A class-B invocation typed mid-turn NEVER reaches the model as steering text (Pitfall 11) | ✓ VERIFIED | resolvesAsCommand predicate + routeUndoActive fill the reserved slot between the ask route and steering enqueue (runtime.go:1046-1081). Tests enumerated + green in battery: TestSteerIngressClassBNotSteered; request-cleanliness assertions inside TestUndoAutoCancel/TestParkedChainCancelSteering |
| 12 | checkpoint.expiry_days / checkpoint.max_per_session advertised as enumerated selects, accept-validated, persisted under the checkpoint: layer key, read back via the generic layer map (D-10) | ✓ VERIFIED | config_surface.go:122-123, menu entries :1386-1405, read-back wired to the sweep (runtime.go:2119-2127 via checkpointGCBounds). Tests re-run green: TestCheckpointOptions. WR-03 Warning: `_meta` blob fills for these four numeric ids are recognized-but-inert (stored at :469, never consulted by effectiveCheckpointBoundLocked) |

**Score:** 11/12 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| internal/session/steerqueue.go | Transport-neutral SteerQueue (Enqueue/Drain/CancelThrough/CancelAll/Pending) | ✓ VERIFIED | Read in full; single-mutex, nil-safe, stdlib-only imports |
| internal/session/steerqueue_test.go | Ticket/cutoff battery incl. -race hammer + import-block gate | ✓ VERIFIED | All tests enumerated; hammer + CancelThrough/CancelAll re-run green under -race |
| internal/session/steering_test.go | E2E delivery, anti-zombie, projector anchor/replay, parked-ask | ✓ VERIFIED | Named tests enumerated and re-run green |
| internal/session/transcript.go | TypeSteeringDelivery + TypeParkedAsk kinds with doc discipline | ✓ VERIFIED | :90-103 steering_delivery; parked_ask present (fold-tolerant by omission) |
| internal/session/manager.go | AppendSteeringDelivery + AppendParkedAsk (REDACTED path) | ✓ VERIFIED | :401-416 over appendLine; sole-writer discipline |
| internal/session/session.go | SetSteerQueue + boundary drain + cancelled-exit resolution | ✓ VERIFIED | :452-456, drain at :631 (correct seam), recordCanceled :1222-1233 |
| internal/session/projector.go | Steering fold after flushBatch, anchor untouched | ✓ VERIFIED | Fold case :693 in shared foldExchanges; anchor loop TypeUserMessage-only |
| internal/runtime/runtime.go | Pre-mutex classifier, SteerQueue wiring, Runner store, guard, sweep, cancel registry | ✓ VERIFIED (with gap) | All present and wired; restoreBlockers scope is the CR-01 gap (truth 8) |
| internal/runtime/steering_ingress_test.go | Ingress battery (pre-mutex, order, combined, engine-chain) | ✓ VERIFIED | Enumerated; named tests re-run green |
| internal/runtime/restore_guard_test.go | Refusal matrix + session-start sweep + nil-store axis | ⚠️ COVERAGE GAP | Same-session cells green; no cross-session cell (CR-01) |
| internal/checkpoint/store.go | SnapshotPreRestore, three-site grammar, RestoreGuard/findNestedRepos, Sweep, EnsureUserRepoExclude | ✓ VERIFIED | All functions present and substantive (read at cited lines) |
| internal/checkpoint/store_test.go | Pre-restore family, grammar table, nested-repo, GC, exclude | ✓ VERIFIED | Enumerated; named tests re-run green |
| internal/acpserve/config_surface.go | Two checkpoint selects + persistence + read-back | ✓ VERIFIED | Entries, accept, layer persistence, read-back all present (WR-03 inert-fill warning) |
| internal/runtime/commands.go | undo 14th RESERVED name, parseUndoDepth, walk, prepare/restore | ✓ VERIFIED | Read :1139-1322; all present, zero-provider path structural |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| runTurn iteration top | SteerQueue drain -> AppendSteeringDelivery -> Projector fold | session.go:631 (between ctx.Err() and Project) | ✓ WIRED | Correct seam; pair-safety structural |
| cancelled exits (3x) + teardown riders | SteerQueue CancelAll | recordCanceled (session.go:1229) + unregisterActiveTurn/cancelParkedChains (runtime.go) | ✓ WIRED | Racy residual WR-01 flagged (advisory) |
| Runner.Run head | classifier BEFORE turnMu -> Enqueue -> queued note + end_turn | runtime.go:1007-1081 routeSteering | ✓ WIRED | Pre-mutex pinned behaviorally (TestSteerIngressMidTurnPreMutex) |
| routeAskReply | parked-cancel grammar BEFORE ordinary reply | runtime.go (routeAskReply entry) | ✓ WIRED | Exact-phrase table; TestParkedAskCancelGrammar green |
| sessionFor store construction | Runner-level field -> guard + /undo + sweep reach it | runtime.go:362, 1927-1963, 2110-2127 | ✓ WIRED | Runner-owned; sessions attach adapter from shared instance |
| readLayerMap -> Sweep arguments | generic layer map read-back | checkpointGCBounds -> sessionFor sweep call | ✓ WIRED | TestCheckpointOptions + TestSessionStartSweep green |
| classifier class-B slot -> undo | nested check -> snapshot -> cancel -> turnMu -> restore | routeUndoActive -> restoreUndoActive (runtime.go:1182-1219) | ✓ WIRED | Ordering verified in code + behavioral timeout tests |
| Store.List ordering -> D-11 walk target | recency re-sort over both families | undoWalkTarget (commands.go:1182) | ✓ WIRED | Recency + tie-breaks; TestUndoWalk green |

### Data-Flow Trace (Level 4)

Not a data-rendering phase; the equivalent check is the transcript-as-truth chain, which flows end-to-end: enqueue (SteerQueue) -> drain -> steering_delivery line (sole writer) -> Projector fold live AND on replay (TestSteeringReplayParity re-reads from disk). No static/mock sources found in the chain.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Boundary steering E2E (marker in request 2, pairs intact) | `go test -race -run TestSteeringDeliveryEndToEnd ./internal/session/` | PASS 0.01s | ✓ PASS |
| Anti-zombie (cancel before boundary -> clean next turn) | `go test -race -run TestSteeringAntiZombie ./internal/session/` | PASS | ✓ PASS |
| Queue conservation under -race hammer | `go test -race -run 'TestSteerQueueHammerConservation\|TestSteerQueueCancelThrough\|TestSteerQueueCancelAll' ./internal/session/` | ok 1.04s | ✓ PASS |
| Transport-neutrality import gate | `go test -race -run TestSteerQueueNoACP ./internal/session/` | PASS | ✓ PASS |
| Pre-mutex steering (returns while provider blocked) | `go test -race -run TestSteerIngressMidTurnPreMutex ./internal/runtime/` | PASS 0.26s | ✓ PASS |
| Combined scenario + projector safety/replay | same runtime run + session run | all PASS | ✓ PASS |
| Pre-restore family + grammar table | `go test -run 'TestPreRestoreSnapshotFamily\|TestIDGrammarTable' ./internal/checkpoint/` | PASS | ✓ PASS |
| Nested-repo refusal + clean non-descent | `go test -run 'TestNestedRepoDetectedAndRefused\|TestRestoreCleanNeverDescendsIntoNestedRepos' ./internal/checkpoint/` | PASS | ✓ PASS |
| GC object expiry + user-repo exclude | `go test -run 'TestCheckpointGCObjectExpiry\|TestUserRepoExclude' ./internal/checkpoint/` | PASS | ✓ PASS |
| Restore guard matrix + session-start sweep | `go test -race -run 'TestRestoreGuardRefusalMatrix\|TestSessionStartSweep' ./internal/runtime/` | PASS | ✓ PASS (same-session scope only — see gap) |
| /undo idle + walk + edges | `go test -race -run 'TestClassBUndoIdle\|TestUndoWalk\|TestClassBUndoEdges' ./internal/runtime/` | PASS | ✓ PASS |
| /undo auto-cancel + fail-closed + nested refusal + chain-steering | `go test -race -run 'TestUndoAutoCancel$\|TestUndoFailClosed\|TestUndoNestedRefusal\|TestParkedChainCancelSteering' ./internal/runtime/` | PASS | ✓ PASS |
| Checkpoint configOptions advertise/persist/read-back | `go test -race -run TestCheckpointOptions ./internal/acpserve/` | PASS | ✓ PASS |
| All 23 phase commits exist | `git cat-file -t` over the 23 ledger hashes | all OK | ✓ PASS |

### Probe Execution

None declared by the plans (test-gated phase; `scripts/*/tests/probe-*.sh` scan found no phase-declared probes).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|---------------------|----------|----------|
| SEEDG-01 | 23-01, 23-02 | Steering queue during a running turn: boundary drain, ticket/cutoff protocol, parked-ask disambiguation, transport-neutral — not descoped to queue-behind | ✓ SATISFIED | Truths 1-4, 11; queue-behind dead for steerable input (pre-mutex classifier, behaviorally pinned); WR-01 race residual flagged |
| SEEDG-02 | 23-03, 23-04 | Checkpoint restore guard: refuse with active turn/chains; pre-restore snapshot; nested-repo refusal; object-expiry GC; .ass-guard/ exclude | ✗ PARTIAL — GAP | Store-level guards, snapshot, nested refusal, GC, exclude all verified (truths 5-7, 12); the active-state refusal holds only per-session — cross-session restore under a live turn is unguarded (truth 8, CR-01) |
| SEEDG-03 | 23-05 | /undo command (class-B) restoring last checkpoint | ✓ SATISFIED | Truths 9-10; live-Zed operator confirmation pending (WINDOWS #23) — human item |

Orphaned requirements: none — REQUIREMENTS.md maps exactly SEEDG-01/02/03 to Phase 23, all claimed by plans.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/runtime/runtime.go | 1995-2005 | CR-01: restore guard session-scoped while workspace/store process-shared — restore can run under another session's live turn (checkout -f + clean -fd over the shared worktree) | 🛑 Blocker | Mid-turn work of a concurrent session destroyed while the operator is told "undo complete"; the exact data-loss class the guard exists to prevent. Structured as the phase gap above |
| internal/session/steerqueue.go | 133-143 | WR-01: CancelAll two-phase locking (read next, unlock, CancelThrough) + pre-mutex enqueue leaves a race window where a stale steering item survives and zombie-delivers into a later turn | ⚠️ Warning | Narrow timing window; deterministic paths test-pinned; fix is a single-critical-section CancelAll and/or post-enqueue activity re-check |
| internal/checkpoint/store.go | 852-865, 920-937 | WR-02: prune backstop evicts by (SessionID, TurnNum) grammar order, not recency — >50 refs between sweeps can delete a session's newest while keeping another's oldest; doc comment claims tie-breaks "track snapshot sequence" | ⚠️ Warning | /undo walk targets can be lost to session naming; Sweep (the retention authority) is correct; fix = recency-ordered victims (expireByCount discipline) |
| internal/acpserve/config_surface.go | 469, 1728-1758 | WR-03: `_meta` blob fills for checkpoint.*/background.* numeric options recognized but never consulted; explicit Set handlers skip the fill-supersede delete | ⚠️ Warning | Initialize-time defaults silently inert for these four ids; inconsistent with the six resolved menu ids |
| internal/acpserve/config_surface.go | 1806-1824 | IN-03: hand-edited non-positive persisted bounds silently convert to defaults (only parse failures log) | ℹ️ Info | Menu path can't reach 0; hand-edits diverge silently |
| internal/session/steerqueue.go | 41-45, 97-126 | IN-04: cutoff write-only; CancelThrough has no production caller (resolution is exclusively CancelAll) | ℹ️ Info | Dead state invites protocol divergence; expose Cutoff() for TG-02 or delete |
| internal/checkpoint/store.go | 623-662 | IN-05: symlinked directory contents share the gitlink property (unprotected by restore) but are not flagged by findNestedRepos | ℹ️ Info | Same silent-loss shape as gitlinks; flag or document beside the gitlink exclusion |
| multiple | — | IN-01/IN-02: `"call: %w"` error prefixes; stale menu/table count comments (pre-existing, pervades reviewed files) | ℹ️ Info | Debuggability/comment drift only |

Debt-marker scan (TBD/FIXME/XXX and TODO/HACK/PLACEHOLDER) over all 16 phase-modified files: clean — no unreferenced markers.

### Human Verification Required

### 1. Live-Zed operator UAT — boundary steering + /undo (WINDOWS #23 PENDING-OPERATOR-CONFIRMATION)

**Test:** Launch ass-guard acp in Zed on a scratch git repo; start a multi-tool-call turn; mid-turn type a short steering instruction; after the turn, run /undo; then start another turn and mid-turn run /undo; inspect Zed's ACP logs (`dev: open acp logs`). Full script: 23-05-PLAN.md Task 3.
**Expected:** Immediate queued note + "steering applied: 1 inputs" at the boundary + visible course change without a restart; instant restore summary (restored id + pre-restore snapshot id), no model spin, clean git status (no .ass-guard/); mid-turn /undo visibly cancels the turn then restores; no malformed ACP frames. Record what Zed actually does with mid-turn prompts (RESEARCH A2 — feeds TG-02 planning).
**Why human:** Requires a human editor session in a live Zed client; Zed's client-side prompt queuing is unobservable by tests.

### 2. (Post-fix) Cross-session restore safety

**Test:** Two Zed sessions on the same workspace; hold a turn open in session A; run /undo from idle session B.
**Expected:** After the CR-01 gap is closed — a refusal naming the other session's activity (or the documented warning), never a restore under A's live turn. Until closed, expect the destructive race described in the gap.
**Why human:** Requires two live editor sessions; the concurrency interleaving is not reachable from the single-session test fixtures.

### Gaps Summary

One gap blocks the phase goal, and it is the review's Critical re-confirmed independently in code (not trusted from 23-REVIEW.md):

**CR-01 — the restore guard is session-scoped, the workspace is process-shared.** `restoreBlockers` (runtime.go:1995-2005) refuses only for the calling session's `clientTurnActive`/`chainCount`, but every session of the Runner shares one `workDir` (runtime.go:113), one workspace-scoped checkpoint store (runtime.go:274, :1927), and the worktree that `Store.Restore` force-checkouts and clean -fd's (store.go:556-583) — while turn serialization is per-session (runtime.go:278). An idle session's `/undo` therefore passes the guard and mutates the shared worktree under another session's live turn, destroying that turn's in-flight work; the live turn then snapshots the torn tree at its next boundary, laundering the corruption. The guard's doc comment covers only the cross-process CLI caveat; nothing documents the in-process cross-session limitation, and no test exercises it (`TestRestoreGuardRefusalMatrix` uses one session id throughout). The phase invariant "checkpoint restore hardened against active turns" is stated unscoped (ROADMAP SC-3, phase goal, SEEDG-02) — the implementation delivers it only within one session. Fix: refuse when ANY session of the Runner has activity (Range over turnActive + a chain-count walk excluding the caller), or explicitly document and surface the session-scoped limitation; add the cross-session test cell. Not deferred: neither Phase 24 (docs/ops/scheduler/CI) nor Phase 25 (kit extraction) covers it.

Three Warnings (WR-01 steering turn-death race; WR-02 prune grammar-order eviction; WR-03 inert blob fills) and five Infos are recorded above; none flip a truth to FAILED under the review's severity semantics, but WR-01's anti-zombie race is flagged as coincidental-reliance so it survives this report.

Everything else the phase promised is real: 23/23 ledger commits exist on the branch, the working-tree "modifications" are mode-only (0 content deltas), all named batteries re-ran green under `-race` in this verifier's own processes, and the two known failing tests (TestRescanConcurrency, TestPermissionsE2E) are the documented pre-existing/base classifications in STATE — not re-litigated here.

---

_Verified: 2026-09-10T12:12:03Z_
_Verifier: Claude (gsd-verifier)_
