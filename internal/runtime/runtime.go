package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	"github.com/Djarvur/ass-guard-agent/internal/perm"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/redact"
	"github.com/Djarvur/ass-guard-agent/internal/sandbox"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
	"github.com/Djarvur/ass-guard-agent/internal/tasks"
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
	// through the SAME resolver that picked the session provider. nil in
	// test runners → no subagent model override (the documented default).
	schedCfg *modelrouting.Config

	// compactionMu guards the runner-level effective compaction settings
	// (19-05/D-03): lazily seeded from schedCfg on first use (NewRunner stays
	// struct-fill-only), re-stamped by ApplyCompactionSettings (the config
	// surface's live-apply relay). armed=false when no scheduling config
	// loaded (test runners) — sessions then keep 19-04's disabled zero value.
	compactionMu      sync.Mutex
	compactionEnabled bool
	compactionPct     int
	compactionArmed   bool

	// 16-05 (ACP-08 live apply): the effective model an editor-driven
	// config change stamped. "" = no editor stamp — the tier-resolved config
	// default governs the wire (16-09 gap 4b chip parity: defaultTurnModel),
	// NOT the profile's own model. ApplyTurnModel writes it under modelMu;
	// sessionFor reads it to stamp sessions created after the change.
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
	// (loaded at startup, REPLACED wholesale by the 20-05 rescan — see
	// installRegistry) + the command mutability table (D-11 boundaries, T3).
	// regMu guards the wholesale swap (Pitfall 1: never in-place mutation);
	// readers take registrySnapshot(). A failed load leaves both zero —
	// expansion no-ops and turns proceed on plain text (graceful degradation).
	regMu         sync.RWMutex
	reg           ecosys.Registry
	cmdMutability map[string]string

	// 20-01 (CMDS-01/ACP-04): the immutable resolver chain behind ONE atomic
	// pointer (RESEARCH Pattern 1 — lock-free on the read-hot path, swapped
	// wholesale by the 20-05 rescan; the atomic-pointer-over-RWMutex choice
	// is documented here per the research alternatives note). chainResolves
	// counts chain consultations (test observability for the single-parse
	// discipline — one resolution per Run). commandsNotify is the re-fire
	// seam the composition binds to available_commands_update fan-out.
	chainPtr         atomic.Pointer[commandChain]
	chainResolves    atomic.Uint64
	commandsNotifyMu sync.RWMutex
	commandsNotify   func()

	// 20-02 delegation seams (CMDS-02): the Phase 19 immediate-compaction
	// entry (19-D-11) and the Phase 18 resume listing (18-D-10) route through
	// registered callbacks — the SetCommandsNotify pattern. NewRunner arms the
	// REAL machinery (both exist in this build); tests nil or replace them to
	// prove the loud-degrade posture.
	compactNowHook func(ctx context.Context, sess *session.Session, args string) (string, error)
	resumeListHook func() (string, error)

	// 20-05 (CMDS-04): the discovery rescan coordinator (one per serve,
	// armed by StartDiscoveryWatcher on the serve ctx) + the invoke-time
	// freshness probe's chain signature (the stat-level fingerprint of the
	// discovery root set the live chain was built under).
	rescanMu       sync.Mutex
	rescanCoord    *rescanCoordinator
	chainSigMu     sync.Mutex
	chainSignature string

	// 20-02 (/cost, D-07): the live usage-endpoint leg's test seams — nil
	// costTransport uses the default HTTP client; a zero costFetchBudget uses
	// the ~10s FAST-CONTROL default.
	costTransport   http.RoundTripper
	costFetchBudget time.Duration

	// 20-03 (D-15): the cross-provider dispatch cache — one second provider
	// per (provider, session), built through the factory's single seam;
	// subagentDegrades counts the D-15 degrade occurrences (the counter IS
	// the warning's audit trail).
	subagentProvMu    sync.Mutex
	subagentProviders map[string]provider.Provider
	subagentDegrades  atomic.Uint64

	// 21-04 (PAR-06/D-10): the Read-rule consult seam for @-mention
	// expansion. Every @file consults it with tool "Read" BEFORE its content
	// enters the prompt; an absolute @path additionally needs an explicit
	// allow just to be ADMITTED (nil admits nothing outside the workspace —
	// ingress resolution is never a whole-FS existence oracle, T-21-14).
	// TESTS inject custom evaluators here; production consults the
	// Runner-scoped perm store below (readRuleEval — ONE rule authority with
	// the gate, 21-REVIEW WR-02/WR-03) and NEVER writes this field, so the
	// lock-free turn-time reads are race-free by construction.
	readRuleEvaluator func(tool, path string) bool

	// permStore is the Runner-scoped perm store (21-REVIEW WR-02/WR-03): ONE
	// live rule authority per project/serve. All sessions share the workDir
	// (hence one permissions.yaml) — per-session stores were one FILE but N
	// divergent in-memory instances: an allow_always/reject_always click in
	// session B updated B's memory while other seams kept consulting a stale
	// instance. Opened lazily by the first sessionFor that succeeds (under
	// sessMu) and published ATOMICALLY, so the lock-free turn-time mention
	// reads (readRuleEval) race cleanly. A failed open leaves it nil (the
	// session degrades rule-less with the loud log) and the NEXT sessionFor
	// retries — residual errors are environmental and can be transient.
	permStore atomic.Pointer[perm.Store]

	// 21-05 (PAR-06/D-09): the image-ingress limit set. Zero →
	// DefaultImageLimits (Anthropic's documented classes, pinned by the
	// CONTEXT discretion). Tests may pin a cheaper set; production keeps the
	// default.
	imageLimits ImageLimits

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

	// 22-01 (D-01..D-03) wake-turn state — see cron_wiring.go: trackers maps
	// each session to its tasks.Tracker (the ONE task-notification
	// subsystem); wakeInFlight deduplicates concurrent drain attempts per
	// session (one active drain chain); wakeRetryInterval is the busy-turn
	// retry cadence (D-01 fallback — the pending batch coalesces while a
	// client turn holds the slot and the chain retries after it frees).
	trackers          sync.Map // sessionID -> *tasks.Tracker
	wakeInFlight      sync.Map // sessionID -> *atomic.Bool
	wakeRetryInterval time.Duration
	// 22-04 (PAR-09/D-07): each session's persistent-shell PTY manager (ONE
	// lazily-started shell per session; the close battery's observation
	// handle — OnClose captures the same manager for the Drain link).
	ptyManagers sync.Map // sessionID -> *coreexec.PTYManager
	// backgroundCaps resolves the D-12 caps at sessionFor time (nil = the
	// 8/16 defaults; the serve composition binds the config surface).
	backgroundCaps func() (subagents, bash int)
	// sandboxHandle (22-06, SAND-01): the startup probe's resolved Handle —
	// the D-04 policy triple + the host availability — stored ONCE per
	// process before the scheduler starts. nil (test runners) or
	// Availability.Mode "off" (the DEFAULT — the operator never asked) means
	// every exec path stays untouched; Mode "on" + Available routes the three
	// Bash-class exec sites through sandbox.WrapCmd (Tasks 2-3 read it at
	// sessionFor into coreexec.Config and PTYOpts).
	sandboxHandle *sandbox.Handle

	// 17-02 (ACP-01): the permission-gate composition. permAskFire is the
	// permission-ask surface callback injected from the serve composition
	// (acpserve) after the acp Server exists — the onSurface-callback
	// precedent; internal/session stays free of internal/acp imports. nil =
	// unwired surface (the gate fails a gated ask safe with a decline —
	// never a silent allow). The ctx parameter is the ask queue's per-firing
	// cancellation seam (17-03 D-13: the drain cancels an OPEN dialog through
	// it). permMode is the LIVE permission-mode accessor
	// (ungated|gated; Task 17-02-3's permissions.mode apply target): the gate
	// reads it PER CALL, so a set_config_option flip lands on the running
	// session without recreation (Pitfall 8). Boot default: ungated
	// (criterion 4 — "available, not default").
	permAskFire func(ctx context.Context, e *session.AskEntry) session.AskOutcome
	permMode    atomic.Value // string

	// 17-04 (ACP-02/D-09): the elicitation-ask surface callback — the SAME
	// injection shape as permAskFire, bound by the serve composition after
	// the acp Server exists. The question-family enqueue (the AskBroker
	// onSurface body) and the engine-ask conversion both fire through it;
	// nil = unwired surface (today's plain-text publish stays primary).
	askFire func(ctx context.Context, e *session.AskEntry) session.AskOutcome

	// 13-00 park state: parkedCancels holds each session's parked-chain ctx
	// cancels (drained by CloseSession/closeAllSessions — the D-03 off-switch
	// extended to parked chains); activeChains counts running engine chains
	// per session (WaitChainIdle's input — the eval/E2E seam's
	// wait-through-suspension).
	parkedMu      sync.Mutex
	parkedCancels map[string]map[*parkedChain]struct{}
	chainMu       sync.Mutex
	activeChains  map[string]int

	// 23-04 (SEEDG-02, Pitfall 9): the WORKSPACE-level checkpoint store,
	// built once per Runner — /undo, the restore guard, and the session-start
	// GC sweep consume it through checkpointStore(); sessions attach their
	// snapshot adapter from the same instance. ckptOpenTried makes the
	// loud open-failure log fire exactly once (nil store = sessions run
	// WITHOUT undo snapshots; /undo reports unavailable, never silently).
	ckptMu        sync.Mutex
	ckptStore     *checkpoint.Store
	ckptOpenTried bool

	// checkpointGC is the D-08 bounds seam (23-04 Task 2 fills it from the
	// persisted checkpoint: layer keys; the serve composition binds it —
	// the askFire late-injection precedent). nil = the embedded defaults
	// (7 days / DefaultKeep per session).
	checkpointGC func() (days int, perSession int)

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
	// 20-02: the delegation seams arm the REAL machinery (Phase 19 CompactNow
	// + the Phase 18 listing engine both exist in this build — a seam is
	// never left unregistered while its machinery is present).
	wd := cfg.WorkDir

	return &Runner{
		bus:            cfg.Bus,
		bodyStore:      cfg.BodyStore,
		profile:        cfg.Profile,
		workDir:        cfg.WorkDir,
		maxConc:        cfg.MaxConc,
		configAdded:    cfg.ConfigAdded,
		makeProvider:   cfg.MakeProvider,
		askTimeout:     cfg.AskTimeout,
		serveCtx:       cfg.ServeCtx,
		schedCfg:       cfg.SchedCfg,
		providerName:   cfg.ProviderName,
		stderr:         cfg.Stderr,
		compactNowHook: realCompactNow,
		resumeListHook: realResumeListing(wd),
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
	// 16-REVIEW WR-01: the shared executor + engine are Bus/Log TEMPLATES only.
	// The session-bound wiring (hook seams, decision Manager, dispatcher) is
	// built PER-TURN in runOneTurn — never rebound on these shared instances
	// (a rebind cross-wires concurrent sessions and parked 13-00 chains).
	r.hookExec = &hookdag.Executor{Bus: r.bus, Log: slog.Default()}

	// Learning store (LRN-01..04) — versioned .ass-guard/learned.yaml.
	learnedPath := filepath.Join(r.workDirOrDefault(), ".ass-guard", "learned.yaml")

	learned, lerr := learning.Open(learnedPath)
	if lerr == nil {
		r.learned = learned
	} else {
		log.Printf("ass-guard: learning store open failed (continuing without learning): %v", lerr)
	}

	// The engine template (Bus/Log only — see the WR-01 note on r.hookExec;
	// the ActionDispatcher is per-turn too).
	r.eng = &engine.Engine{Bus: r.bus, Log: slog.Default()}
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

		r.installRegistry(ecosys.Registry{}, nil)

		return
	}

	r.installRegistry(reg, servers)

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
// 21-04 (PAR-06/D-10): @-mention expansion rides the SAME seam as a second
// pass over the (possibly command-expanded) text — file content sections,
// one-level dir listings, loud fixed-form notes for the unresolvable, one
// mention_provenance line per mention. Every other case returns the blocks
// UNCHANGED (unknown /foo is ordinary text — no match, nothing happens;
// zero mentions — byte-identical). All transcript-write errors degrade to
// stderr logs — never ACP errors.
func (r *Runner) expandUserBlocks( //nolint:funcorder // one pipeline; grouped with the turn seam
	sess *session.Session, blocks []session.ContentBlock,
) []session.ContentBlock {
	idx := firstTextBlockIndex(blocks)
	if idx < 0 {
		return blocks
	}

	out := blocks // borrowed until a change forces the copy (copy-on-write)

	key, args, ok := ecosys.ParseInvocation(blocks[idx].Text)
	if ok {
		if cmd, found := r.resolveSlashCommand(key); found {
			out = append([]session.ContentBlock(nil), blocks...)
			out[idx] = session.ContentBlock{Type: blockText, Text: cmd.Expand(args)}

			if sess != nil && sess.Manager != nil {
				// Mutating commands ALWAYS open the boundary first (D-11): the
				// projector's ReadLastBoundary reset fires for the expanded
				// stage exactly as for a tool-call boundary.
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
			}
		}
	}

	// 21-04: the mention pass runs AFTER the command-invocation logic and is
	// a no-op without @ tokens (expandMentions returns the blocks untouched).
	return r.expandMentions(sess, out, idx)
}

