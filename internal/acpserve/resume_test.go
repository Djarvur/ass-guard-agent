package acpserve //nolint:testpackage // internal package test

// 18-06 Task 3 tests: Options.ResumeTarget — the CLI resume surface's
// serve-side injection through the ONE load engine session/load uses
// (Server.LoadSession, 18-01/18-05). The load happens BEFORE Serve begins
// (frame ordering on the client pipe proves it — replayed frames arrive with
// NO client input), a follow-up prompt continues the turn sequence, an empty
// target is a no-op, and a failing target is a LOUD Run error naming it
// (never a silently fresh serve).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
)

// resumeFixtureID is the fixture transcript's session id (pattern-valid UUID).
const resumeFixtureID = "f0ee1e11-aaaa-4aaa-8aaa-aabbccddeeff"

// writeResumeFixtureTranscript seeds workDir/.ass-guard/ with a CLEAN fixture
// transcript (no dangling expectations — 18-02's reconciliation appends
// nothing): session_start, one complete user/assistant turn-001, session_end.
func writeResumeFixtureTranscript(t *testing.T, workDir string) {
	t.Helper()

	store := filepath.Join(workDir, ".ass-guard")

	err := os.MkdirAll(store, 0o750)
	if err != nil {
		t.Fatalf("mkdir store: %v", err)
	}

	lines := []string{
		`{"type":"session_start","timestamp":"2026-09-03T10:00:00Z"}`,
		`{"type":"user_message","turnID":"` + resumeFixtureID + `-turn-001","timestamp":"2026-09-03T10:00:01Z",` +
			`"content":[{"type":"text","text":"the fixture prompt"}]}`,
		`{"type":"assistant_message","turnID":"` + resumeFixtureID + `-turn-001",` +
			`"timestamp":"2026-09-03T10:00:02Z","text":"the fixture reply"}`,
		`{"type":"session_end","timestamp":"2026-09-03T10:00:03Z"}`,
	}

	werr := os.WriteFile(filepath.Join(store, "transcript_"+resumeFixtureID+".jsonl"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o600)
	if werr != nil {
		t.Fatalf("write fixture transcript: %v", werr)
	}
}

// resumeChunk is one decoded agent_message_chunk session/update.
type resumeChunk struct {
	SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	Update    struct {
		Kind    string `json:"sessionUpdate"` //nolint:tagliatelle // ACP wire field
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"update"`
}

// awaitResumeChunk reads frames until the chunk carrying text arrives (the
// replayed assistant_message), failing on the guard timeout otherwise.
func awaitResumeChunk(t *testing.T, cli *simClient, sessionID, text string) {
	t.Helper()

	deadline := time.After(simGuardTimeout)

	for {
		select {
		case m, ok := <-cli.frames:
			if !ok {
				t.Fatalf("stdout closed before the replayed chunk %q", text)
			}

			if m.Method != simMethodSessionUpdate {
				continue
			}

			var c resumeChunk

			if json.Unmarshal(m.Params, &c) != nil {
				continue
			}

			if c.SessionID == sessionID && c.Update.Kind == simKindChunk && c.Update.Content.Text == text {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for the replayed chunk %q", text)
		}
	}
}

// transcriptHasTurn polls the transcript FILE until a line carrying turnID
// exists (the kill-9 harness's file-poll discipline — never a blind sleep).
func transcriptHasTurn(t *testing.T, workDir, sessionID, turnID string) {
	t.Helper()

	path := filepath.Join(workDir, ".ass-guard", "transcript_"+sessionID+".jsonl")

	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		raw, rerr := os.ReadFile(path)
		if rerr == nil && strings.Contains(string(raw), `"turnID":"`+turnID+`"`) {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("transcript never gained a %s line within 15s", turnID)
}

// TestResumeTargetLoadsBeforeServe (18-06, ACP-06): with ResumeTarget set
// against a clean fixture transcript, the serve composition loads the session
// through Server.LoadSession BEFORE Serve begins — the replayed frames hit
// the client pipe with ZERO client input (nothing else could have produced
// them) — and a follow-up prompt continues the turn sequence (the next turn
// id continues from the transcript maxima, not a fresh 001).
func TestResumeTargetLoadsBeforeServe(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "") // the config literal key wins (deterministic provider construction)

	stub := newSimStub([]simTurnScript{{phases: []simPhase{{text: "resumed turn complete."}}}})
	t.Cleanup(stub.srv.Close)

	workDir := simulatorWorkDir(t, stub.srv.URL)
	writeResumeFixtureTranscript(t, workDir)

	srvInR, srvInW := io.Pipe()
	cliR, cliOutW := io.Pipe()
	stderr := &syncBuffer{}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = Run(ctx, srvInR, cliOutW, stderr, &Options{
			Profile: profileZcode, MaxConcurrent: 2,
			ProfilesDir: repoProfilesDir(t), WorkDir: workDir,
			ResumeTarget: resumeFixtureID,
		})
	}()

	t.Cleanup(func() {
		cancel()

		_ = srvInW.Close()
		_ = cliOutW.Close()
		_ = srvInR.Close()
		_ = cliR.Close()
	})

	cli := newSimClient(t, cliR, srvInW)

	// NO frame has been sent: the replayed assistant_message arriving on the
	// pipe proves the load ran BEFORE Serve read anything.
	awaitResumeChunk(t, cli, resumeFixtureID, "the fixture reply")

	// The handshake still works after the pre-Serve load, and the loaded
	// session accepts a follow-up prompt that completes a full turn.
	cli.sendf(`{"jsonrpc":"2.0","id":"r1","method":"initialize","params":{"protocolVersion":1,` +
		`"clientCapabilities":{"elicitation":{"form":{}}}}}`)

	m := cli.nextResponse(`"r1"`)

	var initResp struct {
		ProtocolVersion int `json:"protocolVersion"` //nolint:tagliatelle // ACP wire field
	}

	if json.Unmarshal(m.Result, &initResp) != nil || initResp.ProtocolVersion != 1 {
		t.Fatalf("post-load initialize response malformed: %s", string(m.Result))
	}

	cli.sendf(`{"jsonrpc":"2.0","id":"r2","method":"session/prompt","params":{"sessionId":` +
		simJSONStr(resumeFixtureID) + `,"prompt":[{"type":"text","text":"continue the session"}]}}`)

	resp := cli.nextResponse(`"r2"`)
	simAssertStopReason(t, resp.Result, `"r2"`, simStopEndTurn)

	// Turn-sequence continuation: the follow-up turn appended turn-002 (the
	// fixture's max was turn-001).
	transcriptHasTurn(t, workDir, resumeFixtureID, resumeFixtureID+"-turn-002")
}

// TestResumeTargetAbsentIsNoop: an empty ResumeTarget performs no load — the
// serve starts normally (initialize answers) and ZERO replay-side frames
// appear on the pipe.
func TestResumeTargetAbsentIsNoop(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	srvInR, srvInW := io.Pipe()
	cliR, cliOutW := io.Pipe()
	stderr := &syncBuffer{}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = Run(ctx, srvInR, cliOutW, stderr, &Options{
			Profile: profileZcode, MaxConcurrent: 2,
			ProfilesDir: repoProfilesDir(t), WorkDir: workDir,
		})
	}()

	t.Cleanup(func() {
		cancel()

		_ = srvInW.Close()
		_ = cliOutW.Close()
		_ = srvInR.Close()
		_ = cliR.Close()
	})

	cli := newSimClient(t, cliR, srvInW)

	cli.sendf(`{"jsonrpc":"2.0","id":"n1","method":"initialize","params":{"protocolVersion":1,` +
		`"clientCapabilities":{"elicitation":{"form":{}}}}}`)

	m := cli.nextResponse(`"n1"`)
	if !strings.Contains(string(m.Result), "agentCapabilities") {
		t.Fatalf("initialize response malformed: %s", string(m.Result))
	}

	// Bounded quiet window: no replay-side session/update ever fires.
	deadline := time.Now().Add(250 * time.Millisecond)

	for time.Now().Before(deadline) {
		select {
		case m, ok := <-cli.frames:
			if !ok {
				return
			}

			if m.Method == simMethodSessionUpdate {
				t.Fatalf("session/update fired without a load or a turn: %s", string(m.Params))
			}
		case <-time.After(time.Until(deadline)):
		}
	}
}

// TestResumeTargetFailureIsLoud: a ResumeTarget with no transcript behind it
// makes Run return an error NAMING the target (the CLI user asked for THAT
// session) — never a silently fresh serve.
func TestResumeTargetFailureIsLoud(t *testing.T) {
	t.Parallel()

	const missing = "99999999-9999-4999-8999-999999999999"

	var stdout, stderr syncBuffer

	err := Run(t.Context(), strings.NewReader(""), &stdout, &stderr, &Options{
		Profile: profileZcode, MaxConcurrent: 2,
		ProfilesDir: repoProfilesDir(t), WorkDir: t.TempDir(),
		ResumeTarget: missing,
	})
	if err == nil {
		t.Fatal("Run returned nil for a missing resume target; want the loud load failure")
	}

	if !strings.Contains(err.Error(), missing) {
		t.Errorf("Run error %q does not name the target %s", err.Error(), missing)
	}

	if !errors.Is(err, io.EOF) && stdout.String() != "" {
		t.Errorf("stdout received %q before the loud failure; want NO frames (serve never started)",
			stdout.String())
	}

	// The typed unknown-session rejection rides inside (the load engine's
	// error, not a generic one).
	if _, ok := errors.AsType[*acp.RPCError](err); !ok {
		t.Errorf("Run error = %T (%v); want the load engine's *acp.RPCError wrapped", err, err)
	}
}
