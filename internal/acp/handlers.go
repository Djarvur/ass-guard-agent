package acp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Djarvur/ass-guard-agent/internal/redact"
)

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

// handleInitialize echoes the protocol version and advertises loadSession:false.
// D-09: NO replay in v1, so loadSession is structurally false (session/load
// returns -32601).
func (s *Server) handleInitialize(ctx context.Context, params json.RawMessage) (any, error) {
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
	}, nil
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
		return nil, errors.New("session/prompt: missing sessionId")
	}

	s.mu.Lock()
	st, ok := s.sessions[p.SessionID]
	s.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("session/prompt: unknown sessionId %q", p.SessionID)
	}

	turnCtx, cancel := context.WithCancel(ctx)

	st.setCancel(cancel)

	defer st.setCancel(nil)

	emit := &adapter{out: s.out, sessionID: p.SessionID}

	stopReason, err := s.turnRunner.Run(turnCtx, p.SessionID, emit, p.Prompt)
	if err != nil {
		// D-16: if the turn was cancelled, report stopReason "cancelled" rather
		// than a hard error (the client expects a stopReason after cancel).
		if errors.Is(turnCtx.Err(), context.Canceled) {
			return sessionPromptResult{StopReason: stopCancelled}, nil
		}

		return nil, err
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

// newSessionID returns a fresh random session id (RFC 4122 v4 UUID shape). It
// panics on a CSPRNG failure — ass-guard cannot run without a working entropy
// source (mirrors internal/shaper uuidV4).
func newSessionID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic("crypto/rand failed: " + err.Error())
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