// Mention-expansion forms (the mention_provenance line's Text field — D-10's
// outcome vocabulary; every mention produces exactly one form).
const (
	mentionFormFile       = "file"
	mentionFormDir        = "dir"
	mentionFormDenied     = "denied"
	mentionFormUnresolved = "unresolved"
)

// Mention-note outcome classes (T-21-14's fixed-form family): a note names
// the token and its class — never per-path filesystem detail, never content.
const (
	mentionClassMissing    = "not found"
	mentionClassOutside    = "outside the admitted roots"
	mentionClassDenied     = "denied by the Read rules"
	mentionClassUnreadable = "could not be read"
)

// expandMentions applies @-mention expansion to the first text block (21-04,
// PAR-06/D-10): each parsed mention resolves against the workspace, gains a
// labeled section (file content / one-level dir listing) or a fixed-form
// loud note, and appends one mention_provenance line (the
// AppendCommandProvenance discipline — the :426-434 loud-continue failure
// handling). Zero mentions returns blocks UNTOUCHED (the existing
// early-return shape — byte-identical passthrough).
func (r *Runner) expandMentions( //nolint:funcorder // one pipeline; grouped with the turn seam
	sess *session.Session, blocks []session.ContentBlock, idx int,
) []session.ContentBlock {
	mentions := ecosys.ParseMentions(blocks[idx].Text)
	if len(mentions) == 0 {
		return blocks
	}

	var text strings.Builder
	text.WriteString(blocks[idx].Text)

	for _, m := range mentions {
		section, resolved, form := r.resolveMention(m)
		text.WriteString(section)

		if sess == nil || sess.Manager == nil {
			continue
		}

		err := sess.Manager.AppendMentionProvenance("", m.Token, resolved, form)
		if err != nil {
			log.Printf("ass-guard: mention provenance write failed (continuing): %v", err)
		}
	}

	out := append([]session.ContentBlock(nil), blocks...)
	out[idx] = session.ContentBlock{Type: blockText, Text: text.String()}

	return out
}

// resolveMention resolves ONE parsed mention into (section, resolved, form):
// the text to append (a labeled content/listing section, or the fixed-form
// loud note), the absolute path that answered the mention ("" when nothing
// did), and the provenance form. Resolution is bounded (T-21-14): relative
// paths must stay inside the workspace root; absolute paths are admitted
// ONLY by an explicit evaluator ruling — the nil default resolves nothing
// outside the workspace, so ingress never becomes a whole-FS existence
// oracle. Notes are fixed-form: token + outcome class, nothing else.
func (r *Runner) resolveMention( //nolint:funcorder,cyclop // one resolution ladder; grouped with the turn seam
	m ecosys.Mention,
) (section, resolved, form string) {
	root := r.workDirOrDefault()

	var abs string

	gated := false

	switch {
	case filepath.IsAbs(m.Path):
		// The nil default admits NO absolute path (no rules are declared to
		// admit any); a wired authority admits and gates in the one consult.
		allow, wired := r.readRuleEval(toolNameRead, m.Path)
		if !wired {
			return mentionNote(m.Token, mentionClassOutside), "", mentionFormUnresolved
		}

		if !allow {
			return mentionNote(m.Token, mentionClassDenied), "", mentionFormDenied
		}

		abs, gated = m.Path, true
	default:
		abs = filepath.Join(root, m.Path)

		if rel, err := filepath.Rel(root, abs); err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return mentionNote(m.Token, mentionClassOutside), "", mentionFormUnresolved
		}
	}

	info, err := os.Stat(abs)
	if err != nil {
		return mentionNote(m.Token, mentionClassMissing), "", mentionFormUnresolved
	}

	m.IsDir = info.IsDir() // parse left it unknown; resolution owns it

	if m.IsDir {
		listing, lerr := mentionDirSection(m.Token, abs)
		if lerr != nil {
			return mentionNote(m.Token, mentionClassUnreadable), "", mentionFormUnresolved
		}

		return listing, abs, mentionFormDir
	}

	// The Read-rule gate: every @file consults the evaluator BEFORE its
	// content enters the prompt (T-21-15; nil = implicit allow).
	if !gated && !r.readAllows(abs) {
		return mentionNote(m.Token, mentionClassDenied), "", mentionFormDenied
	}

	content, rerr := os.ReadFile(abs)
	if rerr != nil {
		return mentionNote(m.Token, mentionClassUnreadable), "", mentionFormUnresolved
	}

	return mentionFileSection(m.Token, content), abs, mentionFormFile
}

// readRuleEval consults the Read-rule authority for one path: the injected
// evaluator seam first (tests pin custom evaluators), else the Runner-scoped
// perm store — the SAME live rule authority the gate consumes (21-06's
// PAR-06 ↔ PAR-03 join; the 21-REVIEW WR-02/WR-03 hoist made it live and
// race-free). Only VerdictDeny denies. ok=false is the nil default: no seam
// and no store — the pre-21-06 implicit-allow shape (and absolute paths
// stay unadmitted).
func (r *Runner) readRuleEval(tool, path string) (allow, wired bool) {
	if f := r.readRuleEvaluator; f != nil {
		return f(tool, path), true
	}

	if st := r.permStore.Load(); st != nil {
		return st.Rules().Evaluate(tool, path) != perm.VerdictDeny, true
	}

	return false, false
}

// readAllows consults the Read-rule authority for one path (21-04, PAR-06):
// the nil default (no seam, no store) is the implicit allow — the pre-21-06
// behavior preserved for rule-less runners.
func (r *Runner) readAllows(path string) bool {
	allow, wired := r.readRuleEval(toolNameRead, path)

	return !wired || allow
}

// mentionNote renders one fixed-form loud note (D-10: the turn proceeds; the
// note names the token + the outcome class, never per-path detail or
// content — the flagged privacy prohibition).
func mentionNote(token, class string) string {
	return "\n\n[" + token + "] could not be expanded: " + class + "."
}

// mentionFileSection renders one @file's labeled section: the raw-token
// header line followed by the file's content verbatim.
func mentionFileSection(token string, content []byte) string {
	return "\n\n[" + token + "]\n" + string(content)
}

// mentionDirSection renders one @dir's ONE-LEVEL listing (D-10): one entry
// per line — name + size in bytes, subdirectories marked with a trailing
// slash. NO recursion: only the directory's immediate entries appear
// (os.ReadDir never descends into the children).
func mentionDirSection(token, dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("call: %w", err)
	}

	var b strings.Builder

	b.WriteString("\n\n[")
	b.WriteString(token)
	b.WriteString("]\n")

	for _, e := range entries {
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}

		size := int64(0)
		if info, ierr := e.Info(); ierr == nil {
			size = info.Size()
		}

		b.WriteString(e.Name())
		b.WriteString(suffix)
		b.WriteString(" (")
		b.WriteString(strconv.FormatInt(size, 10))
		b.WriteString(" bytes)\n")
	}

	return b.String(), nil
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

	if _, found := r.resolveSlashCommand(key); !found {
		return "", "", false
	}

	return key, args, true
}

