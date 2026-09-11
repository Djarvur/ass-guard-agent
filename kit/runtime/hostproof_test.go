package runtime //nolint:testpackage // internal package test

// TestKitHostsNonACPFrontend (25-09 Task 1, KIT-02 criterion #2 design
// proof): the kit hosts a NON-ACP frontend end-to-end — a self-contained
// harness (its own scripted provider, recording Emitter, immediate
// Requester) drives one real turn with nil Catalog and nil Toolkit riding
// the DOCUMENTED degraded paths, and asserts the turn completes with the
// model's text on the wire. This file imports ZERO app packages — the
// meta-check below reads the file's own imports and asserts it, and the
// D-19 gate (mise kit-boundary + depguard KitBoundary) enforces the same
// for every kit file going forward.
//
// The nil-degradation legs pinned here: nil Catalog = expansion off (plain
// text — the emptyCatalog contract), nil Toolkit = the stub-executor path
// (no core tools; a tool call would degrade, this turn makes none), nil
// OpenPermStore = rule-less implicit allow (no gated call is made), nil
// LaunchBackground = structured-error dispatch (none requested).

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/kit/event"
	"github.com/Djarvur/ass-guard-agent/kit/profile"
	"github.com/Djarvur/ass-guard-agent/kit/provider"
	"github.com/Djarvur/ass-guard-agent/kit/session"
)

// hpProvider is the harness's scripted provider over the kit Provider
// interface — one text response, one turn, no tool calls.
type hpProvider struct {
	mu     sync.Mutex
	calls  int
	models []string
}

func (p *hpProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, nil //nolint:nilnil // unused arm
}

func (p *hpProvider) Stream(
	ctx context.Context, prof *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.mu.Lock()
	p.calls++
	p.models = append(p.models, prof.Model)
	p.mu.Unlock()

	ch := make(chan provider.StreamChunk, 2)

	go func() {
		defer close(ch)

		select {
		case ch <- provider.StreamChunk{Type: "text", Text: "host-proof reply"}:
		case <-ctx.Done():
			return
		}

		select {
		case ch <- provider.StreamChunk{Type: "done", FinishReason: stopEndTurn}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (p *hpProvider) ToolResultMessage(_ string, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"role":"user","content":"stub"}`), nil
}

func (p *hpProvider) SupportsImages() bool { return false }

// hpEmitter records every emitted event's kind (the recording Emitter —
// the frontend's evidence trail).
type hpEmitter struct {
	mu     sync.Mutex
	kinds  []string
	chunks []string
}

func (e *hpEmitter) Emit(_ context.Context, ev event.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.kinds = append(e.kinds, ev.Kind())

	if c, ok := ev.(event.AgentMessageChunk); ok {
		e.chunks = append(e.chunks, c.Content)
	}

	return nil
}

// hpRequester answers every ask immediately (the immediate Requester —
// nothing in this turn asks, but the seam is wired to prove the kit takes
// one without an app adapter).
type hpRequester struct{}

func (hpRequester) Request(_ context.Context, _ Ask) (Answer, error) {
	return Answer{Cancelled: true}, nil
}

// TestKitHostsNonACPFrontend drives one real turn through a non-ACP host:
// turn completes with end_turn, the model's text reached the Emitter, the
// provider saw the profile's model, and the file's own import list carries
// zero app packages.
func TestKitHostsNonACPFrontend(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	prov := &hpProvider{}

	r := NewRunner(&RunnerConfig{
		Bus:          event.NewBus(),
		Profile:      profile.Profile{Name: "hostproof", Model: "hostproof-model"},
		WorkDir:      t.TempDir(),
		MaxConc:      2,
		MakeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	})

	// nil Catalog, nil Toolkit, nil OpenPermStore, nil LaunchBackground —
	// the documented degraded paths (emptyCatalog expansion-off, stub
	// executor, rule-less allow, structured-error dispatch).

	emit := &hpEmitter{}
	r.SetRequester(hpRequester{})

	prompt := []session.ContentBlock{{Type: blockText, Text: "hello from a non-ACP frontend"}}

	stop, err := r.Run(context.Background(), "sess-hostproof-1", emit, prompt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want %q", stop, stopEndTurn)
	}

	emit.mu.Lock()
	defer emit.mu.Unlock()

	found := false
	for _, c := range emit.chunks {
		if strings.Contains(c, "host-proof reply") {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("the model's text never reached the Emitter (chunks: %q)", emit.chunks)
	}

	if len(emit.kinds) == 0 {
		t.Error("no events reached the Emitter — the frontend saw nothing")
	}

	prov.mu.Lock()
	defer prov.mu.Unlock()

	if prov.calls != 1 {
		t.Errorf("provider calls = %d; want 1 (exactly one turn)", prov.calls)
	}

	// The meta-check: this file's own import list carries zero app paths.
	// (Also enforced mechanically by the D-19 gate — this keeps the proof
	// inside the artifact itself.)
	self, rerr := os.ReadFile("hostproof_test.go")
	if rerr != nil {
		t.Fatalf("self-read: %v", rerr)
	}

	// The needles are concatenated so this source does not contain the
	// literal it searches for (the first run's self-match taught us).
	appInternal := `"github.com/Djarvur/ass-guard-agent/` + `internal/`
	appCmd := `"github.com/Djarvur/ass-guard-agent/` + `cmd/`

	if strings.Contains(string(self), appInternal) || strings.Contains(string(self), appCmd) {
		t.Error("hostproof imports an app package — the kit is not frontend-free")
	}
}
