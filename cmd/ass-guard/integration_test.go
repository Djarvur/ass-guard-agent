package main

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

// mockStreamProvider is a Provider whose Stream emits canned text chunks then a
// done chunk. Used to drive the real Session Core through the ACP server without
// a live model call (autonomous: true).
type mockStreamProvider struct {
	chunks []string
	finish string
}

func (m *mockStreamProvider) Send(ctx context.Context, _ profile.Profile, _ []provider.Message) (provider.Response, error) {
	return provider.Response{FinishReason: m.finish}, nil
}

func (m *mockStreamProvider) Stream(ctx context.Context, _ profile.Profile, _ []provider.Message) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 8)
	go func() {
		defer close(ch)

		for _, c := range m.chunks {
			select {
			case ch <- provider.StreamChunk{Type: "text", Text: c}:
			case <-ctx.Done():
				return
			}
		}

		select {
		case ch <- provider.StreamChunk{Type: "done", FinishReason: m.finish}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (m *mockStreamProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// driveACP starts runACPServe with a mock provider and returns the client pipe
// ends. The test writes client frames to cliW and reads from cliR.
func driveACP(t *testing.T, mp provider.Provider) (cliW *io.PipeWriter, cliR io.Reader, stop func()) {
	t.Helper()

	bus := event.NewBus()
	prof := profile.Profile{Name: "test", System: []profile.TextBlock{{Type: "text", Text: "test agent"}}}
	runner := &sessionTurnRunner{
		bus:          bus,
		profile:      prof,
		workDir:      t.TempDir(),
		maxConc:      2,
		makeProvider: func() provider.Provider { return mp },
	}
	srvInR, cliW := io.Pipe()
	cliR, srvOutW := io.Pipe()
	srv := acp.NewServer(srvInR, srvOutW, &bytes.Buffer{}, acp.WithTurnRunner(runner))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { _ = srv.Serve(ctx); close(done) }()

	stop = func() {
		cancel()

		_ = cliW.Close()
		_ = srvOutW.Close()
		_ = srvInR.Close()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}

	return cliW, cliR, stop
}

// sendFrame writes one ACP frame to w.
func sendFrame(t *testing.T, w io.Writer, m acp.Message) {
	t.Helper()

	var buf bytes.Buffer

	err := writeFrameDirect(&buf, m)
	if err != nil {
		t.Fatalf("writeFrame: %v", err)
	}

	_, _ = w.Write(buf.Bytes())
}

// writeFrameDirect mirrors acp.writeFrame (unexported); defined here to drive the
// server from the test without exporting internals.
func writeFrameDirect(buf *bytes.Buffer, m acp.Message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}

	buf.Write(raw)
	buf.WriteByte('\n')

	return nil
}

// readFrames reads up to n frames from cliR, returning them.
func readFrames(t *testing.T, cliR io.Reader, n int) []*acp.Message {
	t.Helper()

	br := bufio.NewReader(cliR)

	var out []*acp.Message

	deadline := time.After(3 * time.Second)

	for len(out) < n {
		select {
		default:
		case <-deadline:
			return out
		}

		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			return out
		}

		line = bytes.TrimRight(line, "\n")
		if len(line) == 0 {
			continue
		}

		var m acp.Message

		jerr := json.Unmarshal(line, &m)
		if jerr == nil {
			out = append(out, &m)
		}
	}

	return out
}

// TestIntegration_RealStreamingThroughACP verifies the real Session Core streams
// token-by-token through ACP: initialize → session/new → session/prompt emits
// agent_message_chunk session/update notifications (one per chunk) before the
// stopReason response. NO full-turn buffering (ACP-04).
func TestIntegration_RealStreamingThroughACP(t *testing.T) {
	t.Parallel()

	mp := &mockStreamProvider{chunks: []string{"Hello", " ", "world"}, finish: "end_turn"}

	cliW, cliR, stop := driveACP(t, mp)
	defer stop()

	sendFrame(t, cliW, acp.Message{JSONRPC: "2.0", ID: intPtrACP(0), Method: "initialize", Params: rawJSON(map[string]any{"protocolVersion": 1})})

	frames := readFrames(t, cliR, 1)
	if len(frames) == 0 || !strings.Contains(string(frames[0].Result), "agentCapabilities") {
		t.Fatalf("no initialize response with agentCapabilities: %+v", frames)
	}

	sendFrame(t, cliW, acp.Message{JSONRPC: "2.0", ID: intPtrACP(1), Method: "session/new", Params: rawJSON(map[string]any{"cwd": "/tmp", "mcpServers": []any{}})})
	frames = readFrames(t, cliR, 1)

	var snew struct {
		SessionID string `json:"sessionId"`
	}

	_ = json.Unmarshal(frames[0].Result, &snew)

	if snew.SessionID == "" {
		t.Fatalf("no sessionId in session/new response: %+v", frames)
	}

	sendFrame(t, cliW, acp.Message{JSONRPC: "2.0", ID: intPtrACP(2), Method: "session/prompt", Params: rawJSON(map[string]any{
		"sessionId": snew.SessionID,
		"prompt":    []map[string]any{{"type": "text", "text": "hi"}},
	})})

	// Collect frames: expect ≥1 session/update (agent_message_chunk) + the prompt response.
	br := bufio.NewReader(cliR)
	chunks := 0

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := br.ReadBytes('\n')
		if len(line) == 0 {
			if err != nil {
				break
			}

			continue
		}

		var m acp.Message
		if json.Unmarshal(bytes.TrimRight(line, "\n"), &m) != nil {
			continue
		}

		if m.Method == "session/update" {
			chunks++
		}

		if m.ID != nil && *m.ID == 2 {
			var pres struct {
				StopReason string `json:"stopReason"`
			}

			_ = json.Unmarshal(m.Result, &pres)

			if pres.StopReason != "end_turn" {
				t.Errorf("stopReason = %q; want end_turn", pres.StopReason)
			}

			if chunks == 0 {
				t.Error("session/prompt response arrived with NO preceding session/update (ACP-04 streaming)")
			}

			return
		}
	}

	t.Fatalf("never saw the session/prompt response (chunks=%d)", chunks)
}

// TestIntegration_SessionLoadNoOp verifies session/load returns a -32601 error
// (D-09 — NO replay in v1).
func TestIntegration_SessionLoadNoOp(t *testing.T) {
	t.Parallel()

	mp := &mockStreamProvider{finish: "end_turn"}

	cliW, cliR, stop := driveACP(t, mp)
	defer stop()

	sendFrame(t, cliW, acp.Message{JSONRPC: "2.0", ID: intPtrACP(0), Method: "initialize", Params: rawJSON(map[string]any{"protocolVersion": 1})})
	readFrames(t, cliR, 1)
	sendFrame(t, cliW, acp.Message{JSONRPC: "2.0", ID: intPtrACP(1), Method: "session/load", Params: rawJSON(map[string]any{"sessionId": "x"})})

	frames := readFrames(t, cliR, 1)
	if len(frames) == 0 || frames[0].Error == nil {
		t.Fatalf("session/load did not return an error: %+v", frames)
	}

	if frames[0].Error.Code != -32601 {
		t.Errorf("session/load error code = %d; want -32601 (D-09)", frames[0].Error.Code)
	}
}

// rawJSON marshals m to json.RawMessage.
func rawJSON(m map[string]any) json.RawMessage {
	b, _ := json.Marshal(m)

	return b
}

// intPtrACP returns a pointer to i (test helper).
func intPtrACP(i int) *int { return &i }
