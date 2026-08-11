// Command ass-guard is the entrypoint for the ass-guard agent.
//
// Phase 1 (Plan 01-01 T6) ships the tracer CLI: it loads the zcode profile,
// runs one prompt through the test-harness Turn Loop against the Anthropic
// provider adapter (Z.ai GLM via the Anthropic protocol), and prints the
// resulting tool-calls to STDERR as JSON. stdout is reserved for ACP frames
// (transport discipline, PROJECT.md) — this binary writes NOTHING to stdout.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/djarvur/ass-guard-agent/internal/loop"
	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/provider"
	"github.com/djarvur/ass-guard-agent/internal/shaper"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// cobra already prints the error; exit non-zero. Diagnostics go to stderr.
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		prompt      string
		profileName string
		profilesDir string
	)
	root := &cobra.Command{
		Use:   "ass-guard",
		Short: "ass-guard tracer — shape a profile, send one prompt, print tool-calls (stderr)",
		Long: "Loads the named profile (default: zcode), shapes one outgoing request via the " +
			"Profile Shaper, sends it to the Anthropic-protocol provider (Z.ai GLM by default), " +
			"and prints the parsed tool-calls as JSON to STDERR. stdout stays byte-clean " +
			"(reserved for ACP frames). Needs ZAI_API_KEY in the environment.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrace(cmd.Context(), prompt, profileName, profilesDir)
		},
	}
	root.PersistentFlags().StringVar(&prompt, "prompt", "", "prompt to send through the loop (required for the tracer)")
	root.PersistentFlags().StringVar(&profileName, "profile", "zcode", "profile name to load")
	root.PersistentFlags().StringVar(&profilesDir, "profiles-dir", defaultProfilesDir(), "directory containing profile bundles")

	return root
}

// runTrace loads the profile, runs one turn, and writes the tool-calls as JSON
// to stderr. All diagnostics go to stderr too; stdout is never touched.
func runTrace(ctx context.Context, prompt, name, dir string) error {
	if prompt == "" {
		return fmt.Errorf("--prompt is required")
	}
	prof, err := profile.NewLoader(dir).Load(name)
	if err != nil {
		return fmt.Errorf("load profile %q from %q: %w", name, dir, err)
	}
	p := provider.NewAnthropicProvider(shaper.New())

	calls, err := loop.Run(ctx, prof, p, prompt)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(calls, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tool-calls: %w", err)
	}
	// Transport discipline: tool-call JSON goes to STDERR, never stdout.
	fmt.Fprintln(os.Stderr, string(out))
	return nil
}

// defaultProfilesDir resolves the profiles directory relative to the working
// directory (the binary is run from the repo root in dev; the flag overrides).
func defaultProfilesDir() string {
	if abs, err := filepath.Abs("profiles"); err == nil {
		return abs
	}
	return "profiles"
}
