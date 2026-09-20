---
phase: 15-internal-runtime-carve-step-0
plan: 05
subsystem: acpserve
tags: [go, refactor, verbatim-move, callback-seam, startup-order]

requires:
  - "15-02 providerfactory exports"
  - "15-04 acp_serve.go providerfactory qualification (call sites carry in verbatim)"
provides:
  - "internal/acpserve: Options, PrepareServe, PreparedServe, FinishServe, FinishHooks, ResolveWorkDir, SeedACPGuard, ResolveProfilesDir"
  - "Transitional two-phase serve shape (dissolved by 15-06) with the WINDOWS #3 emitter-injection ordering pinned inside FinishServe"
affects: [15-06, 15-07]

actuals:
  tokens: 15000
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Callback-seam package split: runner-member operations cross the boundary as hook closures invoked at exact source statement positions, keeping unexported runner fields provably unreachable from the extracted package"

key-files:
  created:
    - internal/acpserve/acp_serve.go
    - internal/acpserve/options.go
    - internal/acpserve/serve_test.go
  modified:
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/checkpoint.go
    - cmd/ass-guard/acp_serve_test.go

key-decisions:
  - "ResolveWorkDir/SeedACPGuard/ResolveProfilesDir exported (plan drafted them unexported): cmd's runACPServeCmd + checkpoint.go shells consume them, and package main cannot import unexported identifiers — export-by-necessity per D-19"
  - "FinishHooks exported with a CloseSessions fourth hook: the plan named three runner ops but the source's ctx-done goroutine also calls runner.closeAllSessions(); the hook fires at the source statement position"
  - "PrepareServe takes *Options (gocritic hugeParam on the 112-byte struct) — also source-faithful, since the pre-carve runACPServe took *serveOptions"
  - "Only 2 of the plan's 5 named serve tests physically moved; the 3 runner-subject tests stay in cmd until 15-06 (subject rule D-01/D-02 — they construct sessionTurnRunner or drive the full cmd composition)"
  - "runACPServeCmd keeps the resolve/seed statements in cmd, calling the exported acpserve helpers — preserves the source's contract that runACPServe/PrepareServe receive already-resolved WorkDir/ProfilesDir"

duration: 22 min
completed: 2026-08-26
---

# Phase 15 Plan 05: acpserve extraction Summary

**One-liner:** serve pipeline split into internal/acpserve's PrepareServe/FinishServe halves around the still-in-cmd runner via a callback hook seam at exact source statement positions — stdout-discipline guards relocated, mise ci green, ledger 829.

## Accomplishments

