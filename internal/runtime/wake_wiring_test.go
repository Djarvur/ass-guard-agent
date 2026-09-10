package runtime //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"encoding/json"
	goruntime "runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/tasks"
)

// The wake-turn wiring battery (22-01, D-01/D-02/D-03): a background Bash
// completion reaches the ONE task-notification subsystem and WAKES the model
// through the existing automation-turn rails — exactly one wake turn per
// drain batch, kind-detected payload fields, wake provenance on the
// EngineDecision line, and a busy client turn never interrupted (the pending
// notifications coalesce and deliver after the turn).

// wakeMarker is the background command's output marker the notification tail
// must carry end-to-end.
const wakeMarker = "wake-marker-7f3a"

// wakeUserTexts assembles every user_message line's block texts (the
// assertion lens for turn inputs — the wake block list lands here).
func wakeUserTexts(t *testing.T, sess *session.Session) []string {
	t.Helper()

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var out []string

	for i := range lines {
		if lines[i].Type != session.TypeUserMessage {
			continue
		}

		var blocks []session.ContentBlock

		if uerr := json.Unmarshal(lines[i].Content, &blocks); uerr != nil {
			continue // non-block content (tolerant — other lines may differ)
		}

		var sb strings.Builder

		for _, b := range blocks {
			sb.WriteString(b.Text)
		}

		out = append(out, sb.String())
	}

	return out
}

// wakeEngineDecisions returns the EngineDecision lines whose signal/text
// mention the given provenance string.
func wakeEngineDecisions(t *testing.T, sess *session.Session, provenance string) int {
	t.Helper()

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	n := 0

	for i := range lines {
		if lines[i].Type != session.TypeEngineDecision {
			continue
		}

		if strings.Contains(string(lines[i].Input), provenance) || strings.Contains(lines[i].Text, provenance) {
			n++
		}
	}

	return n
}

// waitFor polls cond until true or the deadline passes (returns the final
// cond value — the caller reports which assertion failed).
func waitFor(deadline time.Duration, cond func() bool) bool {
	end := time.Now().Add(deadline)

	for time.Now().Before(end) {
		if cond() {
			return true
		}

		time.Sleep(20 * time.Millisecond)
	}

	return cond()
}

