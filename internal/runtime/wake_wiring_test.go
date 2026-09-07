package runtime //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// The wake-turn wiring battery (22-01, D-01/D-02/D-03): a background Bash
// completion reaches the ONE task-notification subsystem and WAKES the model
// through the existing automation-turn rails — exactly one wake turn per
// drain batch, kind-detected payload fields, wake provenance on the
// EngineDecision line, and a busy client turn never interrupted (the pending
// notifications coalesce and deliver after the turn).

// wakeMarker is the background command's output marker the notification tail
// must carry end-to-end.
const wakeMarker = "wake-marker-7f3a"

// wakeUserTexts assembles every user_message line's block texts (the
// assertion lens for turn inputs — the wake block list lands here).
func wakeUserTexts(t *testing.T, sess *session.Session) []string {
	t.Helper()

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var out []string

	for i := range lines {
		if lines[i].Type != session.TypeUserMessage {
			continue
		}

		var blocks []session.ContentBlock

		if uerr := json.Unmarshal(lines[i].Content, &blocks); uerr != nil {
			continue // non-block content (tolerant — other lines may differ)
		}

		var sb strings.Builder

		for _, b := range blocks {
			sb.WriteString(b.Text)
		}

		out = append(out, sb.String())
	}

	return out
}

// wakeEngineDecisions returns the EngineDecision lines whose signal/text
// mention the given provenance string.
func wakeEngineDecisions(t *testing.T, sess *session.Session, provenance string) int {
	t.Helper()

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	n := 0

	for i := range lines {
		if lines[i].Type != session.TypeEngineDecision {
			continue
		}

		if strings.Contains(string(lines[i].Input), provenance) || strings.Contains(lines[i].Text, provenance) {
			n++
		}
	}

	return n
}

// waitFor polls cond until true or the deadline passes (returns the final
// cond value — the caller reports which assertion failed).
func waitFor(deadline time.Duration, cond func() bool) bool {
	end := time.Now().Add(deadline)

	for time.Now().Before(end) {
		if cond() {
			return true
		}

		time.Sleep(20 * time.Millisecond)
	}

	return cond()
}

// TestWakeTurn_BackgroundBashCompletion (22-01 Task 1 tracer): a client turn
// starts a run_in_background Bash task; after the child exits 0 exactly ONE
// additional model turn fires whose input block lists the D-02 fields (exec_
// id, kind bash, exit status 0, .ass-guard/outputs/ pointer, tail with the
// marker), an EngineDecision line carries the wake provenance, and the
// foreground scripted exchanges around it are unchanged.
func TestWakeTurn_BackgroundBashCompletion(t *testing.T) { //nolint:funlen,cyclop // flat tracer battery
	t.Parallel()

	r, prov := newExpansionRunner(t, true,
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID:   "call_bg_1",
			Name: "Bash",
			Input: json.RawMessage(
				`{"command":"echo ` + wakeMarker + `; exit 0","run_in_background":true}`),
		}}},
		scriptedResp{text: "background task launched"},
		scriptedResp{text: "wake acknowledged"},
	)

	// The CLIENT turn: starts the background task and returns immediately.
	_, err := r.Run(context.Background(), "sess-wake-1", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "run a background task"}})
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}

	sess := r.sessions["sess-wake-1"]

	// (1) The dispatch returned immediately with the background-start prose.
	var toolResults strings.Builder

	lines, lerr := sess.Manager.ReadAll()
	if lerr != nil {
		t.Fatalf("ReadAll: %v", lerr)
	}

	for i := range lines {
		if lines[i].Type == session.TypeToolResult {
			toolResults.Write(lines[i].Output)
		}
	}

	if !strings.Contains(toolResults.String(), "Command running in background with ID: exec_") {
		t.Error("no background-start prose form in the transcript tool results")
	}

	// (2) After the child exits: exactly ONE wake turn with the D-02 fields.
	wakeOK := waitFor(5*time.Second, func() bool {
		for _, txt := range wakeUserTexts(t, sess) {
			if strings.Contains(txt, "task-notification") &&
				strings.Contains(txt, "kind: bash") &&
				strings.Contains(txt, "exec_") &&
				strings.Contains(txt, ".ass-guard/outputs/") &&
				strings.Contains(txt, wakeMarker) {
				return true
			}
		}

		return false
	})

	if !wakeOK {
		t.Error("no wake-turn user block carrying the D-02 notification fields")
	}

	// Exactly ONE wake turn — poll stable (no second batch may fire later).
	time.Sleep(150 * time.Millisecond)

	wakeTurns := 0

	for _, txt := range wakeUserTexts(t, sess) {
		if strings.Contains(txt, "task-notification") {
			wakeTurns++
		}
	}

	if wakeTurns != 1 {
		t.Errorf("wake turns = %d; want exactly 1 (one coalesced batch)", wakeTurns)
	}

	// (3) The wake turn's EngineDecision line carries the wake provenance.
	if got := wakeEngineDecisions(t, sess, "wake:tasks"); got != 1 {
		t.Errorf("EngineDecision lines with wake provenance = %d; want 1", got)
	}

	// (4) The foreground scripted turns are unchanged: 2 client-turn calls +
	// exactly 1 wake call — no storm, no duplicates.
	if calls := prov.callCount(); calls != 3 {
		t.Errorf("provider calls = %d; want 3 (client tool turn + client final + one wake)", calls)
	}

	// The FIRST user message stays the typed prompt (the wake turn appends,
	// never replaces).
	if first := wakeUserTexts(t, sess)[0]; !strings.Contains(first, "run a background task") {
		t.Errorf("first user message = %q; want the typed client prompt", first)
	}
}
