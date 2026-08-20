package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"

	mcpgo "github.com/modelcontextprotocol/go-sdk/mcp"
)

// tracerEchoArg is the self-exec flag the cmd/ass-guard test binary recognizes
// to run as a minimal echo MCP server over stdio (the TRACER end-to-end path).
const tracerEchoArg = "__ass_guard_tracer_echo"

// Test-fixture constants (golangci goconst — these literals repeat across tests).
const (
	tracerToolUse   = "tool_use"
	tracerReadTool  = "Read"
	tracerHelloTool = "mcp__echo__hello"
)

// TestMain intercepts the self-exec echo-server mode: when re-invoked with the
// tracer flag, it runs a minimal go-sdk echo server (the `hello` tool) over
// stdio and exits. Otherwise it runs the normal test suite. This lets the
// TRACER test spawn its own binary as the MCP server, exercising the real
// CommandTransport stdio path end-to-end through sessionFor.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == tracerEchoArg {
		runTracerEchoServer()
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// runTracerEchoServer serves one tool: `hello` returns "hello, {name}". It
// speaks the real go-sdk stdio transport so the CommandTransport path is real.
func runTracerEchoServer() {
	ctx := context.Background()
	srv := mcpgo.NewServer(&mcpgo.Implementation{Name: "echo", Version: "v0"}, nil) //nolint:goconst // fixture

	srv.AddTool(&mcpgo.Tool{
		Name: "hello", Description: "Echo a greeting",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, req *mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		name := "world"

		if len(req.Params.Arguments) > 0 {
			var args map[string]any

			err := json.Unmarshal(req.Params.Arguments, &args)
			if err == nil {
				if n, ok := args["name"].(string); ok {
					name = n
				}
			}
		}

		return &mcpgo.CallToolResult{
			Content: []mcpgo.Content{&mcpgo.TextContent{Text: "hello, " + name}},
		}, nil
	})

	_ = srv.Run(ctx, &mcpgo.StdioTransport{})
}

