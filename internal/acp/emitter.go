package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// session/update discriminator vocabulary (v1 kinds — Pitfall 7 pins these
// spellings against schema/v1; v2 renames plan→plan_update, which we NEVER emit).
const (
	keySessionUpdate         = "sessionUpdate"
	keyUpdate                = "update" // the session/update params' payload key
	keyAvailableCommands     = "availableCommands"
	keyMessageID             = "messageId"
	keyContent               = "content"
	updKindAgentMessageChunk = "agent_message_chunk"
	updKindUserMessageChunk  = "user_message_chunk" // v1 ContentChunk kind (the D-05 class-B echo)
	updKindToolCall          = "tool_call"
	updKindToolCallUpdate    = "tool_call_update"
	updKindPlan              = "plan" // v1 full-replacement kind
	updKindThoughtChunk      = "agent_thought_chunk"
)

// The TurnEmitter (16-01/D-01..D-04): ONE ordered, bounded inline emitter that
// owns every client-visible session/update notification frame.
//
// Architecture (RESEARCH Pattern 1):
//
//	producers (runtime forwarders, background emitters)
//	           │  block on their class's lane channel (D-01)
//	           ▼
//	   fg lane ─┐
//	            ├─► single drain goroutine (nested select: fg-first) ──► acp.Writer
//	   bg lane ─┘
//
// Invariants:
//   - Nothing is ever dropped (D-01): lanes are bounded; a producer arriving at
//     an exactly-full lane BLOCKS (with ctx awareness) instead of dropping or
//     merging frames — the same bound-block contract as Writer.Write.
//   - Single-writer total order (D-02): the drain goroutine is the ONLY producer
//     of notification frames into the Writer (≤1 frame in flight), so the
//     Writer's buffer can never hold a backlog of reorderable notifications and
//     foreground preemption stays effective under backpressure (Pitfall 2).
//   - Loud stalls (D-03): a per-lane "full since" watermark sampled by a ticker
//     fires one structured stderr line + one counter increment per sustained
//     stall episode. Never a disconnect, never silent blocking.
//   - Lifecycle order (Pitfall 8): Stop() happens before the Writer closes —
//     after Stop, producer sends fail fast (the root ctx wakes them) so no
//     goroutine can wedge on Writer.Write-after-Close.
//
// Presentation honesty (ACP-03 prohibition): the emitter only ever mirrors what
// a producer handed it. It never synthesizes frames — no fake progress cards,
// no cached stale plans.

// NotificationSink consumes one marshaled frame at a time. *Writer (framer.go)
// satisfies it; tests supply recording/blocking fakes.
type NotificationSink interface {
	Write(msg *Message) error
}

// TurnEmitterConfig carries the emitter's discretionary knobs (CONTEXT: lane
// capacity bounds + exact stall threshold are Claude's discretion; D-03's ~5s
// is directional). Zero/negative values fall back to the documented defaults.
type TurnEmitterConfig struct {
	ForegroundCapacity int           // default DefaultForegroundLaneCapacity
	BackgroundCapacity int           // default DefaultBackgroundLaneCapacity
	StallThreshold     time.Duration // default DefaultStallThreshold
}

// Lane capacity bounds (planner discretion, CONTEXT). The invariant tests are
// capacity-independent; these bounds tune how much burst each class absorbs
// before producers feel backpressure.
const (
	DefaultForegroundLaneCapacity = 128
	DefaultBackgroundLaneCapacity = 256
)

// DefaultStallThreshold is the production stall-reporting threshold (D-03):
// any lane held continuously full beyond this duration logs one structured
// stderr line and increments the stall counter. Tests shrink it.
const DefaultStallThreshold = 5 * time.Second

// stallSampleInterval is the watermark sampler's cadence. Small enough that a
// shrunken test threshold resolves within a few ticks; large enough that
// sampling stays cheap next to actual frame writes.
const stallSampleInterval = 25 * time.Millisecond

// emitterGoroutines is the lifecycle WaitGroup width: the drain + the sampler.
const emitterGoroutines = 2

