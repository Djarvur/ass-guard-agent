package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// The ask queue (17-02 minimal core, expanded by 17-03 to the full D-11..
// D-12 discipline): ONE fired head at a time — never stacked modals — with
// priority classes and a visible queue. Producers Enqueue and return; the
// queue's pump goroutine fires the head (the human-timescale surface
// round-trip: registry-backed session/request_permission) and drives its
// completion (the session-side resume), then the next head. The broker's
// single-slot discipline is demoted to this queue's one-outstanding fired-head
// invariant (the 17-02 assumption-delta decision): the queue is the single
// firing path for every ask surface.
//
// Priority (D-11): the foreground turn's asks fire before subagent/engine
// asks — a foreground enqueue preempts the WAITING queue head; within a class
// ask order is preserved (Seq, FIFO); an OPEN entry is never preempted
// regardless of class. Visibility (D-12): an enqueue that does not fire
// immediately emits one session/update note ("ask queued — N pending") through
// the injected subscriber-backed emitter and bumps the enqueue counter — the
// same counter family as the stall metrics. Immediate fires emit no note
// (spam killed by construction).
//
// Concurrency: the queue's own mutex guards the slice and the firing flag and
// is NEVER held across a fire callback's human-scale wait — the pump fires on
// its own goroutine continuation (criterion 2: nothing blocks holding a turn
// lock). The turn-id drain (D-13) lands with the teardown wiring in 17-03's
// second task; every entry already carries its TurnID.

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

// askQueuedNoteFormat is the pinned D-12 note text (CONTEXT locks the wording
// family; the pending count includes the just-queued entry).
const askQueuedNoteFormat = "ask queued — %d pending"

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
	// GateDeps.Fire; nil = unwirable surface → fail-safe Err outcome). The
	// queue hands every fire a cancellable ctx it owns — the drain seam the
	// D-13 turn-death resolution cancels (a registry-backed fire surfaces the
	// cancellation as the cancelled outcome family).
	fire func(ctx context.Context) AskOutcome
}

// queuedAsk pairs the entry with its session-side completion (the resume).
type queuedAsk struct {
	entry   *AskEntry
	resolve func(*AskEntry, AskOutcome)
}

// openAsk is the one OUTSTANDING ask: promoted out of the waiting slice at
// the moment it begins firing (deterministically — by the enqueue that found
// the queue idle, or by the pump between rounds).
type openAsk struct {
	qa queuedAsk
}

// AskQueue is the one-outstanding ask queue with D-11 priority classes and
// D-12 visibility. All state is mutex-guarded; the pump runs on its own
// goroutine and NEVER holds any turn lock — a suspended turn holds nothing
// while its ask is queued or open (criterion 2).
type AskQueue struct {
	mu     sync.Mutex
	queued []queuedAsk
	open   *openAsk
	firing bool
	// pumpRunning tracks the pump goroutine so the idle->busy transition
	// starts exactly one (Enqueue promotes the first entry itself; the pump
	// re-promotes between rounds).
	pumpRunning bool
	seq         atomic.Uint64
	// count is the D-12 enqueue counter (every enqueue — immediate fires
	// included; the same loud-counter family as the stall metrics).
	count atomic.Uint64
	// note is the injected subscriber-backed note emitter (D-12): composition
	// wires it to the bus path a subscriber always serves (the session-
	// lifetime forwarder or the live turn's own). nil = notes unwired.
	note func(e *AskEntry, note string)
}

// NewAskQueue returns an empty queue.
func NewAskQueue() *AskQueue { return &AskQueue{} }

// SetNoteEmitter injects the D-12 note sink. It MUST be wired at composition
// to a subscriber-backed emission path (in-hand emitter or collector-drain) —
// a bare post-turn bus publish is dropped when no subscriber exists (the
// 13-03 timing hazard), so the queue calls the emitter synchronously from the
// enqueue site and composition owns the always-subscribed delivery.
func (q *AskQueue) SetNoteEmitter(fn func(e *AskEntry, note string)) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.note = fn
}

