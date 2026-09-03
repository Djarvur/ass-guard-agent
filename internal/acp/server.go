package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/Djarvur/ass-guard-agent/internal/redact"
)

// ChunkEmitter streams session/update notifications to the client during a turn.
// The adapter implements it; a TurnRunner calls AgentMessageChunk per chunk
// (ACP-04 streaming — NO full-turn buffering). The interface is the seam Plan
// 02-05 rewires to subscribe to the expanded event bus.
type ChunkEmitter interface {
	// AgentMessageChunk streams one text chunk as a session/update notification.
	AgentMessageChunk(messageID, text string) error
}

// TurnRunner runs one session/prompt turn. It streams chunks via emit and
// returns the ACP stopReason ("end_turn", "cancelled", ...). The tracer
// provides a stub; Plan 02-05 provides the real Session.Prompt-backed runner.
// Run MUST honor ctx cancellation (D-16 — session/cancel aborts the turn).
// sessionID identifies the ACP session (the runner creates/looks up the
// underlying Session Core by id).
type TurnRunner interface {
	Run(ctx context.Context, sessionID string, emit ChunkEmitter, prompt []ContentBlock) (stopReason string, err error)
}

// SessionCloser is an OPTIONAL capability a TurnRunner may implement to release
// session-scoped resources (Plan 05-01 T4). When the TurnRunner implements it,
// logout calls CloseSession(sessionID) so MCP subprocesses and other
// session-scoped resources are reaped (D-02). session/cancel deliberately does
// NOT route here (16-REVIEW CR-01): ACP cancel is per-turn, so cancelling must
// leave the session — and its resources — live for the next prompt. It is a
// separate interface (not part of TurnRunner) so stub runners need not
// implement it.
type SessionCloser interface {
	CloseSession(sessionID string) error
}

// SessionLoader is an OPTIONAL capability a TurnRunner may implement (18-01,
// ACP-06/D-01): the resume seam. When the TurnRunner implements it,
// session/load calls ResumeSession BEFORE replay so the runner adopts the
// transcript's session id and seeds its turn counter from the transcript
// maxima — the next turn id continues the on-disk sequence instead of
// restarting at 001 (Pitfall 2). It is a separate interface (not part of
// TurnRunner) so stub runners need not implement it — the same
// optional-capability shape as SessionCloser/AskDrainer.
type SessionLoader interface {
	ResumeSession(ctx context.Context, sessionID string) error
}

// ConfigSurface is the acp-side seam for the v1.2 editor-driven configuration
// surface (16-05/ACP-08). internal/acp owns only the wire shapes and the
// handler; the surface implementation (internal/acpserve) owns the menu
// semantics, the layer writes, and effective-value resolution — acp stays free
// of modelrouting/providerfactory imports (wire-only dependency direction).
// The exact shape is executor-frozen and kept minimal:
//
//   - Options returns the full advertised menu in v1 SessionConfigOption
//     shapes, each currentValue the option's current EFFECTIVE value (D-11).
//   - Set persists+applies one option (D-07 persist-then-apply lives in the
//     surface). It validates the id and value (D-09 typed rejects as
//     *ConfigViolationError), reports layer-write failures as
//     *ConfigPersistError, and answers applied/pending-no-op/idempotent
//     outcomes all with the refreshed FULL option set. sessionID scopes the
//     out-of-band config_option_update notification.
//   - ApplyBlobDefaults applies the initialize _meta blob (D-10): recognized
//     keys fill unset slots in-memory, unknown keys are retained verbatim;
//     changed reports whether any effective value moved.
type ConfigSurface interface {
	Options() []ConfigOptionFrame
	Set(sessionID, optionID string, value any) ([]ConfigOptionFrame, error)
	ApplyBlobDefaults(meta map[string]json.RawMessage) (changed bool, err error)
}

