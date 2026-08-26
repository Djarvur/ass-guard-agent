package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
	"github.com/Djarvur/ass-guard-agent/internal/checkpointcmd"
)

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

			return checkpointcmd.RunCheckpointList(cmd.ErrOrStderr(), dir)
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

			return checkpointcmd.RunCheckpointRestore(cmd.Context(), cmd.ErrOrStderr(), dir, args[0]) //nolint:lll // thin delegation
		},
	}

	for _, sub := range []*cobra.Command{listCmd, restoreCmd} {
		sub.Flags().StringVar(&workDir, "work-dir", "",
			"workspace whose checkpoints to operate on (default: cwd)")
	}

	checkpointCmd.AddCommand(listCmd, restoreCmd)

	return checkpointCmd
}
