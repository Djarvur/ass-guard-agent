package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// wiringAskTool is the interactive tool under test (local alias: the captured
// catalog name).
const wiringAskTool = "AskUserQuestion"

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
			ID: "call_ask_w1", Name: wiringAskTool,
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
func TestAskWiring_ReplyRouting(t *testing.T) { //nolint:gocognit,funlen // flat battery
	t.Parallel()

	r, prov := newAskWiringRunner(t, time.Hour)

	em := &noopEmitter{}

	stop1, err := r.Run(context.Background(), "sess-ask-w1", em,
		[]acp.ContentBlock{{Type: blockText, Text: "I need to add a cache — ask me which library first"}})
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	if stop1 != stopEndTurn {
		t.Fatalf("Run 1 stop = %q; want end_turn (the ask marker is internal; the suspended turn maps to a completed turn)", stop1)
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
		if l.Type == session.TypeAskSuspended && l.ToolCallID == "call_ask_w1" {
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
		if l.Type != session.TypeToolResult || l.ToolCallID != "call_ask_w1" {
			continue
		}

		var got string

		if uerr := json.Unmarshal(l.Output, &got); uerr != nil {
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
		[]acp.ContentBlock{{Type: blockText, Text: "ask me which library"}})
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
		[]acp.ContentBlock{{Type: blockText, Text: "ask me"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var qs []session.AskQuestion

	if uerr := json.Unmarshal(json.RawMessage(wiringAskInput), &struct {
		Questions *[]session.AskQuestion `json:"questions"`
	}{Questions: &qs}); uerr != nil {
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
func TestAskWiring_ConfigKnob(t *testing.T) { //nolint:gocognit // flat battery
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
				[]acp.ContentBlock{{Type: blockText, Text: "ask me"}})
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
		[]acp.ContentBlock{{Type: blockText, Text: "ask me"}})
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
		t.Skip("shared runner catalog carries no AskUserQuestion (engine-off wiring); the coreexec suite pins the discipline")
	}

	if wired.Name != captured.Name ||
		wired.Description != captured.Description ||
		string(wired.InputSchema) != string(captured.InputSchema) ||
		wired.Mutability != captured.Mutability {
		t.Error("wired entry differs from the captured entry beyond Execute (schema-never-rewritten violated)")
	}
}
