package session //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// marker returns a hook command that leaves a marker file under dir.
func marker(dir, name string) string {
	return "touch " + filepath.Join(dir, name)
}

// TestHookSeamsFireAtLifecycle (12-02 Task 4, Test 3) verifies the mapped
// lifecycle seams fire through the REAL runner with fixture-equivalent
// commands: SessionStart lazily on the first Prompt, UserPromptSubmit at turn
// entry with its stdout INJECTED as context, Stop at parent turn end,
// SubagentStop at subagent completion, SessionEnd in Close. All firings are
// synchronous — the markers prove ordering without sleeps.
func TestHookSeamsFireAtLifecycle(t *testing.T) {
	t.Parallel()

	work := t.TempDir()

	s, m, _ := newTestSession(t, nil, []provider.Response{{FinishReason: stopEndTurn}})
	s.WorkDir = work
	s.Hooks = ecosys.NewHookRunner([]ecosys.HookConfig{
		{Event: "SessionStart", Command: marker(work, "ss.marker")},
		{Event: "UserPromptSubmit", Command: "echo up-context-hook"},
		{Event: "Stop", Command: marker(work, "stop.marker")},
		{Event: "SessionEnd", Command: marker(work, "se.marker")},
	}, "s-hooks", work, filepath.Join(work, "audit.jsonl"))

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hello"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	for _, name := range []string{"ss.marker", "stop.marker"} {
		if _, serr := os.Stat(filepath.Join(work, name)); serr != nil {
			t.Errorf("lifecycle hook marker %s missing: %v", name, serr)
		}
	}

	if _, serr := os.Stat(filepath.Join(work, "se.marker")); serr == nil {
		t.Error("SessionEnd must NOT fire before Close")
	}

	// UserPromptSubmit stdout is captured and injected as turn context (the
	// documented context role) — the appended user message records it.
	lines, rerr := m.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	injected := false
	for _, l := range lines {
		if l.Type == TypeUserMessage && string(l.Content) != "" && json.Valid(l.Content) {
			if len(l.Content) > 0 && contains(string(l.Content), "up-context-hook") {
				injected = true
			}
		}
	}

	if !injected {
		t.Error("UserPromptSubmit stdout must be captured and injected as turn context")
	}

	// SessionEnd fires at Close.
	if cerr := s.Close(); cerr != nil {
		t.Fatalf("Close: %v", cerr)
	}

	if _, serr := os.Stat(filepath.Join(work, "se.marker")); serr != nil {
		t.Errorf("SessionEnd hook marker missing after Close: %v", serr)
	}
}

// contains is a tiny substring helper (avoids importing strings for one call).
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}

// TestHookSeamSubagentStop verifies SubagentStop fires when the subagent turn
// completes.
func TestHookSeamSubagentStop(t *testing.T) {
	t.Parallel()

	work := t.TempDir()

	s, _, _ := newTestSession(t, nil, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})
	s.Catalog = nil
	s.WorkDir = work
	s.Hooks = ecosys.NewHookRunner([]ecosys.HookConfig{
		{Event: "SubagentStop", Command: marker(work, "sub.marker")},
	}, "s-sub", work, "")

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if _, serr := os.Stat(filepath.Join(work, "sub.marker")); serr != nil {
		t.Errorf("SubagentStop hook marker missing: %v", serr)
	}
}

// TestHookFailureNeverFatal (AUD-03) verifies a FAILING lifecycle hook cannot
// kill the turn: the prompt still completes with its normal stop reason.
func TestHookFailureNeverFatal(t *testing.T) {
	t.Parallel()

	s, _, _ := newTestSession(t, nil, []provider.Response{{FinishReason: stopEndTurn}})
	s.Hooks = ecosys.NewHookRunner([]ecosys.HookConfig{
		{Event: "UserPromptSubmit", Command: "exit 1"},
		{Event: "Stop", Command: "exit 1"},
	}, "s-fail", t.TempDir(), "")

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
	if err != nil {
		t.Fatalf("a failing hook must never fail the turn: %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q, want %q", stop, stopEndTurn)
	}
}
