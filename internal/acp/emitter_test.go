package acp //nolint:testpackage // internal package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// The 16-01 Task-2 invariant tests: backpressure (block-never-drop), foreground
// preemption at the queue head, background FIFO stability, the loud stall
// detector (D-03), and the updates-before-response turn-end barrier — all under
// a controllable blocking writer (the fake slow writer) and the -race detector.
// Every wait is bounded: the whole file adds seconds at most (D-04).

// gatedWriter is the fake slow writer at the byte level (the io.Writer under
// newWriter): every Write blocks until the gate closes, then records. It
// doubles as a NotificationSink for standalone TurnEmitter tests. firstWrite
// signals that the drain has picked up a frame and is wedged mid-write (the
// deterministic "drain busy" state).
type gatedWriter struct {
	gate       chan struct{}
	firstWrite chan struct{}
	once       sync.Once
	mu         sync.Mutex
	buf        bytes.Buffer
}

func newGatedWriter() *gatedWriter {
	return &gatedWriter{gate: make(chan struct{}), firstWrite: make(chan struct{})}
}

func (g *gatedWriter) Write(p []byte) (int, error) {
	g.once.Do(func() { close(g.firstWrite) })

	<-g.gate

	g.mu.Lock()
	g.buf.Write(p)
	g.mu.Unlock()

	return len(p), nil
}

func (g *gatedWriter) release() { close(g.gate) }

func (g *gatedWriter) raw() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.buf.String()
}

// frames parses every recorded newline-delimited frame, in arrival order.
func (g *gatedWriter) frames() []*Message {
	var out []*Message

	for line := range strings.SplitSeq(g.raw(), "\n") {
		if len(bytes.TrimSpace([]byte(line))) == 0 {
			continue
		}

		var m Message

		if json.Unmarshal([]byte(line), &m) == nil {
			out = append(out, &m)
		}
	}

	return out
}

// syncBuffer is a mutex-guarded bytes.Buffer: emitter/server goroutines write
// it while the test polls it — a plain bytes.Buffer would be a data race.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, err := s.buf.Write(p)
	if err != nil {
		return n, fmt.Errorf("buffer write: %w", err)
	}

	return n, nil
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.String()
}

// sinkView adapts the gated writer to the TurnEmitter's NotificationSink so
// standalone emitter tests share the same blocking substrate. One frame per
// call: marshaled JSON + exactly one newline (framer.go's writeFrame shape).
type sinkView struct{ g *gatedWriter }

func (s sinkView) Write(m *Message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}

	b = append(b, '\n')

	_, err = s.g.Write(b)

	return err
}

// markerOf extracts the chunk marker an enqueuer stashed in messageId.
func markerOf(m *Message) string {
	var params struct {
		Update struct {
			MessageID string `json:"messageId"` //nolint:tagliatelle // ACP wire field
		} `json:"update"`
	}

	_ = json.Unmarshal(m.Params, &params)

	return params.Update.MessageID
}

// waitFor polls cond until it holds or the timeout elapses.
func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for: %s", what)
}

// fillBackgroundLane stuffs the bg lane to exactly its capacity and reports
// how many placeholder frames it held.
func fillBackgroundLane(em *TurnEmitter) int {
	filled := 0

	for {
		select {
		case em.bg <- &Message{Params: json.RawMessage(`{}`)}:
			filled++

			continue
		default:
		}

		return filled
	}
}

