package session

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// The minimal ask queue (17-02, D-11..D-13 core): ONE fired head at a time —
// never stacked modals — with a FIFO completion chain. Producers Enqueue and
// return; the queue's pump goroutine fires the head (the human-timescale
// surface round-trip: registry-backed session/request_permission) and drives
// its completion (the session-side resume), then the next head. The broker's
// single-slot discipline is demoted to this queue's one-outstanding fired-head
// invariant (the 17-02 assumption-delta decision): the queue is the single
// firing path for every ask surface.
//
// Priority classes (D-11: foreground before subagent/engine), queued-entry
// notes + counter (D-12), and the turn-id drain (D-13) expand in 17-03; the
// entries already carry turn id + class + seq for them.

// AskClass mirrors D-11's priority classes (the emitter's fg/bg lineage):
// foreground-turn asks fire before subagent/engine asks. Within a class, ask
// order is preserved (Seq).
type AskClass uint8

const (
	// AskClassForeground — a client-driven turn's ask.
	AskClassForeground AskClass = iota
	// AskClassBackground — an automation/engine (or subagent-origin) ask.
	AskClassBackground
)

// AskOutcome is the session-native ask resolution — what the fire callback
// (the surface round-trip) hands back. It is deliberately wire-agnostic: the
// session package never imports internal/acp.
type AskOutcome struct {
	// Selected is the option kind the operator picked ("" unless a selection
	// was reported). For permission asks: the four canonical option kinds.
	Selected string
	// Cancelled marks the cancelled family: a dismissed dialog, the client's
	// cancelled outcome, a -32800 answer, turn death, or the shutdown drain.
	Cancelled bool
	// Err is any failure that is NOT a cancellation (surface unwired,
	// -32601 from a pre-permission client, D-14 timeout fallback, malformed
	// replies). The ask routes to the fail-safe decline — never a silent
	// allow, never a retry storm.
	Err error
	// Unsupported marks the -32601 degrade family (the client cannot answer
	// this ask method at all): the session records the degradation STICKY for
	// the session (16-D-18) so later asks decline without a new round-trip.
	// A plain Err (timeout, transient transport failure) is not sticky.
	Unsupported bool
}

// AskEntry is one queued ask. The enqueue site fills the identity fields and
// the fire func (built from the injected surface); the queue owns firing.
type AskEntry struct {
	TurnID    string
	SessionID string
	CallID    string
	Tool      string
	// Title is the dialog card's title; Kind its optional tool kind.
	Title string
	Kind  string
	// Input is the gated call's raw input (the dialog shows what it is
	// authorizing — untrusted content DISPLAYED, never executed).
	Input json.RawMessage
	// Class is the D-11 priority class; Seq the FIFO order within it.
	Class AskClass
	Seq   uint64

	// fire performs the surface round-trip (assigned by the enqueue site from
	// GateDeps.Fire; nil = unwirable surface → fail-safe Err outcome).
	fire func() AskOutcome
}

// queuedAsk pairs the entry with its session-side completion (the resume).
type queuedAsk struct {
	entry   *AskEntry
	resolve func(*AskEntry, AskOutcome)
}

// AskQueue is the minimal one-outstanding ask queue. All state is
// mutex-guarded; the pump runs on its own goroutine and NEVER holds any turn
// lock — a suspended turn holds nothing while its ask is queued or open
// (criterion 2).
type AskQueue struct {
	mu     sync.Mutex
	queued []queuedAsk
	firing bool
	seq    atomic.Uint64
}

// NewAskQueue returns an empty queue.
func NewAskQueue() *AskQueue { return &AskQueue{} }

// Enqueue appends one ask and starts the pump when idle. Enqueue never
// blocks on a human: the suspending turn already ended, so callers are
// turn-loop goroutines or resume paths that hold no turn lock.
func (q *AskQueue) Enqueue(e *AskEntry, resolve func(*AskEntry, AskOutcome)) {
	if e == nil {
		return
	}

	if e.Seq == 0 {
		e.Seq = q.seq.Add(1)
	}

	q.mu.Lock()
	q.queued = append(q.queued, queuedAsk{entry: e, resolve: resolve})
	start := !q.firing
	q.firing = true
	q.mu.Unlock()

	if start {
		go q.pump()
	}
}

// Pending reports the number of queued-but-unfired asks (the basis of D-12's
// visibility counter; the note emission lands in 17-03).
func (q *AskQueue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.queued)
}

// pump is the one-outstanding fire loop: pop the head, fire it (the
// human-timescale round-trip), drive its completion, repeat — strictly FIFO
// within the queue (D-11's one-modal invariant; the next dialog never fires
// before the previous ask's resume finished).
func (q *AskQueue) pump() {
	for {
		q.mu.Lock()
		if len(q.queued) == 0 {
			q.firing = false
			q.mu.Unlock()

			return
		}

		head := q.queued[0]
		q.queued = q.queued[1:]
		q.mu.Unlock()

		outcome := AskOutcome{Err: errPermissionSurfaceUnwired}
		if head.entry.fire != nil {
			outcome = head.entry.fire()
		}

		if head.resolve != nil {
			head.resolve(head.entry, outcome)
		}
	}
}
