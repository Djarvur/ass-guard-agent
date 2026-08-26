package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/acpserve"
	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/hookdag"
	"github.com/Djarvur/ass-guard-agent/internal/learning"
	mcp "github.com/Djarvur/ass-guard-agent/internal/mcp"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/openspec"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/redact"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
	"github.com/Djarvur/ass-guard-agent/internal/toolexec"
)

const (
	mnd6            = 6
	mcpStartTimeout = 30 * time.Second

	// Phase-8 command-boundary vocabulary (08-04 D-11): the boundary cause
	// prefix for a mutating command (mutability values come from the openspec
	// config's exported vocabulary).
	mutatingCommandCause = "mutating-command:"

	// skillToolName is the captured core tool the skills closure overrides
	// (08-05 — the captured catalog entry keeps its schema; only Execute is
	// replaced).
	skillToolName = "Skill"

	// Hook-stage vocabulary (08-06): stage-bearing pattern ids select their
	// own hook stage; un-staged ids keep the v1.0 default.
	stagePostImplement = "post-implement"
	stagePostExplore   = "post-explore"
	stagePostPropose   = "post-propose"
)

// stubExecResult is the canned tool result for the non-engine path (mirrors
// session.stubResultMsg). Used by stubCatalogExec so MCPExecutor has a working
// inner executor when the engine is disabled (--no-engine).
const stubExecResult = `{"output":"stubbed (engine disabled)"}`

// nopHost returns a no-op MCP host (zero servers). CallTool on it returns an
// "unknown server" error; Close is a no-op. Used when .mcp.json is absent or
// load fails so the session always has a non-nil host to wrap/reap.
func nopHost() *mcp.Host { return &mcp.Host{} }

// stubCatalogExec is the ToolExecutor for the non-engine path: it returns the
// canned stub result for every non-mcp tool (mirrors session.stubExecutor). MCP
// calls never reach it (MCPExecutor intercepts them first).
type stubCatalogExec struct{}

// toProfileDecls converts toolcat Decls to profile Decls (structurally identical
// types; the Shaper consumes profile.Decl). Used to merge MCP tool decls into
// the per-session profile copy.
func toProfileDecls(in []toolcat.Decl) []profile.Decl {
	out := make([]profile.Decl, len(in))
	for i, d := range in {
		out[i] = profile.Decl{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema}
	}

	return out
}

// Execute returns the canned stub result for every non-mcp tool.
func (stubCatalogExec) Execute(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(stubExecResult), nil
}

// redactorAdapter adapts internal/redact to session.Redactor.
type redactorAdapter struct{}

// Redact satisfies session.Redactor.
func (redactorAdapter) Redact(b []byte) ([]byte, error) {
	out, err := redact.Redact(b)
	if err != nil {
		return nil, fmt.Errorf("redact: %w", err)
	}

	return out, nil
}

func (redactorAdapter) ScrubError(err error) string { return redact.ScrubError(err) }

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
		askTimeout    time.Duration
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

	return runACPServe(ctx, os.Stdin, os.Stdout, os.Stderr, &acpserve.Options{
		Profile:       profileName,
		MaxConcurrent: maxConcurrent,
		ProfilesDir:   resolvedProfilesDir,
		WorkDir:       resolvedWorkDir,
		EngineEnabled: engineEnabled,
		AskTimeout:    askTimeout,
		AuditLogPath:  auditPath,
	})
}

// runACPServe constructs the ACP server and runs it until ctx is cancelled or
// stdin reaches EOF. It wires the real Session Core as the TurnRunner (Plan
// 02-05): each session/prompt drives a session.Session whose Provider.Stream
// streams chunks to the event bus; the sessionTurnRunner forwards bus chunks to
// the ACP adapter as session/update notifications.
//
// TRANSITIONAL (plan 15-05, dissolved by 15-06): the pre-runner pipeline lives
// in acpserve.PrepareServe and the post-engine statements in acpserve.FinishServe;
// the runner literal + its two setup calls stay here until the runner family
// relocates to internal/runtime.
func runACPServe(ctx context.Context, in io.Reader, out, stderr io.Writer, opts *acpserve.Options) error {
	prep, err := acpserve.PrepareServe(ctx, stderr, opts)
	if err != nil {
		return err //nolint:wrapcheck // pipeline error passes through
	}

	runner := &sessionTurnRunner{
		bus:          prep.Bus,
		bodyStore:    prep.BodyStore,
		profile:      prep.Profile,
		workDir:      opts.WorkDir,
		maxConc:      opts.MaxConcurrent,
		configAdded:  opts.ConfigAddedBoundaries,
		askTimeout:   opts.AskTimeout,
		serveCtx:     ctx,
		schedCfg:     prep.SchedCfg,
		providerName: prep.ProviderName,
		stderr:       stderr,
		makeProvider: func(capturer provider.RequestCapturer) provider.Provider {
			// 09-01: the SINGLE factory seam — the same construction the
			// tracer uses (the divergent copy is gone; Pitfall 8).
			// Construction errors keep the existing degradation semantics
			// (ignore, the no-provider path surfaces at first use).
			p, _ := prep.Factory.BuildWithCapturer(prep.ProviderName, shaper.New(), capturer)

			return p
		},
	}
	// Slash-command registry (08-04): load ONCE at startup, engine-independent
	// (the engine-off path expands too). A failed load degrades — turns run on
	// plain text (see loadCommandRegistry).
	runner.loadCommandRegistry()

	if opts.EngineEnabled {
		err := runner.setupEngine()
		if err != nil {
			// A bad config degrades to defaults, never a server crash (the
			// engine is an observer — D-04 graceful degradation at startup).
			log.Printf("ass-guard: engine setup failed (continuing without engine): %v", err)
		}
	}

	return acpserve.FinishServe(ctx, in, out, stderr, opts, runner, acpserve.FinishHooks{ //nolint:lll,wrapcheck // thin delegation
		AssignSchedule: func(store *sched.ScheduleStore) { runner.schedule = store },
		InjectEmitter:  func(emit func(sessionID string) acp.ChunkEmitter) { runner.emitFor = emit },
		StartScheduler: runner.startScheduler,
		CloseSessions:  runner.closeAllSessions,
	})
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
	makeProvider func(capturer provider.RequestCapturer) provider.Provider
	// askTimeout is the D-01 AskUserQuestion wait threaded to every
	// sessionFor-built broker (12-01). runACPServe always sets it from the
	// --ask-timeout flag (default 10m; 0 = block forever); zero-value test
	// runners arm no timer (block-forever — safe for tests).
	askTimeout time.Duration
	// serveCtx is the server-lifetime context (set once in runACPServe from
	// the ACP server ctx). Per-session TranscriptWriters derive from it — a
	// writer bound to a per-TURN ctx would die after turn 1 (09-01 T2 Test 8's
	// pitfall). Test runners may leave it nil; sessionFor falls back to
	// context.Background() there.
	//
	//nolint:containedctx // deliberate serve-lifetime ctx storage (09-01 T2)
	serveCtx context.Context
	// bodyStore is the process-wide capped audit body store (09-05, AUD-03):
	// ONE store per serve process, shared by every session's writer. nil in
	// test runners → metadata lines with refs but no persisted bodies.
	bodyStore *audit.BodyStore

	// schedCfg is the loaded scheduling config from startup (14-05, EARLY-05):
	// the light-tier subagent routing at sessionFor resolves tiers.light
	// through the SAME resolver that picked the session provider. nil in test
	// runners → no subagent model override (the documented default).
	schedCfg *modelrouting.Config

	// providerName is the session provider the factory builds (the heavy-tier
	// resolution's pick, 14-05): the same-provider check for the light binding
	// compares against it. Kept beside schedCfg so both come from the one
	// startup load.
	providerName string

	// stderr is the serve-lifecycle diagnostics writer (transport discipline:
	// stdout stays ACP-only). The 14-05 light-tier degrade warning lands here;
	// nil (test runners that never set one) falls back to os.Stderr.
	stderr io.Writer

	// Phase-4 engine wiring (Plan 04-05). Built once in setupEngine(); nil when
	// the engine is disabled.
	engineEnabled bool
	patternTable  engine.PatternTable
	eng           *engine.Engine
	hookExec      *hookdag.Executor
	hookCfg       []hookdag.Hook
	learned       *learning.Store
	catalog       *toolcat.Catalog // shared catalog (OpenSpec tools registered once)

	// Phase-8 slash-command expansion (08-04): the discovered command registry
	// (loaded ONCE at startup — see loadCommandRegistry) + the command
	// mutability table (D-11 boundaries, T3). A failed load leaves both zero —
	// expansion no-ops and turns proceed on plain text (graceful degradation).
	reg           ecosys.Registry
	cmdMutability map[string]string

	// mcpServers is the NON-project MCP set from ecosys.Discover (12-02:
	// user-scope ~/.claude.json OVER plugin-bundled .mcp.json — the two lowest
	// MCP layers). spawnMCP merges it BELOW the project .mcp.json (project
	// wins on name collision — the chain direction).
	mcpServers []ecosys.ServerConfig

	sessions map[string]*session.Session

	// 12-07 (ACP-04/D-02) cron wiring state — see cron_wiring.go:
	// turnMus is the per-session turn serialization (queue-behind-active-turn);
	// turnActive marks client-driven Runs (the session forwarder's mute flag);
	// sessMu guards lastSessionID + automationProvenance; schedule is the
	// PER-PROJECT store (workDir-scoped, shared across the project's sessions);
	// schedTick/schedStop/catchUpOnce drive the serve-lifetime scheduler
	// goroutine + the one-per-serve catch-up; emitFor builds the session
	// chunk emitter for SERVER-DRIVEN turns (WINDOWS #3 — nil in tests).
	turnMus              sync.Map // sessionID -> *sync.Mutex
	turnActive           sync.Map // sessionID -> *atomic.Bool
	sessMu               sync.Mutex
	lastSessionID        string
	automationProvenance string
	schedule             *sched.ScheduleStore
	schedTick            time.Duration
	schedStop            func()
	catchUpOnce          sync.Once
	emitFor              func(sessionID string) acp.ChunkEmitter

	// 13-00 park state: parkedCancels holds each session's parked-chain ctx
	// cancels (drained by CloseSession/closeAllSessions — the D-03 off-switch
	// extended to parked chains); activeChains counts running engine chains
	// per session (WaitChainIdle's input — the eval/E2E seam's
	// wait-through-suspension).
	parkedMu      sync.Mutex
	parkedCancels map[string]map[*parkedChain]struct{}
	chainMu       sync.Mutex
	activeChains  map[string]int

	// 13-03 (D-05): the advisory-note dedupe — sessionID → set of seen
	// advisory classes. The ENGINE stays stateless (observe.go's design
	// invariant); this WRAPPER holds the per-session state (the reg/
	// patternTable precedent). First-per-class-per-session is client-visible;
	// repeats increment the audit trail only.
	advisoryMu   sync.Mutex
	advisorySeen map[string]map[string]bool
}

