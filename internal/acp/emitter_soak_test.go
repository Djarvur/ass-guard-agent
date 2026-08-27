package acp //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The 16-06 adversarial soak (D-04): MINUTES of chaos standing behind the
// seconds-scale ci stress (TestTurnEmitterPriority et al). Env-gated out of
// ci exactly like the behavioral-eval lane (the ASSGUARD_EVAL_GATE pattern):
// without the flag the test SKIPS, so `mise ci` stays fast by construction.
//
// Chaos: N producers alternate foreground/background classes at random (fixed
// seed) with burst pauses — two of them flooders pinning the lanes into
// sustained backpressure; a tormentor flips the writer between fast and
// stalled modes; barrier callers cancel their ctx mid-wait. Closing
// invariants (T-16-17 — the soak IS the latent-deadlock/leak detector):
//
//  1. total frames written == total successfully enqueued (nothing dropped)
//  2. every producer's background subsequence is FIFO on the wire
//  3. no goroutine leak (NumGoroutine settles after the drain)
//  4. the stall counter is positive (the detector actually fired)
//  5. the writer closed cleanly (zero writes after close)
//
// API note: EmitterHandle producers block ONLY on the emitter's root ctx (a
// per-producer mid-block cancel is not expressible against the shipped API —
// that edge is TestTurnEmitterProducerCtxAbort's unit contract). The soak's
// equivalent chaos is abrupt producer exits at random points plus barrier
// callers whose ctx dies mid-wait.

// Soak knobs (the duration is overridable for longer nightly runs).
const (
	soakEnvFlag         = "ASSGUARD_EMITTER_SOAK"
	soakDurationEnv     = "ASSGUARD_EMITTER_SOAK_DURATION"
	soakDefaultDuration = 2 * time.Minute
	soakProducers       = 8
	soakFlooders        = 2 // producers 0..1: no burst pause — they pin the lanes
	soakStallThreshold  = 100 * time.Millisecond
	soakMaxBudget       = 2_000_000 // per-producer frame budget (memory bound)
	soakDrainTimeout    = 60 * time.Second
	soakSettleWindow    = 5 * time.Second
	soakSeed            = 20260827 // fixed seed — reproducible chaos
)

//nolint:paralleltest // the leak-check baseline needs a quiescent suite
func TestTurnEmitterSoak(t *testing.T) {
	if os.Getenv(soakEnvFlag) != "1" {
		t.Skipf("adversarial soak runs only with %s=1 (D-04: minutes of chaos live in the "+
			"eval lane, never ci); try: %s=1 go test ./internal/acp/ -run TestTurnEmitterSoak "+
			"-count=1 -timeout 10m", soakEnvFlag, soakEnvFlag)
	}

	duration := soakDuration(t)
	baseline := runtime.NumGoroutine()

	sink := newSoakSink()

	em := NewTurnEmitter(sink, &syncBuffer{}, TurnEmitterConfig{StallThreshold: soakStallThreshold})
	defer em.Stop()

	// The tormentor stutters the writer; the flooders keep the lanes full
	// through each stall window so the detector provably fires.
	tormentorDone := make(chan struct{})

	go runSoakTormentor(sink, duration, tormentorDone)

	// Producers: bursty class-flippers + flooders (one per pressure profile).
	fg := em.ForegroundHandle("soak")
	bg := em.BackgroundHandle("soak")

	enqueued, logs := runSoakProducers(t, fg, bg, duration)

	runSoakBarriers(t, em, duration/4)

	// Close the chaos: fast mode, then wait for the drain to settle.
	<-tormentorDone
	sink.setStalled(false)

	waitSoakDrain(t, em, enqueued)

	// --- the five closing invariants ---
	checkSoakNothingDropped(t, em, sink, enqueued)
	checkSoakBackgroundFIFO(t, sink, logs)

	stallEpisodes := em.StallCount()
	if stallEpisodes < 1 {
		t.Errorf("stall counter = %d; want >= 1 (the detector must fire during the chaos)", stallEpisodes)
	}

	em.Stop() // idempotent; joins drain + sampler before the close assertion

	sink.closeSink()

	if n := sink.writeAfterClose(); n != 0 {
		t.Errorf("writes after close = %d; want 0 (clean close, Pitfall 8)", n)
	}

	checkSoakNoGoroutineLeak(t, baseline)

	// The result line: one record of what the chaos actually exercised.
	t.Logf("SOAK RESULT: duration=%s producers=%d frames=%d stall_episodes=%d "+
		"invariants=[no-drop,fifo,no-leak,stall-fired,clean-close] all=PASS",
		duration, soakProducers, enqueued, stallEpisodes)
}

