# Phase 22: Background Execution + Sandbox Reality - Pattern Map

**Mapped:** 2026-08-28
**Files analyzed:** 17 (9 new, 8 modified/tested)
**Analogs found:** 13 / 17 exact-or-role matches; 4 sandbox-leg files have no in-repo analog (first OS-sandbox code) — RESEARCH.md Patterns 4–6 + live-verified examples cover them

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/tasks/tracker.go` (NEW) | service (registry) | event-driven | `internal/coreexec/background.go` TaskRegistry | exact |
| `internal/tasks/notify.go` (NEW) | service (queue/coalescer) | pub-sub | `internal/runtime/cron_wiring.go` queue semantics + `bgTask.exitCh` | role-match |
| `internal/tasks/subagent.go` (NEW) | service (async runner) | event-driven | `internal/session/subagent.go` + `runtime.go:1229-1234` serve-ctx discipline | exact |
| `internal/sandbox/policy.go` (NEW) | config/model (pure types) | transform | `bashArgs` struct + documented-const convention (`bash.go:47-71`) | role-match |
| `internal/sandbox/landlock_linux.go` (NEW, `//go:build linux`) | system-integration service | process-lifecycle | none in repo — RESEARCH §Code Examples (probe + ruleset) | no analog |
| `internal/sandbox/child_linux.go` (NEW, `//go:build linux`) | utility (re-exec entrypoint) | process-lifecycle | none in repo — RESEARCH Pattern 4 | no analog |
| `internal/sandbox/seatbelt_darwin.go` (NEW, `//go:build darwin`) | system-integration service | transform | degrade discipline of `acp_serve.go:77-88` seedACPGuard | partial |
| `internal/coreexec/ptty.go` (NEW) | service (process lifecycle) | streaming | `background.go` TaskRegistry + `bash.go` group-kill idiom | exact |
| `internal/coreexec/ansistrip.go` (NEW) | utility (pure) | transform | `bash.go` trimCaptured/previewFirstChars pure-helper convention | role-match |
| `internal/coreexec/background.go` (MOD) | service | process-lifecycle | itself — queue-on-cap (D-11) reverses `errBgCap` path | self |
| `internal/coreexec/bash.go` (MOD) | executor stub | request-response | itself — persistent arg (D-09), sandbox wrap (D-04), flag semantics | self |
| `internal/session/subagent.go` + `session.go:432-470` (MOD) | controller (dispatch site) | request-response | itself + TaskRegistry immediate-return form | self |
| `internal/runtime/cron_wiring.go` (MOD) | orchestrator | event-driven | itself — `runAutomationTurn` IS the wake-turn template (D-01) | self |
| `internal/runtime/runtime.go` (MOD) | composition root | wiring | itself — OnClose chain `1238-1265`, sessionFor wiring `1107-1138` | self |
| `internal/acpserve/config_surface.go` (MOD) | config surface | request-response | itself — `optionsLocked()` menu builder (D-12) | self |
| `internal/acpserve/acp_serve.go` (MOD) | composition root | wiring | itself — Run startup pipeline; flag + probe + sweep join it | self |
| tests (`tasks/`, `sandbox/`, `coreexec/ptty_test.go`, `ansistrip_test.go`, `background_test.go` MOD) | test | — | `background_test.go`, `cron_wiring_test.go` battery style | exact |

**Planner correction vs RESEARCH.md:** RESEARCH's Wave-0 item "rewrite `background_test.go` cap cases" is imprecise — `errBgCap`/`bgConcurrentCap` have **zero test references today** (grep-verified: only definition site `background.go:145-149,321`). D-11's queue contract needs **new** cap tests, plus the existing six `TestBackground_*` batteries must keep passing (behavior of the happy paths is unchanged).

## Pattern Assignments

### `internal/tasks/tracker.go` (service, event-driven)

**Analog:** `internal/coreexec/background.go` — copy the per-session registry shape wholesale.

