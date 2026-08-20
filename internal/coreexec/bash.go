// Package coreexec implements REAL execution for the /opsx working set of
// core catalog tools (08-08): Bash, Read, Write, Edit, TodoWrite, TodoRead.
// Result-message shapes are CAPTURE-GROUNDED — pinned to the committed
// fixture testdata/zcode-core-results.json (live zcode rollout harvest
// 2026-08-15, sessions 4440f5a7 main + 8a003655 subagent) — with every
// corpus-absent form explicitly flagged (structured {"error":…} convention
// or a documented corpus-informed default) for the Phase-9 re-capture.
// A faked or stubbed executor is exactly the hollow green the gates exist
// to prevent: every function here does the real work.
package coreexec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// Captured Bash result forms (fixture-pinned; provenance: 105 text + 2
// error-text results in the main session, 137/137 subagent errors).
const (
	// bashSentinel is the captured no-output success form.
	bashSentinel = "(Bash completed with no output)"
	// bashExitPrefix is the captured error-form header (Exit code <N>\n<output>).
	bashExitPrefix = "Exit code "
)

// keyError is the structured-error convention's map key (the shipped
// corpus-absent failure convention — {"error":…}).
const keyError = "error"

// errHookRefused marks a PreToolUse exit-2 refusal (12-02): the hook's message
// rides the structured result as the tool result — the operator-configured
// policy channel, not a Go-level failure of the executor itself.
var errHookRefused = errors.New("coreexec: tool call refused by PreToolUse hook")

// Timeout bounds (SCHEMA-DECLARED in coretools.json's Bash description:
// "timeout is in milliseconds: default 120000, max 600000" — the corpus
// itself shows only explicit 60000–660000 ms values in the subagent sessions
// and omits the field entirely in the main session, so the DEFAULT is
// corpus-informed via the schema, flagged in the fixture's corpus_absent).
const (
	bashTimeoutDefaultMS = 120000
	bashTimeoutMaxMS     = 600000
)

// bashArgs is the observed input shape: {command, description} always;
// timeout (ms) sometimes (subagent corpus). run_in_background and
// dangerouslyDisableSandbox are schema-declared but NEVER appear in any
// observed input (zero corpus usage) — accepted-and-ignored here (documented
// deferred: run_in_background needs the re-invoke machinery + TaskStop;
// no sandbox tier exists to disable — the locked no-confirmation-tier
// safety model has no sandbox to bypass).
type bashArgs struct {
	Command                   string  `json:"command"`
	Description               string  `json:"description"`
	Timeout                   float64 `json:"timeout"`
	RunInBackground           bool    `json:"run_in_background"`
	DangerouslyDisableSandbox bool    `json:"dangerouslyDisableSandbox"` //nolint:tagliatelle // captured input key
}

// resolveBashTimeout maps the model-supplied timeout (ms) to the effective
// bound: absent/zero/negative → the schema-declared 120000 default; above the
// schema max → clamped to 600000; fractional → rounded.
func resolveBashTimeout(ms float64) int {
	if ms <= 0 {
		return bashTimeoutDefaultMS
	}

	if ms > bashTimeoutMaxMS {
		return bashTimeoutMaxMS
	}

	return int(ms)
}

// killGroupOnCtx arms cmd so a timeout/cancel SIGKILLs the child's WHOLE
// process group, not just the direct child — a local re-implementation of
// 08-03's openspec.killGroupOnCtx idiom (Setpgid + kill(-pid, SIGKILL),
// ESRCH-clean), NOT an import: the adapter's env guard + binary default are
// openspec-specific. Without the group kill a wrapper shell's grandchildren
// (sh → sleep, npm → node) survive, hold the output pipes, and stall Wait
// for the child's full runtime.
func killGroupOnCtx(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid = every process in the group; SIGKILL so nothing traps it.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil // group already gone — not a failure
		}

		return fmt.Errorf("coreexec: kill process group %d: %w", cmd.Process.Pid, err)
	}
}

// reapGroup closes the fork-vs-kill race the single Cancel kill leaves open:
// a wrapper shell that forks a child in the SAME process group a hair AFTER
// the SIGKILL is delivered produces a straggler that inherits the pgid but
// never receives the (already-delivered) signal. Looping kill(-pid) until
// ESRCH proves the group EMPTY — the only state with no survivor — bounded
// so a pathological forker cannot pin the executor.
func reapGroup(pid int) {
	deadline := time.Now().Add(reapGroupBudget)

	for {
		err := syscall.Kill(-pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return // group provably empty
		}

		if time.Now().After(deadline) {
			return // bounded — a pathological forker cannot pin the executor
		}

		time.Sleep(reapGroupTick)
	}
}

