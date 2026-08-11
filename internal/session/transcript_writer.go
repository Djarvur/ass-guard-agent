package session

import (
	"context"

	"github.com/djarvur/ass-guard-agent/internal/event"
)

// TranscriptWriter is the async bus consumer (LOG-02). It subscribes to the
// streaming/audit event kinds on the bus and appends each to the transcript via
// the Manager. It runs as a goroutine (Run(ctx)) select-looping on the
// subscribed channels; a slow writer does NOT block producers because the bus
// channels are bounded (D-05) and the writer drains independently.
//
// In Plan 02-02 the writer subscribes to RequestShaped (the LOG-01 verbatim
// request). Plan 02-07 folds the Phase-1 AuditLogger into this writer (D-20 —
// one artifact, one writer) and subscribes the full event set.
type TranscriptWriter struct {
	manager *Manager
	bus     *event.Bus
}

// NewTranscriptWriter subscribes w to the bus's audit event kinds and returns
// it. Callers MUST call Run(ctx) in a goroutine to start draining.
func NewTranscriptWriter(manager *Manager, bus *event.Bus) *TranscriptWriter {
	return &TranscriptWriter{manager: manager, bus: bus}
}

// Run drains the subscribed event channels until ctx is cancelled. Each event
// is appended to the transcript via the matching Manager.Append* (the Manager
// redacts per-line, LOG-03 — the writer does not re-redact).
func (w *TranscriptWriter) Run(ctx context.Context) {
	requests := w.bus.Subscribe("RequestShaped", event.BufRequestShaped)
	chunks := w.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	usage := w.bus.Subscribe("UsageUpdate", event.BufUsageUpdate)
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-requests:
			if !ok {
				return
			}
			if rs, ok := e.(event.RequestShaped); ok {
				_ = w.manager.AppendRequestShaped(rs.TurnID, rs.VerbatimRequest, rs.Profile, rs.Timestamp)
			}
		case e, ok := <-chunks:
			if !ok {
				return
			}
			if c, ok := e.(event.AgentMessageChunk); ok {
				_ = w.manager.AppendAgentMessageChunk(c.TurnID, c.MessageID, c.Content)
			}
		case e, ok := <-usage:
			if !ok {
				return
			}
			if u, ok := e.(event.UsageUpdate); ok {
				_ = w.manager.AppendUsage(u.TurnID, u.InputTokens, u.OutputTokens)
			}
		}
	}
}
