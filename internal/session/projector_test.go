package session //nolint:testpackage // internal package test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// fakeProfile builds a minimal profile with one system block for projector tests.
func fakeProfile(systemText string) *profile.Profile {
	return &profile.Profile{
		Name:   "test",
		System: []profile.TextBlock{{Type: blockText, Text: systemText}},
	}
}

// TestProjector_LeanSeedAfterBoundary verifies that after a boundary, the lean
// window carries ZERO prior assistant/tool MESSAGES (D-01): it is a small
// user-only window (the summary + current intent), not the full conversation.
// A truncated excerpt of the last assistant may appear in the summary text
// (D-02), but no prior assistant/turn is carried as a separate message.
func TestProjector_LeanSeedAfterBoundary(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("you are a test agent"), m)

	_ = m.AppendUserMessage("turn_040", []ContentBlock{{Type: blockText, Text: "do something"}})
	_ = m.AppendAssistantMessage("turn_040", "old assistant response that must NOT carry forward")
	_ = m.AppendToolCall("turn_040", "tc1", toolBash, []byte(`{"command":"ls"}`))
	_ = m.AppendToolResult("turn_040", "tc1", []byte(`{"out":"files"}`), false)
	_ = m.AppendBoundary(mutatingCommandBash, "tc1", "turn_040")
	_ = m.AppendUserMessage("turn_041", []ContentBlock{{Type: blockText, Text: "what now"}})

	msgs, err := p.Project("turn_041")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	// D-01: no prior assistant turn carried as a separate message.
	for i, mm := range msgs {
		if strings.EqualFold(mm.Role, "assistant") {
			t.Errorf("lean window[%d] is an assistant-role message (D-01 carry-forward violation): %+v", i, mm)
		}
	}
	// The lean window is small (1-2 user messages), not the full prior conversation.
	if len(msgs) > 3 {
		t.Errorf("lean window has %d messages; want a small lean seed (D-01)", len(msgs))
	}

	combined := ""

	var combinedSb49 strings.Builder
	for _, mm := range msgs {
		combinedSb49.WriteString(mm.Content + "\n")
	}

	combined += combinedSb49.String()

	if !strings.Contains(combined, "what now") {
		t.Errorf("lean window missing the current user message:\n%s", combined)
	}
}

// TestProjector_SummaryExtraction verifies the task summary is mechanically
// extracted (D-02): last user message text (truncated) + files touched + last
// assistant message (truncated).
func TestProjector_SummaryExtraction(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)
	_ = m.AppendUserMessage("turn_040", []ContentBlock{{Type: blockText, Text: "edit the file"}})
	_ = m.AppendToolCall("turn_040", "tc1", toolRead, []byte(`{"file_path":"/a/go.mod"}`))
	_ = m.AppendToolResult("turn_040", "tc1", []byte(`{"out":"module x"}`), false)
	_ = m.AppendAssistantMessage("turn_040", "done editing")
	_ = m.AppendBoundary("mutating-command:Edit", "tc1", "turn_040")
	_ = m.AppendUserMessage("turn_041", []ContentBlock{{Type: blockText, Text: "next step"}})

	msgs, err := p.Project("turn_041")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	combined := ""

	var combinedSb75 strings.Builder
	for _, mm := range msgs {
		combinedSb75.WriteString(mm.Content + "\n")
	}

	combined += combinedSb75.String()

	if !strings.Contains(combined, "edit the file") {
		t.Errorf("summary missing the last user message text (D-02):\n%s", combined)
	}

	if !strings.Contains(combined, "/a/go.mod") {
		t.Errorf("summary missing the files-touched extraction:\n%s", combined)
	}

	if !strings.Contains(combined, "done editing") {
		t.Errorf("summary missing the last assistant message:\n%s", combined)
	}
}

// TestProjector_FirstTurn verifies on a fresh session (no boundary) the window
// is the system context + first user message (no summary).
func TestProjector_FirstTurn(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)
	_ = m.AppendUserMessage("turn_001", []ContentBlock{{Type: blockText, Text: "hello"}})

	msgs, err := p.Project("turn_001")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("no messages in lean window")
	}

	combined := ""

	var combinedSb103 strings.Builder
	for _, mm := range msgs {
		combinedSb103.WriteString(mm.Content + "\n")
	}

	combined += combinedSb103.String()

	if !strings.Contains(combined, "hello") {
		t.Errorf("first-turn window missing the user message:\n%s", combined)
	}
}

