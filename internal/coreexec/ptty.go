package coreexec

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// The per-session persistent-shell PTY manager (22-04, PAR-09, D-07/D-08/
// D-09): ONE lazily-started shell per session — all persistent Bash calls
// share it, cd/export persist, concurrent calls serialize on the mutex.
// Completion is a per-call NONCE sentinel echo (a command whose output
// contains sentinel-looking text cannot end the capture early — the adjacency
// probe); capture is ANSI-stripped (the transport-discipline corollary: PTY
// output never touches stdout — it flows only to the tool result and the
// bounded capture buffer); EIO on the master reads as clean EOF (linux;
// darwin returns EOF — both unblock the read loop, 22-RESEARCH Pitfall 4);
// a dead shell restarts LAZILY on the next call with a visible NoteFn (D-08
// — never silent).
//
// Shell shape (the executor's deliberate refinement over the research
// sketch): the shell's STDIN is a pipe held by the manager (commands and the
// sentinel ride it), while STDOUT/STDERR are the pty SLAVE — a real PTY on
// the output side (programs see a tty: colorize, ANSI, EIO-on-exit) with
// NONE of an interactive shell's artifacts: no tty-echo of typed commands,
// no PS1 prompt, no rc sourcing — and SIGTERM keeps its default disposition
// (an interactive shell IGNORES TERM, which would make Drain's first ladder
// rung dead code and burn the full grace on every close — verified live on
// linux: interactive dash stays in state S through the whole 5s window).
//
// Lifecycle ownership (the background.go contract): the MANAGER — not a call
// ctx — owns the shell; Drain (the OnClose link) TERM→KILLs the session
// group and closes the master fd (Pitfall 5's no-leak letter).

// ptyTermGrace is Drain's TERM→KILL grace (the 22-02 ladder values).
const ptyTermGrace = 5 * time.Second

// ptyReadTimeout bounds each master read slice so a wedged read cannot pin
// the loop between ctx re-checks (EIO/EOF unblocks a dead shell immediately;
// this covers only pathological mid-read hangs).
const ptyReadTimeout = 2 * time.Second

// PTYOpts configures the manager.
type PTYOpts struct {
	WorkDir string
	// NoteFn emits the dead-shell restart note (D-08's visible
	// state-loss acknowledgment); nil drops the note (bare tests).
	NoteFn func(format string, args ...any)
}

// PTYManager owns the session's ONE persistent shell.
type PTYManager struct {
	opts PTYOpts
	mu   sync.Mutex

	ptmx   io.ReadWriteCloser // the pty master (nil = never started / drained / dead)
	stdinW io.WriteCloser     // the shell's command-input pipe (write side)
	cmd    *exec.Cmd
	// started distinguishes "never launched" (no note on first start) from
	// "launched, then died" (D-08's note fires on the lazy restart).
	started bool
	// shellDead is set by the reaper goroutine when THIS generation's shell
	// exited (generation-guarded: a stale reaper cannot mark the successor).
	shellDead bool
	gen       uint64
}

// NewPTYManager returns a lazy manager — no shell process exists until the
// first persistent call (D-07).
func NewPTYManager(opts PTYOpts) *PTYManager {
	return &PTYManager{opts: opts}
}

// Alive reports whether the shell process is currently running (the test and
// drain-state lens; race-free — it consults the reaper's flag, never
// cmd.ProcessState).
func (m *PTYManager) Alive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.ptmx != nil && m.cmd != nil && !m.shellDead
}

// ShellPID exposes the shell's pid (tests kill it externally; Drain uses the
// group).
func (m *PTYManager) ShellPID() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil || m.cmd.Process == nil {
		return 0
	}

	return m.cmd.Process.Pid
}

