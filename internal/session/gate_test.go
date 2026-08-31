package session //nolint:testpackage // internal package test (drives the real tool loop + gate)

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/perm"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
	"github.com/Djarvur/ass-guard-agent/internal/toolexec"
)

// The 17-02 permission-gate battery (ACP-01, D-04/D-05/D-06/D-07): THE
// per-call chokepoint in runTurn's tool loop — hook-verdict head (Phase 21
// joins), rule evaluation in BOTH modes, the gated suspend/ask/resume chain
// through the ask queue, and the fail-safe cancelled/decline outcomes.

// Test-local vocabulary (goconst).
const (
	gateToolWrite = "Write"
	gateToolTask  = "Task"
	gateCall1     = "call_gate_1"
	gateCall2     = "call_gate_2"
	gatePrompt    = "do the gated thing"
	gatePathInput = `{"path":"x"}`
)

// fakePermStore records dialog writes AND execution events in ONE order
// slice — the persist-BEFORE-execute prohibition (ACP-01) is asserted as
// "allow:Write" preceding "exec:Write".
type fakePermStore struct {
	mu    sync.Mutex
	deny  []string
	allow []string
	order []string
}

func (f *fakePermStore) Rules() perm.RuleSet {
	f.mu.Lock()
	defer f.mu.Unlock()

	rs, _ := perm.NewRuleSet(f.deny, nil, f.allow)

	return rs
}

func (f *fakePermStore) Allow(tool string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.allow = append(f.allow, tool)
	f.order = append(f.order, "allow:"+tool)

	return nil
}

func (f *fakePermStore) Forbid(tool string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.deny = append(f.deny, tool)
	f.order = append(f.order, "forbid:"+tool)

	return nil
}

func (f *fakePermStore) noteExec(tool string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.order = append(f.order, "exec:"+tool)
}

func (f *fakePermStore) snapshotOrder() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.order...)
}

// fakeGateSurface is the injected ask fire: it records every enqueued entry,
// pops pre-programmed answers, and can block mid-round-trip (an open dialog).
type fakeGateSurface struct {
	mu      sync.Mutex
	entries []*AskEntry
	answers []AskOutcome
	block   chan struct{}
}

func (f *fakeGateSurface) Fire(e *AskEntry) AskOutcome {
	f.mu.Lock()

	f.entries = append(f.entries, e)

	ans := AskOutcome{}
	if len(f.answers) > 0 {
		ans = f.answers[0]
		f.answers = f.answers[1:]
	}

	block := f.block

	f.mu.Unlock()

	if block != nil {
		<-block
	}

	return ans
}

func (f *fakeGateSurface) fired() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.entries)
}

func (f *fakeGateSurface) lastEntry() *AskEntry {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.entries) == 0 {
		return nil
	}

	return f.entries[len(f.entries)-1]
}

// gateWaitFor polls cond until true or the deadline expires (the ask queue
// resolves + resumes asynchronously — the pump goroutine owns the resume).
func gateWaitFor(t *testing.T, cond func() bool) {
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

// newGateSession builds a catalog-wired gated session: a mutating Write tool
// whose execution is recorded into the store's order slice, the real batch
// dispatch path, and the injected gate deps (store + mode + queue + surface).
func newGateSession(
	t *testing.T, responses []provider.Response, mode string,
	store *fakePermStore, surf *fakeGateSurface,
) *Session {
	t.Helper()

	s := newTestSessionWithCatalog(t, responses)

	s.SetPermissionGate(GateDeps{
		Rules:  store.Rules,
		Mode:   func() string { return mode },
		Allow:  store.Allow,
		Forbid: store.Forbid,
		Fire:   surf.Fire,
		Queue:  NewAskQueue(),
	})

	s.Catalog.Register(toolcat.Tool{
		Name:        gateToolWrite,
		Mutability:  toolcat.MutabilityMutating,
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			store.noteExec(gateToolWrite)

			return json.RawMessage(`{"output":"written"}`), nil
		},
	})
	s.SetToolExecutor(&toolexec.RealExecutor{Catalog: s.Catalog})

	return s
}

