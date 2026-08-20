package session //nolint:testpackage // internal package test (drives the real tool loop + broker)

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
	"github.com/Djarvur/ass-guard-agent/internal/toolexec"
)

// askInput is the captured AskUserQuestion input shape used across the ask
// battery (one question, two labelled options — the plan's Test-1 scenario).
const askInput = `{"questions":[{"question":"Which cache library should we use?",` +
	`"header":"Cache",` +
	`"options":[{"label":"ristretto","description":"fast in-memory cache"},` +
	`{"label":"bigcache","description":"simple disk-backed cache"}]}]}`

// Test-local tool + call ids + shared literals (goconst).
const (
	askToolNameTest = "AskUserQuestion"
	askCallID1      = "call_ask_1"
	askCallID2      = "call_ask_2"
	askPromptText   = "add a cache"
	settleStateSet  = "settled"
	settleStateOpen = "open"
)

// newAskSession builds a catalog-wired Session whose AskUserQuestion entry
// carries the suspension executor (a local twin of coreexec.AskUserQuestionExecute
// — parse → questions on the Output → ErrSuspended; the layering keeps
// internal/session's in-package tests free of the coreexec import — Go rejects
// that cycle — and the REAL executor is proven end-to-end by coreexec's suite +
// the cmd/ass-guard wiring test), plus a broker whose surface callback records
// every surfaced pending ask.
func newAskSession(
	t *testing.T, responses []provider.Response, timeout time.Duration,
) (*Session, *AskBroker, *[]PendingAsk) {
	t.Helper()

	s := newTestSessionWithCatalog(t, responses)

	surfaced := &[]PendingAsk{}

	broker := NewAskBroker(timeout, func(p PendingAsk) {
		*surfaced = append(*surfaced, p)
	})

	s.SetAskBroker(context.Background(), broker)

	s.Catalog.Register(toolcat.Tool{
		Name:        askToolNameTest,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"questions":{"type":"array"}}}`),
		Execute: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			var a struct {
				Questions []AskQuestion `json:"questions"`
			}

			_ = json.Unmarshal(args, &a)

			out, _ := json.Marshal(a.Questions)

			return out, ErrSuspended
		},
	})

	// The production dispatch path: the catalog-backed RealExecutor (an
	// Execute-nil entry would return the canned no-implementation wall).
	s.SetToolExecutor(&toolexec.RealExecutor{Catalog: s.Catalog})

	return s, broker, surfaced
}

// TestAsk_SuspendsAndTimesOutEndToEnd (12-01 T1 Test 1): a model turn calling
// AskUserQuestion SURFACES the question (the callback receives question +
// options in order), the turn ENDS with the ask stop marker, the transcript
// records the suspension with NO tool result for the pending callID, then the
// D-01 timer (50ms here) fires, the turn RESUMES, the pending callID receives
// the corpus-absent-flagged non-answer form as its tool result, and the model's
// continuation completes the turn normally.
func TestAsk_SuspendsAndTimesOutEndToEnd(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen // flat battery
	t.Parallel()

	s, broker, surfaced := newAskSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				ID: askCallID1, Name: askToolNameTest,
				Input: json.RawMessage(askInput),
			}},
		},
		{FinishReason: stopEndTurn}, // the resumed iteration closes the turn
	}, 50*time.Millisecond)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: askPromptText}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want the ask-suspension marker %q", stop, stopAsk)
	}

	// The surfaced callback received the question + options in order.
	if len(*surfaced) != 1 {
		t.Fatalf("surfaced asks = %d; want 1", len(*surfaced))
	}

	p := (*surfaced)[0]

	if len(p.Questions) != 1 || p.Questions[0].Question != "Which cache library should we use?" {
		t.Errorf("surfaced question = %+v; want the parsed question", p.Questions)
	}

	if len(p.Questions[0].Options) != 2 ||
		p.Questions[0].Options[0].Label != "ristretto" ||
		p.Questions[0].Options[1].Label != "bigcache" {
		t.Errorf("surfaced options = %+v; want both labels in order", p.Questions[0].Options)
	}

	// The transcript records the suspension; NO tool result exists yet for the
	// pending callID (the result is appended by the resume path only).
	lines := linesOf(s)

	sawSuspension := false

	sawResult := false

	for _, l := range lines {
		if l.Type == TypeAskSuspended && l.ToolCallID == askCallID1 {
			sawSuspension = true
		}

		if l.Type == TypeToolResult && l.ToolCallID == askCallID1 {
			sawResult = true
		}
	}

	if !sawSuspension {
		t.Fatal("transcript missing the ask_suspended suspension record")
	}

	if sawResult {
		t.Fatal("tool result appended for the suspended callID before the resume " +
			"(the suspension must NOT append a result)")
	}

	// The D-01 timer fires autonomously and drives the SAME turn's resume: the
	// pending callID receives the non-answer form; the model's continuation
	// completes the turn (assistant message recorded; pending cleared).
	deadline := time.Now().Add(5 * time.Second)

	resumed := false

	for time.Now().Before(deadline) {
		for _, l := range linesOf(s) {
			if l.Type == TypeAssistantMessage && l.TurnID == p.TurnID {
				resumed = true
			}
		}

		if resumed {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	if !resumed {
		t.Fatal("the D-01 timer did not resume the turn (no assistant message for the suspended turn)")
	}

	if broker.Pending() {
		t.Error("broker still pending after the timeout resume")
	}

	for _, l := range linesOf(s) {
		if l.Type != TypeToolResult || l.ToolCallID != askCallID1 {
			continue
		}

		var got string

		err := json.Unmarshal(l.Output, &got)
		if err != nil {
			t.Fatalf("non-answer output is not a JSON string: %v (%s)", err, l.Output)
		}

		if !strings.HasPrefix(got, "User has not answered your questions") {
			t.Errorf("non-answer form = %q; want the corpus-absent-flagged D-01 default wrapper", got)
		}

		if !strings.Contains(got, "50ms") {
			t.Errorf("non-answer form = %q; want the configured timeout rendered", got)
		}

		return
	}

	t.Fatal("the resumed turn never appended the tool result for the pending callID")
}

// TestAsk_ReplyResumesSameTurn (the reply-routing half of the suspension seam):
// after a suspension, ResolveAsk embeds the operator's reply verbatim in the
// captured answered form as the pending call's tool result and re-enters the
// SAME turn's model loop — the reply IS the tool result; no new user message.
func TestAsk_ReplyResumesSameTurn(t *testing.T) { //nolint:cyclop,funlen // flat battery
	t.Parallel()

	s, broker, _ := newAskSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				ID: askCallID2, Name: askToolNameTest,
				Input: json.RawMessage(askInput),
			}},
		},
		{FinishReason: stopEndTurn},
	}, time.Hour) // long timeout: the reply must win, not the timer

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: askPromptText}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want %q", stop, stopAsk)
	}

	userMsgs := 0

	for _, l := range linesOf(s) {
		if l.Type == TypeUserMessage {
			userMsgs++
		}
	}

	stop2, err := s.ResolveAsk(context.Background(), "ristretto please")
	if err != nil {
		t.Fatalf("ResolveAsk: %v", err)
	}

	if stop2 != stopEndTurn {
		t.Errorf("resumed turn stop = %q; want end_turn (the model continuation completes)", stop2)
	}

	if broker.Pending() {
		t.Error("broker still pending after the reply resume")
	}

	// The reply lands as the tool result in the captured answered form.
	for _, l := range linesOf(s) {
		if l.Type != TypeToolResult || l.ToolCallID != askCallID2 {
			continue
		}

		var got string

		err := json.Unmarshal(l.Output, &got)
		if err != nil {
			t.Fatalf("answered output is not a JSON string: %v (%s)", err, l.Output)
		}

		const want = `User has answered your questions: "Which cache library should we use?"="ristretto please"` +
			`. You can now continue with the user's answers in mind.`
		if got != want {
			t.Errorf("answered form = %q; want the captured shape %q", got, want)
		}

		// No new user message: the reply was routed as the tool result.
		after := 0

		for _, l2 := range linesOf(s) {
			if l2.Type == TypeUserMessage {
				after++
			}
		}

		if after != userMsgs {
			t.Errorf("user messages went %d → %d; the reply must NOT append a user message", userMsgs, after)
		}

		return
	}

	t.Fatal("the reply never landed as the pending call's tool result")
}

