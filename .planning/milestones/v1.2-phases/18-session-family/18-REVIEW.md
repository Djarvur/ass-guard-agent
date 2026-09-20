---
phase: 18-session-family
reviewed: 2026-09-02T00:00:00Z
depth: standard
files_reviewed: 23
files_reviewed_list:
  - internal/session/reconcile.go
  - internal/session/list.go
  - internal/session/tombstone.go
  - internal/session/seed.go
  - internal/session/session.go
  - internal/session/manager.go
  - internal/session/planmode.go
  - internal/checkpoint/store.go
  - internal/acp/replay.go
  - internal/acp/types.go
  - internal/acp/handlers.go
  - internal/acp/server.go
  - internal/acp/emitter.go
  - internal/acpserve/acp_serve.go
  - internal/acpserve/session_store.go
  - internal/acpserve/options.go
  - internal/acpserve/command_source.go
  - internal/acpserve/config_surface.go
  - internal/runtime/runtime.go
  - cmd/ass-guard/main.go
  - cmd/ass-guard/picker.go
  - cmd/ass-guard/acp_serve.go
  - internal/acpserve/kill9_test.go
findings:
  critical: 1
  warning: 4
  info: 4
  total: 9
fixed:
  critical: 1
  warning: 4
  info: 0
status: fixed
---

# Phase 18: Code Review Report

**Reviewed:** 2026-09-02
**Depth:** standard
**Scope:** commit range 0575165..HEAD (plans 18-01..18-06), production code under `internal/` and `cmd/`
**Files Reviewed:** 23
**Status:** fixed (CR-01 4c67190, WR-01 a48435e, WR-02 a6dd607, WR-03 14b9d70, WR-04 ed5261d; IN-01..04 left open by scope decision)

**Verification:** `go build ./...`, `go vet` (touched packages), and `go test -race` on `internal/session`, `internal/acp`, `internal/acpserve` (incl. the kill -9 matrix), `internal/runtime`, `cmd/ass-guard` — all green. The findings below are race-window and contract issues the suites do not exercise.

## Summary

The session-family core is well built: the reconciliation engine is pure, total, provenance-marked, and idempotent (pinned by `TestReconcileIdempotent`); the D-03 ready-gate ordering (loading-map guard → replay → Barrier → `s.sessions` insert → `setReady`) is correct, with `promptSessionState` covering the insert→ready window; the tombstone surface is confined to `internal/session/tombstone.go` with `os.Remove` reachable only on grace-EXPIRED or orphan markers (D-20 verified by grep — no other production transcript-removal site exists); the list engine's composite cursor is internally consistent (UnixNano encode, strictly-after filter on the same comparator as the sort); the CLI picker writes only to stderr; the transcript is opened O_APPEND|O_CREATE (never truncated on resume).

Four gaps survive scrutiny, all in the seams between the new surfaces and the Phase 17 close/ask machinery: one Critical delete-vs-load race that breaks the D-07 "deleted stays deleted" invariant with a multi-second window, and three Warnings around serialization and resource re-animation on the runner-side resume path. One doc/code contradiction on the CLI flag precedence.

## Critical Issues

### CR-01: `session/delete` racing an in-flight `session/load` resurrects a deleted session (D-07 break, silent post-delete data loss) — **FIXED in 4c67190** (pin: `TestSessionDeleteDuringLoadNeverResurrects`)

**File:** `internal/acp/handlers.go:663-671, 784-788, 899-923, 998`
**Issue:** `LoadSession` checks the tombstone exactly once, before any work (line 663). The loading-guard (lines 697-730) and the final map insert (lines 784-788) never re-check it, and `closeSessionSequence` (lines 899-923) — which `handleSessionDelete` runs before writing the tombstone (line 998) — only consults `s.sessions`, never `s.loading`. So the interleaving below is unguarded:

1. Client A sends `session/load X` → tombstone check passes, id enters `s.loading`, replay begins (a large transcript replays for seconds — a wide window).
2. Client B sends `session/delete X` → `closeSessionSequence` finds no `s.sessions[X]` (still loading) and no-ops; the tombstone marker is written; the response reports success.
3. A's load finishes: replay of the now-deleted transcript, `s.sessions[X] = st`, `setReady()`.

Result: a live, prompt-accepting session for an id the user believes deleted. New turns append to a tombstoned transcript; after the next process start the session is invisible (list filters tombstones, load rejects) — every prompt accepted post-delete silently vanishes. The code's own contract comment ("deleted stays deleted; load never resurrects", lines 660-662) is violated. No test in `session_family_test.go` covers delete-during-load (checked: only load-of-tombstoned, sequentially).

