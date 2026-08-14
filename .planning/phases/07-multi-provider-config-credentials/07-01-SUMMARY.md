---
phase: 07-multi-provider-config-credentials
plan: 01
type: execute
subsystem: scheduler
tags: [go, config, scheduler, provider, credentials]
requires: [03-model-scheduling, 06-distribution-polish]
provides: [07-02-PLAN.md]
affects: [cmd/ass-guard, internal/scheduler, internal/defaults]
tech-stack:
  added: []
  patterns:
    - "lazy credential resolution at factory-construction time (D-02/D-06/D-07 reconciliation)"
    - "warn-not-reject load validation (D-04) via ValidateWithWarnings + stderr surfacing"
    - "typed structural error for uncredentialed providers (D-07), classified like 401/403"
key-files:
  created:
    - internal/scheduler/factory.go
    - internal/scheduler/factory_test.go
    - internal/scheduler/testdata/providers-cred.yaml
  modified:
    - internal/scheduler/config.go
    - internal/scheduler/load.go
    - internal/scheduler/load_test.go
    - internal/scheduler/defaults/scheduling.yaml
    - internal/defaults/seed/scheduling.yaml
decisions:
  - "Credentials resolve lazily at factory-construction time (flag > env > config), never at load — an unset \${VAR} or missing field makes the provider uncredentialed (warn + lazy typed error), not a load fatal (07-CONTEXT surfaced assumption D-02 vs D-06/D-07)"
  - "api_key_env and \${VAR} carry identical lazy semantics (surfaced assumption: strict load-fatal form is a one-line change later if wanted)"
  - "Validate delegates to ValidateWithWarnings so existing Validate callers are untouched"
metrics:
  duration: 10m
  completed: 2026-08-14
  commits: 8
status: complete
estimate_tokens: 56000
actuals:
  tokens: 5513    # chars/4 over the realized diff (22051 added chars); estimate was 56000 — large over-estimate, plan ran as 3 TDD tasks
  tasks: 3
  commits: 8
---

# Phase 7 Plan 1: Config Schema + Credential Resolver + Provider Factory Summary

Credential-aware scheduling config: `ProviderConfig` gains `api_key`/`api_key_env`, a pure `ResolveCredential` implements the locked D-05 precedence (flag > provider env var > config literal, with `${VAR}` expansion), and `ProviderFactory.Build` constructs the correct credentialed Anthropic/OpenAI adapter per declared provider — proven by a precedence table, a construction assertion, a lazy typed no-credential error, and a wire-level round-trip showing the resolved key + configured base_url on the wire. The embedded zero-config default declares `api_key_env: ZAI_API_KEY` (no literal) in both byte-identical copies, and load-time validation warns (never rejects) on credential-less providers while preserving Phase-3 D-10 shape/ref rejection.

## What was built

- **PCFG-01 (schema + loader + validation):** `ProviderConfig.{APIKey,APIKeyEnv}` (`yaml:"api_key"`/`yaml:"api_key_env"`); the loader parses them via the existing deep-merge path; `ValidateWithWarnings(cfg) ([]string, error)` collects the D-04 credential WARN, `Validate` delegates unchanged for existing callers; `Load` prints warnings to stderr (transport discipline, never stdout).
- **PCFG-02 (precedence + expansion + no-log + missing-credential):** `ResolveCredential` implements flag > env (explicit `api_key_env` or derived `<PROVIDER>_API_KEY`) > config literal; `${NAME}` expansion only (malformed `${` without `}` treated as literal — T-07-03); the Key never reaches any log path (grep gate verified; `ProviderError.Error()` already routes through `redact.ScrubError`). An uncredentialed provider yields `noCredentialProvider`, a lazy wrapper whose Send/Stream returns `*provider.ProviderError{Kind: KindStructural}` naming the provider + the env var (D-07, classified like 401/403, never retried).
- **PCFG-03 (factory):** `ProviderFactory` + `NewProviderFactory(cfg, flagKey, log)` + `Build(providerName, shaper)` constructs `provider.NewAnthropicProvider(sh, WithAnthropicBaseURL, WithAnthropicAPIKey)` / `NewOpenAIProvider(WithOpenAIBaseURL, WithOpenAIAPIKey)`; `WarnUncredentialed(w)` emits the startup-warn lines for Plan 07-02.
- **PCFG-04 (zero-config default):** both `internal/scheduler/defaults/scheduling.yaml` and `internal/defaults/seed/scheduling.yaml` carry `api_key_env: ZAI_API_KEY` with no literal (byte-identical — drift guard green). `$ZAI_API_KEY`-only flow unchanged.
- **PROV-01 (configurable base URL):** `TestProviderFactory_WireRoundTrip` proves a factory-built provider POSTs to the configured base_url host with path `/v1/messages` and `X-Api-Key` == the resolved key.

