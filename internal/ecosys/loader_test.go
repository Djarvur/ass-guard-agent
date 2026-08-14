package ecosys //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSkill writes a fixture skill under root/skills/<name>/SKILL.md.
func writeSkill(t *testing.T, root, name, desc string, allowed []string) {
	t.Helper()

	body := "---\nname: " + name + "\ndescription: " + desc + "\n"
	if len(allowed) > 0 {
		body += "allowed-tools:\n"

		var bodySb20 strings.Builder
		for _, a := range allowed {
			bodySb20.WriteString("  - " + a + "\n")
		}

		body += bodySb20.String()
	}

	body += "---\nskill body\n"

	dir := filepath.Join(root, "skills", name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o600))
}

// writeCommand writes a fixture command under root/commands/<name>.md.
func writeCommand(t *testing.T, root, name, desc, bodyText string) {
	t.Helper()

	body := "---\ndescription: " + desc + "\n---\n" + bodyText + "\n"
	dir := filepath.Join(root, "commands")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o600))
}

// writePlugin writes a fixture plugin manifest under root/plugins/<name>/manifest.json.
func writePlugin(t *testing.T, root, name string, skills, commands []string) {
	t.Helper()

	dir := filepath.Join(root, "plugins", name)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	manifest := map[string]any{"name": name, "skills": skills, "commands": commands}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o600))
}

// TestDiscoverSkill verifies a skill with frontmatter is discovered.
func TestDiscoverSkill(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeSkill(t, root, "research", "Research a topic", []string{"Read", "Grep"})

	reg, err := loadTree(root)
	require.NoError(t, err)

	sk, ok := reg.Skills["research"]
	require.True(t, ok, "skill not discovered")
	assert.Equal(t, "Research a topic", sk.Description)
	assert.Equal(t, []string{"Read", "Grep"}, sk.AllowedTools)
	assert.Contains(t, sk.Path, "SKILL.md")
}

// TestDiscoverSkillMissingSkipped verifies a skill dir without SKILL.md is
// silently skipped (not an error).
func TestDiscoverSkillMissingSkipped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// Create a skill dir with no SKILL.md.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills", "broken"), 0o755))

	writeSkill(t, root, "good", "good skill", nil)

	reg, err := loadTree(root)
	require.NoError(t, err)

	_, ok := reg.Skills["broken"]
	assert.False(t, ok, "broken skill should be skipped")
	require.Contains(t, reg.Skills, "good")
}

// TestDiscoverCommand verifies a command is discovered by filename stem.
func TestDiscoverCommand(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCommand(t, root, "summarize", "Summarize text", "Summarize: $ARGUMENTS")

	reg, err := loadTree(root)
	require.NoError(t, err)

	cmd, ok := reg.Commands["summarize"]
	require.True(t, ok, "command not discovered")
	assert.Equal(t, "Summarize text", cmd.Description)
	assert.Contains(t, cmd.Body, "$ARGUMENTS")
	assert.Equal(t, "summarize", cmd.Name)
}

// TestDiscoverPlugin verifies a plugin manifest is discovered.
func TestDiscoverPlugin(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writePlugin(t, root, "myplugin", []string{"a", "b"}, []string{"c"})

	reg, err := loadTree(root)
	require.NoError(t, err)

	pl, ok := reg.Plugins["myplugin"]
	require.True(t, ok, "plugin not discovered")
	assert.Equal(t, []string{"a", "b"}, pl.Skills)
	assert.Equal(t, []string{"c"}, pl.Commands)
	assert.Contains(t, pl.Path, "manifest.json")
}

// TestLoadEmptyDir verifies a non-existent/empty root returns an empty registry,
// nil error (the common case).
func TestLoadEmptyDir(t *testing.T) {
	t.Parallel()

	reg, err := loadTree(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, reg.Skills)
	assert.Empty(t, reg.Commands)
	assert.Empty(t, reg.Plugins)

	// Non-existent dir is also empty + nil error.
	reg2, err := loadTree(filepath.Join(t.TempDir(), "nope"))
	require.NoError(t, err)
	assert.Empty(t, reg2.Skills)
}

// TestLoadMissingClaudeDir verifies Load with no .claude/.ass-guard returns empty.
// HOME is redirected so the real ~/.claude/ does not leak in.
func TestLoadMissingClaudeDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	reg, err := Load(filepath.Join(t.TempDir(), claudeDirName), filepath.Join(t.TempDir(), assguardDirName))
	require.NoError(t, err)
	assert.Empty(t, reg.Skills)
}

// TestRegistryAccessors verifies AllSkills/AllCommands/AllPlugins return sorted
// slices (deterministic ordering — T4).
func TestRegistryAccessors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeSkill(t, root, "zebra", "z", nil)
	writeSkill(t, root, "alpha", "a", nil)
	writeCommand(t, root, "beta", "b", "body")
	writeCommand(t, root, "alpha", "a", "body")
	writePlugin(t, root, "gamma", nil, nil)
	writePlugin(t, root, "delta", nil, nil)

	reg, err := loadTree(root)
	require.NoError(t, err)

	skills := reg.AllSkills()
	require.Len(t, skills, 2)
	assert.Equal(t, "alpha", skills[0].Name)
	assert.Equal(t, "zebra", skills[1].Name)

	cmds := reg.AllCommands()
	require.Len(t, cmds, 2)
	assert.Equal(t, "alpha", cmds[0].Name)
	assert.Equal(t, "beta", cmds[1].Name)

	plugins := reg.AllPlugins()
	require.Len(t, plugins, 2)
	assert.Equal(t, "delta", plugins[0].Name)
	assert.Equal(t, "gamma", plugins[1].Name)
}

// TestLoadUserMCPConfigMissing asserts a missing ~/.claude.json yields an empty
// map (the home dir is redirected to a temp dir so the real file is not read).
func TestLoadUserMCPConfigMissing(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg, err := LoadUserMCPConfig()
	require.NoError(t, err)
	assert.Empty(t, cfg)
}
