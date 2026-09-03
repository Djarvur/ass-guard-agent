package ecosys

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// gitignoreContent is the Phase-2 D-07 self-gitignore pattern. EnsureGitignore
// writes it to `.ass-guard/.gitignore` on first run so the new skills/commands/
// plugins subdirs are excluded from git.
const gitignoreContent = "*\n!.gitignore\n"

// Sentinel errors for EnsureGitignore validation (idiomatic package-level
// errors; no err113 dynamic-error concern).
var (
	errEmptyAssguardDir = errors.New("ecosys: EnsureGitignore requires a non-empty assguard dir")
	errClaudeReadonly   = errors.New("ecosys: EnsureGitignore refuses a .claude/ path (read-only contract)")
	errCommandDropped   = errors.New("ecosys: command dropped (neither description nor body)")
	errAgentNoIdentity  = errors.New("ecosys: agent definition dropped (no name or description)")
	errToolsFieldShape  = errors.New("ecosys: tools field is not a list or scalar")
)

// claudeDirName is the Claude-Code config directory name (read-only).
const claudeDirName = ".claude"

// assguardDirName is ass-guard's own directory name (own additions).
const assguardDirName = ".ass-guard"

// Load discovers skills/commands/plugins from the project `.claude/` (claudeDir)
// and `.ass-guard/` (assguardDir) trees, merged with the user-scope equivalents
// under the home dir, applying the D-06 two-phase precedence:
//
//  1. Within each tree: project overlays user (project wins).
//  2. Across trees: `.claude/` overlays `.ass-guard/` (`.claude/` wins).
//
// 12-02 adds the installed-plugin roots as the TWO LOWEST tiers (operator
// 2026-08-19): the existing chain keeps priority and the PROJECT
// `<project>/.claude/plugins/` root overlays the USER `~/.claude/plugins/`
// root (project over user, mirroring the `.claude/` ordering). The
// `~/.zcode/cli/plugins/` root is NOT probed (dropped — plugins are installed
// FOR Claude Code and consumed as native; the listing merge is dynamic content
// inside the captured shape, so no structural mimicry divergence).
//
// `.claude/` is read-only (D-05); Load never writes to it. A missing/empty dir
// yields an empty contribution (the common case where the user has no `.claude/`
// or `.ass-guard/` extensions yet).
func Load(claudeDir, assguardDir string) (Registry, error) {
	reg, _, err := loadAll(claudeDir, assguardDir)

	return reg, err
}

// loadAll is Load plus the plugin-bundled MCP server set (the lowest MCP
// layer, below LoadUserMCPConfig — Discover consumes both; Load's public
// signature stays Registry-only).
func loadAll(claudeDir, assguardDir string) (Registry, map[string]ServerConfig, error) {
	home, _ := os.UserHomeDir()
	userClaude := filepath.Join(home, claudeDirName)
	userAssguard := filepath.Join(home, assguardDirName)

	// Phase 1: within each tree, user → project.
	claudeReg, err := loadTreeMerged(userClaude, claudeDir)
	if err != nil {
		return Registry{}, nil, err
	}

	assguardReg, err := loadTreeMerged(userAssguard, assguardDir)
	if err != nil {
		return Registry{}, nil, err
	}

	// Phase 2: across trees, `.claude/` wins over `.ass-guard/`.
	core := mergeRegistries(assguardReg, claudeReg)

	// 12-02 phase 3: the installed-plugin roots (the two lowest tiers). The
	// user root is the base; the project root overlays it. projectDir for the
	// v2 registry's projectPath filter is the parent of the project `.claude/`
	// dir (empty claudeDir → no project root).
	projectDir := ""
	if claudeDir != "" {
		projectDir = filepath.Dir(claudeDir)
	}

	pluginMCP := map[string]ServerConfig{}
	pluginHooks := []HookConfig(nil)

	userPlug := newRegistry()
	discoverInstalledPlugins(filepath.Join(userClaude, pluginsDirName), projectDir, userPlug, pluginMCP, &pluginHooks)

	projPlug := newRegistry()
	if claudeDir != "" {
		// Project-root plugin servers overwrite user-root ones on name
		// collision (project over user — the tier order).
		discoverInstalledPlugins(
			filepath.Join(claudeDir, pluginsDirName), projectDir, projPlug, pluginMCP, &pluginHooks)
	}

	merged := mergeRegistries(mergeRegistries(userPlug, projPlug), core)
	hooks := make([]HookConfig, 0, len(pluginHooks)+len(merged.Hooks))
	hooks = append(hooks, pluginHooks...)
	merged.Hooks = append(hooks, merged.Hooks...)

	// 21-01 PAR-03: the settings.json scopes join the flat hooks slice AFTER
	// the plugin hooks. Merge order here is irrelevant to firing — the runner
	// partitions by scope at construction (D-03) — and the loader's overlay
	// precedence above stays untouched (Pitfall 1: other consumers depend on
	// it). Every settings failure path degrades to a stderr warning + skip,
	// never an error up through Load (Pitfall 8).
	merged.Hooks = append(merged.Hooks, loadSettingsHooks(claudeDir, ScopeProject)...)
	merged.Hooks = append(merged.Hooks, loadSettingsHooks(claudeDir, ScopeUser)...)

	return merged, pluginMCP, nil
}

// settingsJSONName is the Claude-Code settings file hooks are read from
// (project <project>/.claude/settings.json and user ~/.claude/settings.json
// — the SAME files CC reads; read-only, never written).
const settingsJSONName = "settings.json"

