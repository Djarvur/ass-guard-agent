package tasks //nolint:testpackage // internal package test

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// The pending-notification queue battery (22-01 Task 2, D-03 + D-01
// fallback): coalescing into one batch per drain, completion-time ordering
// with an injected clock, task-id dedupe (cancel racing complete), empty
// drain no-ops, and rune-boundary tail truncation.

// fakeClock is the deterministic completion-time clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// TestNotify_CoalesceThreeIntoOneBatch (D-03): three completions while idle
// coalesce into ONE drain batch ordered by completion time (injected clock);
// a fourth completion AFTER the drain starts a NEW pending batch.
func TestNotify_CoalesceThreeIntoOneBatch(t *testing.T) {
	t.Parallel()

	clk := &fakeClock{now: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)}
	tr := NewTracker(TrackerOpts{Now: clk.Now})

	tr.Complete(Notification{TaskID: "exec_a", Kind: KindBash, Tail: "aaa"})
	clk.advance(1 * time.Second)
	tr.Complete(Notification{TaskID: "exec_b", Kind: KindBash, Tail: "bbb"})
	clk.advance(1 * time.Second)
	tr.Complete(Notification{TaskID: "exec_c", Kind: KindBash, Tail: "ccc"})

	batch := tr.Drain()
	if len(batch) != 3 {
		t.Fatalf("batch size = %d; want 3 (one coalesced batch)", len(batch))
	}

	wantOrder := []string{"exec_a", "exec_b", "exec_c"}
	for i, want := range wantOrder {
		if batch[i].TaskID != want {
			t.Errorf("batch[%d] = %s; want %s (completion-time order)", i, batch[i].TaskID, want)
		}
	}

	// Empty after the drain.
	if again := tr.Drain(); len(again) != 0 {
		t.Errorf("second drain = %d; want 0 (already drained)", len(again))
	}

	// The fourth completion (after the drain) starts a NEW batch.
	clk.advance(1 * time.Second)
	tr.Complete(Notification{TaskID: "exec_d", Kind: KindBash})

	next := tr.Drain()
	if len(next) != 1 || next[0].TaskID != "exec_d" {
		t.Errorf("post-drain batch = %v; want exactly [exec_d] (batch boundary is the drain)", next)
	}
}

// TestNotify_CompletionTimeOrderingNotArrival: with completions interleaved
// against a pinned clock, the batch is ordered by the CLOCK stamps, not the
// call order (b stamped earlier than a despite arriving later — the
// completion timestamp is the ordering key).
func TestNotify_CompletionTimeOrderingNotArrival(t *testing.T) {
	t.Parallel()

	clk := &fakeClock{now: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)}
	tr := NewTracker(TrackerOpts{Now: clk.Now})

	// a completes LATER (clock advanced between the two calls).
	tr.Complete(Notification{TaskID: "exec_a", Kind: KindBash})
	clk.advance(5 * time.Second)
	tr.Complete(Notification{TaskID: "exec_b", Kind: KindBash})

	// Even though a arrived first, both carry their own stamps; order stays
	// a (t0) then b (t0+5s) — arrival order equals stamp order here. Now the
	// reverse: complete c with an EARLIER stamp than d but LATER arrival.
	tr.Complete(Notification{TaskID: "exec_d", Kind: KindBash})
	clk.advance(-10 * time.Second) // rewind: c stamps before everything
	tr.Complete(Notification{TaskID: "exec_c", Kind: KindBash})

	batch := tr.Drain()
	if len(batch) != 4 {
		t.Fatalf("batch = %d; want 4", len(batch))
	}

	want := []string{"exec_c", "exec_a", "exec_b", "exec_d"}
	for i := range want {
		if batch[i].TaskID != want[i] {
			t.Errorf("batch[%d] = %s; want %s (completion-time order)", i, batch[i].TaskID, want[i])
		}
	}
}

// TestNotify_DedupeByTaskID (PAR-07 concurrency probe): the same task id
// completing twice (a cancel racing a completion) yields exactly ONE pending
// notification — first terminal wins.
func TestNotify_DedupeByTaskID(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{})

	tr.Complete(Notification{TaskID: "exec_x", Kind: KindBash, ExitStatus: "0"})
	tr.Complete(Notification{TaskID: "exec_x", Kind: KindBash, ExitStatus: "killed"})

	batch := tr.Drain()
	if len(batch) != 1 {
		t.Fatalf("batch = %d; want exactly 1 (dedupe by task id)", len(batch))
	}

	if batch[0].ExitStatus != "0" {
		t.Errorf("surviving status = %q; want the FIRST terminal (\"0\")", batch[0].ExitStatus)
	}
}

