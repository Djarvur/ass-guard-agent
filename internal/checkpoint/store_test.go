package checkpoint //nolint:testpackage // internal package test (Task 2 drives the unexported commit step)

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// --- shared test helpers ---

// treeMap fingerprints every regular file under dir (path -> sha256). The
// .ass-guard root is skipped — the checkpoint store itself is never part of
// the workspace comparison.
func treeMap(t *testing.T, dir string) map[string]string {
	t.Helper()

	out := map[string]string{}

	werr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}

		if d.IsDir() {
			if d.Name() == ".ass-guard" {
				return filepath.SkipDir
			}

			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return fmt.Errorf("rel %s: %w", path, rerr)
		}

		data, derr := os.ReadFile(path)
		if derr != nil {
			return fmt.Errorf("read %s: %w", path, derr)
		}

		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])

		return nil
	})
	if werr != nil {
		t.Fatalf("treeMap(%s): %v", dir, werr)
	}

	return out
}

// writeTestFile writes content (string or []byte) under path, creating
// parent directories as needed.
func writeTestFile(t *testing.T, path string, content any) {
	t.Helper()

	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}

	var data []byte

	switch c := content.(type) {
	case string:
		data = []byte(c)
	case []byte:
		data = c
	default:
		t.Fatalf("writeTestFile: unsupported content type %T", content)
	}

	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// seedWorkspace writes the nested fixture tree: plain text at multiple
// depths, a binary file, and an empty-dir marker file (git tracks no truly
// empty directories — the marker keeps the directory restorable).
func seedWorkspace(t *testing.T, dir string) {
	t.Helper()

	writeTestFile(t, filepath.Join(dir, "readme.txt"), "top level text\n")
	writeTestFile(t, filepath.Join(dir, "src", "main.go"), "package main\n\nfunc main() {}\n")

	binBlob := []byte{0x00, 0x01, 0xff, 0xfe, 0x7f, 0x80, 0x00, 0x01}
	writeTestFile(t, filepath.Join(dir, "src", "bin", "blob.bin"), binBlob)
	writeTestFile(t, filepath.Join(dir, "emptydir", ".keep"), "")
}

// openStore opens (initializing when absent) the shadow store over workDir.
func openStore(t *testing.T, workDir string) *Store {
	t.Helper()

	s, err := Open(workDir)
	if err != nil {
		t.Fatalf("Open(%s): %v", workDir, err)
	}

	return s
}

// snap drives one Snapshot call, failing the test on error.
func snap(t *testing.T, s *Store, sessionID, turnID string) {
	t.Helper()

	err := s.Snapshot(context.Background(), sessionID, turnID)
	if err != nil {
		t.Fatalf("Snapshot(%s): %v", turnID, err)
	}
}

// gitUser runs git in the USER's repo (dir) with a hermetic identity; any
// failure is fatal. Returns the combined output.
func gitUser(t *testing.T, dir string, args ...string) string {
	t.Helper()

	full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
	cmd := exec.CommandContext(context.Background(), "git", full...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}

	return string(out)
}

// userGitState is the captured identity of the USER's repository git state:
// HEAD ref bytes, the resolved HEAD commit, the index bytes, the porcelain
// status output, and a recursive fingerprint of the whole .git directory.
type userGitState struct {
	headRef   string
	resolved  string
	indexHash string
	status    string
	gitTree   map[string]string
}

// captureUserGit captures every observable .git facet. BYTE captures come
// FIRST: a `git status` legitimately rewrites the user index (opportunistic
// stat-cache refresh after restore rewrites worktree files), so status runs
// LAST — the invariant under test is what snapshot/restore wrote, not what a
// later status refreshes.
func captureUserGit(t *testing.T, work string) userGitState {
	t.Helper()

	st := userGitState{}

	headBytes, err := os.ReadFile(filepath.Join(work, ".git", "HEAD"))
	if err != nil {
		t.Fatalf("read user .git/HEAD: %v", err)
	}

	st.headRef = string(headBytes)

	indexBytes, err := os.ReadFile(filepath.Join(work, ".git", "index"))
	if err != nil {
		t.Fatalf("read user .git/index: %v", err)
	}

	sum := sha256.Sum256(indexBytes)
	st.indexHash = hex.EncodeToString(sum[:])
	st.gitTree = treeMap(t, filepath.Join(work, ".git"))
	st.resolved = strings.TrimSpace(gitUser(t, work, "rev-parse", "HEAD"))
	st.status = gitUser(t, work, "status", "--porcelain")

	return st
}

