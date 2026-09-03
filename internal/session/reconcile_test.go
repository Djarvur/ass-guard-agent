package session //nolint:testpackage // internal package test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Reconciliation inventory tests (18-02, ACP-06 / 18-CONTEXT D-01/D-02): one
// hand-written fixture per Live-State Inventory row (18-RESEARCH), asserting
// the EXACT synthetic closure set + state seed a kill -9 mid-turn transcript
// must reconcile to. The engine under test is PURE — Reconcile takes the
// reader-filtered []Line and performs no I/O (D-01: transcript-as-truth, zero
// sidecar state); closures are APPENDED on disk by the load path (18-05)
// through Manager.AppendSynthetic, provenance-marked with Cause
// InterruptedCause (D-02: subtle in replay, unambiguous on disk).
//
// Fixture discipline: every fixture line parses as JSON (loadReconcileFixture
// fails the test otherwise) and uses the exact on-disk camelCase spellings of
// the Line envelope (transcript.go). Rows 6/7/8 append NOTHING by themselves —
// they are state seeds (plan mode, turn counter) or an audit-only pair
// (request_shaped without usage: usage counters re-derive from complete usage
// lines only, so no closure is synthesized); row 9 (missing session_end)
// co-fires in every kill -9 fixture by construction — a killed session has no
// session_end, so its closure appears in each fixture's exact set.
//
// class10 (pending permission asks, post-17) needs NO fixture: it is class-3
// machinery applied to permission asks — an unresolved ask_suspended closes as
// the cancelled-normal tool_result for its toolCallID (18-RESEARCH Open
// Question Q2's resolution), and in-memory-only ask registries are empty after
// process death by construction (no parked ask can fire post-resume).

// Shared fixture consts (goconst-clean across the package test files).
const (
	reconcileFixtureSession = "sess_recon"
	reconcileCall1          = "call_1"
	reconcileCall2          = "call_2"
)

// reconcileFixtureNames returns the full inventory (one file per row 1-9).
func reconcileFixtureNames() []string {
	return []string{
		"class01_dangling_tool_call.jsonl",
		"class02_turn_without_terminal.jsonl",
		"class03_ask_suspended_unresolved.jsonl",
		"class04_subagent_unresolved.jsonl",
		"class05_open_chunk_stream.jsonl",
		"class06_plan_mode_state.jsonl",
		"class07_turn_sequence.jsonl",
		"class08_request_without_usage.jsonl",
		"class09_missing_session_end.jsonl",
	}
}

// loadReconcileFixture reads one fixture and enforces the discipline: EVERY
// line must parse as JSON (a malformed fixture is a test bug, never a skip).
func loadReconcileFixture(t *testing.T, name string) []Line {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", "reconcile", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	segs := splitFixtureLines(string(raw))
	out := make([]Line, 0, len(segs))

	for i, s := range segs {
		var l Line

		jerr := json.Unmarshal([]byte(s), &l)
		if jerr != nil {
			t.Fatalf("fixture %s line %d is not valid JSON: %v (line=%q)", name, i+1, jerr, s)
		}

		out = append(out, l)
	}

	return out
}

// splitFixtureLines splits the raw fixture body into non-empty lines.
func splitFixtureLines(raw string) []string {
	out := make([]string, 0, len(raw)/128+1)

	start := 0

	for i := range len(raw) {
		if raw[i] != '\n' {
			continue
		}

		if seg := raw[start:i]; seg != "" {
			out = append(out, seg)
		}

		start = i + 1
	}

	if seg := raw[start:]; seg != "" {
		out = append(out, seg)
	}

	return out
}

// reconcileTurnID builds the fixture session's turn id for n (the exact
// %03d shape Session.nextTurnID mints).
func reconcileTurnID(n int) string {
	return fmt.Sprintf("%s-turn-%03d", reconcileFixtureSession, n)
}

