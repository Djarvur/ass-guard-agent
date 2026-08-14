package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

func loadProfile(t *testing.T, name string) profile.Profile {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "profile", "testdata"))
	if err != nil {
		t.Fatal(err)
	}

	p, err := profile.NewLoader(root).Load(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}

	return p
}

// cannedAnthropicToolUseResponse is a minimal Anthropic Messages SSE response
// carrying one tool_use block. Z.ai requires streaming (stream:true); Send
// delegates to Stream, so tests must return SSE-formatted data: lines that
// drainSSE can parse (VERIFIED-FACTS.md item #1: stop_reason "tool_use"). The
// tool_use block carries the provider's real call id (08-07: the id must
// survive the seam for tool_use/tool_result pairing).
func cannedAnthropicToolUseResponse(id, name string, input map[string]any) string {
	inputJSON, marshalErr := json.Marshal(input)
	if marshalErr != nil {
		panic(marshalErr)
	}
	// partial_json is a STRING in the real Anthropic SSE protocol (JSON fragments)
	partialJSONStr, marshalErr := json.Marshal(string(inputJSON)) // produces a quoted+escaped string
	if marshalErr != nil {
		panic(marshalErr)
	}

	var b strings.Builder

	b.WriteString("data: " +
		`{"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":0}}}` + "\n\n")
	b.WriteString("data: " +
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"` +
		id + `","name":"` + name + `"}}` + "\n\n")
	b.WriteString(`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":` +
		string(partialJSONStr) + `}}` + "\n\n")
	b.WriteString("data: " + `{"type":"content_block_stop","index":0}` + "\n\n")
	b.WriteString("data: " + `{"type":"message_delta","delta":{"stop_reason":"tool_use"}}` + "\n\n")
	b.WriteString("data: [DONE]\n\n")

	return b.String()
}

// TestAnthropicProvider_SendParsesToolUse drives the adapter through a mock
// Anthropic endpoint and asserts the tool_use block parses to a ToolCall.
func TestAnthropicProvider_SendParsesToolUse(t *testing.T) { //nolint:funlen // comprehensive test scenario
	t.Parallel()

	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		capturedBody = body

		w.Header().Set("content-type", "text/event-stream")
		_, _ = io.WriteString(w,
			cannedAnthropicToolUseResponse(toolCallIDDefault, synthToolA, map[string]any{keyPath: goModFile}))
	}))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	resp, err := p.Send(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "read go.mod"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if resp.FinishReason != blockToolUse {
		t.Errorf("FinishReason = %q, want tool_use", resp.FinishReason)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}

	if resp.ToolCalls[0].Name != synthToolA {
		t.Errorf("ToolCalls[0].Name = %q, want synth_tool_a", resp.ToolCalls[0].Name)
	}

	var in map[string]any

	err = json.Unmarshal(resp.ToolCalls[0].Input, &in)
	if err != nil {
		t.Fatalf("unmarshal Input: %v", err)
	}

	if in[keyPath] != goModFile {
		t.Errorf("Input.path = %v, want go.mod", in[keyPath])
	}

	if len(resp.Raw) == 0 {
		t.Error("Raw is empty; want the captured response bytes")
	}

	// The shaped request body the mock received must carry the profile's system
	// text + tool declaration (proof the Shaper drove the adapter).
	if len(capturedBody) == 0 {
		t.Fatal("capturedBody empty; mock never saw a request")
	}

	if !strings.Contains(string(capturedBody), "You are a synthetic test agent.") {
		t.Error("shaped request body missing the synthetic system block")
	}

	if !strings.Contains(string(capturedBody), synthToolA) {
		t.Error("shaped request body missing the tool declaration")
	}
}

// TestToolCallID_AnthropicStreamCarriesID (08-07 T1 Test 2): the SSE stream's
// content_block_start carries {"type":"tool_use","id":"call_abc123"} and the
// parsed Response.ToolCalls[0].ID must equal it — the real provider id is the
// join key for tool_use/tool_result pairing (root cause 1 of the 08-06 gate).
func TestToolCallID_AnthropicStreamCarriesID(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		_, _ = io.WriteString(w, cannedAnthropicToolUseResponse("call_abc123", toolNameRead,
			map[string]any{keyPath: goModFile}))
	}))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	resp, err := p.Send(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "read"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}

	if resp.ToolCalls[0].ID != "call_abc123" {
		t.Errorf("ToolCalls[0].ID = %q, want call_abc123 (the provider id was dropped)", resp.ToolCalls[0].ID)
	}
}

// TestAnthropicProvider_NoAPIKeyErrors asserts a missing API key surfaces a
// clear error rather than a silent call with empty auth (Tier-A fallback).
func TestAnthropicProvider_NoAPIKeyErrors(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "")
	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicBaseURL("http://must-not-be-called.invalid"),
	)

	_, err := p.Send(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "x"}})
	if err == nil {
		t.Fatal("Send returned nil error with no API key; want non-nil")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "api key") &&
		!strings.Contains(strings.ToLower(err.Error()), "apikey") {
		t.Errorf("error message %q does not mention the missing API key", err.Error())
	}
}