// TestTurnEmitterPriority proves the D-01/D-02 invariant under a fake slow
// writer: with the background lane pre-filled beyond the foreground lane and
// the drain wedged on an in-flight write, a LATE foreground frame is written
// BEFORE every queued background frame (preemption at the head); background
// frames stay FIFO among themselves; and total frames written equal total
// frames enqueued (nothing dropped).
func TestTurnEmitterPriority(t *testing.T) { //nolint:funlen // full stress scenario
	t.Parallel()

	const bgCount = 100 // well under the lane capacity: every enqueue is accepted

	gw := newGatedWriter()
	stderr := &bytes.Buffer{}

	em := NewTurnEmitter(sinkView{gw}, stderr, TurnEmitterConfig{})
	defer em.Stop()

	fg := em.ForegroundHandle("sess")
	bg := em.BackgroundHandle("sess")

	// Frame 0: the drain picks it up and wedges inside sink.Write.
	err := bg.AgentMessageChunk("bg-0", "x")
	if err != nil {
		t.Fatalf("enqueue bg-0: %v", err)
	}

	select {
	case <-gw.firstWrite:
	case <-time.After(2 * time.Second):
		t.Fatal("drain never picked up the first frame")
	}

	// Pre-fill the background lane; the foreground lane stays empty.
	for i := 1; i <= bgCount; i++ {
		err = bg.AgentMessageChunk(fmt.Sprintf("bg-%d", i), "x")
		if err != nil {
			t.Fatalf("enqueue bg-%d: %v", i, err)
		}
	}

	// The LATE foreground frame — queued behind bgCount background frames.
	err = fg.AgentMessageChunk("fg-late", "x")
	if err != nil {
		t.Fatalf("enqueue fg-late: %v", err)
	}

	// Both handle classes routed to their own lanes: fg holds exactly the late
	// frame; bg holds bg-1..bgCount (bg-0 is mid-write inside the sink).
	if len(em.fg) != 1 || len(em.bg) != bgCount {
		t.Fatalf("lane routing broken: fg=%d (want 1) bg=%d (want %d)", len(em.fg), len(em.bg), bgCount)
	}

	gw.release()

	total := 1 + bgCount + 1

	waitFor(t, "all frames written", 5*time.Second, func() bool {
		return em.WrittenNotifications() == total
	})

	got := gw.frames()
	if len(got) != total {
		t.Fatalf("sink recorded %d frames, want %d (nothing may be dropped)", len(got), total)
	}

	if m := markerOf(got[0]); m != "bg-0" {
		t.Fatalf("first written frame = %q; the in-flight bg-0 must complete first", m)
	}

	if m := markerOf(got[1]); m != "fg-late" {
		t.Fatalf("second written frame = %q; foreground must PREEMPT the queued background", m)
	}

	for i := 1; i <= bgCount; i++ {
		if m := markerOf(got[i+1]); m != fmt.Sprintf("bg-%d", i) {
			t.Fatalf("background FIFO broken at %d: got %q", i, m)
		}
	}
}

