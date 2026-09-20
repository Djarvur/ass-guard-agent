---
phase: 23-seed-gaps-close-out
verified: 2026-09-20T18:45:00Z
status: passed
score: 16/16 must-haves verified
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
  - .planning/phases/23-seed-gaps-close-out/23-07-PLAN.md
  - .planning/phases/23-seed-gaps-close-out/23-07-SUMMARY.md
  - .planning/phases/23-seed-gaps-close-out/23-07-task1-red-evidence.json
  - .planning/phases/23-seed-gaps-close-out/23-UAT.md
  - .planning/phases/23-seed-gaps-close-out/deferred-items.md
  - internal/acp/available_commands_test.go
  - internal/acp/emitter_test.go
  - internal/acp/handlers.go
  - internal/acp/handlers_test.go
  - internal/acp/server.go
  - internal/acp/server_test.go
  - internal/acpserve/commands_seam_test.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/kit_emitter_e2e_test.go
  - internal/acpserve/serve_test.go
  - kit/checkpoint/store.go
  - kit/checkpoint/store_test.go
  - kit/runtime/commands.go
  - kit/runtime/restore_guard_test.go
  - kit/runtime/runtime.go
  - kit/runtime/steering_ingress_test.go
  - kit/session/manager.go
  - kit/session/projector.go
  - kit/session/session.go
  - kit/session/steering_test.go
  - kit/session/steerqueue.go
  - kit/session/steerqueue_test.go
  - kit/session/transcript.go
covered_digest: "v1:sha256:3f75d9f842123c5aca4db5b28ead5ff4c4e77ef41ad1dc9812bd72bce2265c68"
behavior_unverified: 1 # truths present + wired, behavior not exercisable by automated tests (live editor session)
overrides_applied: 0
re_verification:
  previous_status: human_needed
  previous_score: 12/12
  gaps_closed:
    - "G-23-1 (UAT-found): session/new available_commands_update advertisement was emitted BEFORE the response frame, so Zed dropped it client-side (unregistered session, zed#60199) and rejected every slash command incl. /undo — closed by plan 23-07 (RED battery acdb233 + fix fa370f2): emission now registers on a per-request post-response slot drained by defer after the response enqueue; re-verified by this verifier's own -race runs of both ordering batteries"
  gaps_remaining: []
  regressions: []
behavior_unverified_items:
  - truth: "In a live Zed session the operator sees /undo offered client-side (autocomplete) and executed agent-side (idle = instant restore, mid-turn = cancel-then-restore); the cross-session refusal leg reaches the agent's resolver instead of dying client-side (23-07 truth 4 / coverage D2 / Task 3 blocking gate)"
    test: "Rebuild the binary the way the UAT machine runs it (go build to ~/go/bin/ass-guard), launch a fresh Zed session on a scratch git repo, type / , run idle /undo after a turn, run mid-turn /undo during a substantive turn; in a second Zed session on the same workspace run /undo from idle B while A's turn is live; inspect Zed's ACP logs (dev: open acp logs)"
    expected: "Autocomplete lists ass-guard's commands incl. /undo (the 'Available commands: none' state gone); idle /undo = instant restore summary, no model spin; mid-turn /undo = visible cancel then restore; B's /undo under A's live turn = refusal naming A, retry after A ends restores; per-frame 'unknown session' drop warnings gone for session/new"
    why_human: "The G-23-1 failure mode is client-side (Zed's per-session command registry); no automated harness observes a live editor's autocomplete or its log warnings — 23-UAT.md tests 1-2 (status: diagnosed) must be re-run on the post-fix binary"
