package openspec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrOpenSpecNotFound signals the openspec binary is not on PATH (OPEN-01 — the
// engine routes this to learning ask so the operator can install/configure it).
var ErrOpenSpecNotFound = errors.New("openspec binary not found on PATH")

// Outcome classifications for RunGuarded results (D-10 — the model adapts on
// any of these; none is a Go error).
const (
	ClassOK       = "ok"
	ClassFixable  = "fixable"
	ClassHardErr  = "hard-error"
	ClassTimeout  = "timeout"
	ClassNotFound = "not-found"
)

// GuardedResult is the structured, model-facing outcome of one guarded run:
// stdout/stderr as captured, the exit code, and the classification.
type GuardedResult struct {
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	ExitCode       int    `json:"exit_code"`
	Classification string `json:"classification"`
}

// Adapter is the OpenSpec subprocess host (D-13 / OPEN-01). It shells out to the
// unmodified OpenSpec CLI, capturing stdout (surfaced as the tool result) +
// stderr (operator-visible diagnostics). Binary defaults to "openspec" + is
// overridable for tests (a stub script). ctx cancellation signals + kills the
// child process so no orphan is left (T-04-06).
type Adapter struct {
	Binary string // default "openspec"; overridable for tests (e.g. a stub script path)
	Log    *slog.Logger
}

// Run executes `openspec <command> <args...>` under ctx, returning stdout +
// stderr + err per the exit-code contract:
//
//   - exit 0 => (stdout, "", nil) — stdout is the tool result;
//   - non-zero => (stdout, stderr, err) where err carries the exit code + stderr
//     (stderr is NOT surfaced to the model unless the command failed — T-04-06b);
//   - binary not on PATH => ErrOpenSpecNotFound (typed, errors.Is);
//   - ctx cancelled => the process is killed (no orphan) + the ctx error is
//     returned within ~200ms.
//
//nolint:gocritic // conflicts w/ nonamedreturns
func (a *Adapter) Run(ctx context.Context, command string, args ...string) (string, string, error) {
	binary := a.Binary
	if binary == "" {
		binary = "openspec"
	}

	argv := append([]string{command}, args...)
	cmd := exec.CommandContext(ctx, binary, argv...)

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd.Stdout = &stdoutBuf

	cmd.Stderr = &stderrBuf

	startErr := cmd.Start()
	if startErr != nil {
		// exec.ErrNotFound covers a missing binary (LookPath fails inside Start);
		// the wrapped *os.PathError / *exec.Error chain satisfies errors.Is.
		if errors.Is(startErr, exec.ErrNotFound) {
			return "", "", ErrOpenSpecNotFound
		}

		return "", "", fmt.Errorf("openspec: start %q: %w", binary, startErr)
	}

	waitErr := cmd.Wait()

	stdout, stderr := stdoutBuf.String(), stderrBuf.String()
	if waitErr == nil {
		return stdout, stderr, nil
	}

	if ctx.Err() != nil {
		// ctx-cancelled kill — surface the ctx error (no orphan; T-04-06).
		return stdout, stderr, fmt.Errorf("ctx: %w", ctx.Err())
	}
	// Non-zero exit: carry the exit code + stderr in the error message.
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		//nolint:err113 // dynamic error message
		return stdout, stderr, fmt.Errorf("openspec %s exit %d: %s", command, exitErr.ExitCode(), stderr)
	}

	return stdout, stderr, fmt.Errorf("openspec %s: %w", command, waitErr)
}

// nonInteractiveEnv is the verified non-TTY kill-switch injected into every
// guarded child env (Pitfall 7 — an interactive prompt would hang the turn).
const nonInteractiveEnv = "OPEN_SPEC_INTERACTIVE=0"

// RunGuarded executes `openspec <argv...>` under the Phase-8 non-interactive
// guards (CMD-03) and returns the structured outcome — NEVER a Go error for
// command-level failures (D-10: structure over error; diagnostics go to the
// Adapter's stderr slog):
//
//   - per-command timeout (context.WithTimeout, distinct from the turn ctx —
//     an openspec call must not inherit the turn's long budget); timeoutSecs
//     <= 0 falls back to DefaultTimeoutSecs;
//   - OPEN_SPEC_INTERACTIVE=0 prepended to the child env + nil stdin (any
//     prompt read gets immediate EOF);
//   - classification: exit 0 → ok; non-zero + exitClass "fixable" → fixable;
//     other non-zero → hard-error; timeout → timeout; missing binary →
//     not-found.
func (a *Adapter) RunGuarded(
	ctx context.Context,
	timeoutSecs int,
	exitClass, argv string, args ...string,
) GuardedResult {
	if timeoutSecs <= 0 {
		timeoutSecs = DefaultTimeoutSecs
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	full := append(strings.Fields(argv), args...)

	stdout, stderr, runErr := a.runGuardedProcess(runCtx, full[0], full[1:]...)

	res := GuardedResult{Stdout: stdout, Stderr: stderr}

	switch {
	case runErr == nil:
		res.ExitCode = 0
		res.Classification = ClassOK
	case errors.Is(runErr, ErrOpenSpecNotFound):
		res.Classification = ClassNotFound

		a.log().Warn("openspec binary not found", "argv", argv)
	case errors.Is(runErr, context.DeadlineExceeded) && runCtx.Err() != nil:
		res.Classification = ClassTimeout

		a.log().Warn("openspec command timed out", "argv", argv, "timeout_secs", timeoutSecs)
	default:
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		}

		res.Classification = ClassHardErr
		if exitClass == ExitClassFixable {
			res.Classification = ClassFixable
		}

		a.log().Warn("openspec command failed", "argv", argv, "exit_code", res.ExitCode,
			"classification", res.Classification)
	}

	return res
}

// runGuardedProcess is Run with the guard env/stdin applied (Run stays as-is
// for existing callers/tests).
//
//nolint:gocritic // conflicts w/ nonamedreturns
func (a *Adapter) runGuardedProcess(
	ctx context.Context, command string, args ...string,
) (string, string, error) {
	binary := a.Binary
	if binary == "" {
		binary = "openspec"
	}

	argv := append([]string{command}, args...)
	cmd := exec.CommandContext(ctx, binary, argv...)

	// Non-interactive guards: env kill-switch + nil stdin (prompt reads EOF).
	cmd.Env = append(os.Environ(), nonInteractiveEnv)
	cmd.Stdin = nil

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	startErr := cmd.Start()
	if startErr != nil {
		if errors.Is(startErr, exec.ErrNotFound) {
			return "", "", ErrOpenSpecNotFound
		}

		return "", "", fmt.Errorf("openspec: start %q: %w", binary, startErr)
	}

	waitErr := cmd.Wait()

	stdout, stderr := stdoutBuf.String(), stderrBuf.String()
	if waitErr == nil {
		return stdout, stderr, nil
	}

	if ctx.Err() != nil {
		return stdout, stderr, fmt.Errorf("ctx: %w", ctx.Err())
	}

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		// Wrap (not flatten) so RunGuarded's classifier can extract the code.
		return stdout, stderr, fmt.Errorf("openspec %s exit %d: %s: %w", command, exitErr.ExitCode(), stderr, exitErr)
	}

	return stdout, stderr, fmt.Errorf("openspec %s: %w", command, waitErr)
}

// log returns the Adapter's logger or a stderr default (never stdout — the ACP
// discipline).
func (a *Adapter) log() *slog.Logger {
	if a.Log != nil {
		return a.Log
	}

	return slog.Default()
}
