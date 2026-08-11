package event_test

import (
	"sync"
	"testing"
	"time"

	"github.com/djarvur/ass-guard-agent/internal/event"
)

// TestTypedChannels verifies Subscribe returns a typed receive channel and
// publishing an event of that kind delivers it (D-04).
func TestTypedChannels(t *testing.T) {
	b := event.NewBus()
	ch := b.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	b.Publish(event.AgentMessageChunk{TurnID: "t1", MessageID: "m1", Content: "hi"})
	select {
	case e := <-ch:
		got, ok := e.(event.AgentMessageChunk)
		if !ok {
			t.Fatalf("received %T; want AgentMessageChunk", e)
		}
		if got.Content != "hi" || got.TurnID != "t1" {
			t.Errorf("got = %+v; want Content=hi TurnID=t1", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("did not receive the published AgentMessageChunk")
	}
}

// TestFanOut verifies two subscribers of the same kind both receive a published
// event (each its own channel copy).
func TestFanOut(t *testing.T) {
	b := event.NewBus()
	ch1 := b.Subscribe("RequestShaped", event.BufRequestShaped)
	ch2 := b.Subscribe("RequestShaped", event.BufRequestShaped)
	b.Publish(event.RequestShaped{Profile: "zcode"})
	for i, ch := range []<-chan event.Event{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("subscriber %d did not receive the fan-out event", i)
		}
	}
}

// TestBackpressure verifies a subscriber with a small buffer that never drains
// causes Publish to BLOCK once the buffer fills (D-05 — bounded buffer + block).
// A Publish that should block does not return within 50ms; draining unblocks it.
func TestBackpressure(t *testing.T) {
	b := event.NewBus()
	ch := b.Subscribe("Boundary", 2) // buffer 2, never drained
	b.Publish(event.Boundary{TurnID: "t1"}) // fills slot 1
	b.Publish(event.Boundary{TurnID: "t2"}) // fills slot 2

	// A third Publish must block (buffer full, no drain).
	done := make(chan struct{})
	go func() {
		b.Publish(event.Boundary{TurnID: "t3"})
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("Publish returned while buffer was full and undrained; want it to block (D-05)")
	case <-time.After(50 * time.Millisecond):
		// good: still blocked
	}
	// Drain one slot; the blocked Publish should now complete.
	<-ch
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish did not unblock after draining a slot")
	}
}

// TestNoSubscriber verifies publishing an event with no subscribers does not
// panic (dropped + logged).
func TestNoSubscriber(t *testing.T) {
	b := event.NewBus()
	// Must not panic, must not block.
	done := make(chan struct{})
	go func() {
		b.Publish(event.AgentMessageChunk{Content: "orphan"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Publish with no subscriber blocked; want drop + return")
	}
}

// TestConcurrent verifies 100 publishers + 1 subscriber all deliver with no
// lost events and no races (-race).
func TestConcurrent(t *testing.T) {
	b := event.NewBus()
	ch := b.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Publish(event.AgentMessageChunk{Content: "c"})
		}()
	}
	// Drain n events.
	got := 0
	deadline := time.After(2 * time.Second)
	for got < n {
		select {
		case <-ch:
			got++
		case <-deadline:
			t.Fatalf("received %d/%d events before timeout", got, n)
		}
	}
	wg.Wait()
	if got != n {
		t.Errorf("received %d events; want %d", got, n)
	}
}

// TestEventCatalog verifies each of the 7 event types reports the correct Kind
// discriminator and carries its payload fields (RESEARCH §2.2).
func TestEventCatalog(t *testing.T) {
	cases := []struct {
		kind string
		e    event.Event
	}{
		{"RequestShaped", event.RequestShaped{TurnID: "t", Profile: "p"}},
		{"AgentMessageChunk", event.AgentMessageChunk{TurnID: "t", MessageID: "m", Content: "c"}},
		{"ToolCall", event.ToolCall{TurnID: "t", ToolCallID: "tc", Name: "Bash"}},
		{"ToolCallUpdate", event.ToolCallUpdate{TurnID: "t", ToolCallID: "tc"}},
		{"UsageUpdate", event.UsageUpdate{TurnID: "t", InputTokens: 10, OutputTokens: 5}},
		{"SubagentResult", event.SubagentResult{ParentTurnID: "t", ToolCallID: "tc", Result: "r"}},
		{"Boundary", event.Boundary{TurnID: "t", Cause: "mutating-command:Bash", CommandRef: "tc"}},
	}
	for _, c := range cases {
		if c.e.Kind() != c.kind {
			t.Errorf("Kind() = %q; want %q", c.e.Kind(), c.kind)
		}
	}
}

// TestRequestShapedAdditiveTurnID verifies RequestShaped carries a TurnID (added
// in Phase 2 — additive over the Phase-1 struct).
func TestRequestShapedAdditiveTurnID(t *testing.T) {
	rs := event.RequestShaped{TurnID: "turn_042", Profile: "zcode"}
	if rs.TurnID != "turn_042" {
		t.Errorf("TurnID = %q; want turn_042", rs.TurnID)
	}
}
