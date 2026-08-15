package event

import (
	"encoding/json"
	"time"
)

// Per-kind channel buffer sizes (D-05, RESEARCH §2.3). High-volume streaming
// kinds get generous buffers; low-volume control kinds get small ones. A full
// buffer blocks the producer (backpressure) rather than dropping events.
const (
	BufAgentMessageChunk = 128 // the streaming hot path; absorb bursts
	BufToolCall          = 16
	BufToolCallUpdate    = 32
	BufUsageUpdate       = 8
	BufRequestShaped     = 16
	BufSubagentResult    = 8
	BufBoundary          = 4
	BufEngineDecision    = 8  // Phase-4: one engine verdict per turn (ENG-02 provenance stream)
	BufHookProgress      = 16 // Phase-4: one event per hook step boundary (HOOK-01..05)
)

// RequestShaped carries the verbatim shaped outgoing request (LOG-01 evidence).
// Published by the turn loop before the provider sends; the transcript writer
// (and, until Plan 02-07, the AuditLogger) subscribes. TurnID is additive over
// the Phase-1 struct (Phase-1 struct-literal construction still compiles).
type RequestShaped struct {
	TurnID          string
	VerbatimRequest json.RawMessage
	Profile         string
	Timestamp       time.Time
}

// Kind returns the event discriminator.
func (RequestShaped) Kind() string { return "RequestShaped" }

// AgentMessageChunk is one streamed token/fragment of an assistant message
// (ACP-04 streaming — provider SSE → bus → session/update). The ACP adapter
// forwards each chunk to the client as it arrives (NO full-turn buffering).
type AgentMessageChunk struct {
	TurnID    string
	MessageID string
	Content   string
}

// Kind returns the event discriminator.
func (AgentMessageChunk) Kind() string { return "AgentMessageChunk" }

// ToolCall is a model-selected tool invocation, published when the provider
// response carries tool_use blocks.
type ToolCall struct {
	TurnID     string
	ToolCallID string
	Name       string
	Input      json.RawMessage
}

// Kind returns the event discriminator.
func (ToolCall) Kind() string { return "ToolCall" }

// ToolCallUpdate carries a partial/in-progress update for a tool call (forward-
// compatible with streaming tool execution in Phase 4).
type ToolCallUpdate struct {
	TurnID     string
	ToolCallID string
	Update     json.RawMessage
}

// Kind returns the event discriminator.
func (ToolCallUpdate) Kind() string { return "ToolCallUpdate" }

// UsageUpdate carries token-usage accounting for a turn.
type UsageUpdate struct {
	TurnID          string
	InputTokens     int64
	OutputTokens    int64
	CacheReadTokens int64
}

// Kind returns the event discriminator.
func (UsageUpdate) Kind() string { return "UsageUpdate" }

// SubagentResult carries the outcome of a Task/Agent subagent dispatch (PARA-02).
// ParentTurnID tags the result for the parent's turn; the parent's lean window
// receives only this final result (D-11 — intermediates are excluded).
type SubagentResult struct {
	ParentTurnID   string
	ToolCallID     string
	SubagentTurnID string
	Result         string
	Err            error
}

// Kind returns the event discriminator.
func (SubagentResult) Kind() string { return "SubagentResult" }

// Boundary marks a session context-boundary (a mutating command completed, so
// the next projection resets the lean window — SESS-02/03/04). Published by the
// turn loop after a mutating tool_result; the transcript writer appends a
// `boundary` line.
type Boundary struct {
	TurnID     string
	Cause      string // "mutating-command:<Tool>" or "config-added:<Tool>"
	CommandRef string // the tool-call id that triggered the boundary
}

// Kind returns the event discriminator.
func (Boundary) Kind() string { return "Boundary" }

// EngineDecision is the unified engine's verdict for one turn (Phase-4 ENG-02 —
// the single provenance-tagged stream). The engine emits one EngineDecision per
// turn AFTER the turn completes (D-01 post-turn observer), including
// ActionNothing so the audit log proves the structural-safety property
// (unmatched output triggers nothing — D-03). The ACP adapter surfaces ask
// decisions as a session/update; the transcript writer appends an
// engine_decision line (D-20).
type EngineDecision struct {
	TurnID string
	Action string // "nothing" | "continue" | "hook" | "ask" | "wait"
	Signal string // "text:<patternID>" | "tool:<toolID>" | "unmatched"
	// MatchedSpan is the literal matched text (text signals) or the tool-call
	// name (tool signals) — D-03 full provenance (09-02, AUD-04). Empty when
	// unmatched.
	MatchedSpan string
	// ConfigSource names the config entry that fired, e.g.
	// "openspec.toml patterns/<id>" — identifiers only, never file contents.
	ConfigSource string
	Reason       string // human-readable, investigate-and-fix-ready (PROJECT.md)
}

// Kind returns the event discriminator.
func (EngineDecision) Kind() string { return "EngineDecision" }

// HookProgress is one event at a hook-step boundary (Phase-4 HOOK-01..05). The
// hook-DAG executor emits a start/finish/error/ask event per step so the
// forgotten routine is fully auditable (investigate-and-fix-ready). Defined here
// (consumed by internal/hookdag in Plan 04-03) so the bus has a single
// definition point and avoids a later merge conflict.
type HookProgress struct {
	TurnID    string
	HookName  string
	StepIndex int
	StepKind  string // "run-command" | "send-prompt" | "fresh-context" | "wait"
	Status    string // "start" | "finish" | "error" | "ask"
	Err       string // populated when Status == "error" or "ask"
}

// Kind returns the event discriminator.
func (HookProgress) Kind() string { return "HookProgress" }
