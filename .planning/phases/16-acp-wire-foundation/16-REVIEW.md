---
phase: 16-acp-wire-foundation
reviewed: 2026-08-27T20:13:26Z
depth: deep
files_reviewed: 23
files_reviewed_list:
  - internal/acp/emitter.go
  - internal/acp/request_registry.go
  - internal/acp/server.go
  - internal/acp/handlers.go
  - internal/acp/types.go
  - internal/acp/metrics.go
  - internal/acp/framer.go
  - internal/acp/emitter_test.go
  - internal/acp/emitter_soak_test.go
  - internal/acpserve/acp_serve.go
  - internal/acpserve/config_surface.go
  - internal/runtime/runtime.go
  - internal/runtime/cron_wiring.go
  - internal/runtime/apply_model_test.go
  - internal/session/manager.go
  - internal/session/session.go
  - internal/session/transcript.go
  - internal/session/transcript_newkinds_test.go
  - internal/modelrouting/config.go
  - internal/modelrouting/load.go
  - internal/providerfactory/config_write.go
  - internal/providerfactory/goconst_constants.go
  - .mise.toml
findings:
  critical: 1
  warning: 5
  info: 5
  total: 11
status: issues_found
---

# Phase 16: Code Review Report

**Reviewed:** 2026-08-27T20:13:26Z
**Depth:** deep
**Files Reviewed:** 23
**Status:** issues_found

## Summary

Reviewed the Phase 16 diff (f799b07..HEAD): the ordered TurnEmitter, the outbound request registry, the config layer writer, the ACP-08 config surface, extended transcript kinds, and the simulator/soak coverage. Cross-file call chains were traced through acp → acpserve → runtime → session → event bus, with the phase's core claim (concurrency: emitter ordering, registry pending-map, cancel cascade) scrutinized hardest.

The primitives are well-built overall: the emitter's two-stage foreground select gives genuine preemption (the saturated-lane test pins it), the registry's `done` CAS gives exactly-once resolution with clean retry/late-response handling, the config writer's temp-then-rename is correctly ordered (0600 chmod before rename, temp removal on every failure path), and D-23's redaction exemption is genuinely type-scoped with a counting-fake test pinning zero redactor calls. The live-apply seam's turn-mutex serialization is verified by a mid-turn race test.

One critical liveness defect was found: `Barrier` uses a capacity-1 wake channel that hands each token to at most one waiter, so two concurrent `session/prompt` turns can leave the second barrier waiting forever (its ctx never dies in production), hanging the prompt response. The soak test does not catch this because its concurrent barriers deliberately use short-lived ctxs that escape via `ctx.Done` — production passes the long-lived serve ctx. Five warnings and five info items follow.

The previously recorded pre-stamp Model chip divergence (runtime.go:131 vs config_surface.go:320/:398) is NOT re-litigated here. WR-05 below is a distinct defect in the same surface (the `_global/` scoped twins), not that finding.

## Narrative Findings (AI reviewer)

### Critical Issues

#### CR-01: Barrier lost-wakeup hangs the prompt response with two concurrent waiters

**File:** `internal/acp/emitter.go:181,261-285,304-308` (caller: `internal/acp/handlers.go:400`)
**Issue:** `writeOut` publishes one token into `wake` (capacity 1) per written frame; `Barrier` blocks receiving from it. Go hands a channel send to exactly ONE blocked receiver, so with two goroutines inside `Barrier` simultaneously, each wake token progresses at most one of them. When both waiters' targets are satisfied by the same final frame, the last token wakes only one; the other re-checks nothing (it never receives), the queue is empty so no further tokens are ever produced, and its `ctx`/emitter-ctx escape arms never fire in production — `handleSessionPrompt` passes the long-lived serve/request ctx, not a per-turn deadline. The result is a `session/prompt` response that never returns for the second concurrent turn (two sessions prompting on one connection is supported: requests dispatch on per-request goroutines and Barrier is per-turn). The single-waiter case is safe (stale tokens are consumed and re-checked), which is why every test — including the soak's four concurrent barriers, all deliberately given short-lived ctxs that exit via `ctx.Done` — passes.
**Fix:** Broadcast instead of single-token handoff. Simplest correct shape: swap the channel per generation under `mu` and close it to wake all waiters.

```go
// writeOut (after written++ under mu):
t.wake-close under mu: ch := t.wake; close(ch); t.wake = make(chan struct{})

// Barrier wait arm:
t.mu.Lock(); ch := t.wake; t.mu.Unlock()
select {
case <-ch:            // generation closed — re-check written under mu
case <-ctx.Done():    return
case <-t.ctx.Done():  return
}
```
Alternatively a `sync.Cond` (with a deadline watchdog goroutine for ctx escape), or per-Barrier waiter channels registered in a slice that `writeOut` drains-and-closes. Add a regression test with two Barriers whose targets complete on the same final frame and long-lived ctxs.

