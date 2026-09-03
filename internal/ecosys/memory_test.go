package ecosys_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
)

// PAR-04 memory walker table (21-02 Task 1). Every case pins one D-05..D-08
// rule or a flagged edge-probe row (ordering / empty / adjacency) against a
// t.TempDir tree with HOME redirected to a fresh temp dir. The tests are
// SERIAL (t.Setenv pins HOME; the package cache read-counter is global).

// Pinned D-08 discretion values — deliberately hardcoded here so a constant
// drift fails the table (the package constants stay unexported).
const (
	memPerFileCap = 24576 // 24 KB per file
	memTotalCap   = 65536 // 64 KB total budget (asserted via note text)

	memHeader    = "The following project memory files apply to this session:"
	memNoteIntro = "[note: "
)

// memPinHome redirects HOME to a fresh empty temp dir (no user-global memory)
// and returns it. Serial-only (t.Setenv).
func memPinHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	return home
}

// memMkGitDir plants a .git DIRECTORY at dir (the normal-repo boundary
// marker; the worktree .git FILE has its own case below).
func memMkGitDir(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
}

// memRepoDir creates a temp dir carrying a .git boundary and returns it.
func memRepoDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	memMkGitDir(t, dir)

	return dir
}

// memWrite plants one memory file at path with content.
func memWrite(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// memNoGitAbove skips the test when dir or any ancestor carries a .git marker
// — the no-git fallback asserts the cwd-only span, and an ambient repo above
// the temp tree would widen it (deterministic even under a repo-local TMPDIR).
func memNoGitAbove(t *testing.T, dir string) {
	t.Helper()

	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			t.Skipf("ambient .git at %s — cwd-only span not provable here", dir)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}

		dir = parent
	}
}

// TestMemoryDiscovery_Collision pins D-05: same-directory CLAUDE.md +
// AGENTS.md → CLAUDE.md read, AGENTS.md skipped with EXACTLY one note naming
// the winning file; different levels accumulate (see the no-dedup case).
func TestMemoryDiscovery_Collision(t *testing.T) { //nolint:paralleltest // HOME pin
	memPinHome(t)

	repo := memRepoDir(t)
	claude := filepath.Join(repo, "CLAUDE.md")
	agents := filepath.Join(repo, "AGENTS.md")
	memWrite(t, claude, "claude-wins-the-level")
	memWrite(t, agents, "agents-is-shadowed")

	files := ecosys.DiscoverMemoryFiles(repo)
	if len(files) != 2 {
		t.Fatalf("DiscoverMemoryFiles entries = %d; want 2 (content + collision note)", len(files))
	}

	if files[0].Path != claude || files[0].Content != "claude-wins-the-level" {
		t.Errorf("entry[0] = {%s %q}; want the CLAUDE.md content entry", files[0].Path, files[0].Content)
	}

	if files[0].Truncated {
		t.Error("entry[0].Truncated = true; want false")
	}

	if files[1].Path != agents || files[1].SkipNote == "" {
		t.Errorf("entry[1] = {%s note=%q}; want the AGENTS.md skip-note entry", files[1].Path, files[1].SkipNote)
	}

	if !strings.Contains(files[1].SkipNote, claude) {
		t.Errorf("collision note %q does not name the winning %s", files[1].SkipNote, claude)
	}

	body := ecosys.MemoryInjection(repo)
	if !strings.Contains(body, "claude-wins-the-level") {
		t.Errorf("body missing the CLAUDE.md content:\n%s", body)
	}

	if strings.Contains(body, "agents-is-shadowed") {
		t.Error("body carries the shadowed AGENTS.md content")
	}

	if got := strings.Count(body, memNoteIntro); got != 1 {
		t.Errorf("skip-notes = %d; want exactly 1:\n%s", got, body)
	}
}