// Enqueue appends one ask and starts the pump when idle. Enqueue never
// blocks on a human: the suspending turn already ended, so callers are
// turn-loop goroutines or resume paths that hold no turn lock. An enqueue
// that fires immediately emits no note (and is promoted out of the waiting
// set synchronously); one that queues emits exactly one "ask queued — N
// pending" note (N = the waiting depth including itself). Both bump the
// enqueue counter. (The pump's fire ctx is deliberately Background-derived —
// see pump.)
func (q *AskQueue) Enqueue(e *AskEntry, resolve func(*AskEntry, AskOutcome)) {
	if e == nil {
		return
	}

	var startPump bool

	q.mu.Lock()

	if e.Seq == 0 {
		e.Seq = q.seq.Add(1)
	}

	q.count.Add(1)

	qa := queuedAsk{entry: e, resolve: resolve}

	var note string

	emit := q.note

	if !q.firing {
		// Idle queue: THIS ask is the one outstanding — promote it out of the
		// waiting set synchronously so notes and Pending() are deterministic
		// (the pump goroutine's pop can never race the count).
		q.firing = true
		q.pumpRunning = true
		q.open = &openAsk{qa: qa}
		startPump = true
	} else {
		q.queued = append(q.queued, qa)
		note = fmt.Sprintf(askQueuedNoteFormat, len(q.queued))
	}

	q.mu.Unlock()

	if startPump {
		go q.pump()
	}

	if note != "" && emit != nil {
		emit(e, note)
	}
}

// Enqueued reports the D-12 counter: every enqueue since the queue was
// created, immediate fires included (the counter-family snapshot).
func (q *AskQueue) Enqueued() uint64 { return q.count.Load() }

// Pending reports the number of queued-but-unfired asks (the visible queue
// depth behind the one outstanding dialog).
func (q *AskQueue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.queued)
}

// popHeadLocked selects and removes the next entry to fire (caller holds mu):
// the earliest foreground-class entry — a later foreground enqueue preempts
// the waiting queue head (D-11) — else the earliest overall. The slice is
// append-ordered and Seq is monotonic, so scan order IS age order.
func (q *AskQueue) popHeadLocked() queuedAsk {
	head := 0

	for i, qa := range q.queued {
		if qa.entry.Class == AskClassForeground {
			head = i

			break
		}
	}

	picked := q.queued[head]
	q.queued = append(q.queued[:head], q.queued[head+1:]...)

	return picked
}

// pump is the one-outstanding fire loop: fire the open ask (the
// human-timescale round-trip), drive its completion, promote the next head by
// priority, repeat. The queue mutex is NEVER held across the fire or the
// resolve — a suspended turn holds nothing while a human decides (criterion
// 2), and a drain (D-13) can always reach the state. The fire ctx is
// deliberately Background-derived — the suspending turn's ctx died with its
// prompt response (askResumeCtx precedent); the queue owns the seam.
func (q *AskQueue) pump() {
	for {
		q.mu.Lock()

		cur := q.open

		if cur == nil {
			if len(q.queued) == 0 {
				q.firing = false
				q.pumpRunning = false
				q.mu.Unlock()

				return
			}

			promoted := &openAsk{qa: q.popHeadLocked()}
			q.open = promoted
			cur = promoted
		}

		q.mu.Unlock()

		// The fire ctx is the queue-owned cancellation seam: the drain cancels
		// it to resolve an OPEN dialog through the surface's own cancelled
		// family (a registry-backed fire cascades $/cancel_request). The ctx
		// is released as soon as the round-trip returns.
		fctx, fcancel := context.WithCancel(context.Background())

		outcome := AskOutcome{Err: errPermissionSurfaceUnwired}
		if cur.qa.entry.fire != nil {
			outcome = cur.qa.entry.fire(fctx)
		}

		fcancel()

		q.mu.Lock()

		q.open = nil

		q.mu.Unlock()

		if cur.qa.resolve != nil {
			cur.qa.resolve(cur.qa.entry, outcome)
		}
	}
}
