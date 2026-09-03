package acp //nolint:testpackage // internal package test

// 18-01 Task 2: the replay mapping battery — table-driven subtests over
// hand-written fixture transcripts in temp dirs, driven through the recording
// emitter (the tolerant-reader + full agent-visible vocabulary pin).

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// recordedFrame is one observed emission (the recording emitter's log entry).
type recordedFrame struct {
	kind       string // sessionUpdate kind value
	messageID  string
	text       string
	toolCallID string
	title      string
	status     string
}

// recordingEmitter records every ReplayTranscript emission — both the chunk
// surface and the ActivityEmitter surface — in arrival order.
type recordingEmitter struct {
	mu     sync.Mutex
	frames []recordedFrame
}

func (r *recordingEmitter) AgentMessageChunk(messageID, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames,
		recordedFrame{kind: updKindAgentMessageChunk, messageID: messageID, text: text})

	return nil
}

func (r *recordingEmitter) ToolCall(frame *ToolCallFrame) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames, recordedFrame{
		kind: updKindToolCall, toolCallID: frame.ToolCallID, title: frame.Title,
	})

	return nil
}

func (r *recordingEmitter) ToolCallUpdate(frame *ToolCallUpdateFrame) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames, recordedFrame{
		kind: updKindToolCallUpdate, toolCallID: frame.ToolCallID, status: frame.Status,
	})

	return nil
}

func (r *recordingEmitter) PlanUpdate(_ []PlanEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames, recordedFrame{kind: updKindPlan})

	return nil
}

func (r *recordingEmitter) ThoughtChunk(messageID string, content ContentBlock) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames,
		recordedFrame{kind: updKindThoughtChunk, messageID: messageID, text: content.Text})

	return nil
}

func (r *recordingEmitter) snapshot() []recordedFrame {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]recordedFrame(nil), r.frames...)
}

// Fixture vocabulary (goconst-extracted; shared with the session-family
// battery in session_family_test.go).
const (
	replayFixtureSID = "aaaaaaaa-0b0b-4c0c-8d0d-0e0e0e0e0e0e"
	fixtureTurnOne   = "s-turn-001"
	fixtureToolBash  = "Bash"
	fixtureToolRead  = "Read"
	fixtureCallRead  = "tc-1"
)

// Fixture line builders: one place per line shape (the table stays readable;
// the JSON literals never repeat).
func fixtureUserLine(turn, text string) string {
	return `{"type":"user_message","turnID":"` + turn + `","content":[{"type":"text","text":"` + text + `"}]}`
}

func fixtureChunkLine(turn, messageID, text string) string {
	return `{"type":"agent_message_chunk","turnID":"` + turn + `","messageID":"` + messageID +
		`","text":"` + text + `"}`
}

func fixtureAssistantLine(turn, text string) string {
	return `{"type":"assistant_message","turnID":"` + turn + `","text":"` + text + `"}`
}

func fixtureToolCallLine(turn, callID, name string) string {
	return `{"type":"tool_call","turnID":"` + turn + `","toolCallID":"` + callID + `","name":"` + name + `"}`
}

func fixtureToolResultLine(turn, callID string, isError bool) string {
	isErr := "false"
	if isError {
		isErr = "true"
	}

	return `{"type":"tool_result","turnID":"` + turn + `","toolCallID":"` + callID +
		`","output":{"out":1},"isError":` + isErr + `}`
}

func fixtureErrorLine(turn, callID, component, message string) string {
	idField := ""
	if callID != "" {
		idField = `,"toolCallID":"` + callID + `"`
	}

	return `{"type":"error","turnID":"` + turn + `"` + idField + `,"component":"` + component +
		`","message":"` + message + `"}`
}

func fixtureCanceledLine(turn, reason string) string {
	return `{"type":"canceled","turnID":"` + turn + `","text":"` + reason + `"}`
}

