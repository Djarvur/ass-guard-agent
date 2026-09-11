package acpserve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/perm"
	"github.com/Djarvur/ass-guard-agent/internal/sandbox"
	"github.com/Djarvur/ass-guard-agent/internal/tasks"
	"github.com/Djarvur/ass-guard-agent/kit/runtime"
	"github.com/Djarvur/ass-guard-agent/kit/session"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// errNoToolkitSession is the launcher's missing-session degrade (the
// RunBackgroundSubagent writer-failure family's shape — a dispatch result
// carrying Err, never a panic, never a foreground fallback).
var errNoToolkitSession = errors.New("background subagent launcher: no toolkit session")

// sessionToolkit is the app-side SessionToolkit (25-07, OQ1 coarse): the
// reference app's implementation of the kit's core-executor injection seam.
// Attach relocates sessionFor's direct coreexec registration sequence
// VERBATIM-IN-ROLE — the task registry (+ the 22-02 cap), the 22-06 sandbox
// Handle at all three exec sites (registry launch, PTY spawn, foreground
// Config), the 22-01 task-notification tracker with its completion hook and
// wake-drain signal, RegisterCore, RegisterAsk (binding the env's broker),
// the agent mailbox + session reader, and RegisterInteractive (with the
// 22-09 fallbacks and the 12-07 cron store) — against the LOCKED landed
// state of 21-06 (gate-join at the executor chokepoint — hook verdicts
// consume at the session gate head, never here), 22-04 (the
// coretools.json-declared schemas the registration overrides), and 22-06
// (the sandbox wrap). This file builds NONE of those internals; it only
// registers against them.
//
// The struct carries the concrete app values the composition root injects:
// the sandbox Handle (SetSandboxHandle, resolved once at startup) and the
// per-session trackers/PTY managers the launcher and Reaper address by
// session id.
type sessionToolkit struct {
	// sandbox is the 22-06 (SAND-01) startup probe's resolved Handle: nil
	// or Mode "off" (the default) leaves every exec path untouched; Mode
	// "on" + Available wraps the three Bash-class exec sites.
	sandbox *sandbox.Handle

	// trackers maps sessionID -> the session's *tasks.Tracker (the concrete
	// notification subsystem the launcher and Reaper address). sync.Map:
	// Attach writes under the runner's construction lock while dispatches
	// read concurrently.
	trackers sync.Map // sessionID -> *tasks.Tracker
	// ptyManagers maps sessionID -> the session's *coreexec.PTYManager
	// (observation + teardown bookkeeping; the Reaper holds the live handle
	// in its closure).
	ptyManagers sync.Map // sessionID -> *coreexec.PTYManager
}

// SetSandboxHandle stores the 22-06 startup probe's resolved Handle
// (SAND-01): the serve composition resolves the availability ONCE (off
// short-circuits with zero probing; enabled-but-unavailable warns exactly
// once) before the scheduler starts; every Attach reads the Handle into
// coreexec.Config and PTYOpts (the askFire late-injection precedent — nil
// or Mode "off" leaves every exec path untouched, the locked default).
func (tk *sessionToolkit) SetSandboxHandle(h *sandbox.Handle) {
	tk.sandbox = h
}

