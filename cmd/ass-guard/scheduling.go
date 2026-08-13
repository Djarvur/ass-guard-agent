package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/scheduler"
)

// newSchedulingCmd builds the `ass-guard scheduling` command group: operator
// tooling to validate a scheduling config and inspect a tier's resolution at a
// given time without running the agent (SCHED-01 operator surface). Transport
// discipline (C1): human-readable output goes to STDERR by default; `--json` is
// the ONLY path that writes to STDOUT, and only when explicitly requested.
func newSchedulingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduling",
		Short: "Inspect and validate the model scheduling config",
	}
	cmd.AddCommand(newSchedulingValidateCmd())
	cmd.AddCommand(newSchedulingResolveCmd())

	return cmd
}

// newSchedulingValidateCmd builds `ass-guard scheduling validate [--config]`.
// Loads + validates the config; prints the validation report to STDERR on
// failure (transport discipline) + exits non-zero; on success prints
// "scheduling config valid" to STDERR + exit 0.
func newSchedulingValidateCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:          "validate",
		Short:        "load + validate a scheduling config (D-10 load-time guarantee)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := []string{}
			if configPath != "" {
				paths = append(paths, configPath)
			}

			_, err := scheduler.Load(paths...)
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
	cmd.Flags().StringVar(&configPath, "config", "", "path to scheduling.yaml (default: embedded zero-config floor)")

	return cmd
}

// newSchedulingResolveCmd builds `ass-guard scheduling resolve --tier <t>
// [--project <p>] [--at <RFC3339>] [--json] [--config]`. Resolves a tier at a
// time (default now, or --at) and prints the (provider, model) + fallback chain
// + capability profile. Default human-readable form → STDERR; --json → STDOUT
// (the ONLY stdout path, explicitly requested — transport discipline, pitfall 9).
func newSchedulingResolveCmd() *cobra.Command {
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

			cfg, err := scheduler.Load(paths...)
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

			primary, fallbacks, err := scheduler.NewResolver(cfg).Resolve(tier, project, now, scheduler.CapabilityReq{})
			if err != nil {
				return err
			}

			if asJSON {
				return emitResolveJSON(cmd.OutOrStdout(), tier, project, &primary, fallbacks)
			}

			emitResolveHuman(cmd.ErrOrStderr(), tier, project, &primary, fallbacks)

			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "path to scheduling.yaml (default: embedded zero-config floor)")
	cmd.Flags().StringVar(&tier, "tier", "", "tier to resolve (heavy|good|light) — required")
	cmd.Flags().StringVar(&project, "project", "", "per-project key (default: global)")
	cmd.Flags().StringVar(&atStr, "at", "", "RFC3339 time to resolve at (default: now)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON to stdout (default: human form to stderr)")
	_ = cmd.MarkFlagRequired("tier")

	return cmd
}

// emitResolveHuman writes the human-readable resolution to the STDERR writer
// (transport discipline — stdout stays byte-clean unless --json). In production
// cobra wires this to os.Stderr; tests redirect via SetErr.
func emitResolveHuman(w io.Writer, tier, project string, primary *scheduler.Target, fallbacks []scheduler.Target) {
	proj := project
	if proj == "" {
		proj = "(global)"
	}

	_, _ = fmt.Fprintf(w, "%s [%s] -> %s/%s\n", tier, proj, primary.Provider, primary.Model)
	_, _ = fmt.Fprintf(w, "  base_url: %s\n", primary.BaseURL)
	_, _ = fmt.Fprintf(w, "  shape: %s\n", primary.Shape)
	_, _ = fmt.Fprintf(w, "  capabilities: %s\n", describeCapabilities(primary.Capabilities))
	_, _ = fmt.Fprintf(w, "  pricing: $%.4f/Mtok in, $%.4f/Mtok out\n",
		primary.Pricing.InputPerMToken, primary.Pricing.OutputPerMToken)

	if len(fallbacks) > 0 {
		names := make([]string, 0, len(fallbacks))
		for i := range fallbacks {
			names = append(names, fallbacks[i].Provider+"/"+fallbacks[i].Model)
		}

		_, _ = fmt.Fprintf(w, "  fallback: %s\n", strings.Join(names, ", "))
	} else {
		_, _ = fmt.Fprintf(w, "  fallback: (none)\n")
	}
}

// emitResolveJSON writes the machine-readable resolution to the STDOUT writer —
// the ONLY stdout path in the scheduling CLI, only when --json is explicitly
// requested. In production cobra wires this to os.Stdout; tests redirect via
// SetOut.
func emitResolveJSON(w io.Writer, tier, project string, primary *scheduler.Target, fallbacks []scheduler.Target) error {
	type fb struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}

	out := struct {
		Tier         string                      `json:"tier"`
		Project      string                      `json:"project"`
		Provider     string                      `json:"provider"`
		Model        string                      `json:"model"`
		BaseURL      string                      `json:"base_url"`
		Shape        string                      `json:"shape"`
		Capabilities scheduler.CapabilityProfile `json:"capabilities"`
		Pricing      scheduler.Pricing           `json:"pricing"`
		Fallback     []fb                        `json:"fallback"`
	}{
		Tier: tier, Project: project,
		Provider: primary.Provider, Model: primary.Model,
		BaseURL: primary.BaseURL, Shape: primary.Shape,
		Capabilities: primary.Capabilities, Pricing: primary.Pricing,
	}
	for i := range fallbacks {
		out.Fallback = append(out.Fallback, fb{Provider: fallbacks[i].Provider, Model: fallbacks[i].Model})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out)
}

// describeCapabilities renders a compact human form of the capability profile.
func describeCapabilities(c scheduler.CapabilityProfile) string {
	parts := []string{}
	if c.ToolCalling {
		parts = append(parts, "tool_calling")
	}

	if c.Streaming {
		parts = append(parts, "streaming")
	}

	if c.ExtendedThinking {
		parts = append(parts, "extended_thinking")
	}

	parts = append(parts, fmt.Sprintf("ctx=%d", c.ContextWindow))

	return strings.Join(parts, " ")
}
