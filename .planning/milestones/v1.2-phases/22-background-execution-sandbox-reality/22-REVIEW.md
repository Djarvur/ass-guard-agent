---
phase: 22-background-execution-sandbox-reality
reviewed: 2026-09-10T01:23:37Z
depth: standard
files_reviewed: 49
files_reviewed_list:
  - internal/tasks/tracker.go
  - internal/tasks/notify.go
  - internal/tasks/tracker_test.go
  - internal/tasks/notify_test.go
  - internal/runtime/wake_wiring_test.go
  - internal/coreexec/background.go
  - internal/coreexec/background_test.go
  - internal/runtime/runtime.go
  - internal/runtime/cron_wiring.go
  - internal/redact/redact.go
  - internal/redact/redact_test.go
  - internal/coreexec/procopts_linux.go
  - internal/coreexec/procopts_other.go
  - internal/coreexec/procopts_linux_test.go
  - internal/coreexec/bash.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/config_test.go
  - internal/acpserve/config_surface_lock_test.go
  - internal/acpserve/acp_serve.go
  - internal/acpserve/serve_test.go
  - internal/tasks/subagent.go
  - internal/tasks/subagent_test.go
  - internal/session/subagent.go
  - internal/session/session.go
  - internal/session/subagent_test.go
  - internal/coreexec/ansistrip.go
  - internal/coreexec/ansistrip_test.go
  - internal/coreexec/ptty.go
  - internal/coreexec/ptty_test.go
  - internal/runtime/pty_wiring_test.go
  - internal/coreexec/register.go
  - internal/toolcat/coretools.json
  - internal/toolcat/catalog_test.go
  - internal/sandbox/policy.go
  - internal/sandbox/policy_test.go
  - internal/sandbox/seatbelt_darwin.go
  - internal/sandbox/seatbelt_darwin_test.go
  - internal/sandbox/landlock_linux.go
  - internal/sandbox/landlock_linux_test.go
  - internal/sandbox/child_linux.go
  - internal/sandbox/doc.go
  - internal/coreexec/sandboxchild_test.go
  - cmd/ass-guard/acp_serve.go
  - cmd/ass-guard/main.go
  - internal/acpserve/options.go
  - internal/coreexec/bash_test.go
  - cmd/ass-guard/background_wiring_test.go
findings:
  critical: 4
  warning: 8
  info: 5
  total: 17
status: issues_found
---

# Phase 22: Code Review Report

**Reviewed:** 2026-09-10T01:23:37Z
**Depth:** standard
**Files Reviewed:** 49
**Status:** issues_found

## Narrative Findings (AI reviewer)

### Summary

Phase 22's surface area (task notifications, background Bash, background subagents, persistent PTY shell, landlock/sandbox-exec split) is heavily tested and several phase invariants check out under direct inspection: stdout stays ACP-only (every diagnostic sink is stderr; the serve batteries pin it), the sandbox is default-OFF with a loud probe-and-degrade taxonomy and argv-identity on the off path, the group-kill ladder (TERM → grace → KILL + bounded reap) is funneled through one `terminateGroup` and preserved across the wrapped children, sentinel output-spoofing is defeated by the crypto/rand nonce + digit guard, and persistent calls serialize on the manager mutex. `go vet` is clean across all reviewed packages and `creack/pty` is pinned at v1.1.24 in both go.mod and go.sum.

However, the review found four Critical defects, all in the new background-notification machinery: a subagent-cap counter that never decrements (bricking background subagents in a session after 8 total dispatches), a wake-drain chain that unconditionally restarts itself forever (a permanent CPU-burning gorotation per session), a missing panic recovery on the background-subagent goroutine (one panicking subagent kills the whole agent process), and session-close "resurrection" where late task completions reconstruct and re-run a closed session. The tracker/wake test batteries pass because none of them re-register after completions, none measure CPU after test end, and none exercise panics or close-then-complete orderings.

## Critical Issues

### CR-01: `runningSubagents` is never decremented — background subagents permanently queue after SubagentCap total starts

