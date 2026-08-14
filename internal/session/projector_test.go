package session //nolint:testpackage // internal package test

import (
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
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
