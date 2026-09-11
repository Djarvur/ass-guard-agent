package acpserve //nolint:testpackage // internal package test

// 25-08 subject-split move (kit/runtime -> internal/acpserve): the advisory
// question-ending battery drives a REAL acp.Server over pipes and asserts on
// WIRE FRAMES (exactly ONE agent_message_chunk advisory note AFTER the turn's
// response text) — the serve path is the subject, so per the Phase-15 D-02
// rule the test lives at its subject's home. Construction retargets: the
// unexported Runner literal + the catalog/engine twins -> the PRODUCTION
// composition (wireComposeRunner mirrors acpserve.Run's statements); the
// transcript read rides the on-disk truth. Every assertion is byte-identical.

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/kit/profile"
	"github.com/Djarvur/ass-guard-agent/kit/provider"
	"github.com/Djarvur/ass-guard-agent/kit/session"
)

// questionClosingProvider streams one turn whose closing is question-shaped
// (the captured 08-06 stage-4 class) — the advisory's trigger fixture.
type questionClosingProvider struct{}

func (p *questionClosingProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, wireErrNotUsed
}

func (p *questionClosingProvider) Stream(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 3)

	go func() {
		defer close(ch)

		select {
		case ch <- provider.StreamChunk{Type: wireBlockText, Text: "Analysis done. Which would you like to do next? (1/2)"}:
		case <-ctx.Done():
			return
		}

		select {
		case ch <- provider.StreamChunk{Type: wireChunkDone, FinishReason: wireStopEndTurn}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (p *questionClosingProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// SupportsImages: the fake is text-only (21-05 D-11 seam stub).
func (p *questionClosingProvider) SupportsImages() bool { return false }

// TestAdvisoryWiring_QuestionEndingNote (13-03 T1 Test 4, wiring level):
// through the REAL acp.Server, a turn ending unmatched+question-shaped yields
// exactly ONE agent_message_chunk session/update carrying the advisory note
// AFTER the turn's response; the transcript carries the engine_decision line
// with the advisory signal and NO new user-message line for the note.
func TestAdvisoryWiring_QuestionEndingNote(t *testing.T) { //nolint:cyclop,gocyclo,gocognit,funlen,lll,maintidx // server battery
	t.Parallel()

	runner, workDir := wireComposeRunner(t, &questionClosingProvider{}, 0)

	srvInR, cliW := io.Pipe()

	cliR, srvOutW := io.Pipe()

	srv := acp.NewServer(srvInR, srvOutW, &strings.Builder{}, acp.WithTurnRunner(kitTurnAdapter{runner: runner}))

	ctx, cancel := context.WithCancel(context.Background())

	served := make(chan struct{})

	go func() {
		_ = srv.Serve(ctx)

		close(served)
	}()

	defer func() {
		cancel()

		_ = cliW.Close()
		_ = srvOutW.Close()
		_ = srvInR.Close()

		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}()

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("0"), Method: wireMethodInitialize,
		Params: wireRawJSON(map[string]any{wireKeyProtoVersion: 1}),
	})

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("1"), Method: wireMethodSessNew,
		Params: wireRawJSON(map[string]any{wireCwdKey: "/tmp", wireKeyMcpServers: []any{}}),
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

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("2"), Method: wireMethodSessPrmt,
		Params: wireRawJSON(map[string]any{
			wireKeySessionID: snew.SessionID,
			wireKeyPrompt:    []any{map[string]any{wireKeyType: wireBlockText, wireBlockText: "analyze the codebase"}},
		}),
	})

	// Read until the advisory note arrives (see the ORDERING NOTE in the
	// loop's closing comment).
	br := bufio.NewReader(cliR)

	var (
		gotResponse bool

		noteCount int

		userMsgCount int

		all []*acp.Message
	)

	deadline := time.After(30 * time.Second)

	for noteCount == 0 {
		select {
		case <-deadline:
			t.Fatalf("no advisory note within 30s (response=%v notes=%d frames=%d)",
				gotResponse, noteCount, len(all))
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
			all = append(all, &m)

			if string(m.ID) == "2" && m.Result != nil {
				gotResponse = true
			}

			if m.Method == wireSessionUpdate && strings.Contains(string(m.Params), "AskUserQuestion") {
				noteCount++
			}
		}
	}

	// ORDERING NOTE (2026-08-20): the note emits from inside Run (the in-hand
	// emitter, after the forwarder drained) — the SAME wire position the 12-01
	// ask surface uses — so it reaches the client just BEFORE the prompt's
	// JSON-RPC response (which handleSessionPrompt writes after Run returns).
	// "After the turn ends" (D-05) is about the TURN, not the response frame.
	if !gotResponse {
		respDeadline := time.After(10 * time.Second)

		for !gotResponse {
			select {
			case <-respDeadline:
				t.Fatalf("no prompt response within 10s of the note (frames=%d)",
					len(all))
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
				all = append(all, &m)

				if string(m.ID) == "2" && m.Result != nil {
					gotResponse = true
				}
			}
		}
	}

	if noteCount < 1 {
		t.Fatalf("advisory notes = %d; want >= 1", noteCount)
	}

	// The transcript: the advisory engine_decision line exists; NO new
	// user-message line rides the note (replay fidelity).
	lines := wireTranscriptLines(t, workDir, snew.SessionID)

	sawAdvisory := false

	for i := range lines {
		if lines[i].Type == session.TypeEngineDecision &&
			strings.Contains(string(lines[i].Input), "advisory:") {
			sawAdvisory = true
		}

		if lines[i].Type == session.TypeUserMessage {
			userMsgCount++
		}
	}

	if !sawAdvisory {
		t.Error("no engine_decision line with the advisory signal in the transcript")
	}

	if userMsgCount != 1 {
		t.Errorf("user_message lines = %d; want exactly 1 (the typed prompt — "+
			"the note is never a user message)", userMsgCount)
	}
}
