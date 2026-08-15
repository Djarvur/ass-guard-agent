package engine_test

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
)

// fakeTable is a tiny in-memory PatternTable for the unit tests: it matches
// exact text literals + exact tool names. It is deterministic and pure.
type fakeTable struct {
	textPatterns map[string]engine.Action // literal text → action
	handoffTools map[string]engine.Action // tool name → action
}

func (f fakeTable) MatchText(text string) engine.MatchDetail {
	for lit, act := range f.textPatterns {
		if strings.Contains(text, lit) {
			return engine.MatchDetail{
				ID:           "pat:" + lit,
				Action:       act,
				Span:         lit,
				ConfigSource: "fake patterns/pat:" + lit,
			}
		}
	}

	return engine.MatchDetail{}
}

func (f fakeTable) MatchTool(name string) engine.MatchDetail {
	if act, ok := f.handoffTools[name]; ok {
		return engine.MatchDetail{
			ID:           "tool:" + name,
			Action:       act,
			Span:         name,
			ConfigSource: "fake handoff_tools/tool:" + name,
		}
	}

	return engine.MatchDetail{}
}

func seededTable() fakeTable {
	return fakeTable{
		textPatterns: map[string]engine.Action{
			implementationCompleteMsg: engine.ActionContinue,
		},
		handoffTools: map[string]engine.Action{
			openSpecHandoffKind: engine.ActionContinue,
		},
	}
}

// TestDecide_TextSignal verifies a text-pattern match ⇒ ActionContinue with
// Signal "text:<id>" (D-02 first signal).
func TestDecide_TextSignal(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{TurnID: "t1", Text: "done. ## Implementation Complete — ready for review"}

	dec := engine.Decide(out, seededTable())
	if dec.Action != engine.ActionContinue {
		t.Errorf("Action = %v; want ActionContinue", dec.Action)
	}

	if dec.Signal != "text:pat:## Implementation Complete — ready for review" {
		t.Errorf("Signal = %q; want text:...", dec.Signal)
	}

	if dec.TurnID != "t1" {
		t.Errorf("TurnID = %q; want t1 (provenance)", dec.TurnID)
	}
}

// TestDecide_ToolSignal verifies a handoff tool-call ⇒ ActionContinue with
// Signal "tool:<id>" when no text matches (D-02 second signal).
func TestDecide_ToolSignal(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{TurnID: "t2", Text: "advancing now", ToolCalls: []string{toolRead, openSpecHandoffKind}}

	dec := engine.Decide(out, seededTable())
	if dec.Action != engine.ActionContinue {
		t.Errorf("Action = %v; want ActionContinue", dec.Action)
	}

	if dec.Signal != "tool:tool:OpenSpecHandoff" {
		t.Errorf("Signal = %q; want tool:...", dec.Signal)
	}
}

// TestDecide_DualSignalTextWins verifies BOTH a text match AND a tool match ⇒
// ActionContinue with Signal "text:<id>" (text wins attribution; the tool is
// noted in Reason — D-02).
func TestDecide_DualSignalTextWins(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{
		TurnID:    "t3",
		Text:      implementationCompleteMsg,
		ToolCalls: []string{openSpecHandoffKind},
	}

	dec := engine.Decide(out, seededTable())
	if dec.Action != engine.ActionContinue {
		t.Errorf("Action = %v; want ActionContinue", dec.Action)
	}

	if !strings.HasPrefix(dec.Signal, "text:") {
		t.Errorf("Signal = %q; want text:* (text wins)", dec.Signal)
	}

	if !strings.Contains(dec.Reason, openSpecHandoffKind) {
		t.Errorf("Reason = %q; want it to note the also-present handoff tool", dec.Reason)
	}
}

// TestDecide_UnmatchedIsNothing is the structural-safety cell (D-03): no text
// match + no tool match ⇒ ActionNothing with Signal "unmatched".
func TestDecide_UnmatchedIsNothing(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{TurnID: "t4", Text: "random unrelated text", ToolCalls: []string{toolRead, "Grep"}}

	dec := engine.Decide(out, seededTable())
	if dec.Action != engine.ActionNothing {
		t.Errorf("Action = %v; want ActionNothing (structural safety)", dec.Action)
	}

	if dec.Signal != resultUnmatched {
		t.Errorf("Signal = %q; want unmatched", dec.Signal)
	}
}

