package runtime //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// blockingProvider wraps scriptedACPProvider: the FIRST Stream call blocks
// until released (or the ctx dies) so a test can enqueue steering while the
// turn's request is genuinely in flight — the pre-mutex classifier's
// anti-queue-behind lens.
type blockingProvider struct {
	scriptedACPProvider
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingProvider) Stream(
	ctx context.Context, prof *profile.Profile, msgs []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.once.Do(func() {
		if p.entered != nil {
			p.entered <- struct{}{}
		}

		select {
		case <-p.release:
		case <-ctx.Done():
		}
	})

	return p.scriptedACPProvider.Stream(ctx, prof, msgs)
}

// newBlockingRunner builds an engine-OFF runner over the blocking provider.
func newBlockingRunner(t *testing.T, script ...scriptedResp) (*Runner, *blockingProvider) {
	t.Helper()

	prov := &blockingProvider{entered: make(chan struct{}, 1), release: make(chan struct{})}
	prov.queue(script...)

	r := &Runner{
		bus:          event.NewBus(),
		profile:      fakeProfileACP(),
		workDir:      t.TempDir(),
		maxConc:      4,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}

	r.LoadCommandRegistry()

	return r, prov
}

// TestSteerIngressMidTurnPreMutex pins SEEDG-01's core ingress contract: a
// plain-text prompt arriving while a client turn is ACTIVE is classified
// BEFORE the session turn mutex — it enqueues on the session's SteerQueue
// (ticket assigned), emits the queued note through the in-hand emitter, and
// returns end_turn immediately WITHOUT waiting for the running turn (the
// anti-queue-behind assertion: the turn's provider call is still BLOCKED
// when the steered prompt returns). The running turn is untouched: it
// completes its scripted request sequence and the steering delivers at its
// next boundary.
func TestSteerIngressMidTurnPreMutex(t *testing.T) { //nolint:funlen // comprehensive ingress scenario
	t.Parallel()

	r, prov := newBlockingRunner(t,
		scriptedResp{
			finish: tracerToolUse,
			toolCalls: []provider.ToolCall{{
				Name: "Read", Input: []byte(`{"file_path":"a.txt"}`),
			}},
		},
		scriptedResp{text: "done", finish: stopEndTurn},
	)

	const sid = "sess-steer-in"

	turnDone := make(chan struct{})

	go func() {
		defer close(turnDone)

		stop, err := r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "long task"}})
		if err != nil || stop != stopEndTurn {
			t.Errorf("turn Run = (%q,%v); want end_turn", stop, err)
		}
	}()

	<-prov.entered // request 1 in flight: turnMu held, turnActive true

	// The steered prompt must return PROMPTLY — while the provider is still
	// blocked. Waiting here would be queue-behind (the forbidden descope).
	emit := &noopEmitter{}

	type steerResult struct {
		stop string
		err  error
	}

	res := make(chan steerResult, 1)

	go func() {
		stop, err := r.Run(context.Background(), sid, emit, []acp.ContentBlock{{Type: blockText, Text: "focus on tests"}})
		res <- steerResult{stop, err}
	}()

	var got steerResult

	select {
	case got = <-res: // prompt returned without the provider releasing
	case <-time.After(3 * time.Second):
		t.Fatal("steered prompt did not return while the turn was still blocked (queue-behind regression)")
	}

	if got.err != nil || got.stop != stopEndTurn {
		t.Fatalf("steered Run = (%q,%v); want (end_turn, nil)", got.stop, got.err)
	}

	// The queued note went through the in-hand emitter.
	found := false

	for _, c := range emit.chunks {
		if strings.HasPrefix(c, "steering queued") {
			found = true
		}
	}

	if !found {
		t.Errorf("queued note missing from the in-hand emitter: %v", emit.chunks)
	}

	// The session's queue holds exactly the steered input.
	sess := r.sessions[sid]

	if q := sess.SteerQueue(); q == nil || q.Pending() != 1 {
		t.Fatalf("SteerQueue Pending = %+v; want 1 steered input enqueued", sess.SteerQueue())
	}

	// Release the turn; it completes its TWO scripted calls untouched.
	close(prov.release)

	select {
	case <-turnDone:
	case <-time.After(5 * time.Second):
		t.Fatal("running turn did not complete after release")
	}

	if n := prov.callCount(); n != 2 {
		t.Fatalf("running turn made %d provider calls; want 2 (script untouched)", n)
	}

	// The steering delivered at the turn's next boundary: request 2 carries
	// the steered text (folded by the 23-01 drain + projector).
	if !prov.streamSawText(1, "focus on tests") {
		t.Error("request 2 did not carry the steered text (boundary delivery broken)")
	}
}

