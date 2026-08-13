package firstrun

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnsure_SeedsFreshDir asserts Ensure on a directory with no .ass-guard/
// creates the dir, the self-gitignore, and the full default tree (D-04).
func TestEnsure_SeedsFreshDir(t *testing.T) {
	t.Parallel()

	work := t.TempDir()

	seeded, err := Ensure(work)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if !seeded {
		t.Errorf("seeded = false, want true on a fresh dir")
	}

	for _, rel := range []string{
		".gitignore",
		"profiles/zcode/tools.json",
		"profiles/zcode/profile.yaml",
		"openspec.toml",
		"scheduling.yaml",
	} {
		if _, err := os.Stat(filepath.Join(work, ".ass-guard", rel)); err != nil {
			t.Errorf("expected .ass-guard/%s materialized: %v", rel, err)
		}
	}
}

// TestEnsure_IdempotentSecondRun asserts a second Ensure returns (false, nil)
// and mutates nothing — non-clobbering on an already-initialized dir.
func TestEnsure_IdempotentSecondRun(t *testing.T) {
	t.Parallel()

	work := t.TempDir()

	if _, err := Ensure(work); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}

	// Drop a marker the second run must not touch (and must not delete).
	marker := filepath.Join(work, ".ass-guard", "operator-marker.txt")
	if err := os.WriteFile(marker, []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	seeded, err := Ensure(work)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}

	if seeded {
		t.Errorf("seeded = true, want false on an existing .ass-guard/")
	}

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("marker vanished after second Ensure: %v", err)
	}

	if string(got) != "keep me\n" {
		t.Errorf("marker corrupted by second Ensure: %q", got)
	}
}

// TestEnsure_GitignoreBodyIsCanonical asserts the written .ass-guard/.gitignore
// is byte-equal to the D-07 self-gitignore body ("*\n!.gitignore\n"), identical
// to internal/session/transcript.go's selfGitignoreContent.
func TestEnsure_GitignoreBodyIsCanonical(t *testing.T) {
	t.Parallel()

	work := t.TempDir()

	if _, err := Ensure(work); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(work, ".ass-guard", ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}

	if string(got) != selfGitignoreContent {
		t.Errorf(".gitignore body = %q, want %q (D-07)", got, selfGitignoreContent)
	}
}

// TestEnsure_ExistingEmptyDirIsNotReseeded asserts that when .ass-guard/ exists
// but is empty (partial init), Ensure returns (false, nil) without writing — it
// does not half-seed an existing directory an operator may have customized.
func TestEnsure_ExistingEmptyDirIsNotReseeded(t *testing.T) {
	t.Parallel()

	work := t.TempDir()

	if err := os.MkdirAll(filepath.Join(work, ".ass-guard"), 0o755); err != nil {
		t.Fatalf("mkdir .ass-guard: %v", err)
	}

	seeded, err := Ensure(work)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if seeded {
		t.Errorf("seeded = true, want false on an existing (empty) .ass-guard/")
	}

	// The dir stays empty — Ensure did not write the tree into it.
	if _, err := os.Stat(filepath.Join(work, ".ass-guard", "profiles")); !os.IsNotExist(err) {
		t.Errorf("Ensure wrote profiles/ into an existing empty .ass-guard/ (should not half-seed): %v", err)
	}
}
