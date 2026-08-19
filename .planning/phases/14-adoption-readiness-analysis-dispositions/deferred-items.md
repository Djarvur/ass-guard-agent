# Phase 14 — Deferred Items (out-of-scope discoveries)

Executor log of pre-existing issues observed during plan execution that are NOT
fixed in-phase (scope boundary: only issues caused by the current task's changes
are auto-fixable). One entry per discovery.

## [2026-08-19] cmd/ass-guard live-serve test flake under full-suite load (observed during 14-03 Task 3 verify)

- **Observed:** first `mise run ci` of the session failed 2 tests in
  `cmd/ass-guard`: `TestServeAudit_RequestShapedThroughRealSeam`,
  `TestServeMirror_Override` (10s timeouts each).
- **Diagnosis:** both PASS in isolation (`go test ./cmd/ass-guard/ -run
  'TestServeAudit_RequestShapedThroughRealSeam|TestServeMirror_Override'
  -count=1` → ok) and in two subsequent full `mise run ci` runs (exit 0). This is
  the documented load-dependent timing flake — STATE.md's 14-01 entry records
  `TestServeMirror_Override`'s early-return race history ("2/10 → 0/15" after
  1b38e3d-era fixes; still load-sensitive).
- **Out of scope because:** 14-03's diff touches only `.gitignore` and
  `docs/shaper-pi-audit.md` (zero Go files); the flake predates this plan.
- **Suggested route:** the 12-08 eval net (post-adoption) or a dedicated
  live-serve stability pass; not Phase 14 scope.

- **[14-06, 2026-08-19 — RESOLVED, not a flake] transient cmd/ass-guard full-suite failure during
  execution:** `TestLoadSchedulingFactory_LegacyNameNeverRead` failed once under
  `go test -race -count=1 ./...` (expected "glm-5.2", got "GLM-5.3"), and one stash-verification
  run also saw `TestIntegration_RealStreamingThroughACP` fail. **Root cause (identified at
  close-out):** the OPERATOR concurrently committed `8c7e6e9` (seed sync to the 09-04 re-pin —
  changes the scheduler floor to GLM-5.3 AND the test expectation in provider_factory_test.go)
  and `3ef79ec` (scheduler heavy primary) at 20:08 UTC, mid-execution — the failing run compiled
  a tree caught between the two edits (new floor values + old test expectation). NOT a flake and
  NOT caused by 14-06's changes (leaf packages toolcat/toolexec, no config-layer paths). Both
  tests pass on the coherent post-commit tree; the final `mise run ci` (exit 0) ran against it.
  The earlier 14-03 entry below remains a separate genuine load-flake record.
