package coreexec

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/sandbox"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// Background work (12-06, ACP-05 + ACP-06): the per-session TaskRegistry owns
// background process groups — start (immediate return with the CAPTURED
// form), output accumulation (bounded, stdout-before-stderr), bounded blocking
// retrieval, whole-group stop, and reap-all on session close. The
// process-group discipline reuses bash.go's killGroupOnCtx/reapGroup idiom.
//
// Per-session scoping (T-12-06-03): one registry per session; ids are
// session-local handles — another session's registry answers unknown-id.
const (
	// bgConcurrentDefault is the D-11 default cap for live tasks per session
	// (background.bash, operator-tunable via configOptions — the registry's
	// injected Cap field; 0 → this default).
	bgConcurrentDefault = 16
	// bgQueueBound bounds the over-cap FIFO queue's pathological growth
	// (D-10/D-11 bounded-resources intent): past it, starts fail with the
	// structured error naming the bound.
	bgQueueBound = 64
	// bgOutputCapBytes bounds each task's ACCUMULATED in-memory copy (the
	// on-disk log is the full record; this is the retrieval buffer).
	bgOutputCapBytes = 1024 * 1024
	// bgTaskOutputDefaultMS is the schema-declared TaskOutput timeout default.
	bgTaskOutputDefaultMS = 30000
)

// bgState is the task state machine.
type bgState string

const (
	bgQueued  bgState = "queued"
	bgRunning bgState = "running"
	bgDone    bgState = "done"
	bgStopped bgState = "stopped"
	bgFailed  bgState = "failed"
)

// bgTask is one background process group + its accumulated output. stdout and
// stderr accumulate into SEPARATE bounded buffers; retrieval combines
// stdout-before-stderr — the captured combined discipline (the foreground
// form's order, applied to the accumulated read).
type bgTask struct {
	id      string
	cmd     *exec.Cmd
	mu      sync.Mutex
	out     bytes.Buffer
	errBuf  bytes.Buffer
	state   bgState
	exitCh  chan struct{}
	start   time.Time
	logPath string
	// queuedCommand/queuedWorkDir carry a D-11 waiter's launch inputs (set
	// at queue time; consumed by startNextWaiter).
	queuedCommand string
	queuedWorkDir string
	// disableSandbox is the 22-06 OQ2 per-call escape captured at dispatch
	// (both the immediate and the queued launch read it — a queued task that
	// starts later honors the escape exactly as an immediate one).
	disableSandbox bool
	// hook is the completion callback captured at Start (the registry field
	// is wired before any task launches; capturing keeps the read race-free).
	hook CompletionHook
}

// accumulate appends to the stream's bounded buffer.
func (t *bgTask) accumulate(isStderr bool, p []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()

	buf := &t.out
	if isStderr {
		buf = &t.errBuf
	}

	remaining := bgOutputCapBytes - buf.Len()
	if remaining <= 0 {
		return
	}

	if len(p) > remaining {
		p = p[:remaining]
	}

	buf.Write(p)
}

func (t *bgTask) snapshot() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	return trimCaptured(t.out.String() + t.errBuf.String())
}

// TaskRegistry is the per-session owner of background tasks.
type TaskRegistry struct {
	mu    sync.Mutex
	tasks map[string]*bgTask
	// Cap bounds live (running) tasks per session — the D-11 injected
	// background.bash value (0 → the bgConcurrentDefault 16). Over-cap
	// starts QUEUE FIFO (D-11) up to bgQueueBound.
	Cap int
	// termGrace is the PAR-08 escalation ladder's TERM→KILL grace window
	// (test-injectable; the production default is bgTermGrace = 5s).
	termGrace time.Duration
	// waiters is the FIFO of queued-but-unstarted tasks (D-11) — submission
	// order; drained head-first as slots free.
	waiters []*bgTask
	// CompletionHook (22-01, PAR-08) is fired exactly once per task from the
	// Wait goroutine's terminal transition with PRIMITIVE args (task id,
	// kind, exit status, duration, tail, output-file pointer) — no
	// coreexec→tasks import; the runtime adapter builds the kind-tagged
	// Notification. nil (unwired, e.g. bare registry tests) fires nothing.
	// Set BEFORE the first Start (captured per task under the registry lock).
	CompletionHook CompletionHook
	// Sandbox (22-06, SAND-01): the resolved Handle; nil / Mode "off" / the
	// zero Mode (the default) leave every launch untouched — Start's exec
	// construction byte-identical. Mode "on" + Available wraps EVERY launch
	// through the SAME one WrapCmd entry as the foreground site (this is the
	// second of the three Bash-class exec sites); enabled-but-unavailable and
	// the per-call dangerouslyDisableSandbox escape run UNCONFINED with the
	// loud per-run note + counter (OQ2 parity).
	Sandbox *sandbox.Handle
	// SandboxNote is the unconfined-run note sink (22-06); nil falls back to
	// os.Stderr — a green background result must never imply confinement
	// that did not happen.
	SandboxNote func(format string, args ...any)
}

