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
	writeSkill(t, root, "research", "Research a topic", []string{toolRead, toolGrep})

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
	toolGrep = "Grep"
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

// installedFixture is the committed installed-plugins fixture tree (12-02):
// its CONTENTS are a `.claude/plugins/` root — an installed_plugins.json (v1
// registry: one live entry + one deliberately disk-absent entry) plus the
// cache/<marketplace>/<plugin>/<version>/ layout with a .claude-plugin
// manifest, one bundled skill, and one bundled command. Later tasks extend the
// same tree with agents/, hooks/hooks.json, and .mcp.json.
const installedFixture = "testdata/plugins-installed"

// copyTree recursively copies the fixture tree at src into dst (test-side
// fixture planting; deterministic offline ground truth).
func copyTree(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}

		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return fmt.Errorf("rel %s: %w", path, rerr)
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return fmt.Errorf("read %s: %w", path, rerr)
		}

		return os.WriteFile(target, data, 0o600)
	})
	require.NoError(t, err)
}

// plantProjectPlugins copies the committed fixture into <tmp>/.claude/plugins
// and returns the project dir (the PROJECT plugin root per the 2026-08-19
// revision).
func plantProjectPlugins(t *testing.T) string {
	t.Helper()

	proj := t.TempDir()
	copyTree(t, installedFixture, filepath.Join(proj, claudeDirName, "plugins"))

	return proj
}

// TestInstalledPluginsDiscoveryE2E (Task 1, Test 1) verifies the installed-cache
// shape end-to-end through the real Load: installed_plugins.json resolves the
// cache installPath, the .claude-plugin/plugin.json manifest is read, and the
// plugin's bundled skill + command land in the Registry with Paths inside the
// cache — from BOTH root kinds (project `.claude/plugins/` here; the user root
// is exercised by the precedence matrix + live probe tests).
func TestInstalledPluginsDiscoveryE2E(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := plantProjectPlugins(t)

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	sk, ok := reg.Skills["fixture-skill"]
	require.True(t, ok, "plugin-bundled skill must be discovered through Load")

	assert.Contains(t, sk.Description, "installed plugin cache")
	assert.Contains(t, sk.Path, filepath.Join("cache", "acme-market", "skill-plugin", "1.0.0"))

	cmd, ok := reg.Commands["fixture-cmd"]
	require.True(t, ok, "plugin-bundled command must be discovered through Load")

	assert.Contains(t, cmd.Description, "installed plugin cache")
	assert.Contains(t, cmd.Path, filepath.Join("cache", "acme-market", "skill-plugin", "1.0.0"))
	assert.Contains(t, cmd.Body, "$ARGUMENTS")

	// The plugin itself carries installed-plugin provenance.
	pl, ok := reg.Plugins["skill-plugin"]
	require.True(t, ok, "installed plugin must be registered with provenance")

	assert.Equal(t, "acme-market", pl.Source, "marketplace source must be recorded")
	assert.Equal(t, "1.0.0", pl.Version, "version must be recorded")
	assert.Equal(t, "user", pl.Scope, "scope must be recorded")
	assert.Contains(t, pl.InstallPath, filepath.Join("skill-plugin", "1.0.0"))

	// The listing (the model-visible surface) includes the plugin skill.
	listing := SkillListing(reg)
	assert.Contains(t, listing, "fixture-skill")
	assert.Contains(t, listing, filepath.Join("cache", "acme-market", "skill-plugin", "1.0.0"))
}

// TestInstalledPluginsUserRoot (Task 1, Test 1b) verifies the SAME end-to-end
// discovery through the USER root (~/.claude/plugins/) — both root kinds load.
func TestInstalledPluginsUserRoot(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	copyTree(t, installedFixture, filepath.Join(tmpHome, claudeDirName, "plugins"))

	proj := t.TempDir()

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	require.Contains(t, reg.Skills, "fixture-skill", "user-root plugin skill must be discovered")
	require.Contains(t, reg.Commands, "fixture-cmd", "user-root plugin command must be discovered")
	require.Contains(t, reg.Plugins, "skill-plugin")

	pl := reg.Plugins["skill-plugin"]
	assert.Equal(t, "acme-market", pl.Source)
	assert.Contains(t, pl.Path, tmpHome, "user-root plugin provenance names the user cache path")
}

