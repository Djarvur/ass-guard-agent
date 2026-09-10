package runtime //nolint:testpackage // internal package test

// The 25-04 assertion-preserving test retarget: the production wire↔kit
// adapter lives app-side (internal/acpserve/kit_adapter.go — the OQ2
// composition-root resolution), and the kit Runner no longer structurally
// satisfies acp.TurnRunner (Run now speaks the kit seam: an Emitter,
// session blocks, the raw stop marker). The relocated batteries in this
// package were written against the acp shape — acp.ContentBlock prompts,
// acp.ChunkEmitter/ActivityEmitter fakes, mapped stop assertions — so this
// file provides the in-package TWIN of the production adapter (identical
// translation, identical capability forwards). Batteries keep their fakes,
// prompts, and assertions byte-identical; only the construction of the
// runner-under-test changes (r.Run(...) -> acpRun(ctx, r, ...)). The
// production adapter itself is pinned end-to-end by the acpserve serve /
// simulator suites.

import (
	"context"
	"encoding/json"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/kit/event"
	"github.com/Djarvur/ass-guard-agent/kit/session"
)

// acpRun drives one turn through the kit Runner under the acp.TurnRunner
// shape the batteries were written against (blocks converted, emitter
// wrapped, stop marker mapped — the production adapter's twin).
func acpRun(
	ctx context.Context, r *Runner, sessionID string,
	emit acp.ChunkEmitter, prompt []acp.ContentBlock,
) (string, error) {
	return acpTurnRunner{r: r}.Run(ctx, sessionID, emit, prompt)
}

// acpTurnRunner is the test-local acp.TurnRunner twin of the production
// kitTurnAdapter (internal/acpserve/kit_adapter.go).
type acpTurnRunner struct{ r *Runner }

func (a acpTurnRunner) Run(
	ctx context.Context, sessionID string,
	emit acp.ChunkEmitter, prompt []acp.ContentBlock,
) (string, error) {
	stop, err := a.r.Run(ctx, sessionID, acpKitEmitter{emit: emit}, acpToSessionBlocks(prompt))

	return acpMapKitStop(stop), err
}

// CloseSession forwards acp.SessionCloser.
func (a acpTurnRunner) CloseSession(sessionID string) error { return a.r.CloseSession(sessionID) }

// ResumeSession forwards acp.SessionLoader.
func (a acpTurnRunner) ResumeSession(ctx context.Context, sessionID string) error {
	return a.r.ResumeSession(ctx, sessionID)
}

// LoadedModes forwards acp.ModeStateProvider.
func (a acpTurnRunner) LoadedModes(sessionID string) any { return a.r.LoadedModes(sessionID) }

// DrainAsks / DrainSessionAsks forward acp.AskDrainer.
func (a acpTurnRunner) DrainAsks(sessionID string)        { a.r.DrainAsks(sessionID) }
func (a acpTurnRunner) DrainSessionAsks(sessionID string) { a.r.DrainSessionAsks(sessionID) }

// acpKitEmitter is the test-local twin of the production kitEmitter: kit
// events translated back onto the acp fakes the batteries record against.
type acpKitEmitter struct{ emit acp.ChunkEmitter }

//nolint:wrapcheck // thin translation delegation — the handle owns the error
func (e acpKitEmitter) Emit(_ context.Context, ev event.Event) error {
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

		return toolEmit.ThoughtChunk(c.MessageID, acp.ContentBlock{Type: blockText, Text: c.Content})
	case event.ToolCall:
		if !toolOK {
			return nil
		}

		return toolEmit.ToolCall(&acp.ToolCallFrame{
			ToolCallID: c.ToolCallID,
			Title:      c.Name,
			Kind:       acpToolKindFor(c.Name),
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

// acpKitEmitterFactory adapts the server's wire emitter factory to the kit
// seam (the production kitEmitterFactory's twin) — the SetEmitter test
// wiring sites keep their WINDOWS #3 junction shape.
func acpKitEmitterFactory(
	emitFor func(sessionID string) acp.ChunkEmitter,
) func(sessionID string) Emitter {
	return func(sessionID string) Emitter {
		return acpKitEmitter{emit: emitFor(sessionID)}
	}
}

// acpMapKitStop is the production mapKitStop twin (the 12-01 mapping).
func acpMapKitStop(stop string) string {
	if stop == stopAskACP {
		return stopEndTurn
	}

	return stop
}

// acpToSessionBlocks is the production toSessionBlocks twin.
func acpToSessionBlocks(in []acp.ContentBlock) []session.ContentBlock {
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

// acpToolKindFor is the production toolKindFor twin (the presentation table
// the pre-seam kit code owned).
func acpToolKindFor(name string) string {
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