// loadSettingsHooks reads one settings scope's hooks entries (21-01
// PAR-03/D-02): ScopeProject reads <claudeDir>/settings.json; ScopeUser
// reads ~/.claude/settings.json. The hooks key layout is the SAME
// hooks → event → matcher-group → {type,command,timeout} shape the plugin
// hooks.json parser accepts (CC-compatible). A malformed file degrades to a
// stderr warning + skip; an absent file contributes nothing silently
// (absent is normal); an oversized file skips with a warning (the
// pluginArtifactMaxBytes discipline). NEVER returns an error — a repo must
// not be able to brick session construction (Pitfall 8).
func loadSettingsHooks(claudeDir string, scope HookScope) []HookConfig {
	var path string

	switch scope {
	case ScopeProject:
		if claudeDir == "" {
			return nil // no project context — no project settings
		}

		path = filepath.Join(claudeDir, settingsJSONName)
	case ScopeUser:
		home, uerr := os.UserHomeDir()
		if uerr != nil {
			logPluginSkipf("user settings hooks: home dir unavailable (skipped): %v", uerr)

			return nil
		}

		path = filepath.Join(home, claudeDirName, settingsJSONName)
	case ScopePlugin:
		return nil // plugin scope has its own loader (parseHooksJSON)
	default:
		return nil
	}

	// Oversized settings skip loudly (readCapped re-guards the read itself).
	info, statErr := os.Stat(path)
	if statErr == nil && info.Size() > pluginArtifactMaxBytes {
		logPluginSkipf("settings %s exceeds %d bytes (hooks skipped)", path, pluginArtifactMaxBytes)

		return nil
	}

	hooks := parseHooksFile(path, "", scope)

	// Settings entries default to the 60s bound at parse time (the plan's
	// A2 divergence note: CC's documented command-hook default is 600s; the
	// existing 60s bound is kept to bound tool-loop latency).
	for i := range hooks {
		if hooks[i].TimeoutSec <= 0 {
			hooks[i].TimeoutSec = hookDefaultTimeoutSec
		}
	}

	return hooks
}

// loadTreeMerged loads user-scope then overlays project-scope (project wins).
// A missing user dir is an empty registry (non-fatal).
func loadTreeMerged(userDir, projectDir string) (Registry, error) {
	base, err := loadTree(userDir)
	if err != nil {
		return Registry{}, err
	}

	if projectDir == "" {
		return base, nil
	}

	overlay, err := loadTree(projectDir)
	if err != nil {
		return Registry{}, err
	}

	return mergeRegistries(base, overlay), nil
}

// loadTree walks one root discovering skills (`skills/*/SKILL.md`), commands
// (`commands/*.md`), and plugins (`plugins/*/manifest.json`) using Claude-Code's
// rules (D-07). Every read is via os.ReadFile (read-only). A missing root is an
// empty registry (non-fatal). A skill directory without SKILL.md is silently
// skipped (logged at debug — not an error).
func loadTree(root string) (Registry, error) {
	reg := newRegistry()

	if root == "" {
		return reg, nil
	}

	_, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return reg, nil // opt-in: missing dir is empty
		}

		return reg, fmt.Errorf("stat %q: %w", root, err)
	}

	err = discoverSkills(root, reg)
	if err != nil {
		return reg, err
	}

	err = discoverCommands(root, reg)
	if err != nil {
		return reg, err
	}

	err = discoverPlugins(root, reg)
	if err != nil {
		return reg, err
	}

	err = discoverAgents(root, reg)
	if err != nil {
		return reg, err
	}

	return reg, nil
}

// discoverAgents walks root/agents/*.md — the first-class `.claude/agents/`
// chain entry (project + user; 12-02). The filename stem is the fallback
// name; frontmatter supplies name/description/tools/model; the body is the
// system prompt. Unreadable/malformed files are silently skipped (the same
// `.claude/`-tree semantics as skills/commands).
func discoverAgents(root string, reg Registry) error {
	return discoverAgentsDir(filepath.Join(root, "agents"), reg, false)
}

// discoverAgentsDir walks one agents/ directory — the shared reader for the
// first-class `.claude|ass-guard/agents/` trees AND plugin-bundled `agents/`
// directories. warnSkips routes degradation through the stderr skip seam
// (plugin bundles warn; `.claude/` trees stay silent — the established
// split).
func discoverAgentsDir(agentsDir string, reg Registry, warnSkips bool) error {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read agents dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), markdownExt) {
			continue
		}

		agentPath := filepath.Join(agentsDir, e.Name())

		ag, err := loadAgentFile(agentPath, strings.TrimSuffix(e.Name(), markdownExt))
		if err != nil {
			if warnSkips {
				logPluginSkipf("agent definition %s skipped: %v", agentPath, err)
			}

			continue
		}

		reg.Agents[ag.Name] = ag
	}

	return nil
}

// loadAgentFile reads and parses one agent markdown file into an Agent.
func loadAgentFile(agentPath, fallbackName string) (Agent, error) {
	data, err := os.ReadFile(agentPath)
	if err != nil {
		return Agent{}, fmt.Errorf("read: %w", err)
	}

	return parseAgent(string(data), agentPath, fallbackName)
}

