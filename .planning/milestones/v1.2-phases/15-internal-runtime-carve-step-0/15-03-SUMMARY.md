---
phase: 15-internal-runtime-carve-step-0
plan: 03
subsystem: cli-support
tags: [go, refactor, verbatim-move, cobra-boundary, subject-split]

requires:
  - "15-02 provider-factory extraction (extraction precedent + wrappers)"
provides:
  - "internal/checkpointcmd: RunCheckpointList, RunCheckpointRestore (+ riding consts)"
  - "internal/learningcmd: ResolveLearnedPath, RunLearningList, RunLearningRevert"
  - "internal/modelroutingcmd: EmitResolveHuman, EmitResolveJSON, DescribeCapabilities"
affects: [15-04, 15-05, 15-06, 15-07]

actuals:
  tokens: 11000
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Cobra-boundary split (D-09): command definitions stay in cmd, run/emit logic relocates verbatim; tests follow their subject"

key-files:
  created:
    - internal/checkpointcmd/checkpoint.go
    - internal/checkpointcmd/checkpoint_test.go
    - internal/learningcmd/learning_cmd.go
    - internal/learningcmd/learning_cmd_test.go
    - internal/modelroutingcmd/modelrouting.go
  modified:
    - cmd/ass-guard/checkpoint.go
    - cmd/ass-guard/learning_cmd.go
    - cmd/ass-guard/modelrouting.go
    - cmd/ass-guard/checkpoint_test.go

key-decisions:
  - "modelrouting_test.go stays in cmd: every test drives the cobra tree via runSchedulingCmd→newModelRoutingCmd; a wholesale move would import spf13/cobra into internal/modelroutingcmd and fail the plan's own cobra-free verify gate (subject = the shells, which stay in cmd per D-09)"
  - "Task 1 also re-pointed cmd test call sites in place (checkpoint/learning) so the Task 1 commit compiles; physical relocation then landed in Task 2 as planned"
  - "checkpointerAdapter stays in cmd/ass-guard/checkpoint.go — acp_serve.go:1339 wires it into the runner (relocates with the runner family in 15-06)"
  - "No goconst_constants.go created for any of the three packages: no moved body references a shared-literal constant (the riding consts are file-local)"

duration: 14 min
completed: 2026-08-26
---

# Phase 15 Plan 03: CLI-support extraction Summary

**One-liner:** checkpoint/learning/model-routing run-logic relocated verbatim into three cobra-free internal packages with tests following their subject — mise ci green, ledger unchanged at 829, zero dupes introduced.

## Accomplishments