// toolResultsFor returns the transcript's tool_result lines for one call id.
func toolResultsFor(t *testing.T, s *Session, callID string) []Line {
	t.Helper()

	var out []Line

	for _, l := range linesOf(s) {
		if l.Type == TypeToolResult && l.ToolCallID == callID {
			out = append(out, l)
		}
	}

	return out
}

// TestGatePermissionSuspend pins criterion 1 end-to-end: a gated, ask-class
// call SUSPENDS the turn (ask stop, no executor call), the queue fires the
// surface EXACTLY once carrying the pending-ask identity, the allow_always
// answer persists the rule BEFORE the gated call executes, the result is
// appended loud, the SAME turn resumes, and the follow-up identical call hits
// the allow rule with ZERO further surface firings.
func TestGatePermissionSuspend(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})

	store := &fakePermStore{}
	surf := &fakeGateSurface{
		answers: []AskOutcome{{Selected: acp.PermOptionAllowAlways}},
		block:   block,
	}

	s := newGateSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)},
			},
		},
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{ID: gateCall2, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)},
			},
		},
		{FinishReason: stopEndTurn},
	}, PermModeGated, store, surf)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want the ask-suspension marker %q", stop, stopAsk)
	}

	// The queue fired the surface exactly once — the dialog is "open" (blocked).
	gateWaitFor(t, func() bool { return surf.fired() == 1 })

	// NO executor call while the dialog is open (the deadline wrap must never
	// see a human-scale wait — RESEARCH Pitfall 1).
	if order := store.snapshotOrder(); len(order) != 0 {
		t.Fatalf("executor ran while the dialog was open: %v", order)
	}

	// The entry carries the full pending-ask identity.
	e := surf.lastEntry()
	if e == nil {
		t.Fatal("no ask entry recorded")
	}

	if e.TurnID != s.CurrentTurnID() || e.CallID != gateCall1 || e.Tool != gateToolWrite {
		t.Errorf("entry identity = turn=%q call=%q tool=%q; want turn=%q call=%q tool=Write",
			e.TurnID, e.CallID, e.Tool, s.CurrentTurnID(), gateCall1)
	}

	if string(e.Input) != gatePathInput {
		t.Errorf("entry input = %s; want %s", e.Input, gatePathInput)
	}

	if e.SessionID != s.SessionID {
		t.Errorf("entry sessionID = %q; want %q", e.SessionID, s.SessionID)
	}

	if e.Class != AskClassForeground {
		t.Errorf("entry class = %d; want foreground (client turn, D-11)", e.Class)
	}

	// The dialog "opens": release the fire with allow_always. The rule write
	// MUST land BEFORE the gated call executes (ACP-01 prohibition), the call
	// runs, the turn resumes, and the SECOND identical call executes with no
	// second dialog.
	close(block)

	gateWaitFor(t, func() bool {
		return len(store.snapshotOrder()) == 3 // allow + exec(call1) + exec(call2)
	})

	order := store.snapshotOrder()
	if order[0] != "allow:"+gateToolWrite || order[1] != "exec:"+gateToolWrite {
		t.Fatalf("persist/execute order = %v; want allow BEFORE the first exec", order)
	}

	if surf.fired() != 1 {
		t.Errorf("surface fired %d times; want exactly 1 (the follow-up call hits the allow rule)", surf.fired())
	}

	// Both calls' results are appended loud (non-error tool results).
	for _, call := range []string{gateCall1, gateCall2} {
		results := toolResultsFor(t, s, call)
		if len(results) != 1 || results[0].IsError {
			t.Errorf("tool_result for %s = %+v; want exactly one non-error result", call, results)
		}
	}
}

