package session //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
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
		if !projectedHasCall(msgs, id) {
			t.Errorf("answered call %q missing from the projected window", id)
		}
	}

	if projectedHasCall(msgs, "p4") {
		t.Errorf("unanswered call p4 projected — unpaired tool_use (CR-01 pair-safety violated):\n%s",
			msgSummaryList(msgs))
	}

	last := msgs[len(msgs)-1]
	if last.Role != roleToolMsg || last.ToolCallID != "p3" {
		t.Errorf("last window message = %s; want p3's tool result (p4 contributes nothing)", msgSummary(&last))
	}
}

// projectedHasCall reports whether any assistant batch in the projected
// window carries the given tool-call id.
func projectedHasCall(msgs []provider.Message, id string) bool {
	for _, mm := range msgs {
		for _, tc := range mm.ToolCalls {
			if tc.ID == id {
				return true
			}
		}
	}

	return false
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

// --- PAR-05 thinking projection battery (21-03, Task 3 RED) ---

// thSignedRaw is a signed thinking-block payload fixture (assembled-shape).
const thSignedRaw = `{"type":"thinking","thinking":"planning the batch","signature":"sig-p1"}`

// TestProjector_ThinkingFoldsIntoAssistantBatch (Pitfall 5): raw_thinking
// lines stash into the in-progress assistant accumulation and flush INTO the
// assistant batch message ALONGSIDE its tool_use blocks — never as a separate
// thinking-only message — and end-of-turn thinking rides WITH the final
// assistant text message.
func TestProjector_ThinkingFoldsIntoAssistantBatch(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-th1")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "go"}})
	_ = m.AppendRawThinking("turnT", fixtureModelSlug, json.RawMessage(thSignedRaw))
	_ = m.AppendToolCall("turnT", "call_1", toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult("turnT", "call_1", json.RawMessage(`"files"`), false)
	_ = m.AppendAssistantMessage("turnT", "done")

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	if len(msgs) != 4 {
		t.Fatalf("len(msgs) = %d, want 4 (seed + thinking-folded batch + tool + text):\n%s",
			len(msgs), msgSummaryList(msgs))
	}

	am := msgs[1]
	if am.Role != roleAssistant || len(am.ToolCalls) != 1 {
		t.Fatalf("msgs[1] = %s; want the assistant batch with call_1", msgSummary(&am))
	}

	// THE fold: the thinking block lives ON the batch message.
	if len(am.ThinkingBlocks) != 1 {
		t.Fatalf("batch ThinkingBlocks = %d, want exactly 1 (folded, not separate); got %+v",
			len(am.ThinkingBlocks), am.ThinkingBlocks)
	}

	want := provider.ThinkingBlock{Type: "thinking", Text: "planning the batch", Signature: "sig-p1"}
	if am.ThinkingBlocks[0] != want {
		t.Errorf("folded block = %+v; want %+v (field values extracted untouched)", am.ThinkingBlocks[0], want)
	}

	if txt := msgs[3]; txt.Role != roleAssistant || txt.Content != "done" {
		t.Errorf("msgs[3] = %s; want the final assistant text", msgSummary(&txt))
	}
}

// TestProjector_ThinkingAttachesToAssistantText: a turn whose stream carried
// thinking but NO tool calls attaches the block to the final assistant TEXT
// message (the end-turn unit), still never standalone.
func TestProjector_ThinkingAttachesToAssistantText(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-th2")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "hi"}})
	_ = m.AppendRawThinking("turnT", fixtureModelSlug, json.RawMessage(thSignedRaw))
	_ = m.AppendAssistantMessage("turnT", "hello back")

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2 (seed + assistant text):\n%s", len(msgs), msgSummaryList(msgs))
	}

	am := msgs[1]
	if am.Role != roleAssistant || am.Content != "hello back" {
		t.Fatalf("msgs[1] = %s; want the assistant text message", msgSummary(&am))
	}

	if len(am.ThinkingBlocks) != 1 {
		t.Fatalf("text message ThinkingBlocks = %d, want 1 (end-of-turn thinking rides with its text message)",
			len(am.ThinkingBlocks))
	}
}

// TestProjector_ThinkingOrphanDroppedWithTurn (Pitfall 5's orphan rule): a
// raw_thinking line whose assistant batch never forms — the turn truncated
// before any assistant content — is dropped WITH its turn unit; a standalone
// thinking-only assistant message must never be emitted (it would break the
// provider's thinking/assistant-message pairing and 400).
func TestProjector_ThinkingOrphanDroppedWithTurn(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-th3")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "go"}})
	_ = m.AppendRawThinking("turnT", fixtureModelSlug, json.RawMessage(thSignedRaw))
	// Turn truncated: no tool result, no assistant message.

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	for i := range msgs {
		if len(msgs[i].ThinkingBlocks) > 0 {
			t.Fatalf("msgs[%d] carries %d thinking block(s); want NONE (orphan dropped with its turn):\n%s",
				i, len(msgs[i].ThinkingBlocks), msgSummaryList(msgs))
		}
	}

	// Unanswered tool call variant: the batch is dropped (pair-safety), and the
	// stashed thinking goes WITH it.
	m2 := newTestManager(t, "s-th4")
	p2 := NewProjector(fakeProfile("sys"), m2)

	_ = m2.AppendUserMessage("turnU", []ContentBlock{{Type: blockText, Text: "go"}})
	_ = m2.AppendRawThinking("turnU", fixtureModelSlug, json.RawMessage(thSignedRaw))
	_ = m2.AppendToolCall("turnU", "call_9", toolBash, json.RawMessage(`{"command":"ls"}`))
	// No tool_result for call_9 — unanswered.

	msgs2, err := p2.Project("turnU")
	if err != nil {
		t.Fatalf("Project(unanswered): %v", err)
	}

	for i := range msgs2 {
		if len(msgs2[i].ThinkingBlocks) > 0 {
			t.Fatalf("unanswered-case msgs[%d] carries thinking; want NONE (dropped with its never-formed batch)",
				i)
		}
	}
}

// TestProjector_ThinkingNeverDuplicatedAcrossBatchAndText (21-REVIEW WR-04):
// an assistant_message line following UNFLUSHED tool calls while the thinking
// stash is non-empty must not paste the same signed block onto BOTH the
// flushed batch message and the text message — duplicated signed thinking is
// a provider 400 shape (Pitfall 5's family). The batch consumes the stash;
// the text message follows it only when NO batch took it.
func TestProjector_ThinkingNeverDuplicatedAcrossBatchAndText(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-thdup")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "go"}})
	_ = m.AppendRawThinking("turnT", fixtureModelSlug, json.RawMessage(thSignedRaw))
	_ = m.AppendToolCall("turnT", "call_1", toolBash, json.RawMessage(`{"command":"ls"}`))
	// The ordering hazard: assistant text lands while call_1's batch is still
	// unflushed (its tool_result arrives only AFTER the text line).
	_ = m.AppendAssistantMessage("turnT", "interim note")
	_ = m.AppendToolResult("turnT", "call_1", json.RawMessage(`"files"`), false)

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	total, carriers := 0, []int{}

	for i := range msgs {
		if n := len(msgs[i].ThinkingBlocks); n > 0 {
			total += n
			carriers = append(carriers, i)
		}
	}

	if total != 1 || len(carriers) != 1 {
		t.Fatalf("thinking appears %d time(s) on %v; want EXACTLY ONCE (an emitted batch consumes the stash — "+
			"duplicated signed thinking is a provider 400 shape):\n%s", total, carriers, msgSummaryList(msgs))
	}

	// The one carrier is the assistant BATCH (the fold), never the text message.
	batch := msgs[carriers[0]]
	if batch.Role != roleAssistant || len(batch.ToolCalls) != 1 {
		t.Fatalf("thinking carrier = %s; want the assistant batch carrying call_1", msgSummary(&batch))
	}
}

// TestProjector_ThinkingBoundaryAdjacent pins the flagged PAR-05 boundary row:
// a boundary line landing MID-thinking-turn (the mutating tool's result
// boundary, SESS-02/03) never splits thinking from its assistant message —
// the projection window keeps the WHOLE turn unit (SESS-04 revised 08-09:
// mid-turn boundaries never wipe the producing turn's window).
func TestProjector_ThinkingBoundaryAdjacent(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "s-th5")
	p := NewProjector(fakeProfile("sys"), m)

	_ = m.AppendUserMessage("turnT", []ContentBlock{{Type: blockText, Text: "go"}})
	_ = m.AppendRawThinking("turnT", fixtureModelSlug, json.RawMessage(thSignedRaw))
	_ = m.AppendToolCall("turnT", "call_1", toolBash, json.RawMessage(`{"command":"ls"}`))
	_ = m.AppendToolResult("turnT", "call_1", json.RawMessage(`"files"`), false)
	// The mid-turn boundary (mutating Bash completed) — turnT's own window
	// survives it.
	_ = m.AppendBoundary(mutatingCommandBash, "call_1", "turnT")
	_ = m.AppendAssistantMessage("turnT", "done")

	msgs, err := p.Project("turnT")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var batch *provider.Message

	for i := range msgs {
		if msgs[i].Role == roleAssistant && len(msgs[i].ToolCalls) == 1 {
			batch = &msgs[i]
		}
	}

	if batch == nil {
		t.Fatalf("the assistant batch vanished across the mid-turn boundary:\n%s", msgSummaryList(msgs))
	}

	if len(batch.ThinkingBlocks) != 1 {
		t.Fatalf("batch ThinkingBlocks across a mid-turn boundary = %d, want 1 (whole turn unit kept)",
			len(batch.ThinkingBlocks))
	}

	// No standalone thinking-only message anywhere.
	for i := range msgs {
		if len(msgs[i].ThinkingBlocks) > 0 && len(msgs[i].ToolCalls) == 0 && msgs[i].Content == "" {
			t.Fatalf("msgs[%d] is a standalone thinking-only message — never allowed:\n%s",
				i, msgSummaryList(msgs))
		}
	}
}

