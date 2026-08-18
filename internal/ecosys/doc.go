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
// # Precedence (D-06 + the 12-02 plugin tiers)
//
// Two-phase merge over the four existing roots, unambiguous:
//
//  1. Within each tree: project-scope overlays user-scope (project wins).
//  2. Across trees: `.claude/` overlays `.ass-guard/` (`.claude/` wins).
//
// 12-02 (operator revision 2026-08-19) appends the installed-plugin roots as
// the two lowest tiers, so the full chain is:
//
//	project-claude > {project-assguard, user-claude} > user-assguard
//	  > project `.claude/plugins/` > `~/.claude/plugins/`
//
// The existing chain keeps priority: user + project `.claude/skills/` and
// `.claude/commands/` (and `.claude/agents/`, 12-02) stay first-class chain
// entries that override every plugin contribution; the PROJECT plugin root
// overlays the USER plugin root (project over user, mirroring the `.claude/`
// ordering). Every same-key overwrite emits a stderr shadow warning naming
// both files (CMD-05).
//
// The `~/.zcode/cli/plugins/` root is deliberately NOT probed (operator
// 2026-08-19: plugins are installed FOR Claude Code and consumed as native —
// no popular kit ships an ass-guard target; the skills/agents listing merge
// is dynamic content inside the captured shape, so the request structure —
// the mimicry contract — is unaffected).
//
// Installed-plugin parsing accepts BOTH on-disk registry shapes: the v1 array
// and the v2 object (`{"version":2,"plugins":…}`, measured live). A missing,
// malformed, or escaping artifact degrades to a stderr warning naming the
// path — the registry keeps loading; discovery writes NOTHING anywhere (the
// runtime write-boundary test pins content AND mtimes under every probed
// root).
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