// TestProjector_Truncation verifies long user messages are truncated to the
// N-char limit (D-02, RESEARCH §4.3).
func TestProjector_Truncation(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)
	long := strings.Repeat("a", 1000)
	_ = m.AppendUserMessage("turn_040", []ContentBlock{{Type: blockText, Text: long}})
	_ = m.AppendBoundary(mutatingCommandBash, "tc1", "turn_040")
	_ = m.AppendUserMessage("turn_041", []ContentBlock{{Type: blockText, Text: "go"}})

	msgs, err := p.Project("turn_041")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	combined := ""

	var combinedSb125 strings.Builder
	for _, mm := range msgs {
		combinedSb125.WriteString(mm.Content)
	}

	combined += combinedSb125.String()
	// The 1000-char message must be truncated to <= MaxSummaryUserChars + ellipsis.
	if strings.Contains(combined, strings.Repeat("a", MaxSummaryUserChars+10)) {
		t.Errorf("summary did not truncate the long user message (limit=%d):\n%d chars",
			MaxSummaryUserChars, len(combined))
	}
}

// TestProjector_NoModelCall is a structural guard: the projector must be pure
// mechanical extraction (D-02). We assert the Projector struct has no Provider
// field (the lint `grep ".Send\|.Stream" projector.go` runs in the plan verify
// step; here we assert the type carries no provider dependency).
func TestProjector_NoModelCall(t *testing.T) {
	t.Parallel()
	p := NewProjector(fakeProfile("sys"), newTestManager(t, "s1"))
	// The Projector must not expose a provider-typed field — it builds the lean
	// window by reading the transcript, not by calling the model.
	type hasProvider interface{ Provider() }

	if _, ok := any(p).(hasProvider); ok {
		t.Error("Projector exposes a Provider accessor — D-02 violation (must be pure mechanical extraction)")
	}
}

// keyOpsxExplore is the provenance fixture key (08-04).
const keyOpsxExplore = "opsx:explore"

// TestProvenance_LineAndReplay (08-04 Tests 15-16, D-02/CMD-05) verifies the
// command_provenance line: written next to the expanded user message it
// records the command key + source file + typed args, and the projector
// replays the EXPANDED body as the user message while provenance NEVER leaks
// into model-visible message content (metadata, not a message).
func TestProvenance_LineAndReplay(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, "s-prov")
	p := NewProjector(fakeProfile("you are a test agent"), m)

	const (
		srcPath = "/work/.claude/commands/opsx/explore.md"
		srcArgs = "fix login flow"
	)

	err := m.AppendCommandProvenance("", keyOpsxExplore, srcPath, srcArgs)
	if err != nil {
		t.Fatalf("AppendCommandProvenance: %v", err)
	}

	expanded := "Explore the change: fix login flow\n\n- read the codebase\n- compare options\n"

	err = m.AppendUserMessage("turn_prov", []ContentBlock{{Type: blockText, Text: expanded}})
	if err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}

	assertProvenanceLine(t, m, srcPath, srcArgs)

	// Test 16: the projector replays the expanded body as the user message;
	// the provenance record never appears as message content.
	msgs, err := p.Project("turn_prov")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var combinedSb strings.Builder
	for _, mm := range msgs {
		combinedSb.WriteString(mm.Content + "\n")
	}

	combined := combinedSb.String()

	if !strings.Contains(combined, "Explore the change: fix login flow") {
		t.Errorf("lean window missing the expanded body:\n%s", combined)
	}

	if strings.Contains(combined, srcPath) || strings.Contains(combined, "command_provenance") {
		t.Errorf("provenance leaked into model-visible message content:\n%s", combined)
	}
}

// assertProvenanceLine verifies the command_provenance line records the key,
// source path, and typed args, and sits immediately before the expanded user
// message (append-order adjacency — D-02).
func assertProvenanceLine(t *testing.T, m *Manager, srcPath, srcArgs string) {
	t.Helper()

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	for i := range lines {
		l := &lines[i]
		if l.Type != TypeCommandProvenance {
			continue
		}

		if l.Name != keyOpsxExplore {
			t.Errorf("provenance key = %q; want %q", l.Name, keyOpsxExplore)
		}

		if l.CommandRef != srcPath {
			t.Errorf("provenance source = %q; want %q", l.CommandRef, srcPath)
		}

		if l.Text != srcArgs {
			t.Errorf("provenance args = %q; want %q", l.Text, srcArgs)
		}

		if i+1 >= len(lines) || lines[i+1].Type != TypeUserMessage {
			t.Errorf("provenance line at %d is not followed by the user message", i)
		}

		return
	}

	t.Fatal("no command_provenance line in the transcript")
}

// --- 08-07: within-turn accumulation (mid-turn conversation assembly) ---

// msgSummary renders a provider.Message compactly for failure messages.
func msgSummary(m *provider.Message) string {
	if len(m.ToolCalls) > 0 {
		ids := make([]string, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			ids = append(ids, tc.ID)
		}

		return fmt.Sprintf("assistant{toolCalls:%v text:%q}", ids, m.Content)
	}

	if m.Role == roleToolMsg {
		return fmt.Sprintf("tool{id:%s name:%s err:%v content:%q}", m.ToolCallID, m.ToolName, m.IsError, m.Content)
	}

	return fmt.Sprintf("%s{%q}", m.Role, m.Content)
}

