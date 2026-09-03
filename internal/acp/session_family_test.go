package acp //nolint:testpackage // internal package test

// 18-01 (ACP-06): the session/load replay tracer battery. Drives a real Server
// over the pipe harness against a hand-written store fixture — the same
// in-memory-pipe conventions server_test.go established.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
	"github.com/Djarvur/ass-guard-agent/internal/session"
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
			`"toolCallID":"` + fixtureToolCallID + `","name":"` + fixtureToolBash + `","input":{"command":"ls"}}`,
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
// runtime.Runner does — 18-05 widened the mirror to the FULL resume contract:
// ResumeSession reconciles the transcript (session.Reconcile), appends every
// provenance-marked closure through Manager.AppendSynthetic, seeds the turn
// counter from the maxima, and records the seeded plan-mode state for
// ModeStateProvider (LoadedModes). Run then advances the counter (Add-then-
// format, Session.nextTurnID semantics) and appends the new turn's line,
// making the id-continuation contract observable on disk.
type fakeResumeRunner struct {
	workDir string

	mu    sync.Mutex
	seed  map[string]int64 // sessionID -> current turn counter
	modes map[string]any   // sessionID -> seeded v1 SessionModeState shape (nil = wire null)
}

func newFakeResumeRunner(workDir string) *fakeResumeRunner {
	return &fakeResumeRunner{workDir: workDir, seed: map[string]int64{}, modes: map[string]any{}}
}

// passthroughRedactor is the fake's no-op session.Redactor (synthetic
// closures cross the same append path live lines do — content unchanged).
type passthroughRedactor struct{}

func (passthroughRedactor) Redact(b []byte) ([]byte, error) { return b, nil }
func (passthroughRedactor) ScrubError(err error) string     { return err.Error() }

// fakeModeEntry builds one availableModes row (the production ids verbatim —
// session.ModeIDDefault/ModeIDPlan).
func fakeModeEntry(id, name string) map[string]string {
	return map[string]string{"id": id, "name": name}
}

// fakeModesShape mirrors the v1 SessionModeState shape the production load
// path builds (availableModes + currentModeId; the fake pins the wire shape
// independently of session's builder).
func fakeModesShape(seed session.Seed) any {
	if !seed.PlanModePresent {
		return nil
	}

	current := session.ModeIDDefault
	if seed.PlanMode == session.PlanModeCauseEnter {
		current = session.ModeIDPlan
	}

	return map[string]any{
		"availableModes": []map[string]string{
			fakeModeEntry(session.ModeIDDefault, "Default"),
			fakeModeEntry(session.ModeIDPlan, "Plan"),
		},
		"currentModeId": current,
	}
}

// ResumeSession performs the full 18-05 transcript-side resume (the
// runtime.Runner mirror): reconcile → append closures → seed (counter +
// plan-mode target).
func (f *fakeResumeRunner) ResumeSession(_ context.Context, sessionID string) error {
	mgr, err := session.NewManager(f.workDir, sessionID, passthroughRedactor{})
	if err != nil {
		return err //nolint:wrapcheck // test fake: the typed construction error
	}

	defer func() { _ = mgr.Close() }()

	lines, _ := mgr.ReadAll()

	closures, seed := session.Reconcile(sessionID, lines)

	for i := range closures {
		aerr := mgr.AppendSynthetic(&closures[i])
		if aerr != nil {
			return aerr //nolint:wrapcheck // test fake: the append error verbatim
		}
	}

	f.mu.Lock()
	f.seed[sessionID] = seed.MaxTurns
	f.modes[sessionID] = fakeModesShape(seed)
	f.mu.Unlock()

	return nil
}

