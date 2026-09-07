package session

import (
	"sync"
	"time"
)

// SteerItem is one queued steering input (23-01, SEEDG-01): an operator text
// that arrived while a turn was running, waiting for the next model-request
// boundary. Ticket is the monotonically increasing queue ticket (the
// cancel protocol's cutoff key — PITFALLS.md:329); At is the enqueue time
// (arrival-order provenance for notes/inspection; never reordered — D-02
// arrival order is the SLICE order, which Enqueue appends to).
type SteerItem struct {
	Ticket uint64
	Text   string
	At     time.Time
}

// SteerQueue is the per-session steering input queue (23-01, SEEDG-01): inputs
// enqueued while a turn runs are drained at the NEXT model-request boundary
// (runTurn's iteration top — after the previous iteration's tool results are
// appended and before Project, so a tool_use/tool_result pair can never be
// split) and delivered as ONE coalesced marker-wrapped user-role message
// (D-01/D-02). Steering never cancels or interrupts the running turn (D-04).
//
// Structurally modeled on AskBroker: a single mutex over queue-like state, NO
// channels — producer enqueues never block the drain, and the cancel protocol
// (CancelThrough/CancelAll) resolves covered items as cancelled-normal so an
// undelivered input can never zombie-deliver into a later turn's window
// (Pitfall 3). Transport-neutral by construction (TG-02's compile-level
// contract, pinned by TestSteerQueueNoACP): this file imports nothing outside
// the standard library — no ACP-wire package, no runtime package.
//
// Nil-receiver safe: every method tolerates a nil queue (a session without
// steering wired — "Nil = not wired", the ask *AskBroker field discipline).
type SteerQueue struct {
	mu    sync.Mutex
	items []SteerItem
	next  uint64
	// cutoff is the highest ticket ever covered by a CancelThrough/CancelAll
	// (monotonic inspection state for the ack protocol — 23-02's queued/
	// parked notes; delivered and cancelled items are removed from items, so
	// cutoff is the only record of what the cancel side resolved).
	cutoff uint64
}

// NewSteerQueue returns an empty queue.
func NewSteerQueue() *SteerQueue { return &SteerQueue{} }

// Enqueue appends one steering input and returns its ticket (strictly
// increasing across calls, starting at 1 — ticket 0 never exists, so a
// zero-value cutoff cancels nothing).
func (q *SteerQueue) Enqueue(text string) uint64 {
	if q == nil {
		return 0
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	q.next++

	q.items = append(q.items, SteerItem{Ticket: q.next, Text: text, At: time.Now().UTC()})

	return q.next
}

// Drain returns every undelivered, un-cancelled item in arrival order (D-02)
// and advances the watermark by REMOVING them — delivered items never
// re-deliver at a later boundary, so the queue cannot accumulate across
// boundaries regardless of enqueue count (T-23-02). Empty (nil) batch when
// nothing is queued — the every-iteration drain is a no-op hot path.
func (q *SteerQueue) Drain() []SteerItem {
	if q == nil {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	batch := q.items
	q.items = nil

	return batch
}

// CancelThrough resolves every item with Ticket <= t as cancelled-normal and
// removes it, returning the count (the acknowledgment — a cancelled input
// never becomes a steering_delivery line, so it can never reach a later model
// window, live or on replay). Items with Ticket > t survive and are returned
// by the next Drain (the ticket/cutoff protocol, PITFALLS.md:329).
func (q *SteerQueue) CancelThrough(t uint64) int {
	if q == nil {
		return 0
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if t > q.cutoff {
		q.cutoff = t
	}

	kept := q.items[:0]

	var n int

	for _, it := range q.items {
		if it.Ticket <= t {
			n++

			continue
		}

		kept = append(kept, it)
	}

	q.items = kept

	return n
}

// CancelAll resolves every remaining item (CancelThrough the highest assigned
// ticket) and returns the count — the wrapper the session's cancelled-exit
// funnel (recordCanceled) calls at turn death, so undelivered steering
// resolves cancelled-normal and can never zombie-deliver into the NEXT turn's
// window. Returns 0 on an empty (or nil) queue.
func (q *SteerQueue) CancelAll() int {
	if q == nil {
		return 0
	}

	q.mu.Lock()
	highest := q.next
	q.mu.Unlock()

	return q.CancelThrough(highest)
}

// Pending reports the count of undelivered, un-cancelled items (23-02's
// queued/parked note surface reads it; inspection only — never a delivery
// path).
func (q *SteerQueue) Pending() int {
	if q == nil {
		return 0
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.items)
}
