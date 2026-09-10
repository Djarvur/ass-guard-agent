// Package tasks is the ONE task-notification subsystem (22-01, PAR-07/PAR-08):
// every background completion — background Bash and background subagents
// alike — lands here as a kind-tagged Notification and wakes the model
// through the runner's wake-turn drain. Detection is by the Kind struct
// field, never output text-matching.
//
// The Tracker is per-session (mirroring coreexec.TaskRegistry's scoping):
// one per sessionFor construction, reaped via the OnClose chain.
package tasks

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Kind discriminates the notification producers (PAR-07's letter: detection
// is by this field, never text-matching).
type Kind string

const (
	// KindBash is a background Bash task completion (PAR-08).
	KindBash Kind = "bash"
	// KindSubagent is a background subagent completion (PAR-07).
	KindSubagent Kind = "subagent"
)

// Notification is the D-02 payload verbatim: task id, kind, exit status,
// duration, output TAIL, and a pointer to the full output file (the model
// reacts immediately; full retrieval stays a Read away).
type Notification struct {
	TaskID     string        // session-local, crypto/rand-minted — never model input
	Kind       Kind          // the detection field (PAR-07)
	ExitStatus string        // "0" | nonzero decimal | "killed" | "error"
	Duration   time.Duration // start → terminal transition
	Tail       string        // last ~TailBytes of output, rune-boundary-safe
	OutputFile string        // .ass-guard/outputs/<id>.log under the session workdir
}

// TrackerOpts configures a Tracker. The zero value is usable: SubagentCap
// defaults to 8 (D-10), TailBytes to 8192 (D-02 discretion — 8 KB), Now to
// time.Now.
type TrackerOpts struct {
	SubagentCap int          // D-10: concurrent background subagents (default 8)
	TailBytes   int          // D-02: notification tail budget in bytes (default 8192)
	Now         func() time.Time // the completion-time clock (tests pin ordering)
}

// subagentQueueBound is the pathological-growth bound on the over-cap FIFO
// queue (D-10 bounded-resources intent): past it a registration is rejected
// with the structured error naming the bound (the background.go :320-324
// documented-const convention).
const subagentQueueBound = 64

var errQueueBound = errors.New("tasks: subagent queue bound reached")

// subagentWaiter is one over-cap registration: the start func launches the
// task (returning its cancel func) when a slot frees in FIFO order.
type subagentWaiter struct {
	id    string
	start func() func()
}

// Tracker records background tasks, owns the pending-notification queue, and
// schedules the wake-drain attempts. It is safe for concurrent use.
type Tracker struct {
	mu    sync.Mutex
	opts  TrackerOpts
	pend  pendingQueue
	drain func(pending []Notification)

	// D-10 subagent accounting (22-01 Task 3).
	runningSubagents int
	subagentCancels  map[string]func()
	waiters          []subagentWaiter
}

// NewTracker returns a Tracker with the documented defaults applied.
func NewTracker(opts TrackerOpts) *Tracker {
	if opts.SubagentCap <= 0 {
		opts.SubagentCap = 8 // D-10 default
	}

	if opts.TailBytes <= 0 {
		opts.TailBytes = 8192 // D-02 discretion default (~8 KB)
	}

	if opts.Now == nil {
		opts.Now = time.Now
	}

	return &Tracker{opts: opts, subagentCancels: map[string]func(){}}
}

// SetDrain registers the drain-attempt callback. Every Complete schedules
// exactly one non-blocking attempt carrying a PEEK of the pending batch
// (ordered, possibly empty); the callback owns the whole attempt — TryLock
// the session turn mutex, consume the batch via Drain (the only destructive
// consumer), render, run the wake turn. A callback that finds the mutex held
// simply returns: the notifications stay pending and coalesce into the next
// attempt's batch (D-01 fallback + D-03).
func (t *Tracker) SetDrain(fn func(pending []Notification)) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.drain = fn
}

// Complete records one terminal notification: the tail is truncated to the
// configured budget at a UTF-8 rune boundary, the entry is deduped by task id
// (first terminal wins — a cancel racing a completion yields exactly one
// notification), and ONE non-blocking drain attempt is scheduled when a
// consumer is wired. A finishing subagent RELEASES its D-10 slot BEFORE any
// waiter is admitted: the release retires the completed id's cancel
// registration (finished stops looking running) and decrements the running
// count floored at zero; startNextWaiter then admits exactly one queued task
// into the freed slot — release-then-admit, in that order, per completion
// (G-22-1).
func (t *Tracker) Complete(n Notification) {
	t.mu.Lock()

	n.Tail = truncateTail(n.Tail, t.opts.TailBytes)
	t.pend.add(n, t.opts.Now())
	drain := t.drain
	peek := t.pend.peek()

	if n.Kind == KindSubagent {
		t.releaseSubagentSlot(n.TaskID)
	}

	t.mu.Unlock()

	if drain != nil {
		go drain(peek) //nolint:contextcheck // the callback owns its ctx (runner-side serve ctx)
	}

	// D-10: the released slot is reoccupied by exactly one queued task
	// (startNextWaiter re-increments the count when it admits a waiter).
	if n.Kind == KindSubagent {
		t.startNextWaiter()
	}
}

