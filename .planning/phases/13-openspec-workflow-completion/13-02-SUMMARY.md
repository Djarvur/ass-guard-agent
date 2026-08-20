---
phase: 13-openspec-workflow-completion
plan: "02"
subsystem: expanded-matrix-chaining
tags: [zero-continue, seeded-patterns, d07-verify-handoff, pass2]
requires:
  - "13-01 (the capture corpus)"
provides:
  - "the expanded-matrix chaining rows: post-verify-handoff (D-07) + post-new-continue-handoff, with termini dispositions documented in-table"
  - "the pass-2 zero-continue legs: TestOpsxMatrixVerifyChain_Gated + TestOpsxMatrixNewChain_Gated (audit-trail assertions)"
affects:
  - "13-03 (the advisory must not fire on the matched closings)"
  - "13-04 (the suites assert the chained flows)"
tech-stack:
  added: []
  patterns:
    - "structural-anchor regex (header token + consequence alternation) for template-driven closings"
key-files:
  created: []
  modified:
    - internal/openspec/seeded.toml
    - cmd/ass-guard/e2e_opsx_matrix_test.go
decisions:
  - "verify's anchor = report header + blocking consequence (three observed closings; RE2's 1000-repeat cap forced the .* form under (?s))"
  - "ff/bulk/onboard left as documented termini (archive shield / unmatched / the D-02 question class)"
metrics:
  duration: 130min
  tasks: 2
  commits: 1
status: complete
actuals:
  tokens: 98000
  tasks: 2
  commits: 1
---

# Phase 13 Plan 02: Zero-continue chaining over the expanded matrix Summary

**One-liner:** The expanded matrix chains hands-off from real captured closings — verify routes into
the fix workflow (D-07) and new walks the whole artifact sequence via continue, both proven
zero-continue on fresh gated runs with audit-trail evidence; the termini are documented, not faked.

## What was built

| Task | Result |
|------|--------|
| 1 (tracer, D-07) | `post-verify-handoff` seeded from THREE observed closings (the two 13-01 captures + the chain-leg diagnostic — wording varies, the STRUCTURE does not: the "Verification Report" header + the blocking consequence). Target: `/opsx:continue` (what all three closings' recommendations instruct). TestOpsxMatrixVerifyChain_Gated GREEN: one typed /opsx:verify → continue decision + opsx:continue provenance + design.md on disk + natural terminal. |
| 2 (full matrix) | `post-new-continue-handoff` (new's and continue's own "Run `/opsx:continue`" template phrase — the self-chain walks the spec-driven sequence); TestOpsxMatrixNewChain_Gated GREEN (483s: ≥2 chained continues, provenance, tasks.md live-or-archive, natural terminal). Dispositions documented in-table: ff → the existing archive shield (its closing IS Archive Complete); bulk-archive → unmatched terminus (fixture-specific warning text would over-fit); onboard → the D-02 question class (13-03's advisory). |

## Audit-trail evidence (the phase-gate requirement)

From the green chain runs: `engine_decision` lines with `name="continue"` (≥1 verify-chain, ≥2
new-chain) + `command_provenance` lines `name="opsx:continue"` — both read from the session
transcripts by the legs' assertions; the final decisions are non-continue (natural termination).

## Structural safety re-pin

The existing engine batteries (assistant-role-only, provenance-not-injectable, unmatched⇒nothing)
load the REAL seeded table (`newExpansionRunner` → setupEngine → DefaultConfig), so they re-ran
green WITH the expanded rows — no assertion touched. `go test ./internal/engine/ ./internal/openspec/` + `mise ci` green.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1] The first verify anchor was too narrow** — "cannot be archived" matched 2/2 captures
but a third run phrased "before archiving"; the diagnostic run's full closing (harvested + noted
in-table) drove the structural re-anchor. Two chain-leg runs consumed (both preserved in git
history + /tmp logs).

**2. [Rule 3] RE2 repeat-count cap** — `{0,2500}` rejected (max 1000); the `.*` form under `(?s)`
is equivalent for a bounded single-closing match.

**3. [Environmental] Full-suite load flakes** — the timing-sensitive trio
(ProcessGroupKill/ServeMirror_Override/ChainSurvivesAskTimerResume) trips under heavy parallel
load, passes isolated AND on the D-06 single re-run (zero delta). The chain pin's deadline was
already bumped 5s→15s at 13-01's close.

## D-06 record

- The verify-chain leg: run 1 RED (anchor miss — a SOURCE fix followed, not a re-run), then GREEN.
- The new-chain leg: green first run.
- mise ci: two load-flake singles, each green on the single zero-delta re-run; no best-of-N.

## Known Stubs

None.

## Threat Flags

None (T-13-02-01 mitigated: the anchors are template phrases + structural tokens, the
over-broadening rejection stands — the first widening attempt was CORRECTED narrower, not wider).
