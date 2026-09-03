package acpserve

// The 18-04 SessionStore adapter: the thin bridge from the acp seam to the
// internal/session surfaces (the 18-03 list engine + the tombstone marker
// writer). internal/acp deliberately does NOT import internal/session
// (25-D-13 keeps the frontend decoupled from the session kit seam — the
// replay.go precedent; the session package's own parity tests import acp,
// so a production acp→session import would be a test-time import cycle).
// The adapter therefore lives here, at the composition root, next to the
// ConfigSurface and the ask surfaces.

import (
	"errors"
	"fmt"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// sessionStoreAdapter adapts session.ListSessions + session.Tombstone to the
// acp.SessionStore seam. Stateless: the workspace dir arrives per call (the
// acp handler's resolved store workDir).
type sessionStoreAdapter struct{}

// ListSessions enumerates one page through the 18-03 engine, projecting the
// lean headers onto the acp-side copy. The engine's typed cursor rejection
// translates to the seam's ErrInvalidListCursor so the handler can map it to
// invalid-params.
func (sessionStoreAdapter) ListSessions(
	dir, cursor string, limit int,
) ([]acp.ListedSession, string, error) {
	headers, next, err := session.ListSessions(dir, cursor, limit)
	if err != nil {
		if errors.Is(err, session.ErrInvalidCursor) {
			return nil, "", fmt.Errorf("session/list %s: %w: %w", dir, acp.ErrInvalidListCursor, err)
		}

		return nil, "", fmt.Errorf("session/list %s: %w", dir, err)
	}

	out := make([]acp.ListedSession, len(headers))

	for i, h := range headers {
		out[i] = acp.ListedSession{
			SessionID:    h.SessionID,
			Title:        h.Title,
			TitlePresent: h.TitlePresent,
			LastActivity: h.LastActivity,
		}
	}

	return out, next, nil
}

// Tombstone delegates to the marker writer (D-07 — the one home of the
// tombstone write is internal/session/tombstone.go).
func (sessionStoreAdapter) Tombstone(dir, sessionID string) error {
	return session.Tombstone(dir, sessionID) //nolint:wrapcheck // thin delegation; the handler adds step context
}