// ensureShell lazily starts the session shell (callers hold m.mu): the pty
// pair (24x80) carries the shell's combined output; a pipe carries its
// command input; Setsid makes it a session + group leader (kill(-pid)
// reaches its whole group). The environment pins the non-interactive
// contract (empty PS1/PS2; ENV/BASH_ENV pointed at /dev/null — no rc
// sourcing, no prompt, TERM default).
func (m *PTYManager) ensureShell() error {
	if m.ptmx != nil && !m.shellDead {
		return nil
	}

	ptmx, tty, err := pty.Open()
	if err != nil {
		return fmt.Errorf("ptty: open pty: %w", err)
	}

	if err = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		_ = ptmx.Close()
		_ = tty.Close()

		return fmt.Errorf("ptty: set window size: %w", err)
	}

	sh := exec.Command("sh")
	sh.Dir = m.opts.WorkDir
	sh.Env = append(environSans("PS1", "PS2", "ENV", "BASH_ENV"),
		"PS1=", "PS2=", "ENV=/dev/null", "BASH_ENV=/dev/null")
	sh.Stdout = tty
	sh.Stderr = tty
	sh.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	stdinW, err := sh.StdinPipe()
	if err != nil {
		_ = ptmx.Close()
		_ = tty.Close()

		return fmt.Errorf("ptty: stdin pipe: %w", err)
	}

	if err = sh.Start(); err != nil {
		_ = ptmx.Close()
		_ = tty.Close()

		return fmt.Errorf("ptty: start shell: %w", err)
	}

	// The parent's slave copy must go: the master's EIO/EOF-on-exit signal
	// (Pitfall 4) fires only once EVERY slave descriptor is closed.
	_ = tty.Close()

	m.ptmx = ptmx
	m.stdinW = stdinW
	m.cmd = sh
	m.started = true
	m.shellDead = false

	m.gen++
	myGen := m.gen

	// The reaper: Wait the shell so it is never a zombie; its exit flips
	// this generation's dead flag (Run's entry check restarts lazily).
	go func() {
		_ = sh.Wait()

		m.mu.Lock()
		defer m.mu.Unlock()

		if m.gen == myGen {
			m.shellDead = true
		}
	}()

	return nil
}

// environSans returns the process environment minus the named variables
// (non-interactive pinning for the persistent shell).
func environSans(names ...string) []string {
	environ := os.Environ()
	out := make([]string, 0, len(environ))

outer:
	for _, kv := range environ {
		for _, n := range names {
			if strings.HasPrefix(kv, n+"=") {
				continue outer
			}
		}

		out = append(out, kv)
	}

	return out
}

// markDeadLocked clears the live-shell state (callers hold m.mu) — the next
// Run restarts lazily with the note.
func (m *PTYManager) markDeadLocked() {
	if m.ptmx != nil {
		_ = m.ptmx.Close()
	}

	if m.stdinW != nil {
		_ = m.stdinW.Close()
	}

	m.ptmx = nil
	m.stdinW = nil
	m.cmd = nil
}

// sentinelEchoLine builds the per-call completion probe written to the
// shell's stdin: the nonce makes output-lookalike collisions infeasible
// (PAR-09 adjacency probe) and the trailing $? carries the command's exit
// status.
func sentinelEchoLine(nonce string) string {
	return "echo __ASS_GUARD_DONE_" + nonce + "_$?__"
}

