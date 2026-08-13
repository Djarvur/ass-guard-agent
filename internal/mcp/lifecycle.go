//go:build darwin || linux

package mcp

import (
	"os/exec"
	"syscall"
	"time"
)

// shutdownGrace is the bounded SIGTERM→SIGKILL escalation window (D-02). A
// well-behaved server exits within this; a hanging one is force-killed after.
const shutdownGrace = 3 * time.Second

// signalPollInterval is the group-liveness poll cadence during shutdown.
const signalPollInterval = 50 * time.Millisecond

// prepareCommand sets the subprocess attributes for process-group isolation
// (D-02): the server runs in its OWN process group (Setpgid), so signaling the
// negative PGID reaches the server AND any grandchildren it forked. It also
// applies the config env overlay + working directory. Called before the SDK's
// Connect wraps the cmd in a CommandTransport.
func prepareCommand(cmd *exec.Cmd, cfg *ServerConfig) error {
	cmd.Env = mergeEnv(cfg.Env)
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}

	// Setpgid puts the child in a new process group whose ID == the child PID.
	// Every grandchild inherits the group, so kill(-pgid) reaches the whole tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return nil
}

// groupID returns the server's process-group ID. Under Setpgid, the PGID equals
// the server's PID (the child is the group leader). Returns -1 before Start.
func groupID(cmd *exec.Cmd) int {
	if cmd == nil || cmd.Process == nil {
		return -1
	}

	return cmd.Process.Pid
}

// signalGroup sends SIGTERM to the whole process group (-pgid), waits up to
// grace for the group to exit, then escalates to SIGKILL and confirms death.
// Signaling the negative PGID reaches the server AND any grandchildren (the
// value over the SDK's process-only signal). A non-positive pgid is a no-op.
// The grace window is bounded so a hanging server NEVER blocks shutdown.
func signalGroup(pgid int, grace time.Duration) error {
	if pgid <= 0 {
		return nil
	}

	// SIGTERM the whole group.
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	// Wait for the group to exit within grace.
	if groupGone(pgid, grace) {
		return nil
	}

	// Escalate to SIGKILL + confirm.
	_ = syscall.Kill(-pgid, syscall.SIGKILL)

	_ = groupGone(pgid, shutdownGrace)

	return nil
}

// groupGone polls the group liveness (kill -pgid 0) until it returns ESRCH
// (no process in the group) or the deadline passes. Returns true if the group
// is gone within the budget.
func groupGone(pgid int, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		err := syscall.Kill(-pgid, 0)
		if err != nil {
			return true // ESRCH: no process in the group
		}

		time.Sleep(signalPollInterval)
	}

	return false
}

// reap drains zombie children in the process group via Wait4(-pgid, WNOHANG)
// until none remain. This is the no-zombie guarantee (ECOS-02): any child the
// group-signal killed that ass-guard can reap is collected. It is best-effort —
// Wait4 on a group with no children ass-guard owns returns ECHILD immediately
// (the SDK reaps the server itself via cmd.Wait; init reaps reparented
// grandchildren). Non-blocking: returns immediately when nothing is pending.
func reap(pgid int) {
	if pgid <= 0 {
		return
	}

	for {
		var status syscall.WaitStatus

		pid, err := syscall.Wait4(-pgid, &status, syscall.WNOHANG, nil)
		if pid == 0 || err != nil {
			return // no child pending (0) or no children to reap (ECHILD)
		}
	}
}

// processAlive reports whether pid is currently alive (used by tests). A zombie
// still counts as "exists" for kill -0; callers verify non-zombie separately.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	return syscall.Kill(pid, 0) == nil
}