// Attach performs the per-session registration family in today's exact
// order (the relocated sessionFor sequence): task registry, PTY manager,
// tracker (+ completion hook + wake-drain signal), RegisterCore,
// RegisterAsk, mailbox + session reader, RegisterInteractive. It binds the
// env's broker and plan-mode state (the SAME instances the kit session
// resumes through), extracts the cron quartet's CRUD capability from the
// schedule port, publishes the tracker's wake view back to the kit, and
// returns the Reaper OnClose composes. It cannot fail today (the sequence
// is construction-only); the error leg keeps the kit's degrade contract
// honest for future toolkits.
func (tk *sessionToolkit) Attach( //nolint:funlen,ireturn // relocated registration sequence; the seam IS the interface
	catalog *toolcat.Catalog,
	//nolint:gocritic // hugeParam: the env-by-value signature is pinned by the kit seam
	env runtime.ToolkitEnv,
) (runtime.Reaper, error) {
	taskRegistry := coreexec.NewTaskRegistry()
	taskRegistry.Cap = env.BashCap

	// 22-06 (SAND-01): the startup probe's resolved Handle + the note sink
	// reach the registry (the background site) — one composition, all three
	// exec sites. nil / Mode off (the default) leaves every launch untouched.
	sandboxNote := func(format string, args ...any) {
		_, _ = fmt.Fprintf(env.Stderr, format+"\n", args...)
	}

	taskRegistry.Sandbox = tk.sandbox
	taskRegistry.SandboxNote = sandboxNote

	// 22-04 (PAR-09/D-07/D-08): the session's ONE persistent-shell PTY
	// manager — lazily started (no shell exists until the first persistent
	// Bash call), shared by every persistent call of the session, and
	// drained on close (the Reaper link). The dead-shell restart note rides
	// the loud stderr family (one line per restart — D-08's visible
	// state-loss acknowledgment, never silent). 22-06 (SAND-01): the SAME
	// Handle confines the persistent shell at spawn (the third exec site;
	// D-09 orthogonality — persistence and confinement compose).
	ptyMgr := coreexec.NewPTYManager(coreexec.PTYOpts{
		WorkDir: env.Dir,
		NoteFn: func(format string, args ...any) {
			_, _ = fmt.Fprintf(env.Stderr, "ass-guard: session %s "+format+"\n",
				append([]any{env.SessionID}, args...)...)
		},
		Sandbox: tk.sandbox,
	})
	tk.ptyManagers.Store(env.SessionID, ptyMgr)

	// 22-01 (D-01..D-03, PAR-07/PAR-08): the ONE task-notification tracker
	// beside the registry. Registry completions land as kind-tagged
	// Notifications (primitive-arg CompletionHook — no coreexec→tasks
	// import; this adapter owns the mapping); every completion schedules the
	// session's wake-drain chain; the Reaper drops queued-but-unstarted
	// subagent registrations with a counted note (OQ5).
	tracker := tasks.NewTracker(tasks.TrackerOpts{SubagentCap: env.SubagentCap})
	tk.trackers.Store(env.SessionID, tracker)

	taskRegistry.CompletionHook =
		func(taskID, kind, exitStatus string, duration time.Duration, tail, outputFile string) {
			tracker.Complete(tasks.Notification{
				TaskID: taskID, Kind: tasks.Kind(kind), ExitStatus: exitStatus,
				Duration: duration, Tail: tail, OutputFile: outputFile,
			})
		}

	if env.ScheduleWakeDrain != nil {
		tracker.SetDrain(func(pending []tasks.Notification) {
			_ = pending // peek only — the chain re-reads authoritatively via Drain

			env.ScheduleWakeDrain()
		})
	}

	coreexec.RegisterCore(catalog, coreexec.Config{
		WorkDir: env.Dir, Todos: coreexec.NewTodoStore(), Hooks: env.Hooks, Tasks: taskRegistry,
		PTY: ptyMgr, // 22-04 (PAR-09): the session's persistent shell
		// 22-06 (SAND-01): the foreground site's Handle + note sink — the
		// same resolved pair the registry and the PTY manager carry (one
		// composition, all three exec sites; nil/off = untouched default).
		Sandbox: tk.sandbox, SandboxNote: sandboxNote,
	})

	coreexec.RegisterAsk(catalog, env.Broker)

	mailbox := coreexec.NewAgentMailbox()
	sessionReader := coreexec.NewSessionReader(env.Dir)
	coreexec.RegisterInteractive(catalog, coreexec.InteractiveConfig{
		Ask: env.Broker, PlanMode: env.PlanMode,
		Mailbox: mailbox, Sessions: sessionReader, Tasks: taskRegistry,
		// 22-09 (G-22-5): ids the registry does not know (background-
		// subagent exec_ ids) reach THIS session's tracker. TaskStop cancels
		// through CancelTask — a finished id declines (its cancel entry
		// retired at Complete, 22-07), so no fake ack for a dead id.
		TaskStopFallback: tracker.CancelTask,
		// TaskOutput classifies through SubagentState and reads the output
		// file (bounded tail; stat-miss = not-handled). Primitive args only
		// for both seams (the CompletionHook precedent — coreexec stays
		// tasks-free).
		TaskOutputFallback: subagentOutputFallback(env.Dir, tracker),
		// 12-07: the PER-PROJECT cron store (nil in test runners →
		// structured no-store errors)
		Schedule: cronStoreOrNil(env.Schedule),
	})

	// The wake view crosses kit-ward (the kit's drain chain consumes
	// PendingPeek/Drain only — the narrow interface by design).
	if env.BindTracker != nil {
		env.BindTracker(taskTrackerView{tr: tracker})
	}

	return &sessionReaper{
		tk:           tk,
		sessionID:    env.SessionID,
		taskRegistry: taskRegistry,
		ptyMgr:       ptyMgr,
		tracker:      tracker,
		stderr:       env.Stderr,
	}, nil
}