// TestProjector_MidTurnAccumulation (08-07 T2 Test 2): the current turn's
// tool_call/tool_result lines AFTER the lean seed are carried as messages —
// the model's NEXT request contains its own prior exchange (the 08-06 blocker
// was the projector rebuilding an identical lean window every iteration).
func TestProjector_MidTurnAccumulation(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "run ls"}})
	_ = m.AppendToolCall("turnT", "call_1", toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult("turnT", "call_1", json.RawMessage(`{"out":"files"}`), false)

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3 (seed + assistant + tool):\n%s",
			len(msgs), msgSummaryList(msgs))
	}

	seed := msgs[0]
	if seed.Role != roleUserMsg || !strings.Contains(seed.Content, "run ls") {
		t.Errorf("seed = %s; want user lean seed carrying the intent", msgSummary(&seed))
	}

	am := msgs[1]
	if am.Role != "assistant" || len(am.ToolCalls) != 1 || am.ToolCalls[0].ID != "call_1" {
		t.Errorf("msgs[1] = %s; want assistant batch with call_1", msgSummary(&am))
	}

	if am.ToolCalls[0].Name != toolBash || string(am.ToolCalls[0].Input) != `{"command":"ls"}` {
		t.Errorf("assistant batch call = %+v; want Bash with the recorded input", am.ToolCalls[0])
	}

	tm := msgs[2]
	if tm.Role != roleToolMsg || tm.ToolCallID != "call_1" {
		t.Errorf("msgs[2] = %s; want tool message paired to call_1", msgSummary(&tm))
	}

	if tm.ToolName != toolBash {
		t.Errorf("tool message name = %q; want Bash resolved from the paired call", tm.ToolName)
	}

	if tm.Content != `{"out":"files"}` {
		t.Errorf("tool message content = %q; want the recorded output", tm.Content)
	}
}

// msgSummaryList renders a whole window for failure messages.
func msgSummaryList(msgs []provider.Message) string {
	parts := make([]string, 0, len(msgs))
	for i := range msgs {
		parts = append(parts, fmt.Sprintf("[%d] %s", i, msgSummary(&msgs[i])))
	}

	return strings.Join(parts, "\n")
}

// TestProjector_MidTurnBatchGrouping (08-07 T2 Test 3): three consecutive
// tool_call lines fold into ONE assistant message carrying three ToolCalls,
// followed by three tool-role messages — the capture's batch form (one
// assistant per response batch, one result per call).
func TestProjector_MidTurnBatchGrouping(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "go"}})
	_ = m.AppendToolCall("turnT", "c1", toolRead, json.RawMessage(`{"file_path":"a"}`))
	_ = m.AppendToolCall("turnT", "c2", "Grep", json.RawMessage(`{"pattern":"x"}`))
	_ = m.AppendToolCall("turnT", "c3", toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult("turnT", "c1", json.RawMessage(`{"o":"1"}`), false)
	_ = m.AppendToolResult("turnT", "c2", json.RawMessage(`{"o":"2"}`), false)
	_ = m.AppendToolResult("turnT", "c3", json.RawMessage(`{"o":"3"}`), true)

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	if len(msgs) != 5 {
		t.Fatalf("len(msgs) = %d, want 5:\n%s", len(msgs), msgSummaryList(msgs))
	}

	am := msgs[1]
	if am.Role != roleAssistant || len(am.ToolCalls) != 3 {
		t.Fatalf("msgs[1] = %s; want ONE assistant message with 3 ToolCalls", msgSummary(&am))
	}

	wantIDs := []string{"c1", "c2", "c3"}
	for i, want := range wantIDs {
		if am.ToolCalls[i].ID != want {
			t.Errorf("ToolCalls[%d].ID = %q, want %q", i, am.ToolCalls[i].ID, want)
		}
	}

	for i, want := range wantIDs {
		tm := msgs[2+i]
		if tm.Role != roleToolMsg || tm.ToolCallID != want {
			t.Errorf("msgs[%d] = %s; want tool result for %s", 2+i, msgSummary(&tm), want)
		}
	}

	if !msgs[4].IsError {
		t.Error("third result should carry IsError (recorded true)")
	}
}

// Shared fixture ids for the 08-09 boundary-carry batteries (goconst: each
// appears in both the fixture construction and the assertions of two tests).
const (
	fxPreOne  = "pre_1"
	fxPostOne = "post_1"
)

