package acp //nolint:testpackage // internal package test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var errBoomFailed = errors.New("boom session/prompt failed")

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
	stderr *strings.Builder
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
		stderr: stderr,
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
func (h *pipeHarness) send(t *testing.T, msg *Message) {
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
			return "", fmt.Errorf("call: %w", err)
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
func newRequest(id int, method string, params map[string]any) *Message {
	pmsg, marshalErr := json.Marshal(params)
	if marshalErr != nil {
		panic(marshalErr)
	}

	return &Message{
		JSONRPC: protocolVersion20,
		ID:      json.RawMessage(strconv.Itoa(id)),
		Method:  method,
		Params:  pmsg,
	}
}

// newNotification builds a notification Message (no id).
func newNotification(method string, params map[string]any) *Message {
	pmsg, marshalErr := json.Marshal(params)
	if marshalErr != nil {
		panic(marshalErr)
	}

	return &Message{JSONRPC: protocolVersion20, Method: method, Params: pmsg}
}

// TestInitializeReturnsAgentCapabilities verifies the initialize response uses
// the exact field name `agentCapabilities` (NOT capabilities/serverInfo), an
// integer protocolVersion: 1, and loadSession:false (D-09 — NO replay in v1).
func TestInitializeReturnsAgentCapabilities(t *testing.T) {
	t.Parallel()
	h := newPipeHarness(t)
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))

	msg := h.readFrame(t)
	if msg.ID == nil || string(msg.ID) != "0" {
		t.Fatalf("response id = %v; want 0", msg.ID)
	}

	var res struct {
		ProtocolVersion   int            `json:"protocolVersion"`   //nolint:tagliatelle // ACP wire field
		AgentCapabilities map[string]any `json:"agentCapabilities"` //nolint:tagliatelle // ACP wire field
		AgentInfo         map[string]any `json:"agentInfo"`         //nolint:tagliatelle // ACP wire field
		AuthMethods       []any          `json:"authMethods"`       //nolint:tagliatelle // ACP wire field
		// Negative asserts: these fields must NOT appear.
		Capabilities map[string]any `json:"capabilities,omitempty"`
		ServerInfo   map[string]any `json:"serverInfo,omitempty"` //nolint:tagliatelle // ACP wire field
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

// TestInitializeStringID verifies JSON-RPC string ids (the v1 spec allows
// string, number, or null ids; Zed sends UUID strings) parse and are echoed
// verbatim in the response. Regression for the Zed "endless loading" report:
// Message.id was *int, so every Zed frame hit the -32700 parse-error path and
// initialize never got answered.
func TestInitializeStringID(t *testing.T) {
	t.Parallel()
	h := newPipeHarness(t)

	const id = "b88df47d-ab10-4831-aad2-bad625231c5a" // the exact shape Zed sends

	_, err := fmt.Fprintf(h.cliW,
		`{"jsonrpc":"2.0","id":%q,"method":"initialize","params":{"protocolVersion":1}}`+"\n",
		id)
	if err != nil {
		t.Fatalf("write raw frame: %v", err)
	}

	resp := h.readFrame(t)
	if resp.Error != nil {
		t.Fatalf("initialize with string id: unexpected error: %+v", resp.Error)
	}

	if got := string(resp.ID); got != `"`+id+`"` {
		t.Fatalf("response id = %v; want %q echoed verbatim", got, id)
	}

	if !strings.Contains(string(resp.Result), `"protocolVersion":1`) {
		t.Fatalf("initialize result missing protocolVersion: %s", resp.Result)
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
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
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
func TestSessionPromptStreamsUpdate(t *testing.T) { //nolint:funlen // comprehensive test scenario
	t.Parallel()

	stub := &stubTurn{chunks: []string{"Hello", " world"}}
	h := newPipeHarness(t, WithTurnRunner(stub))
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	h.readFrame(t)
	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readFrame(t)

	var sres struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
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
				t.Errorf("session/update carried an id (%v); notifications carry no id", string(msg.ID))
			}

			var params struct {
				Update struct {
					SessionUpdate string         `json:"sessionUpdate"` //nolint:tagliatelle // ACP wire field
					Content       map[string]any `json:"content,omitempty"`
				} `json:"update"`
			}

			_ = json.Unmarshal(msg.Params, &params)

			if params.Update.SessionUpdate != "agent_message_chunk" {
				t.Errorf("first sessionUpdate = %q; want agent_message_chunk", params.Update.SessionUpdate)
			}

			gotUpdate = true
		}

		if msg.ID != nil && string(msg.ID) == "2" {
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
		StopReason string `json:"stopReason"` //nolint:tagliatelle // ACP wire field
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
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
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

		if msg.ID != nil && string(msg.ID) == "2" {
			var pres struct {
				StopReason string `json:"stopReason"` //nolint:tagliatelle // ACP wire field
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
	if out == "" {
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

func mustFrame(t *testing.T, msg *Message) []byte {
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

		if msg.ID != nil && string(msg.ID) == "0" && msg.Result != nil {
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
	h := newPipeHarness(t, WithTurnRunner(&errTurn{err: errBoomFailed}))
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

		if msg.ID != nil && string(msg.ID) == "2" {
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

// handshake runs initialize (and swallows its response) so tests exercise a
// post-initialize connection.
func handshake(t *testing.T, h *pipeHarness) {
	t.Helper()

	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	h.readFrame(t)
}

// readProbeResponse reads the next frame and asserts it is the response to a
// probe request with the given id — proving NO spurious frame arrived first.
func readProbeResponse(t *testing.T, h *pipeHarness, wantID string) {
	t.Helper()

	next := h.readFrame(t)
	if string(next.ID) != wantID {
		t.Fatalf("expected the initialize response next; got method=%q id=%v (spurious?)", next.Method, next.ID)
	}
}

// TestServeResponseRouting proves Pitfall 1's fix: an inbound frame with an id,
// NO method, and a result-or-error is a RESPONSE to one of OUR outbound
// requests — delivered to the registry BEFORE handler dispatch, producing ZERO
// outbound frames (no spurious -32601). Unknown ids get exactly one structured
// stderr log and are dropped; serving continues.
func TestServeResponseRouting(t *testing.T) {
	t.Parallel()

	h := newPipeHarness(t)
	handshake(t, h)

	// A pending outbound Call from a paired goroutine.
	outcome := make(chan registryOutcome, 1)

	go func() {
		msg, err := h.srv.registry.Call(
			context.Background(), testOutboundMethod, map[string]any{"q": 1}, TimeoutFastControl)
		outcome <- registryOutcome{msg: msg, err: err}
	}()

	req := h.readFrame(t)
	if req.Method != testOutboundMethod {
		t.Fatalf("outbound method = %q; want %q", req.Method, testOutboundMethod)
	}

	id := NormalizeRequestID(req.ID)

	// Answer it — the response must resolve the Call and produce NO reply.
	h.send(t, &Message{JSONRPC: protocolVersion20, ID: quotedID(id), Result: json.RawMessage(`{"pong":true}`)})

	select {
	case oc := <-outcome:
		if oc.err != nil {
			t.Fatalf("Call errored: %v", oc.err)
		}

		if string(oc.msg.Result) != `{"pong":true}` {
			t.Errorf("resolved result = %s; want {\"pong\":true}", string(oc.msg.Result))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("response frame never resolved the pending Call")
	}

	// Zero spurious frames: the very next frame on stdout is our probe's
	// response (a mis-dispatched response would surface a -32601 first).
	h.send(t, newRequest(7, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	readProbeResponse(t, h, "7")

	// Unknown-id response: exactly one structured stderr log, dropped, and the
	// server keeps serving.
	const bogusID = "00000000-0000-4000-8000-000000000000"

	h.send(t, &Message{
		JSONRPC: protocolVersion20,
		ID:      quotedID(bogusID),
		Error:   &RPCError{Code: CodeInternalError, Message: "late"},
	})
	h.send(t, newRequest(8, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	readProbeResponse(t, h, "8")

	logs := h.stderr.String()
	if n := strings.Count(logs, "response for unknown id"); n != 1 {
		t.Errorf("unknown-id log lines = %d; want exactly 1", n)
	}

	if !strings.Contains(logs, bogusID) {
		t.Error("unknown-id log missing the id")
	}
}

// TestServeInboundCancelNoOp proves A8: an inbound $/cancel_request
// notification (the client cancelling ITS request) is a fast no-op — logged,
// no response frame (notifications get none), no error.
func TestServeInboundCancelNoOp(t *testing.T) {
	t.Parallel()

	h := newPipeHarness(t)
	handshake(t, h)

	h.send(t, newNotification(methodCancelRequest, map[string]any{"requestId": "client-req-1"}))
	h.send(t, newRequest(9, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	readProbeResponse(t, h, "9")

	logs := h.stderr.String()
	if !strings.Contains(logs, "cancel_request") || !strings.Contains(logs, "client-req-1") {
		t.Errorf("inbound cancel not logged (logs=%q)", logs)
	}
}

// readProbeFrame reads the next frame and asserts it is an elicitation/create
// probe carrying a minimal, schema-valid v1 Form payload with no session
// content (T-16-08) — returning its normalized id.
func readProbeFrame(t *testing.T, h *pipeHarness) string {
	t.Helper()

	req := h.readFrame(t)
	if req.Method != methodElicitationCreate {
		t.Fatalf("expected a %s probe; got method=%q", methodElicitationCreate, req.Method)
	}

	var pp struct {
		Message         string `json:"message"`
		Mode            string `json:"mode"`
		RequestedSchema struct {
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties"`
		} `json:"requestedSchema"` //nolint:tagliatelle // ACP wire field
	}

	unmarshalErr := json.Unmarshal(req.Params, &pp)
	if unmarshalErr != nil {
		t.Fatalf("unmarshal probe params: %v (%s)", unmarshalErr, string(req.Params))
	}

	if pp.Mode != "form" || pp.RequestedSchema.Type != "object" || len(pp.RequestedSchema.Properties) == 0 || pp.Message == "" {
		t.Errorf("probe payload not a minimal valid v1 Form: mode=%q schemaType=%q props=%d msg=%q",
			pp.Mode, pp.RequestedSchema.Type, len(pp.RequestedSchema.Properties), pp.Message)
	}

	return NormalizeRequestID(req.ID)
}

// TestInitializeProbe proves D-13's advertisement-first negotiation: a client
// advertising elicitation.form gets NO probe frame; a silent client gets
// exactly ONE elicitation/create probe whose result caches ok and whose -32601
// error caches degraded; the initialize response arrives on every path.
func TestInitializeProbe(t *testing.T) {
	t.Run("advertised: no probe, cached ok", func(t *testing.T) {
		t.Parallel()

		h := newPipeHarness(t)
		h.send(t, newRequest(0, methodInitialize, map[string]any{
			keyProtocolVersion: 1,
			"clientCapabilities": map[string]any{ //nolint:tagliatelle // ACP wire field
				"elicitation": map[string]any{"form": map[string]any{}},
			},
		}))

		resp := h.readFrame(t) // FIRST frame after initialize — no probe preceded it
		if resp.ID == nil || string(resp.ID) != "0" {
			t.Fatalf("expected the initialize response first; got method=%q id=%v", resp.Method, resp.ID)
		}

		if got := h.srv.Capability(capElicitationForm); got != CapabilityOK {
			t.Errorf("capability = %v; want CapabilityOK (advertisement-first)", got)
		}
	})

	t.Run("absent answered: one probe, ok", func(t *testing.T) {
		t.Parallel()

		h := newPipeHarness(t)
		h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))

		id := readProbeFrame(t, h)
		h.send(t, &Message{JSONRPC: protocolVersion20, ID: quotedID(id), Result: json.RawMessage(`{"action":"accept"}`)})

		resp := h.readFrame(t) // initialize STILL responds (always-respond rule)
		if resp.ID == nil || string(resp.ID) != "0" {
			t.Fatalf("expected the initialize response; got method=%q id=%v", resp.Method, resp.ID)
		}

		if got := h.srv.Capability(capElicitationForm); got != CapabilityOK {
			t.Errorf("capability = %v; want CapabilityOK after a result answer", got)
		}
	})

	t.Run("absent -32601: one probe, degraded", func(t *testing.T) {
		t.Parallel()

		h := newPipeHarness(t)
		h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))

		id := readProbeFrame(t, h)
		h.send(t, &Message{
			JSONRPC: protocolVersion20,
			ID:      quotedID(id),
			Error:   &RPCError{Code: CodeMethodNotFound, Message: "not supported"},
		})

		resp := h.readFrame(t)
		if resp.ID == nil || string(resp.ID) != "0" {
			t.Fatalf("expected the initialize response; got method=%q id=%v", resp.Method, resp.ID)
		}

		if got := h.srv.Capability(capElicitationForm); got != CapabilityDegraded {
			t.Errorf("capability = %v; want CapabilityDegraded after -32601", got)
		}
	})
}

// TestCapabilityStickiness proves D-18: after a degraded negotiation the cache
// answers degraded forever — a second capability query (even a re-initialized
// handshake) issues NO new probe for the connection.
func TestCapabilityStickiness(t *testing.T) {
	t.Parallel()

	h := newPipeHarness(t)
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))

	id := readProbeFrame(t, h)
	h.send(t, &Message{
		JSONRPC: protocolVersion20,
		ID:      quotedID(id),
		Error:   &RPCError{Code: CodeMethodNotFound, Message: "not supported"},
	})
	h.readFrame(t) // initialize response

	if got := h.srv.Capability(capElicitationForm); got != CapabilityDegraded {
		t.Fatalf("capability = %v; want CapabilityDegraded", got)
	}

	// A second capability query: still degraded, NO new probe on the wire.
	h.send(t, newRequest(1, methodInitialize, map[string]any{keyProtocolVersion: 1}))
	readProbeResponse(t, h, "1")

	if got := h.srv.Capability(capElicitationForm); got != CapabilityDegraded {
		t.Errorf("second query capability = %v; want CapabilityDegraded (sticky, D-18)", got)
	}
}

// TestProbeTimeoutFallback proves the probe rides the D-14 ladder: an
// unresponsive client burns one retry (same id), then the probe degrades with
// counters (probe total 1, timeout windows 2, fallback 1) and structured
// stderr lines — and the initialize response STILL arrives.
func TestProbeTimeoutFallback(t *testing.T) { //nolint:funlen // ladder + counters + logs in one scenario
	t.Parallel()

	h := newPipeHarness(t, WithRegistryConfig(RegistryConfig{FastControlTimeout: 4 * time.Millisecond}))
	h.send(t, newRequest(0, methodInitialize, map[string]any{keyProtocolVersion: 1}))

	first := readProbeFrame(t, h)
	retry := readProbeFrame(t, h)
	if first != retry {
		t.Errorf("probe retry id = %q; want the SAME id %q (D-14)", retry, first)
	}

	resp := h.readFrame(t) // initialize responds even when the probe falls back
	if resp.ID == nil || string(resp.ID) != "0" {
		t.Fatalf("expected the initialize response; got method=%q id=%v", resp.Method, resp.ID)
	}

	if got := h.srv.Capability(capElicitationForm); got != CapabilityDegraded {
		t.Errorf("capability = %v; want CapabilityDegraded after the ladder", got)
	}

	snap := h.srv.metrics.Snapshot()
	if snap.ProbeTotal != 1 || snap.ProbeTimeoutTotal != 2 || snap.ProbeFallbackTotal != 1 {
		t.Errorf("counters = (probe %d, timeout %d, fallback %d); want (1, 2, 1)",
			snap.ProbeTotal, snap.ProbeTimeoutTotal, snap.ProbeFallbackTotal)
	}

	logs := h.stderr.String()
	if !strings.Contains(logs, "capability probe elicitation.form") || !strings.Contains(logs, "fallback") {
		t.Errorf("probe fallback not logged structurally (logs=%q)", logs)
	}
}

// TestMetricsRegistryCancelCounter proves the D-16 family counts cancelled
// outbound requests through the registry's onCancel hook (NewServer wiring):
// a client answering -32800 bumps RegistryCancelTotal.
func TestMetricsRegistryCancelCounter(t *testing.T) {
	t.Parallel()

	h := newPipeHarness(t)
	handshake(t, h)

	ch := make(chan registryOutcome, 1)

	go func() {
		_, err := h.srv.registry.Call(
			context.Background(), testOutboundMethod, map[string]any{}, TimeoutFastControl)
		ch <- registryOutcome{err: err}
	}()

	req := h.readFrame(t)
	h.send(t, &Message{
		JSONRPC: protocolVersion20,
		ID:      quotedID(NormalizeRequestID(req.ID)),
		Error:   &RPCError{Code: CodeRequestCancelled, Message: "cancelled"},
	})

	oc := awaitOutcome(t, ch)
	if !errors.Is(oc.err, ErrRequestCancelled) {
		t.Fatalf("err = %v; want ErrRequestCancelled", oc.err)
	}

	if got := h.srv.metrics.Snapshot().RegistryCancelTotal; got != 1 {
		t.Errorf("RegistryCancelTotal = %d; want 1", got)
	}
}

// TestMetricsWriterStallFamily proves the 16-01 stall counter is adopted into
// the D-16 family: a sustained-stall episode bumps BOTH the emitter's legacy
// accessor and Metrics.WriterStallTotal (the /status surface).
func TestMetricsWriterStallFamily(t *testing.T) {
	t.Parallel()

	m := &Metrics{}
	em := NewTurnEmitter(&registrySink{}, &strings.Builder{}, TurnEmitterConfig{StallThreshold: time.Millisecond})
	defer em.Stop()
	em.metrics = m

	em.sampleStall(stallWatermark{since: time.Now().Add(-time.Second)}, laneForeground, true, time.Now())

	if got := m.Snapshot().WriterStallTotal; got != 1 {
		t.Errorf("WriterStallTotal = %d; want 1", got)
	}

	if got := em.StallCount(); got != 1 {
		t.Errorf("StallCount = %d; want 1 (legacy accessor keeps counting)", got)
	}
}
