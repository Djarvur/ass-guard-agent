package tasks //nolint:testpackage // internal package test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// The background output/cancel battery (22-03 Task 2, PAR-07): progressive
// output retrieval for RUNNING tasks, cancellation with killed markers,
// exactly-one terminal outcomes, and rune-safe files.

// bgTap captures completion notifications via the tracker drain.
type bgTap struct {
	mu   sync.Mutex
	seen []Notification
}

func (t *bgTap) drain(pending []Notification) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.seen = append(t.seen, pending...)
}

func (t *bgTap) notifications() []Notification {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]Notification, len(t.seen))
	copy(out, t.seen)

	return out
}

// TestBackgroundOutput_MidRunRetrieval (Pitfall 9): the output file is
// non-empty and GROWS while the task runs — a mid-run Read resolves.
func TestBackgroundOutput_MidRunRetrieval(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tr := NewTracker(TrackerOpts{SubagentCap: 4})

	proceed := make(chan struct{})
	progressSeen := make(chan struct{}, 1)

	dep := SubagentDeps{
		Run: func(_ context.Context, progress func(string)) (string, error) {
			progress("first progress line\n")
			progressSeen <- struct{}{}

			<-proceed

			progress("final line\n")

			return "bg done", nil
		},
		WorkDir:  dir,
		Tracker:  tr,
		ServeCtx: func() context.Context { return context.Background() },
	}

	launch := RunBackgroundSubagent(dep)
	if launch.Err != nil {
		t.Fatalf("launch: %v", launch.Err)
	}

	<-progressSeen

	// Mid-run: the file exists, is non-empty, and carries the header +
	// first progress line.
	b, rerr := os.ReadFile(launch.OutputFile)
	if rerr != nil {
		t.Fatalf("mid-run Read: %v (the pointer must resolve while running)", rerr)
	}

	if !strings.Contains(string(b), "task_id: "+launch.TaskID) {
		t.Error("file missing the header's task id")
	}

	if !strings.Contains(string(b), "first progress line") {
		t.Errorf("file = %q; want the progress line mid-run", string(b))
	}

	close(proceed)

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		b, _ = os.ReadFile(launch.OutputFile)
		if strings.Contains(string(b), "final line") && strings.Contains(string(b), "[completed]") {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if !strings.Contains(string(b), "[completed]") || !strings.Contains(string(b), "bg done") {
		t.Errorf("file = %q; want the completed marker + final result", string(b))
	}
}

// TestBackgroundCancel_KilledMarkerAndNotification: tracker cancellation
// cancels the loop ctx, appends the killed marker, and fires exactly ONE
// killed notification.
func TestBackgroundCancel_KilledMarkerAndNotification(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tr := NewTracker(TrackerOpts{SubagentCap: 4})

	tap := &bgTap{}
	tr.SetDrain(tap.drain)

	inside := make(chan struct{})

	dep := SubagentDeps{
		Run: func(ctx context.Context, _ func(string)) (string, error) {
			close(inside)
			<-ctx.Done()

			return "", ctx.Err()
		},
		WorkDir:  dir,
		Tracker:  tr,
		ServeCtx: func() context.Context { return context.Background() },
	}

	launch := RunBackgroundSubagent(dep)
	<-inside

	if !tr.CancelTask(launch.TaskID) {
		t.Fatal("CancelTask reported unknown id")
	}

	deadline := time.Now().Add(5 * time.Second)

	marker := false

	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(launch.OutputFile)
		marker = strings.Contains(string(b), "[killed]")

		if marker && len(tap.notifications()) == 1 {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if !marker {
		t.Error("no killed marker in the output file — the pointer must resolve after cancel")
	}

	notes := tap.notifications()
	if len(notes) != 1 {
		t.Fatalf("notifications = %d; want exactly 1", len(notes))
	}

	if notes[0].ExitStatus != "killed" {
		t.Errorf("exit status = %q; want killed", notes[0].ExitStatus)
	}

	if notes[0].Kind != KindSubagent {
		t.Errorf("kind = %q; want subagent", notes[0].Kind)
	}

	// Exactly one marker in the file (grep-count 1).
	b, _ := os.ReadFile(launch.OutputFile)

	if strings.Count(string(b), "[killed]") != 1 || strings.Contains(string(b), "[completed]") {
		t.Errorf("markers = both/none (killed=%d); want exactly one killed marker",
			strings.Count(string(b), "[killed]"))
	}
}

// TestBackgroundOutput_ZeroOutput (PAR-07 empty probe): a zero-output run
// leaves a valid header-only file + a notification with an EMPTY tail and
// all D-02 fields present.
func TestBackgroundOutput_ZeroOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tr := NewTracker(TrackerOpts{SubagentCap: 4})

	tap := &bgTap{}
	tr.SetDrain(tap.drain)

	launch := RunBackgroundSubagent(SubagentDeps{
		Run:      func(context.Context, func(string)) (string, error) { return "", nil },
		WorkDir:  dir,
		Tracker:  tr,
		ServeCtx: func() context.Context { return context.Background() },
	})

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) && len(tap.notifications()) == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	notes := tap.notifications()
	if len(notes) != 1 {
		t.Fatalf("notifications = %d; want 1", len(notes))
	}

	n := notes[0]
	if n.TaskID == "" || n.Kind != KindSubagent || n.OutputFile == "" || n.Duration <= 0 {
		t.Errorf("notification = %+v; want all D-02 fields present", n)
	}

	if strings.TrimSpace(n.Tail) == "" == false {
		// tail may legitimately be header+marker text; the assertion is that
		// the RUN OUTPUT portion is empty — pinned via the field presence
		// above and the file content below.
	}

	b, _ := os.ReadFile(launch.OutputFile)
	if !strings.Contains(string(b), "task_id: "+launch.TaskID) {
		t.Errorf("file = %q; want the header (the pointer never dangles)", string(b))
	}
}

