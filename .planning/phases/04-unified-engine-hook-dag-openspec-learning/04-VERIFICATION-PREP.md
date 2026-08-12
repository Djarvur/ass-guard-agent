# Phase 4 — Verification Prep

Maps each of the 5 ROADMAP Phase-4 success criteria to the test(s) + exact command that proves it, ready for `/gsd:verify-work`. Each row was run during Phase-4 execution (Wave 4, Plan 04-07 T3); the actual output is recorded under **Phase Gate** below.

## Success-criteria → test map

| # | Criterion | REQ-IDs | Test(s) | Command |
|---|-----------|---------|---------|---------|
| 1 | Zero-continue OpenSpec + structural safety (an unmodified OpenSpec workflow runs end-to-end with zero continue taps; unmatched output triggers nothing) | ENG-01/02/03/05, OPEN-01/02/03 | `cmd/ass-guard.TestEndToEnd_ZeroContinue`, `cmd/ass-guard.TestEndToEnd_StructuralSafety`, `cmd/ass-guard.TestE2E_Criterion1_ZeroContinueAndSafety`, `internal/engine.TestObserve_ZeroContinueHappyPath`, `internal/engine.TestDecide_UnmatchedIsNothing`, `internal/engine.TestDecide_QuickUnmatchedIsNothing` | `go test ./cmd/ass-guard ./internal/engine -run 'TestEndToEnd|TestE2E_Criterion1|TestObserve_ZeroContinue|TestDecide_Unmatched|TestDecide_Quick' -race -v` |
| 2 | Seeded hooks fire automatically + graceful degradation (engine failure never prevents a turn from completing) | HOOK-01/02/05, ENG-04 | `internal/hookdag.TestExecute_HappyPath`, `internal/hookdag.TestExecute_SendPromptIsATurn`, `internal/engine.TestObserve_LastTurnOutputPanicDegradation`, `internal/engine.TestObserve_DecidePanicDegradation`, `cmd/ass-guard.TestEndToEnd_ZeroContinue` (forgotten routine observable in transcript) | `go test ./internal/hookdag ./internal/engine ./cmd/ass-guard -run 'TestExecute_|TestObserve_(LastTurnOutputPanic|DecidePanic)|TestEndToEnd_ZeroContinue' -race -v` |
| 3 | on-failure on every hook + provenance loop prevention (every failure declares halt/continue/ask; a hook cannot re-trigger its own stage without opt-in) | HOOK-03/04 | `internal/hookdag.TestExecute_RunCommandHalt`, `internal/hookdag.TestExecute_OnFailureContinue`, `internal/hookdag.TestExecute_OnFailureAsk`, `internal/hookdag.TestReentrant_Refused`, `internal/hookdag.TestReentrant_AllowReentrant`, `internal/hookdag.TestReentrant_InFlightClearedAfterCompletion`, `internal/hookdag.TestRunCommand_ExitCodeContract` | `go test ./internal/hookdag -run 'TestExecute_(RunCommandHalt|OnFailure)|TestReentrant|TestRunCommand_ExitCodeContract' -race -v` |
| 4 | Learning mode: ask-once-and-remember + propose hooks + versioned/revertible | LRN-01/02/03/04 | `internal/learning.TestStore_RecordCandidateConfidence0`, `internal/learning.TestStore_ConfirmThresholdActive`, `internal/learning.TestStore_ConfirmConflict`, `internal/learning.TestStore_ConflictBlocksUse`, `internal/learning.TestStore_ExpiredIgnoredNotDeleted`, `internal/learning.TestProposeHooks_RepeatedSequence`, `internal/learning.TestStore_RevertRemovesEntry`, `cmd/ass-guard.TestLearningList_Populated`, `cmd/ass-guard.TestLearningRevert_Existing`, `cmd/ass-guard.TestE2E_Criterion4_LearningAskOnce` | `go test ./internal/learning ./cmd/ass-guard -run 'TestStore_|TestProposeHooks_|TestLearning|TestE2E_Criterion4' -race -v` |
| 5 | Concurrent reads + serialized mutations + swappable backends (read-only tools run concurrently; mutating tools serialize; WebSearch/WebFetch swap by config) | TOOL-04/05 | `internal/toolexec.TestDispatchBatch_ReadOnlyParallelism`, `internal/toolexec.TestDispatchBatch_MutatingSerialization`, `internal/toolexec.TestDispatchBatch_MutatingAlone`, `internal/toolexec.TestDispatchBatch_MaxConcurrentBound`, `internal/toolexec.TestHTTPBackend_Search`, `internal/toolexec.TestBackendsFromConfig_Select`, `internal/toolexec.TestBackend_SwappableWithoutCodeChange`, `internal/session.TestPromptDispatchBatchLoop` | `go test ./internal/toolexec ./internal/session -run 'TestDispatchBatch_|TestHTTPBackend_|TestBackendsFromConfig|TestBackend_Swappable|TestPromptDispatchBatchLoop' -race -v` |

