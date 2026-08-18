package session //nolint:testpackage // internal package test (drives the real tool loop + broker)

import (
	"context"
	"encoding/json"
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

// Test-local tool + call ids (goconst).
const (
	askToolNameTest = "AskUserQuestion"
	askCallID1      = "call_ask_1"
	askCallID2      = "call_ask_2"
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

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "add a cache"}})
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

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "add a cache"}})
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

		const want = `User has answered your questions: "ristretto please"`
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