// TestWakeTurn_BackgroundBashCompletion (22-01 Task 1 tracer): a client turn
// starts a run_in_background Bash task; after the child exits 0 exactly ONE
// additional model turn fires whose input block lists the D-02 fields (exec_
// id, kind bash, exit status 0, .ass-guard/outputs/ pointer, tail with the
// marker), an EngineDecision line carries the wake provenance, and the
// foreground scripted exchanges around it are unchanged.
func TestWakeTurn_BackgroundBashCompletion(t *testing.T) { //nolint:funlen,cyclop // flat tracer battery
	t.Parallel()

	r, prov := newExpansionRunner(t, true,
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID:   "call_bg_1",
			Name: "Bash",
			Input: json.RawMessage(
				`{"command":"echo ` + wakeMarker + `; exit 0","run_in_background":true}`),
		}}},
		scriptedResp{text: "background task launched"},
		scriptedResp{text: "wake acknowledged"},
	)

	// The CLIENT turn: starts the background task and returns immediately.
	_, err := r.Run(context.Background(), "sess-wake-1", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "run a background task"}})
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}

	sess := r.sessions["sess-wake-1"]

	// (1) The dispatch returned immediately with the background-start prose.
	var toolResults strings.Builder

	lines, lerr := sess.Manager.ReadAll()
	if lerr != nil {
		t.Fatalf("ReadAll: %v", lerr)
	}

	for i := range lines {
		if lines[i].Type == session.TypeToolResult {
			toolResults.Write(lines[i].Output)
		}
	}

	if !strings.Contains(toolResults.String(), "Command running in background with ID: exec_") {
		t.Error("no background-start prose form in the transcript tool results")
	}

	// (2) After the child exits: exactly ONE wake turn with the D-02 fields.
	wakeOK := waitFor(5*time.Second, func() bool {
		for _, txt := range wakeUserTexts(t, sess) {
			if strings.Contains(txt, "task-notification") &&
				strings.Contains(txt, "kind: bash") &&
				strings.Contains(txt, "exec_") &&
				strings.Contains(txt, ".ass-guard/outputs/") &&
				strings.Contains(txt, wakeMarker) {
				return true
			}
		}

		return false
	})

	if !wakeOK {
		t.Error("no wake-turn user block carrying the D-02 notification fields")
	}

	// Exactly ONE wake turn — poll stable (no second batch may fire later).
	time.Sleep(150 * time.Millisecond)

	wakeTurns := 0

	for _, txt := range wakeUserTexts(t, sess) {
		if strings.Contains(txt, "task-notification") {
			wakeTurns++
		}
	}

	if wakeTurns != 1 {
		t.Errorf("wake turns = %d; want exactly 1 (one coalesced batch)", wakeTurns)
	}

	// (3) The wake turn's EngineDecision line carries the wake provenance.
	if got := wakeEngineDecisions(t, sess, "wake:tasks"); got != 1 {
		t.Errorf("EngineDecision lines with wake provenance = %d; want 1", got)
	}

	// (4) The foreground scripted turns are unchanged: 2 client-turn calls +
	// exactly 1 wake call — no storm, no duplicates.
	if calls := prov.callCount(); calls != 3 {
		t.Errorf("provider calls = %d; want 3 (client tool turn + client final + one wake)", calls)
	}

	// The FIRST user message stays the typed prompt (the wake turn appends,
	// never replaces).
	if first := wakeUserTexts(t, sess)[0]; !strings.Contains(first, "run a background task") {
		t.Errorf("first user message = %q; want the typed client prompt", first)
	}
}

// TestWakeTurn_BusyClientTurnAccumulates (22-01 Task 2, D-01 fallback +
// D-03 at the runner level): completions landing while the session's turn
// slot is held leave the notifications pending (zero consumed, zero wake
// turns while busy); after the mutex frees, ONE later batch carries every
// accumulated notification in completion-time order. The client turn is
// never interrupted.
func TestWakeTurn_BusyClientTurnAccumulates(t *testing.T) { //nolint:funlen,cyclop // flat fallback battery
	t.Parallel()

	r, prov := newExpansionRunner(t, true,
		scriptedResp{text: "wake batch acknowledged"},
	)

	_ = r.sessionFor(context.Background(), "sess-wake-busy")

	sess := r.sessions["sess-wake-busy"]

	tr := r.trackerFor("sess-wake-busy")
	if tr == nil {
		t.Fatal("no tracker wired for the session")
	}

	// Hold the session's turn slot (a client turn is active).
	mu := r.sessionTurnMu("sess-wake-busy")
	mu.Lock()

	// Completions during the busy window — they must coalesce, not deliver.
	tr.Complete(tasks.Notification{TaskID: "exec_busy_a", Kind: tasks.KindBash, ExitStatus: "0", Tail: "busy-a marker"})
	tr.Complete(tasks.Notification{TaskID: "exec_busy_b", Kind: tasks.KindBash, ExitStatus: "0", Tail: "busy-b marker"})

	time.Sleep(300 * time.Millisecond) // the retry chain runs and declines

	if calls := prov.callCount(); calls != 0 {
		t.Errorf("provider calls during busy window = %d; want 0 (never interrupt)", calls)
	}

	for _, txt := range wakeUserTexts(t, sess) {
		if strings.Contains(txt, "task-notification") {
			t.Fatal("wake turn fired while the turn slot was held — must stay pending")
		}
	}

	if peek := tr.PendingPeek(); len(peek) != 2 {
		t.Errorf("pending peek = %d; want 2 (coalesced while waiting)", len(peek))
	}

	// The busy window ends: the retry chain delivers ONE batch in order.
	mu.Unlock()

	batched := waitFor(5*time.Second, func() bool {
		for _, txt := range wakeUserTexts(t, sess) {
			if strings.Contains(txt, "busy-a marker") && strings.Contains(txt, "busy-b marker") {
				return true
			}
		}

		return false
	})

	if !batched {
		t.Fatal("no single wake batch carrying both accumulated notifications")
	}

	wakeTurns := 0

	lastWake := ""

	for _, txt := range wakeUserTexts(t, sess) {
		if strings.Contains(txt, "task-notification") {
			wakeTurns++
			lastWake = txt
		}
	}

	if wakeTurns != 1 {
		t.Errorf("wake turns = %d; want exactly 1 (one coalesced post-busy batch)", wakeTurns)
	}

	if a, b := strings.Index(lastWake, "busy-a marker"), strings.Index(lastWake, "busy-b marker"); a > b {
		t.Error("wake batch not in completion-time order (a must precede b)")
	}

	if calls := prov.callCount(); calls != 1 {
		t.Errorf("provider calls after busy window = %d; want exactly 1 (one batch)", calls)
	}
}

