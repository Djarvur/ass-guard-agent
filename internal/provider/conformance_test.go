package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/provider"
	"github.com/djarvur/ass-guard-agent/internal/shaper"
)

// conformanceCase is one row of the PROV-02 round-trip conformance table. Both
// adapters must satisfy the same contract through the common Provider interface.
type conformanceCase struct {
	name      string
	provider  provider.Provider
	servePath string         // the path the mock expects
	serveBody func() string  // canned response body
	wantName  string         // expected parsed ToolCall name
	wantArgs  map[string]any // expected parsed ToolCall args
}

// TestConformance_BothAdapters proves the Anthropic and OpenAI adapters are
// drop-in interchangeable through the Provider interface (PROV-02). Each runs
// the same scenario: Send → ToolCalls, and ToolResultMessage builds the
// provider-correct follow-up.
func TestConformance_BothAdapters(t *testing.T) {
	prof := loadProfile(t, "minimal")
	msgs := []shaper.Message{{Role: "user", Content: "do the thing"}}

	// Anthropic arm.
	antSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, cannedAnthropicToolUseResponse("synth_tool_a", map[string]any{"path": "go.mod"}))
	}))
	defer antSrv.Close()
	ant := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(antSrv.URL),
	)

	// OpenAI arm.
	oaiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, cannedOpenAIToolCallsResponse("synth_tool_a", `{"path":"go.mod"}`))
	}))
	defer oaiSrv.Close()
	oai := provider.NewOpenAIProvider(
		provider.WithOpenAIAPIKey("test-key"),
		provider.WithOpenAIBaseURL(oaiSrv.URL+"/v1"),
	)

	cases := []conformanceCase{
		{name: "anthropic", provider: ant, wantName: "synth_tool_a", wantArgs: map[string]any{"path": "go.mod"}},
		{name: "openai", provider: oai, wantName: "synth_tool_a", wantArgs: map[string]any{"path": "go.mod"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			resp, err := c.provider.Send(context.Background(), prof, msgs)
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			if len(resp.ToolCalls) != 1 {
				t.Fatalf("ToolCalls = %d, want 1", len(resp.ToolCalls))
			}
			if resp.ToolCalls[0].Name != c.wantName {
				t.Errorf("Name = %q, want %q", resp.ToolCalls[0].Name, c.wantName)
			}
			var in map[string]any
			if err := json.Unmarshal(resp.ToolCalls[0].Input, &in); err != nil {
				t.Fatalf("Input not valid JSON: %v", err)
			}
			if in["path"] != "go.mod" {
				t.Errorf("Input.path = %v, want go.mod", in["path"])
			}
			if len(resp.Raw) == 0 {
				t.Error("Raw empty; want captured response bytes")
			}

			// ToolResultMessage builds the provider-correct follow-up (PROV-02
			// TranslateFromInternal). Both must produce valid JSON the consumer
			// can hand back to the provider.
			raw, err := c.provider.ToolResultMessage("call_01", json.RawMessage(`{"ok":true}`))
			if err != nil {
				t.Fatalf("ToolResultMessage: %v", err)
			}
			var msg map[string]any
			if err := json.Unmarshal(raw, &msg); err != nil {
				t.Fatalf("ToolResultMessage output not valid JSON: %v", err)
			}
		})
	}
}

// TestConformance_InterfaceSatisfied is a compile-time-ish guarantee that both
// adapters implement the Provider interface (the test fails to compile otherwise,
// but this makes the intent explicit and greppable).
func TestConformance_InterfaceSatisfied(t *testing.T) {
	var _ provider.Provider = (*provider.AnthropicProvider)(nil)
	var _ provider.Provider = (*provider.OpenAIProvider)(nil)
	// sanity: the test profile loads
	if loadProfile(t, "minimal").Name != "minimal" {
		t.Fatal("minimal fixture did not load")
	}
	_ = profile.Profile{}
}