// TestProjector_MidTurnBoundaryCarry (08-09 T1 Test 1, re-pins 08-07 T2 Test 5
// `TestProjector_MidTurnBoundaryReset`): a mid-turn boundary NO LONGER resets
// the producing turn's own accumulation — the projection carries BOTH the
// pre-boundary and post-boundary exchanges of the SAME turn (capture-faithful:
// the zcode corpus never resets on tool results — 46/46 tail records, 579
// same-turn toolCallId persistences, session 4440f5a7). The boundary line is
// still recorded (writer unchanged, SESS-02/03 audit) and fires as the NEXT
// turn's reset boundary (asserted in TestProjector_BetweenTurnResetAfterMidTurn
// Boundary). An orphaned tool_result (its call never entered the window) is
// still dropped — pair-safety is boundary-independent.
func TestProjector_MidTurnBoundaryCarry(t *testing.T) { //nolint:gocyclo,cyclop,funlen // flat battery
	t.Parallel()

	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "do several"}})
	_ = m.AppendToolCall("turnT", fxPreOne, toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult("turnT", fxPreOne, json.RawMessage(`{"o":"x"}`), false)
	_ = m.AppendBoundary(mutatingCommandBash, fxPreOne, "turnT")
	// Post-boundary exchange, same turn.
	_ = m.AppendToolCall("turnT", fxPostOne, toolRead, json.RawMessage(`{"file_path":"a"}`))
	_ = m.AppendToolResult("turnT", fxPostOne, json.RawMessage(`{"o":"y"}`), false)
	// An orphaned result: its call was never recorded in the window.
	_ = m.AppendToolResult("turnT", "pre_2", json.RawMessage(`{"o":"z"}`), false)

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// Seed + BOTH exchanges (pre-boundary AND post-boundary). The pre_2 orphan
	// is dropped (no in-window batch — pair-safety, boundary-independent).
	if len(msgs) != 5 {
		t.Fatalf("len(msgs) = %d, want 5 (seed + pre_1 batch + pre_1 result + post_1 batch + post_1 result):\n%s",
			len(msgs), msgSummaryList(msgs))
	}

	if msgs[1].Role != roleAssistant || len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].ID != fxPreOne {
		t.Errorf("msgs[1] = %s; want the pre-boundary pre_1 batch carried", msgSummary(&msgs[1]))
	}

	if msgs[2].Role != roleToolMsg || msgs[2].ToolCallID != fxPreOne {
		t.Errorf("msgs[2] = %s; want the pre-boundary pre_1 result carried", msgSummary(&msgs[2]))
	}

	if msgs[3].Role != roleAssistant || len(msgs[3].ToolCalls) != 1 || msgs[3].ToolCalls[0].ID != fxPostOne {
		t.Errorf("msgs[3] = %s; want the post-boundary post_1 batch carried", msgSummary(&msgs[3]))
	}

	if msgs[4].Role != roleToolMsg || msgs[4].ToolCallID != fxPostOne {
		t.Errorf("msgs[4] = %s; want the post-boundary post_1 result carried", msgSummary(&msgs[4]))
	}

	for i, mm := range msgs {
		if mm.Role == roleToolMsg && mm.ToolCallID == "pre_2" {
			t.Errorf("msgs[%d] carries the orphaned pre_2 result (pair-safety violated):\n%s", i, msgSummaryList(msgs))
		}
	}

	// The boundary line is still in the transcript (writer unchanged — the
	// audit half of the revised invariant).
	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	sawBoundary := false

	for i := range lines {
		if lines[i].Type == TypeBoundary && lines[i].Cause == mutatingCommandBash &&
			lines[i].TurnID == "turnT" && lines[i].CommandRef == fxPreOne {
			sawBoundary = true
		}
	}

	if !sawBoundary {
		t.Error("no mutating-command:Bash boundary line in the transcript (SESS-02/03 writer half violated)")
	}
}

// TestProjector_BetweenTurnResetAfterMidTurnBoundary (08-09 T1 Test 2): the
// between-turn half of the revised invariant — the boundary recorded MID-turn N
// is turn N+1's reset boundary: the NEXT turn's projection is the lean seed
// ONLY (no assistant/tool messages), and its summary names the prior turn's
// content (the last user text before the boundary).
func TestProjector_BetweenTurnResetAfterMidTurnBoundary(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "do several"}})
	_ = m.AppendToolCall("turnT", fxPreOne, toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult("turnT", fxPreOne, json.RawMessage(`{"o":"x"}`), false)
	_ = m.AppendBoundary(mutatingCommandBash, fxPreOne, "turnT")
	_ = m.AppendToolCall("turnT", fxPostOne, toolRead, json.RawMessage(`{"file_path":"a"}`))
	_ = m.AppendToolResult("turnT", fxPostOne, json.RawMessage(`{"o":"y"}`), false)
	// The NEXT turn's user message arrives after the mutating work.
	_ = m.AppendUserMessage("turnN1", []ContentBlock{{Type: blockText, Text: "next please"}})

	msgs, err := p.Project("turnN1")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	for i, mm := range msgs {
		if mm.Role == roleAssistant || mm.Role == roleToolMsg {
			t.Errorf("next-turn projection[%d] = %s; want the lean seed ONLY (between-turn reset violated):\n%s",
				i, msgSummary(&mm), msgSummaryList(msgs))
		}
	}

	if len(msgs) > 3 {
		t.Errorf("next-turn projection has %d messages; want the lean seed (<=3):\n%s", len(msgs), msgSummaryList(msgs))
	}

	var combinedSb strings.Builder

	for _, mm := range msgs {
		combinedSb.WriteString(mm.Content + "\n")
	}

	combined := combinedSb.String()

	if !strings.Contains(combined, "next please") {
		t.Errorf("next-turn seed missing the current intent:\n%s", combined)
	}

	if !strings.Contains(combined, "do several") {
		t.Errorf("next-turn seed summary does not name the prior turn's content:\n%s", combined)
	}
}

