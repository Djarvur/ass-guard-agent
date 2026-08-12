package loop_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/loop"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// fakeProvider is a Provider stub that returns a canned tool-call list, proving
// the loop is single-turn and provider-agnostic (D-12).
type fakeProvider struct {
	calls []provider.ToolCall
	err   error
}

func (f *fakeProvider) Send(ctx context.Context, _ profile.Profile, _ []provider.Message) (provider.Response, error) {
	if f.err != nil {
		return provider.Response{}, f.err
	}
	return provider.Response{ToolCalls: f.calls, FinishReason: "tool_use"}, nil
}

func (f *fakeProvider) ToolResultMessage(toolCallID string, result json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"role":"user","content":"stub"}`), nil
}

// Stream satisfies the Phase-2-expanded Provider interface. The Phase-1 test
// harness loop only exercises Send; Stream is a stub that returns a non-streamed
// error so it is never accidentally used here.
func (f *fakeProvider) Stream(ctx context.Context, _ profile.Profile, _ []provider.Message) (<-chan provider.StreamChunk, error) {
	return nil, errors.New("fakeProvider: Stream not used by the Phase-1 test-harness loop")
}

func TestRun_ReturnsProviderToolCalls(t *testing.T) {
	want := []provider.ToolCall{{Name: "Read", Input: []byte(`{"file_path":"go.mod"}`)}}
	p := &fakeProvider{calls: want}
	got, err := loop.Run(context.Background(), profile.Profile{}, p, "read go.mod")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Read" {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestRun_PropagatesProviderError(t *testing.T) {
	p := &fakeProvider{err: errors.New("boom")}
	if _, err := loop.Run(context.Background(), profile.Profile{}, p, "x"); err == nil {
		t.Fatal("Run returned nil error; want the provider error propagated")
	}
}