// parseAgent parses an agents/<name>.md: YAML frontmatter
// (name/description/tools/model) between `---` delimiters, then the body —
// the agent's system prompt. An empty description falls back to the first
// non-empty body line (the command precedent); neither drops the definition.
func parseAgent(content, path, fallbackName string) (Agent, error) {
	frontmatter, body := splitFrontmatter(content)

	var fm struct {
		Name        string    `yaml:"name"`
		Description string    `yaml:"description"`
		Tools       toolsList `yaml:"tools"`
		Model       string    `yaml:"model"`
	}

	err := yaml.Unmarshal([]byte(frontmatter), &fm)
	if err != nil {
		return Agent{}, fmt.Errorf("parse agent frontmatter %s: %w", path, err)
	}

	name := fm.Name
	if name == "" {
		name = fallbackName
	}

	desc := fm.Description
	if desc == "" {
		desc = firstNonEmptyLine(body)
	}

	if name == "" || desc == "" {
		return Agent{}, fmt.Errorf("%s: %w", path, errAgentNoIdentity)
	}

	return Agent{
		Name: name, Description: desc, Tools: fm.Tools, Model: fm.Model,
		Prompt: body, Path: path,
	}, nil
}

// discoverSkills walks root/skills/*/SKILL.md, parsing YAML frontmatter.
func discoverSkills(root string, reg Registry) error {
	return discoverSkillsDir(filepath.Join(root, "skills"), reg)
}

// discoverSkillsDir walks one skills/ directory (`<any-root>/skills/*/SKILL.md`)
// — the shared reader for `.claude|ass-guard/skills/` trees AND plugin-bundled
// `skills/` directories inside an installed cache (12-02).
func discoverSkillsDir(skillsDir string, reg Registry) error {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // no skills/ — fine
		}

		return fmt.Errorf("read skills dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		skillPath := filepath.Join(skillsDir, e.Name(), "SKILL.md")

		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue // missing SKILL.md — silently skip (not an error)
		}

		sk, err := parseSkill(string(data), skillPath)
		if err != nil {
			continue // malformed — skip, don't fail the whole load
		}

		if sk.Name == "" {
			sk.Name = e.Name() // fall back to dir name
		}

		reg.Skills[sk.Name] = sk
	}

	return nil
}

// discoverCommands walks root/commands/*.md (filename stem is the command name)
// and ONE level of subdirectories (commands/<ns>/<name>.md → key "<ns>:<name>",
// colon not slash — the layout `openspec init --tools claude` installs). zcode
// joins exactly one level; deeper trees are not flattened.
func discoverCommands(root string, reg Registry) error {
	return discoverCommandsDir(filepath.Join(root, "commands"), reg)
}

// discoverCommandsDir walks one commands/ directory — the shared reader for
// `.claude|ass-guard/commands/` trees AND plugin-bundled `commands/`
// directories inside an installed cache (12-02; same flat + one-level-ns
// rules, same command-name regex, same drop rules).
func discoverCommandsDir(cmdsDir string, reg Registry) error {
	entries, err := os.ReadDir(cmdsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read commands dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			err := discoverNamespacedCommands(filepath.Join(cmdsDir, e.Name()), e.Name(), reg)
			if err != nil {
				return err
			}

			continue
		}

		if !strings.HasSuffix(e.Name(), markdownExt) {
			continue
		}

		stem := strings.TrimSuffix(e.Name(), markdownExt)
		loadCommandFile(reg, filepath.Join(cmdsDir, e.Name()), stem)
	}

	return nil
}

// discoverNamespacedCommands walks one namespace directory
// (commands/<ns>/*.md), keying each command "<ns>:<stem>". Subdirectories
// inside the namespace are not walked — one level only (zcode rule).
func discoverNamespacedCommands(nsDir, ns string, reg Registry) error {
	entries, err := os.ReadDir(nsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read commands namespace dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), markdownExt) {
			continue
		}

		stem := strings.TrimSuffix(e.Name(), markdownExt)
		loadCommandFile(reg, filepath.Join(nsDir, e.Name()), ns+":"+stem)
	}

	return nil
}

