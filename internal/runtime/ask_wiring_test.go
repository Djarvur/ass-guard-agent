package runtime //nolint:testpackage // internal package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/learning"
	"github.com/Djarvur/ass-guard-agent/internal/openspec"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
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

	// The chained-ask batteries' shared literals (goconst).
	handoffToApply      = "handoff to apply"
	postProposeRowID    = "post-propose-handoff"
	postExploreRowID    = "post-explore-handoff"
	chainExploreDone    = "exploration complete — handoff to propose"
	chainProposeDone    = "proposal written — handoff to apply"
	chainApplyDone      = "applied everything; nothing further to do"
	chainHandoffPropose = "handoff to propose"
	cwdKey              = "cwd"
	methodSessNew       = "session/new"
	methodSessPrmt      = "session/prompt"
	sessionUpdate       = "session/update"
	promptListKey       = "prompt"
	cwdForFrames        = "/tmp"
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
func newAskWiringRunner(t *testing.T, timeout time.Duration) (*Runner, *scriptedACPProvider) {
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

		const want = `User has answered your questions: "Which cache library should we use?"="ristretto, please"` +
			`. You can now continue with the user's answers in mind.`
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
// configurable at the serve layer (the cobra flag's 10m default + block-forever
// documentation are pinned by TestACPServeCommandRegistered in cmd), and the
// value threads runner → sessionFor → broker (7m propagates as 7m; 0
// propagates as block-forever).
func TestAskWiring_ConfigKnob(t *testing.T) {
	t.Parallel()

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

// TestAskWiring_ChainSurvivesAskTimerResume (13-00 T1, RED — the manager
// Rule-4 ruling's behavior pin; diagnosis: 13-01-eval-first-runs/
// iterations-20260820/README.md): a mid-chain AskUserQuestion suspends the
// INJECTED turn; the D-01 timer (50ms here) resumes it with the non-answer;
// the COMPLETED turn MUST then get its engine decision (continue) and the
// chain MUST survive to the next stage. Today BOTH fail: session.Prompt
// returns the stopAsk marker promptly (correct — invariant 1), but
// engine.Observe exits at the post-injection `stop != "end_turn"` check
// (observe.go) WITHOUT deciding, and the timer's resumeAskClaimed runs
// DETACHED at the session layer — engine-invisible — so the resumed turn's
// completion never feeds Decide and every remaining injection is lost (the
// flagship eval-gate death: 2 of 3 completed gate iterations).
//
// Turn script (the flagship death shape):
//
//	turn 1 (typed explore):  closing matching post-explore-handoff → continue
//	turn 2 (injected propose): AskUserQuestion → suspends (stopAsk); the 50ms
//	                           timer lands the non-answer; the resumed turn
//	                           closes matching post-propose-handoff
//	turn 3 (injected apply):  unmatched closing → end_turn (chain terminates)
//
// OFFLINE (no env gates). Under today's code the poll first waits out the
// detached timer resume, then BOTH pinned assertions fail.
func TestAskWiring_ChainSurvivesAskTimerResume(t *testing.T) { //nolint:cyclop,funlen // settle poll
	t.Parallel()

	r, prov := newExpansionRunner(t, true,
		scriptedResp{text: "exploration complete — handoff to propose", finish: stopEndTurn},
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: wiringAskCall, Name: wiringAskTool,
			Input: json.RawMessage(wiringAskInput),
		}}},
		scriptedResp{text: chainProposeDone, finish: stopEndTurn},
		scriptedResp{text: chainApplyDone, finish: stopEndTurn},
	)

	r.askTimeout = 50 * time.Millisecond

	// The seeded chain rows: explore→propose→apply (the flagship shape).
	cfg := &openspec.OpenSpecConfig{Patterns: []openspec.PatternEntry{
		{
			ID: postExploreRowID, Regex: chainHandoffPropose,
			Action: actionContinue, Next: "/opsx:propose ask-chain",
		},
		{ID: postProposeRowID, Regex: handoffToApply, Action: actionContinue, Next: "/opsx:apply ask-chain"},
	}}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	r.patternTable = pt

	const sid = "sess-ask-chain"

	stop, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "/opsx:explore ask-chain"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stop != stopEndTurn {
		t.Fatalf("Run stop = %q; want end_turn (the ask marker is internal; mapAskStop)", stop)
	}

	sess := r.sessions[sid]

	// Wait for the engine chain to go IDLE — the precise, load-immune signal
	// (the chain count reaches zero exactly when the parked Observe goroutine
	// exits: every decision + injection done). Under today's code the chain
	// exits at the suspension; the pins below then fail on the missing
	// decision/continuation.
	idleCtx, idleCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer idleCancel()

	if !r.WaitChainIdle(idleCtx, sid) {
		t.Fatal("the engine chain did not go idle within 15s — the fixture hung")
	}

	lines, rerr := sess.Manager.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	// Locate the suspension (the asking turn) — the engine_decision pin keys
	// on ITS turn id + ordering after it.
	askIdx := -1

	askTurnID := ""

	for i := range lines {
		if lines[i].Type == session.TypeAskSuspended {
			askIdx = i
			askTurnID = lines[i].TurnID

			break
		}
	}

	if askIdx < 0 {
		t.Fatal("no ask_suspended line — the scripted ask never suspended the turn")
	}

	// PIN (a): an engine_decision line EXISTS for the asking turn AFTER its
	// completion (the decision was made on the resolved+completed turn —
	// action continue). Today: Observe exited at the post-injection check
	// before deciding; the detached resume never fed the completion back.
	decided := false

	for i := range lines {
		if i > askIdx && lines[i].Type == session.TypeEngineDecision &&
			lines[i].TurnID == askTurnID && lines[i].Name == actionContinue {
			decided = true

			break
		}
	}

	if !decided {
		t.Errorf("no continue engine_decision for the asking turn %q after its completion — "+
			"Observe exited on stopAsk without deciding and the D-01 timer resume ran "+
			"engine-invisible (the 13-00 blocker, pinned)", askTurnID)
	}

	// PIN (b): the apply-stage turn EXISTS in the transcript (the chain
	// survived the ask). Today: the injection is lost with the engine loop.
	// (user_message bodies ride the Content field — lastUserMessageText
	// assembles them.)
	if got := lastUserMessageText(t, r, sid); !strings.Contains(got, "Apply the change:") {
		t.Errorf("last user_message = %q; want the apply-stage turn — the chain died at the ask "+
			"(the resumed turn's completion produced no continuation)", got)
	}

	// Sanity (not a pinned failure): the resumed turn DID complete — without
	// this the two pins above would fail for the wrong reason.
	resumed := false

	for i := range lines {
		if lines[i].Type == session.TypeAssistantMessage && lines[i].TurnID == askTurnID {
			resumed = true

			break
		}
	}

	if !resumed {
		t.Fatalf("the suspended turn %q never completed after the timer resume — "+
			"the fixture is broken, not the pins", askTurnID)
	}

	_ = prov.callCount() // script-shape debug aid when re-tuned
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

