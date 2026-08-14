// Package ecosys discovers Claude-Code ecosystem extensions (skills, slash-
// commands, plugins) from the `.claude/` directory layout and merges ass-guard's
// own additions from `.ass-guard/` (ECOS-04/05).
//
// # The read-only `.claude/` contract (D-05/D-06)
//
// ass-guard loads the user's existing Claude Code setup from `.claude/`
// (project-scope) and `~/.claude/` (user-scope) READ-ONLY. The package NEVER
// writes to any `.claude/` path: every read goes through os.ReadFile/O_RDONLY,
// and the sole write API (EnsureGitignore) is parametrized to `.ass-guard/` and
// REJECTS `.claude/` paths. A source-level + runtime test enforces this
// (T-5-08 clobber mitigation).
//
// ass-guard's own additions live under `.ass-guard/` (project) and
// `~/.ass-guard/` (user), covered by the existing self-gitignore (Phase 2
// D-07).
//
// # Precedence (D-06)
//
// Two-phase merge, unambiguous:
//
//  1. Within each tree: project-scope overlays user-scope (project wins).
//  2. Across trees: `.claude/` overlays `.ass-guard/` (`.claude/` wins).
//
// Final: project-claude > {project-assguard, user-claude} > user-assguard.
//
// # No viper convention
//
// Like internal/profile, this package uses encoding/json for `.claude/`-compat
// JSON (manifests, ~/.claude.json) and gopkg.in/yaml.v3 for SKILL.md / command
// frontmatter. Never viper.
//
// # No import cycle on internal/mcp
//
// LoadUserMCPConfig returns a neutral map[string]ServerConfig defined HERE; the
// MCP host (internal/mcp) consumes it. internal/ecosys does NOT import
// internal/mcp.
package ecosys