// sessionReaper is the app-side Reaper (25-07): OnClose's app-side leg, in
// today's exact order — the 12-06 background-group reap, the 22-04 PTY
// drain, the 22-01 OQ5 queued-drop count, and the 22-08 running-cancel
// count — then the bookkeeping deletes (a closed session's tracker/PTY
// entries must not linger in the toolkit's maps). The 12-06 invariant (no
// background group outlives the session) holds through this handle.
type sessionReaper struct {
	tk           *sessionToolkit
	sessionID    string
	taskRegistry *coreexec.TaskRegistry
	ptyMgr       *coreexec.PTYManager
	tracker      *tasks.Tracker
	stderr       io.Writer
}

// Reap tears down the session's app-side background groups (idempotent:
// Close runs OnClose exactly once; the component cancels are individually
// idempotent together).
func (rp *sessionReaper) Reap() {
	rp.taskRegistry.ReapAll() // 12-06: no background group outlives the session
	rp.ptyMgr.Drain()         // 22-04 (D-08/Pitfall 5): the persistent shell's group dies here too

	// 22-01 (OQ5): queued-but-unstarted subagent registrations die here
	// — silently dropped (nothing started, nothing to kill), the count
	// noted on stderr only when non-zero.
	if dropped := rp.tracker.CancelQueued(); dropped > 0 {
		_, _ = fmt.Fprintf(rp.stderr,
			"ass-guard: session %s close dropped %d queued background subagent task(s)\n", rp.sessionID, dropped)
	}

	// 22-08 (G-22-4, CR-04): RUNNING background subagents die with the
	// session too — their cancel funcs fire here (queued ones above; the
	// two legs are idempotent together), the count noted on stderr only
	// when non-zero. Without this a running subagent outlived its session
	// and its late completion fired into closed machinery.
	if cancelled := rp.tracker.CancelRunning(); cancelled > 0 {
		_, _ = fmt.Fprintf(rp.stderr,
			"ass-guard: session %s close cancelled %d running background subagent task(s)\n",
			rp.sessionID, cancelled)
	}

	rp.tk.trackers.Delete(rp.sessionID)
	rp.tk.ptyManagers.Delete(rp.sessionID)
}

