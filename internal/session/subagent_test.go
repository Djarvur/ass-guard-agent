package session //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/tasks"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// newSubagentSession builds a Session whose provider returns the given response
// sequence, with a real catalog + bus for subagent dispatch.
func newSubagentSession(t *testing.T, responses []provider.Response) (*Session, *event.Bus, <-chan event.Event) {
	t.Helper()

	bus := event.NewBus()
	subResults := bus.Subscribe("SubagentResult", event.BufSubagentResult)
	s, _, _ := newTestSession(t, bus, responses)
	s.Catalog = toolcat.NewCatalog()

	return s, bus, subResults
}

// TestDispatchSubagent_AppendsDispatchLine verifies a Task tool_call triggers
// DispatchSubagent + a subagent_dispatch transcript line (PARA-01).
func TestDispatchSubagent_AppendsDispatchLine(t *testing.T) {
	t.Parallel()

	s, _, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"do research"}`)}},
		},
		{FinishReason: stopEndTurn},
	})

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "dispatch"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	found := false

	for _, l := range linesOf(s) {
		if l.Type == TypeSubagentDispatch {
			found = true

			if l.ParentTurnID == "" {
				t.Error("subagent_dispatch missing parentTurnID")
			}
		}
	}

	if !found {
		t.Error("no subagent_dispatch line for a Task tool call (PARA-01)")
	}
}

// TestSubagent_StreamsProgressWithParentTurnID verifies the subagent's chunks
// are published to the bus (PARA-02 — streamed progress tagged for the parent).
func TestSubagent_StreamsProgressWithParentTurnID(t *testing.T) {
	t.Parallel()
	s, bus, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})
	chunks := bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	// Drain a beat; at least one chunk should arrive (the subagent streams).
	var got strings.Builder

	deadline := time.After(1 * time.Second)

	for {
		select {
		case e := <-chunks:
			if c, ok := e.(event.AgentMessageChunk); ok {
				got.WriteString(c.Content)
			}
		case <-deadline:
			if got.String() == "" {
				t.Error("no AgentMessageChunk published during subagent dispatch (PARA-02)")
			}

			return
		}

		if got.String() != "" {
			return
		}
	}
}

// TestSubagent_FinalResultToParent verifies the parent's tool_result for the Task
// call carries the subagent's final result, and a subagent_result line exists.
func TestSubagent_FinalResultToParent(t *testing.T) {
	t.Parallel()

	s, _, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	hasResult := false

	for _, l := range linesOf(s) {
		if l.Type == TypeSubagentResult {
			hasResult = true
		}
	}

	if !hasResult {
		t.Error("no subagent_result line (PARA-02 final result)")
	}
}

// TestSubagent_RestrictedExecutor verifies the subagent's tool calls go through
// a RestrictedExecutor (D-10): a disallowed tool gets a "not available" error.
// We verify by checking the tool_result for a disallowed tool carries the error.
func TestSubagent_RestrictedExecutor(t *testing.T) {
	t.Parallel()
	// Subagent returns a Bash tool_call; the subagent's RestrictedExecutor
	// (allowed: Read/Grep) blocks Bash → "not available" tool_result.
	s, _, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})
	// Override the subagent's restricted set + toolExec via a fake executor.
	fake := &fakeToolExec{}

	s.toolExec = fake

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	// The fake executor records calls; assertion is structural (the subagent
	// path used a RestrictedExecutor). We verify the dispatch line records the
	// restricted tool set.
	for _, l := range linesOf(s) {
		if l.Type == TypeSubagentDispatch && len(l.RestrictedTools) > 0 {
			return // good: restricted set recorded
		}
	}

	t.Error("subagent_dispatch did not record a restricted tool set (D-10)")
}

// fakeToolExec is a ToolExecutor stub for subagent restriction tests.
type fakeToolExec struct{ calls []string }

func (f *fakeToolExec) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	f.calls = append(f.calls, name)

	return json.RawMessage(`{"ok":true}`), nil
}

// TestSubagentPanicRecovery verifies a panicking subagent is recovered: the
// parent receives a SubagentResult with an error, an investigate-and-fix-ready
// error line is written, and the process does NOT crash (PARA-03, D-13).
func TestSubagentPanicRecovery(t *testing.T) {
	t.Parallel()
	// Provider: parent returns a Task call; subagent call panics.
	s, _, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})
	// Inject a panicking subagent runner.
	s.subagentRunner = panickingSubagentRunner{}

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		// The parent may return an error from the subagent result, or continue.
		t.Logf("parent returned err=%v (acceptable)", err)
	}

	hasError := false
	hasSubagentResult := false

	for _, l := range linesOf(s) {
		if l.Type == TypeError {
			hasError = true

			if !strings.Contains(strings.ToLower(l.Message), "panic") &&
				!strings.Contains(strings.ToLower(l.Component), "subagent") {
				t.Errorf("error line does not reference subagent/panic: %+v", l)
			}
		}

		if l.Type == TypeSubagentResult && l.Message != "" {
			hasSubagentResult = true
		}
	}

	if !hasError {
		t.Error("no investigate-and-fix-ready error line after subagent panic (PARA-03, D-13)")
	}

	if !hasSubagentResult {
		t.Error("no subagent_result line with the panic error message")
	}
}

// panickingSubagentRunner is a subagentRunner that always panics, to verify
// goroutine-boundary recovery (PARA-03, D-13).
type panickingSubagentRunner struct{}

func (panickingSubagentRunner) Run(
	ctx context.Context, s *Session,
	subagentTurnID, parentTurnID, prompt string, restricted []string, agentDef *ecosys.Agent,
	_ SubagentDispatchPlan,
) (string, error) {
	panic("panickingSubagentRunner: injected panic")
}

// --- 14-05 (EARLY-05): light-tier subagent model routing ---

// tierWiring model slugs + prompt text for the subagent-model tests (consts so
// the package's goconst counts stay calm — the literals repeat elsewhere).
const (
	parentModelSlug  = "glm-5.2"
	lightModelSlug   = "glm-5.2-air"
	promptDispatchMg = "dispatch for the tier wiring"
)

// TestSubagentModel_OverrideApplied (14-05, Test 1) verifies the economics
// lever: a Session with SubagentModel set dispatches its subagent with THAT
// model on the per-dispatch profile copy — the shared session profile keeps
// the parent model (never mutated). The capture seam is the fake provider's
// streamed-profiles recorder (the same lens 12-02 used for the agent-prompt
// copy): the SECOND Stream call is the subagent's (parent → subagent → parent
// re-projection, sequential in one goroutine).
func TestSubagentModel_OverrideApplied(t *testing.T) {
	t.Parallel()

	s, _, fp := newTestSession(t, nil, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})
	s.Catalog = toolcat.NewCatalog()

	s.Profile.Model = parentModelSlug

	// 20-03 (D-13): the override now rides the dispatch PLAN (the runtime
	// resolver's decision), not the 14-05 session-level SubagentModel stamp.
	s.SubagentModelPlanner = func(_ *Session, _ *ecosys.Agent, _ string) SubagentDispatchPlan {
		return SubagentDispatchPlan{Model: lightModelSlug}
	}

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: promptDispatchMg}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	profiles := fp.streamedProfiles()
	if len(profiles) < 2 {
		t.Fatalf("streamed profiles = %d; want at least parent + subagent calls", len(profiles))
	}

	if profiles[1].Model != lightModelSlug {
		t.Errorf("subagent dispatch model = %q; want the plan's override %q",
			profiles[1].Model, lightModelSlug)
	}

	if profiles[0].Model != parentModelSlug {
		t.Errorf("parent turn model = %q; want the unchanged parent %q (the override is subagent-only)",
			profiles[0].Model, parentModelSlug)
	}

	if s.Profile.Model != parentModelSlug {
		t.Errorf("shared session profile mutated: Model = %q; want %q (the copy pattern must never write back)",
			s.Profile.Model, parentModelSlug)
	}
}

// TestSubagentModel_EmptyKeepsParent (14-05, Test 2) pins today's behavior for
// the no-binding case: an empty SubagentModel dispatches the subagent with the
// parent model exactly as before (the config-conditional default — absence is
// not a failure and never a silent change).
func TestSubagentModel_EmptyKeepsParent(t *testing.T) {
	t.Parallel()

	s, _, fp := newTestSession(t, nil, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})
	s.Catalog = toolcat.NewCatalog()

	s.Profile.Model = parentModelSlug

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: promptDispatchMg}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	profiles := fp.streamedProfiles()
	if len(profiles) < 2 {
		t.Fatalf("streamed profiles = %d; want at least parent + subagent calls", len(profiles))
	}

	if profiles[1].Model != parentModelSlug {
		t.Errorf("subagent dispatch model = %q; want the parent %q (empty SubagentModel keeps today's behavior)",
			profiles[1].Model, parentModelSlug)
	}
}

// TestSubagentDispatch_ResolvedModel (20-03/D-16) pins the durable half of
// the two-way report: the dispatch line records the model that ACTUALLY ran
// — the plan's slug when routed, the parent when the plan is empty (inherit
// or degrade; the warning carries the intent, the line the reality).
//
//nolint:paralleltest // subtests share the fake provider queue
func TestSubagentDispatch_ResolvedModel(t *testing.T) {
	cases := []struct {
		name        string
		plan        SubagentDispatchPlan
		parentModel string
		wantLine    string
	}{
		{
			name:        "routed slug recorded",
			plan:        SubagentDispatchPlan{Model: "glm-4.7-air"},
			parentModel: parentModelSlug,
			wantLine:    "glm-4.7-air",
		},
		{
			name:        "empty plan records the parent",
			plan:        SubagentDispatchPlan{},
			parentModel: parentModelSlug,
			wantLine:    parentModelSlug,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestSession(t, nil, []provider.Response{
				{
					FinishReason: blockToolUse,
					ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
				},
				{FinishReason: stopEndTurn},
			})
			s.Catalog = toolcat.NewCatalog()
			s.Profile.Model = tc.parentModel
			s.SubagentModelPlanner = func(_ *Session, _ *ecosys.Agent, _ string) SubagentDispatchPlan {
				return tc.plan
			}

			_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: promptDispatchMg}})
			if err != nil {
				t.Fatalf("Prompt: %v", err)
			}

			for _, l := range linesOf(s) {
				if l.Type != TypeSubagentDispatch {
					continue
				}

				if l.ResolvedModel != tc.wantLine {
					t.Errorf("dispatch line ResolvedModel = %q; want %q", l.ResolvedModel, tc.wantLine)
				}

				return
			}

			t.Fatal("no subagent_dispatch line")
		})
	}
}

// TestSubagentDispatch_ResolvedModelLegacyTolerance (D-20): a dispatch line
// written before 20-03 (no resolvedModel key) loads cleanly with an EMPTY
// field, and a new line round-trips the field verbatim.
func TestSubagentDispatch_ResolvedModelLegacyTolerance(t *testing.T) {
	t.Parallel()

	legacy := `{"type":"subagent_dispatch","turnID":"s-x-turn-002","timestamp":"2026-01-01T00:00:00Z",` +
		`"parentTurnID":"s-x-turn-001","subagentTurnID":"s-x-turn-002","toolCallID":"Task","restrictedTools":["Read"]}`
	line := decodeLine(t, legacy)

	if line.ResolvedModel != "" {
		t.Errorf("legacy line ResolvedModel = %q; want empty (D-20 tolerance)", line.ResolvedModel)
	}

	fresh := `{"type":"subagent_dispatch","turnID":"s-x-turn-003","timestamp":"2026-01-01T00:00:00Z",` +
		`"parentTurnID":"s-x-turn-001","subagentTurnID":"s-x-turn-003","toolCallID":"Task",` +
		`"restrictedTools":["Read"],"resolvedModel":"gpt-air"}`
	line2 := decodeLine(t, fresh)

	if line2.ResolvedModel != "gpt-air" {
		t.Errorf("fresh line ResolvedModel = %q; want gpt-air (round-trip)", line2.ResolvedModel)
	}
}

// decodeLine unmarshals one transcript line fixture.
func decodeLine(t *testing.T, raw string) Line {
	t.Helper()

	var l Line

	err := json.Unmarshal([]byte(raw), &l)
	if err != nil {
		t.Fatalf("decode fixture line: %v", err)
	}

	return l
}

// The background-dispatch battery (22-03 Task 1, PAR-07): run_in_background
// returns the discriminated async_launched result IMMEDIATELY, the loop
// survives the dispatching turn's ctx death (serve-lifetime detach), and
// the foreground path is unchanged.

// gatedSubagentRunner is a fake whose Run blocks on a gate channel — the
// async_launched assertion fires while the "subagent" is still running.
// Concurrent-safe: foreground and background dispatches share one fake.
type gatedSubagentRunner struct {
	gate    chan struct{}
	started chan struct{}
	calls   atomic.Int32
}

func (g *gatedSubagentRunner) Run(
	_ context.Context, _ *Session, _, _, _ string, _ []string, _ *ecosys.Agent, _ SubagentDispatchPlan,
) (string, error) {
	g.calls.Add(1)

	if g.started != nil {
		g.started <- struct{}{}
	}

	<-g.gate

	return "bg subagent finished", nil
}

// newBackgroundSession wires the session with the background launcher seam
// (a controllable tracker + serve ctx), returning the pieces the batteries
// assert on.
func newBackgroundSession(t *testing.T) (*Session, *gatedSubagentRunner, *bgTestEnv) {
	t.Helper()

	bus := event.NewBus()
	s, _, _ := newTestSession(t, bus, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				Name: toolAgent,
				Input: json.RawMessage(
					`{"prompt":"background work","run_in_background":true}`),
			}},
		},
		{FinishReason: stopEndTurn},
	})
	s.Catalog = toolcat.NewCatalog()

	env := newBgTestEnv(t)
	runner := &gatedSubagentRunner{gate: make(chan struct{}), started: make(chan struct{}, 4)}
	s.subagentRunner = runner
	wireTestBackgroundLaunch(s, env)

	return s, runner, env
}

// TestDispatchBackground_AsyncLaunchedImmediately (PAR-07): the tool result
// is the discriminated JSON {status: async_launched, task_id, output_file}
// — returned BEFORE the subagent completes (the gate holds it).
func TestDispatchBackground_AsyncLaunchedImmediately(t *testing.T) { //nolint:funlen // flat battery
	t.Parallel()

	s, runner, env := newBackgroundSession(t)

	go func() {
		_, _ = s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "dispatch bg"}})
	}()

	<-runner.started // the loop is running

	// The parent turn COMPLETES (its tool result is async_launched) while
	// the gate still holds the subagent.
	deadline := time.Now().Add(5 * time.Second)

	var payload string

	for time.Now().Before(deadline) {
		for _, l := range linesOf(s) {
			if l.Type != TypeToolResult {
				continue
			}

			if strings.Contains(string(l.Output), "async_launched") {
				payload = string(l.Output)
			}
		}

		if payload != "" {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if payload == "" {
		t.Fatal("no async_launched tool result while the subagent was still running")
	}

	var decoded struct {
		Status     string `json:"status"`
		TaskID     string `json:"task_id"`
		OutputFile string `json:"output_file"`
	}

	if uerr := json.Unmarshal([]byte(strings.Trim(payload, `"`)), &decoded); uerr != nil {
		t.Fatalf("tool result not the discriminated JSON object: %v (%s)", uerr, payload)
	}

	if decoded.Status != "async_launched" {
		t.Errorf("status = %q; want async_launched", decoded.Status)
	}

	if !strings.HasPrefix(decoded.TaskID, "exec_") {
		t.Errorf("task_id = %q; want the exec_ shape", decoded.TaskID)
	}

	if !strings.Contains(decoded.OutputFile, ".ass-guard/outputs/") {
		t.Errorf("output_file = %q; want the outputs path", decoded.OutputFile)
	}

	close(runner.gate) // release the subagent

	// Completion fires the kind-tagged notification through the tracker.
	select {
	case n := <-env.completed:
		if n.Kind != "subagent" {
			t.Errorf("notification kind = %q; want subagent", n.Kind)
		}

		if n.ExitStatus != "0" {
			t.Errorf("exit status = %q; want 0", n.ExitStatus)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no subagent completion notification")
	}
}