// TestGateChokepoint_UngatedDefaultNoDialog pins criterion 4's default leg:
// ungated (the DEFAULT mode), an ask-class call with no matching rule executes
// immediately with ZERO surface invocations — "zero new dialogs on default
// config" holds by construction (D-05).
func TestGateChokepoint_UngatedDefaultNoDialog(t *testing.T) {
	t.Parallel()

	store := &fakePermStore{}
	surf := &fakeGateSurface{}

	s := newGateSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)},
			},
		},
		{FinishReason: stopEndTurn},
	}, PermModeUngated, store, surf)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopEndTurn {
		t.Fatalf("stop = %q; want end_turn (ungated ask-class executes, no suspension)", stop)
	}

	if surf.fired() != 0 {
		t.Errorf("surface fired %d times; want 0 (ungated never opens a dialog)", surf.fired())
	}

	if order := store.snapshotOrder(); len(order) != 1 || order[0] != "exec:"+gateToolWrite {
		t.Errorf("execution order = %v; want exactly one exec", order)
	}
}

// TestGateChokepoint_UngatedDenyStillDenies pins D-05's rule half: ungated
// mode still EVALUATES rules — a deny rule denies without executing and
// without a dialog (RESEARCH Pitfall 5: "no dialogs" never means "no
// evaluation").
func TestGateChokepoint_UngatedDenyStillDenies(t *testing.T) {
	t.Parallel()

	store := &fakePermStore{deny: []string{gateToolWrite}}
	surf := &fakeGateSurface{}

	s := newGateSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)},
			},
		},
		{FinishReason: stopEndTurn},
	}, PermModeUngated, store, surf)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopEndTurn {
		t.Fatalf("stop = %q; want end_turn", stop)
	}

	if surf.fired() != 0 {
		t.Errorf("surface fired %d times; want 0", surf.fired())
	}

	if order := store.snapshotOrder(); len(order) != 0 {
		t.Errorf("denied call executed: %v", order)
	}

	results := toolResultsFor(t, s, gateCall1)
	if len(results) != 1 || !results[0].IsError {
		t.Errorf("denial result = %+v; want exactly one error result", results)
	}
}

// TestGateChokepoint_CancelledNormalResume pins criterion 2's letter: a
// cancelled dialog outcome appends a cancelled-NORMAL tool result (NOT an
// error result) and the turn resumes.
func TestGateChokepoint_CancelledNormalResume(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})

	store := &fakePermStore{}
	surf := &fakeGateSurface{
		answers: []AskOutcome{{Cancelled: true}},
		block:   block,
	}

	s := newGateSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)},
			},
		},
		{FinishReason: stopEndTurn},
	}, PermModeGated, store, surf)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want the ask marker", stop)
	}

	gateWaitFor(t, func() bool { return surf.fired() == 1 })

	close(block)

	gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

	results := toolResultsFor(t, s, gateCall1)
	if len(results) != 1 || results[0].IsError {
		t.Fatalf("cancelled result = %+v; want exactly one NON-error (cancelled-normal) result", results)
	}

	if order := store.snapshotOrder(); len(order) != 0 {
		t.Errorf("cancelled call executed: %v", order)
	}
}

