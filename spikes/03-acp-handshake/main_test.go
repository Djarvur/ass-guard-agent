// SPDX-License-Identifier: MIT
//
// RED-phase tests for the #3 ACP v1 handshake spike (Phase 0, Plan 00-03).
//
// These tests assert the OFFLINE-VERIFIABLE part of <behavior>: the ACP v1 wire
// framing rule (newline-delimited JSON-RPC, NO Content-Length header, NO embedded
// newlines) and the envelope shapes for the four methods Phase 2's ACP adapter
// will implement (initialize / session/new / session/prompt / session/update).
// They do NOT exercise a live peer (that round-trip + the "≥1 session/update
// before the prompt response" assertion is run by main.go against an in-process
// mock ACP server). Per the TDD flow these MUST fail until main.go exists — the
// RED bar is driven by undefined helpers (writeFrame/readFrame/build*) that
// main.go provides.
//
// Canonical spec grounding (agentclientprotocol.com/protocol/v1/, fetched
// 2026-08-09): transports.md mandates "Messages are delimited by newlines (\n),
// and MUST NOT contain embedded newlines"; overview.md + session-setup.md +
// prompt-turn.md + initialization.md fix the method names, params, and result
// field names asserted below.
//
// Transport discipline (AGENTS.md): this is a test file; `go test` output is not
// ACP traffic and goes through the normal test runner.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestWriteFrameProducesNewlineDelimitedJSON is the load-bearing framing
// assertion for STACK item #3. ACP v1 frames are newline-delimited JSON with NO
// Content-Length header and NO embedded newlines (transports.md). writeFrame
// MUST emit exactly one JSON object followed by a single '\n', and MUST reject
// any value whose marshaled form contains an embedded newline.
func TestWriteFrameProducesNewlineDelimitedJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := writeFrame(&buf, map[string]any{"jsonrpc": "2.0", "id": float64(1), "method": "initialize"}); err != nil {
		t.Fatalf("writeFrame: %v (main.go not implemented yet — RED)", err)
	}
	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("frame not terminated by '\\n': %q", out)
	}
	// Exactly one trailing newline, no leading Content-Length, no embedded newlines.
	body := strings.TrimSuffix(out, "\n")
	if strings.Count(body, "\n") != 0 {
		t.Fatalf("frame body has embedded newlines (forbidden by spec): %q", body)
	}
	if strings.Contains(body, "Content-Length") || strings.Contains(body, "Content-length") {
		t.Fatalf("frame carries a Content-Length header (ACP v1 uses newline framing, NOT LSP Content-Length): %q", body)
	}
	// The body must be a single valid JSON object.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("frame body is not valid JSON: %v (body=%q)", err, body)
	}
	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want \"2.0\"", parsed["jsonrpc"])
	}
}

// TestWriteFrameRejectsEmbeddedNewlines asserts the spec's "MUST NOT contain
// embedded newlines" rule is enforced at write time (a string value carrying a
// literal '\n' would corrupt the framing).
func TestWriteFrameRejectsEmbeddedNewlines(t *testing.T) {
	var buf bytes.Buffer
	err := writeFrame(&buf, map[string]any{"text": "line1\nline2"})
	if err == nil {
		t.Fatalf("writeFrame accepted a value with an embedded newline (spec violation): %q", buf.String())
	}
}

// TestReadWriteFrameRoundTrips confirms readFrame parses exactly what writeFrame
// emits — one frame per '\n', bufio line discipline, no Content-Length parsing.
func TestReadWriteFrameRoundTrips(t *testing.T) {
	var buf bytes.Buffer
	orig := map[string]any{"jsonrpc": "2.0", "id": float64(7), "method": "session/new", "params": map[string]any{"cwd": "/tmp/x"}}
	if err := writeFrame(&buf, orig); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	// Append a second frame to prove line-delimited multi-frame reads work.
	if err := writeFrame(&buf, map[string]any{"jsonrpc": "2.0", "method": "session/update"}); err != nil {
		t.Fatalf("writeFrame #2: %v", err)
	}
	r := bufio.NewReader(&buf)
	first, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame #1: %v", err)
	}
	if first["method"] != "session/new" {
		t.Errorf("first.method = %v, want session/new", first["method"])
	}
	second, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame #2: %v", err)
	}
	if second["method"] != "session/update" {
		t.Errorf("second.method = %v, want session/update", second["method"])
	}
	// EOF after the last frame.
	if _, err := readFrame(r); err == nil {
		t.Fatalf("readFrame #3: expected EOF, got nil")
	}
}

