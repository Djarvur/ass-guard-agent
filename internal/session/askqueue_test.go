package session //nolint:testpackage // internal package test (drives the queue's unexported fire seam)

import (
	"context"
	"sync"
	"testing"
	"time"
)

// The 17-03 ask-queue battery (D-11/D-12): ONE outstanding fired ask at a
// time, foreground-before-background priority (FIFO within a class), the
// visible enqueue notes + counter, and the -race concurrency invariants.
// The turn-death drain families (D-13) live in the TestAskQueueDrain* and
// TestGateTurnDeath tests.

// queueVocabulary pins the test-local literals (goconst).
const (
	queueTurn1  = "turn-1"
	queueTurn2  = "turn-2"
	queueOption = "allow_once"
)

// qRecorder collects fire/resolve events from the queue's pump goroutine.
// Every accessor takes the lock — the pump goroutine and the test goroutine
// are distinct (the -race battery depends on this being honest).
type qRecorder struct {
	mu       sync.Mutex
	fired    []uint64 // entries' Seq in fire order
	resolved []uint64
}

func (r *qRecorder) noteFire(seq uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fired = append(r.fired, seq)
}

func (r *qRecorder) noteResolve(seq uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.resolved = append(r.resolved, seq)
}

func (r *qRecorder) fireCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.fired)
}

func (r *qRecorder) resolveCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.resolved)
}

func (r *qRecorder) fireOrder() []uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]uint64(nil), r.fired...)
}

// qBlockedFire returns a fire func that records its start, blocks until the
// test releases it (or the queue's drain cancels the fire ctx — returning the
// cancelled family), and then reports a selected outcome.
func qBlockedFire(rec *qRecorder, release <-chan struct{}) func(context.Context) AskOutcome {
	return func(ctx context.Context) AskOutcome {
		rec.noteFire(0) // presence only; order assertions use per-entry recorders

		if ctx == nil {
			panic("queue must hand every fire a cancellable ctx (the D-13 drain seam)")
		}

		select {
		case <-release:
			return AskOutcome{Selected: queueOption}
		case <-ctx.Done():
			return AskOutcome{Cancelled: true}
		}
	}
}

