package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/profilecheckcmd"
)

// nightlyDriftExitCode is the distinct drift exit (7) so CI maps drift apart
// from operational failure (exit 1) — the plan 24-04 exit contract
// (0 parity / 7 drift / 1 operational).
const nightlyDriftExitCode = 7

// newParityNightlyCheckCmd builds the `nightly-check` subcommand (TAIL-02
// D-09 light path: zcode version probe + bundle structure hash vs the pinned
// capture — offline, zero API spend). Registered in BOTH the parity group
// (the canonical `ass-guard parity nightly-check` path the nightly workflow
// and mise task invoke) and the profile group (the drift-detection family
// that holds `profile check`). The report JSON goes to --report ONLY and the
// human summary to stderr; stdout stays byte-clean (transport discipline —
// no --json stdout mode in scope).
func newParityNightlyCheckCmd() *cobra.Command {
	var (
		profilesDir string
		pinPath     string
		reportPath  string
		writePin    bool
	)

	cmd := &cobra.Command{
		Use:   "nightly-check",
		Short: "nightly upstream-parity drift gate: zcode probe + bundle hash vs the pinned capture (TAIL-02)",
		Long: "Runs the drift-only light path (OQ3): probes the installed zcode version and compares " +
			"sha256 digests of the pinned profile bundle files against the committed structure pin. " +
			"Writes the report JSON to --report on every run, prints a one-line summary to stderr, and " +
			"exits 0 on parity / 7 on drift / 1 on operational error (missing pin, missing bundle file; " +
			"a failed probe reads as drift). --write-pin regenerates the pin from the current bundle — " +
			"the pin updates ONLY via the recapture runbook, never to silence a drift signal.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if pinPath == "" {
				pinPath = filepath.Join(profilesDir, "structure-pin.json")
			}

			err := profilecheckcmd.RunNightlyCheck(profilecheckcmd.NightlyCheckOptions{
				ProfilesDir: profilesDir,
				PinPath:     pinPath,
				ReportPath:  reportPath,
				WritePin:    writePin,
			})
			if errors.Is(err, profilecheckcmd.ErrNightlyDrift) {
				// The drift verdict is a signal, not a usage failure: map the
				// typed error to the distinct exit code on the spot (main's
				// generic handler only knows exit 1).
				os.Exit(nightlyDriftExitCode)
			}

			return err //nolint:wrapcheck // run-logic error passes through (checkpoint.go precedent)
		},
	}
	cmd.Flags().StringVar(&profilesDir, "profiles-dir", filepath.Join(defaultProfilesDir(), profileZcode),
		"the pinned profile bundle directory (files hashed against the pin)")
	cmd.Flags().StringVar(&pinPath, "pin", "",
		"structure pin JSON (default <profiles-dir>/structure-pin.json)")
	cmd.Flags().StringVar(&reportPath, "report", "",
		"drift report JSON output path (written on every check run; empty = no report file)")
	cmd.Flags().BoolVar(&writePin, "write-pin", false,
		"regenerate the pin from the current bundle (recapture runbook ONLY — never to silence drift)")

	return cmd
}