// TestInstalledPluginAbsentSkipped (Task 1, Test 2) verifies an entry whose
// installPath points nowhere on disk is skipped with a warning (consumed by the
// test's stderr sink) and the REST of the registry still loads.
func TestInstalledPluginAbsentSkipped(t *testing.T) {
	buf := captureShadowLogger(t)

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := plantProjectPlugins(t)

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	assert.NotContains(t, reg.Plugins, "ghost-plugin", "disk-absent entry must be skipped")
	assert.Contains(t, reg.Plugins, "skill-plugin", "the rest of the registry loads")

	warned := buf.String()
	assert.Contains(t, warned, "ghost-plugin", "skip warning must name the absent plugin")
	assert.Contains(t, warned, "ghost-plugin/9.9.9", "skip warning must name the absent path")
}

// TestInstalledPluginMalformedManifest (Task 1, Test 3) verifies a malformed
// plugin.json yields the same skip-with-warning behavior — never a load error.
func TestInstalledPluginMalformedManifest(t *testing.T) {
	buf := captureShadowLogger(t)

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := t.TempDir()
	plugRoot := filepath.Join(proj, claudeDirName, "plugins")
	broken := filepath.Join(plugRoot, "cache", "mkt", "broken-plugin", "0.1.0")

	require.NoError(t, os.MkdirAll(filepath.Join(broken, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(broken, ".claude-plugin", "plugin.json"), []byte(`{"name": `), 0o600))

	registry := []byte(`[{"name":"broken-plugin@mkt","installPath":"cache/mkt/broken-plugin/0.1.0","scope":"user"}]`)
	require.NoError(t, os.WriteFile(filepath.Join(plugRoot, "installed_plugins.json"), registry, 0o600))

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err, "a malformed manifest must never fail the load")

	assert.NotContains(t, reg.Plugins, "broken-plugin", "malformed-manifest plugin must be skipped")
	assert.Contains(t, buf.String(), "plugin.json", "skip warning must name the malformed manifest path")
}

// TestInstalledPluginOldShapeStillWorks (Task 1 regression) verifies the
// EXISTING simple plugins/<name>/manifest.json shape keeps working alongside
// the installed-cache shape (no regression).
func TestInstalledPluginOldShapeStillWorks(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := t.TempDir()
	writePlugin(t, filepath.Join(proj, claudeDirName), "simple-plugin", []string{"a"}, []string{"b"})

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	pl, ok := reg.Plugins["simple-plugin"]
	require.True(t, ok, "the old simple-plugin shape must keep working")

	assert.Equal(t, []string{"a"}, pl.Skills)
	assert.Equal(t, []string{"b"}, pl.Commands)
}

// TestInstalledPluginAgentsDiscovered (Task 3, Test 1) verifies plugin-bundled
// agents/<name>.md parse into Registry.Agents with provenance, and a project
// `.claude/agents/<name>.md` OVERSHADOWS a same-name plugin agent per the
// chain (shadow warning naming both files).
func TestInstalledPluginAgentsDiscovered(t *testing.T) {
	buf := captureShadowLogger(t)

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := plantProjectPlugins(t)

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	ag, ok := reg.Agents["fixture-agent"]
	require.True(t, ok, "plugin-bundled agent must be discovered")

	assert.Contains(t, ag.Description, "installed plugin cache")
	assert.Equal(t, []string{"Read", "Grep", "Glob"}, ag.Tools)
	assert.Equal(t, "sonnet", ag.Model)
	assert.Contains(t, ag.Prompt, "fixture plugin agent")
	assert.Contains(t, ag.Path, filepath.Join("cache", "acme-market", "skill-plugin", "1.0.0"))

	// A project .claude/agents/ definition overshadows the plugin agent.
	claudeAgents := filepath.Join(proj, claudeDirName, "agents")
	require.NoError(t, os.MkdirAll(claudeAgents, 0o755))

	local := "---\nname: fixture-agent\ndescription: local-version\ntools: Write\n---\nLocal agent prompt.\n"
	require.NoError(t, os.WriteFile(filepath.Join(claudeAgents, "fixture-agent.md"), []byte(local), 0o600))

	buf.Reset()

	reg, err = Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	ag, ok = reg.Agents["fixture-agent"]
	require.True(t, ok)
	assert.Equal(t, "local-version", ag.Description, ".claude/agents/ must overshadow the plugin agent")
	assert.Equal(t, "Local agent prompt.\n", ag.Prompt)

	warned := buf.String()
	assert.Contains(t, warned, "shadows", "agent overwrite must warn")
	assert.Contains(t, warned, filepath.Join(claudeAgents, "fixture-agent.md"), "warning names the winning path")
	assert.Contains(t, warned,
		filepath.Join("cache", "acme-market", "skill-plugin", "1.0.0", "agents", "fixture-agent.md"),
		"warning names the shadowed plugin path")
}