// Run drives one session/prompt through the real Session Core.
func (r *Runner) Run( //nolint:funlen // the turn pipeline's composition root
	ctx context.Context, sessionID string,
	emit acp.ChunkEmitter, prompt []acp.ContentBlock,
) (string, error) {
	sess := r.sessionFor(ctx, sessionID)
	if sess == nil {
		// 16-REVIEW WR-05: sessionFor could not construct the session (both
		// transcript locations unusable — the cause is on stderr). A typed
		// error surfaces the real failure; the old shape nil-derefed in the
		// turn path and the client only ever saw a generic -32603.
		//nolint:err113 // dynamic, caller-facing
		return "", fmt.Errorf("session %s unavailable: transcript manager could not be created", sessionID)
	}

	// 23-02 (SEEDG-01): the pre-mutex ingress classifier. This is
	// load-bearing placement — everything below the Lock holds the session
	// turn mutex, so a classifier that waited there could never observe an
	// active turn without self-deadlocking (Pitfall 4's shape): queue-behind
	// masquerading as steering, the exact descope SEEDG-01 forbids. A
	// plain-text prompt with a turn or chain ACTIVE enqueues on the
	// session's SteerQueue and returns promptly (the running turn is
	// untouched — D-04); everything else falls through to today's flow
	// unchanged.
	if stop, handled := r.routeSteering(sess, sessionID, emit, prompt); handled {
		return stop, nil
	}

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
	// 21-03 (PAR-05/D-13): AgentThoughtChunk joins the same forwarder — the
	// live agent_thought_chunk frames ride the ordered emitter chain.
	ch := r.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	thoughtCh := r.bus.Subscribe("AgentThoughtChunk", event.BufAgentThoughtChunk)
	toolCh := r.bus.Subscribe("ToolCall", event.BufToolCall)
	toolUpdCh := r.bus.Subscribe("ToolCallUpdate", event.BufToolCallUpdate)

	defer r.bus.Unsubscribe("AgentMessageChunk", ch)
	defer r.bus.Unsubscribe("AgentThoughtChunk", thoughtCh)
	defer r.bus.Unsubscribe("ToolCall", toolCh)
	defer r.bus.Unsubscribe("ToolCallUpdate", toolUpdCh)

	done := make(chan struct{})
	promptDone := make(chan struct{})

	startChunkForwarder(ctx, ch, thoughtCh, toolCh, toolUpdCh, emit, promptDone, done)

	blocks := toContentBlocks(prompt)

	// 21-05 (PAR-06/D-11): provider-capability validation FIRST — a provider
	// whose protocol cannot carry image content blocks gets them dropped
	// here with EXACTLY ONE loud note naming it, BEFORE ingress persists
	// anything and BEFORE the transcript append (an undeliverable image
	// never becomes a Ref line). The turn proceeds with the text — never
	// silent, never a dead turn. The shaper stays pure (no capability flag):
	// the turn path is the one seam that knows both the blocks and the
	// session's provider.
	blocks = r.dropUnsupportedImages(sess, blocks)

	// 21-05 (PAR-06/D-09): image ingress at the turn entry — BEFORE the
	// transcript append and before either turn path (engine or plain) sees
	// the blocks: validate (DecodeConfig-first), auto-downscale, persist
	// originals, rewrite to the Ref+metadata form. Blocks without image
	// payloads pass through byte-identically.
	blocks = r.ingressImages(sess, blocks)

	// 12-01 reply routing (ACP-01): a prompt arriving while an ask is pending
	// is the OPERATOR'S ANSWER, not a new turn (see routeAskReply).
	if stop, handled := r.routeAskReply(ctx, sess, blocks); handled {
		close(promptDone)
		<-done

		return stop, nil
	}

	// 20-01 (CMDS-02, D-05): the class-B intercept — AFTER routeAskReply (the
	// ask answer outranks a command), BEFORE the engine branch below (an
	// intercept after it would diverge engine-on/off — RESEARCH Anti-Pattern
	// 3). A LIVE builtin winner answers the turn control-plane-fast: echo +
	// output chunks through the in-hand emit handle, a durable local_command
	// line, stopReason end_turn — ZERO provider calls, zero engine
	// involvement. Class-A (/init), dormant reserved names, skills, agents,
	// and file commands fall through to the existing flow unchanged.
	if stop, handled := r.tryLocalCommand(ctx, sess, emit, blocks); handled {
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

	if adv := <-advDone; adv != nil {
		if adv.askSignal != "" {
			// 17-04 (D-09): the engine ask rides the ask queue as a form; the
			// plain advisory note stays its degraded landing.
			r.enqueueEngineAsk(sess, adv.turnID, adv.askSignal)
		} else if r.advisoryNoteDue(sessionID, adv.class) {
			_ = emit.AgentMessageChunk(adv.turnID, adv.text)
		}
	}

	return mapAskStop(stop), err
}

// routeSteering is the pre-mutex ingress classifier (23-02, SEEDG-01 /
// Pattern 6): it runs at the very top of Run, BEFORE the session turn mutex,
// using only the existing race-tested active-state primitives —
// clientTurnActive (the turnActive map) and chainCount (parked chains hold
// no mutex while chainCount > 0). No new flags.
//
// Classification order (the locked order — Pitfall 11's ambiguity matrix):
//
//  1. Nothing active (no client turn, no chain) → ordinary new turn
//     (handled=false; today's flow under the mutex, byte-identical).
//  2. Pending broker ask → handled=false: the input is the operator's
//     ANSWER (or the 23-02 cancel grammar), routed by routeAskReply under
//     the mutex exactly as today — the ask route OUTRANKS steering. (When a
//     broker ask is pending, the suspending turn already released the
//     mutex, so this fall-through never blocks on a turn.)
//  3. Class-B resolve slot (the 20-01 locked contract: AFTER the ask
//     route, BEFORE steering enqueue): a chain-resolved slash-command
//     invocation NEVER becomes steering text — it falls through to the
//     ordinary path. 23-05 fills the pre-mutex /undo handling here (D-12's
//     auto-cancel could never run under the held mutex — Pitfall 4).
//  4. Plain text with a turn/chain ACTIVE → steering: enqueue on the
//     session's SteerQueue, emit the queued note through the in-hand emit
//     (the live-emitter precedent — never a post-turn bus publish, which
//     is LOST), and return end_turn immediately (Open Question 1's
//     resolution: return-after-enqueue). The running turn continues via the
//     session-lifetime forwarder (WINDOWS #3 split) and delivers the
//     steering at its next model-request boundary (23-01's drain).
//
// The steered enqueue path returns the ticket to the transport layer's
// disposal: the transport-neutral API hands it to the caller (TG-02's ack
// contract); the ACP adapter drops it — the queued note and the boundary
// drain's "steering applied: N inputs" are the operator-visible record.
func (r *Runner) routeSteering(
	sess *session.Session, sessionID string, emit acp.ChunkEmitter, prompt []acp.ContentBlock,
) (string, bool) {
	if !r.clientTurnActive(sessionID) && r.chainCount(sessionID) == 0 {
		return "", false // nothing active — the ordinary new-turn path
	}

	// Step 2: a pending ask outranks steering (handled under the mutex).
	if sess.HasPendingAsk() {
		return "", false
	}

	blocks := toContentBlocks(prompt)

	idx := firstTextBlockIndex(blocks)
	if idx < 0 || blocks[idx].Text == "" {
		// Non-text input mid-turn: steering is text-only in v1 — the
		// ordinary path (queue-behind) applies.
		return "", false
	}

	// Step 3: the class-B resolve slot — a chain-resolved invocation never
	// steers (20-01's locked position; 23-05 fills /undo here).
	if r.resolvesAsCommand(blocks[idx].Text) {
		return "", false
	}

	q := sess.SteerQueue()
	if q == nil {
		return "", false // steering unwired (bare sessions) — ordinary path
	}

	_ = q.Enqueue(blocks[idx].Text) // ticket: transport-layer disposal (see doc)

	note := fmt.Sprintf("steering queued — %d pending", q.Pending())

	if err := emit.AgentMessageChunk(sess.CurrentTurnID(), note); err != nil {
		log.Printf("ass-guard: steering queued-note emit failed (input stays queued): %v", err)
	}

	return stopEndTurn, true
}

// resolvesAsCommand reports whether text is a chain-resolved slash-command
// invocation (the class-B predicate — 23-02): the SAME parse+resolve pair
// tryLocalCommand runs, WITHOUT its side effects (no echo, no handler, no
// resolve counter). Only a RESOLVED name counts — an unknown /word is plain
// prose to the system and steers like any text.
func (r *Runner) resolvesAsCommand(text string) bool {
	key, _, ok := ecosys.ParseInvocation(text)
	if !ok {
		return false
	}

	_, found := r.commandChainRef().resolve(key)

	return found
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
// 21-03 (PAR-05/D-13): AgentThoughtChunk events join the same forwarder as
// agent_thought_chunk frames through the ActivityEmitter assertion.
func startChunkForwarder(
	ctx context.Context,
	ch, thoughtCh, toolCh, toolUpdCh <-chan event.Event,
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
			case e, ok := <-thoughtCh:
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
					case e := <-thoughtCh:
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
// ToolCallUpdate → the ACP-03 tool-card frames; AgentThoughtChunk → the
// agent_thought_chunk frame via the ActivityEmitter seam — 21-03, PAR-05).
// Shared by the per-Run and the session-lifetime forwarders.
func routeBusEvent(e event.Event, emit acp.ChunkEmitter, toolEmit acp.ActivityEmitter) {
	switch c := e.(type) {
	case event.AgentMessageChunk:
		_ = emit.AgentMessageChunk(c.MessageID, c.Content)
	case event.AgentThoughtChunk:
		forwardThoughtChunk(toolEmit, c)
	case event.ToolCall:
		forwardToolCall(toolEmit, e)
	case event.ToolCallUpdate:
		forwardToolCallUpdate(toolEmit, e)
	}
}

// forwardThoughtChunk mirrors one bus AgentThoughtChunk event as a v1
// agent_thought_chunk frame (PAR-05, 21-03). A nil emitter (plain
// ChunkEmitter fake) is a no-op — the same 16-01 type-assert skip the tool
// cards use; the frame vocabulary itself is additive (16-D-20).
func forwardThoughtChunk(toolEmit acp.ActivityEmitter, c event.AgentThoughtChunk) {
	if toolEmit == nil {
		return
	}

	_ = toolEmit.ThoughtChunk(c.MessageID, acp.ContentBlock{Type: blockText, Text: c.Content})
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
	// 16-REVIEW WR-01: the engine wiring is PER-INVOCATION, not a rebind of
	// shared state. The old code pointed r.hookExec's seams and r.eng.Manager
	// at the current session on every engine-enabled turn; two sessions turning
	// concurrently — and a parked 13-00 chain resuming AFTER a later session's
	// rebind — then executed hook DAGs and wrote engine decisions into the
	// WRONG session's transcript. SetupEngine's r.hookExec/r.eng stay
	// session-free templates (Bus/Log only); this turn gets its own
	// session-bound executor, dispatcher, and engine instance.
	turnExec := &hookdag.Executor{
		// The hook-DAG seams bind to the ACTIVE session so ActionHook can launch
		// the post-implement/post-phase DAG against the real Session Core
		// (HOOK-05 — send-prompt IS a turn; fresh-context IS a boundary).
		Bus:        r.hookExec.Bus,
		Log:        r.hookExec.Log,
		Commands:   enginebridge.NewRealCommandRunner(),
		Turns:      enginebridge.NewHookSessionTurnRunner(sess),
		Boundaries: enginebridge.NewHookSessionBoundaryOpener(sess.Manager),
	}

	turnDispatcher := enginebridge.NewACPDispatcher(&enginebridge.BridgeConfig{
		Hooks:         turnExec,
		HookCfg:       r.hookCfg,
		Learned:       r.learned,
		Bus:           r.bus,
		NextPromptFor: r.nextPromptFor,
	})

	// engine_decision lines land in THIS session's transcript (the Manager is
	// per-session; the shared Engine template carries none).
	turnEng := &engine.Engine{
		Bus:        r.eng.Bus,
		Log:        r.eng.Log,
		Manager:    sess.Manager,
		Dispatcher: turnDispatcher,
	}

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
	// CloseSession (logout) and closeAllSessions (serve end); a session/cancel
	// of a LIVE turn drains its chain transitively via the request-ctx watchdog
	// below (16-REVIEW CR-01: cancel must not reap the session, but the
	// cancelled turn's parked chain still drains — ENG-03).
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
		stop, err := turnEng.Observe(parkedCtx, adapter, r.patternTable, blocks)

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
// (13-00): cancelParkedChains — CloseSession (logout) and closeAllSessions
// (serve end) — drains every parked chain of that session.
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

// errPermissionAskSurfaceUnwired is the runtime-side sentinel for the gate's
// fire wrapper when the serve composition has not injected the acpserve
// surface yet (the fail-safe decline path — never a silent allow).
var errPermissionAskSurfaceUnwired = errors.New("permission ask surface not wired")

// ResumeSession is the 18-01 SessionLoader seam, widened by 18-05 to the
// FULL transcript-side resume contract (ACP-06/D-01 — one place, the D-01
// ordering's foundation): it adopts the PAST session's id (sessionFor opens
// the Manager under the transcript's own sessionID — transcript_<id>.jsonl
// IS the session identity on resume), classifies every dangling expectation
// (session.Reconcile), APPENDS each provenance-marked synthetic closure
// through Manager.AppendSynthetic (D-02 — on disk, never a rewrite; the acp
// load core then replays the post-closure file), and seeds the session state
// (turn counter from the maxima, Pitfall 2; plan-mode target from the LAST
// plan_mode line, Pitfall 3). Called by the acp session/load handler BEFORE
// replay; reconstruction is transcript-local (D-01) — no provider call
// happens here. A ReadAll failure degrades LOUDLY but still seeds what was
// readable (the AUD-03 audit-write discipline): a partially readable
// transcript yields a partial seed, never a refused resume. A closure append
// failure is loud and non-fatal — the closures already on disk stay, the
// next ResumeSession re-classifies and re-appends only what is missing
// (idempotent by construction, the kill-during-replay row's guarantee).
func (r *Runner) ResumeSession(ctx context.Context, sessionID string) error {
	sess := r.sessionFor(ctx, sessionID)
	if sess == nil {
		// 16-REVIEW WR-05 shape: both transcript locations unusable — the typed
		// caller error surfaces the real failure (logged at the source).
		//nolint:err113 // dynamic, caller-facing
		return fmt.Errorf("session %s unavailable: transcript manager could not be created", sessionID)
	}

	// 17-REVIEW CR-02 discipline, extended to the load path (review WR-01):
	// ReadAll → Reconcile → AppendSynthetic → SeedResume mutates the transcript
	// and the turn counter, so the block holds the SAME per-session turn mutex
	// Run holds for a whole turn (and every async resume driver serializes
	// through). A claimed timer resume still mid-model-loop when the ACP entry
	// drops (close/logout) is untracked by turnWG; an immediate re-load racing
	// it would otherwise double-append one transcript and re-store the turn
	// counter underneath a turn that already minted its next id.
	turnMu := r.sessionTurnMu(sessionID)
	turnMu.Lock()
	defer turnMu.Unlock()

	lines, rerr := sess.Manager.ReadAll()
	if rerr != nil {
		// Loud degrade (plan-pinned): seed what was readable — ReadAll skips
		// non-conforming lines, so a nil slice seeds 0 and the sequence starts
		// fresh rather than the load refusing.
		log.Printf("ass-guard: resume read failed for %s (%v); seeding from what was readable",
			sessionID, rerr)
	}

	closures, seed := session.Reconcile(sessionID, lines)

	for i := range closures {
		aerr := sess.Manager.AppendSynthetic(&closures[i])
		if aerr != nil {
			log.Printf("ass-guard: resume closure append failed for %s (%s): %v — "+
				"continuing (the next resume re-appends what is missing)",
				sessionID, closures[i].Type, aerr)
		}
	}

	planTarget := ""
	if seed.PlanModePresent {
		planTarget = seed.PlanMode
	}

	sess.SeedResume(seed.MaxTurns, planTarget)

	return nil
}

// LoadedModes satisfies the acp ModeStateProvider optional capability
// (18-05, ACP-06): the resumed session's v1 SessionModeState-shaped data for
// the load response's modes field — read AFTER ResumeSession seeded the
// session (the load core's call order), nil when the session never recorded
// a mode transition (wire modes null; the client assumes defaults).
func (r *Runner) LoadedModes(sessionID string) any {
	r.sessMu.Lock()
	sess := r.sessions[sessionID]
	r.sessMu.Unlock()

	if sess == nil {
		return nil
	}

	return sess.ModesState()
}

// CommandRegistry exposes the discovered slash-command registry (08-04) for
// the acpserve composition's available_commands_update adapter (18-05/ACP-06
// "commands re-advertised"; Phase 20's session/new advertisement reuses the
// same source).
func (r *Runner) CommandRegistry() ecosys.Registry { return r.registrySnapshot() }

// registrySnapshot returns the live registry value under the read lock (the
// map headers copy; the maps themselves are immutable post-install — the
// rescan builds fresh ones wholesale).
func (r *Runner) registrySnapshot() ecosys.Registry {
	r.regMu.RLock()
	defer r.regMu.RUnlock()

	return r.reg
}

// The checkpoint GC's embedded defaults (23-04, D-10/D-08): what the sweep
// runs on when no persisted checkpoint: key exists — expiry 7 days, count 50
// per session (A3, aligned with checkpoint.DefaultKeep).
const (
	defaultCheckpointExpiryDays = 7
	defaultCheckpointPerSession = 50
)

// SetCheckpointGCBounds binds the D-08 bounds seam (23-04 Task 2: the serve
// composition injects the config surface's effective read-back; nil resets
// to the embedded defaults).
func (r *Runner) SetCheckpointGCBounds(fn func() (days int, perSession int)) {
	r.ckptMu.Lock()
	defer r.ckptMu.Unlock()

	r.checkpointGC = fn
}

// checkpointGCBounds resolves the sweep bounds: the bound seam when
// injected, else the embedded 7d/50 defaults. A seam result of
// non-positive values falls back per-axis (never a zero-day expiry, never an
// unbounded count).
func (r *Runner) checkpointGCBounds() (days int, perSession int) {
	r.ckptMu.Lock()
	fn := r.checkpointGC
	r.ckptMu.Unlock()

	days, perSession = defaultCheckpointExpiryDays, defaultCheckpointPerSession

	if fn == nil {
		return days, perSession
	}

	if d, p := fn(); d > 0 && p > 0 {
		days, perSession = d, p
	}

	return days, perSession
}

// checkpointStore returns the WORKSPACE-level checkpoint store, opening it
// lazily exactly once (23-04, Pitfall 9): the session snapshot adapter, the
// restore guard's callers (/undo, 23-05), and the session-start sweep all
// share this one instance. An open failure degrades LOUDLY (the AUD-03
// discipline — one log line) and pins nil: sessions run WITHOUT undo
// snapshots, /undo reports unavailable — never a silent trap.
func (r *Runner) checkpointStore() *checkpoint.Store {
	r.ckptMu.Lock()
	defer r.ckptMu.Unlock()

	if r.ckptOpenTried {
		return r.ckptStore
	}

	r.ckptOpenTried = true

	dir := r.workDir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	st, err := checkpoint.Open(dir) //nolint:contextcheck // Store.Open carries no ctx
	if err != nil {
		log.Printf("ass-guard: checkpoint store disabled for %s (%v) — turns run WITHOUT undo snapshots (/undo will report unavailable)", dir, err)

		return nil
	}

	r.ckptStore = st

	// SEEDG-02: the user repo's .git/info/exclude carries the store root —
	// append rides store open (idempotent; the typed skips for non-repo and
	// worktree workdirs are structured notes, never failures).
	if xerr := st.EnsureUserRepoExclude(); xerr != nil {
		if errors.Is(xerr, checkpoint.ErrExcludeSkippedNotRepo) ||
			errors.Is(xerr, checkpoint.ErrExcludeSkippedWorktree) {
			log.Printf("ass-guard: user-repo exclude skipped: %v", xerr)
		} else {
			log.Printf("ass-guard: user-repo exclude append failed (%v) — git status may show .ass-guard/", xerr)
		}
	}

	return r.ckptStore
}

// restoreBlockedError is the SEEDG-02 restore refusal (23-04): a typed error
// NAMING the blocking state — the operator (and /undo's output, 23-05) can
// tell an active turn from a parked chain at a glance.
type restoreBlockedError struct {
	turn  bool
	chain bool
}

func (e *restoreBlockedError) Error() string {
	switch {
	case e.turn:
		return "checkpoint: restore refused: a client turn is active for the session — cancel it or wait for it to end"
	case e.chain:
		return "checkpoint: restore refused: an engine chain is active (possibly parked) for the session — cancel it or wait for the chain to finish"
	default:
		return "checkpoint: restore refused: session busy"
	}
}

// restoreBlockers is the SEEDG-02 guard EVERY in-process restore path calls
// (23-05's /undo composes it): it refuses while a client turn is in flight
// OR an engine chain is active — including PARKED chains (chainCount > 0
// with no mutex held — the trap: a mutex-ownership check would pass and let
// the restore race the chain's next injection). The check reads
// turnActive/chainCount ONLY — it never touches the turn mutex, so it
// returns promptly under a blocked turn (the behavioral pin). The v1.1 CLI
// restore path is a separate process: in-process state is invisible there
// by construction (Open Question 3's documented constraint — no
// cross-process detection is built).
func (r *Runner) restoreBlockers(sessionID string) error {
	if r.clientTurnActive(sessionID) {
		return &restoreBlockedError{turn: true}
	}

	if r.chainCount(sessionID) > 0 {
		return &restoreBlockedError{chain: true}
	}

	return nil
}

// sessionFor returns the Session for sessionID, creating it on first use.
func (r *Runner) sessionFor( //nolint:funcorder,funlen,maintidx,cyclop,gocyclo,gocognit // turn pipeline grouping
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
		// 16-REVIEW WR-06: reuse is ACTIVITY. The firing-target contract
		// (currentSessionID) is most-recently-ACTIVE — re-prompting an older
		// session (or an automation firing into it) must make it the target
		// again, not leave the marker on the last-CREATED session (sessMu is
		// held for the whole construction, so this write is race-free).
		r.lastSessionID = sessionID

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
	// 23-04 (Pitfall 9): the store is RUNNER-owned (checkpointStore — built
	// once per workspace, loudly degraded on open failure); the session
	// attaches its snapshot adapter from the shared instance.
	ckptStore := r.checkpointStore()

	// 23-04 (SEEDG-02, D-08): the session-start GC sweep — the retention
	// authority (age+count dual axis with object expiry). Bounds come from
	// the checkpointGCBounds seam (Task 2's config read-back; the embedded
	// 7d/50 defaults until then). A sweep failure degrades loudly and never
	// fails the session (AUD-03).
	if ckptStore != nil {
		days, perSession := r.checkpointGCBounds()

		if serr := ckptStore.Sweep(ctx, time.Duration(days)*24*time.Hour, perSession); serr != nil {
			log.Printf("ass-guard: checkpoint GC sweep failed for %s (%v) — continuing without sweeping", sessionID, serr)
		}
	}

	mgr, err := session.NewManager(dir, sessionID, redactorAdapter{})
	if err != nil {
		log.Printf("ass-guard: transcript open failed for %s (%v); retrying in temp", dir, err)

		mgr, err = session.NewManager(filepath.Join(os.TempDir(), "ass-guard"), sessionID, redactorAdapter{})
		if err != nil {
			// 16-REVIEW WR-05: both transcript locations are unusable. The old
			// code discarded this error and left mgr nil — the very next
			// mgr.Path() use panicked, recovered per-dispatch into a generic
			// -32603 on EVERY prompt with the real cause (both locations
			// unwritable) swallowed. Degrade deterministically: nil session,
			// typed caller error, loud stderr.
			log.Printf("ass-guard: transcript open failed in temp too (%v) — session %s is unavailable",
				err, sessionID)

			return nil
		}
	}

	// G-18-1: the session_start opener is the transcript's FIRST line, so it
	// can ONLY be written at creation — append-only discipline means never a
	// rewrite, and an opener anywhere but first is useless to the list scan.
	// The size gate is what makes resume safe: ResumeSession reopens the SAME
	// transcript through this same path, and a transcript with bytes gets NO
	// second opener (exactly one session_start line per transcript, still the
	// first). A size-0 file also self-heals a kill -9 between create and
	// first append. The gate covers whichever manager survived the fallback
	// chain above and sits under r.sessMu (held for the whole construction) —
	// race-free by construction. A stat or append failure degrades LOUDLY and
	// construction continues (the AUD-03 audit-write discipline, never a
	// serve refusal): a session that could not write its opener still runs
	// turns; it just stays legacy-shaped for listing.
	fi, serr := os.Stat(mgr.Path())
	if serr != nil {
		log.Printf("ass-guard: transcript stat failed for %s (%v) — session_start opener not written",
			mgr.Path(), serr)
	} else if fi.Size() == 0 {
		aerr := mgr.AppendSessionStart(sessionID)
		if aerr != nil {
			log.Printf("ass-guard: session_start opener write failed for %s (%v) — session stays legacy-shaped",
				sessionID, aerr)
		}
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
	if listing := ecosys.SkillListing(r.registrySnapshot()); listing != "" {
		prof.System = append(append([]profile.TextBlock(nil), prof.System...),
			profile.TextBlock{Type: blockText, Text: listing})
	}

	// 16-05 (ACP-08 live apply): sessions created after an editor-driven model
	// change stamp the effective model at construction — the future-sessions
	// leg of the live-apply seam (the live-session leg is ApplyTurnModel).
	// 16-09 (gap 4b): with NO editor stamp the default is the tier-resolved
	// config model — the same value the advertisement displays (chip==wire);
	// the explicit stamp keeps absolute precedence (D-12).
	m := r.effectiveModelFor()
	if m == "" {
		m = r.defaultTurnModel()
	}

	if m != "" {
		prof.Model = m
	}

	// 12-02 (Task 3): the agent-type listing — the same dedicated-system-block
	// dynamic merge as the skills listing, in the captured Agent-tool
	// type-entry shape. Discovered definitions (plugin-bundled AND first-class
	// `.claude/agents/`) surface here AND as spawnable types below.
	if agentListing := ecosys.AgentListing(r.registrySnapshot()); agentListing != "" {
		prof.System = append(append([]profile.TextBlock(nil), prof.System...),
			profile.TextBlock{Type: blockText, Text: agentListing})
	}

	// 21-02 (PAR-04): the memory injection — the FOURTH trailing-TextBlock
	// dynamic merge (the exact skills/agent-listing vehicle): AGENTS.md /
	// CLAUDE.md bodies discovered cwd→git-root + the user global, mtime-cached
	// and D-08-capped, ride one block AFTER AgentListing so memory stays the
	// closest-to-conversation static context. Zero memory files → no merge
	// (the body renderer's render-then-skip contract); the double-append keeps
	// the shared r.profile unmutated. The renderer cannot fail — every
	// degradation is a note inside the body.
	if memBody := ecosys.MemoryInjection(dir); memBody != "" {
		prof.System = append(append([]profile.TextBlock(nil), prof.System...),
			profile.TextBlock{Type: blockText, Text: memBody})
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
		core.Execute = ecosys.SkillExecute(r.registrySnapshot())
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

	// 22-02 (D-12): the caps resolve per session construction
	// (apply-as-landed — running sessions keep theirs).
	subsCap, bashCap := 8, 16 // D-10/D-11 defaults

	if r.backgroundCaps != nil {
		if s, b := r.backgroundCaps(); s > 0 && b > 0 {
			subsCap, bashCap = s, b
		}
	}

	taskRegistry := coreexec.NewTaskRegistry()
	taskRegistry.Cap = bashCap

	// 22-04 (PAR-09/D-07/D-08): the session's ONE persistent-shell PTY
	// manager — lazily started (no shell exists until the first persistent
	// Bash call), shared by every persistent call of the session, and
	// drained on close (the OnClose link, Task 3). The dead-shell restart
	// note rides the loud stderr family (one line per restart — D-08's
	// visible state-loss acknowledgment, never silent).
	ptyMgr := coreexec.NewPTYManager(coreexec.PTYOpts{
		WorkDir: dir,
		NoteFn: func(format string, args ...any) {
			_, _ = fmt.Fprintf(r.stderrOrDefault(), "ass-guard: session %s "+format+"\n",
				append([]any{sessionID}, args...)...)
		},
	})
	r.ptyManagers.Store(sessionID, ptyMgr)

	// 22-01 (D-01..D-03, PAR-07/PAR-08): the ONE task-notification tracker
	// beside the registry. Registry completions land as kind-tagged
	// Notifications (primitive-arg CompletionHook — no coreexec→tasks
	// import; this adapter owns the mapping); every completion schedules the
	// session's wake-drain chain; OnClose drops queued-but-unstarted
	// subagent registrations with a counted note (OQ5).
	tracker := tasks.NewTracker(tasks.TrackerOpts{SubagentCap: subsCap})
	r.trackers.Store(sessionID, tracker)

	taskRegistry.CompletionHook = func(taskID, kind, exitStatus string, duration time.Duration, tail, outputFile string) {
		tracker.Complete(tasks.Notification{
			TaskID: taskID, Kind: tasks.Kind(kind), ExitStatus: exitStatus,
			Duration: duration, Tail: tail, OutputFile: outputFile,
		})
	}

	tracker.SetDrain(func(pending []tasks.Notification) {
		_ = pending // peek only — the chain re-reads authoritatively via Drain

		r.scheduleWakeDrain(sessionID)
	})

	// 22-03 (PAR-07): the background-subagent launcher seam — the session's
	// run_in_background dispatches hand the launch to tasks.RunBackgroundSubagent
	// over this session's tracker. The loop runs under the SERVE-lifetime ctx
	// (the dispatching turn's ctx dies at return); progress streams from the
	// bus (chunks + tool calls tagged with the subagent's turn id) into the
	// task's output file. The 20-03 routing plan rides the request VERBATIM
	// (Pattern 7: DispatchSubagentBackground resolved it through the SAME
	// planSubagent call site the foreground path uses — never a second
	// resolver).
	backgroundLaunch := func(req session.BackgroundDispatchRequest) session.BackgroundDispatchResult {
		r.sessMu.Lock()
		sessLocal := r.sessions[sessionID]
		r.sessMu.Unlock()

		launch := tasks.RunBackgroundSubagent(tasks.SubagentDeps{
			Run: func(bgCtx context.Context, progress func(string)) (string, error) {
				// 22-03 (OQ3): arm the ask-decline for the loop's ctx — a
				// background subagent has no human; ask-class tools decline
				// with the 17-D-07 note (scoped to this ctx, never the
				// session — a concurrent client turn is unaffected).
				return runSubagentWithProgress(
					session.ContextWithBackgroundSubagent(bgCtx), sessLocal, req, r.bus, progress)
			},
			WorkDir:  dir,
			Tracker:  tracker,
			ServeCtx: r.serveCtxOrBackground,
		})

		return session.BackgroundDispatchResult{
			TaskID: launch.TaskID, OutputFile: launch.OutputFile,
			Queued: launch.Queued, Note: launch.Note, Err: launch.Err,
		}
	}

	coreexec.RegisterCore(sCatalog, coreexec.Config{
		WorkDir: dir, Todos: coreexec.NewTodoStore(), Hooks: hookRunner, Tasks: taskRegistry,
		PTY: ptyMgr, // 22-04 (PAR-09): the session's persistent shell
	})

	// The session variable is declared BEFORE the broker literal so the
	// onSurface closure can hand the pending ask to the queue at FIRE time
	// (mid-turn — sess is fully constructed by then; the 09-01 capturer
	// precedent: closures read late-bound variables, never stale copies).
	var sess *session.Session

	// 12-01 (ACP-01/D-01): the per-session AskUserQuestion surface. The
	// broker holds the pending ask; its surface callback routes the ask.
	// 17-04 (ACP-02/D-09): a QUESTION-shaped ask enqueues on the ask queue —
	// the ONE firing path — whose fire is the capability-gated elicitation
	// dispatcher (the plain-text publish is the dispatcher's FALLBACK branch
	// now, verbatim). Non-question kinds (plan approval keeps its captured
	// string-reply forms) and the unwired-surface degrade keep today's
	// direct publish. Timer-driven resumes run under the serve-lifetime ctx
	// (the suspending turn's ctx dies with its response). The executor
	// registration is the SAME Execute-only override discipline as
	// RegisterCore (the captured schema is never rewritten).
	askBroker := session.NewAskBroker(r.askTimeout, func(p session.PendingAsk) {
		if p.Kind != session.PendingAskKindQuestion || r.askFire == nil || sess == nil {
			r.PublishAskChunk(p.TurnID, coreexec.RenderAskSurface(p.Questions))

			return
		}

		sess.EnqueueElicitationAsk(p, r.askFire)
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
	// BEFORE the Session literal (moved above the ask-broker literal, 17-04)
	// and assigned after — the closure reads CurrentTurnID() at FIRE time
	// (mid-turn), so it sees the in-flight turn. Header VALUES are dropped
	// at the closure (_ per Pitfall 9 discipline: values never enter audit
	// artifacts; names ride 09-06 events only).
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

		// 20-03 (D-13/D-15): dispatch-time per-agent model routing — the
		// runtime resolver hands each dispatch its (model, provider) plan
		// (frontmatter > dispatch-time > session/parent > tier; inherit
		// normalizes to unset; cross-provider ROUTES with one-warning
		// degrade). 14-05's session-level SubagentModel stamp is REVERSED:
		// the tier arm fires only when the session has NO model at all.
		SubagentModelPlanner: r.planSubagentDispatch,

		// 20-03: the LIVE agent registry view (the chain accessor — an agent
		// file added after session construction stays dispatchable across a
		// rescan swap; SubagentTypes stays as the bare-test fallback).
		AgentLookup: r.liveAgentLookup,

		// 22-03 (PAR-07): the background-subagent launcher (tracker +
		// serve-ctx detach + the output-file progress tee).
		BackgroundDispatch: backgroundLaunch,

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
	// 23-02 (SEEDG-01): the per-session steering queue — mid-turn inputs
	// enqueue here through the pre-mutex classifier (routeSteering); the
	// session's runTurn drains it at every model-request boundary (23-01).
	// Transport-neutral: a non-ACP frontend (TG-02) enqueues through the
	// same object.
	s.SetSteerQueue(session.NewSteerQueue())
	// 17-REVIEW CR-02: serialize every ASYNC resume driver (the ask queue's
	// pump resolution — permission AND question families — and the D-01
	// timer) on the SAME per-session turn mutex Run holds for its whole turn.
	// The session cannot import the runtime, so the guarded execution is
	// injected as a func (the OnSuspended/ParkMu injection precedent). The
	// sync reply path (routeAskReply inside Run) already holds the mutex and
	// bypasses this wrapper — it is not reentrant.
	turnMu := r.sessionTurnMu(sessionID)

	s.SetResumeSerial(func(f func()) {
		turnMu.Lock()
		defer turnMu.Unlock()

		f()
	})
	// 12-09 (G-12-3): wire the per-session plan-mode state onto the Session —
	// without this the Enter flip at the tool-result site is skipped
	// (Session.planMode nil), the mutating-tool gate never fires, and
	// ExitPlanMode answers "not in plan mode" (the live-session finding).
	s.SetPlanMode(planMode)

	// 19-05 (D-03): the session's compaction settings initialize from the
	// modelrouting-loaded effective values at construction (the floor's
	// 80/true unless a layer overrides) — the context limit resolved from
	// the capability table for THIS session's model. Unarmed (no scheduling
	// config — test runners) keeps 19-04's disabled zero value; the surface's
	// live-apply relay (ApplyCompactionSettings) re-stamps the same seam
	// between turns.
	if cs := r.effectiveCompaction(); cs.armed {
		s.SetCompactionSettings(cs.enabled, cs.pct, r.compactionContextLimit())
	}

	// 17-02 (ACP-01): the permission gate — THE one per-call chokepoint. The
	// perm store opens on the project's .ass-guard floor (0600 atomic, 17-01);
	// an open failure degrades LOUDLY to a rule-less session (deny rules
	// cannot be enforced without a store; the fail-safe story is untouched —
	// ungated stays today's behavior and gated still declines ask-class calls
	// rather than silent-allowing them). The mode accessor is runner-owned and
	// LIVE (Pitfall 8); the fire callback is read at CALL time so a late
	// injection from the serve composition is picked up.
	//
	// 17-03 (D-12): the session's ask queue notes ride the SUBSCRIBER-BACKED
	// bus path — the session-lifetime chunk forwarder (armed just below, for
	// this session's whole life) or the live client turn's own forwarder
	// delivers the AgentMessageChunk; a bare post-turn publish would be
	// dropped when no subscriber exists (the 13-03 timing hazard).
	// 23-02 (D-05): the emitter also publishes the parked-ask wording — the
	// emitter fires ONLY on the parked branch (an enqueue that fires
	// immediately emits nothing), so the D-05 note appears exactly when an
	// ask actually waits, without touching the immediate-fire wire shapes.
	askQueue := session.NewAskQueue()
	askQueue.SetNoteEmitter(r.publishAskQueueNote)

	permDeps := session.GateDeps{
		Mode:  r.PermMode,
		Queue: askQueue,
		// 21-06 (PAR-03/D-04): the hook-verdict HEAD — the session's
		// HookRunner (plugin bundles + both settings.json scopes + the
		// project settings.local.json override, D-03 firing order) resolves
		// PreToolUse verdicts; the gate head is the
		// ONE consumption site (nil-runner verdicts are a safe no-decision).
		// Deny blocks before rules; ask suspends even ungated; a USER-scope
		// allow executes; no-decision falls through to the rule evaluation.
		PreToolUseVerdict: hookRunner.PreToolUseVerdict,
		// The MCP namespace resolver (Pitfall 7): canonicalize through
		// 17-01's helpers — the catalog registers MCP tools under their full
		// mcp__<server>__<tool> names, so the mapping is a rebuild + identity.
		Subject: func(tool string) string {
			if srv, tl, ok := perm.SplitMCPName(tool); ok {
				return perm.MCPName(srv, tl)
			}

			return tool
		},
		Fire: func(ctx context.Context, e *session.AskEntry) session.AskOutcome {
			if r.permAskFire == nil {
				return session.AskOutcome{Err: errPermissionAskSurfaceUnwired}
			}

			return r.permAskFire(ctx, e)
		},
	}

	// 17-REVIEW WR-01: OpenRepaired fails SAFE on a corrupt document — the
	// unreadable file is quarantined (.corrupt), the floor file recreated, and
	// the degradation logged LOUDLY, so a hand-edit accident can never silently
	// zero the trust store (a deny rule that simply stopped denying). A
	// residual error here is environmental (mkdir/stat/create) — the session
	// degrades rule-less with the loud log, exactly as before.
	//
	// 21-REVIEW WR-02/WR-03: the store is RUNNER-scoped (21-06's PAR-06 ↔
	// PAR-03 join, hoisted): ONE live instance every session's gate AND the
	// @-mention Read-rule consult (readRuleEval) answer to — the sessions
	// already share the workDir/file, so per-session instances were one file
	// but N divergent in-memory authorities. sessMu is held here; the atomic
	// publish keeps the lock-free turn-time mention reads race-free. Only
	// VerdictDeny denies a mention (ask/allow/unmatched all expand); a nil
	// store (failed open) keeps the nil implicit-allow default and the next
	// sessionFor retries.
	permStore := r.permStore.Load()
	if permStore == nil {
		st, permErr := perm.OpenRepaired(filepath.Join(dir, ".ass-guard", "permissions.yaml"))
		if permErr != nil {
			log.Printf("ass-guard: permissions store disabled for %s (%v) — deny/allow rules UNENFORCED for this session",
				dir, permErr)
		} else {
			permStore = st
			r.permStore.Store(st)
		}
	}

	if permStore != nil {
		permDeps.Rules = permStore.Rules
		permDeps.Allow = permStore.AllowTool
		permDeps.Forbid = permStore.ForbidTool
	}

	s.SetPermissionGate(permDeps)
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
		ptyMgr.Drain()          // 22-04 (D-08/Pitfall 5): the persistent shell's group dies here too

		// 22-01 (OQ5): queued-but-unstarted subagent registrations die here
		// — silently dropped (nothing started, nothing to kill), the count
		// noted on stderr only when non-zero.
		if dropped := tracker.CancelQueued(); dropped > 0 {
			_, _ = fmt.Fprintf(r.stderrOrDefault(),
				"ass-guard: session %s close dropped %d queued background subagent task(s)\n", sessionID, dropped)
		}

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

// imageLimitsFor returns the effective ingress limits: the Runner's pinned
// set when non-zero, else DefaultImageLimits (D-09's Anthropic classes).
func (r *Runner) imageLimitsFor() ImageLimits {
	if r.imageLimits.MaxW != 0 || r.imageLimits.MaxH != 0 ||
		r.imageLimits.MaxBytes != 0 || r.imageLimits.TargetLongEdge != 0 {
		return r.imageLimits
	}

	return DefaultImageLimits
}

// sessionImagesDir derives the per-session images dir: beside the session's
// transcript under .ass-guard (the same self-gitignored tree openTranscript
// guarantees), falling back to the workdir tree when the session carries no
// Manager (test harnesses).
func (r *Runner) sessionImagesDir(sess *session.Session) string {
	if sess != nil && sess.Manager != nil && sess.Manager.Path() != "" {
		return filepath.Join(filepath.Dir(sess.Manager.Path()), "images")
	}

	return filepath.Join(r.workDirOrDefault(), ".ass-guard", "images")
}

// sourceMediaOf names the source bytes' DECODED format media type for the
// persisted original's file name (header-only re-parse — no pixel decode; the
// decoded format already won validation inside ValidateAndScaleImage).
func sourceMediaOf(data []byte) string {
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ""
	}

	return mediaTypesByFormat[format]
}

// dropUnsupportedImages is the D-11 leg (21-05, PAR-06): when the session's
// provider declares no image support, every image block is dropped with
// EXACTLY ONE loud note NAMING THE PROVIDER — the turn proceeds with the
// text, never silent, never a dead turn. Runs BEFORE ingress (nothing is
// persisted for an undeliverable image) and BEFORE the transcript append.
// A nil provider (test-constructed sessions) or an image-capable provider
// passes the blocks through untouched.
func (r *Runner) dropUnsupportedImages(
	sess *session.Session, blocks []session.ContentBlock,
) []session.ContentBlock {
	if sess == nil || sess.Provider == nil || sess.Provider.SupportsImages() {
		return blocks
	}

	hasImage := false

	for _, b := range blocks {
		if b.Type == blockImage {
			hasImage = true

			break
		}
	}

	if !hasImage {
		return blocks
	}

	out := make([]session.ContentBlock, 0, len(blocks))
	for _, b := range blocks {
		if b.Type != blockImage {
			out = append(out, b)
		}
	}

	_, _ = fmt.Fprintf(r.stderrOrDefault(),
		"ass-guard: image content dropped: provider %T does not support images "+
			"(D-11) — the turn proceeds with text only\n", sess.Provider)

	// 21-REVIEW WR-07: an image-ONLY prompt would leave the outgoing user
	// message EMPTY (the shaper's byte-compat path emits NewTextBlock("") —
	// an Anthropic 400, a dead turn). The in-band placeholder guarantees a
	// text-bearing body.
	return ensureTextBearingBlock(out, imgDropClassUnsupported)
}

// The in-band drop-note classes (21-REVIEW WR-07): fixed-form outcome
// classes in the D-10/D-11 note family — never the bytes, never per-path
// detail.
const (
	imgDropClassUnsupported = "provider does not support images"
	imgDropClassIngress     = "image failed ingress validation"
)

// ensureTextBearingBlock guarantees blocks still carries a text-bearing
// block after image drops (21-REVIEW WR-07): when the drop left no non-empty
// text block (the image-only-prompt corner), ONE fixed-form placeholder text
// block is appended so the outgoing user message is never empty. The class
// names the drop's outcome — the in-band twin of the stderr note.
func ensureTextBearingBlock(blocks []session.ContentBlock, class string) []session.ContentBlock {
	for _, b := range blocks {
		if b.Type == blockText && b.Text != "" {
			return blocks
		}
	}

	return append(blocks, session.ContentBlock{
		Type: blockText,
		Text: "[image content could not be delivered: " + class + "]",
	})
}

// ingressImages is the image ingress tier (21-05, PAR-06/D-09): every image
// content block arriving on the prompt is validated BEFORE the transcript
// append — base64 decoded, DecodeConfig-first validated (the pixel-bomb
// guard), auto-downscaled when over the provider limits (pure Go), the
// ORIGINAL bytes persisted sha-keyed under the session .ass-guard images dir
// (plus the dims-suffixed scaled derivative when scaling ran; temp+rename
// atomic writes, idempotent on re-ingress) — and the block reaching the
// transcript carries ONLY Ref + metadata + resize provenance (the 09-05
// metadata-in-line discipline; the base64 never enters a line).
//
// Every failure (decode, limits, persistence) drops the block with ONE loud
// fixed-form stderr note naming the outcome class — never the bytes, never a
// dead turn (the D-10 note family; the provider-capability drop is Task 2's
// D-11 leg). Blocks without image payloads pass through untouched
// (byte-identical).
//
//nolint:funlen,cyclop // one ingress pipeline; grouped with the turn seam
func (r *Runner) ingressImages(
	sess *session.Session, blocks []session.ContentBlock,
) []session.ContentBlock {
	hasImage := false

	for _, b := range blocks {
		if b.Type == blockImage && b.Data != "" {
			hasImage = true

			break
		}
	}

	if !hasImage {
		return blocks
	}

	imagesDir := r.sessionImagesDir(sess)
	out := make([]session.ContentBlock, 0, len(blocks))

	dropped := false

	for _, b := range blocks {
		if b.Type != blockImage || b.Data == "" {
			out = append(out, b)

			continue
		}

		raw, derr := base64.StdEncoding.DecodeString(b.Data)
		if derr != nil {
			r.imageDropNote("image block dropped at ingress: undecodable base64 payload")
			dropped = true

			continue
		}

		scaled, media, w, h, origW, origH, didScale, verr :=
			ValidateAndScaleImage(raw, b.MediaType, r.imageLimitsFor())
		if verr != nil {
			r.imageDropNote("image block dropped at ingress: " + verr.Error())
			dropped = true

			continue
		}

		// Originals ALWAYS persist (D-09); the scaled derivative lands beside
		// them when downscaling ran. Sha-keyed: re-ingress converges on the
		// identical name + bytes.
		sum := sha256.Sum256(raw)

		origMedia := sourceMediaOf(raw)
		if origMedia == "" {
			origMedia = media
		}

		origRef := imageRefFor(imagesDir, sum, origMedia, 0, 0, false)
		if perr := writeImageAtomic(imagesDir, origRef, raw); perr != nil {
			r.imageDropNote("image block dropped at ingress: original persistence failed: " + perr.Error())
			dropped = true

			continue
		}

		ref := origRef
		if didScale {
			ref = imageRefFor(imagesDir, sum, media, w, h, true)
			if perr := writeImageAtomic(imagesDir, ref, scaled); perr != nil {
				r.imageDropNote("image block dropped at ingress: scaled persistence failed: " + perr.Error())
				dropped = true

				continue
			}
		}

		out = append(out, session.ContentBlock{
			Type:       blockImage,
			DataRef:    ref,
			MediaType:  media,
			Width:      w,
			Height:     h,
			OrigWidth:  origW,
			OrigHeight: origH,
			OrigSize:   int64(len(raw)),
			Scaled:     didScale,
		})
	}

	// 21-REVIEW WR-07: a drop that emptied the prompt (image-only input whose
	// payload failed ingress) must still leave a text-bearing body — never an
	// empty user message on the wire.
	if dropped {
		out = ensureTextBearingBlock(out, imgDropClassIngress)
	}

	return out
}

// imageDropNote emits ONE fixed-form ingress-degrade note on stderr (the
// D-10/D-11 note family: the outcome class is named, never the bytes).
func (r *Runner) imageDropNote(msg string) {
	_, _ = fmt.Fprintln(r.stderrOrDefault(), "ass-guard: "+msg)
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

// CloseSession closes one session's MCP host AND evicts it from the session
// cache (the logout/close/delete reap path — Plan 05-01 T4, widened by review
// WR-03). It satisfies acp.SessionCloser; the ACP server calls it via type
// assertion when handling logout and the session/close/delete sequences. The
// closed Session's resources (MCP host, TranscriptWriter ctx, session
// forwarder, SessionEnd hook) are unrecoverable — the reap chain runs exactly
// once — so a later sessionFor of the same id must RECONSTRUCT a full session
// instead of adopting the half-reaped cache entry (the editor's
// close-then-restore flow on one connection; the same class as 16-REVIEW
// CR-01). session/cancel deliberately does NOT route here (16-REVIEW CR-01:
// ACP cancel is per-turn — a cancelled mid-turn chain is drained instead by
// runOneTurn's request-ctx watchdog). An unknown sessionID is a no-op.
func (r *Runner) CloseSession(sessionID string) error {
	// 13-00: logout reaches here (the ACP server's closeSessionIfPossible) —
	// drain the session's parked chains FIRST (no decision, no injection after
	// the session end; goroutines released — D-03 stays the only off-switch).
	r.cancelParkedChains(sessionID)

	if r.sessions == nil {
		return nil
	}

	// WR-03: evict BEFORE Close — the map must never hand out a session whose
	// reap is starting, and eviction-before-reap keeps a concurrent sessionFor
	// from joining a session mid-close (in-flight turns hold the pointer and
	// finish on the closing session, exactly as before).
	r.sessMu.Lock()
	s, ok := r.sessions[sessionID]
	if ok {
		delete(r.sessions, sessionID)
	}
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

// SetDefaultTurnModel stamps the model the wire carries when the editor has
// stamped nothing (16-REVIEW CR-02): the serve composition binds it as the
// ConfigSurface's blob-default hook, so an initialize _meta fill of tier/model
// reaches the wire exactly as it reaches the advertisement — chip==wire on the
// BLOB path (16-09 gap 4b pinned the layer path only; the un-stamped blob fill
// left the wire on the tier-resolved config model while the chip showed the
// fill — the operator-observed turn-001 divergence class).
//
// It writes the SAME effective-model slot ApplyTurnModel writes, under modelMu
// ("a distinct writer for a distinct semantics", not a second slot). Two
// deliberate differences from ApplyTurnModel:
//
//   - No live-session restamp: the blob lands during initialize — the
//     connection's first frame — before any session exists; sessions created
//     later pick the stamp up at construction (sessionFor).
//   - Precedence stays honest (D-12): the surface fires the hook only when a
//     fill MOVED the effective model, which fills-unset guarantees means no
//     operator layer sets the slot. An explicit editor write persists to a
//     layer (inerting any later fill) and overrides a blob default through the
//     normal ApplyTurnModel path.
func (r *Runner) SetDefaultTurnModel(model string) error {
	r.modelMu.Lock()
	r.effectiveModel = model
	r.modelMu.Unlock()

	return nil
}

// ApplyCompactionSettings applies an editor-driven compaction change to LIVE
// state (19-05/D-03 — the ApplyTurnModel discipline): the runner's effective
// slot updates under its mutex, then every live session's settings swap UNDER
// that session's turn mutex — a Set arriving mid-turn waits for the in-flight
// turn, so the very next pre-request check reads the new values (no torn
// read). Sessions created later pick the slot up at construction (sessionFor).
// The context limit is deliberately NOT a parameter: it stays resolved from
// the modelrouting capability table (the D-01 discretion item — a config
// override would fork the measurement baseline).
func (r *Runner) ApplyCompactionSettings(enabled bool, thresholdPct int) error {
	r.compactionMu.Lock()
	r.compactionEnabled = enabled
	r.compactionPct = thresholdPct
	r.compactionArmed = true
	r.compactionMu.Unlock()

	limit := r.compactionContextLimit()

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

		sessions[id].SetCompactionSettings(enabled, thresholdPct, limit)

		mu.Unlock()
	}

	return nil
}

// SetEmitter injects the server-driven-turn chunk emitter (WINDOWS #3:
// strictly between server construction and scheduler start).
func (r *Runner) SetEmitter(emit func(sessionID string) acp.ChunkEmitter) { r.emitFor = emit }

// SetPermissionAskFire injects the permission-ask surface callback (17-02,
// ACP-01): the serve composition binds the acpserve surface's Fire
// (registry-backed session/request_permission) after the acp Server exists —
// internal/session never imports internal/acp (the onSurface-callback
// precedent). The gate enqueues through it; nil until injected (an unwired
// surface fails gated asks safe with a decline — never a silent allow).
func (r *Runner) SetPermissionAskFire(f func(ctx context.Context, e *session.AskEntry) session.AskOutcome) {
	r.permAskFire = f
}

// SetAskFire injects the elicitation-ask surface callback (17-04, ACP-02/
// D-09): the serve composition binds the acpserve ElicitationAsk's Fire
// (capability-gated elicitation/create with the plain-text fallback inside)
// after the acp Server exists. nil = unwired — every question-family ask
// keeps today's direct plain-text publish verbatim.
func (r *Runner) SetAskFire(f func(ctx context.Context, e *session.AskEntry) session.AskOutcome) {
	r.askFire = f
}

// SetBackgroundCaps injects the D-12 cap resolver (22-02): the serve
// composition binds the config surface's EffectiveBackgroundCaps (project >
// global > the 8/16 defaults) after the surface exists; every sessionFor
// construction reads the pair into the TaskRegistry (bash cap) and the
// tasks.Tracker (subagent cap). nil = unwired — the documented defaults
// (apply-as-landed: no mid-session cap mutation).
func (r *Runner) SetBackgroundCaps(f func() (subagents, bash int)) {
	r.backgroundCaps = f
}

// SetSandboxHandle stores the 22-06 startup probe's resolved Handle (SAND-01):
// the serve composition resolves the availability ONCE (off short-circuits
// with zero probing; enabled-but-unavailable warns exactly once) before the
// scheduler starts; every sessionFor construction reads the Handle into
// coreexec.Config and PTYOpts (the askFire late-injection precedent — nil or
// Mode "off" leaves every exec path untouched, the locked default).
func (r *Runner) SetSandboxHandle(h *sandbox.Handle) {
	r.sandboxHandle = h
}

// PublishAskChunk publishes one client-visible ask-surface chunk (17-04):
// the plain-text fallback's delivery seam and the queue-note emitter's sink —
// the same bus shape the AskBroker onSurface callback has always used.
// Subscriber-backed at every call site: the queue note rides the
// session-lifetime forwarder composition (17-03), and the post-turn
// engine-ask note publishes through the same always-subscribed path (the
// 13-03 timing hazard dead by construction).
func (r *Runner) PublishAskChunk(turnID, text string) {
	r.bus.Publish(event.AgentMessageChunk{
		TurnID: turnID, MessageID: turnID, Content: text,
	})
}

// publishAskQueueNote is the ask queue's note emitter (17-03/D-12 + 23-02/
// D-05): the pinned count note fires first, verbatim; the D-05 parked-ask
// wording follows one frame later on the same subscriber-backed path. The
// queue invokes the emitter ONLY when an enqueue PARKS (queues behind the
// one outstanding dialog) — immediate fires emit nothing, so the degraded
// plain-text fallback and every other immediate-fire wire shape stay
// byte-identical (TestPermissionsE2E stage 3's pin).
func (r *Runner) publishAskQueueNote(e *session.AskEntry, note string) {
	r.PublishAskChunk(e.TurnID, note)

	summary := e.Title
	if summary == "" {
		summary = "question"
	}

	r.PublishAskChunk(e.TurnID, "ask waiting behind running turn: "+summary)
}

// enqueueEngineAsk converts one engine ask-pending decision into a
// background-class elicitation queue entry (17-04, D-09/D-11): an accept
// writes the learning-store entry through the EXISTING store API (A9 —
// RecordCandidate, the ask-once-remember candidate write); decline/cancel (or
// a degraded surface) render today's advisory non-answer note. The note is
// emitted subscriber-backed (PublishAskChunk → the session-lifetime
// forwarder) — never a bare post-turn publish.
func (r *Runner) enqueueEngineAsk( //nolint:funcorder // the 17-04 engine-ask conversion group
	sess *session.Session, turnID, situation string,
) {
	if r.askFire == nil {
		// Surface unwired (stub runners, tests): today's advisory note.
		r.PublishAskChunk(turnID, advisoryNoteText)

		return
	}

	sess.EnqueueEngineAsk(turnID, situation, r.askFire,
		func(answer string) {
			if r.learned == nil {
				return
			}

			//nolint:noinlineerr // one-shot persist: log-and-continue, never a dead landing
			if err := r.learned.RecordCandidate(situation, answer, turnID); err != nil {
				log.Printf("ass-guard: engine ask answer persist failed (situation %s): %v",
					situation, err)
			}
		},
		func() { r.PublishAskChunk(turnID, advisoryNoteText) })
}

// SetPermMode flips the LIVE permission-mode accessor (17-02 Task 3: the
// permissions.mode apply target — persist-then-apply, 16-D-07 ordering). The
// gate reads the accessor per call, so the very next tool call in any live
// session sees the new mode (Pitfall 8 — no session recreation).
func (r *Runner) SetPermMode(mode string) { r.permMode.Store(mode) }

// PermMode returns the live permission mode — the gate's per-call read and
// the ConfigSurface's advertisement truth (chip==wire). The boot default is
// ungated: zero dialogs until the operator opts in (criterion 4).
func (r *Runner) PermMode() string {
	if m, ok := r.permMode.Load().(string); ok && m != "" {
		return m
	}

	return session.PermModeUngated
}

// StartScheduler launches the due-check goroutine (the serve composition's
// post-emitter step — see cron_wiring.go).
func (r *Runner) StartScheduler(ctx context.Context) { r.startScheduler(ctx) }

// DrainAsks drains one session's ask queue — the session/cancel teardown
// (17-03, D-13/T-17-09; the acp.AskDrainer capability): the OPEN permission
// dialog resolves cancelled through the queue-owned fire-ctx cancellation (the
// registry cascades $/cancel_request) and queued-but-unfired asks drain
// cancelled-normal immediately. THE one shared drain, reached from all three
// teardown paths (cancel notification, logout, serve shutdown).
func (r *Runner) DrainAsks(sessionID string) {
	if r.sessions == nil {
		return
	}

	r.sessMu.Lock()
	s, ok := r.sessions[sessionID]
	r.sessMu.Unlock()

	if ok {
		s.DrainPermissionAsks()
	}
}

// DrainSessionAsks is DrainAsks' session-close twin (logout): identical drain,
// distinct seam so the ACP layer can order drain-before-reap on both paths.
func (r *Runner) DrainSessionAsks(sessionID string) { r.DrainAsks(sessionID) }

// DrainAllAsks drains EVERY live session's ask queue (the serve-shutdown
// teardown — the acpserve ctx-done composition): no dialog outlives the serve
// lifetime (T-17-09).
func (r *Runner) DrainAllAsks() {
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
		s.DrainPermissionAsks()
	}
}

// CloseAllSessions closes every live session at serve end (the ctx-done
// subprocess reap).
func (r *Runner) CloseAllSessions() { r.closeAllSessions() }

// nextPromptFor resolves the pattern table's next-command field (08-06
// chaining) DYNAMICALLY through the capability interface so the table remains
// the single source of truth — tests may swap it after setup and the per-turn
// dispatchers follow.
func (r *Runner) nextPromptFor(patternID string) string {
	if np, ok := r.patternTable.(patternNextPrompter); ok {
		return np.NextPromptFor(patternID)
	}

	return ""
}

// effectiveModelFor returns the editor-stamped effective model ("" = none —
// the tier-resolved config default governs, see defaultTurnModel).
func (r *Runner) effectiveModelFor() string {
	r.modelMu.Lock()
	defer r.modelMu.Unlock()

	return r.effectiveModel
}

// defaultTurnModel returns the tier-resolved config default for the wire
// model (16-09 gap 4b — chip==wire): the SAME resolver-then-static-binding
// ladder the advertisement's resolveModelLocked applies, so with no editor
// stamp the model on the wire equals the advertised value (D-11). Ladder:
// schedCfg == nil → "" (the documented profile-slug default of test runners);
// the session tier, empty → the literal "heavy" (modelrouting's load floor —
// loader-produced schedCfg never has an empty tier; the literal covers
// hand-built schedCfg, and modelrouting's tierHeavy is unexported); the
// resolver's primary; else the tier's static binding; else "".
//
// The default never rewires the live provider: it rides the same heavy-tier
// resolution that picked the session provider (14-05), and cross-provider
// targets keep the loud-degrade guards (the resolveSubagentModel precedent).
func (r *Runner) defaultTurnModel() string {
	if r.schedCfg == nil {
		return ""
	}

	tier := r.schedCfg.SessionTier
	if tier == "" {
		tier = "heavy"
	}

	tgt, _, err := modelrouting.NewResolver(r.schedCfg).Resolve(tier, "", time.Now(), modelrouting.CapabilityReq{})
	if err == nil && tgt.Model != "" {
		return tgt.Model
	}

	if b, ok := r.schedCfg.Tiers[tier]; ok {
		return b.Model
	}

	return ""
}

// compactionState is the runner-level effective compaction pair session
// construction and the surface's apply target read (19-05/D-03). armed=false
// means no scheduling config loaded — sessions then keep 19-04's disabled
// zero value.
type compactionState struct {
	enabled bool
	pct     int
	armed   bool
}

// effectiveCompaction returns the runner's effective compaction state
// (19-05/D-03), lazily seeded from schedCfg on first use (NewRunner stays
// struct-fill-only).
func (r *Runner) effectiveCompaction() compactionState {
	r.compactionMu.Lock()
	defer r.compactionMu.Unlock()

	if !r.compactionArmed && r.schedCfg != nil {
		r.compactionEnabled = r.schedCfg.Compaction.Enabled
		r.compactionPct = r.schedCfg.Compaction.ThresholdPct
		r.compactionArmed = true
	}

	return compactionState{
		enabled: r.compactionEnabled, pct: r.compactionPct, armed: r.compactionArmed,
	}
}

// compactionContextLimit resolves the D-01 discretion item: the context window
// of the session's resolved model from the modelrouting capability table —
// the one and only source (there is deliberately no override key). Uses the
// same effective-model ladder sessionFor stamps profiles with. 0 when
// unresolvable (the threshold check never fires on a non-positive limit).
func (r *Runner) compactionContextLimit() int64 {
	if r.schedCfg == nil {
		return 0
	}

	m := r.effectiveModelFor()
	if m == "" {
		m = r.defaultTurnModel()
	}

	mc, ok := r.schedCfg.Models[m]
	if !ok {
		return 0
	}

	return int64(mc.Capabilities.ContextWindow)
}

// advisoryNote is one collected advisory decision's client-note projection.
type advisoryNote struct {
	turnID string
	class  string
	text   string
	// askSignal is non-empty when the last captured decision was an ENGINE
	// ask-pending (action "ask" whose signal is not the suspended marker —
	// that one is the broker's). 17-04's family conversion turns it into a
	// background elicitation queue entry instead of the advisory note.
	askSignal string
}

// advisoryFromDecision projects one engine decision into its client-note
// shape (17-04): the ask-pending surface (action "ask", signal not the
// suspended marker — that one is the broker's) becomes an engine-ask note;
// an advisory-class signal becomes the advisory note; everything else nil.
func advisoryFromDecision(
	d event.EngineDecision, //nolint:gocritic // hugeParam: value projection
) *advisoryNote {
	if d.Action == engine.ActionAsk.String() && d.Signal != engine.SignalAskSuspended {
		// 17-04 (D-09): the engine ask-pending surface.
		return &advisoryNote{turnID: d.TurnID, askSignal: d.Signal}
	}

	if class, ok := strings.CutPrefix(d.Signal, engine.SignalAdvisory); ok {
		return &advisoryNote{
			turnID: d.TurnID,
			class:  class,
			text:   advisoryNoteText,
		}
	}

	return nil
}

// collectAdvisory drains EngineDecision events until promptDone, capturing
// the LAST advisory-signal decision (the note rides its turn id).
func (r *Runner) collectAdvisory( //nolint:gocognit // one drain loop, two drain phases
	advCh <-chan event.Event, promptDone <-chan struct{}, advDone chan<- *advisoryNote,
) {
	defer r.bus.Unsubscribe("EngineDecision", advCh)

	var last *advisoryNote

	for {
		select {
		case e, ok := <-advCh:
			if !ok {
				advDone <- last

				return
			}

			if d, isDec := e.(event.EngineDecision); isDec {
				if note := advisoryFromDecision(d); note != nil {
					last = note
				}
			}
		case <-promptDone:
			// Drain any buffered decisions, then hand the last advisory over.
			for {
				select {
				case e := <-advCh:
					if d, isDec := e.(event.EngineDecision); isDec {
						if note := advisoryFromDecision(d); note != nil {
							last = note
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

	// 23-02 (SEEDG-01, D-06): the parked-cancel grammar — parsed BEFORE
	// ordinary reply interpretation, so a cancel-shaped input never becomes
	// an answer and an answer-shaped input never becomes a cancel (Pitfall
	// 11). Resolution is cancelled-normal through the existing settle path
	// (the D-01 timer's non-answer form — 17-D-13 drain semantics applied
	// proactively); the RUNNING turn, if any, is untouched.
	if isParkedCancel(blocks[idx].Text) {
		stop, cerr := sess.CancelPendingAsk(ctx)
		if cerr != nil {
			return "", false // the timer/reply won the claim race — an ordinary turn
		}

		return mapAskStop(stop), true
	}

	stop, rerr := sess.ResolveAsk(ctx, blocks[idx].Text)
	if rerr != nil {
		return "", false // the D-01 timer won the race — an ordinary turn
	}

	return mapAskStop(stop), true
}

// parkedCancelPhrases is the parked-ask cancel grammar's accepted vocabulary
// (23-02, D-06 — CONTEXT discretion, conservative and exact-phrase): the
// single place the grammar is defined. Matching is exact after whitespace
// trimming and case-folding — never substring (an answer containing the word
// "cancel" must still route as an answer).
var parkedCancelPhrases = []string{ //nolint:gochecknoglobals // immutable grammar table
	"cancel ask",
	"cancel the ask",
	"cancel question",
	"cancel the question",
	"dismiss ask",
	"dismiss the ask",
	"never mind",
	"nevermind",
}

// isParkedCancel reports whether an input matches the parked-cancel grammar.
func isParkedCancel(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))

	for _, p := range parkedCancelPhrases {
		if trimmed == p {
			return true
		}
	}

	return false
}

// tryLocalCommand is the class-B intercept (20-01/CMDS-02, D-05): parses the
// invocation ONCE on the first text block (the invocationFor single-parse
// discipline), resolves through the live chain, and — when the winner is a
// builtin with a LIVE handler — answers the turn entirely on the control
// plane: the typed command echoes as a user_message_chunk (its OWN
// messageId, derived from the turn id + ":echo" — a change in messageId
// starts a new client message), the handler's output streams as
// agent_message_chunk(s) under the turn's normal message id, the durable
// 16-D-22 local_command line lands through the REDACTED append path, and
// the turn ends end_turn. Zero provider calls, zero engine involvement.
// Every other outcome (no invocation, unknown name, dormant reserved name,
// class-A, skill/agent/file winner) returns handled=false and the existing
// turn flow proceeds unchanged. A handler panic is recovered and recorded as
// a failed local_command outcome with a loud stderr warning — a control-plane
// command NEVER fails the session (T-20-02).
func (r *Runner) tryLocalCommand(
	ctx context.Context, sess *session.Session, emit acp.ChunkEmitter, blocks []session.ContentBlock,
) (string, bool) {
	idx := firstTextBlockIndex(blocks)
	if idx < 0 || blocks[idx].Text == "" {
		return "", false
	}

	key, args, ok := ecosys.ParseInvocation(blocks[idx].Text)
	if !ok {
		return "", false // ordinary prose — never an invocation error
	}

	r.chainResolves.Add(1)

	entry, found := r.commandChainRef().resolve(key)
	if !found {
		return "", false // unknown name: plain text, never an error
	}

	turnID := sess.MintLocalCommandTurnID()

	// 20-04 (D-03/SKLS-02): an AGENT winner dispatches the subagent — the
	// typed args become the subagent prompt, the discovered def's
	// Prompt/Tools apply through the PARA machinery (20-03's model resolution
	// rides DispatchSubagent's planner), the subagent's bus chunks stream
	// through THIS turn's already-subscribed forwarder, and the turn ends
	// end_turn on completion. Zero parent-model turns.
	if entry.kind == chainKindAgent && entry.agent != nil {
		return r.dispatchAgentSlash(ctx, sess, emit, turnID, key, args, *entry.agent)
	}

	// 20-04 (SKLS-01): a skill whose on-disk body is EMPTY must never become
	// an empty model prompt — the loud D-05 error shape + failed record.
	if entry.kind == chainKindSkill && r.skillBodyEmpty(key) {
		r.emitClassBEcho(emit, turnID, blocks[idx].Text)

		msg := fmt.Sprintf("skill %q has an empty body — nothing to expand (skill key: %s)\n", key, entry.key)

		if err := emit.AgentMessageChunk(turnID, msg); err != nil {
			log.Printf("ass-guard: empty-skill output enqueue failed (continuing): %v", err)
		}

		if sess.Manager != nil {
			lerr := sess.Manager.AppendLocalCommand(
				turnID, key, args, "failed: empty skill body", []string{chainKindSkill})

			if lerr != nil {
				log.Printf("ass-guard: empty-skill record write failed (continuing): %v", lerr)
			}
		}

		return stopEndTurn, true
	}

	if entry.kind != chainKindBuiltin || entry.handler == nil {
		// Dormant reserved name / class-A / skill / file winner: the existing
		// flow (expansion path, engine, plain text) owns it.
		return "", false
	}

	// D-05 echo: through the in-hand emitter handle (foreground lane — the
	// same handle the turn's chunks ride; NOT a side channel). A plain
	// ChunkEmitter (legacy fakes) silently skips the echo — the ActivityEmitter
	// assertion precedent.
	r.emitClassBEcho(emit, turnID, blocks[idx].Text)

	outcome := "ok"
	output, outcomeOverride := r.runBuiltinHandler(ctx, entry, sess, turnID, args)
	if outcomeOverride != "" {
		outcome = outcomeOverride
	}

	// The output rides as agent_message_chunk(s) under the turn's normal
	// message id — the emit handle, best-effort (the durable record below is
	// the source of truth).
	if err := emit.AgentMessageChunk(turnID, output); err != nil {
		log.Printf("ass-guard: class-B output enqueue failed (continuing): %v", err)
	}

	if sess.Manager != nil {
		lerr := sess.Manager.AppendLocalCommand(
			turnID, key, args, outcome, []string{sourceChainBuiltin})

		if lerr != nil {
			log.Printf("ass-guard: local_command record write failed (continuing): %v", lerr)
		}
	}

	return stopEndTurn, true
}

// runBuiltinHandler executes one class-B handler inside the panic-recovery
// envelope (T-20-02): a panicking handler records the failed outcome and a
// loud stderr warning, and the turn STILL ends end_turn — a control-plane
// command never wedges or fails the session.
func (r *Runner) runBuiltinHandler(
	ctx context.Context, entry chainEntry, sess *session.Session, turnID, args string,
) (output, outcome string) {
	defer func() {
		if rec := recover(); rec != nil {
			outcome = fmt.Sprintf("failed: handler panic: %v", rec)
			output = "command failed (see stderr)"

			_, _ = fmt.Fprintf(r.stderrOrDefault(),
				"ass-guard: builtin /%s handler panic (recorded as a failed local_command; session continues): %v\n",
				entry.name, rec)
		}
	}()

	return entry.handler(ctx, r, sess, turnID, args)
}

// echoIDSuffix decorates the class-B echo's messageId so it can never collide
// with the output chunks' turn id (two client messages, D-05).
const echoIDSuffix = ":echo"

// sourceChainBuiltin is the local_command source-chain vocabulary's builtin
// element (16-D-22: resolution order builtin → skills → agents → file).
const sourceChainBuiltin = "builtin"

// toContentBlocks converts the ACP content blocks to session content blocks.
// 21-05 (PAR-06): image blocks map through with their Data (the pre-ingress
// base64 carrier — json:"-", never serialized) and declared MimeType; the
// ingress (ingressImages) validates + swaps them for Ref form BEFORE the
// transcript append.
func toContentBlocks(in []acp.ContentBlock) []session.ContentBlock {
	out := make([]session.ContentBlock, len(in))
	for i, b := range in {
		out[i] = session.ContentBlock{
			Type:      b.Type,
			Text:      b.Text,
			Data:      b.Data,
			MediaType: b.MimeType,
		}
	}

	return out
}

// patternNextPrompter is the optional pattern-table capability answering
// "what does the engine inject when this pattern matches" (08-06 chaining).
type patternNextPrompter interface {
	NextPromptFor(patternID string) string
}

// --- 20-03 (SKLS-03): dispatch-time per-agent model routing ---

// subagentModelNoteClass is the D-16 live note's advisory class (the
// advisoryNoteDue dedupe key — exactly one client-visible note per session).
const subagentModelNoteClass = "subagent-model"

// subagentModelDegradeClass is the D-15 degrade warnings' dedupe class.
const subagentModelDegradeClass = "subagent-model-degrade"

// planSubagentDispatch resolves ONE dispatch's routing (D-13 strict
// precedence, D-14 inherit parity, D-15 cross-provider routing):
//
//  1. frontmatter model (inherit normalizes to unset BEFORE lookup);
//  2. dispatch-time model (the call's parameter — effectively unused today);
//  3. session/parent model as-is (14-05's unconditional light-tier default
//     is REVERSED — unset agents run on the parent model);
//  4. tiers.light ONLY when the session has no model at all (the degraded
//     arm the 14-05 machinery still covers).
//
// A non-empty slug resolves through modelrouting: same-provider targets stamp
// the slug; CROSS-provider targets route through a second factory-built
// provider cached per (provider, session) on the Runner. EVERY failure mode
// (unknown slug, uncredentialed provider) degrades to the parent model +
// exactly ONE loud warning naming the intent — a turn NEVER fails over
// routing (D-15). The plan carries the D-16 live note (deduped per class).
func (r *Runner) planSubagentDispatch(
	sess *session.Session, agentDef *ecosys.Agent, dispatchModel string,
) session.SubagentDispatchPlan {
	parent := ""
	if sess != nil {
		parent = sess.Profile.Model
	}

	slug := ""
	if agentDef != nil && agentDef.Model != "" && agentDef.Model != modelInherit {
		slug = agentDef.Model // D-13: frontmatter FIRST (D-14: inherit = unset)
	}

	if slug == "" {
		slug = dispatchModel // D-13 arm 2 (unused today — CC parity slot)
	}

	// Arms 3+4: no slug → parent as-is; tiers.light only on a model-less
	// session (the REVERSED 14-05 default).
	if slug == "" {
		if parent != "" {
			return r.notePlan(session.SubagentDispatchPlan{}, sess,
				fmt.Sprintf("subagent dispatches on the session model %s", parent))
		}

		if tier := resolveSubagentModel(r.schedCfg, r.providerName, time.Now(), r.stderrOrDefault()); tier != "" {
			return r.notePlan(session.SubagentDispatchPlan{Model: tier}, sess,
				fmt.Sprintf("no session model — subagent falls to tiers.light %s", tier))
		}

		return session.SubagentDispatchPlan{}
	}

	// A slug: resolve through the operator's declared models (buildTarget
	// semantics — a slug absent from cfg.Models is the D-15 degrade, never a
	// turn failure).
	target, err := r.resolveDeclaredModel(slug)
	if err != nil {
		return r.degradePlan(sess, slug, "not declared in the scheduling config: "+err.Error())
	}

	if target.Provider == r.providerName {
		return r.notePlan(session.SubagentDispatchPlan{Model: target.Model}, sess,
			fmt.Sprintf("subagent routed to %s (frontmatter)", target.Model))
	}

	// D-15: cross-provider ROUTES. Credentials resolved BEFORE construction —
	// the uncredentialed arm degrades to parent with one warning.
	prov, perr := r.subagentProviderFor(target.Provider, sess)
	if perr != nil {
		return r.degradePlan(sess, slug, perr.Error())
	}

	return r.notePlan(session.SubagentDispatchPlan{Model: target.Model, Provider: prov}, sess,
		fmt.Sprintf("subagent cross-routed to %s on provider %s", target.Model, target.Provider))
}

// modelInherit is the CC-parity keyword normalizing to unset (D-14);
// errNoSchedCfg is the planner's static no-config degrade reason.
const modelInherit = "inherit"

var errNoSchedCfg = errors.New("no scheduling config loaded")

// resolveDeclaredModel resolves a slug through the scheduling config's
// models table (buildTarget semantics; error text names the missing piece).
func (r *Runner) resolveDeclaredModel(slug string) (modelrouting.Target, error) {
	if r.schedCfg == nil {
		return modelrouting.Target{}, errNoSchedCfg // the degrade reason
	}

	mc, ok := r.schedCfg.Models[slug]
	if !ok {
		return modelrouting.Target{}, fmt.Errorf("model %q", slug) //nolint:err113 // degrade reason
	}

	prov, ok := r.schedCfg.Providers[mc.Provider]
	if !ok {
		return modelrouting.Target{},
			fmt.Errorf("provider %q for model %q", mc.Provider, slug) //nolint:err113 // degrade reason
	}

	return modelrouting.Target{
		Provider: mc.Provider, Model: slug, BaseURL: prov.BaseURL, Shape: prov.Shape,
	}, nil
}

// subagentProviderFor builds (or reuses the cached) second provider for
// cross-provider dispatch (D-15): construction goes through the factory's
// SINGLE seam (BuildWithCapturer) so capture/tracer parity holds; the cache
// key is (provider, session) — one construction per pair, lifecycle riding
// the session (the surface Phase 22 builds on). An uncredentialed provider
// errors here (the degrade trigger — checked BEFORE construction via the
// exported credential resolver).
//
//nolint:ireturn // the factory seam's own interface (BuildWithCapturer)
func (r *Runner) subagentProviderFor(providerName string, sess *session.Session) (provider.Provider, error) {
	if r.schedCfg == nil {
		return nil, errNoSchedCfg
	}

	pc, ok := r.schedCfg.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q not declared", providerName) //nolint:err113 // degrade reason
	}

	if cred := modelrouting.ResolveCredential(pc, providerName, ""); cred.Key == "" {
		return nil, fmt.Errorf("provider %q has no credential", providerName) //nolint:err113 // degrade reason
	}

	cacheKey := providerName
	if sess != nil {
		cacheKey += "/" + sess.SessionID
	}

	r.subagentProvMu.Lock()
	defer r.subagentProvMu.Unlock()

	if r.subagentProviders == nil {
		r.subagentProviders = make(map[string]provider.Provider)
	}

	if cached, ok := r.subagentProviders[cacheKey]; ok {
		return cached, nil
	}

	factory := modelrouting.NewProviderFactory(r.schedCfg, "", nil)

	built, err := factory.BuildWithCapturer(providerName, shaper.New(), nil)
	if err != nil {
		return nil, fmt.Errorf("factory build: %w", err)
	}

	r.subagentProviders[cacheKey] = built

	return built, nil
}

// degradePlan renders the D-15 degrade: parent model kept, exactly ONE loud
// stderr warning naming the intended slug + reason, one counter bump, and a
// deduped client-visible note — the dispatch itself SUCCEEDS on the parent.
func (r *Runner) degradePlan(sess *session.Session, slug, reason string) session.SubagentDispatchPlan {
	r.subagentDegrades.Add(1)

	warn := fmt.Sprintf(
		"ass-guard: agent model %q degraded to the parent model (%s) — the dispatch continues on it\n",
		slug, reason)

	_, _ = io.WriteString(r.stderrOrDefault(), warn)

	note := ""
	if r.advisoryNoteDue(sess.SessionID, subagentModelDegradeClass) {
		note = fmt.Sprintf("agent model %q unavailable (%s) — dispatching on the session model", slug, reason)
	}

	return session.SubagentDispatchPlan{Note: note}
}

// notePlan attaches the D-16 live note (deduped per class per session) to a
// resolved plan.
func (r *Runner) notePlan(
	plan session.SubagentDispatchPlan, sess *session.Session, text string,
) session.SubagentDispatchPlan {
	if sess != nil && r.advisoryNoteDue(sess.SessionID, subagentModelNoteClass) {
		plan.Note = text
	}

	return plan
}

// liveAgentLookup resolves a subagent_type through the runner's LIVE chain
// (20-03): the registry-swap-safe view — an agent file added after session
// construction is dispatchable the moment the chain rebuilds.
func (r *Runner) liveAgentLookup(name string) (ecosys.Agent, bool) {
	entry, ok := r.commandChainRef().resolve(name)
	if !ok || entry.kind != chainKindAgent || entry.agent == nil {
		return ecosys.Agent{}, false
	}

	return *entry.agent, true
}

// dispatchAgentSlash runs one /<agent-name> dispatch (20-04, D-03/SKLS-02):
// the D-05 echo, the subagent dispatch through the EXISTING PARA machinery
// (Prompt/Tools/model applied; the planner + ResolvedModel line come with
// DispatchSubagent from 20-03), the subagent's chunks streaming through the
// current turn's forwarder, and a durable local_command record naming the
// agent surface. A subagent ERROR surfaces as the turn's error output —
// never a second error channel, never a wedged turn.
func (r *Runner) dispatchAgentSlash(
	ctx context.Context, sess *session.Session, emit acp.ChunkEmitter,
	turnID, key, args string, agentDef ecosys.Agent,
) (string, bool) {
	r.emitClassBEcho(emit, turnID, "/"+key+" "+args)

	prompt := args
	if strings.TrimSpace(prompt) == "" {
		prompt = "Run the " + key + " agent on the current context."
	}

	result, err := sess.DispatchSubagent(ctx, turnID, agentSlashToolPrefix+key, prompt, &agentDef)

	outcome := "ok"
	output := result

	if err != nil {
		outcome = "failed: " + err.Error()
		output = fmt.Sprintf("agent %q failed: %v\n", key, err)
	}

	if serr := emit.AgentMessageChunk(turnID, output); serr != nil {
		log.Printf("ass-guard: agent-slash output enqueue failed (continuing): %v", serr)
	}

	if sess.Manager != nil {
		lerr := sess.Manager.AppendLocalCommand(
			turnID, key, args, outcome, []string{chainKindAgent})

		if lerr != nil {
			log.Printf("ass-guard: agent-slash record write failed (continuing): %v", lerr)
		}
	}

	return stopEndTurn, true
}

// agentSlashToolPrefix namespaces the /<agent-name> dispatch's toolCallID
// (distinguishes the slash surface from the Agent tool on the dispatch line).
const agentSlashToolPrefix = "slash:"

// emitClassBEcho renders the D-05 user_message_chunk echo through the
// in-hand handle (the ActivityEmitter assertion — plain fakes skip it).
func (r *Runner) emitClassBEcho(emit acp.ChunkEmitter, turnID, text string) {
	if ue, canEcho := emit.(acp.ActivityEmitter); canEcho {
		if eerr := ue.UserMessageChunk(turnID+echoIDSuffix, text); eerr != nil {
			log.Printf("ass-guard: class-B echo enqueue failed (continuing): %v", eerr)
		}
	}
}

// runSubagentWithProgress is the 22-03 background launcher's Run dep: the
// session's nested-loop runner under the background ctx, with a bus
// subscription teeing the subagent's streamed chunks (AgentMessageChunk and
// ToolCall events tagged with the subagent's turn id) into the task's
// output file as produced (Pitfall 9: mid-run Read is truthful). The
// subscription's lifetime is the run's.
//
//nolint:funlen // subscription setup + run + drain reads as one flow
func runSubagentWithProgress(
	bgCtx context.Context, sess *session.Session,
	req session.BackgroundDispatchRequest, bus *event.Bus, progress func(string),
) (string, error) {
	if sess == nil {
		return "", fmt.Errorf("background subagent: session unavailable")
	}

	if bus == nil || progress == nil {
		// Headless (test) runners: run without the progress tee.
		return req.Runner.Run(bgCtx, sess, req.SubagentTurnID, req.ParentTurnID,
			req.Prompt, req.Restricted, req.AgentDef, req.Plan)
	}

	chunks := bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	tools := bus.Subscribe("ToolCall", event.BufToolCall)

	defer func() {
		bus.Unsubscribe("AgentMessageChunk", chunks)
		bus.Unsubscribe("ToolCall", tools)
	}()

	// The tee goroutine: forwards this subagent's events to the file until
	// the run ends (the deferred unsubscribe closes the channels).
	teeDone := make(chan struct{})

	go func() {
		defer close(teeDone)

		for {
			select {
			case e, ok := <-chunks:
				if !ok {
					return
				}

				if c, isChunk := e.(event.AgentMessageChunk); isChunk && c.TurnID == req.SubagentTurnID && c.Content != "" {
					progress(c.Content)
				}
			case e, ok := <-tools:
				if !ok {
					return
				}

				if c, isTool := e.(event.ToolCall); isTool && c.TurnID == req.SubagentTurnID {
					progress(fmt.Sprintf("\n[tool: %s]\n", c.Name))
				}
			}
		}
	}()

	result, err := req.Runner.Run(bgCtx, sess, req.SubagentTurnID, req.ParentTurnID,
		req.Prompt, req.Restricted, req.AgentDef, req.Plan)

	// Drain the tee's in-flight events before returning (the unsubscribe
	// below closes the source channels; give the pump a beat to flush).
	bus.Unsubscribe("AgentMessageChunk", chunks)
	bus.Unsubscribe("ToolCall", tools)

	select {
	case <-teeDone:
	case <-time.After(500 * time.Millisecond): // bounded — never pin the loop
	}

	return result, err
}
