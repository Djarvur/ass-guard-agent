package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/hookdag"
	"github.com/Djarvur/ass-guard-agent/internal/learning"
	"github.com/Djarvur/ass-guard-agent/internal/openspec"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/redact"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
	"github.com/Djarvur/ass-guard-agent/internal/toolexec"
)

// serveOptions carries the `acp serve` subcommand flags. The profile is loaded
// by name (default zcode); --max-concurrent bounds outbound provider concurrency
// (PARA-04, default 6). WorkDir is where .ass-guard/ transcripts live (default
// cwd). ConfigAddedBoundaries lets a project ADD boundaries (SESS-02).
// EngineEnabled (default true; --no-engine disables) wires the Phase-4 unified
// engine + hook-DAG + OpenSpec + learning (Plan 04-05).
type serveOptions struct {
	Profile               string
	MaxConcurrent         int
	ProfilesDir           string
	WorkDir               string
	ConfigAddedBoundaries []string
	EngineEnabled         bool
}

// redactorAdapter adapts internal/redact to session.Redactor.
type redactorAdapter struct{}

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
		profileName   string
		maxConcurrent int
		profilesDir   string
		workDir       string
		noEngine      bool
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

			return runACPServe(ctx, os.Stdin, os.Stdout, os.Stderr, &serveOptions{
				Profile:       profileName,
				MaxConcurrent: maxConcurrent,
				ProfilesDir:   profilesDir,
				WorkDir:       workDir,
				EngineEnabled: !noEngine,
			})
		},
	}
	c.Flags().StringVar(&profileName, "profile", profileZcode, "profile name to load (PROF-01)")
	c.Flags().IntVar(&maxConcurrent, "max-concurrent", 6,
		"max concurrent outbound provider calls across parent + subagents (PARA-04)")
	c.Flags().StringVar(&profilesDir, "profiles-dir", defaultProfilesDir(), "directory containing profile bundles")
	c.Flags().StringVar(&workDir, "work-dir", "", "working directory for .ass-guard/ transcripts (default: cwd)")
	c.Flags().BoolVar(&noEngine, "no-engine", false,
		"disable the Phase-4 unified engine (fall back to manual continue — D-04)")

	return c
}

// runACPServe constructs the ACP server and runs it until ctx is cancelled or
// stdin reaches EOF. It wires the real Session Core as the TurnRunner (Plan
// 02-05): each session/prompt drives a session.Session whose Provider.Stream
// streams chunks to the event bus; the sessionTurnRunner forwards bus chunks to
// the ACP adapter as session/update notifications.
func runACPServe(ctx context.Context, in io.Reader, out, stderr io.Writer, opts *serveOptions) error {
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
	if opts.EngineEnabled {
		err := runner.setupEngine()
		if err != nil {
			// A bad config degrades to defaults, never a server crash (the
			// engine is an observer — D-04 graceful degradation at startup).
			log.Printf("ass-guard: engine setup failed (continuing without engine): %v", err)
		}
	}

	srv := acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner))

	return srv.Serve(ctx)
}

// setupEngine builds the Phase-4 engine wiring (Plan 04-05 D-01/D-13/D-15/D-21):
// the shared catalog + OpenSpec tool registration + the openspec pattern table +
// sessionTurnRunner adapts the Session Core to the ACP TurnRunner interface
// (Plan 02-05). For each session/prompt it creates (or reuses) a Session for the
// ACP sessionId, subscribes a chunk-forwarder to the bus (AgentMessageChunk →
// emit → session/update), and calls Session.Prompt. ctx cancellation (from
// session/cancel) aborts the turn end-to-end (D-16).
//
// Phase-4 wiring (Plan 04-05): when the engine is enabled, Run wraps sess.Prompt
// with engine.Observe — after the user's turn reaches end_turn, the engine's
// dual-signal detector decides continue/hook/ask/nothing. The engine re-enters
// sess.Prompt for continue-injections (real turns through the same bus →
// session/update). A nil engine (engineEnabled=false, e.g. --no-engine) falls
// back to the unwrapped sess.Prompt path (backward-compatible).
type sessionTurnRunner struct {
	bus          *event.Bus
	profile      profile.Profile
	workDir      string
	maxConc      int
	configAdded  []string
	makeProvider func() provider.Provider

	// Phase-4 engine wiring (Plan 04-05). Built once in setupEngine(); nil when
	// the engine is disabled.
	engineEnabled bool
	patternTable  engine.PatternTable
	eng           *engine.Engine
	hookExec      *hookdag.Executor
	hookCfg       []hookdag.Hook
	learned       *learning.Store
	catalog       *toolcat.Catalog // shared catalog (OpenSpec tools registered once)

	sessions map[string]*session.Session
}