// assertUserGitUntouched fails the test unless both captured states are
// byte-identical across every facet (the core EARLY-01 invariant).
func assertUserGitUntouched(t *testing.T, before, after userGitState) {
	t.Helper()

	if before.headRef != after.headRef {
		t.Errorf("user .git/HEAD bytes changed: %q -> %q", before.headRef, after.headRef)
	}

	if before.resolved != after.resolved {
		t.Errorf("user HEAD commit changed: %s -> %s", before.resolved, after.resolved)
	}

	if before.indexHash != after.indexHash {
		t.Error("user .git/index bytes changed across snapshot/restore")
	}

	if before.status != after.status {
		t.Errorf("user git status --porcelain changed:\nbefore:\n%s\nafter:\n%s", before.status, after.status)
	}

	if !reflect.DeepEqual(before.gitTree, after.gitTree) {
		t.Error("user .git recursive tree changed across snapshot/restore")
	}
}

// TestSnapshotRestore_ByteIdentical (Task 1, Test 1 / EARLY-01 SC1): after a
// snapshot and arbitrary mutations (modify, delete, create files and
// directories), Restore returns the workspace to a state that is
// byte-identical to the pre-snapshot tree — compared over the FULL recursive
// path set and every file's bytes, with post-snapshot-created files asserted
// GONE.
func TestSnapshotRestore_ByteIdentical(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	s := openStore(t, work)
	snap(t, s, "sess-bi", "sess-bi-turn-001")

	before := treeMap(t, work)

	// Mutate: modify tracked content, delete a file, create new files in new
	// and existing directories.
	writeTestFile(t, filepath.Join(work, "src", "main.go"), "package main\n\nfunc main() { mutated() }\n")

	err := os.Remove(filepath.Join(work, "readme.txt"))
	if err != nil {
		t.Fatalf("remove readme.txt: %v", err)
	}

	writeTestFile(t, filepath.Join(work, "post-snapshot.txt"), "created after the snapshot")
	writeTestFile(t, filepath.Join(work, "newdir", "nested.txt"), "created after the snapshot, new dir")

	err = s.Restore(context.Background(), "sess-bi-turn-001")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	after := treeMap(t, work)

	if !reflect.DeepEqual(before, after) {
		for path, h := range before {
			ah, ok := after[path]
			switch {
			case !ok:
				t.Errorf("restore: path %q missing after restore", path)
			case ah != h:
				t.Errorf("restore: path %q content differs (want hash %s, got %s)", path, h, ah)
			}
		}

		for path := range after {
			if _, ok := before[path]; !ok {
				t.Errorf("restore: post-snapshot path %q survived the restore", path)
			}
		}
	}
}

// TestSnapshot_UserGitUntouched (Task 1, Test 2 — the core EARLY-01
// invariant): when the workspace IS a git repository, Snapshot leaves the
// user's .git byte-identical: HEAD ref bytes, index bytes, porcelain status,
// and the recursive .git tree.
func TestSnapshot_UserGitUntouched(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	gitUser(t, work, "init", "--quiet")
	gitUser(t, work, "add", "-A")
	gitUser(t, work, "commit", "--quiet", "-m", "initial")

	// Open the store BEFORE capturing state: its .ass-guard/ root must appear
	// identically (as one untracked dir line) on both sides of the comparison.
	s := openStore(t, work)

	before := captureUserGit(t, work)

	snap(t, s, "sess-gu", "sess-gu-turn-001")

	assertUserGitUntouched(t, before, captureUserGit(t, work))

	// The snapshot must not have INGESTED the user's .git either: git's
	// built-in worktree-root .git exclusion holds under the external git-dir.
	shadowDir := filepath.Join(work, ".ass-guard", "checkpoints", "shadow.git")
	lsTree := exec.CommandContext(context.Background(), "git",
		"--git-dir="+shadowDir, "ls-tree", "-r", "--name-only", "refs/checkpoints/sess-gu-turn-001")

	out, err := lsTree.CombinedOutput()
	if err != nil {
		t.Fatalf("ls-tree the shadow snapshot: %v\n%s", err, out)
	}

	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(line, ".git/") || line == ".git" {
			t.Errorf("snapshot ingested the user's .git: %q", line)
		}
	}
}

