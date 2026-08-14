package ecosys

// Shared constants (golangci goconst-friendly).
const (
	markdownExt      = ".md"
	frontmatterDelim = "---"
	claudeJSONFile   = ".claude.json"

	// File-permission constants (gosec G301/G306 compliance + mnd).
	dirPerms  = 0o750 // .ass-guard/ own dir — owner+group
	filePerms = 0o600 // .gitignore — owner-only
)
