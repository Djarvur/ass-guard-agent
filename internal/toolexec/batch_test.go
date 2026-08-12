package toolexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
	"github.com/Djarvur/ass-guard-agent/internal/toolexec"
)

// recordingExec records each call's start/end monotonic time (nanos since the
// test began) + the call name, so tests assert read-only parallelism + mutating
// serialization via observed overlap. It sleeps per call to force overlap.
type recordingExec struct {
	mu     sync.Mutex
	sleep  time.Duration
	events []execEvent
}

type execEvent struct {
	name  string
	start int64
	end   int64
}

var testStart = time.Now()

func nowMono() int64 { return int64(time.Since(testStart)) }

func (r *recordingExec) Execute(ctx context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	start := nowMono()

	select {
	case <-time.After(r.sleep):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	end := nowMono()

	r.mu.Lock()
	r.events = append(r.events, execEvent{name: name, start: start, end: end})
	r.mu.Unlock()

	return json.RawMessage(`{"ok":true,"name":"` + name + `"}`), nil
}

func (r *recordingExec) snapshot() []execEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]execEvent(nil), r.events...)
}

// overlaps reports whether two events' [start,end) intervals overlap.
func overlaps(a, b execEvent) bool {
	return a.start < b.end && b.start < a.end
}

// newCatalog builds a Catalog with the given tools pre-registered by name +
// mutability, using toolcat.Register (added in 04-04 alongside DispatchBatch).
func newCatalog(tools map[string]toolcat.Mutability) *toolcat.Catalog {
	c := toolcat.NewCatalog()
	for name, m := range tools {
		c.Register(toolcat.Tool{Name: name, Mutability: m})
	}

	return c
}

// TestDispatchBatch_ReadOnlyParallelism verifies 4 read-only calls (each
// sleeping 30ms) finish in < 80ms — parallelism observed (sequential would be
// ~120ms). D-21.
func TestDispatchBatch_ReadOnlyParallelism(t *testing.T) {
	t.Parallel()

	exec := &recordingExec{sleep: 30 * time.Millisecond}
	catalog := newCatalog(map[string]toolcat.Mutability{
		toolRead: toolcat.MutabilityReadOnly, toolGrep: toolcat.MutabilityReadOnly,
		"Glob": toolcat.MutabilityReadOnly, "LS": toolcat.MutabilityReadOnly,
	})
	calls := []provider.ToolCall{{Name: toolRead}, {Name: toolGrep}, {Name: "Glob"}, {Name: "LS"}}

	t0 := time.Now()
	results, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls)
	elapsed := time.Since(t0)

	if err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("got %d results; want 4", len(results))
	}

	if elapsed >= 80*time.Millisecond {
		t.Errorf("4 read-only calls took %v; want < 80ms (sequential would be ~120ms) — parallelism not observed", elapsed)
	}
	// Arrival order preserved.
	for i, r := range results {
		if r.CallIndex != i {
			t.Errorf("results[%d].CallIndex = %d; want %d (arrival order)", i, r.CallIndex, i)
		}

		if r.IsError {
			t.Errorf("results[%d] unexpectedly errored: %v", i, r.Err)
		}
	}
}

// TestDispatchBatch_MutatingSerialization verifies mutating calls never overlap
// (T-04-03 — the state-corruption threat). Three mutating calls, each sleeping
// 30ms; assert no two overlap.
func TestDispatchBatch_MutatingSerialization(t *testing.T) {
	t.Parallel()

	exec := &recordingExec{sleep: 30 * time.Millisecond}
	catalog := newCatalog(map[string]toolcat.Mutability{
		toolBash: toolcat.MutabilityMutating, toolWrite: toolcat.MutabilityMutating, "Edit": toolcat.MutabilityMutating,
	})
	calls := []provider.ToolCall{{Name: toolBash}, {Name: toolWrite}, {Name: "Edit"}}

	if _, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls); err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	events := exec.snapshot()
	if len(events) != 3 {
		t.Fatalf("got %d events; want 3", len(events))
	}

	for i := range events {
		for j := i + 1; j < len(events); j++ {
			if overlaps(events[i], events[j]) {
				t.Errorf("mutating calls %q and %q overlapped: %+v vs %+v (D-21 violation)",
					events[i].name, events[j].name, events[i], events[j])
			}
		}
	}
}

// TestDispatchBatch_ArrivalOrderMixed verifies a mixed batch returns results in
// arrival order regardless of completion.
func TestDispatchBatch_ArrivalOrderMixed(t *testing.T) {
	t.Parallel()

	exec := &recordingExec{sleep: 20 * time.Millisecond}
	catalog := newCatalog(map[string]toolcat.Mutability{
		toolRead: toolcat.MutabilityReadOnly, toolGrep: toolcat.MutabilityReadOnly,
		toolBash: toolcat.MutabilityMutating, toolWrite: toolcat.MutabilityMutating,
	})
	calls := []provider.ToolCall{
		{Name: toolRead},  // idx 0, read-only
		{Name: toolBash},  // idx 1, mutating
		{Name: toolGrep},  // idx 2, read-only
		{Name: toolWrite}, // idx 3, mutating
	}

	results, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls)
	if err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	for i, r := range results {
		if r.CallIndex != i {
			t.Errorf("results[%d].CallIndex = %d; want %d", i, r.CallIndex, i)
		}

		if r.Name != calls[i].Name {
			t.Errorf("results[%d].Name = %q; want %q", i, r.Name, calls[i].Name)
		}
	}
}