// StartOpts carries the per-call sandbox escape to BOTH start paths (22-06,
// OQ2): the model's dangerouslyDisableSandbox rides the bgTask so an
// immediate start AND a FIFO-queued start (which launches later, through the
// same launch funnel) honor it identically.
type StartOpts struct {
	DisableSandbox bool
}

// CompletionHook is the terminal-transition callback (22-01). kind is the
// producer discriminator ("bash" for this registry); exitStatus is "0", a
// nonzero decimal, "killed" (Stop), or "error" (a non-exit Wait failure).
type CompletionHook func(taskID, kind, exitStatus string, duration time.Duration, tail, outputFile string)

// bgKindBash is the registry's producer kind (tasks.KindBash's value — the
// adapter maps it; coreexec owns no tasks import by design).
const bgKindBash = "bash"

// NewTaskRegistry returns an empty per-session registry.
func NewTaskRegistry() *TaskRegistry {
	return &TaskRegistry{tasks: map[string]*bgTask{}}
}

// newTaskID mints the captured exec_<uuid> id shape.
func newTaskID() string {
	var b [16]byte

	_, rerr := rand.Read(b[:])
	if rerr != nil {
		return fmt.Sprintf("exec_%d", time.Now().UnixNano()) // unreachable fallback
	}

	return "exec_" + hex.EncodeToString(b[:])
}

// openTaskLog creates the task's progressive log file under the session
// artifact family (parent dirs created).
func openTaskLog(workDir, id string) (*os.File, error) {
	dir := filepath.Join(workDir, ".ass-guard", "outputs")

	merr := os.MkdirAll(dir, dirPermWrite)
	if merr != nil {
		return nil, fmt.Errorf("background: output dir: %w", merr)
	}

	logPath := filepath.Join(dir, id+".log")

	f, err := os.Create(logPath) // the id is hex-minted, not model input
	if err != nil {
		return nil, fmt.Errorf("background: output log: %w", err)
	}

	return f, nil
}

// Start launches command via `sh -c` in its OWN process group under workDir,
// teeing combined output into <workDir>/.ass-guard/outputs/<id>.log (the
// progressive log the start form names), and returns the task id immediately.
//
// D-11 queue contract (22-02): an over-cap start does NOT fail — the task
// (id minted at dispatch, so Stop/Output work immediately) joins the FIFO
// waiters and queued=true returns; it launches when a slot frees, in
// submission order. Past the bounded queue (bgQueueBound) the structured
// errBgCap error survives — the queue can grow only bounded.
func (r *TaskRegistry) Start(workDir, command string) (string, bool, error) {
	return r.StartWithOpts(workDir, command, StartOpts{})
}

// StartWithOpts is Start carrying the 22-06 per-call sandbox escape: the
// opts ride the task so BOTH launch paths (immediate and FIFO-queued) honor
// the escape and the wrap identically through the one launch funnel.
func (r *TaskRegistry) StartWithOpts(workDir, command string, opts StartOpts) (string, bool, error) {
	r.mu.Lock()

	cap := r.Cap
	if cap <= 0 {
		cap = bgConcurrentDefault
	}

	live := 0

	for _, t := range r.tasks {
		t.mu.Lock()
		if t.state == bgRunning {
			live++
		}
		t.mu.Unlock()
	}

	if live >= cap {
		if len(r.waiters) >= bgQueueBound {
			r.mu.Unlock()

			return "", true, fmt.Errorf(
				"background: queue bound %d reached (%d running): %w", bgQueueBound, live, errBgCap)
		}

		id := newTaskID()
		task := &bgTask{id: id, state: bgQueued, exitCh: make(chan struct{}), hook: r.CompletionHook,
			queuedCommand: command, queuedWorkDir: workDir, disableSandbox: opts.DisableSandbox}
		r.tasks[id] = task
		r.waiters = append(r.waiters, task)

		r.mu.Unlock()

		return id, true, nil
	}

	id := newTaskID()
	task := &bgTask{id: id, state: bgRunning, exitCh: make(chan struct{}), start: time.Now(), hook: r.CompletionHook,
		queuedCommand: command, queuedWorkDir: workDir, disableSandbox: opts.DisableSandbox}
	r.tasks[id] = task

	r.mu.Unlock()

	if lerr := r.launch(task, workDir, command); lerr != nil {
		return "", false, lerr
	}

	return id, false, nil
}

