package scheduler

import (
	"log/slog"
	"sync"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// breakerState is the internal state of a CircuitBreaker (sony/gobreaker
// distilled in-process, RESEARCH §6.1 — study, NOT a dep).
type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

func (s breakerState) String() string {
	switch s {
	case breakerClosed:
		return "Closed"
	case breakerOpen:
		return "Open"
	case breakerHalfOpen:
		return "HalfOpen"
	}

	return "Unknown"
}

// CircuitBreaker is the per-(provider, model) Closed/Open/HalfOpen state machine
// (D-07). It implements the Breaker interface. Two trip mechanisms OR together:
// consecutive-failure trips FAST on hard outage (N consecutive Transient
// failures), error-rate trips SLOW on degraded performance (> threshold over
// the last M requests — checked only once the M-window is full so a single
// failure on a near-empty window cannot trip). After tripping, the breaker is
// Open for Cooldown; then a HalfOpen probe tests recovery (success → Closed,
// failure → Open).
//
// Only Transient failures feed the breaker — there is intentionally no method
// to record Structural outcomes (a 401 is a config bug, not an outage, pitfall
// 4). The mutex is held ONLY across Allow/RecordSuccess/RecordTransient (state
// R/W) — NEVER across the provider call (pitfall 6); parent + subagents hit the
// same breaker concurrently.
type CircuitBreaker struct {
	key         providerModelKey
	mu          sync.Mutex
	state       breakerState
	consecutive int
	window      *ringBuffer
	openedAt    time.Time
	cfg         CircuitBreakerConfig
	log         *slog.Logger
}

// NewCircuitBreaker constructs a Closed breaker for one (provider, model). The
// cfg carries the D-07 thresholds (applied defaults are already filled by Load).
func NewCircuitBreaker(key providerModelKey, cfg CircuitBreakerConfig, log *slog.Logger) *CircuitBreaker {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}

	if cfg.ConsecutiveFailures < 1 {
		cfg.ConsecutiveFailures = defaultBreaker.ConsecutiveFailures
	}

	if cfg.ErrorRateWindow < 1 {
		cfg.ErrorRateWindow = defaultBreaker.ErrorRateWindow
	}

	if cfg.ErrorRateThreshold <= 0 {
		cfg.ErrorRateThreshold = defaultBreaker.ErrorRateThreshold
	}

	if cfg.Cooldown <= 0 {
		cfg.Cooldown = defaultBreaker.Cooldown
	}

	return &CircuitBreaker{
		key:    key,
		state:  breakerClosed,
		window: newRingBuffer(cfg.ErrorRateWindow),
		cfg:    cfg,
		log:    log,
	}
}

// Allow reports whether a candidate should be attempted. Closed always allows;
// HalfOpen admits the probe; Open allows only after Cooldown has elapsed (and
// transitions to HalfOpen so the next outcome can close or re-open).
func (b *CircuitBreaker) Allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case breakerClosed:
		return true
	case breakerHalfOpen:
		return true
	case breakerOpen:
		if now.Sub(b.openedAt) >= b.cfg.Cooldown {
			b.transition(breakerHalfOpen, now, "cooldown elapsed — probe")

			return true
		}

		return false
	}

	return true
}

// RecordSuccess resets the consecutive counter and pushes a success into the
// window. From HalfOpen, a success fully recovers the breaker to Closed.
func (b *CircuitBreaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.consecutive = 0
	b.window.push(true)

	if b.state == breakerHalfOpen {
		b.transition(breakerClosed, time.Time{}, "probe succeeded — recovered")
	}
}

// RecordTransient records a Transient failure and trips the breaker if either
// mechanism fires: consecutive >= N OR (once the M-window is full) error-rate >
// threshold. From HalfOpen, a single failure re-opens the breaker (cooldown
// restarts).
//
// There is intentionally no method to record Structural outcomes — Structural
// errors are config bugs, not outages, and must not feed the breaker (pitfall 4).
func (b *CircuitBreaker) RecordTransient(now time.Time, err *provider.ProviderError) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.consecutive++
	b.window.push(false)

	if b.state == breakerHalfOpen {
		b.openedAt = now
		b.transition(breakerOpen, now, "probe failed — re-opened")

		return
	}

	if b.state == breakerClosed {
		rateTrips := false
		if b.window.len() >= b.cfg.ErrorRateWindow && b.window.errorRate() > b.cfg.ErrorRateThreshold {
			rateTrips = true
		}

		if b.consecutive >= b.cfg.ConsecutiveFailures || rateTrips {
			b.openedAt = now
			b.transition(breakerOpen, now, "trip")

			return
		}
	}
}

// transition moves to the target state and logs a Warn line with full
// diagnostic detail (C5 — investigate-and-fix-ready). Caller already holds b.mu.
func (b *CircuitBreaker) transition(to breakerState, now time.Time, reason string) {
	from := b.state
	b.state = to

	cooldownRemaining := time.Duration(0)

	if to == breakerOpen {
		cooldownRemaining = b.cfg.Cooldown
	} else if from == breakerOpen && now.After(b.openedAt) {
		cooldownRemaining = max(b.cfg.Cooldown-now.Sub(b.openedAt), 0)
	}

	b.log.Warn("scheduler: circuit breaker transition",
		"provider", b.key.Provider,
		keyModel, b.key.Model,
		"from", from.String(),
		"to", to.String(),
		"reason", reason,
		"consecutive", b.consecutive,
		"error_rate", b.window.errorRate(),
		"cooldown_remaining", cooldownRemaining.String(),
	)
}

// State returns the current state (test/diagnostic helper).
func (b *CircuitBreaker) State() breakerState {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.state
}

// ringBuffer is a fixed-capacity ring of success/failure booleans backing the
// error-rate mechanism (D-07 mechanism 2). Once full, the oldest entry is
// evicted on push. errorRate = failures / len.
type ringBuffer struct {
	cap int
	buf []bool
	idx int // next write position once full
}

func newRingBuffer(capacity int) *ringBuffer {
	if capacity < 1 {
		capacity = 1
	}

	return &ringBuffer{cap: capacity, buf: make([]bool, 0, capacity)}
}

func (r *ringBuffer) push(v bool) {
	if len(r.buf) < r.cap {
		r.buf = append(r.buf, v)

		return
	}

	r.buf[r.idx] = v
	r.idx = (r.idx + 1) % r.cap
}

func (r *ringBuffer) errorRate() float64 {
	if len(r.buf) == 0 {
		return 0
	}

	failures := 0

	for _, b := range r.buf {
		if !b {
			failures++
		}
	}

	return float64(failures) / float64(len(r.buf))
}

func (r *ringBuffer) len() int { return len(r.buf) }
