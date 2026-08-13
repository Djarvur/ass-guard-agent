package defaults

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// Seed-profile expectations. tools = 103 (catalog drift, STATE.md blocker): the
// dev profiles/zcode/tools.json carries 103 tools, so the embedded seed mirrors
// it byte-for-byte. The plan's stale "77" was superseded by the drift event.
const (
	expectTools   = 103
	expectBlocks  = 3
	expectHeaders = 12
)

// TestWriteTree_MaterializesAllArtifacts asserts WriteTree lays down the full
// runtime tree under root (D-01): the zcode profile (loader-required files),
// openspec.toml, and scheduling.yaml.
func TestWriteTree_MaterializesAllArtifacts(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	if err := WriteTree(tmp, false); err != nil {
		t.Fatalf("WriteTree: %v", err)
	}

	for _, rel := range []string{
		"profiles/zcode/profile.yaml",
		"profiles/zcode/tools.json",
		"profiles/zcode/identity.yaml",
		"profiles/zcode/thinking.json",
		"profiles/zcode/tool_choice.json",
		"profiles/zcode/meta.yaml",
		"profiles/zcode/system/block-0.txt",
		"profiles/zcode/system/block-1.txt",
		"profiles/zcode/system/block-2.txt",
		"openspec.toml",
		"scheduling.yaml",
	} {
		if _, err := os.Stat(filepath.Join(tmp, rel)); err != nil {
			t.Errorf("expected %s materialized under root: %v", rel, err)
		}
	}
}

// TestWriteTree_NonClobberIdempotent asserts a second WriteTree(overwrite=false)
// is a no-op: an operator edit (marker) survives a re-run unchanged (D-04).
func TestWriteTree_NonClobberIdempotent(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	if err := WriteTree(tmp, false); err != nil {
		t.Fatalf("first WriteTree: %v", err)
	}

	// Simulate an operator edit on tools.json.
	marker := []byte("// operator edit — must survive non-clobber\n")
	target := filepath.Join(tmp, "profiles/zcode/tools.json")
	if err := os.WriteFile(target, marker, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	if err := WriteTree(tmp, false); err != nil {
		t.Fatalf("second WriteTree: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}

	if string(got) != string(marker) {
		t.Errorf("non-clobber violated: tools.json overwritten (got %q)", truncate(string(got), 60))
	}
}

// TestWriteTree_OverwriteTrue replaces existing files when overwrite=true.
func TestWriteTree_OverwriteTrue(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	target := filepath.Join(tmp, "openspec.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(target, []byte("stale operator content\n"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	if err := WriteTree(tmp, true); err != nil {
		t.Fatalf("WriteTree overwrite: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if strings.Contains(string(got), "stale operator content") {
		t.Errorf("overwrite failed: openspec.toml still stale")
	}

	if !strings.Contains(string(got), "[openspec]") {
		t.Errorf("overwrite did not restore seed [openspec] content")
	}
}

// TestSeedProfileLoads asserts the embedded seed zcode profile loads via the
// UNCHANGED profile.Loader with the expected tool/block/header counts. This is
// the zero-config proof: the seed satisfies the loader's contract unmodified.
func TestSeedProfileLoads(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	if err := WriteTree(tmp, false); err != nil {
		t.Fatalf("WriteTree: %v", err)
	}

	prof, err := profile.NewLoader(filepath.Join(tmp, "profiles")).Load("zcode")
	if err != nil {
		t.Fatalf("Load zcode from seed: %v", err)
	}

	if len(prof.Tools) != expectTools {
		t.Errorf("tools = %d, want %d (catalog drift — seed mirrors source)", len(prof.Tools), expectTools)
	}

	if len(prof.System) != expectBlocks {
		t.Errorf("system blocks = %d, want %d", len(prof.System), expectBlocks)
	}

	if len(prof.Headers) != expectHeaders {
		t.Errorf("identity headers = %d, want %d", len(prof.Headers), expectHeaders)
	}
}

// TestDriftGuard_SchedulingYAML asserts the embedded seed scheduling.yaml is
// byte-identical to internal/scheduler/defaults/scheduling.yaml (the DIST-03
// floor). The two must not drift — run sync.sh if this fails.
func TestDriftGuard_SchedulingYAML(t *testing.T) {
	t.Parallel()

	seedBytes, err := Seed.ReadFile("seed/scheduling.yaml")
	if err != nil {
		t.Fatalf("read embedded seed/scheduling.yaml: %v", err)
	}

	schedulerFile := repoPath(t, "internal", "scheduler", "defaults", "scheduling.yaml")
	schedulerBytes, err := os.ReadFile(schedulerFile)
	if err != nil {
		t.Fatalf("read scheduler defaults/scheduling.yaml at %s: %v", schedulerFile, err)
	}

	if string(seedBytes) != string(schedulerBytes) {
		t.Errorf("scheduling.yaml drift: seed=%d bytes != scheduler=%d bytes — run ./internal/defaults/seed/sync.sh",
			len(seedBytes), len(schedulerBytes))
	}
}

// TestLeakGuard_NoAbsoluteHomePaths asserts no embedded seed bytes contain a
// build-operator absolute home path (T-06-01, HIGH). Regression net for the
// sync.sh sanitization barrier: fails loudly if /Users/ or /home/ re-appears.
func TestLeakGuard_NoAbsoluteHomePaths(t *testing.T) {
	t.Parallel()

	leaks := []string{"/Users/", "/home/"}

	err := fs.WalkDir(Seed, "seed", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		data, rerr := Seed.ReadFile(path)
		if rerr != nil {
			return rerr
		}

		for _, leak := range leaks {
			if strings.Contains(string(data), leak) {
				t.Errorf("T-06-01 LEAK: embedded %s contains %q", path, leak)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded seed: %v", err)
	}
}

// repoPath resolves a repo-rooted path via runtime.Caller. The test source
// (internal/defaults/defaults_test.go) is two levels below the repo root.
func repoPath(t *testing.T, elem ...string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	root := filepath.Join(filepath.Dir(thisFile), "..", "..")

	return filepath.Join(append([]string{root}, elem...)...)
}

// truncate returns s shortened to at most n runes for readable test output.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "…"
}
