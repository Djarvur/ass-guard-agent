---
phase: quick-261013-uhw
plan: 01
subsystem: security
tags: [permissions, wr02, command-substitution, shell-quoting, scanner, go, tdd]

# Dependency graph
requires:
  - phase: 17-permission-gate
    provides: the perm.RuleSet evaluator and the WR-02 substitution guard (evaluateSingle guard-before-allow-scan) this plan repairs
  - phase: 24-documentation-ops-tails
    provides: the golangci 2.13 lint baseline (cyclop 15 / lll 120 / nolint conventions) the touched files had to satisfy
provides:
  - hasSubstitution with restored double-quote state tracking (live `$(...)`/backtick detection inside double quotes regardless of nested single quotes/apostrophes; single-quoted spans fully literal in both directions)
  - TestHasSubstitutionQuotes — ten-row scanner-level quote-nesting pin (in-package, over the unexported scanner)
  - four evaluate-level WR-02 TestRules rows: three nested-quote never-allow rows plus the compound interplay row (one subcommand AND allow denied)
  - RED evidence record for the tdd_mode #3770 gate (RED_EVIDENCE_OK / target_test_failed)
affects: [permission-gate, acpserve-permissions-e2e, quick-261013-uhx, windows-ledger]

# Commit bookkeeping (#3968)
commits: 4
plan_head_before: 56ca371bfa43edaa4f1473543aea4bf6065003b7

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "quote state machine (0 / '\\' / '\"') restored verbatim from the pre-a710b75 diff; cyclop accommodated with a reasoned nolint, not a restructuring"
    - "Go-to-Node-TAP faithful projection for the tdd-red-evidence checker (24-04 gotap.py precedent, leaf+parent 1:1)"

key-files:
  created:
    - internal/perm/rules_internal_test.go
    - .planning/quick/261013-uhw-fix-hassubstitution-double-quote-tracking-in-internal-perm-r/261013-uhw-task1-red-evidence.json
    - .planning/quick/261013-uhw-fix-hassubstitution-double-quote-tracking-in-internal-perm-r/deferred-items.md
  modified:
    - internal/perm/rules.go
    - internal/perm/rules_test.go

key-decisions:
  - "Verbatim restore over restructuring: hasSubstitution is the pre-a710b75 quote state machine byte-for-byte in logic (extra comments only); the cyclop-15 gate is accommodated with a reasoned //nolint:cyclop — zero behavioral-drift risk from re-shaping proven security code"
  - "Scanner-level test lives in a new in-package rules_internal_test.go (package perm): rules_test.go is the external perm_test package and cannot reach the unexported scanner — store_test.go is the established in-package precedent"
  - "The a710b75-dropped unterminated-double-quote fail-safe direction and the already-working substitution-with-single-quotes case are pinned as passing-at-master rows so the fix cannot over- or under-rotate (T-UHW-02 two-direction pinning)"

requirements-completed: []

# Metrics
duration: 34min
completed: 2026-10-13
status: complete
---

# Quick Batch 261013-uhw: hasSubstitution Double-Quote Tracking Fix Summary

**Restored the a710b75-dropped double-quote state machine in `internal/perm`'s `hasSubstitution` — `git "log 'x $(cmd) y'"` now reports live substitution and can never match an allow rule (WR-02 closed end-to-end), pinned by 14 new table rows at scanner and evaluate level.**

## Performance

- **Duration:** 34 min
- **Started:** 2026-10-13T20:01:59Z
- **Completed:** 2026-10-13T20:36:12Z
- **Tasks:** 2 (Task 1 as RED + GREEN commits)
- **Files modified:** 3 code/test files + 2 planning artifacts

## Accomplishments

