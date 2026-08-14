---
phase: 07-multi-provider-config-credentials
plan: 02
subsystem: cli-wiring
tags: [go, wiring, cli, credentials, factory, config]
requires:
  - phase: 07-01
    provides: "scheduler.ProviderFactory (Build/WarnUncredentialed/ResolveCredential), ProviderConfig.{APIKey,APIKeyEnv}, embedded default api_key_env: ZAI_API_KEY"
provides:
  - "factory-wired acp serve / tracer / parity (loadSchedulingFactory + setupProviderFactory seams)"
  - "D-07 startup uncredentialed-provider warnings + SC3 0600-permission warning (stderr)"
  - "SC1/SC4 proofs: TestEditorZeroEnv_LiteralInConfig + TestBackwardCompat_ZAIEnvOnly"
  - "(*scheduler.ProviderFactory).Endpoint cross-plan accessor"
  - "REQUIREMENTS.md PCFG-01..04 traceability (71 reqs) + ROADMAP.md Phase 7 finalization"
affects: [08-*, verify-work, ship gate]
tech-stack:
  added: []
  patterns:
    - "single provider-construction seam: every runtime path builds its provider via scheduler.ProviderFactory (D-08)"
    - "graceful degrade on operator-config errors: embedded-default fallback + stderr log (T-07-08)"
    - "advisory startup hygiene warnings to stderr: uncredentialed providers + loose config perms"
key-files:
  created:
    - cmd/ass-guard/provider_factory.go
    - cmd/ass-guard/provider_factory_test.go
  modified:
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/main.go
    - cmd/ass-guard/parity.go
    - cmd/ass-guard/acp_engine_e2e_test.go
    - internal/scheduler/factory.go
    - internal/scheduler/factory_test.go
    - .planning/REQUIREMENTS.md
    - .planning/ROADMAP.md
key-decisions:
  - "Factory Endpoint accessor added to 07-01's factory.go (flagged cross-plan edit): main.go reconstructs the AnthropicProvider with factory-RESOLVED base_url+key to attach the LOG-01 capturer — never hardcoded defaults"
  - "One shared setupProviderFactory seam for all three sites (load + degrade + resolve heavy tier); warnLooseConfigPerm stays acp-serve-only per the plan"
  - "Coverage reconciliation: Phase 7 distribution recorded as 4 (PCFG-01..04) with PROV-01 counted under Phase 1 — the plan's '5' would double-count and break the 71 total"
  - "e2e test fixture rewired to the factory seam so the plan's whole-dir bare-ctor grep is honestly zero"
patterns-established:
  - "Construction-site wiring: helper returns (factory, providerName) resolved via the validated scheduler resolver, defaulting to the first declared provider (sorted)"
  - "Lazy no-credential: uncredentialed Build keeps noCredentialProvider (typed structural error at first Send) — warn at startup, fail lazy (D-07)"
requirements-completed: [PCFG-02, PCFG-03, PCFG-04, PROV-01]

coverage:
  - id: D1
    description: "acp serve / tracer / parity delegate provider construction to scheduler.ProviderFactory (D-08); acp serve keeps zero direct NewAnthropicProvider calls"
    requirement: PCFG-03
    verification:
      - kind: unit
        ref: "cmd/ass-guard/provider_factory_test.go#TestLoadSchedulingFactory_WarnsUncredentialed, TestLoadSchedulingFactory_ZeroConfigEnv; TestProviderFactory_Endpoint"
        status: pass
      - kind: integration
        ref: "grep -rn 'NewAnthropicProvider(shaper.New())' cmd/ass-guard/ (zero matches); mise ci"
        status: pass
  - id: D2
    description: "D-07 startup warning naming uncredentialed providers on stderr, never refusing to start; SC3 0600-permission warning for loose scheduling.yaml"
    requirement: PCFG-02
    verification:
      - kind: unit
        ref: "cmd/ass-guard/provider_factory_test.go#TestLoadSchedulingFactory_WarnsUncredentialed, TestStartupWarn_ConfigPermLoose"
        status: pass
  - id: D3
    description: "Zero-config backward-compat (SC4): embedded default + $ZAI_API_KEY resolves Source=env; editor-zero-env (SC1): literal api_key resolves Source=config"
    requirement: PCFG-04
    verification:
      - kind: unit
        ref: "cmd/ass-guard/provider_factory_test.go#TestBackwardCompat_ZAIEnvOnly, TestEditorZeroEnv_LiteralInConfig"
        status: pass
  - id: D4
    description: "PROV-01 completed: factory builds per configured base_url + resolved key at every construction site (wire round-trip from 07-01 + factory wiring)"
    requirement: PROV-01
    verification:
      - kind: unit
        ref: "internal/scheduler/factory_test.go#TestProviderFactory_WireRoundTrip (07-01); TestProviderFactory_Endpoint (07-02)"
        status: pass

