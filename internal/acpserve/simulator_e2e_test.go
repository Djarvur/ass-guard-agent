package acpserve //nolint:testpackage // internal package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
)

// The 16-06 Zed-client simulator (ACP-03/ACP-08 whole-phase story): a small
// scripted client speaking newline-delimited JSON-RPC over io.Pipe drives the
// REAL acpserve.Run composition end-to-end — initialize → session/new →
// prompt streaming → set_config_option → cancel — against a scripted SSE
// provider stub. Every seam the phase built is exercised in one process with
// no env flags and no live model; the script reads as the phase's
// self-running demonstration. The v1 vocabulary assertions here are the
// Pitfall-7 drift guard at the highest level.
//
// Determinism: fixed script, guard timeouts on every wait (no open-ended
// sleeps), sequential stages over ONE serve instance, string request ids
// mirroring Zed's UUID style (D-15).

// The simulated editor's vocabulary (v1 spellings only — Pitfall 7).
const (
	simMethodSessionUpdate = "session/update"
	simMethodElicitation   = "elicitation/create"
	simMethodCancelRequest = "$/cancel_request"

	simKindToolCall      = "tool_call"
	simKindPlan          = "plan"
	simKindChunk         = "agent_message_chunk"
	simKindConfigOptions = "config_option_update"

	simStopEndTurn   = "end_turn"
	simStopCancelled = "cancelled"

	simToolRead = "Read"
	simToolTodo = "TodoWrite"

	simGuardTimeout = 10 * time.Second
)

// simPhase is one content block of a scripted provider response: a text chunk
// or a tool_use block (id + captured tool name + raw input JSON).
type simPhase struct {
	text   string // non-empty: a text content block
	callID string // non-empty: a tool_use block, with name + input
	name   string
	input  string
}

// simTurnScript is ONE provider request's scripted response: either a list of
// content phases (assembled into a real Anthropic SSE stream) or a hold — the
// request is accepted and kept open until the client's cancel aborts it.
type simTurnScript struct {
	phases []simPhase
	hold   bool
}

// simStub is the scripted SSE provider the temp config points the anthropic
// provider at. Each HTTP request consumes the next script entry; every request
// body's model is recorded (the "captured provider request" lens for the
// model-switch story).
type simStub struct {
	script      []simTurnScript
	models      []string
	modelsMu    sync.Mutex
	calls       atomic.Int64
	holdOnce    sync.Once
	holdReached chan struct{}
	srv         *httptest.Server
}

func newSimStub(script []simTurnScript) *simStub {
	st := &simStub{script: script, holdReached: make(chan struct{})}
	st.srv = httptest.NewServer(http.HandlerFunc(st.serveSSE))

	return st
}

func (s *simStub) serveSSE(w http.ResponseWriter, r *http.Request) {
	body, rerr := io.ReadAll(r.Body)
	if rerr == nil {
		var req struct {
			Model string `json:"model"`
		}

		jerr := json.Unmarshal(body, &req)
		if jerr == nil && req.Model != "" {
			s.modelsMu.Lock()
			s.models = append(s.models, req.Model)
			s.modelsMu.Unlock()
		}
	}

	idx := int(s.calls.Add(1)) - 1
	if idx >= len(s.script) || s.script[idx].hold {
		// The hold stage: accept the request, then keep it open until the
		// client's cancel aborts it (the provider request rides the turn ctx).
		s.holdOnce.Do(func() { close(s.holdReached) })

		select {
		case <-r.Context().Done():
		case <-time.After(60 * time.Second): // guard: the turn is cancelled long before
		}

		return
	}

	w.Header().Set("Content-Type", "text/event-stream")

	flusher, _ := w.(http.Flusher)

	for _, frame := range simSSEFrames(s.script[idx].phases) {
		_, _ = io.WriteString(w, frame)

		if flusher != nil {
			flusher.Flush()
		}
	}
}

func (s *simStub) recordedModels() []string {
	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()

	return append([]string(nil), s.models...)
}