// TestTurnEmitterStall proves D-03: a lane held continuously full past the
// (shrunken) threshold produces EXACTLY ONE structured stderr line naming the
// lane and increments the stall counter once per episode; the writer is never
// closed, blocked producers unblock on resume, and frames are never dropped.
func TestTurnEmitterStall(t *testing.T) { //nolint:funlen // full D-03 episode scenario
	t.Parallel()

	gw := newGatedWriter()
	stderr := &syncBuffer{}

	em := NewTurnEmitter(sinkView{gw}, stderr, TurnEmitterConfig{StallThreshold: 60 * time.Millisecond})
	defer em.Stop()

	bg := em.BackgroundHandle("sess")

	// Wedge the drain on an in-flight frame, then hold the bg lane exactly full.
	err := bg.AgentMessageChunk("bg-0", "x")
	if err != nil {
		t.Fatalf("enqueue bg-0: %v", err)
	}

	select {
	case <-gw.firstWrite:
	case <-time.After(2 * time.Second):
		t.Fatal("drain never picked up the first frame")
	}

	filled := fillBackgroundLane(em)

	// A producer arriving at the exactly-full lane BLOCKS (D-01) — track it.
	blockedErr := make(chan error, 1)

	go func() { blockedErr <- bg.AgentMessageChunk("bg-blocked", "x") }()

	// Past the threshold: exactly one stall line for the background lane...
	waitFor(t, "stall line on stderr", 2*time.Second, func() bool {
		return strings.Contains(stderr.String(), "turn emitter stall") &&
			strings.Contains(stderr.String(), "lane=background")
	})

	// ...and the episode fires ONCE: several more sampling periods pass without
	// a second line or a second counter increment.
	time.Sleep(5 * stallSampleInterval)

	if lines := strings.Count(stderr.String(), "turn emitter stall"); lines != 1 {
		t.Fatalf("stall logged %d times, want exactly 1 per episode:\n%s", lines, stderr.String())
	}

	if n := em.StallCount(); n != 1 {
		t.Fatalf("stall counter = %d, want 1", n)
	}

	if strings.Contains(stderr.String(), "lane=foreground") {
		t.Fatalf("foreground lane was not full; spurious stall line:\n%s", stderr.String())
	}

	// Resume: the blocked producer unblocks, frames flow, no close occurred.
	gw.release()

	select {
	case err := <-blockedErr:
		if err != nil {
			t.Fatalf("blocked producer failed on resume: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked producer never unblocked after release")
	}

	err = bg.AgentMessageChunk("bg-after", "x")
	if err != nil {
		t.Fatalf("enqueue after release: %v", err)
	}

	total := 1 + filled + 2 // bg-0 + full lane + bg-blocked + bg-after

	waitFor(t, "all frames written after resume", 5*time.Second, func() bool {
		return em.WrittenNotifications() == total
	})

	if len(gw.frames()) != total {
		t.Fatalf("nothing may be dropped across a stall: recorded %d, want %d", len(gw.frames()), total)
	}

	if n := em.StallCount(); n != 1 {
		t.Fatalf("stall counter changed after resume: %d", n)
	}
}

// TestTurnEmitterProducerCtxAbort proves the D-03 ctx-awareness edge: a
// producer blocked on an exactly-full lane returns PROMPTLY when the emitter's
// root ctx is cancelled (no wedge), and its not-yet-enqueued frame is
// compensated out of the barrier accounting. Stop's join is satisfied only
// after the wedged write is released — teardown semantics mirror Writer.Close.
func TestTurnEmitterProducerCtxAbort(t *testing.T) {
	t.Parallel()

	gw := newGatedWriter()

	em := NewTurnEmitter(sinkView{gw}, &syncBuffer{}, TurnEmitterConfig{})

	bg := em.BackgroundHandle("sess")

	// Wedge the drain, then fill the lane exactly.
	err := bg.AgentMessageChunk("bg-0", "x")
	if err != nil {
		t.Fatalf("enqueue bg-0: %v", err)
	}

	select {
	case <-gw.firstWrite:
	case <-time.After(2 * time.Second):
		t.Fatal("drain never picked up the first frame")
	}

	fillBackgroundLane(em)

	em.mu.Lock()
	target := em.enqueued // the blocked producer's abort must restore this count
	em.mu.Unlock()

	producerDone := make(chan error, 1)

	go func() { producerDone <- bg.AgentMessageChunk("never-arrives", "x") }()

	time.Sleep(50 * time.Millisecond) // let the producer block on the full lane

	stopDone := make(chan struct{})

	go func() { em.Stop(); close(stopDone) }() // cancel fires now; join waits on the wedged write

	select {
	case err := <-producerDone:
		if err == nil {
			t.Fatal("producer reported success; it must fail fast on stop")
		}

		if !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("producer error should name the cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked producer did not return promptly after cancel")
	}

	gw.release() // un-wedge the drain so Stop's join can complete

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not complete after the wedged write released")
	}

	em.mu.Lock()
	compensated := em.enqueued == target
	em.mu.Unlock()

	if !compensated {
		t.Fatal("aborted producer's enqueue bump was not compensated")
	}
}

