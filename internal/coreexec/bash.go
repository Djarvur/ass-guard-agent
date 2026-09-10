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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/sandbox"
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

// Timeout bounds (SCHEMA-DECLARED in coretools.json's Bash description:
// "timeout is in milliseconds: default 120000, max 600000" — the corpus
// itself shows only explicit 60000–660000 ms values in the subagent sessions
// and omits the field entirely in the main session, so the DEFAULT is
// corpus-informed via the schema, flagged in the fixture's corpus_absent).
const (
	bashTimeoutDefaultMS = 120000
	bashTimeoutMaxMS     = 600000
)

// wrapSandboxCmd is the sandbox wrap seam (22-06, SAND-01): every Bash-class
// exec site calls THIS var — the foreground site below, TaskRegistry's
// launch, and the PTY shell spawn — through sandbox's ONE WrapCmd entry (no
// call site branches on GOOS or re-renders profiles, D-06). Var so the
// batteries can observe/intercept the wrap decision; the live confinement
// batteries run the real entry.
//
//nolint:gochecknoglobals // the testable-seam var pattern (pickResumeSession precedent)
var wrapSandboxCmd = sandbox.WrapCmd

// unconfinedRuns counts every run that executed UNCONFINED while the sandbox
// was ON (22-06, SAND-01's audit counter; process-wide — every per-run note
// carries its number, so no unconfined run is silently absorbed into the log
// stream).
//
//nolint:gochecknoglobals // the process-wide audit counter
var unconfinedRuns atomic.Int64

// sandboxHandleEnabled reports whether the operator asked for confinement
// (the shared gate for all three exec sites): a nil Handle (bare configs),
// Availability.Mode "off" (the default and the explicit refusal), and the
// zero-value Mode "" (never resolved through the flag path) are the
// untouched paths — argv byte-identical, zero notes (SAND-01's default-OFF
// contract: confined only when the operator asked, and asking always
// resolves a backend mode: landlock | seatbelt).
func sandboxHandleEnabled(h *sandbox.Handle) bool {
	return h != nil && h.Availability.Mode != "off" && h.Availability.Mode != ""
}

// sandboxEnabled is Config's gate (see sandboxHandleEnabled).
func (cfg Config) sandboxEnabled() bool {
	return sandboxHandleEnabled(cfg.Sandbox)
}

// noteUnconfinedRun emits ONE loud per-run note with the running counter —
// the never-silent-fail-open contract (a green tool result must never imply
// confinement that did not happen). A nil sink falls back to stderr: the
// note is never droppable.
func noteUnconfinedRun(sink func(format string, args ...any), site, reason string) {
	line := fmt.Sprintf("ass-guard: %s: sandbox ON but run UNCONFINED #%d (%s)",
		site, unconfinedRuns.Add(1), reason)

	if sink != nil {
		sink("%s", line)

		return
	}

	fmt.Fprintln(os.Stderr, line) // never silent (SAND-01)
}

// noteUnconfined is Config's note (see noteUnconfinedRun).
func (cfg Config) noteUnconfined(site, reason string) {
	noteUnconfinedRun(cfg.SandboxNote, site, reason)
}

// confineForeground applies the 22-06 foreground-site sandbox policy to cmd:
// enabled+available+not-disabled → the ONE WrapCmd entry (substitution-only —
// Dir/SysProcAttr stay the caller's, so the group discipline below operates
// on the wrapped child exactly as on the bare sh, D-05); the disable escape
// and the unavailable degrade run UNCONFINED with the loud note + counter
// (OQ2 and the plan prohibition — an unconfined arm is a successful degrade,
// never an error). Default OFF: untouched.
func (cfg Config) confineForeground(cmd *exec.Cmd, disable bool) error {
	if !cfg.sandboxEnabled() {
		return nil
	}

	switch {
	case disable:
		cfg.noteUnconfined("bash", "dangerouslyDisableSandbox requested by the model")
	case !cfg.Sandbox.Availability.Available:
		cfg.noteUnconfined("bash", "sandbox unavailable: "+cfg.Sandbox.Availability.Reason)
	default:
		return wrapSandboxCmd(cmd, cfg.Sandbox.Policy)
	}

	return nil
}

