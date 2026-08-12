package provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// TestSemaphore_AllowsMaxConcurrent verifies a Semaphore(max) lets max Acquires
// succeed and blocks the (max+1)th until a Release (PARA-04).
func TestSemaphore_AllowsMaxConcurrent(t *testing.T) {
	sem := provider.NewSemaphore(2)
	ctx := context.Background()
	if err := sem.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	if err := sem.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	// Third acquire must block.
	done := make(chan error, 1)
	go func() { done <- sem.Acquire(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("third Acquire returned %v before a Release; want it to block", err)
	case <-time.After(50 * time.Millisecond):
		// good: blocked
	}
	// Release one; the blocked acquire should complete.
	sem.Release()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("blocked Acquire returned %v after Release; want nil", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocked Acquire did not complete after Release")
	}
	sem.Release()
}

// TestSemaphore_Cancel verifies Acquire on a full semaphore returns ctx.Err()
// when ctx is cancelled (D-16 — cancellation aborts waiting dispatches).
func TestSemaphore_Cancel(t *testing.T) {
	sem := provider.NewSemaphore(1)
	ctx := context.Background()
	if err := sem.Acquire(ctx); err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- sem.Acquire(cctx) }()
	select {
	case <-done:
		t.Fatal("second Acquire returned before cancel; want it to block")
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Acquire returned %v; want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Acquire did not return after ctx cancel")
	}
	// The cancelled acquire did NOT consume a slot; releasing the first leaves
	// the semaphore empty.
	sem.Release()
	if err := sem.Acquire(ctx); err != nil {
		t.Errorf("Acquire after cancel+release returned %v; want nil (slot must be free)", err)
	}
	sem.Release()
}

// TestSemaphore_DefaultMax verifies the default constructor matches the
// configured default (6 — PARA-04 / RESEARCH §11.1).
func TestSemaphore_DefaultMax(t *testing.T) {
	sem := provider.NewDefaultSemaphore()
	ctx := context.Background()
	for i := 0; i < provider.DefaultMaxConcurrent; i++ {
		if err := sem.Acquire(ctx); err != nil {
			t.Fatalf("Acquire %d: %v", i+1, err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- sem.Acquire(ctx) }()
	select {
	case <-done:
		t.Fatalf("Acquire beyond DefaultMaxConcurrent returned; want block")
	case <-time.After(30 * time.Millisecond):
	}
}
