package defaults_test

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/defaults"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/scheduler"
)

// Seed-profile expectations. tools = 79: the 09-04/AUD-05 re-pin (zcode
// 0.16.3, GLM-5.3) superseded the old 103-tool catalog; the embedded seed
// mirrors the re-pinned dev profiles/zcode byte-for-byte (sync.sh).
const (
	expectTools   = 79
	expectBlocks  = 3
	expectHeaders = 12
)

// TestWriteTree_MaterializesAllArtifacts asserts WriteTree lays down the full
// runtime tree under root (D-01): the zcode profile (loader-required files),
// openspec.toml, and config.yaml.
func TestWriteTree_MaterializesAllArtifacts(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	err := defaults.WriteTree(tmp, false)
	if err != nil {
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
		"config.yaml",
	} {
		_, err := os.Stat(filepath.Join(tmp, rel))
		if err != nil {
			t.Errorf("expected %s materialized under root: %v", rel, err)
		}
	}
}

// TestWriteTree_NonClobberIdempotent asserts a second WriteTree(overwrite=false)
// is a no-op: an operator edit (marker) survives a re-run unchanged (D-04).
func TestWriteTree_NonClobberIdempotent(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	err := defaults.WriteTree(tmp, false)
	if err != nil {
		t.Fatalf("first WriteTree: %v", err)
	}

	// Simulate an operator edit on tools.json.
	marker := []byte("// operator edit — must survive non-clobber\n")
	target := filepath.Join(tmp, "profiles", "zcode", "tools.json")

	err = os.WriteFile(target, marker, 0o644)
	if err != nil {
		t.Fatalf("write marker: %v", err)
	}

	err = defaults.WriteTree(tmp, false)
	if err != nil {
		t.Fatalf("second WriteTree: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}

	if !bytes.Equal(got, marker) {
		t.Errorf("non-clobber violated: tools.json overwritten (got %q)", truncate(string(got), 60))
	}
}

// TestWriteTree_OverwriteTrue replaces existing files when overwrite=true.
func TestWriteTree_OverwriteTrue(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	target := filepath.Join(tmp, "openspec.toml")

	err := os.MkdirAll(filepath.Dir(target), 0o755)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err = os.WriteFile(target, []byte("stale operator content\n"), 0o644)
	if err != nil {
		t.Fatalf("write stale: %v", err)
	}

	err = defaults.WriteTree(tmp, true)
	if err != nil {
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

	err := defaults.WriteTree(tmp, false)
	if err != nil {
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

// TestDriftGuard_SchedulingYAML asserts the embedded seed config.yaml is
// byte-identical to internal/scheduler's embedded default (the DIST-03 floor).
// Both sides are build-time embed bytes (no filesystem lookup), so this is
// robust under -trimpath. Run sync.sh if this fails.
func TestDriftGuard_SchedulingYAML(t *testing.T) {
	t.Parallel()

	seedBytes, err := defaults.Seed.ReadFile("seed/config.yaml")
	if err != nil {
		t.Fatalf("read embedded seed/config.yaml: %v", err)
	}

	schedulerBytes := scheduler.EmbeddedDefaultScheduling()

	if !bytes.Equal(seedBytes, schedulerBytes) {
		t.Errorf("config.yaml drift: seed=%d bytes != scheduler=%d bytes — run ./internal/defaults/seed/sync.sh",
			len(seedBytes), len(schedulerBytes))
	}
}

// TestLeakGuard_NoAbsoluteHomePaths asserts no embedded seed bytes contain a
// build-operator absolute home path (T-06-01, HIGH). Regression net for the
// sync.sh sanitization barrier: fails loudly if /Users/ or /home/ re-appears.
func TestLeakGuard_NoAbsoluteHomePaths(t *testing.T) {
	t.Parallel()

	leaks := []string{"/Users/", "/home/"}

	err := fs.WalkDir(defaults.Seed, "seed", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", path, walkErr)
		}

		if d.IsDir() {
			return nil
		}

		data, rerr := defaults.Seed.ReadFile(path)
		if rerr != nil {
			return fmt.Errorf("read %s: %w", path, rerr)
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

// truncate returns s shortened to at most n runes for readable test output.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "…"
}
