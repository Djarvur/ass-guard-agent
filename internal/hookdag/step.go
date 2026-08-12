package hookdag

import (
	"context"
	"fmt"
	"time"
)

// ContentBlock is the local prompt-block shape (mirrors session.ContentBlock so
// 04-05's wiring adapter is a trivial cast — this package does NOT import
// internal/session, keeping the unit tests standalone + deterministic).
type ContentBlock struct {
	Type string
	Text string
}

// runCommandStep dispatches a run-command step via the CommandRunner seam. Exit
// code 0 ⇒ success (stdout returned as the detail); non-zero ⇒ an error whose
// message carries the exit code + stderr (the step's on-failure applies with the
// stderr as the failure detail — the Claudecourse exit-code contract, D-08).
func runCommandStep(ctx context.Context, e *Executor, step Step) (detail string, err error) {
	if e.Commands == nil {
		return "", fmt.Errorf("run-command %q: no command runner configured", step.Command)
	}

	stdout, stderr, code, runErr := e.Commands.Run(ctx, step.Command, step.Args)
	if runErr != nil {
		// A runner-level error (e.g. ctx cancel) takes precedence over the exit
		// code — surface it so the on-failure policy applies.
		return stdout, fmt.Errorf("run-command %q: %w", step.Command, runErr)
	}

	if code != 0 {
		return stdout, fmt.Errorf("run-command %q exit %d: %s", step.Command, code, stderr)
	}

	return stdout, nil
}

// sendPromptStep dispatches a send-prompt step: it is a REAL turn through the
// Session Core (HOOK-05 — the hook orchestrates turns; it does not bypass
// them). The turn's stop reason is recorded in the detail; stop=end_turn is NOT
// an error (the turn completed normally — only a real err is).
func sendPromptStep(ctx context.Context, e *Executor, step Step) (detail string, err error) {
	if e.Turns == nil {
		return "", fmt.Errorf("send-prompt %q: no turn runner configured", step.Name)
	}

	prompt := []ContentBlock{{Type: "text", Text: step.Prompt}}
	stop, runErr := e.Turns.Run(ctx, prompt)

	detail = "stop=" + stop
	if runErr != nil {
		return detail, fmt.Errorf("send-prompt %q: %w", step.Name, runErr)
	}

	return detail, nil
}

// freshContextStep dispatches a fresh-context step: open a new context window
// via the Phase-2 boundary semantics (HOOK-05). The next projection resets the
// lean window.
func freshContextStep(ctx context.Context, e *Executor, step Step) (detail string, err error) {
	if e.Boundaries == nil {
		return "", fmt.Errorf("fresh-context %q: no boundary opener configured", step.Name)
	}

	cause := "fresh-context:" + step.Name
	if err := e.Boundaries.OpenBoundary(ctx, cause); err != nil {
		return "", fmt.Errorf("fresh-context %q: %w", step.Name, err)
	}

	return cause, nil
}

// waitStep sleeps the parsed Duration (or until ctx cancels). A bad duration is
// a no-op sleep (the executor validates the shape at Load time; a stray bad
// value degrades to "no wait" rather than a panic).
func waitStep(ctx context.Context, step Step) (detail string, err error) {
	d := 0 * time.Second

	if step.Duration != "" {
		if parsed, perr := time.ParseDuration(step.Duration); perr == nil {
			d = parsed
		}
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(d):
		return "slept " + d.String(), nil
	}
}
