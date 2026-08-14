package ecosys //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureShadowLogger swaps the package shadow-warning logger for one writing
// to a buffer and returns a function restoring the default. Tests using it
// must NOT run parallel (they mutate the package global).
func captureShadowLogger(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	prev := shadowWarnLogger
	shadowWarnLogger = slog.New(slog.NewTextHandler(buf, nil))

	t.Cleanup(func() { shadowWarnLogger = prev })

	return buf
}

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

// TestShadowWarningCrossTree (Test 11, CMD-05) verifies a same-key command
// across the two trees (.claude/ vs .ass-guard/) resolves to .claude/ AND
// emits a warning naming BOTH file paths.
func TestShadowWarningCrossTree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	buf := captureShadowLogger(t)

	tmp := t.TempDir()
	claudeDir := filepath.Join(tmp, claudeDirName)
	assguardDir := filepath.Join(tmp, assguardDirName)

	writeCommand(t, claudeDir, "foo", "claude-version", "claude body")
	writeCommand(t, assguardDir, "foo", "assguard-version", "assguard body")

	reg, err := Load(claudeDir, assguardDir)
	require.NoError(t, err)

	cmd, ok := reg.Commands["foo"]
	require.True(t, ok)
	assert.Equal(t, "claude-version", cmd.Description, ".claude/ must win on conflict")

	warned := buf.String()
	assert.Contains(t, warned, "shadows", "same-key overwrite must warn")
	assert.Contains(t, warned, "command /foo", "warning must name the command key")
	assert.Contains(t, warned, filepath.Join(claudeDir, "commands", "foo.md"), "warning must name the winning path")
	assert.Contains(t, warned, filepath.Join(assguardDir, "commands", "foo.md"), "warning must name the shadowed path")
}

// TestShadowWarningWithinTree (Test 12) verifies the within-tree precedence
// merge (user → project) also warns: project wins and both paths are named.
func TestShadowWarningWithinTree(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	buf := captureShadowLogger(t)

	userCmd := filepath.Join(tmpHome, claudeDirName, "commands", "foo.md")
	writeCommand(t, filepath.Join(tmpHome, claudeDirName), "foo", "user-version", "user body")

	tmpProject := t.TempDir()
	claudeDir := filepath.Join(tmpProject, claudeDirName)
	writeCommand(t, claudeDir, "foo", "project-version", "project body")

	reg, err := Load(claudeDir, filepath.Join(tmpProject, assguardDirName))
	require.NoError(t, err)

	cmd, ok := reg.Commands["foo"]
	require.True(t, ok)
	assert.Equal(t, "project-version", cmd.Description, "project must win over user")

	warned := buf.String()
	assert.Contains(t, warned, "shadows")
	assert.Contains(t, warned, userCmd, "warning must name the shadowed (user) path")
	assert.Contains(t, warned, filepath.Join(claudeDir, "commands", "foo.md"), "warning must name the winning path")
}

// TestShadowNoFalsePositives (Test 13) verifies a shadow-free merge emits
// zero warnings.
func TestShadowNoFalsePositives(t *testing.T) {
	buf := captureShadowLogger(t)

	tmp := t.TempDir()
	claudeDir := filepath.Join(tmp, claudeDirName)
	assguardDir := filepath.Join(tmp, assguardDirName)

	t.Setenv("HOME", t.TempDir())

	writeCommand(t, claudeDir, "alpha", "a", "body a")
	writeCommand(t, assguardDir, "beta", "b", "body b")

	_, err := Load(claudeDir, assguardDir)
	require.NoError(t, err)

	assert.Empty(t, buf.String(), "shadow-free trees must not warn")
}

// TestShadowWarningSkills (Test 14) verifies the same warning mechanism fires
// for same-key Skills shadowing (one helper, two maps).
func TestShadowWarningSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	buf := captureShadowLogger(t)

	tmp := t.TempDir()
	claudeDir := filepath.Join(tmp, claudeDirName)
	assguardDir := filepath.Join(tmp, assguardDirName)

	writeSkill(t, claudeDir, "research", "claude-version", nil)
	writeSkill(t, assguardDir, "research", "assguard-version", nil)

	reg, err := Load(claudeDir, assguardDir)
	require.NoError(t, err)

	sk, ok := reg.Skills["research"]
	require.True(t, ok)
	assert.Equal(t, "claude-version", sk.Description)

	warned := buf.String()
	assert.Contains(t, warned, "shadows", "same-key skill overwrite must warn")
	assert.Contains(t, warned, "skill research", "warning must name the skill")
	assert.Contains(t, warned, filepath.Join(claudeDir, "skills", "research", "SKILL.md"))
	assert.Contains(t, warned, filepath.Join(assguardDir, "skills", "research", "SKILL.md"))
}
