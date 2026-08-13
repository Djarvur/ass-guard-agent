package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
	"github.com/Djarvur/ass-guard-agent/internal/toolexec"

	"github.com/Djarvur/ass-guard-agent/internal/event"
)

var errToolLoopExceededMax = errors.New("tool loop exceeded max iterations")
var errToolLoopExceeded = errors.New("session: tool loop exceeded max iterations")

// stubToolResult is the canned Phase-2 tool result (D-15 — execution stays
// stubbed; real execution lands in Phase 4). The transcript records it so the
// reconstruction is faithful even before tools are real.
// stubResultMsg is the canned Phase-2 tool-result message (D-15).
const stubResultMsg = `{"output":"stubbed in Phase 2 (real execution in Phase 4)"}`

var stubToolResult = json.RawMessage(stubResultMsg) //nolint:gochecknoglobals // immutable table

// Session is the Session Core (D-17): it owns the transcript Manager, the
// lean-window Projector, and the turn loop (D-18). It is the sole writer of
// session-structure lines (user/assistant/tool/boundary/canceled); streaming
// audit lines (request_shaped, chunks, usage) are written by the async
// TranscriptWriter (LOG-02).
//
// The Session takes the Phase-1 Shaper + Provider as injected dependencies.
// Phase-2 tool execution is stubbed (D-15); Plan 02-05 wires Provider.Stream +
// boundaries; Plan 02-06 wires subagent dispatch.
type Session struct {
	Manager   *Manager
	Projector *Projector
	Provider  provider.Provider
	Bus       *event.Bus
	Semaphore *provider.Semaphore
	Profile   profile.Profile
	WorkDir   string
	SessionID string

	// Catalog + configAdded drive boundary detection (wired in Plan 02-05).
	Catalog     *toolcat.Catalog
	ConfigAdded []string

	// toolExec is the (stubbed) tool executor; Plan 02-06 wraps it in a
	// RestrictedExecutor for subagents.
	toolExec toolcat.ToolExecutor

	// subagentRunner is the seam for Task/Agent dispatch (Plan 02-06). Nil →
	// defaultSubagentRunner (real nested loop). Tests inject a fake.
	subagentRunner subagentRunner

	turnCounter atomic.Int64
}

// nextTurnID returns a monotonically-increasing turn id for this session.
func (s *Session) nextTurnID() string { //nolint:funcorder // ordering groups related logic
	n := s.turnCounter.Add(1)

	return fmt.Sprintf("%s-turn-%03d", s.SessionID, n)
}

