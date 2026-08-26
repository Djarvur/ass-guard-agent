package main

import (
	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/learningcmd"
)

// newLearningCmd builds the `ass-guard learning` command group: operator
// tooling to inspect the learned-config store + revert individual entries
// (LRN-04 / D-20 — versioned + revertible). Transport discipline (C1): the
// listing table goes to STDOUT (one row per entry, machine-parseable); all
// diagnostics go to STDERR.
func newLearningCmd() *cobra.Command {
	var learnedPath string

	cmd := &cobra.Command{
		Use:   "learning",
		Short: "Inspect and revert the learned-config store",
	}
	listCmd := &cobra.Command{
		Use:          "list",
		Short:        "list every learned entry (id, situation, answer, confidence, status, expiry)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return learningcmd.RunLearningList(cmd.OutOrStdout(), cmd.ErrOrStderr(), learningcmd.ResolveLearnedPath(learnedPath)) //nolint:lll // thin delegation
		},
	}

	revertCmd := &cobra.Command{
		Use:          "revert <id>",
		Short:        "revert one learned entry by id (LRN-04)",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return learningcmd.RunLearningRevert(cmd.OutOrStdout(), cmd.ErrOrStderr(), learningcmd.ResolveLearnedPath(learnedPath), args[0]) //nolint:lll // thin delegation
		},
	}
	for _, sub := range []*cobra.Command{listCmd, revertCmd} {
		sub.Flags().StringVar(&learnedPath, "learned", "", "path to learned.yaml (default: .ass-guard/learned.yaml)")
	}

	cmd.AddCommand(listCmd)
	cmd.AddCommand(revertCmd)

	return cmd
}