// launch runs one task's process (the Start body and the waiter-drain path
// share it): log open, `sh -c` child in its own group, Pdeathsig (linux),
// the Wait goroutine with the terminal transition + completion hook, then
// the FIFO drain of the next waiter.
func (r *TaskRegistry) launch(task *bgTask, workDir, command string) error {
	f, err := openTaskLog(workDir, task.id)
	if err != nil {
		task.setState(bgFailed)

		return err
	}

	task.logPath = f.Name()

	//nolint:noctx // T-8-33: model-authored command is the product; the
	// registry — not a call ctx — owns this lifecycle (Cancel unset; Stop/ReapAll kill)
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workDir
	cmd.Stdout = io.MultiWriter(f, taskWriter{task, false})
	cmd.Stderr = io.MultiWriter(f, taskWriter{task, true})
	// Own process group (no cmd.Cancel — the REGISTRY owns this lifecycle,
	// not a call ctx; Stop/ReapAll do the group killing via terminateGroup).
	// Linux: Pdeathsig SIGKILL so a dead ass-guard cannot orphan the child.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	setPdeathsig(cmd.SysProcAttr)

	// 22-06 (SAND-01): the BACKGROUND sandbox site — the SECOND of the three
	// Bash-class exec sites through sandbox's ONE WrapCmd entry (with the
	// foreground site in bash.go and the PTY shell in ptty.go). BOTH launch
	// routes — the immediate Start and the FIFO queued-start pop — funnel
	// through HERE, so a background task is confined no matter which path
	// launched it. The wrap substitutes argv AFTER the group setup and
	// BEFORE Start: terminateGroup/Stop own the WRAPPED child's group
	// exactly as the bare sh's (D-05 — signals are neither FS nor network
	// operations; the Pdeathsig survives exec).
	if cerr := r.confineLaunch(cmd, task.disableSandbox); cerr != nil {
		_ = f.Close()
		task.setState(bgFailed)

		return fmt.Errorf("background: sandbox wrap: %w", cerr)
	}

	task.cmd = cmd

	serr := cmd.Start()
	if serr != nil {
		_ = f.Close()

		task.setState(bgFailed)

		return fmt.Errorf("background: start: %w", serr)
	}

	go func() {
		defer func() { _ = f.Close() }()
		defer close(task.exitCh)

		waitErr := cmd.Wait()

		task.mu.Lock()

		switch {
		case waitErr == nil:
			task.state = bgDone
		case task.state == bgStopped:
			// already recorded by Stop
		default:
			task.state = bgFailed
		}

		task.mu.Unlock()

		// 22-01 (D-02/PAR-08): the terminal transition fires the completion
		// hook exactly once — exit status from waitErr, duration from the
		// start timestamp, tail from the bounded snapshot, pointer from the
		// task's own log path. The callback runs OUTSIDE task.mu (it may
		// schedule a wake drain; never block the Wait goroutine on it).
		if task.hook != nil {
			task.mu.Lock()
			state := task.state
			task.mu.Unlock()

			task.hook(
				task.id, bgKindBash, exitStatusFor(waitErr, state),
				time.Since(task.start), task.snapshot(), task.logPath,
			)
		}

		// D-11: the freed slot starts exactly ONE queued task (FIFO head).
		r.startNextWaiter()
	}()

	return nil
}

