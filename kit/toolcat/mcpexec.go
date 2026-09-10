package toolcat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MCPExecuter is the narrow interface an MCP-routing executor depends on. It is
// defined HERE (in toolcat) rather than by importing internal/mcp so the package
// graph stays cycle-free: the concrete *mcp.Host satisfies it structurally via
// Go's duck-typed interfaces (Host has the matching CallTool method).
type MCPExecuter interface {
	// CallTool routes an mcp__<server>__<tool> call to the MCP host and returns
	// the raw-JSON result.
	CallTool(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error)
}

// MCPExecutor wraps a ToolExecutor and routes mcp__* tool calls to an MCP host,
// falling through to the inner executor for every other tool. This is the exact
// parallel of RestrictedExecutor (wrap + intercept at the execution boundary):
// RestrictedExecutor enforces an allow-list; MCPExecutor routes by name prefix.
// It slots into the existing session.toolExec seam without Session changes
// (var _ ToolExecutor = (*MCPExecutor)(nil)).
type MCPExecutor struct {
	inner ToolExecutor
	mcp   MCPExecuter
}

// NewMCPExecutor wraps inner so mcp__* calls route to the MCP host and all other
// calls reach inner. A nil mcp host makes every mcp__* call return an error
// (there is nowhere to route); a nil inner panics on fall-through use (the
// session always wires a real or stub inner executor).
func NewMCPExecutor(inner ToolExecutor, mcp MCPExecuter) *MCPExecutor {
	return &MCPExecutor{inner: inner, mcp: mcp}
}

// Execute routes mcp__* names to the MCP host and delegates everything else to
// the inner executor unchanged (the model-facing catalog and the existing
// execution behavior are preserved).
func (e *MCPExecutor) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	if strings.HasPrefix(name, mcpToolPrefix) {
		if e.mcp == nil {
			//nolint:err113 // dynamic error message
			return nil, fmt.Errorf("tool %q has no mcp host configured", name)
		}

		return e.mcp.CallTool(ctx, name, input) //nolint:wrapcheck // thin routing
	}

	return e.inner.Execute(ctx, name, input) //nolint:wrapcheck // thin delegation
}

// mcpToolPrefix is the Claude-Code naming convention for MCP tools (D-04).
const mcpToolPrefix = "mcp__"

// compile-time interface check.
var _ ToolExecutor = (*MCPExecutor)(nil)