// TestWakeTurn_EmptyPendingNoTurn (22-01 Task 2, PAR-07 empty probe at the
// runner level): a drain attempt on an empty pending queue makes NO model
// call and writes NO EngineDecision line.
func TestWakeTurn_EmptyPendingNoTurn(t *testing.T) {
	t.Parallel()

	r, prov := newExpansionRunner(t, true)

	_ = r.sessionFor(context.Background(), "sess-wake-empty")

	// Direct drain attempt with nothing pending.
	r.scheduleWakeDrain("sess-wake-empty")

	time.Sleep(200 * time.Millisecond) // the chain runs, finds nothing, exits

	if calls := prov.callCount(); calls != 0 {
		t.Errorf("provider calls = %d; want 0 (empty pending → no wake)", calls)
	}

	sess := r.sessions["sess-wake-empty"]

	if got := wakeEngineDecisions(t, sess, wakeProvenance); got != 0 {
		t.Errorf("wake EngineDecision lines = %d; want 0 (nothing to deliver)", got)
	}
}

// --- 22-08 (G-22-2, CR-02): the wake-chain lifecycle battery -----------------
//
// NONE of the TestWakeChain_* rows calls t.Parallel(): each asserts goroutine
// DELTA BANDS against a per-test baseline, and the package's parallel
// batteries (TestWakeTurn_* at :110/:209/:293) resuming mid-window would break
// the band. Go runs parallel tests only alongside other parallel tests, so
// these serial rows never overlap them — the mixed set is proven stable by
// running -run 'TestWakeChain|TestWakeTurn' with -count=3.

// wakeFlag returns the session's wakeInFlight flag (creating the entry if the
// session never saw a drain attempt — LoadOrStore is scheduleWakeDrain's own
// idempotent seam).
func wakeFlag(r *Runner, sessionID string) *atomic.Bool {
	v, _ := r.wakeInFlight.LoadOrStore(sessionID, &atomic.Bool{})

	return v.(*atomic.Bool) //nolint:forcetypeassert // LoadOrStore stores exactly *atomic.Bool
}

// sampleFlagFalse samples the flag every 20ms across the window and reports
// whether EVERY sample read false (plus the sample count, for the >=25-samples
// contract over a 500ms window).
func sampleFlagFalse(flag *atomic.Bool, window time.Duration) (bool, int) {
	samples := 0

	deadline := time.Now().Add(window)

	for time.Now().Before(deadline) {
		samples++

		if flag.Load() {
			return false, samples
		}

		time.Sleep(20 * time.Millisecond)
	}

	return !flag.Load(), samples + 1
}