// confineLaunch applies the 22-06 background-site sandbox policy to the
// launch's cmd: enabled+available+not-disabled → the SAME one WrapCmd entry
// as the foreground site (OQ2 parity — identical arms); the disable escape
// and the unavailable degrade run UNCONFINED with the loud per-run note +
// counter (a green background result must never imply confinement that did
// not happen). Default OFF: untouched.
func (r *TaskRegistry) confineLaunch(cmd *exec.Cmd, disable bool) error {
	if !sandboxHandleEnabled(r.Sandbox) {
		return nil
	}

	switch {
	case disable:
		noteUnconfinedRun(r.SandboxNote, "background task",
			"dangerouslyDisableSandbox requested by the model")
	case !r.Sandbox.Availability.Available:
		noteUnconfinedRun(r.SandboxNote, "background task",
			"sandbox unavailable: "+r.Sandbox.Availability.Reason)
	default:
		return wrapSandboxCmd(cmd, r.Sandbox.Policy)
	}

	return nil
}

// startNextWaiter launches the FIFO head waiter when a slot is free and it
// is still queued (a Stop'd waiter is skipped — never started).
func (r *TaskRegistry) startNextWaiter() {	for {
		r.mu.Lock()

		cap := r.Cap
		if cap <= 0 {
			cap = bgConcurrentDefault
		}

		live := 0

		for _, t := range r.tasks {
			t.mu.Lock()
			if t.state == bgRunning {
				live++
			}
			t.mu.Unlock()
		}

		if live >= cap || len(r.waiters) == 0 {
			r.mu.Unlock()

			return
		}

		next := r.waiters[0]
		r.waiters = r.waiters[1:]

		next.mu.Lock()
		stillQueued := next.state == bgQueued
		next.state = bgRunning
		next.start = time.Now()
		next.mu.Unlock()

		r.mu.Unlock()

		if !stillQueued {
			continue // stopped while queued — never start, try the next head
		}

		if lerr := r.launch(next, next.queuedWorkDir, next.queuedCommand); lerr != nil {
			continue // launch failure freed the slot — drain the next waiter
		}

		return
	}
}

// QueuedPosition reports a queued task's 1-based FIFO position (0 when not
// queued) — the queued-note form's visibility input.
func (r *TaskRegistry) QueuedPosition(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, w := range r.waiters {
		if w.id == id {
			return i + 1
		}
	}

	return 0
}

// CapValue resolves the effective cap (injected value or the 16 default).
func (r *TaskRegistry) CapValue() int {
	if r.Cap > 0 {
		return r.Cap
	}

	return bgConcurrentDefault
}

// taskWriter adapts accumulate to io.Writer (stream-tagged).
type taskWriter struct {
	t        *bgTask
	isStderr bool
}

func (w taskWriter) Write(p []byte) (int, error) {
	w.t.accumulate(w.isStderr, p)

	return len(p), nil
}

func (t *bgTask) setState(s bgState) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state = s
}

// Lookup reports the task's existence + current state.
func (r *TaskRegistry) Lookup(id string) (bgState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tasks[id]
	if !ok {
		return "", false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	return t.state, true
}

// Output retrieves accumulated output: block=true waits bounded by timeoutMS
// (a still-running task after the bound returns with running=true); block=false
// snapshots immediately.
//
//nolint:gocritic // the positional results read clearly at both call sites
func (r *TaskRegistry) Output(id string, block bool, timeoutMS int) (string, bgState, bool, error) {
	r.mu.Lock()

	t, ok := r.tasks[id]
	r.mu.Unlock()

	if !ok {
		return "", "", false, fmt.Errorf("taskoutput: unknown task %q: %w", id, errUnknownTask)
	}

	if block && timeoutMS <= 0 {
		timeoutMS = bgTaskOutputDefaultMS
	}

	if block {
		select {
		case <-t.exitCh:
		case <-time.After(time.Duration(timeoutMS) * time.Millisecond):
		}
	}

	t.mu.Lock()
	state := t.state
	t.mu.Unlock()

	// A QUEUED task has no process — "running" only in the launched sense.
	running := state == bgRunning

	return t.snapshot(), state, running, nil
}

// bgTermGrace is the PAR-08 escalation ladder's grace window: the group
// gets SIGTERM, this long to die cleanly, then SIGKILL + reap (wrapper
// shells' grandchildren get the chance to clean up — CONTEXT discretion 5s).
const bgTermGrace = 5 * time.Second

// terminateGroup is the ONE PAR-08 termination funnel — every kill path
// (Stop, ReapAll, any future cancel) escalates through here so the ladder
// cannot drift between call sites: SIGTERM the whole group, wait the grace
// window on done (the task's exit signal), then SIGKILL the group + the
// bounded reap. ESRCH at any step is success (the group is already gone —
// the bash.go killGroupOnCtx idiom). A bounded reap runs on BOTH exits —
// a clean TERM death can still leave fork-race stragglers holding the pgid.
func terminateGroup(pid int, done <-chan struct{}, grace time.Duration) {
	if grace <= 0 {
		grace = bgTermGrace
	}

	err := syscall.Kill(-pid, syscall.SIGTERM)
	if errors.Is(err, syscall.ESRCH) {
		return // group already gone — not a failure
	}

	timer := time.NewTimer(grace)
	defer timer.Stop()

	select {
	case <-done:
		reapGroup(pid) // sweep TERM-surviving stragglers in the group
	case <-timer.C:
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		reapGroup(pid)
	}
}

// Stop escalates the task's WHOLE process group through the PAR-08 ladder
// (TERM → grace → KILL + bounded reap — terminateGroup) and marks it
// stopped. A QUEUED task (D-11) simply leaves the FIFO — its state marks
// stopped and startNextWaiter skips it (never started, never killed).
// Unknown ids error (per-session scoping).
func (r *TaskRegistry) Stop(id string) error {
	r.mu.Lock()

	t, ok := r.tasks[id]
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("taskstop: unknown task %q: %w", id, errUnknownTask)
	}

	t.mu.Lock()
	cmd := t.cmd
	prev := t.state
	t.state = bgStopped
	t.mu.Unlock()

	if prev == bgQueued || cmd == nil || cmd.Process == nil {
		return nil // nothing was ever launched — the waiter skip covers it
	}

	terminateGroup(cmd.Process.Pid, t.exitCh, r.termGrace)

	return nil
}

