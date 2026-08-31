package acpserve

// The 17-02 permission-ask surface (ACP-01): builds the v1
// session/request_permission frame (exactly the four neutral option kinds;
// the ToolCallUpdate toolCall showing what is being authorized) and dispatches
// it through 16-03's outbound registry under the HUMAN-ASK class — ids, the
// D-14 ladder, the 16-D-19 cascade, and degradation are the registry's, never
// rebuilt here.
//
// The outcome mapping is fail-safe by construction: cancelled / -32800 → the
// cancelled family; -32601 (a pre-permission client) / D-14 timeout / any
// transport failure → the session-native Err outcome, which the gate's resume
// renders as a decline — never a silent allow, never a retry storm (the
// stickiness policy is the session's, 16-D-18 discipline).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// PermissionAsk dispatches permission asks through the registry (the queue's
// fire callback target). One per serve.
type PermissionAsk struct {
	registry *acp.Registry

	// ctx is the serve-lifetime context: the ask outlives its suspending turn
	// (the turn ctx died with the prompt response), so the registry round-trip
	// runs under the serve ctx — the askResumeCtx precedent.
	//
	//nolint:containedctx // deliberate serve-lifetime ctx storage (see NewPermissionAsk)
	ctx context.Context

	log *log.Logger
}

// NewPermissionAsk constructs the surface over the server's outbound registry.
// serveCtx bounds every ask round-trip; stderr (nil-tolerated) receives the
// structured diagnostics — never stdout (transport discipline).
func NewPermissionAsk(serveCtx context.Context, registry *acp.Registry, stderr io.Writer) *PermissionAsk {
	lg := log.New(io.Discard, "", 0)
	if stderr != nil {
		lg = log.New(stderr, "ass-guard/acpserve/ask: ", log.LstdFlags|log.Lmsgprefix)
	}

	return &PermissionAsk{registry: registry, ctx: serveCtx, log: lg}
}

// BuildPermissionAsk builds the v1 frame for one gated call. The option list
// IS the dialog: Zed renders the agent-supplied options as the dialog buttons
// (17-RESEARCH Zed fact 1) — exactly four, optionId == kind, neutral and
// symmetric labels (the ACP-01 values prohibition: never asymmetric-friction
// or steered framing).
func BuildPermissionAsk(sessionID, callID, title string, input json.RawMessage) acp.RequestPermissionFrame {
	toolCall := acp.ToolCallUpdateFrame{
		ToolCallID: callID,
		Title:      title,
		Kind:       acp.ToolKindExecute,
		Status:     acp.StatusPending,
	}

	if len(input) > 0 {
		// The raw input is the dialog's substance (the threat model's
		// "untrusted content DISPLAYED in the dialog's toolCall shape" —
		// shown for authorization, never executed here).
		toolCall.Content = []acp.ToolCallContent{{
			Type:    "content",
			Content: &acp.ContentBlock{Type: "text", Text: string(input)},
		}}
	}

	return acp.RequestPermissionFrame{
		SessionID: sessionID,
		ToolCall:  toolCall,
		Options: []acp.PermissionOption{
			{OptionID: acp.PermOptionAllowOnce, Name: "Allow once", Kind: acp.PermOptionAllowOnce},
			{OptionID: acp.PermOptionAllowAlways, Name: "Always allow", Kind: acp.PermOptionAllowAlways},
			{OptionID: acp.PermOptionRejectOnce, Name: "Reject once", Kind: acp.PermOptionRejectOnce},
			{OptionID: acp.PermOptionRejectAlways, Name: "Always reject", Kind: acp.PermOptionRejectAlways},
		},
	}
}

// Permission-ask failure sentinels — wrapped (never dynamic) so callers can
// errors.Is/As them; each maps to the fail-safe decline outcome.
var (
	errPermissionUnsupported    = errors.New("client does not implement session/request_permission")
	errPermissionOutcomeBad     = errors.New("malformed permission outcome")
	errPermissionOutcomeUnknown = errors.New("unknown permission outcome")
)

// Fire performs one permission-ask round-trip: frame build → Registry.Call
// under HUMAN-ASK → outcome mapping. It runs on the ask queue's pump
// goroutine (one outstanding ask, D-11); the HUMAN-ASK window means a human
// may think for the full configured wait (16-D-17) — the queue holds no lock
// while they do.
func (p *PermissionAsk) Fire(e *session.AskEntry) session.AskOutcome {
	frame := BuildPermissionAsk(e.SessionID, e.CallID, e.Title, e.Input)
	frame.ToolCall.Kind = e.Kind

	msg, err := p.registry.Call(p.ctx, acp.MethodRequestPermission, frame, acp.TimeoutHumanAsk)
	if err != nil {
		if errors.Is(err, acp.ErrRequestCancelled) {
			// Turn death / client cancel / the -32800 answer / shutdown drain:
			// the cancelled family (the session appends the cancelled-NORMAL
			// form, never an error).
			return session.AskOutcome{Cancelled: true}
		}

		// D-14's terminal fallback (unanswered after ONE retry) and every
		// other transport failure: Err → the gate's fail-safe decline.
		p.log.Printf("permission ask failed (failing safe, never allow): tool=%s callID=%s: %v",
			e.Tool, e.CallID, err)

		return session.AskOutcome{Err: err}
	}

	if msg.Error != nil {
		if msg.Error.Code == acp.CodeMethodNotFound {
			// A pre-permission client answers -32601 (the 16-D-18 degrade
			// family): Err → decline-with-note; the session records the
			// degradation sticky for the session.
			p.log.Printf("client does not implement %s (-32601) — failing safe (decline, sticky)",
				acp.MethodRequestPermission)

			return session.AskOutcome{Err: fmt.Errorf("%w (-32601)", errPermissionUnsupported)}
		}

		return session.AskOutcome{Err: fmt.Errorf("%w: code %d: %s",
			errPermissionOutcomeBad, msg.Error.Code, msg.Error.Message)}
	}

	var of acp.PermissionOutcomeFrame

	uerr := json.Unmarshal(msg.Result, &of)
	if uerr != nil {
		p.log.Printf("malformed permission outcome (failing safe): %v", uerr)

		return session.AskOutcome{Err: fmt.Errorf("%w: %w", errPermissionOutcomeBad, uerr)}
	}

	switch of.Outcome {
	case acp.PermissionOutcomeCancelled:
		return session.AskOutcome{Cancelled: true}
	case acp.PermissionOutcomeSelected:
		return session.AskOutcome{Selected: of.OptionID}
	default:
		p.log.Printf("unknown permission outcome %q (failing safe)", of.Outcome)

		return session.AskOutcome{Err: fmt.Errorf("%w %q", errPermissionOutcomeUnknown, of.Outcome)}
	}
}
