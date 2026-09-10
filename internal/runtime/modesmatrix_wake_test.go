package runtime //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/modesmatrix"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/kit/provider"
)

// The ECOS-04 (TAIL-03, 24-05) wake-mode cells — REAL drivers since 24-06
// (G-24-1). Premise correction: the pre-24-06 file skipped all three cells
// claiming Phase 22 had not executed. That premise was FALSE — the wake
// machinery (22-CONTEXT D-01) shipped in 43a8378 on 2026-09-08 and was
// gap-closed by 5ad02a0/a59baf9 on 2026-09-10, all ancestors of Phase 24's
// first commit 5552942 (see 24-VERIFICATION.md gap 1). The substrate is live
// and green at HEAD, so the cells now drive it for real.
//
// Every cell drives the REAL chain end to end: a client r.Run turn whose
// scripted provider response dispatches a background subagent (a Task tool
// call with run_in_background — the backgroundLaunch seam) → the nested loop
// completes → Tracker.Complete (KindSubagent) → the SetDrain callback →
// scheduleWakeDrain → wakeDrainChain → drainWakeNotifications → runOneTurn.
// The assertions are the TestWakeTurn_BackgroundBashCompletion lens set over
// that chain: exactly ONE wake turn, one EngineDecision line with the
// wake:tasks provenance, the first user message stays the typed prompt.
//
// Assertion-shape decision (24-06 Task 1, applied to the live substrate —
// never assert anything the machinery does not exhibit):
//
//   - commands/skills: the wake input is MACHINE-composed (renderWakeBlocks
//     emits notification-only blocks) and slash expansion is first-text-block
//     start-anchored (expandUserBlocks + the invocation regex), so a typed
//     invocation STRUCTURALLY cannot expand inside a wake turn. The cells
//     assert the strongest TRUE functional outcome over the wake
//     composition: the invocation rides the wake-triggering dispatch (the
//     nested subagent turn's user message carries it verbatim — the pinned
//     subagent-mode carry, expansion being parent-side by design), the real
//     chain fires exactly one wake turn naming the completing dispatch, and
//     the woken model's captured request carries the notification.
//   - hooks: the wake turn's scripted Read tool call consults the gate head
//     and the fixture's PreToolUse hook fires INSIDE the wake turn (the
//     marker file carries the stdin JSON with tool_name Read). SubagentStop
//     is NOT asserted: it fires only inside the foreground DispatchSubagent
//     wrapper (internal/session/subagent.go:150); the background leg calls
//     the nested Runner.Run directly (runtime.go runSubagentWithProgress)
//     and never enters that wrapper — a deliberate descope recorded in
//     24-06-SUMMARY (firing it on the background leg would be a production
//     change, i.e. a NEW gap, not this closure). Instead the cell proves the
//     completing dispatch by its own exhibited observable: the wake turn's
//     notification block carries kind subagent plus the dispatch's minted
//     task id (the id RunBackgroundSubagent returned via
//     BackgroundDispatchResult.TaskID, observed in the client turn's
//     async_launched tool-result payload).
//
// Determinism: HOME pinned (no operator hooks), transcript-as-truth +
// provider-capture lenses, bounded polls (5s deadlines, 20ms intervals).
// The registry row prints alongside the automation row's cells (this test
// binary's combined report on the shared cronRegistry).

// wakeDispatchTaskInput marshals the Task tool-call input that dispatches
// the completing background subagent (the backgroundLaunch seam's entry).
func wakeDispatchTaskInput(t *testing.T, prompt string) json.RawMessage {
	t.Helper()

	in, err := json.Marshal(map[string]any{"prompt": prompt, "run_in_background": true})
	if err != nil {
		t.Fatalf("marshal Task input: %v", err)
	}

	return in
}

// wakeDispatchedTaskID scans the transcript's tool results for the
// async_launched payload the completing background dispatch returned and
// yields the task id the launcher minted (BackgroundDispatchResult.TaskID —
// RunBackgroundSubagent's crypto/rand exec_ id, the same id the wake turn's
// notification block must name).
func wakeDispatchedTaskID(t *testing.T, sess *session.Session) string {
	t.Helper()

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	for i := range lines {
		if lines[i].Type != session.TypeToolResult {
			continue
		}

		var payload struct {
			Status string `json:"status"`
			TaskID string `json:"task_id"`
		}

		if json.Unmarshal(lines[i].Output, &payload) == nil &&
			payload.Status == "async_launched" && payload.TaskID != "" {
			return payload.TaskID
		}
	}

	return ""
}

