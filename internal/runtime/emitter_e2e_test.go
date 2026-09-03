package runtime //nolint:testpackage // internal package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// Repeated frame-literal keys for this file (goconst).
const (
	chunkToolUse   = "tool_use"
	testCwdTmpPath = "/tmp"
	keyPrompt      = "prompt"
	keyMcpServers  = "mcpServers"
	chunkThinking  = "thinking"
)

// The Task-1 tracer (16-01): ONE frame path end-to-end. A bus ToolCall event
// published DURING a live Run turn must flow through the runtime forwarder into
// the TurnEmitter's foreground lane, out the single drain goroutine into the
// Server's Writer, and reach stdout as a v1 session/update tool_call frame —
// with the agent_message_chunk frames around it in EMISSION order.
//
// The TurnEmitter's drain goroutine is the ONLY producer of session/update
// notification frames into the Writer, so the emitter's written-notification
// count must equal the notification frames observed on stdout.

// emitPhase is one paced publication stage of the fake provider stream: a text
// chunk, a tool_use chunk, or a thinking chunk (PAR-05, 21-03), followed by a
// pause so each bus publication is observed (and forwarded + enqueued) BEFORE
// the next one fires. The pacing is what makes "emission order" well-defined
// for the assertions below.
type emitPhase struct {
	text       string        // when non-empty: a text chunk (AgentMessageChunk on the bus)
	toolID     string        // when non-empty: a tool_use chunk (ToolCall on the bus)
	toolName   string        //   … its captured tool name
	input      string        //   … its raw input JSON
	thoughtRaw string        // when non-empty: a thinking chunk (AgentThoughtChunk on the bus)
	pause      time.Duration // sleep AFTER producing this phase's stream chunk
}

// pacedStreamProvider streams emitPhases then the done chunk — ONCE. The
// session tool loop re-streams while a response carries tool calls, so the
// SECOND Stream call returns an empty stream: the turn then terminates after
// the tool pass instead of looping to its 64-iteration runaway bound (the
// deadline-fragile behavior the full-CI run exposed). It is the transport
// under the real Session Core: streamAndEmit publishes AgentMessageChunk /
// ToolCall bus events per chunk exactly as production does.
type pacedStreamProvider struct {
	phases []emitPhase
	finish string
	calls  atomic.Int64
}

func (p *pacedStreamProvider) Send(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{FinishReason: p.finish}, nil
}

