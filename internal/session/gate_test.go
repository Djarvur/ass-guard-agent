package session //nolint:testpackage // internal package test (drives the real tool loop + gate)

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
// hold=true pins the fire until the queue's drain cancels the fire ctx (the
// turn-death battery) — answering Cancelled, like a registry-backed ask.
type fakeGateSurface struct {
	mu      sync.Mutex
	entries []*AskEntry
	answers []AskOutcome
	block   chan struct{}
	hold    bool
}

func (f *fakeGateSurface) Fire(ctx context.Context, e *AskEntry) AskOutcome {
	f.mu.Lock()

	f.entries = append(f.entries, e)

	ans := AskOutcome{}
	if len(f.answers) > 0 {
		ans = f.answers[0]
		f.answers = f.answers[1:]
	}

	block := f.block
	hold := f.hold

	f.mu.Unlock()

	if hold {
		<-ctx.Done()

		return AskOutcome{Cancelled: true}
	}

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

	return newHookGateSession(t, responses, mode, store, surf, nil)
}

// newHookGateSession is newGateSession plus the 21-06 hook-verdict head
// wiring: the injected PreToolUseVerdict (a scripted verdict fake or a REAL
// script-hook ecosys.HookRunner) joins the gate deps exactly as the runtime
// composition does — nil keeps the hookless pre-join gate.
func newHookGateSession(
	t *testing.T, responses []provider.Response, mode string,
	store *fakePermStore, surf *fakeGateSurface,
	verdict func(ctx context.Context, tool string, input json.RawMessage) (ecosys.Verdict, string),
) *Session {
	t.Helper()

	s := newTestSessionWithCatalog(t, responses)

	s.SetPermissionGate(GateDeps{
		PreToolUseVerdict: verdict,
		Rules:             store.Rules,
		Mode:              func() string { return mode },
		Allow:             store.Allow,
		Forbid:            store.Forbid,
		Fire:              surf.Fire,
		Queue:             NewAskQueue(),
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

	lines := linesOf(s)

	var out []Line

	for i := range lines {
		l := &lines[i]
		if l.Type == TypeToolResult && l.ToolCallID == callID {
			out = append(out, *l)
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
func TestGatePermissionSuspend(t *testing.T) { //nolint:cyclop,funlen // flat end-to-end battery
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
	// second dialog. Wait on the RESULTS (the second exec's transcript append
	// lands after its noteExec — asserting on the order slice alone races that
	// millisecond window under full-suite load, the 17-01/17-02 flake family).
	close(block)

	gateWaitFor(t, func() bool {
		return len(store.snapshotOrder()) == 3 && // allow + exec(call1) + exec(call2)
			len(toolResultsFor(t, s, gateCall1)) == 1 &&
			len(toolResultsFor(t, s, gateCall2)) == 1
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

// TestGatePermissionSuspend_MultiCallBatch pins the CR-01 fix: TWO gated
// ask-class calls in ONE provider response (a normal batch shape —
// DispatchBatch exists precisely for batches) BOTH suspend. The overwrite bug
// dropped every suspension but the last: the dropped call kept its recorded
// tool_call line but never got an ask_suspended marker, a queue entry, or a
// tool result — the unpaired tool_use then broke the next provider request.
// Pins: two ask_suspended records, two surface firings fired SEQUENTIALLY by
// the queue (one outstanding, D-11), the mid-suspension projection carries NO
// unpaired tool_use (the CR-01 projector pair-safety filter), and both calls
// land non-error results after their answers.
func TestGatePermissionSuspend_MultiCallBatch(t *testing.T) { //nolint:funlen,cyclop,gocyclo // two-dialog chain
	t.Parallel()

	block1 := make(chan struct{})
	block2 := make(chan struct{})

	store := &fakePermStore{}
	surf := &fakeGateSurface{
		answers: []AskOutcome{
			{Selected: acp.PermOptionAllowOnce},
			{Selected: acp.PermOptionAllowOnce},
		},
		block: block1,
	}

	s := newGateSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)},
				{ID: gateCall2, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)},
			},
		},
		{FinishReason: stopEndTurn}, // the first resume's closing turn
		{FinishReason: stopEndTurn}, // the second resume's closing turn
	}, PermModeGated, store, surf)

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want the ask-suspension marker %q", stop, stopAsk)
	}

	// BOTH calls suspended: two ask_suspended audit markers (the overwrite bug
	// produced exactly one).
	sawCall1, sawCall2 := false, false

	for _, l := range linesOf(s) {
		if l.Type != TypeAskSuspended {
			continue
		}

		switch l.ToolCallID {
		case gateCall1:
			sawCall1 = true
		case gateCall2:
			sawCall2 = true
		}
	}

	if !sawCall1 || !sawCall2 {
		t.Fatalf("ask_suspended records: call1=%v call2=%v; want BOTH (every batch suspension survives)",
			sawCall1, sawCall2)
	}

	// The queue fired ONLY the first dialog (one outstanding — D-11): the
	// second suspension is queued behind it, not dropped.
	gateWaitFor(t, func() bool { return surf.fired() == 1 })

	if e := surf.lastEntry(); e == nil || e.CallID != gateCall1 {
		t.Fatalf("first fired entry = %+v; want call1 (batch order preserved)", e)
	}

	// Projection pair-safety mid-suspension: NEITHER call has a result yet, so
	// the projected window must carry ZERO tool_use blocks (a projected
	// unpaired tool_use is what the provider rejects).
	turnID := s.CurrentTurnID()

	if ids := projectedToolUseIDs(t, s, turnID); len(ids) != 0 {
		t.Errorf("projected tool_use ids = %v while both calls are unanswered; want none (pair-safety)", ids)
	}

	// Retarget the NEXT fire's gate to block2 while dialog 1 is still open, so
	// the second dialog deterministically parks mid-round-trip.
	surf.mu.Lock()
	surf.block = block2
	surf.mu.Unlock()

	close(block1) // answer 1 (allow_once) → result lands → resume → dialog 2 parks

	// Dialog 2 fired; call1's result is in, call2's is not.
	gateWaitFor(t, func() bool { return surf.fired() == 2 })

	if e := surf.lastEntry(); e == nil || e.CallID != gateCall2 {
		t.Fatalf("second fired entry = %+v; want call2 (the queued suspension fired, not dropped)", e)
	}

	gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

	if results := toolResultsFor(t, s, gateCall2); len(results) != 0 {
		t.Errorf("call2 has a result before its dialog was answered: %+v", results)
	}

	// THE CR-01 pin: with call2 still unanswered, the resumed projection
	// carries ONLY call1's tool_use — never the unpaired call2 (pre-fix, the
	// whole batch was projected against one result and the provider rejected
	// the resumed request).
	ids := projectedToolUseIDs(t, s, turnID)
	if len(ids) != 1 || ids[0] != gateCall1 {
		t.Errorf("mid-suspension projected tool_use ids = %v; want exactly [call1] (unpaired call2 dropped)", ids)
	}

	close(block2) // answer 2 → result lands → final resume

	gateWaitFor(t, func() bool {
		return len(toolResultsFor(t, s, gateCall1)) == 1 && len(toolResultsFor(t, s, gateCall2)) == 1
	})

	// Both results non-error; the final projection is fully paired.
	for _, call := range []string{gateCall1, gateCall2} {
		results := toolResultsFor(t, s, call)
		if len(results) != 1 || results[0].IsError {
			t.Errorf("tool_result for %s = %+v; want exactly one non-error result", call, results)
		}
	}

	if ids := projectedToolUseIDs(t, s, turnID); len(ids) != 2 {
		t.Errorf("final projected tool_use ids = %v; want both calls (fully paired batch)", ids)
	}

	if got := surf.fired(); got != 2 {
		t.Errorf("surface fired %d times; want exactly 2 (one per suspension, zero re-asks)", got)
	}
}

