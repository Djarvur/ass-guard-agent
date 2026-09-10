package coreexec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/sandbox"
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

// --- 22-04 Task 2 (D-08, PAR-09 ordering/empty probes): dead-shell lazy
// restart with the visible state-loss note, interrupted-read promptness,
// arrival-order serialization with per-caller attribution, and the
// empty-command edge.

// waitForFalse polls cond until true or the deadline fatals.
func waitForFalse(t *testing.T, what string, cond func() bool, deadline time.Duration) {
	t.Helper()

	end := time.Now().Add(deadline)

	for cond() {
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// TestPTYDeadRestart (D-08): an externally-killed shell never wedges the
// session — the NEXT persistent call restarts it lazily, succeeds, and the
// note fires exactly once naming the state loss; a call already blocked in
// the read loop when the shell dies returns promptly with the interrupted
// outcome (never a hang).
func TestPTYDeadRestart(t *testing.T) { //nolint:paralleltest // real pty shells, kill timing
	t.Run("RestartSucceedsWithNoteOnce", func(t *testing.T) { //nolint:paralleltest // real shell
		workDir := t.TempDir()
		target := t.TempDir()

		var notes []string
		m := NewPTYManager(PTYOpts{WorkDir: workDir, NoteFn: func(format string, args ...any) {
			notes = append(notes, fmt.Sprintf(format, args...))
		}})
		defer m.Drain()

		ptyRunOK(t, m, "cd \""+target+"\"")

		pid := m.ShellPID()
		if pid == 0 {
			t.Fatal("no shell pid after the first call")
		}

		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			t.Fatalf("external kill: %v", err)
		}

		waitForFalse(t, "the killed shell to be observed dead", m.Alive, 5*time.Second)

		// THE next call: restarts lazily, SUCCEEDS, and the fresh shell has
		// LOST the cd (pwd answers the manager's workdir again).
		out := ptyRunOK(t, m, "pwd")

		if got := ptyLastLine(out); got != workDir {
			t.Errorf("pwd after restart = %q; want %q — the restarted shell must start fresh (state lost)", got, workDir)
		}

		if len(notes) != 1 {
			t.Fatalf("restart notes = %d (%v); want exactly 1", len(notes), notes)
		}

		if msg := notes[0]; !strings.Contains(msg, "state") || !strings.Contains(msg, "lost") {
			t.Errorf("note = %q; want the state-loss acknowledgment wording", msg)
		}

		// A healthy follow-up call fires NO further note.
		ptyRunOK(t, m, "echo steady")

		if len(notes) != 1 {
			t.Errorf("notes after a healthy follow-up = %d (%v); still want 1", len(notes), notes)
		}
	})

	t.Run("BlockedReadReturnsPromptly", func(t *testing.T) { //nolint:paralleltest // real shell + kill timing
		m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
		defer m.Drain()

		ptyRunOK(t, m, "echo warmup")

		pid := m.ShellPID()

		type callRes struct {
			out string
			err error
		}

		done := make(chan callRes, 1)

		go func() {
			out, _, err := m.Run(context.Background(), "sleep 30")
			done <- callRes{out: out, err: err}
		}()

		// Let the command provably start, then kill the shell mid-read.
		time.Sleep(500 * time.Millisecond)

		start := time.Now()

		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			t.Fatalf("external kill mid-read: %v", err)
		}

		select {
		case res := <-done:
			elapsed := time.Since(start)

			if elapsed > 3*time.Second {
				t.Errorf("interrupted call took %v to return; want prompt (EIO-as-EOF must unblock the read)", elapsed)
			}

			if res.err == nil {
				t.Fatal("interrupted call must surface the dead-shell outcome as an error")
			}

			if res.err.Error() == "" || !strings.Contains(res.err.Error(), "shell exited") {
				t.Errorf("interrupted outcome = %v; want it to name the shell exit", res.err)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("call blocked in the read loop never returned after shell death — wedged")
		}

		// The NEXT call restarts lazily (D-08) and succeeds.
		if got := ptyLastLine(ptyRunOK(t, m, "echo recovered")); got != "recovered" {
			t.Errorf("post-death recovery call = %q; want recovered", got)
		}
	})
}

// TestPTYSerialize (D-07, the PAR-09 ordering probe): concurrent persistent
// calls serialize in arrival order — outputs never interleave (B never lands
// between A's two appends) and each caller's capture is attributable to
// exactly its own command; the shell is lazy (nothing runs before the first
// persistent call — D-07).
func TestPTYSerialize(t *testing.T) { //nolint:paralleltest // real pty shells, ordering
	t.Run("ArrivalOrderAndAttribution", func(t *testing.T) { //nolint:paralleltest // real shell
		dir := t.TempDir()
		orderFile := filepath.Join(dir, "order.txt")

		m := NewPTYManager(PTYOpts{WorkDir: dir})
		defer m.Drain()

		var wg sync.WaitGroup

		type callRes struct {
			out string
			err error
		}

		resCh := make(chan callRes, 2)

		wg.Add(1)

		go func() { // caller A arrives first
			defer wg.Done()

			out, _, err := m.Run(context.Background(),
				"echo A1 >> "+orderFile+"; sleep 1; echo A2 >> "+orderFile+"; echo A-out")
			resCh <- callRes{out: out, err: err}
		}()

		// A's command is provably executing once its first append lands.
		end := time.Now().Add(10 * time.Second)

		for {
			data, rerr := os.ReadFile(orderFile)

			if rerr == nil && strings.Contains(string(data), "A1") {
				break
			}

			if time.Now().After(end) {
				t.Fatal("caller A never started (no A1 append)")
			}

			time.Sleep(10 * time.Millisecond)
		}

		wg.Add(1)

		go func() { // caller B arrives while A holds the manager
			defer wg.Done()

			out, _, err := m.Run(context.Background(), "echo B >> "+orderFile+"; echo B-out")
			resCh <- callRes{out: out, err: err}
		}()

		wg.Wait()
		close(resCh)

		for res := range resCh {
			if res.err != nil {
				t.Fatalf("concurrent call error: %v", res.err)
			}
		}

		data, rerr := os.ReadFile(orderFile)
		if rerr != nil {
			t.Fatalf("read order file: %v", rerr)
		}

		lines := strings.Fields(string(data))
		want := []string{"A1", "A2", "B"}

		if len(lines) != len(want) {
			t.Fatalf("order file lines = %v; want %v (interleaving or duplication)", lines, want)
		}

		for i := range want {
			if lines[i] != want[i] {
				t.Fatalf("order file lines = %v; want %v — B interleaved into A's window (D-07 broken)", lines, want)
			}
		}
	})

	t.Run("LazyUntilFirstPersistentCall", func(t *testing.T) { //nolint:paralleltest // real shell
		workDir := t.TempDir()
		m := NewPTYManager(PTYOpts{WorkDir: workDir})
		defer m.Drain()

		if m.Alive() {
			t.Error("shell alive at construction — the manager must be lazy (D-07)")
		}

		if m.ShellPID() != 0 {
			t.Errorf("shell pid = %d before any call; want 0", m.ShellPID())
		}

		// Only NON-persistent Bash calls: still zero shells.
		stub := BashExecute(Config{WorkDir: workDir, PTY: m})

		if _, err := stub(context.Background(), bashInput(t, "echo stateless", false)); err != nil {
			t.Fatalf("non-persistent call error: %v", err)
		}

		if m.Alive() {
			t.Error("non-persistent Bash started the persistent shell — D-09 isolation broken")
		}

		if got := ptyLastLine(ptyRunOK(t, m, "echo first")); got != "first" {
			t.Errorf("first persistent call output = %q; want first", got)
		}

		if !m.Alive() {
			t.Error("shell not alive after the first persistent call")
		}
	})
}

// TestPTYEmpty (the PAR-09 empty probe): empty and whitespace-only commands
// are structured errors that touch nothing — the PTY never starts for them
// and an established shell's cwd/env are unchanged afterward.
func TestPTYEmpty(t *testing.T) { //nolint:paralleltest // real pty shells
	t.Run("EmptyCommandsAreStructuredErrors", func(t *testing.T) {
		t.Parallel()

		m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
		defer m.Drain()

		for _, cmd := range []string{"", "   ", "\t"} {
			if _, _, err := m.Run(context.Background(), cmd); err == nil {
				t.Errorf("Run(%q) error = nil; want the structured input error", cmd)
			}
		}

		if m.Alive() {
			t.Error("an empty command started the shell — it must be rejected before any shell interaction")
		}
	})

	t.Run("StateUnchangedAfterEmpty", func(t *testing.T) { //nolint:paralleltest // real shell
		workDir := t.TempDir()
		target := t.TempDir()

		m := NewPTYManager(PTYOpts{WorkDir: workDir})
		defer m.Drain()

		ptyRunOK(t, m, "cd \""+target+"\"")

		if _, _, err := m.Run(context.Background(), "   "); err == nil {
			t.Fatal("whitespace Run error = nil; want the structured input error")
		}

		out := ptyRunOK(t, m, "pwd")
		if got := ptyLastLine(out); got != target {
			t.Errorf("pwd after rejected empty = %q; want %q — the empty call moved the shell state", got, target)
		}

		// No sentinel leaked into the next window either: the output is
		// exactly the pwd, nothing else.
		if out != target {
			t.Errorf("pwd window carries extra output %q; want exactly the path (a stale sentinel leaked)", out)
		}
	})
}

// --- 22-04 Task 3 (D-08, Pitfall 5): session-close drain — TERM→KILL the
// shell's session group, close the master fd, never leak.

// fdCount reports the process's open-descriptor count (linux /proc; ok=false
// on platforms without the probe — the row skips there).
func fdCount() (int, bool) {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0, false
	}

	return len(entries), true
}

