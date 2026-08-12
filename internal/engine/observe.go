package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// TurnRunner is the seam the engine wraps (the Session Core turn). The engine
// calls Run once for the user's prompt, then once per continue-injection. The
// tracer (04-01) injects a fake; Plan 04-05 wires an adapter over the real
// Session.Prompt + Manager.ReadSince.
type TurnRunner interface {
	// Run drives one turn for the given prompt and returns the stop reason +
	// error. ctx cancellation aborts the turn (D-16); the wrapper observes the
	// stop reason to detect end_turn.
	Run(ctx context.Context, prompt []session.ContentBlock) (stop string, err error)
	// LastTurnOutput returns the engine's view of the most recent turn — the
	// assistant text + tool-call names Decide inspects. The real impl (04-05)
	// reads the transcript; the tracer's fake returns canned values.
	LastTurnOutput() TurnOutput
}

// EngineDecisionWriter is the transcript seam the engine writes the
// engine_decision audit line through (D-20 — audit log = transcript). It is a
// small interface so the engine does NOT import session.Manager concretely
// (avoids a cycle: *session.Manager satisfies it; the engine is wrapped AROUND
// a Session in 04-05 but never imports the type).
type EngineDecisionWriter interface {
	AppendEngineDecision(turnID, action, signal, reason string) error
}

// ActionDispatcher dispatches the engine's non-continue actions (hook/ask) to
// the real seams — the hook-DAG executor + the learning store (Plan 04-05 T1
// wires the real impl). The tracer (04-01) leaves it nil; a nil dispatcher
// makes hook/ask degrade to ActionNothing so the loop stops gracefully.
//
// The methods take/return plain strings (NOT hookdag.Result) so this package
// stays dependency-free in 04-01; the real dispatcher in 04-05 adapts between
// this signature and the concrete hookdag.Executor / learning.Store APIs.
type ActionDispatcher interface {
	// Hook launches the hook-DAG matching the trigger/signal derived from the
	// matched pattern. Returns the DAG's terminal status (e.g. "completed",
	// "halted", "asked", "skipped-reentrant").
	Hook(ctx context.Context, signal string, sourceTurnID string) (status string)
	// Ask consults the learning store; if a learned answer exists it is applied
	// (returned as a short verb like "continue" / "wait:30s" / "fresh-context"),
	// else it returns ErrAskPending so the engine surfaces a session/update.
	Ask(ctx context.Context, situation string) (answer string, err error)
}

// ErrAskPending signals that an ActionAsk could not be resolved from the
// learning store + the engine must surface it to the user as a session/update.
// The wrapper returns + the user's reply lands as the next session/prompt.
var ErrAskPending = errors.New("engine: ask pending — no learned answer; surface to the user")

// Engine is the unified decision engine (D-01 post-turn observer). It is
// constructed once at startup (04-05) and Observe is called after each top-level
// sess.Prompt returns. The engine holds no per-turn mutable state — every
// decision is derived from (turn output, pattern table) at call time. A zero
// Engine (nil Bus + nil Manager) emits nothing but still runs the loop; the
// tracer uses a Bus + Manager wired with fakes.
type Engine struct {
	Bus        *event.Bus
	Manager    EngineDecisionWriter
	Log        *slog.Logger
	Dispatcher ActionDispatcher // optional — Plan 04-05 wires hook/ask dispatch
}