// TestAsk_ResolveWithoutPending (T-12-01-01 hygiene): resolving with nothing
// pending reports false — a stray prompt after a timeout resume is an ordinary
// new turn, never a cross-turn injection.
func TestAsk_ResolveWithoutPending(t *testing.T) {
	t.Parallel()

	s, _, _ := newAskSession(t, []provider.Response{{FinishReason: stopEndTurn}}, time.Hour)

	_, err := s.ResolveAsk(context.Background(), "orphan reply")
	if err == nil {
		t.Fatal("ResolveAsk with nothing pending must fail (the caller falls back to a normal turn)")
	}
}

// TestAsk_TimeoutDefaults (12-01 T1 Test 2, the D-01 policy): a negative
// timeout normalizes to the 10-minute default; ZERO arms NO timer — the pending
// ask survives until a reply or session close (block-forever interactive mode).
func TestAsk_TimeoutDefaults(t *testing.T) {
	t.Parallel()

	if DefaultAskTimeout != 10*time.Minute {
		t.Errorf("DefaultAskTimeout = %v; want 10m (D-01)", DefaultAskTimeout)
	}

	if got := NewAskBroker(-1, nil).Timeout(); got != DefaultAskTimeout {
		t.Errorf("negative timeout = %v; want the 10m default", got)
	}

	if got := NewAskBroker(0, nil).Timeout(); got != 0 {
		t.Errorf("zero timeout = %v; want 0 (block forever, D-01 interactive mode)", got)
	}

	// 0 arms no timer: Surface leaves a pending ask that survives past any
	// test-scale interval (proven by the broker still pending + no resume).
	b := NewAskBroker(0, nil)

	b.Surface(PendingAsk{TurnID: "t", CallID: "c"})

	if !b.Pending() {
		t.Fatal("block-forever broker lost its pending ask at Surface time")
	}
}

