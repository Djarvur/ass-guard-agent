package runtime //nolint:testpackage // internal package test

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
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// Plan-mode server-level wiring battery (12-09, gap G-12-3): the per-session
// plan-mode state MUST be wired onto the Session in sessionFor — today only
// RegisterInteractive sees it, so the Enter flip at the tool-result site is
// skipped (Session.planMode stays nil), the mutating-tool gate never fires,
// and ExitPlanMode answers "exit plan mode: not in plan mode" (the live
// go-err113 session finding, transcript d99c7845).
//
// The tests drive the REAL acp.Server over stdio pipes (the
// TestAskWiring_ServerLevelSurface harness) because the runner-level emitter
// cannot see the sessionFor wiring gap.

// Test-local constants (goconst discipline: shared literals named once).
const (
	pmEnterTool   = "EnterPlanMode"
	pmExitTool    = "ExitPlanMode"
	pmEditTool    = "Edit"
	pmCallEnter   = "call_pm_enter_1"
	pmCallEdit    = "call_pm_edit_1"
	pmCallExit    = "call_pm_exit_1"
	pmPlanText    = "## The plan\n\n1. do the thing"
	pmApproveText = "approve"
)

// pmPlanInput is the ExitPlanMode input carrying the plan text.
const pmPlanInput = `{"plan":"` + pmPlanText + `"}`

// pmEditInput is a MUTATING tool call (Edit) the gate must refuse while ON.
const pmEditInput = `{"file_path":"/tmp/pm/x.go","old_string":"a","new_string":"b"}`

// planModeScriptProvider scripts the model side of the scenario:
// call 1 → EnterPlanMode tool_use; later calls → Edit tool_use (must be
// REFUSED while plan mode is ON); final call → ExitPlanMode tool_use
// (suspends on the approval question); after approval → plain text close.
type planModeScriptProvider struct {
	calls int
}

func (p *planModeScriptProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, errNotUsed
}

func (p *planModeScriptProvider) Stream(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.calls++

	ch := make(chan provider.StreamChunk, 4)

	go func() {
		defer close(ch)

		emit := func(tc provider.ToolCall) {
			select {
			case ch <- provider.StreamChunk{Type: tracerToolUse, ToolCall: &tc, ToolCallID: tc.ID}:
			case <-ctx.Done():
			}
		}

		switch p.calls {
		case 1:
			emit(provider.ToolCall{ID: pmCallEnter, Name: pmEnterTool, Input: json.RawMessage(`{}`)})
		case 2:
			// Still ON (no approval yet): this Edit must be refused by the
			// gate and NOT executed; the model sees the refusal.
			emit(provider.ToolCall{ID: pmCallEdit, Name: pmEditTool, Input: json.RawMessage(pmEditInput)})
		case 3:
			emit(provider.ToolCall{ID: pmCallExit, Name: pmExitTool, Input: json.RawMessage(pmPlanInput)})
		default:
			select {
			case ch <- provider.StreamChunk{Type: blockText, Text: "plan approved; coding now"}:
			case <-ctx.Done():
			}
		}

		select {
		case ch <- provider.StreamChunk{Type: chunkDone, FinishReason: stopEndTurn}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (p *planModeScriptProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// SupportsImages: the fake is text-only (21-05 D-11 seam stub).
func (p *planModeScriptProvider) SupportsImages() bool { return false }

// newPlanModeWiringRunner builds an ENGINE-ON runner scripted with the
// plan-mode scenario and a long D-01 timeout (the approval reply must win).
func newPlanModeWiringRunner(t *testing.T) (*Runner, *planModeScriptProvider) {
	t.Helper()

	bus := event.NewBus()

	prov := &planModeScriptProvider{}

	dir := t.TempDir()

	writeOpsxCommandFixtures(t, dir)

	r := &Runner{
		bus:        bus,
		profile:    fakeProfileACP(),
		workDir:    dir,
		maxConc:    2,
		askTimeout: time.Hour,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider {
			return prov
		},
	}

	err := r.SetupEngine()
	if err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	r.LoadCommandRegistry()

	return r, prov
}

// startPlanModeServer boots the REAL acp.Server over pipes and performs the
// handshake, returning the reader/writer pair and the session id.
func startPlanModeServer(
	t *testing.T,
) (*Runner, *planModeScriptProvider, io.WriteCloser, io.ReadCloser, string) {
	t.Helper()

	r, prov := newPlanModeWiringRunner(t)

	srvInR, cliW := io.Pipe()
	cliR, srvOutW := io.Pipe()

	srv := acp.NewServer(srvInR, srvOutW, &bytes.Buffer{}, acp.WithTurnRunner(r))

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
		}
	})

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("0"), Method: methodInitialize,
		Params: rawJSON(map[string]any{keyProtoVersion: 1}),
	})

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("1"), Method: methodSessNew,
		Params: rawJSON(map[string]any{cwdKey: cwdForFrames, keyMCPServers: []any{}}),
	})

	frames := readFrames(t, cliR, 2)

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

	return r, prov, cliW, cliR, snew.SessionID
}

