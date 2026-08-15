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
// The line is recorded per mutating call / config-added entry as the AUDIT
// marker, and it resets the lean window for the NEXT turn's projection — never
// the producing turn's own mid-turn window (SESS-04, revised 2026-08-15 —
// between-turn reset; evidence: 08-09 / 08-08 T4 / corpus session 4440f5a7:
// the zcode corpus's rolling 64-message tail never resets on tool results).
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