// TestAsk_CloseDisarmsTimer (Task-2's no-goroutine-leak contract at the
// session level): closing a session with an armed ask timer disarms it — the
// timeout hook never runs after Close.
func TestAsk_CloseDisarmsTimer(t *testing.T) {
	t.Parallel()

	s := newTestSessionWithCatalog(t, []provider.Response{{FinishReason: stopEndTurn}})

	fired := make(chan struct{}, 1)

	b := NewAskBroker(20*time.Millisecond, nil)
	b.SetOnTimeout(func(PendingAsk) { fired <- struct{}{} })

	s.SetAskBroker(context.Background(), b)

	b.Surface(PendingAsk{TurnID: "t", CallID: "c"})

	err := s.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-fired:
		t.Fatal("the ask timer fired after session Close (leak: Close must disarm it)")
	case <-time.After(80 * time.Millisecond):
		// good: disarmed before firing
	}
}

// TestAsk_BrokerClaimRace (T-12-01-01): exactly ONE of {reply, timeout} claims
// a pending ask — the loser observes no pending and must not inject into the
// resumed turn.
func TestAsk_BrokerClaimRace(t *testing.T) {
	t.Parallel()

	b := NewAskBroker(time.Hour, nil)
	b.Surface(PendingAsk{TurnID: "t", CallID: "c"})

	p1, ok1 := b.Claim()
	if !ok1 {
		t.Fatal("first claim failed")
	}

	p2, ok2 := b.Claim()
	if ok2 {
		t.Fatal("second claim succeeded — double resume would inject into the same turn")
	}

	if p1.CallID != "c" || p2.CallID != "" || p2.TurnID != "" {
		t.Errorf("claims = (%+v, %+v); want the first claimant to win with the payload", p1, p2)
	}
}

// --- 13-00 T2: the per-suspension settle seam (both resume drivers close it) ---

// settleReport probes ch without blocking: settleStateSet when closed (or nil —
// nothing ever armed, indistinguishable from settled for wait-first callers),
// settleStateOpen when a suspension is still in flight.
func settleReport(ch <-chan struct{}) string {
	select {
	case <-ch:
		return settleStateSet
	default:
		return settleStateOpen
	}
}

