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

// TestSteeringProjectAnchorSafety pins Pitfall 1: a mid-turn steering line
// folds as a user-role message in ARRIVAL position WITHOUT moving the
// Projector's anchor — the turn's earlier exchanges survive. A steering line
// of kind user_message would move the anchor past them and wipe the window
// (this test goes red under exactly that regression).
func TestSteeringProjectAnchorSafety(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-anchor")
	p := NewProjector(fakeProfile("test agent"), m)

	const turn = "s-anchor-turn-001"

	_ = m.AppendUserMessage(turn, []ContentBlock{{Type: blockText, Text: "original task"}})
	_ = m.AppendAssistantMessage(turn, "first finding")
	marker := renderSteeringMarker([]SteerItem{{Ticket: 1, Text: "pivot to plan b", At: time.Now().UTC()}})
	_ = m.AppendSteeringDelivery(turn, marker, 1)
	_ = m.AppendAssistantMessage(turn, "second finding")

	msgs, err := p.Project(turn)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// Window order: seed (turn-start user content) + first assistant +
	// steering marker (user-role) + second assistant.
	var first, steer, second int = -1, -1, -1

	for i := range msgs {
		switch {
		case msgs[i].Role == roleAssistant && msgs[i].Content == "first finding":
			first = i
		case msgs[i].Role == roleUserMsg && strings.Contains(msgs[i].Content, "pivot to plan b"):
			steer = i
		case msgs[i].Role == roleAssistant && msgs[i].Content == "second finding":
			second = i
		}
	}

	if first < 0 || steer < 0 || second < 0 {
		t.Fatalf("window incomplete: first=%d steer=%d second=%d; window=%+v", first, steer, second, msgs)
	}

	if !(first < steer && steer < second) {
		t.Errorf("arrival order broken: first=%d steer=%d second=%d", first, steer, second)
	}

	// The seed carries the turn-start user content, NOT the steering marker.
	if strings.Contains(msgs[0].Content, "pivot to plan b") {
		t.Error("steering text became the seed (anchor moved — Pitfall 1 regression)")
	}

	if !strings.Contains(msgs[0].Content, "original task") {
		t.Errorf("seed lost the turn-start user content: %q", msgs[0].Content)
	}
}

// TestSteeringProjectPairSafety pins the flushBatch rule: a steering line
// following an unclosed tool batch flushes the batch FIRST — the steering
// user message is its OWN message, never inside an assistant tool_use batch.
// (The transcript shape cannot occur live — the drain fires only at the
// iteration top, after all results are appended — the fold case defends the
// boundary regardless.)
func TestSteeringProjectPairSafety(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-pair")
	p := NewProjector(fakeProfile("test agent"), m)

	const turn = "s-pair-turn-001"

	_ = m.AppendUserMessage(turn, []ContentBlock{{Type: blockText, Text: "inspect"}})
	_ = m.AppendToolCall(turn, "c1", toolRead, json.RawMessage(`{"file_path":"x"}`))
	marker := renderSteeringMarker([]SteerItem{{Ticket: 1, Text: "check y instead", At: time.Now().UTC()}})
	_ = m.AppendSteeringDelivery(turn, marker, 1)
	_ = m.AppendToolResult(turn, "c1", json.RawMessage(`"contents"`), false)

	msgs, err := p.Project(turn)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var (
		batchIdx = -1
		steerIdx = -1
	)

	for i := range msgs {
		if msgs[i].Role == roleAssistant && len(msgs[i].ToolCalls) > 0 {
			batchIdx = i
		}

		if msgs[i].Role == roleUserMsg && strings.Contains(msgs[i].Content, "check y instead") {
			steerIdx = i
		}
	}

	if batchIdx < 0 || steerIdx < 0 {
		t.Fatalf("window lacks the flushed batch (%d) or the steering message (%d): %+v", batchIdx, steerIdx, msgs)
	}

	// The steering message is its own message OUTSIDE the batch (after the
	// flush, before the result).
	if msgs[batchIdx].Content != "" {
		t.Errorf("assistant batch carries text %q — steering merged into the batch", msgs[batchIdx].Content)
	}

	if steerIdx != batchIdx+1 {
		t.Errorf("steering message at %d; want immediately after the flushed batch (%d)", steerIdx, batchIdx+1)
	}
}

