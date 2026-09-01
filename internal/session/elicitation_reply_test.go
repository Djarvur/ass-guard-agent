package session //nolint:testpackage // internal package test (drives the real suspension + queue)

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
	"github.com/Djarvur/ass-guard-agent/internal/toolexec"
)

// The 17-04 structured-reply + D-10 battery (ACP-02): the widened reply seam
// renders elicitation accept.content maps into the CAPTURED answered form —
// single string fields byte-identically to today (the
// zcode-interactive-results.json golden) — and the D-10 bounded loop
// re-asks ONCE on an invalid accept while decline/cancel route to the
// non-answer directly.

// Test-local vocabulary (goconst).
const (
	elicitCall1   = "call_elicit_1"
	elicitAskTool = "AskUserQuestion"
	elicitQText   = "Which cache library should we use?"
	elicitQText2  = "Which areas should the review cover?"
	elicitHdrQ1   = "Cache"
	elicitHdrQ2   = "Areas"
	elicitReply   = "ristretto, for the speed"
	elicitAnswer2 = "api"
)

// elicFixtureQs is the two-question suspension payload.
func elicFixtureQs() []AskQuestion {
	return []AskQuestion{
		{Question: elicitQText, Header: elicitHdrQ1},
		{Question: elicitQText2, Header: elicitHdrQ2},
	}
}

// newElicitSession builds a session whose AskUserQuestion tool suspends with
// the fixture questions: the real tool loop, the real broker (1h D-01 timer —
// never interferes), and a queue-wired gate so the elicitation entries enqueue.
func newElicitSession(t *testing.T, qs []AskQuestion) *Session {
	t.Helper()

	out, mErr := json.Marshal(qs)
	if mErr != nil {
		t.Fatalf("marshal fixture questions: %v", mErr)
	}

	s := newTestSessionWithCatalog(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{ID: elicitCall1, Name: elicitAskTool, Input: json.RawMessage(`{}`)}},
		},
		{FinishReason: stopEndTurn},
	})

	s.Catalog.Register(toolcat.Tool{
		Name:        elicitAskTool,
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return out, ErrSuspended
		},
	})
	s.SetToolExecutor(&toolexec.RealExecutor{Catalog: s.Catalog})

	broker := NewAskBroker(time.Hour, nil)
	s.SetAskBroker(context.Background(), broker)
	s.SetPermissionGate(GateDeps{Queue: NewAskQueue()})

	return s
}

// suspendElicit drives one real suspension and hands the pending ask back.
func suspendElicit(t *testing.T, s *Session) PendingAsk {
	t.Helper()

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "ask me"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want the ask marker %q", stop, stopAsk)
	}

	pending, ok := s.AskBroker().Snapshot()
	if !ok {
		t.Fatal("no pending ask after the suspending turn")
	}

	return pending
}

// elicitResult returns the single tool_result line's decoded form string.
func elicitResult(t *testing.T, s *Session) string {
	t.Helper()

	results := toolResultsFor(t, s, elicitCall1)
	if len(results) == 0 {
		t.Fatal("no tool result appended for the suspended call")
	}

	if len(results) > 1 {
		t.Fatalf("tool result appended %d times; want exactly one", len(results))
	}

	var form string

	uerr := json.Unmarshal(results[0].Output, &form)
	if uerr != nil {
		t.Fatalf("unmarshal result form: %v (raw %s)", uerr, results[0].Output)
	}

	return form
}

