// AskUserQuestion execution (12-01, ACP-01 + D-01): the interactive-tool class
// of the core-executor template. The executor PARSES the captured
// {questions:[…]} shape and returns session.ErrSuspended with the parsed
// questions riding the result Output — the Stub seam carries no call identity,
// so the session tool loop (the one place that knows both turnID and callID)
// records the suspension, surfaces the question through the per-session
// AskBroker, and ends the turn with the ask marker. The reply (or the D-01
// timeout timer) later appends the tool result and resumes the SAME turn.
//
// Result forms are CAPTURE-GROUNDED — pinned to the committed fixture
// testdata/zcode-interactive-results.json: the ANSWERED form quotes the
// capture (via the 08-08 harvest note); the NON-ANSWER form is corpus-absent
// and follows the documented D-01 corpus-informed default, flagged for plan
// 12-05's re-record. The renderers live in internal/session/ask.go (the
// resume path is session-level; coreexec's conformance test pins them to the
// fixture so the form is defined exactly once).
package coreexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// askToolName is the captured catalog entry this executor fills in behind.
const askToolName = "AskUserQuestion"

// askArgs is the observed input shape (best-effort parse per the captured
// schema's surface; unknown fields — including the optional preview option
// field — are ignored, the 08-08 bashArgs pattern).
type askArgs struct {
	Questions []session.AskQuestion `json:"questions"`
}

// AskUserQuestionExecute returns the AskUserQuestion catalog Stub: it parses
// the model's questions and returns them as the Output alongside
// session.ErrSuspended (the suspension sentinel — the session tool loop ends
// the turn with the ask marker instead of appending a tool result). A nil
// broker yields the structured-error convention, NEVER a suspension (a
// suspension nobody can resolve is the dead end this plan removes).
func AskUserQuestionExecute(b *session.AskBroker) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if b == nil {
			return marshalStructured("ask: no broker configured for this session",
				fmt.Errorf("coreexec: ask: nil broker")) //nolint:wrapcheck // marshalStructured convention
		}

		var a askArgs

		_ = json.Unmarshal(args, &a) // best-effort: malformed input ⇒ zero questions, still suspends

		out, err := json.Marshal(a.Questions)
		if err != nil {
			return marshalStructured("ask: marshal questions failed", fmt.Errorf("coreexec: %w", err))
		}

		return out, session.ErrSuspended //nolint:wrapcheck // the sentinel IS the contract
	}
}

// RegisterAsk sets Execute on the AskUserQuestion catalog entry — the SAME
// registration site + discipline as RegisterCore (08-05/08-08): ONLY the
// Execute field is overridden; Name, Description, InputSchema, and Mutability
// stay byte-identical to the captured entry (the captured schema is the
// mimicry target, never rewritten). A missing catalog entry is skipped, not
// fatal (forward-compat).
func RegisterAsk(catalog *toolcat.Catalog, b *session.AskBroker) {
	if catalog == nil {
		return
	}

	if tool, ok := catalog.Get(askToolName); ok {
		tool.Execute = AskUserQuestionExecute(b)
		catalog.Register(tool)
	}
}

// RenderAskSurface renders the CLIENT-VISIBLE question form (the surface the
// broker's onSurface callback publishes as an agent-message chunk just before
// the suspended turn's response): header, question, option labels with
// descriptions, multiSelect noted. The renderer emits STRUCTURE, not judgment
// — the "(Recommended)" first-option convention is the MODEL's to author (the
// captured schema's own usage notes), never injected here.
func RenderAskSurface(qs []session.AskQuestion) string {
	var sb strings.Builder

	for i, q := range qs {
		if i > 0 {
			sb.WriteString("\n\n")
		}

		switch {
		case q.Header != "" && len(qs) > 1:
			fmt.Fprintf(&sb, "[%s] Question %d: %s\n", q.Header, i+1, q.Question)
		case q.Header != "":
			fmt.Fprintf(&sb, "[%s] %s\n", q.Header, q.Question)
		case len(qs) > 1:
			fmt.Fprintf(&sb, "Question %d: %s\n", i+1, q.Question)
		default:
			fmt.Fprintf(&sb, "%s\n", q.Question)
		}

		if q.MultiSelect {
			sb.WriteString("(multiple selections allowed)\n")
		}

		for j, o := range q.Options {
			if o.Description != "" {
				fmt.Fprintf(&sb, "%d. %s — %s\n", j+1, o.Label, o.Description)
			} else {
				fmt.Fprintf(&sb, "%d. %s\n", j+1, o.Label)
			}
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
