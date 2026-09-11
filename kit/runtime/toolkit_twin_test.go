package runtime //nolint:testpackage // internal package test

// toolkit_twin_test.go — the TEST TWIN of internal/acpserve/toolkit.go (the
// 25-05 engine_setup / 25-06 catalog twin precedent, at toolkit scale):
// white-box batteries in this package cannot import internal/acpserve (the
// test cycle — acpserve imports kit/runtime), so the twin mirrors the app
// adapter's construction VERBATIM-IN-ROLE. Production kit code stays
// app-import-free (T-25-34: test-file imports are the recorded 25-08
// exception with the strict-non-test gate scope). When the app adapter
// changes, this twin changes with it — the twin-difficulty is the price of
// white-box wiring batteries, paid three times before (engine_setup,
// catalog, now toolkit).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/perm"
	"github.com/Djarvur/ass-guard-agent/internal/sandbox"
	"github.com/Djarvur/ass-guard-agent/internal/tasks"
	"github.com/Djarvur/ass-guard-agent/kit/event"
	"github.com/Djarvur/ass-guard-agent/kit/provider"
	"github.com/Djarvur/ass-guard-agent/kit/session"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// errNoTwinSession is the twin launcher's missing-session degrade (the app
// adapter's errNoToolkitSession mirror).
var errNoTwinSession = errors.New("background subagent launcher: no toolkit session")

// toolkitTwin mirrors internal/acpserve's sessionToolkit: the registration
// family, the Reaper, the launcher, the tracker/PTY maps, and the perm
// authority — plus the observation accessors the batteries read.
type toolkitTwin struct {
	sandbox     *sandbox.Handle
	trackers    sync.Map // sessionID -> *tasks.Tracker
	ptyManagers sync.Map // sessionID -> *coreexec.PTYManager
}

// armToolkitTwin installs the twin family on r (the composition the serve
// root performs with the real adapter): the toolkit itself, the background
// launcher, the perm-store opener, and the ask-surface renderer. Returns
// the twin for observation.
func armToolkitTwin(r *Runner) *toolkitTwin {
	tw := &toolkitTwin{}
	r.toolkit = tw
	r.launchBackground = tw.LaunchBackground
	r.openPermStore = twinOpenPermStore
	r.askSurfaceRenderer = coreexec.RenderAskSurface

	return tw
}

// twinOf returns the runner's armed twin (the batteries' observation path —
// the concrete tracker/PTY handles live twin-side post-severance).
func twinOf(t *testing.T, r *Runner) *toolkitTwin {
	t.Helper()

	tw, ok := r.toolkit.(*toolkitTwin)
	if !ok {
		t.Fatal("runner not armed with the toolkit twin")
	}

	return tw
}

// concreteTracker returns the session's concrete tracker through the twin
// (the wake batteries' synthesis handle — r.trackerFor is the narrow
// peek/drain view the wake machinery consumes, by design).
func concreteTracker(t *testing.T, r *Runner, sid string) *tasks.Tracker {
	t.Helper()

	tr := twinOf(t, r).tracker(sid)
	if tr == nil {
		t.Fatalf("no tracker wired for session %s", sid)
	}

	return tr
}

// SetSandboxHandle mirrors the app toolkit's probe-step store.
func (tw *toolkitTwin) SetSandboxHandle(h *sandbox.Handle) { tw.sandbox = h }

// Attach mirrors the app toolkit's registration sequence (verbatim-in-role).
func (tw *toolkitTwin) Attach( //nolint:funlen,ireturn // mirrored registration sequence; the seam IS the interface
	catalog *toolcat.Catalog,
	//nolint:gocritic // hugeParam: the env-by-value signature is pinned by the kit seam
	env ToolkitEnv,
) (Reaper, error) {
	taskRegistry := coreexec.NewTaskRegistry()
	taskRegistry.Cap = env.BashCap

	sandboxNote := func(format string, args ...any) {
		_, _ = fmt.Fprintf(env.Stderr, format+"\n", args...)
	}

	taskRegistry.Sandbox = tw.sandbox
	taskRegistry.SandboxNote = sandboxNote

	ptyMgr := coreexec.NewPTYManager(coreexec.PTYOpts{
		WorkDir: env.Dir,
		NoteFn: func(format string, args ...any) {
			_, _ = fmt.Fprintf(env.Stderr, "ass-guard: session %s "+format+"\n",
				append([]any{env.SessionID}, args...)...)
		},
		Sandbox: tw.sandbox,
	})
	tw.ptyManagers.Store(env.SessionID, ptyMgr)

	tracker := tasks.NewTracker(tasks.TrackerOpts{SubagentCap: env.SubagentCap})
	tw.trackers.Store(env.SessionID, tracker)

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
		PTY: ptyMgr, Sandbox: tw.sandbox, SandboxNote: sandboxNote,
	})

	coreexec.RegisterAsk(catalog, env.Broker)

	mailbox := coreexec.NewAgentMailbox()
	sessionReader := coreexec.NewSessionReader(env.Dir)
	coreexec.RegisterInteractive(catalog, coreexec.InteractiveConfig{
		Ask: env.Broker, PlanMode: env.PlanMode,
		Mailbox: mailbox, Sessions: sessionReader, Tasks: taskRegistry,
		TaskStopFallback:   tracker.CancelTask,
		TaskOutputFallback: twinSubagentOutputFallback(env.Dir, tracker),
		Schedule:           twinCronStoreOrNil(env.Schedule),
	})

	if env.BindTracker != nil {
		env.BindTracker(twinTaskTrackerView{tr: tracker})
	}

	return &twinReaper{
		tw: tw, sessionID: env.SessionID, stderr: env.Stderr,
		taskRegistry: taskRegistry, ptyMgr: ptyMgr, tracker: tracker,
	}, nil
}

