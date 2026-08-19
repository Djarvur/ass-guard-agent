package toolexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

var testStart = time.Now() //nolint:gochecknoglobals // test fixture

func nowMono() int64 { return int64(time.Since(testStart)) }

func (r *recordingExec) Execute(ctx context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	start := nowMono()

	select {
	case <-time.After(r.sleep):
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx: %w", ctx.Err())
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
		toolGlob: toolcat.MutabilityReadOnly, "LS": toolcat.MutabilityReadOnly,
	})
	calls := []provider.ToolCall{{Name: toolRead}, {Name: toolGrep}, {Name: toolGlob}, {Name: "LS"}}

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
		t.Errorf("4 read-only calls took %v; want < 80ms "+
			"(sequential would be ~120ms) — parallelism not observed", elapsed)
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
		toolBash: toolcat.MutabilityMutating, toolWrite: toolcat.MutabilityMutating,
		toolEdit: toolcat.MutabilityMutating,
	})
	calls := []provider.ToolCall{{Name: toolBash}, {Name: toolWrite}, {Name: toolEdit}}

	_, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls)
	if err != nil {
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

	_, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls)
	if err != nil {
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
		t.Errorf("6 read-only calls with MaxConcurrent=2 took %v; "+
			"want >= ~90ms (3 batches) — bound not respected", elapsed)
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

// --- 14-06 (EARLY-06): per-tool timeout backstop at the dispatch seam ---

// slowExec is a timeout-aware fake: per-name sleep durations. It respects ctx
// (selects on Done) so a deadline-bounded call returns at the deadline, not at
// the sleep's end — the executor contract the DispatchBatch wrap relies on.
type slowExec struct {
	mu     sync.Mutex
	sleep  map[string]time.Duration
	events []execEvent
}

func (s *slowExec) Execute(ctx context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	start := nowMono()

	select {
	case <-time.After(s.sleep[name]):
	case <-ctx.Done():
		return nil, fmt.Errorf("exec %s: %w", name, ctx.Err())
	}

	end := nowMono()

	s.mu.Lock()
	s.events = append(s.events, execEvent{name: name, start: start, end: end})
	s.mu.Unlock()

	return json.RawMessage(`{"ok":true,"name":"` + name + `"}`), nil
}

func (s *slowExec) snapshot() []execEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]execEvent(nil), s.events...)
}

// Fixture tool names (14-06 timeout/flag tests).
const (
	toolSlowRO  = "SlowRO"
	toolFlagged = "FlaggedRO"
)

// annotatedCatalog registers tools carrying 14-06 contract annotations
// (timeout_ms) beside the mutability field, the way coretools.json does.
func annotatedCatalog(tools ...toolcat.Tool) *toolcat.Catalog {
	c := toolcat.NewCatalog()
	for _, t := range tools {
		c.Register(t)
	}

	return c
}

// TestDispatchBatch_PerToolTimeout_Bounds (14-06 test 1): a tool annotated
// timeout_ms=50 whose executor sleeps 500ms is bounded to its deadline — the
// result carries IsError=true with a timeout-classified message — while a
// SIBLING call in the same batch (annotated 2000ms, sleeping 100ms) completes
// successfully: one call's deadline never cancels its siblings (each call is
// wrapped in its OWN context).
func TestDispatchBatch_PerToolTimeout_Bounds(t *testing.T) {
	t.Parallel()

	exec := &slowExec{sleep: map[string]time.Duration{
		toolRead: 500 * time.Millisecond,
		toolGrep: 100 * time.Millisecond,
	}}
	catalog := annotatedCatalog(
		toolcat.Tool{Name: toolRead, Mutability: toolcat.MutabilityReadOnly, TimeoutMS: 50},
		toolcat.Tool{Name: toolGrep, Mutability: toolcat.MutabilityReadOnly, TimeoutMS: 2000},
	)
	calls := []provider.ToolCall{{Name: toolRead}, {Name: toolGrep}}

	t0 := time.Now()
	results, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls)
	elapsed := time.Since(t0)

	if err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	// The batch must not wait out the 500ms sleep — bounded by the 50ms cap
	// plus the 100ms sibling, with scheduling slack.
	if elapsed >= 400*time.Millisecond {
		t.Errorf("batch took %v; want < 400ms (Read capped at 50ms, Grep 100ms)", elapsed)
	}

	// The capped call: IsError + the timeout-classified form.
	if !results[0].IsError {
		t.Errorf("capped Read result not IsError: %+v", results[0])
	}

	if !errors.Is(results[0].Err, context.DeadlineExceeded) {
		t.Errorf("capped Read Err = %v; want context.DeadlineExceeded in chain", results[0].Err)
	}

	wantOut := `{"error":"toolexec: tool ` + toolRead + ` timed out after 50ms"}`
	if string(results[0].Output) != wantOut {
		t.Errorf("capped Read Output = %s; want %s", results[0].Output, wantOut)
	}

	// The sibling COMPLETED in the same batch (isolation edge).
	if results[1].IsError {
		t.Errorf("sibling Grep errored: %v — one deadline must not cancel siblings", results[1].Err)
	}

	events := exec.snapshot()
	if len(events) != 1 || events[0].name != toolGrep {
		t.Errorf("exec events = %+v; want exactly the sibling Grep completion", events)
	}
}

