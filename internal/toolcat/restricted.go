package toolcat

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolExecutor is the runtime tool-execution interface (TOOL-04/05 forward-
// compatible). Phase 1 leaves tool execution stubbed (D-15); Phase 2's subagent
// dispatch wraps a ToolExecutor in a RestrictedExecutor to enforce the runtime
// subset (D-10). Real execution lands in Phase 4.
type ToolExecutor interface {
	// Execute runs the named tool with the raw-JSON arguments and returns the
	// raw-JSON result (or an error).
	Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error)
}

// RestrictedExecutor wraps a ToolExecutor and enforces an allow-list at the
// execution boundary (D-10). The model-facing catalog is NOT modified — the
// model sees the full tool set (parent-side mimicry preserved); this executor
// enforces which tools can actually run in a subagent's restricted context. A
// disallowed tool returns a "not available" error so the model adapts (selects
// an allowed tool or reports it cannot proceed).
type RestrictedExecutor struct {
	inner   ToolExecutor
	allowed map[string]struct{}
}

// NewRestrictedExecutor wraps inner so only the named tools in allowed may run.
// The allowed list is the subagent's runtime subset (e.g. Read/Glob/Grep for a
// research-only subagent). An empty allowed list blocks every tool.
func NewRestrictedExecutor(inner ToolExecutor, allowed []string) *RestrictedExecutor {
	m := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		m[name] = struct{}{}
	}

	return &RestrictedExecutor{inner: inner, allowed: m}
}

// Execute delegates to inner only when name is in the allowed subset; otherwise
// it returns a "not available" error WITHOUT calling inner (the model adapts).
func (r *RestrictedExecutor) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	if _, ok := r.allowed[name]; !ok {
		//nolint:err113 // dynamic error message
		return nil, fmt.Errorf("tool %q is not available in this subagent context", name)
	}

	return r.inner.Execute(ctx, name, input) //nolint:wrapcheck // thin delegation
}

// compile-time interface check.
var _ ToolExecutor = (*RestrictedExecutor)(nil)
