package ecosys

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// PAR-04 (21-02): AGENTS.md/CLAUDE.md auto-injection — the memory discovery
// walker (D-05 collision, D-06 span, D-07 user-global precedence), the
// mtime+size-keyed read cache, and the capped injection body (D-08) that
// sessionFor merges as the fourth trailing System TextBlock. Every failure
// path degrades to a skip-note inside the injection body — nothing here EVER
// returns an error to a caller (a repo must not be able to brick session
// construction; the loader's degrade-softly discipline).
const (
	// Memory file names (D-05: both recognized; CLAUDE.md is the CC-native
	// winner on a same-directory collision).
	claudeMDName = "CLAUDE.md"
	agentsMDName = "AGENTS.md"

	// gitDirName is the repo boundary marker. A FILE counts too (a worktree
	// pointer), so the probe is existence — not IsDir.
	gitDirName = ".git"

	// D-08 discretion values pinned by the plan: per-file truncation cap and
	// the total budget across levels (byte caps enforced BEFORE injection —
	// T-21-07).
	memoryPerFileCapBytes  = 24 << 10 // 24 KB per file
	memoryTotalBudgetBytes = 64 << 10 // 64 KB total budget

	// memoryInjectionHeader frames the injected block (the SkillListing
	// render-then-skip shape: header, blank line, one entry per source).
	memoryInjectionHeader = "The following project memory files apply to this session:"
)

// Note templates rendered inside the injection body (loud observability —
// cutting and skipping are always visible, never silent).
const (
	memoryTruncNoteFmt  = "[note: %s truncated from %d bytes to the %d-byte per-file cap]"
	memoryBudgetNoteFmt = "[note: %s skipped — the %d-byte total memory budget is exhausted]"
)

// MemoryFile is one discovered memory source: a read file (Content non-empty
// or empty-but-read), or a pure skip-note entry (SkipNote != "" — collision
// shadowing, unreadable). Truncated marks a per-file cap cut; OrigBytes is
// always the ORIGINAL file size from stat (equal to len(Content) runes-wise
// only when not truncated).
type MemoryFile struct {
	Path      string
	Level     string
	Content   string
	Truncated bool
	OrigBytes int
	SkipNote  string
}

// DiscoverMemoryFiles walks workDir's memory tree: levels from cwd UP TO the
// directory carrying a .git marker (file or dir — the boundary itself
// contributes; with no .git anywhere only the cwd level is considered —
// D-06's deliberate narrowing vs CC's walk-to-$HOME, which is DEFERRED), each
// level contributing CLAUDE.md if present else AGENTS.md (D-05: both present
// → CLAUDE.md read, AGENTS.md a skip-note; levels accumulate WITHOUT content
// dedup), plus the user global (D-07: ~/.ass-guard/{CLAUDE,AGENTS}.md with
// its own D-05 collision blocks ~/.claude/CLAUDE.md; only an empty
// ~/.ass-guard falls through to ~/.claude/CLAUDE.md). The pinned order is
// user global FIRST, then repo root descending to cwd (deepest last) —
// deterministic for a given tree. Per-file cap applied here; the total budget
// is applied by MemoryInjection. Every degradation is a note, never an error.
func DiscoverMemoryFiles(workDir string) []MemoryFile {
	abs := memoryAbsWorkDir(workDir)
	if abs == "" {
		return nil
	}

	levels := memoryLevels(abs)
	if levels == nil {
		levels = []string{abs} // D-06 no-git fallback: cwd level only
	}

	out := make([]MemoryFile, 0, len(levels)+1)

	// Pinned order: user global first, then repo root → cwd (deepest last).
	out = append(out, userGlobalMemory()...)

	for i := len(levels) - 1; i >= 0; i-- {
		out = append(out, levelMemory(levels[i])...)
	}

	return out
}

