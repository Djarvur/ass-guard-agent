package audit_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/event"
)

// TestOpenFileSink (09-06 T1 Test 1): ""/"-" → stderr (nil close); a path →
// an append-only 0600 file; stdout TARGETS are rejected for files too (the
// locked AUD-02 guard).
func TestOpenFileSink(t *testing.T) {
	t.Parallel()

	w, closeFn, err := audit.OpenFileSink("")
	if err != nil || w != os.Stderr || closeFn != nil {
		t.Errorf("empty path = (w=%v, err=%v); want (stderr, nil) with nil closer", w, err)
	}

	w, closeFn, err = audit.OpenFileSink("-")
	if err != nil || w != os.Stderr || closeFn != nil {
		t.Errorf("'-' = (w=%v, err=%v); want (stderr, nil) with nil closer", w, err)
	}

	path := filepath.Join(t.TempDir(), "sink.jsonl")

	w, closeFn, err = audit.OpenFileSink(path)
	if err != nil {
		t.Fatalf("file sink: %v", err)
	}

	_, wErr := w.Write([]byte("{}\n"))
	if wErr != nil {
		t.Fatalf("write: %v", wErr)
	}

	closeErr := closeFn()
	if closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}

	if info.Mode().Perm() != 0o600 {
		t.Errorf("sink perms = %v; want 0600", info.Mode().Perm())
	}

	for _, target := range []string{"stdout", "/dev/stdout"} {
		_, _, err := audit.OpenFileSink(target)
		if err == nil {
			t.Errorf("OpenFileSink(%q) must reject stdout targets", target)
		}
	}
}

