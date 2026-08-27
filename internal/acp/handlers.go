package acp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Djarvur/ass-guard-agent/internal/redact"
)

const asciiDelete = 0x40
const uuidVariantSet = 0x80
const variantMask = 0x3F
const versionMask = 0x0F

var errMissingSessionid = errors.New("session/prompt: missing sessionId")

// registerHandlers populates the method→handler map with the canonical ACP v1
// method set (VERIFIED-FACTS #3): initialize, session/new, session/prompt,
// session/cancel, session/load (no-op per D-09), logout, session/set_mode.
func (s *Server) registerHandlers() {
	s.handlers[methodInitialize] = s.handleInitialize
	s.handlers["session/new"] = s.handleSessionNew
	s.handlers["session/prompt"] = s.handleSessionPrompt
	s.handlers["session/cancel"] = s.handleSessionCancel
	s.handlers["session/load"] = s.handleSessionLoad
	s.handlers["logout"] = s.handleLogout
	s.handlers["session/set_mode"] = s.handleSessionSetMode
	s.handlers[methodCancelRequest] = s.handleCancelRequestNoOp
}

// initializeResponse is the initialize result (INITIALIZATION.md / VERIFIED-
// FACTS #3). The field name is `agentCapabilities` (NOT capabilities/serverInfo),
// protocolVersion is integer 1, and loadSession is false (D-09 — NO replay).
type initializeResponse struct {
	ProtocolVersion   int            `json:"protocolVersion"`   //nolint:tagliatelle // ACP wire field
	AgentCapabilities map[string]any `json:"agentCapabilities"` //nolint:tagliatelle // ACP wire field
	AgentInfo         map[string]any `json:"agentInfo"`         //nolint:tagliatelle // ACP wire field
	AuthMethods       []any          `json:"authMethods"`       //nolint:tagliatelle // ACP wire field
}

// handleInitialize negotiates the elicitation-form capability (D-13) and
// responds. Negotiation is advertisement-first: a clientCapabilities
// elicitation.form advertisement caches ok and skips the probe entirely (real
// Zed handshakes never see a probe — VERIFIED acp.rs:767-795). Without the
// advertisement, at most ONE probe is issued for the connection (sticky after
// either outcome — D-18), bounded by the D-14 ladder. initialize ALWAYS
// responds, degraded or not; agentCapabilities are unchanged by degradation
// (D-13: every surface knows to degrade — the response never withholds).
func (s *Server) handleInitialize(ctx context.Context, params json.RawMessage) (any, error) {
	if len(params) > 0 {
		var p struct {
			ClientCapabilities struct {
				Elicitation struct {
					Form *json.RawMessage `json:"form,omitempty"`
				} `json:"elicitation"`
			} `json:"clientCapabilities"` //nolint:tagliatelle // ACP wire field
		}

		// Tolerant parse (unknown shapes treated as absent — schema-tolerant
		// style): a parse miss just falls through to the probe path.
		unmarshalErr := json.Unmarshal(params, &p)
		if unmarshalErr == nil && p.ClientCapabilities.Elicitation.Form != nil {
			s.capabilities.set(capElicitationForm, CapabilityOK)
			s.log.Printf("capability %s: advertised by client (no probe)", capElicitationForm)

			return s.initializeResult(), nil
		}
	}

	// Sticky check (D-18): already negotiated for this connection — never
	// re-probe, no flapping.
	if state := s.capabilities.get(capElicitationForm); state != CapabilityUnknown {
		return s.initializeResult(), nil
	}

	s.capabilities.set(capElicitationForm, s.probeElicitationCapability(ctx))

	return s.initializeResult(), nil
}

// initializeResult builds the initialize response (integer protocolVersion 1,
// loadSession:false per D-09).
func (s *Server) initializeResult() initializeResponse {
	return initializeResponse{
		ProtocolVersion: 1,
		AgentCapabilities: map[string]any{
			"loadSession": false,
		},
		AgentInfo: map[string]any{
			"name":    "ass-guard",
			"version": "0",
		},
		AuthMethods: []any{},
	}
}

// probeFormMode is the elicitation mode value of the capability probe (v1
// ElicitationFormMode).
const probeFormMode = "form"

// elicitationProbeParams is the minimal schema-valid v1 Form probe payload
// (A3 / CONTEXT discretion): one short text element, static, carrying no
// session content, no paths, no config values (T-16-08). Field names verbatim
// from schema/v1 CreateElicitationRequest + ElicitationFormMode +
// ElicitationSchema.
type elicitationProbeParams struct {
	Message         string                     `json:"message"`
	Mode            string                     `json:"mode"`
	RequestedSchema elicitationRequestedSchema `json:"requestedSchema"` //nolint:tagliatelle // ACP wire field
}

type elicitationRequestedSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]elicitationProp `json:"properties"`
}