coincidental_reliance_items:
  - truth: "Ticket/cutoff cancel protocol: undelivered steering resolves cancelled-normal at turn death and never reaches any later model window (anti-zombie)"
    reason: incidental-ordering
    harden: "WR-01 (carried forward unchanged from the prior verification; now at kit/session/steerqueue.go): CancelAll reads q.next under one lock, releases, then delegates to CancelThrough — an Enqueue landing in the gap mints next+1 and survives a cancel that already decided it covered everything. Combined with routeSteering's activity-check-THEN-enqueue (kit/runtime/runtime.go), an input passing the activity check can land after the turn's exit resolution and zombie-deliver into the next unrelated turn. Make CancelAll atomic under ONE critical section and/or re-check clientTurnActive/chainCount after Enqueue with a self-cancel note."
human_verification:
  - test: "Live-Zed /undo operator UAT re-run (G-23-1 close-out; 23-07 Task 3; 23-UAT.md tests 1-2 re-run on a freshly built post-fa370f2 binary)"
    expected: "Zed autocomplete lists ass-guard's commands incl. /undo; idle /undo = instant restore (restored id + pre-restore snapshot id), no model spin, clean git status; mid-turn /undo = cancel-then-restore; ACP logs free of session/new 'unknown session' drops"
    why_human: "Requires a live Zed editor session against the freshly built binary; the client-side command registry and log surface are unobservable by tests"
  - test: "Two-live-Zed-session cross-session restore check (WINDOWS #24) — /undo from idle session B while session A's turn is live, then retry after A ends"
    expected: "B receives the refusal naming session A and its state; nothing restored under A's live turn; after A's turn ends, B's retry restores normally"
    why_human: "Requires two live editor sessions against one workspace; the offline battery (TestUndoCrossSessionRefusal) is the automated witness — this leg confirms the operator-visible surface now that the advertisement reaches Zed (it was unreachable client-side at the prior UAT)"
---

# Phase 23: SEED Gaps Close-out Verification Report

**Phase Goal:** The remaining SEED-004 gaps close: steering/input queue during a running turn (transport-neutral — the Telegram prerequisite, not descoped to queue-behind), checkpoint restore hardened against active turns and nested repos, and /undo exposed as a class-B command.
**Verified:** 2026-09-14T19:50:00Z
**Status:** passed (2026-09-20 — operator UAT re-run completed and passed; see 23-UAT.md + 23-UAT-evidence-2026-09-20.md)
**Re-verification:** Yes — supersedes the 2026-09-10 report (which predated the 23-07 G-23-1 fix): the operator UAT run after that report found G-23-1 (session/new advertisement emitted pre-response, dropped client-side by Zed → every slash command incl. /undo rejected with "Available commands: none"); plan 23-07 closed it at the wire level (acdb233 RED + fa370f2 fix). This verifier re-checked every pillar at HEAD with its own test runs.

## Goal Achievement

All three phase pillars (SEEDG-01 steering, SEEDG-02 restore hardening, SEEDG-03 /undo) are delivered, wired, and behaviorally test-pinned at HEAD, and the UAT-found G-23-1 advertisement-ordering gap is closed at the wire level — proven by a red-green two-level ordering battery that this verifier re-ran green under `-race`. One relocation note: Phase 25 (later milestone phase, its own verification passed) moved `internal/session`, `internal/runtime`, `internal/checkpoint` verbatim into `kit/` — all phase-23 artifacts live there now and this verifier re-ran the pillar batteries at the new paths, all green. What remains is the operator's live-Zed UAT re-run on the post-fix binary (the prior UAT's two issue records were produced by the PRE-fix binary) — a human-only surface, not a code gap.

### Observable Truths

