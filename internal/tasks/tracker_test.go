package tasks //nolint:testpackage // internal package test

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// The tracker cap/queue battery (22-01 Task 3, D-10 + OQ5): the subagent
// cap, FIFO queueing with a visible note, close-time queued-cancel, the
// bounded queue, and the default cap.

// recordingStart is a start func that records invocation order and hands
// back a cancel func recording its own invocation.
type recordingStart struct {
	mu      sync.Mutex
	started []string
	cancels []string
}

func (rs *recordingStart) start(id string) func() func() {
	return func() func() {
		rs.mu.Lock()
		rs.started = append(rs.started, id)
		rs.mu.Unlock()

		return func() {
			rs.mu.Lock()
			rs.cancels = append(rs.cancels, id)
			rs.mu.Unlock()
		}
	}
}

func (rs *recordingStart) startedList() []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	out := make([]string, len(rs.started))
	copy(out, rs.started)

	return out
}

func (rs *recordingStart) canceledList() []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	out := make([]string, len(rs.cancels))
	copy(out, rs.cancels)

	return out
}

// TestTrackerCap_UnderCapStartsImmediately (D-10): registrations under the
// cap start immediately (queued=false); the cap boundary itself (the Nth
// concurrent registration) still starts.
func TestTrackerCap_UnderCapStartsImmediately(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{SubagentCap: 3})
	rs := &recordingStart{}

	for _, id := range []string{"exec_s1", "exec_s2", "exec_s3"} {
		queued, err := tr.RegisterSubagent(id, rs.start(id))
		if err != nil {
			t.Fatalf("RegisterSubagent(%s): %v", id, err)
		}

		if queued {
			t.Errorf("RegisterSubagent(%s) queued=true; want false (under cap)", id)
		}
	}

	got := rs.startedList()
	if len(got) != 3 {
		t.Fatalf("started = %v; want all 3 immediately", got)
	}

	// The 4th (over cap) queues — the cap is 3, not 4.
	queued, err := tr.RegisterSubagent("exec_s4", rs.start("exec_s4"))
	if err != nil {
		t.Fatalf("RegisterSubagent(exec_s4): %v", err)
	}

	if !queued {
		t.Error("4th registration queued=false; want true (cap 3 reached)")
	}

	if got := rs.startedList(); len(got) != 3 {
		t.Errorf("started = %v; want still 3 (the queued task must NOT start)", got)
	}
}

// TestTrackerQueue_FIFODrainOrder (D-10, PAR-08 ordering probe): over-cap
// registrations queue in submission order; each subagent completion starts
// exactly ONE queued task — FIFO order stable under equal completion times
// (submission order breaks ties).
func TestTrackerQueue_FIFODrainOrder(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{SubagentCap: 2})
	rs := &recordingStart{}

	// Fill the cap.
	for _, id := range []string{"exec_r1", "exec_r2"} {
		if _, err := tr.RegisterSubagent(id, rs.start(id)); err != nil {
			t.Fatalf("RegisterSubagent(%s): %v", id, err)
		}
	}

	// Queue three (no clock advance — equal completion times everywhere).
	for _, id := range []string{"exec_q1", "exec_q2", "exec_q3"} {
		queued, err := tr.RegisterSubagent(id, rs.start(id))
		if err != nil {
			t.Fatalf("RegisterSubagent(%s): %v", id, err)
		}

		if !queued {
			t.Errorf("RegisterSubagent(%s) queued=false; want true", id)
		}
	}

	// A queued-note form is visible (D-10's "visible note").
	note := tr.QueuedNote("exec_q2")
	if !strings.Contains(note, "exec_q2") || !strings.Contains(note, "2") {
		t.Errorf("QueuedNote = %q; want the task id and its position", note)
	}

	// One completion frees a slot: exactly ONE queued task starts (q1).
	tr.Complete(Notification{TaskID: "exec_r1", Kind: KindSubagent, ExitStatus: "0"})

	got := rs.startedList()
	if len(got) != 3 {
		t.Fatalf("started = %v; want 3 (r1, r2, + q1)", got)
	}

	if got[2] != "exec_q1" {
		t.Errorf("first queued starter = %s; want exec_q1 (FIFO head)", got[2])
	}

	// Second completion → q2; third (q1's own completion) → q3.
	tr.Complete(Notification{TaskID: "exec_r2", Kind: KindSubagent, ExitStatus: "0"})
	tr.Complete(Notification{TaskID: "exec_q1", Kind: KindSubagent, ExitStatus: "0"})

	got = rs.startedList()
	if len(got) != 5 {
		t.Fatalf("started = %v; want all 5", got)
	}

	wantTail := []string{"exec_q1", "exec_q2", "exec_q3"}
	for i, want := range wantTail {
		if got[2+i] != want {
			t.Errorf("drain order[%d] = %s; want %s (submission order)", i, got[2+i], want)
		}
	}
}