**Registry struct + per-session scoping** (lines 95-104, 30-40):
```go
type TaskRegistry struct {
	mu    sync.Mutex
	tasks map[string]*bgTask
}

func NewTaskRegistry() *TaskRegistry {
	return &TaskRegistry{tasks: map[string]*bgTask{}}
}
```
Constants are documented corpus-absent defaults in a single const block (`bgConcurrentCap = 16`, `bgOutputCapBytes`, `bgTaskOutputDefaultMS` — lines 30-40). The tracker's `background.subagents=8` / `background.bash=16` caps (D-10/D-11/D-12) follow the same documented-constant style, but become **injected config**, not package consts (they are operator-tunable).

**Task id minting** (lines 107-116) — subagent task ids reuse this verbatim:
```go
func newTaskID() string {
	var b [16]byte
	_, rerr := rand.Read(b[:])
	if rerr != nil {
		return fmt.Sprintf("exec_%d", time.Now().UnixNano()) // unreachable fallback
	}
	return "exec_" + hex.EncodeToString(b[:])
}
```

**State machine + completion signal** (lines 42-64, 185-203):
```go
type bgState string
const (
	bgRunning bgState = "running"
	bgDone    bgState = "done"
	bgStopped bgState = "stopped"
	bgFailed  bgState = "failed"
)
// exitCh chan struct{} closed in the Wait goroutine — the notification
// subsystem's completion hook replaces/augments this channel close:
go func() {
	defer close(task.exitCh)
	waitErr := cmd.Wait()
	task.mu.Lock()
	/* state transition */ 
	task.mu.Unlock()
}()
```
The tracker adds a **kind** (`TaskKind`: `bash`/`subagent`) and a **per-task cancel func** so TaskStop works across both legs (PAR-07) — detection is by struct field, never text-match.

**Structured error taxonomy** (lines 320-324 + `bash.go:388-395`):
```go
var (
	errBgCap       = errors.New("coreexec: background cap")
	errUnknownTask = errors.New("coreexec: unknown task")
	errNoRegistry  = errors.New("coreexec: background: no task registry configured")
)
// rendering at the stub boundary:
func marshalStructured(msg string, err error) (json.RawMessage, error) {
	out, mErr := json.Marshal(map[string]string{keyError: msg})
	...
	return out, err
}
```

---

### `internal/tasks/notify.go` (service, pub-sub)

**Analog:** `internal/runtime/cron_wiring.go` (queue/TryLock semantics) + `bgTask.exitCh` (completion signal).

**Pending-queue discipline — queue, never interrupt** (cron_wiring.go lines 35-39, 153-160):
```go
func (r *Runner) sessionTurnMu(sessionID string) *sync.Mutex {
	mu, _ := r.turnMus.LoadOrStore(sessionID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}
// inside runAutomationTurn — THE coalescing/drain guard D-01/D-03 require:
mu := r.sessionTurnMu(sessionID)
queued := !mu.TryLock()
if queued {
	mu.Lock() // queue behind the active turn — serialized, never interrupts
}
defer mu.Unlock()
```
The notify queue mirrors this: completions append to a per-session pending list (ordered by completion time, deduped by task id — D-03); a single drain point TryLocks the SAME `sessionTurnMu` before calling `runOneTurn`. The TryLock makes concurrent drains impossible by construction (RESEARCH Pitfall 8).

**Completion signal → notification** (background.go lines 63, 185-187):
```go
exitCh chan struct{}   // closed exactly once when Wait returns
// consumers select on it — notify.go's drain trigger selects the same way
```

---

### `internal/tasks/subagent.go` (service, event-driven)

**Analog:** `internal/session/subagent.go` DispatchSubagent + the serve-lifetime-ctx discipline in `runtime.go`.

**Goroutine + recover + result channel** (subagent.go lines 59-101) — the async leg reuses this shape but does NOT select on the caller's ctx:
```go
type outcome struct { result string; err error }
resCh := make(chan outcome, 1)
go func() {
	defer func() {
		r := recover()
		if r != nil {
			stack := debug.Stack()
			err := fmt.Errorf("subagent panic: %v", r)
			_ = s.Manager.AppendError(subagentTurnID, "subagent", err.Error(), nil, false, string(stack))
			...
			resCh <- outcome{err: err}
		}
	}()
	result, err := runner.Run(ctx, s, subagentTurnID, parentTurnID, prompt, restricted, agentDef)
	resCh <- outcome{result: result, err: err}
}()
select {
case <-ctx.Done():          // ← FOREGROUND ONLY. Background mode MUST NOT select here:
	...                     //   the turn ends when dispatch returns; a turn-scoped ctx
case o := <-resCh:          //   kills the subagent instantly (RESEARCH Anti-Pattern).
	...
}
```
Background mode runs the nested loop under a **session/serve-lifetime ctx**; cancellation rides the tracker's per-task cancel func.

