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
	//
	// ResumeTarget is the 18-06 CLI resume surface's resolved target (D-10):
	// `ass-guard --resume[ <id|name>]` / `--continue`/`-c` (and the inherited
	// forms under `acp serve`) resolve BEFORE Run and land the id here; Run
	// loads it through the SAME Server.LoadSession core session/load uses
	// (one engine, two entrypoints) — a load failure is a loud fatal, never a
	// silently fresh serve. Empty (the default) performs no initial load.
	//
	// SandboxMode is the 22-06 --sandbox flag's value (SAND-01): "off" (the
	// DEFAULT — the operator's locked choice) leaves every exec path
	// untouched; "on" resolves the startup sandbox probe (see
	// resolveSandboxAvailability). The CLI layer validates the {off,on}
	// vocabulary at startup (ValidateSandboxMode); the zero value "" behaves
	// as off here so bare Options constructions never confine anything.
	AuditLogPath string
	ResumeTarget string
	SandboxMode  string
}
