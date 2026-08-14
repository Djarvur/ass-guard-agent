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
// function.arguments is a JSON-encoded STRING). The tool_call carries the
// provider's real call id (08-07: the id must survive the seam).
func cannedOpenAIToolCallsResponse(id, name, argsJSON string) string {
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
          {"id": "` + id + `", "type": "function",
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
		_, _ = io.WriteString(w, cannedOpenAIToolCallsResponse(toolCallIDDefault, synthToolA, `{"path":"go.mod"}`))
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

// TestToolCallID_OpenAICarriesID (08-07 T1 Test 3): a canned Chat Completions
// response whose tool_calls[0].id is "call_oai_1" must parse to
// Response.ToolCalls[0].ID == "call_oai_1" (go-openai's ToolCall carries the
// ID; the seam must not drop it).
func TestToolCallID_OpenAICarriesID(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, cannedOpenAIToolCallsResponse("call_oai_1", synthToolA, `{"path":"go.mod"}`))
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
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}

	if resp.ToolCalls[0].ID != "call_oai_1" {
		t.Errorf("ToolCalls[0].ID = %q, want call_oai_1 (the provider id was dropped)", resp.ToolCalls[0].ID)
	}
}

// TestMidTurn_OpenAIRendering (08-07 T1 Test 6): structured mid-turn messages
// build to the OpenAI-native forms on the Send path — an assistant message
// carries tool_calls[{id,type:function,function:{name,arguments}}] (arguments =
// the Input JSON verbatim) and a tool-role message becomes
// {role:"tool", tool_call_id, content}.
func TestMidTurn_OpenAIRendering(t *testing.T) {
	t.Parallel()

	var captured []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, cannedOpenAIToolCallsResponse(toolCallIDDefault, synthToolA, `{"path":"go.mod"}`))
	}))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewOpenAIProvider(
		provider.WithOpenAIAPIKey("test-key"),
		provider.WithOpenAIBaseURL(srv.URL+"/v1"),
		provider.WithOpenAIRequestCapture(func(body []byte, _ map[string]string) { captured = body }),
	)

	msgs := []shaper.Message{
		{Role: roleUser, Content: "go"},
		{
			Role: "assistant", ToolCalls: []shaper.ToolCall{
				{ID: "call_oai_9", Name: "Read", Input: json.RawMessage(`{"path":"go.mod"}`)},
			},
		},
		{Role: "tool", ToolCallID: "call_oai_9", ToolName: "Read", Content: `{"ok":true}`},
	}

	_, err := p.Send(context.Background(), &prof, msgs)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var req struct {
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(captured, &req); err != nil {
		t.Fatalf("captured request not valid JSON: %v", err)
	}

	if len(req.Messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(req.Messages))
	}

	am := req.Messages[1]
	if am.Role != "assistant" || len(am.ToolCalls) != 1 {
		t.Fatalf("assistant message = {role:%s, tool_calls:%d}; want {assistant,1}", am.Role, len(am.ToolCalls))
	}

	tc := am.ToolCalls[0]
	if tc.ID != "call_oai_9" || tc.Type != "function" || tc.Function.Name != "Read" {
		t.Errorf("tool_calls[0] = {id:%s,type:%s,name:%s}; want {call_oai_9,function,Read}", tc.ID, tc.Type, tc.Function.Name)
	}

	if tc.Function.Arguments != `{"path":"go.mod"}` {
		t.Errorf("tool_calls[0].arguments = %q, want the Input JSON verbatim", tc.Function.Arguments)
	}

	tm := req.Messages[2]
	if tm.Role != "tool" || tm.ToolCallID != "call_oai_9" {
		t.Errorf("tool message = {role:%s, tool_call_id:%s}; want {tool,call_oai_9}", tm.Role, tm.ToolCallID)
	}

	if tm.Content != `{"ok":true}` {
		t.Errorf("tool message content = %q, want the result text", tm.Content)
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
