package engine

import (
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// Action is the engine's decision vocabulary (D-02 dual-signal + D-08 step
// kinds the engine can dispatch). Plan 04-01 implements ActionContinue +
// ActionNothing; hook/ask/wait are carried as enum values dispatched by the
// ActionDispatcher in Plan 04-05 (hook) + Plan 04-06 (ask).
type Action int

const (
	// ActionNothing is the structural-safety decision (D-03): unmatched output
	// triggers nothing. The wrapper emits an EngineDecision{Action:nothing} line
	// (the audit log proves the property) and does NOT re-enter the turn loop.
	ActionNothing Action = iota
	// ActionContinue injects the next-stage prompt — a REAL turn through the
	// Session Core (D-12 — the engine orchestrates turns; it does not bypass
	// them). The re-fire budget (MaxContinueInjections) bounds the chain.
	ActionContinue
	// ActionHook launches a hook-DAG (wired in Plan 04-05 via ActionDispatcher).
	ActionHook
	// ActionAsk surfaces a session/update asking the user + waits (wired in
	// Plan 04-06 against the learning store).
	ActionAsk
	// ActionWait delays / schedules a timed re-check.
	ActionWait
)

// String returns the canonical lowercase name used in EngineDecision events +
// the engine_decision transcript line's `name` field (ENG-02).
func (a Action) String() string {
	switch a {
	case ActionNothing:
		return "nothing"
	case ActionContinue:
		return "continue"
	case ActionHook:
		return "hook"
	case ActionAsk:
		return "ask"
	case ActionWait:
		return "wait"
	default:
		return "unknown"
	}
}

// Decision is the engine's verdict for ONE turn (ENG-02 — the single
// provenance-tagged stream). The Observe wrapper emits one Decision per turn
// (including ActionNothing) so the audit log proves the structural-safety
// property: unmatched output triggers nothing (D-03).
type Decision struct {
	// TurnID is the source turn the decision is ABOUT (provenance, D-05).
	TurnID string
	// Action is the chosen next step.
	Action Action
	// Signal identifies what matched: "text:<patternID>", "tool:<toolID>", or
	// "unmatched". This is the investigate-and-fix-ready attribution.
	Signal string
	// NextPrompt is populated for ActionContinue — the prompt re-entered as a
	// real turn. Plan 04-05 fills the real OpenSpec next-stage text via
	// NextStagePrompt; the tracer (04-01) injects a literal "continue".
	NextPrompt []session.ContentBlock
	// Reason is the human-readable, investigate-and-fix-ready note logged in
	// the engine_decision line + the EngineDecision event (PROJECT.md).
	Reason string
}

// TurnOutput is the engine's view of a finished turn — the assistant's final
// text + the names of any tool calls it made (the two dual-signal sources the
// pure Decide function consults). Plan 04-05's real implementation reads the
// transcript via Manager.ReadSince to populate this; the tracer's fake
// TurnRunner returns canned values.
type TurnOutput struct {
	// TurnID is the last assistant_message's turn id (provenance — the source
	// of the decision).
	TurnID string
	// Text is the final assembled assistant text for the turn.
	Text string
	// ToolCalls is the ordered list of tool-call NAMES in this turn (D-02
	// second signal: a known handoff tool ⇒ continue).
	ToolCalls []string
}

// MaxContinueInjections is the re-fire budget — the second infinite-loop bar
// after provenance (D-05). The Observe wrapper injects at most this many
// continue-prompts per top-level Run call; a budget exhaustion ⇒ ActionNothing
// with a Reason noting the cap (logged, not errored — the budget is a safety
// stop, not a failure). Default 8 covers a full SDD scenario's stage count
// with headroom.
const MaxContinueInjections = 8