// --- PAR-01 durable reset-point battery (19-03 Task 2) ---

// Compaction-marker fixture summaries (unique markers per case).
const (
	sumMarkerWins  = "SUMMARY-A: parser work complete, 3 tests green"
	sumDurable     = "SUMMARY-B: db migrated and seeded"
	sumMarkerLate  = "SUMMARY-C: newer marker"
	sumMarkerEarly = "SUMMARY-D: older marker"
	sumMidFlight   = "SUMMARY-MID: compaction fired mid-subagent"
)

// TestProjector_CompactionResetPoint pins PAR-01's projection semantics: the
// compaction marker is the Projector's THIRD class — a durable reset point
// whose summary seed survives later mutating boundaries (D-06) — while every
// transcript WITHOUT a marker projects byte-identically to pre-phase behavior,
// and a marker never disturbs a turn already in flight (Pitfall 5: it resets
// turns that START after it, exactly like TypeBoundary).
func TestProjector_CompactionResetPoint(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen,maintidx // five cases
	t.Parallel()

	t.Run("marker wins the reset-point scan; seed is the marker's summary", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-crp1")
		p := NewProjector(fakeProfile("sys"), m)

		_ = m.AppendUserMessage("turn_0", []ContentBlock{{Type: blockText, Text: "fix the parser"}})
		_ = m.AppendToolCall("turn_0", "tc_p1", toolRead, json.RawMessage(`{"file_path":"/w/parser.go"}`))
		_ = m.AppendToolResult("turn_0", "tc_p1", json.RawMessage(`{"o":"src"}`), false)
		_ = m.AppendAssistantMessage("turn_0", "parser fixed")
		mustAppend(t,
			m.AppendCompaction("turn_0", "line:4", "line:5", sumMarkerWins, 1500, 300, 40), "AppendCompaction")
		_ = m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: "now the docs"}})

		msgs, err := p.Project("turn_1")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		if len(msgs) == 0 {
			t.Fatal("empty projection — fixture broken")
		}

		seed := msgs[0]
		if seed.Role != roleUserMsg {
			t.Fatalf("seed role = %q; want user (single-user-message lean-seed shape)", seed.Role)
		}

		if !strings.Contains(seed.Content, sumMarkerWins) {
			t.Errorf("seed missing the marker's summary (marker must WIN the reset-point scan):\n%s", seed.Content)
		}

		if !strings.Contains(seed.Content, "now the docs") {
			t.Errorf("seed missing the current intent:\n%s", seed.Content)
		}

		// Summary-ONLY seed: the mechanical extractSummary vocabulary (and the
		// pre-marker content it would summarize) must NOT leak into the seed.
		for _, leak := range []string{
			"last_user=", "last_assistant=", "files_touched=", "fix the parser", "parser fixed",
		} {
			if strings.Contains(seed.Content, leak) {
				t.Errorf("seed carries the mechanical no-marker summary %q (marker summary alone):\n%s",
					leak, seed.Content)
			}
		}
	})

	t.Run("later boundary does not displace the summary (D-06 durable, summary-only)", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-crp2")
		p := NewProjector(fakeProfile("sys"), m)

		_ = m.AppendUserMessage("turn_0", []ContentBlock{{Type: blockText, Text: "setup db"}})
		_ = m.AppendToolCall("turn_0", "tc_d1", toolBash, json.RawMessage(`{"command":"migrate"}`))
		_ = m.AppendToolResult("turn_0", "tc_d1", json.RawMessage(`{"o":"ok"}`), false)
		mustAppend(t, m.AppendCompaction("turn_0", "line:3", "line:4", sumDurable, 1600, 200, 60), "AppendCompaction")
		// Post-marker turn whose mutating boundary lands BETWEEN the marker and
		// the projected turn.
		_ = m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: "seed the data"}})
		_ = m.AppendToolCall("turn_1", "tc_d2", toolBash, json.RawMessage(`{"command":"seed"}`))
		_ = m.AppendToolResult("turn_1", "tc_d2", json.RawMessage(`{"o":"rows"}`), false)
		_ = m.AppendBoundary(mutatingCommandBash, "tc_d2", "turn_1")
		_ = m.AppendUserMessage("turn_2", []ContentBlock{{Type: blockText, Text: "verify counts"}})

		msgs, err := p.Project("turn_2")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		seed := msgs[0]

		if !strings.Contains(seed.Content, sumDurable) {
			t.Errorf("the later mutating boundary DISPLACED the durable summary (D-06 violation):\n%s", seed.Content)
		}

		// Open Question 3's planner-pinned reading: summary-ONLY — the boundary
		// does not re-derive a mechanical summary over the post-marker span.
		for _, leak := range []string{"last_user=", "last_assistant=", "seed the data", "files_touched="} {
			if strings.Contains(seed.Content, leak) {
				t.Errorf("seed after a later boundary merges mechanical summary %q (marker summary ALONE):\n%s",
					leak, seed.Content)
			}
		}

		if !strings.Contains(seed.Content, "verify counts") {
			t.Errorf("seed missing the current intent:\n%s", seed.Content)
		}

		// The window stays lean: the boundary dropped the post-marker tail
		// (tail follows existing boundary discipline; only the summary is
		// durable), so turn_2 projects the seed alone.
		for i, mm := range msgs {
			if mm.Role == roleAssistant || mm.Role == roleToolMsg {
				t.Errorf("msgs[%d] = %s; want the lean seed only (tail is boundary-droppable, summary is durable):\n%s",
					i, msgSummary(&mm), msgSummaryList(msgs))
			}
		}
	})

	t.Run("no marker projects byte-identically (boundary-only pin)", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-crp3")
		p := NewProjector(fakeProfile("sys"), m)

		_ = m.AppendUserMessage("turn_0", []ContentBlock{{Type: blockText, Text: "edit the file"}})
		_ = m.AppendToolCall("turn_0", "tc_e1", toolRead, json.RawMessage(`{"file_path":"/a/go.mod"}`))
		_ = m.AppendToolResult("turn_0", "tc_e1", json.RawMessage(`{"out":"module x"}`), false)
		_ = m.AppendAssistantMessage("turn_0", "done editing")
		_ = m.AppendBoundary("mutating-command:Edit", "tc_e1", "turn_0")
		_ = m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: "next step"}})
		_ = m.AppendToolCall("turn_1", "tc_e2", toolBash, json.RawMessage(`{"command":"ls"}`))
		_ = m.AppendToolResult("turn_1", "tc_e2", json.RawMessage(`"files"`), false)

		msgs, err := p.Project("turn_1")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		// The EXACT pre-phase window, hand-derived from the D-01/D-02 rules:
		// mechanical summary over the pre-boundary lines + current intent +
		// the turn's own mid-turn accumulation (boundMidTurn — 3 messages).
		wantSeed := "Task summary (mechanical, post-boundary):\n" +
			"last_user=edit the file\n" +
			"last_assistant=done editing\n" +
			"files_touched=/a/go.mod" +
			"\n\n--- Current request ---\nnext step"

		want := []provider.Message{
			{Role: roleUserMsg, Content: wantSeed},
			{Role: roleAssistant, ToolCalls: []provider.ToolCall{
				{ID: "tc_e2", Name: toolBash, Input: json.RawMessage(`{"command":"ls"}`)},
			}},
			{Role: roleToolMsg, ToolCallID: "tc_e2", ToolName: toolBash, Content: "files"},
		}

		if !reflect.DeepEqual(msgs, want) {
			t.Errorf("boundary-only transcript drifted from the pre-phase window:\n got: %s\nwant: %s",
				msgSummaryList(msgs), msgSummaryList(want))
		}
	})

	t.Run("subagent in-flight window unchanged; next parent turn seeded", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-crp4")
		p := NewProjector(fakeProfile("sys"), m)

		_ = m.AppendUserMessage("turnP1", []ContentBlock{{Type: blockText, Text: "spawn a helper"}})
		// The subagent's turn STARTS (its user message lands) before the marker.
		_ = m.AppendUserMessage("turnSA", []ContentBlock{{Type: blockText, Text: "subagent task"}})
		_ = m.AppendToolCall("turnSA", "tc_s1", toolRead, json.RawMessage(`{"file_path":"a"}`))
		_ = m.AppendToolResult("turnSA", "tc_s1", json.RawMessage(`{"o":"1"}`), false)
		// The marker lands MID-subagent-turn (Pitfall 5).
		mustAppend(t, m.AppendCompaction("turnP1", "line:4", "line:5", sumMidFlight, 1700, 100, 80), "AppendCompaction")
		// The subagent keeps working past the marker.
		_ = m.AppendToolCall("turnSA", "tc_s2", toolBash, json.RawMessage(`{"command":"ls"}`))
		_ = m.AppendToolResult("turnSA", "tc_s2", json.RawMessage(`{"o":"2"}`), false)
		// The next parent turn starts AFTER the marker.
		_ = m.AppendUserMessage("turnP2", []ContentBlock{{Type: blockText, Text: "parent resumes"}})

		saMsgs, err := p.Project("turnSA")
		if err != nil {
			t.Fatalf("Project(subagent): %v", err)
		}

		// The subagent's own window is UNCHANGED: its turn started before the
		// marker, so the marker is not ITS reset point — the seed is the
		// pre-phase mechanical pre-turn summary, never the marker summary.
		if strings.Contains(saMsgs[0].Content, sumMidFlight) {
			t.Errorf("marker summary leaked into the in-flight subagent's seed (Pitfall 5):\n%s", saMsgs[0].Content)
		}

		if !strings.Contains(saMsgs[0].Content, "spawn a helper") {
			t.Errorf("subagent seed lost the pre-phase mechanical summary:\n%s", saMsgs[0].Content)
		}

		// Its mid-turn window keeps BOTH exchanges across the marker.
		for _, id := range []string{"tc_s1", "tc_s2"} {
			if !projectedHasCall(saMsgs, id) {
				t.Errorf("subagent window lost in-flight exchange %s across the marker:\n%s",
					id, msgSummaryList(saMsgs))
			}
		}

		p2Msgs, err := p.Project("turnP2")
		if err != nil {
			t.Fatalf("Project(parent): %v", err)
		}

		if !strings.Contains(p2Msgs[0].Content, sumMidFlight) {
			t.Errorf("next parent turn after the marker missing the summary seed:\n%s", p2Msgs[0].Content)
		}
	})

	t.Run("most recent marker before the user message wins", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-crp5")
		p := NewProjector(fakeProfile("sys"), m)

		_ = m.AppendUserMessage("turn_0", []ContentBlock{{Type: blockText, Text: "one"}})
		_ = m.AppendAssistantMessage("turn_0", "a1")
		mustAppend(t, m.AppendCompaction("turn_0", "line:2", "line:3", sumMarkerEarly, 100, 10, 1), "AppendCompaction")
		_ = m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: "two"}})
		_ = m.AppendAssistantMessage("turn_1", "a2")
		mustAppend(t, m.AppendCompaction("turn_1", "line:5", "line:6", sumMarkerLate, 200, 20, 2), "AppendCompaction")
		_ = m.AppendUserMessage("turn_2", []ContentBlock{{Type: blockText, Text: "three"}})

		msgs, err := p.Project("turn_2")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		seed := msgs[0]

		if !strings.Contains(seed.Content, sumMarkerLate) {
			t.Errorf("seed does not use the MOST RECENT marker's summary:\n%s", seed.Content)
		}

		if strings.Contains(seed.Content, sumMarkerEarly) {
			t.Errorf("an EARLIER marker's summary survived a later marker (only a NEWER marker replaces the seed):\n%s",
				seed.Content)
		}

		// Position rule: a marker appended AFTER the projected turn's user
		// message (mid-flight compaction of turn_2 itself) must NOT become
		// turn_2's reset point.
		mustAppend(t,
			m.AppendCompaction("turn_2", "line:7", "line:8", "SUMMARY-INFLIGHT", 300, 30, 3), "AppendCompaction")

		again, err := p.Project("turn_2")
		if err != nil {
			t.Fatalf("Project(re-probe): %v", err)
		}

		if strings.Contains(again[0].Content, "SUMMARY-INFLIGHT") {
			t.Errorf("a marker landing mid-turn reset the PRODUCING turn's window (only turns STARTING after it):\n%s",
				again[0].Content)
		}

		if !strings.Contains(again[0].Content, sumMarkerLate) {
			t.Errorf("re-probe lost the winning marker summary:\n%s", again[0].Content)
		}
	})
}

