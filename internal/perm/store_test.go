package perm //nolint:testpackage // renameFunc seam injection — the providerfactory config-write convention

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Dialog-click simple-entry fixtures (distinct from the rules_test battery's
// constants — this file is the internal test package).
const (
	testToolWrite = "Write"
	testToolBash  = "Bash"
)

// discardLogger builds a logger that swallows output (warnings stay
// assertable via Warnings() without flooding test logs).
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newStore opens a Store against a temp permissions.yaml so every test is
// isolated. Returns the store and its path for byte-level assertions.
func newStore(t *testing.T) (*Store, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "permissions.yaml")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	return s, path
}

// silenceStore keeps load-time warning logs out of test output (the warnings
// themselves are asserted via Warnings()).
func silenceStore(s *Store) {
	s.log = discardLogger()
}

// TestStoreOpenCreatesEmptyFloor verifies Open on an absent path creates the
// parent dirs (0750) and an empty-valid file (0600) — the zero-config floor.
func TestStoreOpenCreatesEmptyFloor(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "permissions.yaml")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Open did not create %q: %v", path, err)
	}

	if got := info.Mode().Perm(); got != filePermOwner {
		t.Errorf("file perm = %v; want %v", got, filePermOwner)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat parent dir: %v", err)
	}

	if got := dirInfo.Mode().Perm(); got != dirPerm {
		t.Errorf("dir perm = %v; want %v", got, dirPerm)
	}

	if rs := s.Rules(); len(rs.Deny)+len(rs.Ask)+len(rs.Allow) != 0 {
		t.Errorf("fresh store rules = %+v; want empty", rs)
	}
}

// TestStoreOpenLoadsExisting verifies Open loads a hand-written file's
// deny/ask/allow lists verbatim — including richer specifier rules that the
// dialog would never write (D-01: the file supports the full grammar).
func TestStoreOpenLoadsExisting(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "permissions.yaml")
	raw := []byte("deny:\n    - \"Bash(rm -rf *)\"\nask:\n    - \"Write\"\nallow:\n    - \"Bash(git *)\"\n    - mcp__github__get_issue\n")
	if err := os.WriteFile(path, raw, filePermOwner); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	rs := s.Rules()

	if got := rs.Evaluate("Bash", "rm -rf /tmp/x"); got != VerdictDeny {
		t.Errorf("Evaluate(deny rule) = %v; want VerdictDeny", got)
	}

	if got := rs.Evaluate("Write", "main.go"); got != VerdictAsk {
		t.Errorf("Evaluate(ask rule) = %v; want VerdictAsk", got)
	}

	if got := rs.Evaluate("Bash", "git status"); got != VerdictAllow {
		t.Errorf("Evaluate(allow rule) = %v; want VerdictAllow", got)
	}

	if got := rs.Evaluate("mcp__github__get_issue", ""); got != VerdictAllow {
		t.Errorf("Evaluate(mcp allow rule) = %v; want VerdictAllow", got)
	}

	if got := rs.Evaluate("Bash", "make world"); got != Unmatched {
		t.Errorf("Evaluate(unmatched) = %v; want Unmatched", got)
	}
}

// TestStoreAllowToolPersistsAndSurvivesRestart verifies the allow direction
// of the dialog's write surface (D-01): a simple tool entry lands in the
// allow list, evaluates to VerdictAllow, and survives a fresh Open (restart).
func TestStoreAllowToolPersistsAndSurvivesRestart(t *testing.T) {
	t.Parallel()

	s, path := newStore(t)

	if err := s.AllowTool(testToolWrite); err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	if got := s.Rules().Evaluate(testToolWrite, "main.go"); got != VerdictAllow {
		t.Errorf("Evaluate after AllowTool = %v; want VerdictAllow", got)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}

	if got := reopened.Rules().Evaluate(testToolWrite, "main.go"); got != VerdictAllow {
		t.Errorf("Evaluate after restart = %v; want VerdictAllow (persists across restarts)", got)
	}
}

// TestStoreForbidToolPersists verifies the deny direction (D-03, the
// reject-also-always shape) and that both directions coexist in one file.
func TestStoreForbidToolPersists(t *testing.T) {
	t.Parallel()

	s, path := newStore(t)

	if err := s.ForbidTool(testToolWrite); err != nil {
		t.Fatalf("ForbidTool: %v", err)
	}

	if err := s.AllowTool(testToolBash); err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	if got := s.Rules().Evaluate(testToolWrite, "main.go"); got != VerdictDeny {
		t.Errorf("Evaluate after ForbidTool = %v; want VerdictDeny", got)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	for _, want := range []string{"- " + testToolWrite, "- " + testToolBash} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("file missing %q:\n%s", want, raw)
		}
	}
}