// TestAsk_Settle_TimerResumeSettles (13-00 T2 test 1): while the ask is PENDING
// the settle channel is open; after the D-01 timer fires AND the resumed turn
// completes, it is closed — the timer driver's completion is observable.
func TestAsk_Settle_TimerResumeSettles(t *testing.T) {
	t.Parallel()

	s, _, _ := newAskSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				ID: askCallID1, Name: askToolNameTest,
				Input: json.RawMessage(askInput),
			}},
		},
		{FinishReason: stopEndTurn},
	}, 50*time.Millisecond)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: askPromptText}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want %q", stop, stopAsk)
	}

	ch := s.AskSettleChan()

	if ch == nil {
		t.Fatal("AskSettleChan returned nil while the ask is pending — the suspension did not arm the settle signal")
	}

	if got := settleReport(ch); got != settleStateOpen {
		t.Fatalf("settle channel %q while PENDING; want open (the resume has not even started)", got)
	}

	deadline := time.Now().Add(5 * time.Second)

	for settleReport(ch) != settleStateSet {
		if time.Now().After(deadline) {
			t.Fatal("the D-01 timer resume never settled the channel (5s)")
		}

		time.Sleep(10 * time.Millisecond)
	}

	if s.HasPendingAsk() {
		t.Error("ask still pending after the settle (the resume completed)")
	}
}

// TestAsk_Settle_ReplyResumeSettles (13-00 T2 test 2): the REPLY driver's
// resume settles the SAME channel — one signal, both drivers.
func TestAsk_Settle_ReplyResumeSettles(t *testing.T) {
	t.Parallel()

	s, _, _ := newAskSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				ID: askCallID2, Name: askToolNameTest,
				Input: json.RawMessage(askInput),
			}},
		},
		{FinishReason: stopEndTurn},
	}, time.Hour)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: askPromptText}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want %q", stop, stopAsk)
	}

	ch := s.AskSettleChan()

	if got := settleReport(ch); got != settleStateOpen {
		t.Fatalf("settle channel %q while pending; want open", got)
	}

	_, err = s.ResolveAsk(context.Background(), "ristretto please")
	if err != nil {
		t.Fatalf("ResolveAsk: %v", err)
	}

	if got := settleReport(ch); got != settleStateSet {
		t.Fatalf("settle channel %q after the reply resume returned; want settled", got)
	}
}

