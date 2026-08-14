// Package defaults embeds the zero-config default artifacts (D-01) into the
// binary via go:embed: a pre-seeded zcode profile, a pre-seeded openspec.toml
// (OpenSpec handoff schema skeleton), and a default scheduling.yaml (the
// DIST-03 zero-config floor). WriteTree materializes them onto disk under a
// root directory (used by internal/firstrun on first launch).
//
// The seed tree lives under internal/defaults/seed/ and is regenerated from
// canonical sources by seed/sync.sh, which sanitizes build-operator absolute
// paths (T-06-01, HIGH) before they reach the embed. The leak-guard test in
// defaults_test.go fails loudly if /Users/ or /home/ re-appears in the embed.
package defaults

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Seed is the embedded zero-config default tree (D-01). It carries the runtime
// artifacts only (sync.sh is a build tool and is intentionally NOT embedded).
// The embed reaches seed/profiles (the whole zcode profile subtree, including
// system/block-*.txt), seed/openspec.toml, and seed/scheduling.yaml.
//
//go:embed seed/profiles seed/openspec.toml seed/scheduling.yaml
var Seed embed.FS

// seedRoot is the embed.FS subtree WriteTree walks. Every embedded path begins
// with this prefix; it is stripped so files land directly under root (e.g.
// seed/profiles/zcode/tools.json → <root>/profiles/zcode/tools.json).
const seedRoot = "seed"

// dirPerm is the mode for materialized directories (rwxr-xr-x — no group/other
// write; the tree is read-only-by-convention after seeding).
const dirPerm = 0o755

// filePerm is the mode for materialized files (rw-r--r-- — readable by all,
// writable only by owner; matches the self-gitignoring .ass-guard/ convention).
const filePerm = 0o644

// WriteTree walks the embedded Seed and writes every file under root, stripping
// the seed/ prefix so the runtime tree lands as
// <root>/{profiles/zcode/..., openspec.toml, scheduling.yaml}. When overwrite
// is false, any path that already exists is skipped (non-clobbering — D-04: an
// operator's edits survive restarts). When overwrite is true, existing files
// are replaced. Directories are created as needed (mode 0o755); files are
// written mode 0o644. Returns an error wrapping the first failed write.
func WriteTree(root string, overwrite bool) error {
	err := fs.WalkDir(Seed, seedRoot, func(path string, d fs.DirEntry, walkErr error) error {
		return writeSeedEntry(root, overwrite, path, d, walkErr)
	})
	if err != nil {
		return fmt.Errorf("materialize seed tree under %s: %w", root, err)
	}

	return nil
}

// writeSeedEntry is the per-file WalkDir callback for WriteTree. It skips
// directories, preserves existing files when overwrite is false (D-04), and
// writes each embedded file under root (seed/ prefix stripped).
func writeSeedEntry(root string, overwrite bool, path string, d fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return fmt.Errorf("walk %s: %w", path, walkErr)
	}

	if d.IsDir() {
		return nil
	}

	rel := strings.TrimPrefix(path, seedRoot+"/")
	target := filepath.Join(root, rel)

	if !overwrite {
		_, statErr := os.Stat(target)
		if statErr == nil {
			// Existing file preserved (non-clobbering, D-04).
			return nil
		}
	}

	data, rerr := Seed.ReadFile(path)
	if rerr != nil {
		return fmt.Errorf("read embedded %s: %w", path, rerr)
	}

	mkErr := os.MkdirAll(filepath.Dir(target), dirPerm)
	if mkErr != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), mkErr)
	}

	wErr := os.WriteFile(target, data, filePerm)
	if wErr != nil {
		return fmt.Errorf("write %s: %w", target, wErr)
	}

	return nil
}
