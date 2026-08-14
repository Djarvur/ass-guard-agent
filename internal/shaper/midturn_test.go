package shaper_test

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// midturn-test constants (goconst).
const (
	toolNameRead    = "Read"
	toolNameBash    = "Bash"
	callID1         = "call_1"
	callID2         = "call_2"
	callID9         = "call_9"
	roleToolMidturn = "tool"
	contentListOut  = "file_a\nfile_b"
	contentReadOut  = "module x"
)

// midTurnProfile is the minimal profile the mid-turn shaping tests use — the
// message-block rendering under test is profile-independent.
func midTurnProfile() *profile.Profile {
	return &profile.Profile{
		Name:      "midturn-test",
		Model:     synthModel,
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
		Name:  toolNameRead,
		Input: json.RawMessage(`{"file_path":"go.mod"}`),
	}

	if tc.ID != "call_abc123" || tc.Name != toolNameRead {
		t.Errorf("ToolCall fields = {%q,%q}; want {call_abc123,Read}", tc.ID, tc.Name)
	}

	var in map[string]any

	err := json.Unmarshal(tc.Input, &in)
	if err != nil {
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
		{Role: roleUser, Content: "list the files"},
		{
			Role:    "assistant",
			Content: "I will list them.",
			ToolCalls: []shaper.ToolCall{
				{ID: callID1, Name: toolNameBash, Input: json.RawMessage(`{"command":"ls"}`)},
				{ID: callID2, Name: toolNameRead, Input: json.RawMessage(`{"file_path":"go.mod"}`)},
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

	for i, wantID := range []string{callID1, callID2} {
		blk := am.Content[i+1]
		if blk.OfToolUse == nil {
			t.Fatalf("block[%d] is not a tool_use block", i+1)
		}

		if blk.OfToolUse.ID != wantID {
			t.Errorf("tool_use[%d].ID = %q, want %q", i, blk.OfToolUse.ID, wantID)
		}
	}

	if am.Content[1].OfToolUse.Name != toolNameBash {
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
// tool_use_id + content + is_error built via anthropic.NewToolResultBlock.
func TestMidTurn_AnthropicToolResultGrouping(t *testing.T) {
	t.Parallel()

	s := shaper.New()
	msgs := []shaper.Message{
		{Role: roleUser, Content: "go"},
		{
			Role: "assistant", ToolCalls: []shaper.ToolCall{
				{ID: callID1, Name: toolNameBash, Input: json.RawMessage(`{"command":"ls"}`)},
				{ID: callID2, Name: toolNameRead, Input: json.RawMessage(`{"file_path":"go.mod"}`)},
			},
		},
		{Role: roleToolMidturn, ToolCallID: callID1, ToolName: toolNameBash, Content: contentListOut, IsError: false},
		{Role: roleToolMidturn, ToolCallID: callID2, ToolName: toolNameRead, Content: contentReadOut, IsError: true},
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

	assertToolResultBlock(t, &tr.Content[0], callID1, false, contentListOut)
	assertToolResultBlock(t, &tr.Content[1], callID2, true, contentReadOut)
}

// TestMidTurn_AnthropicToolResultLone verifies a LONE tool-role message (no
// adjacent tool messages) renders as its own user message carrying exactly one
// tool_result block (the grouping never merges across non-tool messages).
func TestMidTurn_AnthropicToolResultLone(t *testing.T) {
	t.Parallel()

	s := shaper.New()
	lone := []shaper.Message{
		{Role: roleUser, Content: "x"},
		{Role: roleToolMidturn, ToolCallID: callID9, ToolName: toolNameBash, Content: "out", IsError: false},
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

	assertToolResultBlock(t, &last.Content[0], callID9, false, "out")
}

// assertToolResultBlock checks one rendered content block is a tool_result
// with the expected tool_use_id, is_error, and text content.
func assertToolResultBlock(
	t *testing.T, blk *anthropic.ContentBlockParamUnion, wantID string, wantErr bool, wantText string,
) {
	t.Helper()

	if blk.OfToolResult == nil {
		t.Fatalf("block is not a tool_result block: %+v", blk)
	}

	if blk.OfToolResult.ToolUseID != wantID {
		t.Errorf("tool_result.ToolUseID = %q, want %q", blk.OfToolResult.ToolUseID, wantID)
	}

	if blk.OfToolResult.IsError.Value != wantErr {
		t.Errorf("tool_result.IsError = %v, want %v", blk.OfToolResult.IsError.Value, wantErr)
	}

	res := blk.OfToolResult.Content[0]
	if res.OfText == nil {
		t.Fatalf("tool_result.Content[0] is not a text block: %+v", res)
	}

	if res.OfText.Text != wantText {
		t.Errorf("tool_result.Content = %q, want %q", res.OfText.Text, wantText)
	}
}