// the loaded hook-DAG config + the learning store + the engine + its
// ActionDispatcher. On any error the engine stays disabled (Run falls back to
// the unwrapped sess.Prompt — backward-compatible + D-04 graceful degradation).
func (r *sessionTurnRunner) setupEngine() error {
	catalog := toolcat.NewCatalog()
	r.catalog = catalog

	// OpenSpec config (D-13/D-15) — embedded default; an operator overlay path
	// could be loaded here. Register each command's mutability into the catalog.
	oscfg, err := openspec.DefaultConfig()
	if err != nil {
		return err
	}

	err = openspec.RegisterTools(catalog, oscfg)
	if err != nil {
		return err
	}

	pt, err := openspec.FromConfig(oscfg)
	if err != nil {
		return err
	}

	r.patternTable = pt

	// Hook-DAG config (HOOK-02) — embedded default seeded set.
	hooks, err := hookdag.DefaultHooks()
	if err != nil {
		return err
	}

	r.hookCfg = hooks
	r.hookExec = &hookdag.Executor{Bus: r.bus, Log: slog.Default()}

	// Learning store (LRN-01..04) — versioned .ass-guard/learned.yaml.
	learnedPath := filepath.Join(r.workDirOrDefault(), ".ass-guard", "learned.yaml")

	learned, lerr := learning.Open(learnedPath)
	if lerr == nil {
		r.learned = learned
	} else {
		log.Printf("ass-guard: learning store open failed (continuing without learning): %v", lerr)
	}

	// The engine + dispatcher (ActionDispatcher wiring hook→hookdag, ask→store).
	r.eng = &engine.Engine{Bus: r.bus, Log: slog.Default()}
	r.eng.Dispatcher = &acpDispatcher{
		hooks:   r.hookExec,
		hookCfg: r.hookCfg,
		learned: r.learned,
		bus:     r.bus,
	}
	r.engineEnabled = true

	return nil
}

// workDirOrDefault returns the configured work dir or cwd.
func (r *sessionTurnRunner) workDirOrDefault() string {
	if r.workDir != "" {
		return r.workDir
	}

	wd, _ := os.Getwd()

	return wd
}

// Run drives one session/prompt through the real Session Core.
func (r *sessionTurnRunner) Run(
	ctx context.Context, sessionID string,
	emit acp.ChunkEmitter, prompt []acp.ContentBlock,
) (string, error) {
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
	stop, err := r.runOneTurn(ctx, sess, blocks)

	close(promptDone)
	<-done

	return stop, err
}

