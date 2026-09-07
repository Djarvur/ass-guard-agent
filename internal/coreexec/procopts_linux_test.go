//go:build linux

package coreexec

import (
	"testing"
)

// TestProcopts_PdeathsigArmed (22-02 Task 1, PAR-08): the Start path's
// SysProcAttr carries Pdeathsig SIGKILL so a killed ass-guard cannot orphan
// the child (compile-gated on darwin; runtime-observable only on Linux).
func TestProcopts_PdeathsigArmed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	reg := NewTaskRegistry()

	id, _, err := reg.Start(dir, "echo pdeathsig-probe")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	reg.mu.Lock()
	task := reg.tasks[id]
	reg.mu.Unlock()

	if task == nil || task.cmd == nil || task.cmd.SysProcAttr == nil {
		t.Fatal("task cmd/SysProcAttr missing after Start")
	}

	if got := task.cmd.SysProcAttr.Pdeathsig; got == 0 {
		t.Error("Pdeathsig unset on the background task child — orphans possible after ass-guard death")
	}
}