// TestAskQueueSerialization pins D-11's one-modal invariant: while entry A is
// fired (unresolved), a second enqueue B is QUEUED, not fired; resolving A
// fires B.
func TestAskQueueSerialization(t *testing.T) {
	t.Parallel()

	q := NewAskQueue()
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	rec := &qRecorder{}

	entryA := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
	entryA.fire = func(ctx context.Context) AskOutcome {
		started <- struct{}{}
		rec.noteFire(entryA.Seq)

		select {
		case <-release:
			return AskOutcome{Selected: queueOption}
		case <-ctx.Done():
			return AskOutcome{Cancelled: true}
		}
	}

	resolvedA := make(chan AskOutcome, 1)
	q.Enqueue(entryA, func(_ *AskEntry, o AskOutcome) {
		rec.noteResolve(entryA.Seq)
		resolvedA <- o
	})

	<-started // A fired at once (the queue was idle)

	// B enqueues while A is open: it must NOT fire.
	entryB := &AskEntry{TurnID: queueTurn2, Class: AskClassBackground}
	entryB.fire = func(_ context.Context) AskOutcome {
		rec.noteFire(entryB.Seq)

		return AskOutcome{Selected: queueOption}
	}

	resolvedB := make(chan AskOutcome, 1)
	q.Enqueue(entryB, func(_ *AskEntry, o AskOutcome) {
		rec.noteResolve(entryB.Seq)
		resolvedB <- o
	})

	select {
	case <-started:
		t.Fatal("B fired while A was still open — stacked modals (D-11 violated)")
	case <-time.After(20 * time.Millisecond):
	}

	if got := q.Pending(); got != 1 {
		t.Fatalf("Pending = %d; want 1 (B queued behind the open A)", got)
	}

	close(release)

	if o := <-resolvedA; o.Selected != queueOption {
		t.Fatalf("A outcome = %+v; want selected", o)
	}

	select {
	case o := <-resolvedB:
		if o.Selected != queueOption {
			t.Fatalf("B outcome = %+v; want selected", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("B never fired after A resolved")
	}

	if got := rec.fireCount(); got != 2 {
		t.Errorf("total fires = %d; want 2 (no double-fire, no lost entry)", got)
	}

	if got := rec.resolveCount(); got != 2 {
		t.Errorf("total resolves = %d; want 2", got)
	}

	if got := q.Pending(); got != 0 {
		t.Errorf("Pending = %d; want 0 after both resolved", got)
	}
}

// TestAskQueuePriority pins D-11's ordering discipline: a foreground enqueue
// preempts the WAITING queue head (a background entry waiting fires after the
// later foreground entry), two same-class entries fire in enqueue order, and
// an OPEN entry is never preempted regardless of class.
func TestAskQueuePriority(t *testing.T) {
	t.Parallel()

	t.Run("foreground preempts the waiting background head", func(t *testing.T) {
		t.Parallel()

		q := NewAskQueue()
		release := make(chan struct{})
		order := make(chan string, 8)

		// A (fg) is open and blocked.
		entryA := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
		entryA.fire = func(ctx context.Context) AskOutcome {
			order <- "A-open"

			select {
			case <-release:
			case <-ctx.Done():
			}

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryA, nil)

		<-order // A opened

		// B (bg) waits, then C (fg) arrives LATER — C must fire before B.
		entryB := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
		entryB.fire = func(_ context.Context) AskOutcome {
			order <- "B"

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryB, nil)

		entryC := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
		entryC.fire = func(_ context.Context) AskOutcome {
			order <- "C"

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryC, nil)

		close(release)

		gateWaitFor(t, func() bool { return len(order) == 3 })

		close(order)

		got := make([]string, 0, 3)
		for v := range order {
			got = append(got, v)
		}

		if got[0] != "A-open" || got[1] != "C" || got[2] != "B" {
			t.Fatalf("fire order = %v; want [A-open C B] (fg preempts the waiting bg head)", got)
		}
	})

	t.Run("an open entry is never preempted regardless of class", func(t *testing.T) {
		t.Parallel()

		q := NewAskQueue()
		release := make(chan struct{})
		started := make(chan string, 8)

		// A (bg) is OPEN — a later fg enqueue must queue behind it.
		entryA := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
		entryA.fire = func(ctx context.Context) AskOutcome {
			started <- "A"

			select {
			case <-release:
			case <-ctx.Done():
			}

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryA, nil)

		<-started

		entryB := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
		entryB.fire = func(_ context.Context) AskOutcome {
			started <- "B"

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryB, nil)

		select {
		case <-started:
			t.Fatal("the open background entry was preempted by a foreground enqueue")
		case <-time.After(20 * time.Millisecond):
		}

		close(release)

		gateWaitFor(t, func() bool { return len(started) == 2 })

		if <-started != "B" {
			t.Fatal("B should fire first after A resolved")
		}
	})

	t.Run("same class fires in enqueue order", func(t *testing.T) {
		t.Parallel()

		q := NewAskQueue()
		release := make(chan struct{})
		order := make(chan string, 8)

		entryA := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
		entryA.fire = func(ctx context.Context) AskOutcome {
			order <- "A"

			select {
			case <-release:
			case <-ctx.Done():
			}

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryA, nil)

		<-order

		for _, name := range []string{"B", "C"} {
			e := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
			e.fire = func(_ context.Context) AskOutcome {
				order <- name

				return AskOutcome{Selected: queueOption}
			}
			q.Enqueue(e, nil)
		}

		close(release)

		gateWaitFor(t, func() bool { return len(order) == 3 })

		close(order)

		got := make([]string, 0, 3)
		for v := range order {
			got = append(got, v)
		}

		if got[0] != "A" || got[1] != "B" || got[2] != "C" {
			t.Fatalf("fire order = %v; want [A B C] (FIFO within a class)", got)
		}
	})
}

// TestAskQueueNotes pins D-12's visibility discipline: an enqueue that fires
// immediately emits NO note; an enqueue that queues emits exactly one note
// reading "ask queued — N pending" (N including itself); the counter
// increments on EVERY enqueue (immediate included).
func TestAskQueueNotes(t *testing.T) {
	t.Parallel()

	q := NewAskQueue()

	notes := make(chan string, 8)
	q.SetNoteEmitter(func(_ *AskEntry, note string) { notes <- note })

	release := make(chan struct{})
	started := make(chan struct{}, 8)

	entryA := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
	entryA.fire = func(ctx context.Context) AskOutcome {
		started <- struct{}{}

		select {
		case <-release:
		case <-ctx.Done():
		}

		return AskOutcome{Selected: queueOption}
	}
	q.Enqueue(entryA, nil) // fires immediately — no note

	<-started

	entryB := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
	entryB.fire = func(_ context.Context) AskOutcome {
		return AskOutcome{Selected: queueOption}
	}
	q.Enqueue(entryB, nil) // queues behind A — one note

	entryC := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
	entryC.fire = func(_ context.Context) AskOutcome {
		return AskOutcome{Selected: queueOption}
	}
	q.Enqueue(entryC, nil) // queues behind B — one note

	select {
	case n := <-notes:
		t.Fatalf("immediate-fire enqueue emitted a note %q (spam must be dead by construction)", n)
	case <-time.After(20 * time.Millisecond):
	}

	if got := <-notes; got != "ask queued — 1 pending" {
		t.Errorf("first queued note = %q; want %q", got, "ask queued — 1 pending")
	}

	if got := <-notes; got != "ask queued — 2 pending" {
		t.Errorf("second queued note = %q; want %q", got, "ask queued — 2 pending")
	}

	if got := q.Enqueued(); got != 3 {
		t.Errorf("Enqueued counter = %d; want 3 (every enqueue counts)", got)
	}

	close(release)
}

// TestAskQueueNoteNoSubscriber pins the 13-03 hazard discipline: the note is
// emitted IN-HAND through the injected (subscriber-backed) emitter at enqueue
// time — a queue with no active turn machinery around it still delivers the
// note synchronously; nothing depends on a post-turn publish being observed.
func TestAskQueueNoteNoSubscriber(t *testing.T) {
	t.Parallel()

	q := NewAskQueue()

	got := make(chan string, 1)
	q.SetNoteEmitter(func(_ *AskEntry, note string) { got <- note })

	block := make(chan struct{})
	entry := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
	entry.fire = func(ctx context.Context) AskOutcome {
		select {
		case <-block:
		case <-ctx.Done():
		}

		return AskOutcome{Selected: queueOption}
	}
	q.Enqueue(entry, nil)

	// A second enqueue with NO turn subscriber anywhere: the note still lands.
	queued := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
	queued.fire = func(_ context.Context) AskOutcome {
		return AskOutcome{Selected: queueOption}
	}
	q.Enqueue(queued, nil)

	select {
	case note := <-got:
		if note != "ask queued — 1 pending" {
			t.Errorf("note = %q; want %q", note, "ask queued — 1 pending")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no note without a turn subscriber — the 13-03 timing hazard")
	}

	close(block)
}

// TestAskQueueConcurrency runs 60 interleaved enqueues + resolutions across
// producer goroutines under -race: one-outstanding holds (never two fires
// in flight), FIFO-within-class holds (per-class fire order ascending by
// Seq), and no entry is double-fired or lost.
func TestAskQueueConcurrency(t *testing.T) {
	t.Parallel()

	const (
		producers = 8
		perProd   = 8 // 64 interleaved operations (>= 50 required)
	)

	q := NewAskQueue()

	var (
		mu          sync.Mutex
		inFlight    int
		maxInFlight int
		fireOrder   = map[AskClass][]uint64{
			AskClassForeground: {},
			AskClassBackground: {},
		}
		resolved int
	)

	recordFire := func(class AskClass, seq uint64) {
		mu.Lock()
		defer mu.Unlock()

		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}

		fireOrder[class] = append(fireOrder[class], seq)
	}

	recordResolve := func() {
		mu.Lock()
		defer mu.Unlock()

		resolved++
		inFlight--
	}

	for p := range producers {
		for k := range perProd {
			class := AskClassForeground
			if (p+k)%3 == 0 {
				class = AskClassBackground
			}

			e := &AskEntry{TurnID: queueTurn1, Class: class}
			e.fire = func(_ context.Context) AskOutcome {
				recordFire(class, e.Seq)

				// Yield so producer goroutines genuinely interleave with the
				// pump's fire/resolve loop.
				time.Sleep(time.Millisecond)

				return AskOutcome{Selected: queueOption}
			}
			q.Enqueue(e, func(*AskEntry, AskOutcome) { recordResolve() })
		}
	}

	total := producers * perProd

	gateWaitFor(t, func() bool { return q.Enqueued() == uint64(total) })
	gateWaitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()

		return resolved == total
	})

	mu.Lock()
	defer mu.Unlock()

	if maxInFlight != 1 {
		t.Errorf("max in-flight fires = %d; want exactly 1 (one-outstanding, D-11)", maxInFlight)
	}

	for class, seqs := range fireOrder {
		if len(seqs) == 0 {
			continue
		}

		for i := 1; i < len(seqs); i++ {
			if seqs[i] <= seqs[i-1] {
				t.Fatalf("class %d fire order = %v; seqs must ascend (FIFO within class)", class, seqs)
			}
		}
	}

	firedTotal := len(fireOrder[AskClassForeground]) + len(fireOrder[AskClassBackground])
	if firedTotal != total {
		t.Errorf("total fires = %d; want %d (no double-fire, no lost entry)", firedTotal, total)
	}

	if got := q.Pending(); got != 0 {
		t.Errorf("Pending = %d; want 0", got)
	}
}