type elicitationProp struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// probeElicitationCapability issues the D-13 capability probe through the
// registry (FAST-CONTROL window — D-17). A result response caches ok; an
// error response (-32601 for pre-elicitation clients — the canonical degrade
// signal) caches degraded; an unresponsive client meets the D-14 ladder (one
// retry, then degraded fallback + counters + structured stderr lines). It runs
// inside the initialize request's own goroutine, so blocking is safe — the
// read loop keeps serving (per-request dispatch, no reader deadlock).
func (s *Server) probeElicitationCapability(ctx context.Context) CapabilityState {
	s.metrics.noteProbe()

	// Call owns the marshaling — the static struct passes directly.
	payload := elicitationProbeParams{
		Message: "ass-guard capability probe (no action needed)",
		Mode:    probeFormMode,
		RequestedSchema: elicitationRequestedSchema{
			Type: "object",
			Properties: map[string]elicitationProp{
				"probe": {Type: "string", Description: "Capability probe — any input is fine."},
			},
		},
	}

	msg, callErr := s.registry.Call(ctx, methodElicitationCreate, payload, TimeoutFastControl)

	switch {
	case callErr != nil && errors.Is(callErr, ErrRequestCancelled):
		s.metrics.noteProbeFallback()
		s.log.Printf("capability probe %s: cancelled — degrading (D-13/D-18)", capElicitationForm)

		return CapabilityDegraded
	case callErr != nil:
		var timeoutErr *RequestTimeoutError
		if errors.As(callErr, &timeoutErr) {
			// The D-14 ladder burned BOTH windows (initial + one retry).
			s.metrics.noteProbeTimeout()
			s.metrics.noteProbeTimeout()
			s.metrics.noteProbeFallback()
			s.log.Printf("capability probe %s: fallback after retry — degrading (D-14)", capElicitationForm)
		} else {
			s.metrics.noteProbeFallback()
			s.log.Printf("capability probe %s: %v — degrading", capElicitationForm, callErr)
		}

		return CapabilityDegraded
	case msg.Error != nil:
		s.metrics.noteProbeFallback()
		s.log.Printf("capability probe %s: answered with jsonrpc error %d — degrading (D-13)",
			capElicitationForm, msg.Error.Code)

		return CapabilityDegraded
	default:
		s.log.Printf("capability probe %s: answered — ok", capElicitationForm)

		return CapabilityOK
	}
}

// sessionNewResult carries the sessionId the client threads into session/prompt.
type sessionNewResult struct {
	SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
}

// handleSessionNew creates a sessionState and returns its id.
func (s *Server) handleSessionNew(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		Cwd        string `json:"cwd"`
		McpServers []any  `json:"mcpServers"` //nolint:tagliatelle // ACP wire field
	}

	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}

	id := newSessionID()
	st := &sessionState{id: id}

	s.mu.Lock()
	s.sessions[id] = st
	s.mu.Unlock()

	return sessionNewResult{SessionID: id}, nil
}

