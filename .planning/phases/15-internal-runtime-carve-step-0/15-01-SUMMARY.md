---
phase: 15-internal-runtime-carve-step-0
plan: 01
subsystem: testing
tags: [go, golden-test, cli-contract, test-ledger, characterization]

requires: []
provides:
  - "Pre-move test-function ledger (130 entries, per-file attribution) committed as immutable baseline"
  - "TestCLIBinaryContract* golden suite pinning the full cobra command+flag surface byte-for-byte"
affects: [15-02, 15-03, 15-04, 15-05, 15-06, 15-07]

actuals:
  tokens: 22000
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Binary-spawn CLI characterization: build real binary (CGO_ENABLED=0), spawn --help, compare combined output byte-for-byte against inline goldens transcribed from capture"

key-files:
  created:
    - .planning/phases/15-internal-runtime-carve-step-0/test-ledger.txt
    - cmd/ass-guard/cli_contract_test.go
  modified: []

key-decisions:
  - "Goldens transcribed mechanically from the built binary's actual --help output (never synthesized from source); cobra's default help/completion entries ride along as measured, no hard-coded subcommand count"
  - "Only normalization: temp cwd path (and its macOS symlink-resolved /private/var form) replaced with <TMP> — the sole volatile substring, inside cwd-derived flag defaults (--profiles-dir, --suite, --cache-pin)"
  - "Bare group-name paths (acp, model-routing, profile) derived from the cobra constructors' Use fields, keeping package-wide goconst counts at their clean baseline"
  - "Ledger line format 'func Test <name>' chosen so the grand-total line also matches '^func Test' (131 grep matches = 130 recorded + 1 total), satisfying the plan's verify arithmetic"

patterns-established:
  - "Wave-0 measurement instruments: counted baseline (what must still pass) + contract pin (what the editor invokes) — every later plan's verify leans on these"

requirements-completed: [RUNT-01]

coverage:
  - id: D1
    description: "Pre-move test-function ledger: 130 functions, per-file sections, grand-total line, committed at baseline HEAD"
    requirement: RUNT-01
    verification:
      - kind: other
        ref: "grep -hc '^func Test' cmd/ass-guard/*_test.go | awk '{s+=$1} END {print s}' -> 130; grep -c '^func Test' test-ledger.txt -> 131"
        status: pass
  - id: D2
    description: "CLI contract golden suite pinning root/acp/checkpoint/learning/model-routing/profile/parity command+flag surfaces"
    requirement: RUNT-01
    verification:
      - kind: unit
        ref: "cmd/ass-guard/cli_contract_test.go#TestCLIBinaryContractRootAndACP,TestCLIBinaryContractSupportCommands,TestCLIBinaryContractProfile"
        status: pass

duration: 13 min
completed: 2026-08-26
---

# Phase 15 Plan 01: Wave-0 Measurement Instruments Summary

**One-liner:** Counted test baseline (130 functions) + binary-spawn CLI golden suite pinning all 15 command surfaces byte-for-byte — the equivalence instruments every later carve plan proves against.

## Accomplishments

- Captured the pre-move test-function ledger at HEAD `9be5386`: 130 `^func Test` declarations across 22 `cmd/ass-guard/*_test.go` files, per-file ordered sections, grand-total line, and the post-carve sum rule. Committed before any file moved (immutable history).
- Built `cmd/ass-guard/cli_contract_test.go`: three test functions (root+acp, four support command groups, profile) spawning the real compiled binary (CGO_ENABLED=0, mise recipe) with `--help` per surface, comparing combined output byte-for-byte against inline raw-string goldens transcribed verbatim from the binary's actual output. Root golden carries all six AddCommand groups plus cobra's default help/completion entries as rendered; acp serve pins `--profile --max-concurrent --profiles-dir --work-dir --no-engine --ask-timeout` (+ persistent `--audit-log`); parity pins its 7 flags; checkpoint/learning/model-routing/profile pin their subcommand flags.
- Only normalization is the temp cwd → `<TMP>` (including the macOS `/private/var` symlink-resolved form), which appears exclusively inside cwd-derived flag defaults.

## Verification Results

- `go test ./cmd/ass-guard/ -run TestCLIBinaryContract -count=1` — PASS (3 functions, 15 subtests).
- Ledger verify: live 130 = plan expectation; ledger grep 131 = 130 recorded + 1 total line.
- Full quick gate: `go build ./... && go vet ./... && go test ./cmd/ass-guard/ -count=1` — all green (26s).
- `golangci-lint run cmd/ass-guard/...` — 0 issues (baseline preserved).
- No production file modified: `git status --porcelain cmd/ass-guard/ -- ':!cmd/ass-guard/cli_contract_test.go'` empty.

## Deviations from Plan

None - plan executed exactly as written.

## Notes for Later Plans

- Live `^func Test` count in cmd/ass-guard is now **133** (130 baseline + the 3 sanctioned Wave-0 contract-test functions). The ledger's post-carve sum rule must account for these three remaining in cmd/ass-guard at close-out (see plan 15-07's equivalence battery).
- The contract test is a pre-carve PASSING characterization suite (RED/GREEN collapsed into a baseline pin, per the plan's tdd note).

## Self-Check: PASSED

- test-ledger.txt exists and committed (870576a).
- cli_contract_test.go exists and committed (d4ee9ab).
- Both commit hashes present in `git log`.