**Serve-lifetime ctx pattern** (runtime.go lines 1229-1234 + 1275-1279):
```go
// 09-01 T2 (LOG-02/D-20): ONE TranscriptWriter per session, running for
// the session's lifetime on a context derived from the SERVE ctx (never
// the per-turn ctx — a writer bound to a turn dies after turn 1). Reaped
// via OnClose...
writerCtx, cancelWriter := context.WithCancel(r.serveCtxOrBackground())
go tw.Run(writerCtx)
```
The background subagent's ctx + its output-file writer follow this exact discipline; the cancel func is stored in the tracker for TaskStop.

**Output-file streaming** — copy `openTaskLog` + tee discipline (background.go lines 120-136, 168-169):
```go
dir := filepath.Join(workDir, ".ass-guard", "outputs")
logPath := filepath.Join(dir, id+".log")
f, err := os.Create(logPath) // the id is hex-minted, not model input
...
cmd.Stdout = io.MultiWriter(f, taskWriter{task, false})
cmd.Stderr = io.MultiWriter(f, taskWriter{task, true})
```
Subagent progress (text chunks, tool results) appends to the same `.ass-guard/outputs/<id>.log` family (Pitfall 9); on cancel append a killed marker line so Read always resolves.

**Discriminated result** (PAR-07) — foreground form for reference (session.go 460-461 returns the final text); background form per RESEARCH Pattern 2:
```json
{"status":"async_launched","task_id":"exec_...","output_file":"<workdir>/.ass-guard/outputs/exec_....log"}
```
`renderBackgroundStart` (background.go 435-440) is the prose-form precedent for the bash leg; the subagent leg returns the structured status form.

---

### `internal/sandbox/policy.go` (config, transform)

**Analog (partial):** `bashArgs` + documented-constants convention (`bash.go:47-71`).
```go
type bashArgs struct {
	Command                   string  `json:"command"`
	Timeout                   float64 `json:"timeout"`
	RunInBackground           bool    `json:"run_in_background"`
	DangerouslyDisableSandbox bool    `json:"dangerouslyDisableSandbox"` //nolint:tagliatelle // captured input key
}
```
Policy struct per RESEARCH Pattern 4 (no in-repo analog):
```go
type Policy struct {
	RWPaths     []string // workdir, tmp, .ass-guard (D-04)
	ROSysPaths  []string // per-distro ro set (discretion)
	DenyNetwork bool     // always true for v1 (D-04)
}
```
One struct feeds both backends (D-06 symmetry — golden-test it: same Policy → landlock rules list AND rendered `.sb`).

---

### `internal/sandbox/landlock_linux.go` + `child_linux.go` (NO IN-REPO ANALOG)

Use RESEARCH.md §Code Examples verbatim (probe idiom, `RestrictPaths`/`RestrictNet` v4, re-exec sentinel `__ASS_GUARD_SANDBOX_CHILD=1`, `syscall.Exec`). Convention to copy from the repo: build-tag header + documented consts + loud-degrade errors. **Anti-patterns already locked:** never `BestEffort()` as enforcement (silent fail-open); never apply landlock in ass-guard itself (D-05).

### `internal/sandbox/seatbelt_darwin.go` (partial analog)

**Analog (degrade discipline only):** `acp_serve.go:75-88`:
```go
// seedACPGuard runs the Phase-6 first-run seed (D-04) and logs the outcome to
// stderr. A failed seed is non-fatal — the agent stays runnable on defaults.
func seedACPGuard(workDir string) {
	seeded, err := firstrun.Ensure(workDir)
	if err != nil {
		log.Printf("ass-guard: first-run seeding failed (continuing): %v", err)
		return
	}
	...
}
```
Probe `sandbox-exec` at first use; absence/`-p` failure → ONE loud stderr note + counter, runs unconfined. Profile rendered IN MEMORY via `sandbox-exec -p '<rendered>'` (D-06: no on-disk artifacts). RESEARCH §Code Examples has the live-verified profile text.

