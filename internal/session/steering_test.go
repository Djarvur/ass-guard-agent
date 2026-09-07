package session //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// steerProvider is the scripted steering test provider: every Stream call is
// captured (the projected window — what the model would receive), the FIRST
// call blocks until released (or the ctx dies) so the test can enqueue
// steering while the turn is mid-flight, and each call's entry is signalled
// so the test synchronizes on "request 1 observed" without sleeps.
type steerProvider struct {
	mu       sync.Mutex
	requests [][]provider.Message
	script   []provider.Response
	n        int
	entered  chan struct{} // one send per Stream call
	release  chan struct{} // closed by the test to unblock the first call
}

func (p *steerProvider) Send(context.Context, *profile.Profile, []provider.Message) (provider.Response, error) {
	return provider.Response{}, nil
}

func (p *steerProvider) Stream(
	ctx context.Context, _ *profile.Profile, messages []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.mu.Lock()
	p.n++
	call := p.n
	reqCopy := append([]provider.Message(nil), messages...)
	p.requests = append(p.requests, reqCopy)
	p.mu.Unlock()

	p.entered <- struct{}{}

	if call == 1 {
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	var resp provider.Response
	if call <= len(p.script) {
		resp = p.script[call-1]
	} else {
		resp = provider.Response{FinishReason: stopEndTurn}
	}

	ch := make(chan provider.StreamChunk, 8)

	go func() {
		defer close(ch)

		for _, tc := range resp.ToolCalls {
			tcCopy := tc
			ch <- provider.StreamChunk{Type: blockToolUse, ToolCall: &tcCopy, ToolCallID: tc.ID}
		}

		if len(resp.ToolCalls) == 0 {
			ch <- provider.StreamChunk{Type: blockText, Text: "done"}
		}

		ch <- provider.StreamChunk{Type: stopDone, FinishReason: resp.FinishReason}
	}()

	return ch, nil
}

func (p *steerProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (p *steerProvider) SupportsImages() bool { return false }

func (p *steerProvider) captured() [][]provider.Message {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([][]provider.Message(nil), p.requests...)
}

// newSteerSession builds a bare session on the steerProvider pattern from
// end_to_end_test.go (real Manager + Projector + turn loop; stub executor).
func newSteerSession(t *testing.T, bus *event.Bus, fp *steerProvider, sessionID string) *Session {
	t.Helper()

	m := newTestManager(t, sessionID)

	return &Session{
		Manager: m, Projector: NewProjector(fakeProfile("test agent"), m),
		Provider: fp, Bus: bus, Semaphore: provider.NewSemaphore(2),
		Profile: *fakeProfile("test agent"), WorkDir: t.TempDir(), SessionID: sessionID,
	}
}

// findSteeredMessage returns the first user-role message carrying both the
// steering marker tag and every wanted text, or nil.
func findSteeredMessage(msgs []provider.Message, wants ...string) *provider.Message {
	for i := range msgs {
		m := &msgs[i]
		if m.Role != roleUserMsg || !strings.Contains(m.Content, steeringMarkerTag) {
			continue
		}

		ok := true

		for _, w := range wants {
			if !strings.Contains(m.Content, w) {
				ok = false

				break
			}
		}

		if ok {
			return m
		}
	}

	return nil
}

// steeringMarkerTag is the marker wrapper's stable opening tag (the captured
// system-reminder wire convention — corpus_scan.go).
const steeringMarkerTag = "<system-reminder>"

// TestSteeringDeliveryEndToEnd pins the SEEDG-01 happy path: a steering input
// enqueued while iteration 1's request is IN FLIGHT reaches the model as a
// user-role message wrapped in the steering marker at the NEXT request
// boundary — with iteration 1's tool_call/tool_result pair intact in the same
// request (never split), exactly one steering_delivery transcript line, the
// live note "steering applied: 1 inputs", and no re-delivery on a later turn
// (the watermark advanced).
func TestSteeringDeliveryEndToEnd(t *testing.T) { //nolint:funlen // comprehensive end-to-end scenario
	t.Parallel()

	bus := event.NewBus()
	notes := bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)

	fp := &steerProvider{
		script: []provider.Response{
			{
				FinishReason: blockToolUse,
				ToolCalls: []provider.ToolCall{{
					ID: "call-1", Name: "Read", Input: json.RawMessage(`{"file_path":"a.txt"}`),
				}},
			},
			{FinishReason: stopEndTurn},
		},
		entered: make(chan struct{}, 4),
		release: make(chan struct{}),
	}
	s := newSteerSession(t, bus, fp, "sess-steer")
	q := NewSteerQueue()
	s.SetSteerQueue(q)

	stopCh := make(chan string, 1)

	go func() {
		st, _ := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "do the analysis"}})
		stopCh <- st
	}()

	<-fp.entered // request 1 observed (in flight)

	tk := q.Enqueue("focus on the cache layer")
	if tk != 1 {
		t.Fatalf("first ticket = %d; want 1", tk)
	}

	close(fp.release) // let iteration 1 complete

	select {
	case stop := <-stopCh:
		if stop != stopEndTurn {
			t.Errorf("stopReason = %q; want end_turn", stop)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Prompt did not complete within 5s (steering deadlock?)")
	}

	reqs := fp.captured()
	if len(reqs) < 2 {
		t.Fatalf("captured %d requests; want >= 2", len(reqs))
	}

	// Request 2 carries the steering as a marker-wrapped user-role message.
	if m := findSteeredMessage(reqs[1], "focus on the cache layer"); m == nil {
		t.Errorf("request 2 lacks the steering marker user message; got %+v", reqs[1])
	}

	// Pair safety: iteration 1's tool_call AND its tool_result are both in
	// request 2 (the drain never splits a pair).
	var hasCall, hasResult bool

	for i := range reqs[1] {
		m := &reqs[1][i]
		if m.Role == roleAssistant && len(m.ToolCalls) > 0 && m.ToolCalls[0].ID == "call-1" {
			hasCall = true
		}

		if m.Role == roleToolMsg && m.ToolCallID == "call-1" {
			hasResult = true
		}
	}

	if !hasCall || !hasResult {
		t.Errorf("request 2 pair integrity broken: tool_call=%v tool_result=%v", hasCall, hasResult)
	}

	// Exactly one steering_delivery line, carrying the marker + count 1.
	lines, _ := s.Manager.ReadAll()

	var deliveries []Line

	for _, l := range lines {
		if l.Type == TypeSteeringDelivery {
			deliveries = append(deliveries, l)
		}
	}

	if len(deliveries) != 1 {
		t.Fatalf("transcript holds %d steering_delivery lines; want 1", len(deliveries))
	}

	if !strings.Contains(deliveries[0].Text, steeringMarkerTag) ||
		!strings.Contains(deliveries[0].Text, "focus on the cache layer") {
		t.Error("steering_delivery line lacks the marker or the steered text")
	}

	if got := string(deliveries[0].Input); got != `{"count":1}` {
		t.Errorf("steering_delivery Input = %s; want {\"count\":1}", got)
	}

	// The live note fired through the AgentMessageChunk bus path.
	var noteSeen bool

	for {
		select {
		case ev := <-notes:
			if chunk, ok := ev.(event.AgentMessageChunk); ok &&
				chunk.Content == "steering applied: 1 inputs" {
				noteSeen = true
			}
		default:
			goto drained //nolint:gocritic // label is the drain exit
		}
	}

drained:
	if !noteSeen {
		t.Error("live note 'steering applied: 1 inputs' was not emitted on the bus")
	}

	// Watermark: a subsequent turn does NOT re-receive the delivered text.
	st2, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "next question"}})
	if err != nil || st2 != stopEndTurn {
		t.Fatalf("second Prompt = (%q,%v); want end_turn", st2, err)
	}

	reqs = fp.captured()

	if m := findSteeredMessage(reqs[len(reqs)-1], "focus on the cache layer"); m != nil {
		t.Error("a later turn re-received already-delivered steering (watermark not advanced)")
	}

	if q.Pending() != 0 {
		t.Errorf("queue Pending() = %d after delivery; want 0", q.Pending())
	}
}

