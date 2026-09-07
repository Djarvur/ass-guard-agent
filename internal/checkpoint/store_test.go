package checkpoint //nolint:testpackage // internal package test (Task 2 drives the unexported commit step)

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

// TestRestoreNonLatest_RemovesLaterTurnFiles (CR-01): restoring a NON-latest
// checkpoint must return the workspace to that snapshot's byte-state — files
// created in later turns (staged into the shadow index by the LATER
// snapshot's add -A) must be REMOVED. Overlay checkout never deletes
// index entries absent from the target tree, and clean -fd only sweeps
// UNtracked files — the multi-turn undo story (EARLY-01) is broken for every
// restore except the newest checkpoint without this.
func TestRestoreNonLatest_RemovesLaterTurnFiles(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	s := openStore(t, work)
	snap(t, s, "sess-undo", "sess-undo-turn-001")

	before := treeMap(t, work)

	// Turn 2: create new files (staged into the shadow index by snapshot B)
	// and mutate a tracked one.
	writeTestFile(t, filepath.Join(work, "later.txt"), "created in turn 2\n")
	writeTestFile(t, filepath.Join(work, "laterdir", "nested.txt"), "created in turn 2, new dir\n")
	writeTestFile(t, filepath.Join(work, "readme.txt"), "mutated in turn 2\n")

	snap(t, s, "sess-undo", "sess-undo-turn-002")

	err := s.Restore(context.Background(), "sess-undo-turn-001")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	after := treeMap(t, work)

	if reflect.DeepEqual(before, after) {
		return
	}

	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("restore: path %q created in a LATER turn survived restoring turn-001", path)
		}
	}

	for path, h := range before {
		ah, ok := after[path]
		switch {
		case !ok:
			t.Errorf("restore: path %q from turn-001 missing after restore", path)
		case ah != h:
			t.Errorf("restore: path %q content differs (want hash %s, got %s)", path, h, ah)
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

// --- Task 2: the invariant battery ---

// TestSnapshot_NoChangesStillCommits (Test 6): a turn with zero workspace
// changes still produces a checkpoint — two distinct refs exist, and both
// restore to the same tree (every turn boundary stays restorable).
func TestSnapshot_NoChangesStillCommits(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	s := openStore(t, work)
	snap(t, s, "sess-nc", "sess-nc-turn-001")
	snap(t, s, "sess-nc", "sess-nc-turn-002")

	entries, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("List returned %d entries; want 2 (zero-change turns still commit)", len(entries))
	}

	wantTree := treeMap(t, work)

	for _, id := range []string{"sess-nc-turn-001", "sess-nc-turn-002"} {
		err = s.Restore(context.Background(), id)
		if err != nil {
			t.Fatalf("Restore(%s): %v", id, err)
		}

		if got := treeMap(t, work); !reflect.DeepEqual(got, wantTree) {
			t.Errorf("Restore(%s) changed the tree; want the identical empty-change tree", id)
		}
	}
}

// TestSnapshot_IdempotentSameTurn (Test 7): re-snapshotting the same
// (sessionID, turnID) — the retry-after-transient-failure case — updates the
// SAME ref: exactly one ref exists for the id, no duplicates, and both
// restores yield identical trees.
func TestSnapshot_IdempotentSameTurn(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	s := openStore(t, work)
	snap(t, s, "sess-idem", "sess-idem-turn-001")
	snap(t, s, "sess-idem", "sess-idem-turn-001")

	entries, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	for _, e := range entries {
		if e.Ref != "refs/checkpoints/sess-idem-turn-001" {
			t.Fatalf("duplicate ref %s; want exactly one ref for the re-snapshotted turn", e.Ref)
		}
	}

	if len(entries) != 1 {
		t.Fatalf("List returned %d entries; want 1 (same turn, updated ref)", len(entries))
	}

	wantTree := treeMap(t, work)

	for range 2 {
		err = s.Restore(context.Background(), "sess-idem-turn-001")
		if err != nil {
			t.Fatalf("Restore: %v", err)
		}

		if got := treeMap(t, work); !reflect.DeepEqual(got, wantTree) {
			t.Error("repeated restores yield different trees; want the same tree both times")
		}
	}
}

// TestSnapshot_InterruptedNeverExposesPartialRef (Test 8): the commit-only
// step leaves NO refs/checkpoints/ entry — the turn ref appears only after
// update-ref runs (a ref exists only once its commit object does); and a
// ctx-canceled Snapshot returns an error with the ref set unchanged.
func TestSnapshot_InterruptedNeverExposesPartialRef(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	s := openStore(t, work)

	// Drive the internal commit-only step directly: the commit object exists
	// but no turn ref may.
	sha, err := s.commitSnapshot(context.Background(), "sess-int-turn-001")
	if err != nil {
		t.Fatalf("commitSnapshot: %v", err)
	}

	if sha == "" {
		t.Fatal("commitSnapshot returned an empty sha")
	}

	entries, err := s.List()
	if err != nil {
		t.Fatalf("List after commit-only step: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("commit-only step exposed %d refs; want 0 until update-ref runs", len(entries))
	}

	// The ref appears exactly when update-ref runs.
	err = s.updateRef(context.Background(), "sess-int-turn-001", sha)
	if err != nil {
		t.Fatalf("updateRef: %v", err)
	}

	entries, err = s.List()
	if err != nil {
		t.Fatalf("List after update-ref: %v", err)
	}

	if len(entries) != 1 || entries[0].Ref != "refs/checkpoints/sess-int-turn-001" {
		t.Fatalf("List = %+v; want exactly the just-updated ref", entries)
	}

	// A ctx-canceled Snapshot errors and leaves the ref set unchanged.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cerr := s.Snapshot(canceled, "sess-int", "sess-int-turn-002")
	if cerr == nil {
		t.Fatal("canceled-ctx Snapshot must return an error")
	}

	after, err := s.List()
	if err != nil {
		t.Fatalf("List after canceled snapshot: %v", err)
	}

	if len(after) != 1 || after[0].Ref != "refs/checkpoints/sess-int-turn-001" {
		t.Fatalf("canceled snapshot changed the ref set: %+v", after)
	}
}

// TestConcurrentSnapshotRestoreSerialize (Test 9): a concurrent Snapshot and
// Restore against one store via the PUBLIC API both complete without index
// corruption (run under -race); the store stays usable afterwards.
func TestConcurrentSnapshotRestoreSerialize(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	s := openStore(t, work)
	snap(t, s, "sess-conc", "sess-conc-turn-001")

	writeTestFile(t, filepath.Join(work, "mutated.txt"), "changed between snapshot and restore\n")

	var (
		wg         sync.WaitGroup
		snapErr    error
		restoreErr error
	)

	wg.Add(2)

	go func() {
		defer wg.Done()

		snapErr = s.Snapshot(context.Background(), "sess-conc", "sess-conc-turn-002")
	}()

	go func() {
		defer wg.Done()

		restoreErr = s.Restore(context.Background(), "sess-conc-turn-001")
	}()

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent Snapshot+Restore did not serialize within 30s (lock deadlock?)")
	}

	if snapErr != nil {
		t.Errorf("concurrent Snapshot: %v", snapErr)
	}

	if restoreErr != nil {
		t.Errorf("concurrent Restore: %v", restoreErr)
	}

	// A consistent ref list after the race (no index corruption).
	assertEntryCount(t, s, 2)

	// The store remains fully usable: one more snapshot round-trip.
	snap(t, s, "sess-conc", "sess-conc-turn-003")
	assertEntryCount(t, s, 3)
}

// assertEntryCount fails unless List returns exactly want entries.
func assertEntryCount(t *testing.T, s *Store, want int) {
	t.Helper()

	entries, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != want {
		t.Fatalf("List returned %d entries; want %d", len(entries), want)
	}
}

// TestRetentionPrune (Test 10): DefaultKeep+5 snapshots leave exactly
// DefaultKeep refs — the oldest pruned, the newest kept.
func TestRetentionPrune(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	writeTestFile(t, filepath.Join(work, "tiny.txt"), "tiny workspace\n")

	s := openStore(t, work)

	total := DefaultKeep + 5

	for turn := range total {
		snap(t, s, "sess-ret", fmt.Sprintf("sess-ret-turn-%03d", turn+1))
	}

	entries, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != DefaultKeep {
		t.Fatalf("retention kept %d refs; want exactly DefaultKeep=%d", len(entries), DefaultKeep)
	}

	oldest := entries[0]
	newest := entries[len(entries)-1]

	if oldest.TurnNum != 6 {
		t.Errorf("oldest surviving = turn %d; want turn 6 (turns 1-5 pruned)", oldest.TurnNum)
	}

	if newest.TurnNum != total {
		t.Errorf("newest surviving = turn %d; want turn %d", newest.TurnNum, total)
	}
}

// TestStorePerms (Test 11): after Open on a fresh workDir, the store dirs
// are mode 0700 (T-14-02: the shadow store may carry workspace secrets).
func TestStorePerms(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	openStore(t, work)

	for _, dir := range []string{
		filepath.Join(work, ".ass-guard", "checkpoints"),
		filepath.Join(work, ".ass-guard", "checkpoints", "shadow.git"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}

		if info.Mode().Perm() != 0o700 {
			t.Errorf("%s mode = %v; want 0700", dir, info.Mode().Perm())
		}
	}
}

// TestRestore_RejectsBadIds (Test 12, T-14-01): malformed ids — option-like
// strings, refspecs, traversal paths, the empty string — each return a
// validation error WITHOUT spawning any git subprocess (asserted via the
// gitRun seam); a well-formed but unknown id DOES reach git and errors.
//
// NOT parallel: it swaps the package gitRun seam (sequential tests run to
// completion before paused parallel tests resume, so no interference).
func TestRestore_RejectsBadIds(t *testing.T) { //nolint:paralleltest // swaps the package gitRun seam
	work := t.TempDir()
	seedWorkspace(t, work)

	s := openStore(t, work)

	spawns := 0
	orig := gitRun

	gitRun = func(context.Context, string, []string, string, ...string) ([]byte, error) {
		spawns++

		return nil, errFakeGit
	}

	defer func() { gitRun = orig }()

	for _, id := range []string{"HEAD", "--help", "../../etc", "refs/heads/main", ""} {
		err := s.Restore(context.Background(), id)
		if err == nil {
			t.Errorf("Restore(%q) must fail (malformed checkpoint id)", id)
		}

		if !strings.Contains(err.Error(), "invalid checkpoint id") {
			t.Errorf("Restore(%q) err = %v; want the invalid-id validation error", id, err)
		}
	}

	if spawns != 0 {
		t.Fatalf("malformed ids spawned git %d times; validation must precede ANY exec", spawns)
	}

	// A well-formed but UNKNOWN id passes validation, reaches git, and
	// surfaces the git failure as a structured error.
	err := s.Restore(context.Background(), "sess-unknown-turn-999")
	if err == nil {
		t.Fatal("Restore of an unknown-but-valid id must fail")
	}

	if spawns != 1 {
		t.Errorf("unknown-but-valid id spawned git %d times; want exactly 1", spawns)
	}
}

// errFakeGit is the seam double's canned git failure.
var errFakeGit = errors.New("fake git: always fails")

// --- 23-03 Task 1: pre-restore snapshot family ---

// TestPreRestoreSnapshotFamily pins D-09: a pre-restore snapshot is a
// first-class checkpoint object — per-session -pre- id family, deterministic
// counter across process restarts (max+1 from existing refs), restorable via
// the SAME machinery (byte-identical tree round trip), and listed naturally
// among turn entries.
func TestPreRestoreSnapshotFamily(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	seedWorkspace(t, work)

	s := openStore(t, work)
	snap(t, s, "sess-pr", "sess-pr-turn-001")

	id1, err := s.SnapshotPreRestore(context.Background(), "sess-pr")
	if err != nil {
		t.Fatalf("SnapshotPreRestore: %v", err)
	}

	if id1 != "sess-pr-pre-001" {
		t.Fatalf("first pre-restore id = %q; want sess-pr-pre-001", id1)
	}

	id2, err := s.SnapshotPreRestore(context.Background(), "sess-pr")
	if err != nil {
		t.Fatalf("SnapshotPreRestore (2nd): %v", err)
	}

	if id2 != "sess-pr-pre-002" {
		t.Fatalf("second pre-restore id = %q; want sess-pr-pre-002", id2)
	}

	// Deterministic across restarts: a fresh Store over the same workspace
	// resumes the counter at max+1 (seeded by scanning existing -pre- refs).
	s2 := openStore(t, work)

	id3, err := s2.SnapshotPreRestore(context.Background(), "sess-pr")
	if err != nil {
		t.Fatalf("SnapshotPreRestore (post-reopen): %v", err)
	}

	if id3 != "sess-pr-pre-003" {
		t.Fatalf("post-reopen pre-restore id = %q; want sess-pr-pre-003", id3)
	}

	// List carries turn AND pre entries for the session, naturally ordered.
	entries, err := s2.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var sawTurn1, sawPre1, sawPre2, sawPre3 bool

	for _, e := range entries {
		switch e.Ref {
		case "refs/checkpoints/sess-pr-turn-001":
			sawTurn1 = e.Kind == "turn"
		case "refs/checkpoints/sess-pr-pre-001":
			sawPre1 = e.Kind == "pre"
		case "refs/checkpoints/sess-pr-pre-002":
			sawPre2 = e.Kind == "pre"
		case "refs/checkpoints/sess-pr-pre-003":
			sawPre3 = e.Kind == "pre"
		}
	}

	if !sawTurn1 || !sawPre1 || !sawPre2 || !sawPre3 {
		t.Errorf("List missing entries (turn1=%v pre1=%v pre2=%v pre3=%v): %+v",
			sawTurn1, sawPre1, sawPre2, sawPre3, entries)
	}

	// D-09 restorable via the same machinery: mutate the tree, restore the
	// pre-restore id, get the snapshotted bytes back.
	want := treeMap(t, work)

	writeTestFile(t, filepath.Join(work, "readme.txt"), "mutated after the pre-restore snapshot\n")
	writeTestFile(t, filepath.Join(work, "created-later.txt"), "post-snapshot file\n")

	err = s2.Restore(context.Background(), id1)
	if err != nil {
		t.Fatalf("Restore(%s): %v", id1, err)
	}

	if got := treeMap(t, work); !reflect.DeepEqual(got, want) {
		t.Errorf("restore of pre-restore id changed the tree; want the snapshotted bytes")
	}

	// Fail-closed surface: the empty session id errors (the CALLER aborts the
	// restore on any snapshot failure — the store side surfaces the error).
	if _, err := s2.SnapshotPreRestore(context.Background(), ""); err == nil {
		t.Error("SnapshotPreRestore(\"\") must fail (empty session id)")
	}
}

// TestIDGrammarTable pins the three coupled grammar sites (Pitfall 6): BOTH
// families accepted, malformed variants rejected at the pattern, at
// validateTurnID, and at parseRefLine — neither site accepts anything the
// other rejects.
func TestIDGrammarTable(t *testing.T) {
	t.Parallel()

	accept := []string{
		"sess-turn-001", "sess-pre-001",
		"my-sess-2-pre-010", "A_b-c-turn-999999",
		"sess-turn-pre-001", // session charset allows "sess-turn"; family pre — parses, and such a ref exists only if that literal session minted it
	}
	for _, id := range accept {
		if !idPattern.MatchString(id) {
			t.Errorf("idPattern rejected valid id %q", id)
		}
	}

	reject := []string{
		"sess-pre-1", "sess-turn-1", // short sequence (< 3 digits)
		"sess-prex-001",         // wrong family
		"sess-pre-", "sess-pre", // no sequence
		"sess-pre--001",             // double separator
		"-pre-001", "sess--pre-001", // session charset still matches "-"... sess--pre-001: session="sess", then "--pre-"? see below
		"HEAD", "../../etc", "refs/heads/main", "",
	}
	for _, id := range reject {
		if idPattern.MatchString(id) {
			t.Errorf("idPattern accepted malformed id %q", id)
		}
	}

	// validateTurnID: family-aware session ownership.
	if err := validateTurnID("sess", "sess-turn-001"); err != nil {
		t.Errorf("validateTurnID(turn) = %v; want nil", err)
	}

	if err := validateTurnID("sess", "sess-pre-001"); err != nil {
		t.Errorf("validateTurnID(pre) = %v; want nil", err)
	}

	if err := validateTurnID("sessA", "sessB-pre-001"); err == nil {
		t.Error("validateTurnID must reject an id owned by another session")
	}

	if err := validateTurnID("sess", "sess-pre-1"); err == nil {
		t.Error("validateTurnID must reject a short sequence")
	}

	if err := validateTurnID("", "sess-pre-001"); err == nil {
		t.Error("validateTurnID must reject an empty session id")
	}

	// parseRefLine: SessionID derivation for BOTH families.
	cases := []struct {
		line        string
		wantSession string
		wantNum     int
		wantKind    string
	}{
		{"refs/checkpoints/sess-x-turn-007 1700000000", "sess-x", 7, "turn"},
		{"refs/checkpoints/sess-x-pre-007 1700000000", "sess-x", 7, "pre"},
		{"refs/checkpoints/sess-y-pre-042 1699999999", "sess-y", 42, "pre"},
	}

	for _, c := range cases {
		e, ok := parseRefLine(c.line)
		if !ok {
			t.Errorf("parseRefLine(%q) rejected a valid ref line", c.line)

			continue
		}

		if e.SessionID != c.wantSession || e.TurnNum != c.wantNum || e.Kind != c.wantKind {
			t.Errorf("parseRefLine(%q) = %+v; want session %q num %d kind %q",
				c.line, e, c.wantSession, c.wantNum, c.wantKind)
		}
	}

	for _, line := range []string{
		"refs/checkpoints/last 1700000000", // convenience tip skipped
		"refs/checkpoints/sess-bad 1700000000",
		"garbage",
		"",
	} {
		if e, ok := parseRefLine(line); ok {
			t.Errorf("parseRefLine(%q) accepted a non-checkpoint line: %+v", line, e)
		}
	}
}
