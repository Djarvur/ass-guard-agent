// SPDX-License-Identifier: MIT
//
// RED-phase tests for the #2 go-openai tool-schema spike (Phase 0, Plan 00-02).
//
// These tests assert the OFFLINE-VERIFIABLE part of <behavior>: the Chat Completions
// request/response schema that sashabaranov/go-openai@v1.42.0 produces. They do NOT
// exercise the network (that is the round-trip + follow-up behavior, asserted by
// main.go against a live provider). Per the TDD flow these MUST fail until main.go
// exists (the test file imports nothing from this package yet — it asserts the
// library's serialization directly so the RED bar is driven by a missing helper that
// main.go will provide: the canonical request builder + the schema assertions).
//
// Transport discipline (AGENTS.md): this is a test file; testing output is not ACP
// traffic, so it goes through the normal `go test` runner.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// TestRequestToolSerializesToChatCompletionsShape is the load-bearing schema-fidelity
// assertion for STACK item #2. go-openai's Tool/FunctionDefinition types MUST serialize
// to the OpenAI Chat Completions wrapper: {"type":"function","function":{name,description?,parameters}}.
// This is distinct from the newer Responses API shape (no function wrapper) and from
// Anthropic's flat {name,description,input_schema}.
func TestRequestToolSerializesToChatCompletionsShape(t *testing.T) {
	req := buildWeatherToolRequest()
	if req == nil {
		t.Fatal("buildWeatherToolRequest returned nil — main.go not implemented yet (RED)")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("re-unmarshal request: %v", err)
	}
	tools, ok := generic["tools"]
	if !ok {
		t.Fatalf("request has no tools field; got keys: %v; raw=%s", topKeys(generic), string(raw))
	}
	toolsArr, ok := tools.([]any)
	if !ok || len(toolsArr) == 0 {
		t.Fatalf("tools is not a non-empty array: %v", tools)
	}
	first, ok := toolsArr[0].(map[string]any)
	if !ok {
		t.Fatalf("tools[0] is not an object: %v", toolsArr[0])
	}
	if first["type"] != "function" {
		t.Errorf("tools[0].type = %v, want \"function\"", first["type"])
	}
	fn, ok := first["function"].(map[string]any)
	if !ok {
		t.Fatalf("tools[0].function is not an object: %v", first["function"])
	}
	if fn["name"] != "get_weather" {
		t.Errorf("tools[0].function.name = %v, want get_weather", fn["name"])
	}
	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("tools[0].function.parameters is not an object: %v", fn["parameters"])
	}
	if params["type"] != "object" {
		t.Errorf("parameters.type = %v, want object", params["type"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("parameters.properties is not an object: %v", params["properties"])
	}
	loc, ok := props["location"].(map[string]any)
	if !ok {
		t.Fatalf("properties.location is not an object: %v", props["location"])
	}
	if loc["type"] != "string" {
		t.Errorf("properties.location.type = %v, want string", loc["type"])
	}
	reqArr, _ := params["required"].([]any)
	if len(reqArr) != 1 || reqArr[0] != "location" {
		t.Errorf("parameters.required = %v, want [location]", params["required"])
	}
}

// TestToolCallArgumentsIsStringType asserts the load-bearing divergence point: in the
// Chat Completions schema, tool_calls[].function.arguments is a JSON-ENCODED STRING,
// not a parsed object. go-openai's ToolCall.Function.Arguments must be string-typed.
func TestToolCallArgumentsIsStringType(t *testing.T) {
	tc := openai.ToolCall{
		ID:   "call_abc123",
		Type: openai.ToolTypeFunction,
		Function: openai.FunctionCall{
			Name:      "get_weather",
			Arguments: `{"location":"Paris"}`,
		},
	}
	raw, err := json.Marshal(tc)
	if err != nil {
		t.Fatalf("marshal toolcall: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("re-unmarshal toolcall: %v", err)
	}
	fn, ok := generic["function"].(map[string]any)
	if !ok {
		t.Fatalf("toolcall.function not an object: %v", generic["function"])
	}
	args, ok := fn["arguments"]
	if !ok {
		t.Fatalf("toolcall.function.arguments missing")
	}
	argsStr, isStr := args.(string)
	if !isStr {
		t.Fatalf("arguments is %T, want string (Chat Completions shape)", args)
	}
	// And the string must itself be parseable JSON into the declared schema.
	var parsed map[string]string
	if err := json.Unmarshal([]byte(argsStr), &parsed); err != nil {
		t.Fatalf("arguments string is not valid JSON: %v (raw=%q)", err, argsStr)
	}
	if parsed["location"] != "Paris" {
		t.Errorf("arguments.location = %q, want Paris", parsed["location"])
	}
}

// TestToolResultMessageSerializesChatCompletionsShape asserts the tool-result message
// (role:"tool" + tool_call_id + JSON string content) is the Chat Completions shape the
// follow-up turn sends back to the provider.
func TestToolResultMessageSerializesChatCompletionsShape(t *testing.T) {
	msg := buildToolResultMessage("call_abc123", `{"temperature":25}`)
	if msg == nil {
		t.Fatal("buildToolResultMessage returned nil — main.go not implemented yet (RED)")
	}
	if msg.Role != "tool" {
		t.Errorf("Role = %q, want tool", msg.Role)
	}
	if msg.ToolCallID != "call_abc123" {
		t.Errorf("ToolCallID = %q, want call_abc123", msg.ToolCallID)
	}
	if !strings.Contains(msg.Content, "temperature") {
		t.Errorf("Content = %q, want a JSON string containing temperature", msg.Content)
	}
	// The content must be a JSON string that round-trips.
	var parsed map[string]int
	if err := json.Unmarshal([]byte(msg.Content), &parsed); err != nil {
		t.Fatalf("tool-result Content is not valid JSON: %v", err)
	}
	if parsed["temperature"] != 25 {
		t.Errorf("temperature = %d, want 25", parsed["temperature"])
	}
}

// TestJsonSchemaDefinitionProducesLocationSchema is a guard that the jsonschema.Definition
// helper (used by buildWeatherToolRequest) produces the expected {type,properties,required}
// shape — so the request assertion above is testing real library output, not hand-built JSON.
func TestJsonSchemaDefinitionProducesLocationSchema(t *testing.T) {
	def := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"location": {Type: jsonschema.String},
		},
		Required: []string{"location"},
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if !strings.Contains(string(raw), `"type":"object"`) {
		t.Errorf("definition missing type=object: %s", string(raw))
	}
	if !strings.Contains(string(raw), `"location":{"type":"string"}`) {
		t.Errorf("definition missing location string property: %s", string(raw))
	}
	if !strings.Contains(string(raw), `"required":["location"]`) {
		t.Errorf("definition missing required=[location]: %s", string(raw))
	}
}

func topKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
