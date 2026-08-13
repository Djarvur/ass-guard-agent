package hookdag_test

import (
	"fmt"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/hookdag"
)

// fakeCommands is a recording CommandRunner. Each scripted call returns its
// canned (stdout, stderr, exit, err); sleeps force overlap in concurrency tests.
type fakeCommands struct {
	mu       sync.Mutex
	calls    []recordedCall
	sleep    time.Duration
	scripted map[int]commandResult // call index (0-based) → result
}

type recordedCall struct {
	command string
	args    []string
}

type commandResult struct {
	stdout string
	stderr string
	exit   int
	err    error
}

//nolint:gocritic // conflicts w/ nonamedreturns
func (f *fakeCommands) Run(ctx context.Context, command string, args []string) (string, string, int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, recordedCall{command: command, args: append([]string(nil), args...)})
	idx := len(f.calls) - 1
	f.mu.Unlock()

	if f.sleep > 0 {
		select {
		case <-time.After(f.sleep):
		case <-ctx.Done():
			return "", "", 0, fmt.Errorf("ctx: %w", ctx.Err())
		}
	}

	res, ok := f.scripted[idx]
	if !ok {
		// Default success for unscripted indices.
		return "ok-" + command, "", 0, nil
	}

	return res.stdout, res.stderr, res.exit, res.err
}

func (f *fakeCommands) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.calls)
}

// fakeTurns is a recording TurnRunner for send-prompt steps.
type fakeTurns struct {
	mu      sync.Mutex
	prompts []string
	stop    string
	err     error
}

func (f *fakeTurns) Run(_ context.Context, prompt []hookdag.ContentBlock) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	text := ""
	if len(prompt) > 0 {
		text = prompt[0].Text
	}

	f.prompts = append(f.prompts, text)
	if f.err != nil {
		return f.stop, f.err
	}

	if f.stop == "" {
		return "end_turn", nil
	}

	return f.stop, nil
}

func (f *fakeTurns) promptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.prompts)
}

// fakeBoundaries is a recording BoundaryOpener.
type fakeBoundaries struct {
	mu     sync.Mutex
	causes []string
	err    error
}

func (f *fakeBoundaries) OpenBoundary(_ context.Context, cause string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.causes = append(f.causes, cause)

	return f.err
}

func (f *fakeBoundaries) lastCause() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.causes) == 0 {
		return ""
	}

	return f.causes[len(f.causes)-1]
}

// captureHooks subscribes to HookProgress + drains the buffer synchronously.
func captureHooks(t *testing.T, bus *event.Bus) func() []event.HookProgress {
	t.Helper()

	ch := bus.Subscribe("HookProgress", event.BufHookProgress)

	return func() []event.HookProgress {
		var got []event.HookProgress

		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return got
				}

				if hp, ok := e.(event.HookProgress); ok {
					got = append(got, hp)
				}
			default:
				return got
			}
		}
	}
}

func prov(name string) hookdag.Provenance {
	return hookdag.Provenance{HookName: name, TriggerStage: stagePostImplement, SourceTurnID: "turn-1"}
}

// TestExecute_HappyPath verifies a 3-step all-passing hook completes + emits a
// start+finish event per step.
func TestExecute_HappyPath(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	e := &hookdag.Executor{
		Bus:        bus,
		Commands:   &fakeCommands{},
		Turns:      &fakeTurns{},
		Boundaries: &fakeBoundaries{},
	}
	hook := hookdag.Hook{
		Name: "h", Trigger: stagePostImplement, OnFailure: hookdag.OnFailureHalt,
		Steps: []hookdag.Step{
			{Name: "rc", Kind: hookdag.StepRunCommand, Command: "go", Args: []string{"test"}},
			{Name: "sp", Kind: hookdag.StepSendPrompt, Prompt: reviewPrompt},
			{Name: "w", Kind: hookdag.StepWait, Duration: "1ms"},
		},
	}
	collect := captureHooks(t, bus)

	res := e.Execute(context.Background(), &hook, prov("h"))
	if res.Status != hookdag.StatusCompleted {
		t.Errorf("Status = %q; want completed", res.Status)
	}

	if res.StepsRun != 3 {
		t.Errorf("StepsRun = %d; want 3", res.StepsRun)
	}
	// Each step emitted start + finish = 6 events.
	events := collect()
	if len(events) != 6 {
		t.Errorf("events = %d; want 6 (start+finish per step)", len(events))
	}
}

