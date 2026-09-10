package session //nolint:testpackage // internal package test

// 24-02 (TAIL-01): the live outcome-recording path. One completed provider
// attempt on the REAL turn path (Prompt → runTurn → streamAndEmit) appends
// exactly one mechanical DispatchOutcome to the store; the subagent dispatch
// path (defaultSubagentRunner → streamAndEmitTaggedProf) records its own with
// origin subagent and the effective (possibly overridden) subagent model. A
// broken store never fails a turn: ONE loud stderr note, the turn proceeds
// (T-24-02-01). A nil store (every bare Session construction) records nothing.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// outcomeTestProvider is the scripted provider for the outcome-recording
// battery (the fakeProvider fixture pattern, plus the usage chunk the parent
// stream consumes and a sync Stream error for the failure leg).
type outcomeTestProvider struct {
	usage *provider.Usage // emitted as a "usage" chunk before done (nil = none)
	err   error           // returned synchronously from Stream (nil = stream)
}

func (p *outcomeTestProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{FinishReason: stopEndTurn}, nil
}

func (p *outcomeTestProvider) Stream(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	if p.err != nil {
		return nil, p.err // the sync-open failure path (wrapped "call: %w" by streamAndEmit)
	}

	ch := make(chan provider.StreamChunk, 4)

	go func() {
		defer close(ch)

		ch <- provider.StreamChunk{Type: blockText, Text: "assistant response"}

		if p.usage != nil {
			ch <- provider.StreamChunk{Type: "usage", Usage: p.usage}
		}

		ch <- provider.StreamChunk{Type: stopDone, FinishReason: stopEndTurn}
	}()

	return ch, nil
}