// TestBuildInitializeRequestEnvelope asserts the initialize request matches the
// canonical spec (initialization.md): method "initialize", params.protocolVersion
// present, params.clientCapabilities present, params.clientInfo present.
func TestBuildInitializeRequestEnvelope(t *testing.T) {
	f := buildInitializeRequest(0, 1)
	if f == nil {
		t.Fatal("buildInitializeRequest returned nil — main.go not implemented yet (RED)")
	}
	if f["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want \"2.0\"", f["jsonrpc"])
	}
	if f["method"] != "initialize" {
		t.Errorf("method = %v, want \"initialize\"", f["method"])
	}
	if _, ok := f["id"]; !ok {
		t.Errorf("initialize request missing id field")
	}
	params, ok := f["params"].(map[string]any)
	if !ok {
		t.Fatalf("params is not an object: %v", f["params"])
	}
	if _, ok := params["protocolVersion"]; !ok {
		t.Errorf("params.protocolVersion missing (spec requires it)")
	}
	if _, ok := params["clientCapabilities"]; !ok {
		t.Errorf("params.clientCapabilities missing")
	}
	if _, ok := params["clientInfo"]; !ok {
		t.Errorf("params.clientInfo missing")
	}
}

// TestInitializeResultUsesAgentCapabilitiesFieldName is the Tier-A-critical
// field-name assertion: the initialize result field is "agentCapabilities" —
// NOT "capabilities" or "serverInfo" (differs from LSP/MCP). The build*
// helper for the result MUST use the exact spec field name so a Phase 2 adapter
// copy-pasting from this spike gets it right.
func TestInitializeResultUsesAgentCapabilitiesFieldName(t *testing.T) {
	f := buildInitializeResult(0, 1, map[string]any{"loadSession": true})
	if f == nil {
		t.Fatal("buildInitializeResult returned nil — main.go not implemented yet (RED)")
	}
	if f["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want \"2.0\"", f["jsonrpc"])
	}
	res, ok := f["result"].(map[string]any)
	if !ok {
		t.Fatalf("result is not an object: %v", f["result"])
	}
	if _, ok := res["protocolVersion"]; !ok {
		t.Errorf("result.protocolVersion missing")
	}
	if _, ok := res["agentCapabilities"]; !ok {
		t.Errorf("result.agentCapabilities missing — field name must be exactly \"agentCapabilities\" (NOT capabilities/serverInfo)")
	}
	// Explicitly assert the WRONG field names are absent (catches the LSP/MCP drift).
	if _, ok := res["capabilities"]; ok {
		t.Errorf("result has \"capabilities\" — wrong field name; spec uses \"agentCapabilities\"")
	}
	if _, ok := res["serverInfo"]; ok {
		t.Errorf("result has \"serverInfo\" — wrong field name; spec uses \"agentCapabilities\" + \"agentInfo\"")
	}
}

// TestBuildSessionNewRequestEnvelope asserts session/new carries cwd + mcpServers
// (session-setup.md). This is the Tier-A step STACK omitted from its lifecycle.
func TestBuildSessionNewRequestEnvelope(t *testing.T) {
	f := buildSessionNewRequest(1, "/tmp/spike", []any{})
	if f == nil {
		t.Fatal("buildSessionNewRequest returned nil — main.go not implemented yet (RED)")
	}
	if f["method"] != "session/new" {
		t.Errorf("method = %v, want \"session/new\"", f["method"])
	}
	params, ok := f["params"].(map[string]any)
	if !ok {
		t.Fatalf("params is not an object: %v", f["params"])
	}
	if _, ok := params["cwd"]; !ok {
		t.Errorf("params.cwd missing (session/new requires cwd)")
	}
	servers, ok := params["mcpServers"]
	if !ok {
		t.Fatalf("params.mcpServers missing (session/new requires mcpServers)")
	}
	serversArr, ok := servers.([]any)
	if !ok {
		t.Fatalf("params.mcpServers is not an array: %v", servers)
	}
	if len(serversArr) != 0 {
		t.Errorf("mcpServers len = %d, want 0 (empty array is valid)", len(serversArr))
	}
}