- WR-02 hole closed: `hasSubstitution` tracks a quote state (unquoted / single-quoted literal span / double-quoted), so live `$(...)` and backtick substitution inside double quotes is detected even when single quotes or apostrophes appear inside them; single-quoted spans stay fully literal in both directions; the unterminated-single-quote early-false semantics are preserved.
- The WR-02 one-sided contract is untouched: the guard still fires only in `evaluateSingle` before the allow scan; deny/ask still scan the raw subject; `SplitCompound` untouched; stdlib-only.
- Doc comment states the nesting rule explicitly and the bounded-heuristic scope (process substitution, ANSI-C `$'...'`, heredocs outside WR-02 — T-UHW-03 recorded boundary).
- STATE.md Blockers/Concerns WR-02 item ("deserves a fix ticket in the next phase touching internal/perm") is dischargeable: fix + pins committed, package gate green.

## Task Commits

Each task was committed atomically:

1. **Task 1 RED: pin WR-02 nested-quote substitution cases** - `a4b0529` (test) — compiling-but-failing per tdd_mode #3770; RED evidence record verified `RED_EVIDENCE_OK / target_test_failed` (7 intended failures: 4 scanner live rows + 3 evaluate rows; 6 literal-direction pins already green at master)
2. **Task 1 GREEN: restore double-quote tracking in hasSubstitution** - `4ec174d` (fix)
3. **Task 2: pin WR-02 compound interplay** - `1c7c6c3` (test)

**Plan metadata:** `ee4de04` (docs: deferred-items note for the pre-existing lint baseline)

## Files Created/Modified

- `internal/perm/rules.go` — `hasSubstitution` restored double-quote state machine + doc comment (nesting rule + bounded scope)
- `internal/perm/rules_internal_test.go` — NEW: `TestHasSubstitutionQuotes`, ten quote-nesting rows directly over the unexported scanner (in-package `package perm`)
- `internal/perm/rules_test.go` — four new evaluate-level rows in the `TestRules` WR-02 block (three nested-quote never-allow + the compound interplay row)
- `.planning/quick/261013-uhw-.../261013-uhw-task1-red-evidence.json` — gate-verified RED evidence
- `.planning/quick/261013-uhw-.../deferred-items.md` — out-of-scope discovery record (see Deviations)

## Decisions Made

- **Verbatim restore over restructuring:** the restored body is the pre-a710b75 state machine (recovered from `git show a710b75 -- internal/perm/rules.go`) with comments added; the repo's cyclop-15 gate (post-dating the original code) is accommodated with a reasoned `//nolint:cyclop` — the established repo pattern (SplitMCPName, store_test.go, TestRules) — rather than re-shaping proven security code.
- **In-package scanner test:** the plan's "TestHasSubstitutionQuotes in rules_test.go, directly over hasSubstitution" cannot compile (rules_test.go is `package perm_test`); the table lives in a new `rules_internal_test.go` (`package perm`), mirroring store_test.go's in-package precedent.
- **Two-direction pinning (T-UHW-02):** rows that already pass at master (substitution containing single quotes; unterminated double quote fail-safe; backslash escapes inside double quotes; both single-quote literal controls) are pinned alongside the four live-direction fixes so the fix cannot over- or under-rotate.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Scanner-level table relocated to a new in-package test file**
- **Found during:** Task 1 RED
- **Issue:** the plan prescribed `TestHasSubstitutionQuotes` "in rules_test.go", but rules_test.go is the external `perm_test` package — a direct call to the unexported `hasSubstitution` cannot compile from there
- **Fix:** created `internal/perm/rules_internal_test.go` (`package perm`), following the store_test.go in-package precedent; the evaluate-level rows stayed in rules_test.go as planned
- **Files modified:** internal/perm/rules_internal_test.go (new)
- **Verification:** compiles, RED fails on real assertions, GREEN passes; package lint 0 issues
- **Committed in:** a4b0529 (RED) / 4ec174d (GREEN, nolint comment placement)

### Verify-Criterion Deviation (documented, NOT auto-fixable in scope)

