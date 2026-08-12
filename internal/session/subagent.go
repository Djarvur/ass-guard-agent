package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// subagentRestrictedDefault is the default tool subset a Task/Agent subagent may
// use (D-10). The model sees the FULL catalog (parent mimicry); the
// RestrictedExecutor enforces this subset at runtime.
var subagentRestrictedDefault = []string{toolRead, "Glob", "Grep", toolWebFetch, "WebSearch"} //nolint:gochecknoglobals // immutable lookup table / default (cannot be a const)

// subagentRunner is the seam that runs the nested turn loop. Production uses the
// real nested loop; tests inject a fake to simulate panics / canned results.
type subagentRunner interface {
	Run(ctx context.Context, s *Session, subagentTurnID, parentTurnID, prompt string, restricted []string) (string, error)
}

// DispatchSubagent spawns an isolated goroutine running a nested turn loop with
// a restricted tool subset (PARA-01, D-10). It appends a subagent_dispatch line,
// waits for the subagent to complete (streamed progress flows via the bus tagged
// with parent-turn-id, PARA-02), and returns the final result. A panic is
// recovered at the goroutine boundary (D-13) → SubagentResult error +
// investigate-and-fix-ready error line; the process NEVER crashes.
func (s *Session) DispatchSubagent(ctx context.Context, parentTurnID, toolCallID, prompt string, restricted []string) (string, error) {
	if restricted == nil {
		restricted = subagentRestrictedDefault
	}

	subagentTurnID := s.nextTurnID()
	_ = s.Manager.AppendSubagentDispatch(parentTurnID, subagentTurnID, toolCallID, restricted)

	runner := s.subagentRunner
	if runner == nil {
		runner = defaultSubagentRunner{}
	}

	type outcome struct {
		result string
		err    error
	}

	resCh := make(chan outcome, 1)

	go func() {
		defer func() {
			r := recover()
			if r != nil {
				stack := debug.Stack()
				err := fmt.Errorf("subagent panic: %v", r)
				_ = s.Manager.AppendError(subagentTurnID, "subagent", err.Error(), nil, false, string(stack))

				if s.Bus != nil {
					s.Bus.Publish(event.SubagentResult{ParentTurnID: parentTurnID, ToolCallID: toolCallID, SubagentTurnID: subagentTurnID, Err: err})
				}

				resCh <- outcome{err: err}
			}
		}()

		result, err := runner.Run(ctx, s, subagentTurnID, parentTurnID, prompt, restricted)
		resCh <- outcome{result: result, err: err}
	}()

	select {
	case <-ctx.Done():
		err := ctx.Err()

		_ = s.Manager.AppendSubagentResult(parentTurnID, subagentTurnID, "", err.Error())
		if s.Bus != nil {
			s.Bus.Publish(event.SubagentResult{ParentTurnID: parentTurnID, ToolCallID: toolCallID, SubagentTurnID: subagentTurnID, Err: err})
		}

		return "", err
	case o := <-resCh:
		errMsg := ""
		if o.err != nil {
			errMsg = o.err.Error()
		}

		_ = s.Manager.AppendSubagentResult(parentTurnID, subagentTurnID, o.result, errMsg)
		if s.Bus != nil {
			s.Bus.Publish(event.SubagentResult{ParentTurnID: parentTurnID, ToolCallID: toolCallID, SubagentTurnID: subagentTurnID, Result: o.result, Err: o.err})
		}

		return o.result, o.err
	}
}

// defaultSubagentRunner runs the real nested turn loop. It uses the SAME manager
// (appending subagent-tagged lines), the SAME provider (via the semaphore), and
// a RestrictedExecutor over the session's toolExec. Streamed chunks/tool calls
// are published with ParentTurnID set (PARA-02).
type defaultSubagentRunner struct{}