// TestSessionNewResultCarriesSessionId asserts the session/new result contains a
// sessionId — the value Phase 2 threads into session/prompt.
func TestSessionNewResultCarriesSessionId(t *testing.T) {
	f := buildSessionNewResult(1, "sess_redacted")
	if f == nil {
		t.Fatal("buildSessionNewResult returned nil — main.go not implemented yet (RED)")
	}
	res, ok := f["result"].(map[string]any)
	if !ok {
		t.Fatalf("result is not an object: %v", f["result"])
	}
	if _, ok := res["sessionId"]; !ok {
		t.Errorf("result.sessionId missing (spec: session/new returns {sessionId})")
	}
}

// TestBuildSessionPromptRequestEnvelope asserts session/prompt carries sessionId
// + a prompt array of content blocks (prompt-turn.md). No tools/messages field.
func TestBuildSessionPromptRequestEnvelope(t *testing.T) {
	f := buildSessionPromptRequest(2, "sess_redacted", []any{
		map[string]any{"type": "text", "text": "[REDACTED: prompt]"},
	})
	if f == nil {
		t.Fatal("buildSessionPromptRequest returned nil — main.go not implemented yet (RED)")
	}
	if f["method"] != "session/prompt" {
		t.Errorf("method = %v, want \"session/prompt\"", f["method"])
	}
	params, ok := f["params"].(map[string]any)
	if !ok {
		t.Fatalf("params is not an object: %v", f["params"])
	}
	if _, ok := params["sessionId"]; !ok {
		t.Errorf("params.sessionId missing")
	}
	prompt, ok := params["prompt"]
	if !ok {
		t.Fatalf("params.prompt missing (session/prompt takes a prompt content array)")
	}
	promptArr, ok := prompt.([]any)
	if !ok || len(promptArr) == 0 {
		t.Fatalf("params.prompt is not a non-empty array: %v", prompt)
	}
	first, ok := promptArr[0].(map[string]any)
	if !ok {
		t.Fatalf("prompt[0] is not an object: %v", promptArr[0])
	}
	if first["type"] != "text" {
		t.Errorf("prompt[0].type = %v, want text", first["type"])
	}
	// The agent owns its tool catalog — session/prompt MUST NOT carry tools/messages.
	if _, ok := params["tools"]; ok {
		t.Errorf("params.tools present — session/prompt does not take tools (agent owns catalog)")
	}
	if _, ok := params["messages"]; ok {
		t.Errorf("params.messages present — session/prompt does not take messages (agent owns conversation state)")
	}
}

// TestBuildSessionUpdateNotificationIsNotification asserts session/update is a
// JSON-RPC notification: it has a method + params but NO id, and its params
// carry a sessionUpdate discriminator (prompt-turn.md).
func TestBuildSessionUpdateNotificationIsNotification(t *testing.T) {
	f := buildSessionUpdateNotification("sess_redacted", "agent_message_chunk", nil)
	if f == nil {
		t.Fatal("buildSessionUpdateNotification returned nil — main.go not implemented yet (RED)")
	}
	if f["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want \"2.0\"", f["jsonrpc"])
	}
	if f["method"] != "session/update" {
		t.Errorf("method = %v, want \"session/update\"", f["method"])
	}
	if _, hasID := f["id"]; hasID {
		t.Errorf("session/update has an id field — it is a notification and MUST NOT carry id")
	}
	params, ok := f["params"].(map[string]any)
	if !ok {
		t.Fatalf("params is not an object: %v", f["params"])
	}
	if _, ok := params["sessionId"]; !ok {
		t.Errorf("params.sessionId missing")
	}
	update, ok := params["update"].(map[string]any)
	if !ok {
		t.Fatalf("params.update is not an object: %v", params["update"])
	}
	if _, ok := update["sessionUpdate"]; !ok {
		t.Errorf("params.update.sessionUpdate discriminator missing (spec: {sessionUpdate: <kind>, ...})")
	}
}
