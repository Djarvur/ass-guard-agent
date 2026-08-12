package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/redact"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// serveOptions carries the `acp serve` subcommand flags. The profile is loaded
// by name (default zcode); --max-concurrent bounds outbound provider concurrency
// (PARA-04, default 6). WorkDir is where .ass-guard/ transcripts live (default
// cwd). ConfigAddedBoundaries lets a project ADD boundaries (SESS-02).
type serveOptions struct {
	Profile               string
	MaxConcurrent         int
	ProfilesDir           string
	WorkDir               string
	ConfigAddedBoundaries []string
}

// Redact satisfies session.Redactor.
func (redactorAdapter) Redact(b []byte) ([]byte, error) { return redact.Redact(b) }
func (redactorAdapter) ScrubError(err error) string     { return redact.ScrubError(err) }

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
		workDir       string
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
				WorkDir:       workDir,
			})
		},
	}
	c.Flags().StringVar(&profile, "profile", "zcode", "profile name to load (PROF-01)")
	c.Flags().IntVar(&maxConcurrent, "max-concurrent", 6, "max concurrent outbound provider calls across parent + subagents (PARA-04)")
	c.Flags().StringVar(&profilesDir, "profiles-dir", defaultProfilesDir(), "directory containing profile bundles")
	c.Flags().StringVar(&workDir, "work-dir", "", "working directory for .ass-guard/ transcripts (default: cwd)")

	return c
}

// runACPServe constructs the ACP server and runs it until ctx is cancelled or
// stdin reaches EOF. It wires the real Session Core as the TurnRunner (Plan
// 02-05): each session/prompt drives a session.Session whose Provider.Stream
// streams chunks to the event bus; the sessionTurnRunner forwards bus chunks to
// the ACP adapter as session/update notifications.
func runACPServe(ctx context.Context, in io.Reader, out, stderr io.Writer, opts serveOptions) error {
	bus := event.NewBus()

	prof, err := profile.NewLoader(opts.ProfilesDir).Load(opts.Profile)
	if err != nil {
		return err
	}

	runner := &sessionTurnRunner{
		bus:         bus,
		profile:     prof,
		workDir:     opts.WorkDir,
		maxConc:     opts.MaxConcurrent,
		configAdded: opts.ConfigAddedBoundaries,
		makeProvider: func() provider.Provider {
			return provider.NewAnthropicProvider(shaper.New())
		},
	}
	srv := acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner))

	return srv.Serve(ctx)
}

// sessionTurnRunner adapts the Session Core to the ACP TurnRunner interface
// (Plan 02-05). For each session/prompt it creates (or reuses) a Session for the
// ACP sessionId, subscribes a chunk-forwarder to the bus (AgentMessageChunk →
// emit → session/update), and calls Session.Prompt. ctx cancellation (from
// session/cancel) aborts the turn end-to-end (D-16).
type sessionTurnRunner struct {
	bus          *event.Bus
	profile      profile.Profile
	workDir      string
	maxConc      int
	configAdded  []string
	makeProvider func() provider.Provider

	sessions map[string]*session.Session
}

// Run drives one session/prompt through the real Session Core.
func (r *sessionTurnRunner) Run(ctx context.Context, sessionID string, emit acp.ChunkEmitter, prompt []acp.ContentBlock) (string, error) {
	sess := r.sessionFor(sessionID)
	// Subscribe a chunk-forwarder so streamed AgentMessageChunk events become
	// session/update notifications. The forwarder runs until the turn completes.
	ch := r.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	done := make(chan struct{})
	promptDone := make(chan struct{})

	go func() {
		defer func() { done <- struct{}{} }()

		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return
				}

				if c, ok := e.(event.AgentMessageChunk); ok {
					_ = emit.AgentMessageChunk(c.MessageID, c.Content)
				}
			case <-ctx.Done():
				return
			case <-promptDone:
				// Prompt returned; drain any buffered chunks, then exit.
				for {
					select {
					case e := <-ch:
						if c, ok := e.(event.AgentMessageChunk); ok {
							_ = emit.AgentMessageChunk(c.MessageID, c.Content)
						}
					default:
						return
					}
				}
			}
		}
	}()

	blocks := toContentBlocks(prompt)
	stop, err := sess.Prompt(ctx, blocks)

	close(promptDone)
	<-done

	return stop, err
}

// sessionFor returns the Session for sessionID, creating it on first use.
func (r *sessionTurnRunner) sessionFor(sessionID string) *session.Session {
	if r.sessions == nil {
		r.sessions = map[string]*session.Session{}
	}

	if s, ok := r.sessions[sessionID]; ok {
		return s
	}

	dir := r.workDir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	mgr, err := session.NewManager(dir, sessionID, redactorAdapter{})
	if err != nil {
		// Fall back to a no-op manager path; the error is surfaced via Prompt.
		mgr, _ = session.NewManager(filepath.Join(os.TempDir(), "ass-guard"), sessionID, redactorAdapter{})
	}

	maxConc := r.maxConc
	if maxConc < 1 {
		maxConc = provider.DefaultMaxConcurrent
	}

	s := &session.Session{
		Manager:     mgr,
		Projector:   session.NewProjector(r.profile, mgr),
		Provider:    r.makeProvider(),
		Bus:         r.bus,
		Semaphore:   provider.NewSemaphore(maxConc),
		Profile:     r.profile,
		WorkDir:     dir,
		SessionID:   sessionID,
		Catalog:     toolcat.NewCatalog(),
		ConfigAdded: r.configAdded,
	}
	r.sessions[sessionID] = s

	return s
}

// toContentBlocks converts the ACP content blocks to session content blocks.
func toContentBlocks(in []acp.ContentBlock) []session.ContentBlock {
	out := make([]session.ContentBlock, len(in))
	for i, b := range in {
		out[i] = session.ContentBlock{Type: b.Type, Text: b.Text}
	}

	return out
}

// redactorAdapter adapts internal/redact to session.Redactor.
type redactorAdapter struct{}
