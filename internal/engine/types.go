package engine

import (
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// PatternTable is the dual-signal source the engine consults (D-02). The
// engine depends only on this interface so it stays fully table-testable with
// tiny in-memory fakes; the concrete OpenSpec table lives in
// internal/openspec (dependency direction: openspec -> engine, never the
// reverse).
type PatternTable interface {
	// MatchText scans assistant text for a known handoff pattern. Returns the
	// match detail; the ZERO VALUE (Action == ActionNothing) means no match.
	MatchText(text string) MatchDetail
	// MatchTool reports whether name is a known handoff tool-call (D-02 second
	// signal — the model invoked a known handoff tool). Same zero-value
	// convention.
	MatchTool(name string) MatchDetail
}

// CommandMatcher is an OPTIONAL PatternTable capability (hybrid chaining — the
// findings-6 disposition, 2026-08-15): tables that support command-provenance
// rows answer whether the command key that STARTED a turn carries a configured
// action. Consulted only AFTER the dual signals miss (text/tool stay
// authoritative — the capture-seeded rows own the deterministic boundaries) and
// only for turns actually started by an expansion (TurnOutput.StartedBy). The
// capability is purely additive: tables that don't implement it are unaffected
// (Decide skips the provenance input entirely).
type CommandMatcher interface {
	// MatchCommand reports whether key — the registry command key whose
	// expansion started the turn (e.g. "opsx:explore") — is a configured
	// chaining row. The Span slot carries the command KEY (the matched signal
	// text, mirroring the tool-name convention). Zero value ⇒ no row.
	MatchCommand(key string) MatchDetail
}

// MatchDetail is one pattern-table match with FULL provenance (09-02, AUD-04 /
// D-03): what matched (ID), what to do (Action), the literal matched text
// (Span), and which config entry fired (ConfigSource — supplied by the TABLE
// implementation, keeping the engine table-agnostic). The zero value is "no
// match". For tool signals the Span slot carries the tool-call NAME —
// TurnOutput carries names, not inputs, so the name IS the matched signal
// text.
type MatchDetail struct {
	// ID is the matched entry's id (pattern id or handoff-tool id).
	ID string
	// Action is the matched entry's action.
	Action Action
	// Span is the literal matched substring (text signals) or the tool-call
	// name (tool signals).
	Span string
	// ConfigSource names the config entry that fired, e.g.
	// "openspec.toml patterns/<id>" — identifiers only, never file contents.
	ConfigSource string
}

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
		return stopContinue
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
	// MatchedSpan is the literal matched text (AUD-04 / D-03 full
	// provenance): the exact substring for text signals, the tool-call NAME
	// for tool signals (TurnOutput carries names). Empty when unmatched.
	MatchedSpan string
	// ConfigSource names the config entry that fired (table-supplied, e.g.
	// "openspec.toml patterns/<id>") — identifiers only, never file contents.
	// Empty when unmatched.
	ConfigSource string
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
	// StartedBy is the registry command key whose EXPANSION started this turn
	// (hybrid chaining, findings-6 disposition) — e.g. "opsx:explore" when the
	// user (or an engine injection) invoked /opsx:explore and the 08-04 seam
	// expanded it; "" for plain-text turns. It is sourced EXCLUSIVELY from the
	// expansion seam (the user-side invocation record — the same lookup that
	// writes the command_provenance line), NEVER from assistant/tool content:
	// the assistant-role-only safety property is untouched (model-visible text
	// cannot fabricate a provenance key).
	StartedBy string
	// AskSuspended marks a turn that ENDED on an AskUserQuestion suspension
	// (12-01, ACP-01): the model asked, the turn waits on the operator. Decide
	// returns ActionAsk for such a turn with NO table lookup — a suspended
	// turn must NEVER chain (the Phase-8 pattern table would otherwise
	// auto-continue a suspended stage; the no-chain regression pin).
	AskSuspended bool
	// PlanMode marks a turn that ended while plan mode was ON (12-04, ACP-02):
	// engine-decision PROVENANCE ONLY (signal context) — no chaining behavior
	// keys on it (a plan-mode turn ends like any other; the gate lives at the
	// tool-exec layer, not the engine).
	PlanMode bool
}

// MaxContinueInjections is the re-fire budget — the second infinite-loop bar
// after provenance (D-05). The Observe wrapper injects at most this many
// continue-prompts per top-level Run call; a budget exhaustion ⇒ ActionNothing
// with a Reason noting the cap (logged, not errored — the budget is a safety
// stop, not a failure). Default 8 covers a full SDD scenario's stage count
// with headroom.
const MaxContinueInjections = 8

// SignalAskSuspended is the Decision.Signal for an ask-suspended turn
// (12-01, ACP-01): a LIVE user question with a pending tool call. The
// learning store is NOT consulted for this signal (applyDispatcher keeps
// ActionAsk — a stored answer must never resume-chain a suspended turn).
const SignalAskSuspended = "ask:suspended"

// StopAsk mirrors the session layer's ask-suspension stop marker (13-00 —
// the stopAskACP house pattern: internal/session owns the vocabulary, the
// engine carries its own constant so Observe can recognize the suspension
// without importing the session stop vocabulary's private form). A runner
// Run returning StopAsk means the turn ended suspended on AskUserQuestion;
// with an AskSettler capability the engine WAITS (the manager ruling's
// route 1 — the resume becomes engine-visible); without one, today's
// semantics hold exactly.
const StopAsk = "ask"

// AskSettler is an OPTIONAL TurnRunner extension (13-00, the ContinuePopulator
// precedent — additive interface, nil-safe): a runner whose turns can suspend
// on AskUserQuestion exposes the CURRENT suspension's settle signal — the
// one-shot channel closed after the resumed turn completes (whichever driver
// resumed it: the operator reply or the D-01 timer). Observe consults it on
// the ask stop, blocks until the signal or ctx death, then re-reads the
// completed turn and decides NORMALLY. Nil channel / no capability ⇒ today's
// behavior byte-identical.
type AskSettler interface {
	// AskSettle returns the current suspension's settle channel — nil when
	// nothing is pending, already-closed once the resume completed.
	AskSettle() <-chan struct{}
}