// writeReplayFixture writes dir/.ass-guard/transcript_<sid>.jsonl from lines
// (no trailing newline on the LAST line when torn is set — the torn-line case).
func writeReplayFixture(t *testing.T, dir, sid string, lines []string, torn bool) {
	t.Helper()

	err := os.MkdirAll(filepath.Join(dir, ".ass-guard"), 0o750)
	if err != nil {
		t.Fatalf("mkdir fixture store: %v", err)
	}

	body := strings.Join(lines, "\n")
	if !torn && len(lines) > 0 {
		body += "\n"
	}

	err = os.WriteFile(filepath.Join(dir, ".ass-guard", "transcript_"+sid+".jsonl"),
		[]byte(body), 0o600)
	if err != nil {
		t.Fatalf("write fixture transcript: %v", err)
	}
}

// assertFrames pins the exact frame sequence (order + fields).
func assertFrames(t *testing.T, got, want []recordedFrame) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("frame count = %d; want %d (got=%+v)", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("frame[%d] = %+v; want %+v", i, got[i], want[i])
		}
	}
}

// TestReplayTranscriptMapping is the full agent-visible vocabulary pin: every
// agent-visible line maps to EXACTLY ONE frame in transcript order; error
// lines render terminal (toolCallID-carrying) or as text (bare); canceled
// lines add nothing; torn tails and unknown future kinds skip tolerantly
// (16-D-20).
//
//nolint:funlen // a table over the whole vocabulary
func TestReplayTranscriptMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		lines []string
		torn  bool
		want  []recordedFrame
	}{
		{
			name: "error line with toolCallID closes the call as failed",
			lines: []string{
				fixtureToolCallLine(fixtureTurnOne, "tc-9", fixtureToolBash),
				fixtureErrorLine(fixtureTurnOne, "tc-9", "toolexec", "boom"),
			},
			want: []recordedFrame{
				{kind: updKindToolCall, toolCallID: "tc-9", title: fixtureToolBash},
				{kind: updKindToolCallUpdate, toolCallID: "tc-9", status: StatusFailed},
			},
		},
		{
			name: "bare error line renders as an agent chunk carrying the error text",
			lines: []string{
				fixtureErrorLine(fixtureTurnOne, "", "provider", "stream reset"),
			},
			want: []recordedFrame{
				{
					kind: updKindAgentMessageChunk, messageID: fixtureTurnOne,
					text: "provider: stream reset",
				},
			},
		},
		{
			name: "canceled turn emits no frames beyond its tool lines",
			lines: []string{
				fixtureUserLine(fixtureTurnOne, "go"),
				fixtureChunkLine(fixtureTurnOne, fixtureTurnOne, "partial"),
				fixtureToolCallLine(fixtureTurnOne, fixtureCallRead, fixtureToolRead),
				fixtureToolResultLine(fixtureTurnOne, fixtureCallRead, false),
				fixtureCanceledLine(fixtureTurnOne, "context cancelled during stream"),
			},
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: fixtureTurnOne, text: "partial"},
				{kind: updKindToolCall, toolCallID: fixtureCallRead, title: fixtureToolRead},
				{kind: updKindToolCallUpdate, toolCallID: fixtureCallRead, status: StatusCompleted},
			},
		},
		{
			name: "interleaved pairs across turns map one frame per line in order",
			lines: []string{
				fixtureUserLine(fixtureTurnOne, "a"),
				fixtureToolCallLine(fixtureTurnOne, "tc-a", fixtureToolRead),
				fixtureToolCallLine(fixtureTurnOne, "tc-b", "Grep"),
				fixtureToolResultLine(fixtureTurnOne, "tc-b", false),
				fixtureToolResultLine(fixtureTurnOne, "tc-a", true),
				fixtureAssistantLine(fixtureTurnOne, "done one"),
				fixtureUserLine("s-turn-002", "b"),
				fixtureToolCallLine("s-turn-002", "tc-c", fixtureToolBash),
				fixtureToolResultLine("s-turn-002", "tc-c", false),
				fixtureAssistantLine("s-turn-002", "done two"),
			},
			want: []recordedFrame{
				{kind: updKindToolCall, toolCallID: "tc-a", title: fixtureToolRead},
				{kind: updKindToolCall, toolCallID: "tc-b", title: "Grep"},
				{kind: updKindToolCallUpdate, toolCallID: "tc-b", status: StatusCompleted},
				{kind: updKindToolCallUpdate, toolCallID: "tc-a", status: StatusFailed},
				{kind: updKindAgentMessageChunk, messageID: fixtureTurnOne, text: "done one"},
				{kind: updKindToolCall, toolCallID: "tc-c", title: fixtureToolBash},
				{kind: updKindToolCallUpdate, toolCallID: "tc-c", status: StatusCompleted},
				{kind: updKindAgentMessageChunk, messageID: "s-turn-002", text: "done two"},
			},
		},
		{
			name: "chunk stream closed by a much-later assistant_message keeps order",
			lines: []string{
				fixtureChunkLine(fixtureTurnOne, "m1", "think "),
				fixtureToolCallLine(fixtureTurnOne, fixtureCallRead, fixtureToolRead),
				fixtureToolResultLine(fixtureTurnOne, fixtureCallRead, false),
				fixtureChunkLine(fixtureTurnOne, "m1", "some more"),
				fixtureAssistantLine(fixtureTurnOne, "think some more"),
			},
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "think "},
				{kind: updKindToolCall, toolCallID: fixtureCallRead, title: fixtureToolRead},
				{kind: updKindToolCallUpdate, toolCallID: fixtureCallRead, status: StatusCompleted},
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "some more"},
				{kind: updKindAgentMessageChunk, messageID: fixtureTurnOne, text: "think some more"},
			},
		},
		{
			name: "torn trailing garbage line is skipped without error",
			lines: []string{
				fixtureChunkLine(fixtureTurnOne, "m1", "ok"),
				`{"type":"agent_message_chunk","turnID":"` + fixtureTurnOne + `","text":"tor`,
			},
			torn: true,
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "ok"},
			},
		},
		{
			name: "unknown future kind is skipped, known kinds still frame",
			lines: []string{
				`{"type":"quantum_teleport","turnID":"` + fixtureTurnOne + `","qubit":"spin-up"}`,
				fixtureChunkLine(fixtureTurnOne, "m1", "known"),
			},
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "known"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			writeReplayFixture(t, dir, replayFixtureSID, tc.lines, tc.torn)

			rec := &recordingEmitter{}

			err := ReplayTranscript(rec, dir, replayFixtureSID)
			if err != nil {
				t.Fatalf("ReplayTranscript: %v", err)
			}

			assertFrames(t, rec.snapshot(), tc.want)
		})
	}
}

