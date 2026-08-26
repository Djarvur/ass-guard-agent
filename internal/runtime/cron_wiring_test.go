package runtime //nolint:testpackage // internal package test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// The cron wiring battery (12-07 Task 2): queue-behind-active-turn, engine
// provenance, fire-once catch-up with the missed-window note, the scheduler
// goroutine's serve-lifetime lifecycle, and the server-driven-turn client
// mirror (WINDOWS #3). NO real sleeps beyond milliseconds: the store's clock
// is injected, the due walk is driven directly, and the queue semantics use
// the REAL per-session turn mutex.

// newCronRunner builds an engine-on runner over dir with a scripted provider
// (the expansion harness) plus a REAL schedule store.
//
//nolint:lll // signature carries the harness triple
func newCronRunner(t *testing.T, dir string, script ...scriptedResp) (*Runner, *scriptedACPProvider, *sched.ScheduleStore) {
	t.Helper()

	r, prov := newExpansionRunner(t, true, script...)

	// Repin the runner at the shared workDir (the store is PER-PROJECT).
	r.workDir = dir

	store, err := sched.Open(dir)
	if err != nil {
		t.Fatalf("sched.Open: %v", err)
	}

	r.schedule = store

	return r, prov, store
}

// TestCronWiring_QueueBehindActiveTurn (T2 Test 1): a due automation while a
// turn is active does NOT interrupt — the firing lands after the active turn
// completes, serialized through the same per-session mutex; the fired turn's
// user prompt is the automation's prompt.
func TestCronWiring_QueueBehindActiveTurn(t *testing.T) { //nolint:funlen,cyclop // flat queue-semantics battery
	t.Parallel()

	dir := t.TempDir()
	r, _, store := newCronRunner(t, dir,
		scriptedResp{text: "user turn done"},
		scriptedResp{text: "automation done"},
	)

	// The due automation (clock pinned before creation, so it is overdue).
	past := time.Now().Add(-2 * time.Hour)

	store.SetNow(func() time.Time { return past })

	a, err := store.Create(&sched.Automation{
		Title: "hourly check", Prompt: "run the hourly check", Cron: "0 * * * *", Recurring: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	store.SetNow(time.Now)

	// The project needs an ACTIVE session (the firing target) — created by
	// the first sessionFor, exactly as the first client turn would.
	_ = r.sessionFor(context.Background(), "sess-cron-q")

	// Hold the session's turn slot (an active turn)...
	mu := r.sessionTurnMu("sess-cron-q")
	mu.Lock()

	fired := make(chan struct{})

	go func() {
		defer close(fired)

		r.fireDueAutomations(context.Background())
	}()

	// The automation TURN must not run while the turn slot is held — whether
	// the explicit walk queues on the mutex OR the async catch-up pass (racing
	// since sessionFor) claimed the window and ITS firing queues: both are the
	// queue semantics. The transcript is the ordering lens: no automation
	// prompt may appear while the lock is held.
	time.Sleep(100 * time.Millisecond) // allow both paths to reach the queue point

	sess := r.sessionFor(context.Background(), "sess-cron-q")

	firedWhileActive := false

	lines, lerr := sess.Manager.ReadAll()
	if lerr == nil {
		for _, l := range lines {
			if l.Type == session.TypeUserMessage && strings.Contains(string(l.Content), "run the hourly check") {
				firedWhileActive = true
			}
		}
	}

	if firedWhileActive {
		t.Fatal("automation turn ran while a turn was active — must queue")
	}

	mu.Unlock()
	<-fired // the walk returns (its claim lost the race, or it queued + fired)

	// After the active turn completes the firing lands — poll for the prompt
	// (the queued catch-up firing may still be mid-turn).
	deadline := time.Now().Add(5 * time.Second)

	sawPrompt := false

	for time.Now().Before(deadline) && !sawPrompt {
		lines, err := sess.Manager.ReadAll()
		if err != nil {
			t.Fatal(err)
		}

		for _, l := range lines {
			if l.Type == session.TypeUserMessage && strings.Contains(string(l.Content), "run the hourly check") {
				sawPrompt = true

				break
			}
		}

		if !sawPrompt {
			time.Sleep(20 * time.Millisecond)
		}
	}

	if !sawPrompt {
		t.Error("the fired automation turn's user prompt is missing from the transcript")
	}

	// lastFired advanced (no duplicate fire on the next walk).
	if got := store.Due(time.Now()); len(got) != 0 {
		t.Errorf("Due after fire = %d automations, want 0 (lastFired advanced)", len(got))
	}

	_ = a
}

// TestCronWiring_AuditProvenance (T2 Test 2): every firing emits the
// EngineDecision line carrying automation provenance (the id named in both
// the signal and the reason).
func TestCronWiring_AuditProvenance(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r, _, store := newCronRunner(t, dir, scriptedResp{text: "ok"})

	store.SetNow(func() time.Time { return time.Now().Add(-2 * time.Hour) })

	a, err := store.Create(&sched.Automation{
		Title: "daily report", Prompt: "write the daily report", Cron: "0 9 * * *", Recurring: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Pin the clock 25h past creation: the daily 09:00 slot is missed for
	// EVERY wall-clock the test may run at (the -2h creation pin alone is
	// time-of-day dependent — the timing-sensitive-suite discipline).
	store.SetNow(func() time.Time { return time.Now().Add(25 * time.Hour) })

	// The active session must exist before the due walk (the firing target).
	// The async catch-up pass races the explicit due walk — EXACTLY ONE
	// firing wins (CatchUp marks lastFired before its events return; the
	// walk's Due snapshot then finds nothing), so poll for the ONE decision.
	_ = r.sessionFor(context.Background(), "sess-cron-a")

	r.fireDueAutomations(context.Background())

	sess := r.sessionFor(context.Background(), "sess-cron-a")

	deadline := time.Now().Add(5 * time.Second)
	sawDecision := false

	for time.Now().Before(deadline) && !sawDecision {
		lines, err := sess.Manager.ReadAll()
		if err != nil {
			t.Fatal(err)
		}

		for _, l := range lines {
			if l.Type == session.TypeEngineDecision &&
				strings.Contains(string(l.Input), a.ID) && strings.Contains(l.Text, a.ID) {
				sawDecision = true

				break
			}
		}

		if !sawDecision {
			time.Sleep(20 * time.Millisecond)
		}
	}

	if !sawDecision {
		t.Error("no EngineDecision line carrying the automation provenance (id in signal + reason)")
	}
}

// TestCronWiring_FireOnceCatchUp (T2 Test 3): with the clock past a missed
// window, the FIRST active session fires the missed automation EXACTLY once
// with the missed-window note in its prompt context; a second session does
// not fire it again (lastFired persisted).
func TestCronWiring_FireOnceCatchUp(t *testing.T) { //nolint:cyclop,funlen // flat restart-scenario battery
	t.Parallel()

	dir := t.TempDir()

	// The automation + its last fire live in the PERSISTED store (the process
	// "restart" is a fresh runner over the same workDir).
	store1, err := sched.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	store1.SetNow(func() time.Time { return time.Now().Add(-72 * time.Hour) })

	_, err = store1.Create(&sched.Automation{
		Title: "daily", Prompt: "do the daily thing", Cron: "0 9 * * *", Recurring: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Restart 1: the first active session catches up once, with the note.
	r1, _, _ := newCronRunner(t, dir, scriptedResp{text: "caught up"})
	r1.schedule.SetNow(time.Now)

	_, rerr := r1.Run(context.Background(), "sess-cron-c1", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "hello"}})
	if rerr != nil {
		t.Fatal(rerr)
	}

	// The async catch-up goroutine queues behind the just-finished turn.
	deadline := time.Now().Add(5 * time.Second)

	sawNote := false

	for time.Now().Before(deadline) {
		lines, lerr := r1.sessionFor(context.Background(), "sess-cron-c1").Manager.ReadAll()
		if lerr != nil {
			t.Fatal(lerr)
		}

		for _, l := range lines {
			if l.Type == session.TypeUserMessage && strings.Contains(string(l.Content), "missed its window") {
				sawNote = true
			}
		}

		if sawNote {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	if !sawNote {
		t.Fatal("the catch-up firing's missed-window note never landed in the prompt context")
	}

	// Restart 2: a fresh runner over the same workDir — NO second fire.
	r2, _, _ := newCronRunner(t, dir, scriptedResp{text: "plain"})
	r2.schedule.SetNow(time.Now)

	_, rerr2 := r2.Run(context.Background(), "sess-cron-c2", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "hello again"}})
	if rerr2 != nil {
		t.Fatal(rerr2)
	}

	time.Sleep(100 * time.Millisecond) // the async pass settles

	lines, lerr := r2.sessionFor(context.Background(), "sess-cron-c2").Manager.ReadAll()
	if lerr != nil {
		t.Fatal(lerr)
	}

	for _, l := range lines {
		if l.Type == session.TypeUserMessage && strings.Contains(string(l.Content), "missed its window") {
			t.Error("second restart re-fired the missed window — lastFired must make catch-up exactly-once")
		}
	}
}

// TestCronWiring_SchedulerGoroutineLifecycle (T2 Test 4): the scheduler is a
// goroutine parented on the serve-lifetime ctx — cancellation stops it
// (goroutine-exit asserted via the stop latch). No daemon, no port: nothing
// in this package listens anywhere (structural — no net.Listen call exists
// in cmd/ass-guard or internal/sched).
func TestCronWiring_SchedulerGoroutineLifecycle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r, _, _ := newCronRunner(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	r.startScheduler(ctx)

	cancel()

	stopped := make(chan struct{})

	go func() {
		r.schedStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler goroutine did not exit on ctx cancellation")
	}
}

// recordingEmitter captures AgentMessageChunk emissions (the WINDOWS #3
// mirror assertion lens).
type recordingEmitter struct {
	mu     sync.Mutex
	chunks []string
}

func (e *recordingEmitter) AgentMessageChunk(_, text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.chunks = append(e.chunks, text)

	return nil
}

func (e *recordingEmitter) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]string(nil), e.chunks...)
}

// TestCronWiring_ServerDrivenMirror (WINDOWS #3): with the emitter factory
// wired, a server-driven turn's chunks reach the client through the
// session-lifetime forwarder — no active Run subscription exists at fire
// time.
func TestCronWiring_ServerDrivenMirror(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r, _, store := newCronRunner(t, dir, scriptedResp{text: "scheduled work done"})

	em := &recordingEmitter{}
	r.emitFor = func(string) acp.ChunkEmitter { return em }

	store.SetNow(func() time.Time { return time.Now().Add(-2 * time.Hour) })

	_, err := store.Create(&sched.Automation{
		Title: "t", Prompt: "do it", Cron: "0 * * * *", Recurring: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	store.SetNow(time.Now)

	// The first sessionFor starts the session forwarder; the firing (no Run
	// in flight) mirrors through it.
	sess := r.sessionFor(context.Background(), "sess-cron-m")
	_ = sess

	r.fireDueAutomations(context.Background())

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if len(em.snapshot()) > 0 {
			return // mirrored
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("server-driven turn chunks never reached the client (WINDOWS #3 regression)")
}
