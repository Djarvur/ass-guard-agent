package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
)

// checkpointNoEntriesNote is the empty-store list note (exit 0).
const checkpointNoEntriesNote = "no checkpoints"

// checkpointerAdapter adapts *checkpoint.Store to session.Checkpointer: the
// store's method is Snapshot, the seam speaks SnapshotTurn — the one-method
// adapter lives at the wiring site so internal/session keeps no dependency
// on internal/checkpoint (the OnClose func-seam pattern).
type checkpointerAdapter struct {
	store *checkpoint.Store
}

func (c checkpointerAdapter) SnapshotTurn(
	ctx context.Context, sessionID, turnID string,
) error {
	return c.store.Snapshot(ctx, sessionID, turnID) //nolint:wrapcheck // thin delegation
}

// checkpointRestoredNote is the restore success line.
const checkpointRestoredNote = "workspace restored to pre-turn state"

// checkpointIDHint documents the id grammar in structured errors.
const checkpointIDHint = "want <sessionID>-turn-<NNN>"

// newCheckpointCmd builds the `ass-guard checkpoint` command group
// (list | restore) — the EARLY-01 terminal surface over the shadow-git
// store. EVERY byte of output goes to STDERR: the binary's stdout is
// reserved for ACP JSON-RPC frames and stays byte-clean (transport
// discipline — this file references no stdout writer at all).
func newCheckpointCmd() *cobra.Command {
	checkpointCmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Shadow-git workspace checkpoints (EARLY-01): list and restore pre-turn snapshots",
		Long: "Operates the per-workspace shadow-git checkpoint store under " +
			".ass-guard/checkpoints/shadow.git. Every parent turn snapshots the workspace " +
			"BEFORE its mutations; `checkpoint list` shows the workspace's recovery history " +
			"and `checkpoint restore <sessionID-turn-NNN>` returns the workspace to that " +
			"pre-turn state (the user's repository git state is never touched). All output " +
			"is written to stderr — stdout stays reserved for ACP frames.",
		SilenceUsage: true,
	}

	var workDir string

	listCmd := &cobra.Command{
		Use:          "list",
		Short:        "list checkpoints, oldest first (empty store: \"no checkpoints\", exit 0)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := resolveWorkDir(workDir)
			if err != nil {
				return err
			}

			return runCheckpointList(cmd.ErrOrStderr(), dir)
		},
	}

	restoreCmd := &cobra.Command{
		Use:          "restore <sessionID-turn-NNN>",
		Short:        "restore the workspace to a checkpoint's pre-turn state",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveWorkDir(workDir)
			if err != nil {
				return err
			}

			return runCheckpointRestore(cmd.Context(), cmd.ErrOrStderr(), dir, args[0])
		},
	}

	for _, sub := range []*cobra.Command{listCmd, restoreCmd} {
		sub.Flags().StringVar(&workDir, "work-dir", "",
			"workspace whose checkpoints to operate on (default: cwd)")
	}

	checkpointCmd.AddCommand(listCmd, restoreCmd)

	return checkpointCmd
}

// runCheckpointList opens (lazily creating) the store over workDir and
// prints its entries to stderr, ascending by (sessionID, turn number). An
// empty or absent store prints "no checkpoints" and succeeds — an
// uncheckpointed workspace is a normal state, not an error.
func runCheckpointList(stderr io.Writer, workDir string) error {
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

// runCheckpointRestore returns the workspace to the named checkpoint's
// pre-turn state and reports the restored ref on stderr. A malformed or
// unknown id is a structured error (cobra prints it, exit 1) — never a
// silent success.
func runCheckpointRestore(ctx context.Context, stderr io.Writer, workDir, id string) error {
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