// Prompt runs the D-18 turn cycle for one user prompt: project the lean window,
// shape + send (capturer publishes RequestShaped), emit results, loop on
// tool_calls (stubbed execution), and return the stopReason. ctx cancellation
// aborts the turn and records a canceled line (D-16). A panic is recovered at
// the turn boundary and recorded as an investigate-and-fix-ready error line
// (PROJECT.md, D-13 for the parent).
//
//nolint:gocognit,cyclop,funlen,nonamedreturns // domain complexity; err used by defer
func (s *Session) Prompt(ctx context.Context, userPrompt []ContentBlock) (stop string, err error) {
	turnID := s.nextTurnID()
	// Recover at the goroutine/turn boundary (D-13 parent-side): a panic becomes
	// an investigate-and-fix-ready error line + an error return (never a crash).
	defer func() {
		if r := recover(); r != nil {
			//nolint:err113 // dynamic error message
			s.appendError(turnID, "session", fmt.Errorf("turn panic: %v", r), false)
			//nolint:err113 // dynamic error message
			err = fmt.Errorf("session turn panic recovered: %v", r)
			stop = ""
		}
	}()

	err = s.Manager.AppendUserMessage(turnID, userPrompt)
	if err != nil {
		return "", err
	}

	const maxIterations = 16 // bound the tool loop (avoid runaway in stubs)
	for range maxIterations {
		err = ctx.Err()
		if err != nil {
			s.recordCanceled(turnID, "context cancelled before turn step")

			return stopCancelled, nil
		}
		// Step 1: project the lean window (D-01/D-02).
		messages, err := s.Projector.Project(turnID)
		if err != nil {
			s.appendError(turnID, "projector", err, false)

			return "", fmt.Errorf("session turn project: %w", err)
		}
		// Step 2+3: shape + Stream. The provider's RequestCapturer publishes
		// RequestShaped to the bus (LOG-01); the async TranscriptWriter appends
		// request_shaped. The semaphore bounds outbound concurrency (PARA-04).
		// Plan 02-05 replaced Send with Stream: chunks are read from the stream
		// channel, emitted to the bus as AgentMessageChunk/ToolCall, and assembled
		// into the final Response (ACP-04 — NO full-turn buffering).
		if s.Semaphore != nil {
			err := s.Semaphore.Acquire(ctx)
			if err != nil {
				s.recordCanceled(turnID, "semaphore acquire cancelled")

				return stopCancelled, nil
			}
		}

		resp, textBuf, streamErr := s.streamAndEmit(ctx, turnID, messages)
		if s.Semaphore != nil {
			s.Semaphore.Release()
		}

		if streamErr != nil {
			if ctx.Err() != nil {
				s.recordCanceled(turnID, "context cancelled during stream")

				return stopCancelled, nil
			}

			s.appendError(turnID, "provider", streamErr, false)

			return "", fmt.Errorf("session turn stream: %w", streamErr)
		}
		// Step 5: tool_calls → execute + boundary + loop. Task/Agent tool calls
		// dispatch an isolated goroutine subagent (PARA-01) BEFORE the batch
		// (they are NOT routed through DispatchBatch). The remaining calls are
		// dispatched in one batch via toolexec.DispatchBatch (Phase-4 TOOL-04:
		// read-only concurrent, mutating strictly serial, arrival-order results).
		// A nil toolExec preserves the Phase-2 stub behavior (backward-compat).
		if len(resp.ToolCalls) > 0 { //nolint:nestif // tool-call processing is inherently nested
			// Record every model-selected tool_call first (the audit log shows
			// what the model asked for, independent of how it was executed).
			for _, tc := range resp.ToolCalls {
				_ = s.Manager.AppendToolCall(turnID, tc.Name, tc.Name, tc.Input)
			}
			// Subagent dispatch (Task/Agent) runs inline + is excluded from the
			// batch — it is a nested turn, not a catalog tool execution.
			var batchCalls []provider.ToolCall

			for _, tc := range resp.ToolCalls {
				if isSubagentTool(tc.Name) {
					result, derr := s.DispatchSubagent(ctx, turnID, tc.Name, extractSubagentPrompt(tc.Input), nil)
					if derr != nil {
						errJSON, mErr := json.Marshal(map[string]string{"error": derr.Error()})
						if mErr != nil {
							errJSON = []byte(`{"error":"marshal error failed"}`)
						}

						_ = s.Manager.AppendToolResult(turnID, tc.Name, errJSON, true)
					} else {
						_ = s.Manager.AppendToolResult(turnID, tc.Name, json.RawMessage(`"`+result+`"`), false)
					}
					// SESS-02/03 boundary (subagent tools are read-only; only
					// a config-added entry would fire).
					_ = s.MaybeAppendBoundary(tc.Name, tc.Name, turnID)

					continue
				}

				batchCalls = append(batchCalls, tc)
			}
			// Dispatch the remaining (non-subagent) calls in one batch. Results
			// are in arrival order so the transcript stays deterministic.
			results, batchErr := toolexec.DispatchBatch(ctx, s.toolExecOrStub(), s.Catalog, batchCalls)
			if batchErr != nil {
				// DispatchBatch surfaces per-call errors in results; a non-nil
				// top-level error is a cancelled-ctx path — record + continue so
				// the loop re-projects with whatever partial results we have.
				s.appendError(turnID, "toolexec", batchErr, true)
			}

			for _, res := range results {
				_ = s.Manager.AppendToolResult(turnID, res.Name, res.Output, res.IsError)
				// SESS-02/03: a mutating/config-added tool is a boundary. The
				// next projection resets the lean window.
				_ = s.MaybeAppendBoundary(res.Name, res.Name, turnID)
			}

			continue // loop to project again with the results
		}
		// Step 6: end_turn — append the assembled assistant message + stopReason.
		assistantText := textBuf
		_ = s.Manager.AppendAssistantMessage(turnID, assistantText)

		return mapStopReason(resp.FinishReason), nil
	}

	s.appendError(turnID, "session", errToolLoopExceededMax, true)

	return "", errToolLoopExceeded
}