// mirrorReady waits until the mirror's async consumer has subscribed (the bus
// never replays; re-send until the line lands).
func mirrorReady(t *testing.T, bus *event.Bus, m *audit.Mirror, dir, sid string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		bus.Publish(event.UsageUpdate{TurnID: sid + "-turn-000", InputTokens: 1})

		if m.Dropped() >= 0 { // accessor live
			raw, rerr := os.ReadFile(filepath.Join(dir, sid+".jsonl"))
			if rerr == nil && len(raw) > 0 {
				return
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("mirror did not subscribe/land lines within 2s")
}

// TestMirror_PerSessionRouting (Test 2): TurnID routing — each session gets
// its own file under dir, carrying only its own lines.
func TestMirror_PerSessionRouting(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	dir := t.TempDir()

	m := audit.NewMirror(bus, dir, nil)

	mirrorReady(t, bus, m, dir, "sessA")

	bus.Publish(event.RequestShaped{
		TurnID: "sessA-turn-001", Profile: profileZcode,
		VerbatimRequest: []byte(`{"model":"m"}`), Timestamp: time.Now(),
	})
	bus.Publish(event.RequestShaped{
		TurnID: "sessB-turn-001", Profile: profileZcode,
		VerbatimRequest: []byte(`{"model":"m"}`), Timestamp: time.Now(),
	})

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		a, aerr := os.ReadFile(filepath.Join(dir, "sessA.jsonl"))
		b, berr := os.ReadFile(filepath.Join(dir, "sessB.jsonl"))

		if aerr == nil && berr == nil && strings.Count(string(a), "\n") >= 2 &&
			strings.Count(string(b), "\n") >= 1 {
			if strings.Contains(string(a), "sessB") || strings.Contains(string(b), "sessA") {
				t.Fatal("cross-session leakage in per-session routing")
			}

			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("per-session mirror files did not land within 2s")
}

// TestMirror_EngineDecisionProvenance (Test 3): the enriched EngineDecision
// (span + config source) lands in the operator artifact — the "why did it
// continue" line.
func TestMirror_EngineDecisionProvenance(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	dir := t.TempDir()

	m := audit.NewMirror(bus, dir, nil)

	mirrorReady(t, bus, m, dir, "sessE")

	bus.Publish(event.EngineDecision{
		TurnID:       "sessE-turn-002",
		Action:       "continue",
		Signal:       "text:impl-complete",
		MatchedSpan:  "ready to implement",
		ConfigSource: "openspec.toml patterns/impl-complete",
		Reason:       "text-pattern matched",
	})

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(filepath.Join(dir, "sessE.jsonl"))

		if err == nil &&
			strings.Contains(string(raw), "ready to implement") &&
			strings.Contains(string(raw), "openspec.toml patterns/impl-complete") {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("engine-decision provenance line did not land within 2s")
}

// TestMirror_RedactionChokepoint (Test 4): every line is redacted before the
// sink — secret VALUES gone, field names preserved.
func TestMirror_RedactionChokepoint(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	dir := t.TempDir()

	m := audit.NewMirror(bus, dir, nil)

	mirrorReady(t, bus, m, dir, "sessR")

	bus.Publish(event.RequestShaped{
		TurnID: "sessR-turn-001", Profile: profileZcode,
		VerbatimRequest: []byte(`{"api_key":"sk-live-secret-mirror-canary"}`), Timestamp: time.Now(),
	})

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(filepath.Join(dir, "sessR.jsonl"))

		if err == nil && len(raw) > 0 {
			if strings.Contains(string(raw), "sk-live-secret-mirror-canary") {
				t.Fatal("mirror leaked the secret value (chokepoint failed)")
			}

			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("no mirror line within 2s")
}

// TestMirror_StdoutRejection (Test 5): constructing a file-mode mirror over
// os.Stdout panics with the transport-discipline message.
func TestMirror_StdoutRejection(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()

		msg, _ := r.(string)
		if r == nil || !strings.Contains(msg, "stdout") {
			t.Fatalf("panic = %v; want the stdout transport-discipline message", r)
		}
	}()

	audit.NewMirrorFile(event.NewBus(), os.Stdout, nil)
}

// failingWriter always errors — the drop-with-counter fixture.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errMirrorWriteFixture }

// TestMirror_DropWithCounter (Test 6): write failures log + count + keep
// consuming; Dropped() exposes the count.
func TestMirror_DropWithCounter(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()

	m := audit.NewMirrorFile(bus, failingWriter{}, nil)

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		bus.Publish(event.UsageUpdate{TurnID: "sessD-turn-001", InputTokens: 1})

		if m.Dropped() >= 2 {
			return // logged, counted, still consuming
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("dropped counter = %d; want >= 2 (failures counted, consumer alive)", m.Dropped())
}

// TestMirror_SingleFileMode (Test 7): the operator override interleaves every
// session's lines into ONE file, each line carrying its TurnID.
func TestMirror_SingleFileMode(t *testing.T) {
	t.Parallel()

	bus := event.NewBus()
	path := filepath.Join(t.TempDir(), "all.jsonl")

	w, closeFn, err := audit.OpenFileSink(path)
	if err != nil {
		t.Fatalf("OpenFileSink: %v", err)
	}

	defer func() { _ = closeFn() }()

	_ = audit.NewMirrorFile(bus, w, nil)

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		bus.Publish(event.UsageUpdate{TurnID: "sessX-turn-000", InputTokens: 1})

		raw, rerr := os.ReadFile(path)

		if rerr == nil && len(raw) > 0 {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	bus.Publish(event.UsageUpdate{TurnID: "sessX-turn-001", InputTokens: 1})
	bus.Publish(event.UsageUpdate{TurnID: "sessY-turn-001", InputTokens: 1})

	deadline = time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		raw, rerr := os.ReadFile(path)

		if rerr == nil &&
			strings.Contains(string(raw), "sessX-turn-001") &&
			strings.Contains(string(raw), "sessY-turn-001") {
			return // both sessions' lines in ONE file, TurnIDs intact
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("single-file mode did not interleave both sessions within 2s")
}

// errMirrorWriteFixture is the failingWriter's sentinel.
var errMirrorWriteFixture = errors.New("fixture: sink write always fails")