metrics:
  duration: 16m
  completed: 2026-08-14
  commits: 5
status: complete
estimate_tokens: 46000
actuals:
  tokens: 4583    # chars/4 over the realized diff (18334 added chars); estimate was 46000 — large over-estimate (plan confidence: low)
  tasks: 3
  commits: 5
---

# Phase 7 Plan 2: Factory Wiring into acp serve / tracer / parity Summary

The three hardcoded provider-construction sites now delegate to the Phase-7 `scheduler.ProviderFactory`: `acp serve` loads `.ass-guard/scheduling.yaml` overlaid on the embedded default, resolves the heavy tier through the validated scheduler resolver, and builds the credentialed provider per session; the LOG-01 tracer reconstructs its AnthropicProvider with factory-RESOLVED base_url+key to attach the RequestCapturer; parity builds the factory-driven heavy-tier provider. Startup now warns on stderr (never refuses to start) about uncredentialed providers (D-07) and about a group/world-readable scheduling.yaml (SC3 0600 hygiene), and the zero-config `$ZAI_API_KEY` flow plus the zero-env literal-in-config flow are both proven by tests. Traceability finalized: PCFG-01..04 + PROV-01 (71 v1 requirements, 0 unmapped).

## What was built

- **`loadSchedulingFactory(workDir, apiKeyFlag, stderr)`** (cmd/ass-guard/provider_factory.go) — the D-08 wiring seam: loads `.ass-guard/scheduling.yaml` when present (else embedded default), builds `scheduler.NewProviderFactory`, emits `WarnUncredentialed(stderr)`; load errors are returned for graceful degradation.
- **`setupProviderFactory(workDir, stderr)`** — shared by all three sites: degrades to the embedded default on operator-config errors (log to stderr, T-07-08), resolves the heavy tier via `scheduler.NewResolver(cfg).Resolve(tierHeavy, ...)`, falls back to the first declared provider (sorted) on resolve error.
- **acp_serve.go** — `makeProvider` is now `factory.Build(resolvedProvider, shaper.New())`; zero direct `NewAnthropicProvider` calls remain; the SC3 `warnLooseConfigPerm` check runs at startup.
- **main.go** — tracer builds via `tracerProvider(factory, providerName, capturer)`: when the built provider is a credentialed `*provider.AnthropicProvider`, it is reconstructed with factory-resolved base_url+key + `WithAnthropicRequestCapture`; uncredentialed keeps the lazy `noCredentialProvider` (typed structural error at first Send).
- **parity.go** — heavy-tier provider built through the factory (zero-config `$ZAI_API_KEY` env resolution applies).
- **`(*scheduler.ProviderFactory).Endpoint(providerName) (baseURL, key string, ok bool)`** — the flagged cross-plan accessor in internal/scheduler/factory.go (07-01 file; authorized edit) + `TestProviderFactory_Endpoint`.
- **`warnLooseConfigPerm(path, stderr)`** — one-line advisory warning recommending `chmod 0600` when the overlay config is looser than 0600 (SC3, T-07-05). The seed's 0644 write mode is unchanged (D-06: the seed carries no literal key).
- **Tests** — `cmd/ass-guard/provider_factory_test.go` with 5 cases (warns-uncredentialed, zero-config env, perm-loose, backward-compat, editor-zero-env).