// waitHold blocks until the stub is HOLDING a provider request open — the
// deterministic "turn in flight" point for the cancel stage.
func (s *simStub) waitHold(t *testing.T) {
	t.Helper()

	select {
	case <-s.holdReached:
	case <-time.After(simGuardTimeout):
		t.Fatal("the stub's hold request never went in flight")
	}
}

// simSSEFrames assembles one scripted response into real Anthropic SSE frames:
// message_start (usage) → one content block per phase → message_delta (stop
// tool_use when any phase was a tool call) → [DONE].
func simSSEFrames(phases []simPhase) []string {
	frames := []string{
		simSSE(`{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}`),
	}

	stop := simStopEndTurn

	for i, ph := range phases {
		idx := strconv.Itoa(i)
		frames = append(frames, simPhaseSSE(ph, idx)...)

		if ph.callID != "" {
			stop = "tool_use"
		}
	}

	return append(frames,
		simSSE(`{"type":"message_delta","delta":{"stop_reason":"`+stop+`"}}`),
		"data: [DONE]\n\n")
}

// simPhaseSSE renders one phase's content_block frames (start → delta → stop).
func simPhaseSSE(ph simPhase, idx string) []string {
	if ph.callID != "" {
		return []string{
			simSSE(`{"type":"content_block_start","index":` + idx +
				`,"content_block":{"type":"tool_use","id":` + simJSONStr(ph.callID) +
				`,"name":` + simJSONStr(ph.name) + `}}`),
			simSSE(`{"type":"content_block_delta","index":` + idx +
				`,"delta":{"type":"input_json_delta","partial_json":` + simJSONStr(ph.input) + `}}`),
			simSSE(`{"type":"content_block_stop","index":` + idx + `}`),
		}
	}

	return []string{
		simSSE(`{"type":"content_block_start","index":` + idx +
			`,"content_block":{"type":"text","text":""}}`),
		simSSE(`{"type":"content_block_delta","index":` + idx +
			`,"delta":{"type":"text_delta","text":` + simJSONStr(ph.text) + `}}`),
		simSSE(`{"type":"content_block_stop","index":` + idx + `}`),
	}
}

func simSSE(payload string) string { return "data: " + payload + "\n\n" }

func simJSONStr(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err) // static strings only — cannot fail
	}

	return string(b)
}

// simClient is the scripted Zed client: it writes newline-delimited JSON-RPC
// frames into the serve's stdin and reads stdout frames one at a time, every
// wait bounded by the guard timeout. Read order IS wire emission order (the
// pipe preserves byte order; the single-drain emitter serializes updates).
type simClient struct {
	t      *testing.T
	in     *io.PipeWriter
	frames chan *acp.Message
	seen   int
}

func newSimClient(t *testing.T, srvOut *io.PipeReader, srvIn *io.PipeWriter) *simClient {
	t.Helper()

	c := &simClient{t: t, in: srvIn, frames: make(chan *acp.Message, 512)}

	go func() {
		sc := bufio.NewScanner(srvOut)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}

			var m acp.Message

			if json.Unmarshal(line, &m) == nil {
				c.frames <- &m
			}
		}

		close(c.frames)
	}()

	return c
}

func (c *simClient) sendf(format string, args ...any) {
	c.t.Helper()

	_, err := fmt.Fprintf(c.in, format+"\n", args...)
	if err != nil {
		c.t.Fatalf("simulator: write frame: %v", err)
	}
}

func (c *simClient) next() *acp.Message {
	c.t.Helper()

	select {
	case m, ok := <-c.frames:
		if !ok {
			c.t.Fatalf("simulator: stdout closed early (seen=%d frames)", c.seen)
		}

		c.seen++

		return m
	case <-time.After(simGuardTimeout):
		c.t.Fatalf("simulator: timed out waiting for a frame (seen=%d)", c.seen)
	}

	return nil
}

// nextResponse reads until the response carrying id arrives.
func (c *simClient) nextResponse(id string) *acp.Message {
	c.t.Helper()

	for {
		m := c.next()
		if isResponseID(m, id) {
			return m
		}
	}
}

