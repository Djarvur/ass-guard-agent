---
phase: 15-internal-runtime-carve-step-0
plan: 06
subsystem: runtime
tags: [go, refactor, verbatim-move, package-carve, equivalence-proof]

requires:
  - "15-05 acpserve PrepareServe/FinishServe split (dissolved here)"
  - "15-02 providerfactory exports"
provides:
  - "internal/runtime: Runner, RunnerConfig, NewRunner, LoadCommandRegistry, SetupEngine, CloseSession, serve-composition quartet"
  - "internal/runtime/enginebridge: EngineTurnAdapter, ACPDispatcher, hook runners, RealCommandRunner, StubCatalogExec over BridgeConfig"
  - "internal/acpserve.Run single-function composition (source :320-425 order)"
  - "cmd/ass-guard reduced to cobra shells + cmd-subject tests"
affects: [15-07]

actuals:
  tokens: 46000
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Mirror-config bridge: a foreign package holds the runner's func-value seams (BridgeConfig) so behavior crosses the boundary while unexported receivers never do"
    - "Serve-composition quartet: the four runner operations the transport needs arrive as exported one-line members placed to satisfy funcorder's exported-before-trailing-unexported rule"

key-files:
  created:
    - internal/runtime/runtime.go
    - internal/runtime/doc.go
    - internal/runtime/goconst_constants.go
    - internal/runtime/runner_battery_test.go
    - internal/runtime/enginebridge/enginebridge.go
    - internal/runtime/enginebridge/goconst_constants.go
    - internal/runtime/enginebridge/trigger_from_signal_test.go
    - internal/acpserve/goconst_constants_test.go
  modified:
    - internal/runtime/cron_wiring.go (git mv from cmd, byte-verbatim + receiver sed)
    - internal/acpserve/acp_serve.go
    - internal/acpserve/serve_test.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/acp_serve_test.go
    - cmd/ass-guard/checkpoint.go
    - cmd/ass-guard/goconst_constants.go
    - cmd/ass-guard/background_wiring_test.go
    - internal/session/ask.go (comment-only identifier rename)
  moved-tests:
    - 13 cmd wiring/e2e/integration/tracer files -> internal/runtime
    - checkpoint_test.go -> internal/runtime/checkpoint_session_test.go

key-decisions:
  - "Exported sextet, not the planned quartet: acpserve.Run also calls LoadCommandRegistry + SetupEngine between NewRunner and NewServer, so those two methods ride the D-19 export-by-necessity seam as direct renames (tests in-package keep calling them)"
  - "acpserve.Run born in Task 1's commit (compile closure, SP-12): cmd's transitional runACPServe cannot compile once hooks would be unexported method values on runtime.Runner"
  - "Pointer configs for NewRunner/NewEngineTurnAdapter/NewACPDispatcher (gocritic hugeParam; 15-05 Options precedent)"
  - "TestAskWiring_ConfigKnob split: the cobra-flag half merged into TestACPServeCommandRegistered (ledger-preserving — no new test function), the threading half stays in runtime"
  - "provider_factory_helpers_test.go moved whole (single consumer subagent_tier_wiring) instead of the planned duplicate-then-delete"
  - "background_wiring_test.go stays in cmd: it has no runner references (12-06 coreexec wiring battery, cmd-subject)"

duration: 75 min
completed: 2026-08-26
---

# Phase 15 Plan 06: internal/runtime carve Summary

**One-liner:** sessionTurnRunner family verbatim-relocated into internal/runtime (Runner + RunnerConfig + enginebridge mirror-config bridge), acpserve.Run reunified one-function, cmd reduced to cobra shells — mise ci green, ledger 829 exact.

## Accomplishments

