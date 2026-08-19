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

- **[14-06, 2026-08-19] cmd/ass-guard full-suite `-race` flake (pre-existing, third data point):**
  `TestLoadSchedulingFactory_LegacyNameNeverRead` failed once under `go test -race -count=1 ./...`
  (expected "glm-5.2", got "GLM-5.3" — as if the HOME pin missed a layer). Same run class also
  produced `TestIntegration_RealStreamingThroughACP` failing once on the CLEAN parent commit
  (verified via stash — zero 14-06 changes present). Both pass in isolation, in `-count=10`
  isolation runs, and in single-package `-race` runs. **Diagnosis:** the same load-dependent
  cmd/ass-guard flake family 14-03 documented (STATE.md 14-01 entry records the
  `TestServeMirror_Override` history). **Out of scope because:** 14-06's diff touches
  internal/toolcat + internal/toolexec only — leaf packages with no config-layer or
  serve-path code paths. **Suggested route:** unchanged (12-08 eval net / live-serve stability pass).
