package runtime //nolint:testpackage // internal package test

// The 25-08 kit-side test-double family (the plan's Task 2 artifact): every
// helper the white-box batteries share, expressed in KIT vocabulary only —
// zero app imports (the file that makes the test-exempt D-19 gate scope
// honest: kit test files survive on kit fakes, not on app packages).
//
// Population happens progressively across 25-08's tasks as each battery's
// app-typed fixture retargets; the family ends up covering: the scripted
// provider, the no-op/recording kit Emitters, the fake pattern table /
// learned store / catalog / scheduler / toolkit seams.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ckptLiveTree fingerprints every regular file under dir, skipping the
// .ass-guard root (the store + transcripts live there) — the tree-identity
// lens the /undo walk batteries share (formerly checkpoint_session's).
func ckptLiveTree(t *testing.T, dir string) map[string]string {
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
		t.Fatalf("ckptLiveTree(%s): %v", dir, werr)
	}

	return out
}