// soakDuration resolves the run length (default 2 minutes; the env override
// serves longer nightly runs).
func soakDuration(t *testing.T) time.Duration {
	t.Helper()

	v := os.Getenv(soakDurationEnv)
	if v == "" {
		return soakDefaultDuration
	}

	d, err := time.ParseDuration(v)
	if err == nil && d > 0 {
		return d
	}

	t.Logf("invalid %s=%q — using the default", soakDurationEnv, v)

	return soakDefaultDuration
}

// soakMarker identifies one frame on the wire: its producer and per-producer
// sequence number (encoded in the chunk's messageId).
type soakMarker struct {
	producer int
	seq      int
}

// soakSink is the fake writer under chaos: the tormentor flips it between
// fast and stalled; writes SPIN while stalled (the stutter lives here — the
// drain wedges inside Write exactly as with a slow client). It records every
// delivered frame's marker and guards close semantics.
type soakSink struct {
	mu         sync.Mutex
	stalled    bool
	closed     bool
	afterClose int
	markers    []soakMarker
}

// The soak sink's error sentinels (never wrapped — the caller only checks nil).
var (
	errSoakUndecodable     = errors.New("soak: undecodable frame")
	errSoakWriteAfterClose = errors.New("soak: write after close")
)

func newSoakSink() *soakSink { return &soakSink{} }

func (s *soakSink) Write(m *Message) error {
	mk, ok := soakDecode(m)
	if !ok {
		return errSoakUndecodable
	}

	for {
		s.mu.Lock()
		deliver := !s.stalled && !s.closed

		if deliver {
			s.markers = append(s.markers, mk)
		}

		afterClose := s.closed
		s.mu.Unlock()

		switch {
		case deliver:
			return nil
		case afterClose:
			s.mu.Lock()
			s.afterClose++
			s.mu.Unlock()

			return errSoakWriteAfterClose
		}

		time.Sleep(2 * time.Millisecond) // the stall window the drain wedges in
	}
}

func (s *soakSink) setStalled(v bool) {
	s.mu.Lock()
	s.stalled = v
	s.mu.Unlock()
}

// closeSink closes the sink (the emitter is fully stopped by then — the
// Serve teardown order: emitter Stop BEFORE Writer close, Pitfall 8).
func (s *soakSink) closeSink() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func (s *soakSink) writeAfterClose() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.afterClose
}

func (s *soakSink) snapshot() []soakMarker {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]soakMarker(nil), s.markers...)
}

// soakDecode extracts a frame's marker from the chunk's messageId
// ("p<producer>-<seq>").
func soakDecode(m *Message) (soakMarker, bool) {
	var params struct {
		Update struct {
			MessageID string `json:"messageId"` //nolint:tagliatelle // ACP wire field
		} `json:"update"`
	}

	uerr := json.Unmarshal(m.Params, &params)
	if uerr != nil {
		return soakMarker{}, false
	}

	var mk soakMarker

	_, err := fmt.Sscanf(params.Update.MessageID, "p%d-%d", &mk.producer, &mk.seq)
	if err != nil {
		return soakMarker{}, false
	}

	return mk, true
}

// soakProducerLog records one producer's successfully enqueued frames in
// enqueue order with their lane class — the FIFO ground truth.
type soakProducerLog struct {
	mu     sync.Mutex
	frames []soakEnqueue
}

// soakEnqueue is one accepted enqueue: the frame's sequence number and its
// lane class (0 = foreground, 1 = background).
type soakEnqueue struct {
	seq   int
	class int
}

func (l *soakProducerLog) record(seq, class int) {
	l.mu.Lock()
	l.frames = append(l.frames, soakEnqueue{seq: seq, class: class})
	l.mu.Unlock()
}

func (l *soakProducerLog) background() []int {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]int, 0, len(l.frames))

	for _, f := range l.frames {
		if f.class == 1 {
			out = append(out, f.seq)
		}
	}

	return out
}

// runSoakProducers runs the chaotic producer pool until the deadline and
// returns the total successfully enqueued count plus the per-producer logs.
func runSoakProducers(
	t *testing.T, fg, bg ChunkEmitter, duration time.Duration,
) (int, []*soakProducerLog) {
	t.Helper()

	deadline := time.Now().Add(duration)
	logs := make([]*soakProducerLog, soakProducers)

	var (
		wg       sync.WaitGroup
		enqueued atomic.Int64
	)

	for k := range soakProducers {
		logs[k] = &soakProducerLog{}

		wg.Go(func() {
			enqueued.Add(int64(soakProduce(t, fg, bg, k, deadline, logs[k])))
		})
	}

	wg.Wait()

	return int(enqueued.Load()), logs
}