// TestExecute_RunCommandHalt verifies a run-command exit 1 + on_failure:halt
// stops the chain at the failing step (the 3rd step NOT executed).
func TestExecute_RunCommandHalt(t *testing.T) {
	t.Parallel()

	fc := &fakeCommands{scripted: map[int]commandResult{
		0: {exit: 0},
		1: {exit: 1, stderr: "boom"},
	}}
	e := &hookdag.Executor{Bus: event.NewBus(), Commands: fc}
	hook := hookdag.Hook{Name: "h", Trigger: stagePostImplement,
		Steps: []hookdag.Step{
			{Name: "s0", Kind: hookdag.StepRunCommand, Command: "a"},
			{Name: "s1", Kind: hookdag.StepRunCommand, Command: "b", OnFailure: hookdag.OnFailureHalt},
			{Name: "s2", Kind: hookdag.StepRunCommand, Command: "c"},
		}}

	res := e.Execute(context.Background(), &hook, prov("h"))
	if res.Status != hookdag.StatusHalted {
		t.Errorf("Status = %q; want halted", res.Status)
	}

	if res.FailedStep != 1 {
		t.Errorf("FailedStep = %d; want 1", res.FailedStep)
	}

	if res.StepsRun != 1 {
		t.Errorf("StepsRun = %d; want 1 (s2 not executed)", res.StepsRun)
	}

	if !errors.Is(errors.New(res.Detail), errors.New("boom")) && !contains(res.Detail, "boom") {
		t.Errorf("Detail = %q; want it to carry the stderr boom", res.Detail)
	}

	if fc.callCount() != 2 {
		t.Errorf("command calls = %d; want 2 (s2 skipped)", fc.callCount())
	}
}

// TestExecute_OnFailureContinue verifies on_failure:continue logs the failure +
// proceeds (the chain completes; the failure is in the events stream).
func TestExecute_OnFailureContinue(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	fc := &fakeCommands{scripted: map[int]commandResult{
		0: {exit: 0},
		1: {exit: 1, stderr: "lint warn"},
		2: {exit: 0},
	}}
	e := &hookdag.Executor{Bus: bus, Commands: fc}
	hook := hookdag.Hook{Name: "h", Trigger: stagePostImplement,
		Steps: []hookdag.Step{
			{Name: "s0", Kind: hookdag.StepRunCommand, Command: "a"},
			{Name: "s1", Kind: hookdag.StepRunCommand, Command: "b", OnFailure: hookdag.OnFailureContinue},
			{Name: "s2", Kind: hookdag.StepRunCommand, Command: "c"},
		}}
	collect := captureHooks(t, bus)

	res := e.Execute(context.Background(), &hook, prov("h"))
	if res.Status != hookdag.StatusCompleted {
		t.Errorf("Status = %q; want completed (continue)", res.Status)
	}

	if res.StepsRun != 3 {
		t.Errorf("StepsRun = %d; want 3 (chain proceeded)", res.StepsRun)
	}
	// A HookProgress{error} event was emitted for step s1.
	var sawError bool

	for _, hp := range collect() {
		if hp.Status == "error" && hp.StepIndex == 1 {
			sawError = true
		}
	}

	if !sawError {
		t.Error("no HookProgress{error} emitted for the continued s1 failure (HOOK-03 — never silent)")
	}
}

// TestExecute_OnFailureAsk verifies on_failure:ask suspends the chain (the next
// step is NOT run) + emits an ask event.
func TestExecute_OnFailureAsk(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	fc := &fakeCommands{scripted: map[int]commandResult{
		0: {exit: 1, stderr: "needs input"},
	}}
	e := &hookdag.Executor{Bus: bus, Commands: fc}
	hook := hookdag.Hook{Name: "h", Trigger: stagePostImplement,
		Steps: []hookdag.Step{
			{Name: "s0", Kind: hookdag.StepRunCommand, Command: "a", OnFailure: hookdag.OnFailureAsk},
			{Name: "s1", Kind: hookdag.StepRunCommand, Command: "b"},
		}}
	collect := captureHooks(t, bus)

	res := e.Execute(context.Background(), &hook, prov("h"))
	if res.Status != hookdag.StatusAsked {
		t.Errorf("Status = %q; want asked", res.Status)
	}

	if res.FailedStep != 0 {
		t.Errorf("FailedStep = %d; want 0", res.FailedStep)
	}

	var sawAsk bool

	for _, hp := range collect() {
		if hp.Status == "ask" {
			sawAsk = true
		}
	}

	if !sawAsk {
		t.Error("no HookProgress{ask} emitted (HOOK-03)")
	}
}

// TestExecute_StepInheritsHookDefault verifies a step with empty OnFailure
// inherits the hook's policy (halt) — exit 1 ⇒ halt.
func TestExecute_StepInheritsHookDefault(t *testing.T) {
	t.Parallel()

	fc := &fakeCommands{scripted: map[int]commandResult{0: {exit: 1, stderr: "x"}}}
	e := &hookdag.Executor{Commands: fc}
	hook := hookdag.Hook{Name: "h", Trigger: stagePostImplement, OnFailure: hookdag.OnFailureHalt,
		Steps: []hookdag.Step{{Name: "s0", Kind: hookdag.StepRunCommand, Command: "a"}}}

	res := e.Execute(context.Background(), &hook, prov("h"))
	if res.Status != hookdag.StatusHalted {
		t.Errorf("Status = %q; want halted (inherited hook default)", res.Status)
	}
}