// TestTurnEmitterBarrier proves updates-before-response at the SERVER level:
// with the writer stalled mid-turn, the session/prompt response does not pass
// the queued turn notifications — after release, every notification frame
// precedes the response on the wire (the handler is parked INSIDE Barrier while
// the writer stays wedged, so the ordering is by construction, not luck).
func TestTurnEmitterBarrier(t *testing.T) { //nolint:funlen // full server-level scenario
	t.Parallel()

	gw := newGatedWriter()
	stderr := &syncBuffer{}

	inR, inW := io.Pipe()
	defer func() { _ = inW.Close() }()

	srv := NewServer(inR, gw, stderr,
		WithTurnRunner(&barrierStubTurn{chunks: []string{"c1", "c2", "c3"}}))

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan struct{})

	go func() { _ = srv.Serve(ctx); close(serveDone) }()

	t.Cleanup(func() {
		cancel()

		_ = inR.Close()

		select {
		case <-serveDone:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	})

	w := bufio.NewWriter(inW)
	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{}}`+"\n")
	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/tmp"}}`+"\n")
	_ = w.Flush()

	// The writer is wedged, so responses queue inside the Writer's buffer; ask
	// the server's registry for the created session id.
	sessID := readSessionIDBlind(t, srv)

	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":%q,`+
		`"prompt":[{"type":"text","text":"go"}]}}`+"\n", sessID)
	_ = w.Flush()

	// Give the turn time to finish and the handler to park INSIDE Barrier (the
	// writer is still wedged, so written can never reach the turn's enqueue
	// count). If the barrier were missing, the response would already be
	// enqueued here — the release ordering below would then be luck.
	time.Sleep(150 * time.Millisecond)

	gw.release()

	// After release everything flushes; wait for the response then check order.
	waitFor(t, "prompt response after release", 5*time.Second, func() bool {
		return responseSeen(gw, "2")
	})

	got := gw.frames()
	updateIdx, respIdx := -1, -1

	for i, m := range got {
		if m.Method == methodSessionUpdate && updateIdx == -1 {
			updateIdx = i
		}

		if m.ID != nil && string(m.ID) == "2" {
			respIdx = i
		}
	}

	if updateIdx == -1 || respIdx == -1 {
		t.Fatalf("missing frames: updateIdx=%d respIdx=%d (recorded=%d)", updateIdx, respIdx, len(got))
	}

	if respIdx < updateIdx {
		t.Fatalf("response overtook its turn's notifications (response at %d, update at %d)", respIdx, updateIdx)
	}
}

// TestTurnEmitterBarrierConcurrentWaiters is the CR-01 regression (16-07): TWO
// concurrent turn-end Barriers over one shared emitter whose targets are both
// satisfied by the SAME final frame, each holding a connection-lifetime context
// (context.Background() — deliberately NEVER cancelled, because escaping via a
// short-lived ctx is exactly how the broken state hides: every pre-existing
// waiter test could return through ctx.Done, so none proved the wake itself).
// On the pre-fix capacity-1 token channel the final frame wakes at most ONE
// waiter; the other is stranded at terminal state forever — the RED gate.
// The lone-waiter subtest guards against over-correction: the broadcast must
// not lose the plain single-waiter per-frame wake.
func TestTurnEmitterBarrierConcurrentWaiters(t *testing.T) { //nolint:funlen // two scenarios, one regression
	t.Parallel()

	t.Run("same final frame satisfies both waiters", func(t *testing.T) {
		t.Parallel()

		gw := newGatedWriter()

		em := NewTurnEmitter(sinkView{gw}, &syncBuffer{}, TurnEmitterConfig{})
		defer em.Stop()

		fg := em.ForegroundHandle("sess")

		// Frame 1: the drain picks it up and wedges INSIDE sink.Write, so
		// written stays 0 while the whole turn is enqueued.
		err := fg.AgentMessageChunk("fg-0", "x")
		if err != nil {
			t.Fatalf("enqueue fg-0: %v", err)
		}

		select {
		case <-gw.firstWrite:
		case <-time.After(2 * time.Second):
			t.Fatal("drain never picked up the first frame")
		}

		// Frame 2: queued behind the wedged write. Both Barriers below target
		// written >= 2 — the FINAL frame satisfies both (the CR-01 shape).
		err = fg.AgentMessageChunk("fg-1", "x")
		if err != nil {
			t.Fatalf("enqueue fg-1: %v", err)
		}

		const waiters = 2

		done := make(chan struct{}, waiters)

		for range waiters {
			go func() {
				em.Barrier(context.Background()) // connection-lifetime ctx: no Done escape

				done <- struct{}{}
			}()
		}

		// Let both waiters park while the sink holds write #1 (same bounded
		// settle convention as TestTurnEmitterProducerCtxAbort).
		time.Sleep(100 * time.Millisecond)

		gw.release()

		for i := range waiters {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("Barrier waiter %d/%d still parked after the final frame was written — lost wakeup (CR-01)",
					i+1, waiters)
			}
		}

		waitFor(t, "both frames written", 2*time.Second, func() bool {
			return em.WrittenNotifications() == 2
		})
	})

	t.Run("a lone waiter still wakes per frame", func(t *testing.T) {
		t.Parallel()

		gw := newGatedWriter()

		em := NewTurnEmitter(sinkView{gw}, &syncBuffer{}, TurnEmitterConfig{})
		defer em.Stop()

		fg := em.ForegroundHandle("sess")

		err := fg.AgentMessageChunk("fg-0", "x")
		if err != nil {
			t.Fatalf("enqueue fg-0: %v", err)
		}

		select {
		case <-gw.firstWrite:
		case <-time.After(2 * time.Second):
			t.Fatal("drain never picked up the first frame")
		}

		done := make(chan struct{}, 1)

		go func() {
			em.Barrier(context.Background()) // target=1: the FIRST written frame satisfies it

			done <- struct{}{}
		}()

		// Park the waiter before any further traffic (same settle convention).
		time.Sleep(100 * time.Millisecond)

		// A second frame enqueued WHILE the waiter is parked must not disturb
		// the plain per-frame wake its target already satisfies.
		err = fg.AgentMessageChunk("fg-1", "x")
		if err != nil {
			t.Fatalf("enqueue fg-1: %v", err)
		}

		gw.release()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("lone Barrier waiter never woke on the frame that satisfied its target")
		}

		waitFor(t, "both frames written", 2*time.Second, func() bool {
			return em.WrittenNotifications() == 2
		})
	})
}