// sessionPromptParams is the session/prompt payload (PROMPT-TURN.md): a
// sessionId + a prompt content array.
type sessionPromptParams struct {
	SessionID string         `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	Prompt    []ContentBlock `json:"prompt"`
}

// sessionPromptResult carries the stopReason ("end_turn", "cancelled", ...).
type sessionPromptResult struct {
	StopReason string `json:"stopReason"` //nolint:tagliatelle // ACP wire field
}

// handleSessionPrompt runs the turn: it looks up the session, derives a
// cancellable turn ctx (CancelFunc stored so session/cancel can abort it),
// streams chunks via the adapter, and returns the stopReason. The turn runner is
// the seam (stub in the tracer; real Session.Prompt in Plan 02-05).
func (s *Server) handleSessionPrompt(ctx context.Context, params json.RawMessage) (any, error) {
	var p sessionPromptParams

	err := json.Unmarshal(params, &p)
	if err != nil {
		return nil, fmt.Errorf("session/prompt params: %w", err)
	}

	if p.SessionID == "" {
		return nil, errMissingSessionid
	}

	s.mu.Lock()
	st, ok := s.sessions[p.SessionID]
	s.mu.Unlock()

	if !ok {
		//nolint:err113 // dynamic error message
		return nil, fmt.Errorf("session/prompt: unknown sessionId %q", p.SessionID)
	}

	turnCtx, cancel := context.WithCancel(ctx)

	st.setCancel(cancel)

	defer st.setCancel(nil)

	// 16-01: the turn streams through the FOREGROUND-class emitter handle —
	// its frames preempt any queued background traffic (D-01/D-02) and the
	// single drain owns the notification order.
	emit := s.Emitter(p.SessionID)

	stopReason, err := s.turnRunner.Run(turnCtx, p.SessionID, emit, p.Prompt)

	// Updates-before-response (16-01, Pitfall 4 — the verified cancel contract):
	// the turn's notifications ride the emitter's async lanes, so the handler
	// MUST wait for the drain to flush everything queued during the turn before
	// returning — the session/prompt response is written only after them. The
	// request ctx lets a cancelled turn skip the wait (the connection is dying).
	s.emitter.Barrier(ctx)

	if err != nil {
		// D-16: if the turn was cancelled, report stopReason "cancelled" rather
		// than a hard error (the client expects a stopReason after cancel).
		if errors.Is(turnCtx.Err(), context.Canceled) {
			return sessionPromptResult{StopReason: stopCancelled}, nil
		}

		return nil, fmt.Errorf("call: %w", err)
	}

	if stopReason == "" {
		stopReason = stopEndTurn
	}

	return sessionPromptResult{StopReason: stopReason}, nil
}

// handleSessionCancel cancels the active turn for the session (D-16 mechanism).
// It is a notification (no id, no response): it looks up the session's cancel
// func and calls it. The session/prompt handler observes the cancelled ctx,
// aborts the turn, drains queued events, and returns stopReason "cancelled".
func (s *Server) handleSessionCancel(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}

	if p.SessionID == "" {
		// No id to respond to anyway (notification); log and return.
		return nil, nil //nolint:nilnil // nil result signals "no JSON-RPC response" (notification / unknown session)
	}

	s.mu.Lock()
	st, ok := s.sessions[p.SessionID]
	s.mu.Unlock()

	if !ok {
		return nil, nil //nolint:nilnil // nil result signals "no JSON-RPC response" (notification / unknown session)
	}

	st.cancelTurn()
	// Plan 05-01 T4: reap the session's MCP host + other session-scoped
	// resources. The TurnRunner's SessionCloser (if implemented) drains the
	// subprocesses; cancelTurn already aborted the in-flight turn.
	s.closeSessionIfPossible(p.SessionID)

	return nil, nil //nolint:nilnil // nil result signals "no JSON-RPC response" (notification / unknown session)
}

// handleSessionLoad is a NO-OP per D-09 (NO replay in v1). It returns a -32601
// method-not-supported error; loadSession is advertised false in initialize.
// There is deliberately NO replay code path (Pitfall 6 — scope-creep guard).
func (s *Server) handleSessionLoad(ctx context.Context, params json.RawMessage) (any, error) {
	return nil, &RPCError{
		Code:    CodeMethodNotFound,
		Message: "session/load not supported (loadSession is false; replay is out of v1 scope — D-09)",
	}
}

// handleLogout drops the session. It accepts an optional sessionId param.
func (s *Server) handleLogout(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}

	if p.SessionID != "" {
		// Plan 05-01 T4: reap the session's MCP host before dropping it.
		s.closeSessionIfPossible(p.SessionID)
		s.mu.Lock()
		delete(s.sessions, p.SessionID)
		s.mu.Unlock()
	}

	return map[string]any{}, nil
}

// handleSessionSetMode accepts the mode-change notification. v1 has a single
// mode; this is a no-op that returns an empty result (forward-compatible with a
// future plan-mode / act-mode split).
func (s *Server) handleSessionSetMode(ctx context.Context, params json.RawMessage) (any, error) {
	_ = redact.ScrubError(nil) // keep redact import live for future scrubbing here

	return map[string]any{}, nil
}

// handleCancelRequestNoOp records a client-side cancellation of one of the
// CLIENT'S own in-flight requests (Assumption A8): ass-guard never cancels
// client-initiated requests mid-flight in v1.2, so the event is logged for
// diagnosis and produces no response (it is a notification — no frame by
// construction). The client's answer to OUR outbound asks is a -32800 error
// response, which the registry resolves — this handler is only for the
// client's own $/cancel_request notifications.
func (s *Server) handleCancelRequestNoOp(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		RequestID string `json:"requestId"` //nolint:tagliatelle // ACP wire field
	}

	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}

	s.log.Printf("inbound %s: requestId=%q (fast no-op — A8: no client-initiated cancels in v1.2)",
		methodCancelRequest, p.RequestID)

	return nil, nil //nolint:nilnil // nil result signals "no JSON-RPC response" (notification)
}

// uuidV4 returns a fresh random RFC 4122 v4 UUID string. It panics on a CSPRNG
// failure — ass-guard cannot run without a working entropy source (mirrors the
// internal/shaper helper). 16-02/D-15 generalizes the newSessionID
// construction: request ids are UUID v4 strings BOTH directions, and the same
// CSPRNG shape serves session, message, tool-call, and request ids (RESEARCH
// "Don't Hand-Roll" — collision-by-construction across restarts).
func uuidV4() string {
	var b [16]byte

	_, err := rand.Read(b[:])
	if err != nil {
		panic("crypto/rand failed: " + err.Error())
	}

	b[6] = (b[6] & versionMask) | asciiDelete
	b[8] = (b[8] & variantMask) | uuidVariantSet

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// newSessionID returns a fresh random session id (RFC 4122 v4 UUID shape via
// uuidV4; the thin wrapper keeps session-id generation one named call-site
// away from the general id generator).
func newSessionID() string {
	return uuidV4()
}