// simUpdate is one decoded session/update payload.
type simUpdate struct {
	Kind       string                  `json:"sessionUpdate"` //nolint:tagliatelle // ACP wire field
	ToolCallID string                  `json:"toolCallId"`    //nolint:tagliatelle // ACP wire field
	Title      string                  `json:"title"`
	Entries    []acp.PlanEntry         `json:"entries"`
	Options    []acp.ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
}

func decodeSimUpdate(t *testing.T, m *acp.Message) simUpdate {
	t.Helper()

	var params struct {
		Update simUpdate `json:"update"`
	}

	err := json.Unmarshal(m.Params, &params)
	if err != nil {
		t.Fatalf("decode session/update: %v (%s)", err, string(m.Params))
	}

	return params.Update
}

// isResponseID reports whether m is the response carrying id (string ids —
// Zed's UUID style, D-15).
func isResponseID(m *acp.Message, id string) bool { return m.ID != nil && string(m.ID) == id }

// simulatorWorkDir builds the temp project: a real .ass-guard/config.yaml
// aiming the anthropic provider at stubURL (defaults-only tiers — the embedded
// floor resolves), plus the notes.md file the scripted Read call opens.
func simulatorWorkDir(t *testing.T, stubURL string) string {
	t.Helper()

	workDir := t.TempDir()

	err := os.MkdirAll(filepath.Join(workDir, ".ass-guard"), 0o750)
	if err != nil {
		t.Fatalf("mkdir .ass-guard: %v", err)
	}

	config := "providers:\n  anthropic:\n    base_url: " + strconv.Quote(stubURL) +
		"\n    api_key: \"sk-simulator-canary\"\n"

	werr := os.WriteFile(filepath.Join(workDir, ".ass-guard", "config.yaml"), []byte(config), 0o600)
	if werr != nil {
		t.Fatalf("write config.yaml: %v", werr)
	}

	nerr := os.WriteFile(filepath.Join(workDir, "notes.md"), []byte("the file the turn reads\n"), 0o600)
	if nerr != nil {
		t.Fatalf("write notes.md: %v", nerr)
	}

	return workDir
}

// TestZedSimulatorE2E is the phase's whole story in one deterministic script;
// each stage below is one sentence of it:
//
//  1. the editor introduces itself (initialize: capabilities + _meta default)
//  2. session/new presents the option menu with live effective values
//  3. a prompt streams tool_call → plan → message chunks, then the response
//  4. the editor switches the model; the set persists and the next request
//     carries it
//  5. a mid-turn cancel resolves with the cancelled stop reason
func TestZedSimulatorE2E(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "") // the config literal key wins (deterministic provider construction)

	stubPlaceholder := "PENDING_STUB_URL"
	stub := newSimStub(simTurnScripts(stubPlaceholder))
	t.Cleanup(stub.srv.Close)

	workDir := simulatorWorkDir(t, stubPlaceholder)

	// Repoint the scripted Read target and the config at real paths/URLs now
	// that the stub and workdir exist (Run loads the config at startup).
	stub.script[0].phases[0].input = simReadInput(workDir)

	simRepointConfig(t, workDir, stubPlaceholder, stub.srv.URL)

	// The real Run composition over pipes — every phase seam in one process.
	srvInR, srvInW := io.Pipe()
	cliR, cliOutW := io.Pipe()
	stderr := &syncBuffer{}

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)

	go func() {
		serveDone <- Run(ctx, srvInR, cliOutW, stderr, &Options{
			Profile: profileZcode, MaxConcurrent: 2,
			ProfilesDir: repoProfilesDir(t), WorkDir: workDir,
		})
	}()

	t.Cleanup(func() {
		cancel()

		_ = srvInW.Close()
		_ = cliOutW.Close()
		_ = srvInR.Close()
		_ = cliR.Close()

		select {
		case <-serveDone:
		case <-time.After(simGuardTimeout):
			t.Errorf("serve did not exit")
		}
	})

	cli := newSimClient(t, cliR, srvInW)

	// Stage 1 — the handshake: capabilities advertised, _meta default applied.
	simStageInitialize(t, cli)

	// Stage 2 — the menu with live effective values.
	sessionID := simStageSessionNew(t, cli, workDir)

	// Stage 3 — the activity turn, in emission order.
	simStagePromptStreaming(t, cli, stub, sessionID)

	// Stage 4 — the editor-driven model switch, persisted and applied live.
	simStageModelSwitch(t, cli, stub, workDir, sessionID)

	// Stage 5 — the mid-turn cancel contract.
	simStageCancel(t, cli, stub, sessionID)
}