// soakProduce is one producer's chaos loop: random class per frame (fixed
// per-producer seed), bursty pauses — flooders pause never, pinning the lanes
// into backpressure. Returns the frames it successfully enqueued.
func soakProduce(
	t *testing.T, fg, bg ChunkEmitter, k int, deadline time.Time, log *soakProducerLog,
) int {
	t.Helper()

	rng := rand.New(rand.NewSource(soakSeed + int64(k)))

	for i := range soakMaxBudget {
		if time.Now().After(deadline) {
			return i
		}

		marker := fmt.Sprintf("p%d-%d", k, i)

		class := rng.Intn(2)

		var err error
		if class == 0 {
			err = fg.AgentMessageChunk(marker, "x")
		} else {
			err = bg.AgentMessageChunk(marker, "x")
		}

		if err != nil {
			t.Errorf("producer %d frame %d: %v", k, i, err)

			return i
		}

		log.record(i, class)

		if k >= soakFlooders {
			time.Sleep(time.Duration(rng.Intn(6)) * time.Millisecond)
		}
	}

	return soakMaxBudget
}

// runSoakTormentor stutters the writer: long stalled windows (the flooders
// fill the lanes inside each window — the detector fires) with short fast
// gaps; when the soak ends it leaves the writer FAST so the drain can flush.
func runSoakTormentor(sink *soakSink, duration time.Duration, done chan struct{}) {
	defer close(done)

	rng := rand.New(rand.NewSource(soakSeed))
	deadline := time.Now().Add(duration)

	for time.Now().Before(deadline) {
		sink.setStalled(true)
		time.Sleep(time.Duration(300+rng.Intn(500)) * time.Millisecond)

		sink.setStalled(false)
		time.Sleep(time.Duration(50+rng.Intn(100)) * time.Millisecond)
	}

	sink.setStalled(false)
}

// runSoakBarriers hammers Barrier with short-lived ctxs, some of which die
// mid-wait (the writer is often stalled — the waits are real).
func runSoakBarriers(t *testing.T, em *TurnEmitter, duration time.Duration) {
	t.Helper()

	deadline := time.Now().Add(duration)

	var wg sync.WaitGroup

	for range 4 {
		wg.Go(func() {
			rng := rand.New(rand.NewSource(soakSeed + 99))

			for time.Now().Before(deadline) {
				ctx, cancel := context.WithTimeout(context.Background(),
					time.Duration(rng.Intn(20))*time.Millisecond)

				em.Barrier(ctx)
				cancel()
			}
		})
	}

	wg.Wait()
}

// waitSoakDrain waits (bounded) until every enqueued frame has been written.
func waitSoakDrain(t *testing.T, em *TurnEmitter, enqueued int) {
	t.Helper()

	deadline := time.Now().Add(soakDrainTimeout)

	for time.Now().Before(deadline) {
		if em.WrittenNotifications() == enqueued {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("drain did not settle: written=%d enqueued=%d", em.WrittenNotifications(), enqueued)
}

// checkSoakNothingDropped pins invariant 1: the sink recorded exactly the
// successfully enqueued frames, and the emitter's counter agrees.
func checkSoakNothingDropped(t *testing.T, em *TurnEmitter, sink *soakSink, enqueued int) {
	t.Helper()

	if recorded := len(sink.snapshot()); recorded != enqueued {
		t.Errorf("sink recorded %d frames; want exactly %d (nothing may be dropped)", recorded, enqueued)
	}

	if written := em.WrittenNotifications(); written != enqueued {
		t.Errorf("written counter = %d; want %d", written, enqueued)
	}
}

// checkSoakBackgroundFIFO pins invariant 2: for EVERY producer, its
// background-class frames appear on the wire in exact enqueue order (fg
// preemption may interleave around them — that is the documented policy).
func checkSoakBackgroundFIFO(t *testing.T, sink *soakSink, logs []*soakProducerLog) {
	t.Helper()

	wire := sink.snapshot()

	wirePos := make(map[soakMarker]int, len(wire))
	for i, mk := range wire {
		wirePos[mk] = i
	}

	for k, l := range logs {
		last := -1

		for _, seq := range l.background() {
			pos, ok := wirePos[soakMarker{producer: k, seq: seq}]
			if !ok {
				t.Fatalf("producer %d bg frame %d never reached the wire", k, seq)
			}

			if pos <= last {
				t.Fatalf("producer %d bg FIFO broken: seq %d at wire position %d after %d", k, seq, pos, last)
			}

			last = pos
		}
	}
}

// checkSoakNoGoroutineLeak pins invariant 3: after the drain joins, the
// goroutine count settles back to the pre-soak baseline.
func checkSoakNoGoroutineLeak(t *testing.T, baseline int) {
	t.Helper()

	deadline := time.Now().Add(soakSettleWindow)

	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Errorf("goroutine leak: %d goroutines after settle; baseline was %d", runtime.NumGoroutine(), baseline)
}
