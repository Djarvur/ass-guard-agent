// Package firstrun implements the zero-config first-run detection and seeding
// flow (D-04). When the registry entrypoint (ass-guard acp serve) starts in a
// directory with no .ass-guard/, Ensure creates it with a self-gitignoring
// .gitignore (D-07) and writes the embedded default tree (profiles/zcode,
// openspec.toml, scheduling.yaml) via internal/defaults. The flow is
// non-clobbering and idempotent: an existing .ass-guard/ is left untouched.
package firstrun

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Djarvur/ass-guard-agent/internal/defaults"
)

// selfGitignoreContent is the .ass-guard/.gitignore body (D-07): ignore
// everything except .gitignore itself. This is byte-identical to
// internal/session/transcript.go's selfGitignoreContent — the transcript
// writer and first-run co-own .ass-guard/ idempotently and MUST agree on the
// body. A drift here would change .ass-guard/'s gitignore semantics.
const selfGitignoreContent = "*\n!.gitignore\n"

// dirPerm is the mode for .ass-guard/ (rwxr-xr-x — group/other can traverse).
const dirPerm = 0o755

// filePerm is the mode for the written .gitignore (rw-r--r--).
const filePerm = 0o644

// assGuardDir is the directory name under the working directory.
const assGuardDir = ".ass-guard"

// Ensure resolves <workDir>/.ass-guard. If that directory already exists, it
// returns (false, nil) without mutating anything — an operator's pre-existing
// .ass-guard/ (including a partial/empty init) is left untouched (D-04: never
// clobber; do not half-seed an existing directory). If the directory is missing,
// Ensure creates it (mode 0o755), writes the D-07 self-gitignoring .gitignore
// (only when it does not already exist), lays down the embedded default tree
// via defaults.WriteTree(dir, false) (non-clobbering), and returns (true, nil).
//
// All writes are confined to the fixed .ass-guard/ relative path under workDir;
// no path traversal is possible (the embedded filenames contain no "..").
func Ensure(workDir string) (seeded bool, err error) {
	dir := filepath.Join(workDir, assGuardDir)

	if _, statErr := os.Stat(dir); statErr == nil {
		// Already initialized (including a partial/empty dir). Never clobber.
		return false, nil
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("stat %s: %w", dir, statErr)
	}

	if mkErr := os.MkdirAll(dir, dirPerm); mkErr != nil {
		return false, fmt.Errorf("mkdir %s: %w", dir, mkErr)
	}

	giPath := filepath.Join(dir, ".gitignore")
	if _, statErr := os.Stat(giPath); os.IsNotExist(statErr) {
		if wErr := os.WriteFile(giPath, []byte(selfGitignoreContent), filePerm); wErr != nil {
			return false, fmt.Errorf("write %s: %w", giPath, wErr)
		}
	}

	if wErr := defaults.WriteTree(dir, false); wErr != nil {
		return false, fmt.Errorf("seed defaults: %w", wErr)
	}

	return true, nil
}