// TestDispatchBatch_DefaultBackstop (14-06 test 2): a tool with NO timeout
// annotation resolves to DefaultToolTimeoutMS — asserted at the accessor
// (the resolved value, never by sleeping out the 120s default).
func TestDispatchBatch_DefaultBackstop(t *testing.T) {
	t.Parallel()

	if toolcat.DefaultToolTimeoutMS != 120000 {
		t.Errorf("DefaultToolTimeoutMS = %d; want 120000", toolcat.DefaultToolTimeoutMS)
	}

	for _, tc := range []struct {
		name string
		tool toolcat.Tool
	}{
		{name: "zero value", tool: toolcat.Tool{Name: "Bare"}},
		{name: "read-only registered", tool: toolcat.Tool{Name: toolGrep, Mutability: toolcat.MutabilityReadOnly}},
		{name: "mutating registered", tool: toolcat.Tool{Name: toolBash, Mutability: toolcat.MutabilityMutating}},
		{name: "negative annotation", tool: toolcat.Tool{Name: "Weird", TimeoutMS: -5}},
	} {
		if got := tc.tool.EffectiveTimeoutMS(); got != toolcat.DefaultToolTimeoutMS {
			t.Errorf("%s: EffectiveTimeoutMS() = %d; want default %d", tc.name, got, toolcat.DefaultToolTimeoutMS)
		}
	}

	// Through the catalog round-trip (the DispatchBatch lookup path).
	c := annotatedCatalog(toolcat.Tool{Name: "Bare", Mutability: toolcat.MutabilityReadOnly})
	if tl, ok := c.Get("Bare"); !ok || tl.EffectiveTimeoutMS() != toolcat.DefaultToolTimeoutMS {
		t.Errorf("catalog Get(Bare) = %+v ok=%v; want default timeout %d", tl, ok, toolcat.DefaultToolTimeoutMS)
	}
}

// TestDispatchBatch_TimeoutDoesNotBreakD21 (14-06 test 4): a timing-out
// read-only call is followed by a mutating call — the mutating call still runs
// ALONE-IN-SLOT after the pool drains (D-21 unbroken by the timeout wrap).
func TestDispatchBatch_TimeoutDoesNotBreakD21(t *testing.T) {
	t.Parallel()

	exec := &slowExec{sleep: map[string]time.Duration{
		toolSlowRO: 500 * time.Millisecond,
		toolBash:   30 * time.Millisecond,
	}}
	catalog := annotatedCatalog(
		toolcat.Tool{Name: toolSlowRO, Mutability: toolcat.MutabilityReadOnly, TimeoutMS: 50},
		toolcat.Tool{Name: toolBash, Mutability: toolcat.MutabilityMutating, TimeoutMS: 60000},
	)
	calls := []provider.ToolCall{{Name: toolSlowRO}, {Name: toolBash}}

	t0 := time.Now()
	results, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls)
	elapsed := time.Since(t0)

	if err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	if !results[0].IsError {
		t.Errorf("%s result not IsError: %+v", toolSlowRO, results[0])
	}

	if results[1].IsError {
		t.Errorf("mutating Bash errored after sibling timeout: %v", results[1].Err)
	}

	events := exec.snapshot()
	if len(events) != 1 || events[0].name != toolBash {
		t.Fatalf("exec events = %+v; want exactly the alone-in-slot Bash run", events)
	}

	// The timed-out pool call must not hold the batch: ~50ms cap + 30ms Bash.
	if elapsed >= 400*time.Millisecond {
		t.Errorf("batch took %v; want < 400ms (D-21 drain not blocked by the capped call)", elapsed)
	}
}

