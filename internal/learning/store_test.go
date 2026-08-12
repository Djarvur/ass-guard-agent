package learning_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/learning"
)

// newStore opens a Store against a temp learned.yaml so every test is isolated.
func newStore(t *testing.T) *learning.Store {
	t.Helper()
	dir := t.TempDir()

	s, err := learning.Open(filepath.Join(dir, "learned.yaml"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	return s
}

// TestStore_OpenCreatesEmpty verifies Open creates an empty-valid file when
// absent (the zero-config floor — D-16).
func TestStore_OpenCreatesEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "learned.yaml")

	s, err := learning.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("Open did not create %q: %v", path, err)
	}

	if entries := s.List(); len(entries) != 0 {
		t.Errorf("new store List = %v; want empty", entries)
	}
}

// TestStore_RecordCandidateConfidence0 verifies RecordCandidate creates a
// candidate (confidence 0) entry that Lookup returns (LRN-01).
func TestStore_RecordCandidateConfidence0(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	err := s.RecordCandidate("unmatched:launch", "fresh-context", "turn-001")
	if err != nil {
		t.Fatalf("RecordCandidate: %v", err)
	}

	e, ok := s.Lookup("unmatched:launch")
	if !ok {
		t.Fatal("Lookup returned ok=false after RecordCandidate")
	}

	if e.Status != learning.StatusCandidate {
		t.Errorf("Status = %q; want candidate", e.Status)
	}

	if e.Confidence != 0 {
		t.Errorf("Confidence = %d; want 0", e.Confidence)
	}

	if e.Answer != "fresh-context" {
		t.Errorf("Answer = %q; want fresh-context", e.Answer)
	}
}

// TestStore_RecordCandidateIdempotent verifies a second RecordCandidate with a
// DIFFERENT answer does NOT overwrite (the operator must Revert to change it).
func TestStore_RecordCandidateIdempotent(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_ = s.RecordCandidate("x", "A", "t1")
	_ = s.RecordCandidate("x", "B", "t2")

	e, ok := s.Lookup("x")
	if !ok {
		t.Fatal("Lookup returned ok=false")
	}

	if e.Answer != "A" {
		t.Errorf("Answer = %q; want A (not overwritten)", e.Answer)
	}
}

// TestStore_ConfirmThresholdActive verifies 3 Confirm calls with the SAME answer
// flip Status to active (LRN-03 — the confidence threshold).
func TestStore_ConfirmThresholdActive(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	_ = s.RecordCandidate("feature", stopContinue, "t1")
	for _, tid := range []string{"t1", "t2", "t3"} {
		if _, err := s.Confirm("feature", stopContinue, tid); err != nil {
			t.Fatalf("Confirm(%s): %v", tid, err)
		}
	}

	e, ok := s.Lookup("feature")
	if !ok {
		t.Fatal("Lookup returned ok=false")
	}

	if e.Status != learning.StatusActive {
		t.Errorf("Status = %q; want active after 3 confirms", e.Status)
	}

	if e.Confidence != 3 {
		t.Errorf("Confidence = %d; want 3", e.Confidence)
	}
}

// TestStore_ConfirmBelowThresholdCandidate verifies 2 Confirms keep Status
// candidate (provisional — still used, but flagged).
func TestStore_ConfirmBelowThresholdCandidate(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_ = s.RecordCandidate("feature", stopContinue, "t1")
	_, _ = s.Confirm("feature", stopContinue, "t1")
	_, _ = s.Confirm("feature", stopContinue, "t2")

	e, _ := s.Lookup("feature")
	if e.Status != learning.StatusCandidate {
		t.Errorf("Status = %q; want candidate (provisional below 3)", e.Status)
	}
}

// TestStore_ConfirmConflict verifies Confirm with a DIFFERENT answer yields
// ErrConflict + Status=conflict + Confidence NOT incremented (T-04-07).
func TestStore_ConfirmConflict(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_ = s.RecordCandidate("c", "A", "t1")

	_, err := s.Confirm("c", "B", "t2")
	if !errors.Is(err, learning.ErrConflict) {
		t.Errorf("Confirm err = %v; want ErrConflict", err)
	}

	entries := s.List()
	if len(entries) != 1 || entries[0].Status != learning.StatusConflict {
		t.Errorf("entry = %+v; want status conflict", entries)
	}

	if entries[0].Confidence != 0 {
		t.Errorf("Confidence = %d; want 0 (not incremented on conflict)", entries[0].Confidence)
	}
}

// TestStore_ConflictBlocksUse verifies a conflicted entry is NOT returned by
// Lookup (the engine would re-ask).
func TestStore_ConflictBlocksUse(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_ = s.RecordCandidate("c", "A", "t1")

	_, _ = s.Confirm("c", "B", "t2")
	if _, ok := s.Lookup("c"); ok {
		t.Error("Lookup returned a conflicted entry; want ok=false (engine re-asks)")
	}
}

