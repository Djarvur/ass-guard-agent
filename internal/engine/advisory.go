package engine

import "regexp"

// The unmatched-ending advisory (13-03, D-02 + D-05): when a turn ends
// UNMATCHED with a question-shaped closing, the engine emits an ADVISORY —
// an audit-always EngineDecision carrying the advisory signal with a Reason
// naming the suggested AskUserQuestion route — while the returned Action
// stays exactly ActionNothing. The advisory NEVER holds continuation (the
// Phase-8 question-ending auto-continue rejection is untouched); the ACP-side
// wrapper dedupes the client-visible note per session + class.

// ClassChoiceEnding is the captured 08-06 stage-4 choice-ending class: a
// closing that offers the operator numbered options ("Which would you like…
// (1/2)"). The class id is a FIXED table constant — never derived from the
// matched text (T-13-03-01: model content can never steer the class or the
// note).
const ClassChoiceEnding = "choice"

// classifyTable is the ordered question-shaped-ending classifier (first match
// wins). Each row's regex is RE2-safe and anchored on CAPTURED closings (D-12
// discipline applies to classifier rows too — the provenance comment names
// the source).
//
//nolint:gochecknoglobals // the ordered constant classifier table
var classifyTable = []struct {
	class string
	re    *regexp.Regexp
}{
	{
		class: ClassChoiceEnding,
		// Source: the 08-06 stage-4 capture (2026-08-15) — "Which would you
		// like? (1/2)" — and the 13-01 onboard-happy closing (2026-08-20):
		// "Which task interests you? (Pick a number or describe your own)".
		re: regexp.MustCompile(`(?i)\? *\(([123]/[123]|[Pp]ick a number[^)]*)\)`),
	},
}

// ClassifyQuestionEnding reports whether the closing text is question-shaped
// (an interactive dead-end the operator should route through
// AskUserQuestion), returning the fixed class id. Pure + stateless — the
// dedupe state lives in the ACP-side wrapper, never here.
func ClassifyQuestionEnding(text string) (string, bool) {
	for _, row := range classifyTable {
		if row.re.MatchString(text) {
			return row.class, true
		}
	}

	return "", false
}
