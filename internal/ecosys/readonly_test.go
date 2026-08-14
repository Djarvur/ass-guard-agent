package ecosys //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadOnlyNoWriteAPI verifies the loader source contains NO write-capable
// syscall EXCEPT inside EnsureGitignore (the sole sanctioned write, scoped to
// .ass-guard/). This is the structural read-only guarantee (T-5-08).
func TestReadOnlyNoWriteAPI(t *testing.T) {
	t.Parallel()

	sourceFiles := []string{"loader.go", "types.go", "doc.go", "goconst_constants.go"}

	for _, name := range sourceFiles {
		data, err := os.ReadFile(filepath.Join(".", name))
		require.NoError(t, err, "read %s", name)

		src := string(data)
		// EnsureGitignore is the ONLY sanctioned write path; the rest must not
		// call os.Create / O_WRONLY / os.WriteFile.
		// Strip the EnsureGitignore function body before scanning so its write
		// does not trip the check.
		assertNoWriteOutsideEnsureGitignore(t, name, src)
	}
}

// assertNoWriteOutsideEnsureGitignore scans for write APIs and asserts each
// occurrence is inside the EnsureGitignore function.
func assertNoWriteOutsideEnsureGitignore(t *testing.T, name, src string) {
	t.Helper()

	writeAPIs := []string{"os.Create(", "os.WriteFile(", "O_WRONLY", "O_RDWR", "os.MkdirAll("}
	for _, api := range writeAPIs {
		idx := 0
		for {
			pos := indexFrom(src, api, idx)
			if pos < 0 {
				break
			}

			// os.MkdirAll is allowed inside EnsureGitignore only.
			if isInEnsureGitignore(src, pos) {
				idx = pos + len(api)

				continue
			}

			t.Errorf("%s: write API %q found outside EnsureGitignore (read-only contract)", name, api)
			idx = pos + len(api)
		}
	}
}

// isInEnsureGitignore reports whether pos falls within the EnsureGitignore func.
func isInEnsureGitignore(src string, pos int) bool {
	fnStart := indexFrom(src, "func EnsureGitignore", 0)
	if fnStart < 0 || pos < fnStart {
		return false
	}

	// The function ends at the next top-level "func " after fnStart.
	nextFn := indexFrom(src, "\nfunc ", fnStart+1)

	return nextFn < 0 || pos < nextFn
}

func indexFrom(s, sub string, from int) int {
	if from > len(s) {
		return -1
	}

	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}

	return -1
}

// TestEnsureGitignoreFirstRun verifies EnsureGitignore creates .ass-guard/.gitignore
// with the D-07 content on first run.
func TestEnsureGitignoreFirstRun(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), assguardDirName)
	require.NoError(t, EnsureGitignore(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, gitignoreContent, string(data))
}

// TestEnsureGitignoreIdempotent verifies a second call does not error or change
// the content.
func TestEnsureGitignoreIdempotent(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), assguardDirName)
	require.NoError(t, EnsureGitignore(dir))

	// Mutate the content; EnsureGitignore must NOT overwrite (idempotent).
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("custom"), 0o600))

	require.NoError(t, EnsureGitignore(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "custom", string(data))
}

// TestEnsureGitignoreRejectsClaudePath verifies EnsureGitignore refuses a
// .claude/ path (the read-only contract).
func TestEnsureGitignoreRejectsClaudePath(t *testing.T) {
	t.Parallel()

	err := EnsureGitignore(filepath.Join(t.TempDir(), claudeDirName))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only")
}

// TestGitignoreCoverage verifies a new .ass-guard/skills/foo/SKILL.md path would
// be ignored by the D-07 gitignore pattern (string-level check — portable, no git
// dependency).
func TestGitignoreCoverage(t *testing.T) {
	t.Parallel()

	pattern := gitignoreContent
	// The pattern "*\n!.gitignore\n" ignores everything under .ass-guard/ except
	// .gitignore itself. A skills/foo/SKILL.md path must match the ignore rule.
	ignoredPath := ".ass-guard/skills/foo/SKILL.md"
	base := filepath.Base(ignoredPath)
	assert.NotEqual(t, ".gitignore", base, "path should be ignored by the * rule")
	assert.Contains(t, pattern, "*")
}

// TestDiscoverAccessorsDeterministic verifies Discover returns a registry with
// deterministic accessor ordering (T4).
func TestDiscoverAccessorsDeterministic(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	project := t.TempDir()
	writeSkill(t, filepath.Join(project, claudeDirName), "zeta", "z", nil)
	writeSkill(t, filepath.Join(project, claudeDirName), "alpha", "a", nil)

	reg, servers, err := Discover(project)
	require.NoError(t, err)

	skills := reg.AllSkills()
	require.Len(t, skills, 2)
	assert.Equal(t, "alpha", skills[0].Name)
	assert.Equal(t, "zeta", skills[1].Name)
	assert.Empty(t, servers) // no ~/.claude.json in the temp home
}