// TestReplayTranscriptRejectsBadSource pins the exported surface's guards: a
// malformed id rejects before any file open; a missing transcript errors; a
// planted symlink (non-regular source) rejects with the typed error (T-18-03).
func TestReplayTranscriptRejectsBadSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	rec := &recordingEmitter{}

	err := ReplayTranscript(rec, dir, "../../escape")
	if !errors.Is(err, errReplayMalformedID) {
		t.Errorf("malformed id error = %v; want errReplayMalformedID", err)
	}

	err = ReplayTranscript(rec, dir, replayFixtureSID)
	if err == nil {
		t.Error("missing transcript replay succeeded; want an error")
	}

	// Plant a symlink where the transcript belongs: the Lstat guard rejects
	// the link itself (T-18-03) — even one pointing at a regular file.
	target := filepath.Join(t.TempDir(), "real.jsonl")

	werr := os.WriteFile(target, []byte("{}\n"), 0o600)
	if werr != nil {
		t.Fatalf("write symlink target: %v", werr)
	}

	store := filepath.Join(dir, ".ass-guard")

	merr := os.MkdirAll(store, 0o750)
	if merr != nil {
		t.Fatalf("mkdir store: %v", merr)
	}

	link := filepath.Join(store, "transcript_"+replayFixtureSID+".jsonl")

	serr := os.Symlink(target, link)
	if serr != nil {
		t.Fatalf("symlink fixture: %v", serr)
	}

	err = ReplayTranscript(rec, dir, replayFixtureSID)
	if !errors.Is(err, errReplayNotRegular) {
		t.Errorf("symlink replay error = %v; want errReplayNotRegular", err)
	}
}

// --- 18-05: the full load-path ordering battery (D-02/D-03/ACP-06) ---

// loadOrderingSID fixtures' session id (a loadSessIDPattern-clean UUID form).
const loadOrderingSID = "bbbbbbbb-0b0b-4c0c-8d0d-0e0e0e0e0e0e"

