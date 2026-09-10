package main

import (
	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/profilecheckcmd"
)

// newProfileCheckCmd builds the `ass-guard profile check <name>` subcommand
// (PROF-04, D-08). The fixture path (--capture-file) is fully unit-tested; the
// live path reads the freshest main rollout line and diffs against the manifest.
func newProfileCheckCmd() *cobra.Command {
	var (
		captureFile string
		zcodeBin    string
		profilesDir string
	)

	cmd := &cobra.Command{
		Use:          "check <name>",
		Short:        "diff a fresh capture against the profile's tiered manifest (PROF-04 drift detector)",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return profilecheckcmd.RunProfileCheck(args[0], profilesDir, captureFile)
		},
	}
	cmd.Flags().StringVar(&captureFile, "capture-file", "",
		"fixture JSON capture (a model_io line) — bypasses the live path")
	cmd.Flags().StringVar(&zcodeBin, "zcode-bin", profileZcode, "zcode binary (live path; operator-gated)")
	cmd.Flags().StringVar(&profilesDir, "profiles-dir", defaultProfilesDir(), "directory containing profile bundles")

	return cmd
}

// newProfileCmd builds the `ass-guard profile` command group.
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "profile operations (drift detection, inspection)",
	}
	cmd.AddCommand(newProfileCheckCmd())
	// 24-04 (TAIL-02): the nightly drift gate joins the profile group — the
	// drift-detection family (registered per the plan's profile_check.go:42
	// AddCommand site; also lives under `parity nightly-check`).
	cmd.AddCommand(newParityNightlyCheckCmd())

	return cmd
}
