package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// TestCoreExec_BashThroughSession (08-08 T2 Test 10, catalog path): through the
// real sessionFor wiring + RealExecutor + DispatchBatch, a fake-provider turn
// whose tool_call is Bash `echo hi` records a transcript tool_result whose
// PROJECTED Content (T1's plainContent rule) is the plain text `hi`. Pre-fix
// RED: the result is the `{"error":"tool Bash has no implementation yet …"}`
// structured wall — the exact string the 08-07 E2E observed 88×.
func TestCoreExec_BashThroughSession(t *testing.T) {
	t.Parallel()

	r, _ := newExpansionRunner(t, true,
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: "call_bash_1", Name: "Bash",
			Input: json.RawMessage(`{"command":"echo hi","description":"say hi"}`),
		}}},
		scriptedResp{text: "done"},
	)

	if _, err := r.Run(context.Background(), "sess-coreexec-1", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "run echo"}}); err != nil {
		t.Fatalf("Run err = %v", err)
	}

	sess := r.sessions["sess-coreexec-1"]
	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var (
		turnID   string
		rawOut   json.RawMessage
		isErr    bool
		found    bool
	)

	for i := range lines {
		if lines[i].Type == session.TypeToolResult && lines[i].Name == "" {
			// tool_result lines carry no Name; the paired tool_call names Bash.
			// Find the preceding Bash tool_call with the same id.
			for j := i - 1; j >= 0; j-- {
				if lines[j].Type == session.TypeToolCall && lines[j].ToolCallID == lines[i].ToolCallID {
					if lines[j].Name != "Bash" {
						break
					}

					turnID, rawOut, isErr, found = lines[i].TurnID, lines[i].Output, lines[i].IsError, true

					break
				}
			}
		}
	}

	if !found {
		t.Fatal("no Bash tool_result in the transcript")
	}

	if isErr {
		t.Errorf("echo hi should not be an error result (IsError=true, output=%s)", rawOut)
	}

	var decoded string
	if err := json.Unmarshal(rawOut, &decoded); err != nil {
		t.Fatalf("Bash Output is not a JSON string: %v (%s)", err, rawOut)
	}

	if decoded != "hi" {
		t.Errorf("Bash Output = %q; want the plain captured form %q", decoded, "hi")
	}

	// The PROJECTED content (T1's plainContent rule) renders unquoted.
	msgs, err := sess.Projector.Project(turnID)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var toolContent string

	projected := false

	for i := range msgs {
		if msgs[i].Role == "tool" {
			toolContent, projected = msgs[i].Content, true
		}
	}

	if !projected {
		t.Fatal("no tool message in the projection")
	}

	if toolContent != "hi" {
		t.Errorf("projected Bash content = %q; want %q (plain, unquoted)", toolContent, "hi")
	}
}

// TestCoreExec_BashBatchSerializes (08-08 T2 Test 10, second half): a batch of
// TWO Bash calls (both mutating) executes strictly one-at-a-time, never
// overlapping (D-21 preserved) — each command appends start/end markers to a
// log; serialized execution shows A-start,A-end,B-start,B-end with no
// interleaving.
func TestCoreExec_BashBatchSerializes(t *testing.T) {
	t.Parallel()

	markerCmd := func(tag string) string {
		return "sh -c 'echo " + tag + "-start >> batch.log; sleep 0.2; echo " + tag + "-end >> batch.log'"
	}

	r, _ := newExpansionRunner(t, true,
		scriptedResp{toolCalls: []provider.ToolCall{
			{ID: "call_b1", Name: "Bash", Input: json.RawMessage(
				`{"command":` + jsonString(t, markerCmd("A")) + `}`)},
			{ID: "call_b2", Name: "Bash", Input: json.RawMessage(
				`{"command":` + jsonString(t, markerCmd("B")) + `}`)},
		}},
		scriptedResp{text: "done"},
	)

	if _, err := r.Run(context.Background(), "sess-coreexec-2", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "run both"}}); err != nil {
		t.Fatalf("Run err = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(r.workDir, "batch.log"))
	if err != nil {
		t.Fatalf("read batch.log: %v (commands did not run)", err)
	}

	got := strings.TrimSpace(string(raw))
	want := "A-start\nA-end\nB-start\nB-end"
	if got != want {
		t.Errorf("batch.log = %q; want %q (mutating calls serialized, never overlapping)", got, want)
	}
}

// jsonString marshals s as a JSON string literal (for embedding in raw inputs).
func jsonString(t *testing.T, s string) string {
	t.Helper()

	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal %q: %v", s, err)
	}

	return string(b)
}