- internal/checkpointcmd/checkpoint.go: RunCheckpointList + RunCheckpointRestore verbatim; checkpointNoEntriesNote/checkpointRestoredNote/checkpointIDHint ride along; `//nolint:contextcheck` preserved on the Store.Open call.
- internal/learningcmd/learning_cmd.go: ResolveLearnedPath + RunLearningList + RunLearningRevert verbatim; the sanctioned stdout table exception (SP-4) unchanged byte-for-byte.
- internal/modelroutingcmd/modelrouting.go: EmitResolveHuman + EmitResolveJSON + DescribeCapabilities verbatim; `//nolint:wrapcheck // json encoder` rides the Encode line.
- cmd shells retain every cobra definition and flag bit-identical; only the three delegation lines changed in RunE bodies. resolveWorkDir stays cmd-side — checkpointcmd receives the resolved workDir string (no acpserve import, per the plan's preferred shape).
- Test redistribution: learning_cmd_test.go wholesale to internal/learningcmd; checkpoint_test.go mixed split (4 CLI tests + seedCheckpointStore moved; TestSessionFor_WiresCheckpointer + the gated live rollback test stay in cmd with their ASSGUARD_CHECKPOINT_E2E/ZAI_API_KEY gates and the early-qualified providerfactory.SetupProviderFactory call from 15-02).

## Verification Results

- Task 1: `go build ./... && go vet ./...` clean; `go test ./internal/{checkpointcmd,learningcmd,modelroutingcmd}/ ./cmd/ass-guard/ -count=1` green; `grep -rl spf13/cobra internal/{checkpointcmd,learningcmd,modelroutingcmd}/` → 0 files (cobra-free gate).
- Task 2 wave boundary: `mise ci` green (vet, 0 lint issues, CGO_ENABLED=0 build, race suite).
- Ledger: repo-wide `^func Test` = **829** (= 826 phase-start + 3 goldens from 15-01; unchanged from 15-02 — moves only). Every moved/stayed test name appears exactly once (all 10 checkpoint/learning names counted =1).
- Spot-check: contextcheck nolint present in RunCheckpointRestore; learning table format strings byte-identical.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Task 1 included in-place test call-site qualification**
- **Found during:** Task 1 execution
- **Issue:** Task 1's own verify requires `go test ./cmd/ass-guard/` green, but the cmd test files still called the moved unexported run* symbols — a Task-1-only commit would not compile.
- **Fix:** Qualified the checkpoint/learning test call sites to checkpointcmd.*/learningcmd.* in the same Task 1 commit (files physically relocated in Task 2 as planned).
- **Files modified:** cmd/ass-guard/checkpoint_test.go, cmd/ass-guard/learning_cmd_test.go
- **Commit:** 1157469

**2. [Rule 3 - Blocking] modelrouting_test.go NOT relocated**
- **Found during:** Task 2 planning
- **Issue:** Plan says git mv modelrouting_test.go wholesale to internal/modelroutingcmd, but all five tests exercise the cobra command tree (runSchedulingCmd builds newModelRoutingCmd) — the move would import spf13/cobra into the package and fail the plan's own cobra-free acceptance gate.
- **Fix:** File stays in cmd with the cobra shells it tests (D-02 subject rule: tests follow their subject; the subject here is the CLI tree, which stays in cmd). The moved emit logic stays covered through shell-level delegation. Exported surface unchanged from plan.
- **Files modified:** none (file untouched)
- **Commit:** n/a

**3. [Rule 3 - Blocking] Unused wrapcheck nolint directives dropped from cmd shells**
- **Found during:** Task 1 lint gate
- **Issue:** wrapcheck does not fire inside cobra RunE closures, so the delegation-line wrapcheck directives I added preemptively were flagged unused by nolintlint (strict).
- **Fix:** Removed wrapcheck from the 5 directives (kept `//nolint:lll` on the 4 lines over 120 runes).
- **Files modified:** cmd/ass-guard/{checkpoint.go,learning_cmd.go,modelrouting.go}
- **Commit:** 1157469

**4. [Interpretation - no code change] Plan's uniq -d dupe check reports 2 pre-existing cross-package name collisions**
- `TestLoadLayering` (internal/modelrouting/load_test.go + internal/hookdag/config_test.go) and `TestMain` (per-package mains) both pre-exist at phase-start f1e26b3 — Go permits identical test names in different packages. Scoped check over every name this plan touched: all count exactly 1. No test lost or duplicated by the carve.

## Notes for Later Plans

- cmd/ass-guard/checkpoint_test.go now holds only sessionFor-subject + live-rollback tests (runner-family subjects → 15-06).
- The three new packages are leaf-level (import only their domain internal package) — no import-cycle risk for internal/runtime.
- Live test count: 829 repo-wide; cmd/ass-guard alone = 117 (ledger 130 + 3 goldens − 8 providerfactory tests moved by 15-02 − 8 checkpoint/learning CLI tests moved here). internal/ now carries the 16 moved functions plus everything it owned before.

## Self-Check: PASSED

- internal/checkpointcmd/{checkpoint.go,checkpoint_test.go} exist and committed (1157469, ab972d8).
- internal/learningcmd/{learning_cmd.go,learning_cmd_test.go} exist and committed.
- internal/modelroutingcmd/modelrouting.go exists and committed.
- Both commit hashes present in `git log`.