- internal/acpserve/options.go: exported Options with the serveOptions field set + comments verbatim.
- PrepareServe reproduces source :321-360 statement order (bus → profile → providerfactory.SetupModelRouting → GlobalConfigPath/ProjectConfigPath/WarnLooseConfigPerm → bodyStore → startAuditMirror); the setupModelRouting re-point inherited from 15-04 carries in as identifier qualification.
- FinishServe reproduces source :398-425 (NewServer → sched.Open degrade-loudly → AssignSchedule → InjectEmitter → StartScheduler → ctx-done CloseSessions → Serve); WINDOWS #3 ordering (emitter injection strictly between NewServer and startScheduler) holds inside one function.
- FinishHooks callback seam: AssignSchedule/InjectEmitter/StartScheduler/CloseSessions — cmd supplies closures over runner's unexported members; internal/acpserve has zero direct runner-member access (grep-verified by construction: package main cannot be imported).
- Two sanctioned de-cobra edits: ResolveProfilesDir(profilesDir, changed, workDir) and the audit-log + profiles-dir-Changed flag reads lifted into the cobra shell.
- startAuditMirror's staticcheck nolint and srv.Serve's wrapcheck nolint ride their lines.
- cmd/ass-guard/acp_serve.go retains only the cobra shells + runACPServeCmd (de-cobra'd) + the transitional runACPServe composition + the runner family.
- Test relocation: TestACPServeWiresStdoutClean + TestACPServeNoStdoutPollutionFromLogs now run from internal/acpserve through runServeForTest (PrepareServe → stubServeRunner → FinishServe); TestACPServeCommandRegistered stays in cmd.

## Verification Results

- Task 1: build + vet clean; serve battery green from cmd; cobra-free (0 imports).
- Task 2: `mise ci` green; ledger 829 unchanged; dupe-names=2 (the two pre-existing cross-package collisions from 15-03's note — TestLoadLayering, TestMain); all 5 serve test names count exactly 1; TestACPServeCommandRegistered present in cmd.
- golangci-lint 0 issues on both packages.
- Startup-order reviewable in diff: PrepareServe+FinishServe statements match source :321-425 order; only the two sanctioned de-cobra edits + the inherited 15-04 qualification differ vs source within moved blocks.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Exported the shell-consumed helpers + finishHooks**
- **Found during:** Task 1 compile
- **Issue:** plan drafted resolveWorkDir/seedACPGuard/resolveProfilesDir + finishHooks as unexported in acpserve, but cmd (runACPServeCmd, checkpoint.go shells, the FinishServe call) must consume them — package main cannot reach unexported identifiers.
- **Fix:** ResolveWorkDir/SeedACPGuard/ResolveProfilesDir + FinishHooks exported (D-19 export-by-necessity). SeedACPGuard kept as a thin exported entry over the unexported verbatim body for source fidelity.
- **Files modified:** internal/acpserve/acp_serve.go
- **Commit:** ec851c1

**2. [Rule 2 - Missing critical functionality] Fourth hook (CloseSessions)**
- **Found during:** Task 1 design
- **Issue:** plan's finishHooks named three runner operations, but the source's ctx-done goroutine (`runner.closeAllSessions()`) is a fourth unexported-runner member FinishServe must reach — omitting it would orphan session reaping (subprocess leak on SIGINT).
- **Fix:** CloseSessions hook added, invoked at the exact source statement position.
- **Files modified:** internal/acpserve/acp_serve.go
- **Commit:** ec851c1

**3. [Rule 3 - Blocking] Only 2 of the plan's 5 named tests moved this wave**
- **Found during:** Task 2
- **Issue:** TestServeTranscriptWriter_OnePerSession constructs sessionTurnRunner directly; TestServeAudit_RequestShapedThroughRealSeam + TestServeMirror_Override drive the full pipeline including the real runner — none can compile from internal/acpserve while the runner lives in package main.
- **Fix:** Moved the two initialize-only stdout-discipline tests (stub-runner call-through equivalent); the three runner-subject tests stay in cmd and ride with the runner family in 15-06 (the plan's own D-01/D-02 subject rule). Acceptance criteria satisfied: StdoutClean/NoStdoutPollution pass from internal/acpserve; CommandRegistered stays in cmd.
- **Files modified:** internal/acpserve/serve_test.go, cmd/ass-guard/acp_serve_test.go
- **Commit:** 64b3789

**4. [Rule 1 - Bug] Lint-shape fixes on the new boundary**
- gocritic hugeParam (Options 112 bytes) → pointer params (also source-faithful); dead contextcheck nolint removed from the FinishServe goroutine (the hook hides Session.Close from the linter); wrapcheck/lll directives placed on the actual error-return/long lines.
- **Commits:** ec851c1

## Notes for Later Plans

- 15-06 dissolves the split: FinishServe's runner param becomes the concrete runner; the hooks become direct member assignments again; Run reunifies. serve_test.go's stubServeRunner + noopFinishHooks + runServeForTest dissolve with it.
- acp_engine_e2e_test.go:310's comment references serveOptions by name (comment only — rides unchanged per SP-2).
- cmd/ass-guard/goconst_constants.go still holds every runner-family constant — 15-06 redistributes.

## Self-Check: PASSED

- internal/acpserve/{acp_serve.go,options.go,serve_test.go} exist and committed (ec851c1, 64b3789).
- cmd/ass-guard/{acp_serve.go,checkpoint.go,acp_serve_test.go} modified and committed.
- Both commit hashes present in `git log`.