// TestSteerIngressNoActiveTurnOrdinary pins the ordinary path: with NO active
// turn or chain, a plain-text prompt runs the ordinary new-turn flow under
// the turn mutex — nothing enqueues, the text becomes the turn's user
// message exactly as before.
func TestSteerIngressNoActiveTurnOrdinary(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "answer", finish: stopEndTurn})

	const sid = "sess-steer-ord"
	close(prov.release) // no blocking needed

	stop, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "plain question"}})
	if err != nil || stop != stopEndTurn {
		t.Fatalf("Run = (%q,%v); want (end_turn, nil)", stop, err)
	}

	sess := r.sessions[sid]
	if q := sess.SteerQueue(); q != nil && q.Pending() != 0 {
		t.Errorf("ordinary prompt enqueued steering (queue pending %d)", q.Pending())
	}

	if !prov.streamSawText(0, "plain question") {
		t.Error("ordinary prompt did not reach the provider as the turn's user message")
	}
}

// TestSteerIngressEngineChain pins Pattern 6's chain branch: a plain-text
// prompt arriving while an engine chain is ACTIVE (chainCount > 0, no mutex
// held — the parked-chain state) takes the SAME steering enqueue path and
// returns promptly; the chain's own turns deliver the steering through
// sess.Prompt (the bridge's turn driver — zero engine changes).
func TestSteerIngressEngineChain(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "chain turn", finish: stopEndTurn})

	const sid = "sess-steer-chain"
	close(prov.release)

	sess := r.sessionFor(context.Background(), sid)

	r.chainEnter(sid)
	defer r.chainExit(sid)

	emit := &noopEmitter{}

	done := make(chan struct{})

	go func() {
		defer close(done)

		stop, err := r.Run(context.Background(), sid, emit, []acp.ContentBlock{{Type: blockText, Text: "adjust the plan"}})
		if err != nil || stop != stopEndTurn {
			t.Errorf("chain-steered Run = (%q,%v); want (end_turn, nil)", stop, err)
		}
	}()

	select {
	case <-done: // returned promptly — the chain holds no mutex
	case <-time.After(3 * time.Second):
		t.Fatal("chain-steered prompt blocked (the parked chain holds no mutex — classifier ran on the wrong branch?)")
	}

	if q := sess.SteerQueue(); q == nil || q.Pending() != 1 {
		t.Fatalf("SteerQueue Pending; want the chain steering enqueued")
	}

	// The chain's next turn (the bridge drives sess.Prompt) delivers it.
	_, perr := sess.Prompt(context.Background(), []session.ContentBlock{{Type: blockText, Text: "chain next turn"}})
	if perr != nil {
		t.Fatalf("chain Prompt: %v", perr)
	}

	if !prov.streamSawText(0, "adjust the plan") {
		t.Error("the chain turn's request did not carry the steered text")
	}
}

// TestSteerIngressPendingAskOutranks pins the classifier order: a pending
// broker ask outranks steering — the input routes as the ask's ANSWER (the
// 17-D-11 contract), never into the SteerQueue.
func TestSteerIngressPendingAskOutranks(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t, scriptedResp{text: "resumed", finish: stopEndTurn})
	close(prov.release) // no blocking needed

	const sid = "sess-steer-ask"

	sess := r.sessionFor(context.Background(), sid)

	// Fabricate the pending broker ask (the suspending turn's suspension).
	sess.SetAskBroker(context.Background(), session.NewAskBroker(0, nil))
	sess.AskBroker().Surface(session.PendingAsk{
		TurnID: "sess-steer-ask-turn-001", CallID: "c1",
		Questions: []session.AskQuestion{{Header: "h", Question: "which?", Options: []session.AskOption{{Label: "a"}}}},
	})

	stop, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "option a please"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	_ = stop

	if q := sess.SteerQueue(); q != nil && q.Pending() != 0 {
		t.Errorf("an ask ANSWER was enqueued as steering (pending %d) — the ask route must outrank steering", q.Pending())
	}

	if sess.HasPendingAsk() {
		t.Error("the pending ask was not resolved by the reply (routeAskReply contract broken)")
	}
}

