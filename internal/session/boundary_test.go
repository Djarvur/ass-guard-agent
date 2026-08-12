package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// newTestSessionWithCatalog returns a Session with a real toolcat.Catalog so
// boundary detection (toolcat.IsBoundary) works.
func newTestSessionWithCatalog(t *testing.T, responses []provider.Response) *Session {
	bus := event.NewBus()
	s, _, _ := newTestSession(t, bus, responses)
	s.Catalog = toolcat.NewCatalog()

	return s
}

// TestMaybeAppendBoundary_OnMutatingTool verifies that after a mutating tool
// (Bash) the turn loop appends a boundary line (SESS-02/03).
func TestMaybeAppendBoundary_OnMutatingTool(t *testing.T) {
	t.Parallel()

	s := newTestSessionWithCatalog(t, []provider.Response{
		{FinishReason: "tool_use", ToolCalls: []provider.ToolCall{{Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)}}},
		{FinishReason: "end_turn"},
	})
	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: "text", Text: "run ls"}}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	found := false

	for _, l := range linesOf(s) {
		if l.Type == TypeBoundary && l.Cause == "mutating-command:Bash" {
			found = true
		}
	}

	if !found {
		t.Error("no boundary line after a mutating Bash tool call (SESS-02/03)")
	}
}

// TestMaybeAppendBoundary_ReadOnlyNoBoundary verifies a read-only tool does NOT
// trigger a boundary.
func TestMaybeAppendBoundary_ReadOnlyNoBoundary(t *testing.T) {
	t.Parallel()

	s := newTestSessionWithCatalog(t, []provider.Response{
		{FinishReason: "tool_use", ToolCalls: []provider.ToolCall{{Name: "Read", Input: json.RawMessage(`{"file_path":"x"}`)}}},
		{FinishReason: "end_turn"},
	})
	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: "text", Text: "read"}}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	for _, l := range linesOf(s) {
		if l.Type == TypeBoundary {
			t.Errorf("unexpected boundary for a read-only Read tool: %+v", l)
		}
	}
}

// TestMaybeAppendBoundary_ConfigAddsBoundary verifies config-added boundaries
// trigger a boundary line for a read-only tool (SESS-02 adds-only).
func TestMaybeAppendBoundary_ConfigAddsBoundary(t *testing.T) {
	t.Parallel()
	s := newTestSessionWithCatalog(t, []provider.Response{
		{FinishReason: "tool_use", ToolCalls: []provider.ToolCall{{Name: "WebFetch", Input: json.RawMessage(`{}`)}}},
		{FinishReason: "end_turn"},
	})

	s.ConfigAdded = []string{"WebFetch"}

	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: "text", Text: "fetch"}}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	found := false

	for _, l := range linesOf(s) {
		if l.Type == TypeBoundary && l.Cause == "config-added:WebFetch" {
			found = true
		}
	}

	if !found {
		t.Error("no config-added boundary for WebFetch (SESS-02)")
	}
}

// TestStreamWired verifies the turn loop reads chunks from Provider.Stream and
// publishes AgentMessageChunk events to the bus (D-18 step 4). The fakeProvider
// delivers the response via Stream.
func TestStreamWired(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	chunks := bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	s, _, _ := newTestSession(t, bus, []provider.Response{
		{FinishReason: "end_turn"},
	})

	s.Catalog = toolcat.NewCatalog()

	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: "text", Text: "hi"}}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	// At least one AgentMessageChunk must have been published via Stream.
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
				t.Fatal("no AgentMessageChunk published; the turn loop may not be using Stream")
			}
		}

		if got.String() != "" {
			break
		}
	}
}

func linesOf(s *Session) []Line {
	l, _ := s.Manager.ReadAll()

	return l
}