// TestGateChokepoint_SubagentBranchGated pins D-05's ONE-pipeline rule for the
// subagent dispatch branch: a gated, ask-class Task call suspends BEFORE
// DispatchSubagent (no second permission path), and the allow_once answer
// dispatches the subagent on resume with the result appended.
func TestGateChokepoint_SubagentBranchGated(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})

	store := &fakePermStore{}
	surf := &fakeGateSurface{
		answers: []AskOutcome{{Selected: acp.PermOptionAllowOnce}},
		block:   block,
	}

	s := newGateSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolTask, Input: json.RawMessage(`{"prompt":"explore"}`)},
			},
		},
		{FinishReason: stopEndTurn},
	}, PermModeGated, store, surf)

	// Task gated per its CATALOG class (D-06): register it mutating.
	s.Catalog.Register(toolcat.Tool{
		Name:        gateToolTask,
		Mutability:  toolcat.MutabilityMutating,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	})

	runner := &countingSubagentRunner{}
	s.subagentRunner = runner

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want the ask marker (subagent branch suspends too)", stop)
	}

	gateWaitFor(t, func() bool { return surf.fired() == 1 })

	if runner.dispatched() != 0 {
		t.Fatal("subagent dispatched while the dialog was open (the gate must sit BEFORE DispatchSubagent)")
	}

	close(block)

	gateWaitFor(t, func() bool { return runner.dispatched() == 1 })

	if surf.fired() != 1 {
		t.Errorf("surface fired %d times; want 1", surf.fired())
	}

	results := toolResultsFor(t, s, gateCall1)
	if len(results) != 1 || results[0].IsError {
		t.Errorf("subagent result = %+v; want exactly one non-error result", results)
	}
}

// countingSubagentRunner records DispatchSubagent invocations (the subagentRunner
// seam fake).
type countingSubagentRunner struct {
	mu    sync.Mutex
	calls int
}

func (f *countingSubagentRunner) Run(
	_ context.Context, _ *Session, _, _, _ string, _ []string, _ *ecosys.Agent,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	return "subagent report", nil
}

func (f *countingSubagentRunner) dispatched() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

// TestGateChokepoint_SecondPromptWhileDialogOpen pins the suspension
// discipline (RESEARCH Pitfall 2): the ask wait NEVER holds the turn
// serialization — a second prompt submitted while the dialog is open starts
// and completes. Runs under -race.
func TestGateChokepoint_SecondPromptWhileDialogOpen(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})

	store := &fakePermStore{}
	surf := &fakeGateSurface{
		answers: []AskOutcome{{Selected: acp.PermOptionAllowOnce}},
		block:   block,
	}

	s := newGateSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)},
			},
		},
		{FinishReason: stopEndTurn}, // the second prompt's turn
		{FinishReason: stopEndTurn}, // the resumed turn after the allow
	}, PermModeGated, store, surf)

	stop1, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("prompt 1: %v", err)
	}

	if stop1 != stopAsk {
		t.Fatalf("stop1 = %q; want the ask marker", stop1)
	}

	gateWaitFor(t, func() bool { return surf.fired() == 1 })

	// The dialog is open (blocked) — a second prompt must run to completion.
	stop2, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "second prompt"}})
	if err != nil {
		t.Fatalf("prompt 2 while dialog open: %v", err)
	}

	if stop2 != stopEndTurn {
		t.Errorf("stop2 = %q; want end_turn (nothing blocks behind a human ask)", stop2)
	}

	// Release the dialog; the suspended turn resumes and executes.
	close(block)

	gateWaitFor(t, func() bool { return len(store.snapshotOrder()) == 1 })

	if order := store.snapshotOrder(); order[0] != "exec:"+gateToolWrite {
		t.Errorf("resume order = %v; want the gated call executed", order)
	}
}

// TestGateAskKindsParity pins the session-side option-kind vocabulary to the
// wire constants (the TestHumanAskTimeoutMirrorsSessionDefault pattern — the
// two packages must never drift; session cannot import acp in production).
func TestGateAskKindsParity(t *testing.T) {
	t.Parallel()

	if permOptAllowOnce != acp.PermOptionAllowOnce ||
		permOptAllowAlways != acp.PermOptionAllowAlways ||
		permOptRejectOnce != acp.PermOptionRejectOnce ||
		permOptRejectAlways != acp.PermOptionRejectAlways {
		t.Fatal("session option-kind constants drifted from the ACP wire enum")
	}

	if PendingAskKindPermission == "" || PendingAskKindPermission == PendingAskKindPlanApproval {
		t.Fatal("PendingAskKindPermission must be a distinct non-empty kind")
	}
}