// bashArgs is the observed input shape: {command, description} always;
// timeout (ms) sometimes (subagent corpus). run_in_background EXECUTES via
// the TaskRegistry (12-06; first live corpus usage recorded by the 12-05
// re-record). dangerouslyDisableSandbox is the CAPTURED per-call escape,
// honored since 22-06 (OQ2 — the 12-06 no-op doc retired): with the sandbox
// ON it runs the command UNCONFINED with exactly ONE loud stderr note +
// counter (the operator enabled confinement; the model's explicit escape is
// visible, never silent — the captured contract, CC parity). With the
// sandbox OFF (the default) it stays the documented no-op: there is nothing
// to escape.
type bashArgs struct {
	Command                   string  `json:"command"`
	Description               string  `json:"description"`
	Timeout                   float64 `json:"timeout"`
	RunInBackground           bool    `json:"run_in_background"`
	DangerouslyDisableSandbox bool    `json:"dangerouslyDisableSandbox"` //nolint:tagliatelle // captured input key
	// Persistent (22-04, D-09): per-call opt-in to the session's persistent
	// PTY shell — the ADDITIVE schema property (the OQ1 waiver of the
	// byte-identical catalog discipline, operator-sanctioned by D-09).
	Persistent bool `json:"persistent"`
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

		// 12-06 (ACP-06): the background branch — Start instead of Wait (the
		// registry owns the process from here); the CAPTURED immediate form.
		// 22-04 (D-09): run_in_background WINS over persistent when both are
		// set — a background task is inherently fire-and-forget; stateful
		// foreground work is the persistent contract. 22-06 (OQ2): the
		// per-call escape rides the task into BOTH launch paths.
		if a.RunInBackground {
			return bashBackgroundStart(cfg, a.Command, a.DangerouslyDisableSandbox)
		}

		// 22-04 (PAR-09, D-09): the per-call persistent branch — the session's
		// ONE lazily-started PTY shell (cwd/env persist across persistent
		// calls; D-07). Empty command → the structured error BEFORE any shell
		// interaction (the empty probe).
		//
		// 22-06 (OQ2, the persistent arm): a per-call escape CANNOT unconfine
		// the SHARED shell — other calls' confinement is not this call's to
		// weaken (D-09: persistence is never bought with another call's
		// boundary). With the sandbox ON, the escape falls through to the
		// foreground machinery below: the command runs one-shot UNCONFINED
		// with the loud note (confineForeground), and the persistent shell
		// keeps its own confinement untouched.
		if a.Persistent && !(cfg.sandboxEnabled() && a.DangerouslyDisableSandbox) {
			return bashPersistentRun(ctx, cfg, a.Command, timeoutMS)
		}

		tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
		defer cancel()

		// T-8-33 accepted-and-bound: the model's command IS the input (the
		// locked no-confirmation-tier safety model — no allowlist by design).
		//nolint:gosec // T-8-33: model-authored command is the product
		cmd := exec.CommandContext(tctx, "sh", "-c", a.Command)

		// 22-06 (SAND-01): the FOREGROUND sandbox site — one of exactly THREE
		// Bash-class exec sites that confine through sandbox's ONE WrapCmd
		// entry (this site, TaskRegistry.launch, PTYManager.ensureShell; a
		// fourth exec site added without the wrap is a review-gate violation —
		// the checker-identified silent-gap class this plan closes). The wrap
		// substitutes argv BEFORE the group discipline so killGroupOnCtx/
		// reapGroup own the WRAPPED child exactly as the bare sh (D-05:
		// signals are neither FS nor network operations; landlock rulesets do
		// not intercept them).
		if werr := cfg.confineForeground(cmd, a.DangerouslyDisableSandbox); werr != nil {
			return marshalStructured("bash: "+werr.Error(), fmt.Errorf("coreexec: bash: %w", werr))
		}

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

// bashBackgroundStart is BashExecute's run_in_background branch (12-06,
// ACP-06; 22-02 D-11): registry Start + the CAPTURED immediate-return form,
// or the QUEUED visible-note form when the D-11 cap defers the launch. The
// 22-06 OQ2 escape rides the task (StartWithOpts) so BOTH launch paths honor
// it identically.
func bashBackgroundStart(cfg Config, command string, disableSandbox bool) (json.RawMessage, error) {
	if cfg.Tasks == nil {
		return marshalStructured("bash: run_in_background: no task registry configured", errNoRegistry)
	}

	id, queued, serr := cfg.Tasks.StartWithOpts(cfg.workDirForError(), command,
		StartOpts{DisableSandbox: disableSandbox})
	if serr != nil {
		return marshalStructured(serr.Error(), serr)
	}

	logPath := taskLogPath(cfg.workDirForError(), id)

	form := renderBackgroundStart(id, logPath)
	if queued {
		form = renderBackgroundQueued(id, logPath, cfg.Tasks.QueuedPosition(id), cfg.Tasks.CapValue())
	}

	out, mErr := json.Marshal(form)
	if mErr != nil {
		return nil, fmt.Errorf("coreexec: marshal background start form: %w", mErr)
	}

	return out, nil
}

// errNoPTYManager is the persistent branch's bare-config degrade (no manager
// constructed — the foreground path is unaffected).
var errNoPTYManager = errors.New("coreexec: persistent: no PTY manager configured")

// bashPersistentRun is BashExecute's persistent branch (22-04, PAR-09):
// the session's ONE PTY shell via Config.PTY. An empty/whitespace command
// returns the structured error BEFORE any shell interaction (the empty
// probe — no sentinel written, cwd/env unmoved); a nil manager (bare
// configs) degrades to the structured no-manager error. The command's exit
// code parses from the sentinel's $? — a nonzero command RAN and failed
// (the code renders in the result, not a Go error). The schema-declared
// timeout bounds the call window like every other Bash invocation; an
// interrupted window marks the shell dead — the NEXT persistent call
// restarts it lazily with the visible state-loss note (D-08).
func bashPersistentRun(ctx context.Context, cfg Config, command string, timeoutMS int) (json.RawMessage, error) {
	if strings.TrimSpace(command) == "" {
		return marshalStructured("bash: persistent: empty command", errBadTaskInput)
	}

	if cfg.PTY == nil {
		return marshalStructured("bash: persistent: no PTY manager configured", errNoPTYManager)
	}

	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	output, exitCode, rerr := cfg.PTY.Run(tctx, command)
	if rerr != nil {
		return marshalStructured("bash: persistent: "+rerr.Error(), rerr)
	}

	if exitCode != 0 {
		out, mErr := json.Marshal(bashExitPrefix + strconv.Itoa(exitCode) + "\n" + output)
		if mErr != nil {
			return nil, fmt.Errorf("coreexec: marshal persistent failure form: %w", mErr)
		}

		return out, nil
	}

	if output == "" {
		return json.Marshal(bashSentinel) // the captured silent-success form
	}

	out, mErr := json.Marshal(output)
	if mErr != nil {
		return nil, fmt.Errorf("coreexec: marshal persistent result: %w", mErr)
	}

	return out, nil
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
