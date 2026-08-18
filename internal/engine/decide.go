package engine

// askSuspendedDecision returns the ask-suspension verdict (ActionAsk with NO
// table lookup — a suspended turn matches nothing; ErrAskPending semantics:
// surface the ask + stop the loop) and suspended=true when out is suspended.
func askSuspendedDecision(out TurnOutput) (Decision, bool) { //nolint:gocritic // hugeParam: value semantics
	if !out.AskSuspended {
		return Decision{}, false
	}

	return Decision{
		TurnID: out.TurnID,
		Action: ActionAsk,
		Signal: SignalAskSuspended,
		Reason: "turn suspended on AskUserQuestion — surface the ask, stop the loop (never chain)",
	}, true
}

// provenanceContinue is the THIRD chaining signal (hybrid chaining,
// findings-6 disposition): it fires only when the dual signals missed, the turn
// was STARTED by an expansion (out.StartedBy), AND the table implements the
// optional CommandMatcher with a non-Nothing row for that key — the
// deterministic fallback for the architecturally free-form explore closing.
func provenanceContinue(out TurnOutput, table PatternTable) (Decision, bool) { //nolint:gocritic // hugeParam
	if out.StartedBy == "" {
		return Decision{}, false
	}

	cm, ok := table.(CommandMatcher)
	if !ok {
		return Decision{}, false
	}

	d := cm.MatchCommand(out.StartedBy)
	if d.Action == ActionNothing {
		return Decision{}, false
	}

	return Decision{
		TurnID:       out.TurnID,
		Action:       d.Action,
		Signal:       "command:" + d.ID,
		MatchedSpan:  d.Span,
		ConfigSource: d.ConfigSource,
		Reason:       "command provenance matched (turn started by " + out.StartedBy + ")",
	}, true
}

// Decide is the PURE dual-signal detector (D-01 — a pure function of turn
// output + pattern table; no I/O, no globals, no time). It returns the engine's
// verdict for one finished turn:
//
//   - an ASK-SUSPENDED turn (out.AskSuspended — 12-01, ACP-01) ⇒ ActionAsk
//     with Signal SignalAskSuspended and NO table lookup: a suspended turn
//     must never chain (the Phase-8 pattern table would otherwise auto-continue
//     a suspended stage — the chained-stage hazard the no-chain regression
//     pins);
//   - a TEXT-PATTERN match (table.MatchText returns a non-Nothing action) ⇒
//     ActionContinue with Signal "text:<patternID>" (text wins attribution when
//     both signals are present — D-02);
//   - else a HANDOFF TOOL-CALL (table.MatchTool returns a non-Nothing action
//     for one of the turn's tool calls) ⇒ ActionContinue with Signal
//     "tool:<toolID>";
//   - else COMMAND PROVENANCE (hybrid chaining, findings-6 disposition): when
//     the turn was STARTED by an expanded command (out.StartedBy) and the table
//     implements the optional CommandMatcher with a non-Nothing row for that
//     key ⇒ ActionContinue with Signal "command:<patternID>" — the
//     deterministic fallback for the architecturally free-form explore closing;
//   - else ActionNothing with Signal "unmatched" (D-03 structural safety —
//     unmatched output triggers nothing).
//
// Every returned Decision carries TurnID = out.TurnID (D-05 provenance). The
// precedence is deliberate: the capture-seeded text/tool rows stay
// authoritative for the deterministic boundaries; provenance only carries the
// boundary whose closing text CANNOT be regex-chained honestly. StartedBy is
// sourced from the expansion seam (never assistant/tool content), so the
// assistant-role-only safety property holds unchanged — a non-command turn's
// end_turn triggers NOTHING even against a table carrying command rows.
func Decide(out TurnOutput, table PatternTable) Decision { //nolint:gocritic // hugeParam: pure-function value contract
	// Ask suspension first, before any table consultation (askSuspendedDecision
	// — ErrAskPending semantics: surface the ask + stop the loop).
	if dec, suspended := askSuspendedDecision(out); suspended {
		return dec
	}

	if d := table.MatchText(out.Text); d.Action != ActionNothing {
		reason := "text-pattern matched"
		// If a handoff tool is ALSO present, note it in the reason for the
		// investigate-and-fix-ready audit (text still wins the attribution).
		for _, name := range out.ToolCalls {
			if td := table.MatchTool(name); td.Action != ActionNothing {
				reason = "text-pattern matched (handoff tool " + td.ID + " also present; text wins attribution)"

				break
			}
		}

		return Decision{
			TurnID:       out.TurnID,
			Action:       d.Action,
			Signal:       "text:" + d.ID,
			MatchedSpan:  d.Span,
			ConfigSource: d.ConfigSource,
			Reason:       reason,
		}
	}

	for _, name := range out.ToolCalls {
		if d := table.MatchTool(name); d.Action != ActionNothing {
			return Decision{
				TurnID:       out.TurnID,
				Action:       d.Action,
				Signal:       "tool:" + d.ID,
				MatchedSpan:  d.Span,
				ConfigSource: d.ConfigSource,
				Reason:       "handoff tool-call matched",
			}
		}
	}

	// Third signal: command provenance (hybrid chaining) — see
	// provenanceContinue for the gating.
	if dec, ok := provenanceContinue(out, table); ok {
		return dec
	}

	return Decision{
		TurnID: out.TurnID,
		Action: ActionNothing,
		Signal: "unmatched",
		Reason: "no pattern or handoff tool matched",
	}
}
