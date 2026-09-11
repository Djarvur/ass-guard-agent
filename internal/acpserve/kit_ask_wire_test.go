package acpserve //nolint:testpackage // internal package test

// 25-08 subject-split move (kit/runtime -> internal/acpserve): these two ask
// batteries drive a REAL acp.Server over pipes and assert on WIRE FRAME
// ORDER (the rendered question surface reaching the client before the
// suspension-returned prompt response) — the serve path is the subject, so
// per the Phase-15 D-02 rule they live at their subject's home.
// Construction retargets: the unexported Runner literal + the toolkit/
// catalog/engine twins -> the PRODUCTION composition (wireComposeRunner
// mirrors acpserve.Run's statements). Assertion retarget (recorded in the
// 25-08 SUMMARY): the in-package sess.HasPendingAsk() internal pin becomes
// the wire-observable reply-resume proof — sending the operator's reply and
// observing the resumed turn stream proves the ask was pending AND
// resumable at response time (a resolved ask would not park/resume).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/kit/profile"
	"github.com/Djarvur/ass-guard-agent/kit/provider"
	"github.com/Djarvur/ass-guard-agent/kit/runtime"
)

// Test-local constants (goconst) shared by the moved ask wire batteries.
const (
	wireAskTool  = "AskUserQuestion"
	wireAskCache = "ask me which library"

	wireAskToolUseChunk = "tool_use"
)

// wiringAskInput is the plan's Test-1 question shape (one question, two
// labelled options).
const wiringAskInput = `{"questions":[{"question":"Which cache library should we use?",` +
	`"header":"Cache",` +
	`"options":[{"label":"ristretto","description":"fast in-memory cache"},` +
	`{"label":"bigcache","description":"simple disk-backed cache"}]}]}`

// askToolCallProvider is a provider whose first Stream emits the AskUserQuestion
// tool call (text preamble + tool_use chunk), mirroring the live leg-1 model
// behavior; subsequent calls stream plain text (the resumed turn).
type askToolCallProvider struct {
	calls int
}

func (p *askToolCallProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, wireErrNotUsed
}

func (p *askToolCallProvider) Stream(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.calls++

	ch := make(chan provider.StreamChunk, 6)

	go func() {
		defer close(ch)

		if p.calls == 1 {
			for _, c := range []string{"Let ", "me ", "ask."} {
				select {
				case ch <- provider.StreamChunk{Type: wireBlockText, Text: c}:
				case <-ctx.Done():
					return
				}
			}

			tc := provider.ToolCall{
				ID: "call_srv_ask_1", Name: wireAskTool,
				Input: json.RawMessage(wiringAskInput),
			}

			select {
			case ch <- provider.StreamChunk{Type: wireAskToolUseChunk, ToolCall: &tc, ToolCallID: tc.ID}:
			case <-ctx.Done():
				return
			}
		} else {
			select {
			case ch <- provider.StreamChunk{Type: wireBlockText, Text: "acknowledged; proceeding"}:
			case <-ctx.Done():
				return
			}
		}

		select {
		case ch <- provider.StreamChunk{Type: wireChunkDone, FinishReason: wireStopEndTurn}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (p *askToolCallProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// SupportsImages: the fake is text-only (21-05 D-11 seam stub).
func (p *askToolCallProvider) SupportsImages() bool { return false }

// wireAskHandshake boots the server over pipes around the production
// composition and performs the initialize/session-new handshake, returning
// the pipe ends and the session id.
func wireAskHandshake(
	t *testing.T, runner *runtime.Runner, mp provider.Provider,
) (cliW *io.PipeWriter, cliR io.ReadCloser, sessionID string) {
	t.Helper()

	srvInR, cliW := io.Pipe()

	cliR, srvOutW := io.Pipe()

	srv := acp.NewServer(srvInR, srvOutW, &bytes.Buffer{}, acp.WithTurnRunner(kitTurnAdapter{runner: runner}))

	ctx, cancel := context.WithCancel(context.Background())

	served := make(chan struct{})

	go func() {
		_ = srv.Serve(ctx)

		close(served)
	}()

	t.Cleanup(func() {
		cancel()

		_ = cliW.Close()
		_ = srvOutW.Close()
		_ = srvInR.Close()

		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	})

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("0"), Method: wireMethodInitialize,
		Params: wireRawJSON(map[string]any{wireKeyProtoVersion: 1}),
	})

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("1"), Method: wireMethodSessNew,
		Params: wireRawJSON(map[string]any{wireCwdKey: wireTestCwdTmp, wireKeyMcpServers: []any{}}),
	})

	frames := wireReadResultFrames(t, cliR, 2)

	var snew struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	for _, f := range frames {
		if strings.Contains(string(f.Result), "sessionId") {
			_ = json.Unmarshal(f.Result, &snew)
		}
	}

	if snew.SessionID == "" {
		t.Fatalf("no sessionId from session/new: %+v", frames)
	}

	return cliW, cliR, snew.SessionID
}

