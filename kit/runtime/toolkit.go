package runtime

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/Djarvur/ass-guard-agent/kit/session"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// errNoBackgroundLauncher is the nil-launcher degrade's structured error (the
// RunBackgroundSubagent writer-failure family's shape — a dispatch result
// carrying Err, never a panic, never a foreground fallback).
var errNoBackgroundLauncher = errors.New("background subagent launcher not configured")

// SessionToolkit is the core-tool-executor injection seam (25-07, OQ1's COARSE
// resolution): the app registers the per-session core tool executors — the
// Bash/Read/Write/Edit/Todo working set, the AskUserQuestion broker binding,
// and the interactive family (plan pair, messaging, background Task pair,
// cron quartet) — into the per-session tool catalog the kit hands it, and
// returns the Reaper that tears down everything the registration spawned.
//
// Core executor semantics are app domain (KIT-03: internal/coreexec stays
// app-side, including the locked 21-06 gate-join state and the 22-06 sandbox
// wraps at the three exec sites); the kit's only need is register-and-reap.
// The shape is deliberately ONE coarse method, not per-family methods
// (core/ask/interactive): finer methods would freeze coreexec's internal
// family structure into the public API against D-12. SPLIT-LATER TRIGGER:
// only a kit-only consumer needing PARTIAL toolkits (e.g. a host that wants
// the interactive family without Bash) justifies per-family methods — until
// such a consumer exists, coarse is the contract.
//
// Implemented by the app composition (internal/acpserve/toolkit.go — the
// reference app's adapter). A nil toolkit is the documented degraded state:
// no core tool registers, every session takes the engine-off stub-executor
// path (enginebridge.NewStubCatalogExec — tool calls surface the structured
// no-implementation error, turns still run; the 25-09 hostproof test rides
// this arm), and OnClose has no app-side reap leg. An Attach error degrades
// the SAME way, loudly (one stderr line) — a session is never refused over
// executor registration.
type SessionToolkit interface {
	// Attach registers the per-session core executors onto catalog (the
	// Execute-only override discipline — captured schemas are never
	// rewritten) using the per-session env, and returns the session's
	// Reaper. It is called exactly once per session, under the runner's
	// construction lock, before the session's first turn.
	Attach(catalog *toolcat.Catalog, env ToolkitEnv) (Reaper, error)
}

// ToolkitEnv carries the per-session inputs Attach consumes — exactly the
// inputs sessionFor passed to the direct coreexec construction this seam
// replaced, kit-typed only (no app type crosses; Pitfall 3). Field names
// mirror today's construction-site vocabulary.
type ToolkitEnv struct {
	// Dir is the session's working directory (WorkDir for every executor:
	// Bash cmd.Dir, absolute-path rendering, the tracker's output files).
	Dir string
	// SessionID is the ACP session identifier (note sinks, per-session
	// stores, the tracker map key on the app side).
	SessionID string
	// TranscriptPath is the session transcript's on-disk path (the hook
	// surface input; the hook runner itself is constructed kit-side from
	// the command catalog — the path rides for toolkits that stamp it).
	TranscriptPath string
	// SubagentCap and BashCap are the 22-02 D-12 caps resolved at session
	// construction (apply-as-landed): the background-subagent cap feeds the
	// task tracker, the bash cap the background registry.
	SubagentCap int
	BashCap     int
	// Stderr is the diagnostics sink the app-side note families write to
	// (sandbox notes, PTY restart notes, close-drop counts) — the same
	// serve stderr the runner logs to (transport discipline: never stdout).
	Stderr io.Writer
	// Hooks is the kit hook surface (25-06) the core executors' PostToolUse
	// observation leg consumes. nil = no hooks wired (no observation).
	Hooks session.Hooks
	// PlanMode is the session's plan-mode state (12-04): the interactive
	// family's Enter/Exit pair consults the same instance the session's
	// mutating-tool gate reads.
	PlanMode *session.PlanModeState
	// Broker is the per-session ask broker (12-01): the kit constructs it
	// (the suspension machinery is kit domain) and the toolkit's
	// RegisterAsk wiring binds the SAME broker the session resumes through.
	Broker *session.AskBroker
	// Schedule is the 25-05 cron port. The toolkit extracts the interactive
	// quartet's CRUD capability from it (the optional-capability view); nil
	// (test runners) is the quartet's structured no-store degrade.
	Schedule Scheduler
	// ScheduleWakeDrain signals the kit's wake machinery that task
	// notifications are pending (22-01: every completion schedules the
	// session's wake-drain chain). The toolkit binds it as the task
	// tracker's drain callback. nil = no wake wiring (bare test kits).
	ScheduleWakeDrain func()
	// BindTracker publishes the session's task-notification tracker view to
	// the kit's wake machinery (the r.trackers store — PendingPeek/Drain
	// are the wake drain chain's only consumptions). The toolkit calls it
	// exactly once per Attach with its adapter over the concrete tracker.
	// nil = the wake machinery never sees this session's notifications.
	BindTracker func(TaskTracker)
}

