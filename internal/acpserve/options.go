package acpserve

import "time"

// Options carries the `acp serve` subcommand flags. The profile is loaded
// by name (default zcode); --max-concurrent bounds outbound provider concurrency
// (PARA-04, default 6). WorkDir is where .ass-guard/ transcripts live (default
// cwd). ConfigAddedBoundaries lets a project ADD boundaries (SESS-02).
// EngineEnabled (default true; --no-engine disables) wires the Phase-4 unified
// engine + hook-DAG + OpenSpec + learning (Plan 04-05). AskTimeout is the D-01
// AskUserQuestion wait (default 10m; 0 = block forever — interactive mode).
type Options struct {
	Profile               string
	MaxConcurrent         int
	ProfilesDir           string
	WorkDir               string
	ConfigAddedBoundaries []string
	EngineEnabled         bool
	AskTimeout            time.Duration

	// AuditLogPath is the --audit-log operator override (09-06): "" → the
	// default per-session mirror under <workdir>/.ass-guard/audit/; "-" →
	// stderr; a path → the single-file mirror (D-02).
	AuditLogPath string
}