// simTurnScripts builds the provider script, in request order: (1) the
// activity turn — the Read tool card and the TodoWrite call; (2) the post-tool
// round with the closing chunks; (3) the post-model-switch round; (4) the held
// request the cancel stage aborts. (Tool blocks flush at their SSE block
// boundaries, so all tool phases precede all text phases within one stream.)
func simTurnScripts(stubPlaceholder string) []simTurnScript {
	return []simTurnScript{
		{phases: []simPhase{
			{callID: "call_read_1", name: simToolRead, input: simReadInput(stubPlaceholder)},
			{callID: "call_todo_1", name: simToolTodo, input: simTodoInput()},
		}},
		{phases: []simPhase{
			{text: "turn complete."},
			{text: "the plan panel is live."},
		}},
		{phases: []simPhase{{text: "model switched."}}},
		{hold: true},
	}
}

// simReadInput builds the scripted Read call's input for the workdir's notes.
func simReadInput(workDir string) string {
	raw, err := json.Marshal(map[string]string{"file_path": filepath.Join(workDir, "notes.md")})
	if err != nil {
		panic(err) // a one-key string map — cannot fail
	}

	return string(raw)
}

// simTodoInput is the captured TodoWrite input shape the plan frame maps.
func simTodoInput() string {
	return `{"todos":[` +
		`{"content":"probe the wire","status":"completed","priority":"high"},` +
		`{"content":"prove the plan panel","status":"in_progress","priority":"medium"},` +
		`{"content":"close the story","status":"pending","priority":"low"}]}`
}

// simRepointConfig swaps the stub placeholder for the real URL.
func simRepointConfig(t *testing.T, workDir, placeholder, url string) {
	t.Helper()

	configPath := filepath.Join(workDir, ".ass-guard", "config.yaml")

	fixed, rerr := os.ReadFile(configPath)
	if rerr != nil {
		t.Fatalf("read config: %v", rerr)
	}

	werr := os.WriteFile(configPath,
		[]byte(strings.Replace(string(fixed), placeholder, url, 1)), 0o600)
	if werr != nil {
		t.Fatalf("repoint config at stub: %v", werr)
	}
}

