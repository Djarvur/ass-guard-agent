---
phase: 16-acp-wire-foundation
reviewed: 2026-08-28T13:38:01Z
depth: deep
files_reviewed: 34
files_reviewed_list:
  - .mise.toml
  - internal/acp/emitter.go
  - internal/acp/emitter_soak_test.go
  - internal/acp/emitter_test.go
  - internal/acp/framer.go
  - internal/acp/handlers.go
  - internal/acp/handlers_test.go
  - internal/acp/metrics.go
  - internal/acp/request_registry.go
  - internal/acp/request_registry_test.go
  - internal/acp/server.go
  - internal/acp/server_test.go
  - internal/acp/types.go
  - internal/acpserve/acp_serve.go
  - internal/acpserve/chip_truth_test.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/config_test.go
  - internal/acpserve/simulator_e2e_test.go
  - internal/modelrouting/config.go
  - internal/modelrouting/load.go
  - internal/modelrouting/load_test.go
  - internal/providerfactory/config_write.go
  - internal/providerfactory/config_write_test.go
  - internal/providerfactory/goconst_constants.go
  - internal/runtime/apply_model_test.go
  - internal/runtime/cron_wiring.go
  - internal/runtime/emitter_e2e_test.go
  - internal/runtime/integration_test.go
  - internal/runtime/runtime.go
  - internal/session/manager.go
  - internal/session/session.go
  - internal/session/transcript.go
  - internal/session/transcript_newkinds_test.go
  - internal/session/turn_model_test.go
findings:
  critical: 2
  warning: 6
  info: 6
  total: 14
status: issues_found
---

# Phase 16: Code Review Report (re-review after 16-07/16-08/16-09)

**Reviewed:** 2026-08-28T13:38:01Z
**Depth:** deep
**Files Reviewed:** 34
**Status:** issues_found

## Summary

Re-review of the phase-16 ACP wire foundation after the gap-closure plans landed. The 16-07 CR-01 Barrier broadcast fix is correct (per-generation close+swap under one critical section; the concurrent-waiter regression test with never-cancelled contexts genuinely pins the lost-wakeup), the 16-08 scope-aware idempotence basis is correct (addressed-layer comparison; the "global write persists when combined matches" test pins the silent-swallow fix), and the 16-09 wire-side chip pin (TestDefaultTurnModel_FollowsTierResolution) is correct for the layer path. The D-14/D-15/D-17/D-19 registry work, the atomic 0600 layer writer, and the extended transcript kinds (redaction-exempt raw_thinking, local_command, compaction) all hold up under adversarial tracing, including the Pitfall-8 teardown ordering (handlerWG → registry.Stop → emitter.Stop → Writer.Close).

Two blockers remain. First, the chip==wire invariant that 16-09 closed for the LAYER path is still open for the _meta blob path: the advertisement resolves currentValues through the surface's in-memory blob overlay, but the wire-side default (`Runner.defaultTurnModel`) reads only `schedCfg` and can never see blob fills — the exact divergence class (chip shows X, wire sends Y) the operator observed at turn-001. Second, `session/cancel` reaps session-scoped resources (MCP host, transcript writer, session forwarder, SessionEnd hook) while leaving the session registered and promptable — ACP cancel semantics are per-turn, so the routine cancel-then-re-prompt flow runs every later turn of that session without MCP tools and without audit streaming.

Beyond the blockers: a cross-session contamination window in the engine's shared per-turn rebinding, a semaphore slot leak on the panic path the turn loop explicitly defends against, the ConfigSurface holding its mutex across blocking sends, a narrow cancelled-turn stop-reason mislabel, a swallowed fallback error with a nil-deref tail, and a documented-vs-implemented mismatch in the automation firing target.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: session/cancel half-reaps a session that stays registered and promptable

**File:** `internal/acp/handlers.go:446-451` (with `internal/runtime/runtime.go:1436-1456`, `internal/session/session.go:304-325`)