// TestStructuredReplyRender pins the deterministic serialization (A10): a
// single string-valued field renders BYTE-IDENTICALLY to today's captured
// answered form (the fixture's answered template); multi-field replies render
// in schema property order as title=value lines joined by newlines with array
// values comma-joined and booleans true/false.
func TestStructuredReplyRender(t *testing.T) { //nolint:funlen // pinned-serialization goldens
	t.Parallel()

	t.Run("single_string_byte_identity", func(t *testing.T) {
		t.Parallel()

		qs := []AskQuestion{{Question: elicitQText, Header: elicitHdrQ1}}
		content := map[string]json.RawMessage{"q1": json.RawMessage(`"` + elicitReply + `"`)}

		got := RenderStructuredReply(qs, content)

		if want := RenderAskAnswered(qs, elicitReply); got != want {
			t.Errorf("single-field render =\n%q\nwant byte-identity with the captured form\n%q", got, want)
		}

		// The fixture's literal shape (zcode-interactive-results.json
		// results.AskUserQuestion.results.answered.template): the wrapper, the
		// quoted question pairing, and the reply embedded verbatim.
		if !strings.HasPrefix(got, `User has answered your questions: "`) {
			t.Errorf("render lacks the captured literal prefix: %q", got)
		}

		if !strings.Contains(got, `"`+elicitQText+`"="`+elicitReply+`"`) {
			t.Errorf("render lacks the captured pairing: %q", got)
		}

		if !strings.HasSuffix(got, ". You can now continue with the user's answers in mind.") {
			t.Errorf("render lacks the captured wrapper tail: %q", got)
		}
	})

	t.Run("multi_field_property_order_lines", func(t *testing.T) {
		t.Parallel()

		content := map[string]json.RawMessage{
			"q1": json.RawMessage(`"` + elicitReply + `"`),
			"q2": json.RawMessage(`"` + elicitAnswer2 + `"`),
		}

		got := RenderStructuredReply(elicFixtureQs(), content)

		lines := strings.SplitN(got, "\n", 2)
		if len(lines) != 2 {
			t.Fatalf("render = %q; want two newline-joined lines", got)
		}

		if lines[0] != "User has answered your questions: "+elicitHdrQ1+"="+elicitReply {
			t.Errorf("line 1 = %q; want the first property's title=value", lines[0])
		}

		if !strings.HasPrefix(lines[1], elicitHdrQ2+"="+elicitAnswer2) {
			t.Errorf("line 2 = %q; want the second property's title=value", lines[1])
		}

		if !strings.HasSuffix(lines[1], ". You can now continue with the user's answers in mind.") {
			t.Errorf("line 2 lacks the captured wrapper tail: %q", lines[1])
		}
	})

	t.Run("array_comma_joined_bool_true_false", func(t *testing.T) {
		t.Parallel()

		qs := []AskQuestion{{Question: elicitQText2, Header: elicitHdrQ2}}
		content := map[string]json.RawMessage{
			"q1": json.RawMessage(`["api","docs"]`),
		}

		got := RenderStructuredReply(qs, content)

		if !strings.Contains(got, elicitHdrQ2+"=api,docs") {
			t.Errorf("array value not comma-joined: %q", got)
		}

		boolQs := []AskQuestion{{Question: elicitQText, Header: elicitHdrQ1}}
		boolContent := map[string]json.RawMessage{"q1": json.RawMessage(`true`)}

		boolGot := RenderStructuredReply(boolQs, boolContent)

		if !strings.Contains(boolGot, elicitHdrQ1+"=true") {
			t.Errorf("boolean value not rendered true: %q", boolGot)
		}
	})

	t.Run("missing_value_empty_string", func(t *testing.T) {
		t.Parallel()

		got := RenderStructuredReply(elicFixtureQs(), nil)

		if !strings.Contains(got, elicitHdrQ1+"=") || !strings.Contains(got, elicitHdrQ2+"=") {
			t.Errorf("missing values must render empty (never a nil panic): %q", got)
		}
	})
}

// TestElicitationParity pins the session-native elicitation action vocabulary
// byte-equal to the wire constants (the TestGateAskKindsParity pattern —
// internal/session never imports internal/acp in production code).
func TestElicitationParity(t *testing.T) {
	t.Parallel()

	if ElicitAccept != acp.ElicitationActionAccept || ElicitDecline != acp.ElicitationActionDecline {
		t.Errorf("elicitation action parity broken: (%q, %q) vs wire (%q, %q)",
			ElicitAccept, ElicitDecline, acp.ElicitationActionAccept, acp.ElicitationActionDecline)
	}
}