// TestTrackerClose_DropsQueuedNotRunning (OQ5): CancelQueued drops
// queued-but-unstarted tasks silently (start funcs NEVER invoked) and
// returns the dropped count; RUNNING tasks' cancel funcs are NOT invoked.
func TestTrackerClose_DropsQueuedNotRunning(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{SubagentCap: 1})
	rs := &recordingStart{}

	if _, err := tr.RegisterSubagent("exec_run1", rs.start("exec_run1")); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"exec_w1", "exec_w2"} {
		if _, err := tr.RegisterSubagent(id, rs.start(id)); err != nil {
			t.Fatal(err)
		}
	}

	dropped := tr.CancelQueued()
	if dropped != 2 {
		t.Errorf("dropped = %d; want 2 (the queue length)", dropped)
	}

	if got := rs.startedList(); len(got) != 1 {
		t.Errorf("started = %v; queued tasks must never start", got)
	}

	if got := rs.canceledList(); len(got) != 0 {
		t.Errorf("canceled = %v; Close must NOT touch running tasks' cancels", got)
	}

	// The queued registrations are gone: the next slot-free pops nothing.
	tr.Complete(Notification{TaskID: "exec_run1", Kind: KindSubagent})

	if got := rs.startedList(); len(got) != 1 {
		t.Errorf("started = %v; a post-close completion must start nothing", got)
	}

	// Second close: nothing queued → 0.
	if again := tr.CancelQueued(); again != 0 {
		t.Errorf("second CancelQueued = %d; want 0", again)
	}
}

// TestTrackerQueue_BoundRejects (D-10 bounded resources): a pathological
// queue past the documented bound (64) rejects with the structured error
// naming the bound.
func TestTrackerQueue_BoundRejects(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{SubagentCap: 1})
	rs := &recordingStart{}

	if _, err := tr.RegisterSubagent("exec_b0", rs.start("exec_b0")); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 64; i++ {
		queued, err := tr.RegisterSubagent("exec_b"+string(rune('a'+i%26))+string(rune('a'+i/26)), rs.start("x"))
		if err != nil {
			t.Fatalf("waiter %d: %v", i, err)
		}

		if !queued {
			t.Fatalf("waiter %d queued=false; want true", i)
		}
	}

	// The 65th waiter: past the bound.
	_, err := tr.RegisterSubagent("exec_b65", rs.start("exec_b65"))
	if err == nil {
		t.Fatal("65th registration accepted; want the bounded-queue rejection")
	}

	if !errors.Is(err, errQueueBound) {
		t.Errorf("err = %v; want the errQueueBound sentinel", err)
	}

	if !strings.Contains(err.Error(), "64") {
		t.Errorf("err = %v; want the bound NAMED in the message", err)
	}
}

// TestTrackerDefaultCap (D-10): the zero-value TrackerOpts yields the
// documented default cap of 8 (the 9th registration queues).
func TestTrackerDefaultCap(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{})
	rs := &recordingStart{}

	for i := 0; i < 8; i++ {
		id := "exec_d" + string(rune('0'+i))

		queued, err := tr.RegisterSubagent(id, rs.start(id))
		if err != nil {
			t.Fatalf("RegisterSubagent(%s): %v", id, err)
		}

		if queued {
			t.Fatalf("registration %d queued=true; want under-cap (default 8)", i+1)
		}
	}

	queued, err := tr.RegisterSubagent("exec_d9", rs.start("exec_d9"))
	if err != nil {
		t.Fatal(err)
	}

	if !queued {
		t.Error("9th registration queued=false; want true (default cap 8)")
	}

	if got := rs.startedList(); len(got) != 8 {
		t.Errorf("started = %d; want exactly the 8 default-cap tasks", len(got))
	}
}

// TestTrackerCancelTask invokes the stored cancel for a running subagent
// and reports unknown ids (the 22-03 TaskStop leg's seam).
func TestTrackerCancelTask(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{SubagentCap: 2})
	rs := &recordingStart{}

	if _, err := tr.RegisterSubagent("exec_c1", rs.start("exec_c1")); err != nil {
		t.Fatal(err)
	}

	if ok := tr.CancelTask("exec_c1"); !ok {
		t.Error("CancelTask(known) = false; want true")
	}

	if got := rs.canceledList(); len(got) != 1 || got[0] != "exec_c1" {
		t.Errorf("canceled = %v; want [exec_c1]", got)
	}

	if ok := tr.CancelTask("exec_unknown"); ok {
		t.Error("CancelTask(unknown) = true; want false")
	}
}