// ReapAll stops every live task through the terminateGroup ladder (session
// close — no orphan groups outlive the editor-owned process; T-12-06-02,
// PAR-08's no-orphans letter via Stop → terminateGroup) and DROPS every
// queued-but-unstarted waiter (OQ5: nothing started, nothing to kill —
// startNextWaiter skips stopped heads). Returns the dropped-waiter count.
func (r *TaskRegistry) ReapAll() int {
	r.mu.Lock()

	dropped := len(r.waiters)

	// Mark every waiter stopped IN PLACE — concurrent drain attempts skip
	// them (the stillQueued guard in startNextWaiter), then clear the FIFO.
	for _, w := range r.waiters {
		w.setState(bgStopped)
	}

	r.waiters = nil

	ids := make([]string, 0, len(r.tasks))
	for id := range r.tasks {
		ids = append(ids, id)
	}

	r.mu.Unlock()

	for _, id := range ids {
		if state, ok := r.Lookup(id); ok && state == bgRunning {
			_ = r.Stop(id)
		}
	}

	return dropped
}

var (
	errBgCap       = errors.New("coreexec: background cap")
	errUnknownTask = errors.New("coreexec: unknown task")
	errNoRegistry  = errors.New("coreexec: background: no task registry configured")
)

// exitStatusFor derives the notification's exit-status string from the Wait
// error and the terminal state: "killed" for registry-stopped tasks (the
// D-02 vocabulary), "0" for clean exits, the decimal code (or 128+signal in
// the shell convention for signal deaths) otherwise, "error" for non-exit
// Wait failures.
func exitStatusFor(waitErr error, state bgState) string {
	if state == bgStopped {
		// A TERM-trapped child CHOSE its exit code at the ladder's first
		// rung (e.g. `trap "exit 7" TERM` → 7); only an unchosen death
		// (signaled/nil) reports "killed".
		var ee *exec.ExitError

		if errors.As(waitErr, &ee) {
			if status, ok := ee.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return "killed"
			}

			if code := ee.ExitCode(); code >= 0 {
				return strconv.Itoa(code)
			}
		}

		return "killed"
	}

	if waitErr == nil {
		return "0"
	}

	var ee *exec.ExitError

	if errors.As(waitErr, &ee) {
		if status, ok := ee.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return strconv.Itoa(128 + int(status.Signal()))
		}

		return strconv.Itoa(ee.ExitCode())
	}

	return "error"
}

// --- executors -----------------------------------------------------------------

// bgArgs is the Bash background branch's input (parsed in bashArgs already;
// the branch lives in bash.go — this file carries the registry + executors
// for the retrieval/stop pair).

// taskOutputArgs is the captured TaskOutput input shape (all three required
// by the schema; timeout 0 → the 30000 default).
type taskOutputArgs struct {
	TaskID  string  `json:"task_id"`
	Block   bool    `json:"block"`
	Timeout float64 `json:"timeout"`
}