// the loaded hook-DAG config + the learning store + the engine + its
// ActionDispatcher. On any error the engine stays disabled (Run falls back to
// the unwrapped sess.Prompt — backward-compatible + D-04 graceful degradation).
func (r *sessionTurnRunner) setupEngine() error { //nolint:funcorder // ordering groups related logic
	catalog := toolcat.NewCatalog()
	r.catalog = catalog

	// OpenSpec config (D-13/D-15) — embedded default; an operator overlay path
	// could be loaded here. Register each command's mutability into the catalog.
	oscfg, err := openspec.DefaultConfig()
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	err = openspec.RegisterTools(catalog, oscfg)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	pt, err := openspec.FromConfig(oscfg)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	r.patternTable = pt

	// Hook-DAG config (HOOK-02) — embedded default seeded set.
	hooks, err := hookdag.DefaultHooks()
	if err != nil {
		return fmt.Errorf("call: %w", err)
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
	// nextPromptFor resolves DYNAMICALLY through the pattern-table interface so
	// the table remains the single source of truth (tests may swap it after
	// setup; the dispatcher follows).
	r.eng.Dispatcher = &acpDispatcher{
		hooks:   r.hookExec,
		hookCfg: r.hookCfg,
		learned: r.learned,
		bus:     r.bus,
		nextPromptFor: func(patternID string) string {
			if np, ok := r.patternTable.(patternNextPrompter); ok {
				return np.NextPromptFor(patternID)
			}

			return ""
		},
	}
	r.engineEnabled = true

	return nil
}

// workDirOrDefault returns the configured work dir or cwd.
func (r *sessionTurnRunner) workDirOrDefault() string { //nolint:funcorder // ordering groups related logic
	if r.workDir != "" {
		return r.workDir
	}

	wd, _ := os.Getwd()

	return wd
}

// loadCommandRegistry loads the ecosys command registry (08-04) + the command
// mutability table (D-11) ONCE at startup. Any failure is logged to stderr and
// leaves the fields zero — expansion no-ops and every turn proceeds on plain
// text (T-8-16: an expansion problem NEVER becomes a turn failure or an ACP
// error). Tests call it explicitly after planting fixtures; production calls
// it from runACPServe.
func (r *sessionTurnRunner) loadCommandRegistry() { //nolint:funcorder // startup helper grouped with engine wiring
	reg, servers, err := ecosys.Discover(r.workDirOrDefault())
	if err != nil {
		// Drop to zero (do NOT serve a stale registry): the registry mirrors
		// the on-disk command tree, and an unreadable tree means expansion is
		// OFF — turns proceed on plain text (T-8-16).
		log.Printf("ass-guard: command registry load failed (continuing without slash expansion): %v", err)

		r.reg = ecosys.Registry{}
		r.cmdMutability = nil
		r.mcpServers = nil

		return
	}

	r.reg = reg
	r.mcpServers = servers

	oscfg, cerr := openspec.DefaultConfig()
	if cerr != nil {
		log.Printf("ass-guard: openspec config load failed (continuing without command boundaries): %v", cerr)

		return
	}

	r.cmdMutability = oscfg.CommandMutability
}

// expandUserBlocks applies slash-command expansion to the FIRST text block of
// a prompt (08-04, CMD-02): when it parses as an invocation AND the key is in
// the registry, the block's text becomes the command's expanded body (zcode
// substitution semantics), a command_provenance line is written next to the
// upcoming user message (D-02 — the typed command is metadata, replay shows
// the model what it saw), and a mutating command opens a context boundary
// BEFORE the turn (D-11 — the lean window resets for the expanded stage).
// Every other case returns the blocks UNCHANGED (unknown /foo is ordinary
// text — no match, nothing happens). All transcript-write errors degrade to
// stderr logs — never ACP errors.
func (r *sessionTurnRunner) expandUserBlocks( //nolint:funcorder // one pipeline; grouped with the turn seam
	sess *session.Session, blocks []session.ContentBlock,
) []session.ContentBlock {
	idx := firstTextBlockIndex(blocks)
	if idx < 0 {
		return blocks
	}

	key, args, ok := ecosys.ParseInvocation(blocks[idx].Text)
	if !ok {
		return blocks
	}

	cmd, found := r.reg.Commands[key]
	if !found {
		return blocks
	}

	out := append([]session.ContentBlock(nil), blocks...)
	out[idx] = session.ContentBlock{Type: blockText, Text: cmd.Expand(args)}

	if sess == nil || sess.Manager == nil {
		return out
	}

	// Mutating commands ALWAYS open the boundary first (D-11): the projector's
	// ReadLastBoundary reset fires for the expanded stage exactly as for a
	// tool-call boundary.
	if r.cmdMutability[key] == openspec.MutabilityMutating {
		err := sess.Manager.AppendBoundary(mutatingCommandCause+key, cmd.Path, "")
		if err != nil {
			log.Printf("ass-guard: command boundary write failed (continuing): %v", err)
		}
	}

	// Provenance records which file answered the invocation (CMD-05 /
	// T-8-15): command key + source file + the typed args.
	err := sess.Manager.AppendCommandProvenance("", key, cmd.Path, args)
	if err != nil {
		log.Printf("ass-guard: command provenance write failed (continuing): %v", err)
	}

	return out
}

// firstTextBlockIndex returns the index of the first text-typed block, or -1.
func firstTextBlockIndex(blocks []session.ContentBlock) int {
	for i, b := range blocks {
		if b.Type == blockText {
			return i
		}
	}

	return -1
}

// invocationFor resolves the first text block of blocks as a registry command
// invocation: (key, args, true) when it parses AND the key is registered;
// ("", "", false) otherwise. The single parse both the hybrid chaining seam
// (TurnOutput.StartedBy — findings-6 disposition; the key comes from the
// PROMPT-side invocation only, never from assistant/tool content) and the
// scenario-subject forwarding rule consume; it is exactly the resolution
// expandUserBlocks acts on.
func (r *sessionTurnRunner) invocationFor( //nolint:funcorder,nonamedreturns // sibling of expandUserBlocks
	blocks []session.ContentBlock,
) (key, args string, ok bool) {
	idx := firstTextBlockIndex(blocks)
	if idx < 0 {
		return "", "", false
	}

	key, args, parsed := ecosys.ParseInvocation(blocks[idx].Text)
	if !parsed {
		return "", "", false
	}

	if _, found := r.reg.Commands[key]; !found {
		return "", "", false
	}

	return key, args, true
}

// Run drives one session/prompt through the real Session Core.
func (r *sessionTurnRunner) Run(
	ctx context.Context, sessionID string,
	emit acp.ChunkEmitter, prompt []acp.ContentBlock,
) (string, error) {
	sess := r.sessionFor(ctx, sessionID)

	// 12-07 (D-02 queue semantics): the per-session turn serialization — the
	// whole turn (ask-reply resumes included) holds the session mutex, so an
	// automation firing QUEUES behind it instead of interrupting, and client
	// turns serialize among themselves.
	turnMu := r.sessionTurnMu(sessionID)
	turnMu.Lock()

	defer turnMu.Unlock()

	// The session-lifetime forwarder mutes while this client turn is active
	// (Run's own forwarder below owns these chunks — WINDOWS #3's split).
	r.markClientTurn(sessionID, true)

	defer r.markClientTurn(sessionID, false)

	// Subscribe a chunk-forwarder so streamed AgentMessageChunk events become
	// session/update notifications. The forwarder runs until the turn completes
	// and is UNSUBSCRIBED when Run returns — a leaked dead subscriber's buffer
	// fills and wedges every later turn's chunk publishes (the stack-proven
	// 08-15 multi-stage stall: Bus.Publish blocked on the dead channel).
	ch := r.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)

	defer r.bus.Unsubscribe("AgentMessageChunk", ch)

	done := make(chan struct{})
	promptDone := make(chan struct{})

	startChunkForwarder(ctx, ch, emit, promptDone, done)

	blocks := toContentBlocks(prompt)

	// 12-01 reply routing (ACP-01): a prompt arriving while an ask is pending
	// is the OPERATOR'S ANSWER, not a new turn (see routeAskReply).
	if stop, handled := r.routeAskReply(ctx, sess, blocks); handled {
		close(promptDone)
		<-done

		return stop, nil
	}

	// Slash-command expansion (08-04): a leading /opsx:* invocation becomes
	// the command's expanded body BEFORE the turn runs. On the ENGINE path the
	// adapter expands inside its Run (see engineTurnRunnerAdapter.Run — it must
	// see the RAW invocation to answer TurnOutput.StartedBy, the hybrid
	// chaining input); on the engine-off path (and when the engine degraded at
	// startup) Run expands here. The expansion semantics (body + provenance +
	// boundary before sess.Prompt) are identical on both paths.
	if !r.engineEnabled || r.eng == nil || r.patternTable == nil {
		blocks = r.expandUserBlocks(sess, blocks)
	}

	// 13-03 (D-02/D-05): collect the turn's advisory decisions. Subscribed
	// BEFORE runOneTurn (the decisions publish during the turn — the bus
	// DROPS events with no subscriber); the collector mirrors the chunk
	// forwarder's promptDone/done drain, and AFTER the forwarder has drained
	// Run applies the per-session dedupe and emits the note DIRECTLY through
	// the in-hand emitter (a post-turn bus publish would be lost — the
	// PATTERNS timing hazard).
	advCh := r.bus.Subscribe("EngineDecision", event.BufEngineDecision)

	advDone := make(chan *advisoryNote, 1)

	go r.collectAdvisory(advCh, promptDone, advDone)

	stop, err := r.runOneTurn(ctx, sess, blocks)

	close(promptDone)
	<-done

	if adv := <-advDone; adv != nil && r.advisoryNoteDue(sessionID, adv.class) {
		_ = emit.AgentMessageChunk(adv.turnID, adv.text)
	}

	return mapAskStop(stop), err
}