// updKindAvailableCommands is the v1 sessionUpdate kind the load path
// re-advertises the resumed session's command set with (18-05, ACP-06
// "commands re-advertised").
const updKindAvailableCommands = "available_commands_update"

// modeStateView is the test-side view of the response's v1 modes field.
type modeStateView struct {
	AvailableModes []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"availableModes"` //nolint:tagliatelle // ACP wire field
	CurrentModeID string `json:"currentModeId"` //nolint:tagliatelle // ACP wire field
}

// danglingPlanModeLines is the class-01 + row-6 fixture: a user-message turn
// with a dangling Bash tool_call and an open chunk stream, whose LAST
// plan_mode line is an enter, and no session_end.
func danglingPlanModeLines(sid string) []string {
	turn := sid + "-turn-001"

	return []string{
		`{"type":"session_start","timestamp":"2026-09-02T12:00:00Z","text":"` + sid + `"}`,
		`{"type":"user_message","turnID":"` + turn + `","timestamp":"2026-09-02T12:00:00Z",` +
			`"content":[{"type":"text","text":"run it"}]}`,
		`{"type":"agent_message_chunk","turnID":"` + turn + `","timestamp":"2026-09-02T12:00:00Z",` +
			`"messageID":"` + turn + `","text":"working "}`,
		`{"type":"tool_call","turnID":"` + turn + `","timestamp":"2026-09-02T12:00:00Z",` +
			`"toolCallID":"call-9","name":"Bash","input":{"command":"make test"}}`,
		`{"type":"plan_mode","turnID":"` + turn + `","timestamp":"2026-09-02T12:00:00Z",` +
			`"cause":"plan_mode_enter","toolCallID":"call-8"}`,
	}
}

// danglingPlainLines is the same dangling shape with NO plan_mode line (the
// modes-null arm).
func danglingPlainLines(sid string) []string {
	turn := sid + "-turn-001"

	return []string{
		`{"type":"session_start","timestamp":"2026-09-02T12:00:00Z","text":"` + sid + `"}`,
		`{"type":"user_message","turnID":"` + turn + `","timestamp":"2026-09-02T12:00:00Z",` +
			`"content":[{"type":"text","text":"run it"}]}`,
		`{"type":"tool_call","turnID":"` + turn + `","timestamp":"2026-09-02T12:00:00Z",` +
			`"toolCallID":"call-9","name":"Bash","input":{"command":"make test"}}`,
	}
}

// manyChunkLines pads a fixture with chunk lines so an unread client pipe
// deterministically parks the replay mid-stream (the 18-01 gate-test trick).
func manyChunkLines(sid string, n int) []string {
	turn := sid + "-turn-001"

	out := make([]string, 0, n+1)

	for i := range n {
		out = append(out, `{"type":"agent_message_chunk","turnID":"`+turn+
			`","timestamp":"2026-09-02T12:00:00Z","messageID":"`+turn+`","text":"c`+
			strconv.Itoa(i)+`"}`)
	}

	return out
}

// sendPrompt sends one session/prompt for the id (the battery's shared shape).
func sendPrompt(t *testing.T, h *pipeHarness, reqID int, sessionID, text string) {
	t.Helper()

	h.send(t, newRequest(reqID, "session/prompt", map[string]any{
		keySessionID:  sessionID,
		testKeyPrompt: []any{map[string]string{testKeyTxtBlock: blockText, blockText: text}},
	}))
}

// decodeLoadModes decodes the load response's modes field (nil-safe).
func decodeLoadModes(t *testing.T, msg *Message) *modeStateView {
	t.Helper()

	var res struct {
		Modes *modeStateView `json:"modes"`
	}

	uerr := json.Unmarshal(msg.Result, &res)
	if uerr != nil {
		t.Fatalf("unmarshal load result: %v (raw=%s)", uerr, string(msg.Result))
	}

	return res.Modes
}

