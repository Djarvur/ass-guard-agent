package session //nolint:testpackage // internal package test

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// TestSteerQueueTicketsMonotonic pins D-02's arrival substrate: Enqueue
// returns strictly increasing tickets across calls (the cancel protocol's
// cutoff key needs a total order).
func TestSteerQueueTicketsMonotonic(t *testing.T) {
	t.Parallel()

	q := NewSteerQueue()

	prev := uint64(0)

	for i := 0; i < 10; i++ {
		tk := q.Enqueue("input")
		if tk <= prev {
			t.Fatalf("ticket %d not increasing (prev %d) at enqueue %d", tk, prev, i+1)
		}

		prev = tk
	}
}

// TestSteerQueueDrainArrivalOrderAndWatermark pins the D-02 coalescing
// contract: Drain returns queued items in arrival order, and the watermark
// advances — a second Drain returns an empty batch (no re-delivery), and a
// Drain on a fully-drained/empty queue is a no-op (the every-iteration hot
// path). Ordering is asserted through the seamEventLog discipline (no sleeps).
func TestSteerQueueDrainArrivalOrderAndWatermark(t *testing.T) {
	t.Parallel()

	q := NewSteerQueue()
	log := &seamEventLog{}

	for _, txt := range []string{"first", "second", "third"} {
		log.add("enqueue:" + txt)
		q.Enqueue(txt)
	}

	batch := q.Drain()
	if len(batch) != 3 {
		t.Fatalf("first Drain returned %d items; want 3", len(batch))
	}

	for i, want := range []string{"first", "second", "third"} {
		if batch[i].Text != want {
			t.Errorf("batch[%d].Text = %q; want %q (arrival order)", i, batch[i].Text, want)
		}

		log.add("drained:" + batch[i].Text)
	}

	events := log.snapshot()
	// Every enqueue precedes its drain, and drains preserve enqueue order.
	wantSeq := []string{
		"enqueue:first", "enqueue:second", "enqueue:third",
		"drained:first", "drained:second", "drained:third",
	}

	if len(events) != len(wantSeq) {
		t.Fatalf("event log = %v; want %v", events, wantSeq)
	}

	for i := range wantSeq {
		if events[i] != wantSeq[i] {
			t.Errorf("event[%d] = %q; want %q", i, events[i], wantSeq[i])
		}
	}

	// Watermark advanced: second Drain is empty (nil batch, no error).
	if again := q.Drain(); len(again) != 0 {
		t.Errorf("second Drain returned %d items; want 0 (watermark not advanced)", len(again))
	}

	// Enqueues after a drain continue the ticket sequence.
	if tk := q.Enqueue("fourth"); tk != 4 {
		t.Errorf("post-drain ticket = %d; want 4", tk)
	}
}

// TestSteerQueueDrainEmptyNoop pins the hot-path contract: draining an empty
// (and a nil) queue returns an empty batch without error.
func TestSteerQueueDrainEmptyNoop(t *testing.T) {
	t.Parallel()

	q := NewSteerQueue()
	if b := q.Drain(); len(b) != 0 {
		t.Errorf("empty-queue Drain returned %d items; want 0", len(b))
	}

	var nilQ *SteerQueue
	if b := nilQ.Drain(); len(b) != 0 {
		t.Errorf("nil-queue Drain returned %d items; want 0", len(b))
	}
}

// TestSteerQueueCancelThrough pins the ticket/cutoff protocol
// (PITFALLS.md:329): CancelThrough(c) removes exactly the items with
// Ticket <= c and returns their count; items with Ticket > c survive and are
// returned by the next Drain.
func TestSteerQueueCancelThrough(t *testing.T) {
	t.Parallel()

	q := NewSteerQueue()
	q.Enqueue("one")   // 1
	q.Enqueue("two")   // 2
	q.Enqueue("three") // 3
	q.Enqueue("four")  // 4

	if n := q.CancelThrough(2); n != 2 {
		t.Fatalf("CancelThrough(2) = %d; want 2", n)
	}

	batch := q.Drain()
	if len(batch) != 2 || batch[0].Text != "three" || batch[1].Text != "four" {
		t.Errorf("post-cancel Drain = %+v; want the two survivors (three, four)", batch)
	}

	// Cancelling an already-empty span is 0.
	if n := q.CancelThrough(99); n != 0 {
		t.Errorf("CancelThrough on empty queue = %d; want 0", n)
	}
}