## Success criteria

- **SC1** (multi-provider + file credentials + editor zero-env): proven by `TestEditorZeroEnv_LiteralInConfig` (Source=config, zero env, real adapter).
- **SC2** (correct instance per resolved pair): acp serve resolves the heavy tier and builds that provider's credentialed instance per session (Task 1).
- **SC3** (precedence + no-log + gitignore + 0600): precedence/no-log from 07-01; `.ass-guard/` gitignored from Phase 6; the 0600 startup warning (Task 2).
- **SC4** (zero-config backward-compat): proven by `TestBackwardCompat_ZAIEnvOnly` + `TestLoadSchedulingFactory_ZeroConfigEnv` ($ZAI_API_KEY-only flow preserved).
- **PROV-01**: completed — factory builds per configured base_url + resolved key at every construction site.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Fix] e2e test fixture had a bare hardcoded ctor that tripped the plan's whole-dir grep**
- **Found during:** Task 1 GREEN verification
- **Issue:** `grep -rn 'NewAnthropicProvider(shaper.New())' cmd/ass-guard/` found `acp_engine_e2e_test.go:314` (TestRunACPServe_NoEngineFlag fixture). The production invariant (three sites) was already met, but the plan's verify scans the whole dir.
- **Fix:** rewired the fixture's makeProvider to `factory.Build` via `setupProviderFactory(t.TempDir(), io.Discard)` — the provider is never invoked by that structural check, so no env coupling.
- **Files modified:** cmd/ass-guard/acp_engine_e2e_test.go
- **Commit:** 868c14c

**2. [Rule 1 - Bug] `TestProviderFactory_Endpoint` env collision ("zai" derives $ZAI_API_KEY from its name)**
- **Found during:** Task 1 GREEN
- **Issue:** provider name "zai" derives the env var `ZAI_API_KEY` (upper-cased + `_API_KEY`), so setting that var made the config literal lose (env > config — correct precedence, wrong test setup).
- **Fix:** the literal provider now declares `api_key_env: ZAI_API_KEY` forced empty; the env path uses a separate `OTHER_API_KEY` var.
- **Files modified:** internal/scheduler/factory_test.go
- **Commit:** 868c14c

**3. [Rule 1 - Bug] golangci-lint all-linters violations in new code (11 issues)**
- **Found during:** Task 2 GREEN
- **Issues:** funlen (runTrace 62>60), lll (3 long lines incl. test YAML), noinlineerr (2 inline-if error patterns), nonamedreturns (Endpoint signature), paralleltest (2 tests), unparam (apiKeyFlag), wrapcheck (2 unwrapped external errors), wsl_v5, gofmt (nolint placement).
- **Fix:** extracted `tracerProvider` helper (funlen), wrapped long lines + multi-line YAML capabilities (lll), plain assignments (noinlineerr), `//nolint:nonamedreturns` on Endpoint (plan-mandated signature), `t.Setenv` hygiene on the warns test + `t.Parallel()` on the perm test (paralleltest), `//nolint:unparam` on loadSchedulingFactory (plan-mandated D-05 flag seam), `%w` wrapping (wrapcheck), whitespace + nolint-comment placement (wsl_v5/gofmt).
- **Commits:** fac4d9c
- **Note:** Task-2 RED was a compile-failure gate: only `TestStartupWarn_ConfigPermLoose` needed new code, but the undefined `warnLooseConfigPerm` broke the whole package build (repo RED convention, same as 07-01).

### Acceptance-criteria adaptations

**4. Coverage counts reconciled (plan said "Phase 7 distribution = 5")**
- The plan's Task-3 wording "Phase 7 distribution = 5 (PCFG-01..04 + PROV-01 completion)" would double-count PROV-01 (its traceability row maps it to Phase 1, which stays 16) and break the plan's own "71 total" (16+18+6+19+5+3+5 = 72). Recorded `Phase 7 = 4 (PCFG-01..04; also completes PROV-01, counted under Phase 1)` so the distribution sums to 71.

