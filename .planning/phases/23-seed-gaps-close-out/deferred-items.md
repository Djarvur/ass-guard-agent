# Deferred Items — Phase 23

Out-of-scope discoveries logged per the executor scope boundary (pre-existing,
not caused by 23-07; do not fix inside this phase).

## 23-07 execution (2026-09-11)

### D-23-07-1: Three internal/acpserve simulator batteries fail on this machine — pre-existing, environment-dependent

- **Tests:** `TestZedSimulatorE2E`, `TestSimulatorCommandSurface` (fail with and
  without `-race`), `TestPermissionsE2E` (fails only under `-race`).
- **Verified pre-existing:** all three reproduce byte-identically on clean HEAD
  of this phase (`acdb233`, the 23-07 RED commit — test-only) in a detached
  worktree, with the same `-race` flags. The 23-07 production change introduces
  zero new failures (full `-race` run fails exactly this triple, nothing else).
- **Signatures:** `simulator: timed out waiting for a frame (seen=1)` /
  `(seen=11)` — the harness stalls waiting for a scripted frame.
- **Suspected environment interaction (not fully diagnosed, out of scope):**
  the operator's `~/.claude/settings.json` injects
  `ANTHROPIC_BASE_URL`/`ANTHROPIC_API_BASE_URL`=`http://127.0.0.1:3456` (a live
  local gateway, confirmed listening). The simulator tests point their
  per-test `config.yaml` at an `httptest` stub, but if the provider factory
  lets the env override the config base URL, the scripted provider traffic is
  captured by the real gateway and the scripted phases never match. Zed-sim
  stage 1 also depends on a `config_option_update` frame whose blob fill can
  be short-circuited by user-level config layers.
- **Action for a later phase:** scrub `ANTHROPIC_BASE_URL`-family env in the
  simulator/permissions harnesses (the `TestServeAudit` `t.Setenv` precedent)
  or make the provider factory precedence test-overridable; re-baseline on a
  clean environment before treating these batteries as gating.

### D-23-07-2: TestModesMatrixInteractiveSurfaces flaked once under full-suite -race

- **Observed:** failed in 1 of 3 full `go test ./internal/acpserve/ -race`
  runs during 23-07 Task 2 verification; passed in the other 2 full runs and
  5/5 under `-run 'TestModesMatrix' -race -count=5` (isolated).
- **Not 23-07's advertisement:** the modesmatrix harness is order-tolerant by
  construction — `simStageSessionNew`/`matrixPromptTurn` read via
  `nextResponse` (notifications skipped) and `simStageInitialize`'s loop has
  no fail-on-unknown default; the stray post-response advertisement frame is
  admitted everywhere.
- **Likely cause:** the harness's 10s `simGuardTimeout` frame wait under -race
  with the full suite's parallel load (the same deferred family as
  D-23-07-1).
