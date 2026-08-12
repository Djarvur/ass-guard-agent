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

	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/provider"
	"github.com/djarvur/ass-guard-agent/internal/shaper"
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
// drainSSE can parse (VERIFIED-FACTS.md item #1: stop_reason "tool_use").
func cannedAnthropicToolUseResponse(name string, input map[string]any) string {
	inputJSON, _ := json.Marshal(input)
	var b strings.Builder
	b.WriteString("data: " + `{"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":0}}}` + "\n\n")
	b.WriteString("data: " + `{"type":"content_block_start","content_block":{"type":"tool_use","id":"call_01","name":"` + name + `","input":` + string(inputJSON) + `}}` + "\n\n")
	b.WriteString("data: " + `{"type":"message_delta","delta":{"stop_reason":"tool_use"}}` + "\n\n")
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// TestAnthropicProvider_SendParsesToolUse drives the adapter through a mock
// Anthropic endpoint and asserts the tool_use block parses to a ToolCall.
func TestAnthropicProvider_SendParsesToolUse(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.Header().Set("content-type", "text/event-stream")
		_, _ = io.WriteString(w, cannedAnthropicToolUseResponse("synth_tool_a", map[string]any{"path": "go.mod"}))
	}))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	resp, err := p.Send(context.Background(), prof, []shaper.Message{{Role: "user", Content: "read go.mod"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.FinishReason != "tool_use" {
		t.Errorf("FinishReason = %q, want tool_use", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "synth_tool_a" {
		t.Errorf("ToolCalls[0].Name = %q, want synth_tool_a", resp.ToolCalls[0].Name)
	}
	var in map[string]any
	if err := json.Unmarshal(resp.ToolCalls[0].Input, &in); err != nil {
		t.Fatalf("unmarshal Input: %v", err)
	}
	if in["path"] != "go.mod" {
		t.Errorf("Input.path = %v, want go.mod", in["path"])
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
	if !strings.Contains(string(capturedBody), "synth_tool_a") {
		t.Error("shaped request body missing the tool declaration")
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
	_, err := p.Send(context.Background(), prof, []shaper.Message{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("Send returned nil error with no API key; want non-nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "api key") && !strings.Contains(strings.ToLower(err.Error()), "apikey") {
		t.Errorf("error message %q does not mention the missing API key", err.Error())
	}
}