// projectedToolUseIDs returns every tool-call id carried by the projected
// window's assistant messages (the provider request's tool_use surface).
func projectedToolUseIDs(t *testing.T, s *Session, turnID string) []string {
	t.Helper()

	messages, err := s.Projector.Project(turnID)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var ids []string

	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			ids = append(ids, tc.ID)
		}
	}

	return ids
}

// TestGateResumeRunsUnderResumeSerial pins the CR-02 session-side contract:
// the permission ask's resume (an ASYNC driver — the queue's pump goroutine)
// is routed through the injected resume serializer, never run bare. The
// runtime composition injects the per-session turn mutex; this test injects a
// tracking wrapper and asserts the resume ENTERED it exactly once with zero
// overlap.
func TestGateResumeRunsUnderResumeSerial(t *testing.T) { //nolint:funlen // the serializer tracking battery
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
		{FinishReason: stopEndTurn},
	}, PermModeGated, store, surf)

	var (
		trackMu sync.Mutex

		entered     int
		inFlight    int
		maxInFlight int
	)

	s.SetResumeSerial(func(f func()) {
		trackMu.Lock()
		entered++
		inFlight++

		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}

		trackMu.Unlock()

		f()

		trackMu.Lock()
		inFlight--
		trackMu.Unlock()
	})

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	gateWaitFor(t, func() bool { return surf.fired() == 1 })

	close(block) // the operator answers → the pump resolves → the resume

	gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

	trackMu.Lock()
	defer trackMu.Unlock()

	if entered != 1 {
		t.Errorf("resume serializer entered %d times; want exactly 1 (the async resume must be wrapped)", entered)
	}

	if maxInFlight != 1 {
		t.Errorf("max concurrent serialized resumes = %d; want 1 (per-session turn serialization)", maxInFlight)
	}
}

// TestGatePermissionSuspensionArmsSettle pins the CR-05 fix at the session
// seam: a permission suspension PUBLISHES its settle signal on the broker
// (AskSettleChan — the channel the engine's ask-wait consumes). While the
// dialog is open the channel is live; it closes only after the resumed turn
// returns. Pre-fix the seam stayed nil for the whole permission family, so
// engine chains exited silently at a gated dialog.
func TestGatePermissionSuspensionArmsSettle(t *testing.T) {
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
		{FinishReason: stopEndTurn},
	}, PermModeGated, store, surf)

	// Wire the broker so the settle seam exists (composition always does; the
	// bare gate fixtures skip it because they never consult the seam).
	s.SetAskBroker(context.Background(), NewAskBroker(0, nil))

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want the ask marker", stop)
	}

	gateWaitFor(t, func() bool { return surf.fired() == 1 })

	// THE CR-05 pin: the suspension armed the seam — the engine's wait would
	// block here instead of seeing nil (exit) or a stale channel (hot-spin).
	settle := s.AskSettleChan()
	if settle == nil {
		t.Fatal("AskSettleChan is nil while the permission dialog is open — " +
			"the suspension never armed the broker settle seam (CR-05)")
	}

	select {
	case <-settle:
		t.Fatal("the settle signal closed before the dialog was answered")
	default:
	}

	close(block) // the operator allows → resume runs → settle closes after the turn returns

	// The settle signal closes AFTER the resumed runTurn returns — the 13-00
	// completion discipline, now driven by the dialog answer.
	gateWaitFor(t, func() bool {
		select {
		case <-settle:
			return true
		default:
			return false
		}
	})
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

	// The dispatch completes slightly before its result lands on the
	// transcript (DispatchSubagent's goroutine hands it back) — wait for the
	// append, then assert its form.
	gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

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