// TestStoreIdempotentNoop verifies an AllowTool/ForbidTool click on an
// already-present identical entry is a byte-level no-op — repeated
// always-clicks converge to one entry (save/load round-trip never duplicates).
func TestStoreIdempotentNoop(t *testing.T) {
	t.Parallel()

	s, path := newStore(t)

	if err := s.AllowTool(testToolWrite); err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if err := s.AllowTool(testToolWrite); err != nil {
		t.Fatalf("second AllowTool: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !bytes.Equal(before, after) {
		t.Errorf("idempotent AllowTool changed the file:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	if n := strings.Count(string(after), "- "+testToolWrite); n != 1 {
		t.Errorf("entry count = %d; want 1 (no duplicate lines)", n)
	}

	if err := s.ForbidTool(testToolWrite); err != nil {
		t.Fatalf("ForbidTool: %v", err)
	}

	if err := s.ForbidTool(testToolWrite); err != nil {
		t.Fatalf("second ForbidTool: %v", err)
	}

	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if n := strings.Count(string(final), "- "+testToolWrite); n != 2 {
		t.Errorf("entries across both lists = %d; want 2 (one allow + one deny)", n)
	}
}

// TestStoreRejectsNonSimpleEntries pins D-01's boundary: the store exposes
// NO API that writes specifier rules — richer shapes are rejected, file
// untouched.
func TestStoreRejectsNonSimpleEntries(t *testing.T) {
	t.Parallel()

	s, path := newStore(t)

	if err := s.AllowTool("Bash(git *)"); err == nil {
		t.Error("AllowTool(specifier) = nil error; want error")
	}

	if err := s.AllowTool("mcp__github__get_*"); err == nil {
		t.Error("AllowTool(glob) = nil error; want error")
	}

	if err := s.ForbidTool("Wri te"); err == nil {
		t.Error("ForbidTool(name with space) = nil error; want error")
	}

	if err := s.ForbidTool(""); err == nil {
		t.Error("ForbidTool(empty) = nil error; want error")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if strings.Contains(string(raw), "Bash") {
		t.Errorf("rejected entries leaked into the file:\n%s", raw)
	}
}

// TestStoreRenameFailureLeavesFileIntact proves the atomic discipline: a
// failed rename returns an error and leaves the prior file byte-identical
// with no temp leftover (T-17-02).
func TestStoreRenameFailureLeavesFileIntact(t *testing.T) { //nolint:paralleltest // renameFunc seam is package-global — serial family
	s, path := newStore(t)

	if err := s.AllowTool("Alpha"); err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	saved := renameFunc
	renameFunc = func(_, _ string) error { return errors.New("boom") }
	defer func() { renameFunc = saved }()

	if err := s.AllowTool("Beta"); err == nil {
		t.Fatal("AllowTool with failing rename = nil error; want error")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !bytes.Equal(before, after) {
		t.Errorf("failed rename mutated the file:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	leftovers, err := filepath.Glob(path + ".tmp")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}

	if len(leftovers) != 0 {
		t.Errorf("temp leftovers survived: %v", leftovers)
	}

	if got := s.Rules().Evaluate("Alpha", ""); got != VerdictAllow {
		t.Errorf("Evaluate(Alpha) = %v; want VerdictAllow (in-memory state rolled back)", got)
	}
}

// TestStoreStaleTmpIgnored proves a leftover tmp file (crash residue) neither
// corrupts loading nor blocks a subsequent atomic save.
func TestStoreStaleTmpIgnored(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "permissions.yaml")

	if err := os.WriteFile(path+".tmp", []byte("garbage"), filePermOwner); err != nil {
		t.Fatalf("seed tmp: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := s.ForbidTool(testToolWrite); err != nil {
		t.Fatalf("ForbidTool: %v", err)
	}

	if got := s.Rules().Evaluate(testToolWrite, "x"); got != VerdictDeny {
		t.Errorf("Evaluate = %v; want VerdictDeny (stale tmp ignored)", got)
	}
}

// TestStoreMalformedLineTolerated verifies the hand-edit surface: a malformed
// line loads as skipped with one structured warning, and valid lines around
// it load normally.
func TestStoreMalformedLineTolerated(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "permissions.yaml")
	raw := []byte("deny:\n    - \"Wri te\"\n    - \"Write\"\nallow:\n    - \"Bash(git *)\"\n")
	if err := os.WriteFile(path, raw, filePermOwner); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	silenceStore(s)

	warns := s.Warnings()
	if len(warns) != 1 {
		t.Fatalf("Warnings = %+v; want exactly 1", warns)
	}

	if warns[0].Rule != "Wri te" || warns[0].Reason == "" {
		t.Errorf("warning = %+v; want rule \"Wri te\" with a reason", warns[0])
	}

	rs := s.Rules()

	if got := rs.Evaluate(testToolWrite, "x"); got != VerdictDeny {
		t.Errorf("Evaluate(valid deny around malformed) = %v; want VerdictDeny", got)
	}

	if got := rs.Evaluate("Bash", "git status"); got != VerdictAllow {
		t.Errorf("Evaluate(valid allow around malformed) = %v; want VerdictAllow", got)
	}
}

// TestStoreRicherRulesSurviveDialogWrites verifies a dialog write preserves
// hand-edited specifier rules (the file's read surface is the full grammar;
// the dialog's write surface is only simple entries).
func TestStoreRicherRulesSurviveDialogWrites(t *testing.T) {
	t.Parallel()

	s, path := newStore(t)

	if err := s.ForbidTool("Bash(rm -rf *)"); err == nil {
		t.Fatal("ForbidTool(specifier) = nil error; want error (D-01 boundary)")
	}

	// Seed the specifier rule by hand (operator-owned file), then click.
	raw := []byte("deny:\n    - \"Bash(rm -rf *)\"\nallow: []\n")
	if err := os.WriteFile(path, raw, filePermOwner); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := s2.AllowTool(testToolWrite); err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !strings.Contains(string(after), "Bash(rm -rf *)") {
		t.Errorf("hand-edited specifier rule lost on dialog write:\n%s", after)
	}

	if got := s2.Rules().Evaluate("Bash", "rm -rf /tmp/x"); got != VerdictDeny {
		t.Errorf("Evaluate(specifier deny after dialog write) = %v; want VerdictDeny", got)
	}
}

// TestStoreCopyOnRead verifies Rules() returns a snapshot — mutating it
// cannot touch the store's state.
func TestStoreCopyOnRead(t *testing.T) {
	t.Parallel()

	s, _ := newStore(t)

	if err := s.AllowTool(testToolWrite); err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	snapshot := s.Rules()
	snapshot.Allow[0] = Rule{Tool: "Mutated"}
	snapshot.Allow = append(snapshot.Allow, Rule{Tool: "Extra"})

	fresh := s.Rules()

	if len(fresh.Allow) != 1 || fresh.Allow[0].Tool != testToolWrite {
		t.Errorf("store state mutated through snapshot: %+v", fresh.Allow)
	}
}

// TestStoreConcurrentWritesSerialize verifies concurrent dialog clicks
// serialize under the store mutex and every entry lands (race detector's
// job is the -race run).
func TestStoreConcurrentWritesSerialize(t *testing.T) {
	t.Parallel()

	s, _ := newStore(t)

	var wg sync.WaitGroup

	for i := range writersN {
		wg.Add(1)

		go func() {
			defer wg.Done()

			name := fmt.Sprintf("Tool%d", i)

			var err error
			if i%2 == 0 {
				err = s.AllowTool(name)
			} else {
				err = s.ForbidTool(name)
			}

			if err != nil {
				t.Errorf("write %q: %v", name, err)
			}
		}()
	}

	wg.Wait()

	rs := s.Rules()
	if len(rs.Deny) != writersN/2 || len(rs.Allow) != writersN/2 {
		t.Errorf("after concurrent writes: deny=%d allow=%d; want %d/%d", len(rs.Deny), len(rs.Allow), writersN/2, writersN/2)
	}
}

// TestStoreFilePermHardAfterSave verifies the 0600 permission survives a
// dialog write, not just the floor creation (T-17-02).
func TestStoreFilePermHardAfterSave(t *testing.T) {
	t.Parallel()

	s, path := newStore(t)

	if err := s.AllowTool(testToolWrite); err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if got := info.Mode().Perm(); got != filePermOwner {
		t.Errorf("file perm after save = %v; want %v", got, filePermOwner)
	}
}

// writersN is the concurrent-writers count for the serialization test.
const writersN = 8