**5. Task-3 traceability was verify+reconcile, not add**
- Per the run_notes, the 07-01 executor already added the PCFG-01..04 entries (§Providers + matrix rows + Complete statuses) and PROV-01 Complete. This plan verified those against the final implementation (all four are now genuinely complete: 07-01 for schema/precedence/default, this plan for wiring + proofs), annotated PROV-01 with "(completed by Phase 7)", and updated the coverage counts + last-updated line. No duplicates added.

**6. Shared `setupProviderFactory` seam**
- The plan described the degrade-on-error + heavy-tier-resolve inline in acp_serve (and mirrored in main/parity). Extracted a shared helper so all three sites degrade identically (T-07-08); `warnLooseConfigPerm` is called from runACPServe directly per the plan's Task-2 wiring instruction (also satisfies the acceptance grep on acp_serve.go).

## Decisions Made

- Endpoint accessor (flagged cross-plan edit) chosen over a capturer setter or Build variadic: the tracer reconstructs with factory-resolved values, keeping the factory's Build interface untouched.
- Uncredentialed tracer builds keep `noCredentialProvider` (lazy typed error) rather than hard-failing — consistent with D-07.
- parity hard-fails only if the embedded default itself fails (impossible in practice); operator-config errors degrade to the embedded default + stderr log, same as serve/tracer.

## TDD Gate Compliance

Two complete RED → GREEN cycles, verified in git log:
1. `test(07-02): add RED tests for factory wiring seam + zero-config env` (b1ba77d) — compile-failure RED
2. `feat(07-02): wire scheduler.ProviderFactory into acp serve/tracer/parity` (868c14c) — GREEN
3. `test(07-02): add RED tests for 0600 perm warn + backward-compat + zero-env` (c2b8a18) — compile-failure RED
4. `feat(07-02): add 0600 config-perm startup warning + SC1/SC4 proofs` (fac4d9c) — GREEN

Both cycles comply. In cycle 2, `TestBackwardCompat_ZAIEnvOnly` + `TestEditorZeroEnv_LiteralInConfig` would have passed immediately (they exercise Task-1 helpers) — the RED gate was the whole-package compile failure, the repo convention.

## Verification

- `mise ci` (go vet + golangci-lint all-linters + CGO_ENABLED=0 build + `go test -race -count=1 ./...`) — green, 0 lint issues, all packages pass (including internal/profile, whose previously-documented blocker did not reproduce).
- `go build ./cmd/ass-guard` — 0.
- `grep -rn 'NewAnthropicProvider(shaper.New())' cmd/ass-guard/` — zero matches (bare ctor gone from the whole dir).
- `grep -rn 'loadSchedulingFactory\|NewProviderFactory\|factory.Build' cmd/ass-guard/{acp_serve.go,main.go,parity.go}` — factory used at all three sites.
- No-log gate: warning tests assert the key never appears in any warning buffer (T-07-06); no credential reaches stdout.
- End-to-end manual turn with a real key in `.ass-guard/scheduling.yaml` spawned by Zed — operator-gated, NOT an autonomous gate (per the plan's surfaced assumption; the construction-level zero-env test is the autonomous proof).

## Known Stubs

None. `noCredentialProvider` is intentional D-07 lazy behavior, not a stub.

## Threat Flags

No new surface beyond the plan's registered register (T-07-05..T-07-SC): `warnLooseConfigPerm` only stats the file (no new file-access patterns at a trust boundary), the D-07/0600 warnings write to stderr only, `Endpoint` returns the resolved key to in-process callers already trusted by the factory contract (key never logged), no new network endpoints, no new dependencies (T-07-SC n/a).

## Self-Check

- Files exist: cmd/ass-guard/provider_factory.go, cmd/ass-guard/provider_factory_test.go, internal/scheduler/factory.go (Endpoint), acp_serve.go/main.go/parity.go wiring — FOUND
- Commits: b1ba77d, 868c14c, c2b8a18, fac4d9c, 6875515 — FOUND

## Self-Check: PASSED