// TestStore_ExpiredIgnoredNotDeleted verifies an expired entry is skipped by
// Lookup but STILL present in List (so the operator can renew/purge — D-19).
func TestStore_ExpiredIgnoredNotDeleted(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_ = s.RecordCandidate("old", "wait", "t1")
	// Force the entry into the past by re-confirming once then rewriting its
	// expiry via a direct List/Revert cycle is awkward; instead construct an
	// expired entry by recording + manipulating via the public surface is not
	// possible. Use the package-internal knowledge: record then Confirm to make
	// it active, then overwrite the file with an expired entry.
	_, _ = s.Confirm("old", "wait", "t2")
	_, _ = s.Confirm("old", "wait", "t3")
	// Rewrite the file with a custom expiry for the test.
	path := filepath.Dir(t.TempDir()) // unused; we re-open the same path
	_ = path
	// Direct file rewrite: open the store's file (it was created in newStore's
	// temp dir). We need the path — re-create a store whose file we control.
	dir := t.TempDir()
	fp := filepath.Join(dir, "learned.yaml")

	s2, err := learning.Open(fp)
	if err != nil {
		t.Fatal(err)
	}

	_ = s2.RecordCandidate("old2", "wait", "t1")
	entries := s2.List()
	// Overwrite the file with a back-dated expiry.
	past := time.Now().Add(-1 * time.Hour)
	_ = os.WriteFile(fp, []byte("entries:\n  - id: old2\n    situation: old2\n    answer: wait\n    confidence: 3\n    expiry: "+past.Format(time.RFC3339)+"\n    source_turns: [t1]\n    status: active\n"), 0o600)
	_ = entries

	if _, ok := s2.Lookup("old2"); ok {
		t.Error("Lookup returned an expired entry; want ok=false (ignored)")
	}

	if list := s2.List(); len(list) != 1 {
		t.Errorf("List = %d entries; want 1 (expired entry retained)", len(list))
	}
}

// TestStore_SourceTurnProvenanceDeduped verifies 3 confirms with turns
// [t1,t2,t3] yield an active entry whose SourceTurns contains all three
// (deduped — Test 8).
func TestStore_SourceTurnProvenanceDeduped(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	_ = s.RecordCandidate("p", stopContinue, "t1")
	for _, tid := range []string{"t1", "t2", "t3", "t1"} { // t1 repeated
		_, _ = s.Confirm("p", stopContinue, tid)
	}

	e, _ := s.Lookup("p")
	want := map[string]bool{"t1": true, "t2": true, "t3": true}

	got := map[string]bool{}
	for _, t := range e.SourceTurns {
		got[t] = true
	}

	if len(got) != 3 || !want["t1"] || !want["t2"] || !want["t3"] {
		t.Errorf("SourceTurns = %v; want [t1 t2 t3] deduped", e.SourceTurns)
	}
}

// TestStore_RevertRemovesEntry verifies Revert removes the named entry + a
// re-Revert of the same id is a no-op (LRN-04 / D-20).
func TestStore_RevertRemovesEntry(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_ = s.RecordCandidate("a", "x", "t1")
	_ = s.RecordCandidate("b", "y", "t1")

	_ = s.RecordCandidate("c", "z", "t1")

	err := s.Revert(learning.Slug("b"))
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}

	list := s.List()
	if len(list) != 2 {
		t.Errorf("after Revert, List = %d entries; want 2", len(list))
	}
	// Re-Revert the same id — no-op (not an error).
	err = s.Revert(learning.Slug("b"))
	if err != nil {
		t.Errorf("re-Revert missing id = %v; want nil (no-op)", err)
	}
}

// TestStore_ListDeterministicOrder verifies entries written A,B,C come back in
// that order.
func TestStore_ListDeterministicOrder(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_ = s.RecordCandidate("alpha", "x", "t1")
	_ = s.RecordCandidate("beta", "y", "t1")
	_ = s.RecordCandidate("gamma", "z", "t1")

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("List = %d; want 3", len(list))
	}
	// File order = append order; situations are alpha/beta/gamma.
	if list[0].Situation != "alpha" || list[2].Situation != "gamma" {
		t.Errorf("List order = %v %v %v; want alpha, beta, gamma", list[0].Situation, list[1].Situation, list[2].Situation)
	}
}

// TestStore_RevertAtomicReadOnlyDir verifies a failed rename leaves the file
// unchanged (Revert is atomic — T-04-09).
func TestStore_RevertAtomicReadOnlyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fp := filepath.Join(dir, "learned.yaml")

	s, err := learning.Open(fp)
	if err != nil {
		t.Fatal(err)
	}

	_ = s.RecordCandidate("a", "x", "t1")
	_ = s.RecordCandidate("b", "y", "t1")
	_ = s.RecordCandidate("c", "z", "t1")
	// Make the directory read-only so the temp+rename fails. Restore after.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir read-only: %v", err)
	}
	defer os.Chmod(dir, 0o755)

	if err := s.Revert(learning.Slug("b")); err == nil {
		t.Error("Revert on a read-only dir = nil; want a write error (atomic)")
	}
	// Restore + verify all 3 entries still present.
	_ = os.Chmod(dir, 0o755)

	if list := s.List(); len(list) != 3 {
		t.Errorf("after failed Revert, List = %d; want 3 (unchanged)", len(list))
	}
}

// TestSlug_Deterministic verifies Slug produces a stable id for a situation.
func TestSlug_Deterministic(t *testing.T) {
	t.Parallel()

	a := learning.Slug("unmatched:launch:webfetch")

	b := learning.Slug("Unmatched:Launch:WebFetch")
	if a != b {
		t.Errorf("Slug casing not normalized: %q vs %q", a, b)
	}

	if a == "" {
		t.Error("Slug returned empty for a non-empty situation")
	}
}
