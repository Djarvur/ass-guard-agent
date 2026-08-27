// Package acp implements the ACP v1 stdio server (ACP-01/02/04/05).
//
// The server speaks newline-delimited JSON-RPC 2.0 over stdin/stdout (stdout is
// reserved EXCLUSIVELY for ACP frames; all diagnostics go to stderr — the
// transport-discipline contract from PROJECT.md and VERIFIED-FACTS #5). The
// framing is hand-rolled (D-14 — ~150 LOC, zero deps): a bufio.Reader over stdin
// reads one JSON object per line, and a mutex-guarded Writer emits frames to
// stdout. Notifications (session/update, session/cancel) carry no `id` and get
// no response; requests carry an `id` and always receive a result or error.
//
// The wire shape is pinned to the canonical ACP v1 spec (VERIFIED-FACTS #3):
// the initialize result uses the exact field name `agentCapabilities` (NOT
// capabilities/serverInfo), integer `protocolVersion: 1`, and the
// session/update notification carries a `sessionUpdate` discriminator.
package acp

import (
	"encoding/json"
	"fmt"
)

// Message is the JSON-RPC 2.0 envelope. A request carries an id + method +
// params; a notification carries method + params with NO id; a response carries
// an id + exactly one of result/error.
//
// ID is json.RawMessage so a notification (nil ID) marshals with NO `id` field
// at all (json:"id,omitempty") — distinguishing "no id" (notification) from
// "id: 0" (a valid request id). RawMessage also accepts string ids (jsonrpc.org/
// spec: id may be a string, number, or null) and echoes them VERBATIM in the
// response — Zed sends UUID strings, which *int rejected with a -32700 parse
// error (endless-loading report, 2026-08-14). This preserves the JSON-RPC
// notification-vs-request distinction (VERIFIED-FACTS #3 Note 5).
type Message struct {
	JSONRPC string          `json:"jsonrpc"`          // always "2.0"
	ID      json.RawMessage `json:"id,omitempty"`     // nil for notifications
	Method  string          `json:"method,omitempty"` // present on request/notification
	Params  json.RawMessage `json:"params,omitempty"` // request/notification payload
	Result  json.RawMessage `json:"result,omitempty"` // response success
	Error   *RPCError       `json:"error,omitempty"`  // response failure
}

// RPCError is the JSON-RPC 2.0 error object. It implements the error interface
// so handlers can return a specific JSON-RPC code (e.g. -32601 method-not-found
// for session/load's D-09 no-op) and handleRequest surfaces it verbatim instead
// of wrapping it as a generic -32603.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *RPCError) Error() string { return fmt.Sprintf("jsonrpc %d: %s", e.Code, e.Message) }

// ContentBlock is one entry of an ACP prompt/content array (PROMPT-TURN.md). In
// Phase 2 only the text block is exercised; the shape is forward-compatible with
// the full content-block set.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// --- ACP-03 live-turn frame vocabulary (16-01) ---
//
// Every field name below is pinned VERBATIM against the canonical ACP v1
// schema (RESEARCH §Code Examples, schema/v1/schema.json defs ToolCall,
// ToolCallUpdate, Diff, ToolCallLocation, Plan, PlanEntry, ContentChunk).
// v1 spellings ONLY — the v2 draft renames (the plan kind and the config-id
// field get new names there) NEVER appear in this file (Pitfall 7 — Zed is
// verified v1; an audit grep enforces the absence).

// Tool kind values (v1 ToolKind enum).
const (
	ToolKindRead       = "read"
	ToolKindEdit       = "edit"
	ToolKindDelete     = "delete"
	ToolKindMove       = "move"
	ToolKindSearch     = "search"
	ToolKindExecute    = "execute"
	ToolKindThink      = "think"
	ToolKindFetch      = "fetch"
	ToolKindSwitchMode = "switch_mode"
	ToolKindOther      = "other"
)

// Tool call status values (v1 ToolCallStatus enum — four values ONLY: v1 has
// NO "cancelled" status; agent-side cancellation closure is internal state,
// never a wire status).
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// ToolCallFrame is the payload of a v1 `tool_call` session/update card.
type ToolCallFrame struct {
	ToolCallID string             `json:"toolCallId"` //nolint:tagliatelle // ACP wire field (required)
	Title      string             `json:"title,omitempty"`
	Kind       string             `json:"kind,omitempty"`   // ToolKind values
	Status     string             `json:"status,omitempty"` // Status* values
	Content    []ToolCallContent  `json:"content,omitempty"`
	Locations  []ToolCallLocation `json:"locations,omitempty"`
	// Input mirrors the runtime bus event's raw input so the acp layer can
	// derive presentation variants (e.g. TodoWrite → plan frame). It is NEVER
	// marshaled — v1's tool_call frame carries no rawInput field.
	Input json.RawMessage `json:"-"`
}

