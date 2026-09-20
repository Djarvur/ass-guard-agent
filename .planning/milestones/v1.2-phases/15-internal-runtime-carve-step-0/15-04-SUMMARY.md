---
phase: 15-internal-runtime-carve-step-0
plan: 04
subsystem: cli-support
tags: [go, refactor, verbatim-move, wrapper-retirement, cobra-boundary]

requires:
  - "15-02 providerfactory exports (SetupProviderFactory/SetupModelRouting + the bridge to retire)"
provides:
  - "internal/profilecheckcmd: RunProfileCheck + capture/report helpers + readBufSize"
  - "internal/paritycli: RunParity + zcodeInstalledVersion/parityRun/composeCacheProbeInput seams + probe/footer helpers"
  - "cmd/ass-guard/provider_factory.go deleted — zero cmd-local wrappers remain (wave-3 tree compiles wrapper-free)"
affects: [15-05, 15-06, 15-07]

actuals:
  tokens: 13500
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Wrapper retirement: delete the delegation bridge the moment its last consumer is qualified; re-pointed call sites carry verbatim into later plans"

key-files:
  created:
    - internal/profilecheckcmd/profile_check.go
    - internal/profilecheckcmd/profile_check_test.go
    - internal/profilecheckcmd/goconst_constants.go
    - internal/paritycli/parity.go
    - internal/paritycli/parity_test.go
    - internal/paritycli/goconst_constants.go
  modified:
    - cmd/ass-guard/profile_check.go
    - cmd/ass-guard/parity.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/acp_engine_e2e_test.go
    - cmd/ass-guard/e2e_opsx_test.go
    - cmd/ass-guard/e2e_opsx_matrix_test.go
    - cmd/ass-guard/subagent_tier_wiring_test.go
  deleted:
    - cmd/ass-guard/provider_factory.go

key-decisions:
  - "acp_serve.go's globalConfigPath/warnLooseConfigPerm/projectConfigPath calls qualified in this plan too (the plan text named only :339) — deleting the bridge file retires all five wrappers, and the must_have 'zero cmd-local wrappers' forces qualifying every consumer now rather than in 15-05"
  - "Parity seams kept as unexported package vars — parity_test.go moved same-package, so injection needs no exports (smallest change that compiles)"
  - "D-10 splits: profilecheckcmd carries blockText/keySessionID/keyType/profileZcode (test consumers); paritycli carries blockText/profileZcode"
  - "defaultSuitePath/defaultCachePinPath stay in cmd — they exist only to feed cobra flag defaults (D-09)"

duration: 18 min
completed: 2026-08-26
---

# Phase 15 Plan 04: profile-check + parity extraction Summary

**One-liner:** profile-check and parity run-logic relocated verbatim into two cobra-free packages, the providerfactory delegation bridge deleted with all five of its consumers qualified — wave-3 tree compiles wrapper-free, mise ci green, ledger steady at 829.

## Accomplishments

- internal/profilecheckcmd: RunProfileCheck exported; loadCaptureLine/extractCaptureCounts/reportProfileCheck/readFirstLine + readBufSize verbatim; direct os.Stderr uses preserved (SP-1: no writer rewrite); profile_check_test.go (4 tests) wholesale to the new package.
- internal/paritycli: RunParity exported; the three test-injectable seams stay unexported package vars with their gochecknoglobals nolints; assembleCacheProbe/placementCheck/emitVersionDriftWarning/emitParityFooter/countBothLayerPass verbatim; funlen + err113 nolints ride; the PARITY GATE FAIL exit-semantics comment byte-identical; parity_test.go (5 tests) wholesale, same-package seam injection intact.
- RunParity's provider arm re-pointed to providerfactory.SetupProviderFactory (direct internal import).
- Bridge retired: cmd/ass-guard/provider_factory.go deleted. acp_serve.go: setupModelRouting→providerfactory.SetupModelRouting; globalConfigPath/warnLooseConfigPerm/projectConfigPath qualified; the :335 comment block naming setupModelRouting rides unchanged (SP-2).
- Four cmd test re-points (identifier qualification + import): acp_engine_e2e_test.go:316, e2e_opsx_test.go:128, e2e_opsx_matrix_test.go:101, subagent_tier_wiring_test.go:94. checkpoint_test.go untouched (already qualified by 15-02). Exactly five cmd test files now reference providerfactory.

## Verification Results

- Task 1: build+vet clean; profilecheckcmd + cmd/ass-guard tests green; cobra-free grep = 0.
- Task 2 full battery: cobra-free (0), zero wrapper definitions (0), acp_serve re-point present, zero unqualified test wrapper calls (0), exactly 5 providerfactory-referencing test files, wrapper file absent — all assertions pass.
- `mise ci` green (re-run after fixing one lll in the parity shell delegation line — 126→wrapped).
- TestCLIBinaryContract still green (CLI surface byte-identical through the delegation change).
- Ledger: repo-wide `^func Test` = 829 — unchanged (parity 5 + profile 4 tests moved; none added/lost).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] acp_serve.go's other three wrapper calls qualified here, not in 15-05**
- **Found during:** Task 2 build
- **Issue:** Deleting cmd/ass-guard/provider_factory.go (a plan must-have) also removes globalConfigPath/projectConfigPath/warnLooseConfigPerm, which acp_serve.go calls at :349/:351/:354 — the plan text only named :339 for re-pointing, leaving the tree non-compiling.
- **Fix:** Qualified all three to providerfactory.* (identifier qualification only). These call sites carry verbatim into acpserve.Run in 15-05 as planned for :339.
- **Files modified:** cmd/ass-guard/acp_serve.go
- **Commit:** f058fa2

**2. [Rule 1 - Bug] nolint churn on shell delegation lines**
- **Found during:** Task 1 + Task 2 lint gates
- **Issue:** Preemptive wrapcheck directives unused (wrapcheck skips RunE closures); one delegation line exceeded lll's 120-rune limit.
- **Fix:** Removed unused directives; wrapped the 126-rune RunParity delegation across lines.
- **Files modified:** cmd/ass-guard/{profile_check.go,parity.go}
- **Commits:** 5a9385c (amended), f058fa2

## Notes for Later Plans

- 15-05 (acpserve): acp_serve.go is now fully providerfactory-qualified — no wrapper indirection left to preserve when the Run body moves.
- 15-06: subagent_tier_wiring_test.go still consumes the cmd-local writeTestModelRouting/pinEmptyHome duplicates (provider_factory_helpers_test.go) — Task 2 of that plan retires them.
- Six D-08 packages now exist: providerfactory, checkpointcmd, learningcmd, modelroutingcmd, profilecheckcmd, paritycli — all cobra-free, all leaf-level below the runtime.

## Self-Check: PASSED

- internal/profilecheckcmd/{profile_check.go,profile_check_test.go,goconst_constants.go} exist and committed (5a9385c).
- internal/paritycli/{parity.go,parity_test.go,goconst_constants.go} exist and committed (f058fa2).
- cmd/ass-guard/provider_factory.go absent from working tree (D in f058fa2).
- Both commit hashes present in `git log`.
