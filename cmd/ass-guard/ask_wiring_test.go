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
	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// Test-local constants (goconst): the interactive tool under test, its wiring
// call id, the canned prompt texts, and the ACP handshake frame literals
// (shared counts with integration_test.go push these over the threshold).
const (
	methodInitialize = "initialize"
	keyProtoVersion  = "protocolVersion"
	keyMCPServers    = "mcpServers"
	wiringAskTool    = "AskUserQuestion"
	wiringAskCall    = "call_ask_w1"
	wiringAskMe      = "ask me"
	wiringAskCache   = "ask me which library"
)

// wiringAskInput is the plan's Test-1 question shape (one question, two
// labelled options).
const wiringAskInput = `{"questions":[{"question":"Which cache library should we use?",` +
	`"header":"Cache",` +
	`"options":[{"label":"ristretto","description":"fast in-memory cache"},` +
	`{"label":"bigcache","description":"simple disk-backed cache"}]}]}`

// newAskWiringRunner builds an ENGINE-ON runner (the real sessionFor path)
// scripted for a suspending first turn + a closing resumed turn, with the
// ask timeout set (the reply must win the race in the routing tests).
func newAskWiringRunner(t *testing.T, timeout time.Duration) (*sessionTurnRunner, *scriptedACPProvider) {
	t.Helper()

	r, prov := newExpansionRunner(t, true,
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: wiringAskCall, Name: wiringAskTool,
			Input: json.RawMessage(wiringAskInput),
		}}},
		scriptedResp{text: "acknowledged your answer"},
	)

	r.askTimeout = timeout

	return r, prov
}

// TestAskWiring_ReplyRouting (12-01 T2 Test 1, the phase's named leg at the
// wiring level): two sequential Run calls on one session — the first
// suspends with a pending ask (the client-visible turn completes, stop maps to
// end_turn); the second (the operator's reply text) resolves the broker, the
// reply lands as the pending call's tool result in the CAPTURED answered form
// (verbatim), and the second Run returns the RESUMED turn's stop reason — the
// model's continuation acknowledges the answer — with exactly two provider
// stream calls (no new turn was started).
func TestAskWiring_ReplyRouting(t *testing.T) { //nolint:gocyclo,cyclop,funlen // flat battery
	t.Parallel()

	r, prov := newAskWiringRunner(t, time.Hour)

	em := &noopEmitter{}

	stop1, err := r.Run(context.Background(), "sess-ask-w1", em,
		[]acp.ContentBlock{{Type: blockText, Text: "I need to add a cache — ask me which library first"}})
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	if stop1 != stopEndTurn {
		t.Fatalf("Run 1 stop = %q; want end_turn (the ask marker is internal; "+
			"the suspended turn maps to a completed turn)", stop1)
	}

	sess := r.sessions["sess-ask-w1"]

	if sess == nil || !sess.HasPendingAsk() {
		t.Fatal("no pending ask after the suspending turn (sessionFor must wire the broker)")
	}

	// The suspension is recorded at BOTH layers: the ask_suspended transcript
	// line + the engine_decision line (action=ask — the engine's ask path).
	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	sawSuspension, sawEngineAsk := false, false

	for _, l := range lines {
		if l.Type == session.TypeAskSuspended && l.ToolCallID == wiringAskCall {
			sawSuspension = true
		}

		if l.Type == session.TypeEngineDecision && l.Name == "ask" {
			sawEngineAsk = true
		}
	}

	if !sawSuspension {
		t.Error("transcript missing the ask_suspended record")
	}

	if !sawEngineAsk {
		t.Error("transcript missing the engine_decision action=ask line (the engine ask path)")
	}

	// The operator's reply: an ordinary session/prompt carrying the answer text.
	stop2, err := r.Run(context.Background(), "sess-ask-w1", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "ristretto, please"}})
	if err != nil {
		t.Fatalf("Run 2 (reply): %v", err)
	}

	if stop2 != stopEndTurn {
		t.Errorf("Run 2 stop = %q; want the RESUMED turn's end_turn", stop2)
	}

	if sess.HasPendingAsk() {
		t.Error("pending ask survived the reply routing")
	}

	if got := prov.callCount(); got != 2 {
		t.Errorf("provider stream calls = %d; want 2 (the reply resumed the SAME turn — no new user turn)", got)
	}

	// The reply lands as the pending call's tool result in the captured
	// answered form, verbatim.
	lines, err = sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll (post-reply): %v", err)
	}

	for _, l := range lines {
		if l.Type != session.TypeToolResult || l.ToolCallID != wiringAskCall {
			continue
		}

		var got string

		uerr := json.Unmarshal(l.Output, &got)
		if uerr != nil {
			t.Fatalf("answered output is not a JSON string: %v (%s)", uerr, l.Output)
		}

		const want = `User has answered your questions: "ristretto, please"`
		if got != want {
			t.Errorf("answered form = %q; want the captured shape %q", got, want)
		}

		return
	}

	t.Fatal("the reply never landed as the pending call's tool result")
}