// TestSteerQueueCancelAll pins the turn-death wrapper: CancelAll resolves
// every remaining item (through the highest assigned ticket) and returns the
// count; on an empty queue it returns 0.
func TestSteerQueueCancelAll(t *testing.T) {
	t.Parallel()

	q := NewSteerQueue()
	q.Enqueue("a")
	q.Enqueue("b")

	q.Drain() // deliver ticket 1

	q.Enqueue("c")
	q.Enqueue("d")

	if n := q.CancelAll(); n != 2 {
		t.Fatalf("CancelAll = %d; want 2 (only the undelivered items)", n)
	}

	if p := q.Pending(); p != 0 {
		t.Errorf("Pending() after CancelAll = %d; want 0", p)
	}

	if n := q.CancelAll(); n != 0 {
		t.Errorf("CancelAll on empty queue = %d; want 0", n)
	}
}

// TestSteerQueuePending pins the inspection count: undelivered,
// un-cancelled items only.
func TestSteerQueuePending(t *testing.T) {
	t.Parallel()

	q := NewSteerQueue()

	if p := q.Pending(); p != 0 {
		t.Fatalf("Pending() on empty = %d; want 0", p)
	}

	q.Enqueue("x")
	q.Enqueue("y")

	if p := q.Pending(); p != 2 {
		t.Fatalf("Pending() = %d; want 2", p)
	}

	q.CancelThrough(1)

	if p := q.Pending(); p != 1 {
		t.Errorf("Pending() after cancel = %d; want 1", p)
	}

	q.Drain()

	if p := q.Pending(); p != 0 {
		t.Errorf("Pending() after drain = %d; want 0", p)
	}
}

// TestSteerQueueHammerConservation pins the milestone -race prescription
// (PITFALLS.md:329): 8 producer goroutines Enqueue continuously while the
// test issues CancelThrough at advancing cutoffs, then a final Drain — the
// union of delivered + cancelled tickets equals exactly the set of assigned
// tickets, with no duplicates and no losses.
func TestSteerQueueHammerConservation(t *testing.T) {
	t.Parallel()

	const producers = 8
	const perProducer = 200

	q := NewSteerQueue()

	assigned := make([][]uint64, producers)

	var wg sync.WaitGroup

	for g := range producers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := 0; i < perProducer; i++ {
				tk := q.Enqueue("hammer")
				assigned[g] = append(assigned[g], tk)
			}
		}()
	}

	// Interleave cutoffs while producers run (deterministic accounting: the
	// sum of CancelThrough returns + the final Drain must equal the assigned
	// set exactly — whatever interleaving the scheduler picks).
	cancelled := 0

	for round := 0; round < 10; round++ {
		// Cutoffs advance monotonically so the cancelled sets never overlap.
		cancelled += q.CancelThrough(uint64(round * producers * 20))
	}

	wg.Wait()

	// Anything left after the producers finish is delivered by the final drain.
	delivered := q.Drain()

	// Ticket conservation: assigned set == delivered ∪ cancelled, no dups.
	assignedSet := make(map[uint64]bool, producers*perProducer)

	for _, tickets := range assigned {
		for _, tk := range tickets {
			if tk == 0 {
				t.Fatal("ticket 0 assigned — must never exist")
			}

			if assignedSet[tk] {
				t.Fatalf("ticket %d assigned twice", tk)
			}

			assignedSet[tk] = true
		}
	}

	seen := make(map[uint64]bool, len(delivered)+cancelled)

	for _, it := range delivered {
		if !assignedSet[it.Ticket] {
			t.Fatalf("delivered ticket %d was never assigned", it.Ticket)
		}

		if seen[it.Ticket] {
			t.Fatalf("ticket %d delivered twice", it.Ticket)
		}

		seen[it.Ticket] = true
	}

	// delivered + cancelled == assigned, exactly (every assigned ticket
	// resolved exactly once — the anti-Pitfall-3 conservation invariant).
	if got := len(delivered) + cancelled; got != len(assignedSet) {
		t.Fatalf("delivered(%d) + cancelled(%d) = %d; want %d (assigned)",
			len(delivered), cancelled, got, len(assignedSet))
	}

	if q.Pending() != 0 {
		t.Fatalf("Pending() after final drain = %d; want 0", q.Pending())
	}
}

// TestSteerQueueNoACP pins transport-neutrality at the compile level (TG-02's
// consumer contract): steerqueue.go's import block contains zero ACP-wire
// package references. Parse-level (go/parser), not a grep — the file must
// stay consumable by a non-ACP frontend by construction.
func TestSteerQueueNoACP(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, "steerqueue.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse steerqueue.go: %v", err)
	}

	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("unquote import %s: %v", imp.Path.Value, err)
		}

		if strings.Contains(path, "/internal/acp") {
			t.Errorf("steerqueue.go imports ACP-wire package %q — transport-neutrality violated", path)
		}
	}
}