// simStageInitialize: the client advertises elicitation.form plus one _meta
// default; the agent answers v1 capabilities with NO probe (D-13) and the
// follow-up config_option_update carries the blob-applied full set (D-10).
func simStageInitialize(t *testing.T, cli *simClient) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"zed-1","method":"initialize","params":{"protocolVersion":1,` +
		`"clientInfo":{"name":"zed-simulator","version":"1"},` +
		`"clientCapabilities":{"elicitation":{"form":{}}},` +
		`"_meta":{"compaction-threshold":"65"}}}`)

	var (
		gotResponse bool
		gotUpdate   bool
		updateSet   []acp.ConfigOptionFrame
	)

	for !gotResponse || !gotUpdate { // the update and the response race — either order
		m := cli.next()

		switch {
		case isResponseID(m, `"zed-1"`):
			simAssertInitializeResult(t, m.Result)

			gotResponse = true
		case m.Method == simMethodSessionUpdate && decodeSimUpdate(t, m).Kind == simKindConfigOptions:
			updateSet = decodeSimUpdate(t, m).Options
			gotUpdate = true
		case m.Method == simMethodElicitation:
			t.Fatal("elicitation probe sent although the client advertised elicitation.form (D-13)")
		}
	}

	// The follow-up config_option_update carries the blob-applied FULL set.
	assertFullMenu(t, updateSet, "post-blob config_option_update")

	got := optionByID(t, updateSet, optCompactionThresh).CurrentValue
	if got != testCompactionBlob {
		t.Errorf("compaction-threshold currentValue = %q; want the blob-applied %q (D-10)", got, testCompactionBlob)
	}
}

// simAssertInitializeResult pins the v1 handshake shape (protocolVersion 1,
// loadSession true — the 18-01 replay spine, the advertised menu present).
func simAssertInitializeResult(t *testing.T, raw json.RawMessage) {
	t.Helper()

	var resp struct {
		ProtocolVersion   int `json:"protocolVersion"` //nolint:tagliatelle // ACP wire field
		AgentCapabilities struct {
			LoadSession bool `json:"loadSession"` //nolint:tagliatelle // ACP wire field
		} `json:"agentCapabilities"` //nolint:tagliatelle // ACP wire field
		ConfigOptions []acp.ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
	}

	err := json.Unmarshal(raw, &resp)
	if err != nil {
		t.Fatalf("decode initialize result: %v (%s)", err, string(raw))
	}

	if resp.ProtocolVersion != 1 {
		t.Errorf("protocolVersion = %d; want 1 (Zed speaks v1)", resp.ProtocolVersion)
	}

	if !resp.AgentCapabilities.LoadSession {
		t.Error("agentCapabilities.loadSession = false; want true (18-01/ACP-06 — replay is live)")
	}

	assertFullMenu(t, resp.ConfigOptions, "initialize response advertisement")
}

// simStageSessionNew: the ten-entry menu with effective currentValues —
// the floor-resolved model/tier and the _meta blob fill (D-11).
func simStageSessionNew(t *testing.T, cli *simClient, workDir string) string {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"zed-2","method":"session/new","params":{"cwd":` +
		simJSONStr(workDir) + `,"mcpServers":[]}}`)

	m := cli.nextResponse(`"zed-2"`)

	var resp struct {
		SessionID     string                  `json:"sessionId"`     //nolint:tagliatelle // ACP wire field
		ConfigOptions []acp.ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
	}

	err := json.Unmarshal(m.Result, &resp)
	if err != nil {
		t.Fatalf("decode session/new result: %v (%s)", err, string(m.Result))
	}

	if resp.SessionID == "" {
		t.Fatal("session/new returned no sessionId")
	}

	assertFullMenu(t, resp.ConfigOptions, "session/new advertisement")

	if got := optionByID(t, resp.ConfigOptions, optModel).CurrentValue; got != testModelPrimary {
		t.Errorf("model currentValue = %q; want the floor-resolved %q (D-11)", got, testModelPrimary)
	}

	if got := optionByID(t, resp.ConfigOptions, optTier).CurrentValue; got != testTierHeavy {
		t.Errorf("tier currentValue = %q; want %q", got, testTierHeavy)
	}

	// The _meta blob is the default-of-last-resort — its fill is still the
	// EFFECTIVE advertised value.
	got := optionByID(t, resp.ConfigOptions, optCompactionThresh).CurrentValue
	if got != testCompactionBlob {
		t.Errorf("compaction-threshold currentValue = %q; want the blob fill %q", got, testCompactionBlob)
	}

	return resp.SessionID
}