// ToolCallUpdateFrame is the payload of a v1 `tool_call_update` session/update
// notification. All fields except toolCallId are partial-update options.
type ToolCallUpdateFrame struct {
	ToolCallID string             `json:"toolCallId"` //nolint:tagliatelle // ACP wire field (required)
	Title      string             `json:"title,omitempty"`
	Kind       string             `json:"kind,omitempty"`   // ToolKind values
	Status     string             `json:"status,omitempty"` // Status* values
	Content    []ToolCallContent  `json:"content,omitempty"`
	Locations  []ToolCallLocation `json:"locations,omitempty"`
}

// ToolCallContent is one entry of a tool-call frame's content array. The v1
// schema models it as a union over `type`; this flat struct carries every
// variant's fields and each producer fills exactly its own.
//
//	{"type":"content","content":{…TextBlock}}   — inline text/output preview
//	{"type":"diff","path":"…","newText":"…"}    — file edit diff (OldText null
//	                                              becomes omitted for new files)
type ToolCallContent struct {
	Type    string        `json:"type"`              // required v1: "content" | "diff"
	Content *ContentBlock `json:"content,omitempty"` // the "content" variant payload
	Path    string        `json:"path,omitempty"`    // diff variant: file path (required when diff)
	OldText string        `json:"oldText,omitempty"` //nolint:tagliatelle // ACP wire field (diff)
	NewText string        `json:"newText,omitempty"` //nolint:tagliatelle // ACP wire field (diff)
}

// DiffContent names the diff variant's payload as a plain builder view — v1
// flattens path/oldText/newText onto the ToolCallContent object itself, so
// this type exists to keep constructors legible; Frame() produces the wire shape.
type DiffContent struct {
	Path    string
	OldText string
	NewText string
}

// Frame converts the builder view into its flattened ToolCallContent form.
func (d DiffContent) Frame() ToolCallContent {
	return ToolCallContent{Type: "diff", Path: d.Path, OldText: d.OldText, NewText: d.NewText}
}

// ToolCallLocation names where a tool call touched (v1 ToolCallLocation:
// absolute path + optional line). Line is a pointer so a genuine line number
// stays distinct from "no line".
type ToolCallLocation struct {
	Path string `json:"path"`
	Line *int64 `json:"line,omitempty"`
}

// PlanFrame is the payload of a v1 `plan` session/update. Entries is REQUIRED
// and full-replacement ("The client replaces the entire plan with each update").
type PlanFrame struct {
	Entries []PlanEntry `json:"entries"`
}

// PlanEntry is one row of the live plan panel.
type PlanEntry struct {
	Content  string `json:"content"`  // required
	Priority string `json:"priority"` // required (high | medium | low)
	Status   string `json:"status"`   // required (pending | in_progress | completed)
}

// ThoughtChunkFrame is the payload of an `agent_thought_chunk` session/update —
// v1's ContentChunk shape (messageId shared across one message's chunks; "a
// change in messageId indicates a new message has started"). No provider
// thinking source exists until Phase 21 (PAR-05); the shape completes the
// emitter's frame surface ahead of it.
type ThoughtChunkFrame struct {
	MessageID string       `json:"messageId"` //nolint:tagliatelle // ACP wire field
	Content   ContentBlock `json:"content"`   // required
}

// Canonical JSON-RPC error codes (jsonrpc.org/spec) + the ACP v1
// request-cancellation code (docs/protocol/v1/cancellation.mdx). A response
// carrying CodeRequestCancelled resolves a pending Registry entry as cancelled
// with NO retry (D-14); internal cancellation SHOULD surface the same code.
const (
	CodeParseError       = -32700
	CodeInvalidRequest   = -32600
	CodeMethodNotFound   = -32601
	CodeInvalidParams    = -32602
	CodeInternalError    = -32603
	CodeRequestCancelled = -32800
)

// Wire method names of the outbound-request surface (16-02/D-19).
const (
	// methodCancelRequest is the request-cancellation notification. Agent→client
	// it cancels one of the CLIENT's in-flight activities (the documented
	// cascade); client→agent it cancels one of OURS (a fast no-op for us in
	// v1.2 — Assumption A8).
	methodCancelRequest = "$/cancel_request"

	// methodElicitationCreate is the agent→client elicitation request — the
	// D-13 capability probe subject, and the vehicle for Phase 17's real
	// permission/question forms.
	methodElicitationCreate = "elicitation/create"
)