// cr02GateTool is the mutating tool the CR-02 mutex pins gate on; cr02PermFire
// is the blocking permission surface (the dialog stays open until release).
const cr02GateTool = toolNameWrite

// TestAskWiring_ResumeHoldsTurnMutex (17-REVIEW CR-02, the race pin): a
// gated turn suspends with the dialog OPEN; while the session's turn mutex is
// held (a client turn in flight), the dialog answer must NOT run the resumed
// model loop — the async resume waits on the SAME per-session serialization
// the Run path holds. Pre-fix, the pump's resolve appended the tool result and
// ran the full resumed turn WHILE the mutex was held (two concurrent turn
// drivers on one transcript).
func TestAskWiring_ResumeHoldsTurnMutex(t *testing.T) { //nolint:funlen // the full suspend/race/resume chain
	t.Parallel()

	r, _ := newExpansionRunner(t, true,
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: "call_cr02_w", Name: cr02GateTool, Input: json.RawMessage(`{"file_path":"x"}`),
		}}},
		scriptedResp{text: "resumed turn closed"},
	)

	// A mutating tool in the SHARED catalog (sessionFor clones it) makes the
	// scripted call ask-class; gated mode routes it to the dialog.
	r.catalog.Register(toolcat.Tool{
		Name:        cr02GateTool,
		Mutability:  toolcat.MutabilityMutating,
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"output":"written"}`), nil
		},
	})

	r.SetPermMode(session.PermModeGated)

	dialogOpen := make(chan struct{}, 1)

	release := make(chan struct{})

	r.SetPermissionAskFire(func(_ context.Context, _ *session.AskEntry) session.AskOutcome {
		dialogOpen <- struct{}{}

		<-release

		return session.AskOutcome{Selected: acp.PermOptionAllowOnce}
	})

	const sid = "sess-cr02-mu"

	stop, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "gated work"}})
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	if stop != stopEndTurn {
		t.Fatalf("stop = %q; want end_turn (the suspended turn maps to a completed turn)", stop)
	}

	<-dialogOpen // the dialog is open; the answer has not arrived

	// Hold the session's turn mutex — "a client turn is in flight".
	mu := r.sessionTurnMu(sid)
	mu.Lock()

	close(release) // the operator answers NOW, against the held mutex

	time.Sleep(150 * time.Millisecond) // window for a buggy unserialized resume to append

	if got := len(transcriptLinesOfType(t, r, sid, session.TypeToolResult)); got != 0 {
		mu.Unlock()
		t.Fatalf("the dialog resume ran WHILE the turn mutex was held (%d tool results already) — "+
			"the async resume bypassed the per-session turn serialization (CR-02)", got)
	}

	mu.Unlock()

	// With the mutex free, the queued resume proceeds: the gated call's result
	// lands and the resumed turn closes.
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if len(transcriptLinesOfType(t, r, sid, session.TypeToolResult)) == 1 {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if got := len(transcriptLinesOfType(t, r, sid, session.TypeToolResult)); got != 1 {
		t.Fatalf("the queued resume never landed after the mutex was released (%d results)", got)
	}

	idleCtx, idleCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer idleCancel()

	if !r.WaitChainIdle(idleCtx, sid) {
		t.Fatal("the engine chain did not go idle — the parked chain wedged behind the serialized resume")
	}
}

// TestAskWiring_TimerResumeHoldsTurnMutex (CR-02's D-01 leg, the pre-existing
// shape): the timer fires while the turn mutex is held — its resume must queue
// behind the mutex, not run detached alongside the active turn.
func TestAskWiring_TimerResumeHoldsTurnMutex(t *testing.T) {
	t.Parallel()

	r, _ := newAskWiringRunner(t, 50*time.Millisecond)

	const sid = "sess-cr02-timer"

	_, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "I need to add a cache — ask me which library first"}})
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	// Hold the turn mutex BEFORE the 50ms timer fires.
	mu := r.sessionTurnMu(sid)
	mu.Lock()

	time.Sleep(150 * time.Millisecond) // the timer fires and must BLOCK on the mutex

	if got := len(transcriptLinesOfType(t, r, sid, session.TypeToolResult)); got != 0 {
		mu.Unlock()
		t.Fatalf("the D-01 timer resume ran WHILE the turn mutex was held (%d tool results already) — "+
			"the timer resume bypassed the per-session turn serialization (CR-02)", got)
	}

	mu.Unlock()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if len(transcriptLinesOfType(t, r, sid, session.TypeToolResult)) == 1 {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("the timer resume never landed after the mutex was released")
}

// TestAskWiring_ChainSurvivesPermissionDialogResume (17-REVIEW CR-05, the
// engine-on gate pin — the permission-family sibling of
// TestAskWiring_ChainSurvivesAskTimerResume): an INJECTED engine turn's gated
// call suspends on the permission dialog; the chain must PARK (stay alive)
// while the dialog is open — not exit silently on a nil settle channel — and
// after the operator allows, the resumed turn must feed the engine so the
// chain continues to the apply stage. Pre-fix, the permission family never
// armed AskSettleChan, so waitAskSettled saw nil and the chain died at the
// dialog (decisions + remaining injections lost).
func TestAskWiring_ChainSurvivesPermissionDialogResume(t *testing.T) { //nolint:funlen,cyclop // park/resume chain
	t.Parallel()

	r, _ := newExpansionRunner(t, true,
		scriptedResp{text: chainExploreDone},
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: "call_cr05_w", Name: cr02GateTool, Input: json.RawMessage(`{"file_path":"x"}`),
		}}},
		scriptedResp{text: "proposal written — handoff to apply"},
		scriptedResp{text: "applied everything; nothing further to do"},
	)

	// The gated mutating tool + gated mode (the dialog leg).
	r.catalog.Register(toolcat.Tool{
		Name:        cr02GateTool,
		Mutability:  toolcat.MutabilityMutating,
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"output":"written"}`), nil
		},
	})

	r.SetPermMode(session.PermModeGated)

	dialogOpen := make(chan struct{}, 1)

	release := make(chan struct{})

	r.SetPermissionAskFire(func(_ context.Context, _ *session.AskEntry) session.AskOutcome {
		dialogOpen <- struct{}{}

		<-release

		return session.AskOutcome{Selected: acp.PermOptionAllowOnce}
	})

	// The seeded explore→propose→apply chain (the flagship shape).
	cfg := &openspec.OpenSpecConfig{Patterns: []openspec.PatternEntry{
		{
			ID: postExploreRowID, Regex: chainHandoffPropose,
			Action: actionContinue, Next: "/opsx:propose ask-chain",
		},
		{ID: postProposeRowID, Regex: handoffToApply, Action: actionContinue, Next: "/opsx:apply ask-chain"},
	}}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	r.patternTable = pt

	const sid = "sess-cr05-chain"

	stop, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "/opsx:explore ask-chain"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stop != stopEndTurn {
		t.Fatalf("Run stop = %q; want end_turn (the suspension maps to a completed turn)", stop)
	}

	<-dialogOpen // the gated dialog is open; the chain must be parked behind it

	// PIN (a): the chain is PARKED — alive, waiting on the settle seam — while
	// the dialog is open. Pre-fix, the nil AskSettleChan made Observe exit
	// silently here (the chain went idle with the dialog unanswered).
	parkCtx, parkCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer parkCancel()

	if r.WaitChainIdle(parkCtx, sid) {
		t.Error("the engine chain exited while the permission dialog was open — " +
			"the suspension was engine-invisible (CR-05: nil settle seam)")
	}

	close(release) // the operator allows → resume → settle → the chain continues

	idleCtx, idleCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer idleCancel()

	if !r.WaitChainIdle(idleCtx, sid) {
		t.Fatal("the engine chain did not go idle within 15s — the chain wedged behind the dialog")
	}

	// PIN (b): the apply-stage turn EXISTS — the chain survived the dialog and
	// injected the remaining stage.
	if got := lastUserMessageText(t, r, sid); !strings.Contains(got, "Apply the change:") {
		t.Errorf("last user_message = %q; want the apply-stage turn — the chain died at the gated dialog", got)
	}

	// PIN (c): a continue decision exists for the gated (suspended) turn after
	// its completion — the resumed turn fed Decide.
	lines, rerr := r.sessions[sid].Manager.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	askIdx, askTurnID := -1, ""

	for i := range lines {
		if lines[i].Type == session.TypeAskSuspended {
			askIdx, askTurnID = i, lines[i].TurnID

			break
		}
	}

	if askIdx < 0 {
		t.Fatal("no ask_suspended line — the gated call never suspended")
	}

	decided := false

	for i := range lines {
		if i > askIdx && lines[i].Type == session.TypeEngineDecision &&
			lines[i].TurnID == askTurnID && lines[i].Name == actionContinue {
			decided = true

			break
		}
	}

	if !decided {
		t.Errorf("no continue engine_decision for the gated turn %q after its completion — "+
			"the resumed turn never fed the engine (CR-05)", askTurnID)
	}
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

	// The live path: the REAL engine wiring (SetupEngine — the RealExecutor
	// executes AskUserQuestion; the engine-off driveACP stub never suspends).
	bus := event.NewBus()

	mp := &askToolCallProvider{}

	dir := t.TempDir()

	writeOpsxCommandFixtures(t, dir)

	runner := &Runner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      2,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return mp },
	}

	err := runner.SetupEngine()
	if err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	runner.LoadCommandRegistry()

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
		JSONRPC: protocolVersion20, ID: json.RawMessage("1"), Method: methodSessNew,
		Params: rawJSON(map[string]any{cwdKey: cwdForFrames, keyMCPServers: []any{}}),
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
		JSONRPC: protocolVersion20, ID: json.RawMessage("2"), Method: methodSessPrmt,
		Params: rawJSON(map[string]any{
			keySessionID:  snew.SessionID,
			promptListKey: []any{map[string]any{keyType: blockText, textListKey: wiringAskCache}},
		}),
	})

	// Read until the prompt response (id 2) arrives; collect every frame.
	var (
		gotResponse bool

		all []*acp.Message
	)

	br := bufio.NewReader(cliR)

	// The 30s deadline (not 5s) is intentional: this orchestrated server test
	// runs t.Parallel() with the rest of the suite under -race, and the full
	// round-trip has repeatedly blown a 5s budget on loaded machines (observed
	// 6.4–8.6s on baseline, pre-260819-nlg). The assertion is ordering-based,
	// not timing-based — the deadline only bounds a hang.
	deadline := time.After(30 * time.Second)

	for !gotResponse {
		select {
		case <-deadline:
			t.Fatalf("no prompt response within 30s (frames so far: %d)", len(all))
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
		if m.Method == sessionUpdate && strings.Contains(string(m.Params), "Which cache library") {
			questionOnWire = true
		}
	}

	if !questionOnWire {
		t.Errorf("the rendered question surface never reached the client wire "+
			"(%d frames; the live-witness finding reproduced)", len(all))
	}
}

