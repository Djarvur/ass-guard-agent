package session //nolint:testpackage // internal package test (accesses unexported symbols)

// 16-REVIEW WR-02 regression pin: a panic between Semaphore.Acquire and
// Release must not leak the slot. The recover guards live at the
// runTurn/Prompt deferred level, so with a straight-line Release a panic in
// the stream window (an MCP/catalog tool bridge is enough) unwound past it;
// after maxConc leaks every future turn blocked forever in Acquire — the
// agent wedged with no diagnostic. The fix releases via defer inside
// withSemaphore; this test proves a panicking turn hands the slot back.

import (
	"context"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

func TestSemaphoreSlotSurvivesTurnPanic(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	s, _, fp := newTestSession(t, bus, []provider.Response{{FinishReason: stopEndTurn}})
	// ONE slot: a single leak wedges the next turn (the finding's failure mode).
	s.Semaphore = provider.NewSemaphore(1)

	fp.panicOn = 1 // the first Stream call panics (synchronously — inside the semaphore window)

	// Turn 1 panics inside the stream; the turn-level recover converts it to an
	// error — and (the pin) the semaphore slot is handed back on the way out.
	stop1, err1 := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "panic please"}})
	if err1 == nil {
		t.Fatal("the injected provider panic was not recovered into an error")
	}

	_ = stop1

	// Turn 2 must acquire the returned slot. With the leak the acquire blocks
	// until the deadline and the turn dies as "cancelled" — the assertion below
	// fails and names the leak (bounded, no hung test).
	ctx2, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stop2, err2 := s.Prompt(ctx2, []ContentBlock{{Type: blockText, Text: "again"}})
	if err2 != nil {
		t.Fatalf("second Prompt: %v", err2)
	}

	if stop2 != stopEndTurn {
		t.Errorf("second turn stop = %q; want %q — the semaphore slot leaked on the panicking turn "+
			"(the second turn blocked in Acquire until ctx death)", stop2, stopEndTurn)
	}
}
