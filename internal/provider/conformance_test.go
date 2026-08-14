package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// conformanceCase is one row of the PROV-02 round-trip conformance table. Both
// adapters must satisfy the same contract through the common Provider interface.
type conformanceCase struct {
	name     string
	provider provider.Provider
	wantName string         // expected parsed ToolCall name
	wantArgs map[string]any // expected parsed ToolCall args
}

// TestConformance_BothAdapters proves the Anthropic and OpenAI adapters are
// drop-in interchangeable through the Provider interface (PROV-02). Each runs
// the same scenario: Send → ToolCalls, and ToolResultMessage builds the
// provider-correct follow-up.
func TestConformance_BothAdapters(t *testing.T) { //nolint:funlen,tparallel // shared httptest servers
	t.Parallel()
	prof := loadProfile(t, "minimal")
	msgs := []shaper.Message{{Role: roleUser, Content: "do the thing"}}

	// Anthropic arm.
	antSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		_, _ = io.WriteString(w,
			cannedAnthropicToolUseResponse(toolCallIDDefault, synthToolA, map[string]any{keyPath: goModFile}))
	}))
	defer antSrv.Close()

	ant := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(antSrv.URL),
	)

	// OpenAI arm.
	oaiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, cannedOpenAIToolCallsResponse(toolCallIDDefault, synthToolA, `{"path":"go.mod"}`))
	}))
	defer oaiSrv.Close()

	oai := provider.NewOpenAIProvider(
		provider.WithOpenAIAPIKey("test-key"),
		provider.WithOpenAIBaseURL(oaiSrv.URL+"/v1"),
	)

	cases := []conformanceCase{
		{name: "anthropic", provider: ant, wantName: synthToolA, wantArgs: map[string]any{keyPath: goModFile}},
		{name: "openai", provider: oai, wantName: synthToolA, wantArgs: map[string]any{keyPath: goModFile}},
	}
	for _, c := range cases { //nolint:paralleltest // shared httptest servers
		t.Run(c.name, func(t *testing.T) {
			resp, err := c.provider.Send(context.Background(), &prof, msgs)
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

			err = json.Unmarshal(resp.ToolCalls[0].Input, &in)
			if err != nil {
				t.Fatalf("Input not valid JSON: %v", err)
			}

			if in[keyPath] != goModFile {
				t.Errorf("Input.path = %v, want go.mod", in[keyPath])
			}

			// 08-07 T1 Test 7: the provider tool-call id round-trips through
			// the shared ToolCall type (both adapters must carry {id,name,input}).
			if resp.ToolCalls[0].ID != toolCallIDDefault {
				t.Errorf("ToolCalls[0].ID = %q, want %q", resp.ToolCalls[0].ID, toolCallIDDefault)
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

			err = json.Unmarshal(raw, &msg)
			if err != nil {
				t.Fatalf("ToolResultMessage output not valid JSON: %v", err)
			}
		})
	}
}

// TestConformance_ToolResultMessageMatchesShaper (08-07 T1 Test 7,
// single-source): the Anthropic adapter's ToolResultMessage output must EQUAL
// the Shaper-rendered tool_result message for the same input — the PROV-02
// surface and the turn-loop Shaper path cannot silently diverge.
func TestConformance_ToolResultMessageMatchesShaper(t *testing.T) {
	t.Parallel()

	prof := loadProfile(t, "minimal")
	ant := provider.NewAnthropicProvider(shaper.New(), provider.WithAnthropicAPIKey("test-key"))

	result := json.RawMessage(`{"ok":true}`)

	raw, err := ant.ToolResultMessage(toolCallIDDefault, result)
	if err != nil {
		t.Fatalf("ToolResultMessage: %v", err)
	}

	shaperMsgs := []shaper.Message{
		{Role: roleTool, ToolCallID: toolCallIDDefault, ToolName: synthToolA, Content: string(result)},
	}

	params, _, shapeErr := shaper.New().Shape(&prof, shaperMsgs)
	if shapeErr != nil {
		t.Fatalf("Shape: %v", shapeErr)
	}

	if len(params.Messages) != 1 {
		t.Fatalf("len(shaper Messages) = %d, want 1", len(params.Messages))
	}

	shaped, marshalErr := json.Marshal(params.Messages[0])
	if marshalErr != nil {
		t.Fatalf("marshal shaper message: %v", marshalErr)
	}

	if string(raw) != string(shaped) {
		t.Errorf("ToolResultMessage and the Shaper rendering diverge:\n provider: %s\n shaper:   %s", raw, shaped)
	}
}

// TestConformance_ToolCallAlias (08-07 T1 Test 1): provider.ToolCall must be
// an alias of shaper.ToolCall (the Message pattern extended to the call type) —
// one type flows across the seam; toolexec/session/parity compile unchanged.
func TestConformance_ToolCallAlias(t *testing.T) {
	t.Parallel()

	tc := shaper.ToolCall{ID: "call_x", Name: toolNameRead, Input: json.RawMessage(`{}`)}

	// Passing a shaper.ToolCall where a provider.ToolCall parameter is
	// expected compiles ONLY when the two are the same type (assignability
	// between distinct named types with identical fields does not hold).
	acceptProviderToolCall(tc)

	if tc.ID != "call_x" {
		t.Errorf("alias round-trip lost the ID: %+v", tc)
	}
}

// acceptProviderToolCall exists so the alias test can prove shaper.ToolCall is
// assignable to provider.ToolCall (the alias) without a local declaration.
func acceptProviderToolCall(_ provider.ToolCall) {}

// TestConformance_InterfaceSatisfied is a compile-time-ish guarantee that both
// adapters implement the Provider interface (the test fails to compile otherwise,
// but this makes the intent explicit and greppable).
func TestConformance_InterfaceSatisfied(t *testing.T) {
	t.Parallel()

	var (
		_ provider.Provider = (*provider.AnthropicProvider)(nil)
		_ provider.Provider = (*provider.OpenAIProvider)(nil)
	)
	// sanity: the test profile loads
	if loadProfile(t, "minimal").Name != "minimal" {
		t.Fatal("minimal fixture did not load")
	}

	_ = profile.Profile{}
}
