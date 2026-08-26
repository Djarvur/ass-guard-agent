---
phase: 15
phase_name: internal-runtime-carve-step-0
depth: standard
status: issues_found
files_reviewed: 52
scope_source: "SUMMARY.md extraction + git-diff cross-check (#2666); >50 files demoted deep→standard"
reviewer: inline (sequential execution mode — no subagent dispatch)
findings:
  critical: 0
  warning: 1
  info: 2
  total: 3
---

# Phase 15 Code Review

Scope: 52 files (41 internal/, 11 cmd/) — the full carve surface from the
seven SUMMARY key-files lists, cross-checked against the phase diff
(`330a97e^..HEAD`, planning artifacts excluded). Depth reduced deep→standard
per the >50-file rule.

Method: per-file review of the relocated/new sources with the equivalence
lens — this phase's contract is zero behavior change, so the review focused
on transcription drift, seam-rewiring correctness, and latent hazards
introduced by the package split.

## Findings

### WR-01: BridgeConfig.Expand can be nil while Invoke is set (latent panic)

- **File:** internal/runtime/enginebridge/enginebridge.go (EngineTurnAdapter.Run)
- **Severity:** warning
- **Detail:** `Run` nil-guards `a.cfg.Invoke` and `a.cfg.AutomationProvenance`
  but calls `a.cfg.Expand(...)` unconditionally inside the Invoke branch. All
  three current construction sites pair Invoke+Expand (runOneTurn wires all
  three; the battery's one expansion test pairs Invoke+Expand; the two
  LastTurnOutput-only sites pass an empty config so the branch never runs),
  so no live path panics — but a future caller setting Invoke without Expand
  gets a nil-deref one turn in.
- **Recommendation:** either nil-guard `a.cfg.Expand` (skip expansion when
  absent) or document the Invoke⇒Expand pairing requirement on the
  BridgeConfig fields. Not fixed in-phase: this is a pure-relocation phase;
  the guard is a behavior-adjacent edit best made deliberately.

### IN-01: test-only constants in a production file

- **File:** internal/runtime/goconst_constants.go
- **Severity:** info
- **Detail:** `protocolVersion20`, `keySessionID`, `keyType`, `textListKey`,
  `chunkDone`, `implementationCompleteMsg`, `actionContinue`, `profileZcode`
  are consumed only by the relocated test battery but live in the production
  goconst file (mirrors the pre-carve cmd layout, which mixed them the same
  way). Lint-clean (goconst counts test usage), but the file could be split
  into a `_test.go` sibling for package hygiene.
- **Recommendation:** optional cleanup; zero behavioral impact.

### IN-02: duplicated cross-package constants by design (confirmation, not defect)

- **File:** internal/runtime/goconst_constants.go, internal/acpserve/goconst_constants_test.go, internal/runtime/enginebridge/goconst_constants.go
- **Severity:** info
- **Detail:** `blockText`/`stopAskACP`/`profileZcode` etc. deliberately
  duplicated per package per D-03 (never a shared testutil). Recorded so a
  future reviewer does not "fix" the duplication into a shared dependency
  that would recreate the coupling the carve removed.

## Clean checks (no findings)

- **Equivalence spine:** acpserve.Run statement order matches source
  :320-425 including the WINDOWS #3 emitter-before-scheduler ordering;
  SetSchedule/SetEmitter/StartScheduler/CloseAllSessions hit the verbatim
  bodies; cron_wiring.go is receiver-sed only (amended D-13/D-15).
- **sessionFor/runOneTurn transcription:** heavy battery coverage
  (expansion, skill, ask routing/park, provenance chain, checkpoint wiring,
  cron) green under `-race`; no drift detected beyond the documented
  StubCatalogExec ctor swap.
- **Security surface:** no new endpoints, auth paths, file-access patterns,
  or schema changes; threat register for the phase declared none new —
  confirmed by inspection.
- **cmd shells:** flag defaults, de-cobra'd reads, and resolve/seed order
  preserved; CLI-contract goldens + zero-config green.
- **No dead code:** every moved helper has an in-package consumer (verified
  by golangci-lint unused + build).

## Verdict

No critical findings. One latent-contract warning (WR-01) recommended for a
follow-up guard; two informational notes. The phase's zero-behavior-change
contract holds on the reviewed evidence.