// TestGateOutcomeMatrix pins the full dialog outcome matrix (D-01/D-03, both
// directions) plus its untrusted-input leg: reject_once denies WITHOUT
// persisting (the next call asks again), reject_always persists the deny rule
// BEFORE the denial result (the next matching call denies with NO dialog),
// allow_once executes WITHOUT persisting (no trust recorded), and a Selected
// that is empty or non-canonical (untrusted dialog input — the 17-06
// selected-without-optionId wire shape lands here as "") DECLINES fail-safe
// via the gate's default unknown-option branch (WR-01 pin).
//
//nolint:funlen,cyclop,gocyclo,gocognit,maintidx // three full-loop subtests (lint v2 drift)
func TestGateOutcomeMatrix(t *testing.T) {
	t.Parallel()

	t.Run("reject_once denies and asks again", func(t *testing.T) {
		t.Parallel()

		block := make(chan struct{})

		store := &fakePermStore{}
		surf := &fakeGateSurface{
			answers: []AskOutcome{
				{Selected: acp.PermOptionRejectOnce},
				{Selected: acp.PermOptionAllowOnce}, // the second dialog's answer
			},
			block: block,
		}

		s := newGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall2, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
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

		// The rejection lands as a denial result, NO rule is written, and the
		// follow-up identical call asks AGAIN (second fire).
		gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Fatalf("reject_once result = %+v; want exactly one error result", results)
		}

		// The second dialog fires (the call asks AGAIN), its answer resolves,
		// and ONLY the execution is recorded — no rule write in either
		// direction.
		gateWaitFor(t, func() bool { return surf.fired() == 2 })
		gateWaitFor(t, func() bool { return len(store.snapshotOrder()) == 1 })

		if snaps := store.snapshotOrder(); snaps[0] != "exec:"+gateToolWrite {
			t.Errorf("reject_once order = %v; want no rule writes, just the second call's exec", snaps)
		}
	})

	t.Run("reject_always persists deny before the denial", func(t *testing.T) {
		t.Parallel()

		block := make(chan struct{})

		store := &fakePermStore{}
		surf := &fakeGateSurface{
			answers: []AskOutcome{{Selected: acp.PermOptionRejectAlways}},
			block:   block,
		}

		s := newGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall2, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf)

		_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		gateWaitFor(t, func() bool { return surf.fired() == 1 })
		close(block)

		// The deny rule is persisted BEFORE the denial result (D-03), and the
		// follow-up matching call DENIES with NO dialog (so the order slice
		// ends at exactly the one forbid write).
		gateWaitFor(t, func() bool { return len(store.snapshotOrder()) == 1 })

		order := store.snapshotOrder()
		if len(order) != 1 || order[0] != "forbid:"+gateToolWrite {
			t.Fatalf("reject_always order = %v; want exactly [forbid:Write]", order)
		}

		gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Fatalf("reject_always result = %+v; want exactly one error result", results)
		}

		gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall2)) == 1 })

		if surf.fired() != 1 {
			t.Errorf("surface fired %d times; want 1 (the follow-up call denies by rule, no dialog)", surf.fired())
		}

		results2 := toolResultsFor(t, s, gateCall2)
		if len(results2) != 1 || !results2[0].IsError {
			t.Errorf("follow-up denial result = %+v; want one error result", results2)
		}
	})

	t.Run("allow_once executes without persisting", func(t *testing.T) {
		t.Parallel()

		block := make(chan struct{})

		store := &fakePermStore{}
		surf := &fakeGateSurface{
			answers: []AskOutcome{{Selected: acp.PermOptionAllowOnce}},
			block:   block,
		}

		s := newGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf)

		_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		gateWaitFor(t, func() bool { return surf.fired() == 1 })
		close(block)

		gateWaitFor(t, func() bool { return len(store.snapshotOrder()) == 1 })

		order := store.snapshotOrder()
		if len(order) != 1 || order[0] != "exec:"+gateToolWrite {
			t.Fatalf("allow_once order = %v; want execution with NO rule write", order)
		}

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || results[0].IsError {
			t.Errorf("allow_once result = %+v; want exactly one non-error result", results)
		}
	})

	t.Run("unknown option id declines fail-safe", func(t *testing.T) {
		t.Parallel()

		// Both untrusted shapes of a selected answer: the canonical nested
		// outcome minus optionId (the surface passes it through as "" — WR-01)
		// and a non-canonical id nothing offered. The gate's default branch
		// must decline BOTH — an empty selection is never special-cased.
		cases := []struct{ name, selected string }{
			{"empty optionId (selected without optionId)", ""},
			{"non-canonical optionId", "banana_opt"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				block := make(chan struct{})

				store := &fakePermStore{}
				surf := &fakeGateSurface{
					answers: []AskOutcome{{Selected: tc.selected}},
					block:   block,
				}

				s := newGateSession(t, []provider.Response{
					{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
						{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
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

				// THE WR-01 pin: the default unknown-option branch declines —
				// ONE error result carrying the unknown-option decline form
				// (distinct from the deny-rule form), zero executions, zero
				// rule writes in either direction.
				results := toolResultsFor(t, s, gateCall1)
				if len(results) != 1 || !results[0].IsError {
					t.Fatalf("unknown-option result = %+v; want exactly one error result (the decline)", results)
				}

				if !strings.Contains(string(results[0].Output), "Permission declined") ||
					!strings.Contains(string(results[0].Output), "the dialog returned an unknown option") {
					t.Errorf("decline note = %s; want the unknown-option decline form", results[0].Output)
				}

				if order := store.snapshotOrder(); len(order) != 0 {
					t.Errorf("unknown-option answer executed or wrote a rule: %v; want zero activity", order)
				}
			})
		}
	})
}

// TestGateAutomationDecline pins D-07's fail-safe: on an automation-origin
// turn (no human present) an ask-class call DECLINES with a client-visible
// transcript note + structured log — in BOTH modes — while deny and allow
// rules stay enforced. The human-turn control case DOES suspend.
func TestGateAutomationDecline(t *testing.T) { //nolint:funlen,gocognit,cyclop,gocyclo // the four-row matrix
	t.Parallel()

	newAutomationCase := func(
		t *testing.T, mode string, automation bool, deny []string,
	) (*Session, *fakePermStore, *fakeGateSurface) {
		t.Helper()

		store := &fakePermStore{deny: deny}
		surf := &fakeGateSurface{}

		s := newGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, mode, store, surf)

		s.SetTurnOriginAutomation(automation)

		return s, store, surf
	}

	t.Run("gated automation declines without a dialog", func(t *testing.T) {
		t.Parallel()

		s, store, surf := newAutomationCase(t, PermModeGated, true, nil)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn (a decline never suspends)", stop)
		}

		if surf.fired() != 0 {
			t.Errorf("surface fired %d times; want 0 (never a dialog nobody answers)", surf.fired())
		}

		if order := store.snapshotOrder(); len(order) != 0 {
			t.Errorf("declined call executed: %v", order)
		}

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Fatalf("decline result = %+v; want one error result", results)
		}

		if !strings.Contains(string(results[0].Output), "Permission declined") {
			t.Errorf("decline note = %s; want the client-visible decline form", results[0].Output)
		}
	})

	t.Run("ungated automation declines too (D-07 in BOTH modes)", func(t *testing.T) {
		t.Parallel()

		s, _, surf := newAutomationCase(t, PermModeUngated, true, nil)

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

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Fatalf("decline result = %+v; want one error result (never a silent allow)", results)
		}
	})

	t.Run("rules still enforced on automation turns", func(t *testing.T) {
		t.Parallel()

		t.Run("deny rule denies", func(t *testing.T) {
			t.Parallel()

			s, _, surf := newAutomationCase(t, PermModeGated, true, []string{gateToolWrite})

			_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
			if err != nil {
				t.Fatalf("Prompt: %v", err)
			}

			results := toolResultsFor(t, s, gateCall1)
			if len(results) != 1 || !results[0].IsError ||
				!strings.Contains(string(results[0].Output), "Permission denied") {
				t.Fatalf("deny-on-automation result = %+v; want the DENIED form", results)
			}

			if surf.fired() != 0 {
				t.Errorf("surface fired %d times; want 0", surf.fired())
			}
		})

		t.Run("allow rule executes", func(t *testing.T) {
			t.Parallel()

			block := make(chan struct{})
			close(block) // unused in this path; keep the surface inert

			store := &fakePermStore{allow: []string{gateToolWrite}}
			surf := &fakeGateSurface{block: block}

			s := newGateSession(t, []provider.Response{
				{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
					{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
				{FinishReason: stopEndTurn},
			}, PermModeGated, store, surf)

			s.SetTurnOriginAutomation(true)

			stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
			if err != nil {
				t.Fatalf("Prompt: %v", err)
			}

			if stop != stopEndTurn {
				t.Fatalf("stop = %q; want end_turn (an allow rule short-circuits the decline)", stop)
			}

			if order := store.snapshotOrder(); len(order) != 1 || order[0] != "exec:"+gateToolWrite {
				t.Fatalf("allow-on-automation order = %v; want exactly one exec", order)
			}

			if surf.fired() != 0 {
				t.Errorf("surface fired %d times; want 0", surf.fired())
			}
		})
	})

	t.Run("human control case suspends", func(t *testing.T) {
		t.Parallel()

		block := make(chan struct{})

		store := &fakePermStore{}
		surf := &fakeGateSurface{block: block}

		s := newGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf)

		// The origin marker defaults to false (client turn) — set it
		// explicitly to pin the per-turn signal's polarity.
		s.SetTurnOriginAutomation(false)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopAsk {
			t.Fatalf("stop = %q; want the ask marker (the human control case suspends)", stop)
		}

		gateWaitFor(t, func() bool { return surf.fired() == 1 })
		close(block)
	})
}

