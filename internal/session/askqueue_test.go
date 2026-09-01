package session //nolint:testpackage // internal package test (drives the queue's unexported fire seam)

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
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

// qNames is a mutex-guarded string order recorder (a len()-poll on a channel
// does not synchronize with its senders — the memory-model-honest shape).
type qNames struct {
	mu    sync.Mutex
	names []string
}

func (n *qNames) add(name string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.names = append(n.names, name)
}

func (n *qNames) len() int {
	n.mu.Lock()
	defer n.mu.Unlock()

	return len(n.names)
}

func (n *qNames) all() []string {
	n.mu.Lock()
	defer n.mu.Unlock()

	return append([]string(nil), n.names...)
}

// TestAskQueueSerialization pins D-11's one-modal invariant: while entry A is
// fired (unresolved), a second enqueue B is QUEUED, not fired; resolving A
// fires B.
func TestAskQueueSerialization(t *testing.T) { //nolint:funlen // flat serialization battery
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
func TestAskQueuePriority(t *testing.T) { //nolint:cyclop,funlen // three ordering subtests in one family
	t.Parallel()

	t.Run("foreground preempts the waiting background head", func(t *testing.T) {
		t.Parallel()

		q := NewAskQueue()
		release := make(chan struct{})
		order := &qNames{}

		// A (fg) is open and blocked.
		entryA := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
		entryA.fire = func(ctx context.Context) AskOutcome {
			order.add("A-open")

			select {
			case <-release:
			case <-ctx.Done():
			}

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryA, nil)

		gateWaitFor(t, func() bool { return order.len() == 1 }) // A opened

		// B (bg) waits, then C (fg) arrives LATER — C must fire before B.
		entryB := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
		entryB.fire = func(_ context.Context) AskOutcome {
			order.add("B")

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryB, nil)

		entryC := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
		entryC.fire = func(_ context.Context) AskOutcome {
			order.add("C")

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryC, nil)

		close(release)

		gateWaitFor(t, func() bool { return order.len() == 3 })

		if got := order.all(); got[0] != "A-open" || got[1] != "C" || got[2] != "B" {
			t.Fatalf("fire order = %v; want [A-open C B] (fg preempts the waiting bg head)", got)
		}
	})

	t.Run("an open entry is never preempted regardless of class", func(t *testing.T) {
		t.Parallel()

		q := NewAskQueue()
		release := make(chan struct{})
		order := &qNames{}

		// A (bg) is OPEN — a later fg enqueue must queue behind it.
		entryA := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
		entryA.fire = func(ctx context.Context) AskOutcome {
			order.add("A")

			select {
			case <-release:
			case <-ctx.Done():
			}

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryA, nil)

		gateWaitFor(t, func() bool { return order.len() == 1 })

		entryB := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
		entryB.fire = func(_ context.Context) AskOutcome {
			order.add("B")

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryB, nil)

		time.Sleep(20 * time.Millisecond)

		if got := order.len(); got != 1 {
			t.Fatalf("fires while A is open = %d; want 1 (the open entry is never preempted)", got)
		}

		close(release)

		gateWaitFor(t, func() bool { return order.len() == 2 })

		if got := order.all(); got[1] != "B" {
			t.Fatalf("fire order = %v; B should fire first after A resolved", got)
		}
	})

	t.Run("same class fires in enqueue order", func(t *testing.T) {
		t.Parallel()

		q := NewAskQueue()
		release := make(chan struct{})
		order := &qNames{}

		entryA := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
		entryA.fire = func(ctx context.Context) AskOutcome {
			order.add("A")

			select {
			case <-release:
			case <-ctx.Done():
			}

			return AskOutcome{Selected: queueOption}
		}
		q.Enqueue(entryA, nil)

		gateWaitFor(t, func() bool { return order.len() == 1 })

		for _, name := range []string{"B", "C"} {
			e := &AskEntry{TurnID: queueTurn1, Class: AskClassBackground}
			e.fire = func(_ context.Context) AskOutcome {
				order.add(name)

				return AskOutcome{Selected: queueOption}
			}
			q.Enqueue(e, nil)
		}

		close(release)

		gateWaitFor(t, func() bool { return order.len() == 3 })

		got := order.all()
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

	// The two QUEUED enqueues emitted their notes; the immediate fire emitted
	// none — exactly two notes total, never a third (spam dead by
	// construction).
	if got := <-notes; got != "ask queued — 1 pending" {
		t.Errorf("first queued note = %q; want %q", got, "ask queued — 1 pending")
	}

	if got := <-notes; got != "ask queued — 2 pending" {
		t.Errorf("second queued note = %q; want %q", got, "ask queued — 2 pending")
	}

	select {
	case n := <-notes:
		t.Fatalf("unexpected extra note %q (the immediate fire must emit none)", n)
	case <-time.After(20 * time.Millisecond):
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
func TestAskQueueConcurrency(t *testing.T) { //nolint:funlen // the interleaved producer battery
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

// ─────────────────────────────────────────────────────────────────────────────
// D-13: the turn-scoped drain (Task 2). DrainTurn resolves the OPEN dialog of
// one turn (through the queue-owned fire-ctx cancellation — a registry-backed
// fire cascades $/cancel_request) and drains that turn's queued-but-unfired
// asks as cancelled-normal WITHOUT firing. Entries of other turns are
// untouched.

// qAtomic is a tiny atomic int counter for fire-invocation proofs.
type qAtomic struct {
	mu sync.Mutex
	v  int
}

func (a *qAtomic) add() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.v++
}

func (a *qAtomic) get() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.v
}

// TestAskQueueDrainTurn pins the D-13 drain semantics: the open entry of the
// dead turn resolves cancelled through its fire-ctx cancellation, the dead
// turn's queued entry drains cancelled-normal with ZERO fires (the zombie-ask
// ban), and another turn's waiting entry is untouched.
func TestAskQueueDrainTurn(t *testing.T) { //nolint:funlen,cyclop // the full scoped-drain chain
	t.Parallel()

	q := NewAskQueue()
	release := make(chan struct{})

	// The open dialog of turn-1: its fire blocks until the drain cancels the
	// queue-owned ctx.
	openEntry := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
	openEntry.fire = func(ctx context.Context) AskOutcome {
		select {
		case <-release:
			return AskOutcome{Selected: queueOption}
		case <-ctx.Done():
			return AskOutcome{Cancelled: true}
		}
	}

	resolvedOpen := make(chan AskOutcome, 1)

	q.Enqueue(openEntry, func(_ *AskEntry, o AskOutcome) { resolvedOpen <- o })

	// A queued-but-unfired ask of the SAME (dead) turn: its fire must never
	// be invoked.
	queuedFires := &qAtomic{}

	queuedEntry := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
	queuedEntry.fire = func(_ context.Context) AskOutcome {
		queuedFires.add()

		return AskOutcome{Selected: queueOption}
	}

	resolvedQueued := make(chan AskOutcome, 1)

	q.Enqueue(queuedEntry, func(_ *AskEntry, o AskOutcome) { resolvedQueued <- o })

	// Another turn's ask: the turn-1 drain must leave it untouched.
	otherEntry := &AskEntry{TurnID: queueTurn2, Class: AskClassBackground}
	otherEntry.fire = func(ctx context.Context) AskOutcome {
		select {
		case <-release:
			return AskOutcome{Selected: queueOption}
		case <-ctx.Done():
			return AskOutcome{Cancelled: true}
		}
	}

	resolvedOther := make(chan AskOutcome, 1)

	q.Enqueue(otherEntry, func(_ *AskEntry, o AskOutcome) { resolvedOther <- o })

	gateWaitFor(t, func() bool { return q.Pending() == 2 })

	// Turn death for turn-1.
	q.DrainTurn(queueTurn1)

	select {
	case o := <-resolvedOpen:
		if !o.Cancelled {
			t.Fatalf("open entry outcome = %+v; want cancelled (the drain cancels the fire ctx)", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the open entry never resolved after the drain")
	}

	select {
	case o := <-resolvedQueued:
		if !o.Cancelled {
			t.Fatalf("queued entry outcome = %+v; want cancelled-normal", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the queued entry never drained")
	}

	if got := queuedFires.get(); got != 0 {
		t.Fatalf("the drained queued entry fired %d time(s) — zombie ask (D-13 ban)", got)
	}

	// The other turn's entry was NOT resolved by the drain (a broken scoping
	// would have delivered Cancelled here). The pump may legitimately promote
	// it once the open slot frees — its fire blocks until release, so the
	// outcome stays pending either way.
	select {
	case o := <-resolvedOther:
		t.Fatalf("the other turn's entry resolved during the drain: %+v (scoping broken)", o)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)

	select {
	case o := <-resolvedOther:
		if o.Cancelled || o.Selected != queueOption {
			t.Fatalf("other turn's outcome = %+v; want selected (it survived and fired normally)", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the other turn's entry never fired after the drain")
	}
}

// TestAskQueueDrainAll pins the session/serve-scope drain (the same
// drain-one-turn core applied to every turn present): everything resolves
// cancelled, drained queued entries never fire, and the pump goroutine exits
// (the goroutine-leak check follows the repo's baseline+settle idiom).
func TestAskQueueDrainAll(t *testing.T) { //nolint:funlen,paralleltest // full drain chain; not parallel (goroutines)
	baseline := runtime.NumGoroutine()

	q := NewAskQueue()

	openEntry := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
	openEntry.fire = func(ctx context.Context) AskOutcome {
		<-ctx.Done()

		return AskOutcome{Cancelled: true}
	}

	resolvedOpen := make(chan AskOutcome, 1)

	q.Enqueue(openEntry, func(_ *AskEntry, o AskOutcome) { resolvedOpen <- o })

	queuedFires := &qAtomic{}

	var queuedResolved sync.WaitGroup

	queuedResolved.Add(2)

	for _, turn := range []string{queueTurn1, queueTurn2} {
		e := &AskEntry{TurnID: turn, Class: AskClassBackground}
		e.fire = func(_ context.Context) AskOutcome {
			queuedFires.add()

			return AskOutcome{Selected: queueOption}
		}

		q.Enqueue(e, func(*AskEntry, AskOutcome) { queuedResolved.Done() })
	}

	gateWaitFor(t, func() bool { return q.Pending() == 2 })

	q.DrainAll()

	if o := <-resolvedOpen; !o.Cancelled {
		t.Fatalf("open entry outcome = %+v; want cancelled", o)
	}

	queuedResolved.Wait()

	if got := queuedFires.get(); got != 0 {
		t.Fatalf("drained queued entries fired %d time(s); want 0", got)
	}

	if got := q.Pending(); got != 0 {
		t.Errorf("Pending after the full drain = %d; want 0", got)
	}

	// The pump goroutine exits once the drained state settles (the repo's
	// emitter-soak idiom: baseline + settle window).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Errorf("goroutine leak: %d goroutines after the shutdown drain; baseline was %d",
		runtime.NumGoroutine(), baseline)
}

// TestAskQueueDrainResolvesAsync pins the CR-03 fix: the drain caller (the ACP
// reader goroutine on the session/cancel path) must NOT block on the drained
// entries' resolutions — each resolve is a full resumed model loop, and the
// reader stalling on it freezes the whole connection. The QUEUED entry's drain
// resolve runs on its own goroutine: the drain returns while the resolve is
// still running, and the resolve completes afterwards.
func TestAskQueueDrainResolvesAsync(t *testing.T) { //nolint:funlen,paralleltest // drain chain; baseline idiom
	baseline := runtime.NumGoroutine()

	q := NewAskQueue()

	// The OPEN dialog of the dead turn: resolves through the fire-ctx cancel
	// (the pump's normal completion path — unaffected by CR-03).
	openEntry := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
	openEntry.fire = func(ctx context.Context) AskOutcome {
		<-ctx.Done()

		return AskOutcome{Cancelled: true}
	}

	resolvedOpen := make(chan AskOutcome, 1)

	q.Enqueue(openEntry, func(_ *AskEntry, o AskOutcome) { resolvedOpen <- o })

	// The QUEUED-but-unfired ask of the same turn: its fire must never run,
	// and its resolve — a full resumed model loop — must not block the drain.
	queuedFires := &qAtomic{}

	resolveStarted := make(chan struct{})

	release := make(chan struct{})

	queuedEntry := &AskEntry{TurnID: queueTurn1, Class: AskClassForeground}
	queuedEntry.fire = func(_ context.Context) AskOutcome {
		queuedFires.add()

		return AskOutcome{Selected: queueOption}
	}

	q.Enqueue(queuedEntry, func(_ *AskEntry, _ AskOutcome) {
		close(resolveStarted)
		<-release
	})

	gateWaitFor(t, func() bool { return q.Pending() == 1 })

	drained := make(chan struct{})

	go func() {
		q.DrainTurn(queueTurn1)
		close(drained)
	}()

	// The drain must return even though the queued resolve is still blocked —
	// pre-fix, DrainTurn invoked the resolve synchronously and could not
	// return before release (the reader goroutine stalled for a whole model
	// turn).
	select {
	case <-resolveStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("the drained resolve never started")
	}

	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("DrainTurn blocked on the drained entry's resolution (CR-03: the reader goroutine stalls)")
	}

	close(release)

	// The open dialog resolved cancelled through the ctx cascade, and the
	// drained queued entry never fired (the zombie-ask ban).
	select {
	case o := <-resolvedOpen:
		if !o.Cancelled {
			t.Fatalf("open entry outcome = %+v; want cancelled", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the open entry never resolved after the drain")
	}

	if got := queuedFires.get(); got != 0 {
		t.Fatalf("the drained queued entry fired %d time(s) — zombie ask (D-13 ban)", got)
	}

	// Clean teardown: the resolve goroutine exits (the leak-check idiom).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Errorf("goroutine leak: %d goroutines after the drained resolve exited; baseline was %d",
		runtime.NumGoroutine(), baseline)
}

// TestGateTurnDeath pins criterion 2 end-to-end at the session level (D-13):
// a gated turn suspends with the dialog open (held), a SECOND prompt's gated
// ask queues behind it, and the teardown drain (the seam every teardown path
// reaches) — resolves the OPEN dialog cancelled via the fire-ctx cancellation
// (a registry-backed fire cascades $/cancel_request on the wire),
// — appends the cancelled-NORMAL result and resumes the SAME turn (no error
// result, no hang),
// — drains the queued-but-unfired ask of the other turn as cancelled-normal
// with ZERO additional surface fires (the firing monopoly + zombie-ask ban).
func TestGateTurnDeath(t *testing.T) { //nolint:funlen // the full death chain
	t.Parallel()

	store := &fakePermStore{}
	surf := &fakeGateSurface{hold: true}

	s := newGateSession(t, []provider.Response{
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: gateCall2, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
		{FinishReason: stopEndTurn}, // whichever resume runs first
		{FinishReason: stopEndTurn}, // ...and the second
	}, PermModeGated, store, surf)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("prompt 1: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop1 = %q; want the ask marker", stop)
	}

	gateWaitFor(t, func() bool { return surf.fired() == 1 })

	// A second prompt while the dialog is open: its gated call's ask QUEUES
	// (one outstanding — D-11) and the prompt suspends.
	stop2, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "second prompt"}})
	if err != nil {
		t.Fatalf("prompt 2 while dialog open: %v", err)
	}

	if stop2 != stopAsk {
		t.Fatalf("stop2 = %q; want the ask marker (the ask queued behind the open dialog)", stop2)
	}

	gateWaitFor(t, func() bool { return s.HasQueuedAsks() && s.HasOpenAsk() })

	// Teardown (the shared drain the cancel/close/shutdown paths reach).
	s.DrainPermissionAsks()

	// The OPEN dialog resolved cancelled: the cancelled-NORMAL result is
	// appended (NEVER an error result — criterion 2's letter) and the turn
	// resumes.
	gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

	r1 := toolResultsFor(t, s, gateCall1)
	if len(r1) != 1 || r1[0].IsError {
		t.Fatalf("drained open-dialog result = %+v; want exactly one NON-error (cancelled-normal) result", r1)
	}

	if !strings.Contains(string(r1[0].Output), "cancelled") {
		t.Errorf("result output = %s; want the cancelled-normal form", r1[0].Output)
	}

	// The queued ask drained as cancelled-normal too — appended, never fired.
	gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall2)) == 1 })

	r2 := toolResultsFor(t, s, gateCall2)
	if len(r2) != 1 || r2[0].IsError {
		t.Fatalf("drained queued result = %+v; want exactly one NON-error (cancelled-normal) result", r2)
	}

	// The firing monopoly: through the WHOLE lifecycle (suspend + queued ask
	// + drain) the surface fired exactly once — zero ask-method calls outside
	// the queue's fire step.
	if got := surf.fired(); got != 1 {
		t.Errorf("surface fired %d time(s); want exactly 1 (firing monopoly)", got)
	}

	if order := store.snapshotOrder(); len(order) != 0 {
		t.Errorf("dead turns executed gated calls: %v", order)
	}
}
