package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// Test-local tool names + final text (goconst: values intentionally distinct
// from the chunk-type constants that share some literals).
const (
	wiringToolBash  = "Bash"
	wiringToolTodo  = "TodoWrite"
	wiringFinalText = "finished"
)

// TestCoreExec_BashThroughSession (08-08 T2 Test 10, catalog path): through the
// real sessionFor wiring + RealExecutor + DispatchBatch, a fake-provider turn
// whose tool_call is Bash `echo hi` records a transcript tool_result whose
// PROJECTED Content (T1's plainContent rule) is the plain text `hi`. Pre-fix
// RED: the result is the `{"error":"tool Bash has no implementation yet …"}`
// structured wall — the exact string the 08-07 E2E observed 88×.
func TestCoreExec_BashThroughSession(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen // flat battery
	t.Parallel()

	r, _ := newExpansionRunner(t, true,
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: "call_bash_1", Name: wiringToolBash,
			Input: json.RawMessage(`{"command":"echo hi","description":"say hi"}`),
		}}},
		scriptedResp{text: wiringFinalText},
	)

	blocksRun := []acp.ContentBlock{{Type: blockText, Text: "run echo"}}

	_, err := r.Run(context.Background(), "sess-coreexec-1", &noopEmitter{}, blocksRun)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}

	sess := r.sessions["sess-coreexec-1"]

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var (
		turnID string
		rawOut json.RawMessage
		isErr  bool
		found  bool
	)

	for i := range lines {
		if lines[i].Type == session.TypeToolResult && lines[i].Name == "" {
			// tool_result lines carry no Name; the paired tool_call names Bash.
			// Find the preceding Bash tool_call with the same id.
			for j := i - 1; j >= 0; j-- {
				if lines[j].Type == session.TypeToolCall && lines[j].ToolCallID == lines[i].ToolCallID {
					if lines[j].Name != wiringToolBash {
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

	err = json.Unmarshal(rawOut, &decoded)
	if err != nil {
		t.Fatalf("Bash Output is not a JSON string: %v (%s)", err, rawOut)
	}

	if decoded != "hi" {
		t.Errorf("Bash Output = %q; want the plain captured form %q", decoded, "hi")
	}

	// D-11 boundary discipline, re-pinned 08-09 (BOTH halves): the writer half
	// — Bash is mutating, so a context boundary line with the recorded cause +
	// toolCallID is appended after the result (SESS-02/03, unchanged); the
	// reader half — the boundary does NOT reset the PRODUCING turn's mid-turn
	// window (between-turn reset, SESS-04 as revised 08-09): the projection
	// CARRIES the Bash tool message with the plain captured form `hi`, so the
	// next iteration of the SAME turn sees its own prior exchange (the
	// convergence fix for the 08-08 T4 maxIterations finding).
	sawBoundary := false

	for i := range lines {
		if lines[i].Type == session.TypeBoundary && lines[i].Cause == "mutating-command:"+wiringToolBash {
			sawBoundary = true
		}
	}

	if !sawBoundary {
		t.Fatal("no mutating-command:Bash boundary line in the transcript (SESS-02/03 writer half violated)")
	}

	msgs, err := sess.Projector.Project(turnID)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	carried := false

	for i := range msgs {
		if msgs[i].Role == "tool" && msgs[i].ToolCallID == "call_bash_1" {
			carried = true

			if msgs[i].Content != "hi" {
				t.Errorf("carried Bash tool content = %q; want the plain captured form %q", msgs[i].Content, "hi")
			}
		}
	}

	if !carried {
		t.Errorf("projection carries no Bash tool message past the boundary (between-turn reset re-scope violated):\n%v", msgs)
	}

	// The turn still completes: the final assistant text lands in the window.
	foundText := false

	for i := range msgs {
		if msgs[i].Role == "assistant" && msgs[i].Content == wiringFinalText {
			foundText = true
		}
	}

	if !foundText {
		t.Errorf("projection missing the final assistant text:\n%v", msgs)
	}

	// The BETWEEN-turn reset half: after a NEXT turn's user message arrives,
	// that turn's projection is the lean seed (no tool/assistant messages) —
	// the mid-turn boundary recorded above is the next turn's reset boundary.
	const nextTurn = "sess-coreexec-1-next"

	err = sess.Manager.AppendUserMessage(nextTurn, []session.ContentBlock{{Type: blockText, Text: "and then"}})
	if err != nil {
		t.Fatalf("AppendUserMessage (next turn): %v", err)
	}

	nextMsgs, err := sess.Projector.Project(nextTurn)
	if err != nil {
		t.Fatalf("Project (next turn): %v", err)
	}

	if len(nextMsgs) > 3 {
		t.Errorf("next-turn projection has %d messages; want the lean seed (<=3)", len(nextMsgs))
	}

	for i := range nextMsgs {
		if nextMsgs[i].Role == "tool" || nextMsgs[i].Role == "assistant" {
			t.Errorf("next-turn projection[%d] = %s; want NO tool/assistant messages (between-turn reset violated)",
				i, msgSummaryACP(&nextMsgs[i]))
		}
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
			{ID: "call_b1", Name: wiringToolBash, Input: json.RawMessage(
				`{"command":` + jsonString(t, markerCmd("A")) + `}`)},
			{ID: "call_b2", Name: wiringToolBash, Input: json.RawMessage(
				`{"command":` + jsonString(t, markerCmd("B")) + `}`)},
		}},
		scriptedResp{text: wiringFinalText},
	)

	blocksBoth := []acp.ContentBlock{{Type: blockText, Text: "run both"}}

	_, err := r.Run(context.Background(), "sess-coreexec-2", &noopEmitter{}, blocksBoth)
	if err != nil {
		t.Fatalf("Run err: %v", err)
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

// TestCoreExec_FileTodoMixedBatch (08-08 T3 Test 9, integration): through the
// session wiring, a fake-provider turn mixing Read+TodoWrite+Write+Edit (one
// batch) returns all four results in ARRIVAL ORDER with the mutating
// Write/Edit alone-in-slot (their boundaries carry mutating-command:<tool>;
// the read-only pair carries none).
func TestCoreExec_FileTodoMixedBatch(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen // flat battery
	t.Parallel()

	r, prov := newExpansionRunner(t, true)

	target := filepath.Join(r.workDir, "target.txt")

	err := os.WriteFile(target, []byte("alpha beta\n"), 0o600)
	if err != nil {
		t.Fatalf("seed target: %v", err)
	}

	newFile := filepath.Join(r.workDir, "created.txt")

	prov.queue(
		scriptedResp{toolCalls: []provider.ToolCall{
			{ID: "c_read", Name: tracerReadTool, Input: json.RawMessage(`{"file_path":` + jsonString(t, target) + `}`)},
			{ID: "c_todo", Name: wiringToolTodo, Input: json.RawMessage(
				`{"todos":[{"content":"do it","status":"in_progress","priority":"high"}]}`)},
			{ID: "c_write", Name: "Write", Input: json.RawMessage(
				`{"file_path":` + jsonString(t, newFile) + `,"content":"body\n"}`)},
			{ID: "c_edit", Name: "Edit", Input: json.RawMessage(
				`{"file_path":` + jsonString(t, target) + `,"old_string":"beta","new_string":"BETA"}`)},
		}},
		scriptedResp{text: wiringFinalText},
	)

	blocksMix := []acp.ContentBlock{{Type: blockText, Text: "mix them"}}

	_, err = r.Run(context.Background(), "sess-coreexec-3", &noopEmitter{}, blocksMix)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}

	sess := r.sessions["sess-coreexec-3"]

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	// Tool results in ARRIVAL ORDER with the captured forms.
	var order []string

	outputs := map[string]string{}

	for i := range lines {
		if lines[i].Type != session.TypeToolResult {
			continue
		}

		for j := i - 1; j >= 0; j-- {
			if lines[j].Type == session.TypeToolCall && lines[j].ToolCallID == lines[i].ToolCallID {
				order = append(order, lines[j].Name)

				var s string
				if json.Unmarshal(lines[i].Output, &s) == nil {
					outputs[lines[j].Name] = s
				} else {
					outputs[lines[j].Name] = string(lines[i].Output)
				}

				break
			}
		}
	}

	wantOrder := []string{tracerReadTool, wiringToolTodo, "Write", "Edit"}

	if len(order) != len(wantOrder) {
		t.Fatalf("tool_result order = %v; want %v", order, wantOrder)
	}

	for i, want := range wantOrder {
		if order[i] != want {
			t.Errorf("order[%d] = %s; want %s (arrival order)", i, order[i], want)
		}
	}

	if got := outputs[tracerReadTool]; got != "1\talpha beta" {
		t.Errorf("Read output = %q; want the line-numbered captured form", got)
	}

	if !strings.Contains(outputs["Write"], "File created successfully at: "+newFile) {
		t.Errorf("Write output = %q; want the captured created form", outputs["Write"])
	}

	if !strings.Contains(outputs["Edit"], "has been updated successfully.") {
		t.Errorf("Edit output = %q; want the captured updated form", outputs["Edit"])
	}

	if !strings.Contains(outputs[wiringToolTodo], `"inProgress":1`) {
		t.Errorf("TodoWrite output = %q; want the camelCase echo", outputs[wiringToolTodo])
	}

	// Effects landed.
	body, rerr := os.ReadFile(newFile)
	if rerr != nil || string(body) != "body\n" {
		t.Errorf("created file = %q (%v)", body, rerr)
	}

	if body, _ := os.ReadFile(target); string(body) != "alpha BETA\n" {
		t.Errorf("edited file = %q; want the edit applied", body)
	}

	// Mutating alone-in-slot: Write's and Edit's boundaries carry the
	// mutating-command cause; the read-only pair opened none.
	causes := map[string]bool{}

	for i := range lines {
		if lines[i].Type == session.TypeBoundary {
			causes[lines[i].Cause] = true
		}
	}

	if !causes["mutating-command:Write"] || !causes["mutating-command:Edit"] {
		t.Errorf("boundary causes = %v; want mutating-command:Write and :Edit", causes)
	}
}

// TestCoreExec_ReadOnlyProjection (08-08 T3 Test 9, projection half): a
// read-only batch (Read + TodoWrite — neither opens a boundary) carries into
// the NEXT iteration's projection with the captured forms via T1's
// plainContent rule: the Read result as UNQUOTED line-numbered text, the
// TodoWrite echo as its verbatim JSON (wire-equivalent to the captured plain
// text). The mutating-tool projection is boundary-reset by D-11 and is
// asserted in TestCoreExec_BashThroughSession.
func TestCoreExec_ReadOnlyProjection(t *testing.T) { //nolint:cyclop,funlen // flat battery
	t.Parallel()

	r, prov := newExpansionRunner(t, true)

	target := filepath.Join(r.workDir, "proj.txt")

	err := os.WriteFile(target, []byte("one\ntwo\n"), 0o600)
	if err != nil {
		t.Fatalf("seed target: %v", err)
	}

	prov.queue(
		scriptedResp{toolCalls: []provider.ToolCall{
			{
				ID: "c_pread", Name: tracerReadTool,
				Input: json.RawMessage(`{"file_path":` + jsonString(t, target) + `}`)},
			{ID: "c_ptodo", Name: wiringToolTodo, Input: json.RawMessage(
				`{"todos":[{"content":"step","status":"pending","priority":"medium"}]}`)},
		}},
		scriptedResp{text: wiringFinalText},
	)

	blocksProj := []acp.ContentBlock{{Type: blockText, Text: "read then list"}}

	_, err = r.Run(context.Background(), "sess-coreexec-4", &noopEmitter{}, blocksProj)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}

	sess := r.sessions["sess-coreexec-4"]

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	turnID := ""

	for i := range lines {
		if lines[i].Type == session.TypeToolResult {
			turnID = lines[i].TurnID

			break
		}
	}

	if turnID == "" {
		t.Fatal("no tool result recorded")
	}

	msgs, err := sess.Projector.Project(turnID)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var readContent, todoContent string

	readSeen, todoSeen := false, false

	for i := range msgs {
		if msgs[i].Role != "tool" {
			continue
		}

		switch msgs[i].ToolName {
		case tracerReadTool:
			readContent, readSeen = msgs[i].Content, true
		case wiringToolTodo:
			todoContent, todoSeen = msgs[i].Content, true
		}
	}

	if !readSeen || !todoSeen {
		t.Fatalf("projection missing the read-only exchanges (read=%v todo=%v):\n%v", readSeen, todoSeen, msgs)
	}

	if readContent != "1\tone\n2\ttwo" {
		t.Errorf("projected Read content = %q; want the UNQUOTED line-numbered captured form", readContent)
	}

	if !strings.Contains(todoContent, `"inProgress":0`) || !strings.HasPrefix(todoContent, "{") {
		t.Errorf("projected TodoWrite content = %q; want the verbatim echo JSON", todoContent)
	}
}

// msgSummaryACP renders a provider.Message compactly for failure messages.
func msgSummaryACP(m *provider.Message) string {
	return fmt.Sprintf("%s{content:%q isError:%v toolCallID:%s}", m.Role, m.Content, m.IsError, m.ToolCallID)
}
