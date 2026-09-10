package coreexec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The persistent-shell battery (22-04 Task 1, PAR-09; D-07/D-09): ONE lazy
// PTY per session — cwd and exported env persist across persistent calls,
// completion is the per-call NONCE sentinel (lookalike output cannot end the
// capture early), capture is ANSI-stripped, and the exit code parses from
// the sentinel's $?. The Bash-level rows pin the per-call opt-in (D-09) and
// the structured-error forms.

// ptyRunOK runs one persistent call and fatals on anything but success.
func ptyRunOK(t *testing.T, m *PTYManager, command string) string {
	t.Helper()

	out, code, err := m.Run(context.Background(), command)
	if err != nil {
		t.Fatalf("Run(%q) error = %v", command, err)
	}

	if code != 0 {
		t.Fatalf("Run(%q) exit = %d; want 0 (output %q)", command, code, out)
	}

	return out
}

// ptyLastLine normalizes pty line endings and returns the last line of the
// command's own output (the capture never trails prompt/echo junk).
func ptyLastLine(s string) string {
	norm := strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
	lines := strings.Split(norm, "\n")

	return lines[len(lines)-1]
}

// decodeStructuredError decodes the {"error":…} structured-error convention.
func decodeStructuredError(t *testing.T, raw json.RawMessage) string {
	t.Helper()

	var m map[string]string

	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("output is not the structured-error form: %v (%s)", err, raw)
	}

	msg, ok := m[keyError]
	if !ok {
		t.Fatalf("structured-error form carries no %q key: %s", keyError, raw)
	}

	return msg
}

// TestPersistentShell_CdPersists (D-07's observable): call 1 cds, call 2's
// bare pwd prints the SAME directory — ONE shell held the state.
func TestPersistentShell_CdPersists(t *testing.T) { //nolint:paralleltest // a real pty shell serializes on the manager
	target := t.TempDir()
	m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
	defer m.Drain()

	ptyRunOK(t, m, "cd \""+target+"\"")

	if got := ptyLastLine(ptyRunOK(t, m, "pwd")); got != target {
		t.Errorf("bare pwd after cd = %q; want %q — cwd did not persist", got, target)
	}
}

// TestPersistentShell_ExportedEnvPersists: an export in call 1 is visible to
// call 2's printenv.
func TestPersistentShell_ExportedEnvPersists(t *testing.T) { //nolint:paralleltest // a real pty shell serializes on the manager
	m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
	defer m.Drain()

	ptyRunOK(t, m, "export ASS_GUARD_PERSIST_PROBE=1")

	if got := ptyLastLine(ptyRunOK(t, m, "printenv ASS_GUARD_PERSIST_PROBE")); got != "1" {
		t.Errorf("printenv after export = %q; want 1 — env did not persist", got)
	}
}

// TestPersistentShell_SentinelLookalikeOutputDoesNotTerminateEarly (the
// PAR-09 adjacency probe): command output containing the literal
// __ASS_GUARD_DONE_ text is just output — only the full nonce line ends the
// capture window.
func TestPersistentShell_SentinelLookalikeOutputDoesNotTerminateEarly(t *testing.T) { //nolint:paralleltest // a real pty shell serializes on the manager
	m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
	defer m.Drain()

	out := ptyRunOK(t, m, "echo __ASS_GUARD_DONE_lookalike_99__; echo marker-after")

	if !strings.Contains(out, "__ASS_GUARD_DONE_lookalike_99__") {
		t.Errorf("lookalike text missing from capture: %q", out)
	}

	if !strings.Contains(out, "marker-after") {
		t.Errorf("capture ended before the command finished (no marker-after): %q", out)
	}
}