## ENG-03 cancel-drain (structural off-switch — PROJECT.md's only safety mechanism)

| Test(s) | Command |
|---------|---------|
| `internal/engine.TestObserve_CancelDrain` (unit: ctx cancel ⇒ "cancelled" + 1 Run), `cmd/ass-guard.TestCancelDrainsInjections` (ACP-level: session/cancel after the 1st of N injections ⇒ 1 provider call) | `go test ./internal/engine ./cmd/ass-guard -run 'TestObserve_CancelDrain|TestCancelDrainsInjections' -race -v` |

The drain mechanism is structural: `engine.Observe` checks `ctx.Err()` before each continue-injection; Plan 04-05 runs `Observe` under the SAME ctx derived from the ACP `turnCtx`, so `handleSessionCancel → sessionState.cancelTurn → cancel()` reaches the loop. See the doc comment on `engine.Engine.Observe` (`internal/engine/observe.go`).

## Phase Gate

Run during Wave 4 (Plan 04-07 T3) on the Phase-4 milestone branch:

```
$ go build ./...                 # exit 0 (clean)
$ go vet ./...                   # exit 0 (clean)
$ go test ./internal/engine ./internal/hookdag ./internal/openspec ./internal/learning \
          ./internal/toolexec ./internal/toolcat ./internal/event ./internal/session \
          ./cmd/ass-guard -race
ok  	github.com/Djarvur/ass-guard-agent/cmd/ass-guard
ok  	github.com/Djarvur/ass-guard-agent/internal/engine
ok  	github.com/Djarvur/ass-guard-agent/internal/event
ok  	github.com/Djarvur/ass-guard-agent/internal/hookdag
ok  	github.com/Djarvur/ass-guard-agent/internal/learning
ok  	github.com/Djarvur/ass-guard-agent/internal/openspec
ok  	github.com/Djarvur/ass-guard-agent/internal/session
ok  	github.com/Djarvur/ass-guard-agent/internal/toolcat
ok  	github.com/Djarvur/ass-guard-agent/internal/toolexec
$ grep -rn "TODO\|FIXME" internal/engine internal/hookdag internal/openspec internal/learning \
          internal/toolexec cmd/ass-guard/acp_serve.go cmd/ass-guard/learning_cmd.go | wc -l
0
```

Status: **GREEN** (build clean, vet clean, all 9 Phase-4 packages pass `-race`, 0 TODO/FIXME in Phase-4 production files).

## Carry-forward (not Phase-4 blockers)

- `internal/profile.TestStability_WithinSessionExtractionSource` FAILS — this is the **pre-existing Phase-1 data-source blocker** documented in STATE.md (the pinned zcode rollout session `eea3dc48` is absent; the test globs `~/.zcode/cli/rollout/model-io-sess_eea3dc48*.jsonl`). No Phase-4 file was touched in `internal/profile`. The Phase-1 parity gate remains blocked on operator action (export `ZAI_API_KEY` + re-capture a divergence-prone session) — independent of Phase 4.

## Real-openspec gate (operator-run, not CI)

The OpenSpec subprocess adapter is exercised by a stub script in tests. To run against the REAL `openspec` binary:

```
ASSGUARD_OPENSPEC_BIN=1 go test ./internal/openspec -run TestAdapter_RealOpenspecGated -race -v
```

(Skips otherwise — CI never depends on the binary, per D-13 isolation.)