// TestLoadFullOrdering pins the D-03/D-02/ACP-06 ordering contract end-to-end
// through the Server load core: the client pipe receives the replay INCLUDING
// the synthetic closures' terminal frames, then the available_commands_update
// re-advertisement, then the response — whose modes field carries the seeded
// plan-mode state exactly when the fixture's last plan_mode line is an enter
// (null otherwise); a prompt DURING the parked replay is typed-rejected, and
// a prompt after load is accepted with the turn id continuing the sequence.
//
//nolint:funlen,gocyclo,cyclop // one ordered wire-flow assertion end-to-end
func TestLoadFullOrdering(t *testing.T) {
	t.Parallel()

	store := t.TempDir()
	sid := loadOrderingSID

	// Enough chunk lines that the unread client pipe + shrunken foreground
	// lane park the replay mid-stream (the D-03 during-replay arm).
	lines := append(danglingPlanModeLines(sid), manyChunkLines(sid, 60)...)

	writeLoadFixture(t, store, sid, lines)

	runner := newFakeResumeRunner(store)

	h := newPipeHarness(t,
		WithWorkDir(store),
		WithTurnRunner(runner),
		WithTurnEmitter(TurnEmitterConfig{ForegroundCapacity: 1}))

	sendLoad(t, h, 1, sid, store)

	// One frame read: the replay is provably underway; the unread pipe parks it.
	first := h.readFrame(t)
	if first.Method != methodSessionUpdate {
		t.Fatalf("first frame after load = %v; want a session/update", first.Method)
	}

	// The concurrent prompt: typed rejection naming the replay state (D-03).
	sendPrompt(t, h, 2, sid, "too early")

	during := readUntilResponse(t, h, 2, nil)
	if during.Error == nil {
		t.Fatalf("prompt during replay accepted: %s (D-03 violated)", string(during.Result))
	}

	if !strings.Contains(during.Error.Message, "replay") {
		t.Errorf("prompt-during-replay message = %q; want it to name the replay state",
			during.Error.Message)
	}

	// Drain: collect the wire story until the load response.
	var (
		kinds        []string
		sawClosure   bool
		sawCommands  bool
		commandsAt   = -1
		lastReplayAt = -1
	)

	loadResp := readUntilResponse(t, h, 1, func(u *updateFrame) {
		at := len(kinds)
		kinds = append(kinds, u.Update.SessionUpdate)

		switch u.Update.SessionUpdate {
		case updKindToolCallUpdate:
			// The synthetic closure renders as a terminal failed update for
			// the dangling call (isError=true -> StatusFailed).
			if u.Update.ToolCallID == "call-9" && u.Update.Status == StatusFailed {
				sawClosure = true
				lastReplayAt = at
			}
		case updKindAvailableCommands:
			sawCommands = true
			commandsAt = at
		case updKindAgentMessageChunk, updKindToolCall:
			lastReplayAt = at
		}
	})

	if loadResp.Error != nil {
		t.Fatalf("session/load errored: %+v", loadResp.Error)
	}

	if !sawClosure {
		t.Errorf("replay missing the closure's terminal frame (call-9 failed); kinds=%v", kinds)
	}

	if !sawCommands {
		t.Fatalf("no available_commands_update frame after the replay frames; kinds=%v", kinds)
	}

	if commandsAt < lastReplayAt {
		t.Errorf("available_commands_update at %d precedes the last replay frame at %d; "+
			"want it AFTER the replay, BEFORE the response (kinds=%v)", commandsAt, lastReplayAt, kinds)
	}

	if len(loadResp.Result) == 0 || !json.Valid(loadResp.Result) {
		t.Fatalf("load result malformed: %s", string(loadResp.Result))
	}

	// modes carries the seeded plan-mode state (the fixture's last plan_mode
	// line is an enter): the v1 SessionModeState shape with currentModeId plan.
	modes := decodeLoadModes(t, loadResp)
	if modes == nil {
		t.Fatalf("load response modes = null; want the seeded plan-mode state (raw=%s)",
			string(loadResp.Result))
	}

	if modes.CurrentModeID != "plan" {
		t.Errorf("modes.currentModeId = %q; want \"plan\" (the persisted target)", modes.CurrentModeID)
	}

	if len(modes.AvailableModes) < 2 {
		t.Errorf("modes.availableModes = %+v; want the default+plan pair", modes.AvailableModes)
	}

	// The post-load prompt is accepted; its turn id continues the sequence.
	sendPrompt(t, h, 3, sid, "after load")

	after := readUntilResponse(t, h, 3, nil)
	if after.Error != nil {
		t.Fatalf("post-load prompt errored: %+v (the gate must open after replay)", after.Error)
	}

	wantTurn := sid + "-turn-002"

	found := false

	for _, id := range fixtureTurnIDs(t, store, sid) {
		if id == wantTurn {
			found = true
		}
	}

	if !found {
		t.Errorf("post-load turn id did not continue the sequence: no %q on disk", wantTurn)
	}
}