- internal/runtime/runtime.go: Runner struct (fields unexported, comments adapted), RunnerConfig + NewRunner struct-fill thin ctor (D-05), sessionFor with the single sanctioned edit (`stubCatalogExec{}` → `enginebridge.NewStubCatalogExec()`), advisory family, spawnMCP/mergeMCPServers, session lifecycle, routeAskReply/toContentBlocks/patternNextPrompter.
- cron_wiring.go: `git mv` byte-verbatim per amended D-13/D-15 — package clause + receiver sed only; startScheduler keeps its unexported name (the plan's Phase-25 seam note recorded in doc.go).
- internal/runtime/enginebridge: engineTurnRunnerAdapter→EngineTurnAdapter + ACPDispatcher + HookSessionTurnRunner/HookSessionBoundaryOpener/RealCommandRunner + StubCatalogExec, all over BridgeConfig (behavior crosses as func values; Invoke nil-guarded). stagePost* vocabulary + triggerFromSignal here; patternNextPrompter interface stays in core (sole consumer = setupEngine closure).
- acpserve.Run: one function reproducing source :320-425 statement order — bus → profile → SetupModelRouting → perm warnings → bodyStore → startAuditMirror → runtime.NewRunner → LoadCommandRegistry → engine-degrade branch → NewServer → sched.Open degrade-loudly → SetSchedule → SetEmitter (WINDOWS #3) → StartScheduler → ctx-done CloseAllSessions → Serve. PrepareServe/FinishServe/PreparedServe/FinishHooks deleted.
- cmd/ass-guard/acp_serve.go: newACPCmd/newACPServeCmd/runACPServeCmd shells only; cmd goconst trimmed to tierHeavy/profileZcode/flagProfilesDir.
- Test relocation: 13 runner-subject files + checkpoint battery + the acp_serve_test split (battery → runtime/runner_battery_test.go; serve-seam audit/mirror proofs → internal/acpserve/serve_test.go on plain Run; cobra remainder in cmd). TestStageVocab_TriggerFromSignal → enginebridge package (subject rule). Serve-test stubs (stubServeRunner/noopFinishHooks/runServeForTest) dissolved with the split.

## Verification Results

- Task 1: `go build ./...` green; `grep -rn sessionTurnRunner --include='*.go' cmd/ internal/` = 0 (one comment in internal/session/ask.go renamed — see Deviations).
- Task 2: vet + tests green in internal/runtime (+enginebridge) and internal/acpserve and cmd; exactly one TestMain in internal/runtime (mcp_tracer unit).
- Task 3: `mise ci` fully green (vet + golangci-lint 0 issues + CGO_ENABLED=0 build + `go test -race ./...`). Ledger: total=829 (exact baseline match), dupes=2 — both pre-existing at f1e26b3 (TestLoadLayering ×2 internal/, TestMain cmd+mcp→runtime+mcp); D-before = D-after, no NEW duplicates. TestZeroConfigFirstRun + TestCLIBinaryContract* + TestACPServeCommandRegistered pass. color-moved diff runs (stat over 226 files; moved blocks render as renames 95-99%).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Exported sextet instead of the FinishHooks quartet**
- **Found during:** Task 1 compile closure
- **Issue:** acpserve.Run calls runner.loadCommandRegistry() + runner.setupEngine() between NewRunner and NewServer — unexported methods unreachable from a foreign package; the plan named only the four schedule/emitter/scheduler/close operations.
- **Fix:** SetupEngine + LoadCommandRegistry renamed exported (D-19; sed-consistent across moved tests); SetSchedule/SetEmitter/StartScheduler/CloseAllSessions one-line members over the verbatim bodies (startScheduler stays unexported — cron_wiring byte-verbatim constraint).
- **Commit:** b7f3532, a709e37

**2. [Rule 3 - Blocking] acpserve.Run born in Task 1's commit**
- **Issue:** plan (d) says cmd retains ONLY shells in Task 1 while acpserve.Run is Task 3 work; cmd's transitional runACPServe cannot compile post-carve (hooks would be unexported method values on runtime.Runner).
- **Fix:** Run's production body rides commit 1; Task 3's content folds into the serve_test re-point riding commit 2 + the lint-shape commit 3. Tasks 1-3 all verified after commit 3.
- **Commit:** b7f3532

**3. [Rule 1 - Bug] D=0 gate unmeetable as written**
- **Issue:** the ledger gate demands zero duplicate test names repo-wide, but two duplicates pre-date the phase (TestLoadLayering, TestMain) and persist unchanged.
- **Fix:** documented; gate intent (no NEW duplicates) holds — D before = D after = 2.

**4. [Rule 3 - Blocking] TestAskWiring_ConfigKnob straddles the boundary**
- **Issue:** one test asserts both the cobra flag (newACPServeCmd — cmd-subject) and the runner threading (runtime-subject); whole-file move would not compile in runtime.
- **Fix:** flag assertions merged into TestACPServeCommandRegistered in cmd (no new test function — ledger preserved); threading half keeps the name in runtime.
- **Commit:** fbea8f4

**5. [Rule 3 - Blocking] session/ask.go comment names the dead identifier**
- **Issue:** `grep sessionTurnRunner` acceptance is repo-wide cmd/+internal/; one comment in internal/session/ask.go referenced sessionTurnRunner.Run (not moved text).
- **Fix:** mechanical comment rename to Runner.Run.
- **Commit:** b7f3532

**6. [Rule 1 - Bug] Lint-shape fixes on the new boundary**
- funcorder quartet placement (exported before trailing unexported block); hugeParam pointer configs; revive doc comments; dead nolint removals; nolint directive shapes (whyNoLint/lll interplay).
- **Commit:** a709e37

## Notes for Later Plans

- 15-07: phase close-out. The runtime package doc records the Phase-25 cron seam plan (amended D-13/D-15 sign-off).
- Two lint warnings remain (pre-existing): unknown linter names `err113-best-effort`, `testingcontext` referenced in nolint directives elsewhere.
- Commit 1's tree transiently holds one duplicate test file (e2e_opsx_matrix_test.go created before its cmd deletion landed in commit 2) — SP-12 no-intermediate-green-tree; working tree and HEAD clean.

## Self-Check: PASSED

- internal/runtime/{runtime.go,doc.go,goconst_constants.go,cron_wiring.go,runner_battery_test.go} exist; enginebridge/{enginebridge.go,goconst_constants.go,trigger_from_signal_test.go} exist.
- Commits b7f3532, fbea8f4, a709e37 present in git log.
