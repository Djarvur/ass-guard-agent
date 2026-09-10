package runtime

// The KIT-02 frontend seams (25-04, D-13's minimal pair). Both interfaces are
// defined here at the point of use (the accept-interfaces rule — the same
// family as internal/acp/server.go's seam definitions): the kit consumes
// them; the FRONTEND implements them. The ACP implementation lives app-side
// in internal/acpserve (the OQ2 composition-root resolution); a non-ACP
// frontend (e.g. a chat transport) implements the same two interfaces and
// hosts the identical kit — the design-level proof the 25-09 host-proof test
// pins.

import (
	"context"
	"encoding/json"

	"github.com/Djarvur/ass-guard-agent/kit/event"
	"github.com/Djarvur/ass-guard-agent/kit/session"
)

// Emitter streams the kit's neutral event vocabulary to the frontend (D-14,
// KIT-02 half one): exactly ONE method, one event at a time, in order. The
// kit emits kit/event kinds (AgentMessageChunk, AgentThoughtChunk, ToolCall,
// ToolCallUpdate, UserMessageChunk — the union is open and additive: a new
// frontend need adds an event kind, never a second method). Implemented by
// the frontend adapter (internal/acpserve for the ACP wire), which translates
// each kind to its transport's frames — the kit builds no transport frames
// itself (Phase-15 D-20 by construction).
//
// A nil / uninjected emitter is the documented degraded state: today's
// nil-emitter behavior is preserved verbatim — headless runners keep
// transcript + audit only, with no streaming (startSessionForwarder's
// early return). Errors are the adapter's transport discipline; the kit
// treats them as best-effort (the forwarders log-and-continue, exactly as
// the pre-seam code did).
type Emitter interface {
	// Emit delivers one kit event. Implementations translate it to their
	// transport; ctx is the emitting turn's (or the serve-lifetime) context.
	Emit(ctx context.Context, e event.Event) error
}

// Requester carries the kit's asks to the frontend (D-15, KIT-02 half two):
// exactly ONE method, ctx-BLOCKING — Request returns when the ask resolves
// (an answer, a cancel, or a failure), never earlier. Implemented by the
// frontend adapter over its ask surface (internal/acpserve bridges the
// locked 17-02/17-04 permission/elicitation surfaces).
//
// Timeout policy is KIT-side config mirroring the Phase-12 D-01 ask timeout
// (a configurable wait, then the capture-shaped non-answer; a negative value
// normalizes to the default, 0 blocks forever) — the AskBroker already owns
// that timer, so a Requester implementation must NOT arm a second one: the
// adapter owns transport only. A cancelled context resolves as a NORMAL
// non-answer (the cancelled family), never a wedge.
//
// A Telegram-shaped consumer could implement this same interface to host the
// kit — the minimal pair is the whole frontend contract.
type Requester interface {
	// Request asks one question and blocks the caller's context until it
	// resolves. The returned Answer carries the operator's reply; err (when
	// non-nil) is the surfaced failure, which the kit maps to its fail-safe
	// decline — never a silent allow.
	Request(ctx context.Context, ask Ask) (Answer, error)
}

// Ask is the kit-neutral ask payload: exactly what today's pending-ask flow
// transports (the questions, the ask kind, and the gated call's identity) —
// no wire vocabulary. Family selects the surface the frontend should ride
// (the permission gate vs the question/elicitation family), mirroring which
// fire seam the ask originated from pre-seam.
type Ask struct {
	// Family is the ask's surface family: AskFamilyPermission (a gated tool
	// call's dialog) or AskFamilyQuestion (AskUserQuestion / engine asks —
	// the elicitation family).
	Family AskFamily

	// SessionID, TurnID, CallID identify the asking turn and the pending
	// call (a reply resolves exactly its own ask).
	SessionID string
	TurnID    string
	CallID    string

	// Tool / Title / Kind carry the dialog card's identity (the gated tool's
	// name; the card title; the optional tool-kind hint).
	Tool  string
	Title string
	Kind  string

	// Questions are the model-authored questions (question-family asks).
	Questions []session.AskQuestion

	// Input is the gated call's raw input (permission family — displayed for
	// authorization, never executed by the surface).
	Input json.RawMessage

	// Note names the previous attempt's schema violation on a re-ask (the
	// one-bounded-re-ask discipline); empty on a first ask.
	Note string

	// Background marks an automation/engine-origin ask (the priority class:
	// foreground asks fire before background ones).
	Background bool
}

// AskFamily selects the ask's surface family (see Ask.Family).
type AskFamily uint8

const (
	// AskFamilyPermission — a gated tool call's permission dialog.
	AskFamilyPermission AskFamily = iota
	// AskFamilyQuestion — the question/elicitation family (AskUserQuestion
	// and engine asks).
	AskFamilyQuestion
)

// Answer is the kit-neutral resolution of one Ask: the answers, the
// cancelled flag, and the failure surface — the mirror of the session-native
// outcome the pending-ask flow already transports. Exactly one of the
// answer-bearing fields is meaningful per family (Selected for permission;
// Elicit/Content for questions); Cancelled and Err are family-neutral.
type Answer struct {
	// Selected is the operator's picked option kind (permission family).
	Selected string

	// Cancelled marks the cancelled family: a dismissed dialog, a client
	// cancel, turn death, or a shutdown drain — resolved NORMAL, never an
	// error.
	Cancelled bool

	// Elicit is the question-family action ("accept" / "decline"; cancel
	// rides Cancelled, a degraded surface rides Fallback).
	Elicit string

	// Content is an accept's raw structured payload — UNTRUSTED input; the
	// kit re-validates it against the requested schema before consuming it.
	Content map[string]json.RawMessage

	// Violation names the schema violation of an invalid accept payload (the
	// kit's bounded re-ask reads it); empty when the content is valid.
	Violation string

	// Fallback marks the degraded plain-text surface: the ask's plain-text
	// form was published instead of a structured round-trip, and the kit's
	// own reply routing / timeout own the rest.
	Fallback bool

	// Unsupported marks the cannot-answer-at-all degrade family (the kit
	// records it sticky so later asks decline without a new round-trip).
	Unsupported bool

	// Err is any failure that is not a cancellation (surface unwired,
	// unsupported-client degrade, transport failure). The kit maps it to its
	// fail-safe decline — never a silent allow, never a retry storm.
	Err error
}
