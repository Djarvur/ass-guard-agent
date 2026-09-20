---
phase: 16-acp-wire-foundation
fixed_at: 2026-08-28T18:05:00Z
review_path: .planning/phases/16-acp-wire-foundation/16-REVIEW.md
iteration: 1
findings_in_scope: 8
fixed: 8
skipped: 0
status: all_fixed
---

# Phase 16: Code Review Fix Report

**Fixed at:** 2026-08-28T18:05:00Z
**Source review:** .planning/phases/16-acp-wire-foundation/16-REVIEW.md
**Iteration:** 1
**Fix scope:** critical_warning (2 Critical + 6 Warning; the 6 Info findings are out of scope this iteration)

**Summary:**
- Findings in scope: 8
- Fixed: 8
- Skipped: 0

**Verification:** every fix was verified before its commit: re-read of the modified section (Tier 1), `go vet` + the touched package's `go test -race -count=1` + `golangci-lint run` on the touched packages (Tier 2). After all eight commits: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` (37 packages, 0 failures), and `golangci-lint run ./...` (0 issues). **All gates ran in the MAIN checkout** (`workflow.use_worktrees=false` in `.planning/config.json` — the documented opt-out; commits landed directly on `gsd/v1.2-claude-code-parity`, so the numbers are reproducible from this tree as-is).

## Fixed Issues

### CR-01: session/cancel half-reaps a session that stays registered and promptable

**Files modified:** `internal/acp/handlers.go`, `internal/acp/server.go`, `internal/acp/cancel_session_test.go` (new), `internal/runtime/runtime.go`, `internal/runtime/ask_wiring_test.go`
**Commit:** 42235ce
**Status:** fixed: requires human verification (ACP-semantics decision — confirm product intent matches turn-scoped cancel)

**Applied fix:** Took the review's primary direction: `handleSessionCancel` now calls only `st.cancelTurn()` and never `closeSessionIfPossible`. The session stays registered in `Server.sessions` and fully usable — MCP host, TranscriptWriter, session forwarder, and SessionEnd all stay live for later prompts on the same id; reaping remains on logout (`handleLogout` → `closeSessionIfPossible` + map delete) and serve teardown (`closeAllSessions`). Parked-chain drain on cancel is preserved without the reap: `runOneTurn`'s request-ctx watchdog folds the cancelled turn ctx into the parked cancel (ENG-03), exactly as the existing comment anticipated. Comments updated in `SessionCloser`, `closeSessionIfPossible`, `Runner.CloseSession`, the 13-00 park block, and the ask-wiring test. Regression test `TestSessionCancelDoesNotReapTheSession` pins: cancel → no `CloseSession`, session still in the map, re-prompt of the SAME id succeeds; logout → exactly one `CloseSession`.

### WR-03: ConfigSurface holds its mutex across blocking sends (notify lane) and the apply hook

**Files modified:** `internal/acpserve/config_surface.go`, `internal/acpserve/config_surface_lock_test.go` (new)
**Commit:** f4f971d
**Status:** fixed (behavior-preserving refactor, lock-freedom pinned by tests)

**Applied fix:** `Set` was split: `setLocked` (validate → idempotence guard → persist → drop superseded blob fill → resolve apply target → compute refreshed frames) runs under `s.mu`; the live-apply hook and the out-of-band `notify` run OUTSIDE it, in the same persist→apply→notify order. `applyLocked` became `applyTargetLocked` (pure target resolution + skip logging); the hook invocation and its error log moved to the lock-free caller. `ApplyBlobDefaults` releases `s.mu` before its `notify` send. The pending-no-op and idempotent re-push paths keep their no-emit discipline (a `doNotify` flag on the `setOutcome` struct preserves the pre-refactor emit semantics — caught by the existing `TestConfigSurface_PendingNoOp`/`TestSetIdempotent_*` pins during development). `emitLocked` removed. New tests pin that a blocked apply hook and a blocked notify send never block `Options()`.

### CR-02: _meta blob fills break the chip==wire invariant (16-09 gap 4b not closed on the blob path)

