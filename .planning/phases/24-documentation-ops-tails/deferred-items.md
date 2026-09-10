# Deferred Items — Phase 24

## Deferred Items

- TestEscalation_ReapAllUsesLadder (internal/coreexec/background_test.go:639) flakes under full-suite parallel load
  status: open
  **What:** observed failing once in a full `go test -race ./...` run (24-01), passing 5/5 in isolation in both the 24-01 tree and the pre-plan base commit (59952f8), and passing in the base full-suite run — a PTY TERM-delivery timing flake, not a regression (coreexec does not import modelrouting).
- gocritic hugeParam on internal/modelrouting/factory.go:45 and :72 (pre-existing baseline)
  status: open
  **What:** surfaced once the golangci 2.13 baseline repair (24-01, ce424ea) made the package lint runnable; factory.go is untouched by 24-01 and the findings predate it — fix belongs to the next plan touching factory.go.
- Repo-wide golangci-lint 2.13.2 still reports ~572 findings on pre-Phase-24 files after the 24-01 baseline repair
  status: open
  **What:** residual new-linter-version findings (runtime.go, config_surface.go, coreexec tests, ...) — the 24-01 repair only restored the config's INTENDED exclusion/disable set; per-file cleanup is ongoing baseline work owned by whichever plan next touches each file.
- `mcp.Start` silently skips a server that fails to spawn — the doc comment says "logged via slog; non-fatal" but no logging exists
  status: open
  **What:** observability gap found empirically during the 24-03 dry-run (a PATH-unresolvable `mcp-language-server` vanished with zero stderr evidence); `internal/mcp/host.go` `Start`'s skip branch (`connectOne` error → `continue`) logs nothing. The session degrades as designed (T-5-04) but an operator cannot tell why a configured server contributed no tools. Candidate fix: one slog warning in the skip branch. Out of scope for 24-03 (documentation-only plan, operator decision 2026-08-25).

- golangci-lint 2.13.2 findings in packages 24-04 touched but files it did not modify: internal/paritycli/parity_test.go:446 (lll, 121 chars) and cmd/ass-guard/acp_serve.go:358 (wrapcheck)
  status: open
  **What:** pre-existing findings surfaced when 24-04 linted its touched packages — both files predate this plan (last touched 19-01 / earlier) and the findings are unrelated to the nightly-check additions; every file 24-04 created or modified lints clean under 2.13.2. Fix belongs to the next plan touching each file.