## New exported symbols (Plan 07-02 consumption contract)

- `scheduler.ResolvedCredential{Source, Key}` (Source: `flag`|`env`|`config`|`""`) and `scheduler.ResolveCredential(prov ProviderConfig, providerName, flagKey string) ResolvedCredential`
- `scheduler.ProviderFactory`, `scheduler.NewProviderFactory(cfg *Config, flagKey string, log *slog.Logger) *ProviderFactory`, `(*ProviderFactory).Build(providerName string, sh *shaper.Shaper) (provider.Provider, error)`, `(*ProviderFactory).WarnUncredentialed(w io.Writer)`
- `scheduler.ValidateWithWarnings(cfg *Config) ([]string, error)` (Validate unchanged)
- Config fields: `ProviderConfig.APIKey`, `ProviderConfig.APIKeyEnv`
- Embedded default change: `api_key_env: ZAI_API_KEY` on the anthropic provider (both copies)

## Test coverage

| Test | Proves |
| ---- | ------ |
| `TestResolveCredential_Precedence` | D-05 table: flag > env > config, `${VAR}` expansion, derived env name, malformed brace literal, uncredentialed (no panic) |
| `TestProviderFactory_BuildConstruction` | `Build("zai")` → `*provider.AnthropicProvider`, `Build("oai")` → `*provider.OpenAIProvider` from `testdata/providers-cred.yaml` |
| `TestProviderFactory_NoCredentialLazy` | uncredentialed Build is not an error; first Send yields `KindStructural` naming provider + env var |
| `TestProviderFactory_WarnUncredentialed` | warning output shape; never prints the key |
| `TestProviderFactory_WireRoundTrip` | X-Api-Key == resolved key, request Host == configured base_url host, path `/v1/messages` |
| `TestValidateWarns_NoCredentialField` / `TestValidate_NoWarnWhenAPIKeyEnvDeclared` | D-04 warn-not-reject; `api_key_env` presence stays silent |
| `TestValidate_StillRejectsBadShape` | D-10 shape rejection regression guard |
| `TestDriftGuard_SchedulingYAML` (existing) | seed == scheduler default byte-identical after dual edit |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Fix] gofmt field alignment breaks an acceptance grep pattern**
- **Found during:** Task 1
- **Issue:** `gofmt` aligns struct fields (`APIKey    string` with 4 spaces), so the plan's acceptance grep `grep -c 'APIKey string'` returns 0.
- **Fix:** none needed in code (fields exist and carry the right yaml tags); the acceptance check was verified with `grep -c 'APIKey.*string'` (returns 2) instead. Recorded so the verifier does not trip on the pattern.
- **Files modified:** internal/scheduler/config.go (gofmt-aligned)
- **Commit:** 07b2cba

**2. [Rule 1 - Bug] t.Setenv panic in parallel precedence test**
- **Found during:** Task 1 GREEN
- **Issue:** `TestResolveCredential_Precedence` called `t.Parallel()` while subtests use `t.Setenv` (forbidden — runtime panic).
- **Fix:** removed `t.Parallel()` from that test (env-dependent tests must not run parallel).
- **Files modified:** internal/scheduler/factory_test.go
- **Commit:** 07b2cba