// TestDispatchBatch_MutatingTimeoutBounded: the per-call deadline wraps the
// SERIALIZED slot identically — a mutating tool annotated tight is bounded the
// same way a pooled read-only call is.
func TestDispatchBatch_MutatingTimeoutBounded(t *testing.T) {
	t.Parallel()

	exec := &slowExec{sleep: map[string]time.Duration{
		toolBash: 500 * time.Millisecond,
	}}
	catalog := annotatedCatalog(
		toolcat.Tool{Name: toolBash, Mutability: toolcat.MutabilityMutating, TimeoutMS: 50},
	)
	calls := []provider.ToolCall{{Name: toolBash}}

	t0 := time.Now()
	results, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls)
	elapsed := time.Since(t0)

	if err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	if !results[0].IsError || !errors.Is(results[0].Err, context.DeadlineExceeded) {
		t.Errorf("capped Bash result = %+v; want IsError + DeadlineExceeded", results[0])
	}

	wantOut := `{"error":"toolexec: tool ` + toolBash + ` timed out after 50ms"}`
	if string(results[0].Output) != wantOut {
		t.Errorf("capped Bash Output = %s; want %s", results[0].Output, wantOut)
	}

	if elapsed >= 400*time.Millisecond {
		t.Errorf("serialized slot took %v; want < 400ms (bounded by the 50ms cap)", elapsed)
	}
}

// TestDispatchBatch_UsesConcurrencySafeFlag (14-06 test 7): a read-only tool
// DECLARED concurrency_safe=false runs OUTSIDE the parallel pool (serialized,
// alone-in-slot) even though it is read-only — the declared flag overrides the
// mutability default for pool membership. Mutating tools stay alone-in-slot
// regardless (the D-21 floor is enforced independently of the flag, T-14-19).
func TestDispatchBatch_UsesConcurrencySafeFlag(t *testing.T) {
	t.Parallel()

	no := false
	exec := &slowExec{sleep: map[string]time.Duration{
		toolFlagged: 80 * time.Millisecond,
		toolGrep:    80 * time.Millisecond,
		toolRead:    80 * time.Millisecond,
	}}
	catalog := annotatedCatalog(
		toolcat.Tool{Name: toolFlagged, Mutability: toolcat.MutabilityReadOnly, ConcurrencySafeOpt: &no},
		toolcat.Tool{Name: toolGrep, Mutability: toolcat.MutabilityReadOnly},
		toolcat.Tool{Name: toolRead, Mutability: toolcat.MutabilityReadOnly},
	)
	calls := []provider.ToolCall{{Name: toolFlagged}, {Name: toolGrep}, {Name: toolRead}}

	results, err := toolexec.DispatchBatch(context.Background(), exec, catalog, calls)
	if err != nil {
		t.Fatalf("DispatchBatch err = %v", err)
	}

	for i, r := range results {
		if r.IsError {
			t.Errorf("results[%d] errored: %v", i, r.Err)
		}
	}

	events := exec.snapshot()
	if len(events) != 3 {
		t.Fatalf("exec events = %d; want 3 (%+v)", len(events), events)
	}

	var flagged, grep, read *execEvent

	for i := range events {
		switch events[i].name {
		case toolFlagged:
			flagged = &events[i]
		case toolGrep:
			grep = &events[i]
		case toolRead:
			read = &events[i]
		}
	}

	if flagged == nil || grep == nil || read == nil {
		t.Fatalf("missing events: %+v", events)
	}

	// The two pool-eligible read-only calls DID overlap (the pool works).
	if !overlaps(*grep, *read) {
		t.Errorf("pool-eligible %s/%s did not overlap — parallelism broken", toolGrep, toolRead)
	}

	// The flagged call overlapped NEITHER — it ran serialized (outside the pool).
	if overlaps(*flagged, *grep) || overlaps(*flagged, *read) {
		t.Errorf("concurrency_safe=false call overlapped a pool call — the declaration is not consumed")
	}
}