// assertReconcileClosure pins one closure's contract: kind, keying ids, error
// flag, D-02 provenance, a non-zero timestamp, and kind-appropriate
// engine-authored payload presence (payload text itself is engine-owned,
// never transcript-copied — T-18-05).
func assertReconcileClosure(t *testing.T, idx int, got, want *Line) {
	t.Helper()

	if got.Type != want.Type {
		t.Errorf("closure[%d] type = %q; want %q", idx, got.Type, want.Type)
	}

	if got.TurnID != want.TurnID {
		t.Errorf("closure[%d] (%s) turnID = %q; want %q", idx, got.Type, got.TurnID, want.TurnID)
	}

	if got.ToolCallID != want.ToolCallID {
		t.Errorf("closure[%d] (%s) toolCallID = %q; want %q", idx, got.Type, got.ToolCallID, want.ToolCallID)
	}

	if got.ParentTurnID != want.ParentTurnID {
		t.Errorf("closure[%d] (%s) parentTurnID = %q; want %q", idx, got.Type, got.ParentTurnID, want.ParentTurnID)
	}

	if got.SubagentTurnID != want.SubagentTurnID {
		t.Errorf("closure[%d] (%s) subagentTurnID = %q; want %q",
			idx, got.Type, got.SubagentTurnID, want.SubagentTurnID)
	}

	if got.IsError != want.IsError {
		t.Errorf("closure[%d] (%s) isError = %v; want %v", idx, got.Type, got.IsError, want.IsError)
	}

	if got.Cause != want.Cause {
		t.Errorf("closure[%d] (%s) cause = %q; want %q (D-02 provenance)", idx, got.Type, got.Cause, want.Cause)
	}

	if got.Timestamp.IsZero() {
		t.Errorf("closure[%d] (%s) has no timestamp", idx, got.Type)
	}

	assertClosurePayload(t, idx, got, want.Type)
}

// assertClosurePayload checks kind-specific payload presence for one closure.
func assertClosurePayload(t *testing.T, idx int, got *Line, wantType string) {
	t.Helper()

	switch wantType {
	case TypeToolResult:
		if len(got.Output) == 0 {
			t.Errorf("closure[%d] (tool_result) has no output payload", idx)
		}
	case TypeSubagentResult:
		if got.Message == "" {
			t.Errorf("closure[%d] (subagent_result) has no failure message", idx)
		}
	case TypeCanceled, TypeAssistantMessage:
		if got.Text == "" {
			t.Errorf("closure[%d] (%s) has no text payload", idx, wantType)
		}
	case TypeSessionEnd:
		// no payload kind
	default:
		t.Errorf("closure[%d] unexpected closure kind %q", idx, wantType)
	}
}