// Stall log vocabulary: structured key=value lines on the injected stderr
// logger (transport discipline — diagnostics NEVER touch stdout).
const (
	laneForeground = "foreground"
	laneBackground = "background"

	stallLogFormat = "turn emitter stall: lane=%s full_for_ms=%d threshold_ms=%d; producers remain blocked " +
		"(bounded lanes hold their frames — nothing dropped, no disconnect; D-03)"
)

// errNilFrame guards the ActivityEmitter frame methods against a nil argument
// (a forwarder bug must surface as a dropped card with an error, not a panic).
var errNilFrame = errors.New("acp: nil tool-call frame")

// todoWriteToolName is the captured core tool whose calls render as live plan
// frames instead of tool cards (spelling pinned against the registered core
// working set — coreexec/register.go: "TodoWrite").
const todoWriteToolName = "TodoWrite"

// TurnEmitter owns the ordered session/update notification path. One instance
// per server lifetime, constructed inside NewServer (configured at the
// composition root via WithTurnEmitter) so EVERY notification path — prompt
// turns, server-driven turns, background emitters — shares one drain, hence
// one total order (D-02).
type TurnEmitter struct {
	fg chan *Message // foreground lane: preempts queued background frames (D-02)
	bg chan *Message // background lane: FIFO within itself (subagents/engine/automation)

	sink NotificationSink
	log  *log.Logger // stderr diagnostics (stall reports); nil-safe

	// mu guards the enqueue/written counter pair driving Barrier() plus the
	// wake generation channel. NEVER held across a lane send (Pitfall 3 — a
	// blocked producer holding mu would wedge the drain's bookkeeping; the send
	// blocks OUTSIDE the critical section). Plain ints: counters grow by a few
	// per turn and never wrap.
	mu       sync.Mutex
	enqueued int // notifications accepted toward the lanes (pre-send bump, abort-compensated)
	written  int // notifications fully handed to the sink (post-write bump)
	// wake is the CURRENT Barrier broadcast generation: writeOut closes it and
	// swaps in a fresh channel under mu after each written frame, so every
	// waiter parked on the closed generation re-checks written (CR-01 broadcast
	// — a capacity-1 token channel woke only ONE of N concurrent waiters).
	wake chan struct{}

	// ctx is the emitter's root lifecycle ctx: cancelled by Stop() so blocked
	// producers fail fast and the drain exits before Writer Close (Pitfall 8).
	//
	//nolint:containedctx // deliberate emitter-lifetime ctx storage (mirrors Runner.serveCtx discipline)
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	stopOnce sync.Once

	stallThreshold time.Duration
	stalled        atomic.Int64 // 16-01 stall counter (legacy StallCount accessor)
	metrics        *Metrics     // D-16 family adopted by NewServer; canonical /status counter
}

// NewTurnEmitter builds a TurnEmitter over sink and starts its single drain
// goroutine. Production passes the Server's Writer as sink and wires handles
// through Server.Emitter/Server.BackgroundEmitter; tests construct standalone
// instances with recording or blocking fakes.
func NewTurnEmitter(sink NotificationSink, stderr io.Writer, cfg TurnEmitterConfig) *TurnEmitter {
	fgCap := cfg.ForegroundCapacity
	if fgCap <= 0 {
		fgCap = DefaultForegroundLaneCapacity
	}

	bgCap := cfg.BackgroundCapacity
	if bgCap <= 0 {
		bgCap = DefaultBackgroundLaneCapacity
	}

	threshold := cfg.StallThreshold
	if threshold <= 0 {
		threshold = DefaultStallThreshold
	}

	var lg *log.Logger

	if stderr != nil {
		lg = log.New(stderr, "ass-guard/acp/emitter: ", log.LstdFlags|log.Lmsgprefix)
	}

	ctx, cancel := context.WithCancel(context.Background())

	em := &TurnEmitter{
		fg:             make(chan *Message, fgCap),
		bg:             make(chan *Message, bgCap),
		sink:           sink,
		log:            lg,
		wake:           make(chan struct{}), // unbuffered: capacity is meaningless for close-broadcast
		ctx:            ctx,
		cancel:         cancel,
		stallThreshold: threshold,
	}

	em.wg.Add(emitterGoroutines)

	go em.drain()
	go em.sampleLoop()

	return em
}