// TestLoadModesNullWithoutPlanMode pins the modes-null arm: a dangling fixture
// with NO plan_mode line answers modes null (the client assumes defaults).
func TestLoadModesNullWithoutPlanMode(t *testing.T) {
	t.Parallel()

	store := t.TempDir()
	sid := loadOrderingSID

	writeLoadFixture(t, store, sid, danglingPlainLines(sid))

	h := newPipeHarness(t, WithWorkDir(store), WithTurnRunner(newFakeResumeRunner(store)))

	sendLoad(t, h, 1, sid, store)

	loadResp := readUntilResponse(t, h, 1, nil)
	if loadResp.Error != nil {
		t.Fatalf("session/load errored: %+v", loadResp.Error)
	}

	if modes := decodeLoadModes(t, loadResp); modes != nil {
		t.Errorf("load response modes = %+v; want null (no plan_mode line on the transcript)", modes)
	}

	if !strings.Contains(string(loadResp.Result), `"modes":null`) {
		t.Errorf("load result = %s; want the modes key present and null", string(loadResp.Result))
	}
}

// TestLoadDoubleRegistration pins the 18-05 registration guard: loading an
// already-registered live session id returns the TYPED already-active error,
// while a load whose predecessor died mid-load (closures appended,
// registration never reached — constructed by calling ResumeSession directly
// without the handler) completes successfully (kill-during-replay
// idempotency, the second ResumeSession classifies clean).
func TestLoadDoubleRegistration(t *testing.T) {
	t.Parallel()

	store := t.TempDir()
	sid := loadOrderingSID

	writeLoadFixture(t, store, sid, danglingPlainLines(sid))

	runner := newFakeResumeRunner(store)
	h := newPipeHarness(t, WithWorkDir(store), WithTurnRunner(runner))

	sendLoad(t, h, 1, sid, store)

	first := readUntilResponse(t, h, 1, nil)
	if first.Error != nil {
		t.Fatalf("first session/load errored: %+v", first.Error)
	}

	// The already-registered live id: the typed already-active error.
	sendLoad(t, h, 2, sid, store)

	second := readUntilResponse(t, h, 2, nil)
	if second.Error == nil {
		t.Fatalf("second session/load of the live id succeeded: %s (want the typed already-active error)",
			string(second.Result))
	}

	if second.Error.Code == 0 {
		t.Error("already-active error code = 0; want a typed JSON-RPC error code")
	}

	if !strings.Contains(second.Error.Message, "already active") {
		t.Errorf("already-active message = %q; want it to name the already-active state",
			second.Error.Message)
	}

	// The simulated mid-load interruption: the runner resumed (closures on
	// disk) but the handler-side registration never happened.
	sid2 := "cccccccc-0b0b-4c0c-8d0d-0e0e0e0e0e0e"

	writeLoadFixture(t, store, sid2, danglingPlainLines(sid2))

	rerr := runner.ResumeSession(context.Background(), sid2)
	if rerr != nil {
		t.Fatalf("direct ResumeSession: %v", rerr)
	}

	sendLoad(t, h, 3, sid2, store)

	third := readUntilResponse(t, h, 3, nil)
	if third.Error != nil {
		t.Fatalf("load after the simulated mid-load interruption errored: %+v (idempotency violated)",
			third.Error)
	}

	// The interrupted-then-completed session accepts prompts.
	sendPrompt(t, h, 4, sid2, "continue")

	fourth := readUntilResponse(t, h, 4, nil)
	if fourth.Error != nil {
		t.Fatalf("post-load prompt on the interrupted-then-completed session errored: %+v", fourth.Error)
	}
}
