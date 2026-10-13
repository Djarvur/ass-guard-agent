---
phase: quick-261013-uhx
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/acpserve/permissions_e2e_test.go
  - kit/runtime/runtime.go
  - kit/runtime/steering_ingress_test.go
  - kit/session/session.go
  - kit/session/ask.go
  - kit/session/manager.go
  - kit/session/steering_test.go
  - .github/workflows/nightly.yml
autonomous: true
must_haves:
  truths:
    - "go test -race -count=1 ./internal/acpserve/ -run TestPermissionsE2E passes at the plan head (at master it fails in stage 1 with a frame-wait timeout while the same test passes without -race)"
    - "The offending 40b2bbc hunk is identified by file and code site, with a written mechanism that explains BOTH the -race-only sensitivity AND the teardown signature (the parked-chain cancel resolving exactly 1 queued steering input at kit/runtime/runtime.go:1585)"
    - "The permission ask round-trip semantics are intact: stage 1 still surfaces exactly one request_permission frame with the four canonical option kinds, the allow_always rule lands on disk before the gated call executes, and the identical repeat call runs with zero further permission frames"
    - "The nightly -skip regex names exactly the five remaining documented entries (TestRescanConcurrency, TestRunSuite_, TestZedSimulatorE2E, TestSimulatorCommandSurface, TestEscalation_ReapAllUsesLadder) with no sixth entry, and the workflow file parses as YAML"
    - "Steering-ingress and parked-ask batteries stay green under -race: kit/runtime (minus the sibling-known TestRescanConcurrency race, item 261013-uhy) and kit/session in full"
  artifacts:
    - "Root-cause fix at the diagnosed 40b2bbc hunk site — kit/session/session.go, kit/session/ask.go, kit/session/manager.go, kit/runtime/runtime.go, or internal/acpserve/permissions_e2e_test.go (exactly the site(s) the mechanism indicts; unused candidates stay untouched)"
    - "A deterministic sub-second regression pin for the root-cause ordering, added to the owning package's existing test file"
    - ".github/workflows/nightly.yml with the fixed test's entry removed from the -skip regex and from the reasons comment"
  key_links:
    - "routeSteering classifier (kit/runtime/runtime.go:1073, Runner.Run head) -> prompt classification -> SteerQueue enqueue vs ordinary path — the teardown signature indicts a prompt that was enqueued instead of run"
    - "suspendForAsk (kit/session/session.go:1043) -> AppendParkedAsk + parked-ask chunk publish -> ask.Surface -> the acpserve request_permission frame the E2E awaits"
    - "nightly.yml test step -skip regex (line 69) + reasons comment (lines 55-68) -> the go test -race ./... invocation this plan makes honest for the fixed test"
---