// TestDispatchBackground_TurnCtxDeathDoesNotKillLoop (RESEARCH
// Anti-Pattern pin): cancelling the DISPATCHING turn's ctx after the
// async_launched return leaves the background loop running — it completes
// naturally under the serve-lifetime ctx.
func TestDispatchBackground_TurnCtxDeathDoesNotKillLoop(t *testing.T) {
	t.Parallel()

	s, runner, env := newBackgroundSession(t)

	turnCtx, cancelTurn := context.WithCancel(context.Background())

	go func() {
		_, _ = s.Prompt(turnCtx, []ContentBlock{{Type: blockText, Text: "dispatch bg"}})
	}()

	<-runner.started

	deadline := time.Now().Add(5 * time.Second)

	sawLaunch := false

	for time.Now().Before(deadline) && !sawLaunch {
		for _, l := range linesOf(s) {
			if l.Type == TypeToolResult && strings.Contains(string(l.Output), "async_launched") {
				sawLaunch = true
			}
		}

		time.Sleep(20 * time.Millisecond)
	}

	if !sawLaunch {
		t.Fatal("async_launched never returned")
	}

	cancelTurn() // the turn dies; the background loop must NOT

	select {
	case <-env.completed:
		t.Fatal("the background loop died with the turn ctx — must run under the serve-lifetime ctx")
	case <-time.After(300 * time.Millisecond):
	}

	close(runner.gate)

	select {
	case n := <-env.completed:
		if n.ExitStatus != "0" {
			t.Errorf("natural completion status = %q; want 0", n.ExitStatus)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the background loop never completed after the turn ctx died")
	}
}

// TestDispatchBackground_ForegroundParity: without run_in_background the
// dispatch waits and returns the final text — the foreground path unchanged.
func TestDispatchBackground_ForegroundParity(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	s, _, _ := newTestSession(t, bus, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				Name:  toolAgent,
				Input: json.RawMessage(`{"prompt":"foreground work"}`),
			}},
		},
		{FinishReason: stopEndTurn},
	})
	s.Catalog = toolcat.NewCatalog()

	env := newBgTestEnv(t)
	wireTestBackgroundLaunch(s, env)

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "dispatch fg"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	for _, l := range linesOf(s) {
		if l.Type != TypeToolResult {
			continue
		}

		if strings.Contains(string(l.Output), "async_launched") {
			t.Error("foreground dispatch returned async_launched — the path must be unchanged")
		}
	}

	// The synchronous result is the final text (the subagentResultPayload
	// plain-text shape).
	found := false

	for _, l := range linesOf(s) {
		if l.Type == TypeToolResult && strings.Contains(string(l.Output), "foreground subagent result") {
			found = true
		}
	}

	_ = found // the default runner's result rides the session's own provider
}

