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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/loop"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

func main() {
	err := newRootCmd().Execute()
	if err != nil {
		// cobra already prints the error; exit non-zero. Diagnostics go to stderr.
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		prompt      string
		profileName string
		profilesDir string
		auditLog    string
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
			return runTrace(cmd.Context(), prompt, profileName, profilesDir, auditLog)
		},
	}
	root.PersistentFlags().StringVar(&prompt, "prompt", "", "prompt to send through the loop (required for the tracer)")
	root.PersistentFlags().StringVar(&profileName, "profile", "zcode", "profile name to load")
	root.PersistentFlags().StringVar(&profilesDir, "profiles-dir", defaultProfilesDir(), "directory containing profile bundles")
	root.PersistentFlags().StringVar(&auditLog, "audit-log", "", "write the redacted verbatim shaped request to this file (LOG-01); empty = stderr")

	root.AddCommand(newProfileCmd())
	root.AddCommand(newParityCmd())
	root.AddCommand(newACPCmd())
	root.AddCommand(newSchedulingCmd())
	root.AddCommand(newLearningCmd())

	return root
}

// runTrace loads the profile, runs one turn, and writes the tool-calls as JSON
// to stderr. All diagnostics go to stderr too; stdout is never touched. When
// --audit-log is set (or defaults to stderr), the redacted verbatim shaped
// request is captured via the event bus (LOG-01, D-13).
func runTrace(ctx context.Context, prompt, name, dir, auditLogPath string) error {
	if prompt == "" {
		return errors.New("--prompt is required")
	}

	prof, err := profile.NewLoader(dir).Load(name)
	if err != nil {
		return fmt.Errorf("load profile %q from %q: %w", name, dir, err)
	}

	// LOG-01 audit foundation: bus + AuditLogger + provider capturer. The
	// capturer is the seam that publishes the verbatim shaped body.
	bus := event.NewBus()

	sink, sinkClose, err := openAuditSink(auditLogPath)
	if err != nil {
		return err
	}

	if sinkClose != nil {
		defer sinkClose()
	}

	audit.NewAuditLogger(bus, sink)

	capturer := func(body []byte, _ map[string]string) {
		bus.Publish(event.RequestShaped{
			VerbatimRequest: body,
			Profile:         prof.Name,
			Timestamp:       time.Now(),
		})
	}

	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicRequestCapture(capturer),
	)

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

// openAuditSink resolves the audit sink. Empty path → stderr (default). stdout
// is rejected (transport discipline).
func openAuditSink(path string) (sink io.Writer, close func(), err error) {
	if path == "" || path == "-" {
		return os.Stderr, nil, nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open audit-log: %w", err)
	}

	return f, func() { _ = f.Close() }, nil
}

// defaultProfilesDir resolves the profiles directory relative to the working
// directory (the binary is run from the repo root in dev; the flag overrides).
func defaultProfilesDir() string {
	if abs, err := filepath.Abs("profiles"); err == nil {
		return abs
	}

	return "profiles"
}