func (p *outcomeTestProvider) ToolResultMessage(_ string, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// SupportsImages: the fake is text-only (21-05 D-11 seam stub).
func (p *outcomeTestProvider) SupportsImages() bool { return false }

// newOutcomeSession builds a Session over the scripted provider with a real
// outcome store under root, wired exactly the way sessionFor wires production
// sessions (Outcomes + ProviderName + SessionTier + the injected clock).
func newOutcomeSession(
	t *testing.T, root string, prov provider.Provider, clock func() time.Time,
) (*Session, *modelrouting.OutcomeStore) {
	t.Helper()

	store, err := modelrouting.NewOutcomeStore(root)
	if err != nil {
		t.Fatalf("NewOutcomeStore: %v", err)
	}

	m := newTestManager(t, "sess-out")
	prof := fakeProfile("outcomes agent")
	prof.Model = "GLM-5.3"

	s := &Session{
		Manager:      m,
		Projector:    NewProjector(prof, m),
		Provider:     prov,
		Bus:          event.NewBus(),
		Semaphore:    provider.NewSemaphore(4),
		Profile:      *prof,
		WorkDir:      root,
		SessionID:    "sess-out",
		Outcomes:     store,
		ProviderName: "anthropic",
		SessionTier:  "heavy",
		outcomeNow:   clock,
	}

	return s, store
}

// readOutcomes reads the store's records (the tolerant reader from 24-01).
func readOutcomes(t *testing.T, store *modelrouting.OutcomeStore) []modelrouting.DispatchOutcome {
	t.Helper()

	records, skipped, err := modelrouting.ReadOutcomes(store.Path())
	if err != nil {
		t.Fatalf("ReadOutcomes: %v", err)
	}

	if skipped != 0 {
		t.Fatalf("skipped = %d; want 0 (every appended line must read back)", skipped)
	}

	return records
}

// TestSessionOutcomeRecording drives the REAL turn path (Prompt → runTurn →
// streamAndEmit) against a t.TempDir store and asserts the exact record
// fields: the real provider name, the effective model, the outcome class from
// the error kind, and the summed usage tokens from the streamed usage chunk.
func TestSessionOutcomeRecording(t *testing.T) {
	t.Parallel()

	// The injected clock (the seam the acceptance criterion pins): a fixed
	// reading makes At deterministic and proves the record routes through it.
	fixedAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixedAt }

	t.Run("success turn records one ok record with summed usage", func(t *testing.T) {
		t.Parallel()

		prov := &outcomeTestProvider{usage: &provider.Usage{InputTokens: 120, OutputTokens: 45}}
		s, store := newOutcomeSession(t, t.TempDir(), prov, clock)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn", stop)
		}

		records := readOutcomes(t, store)
		if len(records) != 1 {
			t.Fatalf("records = %d; want exactly 1 (one per completed provider attempt)", len(records))
		}

		rec := records[0]
		if rec.Provider != "anthropic" {
			t.Errorf("provider = %q; want the real session provider name", rec.Provider)
		}

		if rec.Model != "GLM-5.3" {
			t.Errorf("model = %q; want the effective profile model", rec.Model)
		}

		if rec.Tier != "heavy" {
			t.Errorf("tier = %q; want the session tier", rec.Tier)
		}

		if rec.Outcome != modelrouting.OutcomeOK {
			t.Errorf("outcome = %q; want ok", rec.Outcome)
		}

		if rec.Origin != modelrouting.OutcomeOriginTurn {
			t.Errorf("origin = %q; want turn", rec.Origin)
		}

		if rec.FallbackUsed {
			t.Error("fallback_used = true; want false (the live path walks no fallback)")
		}

		if rec.InTokens != 120 || rec.OutTokens != 45 {
			t.Errorf("tokens = %d/%d; want the summed streamed usage 120/45", rec.InTokens, rec.OutTokens)
		}

		if !rec.At.Equal(fixedAt) {
			t.Errorf("at = %v; want the injected clock reading %v (the now seam)", rec.At, fixedAt)
		}

		if rec.LatencyMS < 0 {
			t.Errorf("latency_ms = %d; want >= 0", rec.LatencyMS)
		}
	})

	t.Run("transient ProviderError records transient", func(t *testing.T) {
		t.Parallel()

		prov := &outcomeTestProvider{err: &provider.ProviderError{
			Kind: provider.KindTransient, Provider: "anthropic", Model: "GLM-5.3",
		}}
		s, store := newOutcomeSession(t, t.TempDir(), prov, clock)

		_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
		if err == nil {
			t.Fatal("Prompt returned nil error; want the surfaced stream error")
		}

		records := readOutcomes(t, store)
		if len(records) != 1 {
			t.Fatalf("records = %d; want exactly 1 (the failed attempt records too)", len(records))
		}

		rec := records[0]
		if rec.Outcome != modelrouting.OutcomeTransient {
			t.Errorf("outcome = %q; want transient (the error kind's class)", rec.Outcome)
		}

		if rec.InTokens != 0 || rec.OutTokens != 0 {
			t.Errorf("tokens = %d/%d; want 0/0 (no usage chunk arrived)", rec.InTokens, rec.OutTokens)
		}
	})

	t.Run("broken store never fails the turn: one loud note", func(t *testing.T) {
		t.Parallel()

		//nolint:paralleltest // swaps the default slog logger (process-global)
		func(t *testing.T) {
			logBuf := swapDefaultLogger(t)

			root := t.TempDir()
			prov := &outcomeTestProvider{usage: &provider.Usage{InputTokens: 10, OutputTokens: 5}}

			s, store := newOutcomeSession(t, root, prov, clock)

			// Break the store's append path deterministically: a DIRECTORY
			// where the JSONL file should be makes every Append OpenFile fail
			// (EISDIR) regardless of uid.
			_ = os.Remove(store.Path())
			if merr := os.Mkdir(store.Path(), 0o750); merr != nil {
				t.Fatalf("break store: %v", merr)
			}

			stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
			if err != nil {
				t.Fatalf("Prompt failed on a broken store: %v (a store failure must never fail the turn)", err)
			}

			if stop != stopEndTurn {
				t.Fatalf("stop = %q; want end_turn (the turn completed)", stop)
			}

			if n := strings.Count(logBuf.String(), "outcome store append failed"); n != 1 {
				t.Errorf("stderr notes naming the failed outcome append = %d; want exactly 1 (log = %q)", n, logBuf.String())
			}
		}(t)
	})

	t.Run("subagent dispatch records origin subagent with the override model", func(t *testing.T) {
		t.Parallel()

		prov := &outcomeTestProvider{}
		s, store := newOutcomeSession(t, t.TempDir(), prov, clock)

		// The 20-03 planner seam: this dispatch's resolved (overridden) model
		// is what the record must carry — not the parent profile's.
		s.SubagentModelPlanner = func(*Session, *ecosys.Agent, string) SubagentDispatchPlan {
			return SubagentDispatchPlan{Model: "glm-5.2-light"}
		}

		result, err := s.DispatchSubagent(context.Background(), "turn-par", "call-sub", "do the thing", nil)
		if err != nil {
			t.Fatalf("DispatchSubagent: %v", err)
		}

		if result != "assistant response" {
			t.Errorf("result = %q; want the scripted stream text", result)
		}

		records := readOutcomes(t, store)
		if len(records) != 1 {
			t.Fatalf("records = %d; want exactly 1 (one per subagent dispatch attempt)", len(records))
		}

		rec := records[0]
		if rec.Origin != modelrouting.OutcomeOriginSubagent {
			t.Errorf("origin = %q; want subagent", rec.Origin)
		}

		if rec.Model != "glm-5.2-light" {
			t.Errorf("model = %q; want the effective (overridden) subagent model", rec.Model)
		}

		if rec.Outcome != modelrouting.OutcomeOK {
			t.Errorf("outcome = %q; want ok", rec.Outcome)
		}
	})

	t.Run("nil store records nothing", func(t *testing.T) {
		t.Parallel()

		prov := &outcomeTestProvider{}
		s, _ := newOutcomeSession(t, t.TempDir(), prov, clock)
		s.Outcomes = nil // every bare Session construction site

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn", stop)
		}
	})
}