// TestGateDegradedClient pins the -32601 fail-safe (16-D-18): a client that
// cannot answer permission asks produces a decline (never a silent allow)
// and the degradation is STICKY for the session — the next gated call
// declines without a new surface round-trip. A generic (transient) failure
// is NOT sticky: the next call asks again.
func TestGateDegradedClient(t *testing.T) { //nolint:funlen // two full-loop subtests
	t.Parallel()

	t.Run("unsupported client is sticky", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{
			answers: []AskOutcome{{Err: errFakeUnsupported, Unsupported: true}},
		}

		s := newGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall2, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopAsk {
			t.Fatalf("stop = %q; want the ask marker (the first ask still fires before degrading)", stop)
		}

		gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError ||
			!strings.Contains(string(results[0].Output), "Permission declined") {
			t.Fatalf("degraded result = %+v; want the decline form", results)
		}

		if order := store.snapshotOrder(); len(order) != 0 {
			t.Errorf("degraded call executed: %v", order)
		}

		// The second gated call declines WITHOUT a new surface round-trip.
		gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall2)) == 1 })

		if surf.fired() != 1 {
			t.Errorf("surface fired %d times; want 1 (sticky degradation — no retry storm)", surf.fired())
		}
	})

	t.Run("transient failure is not sticky", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{
			answers: []AskOutcome{
				{Err: errFakeTransient},
				{Selected: acp.PermOptionAllowOnce},
			},
		}

		s := newGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall2, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf)

		_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

		// The second gated call asks AGAIN (a transient failure never
		// widens into a sticky degradation).
		gateWaitFor(t, func() bool { return surf.fired() == 2 })
		gateWaitFor(t, func() bool { return len(store.snapshotOrder()) == 1 })
	})
}

