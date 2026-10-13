# Deferred Items — 261013-uhw

- **[2026-10-13, Task 2] `mise run ci` lint leg fails repo-wide — pre-existing baseline, out of scope (NOT fixed).**
  The full gate was run from the repo root per the plan's verify step: vet, build, and
  kit-boundary legs pass; the LINT leg fails with 653 findings concentrated in kit/
  (runtime.go 58, plus kit test files) — zero findings in internal/perm or any file this
  plan touched (package-local `golangci-lint run internal/perm/...` on a cache-clean
  linter: 0 issues). This is the documented STATE.md baseline drift ("Repo-wide
  golangci-lint 2.13.2 still reports ~572 findings on pre-Phase-24 files"; 25-02 recorded
  599 at its baseline) grown further by later kit/ moves — `mise run ci` has not exited 0
  on lint at master for this reason regardless of this plan's change. Fixing 653
  pre-existing findings is outside this quick item's file scope
  (internal/perm/rules.go + its test files). The test -race leg was run separately to
  completion for the same reason (see SUMMARY).