// TestExecute_SendPromptIsATurn verifies the send-prompt step calls the
// TurnRunner exactly once with the step's Prompt as a text content block.
func TestExecute_SendPromptIsATurn(t *testing.T) {
	t.Parallel()

	ft := &fakeTurns{}
	e := &hookdag.Executor{Turns: ft}
	hook := hookdag.Hook{Name: "h", Trigger: stagePostImplement,
		Steps: []hookdag.Step{{Name: "sp", Kind: hookdag.StepSendPrompt, Prompt: reviewPrompt}}}

	res := e.Execute(context.Background(), &hook, prov("h"))
	if res.Status != hookdag.StatusCompleted {
		t.Errorf("Status = %q; want completed", res.Status)
	}

	if ft.promptCount() != 1 {
		t.Fatalf("turn calls = %d; want 1 (send-prompt IS a turn)", ft.promptCount())
	}

	ft.mu.Lock()
	defer ft.mu.Unlock()

	if ft.prompts[0] != reviewPrompt {
		t.Errorf("prompt = %q; want review me", ft.prompts[0])
	}
}

// TestExecute_SendPromptStopIsNotError verifies stop=end_turn is NOT treated as
// an error (HOOK-05 — the turn completed normally).
func TestExecute_SendPromptStopIsNotError(t *testing.T) {
	t.Parallel()

	ft := &fakeTurns{stop: "end_turn"}
	e := &hookdag.Executor{Turns: ft}
	hook := hookdag.Hook{Name: "h", Trigger: stagePostImplement,
		Steps: []hookdag.Step{{
			Name: "sp", Kind: hookdag.StepSendPrompt,
			Prompt: "x", OnFailure: hookdag.OnFailureHalt,
		}}}

	res := e.Execute(context.Background(), &hook, prov("h"))
	if res.Status != hookdag.StatusCompleted {
		t.Errorf("Status = %q; want completed (stop=end_turn is not an error)", res.Status)
	}
}

// TestExecute_FreshContextIsABoundary verifies the fresh-context step calls
// BoundaryOpener.OpenBoundary once with a cause containing the step/hook name.
func TestExecute_FreshContextIsABoundary(t *testing.T) {
	t.Parallel()

	fb := &fakeBoundaries{}
	e := &hookdag.Executor{Boundaries: fb}
	hook := hookdag.Hook{Name: "h", Trigger: stagePostImplement,
		Steps: []hookdag.Step{{Name: "fc", Kind: hookdag.StepFreshContext}}}

	res := e.Execute(context.Background(), &hook, prov("h"))
	if res.Status != hookdag.StatusCompleted {
		t.Errorf("Status = %q; want completed", res.Status)
	}

	if !contains(fb.lastCause(), "fc") {
		t.Errorf("OpenBoundary cause = %q; want it to contain the step name", fb.lastCause())
	}
}

// TestExecute_Wait verifies the wait step sleeps the parsed Duration (1ms) and
// does not call any other seam.
func TestExecute_Wait(t *testing.T) {
	t.Parallel()

	fc := &fakeCommands{}
	ft := &fakeTurns{}
	fb := &fakeBoundaries{}
	e := &hookdag.Executor{Commands: fc, Turns: ft, Boundaries: fb}
	hook := hookdag.Hook{Name: "h", Trigger: stagePostImplement,
		Steps: []hookdag.Step{{Name: "w", Kind: hookdag.StepWait, Duration: "1ms"}}}
	t0 := time.Now()

	res := e.Execute(context.Background(), &hook, prov("h"))
	if res.Status != hookdag.StatusCompleted {
		t.Errorf("Status = %q; want completed", res.Status)
	}

	if time.Since(t0) < time.Millisecond {
		t.Errorf("wait slept %v; want >= 1ms", time.Since(t0))
	}

	if fc.callCount() != 0 || ft.promptCount() != 0 || len(fb.causes) != 0 {
		t.Error("wait step called another seam; want isolated")
	}
}

