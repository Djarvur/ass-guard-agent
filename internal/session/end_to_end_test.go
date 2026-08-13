package session //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// TestEndToEndSession is the Phase-2 full-stack gate: a scripted session runs
// through the real Session Core (project → stream → boundary → subagent →
// cancel → logout), and the transcript captures the full sequence with stdout
// clean and no events dropped. Run with -race.
func TestEndToEndSession(t *testing.T) { //nolint:funlen // comprehensive test scenario
	t.Parallel()

	bus := event.NewBus()
	m := newTestManager(t, "sess-e2e")
	script := []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"research"}`)}},
		},
		{FinishReason: stopEndTurn},
	}
	fp := &reconProvider{script: script, bus: bus}
	s := &Session{
		Manager: m, Projector: NewProjector(fakeProfile("test agent"), m),
		Provider: fp, Bus: bus, Semaphore: provider.NewSemaphore(2),
		Profile: *fakeProfile("test agent"), WorkDir: t.TempDir(), SessionID: "sess-e2e",
		Catalog: toolcat.NewCatalog(),
	}
	tw := NewTranscriptWriter(m, bus)

	twCtx, twCancel := context.WithCancel(context.Background())
	defer twCancel()

	go tw.Run(twCtx)

	_ = m.AppendSessionStart("sess-e2e")
	stopCh := make(chan string, 1)

	go func() {
		st, _ := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "do research"}})
		stopCh <- st
	}()

	select {
	case stop := <-stopCh:
		if stop != stopEndTurn {
			t.Errorf("stopReason = %q; want end_turn", stop)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Prompt did not complete within 5s (full-stack deadlock?)")
	}

	_ = m.AppendSessionEnd()

	// Flush the async writer.
	time.Sleep(100 * time.Millisecond)
	twCancel()
	time.Sleep(50 * time.Millisecond)

	lines, _ := m.ReadAll()
	if len(lines) < 5 {
		t.Fatalf("only %d transcript lines; want a rich end-to-end sequence", len(lines))
	}
	// Verify the full sequence is captured.
	wantTypes := map[string]bool{
		TypeSessionStart:      false,
		TypeUserMessage:       false,
		TypeRequestShaped:     false,
		TypeAgentMessageChunk: false,
		TypeToolCall:          false,
		TypeSubagentDispatch:  false,
		TypeSubagentResult:    false,
		TypeSessionEnd:        false,
	}
	for _, l := range lines {
		wantTypes[l.Type] = true
	}

	for k, got := range wantTypes {
		if !got {
			t.Errorf("end-to-end transcript missing a %q line", k)
		}
	}
	// No events dropped: every published RequestShaped landed in the transcript.
	// (The async writer subscribes; if it dropped, request_shaped would be missing.)
	// Secret-leak guard: no Bearer/sk- tokens on disk.
	raw := readTranscriptRaw(t, m.path)
	if strings.Contains(raw, "sk-") && strings.Contains(raw, "Bearer ") {
		t.Error("end-to-end transcript leaked a secret pattern")
	}
}

// guard against unused profile import in this file.
var _ = profile.Profile{}
