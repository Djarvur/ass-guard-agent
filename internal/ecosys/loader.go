package ecosys

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
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
// `.claude/` is read-only (D-05); Load never writes to it. A missing/empty dir
// yields an empty contribution (the common case where the user has no `.claude/`
// or `.ass-guard/` extensions yet).
func Load(claudeDir, assguardDir string) (Registry, error) {
	home, _ := os.UserHomeDir()
	userClaude := filepath.Join(home, claudeDirName)
	userAssguard := filepath.Join(home, assguardDirName)

	// Phase 1: within each tree, user → project.
	claudeReg, err := loadTreeMerged(userClaude, claudeDir)
	if err != nil {
		return Registry{}, err
	}

	assguardReg, err := loadTreeMerged(userAssguard, assguardDir)
	if err != nil {
		return Registry{}, err
	}

	// Phase 2: across trees, `.claude/` wins over `.ass-guard/`.
	return mergeRegistries(assguardReg, claudeReg), nil
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

	return reg, nil
}

// discoverSkills walks root/skills/*/SKILL.md, parsing YAML frontmatter.
func discoverSkills(root string, reg Registry) error {
	skillsDir := filepath.Join(root, "skills")

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

// discoverCommands walks root/commands/*.md (filename stem is the command name).
func discoverCommands(root string, reg Registry) error {
	cmdsDir := filepath.Join(root, "commands")

	entries, err := os.ReadDir(cmdsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read commands dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), markdownExt) {
			continue
		}

		cmdPath := filepath.Join(cmdsDir, e.Name())

		data, err := os.ReadFile(cmdPath)
		if err != nil {
			continue
		}

		stem := strings.TrimSuffix(e.Name(), markdownExt)

		cmd, err := parseCommand(string(data), stem, cmdPath)
		if err != nil {
			continue
		}

		reg.Commands[cmd.Name] = cmd
	}

	return nil
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

// parseSkill parses a SKILL.md: YAML frontmatter (name/description/allowed-tools)
// between `---` delimiters, then the body. The body is not stored (skills are
// surfaced as discovered metadata; execution bridging is downstream).
func parseSkill(content, path string) (Skill, error) {
	frontmatter, _ := splitFrontmatter(content)

	var fm struct {
		Name         string   `yaml:"name"`
		Description  string   `yaml:"description"`
		AllowedTools []string `yaml:"allowed-tools"` //nolint:tagliatelle // kebab-case frontmatter
	}

	err := yaml.Unmarshal([]byte(frontmatter), &fm)
	if err != nil {
		return Skill{}, fmt.Errorf("parse skill frontmatter %s: %w", path, err)
	}

	return Skill{
		Name: fm.Name, Description: fm.Description, AllowedTools: fm.AllowedTools, Path: path,
	}, nil
}

// parseCommand parses a command markdown file: frontmatter (description) + body
// (the command body with $ARGUMENTS).
func parseCommand(content, name, path string) (Command, error) {
	frontmatter, body := splitFrontmatter(content)

	var fm struct {
		Description string `yaml:"description"`
	}

	err := yaml.Unmarshal([]byte(frontmatter), &fm)
	if err != nil {
		return Command{}, fmt.Errorf("parse command frontmatter %s: %w", path, err)
	}

	return Command{Name: name, Description: fm.Description, Body: body, Path: path}, nil
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

// mergeRegistries returns base with overlay applied: overlay wins on key
// conflict (per D-06 precedence). Both inputs are left unchanged.
func mergeRegistries(base, overlay Registry) Registry {
	out := newRegistry()

	maps.Copy(out.Skills, base.Skills)

	maps.Copy(out.Skills, overlay.Skills)

	maps.Copy(out.Commands, base.Commands)

	maps.Copy(out.Commands, overlay.Commands)

	maps.Copy(out.Plugins, base.Plugins)

	maps.Copy(out.Plugins, overlay.Plugins)

	return out
}

// newRegistry returns a Registry with initialized maps.
func newRegistry() Registry {
	return Registry{
		Skills: map[string]Skill{}, Commands: map[string]Command{}, Plugins: map[string]Plugin{},
	}
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
	for _, p := range r.Plugins {
		out = append(out, p)
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

// Discover ties the loader together: resolves the four roots from projectDir,
// calls Load + LoadUserMCPConfig, and returns the precedence-resolved registry
// plus the user-scope MCP configs. The project .mcp.json merge happens in the
// MCP host (internal/mcp) — Discover returns ONLY user-scope MCP configs to keep
// the package cycle-free (internal/ecosys does not import internal/mcp).
func Discover(projectDir string) (Registry, []ServerConfig, error) {
	reg, err := Load(filepath.Join(projectDir, claudeDirName), filepath.Join(projectDir, assguardDirName))
	if err != nil {
		return Registry{}, nil, fmt.Errorf("load ecosystem: %w", err)
	}

	userMCP, err := LoadUserMCPConfig()
	if err != nil {
		return Registry{}, nil, fmt.Errorf("load user mcp config: %w", err)
	}

	servers := make([]ServerConfig, 0, len(userMCP))
	for _, sc := range userMCP {
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
