package acp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
)

const asciiDelete = 0x40
const uuidVariantSet = 0x80
const variantMask = 0x3F
const versionMask = 0x0F

var errMissingSessionid = errors.New("session/prompt: missing sessionId")

// AskDrainer is an OPTIONAL capability a TurnRunner may implement (17-03,
// D-13/T-17-09): the teardown-drain seam for the session's ask queue. THE one
// drain is reachable from all three teardown paths — the session/cancel
// notification (handleSessionCancel, before/with cancelTurn), the session-close
// path (handleLogout, before the reap), and serve shutdown (the acpserve
// composition). Turn death resolves the OPEN permission dialog cancelled
// through the registry's ResolveCancelled + $/cancel_request cascade (16-D-19)
// and drains queued-but-unfired asks as cancelled-normal — no orphaned dialogs,
// no zombie asks. It is a separate interface (not part of TurnRunner) so stub
// runners need not implement it.
type AskDrainer interface {
	// DrainAsks drains the session's asks on a turn cancellation: the ACP
	// cancel contract covers ALL pending session/request_permission requests
	// (the cancelled-outcome normative text), so the open dialog resolves and
	// queued asks drain before/with the turn cancel.
	DrainAsks(sessionID string)
	// DrainSessionAsks drains every ask of the session on session close —
	// nothing dialog-shaped outlives the session.
	DrainSessionAsks(sessionID string)
}

// drainAsksIfPossible type-asserts the TurnRunner to AskDrainer and drains the
// session's ask queue (the D-13 teardown seam). A non-implementing runner is a
// no-op (backward-compatible — stub runners and pre-17 sessions).
func (s *Server) drainAsksIfPossible(sessionID string) {
	drainer, ok := s.turnRunner.(AskDrainer)
	if !ok {
		return
	}

	drainer.DrainAsks(sessionID)
}

// drainSessionAsksIfPossible is the session-close variant (logout).
func (s *Server) drainSessionAsksIfPossible(sessionID string) {
	drainer, ok := s.turnRunner.(AskDrainer)
	if !ok {
		return
	}

	drainer.DrainSessionAsks(sessionID)
}

// registerHandlers populates the method→handler map with the canonical ACP v1
// method set (VERIFIED-FACTS #3): initialize, session/new, session/prompt,
// session/cancel, session/load (no-op per D-09), logout, session/set_mode,
// session/set_config_option (16-05/ACP-08).
func (s *Server) registerHandlers() {
	s.handlers[methodInitialize] = s.handleInitialize
	s.handlers["session/new"] = s.handleSessionNew
	s.handlers["session/prompt"] = s.handleSessionPrompt
	s.handlers["session/cancel"] = s.handleSessionCancel
	s.handlers["session/load"] = s.handleSessionLoad
	s.handlers["logout"] = s.handleLogout
	s.handlers["session/set_mode"] = s.handleSessionSetMode
	s.handlers[methodCancelRequest] = s.handleCancelRequestNoOp
	s.handlers[methodSetConfigOption] = s.handleSetConfigOption
}

// initializeResponse is the initialize result (INITIALIZATION.md / VERIFIED-
// FACTS #3). The field name is `agentCapabilities` (NOT capabilities/serverInfo),
// protocolVersion is integer 1, and loadSession is false (D-09 — NO replay).
// configOptions (16-05) advertises the v1.2 menu with effective current values
// when a ConfigSurface is wired; omitted entirely without one (the degrade —
// existing acpserve-less handshakes stay byte-compatible).
type initializeResponse struct {
	ProtocolVersion   int                 `json:"protocolVersion"`         //nolint:tagliatelle // ACP wire field
	AgentCapabilities map[string]any      `json:"agentCapabilities"`       //nolint:tagliatelle // ACP wire field
	AgentInfo         map[string]any      `json:"agentInfo"`               //nolint:tagliatelle // ACP wire field
	AuthMethods       []any               `json:"authMethods"`             //nolint:tagliatelle // ACP wire field
	ConfigOptions     []ConfigOptionFrame `json:"configOptions,omitempty"` //nolint:tagliatelle // ACP wire field
}