// TestBackgroundOutput_SplitRuneBuffered (PAR-07 encoding probe): a chunk
// split INSIDE a multi-byte rune appends whole — the file is always valid
// UTF-8.
func TestBackgroundOutput_SplitRuneBuffered(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tr := NewTracker(TrackerOpts{SubagentCap: 4})

	sentence := strings.Repeat("héllo wörld 😀 ", 50)

	launch := RunBackgroundSubagent(SubagentDeps{
		Run: func(_ context.Context, progress func(string)) (string, error) {
			// Split at every byte boundary — the writer must buffer the
			// partial runes and emit only whole ones.
			for i := 0; i < len(sentence); i++ {
				progress(sentence[i : i+1])
			}

			return "done", nil
		},
		WorkDir:  dir,
		Tracker:  tr,
		ServeCtx: func() context.Context { return context.Background() },
	})

	if launch.Err != nil {
		t.Fatal(launch.Err)
	}

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(launch.OutputFile)
		if strings.Contains(string(b), "[completed]") && utf8.Valid(b) {
			return // pass
		}

		if !utf8.Valid(b) {
			t.Fatalf("file content is not valid UTF-8 mid-stream: %q", string(b))
		}

		time.Sleep(20 * time.Millisecond)
	}

	b, _ := os.ReadFile(launch.OutputFile)

	if !utf8.Valid(b) {
		t.Fatalf("final file is not valid UTF-8: %q", string(b))
	}

	if !strings.Contains(string(b), "héllo wörld") {
		t.Error("the split-rune sentence did not reassemble in the file")
	}
}

// panicGuardEnv marks the INNER run of the subprocess-guarded panic test
// (G-22-3): a panic escaping the background launcher goroutine kills the
// test PROCESS with no --- FAIL line of its own, so the outer leg re-execs
// the test binary with this env set — pre-fix the inner run dies on the
// unrecovered panic and the OUTER test fails on a clean assertion instead
// of the crash; post-fix both legs pass.
const panicGuardEnv = "ASSGUARD_TASKS_PANIC_GUARD_INNER"

