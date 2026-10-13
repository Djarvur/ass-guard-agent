---
phase: quick-261013-uhx
verified: 2026-10-13T21:32:43Z
status: passed
score: 5/5 must-haves verified
covered_files:
  - ".planning/quick/261013-uhx-diagnose-and-fix-the-testpermissionse2e-regression-in-intern/261013-uhx-PLAN.md"
  - ".planning/quick/261013-uhx-diagnose-and-fix-the-testpermissionse2e-regression-in-intern/261013-uhx-SUMMARY.md"
  - "kit/runtime/runtime.go"
  - "kit/runtime/steering_ingress_test.go"
  - ".github/workflows/nightly.yml"
covered_digest: "v1:sha256:2c692c795f909dd9181d91c0b2e627dfe9cbfa7eb77c8c8e92f7b305c00ede16"
behavior_unverified: 0
overrides_applied: 0
---

# Quick Batch 261013-uhx: TestPermissionsE2E -race regression — Verification Report

**Item Goal:** Diagnose and fix the TestPermissionsE2E regression in internal/acpserve introduced by Phase 23 commit 40b2bbc (steering ingress; passes at 721c7bc, fails at dbbb5d0). Reproduce at master, root-cause the offending hunk, fix production or the test per the root cause, then remove the TestPermissionsE2E entry from the nightly test skip list.
**Verified:** 2026-10-13T21:32:43Z (at master 40bd483 — uhx's commits deb0876, efbf759, 79e3eb2 all in history)
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

Every load-bearing claim was re-derived from the codebase and by running the tests myself — nothing accepted from SUMMARY narration. The three uhx commits touch exactly three files (`kit/runtime/steering_ingress_test.go`, `kit/runtime/runtime.go`, `.github/workflows/nightly.yml`); the production E2E test file was NOT touched (its only history since 40b2bbc is the Phase-25 kit move 1d0569d), ruling out assertion-weakening fix shapes.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `go test -race -count=1 ./internal/acpserve/ -run TestPermissionsE2E` passes at the plan head | ✓ VERIFIED | **Ran it myself:** PASS 5.08s, all five stages logged (degraded fallback, turn-death mid-dialog, ungated default). Previously RED at master with the documented signature. |
| 2 | The offending 40b2bbc hunk identified by file+site, with a mechanism explaining the -race-only sensitivity AND the teardown signature | ✓ VERIFIED | SUMMARY "Root Cause" names 40b2bbc hunk (a) — the `routeSteering` classifier (kit/runtime/runtime.go:1073, line matches the PLAN's cited site) — with the completion-window orphan mechanism, why the detector stays silent (all state synchronized; logical ordering race widened by -race slowdown), and the teardown signature (`parked-chain cancel resolved 1 queued steering input(s)`). **Corroborated behaviorally:** the pin's RED at pin-only commit deb0876 reproduces the exact orphaning deterministically (see Spot-Checks) — the mechanism is real, not narration. 40b2bbc confirmed as the Phase 23 steering commit touching internal/runtime/runtime.go. |
| 3 | Permission ask round-trip semantics intact: exactly one request_permission frame with four canonical option kinds, allow_always rule on disk before the gated call executes, zero further frames on the identical repeat call | ✓ VERIFIED | The E2E passed under -race with its assertions untouched (uhx commits did not modify permissions_e2e_test.go). Assertions confirmed present in the file: canonical options + "the rule lands on disk BEFORE the gated" (:609), "The second IDENTICAL call: allowed by the persisted rule — zero dialogs" (:677). |
| 4 | Nightly -skip regex names exactly the five remaining documented entries, no sixth; workflow parses as YAML | ✓ VERIFIED | **At uhx's own commit 79e3eb2** (verified from the diff): regex = `^TestRescanConcurrency$|^TestRunSuite_|^TestZedSimulatorE2E$|^TestSimulatorCommandSurface$|^TestEscalation_ReapAllUsesLadder$` — exactly five, TestPermissionsE2E gone from regex AND reasons comment, TestRescanConcurrency preserved verbatim for the sibling wave, and the diff touches only the comment block + regex line. **At current HEAD** the regex has four entries: sibling item 261013-uhy's own wave (01ba3a4, 40bd483) landed after uhx and removed TestRescanConcurrency as designed — expected sequencing, not a regression. The D-23-07-1 simulator pair (TestZedSimulatorE2E, TestSimulatorCommandSurface) remains excluded per the documented baseline. `ruby -ryaml` parses the file; test step runs the `-race` invocation. |
| 5 | Steering-ingress and parked-ask batteries stay green under -race: kit/runtime and kit/session | ✓ VERIFIED | **Ran both myself:** `./kit/runtime/` full package (42.2s — now includes TestRescanConcurrency, fixed by the sibling's 01ba3a4, so this is stronger than the plan's minus-one exclusion) and `./kit/session/` in full (6.8s) — both `ok` under `-race -count=1`. `go vet` clean on all three packages. |

**Score:** 5/5 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| Root-cause fix at the indicted hunk site | Production fix in kit/runtime/runtime.go | ✓ VERIFIED | `spillOrphanedSteering` (:1719) — drains the SteerQueue and delivers the batch as ONE plain `sess.Prompt` under the session turn mutex on its own goroutine; skips when the serve ctx is dead (teardown keeps the cancelled-normal riders). Wired at :1508 in the Observe-exit goroutine AFTER `chainExit` (:1497) and BEFORE the completion report (:1512) — exactly the ordering the SUMMARY claims (post-exit delivery cannot re-classify as steering; pre-report drain cannot be cancelled by the deferred turn-exit rider). Substantive, wired, not a stub. |
| Deterministic sub-second regression pin | New test in kit/runtime/steering_ingress_test.go | ✓ VERIFIED | `TestSteerSpilloverOnChainExit` (:697) — constructs the completion window deterministically (steer lands while request 1 is BLOCKED in flight, strictly after the boundary drain; scripted final response carries no tool calls so no boundary follows). Asserts enqueue, chain completion, second provider request carrying the steered text, and queue DRAINED (not cancelled-normal). **0.21s under -race.** RED/GREEN independently reproduced (see Spot-Checks). |
| nightly.yml with the fixed test's entry removed | Regex + reasons comment narrowed | ✓ VERIFIED | Commit 79e3eb2 diff verified: only the comment block + `-skip` regex line changed; TestPermissionsE2E clause dropped from both; REMOVE-as-failures-are-fixed instruction kept. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| routeSteering classifier (kit/runtime/runtime.go:1073) | prompt classification → SteerQueue enqueue vs ordinary path | Runner.Run head | ✓ WIRED | Symbol at the exact PLAN-cited line; the pin test exercises the enqueue branch live (Pending==1 asserted after the completion-window steer). |
| suspendForAsk (kit/session/session.go:1043) | AppendParkedAsk + ask.Surface → acpserve request_permission frame | session/manager chain | ✓ WIRED | suspendForAsk → AppendParkedAsk (manager.go:426) → ask.Surface (session.go:1073) all present; the E2E's passing stage 1 drives this chain end-to-end under -race. |
| nightly.yml -skip regex + reasons comment | the `go test -race -count=1 ./...` invocation | test step | ✓ WIRED | Run line present and parseable: `go test -race -count=1 -skip '…' ./...` — TestPermissionsE2E no longer excluded, so the nightly gate now runs the permission surface. |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| E2E regression fixed under -race | `go test -race -count=1 ./internal/acpserve/ -run 'TestPermissionsE2E'` | `--- PASS: TestPermissionsE2E (5.08s)`, 5/5 stages | ✓ PASS |
| Pin GREEN at HEAD (fix present) | `go test -race -count=1 ./kit/runtime/ -run 'TestSteerSpilloverOnChainExit$'` | PASS 0.21s with `steering spillover: delivering 1 orphaned input(s)` log | ✓ PASS |
| Pin RED at pin-only commit (fix absent) — TDD contract | scratch worktree at deb0876, same command | `--- FAIL: TestSteerSpilloverOnChainExit (5.24s): the completion-window steering was never delivered (no spillover turn): the input is orphaned in the queue` | ✓ PASS (RED reproduced — the pin genuinely detects the defect) |
| kit/runtime battery under -race | `go test -race -count=1 ./kit/runtime/` | ok 42.17s (full package, incl. TestRescanConcurrency) | ✓ PASS |
| kit/session battery under -race | `go test -race -count=1 ./kit/session/` | ok 6.80s | ✓ PASS |
| go vet on the three packages | `go vet ./internal/acpserve/ ./kit/runtime/ ./kit/session/` | clean | ✓ PASS |
| nightly.yml YAML parse | `ruby -ryaml -e 'YAML.load_file(...)'` | parses; build-test job + run line intact | ✓ PASS |

### Probe Execution

No conventional or phase-declared probes (`scripts/*/tests/probe-*.sh`); the plan's probe role is discharged by the go test spot-checks above. SKIPPED (no probe files in scope).

### Requirements Coverage

No requirement IDs in the PLAN frontmatter (`requirements-completed: []`) — quick-batch item; the PLAN must_haves are the contract and are fully covered above.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | — | — | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER in any modified file; no stub returns; no forbidden fix shapes. The `time.Sleep(2ms)` at steering_ingress_test.go:751 is a bounded poll (5s deadline) asserting the POSITIVE spillover-delivery behavior on another goroutine — its assertion demonstrably bites (RED reproduced at deb0876), so it is a legitimate results-wait, not a masking sleep. |

### Deviations Reviewed

All three SUMMARY-documented deviations were checked against the plan text and are legitimate: (1) the without-race ground-truth drift is documented and does not change the mechanism or fix (post-fix both legs pass); (2) the plan explicitly instructed "CONFIRM or REFUTE (do not assume)" for the leading hypothesis — the refutation via instrumentation is the instructed outcome; (3) the Rule-3 fix-draft correction (r.Run re-entry → plain sess.Prompt under the turn mutex) is visible in the shipped code and its rationale in the spill function's doc comment.

### Human Verification Required

None. Every must-have was verifiable programmatically or by running the tests directly, including the behavior-dependent legs (the E2E under -race and the pin's RED/GREEN pair were both executed in this verification).

### Gaps Summary

None. The item's full letter is discharged and independently reproduced: the 40b2bbc completion-window steering orphan is root-caused with a mechanism that survives adversarial checks (the pin's RED at the pin-only commit proves the defect is real and deterministic when unfixed), the production fix is placed exactly where the mechanism dictates, the 23-02 ingress contracts and Phase-17 gate semantics are intact (their batteries pass in full under -race, and the E2E test file is untouched), and the nightly skip list no longer names TestPermissionsE2E while the documented baseline entries (including the D-23-07-1 simulator pair) survive. HEAD's four-entry regex is the sibling item 261013-uhy's own committed wave, not a regression of this item.

---

_Verified: 2026-10-13T21:32:43Z_
_Verifier: Claude (gsd-verifier)_
