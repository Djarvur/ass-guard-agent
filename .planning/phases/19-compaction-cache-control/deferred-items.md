# Phase 19 Deferred Items (out-of-scope discoveries)

## [2026-09-06, 19-03] mise lint gate broken repo-wide by golangci-lint version drift

The `mise run lint` gate fails on every package (including ones untouched since
Phase 15) in the current environment:

- mise resolves `golangci-lint = "2"` to **2.13.2** (built with go1.27.0);
  `.golangci.yml` was tuned for **2.12.x** (STATE's research note pins 2.12.2).
- 2.13.2 deprecates `exhaustruct` in favor of `exhaustruct_v5`, so the config's
  wildcard exclusion (`- linters: [exhaustruct] text: ".*"`) no longer matches —
  the new linter flags every partial struct literal in the repo (100+ findings).
- New/renamed linters (`noinlineerr`, `stringsseq`, `slicesbackward`, `wsl_v5`,
  `tparallel`) fire on large amounts of pre-existing code that passed 2.12.x.
- The stale `~/go/bin/golangci-lint` (2.12.2, built go1.26) cannot typecheck
  against the installed go1.27.1 toolchain at all (panics: "file requires newer
  Go version").

Repro: `mise exec -- golangci-lint run ./internal/shaper/...` (untouched by
phases 19-21) shows the same failures.

Suggested fix (config/tooling reconciliation, one commit): pin the linter minor
in `.mise.toml` (e.g. `golangci-lint = "2.12"` or the latest 2.13.x) and update
`.golangci.yml` exclusions for the renamed linters, or migrate the config
forward to 2.13.x deliberately.

## [2026-09-06, 19-03] internal/runtime full-package flakes under machine load

`go test -race ./internal/runtime/ -count=1` intermittently fails (reproduced
at the pre-19-03 commit 00c9da6, so NOT caused by 19-03):

- `TestIntegration_RealStreamingThroughACP` (4.3s fail / 0.4s pass in isolation)
- `TestAskPark_PromptResponsePrecedesResolution` (13-15s, deadline-style)

Both pass in isolation. The host was under sustained load during 19-03's
repeated full-suite verification runs. Worth a dedicated timing-deadline review
next time `internal/runtime` is touched.

## [2026-09-06, 19-03] gate_test.go allow_once race — FIXED in-plan

`TestGateOutcomeMatrix/allow_once_executes_without_persisting` read
`toolResultsFor` without the `gateWaitFor` its sibling subtests use; 19-03's
added parallel batteries widened the scheduling window to ~50% full-suite
flake. Fixed in a5a46a1 (documented as a deviation in 19-03-SUMMARY.md).