// frameClass selects which lane a handle enqueues into (D-02): foreground
// frames preempt the queue head; background frames queue FIFO behind them.
type frameClass int

const (
	classForeground frameClass = iota
	classBackground
)

// StallCount returns the number of sustained-stall episodes reported so far
// (D-16 counter family; surfaced later via /status).
func (t *TurnEmitter) StallCount() int64 { return t.stalled.Load() }

// WrittenNotifications returns how many session/update notifications have been
// fully written to the sink. The tracer test pins this against the notification
// frames observed on stdout (single-producer invariant).
func (t *TurnEmitter) WrittenNotifications() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.written
}

// ForegroundHandle returns a session-scoped foreground-class emitter (preempts
// the queue head). This is what Session/prompt turns and Server.Emitter use.
//
//nolint:ireturn // the handle seam is intentionally interface-compatible (tests inject fakes)
func (t *TurnEmitter) ForegroundHandle(sessionID string) ChunkEmitter {
	return t.newHandle(sessionID, classForeground)
}

// BackgroundHandle returns a session-scoped background-class emitter: subagent,
// engine, and automation streams ride the bg lane and never head-of-line-block
// the active turn (D-01/D-02).
//
//nolint:ireturn // same seam rationale as ForegroundHandle
func (t *TurnEmitter) BackgroundHandle(sessionID string) ChunkEmitter {
	return t.newHandle(sessionID, classBackground)
}

// Stop shuts down the emitter. Idempotent. The cancel fires IMMEDIATELY —
// producers blocked on a full lane wake with an error and the drain's select
// exits — but the join waits for the drain's IN-FLIGHT sink.Write to return,
// so a hard-wedged client delays Stop exactly as long as Writer.Close would
// (same teardown semantics, one wedged write at most). Stop runs only at serve
// teardown, after all handlers finished; still-queued frames are discarded.
func (t *TurnEmitter) Stop() {
	t.stopOnce.Do(func() {
		t.cancel()
		t.wg.Wait()
	})
}

// Barrier waits until every notification enqueued BEFORE this call has been
// written to the sink (updates-before-response — the verified cancel contract:
// the agent MUST ensure pending updates precede its session/prompt response).
//
// Order-safety by construction: the drain increments `written` only AFTER
// sink.Write returns, and it writes ≤1 frame at a time — so once written passes
// the enqueue count observed here, every earlier frame is fully on the wire and
// only one frame could possibly be mid-flight. The empty-queue case returns
// immediately (a zero-activity turn must not delay its response).
//
// Wake discipline (CR-01 per-generation broadcast): waiters never consume
// tokens. Each loop iteration snapshots the CURRENT wake generation under mu,
// and writeOut closes that generation after EVERY written frame — so all
// concurrent waiters re-check and either return or re-park on the fresh
// generation. No waiter stalls at terminal state, none merges into another,
// and none busy-spins: after a close fires, the next snapshot under mu always
// sees the post-swap channel.
//
// ctx escape hatches: the CALLER's ctx dying (turn cancelled — the whole
// connection is going away anyway) or the emitter being stopped both end the
// wait without further ordering guarantees.
func (t *TurnEmitter) Barrier(ctx context.Context) {
	t.mu.Lock()
	target := t.enqueued
	t.mu.Unlock()

	for {
		t.mu.Lock()

		done := t.written >= target
		wake := t.wake // this generation's broadcast channel (same critical section as the check)

		t.mu.Unlock()

		if done {
			return
		}

		select {
		case <-wake: // closed by writeOut's broadcast → re-check; post-swap snapshots park on the fresh generation
		case <-ctx.Done():
			return
		case <-t.ctx.Done():
			return
		}
	}
}

// newHandle builds one session-scoped handle; the class fixes the lane for every
// method call routed through it.
func (t *TurnEmitter) newHandle(sessionID string, class frameClass) *EmitterHandle {
	return &EmitterHandle{em: t, sessionID: sessionID, class: class}
}