// TestPTYDrain: a TERM-sensitive shell dies on TERM (fast — no KILL
// escalation needed); a TERM-immune shell still dies via the escalation
// within the grace window; N start/drain cycles leave the fd count at
// baseline (no master-fd leak, Pitfall 5); Drain on a never-started manager
// is an idempotent no-op.
func TestPTYDrain(t *testing.T) { //nolint:paralleltest // real pty shells, fd baseline
	t.Run("TermSensitiveShellDiesOnTerm", func(t *testing.T) { //nolint:paralleltest // real shell
		m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
		ptyRunOK(t, m, "echo warm")

		if !m.Alive() {
			t.Fatal("shell not alive before Drain")
		}

		start := time.Now()
		m.Drain()

		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("Drain on a TERM-sensitive shell took %v; want the fast TERM path (no full-grace burn)", elapsed)
		}

		if m.Alive() {
			t.Error("Alive() = true after Drain; want false")
		}

		if m.ShellPID() != 0 {
			t.Errorf("ShellPID() = %d after Drain; want 0 (state cleared)", m.ShellPID())
		}
	})

	t.Run("TermImmuneShellDiesViaEscalation", func(t *testing.T) { //nolint:paralleltest // real shell, grace window
		m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
		ptyRunOK(t, m, "trap '' TERM")

		start := time.Now()
		m.Drain()
		elapsed := time.Since(start)

		if !m.Alive() {
			// dead is required; HOW it died is the assertion below
		} else {
			t.Fatal("TERM-immune shell survived Drain — the KILL escalation is broken")
		}

		if elapsed < 3*time.Second {
			t.Errorf("Drain on a TERM-immune shell took %v; want the full-grace escalation path (>= 3s)", elapsed)
		}

		if elapsed > 8*time.Second {
			t.Errorf("Drain took %v; the escalation must stay bounded past the grace", elapsed)
		}
	})

	t.Run("FDCountStableAcrossCycles", func(t *testing.T) { //nolint:paralleltest // fd baseline isolation
		before, ok := fdCount()
		if !ok {
			t.Skip("no /proc/self/fd on this platform — fd leak pinned on linux")
		}

		for i := 0; i < 5; i++ {
			m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})
			ptyRunOK(t, m, "echo cycle")
			m.Drain()
		}

		after, ok := fdCount()
		if !ok {
			t.Skip("fd probe vanished mid-test")
		}

		if after > before+2 {
			t.Errorf("fd count after 5 start/drain cycles = %d; baseline %d — the master fd leaks (Pitfall 5)", after, before)
		}
	})

	t.Run("IdempotentOnNeverStarted", func(t *testing.T) {
		t.Parallel()

		m := NewPTYManager(PTYOpts{WorkDir: t.TempDir()})

		m.Drain() // never started — a no-op, not a panic
		m.Drain() // idempotent

		if m.Alive() {
			t.Error("never-started manager reports Alive after Drain")
		}
	})
}

