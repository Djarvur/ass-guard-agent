package session //nolint:testpackage // internal package test (drives the real tool loop + gate)

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

// pmCallID2 is the ExitPlanMode call id (goconst).
const pmCallID2 = "call_pm_2"

// The 12-04 Task 1 battery: plan mode end-to-end over the REAL Session tool
// loop (the 12-01 ask_test pattern). SCOPE NOTE: the 12-05 capture (zcode
// 0.16.3) answers ACP-02's at-plan-phase question — the TARGET enforces plan
// mode as a RUNTIME-LEVEL mutating-tool gate ('Plan mode only allows
// read-only, non-destructive tools'); these tests pin the CAPTURED behavior
// (the 12-04 plan's original no-gating premise was written pre-capture and is
// superseded by it — documented as a deviation in 12-04-SUMMARY).

// planModeTestTools registers the plan pair as LOCAL twins of the coreexec
// executors (the ask_test layering: in-package session tests cannot import
// coreexec — the real executors are proven by coreexec's suite + the wiring
// test) plus a Bash stub whose output proves real-vs-refused execution.
func planModeTestTools(s *Session, state *PlanModeState) {
	s.Catalog.Register(toolcat.Tool{
		Name:        toolNameEnterPlanMode,
		Mutability:  toolcat.MutabilityReadOnly,
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.Marshal(PlanModeEnteredForm)
		},
	})
	s.Catalog.Register(toolcat.Tool{
		Name:        toolNameExitPlanMode,
		Mutability:  toolcat.MutabilityReadOnly,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"plan":{"type":"string"}}}`),
		Execute: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			if state == nil || !state.IsOn() {
				return nil, errExitPlanModeNotInPlan
			}

			var a struct {
				Plan string `json:"plan"`
			}

			_ = json.Unmarshal(args, &a)
			payload, _ := json.Marshal([]AskQuestion{{
				Question: planApprovalQuestion(a.Plan),
				Header:   "Plan approval",
				Options: []AskOption{
					{Label: "approve", Description: "approve the plan and exit plan mode"},
					{Label: "decline", Description: "keep planning"},
				},
			}})

			return payload, ErrSuspended
		},
	})
	s.Catalog.Register(toolcat.Tool{
		Name:       toolBash,
		Mutability: toolcat.MutabilityMutating,
		Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.Marshal("bash-ran")
		},
	})
	s.Catalog.Register(toolcat.Tool{
		Name:       toolRead,
		Mutability: toolcat.MutabilityReadOnly,
		Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.Marshal("read-ran")
		},
	})
}

func newPlanModeSession(t *testing.T, responses []provider.Response, timeout time.Duration) (*Session, *PlanModeState) {
	t.Helper()

	s := newTestSessionWithCatalog(t, responses)
	state := NewPlanModeState()
	s.SetPlanMode(state)
	planModeTestTools(s, state)

	s.SetAskBroker(context.Background(), NewAskBroker(timeout, func(PendingAsk) {}))
	s.SetToolExecutor(&toolexec.RealExecutor{Catalog: s.Catalog})

	return s, state
}

// TestPlanMode_EnterGateApprovalExit (T1 Tests 1+4 combined): EnterPlanMode
// records the marker + flips the state; a MUTATING call under plan mode is
// REFUSED with the CAPTURED form (never executed); ExitPlanMode suspends on
// the broker; the approval reply lands the approved form as its tool result,
// records the exit marker, flips the state OFF, and the resumed turn executes
// the SAME mutating call for real.
func TestPlanMode_EnterGateApprovalExit(t *testing.T) { //nolint:gocyclo,cyclop,funlen // flat battery
	t.Parallel()

	exitInput := json.RawMessage(`{"plan":"1. add multiply() to calc.js\n2. add a test"}`)

	s, state := newPlanModeSession(t, []provider.Response{
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: "call_pm_1", Name: toolNameEnterPlanMode, Input: json.RawMessage(`{}`)},
		}},
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: "call_bash_1", Name: toolBash, Input: json.RawMessage(`{"command":"echo hi"}`)},
		}},
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: pmCallID2, Name: toolNameExitPlanMode, Input: exitInput},
		}},
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: "call_bash_2", Name: toolBash, Input: json.RawMessage(`{"command":"echo hi"}`)},
		}},
		{FinishReason: stopEndTurn},
	}, 0) // 0 = block forever: this test drives the reply explicitly

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "plan then do it"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want the ask-suspension marker %q", stop, stopAsk)
	}

	if !state.IsOn() {
		t.Fatal("plan-mode state OFF after EnterPlanMode; want ON")
	}

	var (
		enterMarker bool
		refused     string
		refusedErr  bool
	)

	for _, l := range linesOf(s) {
		switch {
		case l.Type == TypePlanMode && l.Cause == planModeCauseEnter:
			enterMarker = true
		case l.Type == TypeToolResult && l.ToolCallID == "call_bash_1":
			_ = json.Unmarshal(l.Output, &refused)
			refusedErr = l.IsError
		}
	}

	if !enterMarker {
		t.Error("no plan_mode_enter marker after EnterPlanMode")
	}

	if refused != planModeRefusalForm || !refusedErr {
		t.Errorf("gated Bash result = %q isError=%v; want the CAPTURED refusal (isError)",
			refused, refusedErr)
	}

	// The approval surfaces on the broker with the plan + kind.
	p, ok := s.ask.Snapshot()
	if !ok {
		t.Fatal("no pending ask after ExitPlanMode")
	}

	if p.Kind != PendingAskKindPlanApproval {
		t.Errorf("pending kind = %q; want %q", p.Kind, PendingAskKindPlanApproval)
	}

	if !strings.Contains(p.Questions[0].Question, "add multiply()") {
		t.Errorf("approval question = %q; want it to carry the plan text", p.Questions[0].Question)
	}

	// Approve: the resumed turn lands the approved form + exit marker + OFF.
	stop2, err := s.ResolveAsk(context.Background(), "approve")
	if err != nil {
		t.Fatalf("ResolveAsk: %v", err)
	}

	if stop2 != stopEndTurn {
		t.Errorf("resumed stop = %q; want end_turn", stop2)
	}

	if state.IsOn() {
		t.Error("plan-mode state ON after approval; want OFF")
	}

	var (
		exitMarker bool
		exitResult string
		bashRan    bool
		bashRanOut string
	)

	for _, l := range linesOf(s) {
		switch {
		case l.Type == TypePlanMode && l.Cause == planModeCauseExit:
			exitMarker = true
		case l.Type == TypeToolResult && l.ToolCallID == pmCallID2:
			_ = json.Unmarshal(l.Output, &exitResult)
		case l.Type == TypeToolResult && l.ToolCallID == "call_bash_2":
			_ = json.Unmarshal(l.Output, &bashRanOut)
			bashRan = true
		}
	}

	if !exitMarker {
		t.Error("no plan_mode_exit marker after the approval")
	}

	if !strings.HasPrefix(exitResult, "User has approved your plan.") {
		t.Errorf("approved form = %q; want the approved-plan prefix", exitResult)
	}

	if !strings.Contains(exitResult, "add multiply()") {
		t.Errorf("approved form = %q; want the plan embedded", exitResult)
	}

	if !bashRan || bashRanOut != "bash-ran" {
		t.Errorf("post-approval Bash = %q ran=%v; want real execution after the gate lifted", bashRanOut, bashRan)
	}
}

// planModeGateMock is intentionally empty: the gate tests construct no provider.

// TestPlanMode_TimeoutStaysOn (T1 Test 2, D-01 inheritance): an approval that
// times out resumes with the NON-ANSWER form and the state STAYS ON — an
// unapproved plan never ungates the mutating tools.
func TestPlanMode_TimeoutStaysOn(t *testing.T) {
	t.Parallel()

	s, state := newPlanModeSession(t, []provider.Response{
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: "call_pm_1", Name: toolNameEnterPlanMode, Input: json.RawMessage(`{}`)},
		}},
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: pmCallID2, Name: toolNameExitPlanMode, Input: json.RawMessage(`{"plan":"do the thing"}`)},
		}},
		{FinishReason: stopEndTurn},
	}, 50*time.Millisecond)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "plan it"}})
	if err != nil || stop != stopAsk {
		t.Fatalf("Prompt stop=%q err=%v; want the suspension", stop, err)
	}

	// The D-01 timer's Claim clears the pending ask BEFORE fire resumes the
	// turn (resumeAskClaimed appends the non-answer line asynchronously), so
	// !HasPendingAsk alone is a racy proxy — poll until the line itself has
	// landed.
	pmNonAnswer := func() string {
		var out string

		for _, l := range linesOf(s) {
			if l.Type == TypeToolResult && l.ToolCallID == pmCallID2 {
				_ = json.Unmarshal(l.Output, &out)
			}
		}

		return out
	}

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if !s.HasPendingAsk() && pmNonAnswer() != "" {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	if s.HasPendingAsk() {
		t.Fatal("the D-01 timer never resolved the approval")
	}

	if !state.IsOn() {
		t.Error("state flipped OFF on a timed-out approval; an unapproved plan must keep the gate ON")
	}

	var nonAnswer string

	for _, l := range linesOf(s) {
		if l.Type == TypeToolResult && l.ToolCallID == pmCallID2 {
			_ = json.Unmarshal(l.Output, &nonAnswer)
		}
	}

	if !strings.HasPrefix(nonAnswer, "User has not answered your questions") {
		t.Errorf("timed-out approval result = %q; want the D-01 non-answer form", nonAnswer)
	}
}

// TestPlanMode_BlockedSet (T1 Test 4, gate set): the gate follows the capture —
// catalog-mutating tools PLUS the captured refusal set (SendMessage, TaskStop,
// the cron family); read-only tools (incl. the plan pair + ReadSessionContext)
// pass.
func TestPlanMode_BlockedSet(t *testing.T) {
	t.Parallel()

	s, state := newPlanModeSession(t, nil, 0)
	state.Enter()

	cases := map[string]bool{ // name -> want blocked
		toolBash:              true,
		"Write":               true,
		"SendMessage":         true, // capture: refused in plan mode (read-only in catalog, side-effecting in target)
		"TaskStop":            true, // capture: refused
		"CronCreate":          true, // capture: 'changes persistent state'
		toolNameEnterPlanMode: false,
		toolNameExitPlanMode:  false,
		toolRead:              false,
		"ReadSessionContext":  false,
	}

	for name, want := range cases {
		if got := s.planModeBlocks(name); got != want {
			t.Errorf("planModeBlocks(%q) = %v; want %v", name, got, want)
		}
	}
}
