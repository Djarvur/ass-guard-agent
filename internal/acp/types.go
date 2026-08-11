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
// ID is *int so a notification (nil ID) marshals with NO `id` field at all
// (json:"id,omitempty") — distinguishing "no id" (notification) from "id: 0"
// (a valid request id). This is the JSON-RPC notification-vs-request distinction
// (VERIFIED-FACTS #3 Note 5).
type Message struct {
	JSONRPC string          `json:"jsonrpc"`           // always "2.0"
	ID      *int            `json:"id,omitempty"`       // nil for notifications
	Method  string          `json:"method,omitempty"`   // present on request/notification
	Params  json.RawMessage `json:"params,omitempty"`   // request/notification payload
	Result  json.RawMessage `json:"result,omitempty"`   // response success
	Error   *RPCError       `json:"error,omitempty"`    // response failure
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

// Canonical JSON-RPC error codes (jsonrpc.org/spec).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)
