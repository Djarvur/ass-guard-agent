package acp //nolint:testpackage // internal package test

// 18-01 (ACP-06): the session/load replay tracer battery. Drives a real Server
// over the pipe harness against a hand-written store fixture — the same
// in-memory-pipe conventions server_test.go established.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixtureSessionID is a loadSessIDPattern-clean RFC 4122 v4 UUID form id (the
// branch internal/acp newSessionID mints and transcript_<id>.jsonl names).
const fixtureSessionID = "11111111-2222-4333-8444-555555555555"

// fixtureTimestamp is a fixed RFC 3339 stamp for deterministic fixture lines.
const fixtureTimestamp = "2026-09-01T10:00:00Z"

// fixtureToolCallID is the one tool call the clean fixture carries (goconst).
const fixtureToolCallID = "call-1"

// writeLoadFixture writes the .ass-guard store transcript for sessionID from
// raw JSONL lines (hand-written session.Line-shaped JSON — the plan's fixture
// discipline; no session-package dependency on the acp test side).
func writeLoadFixture(t *testing.T, storeDir, sessionID string, lines []string) {
	t.Helper()

	dir := filepath.Join(storeDir, ".ass-guard")

	err := os.MkdirAll(dir, 0o750)
	if err != nil {
		t.Fatalf("mkdir fixture store: %v", err)
	}

	body := strings.Join(lines, "\n") + "\n"

	path := filepath.Join(dir, "transcript_"+sessionID+".jsonl")

	err = os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write fixture transcript: %v", err)
	}
}

// cleanSessionFixtureLines is a cleanly-closed single-turn session: start,
// user message, streamed chunks + final assistant message, one tool call +
// result, boundary, end. Turn suffixes top out at 001.
func cleanSessionFixtureLines(sid string) []string {
	turn := sid + "-turn-001"

	return []string{
		`{"type":"session_start","timestamp":"` + fixtureTimestamp + `","text":"` + sid + `"}`,
		`{"type":"user_message","turnID":"` + turn + `","timestamp":"` + fixtureTimestamp + `",` +
			`"content":[{"type":"text","text":"list the files"}]}`,
		`{"type":"agent_message_chunk","turnID":"` + turn + `","timestamp":"` + fixtureTimestamp + `",` +
			`"messageID":"` + turn + `","text":"Here "}`,
		`{"type":"agent_message_chunk","turnID":"` + turn + `","timestamp":"` + fixtureTimestamp + `",` +
			`"messageID":"` + turn + `","text":"is the listing."}`,
		`{"type":"assistant_message","turnID":"` + turn + `","timestamp":"` + fixtureTimestamp + `",` +
			`"text":"Here is the listing."}`,
		`{"type":"tool_call","turnID":"` + turn + `","timestamp":"` + fixtureTimestamp + `",` +
			`"toolCallID":"` + fixtureToolCallID + `","name":"Bash","input":{"command":"ls"}}`,
		`{"type":"tool_result","turnID":"` + turn + `","timestamp":"` + fixtureTimestamp + `",` +
			`"toolCallID":"` + fixtureToolCallID + `","output":{"stdout":"a.go"},"isError":false}`,
		`{"type":"boundary","turnID":"` + turn + `","timestamp":"` + fixtureTimestamp + `",` +
			`"cause":"mutating-command:Bash"}`,
		`{"type":"session_end","timestamp":"` + fixtureTimestamp + `"}`,
	}
}

// fixtureTurnIDs returns every parsed turnID on the transcript (append order).
func fixtureTurnIDs(t *testing.T, storeDir, sessionID string) []string {
	t.Helper()

	path := filepath.Join(storeDir, ".ass-guard", "transcript_"+sessionID+".jsonl")

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open transcript for turn ids: %v", err)
	}

	defer func() { _ = f.Close() }()

	var ids []string

	sc := bufio.NewScanner(f)

	for sc.Scan() {
		var l struct {
			TurnID string `json:"turnID"` //nolint:tagliatelle // on-disk format
		}

		if json.Unmarshal(sc.Bytes(), &l) == nil && l.TurnID != "" {
			ids = append(ids, l.TurnID)
		}
	}

	err = sc.Err()
	if err != nil {
		t.Fatalf("scan transcript turn ids: %v", err)
	}

	return ids
}