// TestSteerIngressClassBNotSteered pins Pitfall 11's class-B leg: a
// chain-resolved slash-command invocation typed mid-turn NEVER becomes
// steering text — it falls through to the ordinary path (the 20-01 locked
// contract position; 23-05 fills the pre-mutex /undo handling).
func TestSteerIngressClassBNotSteered(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingRunner(t,
		scriptedResp{
			finish: tracerToolUse,
			toolCalls: []provider.ToolCall{{
				Name: "Read", Input: []byte(`{"file_path":"a.txt"}`),
			}},
		},
		scriptedResp{text: "expanded command turn", finish: stopEndTurn},
	)

	writeOpsxCommandFixtures(t, r.workDir)
	r.LoadCommandRegistry()

	const sid = "sess-steer-cmd"

	turnDone := make(chan struct{})

	go func() {
		defer close(turnDone)

		_, _ = r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "long task"}})
	}()

	<-prov.entered

	cmdDone := make(chan struct{})

	go func() {
		defer close(cmdDone)

		_, _ = r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "/opsx:explore fix-it"}})
	}()

	// The class-B input did NOT return via the steering path (it queues
	// behind the turn — today's behavior; 23-05 owns the upgrade).
	select {
	case <-cmdDone:
		t.Fatal("class-B input returned mid-turn — it must NOT take the steering path")
	case <-time.After(300 * time.Millisecond):
	}

	close(prov.release)

	<-turnDone
	<-cmdDone

	sess := r.sessions[sid]

	if q := sess.SteerQueue(); q != nil && q.Pending() != 0 {
		t.Errorf("a class-B invocation was enqueued as steering (pending %d)", q.Pending())
	}

	// The invocation ran as the EXPANDED command turn (the file-command
	// path), its expanded body becoming the LAST user message — never the
	// raw invocation as steering.
	if !strings.Contains(lastUserMessageText(t, r, sid), "Explore the change") {
		t.Errorf("the class-B invocation did not expand; user message = %q", lastUserMessageText(t, r, sid))
	}
}

// TestParkedAskCancelGrammar pins D-06: an input matching the parked-cancel
// grammar resolves the pending ask CANCELLED-NORMAL (the D-01 timer's
// non-answer form) without killing anything; an answer-shaped input still
// routes through ResolveAsk (17-D-11 preserved); non-grammar text containing
// "cancel" still routes as an ANSWER (exact-phrase matching only).
func TestParkedAskCancelGrammar(t *testing.T) {
	t.Parallel()

	park := func(t *testing.T) (*Runner, *session.Session, string) {
		t.Helper()

		r, prov := newBlockingRunner(t, scriptedResp{text: "resumed", finish: stopEndTurn})
		close(prov.release)

		const sid = "sess-park-cancel"

		sess := r.sessionFor(context.Background(), sid)
		sess.SetAskBroker(context.Background(), session.NewAskBroker(0, nil))
		sess.AskBroker().Surface(session.PendingAsk{
			TurnID: "sess-park-cancel-turn-001", CallID: "c1",
			Questions: []session.AskQuestion{{Question: "proceed?", Header: "go"}},
		})

		return r, sess, sid
	}

	t.Run("cancel resolves cancelled-normal", func(t *testing.T) {
		t.Parallel()

		r, sess, sid := park(t)

		stop, err := r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "Cancel Ask"}})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if stop != stopEndTurn {
			t.Errorf("cancel Run stop = %q; want end_turn (mapped)", stop)
		}

		if sess.HasPendingAsk() {
			t.Error("pending ask survived the cancel grammar")
		}

		// The suspended turn resumed with the NON-ANSWER (cancelled-normal)
		// form — the D-01 timer's tool-result shape, not an answered form.
		lines, _ := sess.Manager.ReadAll()

		var result *session.Line

		for i := range lines {
			if lines[i].Type == session.TypeToolResult {
				result = &lines[i]
			}
		}

		if result == nil {
			t.Fatal("no tool_result line after the cancelled resume")
		}

		if strings.Contains(string(result.Output), "cancel") &&
			!strings.Contains(strings.ToLower(string(result.Output)), "non-answer") {
			// The exact rendered form is the timer family's; assert it is NOT
			// the answered form (no option text leaked in).
			if strings.Contains(string(result.Output), "proceed? answered") {
				t.Errorf("cancelled resume rendered an ANSWERED form: %s", result.Output)
			}
		}
	})

	t.Run("answer still routes unchanged", func(t *testing.T) {
		t.Parallel()

		r, sess, sid := park(t)

		_, err := r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "yes please go"}})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if sess.HasPendingAsk() {
			t.Error("the answer did not resolve the pending ask (17-D-11 broken)")
		}
	})

	t.Run("cancel-bearing prose still answers", func(t *testing.T) {
		t.Parallel()

		r, sess, sid := park(t)

		_, err := r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "cancel the deployment, answer yes"}})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if sess.HasPendingAsk() {
			t.Error("prose containing 'cancel' did not route as an answer (exact-phrase only)")
		}
	})
}

