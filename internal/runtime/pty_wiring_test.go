package runtime //nolint:testpackage // internal package test

import (
	"context"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
)

// TestSessionClosePTY (22-04 Task 3, D-08/Pitfall 5): a session that used
// the persistent shell has it DRAINED on session close — the OnClose chain
// (beside taskRegistry.ReapAll) TERM→KILLs the shell's session group and
// closes the master fd; no shell outlives its session.
func TestSessionClosePTY(t *testing.T) { //nolint:paralleltest // real pty shell + session close
	r, _ := newExpansionRunner(t, false, scriptedResp{text: "ok", finish: stopEndTurn})

	const sid = "sess-pty-close"

	sess := r.sessionFor(context.Background(), sid)
	if sess == nil {
		t.Fatal("sessionFor returned nil")
	}

	mgrAny, ok := r.ptyManagers.Load(sid)
	if !ok {
		t.Fatal("sessionFor did not store the session's PTY manager (22-04 wiring)")
	}

	mgr, ok := mgrAny.(*coreexec.PTYManager)
	if !ok {
		t.Fatalf("ptyManagers carries %T; want *coreexec.PTYManager", mgrAny)
	}

	// A persistent call starts the shell.
	out, _, rerr := mgr.Run(context.Background(), "echo started")
	if rerr != nil {
		t.Fatalf("persistent call error: %v", rerr)
	}

	if !strings.Contains(out, "started") {
		t.Fatalf("persistent call output = %q; want it to contain started", out)
	}

	if !mgr.Alive() {
		t.Fatal("shell not alive after the persistent call")
	}

	// Session close must drain it (the OnClose link).
	if cerr := sess.Close(); cerr != nil {
		t.Fatalf("session Close error: %v", cerr)
	}

	if mgr.Alive() {
		t.Error("persistent shell survived session close — Drain missing from the OnClose chain (D-08)")
	}

	if mgr.ShellPID() != 0 {
		t.Errorf("ShellPID() = %d after close; want 0 (manager state cleared)", mgr.ShellPID())
	}
}
