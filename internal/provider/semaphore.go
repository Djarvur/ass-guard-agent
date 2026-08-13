package provider

import (
	"context"
)

// DefaultMaxConcurrent is the default outbound provider-concurrency bound
// (PARA-04 / RESEARCH §11.1). It bounds parent + subagent concurrency combined:
// when the semaphore is full, new dispatches wait. Configurable per-server via
// the `acp serve --max-concurrent` flag (wired in Plan 02-05).
const DefaultMaxConcurrent = 6

// Semaphore bounds outbound concurrency (PARA-04). It is a buffered-channel
// semaphore: Acquire sends a token (or blocks on a full semaphore / returns on
// ctx cancel); Release removes a token. The same semaphore is shared across
// parent turns + subagent dispatches (D-12 — combined concurrency).
type Semaphore struct {
	tokens chan struct{}
}

// NewSemaphore returns a Semaphore with the given max concurrency.
func NewSemaphore(maxConcurrency int) *Semaphore {
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}

	return &Semaphore{tokens: make(chan struct{}, maxConcurrency)}
}

// NewDefaultSemaphore returns a Semaphore bounded at DefaultMaxConcurrent.
func NewDefaultSemaphore() *Semaphore { return NewSemaphore(DefaultMaxConcurrent) }

// Acquire blocks until a token is available or ctx is cancelled. A cancelled
// Acquire does NOT consume a token (the slot remains free for the next caller).
func (s *Semaphore) Acquire(ctx context.Context) error {
	select {
	case s.tokens <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release returns one token. Calling Release without a matching Acquire is a
// programming error (it would allow an extra concurrent holder); callers MUST
// pair Acquire/Release (use defer s.Release() immediately after a successful
// Acquire).
func (s *Semaphore) Release() {
	select {
	case <-s.tokens:
	default:
	}
}

// Max reports the configured concurrency bound (for diagnostics).
func (s *Semaphore) Max() int { return cap(s.tokens) }
