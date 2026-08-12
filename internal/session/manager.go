package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"sync"
	"time"
)

// Redactor is the per-line redaction interface the Manager calls before writing
// each transcript line (LOG-03). internal/redact satisfies it.
type Redactor interface {
	Redact(line []byte) ([]byte, error)
	ScrubError(err error) string
}

// Manager is the SOLE owner of the transcript file (SESS-06). Every append goes
// through its mutex-guarded Append* API; concurrent appends never corrupt a line.
// Every Append* marshals the line, redacts the bytes via the injected Redactor
// (LOG-03), then writes under the mutex. The transcript is the ONE audit
// artifact per D-20 (audit log = transcript).
type Manager struct {
	f        *os.File
	path     string
	mu       sync.Mutex
	redactor Redactor
}

// NewManager opens (or creates) the per-session transcript under dir/.ass-guard/
// and returns the sole-owner Manager.
func NewManager(dir, sessionID string, red Redactor) (*Manager, error) {
	if red == nil {
		return nil, errors.New("session: NewManager requires a non-nil Redactor")
	}

	f, path, err := openTranscript(dir, sessionID)
	if err != nil {
		return nil, err
	}

	return &Manager{f: f, path: path, redactor: red}, nil
}

// Path returns the transcript file path (for tests / inspection).
func (m *Manager) Path() string { return m.path }

// Close closes the underlying file. After Close, Append* errors.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.f == nil {
		return nil
	}

	err := m.f.Close()
	m.f = nil

	return err
}

// appendLine marshals the line, redacts it, appends a newline, and writes it
// under the mutex. Redaction happens BEFORE the write (LOG-03).
func (m *Manager) appendLine(line Line) error {
	raw, err := json.Marshal(line)
	if err != nil {
		return err
	}

	red, err := m.redactor.Redact(raw)
	if err != nil {
		red = []byte(m.redactor.ScrubError(errors.New(string(raw))))
	}

	red = append(red, '\n')

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.f == nil {
		return errors.New("session: manager closed")
	}

	_, err = m.f.Write(red)

	return err
}

func now() time.Time { return time.Now().UTC() }

// AppendSessionStart records the session_start line.
func (m *Manager) AppendSessionStart(sessionID string) error {
	return m.appendLine(Line{Type: TypeSessionStart, Timestamp: now(), Text: sessionID})
}

// AppendSessionEnd records the session_end line.
func (m *Manager) AppendSessionEnd() error {
	return m.appendLine(Line{Type: TypeSessionEnd, Timestamp: now()})
}

// AppendUserMessage records a user message (the prompt content blocks).
func (m *Manager) AppendUserMessage(turnID string, content []ContentBlock) error {
	raw, _ := json.Marshal(content)

	return m.appendLine(Line{Type: TypeUserMessage, TurnID: turnID, Timestamp: now(), Content: raw})
}

// AppendAssistantMessage records the final assembled assistant text for a turn.
func (m *Manager) AppendAssistantMessage(turnID, text string) error {
	return m.appendLine(Line{Type: TypeAssistantMessage, TurnID: turnID, Timestamp: now(), Text: text})
}

// AppendAgentMessageChunk records one streamed chunk (for reconstruction).
func (m *Manager) AppendAgentMessageChunk(turnID, messageID, text string) error {
	return m.appendLine(Line{Type: TypeAgentMessageChunk, TurnID: turnID, Timestamp: now(), MessageID: messageID, Text: text})
}

// AppendRequestShaped records the verbatim shaped outgoing request (LOG-01),
// redacted per-line (LOG-03).
func (m *Manager) AppendRequestShaped(turnID string, verbatim json.RawMessage, profileName string, ts time.Time) error {
	return m.appendLine(Line{Type: TypeRequestShaped, TurnID: turnID, Timestamp: ts, VerbatimRequest: verbatim, Profile: profileName})
}

// AppendToolCall records a model-selected tool invocation.
func (m *Manager) AppendToolCall(turnID, toolCallID, name string, input json.RawMessage) error {
	return m.appendLine(Line{Type: TypeToolCall, TurnID: turnID, Timestamp: now(), ToolCallID: toolCallID, Name: name, Input: input})
}

// AppendToolResult records a tool's result (stubbed in Phase 2; real in Phase 4).
func (m *Manager) AppendToolResult(turnID, toolCallID string, output json.RawMessage, isError bool) error {
	return m.appendLine(Line{Type: TypeToolResult, TurnID: turnID, Timestamp: now(), ToolCallID: toolCallID, Output: output, IsError: isError})
}

// AppendBoundary records a context-boundary (a mutating command completed,
// D-08). The next projection resets the lean window.
func (m *Manager) AppendBoundary(cause, commandRef, turnID string) error {
	return m.appendLine(Line{Type: TypeBoundary, TurnID: turnID, Timestamp: now(), Cause: cause, CommandRef: commandRef})
}