---

### `internal/coreexec/ptty.go` (service, streaming)

**Analog:** `background.go` TaskRegistry (per-session owner; registry owns lifecycle, not a call ctx) + `bash.go` group-kill idiom.

**Lifecycle ownership comment is the contract to copy** (background.go lines 164-172):
```go
//nolint:noctx // T-8-33: model-authored command is the product; the
// registry — not a call ctx — owns this lifecycle (Cancel unset; Stop/ReapAll kill)
cmd := exec.Command("sh", "-c", command)
cmd.Dir = workDir
// Own process group (no cmd.Cancel — the REGISTRY owns this lifecycle,
// not a call ctx; Stop/ReapAll do the group killing).
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
```
The PTY shell manager is the same: ONE per session, lazy-started (D-07), a mutex serializing persistent calls, dead-shell detection → lazy restart **with a visible note** (D-08 — model-visible note + stderr counter, the `renderBackgroundStart` prose style).

**Group kill + reap** (bash.go lines 95-138) — reuse directly for shell TERM→KILL drain:
```go
func killGroupOnCtx(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		...
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil // group already gone — not a failure
		}
		...
	}
}
func reapGroup(pid int) { /* loop kill(-pid) until ESRCH, bounded by reapGroupBudget=250ms / tick=25ms */ }
```
PTY-specific (no analog, RESEARCH Pattern 5): `pty.StartWithAttrs(cmd, &pty.Winsize{Rows: 24, Cols: 80}, &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0})`; sentinel echo `echo __ASS_GUARD_DONE_$?__`; `errors.Is(err, syscall.EIO)` → clean EOF (Pitfall 4); close the master fd in the OnClose chain (Pitfall 5).

---

### `internal/coreexec/ansistrip.go` (utility, transform)

**Analog:** pure-helper convention in `bash.go` — small, fixture-documented, trailing-trim style (lines 140-146, 169-186):
```go
// trimCaptured applies the observed trailing-trim: 0/105 text and 0/2
// error-text captured results end with a newline or any trailing whitespace ...
func trimCaptured(s string) string {
	return strings.TrimRight(s, " \t\r\n")
}
```
Hand-rolled regex (~10 lines, RESEARCH "Deliberately NOT dependencies"); pure + table tests pinning the chosen CSI/SGR scope.

---

### `internal/coreexec/background.go` (MOD — queue-on-cap D-11, escalation ladder PAR-08, notify hook)

**Current cap behavior being REVERSED** (lines 140-149):
```go
// Over-cap starts fail with the structured error naming the cap (T-12-06-01).
func (r *TaskRegistry) Start(workDir, command string) (string, error) {
	r.mu.Lock()
	if len(r.tasks) >= bgConcurrentCap {
		r.mu.Unlock()
		return "", fmt.Errorf("background: concurrent task cap %d reached: %w", bgConcurrentCap, errBgCap)
	}
	...
```
D-11: over-cap → append to a FIFO waiter list, return the queued-note form (prose in the `renderBackgroundStart` style), drain waiters as tasks complete; keep a cap-error path only for a bounded pathological queue. Cap becomes injected (`background.bash`, default 16). Update the doc comment — it currently *promises* the fail semantics.

**Escalation ladder** — extend the Wait goroutine (lines 185-203) and `Stop` (278-299, currently SIGKILL-first):
```go
// Stop today (SIGKILL-first — the gap PAR-08 closes):
_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
reapGroup(cmd.Process.Pid)
// Ladder (RESEARCH Pattern 6): TERM the group, grace window (~5s discretion),
// then the existing SIGKILL group-kill + reapGroup. Pdeathsig is linux-only:
// cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
// (in a //go:build linux file; keep the Start path goroutine pinned — Pitfall 3)
```

