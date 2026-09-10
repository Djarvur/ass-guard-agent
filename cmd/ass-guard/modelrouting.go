package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/modelroutingcmd"
)

// newModelRoutingCmd builds the `ass-guard scheduling` command group: operator
// tooling to validate the model-routing config and inspect a tier's resolution at a
// given time without running the agent (SCHED-01 operator surface). Transport
// discipline (C1): human-readable output goes to STDERR by default; `--json` is
// the ONLY path that writes to STDOUT, and only when explicitly requested.
func newModelRoutingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model-routing",
		Short: "Inspect and validate the model scheduling config",
	}
	cmd.AddCommand(newModelRoutingValidateCmd())
	cmd.AddCommand(newModelRoutingResolveCmd())
	cmd.AddCommand(newModelRoutingStatsCmd())

	return cmd
}

// newModelRoutingValidateCmd builds `ass-guard scheduling validate [--config]`.
// Loads + validates the config; prints the validation report to STDERR on
// failure (transport discipline) + exits non-zero; on success prints
// "scheduling config valid" to STDERR + exit 0.
func newModelRoutingValidateCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:          "validate",
		Short:        "load + validate the model-routing config (D-10 load-time guarantee)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := []string{}
			if configPath != "" {
				paths = append(paths, configPath)
			}

			_, err := modelrouting.Load(paths...)
			if err != nil {
				// Validation failures are reported to stderr (cobra's RunE
				// return surfaces them via SetErr); the operator sees the full
				// *ConfigError report.
				return fmt.Errorf("scheduling config invalid: %w", err)
			}

			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "scheduling config valid")

			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "path to config.yaml (default: embedded zero-config floor)")

	return cmd
}

// newModelRoutingResolveCmd builds `ass-guard scheduling resolve --tier <t>
// [--project <p>] [--at <RFC3339>] [--json] [--config]`. Resolves a tier at a
// time (default now, or --at) and prints the (provider, model) + fallback chain
// + capability profile. Default human-readable form → STDERR; --json → STDOUT
// (the ONLY stdout path, explicitly requested — transport discipline, pitfall 9).
func newModelRoutingResolveCmd() *cobra.Command {
	var (
		configPath string
		tier       string
		project    string
		atStr      string
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:          "resolve",
		Short:        "resolve a tier to a concrete (provider, model) at a given time",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := []string{}
			if configPath != "" {
				paths = append(paths, configPath)
			}

			cfg, err := modelrouting.Load(paths...)
			if err != nil {
				return fmt.Errorf("scheduling config invalid: %w", err)
			}

			now := time.Now()

			if atStr != "" {
				parsed, err := time.Parse(time.RFC3339, atStr)
				if err != nil {
					return fmt.Errorf("--at %q: %w", atStr, err)
				}

				now = parsed
			}

			primary, fallbacks, err := modelrouting.NewResolver(cfg).
				Resolve(tier, project, now, modelrouting.CapabilityReq{})
			if err != nil {
				return fmt.Errorf("call: %w", err)
			}

			if asJSON {
				return modelroutingcmd.EmitResolveJSON(cmd.OutOrStdout(), tier, project, &primary, fallbacks) //nolint:lll // thin delegation
			}

			modelroutingcmd.EmitResolveHuman(cmd.ErrOrStderr(), tier, project, &primary, fallbacks)

			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "path to config.yaml (default: embedded zero-config floor)")
	cmd.Flags().StringVar(&tier, "tier", "", "tier to resolve (heavy|good|light) — required")
	cmd.Flags().StringVar(&project, "project", "", "per-project key (default: global)")
	cmd.Flags().StringVar(&atStr, "at", "", "RFC3339 time to resolve at (default: now)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON to stdout (default: human form to stderr)")
	_ = cmd.MarkFlagRequired("tier")

	return cmd
}

// newModelRoutingStatsCmd builds `ass-guard model-routing stats [--store]
// [--json] [--config]` (24-02, TAIL-01 D-07): the aggregated outcome-store
// dump — per-(provider, model) outcome-class counts plus token/cost totals.
// A missing store is a normal state (empty aggregate, exit 0). Transport
// discipline (C1): the human form goes to STDERR; --json is the ONLY stdout
// path, and only when explicitly requested.
func newModelRoutingStatsCmd() *cobra.Command {
	var (
		storePath  string
		configPath string
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:          "stats",
		Short:        "dump aggregated routing-outcome stats from the outcome store",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			agg, skipped, err := modelroutingcmd.LoadAndAggregate(storePath)
			if err != nil {
				return fmt.Errorf("read outcome store: %w", err)
			}

			paths := []string{}
			if configPath != "" {
				paths = append(paths, configPath)
			}

			cfg, err := modelrouting.Load(paths...)
			if err != nil {
				return fmt.Errorf("scheduling config invalid: %w", err)
			}

			rows := modelroutingcmd.BuildStatsRows(agg, cfg)

			if asJSON {
				return modelroutingcmd.EmitStatsJSON(cmd.OutOrStdout(), rows, storePath, skipped)
			}

			modelroutingcmd.EmitStatsHuman(cmd.ErrOrStderr(), rows, storePath, skipped)

			return nil
		},
	}
	cmd.Flags().StringVar(&storePath, "store", ".ass-guard/routing/outcomes.jsonl",
		"path to the outcome store (default: .ass-guard/routing/outcomes.jsonl under the cwd)")
	cmd.Flags().StringVar(&configPath, "config", "",
		"path to config.yaml for tier-binding context (default: embedded zero-config floor)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON to stdout (default: human form to stderr)")

	return cmd
}