// TestTurnEmitterEmptyTurn proves the zero-activity edge: a turn with no
// streamed frames emits zero session/update notifications, its barrier returns
// immediately (no stall, no delay), and the prompt response still arrives.
func TestTurnEmitterEmptyTurn(t *testing.T) {
	t.Parallel()

	var stdout syncBuffer

	inR, inW := io.Pipe()
	defer func() { _ = inW.Close() }()

	srv := NewServer(inR, &stdout, &syncBuffer{}, WithTurnRunner(&barrierStubTurn{}))

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan struct{})

	go func() { _ = srv.Serve(ctx); close(serveDone) }()

	t.Cleanup(func() {
		cancel()

		_ = inR.Close()

		select {
		case <-serveDone:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	})

	w := bufio.NewWriter(inW)
	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{}}`+"\n")
	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/tmp"}}`+"\n")
	_ = w.Flush()

	sessID := readSessionIDBlind(t, srv)

	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":%q,"prompt":[]}}`+"\n",
		sessID)
	_ = w.Flush()

	waitFor(t, "empty-turn prompt response", 5*time.Second, func() bool {
		return strings.Contains(stdout.String(), `"id":2`)
	})

	if strings.Contains(stdout.String(), methodSessionUpdate) {
		t.Fatalf("empty turn emitted notifications:\n%s", stdout.String())
	}
}

// responseSeen reports whether the writer recorded a response with the given id.
func responseSeen(gw *gatedWriter, id string) bool {
	for _, m := range gw.frames() {
		if m.ID != nil && string(m.ID) == id {
			return true
		}
	}

	return false
}

// readSessionIDBlind extracts the created session id while the writer is
// wedged: responses sit in the Writer buffer, so the test asks the SERVER's
// registry instead of the wire.
func readSessionIDBlind(t *testing.T, srv *Server) string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.Lock()

		for id := range srv.sessions {
			srv.mu.Unlock()

			return id
		}

		srv.mu.Unlock()

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal("no session created")

	return ""
}

// barrierStubTurn emits the configured chunks then returns end_turn (zero
// chunks = the empty-turn case).
type barrierStubTurn struct {
	chunks []string
}

func (s *barrierStubTurn) Run(_ context.Context, _ string, emit ChunkEmitter, _ []ContentBlock) (string, error) {
	for _, c := range s.chunks {
		err := emit.AgentMessageChunk(c, c)
		if err != nil {
			return "", fmt.Errorf("emit chunk: %w", err)
		}
	}

	return stopEndTurn, nil
}