// TestSteeringAntiZombie pins the cancelled-exit resolution (Pitfall 3):
// steering enqueued mid-turn, then the turn CANCELLED before the next
// boundary — the input resolves cancelled-normal at turn death (recordCanceled
// funnels CancelAll) and NEVER appears in any later turn's request window.
func TestSteeringAntiZombie(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()

	fp := &steerProvider{
		script: []provider.Response{{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				ID: "call-1", Name: "Read", Input: json.RawMessage(`{"file_path":"a.txt"}`),
			}},
		}},
		entered: make(chan struct{}, 4),
		release: make(chan struct{}),
	}
	s := newSteerSession(t, bus, fp, "sess-steer-cancel")
	q := NewSteerQueue()
	s.SetSteerQueue(q)

	ctx, cancel := context.WithCancel(context.Background())
	stopCh := make(chan string, 1)

	go func() {
		st, _ := s.Prompt(ctx, []ContentBlock{{Type: blockText, Text: "long work"}})
		stopCh <- st
	}()

	<-fp.entered // request 1 in flight
	q.Enqueue("never deliver this")
	cancel() // turn dies BEFORE the next boundary

	select {
	case stop := <-stopCh:
		if stop != stopCancelled {
			t.Errorf("stopReason = %q; want cancelled", stop)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled Prompt did not return within 5s")
	}

	// The cancelled input resolved — the queue is empty and no
	// steering_delivery line exists (a cancelled input never reaches a
	// transcript the Projector folds).
	if n := q.Pending(); n != 0 {
		t.Errorf("queue Pending() = %d after cancelled turn; want 0", n)
	}

	lines, _ := s.Manager.ReadAll()

	for _, l := range lines {
		if l.Type == TypeSteeringDelivery {
			t.Fatalf("cancelled steering became a steering_delivery line: %+v", l)
		}
	}

	// The next turn's request is clean: no marker, no steered text.
	st, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "fresh turn"}})
	if err != nil || st != stopEndTurn {
		t.Fatalf("post-cancel Prompt = (%q,%v); want end_turn", st, err)
	}

	reqs := fp.captured()

	for i, req := range reqs {
		if m := findSteeredMessage(req, "never deliver this"); m != nil {
			t.Errorf("request %d carries zombie steering from the cancelled turn", i+1)
		}

		for j := range req {
			if strings.Contains(req[j].Content, "never deliver this") {
				t.Errorf("request %d message %d carries cancelled steering text", i+1, j)
			}
		}
	}
}
