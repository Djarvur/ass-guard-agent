package session //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// Test-local consts (the thinking-path session battery, PAR-05 21-03).
const (
	thChunkType   = "thinking"
	thTurnFixture = "sess-th-turn-001"
	thModel       = "glm-5.2"
)

// scriptedThinkingProvider streams a fixed chunk sequence then closes — the
// thinking battery's transport (a sibling of the runtime's paced provider,
// local to the session package so session_test.go stays untouched).
type scriptedThinkingProvider struct {
	chunks []provider.StreamChunk
}

func (p *scriptedThinkingProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{FinishReason: stopEndTurn}, nil
}

func (p *scriptedThinkingProvider) Stream(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, len(p.chunks)+1)

	go func() {
		defer close(ch)

		for _, c := range p.chunks {
			ch <- c
		}
	}()

	return ch, nil
}

func (p *scriptedThinkingProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// newThinkingSession builds a bare Session over a counting-redactor Manager +
// bus + the scripted provider (the thinking battery's fixture).
func newThinkingSession(
	t *testing.T, chunks []provider.StreamChunk,
) (*Session, *countingRedactor, *event.Bus) {
	t.Helper()

	m, red := newCountingManager(t)
	bus := event.NewBus()

	s := &Session{
		Manager:   m,
		Bus:       bus,
		Provider:  &scriptedThinkingProvider{chunks: chunks},
		Profile:   profile.Profile{Name: fixtureProfileName, Model: thModel},
		SessionID: "sess-th",
	}

	return s, red, bus
}

// TestStreamAndEmit_ThinkingAppendsRawAndPublishesThought pins the PAR-05
// session case (21-03): a thinking chunk lands in the transcript VERBATIM via
// AppendRawThinking (the redactor-exempt path) with model attribution, AND the
// bus carries AgentThoughtChunk whose Content is the thinking FIELD VALUE
// extracted for display (D-12/D-13).
func TestStreamAndEmit_ThinkingAppendsRawAndPublishesThought(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"type":"thinking","thinking":"pondering the request","signature":"sig-abc"}`)

	s, _, bus := newThinkingSession(t, []provider.StreamChunk{
		{Type: thChunkType, Raw: raw},
		{Type: blockText, Text: "the answer"},
		{Type: stopDone, FinishReason: stopEndTurn},
	})

	sub := bus.Subscribe("AgentThoughtChunk", event.BufAgentThoughtChunk)

	resp, text, err := s.streamAndEmit(context.Background(), thTurnFixture, nil)
	if err != nil {
		t.Fatalf("streamAndEmit: %v", err)
	}

	if text != "the answer" {
		t.Errorf("streamed text = %q; want %q (text path unaffected)", text, "the answer")
	}

	if resp.FinishReason != stopEndTurn {
		t.Errorf("FinishReason = %q; want end_turn", resp.FinishReason)
	}

	// Transcript: the raw_thinking line carries the payload byte-verbatim.
	lines, err := s.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var got *Line

	for i := range lines {
		if lines[i].Type == TypeRawThinking {
			got = &lines[i]
		}
	}

	if got == nil {
		t.Fatal("no raw_thinking line on disk — AppendRawThinking never called")
	}

	if !bytes.Equal(raw, got.Content) {
		t.Errorf("transcript payload NOT verbatim:\n want: %s\n  got: %s", raw, got.Content)
	}

	if got.Model != thModel {
		t.Errorf("provider attribution Model = %q; want %q (same source the assistant message uses)",
			got.Model, thModel)
	}

	if got.TurnID != thTurnFixture {
		t.Errorf("TurnID = %q; want %q", got.TurnID, thTurnFixture)
	}

	// Bus: exactly one AgentThoughtChunk carrying the DISPLAY text.
	var events []event.AgentThoughtChunk

drain:
	for {
		select {
		case e, ok := <-sub:
			if !ok {
				break drain
			}

			if tc, isThought := e.(event.AgentThoughtChunk); isThought {
				events = append(events, tc)
			}
		default:
			break drain
		}
	}

	bus.Unsubscribe("AgentThoughtChunk", sub)

	if len(events) != 1 {
		t.Fatalf("AgentThoughtChunk events = %d; want exactly 1 (%v)", len(events), events)
	}

	want := event.AgentThoughtChunk{TurnID: thTurnFixture, MessageID: thTurnFixture, Content: "pondering the request"}
	if events[0] != want {
		t.Errorf("AgentThoughtChunk = %+v; want %+v (display text + turn ids)", events[0], want)
	}
}

// TestStreamAndEmit_ThinkingRedactedAppendsWithoutPublish pins the redacted
// shape: the block still lands in the transcript verbatim, but with NO thinking
// field value there is NO display text — no AgentThoughtChunk is published.
func TestStreamAndEmit_ThinkingRedactedAppendsWithoutPublish(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"type":"redacted_thinking","data":"enc-blob-42"}`)

	s, _, bus := newThinkingSession(t, []provider.StreamChunk{
		{Type: thChunkType, Raw: raw},
		{Type: stopDone, FinishReason: stopEndTurn},
	})

	sub := bus.Subscribe("AgentThoughtChunk", event.BufAgentThoughtChunk)

	_, _, err := s.streamAndEmit(context.Background(), thTurnFixture, nil)
	if err != nil {
		t.Fatalf("streamAndEmit: %v", err)
	}

	lines, err := s.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	found := false

	for i := range lines {
		if lines[i].Type == TypeRawThinking && bytes.Equal(raw, lines[i].Content) {
			found = true
		}
	}

	if !found {
		t.Fatal("redacted block never reached the transcript (AppendRawThinking is unconditional)")
	}

	select {
	case e := <-sub:
		t.Errorf("AgentThoughtChunk published for a redacted block: %+v (no display text exists)", e)
	default:
	}

	bus.Unsubscribe("AgentThoughtChunk", sub)
}

// TestStreamAndEmit_ThinkingRedactorZeroCalls is the T-16-04 pin under REAL
// traffic (21-03, D-12): a thinking-bearing stream records ZERO redactor
// invocations, while non-thinking content in the same session IS redacted —
// the exemption stays type-scoped.
func TestStreamAndEmit_ThinkingRedactorZeroCalls(t *testing.T) {
	t.Parallel()

	s, red, _ := newThinkingSession(t, []provider.StreamChunk{
		{Type: thChunkType, Raw: json.RawMessage(`{"type":"thinking","thinking":"secret-adjacent reasoning","signature":"s"}`)},
		{Type: stopDone, FinishReason: stopEndTurn},
	})

	_, _, err := s.streamAndEmit(context.Background(), thTurnFixture, nil)
	if err != nil {
		t.Fatalf("streamAndEmit: %v", err)
	}

	if got := red.calls(); got != 0 {
		t.Fatalf("redactor invoked %d time(s) across a thinking-bearing stream; want 0 (type-scoped exemption)",
			got)
	}

	// Control: a redacted kind in the SAME session observes the redactor —
	// the counting fake sits on the real path.
	if err := s.Manager.AppendAssistantMessage(thTurnFixture, "visible answer"); err != nil {
		t.Fatalf("AppendAssistantMessage: %v", err)
	}

	if got := red.calls(); got < 1 {
		t.Fatal("control redacted kind recorded zero redactor calls — counter not observing the path")
	}
}
