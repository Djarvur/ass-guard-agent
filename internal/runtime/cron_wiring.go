package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// The cron firing engine (12-07 Task 2, ACP-04/D-02): the Runner-owned
// scheduler goroutine, the per-session turn serialization its firings queue
// behind, the fire-once catch-up on the first active session, and the
// session-lifetime chunk forwarder that mirrors SERVER-DRIVEN turns
// (automation firings, D-01 timer resumes — WINDOWS #3) to the connected ACP
// client. No daemon, no network port: everything lives and dies with the
// serve lifetime (the editor owns the process lifecycle — preserved by
// construction).

// defaultSchedTick is the due-check cadence (D-02; coarse by design).
const defaultSchedTick = 30 * time.Second

// sessionTurnMu returns the per-session turn mutex — THE serialization
// point: client-driven turns (Run), ask-reply resumes, and automation
// firings all hold it for their whole turn, so a due firing QUEUES behind an
// active turn instead of interrupting it (D-02's queue semantics, mirroring
// mutating-alone-in-slot), and concurrent firings serialize among
// themselves.
func (r *Runner) sessionTurnMu(sessionID string) *sync.Mutex {
	mu, _ := r.turnMus.LoadOrStore(sessionID, &sync.Mutex{})

	return mu.(*sync.Mutex) //nolint:forcetypeassert // LoadOrStore stores exactly *sync.Mutex
}

// clientTurnActive reports whether a client-driven Run holds the session's
// turn slot (the session forwarder mutes while one does — Run's own
// forwarder owns those chunks; WINDOWS #3's no-duplicate rule).
func (r *Runner) clientTurnActive(sessionID string) bool {
	v, ok := r.turnActive.Load(sessionID)
	if !ok {
		return false
	}

	return v.(*atomic.Bool).Load() //nolint:forcetypeassert // stored as *atomic.Bool below
}

// markClientTurn arms/clears the client-turn flag for the session.
func (r *Runner) markClientTurn(sessionID string, active bool) {
	v, _ := r.turnActive.LoadOrStore(sessionID, &atomic.Bool{})
	v.(*atomic.Bool).Store(active) //nolint:forcetypeassert // LoadOrStore stores exactly *atomic.Bool
}

// currentSessionID returns the most recently ACTIVE session (the firing
// target — the project's automations fire into the session the operator is
// driving; "" when none exists, in which case missed schedules persist for
// the catch-up pass).
func (r *Runner) currentSessionID() string {
	r.sessMu.Lock()
	defer r.sessMu.Unlock()

	return r.lastSessionID
}

// startScheduler launches the due-check goroutine on the serve-lifetime ctx
// (the ONLY lifecycle: ctx cancellation stops it — Test 4's goroutine-exit).
// The tick is injectable for tests; a zero schedule disables the loop
// entirely (test runners without a store).
func (r *Runner) startScheduler(ctx context.Context) {
	if r.schedule == nil {
		return
	}

	tick := r.schedTick
	if tick <= 0 {
		tick = defaultSchedTick
	}

	done := make(chan struct{})
	r.schedStop = func() { <-done } // tests may wait for a clean stop

	go func() {
		defer close(done)

		t := time.NewTicker(tick)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.fireDueAutomations(ctx)
			}
		}
	}()
}

// fireDueAutomations walks the per-project store's Due set and fires each
// due automation into the current session as an engine-driven turn.
func (r *Runner) fireDueAutomations(ctx context.Context) {
	if r.schedule == nil {
		return
	}

	sessionID := r.currentSessionID()
	if sessionID == "" {
		return // agent not running: the schedule persists for the catch-up pass
	}

	now := r.schedule.Now() // the injectable clock (tests pin it past windows)

	due := r.schedule.Due(now)
	for i := range due {
		r.runAutomationTurn(ctx, sessionID, &due[i], "")
	}
}