**Notification hook:** the Wait goroutine's terminal transition (lines 191-202) is where the kind-tagged completion callback into `internal/tasks` fires (D-02 payload: exit status from `waitErr`, duration from a start timestamp the task must now record, Tail from the bounded buffers via `t.snapshot()`, OutputFile from `taskLogPath`).

---

### `internal/coreexec/bash.go` (MOD — persistent arg D-09, sandbox wrap, flag semantics)

**Branch point** (lines 265-281):
```go
var a bashArgs
// Best-effort parse per the schema's surface: unknown fields are ignored...
_ = json.Unmarshal(args, &a)
timeoutMS := resolveBashTimeout(a.Timeout)
// 12-06 (ACP-06): the background branch — Start instead of Wait...
if a.RunInBackground {
	return bashBackgroundStart(cfg, a.Command)
}
```
Add the `persistent` branch beside it (D-09) and the sandbox wrap before exec. **Schema tension for the planner (RESEARCH Open Question 1):** `coretools.json:195` has `"additionalProperties": false` with no `persistent` property — the captured schema must be extended additively or persistence is unreachable by the model. `dangerouslyDisableSandbox` currently parses as a documented no-op (lines 57-64 comment block) — under SAND-01 it acquires real meaning (honor per-call unconfined + loud note, per RESEARCH OQ2).

**Foreground execution path** the sandbox wrap encloses (lines 278-304): `exec.CommandContext(tctx, "sh", "-c", a.Command)` + `killGroupOnCtx(cmd)` + `cmd.Dir = cfg.WorkDir` + combined buffers + `reapGroup` on timeout. The wrap (re-exec self on linux / `sandbox-exec -p` on darwin) substitutes the argv; group discipline stays.

---

### `internal/session/subagent.go` + `session.go:432-470` (MOD — background dispatch PAR-07)

**Dispatch site** (session.go 430-470) — parse `run_in_background` from `tc.Input` here (the field exists in the captured schema, `coretools.json:29-32`; session package ignores it today):
```go
if isSubagentTool(tc.Name) {
	var agentDef *ecosys.Agent
	if def, ok := s.agentDefFor(tc.Input); ok {
		agentDef = &def
	}
	result, derr := s.DispatchSubagent(ctx, turnID, tc.Name, extractSubagentPrompt(tc.Input), agentDef)
	...
	payload, isErr := subagentResultPayload(result)
	s.appendToolResultLoud(turnID, callID, tc.Name, boundedToolResult(payload), isErr)
```
Background mode: `DispatchSubagent` (subagent.go 43-123) registers with the tracker, returns the `async_launched` form immediately; the goroutine detaches under a serve-lifetime ctx (see `internal/tasks/subagent.go` assignment above). Input parsing follows the best-effort anonymous-struct idiom (`agentDefFor`, subagent.go 299-315; `extractSubagentPrompt`, 317-340). **Phase-20 seam:** model resolution passes through 20-03's resolver via the same dispatch-time `model` input — if 20-03 is unlanded, precondition-mark, never a second resolver (RESEARCH Pattern 7).

---

### `internal/runtime/cron_wiring.go` (MOD — wake-drain D-01/D-03)

**The template to copy is in this file.** `runAutomationTurn` (lines 133-205) is the complete wake-turn recipe:
```go
sess := r.sessionFor(ctx, sessionID)
if sess == nil { return }
mu := r.sessionTurnMu(sessionID)
queued := !mu.TryLock()
if queued { mu.Lock() }
defer mu.Unlock()
...
r.sessMu.Lock()
r.automationProvenance = "automation:" + a.ID     // ← wake provenance mirrors this
r.sessMu.Unlock()
blocks := []session.ContentBlock{{Type: blockText, Text: prompt}}
stop, err := r.runOneTurn(ctx, sess, blocks)
r.sessMu.Lock()
r.automationProvenance = ""
r.sessMu.Unlock()
...
werr := sess.Manager.AppendEngineDecision(
	"automation:"+a.ID, action, "cron:"+a.ID, "", "sched:schedule", reason)
```
The wake drain renders ALL pending notifications (ordered by completion time) as the blocks, uses a wake provenance string, and writes an EngineDecision audit line. If TryLock fails: leave pending (they coalesce), retry on tick/turn-end — the scheduler tick loop (lines 87-102) shows the retry cadence pattern. 17-D-07 decline-on-ask applies to wake turns (no human).

