package ecosys //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

// opsxRealFixture is the committed REAL-fixture project scope generated from
// actual `openspec init --tools claude` output (CMD-01). See the fixture README
// for the regeneration command.
const opsxRealFixture = "testdata/opsx-real/.claude"

// TestDiscoverNamespaced (Test 1, CMD-01) verifies the REAL opsx fixture tree is
// discovered with colon-joined keys: commands/opsx/explore.md → "opsx:explore".
func TestDiscoverNamespaced(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	reg, err := Load(opsxRealFixture, "")
	require.NoError(t, err)

	for _, key := range []string{"opsx:explore", "opsx:propose", "opsx:apply", "opsx:archive", "opsx:sync"} {
		cmd, ok := reg.Commands[key]
		require.True(t, ok, "namespaced command %q not discovered (CMD-01)", key)
		assert.Contains(t, cmd.Path, "opsx", "Path must point at the real file")
		assert.NotEmpty(t, cmd.Body, "command %q must carry its body", key)
	}
}

// TestDiscoverCoexist (Test 2) verifies both layouts coexist in one registry:
// top-level commands/flat.md keeps its bare stem while commands/opsx/explore.md
// joins under "opsx:explore" — no collision, no data loss.
func TestDiscoverCoexist(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCommand(t, root, "flat", "flat command", "flat body")
	writeNamespacedCommand(t, root, "opsx", "explore", "explore command", "explore body")

	reg, err := loadTree(root)
	require.NoError(t, err)

	require.Contains(t, reg.Commands, "flat", "bare-stem key must survive")
	require.Contains(t, reg.Commands, "opsx:explore", "colon-joined key must coexist")
	assert.Equal(t, "flat body\n", reg.Commands["flat"].Body)
	assert.Equal(t, "explore body\n", reg.Commands["opsx:explore"].Body)
}

// writeNamespacedCommand writes a fixture command under
// root/commands/<ns>/<name>.md (the opsx layout).
func writeNamespacedCommand(t *testing.T, root, ns, name, desc, bodyText string) {
	t.Helper()

	body := "---\ndescription: " + desc + "\n---\n" + bodyText + "\n"
	dir := filepath.Join(root, "commands", ns)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o600))
}

// TestNameRegex (Test 3) verifies zcode's command-name rule: keys must match
// ^[a-z0-9][a-z0-9_:-]{0,63}$ — violators are dropped silently.
func TestNameRegex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCommand(t, root, "Bad_Name", "uppercase dropped", "body")
	writeCommand(t, root, "has space", "space dropped", "body")
	writeCommand(t, root, strings.Repeat("a", 65), "65-char dropped", "body")
	writeCommand(t, root, strings.Repeat("b", 64), "64-char survives", "body")
	writeCommand(t, root, "good-cmd", "valid", "body")

	reg, err := loadTree(root)
	require.NoError(t, err)

	assert.NotContains(t, reg.Commands, "Bad_Name", "uppercase name must be dropped")
	assert.NotContains(t, reg.Commands, "has space", "space in name must be dropped")
	assert.NotContains(t, reg.Commands, strings.Repeat("a", 65), "65-char name must be dropped")
	assert.Contains(t, reg.Commands, strings.Repeat("b", 64), "64-char name must survive")
	assert.Contains(t, reg.Commands, "good-cmd")
}

// TestDescriptionFallback (Test 4) verifies zcode's description rules: empty
// description + non-empty body → first non-empty body line; neither → dropped.
func TestDescriptionFallback(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRawCommand(t, root, "fallback", "---\ndescription:\n---\n\nFirst body line.\nSecond line.\n")
	writeRawCommand(t, root, "empty", "---\ndescription:\n---\n\n")

	reg, err := loadTree(root)
	require.NoError(t, err)

	cmd, ok := reg.Commands["fallback"]
	require.True(t, ok, "command with body but no description must load")
	assert.Equal(t, "First body line.", cmd.Description, "description must fall back to first non-empty body line")

	assert.NotContains(t, reg.Commands, "empty", "command with neither description nor body must be dropped")
}

// writeRawCommand writes raw fixture bytes under root/commands/<name>.md.
func writeRawCommand(t *testing.T, root, name, content string) {
	t.Helper()

	dir := filepath.Join(root, "commands")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o600))
}

// TestOpsxFixtureMatchesRealInit (Test 5, gated) re-runs `openspec init
// --tools claude --force` and asserts the committed fixture command set equals
// the generated set (file names + frontmatter name/description) — the fixture
// cannot silently rot across openspec upgrades. Skips unless
// ASSGUARD_OPENSPEC_BIN=1 AND the binary is on PATH.
func TestOpsxFixtureMatchesRealInit(t *testing.T) {
	t.Parallel()

	if os.Getenv("ASSGUARD_OPENSPEC_BIN") != "1" {
		t.Skip("ASSGUARD_OPENSPEC_BIN not set — fixture-provenance check skipped")
	}

	if _, err := exec.LookPath("openspec"); err != nil { //nolint:noinlineerr // skip-path
		t.Skip("openspec binary not on PATH — fixture-provenance check skipped")
	}

	work := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "openspec", "init", "--tools", "claude", "--force")
	cmd.Dir = work
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "openspec init failed: %s", string(out))

	generated, err := commandFrontmatterSet(filepath.Join(work, ".claude", "commands"))
	require.NoError(t, err)

	committed, err := commandFrontmatterSet(filepath.Join(opsxRealFixture, "commands"))
	require.NoError(t, err)

	assert.Equal(t, generated, committed,
		"committed fixture drifted from real openspec init output — regenerate per the README")
}

