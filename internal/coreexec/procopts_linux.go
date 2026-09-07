//go:build linux

package coreexec

import "syscall"

// setPdeathsig arms the child so the kernel SIGKILLs it when the creating
// thread dies — a killed ass-guard process cannot orphan background task
// children (PAR-08).
//
// THREAD-LIFETIME CAVEAT (go.dev/issue/27505, RESEARCH Pitfall 3/A4):
// Pdeathsig fires on the death of the CREATING OS THREAD, not the process —
// the Go scheduler may retire that thread while ass-guard is alive,
// prematurely killing the child. Set at fork time (here, from the Start
// path); if premature kills appear in practice, pin the Start call with
// runtime.LockOSThread. Residual risk documented; only a Linux host can
// exercise it (darwin compiles the no-op leg).
func setPdeathsig(attr *syscall.SysProcAttr) {
	attr.Pdeathsig = syscall.SIGKILL
}
