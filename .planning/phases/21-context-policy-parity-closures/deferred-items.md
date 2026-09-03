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

## 21-03 (2026-09-03)

- **The 21-02 runtime timing flake recurred under `mise ci` load (pre-existing,
  worktree-proven):** `TestAskPark_PromptResponsePrecedesResolution` failed in
  2 of 3 full `mise ci`/`go test ./...` passes; `TestAskWiring_ServerLevelSurface`
  and the plan-mode wiring tests joined it in one standalone back-to-back loop.
  ALL pass in isolation and in 4/6 standalone package runs. A worktree check at
  the PRE-PLAN commit e7affc4 (before any 21-03 change) reproduces the same
  failures — pre-existing machine-load flakes, not 21-03 regressions. The 21-03
  diff touches runtime only additively (one extra forwarder channel + one
  routeBusEvent case). Left for the owning suite; same candidates as the 21-02
  entry (widen bounds or load-aware skip).
  - Same class, other packages (both ALSO reproduce at pre-plan e7affc4):
    `internal/session TestGateOutcomeMatrix` (0.00s failure, 1-in-3 at the
    pre-plan commit; passes 3/3 post-21-03 standalone) and
    `internal/checkpoint` (one 664s load-starved full-suite pass; 3/3 green
    standalone at pre-plan and post-21-03).
- **`mise ci` lint step still red (WINDOWS #16, unchanged):** filtered run
  shows ZERO findings in every file 21-03 touched (streaming.go,
  goconst_constants.go, events.go, session.go, projector.go, shaper.go,
  provider.go, runtime.go, cron_wiring.go + their tests); the red is the
  ledgered exhaustruct_v5 drift elsewhere.
