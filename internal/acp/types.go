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

// --- 16-05 config-options wire vocabulary (ACP-08) ---
//
// Field names pinned VERBATIM against schema/v1/schema.json defs
// SessionConfigOption, SessionConfigSelect, SessionConfigSelectOption,
// SetSessionConfigOptionRequest/Response, ConfigOptionUpdate. v1 spellings
// ONLY: the advertisement option key is `id` (the v2 draft renames it
// configId — Pitfall 7); the SET REQUEST carries `configId` per the v1
// SetSessionConfigOptionRequest def. Both verified against the fetched schema.

// KindConfigOptionUpdate is the v1 sessionUpdate kind announcing that the
// configuration options changed out-of-band (schema/v1 SessionUpdate const
// "config_option_update"; its payload carries the FULL refreshed set).
const KindConfigOptionUpdate = "config_option_update"

// ConfigOptionTypeSelect is the v1 SessionConfigOption type discriminator for
// the single-value selector variant (the only variant ass-guard advertises).
const ConfigOptionTypeSelect = "select"

// ConfigOptionValue is one selectable value of a select config option (v1
// SessionConfigSelectOption).
type ConfigOptionValue struct {
	Value       string `json:"value"` // required
	Name        string `json:"name"`  // required
	Description string `json:"description,omitempty"`
}

// ConfigOptionFrame is the v1 SessionConfigOption advertisement shape: the
// selector identity plus its select payload (currentValue + options sit flat
// beside the `type` discriminator on the wire — the schema's oneOf/allOf
// composition). CurrentValue is REQUIRED and carries the option's current
// EFFECTIVE value resolved through the precedence chain (D-11) — never a
// static default.
type ConfigOptionFrame struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	Category     string              `json:"category,omitempty"` // mode|model|model_config|thought_level | _custom
	Type         string              `json:"type"`               // ConfigOptionTypeSelect
	CurrentValue string              `json:"currentValue"`       //nolint:tagliatelle // ACP wire field (D-11)
	Options      []ConfigOptionValue `json:"options"`            // required for select
}

// ConfigViolationError is the D-09 typed reject: an option id or value that
// failed menu validation. The set_config_option handler maps it to
// CodeInvalidParams with Data{optionId, violation} so the editor renders the
// rejection. Returned BY the ConfigSurface (the surface owns the menu
// semantics); the handler only translates.
type ConfigViolationError struct {
	OptionID  string
	Violation string
}

// Error implements the error interface.
func (e *ConfigViolationError) Error() string {
	return fmt.Sprintf("config option %q: %s", e.OptionID, e.Violation)
}

// ConfigPersistError is the D-07 persist-failure class: the layer write behind
// a set_config_option failed. DISTINCT from ConfigViolationError (and mapped
// to a distinct JSON-RPC error class) so a write failure never reads as client
// error — the operator's file state is untouched when this returns.
type ConfigPersistError struct {
	OptionID string
	Err      error
}

// Error implements the error interface (names the option, never secret
// material — layer paths carry no credentials).
func (e *ConfigPersistError) Error() string {
	return fmt.Sprintf("config option %q: persist failed: %v", e.OptionID, e.Err)
}

// Unwrap exposes the cause for errors.As/Is chains.
func (e *ConfigPersistError) Unwrap() error { return e.Err }

// Wire method name of the editor-driven configuration surface (16-05/ACP-08).
const (
	// methodSetConfigOption is the client→agent request that persists+applies
	// one advertised option (schema/v1 SetSessionConfigOptionRequest
	// x-method). The response and the out-of-band config_option_update
	// notification BOTH carry the full option set with current values.
	methodSetConfigOption = "session/set_config_option"
)

// Wire method names of the outbound-request surface (16-02/D-19).
const (
	// methodCancelRequest is the request-cancellation notification. Agent→client
	// it cancels one of the CLIENT's in-flight activities (the documented
	// cascade); client→agent it cancels one of OURS (a fast no-op for us in
	// v1.2 — Assumption A8).
	methodCancelRequest = "$/cancel_request"

	// MethodElicitationCreate is the agent→client elicitation request — the
	// D-13 capability probe subject, and the vehicle for Phase 17's real
	// permission/question forms. Exported: the ask surface lives one package
	// up (the MethodRequestPermission precedent).
	MethodElicitationCreate = "elicitation/create"

	// MethodRequestPermission is the agent→client permission request (17-02,
	// ACP-01; schema/v1 RequestPermissionRequest x-method). The ask surface
	// (internal/acpserve) dispatches it through the 16-03 registry under the
	// HUMAN-ASK class. Exported: the ask surface lives one package up; the
	// handler-owned method constants above stay unexported.
	MethodRequestPermission = "session/request_permission"
)

// --- 17-02 permission-ask wire vocabulary (ACP-01) ---
//
// Field names pinned VERBATIM against the canonical ACP v1 schema (17-RESEARCH
// §Wire Shapes, defs RequestPermissionRequest, PermissionOption,
// SelectedPermissionOutcome — parsed from schema/v1/schema.json). v1 spellings
// ONLY (Pitfall 7).

// PermissionOptionKind is the COMPLETE v1 enum (schema PermissionOptionKind):
// allow/reject × once/always — exactly ACP-01's option set. The dialog option
// names must stay neutral and symmetric (the ACP-01 values prohibition).
const (
	PermOptionAllowOnce    = "allow_once"    // "Allow this operation only this time."
	PermOptionAllowAlways  = "allow_always"  // "Allow this operation and remember the choice."
	PermOptionRejectOnce   = "reject_once"   // "Reject this operation only this time."
	PermOptionRejectAlways = "reject_always" // "Reject this operation and remember the choice."
)

