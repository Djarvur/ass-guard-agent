// Package event implements the minimal Phase-1 event bus (D-13). It is the seed
// Phase 2 expands into a fuller bus with more event types and back-pressure.
//
// Publish dispatches to every subscriber of the event's Kind in its OWN
// goroutine (goroutine-per-subscriber from day one) so async consumers like the
// audit log never block the critical path (LOG-02's contract).
package event

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is a value emitted on the bus. Each concrete event reports its Kind.
type Event interface {
	Kind() string
}

// RequestShaped carries the verbatim shaped outgoing request (LOG-01 evidence).
// Published by the Turn Loop before the provider sends; the audit log subscribes.
type RequestShaped struct {
	VerbatimRequest json.RawMessage
	Profile         string
	Timestamp       time.Time
}

// Kind returns the event discriminator.
func (RequestShaped) Kind() string { return "RequestShaped" }

// Handler is a subscriber callback invoked per matching event in its own goroutine.
type Handler func(Event)

// Bus is the minimal pub/sub. A zero Bus is unusable; use NewBus.
type Bus struct {
	mu   sync.Mutex
	subs map[string][]Handler
}

// NewBus returns an empty Bus.
func NewBus() *Bus { return &Bus{subs: map[string][]Handler{}} }

// Subscribe registers a handler for the given event kind.
func (b *Bus) Subscribe(kind string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[kind] = append(b.subs[kind], h)
}

// Publish dispatches e to every subscriber of e.Kind(), each in its own goroutine
// (async, goroutine-per-subscriber — D-13 / LOG-02).
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	subs := append([]Handler(nil), b.subs[e.Kind()]...)
	b.mu.Unlock()
	for _, h := range subs {
		go h(e)
	}
}