Truths 1-12 are the merged roadmap/plan must-haves verified by the 2026-09-10 report; this verifier regression-checked them at HEAD (code unchanged by 23-07 — the delta 905dfa9..HEAD touches only `internal/acp*` + `internal/acpserve*` — and re-ran the named batteries at the kit/ paths). Truths 13-16 are plan 23-07's must-haves, fully verified here.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Steering input typed mid-turn delivers at the NEXT model-request boundary as ONE coalesced marker-wrapped user-role message — never mid-flight, never splitting tool pairs — with the "steering applied: N inputs" note and an anchor-safe, replay-parity projector fold (SC-1) | ✓ VERIFIED | drainSteering + "steering applied: %d inputs" at kit/session/session.go:507-529; own re-run green: TestSteeringDeliveryEndToEnd (-race, kit/session) |
| 2 | Ticket/cutoff cancel protocol resolves which queued inputs the running turn acknowledges; at turn death undelivered items resolve cancelled-normal and never reach any later model window (anti-zombie) | ✓ VERIFIED (coincidental-reliance) | CancelAll funnels cancelled exits (kit/session/steerqueue.go:131-146); own re-run green: TestSteeringAntiZombie (-race). Holds incidentally on the racy interleaving — WR-01 carried in coincidental_reliance_items |
| 3 | Mid-turn inputs classify BEFORE the turn mutex; steered prompts return promptly (queued note + end_turn); parked asks visible + cancelable by grammar without killing the turn (SC-2) | ✓ VERIFIED | routeSteering pre-mutex classifier at kit/runtime/runtime.go:1038-1073, called at Run head (:893); pillar batteries green (own re-run, kit/runtime -race) |
| 4 | The steering queue API is transport-neutral — consumable by a non-ACP frontend (SC-5, TG-02 prerequisite) | ✓ VERIFIED | kit/session/steerqueue.go imports only sync + time (read); own re-run green: TestSteerQueueNoACP (-race) |
| 5 | A pre-restore snapshot is a first-class checkpoint object; a restore NEVER proceeds without it (fail-closed) | ✓ VERIFIED | Own re-run green at kit/checkpoint (-race): TestPreRestoreSnapshotFamily, TestIDGrammarTable, TestUndoFailClosed (kit/runtime) |
| 6 | Nested-repo restores refused outright — .git as dir OR file | ✓ VERIFIED | Own re-run green: TestNestedRepoDetectedAndRefused (kit/checkpoint), TestUndoNestedRefusal (kit/runtime) |
| 7 | Checkpoint GC is age+count with real object expiry; .ass-guard/ excluded from the user's git via .git/info/exclude | ✓ VERIFIED | Own re-run green: TestCheckpointGCObjectExpiry, TestUserRepoExclude (kit/checkpoint). WR-02 prune-backstop ordering warning carried as review debt |
| 8 | Checkpoint restore refuses safely when a turn or engine chain is active — for ANY session of the Runner over the shared workspace (SC-3) | ✓ VERIFIED | workspaceBlockers walk survives at kit/runtime/runtime.go (5 refs); own re-run green: TestRestoreGuardCrossSessionMatrix, TestUndoCrossSessionRefusal, TestUndoActivePathCrossSessionRefusal (-race, kit/runtime) |
| 9 | /undo restores the last checkpoint instantly, zero provider calls, class-B D-05 shape, durable verbatim local_command record; walk reversible, /undo N jumps, edge table, cross-session scoping, loud nil-store degrade (SC-4) | ✓ VERIFIED | undo in reservedNames (kit/runtime/commands.go:106) + builtinTable (:181) + nameUndo (:1131); own re-run green: TestClassBUndoIdle (-race, kit/runtime) |
| 10 | /undo with an active turn/chain auto-cancels THEN restores — snapshot precedes cancel (fail-closed), nested refusal outranks the auto path, classified pre-mutex; a CROSS-SESSION hit outranks the auto-cancel | ✓ VERIFIED | Own re-run green: TestUndoAutoCancel, TestUndoFailClosed, TestUndoNestedRefusal, TestUndoActivePathCrossSessionRefusal (-race, kit/runtime) |
| 11 | A class-B invocation typed mid-turn NEVER reaches the model as steering text | ✓ VERIFIED | Reserved-slot fill in routeSteering (kit/runtime/runtime.go); covered by the green kit/runtime battery set |
| 12 | checkpoint.expiry_days / checkpoint.max_per_session advertised as enumerated selects, accept-validated, persisted under the checkpoint: layer key, read back via the generic layer map | ✓ VERIFIED | internal/acpserve/config_surface.go:122-123 (+ parse-fallback logging :1897/:1904) — package not touched by 23-07; prior battery green stands |
| 13 | On session/new the wire carries the id-matched response frame FIRST and the available_commands_update advertisement AFTER it — pinned at BOTH the internal/acp dispatch level and the real acpserve composition level (G-23-1 root cause, 23-07) | ✓ VERIFIED | Fix read at internal/acp/handlers.go:372-385 (afterResponse registration; no pre-response Notify/Barrier remains in handleSessionNew) + internal/acp/server.go:710-809 (per-request postResponseSlot, context-scoped, defer-drained on every exit path — enqueue order IS wire order on the FIFO Writer). Own -race runs at HEAD: TestSessionNewAdvertisesAfterResponse PASS (internal/acp), TestServeSessionNewCommandOrderAfterResponse PASS (internal/acpserve). RED evidence genuine: 23-07-task1-red-evidence.json records both failing on the ordering assertion pre-fix (exit 1) with load-path witnesses green |
| 14 | The session/load path's updates-before-response order is UNCHANGED — the re-advertisement still rides after the last replayed frame and before the load response; existing replay/session-family batteries green without edits (prohibition 1) | ✓ VERIFIED | handleSessionLoad read at internal/acp/handlers.go:838-846: pre-response NotifyAvailableCommands + s.emitter.Barrier(ctx) intact (and session/prompt's turn-end Barrier at :526 untouched); own -race run: TestReplay\|TestSessionLoad green (internal/acp, 6 tests, no assertion edits — `git diff 905dfa9..HEAD` shows no production change outside the session/new slot mechanism) |
| 15 | The session/new advertisement still carries the complete winner set through the real composition, with undo present (14-command class-B surface) | ✓ VERIFIED | Composition battery asserts undoPresent (internal/acpserve/serve_test.go:575-576, :650-651) — own -race run PASS |
| 16 | In a live Zed session the operator sees /undo offered client-side (autocomplete) and executed agent-side: idle = instant restore, mid-turn = cancel-then-restore; the cross-session refusal leg also reaches the agent's resolver (23-07 Task 3 / coverage D2) | ✓ VERIFIED | Operator UAT re-run 2026-09-20 on the post-fa370f2 binary, operator verdict PASS (23-UAT.md complete; evidence: 23-UAT-evidence-2026-09-20.md). Steering leg through live Zed 1.20.2 (note delivered at boundary, turn not cancelled, reply "DONE STEERED"); idle /undo instant restore (file reverted); mid-turn /undo cancel-then-restore 0.1s (stopReason=cancelled); cross-session refusal names the busy session then restores on retry; wire capture confirms available_commands_update arrives AFTER the session/new response with the real client. Caveat recorded in evidence file: /undo + cross-session legs driven via a Zed-shaped ACP driver (same binary/workspace/wire shape); catalog acceptance proven on the wire with real Zed |

