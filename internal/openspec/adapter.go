package openspec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
)

// ErrOpenSpecNotFound signals the openspec binary is not on PATH (OPEN-01 — the
// engine routes this to learning ask so the operator can install/configure it).
var ErrOpenSpecNotFound = errors.New("openspec binary not found on PATH")

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
func (a *Adapter) Run(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
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

	stdout, stderr = stdoutBuf.String(), stderrBuf.String()
	if waitErr == nil {
		return stdout, stderr, nil
	}

	if ctx.Err() != nil {
		// ctx-cancelled kill — surface the ctx error (no orphan; T-04-06).
		return stdout, stderr, ctx.Err()
	}
	// Non-zero exit: carry the exit code + stderr in the error message.
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return stdout, stderr, fmt.Errorf("openspec %s exit %d: %s", command, exitErr.ExitCode(), stderr)
	}

	return stdout, stderr, fmt.Errorf("openspec %s: %w", command, waitErr)
}