### Warnings

#### WR-01: Data race — `emitter.metrics` written after the sampler goroutine started

**File:** `internal/acp/server.go:183` (write) vs `internal/acp/emitter.go:145,384` (read)
**Issue:** `NewTurnEmitter` starts `sampleLoop` before returning; `NewServer` then assigns `s.emitter.metrics = s.metrics` with no synchronization. `sampleStall` reads `t.metrics` from the sampler goroutine. This is an unsynchronized cross-goroutine write/read — a data race per the Go memory model that `-race` (the standing CI gate per D-04) can flag whenever a lane goes full in the startup window. Impact is bounded (worst case: one missed stall count), but it is exactly the defect class this phase claims to have under discipline.
**Fix:** Arm metrics before the goroutines start — pass the `*Metrics` through `TurnEmitterConfig` (or a `NewTurnEmitter` parameter) and set the field in the constructor before `go em.drain()` / `go em.sampleLoop()`.

#### WR-02: Session forwarder fan-in goroutines never exit — teardown contract is wrong

**File:** `internal/runtime/cron_wiring.go:249,269-285,288-320` (contract source: `internal/event/bus.go:47-63,88-95`)
**Issue:** The new `fanInEvents` machinery (3 pump goroutines + WaitGroup + merger + consumer = 5 goroutines per session forwarder, up from 1) exists so "the stream closes — and the goroutine exits — when stop unsubscribes and the sources drain closed". That is factually wrong: `Bus.Unsubscribe` removes the channel from the fan-out list but never closes it — only `Bus.Close` closes subscriber channels, and nothing in acpserve/runtime/cmd ever calls `Bus.Close`. After a session's stop() runs, the three pumps block forever on `range src`, `wg.Wait` never returns, `close(out)` never happens, and the consumer ranges `merged` forever. The result is a permanent 5-goroutine leak per closed session in any long-lived process, plus a comment that documents a mechanism the code does not have. (The pre-existing single forwarder goroutine had the same leak; this phase multiplied it and asserted the fix that isn't there.)
**Fix:** Either have the stop() closure close the source channels it owns (making `Unsubscribe` + explicit `close(ch)` the teardown pair, mirroring `Bus.Close` semantics for private subscribers), or drop `fanInEvents` and select over the three channels directly like `startChunkForwarder` does (its exit is ctx/done-driven, not close-driven). At minimum, correct the comment so the next reader does not trust the phantom contract.

#### WR-03: Registry `send` TOCTOU vs `Writer.Close` — latent send-on-closed-channel panic

**File:** `internal/acp/request_registry.go:387-401` (send), `:319-348` (Stop); teardown at `internal/acp/server.go:326-332`; `internal/acp/framer.go:117-121,137-149`
**Issue:** `send` reads `r.closed` under the registry mutex, then writes to the Writer outside it. `Serve`'s teardown sequence is `registry.Stop()` → `emitter.Stop()` → `out.Close()`, and `Writer.Write` is `w.ch <- msg` while `Close` does `close(w.ch)` — a Call that has passed the `closed` check but not yet executed its Write when teardown completes panics with "send on closed channel". Today every registry caller is a handler-scoped goroutine (the initialize probe), and `handlerWG.Wait` precedes `registry.Stop`, so the window is closed for current callers. But the shipped `Stop` contract claims "no write may race the closing Writer" while enforcing it only by caller convention; the first non-handler caller (server-driven turns, Phase 17 background asks) opens the panic path.
**Fix:** Make the guard structural, not conventional: either give `Writer.Write` a closed check (atomic flag + recover/`sync.Once`-guarded send, or an RWMutex shared with Close), or have the Registry track in-flight Calls with a WaitGroup that `Stop` joins before returning (after marking closed so no new sends start). Document the remaining guarantee honestly either way.

#### WR-04: ConfigSurface mutex held across the live-apply hook — one mid-turn Set blocks the whole config surface for the rest of the turn

