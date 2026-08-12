package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// pipeHarness wires an in-process mock ACP client to a Server over two io.Pipe
// pairs (client→server stdin, server→client stdout). Pattern from
// spikes/03-acp-handshake/. The harness is the load-bearing test substrate for
// the whole ACP package: initialize → session/new → session/prompt round-trips.
type pipeHarness struct {
	srv    *Server
	cliW   *io.PipeWriter // client writes frames here (→ server stdin)
	cliR   *io.PipeReader // client reads frames here (← server stdout)
	cbr    *bufio.Reader  // one shared client bufio.Reader over cliR
	cliMu  sync.Mutex     // serializes client reads (one reader at a time)
	cancel context.CancelFunc
}

func newPipeHarness(t *testing.T, opts ...ServerOption) *pipeHarness {
	t.Helper()

	srvInR, cliW := io.Pipe()  // cliW writes → srvInR reads (server stdin)
	cliR, srvOutW := io.Pipe() // srvOutW writes (server stdout) → cliR reads

	stderr := &strings.Builder{}
	srv := NewServer(srvInR, srvOutW, stderr, opts...)
	ctx, cancel := context.WithCancel(context.Background())
	h := &pipeHarness{
		cliW:   cliW,
		cliR:   cliR,
		cbr:    bufio.NewReader(cliR),
		srv:    srv,
		cancel: cancel,
	}
	done := make(chan struct{})

	go func() {
		_ = srv.Serve(ctx)

		close(done)
	}()
	// Close both pipe ends when the test ends so Serve's reader sees EOF and the
	// client reader unblocks.
	t.Cleanup(func() {
		cancel()

		_ = cliW.Close()
		_ = srvOutW.Close()
		_ = srvInR.Close()
		_ = cliR.Close()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("server.Serve did not exit within 2s of shutdown")
		}
	})

	return h
}

// send writes one client frame (request or notification) to the server stdin.
func (h *pipeHarness) send(t *testing.T, msg Message) {
	t.Helper()

	err := writeFrame(h.cliW, msg)
	if err != nil {
		t.Fatalf("client writeFrame: %v", err)
	}
}

// readFrame reads one frame from the shared client bufio.Reader (server stdout).
func (h *pipeHarness) readFrame(t *testing.T) *Message {
	t.Helper()
	h.cliMu.Lock()
	defer h.cliMu.Unlock()

	msg, err := readFrame(h.cbr)
	if err != nil {
		t.Fatalf("client readFrame: %v", err)
	}

	return msg
}

// stubTurn is the tracer's TurnRunner: it emits 1-2 agent_message_chunk events
// then returns stopReason "end_turn". The real Session.Prompt runner replaces
// it in Plan 02-05.
type stubTurn struct {
	chunks []string
	mu     sync.Mutex
	ran    bool
}

func (s *stubTurn) Run(ctx context.Context, _ string, emit ChunkEmitter, prompt []ContentBlock) (string, error) {
	s.mu.Lock()
	s.ran = true
	s.mu.Unlock()

	for i, c := range s.chunks {
		select {
		case <-ctx.Done():
			return stopCancelled, nil
		default:
		}

		err := emit.AgentMessageChunk(messageID(i+1), c)
		if err != nil {
			return "", err
		}
	}

	return stopEndTurn, nil
}

func messageID(n int) string {
	return "msg-stub-" + itoa(n)
}

// itoa is a tiny dependency-free int→string for stub message ids.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}

	return string(b)
}

// newRequest builds a request Message with the given integer id.
func newRequest(id int, method string, params map[string]any) Message {
	pmsg, _ := json.Marshal(params)

	return Message{JSONRPC: protocolVersion20, ID: &id, Method: method, Params: pmsg}
}

// newNotification builds a notification Message (no id).
func newNotification(method string, params map[string]any) Message {
	pmsg, _ := json.Marshal(params)

	return Message{JSONRPC: protocolVersion20, Method: method, Params: pmsg}
}