// --- Task 3: frame-surface completion (plan from TodoWrite, thought chunk,
// tool_call_update shapes). A recording sink keeps the emitter-level tests
// synchronous: enqueue, wait for written, assert the recorded frame.

// recordingSink collects every written frame (never blocks).
type recordingSink struct {
	mu   sync.Mutex
	msgs []*Message
}

func (r *recordingSink) Write(m *Message) error {
	r.mu.Lock()
	r.msgs = append(r.msgs, m)
	r.mu.Unlock()

	return nil
}

func (r *recordingSink) recorded() []*Message {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]*Message(nil), r.msgs...)
}

// updateOf decodes a session/update frame's update object.
func updateOf(t *testing.T, m *Message) map[string]any {
	t.Helper()

	var params struct {
		Update map[string]any `json:"update"`
	}

	unmarshalErr := json.Unmarshal(m.Params, &params)
	if unmarshalErr != nil {
		t.Fatalf("decode update: %v (%s)", unmarshalErr, string(m.Params))
	}

	return params.Update
}

// TestPlanFrameFromTodoWrite proves the presentation rule (INSIDE internal/acp
// — the runtime stays ACP-word-free per 15-D-20): a TodoWrite tool call whose
// input carries the captured {todos:[…]} shape renders as a v1 `plan` update —
// full-replacement entries mapped content/priority/status verbatim — and does
// NOT also emit a tool_call card for that call. Any other tool still gets its
// ordinary card, and a malformed TodoWrite input falls back to the card (the
// real activity must stay visible — ACP-03 transparency).
func TestPlanFrameFromTodoWrite(t *testing.T) { //nolint:funlen,cyclop // three scenarios, one rule
	t.Parallel()

	rec := &recordingSink{}

	em := NewTurnEmitter(rec, &syncBuffer{}, TurnEmitterConfig{})
	defer em.Stop()

	fg := em.ForegroundHandle("sess").(ActivityEmitter) //nolint:forcetypeassert // handle implements the full surface

	// Scenario 1: the happy path — plan frame instead of a tool card.
	err := fg.ToolCall(&ToolCallFrame{
		ToolCallID: "tc-todo-1",
		Title:      "TodoWrite",
		Input: json.RawMessage(`{"todos":[` +
			`{"content":"map the seam","status":"completed","priority":"high"},` +
			`{"content":"wire the emitter","status":"in_progress","priority":"medium"},` +
			`{"content":"prove the invariants","status":"pending","priority":"low"}]}`),
	})
	if err != nil {
		t.Fatalf("TodoWrite tool call: %v", err)
	}

	waitFor(t, "plan frame written", 2*time.Second, func() bool {
		return em.WrittenNotifications() == 1
	})

	got := rec.recorded()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 frame for the TodoWrite call, got %d (no card beside the plan)", len(got))
	}

	upd := updateOf(t, got[0])
	if upd["sessionUpdate"] != "plan" {
		t.Fatalf("sessionUpdate = %v; want plan (v1 kind, full replacement)", upd["sessionUpdate"])
	}

	entries, ok := upd["entries"].([]any)
	if !ok || len(entries) != 3 {
		t.Fatalf("entries = %v; want 3 full-replacement rows", upd["entries"])
	}

	first, entryOK := entries[0].(map[string]any)
	if !entryOK || first["content"] != "map the seam" || first["status"] != "completed" ||
		first["priority"] != "high" {
		t.Fatalf("entry[0] = %v; want mapped content/status/priority", entries[0])
	}

	// Scenario 2: any OTHER tool keeps its ordinary tool_call card.
	err = fg.ToolCall(&ToolCallFrame{
		ToolCallID: "tc-bash-1",
		Title:      "Bash",
		Input:      json.RawMessage(`{"command":"ls"}`),
	})
	if err != nil {
		t.Fatalf("Bash tool call: %v", err)
	}

	waitFor(t, "bash card written", 2*time.Second, func() bool {
		return em.WrittenNotifications() == 2
	})

	updBash := updateOf(t, rec.recorded()[1])
	if updBash["sessionUpdate"] != updKindToolCall || updBash["toolCallId"] != "tc-bash-1" {
		t.Fatalf("non-TodoWrite call lost its card: %v", updBash)
	}

	// Scenario 3: a TodoWrite call with unparseable input falls back to the
	// card — the activity is real and must stay visible.
	err = fg.ToolCall(&ToolCallFrame{
		ToolCallID: "tc-todo-2",
		Title:      "TodoWrite",
		Input:      json.RawMessage(`{"todos":"not-an-array"}`),
	})
	if err != nil {
		t.Fatalf("malformed TodoWrite call: %v", err)
	}

	waitFor(t, "fallback card written", 2*time.Second, func() bool {
		return em.WrittenNotifications() == 3
	})

	updFallback := updateOf(t, rec.recorded()[2])
	if updFallback["sessionUpdate"] != updKindToolCall || updFallback["toolCallId"] != "tc-todo-2" {
		t.Fatalf("malformed TodoWrite input must fall back to the card: %v", updFallback)
	}
}