// TestPersistentShell_CapturedOutputIsAnsiStripped (the transport-discipline
// corollary): a command emitting SGR color returns plain text with ZERO ESC
// bytes.
func TestPersistentShell_CapturedOutputIsAnsiStripped(t *testing.T) { //nolint:paralleltest // a real pty shell serializes on the manager
	m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
	defer m.Drain()

	out := ptyRunOK(t, m, "printf '\\033[31mred\\033[0m\\n'")

	if strings.ContainsRune(out, '\x1b') {
		t.Errorf("capture still carries ESC bytes: %q", out)
	}

	if got := ptyLastLine(out); got != "red" {
		t.Errorf("stripped output = %q; want red", got)
	}
}

// TestPersistentShell_ExitCodeParsesFromSentinel: a nonzero command RAN and
// failed — the code parses from the sentinel's $? (no Go error; the shell
// stays alive).
func TestPersistentShell_ExitCodeParsesFromSentinel(t *testing.T) { //nolint:paralleltest // a real pty shell serializes on the manager
	m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
	defer m.Drain()

	out, code, err := m.Run(context.Background(), "(exit 7)")
	if err != nil {
		t.Fatalf("Run((exit 7)) error = %v; the command ran and failed — not a Go error", err)
	}

	if code != 7 {
		t.Errorf("Run((exit 7)) exit = %d; want 7 (output %q)", code, out)
	}

	// The shell survived: the next call still answers.
	if got := ptyLastLine(ptyRunOK(t, m, "echo alive")); got != "alive" {
		t.Errorf("shell did not survive a nonzero command; echo alive = %q", got)
	}
}

// TestPersistentShell_SilentCommandReturnsCleanOutput: a command with no
// output returns the empty string — echoed-command noise never reaches the
// tool result.
func TestPersistentShell_SilentCommandReturnsCleanOutput(t *testing.T) { //nolint:paralleltest // a real pty shell serializes on the manager
	m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
	defer m.Drain()

	if out := ptyRunOK(t, m, "true"); out != "" {
		t.Errorf("silent command output = %q; want empty", out)
	}
}

// TestPersistentShell_BashToolCdPersistsAcrossCalls: the per-call opt-in
// through the FULL Bash stub — persistent:true calls share the session
// shell; a silent call renders the captured sentinel form.
func TestPersistentShell_BashToolCdPersistsAcrossCalls(t *testing.T) { //nolint:paralleltest // a real pty shell serializes on the manager
	workDir := t.TempDir()
	target := t.TempDir()

	mgr := NewPTYManager(PTYOpts{WorkDir: workDir})
	defer mgr.Drain()

	stub := BashExecute(Config{WorkDir: workDir, PTY: mgr})

	out1, err := stub(context.Background(), bashInput(t, "cd \""+target+"\"", true))
	if err != nil {
		t.Fatalf("persistent cd error: %v", err)
	}

	if got := decodeJSONString(t, out1); got != bashSentinel {
		t.Errorf("silent persistent call = %q; want the captured sentinel %q", got, bashSentinel)
	}

	out2, err := stub(context.Background(), bashInput(t, "pwd", true))
	if err != nil {
		t.Fatalf("persistent pwd error: %v", err)
	}

	if got := ptyLastLine(decodeJSONString(t, out2)); got != target {
		t.Errorf("pwd after persistent cd = %q; want %q — the tool-level state did not persist", got, target)
	}
}

// TestPersistentShell_BashToolEmptyCommandIsStructuredError (the PAR-09
// empty probe): an empty/whitespace persistent command returns the
// structured error BEFORE any shell interaction — a follow-up pwd proves no
// sentinel was written and no state moved.
func TestPersistentShell_BashToolEmptyCommandIsStructuredError(t *testing.T) { //nolint:paralleltest // a real pty shell serializes on the manager
	workDir := t.TempDir()
	target := t.TempDir()

	mgr := NewPTYManager(PTYOpts{WorkDir: workDir})
	defer mgr.Drain()

	stub := BashExecute(Config{WorkDir: workDir, PTY: mgr})

	if _, err := stub(context.Background(), bashInput(t, "cd \""+target+"\"", true)); err != nil {
		t.Fatalf("persistent cd error: %v", err)
	}

	out, err := stub(context.Background(), bashInput(t, "   ", true))
	if err == nil {
		t.Fatal("whitespace-only persistent command must surface a non-nil error")
	}

	if msg := decodeStructuredError(t, out); !strings.Contains(msg, "empty") {
		t.Errorf("structured error = %q; want it to name the empty command", msg)
	}

	// State unmoved: the empty call wrote nothing to the shell.
	outAfter, err := stub(context.Background(), bashInput(t, "pwd", true))
	if err != nil {
		t.Fatalf("pwd after empty command error: %v", err)
	}

	if got := ptyLastLine(decodeJSONString(t, outAfter)); got != target {
		t.Errorf("pwd after rejected empty command = %q; want %q — the empty call moved the shell state", got, target)
	}
}

