---
phase: quick-261013-uhx
plan: "01"
subsystem: testing
tags: [go, steering, acp, race-detector, nightly-ci, permissions-gate, engine-chain]
status: complete

requires:
  - phase: Phase 23 (23-02, commit 40b2bbc)
    provides: pre-mutex steering ingress classifier + parked-ask visibility/cancel
  - phase: Phase 25 kit extraction
    provides: the kit/ paths the 40b2bbc code now lives at (kit/runtime, kit/session)
provides:
  - "Root-caused and fixed the TestPermissionsE2E -race regression (WINDOWS #22): steering enqueued during an engine chain's completion window is now delivered as an ordinary spillover turn at chain exit (kit/runtime spillOrphanedSteering)"
  - "Deterministic sub-second regression pin TestSteerSpilloverOnChainExit in kit/runtime/steering_ingress_test.go (RED/GREEN demonstrated)"
  - "Nightly -race drift gate runs the phase-17 permission surface again — TestPermissionsE2E removed from .github/workflows/nightly.yml -skip regex and reasons comment (WINDOWS #27 entry emptied the honest way)"
affects: [261013-uhy (sibling owns the TestRescanConcurrency nightly removal), steering/parked-ask follow-ups, nightly drift gate]

actuals:
  tokens: 2529
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Chain-exit steering spillover: the normal engine-chain Observe-exit path drains the session SteerQueue after chainExit and delivers leftovers as one plain sess.Prompt turn under the session turn mutex"

key-files:
  created:
    - .planning/quick/261013-uhx-diagnose-and-fix-the-testpermissionse2e-regression-in-intern/261013-uhx-SUMMARY.md
  modified:
    - kit/runtime/runtime.go
    - kit/runtime/steering_ingress_test.go
    - .github/workflows/nightly.yml

key-decisions:
  - "Root cause: the 23-02 routeSteering classifier treats chainCount>0 as steerable, but the count stays >0 through the chain's COMPLETION window (between the turn's final boundary drain and the Observe-exit chainExit); a prompt arriving there is enqueued + end_turn-acknowledged and then orphaned — no future boundary drain exists and the normal chain exit has no steering rider"
  - "Fix placed on the DELIVERY side, not the classifier: the pinned 23-02 ingress contract (TestSteerIngressEngineChain requires enqueue with chainCount>0 and no in-flight turn — state identical to the completion window) makes any classifier-side predicate a contract violation; the spillover restores the pre-40b2bbc observable behavior for exactly the orphan window"
  - "Spillover delivery rides plain sess.Prompt under the session turn mutex, NOT an r.Run re-entry: the classifier could see the just-returning client turn's turnActive flag and re-enqueue the very input being delivered (observed empirically with the first fix draft); plain sess.Prompt is the same delivery precedent TestSteerIngressEngineChain pins, keeps the permission gate in path, and queued steering text is never a resolvable command"
  - "Spill drain ordered AFTER chainExit and BEFORE the Observe-completion report: after chainExit so the delivery cannot re-classify as steering; before the report so a non-parked caller's deferred turn-exit rider (which resolves leftover steering cancelled-normal) cannot cancel the batch first"

patterns-established:
  - "Chain-exit steering spillover (kit/runtime spillOrphanedSteering): queued steering that outlives its delivering chain is re-driven as an ordinary turn rather than cancelled-normal at teardown"

requirements-completed: []
---

# Quick Batch 261013-uhx: TestPermissionsE2E -race regression — root cause, fix, nightly unskip — Summary

**One-liner:** The 40b2bbc steering classifier orphaned prompts enqueued during an engine chain's completion window (enqueued + end_turn-acknowledged, then never drained because the dying chain has no boundary left and the normal exit has no steering rider) — fixed with a chain-exit spillover delivery in kit/runtime, pinned deterministically, and the nightly -race gate now runs the permission surface again.

## Root Cause

**Indicted hunk:** 40b2bbc hunk (a) — the `routeSteering` pre-mutex ingress classifier at the `Runner.Run` head (now `kit/runtime/runtime.go:1073` after the Phase-25 extraction; `step 1` predicate `!clientTurnActive && chainCount==0 → ordinary` and `step 4` turn/chain-active → SteerQueue enqueue + immediate end_turn).

**Mechanism.** `permScenarioHappyPath` stage 1: prompt 1 ("write the file") classifies ordinary and the gated dialog round-trip completes exactly as designed — the permission ask suspends the turn, the chain PARKS (`chainEnter`, count 1), the allow_always answer resumes it, the rule persists, the tool executes, and the closing chunk ("done writing.") streams. The test then immediately sends prompt 2 ("write it again"). In the window between that final chunk reaching the wire and the Observe-completion goroutine's `chainExit`, `chainCount(sessionID)` is still 1 while `turnActive` is false (the client Run returned at the park; the resumed turn is driven by the ask-queue resume, which does not mark client turns) and `HasPendingAsk` is false. The classifier therefore takes the steering branch: prompt 2 is enqueued on the SteerQueue, acknowledged with an immediate end_turn, and a "steering queued — 1 pending" note is emitted. But the delivering premise of that branch — "the running turn continues and delivers the steering at its next model-request boundary" — is false: the turn's tool loop has already exited past its LAST boundary (`drainSteering` runs only at the tool-loop iteration head, `kit/session/session.go:665`), the chain is dying, and the NORMAL chain exit carries no steering rider (only the client-turn exit rider `unregisterActiveTurn` and the parked-chain CANCEL path `cancelParkedChains` resolve the queue). The input sits orphaned until test teardown, whose `cancelParkedChains` logs the documented signature: `parked-chain cancel resolved 1 queued steering input(s) (cancelled-normal)` (`kit/runtime/runtime.go:1585` region).

