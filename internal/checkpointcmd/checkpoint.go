package checkpointcmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
)

// checkpointNoEntriesNote is the empty-store list note (exit 0).
const checkpointNoEntriesNote = "no checkpoints"

// checkpointRestoredNote is the restore success line.
const checkpointRestoredNote = "workspace restored to pre-turn state"

// checkpointIDHint documents the id grammar in structured errors.
const checkpointIDHint = "want <sessionID>-turn-<NNN>"

// RunCheckpointList opens (lazily creating) the store over workDir and
// prints its entries to stderr, ascending by (sessionID, turn number). An
// empty or absent store prints "no checkpoints" and succeeds — an
// uncheckpointed workspace is a normal state, not an error.
func RunCheckpointList(stderr io.Writer, workDir string) error {
	store, err := checkpoint.Open(workDir)
	if err != nil {
		return fmt.Errorf("checkpoint list: %w", err)
	}

	entries, err := store.List()
	if err != nil {
		return fmt.Errorf("checkpoint list: %w", err)
	}

	if len(entries) == 0 {
		_, _ = fmt.Fprintln(stderr, checkpointNoEntriesNote)

		return nil
	}

	for _, e := range entries {
		id := strings.TrimPrefix(e.Ref, "refs/checkpoints/")

		_, _ = fmt.Fprintf(stderr, "%s\t%s\n", id, e.CommittedAt.Format(time.RFC3339))
	}

	return nil
}

// RunCheckpointRestore returns the workspace to the named checkpoint's
// pre-turn state and reports the restored ref on stderr. A malformed or
// unknown id is a structured error (cobra prints it, exit 1) — never a
// silent success.
func RunCheckpointRestore(ctx context.Context, stderr io.Writer, workDir, id string) error {
	//nolint:contextcheck // plan-pinned signature: Store.Open carries no ctx
	store, err := checkpoint.Open(workDir)
	if err != nil {
		return fmt.Errorf("checkpoint restore: %w", err)
	}

	err = store.Restore(ctx, id)
	if err != nil {
		return fmt.Errorf("checkpoint restore %q (%s): %w", id, checkpointIDHint, err)
	}

	_, _ = fmt.Fprintf(stderr, "restored %s — %s\n", id, checkpointRestoredNote)

	return nil
}
