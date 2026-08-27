package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
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

	"github.com/Djarvur/ass-guard-agent/internal/runtime/enginebridge"
)

const (
	mcpStartTimeout = 30 * time.Second

	// Phase-8 command-boundary vocabulary (08-04 D-11): the boundary cause
	// prefix for a mutating command (mutability values come from the openspec
	// config's exported vocabulary).
	mutatingCommandCause = "mutating-command:"

	// skillToolName is the captured core tool the skills closure overrides
	// (08-05 — the captured catalog entry keeps its schema; only Execute is
	// replaced).
	skillToolName = "Skill"
)

// nopHost returns a no-op MCP host (zero servers). CallTool on it returns an
// "unknown server" error; Close is a no-op. Used when .mcp.json is absent or
// load fails so the session always has a non-nil host to wrap/reap.
func nopHost() *mcp.Host { return &mcp.Host{} }

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

// Runner adapts the Session Core to the ACP TurnRunner interface
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
type Runner struct {
	bus          *event.Bus
	profile      profile.Profile
	workDir      string
	maxConc      int
	configAdded  []string
	makeProvider func(capturer provider.RequestCapturer) provider.Provider
	// askTimeout is the D-01 AskUserQuestion wait threaded to every
	// sessionFor-built broker (12-01). Run always sets it from the
	// --ask-timeout flag (default 10m; 0 = block forever); zero-value test
	// runners arm no timer (block-forever — safe for tests).
	askTimeout time.Duration
	// serveCtx is the server-lifetime context (set once in Run from
	// the server ctx). Per-session TranscriptWriters derive from it — a
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

	// 16-05 (ACP-08 live apply): the effective model an editor-driven
	// config change stamped ("" = the profile's own model). ApplyTurnModel
	// writes it under modelMu; sessionFor reads it to stamp sessions created
	// after the change.
	modelMu        sync.Mutex
	effectiveModel string

	// providerName is the session provider the factory builds (the heavy-tier
	// resolution's pick, 14-05): the same-provider check for the light binding
	// compares against it. Kept beside schedCfg so both come from the one
	// startup load.
	providerName string

	// stderr is the serve-lifecycle diagnostics writer (transport discipline:
	// stdout stays ACP-only). The 14-05 light-tier degrade warning lands here;
	// nil (test runners that never set one) falls back to os.Stderr.
	stderr io.Writer

	// Phase-4 engine wiring (Plan 04-05). Built once in SetupEngine(); nil when
	// the engine is disabled.
	engineEnabled bool
	patternTable  engine.PatternTable
	eng           *engine.Engine
	hookExec      *hookdag.Executor
	hookCfg       []hookdag.Hook
	learned       *learning.Store
	catalog       *toolcat.Catalog // shared catalog (OpenSpec tools registered once)

	// Phase-8 slash-command expansion (08-04): the discovered command registry
	// (loaded ONCE at startup — see LoadCommandRegistry) + the command
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

// RunnerConfig carries Runner's construction inputs (D-05): the same fields
// today's serve composition sets, names identical to the Runner struct's.
// The engine/cron/park state is NEVER set here — it is runtime-internal,
// armed by SetupEngine/startScheduler.
type RunnerConfig struct {
	Bus          *event.Bus
	BodyStore    *audit.BodyStore
	Profile      profile.Profile
	WorkDir      string
	MaxConc      int
	ConfigAdded  []string
	MakeProvider func(capturer provider.RequestCapturer) provider.Provider
	AskTimeout   time.Duration

	// ServeCtx is the server-lifetime ctx (per-session TranscriptWriters
	// derive from it; nil in test runners → context.Background()).
	//
	//nolint:containedctx // deliberate serve-lifetime ctx storage (09-01 T2)
	ServeCtx context.Context

	SchedCfg     *modelrouting.Config
	ProviderName string
	Stderr       io.Writer
}

// NewRunner fills a Runner from cfg — struct-fill ONLY (D-05 thin ctor).
// Callers invoke LoadCommandRegistry/SetupEngine/startScheduler explicitly,
// in today's order.
func NewRunner(cfg *RunnerConfig) *Runner {
	return &Runner{
		bus:          cfg.Bus,
		bodyStore:    cfg.BodyStore,
		profile:      cfg.Profile,
		workDir:      cfg.WorkDir,
		maxConc:      cfg.MaxConc,
		configAdded:  cfg.ConfigAdded,
		makeProvider: cfg.MakeProvider,
		askTimeout:   cfg.AskTimeout,
		serveCtx:     cfg.ServeCtx,
		schedCfg:     cfg.SchedCfg,
		providerName: cfg.ProviderName,
		stderr:       cfg.Stderr,
	}
}

// checkpointerAdapter adapts *checkpoint.Store to session.Checkpointer: the
// store's method is Snapshot, the seam speaks SnapshotTurn — the one-method
// adapter lives at the wiring site so internal/session keeps no dependency
// on internal/checkpoint (the OnClose func-seam pattern).
type checkpointerAdapter struct {
	store *checkpoint.Store
}

func (c checkpointerAdapter) SnapshotTurn(
	ctx context.Context, sessionID, turnID string,
) error {
	return c.store.Snapshot(ctx, sessionID, turnID) //nolint:wrapcheck // thin delegation
}

// SetupEngine builds the Phase-4 engine wiring (Plan 04-05 D-01/D-13/D-15/D-21):
// the shared catalog + OpenSpec tool registration + the openspec pattern table +
// the loaded hook-DAG config + the learning store + the engine + its
// ActionDispatcher. On any error the engine stays disabled (Run falls back to
// the unwrapped sess.Prompt — backward-compatible + D-04 graceful degradation).
func (r *Runner) SetupEngine() error {
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
	r.eng.Dispatcher = enginebridge.NewACPDispatcher(&enginebridge.BridgeConfig{
		Hooks:   r.hookExec,
		HookCfg: r.hookCfg,
		Learned: r.learned,
		Bus:     r.bus,
		NextPromptFor: func(patternID string) string {
			if np, ok := r.patternTable.(patternNextPrompter); ok {
				return np.NextPromptFor(patternID)
			}

			return ""
		},
	})
	r.engineEnabled = true

	return nil
}

// workDirOrDefault returns the configured work dir or cwd.
func (r *Runner) workDirOrDefault() string { //nolint:funcorder // ordering groups related logic
	if r.workDir != "" {
		return r.workDir
	}

	wd, _ := os.Getwd()

	return wd
}

// LoadCommandRegistry loads the ecosys command registry (08-04) + the command
// mutability table (D-11) ONCE at startup. Any failure is logged to stderr and
// leaves the fields zero — expansion no-ops and every turn proceeds on plain
// text (T-8-16: an expansion problem NEVER becomes a turn failure or an ACP
// error). Tests call it explicitly after planting fixtures; production calls
// it from the serve composition.
func (r *Runner) LoadCommandRegistry() {
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
func (r *Runner) expandUserBlocks( //nolint:funcorder // one pipeline; grouped with the turn seam
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
func (r *Runner) invocationFor( //nolint:funcorder,nonamedreturns // sibling of expandUserBlocks
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
func (r *Runner) Run(
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
	// 16-01: ToolCall / ToolCallUpdate subscriptions join it — the ACP-03 live
	// tool cards ride the same forwarder → ActivityEmitter path (the emitter's
	// single drain owns the notification order).
	ch := r.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	toolCh := r.bus.Subscribe("ToolCall", event.BufToolCall)
	toolUpdCh := r.bus.Subscribe("ToolCallUpdate", event.BufToolCallUpdate)

	defer r.bus.Unsubscribe("AgentMessageChunk", ch)
	defer r.bus.Unsubscribe("ToolCall", toolCh)
	defer r.bus.Unsubscribe("ToolCallUpdate", toolUpdCh)

	done := make(chan struct{})
	promptDone := make(chan struct{})

	startChunkForwarder(ctx, ch, toolCh, toolUpdCh, emit, promptDone, done)

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
	// adapter expands inside its Run (see EngineTurnAdapter.Run — it must
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
// owns the vocabulary; the turn layer only needs the one mapping).
const stopAskACP = "ask"

// startChunkForwarder spawns the per-Run chunk forwarder: streamed
// AgentMessageChunk events become session/update notifications, and (16-01)
// ToolCall / ToolCallUpdate events become the ACP-03 tool-card frames via the
// ActivityEmitter seam, until the turn completes; then any buffered events
// drain before the goroutine exits (the caller signals promptDone + waits on
// done). A plain ChunkEmitter (legacy fakes) silently skips the tool frames.
func startChunkForwarder(
	ctx context.Context,
	ch, toolCh, toolUpdCh <-chan event.Event,
	emit acp.ChunkEmitter,
	promptDone, done chan struct{},
) {
	// 16-01: the ActivityEmitter assertion happens once; a nil toolEmit simply
	// disables tool-card forwarding for plain ChunkEmitter fakes.
	toolEmit, _ := emit.(acp.ActivityEmitter)

	route := func(e event.Event) { routeBusEvent(e, emit, toolEmit) }

	go func() {
		defer func() { done <- struct{}{} }()

		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return
				}

				route(e)
			case e, ok := <-toolCh:
				if !ok {
					return
				}

				route(e)
			case e, ok := <-toolUpdCh:
				if !ok {
					return
				}

				route(e)
			case <-ctx.Done():
				return
			case <-promptDone:
				// Prompt returned; drain any buffered events, then exit.
				for {
					select {
					case e := <-ch:
						route(e)
					case e := <-toolCh:
						route(e)
					case e := <-toolUpdCh:
						route(e)
					default:
						return
					}
				}
			}
		}
	}()
}

// routeBusEvent forwards one bus event of the forwarder-supported kinds to the
// right emitter method (AgentMessageChunk → text chunk; ToolCall /
// ToolCallUpdate → the ACP-03 tool-card frames). Shared by the per-Run and the
// session-lifetime forwarders.
func routeBusEvent(e event.Event, emit acp.ChunkEmitter, toolEmit acp.ActivityEmitter) {
	switch c := e.(type) {
	case event.AgentMessageChunk:
		_ = emit.AgentMessageChunk(c.MessageID, c.Content)
	case event.ToolCall:
		forwardToolCall(toolEmit, e)
	case event.ToolCallUpdate:
		forwardToolCallUpdate(toolEmit, e)
	}
}

// forwardToolCall mirrors one bus ToolCall event as a v1 tool_call frame: the
// card's title IS the tool name and the frame carries the event's raw input so
// the acp layer can derive presentation variants (its own concern, never the
// runtime's). A nil emitter (plain ChunkEmitter fake) is a no-op.
func forwardToolCall(toolEmit acp.ActivityEmitter, e event.Event) {
	if toolEmit == nil {
		return
	}

	c, ok := e.(event.ToolCall)
	if !ok {
		return
	}

	_ = toolEmit.ToolCall(&acp.ToolCallFrame{
		ToolCallID: c.ToolCallID,
		Title:      c.Name,
		Kind:       toolKindFor(c.Name),
		Input:      append(json.RawMessage(nil), c.Input...),
	})
}

// forwardToolCallUpdate mirrors one bus ToolCallUpdate event: the event's
// partial-update JSON is decoded (tolerantly — unknown keys dropped, the
// decoder never fabricates) and pinned to the event's real toolCallId.
func forwardToolCallUpdate(toolEmit acp.ActivityEmitter, e event.Event) {
	if toolEmit == nil {
		return
	}

	c, ok := e.(event.ToolCallUpdate)
	if !ok || len(c.Update) == 0 {
		return
	}

	var uf acp.ToolCallUpdateFrame

	_ = json.Unmarshal(c.Update, &uf)
	uf.ToolCallID = c.ToolCallID

	_ = toolEmit.ToolCallUpdate(&uf)
}

// Captured core tool names referenced by toolKindFor (named so the mapping
// table reads as a table, not a string pile).
const (
	toolNameBash         = "Bash"
	toolNameEdit         = "Edit"
	toolNameWrite        = "Write"
	toolNameRead         = "Read"
	toolNameGrep         = "Grep"
	toolNameGlob         = "Glob"
	toolNameWebFetch     = "WebFetch"
	toolNameWebSearch    = "WebSearch"
	toolNameExitPlanMode = "ExitPlanMode"
)

// toolKindFor maps the captured core tool names to v1 ToolKind values.
// Unknown tools map to "" (kind omitted — the schema treats it as optional;
// never guessed). Presentation vocabulary, not behavior.
func toolKindFor(name string) string {
	switch name {
	case toolNameBash:
		return acp.ToolKindExecute
	case toolNameEdit, toolNameWrite:
		return acp.ToolKindEdit
	case toolNameRead:
		return acp.ToolKindRead
	case toolNameGrep, toolNameGlob:
		return acp.ToolKindSearch
	case toolNameWebFetch, toolNameWebSearch:
		return acp.ToolKindFetch
	case toolNameExitPlanMode:
		return acp.ToolKindSwitchMode
	default:
		return ""
	}
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
func (r *Runner) runOneTurn(
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
		r.hookExec.Commands = enginebridge.NewRealCommandRunner()
		r.hookExec.Turns = enginebridge.NewHookSessionTurnRunner(sess)
		r.hookExec.Boundaries = enginebridge.NewHookSessionBoundaryOpener(sess.Manager)
	}
	// Wire the engine's transcript writer to the active session's Manager so
	// engine_decision lines land in THIS session's transcript (the Manager is
	// per-session; SetupEngine could not bind it).
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

	adapter := enginebridge.NewEngineTurnAdapter(sess, &enginebridge.BridgeConfig{
		Expand: r.expandUserBlocks,
		Invoke: r.invocationFor,
		AutomationProvenance: func() string {
			r.sessMu.Lock()
			defer r.sessMu.Unlock()

			return r.automationProvenance
		},
	})
	suspension := make(chan struct{}, 1)
	adapter.OnSuspended = func() {
		// Runs on the engine goroutine: arming parkMu here orders it before
		// any post-settle injection Run (same goroutine), and the buffered
		// signal never blocks the chain.
		adapter.ParkMu = r.sessionTurnMu(sessionID)

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
func (r *Runner) registerParkedChain(sessionID string, pc *parkedChain) {
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
func (r *Runner) unregisterParkedChain(sessionID string, pc *parkedChain) {
	r.parkedMu.Lock()
	defer r.parkedMu.Unlock()

	if r.parkedCancels[sessionID] != nil {
		delete(r.parkedCancels[sessionID], pc)
	}
}

// cancelParkedChains cancels every parked chain of the session (the
// session/cancel + logout + serve-end drain).
func (r *Runner) cancelParkedChains(sessionID string) { //nolint:funcorder // park helper group
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
func (r *Runner) chainEnter(sessionID string) { //nolint:funcorder // idle-tracking group
	r.chainMu.Lock()
	defer r.chainMu.Unlock()

	if r.activeChains == nil {
		r.activeChains = make(map[string]int)
	}

	r.activeChains[sessionID]++
}

func (r *Runner) chainExit(sessionID string) { //nolint:funcorder // idle-tracking group
	r.chainMu.Lock()
	defer r.chainMu.Unlock()

	if r.activeChains[sessionID] > 0 {
		r.activeChains[sessionID]--
	}
}

func (r *Runner) chainCount(sessionID string) int { //nolint:funcorder // idle-tracking group
	r.chainMu.Lock()
	defer r.chainMu.Unlock()

	return r.activeChains[sessionID]
}

// WaitChainIdle blocks until no engine chain is active for the session (the
// parked-ask resume + its injections all finished) or ctx dies. The harness
// seam (opsxRunnerSeam.RunPrompt / runStageTyped) consumes it — one engine
// loop, two callers, identical wait-through-suspension semantics.
func (r *Runner) WaitChainIdle(ctx context.Context, sessionID string) bool {
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
func (r *Runner) sessionFor( //nolint:funcorder,funlen,maintidx,cyclop // grouping keeps the turn pipeline together
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

	// 16-05 (ACP-08 live apply): sessions created after an editor-driven model
	// change stamp the effective model at construction — the future-sessions
	// leg of the live-apply seam (the live-session leg is ApplyTurnModel).
	if m := r.effectiveModelFor(); m != "" {
		prof.Model = m
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
		s.SetToolExecutor(toolcat.NewMCPExecutor(enginebridge.NewStubCatalogExec(), mcpHost))
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
func (r *Runner) serveCtxOrBackground() context.Context {
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
func (r *Runner) stderrOrDefault() io.Writer {
	if r.stderr != nil {
		return r.stderr
	}

	return os.Stderr
}

// resolveSubagentModel resolves the scheduler light tier for SUBAGENT
// dispatches (14-05, EARLY-05 — the token-economics lever). It is the same
// call shape the routing setup uses for tierHeavy, on the EXISTING tiers table
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
func (r *Runner) spawnMCP( //nolint:funcorder // shutdown helper grouped with session lifecycle
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
// plugin-bundled .mcp.json — ecosys.Discover's output, -02) BELOW the
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
func (r *Runner) closeAllSessions() { //nolint:funcorder // shutdown helper grouped with session lifecycle
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
func (r *Runner) CloseSession(sessionID string) error {
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

// --- serve-composition seam (D-19 export-by-necessity) ---
//
// The serve composition (acpserve.Run) wires the scheduler store + the
// server's emitter + the scheduler goroutine + the ctx-done session reap —
// the four operations the 15-05 FinishHooks callback seam crossed with while
// the runner lived in package main (with LoadCommandRegistry/SetupEngine,
// which Run also calls between NewRunner and NewServer). They are exported
// because acpserve cannot compile otherwise (unexported members are
// unreachable from a foreign package); NOTHING else is exported for it.

// SetSchedule assigns the per-project schedule store (the sched.Open success
// arm of the serve composition).
func (r *Runner) SetSchedule(store *sched.ScheduleStore) { r.schedule = store }

// ApplyTurnModel applies an editor-driven model change to LIVE state (16-05/
// ACP-08, D-05's day-1 handlers): the runner's effective-model state updates
// under its mutex, then every live session's profile copy is re-stamped UNDER
// that session's turn mutex — a Set arriving mid-turn WAITS for the in-flight
// turn to finish, so the in-flight request keeps its model and the next
// request carries the new one (no torn stamp). Sessions created later stamp
// the effective model at construction (sessionFor).
func (r *Runner) ApplyTurnModel(model string) error {
	r.modelMu.Lock()
	r.effectiveModel = model
	r.modelMu.Unlock()

	r.sessMu.Lock()

	ids := make([]string, 0, len(r.sessions))

	sessions := make(map[string]*session.Session, len(r.sessions))

	for id, s := range r.sessions {
		ids = append(ids, id)
		sessions[id] = s
	}

	r.sessMu.Unlock()

	for _, id := range ids {
		mu := r.sessionTurnMu(id)
		mu.Lock()

		sessions[id].SetTurnModel(model)

		mu.Unlock()
	}

	return nil
}

// SetEmitter injects the server-driven-turn chunk emitter (WINDOWS #3:
// strictly between server construction and scheduler start).
func (r *Runner) SetEmitter(emit func(sessionID string) acp.ChunkEmitter) { r.emitFor = emit }

// StartScheduler launches the due-check goroutine (the serve composition's
// post-emitter step — see cron_wiring.go).
func (r *Runner) StartScheduler(ctx context.Context) { r.startScheduler(ctx) }

// CloseAllSessions closes every live session at serve end (the ctx-done
// subprocess reap).
func (r *Runner) CloseAllSessions() { r.closeAllSessions() }

// effectiveModelFor returns the editor-stamped effective model ("" = none —
// the profile's own model governs).
func (r *Runner) effectiveModelFor() string {
	r.modelMu.Lock()
	defer r.modelMu.Unlock()

	return r.effectiveModel
}

// advisoryNote is one collected advisory decision's client-note projection.
type advisoryNote struct {
	turnID string
	class  string
	text   string
}

// collectAdvisory drains EngineDecision events until promptDone, capturing
// the LAST advisory-signal decision (the note rides its turn id).
func (r *Runner) collectAdvisory(advCh <-chan event.Event, promptDone <-chan struct{}, advDone chan<- *advisoryNote) {
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
func (r *Runner) advisoryNoteDue(sessionID, class string) bool {
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
func (r *Runner) routeAskReply(
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

// patternNextPrompter is the optional pattern-table capability answering
// "what does the engine inject when this pattern matches" (08-06 chaining).
type patternNextPrompter interface {
	NextPromptFor(patternID string) string
}