// writeMcpJSON writes a .mcp.json in dir pointing the `echo` server at the test
// binary (self-exec). Returns the dir.
func writeMcpJSON(t *testing.T, dir string) string {
	t.Helper()

	testBin := os.Args[0]
	doc := map[string]any{
		"mcpServers": map[string]any{
			"echo": map[string]any{
				"command": testBin,
				"args":    []string{tracerEchoArg},
				"env":     map[string]string{"PATH": os.Getenv("PATH")},
			},
		},
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(dir+"/.mcp.json", raw, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

// newTracerRunner builds a sessionTurnRunner wired for the TRACER: a fake
// provider that emits the given scripted responses, no engine (so the stub
// inner executor is used and MCPExecutor wraps it).
func newTracerRunner(t *testing.T, script []provider.Response, workDir string) *sessionTurnRunner {
	t.Helper()

	bus := event.NewBus()
	prof := profile.Profile{Name: "tracer", Model: "test-model"}
	fp := &scriptedProvider{script: script, bus: bus}

	writeMcpJSON(t, workDir)

	return &sessionTurnRunner{
		bus:     bus,
		profile: prof,
		workDir: workDir,
		maxConc: 2,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider {
			return fp
		},
	}
}

// scriptedProvider is a streaming provider that replays a script of responses.
type scriptedProvider struct {
	script []provider.Response
	bus    *event.Bus
	n      int
}

func (p *scriptedProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, nil
}

func (p *scriptedProvider) Stream(
	_ context.Context, prof *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	body, marshalErr := json.Marshal(map[string]string{"model": prof.Model})
	if marshalErr != nil {
		panic(marshalErr)
	}

	p.bus.Publish(event.RequestShaped{VerbatimRequest: body, Profile: prof.Name, Timestamp: time.Now()})

	ch := make(chan provider.StreamChunk, 8)

	go func() {
		defer close(ch)

		p.n++

		var resp provider.Response
		if p.n <= len(p.script) {
			resp = p.script[p.n-1]
		} else {
			resp = provider.Response{FinishReason: "end_turn"}
		}

		for _, tc := range resp.ToolCalls {
			tcCopy := tc
			ch <- provider.StreamChunk{Type: tracerToolUse, ToolCall: &tcCopy, ToolCallID: tc.Name}
		}

		ch <- provider.StreamChunk{Type: "stop", FinishReason: resp.FinishReason}
	}()

	return ch, nil
}

func (p *scriptedProvider) ToolResultMessage(_ string, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// TestSessionForMCPProfileCopy verifies sessionFor builds a per-session profile
// COPY containing base + MCP decls, that r.profile is never mutated, and that
// two sessions have independent copies (T4 Test 1).
func TestSessionForMCPProfileCopy(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}

	workDir := t.TempDir()
	baseTools := []profile.Decl{{Name: tracerReadTool, Description: "read", InputSchema: json.RawMessage(`{}`)}}
	r := newTracerRunner(t, nil, workDir)
	r.profile.Tools = baseTools

	s1 := r.sessionFor(context.Background(), "sess-1")
	defer func() { _ = s1.Close() }()

	if !hasProfileTool(s1.Profile.Tools, tracerHelloTool) {
		t.Errorf("s1 profile missing mcp__echo__hello; have %v", profileToolNames(s1.Profile.Tools))
	}

	if !hasProfileTool(s1.Profile.Tools, "Read") {
		t.Error("s1 profile dropped a base tool")
	}

	// r.profile must NOT be mutated.
	for _, d := range r.profile.Tools {
		if d.Name == tracerHelloTool {
			t.Fatal("r.profile was mutated with an MCP tool (per-session copy leaked)")
		}
	}

	// A second session has its own copy.
	s2 := r.sessionFor(context.Background(), "sess-2")
	defer func() { _ = s2.Close() }()

	s1.Profile.Tools[0] = profile.Decl{Name: "MUTATED"}
	if hasProfileTool(r.profile.Tools, "MUTATED") {
		t.Fatal("mutating s1.Tools leaked into r.profile")
	}

	if hasProfileTool(s2.Profile.Tools, "MUTATED") {
		t.Fatal("mutating s1.Tools leaked into s2.Tools")
	}
}

// TestSessionForMCPCatalogRegistration verifies the session catalog contains the
// MCP tool entry (T4 Test 2).
func TestSessionForMCPCatalogRegistration(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}

	r := newTracerRunner(t, nil, t.TempDir())

	s := r.sessionFor(context.Background(), "sess-cat")
	defer func() { _ = s.Close() }()

	got, ok := s.Catalog.Get(tracerHelloTool)
	if !ok {
		t.Fatal("session catalog missing mcp__echo__hello")
	}

	if got.Name != tracerHelloTool {
		t.Errorf("catalog entry name = %q; want mcp__echo__hello", got.Name)
	}
}

// TestTracerMCPEndToEnd is the thinnest end-to-end MCP proof (T4 Test 3+4): a
// scripted provider emits tool_calls for mcp__echo__hello AND a built-in (Read);
// the session's Prompt loop routes the mcp__* call through MCPExecutor → Host →
// echo server ("hello, tracer") and the built-in through the inner executor
// (stub). The transcript contains both tool_call + tool_result lines.
func TestTracerMCPEndToEnd(t *testing.T) { //nolint:cyclop,funlen,paralleltest // subprocess e2e tracer
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}

	workDir := t.TempDir()
	script := []provider.Response{
		{
			FinishReason: tracerToolUse,
			ToolCalls: []provider.ToolCall{
				{Name: tracerHelloTool, Input: json.RawMessage(`{"name":"tracer"}`)},
				{Name: tracerReadTool, Input: json.RawMessage(`{}`)},
			},
		},
		{FinishReason: "end_turn"},
	}

	r := newTracerRunner(t, script, workDir)
	r.profile.Tools = []profile.Decl{
		{Name: tracerHelloTool, Description: "echo", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: tracerReadTool, Description: "read", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	sess := r.sessionFor(context.Background(), "sess-tracer")
	defer func() { _ = sess.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, pErr := sess.Prompt(ctx, []session.ContentBlock{{Type: blockText, Text: "call the echo tool"}})
	if pErr != nil {
		t.Fatalf("Prompt failed: %v", pErr)
	}

	lines, gErr := sess.Manager.ReadAll()
	if gErr != nil {
		t.Fatalf("ReadAll failed: %v", gErr)
	}

	var sawMCPCall, sawMCPResult, sawReadCall, sawReadResult bool

	for _, l := range lines {
		if l.Type == session.TypeToolCall && l.Name == tracerHelloTool {
			sawMCPCall = true
		}

		if l.Type == session.TypeToolCall && l.Name == "Read" {
			sawReadCall = true
		}

		// tool_result lines carry the tool name in ToolCallID, output in Output.
		if l.Type == session.TypeToolResult && l.ToolCallID == tracerHelloTool &&
			strings.Contains(string(l.Output), "hello, tracer") {
			sawMCPResult = true
		}

		if l.Type == session.TypeToolResult && l.ToolCallID == "Read" {
			sawReadResult = true
		}
	}

	if !sawMCPCall {
		t.Error("transcript missing a tool_call line for mcp__echo__hello")
	}

	if !sawMCPResult {
		t.Errorf("transcript missing tool_result containing 'hello, tracer'; %v", summarizeLines(lines))
	}

	if !sawReadCall {
		t.Error("transcript missing a tool_call line for Read (inner executor)")
	}

	if !sawReadResult {
		t.Error("transcript missing a tool_result for Read (inner executor did not run)")
	}
}

// TestSessionShutdownWired verifies the OnClose seam is wired so Close reaches
// the host (T4 Test 5): Close is idempotent and CloseSession (the ACP path)
// works.
func TestSessionShutdownWired(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}

	r := newTracerRunner(t, nil, t.TempDir())
	s := r.sessionFor(context.Background(), "sess-shutdown")

	if s.OnClose == nil {
		t.Fatal("OnClose seam not wired")
	}

	err := s.Close()
	if err != nil {
		t.Errorf("first Close error: %v", err)
	}

	// Idempotent: second Close is a no-op.
	err = s.Close()
	if err != nil {
		t.Errorf("second Close (idempotent) error: %v", err)
	}

	// CloseSession (the acp.SessionCloser path) on an already-closed session.
	err = r.CloseSession("sess-shutdown")
	if err != nil {
		t.Errorf("CloseSession after Close error: %v", err)
	}
}

func hasProfileTool(tools []profile.Decl, name string) bool {
	for _, d := range tools {
		if d.Name == name {
			return true
		}
	}

	return false
}

func profileToolNames(tools []profile.Decl) []string {
	out := make([]string, len(tools))
	for i, d := range tools {
		out[i] = d.Name
	}

	return out
}

func summarizeLines(lines []session.Line) []string {
	out := make([]string, len(lines))
	for i := range lines {
		out[i] = lines[i].Type + ":" + lines[i].Name
	}

	return out
}
