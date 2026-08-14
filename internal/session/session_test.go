package session //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// fakeProvider is a controllable Provider for Session tests. It queues
// responses and simulates the RequestCapturer hook (publishes RequestShaped to
// the bus before returning), mirroring how the real AnthropicProvider fires its
// capturer after shaping and before sending.
type fakeProvider struct {
	mu        sync.Mutex
	responses []provider.Response
	bus       *event.Bus
	callN     int
	panicOn   int // 1-indexed; 0 = never
	delay     time.Duration
}

func (f *fakeProvider) Send(
	ctx context.Context, prof *profile.Profile, msgs []provider.Message,
) (provider.Response, error) {
	f.mu.Lock()
	f.callN++
	n := f.callN

	resp := provider.Response{}
	if len(f.responses) > 0 {
		resp = f.responses[0]
		f.responses = f.responses[1:]
	}

	bus := f.bus
	panicOn := f.panicOn
	delay := f.delay
	f.mu.Unlock()

	// Simulate the capturer firing after shaping, before sending (LOG-01).
	if bus != nil {
		body, marshalErr := json.Marshal(map[string]any{keyModel: prof.Model, "n": n})
		if marshalErr != nil {
			panic(marshalErr)
		}

		bus.Publish(event.RequestShaped{
			TurnID:          turnIDFromMessages(msgs),
			VerbatimRequest: body,
			Profile:         prof.Name,
			Timestamp:       time.Now(),
		})
	}

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return provider.Response{}, fmt.Errorf("ctx: %w", ctx.Err())
		}
	}

	if panicOn > 0 && n == panicOn {
		panic("fakeProvider: injected panic")
	}

	return resp, nil
}