// twinReaper mirrors the app sessionReaper.
type twinReaper struct {
	tw           *toolkitTwin
	sessionID    string
	stderr       io.Writer
	taskRegistry *coreexec.TaskRegistry
	ptyMgr       *coreexec.PTYManager
	tracker      *tasks.Tracker
}

// Reap mirrors the app sessionReaper.Reap.
func (rp *twinReaper) Reap() {
	rp.taskRegistry.ReapAll() // 12-06: no background group outlives the session
	rp.ptyMgr.Drain()         // 22-04 (D-08/Pitfall 5)

	if dropped := rp.tracker.CancelQueued(); dropped > 0 {
		_, _ = fmt.Fprintf(rp.stderr,
			"ass-guard: session %s close dropped %d queued background subagent task(s)\n", rp.sessionID, dropped)
	}

	if cancelled := rp.tracker.CancelRunning(); cancelled > 0 {
		_, _ = fmt.Fprintf(rp.stderr,
			"ass-guard: session %s close cancelled %d running background subagent task(s)\n",
			rp.sessionID, cancelled)
	}

	rp.tw.trackers.Delete(rp.sessionID)
	rp.tw.ptyManagers.Delete(rp.sessionID)
}

// LaunchBackground mirrors the app toolkit's launcher.
func (tw *toolkitTwin) LaunchBackground(
	sessionID, workDir string,
	serveCtx func() context.Context,
	run func(ctx context.Context, progress func(text string)) (string, error),
) session.BackgroundDispatchResult {
	v, ok := tw.trackers.Load(sessionID)
	if !ok {
		return session.BackgroundDispatchResult{
			Err: fmt.Errorf("%w: %s", errNoTwinSession, sessionID),
		}
	}

	tracker, _ := v.(*tasks.Tracker)

	launch := tasks.RunBackgroundSubagent(tasks.SubagentDeps{
		Run: run, WorkDir: workDir, Tracker: tracker, ServeCtx: serveCtx,
	})

	return session.BackgroundDispatchResult{
		TaskID: launch.TaskID, OutputFile: launch.OutputFile,
		Queued: launch.Queued, Note: launch.Note, Err: launch.Err,
	}
}

// tracker returns the session's CONCRETE tracker (the batteries' synthesis
// handle — the kit-side trackerFor view is peek/drain only by design).
func (tw *toolkitTwin) tracker(sessionID string) *tasks.Tracker {
	v, ok := tw.trackers.Load(sessionID)
	if !ok {
		return nil
	}

	tr, _ := v.(*tasks.Tracker)

	return tr
}

// ptyManager returns the session's PTY manager (the close battery's
// observation handle — the pre-seam r.ptyManagers map's twin).
func (tw *toolkitTwin) ptyManager(sessionID string) *coreexec.PTYManager {
	v, ok := tw.ptyManagers.Load(sessionID)
	if !ok {
		return nil
	}

	mgr, _ := v.(*coreexec.PTYManager)

	return mgr
}

// twinTaskTrackerView mirrors the app taskTrackerView.
type twinTaskTrackerView struct{ tr *tasks.Tracker }

// PendingPeek satisfies TaskTracker.
func (v twinTaskTrackerView) PendingPeek() []TaskNotification {
	return twinNotifications(v.tr.PendingPeek())
}

// Drain satisfies TaskTracker.
func (v twinTaskTrackerView) Drain() []TaskNotification {
	return twinNotifications(v.tr.Drain())
}

// twinNotifications mirrors the app kitNotifications mapping.
func twinNotifications(ns []tasks.Notification) []TaskNotification {
	out := make([]TaskNotification, len(ns))
	for i := range ns {
		out[i] = TaskNotification{
			TaskID: ns[i].TaskID, Kind: string(ns[i].Kind), ExitStatus: ns[i].ExitStatus,
			Duration: ns[i].Duration, Tail: ns[i].Tail, OutputFile: ns[i].OutputFile,
		}
	}

	return out
}