// --- PAR-01 budget-fill tail battery (19-03 Task 3) ---

// sumTailFill is the marker summary for the tail-cut fixtures (short, so the
// summary estimate leaves the fill target meaningful).
const sumTailFill = "SUMMARY-TAIL: bulk work absorbed"

// tailMsgCost is the D-05 budget unit as the behavior spec defines it: the
// message's folded JSON size divided by 4, truncating. Mirrors the production
// cost rule (chars/4-class estimate, D-01's no-tokenizer discipline). Pointer
// form matches messageCost (Message is 144 bytes).
func tailMsgCost(m *provider.Message) int64 {
	raw, err := json.Marshal(m)
	if err != nil {
		return 0 // Message always marshals (plain fields + RawMessage inputs)
	}

	return int64(len(raw)) / 4
}

// appendTailGroups appends n uniform exchange groups (tool_call + fat tool
// result) to the given turn. The fat payloads make each group's cost large
// and predictable, so a fixed budget deterministically forces a cut.
func appendTailGroups(t *testing.T, m *Manager, turnID string, n int) {
	t.Helper()

	for i := range n {
		id := fmt.Sprintf("%s_g%02d", turnID, i)
		mustAppend(t, m.AppendToolCall(turnID, id, toolBash, json.RawMessage(`{"command":"ls"}`)), "AppendToolCall")
		mustAppend(t,
			m.AppendToolResult(turnID, id, json.RawMessage(`"`+strings.Repeat("x", 2000)+`"`), false),
			"AppendToolResult")
	}
}