// Handler is one ACP method's handler. params is the raw JSON params; msg is the
// full envelope (so handlers can read the id). A non-nil error surfaces as a
// JSON-RPC error response; a nil error with a non-nil result surfaces as a
// success response. Notifications (msg.ID == nil) are dispatched but produce no
// response frame.
type Handler func(ctx context.Context, params json.RawMessage) (result any, err error)

// Server is the ACP v1 stdio server (D-15). One reader goroutine reads frames
// from stdin; requests are dispatched to handlers in per-request goroutines so
// the reader keeps reading (a session/prompt turn runs concurrently with the
// next frame read). Notifications (no id) are dispatched inline in the reader
// goroutine — session/cancel must cancel the active turn synchronously to avoid
// a race. All frame writes go through the mutex-guarded Writer so concurrent
// goroutines never interleave a line on stdout.
type Server struct {
	in           io.Reader
	out          *Writer
	log          *log.Logger
	handlers     map[string]Handler
	mu           sync.Mutex
	sessions     map[string]*sessionState
	turnRunner   TurnRunner
	configSurf   ConfigSurface     // 16-05/ACP-08 editor-driven configuration; nil = surface absent (degrade)
	emitter      *TurnEmitter      // THE ordered session/update notification path (16-01); non-nil after NewServer
	registry     *Registry         // outbound id'd requests + response matching (16-02); non-nil after NewServer
	metrics      *Metrics          // D-16 counters (probes/timeouts/fallbacks/cancels/stalls)
	capabilities *capabilityCache  // D-13/D-18 sticky capability negotiation cache; non-nil after NewServer
	emitCfg      TurnEmitterConfig // composition-root knobs via WithTurnEmitter
	registryCfg  RegistryConfig    // D-17 timeout windows via WithRegistryConfig (tests shrink FAST-CONTROL)
	handlerWG    sync.WaitGroup    // tracks in-flight request goroutines so Close is safe

	// workDir is the store root session/load resolves .ass-guard/ under
	// (18-01): the acpserve composition injects it via WithWorkDir so the load
	// handler stats tombstones and replays transcripts against the SERVER's
	// workspace — never the client-supplied cwd (T-18-02). "" means the
	// process cwd (acpserve's WorkDir resolution resolves eagerly; the empty
	// flag default degrades to cwd here).
	workDir string

	// loading tracks sessions mid-session/load (18-01/D-03): the id is
	// registered at load start and cleared on every exit path. A prompt
	// arriving for a loading id gets the typed replay-in-progress error — it
	// never interleaves with replayed frames (the map insert into s.sessions
	// is the LAST step of load).
	loading map[string]struct{}
}

// sessionState is one live session (created by session/new, or by session/load
// after replay completes). It carries the active turn's cancel func so
// session/cancel can abort the turn (D-16), and the D-03 ready flag gating
// prompt acceptance: a state exists before it is ready only inside the load
// path's final insert→flip window — session/new marks its states ready at
// construction (no replay to wait for).
type sessionState struct {
	id     string
	mu     sync.Mutex
	cancel context.CancelFunc
	ready  bool
}

// setReady flips the D-03 ready flag — the last step of session/load before
// the response is built (reconcile-then-accept).
func (s *sessionState) setReady() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ready = true
}

// isReady reports whether the session accepts prompts (D-03 gate).
func (s *sessionState) isReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.ready
}

func (s *sessionState) setCancel(c context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cancel = c
}