// Observe is the post-turn wrapper (D-01 — runs AFTER the wrapped Run returns
// end_turn, never inside the turn loop's critical path). It:
//
//  1. runs the user's prompt through the runner once;
//  2. loops: reads the just-finished turn's output, calls Decide, emits an
//     EngineDecision event + transcript line for EVERY decision (including
//     Nothing — the audit log proves the structural-safety property); for
//     ActionContinue it re-enters the runner with the next-stage prompt (a real
//     turn, D-12);
//  3. stops on the first non-continue decision, a non-end_turn turn result, a
//     real error, the re-fire budget cap (MaxContinueInjections — the second
//     infinite-loop bar), OR ctx cancellation (D-03 — the only off-switch; the
//     wrapper drains queued injections and returns ("cancelled", nil)).
//
// Graceful degradation (D-04): a deferred recover catches any panic in Decide /
// the wrapper itself; the original (stop, err) the wrapped Run produced are
// returned UNCHANGED (the turn has already completed, so the engine failure is
// invisible to the caller). Provenance (D-05): every Decision carries the
// source turn id.
//
// Cancel-drain (ENG-03 — the structural off-switch): the drain mechanism is ctx
// cancellation. Observe checks ctx.Err() before each continue-injection; a
// cancelled ctx ⇒ no further runner.Run calls + ("cancelled", nil). Plan 04-05
// runs Observe under the SAME ctx derived from the ACP turnCtx, so session/cancel
// (handleSessionCancel → sessionState.cancelTurn → cancel()) reaches this loop
// and drains every queued injection. See TestObserve_CancelDrain (unit) +
// cmd/ass-guard TestCancelDrainsInjections (ACP-level).
func (e *Engine) Observe(ctx context.Context, runner TurnRunner, table PatternTable, userPrompt []session.ContentBlock) (stop string, err error) {
	// Step 1: run the user's prompt. A panic here or anywhere below is
	// recovered and the ORIGINAL (stop, err) are returned — graceful
	// degradation. Named returns let the defer preserve them.
	stop, err = e.runAndRecover(ctx, runner, userPrompt)

	injections := 0
	for injections < MaxContinueInjections {
		if ctx.Err() != nil {
			// ENG-03: the only off-switch. A cancelled ctx drains queued
			// injections (none are launched) and the wrapper returns.
			return "cancelled", nil
		}
		// Step 2: read the just-finished turn + decide.
		out, perr := e.lastTurnAndRecover(runner)
		if perr != nil {
			// LastTurnOutput panicked — degrade gracefully, keep the original
			// (stop, err) from the user prompt (or the last injection).
			return stop, err
		}

		dec, derr := e.decideAndRecover(out, table)
		if derr != nil {
			// Decide (or the PatternTable) panicked — degrade gracefully.
			return stop, err
		}

		dec = e.applyDispatcher(ctx, dec)
		e.emit(dec)

		if dec.Action != ActionContinue {
			return stop, err
		}
		// Step 3: re-enter the runner with the continue-injection (a real turn).
		nextPrompt := dec.NextPrompt
		if len(nextPrompt) == 0 {
			nextPrompt = NextStagePrompt(dec)
		}

		injStop, injErr := e.runAndRecover(ctx, runner, nextPrompt)
		stop, err = injStop, injErr
		injections++

		if err != nil {
			return stop, err
		}

		if stop != "end_turn" {
			return stop, err
		}
	}
	// Re-fire budget exhausted — the second infinite-loop bar. Emit a Nothing
	// with a Reason noting the cap (logged, not errored — a safety stop).
	if injections == MaxContinueInjections {
		e.emit(Decision{
			TurnID: safeLastTurnID(runner),
			Action: ActionNothing,
			Signal: "budget",
			Reason: fmt.Sprintf("re-fire budget cap reached (%d continue-injections); stopping to prevent an infinite loop", MaxContinueInjections),
		})
	}

	return stop, err
}

// safeLastTurnID reads runner.LastTurnOutput under a recover so the budget
// message never panics even if the fake is misconfigured.
func safeLastTurnID(runner TurnRunner) (turnID string) {
	defer func() { _ = recover() }()

	out := runner.LastTurnOutput()

	return out.TurnID
}

// runAndRecover runs one turn through the runner, recovering any panic to the
// ORIGINAL (stop, err) so graceful degradation holds (D-04 — the turn has
// already completed by the time Decide runs; a wrapper/Decide panic must never
// surface as a crash or a different stop reason).
func (e *Engine) runAndRecover(ctx context.Context, runner TurnRunner, prompt []session.ContentBlock) (stop string, err error) {
	defer func() {
		if r := recover(); r != nil {
			e.logFailure("engine runner.Run panic recovered", r)
			// Leave stop/err as whatever Run already set (named returns). If the
			// panic happened before Run assigned them, they are the zero values
			// ("", nil) — the caller treats that as a no-op, never a crash.
		}
	}()

	return runner.Run(ctx, prompt)
}

// lastTurnAndRecover reads LastTurnOutput under a recover so a panicking fake
// (the T3 graceful-degradation test) is contained.
func (e *Engine) lastTurnAndRecover(runner TurnRunner) (out TurnOutput, err error) {
	defer func() {
		if r := recover(); r != nil {
			e.logFailure("engine LastTurnOutput panic recovered", r)
			err = fmt.Errorf("engine: LastTurnOutput panicked: %v", r)
		}
	}()

	out = runner.LastTurnOutput()

	return out, nil
}

