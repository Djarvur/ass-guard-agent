package shaper_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// --- 08-07 T3: the capture-grounded golden fixture + pairing invariant ---

// midturnFixtureMessage is one fixture entry (the captured zcode-normalized
// request.messages form — keys preserved verbatim per D-03).
type midturnFixtureMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []shaper.ToolCall `json:"toolCalls"`
	ToolCallID string           `json:"toolCallId"`
	ToolName   string           `json:"toolName"`
	IsError    *bool            `json:"isError"`
	ModelRef   string           `json:"modelRef"`
}

type midturnFixture struct {
	Messages []midturnFixtureMessage `json:"messages"`
}

// loadMidTurnFixture reads the committed capture-grounded fixture.
func loadMidTurnFixture(t *testing.T) midturnFixture {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", "zcode-midturn-messages.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var f midturnFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	return f
}

// fixtureMessages maps fixture entries to shaper.Messages (the structured
// mid-turn forms; the modelRef key is runtime bookkeeping, not message shape).
func fixtureMessages(f midturnFixture) []shaper.Message {
	out := make([]shaper.Message, 0, len(f.Messages))
	for _, m := range f.Messages {
		sm := shaper.Message{Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls}
		if m.Role == "tool" {
			sm.ToolCallID = m.ToolCallID
			sm.ToolName = m.ToolName
			if m.IsError != nil {
				sm.IsError = *m.IsError
			}
		}

		out = append(out, sm)
	}

	return out
}

// TestMidTurnCapture_GoldenShape pins the mid-turn message shapes against the
// CAPTURED zcode session (the committed fixture with provenance — the D-06
// fidelity discipline applied to the message-shape dimension): assistant
// {content, toolCalls[{id,name,input}]} renders ordered tool_use blocks; tool
// {content, toolCallId, toolName, isError} renders tool_result blocks with
// tool_use_id == toolCallId and is_error == isError; consecutive tool messages
// group; plain user/assistant text and mid-conversation system messages render
// as text blocks.
func TestMidTurnCapture_GoldenShape(t *testing.T) {
	t.Parallel()

	f := loadMidTurnFixture(t)
	if len(f.Messages) != 8 { //nolint:mnd // fixture shape: [0..7]
		t.Fatalf("fixture messages = %d, want 8 (fixture edited?)", len(f.Messages))
	}

	s := shaper.New()
	params, _, err := s.Shape(midTurnProfile(), fixtureMessages(f))
	if err != nil {
		t.Fatalf("Shape over the capture fixture: %v", err)
	}

	// Expected rendered sequence (consecutive tool messages GROUP):
	//   [0] user (prompt)
	//   [1] assistant: tool_use call_R1 (Bash)
	//   [2] user: tool_result call_R1
	//   [3] assistant: tool_use call_R2 (Read), call_R3 (Grep) — ordered
	//   [4] user: tool_result call_R2, call_R3 — grouped, ordered
	//   [5] assistant: plain text
	//   [6] user: mid-conversation system text (wire: user-role block)
	if len(params.Messages) != 7 { //nolint:mnd // see sequence above
		t.Fatalf("rendered params = %d, want 7", len(params.Messages))
	}

	assertParam := func(i int, role anthropic.MessageParamRole) anthropic.MessageParam {
		t.Helper()

		if params.Messages[i].Role != role {
			t.Errorf("params[%d].Role = %v, want %v", i, params.Messages[i].Role, role)
		}

		return params.Messages[i]
	}

	assertParam(0, anthropic.MessageParamRoleUser)

	b1 := assertParam(1, anthropic.MessageParamRoleAssistant)
	if len(b1.Content) != 1 || b1.Content[0].OfToolUse == nil || b1.Content[0].OfToolUse.ID != "call_R1" {
		t.Errorf("params[1] = %+v; want ONE tool_use block (call_R1, empty content → no text block)", b1.Content)
	}

	if b1.Content[0].OfToolUse != nil && b1.Content[0].OfToolUse.Name != "Bash" {
		t.Errorf("params[1] tool_use name = %q, want Bash (from the capture)", b1.Content[0].OfToolUse.Name)
	}

	assertParam(2, anthropic.MessageParamRoleUser)

	b2 := assertParam(3, anthropic.MessageParamRoleAssistant)
	if len(b2.Content) != 2 {
		t.Fatalf("params[3] blocks = %d, want 2 tool_use blocks", len(b2.Content))
	}

	for i, want := range []struct{ id, name string }{{"call_R2", "Read"}, {"call_R3", "Grep"}} {
		blk := b2.Content[i]
		if blk.OfToolUse == nil || blk.OfToolUse.ID != want.id || blk.OfToolUse.Name != want.name {
			t.Errorf("params[3][%d] = %+v; want tool_use {%s %s} in ORDER", i, blk, want.id, want.name)
		}
	}

	r2 := assertParam(4, anthropic.MessageParamRoleUser)
	if len(r2.Content) != 2 {
		t.Fatalf("params[4] blocks = %d, want 2 grouped tool_result blocks", len(r2.Content))
	}

	for i, wantID := range []string{"call_R2", "call_R3"} {
		blk := r2.Content[i]
		if blk.OfToolResult == nil || blk.OfToolResult.ToolUseID != wantID {
			t.Errorf("params[4][%d] = %+v; want tool_result {%s} (tool_use_id == toolCallId)", i, blk, wantID)
		}
	}

	if r2.Content[1].OfToolResult != nil && r2.Content[1].OfToolResult.IsError.Value {
		t.Error("params[4][1].is_error = true, want false (fixture isError:false)")
	}

	b3 := assertParam(5, anthropic.MessageParamRoleAssistant)
	if len(b3.Content) != 1 || b3.Content[0].OfText == nil {
		t.Errorf("params[5] = %+v; want a plain assistant text block", b3.Content)
	}

	sys := assertParam(6, anthropic.MessageParamRoleUser)
	if len(sys.Content) != 1 || sys.Content[0].OfText == nil {
		t.Errorf("params[6] = %+v; want the mid-conversation system text as a user-role text block", sys.Content)
	}
}

// TestPairingInvariant_CaptureFixture asserts the pairing invariant over the
// capture fixture: every tool message's toolCallId is an element of the
// immediately-preceding assistant batch's ids (T-8-28 — no mispaired results).
func TestPairingInvariant_CaptureFixture(t *testing.T) {
	t.Parallel()

	f := loadMidTurnFixture(t)

	var batchIDs []string

	for i, m := range f.Messages {
		switch m.Role {
		case roleToolMidturn:
			found := false
			for _, id := range batchIDs {
				if id == m.ToolCallID {
					found = true

					break
				}
			}

			if !found {
				t.Errorf("fixture[%d] tool message toolCallId %q not in the preceding batch %v",
					i, m.ToolCallID, batchIDs)
			}
		case "assistant":
			batchIDs = batchIDs[:0]
			for _, tc := range m.ToolCalls {
				batchIDs = append(batchIDs, tc.ID)
			}
		default:
			batchIDs = nil
		}
	}
}