// TestProjector_SeedStabilityWithinTurn (08-09 T1 Test 3): with NO prior
// boundary (the first mutating turn of a session), the seed message is
// BYTE-IDENTICAL across the turn's iterations — the summary scope is the lines
// BEFORE the turn's user message, so the current turn's accumulating exchanges
// never churn the seed (today's all-lines fallback dragged them into the
// summary and duplicated the intent).
func TestProjector_SeedStabilityWithinTurn(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnS", []ContentBlock{{Type: blockText, Text: "stable seed"}})
	_ = m.AppendToolCall("turnS", "s_a", toolRead, json.RawMessage(`{"file_path":"a"}`))
	_ = m.AppendToolResult("turnS", "s_a", json.RawMessage(`{"o":"1"}`), false)

	early, err := p.Project("turnS")
	if err != nil {
		t.Fatalf("Project (early): %v", err)
	}

	// Two more exchanges accumulate within the same turn.
	_ = m.AppendToolCall("turnS", "s_b", toolRead, json.RawMessage(`{"file_path":"b"}`))
	_ = m.AppendToolResult("turnS", "s_b", json.RawMessage(`{"o":"2"}`), false)
	_ = m.AppendToolCall("turnS", "s_c", toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult("turnS", "s_c", json.RawMessage(`{"o":"3"}`), false)

	late, err := p.Project("turnS")
	if err != nil {
		t.Fatalf("Project (late): %v", err)
	}

	if early[0].Content != late[0].Content {
		t.Errorf("seed CHURNED across the turn's iterations:\nearly=%q\nlate=%q", early[0].Content, late[0].Content)
	}

	if len(early) != 3 || len(late) != 7 {
		t.Errorf("window sizes = early %d / late %d; want 3 / 7 (the accumulation grows):\n%s\n%s",
			len(early), len(late), msgSummaryList(early), msgSummaryList(late))
	}

	if strings.Contains(late[0].Content, "s_b") || strings.Count(late[0].Content, "stable seed") != 1 {
		t.Errorf("late seed includes the current turn's exchanges (summary churn):\n%s", late[0].Content)
	}
}

// TestProjector_RepeatedMidTurnBoundaries (08-09 T1 Test 4): three mutating
// exchanges in ONE turn, a boundary after each — the projection still carries
// ALL THREE exchanges (today only the post-last-boundary tail survived, the
// 42/42 E2E signature). The 64-message bound itself is pinned separately by
// TestProjector_MidTurnWindowBound (unmodified).
func TestProjector_RepeatedMidTurnBoundaries(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnR", []ContentBlock{{Type: blockText, Text: "mutate thrice"}})

	for _, id := range []string{"m_1", "m_2", "m_3"} {
		_ = m.AppendToolCall("turnR", id, toolBash, json.RawMessage(`{"command":"cmd"}`))
		_ = m.AppendToolResult("turnR", id, json.RawMessage(`{"o":"`+id+`"}`), false)
		_ = m.AppendBoundary(mutatingCommandBash, id, "turnR")
	}

	msgs, err := p.Project("turnR")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	if len(msgs) != 7 {
		t.Fatalf("len(msgs) = %d, want 7 (seed + 3 exchanges x batch+result):\n%s", len(msgs), msgSummaryList(msgs))
	}

	for wantIdx, wantID := range []string{"m_1", "m_2", "m_3"} {
		batch := msgs[1+wantIdx*2]
		if batch.Role != roleAssistant || len(batch.ToolCalls) != 1 || batch.ToolCalls[0].ID != wantID {
			t.Errorf("msgs[%d] = %s; want the %s batch carried past its boundary",
				1+wantIdx*2, msgSummary(&batch), wantID)
		}

		res := msgs[2+wantIdx*2]
		if res.Role != roleToolMsg || res.ToolCallID != wantID {
			t.Errorf("msgs[%d] = %s; want the %s result carried past its boundary",
				2+wantIdx*2, msgSummary(&res), wantID)
		}
	}
}

// TestProjector_MidTurnWindowBound (08-07 T2 Test 6): a turn with more than
// MidTurnWindowMessages accumulated messages keeps the MOST RECENT tail,
// dropping only COMPLETE exchange groups (an assistant batch is never
// separated from its tool results) and never the seed. The bound is pinned to
// the captured zcode tail window (kind "tail", 64 messages observed).
func TestProjector_MidTurnWindowBound(t *testing.T) {
	t.Parallel()

	if MidTurnWindowMessages != 64 {
		t.Errorf("MidTurnWindowMessages = %d, want 64 (captured zcode tail window)", MidTurnWindowMessages)
	}

	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "many"}})

	// 40 single-call exchanges = 80 mid messages (batch + result each).
	for i := range 40 {
		id := fmt.Sprintf("call_%02d", i)
		_ = m.AppendToolCall("turnT", id, toolRead, json.RawMessage(`{"file_path":"f"}`))
		_ = m.AppendToolResult("turnT", id, json.RawMessage(`{"o":"r"}`), false)
	}

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// Seed + at most MidTurnWindowMessages mid messages.
	if len(msgs) > 1+MidTurnWindowMessages {
		t.Fatalf("len(msgs) = %d, want <= %d (seed + bound)", len(msgs), 1+MidTurnWindowMessages)
	}

	// Pair-safety: the first mid message must be an assistant batch head (a
	// leading tool-role message would be an orphaned result), and every kept
	// tool message's id must belong to a kept assistant batch.
	if msgs[1].Role != roleAssistant || len(msgs[1].ToolCalls) == 0 {
		t.Fatalf("msgs[1] = %s; want an assistant batch head (pair-safety)", msgSummary(&msgs[1]))
	}

	batchIDs := map[string]bool{}

	for _, mm := range msgs {
		for _, tc := range mm.ToolCalls {
			batchIDs[tc.ID] = true
		}
	}

	for i, mm := range msgs[1:] {
		if mm.Role == "tool" && !batchIDs[mm.ToolCallID] {
			t.Fatalf("msgs[%d] = %s — tool result separated from its batch (pair-safety violated)",
				i+1, msgSummary(&mm))
		}
	}

	// The MOST RECENT tail is kept: the last exchange (call_39) must be present.
	last := msgs[len(msgs)-1]
	if last.Role != roleToolMsg || last.ToolCallID != "call_39" {
		t.Errorf("last = %s; want the most recent exchange (call_39 result)", msgSummary(&last))
	}
}