// TestThoughtChunkFrame pins the agent_thought_chunk shape (v1 ContentChunk):
// messageId + a content block, unit-proven only until Phase 21 provides a live
// provider thinking source (PAR-05).
func TestThoughtChunkFrame(t *testing.T) {
	t.Parallel()

	rec := &recordingSink{}

	em := NewTurnEmitter(rec, &syncBuffer{}, TurnEmitterConfig{})
	defer em.Stop()

	fg := em.ForegroundHandle("sess").(ActivityEmitter) //nolint:forcetypeassert // handle implements the full surface

	err := fg.ThoughtChunk("msg-9", ContentBlock{Type: blockText, Text: "considering the ordering"})
	if err != nil {
		t.Fatalf("thought chunk: %v", err)
	}

	waitFor(t, "thought frame written", 2*time.Second, func() bool {
		return em.WrittenNotifications() == 1
	})

	got := rec.recorded()
	if len(got) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(got))
	}

	upd := updateOf(t, got[0])
	if upd["sessionUpdate"] != updKindThoughtChunk {
		t.Fatalf("sessionUpdate = %v; want agent_thought_chunk (v1 spelling)", upd["sessionUpdate"])
	}

	if upd["messageId"] != "msg-9" {
		t.Fatalf("messageId = %v; want msg-9", upd["messageId"])
	}

	content, ok := upd["content"].(map[string]any)
	if !ok || content["type"] != blockText || content["text"] != "considering the ordering" {
		t.Fatalf("content = %v; want the text content block", upd["content"])
	}
}

// TestToolCallUpdateFrame pins the tool_call_update partial-update shape under
// verbatim camelCase v1 names: kind/status plus the diff content variant
// (path/oldText/newText) and locations (absolute path + optional line).
func TestToolCallUpdateFrame(t *testing.T) {
	t.Parallel()

	rec := &recordingSink{}

	em := NewTurnEmitter(rec, &syncBuffer{}, TurnEmitterConfig{})
	defer em.Stop()

	fg := em.ForegroundHandle("sess").(ActivityEmitter) //nolint:forcetypeassert // handle implements the full surface

	line := int64(42)

	err := fg.ToolCallUpdate(&ToolCallUpdateFrame{
		ToolCallID: "upd-1",
		Kind:       ToolKindEdit,
		Status:     StatusInProgress,
		Content: []ToolCallContent{
			DiffContent{Path: "/repo/main.go", OldText: "old", NewText: "new"}.Frame(),
		},
		Locations: []ToolCallLocation{{Path: "/repo/main.go", Line: &line}},
	})
	if err != nil {
		t.Fatalf("tool call update: %v", err)
	}

	waitFor(t, "update frame written", 2*time.Second, func() bool {
		return em.WrittenNotifications() == 1
	})

	raw := string(rec.recorded()[0].Params)
	for _, wireKey := range []string{
		`"toolCallId":"upd-1"`, `"kind":"edit"`, `"status":"in_progress"`,
		`"type":"diff"`, `"path":"/repo/main.go"`, `"oldText":"old"`, `"newText":"new"`,
		`"locations":[{`, `"line":42`,
	} {
		if !strings.Contains(raw, wireKey) {
			t.Fatalf("wire frame missing %s:\n%s", wireKey, raw)
		}
	}
}