// wireAskPromptSends sends the cache-library prompt and collects frames until
// its response arrives.
func wireAskPromptSends(t *testing.T, cliW io.Writer, cliR io.Reader, sessionID, id string) []*acp.Message {
	t.Helper()

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage(id), Method: wireMethodSessPrmt,
		Params: wireRawJSON(map[string]any{
			wireKeySessionID: sessionID,
			wireKeyPrompt:    []any{map[string]any{wireKeyType: wireBlockText, wireBlockText: wireAskCache}},
		}),
	})

	var (
		gotResponse bool

		all []*acp.Message
	)

	br := bufio.NewReader(cliR)

	// The 30s deadline (not 5s) is intentional: this orchestrated server test
	// runs t.Parallel() with the rest of the suite under -race, and the full
	// round-trip has repeatedly blown a 5s budget on loaded machines (observed
	// 6.4–8.6s on baseline, pre-260819-nlg). The assertion is ordering-based,
	// not timing-based — the deadline only bounds a hang.
	deadline := time.After(30 * time.Second)

	for !gotResponse {
		select {
		case <-deadline:
			t.Fatalf("no prompt response within 30s (frames so far: %d)", len(all))
		default:
		}

		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			time.Sleep(10 * time.Millisecond)

			continue
		}

		var m acp.Message

		jerr := json.Unmarshal(bytes.TrimRight(line, "\n"), &m)
		if jerr == nil {
			all = append(all, &m)

			if string(m.ID) == id && m.Result != nil {
				gotResponse = true
			}
		}
	}

	return all
}

// TestAskWiring_ServerLevelSurface: the live path through the REAL engine
// wiring (SetupEngine — the RealExecutor executes AskUserQuestion; the
// engine-off wireDriveACP stub never suspends) — the rendered question
// surface must reach the client wire.
func TestAskWiring_ServerLevelSurface(t *testing.T) { //nolint:cyclop,funlen // comprehensive server scenario
	t.Parallel()

	mp := &askToolCallProvider{}

	runner, _ := wireComposeRunner(t, mp, 0)

	cliW, cliR, sid := wireAskHandshake(t, runner, mp)

	all := wireAskPromptSends(t, cliW, cliR, sid, "2")

	questionOnWire := false

	for _, m := range all {
		if m.Method == wireSessionUpdate && strings.Contains(string(m.Params), "Which cache library") {
			questionOnWire = true
		}
	}

	if !questionOnWire {
		t.Errorf("the rendered question surface never reached the client wire "+
			"(%d frames; the live-witness finding reproduced)", len(all))
	}
}

// --- 13-00 T4: the serve park pins ---

// TestAskPark_PromptResponsePrecedesResolution (T4 pin 1, wire byte-compat —
// server level): the session/prompt RESPONSE arrives AT the suspension, BEFORE
// any resolution — the ask surface reached the client first, the pending ask
// is still unresolved at response time (the D-01 timer is 1h away), and the
// chain continues parked. A synchronous-wait implementation (the overruled
// README option 2) would hold the response for the 1h timer and fail the
// deadline — the stuck-spinner + reply-deadlock shape the 12-01 witness
// verified against.
func TestAskPark_PromptResponsePrecedesResolution(t *testing.T) { //nolint:cyclop,gocyclo,funlen // server scenario
	t.Parallel()

	mp := &askToolCallProvider{}

	// The timer never fires in test time — the response must NOT wait for it.
	runner, _ := wireComposeRunner(t, mp, time.Hour)

	cliW, cliR, sid := wireAskHandshake(t, runner, mp)

	sentAt := time.Now()

	var (
		gotResponse bool

		surfaceBeforeResponse bool

		all []*acp.Message
	)

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("2"), Method: wireMethodSessPrmt,
		Params: wireRawJSON(map[string]any{
			wireKeySessionID: sid,
			wireKeyPrompt:    []any{map[string]any{wireKeyType: wireBlockText, wireBlockText: wireAskCache}},
		}),
	})

	br := bufio.NewReader(cliR)

	deadline := time.After(30 * time.Second)

	for !gotResponse {
		select {
		case <-deadline:
			t.Fatalf("no prompt response within 30s — a synchronous wait is holding it " +
				"(the response must return AT the suspension, ~instantly)")
		default:
		}

		line, rerr := br.ReadBytes('\n')
		if len(line) == 0 && rerr != nil {
			time.Sleep(10 * time.Millisecond)

			continue
		}

		var m acp.Message

		jerr := json.Unmarshal(line, &m)
		if jerr == nil {
			if m.Method == wireSessionUpdate && strings.Contains(string(m.Params), "Which cache library") {
				surfaceBeforeResponse = true
			}

			all = append(all, &m)

			if string(m.ID) == "2" && m.Result != nil {
				gotResponse = true
			}
		}
	}

	if !surfaceBeforeResponse {
		t.Errorf("the ask surface did not precede the response (%d frames)", len(all))
	}

	if elapsed := time.Since(sentAt); elapsed > 10*time.Second {
		t.Errorf("response took %v; want near-instant (suspension return, not resolution)", elapsed)
	}

	// The ask is STILL unresolved at response time — the wire-observable
	// proof (recorded retarget of the in-package HasPendingAsk pin): sending
	// the operator's reply RESUMES the parked turn (the "acknowledged;
	// proceeding" streamed close), which only a pending, resumable ask can
	// produce.
	replyFrames := promptAndWait(t, cliW, cliR, sid, "ristretto", "3")

	resumedOnWire := false

	for _, m := range replyFrames {
		if m.Method == wireSessionUpdate && strings.Contains(string(m.Params), "acknowledged; proceeding") {
			resumedOnWire = true
		}
	}

	if !resumedOnWire {
		t.Errorf("the reply never resumed the parked turn (%d reply frames) — "+
			"the ask was not pending at response time", len(replyFrames))
	}

	// Drain the parked chain (test hygiene; also the cancel path's proof shape).
	closeErr := runner.CloseSession(sid)
	if closeErr != nil {
		t.Fatalf("CloseSession: %v", closeErr)
	}

	idleCtx, idleCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer idleCancel()

	if !runner.WaitChainIdle(idleCtx, sid) {
		t.Error("the parked chain did not go idle after CloseSession (goroutine leak)")
	}
}