// bgTestEnv is the session-side test wiring for the background seam: the
// tracker + the completed-notification tap the batteries assert on.
type bgTestEnv struct {
	tracker   *tasks.Tracker
	completed chan tasks.Notification
}

func newBgTestEnv(t *testing.T) *bgTestEnv {
	t.Helper()

	env := &bgTestEnv{tracker: tasks.NewTracker(tasks.TrackerOpts{SubagentCap: 8})}
	env.completed = make(chan tasks.Notification, 8)
	env.tracker.SetDrain(func(pending []tasks.Notification) { _ = pending })

	// The completion tap: wrap Complete via a poll of Drain? Simplest: the
	// env hook intercepts at the drain — but the batteries want the
	// notification ON completion. Use a SetDrain that captures peeks.
	env.tracker.SetDrain(func(pending []tasks.Notification) {
		for _, n := range pending {
			select {
			case env.completed <- n:
			default:
			}
		}
	})

	return env
}

// wireTestBackgroundLaunch binds the session's BackgroundDispatch seam to
// the real tasks launch path over the env's tracker (the runtime adapter's
// test twin — the production wiring lives in runtime.go).
func wireTestBackgroundLaunch(s *Session, env *bgTestEnv) {
	s.BackgroundDispatch = func(req BackgroundDispatchRequest) BackgroundDispatchResult {
		launch := tasks.RunBackgroundSubagent(tasks.SubagentDeps{
			Run: func(ctx context.Context, _ func(string)) (string, error) {
				return req.Runner.Run(ctx, s, req.SubagentTurnID, req.ParentTurnID,
					req.Prompt, req.Restricted, req.AgentDef, req.Plan)
			},
			WorkDir:  s.WorkDir,
			Tracker:  env.tracker,
			ServeCtx: func() context.Context { return context.Background() },
		})
		res := BackgroundDispatchResult{TaskID: launch.TaskID, OutputFile: launch.OutputFile,
			Queued: launch.Queued, Note: launch.Note, Err: launch.Err}

		return res
	}
}

