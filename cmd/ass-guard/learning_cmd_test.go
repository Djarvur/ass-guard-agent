package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/learning"
)

// TestLearningList_Empty verifies an empty store prints the header + an stderr
// note + exit 0.
func TestLearningList_Empty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "learned.yaml")

	var stdout, stderr bytes.Buffer

	err := runLearningList(&stdout, &stderr, path)
	if err != nil {
		t.Fatalf("runLearningList: %v", err)
	}

	if !strings.Contains(stdout.String(), "ID") || !strings.Contains(stdout.String(), "SITUATION") {
		t.Errorf("stdout = %q; want the header row", stdout.String())
	}
	// Only the header row in stdout.
	if strings.Count(stdout.String(), "\n") != 1 {
		t.Errorf("stdout rows = %d; want 1 (header only) for an empty store", strings.Count(stdout.String(), "\n"))
	}

	if !strings.Contains(stderr.String(), "no learned entries") {
		t.Errorf("stderr = %q; want the empty-store note", stderr.String())
	}
}

// TestLearningList_Populated verifies a store with 3 entries prints 3 rows.
func TestLearningList_Populated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "learned.yaml")

	store, err := learning.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	_ = store.RecordCandidate("alpha situation", "x", "t1")
	_ = store.RecordCandidate("beta situation", "y", "t1")
	_ = store.RecordCandidate("gamma situation", "z", "t1")

	var stdout bytes.Buffer

	err = runLearningList(&stdout, &bytes.Buffer{}, path)
	if err != nil {
		t.Fatalf("runLearningList: %v", err)
	}
	// Header + 3 rows = 4 newlines.
	if got := strings.Count(stdout.String(), "\n"); got != 4 {
		t.Errorf("stdout rows = %d; want 4 (header + 3 entries):\n%s", got, stdout.String())
	}

	for _, want := range []string{"alpha-situation", "beta-situation", "gamma-situation"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing row for %q:\n%s", want, stdout.String())
		}
	}
}

// TestLearningRevert_Existing verifies reverting an existing id prints
// confirmation + the entry is gone.
func TestLearningRevert_Existing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "learned.yaml")

	store, err := learning.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	_ = store.RecordCandidate("alpha", "x", "t1")
	_ = store.RecordCandidate("beta", "y", "t1")

	var stdout, stderr bytes.Buffer

	err = runLearningRevert(&stdout, &stderr, path, learning.Slug("alpha"))
	if err != nil {
		t.Fatalf("runLearningRevert: %v", err)
	}

	if !strings.Contains(stdout.String(), "reverted") {
		t.Errorf("stdout = %q; want reverted confirmation", stdout.String())
	}

	store2, _ := learning.Open(path)
	if len(store2.List()) != 1 {
		t.Errorf("after revert, store has %d entries; want 1", len(store2.List()))
	}
}

// TestLearningRevert_Missing verifies reverting a missing id warns to stderr +
// still exits 0 (Revert is a no-op there per T3 Test 4).
func TestLearningRevert_Missing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "learned.yaml")

	_, err := learning.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	err = runLearningRevert(&stdout, &stderr, path, "does-not-exist")
	if err != nil {
		t.Fatalf("runLearningRevert on missing id: %v (want nil — no-op)", err)
	}

	if !strings.Contains(stderr.String(), "nothing reverted") {
		t.Errorf("stderr = %q; want the missing-id warning", stderr.String())
	}
}