**The unmet await condition:** stage 1's final predicate `responded("e2e-p2") && s.hasChunk("done again.")` — `responded` flips true immediately (the enqueue path's end_turn response), but "done again." can never stream because no turn ever runs the second call; the simulator's bounded frame read then fires (`timed out waiting for a frame (seen=11)`, attributed to the `permScenarioHappyPath` call site).

**Why -race-only (and why the detector is silent).** Every piece of state involved (`turnActive` sync.Map, `activeChains` mutex-guarded map, the SteerQueue mutex) is properly synchronized — there is no data race, so the detector reports nothing. The defect is a LOGICAL race: two goroutines (the chain's Observe-completion teardown vs. the wire ingress) race the gap between the last boundary and `chainExit`, and the classifier's ordering assumption ("chainCount>0 implies a future boundary exists") is simply not enforced anywhere. The race detector's instrumentation slows the teardown side, widening the completion window so the enqueue branch wins essentially every -race run; machine load widens it too (see Deviations — at execution time the window also lost without -race on this host).

**Hunk isolation (scratch worktree at master ee4de04, each candidate neutralized alone, -race E2E rerun after each):**

| Candidate | Neutralization | Result |
|-----------|----------------|--------|
| (a) routeSteering classifier | force the ordinary path at the classifier head | **GREEN (exit 0) — the culprit** |
| (b) suspendForAsk parked-ask pair (AppendParkedAsk record) | drop the record append | still RED (exit 1, same signature) |
| (c) D-06 cancel grammar (Session.CancelPendingAsk resolution) | always claim-nothing | still RED (exit 1, same signature) |
| (d) swept Phase-22 cap wiring (backgroundCaps resolver) | force nil-resolver defaults | still RED (exit 1, same signature) |

**Leading hypothesis from the plan: REFUTED.** Stage 1's FIRST prompt (:634 await) was never the victim — instrumentation (`routeSteering` + `chainEnter`/`chainExit` logging) shows prompt 1 classifying ordinary (turnActive=false, chainCount=0) and the dialog round-trip completing; the enqueued input is the SECOND prompt in the completion window. The plan's rival (pre-Surface chunk publish reordering) is also excluded: the failure sits entirely after the dialog closed. The mechanism above is the third option the plan allowed for.

## The Fix

`kit/runtime/runtime.go` — `spillOrphanedSteering`, wired into the normal Observe-exit path of `runOneTurn`'s chain goroutine, placed AFTER `chainExit` (so the delivery cannot be re-classified as steering by the ingress) and BEFORE the completion report (so a non-parked caller's deferred turn-exit rider cannot cancel the batch first). It drains the queue and runs the batch as ONE plain `sess.Prompt` turn on its own goroutine under the serve-lifetime ctx and the session turn mutex.