// TestMemoryDiscovery_RepoRootStop pins D-06: levels collect from cwd UP TO
// the .git boundary (inclusive) and nothing above the repo is ever read.
func TestMemoryDiscovery_RepoRootStop(t *testing.T) { //nolint:paralleltest // HOME pin
	memPinHome(t)

	outer := t.TempDir()
	repo := filepath.Join(outer, "repo")
	deep := filepath.Join(repo, "deep")

	memMkGitDir(t, repo)
	memWrite(t, filepath.Join(outer, "CLAUDE.md"), "above-the-boundary-memory")
	memWrite(t, filepath.Join(repo, "CLAUDE.md"), "root-level-memory")
	memWrite(t, filepath.Join(deep, "AGENTS.md"), "cwd-level-memory")

	body := ecosys.MemoryInjection(deep)
	for _, want := range []string{"root-level-memory", "cwd-level-memory"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}

	if strings.Contains(body, "above-the-boundary-memory") ||
		strings.Contains(body, filepath.Join(outer, "CLAUDE.md")) {
		t.Errorf("body leaks memory from ABOVE the git boundary (D-06):\n%s", body)
	}
}

// TestMemoryDiscovery_GitFileBoundary pins the worktree-safe boundary: a .git
// FILE (a worktree pointer) terminates the walk exactly like a directory.
func TestMemoryDiscovery_GitFileBoundary(t *testing.T) { //nolint:paralleltest // HOME pin
	memPinHome(t)

	outer := t.TempDir()
	repo := filepath.Join(outer, "repo")

	memWrite(t, filepath.Join(outer, "CLAUDE.md"), "above-worktree-memory")
	memWrite(t, filepath.Join(repo, ".git"), "gitdir: ../elsewhere\n")
	memWrite(t, filepath.Join(repo, "AGENTS.md"), "worktree-root-memory")

	body := ecosys.MemoryInjection(repo)
	if !strings.Contains(body, "worktree-root-memory") {
		t.Errorf("body missing the worktree-root memory:\n%s", body)
	}

	if strings.Contains(body, "above-worktree-memory") {
		t.Errorf("body leaks memory from above a .git FILE boundary:\n%s", body)
	}
}

// TestMemoryDiscovery_NoGitFallback pins the D-06 narrowing: with no .git
// anywhere above, ONLY the cwd level is considered — never a walk to /.
func TestMemoryDiscovery_NoGitFallback(t *testing.T) { //nolint:paralleltest // HOME pin
	memPinHome(t)

	tree := t.TempDir()
	memNoGitAbove(t, tree)
	cwd := filepath.Join(tree, "sub")

	memWrite(t, filepath.Join(tree, "CLAUDE.md"), "parent-level-memory-ignored")
	memWrite(t, filepath.Join(cwd, "AGENTS.md"), "cwd-level-memory-only")

	body := ecosys.MemoryInjection(cwd)
	if !strings.Contains(body, "cwd-level-memory-only") {
		t.Errorf("body missing the cwd-level memory:\n%s", body)
	}

	if strings.Contains(body, "parent-level-memory-ignored") {
		t.Errorf("no-git fallback walked ABOVE cwd (D-06 narrowing violated):\n%s", body)
	}
}

// TestMemoryDiscovery_UserGlobalPrecedence pins D-07's four HOME
// combinations: ~/.ass-guard/{CLAUDE,AGENTS}.md resolves its own D-05
// collision and BLOCKS ~/.claude/CLAUDE.md; only when ~/.ass-guard has
// neither file does ~/.claude/CLAUDE.md join.
func TestMemoryDiscovery_UserGlobalPrecedence(t *testing.T) { //nolint:paralleltest,funlen // HOME pin
	cases := []struct {
		name      string
		assguard  [2]string // {CLAUDE.md content, AGENTS.md content}; "" = absent
		claudeMD  string    // ~/.claude/CLAUDE.md content; "" = absent
		wantIn    []string
		wantNotIn []string
	}{
		{
			name: "both in assguard — CLAUDE wins, user-claude blocked", //nolint:dupl // table rows
			assguard: [2]string{"ag-claude-wins", "ag-agents-shadowed"},
			claudeMD: "user-claude-memory",
			wantIn:   []string{"ag-claude-wins"},
			wantNotIn: []string{
				"ag-agents-shadowed", "user-claude-memory",
			},
		},
		{
			name:      "assguard CLAUDE only — user-claude blocked",
			assguard:  [2]string{"ag-claude-only", ""},
			claudeMD:  "user-claude-memory",
			wantIn:    []string{"ag-claude-only"},
			wantNotIn: []string{"user-claude-memory"},
		},
		{
			name:      "assguard AGENTS only — user-claude blocked",
			assguard:  [2]string{"", "ag-agents-only"},
			claudeMD:  "user-claude-memory",
			wantIn:    []string{"ag-agents-only"},
			wantNotIn: []string{"user-claude-memory"},
		},
		{
			name:     "assguard empty — user-claude read",
			assguard: [2]string{"", ""},
			claudeMD: "user-claude-memory",
			wantIn:   []string{"user-claude-memory"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { //nolint:paralleltest // HOME pin
			home := memPinHome(t)
			repo := memRepoDir(t)
			memWrite(t, filepath.Join(repo, "AGENTS.md"), "repo-level-memory")

			if tc.assguard[0] != "" {
				memWrite(t, filepath.Join(home, ".ass-guard", "CLAUDE.md"), tc.assguard[0])
			}

			if tc.assguard[1] != "" {
				memWrite(t, filepath.Join(home, ".ass-guard", "AGENTS.md"), tc.assguard[1])
			}

			if tc.claudeMD != "" {
				memWrite(t, filepath.Join(home, ".claude", "CLAUDE.md"), tc.claudeMD)
			}

			body := ecosys.MemoryInjection(repo)

			for _, want := range append([]string{"repo-level-memory"}, tc.wantIn...) {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q:\n%s", want, body)
				}
			}

			for _, banned := range tc.wantNotIn {
				if strings.Contains(body, banned) {
					t.Errorf("body carries %q (D-07 precedence violated):\n%s", banned, body)
				}
			}
		})
	}
}