// SetToolExecutor injects the real tool executor (Phase-4 TOOL-04/05 — a
// catalog-backed toolexec.RealExecutor constructed at startup in Plan 04-05).
// When not called, the session uses stubToolResult for every non-subagent tool
// (the Phase-2 backward-compatible behavior — real execution is opt-in).
func (s *Session) SetToolExecutor(tx toolcat.ToolExecutor) { s.toolExec = tx }

// stubExecutor returns the Phase-2 canned stub for every tool (D-15 — execution
// stays stubbed until SetToolExecutor wires the RealExecutor). It is used by
// toolExecOrStub when no real executor is set.
type stubExecutor struct{}

func (stubExecutor) Execute(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return stubToolResult, nil
}

// toolExecOrStub returns the real tool executor if set, else a stub executor
// that returns stubToolResult (Phase-2 backward-compat). DispatchBatch consumes
// this; a nil never reaches it.
func (s *Session) toolExecOrStub() toolcat.ToolExecutor { //nolint:ireturn // ToolExecutor abstraction
	if s.toolExec != nil {
		return s.toolExec
	}

	return stubExecutor{}
}

// executeStub is retained for Phase-2 callers/tests that drive one tool call
// streamAndEmit opens Provider.Stream, reads chunks until the channel closes,
// and emits each to the bus as AgentMessageChunk / ToolCall / UsageUpdate (D-18
// step 4 — ACP-04 streaming, NO full-turn buffering). It returns the assembled
// Response (tool_calls + FinishReason) + the concatenated assistant text. ctx
// cancellation closes the stream (the provider aborts the in-flight request).
func (s *Session) streamAndEmit(
	ctx context.Context, turnID string, messages []provider.Message,
) (provider.Response, string, error) {
	ch, err := s.Provider.Stream(ctx, &s.Profile, messages)
	if err != nil {
		return provider.Response{}, "", fmt.Errorf("call: %w", err)
	}

	var (
		resp provider.Response
		sb   strings.Builder
	)

	for chunk := range ch {
		err := ctx.Err()
		if err != nil {
			return resp, sb.String(), nil //nolint:nilerr // cancellation recorded as a transcript line
		}

		switch chunk.Type {
		case blockText:
			sb.WriteString(chunk.Text)

			if s.Bus != nil && chunk.Text != "" {
				s.Bus.Publish(event.AgentMessageChunk{
					TurnID: turnID, MessageID: turnID, Content: chunk.Text,
				})
			}
		case blockToolUse:
			if chunk.ToolCall != nil {
				tc := *chunk.ToolCall

				resp.ToolCalls = append(resp.ToolCalls, tc)
				if s.Bus != nil {
					s.Bus.Publish(event.ToolCall{
						TurnID: turnID, ToolCallID: chunk.ToolCallID,
						Name: tc.Name, Input: tc.Input,
					})
				}
			}
		case "usage":
			if chunk.Usage != nil && s.Bus != nil {
				s.Bus.Publish(event.UsageUpdate{
					TurnID:      turnID,
					InputTokens: chunk.Usage.InputTokens, OutputTokens: chunk.Usage.OutputTokens,
				})
			}
		case stopDone:
			resp.FinishReason = chunk.FinishReason
			resp.Raw = chunk.Raw
		}
	}
	// If ctx was cancelled mid-stream, surface that so the caller records a
	// canceled line + returns "cancelled" (D-16). The provider's goroutine has
	// already closed the channel (the HTTP request was aborted by ctx).
	err = ctx.Err()
	if err != nil {
		return resp, sb.String(), err
	}

	return resp, sb.String(), nil
}

// recordCanceled appends a canceled line (D-16).
func (s *Session) recordCanceled(turnID, reason string) {
	_ = s.Manager.AppendCanceled(turnID, now(), reason)
}

// appendError is the investigate-and-fix-ready error writer (PROJECT.md). The
// message is scrubbed by the Manager; the stack is captured here.
func (s *Session) appendError(turnID, component string, err error, recoverable bool) {
	if err == nil {
		return
	}

	_ = s.Manager.AppendError(turnID, component, err.Error(), nil, recoverable, string(debug.Stack()))
}

// mapStopReason maps the provider's FinishReason to an ACP stopReason. Unknown
// reasons default to "end_turn".
func mapStopReason(finish string) string {
	switch strings.ToLower(finish) {
	case stopEndTurn, "stop":
		return stopEndTurn
	case blockToolUse:
		return blockToolUse
	case "max_tokens":
		return "max_tokens"
	case "":
		return stopEndTurn
	default:
		return finish
	}
}