// promptAndWait sends one session/prompt and reads frames until its response
// arrives, returning everything collected.
func promptAndWait(
	t *testing.T, cliW io.Writer, cliR io.Reader, sessionID, text, id string,
) []*acp.Message {
	t.Helper()

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage(id), Method: methodSessPrmt,
		Params: rawJSON(map[string]any{
			keySessionID:  sessionID,
			promptListKey: []any{map[string]any{keyType: blockText, textListKey: text}},
		}),
	})

	var (
		gotResponse bool

		all []*acp.Message
	)

	br := bufio.NewReader(cliR)

	deadline := time.After(30 * time.Second)

	for !gotResponse {
		select {
		case <-deadline:
			t.Fatalf("prompt %s: no response within 30s (frames so far: %d)", id, len(all))
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

// replyToAsk sends the operator's reply as the NEXT session/prompt (the ask
// resume path routes it to the pending question).
func replyToAsk(t *testing.T, cliW io.Writer, cliR io.Reader, sessionID, reply, id string) []*acp.Message {
	t.Helper()

	return promptAndWait(t, cliW, cliR, sessionID, reply, id)
}

// TestPlanModeWiring_EnterFlipsStateAndGateRefuses (G-12-3, RED first):
// through the real acp.Server — EnterPlanMode's success result must flip the
// session state ON (the plan_mode_enter marker lands in the transcript), and
// the SUBSEQUENT Edit tool_use must be REFUSED by the runtime gate without
// executing. Today Session.planMode is never wired in sessionFor, so the flip
// is skipped and the Edit sails through (the live finding).
func TestPlanModeWiring_EnterFlipsStateAndGateRefuses(t *testing.T) {
	t.Parallel()

	r, _, cliW, cliR, sid := startPlanModeServer(t)

	// Turn 1: the model calls EnterPlanMode.
	promptAndWait(t, cliW, cliR, sid, "plan the refactor first", "10")

	// The transcript MUST carry the plan_mode_enter marker (written by the
	// tool-result site when the state flips — impossible while planMode is nil).
	markerOn := false

	lines, err := r.sessions[sid].Manager.ReadAll()
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}

	for _, l := range lines {
		if l.Type == "plan_mode" && l.Cause == "plan_mode_enter" {
			markerOn = true
		}
	}

	if !markerOn {
		t.Fatalf("EnterPlanMode succeeded but no plan_mode_enter marker landed "+
			"(Session.planMode unwired in sessionFor — G-12-3 reproduced at server level); %d lines", len(lines))
	}

	// Turn 2: the model attempts an Edit WHILE still ON (no approval happened).
	promptAndWait(t, cliW, cliR, sid, "go ahead and edit", "11")

	lines, err = r.sessions[sid].Manager.ReadAll()
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}

	editRefused := false

	for _, l := range lines {
		if l.Type == "tool_result" && l.ToolCallID == pmCallEdit && l.IsError &&
			strings.Contains(string(l.Output), "read-only") {
			editRefused = true
		}
	}

	if !editRefused {
		t.Fatalf("mutating Edit executed (or wrong form) while plan mode was ON — the gate never fired")
	}
}

// TestPlanModeWiring_ExitApprovalResumesSameTurn (G-12-3): ExitPlanMode must
// surface the approval question as an agent_message_chunk BEFORE the suspended
// response (the ask surface contract), and the approving reply resumes the
// SAME turn landing the approved-form tool_result. Today ExitPlanMode errors
// "not in plan mode" because the state never flipped ON.
func TestPlanModeWiring_ExitApprovalResumesSameTurn(t *testing.T) {
	t.Parallel()

	_, _, cliW, cliR, sid := startPlanModeServer(t)

	// Turn 1: enter → edit-refused → ExitPlanMode suspends with the approval
	// question. The prompt response arrives AT the suspension.
	frames := promptAndWait(t, cliW, cliR, sid, "plan the refactor first", "20")

	approvalOnWire := false

	for _, m := range frames {
		if m.Method == sessionUpdate && strings.Contains(string(m.Params), "Approve this implementation plan?") {
			approvalOnWire = true
		}
	}

	if !approvalOnWire {
		t.Fatalf("the ExitPlanMode approval question never reached the client wire (%d frames)", len(frames))
	}

	// The operator approves; the resumed turn closes.
	replyFrames := replyToAsk(t, cliW, cliR, sid, pmApproveText, "21")

	approvedOnWire := false

	for _, m := range replyFrames {
		if m.Method == sessionUpdate && strings.Contains(string(m.Params), "coding now") {
			approvedOnWire = true
		}
	}

	if !approvedOnWire {
		t.Fatalf("the post-approval turn never streamed to the client (%d frames)", len(replyFrames))
	}
}
