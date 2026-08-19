package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// zcodeVersionTimeout bounds the `zcode --version` exec (T-14-12 DoS: a hung
// binary must never hang the parity run; unresolvable -> skip note).
const zcodeVersionTimeout = 3 * time.Second

// zcodeInstalledVersion resolves the INSTALLED zcode's version: a fixed-argv
// exec of `zcode --version` (T-14-11 Tampering: argv is the literal slice —
// no shell, no user input, no interpolation) under a bounded timeout, output
// trimmed. It is a package-level seam var so tests inject a fake installed
// version without a live binary (offline CI); the default is the real exec.
var zcodeInstalledVersion = func() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), zcodeVersionTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "zcode", "--version").Output()
	if err != nil {
		return "", fmt.Errorf("exec zcode --version: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// parityRun is the package-level seam over parity.Run (offline tests fake the
// A/B arms — no live provider needed); the default drives the real harness.
var parityRun = parity.Run

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
//
//nolint:funlen // domain complexity is inherent
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

	// 14-04 (EARLY-03, the 2026-08-19 borrow-#13 cheap slice): target-version
	// drift visibility. The pinned side is the coverage manifest's recorded
	// capture version (09-03 provenance); the installed side is the exec seam.
	// LOUD on drift, NEVER blocking — exit semantics are untouched below.
	emitVersionDriftWarning(os.Stderr, dir, name)

	// Phase 7 (D-08): the parity arm builds its provider through the scheduler
	// factory (zero-config $ZAI_API_KEY env resolution applies — the gate is
	// operator-gated on the env var). The provider surfaces a clear typed error
	// if the heavy-tier credential is unresolvable (Tier-A).
	factory, providerName, ferr := setupProviderFactory("", os.Stderr)
	if ferr != nil {
		return fmt.Errorf("setup provider factory: %w", ferr)
	}

	prov, ferr := factory.Build(providerName, shaper.New())
	if ferr != nil {
		return fmt.Errorf("build provider %q: %w", providerName, ferr)
	}

	res, err := parityRun(context.Background(), &parity.RunOptions{
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
			sres, err := parityRun(context.Background(), &parity.RunOptions{
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
		//nolint:err113 // dynamic error message
		return fmt.Errorf("PARITY GATE FAIL: %d/%d turns matched on both layers (Layer1=%.2f Layer2=%.2f)",
			countBothLayerPass(&res), res.Summary.SuiteSize, res.Summary.Layer1PassRate, res.Summary.Layer2PassRate)
	}

	return nil
}

// emitVersionDriftWarning compares the installed zcode (the exec seam) against
// the coverage manifest's pinned capture version and prints exactly one line:
// a loud drift warning naming BOTH versions on mismatch, an explicit
// provenance line on match, or a skip note when either side is unresolvable
// (binary absent, manifest missing/unreadable, pin unrecorded). It NEVER
// returns an error — the drift warning is non-blocking by design (locked
// prohibition: exit semantics are the parity gate's alone; the full nightly
// gate is 12-08's scope per the 2026-08-19 disposition).
func emitVersionDriftWarning(w io.Writer, profilesDir, name string) {
	manifest, err := profile.LoadCoverage(filepath.Join(profilesDir, name, "coverage.yaml"))
	if err != nil {
		fmt.Fprintf(w, "zcode version check skipped: coverage manifest unavailable (%v)\n", err)

		return
	}

	pinned := manifest.TargetCaptureRef.ZcodeVersion
	if pinned == "" {
		fmt.Fprintf(w, "zcode version check skipped: pinned zcode version unrecorded in the coverage manifest\n")

		return
	}

	installed, ierr := zcodeInstalledVersion()
	if ierr != nil {
		fmt.Fprintf(w, "zcode version check skipped: installed zcode unresolvable (%v); pinned capture zcode %s\n",
			ierr, pinned)

		return
	}

	if installed != pinned {
		fmt.Fprintf(w, "WARNING: zcode version drift — installed %s, pinned capture %s "+
			"(the recorded parity reference ages as zcode updates; non-blocking)\n", installed, pinned)

		return
	}

	fmt.Fprintf(w, "capture provenance: zcode %s matches installed\n", pinned)
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