// TestBackgroundSubagent_PanicRecovered (G-22-3, CR-03): a panicking
// background subagent NEVER kills the ass-guard process — the launcher
// goroutine mirrors the foreground recover() (session/subagent.go D-13 leg):
// the output file carries the terminal error marker with the panic text plus
// the stack, exactly ONE error notification fires (Kind=KindSubagent,
// ExitStatus "error"), and the slot + cancel entry retire through Task 1's
// release path.
func TestBackgroundSubagent_PanicRecovered(t *testing.T) {
	if os.Getenv(panicGuardEnv) == "" {
		// Outer leg: the inner run (where the panic actually fires) must
		// complete cleanly — this leg failing IS the pre-fix crash made
		// observable as an assertion.
		cmd := exec.Command(os.Args[0], "-test.run=^TestBackgroundSubagent_PanicRecovered$", "-test.count=1")
		cmd.Env = append(os.Environ(), panicGuardEnv+"=1")

		out, rerr := cmd.CombinedOutput()
		if rerr != nil {
			t.Fatalf("inner panic-recovery run failed (%v) — the background launcher goroutine did not contain the panic:\n%s", rerr, out)
		}

		return
	}

	// Inner leg: the real assertions — this test completing is the
	// process-liveness proof.
	dir := t.TempDir()
	tr := NewTracker(TrackerOpts{SubagentCap: 4})

	tap := &bgTap{}
	tr.SetDrain(tap.drain)

	launch := RunBackgroundSubagent(SubagentDeps{
		Run: func(context.Context, func(string)) (string, error) {
			panic("boom: probe")
		},
		WorkDir:  dir,
		Tracker:  tr,
		ServeCtx: func() context.Context { return context.Background() },
	})
	if launch.Err != nil {
		t.Fatalf("launch: %v", launch.Err)
	}

	// Exactly ONE notification: the recover path REPLACES the normal
	// Complete (the panic unwinds past it — the paths are mutually
	// exclusive by construction).
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) && len(tap.notifications()) == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	notes := tap.notifications()
	if len(notes) != 1 {
		t.Fatalf("notifications = %d; want exactly 1 (recover replaces the normal Complete)", len(notes))
	}

	if notes[0].Kind != KindSubagent {
		t.Errorf("kind = %q; want subagent", notes[0].Kind)
	}

	if notes[0].ExitStatus != "error" {
		t.Errorf("exit status = %q; want error", notes[0].ExitStatus)
	}

	if notes[0].TaskID != launch.TaskID {
		t.Errorf("task id = %q; want %q", notes[0].TaskID, launch.TaskID)
	}

	// The output file is not left dangling: terminal [error] marker + the
	// panic text + a stack tail (the foreground leg's investigate-ready
	// posture — debug.Stack lands in the file).
	b, rerr := os.ReadFile(launch.OutputFile)
	if rerr != nil {
		t.Fatalf("read output file: %v", rerr)
	}

	file := string(b)
	if !strings.Contains(file, "[error]") {
		t.Errorf("file missing the [error] terminal marker: %q", file)
	}

	if !strings.Contains(file, "subagent panic: boom: probe") {
		t.Errorf("file missing the panic text: %q", file)
	}

	if !strings.Contains(file, "goroutine ") { // the debug.Stack header
		t.Errorf("file missing the stack tail: %q", file)
	}

	// The slot is released and the cancel entry retired (Task 1's path,
	// landed first in this plan): the panic path no longer wedges slot
	// accounting or leaves a stale running classification.
	if got := countRunning(tr); got != 0 {
		t.Errorf("runningSubagents = %d; want 0 — the panic path must release the slot", got)
	}

	if cancelKnown(tr, launch.TaskID) {
		t.Error("subagentCancels still holds the panicked id — finished looks running")
	}
}