// mapAskStop maps the INTERNAL ask-suspension stop marker to the ACP-facing
// stopReason of a completed turn (12-01, ACP-01): the client received its
// prompt response — the question arrived just before as a client-visible
// update, and the pending ask lives in the session broker. Every other stop
// reason passes through unchanged.
func mapAskStop(stop string) string {
	if stop == stopAskACP {
		return stopEndTurn
	}

	return stop
}

// stopAskACP mirrors session's ask stop marker (kept local: internal/session
// owns the vocabulary; the ACP layer only needs the one mapping).
const stopAskACP = "ask"

// startChunkForwarder spawns the per-Run chunk forwarder: streamed
// AgentMessageChunk events become session/update notifications until the turn
// completes, then any buffered chunks drain before the goroutine exits (the
// caller signals promptDone + waits on done).
func startChunkForwarder(
	ctx context.Context,
	ch <-chan event.Event,
	emit acp.ChunkEmitter,
	promptDone, done chan struct{},
) {
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
}

// runOneTurn drives ONE ACP session/prompt through the Session Core, wrapping it
// with the Phase-4 engine when enabled (Plan 04-05 D-01). The engine runs AFTER
// sess.Prompt returns end_turn + re-enters sess.Prompt for continue-injections
// (real turns through the same bus → session/update). The original (stop, err)
// are returned to the ACP handler unchanged (the engine is an observer — D-04).
// The engine + the continue-injections all run under the SAME ctx derived from
// the ACP turnCtx (ENG-03 — session/cancel reaches the engine + drains queued
// injections).
//
//nolint:funcorder,contextcheck,funlen // grouping; serveCtx-derived park; one flow
func (r *sessionTurnRunner) runOneTurn(
	ctx context.Context, sess *session.Session, blocks []session.ContentBlock,
) (string, error) {
	if !r.engineEnabled || r.eng == nil || r.patternTable == nil {
		// Backward-compatible path: no engine wrap.
		return sess.Prompt(ctx, blocks) //nolint:wrapcheck // session delegation
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

	// 13-00 THE PARK (the manager ruling's route 1): the engine chain runs on
	// a goroutine under a per-session parked-chain ctx derived from serveCtx
	// (the same discipline the D-01 timer's resumeCtx uses — the suspending
	// request's ctx must not kill the chain). When the chain suspends on an
	// ask, the adapter's AskSettle fires the signal AFTER the first-turn ask
	// decision is on disk, and THIS caller returns the suspension to the ACP
	// layer NOW (mapAskStop → a completed turn — the live-proven wire
	// contract: no stuck spinner, no reply deadlock) while the Observe
	// continuation parks. Post-settle injections hold the session turn mutex
	// per turn (12-07 queue semantics) and stream through the session-lifetime
	// forwarder (WINDOWS #3). cancelParkedChains — reached from
	// CloseSession (session/cancel + logout) and closeAllSessions (serve end)
	// — cancels the parked ctx (D-03 stays the only off-switch).
	sessionID := sess.SessionID

	//nolint:contextcheck // deliberately serveCtx-derived: the request ctx must not bound the parked chain
	parkedCtx, parkedCancel := context.WithCancel(r.serveCtxOrBackground())
	pc := &parkedChain{cancel: parkedCancel}

	r.registerParkedChain(sessionID, pc)

	// ENG-03/D-16 keeps its reach: the request ctx dying (session/cancel's
	// cancelTurn while the turn is still live, or the serve ctx ending) must
	// drain the chain — the watchdog folds request-ctx death into the parked
	// cancel. (The response's own return does NOT cancel the request ctx —
	// handleSessionPrompt only deregisters st.setCancel — so the park
	// survives it, by design.)
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				parkedCancel()
			case <-parkedCtx.Done():
			}
		}()
	}

	adapter := &engineTurnRunnerAdapter{sess: sess, mgr: sess.Manager, r: r}
	suspension := make(chan struct{}, 1)
	adapter.onSuspended = func() {
		// Runs on the engine goroutine: arming parkMu here orders it before
		// any post-settle injection Run (same goroutine), and the buffered
		// signal never blocks the chain.
		adapter.parkMu = r.sessionTurnMu(sessionID)

		suspension <- struct{}{}
	}

	type observeResult struct {
		stop string
		err  error
	}

	done := make(chan observeResult, 1)

	r.chainEnter(sessionID)

	go func() {
		stop, err := r.eng.Observe(parkedCtx, adapter, r.patternTable, blocks)

		// Idle + unregister BEFORE the report so a WaitChainIdle following
		// the response never waits on a finished chain.
		r.chainExit(sessionID)
		r.unregisterParkedChain(sessionID, pc)
		parkedCancel()

		done <- observeResult{stop, err}
	}()

	select {
	case <-suspension:
		// The chain parked: return the suspension marker; the ACP layer maps
		// it to a completed turn and the reply (or D-01 timer) resumes under
		// the SAME session — turnMu is FREE (this Run returns), exactly the
		// 12-01 reply-routing contract.
		return stopAskACP, nil
	case res := <-done:
		stop, err := res.stop, res.err
		if err == nil && stop == "" {
			stop = stopEndTurn
		}

		if err != nil {
			return stop, fmt.Errorf("call: %w", err)
		}

		return stop, nil
	}
}