Deliberate shape decisions:
- **Not an `r.Run` re-entry** — the first draft re-entered Run and the classifier re-enqueued the delivered input (the main Run's `turnActive` was still set during unwind; observed empirically, pin failing). Plain `sess.Prompt` under `sessionTurnMu` is the exact delivery precedent the 23-02 battery pins ("the chain's next turn — the bridge drives sess.Prompt — delivers it"), keeps the permission gate in path (`gateCall` lives inside the turn loop), and queued steering text is never a resolvable command (the classifier routes resolved commands to the ordinary path, never to the queue), so no ingress-side expansion is missed.
- **Preserved unconditionally:** the Phase-17 gate precedence, the 23-02 ingress contract (nothing-active ordinary path byte-identical; pending ask outranks — observed live in stage 3 where the answer "ship it" correctly outranks with chainCount=1; return-after-enqueue), and the D-05/D-06 parked-ask semantics. Forbidden shapes: none used (no sleeps, no timeout bumps, no skips, no assertion weakening).
- **Termination:** one spill consumes the whole drained batch in one turn; a further spill requires a NEW enqueued input, so the spill chain terminates. Serve teardown skips the spill (ctx dead → the cancelled-normal riders own the queue).

## TDD Gate Compliance (Task 2, tdd="true")

- **RED (pre-existing, captured at master before the fix):** `go test -race -count=1 ./internal/acpserve/ -run 'TestPermissionsE2E'` — exit 1, `--- FAIL: TestPermissionsE2E (11.07s)`, stage 1 frame-wait timeout (`timed out waiting for a frame (seen=11)`) with the teardown signature `parked-chain cancel resolved 1 queued steering input(s) (cancelled-normal)`. A real assertion/timeout failure (target_test_failed), not a compile error. Captured at `/tmp/uhx-red.log`.
- **PIN RED (fix hunk temporarily reverted):** `go test -race -count=1 ./kit/runtime/ -run 'TestSteerSpilloverOnChainExit'` — exit 1, `the completion-window steering was never delivered (no spillover turn): the input is orphaned in the queue` (5.26s, deterministic). Captured at `/tmp/uhx-pin-red.log`.
- **PIN GREEN (fix restored):** same command — exit 0 in 0.23s with the spillover delivery log line. Captured at `/tmp/uhx-pin-green.log`, `/tmp/uhx-pin-green2.log`, `/tmp/uhx-pin-green3.log`.
- **Commit-scope contract honored:** the pin landed as its own `test(quick-261013-uhx)` commit (deb0876, RED at that commit — the fix is not in it) BEFORE the `fix(quick-261013-uhx)` commit (efbf759). Note: the mechanical `gsd_run check tdd-red-evidence` record check parses node:test TAP summaries and cannot classify Go test output (fail-closed INVALID_RED for any Go run); RED evidence is documented here in the record's field shape (command, exit code, failing test, expected, actual) instead. The plan's Task 2 behavior block pre-authorized this exact RED shape ("pre-existing, captured by Task 1 at master ... target_test_failed, not a compile error").

## Verification Results

- `go test -race -count=1 ./internal/acpserve/ -run 'TestPermissionsE2E'` — PASS 5/5 consecutive runs with the fix (was 1/1 RED pre-fix), plus PASS without -race.
- Full package gates under -race: `./internal/acpserve/` minus the D-23-07-1 simulator pair (`TestZedSimulatorE2E`, `TestSimulatorCommandSurface` — environment-failing, stay nightly-skipped, out of scope) — PASS; `./kit/runtime/` minus `TestRescanConcurrency` (sibling item 261013-uhy's known race) — PASS; `./kit/session/` in full — PASS.
- `go vet ./internal/acpserve/ ./kit/runtime/ ./kit/session/` — clean.
- Nightly skip list: the -skip regex names exactly the five remaining documented entries (TestRescanConcurrency, TestRunSuite_, TestZedSimulatorE2E, TestSimulatorCommandSurface, TestEscalation_ReapAllUsesLadder); `TestRescanConcurrency` survives verbatim for sibling 261013-uhy; `ruby -ryaml -e 'YAML.load_file(...)'` parses; `git diff` shows only the regex line + reasons comment hunk.

## Deviations from Plan

1. **[Ground-truth drift, documented] The without-race control FAILED at execution time** (exit 1, byte-identical signature to the -race RED). The plan's verified ground truth (at baf13b8: passes without -race in ~1.5s) did not hold on this host at execution time — the completion-window race lost without -race too under current machine conditions. This does not change the diagnosis (the mechanism is a logical ordering race that -race's slowdown makes near-deterministic; load widens the same window) and does not change the fix (the orphaned delivery is repaired either way). Post-fix, both legs pass consistently.
2. **[Refuted hypothesis, per plan instruction] The plan's leading hypothesis** (stage-1's first prompt enqueued via the turn/chain-active branch tripped by the set_config_option persist) **was refuted by instrumentation** — prompt 1 classifies ordinary; the victim is the SECOND prompt in the chain-completion window. Documented under Root Cause; hunk isolation performed with all four candidates per the task's method.
3. **[Rule 3 - blocking] First fix draft re-orphaned the input** — an `r.Run`-based spillover let the classifier re-enqueue the delivered input while the main Run unwound (pin RED with the fix present). Re-derived the delivery to plain `sess.Prompt` under the session turn mutex per the 23-02 battery's own precedent; no plan text contradicted.

## TDD/AUTH Gates

None — no authentication gates encountered.

## Known Stubs

None — no stub patterns introduced; the fix is production delivery logic plus a real regression pin.

## Threat Flags

None — no new trust-boundary surface. T-uhx-01 honored (the gate is untouched: the spillover rides the same `sess.Prompt` gate path; stage 1's persist-before-execute and zero-reframe assertions keep passing); T-uhx-03 mitigated exactly as planned (one entry removed only after its failure was proven fixed under -race, with the YAML-parse and verbatim-survival gates).

## Self-Check: PASSED

- All modified files exist on disk (kit/runtime/runtime.go, kit/runtime/steering_ingress_test.go, .github/workflows/nightly.yml, this SUMMARY).
- All three commits exist in history: deb0876 (test pin), efbf759 (fix), 79e3eb2 (nightly unskip).
- Task 1's verify greps hold: RED log carries the parked-chain signature (/tmp/uhx-red.log), the SUMMARY carries the Root Cause section.
- Final gate suite re-run green on the committed tree: E2E under -race and without, acpserve/kit-runtime/kit-session batteries, go vet, nightly.yml entry + YAML checks.
