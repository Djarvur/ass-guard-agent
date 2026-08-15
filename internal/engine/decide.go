package engine

// Decide is the PURE dual-signal detector (D-01 — a pure function of turn
// output + pattern table; no I/O, no globals, no time). It returns the engine's
// verdict for one finished turn:
//
//   - a TEXT-PATTERN match (table.MatchText returns a non-Nothing action) ⇒
//     ActionContinue with Signal "text:<patternID>" (text wins attribution when
//     both signals are present — D-02);
//   - else a HANDOFF TOOL-CALL (table.MatchTool returns a non-Nothing action
//     for one of the turn's tool calls) ⇒ ActionContinue with Signal
//     "tool:<toolID>";
//   - else ActionNothing with Signal "unmatched" (D-03 structural safety —
//     unmatched output triggers nothing).
//
// Every returned Decision carries TurnID = out.TurnID (D-05 provenance).
func Decide(out TurnOutput, table PatternTable) Decision {
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

	return Decision{
		TurnID: out.TurnID,
		Action: ActionNothing,
		Signal: "unmatched",
		Reason: "no pattern or handoff tool matched",
	}
}