// parkedChain is one parked engine chain's registration (the struct makes the
// cancel func a comparable map key).
type parkedChain struct {
	cancel context.CancelFunc
}

// registerParkedChain records a parked chain's cancel func for the session
// (13-00): cancelParkedChains — CloseSession (session/cancel + logout) and
// closeAllSessions (serve end) — drains every parked chain of that session.
//
//nolint:funcorder // park helper group
func (r *sessionTurnRunner) registerParkedChain(sessionID string, pc *parkedChain) {
	r.parkedMu.Lock()
	defer r.parkedMu.Unlock()

	if r.parkedCancels == nil {
		r.parkedCancels = make(map[string]map[*parkedChain]struct{})
	}

	if r.parkedCancels[sessionID] == nil {
		r.parkedCancels[sessionID] = make(map[*parkedChain]struct{})
	}

	r.parkedCancels[sessionID][pc] = struct{}{}
}

// unregisterParkedChain removes a finished chain's registration (idempotent;
// the chain exited on its own — no cancel fired).
//
//nolint:funcorder // park helper group
func (r *sessionTurnRunner) unregisterParkedChain(sessionID string, pc *parkedChain) {
	r.parkedMu.Lock()
	defer r.parkedMu.Unlock()

	if r.parkedCancels[sessionID] != nil {
		delete(r.parkedCancels[sessionID], pc)
	}
}

// cancelParkedChains cancels every parked chain of the session (the
// session/cancel + logout + serve-end drain).
func (r *sessionTurnRunner) cancelParkedChains(sessionID string) { //nolint:funcorder // park helper group
	r.parkedMu.Lock()
	chains := r.parkedCancels[sessionID]
	delete(r.parkedCancels, sessionID)
	r.parkedMu.Unlock()

	for pc := range chains {
		pc.cancel()
	}
}

// chainEnter/chainExit/chainCount track the active engine chains per session
// (13-00): WaitChainIdle blocks while any chain — parked or running — is
// active, giving the eval/E2E seam the serve loop's wait-through-suspension
// semantics without re-driving anything.
func (r *sessionTurnRunner) chainEnter(sessionID string) { //nolint:funcorder // idle-tracking group
	r.chainMu.Lock()
	defer r.chainMu.Unlock()

	if r.activeChains == nil {
		r.activeChains = make(map[string]int)
	}

	r.activeChains[sessionID]++
}

func (r *sessionTurnRunner) chainExit(sessionID string) { //nolint:funcorder // idle-tracking group
	r.chainMu.Lock()
	defer r.chainMu.Unlock()

	if r.activeChains[sessionID] > 0 {
		r.activeChains[sessionID]--
	}
}

func (r *sessionTurnRunner) chainCount(sessionID string) int { //nolint:funcorder // idle-tracking group
	r.chainMu.Lock()
	defer r.chainMu.Unlock()

	return r.activeChains[sessionID]
}

// WaitChainIdle blocks until no engine chain is active for the session (the
// parked-ask resume + its injections all finished) or ctx dies. The harness
// seam (opsxRunnerSeam.RunPrompt / runStageTyped) consumes it — one engine
// loop, two callers, identical wait-through-suspension semantics.
func (r *sessionTurnRunner) WaitChainIdle(ctx context.Context, sessionID string) bool {
	for {
		if r.chainCount(sessionID) == 0 {
			return true
		}

		if ctx.Err() != nil {
			return false
		}

		time.Sleep(chainIdlePollInterval)
	}
}

// chainIdlePollInterval is WaitChainIdle's poll granularity (13-00): idle
// waits are seconds-scale; 10ms keeps the poll cheap.
const chainIdlePollInterval = 10 * time.Millisecond