// assertTailPairSafe walks the tail (msgs[1:]) proving the cut is pair-atomic:
// the tail starts at a group head (assistant batch), and every tool message's
// id belongs to a preceding kept batch — no orphaned tool result anywhere.
func assertTailPairSafe(t *testing.T, msgs []provider.Message) {
	t.Helper()

	if len(msgs) < 2 {
		t.Fatal("no tail after the seed — fixture broken (or an over-aggressive cut)")
	}

	if msgs[1].Role != roleAssistant || len(msgs[1].ToolCalls) == 0 {
		t.Fatalf("tail head = %s; want an assistant batch head (never an orphaned tool result)", msgSummary(&msgs[1]))
	}

	batchIDs := map[string]bool{}

	for i := range msgs {
		for _, tc := range msgs[i].ToolCalls {
			batchIDs[tc.ID] = true
		}
	}

	for i := 1; i < len(msgs); i++ {
		if msgs[i].Role == roleToolMsg && !batchIDs[msgs[i].ToolCallID] {
			t.Fatalf("tail[%d] = %s — tool result separated from its batch (pair-atomicity violated)",
				i, msgSummary(&msgs[i]))
		}
	}
}

// TestProjector_CompactionTailCut pins the D-04/D-05 post-marker tail: the
// durable summary seed is followed by a budget-fill tail of the most recent
// post-marker messages (across turns), cut only at group boundaries — never
// starting with an orphaned tool result, never rewriting thinking (complete
// groups drop alone) — with a zero/unset budget degrading to the
// MidTurnWindowMessages count bound.
func TestProjector_CompactionTailCut(t *testing.T) { //nolint:funlen,gocognit,gocyclo,cyclop // three fixtures
	t.Parallel()

	t.Run("budget-fill cut is pair-atomic, multi-turn, and respects the fill target", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-ctc1")
		p := NewProjector(fakeProfile("sys"), m)

		_ = m.AppendUserMessage("turn_0", []ContentBlock{{Type: blockText, Text: "bulk history"}})
		mustAppend(t, m.AppendCompaction("turn_0", "line:1", "line:2", sumTailFill, 9000, 900, 900), "AppendCompaction")

		// Post-marker span: a prior turn's 12 groups + the projected turn's
		// own 4 — the tail must draw from BOTH (D-04's recent-tail grip).
		_ = m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: "phase one"}})
		appendTailGroups(t, m, "turn_1", 12)
		_ = m.AppendUserMessage("turn_2", []ContentBlock{{Type: blockText, Text: "phase two"}})
		appendTailGroups(t, m, "turn_2", 4)

		const budget = 8000

		p.SetCompactionTailBudget(budget)

		msgs, err := p.Project("turn_2")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		// Seed half: durable summary + current intent.
		if !strings.Contains(msgs[0].Content, sumTailFill) || !strings.Contains(msgs[0].Content, "phase two") {
			t.Errorf("seed = %q; want the marker summary + current intent", msgs[0].Content)
		}

		// The budget forced a cut: leading groups are dropped...
		if projectedHasCall(msgs, "turn_1_g00") {
			t.Errorf("the oldest post-marker group survived the budget cut (tail not budget-filled):\n%s",
				msgSummaryList(msgs))
		}

		// ...the tail spans turns (starts inside turn_1's groups) and keeps
		// the projected turn's own most recent exchange.
		if len(msgs[1].ToolCalls) == 0 || !strings.HasPrefix(msgs[1].ToolCalls[0].ID, "turn_1_g") {
			t.Errorf("tail head = %s; want a turn_1 group (the tail must span prior-turn exchanges)",
				msgSummary(&msgs[1]))
		}

		if !projectedHasCall(msgs, "turn_2_g03") {
			t.Errorf("the most recent exchange (turn_2_g03) missing from the tail:\n%s", msgSummaryList(msgs))
		}

		// Pair-atomicity at the cut.
		assertTailPairSafe(t, msgs)

		// D-05 fill target: summary estimate + tail cost <= 60% of the limit
		// (headroom under the 80% trigger).
		tail := msgs[1:]

		var total int64

		for i := range tail {
			total += tailMsgCost(&tail[i])
		}

		summaryEst := int64(len(sumTailFill)) / 4
		fillTarget := int64(budget) * compactionFillTargetPct / 100

		if total+summaryEst > fillTarget {
			t.Errorf("summary estimate + tail cost = %d; want <= %d (60%% of the %d limit)",
				total+summaryEst, fillTarget, budget)
		}
	})

	t.Run("zero budget falls back to the 64-message bound", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-ctc2")
		p := NewProjector(fakeProfile("sys"), m)

		_ = m.AppendUserMessage("turn_0", []ContentBlock{{Type: blockText, Text: "count me"}})
		mustAppend(t, m.AppendCompaction("turn_0", "line:1", "line:2", sumTailFill, 100, 10, 10), "AppendCompaction")

		// 40 uniform groups = 80 post-marker messages — over the 64 bound.
		_ = m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: "grow"}})
		appendTailGroups(t, m, "turn_1", 40)
		_ = m.AppendUserMessage("turn_2", []ContentBlock{{Type: blockText, Text: "inspect"}})

		// CompactionTailBudget left at its zero value — the fallback.

		msgs, err := p.Project("turn_2")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		if want := 1 + MidTurnWindowMessages; len(msgs) != want {
			t.Fatalf("len(msgs) = %d; want %d (seed + the 64-message fallback bound):\n%s",
				len(msgs), want, msgSummaryList(msgs))
		}

		// The count cut keeps the MOST RECENT 64: uniform groups mean the cut
		// lands exactly at group 8's batch head (80-64=16=2*8) and the final
		// group's result survives.
		if id := msgs[1].ToolCalls[0].ID; id != "turn_1_g08" {
			t.Errorf("fallback tail head = %s; want turn_1_g08 (the group-head advance at cut 16)", id)
		}

		last := msgs[len(msgs)-1]
		if last.Role != roleToolMsg || last.ToolCallID != "turn_1_g39" {
			t.Errorf("tail end = %s; want turn_1_g39's result (most-recent kept)", msgSummary(&last))
		}

		assertTailPairSafe(t, msgs)
	})

	t.Run("thinking survives untouched inside kept groups", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-ctc3")
		p := NewProjector(fakeProfile("sys"), m)

		const sumThink = "SUMMARY-THINK: exact"

		_ = m.AppendUserMessage("turn_0", []ContentBlock{{Type: blockText, Text: "think work"}})
		mustAppend(t, m.AppendCompaction("turn_0", "line:1", "line:2", sumThink, 100, 10, 10), "AppendCompaction")

		_ = m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: "go"}})

		// Four thinking-bearing groups; the budget keeps only the last two —
		// dropped groups take their thinking with them (complete groups drop
		// alone; no chain is ever rewritten).
		for i := range 4 {
			id := fmt.Sprintf("t_g%02d", i)
			thinking := json.RawMessage(
				fmt.Sprintf(`{"type":"thinking","thinking":"think-%d","signature":"sig-%d"}`, i, i))
			mustAppend(t, m.AppendRawThinking("turn_1", fixtureModelSlug, thinking), "AppendRawThinking")
			mustAppend(t,
				m.AppendToolCall("turn_1", id, toolBash, json.RawMessage(`{"command":"ls"}`)), "AppendToolCall")
			mustAppend(t,
				m.AppendToolResult("turn_1", id, json.RawMessage(`"`+strings.Repeat("x", 3000)+`"`), false),
				"AppendToolResult")
		}

		_ = m.AppendUserMessage("turn_2", []ContentBlock{{Type: blockText, Text: "report"}})

		// Two groups (~2x840) fit the 60% target of 3000; three do not.
		p.SetCompactionTailBudget(3000)

		msgs, err := p.Project("turn_2")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		for i := range msgs {
			for _, tb := range msgs[i].ThinkingBlocks {
				if tb.Text == "think-0" || tb.Text == "think-1" {
					t.Errorf("dropped group's thinking (%q) survived outside its group — groups drop COMPLETE",
						tb.Text)
				}
			}
		}

		// Kept groups carry their thinking untouched (field values exact —
		// PAR-01's never-rewrite letter).
		want := map[string]provider.ThinkingBlock{
			"t_g02": {Type: chunkTypeThinking, Text: "think-2", Signature: "sig-2"},
			"t_g03": {Type: chunkTypeThinking, Text: "think-3", Signature: "sig-3"},
		}

		got := map[string][]provider.ThinkingBlock{}

		for i := range msgs {
			if len(msgs[i].ToolCalls) == 1 {
				got[msgs[i].ToolCalls[0].ID] = msgs[i].ThinkingBlocks
			}
		}

		for id, wantBlocks := range want {
			blocks, ok := got[id]
			if !ok {
				t.Errorf("kept group %s missing from the tail (budget math off):\n%s", id, msgSummaryList(msgs))

				continue
			}

			if len(blocks) != 1 || blocks[0] != wantBlocks {
				t.Errorf("group %s thinking = %+v; want exactly [%+v] (untouched within the kept group)",
					id, blocks, wantBlocks)
			}
		}

		assertTailPairSafe(t, msgs)
	})
}