// TestMemoryDiscovery_Ordering pins the flagged ordering row: user global
// FIRST, then repo root descending to cwd (deepest LAST) — the injection is
// deterministic for a given tree.
func TestMemoryDiscovery_Ordering(t *testing.T) { //nolint:paralleltest // HOME pin
	home := memPinHome(t)

	repo := memRepoDir(t)
	mid := filepath.Join(repo, "mid")
	deep := filepath.Join(repo, "mid", "deep")

	memWrite(t, filepath.Join(home, ".claude", "CLAUDE.md"), "user-global-first")
	memWrite(t, filepath.Join(repo, "CLAUDE.md"), "root-second")
	memWrite(t, filepath.Join(mid, "CLAUDE.md"), "mid-third")
	memWrite(t, filepath.Join(deep, "AGENTS.md"), "deep-last")

	body := ecosys.MemoryInjection(deep)

	last := -1

	for _, marker := range []string{"user-global-first", "root-second", "mid-third", "deep-last"} {
		idx := strings.Index(body, marker)
		if idx < 0 {
			t.Fatalf("body missing %q:\n%s", marker, body)
		}

		if idx < last {
			t.Errorf("marker %q at %d precedes the previous one (pinned order broken):\n%s", marker, idx, body)
		}

		last = idx
	}
}

// TestMemoryDiscovery_AccumulateNoDedup pins the flagged adjacency row's
// accumulate half: IDENTICAL bodies at two levels inject TWICE, each under
// its own level entry — no content dedup.
func TestMemoryDiscovery_AccumulateNoDedup(t *testing.T) { //nolint:paralleltest // HOME pin
	memPinHome(t)

	repo := memRepoDir(t)
	deep := filepath.Join(repo, "deep")
	const twin = "twin-memory-body"

	memWrite(t, filepath.Join(repo, "CLAUDE.md"), twin)
	memWrite(t, filepath.Join(deep, "AGENTS.md"), twin)

	body := ecosys.MemoryInjection(repo)
	if got := strings.Count(body, twin); got != 2 {
		t.Errorf("twin body occurrences = %d; want 2 (levels accumulate without dedup):\n%s", got, body)
	}
}