// simStagePromptStreaming: one scripted turn — tool card for Read, a plan
// frame (NOT a second card) for TodoWrite, then message chunks — all strictly
// before the prompt response (updates-before-response). The captured provider
// request carried the floor model.
func simStagePromptStreaming(t *testing.T, cli *simClient, stub *simStub, sessionID string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"zed-3","method":"session/prompt","params":{"sessionId":` +
		simJSONStr(sessionID) + `,"prompt":[{"type":"text","text":"watch the turn"}]}}`)

	wire, plans, todoCard := simCollectTurnStory(t, cli, `"zed-3"`)

	want := []string{"tool_call:call_read_1", "plan", "chunk", "chunk", "RESPONSE"}
	if strings.Join(wire, ",") != strings.Join(want, ",") {
		t.Fatalf("emission order broken:\n got: %v\nwant: %v", wire, want)
	}

	if plans != 1 {
		t.Errorf("plan frames = %d; want exactly 1 (the TodoWrite call renders ONLY the plan)", plans)
	}

	if todoCard {
		t.Error("the TodoWrite call rendered a second tool card (only the plan frame may represent it)")
	}

	models := stub.recordedModels()
	if len(models) < 1 || models[0] != testModelPrimary {
		t.Fatalf("first provider request model = %v; want [%s ...]", models, testModelPrimary)
	}
}

// simCollectTurnStory reads frames until the response carrying id arrives,
// returning the update story in arrival order (RESPONSE last — the ordering
// invariant is structural: the pipe delivers bytes in write order).
//
//nolint:nonamedreturns // the names document the triple for the caller
func simCollectTurnStory(t *testing.T, cli *simClient, id string) (wire []string, plans int, todoCard bool) {
	t.Helper()

	for {
		m := cli.next()

		if isResponseID(m, id) {
			simAssertStopReason(t, m.Result, id, simStopEndTurn)

			wire = append(wire, "RESPONSE")

			for i, w := range wire[:len(wire)-1] {
				if w == "RESPONSE" {
					t.Fatalf("an earlier response appeared at %d (updates-before-response broken): %v", i, wire)
				}
			}

			return wire, plans, todoCard
		}

		if m.Method != simMethodSessionUpdate {
			continue // usage/other notifications carry no session/update story
		}

		upd := decodeSimUpdate(t, m)

		switch upd.Kind {
		case simKindToolCall:
			wire = append(wire, "tool_call:"+upd.ToolCallID)

			if upd.ToolCallID == "call_todo_1" {
				todoCard = true
			}
		case simKindPlan:
			wire = append(wire, "plan")
			plans++
		case simKindChunk:
			wire = append(wire, "chunk")
		default:
			wire = append(wire, upd.Kind)
		}
	}
}

// simAssertStopReason decodes a prompt response and pins its stopReason.
func simAssertStopReason(t *testing.T, raw json.RawMessage, id, want string) {
	t.Helper()

	var resp struct {
		StopReason string `json:"stopReason"` //nolint:tagliatelle // ACP wire field
	}

	err := json.Unmarshal(raw, &resp)
	if err != nil {
		t.Fatalf("decode prompt response %s: %v (%s)", id, err, string(raw))
	}

	if resp.StopReason != want {
		t.Errorf("response %s stopReason = %q; want %q", id, resp.StopReason, want)
	}
}

// simStageModelSwitch: the editor sets the model — the response and the
// out-of-band update both carry the full refreshed set, the write lands in
// the project layer (re-loaded through the REAL loader), and the very next
// provider request carries the new model (persist-then-live-apply, D-07/D-12).
func simStageModelSwitch(t *testing.T, cli *simClient, stub *simStub, workDir, sessionID string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"zed-4","method":"session/set_config_option","params":{"sessionId":` +
		simJSONStr(sessionID) + `,"configId":"` + optModel + `","value":"` + testModelFallback + `"}}`)

	simAwaitSetConfirmation(t, cli)

	// Persistence (the D-07 write half): the REAL loader resolves the write.
	cfg, lerr := modelrouting.Load(filepath.Join(workDir, ".ass-guard", "config.yaml"))
	if lerr != nil {
		t.Fatalf("reload written project layer: %v", lerr)
	}

	if cfg.Tiers[testTierHeavy].Model != testModelFallback {
		t.Errorf("loaded tiers.%s.model = %q; want the persisted %q",
			testTierHeavy, cfg.Tiers[testTierHeavy].Model, testModelFallback)
	}

	// Live apply: the very next provider request carries the new model.
	cli.sendf(`{"jsonrpc":"2.0","id":"zed-5","method":"session/prompt","params":{"sessionId":` +
		simJSONStr(sessionID) + `,"prompt":[{"type":"text","text":"again"}]}}`)

	m := cli.nextResponse(`"zed-5"`)
	simAssertStopReason(t, m.Result, `"zed-5"`, simStopEndTurn)

	models := stub.recordedModels()
	if len(models) < 3 {
		t.Fatalf("provider requests recorded = %d; want at least 3", len(models))
	}

	if models[2] != testModelFallback {
		t.Errorf("post-switch request model = %q; want %q (the next request carries the editor's model)",
			models[2], testModelFallback)
	}
}

