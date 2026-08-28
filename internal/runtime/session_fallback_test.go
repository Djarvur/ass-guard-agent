package runtime //nolint:testpackage // internal package test (accesses unexported fields)

// 16-REVIEW WR-05 regression pin: when BOTH transcript locations are unusable
// (the work dir AND the temp fallback fail to open), sessionFor returns nil and
// Run surfaces a typed error — never a nil-Manager Session that nil-derefs at
// the first mgr use (the old code discarded the fallback error; the panic was
// recovered per-dispatch into a generic -32603 with the cause swallowed).

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// TestSessionUnavailableWhenTranscriptLocationsFail poisons both transcript
// locations: the work dir path is a FILE (NewManager cannot create the
// transcript there), and TMPDIR points at a file too (the os.TempDir() fallback
// path inherits the poison). NOT parallel: t.Setenv is process-scoped and
// serial tests never overlap the package's parallel tests.
func TestSessionUnavailableWhenTranscriptLocationsFail(t *testing.T) {
	root := t.TempDir()

	blocker := filepath.Join(root, "blocker")

	werr := os.WriteFile(blocker, []byte("x"), 0o600)
	if werr != nil {
		t.Fatalf("write blocker file: %v", werr)
	}

	// TMPDIR points AT the file: the fallback location
	// <TMPDIR>/ass-guard cannot be created (MkdirAll hits a non-directory
	// component), so BOTH transcript locations fail.
	t.Setenv("TMPDIR", blocker)

	runner := &Runner{
		bus:     event.NewBus(),
		profile: profile.Profile{Name: testProfileName, Model: testModelBefore},
		workDir: blocker, // a FILE where the transcript dir would live
		maxConc: 2,
		makeProvider: func(provider.RequestCapturer) provider.Provider {
			return newGatedStreamProvider()
		},
	}

	sess := runner.sessionFor(context.Background(), "s-broken")
	if sess != nil {
		t.Fatal("sessionFor built a session with no usable transcript location — " +
			"the old shape returned a nil-Manager session that panicked at first use")
	}

	// Run surfaces a typed error instead of the nil-deref panic.
	_, rerr := runner.Run(context.Background(), "s-broken", nopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "hi"}})
	if rerr == nil {
		t.Fatal("Run returned no error for an unconstructable session — the turn cannot run without a transcript")
	}
}
