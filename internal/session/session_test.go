package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/djarvur/ass-guard-agent/internal/event"
	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/provider"
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
	cancelErr error // returned when ctx is cancelled (e.g. context.Canceled)
}

func (f *fakeProvider) Send(ctx context.Context, prof profile.Profile, msgs []provider.Message) (provider.Response, error) {
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
		body, _ := json.Marshal(map[string]any{"model": prof.Model, "n": n})
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
			return provider.Response{}, ctx.Err()
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
func (f *fakeProvider) Stream(ctx context.Context, prof profile.Profile, msgs []provider.Message) (<-chan provider.StreamChunk, error) {
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
		body, _ := json.Marshal(map[string]any{"model": prof.Model, "n": n})
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
			case ch <- provider.StreamChunk{Type: "tool_use", ToolCall: &tcCopy, ToolCallID: tc.Name}:
			case <-ctx.Done():
				return
			}
		}
		if len(resp.ToolCalls) == 0 {
			select {
			case ch <- provider.StreamChunk{Type: "text", Text: "assistant response"}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case ch <- provider.StreamChunk{Type: "done", FinishReason: resp.FinishReason}:
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
		Profile:   fakeProfile("test agent"),
		WorkDir:   t.TempDir(),
		SessionID: "sess-test",
	}
	return s, m, fp
}

// TestPromptRunsTurnLoop verifies Session.Prompt runs the D-18 cycle: appends
// user_message, publishes RequestShaped, calls Provider, appends assistant_message,
// returns the stopReason. Tool execution stays stubbed.
func TestPromptRunsTurnLoop(t *testing.T) {
	bus := event.NewBus()
	s, m, _ := newTestSession(t, bus, []provider.Response{
		{FinishReason: "end_turn"},
	})
	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: "text", Text: "hi"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if stop != "end_turn" {
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
	bus := event.NewBus()
	ch := bus.Subscribe("RequestShaped", event.BufRequestShaped)
	s, _, _ := newTestSession(t, bus, []provider.Response{{FinishReason: "end_turn"}})
	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: "text", Text: "hi"}}); err != nil {
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
func TestTranscriptWriterAsync(t *testing.T) {
	bus := event.NewBus()
	s, m, fp := newTestSession(t, bus, []provider.Response{{FinishReason: "end_turn"}})
	fp.delay = 30 * time.Millisecond // baseline Send latency
	tw := NewTranscriptWriter(m, bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tw.Run(ctx)

	start := time.Now()
	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: "text", Text: "hi"}}); err != nil {
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
	bus := event.NewBus()
	s, m, _ := newTestSession(t, bus, []provider.Response{{FinishReason: "end_turn"}})
	// Make Send block until ctx cancel.
	s.Provider.(*fakeProvider).delay = 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()
	stop, err := s.Prompt(ctx, []ContentBlock{{Type: "text", Text: "hi"}})
	// The turn returns "cancelled" (not a hard error) on ctx cancel.
	if err != nil {
		t.Logf("Prompt returned err=%v (acceptable if ctx cancelled)", err)
	}
	if stop != "cancelled" {
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
	bus := event.NewBus()
	s, m, _ := newTestSession(t, bus, []provider.Response{
		{FinishReason: "tool_use", ToolCalls: []provider.ToolCall{{Name: "Read", Input: json.RawMessage(`{"file_path":"x"}`)}}},
		{FinishReason: "end_turn"},
	})
	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: "text", Text: "read x"}}); err != nil {
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
	bus := event.NewBus()
	s, m, fp := newTestSession(t, bus, []provider.Response{{FinishReason: "end_turn"}})
	fp.panicOn = 1
	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: "text", Text: "hi"}})
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

// guard against unused fmt import if tests evolve.
var _ = fmt.Sprintf
