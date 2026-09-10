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

// Unsubscribe removes a subscriber (matched by channel identity) so its
// channel stops receiving events. Callers that subscribe per-unit-of-work (e.g.
// a per-turn forwarder) MUST unsubscribe when the unit ends: Publish blocks on
// a full subscriber channel (D-05 backpressure), and a channel nobody drains
// anymore wedges every later publish once its buffer fills (the stack-proven
// 08-15 finding: a leaked per-turn forwarder stalled the multi-stage E2E at
// chunk #129). It is the caller's job to stop publishing to that kind before
// (or while) unsubscribing — a Publish already holding the pre-removal
// subscriber list may still send to the removed channel.
func (b *Bus) Unsubscribe(kind string, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subs[kind]
	for i, c := range subs {
		if c == ch { // channel identity (receive-only channels compare by the same backing channel)
			b.subs[kind] = append(subs[:i], subs[i+1:]...)

			return
		}
	}
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