func (s *sessionState) cancelTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithTurnRunner installs the turn runner (default: a stub that returns
// "end_turn" with no chunks). Plan 02-05 installs the real Session.Prompt runner.
func WithTurnRunner(r TurnRunner) ServerOption {
	return func(s *Server) { s.turnRunner = r }
}

// WithLogger installs a stderr logger (default: a logger writing to the supplied
// stderr sink). All diagnostics go here, NEVER stdout.
func WithLogger(l *log.Logger) ServerOption {
	return func(s *Server) { s.log = l }
}

// WithTurnEmitter configures the composition-root TurnEmitter's knobs (lane
// capacities + stall threshold; zero values keep the documented defaults).
// The emitter itself is ALWAYS armed by NewServer so every session/update
// notification shares one ordered drain (16-01/D-02 single-writer total order).
func WithTurnEmitter(cfg TurnEmitterConfig) ServerOption {
	return func(s *Server) { s.emitCfg = cfg }
}

// WithRegistryConfig tunes the outbound-request registry's D-17 timeout
// windows (zero values keep the documented defaults; tests shrink FAST-CONTROL
// so the D-14 ladder runs in milliseconds).
func WithRegistryConfig(cfg RegistryConfig) ServerOption {
	return func(s *Server) { s.registryCfg = cfg }
}

// WithConfigSurface injects the editor-driven configuration surface (16-05/
// ACP-08). Absent (the default): advertisements omit configOptions entirely
// and session/set_config_option answers the typed not-available error — the
// degrade never crashes acpserve-less setups.
func WithConfigSurface(cs ConfigSurface) ServerOption {
	return func(s *Server) { s.configSurf = cs }
}

// WithWorkDir sets the store root the session/load path resolves .ass-guard/
// under (18-01): tombstone stats + transcript reads + the replay source all
// resolve against the SERVER's workspace, never the client-supplied cwd
// (T-18-02 — a crafted cwd cannot point replay at another directory's
// session). "" (the default) means the process cwd, matching acpserve's
// WorkDir resolution (which resolves the --work-dir flag eagerly and falls
// back to Getwd).
func WithWorkDir(dir string) ServerOption {
	return func(s *Server) { s.workDir = dir }
}

// NewServer builds a Server reading frames from in, writing frames to out, and
// diagnostics to stderrSink (must NEVER be stdout — transport discipline).
// Construction arms the TurnEmitter over the same Writer: notifications route
// through its lanes; responses and parse errors still use writeResult/
// writeError directly (response-vs-notification ordering is not spec-constrained
// except at turn end, which handleSessionPrompt's Barrier covers).
func NewServer(in io.Reader, out, stderrSink io.Writer, opts ...ServerOption) *Server {
	s := &Server{
		in:         in,
		out:        newWriter(out),
		log:        log.New(stderrSink, "ass-guard/acp: ", log.LstdFlags|log.Lshortfile),
		handlers:   map[string]Handler{},
		sessions:   map[string]*sessionState{},
		loading:    map[string]struct{}{},
		turnRunner: stubNoChunkRunner{},
	}
	for _, o := range opts {
		o(s)
	}

	s.metrics = &Metrics{}
	s.emitter = NewTurnEmitter(s.out, stderrSink, s.emitCfg)
	s.emitter.metrics = s.metrics // the D-16 family adopts 16-01's writer-stall counter

	// The registry (16-02) writes id'd request frames straight through the
	// Writer (unconstrained by notification ordering) and routes its D-19
	// synthetic-cancel cascade through the emitter's FOREGROUND lane so the
	// turn-end Barrier orders it before the prompt response.
	s.registry = NewRegistry(s.out, stderrSink,
		WithRegistryTimeouts(s.registryCfg),
		WithRegistryCascade(s.emitter.newHandle("", classForeground).Notify),
		WithRegistryOnCancel(s.metrics.noteRegistryCancel))

	s.capabilities = newCapabilityCache()

	s.registerHandlers()

	return s
}

// CapabilityState is one negotiated capability's sticky result (D-13/D-18).
// Tier note: the cache deliberately rides internal/acp beside the registry —
// the probe IS a registry Call issued inside handleInitialize, Server lifetime
// equals connection lifetime (exactly D-13/D-18's scope), and acpserve owns
// the Server via composition, so the state is acpserve-owned transitively.
type CapabilityState int

// Negotiated capability states (the zero value is "not yet negotiated").
const (
	CapabilityUnknown  CapabilityState = iota
	CapabilityOK                       // advertised by the client, or the probe was answered
	CapabilityDegraded                 // probe failed (-32601/cancelled/ladder) — sticky, no re-probe (D-18)
)

// capabilityCache is the D-13/D-18 sticky per-connection capability cache:
// negotiation happens ONCE (advertisement at initialize, else one probe) and
// a result — ok or degraded — STAYS for the connection lifetime. No flapping,
// no re-probe backoff; the NEXT connection probes fresh (the cache dies with
// the Server).
type capabilityCache struct {
	mu      sync.Mutex
	results map[string]CapabilityState
}

func newCapabilityCache() *capabilityCache {
	return &capabilityCache{results: map[string]CapabilityState{}}
}

func (c *capabilityCache) set(key string, state CapabilityState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.results[key] = state
}

func (c *capabilityCache) get(key string) CapabilityState {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.results[key]
}

// Capability reports the sticky negotiation result for one capability key.
// Later phases (Phase 17's permission/elicitation asks) consult this instead
// of ever re-probing (D-13/D-18).
func (s *Server) Capability(key string) CapabilityState {
	return s.capabilities.get(key)
}

// stubNoChunkRunner is the default turn runner: returns "end_turn" with no
// chunks. Replaced by WithTurnRunner in real wiring.
type stubNoChunkRunner struct{}

func (stubNoChunkRunner) Run(ctx context.Context, _ string, emit ChunkEmitter, prompt []ContentBlock) (string, error) {
	return stopEndTurn, nil
}

// Emitter returns a session-scoped FOREGROUND-class emitter handle over the
// Server's TurnEmitter (16-01): its frames preempt the queue head. Usable
// OUTSIDE a session/prompt request, it lets server-driven turns (timer resumes,
// automation firings — 12-07/WINDOWS #3) stream session/update notifications
// through the same single ordered drain every other notification uses.
// The returned value satisfies ChunkEmitter; type-assert to ActivityEmitter for
// the extended v1 frame vocabulary.
//
//nolint:ireturn // the emitter seam is intentionally the interface (tests inject fakes)
func (s *Server) Emitter(sessionID string) ChunkEmitter {
	return s.emitter.ForegroundHandle(sessionID)
}

// BackgroundEmitter returns a session-scoped BACKGROUND-class handle: frames
// enqueue FIFO behind any foreground preemption (subagent / engine /
// automation streams — D-01/D-02).
//
//nolint:ireturn // same seam rationale as Emitter
func (s *Server) BackgroundEmitter(sessionID string) ChunkEmitter {
	return s.emitter.BackgroundHandle(sessionID)
}

// TurnEmitter exposes the server's ordered notification emitter (16-01): the
// registry (16-02) and tests read its counters; the composition root may tune
// knobs only via WithTurnEmitter before traffic starts.
func (s *Server) TurnEmitter() *TurnEmitter {
	return s.emitter
}

// Registry exposes the outbound-request registry (16-02/16-03) — the 17-02
// composition seam: the acpserve ask surface dispatches
// session/request_permission through it (HUMAN-ASK class) and the acpserve
// Run composition binds the fire callback runner-side (internal/session stays
// free of internal/acp imports). Non-nil after NewServer.
func (s *Server) Registry() *Registry {
	return s.registry
}

// NotifyConfigOptions emits one config_option_update session/update
// notification through the emitter's FOREGROUND lane (16-05: out-of-band
// configuration changes — D-10's post-blob-application notification and
// applied editor sets). The frame carries the FULL refreshed option set per
// v1. An empty sessionID is legal on the only sessionless path (the
// initialize _meta blob precedes session creation on the connection): the
// frame still carries the refreshed set, and a sessionless client has no view
// to re-render. Best-effort: enqueue errors return (serve teardown races).
func (s *Server) NotifyConfigOptions(sessionID string, opts []ConfigOptionFrame) error {
	if opts == nil {
		opts = []ConfigOptionFrame{}
	}

	raw, err := json.Marshal(map[string]any{
		keySessionID: sessionID,
		"update": map[string]any{
			keySessionUpdate: KindConfigOptionUpdate,
			"configOptions":  opts,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal config_option_update: %w", err)
	}

	return s.emitter.newHandle(sessionID, classForeground).Notify(
		&Message{JSONRPC: protocolVersion20, Method: methodSessionUpdate, Params: raw})
}

// Serve runs the reader loop until ctx is cancelled or stdin reaches EOF. Each
// frame is dispatched: requests (with id) go to a per-request goroutine;
// notifications (no id) are handled inline. Parse errors surface a -32700
// response (asynchronously, via the Writer's drain) and the loop continues. On
// exit, Serve waits for in-flight handlers, stops the TurnEmitter (its drain
// must exit BEFORE the Writer closes — Pitfall 8), then closes the Writer so
// all buffered frames flush before the caller inspects stdout.
func (s *Server) Serve(ctx context.Context) error {
	defer func() {
		s.handlerWG.Wait()
		s.registry.Stop() // drain pending outbound requests BEFORE the Writer closes (Pitfall 8)
		s.emitter.Stop()
		s.out.Close()
	}()

	br := bufio.NewReader(s.in)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("ctx: %w", ctx.Err())
		default:
		}

		msg, err := readFrame(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			// Parse error: surface -32700 and keep reading. Only respond when we
			// can recover an id; otherwise log and continue.
			s.handleParseError(err)

			continue
		}

		// Response interception (16-02, RESEARCH Pattern 2 — Pitfall 1): a frame
		// with an id, NO method, and a result-or-error is a RESPONSE to one of
		// OUR outbound requests. Route it to the registry and produce NO reply —
		// dispatching it to the handler map would emit a spurious -32601
		// (protocol garbage the client may treat as a failure). The condition
		// captures neither notifications (no id) nor requests (method present).
		if msg.ID != nil && msg.Method == "" && (msg.Result != nil || msg.Error != nil) {
			s.registry.Deliver(msg)

			continue
		}

		if msg.ID == nil {
			// Notification: dispatch inline (session/cancel must cancel the
			// active turn synchronously). Recover per-dispatch so a panicking
			// notification handler never crashes the reader (Pitfall 7).
			s.safeDispatchInline(ctx, msg)

			continue
		}
		// Request: dispatch in a goroutine so the reader keeps reading.
		m := *msg // capture the envelope value

		s.handlerWG.Go(func() {
			defer s.recoverDispatch(&m)

			s.handleRequest(ctx, &m)
		})
	}
}

// closeSessionIfPossible type-asserts the TurnRunner to SessionCloser and calls
// CloseSession. A non-implementing runner is a no-op (backward-compatible).
// Plan 05-01 T4: logout reaches the session's MCP host via this seam so
// subprocesses are reaped on session end. Only logout calls it — session/cancel
// is turn-scoped and must not reap (16-REVIEW CR-01).
func (s *Server) closeSessionIfPossible(sessionID string) {
	closer, ok := s.turnRunner.(SessionCloser)
	if !ok {
		return
	}

	_ = closer.CloseSession(sessionID)
}

// storeWorkDir resolves the workspace the session/load path resolves
// .ass-guard/ under (18-01/T-18-02): WithWorkDir's composition-root value, or
// the process cwd when unset (acpserve's eager resolution leaves "" only for
// acpserve-less constructions — tests, hand-built servers).
func (s *Server) storeWorkDir() string {
	if s.workDir != "" {
		return s.workDir
	}

	wd, _ := os.Getwd() // no flag to resolve; cwd IS the documented default

	return wd
}

// beginLoading registers a session id as mid-session/load under s.mu (18-01/
// D-03). It returns false when a load is already in flight for the id — one
// load at a time per id; the caller surfaces the typed busy error.
func (s *Server) beginLoading(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, busy := s.loading[sessionID]; busy {
		return false
	}

	s.loading[sessionID] = struct{}{}

	return true
}

// endLoading clears the mid-load marker — EVERY load exit path calls it
// (rejections included: a failed load must not leave a ghost busy marker).
func (s *Server) endLoading(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.loading, sessionID)
}