// simAwaitSetConfirmation waits for BOTH the set response and the out-of-band
// config_option_update (they race), each carrying the full refreshed set with
// the new effective value.
func simAwaitSetConfirmation(t *testing.T, cli *simClient) {
	t.Helper()

	var (
		gotResponse bool
		gotUpdate   bool
	)

	for !gotResponse || !gotUpdate {
		m := cli.next()

		switch {
		case isResponseID(m, `"zed-4"`):
			var resp struct {
				ConfigOptions []acp.ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
			}

			err := json.Unmarshal(m.Result, &resp)
			if err != nil {
				t.Fatalf("decode set response: %v (%s)", err, string(m.Result))
			}

			simAssertOptionSet(t, resp.ConfigOptions, "set response")

			gotResponse = true
		case m.Method == simMethodSessionUpdate && decodeSimUpdate(t, m).Kind == simKindConfigOptions:
			simAssertOptionSet(t, decodeSimUpdate(t, m).Options, "post-set config_option_update")

			gotUpdate = true
		}
	}
}

// simAssertOptionSet pins a full refreshed option set carrying the new model.
func simAssertOptionSet(t *testing.T, opts []acp.ConfigOptionFrame, where string) {
	t.Helper()

	assertFullMenu(t, opts, where)

	got := optionByID(t, opts, optModel).CurrentValue
	if got != testModelFallback {
		t.Errorf("%s model currentValue = %q; want %q", where, got, testModelFallback)
	}
}

// simStageCancel: session/cancel mid-turn — the pending turn resolves, the
// prompt response carries the cancelled stop reason, and any registry cascade
// frame (and every update) strictly precedes the response.
func simStageCancel(t *testing.T, cli *simClient, stub *simStub, sessionID string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"zed-6","method":"session/prompt","params":{"sessionId":` +
		simJSONStr(sessionID) + `,"prompt":[{"type":"text","text":"hold the turn"}]}}`)

	stub.waitHold(t) // the provider request is in flight — the turn is pending

	cli.sendf(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":` +
		simJSONStr(sessionID) + `}}`)

	var cascade []int

	for {
		m := cli.next()

		switch {
		case m.Method == simMethodCancelRequest:
			// Any registry cascade precedes the response (record its position).
			cascade = append(cascade, len(cascade))
		case m.Method == simMethodSessionUpdate:
			// Late turn activity before the response is legal.
		case isResponseID(m, `"zed-6"`):
			simAssertStopReason(t, m.Result, `"zed-6"`, simStopCancelled)

			simAssertQuietAfterResponse(t, cli)

			_ = cascade // non-empty only when a pending outbound ask existed; both orders hold above

			return
		}
	}
}

// simAssertQuietAfterResponse proves the ordering discipline on the cancel
// path: a bounded drain window finds NO session/update after the response.
func simAssertQuietAfterResponse(t *testing.T, cli *simClient) {
	t.Helper()

	deadline := time.Now().Add(250 * time.Millisecond)

	for time.Now().Before(deadline) {
		select {
		case m, ok := <-cli.frames:
			if !ok {
				return
			}

			if m.Method == simMethodSessionUpdate {
				t.Fatalf("session/update arrived after the cancelled response: %s", string(m.Params))
			}
		case <-time.After(time.Until(deadline)):
		}
	}
}
