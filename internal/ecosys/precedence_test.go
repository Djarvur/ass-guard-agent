package ecosys //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

// writeInstalledSkillPlugin plants one installed plugin carrying ONE skill
// into a plugins root (12-02 precedence fixtures): cache/<marketplace>/<plugin>/<version>/
// with manifest + skills/<name>/SKILL.md, and its registry entry APPENDED to
// the root's installed_plugins.json (repeat calls accumulate — one registry,
// many plugins).
func writeInstalledSkillPlugin(t *testing.T, pluginsRoot, marketplace, plugin, version, scope, skill, desc string) {
	t.Helper()

	rel := filepath.Join("cache", marketplace, plugin, version)
	install := filepath.Join(pluginsRoot, rel)

	require.NoError(t, os.MkdirAll(filepath.Join(install, ".claude-plugin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(install, "skills", skill), 0o755))

	manifest := `{"name":"` + plugin + `","description":"fixture"}`
	require.NoError(t, os.WriteFile(filepath.Join(install, ".claude-plugin", "plugin.json"), []byte(manifest), 0o600))

	skillMD := "---\nname: " + skill + "\ndescription: " + desc + "\n---\nbody\n"
	require.NoError(t, os.WriteFile(filepath.Join(install, "skills", skill, "SKILL.md"), []byte(skillMD), 0o600))

	// Append the entry to the v1 registry (read-modify-write).
	entry := map[string]string{
		"name": plugin + "@" + marketplace, "installPath": rel, "scope": scope, "version": version,
	}

	var entries []map[string]string
	if prev, rerr := os.ReadFile(filepath.Join(pluginsRoot, installedPluginsFile)); rerr == nil {
		_ = json.Unmarshal(prev, &entries)
	}

	entries = append(entries, entry)

	raw, merr := json.Marshal(entries)
	require.NoError(t, merr)
	require.NoError(t, os.WriteFile(filepath.Join(pluginsRoot, installedPluginsFile), raw, 0o600))
}

// skillPathOf returns the on-disk SKILL.md path the plugin fixture wrote.
func installedSkillPath(t *testing.T, pluginsRoot, marketplace, plugin, version, skill string) string {
	t.Helper()

	return filepath.Join(pluginsRoot, "cache", marketplace, plugin, version, "skills", skill, "SKILL.md")
}

// TestPrecedenceMatrixPlugins (12-02 Task 2, Test 1) pins the five-tier matrix
// with overlapping fixture layers: the existing chain (D-06 unchanged —
// project-claude > {project-assguard, user-claude} > user-assguard) keeps
// priority over BOTH plugin roots; the project plugin root overlays the user
// plugin root; plugin-only keys resolve to the plugin entry; user+project
// `.claude/skills/`+`commands/` stay first-class ABOVE the plugin tiers; every
// same-key overwrite emits a shadow warning naming both files.
func TestPrecedenceMatrixPlugins(t *testing.T) {
	buf := captureShadowLogger(t)

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	userClaude := filepath.Join(tmpHome, claudeDirName)
	userAssguard := filepath.Join(tmpHome, assguardDirName)
	userPlugins := filepath.Join(userClaude, pluginsDirName)

	proj := t.TempDir()
	projClaude := filepath.Join(proj, claudeDirName)
	projAssguard := filepath.Join(proj, assguardDirName)
	projPlugins := filepath.Join(projClaude, pluginsDirName)

	// "shared" exists in EVERY tier — the existing chain must win (D-06:
	// project-claude is the top of the existing chain).
	writeSkill(t, userClaude, "shared", "claude-user", nil)
	writeSkill(t, projClaude, "shared", "claude-project", nil)
	writeSkill(t, userAssguard, "shared", "assguard-user", nil)
	writeSkill(t, projAssguard, "shared", "assguard-project", nil)
	writeInstalledSkillPlugin(t, userPlugins, "mkt-u", "plug-u", "1.0.0", "user", "shared", "plugin-user")
	writeInstalledSkillPlugin(t, projPlugins, "mkt-p", "plug-p", "1.0.0", "project", "shared", "plugin-project")

	// "plugin-only" exists ONLY in the user plugin root → resolves to it.
	writeInstalledSkillPlugin(t, userPlugins, "mkt-u", "plug-only", "1.0.0", "user", "plugin-only", "only-user")

	// "plugin-both" exists in BOTH plugin roots → project plugin root wins.
	writeInstalledSkillPlugin(t, userPlugins, "mkt-u", "plug-b", "1.0.0", "user", "plugin-both", "plugin-user")
	writeInstalledSkillPlugin(t, projPlugins, "mkt-p", "plug-b2", "1.0.0", "project", "plugin-both", "plugin-project")

	// First-class `.claude/commands/` chain positions inside the same matrix:
	// project `.claude/commands/` beats user `.claude/commands/` AND both
	// plugin roots (the plugin roots carry commands via their own bundles —
	// the same plugin fixture mechanism, keyed by command below).
	writeCommand(t, userClaude, "shared-cmd", "user-version", "user body")
	writeCommand(t, projClaude, "shared-cmd", "project-version", "project body")

	reg, err := Load(projClaude, projAssguard)
	require.NoError(t, err)

	// Existing chain keeps priority over every plugin tier.
	shared, ok := reg.Skills["shared"]
	require.True(t, ok)
	assert.Equal(t, "claude-project", shared.Description,
		"the existing chain (project .claude/) must beat every plugin root")

	// Plugin-only key resolves to the plugin entry with cache-internal Path.
	only, ok := reg.Skills["plugin-only"]
	require.True(t, ok, "plugin-only key must resolve to the plugin entry")
	assert.Equal(t, "only-user", only.Description)
	assert.Contains(t, only.Path, filepath.Join("cache", "mkt-u"))

	// Both plugin roots → project plugins wins.
	both, ok := reg.Skills["plugin-both"]
	require.True(t, ok)
	assert.Equal(t, "plugin-project", both.Description,
		"project .claude/plugins/ must overlay user ~/.claude/plugins/")

	// First-class .claude commands: project wins over user (and plugins below).
	cmd, ok := reg.Commands["shared-cmd"]
	require.True(t, ok)
	assert.Equal(t, "project-version", cmd.Description)

	// Shadow warnings: the plugin-root overwrite pair names BOTH cache files
	// in exactly ONE line; the plugin-vs-.claude overwrite names the winning
	// .claude file and the shadowed plugin cache file.
	warnings := buf.String()

	projBothPath := installedSkillPath(t, projPlugins, "mkt-p", "plug-b2", "1.0.0", "plugin-both")
	userBothPath := installedSkillPath(t, userPlugins, "mkt-u", "plug-b", "1.0.0", "plugin-both")

	pairLines := 0
	for line := range strings.SplitSeq(warnings, "\n") {
		if strings.Contains(line, projBothPath) && strings.Contains(line, userBothPath) {
			pairLines++
		}
	}

	assert.Equal(t, 1, pairLines,
		"the both-roots plugin overwrite must warn EXACTLY once naming both files")

	assert.Contains(t, warnings, filepath.Join(projClaude, "skills", "shared", "SKILL.md"),
		"the winning .claude path must be named")
	assert.Contains(t, warnings, filepath.Join(userPlugins, "cache", "mkt-u", "plug-u", "1.0.0", "skills", "shared", "SKILL.md"),
		"the shadowed user-plugin path must be named")
}

// TestLiveInstalledPluginsProbe (12-02 Task 2, Test 2 — LIVE, presence-gated)
// probes the operator's REAL `~/.claude/plugins/` read-only when it exists:
// the live registry (a v2 file on this machine) must parse and its user-scope
// installs must surface with cache-internal paths. Skips cleanly (t.Skip,
// never fails) on a machine without the roots. NEVER modifies the live roots.
func TestLiveInstalledPluginsProbe(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	root := filepath.Join(home, claudeDirName, pluginsDirName)

	if _, rerr := os.Stat(filepath.Join(root, installedPluginsFile)); rerr != nil {
		t.Skip("no live ~/.claude/plugins/installed_plugins.json — live probe skipped (presence-gated)")
	}

	cwd, _ := os.Getwd()

	reg := newRegistry()
	discoverInstalledPlugins(root, cwd, reg)

	require.NotEmpty(t, reg.Plugins,
		"live root exists with a registry — at least the user-scope installs must parse")

	for _, p := range reg.AllPlugins() {
		t.Logf("live installed plugin: %s@%s version=%s scope=%s skills=%v commands=%v install=%s",
			p.Name, p.Source, p.Version, p.Scope, p.Skills, p.Commands, p.InstallPath)

		for _, s := range p.Skills {
			sk, ok := reg.Skills[s]
			require.True(t, ok, "plugin %s lists skill %s — it must be in the registry", p.Name, s)
			assert.Contains(t, sk.Path, filepath.Join("cache"),
				"plugin-bundled skill path must be inside the cache layout")
		}
	}
}

// TestDocumentedPrecedenceChain (12-02 Task 2, Test 4) pins the package doc:
// the full chain including both plugin roots, their order, the DROPPED zcode
// root (with the operator rationale), and the first-class `.claude/`
// skills/commands entries.
func TestDocumentedPrecedenceChain(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("doc.go")
	require.NoError(t, err)

	doc := string(data)
	for _, phrase := range []string{
		"project `.claude/plugins/`",
		"`~/.claude/plugins/`",
		"~/.zcode/cli/plugins/",
		"first-class",
		"lowest",
	} {
		assert.Contains(t, doc, phrase, "doc.go must document: %s", phrase)
	}
}