// isLoading reports whether a session/load is in flight for the id (the
// prompt-gate consults it to name the replay state, D-03).
func (s *Server) isLoading(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.loading[sessionID]

	return ok
}

// handleRequest looks up the handler, calls it, and writes the response (result
// or error). Errors are scrubbed via redact.ScrubError before reaching the wire
// (T-02-03 — error responses never leak a credential).
func (s *Server) handleRequest(ctx context.Context, msg *Message) {
	handler, ok := s.handlers[msg.Method]
	if !ok {
		s.writeError(msg.ID, &RPCError{
			Code:    CodeMethodNotFound,
			Message: fmt.Sprintf("method %q not found", msg.Method),
		})

		return
	}

	result, err := handler(ctx, msg.Params)
	if err != nil {
		// A handler may return a *RPCError to surface a specific JSON-RPC code
		// (e.g. session/load's typed rejections, 18-01). Any other error is
		// scrubbed and wrapped as a generic -32603 internal error (T-02-03).
		rpcErr := &RPCError{}
		if errors.As(err, &rpcErr) {
			s.writeError(msg.ID, rpcErr)

			return
		}

		s.writeError(msg.ID, &RPCError{Code: CodeInternalError, Message: redact.ScrubError(err)})

		return
	}

	raw, err := json.Marshal(result)
	if err != nil {
		s.writeError(msg.ID, &RPCError{Code: CodeInternalError, Message: redact.ScrubError(err)})

		return
	}

	s.writeResult(msg.ID, raw)
}