// fakeResumeRunner is the Session-family test TurnRunner: it implements the
// TurnRunner seam AND the 18-01 SessionLoader optional capability the way
// runtime.Runner does — ResumeSession adopts the transcript's id seeding the
// turn counter from the transcript maxima; Run then advances the counter
// (Add-then-format, Session.nextTurnID semantics) and appends the new turn's
// line, making the id-continuation contract observable on disk.
type fakeResumeRunner struct {
	workDir string

	mu   sync.Mutex
	seed map[string]int64 // sessionID -> current turn counter
}

func newFakeResumeRunner(workDir string) *fakeResumeRunner {
	return &fakeResumeRunner{workDir: workDir, seed: map[string]int64{}}
}

// ResumeSession seeds the per-session counter from the transcript's max
// <sessionID>-turn-%03d suffix (Pitfall 2: ids continue, never restart).
func (f *fakeResumeRunner) ResumeSession(_ context.Context, sessionID string) error {
	ids := scanTurnIDs(filepath.Join(f.workDir, ".ass-guard", "transcript_"+sessionID+".jsonl"))

	prefix := sessionID + "-turn-"

	var turnMax int64

	for _, id := range ids {
		if !strings.HasPrefix(id, prefix) {
			continue
		}

		n, err := strconv.ParseInt(strings.TrimPrefix(id, prefix), 10, 64)
		if err != nil || n <= 0 {
			continue
		}

		if n > turnMax {
			turnMax = n
		}
	}

	f.mu.Lock()
	f.seed[sessionID] = turnMax
	f.mu.Unlock()

	return nil
}