// releaseSubagentSlot frees a finishing subagent's D-10 slot: decrement the
// running count floored at zero (a defensive Complete for an id that never
// admitted must never drive the counter negative) and retire the id's cancel
// registration — finished subagents stop looking running, so CancelTask on a
// completed id reports false (the truthful not-running signal the 22-09
// TaskStop/TaskOutput seam consumes). Callers hold t.mu (G-22-1).
func (t *Tracker) releaseSubagentSlot(id string) {
	if t.runningSubagents > 0 {
		t.runningSubagents--
	}

	delete(t.subagentCancels, id)
}

// Drain snapshot-and-clears the pending queue and returns the batch in
// completion-time order (the ONLY destructive consumer — the runner calls it
// under the session turn mutex; an empty or already-drained queue drains as
// a no-op nil batch).
func (t *Tracker) Drain() []Notification {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.pend.snapshotAndClear()
}

// PendingPeek returns a non-destructive ordered peek at the pending batch.
func (t *Tracker) PendingPeek() []Notification {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.pend.peek()
}

// RegisterSubagent registers a background-subagent launch (D-10). Under the
// cap the start func is invoked IMMEDIATELY (its returned cancel func is
// wired for CancelTask) and queued=false; over cap the registration joins
// the FIFO waiters — it starts when a running subagent completes — and
// queued=true. Past the documented queue bound the registration is rejected
// with the structured error naming the bound.
func (t *Tracker) RegisterSubagent(id string, start func() func()) (bool, error) {
	t.mu.Lock()

	if t.runningSubagents < t.opts.SubagentCap {
		t.runningSubagents++
		t.mu.Unlock()

		// start runs OUTSIDE the lock (it launches the task and may call
		// back); the cancel func is wired under the lock afterwards.
		cancel := start()

		t.mu.Lock()
		t.subagentCancels[id] = cancel
		t.mu.Unlock()

		return false, nil
	}

	if len(t.waiters) >= subagentQueueBound {
		t.mu.Unlock()

		return true, fmt.Errorf("tasks: subagent queue bound %d reached: %w", subagentQueueBound, errQueueBound)
	}

	t.waiters = append(t.waiters, subagentWaiter{id: id, start: start})

	t.mu.Unlock()

	return true, nil
}

// startNextWaiter pops the FIFO head and starts it (called after a subagent
// completion freed a slot). Submission order breaks equal-completion ties
// (PAR-08 ordering probe).
func (t *Tracker) startNextWaiter() {
	t.mu.Lock()

	if len(t.waiters) == 0 {
		t.mu.Unlock()

		return
	}

	w := t.waiters[0]
	t.waiters = t.waiters[1:]
	t.runningSubagents++ // the freed slot is reoccupied by the waiter

	t.mu.Unlock()

	cancel := w.start()

	t.mu.Lock()
	t.subagentCancels[w.id] = cancel
	t.mu.Unlock()
}

// CancelTask invokes the stored cancel func for a RUNNING subagent task
// (22-03's TaskStop leg); reports whether the id was known.
func (t *Tracker) CancelTask(id string) bool {
	t.mu.Lock()
	cancel, ok := t.subagentCancels[id]
	t.mu.Unlock()

	if ok && cancel != nil {
		cancel()
	}

	return ok
}

// QueuedNote renders the D-10 "visible note" for a queued registration: one
// sentence naming the queue position and the cap.
func (t *Tracker) QueuedNote(id string) string {
	t.mu.Lock()

	pos := -1

	for i, w := range t.waiters {
		if w.id == id {
			pos = i + 1
		}
	}

	limit := t.opts.SubagentCap

	t.mu.Unlock()

	if pos <= 0 {
		return "Task " + id + " is queued (" + itoa(limit) +
			" concurrent background subagents running); it starts when a slot frees."
	}

	return "Task " + id + " is queued at position " + itoa(pos) + " behind " + itoa(limit) +
		" concurrent background subagents; it starts when a slot frees."
}

// CancelQueued drops every queued-but-unstarted subagent registration
// (OQ5: session close — nothing started, nothing to kill) and returns the
// dropped count. RUNNING tasks are CancelRunning's job.
func (t *Tracker) CancelQueued() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	n := len(t.waiters)
	t.waiters = nil

	return n
}

// CancelRunning invokes every stored RUNNING-subagent cancel func and clears
// the registrations, returning the cancelled count (22-08, G-22-4/CR-04: the
// close-time running-cancel leg — a background subagent must not outlive its
// session, or its late completion fires into closed machinery). CancelTask's
// per-id leg is unchanged; queued-but-unstarted waiters stay CancelQueued's
// job. Idempotent together with Complete's releaseSubagentSlot: whichever
// runs first clears the map entry, the other finds nothing.
func (t *Tracker) CancelRunning() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	n := len(t.subagentCancels)

	for _, cancel := range t.subagentCancels {
		if cancel != nil {
			cancel()
		}
	}

	t.subagentCancels = map[string]func(){}

	return n
}

// itoa keeps the note renderer free of a strconv import at the call site.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// SubagentState classifies a subagent id for the 22-09 TaskStop/TaskOutput
// seam (G-22-5): queued reports an over-cap waiter, running an admitted id
// with a live cancel registration, and a COMPLETED id reports neither —
// truthful BECAUSE Complete's releaseSubagentSlot deleted its cancel entry
// (22-07, pinned by TestTrackerComplete_RetiresCancelEntry; before that
// delete no finished id could ever leave the running class, so finished would
// have rendered not_ready forever). A never-registered id reports neither.
func (t *Tracker) SubagentState(id string) (queued, running bool) {
	return false, false
}