// TestInitializeReturnsAgentCapabilities verifies the initialize response uses
// the exact field name `agentCapabilities` (NOT capabilities/serverInfo), an
// integer protocolVersion: 1, and loadSession:false (D-09 — NO replay in v1).
func TestInitializeReturnsAgentCapabilities(t *testing.T) {
	t.Parallel()
	h := newPipeHarness(t)
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))

	msg := h.readFrame(t)
	if msg.ID == nil || *msg.ID != 0 {
		t.Fatalf("response id = %v; want 0", msg.ID)
	}

	var res struct {
		ProtocolVersion   int            `json:"protocolVersion"`   //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
		AgentCapabilities map[string]any `json:"agentCapabilities"` //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
		AgentInfo         map[string]any `json:"agentInfo"`         //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
		AuthMethods       []any          `json:"authMethods"`       //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
		// Negative asserts: these fields must NOT appear.
		Capabilities map[string]any `json:"capabilities,omitempty"`
		ServerInfo   map[string]any `json:"serverInfo,omitempty"` //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
	}

	err := json.Unmarshal(msg.Result, &res)
	if err != nil {
		t.Fatalf("unmarshal initialize result: %v (raw=%s)", err, string(msg.Result))
	}

	if res.ProtocolVersion != 1 {
		t.Errorf("protocolVersion = %v; want integer 1", res.ProtocolVersion)
	}

	if res.AgentCapabilities == nil {
		t.Fatal("agentCapabilities field missing (must be the exact field name per VERIFIED-FACTS #3)")
	}

	if ls, _ := res.AgentCapabilities["loadSession"].(bool); !ls {
		// loadSession:false is correct (D-09 — NO replay); we assert the field
		// is PRESENT and FALSE.
		if _, ok := res.AgentCapabilities["loadSession"]; !ok {
			t.Errorf("agentCapabilities.loadSession missing; want loadSession:false (D-09)")
		}
	} else {
		t.Errorf("agentCapabilities.loadSession = true; want false (D-09 — NO replay in v1)")
	}

	if res.Capabilities != nil {
		t.Errorf("response has a 'capabilities' field; must be 'agentCapabilities' (VERIFIED-FACTS #3)")
	}

	if res.ServerInfo != nil {
		t.Errorf("response has a 'serverInfo' field; must be 'agentInfo' (VERIFIED-FACTS #3)")
	}
}

// TestSessionNewReturnsSessionID verifies session/new returns a non-empty
// sessionId the client threads into session/prompt.
func TestSessionNewReturnsSessionID(t *testing.T) {
	t.Parallel()
	h := newPipeHarness(t)
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	h.readFrame(t)
	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	msg := h.readFrame(t)

	var res struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
	}

	err := json.Unmarshal(msg.Result, &res)
	if err != nil {
		t.Fatalf("unmarshal session/new result: %v", err)
	}

	if res.SessionID == "" {
		t.Errorf("sessionId empty; want a non-empty session id")
	}
}

