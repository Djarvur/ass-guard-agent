// Package version carries the build-time-injected version string. The default
// "dev" is overridden by goreleaser ldflags
// (-X github.com/Djarvur/ass-guard-agent/internal/version.Version=<v>) at
// release time. String is the single accessor used by the --version flag.
//
// The module path in go.mod is capital-D `github.com/Djarvur/ass-guard-agent`
// (STATE.md deviation log); the ldflags -X target MUST match that casing — a
// lowercase github.com/djarvur/... target would silently fail to inject.
package version

// Version is the build version. Defaults to "dev" for a plain `go build`; set
// via ldflags at release (goreleaser). Informational only — the released
// archives are the authoritative artifacts (sha256-pinned in agent.json).
var Version = "dev"

// String returns the version string. Trivial accessor today, but keeps the
// call site stable if Version later derives from build info (vcs.revision).
func String() string { return Version }
