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
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// questionClosingProvider streams one turn whose closing is question-shaped
// (the captured 08-06 stage-4 class) — the advisory's trigger fixture.
type questionClosingProvider struct{}

func (p *questionClosingProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, errNotUsed
}

func (p *questionClosingProvider) Stream(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 3)

	go func() {
		defer close(ch)

		select {
		case ch <- provider.StreamChunk{Type: blockText, Text: "Analysis done. Which would you like to do next? (1/2)"}:
		case <-ctx.Done():
			return
		}

		select {
		case ch <- provider.StreamChunk{Type: chunkDone, FinishReason: stopEndTurn}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (p *questionClosingProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// TestAdvisoryWiring_QuestionEndingNote (13-03 T1 Test 4, wiring level):
// through the REAL acp.Server, a turn ending unmatched+question-shaped yields
// exactly ONE agent_message_chunk session/update carrying the advisory note
// AFTER the turn's response; the transcript carries the engine_decision line
// with the advisory signal and NO new user-message line for the note.
func TestAdvisoryWiring_QuestionEndingNote(t *testing.T) { //nolint:cyclop,gocyclo,gocognit,funlen,lll,maintidx // server battery
	t.Parallel()

	bus := event.NewBus()

	mp := &questionClosingProvider{}

	dir := t.TempDir()

	writeOpsxCommandFixtures(t, dir)

	runner := &Runner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      2,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return mp },
	}

	err := runner.SetupEngine()
	if err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	runner.LoadCommandRegistry()

	srvInR, cliW := io.Pipe()

	cliR, srvOutW := io.Pipe()

	srv := acp.NewServer(srvInR, srvOutW, &bytes.Buffer{}, acp.WithTurnRunner(runner))

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

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("0"), Method: methodInitialize,
		Params: rawJSON(map[string]any{keyProtoVersion: 1}),
	})

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("1"), Method: "session/new",
		Params: rawJSON(map[string]any{"cwd": "/tmp", keyMCPServers: []any{}}),
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

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("2"), Method: "session/prompt",
		Params: rawJSON(map[string]any{
			keySessionID:  snew.SessionID,
			promptListKey: []any{map[string]any{keyType: blockText, textListKey: "analyze the codebase"}},
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

		jerr := json.Unmarshal(bytes.TrimRight(line, "\n"), &m)
		if jerr == nil {
			all = append(all, &m)

			if string(m.ID) == "2" && m.Result != nil {
				gotResponse = true
			}

			if m.Method == sessionUpdate && strings.Contains(string(m.Params), "AskUserQuestion") {
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

			jerr := json.Unmarshal(bytes.TrimRight(line, "\n"), &m)
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
	runner.sessMu.Lock()
	sess := runner.sessions[snew.SessionID]
	runner.sessMu.Unlock()

	if sess == nil {
		t.Fatal("no session")
	}

	lines, rerr := sess.Manager.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

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

// countingEmitter counts AgentMessageChunk emissions.
type countingEmitter struct {
	n      int
	chunks []string
}

func (c *countingEmitter) AgentMessageChunk(_, text string) error {
	c.n++

	c.chunks = append(c.chunks, text)

	return nil
}

// TestAdvisoryWiring_DedupeSemantics (13-03 T2 Test 8, D-05): two consecutive
// same-class advisory turns in one session → ONE client note (the first) +
// TWO audit lines; a DIFFERENT class in the same session → its own first
// note; a NEW session → the note fires again for the same class.
func TestAdvisoryWiring_DedupeSemantics(t *testing.T) { //nolint:gocognit,cyclop,funlen // D-05 battery
	t.Parallel()

	newRunner := func() (*Runner, *countingEmitter) {
		r, _ := newExpansionRunner(t, true,
			scriptedResp{text: "Done. Which would you like? (1/2)", finish: stopEndTurn},
			scriptedResp{text: "Done again. Which would you like? (1/2)", finish: stopEndTurn},
			scriptedResp{text: "Pick one — just say the word.", finish: stopEndTurn},
		)

		em := &countingEmitter{}

		return r, em
	}

	t.Run("same class twice one session", func(t *testing.T) {
		t.Parallel()

		r, em := newRunner()

		const sid = "sess-adv-dedupe"

		_, err := r.Run(context.Background(), sid, em, []acp.ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Run 1: %v", err)
		}

		_, err = r.Run(context.Background(), sid, em, []acp.ContentBlock{{Type: blockText, Text: "again"}})
		if err != nil {
			t.Fatalf("Run 2: %v", err)
		}

		advisoryNotes := 0

		for _, c := range em.chunks {
			if strings.Contains(c, "AskUserQuestion") {
				advisoryNotes++
			}
		}

		if advisoryNotes != 1 {
			t.Errorf("advisory notes = %d; want exactly 1 (first-per-class-per-session visible)", advisoryNotes)
		}

		// BOTH audit lines exist (repeats are audit-trail-only).
		if got := countAdvisoryDecisions(t, r, sid); got != 2 {
			t.Errorf("advisory engine_decision lines = %d; want 2 (audit-always)", got)
		}
	})

	t.Run("different class same session", func(t *testing.T) {
		t.Parallel()

		r, _ := newExpansionRunner(t, true,
			scriptedResp{text: "Done. Which would you like? (1/2)", finish: stopEndTurn},
			scriptedResp{text: "Pick one — just say the word.", finish: stopEndTurn},
		)

		em := &countingEmitter{}

		const sid = "sess-adv-classes"

		_, err := r.Run(context.Background(), sid, em, []acp.ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Run 1: %v", err)
		}

		// Turn 2 closes with the OPEN-QUESTION class phrase.
		_, err = r.Run(context.Background(), sid, em, []acp.ContentBlock{{Type: blockText, Text: "next"}})
		if err != nil {
			t.Fatalf("Run 2: %v", err)
		}

		advisoryNotes := 0

		for _, c := range em.chunks {
			if strings.Contains(c, "AskUserQuestion") {
				advisoryNotes++
			}
		}

		if advisoryNotes != 2 {
			t.Errorf("advisory notes = %d; want 2 (one per class)", advisoryNotes)
		}
	})

	t.Run("new session notes again", func(t *testing.T) {
		t.Parallel()

		r, em := newRunner()

		for _, sid := range []string{"sess-adv-a", "sess-adv-b"} {
			_, err := r.Run(context.Background(), sid, em, []acp.ContentBlock{{Type: blockText, Text: "go"}})
			if err != nil {
				t.Fatalf("Run (%s): %v", sid, err)
			}
		}

		advisoryNotes := 0

		for _, c := range em.chunks {
			if strings.Contains(c, "AskUserQuestion") {
				advisoryNotes++
			}
		}

		if advisoryNotes != 2 {
			t.Errorf("advisory notes = %d; want 2 (per-session isolation)", advisoryNotes)
		}
	})
}

// countAdvisoryDecisions counts advisory-signal engine_decision lines.
func countAdvisoryDecisions(t *testing.T, r *Runner, sessionID string) int {
	t.Helper()

	lines, err := r.sessions[sessionID].Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	n := 0

	for i := range lines {
		if lines[i].Type == session.TypeEngineDecision &&
			strings.Contains(string(lines[i].Input), "advisory:") {
			n++
		}
	}

	return n
}

// TestAdvisoryWiring_NoteWordingElements (13-03 T2 Test 10): the note names
// the question ending, names AskUserQuestion as the route, and carries NO
// wording that asserts a pending hold.
func TestAdvisoryWiring_NoteWordingElements(t *testing.T) {
	t.Parallel()

	note := advisoryNoteText

	if !strings.Contains(note, "question") {
		t.Errorf("note does not name the question ending: %q", note)
	}

	if !strings.Contains(note, "AskUserQuestion") {
		t.Errorf("note does not name AskUserQuestion as the route: %q", note)
	}

	for _, hold := range []string{"holding", "held", "waiting for your", "pending", "paused"} {
		if strings.Contains(strings.ToLower(note), hold) {
			t.Errorf("note asserts a pending hold (%q): %q", hold, note)
		}
	}
}