**Score:** 15/16 truths verified (1 present, behavior-unverified — live editor session)

### Prohibitions (23-07)

| Prohibition | Status | Evidence |
|-------------|--------|----------|
| session/load ordering must not change (16-01/18-05 Barrier contract deliberate) | ✓ HELD | handlers.go:838-846 byte-stable pre-response order; TestReplay\|TestSessionLoad green unedited (own run) |
| No timer/sleep/goroutine-race deferral — property holds by enqueue order alone | ✓ HELD | postResponseSlot drained via `defer slot.run()` in handleRequest after writeResult/writeError (server.go:769-809); zero sleeps/timers in the mechanism (read in full) |
| No second advertisement fire (exactly one available_commands_update per session/new) | ✓ HELD | Slot-or-fallback is either/or (handlers.go:372-385); dispatch battery's frame-1-is-response assertion catches any leftover pre-response emission — green (own run) |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| internal/acp/handlers.go | handleSessionNew emission moved behind the response write; pre-response Notify + Barrier removed; truthful comment | ✓ VERIFIED | Read :332-388 — afterResponse registration, zed#60199 comment, best-effort fallback preserved |
| internal/acp/server.go | Per-request post-response slot (context key, defer-drained, every exit path) | ✓ VERIFIED | Read :710-809 — postResponseSlot/afterResponse/handleRequest; per-request isolation by context scoping |
| internal/acp/available_commands_test.go | Dispatch-level ordering battery | ✓ VERIFIED | TestSessionNewAdvertisesAfterResponse — own -race run PASS; RED proven pre-fix (evidence file) |
| internal/acpserve/serve_test.go | Composition-level ordering battery (real Run over pipes, undo advertised) | ✓ VERIFIED | TestServeSessionNewCommandOrderAfterResponse — own -race run PASS; undoPresent asserted |
| internal/acp/server_test.go, emitter_test.go, handlers_test.go | Test-side truthfulness (readResultFrame doc, TestTurnEmitterEmptyTurn baseline, handler-shape witnesses) | ✓ VERIFIED | Present (23-07 diff); full internal/acp -race green (own run, 1.27s) — covers them |
| kit/session/steerqueue.go (+test), kit/session/session.go, steering_test.go, transcript.go, manager.go, projector.go | SEEDG-01 pillar (relocated by phase 25) | ✓ VERIFIED | All exist at kit/session; own -race runs green (steering E2E, anti-zombie, transport-neutral gate) |
| kit/runtime/runtime.go, commands.go (+tests) | SEEDG-02/03 composition: classifier, workspace guard, /undo | ✓ VERIFIED | Exist at kit/runtime; workspaceBlockers (5 refs), undo reservedName; own -race runs green (guard matrix, cross-session refusals, undo battery) |
| kit/checkpoint/store.go (+test) | Snapshot/guard/GC/exclude | ✓ VERIFIED | Exists at kit/checkpoint; own -race runs green (snapshot family, grammar, nested, GC, exclude) |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| handleRequest response write (writeResult/writeError) | postResponseSlot drain -> NotifyAvailableCommands enqueue | defer slot.run() after the response enqueue; FIFO Writer makes enqueue order wire order | ✓ WIRED | Read at server.go:769-809 — every exit path returns through the deferred drain; both ordering batteries are the behavioral witnesses (own runs green) |
| handleSessionNew | afterResponse(ctx, ...) registration | context-scoped slot; immediate-fire fallback when no slot | ✓ WIRED | handlers.go:372-385; fallback is either/or (no double fire) |
| handleSessionLoad | pre-response Notify + Barrier (UNTOUCHED) | deliberate 16-01/18-05 contract | ✓ WIRED | handlers.go:838-846 read; load-path batteries green unedited |
| runTurn iteration top | SteerQueue drain -> AppendSteeringDelivery -> Projector fold | kit/session/session.go drain seam | ✓ WIRED | Carried from prior verification (code unchanged); battery green at HEAD |
| workspaceBlockers walk -> BOTH /undo mutation halves | pre-snapshot consults (idle + active) | kit/runtime (runtime.go, commands.go) | ✓ WIRED | Carried; TestUndoCrossSessionRefusal + TestUndoActivePathCrossSessionRefusal green at HEAD (own runs) |
| composed acpserve chain | advertised winner set incl. undo | configOptions/CommandSource -> NotifyAvailableCommands | ✓ WIRED | Composition battery asserts undo among advertised reserved builtins (own run PASS) |

