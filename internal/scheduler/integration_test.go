package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// TestSchedulerEndToEnd drives the full scheduler as one system: load config →
// validate → construct Scheduler with real breakers + cost + a fake provider →
// Dispatch → observe the ProviderFallback event on the bus + the breaker
// advancing for the failed primary. This is the phase-goal promise: the
// developer selects a tier; the operator manages everything via config.
func TestSchedulerEndToEnd(t *testing.T) {
	t.Parallel()
	cfg := loadValid(t)
	bus := event.NewBus()

	t.Cleanup(func() { bus.Close() })

	fp := newFakeProvider().
		set("glm-5.2", fakeOutcome{err: transientErr("glm-5.2", 429)}).
		set("minimax-m3", fakeOutcome{resp: provider.Response{FinishReason: "stop"}})

	s := NewScheduler(cfg, bus, nil, map[string]provider.Provider{
		"anthropic": fp, "openai": fp, "groq": fp,
	}, nil)
	s.InstallSafety(cfg, bus)
	s.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) }) // Sunday noon → global table

	ch := bus.Subscribe("ProviderFallback", 8)
	ctx := WithTurnID(context.Background(), "turn-end-to-end")
	resp, err := s.Dispatch(ctx, "heavy", "myproj", CapabilityReq{NeedsTools: true}, profile.Profile{}, nil)

	// The fallback (minimax-m3) succeeded — Dispatch returns its response.
	require.NoError(t, err)
	require.Equal(t, "stop", resp.FinishReason)

	// Exactly one ProviderFallback event (glm-5.2 → minimax-m3), tagged with the
	// turn id, carrying the transient kind.
	select {
	case e := <-ch:
		pf, ok := e.(ProviderFallback)
		require.True(t, ok)
		require.Equal(t, "turn-end-to-end", pf.TurnID)
		require.Equal(t, "glm-5.2", pf.FromModel)
		require.Equal(t, "minimax-m3", pf.ToModel)
		require.Equal(t, provider.KindTransient, pf.ErrorKind)
	default:
		t.Fatal("expected a ProviderFallback event on the bus")
	}

	// The failed primary's breaker advanced (state persists for the next dispatch).
	cb := s.Breaker("anthropic", "glm-5.2")
	require.NotNil(t, cb)
	require.Equal(t, 1, consecutiveOf(cb), "breaker recorded the transient on the primary")

	// The capability gate is honored end-to-end: a tool-using turn never reached
	// a tool-less model. (Here both heavy candidates have tools; the assertion is
	// that the tool-capable fallback was the one called.)
	require.Contains(t, fp.calledModels(), "minimax-m3")
}
