package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

func newParityCmd() *cobra.Command {
	var (
		suitePath     string
		fromRollout   string
		profileName   string
		profilesDir   string
		resultsPath   string
		surpriseCheck string
	)

	cmd := &cobra.Command{
		Use:          "parity",
		Short:        "run the behavioral mimicry A/B parity gate (MIMC-03, the north-star gate)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runParity(suitePath, fromRollout, profileName, profilesDir, resultsPath, surpriseCheck)
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

	return cmd
}

func defaultSuitePath() string {
	abs, err := filepath.Abs(filepath.Join("internal", "parity", "suite", "curated_suite.json"))
	if err != nil {
		return "internal/parity/suite/curated_suite.json"
	}

	return abs
}

// runParity loads the suite + profile, constructs the live Anthropic arm, runs
// the harness, and emits the structured footer + results JSON. Exit code reflects
// the gate (0 iff OverallPass). Needs ZAI_API_KEY (operator-gated).
func runParity(suitePath, rollout, name, dir, results, surprise string) error {
	var (
		suite []parity.CapturedTurn
		err   error
	)
	if rollout != "" {
		suite, err = parity.ExtractTurnsFromRollout(rollout)
		if err != nil {
			return fmt.Errorf("extract turns from rollout %q: %w", rollout, err)
		}

		fmt.Fprintf(os.Stderr, "parity: generated suite of %d turns from rollout %s\n",
			len(suite), filepath.Base(rollout))
	} else {
		suite, err = parity.LoadReplaySession(suitePath)
		if err != nil {
			return fmt.Errorf("load suite %q: %w", suitePath, err)
		}
	}

	prof, err := profile.NewLoader(dir).Load(name)
	if err != nil {
		return fmt.Errorf("load profile %q: %w", name, err)
	}
	// The provider surfaces a clear error if ZAI_API_KEY is unset (Tier-A).
	prov := provider.NewAnthropicProvider(shaper.New())

	res, err := parity.Run(context.Background(), &parity.RunOptions{
		Suite:       suite,
		Profile:     prof,
		Provider:    prov,
		ResultsPath: results,
		Model:       prof.Model,
	})
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	emitParityFooter("curated", &res)

	if res.Summary.OverallPass && surprise != "" {
		surpriseSuite, err := parity.LoadReplaySession(surprise)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parity: surprise-check suite did not load (%v); skipping\n", err)
		} else {
			sres, err := parity.Run(context.Background(), &parity.RunOptions{
				Suite: surpriseSuite, Profile: prof, Provider: prov,
				ResultsPath: "", Model: prof.Model,
			})
			if err != nil {
				return fmt.Errorf("call: %w", err)
			}

			emitParityFooter("surprise-check", &sres)
		}
	}

	if !res.Summary.OverallPass {
		// Non-zero exit signals the gate failed (PROJECT.md Anti-Pattern 5: stop-and-replan).
		return fmt.Errorf("PARITY GATE FAIL: %d/%d turns matched on both layers (Layer1=%.2f Layer2=%.2f)", //nolint:err113 // dynamic error message
			countBothLayerPass(&res), res.Summary.SuiteSize, res.Summary.Layer1PassRate, res.Summary.Layer2PassRate)
	}

	return nil
}

func emitParityFooter(label string, res *parity.RunResult) {
	status := "PASS"
	if !res.Summary.OverallPass {
		status = "FAIL"
	}

	fmt.Fprintf(os.Stderr, "=== PARITY RESULT (%s) ===\n", label)
	fmt.Fprintf(os.Stderr, "suite_size: %d\n", res.Summary.SuiteSize)
	fmt.Fprintf(os.Stderr, "layer1_pass_rate: %.4f\n", res.Summary.Layer1PassRate)
	fmt.Fprintf(os.Stderr, "layer2_pass_rate: %.4f\n", res.Summary.Layer2PassRate)
	fmt.Fprintf(os.Stderr, "overall_status: %s\n", status)

	for _, t := range res.Turns {
		match := "match"
		if !t.Layer1Match || t.Layer2Mismatch > 0 {
			match = fmt.Sprintf("MISMATCH (layer1=%v layer2_mismatch=%d)", t.Layer1Match, t.Layer2Mismatch)
		}

		fmt.Fprintf(os.Stderr, "  %s — %s\n", t.TurnID, match)
	}
}

func countBothLayerPass(res *parity.RunResult) int {
	n := 0

	for _, t := range res.Turns {
		if t.Layer1Match && t.Layer2Mismatch == 0 {
			n++
		}
	}

	return n
}