**File:** `internal/tasks/tracker.go:74`, `internal/tasks/tracker.go:163-176`, `internal/tasks/tracker.go:194-214`
**Issue:** The `runningSubagents` counter is incremented in `RegisterSubagent` (line 164) and again in `startNextWaiter` (line 205, "the freed slot is reoccupied by the waiter"), but NO code path ever decrements it — `grep -rn runningSubagents` across the repo shows only the field declaration and the two `++` sites. `Complete` (lines 115-133) does not release the completing subagent's slot; the "freed slot" is only implicitly acknowledged by the waiter's increment, so the counter monotonically grows.

Consequence: `RegisterSubagent`'s admission check `t.runningSubagents < t.opts.SubagentCap` (line 163) is permanently false after `SubagentCap` (default 8) **total** starts in a session — the completions need not even be concurrent. Every subsequent background-subagent dispatch in that session joins `t.waiters` and, since no subagent is running to fire `Complete` → `startNextWaiter`, it sits queued forever. The model is told "it starts when a slot frees" (QueuedNote) and it never does — silent starvation of the 22-03 feature after 8 uses. `TestTrackerQueue_FIFODrainOrder` and friends pass only because they never register again after their completions fire.

**Fix:**
```go
// tracker.go — Complete:
if n.Kind == KindSubagent {
	t.releaseSubagentSlot()   // the completing subagent frees its slot
}

// new helper: decrement (floored), then admit exactly one waiter
func (t *Tracker) releaseSubagentSlot() {
	t.mu.Lock()
	if t.runningSubagents > 0 {
		t.runningSubagents--
	}
	t.mu.Unlock()
	t.startNextWaiter()
}
```
And add a regression test that registers `cap` tasks, completes them all, then registers once more and asserts `queued == false`.

### CR-02: `wakeDrainChain` restarts itself unconditionally — an infinite, never-idle goroutine chain per session

**File:** `internal/runtime/cron_wiring.go:393-400` (defer), `internal/runtime/cron_wiring.go:369-383` (`scheduleWakeDrain`)
**Issue:** `wakeDrainChain`'s deferred function runs on EVERY exit path:

```go
defer func() {
	flag.Store(false)
	// A completion raced the exit: restart so the batch is not stranded
	r.scheduleWakeDrain(sessionID)
}()
```

The chain's documented contract ("exits when the queue is empty") is broken by its own exit handler: when the chain exits because `PendingPeek()` is empty (line 412-414), the defer clears the in-flight flag and calls `scheduleWakeDrain`, whose CAS on the just-cleared flag always succeeds, spawning a successor chain — which again finds the queue empty, exits, and restarts. This is a self-perpetuating spawn loop that never terminates: one goroutine at a time per session, spinning at full speed (sync.Map ops + tracker mutex + goroutine spawn per cycle) for the life of the process after the first completion a session ever sees. Worse, the restart is not even gated on the serve context: after `ctx.Err() != nil` the chain returns at the loop top, the defer still restarts it, and the successor dies the same way — the churn continues through and past serve shutdown for as long as the process lives. `TestWakeTurn_EmptyPendingNoTurn` passes because the spin produces no provider calls and the test binary exits before the damage is observable.

**Fix:**
```go
defer func() {
	flag.Store(false)
	// Restart ONLY if a completion genuinely raced the exit (pending
	// non-empty) AND the serve ctx is still live.
	tr := r.trackerFor(sessionID)
	if ctx.Err() == nil && tr != nil && len(tr.PendingPeek()) > 0 {
		r.scheduleWakeDrain(sessionID)
	}
}()
```
Additionally, `scheduleWakeDrain` should refuse to spawn when `r.serveCtxOrBackground().Err() != nil`.

### CR-03: No panic recovery in the background-subagent goroutine — one panicking subagent crashes the whole ass-guard process

**File:** `internal/tasks/subagent.go:77-100`
**Issue:** `RunBackgroundSubagent`'s launcher goroutine calls `dep.Run(ctx, w.append)` with no `recover()`:

```go
go func() {
	defer cancelFn()
	result, rerr := dep.Run(ctx, w.append)
	...
}()
```

`dep.Run` is `runSubagentWithProgress` → the session's nested turn loop (provider streaming, restricted tool execution, transcript writes). The FOREGROUND dispatch site guards exactly this class with a `recover()` at the goroutine boundary (`internal/session/subagent.go:105-126`, whose doc states the D-13 invariant: "A panic is recovered at the goroutine boundary … the process NEVER crashes", pinned by `TestSubagentPanicRecovery`). The background leg has no such guard, so any panic inside a background subagent's loop takes down the entire agent — the ACP server, every session, every PTY shell — violating the project's own stated invariant. The panic also bypasses `w.finish`, leaving a dangling output file, and skips `Tracker.Complete`, wedging the slot accounting (compounding CR-01).

**Fix:** mirror the foreground recovery inside the goroutine:
```go
go func() {
	defer cancelFn()

	defer func() {
		if r := recover(); r != nil {
			w.finish("error", fmt.Sprintf("subagent panic: %v", r))
			dep.Tracker.Complete(Notification{
				TaskID: id, Kind: KindSubagent, ExitStatus: "error",
				Duration: time.Since(started), Tail: w.tail(), OutputFile: logPath,
			})
		}
	}()

	result, rerr := dep.Run(ctx, w.append)
	...
}()
```

### CR-04: Late task completions resurrect a closed session — `drainWakeNotifications` reconstructs evicted sessions and runs provider turns into them

**File:** `internal/runtime/cron_wiring.go:441-448` (`drainWakeNotifications` → `r.sessionFor`), `internal/runtime/runtime.go:2783-2809` (`CloseSession`), `internal/runtime/runtime.go:2348-2362` (OnClose)
**Issue:** `CloseSession` evicts the session from `r.sessions` but never removes the session's entry from `r.trackers` or `r.wakeInFlight` (grep confirms no `trackers.Delete`/`wakeInFlight.Delete` anywhere). Completions still land after close: `OnClose` → `taskRegistry.ReapAll()` kills live tasks whose Wait goroutines then fire `CompletionHook` → `tracker.Complete` → `scheduleWakeDrain`; running background SUBAGENTS are not cancelled at close at all (only queued ones are dropped via `CancelQueued`), so their completions land minutes later. The wake chain then calls `drainWakeNotifications`, whose first step is `sess := r.sessionFor(ctx, sessionID)` — and since the session was evicted, `sessionFor` **reconstructs a brand-new full session** for the closed id (fresh MCP host subprocesses, transcript writer, PTY manager, task registry) and proceeds to hold the turn mutex, drain the batch, and run a real model wake turn (`runOneTurn` → provider call) into a session the operator explicitly closed. This is ghost activity: provider cost, transcript appends, and spawned subprocesses attributable to nobody, plus the resurrected session lingering in `r.sessions` afterwards.

**Fix:** at `CloseSession`, after evicting, delete the per-session notification state and make the drain treat a missing tracker as terminal:
```go
// CloseSession, after delete(r.sessions, sessionID):
r.trackers.Delete(sessionID)
r.wakeInFlight.Delete(sessionID)
```
and in `drainWakeNotifications`, replace the constructing `r.sessionFor(...)` with a non-constructing lookup (`r.sessions[sessionID]` under `sessMu`); when the session is gone, log once and drop the batch (return `true` so the chain exits instead of retrying).

## Warnings

### WR-01: TaskStop/TaskOutput cannot address background-subagent tasks — `Tracker.CancelTask` is dead code

