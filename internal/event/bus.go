// Package event implements the typed-channel event bus (D-04/D-05) — the spine
// of the streaming path AND the async audit/transcript writer AND future
// subagent results.
//
// Producers Publish; consumers Subscribe to a kind and receive a typed receive
// channel. Each channel is buffered to a per-kind bound (D-05 — bounded buffer
// + block): a slow consumer fills its buffer, then the producer blocks on send,
// which naturally slows the provider SSE read (Go's io.Reader blocks until
// consumed). Backpressure propagates end-to-end. A stuck client stalls the turn
// rather than dropping user-visible tokens or growing memory unbounded.
//
// The Phase-1 seed (goroutine-per-subscriber handler API) is REPLACED by this
// channel API in Phase 2: consumers select on the channels they care about
// (idiomatic Go, type-safe, zero-dep).
package event

import (
	"log"
	"sync"
)

// Event is a value emitted on the bus. Each concrete event reports its Kind.
type Event interface{ Kind() string }

// Bus is the typed-channel pub/sub. A zero Bus is unusable; use NewBus.
type Bus struct {
	mu   sync.RWMutex
	subs map[string][]chan Event
}

// NewBus returns an empty Bus.
func NewBus() *Bus { return &Bus{subs: map[string][]chan Event{}} }

// Subscribe registers a new subscriber for the given event kind and returns its
// receive-only channel. buffer is the bounded capacity (D-05). Subscribe is
// safe to call before publishes begin; new subscribers only see events
// published AFTER they subscribe (no replay of the backlog).
func (b *Bus) Subscribe(kind string, buffer int) <-chan Event {
	if buffer < 0 {
		buffer = 0
	}

	ch := make(chan Event, buffer)

	b.mu.Lock()
	b.subs[kind] = append(b.subs[kind], ch)
	b.mu.Unlock()

	return ch
}

// Publish fan-outs e to every subscriber of e.Kind(), blocking on each full
// channel (D-05 — a slow consumer blocks the producer, propagating backpressure
// end-to-end). If there are no subscribers, the event is dropped + logged (an
// absent consumer never blocks a producer).
func (b *Bus) Publish(e Event) {
	if e == nil {
		return
	}

	b.mu.RLock()
	subs := append([]chan Event(nil), b.subs[e.Kind()]...)
	b.mu.RUnlock()

	if len(subs) == 0 {
		log.Printf("event bus: no subscriber for %s (event dropped)", e.Kind())

		return
	}

	for _, ch := range subs {
		ch <- e // blocks on full (D-05 backpressure)
	}
}

// Close closes every subscriber channel so consumer `range` loops exit cleanly
// at shutdown. After Close, Publish panics (send on closed channel) — only call
// Close at shutdown after all publishers have stopped.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for kind, subs := range b.subs {
		for _, ch := range subs {
			close(ch)
		}

		delete(b.subs, kind)
	}
}
