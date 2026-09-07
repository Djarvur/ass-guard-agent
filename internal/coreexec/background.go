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
	// bgConcurrentCap bounds live tasks per session (a documented corpus-absent
	// default — the schema offers no guidance; the 12-05 fixture flags it) so
	// a model loop cannot fork-bomb the host.
	bgConcurrentCap = 16
	// bgOutputCapBytes bounds each task's ACCUMULATED in-memory copy (the
	// on-disk log is the full record; this is the retrieval buffer).
	bgOutputCapBytes = 1024 * 1024
	// bgTaskOutputDefaultMS is the schema-declared TaskOutput timeout default.
	bgTaskOutputDefaultMS = 30000
)

// bgState is the task state machine.
type bgState string

const (
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
	id     string
	cmd    *exec.Cmd
	mu     sync.Mutex
	out    bytes.Buffer
	errBuf bytes.Buffer
	state  bgState
	exitCh chan struct{}
	start  time.Time
	logPath string
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
	// CompletionHook (22-01, PAR-08) is fired exactly once per task from the
	// Wait goroutine's terminal transition with PRIMITIVE args (task id,
	// kind, exit status, duration, tail, output-file pointer) — no
	// coreexec→tasks import; the runtime adapter builds the kind-tagged
	// Notification. nil (unwired, e.g. bare registry tests) fires nothing.
	// Set BEFORE the first Start (captured per task under the registry lock).
	CompletionHook CompletionHook
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
// Over-cap starts fail with the structured error naming the cap (T-12-06-01).
func (r *TaskRegistry) Start(workDir, command string) (string, error) {
	r.mu.Lock()

	if len(r.tasks) >= bgConcurrentCap {
		r.mu.Unlock()

		return "", fmt.Errorf("background: concurrent task cap %d reached: %w", bgConcurrentCap, errBgCap)
	}

	id := newTaskID()
	task := &bgTask{id: id, state: bgRunning, exitCh: make(chan struct{}), start: time.Now(), hook: r.CompletionHook}
	r.tasks[id] = task

	r.mu.Unlock()

	f, err := openTaskLog(workDir, id)
	if err != nil {
		task.setState(bgFailed)

		return "", err
	}

	task.logPath = f.Name()

	//nolint:noctx // T-8-33: model-authored command is the product; the
	// registry — not a call ctx — owns this lifecycle (Cancel unset; Stop/ReapAll kill)
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workDir
	cmd.Stdout = io.MultiWriter(f, taskWriter{task, false})
	cmd.Stderr = io.MultiWriter(f, taskWriter{task, true})
	// Own process group (no cmd.Cancel — the REGISTRY owns this lifecycle,
	// not a call ctx; Stop/ReapAll do the group killing).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	task.cmd = cmd

	serr := cmd.Start()
	if serr != nil {
		_ = f.Close()

		task.setState(bgFailed)

		return "", fmt.Errorf("background: start: %w", serr)
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
	}()

	return id, nil
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

	return t.snapshot(), state, state == bgRunning, nil
}

// Stop kills the task's WHOLE process group + reaps stragglers (the 08-03
// discipline) and marks it stopped. Unknown ids error (per-session scoping).
func (r *TaskRegistry) Stop(id string) error {
	r.mu.Lock()

	t, ok := r.tasks[id]
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("taskstop: unknown task %q: %w", id, errUnknownTask)
	}

	t.mu.Lock()
	cmd := t.cmd
	t.state = bgStopped
	t.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		reapGroup(cmd.Process.Pid)
	}

	return nil
}

// ReapAll stops every live task (session close — no orphan groups outlive the
// editor-owned process; T-12-06-02).
func (r *TaskRegistry) ReapAll() {
	r.mu.Lock()

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

		if running {
			notReady := "<retrieval_status>not_ready</retrieval_status>\n\n" +
				"<task_id>" + a.TaskID + "</task_id>\n\n" +
				"<task_type>local_bash</task_type>\n\n" +
				"<status>running</status>"

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

// taskLogPath is the progressive log's path (mirrors Start's layout).
func taskLogPath(workDir, id string) string {
	return filepath.Join(workDir, ".ass-guard", "outputs", id+".log")
}