// MemoryInjection renders the capped, note-annotated injection body for
// workDir's memory tree (DiscoverMemoryFiles + the D-08 total budget: a file
// whose capped content no longer fits the remaining budget is skipped with a
// note naming it — whole-file skip, never a partial budget cut). Zero files
// yield "" (the render-then-skip contract — the caller skips the merge).
func MemoryInjection(workDir string) string {
	files := DiscoverMemoryFiles(workDir)
	if len(files) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString(memoryInjectionHeader)

	remaining := memoryTotalBudgetBytes

	for _, f := range files {
		b.WriteString("\n\n### " + f.Path + "\n\n")

		if f.SkipNote != "" {
			b.WriteString(f.SkipNote)

			continue
		}

		if len(f.Content) > remaining {
			b.WriteString(fmt.Sprintf(memoryBudgetNoteFmt, f.Path, memoryTotalBudgetBytes))

			continue
		}

		remaining -= len(f.Content)

		if f.Truncated {
			b.WriteString(fmt.Sprintf(memoryTruncNoteFmt, f.Path, f.OrigBytes, memoryPerFileCapBytes))
			b.WriteString("\n\n")
		}

		b.WriteString(f.Content)
	}

	return b.String()
}

// memoryAbsWorkDir resolves the walker's start to an absolute cleaned path
// ("" on resolution failure — no memory rather than a relative walk).
func memoryAbsWorkDir(workDir string) string {
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	abs, err := filepath.Abs(workDir)
	if err != nil {
		return ""
	}

	return abs
}

// memoryLevels collects the ancestor chain from dir (inclusive) UP TO the
// first directory carrying a .git marker (inclusive), in cwd→root order.
// nil means no boundary exists anywhere above — the D-06 cwd-only fallback.
// No memory file above the boundary is ever probed: reads happen only after
// the boundary is fixed (T-21-08).
func memoryLevels(dir string) []string {
	var up []string

	for {
		up = append(up, dir)

		if hasGitMarker(dir) {
			return up
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil // filesystem root without .git — no repo boundary
		}

		dir = parent
	}
}

// hasGitMarker reports whether dir carries a .git entry (file or directory —
// worktree-safe).
func hasGitMarker(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, gitDirName))

	return err == nil
}

// memoryCandidate is one probed memory file path at a level.
type memoryCandidate struct {
	path    string
	present bool // stat OK (file, dir, anything)
	regular bool // stat OK and NOT a directory
}

// probeMemory stats one memory file candidate.
func probeMemory(path string) memoryCandidate {
	info, err := os.Stat(path)
	if err != nil {
		return memoryCandidate{path: path}
	}

	return memoryCandidate{path: path, present: true, regular: !info.IsDir()}
}

// levelMemory resolves ONE level's contribution (D-05): CLAUDE.md if
// readable, else AGENTS.md, else nothing; both present → CLAUDE.md read +
// an AGENTS.md collision note; present-but-unreadable entries (a directory
// named AGENTS.md, a chmod-000 file) degrade to unreadable notes.
func levelMemory(dir string) []MemoryFile {
	claude := probeMemory(filepath.Join(dir, claudeMDName))
	agents := probeMemory(filepath.Join(dir, agentsMDName))

	if claude.regular {
		return append(
			[]MemoryFile{readMemoryEntry(claude.path, dir)},
			collisionNote(agents, claude.path, dir)...,
		)
	}

	if agents.regular {
		out := []MemoryFile{readMemoryEntry(agents.path, dir)}

		if claude.present { // CLAUDE.md on disk but unreadable — the fallback fires
			out = append(out, unreadableNote(claude.path, dir))
		}

		return out
	}

	var out []MemoryFile

	if claude.present {
		out = append(out, unreadableNote(claude.path, dir))
	}

	if agents.present {
		out = append(out, unreadableNote(agents.path, dir))
	}

	return out
}

// collisionNote returns the AGENTS.md shadow note when both names exist in
// one directory (D-05: CLAUDE.md is the CC-native winner).
func collisionNote(agents memoryCandidate, claudePath, dir string) []MemoryFile {
	if !agents.present {
		return nil
	}

	return []MemoryFile{{
		Path:  agents.path,
		Level: dir,
		SkipNote: "[note: " + agents.path + " skipped — " + claudePath +
			" wins the same-directory collision]",
	}}
}

// unreadableNote builds the unreadable skip-note entry (never an error).
func unreadableNote(path, dir string) MemoryFile {
	return MemoryFile{Path: path, Level: dir, SkipNote: "[note: " + path + " skipped — unreadable]"}
}