// TestMemoryDiscovery_PerFileCap pins the D-08 per-file cap: a >24 KB file
// truncates rune-safe with a loud note carrying the ORIGINAL byte count.
func TestMemoryDiscovery_PerFileCap(t *testing.T) { //nolint:paralleltest // HOME pin
	t.Run("ascii hard cut", func(t *testing.T) { //nolint:paralleltest // HOME pin
		memPinHome(t)

		repo := memRepoDir(t)
		memWrite(t, filepath.Join(repo, "CLAUDE.md"), strings.Repeat("a", 30000))

		files := ecosys.DiscoverMemoryFiles(repo)
		if len(files) != 1 || !files[0].Truncated {
			t.Fatalf("entries = %+v; want one truncated entry", files)
		}

		if got := len(files[0].Content); got != memPerFileCap {
			t.Errorf("capped content len = %d; want %d", got, memPerFileCap)
		}

		if files[0].OrigBytes != 30000 {
			t.Errorf("OrigBytes = %d; want 30000", files[0].OrigBytes)
		}

		body := ecosys.MemoryInjection(repo)
		if !strings.Contains(body, "truncated from 30000 bytes to the 24576-byte per-file cap") {
			t.Errorf("body missing the original-count truncation note:\n%s", body[:min(400, len(body))])
		}

		if strings.Contains(body, strings.Repeat("a", memPerFileCap+1)) {
			t.Error("content exceeded the per-file cap")
		}
	})

	t.Run("rune-safe back-off", func(t *testing.T) { //nolint:paralleltest // HOME pin
		memPinHome(t)

		repo := memRepoDir(t)
		// "x" + 12288×"é" = 24577 bytes; the byte cut at 24576 lands INSIDE
		// the final é → backs off one byte to 24575 (complete runes only).
		memWrite(t, filepath.Join(repo, "AGENTS.md"), "x"+strings.Repeat("é", 12288))

		files := ecosys.DiscoverMemoryFiles(repo)
		if len(files) != 1 || !files[0].Truncated {
			t.Fatalf("entries = %+v; want one truncated entry", files)
		}

		if !utf8.ValidString(files[0].Content) {
			t.Error("truncated content splits a multi-byte rune")
		}

		if got := len(files[0].Content); got != 24575 {
			t.Errorf("rune-safe cut len = %d; want 24575", got)
		}
	})
}

// TestMemoryDiscovery_TotalBudget pins the D-08 total budget: once the 64 KB
// budget is exhausted the remaining files are skipped, each NAMED in a note.
func TestMemoryDiscovery_TotalBudget(t *testing.T) { //nolint:paralleltest // HOME pin
	memPinHome(t)

	repo := memRepoDir(t)
	m1 := filepath.Join(repo, "m1")
	m2 := filepath.Join(repo, "m1", "m2")
	deep := filepath.Join(repo, "m1", "m2", "deep")

	memWrite(t, filepath.Join(repo, "CLAUDE.md"), strings.Repeat("a", 30000)) // fits (capped)
	memWrite(t, filepath.Join(m1, "CLAUDE.md"), strings.Repeat("b", 30000))   // fits (capped)
	skipped1 := filepath.Join(m2, "AGENTS.md")
	skipped2 := filepath.Join(deep, "AGENTS.md")
	memWrite(t, skipped1, strings.Repeat("c", 30000)) // budget gone
	memWrite(t, skipped2, strings.Repeat("d", 30000)) // budget gone

	body := ecosys.MemoryInjection(deep)

	if strings.Contains(body, "cccc") || strings.Contains(body, "dddd") {
		t.Error("body carries over-budget file content")
	}

	if got := strings.Count(body, "total memory budget is exhausted"); got != 2 {
		t.Errorf("budget-skip notes = %d; want 2:\n%s", got, body)
	}

	for _, skipped := range []string{skipped1, skipped2} {
		note := memNoteIntro + skipped + " skipped"
		if !strings.Contains(body, note) {
			t.Errorf("body does not NAME the skipped file %s:\n%s", skipped, body)
		}
	}

	// The first two files DID ride (capped) — the budget is not a global off switch.
	if !strings.Contains(body, strings.Repeat("a", memPerFileCap)) ||
		!strings.Contains(body, strings.Repeat("b", memPerFileCap)) {
		t.Error("body missing the in-budget (capped) file contents")
	}
}

// TestMemoryDiscovery_EmptyTree pins the render-then-skip contract: zero
// memory files anywhere (repo + HOME) → "" — the caller never merges a block.
func TestMemoryDiscovery_EmptyTree(t *testing.T) { //nolint:paralleltest // HOME pin
	memPinHome(t)

	repo := memRepoDir(t)

	if got := ecosys.MemoryInjection(repo); got != "" {
		t.Errorf("MemoryInjection = %q; want \"\" for an empty tree", got)
	}

	if files := ecosys.DiscoverMemoryFiles(repo); len(files) != 0 {
		t.Errorf("DiscoverMemoryFiles = %+v; want none", files)
	}
}