// TestElicitationRevalidation pins the D-10 bounded loop end-to-end through
// the real suspension + queue: an invalid accept re-asks EXACTLY once (a NEW
// queue entry with the violation noted) and a second failure routes to the
// 12-D-01 non-answer; decline and cancel route directly with zero re-asks.
func TestElicitationRevalidation(t *testing.T) { //nolint:funlen,cyclop,gocognit // D-10 battery
	t.Parallel()

	// fireOutcomes builds the injected fire: programmable outcomes + the
	// fired-count / last-carried-note probes.
	fireOutcomes := func(t *testing.T, outcomes ...AskOutcome) (
		func(ctx context.Context, e *AskEntry) AskOutcome,
		func() int,
		func() string,
	) {
		t.Helper()

		var (
			mu    sync.Mutex
			count int
			note  string
		)

		fireFn := func(_ context.Context, e *AskEntry) AskOutcome {
			mu.Lock()

			count++
			note = e.Note
			mu.Unlock()

			if len(outcomes) == 0 {
				return AskOutcome{}
			}

			ans := outcomes[0]
			outcomes = outcomes[1:]

			return ans
		}

		return fireFn, func() int {
				mu.Lock()
				defer mu.Unlock()

				return count
			}, func() string {
				mu.Lock()
				defer mu.Unlock()

				return note
			}
	}

	waitResult := func(t *testing.T, s *Session) string {
		t.Helper()

		gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, elicitCall1)) > 0 })

		return elicitResult(t, s)
	}

	t.Run("invalid_accept_reasks_once_then_non_answer", func(t *testing.T) {
		t.Parallel()

		s := newElicitSession(t, elicFixtureQs())
		pending := suspendElicit(t, s)

		fire, fired, lastNote := fireOutcomes(t,
			AskOutcome{Elicit: ElicitAccept, Violation: `value "x" is not one of the offered choices`},
			AskOutcome{Elicit: ElicitAccept, Violation: `missing required property "q2"`},
		)

		s.EnqueueElicitationAsk(pending, fire)

		form := waitResult(t, s)

		if n := fired(); n != 2 {
			t.Errorf("asks fired = %d; want exactly two (initial + the ONE bounded re-ask)", n)
		}

		if note := lastNote(); note != `value "x" is not one of the offered choices` {
			t.Errorf("re-ask note = %q; want the first violation carried on the new entry", note)
		}

		want := RenderAskNonAnswer(s.AskBroker().Timeout())
		if form != want {
			t.Errorf("final form = %q; want the 12-D-01 non-answer %q", form, want)
		}
	})

	t.Run("decline_routes_directly_no_reask", func(t *testing.T) {
		t.Parallel()

		s := newElicitSession(t, elicFixtureQs())
		pending := suspendElicit(t, s)

		fire, fired, _ := fireOutcomes(t, AskOutcome{Elicit: ElicitDecline})

		s.EnqueueElicitationAsk(pending, fire)

		form := waitResult(t, s)

		if n := fired(); n != 1 {
			t.Errorf("asks fired = %d; decline is an answer-shaped refusal (zero re-asks)", n)
		}

		if want := RenderAskNonAnswer(s.AskBroker().Timeout()); form != want {
			t.Errorf("form = %q; want the non-answer %q", form, want)
		}
	})

	t.Run("cancel_routes_directly_no_reask", func(t *testing.T) {
		t.Parallel()

		s := newElicitSession(t, elicFixtureQs())
		pending := suspendElicit(t, s)

		fire, fired, _ := fireOutcomes(t, AskOutcome{Cancelled: true})

		s.EnqueueElicitationAsk(pending, fire)

		form := waitResult(t, s)

		if n := fired(); n != 1 {
			t.Errorf("asks fired = %d; cancel routes directly (zero re-asks)", n)
		}

		if want := RenderAskNonAnswer(s.AskBroker().Timeout()); form != want {
			t.Errorf("form = %q; want the non-answer %q", form, want)
		}
	})

	t.Run("valid_single_string_accept_byte_identity_resume", func(t *testing.T) {
		t.Parallel()

		s := newElicitSession(t, []AskQuestion{{Question: elicitQText, Header: elicitHdrQ1}})
		pending := suspendElicit(t, s)

		fire, fired, _ := fireOutcomes(t, AskOutcome{
			Elicit:  ElicitAccept,
			Content: map[string]json.RawMessage{"q1": json.RawMessage(`"` + elicitReply + `"`)},
		})

		s.EnqueueElicitationAsk(pending, fire)

		form := waitResult(t, s)

		if n := fired(); n != 1 {
			t.Errorf("asks fired = %d; want one (a valid accept never re-asks)", n)
		}

		if want := RenderAskAnswered(pending.Questions, elicitReply); form != want {
			t.Errorf("form = %q; want the byte-identical captured answered form %q", form, want)
		}
	})

	t.Run("valid_multi_field_accept_deterministic_resume", func(t *testing.T) {
		t.Parallel()

		s := newElicitSession(t, elicFixtureQs())
		pending := suspendElicit(t, s)

		content := map[string]json.RawMessage{
			"q1": json.RawMessage(`"` + elicitReply + `"`),
			"q2": json.RawMessage(`"` + elicitAnswer2 + `"`),
		}

		fire, fired, _ := fireOutcomes(t, AskOutcome{Elicit: ElicitAccept, Content: content})

		s.EnqueueElicitationAsk(pending, fire)

		form := waitResult(t, s)

		if n := fired(); n != 1 {
			t.Errorf("asks fired = %d; want one", n)
		}

		if want := RenderStructuredReply(pending.Questions, content); form != want {
			t.Errorf("form = %q; want the deterministic multi-field render %q", form, want)
		}
	})

	t.Run("fallback_resolves_without_resume", func(t *testing.T) {
		t.Parallel()

		s := newElicitSession(t, []AskQuestion{{Question: elicitQText, Header: elicitHdrQ1}})
		pending := suspendElicit(t, s)

		fire, fired, _ := fireOutcomes(t, AskOutcome{Fallback: true})

		s.EnqueueElicitationAsk(pending, fire)

		time.Sleep(50 * time.Millisecond) // the queue's pump: the fallback outcome resolves the entry

		if n := fired(); n != 1 {
			t.Fatalf("asks fired = %d; want one", n)
		}

		if results := toolResultsFor(t, s, elicitCall1); len(results) != 0 {
			t.Errorf("fallback appended %d results; the broker's reply routing + D-01 timer own the ask",
				len(results))
		}

		if !s.HasPendingAsk() {
			t.Error("fallback must leave the ask pending in the broker (v1.1 semantics)")
		}
	})
}

// TestResolveAskStructured pins the widened public seam: a structured accept
// resolves the pending ask into the captured answered form and resumes the
// suspended turn (the same claim discipline as ResolveAsk).
func TestResolveAskStructured(t *testing.T) {
	t.Parallel()

	s := newElicitSession(t, []AskQuestion{{Question: elicitQText, Header: elicitHdrQ1}})
	suspendElicit(t, s)

	stop, err := s.ResolveAskStructured(context.Background(), map[string]json.RawMessage{
		"q1": json.RawMessage(`"` + elicitReply + `"`),
	})
	if err != nil {
		t.Fatalf("ResolveAskStructured: %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want the resumed turn's end_turn", stop)
	}

	form := elicitResult(t, s)

	want := RenderAskAnswered(
		[]AskQuestion{{Question: elicitQText, Header: elicitHdrQ1}}, elicitReply)

	if form != want {
		t.Errorf("form = %q; want the captured answered form %q", form, want)
	}

	if s.HasPendingAsk() {
		t.Error("the pending ask survived a structured resolution")
	}
}
