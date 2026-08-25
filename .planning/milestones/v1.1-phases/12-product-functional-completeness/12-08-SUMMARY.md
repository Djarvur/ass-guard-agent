---
phase: 12-product-functional-completeness
plan: "08"
subsystem: eval
tags: [acp-08, d-03, eval-net, evalharness, evalsuite, pass-at-k, change-class-detector, first-green-blocked]

requires:
  - phase: 12-product-functional-completeness
    provides: "the settled waves-1..4 behavior (12-01..12-07) the net's first runs certify"
provides:
  - "internal/evalharness: the Phase-8 gated-E2E machinery extracted importable (scratch bootstrap with the real binary, zero-continue assertions, capture mode, fixable-recovery scan, the double env gate) — ONE implementation for the runtime E2E tests AND the suite"
  - "internal/evalsuite: the scenario runner — JSON scenario schema with harness-owned assertion keys (a scenario cannot weaken the gate, T-12-08-01), LoadScenarios with unknown-key rejection, the embedded scenarios/ extension point (Phase 13 adds a scenario = adds a JSON file), k passes over fresh scratches, pass@k + pass@1 scoring, JSON artifacts under .ass-guard/eval/, the loud ASSGUARD_EVAL_GATE skip"
  - "The D-03 gate surface: mise eval-gate (k=1, env-flag triple, 15m ceiling) / eval-deep (k=3 manual) / eval-check-changed — mise ci gains NO eval dependency"
  - "scripts/eval-change-class.sh: the locked change-class detector (exit-3 protocol, MERGE_BASE/@{push}/HEAD~1 base, CI hook documented, --selftest)"
  - "The offline battery: pass@k arithmetic (incl. the deliberate 2/3), schema rejection, loud-skip naming the flags, artifact emission, the deterministic-layer completeness enumeration"
affects: [Phase 13 (OS-03 extends the suites by adding scenario files), every future profile/model/turn-behavior change (the detector + gate)]

actuals:
  tokens: 46000   # chars/4 over the plan's production commits (estimate was 72000)
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "RunnerSeam injection: the turn-runner construction is package-main machinery; the harness takes it as an interface — extraction without relocation (the runtime tests keep their names + gates as the extraction's regression proof)"
    - "Assertions are named KEYS resolved by the harness, never free-form scripts in the scenario JSON (T-12-08-01); the scenario schema rejects unknown keys outright"
    - "pass@k = all-k-pass (the gate's determinism bar); pass@1 = first pass alone — the 2/3 case distinguishes them (pinned offline)"
    - "The D-01 askTimeout=45s on E2E runners: the first live finding's fix configuration — with asks REAL since 12-01, a mid-chain ask suspends the chain; D-01's bounded timeout returns the non-answer and the model proceeds (hands-off mode, documented — not an assertion weakening)"

key-files:
  created:
    - internal/evalharness/harness.go
    - internal/evalsuite/suite.go
    - internal/evalsuite/suite_test.go
    - internal/evalsuite/scenarios/opsx-flagship.json
    - cmd/ass-guard/evalsuite_bridge_test.go
    - scripts/eval-change-class.sh
  modified:
    - cmd/ass-guard/e2e_opsx_test.go (re-pointed at the harness; newOpsxRunnerAt split; askTimeout)
    - .mise.toml (eval-gate/eval-deep/eval-check-changed; ci untouched)

key-decisions:
  - "The gated flagship suite test lives in cmd/ass-guard (TestEvalSuite_Flagship_Gated) — the runner construction cannot move below package main without a large refactor; the bridge is the designed injection point (the suite package owns schema/scoring/artifacts and its offline battery runs in ci)" — documented deviation from the plan's internal/evalsuite-only verify shape
  - "The first gated runs are a REAL FINDING, not a gate failure to paper over: three live runs, three chain shapes (stall after explore / archive-stage ask suspension / apply self-injection x8 stopped by the budget cap) — the pattern table's stage-transition signals have drifted against the current model's phrasing. Assertions untouched (Pitfall 18); the first GREEN is blocked and recorded (WINDOWS, STATE blockers) with the fix routed to capture-informed pattern re-tuning"
  - "Evidence preserved: /tmp/eval-net-evidence/12-08-first-runs/ (both suite artifacts + the loop-run transcript + the README diagnosis)"

metrics:
  duration: ~150min
  completed: 2026-08-20

status: complete
---

# Phase 12 Plan 08: The behavioral-eval regression net Summary

**One-liner:** The three-layer eval net exists end-to-end — extracted harness, scenario suite with pass@k scoring and artifacts, the D-03 mise gate surface + change-class detector — and its first three live runs did exactly what a regression net is for: they caught that the flagship chain's pattern table has drifted against the current model (first GREEN blocked, evidence preserved, assertions untouched).

## What Was Built

### Task 1 — the extracted harness + the flagship suite (tracer)