// AppendCanceled records that a turn was cancelled (D-16).
func (m *Manager) AppendCanceled(turnID string, ts time.Time, reason string) error {
	return m.appendLine(Line{Type: TypeCanceled, TurnID: turnID, Timestamp: ts, Text: reason})
}

// AppendError records an investigate-and-fix-ready error line (PROJECT.md). The
// message is scrubbed (LOG-03); component + recoverable + stack are preserved.
func (m *Manager) AppendError(turnID, component, message string, inputs json.RawMessage, recoverable bool, stack string) error {
	scrubbed := m.redactor.ScrubError(errors.New(message))

	return m.appendLine(Line{
		Type: TypeError, TurnID: turnID, Timestamp: now(),
		Component: component, Message: scrubbed, Recoverable: recoverable,
		Stack: stack, Input: inputs,
	})
}

// AppendSubagentDispatch records a Task/Agent subagent dispatch.
func (m *Manager) AppendSubagentDispatch(parentTurnID, subagentTurnID, toolCallID string, restricted []string) error {
	return m.appendLine(Line{
		Type: TypeSubagentDispatch, TurnID: subagentTurnID, Timestamp: now(),
		ParentTurnID: parentTurnID, SubagentTurnID: subagentTurnID,
		ToolCallID: toolCallID, RestrictedTools: restricted,
	})
}

// AppendSubagentResult records a subagent's final result.
func (m *Manager) AppendSubagentResult(parentTurnID, subagentTurnID, result, errMsg string) error {
	return m.appendLine(Line{
		Type: TypeSubagentResult, TurnID: subagentTurnID, Timestamp: now(),
		ParentTurnID: parentTurnID, SubagentTurnID: subagentTurnID,
		Result: result, Message: errMsg,
	})
}

// AppendUsage records a token-usage update.
func (m *Manager) AppendUsage(turnID string, input, output int64) error {
	return m.appendLine(Line{Type: TypeUsage, TurnID: turnID, Timestamp: now(), InputTokens: input, OutputTokens: output})
}

// AppendEngineDecision records the unified engine's verdict for one turn
// (Phase-4 ENG-02 — the single provenance-tagged stream). The line type constant
// TypeEngineDecision is already reserved in transcript.go. action is the
// engine's decision vocabulary (nothing/continue/hook/ask/wait); signal is the
// matched signal ("text:<id>", "tool:<id>", or "unmatched"); reason is the
// investigate-and-fix-ready human note (PROJECT.md). The audit log therefore
// proves the structural-safety property: unmatched output triggers nothing.
func (m *Manager) AppendEngineDecision(turnID, action, signal, reason string) error {
	return m.appendLine(Line{
		Type:      TypeEngineDecision,
		TurnID:    turnID,
		Timestamp: now(),
		Name:      action,
		Input:     json.RawMessage(`"` + signal + `"`),
		Text:      reason,
	})
}

// ReadAll reads every line from the transcript in append order.
func (m *Manager) ReadAll() ([]Line, error) {
	m.mu.Lock()
	fname := m.path
	m.mu.Unlock()

	return readTranscriptFile(fname)
}

// ReadLastBoundary returns the most recent boundary line, or (nil, nil) if none.
func (m *Manager) ReadLastBoundary() (*Line, error) {
	lines, err := m.ReadAll()
	if err != nil {
		return nil, err
	}

	for _, v := range slices.Backward(lines) {
		if v.Type == TypeBoundary {
			return &v, nil
		}
	}

	return nil, nil //nolint:nilnil // nil result signals "no boundary line found"; caller checks for nil
}

// ReadSince returns the lines appended AFTER the last line carrying the given
// turnID. If turnID is empty or not found, returns all lines.
func (m *Manager) ReadSince(turnID string) ([]Line, error) {
	lines, err := m.ReadAll()
	if err != nil {
		return nil, err
	}

	if turnID == "" {
		return lines, nil
	}

	lastIdx := -1

	for i, l := range lines {
		if l.TurnID == turnID {
			lastIdx = i
		}
	}

	if lastIdx < 0 {
		return lines, nil
	}

	return lines[lastIdx+1:], nil
}

// readTranscriptFile parses every JSONL line in path into a Line slice.
func readTranscriptFile(path string) ([]Line, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	var out []Line

	br := bufio.NewReader(f)

	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) == 0 && rerr != nil {
			break
		}

		for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
			line = line[:len(line)-1]
		}

		if len(line) == 0 {
			if rerr != nil {
				break
			}

			continue
		}

		var l Line

		jerr := json.Unmarshal(line, &l)
		if jerr != nil {
			continue
		}

		out = append(out, l)

		if rerr != nil {
			break
		}
	}

	return out, nil
}