// TaskOutputExecute returns the TaskOutput catalog Stub: block/timeout per
// the schema; running tasks render the CAPTURED not_ready XML form; finished
// tasks render the ready form (documented corpus-absent default) with the
// accumulated output; unknown ids return the structured error.
func TaskOutputExecute(r *TaskRegistry) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if r == nil {
			return marshalStructured("taskoutput: no task registry configured", errNoRegistry)
		}

		var a taskOutputArgs

		perr := json.Unmarshal(args, &a)
		if perr != nil || a.TaskID == "" {
			return marshalStructured("taskoutput: invalid input (task_id, block, timeout required)", errBadTaskInput)
		}

		output, state, running, oerr := r.Output(a.TaskID, a.Block, int(a.Timeout))
		if oerr != nil {
			return marshalStructured(oerr.Error(), oerr)
		}

		if running || state == bgQueued {
			// D-11: a queued task reports the QUEUED state in the captured
			// not_ready envelope (not running — nothing launched yet).
			status := "running"
			if state == bgQueued {
				status = "queued"
			}

			notReady := "<retrieval_status>not_ready</retrieval_status>\n\n" +
				"<task_id>" + a.TaskID + "</task_id>\n\n" +
				"<task_type>local_bash</task_type>\n\n" +
				"<status>" + status + "</status>"

			return json.Marshal(notReady)
		}

		ready := "<retrieval_status>" + readyStatusFor(state) + "</retrieval_status>\n\n" +
			"<task_id>" + a.TaskID + "</task_id>\n\n" +
			"<task_type>local_bash</task_type>\n\n" +
			"<status>" + string(state) + "</status>\n\n" +
			"<output>\n" + output + "\n</output>"

		return json.Marshal(ready)
	}
}

// readyStatusFor maps the terminal state onto the retrieval-status vocabulary
// (the corpus_absent ready form's documented default).
func readyStatusFor(s bgState) string {
	if s == bgFailed {
		return "error"
	}

	return "ready"
}

// taskStopArgs accepts task_id with the deprecated shell_id fallback
// (task_id preferred — the schema's own deprecation note).
type taskStopArgs struct {
	TaskID  string `json:"task_id"`
	ShellID string `json:"shell_id"`
}

// TaskStopExecute returns the TaskStop catalog Stub: whole-group kill + reap;
// the ack names the task (corpus_absent documented default — the 12-05 hunt);
// unknown ids (including the deprecated alias) error structurally.
func TaskStopExecute(r *TaskRegistry) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if r == nil {
			return marshalStructured("taskstop: no task registry configured", errNoRegistry)
		}

		var a taskStopArgs

		_ = json.Unmarshal(args, &a)

		id := a.TaskID
		if id == "" {
			id = a.ShellID // deprecated alias
		}

		if id == "" {
			return marshalStructured("taskstop: task_id required", errBadTaskInput)
		}

		serr := r.Stop(id)
		if serr != nil {
			return marshalStructured(serr.Error(), serr)
		}

		ack := "Task " + id + " stopped."

		return json.Marshal(ack)
	}
}

var errBadTaskInput = errors.New("coreexec: task input invalid")

// renderBackgroundStart renders the CAPTURED immediate-return form with the
// real task id + progressive log path (bash.go's run_in_background branch).
func renderBackgroundStart(id, logPath string) string {
	return "Command running in background with ID: " + id +
		". Output is being written to: " + logPath +
		". You will be notified when it completes. " +
		"To check interim output, use Read on that file path."
}

// renderBackgroundQueued renders the D-11 queued form (the visible note —
// the renderBackgroundStart prose family): the id exists, the position and
// cap are named, and the output pointer is real (the log exists from queue
// time only after launch; interim Read resolves once it starts).
func renderBackgroundQueued(id, logPath string, position, cap int) string {
	return "Command queued with ID: " + id +
		" (position " + strconv.Itoa(position) + " behind " + strconv.Itoa(cap) +
		" concurrent background commands; it starts when a slot frees)." +
		" Output will be written to: " + logPath +
		". You will be notified when it completes."
}

// taskLogPath is the progressive log's path (mirrors Start's layout).
func taskLogPath(workDir, id string) string {
	return filepath.Join(workDir, ".ass-guard", "outputs", id+".log")
}