// --- 13-00 T4: the serve park pins ---

// TestAskPark_PromptResponsePrecedesResolution (T4 pin 1, wire byte-compat —
// server level): the session/prompt RESPONSE arrives AT the suspension, BEFORE
// any resolution — the ask surface reached the client first, the pending ask
// is still unresolved at response time (the D-01 timer is 1h away), and the
// chain continues parked. A synchronous-wait implementation (the overruled
// README option 2) would hold the response for the 1h timer and fail the
// deadline — the stuck-spinner + reply-deadlock shape the 12-01 witness
// verified against.
func TestAskPark_PromptResponsePrecedesResolution(t *testing.T) { //nolint:cyclop,gocyclo,funlen // server scenario
	t.Parallel()

	bus := event.NewBus()

	mp := &askToolCallProvider{}

	dir := t.TempDir()

	writeOpsxCommandFixtures(t, dir)

	runner := &Runner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      2,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return mp },
		askTimeout:   time.Hour, // the timer never fires in test time — the response must NOT wait for it
	}

	err := runner.SetupEngine()
	if err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	runner.LoadCommandRegistry()

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
		JSONRPC: protocolVersion20, ID: json.RawMessage("1"), Method: methodSessNew,
		Params: rawJSON(map[string]any{cwdKey: cwdForFrames, keyMCPServers: []any{}}),
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

	sentAt := time.Now()

	sendFrame(t, cliW, &acp.Message{
		JSONRPC: protocolVersion20, ID: json.RawMessage("2"), Method: methodSessPrmt,
		Params: rawJSON(map[string]any{
			keySessionID:  snew.SessionID,
			promptListKey: []any{map[string]any{keyType: blockText, textListKey: wiringAskCache}},
		}),
	})

	br := bufio.NewReader(cliR)

	var (
		gotResponse bool

		surfaceBeforeResponse bool

		all []*acp.Message
	)

	deadline := time.After(30 * time.Second)

	for !gotResponse {
		select {
		case <-deadline:
			t.Fatalf("no prompt response within 30s — a synchronous wait is holding it " +
				"(the response must return AT the suspension, ~instantly)")
		default:
		}

		line, rerr := br.ReadBytes('\n')
		if len(line) == 0 && rerr != nil {
			time.Sleep(10 * time.Millisecond)

			continue
		}

		var m acp.Message

		jerr := json.Unmarshal(bytes.TrimRight(line, "\n"), &m)
		if jerr == nil {
			if m.Method == sessionUpdate && strings.Contains(string(m.Params), "Which cache library") {
				surfaceBeforeResponse = true
			}

			all = append(all, &m)

			if string(m.ID) == "2" && m.Result != nil {
				gotResponse = true
			}
		}
	}

	if !surfaceBeforeResponse {
		t.Errorf("the ask surface did not precede the response (%d frames)", len(all))
	}

	if elapsed := time.Since(sentAt); elapsed > 10*time.Second {
		t.Errorf("response took %v; want near-instant (suspension return, not resolution)", elapsed)
	}

	// The ask is STILL unresolved: the response did not wait for the (1h)
	// timer or any reply.
	runner.sessMu.Lock()
	sess := runner.sessions[snew.SessionID]
	runner.sessMu.Unlock()

	if sess == nil || !sess.HasPendingAsk() {
		t.Error("no pending ask at response time — the response outlived the suspension")
	}

	// Drain the parked chain (test hygiene; also the cancel path's proof shape).
	closeErr := runner.CloseSession(snew.SessionID)
	if closeErr != nil {
		t.Fatalf("CloseSession: %v", closeErr)
	}

	idleCtx, idleCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer idleCancel()

	if !runner.WaitChainIdle(idleCtx, snew.SessionID) {
		t.Error("the parked chain did not go idle after CloseSession (goroutine leak)")
	}
}