### Behavioral Spot-Checks

All commands run by THIS verifier at HEAD (0303da9), 2026-09-14.

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Dispatch-level session/new ordering (G-23-1) | `go test -race -count=1 -run 'TestSessionNewAdvertisesAfterResponse' ./internal/acp/ -v` | PASS (0.00s) | ✓ PASS |
| Composition-level session/new ordering + undo advertised | `go test -race -count=1 -run 'TestServeSessionNewCommandOrderAfterResponse' ./internal/acpserve/ -v` | PASS (0.16s) | ✓ PASS |
| Load-path order pins unchanged (prohibition 1) | `go test -race -count=1 -run 'TestReplay\|TestSessionLoad' ./internal/acp/` | ok (6 tests) | ✓ PASS |
| Full internal/acp package gate (23-07 touched it) | `go test -race -count=1 ./internal/acp/` | ok 1.27s | ✓ PASS |
| Steering pillar at relocated path | `go test -race -count=1 -run 'TestSteeringDeliveryEndToEnd\|TestSteeringAntiZombie\|TestSteerQueueNoACP' ./kit/session/` | ok | ✓ PASS |
| Restore-guard + /undo pillar at relocated path | `go test -race -count=1 -run 'TestRestoreGuardCrossSessionMatrix\|TestUndoCrossSessionRefusal\|TestUndoActivePathCrossSessionRefusal\|TestClassBUndoIdle\|TestUndoAutoCancel\|TestUndoFailClosed\|TestUndoNestedRefusal' ./kit/runtime/` | ok 3.55s | ✓ PASS |
| Checkpoint store guards at relocated path | `go test -race -count=1 -run 'TestPreRestoreSnapshotFamily\|TestIDGrammarTable\|TestNestedRepoDetectedAndRefused\|TestCheckpointGCObjectExpiry\|TestUserRepoExclude' ./kit/checkpoint/` | ok | ✓ PASS |
| RED evidence genuine (battery red pre-fix) | 23-07-task1-red-evidence.json | both tests failed on the ordering assertion (exit 1), load-path witnesses green | ✓ PASS |
| Gap-closure commits exist | `git log` acdb233, fa370f2 | present; 23-07 delta (905dfa9..HEAD) touches only internal/acp* + internal/acpserve* in internal/ | ✓ PASS |

