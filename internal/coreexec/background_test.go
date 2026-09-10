package coreexec //nolint:testpackage // internal package test (fixture helpers shared)

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/sandbox"
)

// The 12-06 battery: background Bash + TaskOutput + TaskStop over the
// per-session TaskRegistry. Forms pin from the 12-05 re-record fixture
// (background_start 32 obs; TaskOutput not_ready 30 obs — both CAPTURED;
// the ready + stop-ack forms are the documented corpus-absent defaults).

// TestBackground_StartRetrieveRoundTrip (T1 Test 1): run_in_background returns
// IMMEDIATELY (well under the command's runtime) with the CAPTURED start form
// carrying a task id; a blocking TaskOutput then returns the accumulated
// output including the command's final marker.
func TestBackground_StartRetrieveRoundTrip(t *testing.T) { //nolint:funlen // flat battery
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	bashExec := BashExecute(Config{WorkDir: dir, Tasks: reg})

	start := time.Now()

	input := `{"command":"echo bg-started; sleep 1; echo bg-done","run_in_background":true}`

	out, err := bashExec(context.Background(), json.RawMessage(input))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("err = %v; want nil (a background start is a success)", err)
	}

	if elapsed > 500*time.Millisecond {
		t.Errorf("start took %v; want immediate (<500ms — the call must not wait)", elapsed)
	}

	var text string

	uerr := json.Unmarshal(out, &text)
	if uerr != nil {
		t.Fatalf("Output not a JSON string: %v (%s)", uerr, out)
	}

	// The CAPTURED start form: exec_<uuid> id + a real output path.
	if !strings.HasPrefix(text, "Command running in background with ID: exec_") {
		t.Errorf("start form = %q; want the captured prefix", text[:40])
	}

	if !strings.Contains(text, "Output is being written to: ") ||
		!strings.Contains(text, "To check interim output, use Read on that file path.") {
		t.Errorf("start form = %q; want the captured path + guidance lines", text)
	}

	// The task id resolves; the output file exists from the start.
	taskID := text[len("Command running in background with ID: "):]
	if i := strings.IndexAny(taskID, " ."); i >= 0 {
		taskID = taskID[:i]
	}

	if !strings.HasPrefix(taskID, "exec_") {
		t.Fatalf("task id = %q; want the exec_ prefix", taskID)
	}

	pathStart := strings.Index(text, "written to: ") + len("written to: ")
	pathEnd := strings.Index(text[pathStart:], ".")
	saved := text[pathStart : pathStart+pathEnd]

	_, serr := os.Stat(saved)
	if serr != nil {
		t.Errorf("output file %q not live from the start: %v", saved, serr)
	}

	// Blocking TaskOutput returns the accumulated output incl. bg-done.
	to := TaskOutputExecute(reg, nil)

	out2, err2 := to(context.Background(), json.RawMessage(
		`{"task_id":"`+taskID+`","block":true,"timeout":10000}`))
	if err2 != nil {
		t.Fatalf("TaskOutput err = %v; want nil", err2)
	}

	var retrieved string

	_ = json.Unmarshal(out2, &retrieved)

	if !strings.Contains(retrieved, "bg-done") {
		t.Errorf("retrieved = %q; want the accumulated output incl. bg-done", retrieved)
	}
}

// TestBackground_NonBlockingAndUnknown (T1 Test 2): block=false returns
// immediately with whatever exists (possibly the CAPTURED not_ready form);
// unknown ids return the structured error form.
func TestBackground_NonBlockingAndUnknown(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()
	to := TaskOutputExecute(reg, nil)

	// Unknown id: structured error.
	out, err := to(context.Background(), json.RawMessage(`{"task_id":"exec_nope","block":false,"timeout":1000}`))
	if err == nil {
		t.Fatal("err = nil; want the unknown-task error")
	}

	var structured struct {
		Error string `json:"error"`
	}

	_ = json.Unmarshal(out, &structured)

	if !strings.Contains(structured.Error, "exec_nope") {
		t.Errorf("Output = %s; want the unknown-task error echoing the id", out)
	}

	// Non-blocking on a live task: immediate return with the not_ready family.
	id, _, serr := reg.Start(t.TempDir(), "sleep 2")
	if serr != nil {
		t.Fatalf("Start: %v", serr)
	}

	start := time.Now()

	out2, err2 := to(context.Background(), json.RawMessage(`{"task_id":"`+id+`","block":false,"timeout":1000}`))
	if err2 != nil {
		t.Fatalf("err = %v; want nil (not_ready is not an error)", err2)
	}

	if time.Since(start) > 300*time.Millisecond {
		t.Error("block=false waited; want an immediate snapshot")
	}

	var text string

	_ = json.Unmarshal(out2, &text)

	if !strings.HasPrefix(text, "<retrieval_status>not_ready</retrieval_status>") {
		t.Errorf("not_ready form = %q; want the captured XML head", text[:40])
	}
}