**Fix:** Re-verify the tombstone under `s.mu` immediately before the insert, as the last gate:

```go
// D-03 reconcile-then-accept, D-07 resurrect guard: the insert is the last
// step, so it must also be the LAST tombstone check — a delete that raced
// the replay wins here.
s.mu.Lock()
if _, terr := os.Stat(filepath.Join(storeDir, sessionID+".deleted")); terr == nil {
	s.mu.Unlock()
	return zero, &RPCError{Code: CodeInvalidRequest,
		Message: "session/load: session " + sessionID + " was deleted during replay"}
}
s.sessions[sessionID] = st
s.mu.Unlock()
st.setReady()
```

(Optionally also have `handleSessionDelete` consult `s.loading` to surface a typed "load in progress" error instead of racing.)

## Warnings

### WR-01: `Runner.ResumeSession` skips the per-session turn mutex (17-REVIEW CR-02 discipline not extended to the load path) — **FIXED in a48435e** (pin: `TestResumeSessionSerializesWithTurnMutex`)

**File:** `internal/runtime/runtime.go:1051-1091`
**Issue:** `Run` holds `r.sessionTurnMu(sessionID)` for the whole turn (runtime.go:512-515) and every ASYNC resume driver serializes through the injected `resumeSerial` (the same mutex; runtime.go:1393-1400, per 17-REVIEW CR-02). The new `ResumeSession` — which does `ReadAll` → `Reconcile` → `AppendSynthetic` loop → `SeedResume` (a write to `turnCounter`) — takes no mutex at all. Reachable race: `session/close`/`logout` X deletes the ACP `s.sessions` entry while an armed D-01 ask timer (or pump resolution) has already claimed the pending ask and is mid-model-loop — the drain resolves queued asks, but a claimed timer resume runs to completion and `waitTurnDrain` never tracks it (turnWG only counts `session/prompt` handlers). An immediate `session/load X` then runs `ResumeSession` concurrently with that in-flight turn: both append to one transcript, and `SeedResume` can re-store the turn counter underneath a turn that already minted its next id.

**Fix:** Wrap the reconcile-append-seed block in the same mutex:

```go
func (r *Runner) ResumeSession(ctx context.Context, sessionID string) error {
	sess := r.sessionFor(ctx, sessionID)
	// ... nil guard ...
	turnMu := r.sessionTurnMu(sessionID)
	turnMu.Lock()
	defer turnMu.Unlock()
	// ReadAll → Reconcile → AppendSynthetic → SeedResume
}
```

### WR-02: `session/close` racing `session/prompt` can drain past a turn that registers afterwards — **FIXED in a6dd607** (pin: `TestPromptRegistrationAbortsAfterCloseReaped`)

**File:** `internal/acp/handlers.go:456-477` vs `internal/acp/handlers.go:899-923`
**Issue:** `handleSessionPrompt` looks the session up (`promptSessionState`, line 456) and only later registers in `st.turnWG` (line 477) — two separate steps with no common lock. `closeSessionSequence` looks up, `cancelTurn()`, `waitTurnDrain(30s)`, `CloseSession`, delete. If close's entire sequence runs between prompt's gate pass and `turnWG.Add(1)`, the Add lands on a zero WG after the drain already returned, `cancelTurn` saw no cancel func, and `Run` then executes against a session whose resources `CloseSession` just reaped (MCP host closed, ask timer disarmed, forwarder stopped) — a doomed turn that streams frames for a deleted session id and can write nothing durable. Narrow (microseconds), but the D-12 machinery exists precisely to make close-vs-turn linearizable.

**Fix:** After `st.turnWG.Add(1)`, re-confirm registration under `s.mu` and abort if the session was dropped:

```go
st.turnWG.Add(1)
s.mu.Lock()
_, stillLive := s.sessions[p.SessionID]
s.mu.Unlock()
if !stillLive {
	st.turnWG.Done()
	return nil, &RPCError{Code: CodeInvalidRequest,
		Message: "session/prompt: session " + p.SessionID + " closed while the prompt was arriving"}
}
```

### WR-03: In-process re-load after `session/close` reuses a half-reaped runner session (MCP host dead, TranscriptWriter cancelled, forwarder stopped) — **FIXED in 14b9d70** (pin: `TestInProcessReloadAfterCloseRebuildsSession`)