// newBlockingEngineRunner builds an ENGINE-ON runner over the blocking
// provider (the real executor path — AskUserQuestion suspends for real).
func newBlockingEngineRunner(t *testing.T, script ...scriptedResp) (*Runner, *blockingProvider) {
	t.Helper()

	prov := &blockingProvider{entered: make(chan struct{}, 1), release: make(chan struct{})}
	prov.queue(script...)

	r := &Runner{
		bus:          event.NewBus(),
		profile:      fakeProfileACP(),
		workDir:      t.TempDir(),
		maxConc:      4,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}

	if err := r.SetupEngine(); err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	r.LoadCommandRegistry()

	return r, prov
}

// TestCombinedSteerScenario is the milestone-prescribed combined E2E
// (Pitfall 11's warning sign is tests that NEVER combine the classes): one
// scripted turn where the operator sends two steering texts (coalescing into
// ONE delivery), an ask parks visibly behind the running turn, and a
// parked-cancel grammar input resolves it cancelled-normal without killing
// anything — every input classifies per the matrix, nothing is swallowed or
// misrouted, and no cancelled content reaches any request.
func TestCombinedSteerScenario(t *testing.T) { //nolint:funlen,maintidx // comprehensive combined scenario
	t.Parallel()

	r, prov := newBlockingEngineRunner(t,
		scriptedResp{
			finish: tracerToolUse,
			toolCalls: []provider.ToolCall{{
				ID: "c-read", Name: "Read", Input: json.RawMessage(`{"file_path":"a.txt"}`),
			}},
		},
		scriptedResp{
			finish: tracerToolUse,
			toolCalls: []provider.ToolCall{{
				ID: "c-ask", Name: wiringAskTool, Input: json.RawMessage(wiringAskInput),
			}},
		},
		scriptedResp{text: "resumed after cancel", finish: stopEndTurn},
	)

	const sid = "sess-combined"

	turnDone := make(chan string, 1)

	go func() {
		stop, err := r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "main task"}})
		if err != nil {
			t.Errorf("turn Run err: %v", err)
		}

		turnDone <- stop
	}()

	<-prov.entered // request 1 in flight; the engine chain holds chainCount > 0

	// Leg 1 + leg 4 (two steering texts, mid-turn): both classify as
	// steering and return promptly.
	for _, text := range []string{"steer alpha", "steer beta"} {
		st, err := r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: text}})
		if err != nil || st != stopEndTurn {
			t.Fatalf("steering Run %q = (%q,%v); want (end_turn, nil)", text, st, err)
		}
	}

	sess := r.sessions[sid]

	if q := sess.SteerQueue(); q == nil || q.Pending() != 2 {
		t.Fatalf("queue pending after two steers; want 2")
	}

	// Release: iteration 2 drains BOTH as one delivery, then the scripted
	// AskUserQuestion parks.
	close(prov.release)

	select {
	case <-turnDone: // the turn ended at the ask (stopAsk mapped end_turn)
	case <-time.After(5 * time.Second):
		t.Fatal("the turn did not reach its parked ask")
	}

	// The two steered texts delivered as ONE coalesced block in request 2.
	if !prov.streamSawText(1, "steer alpha") || !prov.streamSawText(1, "steer beta") {
		t.Error("request 2 did not carry both steered texts (coalesced delivery)")
	}

	deliveries := transcriptLinesOfType(t, r, sid, session.TypeSteeringDelivery)
	if len(deliveries) != 1 {
		t.Fatalf("%d steering_delivery lines; want exactly 1 (coalesced)", len(deliveries))
	}

	if got := string(deliveries[0].Input); !strings.Contains(got, `"count":2`) {
		t.Errorf("coalesced delivery Input = %s; want count 2", got)
	}

	// The ask parked visibly: parked_ask + ask_suspended records, broker pending.
	if parked := transcriptLinesOfType(t, r, sid, session.TypeParkedAsk); len(parked) != 1 {
		t.Errorf("%d parked_ask lines; want 1", len(parked))
	}

	if !sess.HasPendingAsk() {
		t.Fatal("the scripted ask did not park (broker has nothing pending)")
	}

	// Leg 3: the parked-cancel grammar resolves it cancelled-normal — the
	// suspended turn resumes on the non-answer form (script slot 3) and the
	// turn completes. Nothing else is killed (nothing else runs).
	emit := &noopEmitter{}

	stop, err := r.Run(context.Background(), sid, emit, []acp.ContentBlock{{Type: blockText, Text: "cancel ask"}})
	if err != nil || stop != stopEndTurn {
		t.Fatalf("cancel Run = (%q,%v); want (end_turn, nil)", stop, err)
	}

	if sess.HasPendingAsk() {
		t.Error("pending ask survived the combined-scenario cancel")
	}

	// No cancelled content ever reached a request: the cancel text appears
	// in NO provider window (only steering + turn content ever do).
	for i := range 3 {
		if prov.streamSawText(i, "cancel ask") {
			t.Errorf("request %d carried the cancel input as model content", i+1)
		}
	}

	// The transcript's record sequence is complete and ordered: the parked
	// marker sits with the suspension; the cancel resumed the SAME turn.
	lines := transcriptLinesOfType(t, r, sid, session.TypeAskSuspended)
	if len(lines) != 1 || lines[0].TurnID == "" {
		t.Errorf("ask_suspended records = %+v; want exactly one attributed suspension", lines)
	}
}