// TestProjector_MidTurnEndOfTurnText (08-07 T2 Test 7): the turn's final
// assistant_message line renders as a plain assistant text message AFTER the
// last exchange within the same turn's window (the capture's assistant text
// blocks between batches).
func TestProjector_MidTurnEndOfTurnText(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "read then report"}})
	_ = m.AppendToolCall("turnT", "c1", toolRead, json.RawMessage(`{"file_path":"a"}`))
	_ = m.AppendToolResult("turnT", "c1", json.RawMessage(`{"o":"1"}`), false)
	_ = m.AppendAssistantMessage("turnT", "the file contains one entry")

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	if len(msgs) != 4 {
		t.Fatalf("len(msgs) = %d, want 4:\n%s", len(msgs), msgSummaryList(msgs))
	}

	last := msgs[3]
	if last.Role != roleAssistant || last.Content != "the file contains one entry" || len(last.ToolCalls) != 0 {
		t.Errorf("msgs[3] = %s; want plain assistant text after the exchange", msgSummary(&last))
	}
}

// TestPairingInvariant_ProjectorWindow (08-07 T3 Test 2, second arm): the
// pairing invariant holds over a Projector-PRODUCED window from a synthetic
// transcript — every tool message's ToolCallID is an element of a preceding
// assistant batch's ids in the window (T-8-28).
func TestPairingInvariant_ProjectorWindow(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-pair")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnP", []ContentBlock{{Type: blockText, Text: "multi-batch turn"}})

	// Batch 1: two calls + results. Batch 2: one call + result. Interleaved
	// assistant text. Then a dangling tool_call whose result has not landed —
	// the multi-permission-suspension resume state (17-REVIEW CR-01).
	_ = m.AppendToolCall("turnP", "p1", toolRead, json.RawMessage(`{"file_path":"a"}`))
	_ = m.AppendToolCall("turnP", "p2", toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult("turnP", "p1", json.RawMessage(`{"o":"1"}`), false)
	_ = m.AppendToolResult("turnP", "p2", json.RawMessage(`{"o":"2"}`), false)
	_ = m.AppendAssistantMessage("turnP", "interim note")
	_ = m.AppendToolCall("turnP", "p3", "Grep", json.RawMessage(`{"pattern":"x"}`))
	_ = m.AppendToolResult("turnP", "p3", json.RawMessage(`{"o":"3"}`), false)
	_ = m.AppendToolCall("turnP", "p4", toolRead, json.RawMessage(`{"file_path":"b"}`))

	msgs, err := p.Project("turnP")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// Walk the window: a tool message's id must be in the MOST RECENT batch.
	var batchIDs []string

	for i, mm := range msgs {
		switch {
		case len(mm.ToolCalls) > 0:
			batchIDs = batchIDs[:0]
			for _, tc := range mm.ToolCalls {
				batchIDs = append(batchIDs, tc.ID)
			}
		case mm.Role == roleToolMsg:
			if !slices.Contains(batchIDs, mm.ToolCallID) {
				t.Fatalf("window[%d] tool message id %q not in the preceding batch %v — pairing violated:\n%s",
					i, mm.ToolCallID, batchIDs, msgSummaryList(msgs))
			}
		}
	}

	// The UNANSWERED p4 call is NOT projected (CR-01): a projected tool_use
	// without its tool_result is an unpaired block the Anthropic-protocol
	// provider rejects — the pre-fix shape that broke every resumed turn
	// after a partial batch answer. (The old pin projected the dangling batch
	// deliberately; that state is only reachable across a suspension resume,
	// where projecting it IS the bug.) p1..p3 (answered) all still project,
	// and the window ends at p3's result.
	for _, id := range []string{"p1", "p2", "p3"} {
		found := false

		for _, mm := range msgs {
			for _, tc := range mm.ToolCalls {
				if tc.ID == id {
					found = true
				}
			}
		}

		if !found {
			t.Errorf("answered call %q missing from the projected window", id)
		}
	}

	for _, mm := range msgs {
		for _, tc := range mm.ToolCalls {
			if tc.ID == "p4" {
				t.Errorf("unanswered call p4 projected — unpaired tool_use (CR-01 pair-safety violated):\n%s",
					msgSummaryList(msgs))
			}
		}
	}

	last := msgs[len(msgs)-1]
	if last.Role != roleToolMsg || last.ToolCallID != "p3" {
		t.Errorf("last window message = %s; want p3's tool result (p4 contributes nothing)", msgSummary(&last))
	}
}

