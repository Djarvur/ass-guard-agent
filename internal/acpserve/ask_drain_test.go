package acpserve //nolint:testpackage // drives the queue + REAL surface composition (unexported fire seam via SetFire)

import (
	"context"
	"encoding/json"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// The 17-03 D-13 drain battery at the composition level: the session ask
// queue's DrainTurn/DrainAll resolving a REAL registry-backed
// session/request_permission ask — the turn-death cascade ($/cancel_request
// observed on the wire, exactly once) and the shutdown drain (zero goroutine
// leaks, no writes after the registry closed).

// drainSink is the permAskSink shape plus a post-close write tripwire.
type drainSink struct {
	permAskSink

	mu        sync.Mutex
	closed    bool
	afterGood int
}

func (s *drainSink) Write(m *acp.Message) error {
	s.mu.Lock()

	closed := s.closed

	if closed {
		s.afterGood++
	}
	s.mu.Unlock()

	if closed {
		return nil
	}

	return s.permAskSink.Write(m)
}

func (s *drainSink) markClosed() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
}

func (s *drainSink) postCloseWrites() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.afterGood
}

// waitForCascade reads frames until the $/cancel_request cascade for the
// given request id arrives.
func waitForCascade(t *testing.T, sink *drainSink, reqID string) *acp.Message {
	t.Helper()

	deadline := time.After(2 * time.Second)

	for {
		select {
		case m := <-sink.got:
			if m.Method == "$/cancel_request" {
				var p struct {
					RequestID string `json:"requestId"` //nolint:tagliatelle // ACP wire field
				}

				uerr := json.Unmarshal(m.Params, &p)
				if uerr != nil {
					t.Fatalf("decode cascade params: %v", uerr)
				}

				if p.RequestID != reqID {
					t.Fatalf("cascade requestId = %q; want %q", p.RequestID, reqID)
				}

				return m
			}
		case <-deadline:
			t.Fatalf("no $/cancel_request cascade for %s within 2s", reqID)
		}
	}
}

// drainAskEntry wires a REAL PermissionAsk as one queue entry's fire (the
// production shape: the queue promotes and calls the surface round-trip).
func drainAskEntry(pa *PermissionAsk) *session.AskEntry {
	e := gateAskEntry()
	e.SetFire(func(ctx context.Context) session.AskOutcome {
		return pa.Fire(ctx, e)
	})

	return e
}

// TestPermissionAskDrainCascade pins the criterion-2 cascade: turn death
// (DrainTurn) resolves the OPEN registry-backed dialog cancelled AND exactly
// one $/cancel_request for the ask's request id rides the wire.
func TestPermissionAskDrainCascade(t *testing.T) {
	t.Parallel()

	sink := &drainSink{permAskSink: permAskSink{got: make(chan *acp.Message, 8)}}
	reg := acp.NewRegistry(sink, io.Discard,
		acp.WithRegistryCascade(func(m *acp.Message) error { return sink.Write(m) }))
	pa := NewPermissionAsk(context.Background(), reg, io.Discard)

	q := session.NewAskQueue()

	resolved := make(chan session.AskOutcome, 1)

	resolve := func(_ *session.AskEntry, o session.AskOutcome) { resolved <- o }

	q.Enqueue(drainAskEntry(pa), resolve)

	req := waitForRequest(t, &sink.permAskSink)
	reqID := acp.NormalizeRequestID(req.ID)

	// Turn death: the scoped drain cancels the queue-owned fire ctx; the
	// registry resolves the entry cancelled and emits the D-19 cascade.
	q.DrainTurn("sess-1-turn-001")

	waitForCascade(t, sink, reqID)

	select {
	case o := <-resolved:
		if !o.Cancelled {
			t.Fatalf("outcome = %+v; want the cancelled family (never an error)", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the ask never resolved after the drain")
	}

	select {
	case m := <-sink.got:
		if m.Method == "$/cancel_request" {
			t.Errorf("second $/cancel_request observed — the cascade must fire exactly once")
		}
	case <-time.After(50 * time.Millisecond):
	}
}

// TestAskQueueShutdownDrain pins the serve-shutdown drain semantics (T-17-09):
// the full drain resolves everything cancelled, drained queued asks never
// fire, the pump goroutine exits (baseline+settle idiom), and no write is
// attempted after the registry closed (Pitfall 8).
func TestAskQueueShutdownDrain(t *testing.T) { //nolint:funlen,paralleltest // full chain; not parallel (goroutines)
	baseline := runtime.NumGoroutine()

	sink := &drainSink{permAskSink: permAskSink{got: make(chan *acp.Message, 8)}}
	reg := acp.NewRegistry(sink, io.Discard,
		acp.WithRegistryCascade(func(m *acp.Message) error { return sink.Write(m) }))
	pa := NewPermissionAsk(context.Background(), reg, io.Discard)

	q := session.NewAskQueue()

	resolved := make(chan session.AskOutcome, 2)

	resolve := func(_ *session.AskEntry, o session.AskOutcome) { resolved <- o }

	q.Enqueue(drainAskEntry(pa), resolve)

	queuedFires := 0 // touched only via the atomic below (pump goroutine visibility)

	fireMu := sync.Mutex{}

	queued := gateAskEntry()
	queued.TurnID = "sess-1-turn-002"
	queued.SetFire(func(_ context.Context) session.AskOutcome {
		fireMu.Lock()
		queuedFires++
		fireMu.Unlock()

		return session.AskOutcome{Selected: acp.PermOptionAllowOnce}
	})

	q.Enqueue(queued, func(_ *session.AskEntry, o session.AskOutcome) { resolved <- o })

	waitForRequest(t, &sink.permAskSink) // the open ask is on the wire

	q.DrainAll()

	for range 2 {
		select {
		case o := <-resolved:
			if !o.Cancelled {
				t.Fatalf("shutdown outcome = %+v; want cancelled", o)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("an ask never resolved during the shutdown drain")
		}
	}

	fireMu.Lock()
	got := queuedFires
	fireMu.Unlock()

	if got != 0 {
		t.Errorf("the drained queued ask fired %d time(s); want 0", got)
	}

	// The registry closes AFTER the drain (the Serve teardown order) — no
	// write may follow.
	reg.Stop()
	sink.markClosed()

	if got := sink.postCloseWrites(); got != 0 {
		t.Errorf("%d write(s) observed after the registry closed", got)
	}

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
