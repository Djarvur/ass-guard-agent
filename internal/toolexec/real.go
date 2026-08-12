package toolexec

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// queryArgs is the minimal shape RealExecutor extracts from a WebSearch input
// to forward the query to the configured Backend.
type queryArgs struct {
	Query string `json:"query"`
}

// urlArgs is the minimal shape RealExecutor extracts from a WebFetch input to
// forward the URL to the configured Backend.
type urlArgs struct {
	URL string `json:"url"`
}

// RealExecutor implements toolcat.ToolExecutor over the catalog (TOOL-04/05).
// WebSearch + WebFetch delegate to the configured Backend (swappable, D-22);
// every other tool calls its catalog Tool.Execute; an unknown tool returns a
// structured error. The Session is wired with a RealExecutor at startup (Plan
// 04-05 via SetToolExecutor); DispatchBatch drives it per turn-step.
type RealExecutor struct {
	Catalog  *toolcat.Catalog
	Backends map[string]Backend // keyed by tool name ("WebSearch", "WebFetch")
	Log      *slog.Logger
}

// Execute runs the named tool. WebSearch/WebFetch route to the configured
// Backend; everything else looks the tool up in the catalog + calls its
// Execute. A catalog entry whose Execute is nil returns a canned
// "not implemented" structured payload (forward-compat — the catalog schema is
// authoritative even before the impl lands).
func (r *RealExecutor) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	if r == nil {
		return nil, ErrNoExecutor
	}
	// Swappable backends first (D-22).
	if name == "WebSearch" {
		if be, ok := r.Backends["WebSearch"]; ok {
			return be.Search(ctx, extractQuery(input))
		}
	}

	if name == toolWebFetch {
		if be, ok := r.Backends[toolWebFetch]; ok {
			return be.Fetch(ctx, extractURL(input))
		}
	}
	// Catalog-driven tools.
	if r.Catalog == nil {
		return nil, fmt.Errorf("toolexec: tool %q not executable (no catalog)", name)
	}

	tool, ok := r.Catalog.Get(name)
	if !ok {
		return nil, fmt.Errorf("toolexec: tool %q not in catalog", name)
	}

	if tool.Execute == nil {
		// Forward-compat: the schema is authoritative even before the impl.
		return json.RawMessage(`{"error":"tool ` + name +
			` has no implementation yet (catalog schema authoritative)"}`), nil
	}

	return tool.Execute(ctx, input)
}

// extractQuery best-effort parses a WebSearch input's `query` field. An
// unparseable input yields the empty string (the backend reports the error).
func extractQuery(input json.RawMessage) string {
	var q queryArgs

	err := json.Unmarshal(input, &q)
	if err != nil {
		return ""
	}

	return q.Query
}

// extractURL best-effort parses a WebFetch input's `url` field.
func extractURL(input json.RawMessage) string {
	var u urlArgs

	err := json.Unmarshal(input, &u)
	if err != nil {
		return ""
	}

	return u.URL
}

// compile-time interface checks.
var (
	_ toolcat.ToolExecutor = (*RealExecutor)(nil)
	_ Backend              = (*HTTPBackend)(nil)
)