// twinPermAuthority mirrors the app permAuthority (+ the kitRuleSet verdict
// mapping the app side owns post-severance).
type twinPermAuthority struct{ st *perm.Store }

// Rules satisfies PermAuthority.
//
//nolint:ireturn // the seam IS the interface (the kit RuleSet view)
func (a twinPermAuthority) Rules() session.RuleSet { return twinKitRuleSet{rs: a.st.Rules()} }

// AllowTool satisfies PermAuthority.
func (a twinPermAuthority) AllowTool(name string) error {
	return a.st.AllowTool(name) //nolint:wrapcheck // thin delegation
}

// ForbidTool satisfies PermAuthority.
func (a twinPermAuthority) ForbidTool(name string) error {
	return a.st.ForbidTool(name) //nolint:wrapcheck // thin delegation
}

// twinOpenPermStore mirrors the app openPermAuthority.
func twinOpenPermStore( //nolint:ireturn // the seam IS the interface (the runner's OpenPermStore field)
	path string,
) (PermAuthority, error) {
	st, err := perm.OpenRepaired(path)
	if err != nil {
		return nil, err //nolint:wrapcheck // the kit's log names the degrade verbatim
	}

	return twinPermAuthority{st: st}, nil
}

// twinKitRuleSet mirrors the app kitRuleSet (the verdict enum mapping).
type twinKitRuleSet struct{ rs perm.RuleSet }

// Evaluate mirrors the app kitRuleSet.Evaluate.
func (k twinKitRuleSet) Evaluate(toolName, primaryArg string) session.RuleVerdict {
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
		return session.RuleUnmatched
	}
}

// twinSubagentOutputFallback mirrors the app subagentOutputFallback.
func twinSubagentOutputFallback(
	dir string, tracker *tasks.Tracker,
) func(id string) (string, string, bool, bool) {
	return func(id string) (string, string, bool, bool) {
		if queued, running := tracker.SubagentState(id); queued || running {
			if queued {
				return "", "queued", true, true
			}

			return "", "running", true, true
		}

		content, ok := twinReadTail(
			filepath.Join(dir, ".ass-guard", "outputs", id+".log"), int64(64*1024))
		if !ok {
			return "", "", false, false
		}

		return content, "finished", false, true
	}
}

// twinReadTail mirrors the app readTail.
func twinReadTail(path string, budget int64) (string, bool) {
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

// twinCronStoreOrNil mirrors the app cronStoreOrNil.
func twinCronStoreOrNil( //nolint:ireturn // optional-capability extraction (the app mirror)
	s Scheduler,
) coreexec.CronStore {
	crud, _ := s.(coreexec.CronStore) // the comma-ok zero value IS the nil degrade

	return crud
}

// nilToolkitStubsExecution pins the nil-toolkit degraded path (25-07 Task
// 2's acceptance: from this commit on, a runner with NO toolkit takes the
// stub-executor arm — the exact pre-seam engine-off behavior — even with
// the engine ON): a Bash tool call resolves through enginebridge's canned
// stub result (no core registration, no panic, the turn completes). The
// 25-09 hostproof rides the same arm. Invoked as a SUBTEST of the coreexec
// wiring battery (the D-20 ledger counts ^func Test — helpers ride free).
func nilToolkitStubsExecution(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()

	prov := &scriptedACPProvider{}
	prov.queue(
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: "call_stub_1", Name: toolNameBash,
			Input: json.RawMessage(`{"command":"echo hi","description":"say hi"}`),
		}}},
		scriptedResp{text: chunkDone},
	)

	r := &Runner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      t.TempDir(),
		maxConc:      4,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}

	if err := r.SetupEngine(testEngineSetup(t)); err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	r.SetCatalog(newTestCatalog(r.workDirOrDefault()))

	if _, err := r.Run(context.Background(), "sess-niltoolkit", &noopEmitter{},
		[]session.ContentBlock{{Type: blockText, Text: "run echo"}}); err != nil {
		t.Fatalf("Run err: %v", err)
	}

	if !bashCallResolvedStubbed(t, r, "sess-niltoolkit") {
		t.Fatal("nil-toolkit Bash call did not resolve through the stub-executor arm (the documented degrade)")
	}
}

// bashCallResolvedStubbed scans the session transcript for a Bash tool_result
// carrying the canned stub output (the nil-toolkit arm's observable).
func bashCallResolvedStubbed(t *testing.T, r *Runner, sessionID string) bool {
	t.Helper()

	sess := r.sessions[sessionID]
	if sess == nil {
		t.Fatal("session not constructed")
	}

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	for i := range lines {
		if lines[i].Type != session.TypeToolResult {
			continue
		}

		for j := i - 1; j >= 0; j-- {
			if lines[j].Type == session.TypeToolCall && lines[j].ToolCallID == lines[i].ToolCallID {
				return lines[j].Name == toolNameBash &&
					strings.Contains(string(lines[i].Output), "stubbed")
			}
		}
	}

	return false
}
