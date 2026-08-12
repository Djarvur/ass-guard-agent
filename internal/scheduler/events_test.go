package scheduler

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// TestProviderFallbackKind asserts the discriminator string.
func TestProviderFallbackKind(t *testing.T) {
	pf := ProviderFallback{
		TurnID: "t1", FromProvider: "anthropic", FromModel: "glm-5.2",
		ToProvider: "openai", ToModel: "minimax-m3",
		Reason: "Transient: HTTP 429", ErrorKind: provider.KindTransient, Attempt: 1,
	}
	require.Equal(t, "ProviderFallback", pf.Kind())
}

// TestCostCeilingWarnKind asserts the discriminator string.
func TestCostCeilingWarnKind(t *testing.T) {
	cc := CostCeilingWarn{
		TurnID: "t1", Window: "24h ending 2026-08-09T00:00:00Z",
		Spent: 50.0, Ceiling: 50.0, DegradedTo: "light", HardStop: false,
	}
	require.Equal(t, "CostCeilingWarn", cc.Kind())
}

// TestEventsBusRoundTrip asserts the two new event kinds route through the real
// Phase-2 event bus: a Published ProviderFallback is received by a subscriber
// of "ProviderFallback" and type-asserts back to the original struct with all
// fields intact (D-06 → ACP session/update path).
func TestEventsBusRoundTrip(t *testing.T) {
	bus := event.NewBus()
	defer bus.Close()

	ch := bus.Subscribe("ProviderFallback", 4)

	pf := ProviderFallback{
		TurnID: "turn-42", FromProvider: "anthropic", FromModel: "glm-5.2",
		ToProvider: "openai", ToModel: "minimax-m3",
		Reason: "Transient: HTTP 429", ErrorKind: provider.KindTransient, Attempt: 1,
	}
	bus.Publish(pf)

	select {
	case got := <-ch:
		require.Equal(t, "ProviderFallback", got.Kind())
		back, ok := got.(ProviderFallback)
		require.True(t, ok, "received event must type-assert back to ProviderFallback")
		require.Equal(t, pf, back, "all fields must survive the round-trip")
	default:
		t.Fatal("ProviderFallback was not received by the subscriber")
	}
}

// TestEventsBusCostCeilingRoundTrip is the warn event's bus round-trip.
func TestEventsBusCostCeilingRoundTrip(t *testing.T) {
	bus := event.NewBus()
	defer bus.Close()

	ch := bus.Subscribe("CostCeilingWarn", 4)

	cc := CostCeilingWarn{
		TurnID: "turn-7", Window: "24h",
		Spent: 51.0, Ceiling: 50.0, DegradedTo: "light", HardStop: false,
	}
	bus.Publish(cc)

	select {
	case got := <-ch:
		require.Equal(t, "CostCeilingWarn", got.Kind())
		back, ok := got.(CostCeilingWarn)
		require.True(t, ok)
		require.Equal(t, cc, back)
	default:
		t.Fatal("CostCeilingWarn was not received by the subscriber")
	}
}