// TestDecide_QuickUnmatchedIsNothing is the property assertion: for ANY turn
// output whose Text + ToolCalls are random strings/names NOT in the table,
// Decide returns ActionNothing (testing/quick — D-03 as a property).
func TestDecide_QuickUnmatchedIsNothing(t *testing.T) {
	t.Parallel()

	table := seededTable()
	gen := rand.New(rand.NewSource(1))
	// randomTurnOutput generates a Text over [a-z ] (length 0-40) + 0-2 tool
	// names over [a-z] (length 3-8) — none of which can ever equal the seeded
	// literals/handoff names, so the property "unmatched ⇒ Nothing" holds.
	randomTurnOutput := func() engine.TurnOutput {
		textLen := gen.Intn(41)

		var sb strings.Builder

		for range textLen {
			sb.WriteByte(byte('a' + gen.Intn(27))) // 'a'..'z' + space
		}

		nCalls := gen.Intn(3)
		calls := make([]string, 0, nCalls)

		for range nCalls {
			nameLen := 3 + gen.Intn(6)

			var nb strings.Builder

			for range nameLen {
				nb.WriteByte(byte('a' + gen.Intn(26)))
			}

			calls = append(calls, nb.String())
		}

		return engine.TurnOutput{TurnID: "rand", Text: sb.String(), ToolCalls: calls}
	}
	for i := range 500 {
		out := randomTurnOutput()

		dec := engine.Decide(out, table)

		if dec.Action != engine.ActionNothing {
			t.Fatalf("iteration %d: Decide(%+v) = %v; want ActionNothing (property violated)", i, out, dec.Action)
		}

		if dec.TurnID != out.TurnID {
			t.Fatalf("iteration %d: TurnID not carried (provenance)", i)
		}
	}
}

// TestDecide_QuickProperty runs the structural-safety property through
// testing/quick on a value generator (the canonical Go property-test harness).
func TestDecide_QuickProperty(t *testing.T) {
	t.Parallel()

	table := seededTable()

	property := func(text string, toolA, toolB string) bool {
		out := engine.TurnOutput{TurnID: "q", Text: text, ToolCalls: []string{}}
		if toolA != "" {
			out.ToolCalls = append(out.ToolCalls, toolA)
		}

		if toolB != "" {
			out.ToolCalls = append(out.ToolCalls, toolB)
		}

		dec := engine.Decide(out, table)
		// Either it matched a known signal (extremely unlikely from random
		// strings) or it returned Nothing. The load-bearing assertion: every
		// non-match is Nothing, never an ungrounded Continue.
		if dec.Action == engine.ActionContinue && dec.Signal == resultUnmatched {
			return false
		}

		if dec.Action == engine.ActionNothing && dec.Signal != resultUnmatched {
			return false
		}

		return dec.TurnID == "q"
	}

	err := quick.Check(property, &quick.Config{MaxCount: 200})
	if err != nil {
		t.Fatalf("quick check structural-safety property failed: %v", err)
	}
}

// TestDecide_EmptyOutputIsNothing verifies empty text + nil tool calls never
// panic and return ActionNothing (defensive — the engine may observe a turn
// that produced no assistant text).
func TestDecide_EmptyOutputIsNothing(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{TurnID: "empty"}

	dec := engine.Decide(out, seededTable())
	if dec.Action != engine.ActionNothing {
		t.Errorf("Action = %v; want ActionNothing for empty output", dec.Action)
	}
}

// TestDecide_ProvenanceCarried verifies Decision.TurnID == out.TurnID for every
// branch (D-05 provenance — the chain is auditable).
func TestDecide_ProvenanceCarried(t *testing.T) {
	t.Parallel()

	cases := []engine.TurnOutput{
		{TurnID: "text-turn", Text: implementationCompleteMsg},
		{TurnID: "tool-turn", ToolCalls: []string{openSpecHandoffKind}},
		{TurnID: "none-turn", Text: "nothing here"},
		{TurnID: "empty-turn"},
	}
	for _, out := range cases {
		dec := engine.Decide(out, seededTable())
		if dec.TurnID != out.TurnID {
			t.Errorf("TurnID = %q; want %q (provenance not carried)", dec.TurnID, out.TurnID)
		}
	}
}

