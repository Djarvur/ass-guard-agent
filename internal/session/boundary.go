package session

import (
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// MaybeAppendBoundary checks whether the named tool is a session boundary
// (SESS-02/03) via toolcat.IsBoundary and, if so, appends a boundary line to the
// transcript (D-08). The cause distinguishes catalog-mutating tools
// ("mutating-command:<Tool>") from config-added boundaries ("config-added:<Tool>")
// so a human investigating the transcript can tell why a boundary fired.
//
// The NEXT projection's ReadLastBoundary finds this line and resets the lean
// window to the lean seed (SESS-04).
func (s *Session) MaybeAppendBoundary(toolName, toolCallID, turnID string) error {
	if s.Catalog == nil {
		return nil // boundary detection not configured (tracer mode)
	}

	if !toolcat.IsBoundary(toolName, s.Catalog, s.ConfigAdded) {
		return nil
	}

	cause := "config-added:" + toolName
	if isCatalogMutating(s.Catalog, toolName) {
		cause = "mutating-command:" + toolName
	}

	return s.Manager.AppendBoundary(cause, toolCallID, turnID)
}

// isCatalogMutating reports whether the tool is declared mutating in the catalog
// (vs a config-added boundary on a read-only tool).
func isCatalogMutating(c *toolcat.Catalog, name string) bool {
	if c == nil {
		return false
	}

	t, ok := c.Get(name)

	return ok && t.IsMutating()
}