**Files modified:** `internal/runtime/runtime.go`, `internal/acpserve/config_surface.go`, `internal/acpserve/acp_serve.go`, `internal/acpserve/blob_chip_wire_test.go` (new)
**Commit:** 9bc95eb
**Status:** fixed: requires human verification (D-12 precedence edge — see note)

**Applied fix:** Took direction (a) — propagate the blob-resolved effective default across the seam. `Runner.SetDefaultTurnModel(model string) error` writes the SAME `effectiveModel` slot `ApplyTurnModel` writes, under `modelMu` (a distinct writer for a distinct semantics, not a second slot); it deliberately does no live-session restamp (the blob lands at initialize, before any session exists; future sessions stamp at construction). `ConfigSurface.SetBlobDefaultHook` + a `blobHook` field: `ApplyBlobDefaults` fires the hook only when a fill MOVED the effective tier/model (`after.model != before.model && != ""` — perm/compaction movement has no wire side), captured under the lock, fired outside it (composes with WR-03). `acpserve.Run` wires `surface.SetBlobDefaultHook(runner.SetDefaultTurnModel)`. Regression tests drive the REAL composition end-to-end: `TestBlobFillReachesTheWire` (blob `model` fill shows on the chip AND rides the next provider request, with the tier-resolved GLM-5.3 provably available as the pre-fix divergence value) and `TestBlobFillYieldsToExplicitLayer` (an explicit layer setting makes the fill inert — nothing changes, hook never fired).

**Human-verification note:** when a blob `tier` fill changes the resolved tier, the fired hook overwrites a prior editor stamp that was made under the OLD tier. This preserves chip==wire (the invariant the finding ranks highest) and cannot stomp a layer-backed stamp (a persisted layer makes any later fill inert), but it is the one semantic edge a human should confirm as intended.

### WR-01: Engine's shared per-turn rebinding cross-wires sessions under concurrency

**Files modified:** `internal/runtime/runtime.go`, `internal/runtime/enginebridge/enginebridge.go`, `internal/runtime/engine_binding_test.go` (new)
**Commit:** 93e05f6
**Status:** fixed: requires human verification (concurrency semantics — the per-turn executor also makes the HOOK-04 in-flight guard per-turn-instance; see note)

