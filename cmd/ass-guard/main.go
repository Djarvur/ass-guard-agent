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
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/loop"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
	"github.com/Djarvur/ass-guard-agent/internal/version"
)

var errIsRequired = errors.New("--prompt is required")

// errUnknownCommand backs the root's legacy positional-arg rejection (18-06
// keeps it verbatim for the non-resume path; the message shape mirrors
// cobra's own unknown-command error).
var errUnknownCommand = errors.New("unknown command")

func main() {
	err := newRootCmd().Execute()
	if err != nil {
		// cobra already prints the error; exit non-zero. Diagnostics go to stderr.
		os.Exit(1)
	}
}

// rootArgsValidator is the root's positional-arg policy (18-06, D-10): the
// root takes a positional arg ONLY on the resume path — the `--resume
// <target>` space form leaves the target in args (pflag's NoOptDefVal
// contract). Without a resume flag the legacy root behavior stays:
// positional args are unknown subcommands.
func rootArgsValidator(cmd *cobra.Command, args []string) error {
	if readResumeFlags(cmd, args).active() {
		return cobra.MaximumNArgs(1)(cmd, args)
	}

	if len(args) > 0 {
		return fmt.Errorf("%w %q for %q", errUnknownCommand, args[0], cmd.CommandPath())
	}

	return nil
}

func newRootCmd() *cobra.Command {
	var (
		prompt      string
		profileName string
		profilesDir string
		auditLog    string
		versionFlag bool
		resumeFlag  string
		continueIt  bool
	)

	root := &cobra.Command{
		Use:   "ass-guard",
		Short: "ass-guard tracer — shape a profile, send one prompt, print tool-calls (stderr)",
		Long: "Loads the named profile (default: zcode), shapes one outgoing request via the " +
			"Profile Shaper, sends it to the Anthropic-protocol provider (Z.ai GLM by default), " +
			"and prints the parsed tool-calls as JSON to STDERR. stdout stays byte-clean " +
			"(reserved for ACP frames). Needs ZAI_API_KEY in the environment.",
		SilenceUsage: true,
		Args:         rootArgsValidator,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --version takes precedence over the tracer default RunE. Output goes
			// to STDERR (transport discipline — stdout stays clean for ACP frames).
			if versionFlag {
				fmt.Fprintln(os.Stderr, "ass-guard version "+version.String())

				return nil
			}

			// 18-06 (D-10): the CC-parity resume trio is typed at the ROOT, so
			// `ass-guard --resume[ <id|name>]` and `ass-guard --continue`/`-c`
			// delegate into the serve flow with the resolved target — the flag
			// works in any launch context, not only via editor RPC.
			rf := readResumeFlags(cmd, args)
			if rf.active() {
				return runRootResume(cmd, rf)
			}

			return runTrace(cmd.Context(), prompt, profileName, profilesDir, auditLog)
		},
	}
	root.PersistentFlags().StringVar(&prompt, "prompt", "", "prompt to send through the loop (required for the tracer)")
	root.PersistentFlags().StringVar(&profileName, "profile", profileZcode, "profile name to load")
	root.PersistentFlags().StringVar(&profilesDir, "profiles-dir", defaultProfilesDir(),
		"directory containing profile bundles")
	root.PersistentFlags().StringVar(&auditLog, "audit-log", "",
		"write the redacted verbatim shaped request to this file (LOG-01); empty = stderr")
	registerResumeFlags(root, &resumeFlag, &continueIt)
	root.Flags().BoolVar(&versionFlag, "version", false,
		"print the ass-guard version (build-time-injected) to stderr and exit")

	root.AddCommand(newProfileCmd())
	root.AddCommand(newParityCmd())
	root.AddCommand(newACPCmd())
	root.AddCommand(newModelRoutingCmd())
	root.AddCommand(newLearningCmd())
	root.AddCommand(newCheckpointCmd())

	return root
}

// registerResumeFlags registers the 18-06 D-10 trio as ROOT-persistent flags
// (extracted from newRootCmd for length): --resume with NoOptDefVal lets bare
// `--resume` parse to the picker sentinel, `--resume=<target>` carry the
// target directly, and the space form leave the target in args (see
// pickSentinel); --continue/-c is the cwd-scoped zero-ceremony resume.
func registerResumeFlags(root *cobra.Command, resumeFlag *string, continueIt *bool) {
	root.PersistentFlags().StringVar(resumeFlag, "resume", "",
		"resume a past session by id or title prefix; bare opens the picker (D-10)")
	root.PersistentFlags().Lookup("resume").NoOptDefVal = pickSentinel
	root.PersistentFlags().BoolVarP(continueIt, "continue", "c", false,
		"resume the most recent session in the current directory (D-10)")
}

// runTrace loads the profile, runs one turn, and writes the tool-calls as JSON
// to stderr. All diagnostics go to stderr too; stdout is never touched. When
// --audit-log is set (or defaults to stderr), the redacted verbatim shaped
// request is captured via the event bus (LOG-01, D-13).
func runTrace(ctx context.Context, prompt, name, dir, auditLogPath string) error {
	if prompt == "" {
		return errIsRequired
	}

	prof, err := profile.NewLoader(dir).Load(name)
	if err != nil {
		return fmt.Errorf("load profile %q from %q: %w", name, dir, err)
	}

	// LOG-01 audit foundation: bus + AuditLogger + provider capturer. The
	// capturer is the seam that publishes the verbatim shaped body.
	bus := event.NewBus()

	sink, sinkClose, err := audit.OpenFileSink(auditLogPath)
	if err != nil {
		return fmt.Errorf("open audit sink: %w", err)
	}

	if sinkClose != nil {
		defer func() { _ = sinkClose() }()
	}

	audit.NewAuditLogger(bus, sink)

	capturer := func(body []byte, _ map[string]string) {
		bus.Publish(event.RequestShaped{
			VerbatimRequest: body,
			Profile:         prof.Name,
			Timestamp:       time.Now(),
		})
	}

	// Phase 7 (D-08) + 09-01: the tracer builds its provider through the SAME
	// factory seam as the serve path — BuildWithCapturer attaches the
	// RequestCapturer for both shapes with factory-RESOLVED base_url+key. The
	// pre-09-01 hand-rebuilt construction path is gone (Pitfall 8). An
	// uncredentialed build keeps the lazy noCredentialProvider wrapper (D-07).
	factory, providerName, ferr := providerfactory.SetupProviderFactory("", os.Stderr)
	if ferr != nil {
		return fmt.Errorf("setup provider factory: %w", ferr)
	}

	p, err := factory.BuildWithCapturer(providerName, shaper.New(), capturer)
	if err != nil {
		return fmt.Errorf("build tracer provider: %w", err)
	}

	calls, err := loop.Run(ctx, &prof, p, prompt)
	if err != nil {
		return fmt.Errorf("call: %w", err)
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
	abs, err := filepath.Abs("profiles")
	if err == nil {
		return abs
	}

	return "profiles"
}