**File:** `internal/runtime/runtime.go:1127-1136` (sessionFor reuse branch), `internal/runtime/runtime.go:1489-1516` (OnClose reap chain)
**Issue:** `Runner.CloseSession` calls `sess.Close()` (closeOnce: `ask.Disarm`, SessionEnd hook, `OnClose` → `cancelWriter()` + `taskRegistry.ReapAll()` + `mcpHost.Close()` + forwarder stop) but leaves the entry in `r.sessions`. A subsequent `session/load X` on the same connection passes the ACP guard (the id was deleted from `s.sessions`) and `sessionFor` returns the cached Session unchanged — nothing re-arms the reaped resources. The resumed session then runs with `mcp__*` calls routed to a closed host, no async TranscriptWriter (streaming audit lines silently stop), no session forwarder, and a SessionEnd hook that can never fire again. The CLI resume path (fresh process) is unaffected; this hits the editor flow of close-then-restore on one connection. This is the same "half-reaped Session" class as 16-REVIEW CR-01.

**Fix:** Either drop the entry from `r.sessions` in `CloseSession` (so the next `sessionFor` reconstructs fully), or have `ResumeSession` detect a closed session (`Close` was run) and rebuild it before adopting.

### WR-04: `--continue` + `--resume` both typed: code lets `--continue` win, doc (and intent) say `--resume` wins — **FIXED in ed5261d** (pin: `TestResumeWinsOverContinue`)

**File:** `cmd/ass-guard/acp_serve.go:96-107`
**Issue:** `resolveResumeTarget`'s doc comment ends "`--resume` wins when both resume flags are typed", but the body checks `rf.cont` first and returns `continueMostRecent(dir)` unconditionally — with both flags typed, the explicit `--resume <target>` is silently ignored and the newest session loads instead. No test in `resume_flags_test.go` pins the combination (checked: only single-flag forms and the `--prompt` conflict), so whichever behavior is intended is unverified. A user typing `--continue --resume <specific-id>` gets a different session than asked for with no warning.

**Fix:** Decide and pin. If the comment is right:

```go
if rf.cont && rf.resume == "" {
	return continueMostRecent(dir)
}
target := rf.resume
if target == pickSentinel {
	if len(rf.args) > 0 {
		target = rf.args[0]
	} else if rf.cont {
		return continueMostRecent(dir) // bare --resume + --continue: newest
	} else {
		return pickerChoice(dir, pick)
	}
}
```

plus a `--continue --resume <id>` case in `TestResumeFlagParsing`/`TestResolveResumeTarget`.

## Info

### IN-01: CLI resume emits replayed `session/update` frames before `initialize`

**File:** `internal/acpserve/acp_serve.go:349-354`
**Issue:** `Run` performs the ResumeTarget load before `srv.Serve`, so replayed notifications hit stdout before the client's `initialize`. This is plan-pinned (18-06: "the client pipe observes the replay frames"), but a strict initialize-first ACP client may drop or reject pre-initialize notifications. Worth a note in the 18-06 user-facing docs so pipe consumers know frames precede the handshake.

### IN-02: Tombstone GC runs only once per serve start

**File:** `internal/acpserve/acp_serve.go:217-230`
**Issue:** The sweep is startup-only ("the chosen trigger point" per D-09). A serve process living longer than the grace window never purges expired tombstones until restart — deletes stay reversible longer than configured, and disk reclamation is deferred. Functionally safe (list/load stat-filter markers regardless of age); consider a periodic sweep if long-lived processes become the norm.

### IN-03: `logout` does not use the new cancel-and-drain sequence

**File:** `internal/acp/handlers.go:1028-1051` vs `handlers.go:899-923`
**Issue:** 18-04's `closeSessionSequence` (drain → cancelTurn → waitTurnDrain → reap → delete) is applied to `session/close` and `session/delete` but `handleLogout` still drains asks, reaps, and deletes without cancelling or waiting for in-flight turns. Typically benign (logout accompanies connection teardown, where the serve ctx dies and turns unwind), but an in-flight turn can outlive the logout response and keep streaming frames for a deleted session id. Consider routing logout through the same sequence for uniformity.

### IN-04: Stray positional arg silently ignored when `--continue` is typed

**File:** `cmd/ass-guard/main.go:49-52` with `cmd/ass-guard/acp_serve.go:105-107`
**Issue:** `rootArgsValidator` allows one positional arg whenever any resume flag is active, but `resolveResumeTarget`'s `--continue` branch never reads `rf.args` — `ass-guard --continue foo` runs with `foo` silently ignored. Harmless but undocumented; either reject the arg when `--continue` is set or document it.

---

_Reviewed: 2026-09-02_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