// TestNotify_EmptyDrainNoop (PAR-07 empty probe): draining an empty queue is
// a no-op — nil batch, no consumer invocation.
func TestNotify_EmptyDrainNoop(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{})

	var mu sync.Mutex

	consumed := 0

	tr.SetDrain(func(pending []Notification) {
		mu.Lock()
		defer mu.Unlock()

		consumed++
	})

	if batch := tr.Drain(); batch != nil {
		t.Errorf("empty drain = %v; want nil", batch)
	}

	// Complete with a drain hook schedules an attempt; the hook receives a
	// peek, but Drain (the consumer) is what the runner calls under the
	// lock. An empty-drain attempt must not double-fire the consumer.
	tr.Drain()

	if consumed != 0 {
		t.Errorf("consumer invocations = %d; want 0 (nothing pending)", consumed)
	}
}

// TestNotify_TruncateTailRuneBoundary (PAR-07 encoding probe): a 20 KB tail
// cuts to TailBytes ending on a COMPLETE UTF-8 rune — a multi-byte rune
// straddling the boundary is dropped whole, never split.
func TestNotify_TruncateTailRuneBoundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		tail  string
		limit int
		check func(t *testing.T, got string)
	}{
		{
			name:  "ascii cut exact",
			tail:  strings.Repeat("a", 20*1024),
			limit: 8192,
			check: func(t *testing.T, got string) {
				if len(got) != 8192 {
					t.Errorf("len = %d; want exactly the 8192 budget", len(got))
				}
			},
		},
		{
			name:  "straddling multibyte rune dropped whole",
			tail:  strings.Repeat("é", 5000), // 2-byte runes
			limit: 8191,                      // 8191 = 4095 runes + 1 byte → straddles
			check: func(t *testing.T, got string) {
				// The boundary straddles rune 4096; it must be dropped whole:
				// 4095 complete runes = 8190 bytes.
				if len(got) != 8190 {
					t.Errorf("len = %d; want 8190 (straddling rune dropped whole)", len(got))
				}

				if !strings.HasPrefix(got, "é") || !strings.HasSuffix(got, "é") {
					t.Error("cut tail lost rune alignment (starts/ends mid-rune)")
				}
			},
		},
		{
			name:  "4-byte emoji straddle",
			tail:  strings.Repeat("😀", 3000), // 4-byte runes
			limit: 8190,                       // 8190 = 2047 runes (8188) + 2 bytes → straddles
			check: func(t *testing.T, got string) {
				if len(got) != 8188 {
					t.Errorf("len = %d; want 8188 (straddling emoji dropped whole)", len(got))
				}
			},
		},
		{
			name:  "under budget untouched",
			tail:  "short tail",
			limit: 8192,
			check: func(t *testing.T, got string) {
				if got != "short tail" {
					t.Errorf("got %q; want verbatim (under budget)", got)
				}
			},
		},
	}

	for _, tc := range cases { //nolint:varnamelen // tc is the table-case convention
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := truncateTail(tc.tail, tc.limit)
			tc.check(t, got)
		})
	}
}

// TestNotify_TrackerTruncatesOnComplete: the Tracker applies the configured
// TailBytes budget at enqueue (the notification payload carries the
// truncated tail; the model never receives more than the budget).
func TestNotify_TrackerTruncatesOnComplete(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{TailBytes: 64})

	tr.Complete(Notification{TaskID: "exec_t", Kind: KindBash, Tail: strings.Repeat("z", 512)})

	batch := tr.Drain()
	if len(batch) != 1 {
		t.Fatalf("batch = %d; want 1", len(batch))
	}

	if len(batch[0].Tail) > 64 {
		t.Errorf("tail len = %d; want <= the 64-byte budget", len(batch[0].Tail))
	}
}

// TestNotify_BusyDrainLeavesPending (D-01 fallback, queue level): a drain
// attempt that declines (the runner's busy-turn path never calls Drain while
// the mutex is held) leaves every notification pending; the next drain
// carries ALL of them in one batch — they coalesced while waiting.
func TestNotify_BusyDrainLeavesPending(t *testing.T) {
	t.Parallel()

	tr := NewTracker(TrackerOpts{})

	tr.Complete(Notification{TaskID: "exec_w1", Kind: KindBash, Tail: "one"})
	tr.Complete(Notification{TaskID: "exec_w2", Kind: KindBash, Tail: "two"})
	tr.Complete(Notification{TaskID: "exec_w3", Kind: KindBash, Tail: "three"})

	// The busy window: NO Drain call happens (the runner's contract).

	batch := tr.Drain()
	if len(batch) != 3 {
		t.Fatalf("post-busy batch = %d; want all 3 coalesced", len(batch))
	}
}