// reapGroup pacing constants (total worst-case ≈ 300ms past the kill).
const (
	reapGroupBudget = 250 * time.Millisecond
	reapGroupTick   = 25 * time.Millisecond
)

// trimCaptured applies the observed trailing-trim: 0/105 text and 0/2
// error-text captured results end with a newline or any trailing whitespace
// (fixture: Bash.results.success_with_output.trailing_trim_observed) —
// `echo hi` renders as "hi", not "hi\n".
func trimCaptured(s string) string {
	return strings.TrimRight(s, " \t\r\n")
}

// Captured output-truncation constants (12-05 re-record, 2026-08-20,
// sess_6e4b5cc7, zcode 0.16.3 — 19 observations of the envelope). The target
// persists outputs above the inline budget and returns a <persisted-output>
// envelope with a bounded preview. Budget/preview sizes are target-source
// verified (15e3 / 2e3) and capture-consistent (a 135.6KB output persisted;
// preview labelled "first 2KB" = 2000 chars); the exact boundary byte is not
// observation-bracketed (the 08-08 corpus's ~70KB pass predates this render
// path) — flagged in the re-record fixture.
const (
	bashInlineBudget = 15000
	bashPreviewChars = 2000
)

// formatKB renders byte counts in the captured KB form: ÷1024, one decimal,
// a trailing ".0" stripped (138888 → "135.6KB"; 2000 → "2KB").
func formatKB(n int) string {
	s := strconv.FormatFloat(float64(n)/1024, 'f', 1, 64)

	return strings.TrimSuffix(s, ".0") + "KB"
}

// previewFirstChars cuts the preview at a word boundary: the last space before
// the limit when one exists past the halfway point, else the hard limit.
// Returns whether anything was cut (the "..." continuation line renders only
// then).
func previewFirstChars(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}

	cut := limit

	space := strings.LastIndex(s[:limit], " ")
	if space > limit/2 {
		cut = space
	}

	return s[:cut], true
}

// renderPersistedOutput renders the CAPTURED truncation envelope with the
// real saved-to path (the file write happens at the call site):
//
//	<persisted-output>
//	Output too large (<size>KB). Full output saved to: <path>
//
//	Preview (first 2KB):
//	<preview>
//	...
//	</persisted-output>
func renderPersistedOutput(full, savedTo string) string {
	preview, more := previewFirstChars(full, bashPreviewChars)

	var b strings.Builder
	b.WriteString("<persisted-output>\n")
	fmt.Fprintf(&b, "Output too large (%s). Full output saved to: %s\n\n", formatKB(len(full)), savedTo)
	fmt.Fprintf(&b, "Preview (first %s):\n%s\n", formatKB(bashPreviewChars), preview)

	if more {
		b.WriteString("...\n")
	}

	b.WriteString("</persisted-output>")

	return b.String()
}

// persistOversize writes the full output under the session's .ass-guard/
// artifact family (the established write boundary) and returns its path.
func (cfg Config) persistOversize(full string) string {
	dir := filepath.Join(cfg.workDirForError(), ".ass-guard", "outputs")

	err := os.MkdirAll(dir, dirPermWrite)
	if err != nil {
		return dir // unwritable: render the family path anyway (the best real path)
	}

	f, err := os.CreateTemp(dir, "bash-*.log")
	if err != nil {
		return dir
	}
	defer func() { _ = f.Close() }()

	_, werr := f.WriteString(full)
	if werr != nil {
		return f.Name()
	}

	return f.Name()
}

