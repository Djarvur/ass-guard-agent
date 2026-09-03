package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/acpserve"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

const mnd6 = 6

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
		profileName   string
		maxConcurrent int
		profilesDir   string
		workDir       string
		noEngine      bool
		askTimeout    time.Duration
	)

	c := &cobra.Command{
		Use:   "serve",
		Short: "Run the ACP v1 server over stdio (the entrypoint Zed spawns)",
		Long: "Speaks ACP v1 (newline-delimited JSON-RPC) over stdio. stdout carries ONLY " +
			"valid ACP frames; all diagnostics go to stderr (transport discipline). " +
			"The lifecycle is initialize → session/new → session/prompt with streamed " +
			"session/update notifications (ACP-04). session/load restores a past " +
			"session and replays it through the same ordered frames (ACP-06, 18-01). " +
			"Needs ZAI_API_KEY for real model turns; the server skeleton " +
			"works without it for the ACP handshake.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			// 09-06: the persistent --audit-log flag reaches the serve path
			// (it was declared but never read — the dead-flag finding).
			// De-cobra'd in plan 15-05: flag reads stay in the shell.
			auditPath, _ := cmd.Flags().GetString("audit-log")

			changedProfilesDir := cmd.Flags().Changed(flagProfilesDir)

			return runACPServeCmd(ctx, auditPath, profilesDir, workDir, changedProfilesDir,
				profileName, maxConcurrent, !noEngine, askTimeout)
		},
	}
	c.Flags().StringVar(&profileName, "profile", profileZcode, "profile name to load (PROF-01)")
	c.Flags().IntVar(&maxConcurrent, "max-concurrent", mnd6,
		"max concurrent outbound provider calls across parent + subagents (PARA-04)")
	c.Flags().StringVar(&profilesDir, flagProfilesDir, defaultProfilesDir(), "directory containing profile bundles")
	c.Flags().StringVar(&workDir, "work-dir", "", "working directory for .ass-guard/ transcripts (default: cwd)")
	c.Flags().BoolVar(&noEngine, "no-engine", false,
		"disable the Phase-4 unified engine (fall back to manual continue — D-04)")
	c.Flags().DurationVar(&askTimeout, "ask-timeout", session.DefaultAskTimeout,
		"how long an unanswered AskUserQuestion waits before the turn resumes with the "+
			"non-answer form (D-01); 0 = block forever (interactive mode)")

	return c
}

// runACPServeCmd is the `acp serve` RunE body (extracted for readability): it
// resolves workDir, runs the Phase-6 first-run seed (D-04), resolves the
// zero-config profiles dir (DIST-03), then constructs + serves the ACP server.
// All diagnostics go to stderr (transport discipline — stdout = ACP frames).
//
// De-cobra'd in plan 15-05 (one of the two sanctioned edits): the audit-log
// flag read and the profiles-dir Changed() probe live in the cobra shell and
// arrive here as plain values.
func runACPServeCmd(
	ctx context.Context, auditPath, profilesDir, workDir string, profilesDirChanged bool,
	profileName string, maxConcurrent int, engineEnabled bool,
	askTimeout time.Duration,
) error {
	log.SetOutput(os.Stderr)

	resolvedWorkDir, err := acpserve.ResolveWorkDir(workDir)
	if err != nil {
		return err //nolint:wrapcheck // flag-resolution error passes through
	}

	// Phase 6 first-run (D-04): seed .ass-guard/ when missing. Non-clobbering; a
	// failed seed degrades to defaults, never a server crash.
	acpserve.SeedACPGuard(resolvedWorkDir)

	// Zero-config profiles dir (DIST-03): prefer .ass-guard/profiles when the
	// flag is default and that dir exists; else the dev ./profiles default.
	resolvedProfilesDir := acpserve.ResolveProfilesDir(profilesDir, profilesDirChanged, resolvedWorkDir)

	return acpserve.Run( //nolint:wrapcheck // thin shell delegation
		ctx, os.Stdin, os.Stdout, os.Stderr, &acpserve.Options{
			Profile:       profileName,
			MaxConcurrent: maxConcurrent,
			ProfilesDir:   resolvedProfilesDir,
			WorkDir:       resolvedWorkDir,
			EngineEnabled: engineEnabled,
			AskTimeout:    askTimeout,
			AuditLogPath:  auditPath,
		})
}
