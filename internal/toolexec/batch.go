package toolexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// DefaultMaxConcurrent is the default read-only parallelism bound (TOOL-04),
// matching the provider semaphore (PARA-04 = 6). A read-only batch with more
// calls than this is re-batched into groups of at most this size by the
// semaphore (goroutines block on acquire until a slot frees).
const DefaultMaxConcurrent = 6

// keyError is the structured-error convention's map key (the shipped
// corpus-absent failure convention — {"error":…}; same key as coreexec).
const keyError = "error"

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
// Every call — pooled or serialized — is wrapped in its OWN context deadline
// derived from the tool's timeout_ms annotation (default
// toolcat.DefaultToolTimeoutMS; 14-06/EARLY-06): a timing-out call returns an
// IsError result with the timeout-classified form and NEVER cancels its
// siblings. Inner tool-specific deadlines (Bash model-ms, openspec
// per-command) fire first — this is the outer backstop.
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

			results[ro.idx] = executeBounded(ctx, exec, catalog, ro.idx, ro.call)
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
		// Synchronous (alone) — no goroutine, no overlap. The per-call
		// deadline wraps the serialized slot identically to the pool (14-06).
		results[m.idx] = executeBounded(ctx, exec, catalog, m.idx, m.call)
	}

	return results, nil
}

// timeoutFor resolves a call's per-tool timeout backstop: the catalog entry's
// timeout_ms annotation when set, else toolcat.DefaultToolTimeoutMS (unknown
// and unannotated tools — MCP/plugin/dynamically-registered — get the
// default). This is the OUTER bound: a tool's own tighter inner deadline
// (Bash's model-provided ms timeout, openspec's per-command timeout) fires
// first because it is always <= this value.
func timeoutFor(name string, catalog *toolcat.Catalog) time.Duration {
	if catalog != nil {
		if t, ok := catalog.Get(name); ok {
			return time.Duration(t.EffectiveTimeoutMS()) * time.Millisecond
		}
	}

	return time.Duration(toolcat.DefaultToolTimeoutMS) * time.Millisecond
}

// executeBounded wraps one call in its OWN context deadline derived from the
// tool's timeout annotation (14-06 / EARLY-06 — no tool call can hold a turn
// indefinitely), runs it, and maps a deadline expiry to an IsError result
// carrying the timeout-classified structured form (the openspec ClassTimeout
// vocabulary, rendered in the shipped {"error":…} convention). Each call's
// deadline is derived from the batch ctx, so ONE call timing out never cancels
// its siblings.
func executeBounded(
	ctx context.Context, exec toolcat.ToolExecutor, catalog *toolcat.Catalog, idx int, call provider.ToolCall,
) ToolResult {
	d := timeoutFor(call.Name, catalog)

	callCtx, cancel := context.WithTimeout(ctx, d)
	defer cancel()

	res := executeOne(callCtx, exec, idx, call)

	// Deadline-expiry mapping: the wrap's own deadline fired (not the parent
	// ctx cancelling) AND the executor surfaced the deadline error.
	if !res.IsError || !errors.Is(callCtx.Err(), context.DeadlineExceeded) ||
		!errors.Is(res.Err, context.DeadlineExceeded) {
		return res
	}

	if len(res.Output) == 0 {
		res.Output = timeoutOutput(call.Name, d)
	}

	return res
}

// timeoutOutput renders the wrap's structured timeout form (the shipped
// {"error":…} convention; best-effort — a marshal failure leaves the result's
// Err as the sole signal).
func timeoutOutput(name string, d time.Duration) json.RawMessage {
	msg := fmt.Sprintf("toolexec: tool %s timed out after %dms", name, d.Milliseconds())

	out, err := json.Marshal(map[string]string{keyError: msg})
	if err != nil {
		return nil
	}

	return out
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