// safeDispatchInline runs a notification handler with a recover guard so a
// panicking handler never crashes the reader.
func (s *Server) safeDispatchInline(ctx context.Context, msg *Message) {
	defer s.recoverDispatch(msg)

	handler, ok := s.handlers[msg.Method]
	if !ok {
		// Unknown notification: ignore (notifications get no response).
		return
	}

	_, _ = handler(ctx, msg.Params)
}

// recoverDispatch converts a panic in a handler/dispatch into a stderr log line
// (investigate-and-fix-ready). For requests, also surface a -32603 error so the
// client isn't left waiting. Never re-panics.
func (s *Server) recoverDispatch(msg *Message) {
	r := recover()
	if r == nil {
		return
	}

	s.log.Printf("panic dispatching method=%q id=%v: %v", msg.Method, msg.ID, r)

	if msg.ID != nil {
		s.writeError(msg.ID, &RPCError{Code: CodeInternalError, Message: "internal error (recovered)"})
	}
}

// handleParseError surfaces a -32700 parse error response when a frame cannot be
// decoded. Since the id is unrecoverable on a parse error, we respond with
// id:null per JSON-RPC convention and keep reading.
func (s *Server) handleParseError(err error) {
	s.log.Printf("frame parse error: %v", err)
	_ = s.out.Write(&Message{
		JSONRPC: protocolVersion20,
		ID:      nil,
		Error:   &RPCError{Code: CodeParseError, Message: "parse error"},
	})
}

// writeResult writes a success response with the given id.
func (s *Server) writeResult(id, result json.RawMessage) {
	_ = s.out.Write(&Message{JSONRPC: protocolVersion20, ID: id, Result: result})
}

// writeError writes an error response with the given id.
func (s *Server) writeError(id json.RawMessage, e *RPCError) {
	_ = s.out.Write(&Message{JSONRPC: protocolVersion20, ID: id, Error: e})
}
