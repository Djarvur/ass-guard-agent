package shaper_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// midTurnProfile is the minimal profile the mid-turn shaping tests use — the
// message-block rendering under test is profile-independent.
func midTurnProfile() *profile.Profile {
	return &profile.Profile{
		Name:      "midturn-test",
		Model:     "synth-model",
		MaxTokens: 1024,
		System:    []profile.TextBlock{{Type: "text", Text: "sys"}},
	}
}

// TestMidTurn_ToolCallStruct pins the shaper-owned tool-call type: the captured
// zcode-normalized shape is {id, name, input} (VERIFIED-FACTS.md item #1:
// response.toolCalls[] — provider.ToolCall is an alias of this type, so the ID
// the provider parsed carries end-to-end).
func TestMidTurn_ToolCallStruct(t *testing.T) {
	t.Parallel()

	tc := shaper.ToolCall{
		ID:    "call_abc123",
		Name:  "Read",
		Input: json.RawMessage(`{"file_path":"go.mod"}`),
	}

	if tc.ID != "call_abc123" || tc.Name != "Read" {
		t.Errorf("ToolCall fields = {%q,%q}; want {call_abc123,Read}", tc.ID, tc.Name)
	}

	var in map[string]any
	if err := json.Unmarshal(tc.Input, &in); err != nil {
		t.Fatalf("Input not valid JSON: %v", err)
	}

	if in["file_path"] != "go.mod" {
		t.Errorf("Input.file_path = %v, want go.mod", in["file_path"])
	}
}

// TestMidTurn_AnthropicToolUseRendering verifies an assistant message carrying
// a ToolCalls batch shapes to an assistant MessageParam whose content blocks
// are the text block FIRST, then one tool_use block per call (id, name, input)
// built via anthropic.NewToolUseBlock — the captured mid-turn assistant form.
func TestMidTurn_AnthropicToolUseRendering(t *testing.T) {
	t.Parallel()

	s := shaper.New()
	msgs := []shaper.Message{
		{Role: "user", Content: "list the files"},
		{
			Role:    "assistant",
			Content: "I will list them.",
			ToolCalls: []shaper.ToolCall{
				{ID: "call_1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
				{ID: "call_2", Name: "Read", Input: json.RawMessage(`{"file_path":"go.mod"}`)},
			},
		},
	}

	params, _, err := s.Shape(midTurnProfile(), msgs)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	if len(params.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(params.Messages))
	}

	am := params.Messages[1]
	if am.Role != anthropic.MessageParamRoleAssistant {
		t.Fatalf("mid-turn message role = %v, want assistant", am.Role)
	}

	if len(am.Content) != 3 { // text first, then 2 tool_use blocks
		t.Fatalf("content blocks = %d, want 3 (text + 2 tool_use)", len(am.Content))
	}

	// Block order: text first, then tool_use blocks.
	if am.Content[0].OfText == nil {
		t.Errorf("block[0] is not a text block (want text BEFORE tool_use)")
	}

	for i, wantID := range []string{"call_1", "call_2"} {
		blk := am.Content[i+1]
		if blk.OfToolUse == nil {
			t.Fatalf("block[%d] is not a tool_use block", i+1)
		}

		if blk.OfToolUse.ID != wantID {
			t.Errorf("tool_use[%d].ID = %q, want %q", i, blk.OfToolUse.ID, wantID)
		}
	}

	if am.Content[1].OfToolUse.Name != "Bash" {
		t.Errorf("tool_use[0].Name = %q, want Bash", am.Content[1].OfToolUse.Name)
	}

	cmd, ok := am.Content[1].OfToolUse.Input.(map[string]any)
	if !ok {
		t.Fatalf("tool_use[0].Input = %T, want map[string]any (parsed JSON)", am.Content[1].OfToolUse.Input)
	}

	if cmd["command"] != "ls" {
		t.Errorf("tool_use[0].Input.command = %v, want ls", cmd["command"])
	}
}

// TestMidTurn_AnthropicToolResultGrouping verifies the tool-role rendering:
// consecutive tool-role messages group into ONE user-role MessageParam carrying
// one tool_result block per message (canonical Anthropic batch form; the
// capture's per-result granularity is preserved by block order), each with
// tool_use_id + content + is_error built via anthropic.NewToolResultBlock. A
// lone tool-role message still renders as its own user message.
func TestMidTurn_AnthropicToolResultGrouping(t *testing.T) {
	t.Parallel()

	s := shaper.New()
	msgs := []shaper.Message{
		{Role: "user", Content: "go"},
		{
			Role: "assistant", ToolCalls: []shaper.ToolCall{
				{ID: "call_1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
				{ID: "call_2", Name: "Read", Input: json.RawMessage(`{"file_path":"go.mod"}`)},
			},
		},
		{Role: "tool", ToolCallID: "call_1", ToolName: "Bash", Content: "file_a\nfile_b", IsError: false},
		{Role: "tool", ToolCallID: "call_2", ToolName: "Read", Content: "module x", IsError: true},
	}

	params, _, err := s.Shape(midTurnProfile(), msgs)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	// user, assistant, ONE grouped tool-result user message.
	if len(params.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3 (grouped tool results)", len(params.Messages))
	}

	tr := params.Messages[2]
	if tr.Role != anthropic.MessageParamRoleUser {
		t.Fatalf("grouped tool-result role = %v, want user", tr.Role)
	}

	if len(tr.Content) != 2 {
		t.Fatalf("grouped content blocks = %d, want 2 tool_result blocks", len(tr.Content))
	}

	wantIDs := []string{"call_1", "call_2"}
	wantErr := []bool{false, true}
	for i := range 2 {
		blk := tr.Content[i]
		if blk.OfToolResult == nil {
			t.Fatalf("block[%d] is not a tool_result block", i)
		}

		if blk.OfToolResult.ToolUseID != wantIDs[i] {
			t.Errorf("tool_result[%d].ToolUseID = %q, want %q", i, blk.OfToolResult.ToolUseID, wantIDs[i])
		}

		if bool(blk.OfToolResult.IsError) != wantErr[i] {
			t.Errorf("tool_result[%d].IsError = %v, want %v", i, blk.OfToolResult.IsError, wantErr[i])
		}

		if !strings.Contains(blk.OfToolResult.Content[0].Text, "file") && i == 0 {
			t.Errorf("tool_result[0].Content = %v, want the result text", blk.OfToolResult.Content)
		}
	}

	// A LONE tool-role message (no adjacent tool messages) renders as its own
	// user message carrying exactly one tool_result block.
	lone := []shaper.Message{
		{Role: "user", Content: "x"},
		{Role: "tool", ToolCallID: "call_9", ToolName: "Bash", Content: "out", IsError: false},
	}

	lparams, _, err := s.Shape(midTurnProfile(), lone)
	if err != nil {
		t.Fatalf("Shape(lone): %v", err)
	}

	if len(lparams.Messages) != 2 {
		t.Fatalf("len(lone Messages) = %d, want 2", len(lparams.Messages))
	}

	last := lparams.Messages[len(lparams.Messages)-1]
	if last.Role != anthropic.MessageParamRoleUser || len(last.Content) != 1 || last.Content[0].OfToolResult == nil {
		t.Fatalf("lone tool message did not render as its own user tool_result param: %+v", last)
	}

	if last.Content[0].OfToolResult.ToolUseID != "call_9" {
		t.Errorf("lone tool_result ToolUseID = %q, want call_9", last.Content[0].OfToolResult.ToolUseID)
	}
}