func (p *pacedStreamProvider) Stream(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, len(p.phases)+1)

	first := p.calls.Add(1) == 1

	go func() {
		defer close(ch)

		if !first {
			// The post-tool model round: nothing selected, turn ends.
			select {
			case ch <- provider.StreamChunk{Type: chunkDone, FinishReason: p.finish}:
			case <-ctx.Done():
			}

			return
		}

		for _, ph := range p.phases {
			var chunk provider.StreamChunk

			switch {
			case ph.text != "":
				chunk = provider.StreamChunk{Type: blockText, Text: ph.text}
			case ph.toolID != "":
				chunk = provider.StreamChunk{
					Type:       chunkToolUse,
					ToolCall:   &provider.ToolCall{ID: ph.toolID, Name: ph.toolName, Input: json.RawMessage(ph.input)},
					ToolCallID: ph.toolID,
				}
			case ph.thoughtRaw != "":
				chunk = provider.StreamChunk{Type: chunkThinking, Raw: json.RawMessage(ph.thoughtRaw)}
			}

			select {
			case ch <- chunk:
			case <-ctx.Done():
				return
			}

			if ph.pause > 0 {
				time.Sleep(ph.pause)
			}
		}

		select {
		case ch <- provider.StreamChunk{Type: chunkDone, FinishReason: p.finish}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (p *pacedStreamProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// TestTurnEmitterEndToEnd proves the tracer slice: initialize → session/new →
// session/prompt over a REAL acp.Server over pipes, with the real Runner fixture
// and the emitter-backed notification path. Asserts:
//
//  1. stdout carries agent_message_chunk("A"), then a v1 tool_call frame for
//     the Bash call (toolCallId + title from the tool name), then
//     agent_message_chunk("B") — in emission order;
//  2. the prompt response arrives after those updates are flushed;
//  3. the emitter's written-frame count equals the notification frames
//     observed on stdout (sole-producer invariant — no direct Writer writes).
func TestTurnEmitterEndToEnd(t *testing.T) { //nolint:funlen // full end-to-end scenario
	t.Parallel()

	mp := &pacedStreamProvider{
		finish: stopEndTurn,
		phases: []emitPhase{
			{text: "A", pause: 15 * time.Millisecond},
			{toolID: "tc-tracer-1", toolName: "Bash", input: `{"command":"ls"}`, pause: 15 * time.Millisecond},
			{text: "B", pause: 15 * time.Millisecond},
		},
	}

	bus := event.NewBus()
	prof := profile.Profile{Name: profileZcode, System: []profile.TextBlock{{Type: blockText, Text: "emitter e2e"}}}
	runner := &Runner{
		bus:          bus,
		profile:      prof,
		workDir:      t.TempDir(),
		maxConc:      2,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return mp },
	}

	srvInR, cliW := io.Pipe()
	cliR, srvOutW := io.Pipe()

	stderr := &bytes.Buffer{}

	// WithTurnEmitter arms the composition-root TurnEmitter; srv.Emitter hands
	// out foreground-class handles so every notification routes through the
	// single ordered drain.
	srv := acp.NewServer(srvInR, srvOutW, stderr, acp.WithTurnRunner(runner),
		acp.WithTurnEmitter(acp.TurnEmitterConfig{}))
	runner.SetEmitter(srv.Emitter) // WINDOWS #3 wiring — same junction as acpserve.Run

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { _ = srv.Serve(ctx); close(done) }()

	t.Cleanup(func() {
		cancel()

		_ = cliW.Close()
		_ = srvOutW.Close()
		_ = srvInR.Close()
		_ = cliR.Close()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	})

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("0"), Method: methodInitialize,
		Params: rawJSON(map[string]any{keyProtoVersion: 1,
			// Zed-like elicitation advertisement (acp.rs:767-795) — the D-13
			// advertisement-first rule means NO capability probe fires.
			keyClientCapabilities: map[string]any{
				keyElicitation: map[string]any{keyForm: map[string]any{}},
			}}),
	})

	frames := readFrames(t, cliR, 1)
	if len(frames) == 0 || !strings.Contains(string(frames[0].Result), "agentCapabilities") {
		t.Fatalf("no initialize response: %+v", frames)
	}

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("1"), Method: methodSessNew,
		Params: rawJSON(map[string]any{cwdKey: testCwdTmpPath, keyMcpServers: []any{}}),
	})
	frames = readFrames(t, cliR, 1)

	var snew struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	snewErr := json.Unmarshal(frames[0].Result, &snew)
	if snewErr != nil || snew.SessionID == "" {
		t.Fatalf("no sessionId from session/new: %v %+v", snewErr, frames)
	}

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("2"), Method: methodSessPrmt,
		Params: rawJSON(map[string]any{
			keySessionID: snew.SessionID,
			keyPrompt:    []map[string]any{{keyType: blockText, blockText: "hi"}},
		}),
	})

	// Read sequentially until the prompt response: the pipe preserves byte
	// order, and the single-drain emitter serializes notifications, so read
	// order IS wire emission order.
	br := bufio.NewReader(cliR)

	updates := readSessionUpdatesUntilResponse(t, br, "2")

	want := []string{
		"agent_message_chunk||",
		"tool_call|tc-tracer-1|Bash",
		"agent_message_chunk||",
	}
	if len(updates) < len(want) {
		t.Fatalf("expected at least %d notification frames, got %d: %v", len(want), len(updates), updates)
	}

	for i, w := range want {
		if updates[i] != w {
			t.Fatalf("emission order broken at %d:\n got: %v\nwant prefix: %v", i, updates, want)
		}
	}

	// Sole-producer invariant: the emitter's written-notification count equals
	// the notification frames observed on stdout. Allow the final bookkeeping
	// write to land, then settle-check twice with a gap (count must stabilize).
	time.Sleep(50 * time.Millisecond)

	written := srv.TurnEmitter().WrittenNotifications()

	time.Sleep(50 * time.Millisecond)

	if stable := srv.TurnEmitter().WrittenNotifications(); stable != written {
		t.Fatalf("written count unstable: %d then %d", written, stable)
	}

	if written != len(updates) {
		t.Fatalf("notification leak: emitter wrote %d but stdout carried %d update frames",
			written, len(updates))
	}
}

