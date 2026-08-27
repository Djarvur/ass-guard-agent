package runtime //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// The 16-05 live-apply seam tests (ACP-08): ApplyTurnModel changes the model
// of the very next provider request, a mid-turn Set waits for the in-flight
// turn (no torn stamp), and sessions created after the change stamp the
// effective model at construction.

// Repeated literals (goconst).
const (
	testProfileName = "test"
	testModelBefore = "model-a"
	testModelAfter  = "model-b"
)

// nopEmitter satisfies acp.ChunkEmitter for turns that stream no chunks.
type nopEmitter struct{}

func (nopEmitter) AgentMessageChunk(string, string) error { return nil }

// gatedStreamProvider records the profile model of every Stream (what the
// Shaper would stamp onto the real request) and holds the FIRST request open
// until the test releases it — the controllable in-flight request.
type gatedStreamProvider struct {
	mu           sync.Mutex
	models       []string
	seen         chan string   // one entry per Stream call, in order
	releaseFirst chan struct{} // closed by the test to free the held request
	heldOnce     sync.Once
}

func newGatedStreamProvider() *gatedStreamProvider {
	return &gatedStreamProvider{
		seen:         make(chan string, 8),
		releaseFirst: make(chan struct{}),
	}
}

func (p *gatedStreamProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{FinishReason: stopEndTurn}, nil
}

func (p *gatedStreamProvider) Stream(
	ctx context.Context, prof *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.mu.Lock()
	first := len(p.models) == 0
	p.models = append(p.models, prof.Model)
	p.mu.Unlock()

	p.seen <- prof.Model

	if first {
		p.heldOnce.Do(func() {})

		select {
		case <-p.releaseFirst:
		case <-ctx.Done():
			return nil, ctx.Err() //nolint:wrapcheck // test fake passthrough
		}
	}

	ch := make(chan provider.StreamChunk, 1)

	ch <- provider.StreamChunk{Type: chunkDone, FinishReason: stopEndTurn}

	close(ch)

	return ch, nil
}

func (p *gatedStreamProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func TestApplyTurnModel(t *testing.T) { //nolint:paralleltest // drives a background turn with real timing
	gated := newGatedStreamProvider()
	runner := newModelTestRunner(t, gated, testModelBefore)

	ctx := context.Background()

	// Sessions stamp the profile's model at construction before any change.
	sess1 := runner.sessionFor(ctx, "s1")
	if sess1.Profile.Model != testModelBefore {
		t.Fatalf("initial session model = %q; want the profile's model", sess1.Profile.Model)
	}

	turnDone := startTestTurn(t, runner, "s1", ctx)

	// Wait until the in-flight request is open (its model recorded).
	awaitSeenModel(t, gated, testModelBefore)

	// A Set arriving MID-TURN must wait: the in-flight request keeps its model.
	applyDone := make(chan error, 1)

	go func() { applyDone <- runner.ApplyTurnModel(testModelAfter) }()

	time.Sleep(150 * time.Millisecond)

	if got := sess1.Profile.Model; got != testModelBefore {
		t.Errorf("mid-turn Set tore the live model: %q (want %q until the turn ends)", got, testModelBefore)
	}

	select {
	case err := <-applyDone:
		t.Fatalf("ApplyTurnModel returned mid-turn (err=%v); want it waiting on the turn mutex", err)
	default:
	}

	// The turn ends; the serialized apply lands BETWEEN turns.
	close(gated.releaseFirst)
	<-turnDone

	select {
	case err := <-applyDone:
		if err != nil {
			t.Fatalf("ApplyTurnModel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ApplyTurnModel never returned after the turn ended")
	}

	if got := sess1.Profile.Model; got != testModelAfter {
		t.Errorf("session model after apply = %q; want %q", got, testModelAfter)
	}

	// The very next request on the SAME session carries the new model.
	_, _ = runner.Run(ctx, "s1", nopEmitter{}, []acp.ContentBlock{{Type: blockText, Text: "again"}})

	awaitSeenModel(t, gated, testModelAfter)
}

// newModelTestRunner builds a Runner wired to the gated provider.
func newModelTestRunner(t *testing.T, gated *gatedStreamProvider, model string) *Runner {
	t.Helper()

	return &Runner{
		bus:     event.NewBus(),
		profile: profile.Profile{Name: testProfileName, Model: model},
		workDir: t.TempDir(),
		maxConc: 2,
		makeProvider: func(provider.RequestCapturer) provider.Provider {
			return gated
		},
	}
}

// startTestTurn drives one background prompt turn and returns its done signal.
//
//nolint:revive // test helper keeps the caller's arg order
func startTestTurn(t *testing.T, runner *Runner, sessionID string, ctx context.Context) chan struct{} {
	t.Helper()

	turnDone := make(chan struct{})

	go func() {
		defer close(turnDone)

		_, _ = runner.Run(ctx, sessionID, nopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "hi"}})
	}()

	return turnDone
}

// awaitSeenModel waits for the provider's next recorded request model and
// asserts it carries the live-applied value.
func awaitSeenModel(t *testing.T, gated *gatedStreamProvider, want string) {
	t.Helper()

	select {
	case got := <-gated.seen:
		if got != want {
			t.Errorf("next request model = %q; want %q (the live apply reached the wire)", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second turn's request never opened")
	}
}

// TestApplyTurnModel_StampsFutureSessions pins the future-sessions leg:
// sessions created after the change stamp the effective model at construction.
func TestApplyTurnModel_StampsFutureSessions(t *testing.T) {
	t.Parallel()

	runner := &Runner{
		bus:     event.NewBus(),
		profile: profile.Profile{Name: testProfileName, Model: testModelBefore},
		workDir: t.TempDir(),
		maxConc: 2,
		makeProvider: func(provider.RequestCapturer) provider.Provider {
			return newGatedStreamProvider()
		},
	}

	ctx := context.Background()

	_ = runner.sessionFor(ctx, "s1")

	aerr := runner.ApplyTurnModel(testModelAfter)
	if aerr != nil {
		t.Fatalf("ApplyTurnModel: %v", aerr)
	}

	sess2 := runner.sessionFor(ctx, "s2")
	if got := sess2.Profile.Model; got != testModelAfter {
		t.Errorf("new session model = %q; want the effective %q stamped at construction", got, testModelAfter)
	}
}