// TestRestore_UserGitUntouched (acceptance-named): a mutating turn followed
// by Restore also leaves the user's repository git state byte-identical —
// HEAD ref bytes, index bytes, status output, recursive .git tree.
func TestRestore_UserGitUntouched(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	gitUser(t, work, "init", "--quiet")
	gitUser(t, work, "add", "-A")
	gitUser(t, work, "commit", "--quiet", "-m", "initial")

	s := openStore(t, work)
	snap(t, s, "sess-ru", "sess-ru-turn-001")

	before := captureUserGit(t, work)

	// A mutating "turn": tracked file modified, untracked file created.
	writeTestFile(t, filepath.Join(work, "src", "main.go"), "package main\n\nfunc main() { changed() }\n")
	writeTestFile(t, filepath.Join(work, "mutated-turn.txt"), "a bad turn created this")

	err := s.Restore(context.Background(), "sess-ru-turn-001")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	assertUserGitUntouched(t, before, captureUserGit(t, work))

	// And the workspace itself is back to the pre-turn content.
	if got := treeMap(t, work); got["src/main.go"] == "" {
		t.Error("restore lost src/main.go")
	}
}

// TestCheckpointListOrdering (Task 1, Test 3): three snapshots across two
// sessions list ascending by (sessionID, zero-padded turn number) regardless
// of creation order, and repeated List calls return identical output.
func TestCheckpointListOrdering(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	s := openStore(t, work)

	// Insert out of sequence: sess-b turn 1, then sess-a turn 2, then sess-a
	// turn 1. Distinct workspace content per snapshot keeps the commits real.
	snap(t, s, "sess-b", "sess-b-turn-001")
	writeTestFile(t, filepath.Join(work, "one.txt"), "change one\n")

	snap(t, s, "sess-a", "sess-a-turn-002")
	writeTestFile(t, filepath.Join(work, "two.txt"), "change two\n")

	snap(t, s, "sess-a", "sess-a-turn-001")

	first, lerr := s.List()
	if lerr != nil {
		t.Fatalf("List: %v", lerr)
	}

	want := []struct {
		sessionID string
		turnNum   int
	}{
		{"sess-a", 1}, {"sess-a", 2}, {"sess-b", 1},
	}

	if len(first) != len(want) {
		t.Fatalf("List returned %d entries; want %d (%+v)", len(first), len(want), first)
	}

	for i, w := range want {
		got := first[i]
		if got.SessionID != w.sessionID || got.TurnNum != w.turnNum {
			t.Errorf("List[%d] = (%s, turn %d); want (%s, turn %d)",
				i, got.SessionID, got.TurnNum, w.sessionID, w.turnNum)
		}

		wantRef := "refs/checkpoints/" + w.sessionID + fmt.Sprintf("-turn-%03d", w.turnNum)
		if got.Ref != wantRef {
			t.Errorf("List[%d].Ref = %q; want %q", i, got.Ref, wantRef)
		}

		if got.CommittedAt.IsZero() || time.Since(got.CommittedAt) > time.Hour {
			t.Errorf("List[%d].CommittedAt = %v; want a plausible timestamp", i, got.CommittedAt)
		}
	}

	// Stable across repeated invocations.
	second, lerr2 := s.List()
	if lerr2 != nil {
		t.Fatalf("List (repeat): %v", lerr2)
	}

	if !reflect.DeepEqual(first, second) {
		t.Errorf("List output unstable:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}
