package session

import (
	"context"

	"github.com/Djarvur/ass-guard-agent/internal/event"
)

// TranscriptWriter is the async bus consumer (LOG-02) and the ONE writer for the
// session-path audit (D-20 — audit log = transcript, one artifact). It
// subscribes to ALL 7 event kinds on the bus and appends each to the transcript
// via the Manager. It runs as a goroutine (Run(ctx)) select-looping on the
// subscribed channels; a slow writer never blocks producers because the bus
// channels are bounded (D-05) and the writer drains independently.
//
// The Manager redacts per-line (LOG-03) — the writer does not re-redact. Within
// a session there is exactly ONE writer (this one) and ONE artifact (the
// transcript): no two-writer drift. (The Phase-1 AuditLogger in internal/audit
// is the NON-SESSION tracer-CLI LOG-01 path only; it is not used in the session
// path, so D-20's one-writer property holds for sessions.)
type TranscriptWriter struct {
	manager *Manager
	bus     *event.Bus
}

// NewTranscriptWriter returns a TranscriptWriter over the given Manager + bus.
// Callers MUST call Run(ctx) in a goroutine to start draining.
func NewTranscriptWriter(manager *Manager, bus *event.Bus) *TranscriptWriter {
	return &TranscriptWriter{manager: manager, bus: bus}
}

// Run drains all 7 subscribed event channels until ctx is cancelled. Each event
// is appended to the transcript via the matching Manager.Append*.
func (w *TranscriptWriter) Run(ctx context.Context) {
	subs := w.subscribeAll()

	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-subs.requestShaped:
			if !ok {
				return
			}

			if rs, ok := e.(event.RequestShaped); ok {
				_ = w.manager.AppendRequestShaped(rs.TurnID, rs.VerbatimRequest, rs.Profile, rs.Timestamp)
			}
		case e, ok := <-subs.chunks:
			if !ok {
				return
			}

			if c, ok := e.(event.AgentMessageChunk); ok {
				_ = w.manager.AppendAgentMessageChunk(c.TurnID, c.MessageID, c.Content)
			}
		case e, ok := <-subs.toolCalls:
			if !ok {
				return
			}

			if tc, ok := e.(event.ToolCall); ok {
				_ = w.manager.AppendToolCall(tc.TurnID, tc.ToolCallID, tc.Name, tc.Input)
			}
		case e, ok := <-subs.usage:
			if !ok {
				return
			}

			if u, ok := e.(event.UsageUpdate); ok {
				_ = w.manager.AppendUsage(u.TurnID, u.InputTokens, u.OutputTokens)
			}
		case e, ok := <-subs.boundaries:
			if !ok {
				return
			}

			if b, ok := e.(event.Boundary); ok {
				_ = w.manager.AppendBoundary(b.Cause, b.CommandRef, b.TurnID)
			}
		case e, ok := <-subs.subagentResults:
			if !ok {
				return
			}

			if sr, ok := e.(event.SubagentResult); ok {
				msg := ""
				if sr.Err != nil {
					msg = sr.Err.Error()
				}

				_ = w.manager.AppendSubagentResult(sr.ParentTurnID, sr.SubagentTurnID, sr.Result, msg)
			}
		}
	}
}

// twSubs bundles the subscribed channels for all 7 event kinds.
type twSubs struct {
	requestShaped   <-chan event.Event
	chunks          <-chan event.Event
	toolCalls       <-chan event.Event
	usage           <-chan event.Event
	boundaries      <-chan event.Event
	subagentResults <-chan event.Event
}

// subscribeAll subscribes the TranscriptWriter to every event kind and returns
// the channel bundle.
func (w *TranscriptWriter) subscribeAll() twSubs {
	return twSubs{
		requestShaped:   w.bus.Subscribe("RequestShaped", event.BufRequestShaped),
		chunks:          w.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk),
		toolCalls:       w.bus.Subscribe("ToolCall", event.BufToolCall),
		usage:           w.bus.Subscribe("UsageUpdate", event.BufUsageUpdate),
		boundaries:      w.bus.Subscribe("Boundary", event.BufBoundary),
		subagentResults: w.bus.Subscribe("SubagentResult", event.BufSubagentResult),
	}
}