**File:** `internal/coreexec/background.go:700-742` (`TaskOutputExecute`), `internal/coreexec/background.go:764-792` (`TaskStopExecute`), `internal/tasks/tracker.go:216-228` (`CancelTask`)
**Issue:** Both executors consult only the `*TaskRegistry` (bash tasks). A background subagent's `exec_<hex>` id lives in the `tasks.Tracker`, so the model — handed exactly that id by the `async_launched` payload — gets `taskstop: unknown task` / `taskoutput: unknown task` for it. `Tracker.CancelTask`'s own doc calls it "22-03's TaskStop leg", but it has zero production callers (grep: only `tracker_test.go`). The captured TaskOutput schema (coretools.json) explicitly promises "Works with all task types: background shells, async agents, and remote sessions" — so the model-visible contract is broken, and background subagents are unstoppable through the tool surface for their entire (serve-lifetime) run.
**Fix:** thread the session's tracker (or a `Cancel func(id string) bool` / output-lookup seam) into `TaskStopExecute`/`TaskOutputExecute` at the `RegisterInteractive` site: on `errUnknownTask`, fall through to `tracker.CancelTask(id)`; wire the runtime adapter in `sessionFor` (it already holds both the registry and the tracker).

### WR-02: `Stop` on an already-terminal task runs the kill ladder on a possibly recycled pgid

**File:** `internal/coreexec/background.go:585-597`
**Issue:** `Stop` early-returns only for `bgQueued`/nil-cmd. For a task in `bgDone`/`bgFailed` (already reaped by `cmd.Wait`), it still calls `terminateGroup(cmd.Process.Pid, ...)`, which sends SIGTERM (and after grace, SIGKILL) to `-pid`. The child was already reaped, so the kernel may have reused the pid; the negative-pid kill can then TERM/KILL an **unrelated** process group. The tests only exercise Stop-on-exited within milliseconds (`TestEscalation_AlreadyGoneIsSuccess`), where reuse is unlikely — the exposure is a stale task id stopped much later in a long session.
**Fix:** treat terminal states as no-ops: `if prev == bgQueued || prev == bgDone || prev == bgFailed || cmd == nil || cmd.Process == nil { return nil }`.

### WR-03: Persistent-shell sentinel can be swallowed by stdin-consuming commands — 120s wedge + shell-state loss

**File:** `internal/coreexec/ptty.go:407-417`, `internal/coreexec/ptty.go:372-435`
**Issue:** The command and the sentinel echo ride ONE stdin pipe (`script := command + "\n" + sentinelEchoLine(nonce) + "\n"`). The nonce+digits guard defeats OUTPUT spoofing (verified by the adjacency battery), but a model-authored command that reads stdin — an accidental bare `cat`, `read x`, `sudo`/`mysql` password prompts — consumes the sentinel line itself. The sentinel then never executes, `readWindow` blocks until the schema timeout (default 120000ms), and the ctx-cancel path calls `markDeadLocked`, SIGKILLing the whole persistent shell and its state (the D-08 restart note fires on the next call). The per-call cost is a two-minute wedge of the serialized persistent shell plus lost cwd/env; the session turn is blocked for that window.
**Fix:** deliver the sentinel out-of-band from the command's stdin — e.g. hold the sentinel back and write it via a second write only after the command's output quiesces, or open a spare fd (`exec 9>` pipe) for sentinels. At minimum, document the hazard in the Bash tool's `persistent` property description so the model avoids stdin-readers in persistent mode, and consider a shorter default persistent timeout.

### WR-04: The startup stale sweep tombstones oversize-Bash output files, breaking `<persisted-output>` pointers

**File:** `internal/acpserve/acp_serve.go:524-571` (`sweepStaleTaskLogs`), `internal/acpserve/acp_serve.go:515-523` (T-22-07 claim), `internal/coreexec/bash.go:300-320` (`persistOversize`)
**Issue:** `sweepStaleTaskLogs` renames EVERY `.ass-guard/outputs/*.log` to `.log.stale` at startup and deletes `.stale` entries after 7 days. But `persistOversize` writes `bash-*.log` files into that same directory (bash.go:308, `os.CreateTemp(dir, "bash-*.log")`), and those paths are embedded in tool results as "Full output saved to: <path>" — pointers a resumed session's transcript still references. After any restart, every such pointer 404s (renamed), and after 7 days the content is gone. The comment's claim "only the <id>.log / <id>.log.stale shapes this process family creates are touched (T-22-07)" is factually wrong — `bash-*.log` is the same process family in the same dir.
**Fix:** constrain the sweep to the task-id shape (`exec_*.log`) via prefix match, or move oversize persists to a sibling dir (`.ass-guard/outputs/oversize/`) that the sweep never touches.

