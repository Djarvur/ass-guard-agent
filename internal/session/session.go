package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"

	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/provider"
	"github.com/djarvur/ass-guard-agent/internal/toolcat"

	"github.com/djarvur/ass-guard-agent/internal/event"
)

// stubToolResult is the canned Phase-2 tool result (D-15 — execution stays
// stubbed; real execution lands in Phase 4). The transcript records it so the
// reconstruction is faithful even before tools are real.
var stubToolResult = json.RawMessage(`{"output":"stubbed in Phase 2 (real execution in Phase 4)"}`)

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
	Manager     *Manager
	Projector   *Projector
	Provider    provider.Provider
	Bus         *event.Bus
	Semaphore   *provider.Semaphore
	Profile     profile.Profile
	WorkDir     string
	SessionID   string

	// Catalog + configAdded drive boundary detection (wired in Plan 02-05).
	Catalog     *toolcat.Catalog
	ConfigAdded []string

	// toolExec is the (stubbed) tool executor; Plan 02-06 wraps it in a
	// RestrictedExecutor for subagents.
	toolExec toolcat.ToolExecutor

	turnCounter int64
}

// nextTurnID returns a monotonically-increasing turn id for this session.
func (s *Session) nextTurnID() string {
	n := atomic.AddInt64(&s.turnCounter, 1)
	return fmt.Sprintf("%s-turn-%03d", s.SessionID, n)
}

// Prompt runs the D-18 turn cycle for one user prompt: project the lean window,
// shape + send (capturer publishes RequestShaped), emit results, loop on
// tool_calls (stubbed execution), and return the stopReason. ctx cancellation
// aborts the turn and records a canceled line (D-16). A panic is recovered at
// the turn boundary and recorded as an investigate-and-fix-ready error line
// (PROJECT.md, D-13 for the parent).
func (s *Session) Prompt(ctx context.Context, userPrompt []ContentBlock) (stop string, err error) {
	turnID := s.nextTurnID()
	// Recover at the goroutine/turn boundary (D-13 parent-side): a panic becomes
	// an investigate-and-fix-ready error line + an error return (never a crash).
	defer func() {
		if r := recover(); r != nil {
			s.appendError(turnID, "session", fmt.Errorf("turn panic: %v", r), false)
			err = fmt.Errorf("session turn panic recovered: %v", r)
			stop = ""
		}
	}()

	if err := s.Manager.AppendUserMessage(turnID, userPrompt); err != nil {
		return "", err
	}

	const maxIterations = 16 // bound the tool loop (avoid runaway in stubs)
	for iter := 0; iter < maxIterations; iter++ {
		if err := ctx.Err(); err != nil {
			s.recordCanceled(turnID, "context cancelled before turn step")
			return "cancelled", nil
		}
		// Step 1: project the lean window (D-01/D-02).
		messages, err := s.Projector.Project(turnID)
		if err != nil {
			s.appendError(turnID, "projector", err, false)
			return "", fmt.Errorf("session turn project: %w", err)
		}
		// Step 2+3: shape + send. The provider's RequestCapturer publishes
		// RequestShaped to the bus (LOG-01); the async TranscriptWriter appends
		// request_shaped. The semaphore bounds outbound concurrency (PARA-04).
		if s.Semaphore != nil {
			if err := s.Semaphore.Acquire(ctx); err != nil {
				s.recordCanceled(turnID, "semaphore acquire cancelled")
				return "cancelled", nil
			}
		}
		resp, err := s.Provider.Send(ctx, s.Profile, messages)
		if s.Semaphore != nil {
			s.Semaphore.Release()
		}
		if err != nil {
			if ctx.Err() != nil {
				s.recordCanceled(turnID, "context cancelled during send")
				return "cancelled", nil
			}
			s.appendError(turnID, "provider", err, false)
			return "", fmt.Errorf("session turn send: %w", err)
		}
		// Step 4: emit tool_calls / usage to the bus (non-streaming in 02-02 —
		// the full response is one logical chunk; Plan 02-05 wires Stream).
		s.emitResponseEvents(turnID, resp)
		// Step 5: tool_calls → stub execute + loop.
		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				_ = s.Manager.AppendToolCall(turnID, tc.Name, tc.Name, tc.Input)
				s.publishToolCall(turnID, tc)
				out, _ := s.executeStub(ctx, tc)
				_ = s.Manager.AppendToolResult(turnID, tc.Name, out, false)
			}
			continue // loop to project again with the stub results
		}
		// Step 6: end_turn — append the assistant message + return stopReason.
		assistantText := extractAssistantText(resp)
		_ = s.Manager.AppendAssistantMessage(turnID, assistantText)
		return mapStopReason(resp.FinishReason), nil
	}
	s.appendError(turnID, "session", errors.New("tool loop exceeded max iterations"), true)
	return "", errors.New("session: tool loop exceeded max iterations")
}

// executeStub runs the stubbed tool (Phase-2 D-15). If a real toolExec is wired
// (Plan 02-06 RestrictedExecutor), it is called; otherwise the canned stub
// result is returned. Real execution lands in Phase 4.
func (s *Session) executeStub(ctx context.Context, tc provider.ToolCall) (json.RawMessage, error) {
	if s.toolExec != nil {
		if out, err := s.toolExec.Execute(ctx, tc.Name, tc.Input); err == nil {
			return out, nil
		}
	}
	return stubToolResult, nil
}

// emitResponseEvents publishes ToolCall + a single AgentMessageChunk for the
// non-streaming response (02-02). Plan 02-05 replaces this with per-chunk
// streaming events from Provider.Stream.
func (s *Session) emitResponseEvents(turnID string, resp provider.Response) {
	if s.Bus == nil {
		return
	}
	text := extractAssistantText(resp)
	if text != "" {
		s.Bus.Publish(event.AgentMessageChunk{TurnID: turnID, MessageID: turnID, Content: text})
	}
}

// publishToolCall publishes a ToolCall event for visibility (the ACP adapter
// forwards it to session/update in Plan 02-05).
func (s *Session) publishToolCall(turnID string, tc provider.ToolCall) {
	if s.Bus == nil {
		return
	}
	s.Bus.Publish(event.ToolCall{TurnID: turnID, Name: tc.Name, Input: tc.Input})
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
	case "end_turn", "stop":
		return "end_turn"
	case "tool_use":
		return "tool_use"
	case "max_tokens":
		return "max_tokens"
	case "":
		return "end_turn"
	default:
		return finish
	}
}

// extractAssistantText pulls the assistant text out of a non-streaming Response
// (best-effort parse of resp.Raw; Phase-1 Send captures it). Plan 02-05 uses
// Stream chunks instead.
func extractAssistantText(resp provider.Response) string {
	if len(resp.Raw) == 0 {
		return ""
	}
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resp.Raw, &msg); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range msg.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}