// --- 22-06 (SAND-01): the persistent-site sandbox batteries ---

// ptySandboxNote returns the PTY opts with a capturing NoteFn (the restart
// note and the unconfined-run note share the loud stderr family).
func ptySandboxNote() (PTYOpts, *[]string) {
	lines := &[]string{}

	return PTYOpts{NoteFn: func(format string, args ...any) {
		*lines = append(*lines, fmt.Sprintf(format, args...))
	}}, lines
}

// runPersistent runs one persistent-mode Bash call through the executor (the
// full branch: args parse → PTY.Run → sentinel parse → result render).
func runPersistent(t *testing.T, cfg Config, command string) (string, error) {
	t.Helper()

	out, err := BashExecute(cfg)(context.Background(),
		json.RawMessage(`{"command":`+strconv.Quote(command)+`,"persistent":true}`))
	if err != nil {
		return "", err
	}

	return decodeJSONString(t, out), nil
}

// TestPTYSandbox_LiveShellConfined: with the sandbox ON and available, the
// persistent shell itself is CONFINED at spawn — a persistent touch OUTSIDE
// the rw set fails (nonzero exit in the result) while a touch INSIDE the
// system tmp succeeds, and the sentinel cycle completes normally both times
// (confinement does not affect the sentinel echo — plain text on the master).
func TestPTYSandbox_LiveShellConfined(t *testing.T) { //nolint:paralleltest // live pty shell + host probe
	workDir := t.TempDir()
	handle := liveSandboxHandle(t, workDir)

	opts, _ := ptySandboxNote()
	opts.WorkDir = workDir
	opts.Sandbox = &handle

	cfg := Config{WorkDir: workDir, PTY: NewPTYManager(opts),
		SandboxNote: func(string, ...any) {}}

	// Inside-tmp write succeeds through the persistent shell.
	out, err := runPersistent(t, cfg, "touch "+os.TempDir()+"/pty-inside-"+fmt.Sprint(os.Getpid()))
	if err != nil {
		t.Fatalf("inside-tmp persistent write errored: %v", err)
	}

	if strings.Contains(out, "Exit code") {
		t.Errorf("inside-tmp persistent write failed in the confined shell: %q", out)
	}

	// Outside write fails (the shell's confinement applies to its commands).
	outside := dirOutsidePolicy(t)

	out, err = runPersistent(t, cfg, "touch "+outside+"/nope")
	if err != nil {
		t.Fatalf("the outside-write persistent call ERRORED (it must complete and render the exit code): %v", err)
	}

	if !strings.Contains(out, "Exit code 1") {
		t.Errorf("outside write SUCCEEDED in the persistent shell — the shell is unconfined: %q", out)
	}

	// The sentinel cycle still completes: a follow-up plain call runs clean.
	out, err = runPersistent(t, cfg, "echo still-alive")
	if err != nil || !strings.Contains(out, "still-alive") {
		t.Errorf("the sentinel cycle broke under confinement: %q (%v)", out, err)
	}
}