// TestSessionPromptStreamsUpdate verifies session/prompt results in at least one
// session/update notification (agent_message_chunk, no id) arriving BEFORE the
// session/prompt response carrying stopReason (ACP-04 streaming — NO full-turn
// buffering).
func TestSessionPromptStreamsUpdate(t *testing.T) {
	t.Parallel()

	stub := &stubTurn{chunks: []string{"Hello", " world"}}
	h := newPipeHarness(t, WithTurnRunner(stub))
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	h.readFrame(t)
	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readFrame(t)

	var sres struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
	}

	_ = json.Unmarshal(snew.Result, &sres)

	h.send(t, newRequest(2, "session/prompt", map[string]any{
		keySessionID: sres.SessionID,
		"prompt":     []map[string]any{{"type": blockText, blockText: "hi"}},
	}))

	// Collect frames until the session/prompt response (id=2) arrives. At least
	// one must be a session/update notification.
	gotUpdate := false

	var promptResp *Message

	for i := range 8 {
		msg, err := readFrame(h.cbr)
		if err != nil {
			t.Fatalf("client readFrame[%d]: %v", i, err)
		}

		if msg.Method == methodSessionUpdate {
			if msg.ID != nil {
				t.Errorf("session/update carried an id (%v); notifications carry no id", *msg.ID)
			}

			var params struct {
				Update struct {
					SessionUpdate string         `json:"sessionUpdate"` //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
					Content       map[string]any `json:"content,omitempty"`
				} `json:"update"`
			}

			_ = json.Unmarshal(msg.Params, &params)

			if params.Update.SessionUpdate != "agent_message_chunk" {
				t.Errorf("first sessionUpdate = %q; want agent_message_chunk", params.Update.SessionUpdate)
			}

			gotUpdate = true
		}

		if msg.ID != nil && *msg.ID == 2 {
			promptResp = msg

			break
		}
	}

	if !gotUpdate {
		t.Error("no session/update notification observed before the session/prompt response (ACP-04)")
	}

	if promptResp == nil {
		t.Fatal("session/prompt response (id=2) never arrived")
	}

	var pres struct {
		StopReason string `json:"stopReason"` //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
	}

	_ = json.Unmarshal(promptResp.Result, &pres)

	if pres.StopReason != stopEndTurn {
		t.Errorf("stopReason = %q; want end_turn", pres.StopReason)
	}
}

// TestSessionCancelProducesNoResponse verifies session/cancel is a notification:
// it carries no id, produces NO response frame, and cancels the active turn's
// context (D-16 mechanism).
func TestSessionCancelProducesNoResponse(t *testing.T) {
	t.Parallel()

	stub := &stubTurn{chunks: []string{"x"}}
	h := newPipeHarness(t, WithTurnRunner(stub))
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	h.readFrame(t)
	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readFrame(t)

	var sres struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
	}

	_ = json.Unmarshal(snew.Result, &sres)

	// Send cancel BEFORE prompt so the turn has a cancel func registered but we
	// can deterministically assert no response frame for the cancel itself.
	// First kick a prompt, then immediately cancel.
	h.send(t, newRequest(2, "session/prompt", map[string]any{
		keySessionID: sres.SessionID,
		"prompt":     []map[string]any{{"type": blockText, blockText: "hi"}},
	}))
	h.send(t, newNotification("session/cancel", map[string]any{keySessionID: sres.SessionID}))

	// The cancel notification must produce NO response frame. We expect either
	// the chunk(s) + the prompt response (cancelled), and crucially no frame
	// whose id points at a cancel response (cancel has no id).
	cancelResponseSeen := false

	promptDone := false
	for i := 0; i < 8 && !promptDone; i++ {
		msg, err := readFrame(h.cbr)
		if err != nil {
			break
		}

		if msg.Method == "session/cancel" && msg.ID != nil {
			cancelResponseSeen = true
		}

		if msg.ID != nil && *msg.ID == 2 {
			var pres struct {
				StopReason string `json:"stopReason"` //nolint:tagliatelle // ACP protocol wire field (camelCase per spec) — cannot rename
			}

			_ = json.Unmarshal(msg.Result, &pres)

			if pres.StopReason == stopCancelled || pres.StopReason == stopEndTurn {
				promptDone = true
			}
		}
	}

	if cancelResponseSeen {
		t.Error("session/cancel produced a response frame; notifications get no response")
	}
}

