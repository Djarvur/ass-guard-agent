package runtime //nolint:testpackage // internal package test (drives unexported sessionFor)

// G-18-1 serve-path opener pins: Manager.AppendSessionStart had ZERO
// production callers, so real sessions never wrote the session_start opener
// and ListSessions skipped every real transcript ("no sessions to resume"
// with live transcripts on disk, ~/tmp/perm-uat). The list fixtures
// hand-wrote the opener, so the whole battery was green against a shape real
// sessions never produce — the G-17-1 fixture-fiction failure mode. These
// pins drive the REAL session-construction path (Runner.sessionFor) and
// assert the on-disk truth: the opener is the transcript's FIRST line on
// creation, never duplicated on reopen, and written into a zero-byte
// transcript (the kill-9-between-create-and-first-append self-heal).

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// openerRunner builds the minimal Runner literal (the session_fallback_test.go
// template) whose sessionFor drives the real construction path.
func openerRunner(workDir string) *Runner {
	return &Runner{
		bus:     event.NewBus(),
		profile: profile.Profile{Name: testProfileName, Model: testModelBefore},
		workDir: workDir,
		maxConc: 2,
		makeProvider: func(provider.RequestCapturer) provider.Provider {
			return newGatedStreamProvider()
		},
	}
}

// openerTranscriptPath is the transcript's on-disk home for id under workDir.
func openerTranscriptPath(workDir, id string) string {
	return filepath.Join(workDir, ".ass-guard", "transcript_"+id+".jsonl")
}

// openerLines reads and parses the whole transcript through the real
// session.Line envelope.
func openerLines(t *testing.T, workDir, id string) []session.Line {
	t.Helper()

	raw, err := os.ReadFile(openerTranscriptPath(workDir, id))
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}

	var lines []session.Line

	for b := range strings.SplitSeq(strings.TrimRight(string(raw), "\n"), "\n") {
		if b == "" {
			continue
		}

		var l session.Line

		uerr := json.Unmarshal([]byte(b), &l)
		if uerr != nil {
			t.Fatalf("parse transcript line %q: %v", b, uerr)
		}

		lines = append(lines, l)
	}

	return lines
}

// TestSessionForWritesOpener asserts the serve path writes the session_start
// opener as the transcript's FIRST line (sessionID in Text), never appends a
// second one on reopen (the resume shape: ResumeSession reopens through
// sessionFor), and self-heals a zero-byte transcript.
func TestSessionForWritesOpener(t *testing.T) { //nolint:cyclop,gocognit,funlen // three assertion pins, one per shape
	t.Parallel()

	const id = "sess_opener-pin"

	t.Run("fresh construction writes opener first", func(t *testing.T) {
		t.Parallel()

		workDir := t.TempDir()

		sess := openerRunner(workDir).sessionFor(context.Background(), id)
		if sess == nil {
			t.Fatal("sessionFor returned nil — construction failed")
		}

		lines := openerLines(t, workDir, id)
		if len(lines) == 0 {
			t.Fatal("transcript is empty — the session_start opener was not written at creation (G-18-1)")
		}

		first := lines[0]
		if first.Type != session.TypeSessionStart {
			t.Fatalf("first line type = %q, want session_start", first.Type)
		}

		if first.Text != id {
			t.Fatalf("opener Text = %q, want the session id %q", first.Text, id)
		}

		if first.Timestamp.IsZero() {
			t.Fatal("opener timestamp is zero")
		}
	})

	t.Run("reopen appends no second opener", func(t *testing.T) {
		t.Parallel()

		workDir := t.TempDir()

		// A SECOND Runner over the same workDir reopens the SAME transcript
		// through sessionFor — the resume shape (ResumeSession → sessionFor).
		for i := range 2 {
			sess := openerRunner(workDir).sessionFor(context.Background(), id)
			if sess == nil {
				t.Fatalf("sessionFor run %d returned nil — construction failed", i)
			}
		}

		lines := openerLines(t, workDir, id)
		count := 0

		for _, l := range lines {
			if l.Type == session.TypeSessionStart {
				count++
			}
		}

		if count != 1 {
			t.Fatalf("session_start lines = %d, want exactly 1 (append-only: reopen never duplicates)", count)
		}

		if lines[0].Type != session.TypeSessionStart {
			t.Fatalf("first line type = %q, want session_start still first", lines[0].Type)
		}
	})

	t.Run("zero-byte transcript self-heals", func(t *testing.T) {
		t.Parallel()

		workDir := t.TempDir()
		store := filepath.Join(workDir, ".ass-guard")

		err := os.MkdirAll(store, 0o750)
		if err != nil {
			t.Fatalf("mkdir store: %v", err)
		}

		// The kill-9-between-create-and-first-append shape: the transcript
		// exists but holds zero bytes.
		err = os.WriteFile(openerTranscriptPath(workDir, id), nil, 0o600)
		if err != nil {
			t.Fatalf("plant zero-byte transcript: %v", err)
		}

		sess := openerRunner(workDir).sessionFor(context.Background(), id)
		if sess == nil {
			t.Fatal("sessionFor returned nil — construction failed")
		}

		lines := openerLines(t, workDir, id)
		if len(lines) != 1 || lines[0].Type != session.TypeSessionStart || lines[0].Text != id {
			t.Fatalf("self-heal wrote %d lines, want exactly the session_start opener first", len(lines))
		}
	})
}