// TestMemoryDiscovery_UnreadableEntries pins the degrade-softly row: a
// directory named AGENTS.md and an unreadable (chmod 000) file are skipped
// with a note — never an error, never content.
func TestMemoryDiscovery_UnreadableEntries(t *testing.T) { //nolint:paralleltest // HOME pin
	memPinHome(t)

	repo := memRepoDir(t)

	// A DIRECTORY named AGENTS.md — present on disk but not a file.
	agentsDir := filepath.Join(repo, "AGENTS.md")
	if err := os.MkdirAll(agentsDir, 0o750); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}

	files := ecosys.DiscoverMemoryFiles(repo)
	if len(files) != 1 || files[0].Path != agentsDir || files[0].SkipNote == "" {
		t.Fatalf("entries = %+v; want one unreadable skip-note for %s", files, agentsDir)
	}

	if strings.Contains(files[0].SkipNote, "collision") {
		t.Errorf("note %q claims a collision; want unreadable", files[0].SkipNote)
	}

	// A chmod-000 CLAUDE.md beside it: both levels degrade to notes.
	lockPath := filepath.Join(repo, "CLAUDE.md")
	memWrite(t, lockPath, "locked-memory")

	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 000 does not block reads")
	}

	if err := os.Chmod(lockPath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(lockPath, 0o600) }) //nolint:errcheck // best-effort restore

	files = ecosys.DiscoverMemoryFiles(repo)
	if len(files) != 2 {
		t.Fatalf("entries = %d; want 2 unreadable notes:\n%+v", len(files), files)
	}

	for _, f := range files {
		if f.Content != "" {
			t.Errorf("entry %s carried content from an unreadable tree: %q", f.Path, f.Content)
		}
	}
}

// TestMemoryDiscovery_Cache pins the mtime+size cache: unchanged files are
// served without re-reading; a touched file (new mtime) re-reads; a
// size-changed file with an IDENTICAL mtime also re-reads — the key is
// path+mtime+size (a pure optimization; a miss always re-reads).
func TestMemoryDiscovery_Cache(t *testing.T) { //nolint:paralleltest // HOME pin
	memPinHome(t)

	repo := memRepoDir(t)
	path := filepath.Join(repo, "CLAUDE.md")

	t0 := time.Now().Add(-2 * time.Hour)
	t1 := time.Now().Add(-1 * time.Hour)

	memWrite(t, path, "cache-v1")
	if err := os.Chtimes(path, t0, t0); err != nil {
		t.Fatalf("chtimes t0: %v", err)
	}

	ecosys.ResetMemoryCache()

	if files := ecosys.DiscoverMemoryFiles(repo); len(files) != 1 || files[0].Content != "cache-v1" {
		t.Fatalf("first discovery = %+v; want cache-v1", files)
	}

	if got := ecosys.MemoryCacheReads(); got != 1 {
		t.Fatalf("reads after first discovery = %d; want 1", got)
	}

	// Unchanged mtime+size → cache hit, zero new reads.
	_ = ecosys.DiscoverMemoryFiles(repo)
	if got := ecosys.MemoryCacheReads(); got != 1 {
		t.Fatalf("reads after repeat discovery = %d; want 1 (cache hit)", got)
	}

	// New mtime (same size) → miss → re-read → fresh content.
	memWrite(t, path, "cache-v2")
	if err := os.Chtimes(path, t1, t1); err != nil {
		t.Fatalf("chtimes t1: %v", err)
	}

	if files := ecosys.DiscoverMemoryFiles(repo); files[0].Content != "cache-v2" {
		t.Fatalf("content after touch = %q; want cache-v2 (edits picked up)", files[0].Content)
	}

	if got := ecosys.MemoryCacheReads(); got != 2 {
		t.Fatalf("reads after touch = %d; want 2", got)
	}

	// Same mtime, different SIZE → still a miss (size is part of the key).
	memWrite(t, path, "cache-v3-longer")
	if err := os.Chtimes(path, t1, t1); err != nil {
		t.Fatalf("chtimes t1 restore: %v", err)
	}

	if files := ecosys.DiscoverMemoryFiles(repo); files[0].Content != "cache-v3-longer" {
		t.Fatalf("content after size change = %q; want cache-v3-longer", files[0].Content)
	}

	if got := ecosys.MemoryCacheReads(); got != 3 {
		t.Fatalf("reads after size change = %d; want 3", got)
	}
}
