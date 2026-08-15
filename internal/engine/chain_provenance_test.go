package engine_test

import (
	"context"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// Hybrid provenance chaining (the findings-6 disposition, 2026-08-15): the
// nondeterministic explore→propose boundary chains on COMMAND PROVENANCE —
// when a turn STARTED by an expanded command (e.g. /opsx:explore) completes
// with end_turn, a table row keyed on that command key may carry the continue.
// The capture-seeded text rows stay authoritative for the deterministic
// boundaries (text/tool signals outrank provenance), and a NON-command turn's
// end_turn still triggers NOTHING (the structural-safety property extended to
// the provenance path).

// provenanceConstants (goconst).
const (
	exploreKey        = "opsx:explore"
	proposeCmd        = "/opsx:propose"
	signalExploreProv = "command:post-explore-handoff"
)

// provenanceTable matches a command row for "opsx:explore" only; text/tool
// signals are never set (the dual signals miss by construction here).
type provenanceTable struct{}

func (provenanceTable) MatchText(string) engine.MatchDetail { return engine.MatchDetail{} }
func (provenanceTable) MatchTool(string) engine.MatchDetail { return engine.MatchDetail{} }
func (provenanceTable) MatchCommand(key string) engine.MatchDetail {
	if key == exploreKey {
		return engine.MatchDetail{
			ID:           chainPatternID,
			Action:       engine.ActionContinue,
			Span:         key,
			ConfigSource: "fake command_patterns/" + chainPatternID,
		}
	}

	return engine.MatchDetail{}
}

// textWinsProvenanceTable matches ANY text ("handoff") AND carries the explore
// command row — the precedence fixture.
type textWinsProvenanceTable struct{ continueTable }

func (textWinsProvenanceTable) MatchCommand(key string) engine.MatchDetail {
	return provenanceTable{}.MatchCommand(key)
}

// TestDecide_CommandProvenanceSignal (hybrid Test 1): an explore-started turn
// whose free-form closing matches NO text/tool signal chains via the command
// row — Signal "command:<id>", the span is the COMMAND KEY, provenance carried.
func TestDecide_CommandProvenanceSignal(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{
		TurnID:    "t1",
		StartedBy: exploreKey,
		Text:      "Which thread pulls at you?", // the run-4 Socratic close — zero anchors
	}

	dec := engine.Decide(out, provenanceTable{})
	if dec.Action != engine.ActionContinue {
		t.Fatalf("Action = %v; want continue (command provenance)", dec.Action)
	}

	if dec.Signal != signalExploreProv {
		t.Errorf("Signal = %q; want %q", dec.Signal, signalExploreProv)
	}

	if dec.MatchedSpan != exploreKey {
		t.Errorf("MatchedSpan = %q; want the command key %q", dec.MatchedSpan, exploreKey)
	}

	if dec.TurnID != "t1" {
		t.Errorf("TurnID = %q; want t1 (provenance)", dec.TurnID)
	}

	if dec.ConfigSource != "fake command_patterns/post-explore-handoff" {
		t.Errorf("ConfigSource = %q; want the table-supplied source", dec.ConfigSource)
	}
}

// TestDecide_TextWinsOverCommandProvenance (hybrid Test 2): the dual signals
// stay authoritative — when text matches, the text row wins attribution even
// though the turn also carries a matching command provenance.
func TestDecide_TextWinsOverCommandProvenance(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{
		TurnID:    "t2",
		StartedBy: exploreKey,
		Text:      "done — handoff present",
	}

	dec := engine.Decide(out, textWinsProvenanceTable{})
	if dec.Action != engine.ActionContinue {
		t.Fatalf("Action = %v; want continue", dec.Action)
	}

	if dec.Signal != "text:"+chainPatternID {
		t.Errorf("Signal = %q; want text:%s (text wins attribution)", dec.Signal, chainPatternID)
	}
}

// TestDecide_NonCommandTurnTriggersNothing (hybrid Test 3 — the required
// regression): a turn NOT started by a command (StartedBy empty) triggers
// NOTHING even against a table carrying command rows — the unmatched-triggers
// -nothing property extended to the provenance path.
func TestDecide_NonCommandTurnTriggersNothing(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{
		TurnID:    "t3",
		Text:      "an ordinary free-form closing with no anchor",
		ToolCalls: []string{"Read", "Bash"},
	}

	dec := engine.Decide(out, provenanceTable{})
	if dec.Action != engine.ActionNothing {
		t.Errorf("Action = %v; want nothing (non-command turn — provenance path must stay silent)", dec.Action)
	}

	if dec.Signal != resultUnmatched {
		t.Errorf("Signal = %q; want unmatched", dec.Signal)
	}
}

// TestDecide_CommandProvenanceUnknownKey (hybrid Test 4): StartedBy names a
// command the table has NO row for (e.g. opsx:propose — regex stays
// authoritative there) ⇒ Nothing.
func TestDecide_CommandProvenanceUnknownKey(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{TurnID: "t4", StartedBy: "opsx:propose", Text: "free-form close"}

	dec := engine.Decide(out, provenanceTable{})
	if dec.Action != engine.ActionNothing {
		t.Errorf("Action = %v; want nothing (no command row for the key)", dec.Action)
	}
}

// TestDecide_TableWithoutCommandMatcherUnaffected (hybrid Test 5): a table
// that does NOT implement the optional CommandMatcher ignores the provenance
// input entirely — existing tables unchanged (the capability is additive).
func TestDecide_TableWithoutCommandMatcherUnaffected(t *testing.T) {
	t.Parallel()

	out := engine.TurnOutput{
		TurnID:    "t5",
		StartedBy: exploreKey,
		Text:      implementationCompleteMsg, // matches seededTable's text row
	}

	dec := engine.Decide(out, seededTable())
	if dec.Action != engine.ActionContinue {
		t.Fatalf("Action = %v; want continue via the TEXT row (existing behavior)", dec.Action)
	}

	if dec.Signal != "text:pat:"+implementationCompleteMsg {
		t.Errorf("Signal = %q; want the text attribution", dec.Signal)
	}

	// And the unmatched case stays nothing with StartedBy set: without the
	// capability the provenance input is inert.
	plain := engine.TurnOutput{TurnID: "t5b", StartedBy: exploreKey, Text: "no anchor at all"}

	dec = engine.Decide(plain, seededTable())
	if dec.Action != engine.ActionNothing {
		t.Errorf("Action = %v; want nothing (capability absent — provenance inert)", dec.Action)
	}
}

// TestChain_CommandProvenanceInjectsNextStage (hybrid Test 6): through the
// full Observe loop, an explore-started turn with an anchorless closing gets
// the populator's /opsx:propose injected as a REAL next turn — the existing
// ContinuePopulator machinery, driven by the provenance signal.
func TestChain_CommandProvenanceInjectsNextStage(t *testing.T) {
	t.Parallel()

	runner := &scriptedRunner{outputs: []engine.TurnOutput{
		{TurnID: "t1", StartedBy: exploreKey, Text: "Which thread pulls at you?"},
		{TurnID: "t2", Text: "proposed; no further handoff"},
	}}

	d := &populatingDispatcher{next: map[string]string{
		chainPatternID: proposeCmd,
	}}

	eng := &engine.Engine{Bus: event.NewBus(), Dispatcher: d}

	_, err := eng.Observe(context.Background(), runner, provenanceTable{},
		[]session.ContentBlock{{Type: blockText, Text: "/opsx:explore add-login"}})
	if err != nil {
		t.Fatalf("Observe err = %v", err)
	}

	if got := runner.calls.Load(); got != 2 {
		t.Fatalf("runner Run calls = %d; want 2 (explore turn + the provenance-injected propose)", got)
	}

	if len(runner.prompts) < 2 {
		t.Fatal("runner recorded fewer than 2 prompts")
	}

	injected := runner.prompts[1]
	if len(injected) != 1 || injected[0].Text != proposeCmd {
		t.Errorf("injected prompt = %+v; want the populator's %s text", injected, proposeCmd)
	}
}
