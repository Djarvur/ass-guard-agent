---
phase: 07-multi-provider-config-credentials
verified: 2026-08-14T12:30:00Z
status: passed
score: 14/14 must-haves verified
behavior_unverified: 0
overrides_applied: 0
gaps: []
human_verification:
  - test: "Operator-gated live turn (optional per 07-02-PLAN surfaced assumption): put a real api_key literal in .ass-guard/scheduling.yaml (or export it into the editor's launch env), spawn `ass-guard acp serve` from an editor (Zed) with ZERO shell env vars, and make one real model turn."
    expected: "The turn authenticates using the file credential (Source=config) with no environment; the request goes to the configured base_url with the resolved key; no credential appears on stdout or in logs."
    why_human: "External service integration against the live Z.ai/OpenAI API requires a real key (operator setup). The autonomous proof (TestEditorZeroEnv_LiteralInConfig + TestProviderFactory_WireRoundTrip) proves the full path config→factory→adapter→wire with zero env against an httptest server, but a live end-to-end model turn is an external-service behavior no automated test can exercise."
---

# Phase 7: Multi-Provider Config & Credentials — Verification Report

**Phase Goal:** An operator can declare multiple model providers in a single config — each with its base URL, protocol shape (anthropic/openai), and credential — and multiple models per provider with capability/pricing metadata. ass-guard resolves a scheduler tier to a fully-credentialed provider+model instance at request time. Credentials read from config so editor-spawned processes (Zed → `ass-guard acp serve`) work with zero env vars, while an explicit env var (or CLI flag) still overrides for CI/operators. This completes the multi-provider promise of PROV-01 and connects SCHED tier-resolution to real provider instances.
**Verified:** 2026-08-14
**Status:** passed — UAT 1/1 (live zero-env editor-spawned turn authenticated from the config-file credential, 2026-08-14); 14/14 automated truths verified
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A scheduling.yaml declaring >=2 providers (one anthropic-shape + one openai-shape) each with base_url + shape + resolvable credential LOADS without error; `ProviderFactory.Build(<name>)` returns the correct adapter type constructed with the provider's configured base_url | ✓ VERIFIED | `internal/scheduler/testdata/providers-cred.yaml` (zai + oai, 2 models each, tier fallback crossing providers); `TestProviderFactory_BuildConstruction` asserts `*provider.AnthropicProvider` / `*provider.OpenAIProvider`; `TestProviderFactory_WireRoundTrip` asserts base_url honored on the wire — all pass under `-race` |
| 2 | Credential precedence observable + table-tested: flag > provider env var > config literal; winning source recorded (flag\|env\|config) | ✓ VERIFIED | `factory.go ResolveCredential` (lines 45-67) implements exact D-05 order incl. `${VAR}` expansion + derived `<PROVIDER>_API_KEY`; `TestResolveCredential_Precedence` (8-case table incl. flag-beats-env-and-config, dollar-brace unset, malformed-brace literal) passes |
| 3 | `api_key_env: VAR` / `api_key: ${VAR}` resolved from env; uncredentialed provider does NOT make `Load()` fatal; first Send/Stream yields `*provider.ProviderError{Kind: Structural}` naming provider + env var | ✓ VERIFIED | `noCredentialProvider` wrapper (factory.go 191-218) returns `KindStructural` lazily; `TestProviderFactory_NoCredentialLazy` (KindStructural + names env var) and `TestLoadSchedulingFactory_WarnsUncredentialed` (load not fatal) pass |
| 4 | Embedded zero-config default declares anthropic provider with `api_key_env: ZAI_API_KEY` and NO literal key | ✓ VERIFIED | Both `internal/scheduler/defaults/scheduling.yaml` and `internal/defaults/seed/scheduling.yaml` carry `api_key_env: ZAI_API_KEY`, no literal; byte-identical (`diff` empty); `TestDriftGuard_SchedulingYAML` green |
| 5 | Wire receives resolved key + configured base_url (host == configured base_url host; X-Api-Key == resolved key) | ✓ VERIFIED | `TestProviderFactory_WireRoundTrip` asserts path `/v1/messages`, request Host == httptest server host, X-Api-Key == "sk-wire-secret" — behavioral wire-level proof, passes under `-race` |
| 6 | No credential string reaches a log/stdout path; `ProviderError.Error()` routes Cause through `redact.ScrubError`; `ResolveCredential` never logs the key | ✓ VERIFIED | `internal/provider/errors.go:54-57` scrubs Cause; grep gate: no factory.go line prints `cred.Key` (line 179 Fprintf writes provider name + env-var name only); `TestProviderFactory_WarnUncredentialed` + `TestLoadSchedulingFactory_WarnsUncredentialed` assert no key in output |
| 7 | Load-time validation warns (stderr), never rejects, on provider declaring neither api_key nor api_key_env; still rejects unknown shape + dangling provider ref | ✓ VERIFIED | `ValidateWithWarnings` (load.go 266-372, warn at 352-355); `Load` prints warnings to stderr; `TestValidateWarns_NoCredentialField`, `TestValidate_NoWarnWhenAPIKeyEnvDeclared`, `TestValidate_StillRejectsBadShape` all pass |
| 8 | `ass-guard acp serve` loads `.ass-guard/scheduling.yaml` overlaid on embedded default, builds `scheduler.ProviderFactory` once at startup, resolves heavy tier, session provider is factory-built credentialed instance; hardcoded makeProvider gone | ✓ VERIFIED | `acp_serve.go:256` `setupProviderFactory` → `makeProvider` closure `factory.Build(providerName, shaper.New())` (272-275) → `session.Session.Provider` (line 554); `provider_factory.go:75` resolves tierHeavy via `scheduler.NewResolver(cfg).Resolve`; bare-ctor grep zero matches in cmd/; e2e `TestRunACPServe_NoEngineFlag` green |
| 9 | Operator declares >=2 providers, >=1 with literal api_key; with NO relevant env var exported, factory builds that provider from file credential (Source=config) — editor-spawned zero-env path | ✓ VERIFIED | `TestEditorZeroEnv_LiteralInConfig` passes: Source=config, key from file, real adapter built with all provider-key env vars forced empty (construction-level proof — live-turn element is the operator-gated item below) |
| 10 | Startup writes ONE stderr warning per unresolvable-credential provider; does NOT refuse to start | ✓ VERIFIED | `loadSchedulingFactory` calls `factory.WarnUncredentialed(stderr)`; `TestLoadSchedulingFactory_WarnsUncredentialed` asserts exactly the nokey warning + non-nil factory + nil error |
| 11 | If `.ass-guard/scheduling.yaml` mode looser than 0600, warn once on stderr at startup | ✓ VERIFIED | `warnLooseConfigPerm` (provider_factory.go 133-145) wired at `acp_serve.go:264`; `TestStartupWarn_ConfigPermLoose` passes (warns at 0644, silent at 0600) |
| 12 | Zero-config backward-compat: no config changes, no flag, `$ZAI_API_KEY` set → anthropic key resolved from env (Source=env); tracer + parity build the same way | ✓ VERIFIED | `TestBackwardCompat_ZAIEnvOnly` + `TestLoadSchedulingFactory_ZeroConfigEnv` pass (Source=env, key == $ZAI_API_KEY, real adapter); main.go + parity.go both use `setupProviderFactory` |
| 13 | No credential string reaches stdout; all warnings + 0600 check write to stderr | ✓ VERIFIED | Every warning path takes the stderr writer; test buffers assert no key; tracer output goes to stderr; stdout stays ACP-only (transport discipline preserved in acp_serve) |
| 14 | All three former hardcoded construction sites (acp_serve makeProvider, main.go runTrace, parity.go runParity) delegate to factory; no site hardcodes bare `NewAnthropicProvider(shaper.New())`; acp_serve keeps zero direct calls; tracer reconstructs only with factory-RESOLVED base_url+key | ✓ VERIFIED | `grep -rn 'NewAnthropicProvider(shaper.New())' cmd/ass-guard/` → zero matches; acp_serve.go has zero NewAnthropicProvider occurrences; main.go uses `tracerProvider` → `factory.Endpoint` + factory-resolved reconstruction (provider_factory.go 90-109); parity.go:95 `factory.Build` |

