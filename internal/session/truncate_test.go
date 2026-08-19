package session //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// --- 14-05 (EARLY-05): tool-output truncation at the append chokepoint ---

// truncationMarkerPrefix is the leading shape of the corpus-absent truncation
// marker (the exact one-line form pinned by the plan).
const truncationMarkerPrefix = "[ass-guard: tool output truncated;"

// markerOverheadSlack bounds the marker line's byte overhead in size checks.
const markerOverheadSlack = 200

// promptRunTrunc / promptDispatchTrunc are the user-prompt texts for the
// truncation wiring tests (distinct from the legacy "run"/"dispatch" literals
// so the package's goconst counts stay calm).
const (
	promptRunTrunc      = "run the big one"
	promptDispatchTrunc = "dispatch the subagent"
)

// overCapResult builds a deterministic over-cap result ending in a tail
// sentinel, so tail-retention is directly assertable.
func overCapResult(t *testing.T) string {
	t.Helper()

	return strings.Repeat("x", 200*1024) + "TAIL-SENTINEL"
}

// TestTruncateToolResult_Cap (14-05, Test 5): a 200KB result is bounded to ≤
// cap + marker-line overhead, keeps the TAIL of the original, and the marker
// states the original size and the kept size. The marker must not collide
// with the captured Bash error/sentinel forms (no "Exit code " prefix).
func TestTruncateToolResult_Cap(t *testing.T) {
	t.Parallel()

	big := overCapResult(t)

	got := truncateToolResult(big)

	if !strings.HasPrefix(got, truncationMarkerPrefix) {
		t.Fatalf("truncated result must open with the marker line; got prefix %q", firstN(got, 60))
	}

	if !strings.HasSuffix(got, "TAIL-SENTINEL") {
		t.Error("truncation must keep the TAIL of the original (error summaries live at the tail)")
	}

	if !strings.Contains(got, fmt.Sprintf("original %d bytes", len(big))) {
		t.Errorf("marker must state the original size; got %q", markerLine(got))
	}

	if !strings.Contains(got, "kept tail ") {
		t.Errorf("marker must state the kept tail size; got %q", markerLine(got))
	}

	limit := DefaultToolResultCapBytes + markerOverheadSlack

	if len(got) > limit {
		t.Errorf("truncated result = %d bytes; want ≤ cap (%d) + marker overhead (%d)",
			len(got), DefaultToolResultCapBytes, markerOverheadSlack)
	}

	if len(got) >= len(big) {
		t.Errorf("truncated result (%d bytes) must be smaller than the original (%d bytes)", len(got), len(big))
	}

	if strings.HasPrefix(got, "Exit code ") {
		t.Error("the marker must not collide with the captured Bash error-form prefix")
	}
}

// TestTruncateToolResult_UnderCapUnmodified (14-05, Test 6): a corpus-scale
// 70KB result (the 08-08 harvest's largest observed result) passes through
// BYTE-IDENTICAL — no marker, no re-encoding, capture fidelity preserved.
func TestTruncateToolResult_UnderCapUnmodified(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("c", 70*1024) // the corpus-maximum scale, untruncated in the capture

	if got := truncateToolResult(body); got != body {
		t.Fatalf("a 70KB (under-cap) result must be byte-identical; got %d bytes", len(got))
	}
}

// TestTruncateToolResult_Edges (14-05, Test 7): the empty string stays empty
// (no marker); a result of EXACTLY cap bytes is unmodified (the boundary is
// >, not >=); cap+1 truncates; and a byte-boundary slice never splits a UTF-8
// rune (the marker's size math survives re-encoding).
func TestTruncateToolResult_Edges(t *testing.T) {
	t.Parallel()

	if got := truncateToolResult(""); got != "" {
		t.Errorf("empty result = %q; want empty with no marker", got)
	}

	exact := strings.Repeat("a", DefaultToolResultCapBytes)

	if got := truncateToolResult(exact); got != exact {
		t.Errorf("an exactly-at-cap result must be unmodified (boundary is >, not >=); changed to %d bytes", len(got))
	}

	oneOver := strings.Repeat("a", DefaultToolResultCapBytes) + "Z"

	got := truncateToolResult(oneOver)

	if !strings.HasPrefix(got, truncationMarkerPrefix) {
		t.Errorf("cap+1 bytes must truncate; got prefix %q", firstN(got, 60))
	}

	if !strings.HasSuffix(got, "Z") {
		t.Error("cap+1 truncation must keep the final byte (the tail)")
	}

	// Multibyte tail: the kept tail must remain valid UTF-8 even when the cap
	// boundary falls mid-rune (U+20AC is a 3-byte rune).
	multibyte := strings.Repeat("y", DefaultToolResultCapBytes+64) + strings.Repeat("€", 8192)

	mgot := truncateToolResult(multibyte)

	if !utf8.ValidString(mgot) {
		t.Error("the kept tail must stay valid UTF-8 (byte-boundary slices advance past a partial rune)")
	}

	if !strings.HasSuffix(mgot, "€") {
		t.Error("the multibyte tail must be retained through the rune-aligned boundary")
	}
}

// bigResultRunner is a subagentRunner returning a canned (over-cap) result —
// the Task-tool result path's input.
type bigResultRunner struct{ result string }

func (b bigResultRunner) Run(
	_ context.Context, _ *Session, _, _, _ string, _ []string, _ *ecosys.Agent,
) (string, error) {
	return b.result, nil
}

// bigOutputExec is a ToolExecutor returning a canned JSON-STRING result — the
// captured plain-text result form (Bash output renders as a JSON string).
type bigOutputExec struct{ output json.RawMessage }

func (b *bigOutputExec) Execute(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return b.output, nil
}