// TestPersistentShell_BashToolNilManagerIsStructuredError: a bare Config
// (no PTY manager) degrades to the structured no-manager error.
func TestPersistentShell_BashToolNilManagerIsStructuredError(t *testing.T) {
	t.Parallel()

	stub := BashExecute(Config{WorkDir: t.TempDir()})

	out, err := stub(context.Background(), json.RawMessage(`{"command":"pwd","persistent":true}`))
	if err == nil {
		t.Fatal("persistent call without a manager must surface a non-nil error")
	}

	if msg := decodeStructuredError(t, out); !strings.Contains(msg, "no PTY manager") {
		t.Errorf("structured error = %q; want the no-PTY-manager form", msg)
	}
}

// TestPersistentShell_BashToolNonzeroExitRendersCode: a failed persistent
// command renders the captured `Exit code <N>` form (the command RAN; it
// failed — not a Go error).
func TestPersistentShell_BashToolNonzeroExitRendersCode(t *testing.T) { //nolint:paralleltest // a real pty shell serializes on the manager
	workDir := t.TempDir()
	mgr := NewPTYManager(PTYOpts{WorkDir: workDir})
	defer mgr.Drain()

	stub := BashExecute(Config{WorkDir: workDir, PTY: mgr})

	out, err := stub(context.Background(), bashInput(t, "(exit 7)", true))
	if err != nil {
		t.Fatalf("(exit 7) error = %v; the command ran and failed — not a Go error", err)
	}

	if got := decodeJSONString(t, out); !strings.HasPrefix(got, "Exit code 7") {
		t.Errorf("nonzero persistent form = %q; want the `Exit code 7` prefix", got)
	}
}

// TestPersistentShell_NonPersistentCallsStayStateless (D-09): WITHOUT the
// persistent flag the default sh -c path is untouched — cd does NOT stick.
func TestPersistentShell_NonPersistentCallsStayStateless(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	target := t.TempDir()

	mgr := NewPTYManager(PTYOpts{WorkDir: workDir})
	defer mgr.Drain() // no-op by construction: no persistent call ever starts the shell

	stub := BashExecute(Config{WorkDir: workDir, PTY: mgr})

	if _, err := stub(context.Background(), bashInput(t, "cd \""+target+"\"", false)); err != nil {
		t.Fatalf("plain cd error: %v", err)
	}

	out, err := stub(context.Background(), bashInput(t, "pwd", false))
	if err != nil {
		t.Fatalf("plain pwd error: %v", err)
	}

	if got := ptyLastLine(decodeJSONString(t, out)); got == target {
		t.Errorf("non-persistent pwd = %q — state leaked across stateless calls (D-09 broken)", got)
	}
}

// jsonString renders a JSON string literal (test-command embedding).
func jsonString(s string) string {
	b, _ := json.Marshal(s)

	return string(b)
}

// bashInput marshals a Bash tool input for the battery.
func bashInput(t *testing.T, command string, persistent bool) json.RawMessage {
	t.Helper()

	b, err := json.Marshal(map[string]any{"command": command, "persistent": persistent})
	if err != nil {
		t.Fatalf("marshal bash input: %v", err)
	}

	return b
}
