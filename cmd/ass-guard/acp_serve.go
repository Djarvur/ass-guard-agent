package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/acpserve"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

const mnd6 = 6

// --- 18-06 (D-10): the CC-parity resume trio's shared resolution home ---
//
// The flags are ROOT-persistent (main.go registers them), so they are typed
// the same at `ass-guard --resume …` (the root RunE delegates to the serve
// flow) and at `ass-guard acp serve --resume …` (the serve RunE resolves the
// inherited flags via cmd.Flags() — cobra resolves parent persistent flags in
// the child). ONE resolver serves both entrypoints; resolution is cwd-scoped
// against the store (the documented 18-06 deviation: CC's cross-project
// registry search needs a store design this phase does not carry).

// pickSentinel is --resume's NoOptDefVal: bare `--resume` parses to it
// (picker mode), `--resume=<target>` carries the target directly, and the
// space form leaves the target as the first positional arg (pflag's
// NoOptDefVal contract — the RunE consumes args[0]).
const pickSentinel = "@picker"

// pickerMaxRows bounds the numbered picker's rows (D-11: the N most recent
// sessions; ListSessions is already recency-ordered, so the first page's
// prefix is the choice set).
const pickerMaxRows = 10

// sessIDPattern is cmd's local copy of the traversal-safe session-id grammar
// (provenance: internal/coreexec/messaging.go — the per-package-copy
// convention, T-12-04-03; internal/session/list.go carries the sibling copy).
// Both alternatives exclude path separators, so an id validated here cannot
// escape the store directory through any later filepath.Join (T-18-13).
var sessIDPattern = regexp.MustCompile(
	`^(sess_[A-Za-z0-9._-]+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// The trio's typed errors (err113 discipline: static sentinels, wrapped with
// directory/candidate context at the raise sites).
var (
	// errResumeWithPrompt: the resume flags start a serve; --prompt runs the
	// one-shot tracer — the two are mutually exclusive.
	errResumeWithPrompt = errors.New("--resume/--continue cannot be combined with --prompt")
	// errResumeNoSessions: the store has nothing to resume.
	errResumeNoSessions = errors.New("no sessions to resume")
	// errResumeAmbiguousName: a name prefix matched more than one title.
	errResumeAmbiguousName = errors.New("ambiguous session name")
	// errResumeUnknownName: a name prefix matched nothing.
	errResumeUnknownName = errors.New("no session with that name")
	// errResumeInvalidTarget: a target that can be neither an id nor a name.
	errResumeInvalidTarget = errors.New("invalid resume target")
)

// resumeFlags is the trio's parsed state as the RunEs read it from any
// command in the tree (the flags are root-persistent).
type resumeFlags struct {
	resume string   // "" absent; pickSentinel bare; otherwise the = form's target
	cont   bool     // --continue / -c
	args   []string // positional args (the space form's target rides args[0])
	prompt string   // --prompt (the one-shot tracer; conflicts with the trio)
}

// active reports whether any resume flag was typed.
func (rf resumeFlags) active() bool { return rf.resume != "" || rf.cont }

// readResumeFlags extracts the trio from a cobra command's resolved flag set
// (cmd.Flags() sees inherited persistent flags in child RunEs too).
func readResumeFlags(cmd *cobra.Command, args []string) resumeFlags {
	resume, _ := cmd.Flags().GetString("resume")
	cont, _ := cmd.Flags().GetBool("continue")
	prompt, _ := cmd.Flags().GetString("prompt")

	return resumeFlags{resume: resume, cont: cont, args: args, prompt: prompt}
}

// resolveResumeTarget resolves the trio against the store rooted at dir.
// --continue takes the newest row (cwd-scoped, CC parity); bare --resume
// funnels the newest rows through the pick seam (the D-11 picker); every
// other form is a direct target — a pattern-valid id passes through
// untouched (validation BEFORE any file access, T-18-13; existence is the
// load engine's typed unknown-session error — one checker, not two), and
// anything else is a session NAME matched against titles (the name never
// joins a path). --resume wins when both resume flags are typed: the
// explicit target loads, and only a TARGET-LESS combination falls back to
// --continue's newest row (never the picker).
func resolveResumeTarget(
	dir string, rf resumeFlags, pick func([]session.SessionHeader) (string, error),
) (string, error) {
	if rf.prompt != "" {
		return "", errResumeWithPrompt
	}

	if rf.cont && rf.resume == "" {
		return continueMostRecent(dir)
	}

	target := rf.resume

	if target == pickSentinel {
		switch {
		case len(rf.args) > 0:
			target = rf.args[0]
		case rf.cont:
			// Bare --resume + --continue: no explicit target, so the newest
			// row wins (the --continue semantics; the picker stays out of the
			// combination — an unattended serve start must not prompt).
			return continueMostRecent(dir)
		default:
			return pickerChoice(dir, pick)
		}
	}

	return resolveDirectTarget(dir, target)
}

// continueMostRecent resolves --continue: ListSessions is lastActivity-desc,
// so the first row is the newest session of THAT directory (tombstones are
// already stat-filtered by the engine). An empty store is a clean not-found
// error naming the directory searched.
func continueMostRecent(dir string) (string, error) {
	rows, _, err := session.ListSessions(dir, "", 0)
	if err != nil {
		return "", fmt.Errorf("resume %s: %w", dir, err)
	}

	if len(rows) == 0 {
		return "", fmt.Errorf("%w in %s (start a session first, or --resume to pick one)", errResumeNoSessions, dir)
	}

	return rows[0].SessionID, nil
}

// pickerChoice resolves bare --resume: the newest pickerMaxRows rows funnel
// through the pick seam (production passes pickResumeSession — the D-11
// numbered picker over the process stdio; tests inject a fake).
func pickerChoice(dir string, pick func([]session.SessionHeader) (string, error)) (string, error) {
	if pick == nil {
		return "", fmt.Errorf("%w: no picker wired", errPickerInput)
	}

	rows, _, err := session.ListSessions(dir, "", 0)
	if err != nil {
		return "", fmt.Errorf("resume %s: %w", dir, err)
	}

	if len(rows) == 0 {
		return "", fmt.Errorf("%w in %s (start a session first)", errResumeNoSessions, dir)
	}

	id, perr := pick(rows[:min(len(rows), pickerMaxRows)])
	if perr != nil {
		return "", perr
	}

	// The picked id came from the store, but re-validate anyway (defense in
	// depth — the load engine rejects it a second time regardless).
	return resolveDirectTarget(dir, id)
}

// resolveDirectTarget resolves one explicit target (id or name form).
// Traversal-shaped or separator-carrying targets reject BEFORE any directory
// scan (T-18-13); pattern-valid ids pass through; names match titles
// case-insensitively by prefix, with ambiguity errors listing every candidate
// (id + title) and miss errors naming the directory searched.
func resolveDirectTarget(dir, target string) (string, error) {
	if sessIDPattern.MatchString(target) {
		return target, nil
	}

	if looksPathlike(target) {
		return "", fmt.Errorf("%w %q (a session id or a name prefix never carries path segments)",
			errResumeInvalidTarget, target)
	}

	rows, _, err := session.ListSessions(dir, "", 0)
	if err != nil {
		return "", fmt.Errorf("resume %s: %w", dir, err)
	}

	needle := strings.ToLower(target)

	var matches []session.SessionHeader

	for _, row := range rows {
		if row.TitlePresent && strings.HasPrefix(strings.ToLower(row.Title), needle) {
			matches = append(matches, row)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0].SessionID, nil
	case 0:
		return "", fmt.Errorf("%w %q in %s", errResumeUnknownName, target, dir)
	default:
		candidates := make([]string, len(matches))

		for i, m := range matches {
			candidates[i] = m.SessionID + " — " + m.Title
		}

		return "", fmt.Errorf("%w %q in %s: did you mean %s?",
			errResumeAmbiguousName, target, dir, strings.Join(candidates, " or "))
	}
}

// looksPathlike reports whether target can be neither an id nor a sane name
// match input: path separators or a dot-dot segment (T-18-13 — reject before
// any file access; titles are still matchable when they carry no such shape).
func looksPathlike(target string) bool {
	return strings.ContainsAny(target, `/\`) || strings.Contains(target, "..")
}

// resolveServeResumeTarget resolves the inherited root-persistent trio at the
// `acp serve` RunE (18-06, D-10): cmd.Flags() resolves parent persistent flags
// in the child RunE, so `ass-guard acp serve --resume <id|name>` and the bare
// picker form work at this entrypoint too, through the ONE shared resolver —
// cwd-scoped against the resolved --work-dir store. "" = no resume typed.
func resolveServeResumeTarget(workDir string, rf resumeFlags) (string, error) {
	if !rf.active() {
		return "", nil
	}

	if rf.prompt != "" {
		return "", errResumeWithPrompt
	}

	resolvedDir, err := acpserve.ResolveWorkDir(workDir)
	if err != nil {
		return "", err //nolint:wrapcheck // flag-resolution error passes through
	}

	return resolveResumeTarget(resolvedDir, rf, pickResumeSession)
}

// pickResumeSession is the D-11 picker composition edge: bare --resume reads
// the numbered choice over the process stdio (rows on stderr, the number from
// stdin — pipe-safe by construction). Var so the resolution path stays
// testable (SelectSession itself takes io.Reader/io.Writer).
//
//nolint:gochecknoglobals // the testable-seam var pattern (the runACPServeCmd extraction precedent)
var pickResumeSession = func(rows []session.SessionHeader) (string, error) {
	return SelectSession(rows, os.Stdin, os.Stderr)
}

// resumeServeDelegate is everything the root resume branch forwards into the
// serve composition (root exposes only the persistent flags; the serve-local
// knobs keep their defaults).
type resumeServeDelegate struct {
	target             string
	auditPath          string
	profilesDir        string
	profilesDirChanged bool
	profileName        string
}

// runServeWithResumeTarget is the root RunE's delegation seam into the `acp
// serve` composition (the runACPServeCmd extraction precedent). Var so tests
// intercept the delegation without spawning a real serve.
//
//nolint:gochecknoglobals // the testable-seam var pattern (the runACPServeCmd extraction precedent)
var runServeWithResumeTarget = func(ctx context.Context, d resumeServeDelegate) error {
	return runACPServeCmd(ctx, d.auditPath, d.profilesDir, "", d.profilesDirChanged,
		d.profileName, mnd6, true, session.DefaultAskTimeout, d.target)
}

// runRootResume is the root RunE's D-10 branch: guard the --prompt
// combination, resolve the target against the cwd store, delegate to the
// serve composition with the resolved target (Options.ResumeTarget — one
// load engine, two entrypoints).
func runRootResume(cmd *cobra.Command, rf resumeFlags) error {
	if rf.prompt != "" {
		return errResumeWithPrompt
	}

	dir, err := acpserve.ResolveWorkDir("")
	if err != nil {
		return err //nolint:wrapcheck // flag-resolution error passes through
	}

	target, rerr := resolveResumeTarget(dir, rf, pickResumeSession)
	if rerr != nil {
		return rerr
	}

	auditPath, _ := cmd.Flags().GetString("audit-log")
	profilesDir, _ := cmd.Flags().GetString(flagProfilesDir)
	profileName, _ := cmd.Flags().GetString("profile")

	return runServeWithResumeTarget(cmd.Context(), resumeServeDelegate{
		target:             target,
		auditPath:          auditPath,
		profilesDir:        profilesDir,
		profilesDirChanged: cmd.Flags().Changed(flagProfilesDir),
		profileName:        profileName,
	})
}

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

			// 18-06 (D-10): the inherited root-persistent resume trio,
			// resolved through the ONE shared resolver.
			resumeTarget, rerr := resolveServeResumeTarget(workDir, readResumeFlags(cmd, args))
			if rerr != nil {
				return rerr
			}

			return runACPServeCmd(ctx, auditPath, profilesDir, workDir, changedProfilesDir,
				profileName, maxConcurrent, !noEngine, askTimeout, resumeTarget)
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
// arrive here as plain values. 18-06 added the trailing resumeTarget: the
// ALREADY-RESOLVED CLI resume target (both the root delegation and the serve
// RunE resolve before this point); "" performs no initial load.
func runACPServeCmd(
	ctx context.Context, auditPath, profilesDir, workDir string, profilesDirChanged bool,
	profileName string, maxConcurrent int, engineEnabled bool,
	askTimeout time.Duration, resumeTarget string,
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
			ResumeTarget:  resumeTarget,
		})
}