// --- D-14 golden battery (PAR-05, 21-03 Task 3) ---

// goldenThinkingCase is one committed wire-pair record from
// testdata/thinking-golden/sse-thinking.jsonl.
type goldenThinkingCase struct {
	Name            string            `json:"name"`
	Frames          []json.RawMessage `json:"frames"`
	ExpectBlocks    []goldenBlockView `json:"expectBlocks"`
	ToolCall        *goldenToolCall   `json:"toolCall"`
	ToolResult      string            `json:"toolResult"`
	AssistantText   string            `json:"assistantText"`
	BoundaryMidTurn bool              `json:"boundaryMidTurn"`
}

// goldenBlockView is the D-14 oracle: the provider-side field VALUES each hop
// must reproduce identically.
type goldenBlockView struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
	Data      string `json:"data"`
}

type goldenToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// loadGoldenThinking parses the committed fixture; records without a name
// (the provenance header) are skipped.
func loadGoldenThinking(t *testing.T) []goldenThinkingCase {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", "thinking-golden", "sse-thinking.jsonl"))
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}

	var cases []goldenThinkingCase

	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var c goldenThinkingCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("parse golden record: %v", err)
		}

		if c.Name != "" {
			cases = append(cases, c)
		}
	}

	if len(cases) == 0 {
		t.Fatal("golden fixture carries no named cases")
	}

	return cases
}

// goldenView extracts the field values of one assembled block payload (the
// battery's comparison instrument — production never unmarshals for storage).
func goldenView(t *testing.T, raw json.RawMessage) goldenBlockView {
	t.Helper()

	var v goldenBlockView
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal block payload %s: %v", raw, err)
	}

	return v
}

func (v goldenBlockView) String() string {
	return fmt.Sprintf("{type:%s thinking:%q signature:%q data:%q}", v.Type, v.Thinking, v.Signature, v.Data)
}