// --- 09-02 T1: MatchDetail — span + config-source provenance (AUD-04) ---

// spanTable is a fake table returning span/config-source details.
type spanTable struct{}

func (spanTable) MatchText(text string) engine.MatchDetail {
	if i := strings.Index(text, "ready to implement"); i >= 0 {
		return engine.MatchDetail{
			ID:           "impl-complete",
			Action:       engine.ActionContinue,
			Span:         "ready to implement",
			ConfigSource: "openspec.toml patterns/impl-complete",
		}
	}

	return engine.MatchDetail{}
}

func (spanTable) MatchTool(name string) engine.MatchDetail {
	if name == "Task" {
		return engine.MatchDetail{
			ID:           "os-handoff",
			Action:       engine.ActionContinue,
			Span:         "Task",
			ConfigSource: "openspec.toml handoff_tools/os-handoff",
		}
	}

	return engine.MatchDetail{}
}

// TestDecide_TextMatchCarriesSpan (09-02 T1 Test 1): a text match populates
// MatchedSpan with the exact matched substring + ConfigSource from the table.
func TestDecide_TextMatchCarriesSpan(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{TurnID: "t1", Text: "The change is ready to implement the next stage now."}
	d := engine.Decide(out, spanTable{})

	if d.Signal != "text:impl-complete" || d.Action != engine.ActionContinue {
		t.Fatalf("Signal/Action = %q/%v; want text:impl-complete/continue", d.Signal, d.Action)
	}

	if d.MatchedSpan != "ready to implement" {
		t.Errorf("MatchedSpan = %q; want the exact matched substring", d.MatchedSpan)
	}

	if d.ConfigSource != "openspec.toml patterns/impl-complete" {
		t.Errorf("ConfigSource = %q; want the table-supplied source", d.ConfigSource)
	}
}

// TestDecide_ToolMatchSpanIsName (09-02 T1 Test 2): for a tool signal the
// span slot carries the tool-call NAME (TurnOutput carries names only).
func TestDecide_ToolMatchSpanIsName(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{TurnID: "t2", Text: "done", ToolCalls: []string{"Read", "Task"}}
	d := engine.Decide(out, spanTable{})

	if d.Signal != "tool:os-handoff" {
		t.Fatalf("Signal = %q; want tool:os-handoff", d.Signal)
	}

	if d.MatchedSpan != "Task" {
		t.Errorf("MatchedSpan = %q; want Task (the tool name IS the span)", d.MatchedSpan)
	}

	if d.ConfigSource != "openspec.toml handoff_tools/os-handoff" {
		t.Errorf("ConfigSource = %q; want the handoff entry source", d.ConfigSource)
	}
}

// TestDecide_UnmatchedStaysLean (09-02 T1 Test 3): no match ⇒ empty span +
// empty config source + signal "unmatched" (structural safety unchanged).
func TestDecide_UnmatchedStaysLean(t *testing.T) {
	t.Parallel()

	d := engine.Decide(engine.TurnOutput{TurnID: "t3", Text: "nothing here"}, spanTable{})

	if d.Action != engine.ActionNothing || d.Signal != "unmatched" {
		t.Fatalf("Action/Signal = %v/%q; want nothing/unmatched", d.Action, d.Signal)
	}

	if d.MatchedSpan != "" || d.ConfigSource != "" {
		t.Errorf("unmatched decision carries provenance: span=%q source=%q; want empty", d.MatchedSpan, d.ConfigSource)
	}
}

// TestDecide_DualSignalSpanIsText (09-02 T1 Test 4): text + handoff tool both
// present ⇒ text wins attribution and the span is the TEXT span (D-02 rule).
func TestDecide_DualSignalSpanIsText(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{TurnID: "t4", Text: "ready to implement", ToolCalls: []string{"Task"}}
	d := engine.Decide(out, spanTable{})

	if d.Signal != "text:impl-complete" {
		t.Fatalf("Signal = %q; want text:impl-complete (text wins)", d.Signal)
	}

	if d.MatchedSpan != "ready to implement" {
		t.Errorf("MatchedSpan = %q; want the TEXT span under dual signal", d.MatchedSpan)
	}

	if !strings.Contains(d.Reason, "also present") {
		t.Errorf("Reason = %q; want the co-present handoff note", d.Reason)
	}
}