// Test-local failure sentinels for the degraded-client battery.
var (
	errFakeUnsupported = errors.New("fake: client does not implement session/request_permission")
	errFakeTransient   = errors.New("fake: transient registry failure")
)

// TestGateMCPNamespace pins the rule-subject namespace mapping (Pitfall 7):
// the gate's rule subject for a fake-registered MCP tool is the canonical
// mcp__server__tool name (via 17-01's MCPName/SplitMCPName) — a rule written
// in that namespace matches, and a bare-server rule selects the whole
// namespace. Both directions of the mapping are pinned.
func TestGateMCPNamespace(t *testing.T) { //nolint:funlen,cyclop // the mapping table reads as one flow
	t.Parallel()

	const (
		mcpSrv       = "srv"
		mcpTool      = "tool1"
		mcpFullName  = "mcp__srv__tool1"
		mcpSrvBare   = "mcp__srv"
		mcpInputTest = `{"query":"x"}`
	)

	// Direction 1 (build): the gate subject of the fake-registered tool equals
	// the MCPName-built namespace name; Direction 2 (split): SplitMCPName
	// recovers the pair.
	s := newGateSession(t, []provider.Response{{FinishReason: stopEndTurn}}, PermModeGated,
		&fakePermStore{}, &fakeGateSurface{})

	s.Catalog.Register(toolcat.Tool{
		Name:        mcpFullName,
		Mutability:  toolcat.MutabilityMutating,
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"output":"mcp ok"}`), nil
		},
	})

	subject := s.ruleSubject(mcpFullName)
	if subject != perm.MCPName(mcpSrv, mcpTool) {
		t.Errorf("ruleSubject = %q; want the canonical %q", subject, perm.MCPName(mcpSrv, mcpTool))
	}

	gotSrv, gotTool, ok := perm.SplitMCPName(subject)
	if !ok || gotSrv != mcpSrv || gotTool != mcpTool {
		t.Errorf("SplitMCPName(%q) = (%q,%q,%v); want (srv,tool1,true)", subject, gotSrv, gotTool, ok)
	}

	// Behavior: a rule in the namespace form matches the fake-registered
	// tool's gate subject (allow → executes, no dialog); a bare-server rule
	// selects the whole namespace (deny → denied, no dialog).
	newMCPCase := func(t *testing.T, rule, ruleList string) (*Session, *fakePermStore, *fakeGateSurface) {
		t.Helper()

		store := &fakePermStore{}

		switch ruleList {
		case "deny":
			store.deny = []string{rule}
		case "allow":
			store.allow = []string{rule}
		}

		surf := &fakeGateSurface{}

		s := newGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: mcpFullName, Input: json.RawMessage(mcpInputTest)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf)

		s.Catalog.Register(toolcat.Tool{
			Name:        mcpFullName,
			Mutability:  toolcat.MutabilityMutating,
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Execute: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
				store.noteExec(mcpFullName)

				return json.RawMessage(`{"output":"mcp ok"}`), nil
			},
		})

		return s, store, surf
	}

	t.Run("allow rule in namespace form executes", func(t *testing.T) {
		t.Parallel()

		s, store, surf := newMCPCase(t, mcpFullName, "allow")

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn || surf.fired() != 0 {
			t.Fatalf("stop=%q fired=%d; want end_turn, zero dialogs (allow matched)", stop, surf.fired())
		}

		if order := store.snapshotOrder(); len(order) != 1 || order[0] != "exec:"+mcpFullName {
			t.Errorf("order = %v; want exactly one mcp exec", order)
		}
	})

	t.Run("bare-server deny selects the whole namespace", func(t *testing.T) {
		t.Parallel()

		s, store, surf := newMCPCase(t, mcpSrvBare, "deny")

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn || surf.fired() != 0 {
			t.Fatalf("stop=%q fired=%d; want end_turn with zero dialogs", stop, surf.fired())
		}

		if order := store.snapshotOrder(); len(order) != 0 {
			t.Errorf("denied mcp call executed: %v", order)
		}

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Errorf("denial result = %+v; want one error result", results)
		}
	})
}

// TestGatePermissionModeFlip pins criterion 4's live-flip leg at the session
// level (Pitfall 8): the permissions.mode handler's apply hook flips the
// SHARED mode accessor, and the very next mutating call in the SAME session
// changes gate behavior — no session recreation, no restart. Flipping back
// restores the dialog-free default with deny rules still enforced.
func TestGatePermissionModeFlip(t *testing.T) { //nolint:funlen // one flow across three prompts
	t.Parallel()

	mode := PermModeUngated

	store := &fakePermStore{}
	surf := &fakeGateSurface{
		answers: []AskOutcome{{Selected: acp.PermOptionAllowOnce}},
	}

	// The fake provider serves responses per STREAM (each turn loop iteration
	// is one stream): prompt 1 = [Write, end_turn]; prompt 2 = [Write] (the
	// gated suspension; the resume executes and closes on the next stream);
	// prompt 3 = [Write] (denied ungated) + [end_turn].
	s := newGateSessionFunc(t, []provider.Response{
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
		{FinishReason: stopEndTurn},
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: gateCall2, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
		{FinishReason: stopEndTurn},
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
		{FinishReason: stopEndTurn},
	}, func() string { return mode }, store, surf)

	// Ungated (the boot default): the first identical call executes with zero
	// surface invocations.
	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("prompt 1: %v", err)
	}

	if stop != stopEndTurn || surf.fired() != 0 {
		t.Fatalf("stop=%q fired=%d; want end_turn/0 under the ungated boot default", stop, surf.fired())
	}

	// The handler's apply hook flips the accessor — no session recreation.
	mode = PermModeGated

	stop, err = s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("prompt 2: %v", err)
	}

	if stop != stopAsk {
		t.Fatalf("stop = %q; want the ask marker (the flip reached the RUNNING session)", stop)
	}

	gateWaitFor(t, func() bool { return surf.fired() == 1 })

	// The suspended call resolves (allow_once — no rule written) and only the
	// two executions are recorded: turn 1's ungated exec + the resumed gated
	// exec. No allow/forbid entry ever lands.
	gateWaitFor(t, func() bool { return len(store.snapshotOrder()) == 2 })

	if order := store.snapshotOrder(); order[0] != "exec:"+gateToolWrite || order[1] != "exec:"+gateToolWrite {
		t.Fatalf("order = %v; want exactly the two execs, no rule writes (allow_once)", order)
	}

	// Flip back: the next identical call executes with no dialog; a deny rule
	// still denies in the dialog-free mode.
	mode = PermModeUngated
	store.deny = []string{gateToolWrite}

	stop, err = s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
	if err != nil {
		t.Fatalf("prompt 3: %v", err)
	}

	if stop != stopEndTurn {
		t.Fatalf("stop = %q; want end_turn after the flip back", stop)
	}

	if surf.fired() != 1 {
		t.Errorf("surface fired %d times; want 1 (flip-back restored the dialog-free default)", surf.fired())
	}

	// The resume chain settles asynchronously (the queue's pump may consume
	// the next stream either side of prompt 3) — wait for the CONVERGED
	// state: two call results, the second the ungated deny-rule denial.
	gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 2 })

	results := toolResultsFor(t, s, gateCall1)
	if len(results) != 2 {
		t.Fatalf("deny-rule results = %d; want 2 (turn 1 ungated exec + turn 3 ungated deny)", len(results))
	}

	if !results[1].IsError {
		t.Errorf("flip-back denial result = %+v; want an error result (deny still denies ungated)", results[1])
	}
}

// newGateSessionFunc is newGateSession with a live mode accessor (the
// TestGatePermissionModeFlip seam — the same shared accessor the
// permissions.mode apply hook flips).
func newGateSessionFunc(
	t *testing.T, responses []provider.Response, modeFn func() string,
	store *fakePermStore, surf *fakeGateSurface,
) *Session {
	t.Helper()

	s := newTestSessionWithCatalog(t, responses)

	s.SetPermissionGate(GateDeps{
		Rules:  store.Rules,
		Mode:   modeFn,
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

// ── 21-06 Task 1: the hook-verdict head battery (PAR-03, D-01/D-03/D-04) ──
//
// The joined head, end-to-end: REAL script hooks (printf'd
// hookSpecificOutput JSON through the real ecosys runner + deny-wins
// resolver) and scripted verdict fakes drive the gate — deny blocks at the
// chokepoint with the FIRST denying hook's reason in BOTH dispatch branches,
// ask opens the dialog EVEN UNGATED (D-04), a user-scope allow executes
// without consulting rules or opening a dialog, and no-decision falls
// through to the untouched 17-02 tree below the head.

// hookVerdictJSON renders one CC hookSpecificOutput verdict stdout body
// (decision + reason; the battery's reasons carry no characters a JSON
// string would escape).
func hookVerdictJSON(decision, reason string) string {
	return `{"hookSpecificOutput":{"permissionDecision":"` + decision +
		`","permissionDecisionReason":"` + reason + `"}}`
}

// hookPrintfCmd wraps a stdout body in a printf script (single-quoted — the
// battery's bodies carry no single quotes).
func hookPrintfCmd(body string) string {
	return "printf '%s' '" + body + "'"
}

// hookDeny/hookAsk/hookAllow build one PreToolUse HookConfig emitting the
// given verdict JSON at the given discovery scope.
func hookDeny(matcher, reason string, scope ecosys.HookScope) ecosys.HookConfig {
	return ecosys.HookConfig{
		Event: "PreToolUse", Matcher: matcher, Scope: scope,
		Command: hookPrintfCmd(hookVerdictJSON("deny", reason)),
	}
}

func hookAllow(matcher, reason string, scope ecosys.HookScope) ecosys.HookConfig {
	return ecosys.HookConfig{
		Event: "PreToolUse", Matcher: matcher, Scope: scope,
		Command: hookPrintfCmd(hookVerdictJSON("allow", reason)),
	}
}

// TestGateHookVerdict is the 21-06 join battery: settings.json hook
// verdicts consume at Phase 17's single gateCall head.
func TestGateHookVerdict(t *testing.T) { //nolint:funlen,cyclop,gocyclo,gocognit,maintidx // the joined-head matrix
	t.Parallel()

	t.Run("script deny blocks end-to-end (batch branch)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		runner := ecosys.NewHookRunner([]ecosys.HookConfig{
			hookDeny(gateToolWrite, "no writes today", ecosys.ScopeProject),
		}, "sess-hook-deny", t.TempDir(), "")

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeUngated, store, surf, runner.PreToolUseVerdict)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn (a hook deny appends the denial, the turn continues)", stop)
		}

		if order := store.snapshotOrder(); len(order) != 0 {
			t.Errorf("hook-denied call executed: %v", order)
		}

		if surf.fired() != 0 {
			t.Errorf("surface fired %d times; want 0 (a deny never opens a dialog)", surf.fired())
		}

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Fatalf("hook-deny result = %+v; want exactly one error result (the transcript denial line)", results)
		}

		if !strings.Contains(string(results[0].Output), "Permission denied") ||
			!strings.Contains(string(results[0].Output), "no writes today") {
			t.Errorf("hook-deny form = %s; want the structured denial carrying the hook's reason", results[0].Output)
		}
	})

	t.Run("script deny blocks end-to-end (subagent branch)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		runner := ecosys.NewHookRunner([]ecosys.HookConfig{
			hookDeny(gateToolTask, "no subagents today", ecosys.ScopeProject),
		}, "sess-hook-deny-sub", t.TempDir(), "")

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolTask, Input: json.RawMessage(`{"prompt":"explore"}`)}}},
			{FinishReason: stopEndTurn},
		}, PermModeUngated, store, surf, runner.PreToolUseVerdict)

		s.Catalog.Register(toolcat.Tool{
			Name:        gateToolTask,
			Mutability:  toolcat.MutabilityMutating,
			InputSchema: json.RawMessage(`{"type":"object"}`),
		})

		sub := &countingSubagentRunner{}
		s.subagentRunner = sub

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn", stop)
		}

		if sub.dispatched() != 0 {
			t.Errorf("subagent dispatched %d times; want 0 (the hook deny sits BEFORE DispatchSubagent)", sub.dispatched())
		}

		if order := store.snapshotOrder(); len(order) != 0 {
			t.Errorf("hook-denied subagent call executed: %v", order)
		}

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError ||
			!strings.Contains(string(results[0].Output), "no subagents today") {
			t.Fatalf("subagent-branch hook-deny result = %+v; want the structured denial with the reason", results)
		}
	})

	t.Run("the FIRST denying hook's reason wins (D-03)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		runner := ecosys.NewHookRunner([]ecosys.HookConfig{
			hookDeny("", "first-deny-reason-marker", ecosys.ScopeProject),
			hookDeny("", "second-deny-reason-marker", ecosys.ScopeProject),
		}, "sess-hook-deny-order", t.TempDir(), "")

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeUngated, store, surf, runner.PreToolUseVerdict)

		_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Fatalf("deny-order result = %+v; want one error result", results)
		}

		if !strings.Contains(string(results[0].Output), "first-deny-reason-marker") {
			t.Errorf("deny reason = %s; want the FIRST denying hook's reason", results[0].Output)
		}

		if strings.Contains(string(results[0].Output), "second-deny-reason-marker") {
			t.Errorf("deny reason = %s; must not carry the second denying hook's reason", results[0].Output)
		}
	})

	t.Run("hook ask in ungated mode opens the dialog (D-04)", func(t *testing.T) {
		t.Parallel()

		block := make(chan struct{})

		store := &fakePermStore{}
		surf := &fakeGateSurface{
			answers: []AskOutcome{{Selected: acp.PermOptionAllowOnce}},
			block:   block,
		}

		askVerdict := func(context.Context, string, json.RawMessage) (ecosys.Verdict, string) {
			return ecosys.VerdictAsk, "operator review required"
		}

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeUngated, store, surf, askVerdict)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopAsk {
			t.Fatalf("stop = %q; want the ask marker (a hook ask suspends EVEN UNGATED — D-04)", stop)
		}

		gateWaitFor(t, func() bool { return surf.fired() == 1 })

		if order := store.snapshotOrder(); len(order) != 0 {
			t.Fatalf("executor ran while the hook-asked dialog was open: %v", order)
		}

		close(block) // the operator answers allow_once

		// Wait on the RESULT, not the order slice alone (noteExec fires inside
		// the tool; the transcript append lands after it — the 17-01/17-02
		// flake family's documented discipline).
		gateWaitFor(t, func() bool {
			return len(store.snapshotOrder()) == 1 && len(toolResultsFor(t, s, gateCall1)) == 1
		})

		if order := store.snapshotOrder(); order[0] != "exec:"+gateToolWrite {
			t.Errorf("allow_once on the hook ask = %v; want exactly the exec (no rule write)", order)
		}

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || results[0].IsError {
			t.Errorf("hook-ask allow_once result = %+v; want one non-error result", results)
		}
	})

	t.Run("hook ask answered reject_once denies without a rule write", func(t *testing.T) {
		t.Parallel()

		block := make(chan struct{})

		store := &fakePermStore{}
		surf := &fakeGateSurface{
			answers: []AskOutcome{{Selected: acp.PermOptionRejectOnce}},
			block:   block,
		}

		askVerdict := func(context.Context, string, json.RawMessage) (ecosys.Verdict, string) {
			return ecosys.VerdictAsk, "operator review required"
		}

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeUngated, store, surf, askVerdict)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopAsk {
			t.Fatalf("stop = %q; want the ask marker", stop)
		}

		gateWaitFor(t, func() bool { return surf.fired() == 1 })

		close(block) // the operator answers reject_once

		gateWaitFor(t, func() bool { return len(toolResultsFor(t, s, gateCall1)) == 1 })

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Fatalf("reject_once result = %+v; want one error result (the denial)", results)
		}

		if order := store.snapshotOrder(); len(order) != 0 {
			t.Errorf("reject_once order = %v; want zero activity (no exec, no rule write)", order)
		}
	})

	t.Run("hookless ungated control stays dialog-free (criterion 4 coexists)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		// No verdict wired: the pre-join hookless gate — the join added ZERO
		// dialogs to hookless sessions (17's criterion 4 inside the join
		// battery).
		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeUngated, store, surf, nil)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn", stop)
		}

		if surf.fired() != 0 {
			t.Errorf("surface fired %d times; want 0 (hookless ungated never opens a dialog)", surf.fired())
		}

		if order := store.snapshotOrder(); len(order) != 1 || order[0] != "exec:"+gateToolWrite {
			t.Errorf("order = %v; want exactly one exec", order)
		}
	})

	t.Run("user-scope allow executes without rules or dialog (D-01)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		runner := ecosys.NewHookRunner([]ecosys.HookConfig{
			hookAllow(gateToolWrite, "trusted operator flow", ecosys.ScopeUser),
		}, "sess-hook-allow", t.TempDir(), "")

		// GATED mode + ask-class tool + no rules: without the hook this call
		// suspends — the user allow is the ONLY thing that can skip the ask.
		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf, runner.PreToolUseVerdict)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn (the user-scope allow executes, no suspension)", stop)
		}

		if surf.fired() != 0 {
			t.Errorf("surface fired %d times; want 0 (the allow consulted no dialog)", surf.fired())
		}

		if order := store.snapshotOrder(); len(order) != 1 || order[0] != "exec:"+gateToolWrite {
			t.Errorf("user-allow order = %v; want exactly one exec, NO rule write (the allow is hook-scoped)", order)
		}
	})

	t.Run("the same call without the hook asks (the allow was hook-scoped)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf, nil)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopAsk {
			t.Fatalf("stop = %q; want the ask marker (the hook's allow never persisted — rules/mode apply again)", stop)
		}

		gateWaitFor(t, func() bool { return surf.fired() == 1 })
	})

	t.Run("silent hook falls through to the rules (no decision)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		runner := ecosys.NewHookRunner([]ecosys.HookConfig{
			{Event: "PreToolUse", Matcher: gateToolWrite, Command: "exit 0", Scope: ecosys.ScopeProject},
		}, "sess-hook-silent", t.TempDir(), "")

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeUngated, store, surf, runner.PreToolUseVerdict)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn", stop)
		}

		if order := store.snapshotOrder(); len(order) != 1 || order[0] != "exec:"+gateToolWrite {
			t.Errorf("silent-hook order = %v; want the pre-join ungated fall-through (one exec)", order)
		}

		if surf.fired() != 0 {
			t.Errorf("surface fired %d times; want 0", surf.fired())
		}
	})

	t.Run("timed-out hook falls through to the rules (fail-open, PAR-03)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		runner := ecosys.NewHookRunner([]ecosys.HookConfig{
			{Event: "PreToolUse", Matcher: gateToolWrite, Command: "sleep 5", TimeoutSec: 1, Scope: ecosys.ScopeUser},
		}, "sess-hook-timeout", t.TempDir(), "")

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeUngated, store, surf, runner.PreToolUseVerdict)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn", stop)
		}

		if order := store.snapshotOrder(); len(order) != 1 || order[0] != "exec:"+gateToolWrite {
			t.Errorf("timed-out-hook order = %v; want the fail-open fall-through (one exec)", order)
		}

		if surf.fired() != 0 {
			t.Errorf("surface fired %d times; want 0", surf.fired())
		}
	})
}