// TestProjector_ToolResultRedactionCarry (08-07 T3 Test 4, LOG-03 extends to
// the mid-turn window): a tool result written through the Manager with a
// redactable secret projects as the REDACTED content — the projection reads
// the redacted transcript file, so the model never sees the secret.
func TestProjector_ToolResultRedactionCarry(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-red")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnR", []ContentBlock{{Type: blockText, Text: "run"}})
	_ = m.AppendToolCall("turnR", "r1", toolBash, json.RawMessage(`{"command":"env"}`))
	_ = m.AppendToolResult("turnR", "r1",
		json.RawMessage(`{"token":"sk-livesecretvalue123","note":"ok"}`), false)

	msgs, err := p.Project("turnR")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var toolMsg *provider.Message

	for i := range msgs {
		if msgs[i].Role == roleToolMsg {
			toolMsg = &msgs[i]
		}
	}

	if toolMsg == nil {
		t.Fatal("no tool message in the projection")
	}

	if strings.Contains(toolMsg.Content, "sk-livesecretvalue123") {
		t.Errorf("tool result content leaked the secret: %q", toolMsg.Content)
	}

	if !strings.Contains(toolMsg.Content, "[REDACTED]") {
		t.Errorf("tool result content = %q; want the redacted placeholder", toolMsg.Content)
	}
}

// --- 08-08 T1: plain-text rendering of captured tool-result forms ---

// TestPlainContent_JSONStringUnquoted (08-08 T1 Test 2): a transcript
// tool_result line whose Output is the JSON string "Exit code 1\nTraceback…"
// projects to a tool-role Message whose Content is the UNQUOTED text — the
// captured zcode Bash error form is PLAIN TEXT on the wire-normalized tool
// message, so a marshaled JSON string must be decoded before it reaches the
// shaper (RED pre-fix: string(l.Output) carried the quotes).
func TestPlainContent_JSONStringUnquoted(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-plain1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnQ", []ContentBlock{{Type: blockText, Text: "run it"}})
	_ = m.AppendToolCall("turnQ", "q1", toolBash, json.RawMessage(`{"command":"false"}`))
	_ = m.AppendToolResult("turnQ", "q1", json.RawMessage(`"Exit code 1\nTraceback"`), true)

	msgs, err := p.Project("turnQ")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var toolMsg *provider.Message

	for i := range msgs {
		if msgs[i].Role == roleToolMsg {
			toolMsg = &msgs[i]
		}
	}

	if toolMsg == nil {
		t.Fatal("no tool message in the projection")
	}

	if toolMsg.Content != "Exit code 1\nTraceback" {
		t.Errorf("tool content = %q; want the UNQUOTED captured form \"Exit code 1\\nTraceback\"",
			toolMsg.Content)
	}
}