// handleInitialize negotiates the elicitation-form capability (D-13) and
// responds. Negotiation is advertisement-first: a clientCapabilities
// elicitation.form advertisement caches ok and skips the probe entirely (real
// Zed handshakes never see a probe — VERIFIED acp.rs:767-795). Without the
// advertisement, at most ONE probe is issued for the connection (sticky after
// either outcome — D-18), bounded by the D-14 ladder. initialize ALWAYS
// responds, degraded or not; agentCapabilities are unchanged by degradation
// (D-13: every surface knows to degrade — the response never withholds).
//
// 16-05 (D-10): the request's _meta object is the schema-tolerant
// client-defaults blob channel — recognized option keys fill unset slots
// in-memory (explicit config always wins), unknown keys are retained verbatim,
// and the surface emits the out-of-band config_option_update carrying the full
// refreshed set when application moved a value. The response advertisement is
// built AFTER application so it carries the post-blob effective values.
func (s *Server) handleInitialize(ctx context.Context, params json.RawMessage) (any, error) {
	s.applyMetaBlob(params)

	if len(params) > 0 {
		var p struct {
			ClientCapabilities struct {
				Elicitation struct {
					Form *json.RawMessage `json:"form,omitempty"`
				} `json:"elicitation"`
				Session struct {
					ConfigOptions struct {
						Boolean *json.RawMessage `json:"boolean,omitempty"`
					} `json:"configOptions"` //nolint:tagliatelle // ACP wire field
				} `json:"session"`
			} `json:"clientCapabilities"` //nolint:tagliatelle // ACP wire field
		}

		// Tolerant parse (unknown shapes treated as absent — schema-tolerant
		// style): a parse miss just falls through to the probe path.
		unmarshalErr := json.Unmarshal(params, &p)
		if unmarshalErr == nil {
			// 17-04 (D-08): the boolean config-option advertisement rides the
			// same negotiation — the conservative boolean-property gate reads
			// it (CapBooleanConfigOption; unadvertised degrades to the
			// two-value string select, the CapabilityUnknown zero value).
			if p.ClientCapabilities.Session.ConfigOptions.Boolean != nil {
				s.capabilities.set(CapBooleanConfigOption, CapabilityOK)
			}

			if p.ClientCapabilities.Elicitation.Form != nil {
				s.capabilities.set(CapElicitationForm, CapabilityOK)
				s.log.Printf("capability %s: advertised by client (no probe)", CapElicitationForm)

				return s.initializeResult(), nil
			}
		}
	}

	// Sticky check (D-18): already negotiated for this connection — never
	// re-probe, no flapping.
	if state := s.capabilities.get(CapElicitationForm); state != CapabilityUnknown {
		return s.initializeResult(), nil
	}

	s.capabilities.set(CapElicitationForm, s.probeElicitationCapability(ctx))

	return s.initializeResult(), nil
}

// applyMetaBlob parses the initialize request's _meta object schema-tolerantly
// and hands it to the ConfigSurface (D-10's blob channel). Absent surface,
// absent _meta, or a malformed _meta all degrade quietly — a defaults channel
// can never fail the handshake. Application errors are logged loudly (the
// surface owns fills-unset semantics; a partial apply must be visible).
func (s *Server) applyMetaBlob(params json.RawMessage) {
	if s.configSurf == nil || len(params) == 0 {
		return
	}

	var p struct {
		Meta map[string]json.RawMessage `json:"_meta"` //nolint:tagliatelle // ACP wire field
	}

	uerr := json.Unmarshal(params, &p)
	if uerr != nil || len(p.Meta) == 0 {
		return
	}

	changed, err := s.configSurf.ApplyBlobDefaults(p.Meta)
	if err != nil {
		s.log.Printf("config: initialize _meta blob application failed (continuing): %v", err)

		return
	}

	if changed {
		s.log.Printf("config: initialize _meta defaults applied (config_option_update emitted)")
	}
}

