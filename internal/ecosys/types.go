package ecosys

// Skill is one Claude-Code skill discovered from `<name>/SKILL.md` (D-07). The
// YAML frontmatter supplies Name/Description/AllowedTools; the body is the
// instruction text. Path is the on-disk SKILL.md location (read-only).
type Skill struct {
	Name         string
	Description  string
	AllowedTools []string
	Path         string

	// UserInvocable carries the CC-parity `user-invocable` frontmatter
	// (20-04/D-04): nil (absent) or true keeps the skill slash-addressable;
	// an explicit false EXCLUDES it from the slash chain and the
	// advertisement (it cannot fire via /name) while the model-invocation
	// path keeps it — only the slash surface excludes it.
	UserInvocable *bool
}

// Command is one slash-command discovered from `commands/<name>.md` (flat) or
// `commands/<ns>/<name>.md` (one-level namespaced, keyed "<ns>:<name>" — the
// layout `openspec init --tools claude` installs). The filename stem (or
// colon-joined namespace:stem) is the Name; the frontmatter supplies
// Description plus the extended parsed-only fields; the body (carrying the
// `$ARGUMENTS` placeholder) is Body. Path is the on-disk location.
type Command struct {
	Name        string
	Description string
	Body        string
	Path        string

	// ArgumentHint, AllowedTools, and Model are PARSED from frontmatter but NOT
	// acted on functionally (D-09: parse-side only — zcode surfaces them for
	// UX/matching; ass-guard stores them for mimicry parity without behavior).
	ArgumentHint string   // frontmatter `argument-hint`
	AllowedTools []string // frontmatter `allowed-tools` (comma scalar or list)
	Model        string   // frontmatter `model`
}

// Plugin is one plugin discovered from `plugins/<name>/manifest.json` (the
// simple D-07 shape) or from an installed-plugin cache entry (12-02: the
// `installed_plugins.json` registry + `plugins/cache/<marketplace>/<plugin>/<version>/`
// layout with `.claude-plugin/plugin.json` manifests). The manifest declares
// the plugin's contributed skill/command names. Path is the on-disk manifest
// location. Installed-cache provenance (Source marketplace, Version, Scope,
// InstallPath) is empty for the simple shape.
type Plugin struct {
	Name     string
	Skills   []string
	Commands []string
	Path     string

	// Installed-plugin provenance (12-02; zero for the simple shape).
	Source      string // marketplace id parsed from the name@marketplace key
	Version     string // installed version (dir name / registry field)
	Scope       string // "user" | "project" | "local"
	InstallPath string // resolved cache install path
}

// Agent is one subagent type definition (12-02): a YAML-frontmatter markdown
// file — `agents/<name>.md` inside a plugin cache bundle or a first-class
// `.claude/agents/<name>.md` (project + user). Frontmatter supplies
// name/description/tools/model; the body after frontmatter is the agent's
// system prompt. Discovered agents register as spawnable subagent types on the
// existing PARA machinery (the type listing is dynamic content merged into the
// captured shape; per-type prompts/tools apply at dispatch).
type Agent struct {
	Name        string
	Description string
	Tools       []string
	Model       string
	Prompt      string
	Path        string
}

// Registry aggregates discovered skills/commands/plugins/agents by name. Load
// returns the precedence-resolved registry; accessors (AllSkills/AllCommands/
// AllPlugins/AllAgents) return deterministically-ordered slices for the
// Shaper/Session to consume.
type Registry struct {
	Skills   map[string]Skill
	Commands map[string]Command
	Plugins  map[string]Plugin
	Agents   map[string]Agent

	// Hooks carries every parsed plugin-bundled hook (12-02 Task 4), all
	// events — mapped AND unmapped (observe-only). A slice, not a map: firing
	// order follows merge order (base tiers then overlays).
	Hooks []HookConfig
}

// ServerConfig is a neutral MCP server config (mirrors .mcp.json's per-server
// shape). It is defined HERE so internal/ecosys has no import cycle on
// internal/mcp; the MCP host converts. LoadUserMCPConfig returns
// map[string]ServerConfig.
type ServerConfig struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
	Cwd     string
}
