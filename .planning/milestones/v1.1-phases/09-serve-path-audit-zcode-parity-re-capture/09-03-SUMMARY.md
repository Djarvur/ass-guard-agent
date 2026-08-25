---
phase: 09-serve-path-audit-zcode-parity-re-capture
plan: 03
subsystem: stability-regrounding
tags: [aud-05, stability, pinned-session, canary, runbook, version-provenance]
status: complete

# Dependency graph
requires:
  - phase: Phase 1 (extraction + coverage manifest) + research Pitfalls 17/18
    provides: ExtractFromRollout, coverage.yaml, the bias critique
provides:
  - "the stability test consumes the PINNED session from coverage.yaml (selection bias removed)"
  - "the divergence canary: extraction provably rejects mid-session catalog changes (non-vacuity)"
  - "zcode_version capture provenance: -zcode-version flag → coverage.yaml + meta.yaml → profile check footer"
  - "docs/recapture-runbook.md — the operator procedure (D-04 verbatim), Run record empty for 09-04"
affects: [09-04 (the delegated harvest + capture + re-pin rides this machinery)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "pinned-ID consumption reads exactly ONE file (no wholesale scan on the within-session path — T-9-13)"
    - "synthetic canary fixtures carry fake values only, structure copied (never real session content — T-9-12)"

key-files:
  created:
    - internal/profile/testdata/divergent-session/model-io-sess_testdivergent.jsonl
    - internal/profile/testdata/divergent-session/model-io-sess_testhomogeneous.jsonl
    - docs/recapture-runbook.md
  modified:
    - internal/profile/stability_test.go
    - internal/profile/types.go
    - internal/profile/coverage_test.go
    - cmd/extract-profile/main.go
    - cmd/ass-guard/profile_check.go
    - cmd/ass-guard/profile_check_test.go

key-decisions:
  - "the cross-session header-name test KEEPS its scan (version-insensitive invariant; the bias critique targets extraction-SOURCE selection) — noted in a comment, per plan"
  - "ZcodeVersion threaded via the run() flag → writeArtifact → manifest+meta (single source, no dual writes)"
  - "shipped profiles/zcode untouched — its zcode_version lands with 09-04's REAL capture, never hand-edited"

patterns-established:
  - "acceptance greps as spec: the selection-path words are purged even from comments"

requirements-completed: [AUD-05-machinery]

# Metrics
duration: 55min
completed: 2026-08-15
---

# Phase 9 Plan 03: Stability re-grounding machinery Summary

**The stability test now runs on the PINNED session (richest-selection and wholesale scans gone from its path), provably CAN fail (the divergence canary), records both capture-time versions, and the operator runbook lives in-repo verbatim — everything 09-04's single capture needs.**

## Performance

- **Duration:** ~55 min
- **Tasks:** T1 (RED `3fa4efa` → GREEN `28b5357`), T2 (`6bc0d96`), T3 (`b3ae408`)

## Accomplishments

- **T1** — `TestStability_WithinSessionExtractionSource` rewritten: loads `profiles/zcode/coverage.yaml`, resolves `TargetCaptureRef.Sessions[0].ID`, stats `<rolloutDir>/model-io-sess_<ID>.jsonl`, skips CLEAN naming the pinned ID + the runbook while absent (today's state — the pinned `sess_016eee8a…` is not on disk), else asserts `ExtractFromRollout` succeeds. Selection-path grep (`PickRichestMain|ScanRolloutDir` within the test) = 0 — Pitfall 17 closed. Canary fixtures: `model-io-sess_testdivergent.jsonl` (second line ADDS a tool — extraction FAILS) + `model-io-sess_testhomogeneous.jsonl` (identical catalogs — passes). Cross-session test unchanged + the version-insensitivity note.
- **T2** — `TargetCaptureRef.ZcodeVersion` (`zcode_version`); `extract-profile -zcode-version` stamps coverage.yaml `target_capture_ref.zcode_version` AND meta.yaml `zcode_version`; `profile check` prints `capture provenance: zcode <v|unrecorded>, extractor <v|unrecorded>` after the drift footer. Round-trip + footer tests green; shipped profile untouched.
- **T3** — `docs/recapture-runbook.md`: purpose (non-vacuous stability), preconditions (fresh rollout dir / zcode version FIRST), the scripted 5-turn workload with REJECTION THRESHOLDS (≥5 turns, ≥10 tools, ≥1 subagent, ≥1 mid-session catalog change — re-run never relax), pin+verify (canary sanity-run), extraction (NO API key; drift report committed BEFORE the profile update; `-sessions` pinning mandatory, flag-less default FORBIDDEN), re-baseline (parity DOES need ZAI_API_KEY; Phase-1 numbers SUPERSEDED explicitly), empty Run record table. Acceptance greps all ≥ the plan's floors.

## Task Commits

1. **T1 RED** `3fa4efa` → **GREEN** `28b5357`
2. **T2** `6bc0d96`
3. **T3** `b3ae408`

**Plan metadata:** this commit

## Deviations from Plan

None.

## TDD Gate Compliance

T1 strict RED→GREEN; T2/T3 are `type:auto` tasks with their specified tests (round-trip, footer). `go test ./internal/profile/ ./cmd/ass-guard/ -race` green; `mise run ci` green (exit 0); lint 0 issues.

## Verification (re-runnable)

- `go test ./internal/profile/ -race -run 'TestStability' -v` — pinned test SKIP (names the runbook), canary PASS, homogeneous PASS
- `awk '/TestStability_WithinSessionExtractionSource/,/^}/' internal/profile/stability_test.go | grep -c "PickRichestMain\|ScanRolloutDir"` → 0
- `grep -n "zcode_version" internal/profile/types.go cmd/extract-profile/main.go` → both
- `grep -n "capture provenance" cmd/ass-guard/profile_check.go` → the footer
- `git log --oneline -- profiles/zcode/` → untouched by this plan

---
*Phase: 09-serve-path-audit-zcode-parity-re-capture*
*Completed: 2026-08-15*