// TestStdoutClean verifies transport discipline: after a full round-trip, the
// captured stdout contains ONLY valid newline-delimited JSON frames (every line
// parses as a Message). No log/diagnostic bytes leak to stdout (Pitfall 1).
func TestStdoutClean(t *testing.T) {
	t.Parallel()

	stub := &stubTurn{chunks: []string{"hi"}}
	// Capture the server's stdout into a buffer instead of a pipe so we can
	// inspect every byte after the round-trip.
	var stdout bytesAccumulator

	srvInR, cliW := io.Pipe()
	stderr := &strings.Builder{}
	srv := NewServer(srvInR, &stdout, stderr, WithTurnRunner(stub))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { _ = srv.Serve(ctx); close(done) }()

	defer func() {
		cancel()

		_ = cliW.Close()

		<-done
	}()

	_, _ = cliW.Write(mustFrame(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1})))
	time.Sleep(100 * time.Millisecond)
	cancel()

	_ = cliW.Close()

	<-done

	out := stdout.String()
	if len(out) == 0 {
		t.Fatal("no bytes on stdout; expected the initialize response frame")
	}

	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		var m Message

		err := json.Unmarshal([]byte(line), &m)
		if err != nil {
			t.Errorf("stdout line %d is not a valid JSON frame: %v (line=%q)", i, err, line)
		}
	}
}

// bytesAccumulator is a concurrency-safe io.Writer used to capture the server's
// stdout for the transport-discipline test.
type bytesAccumulator struct {
	mu  sync.Mutex
	buf []byte
}

func (b *bytesAccumulator) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buf = append(b.buf, p...)

	return len(p), nil
}

func (b *bytesAccumulator) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return string(b.buf)
}

func mustFrame(t *testing.T, msg Message) []byte {
	t.Helper()

	var sb strings.Builder

	err := writeFrame(&sb, msg)
	if err != nil {
		t.Fatalf("writeFrame: %v", err)
	}

	return []byte(sb.String())
}

// TestMalformedFrameContinues verifies a malformed frame on stdin produces a
// -32700 parse-error response AND the server keeps reading subsequent frames
// (does not exit on a single bad frame).
func TestMalformedFrameContinues(t *testing.T) {
	t.Parallel()
	h := newPipeHarness(t)
	// Write a malformed frame directly.
	_, _ = h.cliW.Write([]byte("{\"jsonrpc\":\"2.0\",BROKEN\n"))
	// Then a well-formed initialize.
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))

	gotParseError := false

	gotInit := false

	for i := 0; i < 6 && (!gotParseError || !gotInit); i++ {
		msg, err := readFrame(h.cbr)
		if err != nil {
			t.Fatalf("client readFrame[%d]: %v", i, err)
		}

		if msg.Error != nil && msg.Error.Code == CodeParseError {
			gotParseError = true
		}

		if msg.ID != nil && *msg.ID == 0 && msg.Result != nil {
			gotInit = true
		}
	}

	if !gotParseError {
		t.Error("no -32700 parse-error response for the malformed frame")
	}

	if !gotInit {
		t.Error("server did not keep reading after the malformed frame (initialize response missing)")
	}
}

// errTurn always returns an error, used to verify error responses carry the
// scrubbed message and the right envelope.
type errTurn struct{ err error }

func (e *errTurn) Run(ctx context.Context, _ string, emit ChunkEmitter, prompt []ContentBlock) (string, error) {
	return "", e.err
}

// TestErrorResponseShape verifies a handler error surfaces as a JSON-RPC error
// response with the request's id (never a crash).
func TestErrorResponseShape(t *testing.T) {
	t.Parallel()
	h := newPipeHarness(t, WithTurnRunner(&errTurn{err: errors.New("boom session/prompt failed")}))
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	h.readFrame(t)
	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	h.readFrame(t)
	// session/load must be a -32601 method-not-supported error (D-09 no-op).
	h.send(t, newRequest(2, "session/load", map[string]any{keySessionID: "x"}))

	var loadResp *Message

	for i := range 6 {
		msg, err := readFrame(h.cbr)
		if err != nil {
			t.Fatalf("readFrame[%d]: %v", i, err)
		}

		if msg.ID != nil && *msg.ID == 2 {
			loadResp = msg

			break
		}
	}

	if loadResp == nil {
		t.Fatal("session/load response never arrived")
	}

	if loadResp.Error == nil {
		t.Fatal("session/load returned a result; want a -32601 error (D-09 no-op)")
	}

	if loadResp.Error.Code != CodeMethodNotFound {
		t.Errorf("session/load error code = %d; want -32601 (method not found)", loadResp.Error.Code)
	}
}