// Run executes one command in the session's persistent shell (22-04,
// PAR-09): serialized on the manager mutex (D-07 arrival order); empty and
// whitespace-only commands are structured errors that touch nothing (the
// empty probe — no sentinel written, cwd/env unmoved); the write is
// `<command>\n<sentinel echo>\n`; the read loop captures the master until
// the FULL nonce result line (exit parsed from its $?), ANSI-stripping the
// capture. A dead shell (detected on entry via the reaper flag, or mid-read
// via EIO/EOF without the nonce) restarts LAZILY with the NoteFn — the
// restart call itself then proceeds on the fresh shell.
func (m *PTYManager) Run(ctx context.Context, command string) (string, int, error) { //nolint:funlen,cyclop,gocognit,maintidx // one flow
	if strings.TrimSpace(command) == "" {
		return "", 0, errBadTaskInput // the empty probe: structured error, no shell interaction
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Dead-shell lazy restart (D-08): the previous shell died (reaper flag
	// or an earlier markDead) — restart, visibly (the note fires exactly
	// once per restart; a first-ever start is not a restart).
	if m.started && (m.ptmx == nil || m.shellDead) {
		m.markDeadLocked()

		if m.opts.NoteFn != nil {
			m.opts.NoteFn("persistent shell restarted; previous shell state (cwd/env) was lost")
		}
	}

	if err := m.ensureShell(); err != nil {
		return "", 0, err
	}

	nonce := newPTYNonce()

	// The command + sentinel echo ride one stdin write (the shell executes
	// them in order).
	script := command + "\n" + sentinelEchoLine(nonce) + "\n"

	if _, werr := m.stdinW.Write([]byte(script)); werr != nil {
		m.markDeadLocked()

		return "", 0, fmt.Errorf("ptty: write: %w", werr)
	}

	marker := []byte("__ASS_GUARD_DONE_" + nonce + "_")

	var capture bytes.Buffer

	exitCode, rerr := m.readUntilSentinel(ctx, marker, &capture)
	if rerr != nil {
		// The shell died (or the call was cancelled) mid-window (Pitfall 4
		// made the read END, not hang): report the interrupted state; the
		// NEXT call restarts lazily with the note. The capture is still
		// ANSI-stripped — escape sequences never reach the tool result.
		m.markDeadLocked()

		return StripANSI(capture.String()), exitCode, rerr
	}

	return trimPTYCapture(capture.String(), nonce), exitCode, nil
}

// trimPTYCapture converts the raw sentinel-bounded window into the tool
// result: ANSI-strip + pty line-ending normalization, then cut at the
// sentinel RESULT line (the last `__ASS_GUARD_DONE_<nonce>_<N>__` — digits
// distinguish it from any output-lookalike text the command itself printed),
// then the trailing trim. The stdin-pipe shell shape guarantees no other
// artifacts: nothing echoes the command or the sentinel probe back.
func trimPTYCapture(raw, nonce string) string {
	out := StripANSI(raw)
	out = strings.ReplaceAll(out, "\r\n", "\n")

	if idx := lastSentinelResult(out, nonce); idx >= 0 {
		out = out[:idx]
	}

	return trimCaptured(out)
}

// lastSentinelResult returns the byte index of the LAST full sentinel result
// line in s, or -1. A result line is `__ASS_GUARD_DONE_<nonce>_<digits>__`;
// anything else after the marker (a lookalike the command printed, or an
// unset `$?`) is not a result.
func lastSentinelResult(s, nonce string) int {
	marker := "__ASS_GUARD_DONE_" + nonce + "_"

	idx := strings.LastIndex(s, marker)
	if idx < 0 {
		return -1
	}

	tail := s[idx+len(marker):]
	end := strings.Index(tail, "__")
	if end <= 0 {
		return -1 // unset or empty status — not the result form
	}

	for _, c := range tail[:end] {
		if c < '0' || c > '9' {
			return -1 // not digits — a lookalike
		}
	}

	return idx
}

// readUntilSentinel reads the master until the full nonce result line
// appears (parsing the exit code from its trailing digits) or the read ends
// (EIO-as-EOF — linux; EOF — darwin). The captured bytes append RAW;
// artifact trimming happens at the caller (one pass, table-tested).
//
//nolint:funlen // the read loop reads best as one flow
func (m *PTYManager) readUntilSentinel(ctx context.Context, marker []byte, capture *bytes.Buffer) (int, error) {
	buf := make([]byte, 4096)

	for {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("ptty: cancelled: %w", ctx.Err())
		}

		if ds, ok := m.ptmx.(deadlineSetter); ok {
			_ = ds.SetReadDeadline(time.Now().Add(ptyReadTimeout))
		}

		n, rerr := m.ptmx.Read(buf)

		if n > 0 {
			capture.Write(buf[:n])

			if code, ok := parseSentinel(capture.Bytes(), marker); ok {
				m.clearReadDeadline()

				return code, nil
			}
		}

		if rerr == nil {
			continue
		}

		m.clearReadDeadline()

		var nerr netErr
		if errors.As(rerr, &nerr) && nerr.Timeout() {
			continue // bounded poll tick — the ctx re-check above is the exit
		}

		if errors.Is(rerr, syscall.EIO) || errors.Is(rerr, io.EOF) {
			// EIO-as-EOF (linux) / EOF (darwin): the shell is gone. The
			// sentinel never arrives — the interrupted outcome surfaces
			// (never a spin, never a healthy-session failure mark).
			return 0, errors.New("ptty: shell exited before the command completed")
		}

		return 0, fmt.Errorf("ptty: read: %w", rerr)
	}
}

// netErr is the read-deadline timeout interface subset (io goes through
// os.File's poll errors on both darwin and linux).
type netErr interface {
	Timeout() bool
}

// deadlineSetter is the master's deadline interface (creack/pty returns
// *os.File; the narrow interface keeps the field type io-level).
type deadlineSetter interface {
	SetReadDeadline(t time.Time) error
}

// clearReadDeadline resets the master's read deadline (best-effort).
func (m *PTYManager) clearReadDeadline() {
	if ds, ok := m.ptmx.(deadlineSetter); ok {
		_ = ds.SetReadDeadline(time.Time{})
	}
}

// parseSentinel reports whether the capture window contains the FULL
// sentinel result line and extracts its exit code
// (`__ASS_GUARD_DONE_<nonce>_<N>__`). LAST occurrence wins; the digit guard
// rejects any lookalike the command's own output carries.
func parseSentinel(window, marker []byte) (int, bool) {
	idx := bytes.LastIndex(window, marker)
	if idx < 0 {
		return 0, false
	}

	tail := window[idx+len(marker):]

	end := bytes.Index(tail, []byte("__"))
	if end <= 0 {
		return 0, false
	}

	code := 0

	for _, c := range tail[:end] {
		if c < '0' || c > '9' {
			return 0, false // not the exit-status form — a lookalike
		}

		code = code*10 + int(c-'0')
	}

	return code, true
}

// Drain ends the shell (the OnClose link — session close and agent shutdown
// both sweep it, D-08/PAR-08's no-orphans letter): SIGTERM the session group
// (Setsid made the shell a session leader), bounded grace, SIGKILL + reap
// escalation, then close the master fd and the stdin pipe (Pitfall 5 —
// unconditional once started). Idempotent; a never-started manager is a
// no-op.
func (m *PTYManager) Drain() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil || m.cmd.Process == nil {
		if m.ptmx != nil {
			_ = m.ptmx.Close()
			m.ptmx = nil
		}

		if m.stdinW != nil {
			_ = m.stdinW.Close()
			m.stdinW = nil
		}

		return
	}

	pid := m.cmd.Process.Pid

	if err := syscall.Kill(-pid, syscall.SIGTERM); err == nil {
		deadline := time.Now().Add(ptyTermGrace)

		for time.Now().Before(deadline) {
			if kerr := syscall.Kill(-pid, 0); kerr != nil {
				break // group gone — TERM sufficed
			}

			time.Sleep(25 * time.Millisecond)
		}
	}

	_ = syscall.Kill(-pid, syscall.SIGKILL)
	reapGroup(pid)

	if m.ptmx != nil {
		_ = m.ptmx.Close()
	}

	if m.stdinW != nil {
		_ = m.stdinW.Close()
	}

	m.ptmx = nil
	m.stdinW = nil
	m.cmd = nil
}

// newPTYNonce mints the per-call sentinel nonce (crypto/rand hex — the
// adjacency probe's collision-infeasibility guarantee).
func newPTYNonce() string {
	var b [8]byte

	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("n%d", time.Now().UnixNano()) // unreachable fallback
	}

	return hex.EncodeToString(b[:])
}
