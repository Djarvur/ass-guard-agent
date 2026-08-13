package hookdag

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Djarvur/ass-guard-agent/internal/event"
)

// CommandRunner is the run-command seam (exec a shell command, capture
// stdout/stderr/exit-code). Plan 04-05 wires a real exec wrapper; tests inject
// a recording fake.
type CommandRunner interface {
	Run(ctx context.Context, command string, args []string) (stdout, stderr string, exitCode int, err error)
}

// TurnRunner is the send-prompt seam (one real turn through the Session Core).
type TurnRunner interface {
	Run(ctx context.Context, prompt []ContentBlock) (stop string, err error)
}

// BoundaryOpener is the fresh-context seam (open a new context window).
type BoundaryOpener interface {
	OpenBoundary(ctx context.Context, cause string) error
}

// Result is the outcome of one hook execution (HOOK-03 on-failure + HOOK-04
// provenance reflected in Status).
type Result struct {
	// Status ∈ completed / halted / asked / skipped-reentrant / error.
	Status string
	// StepsRun is how many steps completed before the chain stopped.
	StepsRun int
	// FailedStep is the 0-based index of the step that halted/asked, or -1.
	FailedStep int
	// Detail carries the failure detail (stderr, the ask message, etc.).
	Detail string
}

// Result status values (canonical strings).
const (
	StatusCompleted        = "completed"
	StatusHalted           = "halted"
	StatusAsked            = "asked"
	StatusSkippedReentrant = "skipped-reentrant"
	StatusError            = "error"
)

// Executor is the hand-rolled DAG executor (D-07). The only mutable state is
// the provenance in-flight set (HOOK-04), guarded by a mutex; every Execute
// call is otherwise stateless + deterministic over its inputs.
type Executor struct {
	Bus        *event.Bus
	Log        *slog.Logger
	Commands   CommandRunner
	Turns      TurnRunner
	Boundaries BoundaryOpener

	mu       sync.Mutex
	inFlight map[string]struct{} // key = HookName + "|" + TriggerStage
}

// Execute walks the hook's steps in declared order, dispatching each by Kind.
// on-failure (HOOK-03): halt stops the chain; continue logs + proceeds; ask
// suspends (returns StatusAsked). Provenance (HOOK-04): a non-reentrant hook
// whose (Name, Trigger) is already in-flight is refused (StatusSkippedReentrant)
// — the structural loop-prevention. Every step boundary emits a HookProgress
// event (investigate-and-fix-ready).
func (e *Executor) Execute(ctx context.Context, hook *Hook, prov Provenance) Result {
	if !e.begin(hook, prov) {
		detail := fmt.Sprintf(
			"hook %q on trigger %q already in-flight (allow_reentrant not set)",
			hook.Name, prov.TriggerStage)

		return Result{Status: StatusSkippedReentrant, FailedStep: -1, Detail: detail}
	}
	defer e.end(hook, prov)

	res := Result{Status: StatusCompleted, FailedStep: -1}

	for i := range hook.Steps {
		step := &hook.Steps[i]
		err := ctx.Err()
		if err != nil {
			res.Status = StatusError
			res.FailedStep = i
			res.Detail = fmt.Sprintf("ctx cancelled before step %q: %v", step.Name, err)
			e.emit(prov.SourceTurnID, hook.Name, i, step.Kind, StatusError, res.Detail)

			return res
		}

		e.emit(prov.SourceTurnID, hook.Name, i, step.Kind, "start", "")

		detail, err := e.dispatch(ctx, step)
		if err != nil {
			policy := effectiveOnFailure(step, hook)
			switch policy {
			case OnFailureHalt:
				e.emit(prov.SourceTurnID, hook.Name, i, step.Kind, StatusError, err.Error())

				res.Status = StatusHalted
				res.FailedStep = i
				res.Detail = err.Error()

				return res
			case OnFailureAsk:
				e.emit(prov.SourceTurnID, hook.Name, i, step.Kind, "ask", err.Error())

				res.Status = StatusAsked
				res.FailedStep = i
				res.Detail = err.Error()

				return res
			case OnFailureContinue:
				e.emit(prov.SourceTurnID, hook.Name, i, step.Kind, StatusError, err.Error()+" (on_failure:continue)")
				// log + proceed to the next step
			}
		} else {
			e.emit(prov.SourceTurnID, hook.Name, i, step.Kind, "finish", detail)
		}

		res.StepsRun++
	}

	return res
}

// dispatch routes a step to its impl by Kind.
func (e *Executor) dispatch(ctx context.Context, step *Step) (string, error) {
	switch step.Kind {
	case StepRunCommand:
		return runCommandStep(ctx, e, step)
	case StepSendPrompt:
		return sendPromptStep(ctx, e, step)
	case StepFreshContext:
		return freshContextStep(ctx, e, step)
	case StepWait:
		return waitStep(ctx, step)
	default:
		return "", fmt.Errorf("unknown step kind %q", step.Kind)
	}
}

// effectiveOnFailure returns the step's on-failure, falling back to the hook's
// when the step omits its policy.
func effectiveOnFailure(step *Step, hook *Hook) OnFailure {
	if step.OnFailure != "" {
		return step.OnFailure
	}

	if hook.OnFailure != "" {
		return hook.OnFailure
	}

	return OnFailureHalt // the safe default for steps that mutate
}

// emit publishes a HookProgress event at a step boundary (if the bus is wired).
// Failures here never block the chain (the executor is an observer of its own
// telemetry; a missing/lost event is logged but does not break the routine).
func (e *Executor) emit(turnID, hookName string, stepIdx int, kind StepKind, status, detail string) {
	if e.Bus == nil {
		return
	}

	e.Bus.Publish(event.HookProgress{
		TurnID:    turnID,
		HookName:  hookName,
		StepIndex: stepIdx,
		StepKind:  string(kind),
		Status:    status,
		Err:       detail,
	})
}

// begin is the provenance in-flight guard (HOOK-04). It returns false if the
// hook is already in-flight AND not allow_reentrant (the structural loop
// prevention). On success the entry is added + cleared by the matching end().
func (e *Executor) begin(hook *Hook, prov Provenance) bool {
	key := inFlightKey(hook, prov)

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.inFlight == nil {
		e.inFlight = map[string]struct{}{}
	}

	if _, ok := e.inFlight[key]; ok && !hook.AllowReentrant {
		return false
	}

	e.inFlight[key] = struct{}{}

	return true
}

// end clears the in-flight entry (deferred after every Execute).
func (e *Executor) end(hook *Hook, prov Provenance) {
	key := inFlightKey(hook, prov)

	e.mu.Lock()
	delete(e.inFlight, key)
	e.mu.Unlock()
}

func inFlightKey(hook *Hook, prov Provenance) string {
	return hook.Name + "|" + prov.TriggerStage
}
