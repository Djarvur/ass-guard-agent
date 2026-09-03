# Phase 21 Deferred Items

Out-of-scope discoveries logged per the executor scope boundary (observed,
not fixed — pre-existing, unrelated to the plan's changed files).

## 21-02 (2026-09-03)

- **Timing-sensitive flakes in untouched packages (load-only, 1-in-3 runs):**
  - `internal/runtime/ask_wiring_test.go TestAskPark_PromptResponsePrecedesResolution`
    failed once under the full-suite load bar ("response took 10.21s; want
    near-instant" — its 10s bound). Passes in isolation and on every re-run.
    The 21-02 diff adds only a handful of stat calls per session construction.
  - `internal/acpserve/simulator_e2e_test.go TestZedSimulatorE2E` failed once
    in a full `mise run test` pass (pipe-based simulator over a stub HTTP
    provider). Passes in isolation and on the full-package re-run (2/2 green).
  Both are machine-load flakes in files 21-02 does not touch; left for their
  owning suites (candidates: widen the bounds or mark `testing.Short()` skips
  under parallel load).
- **`mise ci` lint step red — pre-existing, ledgered** (WINDOWS #16): PATH
  golangci-lint 2.12.2 panics on the go1.27 stdlib; mise's 2.13.2 runs but
  exhaustruct_v5 flags ~2293 repo-wide findings the config's `exhaustruct`
  exclusion rule no longer matches (linter renamed to `_v5`). The 21-02
  files are clean apart from that linter (verified by filtered run).
