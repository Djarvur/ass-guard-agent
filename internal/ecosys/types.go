package ecosys

// Skill is one Claude-Code skill discovered from `<name>/SKILL.md` (D-07). The
// YAML frontmatter supplies Name/Description/AllowedTools; the body is the
// instruction text. Path is the on-disk SKILL.md location (read-only).
type Skill struct {
	Name         string
	Description  string
	AllowedTools []string
	Path         string
}

// Command is one slash-command discovered from `commands/<name>.md` (D-07). The
// filename stem is the Name; the frontmatter supplies Description; the body
// (carrying the `$ARGUMENTS` placeholder) is Body. Path is the on-disk location.
type Command struct {
	Name        string
	Description string
	Body        string
	Path        string
}

// Plugin is one plugin discovered from `plugins/<name>/manifest.json` (D-07).
// The manifest declares the plugin's contributed skill/command names. Path is
// the on-disk manifest location.
type Plugin struct {
	Name     string
	Skills   []string
	Commands []string
	Path     string
}

// Registry aggregates discovered skills/commands/plugins by name. Load returns
// the precedence-resolved registry; accessors (AllSkills/AllCommands/AllPlugins)
// return deterministically-ordered slices for the Shaper/Session to consume.
type Registry struct {
	Skills   map[string]Skill
	Commands map[string]Command
	Plugins  map[string]Plugin
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