// runAutomationTurn fires one automation as an engine-driven turn through
// the SAME per-session serialization the Run path uses (the turn mutex —
// a firing waits for any active turn), with automation StartedBy provenance
// and an EngineDecision audit line. note (the catch-up missed-window text)
// prefixes the prompt context when non-empty. Every firing marks lastFired
// AFTER the turn completes (a crash mid-fire re-fires at most once — the
// exactly-once bound holds per missed window).
//
//nolint:funlen // the firing pipeline reads best as one flow
func (r *Runner) runAutomationTurn(ctx context.Context, sessionID string, a *sched.Automation, note string) {
	if note == "" {
		// The exactly-once claim: re-check + advance lastFired UNDER the store
		// mutex — the due-walk's snapshot may be stale (the catch-up pass or a
		// concurrent tick may have claimed the window already). Catch-up
		// events skip the claim: CatchUp already advanced lastFired atomically
		// when it computed the event (claiming again would find nothing due).
		claimed, ok := r.schedule.ClaimForFire(a.ID, r.schedule.Now())
		if !ok {
			return
		}

		*a = claimed
	}

	sess := r.sessionFor(ctx, sessionID)
	if sess == nil {
		return
	}

	mu := r.sessionTurnMu(sessionID)

	queued := !mu.TryLock()
	if queued {
		mu.Lock() // queue behind the active turn — serialized, never interrupts
	}

	defer mu.Unlock()

	prompt := a.Prompt
	if note != "" {
		prompt = note + "\n\n" + a.Prompt
	}

	// Automation provenance (T-12-07-03): sourced EXCLUSIVELY from this
	// runner-side path — model-visible content can never set it (the
	// Phase-8 command-provenance discipline extended; the 12-07 vocabulary
	// extension of TurnOutput.StartedBy).
	r.sessMu.Lock()
	r.automationProvenance = "automation:" + a.ID
	r.sessMu.Unlock()

	blocks := []session.ContentBlock{{Type: blockText, Text: prompt}}

	stop, err := r.runOneTurn(ctx, sess, blocks)

	r.sessMu.Lock()
	r.automationProvenance = ""
	r.sessMu.Unlock()

	action := "automation fire"
	if note != "" {
		action = "automation catch-up"
	}

	if queued {
		action += " (queued behind active turn)"
	}

	reason := "automation " + a.ID + " (" + a.Title + ") fired as engine-driven turn; schedule: " +
		a.ScheduleDisplay() + "; stop=" + stop
	if err != nil {
		reason += "; error: " + err.Error()
	}

	werr := sess.Manager.AppendEngineDecision(
		"automation:"+a.ID, action, "cron:"+a.ID, "", "sched:schedule", reason,
	)
	if werr != nil {
		_, _ = fmt.Fprintf(r.stderrOrDefault(),
			"ass-guard: automation %s engine-decision write failed: %v\n", a.ID, werr)
	}
}

// runCatchUpOnce guards the one-per-serve catch-up pass (D-02: the FIRST
// active session of a serve lifetime fires each missed automation exactly
// once, lastFired making restarts no-ops).
func (r *Runner) runCatchUpOnce(ctx context.Context, sessionID string) {
	if r.schedule == nil {
		return
	}

	r.catchUpOnce.Do(func() {
		events := r.schedule.CatchUp(r.schedule.Now())
		for i := range events {
			r.runAutomationTurn(ctx, sessionID, &events[i].Automation, events[i].Note)
		}
	})
}

// startSessionForwarder subscribes the session-lifetime chunk forwarder
// (WINDOWS #3's fix): AgentMessageChunk events belonging to this session —
// INCLUDING server-driven turns (D-01 timer resumes, automation firings)
// where no Run subscription exists — reach the connected client. While a
// client-driven Run is active its OWN forwarder owns the chunks (the
// client-turn flag mutes this one — no duplicates). stop unsubscribes (the
// session close chain).
func (r *Runner) startSessionForwarder(sessionID string) (func(), bool) {
	if r.emitFor == nil {
		return func() {}, false // test runners: headless (transcript + audit only)
	}

	emit := r.emitFor(sessionID)
	if emit == nil {
		return func() {}, false
	}

	ch := r.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
	prefix := sessionID + "-turn-"

	go func() {
		for e := range ch {
			if r.clientTurnActive(sessionID) {
				continue // Run's forwarder owns client-turn chunks
			}

			c, ok := e.(event.AgentMessageChunk)
			if !ok || !strings.HasPrefix(c.TurnID, prefix) {
				continue
			}

			_ = emit.AgentMessageChunk(c.MessageID, c.Content)
		}
	}()

	return func() { r.bus.Unsubscribe("AgentMessageChunk", ch) }, true
}