// LoadedModes satisfies the acp ModeStateProvider optional capability the way
// runtime.Runner does: the seeded plan-mode state for the load response's
// modes field (nil = wire null).
func (f *fakeResumeRunner) LoadedModes(sessionID string) any {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.modes[sessionID]
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

// updateFrame is the test-side view of one session/update notification.
type updateFrame struct {
	Update struct {
		SessionUpdate string          `json:"sessionUpdate"` //nolint:tagliatelle // ACP wire field
		ToolCallID    string          `json:"toolCallId"`    //nolint:tagliatelle // ACP wire field
		Title         string          `json:"title,omitempty"`
		Status        string          `json:"status,omitempty"`
		MessageID     string          `json:"messageId,omitempty"` //nolint:tagliatelle // ACP wire field
		Content       json.RawMessage `json:"content,omitempty"`

		// AvailableCommands carries the available_commands_update payload
		// (18-05): the full v1 AvailableCommand set.
		AvailableCommands []struct {
			Name        string  `json:"name"`
			Description *string `json:"description"`
		} `json:"availableCommands,omitempty"` //nolint:tagliatelle // ACP wire field
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

// --- 18-04: session/list round-trip + capability advertisement battery ---

// sessionBackedStore adapts the REAL 18-03/18-04 session-package surfaces to
// the acp.SessionStore seam for this battery (the production adapter lives
// in internal/acpserve/session_store.go; this test-local twin exists because
// package-acp tests cannot import acpserve — acpserve imports acp). The
// round-trip therefore drives the real engine through the real injection
// path.
type sessionBackedStore struct{}

func (sessionBackedStore) ListSessions(
	dir, cursor string, limit int,
) ([]ListedSession, string, error) {
	headers, next, err := session.ListSessions(dir, cursor, limit)
	if err != nil {
		if errors.Is(err, session.ErrInvalidCursor) {
			return nil, "", fmt.Errorf("session/list %s: %w: %w", dir, ErrInvalidListCursor, err)
		}

		return nil, "", fmt.Errorf("session/list %s: %w", dir, err)
	}

	out := make([]ListedSession, len(headers))

	for i, h := range headers {
		out[i] = ListedSession{
			SessionID:    h.SessionID,
			Title:        h.Title,
			TitlePresent: h.TitlePresent,
			LastActivity: h.LastActivity,
		}
	}

	return out, next, nil
}

func (sessionBackedStore) Tombstone(dir, sessionID string) error {
	return session.Tombstone(dir, sessionID) //nolint:wrapcheck // thin test twin of the acpserve adapter
}

// Repeated 18-04 wire literals (goconst — the sibling tests' vocabulary).
const (
	methodSessionList   = "session/list"
	methodSessionClose  = "session/close"
	methodSessionDelete = "session/delete"
	keyCursor           = "cursor"
	keyNextCursor       = "nextCursor"
	keySessions         = "sessions"
	resumeMethod        = "session/resume"

	// listLiveSessions exceeds the engine's default page (50) so the RPC
	// round-trip drives a REAL page boundary: page one holds 50 rows, page
	// two the remaining 2.
	listLiveSessions = 52
	listPageSize     = 50
	listTombIndex    = 999
)

// listBaseTime pins fixture transcript mtimes — the 18-03 engine's
// lastActivity source — so engine order is deterministic (desc mtime).
func listBaseTime() time.Time {
	return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
}

// listFixtureID mints a sessIDPattern-valid UUID-form id whose fixed-width
// hex tail orders with i.
func listFixtureID(i int) string {
	return fmt.Sprintf("%08x-1111-4222-8333-%012x", i, i)
}

// writeListSession writes one conforming transcript (opener + optional first
// user prompt — empty prompt leaves the engine's fallback title) with a
// pinned mtime, returning the transcript path.
func writeListSession(t *testing.T, storeDir, sessionID, prompt string, mtime time.Time) string {
	t.Helper()

	lines := []string{
		`{"type":"session_start","timestamp":"` + fixtureTimestamp + `","text":"` + sessionID + `"}`,
	}

	if prompt != "" {
		lines = append(lines, `{"type":"user_message","turnID":"`+sessionID+`-turn-001","timestamp":"`+
			fixtureTimestamp+`","content":[{"type":"text","text":"`+prompt+`"}]}`)
	}

	writeLoadFixture(t, storeDir, sessionID, lines)

	path := filepath.Join(storeDir, ".ass-guard", "transcript_"+sessionID+".jsonl")

	err := os.Chtimes(path, mtime, mtime)
	if err != nil {
		t.Fatalf("chtimes transcript %s: %v", sessionID, err)
	}

	return path
}

// listRow is the test-side view of one v1 SessionInfo row.
type listRow struct {
	SessionID string  `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	Cwd       string  `json:"cwd"`
	Title     *string `json:"title"`
	UpdatedAt *string `json:"updatedAt"` //nolint:tagliatelle // ACP wire field
}

// listResult is the test-side view of ListSessionsResponse.
type listResult struct {
	NextCursor *string   `json:"nextCursor"` //nolint:tagliatelle // ACP wire field
	Sessions   []listRow `json:"sessions"`
}

// sendList sends one session/list request; cursor nil is the first page.
func sendList(t *testing.T, h *pipeHarness, reqID int, cursor *string) {
	t.Helper()

	h.send(t, newRequest(reqID, methodSessionList, map[string]any{
		keyCursor: cursor, keyCwd: testCwdTmp,
	}))
}

// decodeListResponse reads and decodes the session/list response for wantID.
func decodeListResponse(t *testing.T, msg *Message, wantID int) listResult {
	t.Helper()

	if msg.ID == nil || string(msg.ID) != strconv.Itoa(wantID) {
		t.Fatalf("list response id = %v; want %d", msg.ID, wantID)
	}

	if msg.Error != nil {
		t.Fatalf("session/list errored: %+v", msg.Error)
	}

	var res listResult

	uerr := json.Unmarshal(msg.Result, &res)
	if uerr != nil {
		t.Fatalf("unmarshal list result: %v (raw=%s)", uerr, string(msg.Result))
	}

	return res
}

// TestHandleSessionListRoundTrip drives session/list over a store of 52 live
// sessions plus one tombstoned: page one (null cursor) returns 50 rows in
// engine order (lastActivity desc) with the workDir as cwd, RFC3339
// updatedAt, title only where the header carries a real first prompt (the
// fallback stays server-side so the client renders its own placeholder), and
// NO tombstoned row; the page-one cursor drives page two without re-emission
// and exhausts with a null nextCursor.
//
//nolint:funlen,gocyclo,cyclop // one ordered wire-flow assertion end-to-end
func TestHandleSessionListRoundTrip(t *testing.T) {
	t.Parallel()

	store := t.TempDir()

	for i := range listLiveSessions {
		prompt := "prompt " + strconv.Itoa(i)

		if i == 1 {
			prompt = "" // no user_message: title stays null (fallback is server-side)
		}

		writeListSession(t, store, listFixtureID(i), prompt, listBaseTime().Add(-time.Duration(i)*time.Minute))
	}

	// The tombstoned session: NEWEST mtime of all — it must never appear on
	// any page (the 18-03 stat filter made visible through the RPC).
	tombID := listFixtureID(listTombIndex)

	writeListSession(t, store, tombID, "delete me", listBaseTime())

	err := os.WriteFile(filepath.Join(store, ".ass-guard", tombID+".deleted"), nil, 0o600)
	if err != nil {
		t.Fatalf("write tombstone: %v", err)
	}

	h := newPipeHarness(t, WithWorkDir(store), WithSessionStore(sessionBackedStore{}))

	// Page one: null cursor, 50 of the 52 live rows in engine order.
	sendList(t, h, 1, nil)

	page1 := decodeListResponse(t, h.readFrame(t), 1)

	if len(page1.Sessions) != listPageSize {
		t.Fatalf("page one rows = %d; want %d (default page)", len(page1.Sessions), listPageSize)
	}

	if page1.NextCursor == nil || *page1.NextCursor == "" {
		t.Fatalf("page one nextCursor = %v; want a continuation cursor", page1.NextCursor)
	}

	row0 := page1.Sessions[0]

	if row0.SessionID != listFixtureID(0) {
		t.Errorf("first row sessionId = %s; want %s (engine order: lastActivity desc)",
			row0.SessionID, listFixtureID(0))
	}

	if row0.Cwd != store {
		t.Errorf("row cwd = %q; want the server workDir %q (enumeration scope is the project)", row0.Cwd, store)
	}

	if row0.Title == nil || *row0.Title != "prompt 0" {
		t.Errorf("row 0 title = %v; want the first user prompt %q", row0.Title, "prompt 0")
	}

	if row0.UpdatedAt == nil {
		t.Fatal("row 0 updatedAt = nil; want the RFC3339 lastActivity")
	}

	gotAt, perr := time.Parse(time.RFC3339, *row0.UpdatedAt)
	if perr != nil {
		t.Fatalf("row 0 updatedAt %q is not RFC3339: %v", *row0.UpdatedAt, perr)
	}

	if !gotAt.Equal(listBaseTime()) {
		t.Errorf("row 0 updatedAt = %s; want the RFC3339 instant of lastActivity %s",
			*row0.UpdatedAt, listBaseTime().Format(time.RFC3339))
	}

	if row1 := page1.Sessions[1]; row1.Title != nil {
		t.Errorf("fallback-titled row carried title %q; want null (TitlePresent drives the mapping)", *row1.Title)
	}

	for i, row := range page1.Sessions {
		if row.SessionID == tombID {
			t.Errorf("tombstoned session listed at page-one index %d (D-07 stat filter violated)", i)
		}
	}

	// Page two: the cursor from page one — the remaining 2 rows, strictly
	// after the cursor tuple, no re-emission, null nextCursor.
	sendList(t, h, 2, page1.NextCursor)

	page2 := decodeListResponse(t, h.readFrame(t), 2)

	if len(page2.Sessions) != listLiveSessions-listPageSize {
		t.Fatalf("page two rows = %d; want %d", len(page2.Sessions), listLiveSessions-listPageSize)
	}

	if page2.NextCursor != nil {
		t.Errorf("exhausted page nextCursor = %v; want null", *page2.NextCursor)
	}

	want2 := []string{listFixtureID(listPageSize), listFixtureID(listPageSize + 1)}

	for i, want := range want2 {
		if page2.Sessions[i].SessionID != want {
			t.Errorf("page two row %d = %s; want %s (no re-emission, tuple order)",
				i, page2.Sessions[i].SessionID, want)
		}
	}

	page1IDs := make(map[string]struct{}, len(page1.Sessions))

	for _, row := range page1.Sessions {
		page1IDs[row.SessionID] = struct{}{}
	}

	for _, row := range page2.Sessions {
		if _, seen := page1IDs[row.SessionID]; seen {
			t.Errorf("page two re-emitted %s (cursor must never re-emit a row)", row.SessionID)
		}

		if row.SessionID == tombID {
			t.Error("tombstoned session listed on page two (D-07 stat filter violated)")
		}
	}
}

// TestHandleSessionListEmptyStore pins the empty-store wire shape: the
// sessions array serializes as [] (NEVER null) with a null nextCursor — the
// raw response bytes carry the empty-array token.
func TestHandleSessionListEmptyStore(t *testing.T) {
	t.Parallel()

	store := t.TempDir()

	err := os.MkdirAll(filepath.Join(store, ".ass-guard"), 0o750)
	if err != nil {
		t.Fatalf("mkdir store: %v", err)
	}

	h := newPipeHarness(t, WithWorkDir(store), WithSessionStore(sessionBackedStore{}))

	sendList(t, h, 0, nil)

	msg := h.readFrame(t)

	if msg.ID == nil || string(msg.ID) != "0" {
		t.Fatalf("list response id = %v; want 0", msg.ID)
	}

	if msg.Error != nil {
		t.Fatalf("session/list on empty store errored: %+v", msg.Error)
	}

	raw := string(msg.Result)

	if !strings.Contains(raw, `"`+keySessions+`":[]`) {
		t.Errorf("empty-store result = %s; want the sessions array to serialize as [] (never null)", raw)
	}

	if !strings.Contains(raw, `"`+keyNextCursor+`":null`) {
		t.Errorf("empty-store result = %s; want a null nextCursor", raw)
	}
}

// TestInitializeAdvertisesSessionCapabilities pins the v1 capability split
// (Pitfall 8): session/list|close|delete advertise under the NESTED
// sessionCapabilities object as exactly three empty entries, session/load
// stays under the TOP-LEVEL loadSession flag, and the v1 no-replay
// session/resume method appears NOWHERE (18-RESEARCH A3 — reconcile-then-
// accept makes full load the only safe resume path).
func TestInitializeAdvertisesSessionCapabilities(t *testing.T) {
	t.Parallel()

	h := newPipeHarness(t)

	h.send(t, newRequest(0, methodInitialize, zedLikeInitializeParams()))

	msg := h.readFrame(t)

	if msg.Error != nil {
		t.Fatalf("initialize errored: %+v", msg.Error)
	}

	var res struct {
		AgentCapabilities struct {
			LoadSession         bool           `json:"loadSession"`         //nolint:tagliatelle // ACP wire field
			SessionCapabilities map[string]any `json:"sessionCapabilities"` //nolint:tagliatelle // ACP wire field
		} `json:"agentCapabilities"` //nolint:tagliatelle // ACP wire field
	}

	uerr := json.Unmarshal(msg.Result, &res)
	if uerr != nil {
		t.Fatalf("unmarshal initialize result: %v (raw=%s)", uerr, string(msg.Result))
	}

	if !res.AgentCapabilities.LoadSession {
		t.Error("agentCapabilities.loadSession = false; want true (18-01 top-level flag)")
	}

	caps := res.AgentCapabilities.SessionCapabilities

	want := map[string]struct{}{"list": {}, "close": {}, "delete": {}}

	if len(caps) != len(want) {
		t.Errorf("sessionCapabilities = %v; want exactly the keys %v", caps, want)
	}

	for k, v := range caps {
		if _, ok := want[k]; !ok {
			t.Errorf("sessionCapabilities carries unexpected entry %q", k)

			continue
		}

		if obj, isObj := v.(map[string]any); !isObj || len(obj) != 0 {
			t.Errorf("sessionCapabilities[%q] = %v; want an empty object", k, v)
		}
	}

	if strings.Contains(string(msg.Result), resumeMethod) {
		t.Errorf("initialize result mentions %s; the no-replay resume method is NOT advertised (A3)", resumeMethod)
	}

	if _, registered := h.srv.handlers[resumeMethod]; registered {
		t.Errorf("%s registered in the handler map (A3)", resumeMethod)
	}
}

// --- 18-04: session/close cancel-and-drain + session/delete tombstone ---

// blockingCloseRunner is the 18-04 close-battery TurnRunner: Run parks on
// ctx.Done (the in-flight turn the close must cancel-and-drain), and the
// runner records the teardown seams — CloseSession (SessionCloser) and the
// ask drains (AskDrainer) — plus a stand-in parked-ask queue, so the test
// asserts the D-12 ordering: close returns only after Run returned AND the
// session's asks drained.
//
// Inherited-coverage note: the queue-level cancellation semantics (the open
// dialog resolving cancelled through the registry cascade) are pinned at the
// 17 seams — internal/session/askqueue_test.go (TestAskQueueDrainAll) and
// internal/acpserve/ask_drain_test.go; THIS runner pins the drain CONTRACT
// the close path calls (the AskDrainer seam fires before close returns).
type blockingCloseRunner struct {
	startOnce sync.Once
	doneOnce  sync.Once
	started   chan struct{}
	finished  chan struct{}

	mu     sync.Mutex
	asks   map[string]int
	drains []string
	closes []string
}

func newBlockingCloseRunner() *blockingCloseRunner {
	return &blockingCloseRunner{
		started:  make(chan struct{}),
		finished: make(chan struct{}),
		asks:     map[string]int{},
	}
}

// Run parks until the turn ctx is cancelled, then reports the D-16
// stopReason for a cancelled turn.
func (r *blockingCloseRunner) Run(
	ctx context.Context, _ string, _ ChunkEmitter, _ []ContentBlock,
) (string, error) {
	r.startOnce.Do(func() { close(r.started) })

	<-ctx.Done() // park until the close path cancels the turn

	r.doneOnce.Do(func() { close(r.finished) })

	return stopCancelled, nil
}

// CloseSession satisfies SessionCloser (the reap seam the close path rides).
func (r *blockingCloseRunner) CloseSession(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closes = append(r.closes, sessionID)

	return nil
}

// DrainAsks satisfies AskDrainer's turn-scoped half (session/cancel).
func (r *blockingCloseRunner) DrainAsks(sessionID string) { r.drainSession(sessionID) }

// DrainSessionAsks satisfies AskDrainer's session-close half — the seam
// session/close calls: parked asks resolve cancelled, the queue empties.
func (r *blockingCloseRunner) DrainSessionAsks(sessionID string) { r.drainSession(sessionID) }

func (r *blockingCloseRunner) drainSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.drains = append(r.drains, sessionID)
	delete(r.asks, sessionID)
}

func (r *blockingCloseRunner) parkAsk(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.asks[sessionID]++
}

func (r *blockingCloseRunner) pendingAsks(sessionID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.asks[sessionID]
}

func (r *blockingCloseRunner) sessionDrains() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.drains...)
}

func (r *blockingCloseRunner) closeCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.closes...)
}

// TestSessionCloseCancelsAndDrains pins D-12: session/close on a session with
// an in-flight turn (stub parked in Run) and ONE parked ask returns only
// after the turn's Run has returned AND the ask drained (17-D-13 aimed at the
// whole session), reaps through the SessionCloser seam, drops the map entry,
// answers the aborted turn stopReason "cancelled", and a second close of the
// already-closed id is the empty-object success.
//
//nolint:funlen,gocyclo,cyclop // one deterministic drain-ordering scenario end-to-end
func TestSessionCloseCancelsAndDrains(t *testing.T) {
	t.Parallel()

	runner := newBlockingCloseRunner()
	h := newPipeHarness(t, WithTurnRunner(runner))

	sid := handshakeRecordingSession(t, h)

	runner.parkAsk(sid)

	h.send(t, newRequest(2, "session/prompt", promptRequest(sid, "long turn")))

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("stub turn never started")
	}

	h.send(t, newRequest(3, methodSessionClose, map[string]any{keySessionID: sid}))

	// The close response and the aborted prompt response may arrive in EITHER
	// order (close's drain waits for Run's RETURN, not the prompt handler's
	// response write) — collect by id instead of racing the two reads.
	var closeResp, promptResp *Message

	for closeResp == nil || promptResp == nil {
		frame := h.readFrame(t)

		if frame.ID == nil {
			continue // notifications (none expected here)
		}

		switch string(frame.ID) {
		case "3":
			closeResp = frame
		case "2":
			promptResp = frame
		default:
			t.Fatalf("unexpected response id %s while collecting close+prompt responses", string(frame.ID))
		}
	}

	if closeResp.Error != nil {
		t.Fatalf("session/close errored: %+v", closeResp.Error)
	}

	// The drain wait is observable: close returned strictly AFTER Run.
	select {
	case <-runner.finished:
	default:
		t.Error("session/close returned before the in-flight turn's Run returned (D-12 drain violated)")
	}

	if got := runner.pendingAsks(sid); got != 0 {
		t.Errorf("parked ask survived close: %d pending (the drain resolves it cancelled)", got)
	}

	if drains := runner.sessionDrains(); len(drains) != 1 || drains[0] != sid {
		t.Errorf("close-path ask drains = %v; want exactly [%s]", drains, sid)
	}

	if closes := runner.closeCalls(); len(closes) != 1 || closes[0] != sid {
		t.Errorf("CloseSession calls = %v; want exactly [%s]", closes, sid)
	}

	h.srv.mu.Lock()
	_, alive := h.srv.sessions[sid]
	h.srv.mu.Unlock()

	if alive {
		t.Error("closed session still registered in the map")
	}

	if promptResp.Error != nil {
		t.Fatalf("aborted turn errored: %+v (D-16 wants stopReason cancelled)", promptResp.Error)
	}

	var pr struct {
		StopReason string `json:"stopReason"` //nolint:tagliatelle // ACP wire field
	}

	uerr := json.Unmarshal(promptResp.Result, &pr)
	if uerr != nil {
		t.Fatalf("decode prompt result: %v (raw=%s)", uerr, string(promptResp.Result))
	}

	if pr.StopReason != stopCancelled {
		t.Errorf("aborted turn stopReason = %q; want %q", pr.StopReason, stopCancelled)
	}

	h.send(t, newRequest(4, methodSessionClose, map[string]any{keySessionID: sid}))

	second := readUntilResponse(t, h, 4, nil)
	if second.Error != nil {
		t.Fatalf("second close errored: %+v (D-12 idempotency)", second.Error)
	}

	if got := strings.TrimSpace(string(second.Result)); got != "{}" {
		t.Errorf("close result = %s; want the empty object {}", got)
	}
}

// TestSessionCloseAlreadyClosed: close of an unknown/never-created id is the
// empty-object success — never an error (D-12 idempotency).
func TestSessionCloseAlreadyClosed(t *testing.T) {
	t.Parallel()

	h := newPipeHarness(t)

	h.send(t, newRequest(0, methodSessionClose,
		map[string]any{keySessionID: "00000000-0000-4000-8000-00000000000f"}))

	msg := h.readFrame(t)

	if msg.ID == nil || string(msg.ID) != "0" {
		t.Fatalf("close response id = %v; want 0", msg.ID)
	}

	if msg.Error != nil {
		t.Fatalf("close of unknown id errored: %+v (want the idempotent success)", msg.Error)
	}

	if got := strings.TrimSpace(string(msg.Result)); got != "{}" {
		t.Errorf("close result = %s; want the empty object {}", got)
	}
}

// TestSessionDeleteTombstones pins the delete contract (D-07/D-08/D-20):
// deleting an OPEN session closes it, writes the zero-byte 0600 marker
// beside the transcript, removes the session's checkpoint objects through
// the store surface, leaves the transcript bytes UNCHANGED and the audit
// artifacts present, drops the session from the list, and a second delete of
// the same id is the idempotent success (marker rewrite).
//
//nolint:funlen,gocyclo,cyclop // one ordered delete-flow assertion end-to-end
func TestSessionDeleteTombstones(t *testing.T) {
	t.Parallel()

	store := t.TempDir()
	sid := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	writeLoadFixture(t, store, sid, cleanSessionFixtureLines(sid))

	transcript := filepath.Join(store, ".ass-guard", "transcript_"+sid+".jsonl")

	before, rerr := os.ReadFile(transcript)
	if rerr != nil {
		t.Fatalf("read transcript before delete: %v", rerr)
	}

	auditDir := filepath.Join(store, ".ass-guard", "audit")

	err := os.MkdirAll(auditDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir audit dir: %v", err)
	}

	auditFile := filepath.Join(auditDir, "mirror.jsonl")

	err = os.WriteFile(auditFile, []byte("{\"v\":1}\n"), 0o600)
	if err != nil {
		t.Fatalf("write audit artifact: %v", err)
	}

	ckpt, oerr := checkpoint.Open(store)
	if oerr != nil {
		t.Fatalf("checkpoint open: %v", oerr)
	}

	serr := ckpt.Snapshot(context.Background(), sid, sid+"-turn-001")
	if serr != nil {
		t.Fatalf("checkpoint snapshot: %v", serr)
	}

	h := newPipeHarness(t,
		WithWorkDir(store),
		WithTurnRunner(newFakeResumeRunner(store)),
		WithSessionStore(sessionBackedStore{}),
		WithCheckpointStore(ckpt))

	sendLoad(t, h, 1, sid, store)

	if loadResp := readUntilResponse(t, h, 1, nil); loadResp.Error != nil {
		t.Fatalf("session/load errored: %+v", loadResp.Error)
	}

	h.send(t, newRequest(2, methodSessionDelete, map[string]any{keySessionID: sid}))

	delResp := readUntilResponse(t, h, 2, nil)
	if delResp.Error != nil {
		t.Fatalf("session/delete errored: %+v", delResp.Error)
	}

	marker := filepath.Join(store, ".ass-guard", sid+".deleted")

	mfi, serr := os.Stat(marker)
	if serr != nil {
		t.Fatalf("tombstone marker missing after delete: %v", serr)
	}

	if mfi.Size() != 0 {
		t.Errorf("marker size = %d; want the zero-byte tombstone (D-07)", mfi.Size())
	}

	if mfi.Mode().Perm() != 0o600 {
		t.Errorf("marker perms = %o; want 0600", mfi.Mode().Perm())
	}

	after, rerr := os.ReadFile(transcript)
	if rerr != nil {
		t.Fatalf("read transcript after delete: %v", rerr)
	}

	if !bytes.Equal(before, after) {
		t.Error("delete mutated the transcript bytes (D-07: tombstone, never rewrite)")
	}

	_, aerr := os.Stat(auditFile)
	if aerr != nil {
		t.Errorf("audit artifact removed by delete: %v (D-08/D-20 — audit survives unconditionally)", aerr)
	}

	entries, lerr := ckpt.List()
	if lerr != nil {
		t.Fatalf("checkpoint list after delete: %v", lerr)
	}

	for _, e := range entries {
		if e.SessionID == sid {
			t.Errorf("checkpoint %s survived delete", e.Ref)
		}
	}

	h.srv.mu.Lock()
	_, alive := h.srv.sessions[sid]
	h.srv.mu.Unlock()

	if alive {
		t.Error("deleted session still registered in the map")
	}

	sendList(t, h, 3, nil)

	for _, row := range decodeListResponse(t, h.readFrame(t), 3).Sessions {
		if row.SessionID == sid {
			t.Error("tombstoned session still listed after delete")
		}
	}

	h.send(t, newRequest(4, methodSessionDelete, map[string]any{keySessionID: sid}))

	if second := readUntilResponse(t, h, 4, nil); second.Error != nil {
		t.Fatalf("second delete errored: %+v (idempotent)", second.Error)
	}

	mfi2, serr2 := os.Stat(marker)
	if serr2 != nil || mfi2.Size() != 0 {
		t.Errorf("marker after second delete = (%v, %d bytes); want present and zero-byte", serr2, mfi2.Size())
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
			sawTool = u.Update.ToolCallID == fixtureToolCallID && u.Update.Title == fixtureToolBash
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
	// 18-05: the available_commands_update re-advertisement closes the replay
	// (ACP-06 "commands re-advertised") — after the last replayed frame,
	// before the response.
	wantKinds := []string{
		updKindAgentMessageChunk, updKindAgentMessageChunk, updKindAgentMessageChunk,
		updKindToolCall, updKindToolCallUpdate, "available_commands_update",
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
//
//nolint:funlen // one deterministic concurrency scenario end-to-end (the gate pin)
func TestLoadGateRejectsPromptDuringReplay(t *testing.T) {
	t.Parallel()

	store := t.TempDir()
	sid := "44444444-5555-6666-8777-888888888888"

	// 400 chunk lines: more than the writer buffer (256) + lane capacity can
	// absorb, so replay deterministically parks mid-stream until the client
	// starts draining.
	const chunkLines = 400

	lines := make([]string, 0, chunkLines+2)
	lines = append(lines, `{"type":"session_start","timestamp":"`+fixtureTimestamp+`","text":"`+sid+`"}`)

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
