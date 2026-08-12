package engine

// PatternTable is the dual-signal source the engine consults (D-02). The
// concrete loader (TOML, from the OpenSpec overlay) lands in Plan 04-02; the
// engine depends only on this interface so the tracer (04-01) is dep-free and
// fully table-testable with a tiny in-memory fake.
type PatternTable interface {
	// MatchText scans assistant text for a known handoff pattern. Returns the
	// matched pattern's id + its action, or ("", ActionNothing) if no match.
	MatchText(text string) (patternID string, action Action)
	// MatchTool reports whether name is a known handoff tool-call and returns
	// its action (D-02 second signal — the model invoked a known handoff tool).
	MatchTool(name string) (toolID string, action Action)
}

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
	if pid, pAct := table.MatchText(out.Text); pAct != ActionNothing {
		reason := "text-pattern matched"
		// If a handoff tool is ALSO present, note it in the reason for the
		// investigate-and-fix-ready audit (text still wins the attribution).
		for _, name := range out.ToolCalls {
			if tid, tAct := table.MatchTool(name); tAct != ActionNothing {
				reason = "text-pattern matched (handoff tool " + tid + " also present; text wins attribution)"
				break
			}
		}
		return Decision{
			TurnID:  out.TurnID,
			Action:  pAct,
			Signal:  "text:" + pid,
			Reason:  reason,
		}
	}
	for _, name := range out.ToolCalls {
		if tid, tAct := table.MatchTool(name); tAct != ActionNothing {
			return Decision{
				TurnID: out.TurnID,
				Action: tAct,
				Signal: "tool:" + tid,
				Reason: "handoff tool-call matched",
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
