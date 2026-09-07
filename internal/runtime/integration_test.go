package runtime //nolint:testpackage // internal package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func (m *mockStreamProvider) Send(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{FinishReason: m.finish}, nil
}

func (m *mockStreamProvider) Stream(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 8)
	go func() {
		defer close(ch)

		for _, c := range m.chunks {
			select {
			case ch <- provider.StreamChunk{Type: blockText, Text: c}:
			case <-ctx.Done():
				return
			}
		}

		select {
		case ch <- provider.StreamChunk{Type: chunkDone, FinishReason: m.finish}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (m *mockStreamProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// SupportsImages: the fake is text-only (21-05 D-11 seam stub).
func (m *mockStreamProvider) SupportsImages() bool { return false }

// driveACP starts runACPServe with a mock provider and returns the client pipe
// ends. The test writes client frames to cliW and reads from cliR.
func driveACP(t *testing.T, mp provider.Provider) ( //nolint:nonamedreturns // names document the teardown triple
	cliW *io.PipeWriter, cliR io.Reader, stop func(),
) {
	t.Helper()

	bus := event.NewBus()
	prof := profile.Profile{Name: "test", System: []profile.TextBlock{{Type: blockText, Text: "test agent"}}}
	runner := &Runner{
		bus:          bus,
		profile:      prof,
		workDir:      t.TempDir(),
		maxConc:      2,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return mp },
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
func sendFrame(t *testing.T, w io.Writer, m *acp.Message) {
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
func writeFrameDirect(buf *bytes.Buffer, m *acp.Message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("call: %w", err)
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
// Zed-like initialize advertisement fragments (acp.rs:767-795): advertising
// elicitation.form keeps the D-13 advertisement-first path probe-free.
const (
	keyClientCapabilities = "clientCapabilities"
	keyElicitation        = "elicitation"
	keyForm               = "form"
)

func TestIntegration_RealStreamingThroughACP(t *testing.T) { //nolint:funlen // comprehensive test scenario
	t.Parallel()

	mp := &mockStreamProvider{chunks: []string{"Hello", " ", "world"}, finish: stopEndTurn}

	cliW, cliR, stop := driveACP(t, mp)
	defer stop()

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("0"), Method: "initialize",
		Params: rawJSON(map[string]any{"protocolVersion": 1,
			// Zed-like elicitation advertisement (acp.rs:767-795) — the D-13
			// advertisement-first rule means NO capability probe fires.
			keyClientCapabilities: map[string]any{
				keyElicitation: map[string]any{keyForm: map[string]any{}},
			}}),
	})

	frames := readFrames(t, cliR, 1)
	if len(frames) == 0 || !strings.Contains(string(frames[0].Result), "agentCapabilities") {
		t.Fatalf("no initialize response with agentCapabilities: %+v", frames)
	}

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("1"), Method: methodSessNew,
		Params: rawJSON(map[string]any{cwdKey: cwdForFrames, "mcpServers": []any{}}),
	})
	frames = readResultFrames(t, cliR, 1)

	var snew struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	_ = json.Unmarshal(frames[0].Result, &snew)

	if snew.SessionID == "" {
		t.Fatalf("no sessionId in session/new response: %+v", frames)
	}

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("2"), Method: methodSessPrmt,
		Params: rawJSON(map[string]any{
			keySessionID: snew.SessionID,
			"prompt":     []map[string]any{{keyType: blockText, blockText: "hi"}},
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

		if m.Method == sessionUpdate {
			chunks++
		}

		if m.ID != nil && string(m.ID) == "2" {
			var pres struct {
				StopReason string `json:"stopReason"` //nolint:tagliatelle // ACP wire field
			}

			_ = json.Unmarshal(m.Result, &pres)

			if pres.StopReason != stopEndTurn {
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

// TestIntegration_SessionLoadMalformedRejected verifies session/load with a
// malformed sessionId rejects with the typed -32602 BEFORE any file open
// (18-01/T-18-01 — the D-09 -32601 no-op ended with Phase 18; load is real,
// and the structural rejection is the first gate of the pipeline). Runs
// against the REAL runner wiring (driveACP) so the integration pin covers the
// composed server, not just the handler unit.
func TestIntegration_SessionLoadMalformedRejected(t *testing.T) {
	t.Parallel()

	mp := &mockStreamProvider{finish: stopEndTurn}

	cliW, cliR, stop := driveACP(t, mp)
	defer stop()

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("0"), Method: "initialize",
		Params: rawJSON(map[string]any{"protocolVersion": 1,
			// Zed-like elicitation advertisement (acp.rs:767-795) — the D-13
			// advertisement-first rule means NO capability probe fires.
			keyClientCapabilities: map[string]any{
				keyElicitation: map[string]any{keyForm: map[string]any{}},
			}}),
	})
	readFrames(t, cliR, 1)
	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("1"), Method: "session/load",
		Params: rawJSON(map[string]any{keySessionID: "x"}),
	})

	frames := readFrames(t, cliR, 1)
	if len(frames) == 0 || frames[0].Error == nil {
		t.Fatalf("session/load did not return an error: %+v", frames)
	}

	if frames[0].Error.Code != -32602 {
		t.Errorf("session/load error code = %d; want -32602 (malformed sessionId, 18-01)",
			frames[0].Error.Code)
	}
}

// rawJSON marshals m to json.RawMessage.
func rawJSON(m map[string]any) json.RawMessage {
	b, marshalErr := json.Marshal(m)
	if marshalErr != nil {
		panic(marshalErr)
	}

	return b
}

// readResultFrames reads frames until n REQUEST RESPONSES (frames carrying
// an id) arrive — session/update notifications emitted ahead of responses
// are skipped (20-01: session/new now precedes its response with the
// available_commands_update advertisement).
func readResultFrames(t *testing.T, cliR io.Reader, n int) []*acp.Message {
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

		var msg acp.Message
		if jerr := json.Unmarshal(line, &msg); jerr != nil {
			continue
		}

		if msg.ID == nil {
			continue // a notification — not a response; keep reading
		}

		out = append(out, &msg)
	}

	return out
}

// readResultFramesCounting reads n request responses, returning them plus
// the count of session/update notifications skipped ahead of them (20-01:
// session start's available_commands_update advertisement is a REAL
// emitter-written notification — WrittenNotifications accounting must
// include it even when a test's own update collection starts later).
func readResultFramesCounting(t *testing.T, cliR io.Reader, n int) ([]*acp.Message, int) {
	t.Helper()

	br := bufio.NewReader(cliR)

	var out []*acp.Message

	skipped := 0

	deadline := time.After(3 * time.Second)

	for len(out) < n {
		select {
		default:
		case <-deadline:
			return out, skipped
		}

		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			return out, skipped
		}

		line = bytes.TrimRight(line, "\n")
		if len(line) == 0 {
			continue
		}

		var msg acp.Message
		if jerr := json.Unmarshal(line, &msg); jerr != nil {
			continue
		}

		if msg.ID == nil {
			if msg.Method == sessionUpdate {
				skipped++
			}

			continue
		}

		out = append(out, &msg)
	}

	return out, skipped
}
