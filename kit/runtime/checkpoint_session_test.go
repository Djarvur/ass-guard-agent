package runtime //nolint:testpackage // internal package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/kit/checkpoint"
	"github.com/Djarvur/ass-guard-agent/kit/session"
)

// --- Task 3: serve wiring + the gated live rollback demonstration ---

// TestSessionFor_WiresCheckpointer (Test 13, EARLY-01): sessionFor opens the
// checkpoint store at <workDir>/.ass-guard/checkpoints/shadow.git, the
// Session's Checkpointer is non-nil, and one Prompt through the wired
// session leaves a refs/checkpoints/ entry (default ON).
func TestSessionFor_WiresCheckpointer(t *testing.T) {
	t.Parallel()

	r, _ := newExpansionRunner(t, false, scriptedResp{text: "wired", finish: stopEndTurn})

	const sessionID = "sess-ckpt-w"

	sess := r.sessionFor(context.Background(), sessionID)

	// The store exists at the workspace-pinned path.
	_, serr := os.Stat(filepath.Join(r.workDir, ".ass-guard", "checkpoints", "shadow.git"))
	if serr != nil {
		t.Fatalf("checkpoint store missing after sessionFor: %v", serr)
	}

	// The Session's Checkpointer is wired (nil would mean silently disabled).
	if sess.Checkpointer == nil {
		t.Fatal("sessionFor must wire the Session.Checkpointer (default ON)")
	}

	// One Prompt through the wired session leaves a turn checkpoint.
	_, perr := sess.Prompt(context.Background(),
		[]session.ContentBlock{{Type: blockText, Text: "leave a checkpoint"}})
	if perr != nil {
		t.Fatalf("Prompt: %v", perr)
	}

	store, oerr := checkpoint.Open(r.workDir)
	if oerr != nil {
		t.Fatalf("checkpoint.Open: %v", oerr)
	}

	entries, lerr := store.List()
	if lerr != nil {
		t.Fatalf("List: %v", lerr)
	}

	if len(entries) != 1 {
		t.Fatalf("List = %+v; want exactly the turn's checkpoint", entries)
	}

	if entries[0].Ref != "refs/checkpoints/"+sessionID+"-turn-001" {
		t.Errorf("checkpoint ref = %q; want the turn-001 ref for %s", entries[0].Ref, sessionID)
	}
}
