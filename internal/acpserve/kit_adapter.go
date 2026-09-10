package acpserve

// The 25-04 acp↔kit adapter (OQ2 resolution): ALL wire↔kit translation lives
// here, in the composition root — internal/acp stays wire-only (its
// ConfigSurface precedent; it imports nothing from the kit), and kit/runtime
// carries zero transport vocabulary (Phase-15 D-20 by construction). This
// file owns: the kit Emitter implementation (kit events → session/update
// frames), the TurnRunner adapter (wire blocks → session blocks, the kit-raw
// stop marker → the wire stop reason, OQ3), the relocated presentation table
// (tool name → ToolKind), and the optional-capability forwards the acp
// server type-asserts on its runner.

import (
	"context"
	"encoding/json"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/kit/event"
	"github.com/Djarvur/ass-guard-agent/kit/runtime"
	"github.com/Djarvur/ass-guard-agent/kit/session"
)

// blockTypeText is the wire's text content-block type (the literal the
// pre-seam kit code spelled with its own constant; wire vocabulary belongs
// app-side now).
const blockTypeText = "text"

// stopKitAsk is the kit's raw ask-suspension stop marker (session's own
// vocabulary, mirrored by value — the adapter maps it, the kit never does;
// OQ3).
const stopKitAsk = "ask"

// stopWireEndTurn is the wire stopReason a completed turn carries (the ask
// suspension maps here: the client received its prompt response, the
// question arrived just before as a client-visible update, and the pending
// ask lives in the session broker — 12-01 ACP-01).
const stopWireEndTurn = "end_turn"

// mapKitStop maps the kit-raw stop marker to the ACP-facing stopReason of a
// completed turn (the relocated 12-01 mapping, OQ3: ONLY the adapter maps;
// cron audit lines keep recording the kit-raw value). Every other stop
// reason passes through unchanged.
func mapKitStop(stop string) string {
	if stop == stopKitAsk {
		return stopWireEndTurn
	}

	return stop
}