// TestReconcile is the inventory table: one subtest per fixture
// class01..class09, asserting the exact ordered closure set and the seed
// values.
//
//nolint:funlen // a table over the whole inventory (one entry per row)
func TestReconcile(t *testing.T) {
	t.Parallel()

	t1 := reconcileTurnID(1)
	t2 := reconcileTurnID(2)

	tests := []struct {
		name     string
		fixture  string
		want     []Line
		wantSeed Seed
	}{
		{
			// Row 1 + co-firing rows 2/9: the dangling tool_call gets its
			// failed tool_result; the unterminated turn gets its canceled
			// terminal; the killed session gets its synthetic session_end.
			// Closures order by their dangling opener's transcript index
			// (the user_message opens the turn before the tool_call opens the pair).
			name:    "class01_dangling_tool_call",
			fixture: "class01_dangling_tool_call.jsonl",
			want: []Line{
				{Type: TypeCanceled, TurnID: t1, Cause: InterruptedCause},
				{
					Type: TypeToolResult, TurnID: t1, ToolCallID: reconcileCall1,
					IsError: true, Cause: InterruptedCause,
				},
				{Type: TypeSessionEnd, Cause: InterruptedCause},
			},
			wantSeed: Seed{MaxTurns: 1},
		},
		{
			// Row 2 in isolation (plus row 9): a user_message turn with no
			// terminal line reconciles to a synthetic canceled terminal.
			name:    "class02_turn_without_terminal",
			fixture: "class02_turn_without_terminal.jsonl",
			want: []Line{
				{Type: TypeCanceled, TurnID: t1, Cause: InterruptedCause},
				{Type: TypeSessionEnd, Cause: InterruptedCause},
			},
			wantSeed: Seed{MaxTurns: 1},
		},
		{
			// Row 3 + row 9 ONLY: the unresolved ask_suspended closes as the
			// cancelled-NORMAL tool_result for its toolCallID (isError=false,
			// the permissionCancelledForm family) — and ask_suspended IS the
			// turn's terminal (the turn ended at the ask, 12-01), so NO
			// canceled closure fires for it.
			name:    "class03_ask_suspended_unresolved",
			fixture: "class03_ask_suspended_unresolved.jsonl",
			want: []Line{
				{Type: TypeToolResult, TurnID: t1, ToolCallID: reconcileCall1, IsError: false, Cause: InterruptedCause},
				{Type: TypeSessionEnd, Cause: InterruptedCause},
			},
			wantSeed: Seed{MaxTurns: 1},
		},
		{
			// Row 4 + rows 2/9: the dangling subagent_dispatch closes as a
			// failed subagent_result keyed by subagentTurnID (with its
			// parentTurnID carried from the dispatch); the parent turn gets
			// its canceled terminal. The subagent turn itself opened no
			// user_message (killed before the nested loop's first append), so
			// no row-2 closure for it.
			name:    "class04_subagent_unresolved",
			fixture: "class04_subagent_unresolved.jsonl",
			want: []Line{
				{Type: TypeCanceled, TurnID: t1, Cause: InterruptedCause},
				{
					Type: TypeSubagentResult, TurnID: t2, ParentTurnID: t1, SubagentTurnID: t2,
					Cause: InterruptedCause,
				},
				{Type: TypeSessionEnd, Cause: InterruptedCause},
			},
			wantSeed: Seed{MaxTurns: 2}, // the dispatch's subagentTurnID continues the same sequence
		},
		{
			// Row 5 + rows 2/9: the open agent_message_chunk stream closes
			// with a terminal assistant_message; the turn still reconciles to
			// canceled (it never completed).
			name:    "class05_open_chunk_stream",
			fixture: "class05_open_chunk_stream.jsonl",
			want: []Line{
				{Type: TypeCanceled, TurnID: t1, Cause: InterruptedCause},
				{Type: TypeAssistantMessage, TurnID: t1, Cause: InterruptedCause},
				{Type: TypeSessionEnd, Cause: InterruptedCause},
			},
			wantSeed: Seed{MaxTurns: 1},
		},
		{
			// Row 6 (state seed, appends NOTHING for its own machinery): the
			// LAST plan_mode line's cause is the persisted plan-mode target
			// (Pitfall 3). The turn itself has no terminal (killed right
			// after the enter marker) and the session has no session_end.
			name:    "class06_plan_mode_state",
			fixture: "class06_plan_mode_state.jsonl",
			want: []Line{
				{Type: TypeCanceled, TurnID: t1, Cause: InterruptedCause},
				{Type: TypeSessionEnd, Cause: InterruptedCause},
			},
			wantSeed: Seed{MaxTurns: 1, PlanMode: PlanModeCauseEnter, PlanModePresent: true},
		},
		{
			// Row 7 (state seed): MaxTurns is a MAX scan, not a count — turn
			// 003 is absent (died before its first line) yet the seed is 4.
			name:    "class07_turn_sequence",
			fixture: "class07_turn_sequence.jsonl",
			want: []Line{
				{Type: TypeSessionEnd, Cause: InterruptedCause},
			},
			wantSeed: Seed{MaxTurns: 4},
		},
		{
			// Row 8 (audit-only pair, appends NOTHING): request_shaped
			// without usage synthesizes NO usage closure — usage/cost counters
			// re-derive from complete usage lines only. The turn itself was
			// killed mid-request (no chunks, no assistant), so rows 2/9 fire.
			name:    "class08_request_without_usage",
			fixture: "class08_request_without_usage.jsonl",
			want: []Line{
				{Type: TypeCanceled, TurnID: t1, Cause: InterruptedCause},
				{Type: TypeSessionEnd, Cause: InterruptedCause},
			},
			wantSeed: Seed{MaxTurns: 1},
		},
		{
			// Row 9 in isolation: an otherwise cleanly-closed conversation
			// with NO session_end reconciles to exactly the synthetic
			// session_end.
			name:    "class09_missing_session_end",
			fixture: "class09_missing_session_end.jsonl",
			want: []Line{
				{Type: TypeSessionEnd, Cause: InterruptedCause},
			},
			wantSeed: Seed{MaxTurns: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lines := loadReconcileFixture(t, tt.fixture)

			got, seed := Reconcile(reconcileFixtureSession, lines)

			if len(got) != len(tt.want) {
				t.Fatalf("Reconcile returned %d closures; want %d: %+v", len(got), len(tt.want), got)
			}

			for i := range tt.want {
				assertReconcileClosure(t, i, &got[i], &tt.want[i])
			}

			if seed != tt.wantSeed {
				t.Errorf("seed = %+v; want %+v", seed, tt.wantSeed)
			}
		})
	}
}

