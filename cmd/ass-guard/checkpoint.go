package main

import (
	"context"
	"errors"
	"io"

	"github.com/spf13/cobra"
)

// errCheckpointStub is the RED-phase stub error (GREEN replaces the bodies).
var errCheckpointStub = errors.New("checkpoint command not implemented yet") //nolint:err113,gochecknoglobals // RED-phase stub

// newCheckpointCmd builds the `ass-guard checkpoint` command group
// (list | restore) — the EARLY-01 terminal surface over the shadow-git
// store. EVERY byte of output goes to STDERR: the binary's stdout is
// reserved for ACP frames and stays byte-clean (transport discipline).
func newCheckpointCmd() *cobra.Command {
	checkpointCmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Shadow-git workspace checkpoints (EARLY-01): list and restore pre-turn snapshots",
		Long: "Operates the per-workspace shadow-git checkpoint store under " +
			".ass-guard/checkpoints/shadow.git. Every parent turn snapshots the workspace " +
			"BEFORE its mutations; `checkpoint list` shows the recovery history and " +
			"`checkpoint restore <sessionID-turn-NNN>` returns the workspace to that " +
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

// runCheckpointList prints the store's entries (ascending turn sequence) to
// stderr. An empty or absent store prints "no checkpoints" and succeeds.
func runCheckpointList(_ io.Writer, _ string) error {
	return errCheckpointStub
}

// runCheckpointRestore restores the workspace to the named checkpoint and
// prints the restored ref. An unknown or malformed id is a structured error
// (exit 1).
func runCheckpointRestore(_ context.Context, _ io.Writer, _, _ string) error {
	return errCheckpointStub
}