// TestAsk_Settle_ReflectsTurnCompletionNotClaim (13-00 T2 test 3): the signal
// reflects TURN COMPLETION, not the broker claim — while the resumed turn is
// still running (the fake provider's delay holds the model call open, the
// claim already happened) the channel is NOT yet closed: decisions can never
// fire on a half-resumed turn.
func TestAsk_Settle_ReflectsTurnCompletionNotClaim(t *testing.T) { //nolint:funlen // delayed-provider battery
	t.Parallel()

	s := newTestSessionWithCatalog(t, nil)

	surfaced := &[]PendingAsk{}

	broker := NewAskBroker(50*time.Millisecond, func(p PendingAsk) { *surfaced = append(*surfaced, p) })

	s.SetAskBroker(context.Background(), broker)

	s.Catalog.Register(toolcat.Tool{
		Name:        askToolNameTest,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"questions":{"type":"array"}}}`),
		Execute: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			var a struct {
				Questions []AskQuestion `json:"questions"`
			}

			_ = json.Unmarshal(args, &a)

			out, _ := json.Marshal(a.Questions)

			return out, ErrSuspended
		},
	})

	s.SetToolExecutor(&toolexec.RealExecutor{Catalog: s.Catalog})

	// Reach the fakeProvider to set the per-call delay (every model call
	// sleeps 300ms — the resumed call stays in flight well past the 50ms
	// timer claim).
	fp := s.Provider.(*fakeProvider) //nolint:forcetypeassert // constructed above
	fp.responses = []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				ID: askCallID1, Name: askToolNameTest,
				Input: json.RawMessage(askInput),
			}},
		},
		{FinishReason: stopEndTurn},
	}
	fp.delay = 300 * time.Millisecond

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: askPromptText}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want %q", stop, stopAsk)
	}

	ch := s.AskSettleChan()

	// Wait until the claim happened (no longer pending) but the resumed turn
	// is still inside its delayed model call.
	deadline := time.Now().Add(5 * time.Second)

	for s.HasPendingAsk() {
		if time.Now().After(deadline) {
			t.Fatal("the timer never claimed the pending ask (5s)")
		}

		time.Sleep(5 * time.Millisecond)
	}

	// Claimed, resumed turn mid-flight (its 300ms model call started at the
	// ~50ms claim; this check runs well inside that window).
	if got := settleReport(ch); got != settleStateOpen {
		t.Fatalf("settle channel %q while the resumed turn is STILL RUNNING (claim ≠ completion); want open", got)
	}

	// Now it settles once the turn completes.
	for settleReport(ch) != settleStateSet {
		if time.Now().After(deadline) {
			t.Fatal("the resumed turn never completed/settled (5s)")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// TestAsk_Settle_SequentialSuspensionsFreshSignal (13-00 T2 test 4): a second,
// sequential suspension (the resumed turn asks again) arms a FRESH signal — no
// stale-settle ABA; the first signal closes with ITS resume, the second stays
// open until its own driver completes it.
func TestAsk_Settle_SequentialSuspensionsFreshSignal(t *testing.T) { //nolint:cyclop,funlen // flat settle battery
	t.Parallel()

	s, _, _ := newAskSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				ID: askCallID1, Name: askToolNameTest,
				Input: json.RawMessage(askInput),
			}},
		},
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				ID: askCallID2, Name: askToolNameTest,
				Input: json.RawMessage(askInput),
			}},
		},
		{FinishReason: stopEndTurn},
	}, time.Hour)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: askPromptText}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop 1 = %q; want %q", stop, stopAsk)
	}

	ch1 := s.AskSettleChan()

	_, err = s.ResolveAsk(context.Background(), "first answer")
	if err != nil {
		t.Fatalf("ResolveAsk 1: %v", err)
	}

	// The resumed turn asked again: wait for the second suspension.
	deadline := time.Now().Add(5 * time.Second)

	for !s.HasPendingAsk() {
		if time.Now().After(deadline) {
			t.Fatal("the resumed turn never asked again (5s)")
		}

		time.Sleep(5 * time.Millisecond)
	}

	ch2 := s.AskSettleChan()

	if ch1 == nil || ch2 == nil {
		t.Fatal("a sequential suspension lost its settle signal (nil channel)")
	}

	// The accessor now returns the FRESH signal — the two are distinct
	// channels (compare via behavior: ch2 open; ch1 settles shortly).
	if c1, c2 := settleReport(ch1), settleReport(ch2); c1 == settleStateOpen && c2 == settleStateOpen {
		// channels could still be identical — prove distinctness by pointer.
		if anyEqual(ch1, ch2) {
			t.Fatal("the second suspension re-used the first settle signal (stale ABA hazard)")
		}
	}

	for settleReport(ch1) != settleStateSet {
		if time.Now().After(deadline) {
			t.Fatal("the FIRST suspension's signal never settled after its resume returned (5s)")
		}

		time.Sleep(5 * time.Millisecond)
	}

	if got := settleReport(ch2); got != settleStateOpen {
		t.Fatalf("the SECOND suspension's signal %q before its driver ran; want open", got)
	}

	_, err = s.ResolveAsk(context.Background(), "second answer")
	if err != nil {
		t.Fatalf("ResolveAsk 2: %v", err)
	}

	if got := settleReport(ch2); got != settleStateSet {
		t.Fatalf("the second signal %q after its resume; want settled", got)
	}
}

// anyEqual compares two receive-only channels by identity (the only way
// outside the package that produced them).
func anyEqual(a, b <-chan struct{}) bool {
	return fmt.Sprintf("%p", a) == fmt.Sprintf("%p", b)
}

// TestAsk_Settle_WaiterCancellationPrompt (13-00 T2 test 5): a waiter's ctx
// cancellation returns unsettled promptly — including the block-forever
// timeout=0 mode (no timer driver at all: the channel alone NEVER settles; the
// waiter's ctx is the only exit).
func TestAsk_Settle_WaiterCancellationPrompt(t *testing.T) {
	t.Parallel()

	s, _, _ := newAskSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				ID: askCallID1, Name: askToolNameTest,
				Input: json.RawMessage(askInput),
			}},
		},
		{FinishReason: stopEndTurn},
	}, 0) // block forever: no timer is armed

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: askPromptText}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want %q", stop, stopAsk)
	}

	ch := s.AskSettleChan()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()

	select {
	case <-ch:
		t.Fatal("block-forever suspension settled with no driver (nothing ran)")
	case <-ctx.Done():
		elapsed := time.Since(start)

		if elapsed > 500*time.Millisecond {
			t.Errorf("waiter ctx return took %v; want ~the ctx deadline (prompt return)", elapsed)
		}
	}

	if got := settleReport(ch); got != settleStateOpen {
		t.Errorf("channel %q after the cancelled wait; want still open (no driver ran)", got)
	}
}