// TestReconcileIdempotent pins the kill-during-replay row: feeding a
// transcript PLUS its own closures back through Reconcile classifies as CLEAN
// — zero closures, unchanged seed (second loads never duplicate closures).
func TestReconcileIdempotent(t *testing.T) {
	t.Parallel()

	for _, name := range reconcileFixtureNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			lines := loadReconcileFixture(t, name)

			first, seed1 := Reconcile(reconcileFixtureSession, lines)

			replayed := make([]Line, 0, len(lines)+len(first))
			replayed = append(replayed, lines...)
			replayed = append(replayed, first...)

			second, seed2 := Reconcile(reconcileFixtureSession, replayed)

			if len(second) != 0 {
				t.Fatalf("second pass returned %d closures; want 0: %+v", len(second), second)
			}

			if seed1 != seed2 {
				t.Errorf("seed drifted on second pass: %+v -> %+v", seed1, seed2)
			}
		})
	}
}

// reconcileUserLine builds one user_message line for the inline clean fixtures.
func reconcileUserLine(turn, text string, ts time.Time) Line {
	return Line{
		Type: TypeUserMessage, TurnID: turn, Timestamp: ts,
		Content: json.RawMessage(`[{"type":"text","text":"` + text + `"}]`),
	}
}

// reconcileCleanMultiTurn is the inline cleanly-closed multi-turn fixture: a
// completed turn, a completed subagent turn (dispatch → its own user_message
// + chunks + result → the parent's assistant terminal), and a live-cancelled
// turn whose chunks stay UNCLOSED by design (the live cancel contract,
// WR-04 — a canceled turn's chunks are not a dangling expectation).
func reconcileCleanMultiTurn(ts time.Time) []Line {
	t1 := reconcileTurnID(1)
	t2 := reconcileTurnID(2)
	t3 := reconcileTurnID(3)
	t4 := reconcileTurnID(4)

	return []Line{
		{Type: TypeSessionStart, Timestamp: ts, Text: reconcileFixtureSession},
		reconcileUserLine(t1, "first turn", ts),
		{Type: TypeAssistantMessage, TurnID: t1, Timestamp: ts, Text: "first done"},
		reconcileUserLine(t2, "research the layout", ts),
		{
			Type: TypeSubagentDispatch, TurnID: t3, Timestamp: ts,
			ParentTurnID: t2, SubagentTurnID: t3, ToolCallID: reconcileCall1,
		},
		reconcileUserLine(t3, "subagent task", ts),
		{
			Type: TypeAgentMessageChunk, TurnID: t3, Timestamp: ts,
			MessageID: t3, Text: "partial subagent text",
		},
		{
			Type: TypeSubagentResult, TurnID: t3, Timestamp: ts,
			ParentTurnID: t2, SubagentTurnID: t3, Result: "layout mapped",
		},
		{Type: TypeAssistantMessage, TurnID: t2, Timestamp: ts, Text: "research done"},
		reconcileUserLine(t4, "a turn the user cancels mid-stream", ts),
		{Type: TypeAgentMessageChunk, TurnID: t4, Timestamp: ts, MessageID: t4, Text: "partial"},
		{
			Type: TypeCanceled, TurnID: t4, Timestamp: ts,
			Text: "user cancelled via session/cancel",
		},
		{Type: TypeSessionEnd, Timestamp: ts},
	}
}