// TestBackground_BlockTimeout (T1 Test 3): block=true with a short timeout
// against a still-running task returns the not_ready form — never a hang.
func TestBackground_BlockTimeout(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()
	to := TaskOutputExecute(reg, nil)

	id, _, serr := reg.Start(t.TempDir(), "sleep 2")
	if serr != nil {
		t.Fatalf("Start: %v", serr)
	}

	start := time.Now()

	out, err := to(context.Background(), json.RawMessage(`{"task_id":"`+id+`","block":true,"timeout":200}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (a bounded wait is not an error)", err)
	}

	if elapsed := time.Since(start); elapsed < 150*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("elapsed = %v; want the bounded ~200ms wait", elapsed)
	}

	var text string

	_ = json.Unmarshal(out, &text)

	if !strings.Contains(text, "<status>running</status>") {
		t.Errorf("bounded-wait result = %q; want the running-status form", text)
	}
}

// TestBackground_OutputFidelity (T1 Test 4): accumulated output preserves
// stdout-before-stderr order and the trailing-trim convention.
func TestBackground_OutputFidelity(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()
	to := TaskOutputExecute(reg, nil)

	id, _, serr := reg.Start(t.TempDir(), "echo out-line; echo err-line 1>&2")
	if serr != nil {
		t.Fatalf("Start: %v", serr)
	}

	deadline := time.Now().Add(5 * time.Second)

	var text string

	for time.Now().Before(deadline) {
		out, err := to(context.Background(), json.RawMessage(`{"task_id":"`+id+`","block":true,"timeout":5000}`))
		if err != nil {
			t.Fatalf("TaskOutput: %v", err)
		}

		_ = json.Unmarshal(out, &text)

		if strings.Contains(text, "err-line") {
			break
		}
	}

	if !strings.Contains(text, "err-line") {
		t.Fatalf("accumulated output = %q; want both lines", text)
	}

	outIdx := strings.Index(text, "out-line")
	errIdx := strings.Index(text, "err-line")

	if outIdx > errIdx {
		t.Errorf("output order reversed: %q", text)
	}
}

// TestBackground_FixtureConformance (T1 Test 5): the shipped forms
// string-compare against the 12-05 fixture families.
func TestBackground_FixtureConformance(t *testing.T) {
	t.Parallel()

	start := recapturedFamily(t, "Bash", "background_start")
	if !strings.Contains(start.Template, "Command running in background with ID: exec_<uuid>") {
		t.Errorf("fixture background_start = %q; want the captured id form", start.Template)
	}

	notReady := recapturedFamily(t, "TaskOutput", "not_ready")
	if !strings.Contains(notReady.Template, "<retrieval_status>not_ready</retrieval_status>") ||
		!strings.Contains(notReady.Template, "<task_type>local_bash</task_type>") {
		t.Errorf("fixture not_ready = %q; want the captured XML form", notReady.Template)
	}
}

// TestBackground_OutputFilePersisted: the start form's path is a REAL
// progressive log (the file carries the command's output after completion).
func TestBackground_OutputFilePersisted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	bashExec := BashExecute(Config{WorkDir: dir, Tasks: reg})

	out, err := bashExec(context.Background(), json.RawMessage(
		`{"command":"echo persisted-line","run_in_background":true}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	var text string

	_ = json.Unmarshal(out, &text)

	_, rest, ok := strings.Cut(text, "written to: ")
	if !ok {
		t.Fatalf("start form = %q; cannot extract the log path", text)
	}

	saved, _, ok := strings.Cut(rest, ". You will be notified")
	if !ok || saved == "" {
		t.Fatalf("start form = %q; cannot extract the log path", text)
	}

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		b, rerr := os.ReadFile(saved)
		if rerr == nil && strings.Contains(string(b), "persisted-line") {
			return // pass
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("output file %q never carried the command output", saved)
}

// TestBackground_CompletionHook (22-01 Task 1): the CompletionHook fires
// EXACTLY ONCE per task from the Wait goroutine's terminal transition, with
// the exit status derived from waitErr ("0" for clean exits, a nonzero
// decimal for failures, "killed" for Stop), the duration from the start
// timestamp, the tail from the bounded snapshot, and the pointer from the
// task's own log path — and ZERO times when the hook is nil.
func TestBackground_CompletionHook(t *testing.T) { //nolint:funlen // flat hook battery
	t.Parallel()

	type hookCall struct {
		id, kind, status, tail, logPath string
		duration                        time.Duration
	}

	var (
		mu    sync.Mutex
		calls []hookCall
	)

	dir := t.TempDir()
	reg := NewTaskRegistry()
	reg.CompletionHook = func(taskID, kind, exitStatus string, duration time.Duration, tail, outputFile string) {
		mu.Lock()
		defer mu.Unlock()

		calls = append(calls, hookCall{taskID, kind, exitStatus, tail, outputFile, duration})
	}

	// Exit 0 leg.
	id0, _, err := reg.Start(dir, "echo hook-zero-marker")
	if err != nil {
		t.Fatalf("Start (exit 0): %v", err)
	}

	// Nonzero exit leg.
	id1, _, err := reg.Start(dir, "echo hook-fail-marker; exit 3")
	if err != nil {
		t.Fatalf("Start (exit 3): %v", err)
	}

	// Killed leg (Stop marks the state before the group kill).
	id2, _, err := reg.Start(dir, "echo hook-kill-marker; sleep 30")
	if err != nil {
		t.Fatalf("Start (killed): %v", err)
	}

	if serr := reg.Stop(id2); serr != nil {
		t.Fatalf("Stop: %v", serr)
	}

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(calls)
		mu.Unlock()

		if n >= 3 {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 3 {
		t.Fatalf("hook calls = %d; want exactly 3 (one per task)", len(calls))
	}

	byID := map[string]hookCall{}
	for _, c := range calls {
		byID[c.id] = c
	}

	c0 := byID[id0]
	if c0.status != "0" {
		t.Errorf("exit-0 task status = %q; want \"0\"", c0.status)
	}

	if !strings.Contains(c0.tail, "hook-zero-marker") {
		t.Errorf("exit-0 tail = %q; want the snapshot marker", c0.tail)
	}

	if c0.kind != "bash" {
		t.Errorf("kind = %q; want bash (the registry's producer kind)", c0.kind)
	}

	if c0.duration <= 0 {
		t.Errorf("duration = %v; want > 0 (start → terminal)", c0.duration)
	}

	if !strings.HasSuffix(c0.logPath, id0+".log") || !strings.Contains(c0.logPath, ".ass-guard/outputs/") {
		t.Errorf("logPath = %q; want the task's outputs log path", c0.logPath)
	}

	if got := byID[id1].status; got != "3" {
		t.Errorf("exit-3 task status = %q; want \"3\"", got)
	}

	if got := byID[id2].status; got != "killed" {
		t.Errorf("stopped task status = %q; want \"killed\"", got)
	}
}

// TestBackground_CompletionHookNilNoop: a nil CompletionHook (bare registry,
// e.g. the pre-existing batteries) fires nothing and breaks nothing.
func TestBackground_CompletionHookNilNoop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry() // no hook wired

	id, _, err := reg.Start(dir, "echo no-hook")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if state, ok := reg.Lookup(id); ok && state != bgRunning {
			return // terminal transition completed with zero hook panics
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("task never reached a terminal state")
}

// The PAR-08 escalation battery (22-02 Task 1): TERM-before-KILL on the
// process group from every termination path — a TERM-immune child survives
// the TERM (observing it) and dies only at the post-grace SIGKILL; a
// TERM-respecting child dies BY the TERM with its trapped status; ESRCH
// (already-gone groups) is success, never an error.
const (
	escalationGraceTest = 300 * time.Millisecond
	escTermMarker       = "term-delivered-marker"
)

// TestEscalation_TermImmuneChildKilledAfterGrace (PAR-08): the group
// receives SIGTERM first (a TERM-sensitive observer in the group records
// it), the immune child survives past the shortened grace, and the
// post-grace SIGKILL + reap makes the task terminal.
func TestEscalation_TermImmuneChildKilledAfterGrace(t *testing.T) { //nolint:funlen // flat ladder battery
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	reg.termGrace = escalationGraceTest

	marker := filepath.Join(dir, escTermMarker)
	ready := filepath.Join(dir, "immune-ready")
	// Start wraps in `sh -c`, so THIS script is the group leader: it signals
	// ready, traps TERM (records delivery), ignores the TERM death, and
	// loops until the post-grace SIGKILL. The ready-gate matters: a Stop
	// racing the exec leaves the TRAP uninstalled and the shell dies at the
	// TERM's default action — no observation possible.
	command := `echo up > ` + ready + `; trap "echo seen > ` + marker + `" TERM; while true; do sleep 0.05; done`

	id, _, err := reg.Start(dir, command)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !waitForFile(t, ready, 5*time.Second) {
		t.Fatal("immune child never reached its trap installation")
	}

	if serr := reg.Stop(id); serr != nil {
		t.Fatalf("Stop: %v", serr)
	}

	// The task must be terminal well within grace*10 (TERM observed, KILL
	// after grace, reap bounded).
	deadline := time.Now().Add(escalationGraceTest * 10)

	for time.Now().Before(deadline) {
		if state, ok := reg.Lookup(id); ok && state != bgRunning {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if state, _ := reg.Lookup(id); state == bgRunning {
		t.Fatal("TERM-immune child still running after grace + SIGKILL — the ladder's KILL rung failed")
	}

	// SIGTERM was DELIVERED first (the observer recorded it before any KILL
	// could end the group).
	termSeen := false

	for time.Now().Before(deadline) {
		if b, rerr := os.ReadFile(marker); rerr == nil && strings.TrimSpace(string(b)) == "seen" {
			termSeen = true

			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if !termSeen {
		t.Error("no TERM observation — the ladder must TERM the group before any KILL")
	}
}

// TestEscalation_TermRespectingChildDiesByTerm (PAR-08): a child whose TERM
// trap exits 7 ends at the SIGTERM rung — the recorded exit status is the
// trapped 7 (a SIGKILL would read 137), and the task is terminal BEFORE the
// grace window could have fired (2s grace; a TERM death lands in ms).
func TestEscalation_TermRespectingChildDiesByTerm(t *testing.T) { //nolint:funlen // flat ladder battery
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	reg.termGrace = 2 * time.Second // a KILL would take >= 2s; TERM lands in ms

	var (
		hmu    sync.Mutex
		status string
	)

	reg.CompletionHook = func(taskID, kind, exitStatus string, _ time.Duration, _, _ string) {
		hmu.Lock()
		defer hmu.Unlock()

		status = exitStatus
	}

	ready := filepath.Join(dir, "respect-ready")

	id, _, err := reg.Start(dir, `echo up > `+ready+`; trap "exit 7" TERM; sleep 300`)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !waitForFile(t, ready, 5*time.Second) {
		t.Fatal("TERM-respecting child never reached its trap installation")
	}

	stopStart := time.Now()

	if serr := reg.Stop(id); serr != nil {
		t.Fatalf("Stop: %v", serr)
	}

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if state, ok := reg.Lookup(id); ok && state != bgRunning {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	elapsed := time.Since(stopStart)

	if state, _ := reg.Lookup(id); state == bgRunning {
		t.Fatal("TERM-respecting child still running — the TERM rung failed")
	}

	if elapsed >= 2*time.Second {
		t.Errorf("Stop took %v — the child died at the grace/KILL rung, not the TERM rung", elapsed)
	}

	// The trapped status 7 (SIGKILL would surface as 137 via 128+SIGKILL).
	hmu.Lock()
	defer hmu.Unlock()

	if status != "7" {
		t.Errorf("completion exit status = %q; want \"7\" (the TERM trap's chosen exit)", status)
	}
}

// TestEscalation_AlreadyGoneIsSuccess (ESRCH-clean): Stop on a task that
// already exited cleanly is a nil error (group gone — never a failure).
func TestEscalation_AlreadyGoneIsSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()

	id, _, err := reg.Start(dir, "echo done")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if state, ok := reg.Lookup(id); ok && state != bgRunning {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	if serr := reg.Stop(id); serr != nil {
		t.Errorf("Stop on an already-exited task = %v; want nil (ESRCH-clean)", serr)
	}
}

// TestEscalation_ReapAllUsesLadder: ReapAll terminates a live task through
// the same ladder (TERM rung observable) and the registry ends empty of
// running tasks.
func TestEscalation_ReapAllUsesLadder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	reg.termGrace = escalationGraceTest

	marker := filepath.Join(dir, "reap-term-marker")

	ready := filepath.Join(dir, "reap-ready")

	id, _, err := reg.Start(dir, `echo up > `+ready+`; trap "echo seen > `+marker+`" TERM; sleep 300`)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !waitForFile(t, ready, 5*time.Second) {
		t.Fatal("reap child never reached its trap installation")
	}

	reg.ReapAll()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if state, ok := reg.Lookup(id); ok && state != bgRunning {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if state, _ := reg.Lookup(id); state == bgRunning {
		t.Fatal("ReapAll left a task running — the ladder must terminate it")
	}

	if b, rerr := os.ReadFile(marker); rerr != nil || strings.TrimSpace(string(b)) != "seen" {
		t.Error("ReapAll's termination never delivered TERM (the ladder's first rung)")
	}
}

// waitForFile polls for a file's existence (the escalation battery's
// ready-gate: traps must be INSTALLED before the ladder fires).
func waitForFile(t *testing.T, path string, timeout time.Duration) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}

		time.Sleep(10 * time.Millisecond)
	}

	return false
}

// The D-11 queue-on-cap battery (22-02 Task 2): over-cap starts QUEUE with
// a visible note (FIFO, drained on completion) instead of failing; the
// pathological bound still errors; ReapAll drops waiters (OQ5).

// TestBackgroundCap_OverCapQueues (D-11): past the injected cap, Start
// returns immediately with the id + queued=true; the command starts only
// when a slot frees; the registry reports the queued state.
func TestBackgroundCap_OverCapQueues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	reg.Cap = 1
	reg.termGrace = escalationGraceTest

	id1, queued1, err := reg.Start(dir, "echo first; sleep 0.4")
	if err != nil {
		t.Fatalf("Start 1: %v", err)
	}

	if queued1 {
		t.Error("first start queued=true; want false (under cap)")
	}

	id2, queued2, err := reg.Start(dir, "echo second")
	if err != nil {
		t.Fatalf("Start 2: %v", err)
	}

	if !queued2 {
		t.Error("over-cap start queued=false; want true (D-11 queue contract)")
	}

	if state, ok := reg.Lookup(id2); !ok || state != bgQueued {
		t.Errorf("queued task state = %v; want queued", state)
	}

	// Both ids are distinct exec_-shaped handles minted at dispatch.
	if id1 == id2 || !strings.HasPrefix(id2, "exec_") {
		t.Errorf("queued id = %q; want a distinct exec_ handle", id2)
	}

	// Non-blocking Output on the queued task reports the queued state.
	out, state, running, oerr := reg.Output(id2, false, 0)
	if oerr != nil || running || state != bgQueued {
		t.Errorf("queued Output = (%q, %v, running=%v, err=%v); want the queued state, not-running", out, state, running, oerr)
	}

	// When slot 1 frees, the queued task starts and completes.
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if state, ok := reg.Lookup(id2); ok && state == bgDone {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if state, _ := reg.Lookup(id2); state != bgDone {
		t.Errorf("queued task state = %v; want done (FIFO drain on completion)", state)
	}

	got, _, _, _ := reg.Output(id2, false, 0)
	if !strings.Contains(got, "second") {
		t.Errorf("drained task output = %q; want the queued command's output", got)
	}
}

// TestBackgroundCap_FIFOOrder (PAR-08 ordering probe): queued starts drain
// in submission order — with cap 1 the single slot serializes, and the
// completion-hook order must match the submission order exactly (ties by
// sequence, never map order).
func TestBackgroundCap_FIFOOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	reg.Cap = 1

	var (
		hmu   sync.Mutex
		order []string
	)

	reg.CompletionHook = func(taskID, _, _ string, _ time.Duration, tail, _ string) {
		hmu.Lock()
		defer hmu.Unlock()

		order = append(order, tail)
	}

	// The tags embed submission order in the output; FIFO drain makes the
	// completion order fifo-a, fifo-b, fifo-c.
	for _, cmd := range []string{
		"echo fifo-a; sleep 0.3", // the cap occupant
		"echo fifo-b",
		"echo fifo-c",
	} {
		if _, _, err := reg.Start(dir, cmd); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		hmu.Lock()
		n := len(order)
		hmu.Unlock()

		if n == 3 {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	hmu.Lock()
	defer hmu.Unlock()

	want := []string{"fifo-a", "fifo-b", "fifo-c"}
	if len(order) != 3 {
		t.Fatalf("completions = %v; want 3", order)
	}

	for i, w := range want {
		if !strings.Contains(order[i], w) {
			t.Errorf("drain order[%d] = %q; want %q (FIFO submission order)", i, order[i], w)
		}
	}
}

// TestBackgroundCap_QueuedStop (D-11): Stop on a queued task removes it
// from the queue with a stopped outcome — its start func never fires.
func TestBackgroundCap_QueuedStop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	reg.Cap = 1

	if _, _, err := reg.Start(dir, "echo hold; sleep 0.5"); err != nil {
		t.Fatal(err)
	}

	id2, queued, err := reg.Start(dir, "echo never-starts")
	if err != nil || !queued {
		t.Fatalf("over-cap Start = (%v, %v)", queued, err)
	}

	if serr := reg.Stop(id2); serr != nil {
		t.Fatalf("Stop(queued): %v", serr)
	}

	if state, ok := reg.Lookup(id2); !ok || state != bgStopped {
		t.Errorf("stopped queued task state = %v; want stopped", state)
	}

	// When the slot frees, the stopped task must NOT start.
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if state, ok := reg.Lookup(id2); ok && state != bgQueued {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	out, state, _, _ := reg.Output(id2, false, 0)
	if strings.Contains(out, "never-starts") {
		t.Errorf("stopped queued task RAN (output %q) — must never start", out)
	}

	if state != bgStopped {
		t.Errorf("state = %v; want stopped (never restarted)", state)
	}
}

// TestBackgroundCap_QueueBound (D-10 bounded resources): past the
// documented bound (64) the structured cap error survives — the queue
// cannot grow unbounded.
func TestBackgroundCap_QueueBound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	reg.Cap = 1

	if _, _, err := reg.Start(dir, "echo bound-hold; sleep 0.6"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < bgQueueBound; i++ {
		_, _, err := reg.Start(dir, "echo bound-waiter")
		if err != nil {
			t.Fatalf("waiter %d: %v", i, err)
		}
	}

	_, _, err := reg.Start(dir, "echo bound-overflow")
	if err == nil {
		t.Fatal("bound-breach Start accepted; want the structured cap error")
	}

	if !errors.Is(err, errBgCap) {
		t.Errorf("err = %v; want the errBgCap sentinel", err)
	}

	if !strings.Contains(err.Error(), "64") {
		t.Errorf("err = %v; want the bound named", err)
	}
}

// TestBackgroundCap_ReapAllDropsWaiters (OQ5, bash leg): ReapAll cancels
// running tasks AND drops queued-but-unstarted ones without starting them.
func TestBackgroundCap_ReapAllDropsWaiters(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()
	reg.Cap = 1
	reg.termGrace = escalationGraceTest

	ready := filepath.Join(dir, "cap-reap-ready")

	id1, _, err := reg.Start(dir, "echo up > "+ready+"; sleep 300")
	if err != nil {
		t.Fatal(err)
	}

	if !waitForFile(t, ready, 5*time.Second) {
		t.Fatal("running task never started")
	}

	id2, queued, err := reg.Start(dir, "echo queued-never")
	if err != nil || !queued {
		t.Fatalf("over-cap Start = (%v, %v)", queued, err)
	}

	dropped := reg.ReapAll()

	if dropped != 1 {
		t.Errorf("ReapAll dropped = %d; want 1 (the queued waiter)", dropped)
	}

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if state, ok := reg.Lookup(id1); ok && state != bgRunning {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if state, _ := reg.Lookup(id1); state == bgRunning {
		t.Error("running task survived ReapAll — must terminate via the ladder")
	}

	if out, _, _, _ := reg.Output(id2, false, 0); strings.Contains(out, "queued-never") {
		t.Error("queued waiter started after ReapAll — must be dropped, never started")
	}
}

// --- 22-06 (SAND-01): the background-site sandbox batteries ---

// bgSandboxHandle builds the live Handle exactly as the flag path does (see
// liveSandboxHandle in bash_test.go); skips where the host cannot enforce.
func bgSandboxHandle(t *testing.T, workDir string) sandbox.Handle {
	t.Helper()

	h := sandbox.Resolve(sandbox.DefaultPolicy(workDir, os.TempDir(),
		filepath.Join(workDir, ".ass-guard")))
	if !h.Availability.Available {
		t.Skipf("sandbox enforcement unavailable on this host: %s", h.Availability.Reason)
	}

	return h
}

// waitForTerminal polls the registry until the task leaves running/queued.
func waitForTerminal(t *testing.T, reg *TaskRegistry, id string) bgState {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		state, ok := reg.Lookup(id)
		if !ok {
			t.Fatalf("task %s vanished", id)
		}

		if state != bgRunning && state != bgQueued {
			return state
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("task %s never reached a terminal state", id)

	return ""
}

// bgStartCurl starts a backgrounded curl-style connect against the local
// listener through StartWithOpts and returns the task id + port.
func bgStartCurl(t *testing.T, reg *TaskRegistry, workDir string, opts StartOpts) (string, int) {
	t.Helper()

	ln, lerr := net.Listen("tcp", "127.0.0.1:0")
	if lerr != nil {
		t.Fatalf("listen: %v", lerr)
	}

	srv := &http.Server{}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close(); _ = ln.Close() })

	port := ln.Addr().(*net.TCPAddr).Port

	id, _, serr := reg.StartWithOpts(workDir,
		fmt.Sprintf("curl -s --max-time 3 -o /dev/null http://127.0.0.1:%d/ ; echo CURL_EXIT_$?", port), opts)
	if serr != nil {
		t.Fatalf("StartWithOpts: %v", serr)
	}

	return id, port
}

// TestBackgroundSandbox_LiveDeniesNetwork: a backgrounded curl-style command
// started via TaskRegistry.Start fails its network connect while the task
// lifecycle completes normally (running → terminal) — confinement proven
// through START'S OWN exec construction, not the foreground site (the
// checker-identified silent gap this closes). The control arm (sandbox off)
// connects.
func TestBackgroundSandbox_LiveDeniesNetwork(t *testing.T) { //nolint:paralleltest // live pty/listener + shared host probe
	workDir := t.TempDir()
	handle := bgSandboxHandle(t, workDir)

	// Confined arm: the connect must fail.
	reg := NewTaskRegistry()
	reg.Sandbox = &handle
	reg.SandboxNote = func(string, ...any) {}
	reg.Cap = 4

	id, _ := bgStartCurl(t, reg, workDir, StartOpts{})

	state := waitForTerminal(t, reg, id)

	out, _, _, oerr := reg.Output(id, false, 0)
	if oerr != nil {
		t.Fatalf("Output: %v", oerr)
	}

	if strings.Contains(out, "CURL_EXIT_0") {
		t.Errorf("the backgrounded curl CONNECTED under --sandbox=on — Start's construction is unconfined: %q", out)
	}

	if state == bgRunning {
		t.Error("the confined task never left the running state")
	}

	// Control arm: the SAME shape with the sandbox off connects.
	plain := NewTaskRegistry()
	plain.Cap = 4

	pid, _ := bgStartCurl(t, plain, workDir, StartOpts{})

	waitForTerminal(t, plain, pid)

	pout, _, _, perr := plain.Output(pid, false, 0)
	if perr != nil {
		t.Fatalf("control Output: %v", perr)
	}

	if !strings.Contains(pout, "CURL_EXIT_0") {
		t.Errorf("the CONTROL background curl failed with the sandbox OFF (a broken battery): %q", pout)
	}
}

// TestBackgroundSandbox_QueuedStartWrapsIdentically: the FIFO queued-start
// path (22-02's D-11) wraps identically — a registry at cap 1 with one
// running filler queues the curl task; when the slot frees it launches and is
// confined exactly as an immediately-started one.
func TestBackgroundSandbox_QueuedStartWrapsIdentically(t *testing.T) { //nolint:paralleltest // queued timing + live probe
	workDir := t.TempDir()
	handle := bgSandboxHandle(t, workDir)

	reg := NewTaskRegistry()
	reg.Sandbox = &handle
	reg.SandboxNote = func(string, ...any) {}
	reg.Cap = 1

	fillerID, _, ferr := reg.StartWithOpts(workDir, "sleep 1", StartOpts{})
	if ferr != nil {
		t.Fatalf("filler start: %v", ferr)
	}

	id, _ := bgStartCurl(t, reg, workDir, StartOpts{})

	if pos := reg.QueuedPosition(id); pos == 0 {
		t.Fatalf("the curl task did not queue behind the filler (position 0)")
	}

	waitForTerminal(t, reg, fillerID)
	waitForTerminal(t, reg, id)

	out, _, _, oerr := reg.Output(id, false, 0)
	if oerr != nil {
		t.Fatalf("Output: %v", oerr)
	}

	if strings.Contains(out, "CURL_EXIT_0") {
		t.Errorf("the QUEUED background curl CONNECTED under --sandbox=on — the FIFO launch path is unconfined: %q", out)
	}
}

// TestBackgroundSandbox_DefaultOffArgvIdentity: with the sandbox code present
// but off, Start's exec construction is byte-identical — the wrap seam never
// fires, zero notes, and the task runs to completion (the six TestBackground_*
// batteries plus 22-02's queue ladder already pin the behavior; this row pins
// the SEAM).
func TestBackgroundSandbox_DefaultOffArgvIdentity(t *testing.T) {
	// NOT t.Parallel: swaps the package wrap seam.
	restore, called := wrapRecorder()
	defer restore()

	notes, lines := captureNotes()

	reg := NewTaskRegistry()
	reg.SandboxNote = notes
	reg.Cap = 4

	id, _, serr := reg.StartWithOpts(t.TempDir(), "echo off-ok", StartOpts{})
	if serr != nil {
		t.Fatalf("start: %v", serr)
	}

	waitForTerminal(t, reg, id)

	if *called {
		t.Error("the wrap seam fired on the off path — Start's construction must stay byte-identical")
	}

	if len(*lines) != 0 {
		t.Errorf("off path emitted notes: %v", *lines)
	}

	out, _, _, oerr := reg.Output(id, false, 0)
	if oerr != nil || !strings.Contains(out, "off-ok") {
		t.Errorf("off-path output = %q err = %v; want the plain run", out, oerr)
	}
}

// TestBackgroundSandbox_StopOnConfinedTaskCompletes: Stop's ladder
// (terminateGroup TERM → grace → KILL + reap) completes on a WRAPPED child —
// confinement never breaks cleanup (D-05: signals are neither FS nor network
// operations).
func TestBackgroundSandbox_StopOnConfinedTaskCompletes(t *testing.T) { //nolint:paralleltest // kill timing + live probe
	workDir := t.TempDir()
	handle := bgSandboxHandle(t, workDir)

	reg := NewTaskRegistry()
	reg.Sandbox = &handle
	reg.SandboxNote = func(string, ...any) {}
	reg.Cap = 4

	id, _, serr := reg.StartWithOpts(workDir, "sleep 2995 & sleep 2996", StartOpts{})
	if serr != nil {
		t.Fatalf("start: %v", serr)
	}

	time.Sleep(300 * time.Millisecond) // let the group form

	if serr := reg.Stop(id); serr != nil {
		t.Fatalf("Stop on a confined task: %v", serr)
	}

	if _, perr := exec.LookPath("pgrep"); perr == nil {
		deadline := time.Now().Add(3 * time.Second)

		for time.Now().Before(deadline) {
			pout, _ := exec.CommandContext(context.Background(), "pgrep", "-f", "sleep 299").Output()
			if strings.TrimSpace(string(pout)) == "" {
				return // the confined group is provably gone
			}

			time.Sleep(100 * time.Millisecond)
		}

		pout, _ := exec.CommandContext(context.Background(), "pgrep", "-f", "sleep 299").Output()
		t.Errorf("orphaned confined child after Stop's ladder: %s", pout)
	}
}

// TestBackgroundSandbox_DisableAndUnavailableNotes: OQ2 parity at the
// background site — the disable escape runs UNCONFINED (the confined-deny
// curl now CONNECTS) with exactly ONE note; a faked-unavailable Handle runs
// unconfined with a per-run note + counter (never a silent fail-open).
func TestBackgroundSandbox_DisableAndUnavailableNotes(t *testing.T) { //nolint:funlen,paralleltest // live probe + shared counter
	workDir := t.TempDir()
	handle := bgSandboxHandle(t, workDir)

	notes, lines := captureNotes()

	// Disable arm: on + available + dangerouslyDisableSandbox → connect
	// SUCCEEDS + exactly one note.
	reg := NewTaskRegistry()
	reg.Sandbox = &handle
	reg.SandboxNote = notes
	reg.Cap = 4

	id, _ := bgStartCurl(t, reg, workDir, StartOpts{DisableSandbox: true})
	waitForTerminal(t, reg, id)

	out, _, _, oerr := reg.Output(id, false, 0)
	if oerr != nil {
		t.Fatalf("disable-arm Output: %v", oerr)
	}

	if !strings.Contains(out, "CURL_EXIT_0") {
		t.Errorf("the disable-arm background curl FAILED — the escape must run unconfined: %q", out)
	}

	if len(*lines) != 1 || !strings.Contains((*lines)[0], "UNCONFINED") {
		t.Errorf("disable-arm notes = %v; want exactly ONE loud note", *lines)
	}

	// Unavailable arm: faked availability → unconfined + one note PER RUN.
	//nolint:exhaustruct // faked availability (the on marker, probe failed)
	fake := &sandbox.Handle{
		Policy:       sandbox.DefaultPolicy(workDir, os.TempDir(), filepath.Join(workDir, ".ass-guard")),
		Availability: sandbox.Availability{Mode: "landlock", Reason: "faked-unavailable-bg"},
	}

	notes2, lines2 := captureNotes()

	reg2 := NewTaskRegistry()
	reg2.Sandbox = fake
	reg2.SandboxNote = notes2
	reg2.Cap = 4

	for i := 0; i < 2; i++ {
		bid, _ := bgStartCurl(t, reg2, workDir, StartOpts{})
		waitForTerminal(t, reg2, bid)

		bout, _, _, berr := reg2.Output(bid, false, 0)
		if berr != nil || !strings.Contains(bout, "CURL_EXIT_0") {
			t.Errorf("unavailable-arm run %d did not run unconfined: %q (%v)", i, bout, berr)
		}
	}

	if len(*lines2) != 2 {
		t.Errorf("unavailable-arm notes = %d; want one PER RUN: %v", len(*lines2), *lines2)
	}

	if len(*lines2) > 0 && !strings.Contains((*lines2)[0], "faked-unavailable-bg") {
		t.Errorf("the note does not name the reason: %q", (*lines2)[0])
	}
}

// --- 22-09 (G-22-5, Task 1): the registry-unknown-id fallback seam ---------------
//
// Background-subagent task ids (exec_* minted by the tasks tracker) are
// structurally unknown to the TaskRegistry; the two constructor seams let the
// runtime address them WITHOUT a dependency on the tasks package (the
// CompletionHook precedent). The rows pin: the captured ack / not_ready /
// ready envelope forms on handled ids, the declined and nil shapes (today's
// structured unknown-task error, unchanged), and that registry-owned ids
// never consult the seam.

// TestTaskStop_FallbackAck: the registry does NOT know the id; the fallback
// claims and stops it → the CAPTURED ack names the id, and exactly the
// requested id rides the seam.
func TestTaskStop_FallbackAck(t *testing.T) {
	t.Parallel()

	var seen []string

	reg := NewTaskRegistry()
	stop := TaskStopExecute(reg, func(id string) bool {
		seen = append(seen, id)
		return true
	})

	out, err := stop(context.Background(), json.RawMessage(`{"task_id":"exec_sub_stop1"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (the fallback stopped the subagent)", err)
	}

	var ack string

	_ = json.Unmarshal(out, &ack)

	if ack != "Task exec_sub_stop1 stopped." {
		t.Errorf("ack = %q; want the captured ack naming the id", ack)
	}

	if len(seen) != 1 || seen[0] != "exec_sub_stop1" {
		t.Errorf("fallback consulted with %v; want exactly [exec_sub_stop1]", seen)
	}
}

// TestTaskStop_FallbackDeclined: the fallback declines the id → the existing
// structured unknown-task error, unchanged.
func TestTaskStop_FallbackDeclined(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()
	stop := TaskStopExecute(reg, func(string) bool { return false })

	out, err := stop(context.Background(), json.RawMessage(`{"task_id":"exec_sub_no"}`))
	if err == nil {
		t.Fatal("err = nil; want the unknown-task error (the fallback declined the id)")
	}

	var structured struct {
		Error string `json:"error"`
	}

	_ = json.Unmarshal(out, &structured)

	if !strings.Contains(structured.Error, "exec_sub_no") || !strings.Contains(structured.Error, "unknown task") {
		t.Errorf("error = %q; want the structured unknown-task error echoing the id", structured.Error)
	}
}

// TestTaskStop_NilFallback: a nil seam (the pre-22-09 callers — the cmd
// wiring battery's nil-binding contract) keeps exactly today's behavior for
// registry-unknown ids: the structured unknown-task error.
func TestTaskStop_NilFallback(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()
	stop := TaskStopExecute(reg, nil)

	out, err := stop(context.Background(), json.RawMessage(`{"task_id":"exec_sub_nil"}`))
	if err == nil {
		t.Fatal("err = nil; want the unknown-task error (nil seam never fires)")
	}

	var structured struct {
		Error string `json:"error"`
	}

	_ = json.Unmarshal(out, &structured)

	if !strings.Contains(structured.Error, "exec_sub_nil") {
		t.Errorf("error = %q; want the structured unknown-task error echoing the id", structured.Error)
	}
}

// TestTaskStop_RegistryOwnedSkipsFallback: the fallthrough arms ONLY on the
// registry's unknown-task error — a registry-owned id stops through the
// registry machinery (the same ack) and the seam is never consulted.
func TestTaskStop_RegistryOwnedSkipsFallback(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()

	id, _, err := reg.Start(t.TempDir(), "sleep 5")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	consulted := false

	stop := TaskStopExecute(reg, func(string) bool {
		consulted = true
		return true
	})

	out, err := stop(context.Background(), json.RawMessage(`{"task_id":"`+id+`"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (the registry owns the id)", err)
	}

	var ack string

	_ = json.Unmarshal(out, &ack)

	if ack != "Task "+id+" stopped." {
		t.Errorf("ack = %q; want the captured ack naming the registry id", ack)
	}

	if consulted {
		t.Error("the fallback was consulted for a registry-owned id; the fallthrough must arm only on the unknown-task error")
	}
}

// TestTaskOutput_FallbackNotReadyStates: a handled fallback id renders the
// CAPTURED not_ready envelope — status queued for a queued subagent, status
// running for a running one — identical to the registry's own form. block and
// timeout are IGNORED for fallback ids (the documented choice: the tracker
// seam renders the CURRENT state immediately; it carries no bounded-wait
// machinery, and the schema's promise is that the id is ADDRESSABLE).
func TestTaskOutput_FallbackNotReadyStates(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()

	for _, tc := range []struct{ name, id, status string }{
		{"queued", "exec_sub_q1", "queued"},
		{"running", "exec_sub_r1", "running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			status := tc.status
			to := TaskOutputExecute(reg, func(string) (string, string, bool, bool) {
				return "", status, true, true // running=true → the not_ready family
			})

			start := time.Now()

			out, err := to(context.Background(), json.RawMessage(
				`{"task_id":"`+tc.id+`","block":true,"timeout":5000}`))
			if err != nil {
				t.Fatalf("err = %v; want nil (a not_ready snapshot is not an error)", err)
			}

			if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
				t.Errorf("elapsed = %v; want immediate (block/timeout ignored for fallback ids)", elapsed)
			}

			var text string

			_ = json.Unmarshal(out, &text)

			want := "<retrieval_status>not_ready</retrieval_status>\n\n" +
				"<task_id>" + tc.id + "</task_id>\n\n" +
				"<task_type>local_bash</task_type>\n\n" +
				"<status>" + tc.status + "</status>"

			if text != want {
				t.Errorf("not_ready envelope = %q; want %q", text, want)
			}
		})
	}
}

// TestTaskOutput_FallbackFinishedOutput: a handled FINISHED fallback id
// renders the ready envelope whose output section carries the fallback's
// output content (the tracker-side seam reads the subagent's output file).
func TestTaskOutput_FallbackFinishedOutput(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()
	to := TaskOutputExecute(reg, func(string) (string, string, bool, bool) {
		return "subagent finished: 3 files changed", "finished", false, true
	})

	out, err := to(context.Background(), json.RawMessage(
		`{"task_id":"exec_sub_f1","block":true,"timeout":5000}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (the finished envelope is not an error)", err)
	}

	var text string

	_ = json.Unmarshal(out, &text)

	want := "<retrieval_status>ready</retrieval_status>\n\n" +
		"<task_id>exec_sub_f1</task_id>\n\n" +
		"<task_type>local_bash</task_type>\n\n" +
		"<status>finished</status>\n\n" +
		"<output>\nsubagent finished: 3 files changed\n</output>"

	if text != want {
		t.Errorf("ready envelope = %q; want %q", text, want)
	}
}

// TestTaskOutput_FallbackDeclined: the fallback does not handle the id → the
// existing structured unknown-task error, unchanged.
func TestTaskOutput_FallbackDeclined(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()
	to := TaskOutputExecute(reg, func(string) (string, string, bool, bool) {
		return "", "", false, false
	})

	out, err := to(context.Background(), json.RawMessage(
		`{"task_id":"exec_sub_outno","block":false,"timeout":100}`))
	if err == nil {
		t.Fatal("err = nil; want the unknown-task error (the fallback declined the id)")
	}

	var structured struct {
		Error string `json:"error"`
	}

	_ = json.Unmarshal(out, &structured)

	if !strings.Contains(structured.Error, "exec_sub_outno") || !strings.Contains(structured.Error, "unknown task") {
		t.Errorf("error = %q; want the structured unknown-task error echoing the id", structured.Error)
	}
}

// TestTaskOutput_NilFallback: a nil seam keeps exactly today's behavior for
// registry-unknown ids: the structured unknown-task error (the fallthrough
// never fires).
func TestTaskOutput_NilFallback(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()
	to := TaskOutputExecute(reg, nil)

	out, err := to(context.Background(), json.RawMessage(
		`{"task_id":"exec_sub_outnil","block":false,"timeout":100}`))
	if err == nil {
		t.Fatal("err = nil; want the unknown-task error (nil seam never fires)")
	}

	var structured struct {
		Error string `json:"error"`
	}

	_ = json.Unmarshal(out, &structured)

	if !strings.Contains(structured.Error, "exec_sub_outnil") {
		t.Errorf("error = %q; want the structured unknown-task error echoing the id", structured.Error)
	}
}

// TestTaskOutput_RegistryOwnedSkipsFallback: a registry-owned id renders
// through the registry path (not_ready while running) and the seam is never
// consulted.
func TestTaskOutput_RegistryOwnedSkipsFallback(t *testing.T) {
	t.Parallel()

	reg := NewTaskRegistry()

	id, _, err := reg.Start(t.TempDir(), "sleep 2")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	consulted := false

	to := TaskOutputExecute(reg, func(string) (string, string, bool, bool) {
		consulted = true
		return "", "running", true, true
	})

	out, err := to(context.Background(), json.RawMessage(
		`{"task_id":"`+id+`","block":false,"timeout":100}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (the registry owns the id)", err)
	}

	var text string

	_ = json.Unmarshal(out, &text)

	if !strings.HasPrefix(text, "<retrieval_status>not_ready</retrieval_status>") ||
		!strings.Contains(text, "<status>running</status>") {
		t.Errorf("registry form = %q; want the captured not_ready/running envelope", text)
	}

	if consulted {
		t.Error("the fallback was consulted for a registry-owned id; the fallthrough must arm only on the unknown-task error")
	}
}