// TestSteeringProjectIntentSummaryUnchanged pins the intent/summary
// extraction safety: steering lines never become the turn's intent or the
// mechanical summary (they are invisible to findIntentLine/extractSummary by
// construction of the dedicated kind).
func TestSteeringProjectIntentSummaryUnchanged(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-intent")
	p := NewProjector(fakeProfile("test agent"), m)

	const turnA = "s-intent-turn-001"
	const turnB = "s-intent-turn-002"

	_ = m.AppendUserMessage(turnA, []ContentBlock{{Type: blockText, Text: "original intent"}})
	_ = m.AppendAssistantMessage(turnA, "work done")
	marker := renderSteeringMarker([]SteerItem{{Ticket: 1, Text: "secret steering", At: time.Now().UTC()}})
	_ = m.AppendSteeringDelivery(turnA, marker, 1)
	_ = m.AppendUserMessage(turnB, []ContentBlock{{Type: blockText, Text: "next task"}})

	msgs, err := p.Project(turnB)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// The seed: summary half from the PRIOR user/assistant exchange, intent
	// half = the CURRENT turn's user message. The steering text appears in
	// NEITHER half (it folds as a mid-turn message of turn A only, and turn
	// B's window carries just the lean seed).
	if strings.Contains(msgs[0].Content, "secret steering") {
		t.Errorf("steering leaked into turn B's seed: %q", msgs[0].Content)
	}

	if !strings.Contains(msgs[0].Content, "next task") {
		t.Errorf("seed lost the current intent: %q", msgs[0].Content)
	}

	if !strings.Contains(msgs[0].Content, "original intent") {
		t.Errorf("seed lost the prior-turn summary half: %q", msgs[0].Content)
	}
}