// TestPlainContent_ObjectsVerbatim (08-08 T1 Test 3): a tool_result whose
// Output is a JSON OBJECT projects Content as the same JSON object (not a
// decoded string) — the shipped openspec/Skill/subagent result rendering is
// unchanged; only JSON-string outputs decode. NOTE: the Manager's redactor
// parses + re-marshals object payloads (sorted key order, pre-existing 08-07
// behavior), so the assertion is deep-equal on the decoded value + object
// FORM (leading '{'), not raw byte order.
func TestPlainContent_ObjectsVerbatim(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-plain2")
	p := NewProjector(fakeProfile("sys"), m)

	const objOut = `{"stdout":"files","stderr":"","exit_code":0,"classification":"ok"}`

	_ = m.AppendUserMessage("turnO", []ContentBlock{{Type: blockText, Text: "go"}})
	_ = m.AppendToolCall("turnO", "o1", "openspec:explore", json.RawMessage(`{"topic":"x"}`))
	_ = m.AppendToolResult("turnO", "o1", json.RawMessage(objOut), false)

	msgs, err := p.Project("turnO")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var toolMsg *provider.Message

	for i := range msgs {
		if msgs[i].Role == roleToolMsg {
			toolMsg = &msgs[i]
		}
	}

	if toolMsg == nil {
		t.Fatal("no tool message in the projection")
	}

	if !strings.HasPrefix(toolMsg.Content, "{") {
		t.Fatalf("tool content = %q; want the JSON object form (not a decoded string)", toolMsg.Content)
	}

	var got, want any

	err = json.Unmarshal([]byte(toolMsg.Content), &got)
	if err != nil {
		t.Fatalf("tool content is not valid JSON: %v", err)
	}

	err = json.Unmarshal([]byte(objOut), &want)
	if err != nil {
		t.Fatalf("fixture output is not valid JSON: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("tool object = %#v; want the object verbatim %#v", got, want)
	}
}

// TestPlainContent_IsErrorPreserved (08-08 T1 Test 4): a tool_result line
// with IsError=true projects a tool Message with IsError=true — the flag the
// shaper renders as is_error (the captured `Exit code N` results carry it).
func TestPlainContent_IsErrorPreserved(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-plain3")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnE", []ContentBlock{{Type: blockText, Text: "go"}})
	_ = m.AppendToolCall("turnE", "e1", toolBash, json.RawMessage(`{"command":"false"}`))
	_ = m.AppendToolResult("turnE", "e1", json.RawMessage(`"Exit code 1\nboom"`), true)

	msgs, err := p.Project("turnE")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var toolMsg *provider.Message

	for i := range msgs {
		if msgs[i].Role == roleToolMsg {
			toolMsg = &msgs[i]
		}
	}

	if toolMsg == nil {
		t.Fatal("no tool message in the projection")
	}

	if !toolMsg.IsError {
		t.Error("tool message IsError = false; want true (the recorded line carries it)")
	}
}

// TestPlainContent_RedactionCarryOnStringOutputs (08-08 T1 Test 5, extends
// 08-07 T3 Test 4 to PLAIN-TEXT outputs): a JSON-string tool result containing
// a redactable secret projects as the REDACTED text — the Manager redacts the
// line before it lands on disk, and the plainContent decode of the redacted
// (still-valid) JSON string carries the placeholder, never the secret.
func TestPlainContent_RedactionCarryOnStringOutputs(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-plain4")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnS", []ContentBlock{{Type: blockText, Text: "env"}})
	_ = m.AppendToolCall("turnS", "s1", toolBash, json.RawMessage(`{"command":"env"}`))
	_ = m.AppendToolResult("turnS", "s1",
		json.RawMessage(`"KEY=sk-livesecretvalue123 done"`), false)

	msgs, err := p.Project("turnS")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var toolMsg *provider.Message

	for i := range msgs {
		if msgs[i].Role == roleToolMsg {
			toolMsg = &msgs[i]
		}
	}

	if toolMsg == nil {
		t.Fatal("no tool message in the projection")
	}

	if strings.Contains(toolMsg.Content, "sk-livesecretvalue123") {
		t.Errorf("plain-text tool result leaked the secret: %q", toolMsg.Content)
	}

	if !strings.Contains(toolMsg.Content, "[REDACTED]") {
		t.Errorf("plain-text tool result = %q; want the redacted placeholder", toolMsg.Content)
	}
}

// TestPlainContent_InvalidJSONFallback (08-08 T1 Test 6): an Output that is
// not valid JSON falls back to string(raw) unchanged — plainContent must
// never panic or drop results on undecodable bytes. NOTE: tested directly
// against plainContent because the Manager's appendLine MARSHALS the line
// (a json.RawMessage holding invalid JSON fails json.Marshal), so a
// non-JSON Output can never enter the transcript through AppendToolResult —
// the fallback guards the decode seam itself.
func TestPlainContent_InvalidJSONFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"json string decodes", `"Exit code 1\nboom"`, "Exit code 1\nboom"},
		{"object verbatim", `{"a":"b"}`, `{"a":"b"}`},
		{"number verbatim", `42`, `42`},
		{"invalid json raw fallback", `raw not-json text`, `raw not-json text`},
		{"lone quote raw fallback", `"unterminated`, `"unterminated`},
		{"empty verbatim", ``, ``},
	}

	for _, tc := range cases {
		if got := plainContent(json.RawMessage(tc.raw)); got != tc.want {
			t.Errorf("%s: plainContent(%q) = %q; want %q", tc.name, tc.raw, got, tc.want)
		}
	}
}