**Issue:** In ACP v1, `session/cancel` cancels the current prompt **turn**; the session persists and the client is expected to re-prompt the same `sessionId` (the standard editor flow: user presses escape, then asks again). The handler cancels the turn (correct) but then calls `closeSessionIfPossible` → `Runner.CloseSession` → `Session.Close()`, which runs the once-chain: `ask.Disarm()`, the `SessionEnd` hook, and `OnClose` — which stops the session-lifetime chunk forwarder, cancels the TranscriptWriter ctx, reaps background tasks, and closes the MCP host. Crucially the handler does **not** delete the session from `Server.sessions` or `Runner.sessions`, so every later `session/prompt` on that id still resolves to the half-reaped `Session`:

- `mcp__*` tool calls fail forever (host closed; `toolcat.MCPExecutor` answers "unknown server");
- `request_shaped`/usage audit lines stop streaming (TranscriptWriter ctx dead);
- server-driven turn forwarding for the session is gone (forwarder unsubscribed);
- `SessionEnd` already fired, so hook-based lifecycle accounting is skewed.

The half-reaped state is inconsistent under either interpretation — if cancel meant session-end, the session should be removed from both maps (as `handleLogout` does); if it means turn-cancel (the ACP meaning), no session-scoped resources should be reaped.

**Fix:** Keep `cancelParkedChains` (the D-03 off-switch) and drop the resource reaping from the cancel path; reap only on `logout` and serve teardown:

```go
// handleSessionCancel — cancel the turn only; the session stays live for the next prompt.
st.cancelTurn()
// removed: s.closeSessionIfPossible(p.SessionID)
return nil, nil
```

If product intent really is "cancel ends the session", then delete the session from `s.sessions` in the same handler so the next prompt cannot reach the reaped Session — but that contradicts the ACP turn-cancel contract and the D-16 stopReason "cancelled" flow, which explicitly anticipates the client continuing.

### CR-02: _meta blob fills break the chip==wire invariant (16-09 gap 4b not closed on the blob path)

**File:** `internal/acpserve/config_surface.go:312-319` (with `internal/runtime/runtime.go:1067-1075, 1543-1563`)

**Issue:** 16-09 pinned chip==wire for the layer path: the advertisement's bare `model` currentValue equals the resolver evaluation over the layer files, and the wire stamps the same value (`defaultTurnModel`). But the advertisement's `resolveLocked` also applies the **in-memory `_meta` blob overlay** (`blobFills` for `tier` and `model`, fills-unset per D-10), while the wire side cannot see it at all — `defaultTurnModel` reads only `r.schedCfg.SessionTier` + the resolver, and the blob lives privately inside the `ConfigSurface` (the acp→runtime seam carries no blob state). Concretely, on a connection whose initialize `_meta` carries `{"model":"glm-5.2"}` (or a `tier` fill) with no operator layer setting that slot and no editor stamp:

- chip: `optionsLocked` → `resolveLocked` → model = blob fill (glm-5.2);
- wire: `sessionFor` → `effectiveModelFor()` == "" → `defaultTurnModel()` → tier-resolved floor model (GLM-5.3).

The user sees the chip claim one model while the provider request carries another — the exact operator-observed turn-001 divergence (`chip_truth_test.go:4-9`) that 16-09 declared "cannot recur without failing a test". The existing pins (`TestConfigAdvertisement_ResolverTruth`, `TestDefaultTurnModel_FollowsTierResolution`) exercise layers only, so the blob path is untested end-to-end; the simulator test only blobs `compaction-threshold`, which the wire legitimately ignores today.

**Fix:** Make one side honest. Either (a) propagate the blob-resolved effective default across the seam — e.g. after `ApplyBlobDefaults` reports a change, have the composition stamp the runner's pre-stamp default (a distinct `SetDefaultTurnModel` writing `effectiveModel` without marking it an explicit editor stamp), or (b) stop advertising blob-filled values as `currentValue` for `model`/`tier` (keep fills visible only for the pending options that have no wire side), documenting the D-11 deviation. (a) preserves D-11 and is the smaller semantic change:

```go
// acpserve.Run, after the notify wiring:
surface.SetBlobDefaultHook(func(model string) { // fired by ApplyBlobDefaults when tier/model moved
    _ = runner.SetDefaultTurnModel(model) // stamps the default the wire will use pre-editor-stamp
})
```

with `SetDefaultTurnModel` writing `r.effectiveModel` under `modelMu` exactly as `ApplyTurnModel` does, so chip and wire read the same slot.

## Warnings

### WR-01: Engine's shared per-turn rebinding cross-wires sessions under concurrency

**File:** `internal/runtime/runtime.go:762-770`

**Issue:** `runOneTurn` rebinds package-shared engine state to the *current* session on every engine-enabled turn: `r.hookExec.Commands/Turns/Boundaries = …(sess)` and `r.eng.Manager = sess.Manager`. Client turns serialize per session (`sessionTurnMu`), but two *different* sessions turn concurrently (per-request goroutines, distinct mutexes), and a parked engine chain (13-00) resumes **without** holding any of those mutexes after a rebind. A hook DAG or post-settle injection dispatched for session A after session B's `runOneTurn` rebound the executors sends prompt-turns and boundary writes into B's Manager/transcript. Engine decisions land in the wrong session's transcript; hook `send-prompt` turns run against the wrong session.

**Fix:** Make the bindings per-invocation instead of shared mutable fields: pass the session-bound runners through the `BridgeConfig`/`Observe` call (a closure over `sess` captured at the `runOneTurn` site), or guard rebind + chain execution under a single engine-wide mutex. E.g. build a per-turn `*hookdag.Executor` copy and hand it to the adapter/dispatcher rather than storing it on the shared `r.hookExec`.

### WR-02: Semaphore slot leaks when the turn panics between Acquire and Release

**File:** `internal/session/session.go:385-397`

**Issue:** `runTurn` acquires `s.Semaphore`, then calls `streamAndEmit` and `toolexec.DispatchBatch` (which executes arbitrary catalog/MCP tool code) before releasing. The recover guards live at the `runTurn`/`Prompt` deferred level, so a panic anywhere in that window unwinds past the `Release()` line and the slot is never returned. Repeated panics (a flaky MCP tool bridge is enough) permanently consume the concurrency budget — after `maxConc` leaks, every future turn blocks forever in `Acquire`, wedging the agent with no diagnostic.

**Fix:** Release deterministically — acquire in a helper that defers the release, so every path between Acquire and Release (including panics recovered upstream) hands the slot back exactly once:

```go
func (s *Session) withSemaphore(ctx context.Context, fn func() (provider.Response, string, error)) (provider.Response, string, error) {
    if s.Semaphore == nil {
        return fn()
    }
    if err := s.Semaphore.Acquire(ctx); err != nil {
        return provider.Response{}, "", err
    }
    defer s.Semaphore.Release()
    return fn()
}
```

### WR-03: ConfigSurface holds its mutex across blocking sends (notify lane) and the apply hook

**File:** `internal/acpserve/config_surface.go:200-213, 505-542` (with `internal/acp/handlers.go:104-128`)

**Issue:** `Set`/`ApplyBlobDefaults` run under `s.mu` and call `emitLocked` → `srv.NotifyConfigOptions` → `EmitterHandle.Notify`, a **blocking** send on the foreground lane (bounded 128; blocks while the emitter drain is wedged on a slow client), and `applyLocked` → `runner.ApplyTurnModel`, which blocks on every live session's turn mutex (the documented mid-turn wait). While blocked, `s.mu` is held, so every other config operation — including `handleInitialize`'s `applyMetaBlob` — queues behind client backpressure or an in-flight turn. This is the exact lock-across-blocking-send pattern the emitter explicitly avoids (Pitfall 3), and it puts the initialize handshake's latency under the worst client stall of the connection. No deadlock today (the turn path never takes `s.mu`), but the serialization is unbounded in time.

**Fix:** Compute the refreshed option set under the lock, then send and apply outside it:

