---
phase: quick-261014-1ne
plan: "01"
subsystem: lint-gate
tags: [golangci-lint, ci-gate, config, code-hygiene]
requires: [".golangci.yml (2.13.2 schema)", "mise lint task", "findings census (655 @ master)"]
provides: ["green cache-clean mise run lint", "dated config disposition block", "zero unused nolint directives"]
affects: [".golangci.yml", "~170 .go files across internal/ and kit/"]
tech-stack:
  added: []
  patterns:
    - "config+code hybrid gate repair (24-01 baseline-repair precedent, extended to the 2.13 linter set)"
    - "disposition taxonomy: fix / site-nolint-with-reason / deferred-items record"
key-files:
  created:
    - .planning/quick/261014-1ne-restore-the-golangci-lint-gate-make-mise/261014-1ne-deferred-items.md
  modified:
    - .golangci.yml
    - internal/coreexec/{ansistrip,background,bash,ptty,subagent}.go
    - kit/runtime/{runtime,commands,cron_wiring,seams}.go
    - kit/session/{session,steering,subagent}_test.go
    - internal/sandbox/{policy,child_linux,landlock_linux}.go
    - "plus ~125 further .go files (full per-commit file lists in git)"
decisions:
  - "gocritic sizeThreshold 129 not 128 (Rule 3): gocritic compares >= the threshold, so the plan-pinned 128 still flagged the exactly-128-byte parity structs the plan's own comment meant to exempt"
  - "seven opinionated 2.13 classes disabled with dated rationale; every real-signal class disposed site-by-site (nilerr/contextcheck/noctx confirmed against the 22-08/23-04/19-02 doctrines)"
  - "nolint directives for func-level linters must be same-line — preceding-line directives are reported unused by nolintlint while the finding survives"
  - "unknown-linter directive names (err113-best-effort, testingcontext) purged — kills the per-run nolint_filter warning"
metrics:
  duration: 54 min
  completed: 2026-10-13
status: complete
commits: 3
plan_head_before: 0f06e1b0043ee77c00b226e5a8d51c12889a5471
actuals:
  tokens: 57246
  tasks: 3
  commits: 3
---

# Quick Task 261014-1ne: Restore the golangci-lint gate Summary

