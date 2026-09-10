package tasks //nolint:testpackage // internal package test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
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

// countRunning snapshots the tracker's live-subagent count (the same-package
// D-10 accounting seam — the G-22-1 battery pins the counter directly).
func countRunning(tr *Tracker) int {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	return tr.runningSubagents
}

// cancelKnown reports whether the tracker still holds a cancel registration
// for id (the finished-vs-running seam 22-09's classifier consumes).
func cancelKnown(tr *Tracker, id string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	_, ok := tr.subagentCancels[id]

	return ok
}

// TestTrackerCap_SlotsFreeAfterCompletion (G-22-1, CR-01): completions
// actually FREE D-10 slots — after SubagentCap TOTAL starts in a session the
// admission check must still pass, the exact row that is the blind spot of
// TestTrackerQueue_FIFODrainOrder (that battery never re-registers after its
// completions fire). The handoff row also pins that release-then-admit keeps
// the count AT the cap, never above it.
func TestTrackerCap_SlotsFreeAfterCompletion(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{SubagentCap: 2})
	rs := &recordingStart{}

	// Fill the cap.
	for _, id := range []string{"exec_f1", "exec_f2"} {
		queued, err := tr.RegisterSubagent(id, rs.start(id))
		if err != nil {
			t.Fatalf("RegisterSubagent(%s): %v", id, err)
		}

		if queued {
			t.Fatalf("RegisterSubagent(%s) queued=true; want false (under cap)", id)
		}
	}

	// The third registration queues with the visible position note.
	started3 := make(chan string, 1)

	queued, err := tr.RegisterSubagent("exec_f3", func() func() {
		started3 <- "exec_f3"
		return func() {}
	})
	if err != nil {
		t.Fatalf("RegisterSubagent(exec_f3): %v", err)
	}

	if !queued {
		t.Fatal("3rd registration queued=false; want true (cap 2 reached)")
	}

	if note := tr.QueuedNote("exec_f3"); !strings.Contains(note, "exec_f3") || !strings.Contains(note, "position 1") {
		t.Errorf("QueuedNote = %q; want the id and position 1", note)
	}

	// Completing f1 frees its slot: the waiter starts (channel-observable,
	// bounded so a pre-fix failure is an assertion, not a hang) and the
	// release-then-admit handoff keeps the count at the cap — never above.
	tr.Complete(Notification{TaskID: "exec_f1", Kind: KindSubagent, ExitStatus: "0"})

	select {
	case id := <-started3:
		if id != "exec_f3" {
			t.Errorf("waiter started = %s; want exec_f3", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued task never started — the completion did not free its slot")
	}

	if got := countRunning(tr); got != 2 {
		t.Errorf("runningSubagents = %d after the handoff; want 2 (never above the cap)", got)
	}

	// Drain everything: with empty waiters the count returns to zero.
	tr.Complete(Notification{TaskID: "exec_f2", Kind: KindSubagent, ExitStatus: "0"})
	tr.Complete(Notification{TaskID: "exec_f3", Kind: KindSubagent, ExitStatus: "0"})

	if got := countRunning(tr); got != 0 {
		t.Errorf("runningSubagents = %d; want 0 after all completions with empty waiters", got)
	}

	// THE blind-spot row: after cap TOTAL starts (3 so far), a further
	// registration still admits directly and really starts.
	started4 := make(chan string, 1)

	queued, err = tr.RegisterSubagent("exec_f4", func() func() {
		started4 <- "exec_f4"
		return func() {}
	})
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}

	if queued {
		t.Fatal("re-register after completions queued=true; want false (slots must free)")
	}

	select {
	case id := <-started4:
		if id != "exec_f4" {
			t.Errorf("re-registered start = %s; want exec_f4", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("re-registered task never started — the freed slot was not admitted")
	}

	tr.Complete(Notification{TaskID: "exec_f4", Kind: KindSubagent, ExitStatus: "0"})

	if got := countRunning(tr); got != 0 {
		t.Errorf("runningSubagents = %d; want 0 at rest", got)
	}
}

// TestTrackerCap_SequentialReuseNeverQueues (G-22-1, CR-01): sequential use
// never starves — 3×cap register→complete cycles all admit directly (the cap
// bounds CONCURRENT subagents, not lifetime starts; a monotonic counter
// bricks every session past its first cap-full of dispatches).
func TestTrackerCap_SequentialReuseNeverQueues(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{SubagentCap: 2})
	rs := &recordingStart{}

	for i := 0; i < 6; i++ {
		id := "exec_u" + string(rune('0'+i))

		queued, err := tr.RegisterSubagent(id, rs.start(id))
		if err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}

		if queued {
			t.Fatalf("cycle %d queued=true; want direct admission (sequential reuse)", i)
		}

		tr.Complete(Notification{TaskID: id, Kind: KindSubagent, ExitStatus: "0"})
	}

	if got := rs.startedList(); len(got) != 6 {
		t.Errorf("started = %v; want all 6", got)
	}

	if got := countRunning(tr); got != 0 {
		t.Errorf("runningSubagents = %d; want 0 after 6 sequential cycles", got)
	}
}

// TestTrackerComplete_RetiresCancelEntry (G-22-1): Complete retires the
// completing subagent's cancel registration — finished subagents stop looking
// running (pre-fix the map is populated at RegisterSubagent and
// startNextWaiter only, with no delete path, so a finished id classifies as
// running forever), CancelTask on a completed id is an idempotent no-op
// (false), the decrement is floored at zero, and KindBash completions never
// touch the subagent count.
func TestTrackerComplete_RetiresCancelEntry(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{SubagentCap: 2})
	rs := &recordingStart{}

	for _, id := range []string{"exec_x1", "exec_x2"} {
		if _, err := tr.RegisterSubagent(id, rs.start(id)); err != nil {
			t.Fatalf("RegisterSubagent(%s): %v", id, err)
		}
	}

	// KindBash completions never touch the subagent count.
	before := countRunning(tr)
	tr.Complete(Notification{TaskID: "bash_b1", Kind: KindBash, ExitStatus: "0"})

	if after := countRunning(tr); after != before {
		t.Errorf("runningSubagents %d -> %d on a KindBash completion; want untouched", before, after)
	}

	// Complete both subagents: the cancel entries retire with the slots.
	tr.Complete(Notification{TaskID: "exec_x1", Kind: KindSubagent, ExitStatus: "0"})
	tr.Complete(Notification{TaskID: "exec_x2", Kind: KindSubagent, ExitStatus: "0"})

	for _, id := range []string{"exec_x1", "exec_x2"} {
		if cancelKnown(tr, id) {
			t.Errorf("subagentCancels still holds %s after Complete — finished looks running", id)
		}
	}

	if tr.CancelTask("exec_x1") {
		t.Error("CancelTask(completed) = true; want false (the cancel retired)")
	}

	// Floor safety: a KindSubagent Complete with no matching admission
	// leaves the count at zero, never negative.
	tr.Complete(Notification{TaskID: "exec_ghost", Kind: KindSubagent, ExitStatus: "error"})

	if got := countRunning(tr); got != 0 {
		t.Errorf("runningSubagents = %d after a ghost completion; want 0 (floored)", got)
	}
}