// writeOut hands one frame to the sink and books it as fully written ONLY after
// the sink call returns — Barrier's target arithmetic depends on that ordering.
// The written bump and the wake broadcast share ONE critical section: the
// close + fresh-channel swap under mu turns the wake into a per-generation
// BROADCAST, so every parked Barrier waiter re-checks after each written frame
// (CR-01). Because the drain is writeOut's only caller, closes are serialized
// and never double-close. Sink errors are swallowed best-effort (closed pipe
// on shutdown), mirroring Writer.drain.
func (t *TurnEmitter) writeOut(m *Message) {
	_ = t.sink.Write(m) // best-effort flush (closed pipe on shutdown surfaces elsewhere)

	t.mu.Lock()
	t.written++
	close(t.wake)                // broadcast: wake EVERY parked waiter (each re-checks under mu)
	t.wake = make(chan struct{}) // fresh generation for waiters arriving after this frame
	t.mu.Unlock()
}

// drain is THE total-order point (D-02): foreground frames preempt always;
// when the fg lane is empty both lanes compete with equal chance. Exit paths:
// the root ctx (Stop) — queued-but-unwritten frames are discarded at teardown.
func (t *TurnEmitter) drain() {
	defer t.wg.Done()

	for {
		// Foreground first, ALWAYS (D-02 preemption-at-the-head).
		select {
		case m := <-t.fg:
			t.writeOut(m)

			continue
		default:
		}

		select {
		case m := <-t.fg:
			t.writeOut(m)
		case m := <-t.bg:
			t.writeOut(m)
		case <-t.ctx.Done():
			return
		}
	}
}

// sampleLoop is D-03's watermark sampler, deliberately SEPARATE from the drain:
// the drain blocks INSIDE sink.Write on a slow client, so stall sampling that
// lived in its select would stop exactly when a stall is real (the anti-D-03 —
// silent blocking). A dedicated goroutine keeps observing lane depths while the
// drain is wedged mid-write.
func (t *TurnEmitter) sampleLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(stallSampleInterval)
	defer ticker.Stop()

	var (
		fgWatermark stallWatermark
		bgWatermark stallWatermark
	)

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			fgWatermark = t.sampleStall(fgWatermark, laneForeground, len(t.fg) == cap(t.fg), now)
			bgWatermark = t.sampleStall(bgWatermark, laneBackground, len(t.bg) == cap(t.bg), now)
		}
	}
}

// sampleStall advances one lane's "full since" watermark (D-03): while the lane
// stays continuously full past the threshold it fires EXACTLY ONE structured
// stderr line + one counter increment per episode; observing the lane not-full
// clears the watermark (the next episode may fire again). Never disconnects,
// never drops — producers stay blocked by design (D-01).
func (t *TurnEmitter) sampleStall(wm stallWatermark, lane string, full bool, now time.Time) stallWatermark {
	if !full {
		return stallWatermark{}
	}

	if wm.since.IsZero() {
		return stallWatermark{since: now}
	}

	if wm.logged || now.Sub(wm.since) < t.stallThreshold {
		return wm
	}

	t.stalled.Add(1)
	t.metrics.noteWriterStall() // D-16 family (nil-safe — adopted by NewServer)

	if t.log != nil {
		t.log.Printf(stallLogFormat, lane, now.Sub(wm.since).Milliseconds(), t.stallThreshold.Milliseconds())
	}

	wm.logged = true

	return wm
}

// stallWatermark tracks one lane's continuously-full episode: since anchors the
// duration math; logged pins the exactly-one-line-per-episode rule.
type stallWatermark struct {
	since  time.Time
	logged bool
}

// EmitterHandle is one session's class-bound view over the TurnEmitter. It
// satisfies ChunkEmitter (backward compat with existing fakes/seams) and
// ActivityEmitter (the extended v1 frame vocabulary). Every method mirrors ONLY
// the payload it was given — synthesis is prohibited (ACP-03 transparency).
type EmitterHandle struct {
	em        *TurnEmitter
	sessionID string
	class     frameClass
}