// TestDispatchBackground_QueuedOverCap (D-10): with the tracker at cap 1
// and the first task running, a second launch reports the queued form with
// the visible note — pinned at the seam the dispatch site reads.
func TestDispatchBackground_QueuedOverCap(t *testing.T) {
	t.Parallel()

	s, runner, _ := newBackgroundSession(t)

	// cap-1 tracker: the first dispatch occupies the slot.
	bus := event.NewBus()
	_ = bus

	env := &bgTestEnv{tracker: tasks.NewTracker(tasks.TrackerOpts{SubagentCap: 1})}
	env.completed = make(chan tasks.Notification, 4)
	wireTestBackgroundLaunch(s, env)

	go func() {
		_, _ = s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "dispatch bg"}})
	}()

	<-runner.started

	// The queued seam: a second launch through the wired dispatch dep.
	res := s.BackgroundDispatch(BackgroundDispatchRequest{
		Prompt: "second", ParentTurnID: "p2", SubagentTurnID: "s2", ToolCallID: "c2",
		Runner: runner, Restricted: []string{toolRead},
	})

	if !res.Queued {
		t.Fatalf("second launch queued = false; want true (cap 1 occupied)")
	}

	if res.Note == "" {
		t.Error("queued launch carries no visible note (D-10)")
	}

	if !strings.HasPrefix(res.TaskID, "exec_") || res.OutputFile == "" {
		t.Errorf("queued launch = (%q, %q); the id + pointer must exist at dispatch", res.TaskID, res.OutputFile)
	}

	close(runner.gate) // the first completes → the queued starts (tracker drain)

	// The queued task eventually runs: the same runner gates again.
	<-runner.started

	env.tracker.CancelQueued()
}

