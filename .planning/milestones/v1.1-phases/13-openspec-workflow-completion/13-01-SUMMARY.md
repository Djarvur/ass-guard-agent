---
phase: 13-openspec-workflow-completion
plan: "01"
subsystem: expanded-matrix-e2e
tags: [opsx-matrix, e2e, eval-gate, profile-guard, d01-matrix]
requires:
  - "13-00 (the engine-visible ask resume — the gate's precondition)"
  - "12-08 eval net (the flagship scenario + gate)"
provides:
  - "mise eval-gate flagship GREEN (the amended opener — ACP-08's deferred exit evidence, re-verified at the 13-00 tree)"
  - "evalharness.GuardOpenSpecGlobalConfig + ExpandedMatrixProfileJSON (the shared bootstrap primitive)"
  - "newOpsxMatrixRunner + the 12 gated matrix legs (6 happy + 6 fixable — D-01's full 2-path matrix)"
  - "cmd/ass-guard/testdata/opsx-e2e-matrix/ — the D-03 pass-1 + fixable capture corpus (12 files + README)"
  - "six [command_mutability] rows for the expanded commands"
affects:
  - "13-02 (harvests the captures into chaining seeds)"
  - "13-03 (reads the fixable corpus for the dead-end scan)"
  - "13-04 (consumes the guard for the eval bootstrap)"
tech-stack:
  added: []
  patterns:
    - "guard-before-init bootstrap (LIFO cleanup ordering for measured restore)"
    - "probed deterministic fixable triggers (probe → record → never soften)"
key-files:
  created:
    - internal/evalharness/profile_guard_test.go
    - cmd/ass-guard/e2e_opsx_matrix_test.go
    - cmd/ass-guard/testdata/opsx-e2e-matrix/
  modified:
    - internal/evalharness/harness.go
    - internal/openspec/seeded.toml
decisions:
  - "The parked draft adopted as-is (reviewed against the spec; committed in TDD order)"
  "verify-fixable CAPTURE-RESCOPED: the command is report-driven (no CLI validate step) — the leg asserts the structural gap is found + the verdict delivered"
  - "The wrong-name candidates produce ASKs (do-not-guess is mandated by the command files) — recorded as the D-02 class, not forced into fixable shapes"
metrics:
  duration: 330min
  tasks: 4
  commits: 9
status: complete
actuals:
  tokens: 560000   # chars/4 over the realized 13-01 diff
  tasks: 4
  commits: 9
---

# Phase 13 Plan 01: Flagship re-tuning opener + expanded-profile matrix E2E Summary

**One-liner:** The eval-gate flagship is GREEN (the amended opener, re-verified on the 13-00 tree)
and all six expanded commands drive real E2E through ass-guard against the real binary + model on
BOTH happy and fixable paths (12 gated legs, D-01 complete), with the profile-guard primitive
proving the operator's global config byte-identical after every run.

## Task results

| Task | Commits | Result |
|------|---------|--------|
| 1 (opener) | prior 8c56b29/d06d9f7 + this plan's re-run | **mise eval-gate GREEN**: opsx-flagship pass@1 (755.2s; artifact `.ass-guard/eval/eval-20260820-163220-k1.json`, archived `/tmp/eval-net-evidence/13-01-task1/`). The re-tuning + in-repo evidence landed before the stop; the re-run at the 13-00-fixed tree discharges the amended opener. |
| 2 (tracer) | a6b1fa6 → ad090b0 → 6ffccb4 → 82ce4ca | The parked draft reviewed + committed in TDD order (guard battery RED a6b1fa6 → implementation ad090b0; offline battery green). newOpsxMatrixRunner (guard-before-init + LIFO pre/post byte-compare + FAIL-LOUD on missing commands/skills) + the tracer leg green (70.7s, capture committed, CONFIG_UNTOUCHED). Tracer verify re-run end-to-end (auto mode). 6 mutability rows. |
| 3 (5 happy legs) | b591b60 | continue (proposal lands), ff (CAPTURE-WINS: ff archives in one pass — tasks.md asserted live-or-archive; first run's assertion missed the move, fixed + re-run green), verify (verdict), bulk-archive (both changes archived), onboard (D-08 full-tilt, asks ride 13-00's resume). 6 happy captures committed. |
| 4 (6 fixable legs) | 5dd912c | Probed deterministic triggers: new=already-exists; continue/ff=bogus sidecar schema; verify=scenario-less delta (CAPTURE-RESCOPED, see divergences); bulk-archive=incomplete-tasks batch; onboard=D-08 idempotent re-run. 6 fixable captures committed. |

## The gate + evidence

- `mise run ci` green at every task close.
- `mise run eval-gate` GREEN (Task 1's exit + 13-00's same gate).
- The operator's `~/.config/openspec/config.json` byte-identical after EVERY gated run
  (in-runner LIFO byte-compare + the verify command's pre/post shasum — CONFIG_UNTOUCHED
  printed on every leg batch).
- Run logs preserved under `/tmp/opsx-matrix-evidence/` (fixable runs 1-3 + the re-scoped verify).

## Deviations from Plan

### Rescoped items (capture-wins, recorded per the plan's own divergence rule)

**1. verify's fixable trigger re-scoped to the report contract.**
The planner's candidate ("specs fail validation") assumed a CLI validate step; the command's real
flow (its .claude/commands body) never runs `openspec validate` — it is a model-authored report
over status + instructions + artifact reads. The deterministic fixable shape is the STRUCTURAL gap
(a scenario-less delta cannot show scenario coverage) surfacing as a Correctness finding; the leg
asserts the gap is found + the verdict delivered. Probed-but-unused: `openspec validate --type
change` fails deterministically ("must have at least one delta... #### Scenario:") — recorded in
the leg comment.

**2. The wrong-name candidates (continue/ff) produce ASKs, not failures.**
Run-1 captures show the model ask-stops (or politely redirects) per the command files' own
MANDATE ("Do NOT guess or auto-select a change. Always let the user choose") — the D-02 dead-end
class 13-03's advisory handles. The bogus-sidecar-schema trigger keeps fail→adapt→goal on the
NAMED change ("Error: Unknown schema 'bogus-schema'. Available: spec-driven").

### Auto-fixed issues

**3. [Rule 1] ff's happy assertion missed the archive move** — first run failed, the capture
showed ff completes AND archives in one pass; assertion made archive-aware, re-run green.

**4. [Rule 3] lint cascade on the new test file** — consts/nolint/wsl conformance; zero behavior
change.

### Operational incident (recorded in STATE.md Concerns)

**5. The global openspec config's telemetry anonymousId was rotated by a manual probe** — the
executor's layout probe overwrote the config without a bytes copy (hash-only). Functional state
restored immediately (core default, notice suppressed); the original anonymousId is unrecoverable
in-session (APFS snapshot mount needs sudo). Recovery command recorded in STATE.md. All subsequent
probes went through the guard / bytes-copying snapshots.

## D-06 notes

- The FF happy leg's re-run followed a SOURCE delta (the assertion fix) — a fix-then-green, not a
  flake re-run.
- The verify fixable leg's final run followed the capture-rescope (source delta).
- The fixable runs 1-3 iteration logs are preserved; no best-of-N at zero delta anywhere.

## Known Stubs

None.

## Threat Flags

None beyond the plan's register (T-13-01-01 mitigated by the measured double byte-compare;
T-13-01-02 accepted as planned).