// ActivityEmitter extends ChunkEmitter with the ACP-03 live-turn vocabulary:
// tool_call / tool_call_update / plan / agent_thought_chunk frames, plus the
// v1 user_message_chunk echo (20-01/D-05 — the class-B command turn's typed
// echo rides the same in-hand handle). Runtime forwarders type-assert their
// emit to this interface; plain ChunkEmitters (existing test fakes across the
// repo) keep compiling untouched (RESEARCH Open Question 1 resolution —
// growth via embedding, never in-place widening).
type ActivityEmitter interface {
	ChunkEmitter

	UserMessageChunk(messageID, text string) error
	ToolCall(frame *ToolCallFrame) error
	ToolCallUpdate(frame *ToolCallUpdateFrame) error
	PlanUpdate(entries []PlanEntry) error
	ThoughtChunk(messageID string, content ContentBlock) error
}

// AgentMessageChunk streams one text chunk as a session/update notification.
func (h *EmitterHandle) AgentMessageChunk(messageID, text string) error {
	return h.enqueueUpdate(map[string]any{
		keySessionUpdate: updKindAgentMessageChunk,
		keyMessageID:     messageID,
		keyContent:       ContentBlock{Type: blockText, Text: text},
	})
}

// UserMessageChunk streams one USER-side text chunk as a session/update
// notification (20-01/D-05): the class-B command turn echoes the typed
// invocation as a user_message_chunk ContentChunk BEFORE the output's
// agent_message_chunks. The messageId MUST differ from the output chunks'
// (a change in messageId indicates a new message has started) so the client
// renders the echo and the output as two messages. Mirror of
// AgentMessageChunk line-for-line except the kind.
func (h *EmitterHandle) UserMessageChunk(messageID, text string) error {
	return h.enqueueUpdate(map[string]any{
		keySessionUpdate: updKindUserMessageChunk,
		keyMessageID:     messageID,
		keyContent:       ContentBlock{Type: blockText, Text: text},
	})
}

// ToolCall mirrors a model-selected tool invocation as a v1 tool_call card —
// except TodoWrite (16-01 presentation rule, applied HERE in the acp layer so
// the runtime stays ACP-word-free per 15-D-20): its captured {todos:[…]} input
// becomes a full-replacement `plan` update instead of a card, and ONLY the
// plan frame is emitted for that call. A malformed TodoWrite input falls back
// to the ordinary card — the activity is real either way and must stay visible
// (ACP-03 transparency: presentation never fabricates or hides activity).
func (h *EmitterHandle) ToolCall(frame *ToolCallFrame) error {
	if frame == nil {
		return errNilFrame
	}

	if frame.Title == todoWriteToolName {
		if entries, ok := planEntriesFromTodoInput(frame.Input); ok {
			return h.PlanUpdate(entries)
		}
	}

	update := map[string]any{
		keySessionUpdate: updKindToolCall,
		"toolCallId":     frame.ToolCallID,
	}

	h.applyOptionalCardFields(update, frame.Title, frame.Kind, frame.Status, frame.Content, frame.Locations)

	return h.enqueueUpdate(update)
}

// planEntriesFromTodoInput decodes the captured TodoWrite input shape
// ({todos:[{content,status,priority}]}, coreexec/todo.go) into v1 plan entries.
// The field names and the status/priority enums coincide verbatim with the
// captured fixtures, so the mapping is a pure re-type — never a rewrite. ok is
// false when the input is absent or unparseable (the caller falls back to the
// tool card).
func planEntriesFromTodoInput(raw json.RawMessage) ([]PlanEntry, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var parsed struct {
		Todos []PlanEntry `json:"todos"`
	}

	unmarshalErr := json.Unmarshal(raw, &parsed)
	if unmarshalErr != nil {
		return nil, false
	}

	return parsed.Todos, true
}

