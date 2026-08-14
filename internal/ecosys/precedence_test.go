package ecosys //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrecedenceClaudeWins verifies that on a name collision, the `.claude/`
// version wins over the `.ass-guard/` version (D-06). HOME is redirected so the
// real ~/.claude/ does not leak in.
func TestPrecedenceClaudeWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tmp := t.TempDir()
	claudeDir := filepath.Join(tmp, claudeDirName)
	assguardDir := filepath.Join(tmp, assguardDirName)

	writeSkill(t, claudeDir, "research", "claude-version", nil)
	writeSkill(t, assguardDir, "research", "assguard-version", nil)

	reg, err := Load(claudeDir, assguardDir)
	require.NoError(t, err)

	sk, ok := reg.Skills["research"]
	require.True(t, ok)
	assert.Equal(t, "claude-version", sk.Description, ".claude/ must win on conflict")
}

// TestPrecedenceProjectWinsWithinTree verifies that within a tree, project-scope
// wins over user-scope (Claude Code direction). The user home is redirected to a
// temp dir.
func TestPrecedenceProjectWinsWithinTree(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// User-scope command (~/.claude/commands/summarize.md).
	writeCommand(t, filepath.Join(tmpHome, claudeDirName), "summarize", "user-version", "user body")

	// Project-scope command (./.claude/commands/summarize.md).
	tmpProject := t.TempDir()
	claudeDir := filepath.Join(tmpProject, claudeDirName)
	writeCommand(t, claudeDir, "summarize", "project-version", "project body")

	reg, err := Load(claudeDir, filepath.Join(tmpProject, assguardDirName))
	require.NoError(t, err)

	cmd, ok := reg.Commands["summarize"]
	require.True(t, ok)
	assert.Equal(t, "project-version", cmd.Description, "project-scope must win over user-scope")
}

// TestAssguardAdditionsSurface verifies a skill that exists ONLY in `.ass-guard/`
// is in the registry (ass-guard's additions surface; they just lose on conflict).
func TestAssguardAdditionsSurface(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tmp := t.TempDir()
	claudeDir := filepath.Join(tmp, claudeDirName)
	assguardDir := filepath.Join(tmp, assguardDirName)

	writeSkill(t, assguardDir, "ag-extra", "ass-guard-only", nil)

	reg, err := Load(claudeDir, assguardDir)
	require.NoError(t, err)

	sk, ok := reg.Skills["ag-extra"]
	require.True(t, ok, "ass-guard-only skill must surface")
	assert.Equal(t, "ass-guard-only", sk.Description)
}

// TestNoClobberInvariant verifies Load never mutates the source files: after
// Load, both the .claude/ and .ass-guard/ skill files are unchanged.
func TestNoClobberInvariant(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tmp := t.TempDir()
	claudeDir := filepath.Join(tmp, claudeDirName)
	assguardDir := filepath.Join(tmp, assguardDirName)

	writeSkill(t, claudeDir, "research", "claude-version", nil)
	writeSkill(t, assguardDir, "research", "assguard-version", nil)

	claudeBefore := readSkillFile(t, claudeDir, "research")
	assguardBefore := readSkillFile(t, assguardDir, "research")

	_, err := Load(claudeDir, assguardDir)
	require.NoError(t, err)

	assert.Equal(t, claudeBefore, readSkillFile(t, claudeDir, "research"), ".claude/ file must be unchanged")
	assert.Equal(t, assguardBefore, readSkillFile(t, assguardDir, "research"), ".ass-guard/ file must be unchanged")
}

func readSkillFile(t *testing.T, root, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, "skills", name, "SKILL.md"))
	require.NoError(t, err)

	return string(data)
}

// TestMergeRegistriesOverlayWins is a unit test for the merge primitive.
func TestMergeRegistriesOverlayWins(t *testing.T) {
	t.Parallel()

	base := newRegistry()
	base.Skills["a"] = Skill{Name: "a", Description: "base"}
	base.Skills["b"] = Skill{Name: "b", Description: "base"}

	overlay := newRegistry()
	overlay.Skills["b"] = Skill{Name: "b", Description: "overlay"}
	overlay.Skills["c"] = Skill{Name: "c", Description: "overlay"}

	out := mergeRegistries(base, overlay)

	assert.Equal(t, "base", out.Skills["a"].Description)
	assert.Equal(t, "overlay", out.Skills["b"].Description, "overlay must win on conflict")
	assert.Equal(t, "overlay", out.Skills["c"].Description)
}