// TestUserAgentsDirFirstClass (Task 3, Test 1b) verifies the USER
// `~/.claude/agents/` dir is a first-class chain entry (and loses to the
// project `.claude/agents/` on collision).
func TestUserAgentsDirFirstClass(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	userAgents := filepath.Join(tmpHome, claudeDirName, "agents")
	require.NoError(t, os.MkdirAll(userAgents, 0o755))

	userDef := "---\nname: user-agent\ndescription: from user dir\n---\nUser agent prompt.\n"
	require.NoError(t, os.WriteFile(filepath.Join(userAgents, "user-agent.md"), []byte(userDef), 0o600))

	proj := t.TempDir()

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	ag, ok := reg.Agents["user-agent"]
	require.True(t, ok, "user .claude/agents/ definitions are first-class")
	assert.Equal(t, "from user dir", ag.Description)
}

// TestPluginMCPJSONMergesLowest (Task 3, Test 3) verifies a plugin-bundled
// .mcp.json parses into the Discover server set and merges BELOW the user
// ~/.claude.json scope: a same-name collision resolves to the non-plugin
// (user) entry.
func TestPluginMCPJSONMergesLowest(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := plantProjectPlugins(t)

	// No user config: the plugin server surfaces alone.
	reg, servers, err := Discover(proj)
	require.NoError(t, err)
	require.Contains(t, reg.Plugins, "skill-plugin")

	fixture := findServer(servers, "fixture-mcp")

	require.NotNil(t, fixture, "plugin .mcp.json server must surface in the Discover set")
	assert.Equal(t, "/bin/echo", fixture.Command)
	assert.Equal(t, []string{"fixture-mcp-ready"}, fixture.Args)

	// User-scope collision: the USER entry wins (plugin MCP is the lowest layer).
	userJSON := `{"mcpServers":{"fixture-mcp":{"command":"/usr/bin/true","args":["user-wins"]}}}`
	require.NoError(t, os.WriteFile(filepath.Join(tmpHome, claudeJSONFile), []byte(userJSON), 0o600))

	_, servers, err = Discover(proj)
	require.NoError(t, err)

	fixture = findServer(servers, "fixture-mcp")

	require.NotNil(t, fixture, "collided server must still surface")
	assert.Equal(t, "/usr/bin/true", fixture.Command, "user-scope config must win over the plugin server")
	assert.Equal(t, []string{"user-wins"}, fixture.Args)
}

// findServer returns a pointer to the named server in servers (nil when
// absent).
func findServer(servers []ServerConfig, name string) *ServerConfig {
	for i := range servers {
		if servers[i].Name == name {
			return &servers[i]
		}
	}

	return nil
}

// TestAgentAndMCPTolerance (Task 3, Test 4) verifies malformed agent
// frontmatter / .mcp.json degrade to skip-with-warning — the rest loads.
func TestAgentAndMCPTolerance(t *testing.T) {
	buf := captureShadowLogger(t)

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := t.TempDir()
	plugRoot := filepath.Join(proj, claudeDirName, "plugins")
	install := filepath.Join(plugRoot, "cache", "mkt", "sick-plugin", "0.1.0")

	require.NoError(t, os.MkdirAll(filepath.Join(install, ".claude-plugin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(install, "agents"), 0o755))

	manifest := `{"name":"sick-plugin"}`
	require.NoError(t, os.WriteFile(filepath.Join(install, ".claude-plugin", "plugin.json"), []byte(manifest), 0o600))

	// Agent with unparseable frontmatter (tab-indented garbage under strict YAML
	// AND no name/description/body salvage) — skipped, not fatal.
	require.NoError(t, os.WriteFile(
		filepath.Join(install, "agents", "broken.md"), []byte("---\nname: [unclosed\n---\n"), 0o600))

	// Malformed .mcp.json — skipped, not fatal.
	require.NoError(t, os.WriteFile(filepath.Join(install, ".mcp.json"), []byte(`{"mcpServers": `), 0o600))

	registry := `[{"name":"sick-plugin@mkt","installPath":"cache/mkt/sick-plugin/0.1.0","scope":"user"}]`
	require.NoError(t, os.WriteFile(filepath.Join(plugRoot, installedPluginsFile), []byte(registry), 0o600))

	reg, servers, err := Discover(proj)
	require.NoError(t, err, "malformed agent frontmatter/.mcp.json must never fail the load")

	assert.NotContains(t, reg.Agents, "broken", "malformed agent must be skipped")
	assert.Empty(t, servers, "malformed .mcp.json must contribute no servers")
	assert.Contains(t, reg.Plugins, "sick-plugin", "the plugin itself still loads")
	assert.Contains(t, buf.String(), "agents", "warning names the agent artifact")
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