// TestNamespacedKeyShape (Test 6) verifies the colon-join: commands/opsx/explore.md
// registers under key "opsx:explore" with Name, Path, and Body populated.
func TestNamespacedKeyShape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeNamespacedCommand(t, root, "opsx", "explore", "explore command", "explore body")

	reg, err := loadTree(root)
	require.NoError(t, err)

	cmd, ok := reg.Commands["opsx:explore"]
	require.True(t, ok, "key must be the colon-joined opsx:explore")

	assert.Equal(t, "opsx:explore", cmd.Name)

	suffix := filepath.Join("commands", "opsx", "explore.md")
	assert.True(t, strings.HasSuffix(cmd.Path, suffix), "Path %q must point at the real file", cmd.Path)

	assert.Equal(t, "explore body\n", cmd.Body)
}

// TestOneLevelOnly (Test 7) verifies zcode joins exactly ONE subdirectory
// level: commands/a/b/deep.md (two levels) is not discovered.
func TestOneLevelOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deepDir := filepath.Join(root, "commands", "a", "b")
	require.NoError(t, os.MkdirAll(deepDir, 0o755))

	deepFile := filepath.Join(deepDir, "deep.md")
	require.NoError(t, os.WriteFile(deepFile, []byte("---\ndescription: d\n---\nbody\n"), 0o600))

	reg, err := loadTree(root)
	require.NoError(t, err)

	assert.Empty(t, reg.Commands, "two-level commands must not be discovered (no flattening)")
}

// TestFrontmatterExtension (Test 8, D-09) verifies argument-hint /
// allowed-tools / model are PARSED (not acted on): comma-scalar and
// YAML-flow-list forms both land in Command.AllowedTools.
func TestFrontmatterExtension(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRawCommand(t, root, "comma",
		"---\nargument-hint: name\nallowed-tools: Bash, Read\nmodel: opus\ndescription: comma form\n---\nbody\n")
	writeRawCommand(t, root, "flowlist",
		"---\nallowed-tools: [Bash, Read]\ndescription: flow form\n---\nbody\n")

	reg, err := loadTree(root)
	require.NoError(t, err)

	wantTools := []string{toolBash, toolRead}

	comma, ok := reg.Commands["comma"]
	require.True(t, ok, "comma-scalar frontmatter command must load")

	assert.Equal(t, "name", comma.ArgumentHint)
	assert.Equal(t, wantTools, comma.AllowedTools)
	assert.Equal(t, "opus", comma.Model)

	flow, ok := reg.Commands["flowlist"]
	require.True(t, ok, "flow-list frontmatter command must load")
	assert.Equal(t, wantTools, flow.AllowedTools, "flow-list form must parse to the same slice")
}

// Fixture tool names shared across frontmatter tests (goconst).
const (
	toolBash = "Bash"
	toolRead = "Read"
)

// TestFlatFallback (Test 9) verifies zcode's flat frontmatter semantics: a
// file with tab-indented (yaml-invalid) frontmatter still loads via the flat
// single-line view, and a multi-line allowed-tools list is DROPPED (zcode
// parity — accepting what zcode drops is a mimicry divergence, PITFALLS 6).
func TestFlatFallback(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRawCommand(t, root, "tabbed",
		"---\ndescription: x\nallowed-tools:\n\t- Bash\n\t- Read\n---\nbody\n")
	writeRawCommand(t, root, "blocklist",
		"---\ndescription: block list\nallowed-tools:\n  - Bash\n  - Read\n---\nbody\n")

	reg, err := loadTree(root)
	require.NoError(t, err)

	tabbed, ok := reg.Commands["tabbed"]
	require.True(t, ok, "tab-indented frontmatter must still load via the flat fallback")
	assert.Equal(t, "x", tabbed.Description)
	assert.Empty(t, tabbed.AllowedTools, "multi-line allowed-tools value must be dropped (flat view)")

	block, ok := reg.Commands["blocklist"]
	require.True(t, ok, "valid-yaml block list must still load")
	assert.Empty(t, block.AllowedTools, "block-style list has no single-line form — zcode drops it")
}

// TestUnknownKeysIgnored (Test 10) verifies unknown frontmatter keys (category,
// tags flow array — the real opsx v1.5.0 shape) load cleanly.
func TestUnknownKeysIgnored(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRawCommand(t, root, "opsxlike",
		"---\nname: \"OPSX: Explore\"\ndescription: \"Enter explore mode\"\n"+
			"category: Workflow\ntags: [workflow, explore, experimental, thinking]\n---\nbody\n")

	reg, err := loadTree(root)
	require.NoError(t, err)

	cmd, ok := reg.Commands["opsxlike"]
	require.True(t, ok, "command with unknown frontmatter keys must load cleanly")
	assert.Equal(t, "Enter explore mode", cmd.Description)
}

// commandFrontmatterSet maps command file names → {name, description} parsed
// from the file's YAML frontmatter (fixture-provenance comparison).
func commandFrontmatterSet(commandsDir string) (map[string][2]string, error) {
	entries, err := os.ReadDir(filepath.Join(commandsDir, "opsx"))
	if err != nil {
		return nil, fmt.Errorf("read opsx commands dir: %w", err)
	}

	out := map[string][2]string{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		out[e.Name()], err = commandNameDescription(filepath.Join(commandsDir, "opsx", e.Name()))
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// commandNameDescription reads one command file and returns its frontmatter
// name/description pair (fixture-provenance comparison).
func commandNameDescription(path string) ([2]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [2]string{}, fmt.Errorf("read %s: %w", path, err)
	}

	fm, _ := splitFrontmatter(string(data))

	var parsed struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}

	if err := yaml.Unmarshal([]byte(fm), &parsed); err != nil { //nolint:noinlineerr // error wrapped with path below
		return [2]string{}, fmt.Errorf("parse %s frontmatter: %w", path, err)
	}

	return [2]string{parsed.Name, parsed.Description}, nil
}