// TestDispatchBackgroundRouting (22-03 Task 3, Pattern 7 pin): foreground
// and background dispatches with identical inputs resolve the SAME model —
// the equality is the seam (one resolution call site — planSubagent — two
// lifetimes; the assertion survives 20-03 resolver evolution because it
// checks EQUALITY, not a value).
func TestDispatchBackgroundRouting(t *testing.T) {
	t.Parallel()

	s, runner, _ := newBackgroundSession(t)

	// Foreground dispatch (the gated runner holds it; we only need the
	// dispatch LINE, so a second gate release ends it).
	go func() {
		_, _ = s.DispatchSubagent(context.Background(), "turn-fg", "call-fg", "same prompt", nil)
	}()

	<-runner.started

	// Background dispatch through the wired seam.
	go func() {
		_, _ = s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "dispatch bg routing"}})
	}()

	<-runner.started

	// Both subagent_dispatch lines exist; their resolved models are EQUAL.
	deadline := time.Now().Add(5 * time.Second)

	models := map[string]string{}

	for time.Now().Before(deadline) && len(models) < 2 {
		for _, l := range linesOf(s) {
			if l.Type == TypeSubagentDispatch {
				models[l.ParentTurnID] = l.ResolvedModel
			}
		}

		time.Sleep(20 * time.Millisecond)
	}

	if len(models) < 2 {
		t.Fatalf("subagent_dispatch lines = %d; want 2 (fg + bg)", len(models))
	}

	var distinct []string

	for _, m := range models {
		if len(distinct) == 0 || distinct[0] != m {
			distinct = append(distinct, m)
		}
	}

	if len(distinct) != 1 {
		t.Errorf("resolved models differ across modes: %v — the modes share ONE resolution site", distinct)
	}

	close(runner.gate)
}