// commandNameRe is zcode's command-name rule: keys must match
// ^[a-z0-9][a-z0-9_:-]{0,63}$ — a key that passes cannot be a path (no dots,
// slashes, or separators — T-8-04). Violators are dropped silently (the
// mimicry target is silent; no error, no warning).
var commandNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_:-]{0,63}$`)

// loadCommandFile reads and parses one command markdown file into reg. Invalid
// names, unreadable files, and files failing zcode's drop rules are silently
// skipped.
func loadCommandFile(reg Registry, cmdPath, name string) {
	if !commandNameRe.MatchString(name) {
		return // zcode drops invalid names silently
	}

	data, err := os.ReadFile(cmdPath)
	if err != nil {
		return
	}

	cmd, err := parseCommand(string(data), name, cmdPath)
	if err != nil {
		return // dropped (e.g. neither description nor body)
	}

	reg.Commands[cmd.Name] = cmd
}

// discoverPlugins walks root/plugins/*/manifest.json.
func discoverPlugins(root string, reg Registry) error {
	pluginsDir := filepath.Join(root, "plugins")

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read plugins dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		manifestPath := filepath.Join(pluginsDir, e.Name(), "manifest.json")

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // missing manifest — skip
		}

		var m struct {
			Name     string   `json:"name"`
			Skills   []string `json:"skills"`
			Commands []string `json:"commands"`
		}

		err = json.Unmarshal(data, &m)
		if err != nil {
			continue // malformed manifest — skip
		}

		if m.Name == "" {
			m.Name = e.Name()
		}

		reg.Plugins[m.Name] = Plugin{Name: m.Name, Skills: m.Skills, Commands: m.Commands, Path: manifestPath}
	}

	return nil
}

// installedPlugin is one entry of an installed_plugins.json registry (12-02,
// ECOSYSTEM-AUDIT §4.1 PLUG-01): the `name@marketplace` key plus the resolved
// install scope. Both on-disk shapes parse into this (measured live
// 2026-08-18): the v1 array form and the v2 object form the operator's real
// `~/.claude/plugins/` carries (`{"version":2,"plugins":{key:[entries]}}` —
// a per-plugin LIST of scoped installs, from which the first entry whose
// projectPath is empty or matches the active project is taken; entries scoped
// to OTHER projects never leak in).
type installedPlugin struct {
	Key         string // full "name@marketplace"
	Name        string // pre-@ plugin name
	Source      string // marketplace id (post-@)
	InstallPath string // as recorded (relative or absolute)
	Scope       string // "user" | "project" | "local"
	Version     string
}

// parseInstalledPlugins reads `<pluginsRoot>/installed_plugins.json` into the
// neutral entry list. A missing file yields nil (plugins are opt-in). A
// malformed file degrades to skip-with-warning + nil error (the registry keeps
// loading — the graceful-degradation contract). projectDir gates the v2
// project-scoped entries.
func parseInstalledPlugins(pluginsRoot, projectDir string) []installedPlugin {
	data, err := os.ReadFile(filepath.Join(pluginsRoot, installedPluginsFile))
	if err != nil {
		return nil // missing/unreadable registry — opt-in, not an error
	}

	// v1: a top-level array of {name, installPath, scope, version}.
	var v1 []struct {
		Name        string `json:"name"`
		InstallPath string `json:"installPath"` //nolint:tagliatelle // camelCase wire field
		Scope       string `json:"scope"`
		Version     string `json:"version"`
	}

	if json.Unmarshal(data, &v1) == nil && len(v1) > 0 {
		out := make([]installedPlugin, 0, len(v1))
		for _, e := range v1 {
			out = append(out, installedPluginFromKey(e.Name, e.InstallPath, e.Scope, e.Version))
		}

		return out
	}

	// v2: {"version":2,"plugins":{"name@marketplace":[{scope,projectPath,installPath,version}]}}.
	var v2 struct {
		Plugins map[string][]struct {
			Scope       string `json:"scope"`
			ProjectPath string `json:"projectPath"` //nolint:tagliatelle // camelCase wire field
			InstallPath string `json:"installPath"` //nolint:tagliatelle // camelCase wire field
			Version     string `json:"version"`
		} `json:"plugins"`
	}

	jerr := json.Unmarshal(data, &v2)
	if jerr != nil || len(v2.Plugins) == 0 {
		logPluginSkipf("unrecognized installed_plugins.json shape in %s (skipping plugins)", pluginsRoot)

		return nil
	}

	keys := make([]string, 0, len(v2.Plugins))
	for k := range v2.Plugins {
		keys = append(keys, k)
	}

	sort.Strings(keys) // deterministic regardless of map order

	out := make([]installedPlugin, 0, len(keys))

	for _, key := range keys {
		for _, e := range v2.Plugins[key] {
			// A project/local-scoped install applies only to ITS project —
			// entries scoped elsewhere (or when no project context exists)
			// never leak into this load.
			if e.ProjectPath != "" && (projectDir == "" || !samePath(e.ProjectPath, projectDir)) {
				continue
			}

			out = append(out, installedPluginFromKey(key, e.InstallPath, e.Scope, e.Version))

			break // first applicable install wins
		}
	}

	return out
}

// installedPluginFromKey splits the "name@marketplace" key and fills the
// provenance fields.
func installedPluginFromKey(key, installPath, scope, version string) installedPlugin {
	name, source, _ := strings.Cut(key, "@")

	return installedPlugin{
		Key: key, Name: name, Source: source,
		InstallPath: installPath, Scope: scope, Version: version,
	}
}

// samePath compares two filesystem paths modulo symlink resolution (best
// effort; on error the raw strings decide).
func samePath(a, b string) bool {
	ra, aerr := filepath.EvalSymlinks(a)
	if aerr == nil {
		a = ra
	}

	rb, berr := filepath.EvalSymlinks(b)
	if berr == nil {
		b = rb
	}

	return filepath.Clean(a) == filepath.Clean(b)
}

// pluginArtifactMaxBytes caps every plugin-artifact read (manifests, SKILL.md,
// command files — threat T-12-02-01/T-12-02-03: untrusted filesystem content
// cannot pin the loader on a pathological file).
const pluginArtifactMaxBytes = 1 << 20

// readCapped reads path when it exists and is at most maxBytes; ok=false on
// missing/oversized (the caller skips with a warning).
func readCapped(path string, maxBytes int64) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxBytes {
		return nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	return data, true
}

// resolveInstallPath resolves an entry's installPath against the plugins root
// (relative → root-joined; absolute → as-is) and enforces the containment
// invariant (threat T-12-02-01): the symlink-resolved result must stay UNDER
// the symlink-resolved root. An escaping or nonexistent path returns ""
// (the caller skips with a warning).
func resolveInstallPath(pluginsRoot, installPath string) string {
	if installPath == "" {
		return ""
	}

	path := installPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(pluginsRoot, installPath)
	}

	rootReal, rerr := filepath.EvalSymlinks(pluginsRoot)
	if rerr != nil {
		rootReal = pluginsRoot
	}

	pathReal, perr := filepath.EvalSymlinks(path)
	if perr != nil {
		return "" // nonexistent — the caller's absent-entry skip
	}

	rel, relErr := filepath.Rel(rootReal, pathReal)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "" // escapes the probed root — refuse (symlinked traversal)
	}

	return pathReal
}

// discoverInstalledPlugins walks one plugins root's installed_plugins.json
// registry (12-02): for every entry it resolves the cache install path, reads
// the `.claude-plugin/plugin.json` manifest, and merges the bundled
// contributions (skills/, commands/, agents/, .mcp.json, hooks/hooks.json)
// into reg / mcpOut / hooksOut with the plugin's provenance. The out-params
// exist because Registry's maps mutate through the by-value receiver but the
// Hooks SLICE does not — callers pass their own accumulators. Every
// degradation is a stderr warning naming the path — never a load error (the
// registry keeps loading); nothing is written anywhere.
func discoverInstalledPlugins(
	pluginsRoot, projectDir string, reg Registry,
	mcpOut map[string]ServerConfig, hooksOut *[]HookConfig,
) {
	if pluginsRoot == "" {
		return
	}

	if _, err := os.Stat(pluginsRoot); err != nil { //nolint:noinlineerr // skip-path
		return // missing root — opt-in
	}

	entries := parseInstalledPlugins(pluginsRoot, projectDir)

	for i := range entries {
		loadInstalledPlugin(pluginsRoot, &entries[i], reg, mcpOut, hooksOut)
	}
}

// loadInstalledPlugin resolves, validates, and registers ONE installed-plugin
// entry. Every degradation path skips THIS plugin with a stderr warning
// naming the artifact — the registry keeps loading.
func loadInstalledPlugin(
	pluginsRoot string, entry *installedPlugin,
	reg Registry, mcpOut map[string]ServerConfig, hooksOut *[]HookConfig,
) {
	install, name, version, manifestPath, ok := resolveInstalledPlugin(pluginsRoot, entry)
	if !ok {
		return
	}

	pluginReg := discoverPluginBundle(install)

	maps.Copy(reg.Skills, pluginReg.Skills)
	maps.Copy(reg.Commands, pluginReg.Commands)
	maps.Copy(reg.Agents, pluginReg.Agents)
	maps.Copy(mcpOut, parsePluginMCPJSON(install))

	// 12-02 Task 4: the bundled hooks/hooks.json (all events — mapped AND
	// unmapped/observe-only; the runner decides firing).
	*hooksOut = append(*hooksOut, parseHooksJSON(
		filepath.Join(install, "hooks", "hooks.json"), install)...)

	reg.Plugins[name] = Plugin{
		Name: name, Skills: sortedKeys(pluginReg.Skills), Commands: sortedKeys(pluginReg.Commands),
		Path: manifestPath, Source: entry.Source, Version: version,
		Scope: entry.Scope, InstallPath: install,
	}
}

// resolveInstalledPlugin resolves the entry's install path, reads + validates
// the .claude-plugin/plugin.json manifest, and returns the registration
// identity (resolved install dir, plugin name, manifest path). ok=false on
// every degradation (absent path, unreadable/malformed manifest) — warned by
// the caller-visible seam, never fatal.
func resolveInstalledPlugin( //nolint:nonamedreturns // five-string result clarity
	pluginsRoot string, entry *installedPlugin,
) (install, name, version, manifestPath string, ok bool) {
	install = resolveInstallPath(pluginsRoot, entry.InstallPath)
	if install == "" {
		logPluginSkipf("installed plugin %s: install path %q not found under %s (skipped)",
			entry.Key, entry.InstallPath, pluginsRoot)

		return "", "", "", "", false
	}

	manifestPath = filepath.Join(install, pluginManifestRelPath)

	data, readable := readCapped(manifestPath, pluginArtifactMaxBytes)
	if !readable {
		logPluginSkipf("installed plugin %s: unreadable manifest %s (skipped)", entry.Key, manifestPath)

		return "", "", "", "", false
	}

	var m struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	if json.Unmarshal(data, &m) != nil || m.Name == "" && entry.Name == "" {
		logPluginSkipf("installed plugin %s: malformed manifest %s (skipped)", entry.Key, manifestPath)

		return "", "", "", "", false
	}

	name = m.Name
	if name == "" {
		name = entry.Name
	}

	version = entry.Version
	if version == "" {
		version = m.Version
	}

	return install, name, version, manifestPath, true
}

// discoverPluginBundle walks ONE cache install's bundled contribution dirs
// (skills/, commands/, agents/) into a fresh registry.
func discoverPluginBundle(install string) Registry {
	pluginReg := newRegistry()

	_ = discoverSkillsDir(filepath.Join(install, "skills"), pluginReg)
	_ = discoverCommandsDir(filepath.Join(install, "commands"), pluginReg)
	_ = discoverAgentsDir(filepath.Join(install, "agents"), pluginReg, true)

	return pluginReg
}

// sortedKeys returns the map's keys sorted (the Plugin contribution lists).
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}

	sort.Strings(out)

	return out
}

// parsePluginMCPJSON reads a plugin bundle's .mcp.json (the same
// {"mcpServers":{…}} shape as the project file / ~/.claude.json) into neutral
// ServerConfigs. A missing file yields nil; a malformed file warns and yields
// nil (skip-with-warning — the rest of the bundle still loaded).
func parsePluginMCPJSON(install string) map[string]ServerConfig {
	data, ok := readCapped(filepath.Join(install, mcpJSONName), pluginArtifactMaxBytes)
	if !ok {
		return nil
	}

	var raw struct {
		McpServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			Cwd     string            `json:"cwd"`
		} `json:"mcpServers"` //nolint:tagliatelle // .mcp.json camelCase wire field
	}

	if json.Unmarshal(data, &raw) != nil {
		logPluginSkipf("malformed %s in %s (skipped)", mcpJSONName, install)

		return nil
	}

	out := make(map[string]ServerConfig, len(raw.McpServers))
	for serverName, s := range raw.McpServers {
		out[serverName] = ServerConfig{
			Name: serverName, Command: s.Command, Args: s.Args, Env: s.Env, Cwd: s.Cwd,
		}
	}

	return out
}

// logPluginSkip emits one stderr warning line for a degraded plugin artifact
// (the same swappable stderr seam as the shadow warnings — stderr NEVER
// stdout, the ACP discipline).
func logPluginSkipf(format string, args ...any) {
	shadowWarnLogger.Warn("plugin skip: " + fmt.Sprintf(format, args...))
}

// toolsList is a frontmatter tools field accepting every documented form:
// a YAML list (`[Read, Bash]` / block list), a SPACE-separated scalar
// (`Read Edit Bash(git:*)` — Claude Code's allowed-tools syntax, measured live
// in samber/cc-skills-golang 1.5.0), or a COMMA-separated scalar (`Read, Grep`
// — the .claude/agents convention). A strict []string would drop the ENTIRE
// skill/command on the scalar forms (live-proven 2026-08-18: 40 real skills
// silently lost) — this is the native-consumer tolerance, not a divergence.
type toolsList []string

// UnmarshalYAML implements the tolerant list decoding.
func (t *toolsList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			out = append(out, strings.TrimSpace(item.Value))
		}

		*t = out

		return nil
	case yaml.ScalarNode:
		// Commas and whitespace both separate; parens stay (Bash(git:*) is one tool spec).
		fields := strings.FieldsFunc(node.Value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })

		out := make([]string, 0, len(fields))
		for _, f := range fields {
			if f != "" {
				out = append(out, f)
			}
		}

		*t = out

		return nil
	case 0: // explicit null (`allowed-tools:` with no value)
		*t = nil

		return nil
	case yaml.DocumentNode, yaml.MappingNode, yaml.AliasNode:
		return errToolsFieldShape
	default:
		return errToolsFieldShape
	}
}

// parseSkill parses a SKILL.md: YAML frontmatter (name/description/allowed-tools)
// between `---` delimiters, then the body. The body is not stored (skills are
// surfaced as discovered metadata; execution bridging is downstream).
func parseSkill(content, path string) (Skill, error) {
	frontmatter, _ := splitFrontmatter(content)

	var fm struct {
		Name         string    `yaml:"name"`
		Description  string    `yaml:"description"`
		AllowedTools toolsList `yaml:"allowed-tools"` //nolint:tagliatelle // kebab-case frontmatter
	}

	err := yaml.Unmarshal([]byte(frontmatter), &fm)
	if err != nil {
		return Skill{}, fmt.Errorf("parse skill frontmatter %s: %w", path, err)
	}

	return Skill{
		Name: fm.Name, Description: fm.Description, AllowedTools: fm.AllowedTools, Path: path,
	}, nil
}

// commandFrontmatter is the recognized command-frontmatter key set (zcode's).
// All fields are parsed but NOT acted on functionally (D-09).
type commandFrontmatter struct {
	Description  string   `yaml:"description"`
	ArgumentHint string   `yaml:"argument-hint"` //nolint:tagliatelle // kebab-case frontmatter key
	AllowedTools []string `yaml:"allowed-tools"` //nolint:tagliatelle // kebab-case frontmatter key
	Model        string   `yaml:"model"`
}

// parseCommand parses a command markdown file: flat frontmatter + body.
//
// zcode's frontmatter parser is FLAT: only single-line top-level entries count;
// indented lines and multi-line (block) arrays are silently dropped (STACK
// §Feature 1a / PITFALLS Pitfall 6 — accepting values zcode drops is a mimicry
// divergence). Strict yaml.Unmarshal is attempted first so single-line values
// (quoted strings, flow arrays) decode robustly with unknown keys ignored; a
// file whose frontmatter fails strict YAML (tab indentation, comma-scalar
// lists) still loads through the flat single-line view — zcode never rejects a
// command file for YAML invalidity.
//
// Description rules (zcode): empty description + non-empty body → first
// non-empty body line; neither description nor body → the command is dropped.
func parseCommand(content, name, path string) (Command, error) {
	frontmatter, body := splitFrontmatter(content)

	fm := parseCommandFrontmatter(frontmatter)

	desc := fm.Description
	if desc == "" {
		desc = firstNonEmptyLine(body)
	}

	if desc == "" {
		// Dropped (zcode rule): neither description nor body. The caller skips
		// the file silently; the error only signals the drop.
		return Command{}, errCommandDropped
	}

	return Command{
		Name: name, Description: desc, Body: body, Path: path,
		ArgumentHint: fm.ArgumentHint, AllowedTools: fm.AllowedTools, Model: fm.Model,
	}, nil
}

// parseCommandFrontmatter decodes the recognized keys under zcode's flat
// semantics. Strict YAML is tried first (unknown keys ignored); on error the
// flat single-line parser takes over. On success, recognized keys without a
// single-line entry (multi-line/indented blocks) are dropped — zcode parity.
func parseCommandFrontmatter(frontmatter string) commandFrontmatter {
	var fm commandFrontmatter

	err := yaml.Unmarshal([]byte(frontmatter), &fm)
	if err != nil {
		return parseFlatCommandFrontmatter(frontmatter)
	}

	// Flat overlay: recognized keys count only from single-line entries. A key
	// whose only form is a multi-line block (valid YAML — e.g. a block-style
	// allowed-tools list) is dropped, matching what zcode's flat parser sees.
	entries := flatSingleLineEntries(frontmatter)

	if _, ok := entries["description"]; !ok {
		fm.Description = ""
	}

	if _, ok := entries[fmKeyArgumentHint]; !ok {
		fm.ArgumentHint = ""
	}

	if _, ok := entries[fmKeyModel]; !ok {
		fm.Model = ""
	}

	if _, ok := entries[fmKeyAllowedTools]; !ok {
		fm.AllowedTools = nil
	}

	return fm
}

// Recognized flat-frontmatter keys (zcode's set — kebab-case literals).
const (
	fmKeyArgumentHint = "argument-hint"
	fmKeyAllowedTools = "allowed-tools"
	fmKeyModel        = "model"
	minQuotedValueLen = 2 // shortest possible quoted value: `""`
)

// parseFlatCommandFrontmatter is the flat single-line fallback: split the
// frontmatter on newlines, take lines matching `^key: value` for the recognized
// keys only, drop indented continuations and multi-line values (zcode's
// observable outcome on the same file).
func parseFlatCommandFrontmatter(frontmatter string) commandFrontmatter {
	var fm commandFrontmatter

	for key, value := range flatSingleLineEntries(frontmatter) {
		switch key {
		case "description":
			fm.Description = value
		case fmKeyArgumentHint:
			fm.ArgumentHint = value
		case fmKeyModel:
			fm.Model = value
		case fmKeyAllowedTools:
			fm.AllowedTools = parseAllowedToolsValue(value)
		}
	}

	return fm
}

// flatSingleLineEntries returns the single-line `key: value` entries for the
// recognized keys. Indented lines (block-list continuations) never match, and
// a bare `key:` line (block-list opener with no inline value) is treated as
// absent — zcode's flat parser sees no value for it.
func flatSingleLineEntries(frontmatter string) map[string]string {
	recognized := map[string]bool{
		"description": true, fmKeyArgumentHint: true, fmKeyAllowedTools: true, fmKeyModel: true,
	}

	out := map[string]string{}

	for line := range strings.SplitSeq(frontmatter, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue // indented continuation — zcode drops it
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if !recognized[key] {
			continue
		}

		value = unquoteFrontmatterValue(strings.TrimSpace(value))
		if value == "" {
			continue // bare `key:` opener — no single-line value
		}

		out[key] = value
	}

	return out
}

// parseAllowedToolsValue parses an allowed-tools value from its single-line
// form: a comma-separated scalar (`Bash, Read`) or a YAML flow array
// (`[Bash, Read]`) — both yield the same slice.
func parseAllowedToolsValue(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")

	parts := strings.Split(value, ",")

	out := make([]string, 0, len(parts))

	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// unquoteFrontmatterValue strips one level of surrounding single or double
// quotes from a flat frontmatter value.
func unquoteFrontmatterValue(value string) string {
	if len(value) >= minQuotedValueLen {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}

	return value
}

// firstNonEmptyLine returns the first non-empty (trimmed) line of body, or ""
// (zcode's description fallback).
func firstNonEmptyLine(body string) string {
	for line := range strings.SplitSeq(body, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

// splitFrontmatter splits a markdown file into (frontmatter, body). A leading
// `---` line starts the frontmatter; the next line starting with `---` ends it.
// Files without frontmatter return ("", content).
//
//nolint:nonamedreturns // gocritic unnamedResult prefers names for two-string clarity
func splitFrontmatter(content string) (frontmatter, body string) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != frontmatterDelim {
		return "", content
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontmatterDelim {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n")
		}
	}

	return "", content // no closing delim — treat as no frontmatter
}

// shadowWarnLogger emits same-key precedence-overwrite warnings. Default
// target is stderr (NEVER stdout — the ACP discipline); tests swap it to
// capture output. Swappable seam per the Phase-8 plan (T3).
var shadowWarnLogger = slog.New(slog.NewTextHandler(os.Stderr, nil)) //nolint:gochecknoglobals // swappable test seam

// mergeRegistries returns base with overlay applied: overlay wins on key
// conflict (per D-06 precedence — direction unchanged, locked v1.0 D-06).
// Every same-key overwrite in the Skills and Commands maps emits one warning
// naming the winning and shadowed file paths (CMD-05: "which file answered my
// invocation?" must be answerable from the log alone).
func mergeRegistries(base, overlay Registry) Registry {
	out := newRegistry()

	mergeSkillsWithShadowWarnings(out.Skills, base.Skills, overlay.Skills)
	mergeCommandsWithShadowWarnings(out.Commands, base.Commands, overlay.Commands)
	mergeAgentsWithShadowWarnings(out.Agents, base.Agents, overlay.Agents)

	// Hooks accumulate (a slice — every tier's hooks fire; base tiers first).
	out.Hooks = append(append([]HookConfig(nil), base.Hooks...), overlay.Hooks...)

	maps.Copy(out.Plugins, base.Plugins)

	maps.Copy(out.Plugins, overlay.Plugins)

	return out
}

// mergeAgentsWithShadowWarnings copies base then overlay into out, warning on
// each same-key agent-definition overwrite (the shared mechanism; agent keys
// shadow across the full chain incl. plugin bundles).
func mergeAgentsWithShadowWarnings(out, base, overlay map[string]Agent) {
	maps.Copy(out, base)

	for key, ov := range overlay {
		if bv, shadowed := base[key]; shadowed {
			logShadowWarning("agent", key, ov.Path, bv.Path)
		}

		out[key] = ov
	}
}

// mergeSkillsWithShadowWarnings copies base then overlay into out, warning on
// each same-key skill overwrite (the shared mechanism — one helper per map).
func mergeSkillsWithShadowWarnings(out, base, overlay map[string]Skill) {
	maps.Copy(out, base)

	for key, ov := range overlay {
		if bv, shadowed := base[key]; shadowed {
			logShadowWarning("skill", key, ov.Path, bv.Path)
		}

		out[key] = ov
	}
}

// mergeCommandsWithShadowWarnings copies base then overlay into out, warning
// on each same-key command overwrite.
func mergeCommandsWithShadowWarnings(out, base, overlay map[string]Command) {
	maps.Copy(out, base)

	for key, ov := range overlay {
		if bv, shadowed := base[key]; shadowed {
			logShadowWarning("command", "/"+key, ov.Path, bv.Path)
		}

		out[key] = ov
	}
}

// logShadowWarning emits one stderr warning line naming both file paths so a
// support session can answer "which file answered my invocation?" from the
// log alone (PITFALLS Pitfall 5 — shadowing observability).
func logShadowWarning(kind, key, winningPath, shadowedPath string) {
	shadowWarnLogger.Warn(kind + " " + key + ": " + winningPath + " shadows " + shadowedPath)
}

// newRegistry returns a Registry with initialized maps.
func newRegistry() Registry {
	return Registry{
		Skills: map[string]Skill{}, Commands: map[string]Command{},
		Plugins: map[string]Plugin{}, Agents: map[string]Agent{},
	}
}

// AllAgents returns the registry's agent definitions sorted by name
// (deterministic — the type-listing order).
func (r Registry) AllAgents() []Agent {
	out := make([]Agent, 0, len(r.Agents))
	for _, a := range r.Agents {
		out = append(out, a)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// AllSkills returns the registry's skills sorted by name (deterministic).
func (r Registry) AllSkills() []Skill {
	out := make([]Skill, 0, len(r.Skills))
	for _, s := range r.Skills {
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// AllCommands returns the registry's commands sorted by name (deterministic).
func (r Registry) AllCommands() []Command {
	out := make([]Command, 0, len(r.Commands))
	for _, c := range r.Commands {
		out = append(out, c)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// AllPlugins returns the registry's plugins sorted by name (deterministic).
func (r Registry) AllPlugins() []Plugin {
	out := make([]Plugin, 0, len(r.Plugins))
	for name := range r.Plugins {
		out = append(out, r.Plugins[name])
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// LoadUserMCPConfig reads the user-scope MCP servers from ~/.claude.json's
// `mcpServers` object (same shape as .mcp.json). It returns a neutral
// map[string]ServerConfig the MCP host merges with the project .mcp.json
// (project wins on name collision — same precedence direction as the rest). A
// missing file or missing mcpServers key yields an empty map (non-fatal).
func LoadUserMCPConfig() (map[string]ServerConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home dir: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(home, claudeJSONFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]ServerConfig{}, nil
		}

		return nil, fmt.Errorf("read %s: %w", claudeJSONFile, err)
	}

	var raw struct {
		McpServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			Cwd     string            `json:"cwd"`
		} `json:"mcpServers"` //nolint:tagliatelle // ~/.claude.json wire field
	}

	err = json.Unmarshal(data, &raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", claudeJSONFile, err)
	}

	out := make(map[string]ServerConfig, len(raw.McpServers))
	for name, s := range raw.McpServers {
		out[name] = ServerConfig{Name: name, Command: s.Command, Args: s.Args, Env: s.Env, Cwd: s.Cwd}
	}

	return out, nil
}

// Discover ties the loader together: resolves the roots from projectDir, calls
// loadAll + LoadUserMCPConfig, and returns the precedence-resolved registry
// plus the non-project MCP server set — user-scope (~/.claude.json) OVER the
// plugin-bundled .mcp.json servers (the plugin set is the LOWEST layer: a
// same-name collision resolves to the non-plugin entry; 12-02 Task 3). The
// PROJECT .mcp.json merge happens at the host seam (cmd/ass-guard's spawnMCP —
// project wins over this whole set) to keep the package cycle-free
// (internal/ecosys does not import internal/mcp).
func Discover(projectDir string) (Registry, []ServerConfig, error) {
	reg, pluginMCP, err := loadAll(filepath.Join(projectDir, claudeDirName), filepath.Join(projectDir, assguardDirName))
	if err != nil {
		return Registry{}, nil, fmt.Errorf("load ecosystem: %w", err)
	}

	userMCP, err := LoadUserMCPConfig()
	if err != nil {
		return Registry{}, nil, fmt.Errorf("load user mcp config: %w", err)
	}

	// Merge: plugin servers first (lowest), user-scope over them.
	merged := make(map[string]ServerConfig, len(pluginMCP)+len(userMCP))
	maps.Copy(merged, pluginMCP)
	maps.Copy(merged, userMCP)

	servers := make([]ServerConfig, 0, len(merged))
	for _, sc := range merged {
		servers = append(servers, sc)
	}

	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })

	return reg, servers, nil
}

// EnsureGitignore creates `.ass-guard/.gitignore` with the Phase-2 D-07 content
// on first run (idempotent on subsequent runs). This is the ONE write the
// package performs, ONLY under assguardDir, NEVER under .claude/. A path that
// resolves to a `.claude/` directory is rejected (T-5-08 clobber mitigation).
func EnsureGitignore(assguardDir string) error {
	if assguardDir == "" {
		return errEmptyAssguardDir
	}

	if looksLikeClaude(assguardDir) {
		return errClaudeReadonly
	}

	giPath := filepath.Join(assguardDir, ".gitignore")

	_, statErr := os.Stat(giPath)
	if statErr == nil {
		return nil // already exists — idempotent
	}

	err := os.MkdirAll(assguardDir, dirPerms)
	if err != nil {
		return fmt.Errorf("create assguard dir: %w", err)
	}

	err = os.WriteFile(giPath, []byte(gitignoreContent), filePerms)
	if err != nil {
		return fmt.Errorf("write gitignore: %w", err)
	}

	return nil
}

// looksLikeClaude reports whether path is a `.claude/` directory (the read-only
// contract target). It checks the base name conservatively.
func looksLikeClaude(path string) bool {
	return filepath.Base(path) == claudeDirName
}