**2. `mise run ci` does not exit 0 — pre-existing repo-wide lint baseline (out of scope per the scope boundary)**
- **Found during:** Task 2 full-gate run
- **Issue:** the lint leg fails with 653 repo-wide findings concentrated in kit/ (runtime.go 58 + kit test files); zero findings in internal/perm or any file this plan touched. This is the documented STATE baseline drift (~572-599 findings recorded in Phase 24/25) grown by later kit/ moves — `mise run ci` has not exited 0 on lint at master regardless of this plan. vet, build, and kit-boundary legs pass; the test -race leg was run to completion separately: 39 packages ok, with exactly 4 failures (TestPermissionsE2E, TestZedSimulatorE2E, TestSimulatorCommandSurface, TestLiveInstalledPluginsProbe) — all four REPRODUCED IDENTICALLY at the pre-plan HEAD (56ca371) in a detached worktree, matching documented deferred items (D-23-07-1/3, CROSS-WORKSTREAM TestPermissionsE2E regression owned by sibling quick item 261013-uhx)
- **Fix:** none taken (fixing 653 pre-existing findings or the four pre-existing test failures is outside this item's file scope: internal/perm/rules.go + its test files). Documented in deferred-items.md
- **Verification:** `go test -race -count=1 ./internal/perm/` green; `go vet ./internal/perm/` clean; `golangci-lint run internal/perm/...` (cache-clean) 0 issues
- **Committed in:** ee4de04 (deferred-items.md)

---

**Total deviations:** 1 auto-fixed (Rule 3 blocking) + 1 documented out-of-scope verify deviation
**Impact on plan:** All security truths in `must_haves` are met and pinned; only the repo-wide `mise run ci` exit-0 phrasing is unmet, by a proven pre-existing baseline this item must not touch.

## Issues Encountered

- `gsd check tdd-red-evidence` initially returned INVALID_RED (no_target_test_failure): the checker's TAP parser needs the exact target test name, and Go's `--- FAIL:` parent lines had been filtered out by the leaf-only projection. Fixed by projecting every Go result line 1:1 (Go reports the parent FAIL when a child fails; node:test's dialect emits the parent result too) — verdict then RED_EVIDENCE_OK / target_test_failed. Projection script: /tmp/gsd-uhw-gotap.py, raw log /tmp/gsd-261013-uhw-red.log (24-04 gotap.py precedent).
- `gsd windows append` for the deviation ledger failed: pre-existing WINDOWS.md row-29 table-vs-fenced-JSON disagreement ("never hand-edit the rendered table"). Row 29 is the ECOS-04 24-06 entry; the ledger bookkeeping is owned by sibling quick item 261013-uhz in this dispatch round — the deviation is instead recorded in this item's deferred-items.md and this SUMMARY.

## Known Stubs

None — no stub patterns in the created/modified files.

## Threat Flags

None — no security surface beyond the plan's threat model was introduced; the change IS the T-UHW-01 mitigation. T-UHW-03 residual scope (process substitution, ANSI-C quotes, heredocs) is stated in the restored doc comment; deny/ask remain the strong unbounded control.

## Next Phase Readiness

- STATE.md Blockers/Concerns WR-02 entry is dischargeable by the orchestrator: the fix + pins are committed and the package gate (test -race, vet, lint) is green.
- internal/perm is touched for the first time since Phase 17 — future phases touching it inherit the two-direction quote-nesting pin battery.
- Sibling quick item 261013-uhx (TestPermissionsE2E regression) is unaffected: the four failing acpserve/ecosys tests fail identically with and without this plan's commits (baseline-worktree proof).

## Self-Check: PASSED

- Files: internal/perm/rules.go, internal/perm/rules_test.go, internal/perm/rules_internal_test.go — FOUND
- Commits: a4b0529, 4ec174d, 1c7c6c3, ee4de04 — FOUND (4 commits over ledger base 56ca371)
- `go test -race -count=1 ./internal/perm/` green; `go vet ./internal/perm/` clean; package lint 0 issues

---
*Phase: quick-261013-uhw*
*Completed: 2026-10-13*
