package session //nolint:testpackage // internal package test

import (
	"encoding/json"
	"fmt"
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

// TestProjector_MidTurnBoundaryReset (08-07 T2 Test 5, NEW): a mid-turn
// boundary resets the accumulation — the projection shows the lean seed + only
// POST-boundary exchanges (D-11 within-turn semantics). Pre-boundary exchanges
// are never carried (D-01), and an orphaned tool_result (its batch cut off by
// the boundary) is dropped — pair-safety.
func TestProjector_MidTurnBoundaryReset(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "do several"}})
	_ = m.AppendToolCall("turnT", "pre_1", toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult("turnT", "pre_1", json.RawMessage(`{"o":"x"}`), false)
	_ = m.AppendBoundary(mutatingCommandBash, "pre_1", "turnT")
	// Post-boundary exchange.
	_ = m.AppendToolCall("turnT", "post_1", toolRead, json.RawMessage(`{"file_path":"a"}`))
	_ = m.AppendToolResult("turnT", "post_1", json.RawMessage(`{"o":"y"}`), false)
	// An orphaned result: its call was recorded BEFORE the boundary.
	_ = m.AppendToolResult("turnT", "pre_2", json.RawMessage(`{"o":"z"}`), false)

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	// Seed + post-boundary batch + post-boundary result ONLY. The pre_2 orphan
	// is dropped (no in-window batch), pre_1's exchange was reset away.
	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3 (post-boundary only):\n%s", len(msgs), msgSummaryList(msgs))
	}

	if msgs[1].Role != roleAssistant || len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].ID != "post_1" {
		t.Errorf("msgs[1] = %s; want post_1 batch only", msgSummary(&msgs[1]))
	}

	if msgs[2].Role != roleToolMsg || msgs[2].ToolCallID != "post_1" {
		t.Errorf("msgs[2] = %s; want post_1 result", msgSummary(&msgs[2]))
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