// userGlobalMemory resolves the D-07 user global: ~/.ass-guard/{CLAUDE,AGENTS}.md
// (its own D-05 collision applies); ANY present candidate there — even an
// unreadable one — blocks ~/.claude/CLAUDE.md (existence-gate win-on-conflict).
// Only an empty ~/.ass-guard falls through to ~/.claude/CLAUDE.md (the one
// name honored under ~/.claude).
func userGlobalMemory() []MemoryFile {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	assguard := levelMemory(filepath.Join(home, assguardDirName))
	if len(assguard) > 0 {
		return assguard
	}

	claudeDir := filepath.Join(home, claudeDirName)
	claude := probeMemory(filepath.Join(claudeDir, claudeMDName))
	if claude.regular {
		return []MemoryFile{readMemoryEntry(claude.path, claudeDir)}
	}

	if claude.present {
		return []MemoryFile{unreadableNote(claude.path, claudeDir)}
	}

	return nil
}

// readMemoryEntry reads one memory file through the mtime+size cache and
// applies the per-file cap (Content capped rune-safe, Truncated set,
// OrigBytes the original size). Every read failure degrades to an
// unreadable note — never an error, never partial content.
func readMemoryEntry(path, level string) MemoryFile {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return unreadableNote(path, level)
	}

	raw, ok := memCachedRead(path, info)
	if !ok {
		return unreadableNote(path, level)
	}

	content, truncated := truncateMemory(raw)

	return MemoryFile{
		Path: path, Level: level, Content: content,
		Truncated: truncated, OrigBytes: int(info.Size()),
	}
}

// memCacheEntry is the cached read of one memory file, keyed by path and
// validated by (modTime, size) — the key trio path+mtime+size.
type memCacheEntry struct {
	modTime time.Time
	size    int64
	content string
}

// The package-level memory cache (sessions are constructed repeatedly against
// the same files — a pure optimization: a key miss always re-reads, content
// is never served stale within the keyed guarantee). memReads counts actual
// file reads (cache misses) — the test hook behind MemoryCacheReads.
var (
	memCacheMu sync.Mutex
	memCache   = map[string]memCacheEntry{} //nolint:gochecknoglobals // package read cache
	memReads   int                          //nolint:gochecknoglobals // test/diagnostic counter
)

// memCachedRead serves path's content from the cache when the path+mtime+size
// key matches, else reads at most memoryPerFileCapBytes+1 bytes (the +1
// detects over-cap — giant files are never fully read; T-21-07) and stores
// the raw prefix.
func memCachedRead(path string, info os.FileInfo) (string, bool) {
	memCacheMu.Lock()

	if e, hit := memCache[path]; hit && e.modTime.Equal(info.ModTime()) && e.size == info.Size() {
		memCacheMu.Unlock()

		return e.content, true
	}

	memCacheMu.Unlock()

	raw, err := readMemoryPrefix(path)
	if err != nil {
		return "", false
	}

	memCacheMu.Lock()
	memCache[path] = memCacheEntry{modTime: info.ModTime(), size: info.Size(), content: raw}
	memReads++
	memCacheMu.Unlock()

	return raw, true
}

// readMemoryPrefix reads at most cap+1 bytes of path (bounded by
// construction — a pathological multi-GB AGENTS.md costs 24 KB + 1).
func readMemoryPrefix(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer f.Close() //nolint:errcheck // read-only handle

	data, err := io.ReadAll(io.LimitReader(f, memoryPerFileCapBytes+1))
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// truncateMemory applies the per-file cap: over-cap content hard-cuts at the
// cap and backs off to a rune boundary (the truncateSkillDesc idiom at file
// scale — a cut never splits a multi-byte rune).
func truncateMemory(content string) (string, bool) {
	if len(content) <= memoryPerFileCapBytes {
		return content, false
	}

	cut := content[:memoryPerFileCapBytes]

	for len(cut) > 0 {
		r, size := utf8.DecodeLastRuneInString(cut)
		if r != utf8.RuneError || size != 1 {
			break // the trailing rune is complete
		}

		cut = cut[:len(cut)-1] // incomplete tail byte — back off
	}

	return cut, true
}

// ResetMemoryCache clears the package-level memory cache and read counter
// (test seam — hermetic cache assertions).
func ResetMemoryCache() {
	memCacheMu.Lock()
	defer memCacheMu.Unlock()

	memCache = map[string]memCacheEntry{}
	memReads = 0
}

// MemoryCacheReads reports the number of actual memory-file reads (cache
// misses) since the last ResetMemoryCache — the cache-hit test hook.
func MemoryCacheReads() int {
	memCacheMu.Lock()
	defer memCacheMu.Unlock()

	return memReads
}