// TestGateHookAskFailSafes pins the hook-ask × fail-safe interactions
// (21-REVIEW WR-01): a PreToolUse ask verdict is still an ASK, so the D-07
// automation decline and the 16-D-18 degraded-client guard apply BEFORE the
// suspension — a hook ask never opens a dialog nobody would answer and never
// re-fires a surface round-trip a sticky-degraded client cannot answer. The
// human + capable-client control still suspends (the escalation lever's
// ungated suspend is pinned by the TestGateHookVerdict battery above).
func TestGateHookAskFailSafes(t *testing.T) {
	t.Parallel()

	askVerdict := func(context.Context, string, json.RawMessage) (ecosys.Verdict, string) {
		return ecosys.VerdictAsk, "operator review required"
	}

	t.Run("automation turn declines fail-safe (D-07), even ungated", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeUngated, store, surf, askVerdict)

		s.SetTurnOriginAutomation(true)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn (a hook ask on an automation turn declines, never suspends)", stop)
		}

		if surf.fired() != 0 {
			t.Errorf("surface fired %d times; want 0 (no dialog nobody would answer)", surf.fired())
		}

		if order := store.snapshotOrder(); len(order) != 0 {
			t.Errorf("declined hook-asked call executed: %v", order)
		}

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Fatalf("automation-decline result = %+v; want one error result", results)
		}

		if !strings.Contains(string(results[0].Output), "Permission declined") ||
			!strings.Contains(string(results[0].Output), "no human is present") {
			t.Errorf("automation-decline note = %s; want the D-07 decline form", results[0].Output)
		}
	})

	t.Run("degraded client declines without a surface round-trip (16-D-18)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}
		surf := &fakeGateSurface{}

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf, askVerdict)

		s.SetTurnOriginAutomation(false) // human turn — only the degradation declines
		s.markPermDegraded()

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn (a hook ask on a degraded client declines, never suspends)", stop)
		}

		if surf.fired() != 0 {
			t.Errorf("surface fired %d times; want 0 (the sticky guard suppresses the round-trip)", surf.fired())
		}

		if order := store.snapshotOrder(); len(order) != 0 {
			t.Errorf("declined hook-asked call executed: %v", order)
		}

		results := toolResultsFor(t, s, gateCall1)
		if len(results) != 1 || !results[0].IsError {
			t.Fatalf("degraded-decline result = %+v; want one error result", results)
		}

		if !strings.Contains(string(results[0].Output), "Permission declined") ||
			!strings.Contains(string(results[0].Output), "cannot answer permission asks") {
			t.Errorf("degraded-decline note = %s; want the 16-D-18 decline form", results[0].Output)
		}
	})

	t.Run("human turn with a capable client still suspends (gated control)", func(t *testing.T) {
		t.Parallel()

		store := &fakePermStore{}

		block := make(chan struct{})
		surf := &fakeGateSurface{block: block}

		s := newHookGateSession(t, []provider.Response{
			{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
				{ID: gateCall1, Name: gateToolWrite, Input: json.RawMessage(gatePathInput)}}},
			{FinishReason: stopEndTurn},
		}, PermModeGated, store, surf, askVerdict)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: gatePrompt}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopAsk {
			t.Fatalf("stop = %q; want the ask marker (the fail-safes must not swallow a askable hook ask)", stop)
		}

		gateWaitFor(t, func() bool { return surf.fired() == 1 })
		close(block)
	})
}
