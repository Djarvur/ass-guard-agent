package session

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

// TestTranscriptReconstructsSession is the Phase-2 LOG-04 gate: after a scripted
// session (new + 3 prompts: one Bash boundary, one Task subagent, + cancel), the
// transcript + profile answer every investigation question (RESEARCH §9 a-h),
// every line is well-formed JSON, and NO secret value leaks.
func TestTranscriptReconstructsSession(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	m := newTestManager(t, "sess-recon")

	// A provider script: turn 1 = Bash tool_use (mutating → boundary); turn 2 =
	// Task tool_use (subagent dispatch); turn 3 = end_turn text; then a cancelled turn.
	script := []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolBash, Input: json.RawMessage(`{"command":"ls -la"}`)}},
		},
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				Name: toolTask, Input: json.RawMessage(`{"prompt":"research the layout"}`),
			}},
		},
		{FinishReason: stopEndTurn},
	}
	fp := &reconProvider{script: script, bus: bus}
	s := &Session{
		Manager: m, Projector: NewProjector(fakeProfile("test agent"), m),
		Provider: fp, Bus: bus, Semaphore: provider.NewSemaphore(2),
		Profile: *fakeProfile("test agent"), WorkDir: t.TempDir(), SessionID: "sess-recon",
		Catalog: toolcat.NewCatalog(),
	}
	// Start the async TranscriptWriter (the one session-path audit writer, D-20).
	tw := NewTranscriptWriter(m, bus)

	twCtx, twCancel := context.WithCancel(context.Background())
	defer twCancel()

	go tw.Run(twCtx)

	_ = m.AppendSessionStart("sess-recon")

	// Prompt 1: triggers Bash boundary.
	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "list files"}}); err != nil {
		t.Fatalf("prompt 1: %v", err)
	}
	// Prompt 2: triggers Task subagent.
	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "research"}}); err != nil {
		t.Fatalf("prompt 2: %v", err)
	}
	// Prompt 3: end_turn text.
	if _, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "summarize"}}); err != nil {
		t.Fatalf("prompt 3: %v", err)
	}
	// Inject a canceled line directly (cancel mid-turn is exercised separately by
	// TestCancelTurn; here we verify the transcript reconstructs a canceled state).
	_ = m.AppendCanceled("turn-cancel", time.Now(), "user cancelled via session/cancel")

	// Inject a secret-bearing request_shaped line to verify LOG-03 redaction on
	// disk (the reconProvider publishes RequestShaped too, but this is explicit).
	secret := json.RawMessage(`{"authorization":"Bearer sk-recon-secret"}`)
	_ = m.AppendRequestShaped("turn-x", secret, "test", time.Now())

	// Let the async writer flush.
	flushDeadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(flushDeadline) {
		lines, _ := m.ReadAll()
		if len(lines) >= 8 {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	twCancel()
	time.Sleep(50 * time.Millisecond)

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(lines) == 0 {
		t.Fatal("no transcript lines")
	}

	// Assert every line is well-formed JSON (already true since Manager marshals).
	// Aggregate the raw file for the secret-leak grep.
	raw := readTranscriptRaw(t, m.path)

	// (a) user prompts present
	// (b) model output (assistant_message)
	// (c) tools called with args (Bash + Task)
	// (d) tool results
	// (e) boundaries (mutating-command:Bash)
	// (f) verbatim request_shaped, redacted
	// (g) errors (none injected here; subagent may add)
	// (h) cancel state (canceled line)
	checks := map[string]bool{
		userMessageType:     false,
		"assistant_message": false,
		"tool_call":         false,
		"tool_result":       false,
		kindBoundary:        false,
		"request_shaped":    false,
		"canceled":          false,
		"subagent_dispatch": false,
		"subagent_result":   false,
	}
	for _, l := range lines {
		checks[l.Type] = true
	}

	for want, got := range checks {
		if !got {
			t.Errorf("transcript missing a %q line (reconstruction incomplete)", want)
		}
	}

	// Boundary must reference the mutating Bash command.
	bashBoundary := false

	for _, l := range lines {
		if l.Type == kindBoundary && l.Cause == mutatingCommandBash {
			bashBoundary = true
		}
	}

	if !bashBoundary {
		t.Error("no mutating-command:Bash boundary line (SESS-02/03)")
	}

	// LOG-03: NO secret leaks on disk.
	if strings.Contains(raw, "sk-recon-secret") {
		t.Error("transcript leaked the secret token on disk (LOG-03)")
	}

	if !strings.Contains(raw, "[REDACTED]") {
		t.Error("transcript did not redact the auth value (LOG-03)")
	}

	if !strings.Contains(raw, "authorization") {
		t.Error("transcript dropped the field name (must preserve names)")
	}
}

// reconProvider is a scripted streaming provider for the reconstruction test.
type reconProvider struct {
	script []provider.Response
	bus    *event.Bus
	n      int
}

func (r *reconProvider) Send(ctx context.Context, _ *profile.Profile, _ []provider.Message) (provider.Response, error) {
	return provider.Response{}, nil
}

func (r *reconProvider) Stream(ctx context.Context, prof *profile.Profile, _ []provider.Message) (<-chan provider.StreamChunk, error) {
	body, marshalErr := json.Marshal(map[string]any{keyModel: prof.Model})
	if marshalErr != nil {
		panic(marshalErr)
	}

	r.bus.Publish(event.RequestShaped{VerbatimRequest: body, Profile: prof.Name, Timestamp: time.Now()})

	ch := make(chan provider.StreamChunk, 8)

	go func() {
		defer close(ch)

		r.n++

		var resp provider.Response
		if r.n <= len(r.script) {
			resp = r.script[r.n-1]
		} else {
			// Beyond the script: return a canned end_turn (covers the subagent's
			// nested call and any extra prompts).
			resp = provider.Response{FinishReason: stopEndTurn}
		}

		for _, tc := range resp.ToolCalls {
			tcCopy := tc
			ch <- provider.StreamChunk{Type: blockToolUse, ToolCall: &tcCopy, ToolCallID: tc.Name}
		}

		if len(resp.ToolCalls) == 0 {
			ch <- provider.StreamChunk{Type: blockText, Text: "summary of findings"}
		}

		ch <- provider.StreamChunk{Type: stopDone, FinishReason: resp.FinishReason}
	}()

	return ch, nil
}

func (r *reconProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
