package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const filePermOwner = 0o600
const dirPerm = 0o750
const filePermDefault = 0o600

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

	// TypeCommandProvenance records WHICH discovered command file answered a
	// slash-command invocation (08-04 / CMD-05): the typed command + source
	// path + args as metadata NEXT TO the expanded user message (D-02 — the
	// user_message line holds the expanded body; provenance disambiguates
	// typed-vs-expanded on replay).
	TypeCommandProvenance = "command_provenance"

	// TypeAskSuspended records an AskUserQuestion suspension (12-01, ACP-01):
	// the turn ENDED at the ask (no tool result for the pending callID — the
	// operator's reply or the D-01 timer appends it and resumes the SAME turn).
	// The line carries the pending toolCallID + the surfaced questions payload;
	// the engine_decision line (action=ask) records the decision-level view.
	// The Projector skips this line type (an audit marker, like boundary).
	TypeAskSuspended = "ask_suspended"

	// TypeRawThinking records one RAW provider thinking block (Phase 16,
	// D-20/D-23): the payload is the provider's own JSON bytes carried
	// verbatim in Content (json.RawMessage — never re-serialized) plus the
	// provider model attribution in Model (Phase 21 / PAR-05 provenance
	// rendering). The line takes the UNREDACTED append path — the Redactor
	// is never invoked on thinking bytes (counting-fake test pins zero
	// calls). The exemption is TYPE-SCOPED to this kind and must never
	// widen to tool/command/user content (T-16-04).
	TypeRawThinking = "raw_thinking"

	// TypeLocalCommand records a full local-command invocation record
	// (D-22): the command key (Name) + the VERBATIM typed args (Args —
	// never re-quoted or normalized) + the resolution-source chain in
	// resolution order (SourceChain — builtin → skills → agents → file per
	// CMDS-01) + the expansion outcome (Expansion). Metadata routed through
	// the REDACTED path like every non-thinking kind (D-23 type-scoping).
	TypeLocalCommand = "local_command"

	// TypeCompaction records a compaction boundary marker (D-21, rich
	// boundary record): a fresh boundary id (BoundaryID, crypto/rand UUID
	// v4) + the token-usage snapshot at the boundary (input/output/cache
	// totals) + opaque pre/post transcript pointers (PreRef/PostRef — what
	// survived / where the projected window resets). Phase 19 reconstructs
	// the reset from the marker alone; until then the Projector treats the
	// kind as inert (D-20 additive-only — it is NOT a TypeBoundary reset
	// point today).
	TypeCompaction = "compaction"
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

	// request_shaped — METADATA-ONLY (09-05, AUD-03/D-01): the line carries
	// the correlation triple (session-from-filename, turn, request=Ref) + the
	// shape fingerprint; the full REDACTED body lives in the capped audit body
	// store, retrievable by Ref. This SUPERSEDES v1.0 D-20's inline-verbatim
	// request line (~80 KB × hands-off multiplication — Pitfall 10); do NOT
	// restore inline bodies.
	Profile      string `json:"profile,omitempty"`
	Ref          string `json:"ref,omitempty"`
	SystemBlocks int    `json:"systemBlocks,omitempty"` //nolint:tagliatelle // on-disk format (camelCase, matches Line)
	Tools        int    `json:"tools,omitempty"`
	Model        string `json:"model,omitempty"`
	Bytes        int    `json:"bytes,omitempty"`

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

	// engine_decision (09-02, AUD-04/D-03 full provenance — one line answers
	// "why did it continue" without cross-referencing)
	MatchedSpan  string `json:"matchedSpan,omitempty"`  //nolint:tagliatelle // on-disk format
	ConfigSource string `json:"configSource,omitempty"` //nolint:tagliatelle // on-disk format

	// Phase-16 additive kinds (D-20..D-23). raw_thinking rides the EXISTING
	// Content (payload) + Model (provider attribution) fields; local_command
	// carries the invocation record; compaction carries the D-21 boundary
	// record. Readers must tolerate these kinds AND unknown future
	// kinds/fields.
	Args        string   `json:"args,omitempty"`
	SourceChain []string `json:"sourceChain,omitempty"` //nolint:tagliatelle // on-disk format
	Expansion   string   `json:"expansion,omitempty"`
	BoundaryID  string   `json:"boundaryID,omitempty"`  //nolint:tagliatelle // on-disk format
	CacheTokens int64    `json:"cacheTokens,omitempty"` //nolint:tagliatelle // on-disk format
	PreRef      string   `json:"preRef,omitempty"`      //nolint:tagliatelle // on-disk format
	PostRef     string   `json:"postRef,omitempty"`     //nolint:tagliatelle // on-disk format
}

// selfGitignoreContent is the .ass-guard/.gitignore body (D-07): ignore
// everything except .gitignore itself.
const selfGitignoreContent = "*\n!.gitignore\n"

// openTranscript ensures dir/.ass-guard exists (with a self-gitignore), then
// opens the per-session JSONL file O_APPEND|O_CREATE|O_WRONLY mode 0600.
func openTranscript(dir, sessionID string) (*os.File, string, error) {
	storeDir := filepath.Join(dir, ".ass-guard")

	err := os.MkdirAll(storeDir, dirPerm)
	if err != nil {
		return nil, "", fmt.Errorf("call: %w", err)
	}

	giPath := filepath.Join(storeDir, ".gitignore")

	_, err = os.Stat(giPath)
	if os.IsNotExist(err) {
		err = os.WriteFile(giPath, []byte(selfGitignoreContent), filePermDefault)
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
