package session //nolint:testpackage // internal package test (accesses unexported symbols)

// 16-REVIEW WR-04 regression pin: a turn cancelled mid-stream must report
// stopReason "cancelled" — never fall through to the end_turn branch (which
// appends the partial assistant message and fires the Stop hook for a turn the
// user cancelled, breaking the D-16 contract).
//
// The race window is deterministic here: the provider cancels the turn ctx
// INSIDE Stream — after runTurn's loop-head ctx check already passed — and
// returns a buffered channel whose partial text + done(end_turn) chunks are
// already in flight. streamAndEmit's mid-loop cancellation check is the gate
// that must route the turn to the cancelled branch.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// midStreamCancelProvider cancels the turn ctx inside Stream and hands back a
// buffered stream carrying a partial text chunk plus a terminal done(end_turn):
// everything arrived, but the context is already dead.
type midStreamCancelProvider struct {
	cancel context.CancelFunc
}

func (p *midStreamCancelProvider) Send(
	context.Context, *profile.Profile, []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, nil
}

func (p *midStreamCancelProvider) Stream(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.cancel() // ctx dies BEFORE any chunk is read — the loop-head check already passed

	ch := make(chan provider.StreamChunk, 2)

	ch <- provider.StreamChunk{Type: blockText, Text: "partial text before the abort"}

	ch <- provider.StreamChunk{Type: stopDone, FinishReason: stopEndTurn}

	close(ch)

	return ch, nil
}

func (p *midStreamCancelProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// SupportsImages: the fake is text-only (21-05 D-11 seam stub).
func (p *midStreamCancelProvider) SupportsImages() bool { return false }

func TestCancelledMidStreamReportsCancelled(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	m := newTestManager(t, "sess-midcancel")

	ctx, cancel := context.WithCancel(context.Background())

	s := &Session{
		Manager: m, Projector: NewProjector(fakeProfile("test"), m),
		Provider: &midStreamCancelProvider{cancel: cancel}, Bus: bus,
		Semaphore: provider.NewSemaphore(2),
		Profile:   *fakeProfile("test"), WorkDir: t.TempDir(), SessionID: "sess-midcancel",
		Catalog: toolcat.NewCatalog(),
	}

	stop, err := s.Prompt(ctx, []ContentBlock{{Type: blockText, Text: "hi"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopCancelled {
		t.Errorf("stop = %q; want %q — a mid-stream cancel fell through to the end_turn branch "+
			"(partial text appended, Stop hook fired; D-16 broken)", stop, stopCancelled)
	}

	// The canceled line is recorded (D-16 transcript honesty).
	hasCanceled := false

	lines, rerr := m.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	for _, l := range lines {
		if l.Type == TypeCanceled {
			hasCanceled = true
		}
	}

	if !hasCanceled {
		t.Error("transcript missing a canceled line after the mid-stream cancel (D-16)")
	}
}
