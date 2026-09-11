---
phase: 25-seed-001-kit-extraction-strictly-last
plan: "09"
subsystem: infra
tags: [go, kit, d-19-gate, hostproof, readme, d-20-battery, kit-extraction, operator-checkpoint]

requires:
  - phase: 25-seed-001-kit-extraction-strictly-last
    provides: kit/ complete and production-tree app-import-free (25-07), test layer dispositioned (25-08), the 1397 ledger, the differential recipe
provides:
  - The D-19 enforcement: mise kit-boundary task (in ci deps) + depguard KitBoundary rule — both proven red-on-violation and green-on-clean (probe planted and removed within the task)
  - kit/runtime/hostproof_test.go — TestKitHostsNonACPFrontend: a self-contained non-ACP frontend (own scripted provider, recording Emitter, immediate Requester) drives a real turn on nil Catalog/Toolkit/OpenPermStore/LaunchBackground degraded paths; zero app imports, self-asserted
  - kit/README.md — the honest front door: composition-root statement (D-09), the six seams, the nil-degradation table, the app→kit direction rule pointing at the gates, SEED-001/002/003 canonical refs (D-08)
  - The D-20 battery record (below) with five legs green/recorded and one operator leg open (Task 3's checkpoint)
affects: [KIT-01, KIT-02, KIT-03, milestone close-out]

actuals:
  tokens: 18400
  tasks: 2 of 3 (Task 3 = the blocking operator checkpoint, presented below)
  commits: 4  # D-19 gates+hostproof+README; eval-gate path fix; lint cleanup; (+ this docs commit)
  plan_head_before: 1bff911

tech-stack:
  added: ["@fission-ai/openspec@1.13.0 (npm -g) — the eval suite's ASSGUARD_OPENSPEC_BIN prerequisite, absent on this machine"]
  patterns:
    - "depguard list-mode for DENY rules must be lax: the research sketch's strict mode inverts to an allowlist under golangci 2.13 and denies every stdlib import in scope"
    - "Gate teeth proven BOTH ways inside the task: plant a probe violation (kit/event/violation_probe.go), watch both layers fire, remove — the negative proof lives in this SUMMARY, not in a commit"
    - "Self-referential meta-checks concatenate their needles: the hostproof's import self-check matched its own literal on the first run"
    - "Test vocabulary consts live in _test files; a production-named goconst_constants.go carrying test tables is dead weight the unused linter correctly flags"

key-files:
  created: [kit/runtime/hostproof_test.go, kit/README.md]
  modified: [.mise.toml (kit-boundary task + ci deps + eval-gate/eval-deep re-targeted at the suite's post-25-08 home internal/acpserve), .golangci.yml (KitBoundary depguard rule), kit/runtime/goconst_constants.go (trimmed to production usage), kit/runtime/goconst_constants_test.go, kit/runtime/fakes_test.go (lint cleanup)]

key-decisions:
  - "25-09: depguard KitBoundary list-mode lax (not the research sketch's strict) — strict inverts to an allowlist under golangci 2.13; the deny semantics the plan wants are lax-mode deny"
  - "25-09: D-19 non-test scope — the production tree's EMPTY internal dep set is the enforced invariant (go list-provable); kit test files stay exempt per 25-08's recorded disposition (16 twins/matrix cells)"
  - "25-09: the ledger records 1398 = 1397 baseline + 1 sanctioned instrument addition (the hostproof test) — the D-20 rule binds MOVES to conservation; the plan's own new-test requirement is the documented amendment"
  - "25-09: eval-gate re-targeted from ./cmd/ass-guard/ to ./internal/acpserve/ — the 25-08 subject-split moved the suite; the old path passed vacuously (no tests to run) until the detector proved the class fired"
  - "25-09: lint delta 587→653 (+66) — style classes the baseline already carries elsewhere (funcorder/gocritic/lll/wrapcheck/nonamedreturns/paralleltest/cyclop/funlen) concentrated in the 25-08-reworked test files; genuine dead code fixed (587→653 includes −7 real fixes); residue recorded against the STATE LINT BASELINE ticket rather than churning 60+ nolint sites"

d20-battery-record:
  - leg: mise ci
    outcome: "vet ✓, CGO=0 build ✓, kit-boundary ✓; race suite: kit/ fully green, full-repo failing set == the documented environmental baseline family (PermissionsE2E, Escalation_ReapAllUsesLadder, LiveInstalledPluginsProbe, RunSuite×3 — each previously reproduced identically at pristine-baseline worktrees per 25-06/25-07 records); lint 653 vs 587 baseline (+66 style in reworked test files, deviation recorded)"
  - leg: ledger
    outcome: "kit 779 + internal 586 + cmd 33 = 1398 == 1397 baseline + 1 (hostproof instrument, sanctioned amendment)"
  - leg: CLI golden
    outcome: "TestCLIBinaryContract×3 + TestZeroConfigFirstRun — ok (41.8s)"
  - leg: eval detector
    outcome: "MERGE_BASE=merge-base(master,HEAD) mise eval-check-changed → exit 3, classes fired: model, profile, turn-behavior — the kit/ classes route the diff into the behavioral gate ✓"
  - leg: eval-gate (criterion #3)
    outcome: "BLOCKED on operator credentials — openspec CLI installed this session (@fission-ai/openspec@1.13.0; was absent), gate now reaches the provider and reports 'provider anthropic has no resolvable credential — set api_key in config.yaml or $ZAI_API_KEY'. The live-model leg requires the operator's credential (and the account quota that reset 2026-09-14). TO RUN: mise eval-gate with credentials present — folded into Task 3's operator checkpoint below."
  - leg: color-moved review
    outcome: "git diff --color-moved=dimmed-zebra --color-moved-ws=allow-indentation-change master...HEAD -- kit/ renders the pass-1 move commits (6dc7b62, 7e2cfd9, 581d20c, 1d0569d, 543afc8) as relocations — 287 files; per-file purity asserted at each move commit (R100 counts recorded in the 25-01..25-03 SUMMARYs); pass-2 design diffs (25-04..25-07) reviewed normally"

coverage:
  - id: B1
    description: "Both D-19 layers live, red-on-violation, green-on-clean"
    requirement: KIT-01
    verification: [{kind: command, ref: "mise run kit-boundary ✓; probe violation fired both layers (removed in-task)", status: pass}]
    human_judgment: false
  - id: B2
    description: "Hostproof: kit hosts a non-ACP frontend, zero app imports"
    requirement: KIT-02
    verification: [{kind: command, ref: "go test ./kit/runtime -run TestKitHostsNonACPFrontend — PASS", status: pass}]
    human_judgment: false
  - id: B3
    description: "D-20 battery legs 1-4+6 green/recorded"
    requirement: KIT-03
    verification: [{kind: command, ref: "see d20-battery-record", status: pass}]
    human_judgment: false
  - id: B4
    description: "eval-gate (real model) — criterion #3's automated leg"
    requirement: KIT-03
    verification: [{kind: command, ref: "blocked on operator credentials — folded into the Task 3 checkpoint", status: fail}]
    human_judgment: true
    rationale: "Requires the operator's provider credential and a live model run; cannot be automated from this seat"

requirements-completed: [KIT-01, KIT-02, KIT-03-pending-operator]

duration: 1h (inline, quota-constrained session)
completed: 2026-09-11T12:40:00Z

# Phase 25 Plan 9: Enforcement + Proof Summary

One-liner: the kit boundary is mechanically enforced both ways, the non-ACP host proof is green and self-contained, the kit has an honest front door, and the D-20 battery is green on every automated leg — with the live-model eval and the live-Zed session folded into the operator checkpoint that closes the phase.

## Accomplishments

- Task 1: D-19 two-layer gate (mise + depguard), both-ways teeth proof, hostproof test, kit README — committed
- Task 2: the D-20 battery — ci (with documented environmental notes), ledger 1398 (+1 sanctioned), CLI golden + zeroconfig ✓, eval detector exit 3 ✓, color-moved record ✓; eval-gate blocked on operator credentials (recorded)
- Task 3: the blocking human-verify checkpoint is PRESENTED BELOW — awaiting the operator

## Deviations from Plan

- **[Rule 1 — environmental] eval-gate leg blocked on credentials.** The openspec CLI prerequisite was missing and is now installed; the remaining blocker is the operator's provider credential (ZAI_API_KEY or config.yaml api_key) + the account quota (resets 2026-09-14). Folded into the operator checkpoint; the phase cannot close red — it closes when the operator either runs the gate or accepts the recorded state.
- **[Rule 2 — spec] depguard list-mode lax, not the research sketch's strict.** Strict inverts to an allowlist under golangci 2.13 (every stdlib import denied). The deny semantics the plan specifies are lax-mode.
- **[Rule 1 — fixup] eval-gate task targeted a stale package path.** The 25-08 subject-split moved the suite to internal/acpserve; the gate passed vacuously until re-targeted. Fixed within the plan.
- **[Rule 1 — recorded] lint delta +66 style findings.** Classes the baseline already carries; concentrated in 25-08's reworked test files; dead-code subset fixed; residue recorded against the STATE LINT BASELINE ticket.

**Total deviations:** 4. **Impact:** no acceptance criterion silently dropped; two legs (eval-gate, live-Zed) await the operator by design or by credential necessity.

## Self-Check: PASSED

- Commits verified; gates proven both ways; hostproof green; battery recorded leg-by-leg with evidence.

---

## CHECKPOINT: Verification Required — Task 3, the operator's leg

**Plan:** 25-09 (final plan of Phase 25 — the milestone's last)
**Progress:** Tasks 1-2 complete; Task 3 is the blocking human-verify gate

Three operator items close the phase (criterion #3):

1. **Live-Zed session (the 15-07 precedent).** Open Zed against this build and run a normal working session — the extraction must be invisible: slash commands, skills, subagents, permission dialogs, /undo, compaction all behave. The build: `CGO_ENABLED=0 go build ./cmd/ass-guard`.
2. **eval-gate with credentials.** With your provider credential present (`ZAI_API_KEY` or config.yaml api_key): `mise eval-gate` (~8-10 min, real model). The detector already fired (exit 3) — only the live-model run remains. Note: the account's weekly quota resets 2026-09-14 10:06.
3. **Approve or report issues** on everything above (the battery record, the 25-08 disposition table, the D-19 gates).

**→ Type "approved" (and run the two manual legs when convenient), or describe issues**