<objective>
Diagnose and fix the TestPermissionsE2E regression in internal/acpserve introduced by Phase 23 commit 40b2bbc (steering ingress; operator-bisected: passes at 721c7bc, fails at dbbb5d0, WINDOWS #22), then remove the fixed test's entry from the nightly test skip list (WINDOWS #27).

Purpose: the E2E is the phase-17 ask-surface story — the gated permission dialog, elicitation form/fallback, turn-death cascade, and the ungated default — and it currently cannot run under -race, so the nightly drift gate is permanently blind to the permission surface. The skip list exists to be emptied as failures are fixed; this item empties its entry the honest way.

Output: a documented root cause (which hunk of 40b2bbc, why only under -race), a production-or-test fix per that mechanism, a deterministic regression pin, and a narrowed nightly skip list.

Verified ground truth at master (baf13b8, 2026-10-13): the RED reproduces — under -race the test fails in stage 1 after ~10s (the frame-wait await at permissions_e2e_test.go:634 never completes; teardown logs the parked-chain cancel resolving 1 queued steering input, kit/runtime/runtime.go:1585); without -race it passes in ~1.5s. The -race detector fires NO report — this is a timing/ordering defect exposed by -race slowdown, not a mutex-missing data race.
</objective>

<execution_context>
@/home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/workflows/execute-plan.md
@/home/nil/DiskD/W/Djarvur/ass-guard-agent/.zcode/gsd-core/templates/summary.md
</execution_context>

<context>
@/home/nil/DiskD/W/Djarvur/ass-guard-agent/.planning/STATE.md (Blockers/Concerns — CROSS-WORKSTREAM entry: 40b2bbc regresses TestPermissionsE2E; also swept Phase-22 runtime cap-wiring hunks into the same commit)

Introduction commit hunks (operator bisected the COMMIT; the HUNK is this plan's job). 40b2bbc code now lives at kit/ paths after the Phase-25 extraction (moves were assertion-preserving; internal/runtime no longer exists):
- kit/runtime/runtime.go:1073 — routeSteering classifier at Runner.Run head ("nothing-active -> ordinary path byte-identical; pending ask outranks; turn/chain active -> SteerQueue enqueue + immediate end_turn return"); SetSteerQueue wiring around :2465; parked-chain cancel walk :1569-1585
- kit/session/session.go:1043 — suspendForAsk: 40b2bbc added AppendParkedAsk + an UNCONDITIONAL Bus.Publish of an "ask waiting behind running turn" AgentMessageChunk BEFORE ask.Surface (D-05 parked-ask visibility)
- kit/session/ask.go — CancelPendingAsk (D-06 parked-cancel grammar's session side); routeAskReply parses the cancel table before ResolveAsk
- kit/session/manager.go — AppendParkedAsk (TypeParkedAsk transcript record)
- Alternate suspect per the STATE CROSS-WORKSTREAM note: the Phase-22 runtime cap-wiring hunks swept into the same commit

The failing test: internal/acpserve/permissions_e2e_test.go — TestPermissionsE2E :585, five sequential scenarios; stage 1's first await (:634) requires responded("e2e-p1") AND exactly one permission frame. permAwait :361 owns the frame-wait timeout.

The skip list: .github/workflows/nightly.yml — the -skip regex at :69 and the reasons comment :55-68. NOTE the path correction: the item text and WINDOWS #27 say nightly-parity.yml; the real committed file is .github/workflows/nightly.yml (only workflow file in the repo — verified). Sibling item 261013-uhy removes a DIFFERENT entry (TestRescanConcurrency, now at kit/runtime/rescan_test.go) from this same file in its own wave — edit narrowly, touch only this test's entry.

Sibling batteries that must stay green: kit/runtime/steering_ingress_test.go (TestSteerIngressNoActiveTurnOrdinary :176, TestSteerIngressPendingAskOutranks :256, TestParkedAskCancelGrammar :364, TestParkedAskNoteEmitter :688), kit/session/steering_test.go (TestParkedAskRecord :630, TestSteeringDeliveryEndToEnd :146).

git show 40b2bbc — read the full diff before diagnosing; the commit message enumerates its own hunks.
</context>

<tasks>

<task type="auto">
  <name>Task 1: Root-cause the offending 40b2bbc hunk at master</name>
  <files>.planning/quick/261013-uhx-diagnose-and-fix-the-testpermissionse2e-regression-in-intern/261013-uhx-SUMMARY.md</files>
  <action>Reproduce the RED and isolate the culprit hunk. (1) Capture the RED under -race and the without-race control (commands in verify) into /tmp logs; if the RED does not reproduce on the first -race run, retry with -count=3 before concluding anything. (2) Work in a scratch worktree (git worktree add) so master stays clean. (3) Neutralize each 40b2bbc candidate hunk ONE at a time — revert the single hunk or stub its specific behavior — rerunning the -race RED after each: (a) the routeSteering classification at the Runner.Run head in kit/runtime/runtime.go (force the nothing-active ordinary path), (b) the suspendForAsk parked-ask visibility pair in kit/session/session.go:1043 (the AppendParkedAsk record + the unconditional pre-Surface chunk publish), (c) the routeAskReply parked-cancel grammar / CancelPendingAsk in kit/session/ask.go, (d) the swept Phase-22 cap-wiring hunks. The culprit is the single hunk whose neutralization turns the -race run green while the kit/runtime and kit/session steering batteries stay green. (4) Write the mechanism into the plan SUMMARY under a "Root Cause" heading: the indicted file + function + behavior, why the defect is -race-ordering-sensitive (the detector stays silent — say what loses a race or reorders), and why stage 1's await predicate never completes. Leading hypothesis to CONFIRM or REFUTE (do not assume): the teardown signature (exactly 1 queued steering input resolved at kit/runtime/runtime.go:1585) is the test's own stage-1 prompt — it was enqueued as steering instead of taking the ordinary path (the permSetMode set_config_option persist, D-07 persist-then-apply, may leave turn/chain active-state set or in flight when the prompt arrives, tripping the turn/chain-active branch of the classifier), so the turn ends immediately, responded flips true but the permission frame never exists, and the await times out. The rival hypothesis: the unconditional pre-Surface chunk publish reorders the dialog frame out of the wait window. Decide between them (or find a third) with hunk isolation, not plausibility. Name the awaiting predicate's exact unmet condition in the mechanism.</action>
  <verify>
    <automated>cd /home/nil/DiskD/W/Djarvur/ass-guard-agent && go test -race -count=1 ./internal/acpserve/ -run 'TestPermissionsE2E' > /tmp/uhx-red.log 2>&1; r1=$?; go test -count=1 ./internal/acpserve/ -run 'TestPermissionsE2E' > /tmp/uhx-ctl.log 2>&1; r2=$?; test "$r1" = 1 -a "$r2" = 0 && grep -q "parked-chain cancel resolved 1 queued steering input" /tmp/uhx-red.log && grep -qi "root cause" .planning/quick/261013-uhx-diagnose-and-fix-the-testpermissionse2e-regression-in-intern/261013-uhx-SUMMARY.md</automated>
  </verify>
  <done>The RED fails under -race with the documented signature (stage-1 frame-wait timeout; teardown resolves 1 queued steering input), the without-race control passes, and the SUMMARY names the indicted hunk (file + function + behavior) with a mechanism covering the -race-only sensitivity, the queued-steering-input teardown signature, and the unmet await condition.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Fix production (or the test) per the root cause and pin the ordering</name>
  <files>kit/session/session.go, kit/session/ask.go, kit/session/manager.go, kit/runtime/runtime.go, internal/acpserve/permissions_e2e_test.go, kit/runtime/steering_ingress_test.go, kit/session/steering_test.go</files>
  <behavior>
    - RED (pre-existing, captured by Task 1 at master): TestPermissionsE2E under -race fails in stage 1 — a real assertion/timeout failure (target_test_failed), not a compile error.
    - GREEN 1: under -race, stage 1 completes the dialog round-trip — exactly one request_permission frame carrying the four canonical option kinds in order (allow_once, allow_always, reject_once, reject_always), the allow_always rule persisted to disk before the gated call executes, and the identical repeat call runs with zero further permission frames; stages 2-5 (elicitation capable, -32601 degraded fallback, turn-death mid-dialog with zero orphans, ungated default with zero permission frames) all pass.
    - GREEN 2 (ordinary-path property): a prompt arriving while nothing is active takes the ordinary path and is never enqueued as steering — TestSteerIngressNoActiveTurnOrdinary stays green.
    - GREEN 3 (23-02 semantics survive): parked-ask visibility (D-05 record + note) and the parked-cancel grammar (D-06) keep their batteries green — TestParkedAskRecord, TestParkedAskNoteEmitter, TestParkedAskCancelGrammar, TestSteerIngressPendingAskOutranks.
    - PIN: one new deterministic sub-second test row/battery in the owning package's existing test file pins the root-cause ordering (e.g. classifier state after a set_config_option persist, or frame order around the pre-Surface publish — per Task 1's mechanism). Demonstrate its RED by temporarily reverting the fix hunk (pin fails), then restoring (pin passes) — capture both runs.
  </behavior>
  <action>Implement the minimal fix at Task 1's indicted site. Fix PRODUCTION code unless the mechanism proves the TEST wrong — a test-side fix is authorized only when the mechanism shows the test's expectation contradicts the 23-02 design intent (D-05/D-06 as summed in the 40b2bbc commit message), never merely because changing the test is easier. Preserve unconditionally: the Phase-17 gate precedence (hook verdict -> permission ask -> execute, one gate at gateCall), the 23-02 ingress contract (nothing-active -> ordinary path byte-identical; a pending ask outranks steering; return-after-enqueue), and the D-05/D-06 parked-ask visibility and cancel semantics — fix the ordering/classification defect, do not delete the feature to stop the flake. Forbidden fix shapes: sleeps or timeout bumps in the test, skip markers, widening any -skip list, weakening the dialog assertions. Record the fix and the pin's RED/GREEN evidence in the SUMMARY.</action>
  <verify>
    <automated>cd /home/nil/DiskD/W/Djarvur/ass-guard-agent && go test -race -count=1 ./internal/acpserve/ -run 'TestPermissionsE2E' > /tmp/uhx-fix.log 2>&1; r1=$?; go test -race -count=1 ./internal/acpserve/ -skip '^TestZedSimulatorE2E$|^TestSimulatorCommandSurface$' > /tmp/uhx-pkg.log 2>&1; r2=$?; go test -race -count=1 ./kit/runtime/ -skip '^TestRescanConcurrency$' > /tmp/uhx-kr.log 2>&1; r3=$?; go test -race -count=1 ./kit/session/ > /tmp/uhx-ks.log 2>&1; r4=$?; go vet ./internal/acpserve/ ./kit/runtime/ ./kit/session/ > /tmp/uhx-vet.log 2>&1; r5=$?; test "$r1" = 0 -a "$r2" = 0 -a "$r3" = 0 -a "$r4" = 0 -a "$r5" = 0</automated>
  </verify>
  <done>The captured RED from Task 1 now passes under -race; the full acpserve package (minus the D-23-07-1 environment-failing simulator pair, which stays nightly-skipped and out of scope), kit/runtime (minus the sibling item's TestRescanConcurrency race), and kit/session are green under -race; go vet is clean on all three packages; the new pin's RED/GREEN evidence is captured in the SUMMARY.</done>
</task>

<task type="auto">
  <name>Task 3: Remove the fixed test's entry from the nightly skip list</name>
  <files>.github/workflows/nightly.yml</files>
  <action>Narrow edit to .github/workflows/nightly.yml (the real file — nightly-parity.yml in the item text and WINDOWS #27 is a stale path; do not create or hunt for it). Two touch points, nothing else: (1) in the test step's -skip regex (line 69), remove the ^TestPermissionsE2E$ alternation token — the regex then begins with the TestRescanConcurrency token; (2) in the reasons comment above it (lines 55-68), drop the TestPermissionsE2E clause ("D-23-07-1's -race-only leg; Phase 23 commit 40b2bbc regression") and reflow the sentence so the remaining entries' reasons read coherently, keeping the trailing REMOVE-entries-as-failed-are-fixed instruction intact. Leave the other five entries (TestRescanConcurrency, TestRunSuite_, TestZedSimulatorE2E, TestSimulatorCommandSurface, TestEscalation_ReapAllUsesLadder) and every other line byte-identical — sibling item 261013-uhy owns the TestRescanConcurrency removal in its own wave, and the two simulator entries are the D-23-07-1 pair this item explicitly does not cover. Do not touch .planning/WINDOWS.md (sibling item 261013-uhz owns the ledger).</action>
  <verify>
    <automated>cd /home/nil/DiskD/W/Djarvur/ass-guard-agent && grep -c 'TestRescanConcurrency' .github/workflows/nightly.yml | grep -qx '[12]' && grep -q 'TestRunSuite_' .github/workflows/nightly.yml && grep -q 'TestZedSimulatorE2E' .github/workflows/nightly.yml && grep -q 'TestSimulatorCommandSurface' .github/workflows/nightly.yml && grep -q 'TestEscalation_ReapAllUsesLadder' .github/workflows/nightly.yml && ! grep -q 'TestPermissionsE2E' .github/workflows/nightly.yml && ruby -ryaml -e 'YAML.load_file(".github/workflows/nightly.yml")'</automated>
  </verify>
  <done>The -skip regex in the test step names exactly the five remaining documented entries and the reasons comment no longer cites the fixed test; the other five entries survive verbatim for sibling waves; the workflow file parses as YAML (ruby YAML.load_file); nothing outside the regex line and the comment block changed (git diff of the file shows only that hunk).</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| model -> ask surface | Model-authored question content flows through the permission dialog and the D-05 parked-ask summary (display-only, truncated at 160 chars by askSummaryOf) |
| nightly workflow -> test invocation | The -skip regex selects which tests gate the hosted nightly; entries removed without fixes would silently ungate known-broken tests |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-uhx-01 | Tampering | the Task 2 fix site (permission ask surface / steering classifier) | medium | mitigate | The fix must not weaken the gate: stage 1's four-option dialog round-trip, persist-before-execute, and zero-reframe repeat-call assertions must keep passing (Task 2 GREEN 1); the Phase-17 precedence (hook verdict -> permission ask -> execute) and the 23-02 ingress contract are named untouchable in the task action; forbidden fix shapes (sleeps, skips, assertion-weakening) are enumerated |
| T-uhx-02 | Information Disclosure | D-05 parked-ask summary chunk (model-authored question text on the live chunk family) | low | accept | Pre-existing 23-02 design: display-only, truncated to 160 chars; revisited only if Task 1's mechanism indicts it — the fix may reorder it but not widen its exposure |
| T-uhx-03 | Denial of Service | nightly.yml -skip regex edit | low | mitigate | Task 3 removes exactly one entry whose underlying failure Task 2 proved fixed under -race; the YAML-parse verify gate and the verbatim-survival gate on the five remaining entries prevent a sloppy edit from ungating known-broken tests or breaking the workflow |
| T-uhx-SC | Tampering | package installs | none | accept | No package-manager installs in this item's scope — no supply-chain surface |
</threat_model>

<verification>
- RED/GREEN pair: `go test -race -count=1 ./internal/acpserve/ -run 'TestPermissionsE2E'` exits 1 at master (Task 1, with the documented signature) and 0 after Task 2; the without-race control exits 0 throughout.
- Package gates under -race: internal/acpserve minus the D-23-07-1 simulator pair green; kit/runtime minus TestRescanConcurrency (sibling 261013-uhy's item) green; kit/session green in full.
- `go vet ./internal/acpserve/ ./kit/runtime/ ./kit/session/` clean.
- Skip list: the nightly -skip regex names exactly the five remaining entries; the workflow parses as YAML; git diff shows only the regex line + comment block changed.
- The SUMMARY records: indicted hunk (file + function + behavior), the mechanism, the fix, and the pin's RED/GREEN evidence.
</verification>

<success_criteria>
The item's full letter is discharged: the regression is reproduced at master under -race, root-caused to a specific 40b2bbc hunk with a written mechanism, fixed in production or in the test per that mechanism (gate semantics intact, steering/parked-ask features survive), the fix is pinned by a deterministic regression test, and the TestPermissionsE2E entry is gone from the nightly skip list so the hosted drift gate runs the permission surface again (WINDOWS #22/#27 pressure released for this test).
</success_criteria>

<output>
Create `.planning/quick/261013-uhx-diagnose-and-fix-the-testpermissionse2e-regression-in-intern/261013-uhx-SUMMARY.md` when done
</output>