// initializeResult builds the initialize response (integer protocolVersion 1,
// loadSession:false per D-09) plus the configOptions advertisement (16-05) via
// the shared builder — omitted entirely when no surface is wired.
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
		AuthMethods:   []any{},
		ConfigOptions: s.configOptionsFor(),
	}
}

// configOptionsFor is THE advertisement builder (16-05): one function feeding
// initialize and session/new (and later load/resume in Phase 18), reading the
// injected ConfigSurface. Nil surface → nil (the caller's omitempty drops the
// field — the degrade shape).
func (s *Server) configOptionsFor() []ConfigOptionFrame {
	if s.configSurf == nil {
		return nil
	}

	return s.configSurf.Options()
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

	msg, callErr := s.registry.Call(ctx, MethodElicitationCreate, payload, TimeoutFastControl)

	switch {
	case callErr != nil && errors.Is(callErr, ErrRequestCancelled):
		s.metrics.noteProbeFallback()
		s.log.Printf("capability probe %s: cancelled — degrading (D-13/D-18)", CapElicitationForm)

		return CapabilityDegraded
	case callErr != nil:
		var timeoutErr *RequestTimeoutError
		if errors.As(callErr, &timeoutErr) {
			// The D-14 ladder burned BOTH windows (initial + one retry).
			s.metrics.noteProbeTimeout()
			s.metrics.noteProbeTimeout()
			s.metrics.noteProbeFallback()
			s.log.Printf("capability probe %s: fallback after retry — degrading (D-14)", CapElicitationForm)
		} else {
			s.metrics.noteProbeFallback()
			s.log.Printf("capability probe %s: %v — degrading", CapElicitationForm, callErr)
		}

		return CapabilityDegraded
	case msg.Error != nil:
		s.metrics.noteProbeFallback()
		s.log.Printf("capability probe %s: answered with jsonrpc error %d — degrading (D-13)",
			CapElicitationForm, msg.Error.Code)

		return CapabilityDegraded
	default:
		s.log.Printf("capability probe %s: answered — ok", CapElicitationForm)

		return CapabilityOK
	}
}

// sessionNewResult carries the sessionId the client threads into session/prompt,
// plus the configOptions advertisement (16-05 — the v1 NewSessionResponse
// shape; omitted entirely without a wired surface).
type sessionNewResult struct {
	SessionID     string              `json:"sessionId"`               //nolint:tagliatelle // ACP wire field
	ConfigOptions []ConfigOptionFrame `json:"configOptions,omitempty"` //nolint:tagliatelle // ACP wire field
}

// handleSessionNew creates a sessionState and returns its id (plus the shared
// configOptions advertisement — Zed applies its stored defaults against the
// advertised menu right after session/new, acp.rs:1303-1391).
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

	return sessionNewResult{SessionID: id, ConfigOptions: s.configOptionsFor()}, nil
}

// setConfigOptionParams is the session/set_config_option payload (schema/v1
// SetSessionConfigOptionRequest: sessionId + configId + value; the request's
// option-key field is verbatim `configId` while the ADVERTISEMENT uses `id` —
// both pinned against the fetched v1 schema). The value is the value_id string
// variant for select options (ass-guard advertises selects only).
type setConfigOptionParams struct {
	SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	ConfigID  string `json:"configId"`  //nolint:tagliatelle // ACP wire field
	Value     any    `json:"value,omitempty"`
}

// setConfigOptionResult carries the FULL refreshed option set (schema/v1
// SetSessionConfigOptionResponse: "the full set of configuration options and
// their current values" — the response shape after EVERY non-error outcome).
type setConfigOptionResult struct {
	ConfigOptions []ConfigOptionFrame `json:"configOptions"` //nolint:tagliatelle // ACP wire field
}

