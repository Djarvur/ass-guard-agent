package tasks

import (
	"sort"
	"time"
	"unicode/utf8"
)

// notify.go carries the pending-notification queue internals (22-01): the
// completion-time-ordered, task-id-deduped pending list behind Tracker, the
// snapshotAndClear consumption, and the rune-boundary-safe tail truncation.
// The queue is guarded by the owning Tracker's mutex (it is embedded, not
// standalone — no second lock hierarchy).

// pendingItem pairs a Notification with its completion timestamp (the D-03
// ordering key; submission order breaks ties — sort.SliceStable).
type pendingItem struct {
	n           Notification
	completedAt time.Time
	seq         int // arrival sequence — stable tie-breaker (PAR-08 FIFO probe family)
}

// pendingQueue is the per-session pending list. All methods assume the
// caller holds the owning Tracker's mutex.
type pendingQueue struct {
	items map[string]pendingItem // dedupe by task id — first terminal wins
	order []string              // arrival order of live ids
	seq   int
}

// add appends n ordered by arrival; a second terminal event for an id
// already pending is IGNORED (exactly-once per task id — PAR-07's
// concurrency probe: a cancel racing a completion yields one notification).
// Ordering by completedAt is applied at read time (peek/snapshotAndClear):
// arrival order is preserved for equal timestamps via the stable sort.
func (q *pendingQueue) add(n Notification, completedAt time.Time) {
	if q.items == nil {
		q.items = map[string]pendingItem{}
	}

	if _, exists := q.items[n.TaskID]; exists {
		return // first terminal wins
	}

	q.seq++
	q.items[n.TaskID] = pendingItem{n: n, completedAt: completedAt, seq: q.seq}
	q.order = append(q.order, n.TaskID)
}

// snapshotOrdered returns the live notifications in completion-time order
// (arrival order breaking ties) without consuming them.
func (q *pendingQueue) snapshotOrdered() []Notification {
	live := make([]pendingItem, 0, len(q.order))

	for _, id := range q.order {
		if it, ok := q.items[id]; ok {
			live = append(live, it)
		}
	}

	sort.SliceStable(live, func(i, j int) bool { //nolint:gocritic // the closure reads clearly
		if live[i].completedAt.Equal(live[j].completedAt) {
			return live[i].seq < live[j].seq // submission order breaks ties
		}

		return live[i].completedAt.Before(live[j].completedAt)
	})

	out := make([]Notification, len(live))

	for i := range live {
		out[i] = live[i].n
	}

	return out
}

// peek returns a non-destructive ordered copy of the pending batch.
func (q *pendingQueue) peek() []Notification {
	if len(q.items) == 0 {
		return nil
	}

	return q.snapshotOrdered()
}

// snapshotAndClear drains the queue: the ordered batch is returned and the
// queue emptied (the single destructive consumer — Tracker.Drain).
func (q *pendingQueue) snapshotAndClear() []Notification {
	if len(q.items) == 0 {
		return nil
	}

	out := q.snapshotOrdered()
	q.items = map[string]pendingItem{}
	q.order = nil

	return out
}

// truncateTail cuts s to its LAST limit bytes ending on a complete UTF-8
// rune: a multi-byte rune straddling the boundary is dropped WHOLE, never
// split (PAR-07's encoding probe).
func truncateTail(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}

	cut := s[len(s)-limit:]

	// Advance past any leading continuation bytes: the straddling rune's
	// lead bytes were cut off, so the partial rune is dropped whole.
	for i := 0; i < len(cut) && i < utf8.UTFMax; i++ {
		if utf8.RuneStart(cut[i]) {
			return cut[i:]
		}
	}

	return cut // degenerate all-continuation slice — nothing whole remains
}