// TestPTYSandbox_RestartRewrapsWithNoteOnce: a dead shell's lazy restart
// under --sandbox=on re-wraps the replacement (still confined) and the D-08
// state-loss note fires exactly once — confinement survives restarts.
func TestPTYSandbox_RestartRewrapsWithNoteOnce(t *testing.T) { //nolint:paralleltest // real pty shells, kill timing
	workDir := t.TempDir()
	handle := liveSandboxHandle(t, workDir)

	opts, notes := ptySandboxNote()
	opts.WorkDir = workDir
	opts.Sandbox = &handle

	mgr := NewPTYManager(opts)
	cfg := Config{WorkDir: workDir, PTY: mgr, SandboxNote: func(string, ...any) {}}

	if _, err := runPersistent(t, cfg, "echo first"); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Kill the shell externally; the next call restarts it lazily.
	if pid := mgr.ShellPID(); pid != 0 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && mgr.Alive() {
		time.Sleep(25 * time.Millisecond)
	}

	outside := dirOutsidePolicy(t)

	// The restart's own probe: the replacement shell is still confined.
	out, err := runPersistent(t, cfg, "touch "+outside+"/after-restart")
	if err != nil {
		t.Fatalf("post-restart call errored: %v", err)
	}

	if !strings.Contains(out, "Exit code 1") {
		t.Errorf("the RESTARTED shell is unconfined: %q", out)
	}

	restartNotes := 0
	for _, n := range *notes {
		if strings.Contains(n, "restarted") {
			restartNotes++
		}
	}

	if restartNotes != 1 {
		t.Errorf("restart notes = %d; want exactly ONE (D-08): %v", restartNotes, *notes)
	}
}