// assertGoroutineBand asserts every sample lies within [baseline, baseline+2]
// — a DELTA BAND, never exact process-wide equality: stray background
// goroutines from earlier tests make exact counts flaky under the full-package
// -race run, while a permanent per-session chain goroutine (the CR-02 spin)
// still breaches a +2 band only when it starts DURING the window; the flag
// sampling is the spin's deterministic witness, the band guards "no NEW
// persistent goroutine per quiesced session".
func assertGoroutineBand(t *testing.T, baseline int, window time.Duration) {
	t.Helper()

	deadline := time.Now().Add(window)

	for time.Now().Before(deadline) {
		if n := goruntime.NumGoroutine(); n < baseline || n > baseline+2 {
			t.Errorf("goroutine count = %d; want within the [%d, %d] delta band", n, baseline, baseline+2)

			return
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// TestWakeChain_IdlesWhenEmpty (22-08 Task 1, G-22-2/CR-02): after ONE
// delivered batch the chain exits AND no successor spawns — idle means idle
// (zero chain goroutines per quiesced session). The buggy unconditional exit
// defer re-spawns a successor within microseconds of every exit, so the
// in-flight flag reads true through nearly the whole spin cycle and the
// >=500ms sampled window catches it deterministically.
func TestWakeChain_IdlesWhenEmpty(t *testing.T) { //nolint:paralleltest // goroutine-band + flag-window battery (see the file comment)
	r, prov := newExpansionRunner(t, true,
		scriptedResp{text: "wake acknowledged"},
	)

	const sid = "sess-wake-idle"

	_ = r.sessionFor(context.Background(), sid)

	sess := r.sessions[sid]

	tr := r.trackerFor(sid)
	if tr == nil {
		t.Fatal("no tracker wired for the session")
	}

	flag := wakeFlag(r, sid)

	// Construction transients (the catch-up no-op goroutine) settle first, so
	// the baseline only counts goroutines that live through the window.
	time.Sleep(150 * time.Millisecond)

	// Baseline sampled immediately BEFORE triggering the chain (delta band).
	baseline := goruntime.NumGoroutine()

	// One delivered batch: the chain wakes, delivers, exits.
	tr.Complete(tasks.Notification{
		TaskID: "exec_idle", Kind: tasks.KindBash, ExitStatus: "0", Tail: "idle marker",
	})

	if !waitFor(5*time.Second, func() bool { return prov.callCount() >= 1 }) {
		t.Fatal("the wake batch never delivered (no provider call)")
	}

	// Settle: any successor spun by a broken exit defer is in full churn by
	// now (each successor spawns within microseconds of its parent's exit).
	time.Sleep(200 * time.Millisecond)

	// (1) The flag stays FALSE across the whole sampled window.
	idle, samples := sampleFlagFalse(flag, 500*time.Millisecond)
	if !idle {
		t.Error("wakeInFlight flag read true while idle — a successor chain keeps spawning (CR-02 spin)")
	}

	if samples < 25 {
		t.Errorf("flag samples = %d; want >= 25 over the 500ms window", samples)
	}

	// (2) No persistent chain goroutine: the count stays in the delta band.
	assertGoroutineBand(t, baseline, 500*time.Millisecond)

	// (3) Exactly the one legitimate wake turn — no re-drain storm.
	if calls := prov.callCount(); calls != 1 {
		t.Errorf("provider calls = %d; want exactly 1 (one batch, no storm)", calls)
	}

	if got := wakeEngineDecisions(t, sess, wakeProvenance); got != 1 {
		t.Errorf("wake EngineDecision lines = %d; want 1", got)
	}
}

// TestWakeChain_NoSpawnPastServeShutdown (22-08 Task 1, G-22-2/CR-02): with a
// CANCELLED serve ctx, scheduleWakeDrain leaves the in-flight flag false (the
// LoadOrStore'd entry never flips) and spawns nothing — the chains' lifetime
// is owned by the ctx the editor controls.
func TestWakeChain_NoSpawnPastServeShutdown(t *testing.T) { //nolint:paralleltest // goroutine-band battery
	r, prov := newExpansionRunner(t, true)

	const sid = "sess-wake-nospawn"

	_ = r.sessionFor(context.Background(), sid)

	// Serve shutdown: the serve ctx is DONE from here on.
	serveCtx, cancel := context.WithCancel(context.Background())
	r.serveCtx = serveCtx
	cancel()

	if r.serveCtxOrBackground().Err() == nil {
		t.Fatal("test setup: serve ctx not cancelled")
	}

	flag := wakeFlag(r, sid)

	// Construction transients settle before the band baseline.
	time.Sleep(150 * time.Millisecond)

	baseline := goruntime.NumGoroutine()

	r.scheduleWakeDrain(sid)

	// The entry must NEVER flip: sampled across the window, not read once —
	// the RED spin holds the flag true only ~86% of the time under -race, so
	// a single read can miss it; >=25 samples cannot.
	idle, samples := sampleFlagFalse(flag, 500*time.Millisecond)
	if !idle {
		t.Error("wakeInFlight flag read true with a dead serve ctx — scheduleWakeDrain spawned a chain past shutdown")
	}

	if samples < 25 {
		t.Errorf("flag samples = %d; want >= 25 over the 500ms window", samples)
	}

	assertGoroutineBand(t, baseline, 500*time.Millisecond)

	if calls := prov.callCount(); calls != 0 {
		t.Errorf("provider calls = %d; want 0 (nothing may run past shutdown)", calls)
	}
}

// TestWakeChain_CtxCancelStopsChain (22-08 Task 1, G-22-2/CR-02): a chain
// mid-retry (busy turn slot) STOPS when the serve ctx cancels, and its exit
// defer does not restart it — flag false, stays false.
func TestWakeChain_CtxCancelStopsChain(t *testing.T) { //nolint:paralleltest // flag-window battery
	r, prov := newExpansionRunner(t, true,
		scriptedResp{text: "wake acknowledged"},
	)

	const sid = "sess-wake-ctxcancel"

	_ = r.sessionFor(context.Background(), sid)

	tr := r.trackerFor(sid)
	if tr == nil {
		t.Fatal("no tracker wired for the session")
	}

	serveCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.serveCtx = serveCtx
	r.wakeRetryInterval = 10 * time.Second // the in-flight chain sits in the retry select once busy

	flag := wakeFlag(r, sid)

	// Hold the turn slot: the spawned chain TryLock-fails and enters its
	// first retry sleep — an in-flight chain mid-retry.
	mu := r.sessionTurnMu(sid)
	mu.Lock()
	defer mu.Unlock()

	tr.Complete(tasks.Notification{
		TaskID: "exec_ctx", Kind: tasks.KindBash, ExitStatus: "0", Tail: "ctx marker",
	})

	time.Sleep(150 * time.Millisecond) // the chain is inside the retry select by now

	if !flag.Load() {
		t.Fatal("test setup: the chain never went in flight (flag false before cancel)")
	}

	if calls := prov.callCount(); calls != 0 {
		t.Fatalf("test setup: provider calls = %d during the busy window; want 0", calls)
	}

	// Serve shutdown mid-retry: the chain must stop, never restart.
	cancel()

	time.Sleep(200 * time.Millisecond) // the chain observes Done and exits

	idle, samples := sampleFlagFalse(flag, 500*time.Millisecond)
	if !idle {
		t.Error("wakeInFlight flag read true after ctx cancel — the exit defer restarted the chain past shutdown (CR-02 churn)")
	}

	if samples < 25 {
		t.Errorf("flag samples = %d; want >= 25 over the 500ms window", samples)
	}

	if calls := prov.callCount(); calls != 0 {
		t.Errorf("provider calls = %d; want 0 (the cancelled chain delivered nothing)", calls)
	}
}

// TestWakeChain_RacingCompletionStillWakes (22-08 Task 1, G-22-2 behavior row
// "genuine race still recovered"): a completion landing after the chain's
// delivery decision but before its exit (the flag-held window — planted by
// firing completion B immediately after completion A, while chain 1 is still
// delivering batch A) still gets delivered: B's own drain callback CASes
// against the still-true flag and loses, so only chain 1's exit defer (the
// gated restart) can carry it. The gating must not strand a racing batch.
// Whether B coalesces into A's batch or rides the restart is a legitimate
// race — the OUTCOME is pinned: both markers delivered exactly once, wake
// turns == provider calls, then idle.
func TestWakeChain_RacingCompletionStillWakes(t *testing.T) { //nolint:paralleltest // flag-window battery
	r, prov := newExpansionRunner(t, true,
		scriptedResp{text: "wake acknowledged 1"},
		scriptedResp{text: "wake acknowledged 2"},
	)

	const sid = "sess-wake-race"

	_ = r.sessionFor(context.Background(), sid)

	sess := r.sessions[sid]

	tr := r.trackerFor(sid)
	if tr == nil {
		t.Fatal("no tracker wired for the session")
	}

	flag := wakeFlag(r, sid)

	// Batch A starts its delivery (chain 1 in flight)...
	tr.Complete(tasks.Notification{
		TaskID: "exec_race_a", Kind: tasks.KindBash, ExitStatus: "0", Tail: "race-a marker",
	})

	// ...and B lands inside the flag-held window: B's scheduleWakeDrain CAS
	// fails (a chain looks active), so the gated restart is B's only ride.
	tr.Complete(tasks.Notification{
		TaskID: "exec_race_b", Kind: tasks.KindBash, ExitStatus: "0", Tail: "race-b marker",
	})

	both := waitFor(5*time.Second, func() bool {
		a, b := false, false

		for _, txt := range wakeUserTexts(t, sess) {
			if strings.Contains(txt, "race-a marker") {
				a = true
			}

			if strings.Contains(txt, "race-b marker") {
				b = true
			}
		}

		return a && b
	})
	if !both {
		t.Fatal("a racing completion was stranded — the gated restart must carry the flag-held batch (B never delivered)")
	}

	// Exactly once each: notifications are consumed exactly once by Drain.
	time.Sleep(200 * time.Millisecond) // settle — any restart chain finished

	wakeTurns, aSeen, bSeen := 0, 0, 0

	for _, txt := range wakeUserTexts(t, sess) {
		if !strings.Contains(txt, "task-notification") {
			continue
		}

		wakeTurns++

		if strings.Contains(txt, "race-a marker") {
			aSeen++
		}

		if strings.Contains(txt, "race-b marker") {
			bSeen++
		}
	}

	if aSeen != 1 || bSeen != 1 {
		t.Errorf("marker deliveries: a=%d b=%d; want 1 and 1 (exactly once each)", aSeen, bSeen)
	}

	if wakeTurns < 1 || wakeTurns > 2 {
		t.Errorf("wake turns = %d; want 1 (coalesced) or 2 (restart-carried)", wakeTurns)
	}

	if calls := prov.callCount(); calls != wakeTurns {
		t.Errorf("provider calls = %d; want %d (one call per delivered batch)", calls, wakeTurns)
	}

	idle, _ := sampleFlagFalse(flag, 500*time.Millisecond)
	if !idle {
		t.Error("flag read true after both batches settled — successor spin")
	}
}

// TestWakeChain_ClosedSessionDropsBatch (22-08 Task 2, G-22-4/CR-04): close is
// TERMINAL for the background machinery — a late completion must drop its
// batch loudly instead of resurrecting the closed session through the
// constructing sessionFor lookup and billing a ghost provider turn.
func TestWakeChain_ClosedSessionDropsBatch(t *testing.T) { //nolint:funlen // two-row close battery
	// Row 1 (terminal unit row): a tracker with pending planted whose session
	// is ABSENT from r.sessions — one drainWakeNotifications call is terminal:
	// true, batch consumed, exactly ONE stderr line naming the session id and
	// the dropped count, zero provider calls, nothing re-enters r.sessions.
	t.Run("missing session is terminal", func(t *testing.T) {
		stderr := &bytes.Buffer{}

		r, prov := newExpansionRunner(t, true)
		r.stderr = stderr

		const sid = "sess-wake-drop"

		_ = r.sessionFor(context.Background(), sid)

		tr := r.trackerFor(sid)
		if tr == nil {
			t.Fatal("no tracker wired for the session")
		}

		// Evict WITHOUT the close chain (direct map surgery): the tracker
		// stays registered while the session is gone — exactly the state a
		// late completion sees when the session is closed or failed.
		r.sessMu.Lock()
		delete(r.sessions, sid)
		r.sessMu.Unlock()

		// Plant pending without a chain: the drain callback is neutralized so
		// the direct call below is the ONLY consumer under test.
		tr.SetDrain(func([]tasks.Notification) {})

		tr.Complete(tasks.Notification{
			TaskID: "exec_drop", Kind: tasks.KindBash, ExitStatus: "0", Tail: "drop marker",
		})

		if len(tr.PendingPeek()) != 1 {
			t.Fatalf("pending peek = %d; want 1 planted", len(tr.PendingPeek()))
		}

		done := r.drainWakeNotifications(context.Background(), sid, tr)
		if !done {
			t.Error("drainWakeNotifications returned false for a missing session — must be terminal (true), never a retry loop")
		}

		if peek := tr.PendingPeek(); len(peek) != 0 {
			t.Errorf("pending after drop = %d; want 0 (the batch is consumed)", len(peek))
		}

		drops := strings.Count(stderr.String(), "session closed — dropping")
		if drops != 1 {
			t.Errorf("drop lines = %d; want exactly 1; stderr: %q", drops, stderr.String())
		}

		if !strings.Contains(stderr.String(), sid) || !strings.Contains(stderr.String(), "dropping 1 pending") {
			t.Errorf("drop line must name the session id and the count; stderr: %q", stderr.String())
		}

		if calls := prov.callCount(); calls != 0 {
			t.Errorf("provider calls = %d; want 0 (a closed session never runs a wake turn)", calls)
		}

		r.sessMu.Lock()
		_, present := r.sessions[sid]
		r.sessMu.Unlock()

		if present {
			t.Error("r.sessions re-contains the dropped session id — the drain reconstructed it (CR-04 resurrection)")
		}
	})

	// Row 2 (end-to-end no-resurrection row): a real session, CloseSession,
	// then a late completion fired on the STILL-HELD tracker reference — the
	// closed id must never re-enter r.sessions, the provider must stay at
	// zero calls, and the chain flag must settle false. In the pre-fix code
	// the drain's constructing sessionFor rebuilds a full session and runs a
	// real ghost wake turn into it.
	t.Run("late completion never resurrects", func(t *testing.T) {
		r, prov := newExpansionRunner(t, true,
			scriptedResp{text: "ghost turn bait — must never stream"},
		)

		const sid = "sess-wake-closed"

		_ = r.sessionFor(context.Background(), sid)

		tr := r.trackerFor(sid)
		if tr == nil {
			t.Fatal("no tracker wired for the session")
		}

		if cerr := r.CloseSession(sid); cerr != nil {
			t.Fatalf("CloseSession: %v", cerr)
		}

		// The late completion: the tracker reference outlived the close (the
		// registry's Wait goroutine fires this minutes later in production).
		tr.Complete(tasks.Notification{
			TaskID: "exec_late", Kind: tasks.KindBash, ExitStatus: "0", Tail: "late marker",
		})

		time.Sleep(300 * time.Millisecond) // settle — a ghost chain would have run by now

		r.sessMu.Lock()
		_, present := r.sessions[sid]
		r.sessMu.Unlock()

		if present {
			t.Error("r.sessions re-contains the closed session id after a late completion (CR-04 resurrection)")
		}

		if calls := prov.callCount(); calls != 0 {
			t.Errorf("provider calls = %d; want 0 (no ghost turn for a closed session)", calls)
		}

		if flag := wakeFlag(r, sid); flag.Load() {
			t.Error("chain flag true after the late completion settled — machinery still running for a closed session")
		}
	})
}

// TestCloseSession_PrunesWakeState (22-08 Task 2, G-22-4/CR-04 + IN-03's
// in-scope half): CloseSession prunes the runner's per-session wake state
// (trackers, wakeInFlight, ptyManagers — after OnClose's Drain ran) and
// cancels RUNNING background subagents beside the queued ones.
func TestCloseSession_PrunesWakeState(t *testing.T) { //nolint:funlen // prune + running-cancel battery
	stderr := &bytes.Buffer{}

	r, _ := newExpansionRunner(t, false)
	r.stderr = stderr

	// Row 1: the per-session state entries exist while the session lives and
	// are gone after CloseSession.
	const sid = "sess-close-prune"

	_ = r.sessionFor(context.Background(), sid)

	if _, ok := r.trackers.Load(sid); !ok {
		t.Fatal("test setup: no tracker entry after sessionFor")
	}

	if _, ok := r.ptyManagers.Load(sid); !ok {
		t.Fatal("test setup: no ptyManagers entry after sessionFor")
	}

	r.scheduleWakeDrain(sid) // creates the wakeInFlight entry (empty queue → clean exit)

	time.Sleep(100 * time.Millisecond)

	if _, ok := r.wakeInFlight.Load(sid); !ok {
		t.Fatal("test setup: no wakeInFlight entry after a drain attempt")
	}

	if cerr := r.CloseSession(sid); cerr != nil {
		t.Fatalf("CloseSession: %v", cerr)
	}

	if _, ok := r.trackers.Load(sid); ok {
		t.Error("r.trackers still carries the closed session (CR-04/IN-03 — late completions can find it)")
	}

	if _, ok := r.wakeInFlight.Load(sid); ok {
		t.Error("r.wakeInFlight still carries the closed session (CR-04/IN-03)")
	}

	if _, ok := r.ptyManagers.Load(sid); ok {
		t.Error("r.ptyManagers still carries the closed session — the entry must go AFTER OnClose's Drain ran")
	}

	// Row 2: RUNNING background subagents are cancelled at close (the NEW
	// CancelRunning link beside CancelQueued in OnClose) with a counted
	// stderr note.
	const sid2 = "sess-close-running"

	_ = r.sessionFor(context.Background(), sid2)

	tr := r.trackerFor(sid2)
	if tr == nil {
		t.Fatal("no tracker wired for the second session")
	}

	cancelled := make(chan struct{})

	queued, err := tr.RegisterSubagent("sub_close_1", func() func() {
		return func() { close(cancelled) }
	})
	if err != nil {
		t.Fatalf("RegisterSubagent: %v", err)
	}

	if queued {
		t.Fatal("test setup: the registration queued — the cap has room")
	}

	if cerr := r.CloseSession(sid2); cerr != nil {
		t.Fatalf("CloseSession (running): %v", cerr)
	}

	select {
	case <-cancelled:
	default:
		t.Error("CloseSession left the RUNNING background subagent alive — CancelRunning missing from the OnClose chain (CR-04)")
	}

	if !strings.Contains(stderr.String(), sid2+" close cancelled 1 running background subagent task(s)") {
		t.Errorf("no counted running-cancel note for %s; stderr: %q", sid2, stderr.String())
	}
}
