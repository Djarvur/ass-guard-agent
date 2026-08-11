package loop_test

import (
	"context"
	"errors"
	"testing"

	"github.com/djarvur/ass-guard-agent/internal/loop"
	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/provider"
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