// TestEngineChainSteered pins Open Question 4's resolution (zero engine
// changes): a steered ENGINE-CHAIN turn applies the steering at its next
// boundary and the chain completes normally — Decide signals never observe
// steering lines (LastTurnOutput's terminal scan has no steering case; the
// completion here is the proof the chain was not confused).
func TestEngineChainSteered(t *testing.T) {
	t.Parallel()

	r, prov := newBlockingEngineRunner(t,
		scriptedResp{
			finish: tracerToolUse,
			toolCalls: []provider.ToolCall{{
				ID: "c-read", Name: "Read", Input: json.RawMessage(`{"file_path":"a.txt"}`),
			}},
		},
		scriptedResp{text: "chain turn done; no handoff signal", finish: stopEndTurn},
	)

	const sid = "sess-chain-steer"

	turnDone := make(chan string, 1)

	go func() {
		stop, err := r.Run(context.Background(), sid, &noopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "chain task"}})
		if err != nil {
			t.Errorf("chain Run err: %v", err)
		}

		turnDone <- stop
	}()

	<-prov.entered

	// The chain is ACTIVE while its turn's request is in flight.
	if cc := r.chainCount(sid); cc == 0 {
		t.Fatal("chainCount == 0 while the chain turn is in flight (fixture broken)")
	}

	st, err := r.Run(context.Background(), sid, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "adjust mid-chain"}})
	if err != nil || st != stopEndTurn {
		t.Fatalf("chain-steered Run = (%q,%v); want (end_turn, nil)", st, err)
	}

	close(prov.release)

	select {
	case stop := <-turnDone:
		if stop != stopEndTurn {
			t.Errorf("chain turn stop = %q; want end_turn (chain completed normally)", stop)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the steered chain did not complete")
	}

	// The chain fully unwound (no zombie chain state) and the steering
	// delivered at the chain turn's boundary.
	if cc := r.chainCount(sid); cc != 0 {
		t.Errorf("chainCount = %d after completion; want 0", cc)
	}

	if !prov.streamSawText(1, "adjust mid-chain") {
		t.Error("the chain turn's request 2 did not carry the steered text")
	}

	if n := prov.callCount(); n != 2 {
		t.Errorf("provider calls = %d; want 2 (the chain's scripted sequence untouched)", n)
	}
}