**Applied fix:** Took the review's per-invocation direction. `runOneTurn` no longer rebinds `r.hookExec.Commands/Turns/Boundaries` or `r.eng.Manager`. Instead each engine-enabled turn builds its own session-bound `*hookdag.Executor` (seams bound to THIS turn's session), its own `*enginebridge.ACPDispatcher` (carrying the per-turn executor + the shared `hookCfg`/`learned`/`bus`/`NextPromptFor`), and its own `*engine.Engine` (Manager = this session's Manager). `SetupEngine`'s `r.eng`/`r.hookExec` remain session-free Bus/Log templates; the dead shared dispatcher construction was removed and the `NextPromptFor` closure factored into `r.nextPromptFor`. A parked 13-00 chain now holds its own executor/dispatcher/engine for its whole life — a later session's turn cannot re-route its hooks or decisions. Regression test `TestEngineWiringStaysSessionFree` drives engine-enabled turns for two sessions and pins the structural property: the shared templates never carry session state (a rebinding regression fails on the first turn).

**Human-verification note:** the hook executor's HOOK-04 in-flight reentrancy guard (`inFlight` map) is now per-turn-instance. Loop prevention within one chain (one executor across all its injections) is unchanged; two DIFFERENT sessions may now run the same hook concurrently, which was previously refused. That cross-session allowance is arguably correct (independent sessions), but it is a behavior change to confirm.

### WR-02: Semaphore slot leaks when the turn panics between Acquire and Release

**Files modified:** `internal/session/session.go`, `internal/session/semaphore_test.go` (new)
**Commit:** 67e9ed4
**Status:** fixed (deterministic regression test pins the fix)

**Applied fix:** The acquire/stream/release window in `runTurn` was replaced with a `withSemaphore(ctx, fn)` helper that defers `Release` immediately after a successful `Acquire` — every exit path, including a panic unwinding from `streamAndEmit` toward the runTurn/Prompt recover guards, hands the slot back exactly once. An cancelled `Acquire` returns the wrapped ctx error, which flows through the existing `streamErr != nil` + `ctx.Err()` branch to `recordCanceled` + `stopCancelled` (externally identical to the old dedicated branch; the only observable difference is the transcript reason text for that path). Regression test `TestSemaphoreSlotSurvivesTurnPanic`: with a 1-slot semaphore, a turn whose provider panics inside `Stream` recovers to an error AND the next turn completes with `end_turn` (bounded by a ctx deadline so a leak regression fails instead of hanging).

### WR-04: A cancelled turn can report `end_turn` and fire the Stop hook

**Files modified:** `internal/session/session.go`, `internal/session/cancel_midstream_test.go` (new)
**Commit:** f81ff30
**Status:** fixed (deterministic regression test pins the D-16 contract)

**Applied fix:** Both halves of the review's fix. (1) `streamAndEmit`'s mid-loop cancellation check now returns the ctx error instead of `(partial, nil)` (the `//nolint:nilerr` is gone) — `runTurn`'s `streamErr != nil` + `ctx.Err()` branch routes to `recordCanceled` + `stopCancelled`. (2) A ctx re-check was added before Step 6, so a ctx dying between the last delivered chunk and the end_turn bookkeeping can no longer append the partial assistant message, fire the Stop hook, and answer `end_turn`. Regression test `TestCancelledMidStreamReportsCancelled` makes the race deterministic: the provider cancels the ctx INSIDE Stream (after the loop-head check passed) and returns buffered partial-text + done(end_turn) chunks; the turn must answer `cancelled` with a canceled transcript line (pre-fix it answered `end_turn`).

### WR-05: Fallback Manager construction error swallowed — nil-deref tail

**Files modified:** `internal/runtime/runtime.go`, `internal/runtime/session_fallback_test.go` (new)
**Commit:** a910194
**Status:** fixed (deterministic regression test pins the degrade)

**Applied fix:** `sessionFor` no longer discards the temp-fallback error. First failure logs loudly and retries in temp; a second failure logs the cause and returns `nil` — before any MCP spawn, so nothing needs reaping. The remaining early return is reachable only with both transcript locations unwritable; callers degrade deterministically: `Run` nil-guards and returns a typed error ("session %s unavailable: transcript manager could not be created") instead of nil-derefing at `mgr.Path()`, and the automation firing path already nil-guarded. Regression test `TestSessionUnavailableWhenTranscriptLocationsFail` poisons both locations (the work-dir path is a FILE; `TMPDIR` points at that file so the `os.TempDir()` fallback inherits the poison) and pins: `sessionFor` returns nil and `Run` returns an error — no panic.

### WR-06: Automation firing target is "most recently created", not "most recently active"

**Files modified:** `internal/runtime/runtime.go`, `internal/runtime/firing_target_test.go` (new)
**Commit:** ac20f6f
**Status:** fixed (deterministic regression test pins the contract)

**Applied fix:** Took the review's primary direction (activity marker at turn start). `sessionFor`'s reuse path now updates `r.lastSessionID = sessionID` before returning (race-free — `sessMu` spans the whole function). Since `sessionFor` is the turn-start resolve for BOTH the client `Run` path and `runAutomationTurn` (and the timer-resume path re-enters through them), the marker now tracks the most-recently-ACTIVE session exactly as `currentSessionID`'s contract comment promises; the creation-time assignment remains as the first-activity write. Regression test `TestCurrentSessionIDFollowsActivity`: create s-1, create s-2 (target = s-2), then resolve s-1 again — the target must move to s-1 (pre-fix it stayed on s-2 and every due automation fired into the unwatched session).

## Skipped Issues

None — all 8 in-scope findings were fixed.

**Out of scope (fix_scope=critical_warning):** IN-01 (dead keep-alive `ScrubError(nil)` call), IN-02 (`asciiDelete` misnomer), IN-03 (duplicate tool-loop sentinels), IN-04 (optionsLocked comment vs pending-twins blob fills), IN-05 (toolchain pin 1.26 vs documented 1.25 floor), IN-06 (blank stdin line → spurious -32700). These remain open in `16-REVIEW.md` for a follow-up pass.

---

_Fixed: 2026-08-28T18:05:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