// TestReentrant_Refused verifies two CONCURRENT Execute calls with the same
// (Hook.Name, Trigger) + allow_reentrant:false ⇒ exactly one skipped-reentrant.
func TestReentrant_Refused(t *testing.T) {
	t.Parallel()
	// A 50ms sleep on the first command call forces overlap between two
	// concurrent Execute calls.
	fc := &fakeCommands{
		sleep:    50 * time.Millisecond,
		scripted: map[int]commandResult{0: {exit: 0}},
	}
	e := &hookdag.Executor{Commands: fc}
	hook := hookdag.Hook{Name: "rh", Trigger: stagePostImplement,
		Steps: []hookdag.Step{{Name: "s", Kind: hookdag.StepRunCommand, Command: "x"}}}

	var wg sync.WaitGroup

	results := make(chan hookdag.Result, 2)

	for range 2 {
		wg.Go(func() {
			results <- e.Execute(context.Background(), &hook, prov("rh"))
		})
	}

	wg.Wait()
	close(results)

	var statuses []string
	for r := range results {
		statuses = append(statuses, r.Status)
	}

	skipped := 0

	for _, s := range statuses {
		if s == hookdag.StatusSkippedReentrant {
			skipped++
		}
	}

	if skipped != 1 {
		t.Errorf("got %d skipped-reentrant; want exactly 1 (HOOK-04): statuses=%v", skipped, statuses)
	}
}

// TestReentrant_AllowReentrant verifies allow_reentrant:true permits overlap —
// neither call is skipped.
func TestReentrant_AllowReentrant(t *testing.T) {
	t.Parallel()

	fc := &fakeCommands{sleep: 30 * time.Millisecond, scripted: map[int]commandResult{0: {exit: 0}}}
	e := &hookdag.Executor{Commands: fc}
	hook := hookdag.Hook{Name: "rh2", Trigger: stagePostImplement, AllowReentrant: true,
		Steps: []hookdag.Step{{Name: "s", Kind: hookdag.StepRunCommand, Command: "x"}}}

	var wg sync.WaitGroup

	results := make(chan hookdag.Result, 2)

	for range 2 {
		wg.Go(func() {
			results <- e.Execute(context.Background(), &hook, prov("rh2"))
		})
	}

	wg.Wait()
	close(results)

	for r := range results {
		if r.Status == hookdag.StatusSkippedReentrant {
			t.Error("got skipped-reentrant with allow_reentrant:true; want both completed")
		}
	}
}

// TestReentrant_InFlightClearedAfterCompletion verifies a sequential second
// Execute with the same provenance is NOT skipped (the in-flight entry was
// cleared by the defer).
func TestReentrant_InFlightClearedAfterCompletion(t *testing.T) {
	t.Parallel()

	fc := &fakeCommands{}
	e := &hookdag.Executor{Commands: fc}
	hook := hookdag.Hook{Name: "seq", Trigger: stagePostImplement,
		Steps: []hookdag.Step{{Name: "s", Kind: hookdag.StepRunCommand, Command: "x"}}}
	r1 := e.Execute(context.Background(), &hook, prov("seq"))

	r2 := e.Execute(context.Background(), &hook, prov("seq"))
	if r1.Status != hookdag.StatusCompleted || r2.Status != hookdag.StatusCompleted {
		t.Errorf("sequential statuses = %q,%q; want completed,completed (in-flight cleared)", r1.Status, r2.Status)
	}
}

// TestRunCommand_ExitCodeContract verifies the run-command exit-code contract
// directly (0 ⇒ no error; non-zero ⇒ error carrying stderr) — D-08.
func TestRunCommand_ExitCodeContract(t *testing.T) {
	t.Parallel()
	t.Run("zero no error", func(t *testing.T) {
		t.Parallel()

		fc := &fakeCommands{scripted: map[int]commandResult{0: {exit: 0, stdout: "out"}}}
		e := &hookdag.Executor{Commands: fc}

		hook := hookdag.Hook{Name: "h", Trigger: "t",
			Steps: []hookdag.Step{{
				Name: "s", Kind: hookdag.StepRunCommand,
				Command: "x", OnFailure: hookdag.OnFailureHalt,
			}}}
		if res := e.Execute(context.Background(), &hook, prov("h")); res.Status != hookdag.StatusCompleted {
			t.Errorf("exit-0 Status = %q; want completed", res.Status)
		}
	})
	t.Run("nonzero carries stderr", func(t *testing.T) {
		t.Parallel()

		fc := &fakeCommands{scripted: map[int]commandResult{0: {exit: 2, stderr: "the stderr"}}}
		e := &hookdag.Executor{Commands: fc}
		hook := hookdag.Hook{Name: "h", Trigger: "t",
			Steps: []hookdag.Step{{
				Name: "s", Kind: hookdag.StepRunCommand,
				Command: "x", OnFailure: hookdag.OnFailureHalt,
			}}}

		res := e.Execute(context.Background(), &hook, prov("h"))
		if res.Status != hookdag.StatusHalted {
			t.Errorf("exit-2 Status = %q; want halted", res.Status)
		}

		if !contains(res.Detail, "the stderr") {
			t.Errorf("Detail = %q; want it to carry stderr", res.Detail)
		}
	})
}

// guard against unused imports if the test set evolves.
var _ = json.RawMessage(nil)