// TestThoughtForward pins PAR-05's live leg (21-03, D-13): a provider thinking
// chunk streams through streamAndEmit → the AgentThoughtChunk bus event → the
// runtime forwarder → emit.ThoughtChunk, arriving on stdout as an
// agent_thought_chunk frame BEFORE the turn's agent_message_chunk (the
// ordered-emitter guarantee, 16-D-02).
func TestThoughtForward(t *testing.T) { //nolint:funlen // full end-to-end scenario
	t.Parallel()

	mp := &pacedStreamProvider{
		finish: stopEndTurn,
		phases: []emitPhase{
			{thoughtRaw: `{"type":"thinking","thinking":"weighing options","signature":"sig-tf"}`, pause: 15 * time.Millisecond},
			{text: "A", pause: 15 * time.Millisecond},
		},
	}

	bus := event.NewBus()
	prof := profile.Profile{Name: profileZcode, System: []profile.TextBlock{{Type: blockText, Text: "thought e2e"}}}
	runner := &Runner{
		bus:          bus,
		profile:      prof,
		workDir:      t.TempDir(),
		maxConc:      2,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return mp },
	}

	srvInR, cliW := io.Pipe()
	cliR, srvOutW := io.Pipe()

	stderr := &bytes.Buffer{}

	srv := acp.NewServer(srvInR, srvOutW, stderr, acp.WithTurnRunner(runner),
		acp.WithTurnEmitter(acp.TurnEmitterConfig{}))
	runner.SetEmitter(srv.Emitter)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { _ = srv.Serve(ctx); close(done) }()

	t.Cleanup(func() {
		cancel()

		_ = cliW.Close()
		_ = srvOutW.Close()
		_ = srvInR.Close()
		_ = cliR.Close()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	})

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("0"), Method: methodInitialize,
		Params: rawJSON(map[string]any{keyProtoVersion: 1,
			keyClientCapabilities: map[string]any{
				keyElicitation: map[string]any{keyForm: map[string]any{}},
			}}),
	})

	frames := readFrames(t, cliR, 1)
	if len(frames) == 0 || !strings.Contains(string(frames[0].Result), "agentCapabilities") {
		t.Fatalf("no initialize response: %+v", frames)
	}

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("1"), Method: methodSessNew,
		Params: rawJSON(map[string]any{cwdKey: testCwdTmpPath, keyMcpServers: []any{}}),
	})
	frames = readFrames(t, cliR, 1)

	var snew struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	snewErr := json.Unmarshal(frames[0].Result, &snew)
	if snewErr != nil || snew.SessionID == "" {
		t.Fatalf("no sessionId from session/new: %v %+v", snewErr, frames)
	}

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("2"), Method: methodSessPrmt,
		Params: rawJSON(map[string]any{
			keySessionID: snew.SessionID,
			keyPrompt:    []map[string]any{{keyType: blockText, blockText: "hi"}},
		}),
	})

	br := bufio.NewReader(cliR)

	updates := readSessionUpdatesUntilResponse(t, br, "2")

	want := []string{
		"agent_thought_chunk||",
		"agent_message_chunk||",
	}
	if len(updates) < len(want) {
		t.Fatalf("expected at least %d notification frames, got %d: %v", len(want), len(updates), updates)
	}

	for i, w := range want {
		if updates[i] != w {
			t.Fatalf("emission order broken at %d:\n got: %v\nwant prefix: %v (thought before message, 16-D-02)",
				i, updates, want)
		}
	}
}

// readSessionUpdatesUntilResponse reads frames sequentially until the response
// with the given id arrives, returning every session/update's discriminator as
// "kind|toolCallId|title" in exact arrival order (the wire's ground truth).
func readSessionUpdatesUntilResponse( //nolint:nonamedreturns // name documents the return
	t *testing.T, br *bufio.Reader, responseID string,
) (updates []string) {
	t.Helper()

	deadline := time.After(10 * time.Second)

	gotResp := false
	for !gotResp {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for prompt response; updates=%v", updates)
		default:
		}

		line, rerr := br.ReadBytes('\n')
		if len(line) == 0 && rerr != nil {
			t.Fatalf("stdout closed before response; updates=%v", updates)
		}

		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var m acp.Message

		if json.Unmarshal(bytes.TrimRight(line, "\n"), &m) != nil {
			continue
		}

		switch {
		case m.ID != nil && string(m.ID) == responseID:
			gotResp = true
		case m.Method == sessionUpdate:
			var upd struct {
				Update struct {
					SessionUpdate string `json:"sessionUpdate"` //nolint:tagliatelle // ACP wire field
					ToolCallID    string `json:"toolCallId"`    //nolint:tagliatelle // ACP wire field
					Title         string `json:"title"`
				} `json:"update"`
			}

			jerr := json.Unmarshal(m.Params, &upd)
			if jerr == nil {
				updates = append(updates, upd.Update.SessionUpdate+"|"+upd.Update.ToolCallID+"|"+upd.Update.Title)
			}
		}
	}

	return updates
}