// reconcileCleanPlanMode is the inline cleanly-closed plan-mode fixture: an
// enter followed by an approved exit — the LAST plan_mode line must seed the
// persisted target (Pitfall 3).
func reconcileCleanPlanMode(ts time.Time) []Line {
	t1 := reconcileTurnID(1)
	t2 := reconcileTurnID(2)

	return []Line{
		{Type: TypeSessionStart, Timestamp: ts, Text: reconcileFixtureSession},
		reconcileUserLine(t1, "plan the migration", ts),
		{
			Type: TypeToolCall, TurnID: t1, Timestamp: ts,
			ToolCallID: reconcileCall1, Name: toolNameEnterPlanMode, Input: json.RawMessage(`{}`),
		},
		{
			Type: TypeToolResult, TurnID: t1, Timestamp: ts,
			ToolCallID: reconcileCall1, Output: json.RawMessage(`"entered"`),
		},
		{Type: TypePlanMode, TurnID: t1, Timestamp: ts, Cause: PlanModeCauseEnter, ToolCallID: reconcileCall1},
		{Type: TypeAssistantMessage, TurnID: t1, Timestamp: ts, Text: "planning"},
		reconcileUserLine(t2, "approve it", ts),
		{
			Type: TypeToolCall, TurnID: t2, Timestamp: ts,
			ToolCallID: reconcileCall2, Name: toolNameExitPlanMode, Input: json.RawMessage(`{}`),
		},
		{
			Type: TypeToolResult, TurnID: t2, Timestamp: ts,
			ToolCallID: reconcileCall2, Output: json.RawMessage(`"approved"`),
		},
		{Type: TypePlanMode, TurnID: t2, Timestamp: ts, Cause: PlanModeCauseExit, ToolCallID: reconcileCall2},
		{Type: TypeAssistantMessage, TurnID: t2, Timestamp: ts, Text: "executing"},
		{Type: TypeSessionEnd, Timestamp: ts},
	}
}

// TestReconcileClean pins the zero-closure path over inline cleanly-closed
// transcripts (the nine on-disk fixtures are all kill -9 transcripts by
// design, so the clean shapes live here).
func TestReconcileClean(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)

	t.Run("multi_turn_with_cancel_and_subagent", func(t *testing.T) {
		t.Parallel()

		got, seed := Reconcile(reconcileFixtureSession, reconcileCleanMultiTurn(ts))

		if len(got) != 0 {
			t.Fatalf("clean transcript returned %d closures; want 0: %+v", len(got), got)
		}

		want := Seed{MaxTurns: 4}
		if seed != want {
			t.Errorf("seed = %+v; want %+v (no plan_mode line -> PlanModePresent false)", seed, want)
		}
	})

	t.Run("plan_mode_seeded_from_last_line", func(t *testing.T) {
		t.Parallel()

		got, seed := Reconcile(reconcileFixtureSession, reconcileCleanPlanMode(ts))

		if len(got) != 0 {
			t.Fatalf("clean plan-mode transcript returned %d closures; want 0: %+v", len(got), got)
		}

		want := Seed{MaxTurns: 2, PlanMode: PlanModeCauseExit, PlanModePresent: true}
		if seed != want {
			t.Errorf("seed = %+v; want %+v (the LAST plan_mode line wins, Pitfall 3)", seed, want)
		}
	})
}