// wakeMatrixDrive runs the wake-triggering composition: the client turn over
// the given script (whose first response dispatches the completing background
// subagent), then extracts the dispatch's minted task id from the transcript.
func wakeMatrixDrive(
	t *testing.T, project, sessID, typedPrompt string, script ...scriptedResp,
) (*Runner, *scriptedACPProvider, string, *session.Session) {
	t.Helper()

	r, prov := newMatrixRunner(t, project, script...)

	blocks := []acp.ContentBlock{{Type: blockText, Text: typedPrompt}}
	if _, err := r.Run(context.Background(), sessID, &noopEmitter{}, blocks); err != nil {
		t.Fatalf("Run err: %v", err)
	}

	sess := r.sessions[sessID]

	taskID := wakeDispatchedTaskID(t, sess)
	if taskID == "" {
		t.Fatal("no async_launched payload in the transcript tool results — the background dispatch never launched")
	}

	return r, prov, taskID, sess
}

// wakeMatrixWaitForTurn polls (bounded) for the wake turn the completing
// dispatch schedules and returns its user text ("" on deadline — the caller
// fails on the missing wake).
func wakeMatrixWaitForTurn(t *testing.T, sess *session.Session, taskID string) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		for _, txt := range wakeUserTexts(t, sess) {
			if strings.Contains(txt, "task-notification") &&
				strings.Contains(txt, "kind: subagent") &&
				strings.Contains(txt, "task_id: "+taskID) {
				return txt
			}
		}

		time.Sleep(20 * time.Millisecond)
	}

	return ""
}

// assertWakeTurnFiredOnce pins the exactly-one-wake-turn property over the
// REAL chain: ONE transcript user message carries the task-notification
// block naming the completing dispatch (kind subagent + its minted id),
// exactly one EngineDecision line carries the wake provenance, and the first
// user message stays the typed client prompt (the wake appends, never
// replaces).
func assertWakeTurnFiredOnce(t *testing.T, sess *session.Session, taskID, typedPrompt string) {
	t.Helper()

	wakeTurns := 0

	lastWake := ""

	for _, txt := range wakeUserTexts(t, sess) {
		if strings.Contains(txt, "task-notification") {
			wakeTurns++
			lastWake = txt
		}
	}

	if wakeTurns != 1 {
		t.Errorf("wake turns = %d; want exactly 1 (one coalesced batch)", wakeTurns)
	}

	if wakeTurns == 1 &&
		(!strings.Contains(lastWake, "kind: subagent") || !strings.Contains(lastWake, "task_id: "+taskID)) {
		t.Errorf("the wake notification block must name the completing dispatch (kind subagent, task id %s); got %.160s",
			taskID, lastWake)
	}

	// The EngineDecision line lands AFTER the turn's user message (the drain
	// appends it post-runOneTurn) — bounded poll, never a single read (the
	// write-order race a one-shot count would flap on).
	if !waitFor(5*time.Second, func() bool {
		return wakeEngineDecisions(t, sess, wakeProvenance) == 1
	}) {
		t.Errorf("EngineDecision lines with wake provenance = %d; want 1", wakeEngineDecisions(t, sess, wakeProvenance))
	}

	if first := wakeUserTexts(t, sess)[0]; !strings.Contains(first, typedPrompt) {
		t.Errorf("first user message = %q; want the typed client prompt", first)
	}
}

// wakeMatrixNestedCarried reports whether any user message in the transcript
// carries needle verbatim (the nested subagent turn's message — the pinned
// subagent-mode carry lens).
func wakeMatrixNestedCarried(t *testing.T, sess *session.Session, needle string) bool {
	t.Helper()

	for _, txt := range wakeUserTexts(t, sess) {
		if strings.Contains(txt, needle) {
			return true
		}
	}

	return false
}

// TestModesMatrixWakeCommands (wake x commands): a completing background
// subagent dispatched with the fixture command invocation as its prompt
// wakes the model through the real chain — the invocation rides the
// wake-triggering dispatch verbatim (the nested turn's user message), the
// woken model's captured request carries the notification.
func TestModesMatrixWakeCommands(t *testing.T) { // HOME-pinned leg
	t.Setenv("HOME", t.TempDir())

	project := t.TempDir()

	modesmatrix.MountFixture(t, project)

	const invocation = "/matrix-echo wake-cmd-args"
	const typed = "run a background matrix subagent for the wake commands cell"

	_, prov, taskID, sess := wakeMatrixDrive(t, project, "sess-matrix-wake-c", typed,
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: "call_wake_bg_c", Name: "Task", Input: wakeDispatchTaskInput(t, invocation),
		}}},
		scriptedResp{text: "nested subagent turn done"},
		scriptedResp{text: "client turn done after the background dispatch"},
		scriptedResp{text: "wake acknowledged"},
	)

	if wakeMatrixWaitForTurn(t, sess, taskID) == "" {
		t.Fatal("the completing background subagent never woke the model (no notification block naming the dispatch)")
	}

	assertWakeTurnFiredOnce(t, sess, taskID, typed)

	if !wakeMatrixNestedCarried(t, sess, invocation) {
		t.Errorf("the nested subagent turn never carried the invocation %q verbatim", invocation)
	}

	if !provSawText(prov, "task-notification") {
		t.Error("no captured provider request carried the task notification (the woken model never saw it)")
	}

	cronRegistry.Record(
		modesmatrix.ModeWake, modesmatrix.SurfaceCommands, modesmatrix.StatusPass,
		"completing background subagent (Task run_in_background) woke exactly one real turn; "+
			"invocation rode the dispatch verbatim; woken model saw the notification",
	)

	cronRegistry.ReportT(t)
}