**3. [Rule 1 - Bug] Lint gate violations in the wire round-trip test (lll, wsl_v5, canonicalheader)**
- **Found during:** Task 2
- **Issue:** three separate lint violations (2 long lines, 1 whitespace, 1 non-canonical header name).
- **Fix:** wrapped lines, added required blank line, `Content-Type` canonical header.
- **Files modified:** internal/scheduler/factory_test.go
- **Commits:** 0816adf, ffdabbc

**4. [Rule 3 - Fix] wsl_v5 "never cuddle decl" + lll on ValidateWithWarnings**
- **Found during:** Task 3 GREEN
- **Issue:** cuddled `var` declarations after the function brace; nolint comment pushed the signature line past 120 chars.
- **Fix:** merged into a `var (...)` block; moved nolint to its own line.
- **Files modified:** internal/scheduler/load.go
- **Commit:** b0e2cfe

### Acceptance-criteria adaptations (estimate.confidence: low)

- Task 1 grep pattern adapted for gofmt alignment (see deviation 1 above).
- Task 2 TDD RED phase used the drift guard as the failing gate: the wire round-trip test passed immediately (Task 1 had already built the base_url/key seam), so the RED commit shipped the scheduler-side default edit alone and proved `TestDriftGuard_SchedulingYAML` catches the one-sided edit (906 vs 935 bytes). GREEN mirrored the line into the seed.
- Task 3 added an extra test beyond the plan's list (`TestValidate_NoWarnWhenAPIKeyEnvDeclared`) because the plan's `<behavior>` required "api_key_env set → no warn even if env unset" — asserting it directly.

## Decisions Made

- Lazy credential resolution at factory-construction time (not load time) — the 07-CONTEXT surfaced-assumption reconciliation (D-02 strict-load-fatal would break D-06 zero-config); recorded in commit 07b2cba.
- `ValidateWithWarnings` delegation keeps `Validate`'s signature stable for existing callers (`scheduling resolve` CLI, first-run validation).

## TDD Gate Compliance

RED → GREEN gate sequence verified in git log:
1. `test(07-01): add RED tests for credential resolver + provider factory` (be9481e) — compile-failure RED, repo convention
2. `feat(07-01): implement credential resolver + provider factory` (07b2cba) — GREEN
3. `test(07-01): add wire round-trip test + D-06 default ... (drift guard red)` (c49b8dc) — RED
4. `feat(07-01): mirror api_key_env into seed default ...` (5d3e53f) — GREEN
5. `test(07-01): add RED tests for D-04 load-time credential WARN` (8b73c7f) — RED
6. `feat(07-01): D-04 load-time credential WARN ...` (b0e2cfe) — GREEN

All three TDD cycles comply. No tests pass unexpectedly during RED except the Task-2 wire round-trip (by design — the factory seam pre-existed from Task 1; the drift guard was the actual red gate).

## Verification

- `go test ./internal/scheduler/... ./internal/defaults/ -race -count=1` — green
- `mise ci` (vet + golangci-lint all-linters + CGO_ENABLED=0 build + `go test -race ./...`) — green, 0 lint issues
- `diff internal/scheduler/defaults/scheduling.yaml internal/defaults/seed/scheduling.yaml` — empty
- `grep -rn 'sk-[A-Za-z0-9]' internal/scheduler/testdata/providers-cred.yaml` — only the deliberate fake literal `sk-test-literal`
- No-log grep gate: no factory.go line prints `cred.Key`

## Known Stubs

None. `noCredentialProvider` is intentional D-07 lazy behavior, not a stub.

## Threat Flags

None beyond the plan's registered register (T-07-01..T-07-SC): no new network endpoints, auth paths, or file-access patterns. T-07-01 mitigation verified by the no-log grep gate; T-07-03 mitigated by `${NAME}`-only expansion; no new dependencies (T-07-SC n/a).

## Self-Check

- Files exist: factory.go, factory_test.go, testdata/providers-cred.yaml, load.go changes, both yaml defaults — FOUND
- Commits: be9481e, 07b2cba, c49b8dc, 5d3e53f, 0816adf, ffdabbc, 8b73c7f, b0e2cfe — FOUND

## Self-Check: PASSED
