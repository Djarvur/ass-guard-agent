package event_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/djarvur/ass-guard-agent/internal/event"
)

// TestBus_DeliversToMatchingSubscriber confirms a RequestShaped event reaches a
// subscriber of its kind with the correct payload.
func TestBus_DeliversToMatchingSubscriber(t *testing.T) {
	b := event.NewBus()
	var wg sync.WaitGroup
	wg.Add(1)
	var got event.RequestShaped
	b.Subscribe("RequestShaped", func(e event.Event) {
		defer wg.Done()
		got = e.(event.RequestShaped)
	})
	want := event.RequestShaped{
		VerbatimRequest: json.RawMessage(`{"model":"x"}`),
		Profile:         "zcode",
		Timestamp:       time.Now(),
	}
	b.Publish(want)
	wg.Wait()
	if got.Profile != "zcode" {
		t.Errorf("Profile = %q", got.Profile)
	}
	if string(got.VerbatimRequest) != `{"model":"x"}` {
		t.Errorf("VerbatimRequest = %s", got.VerbatimRequest)
	}
	if got.Kind() != "RequestShaped" {
		t.Errorf("Kind = %q, want RequestShaped", got.Kind())
	}
}

// TestBus_DoesNotDeliverToOtherKind confirms subscribers only get their kind.
func TestBus_DoesNotDeliverToOtherKind(t *testing.T) {
	b := event.NewBus()
	called := false
	b.Subscribe("OtherKind", func(e event.Event) { called = true })
	b.Publish(event.RequestShaped{Profile: "zcode"})
	// Publish dispatches in goroutines; give them a beat to (not) run.
	time.Sleep(20 * time.Millisecond)
	if called {
		t.Error("OtherKind subscriber was called for a RequestShaped event")
	}
}

// TestBus_MultipleSubscribers confirms every matching subscriber is invoked.
func TestBus_MultipleSubscribers(t *testing.T) {
	b := event.NewBus()
	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0
	for i := 0; i < 3; i++ {
		wg.Add(1)
		b.Subscribe("RequestShaped", func(e event.Event) {
			defer wg.Done()
			mu.Lock()
			count++
			mu.Unlock()
		})
	}
	b.Publish(event.RequestShaped{})
	wg.Wait()
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}