- **internal/evalharness** (moved, not rewritten): scratch bootstrap with the real openspec binary + seeded codebase, `AssertZeroContinue` (the full zero-continue product-proof assertion set), capture mode, `ScanFixableRecovery`, the double env gate with the loud skip. The two runtime gated tests (TestOpsxEndToEnd_Gated / TestOpsxFixableRecovery_Gated) keep their names + gates and now run THROUGH the package — the extraction's regression proof (both loud-skip correctly when ungated; ran live during triage below).
- **internal/evalsuite**: the Scenario schema (assertions are harness-owned named keys), LoadScenarios (unknown keys + missing fields + unknown assertion keys all rejected), the embedded `scenarios/` (Phase-13 extension: add a JSON file), `RunSuite` (k passes, pass@k all-k + pass@1, JSON artifact per run under `.ass-guard/eval/`), the loud `ASSGUARD_EVAL_GATE=1` skip naming every missing flag.
- **opsx-flagship.json**: the Phase-8-proven 4-stage flow.
- **The bridge** (`TestEvalSuite_Flagship_Gated` in cmd/ass-guard): the suite driving the real runner over harness scratches — where the gate's green is proven.

### Task 2 — the gate surface + the offline battery

- mise `eval-gate` (k=1, ASSGUARD_EVAL_GATE=1 + the standing binary/model gates, 15m ceiling), `eval-deep` (k=3, manual), `eval-check-changed`; **mise ci untouched** (D-03 verbatim).
- `scripts/eval-change-class.sh`: the locked classes (profile / model / turn-behavior) with the exit-3 protocol, base-ref resolution (MERGE_BASE > @{push} > HEAD~1), the CI hook documented in the header, `--selftest` green (executable asserted).
- The offline battery: k semantics (1/1, 3/3, 2/3), schema rejection, loud skip, artifact emission, the deterministic-layer completeness enumeration (every phase-12 executor battery + sched present).

## THE FIRST RUNS — the net's opening catches (the real finding)

The gate ran LIVE (the operator's repo config carries credentialed endpoints — no env key needed). Three runs, three chain shapes — **the flagship's pattern table has drifted against the current model's phrasing**:

1. **Run 1** (suite, 78.9s): chain stalled after explore (decisions=[nothing]). Artifact: the first ever emitted, `eval-20260820-130906-k1.json`.
2. **Run 2** (suite, 78s): the ARCHIVE stage suspended on a REAL AskUserQuestion (`ask_suspended`) — asks became real in 12-01; the Phase-8 proof predates that (the ask tool was then a no-implementation stub, so the model's questions never suspended anything). Fixed-by-configuration: `askTimeout=45s` on the E2E runners (D-01's documented hands-off mode — the bounded timeout returns the capture-shaped non-answer and the model proceeds; NOT an assertion change).
3. **Run 3** (runtime proof, 323.8s, with the timeout): explore→propose→apply chained, then the apply turn's ending re-matched the →apply injection EIGHT times (budget cap stopped it: "re-fire budget cap reached"). The apply→archive transition never fired.

**Safety pins held everywhere**: the re-fire budget cap, the no-chain-suspension pin, full audit trails. **Assertions untouched (Pitfall 18)** — the first GREEN is blocked and recorded (WINDOWS unrun-verify + deviation entries; STATE Blockers/Concerns) with the fix routed to capture-informed pattern re-tuning (the 08-06 seeded stage-transition patterns vs the current model's output — an operator-scoped decision, likely a small Phase-12 residual or the Phase-13 opener). Evidence: `/tmp/eval-net-evidence/12-08-first-runs/` (both artifacts, the loop-run transcript, the README diagnosis).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] offline suite seam self-deadlock**
- **Found during:** Task 2 — a package-global bootstrap seam + per-test mutex deadlocked parallel tests.
- **Fix:** refactored to explicit `SuiteOpts{Bootstrap}` parameter injection (no globals, no locks).
- **Commits:** cb5febe/176d16f

**2. [Rule 3 - Blocking] the plan's verify targets ./internal/evalsuite for the gated run**
- **Issue:** the runner construction is package-main; a suite-package gated test cannot build the real driver.
- **Fix:** the bridge test in cmd/ass-guard (the designed injection point); the suite's own package carries the offline battery (in ci). The plan's `internal/runtime` path also does not exist (same 12-07 finding — grep located the real site).
- **Commits:** cb5febe

## Verification Evidence

- `mise ci` green (exit 0 — the full race battery + whole-repo lint; log at /tmp/eval-net-evidence/12-08-mise-ci.log). ci's dependency set UNCHANGED (no eval task joined).
- go test ./internal/evalsuite/ ./internal/evalharness/ green (offline batteries); the gated legs loud-skip with the missing flag names.
- `./scripts/eval-change-class.sh --selftest` green; the detector executable.
- The gated live runs executed (three runs, artifacts + transcripts preserved — the evidence trail above).

## Threat-Model Mitigations Landed

- **T-12-08-01 (scenario tampering):** harness-owned assertion keys + schema unknown-key rejection — a scenario cannot add or relax assertions.
- **T-12-08-02 (missing gate run):** loud skips naming every missing flag; the artifact carries per-pass evidence; the detector's exit-3 protocol + CI hook make "should have run" machine-checkable.
- **T-12-08-03 (eval cost):** change-class-only gating, k=1 in the gate, 15m ceiling; ci untouched.
- **T-12-08-04 (artifact disclosure):** artifacts under the gitignored `.ass-guard/eval/` family.
- **T-12-08-05 (detector bypass):** the class list locked verbatim in the script header; the selftest pins match behavior.

## Self-Check: PASSED

- internal/evalharness/harness.go, internal/evalsuite/suite.go (+ test + scenario), cmd/ass-guard/evalsuite_bridge_test.go, scripts/eval-change-class.sh — FOUND
- Commits cb5febe / 176d16f — FOUND in git log