// TestPTYSandbox_UnavailableNotedPerRun: enabled-but-unavailable (faked)
// runs each persistent call UNCONFINED (the outside write succeeds) with one
// note PER RUN carrying the counter — never a silent fail-open.
func TestPTYSandbox_UnavailableNotedPerRun(t *testing.T) { //nolint:paralleltest // shared process counter
	workDir := t.TempDir()

	//nolint:exhaustruct // faked availability (the on marker, probe failed)
	fake := &sandbox.Handle{
		Policy:       sandbox.DefaultPolicy(workDir, os.TempDir(), filepath.Join(workDir, ".ass-guard")),
		Availability: sandbox.Availability{Mode: "landlock", Reason: "faked-unavailable-pty"},
	}

	opts, notes := ptySandboxNote()
	opts.WorkDir = workDir
	opts.Sandbox = fake

	cfg := Config{WorkDir: workDir, PTY: NewPTYManager(opts), Sandbox: &sandbox.Handle{
		Policy: fake.Policy, Availability: sandbox.Availability{Mode: "off"},
	}, SandboxNote: func(string, ...any) {}}

	outside := dirOutsidePolicy(t)

	for i := 0; i < 2; i++ {
		out, err := runPersistent(t, cfg, "touch "+outside+"/u"+fmt.Sprint(i))
		if err != nil {
			t.Fatalf("run %d errored (unavailable degrades to unconfined, not fail): %v", i, err)
		}

		if strings.Contains(out, "Exit code") {
			t.Errorf("run %d did NOT run unconfined: %q", i, out)
		}
	}

	sandboxNotes := 0
	for _, n := range *notes {
		if strings.Contains(n, "UNCONFINED") {
			sandboxNotes++
		}
	}

	if sandboxNotes != 2 {
		t.Errorf("unconfined notes = %d; want one PER RUN: %v", sandboxNotes, *notes)
	}
}

// TestPTYSandbox_DefaultOffByteIdentical: with the sandbox off, the shell
// spawn is byte-identical (the wrap seam never fires, zero notes) and a
// would-be-denied write SUCCEEDS — 22-04's batteries pin the rest.
func TestPTYSandbox_DefaultOffByteIdentical(t *testing.T) {
	// NOT t.Parallel: swaps the package wrap seam.
	restore, called := wrapRecorder()
	defer restore()

	opts, notes := ptySandboxNote()
	opts.WorkDir = t.TempDir()

	cfg := Config{WorkDir: opts.WorkDir, PTY: NewPTYManager(opts),
		SandboxNote: func(string, ...any) {}}

	outside := dirOutsidePolicy(t)

	out, err := runPersistent(t, cfg, "touch "+outside+"/off-ok")
	if err != nil {
		t.Fatalf("off-path persistent call errored: %v", err)
	}

	if strings.Contains(out, "Exit code") {
		t.Errorf("the off-path write failed (must be unconfined): %q", out)
	}

	if *called {
		t.Error("the wrap seam fired on the off path — ensureShell must stay byte-identical")
	}

	if len(*notes) != 0 {
		t.Errorf("the off path emitted notes: %v", *notes)
	}
}

// TestPTYSandbox_DisableEscapeRunsUnconfinedShellStaysConfined: the OQ2
// persistent arm — a persistent call with dangerouslyDisableSandbox under
// --sandbox=on runs UNCONFINED with the loud note, while the SHARED shell
// stays confined (the next persistent call without the escape still cannot
// write outside — one call's escape never weakens another's confinement).
func TestPTYSandbox_DisableEscapeRunsUnconfinedShellStaysConfined(t *testing.T) { //nolint:funlen,paralleltest // live probe + shared shell
	workDir := t.TempDir()
	handle := liveSandboxHandle(t, workDir)

	opts, _ := ptySandboxNote()
	opts.WorkDir = workDir
	opts.Sandbox = &handle

	notes, lines := captureNotes()

	cfg := Config{WorkDir: workDir, PTY: NewPTYManager(opts), Sandbox: &handle, SandboxNote: notes}

	outside := dirOutsidePolicy(t)

	// The escape call: unconfined (the outside write succeeds) + one note.
	out, err := BashExecute(cfg)(context.Background(), json.RawMessage(
		`{"command":"touch `+outside+`/escaped","persistent":true,"dangerouslyDisableSandbox":true}`))
	if err != nil {
		t.Fatalf("the escape call errored: %v", err)
	}

	if got := decodeJSONString(t, out); strings.Contains(got, "Exit code") {
		t.Errorf("the escape call did not run unconfined: %q", got)
	}

	if len(*lines) != 1 || !strings.Contains((*lines)[0], "UNCONFINED") {
		t.Errorf("escape notes = %v; want exactly ONE loud note", *lines)
	}

	// The shared shell stays confined: a follow-up persistent call without
	// the escape still cannot write outside.
	followUp, ferr := runPersistent(t, cfg, "touch "+outside+"/still-confined")
	if ferr != nil {
		t.Fatalf("the follow-up call errored: %v", ferr)
	}

	if !strings.Contains(followUp, "Exit code 1") {
		t.Errorf("the SHARED shell was weakened by the escape call: %q", followUp)
	}
}