```go
s.mu.Lock()
… persist/apply bookkeeping …
frames := s.optionsLocked()
notify, hook := s.notify, s.applyHook
s.mu.Unlock()
if notify != nil { notify(sessionID, frames) }
```

(The apply hook already carries its own serialization contract via the turn mutex; moving it below the unlock preserves the persist-then-apply ordering for the responding caller while unblocking other config operations.)

### WR-04: A cancelled turn can report `end_turn` and fire the Stop hook

**File:** `internal/session/session.go:657-661, 567-577`

**Issue:** `streamAndEmit`'s mid-loop cancellation check returns `(partial resp, text, nil)` — a **nil** error with ctx already cancelled (the `//nolint:nilerr` path). `runTurn`'s `streamErr != nil` guard therefore doesn't route to the cancelled branch. If the partial response carries no tool calls (only text chunks arrived before the abort), control falls to the Step-6 end_turn branch: it appends the partial assistant message, **fires the `Stop` hook**, and returns `mapStopReason("")` = `"end_turn"`. The D-16 contract ("report stopReason `cancelled`") is broken in this race window (ctx dies between chunk deliveries while the provider goroutine hasn't closed the stream yet), and hooks observe a Stop for a turn the user cancelled.

**Fix:** Make the mid-loop check honest — return the ctx error (the caller already maps it to `stopCancelled` via the `streamErr != nil` + `ctx.Err()` branch), and additionally re-check ctx before Step 6:

```go
case blockText:
    …
    if err := ctx.Err(); err != nil {
        return resp, sb.String(), err
    }
```

```go
// before Step 6 in runTurn:
if err := ctx.Err(); err != nil {
    s.recordCanceled(turnID, "context cancelled before turn end")
    return stopCancelled, nil
}
```

### WR-05: Fallback Manager construction error swallowed — nil-deref tail

**File:** `internal/runtime/runtime.go:1014-1018`

**Issue:** When `session.NewManager(dir, …)` fails, the fallback discards the second error: `mgr, _ = session.NewManager(filepath.Join(os.TempDir(), "ass-guard"), …)`. `NewManager` returns `(nil, err)` when its directory cannot be created/opened, so a double failure (read-only workdir + unwritable temp) leaves `mgr == nil`; the very next use `ecosys.NewHookRunner(…, mgr.Path())` dereferences nil. The panic is recovered by the server's dispatch recover (client gets -32603 on every prompt), but the session is permanently broken with only a generic internal error — the real cause (both transcript locations unwritable) is never logged.

**Fix:** Log and surface the fallback failure; degrade deterministically instead of nil-deref:

```go
mgr, err := session.NewManager(dir, sessionID, redactorAdapter{})
if err != nil {
    log.Printf("ass-guard: transcript open failed for %s (%v); retrying in temp", dir, err)
    mgr, err = session.NewManager(filepath.Join(os.TempDir(), "ass-guard"), sessionID, redactorAdapter{})
    if err != nil {
        log.Printf("ass-guard: transcript open failed in temp too: %v", err)
        return nil // callers already nil-guard sessions (expandUserBlocks precedent)
    }
}
```

(then nil-guard `mgr` at the `NewHookRunner`/`s.Manager` sites, matching the existing `sess == nil || sess.Manager == nil` discipline).

### WR-06: Automation firing target is "most recently created", not "most recently active"

**File:** `internal/runtime/cron_wiring.go:59-68` (with `internal/runtime/runtime.go:1254-1257`)

**Issue:** `currentSessionID`'s contract comment says "the most recently ACTIVE session (the firing target — the project's automations fire into the session the operator is driving)". The only writer is `sessionFor`, and only on **creation** (`r.lastSessionID = sessionID` at line 1257); re-prompting an older session never updates it. With two sessions on one connection (session/new is client-controlled), after creating session B every due automation fires into B even while the operator is actively driving A — turns (and their tool side effects) land in a session nobody is watching.

**Fix:** Update the marker at turn start instead of construction: in `Run` (and the timer-resume path), after resolving `sess`, set `r.lastSessionID = sessionID` under `sessMu` — or record a `lastActiveAt` per session and have `currentSessionID` return the max.