### WR-05: DefaultPolicy grants read-write on the ENTIRE shared system temp directory

**File:** `internal/acpserve/acp_serve.go:176-178` (`sandboxPolicyFor` → `os.TempDir()`), `internal/sandbox/policy.go:45-54`
**Issue:** The D-04 "rw triple" in production is `workDir + os.TempDir() + workDir/.ass-guard`. `os.TempDir()` is the world-shared `/tmp` (or `/var/folders/...` per-user on macOS): every confined model command can read, write, and execute anything there — other ass-guard sessions' scratch data, unrelated local processes' tmp files and sockets. The test batteries use a per-session private tmp (`filepath.Join(t.TempDir(), "session-tmp")` in policy_test.go, and a private dir on darwin), so the live tests never exercise the production breadth. On multi-user Linux hosts this is a real confinement weakness (cross-session/cross-process tampering via /tmp).
**Fix:** allocate a per-serve (or per-session) private tmp directory (`os.MkdirTemp(os.TempDir(), "ass-guard-")`) at startup and pass that as the policy's tmpDir, matching the tested shape.

### WR-06: Seatbelt profile interpolates paths unescaped — a `"` in a workdir breaks the filter syntax

**File:** `internal/sandbox/policy.go:100-108`
**Issue:** `SeatbeltProfile` builds `(subpath "` + resolveForSeatbelt(path) + `")` by concatenation. Paths are operator/session-sourced (not model input), but nothing rejects or escapes a double quote: a workdir containing `"` produces a malformed or — with crafted content — a semantically widened profile (e.g. closing the string early and appending another `(subpath ...)` grant), silently altering confinement on darwin. The landlock leg is safe (JSON-marshaled env), so the two backends can also diverge, violating the "drift impossible by construction" claim.
**Fix:** in `resolveForSeatbelt` (or at profile build), reject paths containing `"`/non-printables with a loud error (Wrap then degrades unconfined-with-note per SAND-01), or escape per seatbelt's string syntax.

### WR-07: Queued-then-dropped subagent tasks leak their output-file handle