// TestModesMatrixWakeSkills (wake x skills): the same composition over the
// fixture skill invocation as the dispatch prompt.
func TestModesMatrixWakeSkills(t *testing.T) { // HOME-pinned leg
	t.Setenv("HOME", t.TempDir())

	project := t.TempDir()

	modesmatrix.MountFixture(t, project)

	const invocation = "/matrix-skill wake-skill-args"
	const typed = "run a background matrix subagent for the wake skills cell"

	_, prov, taskID, sess := wakeMatrixDrive(t, project, "sess-matrix-wake-s", typed,
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: "call_wake_bg_s", Name: "Task", Input: wakeDispatchTaskInput(t, invocation),
		}}},
		scriptedResp{text: "nested subagent turn done"},
		scriptedResp{text: "client turn done after the background dispatch"},
		scriptedResp{text: "wake acknowledged"},
	)

	if wakeMatrixWaitForTurn(t, sess, taskID) == "" {
		t.Fatal("the completing background subagent never woke the model (no notification block naming the dispatch)")
	}

	assertWakeTurnFiredOnce(t, sess, taskID, typed)

	if !wakeMatrixNestedCarried(t, sess, invocation) {
		t.Errorf("the nested subagent turn never carried the invocation %q verbatim", invocation)
	}

	if !provSawText(prov, "task-notification") {
		t.Error("no captured provider request carried the task notification (the woken model never saw it)")
	}

	cronRegistry.Record(
		modesmatrix.ModeWake, modesmatrix.SurfaceSkills, modesmatrix.StatusPass,
		"same real-chain composition over /matrix-skill; invocation rode the dispatch verbatim; "+
			"woken model saw the notification",
	)

	cronRegistry.ReportT(t)
}

// TestModesMatrixWakeHooks (wake x hooks): the wake turn's scripted Read tool
// call consults the gate head, which executes the fixture's PreToolUse hook
// INSIDE the wake turn — the marker file inside the temp project receives
// the hook's stdin JSON (tool_name Read). The completing dispatch is proven
// by the wake notification's kind/task-id linkage (see the file comment for
// the SubagentStop descope).
func TestModesMatrixWakeHooks(t *testing.T) { //nolint:funlen // driver + both observable sets // HOME-pinned leg
	t.Setenv("HOME", t.TempDir())

	project := t.TempDir()

	modesmatrix.MountFixture(t, project)

	notes := filepath.Join(project, "notes.md")
	if err := os.WriteFile(notes, []byte("the wake turn reads this\n"), 0o600); err != nil {
		t.Fatalf("write notes.md: %v", err)
	}

	readInput, merr := json.Marshal(map[string]string{"file_path": notes})
	if merr != nil {
		t.Fatalf("marshal read input: %v", merr)
	}

	const typed = "run a background matrix subagent for the wake hooks cell"

	_, _, taskID, sess := wakeMatrixDrive(t, project, "sess-matrix-wake-h", typed,
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: "call_wake_bg_h", Name: "Task",
			Input: wakeDispatchTaskInput(t, "background subagent for the wake hooks cell"),
		}}},
		scriptedResp{text: "nested subagent turn done"},
		scriptedResp{text: "client turn done after the background dispatch"},
		scriptedResp{
			toolCalls: []provider.ToolCall{{Name: toolNameRead, Input: readInput}},
			finish:    chunkToolUse,
		},
		scriptedResp{text: "wake hooks turn done"},
	)

	if wakeMatrixWaitForTurn(t, sess, taskID) == "" {
		t.Fatal("the completing background subagent never woke the model (no notification block naming the dispatch)")
	}

	assertWakeTurnFiredOnce(t, sess, taskID, typed)

	// The PreToolUse firing INSIDE the wake turn (bounded poll — the wake
	// turn's Read executes after the notification lands).
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) && len(matrixMarkerEvents(t, project, "PreToolUse")) == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	pre := matrixMarkerEvents(t, project, "PreToolUse")
	if len(pre) == 0 {
		t.Fatal("no PreToolUse payload in the marker file — the fixture hook never fired inside the wake turn")
	}

	if got := pre[0]["tool_name"]; got != toolNameRead {
		t.Errorf("PreToolUse tool_name = %v; want %s", got, toolNameRead)
	}

	cronRegistry.Record(
		modesmatrix.ModeWake, modesmatrix.SurfaceHooks, modesmatrix.StatusPass,
		"wake turn's Read consulted the gate head (PreToolUse fired inside the wake turn); "+
			"notification names the completing dispatch (kind subagent + minted task id)",
	)

	cronRegistry.ReportT(t)
}