## Info

### IN-01: Dead keep-alive call in handleSessionSetMode

**File:** `internal/acp/handlers.go:489`

**Issue:** `_ = redact.ScrubError(nil) // keep redact import live` — a no-op call whose only purpose is pinning an import. Dead code that will confuse the next reader; if the import were genuinely needed it would be used, not kept warm.

**Fix:** Remove the call; re-add the import when real scrubbing lands in this handler.

### IN-02: Misleading uuidV4 constant name `asciiDelete`

**File:** `internal/acp/handlers.go:13, 530`

**Issue:** `const asciiDelete = 0x40` — 0x40 is `'@'`, not ASCII DEL (0x7F). The value is correct (it ORs the version nibble to 4 in `(b[6]&0x0F)|0x40`), but the name asserts a wrong fact about the code. `internal/session` names the same value `uuidVersionV4` correctly.

**Fix:** Rename to `uuidVersionV4` (matching `internal/session/manager.go:141`), keeping the literal.

### IN-03: Duplicate identical error sentinels in the tool-loop bound

**File:** `internal/session/session.go:23-24`

**Issue:** `errToolLoopExceededMax` and `errToolLoopExceeded` carry the identical message "tool loop exceeded max iterations"; one goes to the transcript, one is returned. Two names for one fact invites drift, and `errors.Is` against either sentinel misses the other.

**Fix:** Keep one sentinel; use it at both the `appendError` and the return site (distinguish by wrapping: `fmt.Errorf("session: %w", errToolLoopExceeded)` for the caller-facing form).

### IN-04: optionsLocked comment contradicts the code for the _global pending twins

**File:** `internal/acpserve/config_surface.go:580-584` vs `624-628` (and `233-245`)

**Issue:** The comment says the twins "describe a layer FILE; the blob channel stays on the bare options", but for the pending twins the code calls `s.pendingCurrentLocked(...)`, which **does** apply `blobFills` — so `_global/permissions.mode` and `_global/compaction-threshold` advertise blob-derived values, contradicting the stated invariant. Relatedly, `ApplyBlobDefaults` strips the `_global/` prefix before lookup, so a `_global/model` blob key silently fills the bare option's slot (scope-flattening the comment never mentions).

**Fix:** Either route the pending twins through a fill-less resolver (they have no layer truth today — plain defaults), or amend the comment to state that pending options (bare and twins) advertise the blob fill and that blob keys are scope-flattened by design. Either way, make comment and code agree and pin the chosen semantics in `config_test.go`.

### IN-05: Toolchain pin (go 1.26) diverges from the documented supported floor (1.25)

**File:** `.mise.toml:2` (with `go.mod`: `go 1.26`)

**Issue:** The workspace stack document says to pin `go 1.25` in go.mod as the supported floor; the repo declares `go 1.26` and mise installs 1.26. That raises the minimum toolchain above the documented floor and silently changes the compatibility contract for contributors and CI.

**Fix:** Either lower go.mod to `go 1.25` (the code uses nothing newer than `sync.WaitGroup.Go`, which is 1.25) or update the stack document to record 1.26 as the floor. One line either way; the point is that doc and build agree.

### IN-06: A blank stdin line produces a spurious -32700 error frame

**File:** `internal/acp/framer.go:63-66` (with `internal/acp/server.go:344-353`)

**Issue:** `readFrame` returns `errEmptyFrameLine` for a blank line, and `Serve` routes every read error that isn't EOF to `handleParseError`, which writes a `{"id":null,"error":{"code":-32700}}` frame to stdout. A client (or transport layer) that emits a stray blank line gets a protocol-level error response for a frame it never sent; the line is information-free — there is nothing to parse or report.

**Fix:** Treat an empty line as a silent skip — return a `(nil, nil)` sentinel from `readFrame` and `continue` in `Serve` when both are nil, or special-case `errEmptyFrameLine` before `handleParseError`.

---

_Reviewed: 2026-08-28T13:38:01Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