// TestAskPark_CancelAndCloseDrainParkedChains (T4 pin 2 + W4): a parked chain
// (first-turn ask, 1h timer — parked indefinitely) is DRAINED by BOTH
// session-close routes: CloseSession (the logout path — 16-REVIEW CR-01 took
// session/cancel OFF this path: a cancelled LIVE turn drains through
// runOneTurn's request-ctx watchdog instead) and closeAllSessions (the serve
// end). No decision, no injection after the drain; the goroutine is released
// (chain count reaches zero).
func TestAskPark_CancelAndCloseDrainParkedChains(t *testing.T) { //nolint:funlen // two-route drain battery
	t.Parallel()

	t.Run("session close (cancel/logout route)", func(t *testing.T) {
		t.Parallel()

		r, _ := newExpansionRunner(t, true,
			scriptedResp{toolCalls: []provider.ToolCall{{
				ID: wiringAskCall, Name: wiringAskTool,
				Input: json.RawMessage(wiringAskInput),
			}}},
			scriptedResp{text: "acknowledged; done", finish: stopEndTurn},
		)

		r.askTimeout = time.Hour

		const sid = "sess-park-cancel"

		_, err := r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: wiringAskMe}})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		// WR-03: CloseSession EVICTS the cache entry, so the transcript reads
		// hold the session reference itself — the manager outlives the reap.
		parkSess := r.sessions[sid]
		if parkSess == nil {
			t.Fatal("Run constructed no session")
		}

		before := countEngineDecisions(t, parkSess)

		err = r.CloseSession(sid)
		if err != nil {
			t.Fatalf("CloseSession: %v", err)
		}

		idleCtx, idleCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer idleCancel()

		if !r.WaitChainIdle(idleCtx, sid) {
			t.Fatal("the parked chain did not drain after CloseSession (goroutine leak)")
		}

		// No post-drain decision/injection: the count is stable.
		if after := countEngineDecisions(t, parkSess); after != before {
			t.Errorf("engine_decision count went %d → %d after the cancel drain; "+
				"want stable (no decision, no injection)", before, after)
		}
	})

	t.Run("serve end (closeAllSessions route)", func(t *testing.T) {
		t.Parallel()

		r, _ := newExpansionRunner(t, true,
			scriptedResp{toolCalls: []provider.ToolCall{{
				ID: wiringAskCall, Name: wiringAskTool,
				Input: json.RawMessage(wiringAskInput),
			}}},
			scriptedResp{text: "acknowledged; done", finish: stopEndTurn},
		)

		r.askTimeout = time.Hour

		const sid = "sess-park-closeall"

		_, err := r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: wiringAskMe}})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		endSess := r.sessions[sid]
		if endSess == nil {
			t.Fatal("Run constructed no session")
		}

		before := countEngineDecisions(t, endSess)

		r.closeAllSessions()

		idleCtx, idleCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer idleCancel()

		if !r.WaitChainIdle(idleCtx, sid) {
			t.Fatal("the parked chain did not drain after closeAllSessions (goroutine leak)")
		}

		if after := countEngineDecisions(t, endSess); after != before {
			t.Errorf("engine_decision count went %d → %d after the serve-end drain; want stable", before, after)
		}
	})
}