// sessionFor returns the Session for sessionID, creating it on first use.
func (r *sessionTurnRunner) sessionFor( //nolint:funcorder,funlen,maintidx // grouping keeps the turn pipeline together
	ctx context.Context, sessionID string,
) *session.Session {
	// sessMu spans the WHOLE construction: a concurrent sessionFor for the
	// same id (the async catch-up firing racing the first Run — 12-07) must
	// never double-construct a session (two transcripts, one overwritten).
	r.sessMu.Lock()
	defer r.sessMu.Unlock()

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

	// 14-01 (EARLY-01): the shadow-git checkpoint store — DEFAULT ON, no
	// flag in v1 (the reversibility backstop only works if it is always
	// there; a disable knob is a post-adoption config option if the operator
	// asks). One store per WORKSPACE; every parent turn snapshots at entry.
	// An open failure degrades LOUDLY to a session without checkpointing
	// (the AUD-03 audit-write discipline — never a serve refusal).
	var ckptStore *checkpoint.Store

	st, cerr := checkpoint.Open(dir) //nolint:contextcheck // plan-pinned signature: Store.Open carries no ctx
	if cerr == nil {
		ckptStore = st
	} else {
		log.Printf("ass-guard: checkpoint store disabled for %s (%v) — turns run WITHOUT undo snapshots", dir, cerr)
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

	// Phase 5 (Plan 05-01): spawn configured MCP servers + bridge their tools.
	// MCP is opt-in — a missing .mcp.json yields an empty host (no servers, no
	// decls). The host lives for the session and is reaped via OnClose (D-02).
	mcpHost, mcpDecls := r.spawnMCP(ctx, dir)

	// Per-session profile COPY: append MCP tool decls so the Shaper surfaces them
	// to the model (D-01/D-16). r.profile is shared; the double-append makes a
	// fresh slice so the shared profile is NEVER mutated.
	prof := r.profile
	if len(mcpDecls) > 0 {
		prof.Tools = append(append([]profile.Decl(nil), r.profile.Tools...), toProfileDecls(mcpDecls)...)
	}

	// Runtime cwd composition (the 08-09 profile-fidelity finding): the
	// mimicry target composes its env block ("Primary working directory: …")
	// from the RUNTIME cwd per session — replaying the captured value
	// statically told the model it stood in the CAPTURED repo (the E2E's
	// explore leg read the real repo instead of the scratch). Form identical
	// to the capture; value = this session's working directory. The System
	// slice is COPIED first (a struct copy alone would share the backing
	// array — the shared r.profile must never be mutated; the parity path
	// composes the unmodified capture).
	prof.System = append([]profile.TextBlock(nil), prof.System...)
	shaper.ComposeRuntimeWorkDir(&prof, dir)

	// Phase 8 (08-05, CMD-06): merge the skills listing into the profile COPY
	// in the captured zcode shape (a dedicated system-role listing — see
	// ecosys.SkillListing's captured-source note). Empty registry → no merge.
	// KNOWN PLACEMENT NUISANCE: the capture places this listing as its own
	// system message AFTER the first user message; ass-guard's Shaper today
	// supports only leading system blocks, so the listing rides as a trailing
	// System TextBlock — a documented divergence for the Phase-9 re-capture.
	if listing := ecosys.SkillListing(r.reg); listing != "" {
		prof.System = append(append([]profile.TextBlock(nil), prof.System...),
			profile.TextBlock{Type: blockText, Text: listing})
	}

	// 12-02 (Task 3): the agent-type listing — the same dedicated-system-block
	// dynamic merge as the skills listing, in the captured Agent-tool
	// type-entry shape. Discovered definitions (plugin-bundled AND first-class
	// `.claude/agents/`) surface here AND as spawnable types below.
	if agentListing := ecosys.AgentListing(r.reg); agentListing != "" {
		prof.System = append(append([]profile.TextBlock(nil), prof.System...),
			profile.TextBlock{Type: blockText, Text: agentListing})
	}

	// Per-session catalog: clone the shared engine catalog (OpenSpec + core) so
	// MCP tools never leak across sessions or back into r.catalog (D-16).
	var sCatalog *toolcat.Catalog
	if r.catalog != nil {
		sCatalog = r.catalog.Clone()
	} else {
		sCatalog = toolcat.NewCatalog()
	}

	mcpHost.Register(sCatalog)

	// Phase 8 (08-05, D-05): make the captured Skill tool EXECUTABLE per
	// session — override ONLY Execute (the captured Description/InputSchema
	// stay byte-identical; resolution is by registry key via SkillExecute).
	if core, ok := sCatalog.Get(skillToolName); ok {
		core.Execute = ecosys.SkillExecute(r.reg)
		sCatalog.Register(core)
	}

	// Phase 8 (08-08): REAL execution for the core /opsx working set —
	// Bash, Read, Write, Edit, TodoWrite, TodoRead (capture-grounded result
	// forms, internal/coreexec). The SAME per-session registration site as
	// the Skill override above: the per-session clone carries the WorkDir +
	// a fresh per-session TodoStore (D-16 isolation); the shared engine
	// catalog is never mutated.
	//
	// 12-02 Task 4: the SAME site is the PreToolUse/PostToolUse chokepoint —
	// one HookRunner per session (discovered plugin hooks; the runner is
	// nil-safe when none are installed) wraps every core executor.
	hookRunner := ecosys.NewHookRunner(r.reg.Hooks, sessionID, dir, mgr.Path())
	taskRegistry := coreexec.NewTaskRegistry()
	coreexec.RegisterCore(sCatalog, coreexec.Config{
		WorkDir: dir, Todos: coreexec.NewTodoStore(), Hooks: hookRunner, Tasks: taskRegistry,
	})

	// 12-01 (ACP-01/D-01): the per-session AskUserQuestion surface. The
	// broker holds the pending ask; its surface callback publishes the
	// rendered question to the BUS as an AgentMessageChunk — Run's chunk
	// forwarder turns it into the client-visible session/update JUST BEFORE
	// the suspended turn's response. Timer-driven resumes run under the
	// serve-lifetime ctx (the suspending turn's ctx dies with its response).
	// The executor registration is the SAME Execute-only override discipline
	// as RegisterCore (the captured schema is never rewritten).
	askBroker := session.NewAskBroker(r.askTimeout, func(p session.PendingAsk) {
		r.bus.Publish(event.AgentMessageChunk{
			TurnID: p.TurnID, MessageID: p.TurnID,
			Content: coreexec.RenderAskSurface(p.Questions),
		})
	})
	coreexec.RegisterAsk(sCatalog, askBroker)

	// 12-04 (ACP-02): the per-session plan-mode state + the interactive-tool
	// family. The state carries the CAPTURED runtime-level mutating-tool gate
	// (the 12-05 re-record proved the target enforces it); the plan pair
	// registers through RegisterInteractive — the SAME Execute-only override
	// discipline as RegisterCore, wired at the same site.
	planMode := session.NewPlanModeState()
	mailbox := coreexec.NewAgentMailbox()
	sessionReader := coreexec.NewSessionReader(dir)
	coreexec.RegisterInteractive(sCatalog, coreexec.InteractiveConfig{
		Ask: askBroker, PlanMode: planMode,
		Mailbox: mailbox, Sessions: sessionReader, Tasks: taskRegistry,
		Schedule: r.schedule, // 12-07: the PER-PROJECT cron store (nil in test runners → structured no-store errors)
	})

	// 09-01 T2 (AUD-02): the late-bound capturer closure. sess is declared
	// BEFORE the Session literal and assigned after — the closure reads
	// CurrentTurnID() at FIRE time (mid-turn), so it sees the in-flight turn.
	// Header VALUES are dropped at the closure (_ per Pitfall 9 discipline:
	// values never enter audit artifacts; names ride 09-06 events only).
	var sess *session.Session

	capturer := func(body []byte, headers map[string]string) {
		// 09-06 (Pitfall 9 audit-path discipline): header NAMES only — sorted,
		// shape evidence for the TIER-2 identity invariant. VALUES are never
		// referenced and can never enter an artifact via this path.
		names := make([]string, 0, len(headers))

		for k := range headers {
			names = append(names, k)
		}

		slices.Sort(names)

		r.bus.Publish(event.RequestShaped{
			TurnID:          sess.CurrentTurnID(),
			VerbatimRequest: body,
			Profile:         prof.Name,
			HeaderNames:     names,
			Timestamp:       time.Now(),
		})
	}

	s := &session.Session{
		Manager:     mgr,
		Projector:   session.NewProjector(&prof, mgr),
		Provider:    r.makeProvider(capturer),
		Bus:         r.bus,
		Semaphore:   provider.NewSemaphore(maxConc),
		Profile:     prof,
		WorkDir:     dir,
		SessionID:   sessionID,
		Catalog:     sCatalog,
		ConfigAdded: r.configAdded,

		// 14-05 (EARLY-05): the light-tier subagent model — resolved through
		// the EXISTING scheduler tiers table (config-conditional; empty keeps
		// the parent model exactly as today).
		SubagentModel: resolveSubagentModel(r.schedCfg, r.providerName, time.Now(), r.stderrOrDefault()),

		// 12-02: discovered agent definitions register as spawnable subagent
		// types (a subagent_type match applies the definition's Prompt + Tools
		// on the existing PARA machinery — advisory listing, no new tier).
		SubagentTypes: r.reg.Agents,

		// 12-02 Task 4: the lifecycle hook seams (UserPromptSubmit at Prompt
		// entry, Stop at turn end, SubagentStop, SessionStart/SessionEnd).
		Hooks: hookRunner,
	}
	sess = s

	// 14-01: wire the checkpoint store (nil-guarded — a typed-nil *Store in
	// the interface would satisfy != nil and panic on SnapshotTurn; an
	// unwired field is the documented disabled state).
	if ckptStore != nil {
		s.Checkpointer = checkpointerAdapter{store: ckptStore}
	}

	// 12-01: wire the ask broker (suspension + reply routing + the D-01
	// timer; resumes run under the serve-lifetime ctx).
	//nolint:contextcheck // the serve-lifetime ctx is a stored field, not derived here
	s.SetAskBroker(r.serveCtxOrBackground(), askBroker)
	// 12-09 (G-12-3): wire the per-session plan-mode state onto the Session —
	// without this the Enter flip at the tool-result site is skipped
	// (Session.planMode nil), the mutating-tool gate never fires, and
	// ExitPlanMode answers "not in plan mode" (the live-session finding).
	s.SetPlanMode(planMode)
	// Phase-4 TOOL-04/05: inject the catalog-backed real executor (WebSearch/
	// WebFetch delegate to the configured backend; others call catalog
	// Tool.Execute). Phase 5 wraps it in toolcat.MCPExecutor so mcp__* calls
	// route to the MCP host and everything else reaches the inner executor.
	if r.engineEnabled {
		s.SetToolExecutor(toolcat.NewMCPExecutor(
			&toolexec.RealExecutor{Catalog: sCatalog, Log: slog.Default()}, mcpHost))
	} else {
		s.SetToolExecutor(toolcat.NewMCPExecutor(stubCatalogExec{}, mcpHost))
	}

	// 09-01 T2 (LOG-02/D-20): ONE TranscriptWriter per session, running for
	// the session's lifetime on a context derived from the SERVE ctx (never
	// the per-turn ctx — a writer bound to a turn dies after turn 1). Reaped
	// via OnClose, chained ahead of the existing MCP-host reaper.
	writerCtx, cancelWriter := context.WithCancel(r.serveCtxOrBackground())

	tw := session.NewTranscriptWriter(mgr, r.bus, r.bodyStore)

	//nolint:contextcheck // writerCtx inherits serveCtx (nil only in tests)
	go tw.Run(writerCtx)

	// Phase 5: reap MCP subprocesses on session end (logout/cancel/ctx-done) —
	// composed with the writer's cancel (Close runs the chain exactly once).
	s.OnClose = func() error {
		cancelWriter()
		taskRegistry.ReapAll() // 12-06: no background group outlives the session

		return mcpHost.Close()
	}

	r.sessions[sessionID] = s

	// 12-07 (sessMu held): this session is the project's current firing target.
	r.lastSessionID = sessionID

	// ...the session-lifetime chunk forwarder mirrors SERVER-DRIVEN turns
	// (timer resumes, automation firings) to the client (WINDOWS #3)...
	stopForwarder, _ := r.startSessionForwarder(sessionID)

	// ...and the FIRST active session of a serve lifetime runs the fire-once
	// catch-up pass (D-02). Async: the pass queues behind any turn through
	// the same per-session mutex.
	//nolint:contextcheck // serve-lifetime ctx (nil only in tests → Background)
	go r.runCatchUpOnce(r.serveCtxOrBackground(), sessionID)

	prevOnClose := s.OnClose
	s.OnClose = func() error {
		stopForwarder()

		return prevOnClose()
	}

	return s
}

// serveCtxOrBackground returns the serve-lifetime context, falling back to
// context.Background() for test runners that never set one (the writer then
// lives until OnClose — the observable contract Test 8 pins).
//
//nolint:funcorder // helper for sessionFor
func (r *sessionTurnRunner) serveCtxOrBackground() context.Context {
	if r.serveCtx != nil {
		return r.serveCtx
	}

	return context.Background()
}

// stderrOrDefault returns the serve-lifetime diagnostics writer, falling back
// to os.Stderr for test runners that never set one (the 14-05 degrade warning
// must stay LOUD even on the fallback path — transport discipline: stderr
// only, stdout is reserved for ACP frames).
//
//nolint:funcorder // helper for sessionFor
func (r *sessionTurnRunner) stderrOrDefault() io.Writer {
	if r.stderr != nil {
		return r.stderr
	}

	return os.Stderr
}

// resolveSubagentModel resolves the scheduler light tier for SUBAGENT
// dispatches (14-05, EARLY-05 — the token-economics lever). It is the same
// call shape setupModelRouting uses for tierHeavy, on the EXISTING tiers table
// (no new config surface):
//
//   - resolve error / absent binding → "" (silently: absence is the
//     documented parent-model default, not a failure);
//   - binding on the SESSION's provider → the light model slug (the override
//     rides the per-dispatch profile copy in session.subagentProfile);
//   - binding on a DIFFERENT provider → "" + exactly ONE loud degrade warning
//     naming both providers: a second provider instance threaded through the
//     subagent runner is ROUTED to the post-adoption queue (the override
//     covers the tier's primary purpose — a cheaper model on the same wire
//     shape); never a silent wrong-wire.
func resolveSubagentModel(cfg *modelrouting.Config, sessionProvider string, now time.Time, stderr io.Writer) string {
	if cfg == nil {
		return ""
	}

	primary, _, err := modelrouting.NewResolver(cfg).Resolve(tierLight, "", now, modelrouting.CapabilityReq{})
	if err != nil {
		return "" // no tiers.light binding — the documented default
	}

	if primary.Provider != sessionProvider {
		_, _ = fmt.Fprintf(stderr,
			"ass-guard: tiers.light is bound to provider %q but the session provider is %q — "+
				"subagent model override SKIPPED (parent model kept; cross-provider light-tier "+
				"routing is routed post-adoption)\n", primary.Provider, sessionProvider)

		return ""
	}

	return primary.Model
}

// spawnMCP loads the project .mcp.json from dir and starts the MCP host. It
// returns (host, mcpDecls); on any load/start error it returns an empty host
// (MCP is opt-in — one bad config never breaks the session). The ctx is
// derived from the ACP turn ctx (session/cancel reaches MCP spawn); a bounded
// timeout guards a hanging server.
func (r *sessionTurnRunner) spawnMCP( //nolint:funcorder // shutdown helper grouped with session lifecycle
	ctx context.Context, dir string,
) (*mcp.Host, []toolcat.Decl) {
	cfg, err := mcp.LoadConfig(dir)
	if err == nil {
		cfg = mergeMCPServers(cfg, r.mcpServers)
	}

	if err != nil || len(cfg.Servers) == 0 {
		return nopHost(), nil
	}

	// Use a bounded context so a hanging server cannot block session/new.
	spawnCtx, cancel := context.WithTimeout(ctx, mcpStartTimeout)
	defer cancel()

	host, decls, err := mcp.Start(spawnCtx, cfg)
	if err != nil {
		return nopHost(), nil
	}

	return host, decls
}

// mergeMCPServers merges the non-project server set (user ~/.claude.json OVER
// plugin-bundled .mcp.json — ecosys.Discover's output, 12-02) BELOW the
// project .mcp.json: project entries win on name collision, non-colliding
// lower-layer servers join the spawn set. The result feeds the EXISTING host
// unchanged — same spawn discipline, same mcp__<server>__<tool> naming,
// per-connection tools/list (ECOS-02/03).
func mergeMCPServers(project mcp.Config, lower []ecosys.ServerConfig) mcp.Config {
	taken := make(map[string]bool, len(project.Servers))
	for _, s := range project.Servers {
		taken[s.Name] = true
	}

	for _, sc := range lower {
		if taken[sc.Name] {
			continue // project wins on collision — the chain direction
		}

		project.Servers = append(project.Servers, mcp.ServerConfig{
			Name: sc.Name, Command: sc.Command, Args: sc.Args, Env: sc.Env, Cwd: sc.Cwd,
		})
	}

	return project
}

// closeAllSessions closes every live session's host (the ctx-done path — Plan
// 05-02 T4). Called when the server-level ctx is cancelled (SIGINT/SIGTERM) so
// no MCP subprocess outlives the ass-guard process.
func (r *sessionTurnRunner) closeAllSessions() { //nolint:funcorder // shutdown helper grouped with session lifecycle
	if r.sessions == nil {
		return
	}

	r.sessMu.Lock()
	sessions := make([]*session.Session, 0, len(r.sessions))

	for _, s := range r.sessions {
		sessions = append(sessions, s)
	}

	r.sessMu.Unlock()

	for _, s := range sessions {
		_ = s.Close()
	}

	// 13-00: drain every session's parked chains at serve end (goroutine
	// release — no chain outlives the serve lifetime).
	r.parkedMu.Lock()
	ids := make([]string, 0, len(r.parkedCancels))

	for id := range r.parkedCancels {
		ids = append(ids, id)
	}

	r.parkedMu.Unlock()

	for _, id := range ids {
		r.cancelParkedChains(id)
	}
}

// CloseSession closes one session's MCP host (the logout/cancel path — Plan
// 05-01 T4). It satisfies acp.SessionCloser; the ACP server calls it via type
// assertion when handling logout/session-cancel. An unknown sessionID is a no-op.
func (r *sessionTurnRunner) CloseSession(sessionID string) error {
	// 13-00: session/cancel + logout reach here (the ACP server's
	// closeSessionIfPossible) — drain the session's parked chains FIRST (no
	// decision, no injection after the cancel; goroutines released — D-03
	// stays the only off-switch).
	r.cancelParkedChains(sessionID)

	if r.sessions == nil {
		return nil
	}

	r.sessMu.Lock()
	s, ok := r.sessions[sessionID]
	r.sessMu.Unlock()

	if ok {
		return s.Close() //nolint:wrapcheck // session delegation
	}

	return nil
}

// advisoryNote is one collected advisory decision's client-note projection.
type advisoryNote struct {
	turnID string
	class  string
	text   string
}

// collectAdvisory drains EngineDecision events until promptDone, capturing
// the LAST advisory-signal decision (the note rides its turn id).
//
//nolint:lll // the drain-mirror signature
func (r *sessionTurnRunner) collectAdvisory(advCh <-chan event.Event, promptDone <-chan struct{}, advDone chan<- *advisoryNote) {
	defer r.bus.Unsubscribe("EngineDecision", advCh)

	var last *advisoryNote

	for {
		select {
		case e, ok := <-advCh:
			if !ok {
				advDone <- last

				return
			}

			if d, isDec := e.(event.EngineDecision); isDec && strings.HasPrefix(d.Signal, engine.SignalAdvisory) {
				last = &advisoryNote{
					turnID: d.TurnID,
					class:  strings.TrimPrefix(d.Signal, engine.SignalAdvisory),
					text:   advisoryNoteText,
				}
			}
		case <-promptDone:
			// Drain any buffered decisions, then hand the last advisory over.
			for {
				select {
				case e := <-advCh:
					d, isDec := e.(event.EngineDecision)
					if isDec && strings.HasPrefix(d.Signal, engine.SignalAdvisory) {
						last = &advisoryNote{
							turnID: d.TurnID,
							class:  strings.TrimPrefix(d.Signal, engine.SignalAdvisory),
							text:   advisoryNoteText,
						}
					}
				default:
					advDone <- last

					return
				}
			}
		}
	}
}

// advisoryNoteText is the FIXED client-visible advisory note (single-line —
// the Writer's decoded-newline transport guard; wording elements: the turn
// ended on a question, AskUserQuestion is the hands-off route, nothing is
// being held).
const advisoryNoteText = "The turn ended with a question for you. " +
	"AskUserQuestion is the hands-off route for questions like this; " +
	"the turn is complete and nothing further is queued."

// advisoryNoteDue applies the D-05 dedupe: the FIRST advisory of a class in
// a session is client-visible; repeats are audit-trail-only.
func (r *sessionTurnRunner) advisoryNoteDue(sessionID, class string) bool {
	r.advisoryMu.Lock()
	defer r.advisoryMu.Unlock()

	if r.advisorySeen == nil {
		r.advisorySeen = make(map[string]map[string]bool)
	}

	if r.advisorySeen[sessionID] == nil {
		r.advisorySeen[sessionID] = make(map[string]bool)
	}

	if r.advisorySeen[sessionID][class] {
		return false
	}

	r.advisorySeen[sessionID][class] = true

	return true
}

// routeAskReply routes a pending-ask reply (12-01, ACP-01): the reply text
// resolves the broker, lands as the pending call's tool result (the captured
// answered form), and the SUSPENDED turn's model loop resumes under THIS
// request's ctx (cancellation of the reply request cancels the resume). A
// non-text prompt or a lost race (the D-01 timer already resumed the turn)
// falls through to an ordinary turn (handled=false).
func (r *sessionTurnRunner) routeAskReply(
	ctx context.Context, sess *session.Session, blocks []session.ContentBlock,
) (string, bool) {
	if !sess.HasPendingAsk() {
		return "", false
	}

	idx := firstTextBlockIndex(blocks)
	if idx < 0 || blocks[idx].Text == "" {
		return "", false
	}

	stop, rerr := sess.ResolveAsk(ctx, blocks[idx].Text)
	if rerr != nil {
		return "", false // the D-01 timer won the race — an ordinary turn
	}

	return mapAskStop(stop), true
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
// (Plan 04-05 D-01). Run delegates to sess.Prompt (a real turn) after applying
// slash-command expansion (08-04: the engine's continue-injections re-enter
// here, so an injected "/opsx:propose …" gets identical expansion + provenance
// + boundary treatment as a user-typed command — and the USER prompt also
// arrives raw here, because sessionTurnRunner.Run defers engine-path expansion
// to this adapter); LastTurnOutput reads the transcript via Manager.ReadAll to
// extract the most-recent assistant_message text + the turn's tool-call names.
type engineTurnRunnerAdapter struct {
	sess *session.Session
	mgr  *session.Manager
	r    *sessionTurnRunner // the expansion owner (nil-safe: expansion no-ops)

	// startedBy is the registry command key whose invocation the LAST Run call
	// expanded (hybrid chaining, findings-6 disposition — TurnOutput.StartedBy).
	// Set from the RAW prompt inside Run, before expansion; a plain-text turn
	// leaves it "". Single-threaded by construction: the engine's Observe loop
	// calls Run and LastTurnOutput sequentially.
	startedBy string

	// subject is the FIRST turn's typed invocation arguments ("add-login" for
	// "/opsx:explore add-login") — the scenario subject. A later BARE injected
	// command ("/opsx:propose" with no args) inherits it, mirroring the
	// captured operator behavior (every stage invocation named the change; a
	// bare propose would ask for a subject and the chain would die — live
	// evidence: the first hybrid-chained E2E run). Sourced from the USER-side
	// typed prompt only; empty when the first prompt was not an invocation
	// with arguments.
	subject   string
	seenFirst bool

	// 13-00 park plumbing (all mutated ONLY on the engine's Observe
	// goroutine — single-threaded by construction, see above):
	//
	// lastStop records the stop the LAST sess.Prompt returned; onSuspended
	// fires once when the engine first consults AskSettle after an ask stop
	// (i.e. AFTER the first-turn ask decision has been emitted — the park
	// signal therefore implies the audit line is already on disk); parkMu,
	// armed by that signal, is the session's turn mutex — every post-park
	// Run (the settled chain's injections) holds it around sess.Prompt so
	// server-driven injections serialize with client turns exactly like the
	// cron/automation precedent.
	lastStop     string
	suspSignaled bool
	onSuspended  func()
	parkMu       *sync.Mutex
}

// Run drives one turn through the Session Core.
func (a *engineTurnRunnerAdapter) Run(ctx context.Context, prompt []session.ContentBlock) (string, error) {
	a.startedBy = ""
	if a.r != nil {
		key, args, ok := a.r.invocationFor(prompt)

		// Resolve the starting command from the RAW prompt (the same lookup
		// expandUserBlocks acts on) BEFORE expansion — an already-expanded body
		// carries no invocation. Prompt-side only: assistant/tool content can
		// never set this.
		a.startedBy = key

		// 12-07 vocabulary extension: an automation-fired turn carries the
		// automation's provenance INSTEAD of a command key (set exclusively by
		// the runner-side firing path — model content can never reach it,
		// T-12-07-03; no behavior change for user/command turns).
		if a.r != nil {
			a.r.sessMu.Lock()

			if prov := a.r.automationProvenance; prov != "" {
				key = prov
			}

			a.r.sessMu.Unlock()
		}

		if !a.seenFirst {
			a.seenFirst = true
			a.subject = args
		} else if ok && key != "" && args == "" && a.subject != "" {
			// Scenario-subject forwarding: a BARE injected command inherits the
			// first invocation's arguments (an injection carrying its own args
			// is left alone). Rebuild the invocation text with the subject and
			// re-resolve so the expansion below sees it.
			idx := firstTextBlockIndex(prompt)
			withSubject := append([]session.ContentBlock(nil), prompt...)
			withSubject[idx] = session.ContentBlock{
				Type: blockText, Text: "/" + key + " " + a.subject,
			}
			prompt = withSubject
		}

		prompt = a.r.expandUserBlocks(a.sess, prompt)
	}

	if a.parkMu != nil {
		// 13-00: a post-park injection — a server-driven turn on the parked
		// chain. Hold the session's turn mutex for the WHOLE turn (the cron
		// firing discipline): client turns and replies queue against it,
		// never overlap.
		a.parkMu.Lock()
		defer a.parkMu.Unlock()
	}

	stop, err := a.sess.Prompt(ctx, prompt)
	a.lastStop = stop

	return stop, err //nolint:wrapcheck // session delegation
}

// AskSettle implements engine.AskSettler (13-00): the engine's ask-wait
// consumes the session's per-suspension settle channel (closed after the
// resumed turn completes — whichever driver resumed it). The FIRST
// consultation after an ask stop also fires the park signal — at that point
// the first-turn ask decision has already been emitted, so the caller
// returning the prompt response at the suspension cannot race the audit
// line.
func (a *engineTurnRunnerAdapter) AskSettle() <-chan struct{} {
	if a.lastStop == stopAskACP && !a.suspSignaled && a.onSuspended != nil {
		a.suspSignaled = true
		a.onSuspended()
	}

	return a.sess.AskSettleChan()
}

// LastTurnOutput reads the transcript to build the engine's view of the most
// recent turn: the last assistant_message's TurnID + Text + the tool-call names
// recorded under that TurnID (D-02 second signal). A missing assistant_message
// yields an empty TurnOutput (the engine treats it as unmatched ⇒ nothing).
//
// 12-01 (ACP-01): the turn-TERMINAL line is the last of assistant_message |
// ask_suspended — a suspended turn has NO assistant_message (it ended at the
// tool loop), so scanning only assistant lines would attribute the suspension
// to the PREVIOUS turn. An ask_suspended terminal line yields
// TurnOutput with AskSuspended set (Decide → ActionAsk, never Continue).
//
//nolint:cyclop,funlen // the backward scan is one cohesive walk
func (a *engineTurnRunnerAdapter) LastTurnOutput() engine.TurnOutput {
	if a.mgr == nil {
		return engine.TurnOutput{}
	}

	lines, err := a.mgr.ReadAll()
	if err != nil {
		return engine.TurnOutput{}
	}

	var lastAssistant *session.Line

	var askSuspended *session.Line

	// 12-04 (ACP-02): the plan-mode state at turn end — the LAST plan_mode
	// marker's cause (enter/exit) is the state the turn ENDED in; it rides
	// TurnOutput as engine-decision provenance (signal context only).
	planModeOn := false

	// 12-09 (G-12-3): the terminal line is found FIRST scanning backward; its
	// turn's plan_mode markers sit EARLIER in the transcript, so the scan must
	// continue to the terminal line's own boundary before honoring the break.
	// The marker is always written BEFORE its turn's terminal line (the
	// tool-result site vs Step 6), so once the scan crosses INTO earlier turns
	// (a user_message of a different turn) the newest-marker read is final.
	var terminalTurn string

	// 12-09: per-turn "a tool_result landed BELOW this point" flags — a
	// suspension whose result already landed is RESOLVED and no longer
	// terminal (the reply or the D-01 timer completed it; the resumed turn's
	// own assistant_message is the real terminal).
	resolvedBelow := map[string]bool{}

	for i := len(lines) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
		switch lines[i].Type {
		case session.TypeAssistantMessage:
			if lastAssistant == nil {
				lastAssistant = &lines[i]

				terminalTurn = lines[i].TurnID
			}
		case session.TypeToolResult:
			resolvedBelow[lines[i].TurnID] = true
		case session.TypeAskSuspended:
			// Only an UNRESOLVED suspension is terminal: one whose tool_result
			// already landed below it was completed by the reply/timer resume.
			if !resolvedBelow[lines[i].TurnID] && askSuspended == nil {
				askSuspended = &lines[i]

				terminalTurn = lines[i].TurnID
			}
		case session.TypePlanMode:
			// Newest marker within/at-or-before the terminal turn wins; keep
			// reading until we cross out of that turn so the marker written
			// mid-turn is seen.
			planModeOn = lines[i].Cause == session.PlanModeCauseEnter
		}

		if terminalTurn != "" && lines[i].Type == session.TypeUserMessage && lines[i].TurnID == terminalTurn {
			break
		}
	}

	if askSuspended != nil {
		return engine.TurnOutput{
			TurnID: askSuspended.TurnID, StartedBy: a.startedBy,
			AskSuspended: true,
			ToolCalls:    toolCallNamesUnder(lines, askSuspended.TurnID),
			PlanMode:     planModeOn,
		}
	}

	if lastAssistant == nil {
		return engine.TurnOutput{}
	}

	return engine.TurnOutput{
		TurnID: lastAssistant.TurnID, Text: lastAssistant.Text,
		StartedBy: a.startedBy, ToolCalls: toolCallNamesUnder(lines, lastAssistant.TurnID),
		PlanMode: planModeOn,
	}
}

// toolCallNamesUnder collects the tool-call NAMES recorded under turnID (D-02
// second signal; shared by the assistant-terminal + ask-suspended paths).
func toolCallNamesUnder(lines []session.Line, turnID string) []string {
	var names []string

	for i := range lines {
		if lines[i].TurnID == turnID && lines[i].Type == session.TypeToolCall {
			names = append(names, lines[i].Name)
		}
	}

	return names
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

	// nextPromptFor answers "what does the engine inject when this pattern
	// matches" (08-06 chaining — the pattern table's next-command field).
	nextPromptFor func(patternID string) string
}

// PopulateContinue satisfies engine.ContinuePopulator (08-06): a continue
// decision with an empty NextPrompt gets the matched pattern's next /opsx:*
// command — the injection then flows through the 08-04 expansion seam
// (expand + provenance + boundary) like a typed command. Handles all three
// signal vocabularies: text:/tool: (the dual signals) and command: (hybrid
// chaining — the provenance rows share the same next field).
func (d *acpDispatcher) PopulateContinue(dec *engine.Decision) {
	if d.nextPromptFor == nil {
		return
	}

	id := dec.Signal
	for _, p := range []string{"text:", "tool:", "command:"} {
		if len(id) > len(p) && id[:len(p)] == p {
			id = id[len(p):]
		}
	}

	if next := d.nextPromptFor(id); next != "" {
		dec.NextPrompt = []session.ContentBlock{{Type: blockText, Text: next}}
	}
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

// patternNextPrompter is the optional pattern-table capability answering
// "what does the engine inject when this pattern matches" (08-06 chaining).
type patternNextPrompter interface {
	NextPromptFor(patternID string) string
}

// triggerFromSignal extracts the trigger stage from an engine signal string.
// The signal shape is "hook:<id>" (the OpenSpec pattern id), "text:<id>", or
// "command:<id>" (hybrid chaining — the provenance rows carry the same
// stage-bearing ids); v1 defaults to "post-implement" unless the id carries an
// explicit stage.
func triggerFromSignal(hookSignal string) string {
	// Strip a "hook:" / "text:" / "command:" prefix.
	id := hookSignal
	for _, p := range []string{"hook:", "text:", "command:"} {
		if len(id) > len(p) && id[:len(p)] == p {
			id = id[len(p):]
		}
	}

	if id == "changes-proposed" || id == "proposal-ready" {
		return "post-phase"
	}

	// 08-06 stage vocabulary: stage-bearing pattern ids ("post-<stage>-...")
	// select their own hook stage; un-staged ids keep the v1.0 default.
	for _, stage := range []string{stagePostExplore, stagePostPropose, "post-apply", "post-archive"} {
		if strings.HasPrefix(id, stage) {
			return stage
		}
	}

	return stagePostImplement
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

	return h.sess.Prompt(ctx, blocks) //nolint:wrapcheck // session delegation
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

	return h.mgr.AppendBoundary(cause, "", h.turnID) //nolint:wrapcheck // manager delegation
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