// TestReconcileSkipsGarbageTail documents the reader contract at the
// reconciliation boundary: the transcript readers skip non-conforming lines
// (readTranscriptFile; 16-D-20 tolerance), so a torn final line (an OS-crash /
// power-loss artifact — kill -9 alone cannot tear an O_APPEND write) never
// reaches the engine. Classification runs over the conforming prefix and is
// identical to the untorn case.
func TestReconcileSkipsGarbageTail(t *testing.T) {
	t.Parallel()

	t1 := reconcileTurnID(1)

	raw, err := os.ReadFile(filepath.Join("testdata", "reconcile", "class02_turn_without_terminal.jsonl"))
	if err != nil {
		t.Fatalf("read class02 fixture: %v", err)
	}

	torn := append([]byte{}, raw...)
	torn = append(torn, []byte(`{"type":"user_message","turnID":"sess_rec`)...)
	torn = append(torn, '\n')

	path := filepath.Join(t.TempDir(), "transcript_sess_recon.jsonl")

	werr := os.WriteFile(path, torn, filePermOwner)
	if werr != nil {
		t.Fatalf("write torn transcript: %v", werr)
	}

	lines, err := readTranscriptFile(path)
	if err != nil {
		t.Fatalf("readTranscriptFile: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("reader returned %d lines; want 2 (the garbage tail is skipped)", len(lines))
	}

	got, seed := Reconcile(reconcileFixtureSession, lines)

	want := []Line{
		{Type: TypeCanceled, TurnID: t1, Cause: InterruptedCause},
		{Type: TypeSessionEnd, Cause: InterruptedCause},
	}

	if len(got) != len(want) {
		t.Fatalf("Reconcile returned %d closures; want %d: %+v", len(got), len(want), got)
	}

	for i := range want {
		assertReconcileClosure(t, i, &got[i], &want[i])
	}

	if seed != (Seed{MaxTurns: 1}) {
		t.Errorf("seed = %+v; want {MaxTurns:1} (classification over the conforming prefix)", seed)
	}
}

// TestAppendSynthetic pins the append seam 18-05 drives: AppendSynthetic
// routes through the SAME marshal→redact→mutex→single-write path as
// appendLine (one write call, redactor invoked, line lands on disk with its
// provenance Cause intact), never a second write path. The countingRedactor
// fake (transcript_newkinds_test.go) is the routing proof — synthetic content
// crosses the redactor exactly like every other non-thinking append (16-D-23's
// exemption is raw_thinking-only).
func TestAppendSynthetic(t *testing.T) {
	t.Parallel()

	cr := &countingRedactor{}

	m, err := NewManager(t.TempDir(), "sess-syn", cr)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	t.Cleanup(func() { _ = m.Close() })

	line := &Line{
		Type: TypeCanceled, TurnID: "sess-syn-turn-001", Timestamp: time.Now().UTC(),
		Text: interruptedText, Cause: InterruptedCause,
	}

	aerr := m.AppendSynthetic(line)
	if aerr != nil {
		t.Fatalf("AppendSynthetic: %v", aerr)
	}

	if cr.calls() != 1 {
		t.Errorf("redactor invoked %d times; want 1 (synthetic content crosses the redactor)", cr.calls())
	}

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(lines) != 1 {
		t.Fatalf("transcript has %d lines; want 1", len(lines))
	}

	if lines[0].Type != TypeCanceled || lines[0].TurnID != "sess-syn-turn-001" || lines[0].Cause != InterruptedCause {
		t.Errorf("synthetic line lost its contract on disk: %+v", lines[0])
	}
}
