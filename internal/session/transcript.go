package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const filePermOwner = 0o600

// Line-type discriminators for the append-only JSONL transcript (D-03/D-20).
// Each line is one structured JSON object with a `type` discriminator. The 15
// types cover every event the Session Core, the turn loop, and the engine emit.
const (
	TypeSessionStart      = "session_start"
	TypeUserMessage       = userMessageType
	TypeRequestShaped     = "request_shaped"
	TypeAgentMessageChunk = "agent_message_chunk"
	TypeAssistantMessage  = "assistant_message"
	TypeToolCall          = "tool_call"
	TypeToolResult        = "tool_result"
	TypeBoundary          = kindBoundary
	TypeSubagentDispatch  = "subagent_dispatch"
	TypeSubagentResult    = "subagent_result"
	TypeUsage             = "usage"
	TypeCanceled          = "canceled"
	TypeEngineDecision    = "engine_decision"
	TypeError             = "error"
	TypeSessionEnd        = "session_end"
)

// ContentBlock is one entry of a user/assistant message's content (mirrors the
// ACP content block shape). Phase 2 exercises text blocks.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Line is one JSONL transcript entry. It is a flat struct: every type uses the
// subset of fields it needs (the rest are omitted). This keeps the on-disk
// format one-line-per-event, human-greppable, and forward-compatible.
type Line struct {
	Type      string    `json:"type"`
	TurnID    string    `json:"turnID,omitempty"` //nolint:tagliatelle // on-disk format
	Timestamp time.Time `json:"timestamp"`

	// user_message / assistant_message / agent_message_chunk
	Content   json.RawMessage `json:"content,omitempty"`
	Text      string          `json:"text,omitempty"`
	MessageID string          `json:"messageID,omitempty"` //nolint:tagliatelle // on-disk format

	// request_shaped (the verbatim outgoing request, LOG-01 mimicry evidence)
	VerbatimRequest json.RawMessage `json:"verbatimRequest,omitempty"` //nolint:tagliatelle // on-disk format
	Profile         string          `json:"profile,omitempty"`

	// tool_call / tool_result
	ToolCallID string          `json:"toolCallID,omitempty"` //nolint:tagliatelle // on-disk format
	Name       string          `json:"name,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	IsError    bool            `json:"isError,omitempty"` //nolint:tagliatelle // on-disk format

	// boundary
	Cause      string `json:"cause,omitempty"`
	CommandRef string `json:"commandRef,omitempty"` //nolint:tagliatelle // on-disk format

	// error
	Component   string `json:"component,omitempty"`
	Message     string `json:"message,omitempty"`
	Recoverable bool   `json:"recoverable,omitempty"`
	Stack       string `json:"stack,omitempty"`

	// subagent
	ParentTurnID    string   `json:"parentTurnID,omitempty"`    //nolint:tagliatelle // on-disk format
	SubagentTurnID  string   `json:"subagentTurnID,omitempty"`  //nolint:tagliatelle // on-disk format
	RestrictedTools []string `json:"restrictedTools,omitempty"` //nolint:tagliatelle // on-disk format
	Result          string   `json:"result,omitempty"`

	// usage
	InputTokens  int64 `json:"inputTokens,omitempty"`  //nolint:tagliatelle // on-disk format
	OutputTokens int64 `json:"outputTokens,omitempty"` //nolint:tagliatelle // on-disk format
}

// selfGitignoreContent is the .ass-guard/.gitignore body (D-07): ignore
// everything except .gitignore itself.
const selfGitignoreContent = "*\n!.gitignore\n"

// openTranscript ensures dir/.ass-guard exists (with a self-gitignore), then
// opens the per-session JSONL file O_APPEND|O_CREATE|O_WRONLY mode 0600.
func openTranscript(dir, sessionID string) (*os.File, string, error) {
	storeDir := filepath.Join(dir, ".ass-guard")

	err := os.MkdirAll(storeDir, 0o755)
	if err != nil {
		return nil, "", fmt.Errorf("call: %w", err)
	}

	giPath := filepath.Join(storeDir, ".gitignore")

	_, err = os.Stat(giPath)
	if os.IsNotExist(err) {
		err = os.WriteFile(giPath, []byte(selfGitignoreContent), 0o644)
		if err != nil {
			return nil, "", fmt.Errorf("call: %w", err)
		}
	}

	fname := "transcript_" + sessionID + ".jsonl"
	path := filepath.Join(storeDir, fname)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePermOwner)
	if err != nil {
		return nil, "", fmt.Errorf("call: %w", err)
	}

	return f, path, nil
}