// BashExecute returns the Bash catalog Stub over cfg: it runs the model's
// command via `sh -c` with cmd.Dir = cfg.WorkDir (the session workdir — the
// schema has no cwd field; empty WorkDir inherits), captures stdout+stderr,
// enforces the model-supplied ms timeout (clamped per the schema), and
// renders the CAPTURED result forms:
//
//   - success, output  → the combined output as plain text (trailing-trimmed);
//     above the inline budget → the CAPTURED <persisted-output> envelope with
//     the full output saved under .ass-guard/outputs/ (12-05 re-record)
//   - success, silent  → "(Bash completed with no output)"  (the sentinel)
//   - failure          → "Exit code <N>\n<output>" AND a non-nil error
//     (executeOne sets IsError — the captured 137/137 error shape)
//   - timeout/cancel   → "Command timed out after <duration>\n<error>Command
//     was aborted before completion</error>" AND a non-nil error (the
//     CAPTURED form — 18 observations, 12-05 re-record; replaces the interim
//     structured {"error":…} corpus-absent default)
//
// The locked safety model is unchanged: NO confirmation tier, no allowlists
// (PROJECT.md Constraints — pattern table + manual cancellation are the only
// mechanisms); the bounds are the established ones (mutating alone-in-slot
// D-21, context boundary D-11, per-call timeout + process-group SIGKILL).
func BashExecute(cfg Config) toolcat.Stub {
	return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		var a bashArgs
		// Best-effort parse per the schema's surface: unknown fields are
		// ignored; an empty command runs `sh -c ""` → exit 0 → the sentinel.
		_ = json.Unmarshal(args, &a)

		timeoutMS := resolveBashTimeout(a.Timeout)

		tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
		defer cancel()

		// T-8-33 accepted-and-bound: the model's command IS the input (the
		// locked no-confirmation-tier safety model — no allowlist by design).
		//nolint:gosec // T-8-33: model-authored command is the product
		cmd := exec.CommandContext(tctx, "sh", "-c", a.Command)

		killGroupOnCtx(cmd)

		if cfg.WorkDir != "" {
			cmd.Dir = cfg.WorkDir
		}

		var stdout, stderr bytes.Buffer

		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		// nil Stdin: reads hit immediate EOF (prompt-hang guard).

		waitErr := cmd.Start()
		if waitErr == nil {
			waitErr = cmd.Wait()
		}

		// Close the fork-vs-kill straggler race on the timed-out path (see
		// reapGroup) before the executor returns.
		if tctx.Err() != nil && cmd.Process != nil {
			reapGroup(cmd.Process.Pid)
		}

		// Captured combination order: stdout before stderr (fixture-pinned).
		combined := trimCaptured(stdout.String() + stderr.String())

		if waitErr != nil {
			return bashFailure(tctx, waitErr, timeoutMS, combined)
		}

		if combined == "" {
			return json.Marshal(bashSentinel)
		}

		// The CAPTURED oversize envelope (12-05 re-record): outputs above the
		// inline budget persist in full and return the bounded preview form.
		if len(combined) > bashInlineBudget {
			savedTo := cfg.persistOversize(combined)

			return json.Marshal(renderPersistedOutput(combined, savedTo))
		}

		return json.Marshal(combined)
	}
}

// bashFailure renders the failure forms for a finished command (extracted
// from BashExecute to keep its nesting bounded): the corpus-absent structured
// timeout form, the CAPTURED `Exit code <N>` form, and the corpus-absent
// structured start-failure form.
func bashFailure(tctx context.Context, waitErr error, timeoutMS int, combined string) (json.RawMessage, error) {
	if tctx.Err() != nil {
		// The CAPTURED timeout form (12-05 re-record, 18 observations):
		// plain text, non-nil error → IsError (the 137/137 error discipline).
		text := "Command timed out after " +
			(time.Duration(timeoutMS) * time.Millisecond).String() +
			"\n<error>Command was aborted before completion</error>"

		out, mErr := json.Marshal(text)
		if mErr != nil {
			return nil, fmt.Errorf("coreexec: marshal bash timeout form: %w", mErr)
		}

		return out, fmt.Errorf("coreexec: bash: timed out after %s: %w",
			(time.Duration(timeoutMS) * time.Millisecond), tctx.Err())
	}

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		// The captured error form, byte-pinned to the fixture.
		out, mErr := json.Marshal(bashExitPrefix + strconv.Itoa(exitErr.ExitCode()) + "\n" + combined)
		if mErr != nil {
			return nil, fmt.Errorf("coreexec: marshal bash exit form: %w", mErr)
		}

		//nolint:err113 // the exit code IS the message
		return out, fmt.Errorf("coreexec: bash: exit code %d", exitErr.ExitCode())
	}

	// Start failure (missing shell, permission, …) — corpus-absent.
	return marshalStructured("bash: "+waitErr.Error(), fmt.Errorf("coreexec: bash: %w", waitErr))
}

// marshalStructured renders the {"error":…} convention with the given Go
// error (IsError) carried alongside.
func marshalStructured(msg string, err error) (json.RawMessage, error) {
	out, mErr := json.Marshal(map[string]string{keyError: msg})
	if mErr != nil {
		return nil, fmt.Errorf("coreexec: %w (marshal failed: %w)", err, mErr)
	}

	return out, err
}
