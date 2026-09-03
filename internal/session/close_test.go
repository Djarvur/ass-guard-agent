package session //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closeRecorder is a fake OnClose hook that counts Close calls (stands in for
// an MCP host).
type closeRecorder struct {
	count atomic.Int32
}

func (c *closeRecorder) close() error {
	c.count.Add(1)

	return nil
}

// newCloseTestSession builds a minimal Session wired with a scripted provider
// and an OnClose hook (the MCP-host reap seam). The provider's script is
// replayed across Prompt calls.
func newCloseTestSession(
	t *testing.T, id string, script []provider.Response, bus *event.Bus,
) (*Session, *closeRecorder) {
	t.Helper()

	m := newTestManager(t, id)
	rec := &closeRecorder{}
	fp := &reconProvider{script: script, bus: bus}

	s := &Session{
		Manager: m, Projector: NewProjector(fakeProfile("test"), m),
		Provider: fp, Bus: bus, Semaphore: provider.NewSemaphore(2),
		Profile: *fakeProfile("test"), WorkDir: t.TempDir(), SessionID: id,
		Catalog: toolcat.NewCatalog(),
		OnClose: rec.close,
	}

	return s, rec
}

// TestSessionCloseRunsOnClose proves Session.Close runs the OnClose hook exactly
// once (the MCP-host reap seam — Plan 05-01/05-02 T4).
func TestSessionCloseRunsOnClose(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	s, rec := newCloseTestSession(t, "sess-close", nil, bus)

	require.NoError(t, s.Close())
	assert.Equal(t, int32(1), rec.count.Load(), "OnClose must run exactly once")

	// Idempotent: a second Close does not re-run the hook.
	require.NoError(t, s.Close())
	assert.Equal(t, int32(1), rec.count.Load(), "OnClose must not run twice (idempotent)")
}

// TestSessionCloseNilOnClose proves Close works with no OnClose hook (a session
// that does not host MCP servers).
func TestSessionCloseNilOnClose(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "sess-nil")
	s := &Session{
		Manager: m, Projector: NewProjector(fakeProfile("test"), m),
		Profile: *fakeProfile("test"), SessionID: "sess-nil", Catalog: toolcat.NewCatalog(),
	}

	require.NoError(t, s.Close()) // nil OnClose is a no-op
}

// TestPromptPanicKeepsSession proves a recovered turn panic does NOT close the
// session — the session survives for the next turn (Plan 05-02 T4 Test 4).
func TestPromptPanicKeepsSession(t *testing.T) { //nolint:paralleltest // mutates shared scriptedProvider state
	// A provider whose Stream panics.
	s, rec := newCloseTestSession(t, "sess-panic", nil, event.NewBus())
	s.Provider = &panicProvider{}

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
	require.Error(t, err, "a turn panic must surface as an error, not a crash")

	// The OnClose hook must NOT have fired (session survives for the next turn).
	assert.Equal(t, int32(0), rec.count.Load(), "a recovered panic must not close the session")

	// The session is still usable: Close works (reap on session END only).
	require.NoError(t, s.Close())
	assert.Equal(t, int32(1), rec.count.Load(), "Close runs OnClose at session end")
}

// TestSessionCloseConcurrentSafe proves concurrent Close calls are safe (the
// closeOnce guard).
func TestSessionCloseConcurrentSafe(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	s, rec := newCloseTestSession(t, "sess-conc", nil, bus)

	done := make(chan struct{})

	go func() {
		_ = s.Close()

		close(done)
	}()

	<-done

	require.NoError(t, s.Close())
	assert.Equal(t, int32(1), rec.count.Load())
}

// panicProvider is a provider whose Stream panics (to exercise the turn-recover
// path without closing the session).
type panicProvider struct{}

func (panicProvider) Send(_ context.Context, _ *profile.Profile, _ []provider.Message) (provider.Response, error) {
	return provider.Response{}, nil
}

func (panicProvider) Stream(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	panic("simulated turn panic")
}

func (panicProvider) ToolResultMessage(_ string, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// SupportsImages: the fake is text-only (21-05 D-11 seam stub).
func (panicProvider) SupportsImages() bool { return false }