// toolResultPayloads returns ToolCallID → the decoded model-visible payload
// of every tool_result line (the plainContent decoding rule).
func toolResultPayloads(t *testing.T, s *Session) map[string]string {
	t.Helper()

	out := map[string]string{}

	lines := linesOf(s)

	for i := range lines {
		l := &lines[i]

		if l.Type != TypeToolResult {
			continue
		}

		if len(l.Output) > 0 && l.Output[0] == '"' {
			var payload string

			if json.Unmarshal(l.Output, &payload) == nil {
				out[l.ToolCallID] = payload

				continue
			}
		}

		out[l.ToolCallID] = string(l.Output)
	}

	return out
}

// TestSubagentResultAlsoBounded (14-05, artifacts test): the Task tool-result
// path — an over-cap subagent final result lands in the transcript ALREADY
// bounded (the chokepoint applies at the append boundary, before the line
// exists, hence before the Projector folds it into the projected window).
func TestSubagentResultAlsoBounded(t *testing.T) {
	t.Parallel()

	s, _, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})

	big := overCapResult(t)
	s.subagentRunner = bigResultRunner{result: big}

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: promptDispatchTrunc}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	payloads := toolResultPayloads(t, s)

	got, ok := payloads[toolTask]
	if !ok {
		t.Fatalf("no tool_result for the Task call; payloads = %v", keysOf(payloads))
	}

	if !strings.HasPrefix(got, truncationMarkerPrefix) {
		t.Errorf("the Task tool result must carry the truncated form; got prefix %q", firstN(got, 60))
	}

	if !strings.HasSuffix(got, "TAIL-SENTINEL") {
		t.Error("the Task tool result must retain the tail of the subagent result")
	}

	if len(got) >= len(big) {
		t.Errorf("bounded Task result (%d bytes) must be smaller than the original (%d bytes)", len(got), len(big))
	}
}

// truncParentRow drives the PARENT loop's per-call result append: a Bash tool
// call whose (over-cap) result flows through the batch append boundary.
func truncParentRow(t *testing.T, big string) map[string]string {
	t.Helper()

	s, _, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{Name: toolBash, Input: json.RawMessage(`{"command":"cat big"}`)},
			},
		},
		{FinishReason: stopEndTurn},
	})

	raw, mErr := json.Marshal(big)
	if mErr != nil {
		t.Fatalf("marshal big output: %v", mErr)
	}

	s.toolExec = &bigOutputExec{output: raw}

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: promptRunTrunc}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	return toolResultPayloads(t, s)
}

// truncSubagentRow drives the subagent Task-result append: a Task tool call
// whose over-cap final result flows through the Task append boundary.
func truncSubagentRow(t *testing.T, big string) map[string]string {
	t.Helper()

	s, _, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls:    []provider.ToolCall{{Name: toolTask, Input: json.RawMessage(`{"prompt":"x"}`)}},
		},
		{FinishReason: stopEndTurn},
	})

	s.subagentRunner = bigResultRunner{result: big}

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: promptDispatchTrunc}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	return toolResultPayloads(t, s)
}

// TestAppendToolResult_TruncatedBeforeTranscript (14-05, Test 8): the
// chokepoint proven AT the boundary through the real Session append path —
// BOTH consumers (the parent loop's per-call result append AND the subagent
// Task-result append) write the truncated form into the transcript line.
func TestAppendToolResult_TruncatedBeforeTranscript(t *testing.T) {
	t.Parallel()

	big := overCapResult(t)

	table := []struct {
		name string
		run  func(t *testing.T, big string) map[string]string
	}{
		{name: "parent tool result", run: truncParentRow},
		{name: "subagent Task result", run: truncSubagentRow},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payloads := tc.run(t, big)

			found := false

			limit := DefaultToolResultCapBytes + markerOverheadSlack

			for id, got := range payloads {
				if !strings.HasPrefix(got, truncationMarkerPrefix) {
					continue
				}

				found = true

				if !strings.HasSuffix(got, "TAIL-SENTINEL") {
					t.Errorf("result %s must retain the tail", id)
				}

				if len(got) > limit {
					t.Errorf("result %s = %d bytes; want ≤ cap + marker overhead", id, len(got))
				}
			}

			if !found {
				t.Errorf("no transcript tool_result carries the truncated form; payloads = %v", keysOf(payloads))
			}
		})
	}
}

// TestTruncation_UnderCapTranscriptByteIdentical pins the fidelity property at
// the boundary: an under-cap tool result reaches the transcript byte-identical
// (the chokepoint never re-encodes what it does not truncate).
func TestTruncation_UnderCapTranscriptByteIdentical(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("u", 70*1024) // corpus-maximum scale

	s, _, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{
				{Name: toolBash, Input: json.RawMessage(`{"command":"cat corpus"}`)},
			},
		},
		{FinishReason: stopEndTurn},
	})

	raw, mErr := json.Marshal(body)
	if mErr != nil {
		t.Fatalf("marshal body: %v", mErr)
	}

	original := string(raw)
	s.toolExec = &bigOutputExec{output: raw}

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: promptRunTrunc}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	found := false

	for _, l := range linesOf(s) {
		if l.Type != TypeToolResult {
			continue
		}

		found = true

		if string(l.Output) != original {
			t.Errorf("an under-cap result must reach the transcript BYTE-IDENTICAL; %d bytes changed", len(l.Output))
		}
	}

	if !found {
		t.Fatal("no tool_result line for the under-cap run")
	}
}

// firstN returns the first n bytes of s for failure messages.
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n]
}

// markerLine returns the first line of s (the marker line for failure messages).
func markerLine(s string) string {
	head, _, _ := strings.Cut(s, "\n")

	return head
}

// keysOf returns the map's keys (failure-message helper).
func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}