**File:** `internal/acpserve/config_surface.go:149-207` (Set), `:435-464` (applyLocked); `internal/runtime/runtime.go:1470-1498`
**Issue:** `Set` holds `s.mu` for its entire body, including `applyLocked` → `applyHook` → `Runner.ApplyTurnModel`, which blocks on the session's turn mutex until the in-flight turn finishes — deliberately (the mid-turn serialization is tested and correct in isolation). But because the wait happens under `s.mu`, one `session/set_config_option` arriving mid-turn stalls every other config operation until the turn ends: `Options()` (initialize and session/new advertisements), other Sets, and `ApplyBlobDefaults` all queue behind it. The requesting handler goroutine also stays blocked for the remainder of what can be a minutes-long agent turn, and `Serve`'s `handlerWG.Wait` teardown inherits the same wait. Waiting for the turn is the documented intent; holding the surface-wide lock while doing it is not.
**Fix:** Snapshot, validate, and persist under `s.mu` (D-07's persist-then-apply ordering only needs the persist inside the lock); release `s.mu`, then invoke the apply hook and the notify emission. ApplyTurnModel's own modelMu/turnMu discipline already makes the apply safe outside the surface lock; the refresh for the return value can be re-taken under `s.mu` after the apply.

#### WR-05: `_global/` option twins advertise the combined effective value — global-scope writes are mis-displayed and can be silently swallowed

**File:** `internal/acpserve/config_surface.go:182-191` (idempotence guard), `:513-530` (advertisement), `:398-404`, `:411-428`
**Issue:** Two related defects in the D-08 scope surface. (1) The `_global/model` and `_global/tier` twins are built with `res.model`/`res.tier` — the PROJECT-won combined effective values — so the editor renders the project's value under "Model (global default)". (2) The idempotence guard `val == s.effectiveFor(bare, res.tier, res.model)` is scope-blind: `Set("_global/model", X)` where X equals the combined effective value returns "idempotent re-push, no layer write" and the global layer is never written — the operator's explicitly global-scoped mutation is silently dropped (it would not survive a later removal of the project-layer key). D-11's "current EFFECTIVE value" reads naturally for the project-scope options; for a global-default option the effective value of the GLOBAL layer is the truthful display, and the idempotence comparison must be against that layer's value.
**Fix:** Resolve the global twins' `currentValue` from the global layer alone (load the global file through `modelrouting.Load(s.globalPath)`), and make the idempotence guard scope-aware: compare against the addressed layer's current value, not the combined resolution. Distinct from the recorded pre-stamp chip finding — this concerns the `_global/` namespace, not the tier-resolved vs profile-slug divergence.

### Info

#### IN-01: Dead redact call kept only to pin an import

**File:** `internal/acp/handlers.go:489`
**Issue:** `_ = redact.ScrubError(nil) // keep redact import live` in `handleSessionSetMode` is dead code masking an unused import — the compile error is the honest signal.
**Fix:** Drop the line and the import; re-add both when real scrubbing lands.

#### IN-02: `asciiDelete` misnames the UUID version bits

**File:** `internal/acp/handlers.go:13,530`
**Issue:** `const asciiDelete = 0x40` — 0x40 is `@`; ASCII DEL is 0x7F. The value is the RFC 4122 version-4 bit pattern (`0b0100_0000`) OR-ed into byte 6. The misleading name sits in a crypto-adjacent id path (the session package's twin names the same constant correctly: `uuidVersionV4`).
**Fix:** Rename to `uuidVersionV4` (matching `internal/session/manager.go`), same for the variant constant.

#### IN-03: raw_thinking "verbatim / byte-identical / no re-serialization" overclaims the mechanism

**File:** `internal/session/manager.go:106-128`; comments at `internal/session/transcript.go:50-56,143-150`
**Issue:** D-23's actual invariant — the Redactor never sees thinking bytes — is real and tested. But the comments claim the payload is "never re-serialized" and retained "byte-identical": `appendLineUnredacted` calls `json.Marshal(line)`, and Go's encoder re-processes embedded `json.RawMessage` through `compact` (stripping insignificant whitespace, HTML-escaping `<>&` under the default `escapeHTML`). The JSON value is preserved; the bytes are not guaranteed identical. Later phases lock against this transcript contract (D-20 is one-way), so the on-disk guarantee should be stated accurately.
**Fix:** Reword the comments to "JSON-value-preserving; the Redactor is never invoked" (or marshal with an `escapeHTML=false` encoder if byte-fidelity is actually required).

#### IN-04: Malformed response frame (id, no method, neither result nor error) produces a spurious -32601

**File:** `internal/acp/server.go:361-365`
**Issue:** The response-interception condition requires `Result != nil || Error != nil`; a response-shaped frame carrying neither falls through to request dispatch, and `handleRequest` answers method `""` with a -32601 error echoing the peer's id — protocol garbage in reply to a malformed peer frame. Low likelihood (Zed controls that side), but the drop-and-log path used for unknown registry ids is the better neighbor.
**Fix:** Treat `ID != nil && Method == ""` with neither payload field as a malformed response: log structured and drop, mirroring `registryUnknownResponseLogFormat`.

#### IN-05: `SetNotify`/`SetApplyHook` write ConfigSurface fields without the surface mutex

**File:** `internal/acpserve/config_surface.go:126-134`
**Issue:** `s.notify` and `s.applyHook` are read under `s.mu` (emitLocked/applyLocked) but written without it. Safe today only because the Run composition wires both strictly before `srv.Serve` starts reading stdin; any future late wiring or re-wiring is a data race.
**Fix:** Take `s.mu` in both setters (they are called once at startup; the lock is free), or document the wired-before-serve constraint at the field declarations.

---

_Reviewed: 2026-08-27T20:13:26Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