// Stream implements the Phase-2 streaming path for the fake provider: it
// simulates the RequestCapturer (publishes RequestShaped), honors delay/cancel,
// and delivers the queued response as chunks (tool_use chunks for ToolCalls, a
// text chunk otherwise) + a terminal "done" chunk carrying FinishReason.
//
//nolint:cyclop,funlen // comprehensive test scenario
func (f *fakeProvider) Stream(
	ctx context.Context, prof *profile.Profile, msgs []provider.Message,
) (<-chan provider.StreamChunk, error) {
	f.mu.Lock()
	f.callN++
	n := f.callN

	resp := provider.Response{}
	if len(f.responses) > 0 {
		resp = f.responses[0]
		f.responses = f.responses[1:]
	}

	bus := f.bus
	panicOn := f.panicOn
	delay := f.delay
	f.mu.Unlock()

	if bus != nil {
		body, marshalErr := json.Marshal(map[string]any{keyModel: prof.Model, "n": n})
		if marshalErr != nil {
			panic(marshalErr)
		}

		bus.Publish(event.RequestShaped{
			TurnID: turnIDFromMessages(msgs), VerbatimRequest: body,
			Profile: prof.Name, Timestamp: time.Now(),
		})
	}
	// A panic in the SYNCHRONOUS part of Stream (here) is recovered by
	// Session.Prompt's deferred recover (the goroutine below is NOT covered, so
	// we panic before spawning it).
	if panicOn > 0 && n == panicOn {
		panic("fakeProvider: injected panic")
	}

	ch := make(chan provider.StreamChunk, 8)

	go func() {
		defer close(ch)

		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
		}

		for _, tc := range resp.ToolCalls {
			tcCopy := tc
			select {
			case ch <- provider.StreamChunk{Type: blockToolUse, ToolCall: &tcCopy, ToolCallID: tc.Name}:
			case <-ctx.Done():
				return
			}
		}

		if len(resp.ToolCalls) == 0 {
			select {
			case ch <- provider.StreamChunk{Type: blockText, Text: "assistant response"}:
			case <-ctx.Done():
				return
			}
		}

		select {
		case ch <- provider.StreamChunk{Type: stopDone, FinishReason: resp.FinishReason}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (f *fakeProvider) ToolResultMessage(toolCallID string, result json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"role":"user","content":"stub"}`), nil
}

// turnIDFromMessages is a no-op placeholder (the real capturer gets the turn id
// from the Session; the fake just embeds an empty string here).
func turnIDFromMessages(_ []provider.Message) string { return "" }

// newTestSession builds a Session with a fake provider over a temp-dir Manager.
func newTestSession(t *testing.T, bus *event.Bus, responses []provider.Response) (*Session, *Manager, *fakeProvider) {
	t.Helper()

	if bus == nil {
		bus = event.NewBus()
	}

	m := newTestManager(t, "sess-test")
	fp := &fakeProvider{responses: responses, bus: bus}
	pj := NewProjector(fakeProfile("test agent"), m)
	sem := provider.NewSemaphore(4)
	s := &Session{
		Manager:   m,
		Projector: pj,
		Provider:  fp,
		Bus:       bus,
		Semaphore: sem,
		Profile:   *fakeProfile("test agent"),
		WorkDir:   t.TempDir(),
		SessionID: "sess-test",
	}

	return s, m, fp
}

// TestPromptRunsTurnLoop verifies Session.Prompt runs the D-18 cycle: appends
// user_message, publishes RequestShaped, calls Provider, appends assistant_message,
// returns the stopReason. Tool execution stays stubbed.
func TestPromptRunsTurnLoop(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	s, m, _ := newTestSession(t, bus, []provider.Response{
		{FinishReason: stopEndTurn},
	})

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stopReason = %q; want end_turn", stop)
	}

	lines, _ := m.ReadAll()
	hasUser, hasAssistant := false, false

	for _, l := range lines {
		if l.Type == TypeUserMessage {
			hasUser = true
		}

		if l.Type == TypeAssistantMessage {
			hasAssistant = true
		}
	}

	if !hasUser {
		t.Error("transcript missing user_message line")
	}

	if !hasAssistant {
		t.Error("transcript missing assistant_message line")
	}
}

// TestRequestShapedPublished verifies the turn loop publishes RequestShaped to
// the bus (the capturer fires before the response returns).
func TestRequestShapedPublished(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	ch := bus.Subscribe("RequestShaped", event.BufRequestShaped)

	s, _, _ := newTestSession(t, bus, []provider.Response{{FinishReason: stopEndTurn}})

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	select {
	case <-ch:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RequestShaped not published")
	}
}

// TestTranscriptWriterAsync verifies the transcript writer subscribes to
// RequestShaped and appends request_shaped to the transcript WITHOUT blocking
// the turn loop (LOG-02 — a slow writer does not delay the provider call).
func TestTranscriptWriterAsync(t *testing.T) { //nolint:paralleltest // timing-sensitive: 1s async deadline
	bus := event.NewBus()
	s, m, fp := newTestSession(t, bus, []provider.Response{{FinishReason: stopEndTurn}})
	fp.delay = 30 * time.Millisecond // baseline Send latency
	tw := NewTranscriptWriter(m, bus)

	ctx := t.Context()

	go tw.Run(ctx)

	start := time.Now()

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	elapsed := time.Since(start)
	// The writer should not block the turn; elapsed is roughly the Send delay.
	if elapsed > 500*time.Millisecond {
		t.Errorf("Prompt took %v; the async writer may be blocking the turn", elapsed)
	}
	// Wait for the async writer to flush request_shaped.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		lines, _ := m.ReadAll()
		for _, l := range lines {
			if l.Type == TypeRequestShaped {
				return // good
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("transcript writer did not append request_shaped within 1s")
}

// TestCancelTurn verifies cancelling ctx aborts the turn, appends a canceled
// line, and returns stopReason "cancelled" (D-16).
func TestCancelTurn(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	s, m, _ := newTestSession(t, bus, []provider.Response{{FinishReason: stopEndTurn}})
	// Make Send block until ctx cancel.
	fp, _ := s.Provider.(*fakeProvider)
	fp.delay = 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	stop, err := s.Prompt(ctx, []ContentBlock{{Type: blockText, Text: "hi"}})
	// The turn returns "cancelled" (not a hard error) on ctx cancel.
	if err != nil {
		t.Logf("Prompt returned err=%v (acceptable if ctx cancelled)", err)
	}

	if stop != stopCancelled {
		t.Errorf("stopReason = %q; want cancelled", stop)
	}

	hasCanceled := false

	lines, _ := m.ReadAll()
	for _, l := range lines {
		if l.Type == TypeCanceled {
			hasCanceled = true
		}
	}

	if !hasCanceled {
		t.Error("transcript missing a canceled line after ctx cancel (D-16)")
	}
}

// TestStubToolExecution verifies that when the provider returns tool_calls, the
// turn loop appends a tool_call line + a STUB tool_result and loops to project
// again (real execution is Phase 4; D-15 stubs survive).
func TestStubToolExecution(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()

	s, m, _ := newTestSession(t, bus, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolRead, Input: json.RawMessage(`{"file_path":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "read x"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	lines, _ := m.ReadAll()
	haveToolCall, haveStubResult := false, false

	for _, l := range lines {
		if l.Type == TypeToolCall {
			haveToolCall = true
		}

		if l.Type == TypeToolResult {
			haveStubResult = true

			if !strings.Contains(strings.ToLower(string(l.Output)), "stub") {
				t.Errorf("tool_result output = %s; want a stub marker (Phase 2)", string(l.Output))
			}
		}
	}

	if !haveToolCall {
		t.Error("transcript missing tool_call line")
	}

	if !haveStubResult {
		t.Error("transcript missing stub tool_result line")
	}
}

// TestParentPanicRecovery verifies a panicking provider is recovered: the panic
// produces an error transcript line, and Prompt returns an error (no crash).
func TestParentPanicRecovery(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	s, m, fp := newTestSession(t, bus, []provider.Response{{FinishReason: stopEndTurn}})
	fp.panicOn = 1

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
	if err == nil {
		t.Fatal("Prompt returned nil error after a provider panic; want a recovered error")
	}

	hasError := false

	lines, _ := m.ReadAll()
	for _, l := range lines {
		if l.Type == TypeError {
			hasError = true

			if l.Component != "session" && l.Component != "turn" && l.Component != "provider" {
				t.Errorf("error line component = %q; want a known component", l.Component)
			}

			if l.Stack == "" {
				t.Error("error line missing a stack trace (investigate-and-fix-ready)")
			}
		}
	}

	if !hasError {
		t.Error("transcript missing an error line after parent panic (investigate-and-fix-ready)")
	}
}

// recordingToolExec is a toolcat.ToolExecutor that records call names + sleeps
// per call so the DispatchBatch loop's parallelism/serialization is observable.
type recordingToolExec struct {
	mu     sync.Mutex
	calls  []string
	starts []int64
	sleep  time.Duration
}

func (r *recordingToolExec) Execute(ctx context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
	r.mu.Lock()
	r.calls = append(r.calls, name)
	r.mu.Unlock()

	start := time.Since(testStartTool).Nanoseconds()

	select {
	case <-time.After(r.sleep):
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx: %w", ctx.Err())
	}

	r.mu.Lock()
	r.starts = append(r.starts, start)
	r.mu.Unlock()

	return json.RawMessage(`{"echo":"` + name + `"}`), nil
}

var testStartTool = time.Now() //nolint:gochecknoglobals // test fixture

// TestPromptDispatchBatchLoop verifies the Phase-4 tool loop: a turn with [Read,
// Bash] tool calls drives both through toolexec.DispatchBatch, appends BOTH
// tool_result lines in ARRIVAL ORDER, and fires a boundary ONLY for the mutating
// Bash tool. A real (recording) executor is injected via SetToolExecutor.
func TestPromptDispatchBatchLoop(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	s, m, _ := newTestSession(t, bus, []provider.Response{
		{ToolCalls: []provider.ToolCall{{Name: toolRead}, {Name: toolBash}}, FinishReason: blockToolUse},
		{FinishReason: stopEndTurn},
	})
	// Catalog marks Read read-only + Bash mutating (so the boundary fires for
	// Bash only — SESS-02).
	s.Catalog = toolcat.NewCatalog()
	s.Catalog.Register(toolcat.Tool{Name: toolRead, Mutability: toolcat.MutabilityReadOnly})
	s.Catalog.Register(toolcat.Tool{Name: toolBash, Mutability: toolcat.MutabilityMutating})

	rec := &recordingToolExec{sleep: 10 * time.Millisecond}
	s.SetToolExecutor(rec)

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "do it"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	// Both tools executed via the injected (recording) executor — DispatchBatch
	// routed them, NOT the stub path.
	rec.mu.Lock()
	gotCalls := append([]string(nil), rec.calls...)
	rec.mu.Unlock()

	if len(gotCalls) != 2 {
		t.Fatalf("executor calls = %v; want [Read Bash]", gotCalls)
	}
	// Arrival order preserved in the transcript: tool_result lines in [Read, Bash].
	lines, _ := m.ReadAll()

	var results []string

	for _, l := range lines {
		if l.Type == TypeToolResult {
			// AppendToolResult stores the tool-call id (the name) in ToolCallID.
			results = append(results, l.ToolCallID)
		}
	}

	if len(results) != 2 || results[0] != toolRead || results[1] != toolBash {
		t.Errorf("tool_result order = %v; want [Read Bash] (arrival order)", results)
	}
	// Boundary fires for Bash (mutating) only — SESS-02 invariant.
	var boundaries []string

	for _, l := range lines {
		if l.Type == TypeBoundary {
			boundaries = append(boundaries, l.Cause)
		}
	}

	if len(boundaries) != 1 || !strings.Contains(boundaries[0], toolBash) {
		t.Errorf("boundaries = %v; want exactly one Bash boundary", boundaries)
	}
}

// TestPromptNilToolExecutorStubs verifies backward compatibility: a Session
// WITHOUT SetToolExecutor returns the canned stub result for every non-subagent
// tool (Phase-2 D-15 behavior unchanged).
func TestPromptNilToolExecutorStubs(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	s, m, _ := newTestSession(t, bus, []provider.Response{
		{ToolCalls: []provider.ToolCall{{Name: toolRead}}, FinishReason: blockToolUse},
		{FinishReason: stopEndTurn},
	})
	s.Catalog = toolcat.NewCatalog()
	s.Catalog.Register(toolcat.Tool{Name: toolRead, Mutability: toolcat.MutabilityReadOnly})
	// SetToolExecutor NOT called — toolExec is nil.

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	lines, _ := m.ReadAll()
	for _, l := range lines {
		if l.Type == TypeToolResult {
			if !strings.Contains(string(l.Output), "stubbed in Phase 2") {
				t.Errorf("nil-exec tool_result = %s; want the Phase-2 stub", l.Output)
			}

			return
		}
	}

	t.Fatal("no tool_result line written (nil-exec stub path broken)")
}

// guard against unused fmt import if tests evolve.
var _ = fmt.Sprintf
