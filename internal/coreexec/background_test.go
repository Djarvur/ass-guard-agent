package coreexec //nolint:testpackage // internal package test (fixture helpers shared)

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
	to := TaskOutputExecute(reg)

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
	to := TaskOutputExecute(reg)

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
	id, serr := reg.Start(t.TempDir(), "sleep 2")
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
	to := TaskOutputExecute(reg)

	id, serr := reg.Start(t.TempDir(), "sleep 2")
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
	to := TaskOutputExecute(reg)

	id, serr := reg.Start(t.TempDir(), "echo out-line; echo err-line 1>&2")
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
	id0, err := reg.Start(dir, "echo hook-zero-marker")
	if err != nil {
		t.Fatalf("Start (exit 0): %v", err)
	}

	// Nonzero exit leg.
	id1, err := reg.Start(dir, "echo hook-fail-marker; exit 3")
	if err != nil {
		t.Fatalf("Start (exit 3): %v", err)
	}

	// Killed leg (Stop marks the state before the group kill).
	id2, err := reg.Start(dir, "echo hook-kill-marker; sleep 30")
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

	id, err := reg.Start(dir, "echo no-hook")
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
	command := `sh -c 'trap "echo seen > ` + marker + `" TERM; while true; do sleep 0.05; done'`

	id, err := reg.Start(dir, command)
	if err != nil {
		t.Fatalf("Start: %v", err)
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

	id, err := reg.Start(dir, `sh -c 'trap "exit 7" TERM; sleep 300'`)
	if err != nil {
		t.Fatalf("Start: %v", err)
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

	id, err := reg.Start(dir, "echo done")
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

	id, err := reg.Start(dir, `sh -c 'trap "echo seen > `+marker+`" TERM; sleep 300'`)
	if err != nil {
		t.Fatalf("Start: %v", err)
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
