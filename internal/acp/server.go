package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
// logout/session-cancel call CloseSession(sessionID) so MCP subprocesses and
// other session-scoped resources are reaped (D-02). It is a separate interface
// (not part of TurnRunner) so stub runners need not implement it.
type SessionCloser interface {
	CloseSession(sessionID string) error
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
	in         io.Reader
	out        *Writer
	log        *log.Logger
	handlers   map[string]Handler
	mu         sync.Mutex
	sessions   map[string]*sessionState
	turnRunner TurnRunner
	emitter    *TurnEmitter      // THE ordered session/update notification path (16-01); non-nil after NewServer
	registry   *Registry         // outbound id'd requests + response matching (16-02); non-nil after NewServer
	emitCfg    TurnEmitterConfig // composition-root knobs via WithTurnEmitter
	handlerWG  sync.WaitGroup    // tracks in-flight request goroutines so Close is safe
}

// sessionState is one live session (created by session/new). It carries the
// active turn's cancel func so session/cancel can abort the turn (D-16).
type sessionState struct {
	id     string
	mu     sync.Mutex
	cancel context.CancelFunc
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
		turnRunner: stubNoChunkRunner{},
	}
	for _, o := range opts {
		o(s)
	}

	s.emitter = NewTurnEmitter(s.out, stderrSink, s.emitCfg)

	// The registry (16-02) writes id'd request frames straight through the
	// Writer (unconstrained by notification ordering) and routes its D-19
	// synthetic-cancel cascade through the emitter's FOREGROUND lane so the
	// turn-end Barrier orders it before the prompt response.
	s.registry = NewRegistry(s.out, stderrSink, WithRegistryCascade(s.emitter.newHandle("", classForeground).Notify))

	s.registerHandlers()

	return s
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
// Plan 05-01 T4: logout/session-cancel reach the session's MCP host via this
// seam so subprocesses are reaped on session end.
func (s *Server) closeSessionIfPossible(sessionID string) {
	closer, ok := s.turnRunner.(SessionCloser)
	if !ok {
		return
	}

	_ = closer.CloseSession(sessionID)
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
		// (e.g. session/load's -32601 no-op, D-09). Any other error is scrubbed
		// and wrapped as a generic -32603 internal error (T-02-03).
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
