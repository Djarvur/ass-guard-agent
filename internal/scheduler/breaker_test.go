package scheduler

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// newTestBreaker builds a Closed breaker with the documented D-07 defaults and a
// captured stderr log buffer.
func newTestBreaker(t *testing.T, key providerModelKey, cfg CircuitBreakerConfig) (*CircuitBreaker, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer

	log := newCapturingLogger(&buf)

	if cfg == (CircuitBreakerConfig{}) {
		cfg = defaultBreaker
	}

	return NewCircuitBreaker(key, cfg, log), &buf
}

func newCapturingLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

// TestBreakerTripViaConsecutive: N=5 consecutive Transient failures trip Closed
// → Open; Allow returns false; a Warn line is emitted.
func TestBreakerTripViaConsecutive(t *testing.T) {
	t.Parallel()
	b, buf := newTestBreaker(t, providerModelKey{"anthropic", "glm-5.2"}, defaultBreaker)

	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)

	for i := range 4 {
		b.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
		require.True(t, b.Allow(now), "still Closed after %d transients (below N=5)", i+1)
	}
	// 5th transient trips it.
	b.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
	require.Equal(t, breakerOpen, b.State(), "5 consecutive transients must trip Open")
	require.False(t, b.Allow(now), "Open breaker rejects")
	require.Contains(t, buf.String(), "circuit breaker transition", "Warn line emitted")
	require.Contains(t, buf.String(), "trip")
}

// TestBreakerTripViaErrorRate: fill the M=20 window with a >50% failure rate
// while keeping consecutive < N=5, so the trip MUST be via the error-rate
// mechanism (not consecutive). Confirms the OR + the "rate needs a full M-window
// before it is consulted" guard (a near-empty window cannot trip on rate alone,
// RESEARCH §6.1 "over the last M requests").
func TestBreakerTripViaErrorRate(t *testing.T) {
	t.Parallel()

	cfg := CircuitBreakerConfig{ConsecutiveFailures: 5, ErrorRateWindow: 20, ErrorRateThreshold: 0.50, Cooldown: 60 * time.Second, HalfOpenProbes: 1}
	b, _ := newTestBreaker(t, providerModelKey{"p", "m"}, cfg)
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)

	// 9 (Transient, Success) pairs = 18 calls. Window not full (18 < 20); rate
	// not consulted. consecutive stays ≤ 1 (each S resets it). Stays Closed.
	for range 9 {
		b.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
		b.RecordSuccess()
	}

	require.Equal(t, breakerClosed, b.State(), "rate not consulted until the M-window is full")
	// Two more Transients → window full (20): 11T + 9S = 0.55 > 0.50; consecutive
	// = 2 (the last two were both T, previous was S). 2 < 5, so consecutive did
	// NOT fire — the trip is via error-rate.
	b.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
	require.Equal(t, breakerClosed, b.State(), "window=19 not yet full, rate still not consulted")
	b.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
	require.Equal(t, breakerOpen, b.State(), "window full (20) + 0.55 rate > 0.50 → trip via error-rate")
}

// TestBreakerSuccessResetsConsecutive: 3 consecutive failures (below N=5) then a
// RecordSuccess resets consecutive; breaker stays Closed.
func TestBreakerSuccessResetsConsecutive(t *testing.T) {
	t.Parallel()
	b, _ := newTestBreaker(t, providerModelKey{"p", "m"}, defaultBreaker)

	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)

	for range 3 {
		b.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
	}

	require.Equal(t, breakerClosed, b.State())
	b.RecordSuccess()
	// 4 more transients would have tripped if consecutive had stayed at 3; it's 0 now.
	for range 4 {
		b.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
	}

	require.Equal(t, breakerClosed, b.State(), "consecutive reset by success — 4 more is below N=5")
}

// TestBreakerOpenToHalfOpenAfterCooldown: trip at t0; Allow(t0+30s) still false
// (cooldown 60s not elapsed); Allow(t0+61s) → HalfOpen and returns true.
func TestBreakerOpenToHalfOpenAfterCooldown(t *testing.T) {
	t.Parallel()
	b, _ := newTestBreaker(t, providerModelKey{"p", "m"}, defaultBreaker)

	t0 := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)

	for range 5 {
		b.RecordTransient(t0, &provider.ProviderError{Kind: provider.KindTransient})
	}

	require.Equal(t, breakerOpen, b.State())

	require.False(t, b.Allow(t0.Add(30*time.Second)), "cooldown not elapsed")
	require.Equal(t, breakerOpen, b.State())

	require.True(t, b.Allow(t0.Add(61*time.Second)), "cooldown elapsed — admit probe")
	require.Equal(t, breakerHalfOpen, b.State(), "Open → HalfOpen after cooldown")
}