// PermissionOption is one selectable option of the permission dialog (schema
// PermissionOption; required: optionId, name, kind).
type PermissionOption struct {
	OptionID string `json:"optionId"` //nolint:tagliatelle // ACP wire field (required)
	Name     string `json:"name"`     // required — the human label (neutral, symmetric)
	Kind     string `json:"kind"`     // required — PermOption* values
}

// RequestPermissionFrame is the session/request_permission payload (schema
// RequestPermissionRequest; required: sessionId, toolCall, options). toolCall
// is the SAME ToolCallUpdate shape as the streaming update — the dialog shows
// what it is authorizing.
type RequestPermissionFrame struct {
	SessionID string              `json:"sessionId"` //nolint:tagliatelle // ACP wire field (required)
	ToolCall  ToolCallUpdateFrame `json:"toolCall"`  // required
	Options   []PermissionOption  `json:"options"`   // required
}

// PermissionOutcomeFrame is the session/request_permission RESPONSE (schema
// RequestPermissionOutcome): either {"outcome":"cancelled"} or
// {"outcome":"selected","optionId":"…"}. Per the schema's normative text, a
// client cancelling the turn MUST answer every pending request_permission
// with the cancelled outcome.
type PermissionOutcomeFrame struct {
	Outcome  string `json:"outcome"`            // "selected" | "cancelled"
	OptionID string `json:"optionId,omitempty"` //nolint:tagliatelle // ACP wire field (required when selected)
}

// Permission outcome discriminators (schema RequestPermissionOutcome).
const (
	PermissionOutcomeSelected  = "selected"
	PermissionOutcomeCancelled = "cancelled"
)

// --- 17-04 elicitation wire vocabulary (ACP-02) ---
//
// Field names pinned VERBATIM against the canonical ACP v1 schema (17-RESEARCH
// §Wire Shapes, defs CreateElicitationRequest, ElicitationSchema,
// CreateElicitationResponse — parsed from schema/v1/schema.json). v1 spellings
// ONLY (Pitfall 7). Schema properties ride as json.RawMessage per property so
// every ElicitationPropertySchema variant's exact shape is built and delivered
// verbatim (the D-08 mapping) — never flattened into a lowest-common-denominator
// struct.

const (
	// ElicitationModeForm is the form-mode discriminator (the only mode
	// ass-guard sends; url mode is wire-defined but explicitly deferred).
	ElicitationModeForm = "form"
)

// Elicitation response actions (schema CreateElicitationResponse). decline is
// an ANSWER-SHAPED refusal (D-10: it routes to the non-answer form with NO
// re-ask — unlike an invalid accept payload, which re-asks once); cancel is
// the dismissed dialog.
const (
	ElicitationActionAccept  = "accept"
	ElicitationActionDecline = "decline"
	ElicitationActionCancel  = "cancel"
)

// Sticky capability keys (the 16-D-13/D-18 per-connection negotiation cache;
// Server.Capability consults them).
const (
	// CapElicitationForm is the form-elicitation capability — the D-13 probe
	// subject, and the sticky state Phase 17's question-family dispatcher
	// consults per ask.
	CapElicitationForm = "elicitation.form"

	// CapBooleanConfigOption is the client's boolean config-option
	// advertisement (session.configOptions.boolean) — D-08's conservative
	// boolean-property gate reads the same connection capability family.
	CapBooleanConfigOption = "session.configOptions.boolean"
)

// ElicitationFormFrame is the elicitation/create payload (schema
// CreateElicitationRequest): a human-readable message + the form-mode
// discriminator + the requestedSchema the accept content must match. ass-guard
// always asks in the session scope (sessionId + optional toolCallId).
type ElicitationFormFrame struct {
	Message         string            `json:"message"`
	Mode            string            `json:"mode"`                 // ElicitationModeForm
	RequestedSchema ElicitationSchema `json:"requestedSchema"`      //nolint:tagliatelle // ACP wire field (required)
	SessionID       string            `json:"sessionId,omitempty"`  //nolint:tagliatelle // ACP wire field
	ToolCallID      string            `json:"toolCallId,omitempty"` //nolint:tagliatelle // ACP wire field
}

// ElicitationSchema is the form-mode requestedSchema (schema
// ElicitationSchema): the "object" discriminator + named properties + required.
// Each property value IS its ElicitationPropertySchema variant verbatim (raw)
// — single-select string+oneOf titled consts, multi-select array items,
// boolean, free-text string (the D-08 mapping builds the exact variants; the
// MCP-legacy enumNames key never appears — RFD ban).
type ElicitationSchema struct {
	Type       string                     `json:"type"` // "object"
	Title      string                     `json:"title,omitempty"`
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required,omitempty"`
}

// ElicitationOutcomeFrame is the elicitation/create RESPONSE (schema
// CreateElicitationResponse): the action discriminator; accept carries the
// content object matching requestedSchema (values are
// string|number|integer|boolean|array-of-string). The content is UNTRUSTED
// input — the D-10 validator re-checks it against the requestedSchema before
// anything consumes it (RFD defense-in-depth).
type ElicitationOutcomeFrame struct {
	Action  string                     `json:"action"`
	Content map[string]json.RawMessage `json:"content,omitempty"`
}