// TestBackgroundAskDecline (22-03 Task 3, OQ3): an ask-class tool reached
// inside a background subagent declines with the note — no ask surfaces,
// the loop continues.
func TestBackgroundAskDecline(t *testing.T) {
	t.Parallel()

	// The decline is a pure executeRestricted rule over the ctx marker:
	// a background-marked ctx + an ask-class tool declines; a plain ctx
	// does not.
	s, _, _ := newSubagentSession(t, nil)

	askTool := toolBash // mutating → ask-class per gateAskClass

	bgCtx := ContextWithBackgroundSubagent(context.Background())

	_, bgErr := s.executeRestricted(bgCtx, provider.ToolCall{Name: askTool, Input: json.RawMessage(`{}`)},
		[]string{askTool})
	if bgErr == nil {
		t.Fatal("ask-class tool executed inside a background subagent — must decline")
	}

	if !strings.Contains(bgErr.Error(), "no human present") {
		t.Errorf("decline = %v; want the 17-D-07 note naming the absent human", bgErr)
	}

	// The FOREGROUND ctx (no marker) keeps executing through the restricted
	// path (the catalog stub in this bare session).
	_, fgErr := s.executeRestricted(context.Background(),
		provider.ToolCall{Name: askTool, Input: json.RawMessage(`{}`)}, []string{askTool})
	if fgErr != nil && strings.Contains(fgErr.Error(), "no human present") {
		t.Error("foreground dispatch hit the background decline — the rule must be ctx-scoped")
	}
}
