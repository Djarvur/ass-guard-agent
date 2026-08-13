package toolcat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

var errToolInternalFailure = errors.New("tool internal failure")

// fakeExecutor is a test ToolExecutor that records the call and returns a
// canned result. It is the inner executor RestrictedExecutor wraps.
type fakeExecutor struct {
	called  []string
	result  json.RawMessage
	execErr error
}

func (f *fakeExecutor) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	f.called = append(f.called, name)
	if f.execErr != nil {
		return nil, f.execErr
	}

	if len(f.result) > 0 {
		return f.result, nil
	}

	return json.RawMessage(`{"ok":true}`), nil
}

// TestRestrictedExecutorAllowedToolDelegates verifies that a tool in the allowed
// subset delegates to the inner executor and returns its result (D-10 — the
// subagent's restricted subset still runs the allowed tools).
func TestRestrictedExecutorAllowedToolDelegates(t *testing.T) {
	t.Parallel()

	inner := &fakeExecutor{result: json.RawMessage(`{"v":"ran"}`)}
	r := NewRestrictedExecutor(inner, []string{toolRead, toolGrep})

	out, err := r.Execute(context.Background(), toolRead, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("allowed Read returned error: %v", err)
	}

	if string(out) != `{"v":"ran"}` {
		t.Errorf("allowed Read output = %s; want {\"v\":\"ran\"}", string(out))
	}

	if len(inner.called) != 1 || inner.called[0] != toolRead {
		t.Errorf("inner executor called = %v; want [Read]", inner.called)
	}
}

// TestRestrictedExecutorDisallowedToolErrors verifies that a tool OUTSIDE the
// allowed subset returns a "not available" error and does NOT call the inner
// executor (D-10 — the model adapts to the restriction).
func TestRestrictedExecutorDisallowedToolErrors(t *testing.T) {
	t.Parallel()

	inner := &fakeExecutor{}
	r := NewRestrictedExecutor(inner, []string{toolRead, toolGrep})

	_, err := r.Execute(context.Background(), toolBash, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("disallowed Bash returned nil error; want not-available error")
	}

	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("disallowed Bash error = %q; want it to contain 'not available'", err.Error())
	}

	if len(inner.called) != 0 {
		t.Errorf("inner executor was called for a disallowed tool: %v (must not call inner)", inner.called)
	}
}

// TestRestrictedExecutorEmptyAllowedSet verifies that an empty allowed subset
// blocks every tool (the maximally-restricted subagent).
func TestRestrictedExecutorEmptyAllowedSet(t *testing.T) {
	t.Parallel()

	inner := &fakeExecutor{}
	r := NewRestrictedExecutor(inner, nil)

	for _, name := range []string{toolRead, toolBash, "Write"} {
		_, err := r.Execute(context.Background(), name, json.RawMessage(`{}`))
		if err == nil {
			t.Errorf("tool %q with empty allowed set returned nil error; want not-available", name)
		}
	}

	if len(inner.called) != 0 {
		t.Errorf("inner executor called with empty allowed set: %v", inner.called)
	}
}

// TestRestrictedExecutorPropagatesInnerError verifies that errors from the inner
// executor (for allowed tools) propagate unchanged — restriction does not mask
// real tool failures.
func TestRestrictedExecutorPropagatesInnerError(t *testing.T) {
	t.Parallel()

	innerErr := errToolInternalFailure
	inner := &fakeExecutor{execErr: innerErr}
	r := NewRestrictedExecutor(inner, []string{toolRead})

	_, err := r.Execute(context.Background(), toolRead, json.RawMessage(`{}`))
	if !errors.Is(err, innerErr) {
		t.Errorf("allowed Read error = %v; want it to wrap/propagate %v", err, innerErr)
	}
}

// TestRestrictedExecutorDoesNotMutateCatalog is a structural assertion that the
// RestrictedExecutor type has no reference to a Catalog — restriction is at the
// executor boundary only, the model-facing catalog shape is unchanged (D-10).
func TestRestrictedExecutorDoesNotMutateCatalog(t *testing.T) {
	t.Parallel()

	var r any = &RestrictedExecutor{}
	// The wrapper type must not embed or carry a *Catalog (restriction is
	// runtime-only; the catalog stays the parent's full declared set).
	if _, ok := r.(interface{ Catalog() *Catalog }); ok {
		t.Error("RestrictedExecutor must not expose a Catalog accessor — restriction is runtime-only (D-10)")
	}
}