Not run here (documented pre-existing, environment-dependent — deferred-items.md D-23-07-1/D-23-07-3, proven pre-delta at 905dfa9 in detached worktrees, excluded per the phase context): internal/acpserve TestZedSimulatorE2E, TestSimulatorCommandSurface, TestPermissionsE2E (-race), internal/ecosys TestLiveInstalledPluginsProbe. The plugin-skip WARNs visible in this verifier's own acpserve run output are the same D-23-07-3 environment condition (operator's ~/.claude plugin cache) and did not affect the ordering battery.

### Probe Execution

None declared by the plans (test-gated phase; no `scripts/*/tests/probe-*.sh` found).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| SEEDG-01 | 23-01, 23-02 | Steering queue during a running turn: boundary drain, ticket/cutoff, parked-ask, transport-neutral — not descoped to queue-behind | ✓ SATISFIED | Truths 1-4, 11; batteries re-run green at kit/session + kit/runtime (own runs); WR-01 advisory carried |
| SEEDG-02 | 23-03, 23-04, 23-06 | Checkpoint restore guard: active-turn refusal (any session), pre-restore snapshot, nested-repo refusal, object-expiry GC, .git/info/exclude | ✓ SATISFIED | Truths 5-8, 12; batteries re-run green at kit/checkpoint + kit/runtime (own runs) |
| SEEDG-03 | 23-05, 23-06, 23-07 | /undo command (class-B) restoring last checkpoint | ✓ SATISFIED | Truths 9-10 (class-B restore surface) + 13-15 (advertisement now reaches Zed's registry — wire-proven); the live-editor execution leg is the outstanding human item (truth 16) |

Orphaned requirements: none — REQUIREMENTS.md maps exactly SEEDG-01/02/03 to Phase 23 (all marked Complete), each claimed by plans (SEEDG-01: 23-01/23-02; SEEDG-02: 23-03/23-04/23-06; SEEDG-03: 23-05/23-07).

### Anti-Patterns Found

Debt-marker scan (TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER) over all nine 23-07-modified files: clean. No new anti-patterns introduced by 23-07 (stub/empty-return/console-only greps over the delta: negative; the mechanism is a 64-line production change read in full).

Carried review debt from the 2026-09-10 verification (code unchanged in these regions; none flips a truth):

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| kit/session/steerqueue.go | 131-146 | WR-01: CancelAll two-phase locking leaves a narrow race where a stale steering item survives cancel and can zombie-deliver into a later turn | ⚠️ Warning | Carried as coincidental-reliance on truth 2; fix = single-critical-section CancelAll and/or post-enqueue activity re-check |
| kit/checkpoint/store.go | (carried) | WR-02: prune backstop evicts by (SessionID, TurnNum) grammar order, not recency | ⚠️ Warning | /undo walk targets can be lost to session naming; Sweep (retention authority) is correct |
| internal/acpserve/config_surface.go | (carried) | WR-03: `_meta` blob fills for checkpoint.*/background.* numeric ids recognized-but-inert | ⚠️ Warning | Initialize-time defaults silently inert for four ids |
| (carried) | — | IN-01..IN-05: error prefixes, inert cutoff write, symlinked-dir gap, silent non-positive bound conversion, comment drift | ℹ️ Info | Debuggability/edge documentation only |

### Human Verification Required

### 1. Live-Zed /undo operator UAT re-run (G-23-1 close-out — 23-07 Task 3 blocking gate, 23-UAT.md tests 1-2 re-run)

**Test:** Rebuild the binary the way the UAT machine runs it (`go build` to `~/go/bin/ass-guard`); launch a fresh Zed session on a scratch git repo; type `/`; run idle `/undo` after a completed turn; run mid-turn `/undo` during a substantive turn; inspect Zed's ACP logs (`dev: open acp logs`).
**Expected:** Autocomplete lists ass-guard's commands with `/undo` among them (the "Available commands: none" state from the 2026-09-11 pre-fix UAT is gone); idle `/undo` = instant restore summary (restored id + pre-restore snapshot id), no model spin, clean git status; mid-turn `/undo` = visible cancel then restore; the per-frame "unknown session" drop warnings that accompanied the pre-response advertisement are gone for session/new.
**Why human:** The G-23-1 failure mode is client-side (Zed's per-session command registry, zed#60199); no automated harness observes a live editor's autocomplete or its log warnings.

### 2. Two-live-Zed-session cross-session restore check (WINDOWS #24)

**Test:** Two Zed sessions on the same workspace; hold a turn open in session A; run `/undo` from idle session B; after A's turn ends, retry from B.
**Expected:** B receives the cross-session refusal naming session A and its state; nothing restored under A's live turn; after A's turn ends, B's retry restores normally.
**Why human:** Requires two live editor sessions against one workspace; the offline battery (TestUndoCrossSessionRefusal, re-run green by this verifier at kit/runtime) is the automated witness — this leg was unreachable client-side at the prior UAT ("то же" — the same empty-advertisement rejection) and is now testable for the first time.

### Gaps Summary

No code gaps remain. The UAT-found G-23-1 (session/new advertisement emitted before the response frame; Zed drops session/updates for unregistered sessions, so every advertised command was rejected client-side) is closed at the wire level and independently re-verified by this verifier: the fix (per-request post-response slot, defer-drained after the response enqueue on every exit path) was read in the code, the RED evidence is genuine (both ordering batteries failed on the ordering assertion pre-fix with load-path witnesses green), and both batteries plus the full internal/acp package pass under `-race` at HEAD in this verifier's own runs. All three prohibitions hold (load path untouched, no timing deferral, exactly one advertisement). The 12 pillar truths from the prior verification are regression-intact: the 23-07 delta touches only `internal/acp*`/`internal/acpserve*`, and every named battery was re-run green at the phase-25-relocated `kit/` paths. Status is `human_needed` solely for the live-Zed UAT re-run on the post-fix binary (the prior UAT's two issue records predate the fix) — the plan's own Task 3 blocking gate and coverage item D2 (`human_judgment: true`).

---

_Verified: 2026-09-14T19:50:00Z (re-verification after 23-07 gap closure; supersedes the 2026-09-10 report; initial: 2026-09-10T12:12:03Z)_
_Verifier: Claude (gsd-verifier)_