Cache-clean `mise run lint` exits 0 again — the 655-finding backlog (broken since
2026-09-03 by the mise auto-update to golangci-lint 2.13.2, WINDOWS #16/#19/#20)
is fully disposed via a config+code hybrid: seven opinionated 2.13-only classes
disabled with dated rationale, fixture-noisy classes excluded for test files,
~170 .go files mechanically repaired, and every real-signal class
(nilerr, contextcheck, noctx, staticcheck, unused, forcetypeassert, errchkjson,
embeddedstructfieldcheck) inspected and disposed site-by-site.

## Tasks Completed

| Task | Name | Commit | Files |
| ---- | ---- | ------ | ----- |
| 1 | Repair .golangci.yml — disable, tune, exclude with dated rationale | cdb10a1 | 1 |
| 2 | Mechanical code fixes — stale nolints, formatting, unambiguous rewrites | da54b07 | 123 |
| 3 | Inspect-and-dispose real-signal classes, residual sweep, acceptance gate | 72932fb | 50 |

## What Was Done

**Task 1 — config (cdb10a1).** Disabled funcorder, paralleltest, nonamedreturns,
wrapcheck, err113, nlreturn, testpackage — each with a dated (2026-10-14) comment
citing the concrete repo invariant. Tuned cyclop to 20 (measured: no prod function
exceeds 18) and funlen lines to 80 (gateCall chokepoint at 63). Added test-file
exclusions for lll/goconst/tagliatelle/noctx (fixture-noisy classes). Set gocritic
hugeParam/rangeValCopy size thresholds. Real-signal linters untouched. Findings
655 → 471; the seven disabled classes report zero; nolintlint exploded 57 → 272
(the stale-directive cascade Task 2 sweeps).

**Task 2 — mechanical (da54b07).** Stale-nolint sweep looped to zero (272 → 0 over
three passes: dead directives + reasons deleted, partially-stale lists trimmed).
gofmt/doc-comment reflow; 12 intrange rewrites (one reasoned skip:
range-over-string would change the byte-boundary fixture's semantics); perfsprint,
thelper, canonicalheader, whitespace, dogsled, godoclint, paramTypeCombine (all
matches), builtinShadow (cap → capacity), unnamedResult (16 result lists named),
mnd (12 named constants + one reasoned 0o750), goconst prod (2 constants), lll
prod (13 wraps, string bytes preserved via concatenation splits). Build and vet
green; every Task 2 class reports zero.

**Task 3 — inspect-and-dispose (72932fb).** nilerr ×2 and the contextcheck chain
confirmed against the 23-04 sync-store / 22-08 serve-gated-wake / 19-02 envelope
doctrines and dispositioned with site nolints (plus one trivial test fix: the
gitRun seam now inherits the caller ctx). noctx: persistent-shell doctrine nolint
(22-04 — CommandContext would kill the request-outliving shell). staticcheck ×5,
unused ×6 (deleted after reflection-usage checks), forcetypeassert ×2 (comma-ok),
errchkjson ×4 (RawMessage marshals now checked loudly), embeddedstructfieldcheck
×4 (blank-line separation — declaration-order-neutral, pin-safe by construction),
gochecknoglobals ×5, appendAssign ×2 (explicit fresh-buffer appends), and the
complexity classes (gocyclo/cyclop/gocognit/funlen/maintidx/nestif) disposed as
flat-battery / ordered-sequence reasoned nolints. No correctness class disabled.
Accepted gate: `mise exec -- golangci-lint cache clean && mise run lint` exits 0.

## Verification

- `mise exec -- golangci-lint cache clean && mise run lint` → **exit 0, 0 issues** (655 → 0)
- `go build ./...`, `go vet ./...` → exit 0
- `go test -race -count=1` green on every touched package except the documented
  pre-existing failures: D-23-07-1 simulator pair (reproduced at pre-edit HEAD in
  a throwaway worktree — proven pre-existing) and D-23-07-3
  TestLiveInstalledPluginsProbe. TestEscalation_TermRespectingChildDiesByTerm
  failed once under full-suite load, passed in isolation and on package re-run
  (documented flake family; background.go edits are semantic no-ops). Nothing
  fixed, skipped, or widened for them.
- Every nolint directive carries a same-line reason and fires (nolintlint: 0;
  the unknown-name warning is gone)
- No edits to .planning/WINDOWS.md, STATE.md, or ROADMAP.md

## Deviations from Plan

**1. [Rule 3 - Blocking issue] gocritic sizeThreshold 129, not the plan-pinned 128**
- **Found during:** Task 3
- **Issue:** gocritic compares copy size >= the threshold; at 128 the
  exactly-128-byte parity structs (session.ContentBlock et al.) were still
  flagged — contradicting the plan's own stated intent ("128 keeps the check on
  genuinely oversized copies without flagging them [~80-128-byte parity shapes]").
- **Fix:** threshold set to 129 (the smallest value honoring the intent), comment
  updated with the measured comparison semantics; 14 exactly-128-byte sites
  cleared by the correction; all larger copies disposed site-by-site.
- **Files modified:** .golangci.yml
- **Commit:** 72932fb

**2. [Rule 3 - Blocking issue] v2 schema key naming for gocritic thresholds**
- **Found during:** Task 1
- **Issue:** `hugeParam.size-threshold` fails `golangci-lint config verify` — the
  v2 schema uses camelCase `sizeThreshold` (verified against the published
  v2.13.2 JSON schema).
- **Fix:** camelCase keys.
- **Commit:** cdb10a1

**3. [Rule 2 - Missing critical functionality] complexity-class reporting cascade**
- **Found during:** Task 3
- **Issue:** only ONE complexity-class issue per function reports per run —
  covering gocognit surfaced gocyclo, then cyclop/funlen/maintidx in successive
  cache-clean runs; directive lists had to be extended iteratively (five sweep
  iterations).
- **Fix:** residual-sweep loop per the plan's own contract; the mechanism is
  recorded in deferred-items.md for future gate work.

## Auth Gates

None.

## Known Stubs

None — this plan adds no runtime features; every disposition is a repair or an
auditable suppression.

## Deferred Issues

See `261014-1ne-deferred-items.md` in this directory: the floating
`golangci-lint = "2"` mise pin (root cause of the breakage — operator decision),
the exhaustruct deprecation warning (config keeps both names per 24-01), the
documented pre-existing -race failures (untouched, unskipped), the terse-reason
rationale table, and the gate-mechanism observations (>= threshold semantics,
per-function single-complexity-report cascade, preceding-line nolint
unreliability for func-level linters, latent-lint surfacing).

## Self-Check: PASSED

- Commits exist: cdb10a1, da54b07, 72932fb (git log verified; measured
  `git rev-list --count 0f06e1b..HEAD` = 3)
- .golangci.yml modified and verifies against the 2.13.2 schema
- Acceptance gate re-run at HEAD: cache-clean `mise run lint` → exit 0
- deferred-items.md exists in the task directory
