---
phase: 15-internal-runtime-carve-step-0
plan: 02
subsystem: provider-factory
tags: [go, refactor, verbatim-move, goconst-split, delegation-bridge]

requires:
  - "15-01 test ledger + CLI contract goldens (equivalence instruments)"
provides:
  - "internal/providerfactory package: SetupProviderFactory, SetupModelRouting, LoadModelRoutingFactory, GlobalConfigPath, ProjectConfigPath, WarnLooseConfigPerm"
  - "internal/providerfactory/goconst_constants.go carrying tierHeavy (D-10 split)"
  - "cmd/ass-guard transitional 5-wrapper delegation bridge (setupProviderFactory/setupModelRouting/globalConfigPath/projectConfigPath/warnLooseConfigPerm)"
  - "cmd-local writeTestModelRouting + pinEmptyHome duplicates (D-03) in provider_factory_helpers_test.go"
affects: [15-03, 15-04, 15-05, 15-06, 15-07]

actuals:
  tokens: 9500
  tasks: 2
  commits: 1

tech-stack:
  added: []
  patterns:
    - "Verbatim git-mv extraction with export-by-necessity (D-09) and same-name cmd wrapper bridge deleted by later plans"

key-files:
  created:
    - internal/providerfactory/goconst_constants.go
    - cmd/ass-guard/provider_factory_helpers_test.go
  modified:
    - internal/providerfactory/provider_factory.go
    - internal/providerfactory/provider_factory_test.go
    - cmd/ass-guard/provider_factory.go
    - cmd/ass-guard/main.go
    - cmd/ass-guard/checkpoint_test.go

key-decisions:
  - "Bridge carries all 5 wrappers (not just the 2 the plan named): grep showed acp_serve.go also calls globalConfigPath/projectConfigPath/warnLooseConfigPerm directly — the operative requirement is that acp_serve.go compiles untouched, which needs all five"
  - "Removed the //nolint:unparam directive riding LoadModelRoutingFactory: unparam skips exported functions, so the directive is dead in the new package and nolintlint (strict) fails the build on it"
  - "testpackage + wrapcheck nolint directives added per existing repo precedent (internal/modelrouting, internal/acp pattern; //nolint:wrapcheck // thin delegation pattern)"

duration: 16 min
completed: 2026-08-26
---

# Phase 15 Plan 02: provider-factory extraction Summary

**One-liner:** provider_factory.go + its suite git-mv'd verbatim into internal/providerfactory (6 exports, D-10 goconst split, D-03 helper duplication) behind a 5-wrapper cmd bridge — mise ci green, zero test-count drift.

## Accomplishments