---

### `internal/runtime/runtime.go` (MOD — wire tracker, PTY drain, startup sweep)

**Registration site** (lines 1107-1138, verified via grep):
```go
taskRegistry := coreexec.NewTaskRegistry()
// line 1109: coreexec.Config{WorkDir: dir, Todos: coreexec.NewTodoStore(), Hooks: hookRunner, Tasks: taskRegistry}
// line 1138: {... Mailbox: mailbox, Sessions: sessionReader, Tasks: taskRegistry}
```
The tracker + PTY manager join this construction; deps flow into both coreexec.Config and the runner.

**OnClose chain — where PTY drain joins** (lines 1238-1243 + 1260-1265):
```go
s.OnClose = func() error {
	cancelWriter()
	taskRegistry.ReapAll() // 12-06: no background group outlives the session
	return mcpHost.Close()
}
...
prevOnClose := s.OnClose
s.OnClose = func() error {
	stopForwarder()
	return prevOnClose()
}
```
The PTY drain (TERM→KILL shell group + close master fd, D-08) is a new link in this exact chain; ReapAll also cancels queued-but-unstarted tasks (OQ5). `ReapAll` itself (background.go 303-318) is the sweep-loop style for the startup stale-log sweep.

**Startup sweep ordering (Pitfall 10):** runs in `acp_serve.Run`'s pipeline BEFORE scheduler start — the pipeline order to extend is `Run`'s sequence (acp_serve.go 121-252: bus → profile → factory → warnings → body store → mirror → runner → registry load → engine → surface → server → schedule store → emitter → scheduler → ctx-done reap goroutine).

---

### `internal/acpserve/config_surface.go` (MOD — D-12 caps menu)

**Menu builder** (lines 479-531):
```go
build := func(id, name, desc, category, current string, values []string) acp.ConfigOptionFrame {
	opts := make([]acp.ConfigOptionValue, 0, len(values))
	for _, v := range values {
		opts = append(opts, acp.ConfigOptionValue{Value: v, Name: v})
	}
	return acp.ConfigOptionFrame{
		ID: id, Name: name, Description: desc, Category: category,
		Type: acp.ConfigOptionTypeSelect, CurrentValue: current, Options: opts,
	}
}
return []acp.ConfigOptionFrame{
	build(optModel, "Model", "Model the agent sends requests to", categoryModel, res.model, models),
	...
}
```
`background.subagents` (8) and `background.bash` (16) join as menu entries — numeric-valued options following the Phase-16 "advertise all, apply-as-landed" rule (pending-option machinery at `Set` lines 149-184 handles apply semantics).

---

### `internal/acpserve/acp_serve.go` (MOD — `--sandbox` flag + probe)

The flag lands on `Options` (the composition-root options struct `Run` consumes, line 122) and the startup probe joins the `Run` pipeline with the established degrade idiom — `startAuditMirror`'s doc comment is the exact wording style (lines 28-32): *"Any construction failure degrades loudly (stderr log, mirror disabled) — never a serve refusal."* Default OFF (SAND-01); `--sandbox=off` always escapes. Degrade = one `log.Printf`/`fmt.Fprintf(stderr, ...)` note + counter + per-run unconfined note.

---

### Tests

**Analog batteries:** `background_test.go` (six `TestBackground_*` batteries: round-trip, non-blocking/unknown, block-timeout, output-fidelity, fixture-conformance, output-file-persisted) and `cron_wiring_test.go` (fake clock + TryLock queue assertions). All new tests: stdlib `testing` + `stretchr/testify` v1.11.1 (go.mod line 12), `-race` always; quick command `go test -race -count=1 ./internal/coreexec/ ./internal/runtime/ ./internal/session/ ./internal/tasks/ ./internal/sandbox/`; linux-tagged files compile-gated via `GOOS=linux CGO_ENABLED=0 go build ./... && go vet`. Seatbelt live-probe tests run on this darwin host (`//go:build darwin`); landlock runtime tests are linux-host-gated, skipped-by-tag here.