// runOneTurn drives ONE ACP session/prompt through the Session Core, wrapping it
// with the Phase-4 engine when enabled (Plan 04-05 D-01). The engine runs AFTER
// sess.Prompt returns end_turn + re-enters sess.Prompt for continue-injections
// (real turns through the same bus → session/update). The original (stop, err)
// are returned to the ACP handler unchanged (the engine is an observer — D-04).
// The engine + the continue-injections all run under the SAME ctx derived from
// the ACP turnCtx (ENG-03 — session/cancel reaches the engine + drains queued
// injections).
func (r *sessionTurnRunner) runOneTurn(
	ctx context.Context, sess *session.Session, blocks []session.ContentBlock,
) (string, error) {
	if !r.engineEnabled || r.eng == nil || r.patternTable == nil {
		// Backward-compatible path: no engine wrap.
		return sess.Prompt(ctx, blocks)
	}
	// Wire the hook-DAG seams to the active session so ActionHook can launch the
	// post-implement/post-phase DAG against the real Session Core (HOOK-05 —
	// send-prompt IS a turn; fresh-context IS a boundary).
	if r.hookExec != nil {
		r.hookExec.Commands = realCommandRunner{}
		r.hookExec.Turns = &hookSessionTurnRunner{sess: sess}
		r.hookExec.Boundaries = &hookSessionBoundaryOpener{mgr: sess.Manager}
	}
	// Wire the engine's transcript writer to the active session's Manager so
	// engine_decision lines land in THIS session's transcript (the Manager is
	// per-session; setupEngine could not bind it).
	r.eng.Manager = sess.Manager
	adapter := &engineTurnRunnerAdapter{sess: sess, mgr: sess.Manager}
	// The engine runs the user prompt + every continue-injection through the
	// adapter (which calls sess.Prompt). Nil userPrompt would make Observe skip
	// the first Run — pass blocks explicitly.
	stop, err := r.eng.Observe(ctx, adapter, r.patternTable, blocks)
	if err == nil && stop == "" {
		stop = stopEndTurn
	}

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
		Projector:   session.NewProjector(&r.profile, mgr),
		Provider:    r.makeProvider(),
		Bus:         r.bus,
		Semaphore:   provider.NewSemaphore(maxConc),
		Profile:     r.profile,
		WorkDir:     dir,
		SessionID:   sessionID,
		Catalog:     r.catalog, // shared: nil when the engine is disabled → NewCatalog below
		ConfigAdded: r.configAdded,
	}
	if s.Catalog == nil {
		s.Catalog = toolcat.NewCatalog()
	}
	// Phase-4 TOOL-04/05: inject the catalog-backed real executor (WebSearch/
	// WebFetch delegate to the configured backend; others call catalog
	// Tool.Execute). Nil Backends ⇒ WebSearch/WebFetch return a not-configured
	// error until the operator configures templates — every other tool still
	// runs via the catalog. When the engine is disabled the stub path is used
	// (SetToolExecutor not called) — backward-compatible.
	if r.engineEnabled {
		s.SetToolExecutor(&toolexec.RealExecutor{Catalog: s.Catalog, Log: slog.Default()})
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

// engineTurnRunnerAdapter adapts the Session Core to the engine.TurnRunner seam
// (Plan 04-05 D-01). Run delegates to sess.Prompt (a real turn); LastTurnOutput
// reads the transcript via Manager.ReadAll to extract the most-recent
// assistant_message text + the turn's tool-call names.
type engineTurnRunnerAdapter struct {
	sess *session.Session
	mgr  *session.Manager
}

// Run drives one turn through the Session Core.
func (a *engineTurnRunnerAdapter) Run(ctx context.Context, prompt []session.ContentBlock) (string, error) {
	return a.sess.Prompt(ctx, prompt)
}

// LastTurnOutput reads the transcript to build the engine's view of the most
// recent turn: the last assistant_message's TurnID + Text + the tool-call names
// recorded under that TurnID (D-02 second signal). A missing assistant_message
// yields an empty TurnOutput (the engine treats it as unmatched ⇒ nothing).
func (a *engineTurnRunnerAdapter) LastTurnOutput() engine.TurnOutput {
	if a.mgr == nil {
		return engine.TurnOutput{}
	}

	lines, err := a.mgr.ReadAll()
	if err != nil {
		return engine.TurnOutput{}
	}

	var lastAssistant *session.Line

	for _, v := range slices.Backward(lines) {
		if v.Type == session.TypeAssistantMessage {
			lastAssistant = &v

			break
		}
	}

	if lastAssistant == nil {
		return engine.TurnOutput{}
	}

	out := engine.TurnOutput{TurnID: lastAssistant.TurnID, Text: lastAssistant.Text}
	for i := range lines {
		if lines[i].TurnID == lastAssistant.TurnID && lines[i].Type == session.TypeToolCall {
			out.ToolCalls = append(out.ToolCalls, lines[i].Name)
		}
	}

	return out
}

// acpDispatcher implements engine.ActionDispatcher, routing the engine's
// hook/ask actions to the real seams (Plan 04-05 T1).
//
//   - Hook maps the matched signal to a hook in the loaded config + launches the
//     hook-DAG via hookdag.Executor (the real CommandRunner/TurnRunner/
//     BoundaryOpener seams are wired lazily per session — see hookSeamsFor).
//   - Ask consults the learning store (Lookup → use the stored answer; else
//     ErrAskPending so the engine surfaces a session/update asking the user).
type acpDispatcher struct {
	hooks   *hookdag.Executor
	hookCfg []hookdag.Hook
	learned *learning.Store
	bus     *event.Bus
}

// Hook launches the hook-DAG matching the trigger derived from the signal. The
// hook executor's seams (CommandRunner/TurnRunner/BoundaryOpener) are wired by
// runOneTurn for the active session before Observe runs, so Hook just selects
// the matching hook + executes it. The signal shape is "hook:<id>"; v1 maps
// proposal-ready/changes-proposed to post-phase, everything else to
// post-implement.
func (d *acpDispatcher) Hook(ctx context.Context, hookSignal, sourceTurnID string) string {
	if d.hooks == nil || len(d.hookCfg) == 0 {
		return "no-hooks-configured"
	}

	trigger := triggerFromSignal(hookSignal)
	for _, h := range d.hookCfg {
		if h.Trigger != trigger {
			continue
		}

		prov := hookdag.Provenance{HookName: h.Name, TriggerStage: trigger, SourceTurnID: sourceTurnID}
		res := d.hooks.Execute(ctx, &h, prov)

		return res.Status
	}

	return "no-matching-hook:" + trigger
}

// Ask consults the learning store. A stored answer (active/candidate) is
// returned; otherwise ErrAskPending surfaces (the engine emits an ask
// EngineDecision + the ACP adapter surfaces it as a session/update).
func (d *acpDispatcher) Ask(_ context.Context, situation string) (string, error) {
	if d.learned == nil {
		return "", engine.ErrAskPending
	}

	if e, ok := d.learned.Lookup(situation); ok {
		return e.Answer, nil
	}

	return "", engine.ErrAskPending
}

// triggerFromSignal extracts the trigger stage from an engine signal string.
// The signal shape is "hook:<id>" (the OpenSpec pattern id) or "text:<id>";
// v1 defaults to "post-implement" unless the id carries an explicit stage.
func triggerFromSignal(hookSignal string) string {
	// Strip a "hook:" / "text:" prefix.
	id := hookSignal
	for _, p := range []string{"hook:", "text:"} {
		if len(id) > len(p) && id[:len(p)] == p {
			id = id[len(p):]
		}
	}

	if id == "changes-proposed" || id == "proposal-ready" {
		return "post-phase"
	}

	return "post-implement"
}

// hookSessionTurnRunner adapts the active Session to the hookdag.TurnRunner seam
// (send-prompt IS a turn — HOOK-05). It converts hookdag.ContentBlock to
// session.ContentBlock + delegates to sess.Prompt.
type hookSessionTurnRunner struct {
	sess *session.Session
}

func (h *hookSessionTurnRunner) Run(ctx context.Context, prompt []hookdag.ContentBlock) (string, error) {
	blocks := make([]session.ContentBlock, len(prompt))
	for i, b := range prompt {
		blocks[i] = session.ContentBlock{Type: b.Type, Text: b.Text}
	}

	return h.sess.Prompt(ctx, blocks)
}

// hookSessionBoundaryOpener adapts the active Manager to the hookdag
// BoundaryOpener seam (fresh-context IS a boundary — HOOK-05).
type hookSessionBoundaryOpener struct {
	mgr    *session.Manager
	turnID string
}

func (h *hookSessionBoundaryOpener) OpenBoundary(_ context.Context, cause string) error {
	if h.mgr == nil {
		return nil
	}

	return h.mgr.AppendBoundary(cause, "", h.turnID)
}

// realCommandRunner is the hookdag.CommandRunner seam: exec a shell command,
// capture stdout/stderr, return the exit code (D-08 exit-code contract).
type realCommandRunner struct{}

//nolint:gocritic // conflicts w/ nonamedreturns
func (realCommandRunner) Run(ctx context.Context, command string, args []string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0

	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			return stdout.String(), stderr.String(), 0, err
		}
	}

	return stdout.String(), stderr.String(), code, nil
}
