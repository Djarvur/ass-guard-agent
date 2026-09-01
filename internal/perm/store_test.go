package perm //nolint:testpackage // renameFunc seam injection — the providerfactory config-write convention

import (
	"bytes"
	"errors"
	"fmt"
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

// writersN is the concurrent-writers count for the serialization test.
const writersN = 8

// errRenameBoom is the static failure the rename seam injects (T-17-02).
var errRenameBoom = errors.New("rename boom")

// newStore opens a Store against a temp permissions.yaml so every test is
// isolated. Returns the store and its path for byte-level assertions.
func newStore(t *testing.T) (*Store, string) { //nolint:gocritic // unnamedResult: house config forbids named returns
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
	s.log = slog.New(slog.DiscardHandler)
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

	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("Open did not create %q: %v", path, statErr)
	}

	if got := info.Mode().Perm(); got != filePermOwner {
		t.Errorf("file perm = %v; want %v", got, filePermOwner)
	}

	dirInfo, dirErr := os.Stat(filepath.Dir(path))
	if dirErr != nil {
		t.Fatalf("Stat parent dir: %v", dirErr)
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

	raw := []byte("deny:\n" +
		"    - \"Bash(rm -rf *)\"\n" +
		"ask:\n" +
		"    - \"Write\"\n" +
		"allow:\n" +
		"    - \"Bash(git *)\"\n" +
		"    - mcp__github__get_issue\n")

	err := os.WriteFile(path, raw, filePermOwner)
	if err != nil {
		t.Fatalf("seed file: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	rs := s.Rules()

	cases := []struct {
		tool, arg string
		want      Verdict
	}{
		{testToolBash, "rm -rf /tmp/x", VerdictDeny},
		{testToolWrite, "main.go", VerdictAsk},
		{testToolBash, "git status", VerdictAllow},
		{"mcp__github__get_issue", "", VerdictAllow},
		{testToolBash, "make world", Unmatched},
	}

	for _, tc := range cases {
		if got := rs.Evaluate(tc.tool, tc.arg); got != tc.want {
			t.Errorf("Evaluate(%q, %q) = %v; want %v", tc.tool, tc.arg, got, tc.want)
		}
	}
}

// TestStoreAllowToolPersistsAndSurvivesRestart verifies the allow direction
// of the dialog's write surface (D-01): a simple tool entry lands in the
// allow list, evaluates to VerdictAllow, and survives a fresh Open (restart).
func TestStoreAllowToolPersistsAndSurvivesRestart(t *testing.T) {
	t.Parallel()

	s, path := newStore(t)

	err := s.AllowTool(testToolWrite)
	if err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	if got := s.Rules().Evaluate(testToolWrite, "main.go"); got != VerdictAllow {
		t.Errorf("Evaluate after AllowTool = %v; want VerdictAllow", got)
	}

	reopened, reopenErr := Open(path)
	if reopenErr != nil {
		t.Fatalf("re-Open: %v", reopenErr)
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

	err := s.ForbidTool(testToolWrite)
	if err != nil {
		t.Fatalf("ForbidTool: %v", err)
	}

	err = s.AllowTool(testToolBash)
	if err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	if got := s.Rules().Evaluate(testToolWrite, "main.go"); got != VerdictDeny {
		t.Errorf("Evaluate after ForbidTool = %v; want VerdictDeny", got)
	}

	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
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

	err := s.AllowTool(testToolWrite)
	if err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	before, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}

	err = s.AllowTool(testToolWrite)
	if err != nil {
		t.Fatalf("second AllowTool: %v", err)
	}

	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}

	if !bytes.Equal(before, after) {
		t.Errorf("idempotent AllowTool changed the file:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	if n := strings.Count(string(after), "- "+testToolWrite); n != 1 {
		t.Errorf("entry count = %d; want 1 (no duplicate lines)", n)
	}

	err = s.ForbidTool(testToolWrite)
	if err != nil {
		t.Fatalf("ForbidTool: %v", err)
	}

	err = s.ForbidTool(testToolWrite)
	if err != nil {
		t.Fatalf("second ForbidTool: %v", err)
	}

	final, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
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

	err := s.AllowTool("Bash(git *)")
	if err == nil {
		t.Error("AllowTool(specifier) = nil error; want error")
	}

	err = s.AllowTool("mcp__github__get_*")
	if err == nil {
		t.Error("AllowTool(glob) = nil error; want error")
	}

	err = s.ForbidTool("Wri te")
	if err == nil {
		t.Error("ForbidTool(name with space) = nil error; want error")
	}

	err = s.ForbidTool("")
	if err == nil {
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
func TestStoreRenameFailureLeavesFileIntact(t *testing.T) { //nolint:paralleltest // renameFunc seam is global — serial
	s, path := newStore(t)

	err := s.AllowTool("Alpha")
	if err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	before, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}

	saved := renameFunc
	renameFunc = func(_, _ string) error { return errRenameBoom }

	defer func() { renameFunc = saved }()

	err = s.AllowTool("Beta")
	if err == nil {
		t.Fatal("AllowTool with failing rename = nil error; want error")
	}

	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}

	if !bytes.Equal(before, after) {
		t.Errorf("failed rename mutated the file:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	leftovers, globErr := filepath.Glob(path + ".tmp")
	if globErr != nil {
		t.Fatalf("Glob: %v", globErr)
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

	err := os.WriteFile(path+".tmp", []byte("garbage"), filePermOwner)
	if err != nil {
		t.Fatalf("seed tmp: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	err = s.ForbidTool(testToolWrite)
	if err != nil {
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

	raw := []byte("deny:\n" +
		"    - \"Wri te\"\n" +
		"    - \"Write\"\n" +
		"allow:\n" +
		"    - \"Bash(git *)\"\n")

	err := os.WriteFile(path, raw, filePermOwner)
	if err != nil {
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

	err := s.ForbidTool("Bash(rm -rf *)")
	if err == nil {
		t.Fatal("ForbidTool(specifier) = nil error; want error (D-01 boundary)")
	}

	// Seed the specifier rule by hand (operator-owned file), then click.
	raw := []byte("deny:\n" +
		"    - \"Bash(rm -rf *)\"\n" +
		"allow: []\n")

	seedErr := os.WriteFile(path, raw, filePermOwner)
	if seedErr != nil {
		t.Fatalf("seed file: %v", seedErr)
	}

	s2, openErr := Open(path)
	if openErr != nil {
		t.Fatalf("Open: %v", openErr)
	}

	err = s2.AllowTool(testToolWrite)
	if err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
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

	err := s.AllowTool(testToolWrite)
	if err != nil {
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
		wg.Go(func() {
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
		})
	}

	wg.Wait()

	rs := s.Rules()

	half := writersN / 2
	if len(rs.Deny) != half || len(rs.Allow) != half {
		t.Errorf("after concurrent writes: deny=%d allow=%d", len(rs.Deny), len(rs.Allow))
		t.Errorf("want %d/%d", half, half)
	}
}

// TestStoreFilePermHardAfterSave verifies the 0600 permission survives a
// dialog write, not just the floor creation (T-17-02).
func TestStoreFilePermHardAfterSave(t *testing.T) {
	t.Parallel()

	s, path := newStore(t)

	err := s.AllowTool(testToolWrite)
	if err != nil {
		t.Fatalf("AllowTool: %v", err)
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("Stat: %v", statErr)
	}

	if got := info.Mode().Perm(); got != filePermOwner {
		t.Errorf("file perm after save = %v; want %v", got, filePermOwner)
	}
}

// corruptDocFixture is a DOCUMENT-level YAML parse failure (a tab indent —
// invalid YAML everywhere), distinct from a malformed rule LINE (tolerated
// with a Warning).
const corruptDocFixture = "deny:\n\t- \"Write\"\n"

// TestOpenCorruptDocumentTypedError (WR-01): a corrupt document is reported
// through the typed ErrCorrupt sentinel — the key OpenRepaired's quarantine
// branches on — while Open's behavior otherwise stays verbatim.
func TestOpenCorruptDocumentTypedError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "permissions.yaml")

	if werr := os.WriteFile(path, []byte(corruptDocFixture), filePermOwner); werr != nil {
		t.Fatalf("seed corrupt file: %v", werr)
	}

	_, err := Open(path)
	if err == nil {
		t.Fatal("Open succeeded on a corrupt document")
	}

	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("error = %v; want it to wrap ErrCorrupt", err)
	}
}

// TestOpenRepairedQuarantinesCorruptFile (WR-01, the fail-safe degrade): a
// corrupt document is quarantined byte-intact to "<path>.corrupt", the floor
// 0600 file is recreated (dialog writes work again), and the resulting store
// is usable — never the silent rule-less session the raw parse failure used
// to produce.
func TestOpenRepairedQuarantinesCorruptFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	path := filepath.Join(dir, "permissions.yaml")

	raw := []byte("deny:\n    - \"Bash(rm *)\"\nallow:\n    - \"Read\"\n" + corruptDocFixture)

	if werr := os.WriteFile(path, raw, filePermOwner); werr != nil {
		t.Fatalf("seed corrupt file: %v", werr)
	}

	s, err := OpenRepaired(path)
	if err != nil {
		t.Fatalf("OpenRepaired: %v", err)
	}

	silenceStore(s)

	// The unreadable file was quarantined BYTE-INTACT (the operator's evidence).
	quarantined, rerr := os.ReadFile(path + ".corrupt")
	if rerr != nil {
		t.Fatalf("quarantined file missing: %v", rerr)
	}

	if string(quarantined) != string(raw) {
		t.Errorf("quarantined bytes differ from the original file")
	}

	// The floor file was recreated (0600) and dialog writes work.
	if err := s.AllowTool(testToolWrite); err != nil {
		t.Errorf("AllowTool on the recreated store: %v", err)
	}

	info, serr := os.Stat(path)
	if serr != nil {
		t.Fatalf("Stat recreated file: %v", serr)
	}

	if got := info.Mode().Perm(); got != filePermOwner {
		t.Errorf("recreated file perm = %v; want %v", got, filePermOwner)
	}

	// The store starts from the floor (the old rules are quarantined, not
	// loaded) — the LOUD quarantine log is what tells the operator to restore.
	rs := s.Rules()
	if got := rs.Evaluate(testToolBash, "rm x"); got != Unmatched {
		t.Errorf("Evaluate after quarantine = %v; want Unmatched (floor store)", got)
	}
}

// TestOpenRepairedHealthyFileUnchanged (WR-01 scoping): a healthy or
// line-malformed file never takes the quarantine path — OpenRepaired behaves
// exactly like Open.
func TestOpenRepairedHealthyFileUnchanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	path := filepath.Join(dir, "permissions.yaml")

	raw := []byte("deny:\n    - \"Wri te\"\n    - \"Write\"\n")

	if werr := os.WriteFile(path, raw, filePermOwner); werr != nil {
		t.Fatalf("seed file: %v", werr)
	}

	s, err := OpenRepaired(path)
	if err != nil {
		t.Fatalf("OpenRepaired on a line-malformed file: %v", err)
	}

	silenceStore(s)

	if _, serr := os.Stat(path + ".corrupt"); !os.IsNotExist(serr) {
		t.Error("a line-malformed (not document-corrupt) file was quarantined — scoping broken")
	}

	if got := s.Rules().Evaluate(testToolWrite, "x"); got != VerdictDeny {
		t.Errorf("Evaluate = %v; want VerdictDeny (the valid rule loaded)", got)
	}
}

// TestOpenTightensLoosePerms (WR-04): Open on an EXISTING loose file and
// directory tightens them to the 0600/0750 discipline — pre-fix only files the
// store created or rewrote were hard, so a hand-created 0644 permissions.yaml
// stayed world/group-readable indefinitely.
func TestOpenTightensLoosePerms(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "ass-guard")

	if merr := os.MkdirAll(dir, 0o777); merr != nil {
		t.Fatalf("seed loose dir: %v", merr)
	}

	path := filepath.Join(dir, "permissions.yaml")

	raw := []byte("deny:\n    - \"Bash(rm *)\"\n")

	if werr := os.WriteFile(path, raw, 0o644); werr != nil {
		t.Fatalf("seed loose file: %v", werr)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	silenceStore(s)

	info, serr := os.Stat(path)
	if serr != nil {
		t.Fatalf("Stat file: %v", serr)
	}

	if got := info.Mode().Perm(); got != filePermOwner {
		t.Errorf("file perm after Open = %v; want %v (loose 0644 tightened)", got, filePermOwner)
	}

	dinfo, serr := os.Stat(dir)
	if serr != nil {
		t.Fatalf("Stat dir: %v", serr)
	}

	if got := dinfo.Mode().Perm(); got != dirPerm {
		t.Errorf("dir perm after Open = %v; want %v (loose 0777 tightened)", got, dirPerm)
	}

	// The rules still load (the tighten never touches content).
	if got := s.Rules().Evaluate(testToolBash, "rm x"); got != VerdictDeny {
		t.Errorf("Evaluate = %v; want VerdictDeny (rules intact after tighten)", got)
	}
}

// TestOpenKeepsTightPermsQuietShape (WR-04 scoping): an already-tight store
// opens with the discipline unchanged (the tighten is a no-op).
func TestOpenKeepsTightPermsQuietShape(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "permissions.yaml")

	if werr := os.WriteFile(path, []byte("allow:\n    - \"Read\"\n"), filePermOwner); werr != nil {
		t.Fatalf("seed tight file: %v", werr)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	silenceStore(s)

	info, serr := os.Stat(path)
	if serr != nil {
		t.Fatalf("Stat: %v", serr)
	}

	if got := info.Mode().Perm(); got != filePermOwner {
		t.Errorf("file perm = %v; want %v", got, filePermOwner)
	}
}