// toSessionBlocks converts the ACP content blocks to session content blocks
// (the relocated 25-04 conversion; 21-05 PAR-06 semantics carried verbatim:
// image blocks map through with their Data — the pre-ingress base64 carrier
// — and declared MimeType; the kit's ingress validates + swaps them for Ref
// form before the transcript append).
func toSessionBlocks(in []acp.ContentBlock) []session.ContentBlock {
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

// Captured core tool names referenced by toolKindFor (named so the mapping
// table reads as a table, not a string pile — relocated verbatim-in-role
// from the pre-seam kit forwarders).
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

// kitEmitter implements the kit Emitter seam (D-14) over the server's
// per-session emitter handle: kit events become session/update frames. The
// frame construction is the relocated pre-seam translation, verbatim-in-role
// — AgentMessageChunk → the text chunk, ToolCall/ToolCallUpdate → the
// ACP-03 tool-card frames (toolKindFor table included), AgentThoughtChunk →
// agent_thought_chunk, UserMessageChunk → user_message_chunk (the class-B
// echo, 20-01 D-05). A handle without the ActivityEmitter capability (plain
// ChunkEmitter fakes) silently skips the tool/thought/user frames — the
// 16-01 assertion-skip preserved.
type kitEmitter struct {
	emit acp.ChunkEmitter
}

// Emit translates one kit event to its wire frame(s).
//
//nolint:wrapcheck // thin translation delegation — the handle owns the error
func (e kitEmitter) Emit(_ context.Context, ev event.Event) error {
	toolEmit, toolOK := e.emit.(acp.ActivityEmitter)

	switch c := ev.(type) {
	case event.AgentMessageChunk:
		return e.emit.AgentMessageChunk(c.MessageID, c.Content)
	case event.UserMessageChunk:
		if !toolOK {
			return nil
		}

		return toolEmit.UserMessageChunk(c.MessageID, c.Content)
	case event.AgentThoughtChunk:
		if !toolOK {
			return nil
		}

		return toolEmit.ThoughtChunk(c.MessageID, acp.ContentBlock{Type: blockTypeText, Text: c.Content})
	case event.ToolCall:
		if !toolOK {
			return nil
		}

		return toolEmit.ToolCall(&acp.ToolCallFrame{
			ToolCallID: c.ToolCallID,
			Title:      c.Name,
			Kind:       toolKindFor(c.Name),
			Input:      append(json.RawMessage(nil), c.Input...),
		})
	case event.ToolCallUpdate:
		if !toolOK || len(c.Update) == 0 {
			return nil
		}

		var uf acp.ToolCallUpdateFrame

		_ = json.Unmarshal(c.Update, &uf)
		uf.ToolCallID = c.ToolCallID

		return toolEmit.ToolCallUpdate(&uf)
	default:
		return nil
	}
}

// kitEmitterFactory adapts the server's per-session emitter handle factory
// (func(sessionID) acp.ChunkEmitter) to the kit's Emitter factory seam
// (func(sessionID) runtime.Emitter) — the SetEmitter injection keeps its
// exact statement position in Run (WINDOWS #3).
func kitEmitterFactory(
	emitFor func(sessionID string) acp.ChunkEmitter,
) func(sessionID string) runtime.Emitter {
	return func(sessionID string) runtime.Emitter {
		return kitEmitter{emit: emitFor(sessionID)}
	}
}

// kitTurnAdapter implements the FROZEN acp.TurnRunner interface (16-20 wire
// surface, server.go:33 — signature byte-unchanged) over the kit Runner:
// wire blocks convert to session blocks (toSessionBlocks), the in-hand wire
// emitter wraps as the kit Emitter (kitEmitter), and the kit-raw stop marker
// maps to the wire stop reason at THIS layer only (OQ3: the kit returns its
// raw marker; cron audit lines record the raw value).
//
// The optional capabilities the acp server type-asserts on its TurnRunner
// (SessionCloser, SessionLoader, ModeStateProvider, AskDrainer) are
// forwarded to the wrapped runner by delegation — the runner keeps
// implementing them; the wire keeps seeing them.
type kitTurnAdapter struct {
	runner *runtime.Runner
}

// Run drives one session/prompt through the kit Runner (see acp.TurnRunner).
func (a kitTurnAdapter) Run(
	ctx context.Context, sessionID string,
	emit acp.ChunkEmitter, prompt []acp.ContentBlock,
) (string, error) {
	stop, err := a.runner.Run(ctx, sessionID, kitEmitter{emit: emit}, toSessionBlocks(prompt))

	return mapKitStop(stop), err
}

// CloseSession forwards acp.SessionCloser (logout / session close+delete).
func (a kitTurnAdapter) CloseSession(sessionID string) error {
	return a.runner.CloseSession(sessionID) //nolint:wrapcheck // thin delegation
}

// ResumeSession forwards acp.SessionLoader (session/load, 18-01/18-05).
func (a kitTurnAdapter) ResumeSession(ctx context.Context, sessionID string) error {
	return a.runner.ResumeSession(ctx, sessionID) //nolint:wrapcheck // thin delegation
}

// LoadedModes forwards acp.ModeStateProvider (the load response's modes).
func (a kitTurnAdapter) LoadedModes(sessionID string) any {
	return a.runner.LoadedModes(sessionID)
}

// DrainAsks forwards acp.AskDrainer (the session/cancel teardown, 17-03).
func (a kitTurnAdapter) DrainAsks(sessionID string) { a.runner.DrainAsks(sessionID) }

// DrainSessionAsks forwards acp.AskDrainer's close twin (logout ordering).
func (a kitTurnAdapter) DrainSessionAsks(sessionID string) { a.runner.DrainSessionAsks(sessionID) }

// kitRequester implements the kit Requester seam (D-15, 25-04 Task 3) over
// the LOCKED 17-02/17-04 ask surfaces: the permission family bridges to
// PermissionAsk.Fire (registry-backed session/request_permission), the
// question family to ElicitationAsk.Fire (capability-gated
// elicitation/create with the plain-text fallback inside). Neither surface's
// wire shape is redesigned — the entry is rebuilt field-for-field from the
// seam payload and the outcome projected back.
//
// Timer ownership (the no-double-timeout record): the LANDED 17-02 code arms
// exactly two windows — the registry's HUMAN-ASK timeout bounds the WIRE
// round-trip (transport, adapter-owned) and the kit's AskBroker owns the
// D-01 suspension timer (the capture-shaped non-answer resume, kit-side
// config per D-15). This bridge arms NEITHER; it adds no timer of its own,
// so cancelled/timeout resolution keeps the landed semantics byte-identical.
type kitRequester struct {
	perm   *PermissionAsk
	elicit *ElicitationAsk
}

// newKitRequester wires the bridge over the two landed ask surfaces.
//
//nolint:ireturn // the kit seam's own interface (the SetRequester input type)
func newKitRequester(perm *PermissionAsk, elicit *ElicitationAsk) runtime.Requester {
	return kitRequester{perm: perm, elicit: elicit}
}

// Request performs one ask round-trip (ctx-blocking until the surface
// resolves — both landed Fire implementations block on their registry call).
//
//nolint:gocritic // hugeParam: the D-15 seam signature speaks values
func (k kitRequester) Request(ctx context.Context, ask runtime.Ask) (runtime.Answer, error) {
	entry := &session.AskEntry{
		TurnID:    ask.TurnID,
		SessionID: ask.SessionID,
		CallID:    ask.CallID,
		Tool:      ask.Tool,
		Title:     ask.Title,
		Kind:      ask.Kind,
		Input:     ask.Input,
		Note:      ask.Note,
		Class:     entryClassOf(ask.Background),
		// The question family's degraded surface stays ANSWERABLE (the
		// broker's reply routing + D-01 timer own it); engine asks carried
		// false at enqueue and keep it here.
		PlainTextFallback: ask.PlainTextFallback,
	}

	var out session.AskOutcome

	if ask.Family == runtime.AskFamilyPermission {
		out = k.perm.Fire(ctx, entry)
	} else {
		out = k.elicit.Fire(ctx, entry)
	}

	return runtime.Answer{
		Selected:    out.Selected,
		Cancelled:   out.Cancelled,
		Unsupported: out.Unsupported,
		Elicit:      out.Elicit,
		Content:     out.Content,
		Violation:   out.Violation,
		Fallback:    out.Fallback,
		Err:         out.Err,
	}, nil
}

// entryClassOf mirrors the D-11 priority class onto the entry (queue
// fidelity — the rebuilt entry carries what the original enqueue set).
func entryClassOf(background bool) session.AskClass {
	if background {
		return session.AskClassBackground
	}

	return session.AskClassForeground
}