// handleSetConfigOption is the relay half of editor-driven configuration
// (16-05/D-05..D-09): it validates request SHAPE, relays to the injected
// ConfigSurface (which owns menu semantics, persist-then-apply ordering,
// pending no-ops, and idempotent re-pushes), and translates surface outcomes
// to typed JSON-RPC errors. A absent surface degrades to the typed
// not-available error — never a crash. Handler validation covers shape only;
// menu/value validation is the surface's (D-09 violations arrive as
// *ConfigViolationError, persist failures as *ConfigPersistError — distinct
// wire classes).
func (s *Server) handleSetConfigOption(ctx context.Context, params json.RawMessage) (any, error) {
	if s.configSurf == nil {
		return nil, &RPCError{
			Code:    CodeInvalidRequest,
			Message: "session/set_config_option not available (no configuration surface wired)",
		}
	}

	var p setConfigOptionParams

	uerr := json.Unmarshal(params, &p)
	if uerr != nil {
		return nil, &ConfigViolationError{Violation: "malformed params: " + uerr.Error()}
	}

	opts, err := s.configSurf.Set(p.SessionID, p.ConfigID, p.Value)
	if err != nil {
		var violation *ConfigViolationError
		if errors.As(err, &violation) {
			return nil, &RPCError{
				Code:    CodeInvalidParams,
				Message: violation.Error(),
				Data:    map[string]string{"optionId": violation.OptionID, "violation": violation.Violation},
			}
		}

		var persist *ConfigPersistError
		if errors.As(err, &persist) {
			return nil, &RPCError{
				Code:    CodeInternalError,
				Message: persist.Error(),
				Data: map[string]string{
					"optionId":  persist.OptionID,
					"violation": "persist failed (state untouched)",
				},
			}
		}

		// Unknown surface error: generic internal class, scrubbed upstream.
		return nil, fmt.Errorf("set config option: %w", err)
	}

	return setConfigOptionResult{ConfigOptions: opts}, nil
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
//
// ACP v1 cancel semantics are PER-TURN: the session persists and the client is
// expected to re-prompt the same sessionId (the standard editor flow — escape,
// then ask again). This handler therefore cancels ONLY the turn; it never
// reaps session-scoped resources (16-REVIEW CR-01). Resource reaping stays on
// the true session-end paths: logout (closeSessionIfPossible + the sessions
// map delete, as handleLogout does) and serve teardown (the runner's
// ctx-done CloseAllSessions). A parked engine chain of a LIVE cancelled turn
// is drained transitively — runOneTurn's request-ctx watchdog folds the
// cancelled turn ctx into the parked cancel (ENG-03).
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

	// 17-03 (D-13): the ask-queue drain runs BEFORE/with the turn cancel —
	// the ACP cancel contract covers ALL pending session/request_permission
	// requests: the OPEN dialog resolves cancelled (the registry cascade
	// closes it on the wire) and queued-but-unfired asks drain cancelled-normal
	// immediately (no zombie asks firing for a dead turn).
	s.drainAsksIfPossible(p.SessionID)

	st.cancelTurn()
	// 16-REVIEW CR-01: NO closeSessionIfPossible here. The old call reaped the
	// session's MCP host, transcript writer, session forwarder, and SessionEnd
	// hook while the session stayed registered in s.sessions — every later
	// session/prompt on the id then ran against the half-reaped Session (mcp__*
	// calls dead, audit streaming dead, forwarding gone, SessionEnd fired
	// twice-ish). Cancel means "abort this turn"; the session stays live.

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
		// 17-03 (D-13/T-17-09): no dialog or queued ask outlives the session —
		// the open dialog resolves cancelled (the cascade closes it on the
		// wire) and queued asks drain cancelled-normal BEFORE the reap.
		s.drainSessionAsksIfPossible(p.SessionID)

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