// Run appends the next turn's user_message line (counter+1, %03d) and emits
// one chunk — the minimal observable post-load turn.
func (f *fakeResumeRunner) Run(
	_ context.Context, sessionID string, emit ChunkEmitter, _ []ContentBlock,
) (string, error) {
	f.mu.Lock()
	f.seed[sessionID]++

	n := f.seed[sessionID]
	f.mu.Unlock()

	next := fmt.Sprintf("%s-turn-%03d", sessionID, n)

	line, merr := json.Marshal(map[string]any{
		testKeyTxtBlock: "user_message", "turnID": next,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	if merr != nil {
		return "", fmt.Errorf("fake runner marshal: %w", merr)
	}

	path := filepath.Join(f.workDir, ".ass-guard", "transcript_"+sessionID+".jsonl")

	fout, oerr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if oerr != nil {
		return "", fmt.Errorf("fake runner open transcript: %w", oerr)
	}

	_, werr := fout.Write(append(line, '\n'))
	if werr != nil {
		_ = fout.Close()

		return "", fmt.Errorf("fake runner append: %w", werr)
	}

	cerr := fout.Close()
	if cerr != nil {
		return "", fmt.Errorf("fake runner close: %w", cerr)
	}

	eerr := emit.AgentMessageChunk(next, "turn ok")
	if eerr != nil {
		return "", fmt.Errorf("fake runner emit: %w", eerr)
	}

	return stopEndTurn, nil
}

// scanTurnIDs is the fake's transcript turnID reader (nil on any open error —
// the seed then degrades to 0, the same loud-degrade contract the runtime has).
func scanTurnIDs(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}

	defer func() { _ = f.Close() }()

	var ids []string

	sc := bufio.NewScanner(f)

	for sc.Scan() {
		var l struct {
			TurnID string `json:"turnID"` //nolint:tagliatelle // on-disk format
		}

		if json.Unmarshal(sc.Bytes(), &l) == nil && l.TurnID != "" {
			ids = append(ids, l.TurnID)
		}
	}

	return ids
}

// updateFrame is the test-side view of one session/update notification.
type updateFrame struct {
	Update struct {
		SessionUpdate string          `json:"sessionUpdate"` //nolint:tagliatelle // ACP wire field
		ToolCallID    string          `json:"toolCallId"`    //nolint:tagliatelle // ACP wire field
		Title         string          `json:"title,omitempty"`
		Status        string          `json:"status,omitempty"`
		MessageID     string          `json:"messageId,omitempty"` //nolint:tagliatelle // ACP wire field
		Content       json.RawMessage `json:"content,omitempty"`
	} `json:"update"`
}

// chunkTextOf extracts the text of one agent_message_chunk frame.
func chunkTextOf(t *testing.T, u *updateFrame) string {
	t.Helper()

	var cb ContentBlock

	uerr := json.Unmarshal(u.Update.Content, &cb)
	if uerr != nil {
		t.Fatalf("unmarshal chunk content: %v (raw=%s)", uerr, string(u.Update.Content))
	}

	return cb.Text
}

// readUntilResponse reads frames (handing each session/update to cb) until the
// response with the given integer id arrives; bounded so a pathological frame
// stream fails instead of hanging. The bound comfortably exceeds the writer
// buffer + lane capacities of the slowed-replay gate test.
func readUntilResponse(t *testing.T, h *pipeHarness, wantID int, cb func(*updateFrame)) *Message {
	t.Helper()

	want := strconv.Itoa(wantID)

	const maxFrames = 1024

	for range maxFrames {
		msg := h.readFrame(t)

		if msg.ID != nil && string(msg.ID) == want {
			return msg
		}

		if msg.Method == methodSessionUpdate {
			var u updateFrame

			uerr := json.Unmarshal(msg.Params, &u)
			if uerr != nil {
				t.Fatalf("unmarshal session/update params: %v (raw=%s)", uerr, string(msg.Params))
			}

			if cb != nil {
				cb(&u)
			}
		}
	}

	t.Fatalf("no response with id %s within %d frames", want, maxFrames)

	return nil
}

// sendLoad sends one session/load request for the id (the shared request
// shape of the battery).
func sendLoad(t *testing.T, h *pipeHarness, reqID int, sessionID, cwd string) {
	t.Helper()

	h.send(t, newRequest(reqID, "session/load", map[string]any{
		keySessionID: sessionID, keyCwd: cwd, keyMcpServers: []any{},
	}))
}

// assertLoadRejected pins the typed-error contract: a typed JSON-RPC code (not
// the zero value) whose message names the rejection cause.
func assertLoadRejected(t *testing.T, msg *Message, wantID int, wantCause string) {
	t.Helper()

	if msg.ID == nil || string(msg.ID) != strconv.Itoa(wantID) {
		t.Fatalf("response id = %v; want %d", msg.ID, wantID)
	}

	if msg.Error == nil {
		t.Fatalf("session/load unexpectedly succeeded: %s", string(msg.Result))
	}

	if msg.Error.Code == 0 {
		t.Error("load rejection code = 0; want a typed JSON-RPC error code")
	}

	if !strings.Contains(msg.Error.Message, wantCause) {
		t.Errorf("load rejection message = %q; want it to name %q", msg.Error.Message, wantCause)
	}
}

// assertPromptNotAccepted proves no sessionState exists for the id (a prompt
// errors instead of running a turn).
func assertPromptNotAccepted(t *testing.T, h *pipeHarness, reqID int, sessionID string) {
	t.Helper()

	h.send(t, newRequest(reqID, "session/prompt", map[string]any{
		keySessionID:  sessionID,
		testKeyPrompt: []any{map[string]string{testKeyTxtBlock: blockText, blockText: "hi"}},
	}))

	msg := h.readFrame(t)

	if msg.Error == nil {
		t.Fatalf("session/prompt on non-loaded session %q accepted: %s", sessionID, string(msg.Result))
	}
}

// TestSessionLoadReplaysCleanSession is the 18-01 tracer: a cleanly-closed
// past session replays as ordered session/update frames BEFORE the load
// response, the response carries the exact v1 shape (configOptions + modes,
// NO sessionId), and the session accepts prompts whose turn id continues the
// fixture's sequence (max 001 -> next 002).
//
//nolint:funlen,gocyclo,cyclop // one ordered wire-flow assertion end-to-end (the tracer)
func TestSessionLoadReplaysCleanSession(t *testing.T) {
	t.Parallel()

	store := t.TempDir()
	sid := fixtureSessionID

	writeLoadFixture(t, store, sid, cleanSessionFixtureLines(sid))

	runner := newFakeResumeRunner(store)
	h := newPipeHarness(t, WithWorkDir(store), WithTurnRunner(runner))

	// initialize advertises loadSession true (the Phase-18 flip).
	h.send(t, newRequest(0, methodInitialize, zedLikeInitializeParams()))

	initResp := h.readFrame(t)
	if initResp.Error != nil {
		t.Fatalf("initialize errored: %+v", initResp.Error)
	}

	var ires struct {
		AgentCapabilities struct {
			LoadSession bool `json:"loadSession"` //nolint:tagliatelle // ACP wire field
		} `json:"agentCapabilities"` //nolint:tagliatelle // ACP wire field
	}

	uerr := json.Unmarshal(initResp.Result, &ires)
	if uerr != nil {
		t.Fatalf("unmarshal initialize result: %v (raw=%s)", uerr, string(initResp.Result))
	}

	if !ires.AgentCapabilities.LoadSession {
		t.Error("agentCapabilities.loadSession = false; want true (Phase 18 — replay is live)")
	}

	// session/load: replay frames stream BEFORE the response is readable.
	sendLoad(t, h, 1, sid, store)

	var (
		kinds      []string
		chunkText  strings.Builder
		sawTool    bool
		sawUpdOK   bool
		sawUpdFail bool
	)

	loadResp := readUntilResponse(t, h, 1, func(u *updateFrame) {
		kinds = append(kinds, u.Update.SessionUpdate)

		switch u.Update.SessionUpdate {
		case updKindAgentMessageChunk:
			chunkText.WriteString(chunkTextOf(t, u))
		case updKindToolCall:
			sawTool = u.Update.ToolCallID == fixtureToolCallID && u.Update.Title == "Bash"
		case updKindToolCallUpdate:
			sawUpdOK = u.Update.ToolCallID == fixtureToolCallID && u.Update.Status == StatusCompleted
			sawUpdFail = u.Update.ToolCallID == fixtureToolCallID
		}
	})

	if loadResp.Error != nil {
		t.Fatalf("session/load errored: %+v (stderr=%s)", loadResp.Error, h.stderr.String())
	}

	// Frame sequence: the assistant text as chunks (2 stream chunks + the
	// assistant_message final chunk), then the tool_call, then the terminal
	// tool_call_update — transcript order, and nothing for the bookkeeping
	// kinds (user_message/session_start/boundary/session_end emit no frame).
	wantKinds := []string{
		updKindAgentMessageChunk, updKindAgentMessageChunk, updKindAgentMessageChunk,
		updKindToolCall, updKindToolCallUpdate,
	}

	if strings.Join(kinds, ",") != strings.Join(wantKinds, ",") {
		t.Errorf("replay kinds = %v; want %v", kinds, wantKinds)
	}

	if got := chunkText.String(); got != "Here is the listing.Here is the listing." {
		t.Errorf("replayed chunk text = %q; want the streamed + final chunks", got)
	}

	if !sawTool {
		t.Error("replay missing the tool_call frame (toolCallId=call-1, title=Bash)")
	}

	if !sawUpdOK || !sawUpdFail {
		t.Error("replay missing the terminal tool_call_update (call-1, completed)")
	}

	// Response shape: exactly configOptions + modes present; no sessionId key.
	var res map[string]any

	uerr = json.Unmarshal(loadResp.Result, &res)
	if uerr != nil {
		t.Fatalf("unmarshal load result: %v (raw=%s)", uerr, string(loadResp.Result))
	}

	if _, ok := res["configOptions"]; !ok {
		t.Error("load response missing configOptions key (v1 required property)")
	}

	if _, ok := res["modes"]; !ok {
		t.Error("load response missing modes key (v1 required property)")
	}

	if _, ok := res[keySessionID]; ok {
		t.Error("load response carries a sessionId key; v1 LoadSessionResponse has NONE (Pitfall 7)")
	}

	// Post-load prompt accepted; its turn id continues the fixture sequence.
	h.send(t, newRequest(2, "session/prompt", map[string]any{
		keySessionID:  sid,
		testKeyPrompt: []any{map[string]string{testKeyTxtBlock: blockText, blockText: "again"}},
	}))

	promptResp := readUntilResponse(t, h, 2, nil)
	if promptResp.Error != nil {
		t.Fatalf("post-load session/prompt errored: %+v", promptResp.Error)
	}

	wantTurn := sid + "-turn-002"

	found := false

	for _, id := range fixtureTurnIDs(t, store, sid) {
		if id == wantTurn {
			found = true
		}
	}

	if !found {
		t.Errorf("post-load turn id did not continue the sequence: no %q on disk (have %v)",
			wantTurn, fixtureTurnIDs(t, store, sid))
	}
}

// TestSessionLoadTombstonedRejected: a zero-byte <id>.deleted sibling refuses
// the load with a typed error naming the tombstone — and creates no
// sessionState, no transcript mutation.
func TestSessionLoadTombstonedRejected(t *testing.T) {
	t.Parallel()

	store := t.TempDir()
	sid := "22222222-3333-4444-8555-666666666666"

	writeLoadFixture(t, store, sid, cleanSessionFixtureLines(sid))

	tomb := filepath.Join(store, ".ass-guard", sid+".deleted")

	err := os.WriteFile(tomb, nil, 0o600)
	if err != nil {
		t.Fatalf("write tombstone: %v", err)
	}

	before, rerr := os.ReadFile(filepath.Join(store, ".ass-guard", "transcript_"+sid+".jsonl"))
	if rerr != nil {
		t.Fatalf("read transcript before: %v", rerr)
	}

	h := newPipeHarness(t, WithWorkDir(store), WithTurnRunner(newFakeResumeRunner(store)))

	sendLoad(t, h, 0, sid, store)

	assertLoadRejected(t, h.readFrame(t), 0, "deleted")

	after, rerr := os.ReadFile(filepath.Join(store, ".ass-guard", "transcript_"+sid+".jsonl"))
	if rerr != nil {
		t.Fatalf("read transcript after: %v", rerr)
	}

	if !bytes.Equal(before, after) {
		t.Error("tombstoned load mutated the transcript bytes; rejection must be side-effect free")
	}

	assertPromptNotAccepted(t, h, 1, sid)
}

// TestSessionLoadUnknownIDRejected: no transcript_<id>.jsonl means a typed
// unknown-session error — never a fresh session under the client's id.
func TestSessionLoadUnknownIDRejected(t *testing.T) {
	t.Parallel()

	store := t.TempDir()

	err := os.MkdirAll(filepath.Join(store, ".ass-guard"), 0o750)
	if err != nil {
		t.Fatalf("mkdir store: %v", err)
	}

	sid := "33333333-4444-5555-8666-777777777777"

	h := newPipeHarness(t, WithWorkDir(store), WithTurnRunner(newFakeResumeRunner(store)))

	sendLoad(t, h, 0, sid, store)

	assertLoadRejected(t, h.readFrame(t), 0, sid)

	_, err = os.Stat(filepath.Join(store, ".ass-guard", "transcript_"+sid+".jsonl"))
	if err == nil {
		t.Error("unknown-id load created a transcript file; load never creates a fresh session")
	}

	assertPromptNotAccepted(t, h, 1, sid)
}

// TestSessionLoadTraversalIDRejected: traversal-shaped ids (slash-bearing,
// dots-only) reject structurally BEFORE any file open.
func TestSessionLoadTraversalIDRejected(t *testing.T) {
	t.Parallel()

	store := t.TempDir()

	err := os.MkdirAll(filepath.Join(store, ".ass-guard"), 0o750)
	if err != nil {
		t.Fatalf("mkdir store: %v", err)
	}

	h := newPipeHarness(t, WithWorkDir(store), WithTurnRunner(newFakeResumeRunner(store)))

	for i, sid := range []string{"../../etc/passwd", "..", "a/b"} {
		sendLoad(t, h, i, sid, store)

		assertLoadRejected(t, h.readFrame(t), i, keySessionID)
	}

	entries, derr := os.ReadDir(filepath.Join(store, ".ass-guard"))
	if derr != nil {
		t.Fatalf("readdir store after rejections: %v", derr)
	}

	if len(entries) != 0 {
		t.Errorf("traversal rejections left files in the store: %v", entries)
	}
}

// TestLoadGateRejectsPromptDuringReplay is the D-03 gate under concurrency:
// while a load's replay is still streaming (paused mid-stream by an unread
// client pipe — the io.Pipe backpressure fills the writer buffer + the shrunken
// foreground lane, blocking the replay's next enqueue), a concurrent
// session/prompt for that id returns the TYPED replay-in-progress error; after
// the load completes, the SAME prompt is accepted. Run under -race: the ready
// flag and the sessions map are accessed from concurrent handler goroutines.
func TestLoadGateRejectsPromptDuringReplay(t *testing.T) {
	t.Parallel()

	store := t.TempDir()
	sid := "44444444-5555-6666-8777-888888888888"

	// 400 chunk lines: more than the writer buffer (256) + lane capacity can
	// absorb, so replay deterministically parks mid-stream until the client
	// starts draining.
	const chunkLines = 400

	lines := []string{
		`{"type":"session_start","timestamp":"` + fixtureTimestamp + `","text":"` + sid + `"}`,
	}
	for i := range chunkLines {
		turn := sid + "-turn-001"

		lines = append(lines, `{"type":"agent_message_chunk","turnID":"`+turn+
			`","timestamp":"`+fixtureTimestamp+`","messageID":"`+turn+`","text":"c`+
			strconv.Itoa(i)+`"}`)
	}
	lines = append(lines, `{"type":"session_end","timestamp":"`+fixtureTimestamp+`"}`)

	writeLoadFixture(t, store, sid, lines)

	runner := newFakeResumeRunner(store)

	// Foreground lane of 1: the replay's second pending frame already blocks
	// once the drain stalls on the unread client pipe.
	h := newPipeHarness(t,
		WithWorkDir(store),
		WithTurnRunner(runner),
		WithTurnEmitter(TurnEmitterConfig{ForegroundCapacity: 1}))

	sendLoad(t, h, 1, sid, store)

	// Read ONE frame: replay is provably underway (its first chunk reached
	// the client), so the loading marker is set. The unread pipe then stalls
	// the drain; the lane (capacity 1) fills; the replay parks mid-stream.
	first := h.readFrame(t)
	if first.Method != methodSessionUpdate {
		t.Fatalf("first frame after load = %v; want a session/update chunk", first.Method)
	}

	// The concurrent prompt: typed rejection naming the replay state — never
	// a turn interleaved with the replayed frames.
	h.send(t, newRequest(2, "session/prompt", map[string]any{
		keySessionID:  sid,
		testKeyPrompt: []any{map[string]string{testKeyTxtBlock: blockText, blockText: "early"}},
	}))

	during := readUntilResponse(t, h, 2, nil)
	if during.Error == nil {
		t.Fatalf("prompt during replay accepted: %s (D-03 violated)", string(during.Result))
	}

	if during.Error.Code != CodeInvalidRequest {
		t.Errorf("prompt-during-replay code = %d; want %d (typed)", during.Error.Code, CodeInvalidRequest)
	}

	if !strings.Contains(during.Error.Message, "replay") {
		t.Errorf("prompt-during-replay message = %q; want it to name the replay state",
			during.Error.Message)
	}

	// Drain the rest: the parked replay resumes, the load response lands
	// AFTER its last replayed frame (updates-before-response).
	loadResp := readUntilResponse(t, h, 1, nil)
	if loadResp.Error != nil {
		t.Fatalf("session/load errored: %+v", loadResp.Error)
	}

	// The SAME prompt is accepted now that the session is ready.
	h.send(t, newRequest(3, "session/prompt", map[string]any{
		keySessionID:  sid,
		testKeyPrompt: []any{map[string]string{testKeyTxtBlock: blockText, blockText: "after"}},
	}))

	after := readUntilResponse(t, h, 3, nil)
	if after.Error != nil {
		t.Fatalf("post-load prompt errored: %+v (gate must open after replay)", after.Error)
	}
}