## Shared Patterns

### Transport discipline (stdout reserved for ACP frames)
**Source:** `AGENTS.md` constraint + `acp_serve.go:42` (`log.Printf` → stderr), `cron_wiring.go:202-204` (`fmt.Fprintf(r.stderrOrDefault(), ...)`)
**Apply to:** ptty.go, background.go notify hooks, sandbox backends — ALL PTY/child output and diagnostics go to stderr/counters, never stdout.

### Degrade loudly, never refuse
**Source:** `acp_serve.go:75-88` (seed), `:191-198` (engine setup), `:231-237` (schedule store), `:28-32` (audit mirror)
**Apply to:** sandbox probe (SAND-01), dead-PTY restart note (D-08), queue notes (D-10/D-11), stale sweep. One loud stderr note + counter; the feature stays runnable.

### Process-group lifecycle (Setpgid + negative-pid kill + bounded reap)
**Source:** `bash.go:95-138`
**Apply to:** background.go escalation ladder, ptty.go drain, sandbox-wrapped children. ESRCH-clean; `reapGroup` bounds the fork-vs-kill race.

### Serve-lifetime ctx for work that outlives a turn
**Source:** `runtime.go:1229-1234` (`writerCtx`), `runtime.go:787` (parked chain), `cron_wiring.go:87-102` (scheduler goroutine)
**Apply to:** background subagent loops, output-file writers, notify drain goroutine. `r.serveCtxOrBackground()` for test-runner tolerance; OnClose reaps.

### Per-session registry + OnClose reaping
**Source:** `background.go:96-104,303-318` + `runtime.go:1238-1243`
**Apply to:** tasks tracker and PTY manager — one per session, ids session-local, ReapAll/Drain in the OnClose chain.

### Structured errors + fixture-pinned result forms
**Source:** `bash.go:388-395` (`marshalStructured`), `background.go:320-324` (sentinel errors), `background.go:435-440` (`renderBackgroundStart` prose form)
**Apply to:** all new tool-result surfaces (async_launched form, queued-note form, wake blocks). Render helpers keep forms in one function with a provenance doc comment.

### Turn serialization via sessionTurnMu TryLock
**Source:** `cron_wiring.go:35-39,153-160`
**Apply to:** the wake-turn drain ONLY — the single drain point; never a second mutex.

### Executor wiring: Config struct + RegisterCore closures
**Source:** `register.go:27-67`
**Apply to:** new Config fields (sandbox policy handle, PTY manager, cap config) flow through `coreexec.Config` constructed once at sessionFor; stubs close over it; RegisterCore overrides ONLY `Execute` (captured schema stays byte-identical unless the operator sanctions the `persistent` waiver — OQ1).

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/sandbox/landlock_linux.go` | system service | process-lifecycle | First OS-sandbox code in repo; RESEARCH §Code Examples (probe idiom, v4 ruleset) is the source |
| `internal/sandbox/child_linux.go` | utility | process-lifecycle | Re-exec-sentinel entrypoint has no precedent; RESEARCH Pattern 4 + go doc citations |
| `internal/sandbox/seatbelt_darwin.go` | system service | transform | Embedded `.sb` template + `-p` render is new; copy only the degrade discipline from `seedACPGuard` |
| `internal/sandbox/policy.go` | config | transform | Pure policy struct; only the documented-const/typed-struct convention transfers |

## Metadata

**Analog search scope:** `internal/coreexec/`, `internal/session/`, `internal/runtime/`, `internal/acpserve/`, `internal/toolcat/coretools.json`, `go.mod`
**Files read in full or in targeted sections:** 11 source files + 2 schema sections + 3 grep surveys
**Key verification:** `errBgCap` has zero test references (cap coverage is additive, not a rewrite); `coretools.json:195` Bash schema is closed (`additionalProperties: false`) — the `persistent` arg (D-09) requires an explicit operator-waived schema extension or an alternative surface (RESEARCH OQ1); `dangerouslyDisableSandbox` is a documented no-op today (`bash.go:57-64`) that SAND-01 makes real (OQ2).
**Pattern extraction date:** 2026-08-28
