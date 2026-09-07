package acp //nolint:testpackage // internal package test (drives srv.sessions directly)

// 16-REVIEW CR-01 regression pin: session/cancel is TURN-scoped (ACP v1). It
// must cancel the active turn and nothing else — the session stays registered
// and fully usable for the next prompt on the SAME sessionId (the standard
// editor flow: escape, then ask again). Resource reaping (MCP host, transcript
// writer, session forwarder, SessionEnd hook) belongs to the real session-end
// paths only: logout and serve teardown.

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// Repeated wire literals (goconst — matches the sibling tests' vocabulary).
const (
	testKeyPrompt   = "prompt"
	testKeyTxtBlock = "type"
)

// closerRecordingRunner is a TurnRunner that also implements SessionCloser and
// records every prompt and every CloseSession call, so the test can observe
// which wire methods reap session-scoped resources.
type closerRecordingRunner struct {
	mu      sync.Mutex
	prompts int
	closes  []string
}

func (r *closerRecordingRunner) Run(
	ctx context.Context, _ string, emit ChunkEmitter, _ []ContentBlock,
) (string, error) {
	r.mu.Lock()
	r.prompts++
	r.mu.Unlock()

	_ = emit.AgentMessageChunk("m1", "x")

	select {
	case <-ctx.Done():
		return stopCancelled, nil
	default:
	}

	return stopEndTurn, nil
}

// CloseSession satisfies acp.SessionCloser (the reap seam logout routes to).
func (r *closerRecordingRunner) CloseSession(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closes = append(r.closes, sessionID)

	return nil
}

func (r *closerRecordingRunner) promptCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.prompts
}

func (r *closerRecordingRunner) closeCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.closes...)
}

// awaitResponseID reads frames (skipping notifications) until the response with
// the given id arrives.
func awaitResponseID(t *testing.T, h *pipeHarness, id string) *Message {
	t.Helper()

	h.cliMu.Lock()
	defer h.cliMu.Unlock()

	for range 16 {
		msg, err := readFrame(h.cbr)
		if err != nil {
			t.Fatalf("readFrame waiting for response %s: %v", id, err)
		}

		if msg.ID != nil && string(msg.ID) == id {
			return msg
		}
	}

	t.Fatalf("response id %s never arrived", id)

	return nil
}

// handshakeRecordingSession runs the initialize + session/new handshake against
// the harness wired with runner and returns the live session id.
func handshakeRecordingSession(t *testing.T, h *pipeHarness) string {
	t.Helper()

	h.send(t, newRequest(0, methodInitialize, zedLikeInitializeParams()))
	h.readFrame(t)

	h.send(t, newRequest(1, "session/new", map[string]any{keyCwd: testCwdTmp, keyMcpServers: []any{}}))
	snew := h.readResultFrame(t)

	var sres struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	uerr := json.Unmarshal(snew.Result, &sres)
	if uerr != nil {
		t.Fatalf("decode session/new result: %v", uerr)
	}

	return sres.SessionID
}

// promptRequest builds a session/prompt request payload for the session.
func promptRequest(sessionID, text string) map[string]any {
	return map[string]any{
		keySessionID:  sessionID,
		testKeyPrompt: []map[string]any{{testKeyTxtBlock: blockText, blockText: text}},
	}
}