// Reaper is the session-teardown handle Attach returns (25-07): one coarse
// method that tears down EVERY background group the registration spawned —
// the background task registry's process groups (12-06: no background group
// outlives the session — THE invariant), the persistent shell's PTY group
// (22-04 D-08), and the queued/running background-subagent cancels (22-08
// G-22-4, with the counted stderr notes). OnClose composes it between the
// transcript-writer cancel and the MCP host close; Close runs the chain
// exactly once. A nil Reaper (nil-toolkit degrade) means nothing app-side
// was spawned — OnClose's reap leg is skipped, never panicked.
type Reaper interface {
	// Reap tears down the session's app-side background groups. Idempotent
	// by contract (Close runs OnClose exactly once; the components'
	// cancels are individually idempotent together).
	Reap()
}

// TaskNotification is one completed background task's notification (25-07:
// the promoted pure-data mirror of the app tracker's notification record —
// the kit/schedule precedent; the wake turn's <task-notification> block and
// the audit reason line render from exactly these fields). The concrete
// record stays app-side; the app toolkit's adapter maps it kit-ward.
type TaskNotification struct {
	TaskID     string
	Kind       string
	ExitStatus string
	Duration   time.Duration
	Tail       string
	OutputFile string
}

// TaskTracker is the wake machinery's view of the session's
// task-notification tracker (25-07): PendingPeek and Drain are the wake
// drain chain's only consumptions (22-01 D-01/D-03 — one coalesced batch
// per delivery, the tracker's atomic snapshot-and-clear the only destructive
// consumer). Implemented app-side by the toolkit's adapter over the concrete
// tracker; bound to the kit through ToolkitEnv.BindTracker. A missing
// tracker (session never Attach-ed) is terminal for the chain — it exits
// without draining (22-08: closed is closed).
type TaskTracker interface {
	// PendingPeek returns the pending notifications non-destructively
	// (ordered by completion time).
	PendingPeek() []TaskNotification
	// Drain consumes and returns the pending batch atomically.
	Drain() []TaskNotification
}

// BackgroundLauncher launches one background subagent run (25-07: the 22-03
// PAR-07 seam inverted app-side — tasks.RunBackgroundSubagent and the
// per-session tracker are app domain). The kit resolves the session's
// Session and builds the nested-loop Run closure (runSubagentWithProgress —
// kit domain); the launcher mints the task id, owns the output file, and
// enforces the subagent cap through the tracker it holds for sessionID.
// serveCtx is the serve-lifetime ctx source (the dispatching turn's ctx
// dies at return). Injected through RunnerConfig (the MakeProvider
// precedent); nil degrades to a structured-error dispatch result — never a
// panic, never a foreground fallback.
type BackgroundLauncher func(
	sessionID, workDir string,
	serveCtx func() context.Context,
	run func(ctx context.Context, progress func(text string)) (string, error),
) session.BackgroundDispatchResult

// PermAuthority is the runner-scoped permission-store view (25-07: the last
// perm edge — the app store, its open-repair, and the rule grammar stay
// app-side per KIT-03; the kit consumes the live rule snapshot the gate and
// the @-mention Read-rule consult share — 21-REVIEW WR-02/WR-03's ONE rule
// authority). Implemented app-side by the composition's adapter over
// *perm.Store (the kitRuleSet verdict mapping moved with it); injected
// through RunnerConfig's OpenPermStore opener. A nil authority (no opener,
// or a failed open) is the documented degrade: rule-less sessions — deny
// rules unenforced with the loud log, implicit allow preserved (the
// pre-21-06 shape), and the next session retries the open.
type PermAuthority interface {
	// Rules returns the live rule-set snapshot (the kit session.RuleSet
	// view — verdicts in the kit enum).
	Rules() session.RuleSet
	// AllowTool persists a dialog "always allow" click.
	AllowTool(name string) error
	// ForbidTool persists a dialog "reject, and don't ask again" click.
	ForbidTool(name string) error
}