// TestAskWiring_ClientSurface (12-01 T2 Test 2): during suspension the
// client-visible emitter receives the question + options rendered in the
// captured shape (header, question, option labels with descriptions) — the
// renderer emits structure, not judgment — before the suspended turn's
// response (the chunk flows through the SAME Run's forwarder).
func TestAskWiring_ClientSurface(t *testing.T) {
	t.Parallel()

	r, _ := newAskWiringRunner(t, time.Hour)

	em := &noopEmitter{}

	_, err := r.Run(context.Background(), "sess-ask-w2", em,
		[]acp.ContentBlock{{Type: blockText, Text: wiringAskCache}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	joined := strings.Join(em.chunks, "\n")

	for _, want := range []string{
		"[Cache]",                            // the header chip
		"Which cache library should we use?", // the question
		"ristretto",                          // option 1 label
		"fast in-memory cache",               // option 1 description
		"bigcache",                           // option 2 label
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("client chunks missing %q (rendered surface = %q)", want, joined)
		}
	}

	if strings.Contains(joined, "Recommended") {
		t.Errorf("client surface injected judgment (%q) — the (Recommended) convention is the model's", joined)
	}
}

// TestAskWiring_SurfaceMatchesRenderer (single-definition pin): the surface
// the client receives is exactly coreexec.RenderAskSurface of the parsed
// questions — the form is defined once, beside the executor.
func TestAskWiring_SurfaceMatchesRenderer(t *testing.T) {
	t.Parallel()

	r, _ := newAskWiringRunner(t, time.Hour)

	em := &noopEmitter{}

	_, err := r.Run(context.Background(), "sess-ask-w3", em,
		[]acp.ContentBlock{{Type: blockText, Text: wiringAskMe}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var qs []session.AskQuestion

	uerr := json.Unmarshal(json.RawMessage(wiringAskInput), &struct {
		Questions *[]session.AskQuestion `json:"questions"`
	}{Questions: &qs})
	if uerr != nil {
		t.Fatalf("unmarshal ask input: %v", uerr)
	}

	want := coreexec.RenderAskSurface(qs)

	found := false

	for _, c := range em.chunks {
		if c == want {
			found = true
		}
	}

	if !found {
		t.Errorf("no client chunk equals RenderAskSurface(%q); chunks = %q", want, em.chunks)
	}
}

// TestAskWiring_ConfigKnob (12-01 T2 Test 3, D-01): the ask timeout is
// configurable at the serve layer — the cobra flag exists with the 10m default
// and block-forever documentation, and the value threads runner → sessionFor →
// broker (7m propagates as 7m; 0 propagates as block-forever).
func TestAskWiring_ConfigKnob(t *testing.T) {
	t.Parallel()

	// The cobra flag: exists, default 10m, documents 0 = block forever.
	cmd := newACPServeCmd()

	flag := cmd.Flags().Lookup("ask-timeout")
	if flag == nil {
		t.Fatal("acp serve has no --ask-timeout flag")
	}

	if flag.DefValue != (10 * time.Minute).String() {
		t.Errorf("--ask-timeout default = %q; want the D-01 10m default", flag.DefValue)
	}

	if !strings.Contains(flag.Usage, "block forever") {
		t.Errorf("--ask-timeout usage = %q; want the 0 = block forever (interactive mode) documentation", flag.Usage)
	}

	// Threading: the runner's askTimeout reaches the sessionFor-built broker.
	for _, tc := range []struct {
		name string
		val  time.Duration
	}{
		{"custom 7m", 7 * time.Minute},
		{"block forever", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, _ := newAskWiringRunner(t, tc.val)

			_, err := r.Run(context.Background(), "sess-ask-knob-"+tc.name, &noopEmitter{},
				[]acp.ContentBlock{{Type: blockText, Text: wiringAskMe}})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			sess := r.sessions["sess-ask-knob-"+tc.name]

			b := sess.AskBroker()
			if b == nil {
				t.Fatal("sessionFor built no broker")
			}

			if got := b.Timeout(); got != tc.val {
				t.Errorf("broker timeout = %v; want the configured %v", got, tc.val)
			}
		})
	}
}

// TestAskWiring_SchemaDisciplineAtWiring (12-01 T2 + the no-confirmation-tier
// truth): at the REAL wiring site the registered AskUserQuestion entry is
// byte-identical to the shared catalog's captured entry EXCEPT Execute, and it
// is NOT mutating — a model-authored question reads nothing and gates nothing
// (the safety model is untouched; the safety battery passes unmodified).
func TestAskWiring_SchemaDisciplineAtWiring(t *testing.T) {
	t.Parallel()

	r, _ := newAskWiringRunner(t, time.Hour)

	_, err := r.Run(context.Background(), "sess-ask-w4", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: wiringAskMe}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	sess := r.sessions["sess-ask-w4"]

	wired, ok := sess.Catalog.Get(wiringAskTool)
	if !ok {
		t.Fatal("wired catalog missing AskUserQuestion")
	}

	if wired.Execute == nil {
		t.Fatal("wired AskUserQuestion has no Execute — the dead end is still there")
	}

	if wired.IsMutating() {
		t.Error("wired AskUserQuestion is mutating — a question reads nothing (the catalog entry is NOT mutating)")
	}

	// The captured entry (pre-registration): the shared runner catalog, whose
	// entry the per-session clone copied BEFORE RegisterAsk overrode Execute.
	captured, ok := r.catalog.Get(wiringAskTool)
	if !ok {
		t.Skip("shared runner catalog carries no AskUserQuestion (engine-off wiring); " +
			"the coreexec suite pins the discipline")
	}

	if wired.Name != captured.Name ||
		wired.Description != captured.Description ||
		string(wired.InputSchema) != string(captured.InputSchema) ||
		wired.Mutability != captured.Mutability {
		t.Error("wired entry differs from the captured entry beyond Execute (schema-never-rewritten violated)")
	}
}

// askToolCallProvider is a provider whose first Stream emits the AskUserQuestion
// tool call (text preamble + tool_use chunk), mirroring the live leg-1 model
// behavior; subsequent calls stream plain text (the resumed turn).
type askToolCallProvider struct {
	calls int
}

func (p *askToolCallProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, errNotUsed
}

func (p *askToolCallProvider) Stream(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.calls++

	ch := make(chan provider.StreamChunk, 6)

	go func() {
		defer close(ch)

		if p.calls == 1 {
			for _, c := range []string{"Let ", "me ", "ask."} {
				select {
				case ch <- provider.StreamChunk{Type: blockText, Text: c}:
				case <-ctx.Done():
					return
				}
			}

			tc := provider.ToolCall{
				ID: "call_srv_ask_1", Name: wiringAskTool,
				Input: json.RawMessage(wiringAskInput),
			}

			select {
			case ch <- provider.StreamChunk{Type: tracerToolUse, ToolCall: &tc, ToolCallID: tc.ID}:
			case <-ctx.Done():
				return
			}
		} else {
			select {
			case ch <- provider.StreamChunk{Type: blockText, Text: "acknowledged; proceeding"}:
			case <-ctx.Done():
				return
			}
		}

		select {
		case ch <- provider.StreamChunk{Type: chunkDone, FinishReason: stopEndTurn}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (p *askToolCallProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// TestAskWiring_ServerLevelSurface (the 12-01 live-witness finding, pinned):
// through the REAL acp.Server (stdio frames, the adapter emitter, the Writer —
// the exact live path), the rendered question surface MUST reach the client as
// an agent_message_chunk session/update BEFORE the suspended turn's stopReason
// response. The witness (leg 1, session d9f98023) caught the question missing
// from the live stream while the transcript recorded it; the runner-level
// emitter test alone could not see the gap.
func TestAskWiring_ServerLevelSurface(t *testing.T) { //nolint:cyclop,funlen // comprehensive server scenario
	t.Parallel()

	// The live path: the REAL engine wiring (setupEngine — the RealExecutor
	// executes AskUserQuestion; the engine-off driveACP stub never suspends).
	bus := event.NewBus()

	mp := &askToolCallProvider{}

	dir := t.TempDir()

	writeOpsxCommandFixtures(t, dir)

	runner := &sessionTurnRunner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      2,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return mp },
	}

	err := runner.setupEngine()
	if err != nil {
		t.Fatalf("setupEngine: %v", err)
	}

	runner.loadCommandRegistry()

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
			keySessionID: snew.SessionID,
			"prompt":     []any{map[string]any{"type": blockText, "text": "ask me which library"}},
		}),
	})

	// Read until the prompt response (id 2) arrives; collect every frame.
	var (
		gotResponse bool

		all []*acp.Message
	)

	br := bufio.NewReader(cliR)

	deadline := time.After(5 * time.Second)

	for !gotResponse {
		select {
		case <-deadline:
			t.Fatalf("no prompt response within 5s (frames so far: %d)", len(all))
		default:
		}

		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err != nil {
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

	questionOnWire := false

	for _, m := range all {
		if m.Method == "session/update" && strings.Contains(string(m.Params), "Which cache library") {
			questionOnWire = true
		}
	}

	if !questionOnWire {
		t.Errorf("the rendered question surface never reached the client wire "+
			"(%d frames; the live-witness finding reproduced)", len(all))
	}
}