// TestThinkingGolden replays the committed wire pairs through the REAL chain
// — scripted SSE → drainSSE → StreamChunk → AppendRawThinking → projector →
// shaper → toMessageParams — and asserts FIELD-VALUE identity at every hop
// (D-14: the fixture's provider values == the chunk's Raw values == the
// transcript's verbatim bytes == the projector's extraction == the outgoing
// SDK param values), for both block types plus the boundary-adjacent case.
// The replay pin (D-13's data path) rides hop 2: appended-then-re-read bytes
// are identical.
func TestThinkingGolden(t *testing.T) {
	for _, c := range loadGoldenThinking(t) {
		t.Run(c.Name, func(t *testing.T) { runGoldenThinkingCase(t, c) })
	}
}

// runGoldenThinkingCase drives one wire-pair record through the five hops.
//
//nolint:funlen,gocognit // one labeled block per hop is the point
func runGoldenThinkingCase(t *testing.T, c goldenThinkingCase) {
	t.Helper()

	// Hop 1: scripted SSE → drainSSE → thinking StreamChunks.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")

		flusher, _ := w.(http.Flusher)
		for _, f := range c.Frames {
			fmt.Fprintf(w, "data: %s\n\n", f)

			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	prof := profile.Profile{Name: fixtureProfileName, Model: fixtureModelSlug, MaxTokens: 64}

	ap := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	ch, err := ap.Stream(context.Background(), &prof, []provider.Message{{Role: roleUserMsg, Content: "go"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var rawBlocks []json.RawMessage

	deadline := time.After(5 * time.Second)

	for open := true; open; {
		select {
		case chunk, ok := <-ch:
			if !ok {
				open = false

				break
			}

			if chunk.Type == chunkTypeThinking {
				rawBlocks = append(rawBlocks, chunk.Raw)
			}
		case <-deadline:
			t.Fatal("golden stream did not close within 5s")
		}
	}

	if len(rawBlocks) != len(c.ExpectBlocks) {
		t.Fatalf("hop 1: thinking chunks = %d; want %d (fixture blocks)", len(rawBlocks), len(c.ExpectBlocks))
	}

	for i, want := range c.ExpectBlocks {
		if got := goldenView(t, rawBlocks[i]); got != want {
			t.Errorf("hop 1 (SSE→StreamChunk) block %d = %s; want %s", i, got, want)
		}
	}

	// Hop 2: AppendRawThinking → transcript; the replay pin re-reads the bytes.
	turnID := "tg-" + c.Name

	m := newTestManager(t, "s-golden")

	if err := m.AppendUserMessage(turnID, []ContentBlock{{Type: blockText, Text: "go"}}); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}

	for _, raw := range rawBlocks {
		if err := m.AppendRawThinking(turnID, fixtureModelSlug, raw); err != nil {
			t.Fatalf("AppendRawThinking: %v", err)
		}
	}

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var stored []json.RawMessage

	for i := range lines {
		if lines[i].Type == TypeRawThinking {
			stored = append(stored, lines[i].Content)
		}
	}

	if len(stored) != len(rawBlocks) {
		t.Fatalf("hop 2: stored raw_thinking lines = %d; want %d", len(stored), len(rawBlocks))
	}

	for i := range rawBlocks {
		if !bytes.Equal(rawBlocks[i], stored[i]) {
			t.Errorf("hop 2 replay pin: re-read bytes differ from appended bytes at block %d:\n append: %s\nre-read: %s",
				i, rawBlocks[i], stored[i])
		}

		if got := goldenView(t, stored[i]); got != c.ExpectBlocks[i] {
			t.Errorf("hop 2 (transcript) block %d = %s; want %s", i, got, c.ExpectBlocks[i])
		}
	}

	// Complete the turn unit per the record: tool round (+ optional mid-turn
	// boundary) and/or the final assistant text.
	if c.ToolCall != nil {
		if err := m.AppendToolCall(turnID, c.ToolCall.ID, c.ToolCall.Name, c.ToolCall.Input); err != nil {
			t.Fatalf("AppendToolCall: %v", err)
		}

		if err := m.AppendToolResult(turnID, c.ToolCall.ID, json.RawMessage(c.ToolResult), false); err != nil {
			t.Fatalf("AppendToolResult: %v", err)
		}

		if c.BoundaryMidTurn {
			if err := m.AppendBoundary(mutatingCommandBash, c.ToolCall.ID, turnID); err != nil {
				t.Fatalf("AppendBoundary: %v", err)
			}
		}
	}

	if c.AssistantText != "" {
		if err := m.AppendAssistantMessage(turnID, c.AssistantText); err != nil {
			t.Fatalf("AppendAssistantMessage: %v", err)
		}
	}

	// Hop 3: projector — the ONLY field-extraction site on the replay path.
	msgs, err := NewProjector(&profile.Profile{Name: fixtureProfileName}, m).Project(turnID)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var projected []provider.ThinkingBlock

	for i := range msgs {
		if msgs[i].Role == roleAssistant {
			projected = append(projected, msgs[i].ThinkingBlocks...)
		}
	}

	if len(projected) != len(c.ExpectBlocks) {
		t.Fatalf("hop 3: projected thinking blocks = %d; want %d (window: %s)",
			len(projected), len(c.ExpectBlocks), msgSummaryList(msgs))
	}

	for i, want := range c.ExpectBlocks {
		got := projected[i]

		switch want.Type {
		case chunkTypeThinking:
			if got.Type != want.Type || got.Text != want.Thinking || got.Signature != want.Signature {
				t.Errorf("hop 3 (projector) block %d = %+v; want {type:%s text:%q sig:%q}",
					i, got, want.Type, want.Thinking, want.Signature)
			}
		case blockRedactedThinking:
			if got.Type != want.Type || got.Data != want.Data {
				t.Errorf("hop 3 (projector) block %d = %+v; want {type:%s data:%q}", i, got, want.Type, want.Data)
			}
		}
	}

	// Hop 4: shaper → outgoing SDK params (the SDK re-serializes; VALUES must
	// survive untouched).
	shaped, _, err := shaper.New().Shape(&prof, msgs)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}

	var (
		gotSig   []string
		gotThink []string
		gotData  []string
	)

	for _, mp := range shaped.Messages {
		if mp.Role != anthropic.MessageParamRoleAssistant {
			continue
		}

		for _, b := range mp.Content {
			switch {
			case b.OfThinking != nil:
				gotSig = append(gotSig, b.OfThinking.Signature)
				gotThink = append(gotThink, b.OfThinking.Thinking)
			case b.OfRedactedThinking != nil:
				gotData = append(gotData, b.OfRedactedThinking.Data)
			}
		}
	}

	var wantSig, wantThink, wantData []string

	for _, want := range c.ExpectBlocks {
		switch want.Type {
		case chunkTypeThinking:
			wantSig = append(wantSig, want.Signature)
			wantThink = append(wantThink, want.Thinking)
		case blockRedactedThinking:
			wantData = append(wantData, want.Data)
		}
	}

	if !slices.Equal(gotSig, wantSig) || !slices.Equal(gotThink, wantThink) || !slices.Equal(gotData, wantData) {
		t.Errorf("hop 4 (outgoing params) field values differ:\n sig: got %v want %v\n think: got %v want %v\n data: got %v want %v",
			gotSig, wantSig, gotThink, wantThink, gotData, wantData)
	}
}

// --- G-19-1 same-turn carve-out battery (19-06 Task 1) ---

// Same-turn carve-out fixture summaries (unique markers per case, the 19-03
// battery's discipline).
const (
	sumSameTurnNew  = "SUMMARY-STC-NEW: mid-turn overflow absorbed"
	sumSameTurnOld  = "SUMMARY-STC-OLD: first mid-turn marker"
	sumPreUserStc   = "SUMMARY-STC-PRE: marker before the turn"
	intentStcParent = "please refactor everything"
	intentStcSub    = "subagent task prompt"
)

// stcAppendExchanges appends n answered tool exchanges to the given turn.
func stcAppendExchanges(t *testing.T, m *Manager, turnID string, n int) {
	t.Helper()

	for i := range n {
		id := fmt.Sprintf("%s_ex%02d", turnID, i)
		mustAppend(t, m.AppendToolCall(turnID, id, toolRead,
			json.RawMessage(`{"file_path":"/w/a.go"}`)), "AppendToolCall")
		mustAppend(t, m.AppendToolResult(turnID, id, json.RawMessage(`{"o":"src"}`), false),
			"AppendToolResult")
	}
}

// TestProjector_SameTurnCarveOut pins the G-19-1 engine-armed carve-out: a
// marker whose TurnID equals the projected turn reshapes that turn's
// projection ONLY when the engine armed the per-turn override
// (SetRetryCompactedTurn) AND no pre-user marker exists. The same transcript
// through a plain projector is byte-identical to the marker-free transcript
// (kill-9 replay + tamper safety), the pre-user marker keeps 19-03's winning
// scan, a subagent prompt never shadows the turn's own intent, the most
// recent same-turn marker wins, a subagent turn is never reshaped by a parent
// marker (Pitfall 5), and the projection stays a pure function of (lines,
// turnID, override) — deterministic across re-projections.
func TestProjector_SameTurnCarveOut(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen,maintidx // battery
	t.Parallel()

	const (
		turnT  = "turnT"
		turnSA = "turnSA"
		turnP1 = "turnP1"
	)

	t.Run("armed + matching marker projects post-marker: summary seed + own intent, empty tail", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-stc1")
		p := NewProjector(fakeProfile("sys"), m)

		mustAppend(t, m.AppendUserMessage(turnT,
			[]ContentBlock{{Type: blockText, Text: intentStcParent}}), "AppendUserMessage")
		stcAppendExchanges(t, m, turnT, 2)
		mustAppend(t,
			m.AppendCompaction(turnT, "line:5", "line:6", sumSameTurnNew, 9000, 900, 900), "AppendCompaction")

		p.SetRetryCompactedTurn(turnT)

		msgs, err := p.Project(turnT)
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		// The marker is the LAST line: the post-marker tail is empty, so the
		// armed projection is the seed ALONE.
		if len(msgs) != 1 {
			t.Fatalf("len(msgs) = %d; want 1 (seed only — the tail is empty immediately after the marker):\n%s",
				len(msgs), msgSummaryList(msgs))
		}

		seed := msgs[0]
		if seed.Role != roleUserMsg {
			t.Fatalf("seed role = %q; want user (the single-user-message lean-seed shape)", seed.Role)
		}

		if !strings.Contains(seed.Content, sumSameTurnNew) {
			t.Errorf("armed seed missing the same-turn marker's summary:\n%s", seed.Content)
		}

		if !strings.Contains(seed.Content, intentStcParent) {
			t.Errorf("armed seed missing the turn's OWN current intent:\n%s", seed.Content)
		}

		// The seed is the marker summary + the intent alone — the mechanical
		// vocabulary must not leak (the seedContent wrapper, summary-sourced).
		for _, leak := range []string{"last_user=", "last_assistant=", "files_touched="} {
			if strings.Contains(seed.Content, leak) {
				t.Errorf("armed seed carries the mechanical summary vocabulary %q:\n%s", leak, seed.Content)
			}
		}
	})

	t.Run("not armed: marker-present projects byte-identically to marker-free", func(t *testing.T) {
		t.Parallel()

		// The marker-bearing transcript.
		marked := newTestManager(t, "s-stc2a")
		mustAppend(t, marked.AppendUserMessage(turnT,
			[]ContentBlock{{Type: blockText, Text: intentStcParent}}), "AppendUserMessage")
		stcAppendExchanges(t, marked, turnT, 2)
		mustAppend(t,
			marked.AppendCompaction(turnT, "line:5", "line:6", sumSameTurnNew, 9000, 900, 900), "AppendCompaction")

		projMarked, err := NewProjector(fakeProfile("sys"), marked).Project(turnT)
		if err != nil {
			t.Fatalf("Project(marker-present): %v", err)
		}

		// The same lines MINUS the marker line.
		free := newTestManager(t, "s-stc2b")
		mustAppend(t, free.AppendUserMessage(turnT,
			[]ContentBlock{{Type: blockText, Text: intentStcParent}}), "AppendUserMessage")
		stcAppendExchanges(t, free, turnT, 2)

		projFree, err := NewProjector(fakeProfile("sys"), free).Project(turnT)
		if err != nil {
			t.Fatalf("Project(marker-free): %v", err)
		}

		// The marker ALONE changes nothing without the in-memory override —
		// kill-9 replay and crafted transcripts stay deterministic.
		if !reflect.DeepEqual(projMarked, projFree) {
			t.Errorf("un-armed projection over a same-turn marker drifted from the marker-free transcript:\n marked: %s\n   free: %s",
				msgSummaryList(projMarked), msgSummaryList(projFree))
		}
	})

	t.Run("pre-user marker wins; the armed override cannot displace 19-03's scan", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-stc3")
		p := NewProjector(fakeProfile("sys"), m)

		mustAppend(t, m.AppendUserMessage("turn_0", []ContentBlock{{Type: blockText, Text: "early work"}}),
			"AppendUserMessage")
		mustAppend(t,
			m.AppendCompaction("turn_0", "line:1", "line:2", sumPreUserStc, 100, 10, 10), "AppendCompaction")
		mustAppend(t, m.AppendUserMessage(turnT,
			[]ContentBlock{{Type: blockText, Text: intentStcParent}}), "AppendUserMessage")
		stcAppendExchanges(t, m, turnT, 1)
		mustAppend(t,
			m.AppendCompaction(turnT, "line:4", "line:5", sumSameTurnNew, 9000, 900, 900), "AppendCompaction")

		p.SetRetryCompactedTurn(turnT)

		msgs, err := p.Project(turnT)
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		seed := msgs[0]
		if !strings.Contains(seed.Content, sumPreUserStc) {
			t.Errorf("pre-user marker's summary lost the seed (19-03's winning scan displaced):\n%s", seed.Content)
		}

		if strings.Contains(seed.Content, sumSameTurnNew) {
			t.Errorf("the same-turn marker DISPLACED the pre-user marker (carve-out must run only when compactionMarkerIdx found nothing):\n%s",
				seed.Content)
		}

		if !strings.Contains(seed.Content, intentStcParent) {
			t.Errorf("seed missing the current intent:\n%s", seed.Content)
		}
	})

	t.Run("subagent shadow: the turn's own user message stays the intent", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-stc4")
		p := NewProjector(fakeProfile("sys"), m)

		mustAppend(t, m.AppendUserMessage(turnT,
			[]ContentBlock{{Type: blockText, Text: intentStcParent}}), "AppendUserMessage")
		// The subagent prompt lands BETWEEN T's user message and the marker —
		// it must not hijack the seed's current intent.
		mustAppend(t, m.AppendUserMessage(turnSA,
			[]ContentBlock{{Type: blockText, Text: intentStcSub}}), "AppendUserMessage")
		stcAppendExchanges(t, m, turnSA, 1)
		mustAppend(t,
			m.AppendCompaction(turnT, "line:4", "line:5", sumSameTurnNew, 9000, 900, 900), "AppendCompaction")

		p.SetRetryCompactedTurn(turnT)

		msgs, err := p.Project(turnT)
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		seed := msgs[0]
		if !strings.Contains(seed.Content, sumSameTurnNew) {
			t.Fatalf("armed seed missing the same-turn marker's summary:\n%s", seed.Content)
		}

		if !strings.Contains(seed.Content, intentStcParent) {
			t.Errorf("seed intent is not the projected turn's OWN user message (subagent shadow):\n%s", seed.Content)
		}

		if strings.Contains(seed.Content, intentStcSub) {
			t.Errorf("the subagent prompt hijacked the seed's current intent:\n%s", seed.Content)
		}
	})

	t.Run("most recent same-turn marker wins on a twice-armed turn", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-stc5")
		p := NewProjector(fakeProfile("sys"), m)

		mustAppend(t, m.AppendUserMessage(turnT,
			[]ContentBlock{{Type: blockText, Text: intentStcParent}}), "AppendUserMessage")
		stcAppendExchanges(t, m, turnT, 1)
		mustAppend(t,
			m.AppendCompaction(turnT, "line:3", "line:4", sumSameTurnOld, 8000, 800, 800), "AppendCompaction")
		stcAppendExchanges(t, m, turnT, 1)
		mustAppend(t,
			m.AppendCompaction(turnT, "line:6", "line:7", sumSameTurnNew, 9000, 900, 900), "AppendCompaction")

		p.SetRetryCompactedTurn(turnT)

		msgs, err := p.Project(turnT)
		if err != nil {
			t.Fatalf("Project: %v", err)
		}

		seed := msgs[0]
		if !strings.Contains(seed.Content, sumSameTurnNew) {
			t.Errorf("seed does not use the MOST RECENT same-turn marker:\n%s", seed.Content)
		}

		if strings.Contains(seed.Content, sumSameTurnOld) {
			t.Errorf("an EARLIER same-turn marker survived a newer one:\n%s", seed.Content)
		}
	})

	t.Run("subagent turn safe: a parent marker never reshapes it, armed or not", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-stc6")

		mustAppend(t, m.AppendUserMessage(turnP1,
			[]ContentBlock{{Type: blockText, Text: "parent directs"}}), "AppendUserMessage")
		mustAppend(t, m.AppendUserMessage(turnSA,
			[]ContentBlock{{Type: blockText, Text: intentStcSub}}), "AppendUserMessage")
		stcAppendExchanges(t, m, turnSA, 1)
		// Mid-subagent compaction attributed to the PARENT turn (Pitfall 5).
		mustAppend(t,
			m.AppendCompaction(turnP1, "line:4", "line:5", sumSameTurnNew, 9000, 900, 900), "AppendCompaction")

		projPlain, err := NewProjector(fakeProfile("sys"), m).Project(turnSA)
		if err != nil {
			t.Fatalf("Project(plain): %v", err)
		}

		// Armed FOR the subagent turn: no marker carries TurnID == turnSA, so
		// sameTurnMarkerIdx finds nothing and the projection is unchanged.
		armedSA := NewProjector(fakeProfile("sys"), m)
		armedSA.SetRetryCompactedTurn(turnSA)

		projArmedSA, err := armedSA.Project(turnSA)
		if err != nil {
			t.Fatalf("Project(armed-for-subagent): %v", err)
		}

		if !reflect.DeepEqual(projPlain, projArmedSA) {
			t.Errorf("armed-for-subagent projection drifted:\n plain: %s\n armed: %s",
				msgSummaryList(projPlain), msgSummaryList(projArmedSA))
		}

		// Armed FOR THE PARENT while projecting the subagent: the override
		// matches a different turn than the one projected — the TurnID key
		// never reaches the carve-out.
		armedParent := NewProjector(fakeProfile("sys"), m)
		armedParent.SetRetryCompactedTurn(turnP1)

		projArmedParent, err := armedParent.Project(turnSA)
		if err != nil {
			t.Fatalf("Project(armed-for-parent): %v", err)
		}

		if !reflect.DeepEqual(projPlain, projArmedParent) {
			t.Errorf("parent-armed projection of the subagent turn drifted:\n plain: %s\n armed: %s",
				msgSummaryList(projPlain), msgSummaryList(projArmedParent))
		}
	})

	t.Run("deterministic across re-projections, with a post-marker tail", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-stc7")
		p := NewProjector(fakeProfile("sys"), m)

		mustAppend(t, m.AppendUserMessage(turnT,
			[]ContentBlock{{Type: blockText, Text: intentStcParent}}), "AppendUserMessage")
		stcAppendExchanges(t, m, turnT, 1)
		mustAppend(t,
			m.AppendCompaction(turnT, "line:3", "line:4", sumSameTurnNew, 9000, 900, 900), "AppendCompaction")
		// A post-marker exchange: the budget-fill tail is non-empty (folded
		// post-marker span; the zero budget degrades to the 64-message count).
		stcAppendExchanges(t, m, turnT, 1)

		p.SetRetryCompactedTurn(turnT)

		first, err := p.Project(turnT)
		if err != nil {
			t.Fatalf("Project(1): %v", err)
		}

		second, err := p.Project(turnT)
		if err != nil {
			t.Fatalf("Project(2): %v", err)
		}

		if !reflect.DeepEqual(first, second) {
			t.Errorf("armed projection is not deterministic:\n 1: %s\n 2: %s",
				msgSummaryList(first), msgSummaryList(second))
		}

		if len(first) != 3 {
			t.Fatalf("len(first) = %d; want 3 (seed + the post-marker exchange's batch + result):\n%s",
				len(first), msgSummaryList(first))
		}

		if !strings.Contains(first[0].Content, sumSameTurnNew) || !strings.Contains(first[0].Content, intentStcParent) {
			t.Errorf("seed lost the summary or the intent:\n%s", first[0].Content)
		}

		if !projectedHasCall(first, "turnT_ex00") {
			t.Errorf("post-marker exchange missing from the budget-fill tail:\n%s", msgSummaryList(first))
		}
	})
}