**Score:** 14/14 truths verified (0 present-but-behavior-unverified; every behavior-dependent truth has a passing behavioral test under `-race`)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/scheduler/config.go` | `ProviderConfig.APIKey` (`yaml:"api_key"`) + `APIKeyEnv` (`yaml:"api_key_env"`) | ✓ VERIFIED | Lines 32-33; gofmt-aligned (documented deviation 07-01 #1); BaseURL/Shape preserved |
| `internal/scheduler/factory.go` (NEW) | ResolvedCredential, ResolveCredential, ProviderFactory, NewProviderFactory, Build, noCredentialProvider, WarnUncredentialed, + Endpoint (07-02 cross-plan accessor) | ✓ VERIFIED | All symbols present + substantive (219 lines); no-log gate holds |
| `internal/scheduler/factory_test.go` (NEW) | Precedence table, construction, lazy no-cred, warn shape, Endpoint, wire round-trip | ✓ VERIFIED | 6 tests, all pass under `-race` |
| `internal/scheduler/testdata/providers-cred.yaml` (NEW) | 2 providers (anthropic+openai shapes), 2 models each, mixed credentials | ✓ VERIFIED | zai (api_key_env) + oai (literal sk-test-literal fake); loads via `Load` |
| `internal/scheduler/load.go` | `ValidateWithWarnings(cfg) ([]string, error)`; Validate delegates; Load prints warnings to stderr | ✓ VERIFIED | Lines 252-372; D-10 shape/ref rejection preserved |
| `internal/scheduler/defaults/scheduling.yaml` + `internal/defaults/seed/scheduling.yaml` | `api_key_env: ZAI_API_KEY`, no literal, byte-identical | ✓ VERIFIED | `diff` empty; drift guard green |
| `cmd/ass-guard/provider_factory.go` (NEW) | loadSchedulingFactory, setupProviderFactory, tracerProvider, firstDeclaredProvider, warnLooseConfigPerm | ✓ VERIFIED | All present + substantive; factory used at all three sites |
| `cmd/ass-guard/provider_factory_test.go` (NEW) | warns-uncredentialed, zero-config env, perm-loose, backward-compat, editor-zero-env | ✓ VERIFIED | 5 tests, all pass |
| `cmd/ass-guard/acp_serve.go` / `main.go` / `parity.go` | Factory-wired construction sites | ✓ VERIFIED | acp_serve.go:256-275 + 264 + 554; main.go:131 + tracerProvider; parity.go:90-95 |
| `.planning/REQUIREMENTS.md` / `.planning/ROADMAP.md` | PCFG-01..04 + PROV-01 Complete; 71 coverage; Phase 7 plan list | ✓ VERIFIED | All four PCFG rows `[x]`; PROV-01 `[x]` "(completed by Phase 7)"; matrix rows; 71 total / 0 unmapped; ROADMAP plans list finalized |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | --- | --- | ------ | ------- |
| `ProviderConfig.{APIKey,APIKeyEnv}` | `provider.NewAnthropicProvider` / `NewOpenAIProvider` | `ResolveCredential` → `ProviderFactory.Build` → `WithAnthropicBaseURL`/`WithAnthropicAPIKey` (or OpenAI equivalents) | WIRED | factory.go 129-142; wire-proven by TestProviderFactory_WireRoundTrip |
| Embedded default `api_key_env: ZAI_API_KEY` | `$ZAI_API_KEY` → today's behavior | factory `ResolveCredential` reads the env var lazily | WIRED | TestBackwardCompat_ZAIEnvOnly + TestLoadSchedulingFactory_ZeroConfigEnv pass |
| `scheduler.Load` | `session.Session.Provider` | `NewProviderFactory` → `resolver.Resolve(tierHeavy)` → `factory.Build(providerName)` → `makeProvider` closure | WIRED | acp_serve.go 256 → 272-275 → 554; e2e serve tests green |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| factory.Build → AnthropicProvider | key | ResolveCredential: flag → env (`api_key_env`/`<PROVIDER>_API_KEY`) → `${VAR}` expansion → config literal | ✓ FLOWING — X-Api-Key on wire asserted in TestProviderFactory_WireRoundTrip | ✓ VERIFIED |
| factory.Build → AnthropicProvider | base_url | configured `providers.<name>.base_url` (fixture/embedded/httptest URL) | ✓ FLOWING — request Host == configured host asserted | ✓ VERIFIED |
| acp serve makeProvider | providerName | `resolver.Resolve(tierHeavy, ...)`.Provider (fallback: first declared provider sorted) | ✓ FLOWING — resolves from validated config, not hardcoded | ✓ VERIFIED |
| main.go tracer provider | base_url+key | `factory.Endpoint(providerName)` — factory-resolved, never hardcoded defaults | ✓ FLOWING — TestProviderFactory_Endpoint asserts resolved values | ✓ VERIFIED |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Precedence table + construction + lazy no-cred + warn + endpoint + wire round-trip | `go test ./internal/scheduler/ -run 'TestResolveCredential_Precedence\|TestProviderFactory_BuildConstruction\|TestProviderFactory_NoCredentialLazy\|TestProviderFactory_WireRoundTrip\|TestProviderFactory_Endpoint\|TestProviderFactory_WarnUncredentialed' -race -count=1` | ok | ✓ PASS |
| D-04 warn-not-reject + no-warn-when-env-declared + D-10 shape rejection | `go test ./internal/scheduler/ -run 'TestValidateWarns_NoCredentialField\|TestValidate_NoWarnWhenAPIKeyEnvDeclared\|TestValidate_StillRejectsBadShape' -race -count=1` | ok | ✓ PASS |
| Drift guard (seed == scheduler default) | `go test ./internal/defaults/ -run TestDriftGuard_SchedulingYAML -race -count=1` | ok | ✓ PASS |
| SC1/SC3/SC4 wiring proofs (warns, zero-config env, perm, backward-compat, zero-env) | `go test ./cmd/ass-guard/ -run 'TestLoadSchedulingFactory_WarnsUncredentialed\|TestLoadSchedulingFactory_ZeroConfigEnv\|TestStartupWarn_ConfigPermLoose\|TestBackwardCompat_ZAIEnvOnly\|TestEditorZeroEnv_LiteralInConfig' -race -count=1 -v` | 5/5 PASS | ✓ PASS |
| Factory-wired serve path e2e | `go test ./cmd/ass-guard/ -run 'TestRunACPServe_NoEngineFlag\|TestEndToEnd_ZeroContinue' -race -count=1` | ok | ✓ PASS |
| Full workspace suite incl. previously-blocked internal/profile | `go test ./... -race -count=1` | exit 0, all packages pass (incl. internal/profile — prior blocker resolved) | ✓ PASS |
| vet | `go vet ./internal/scheduler/... ./cmd/ass-guard/` | exit 0 | ✓ PASS |

### Probe Execution

No probes declared for this phase (Go unit/integration tests are the gates). N/A.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| PCFG-01 | 07-01 | Provider/model/credential config schema + loader + validation | ✓ SATISFIED | config.go fields; load.go ValidateWithWarnings; REQUIREMENTS.md `[x]` |
| PCFG-02 | 07-01, 07-02 | Credential precedence + `${VAR}` expansion + no-log/redaction + missing-credential warn+lazy | ✓ SATISFIED | factory.go ResolveCredential + noCredentialProvider + stderr warnings; REQUIREMENTS.md `[x]` |
| PCFG-03 | 07-01, 07-02 | Scheduler→provider credentialed-instance factory wiring | ✓ SATISFIED | ProviderFactory + wiring at all three sites; REQUIREMENTS.md `[x]` |
| PCFG-04 | 07-01, 07-02 | Zero-config backward-compat (embedded default seeds env-only) | ✓ SATISFIED | defaults `api_key_env: ZAI_API_KEY`, no literal; backward-compat tests; REQUIREMENTS.md `[x]` |
| PROV-01 | 07-01, 07-02 | Configurable base URL | ✓ SATISFIED (COMPLETED by Phase 7) | WireRoundTrip non-default base_url honored; REQUIREMENTS.md `[x]` "(completed by Phase 7)" |

Traceability: all 5 IDs present in REQUIREMENTS.md §Providers + matrix rows (PCFG-01..04 → Phase 7 Complete; PROV-01 → Phase 1 Complete). Coverage: 71 total, 0 unmapped. No orphaned requirements.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | — | — | Zero TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER markers in phase-modified files; no stubs (`noCredentialProvider` is deliberate D-07 lazy behavior, documented in both plans + summaries); no hardcoded empty data flowing to renders |

### Deliberate Refinements (correct, not gaps — per verification notes + documented plan assumptions)

1. **Lazy env resolution at factory-construction time** (not load-time fatal) — the D-02 vs D-06/D-07 reconciliation; recorded in 07-CONTEXT surfaced assumptions, 07-01/02-SUMMARY, STATE.md decision log. Confirmed: `ResolveCredential` is the only env-reading site; `Load` never fails on unset vars.
2. **Scheduler per-turn Dispatch not wired** — `NewScheduler` has zero production call sites; Phase 7 replaces `makeProvider` and resolves the heavy tier at startup; `Factory.Build(providerName)` is the future per-turn drop-in. Confirmed: acp_serve resolves tierHeavy once at startup.
3. **No `--api-key` CLI flag shipped** — deferred per 07-CONTEXT; precedence still implemented + table-tested in `ResolveCredential(flagKey)`, and the `apiKeyFlag` seam exists in `loadSchedulingFactory` (all call sites pass "").
4. **main.go LOG-01 tracer reconstruction** — uses `(*ProviderFactory).Endpoint(providerName)` for factory-RESOLVED base_url+key + RequestCapturer; never hardcoded defaults. Confirmed in `tracerProvider`.
5. **internal/profile TestStability_WithinSessionExtractionSource** — now PASSING (rollout session available again); verified in the full `-race` suite run.

### Human Verification Required

**1. Live editor-spawned zero-env turn (operator-gated, OPTIONAL per 07-02-PLAN surfaced assumption)**

**Test:** Put a real `api_key` literal in `.ass-guard/scheduling.yaml` (or export the key into the editor's launch environment), spawn `ass-guard acp serve` from an editor (Zed) with zero shell env vars, and make one real model turn.
**Expected:** The turn authenticates with the file credential (Source=config), requests go to the configured base_url with the resolved key, and no credential appears on stdout/logs.
**Why human:** External service integration requires a real key (operator setup). The autonomous proof — `TestEditorZeroEnv_LiteralInConfig` (Source=config, zero env, real adapter) + `TestProviderFactory_WireRoundTrip` (httptest wire: host + X-Api-Key + path) — proves the full config→factory→adapter→wire path; the live end-to-end model turn against a real provider is the only element no automated test can exercise. Both plans explicitly mark this "NOT an autonomous gate".

### Gaps Summary

No gaps found. All 14 must-have truths are verified with explicit code evidence and passing behavioral tests (`-race`), all 10 required artifacts exist and are substantive + wired, all 3 key links are wired with data flowing (Level 4), all 5 requirement IDs (PCFG-01..04, PROV-01) are satisfied with complete traceability (71/71, 0 unmapped), the full workspace suite is green (including the previously-blocked `internal/profile` test), and no anti-patterns or debt markers were found in phase-modified files.

The single human verification item is the operator-gated live turn against a real external API — explicitly documented in both plans as optional operator verification, not an autonomous gate. Status is `human_needed` per the verifier contract (external service integration always requires human), not `gaps_found` (no truth failed).

---

_Verified: 2026-08-14_
_Verifier: Claude (gsd-verifier)_
