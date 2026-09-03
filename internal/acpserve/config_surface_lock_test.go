package acpserve //nolint:testpackage // internal package test (reuses config_test fixtures)

// 16-REVIEW WR-03 regression pins: the ConfigSurface mutex must never span the
// blocking operations — the live-apply hook (blocks on every live session's
// turn mutex) and the out-of-band config_option_update send (a blocking send on
// the emitter lane, bounded 128, which blocks while the emitter drain is wedged
// on a slow client). While either is in flight, every OTHER config operation
// (Options() here — the handleInitialize applyMetaBlob path's advertisement
// read) must proceed.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
)

// blockedOptionsProbe runs Options() on a goroutine and fails the test if it
// does not return within the deadline (i.e. it queued behind a blocked
// hook/send holding s.mu — the WR-03 lock-across-blocking-send shape).
func blockedOptionsProbe(t *testing.T, f *surfaceFixture, why string) {
	t.Helper()

	optionsDone := make(chan int, 1)

	go func() { optionsDone <- len(f.surface.Options()) }()

	select {
	case got := <-optionsDone:
		if got != 10 {
			t.Fatalf("Options() returned %d entries; want the ten-entry menu (%s)", got, why)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Options() blocked behind an in-flight config operation — s.mu spans a blocking send/hook (%s)", why)
	}
}

// TestSetApplyHookDoesNotHoldSurfaceMutex pins the Set half of WR-03: the live
// apply runs OUTSIDE s.mu (the persist→apply→notify order is unchanged; the
// lock is not held while the hook waits on the turn mutexes).
func TestSetApplyHookDoesNotHoldSurfaceMutex(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	hookEntered := make(chan struct{})
	releaseHook := make(chan struct{})

	f.surface.SetApplyHook(func(string) error {
		close(hookEntered)
		<-releaseHook

		return nil
	})

	setDone := make(chan error, 1)

	go func() {
		_, err := f.surface.Set("sess-lock", optModel, testModelFallback)

		setDone <- err
	}()

	<-hookEntered // the hook is executing; the persist has already landed

	blockedOptionsProbe(t, f, "in-flight apply hook")

	close(releaseHook)

	select {
	case err := <-setDone:
		if err != nil {
			t.Fatalf("Set: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Set never returned after the apply hook was released")
	}
}

// TestApplyBlobDefaultsNotifyDoesNotHoldSurfaceMutex pins the blob half of
// WR-03: the out-of-band config_option_update send runs OUTSIDE s.mu on frames
// computed under the lock.
func TestApplyBlobDefaultsNotifyDoesNotHoldSurfaceMutex(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	notifyEntered := make(chan struct{})
	releaseNotify := make(chan struct{})

	f.surface.SetNotify(func(_ string, _ []acp.ConfigOptionFrame) {
		close(notifyEntered)
		<-releaseNotify
	})

	blobDone := make(chan error, 1)

	go func() {
		_, err := f.surface.ApplyBlobDefaults(map[string]json.RawMessage{
			optTier: json.RawMessage(`"` + testTierLight + `"`),
		})

		blobDone <- err
	}()

	<-notifyEntered // the blocking send is in flight

	blockedOptionsProbe(t, f, "in-flight config_option_update send")

	close(releaseNotify)

	select {
	case err := <-blobDone:
		if err != nil {
			t.Fatalf("ApplyBlobDefaults: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ApplyBlobDefaults never returned after the notify was released")
	}
}