// TestBreakerHalfOpenToClosedOnSuccess: after HalfOpen, RecordSuccess → Closed.
func TestBreakerHalfOpenToClosedOnSuccess(t *testing.T) {
	t.Parallel()
	b, buf := newTestBreaker(t, providerModelKey{"p", "m"}, defaultBreaker)

	t0 := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)

	for range 5 {
		b.RecordTransient(t0, &provider.ProviderError{Kind: provider.KindTransient})
	}

	require.True(t, b.Allow(t0.Add(61*time.Second)))
	require.Equal(t, breakerHalfOpen, b.State())

	b.RecordSuccess()
	require.Equal(t, breakerClosed, b.State(), "probe success → Closed")
	require.True(t, b.Allow(t0.Add(61*time.Second)))
	require.Contains(t, buf.String(), "recovered", "recovery logged")
}

// TestBreakerHalfOpenToOpenOnFailure: after HalfOpen, RecordTransient → Open
// with openedAt reset (cooldown restarts).
func TestBreakerHalfOpenToOpenOnFailure(t *testing.T) {
	t.Parallel()
	b, buf := newTestBreaker(t, providerModelKey{"p", "m"}, defaultBreaker)

	t0 := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)

	for range 5 {
		b.RecordTransient(t0, &provider.ProviderError{Kind: provider.KindTransient})
	}

	require.True(t, b.Allow(t0.Add(61*time.Second)))
	require.Equal(t, breakerHalfOpen, b.State())

	probeFailureTime := t0.Add(70 * time.Second)
	b.RecordTransient(probeFailureTime, &provider.ProviderError{Kind: provider.KindTransient})
	require.Equal(t, breakerOpen, b.State(), "probe failure → re-Open")
	// Cooldown restarted from probeFailureTime; t0+61s is BEFORE probeFailureTime,
	// but more importantly 30s after probeFailureTime is still in cooldown.
	require.False(t, b.Allow(probeFailureTime.Add(30*time.Second)), "cooldown restarted after probe failure")
	require.Contains(t, buf.String(), "probe failed", "re-open logged")
}

// TestBreakerNoRecordStructural: the Breaker interface has NO RecordStructural
// method — only Transient feeds the breaker (pitfall 4). A breaker that never
// sees RecordTransient stays Closed forever.
func TestBreakerNoRecordStructural(t *testing.T) {
	t.Parallel()
	b, _ := newTestBreaker(t, providerModelKey{"p", "m"}, defaultBreaker)
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	// Simulate 100 structural failures — Dispatch never calls Record* for them,
	// so the breaker sees nothing and stays Closed.
	for range 100 {
		require.True(t, b.Allow(now))
	}

	require.Equal(t, breakerClosed, b.State(), "Structural outcomes never feed the breaker")
}

// TestBreakerConcurrency: 100 goroutines each call RecordSuccess /
// RecordTransient 50× on ONE breaker; under -race no race is detected and the
// final state is consistent (one of the three states; window length bounded by
// M). This is the load-bearing concurrency guarantee (pitfall 6).
func TestBreakerConcurrency(t *testing.T) {
	t.Parallel()
	b, _ := newTestBreaker(t, providerModelKey{"p", "m"}, defaultBreaker)
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for g := range 100 {
		wg.Add(1)

		go func(seed int) {
			defer wg.Done()

			for i := range 50 {
				if (i+seed)%2 == 0 {
					b.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
				} else {
					b.RecordSuccess()
				}
			}
		}(g)
	}

	wg.Wait()
	// Consistency: state is one of the three valid values; window bounded by M.
	state := b.State()
	require.Contains(t, []breakerState{breakerClosed, breakerOpen, breakerHalfOpen}, state)
	b.mu.Lock()
	require.GreaterOrEqual(t, b.consecutive, 0)
	require.LessOrEqual(t, b.window.len(), defaultBreaker.ErrorRateWindow, "window never exceeds M")
	b.mu.Unlock()
}

// TestBreakerPerKeyIsolation: tripping breaker A does not affect breaker B's
// Allow (per-(provider,model) isolation, D-07).
func TestBreakerPerKeyIsolation(t *testing.T) {
	t.Parallel()
	a, _ := newTestBreaker(t, providerModelKey{"anthropic", "glm-5.2"}, defaultBreaker)
	b, _ := newTestBreaker(t, providerModelKey{"openai", "minimax-m3"}, defaultBreaker)

	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	for range 5 {
		a.RecordTransient(now, &provider.ProviderError{Kind: provider.KindTransient})
	}

	require.Equal(t, breakerOpen, a.State())
	require.True(t, b.Allow(now), "tripping A must not affect B")
	require.Equal(t, breakerClosed, b.State())
}

// TestRingBufferBounds checks the ring's capacity + error-rate math.
func TestRingBufferBounds(t *testing.T) {
	t.Parallel()

	r := newRingBuffer(3)
	require.Equal(t, 0.0, r.errorRate())
	r.push(false)
	r.push(true)
	r.push(false) // full: [F,T,F] → 2/3
	require.Equal(t, 3, r.len())
	require.InDelta(t, 2.0/3.0, r.errorRate(), 1e-9)
	r.push(true) // evicts oldest (F): [T,T,F] (idx wrapped) → 1/3
	require.Equal(t, 3, r.len())
	require.InDelta(t, 1.0/3.0, r.errorRate(), 1e-9)
}

// keep strings import used (for Contains helpers above) explicit in case of
// future trim.
var _ = strings.TrimSpace