// countEngineDecisions counts the session's engine_decision transcript lines.
// Takes the *session.Session (not the runner + id): CloseSession evicts the
// cache entry (WR-03), so post-close reads must hold the reference — the
// manager's ReadAll outlives the reap chain.
func countEngineDecisions(t *testing.T, sess *session.Session) int {
	t.Helper()

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	n := 0

	for i := range lines {
		if lines[i].Type == session.TypeEngineDecision {
			n++
		}
	}

	return n
}

// TestAskPark_ReplyDuringParkResumesAndQueuesInjection (T4 pin 3,
// serialization): a reply arriving while the chain is parked resolves the ask
// through routeAskReply (turnMu is FREE — Run returned at the suspension), the
// resumed turn completes, and the post-settle injection QUEUES BEHIND the
// reply's turn rather than overlapping (12-07 semantics; the transcript's
// turn ordering proves it).
func TestAskPark_ReplyDuringParkResumesAndQueuesInjection(t *testing.T) { //nolint:gocyclo,cyclop,funlen // ordering
	t.Parallel()

	r, prov := newExpansionRunner(t, true,
		scriptedResp{text: "exploration complete — handoff to propose", finish: stopEndTurn},
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: wiringAskCall, Name: wiringAskTool,
			Input: json.RawMessage(wiringAskInput),
		}}},
		scriptedResp{text: chainProposeDone, finish: stopEndTurn},
		scriptedResp{text: chainApplyDone, finish: stopEndTurn},
	)

	r.askTimeout = time.Hour // the REPLY must win — no timer competition

	cfg := &openspec.OpenSpecConfig{Patterns: []openspec.PatternEntry{
		{
			ID: postExploreRowID, Regex: chainHandoffPropose,
			Action: actionContinue, Next: "/opsx:propose park-subj",
		},
		{ID: postProposeRowID, Regex: handoffToApply, Action: actionContinue, Next: "/opsx:apply park-subj"},
	}}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	r.patternTable = pt

	const sid = "sess-park-reply"

	stop1, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "/opsx:explore park-subj"}})
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	if stop1 != stopEndTurn {
		t.Fatalf("Run 1 stop = %q; want end_turn (the suspension maps to a completed turn)", stop1)
	}

	// The chain is parked; the ask is pending.
	sess := r.sessions[sid]

	if !sess.HasPendingAsk() {
		t.Fatal("no pending ask after the suspension")
	}

	if got := r.chainCount(sid); got != 1 {
		t.Fatalf("chainCount = %d; want 1 (parked)", got)
	}

	// The operator's reply — an ordinary session/prompt.
	stop2, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "ristretto, please"}})
	if err != nil {
		t.Fatalf("Run 2 (reply): %v", err)
	}

	if stop2 != stopEndTurn {
		t.Fatalf("Run 2 stop = %q; want the resumed turn's end_turn", stop2)
	}

	// The parked chain completes: the resumed turn's continuation decision +
	// the queued apply injection.
	idleCtx, idleCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer idleCancel()

	if !r.WaitChainIdle(idleCtx, sid) {
		t.Fatal("the parked chain never went idle after the reply (10s)")
	}

	if got := prov.callCount(); got != 4 {
		t.Errorf("provider calls = %d; want 4 (explore + asking propose + resumed turn + queued apply)", got)
	}

	// ORDERING (the serialization pin): the apply injection's user_message
	// lands AFTER the resumed turn's assistant_message — queued, never
	// overlapped.
	lines, rerr := sess.Manager.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	resumedIdx, applyIdx := -1, -1

	askTurnID := ""

	for i := range lines {
		if lines[i].Type == session.TypeAskSuspended && askTurnID == "" {
			askTurnID = lines[i].TurnID
		}

		if lines[i].Type == session.TypeAssistantMessage && lines[i].TurnID == askTurnID {
			resumedIdx = i // the resumed turn's closing (last assistant line of the asking turn)
		}

		if applyIdx < 0 && lines[i].Type == session.TypeCommandProvenance && lines[i].Name == "opsx:apply" {
			applyIdx = i
		}
	}

	if resumedIdx < 0 {
		t.Fatal("the resumed turn's assistant message never landed (the reply resume failed)")
	}

	if applyIdx < 0 {
		t.Fatal("the apply injection never ran — the chain died at the ask (the park is broken)")
	}

	if applyIdx < resumedIdx {
		t.Errorf("apply injection (line %d) preceded the resumed turn's closing (line %d) — "+
			"overlapping turn drivers on one session", applyIdx, resumedIdx)
	}

	if got := lastUserMessageText(t, r, sid); !strings.Contains(got, "Apply the change:") {
		t.Errorf("last user_message = %q; want the expanded apply body", got)
	}
}