- `git mv` cmd/ass-guard/provider_factory.go → internal/providerfactory/provider_factory.go; package clause change only, plus D-09 export-by-necessity: SetupProviderFactory, SetupModelRouting, LoadModelRoutingFactory, GlobalConfigPath, ProjectConfigPath, WarnLooseConfigPerm (acp_serve.go:339 reaches setupModelRouting via the wrapper, so a cmd wrapper cannot delegate to an unexported symbol — SetupModelRouting MUST export). firstDeclaredProvider stays unexported. Declaration order preserved.
- Test file moved likewise (package providerfactory, two call-site renames to the exported WarnLooseConfigPerm/LoadModelRoutingFactory).
- internal/providerfactory/goconst_constants.go created with tierHeavy (the only former cmd goconst constant this package's moved code references), internal/acp header style per D-10.
- cmd/ass-guard/provider_factory.go rebuilt as a 5-wrapper delegation bridge (setupProviderFactory, setupModelRouting, globalConfigPath, projectConfigPath, warnLooseConfigPerm) so acp_serve.go, parity.go, and the runner-adjacent tests compile untouched until 15-04/15-05 re-point them.
- D-03 same-task duplication: writeTestModelRouting + pinEmptyHome copied verbatim into cmd/ass-guard/provider_factory_helpers_test.go (package main) because subagent_tier_wiring_test.go stays in cmd until 15-06 and calls both.
- Re-points riding this task: main.go:130 runTrace and checkpoint_test.go:319 now call providerfactory.SetupProviderFactory directly (checkpoint_test.go qualified early so 15-04's wrapper deletion is compile-safe under every intra-wave-3 ordering).

## Verification Results

- `mise ci` (vet → golangci-lint → CGO_ENABLED=0 build → `go test -race -count=1 ./...`) — PASS.
- `golangci-lint run ./internal/providerfactory/... ./cmd/ass-guard/...` — 0 issues.
- warnLooseConfigPerm byte-identity: `diff <(git show HEAD~1:... | sed -n '159,171p') <(sed -n '159,171p' new)` — only line 1 differs (the sanctioned lowercase→uppercase signature rename); body byte-identical, `perm&0o077 != 0` check and the 0600-advice message intact (T-15-02 mitigated).
- Ledger accounting (plan Task 2): repo-wide `^func Test` across cmd/ + internal/ = **829**; phase-start (f1e26b3) = 826; delta = **3 = exactly the TestCLIBinaryContract{RootAndACP,SupportCommands,Profile} functions added by 15-01 after ledger capture**. 15-02 itself added/lost zero test functions — the moved suite's functions counted identically before and after.
- Diff scope vs phase start: only the 8 code files listed in key-files + 15-01 artifacts + planning docs. (The plan's `git diff master` criterion is stale — master sits 220 commits behind at v1.1 phase 9; the phase-start commit f1e26b3 is the operative baseline. Recorded below as an interpretation note.)
- `git diff f1e26b3 -- cmd/ass-guard/provider_factory.go`: 155-line deletion — file fully gone from cmd except the new bridge content (git tracks it as rewrite-by-rename; `git log --follow` chains into internal/).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Dropped dead //nolint:unparam directive on LoadModelRoutingFactory**
- **Found during:** Task 1 lint gate
- **Issue:** unparam does not evaluate exported functions, so the directive that rode the function verbatim out of cmd is unused in the new package; strict nolintlint fails the build on unused directives.
- **Fix:** Removed the directive line (plus the trailing `//` gofmt then wanted gone). History preserved via `git log --follow`.
- **Files modified:** internal/providerfactory/provider_factory.go
- **Commit:** 92ecd40

**2. [Rule 3 - Blocking] New-package lint surface: testpackage + wrapcheck directives per repo precedent**
- **Found during:** Task 1 lint gate
- **Issue:** package-internal test file now trips testpackage (was invisible under `package main`); the 3 error-returning bridge wrappers trip wrapcheck (delegating external-package errors).
- **Fix:** `//nolint:testpackage // internal package test` on the test package clause (internal/modelrouting + internal/acp precedent); `//nolint:wrapcheck // thin delegation` on the 3 bridge returns (cmd/ass-guard/checkpoint.go:29 precedent).
- **Files modified:** internal/providerfactory/provider_factory_test.go, cmd/ass-guard/provider_factory.go
- **Commit:** 92ecd40

**3. [Interpretation - no code change] Plan's `git diff master` acceptance read against phase-start commit**
- master is 220 commits behind the working branch (stale at v1.1); the plan author assumed master == phase start. Scope check performed against f1e26b3 instead — result clean (only sanctioned files).

**4. [Scope boundary] Reverted incidental gofmt edits in spikes/**
- `gofmt -w .` (mise fmt) also reformatted spikes/03-acp-handshake/main.go and spikes/05-stdout-collision/main.go — pre-existing formatting drift unrelated to the carve. Reverted with `git checkout --` on those two files only; out of scope per deviation rules (logged here rather than in deferred-items.md because the drift is formatting-only and spikes/ are frozen experiments).

## Notes for Later Plans

- Call sites still on cmd wrappers (delete in the named plan): parity.go:240 (15-04); acp_serve.go:339/348/350/353 (15-05); acp_engine_e2e_test.go:316, e2e_opsx_test.go:128, e2e_opsx_matrix_test.go:101, subagent_tier_wiring_test.go:94 (15-06, riding the file moves).
- 15-04 deletes setupProviderFactory; 15-05 deletes the other four wrappers.
- 15-06 Task 2 retires cmd/ass-guard/provider_factory_helpers_test.go when subagent_tier_wiring_test.go relocates.
- Live repo-wide test count now 829 (826 phase-start + 3 goldens); cmd/ass-guard alone = 133.

## Self-Check: PASSED

- internal/providerfactory/{provider_factory.go,provider_factory_test.go,goconst_constants.go} exist and committed (92ecd40).
- cmd/ass-guard/{provider_factory.go,provider_factory_helpers_test.go} exist and committed (92ecd40).
- 92ecd40 present in `git log`.
