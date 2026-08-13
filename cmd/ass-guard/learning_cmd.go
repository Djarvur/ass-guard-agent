package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/learning"
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
			return runLearningList(cmd.OutOrStdout(), cmd.ErrOrStderr(), resolveLearnedPath(learnedPath))
		},
	}

	revertCmd := &cobra.Command{
		Use:          "revert <id>",
		Short:        "revert one learned entry by id (LRN-04)",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLearningRevert(cmd.OutOrStdout(), cmd.ErrOrStderr(), resolveLearnedPath(learnedPath), args[0])
		},
	}
	for _, sub := range []*cobra.Command{listCmd, revertCmd} {
		sub.Flags().StringVar(&learnedPath, "learned", "", "path to learned.yaml (default: .ass-guard/learned.yaml)")
	}

	cmd.AddCommand(listCmd)
	cmd.AddCommand(revertCmd)

	return cmd
}

// resolveLearnedPath applies the default (.ass-guard/learned.yaml relative to
// cwd) when the --learned flag is empty.
func resolveLearnedPath(flag string) string {
	if flag != "" {
		return flag
	}

	return filepath.Join(".ass-guard", "learned.yaml")
}

// runLearningList prints one row per entry to stdout. An empty store prints a
// header only (exit 0). Diagnostics go to stderr.
func runLearningList(stdout, stderr io.Writer, path string) error {
	store, err := learning.Open(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "learning: open %q: %v\n", path, err)

		return err
	}

	entries := store.List()

	_, _ = fmt.Fprintln(stdout, "ID\tSITUATION\tANSWER\tCONFIDENCE\tSTATUS\tEXPIRY")

	for _, e := range entries {
		expiry := ""
		if !e.Expiry.IsZero() {
			expiry = e.Expiry.UTC().Format("2006-01-02")
		}

		_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\t%d\t%s\t%s\n",
			e.ID, e.Situation, e.Answer, e.Confidence, e.Status, expiry)
	}

	if len(entries) == 0 {
		_, _ = fmt.Fprintln(stderr, "learning: no learned entries (empty store)")
	}

	return nil
}

// runLearningRevert removes the entry by id. A missing id is a no-op + a stderr
// warning (the store's Revert does not error on a missing id — the CLI warns so
// the operator knows nothing changed).
func runLearningRevert(stdout, stderr io.Writer, path, id string) error {
	store, err := learning.Open(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "learning: open %q: %v\n", path, err)

		return err
	}

	before := len(store.List())

	err = store.Revert(id)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "learning: revert %q: %v\n", id, err)

		return err
	}

	after := len(store.List())
	if after == before {
		_, _ = fmt.Fprintf(stderr, "learning: no entry with id %q (nothing reverted)\n", id)
	}

	_, _ = fmt.Fprintf(stdout, "reverted %s\n", id)

	return nil
}
