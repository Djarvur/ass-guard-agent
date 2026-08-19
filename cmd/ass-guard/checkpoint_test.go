package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
)

// seedCheckpointStore opens a store over a seeded temp workspace and takes
// one snapshot, returning the store.
func seedCheckpointStore(t *testing.T, work string) *checkpoint.Store {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(work, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(filepath.Join(work, "src", "app.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatalf("write src/app.txt: %v", err)
	}

	s, err := checkpoint.Open(work)
	if err != nil {
		t.Fatalf("checkpoint.Open: %v", err)
	}

	if err := s.Snapshot(context.Background(), "sess-cli", "sess-cli-turn-001"); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	return s
}

// TestCheckpointListEmptyStore verifies an empty (absent) store prints
// "no checkpoints" to stderr and exits 0.
func TestCheckpointListEmptyStore(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	err := runCheckpointList(&out, t.TempDir())
	if err != nil {
		t.Fatalf("runCheckpointList on empty store: %v", err)
	}

	if !strings.Contains(out.String(), "no checkpoints") {
		t.Errorf("output = %q; want the no-checkpoints note", out.String())
	}
}

// TestCheckpointListShowsSnapshots verifies list output carries the turn
// ids, oldest first, on the provided (stderr) writer.
func TestCheckpointListShowsSnapshots(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	s := seedCheckpointStore(t, work)

	if err := s.Snapshot(context.Background(), "sess-cli", "sess-cli-turn-002"); err != nil {
		t.Fatalf("Snapshot 2: %v", err)
	}

	var out bytes.Buffer

	if err := runCheckpointList(&out, work); err != nil {
		t.Fatalf("runCheckpointList: %v", err)
	}

	got := out.String()
	first := strings.Index(got, "sess-cli-turn-001")
	second := strings.Index(got, "sess-cli-turn-002")

	if first < 0 || second < 0 {
		t.Fatalf("output = %q; want both turn ids", got)
	}

	if first > second {
		t.Errorf("output = %q; turn 001 must list before turn 002", got)
	}
}

// TestCheckpointRestoreRoundtripCLI verifies the restore subcommand's body:
// after a mutating turn, runCheckpointRestore returns the workspace to the
// snapshot's bytes and reports the restored ref on stderr.
func TestCheckpointRestoreRoundtripCLI(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedCheckpointStore(t, work)

	if err := os.WriteFile(filepath.Join(work, "src", "app.txt"), []byte("mutated by a bad turn\n"), 0o644); err != nil {
		t.Fatalf("mutate src/app.txt: %v", err)
	}

	var out bytes.Buffer

	if err := runCheckpointRestore(context.Background(), &out, work, "sess-cli-turn-001"); err != nil {
		t.Fatalf("runCheckpointRestore: %v", err)
	}

	if !strings.Contains(out.String(), "sess-cli-turn-001") ||
		!strings.Contains(out.String(), "workspace restored to pre-turn state") {
		t.Errorf("output = %q; want the restored ref + the restored note", out.String())
	}

	data, err := os.ReadFile(filepath.Join(work, "src", "app.txt"))
	if err != nil {
		t.Fatalf("read src/app.txt: %v", err)
	}

	if string(data) != "original\n" {
		t.Errorf("src/app.txt = %q; want the pre-turn bytes", string(data))
	}
}

// TestCheckpointRestoreRejectsBadID verifies a malformed id is a structured
// error (never a silent success, never a shell-injection surface).
func TestCheckpointRestoreRejectsBadID(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedCheckpointStore(t, work)

	var out bytes.Buffer

	err := runCheckpointRestore(context.Background(), &out, work, "HEAD")
	if err == nil {
		t.Fatal("restore HEAD must fail (malformed checkpoint id)")
	}

	if !strings.Contains(err.Error(), "invalid checkpoint id") {
		t.Errorf("err = %v; want the invalid-id structured error", err)
	}
}