// LaunchBackground runs one background subagent (the 22-03 PAR-07 seam,
// 25-07-inverted app-side): the kit resolved the session + built the nested
// loop; this mints the task id, owns the output file, and enforces the
// subagent cap through the tracker Attach stored for sessionID. The serve
// ctx is the kit's serve-lifetime source (the dispatching turn's ctx dies
// at return). Satisfies runtime.BackgroundLauncher.
func (tk *sessionToolkit) LaunchBackground(
	sessionID, workDir string,
	serveCtx func() context.Context,
	run func(ctx context.Context, progress func(text string)) (string, error),
) session.BackgroundDispatchResult {
	v, ok := tk.trackers.Load(sessionID)
	if !ok {
		return session.BackgroundDispatchResult{
			Err: fmt.Errorf("%w: %s", errNoToolkitSession, sessionID),
		}
	}

	tracker, _ := v.(*tasks.Tracker)

	launch := tasks.RunBackgroundSubagent(tasks.SubagentDeps{
		Run:      run,
		WorkDir:  workDir,
		Tracker:  tracker,
		ServeCtx: serveCtx,
	})

	return session.BackgroundDispatchResult{
		TaskID: launch.TaskID, OutputFile: launch.OutputFile,
		Queued: launch.Queued, Note: launch.Note, Err: launch.Err,
	}
}

// --- the wake view + the perm authority (the app→kit adapters) ---

// taskTrackerView adapts the concrete *tasks.Tracker to the kit's narrow
// wake view (PendingPeek/Drain over the kit notification mirror — the
// kit/schedule promoted-pure-data precedent).
type taskTrackerView struct{ tr *tasks.Tracker }

// PendingPeek satisfies runtime.TaskTracker.
func (v taskTrackerView) PendingPeek() []runtime.TaskNotification {
	return kitNotifications(v.tr.PendingPeek())
}

// Drain satisfies runtime.TaskTracker.
func (v taskTrackerView) Drain() []runtime.TaskNotification {
	return kitNotifications(v.tr.Drain())
}

// kitNotifications maps the app notification records onto the kit mirror
// (field-for-field; Kind widens to the kit's string vocabulary).
func kitNotifications(ns []tasks.Notification) []runtime.TaskNotification {
	out := make([]runtime.TaskNotification, len(ns))
	for i := range ns {
		out[i] = runtime.TaskNotification{
			TaskID: ns[i].TaskID, Kind: string(ns[i].Kind), ExitStatus: ns[i].ExitStatus,
			Duration: ns[i].Duration, Tail: ns[i].Tail, OutputFile: ns[i].OutputFile,
		}
	}

	return out
}

// permAuthority adapts *perm.Store to the kit's runner-scoped PermAuthority
// view (25-07: the last perm edge). The verdict mapping is the kitRuleSet
// adapter 25-06 staged kit-side — its documented permanent home was always
// the app side of the seam.
type permAuthority struct{ st *perm.Store }

// Rules returns the live rule-set snapshot in the kit verdict vocabulary.
//
//nolint:ireturn // the seam IS the interface (the kit RuleSet view)
func (a permAuthority) Rules() session.RuleSet { return kitRuleSet{rs: a.st.Rules()} }

// AllowTool persists a dialog "always allow" click.
func (a permAuthority) AllowTool(name string) error {
	return a.st.AllowTool(name) //nolint:wrapcheck // thin delegation
}

// ForbidTool persists a dialog "reject, and don't ask again" click.
func (a permAuthority) ForbidTool(name string) error {
	return a.st.ForbidTool(name) //nolint:wrapcheck // thin delegation
}

// openPermAuthority opens the permission store at the path the kit composes
// (the .ass-guard floor), keeping OpenRepaired's quarantine-on-corrupt
// fail-safe semantics (WR-01) app-side. Satisfies RunnerConfig.OpenPermStore.
func openPermAuthority( //nolint:ireturn // the seam IS the interface (the runner's OpenPermStore field)
	path string,
) (runtime.PermAuthority, error) {
	st, err := perm.OpenRepaired(path)
	if err != nil {
		return nil, err //nolint:wrapcheck // the kit's log names the degrade verbatim
	}

	return permAuthority{st: st}, nil
}