// TestSteeringReplayParity pins 18-D-01 for steering: projecting a transcript
// containing steering deliveries after a full Manager close/reopen produces a
// window byte-identical to the live projection.
func TestSteeringReplayParity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	m1, err := NewManager(dir, "s-replay", redactorAdapter{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	const turn = "s-replay-turn-001"

	_ = m1.AppendUserMessage(turn, []ContentBlock{{Type: blockText, Text: "do work"}})
	_ = m1.AppendToolCall(turn, "c1", toolRead, json.RawMessage(`{"file_path":"x"}`))
	_ = m1.AppendToolResult(turn, "c1", json.RawMessage(`"data"`), false)
	_ = m1.AppendSteeringDelivery(turn, renderSteeringMarker([]SteerItem{
		{Ticket: 1, Text: "first nudge", At: time.Now().UTC()},
		{Ticket: 2, Text: "second nudge", At: time.Now().UTC()},
	}), 2)
	_ = m1.AppendAssistantMessage(turn, "done with nudges applied")

	p1 := NewProjector(fakeProfile("test agent"), m1)

	live, err := p1.Project(turn)
	if err != nil {
		t.Fatalf("live Project: %v", err)
	}

	if err := m1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Full reopen on the SAME transcript file, fresh projector.
	m2, err := NewManager(dir, "s-replay", redactorAdapter{})
	if err != nil {
		t.Fatalf("reopen NewManager: %v", err)
	}

	t.Cleanup(func() { _ = m2.Close() })

	p2 := NewProjector(fakeProfile("test agent"), m2)

	replayed, err := p2.Project(turn)
	if err != nil {
		t.Fatalf("replay Project: %v", err)
	}

	liveJSON, _ := json.Marshal(live)
	replayJSON, _ := json.Marshal(replayed)

	if string(liveJSON) != string(replayJSON) {
		t.Errorf("replay divergence:\nlive:    %s\nreplay:  %s", liveJSON, replayJSON)
	}

	if !strings.Contains(string(replayJSON), "second nudge") {
		t.Error("replayed window lost a coalesced steering input")
	}
}

// TestSteeringProjectLegacyTolerance pins 16-D-20's reader-tolerance read of
// the fold: a pre-phase-23 transcript (NO steering lines) projects exactly as
// it did before the kind existed — the new fold case changes nothing for
// transcripts that never carry it.
func TestSteeringProjectLegacyTolerance(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-legacy")
	p := NewProjector(fakeProfile("test agent"), m)

	const turn = "s-legacy-turn-001"

	_ = m.AppendUserMessage(turn, []ContentBlock{{Type: blockText, Text: "legacy prompt"}})
	_ = m.AppendToolCall(turn, "c1", toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult(turn, "c1", json.RawMessage(`"files"`), false)
	_ = m.AppendAssistantMessage(turn, "legacy answer")

	msgs, err := p.Project(turn)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// The pre-23 shape: seed + one assistant batch + one tool result + one
	// assistant message — no marker, no extra user message.
	var userCount int

	for i := range msgs {
		if msgs[i].Role == roleUserMsg {
			userCount++
		}

		if strings.Contains(msgs[i].Content, steeringMarkerTag) {
			t.Errorf("legacy projection gained marker content: %+v", msgs[i])
		}
	}

	if userCount != 1 {
		t.Errorf("legacy window carries %d user messages; want exactly the seed (1)", userCount)
	}

	if len(msgs) != 4 { //nolint:mnd // seed + batch + result + assistant
		t.Errorf("legacy window has %d messages; want the pre-23 shape of 4: %+v", len(msgs), msgs)
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

// TestParkedAskRecord pins D-05's durable half (23-02): an ask surfaced by
// the turn loop parks with exactly one parked_ask transcript line (REDACTED
// path) carrying the turn id + summary. The LIVE half (the D-05 note) rides
// the ask queue's parked-enqueue path at the runtime wiring — the
// publishAskQueueNote test there (the fallback wire-order pin lives in
// TestPermissionsE2E stage 3).
func TestParkedAskRecord(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()

	m := newTestManager(t, "s-parked")
	s := &Session{
		Manager: m, Projector: NewProjector(fakeProfile("test agent"), m),
		Bus: bus, SessionID: "s-parked",
	}
	s.SetAskBroker(context.Background(), NewAskBroker(0, nil))

	qJSON, _ := json.Marshal([]AskQuestion{{
		Question: "which database?", Header: "db",
		Options: []AskOption{{Label: "postgres"}},
	}})

	s.suspendForAsk("s-parked-turn-001", "call-1", qJSON, "AskUserQuestion")

	// Exactly one parked_ask line with the summary.
	lines, _ := m.ReadAll()

	var parked []Line

	for _, l := range lines {
		if l.Type == TypeParkedAsk {
			parked = append(parked, l)
		}
	}

	if len(parked) != 1 {
		t.Fatalf("%d parked_ask lines; want 1", len(parked))
	}

	if parked[0].TurnID != "s-parked-turn-001" || parked[0].Text != "which database?" {
		t.Errorf("parked_ask line = turn %q text %q; want the turn id + question summary",
			parked[0].TurnID, parked[0].Text)
	}

	// No wire drift from the suspension itself: the note is NOT published
	// here (the queue's parked branch owns it — the fallback-order pin).
	for {
		select {
		case ev := <-bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk):
			if chunk, ok := ev.(event.AgentMessageChunk); ok &&
				strings.HasPrefix(chunk.Content, "ask waiting") {
				t.Errorf("suspendForAsk published the D-05 note itself: %q", chunk.Content)
			}
		default:
			return
		}
	}
}

// TestParkedAskReplayTolerance pins 16-D-20 for the new kind: parked_ask
// lines are audit markers the Projector never folds — a transcript carrying
// them projects identically to one without.
func TestParkedAskReplayTolerance(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-park-tol")
	p := NewProjector(fakeProfile("test agent"), m)

	const turn = "s-park-tol-turn-001"

	_ = m.AppendUserMessage(turn, []ContentBlock{{Type: blockText, Text: "the task"}})
	_ = m.AppendParkedAsk(turn, "which way?")
	_ = m.AppendAssistantMessage(turn, "the answer")

	msgs, err := p.Project(turn)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// The parked marker is invisible: seed + one assistant message only —
	// no extra user message, no marker content anywhere.
	for i := range msgs {
		if strings.Contains(msgs[i].Content, "which way?") || strings.Contains(msgs[i].Content, "ask waiting") {
			t.Errorf("parked-ask content leaked into the projected window: %+v", msgs[i])
		}
	}

	if len(msgs) != 2 { //nolint:mnd // seed + assistant
		t.Errorf("window has %d messages; want 2 (the parked marker folds to nothing): %+v", len(msgs), msgs)
	}
}
