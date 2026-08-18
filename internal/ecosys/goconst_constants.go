package ecosys

// Shared constants (golangci goconst-friendly).
const (
	markdownExt      = ".md"
	frontmatterDelim = "---"
	claudeJSONFile   = ".claude.json"

	// Installed-plugin layout names (12-02, ECOSYSTEM-AUDIT §4.1 PLUG-01).
	// Forward-slash literals: the supported platforms (macOS + Linux) agree.
	pluginsDirName        = "plugins"
	installedPluginsFile  = "installed_plugins.json"
	pluginManifestRelPath = ".claude-plugin/plugin.json"

	// File-permission constants (gosec G301/G306 compliance + mnd).
	dirPerms  = 0o750 // .ass-guard/ own dir — owner+group
	filePerms = 0o600 // .gitignore — owner-only
)
