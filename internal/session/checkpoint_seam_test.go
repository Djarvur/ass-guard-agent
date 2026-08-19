package session //nolint:testpackage // internal package test (seam ordering via the public API)

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// seamEventLog records cross-component ordering (checkpoint vs provider) with
// one mutex so the sequencing assertion needs no sleeps.
type seamEventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *seamEventLog) add(ev string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = append(l.events, ev)
}

func (l *seamEventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return append([]string(nil), l.events...)
}

// recordingCheckpointer is the Checkpointer test double: it records the
// (sessionID, turnID) it was called with, in the shared event log, and
// returns the configured error (nil by default).
type recordingCheckpointer struct {
	log *seamEventLog
	err error
}

func (c *recordingCheckpointer) SnapshotTurn(_ context.Context, sessionID, turnID string) error {
	c.log.add("checkpoint:" + sessionID + ":" + turnID)

	return c.err
}

// observingProvider wraps the fake provider, recording every Stream call in
// the shared event log so the test can assert the snapshot fired BEFORE the
// turn's first provider call.
type observingProvider struct {
	*fakeProvider

	log *seamEventLog
}

func (p *observingProvider) Stream(
	ctx context.Context, prof *profile.Profile, msgs []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.log.add("provider-stream")

	return p.fakeProvider.Stream(ctx, prof, msgs)
}

// TestPrompt_SnapshotsAtTurnEntry (Task 1, Test 4): Prompt calls the
// Checkpointer with the session's id and the turn's id, and the call lands
// BEFORE the turn's first provider call — the snapshot is the PRE-turn state.
func TestPrompt_SnapshotsAtTurnEntry(t *testing.T) {
	t.Parallel()

	s, _, fp := newTestSession(t, nil, []provider.Response{{FinishReason: stopEndTurn}})

	log := &seamEventLog{}
	s.Checkpointer = &recordingCheckpointer{log: log}
	s.Provider = &observingProvider{fakeProvider: fp, log: log}

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want %q", stop, stopEndTurn)
	}

	events := log.snapshot()
	want := []string{"checkpoint:sess-test:sess-test-turn-001", "provider-stream"}

	if !equalStrings(events, want) {
		t.Errorf("event sequence = %v; want %v (snapshot must precede the first provider call)", events, want)
	}
}

// TestPrompt_CheckpointFailureNonFatal (Task 1, Test 5): a failing
// Checkpointer never kills the turn — Prompt completes, returns the
// provider's stop reason, and the failure is observable as a warning.
func TestPrompt_CheckpointFailureNonFatal(t *testing.T) {
	t.Parallel()

	s, m, _ := newTestSession(t, nil, []provider.Response{{FinishReason: stopEndTurn}})
	s.Checkpointer = &recordingCheckpointer{
		log: &seamEventLog{}, err: errors.New("shadow store unwritable"), //nolint:err113 // test fixture error
	}

	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
	if err != nil {
		t.Fatalf("a checkpoint failure must never fail the turn: %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want %q", stop, stopEndTurn)
	}

	// The warning is observable: the transcript carries the investigate-and-
	// fix-ready error line for the checkpoint component.
	lines, rerr := m.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	found := false

	for _, l := range lines {
		if l.Type == TypeError && l.Component == "checkpoint" {
			found = true
		}
	}

	if !found {
		t.Error("no checkpoint failure line in the transcript (the warning must be observable)")
	}
}

// equalStrings is a tiny slice equality helper.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