**File:** `internal/tasks/subagent.go:56-110`, `internal/tasks/tracker.go:260-268` (`CancelQueued`)
**Issue:** `RunBackgroundSubagent` creates and writes the header to the log file BEFORE registration. On the queue-bound error path the file is closed, and a started task closes it in `finish` — but a task that is queued and then dropped by `CancelQueued` (session close, or `TestDispatchBackground_QueuedOverCap`'s cleanup) is neither started nor closed: `w.f` stays open for the life of the process. A session that queues many subagents and closes leaks one fd per dropped task.
**Fix:** have `CancelQueued` return the dropped waiter ids (or accept a per-id cleanup func registered at queue time) and close the corresponding writers; alternatively defer an idempotent close on the subagentWriter itself.

### WR-08: ANSI stripper terminates an OSC sequence at a bare backslash — escape payload leaks into the tool result

**File:** `internal/coreexec/ansistrip.go:63-67`
**Issue:** In the OSC scan, `if s[j] == '\\' && j > i+2 { j++; break }` treats ANY mid-OSC backslash as the ST terminator. Real ST is `ESC \` (already handled separately at lines 69-76); a plain backslash inside the title text (e.g. a shell printing `ESC]0;C:\path\a`) terminates the "sequence" early, so the OSC tail (`path` + the raw BEL byte) survives into the stripped output. The documented scope ("OSC sequences … are stripped whole") is violated and a control byte reaches the tool result.
**Fix:** delete the bare-backslash branch — keep only the BEL, ESC-`\` (ST), and next-ESC cases:
```go
for j < len(s) {
	if s[j] == '\a' { j++; break }
	if s[j] == '\x1b' {
		if j+1 < len(s) && s[j+1] == '\\' { j += 2 }
		break
	}
	j++
}
```

## Info

### IN-01: Stale menu-size doc comments

**File:** `internal/acpserve/config_surface.go:306`, `internal/acpserve/config_surface.go:1292-1293`
**Issue:** `Options` is documented as "the full eight-entry menu" and `optionsLocked` as "the twelve-entry menu"; the menu is now 20 entries (pinned as 20 by `config_surface_lock_test.go:32`). Misleading docs on a frequently edited surface.
**Fix:** update both comments to "twenty-entry" or make them count-free ("the full menu").

### IN-02: Dead no-op conditional in TestBackgroundOutput_ZeroOutput

**File:** `internal/tasks/subagent_test.go:216-220`
**Issue:** `if strings.TrimSpace(n.Tail) == "" == false { /* comment only */ }` — an always-taken, empty-bodied conditional with a doubled `==` comparison; it asserts nothing and reads like a forgotten assertion. Test-reliability blemish on an otherwise assertive battery.
**Fix:** replace with a real assertion (e.g. the notification tail contains no run-output marker beyond the header/marker text) or delete the block.

### IN-03: Per-session maps are never pruned on session close

**File:** `internal/runtime/runtime.go:295-301` (`trackers`, `wakeInFlight`, `ptyManagers`), `internal/runtime/runtime.go:2783-2809`
**Issue:** `turnMus`, `turnActive`, `trackers`, `wakeInFlight`, and `ptyManagers` grow monotonically; `CloseSession` evicts `r.sessions` only. For a long-lived serve cycling many sessions (editor close/restore flows), each closed session leaves five map entries behind. Small, but unbounded — and it is also the substrate that makes CR-04 possible.
**Fix:** delete these entries in `CloseSession` after eviction (ptyManagers only after `Drain`), keeping the turn mutex if in-flight holders may still reference it.

### IN-04: subagentWriter ignores output-file write errors

**File:** `internal/tasks/subagent.go:205-214` (`writeLocked`)
**Issue:** `if _, err := w.f.Write(p); err == nil { _ = w.f.Sync() }` — a failed write (ENOSPC, closed fd) is silently skipped while `tailBuf` still accumulates the text. The notification tail then shows output the durable `.ass-guard/outputs/<id>.log` (the thing the model is told to Read) does not contain, and nothing is ever surfaced.
**Fix:** record the first write error on the writer and include it in the terminal marker (`[error: write failed: …]`) so mid-run Reads and the notification stay truthful.

### IN-05: Wake drain retries forever with per-retry stderr spam when the session cannot be constructed

**File:** `internal/runtime/cron_wiring.go:442-448`
**Issue:** When `sessionFor` returns nil (both transcript locations unusable), `drainWakeNotifications` returns false and the chain sleeps 500ms and retries indefinitely — one stderr line per attempt, forever. The "notifications stay pending" posture is safe but the unbounded log spam masks real diagnostics.
**Fix:** bound the retry count (then drop the batch loudly once) or rate-limit the message to the first occurrence.

---

**Verified invariants (no findings):** stdout is ACP-only across the server path (all sinks are `stderrOrDefault()`/`os.Stderr`; `TestACPServeWiresStdoutClean`/`TestACPServeNoStdoutPollutionFromLogs` pin it); sandbox default-OFF with zero probing, loud one-time degrade, per-run unconfined notes + process-wide counter, and argv identity on the off path; group-kill ladder funneled through one `terminateGroup` with ESRCH-clean and bounded reap, surviving wrapped children; PTY capture ANSI-stripped (modulo WR-08) and never touching stdout; persistent-call serialization on the manager mutex with arrival-order probe; TaskOutput timeout units match the schema (ms); `creack/pty v1.1.24` pinned; `go vet` clean on all reviewed packages.

_Reviewed: 2026-09-10T01:23:37Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