// kitRuleSet adapts the app permission rule set to the kit session.RuleSet
// view (the 25-03-assigned session→perm severance's composition-side half —
// moved app-side at the 25-07 perm severance, its documented destination).
type kitRuleSet struct{ rs perm.RuleSet }

// Evaluate maps the app verdict onto the kit enum (explicit switch — the
// enum orders are deliberately not assumed to coincide).
func (k kitRuleSet) Evaluate(toolName, primaryArg string) session.RuleVerdict {
	switch k.rs.Evaluate(toolName, primaryArg) {
	case perm.Unmatched:
		return session.RuleUnmatched
	case perm.VerdictAllow:
		return session.RuleAllow
	case perm.VerdictAsk:
		return session.RuleAsk
	case perm.VerdictDeny:
		return session.RuleDeny
	default:
		return session.RuleUnmatched // an unknown app verdict stays unmatched
	}
}

// --- the relocated pure helpers (verbatim moves from kit/runtime) ---

// subagentOutputTailBytes bounds the finished-subagent output read (the last
// 64 KiB of the progressive output file — the D-02 tail discipline's
// TaskOutput analogue; the full file stays a Read away, exactly as the
// notification's OutputFile pointer promises).
const subagentOutputTailBytes = 64 * 1024

// subagentOutputFallback is the 22-09 (G-22-5) TaskOutputFallback binding:
// classify the id against the session's tracker — queued/running render the
// not_ready family with no output — and anything the tracker no longer knows
// as live is treated as FINISHED: the subagent's output file (the production
// path convention .ass-guard/outputs/<id>.log under the session workdir) is
// read as a bounded TAIL and surfaces through the ready envelope. A stat miss
// reports not-handled: the id is nobody's (never registered, or the file is
// gone) and TaskOutput renders the structured unknown-task error.
// block/timeout are ignored for these ids — the CURRENT state renders
// immediately (the coreexec stub comment's documented choice; the tracker
// seam carries no bounded-wait machinery).
func subagentOutputFallback(dir string, tracker *tasks.Tracker) func(id string) (string, string, bool, bool) {
	return func(id string) (string, string, bool, bool) {
		if queued, running := tracker.SubagentState(id); queued || running {
			if queued {
				return "", "queued", true, true
			}

			return "", "running", true, true
		}

		content, ok := readTail(filepath.Join(dir, ".ass-guard", "outputs", id+".log"), subagentOutputTailBytes)
		if !ok {
			return "", "", false, false
		}

		return content, "finished", false, true
	}
}

// readTail returns the last budget bytes of path (a bounded read — the file
// is never loaded whole). ok=false on any miss (absent or unreadable: the
// caller reports not-handled either way).
func readTail(path string, budget int64) (string, bool) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return "", false
	}

	f, err := os.Open(path)
	if err != nil {
		return "", false
	}

	defer func() { _ = f.Close() }()

	offset := int64(0)
	if st.Size() > budget {
		offset = st.Size() - budget
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", false
	}

	data, err := io.ReadAll(io.LimitReader(f, budget))
	if err != nil {
		return "", false
	}

	return string(data), true
}

// cronStoreOrNil extracts the interactive cron quartet's CRUD capability
// from the schedule port value (25-05 Task 1; 25-07 moved app-side wholesale
// — the last coreexec-touching kit helper): the concrete store satisfies
// BOTH the four-method port and coreexec.CronStore structurally, so the
// per-session toolkit registration keeps receiving the full CRUD surface
// through one injected value. A nil port (test runners) yields a nil
// CronStore — the quartet's structured no-store errors, exactly today's
// semantics.
func cronStoreOrNil( //nolint:ireturn // optional-capability extraction (the acp.SessionCloser pattern)
	s runtime.Scheduler,
) coreexec.CronStore {
	crud, _ := s.(coreexec.CronStore) // the comma-ok zero value IS the nil degrade

	return crud
}
