package main

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/paritycli"
)

func newParityCmd() *cobra.Command {
	var (
		suitePath     string
		fromRollout   string
		profileName   string
		profilesDir   string
		resultsPath   string
		surpriseCheck string
		cachePin      string
	)

	cmd := &cobra.Command{
		Use:          "parity",
		Short:        "run the behavioral mimicry A/B parity gate (MIMC-03, the north-star gate)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return paritycli.RunParity(
				suitePath, fromRollout, profileName, profilesDir, resultsPath, surpriseCheck, cachePin,
			)
		},
	}
	cmd.Flags().StringVar(&suitePath, "suite", defaultSuitePath(),
		"curated divergence suite JSON (ignored if --from-rollout is set)")
	cmd.Flags().StringVar(&fromRollout, "from-rollout", "",
		"generate the suite from a zcode rollout JSONL (same-session = matching system prompt + tools)")
	cmd.Flags().StringVar(&profileName, "profile", profileZcode, "profile to load for the ass-guard arm")
	cmd.Flags().StringVar(&profilesDir, "profiles-dir", defaultProfilesDir(), "directory containing profile bundles")
	cmd.Flags().StringVar(&resultsPath, "results", "parity-results.json", "results JSON output path")
	cmd.Flags().StringVar(&surpriseCheck, "surprise-check", "",
		"optional second suite JSON to run after the curated suite passes")
	cmd.Flags().StringVar(&cachePin, "cache-pin", defaultCachePinPath(),
		"corpus cache_control placement pin fixture (14-02 JSONL; empty = skip the placement check)")
	// 24-04 (TAIL-02): the nightly drift gate's canonical home — the workflow
	// and mise task invoke `parity nightly-check` (also under `profile`).
	cmd.AddCommand(newParityNightlyCheckCmd())

	return cmd
}

func defaultSuitePath() string {
	abs, err := filepath.Abs(filepath.Join("internal", "parity", "suite", "curated_suite.json"))
	if err != nil {
		return "internal/parity/suite/curated_suite.json"
	}

	return abs
}

// defaultCachePinPath resolves 14-02's committed corpus fixture — the
// cache_control placement pin — under the same repo-relative contract as the
// default suite path (the parity gate is an operator/dev-run command).
func defaultCachePinPath() string {
	rel := filepath.Join("internal", "profile", "testdata", "context-behavior", "cache-control.jsonl")

	abs, err := filepath.Abs(rel)
	if err != nil {
		return rel
	}

	return abs
}