func (defaultSubagentRunner) Run(ctx context.Context, s *Session, subagentTurnID, parentTurnID, prompt string, restricted []string) (string, error) {
	// Append the subagent's user message (subagent-tagged).
	_ = s.Manager.AppendUserMessage(subagentTurnID, []ContentBlock{{Type: blockText, Text: prompt}})

	// One nested provider call (Phase-2 stubs tool execution; a full subagent
	// tool-loop is Phase 4). The RestrictedExecutor wraps the session toolExec.
	var inner = s.toolExec
	if s.toolExec != nil {
		inner = toolcat.NewRestrictedExecutor(s.toolExec, restricted)
	}

	_ = inner // restricted executor is wired; real subagent tool-loop is Phase 4

	const maxIter = 8

	messages := []provider.Message{{Role: "user", Content: prompt}}

	for range maxIter {
		err := ctx.Err()
		if err != nil {
			return "", err
		}

		if s.Semaphore != nil {
			err := s.Semaphore.Acquire(ctx)
			if err != nil {
				return "", err
			}
		}

		resp, textBuf, streamErr := s.streamAndEmitTagged(ctx, subagentTurnID, parentTurnID, messages)
		if s.Semaphore != nil {
			s.Semaphore.Release()
		}

		if streamErr != nil {
			return textBuf, streamErr
		}
		// Tool calls inside the subagent: stub-execute via the restricted set.
		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				out, execErr := s.executeRestricted(ctx, tc, restricted)
				_ = s.Manager.AppendToolCall(subagentTurnID, tc.Name, tc.Name, tc.Input)

				if execErr != nil {
					errJSON, mErr := json.Marshal(map[string]string{"error": execErr.Error()})
					if mErr != nil {
						errJSON = []byte(`{"error":"marshal error failed"}`)
					}

					_ = s.Manager.AppendToolResult(subagentTurnID, tc.Name, errJSON, true)
				} else {
					_ = s.Manager.AppendToolResult(subagentTurnID, tc.Name, out, false)
				}
			}
			// Loop with the assistant turn included (simplified: re-send prompt).
			continue
		}

		return textBuf, nil
	}

	return "", errors.New("subagent: tool loop exceeded max iterations")
}

// streamAndEmitTagged is the subagent's streaming variant: it publishes events
// tagged with ParentTurnID (PARA-02 — streamed progress for parent visibility).
func (s *Session) streamAndEmitTagged(ctx context.Context, subagentTurnID, parentTurnID string, messages []provider.Message) (provider.Response, string, error) {
	ch, err := s.Provider.Stream(ctx, s.Profile, messages)
	if err != nil {
		return provider.Response{}, "", err
	}

	var (
		resp provider.Response
		sb   strings.Builder
	)

	for chunk := range ch {
		err := ctx.Err()
		if err != nil {
			return resp, sb.String(), err
		}

		switch chunk.Type {
		case blockText:
			sb.WriteString(chunk.Text)

			if s.Bus != nil && chunk.Text != "" {
				s.Bus.Publish(event.AgentMessageChunk{TurnID: subagentTurnID, MessageID: subagentTurnID, Content: chunk.Text})
			}
		case blockToolUse:
			if chunk.ToolCall != nil {
				resp.ToolCalls = append(resp.ToolCalls, *chunk.ToolCall)
			}
		case stopDone:
			resp.FinishReason = chunk.FinishReason
		}
	}

	_ = parentTurnID // events are tagged via the SubagentResult; chunks use TurnID

	return resp, sb.String(), nil
}

// executeRestricted runs a tool via the restricted executor (D-10). If no
// toolExec is wired, returns the canned stub.
func (s *Session) executeRestricted(ctx context.Context, tc provider.ToolCall, restricted []string) (json.RawMessage, error) {
	if s.toolExec == nil {
		return stubToolResult, nil
	}

	re := toolcat.NewRestrictedExecutor(s.toolExec, restricted)

	return re.Execute(ctx, tc.Name, tc.Input)
}

// isSubagentTool reports whether the tool name dispatches a subagent (PARA-01).
func isSubagentTool(name string) bool {
	return name == toolTask || name == "Agent"
}

// extractSubagentPrompt pulls the prompt field from a Task/Agent tool-call input
// (best-effort; returns a default if absent).
func extractSubagentPrompt(input json.RawMessage) string {
	if len(input) == 0 {
		return subagentTaskPrompt
	}

	var m map[string]any

	err := json.Unmarshal(input, &m)
	if err != nil {
		return subagentTaskPrompt
	}

	if p, ok := m["prompt"].(string); ok && p != "" {
		return p
	}

	if d, ok := m["description"].(string); ok && d != "" {
		return d
	}

	return subagentTaskPrompt
}
