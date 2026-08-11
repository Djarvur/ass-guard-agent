package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/djarvur/ass-guard-agent/internal/acp"
)

// serveOptions carries the `acp serve` subcommand flags. The profile is loaded
// by name (default zcode); --max-concurrent bounds outbound provider concurrency
// (PARA-04, default 6 — wired in Plan 02-04 when the provider semaphore lands).
type serveOptions struct {
	Profile       string
	MaxConcurrent int
	ProfilesDir   string
}

// newACPCmd builds the `acp` parent command. Today it carries the `serve`
// subcommand (the IDE entrypoint); future ACP-facing subcommands nest here.
func newACPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "acp",
		Short: "ACP (IDE-native) interface",
	}
	cmd.AddCommand(newACPServeCmd())
	return cmd
}

// newACPServeCmd builds the `acp serve` subcommand — the entrypoint Zed spawns.
// It sets log output to stderr (transport discipline — stdout is reserved for
// ACP frames), wires stdin→framer reader, stdout→framer writer, and calls
// server.Serve(ctx) with a signal-cancelled context.
func newACPServeCmd() *cobra.Command {
	var (
		profile       string
		maxConcurrent int
		profilesDir   string
	)
	c := &cobra.Command{
		Use:   "serve",
		Short: "Run the ACP v1 server over stdio (the entrypoint Zed spawns)",
		Long: "Speaks ACP v1 (newline-delimited JSON-RPC) over stdio. stdout carries ONLY " +
			"valid ACP frames; all diagnostics go to stderr (transport discipline). " +
			"The lifecycle is initialize → session/new → session/prompt with streamed " +
			"session/update notifications (ACP-04). session/load is a no-op (D-09 — NO " +
			"replay in v1). Needs ZAI_API_KEY for real model turns; the server skeleton " +
			"works without it for the ACP handshake.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Transport discipline (Pitfall 1): stdout is reserved EXCLUSIVELY for
			// ACP frames. All log/diagnostic output goes to stderr.
			log.SetOutput(os.Stderr)
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runACPServe(ctx, os.Stdin, os.Stdout, os.Stderr, serveOptions{
				Profile:       profile,
				MaxConcurrent: maxConcurrent,
				ProfilesDir:   profilesDir,
			})
		},
	}
	c.Flags().StringVar(&profile, "profile", "zcode", "profile name to load (PROF-01)")
	c.Flags().IntVar(&maxConcurrent, "max-concurrent", 6, "max concurrent outbound provider calls across parent + subagents (PARA-04)")
	c.Flags().StringVar(&profilesDir, "profiles-dir", defaultProfilesDir(), "directory containing profile bundles")
	return c
}

// runACPServe constructs the ACP server and runs it until ctx is cancelled or
// stdin reaches EOF. It is split from the cobra command so tests can drive it
// with an in-process stdin/stdout/stderr. The turn runner is the stub for the
// tracer (Plan 02-01); Plan 02-05 installs the real Session.Prompt runner.
func runACPServe(ctx context.Context, in io.Reader, out, stderr io.Writer, opts serveOptions) error {
	// The profile load + provider wiring land in Plan 02-05 (real Session Core).
	// For the tracer, NewServer's default turn runner returns "end_turn" with no
	// chunks; the ACP transport seam (framer + handlers + adapter) is what this
	// plan proves end-to-end.
	_ = opts // profile/max-concurrent wired in Plan 02-04/02-05
	srv := acp.NewServer(in, out, stderr)
	return srv.Serve(ctx)
}