// decideAndRecover runs the pure Decide under a recover so a panicking
// PatternTable (MatchText/MatchTool) is contained — graceful degradation (D-04,
// T3 Test 4). On panic the original (stop, err) are preserved by the caller
// (Observe returns them unchanged); Decide itself never panics on valid input.
func (e *Engine) decideAndRecover(out TurnOutput, table PatternTable) (dec Decision, err error) {
	defer func() {
		if r := recover(); r != nil {
			e.logFailure("engine Decide panic recovered", r)
			err = fmt.Errorf("engine: Decide panicked: %v", r)
		}
	}()

	return Decide(out, table), nil
}

// applyDispatcher maps hook/ask decisions through the optional ActionDispatcher
// (wired in Plan 04-05). When the Dispatcher is nil (the tracer), hook/ask
// degrade to ActionNothing so the loop stops gracefully. Continue is left to
// the wrapper's runner.Run re-entry (the Dispatcher is NOT used for continue —
// keeping one path for the real turn simplifies the contract).
func (e *Engine) applyDispatcher(ctx context.Context, dec Decision) Decision {
	if e.Dispatcher == nil {
		if dec.Action == ActionHook || dec.Action == ActionAsk {
			dec.Reason = dec.Reason + " (dispatcher not configured; degrading to nothing)"
			dec.Action = ActionNothing
		}

		return dec
	}

	switch dec.Action {
	case ActionHook:
		status := e.Dispatcher.Hook(ctx, dec.Signal, dec.TurnID)
		dec.Reason = fmt.Sprintf("hook %s: %s", dec.Signal, status)
		// A hook launches its own DAG; the loop does not re-enter the runner.
		dec.Action = ActionNothing
	case ActionAsk:
		ans, aerr := e.Dispatcher.Ask(ctx, dec.Signal)
		if aerr != nil {
			// No stored answer / ask pending — surface the ask + stop the loop.
			dec.Reason = fmt.Sprintf("ask pending (%s): %v", dec.Signal, aerr)
			dec.Action = ActionAsk // keep ask so the loop stops (ActionAsk != Continue)

			return dec
		}
		// A learned answer maps onto an action. The tracer only knows
		// "continue"; richer answers land in 04-05's real dispatcher.
		switch ans {
		case "continue":
			dec.Action = ActionContinue
			dec.Reason = "learned answer: continue"
		default:
			dec.Reason = "learned answer: " + ans
			dec.Action = ActionNothing
		}
	}

	return dec
}

// emit publishes the EngineDecision event + writes the engine_decision
// transcript line (for EVERY decision, including Nothing — D-20 audit log).
// Failures in the bus/manager are logged investigate-and-fix-ready but never
// block the loop (the engine is an observer; its telemetry must not break the
// turn).
func (e *Engine) emit(dec Decision) {
	if e.Bus != nil {
		e.Bus.Publish(event.EngineDecision{
			TurnID: dec.TurnID,
			Action: dec.Action.String(),
			Signal: dec.Signal,
			Reason: dec.Reason,
		})
	}

	if e.Manager != nil {
		werr := e.Manager.AppendEngineDecision(dec.TurnID, dec.Action.String(), dec.Signal, dec.Reason)
		if werr != nil {
			e.logFailure("engine AppendEngineDecision failed", werr)
		}
	}
}

// logFailure writes an investigate-and-fix-ready line (PROJECT.md) for a
// recovered engine panic/failure. It never panics itself.
func (e *Engine) logFailure(msg string, r any) {
	if e.Log != nil {
		e.Log.Error(msg, "detail", fmt.Sprint(r), "stack", string(debug.Stack()))

		return
	}
	// Fallback to slog.Default so the message is never lost.
	slog.Error(msg, "detail", fmt.Sprint(r), "stack", string(debug.Stack()))
}

// NextStagePrompt is the tracer's continue-injection: a single text block
// carrying the literal "continue". Plan 04-05's real wiring replaces this with
// the OpenSpec config's per-pattern next-stage prompt template.
func NextStagePrompt(dec Decision) []session.ContentBlock {
	return []session.ContentBlock{{Type: "text", Text: "continue"}}
}

// compile-time interface check: *Engine has Observe with the documented shape.
var _ interface {
	Observe(context.Context, TurnRunner, PatternTable, []session.ContentBlock) (string, error)
} = (*Engine)(nil)