// wiringWaitFor polls cond until true or the deadline expires (the elicitation
// queue fires + resumes asynchronously — the pump goroutine owns the round
// trip; results-based waiting, never fixed sleeps).
func wiringWaitFor(t *testing.T, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(2 * time.Millisecond)
	}

	t.Fatal("condition never became true within 2s")
}

// TestAskWiring_ElicitationQueueRoundTrip pins 17-04 Task 3's family
// conversion end-to-end at the REAL wiring site: the AskUserQuestion
// suspension enqueues a FOREGROUND-class queue entry (one entry — the ONE
// firing path), the injected ask fire receives the questions-shaped payload,
// and the structured accept renders through the captured answered form into
// the resumed turn's tool result. The executor suspension contract (ErrSuspended
// + questions in Output, coreexec untouched) is observed intact: the first
// turn stops with the ask marker and appends NO tool result of its own.
func TestAskWiring_ElicitationQueueRoundTrip(t *testing.T) { //nolint:funlen,cyclop,gocyclo,gocognit,lll // flat end-to-end battery
	t.Parallel()

	r, prov := newAskWiringRunner(t, time.Hour)

	var (
		fireMu    sync.Mutex
		firedEnts []*session.AskEntry
	)

	// release holds the dialog OPEN until the test has verified the suspended
	// state (the fakeGateSurface hold discipline — a real human decides on a
	// human timescale; the instant accept would race Run 1's return).
	release := make(chan struct{})

	r.SetAskFire(func(_ context.Context, e *session.AskEntry) session.AskOutcome {
		fireMu.Lock()

		firedEnts = append(firedEnts, e)

		fireMu.Unlock()

		<-release

		return session.AskOutcome{
			Elicit:  acp.ElicitationActionAccept,
			Content: map[string]json.RawMessage{"q1": json.RawMessage(`"ristretto"`)},
		}
	})

	em := &noopEmitter{}

	stop1, err := r.Run(context.Background(), "sess-ask-elicit", em,
		[]acp.ContentBlock{{Type: blockText, Text: wiringAskMe}})
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	if stop1 != stopEndTurn {
		t.Fatalf("Run 1 stop = %q; want end_turn (the suspension maps to a completed turn)", stop1)
	}

	sess := r.sessions["sess-ask-elicit"]

	if sess == nil || !sess.HasPendingAsk() {
		t.Fatal("no pending ask after the suspending turn (the broker still owns the ask)")
	}

	// The suspension itself appended NO tool result — ErrSuspended contract.
	lines, rerr := sess.Manager.ReadAll()
	if rerr == nil {
		for i := range lines {
			if lines[i].Type == session.TypeToolResult && lines[i].ToolCallID == wiringAskCall {
				t.Fatalf("tool result landed before the resolution (callID %s)", wiringAskCall)
			}
		}
	}

	// Now answer: release the dialog; the accept resumes the SAME turn
	// (async to Run 1).
	close(release)

	// The queue fires + the accept resumes the SAME turn.
	wiringWaitFor(t, func() bool {
		lines, rerr := sess.Manager.ReadAll()
		if rerr != nil {
			return false
		}

		for i := range lines {
			if lines[i].Type == session.TypeToolResult && lines[i].ToolCallID == wiringAskCall {
				return true
			}
		}

		return false
	})

	fireMu.Lock()

	ents := append([]*session.AskEntry(nil), firedEnts...)

	fireMu.Unlock()

	if len(ents) != 1 {
		t.Fatalf("queue fired the surface %d times; want exactly one", len(ents))
	}

	if ents[0].Class != session.AskClassForeground {
		t.Errorf("entry class = %v; want foreground (D-11)", ents[0].Class)
	}

	var qs []session.AskQuestion

	uerr := json.Unmarshal(ents[0].Input, &qs)
	if uerr != nil || len(qs) != 1 {
		t.Fatalf("entry input = %s (%v); want the marshaled questions payload", ents[0].Input, uerr)
	}

	afterLines, aerr := sess.Manager.ReadAll()
	if aerr != nil {
		t.Fatalf("ReadAll: %v", aerr)
	}

	lines = afterLines

	var form string

	found := false

	for i := range lines {
		if lines[i].Type == session.TypeToolResult && lines[i].ToolCallID == wiringAskCall {
			ferr := json.Unmarshal(lines[i].Output, &form)
			if ferr != nil {
				t.Fatalf("unmarshal result form: %v", ferr)
			}

			found = true
		}
	}

	if !found {
		t.Fatal("no tool result for the suspended call after the accept")
	}

	want := session.RenderAskAnswered(qs, "ristretto")
	if form != want {
		t.Errorf("result form = %q; want the captured answered form %q", form, want)
	}

	// The resumed turn streamed through the provider (two stream calls total).
	// The result append and the first Stream are sequential WITHIN the resume,
	// but this test observes them through independent polls — under a loaded
	// -race run the result can be seen before the stream counter is (the
	// repo's known flake family), so wait for the stream rather than assume
	// its ordering against the previous ReadAll.
	deadline := time.Now().Add(5 * time.Second)

	var streams int

	for {
		prov.mu.Lock()
		streams = prov.calls
		prov.mu.Unlock()

		if streams >= 2 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("provider stream calls = %d; want 2 (suspend + resumed turn)", streams)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// TestAskWiring_EngineAskConversion pins the engine-ask glue (17-04): an ask-
// pending decision converts into a background elicitation entry whose accept
// writes the learning store through the EXISTING store API, and whose decline
// lands the advisory non-answer note through the subscriber-backed chunk path.
func TestAskWiring_EngineAskConversion(t *testing.T) { //nolint:funlen // one battery over the conversion glue
	t.Parallel()

	situation := "text:wiring-engine-situation"

	newRig := func(t *testing.T) (*Runner, *session.Session, *learning.Store) {
		t.Helper()

		st, lerr := learning.Open(filepath.Join(t.TempDir(), "learned.yaml"))
		if lerr != nil {
			t.Fatalf("learning open: %v", lerr)
		}

		r := &Runner{bus: event.NewBus(), learned: st}

		sess := &session.Session{SessionID: "sess-engine-w"}
		sess.SetPermissionGate(session.GateDeps{Queue: session.NewAskQueue()})

		return r, sess, st
	}

	t.Run("accept_writes_learning_entry", func(t *testing.T) {
		t.Parallel()

		r, sess, st := newRig(t)
		r.SetAskFire(func(_ context.Context, _ *session.AskEntry) session.AskOutcome {
			return session.AskOutcome{
				Elicit:  acp.ElicitationActionAccept,
				Content: map[string]json.RawMessage{"q1": json.RawMessage(`"proceed to propose"`)},
			}
		})

		r.enqueueEngineAsk(sess, "turn-e1", situation)

		// The accept lands synchronously inside the resolution; poll the store.
		wiringWaitFor(t, func() bool {
			_, ok := st.Lookup(situation)

			return ok
		})

		e, _ := st.Lookup(situation)
		if e.Answer != "proceed to propose" {
			t.Errorf("stored answer = %q; want the structured seam's single-field value", e.Answer)
		}
	})

	t.Run("decline_lands_advisory_note", func(t *testing.T) {
		t.Parallel()

		r, sess, st := newRig(t)
		r.SetAskFire(func(_ context.Context, _ *session.AskEntry) session.AskOutcome {
			return session.AskOutcome{Elicit: acp.ElicitationActionDecline}
		})

		chunks := r.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
		defer r.bus.Unsubscribe("AgentMessageChunk", chunks)

		r.enqueueEngineAsk(sess, "turn-e2", situation)

		deadline := time.After(2 * time.Second)

		for {
			select {
			case e := <-chunks:
				if c, ok := e.(event.AgentMessageChunk); ok && c.Content == advisoryNoteText {
					// The advisory note landed subscriber-backed.
					if _, stored := st.Lookup(situation); stored {
						t.Error("decline must not write the learning store")
					}

					return
				}
			case <-deadline:
				t.Fatal("the advisory note never landed on a decline")
			}
		}
	})
}
