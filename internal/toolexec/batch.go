package toolexec

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// DefaultMaxConcurrent is the default read-only parallelism bound (TOOL-04),
// matching the provider semaphore (PARA-04 = 6). A read-only batch with more
// calls than this is re-batched into groups of at most this size by the
// semaphore (goroutines block on acquire until a slot frees).
const DefaultMaxConcurrent = 6

// ErrNoExecutor is returned for a call when DispatchBatch is given a nil
// executor (a Session without SetToolExecutor should use its stub path, not
// reach DispatchBatch — but the error is structured if it does).
var ErrNoExecutor = errors.New("toolexec: no tool executor configured")

// ToolResult is one call's outcome, indexed by the call's position in the input
// slice (CallIndex) so callers receive results in ARRIVAL ORDER regardless of
// completion order (the transcript stays deterministic — D-21 ordering rule).
type ToolResult struct {
	// CallIndex is the position in the input calls slice (arrival order).
	CallIndex int
	// Name is the tool-call's name (mirrors calls[CallIndex].Name).
	Name string
	// Output is the raw-JSON tool result (written to the transcript).
	Output json.RawMessage
	// Err is a non-nil error when execution failed.
	Err error
	// IsError mirrors the ACP is_error flag (a tool can return a structured
	// error in its Output rather than a Go err).
	IsError bool
}

// BatchOpt is a functional option for DispatchBatch.
type BatchOpt func(*batchConfig)

type batchConfig struct {
	maxConcurrent int
}

// MaxConcurrent sets the read-only parallelism bound (default
// DefaultMaxConcurrent). Values < 1 fall back to the default.
func MaxConcurrent(n int) BatchOpt {
	return func(c *batchConfig) {
		if n > 0 {
			c.maxConcurrent = n
		}
	}
}

// DispatchBatch runs the calls: read-only calls run concurrently (bounded by
// MaxConcurrent), mutating calls run strictly sequentially relative to each
// other (D-21 — the Phase-2 D-19 mutability field drives the classification).
// Results are returned in ARRIVAL ORDER (CallIndex matches the input index).
//
// A mutating call NEVER overlaps ANY other call (neither read-only nor
// mutating) — it is alone in its serialization slot: DispatchBatch waits for
// all read-only goroutines to drain, then runs mutating calls one at a time,
// waiting for each to finish before the next.
//
// An unknown tool (not in the catalog, not config-added) is treated as
// read-only by default and dispatched; its executor surfaces the error in the
// result (IsError=true). ctx cancellation aborts in-flight calls (the read-only
// semaphore acquire + the mutating loop both respect ctx).
//
//nolint:funlen // domain complexity is inherent
func DispatchBatch(
	ctx context.Context, exec toolcat.ToolExecutor, catalog *toolcat.Catalog,
	calls []provider.ToolCall, opts ...BatchOpt,
) ([]ToolResult, error) {
	cfg := batchConfig{maxConcurrent: DefaultMaxConcurrent}
	for _, opt := range opts {
		opt(&cfg)
	}

	results := make([]ToolResult, len(calls))
	if len(calls) == 0 {
		return results, nil
	}

	type indexed struct {
		idx  int
		call provider.ToolCall
	}

	var readOnly, mutating []indexed

	for i, c := range calls {
		if isMutating(c.Name, catalog) {
			mutating = append(mutating, indexed{idx: i, call: c})
		} else {
			readOnly = append(readOnly, indexed{idx: i, call: c})
		}
	}

	// Read-only calls: spawn all goroutines immediately; the semaphore bounds
	// concurrency, not goroutine count (typical batches are small). Each
	// goroutine acquires a slot (ctx-aware), executes, writes its OWN results
	// slot, releases. No shared slot ⇒ no synchronization beyond the semaphore.
	var wg sync.WaitGroup

	sem := make(chan struct{}, cfg.maxConcurrent)

	for _, ro := range readOnly {
		wg.Add(1)
		go func(ro indexed) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[ro.idx] = ToolResult{CallIndex: ro.idx, Name: ro.call.Name, Err: ctx.Err(), IsError: true}

				return
			}

			defer func() { <-sem }()

			results[ro.idx] = executeOne(ctx, exec, ro.idx, ro.call)
		}(ro)
	}

	// Wait for all read-only calls to drain before the first mutating call so
	// each mutating call is provably alone in its slot (D-21 invariant).
	wg.Wait()

	// Mutating calls: strictly sequential, each one alone. ctx checked before
	// every call so a cancel surfaces promptly without deadlock.
	for _, m := range mutating {
		err := ctx.Err()
		if err != nil {
			results[m.idx] = ToolResult{CallIndex: m.idx, Name: m.call.Name, Err: err, IsError: true}

			continue
		}
		// Synchronous (alone) — no goroutine, no overlap.
		results[m.idx] = executeOne(ctx, exec, m.idx, m.call)
	}

	return results, nil
}

// executeOne runs one call through the executor and packages the result. A nil
// executor surfaces ErrNoExecutor; an executor error sets IsError + Err but
// still returns a (possibly empty) Output so a structured error payload is not
// lost.
func executeOne(ctx context.Context, exec toolcat.ToolExecutor, idx int, call provider.ToolCall) ToolResult {
	res := ToolResult{CallIndex: idx, Name: call.Name}
	if exec == nil {
		res.Err = ErrNoExecutor
		res.IsError = true

		return res
	}

	out, err := exec.Execute(ctx, call.Name, call.Input)
	if err != nil {
		res.Err = err

		res.IsError = true
		if len(out) > 0 {
			res.Output = out
		}

		return res
	}

	res.Output = out

	return res
}

// isMutating reports whether name is a catalog-declared mutating tool. Unknown
// tools default to read-only (D-21 — they cannot mutate anything ass-guard
// tracks, and the model sees them in the catalog regardless).
func isMutating(name string, catalog *toolcat.Catalog) bool {
	if catalog == nil {
		return false
	}

	t, ok := catalog.Get(name)
	if !ok {
		return false
	}

	return t.IsMutating()
}