// ToolCallUpdate mirrors a partial/in-progress update for a tool call.
func (h *EmitterHandle) ToolCallUpdate(frame *ToolCallUpdateFrame) error {
	if frame == nil {
		return errNilFrame
	}

	update := map[string]any{
		keySessionUpdate: updKindToolCallUpdate,
		"toolCallId":     frame.ToolCallID,
	}

	h.applyOptionalCardFields(update, frame.Title, frame.Kind, frame.Status, frame.Content, frame.Locations)

	return h.enqueueUpdate(update)
}

// PlanUpdate publishes a full-replacement plan update (v1 `plan` kind): the
// client replaces its ENTIRE plan with entries each update — callers always
// send the complete current set.
func (h *EmitterHandle) PlanUpdate(entries []PlanEntry) error {
	if entries == nil {
		entries = []PlanEntry{}
	}

	return h.enqueueUpdate(map[string]any{
		keySessionUpdate: updKindPlan,
		"entries":        entries,
	})
}

// ThoughtChunk streams one thought fragment as an agent_thought_chunk frame
// (v1 ContentChunk shape). No provider thinking source exists until Phase 21
// (PAR-05); the method completes the frame vocabulary ahead of it.
func (h *EmitterHandle) ThoughtChunk(messageID string, content ContentBlock) error {
	return h.enqueueUpdate(map[string]any{
		keySessionUpdate: updKindThoughtChunk,
		keyMessageID:     messageID,
		keyContent:       content,
	})
}

// Notify enqueues one PRE-BUILT notification frame through this handle's class
// lane with full Barrier accounting (16-02/D-19: the request registry's
// synthetic-cancel $/cancel_request rides the FOREGROUND lane so the turn-end
// barrier orders it before the prompt response — the documented cascade).
// Unlike the frame methods above, the caller owns the payload; the handle only
// routes and counts.
func (h *EmitterHandle) Notify(msg *Message) error {
	h.em.mu.Lock()
	h.em.enqueued++
	h.em.mu.Unlock()

	lane := h.em.bg
	if h.class == classForeground {
		lane = h.em.fg
	}

	select {
	case lane <- msg:
		return nil
	case <-h.em.ctx.Done(): // emitter stopped (serve teardown) — fail fast, never wedge post-Close (Pitfall 8)
		h.em.mu.Lock()
		h.em.enqueued--
		h.em.mu.Unlock()

		return fmt.Errorf("enqueue notification: %w", context.Canceled)
	}
}

// applyOptionalCardFields fills the shared optional tool-card fields, omitting
// every empty one (the wire shape treats them as partial-update options).
func (h *EmitterHandle) applyOptionalCardFields(
	update map[string]any,
	title, kind, status string,
	content []ToolCallContent,
	locations []ToolCallLocation,
) {
	if title != "" {
		update["title"] = title
	}

	if kind != "" {
		update["kind"] = kind
	}

	if status != "" {
		update["status"] = status
	}

	if len(content) > 0 {
		update[keyContent] = content
	}

	if len(locations) > 0 {
		update["locations"] = locations
	}
}

// enqueueUpdate marshals {sessionId, update:{...}} and routes the frame into
// this handle's class lane.
// Enqueue accounting: the pre-send bump lets Barrier snapshot a target that
// includes concurrently-publishing producers; the send happens OUTSIDE mu (a
// full lane blocks WITHOUT holding the lock — Pitfall 3). If the emitter stops
// mid-block the bump is compensated, because that frame will never be written.
func (h *EmitterHandle) enqueueUpdate(update map[string]any) error {
	params := map[string]any{keySessionID: h.sessionID, keyUpdate: update}

	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal session/update: %w", err)
	}

	h.em.mu.Lock()
	h.em.enqueued++
	h.em.mu.Unlock()

	lane := h.em.bg
	if h.class == classForeground {
		lane = h.em.fg
	}

	msg := &Message{JSONRPC: protocolVersion20, Method: methodSessionUpdate, Params: raw}

	select {
	case lane <- msg:
		return nil
	case <-h.em.ctx.Done(): // emitter stopped (serve teardown) — fail fast, never wedge post-Close (Pitfall 8)
		h.em.mu.Lock()
		h.em.enqueued--
		h.em.mu.Unlock()

		return fmt.Errorf("enqueue session/update: %w", context.Canceled)
	}
}
