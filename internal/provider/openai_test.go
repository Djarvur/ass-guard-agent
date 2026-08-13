package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// cannedOpenAIToolCallsResponse is a minimal Chat Completions response with one
// tool_call (VERIFIED-FACTS.md item #2: choices[].message.tool_calls[],
// function.arguments is a JSON-encoded STRING).
func cannedOpenAIToolCallsResponse(name, argsJSON string) string {
	return `{
  "id": "chatcmpl-test",
  "object": "chat.completion",
  "model": "synth-model",
  "choices": [
    {
      "index": 0,
      "finish_reason": "tool_calls",
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {"id": "call_01", "type": "function",
           "function": {"name": "` + name + `", "arguments": ` + jsonQuote(argsJSON) + `}}
        ]
      }
    }
  ],
  "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
}`
}

// jsonQuote returns the JSON-string-encoded form of s (the OpenAI Arguments
// field is a JSON-encoded string, so an object becomes "{\"path\":\"go.mod\"}").
func jsonQuote(s string) string {
	b, marshalErr := json.Marshal(s)
	if marshalErr != nil {
		panic(marshalErr)
	}

	return string(b)
}

// TestOpenAIProvider_SendParsesToolCalls drives the OpenAI adapter through a
// mock Chat Completions endpoint and asserts the tools[].function wrapper is
// sent and the tool_calls parse to one ToolCall with a parsed JSON Input.
func TestOpenAIProvider_SendParsesToolCalls(t *testing.T) {
	t.Parallel()

	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("unexpected path %q (want .../chat/completions)", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		capturedBody = body

		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, cannedOpenAIToolCallsResponse(synthToolA, `{"path":"go.mod"}`))
	}))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewOpenAIProvider(
		provider.WithOpenAIAPIKey("test-key"),
		provider.WithOpenAIBaseURL(srv.URL+"/v1"),
	)

	resp, err := p.Send(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "x"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d, want 1", len(resp.ToolCalls))
	}

	if resp.ToolCalls[0].Name != synthToolA {
		t.Errorf("Name = %q", resp.ToolCalls[0].Name)
	}

	var in map[string]any
	err = json.Unmarshal(resp.ToolCalls[0].Input, &in)
	if err != nil {
		t.Fatalf("Input not valid JSON: %v", err)
	}

	if in[keyPath] != goModFile {
		t.Errorf("Input.path = %v", in[keyPath])
	}
	// The Chat Completions tools[].function wrapper must be in the request body.
	if !strings.Contains(string(capturedBody), `"function":{`) {
		t.Error("request body missing the tools[].function wrapper (Chat Completions shape)")
	}
}

// TestOpenAIProvider_ToolResultMessageShape confirms the follow-up is role:tool
// with tool_call_id + JSON-string content (VERIFIED-FACTS.md item #2).
func TestOpenAIProvider_ToolResultMessageShape(t *testing.T) {
	t.Parallel()

	p := provider.NewOpenAIProvider(provider.WithOpenAIAPIKey("k"))

	raw, err := p.ToolResultMessage("call_01", json.RawMessage(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}

	var msg map[string]any
	err = json.Unmarshal(raw, &msg)
	if err != nil {
		t.Fatal(err)
	}

	if msg["role"] != "tool" {
		t.Errorf("role = %v, want tool", msg["role"])
	}

	if msg["tool_call_id"] != "call_01" {
		t.Errorf("tool_call_id = %v", msg["tool_call_id"])
	}
}

// TestOpenAIProvider_NoAPIKeyErrors confirms the Tier-A fallback.
func TestOpenAIProvider_NoAPIKeyErrors(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	p := provider.NewOpenAIProvider(provider.WithOpenAIBaseURL("http://must-not-be-called.invalid"))

	minProf := loadProfile(t, "minimal")
	_, err := p.Send(context.Background(), &minProf, []shaper.Message{{Role: roleUser, Content: "x"}})
	if err == nil {
		t.Fatal("nil error with no API key; want non-nil")
	}
}
