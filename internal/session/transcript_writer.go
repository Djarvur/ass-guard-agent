package session

import (
	"context"
	"log/slog"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
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
	// store is the capped audit body store (09-05, AUD-03/D-01): full
	// REDACTED bodies land here; the transcript line carries metadata + ref.
	// nil → metadata lines with the ref but no persisted body (degraded, never
	// broken — the hash exists via SummarizeRequest regardless).
	store *audit.BodyStore
}

// NewTranscriptWriter returns a TranscriptWriter over the given Manager + bus
// (+ optional body store). Callers MUST call Run(ctx) in a goroutine to start
// draining.
func NewTranscriptWriter(manager *Manager, bus *event.Bus, store *audit.BodyStore) *TranscriptWriter {
	return &TranscriptWriter{manager: manager, bus: bus, store: store}
}

// appendRequestShaped converts one RequestShaped event into the metadata-only
// index line (09-05): SummarizeRequest → Put (best-effort; a store failure is
// slog'd to stderr and the line keeps the ref — loud, never turn-fatal,
// Pitfall 10) → AppendRequestShaped.
//
//nolint:funcorder // helper beside the case that calls it
func (w *TranscriptWriter) appendRequestShaped(rs *event.RequestShaped) {
	meta, err := audit.SummarizeRequest(rs.VerbatimRequest)
	if err != nil {
		slog.Error("audit: summarize request failed (line written with zero fingerprint)",
			"error", err.Error())
	}

	if w.store != nil {
		_, putErr := w.store.Put(rs.VerbatimRequest)
		if putErr != nil {
			slog.Error("audit: body store Put failed (ref kept, body not persisted)",
				"error", putErr.Error(), "ref", meta.Ref)
		}
	}

	appErr := w.manager.AppendRequestShaped(rs.TurnID, rs.Profile, rs.Timestamp, meta)
	if appErr != nil {
		slog.Error("audit: AppendRequestShaped failed", "error", appErr.Error())
	}
}

// Run drains the subscribed event channels until ctx is cancelled. Each event
// is appended to the transcript via the matching Manager.Append*.
//
// ToolCall events are deliberately NOT written here: the Session is the SOLE
// writer of tool_call lines (its documented contract — the sync loop over
// resp.ToolCalls). Writing them here too duplicated every call into the
// transcript (the 08-09 duplicate-tool_use finding: every id exactly 2x in
// live transcripts; the Projector folds each line into the carried assistant
// batch, so the duplicates were a request-shape divergence).
//
//nolint:gocognit,cyclop,gocyclo,funlen // async writer complexity is inherent
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
				w.appendRequestShaped(&rs)
			}
		case e, ok := <-subs.chunks:
			if !ok {
				return
			}

			if c, ok := e.(event.AgentMessageChunk); ok {
				_ = w.manager.AppendAgentMessageChunk(c.TurnID, c.MessageID, c.Content)
			}
		case _, ok := <-subs.toolCalls:
			if !ok {
				return
			}

			// Drain-and-drop (see the comment above): the tool_call line is
			// written by the Session's sync loop; a second write here would
			// duplicate the call into every consumer of the transcript.
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