// TestSessionCancelDoesNotReapTheSession pins the CR-01 contract: cancel leaves
// the session alive (no CloseSession, still in the server map, re-promptable);
// logout is the path that reaps.
func TestSessionCancelDoesNotReapTheSession(t *testing.T) {
	t.Parallel()

	runner := &closerRecordingRunner{}
	h := newPipeHarness(t, WithTurnRunner(runner))
	sid := handshakeRecordingSession(t, h)

	// Turn 1 completes normally.
	h.send(t, newRequest(2, "session/prompt", promptRequest(sid, "hi")))
	awaitResponseID(t, h, "2")

	// The editor flow: the user presses escape (session/cancel — a
	// notification, no live turn to abort after turn 1 finished).
	h.send(t, newNotification("session/cancel", map[string]any{keySessionID: sid}))

	// The cancel dispatches inline in the reader goroutine; give it a beat,
	// then assert the session was NOT reaped.
	time.Sleep(100 * time.Millisecond)

	if closes := runner.closeCalls(); len(closes) != 0 {
		t.Errorf("session/cancel reaped the session: CloseSession called %d time(s) (%v) — cancel is turn-scoped",
			len(closes), closes)
	}

	h.srv.mu.Lock()
	_, alive := h.srv.sessions[sid]
	h.srv.mu.Unlock()

	if !alive {
		t.Fatal("session/cancel removed the session from the map; " +
			"the client must be able to re-prompt the same id")
	}

	// Re-prompt the SAME id: the half-reaped state (CR-01) would make this turn
	// run without MCP tools or audit streaming; the pin is that the turn runs
	// at all and the session was never closed behind it.
	h.send(t, newRequest(3, "session/prompt", promptRequest(sid, "again")))
	awaitResponseID(t, h, "3")

	if got := runner.promptCount(); got != 2 {
		t.Errorf("prompt count after cancel + re-prompt = %d; want 2 (the session stayed promptable)", got)
	}

	// logout is the REAL session end: it must reap exactly once.
	h.send(t, newRequest(4, "logout", map[string]any{keySessionID: sid}))
	awaitResponseID(t, h, "4")

	closes := runner.closeCalls()
	if len(closes) != 1 || closes[0] != sid {
		t.Errorf("logout CloseSession calls = %v; want exactly [%s]", closes, sid)
	}
}

// TestPromptRegistrationAbortsAfterCloseReaped pins the WR-02 half of the
// D-12 linearizability: the prompt's gate pass and its turnWG registration
// are two steps, so a session/close running ENTIRELY between them (the gate
// already returned st; close's drain consumed a zero WaitGroup; the map entry
// is gone) must convert the late registration into the typed error — the Add
// would otherwise land on a dead sessionState and Run would execute against
// reaped resources (MCP host closed, writer cancelled, forwarder stopped).
// Sequenced deterministically at the method level: the wire window is
// microseconds wide, but the interleaving is exactly gate → close → Add.
//
//nolint:funlen // one ordered interleaving, asserted end-to-end
func TestPromptRegistrationAbortsAfterCloseReaped(t *testing.T) {
	t.Parallel()

	runner := &closerRecordingRunner{}
	h := newPipeHarness(t, WithTurnRunner(runner))
	sid := handshakeRecordingSession(t, h)

	// The gate passes — the session is live and ready.
	st, gerr := h.srv.promptSessionState(sid)
	if gerr != nil {
		t.Fatalf("promptSessionState: %v", gerr)
	}

	// The racing close runs to completion between the gate and the
	// registration: drain, cancelTurn (no cancel func yet), zero-WG drain,
	// reap, map delete.
	h.srv.closeSessionSequence(sid)

	h.srv.mu.Lock()
	_, alive := h.srv.sessions[sid]
	h.srv.mu.Unlock()

	if alive {
		t.Fatal("closeSessionSequence left the session in the map; sequencing broken")
	}

	rerr := h.srv.registerTurn(sid, st)
	if rerr == nil {
		t.Fatal("registerTurn accepted a session the close already reaped (WR-02)")
	}

	var rpcErr *RPCError

	if !errors.As(rerr, &rpcErr) {
		t.Fatalf("registerTurn error = %v; want the typed RPCError", rerr)
	}

	if rpcErr.Code != CodeInvalidRequest {
		t.Errorf("registerTurn code = %d; want %d", rpcErr.Code, CodeInvalidRequest)
	}

	// The aborted registration leaves no ghost turn: the drain returns
	// immediately instead of waiting out its timeout on a leaked Add.
	if !st.waitTurnDrain(50 * time.Millisecond) {
		t.Error("aborted registration leaked a turnWG count; a later close would drain a ghost turn")
	}

	// And the runner never saw a prompt for the reaped session.
	if got := runner.promptCount(); got != 0 {
		t.Errorf("runner ran %d turn(s) against the reaped session; want 0", got)
	}
}