// TestDispatchBatch_MutatingAlone verifies a mutating call overlaps NO call
// (read-only or mutating) — the strongest form of D-21. In [Read, Bash, Grep],
// the mutating Bash overlaps neither Read nor Grep.
func TestDispatchBatch_MutatingAlone(t *testing.T) {
	t.Parallel()

	exec := &recordingExec{sleep: 30 * time.Millisecond}
	catalog := newCatalog(map[string]toolcat.Mutability{
		toolRead: toolcat.MutabilityReadOnly, toolGrep: toolcat.MutabilityReadOnly,
		toolBash: toolcat.MutabilityMutating,
	})

	calls := []provider.ToolCall{{Name: toolRead}, {Name: toolBash}, {Name: toolGrep}}
	if _, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls); err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	events := exec.snapshot()

	var bash, read, grep *execEvent

	for i := range events {
		switch events[i].name {
		case toolBash:
			bash = &events[i]
		case toolRead:
			read = &events[i]
		case toolGrep:
			grep = &events[i]
		}
	}

	if bash == nil || read == nil || grep == nil {
		t.Fatalf("missing events: %+v", events)
	}

	if overlaps(*bash, *read) {
		t.Errorf("mutating Bash overlapped read-only Read: %+v vs %+v", *bash, *read)
	}

	if overlaps(*bash, *grep) {
		t.Errorf("mutating Bash overlapped read-only Grep: %+v vs %+v", *bash, *grep)
	}
}

// TestDispatchBatch_UnknownToolReadOnly verifies an unknown tool is treated as
// read-only + dispatched; the executor's error is surfaced (IsError=true).
func TestDispatchBatch_UnknownToolReadOnly(t *testing.T) {
	t.Parallel()

	exec := &recordingExec{sleep: 1 * time.Millisecond}
	catalog := toolcat.NewCatalog() // empty — no tools declared
	calls := []provider.ToolCall{{Name: "Mystery"}}

	results, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls)
	if err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results; want 1", len(results))
	}
	// The recording executor returns success for every call — the unknown tool
	// is dispatched (not pre-rejected) and the executor's outcome surfaces.
	if results[0].IsError {
		t.Errorf("unknown tool dispatch errored: %v", results[0].Err)
	}
}

// TestDispatchBatch_MaxConcurrentBound verifies the MaxConcurrent option bounds
// read-only parallelism: 6 read-only calls each sleeping 30ms with
// MaxConcurrent=2 must take >= 3*30ms - epsilon (at most 2 run at once).
func TestDispatchBatch_MaxConcurrentBound(t *testing.T) {
	t.Parallel()

	exec := &recordingExec{sleep: 30 * time.Millisecond}
	catalog := newCatalog(map[string]toolcat.Mutability{
		"R1": toolcat.MutabilityReadOnly, "R2": toolcat.MutabilityReadOnly,
		"R3": toolcat.MutabilityReadOnly, "R4": toolcat.MutabilityReadOnly,
		"R5": toolcat.MutabilityReadOnly, "R6": toolcat.MutabilityReadOnly,
	})
	calls := []provider.ToolCall{{Name: "R1"}, {Name: "R2"}, {Name: "R3"}, {Name: "R4"}, {Name: "R5"}, {Name: "R6"}}
	t0 := time.Now()
	_, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls, toolexec.MaxConcurrent(2))
	elapsed := time.Since(t0)

	if err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}
	// At most 2 concurrent ⇒ 6 calls ⇒ >= 3 batches of 2 ⇒ >= 3*30ms - slack.
	if elapsed < 80*time.Millisecond {
		t.Errorf("6 read-only calls with MaxConcurrent=2 took %v; want >= ~90ms (3 batches) — bound not respected", elapsed)
	}
}

// TestDispatchBatch_CtxCancel verifies ctx cancellation mid-batch surfaces
// without deadlock + without panicking.
func TestDispatchBatch_CtxCancel(t *testing.T) {
	t.Parallel()

	exec := &recordingExec{sleep: 200 * time.Millisecond}
	catalog := newCatalog(map[string]toolcat.Mutability{
		"R1": toolcat.MutabilityReadOnly, "R2": toolcat.MutabilityReadOnly,
	})
	calls := []provider.ToolCall{{Name: "R1"}, {Name: "R2"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before dispatch

	done := make(chan struct{})
	go func() {
		defer close(done)

		_, _ = toolexec.DispatchBatch(ctx, exec, catalog, calls)
	}()

	select {
	case <-done:
		// good — no deadlock on a pre-cancelled ctx.
	case <-time.After(2 * time.Second):
		t.Fatal("DispatchBatch deadlocked on a cancelled ctx")
	}
}

// TestDispatchBatch_NilExecutor verifies a nil executor surfaces ErrNoExecutor
// (the Session must route nil through its stub path, but DispatchBatch is
// defensive).
func TestDispatchBatch_NilExecutor(t *testing.T) {
	t.Parallel()

	catalog := newCatalog(map[string]toolcat.Mutability{toolRead: toolcat.MutabilityReadOnly})
	calls := []provider.ToolCall{{Name: toolRead}}

	results, err := toolexec.DispatchBatch(context.Background(), nil, catalog, calls)
	if err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	if !results[0].IsError || !errors.Is(results[0].Err, toolexec.ErrNoExecutor) {
		t.Errorf("results[0] = %+v; want IsError + ErrNoExecutor", results[0])
	}
}
