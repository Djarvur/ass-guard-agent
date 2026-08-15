package audit // 09-06, AUD-02/D-02: the per-session mirror (see doc.go)

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/redact"
)

// mirrorLine is the on-disk mirror record: the correlation triple + the event
// kind + the event's own fields (flattened per kind below).
type mirrorLine struct {
	Timestamp time.Time `json:"timestamp"`
	Session   string    `json:"session"`
	Turn      string    `json:"turn"`
	Kind      string    `json:"kind"`

	// RequestShaped
	Profile string `json:"profile,omitempty"`
	//nolint:tagliatelle // on-disk format: camelCase matches the transcript's Line convention
	HeaderNames []string `json:"headerNames,omitempty"`

	// EngineDecision
	Action string `json:"action,omitempty"`
	Signal string `json:"signal,omitempty"`
	//nolint:tagliatelle // on-disk format: camelCase matches the transcript's Line convention
	MatchedSpan string `json:"matchedSpan,omitempty"`
	//nolint:tagliatelle // on-disk format: camelCase matches the transcript's Line convention
	ConfigSource string `json:"configSource,omitempty"`
	Reason       string `json:"reason,omitempty"`

	// UsageUpdate
	//nolint:tagliatelle // on-disk format: camelCase matches the transcript's Line convention
	InputTokens int64 `json:"inputTokens,omitempty"`
	//nolint:tagliatelle // on-disk format: camelCase matches the transcript's Line convention
	OutputTokens int64 `json:"outputTokens,omitempty"`
}

// Mirror is the compact per-session audit consumer (D-02): per-session mode
// writes <dir>/<sessionID>.jsonl (append-only 0600, lazily opened, cached
// handles); single-file mode (the --audit-log override) interleaves every
// session's lines into one writer. It subscribes to RequestShaped,
// EngineDecision, and UsageUpdate; lines are routed by TurnID's session
// prefix (the stable "<sessionID>-turn-%03d" format). Write failures are
// logged once per distinct error, counted (Dropped()), and dropped — the bus
// never blocks, the turn never learns (Pitfall 10).
type Mirror struct {
	dir    string // per-session mode root; empty in single-file mode
	single io.Writer

	log     *slog.Logger
	dropped atomic.Int64

	mu      sync.Mutex
	handles map[string]io.Writer // sessionID → cached per-session sink

	lastErrStr atomic.Value // distinct-failure logging guard
}

// NewMirror returns the per-session mirror rooted at dir (files created lazily
// via the shared OpenFileSink semantics — 0600 append-only; NEVER os.Create
// default perms). A nil logger falls back to the slog default (stderr).
func NewMirror(bus *event.Bus, dir string, log *slog.Logger) *Mirror {
	// Best-effort root creation at construction: a missing dir would turn the
	// first per-session open into a drop (the body store creates its own
	// subtree lazily — the mirror must not depend on that ordering).
	_ = os.MkdirAll(dir, dirPermOwnerOnly)

	m := &Mirror{dir: dir, log: log, handles: map[string]io.Writer{}}
	m.subscribe(bus)

	return m
}

// NewMirrorFile returns the single-file (operator --audit-log override)
// mirror writing every session's lines to w, interleaved, TurnIDs intact.
// Panics when w == os.Stdout (transport discipline — same guard class as
// NewAuditLogger).
func NewMirrorFile(bus *event.Bus, w io.Writer, log *slog.Logger) *Mirror {
	if w == os.Stdout {
		panic("audit: mirror sink must not be os.Stdout (transport discipline — stdout is reserved for ACP frames)")
	}

	m := &Mirror{single: w, log: log, handles: map[string]io.Writer{}}
	m.subscribe(bus)

	return m
}

// Dropped returns the count of lines lost to sink failures (observability;
// tests). Failures never block the bus.
func (m *Mirror) Dropped() int64 { return m.dropped.Load() }

// subscribe wires the three consumed event kinds with bounded buffers.
func (m *Mirror) subscribe(bus *event.Bus) {
	reqs := bus.Subscribe("RequestShaped", event.BufRequestShaped)
	decisions := bus.Subscribe("EngineDecision", event.BufEngineDecision)
	usage := bus.Subscribe("UsageUpdate", event.BufUsageUpdate)

	go func() {
		for e := range reqs {
			m.consume(e)
		}
	}()

	go func() {
		for e := range decisions {
			m.consume(e)
		}
	}()

	go func() {
		for e := range usage {
			m.consume(e)
		}
	}()
}

// consume converts one event to a mirror line, redacts it, routes it.
func (m *Mirror) consume(e event.Event) {
	line := mirrorLine{Timestamp: time.Now().UTC()}

	switch ev := e.(type) {
	case event.RequestShaped:
		line.Kind = "request_shaped"
		line.Turn = ev.TurnID
		line.Profile = ev.Profile
		line.HeaderNames = ev.HeaderNames
	case event.EngineDecision:
		line.Kind = "engine_decision"
		line.Turn = ev.TurnID
		line.Action = ev.Action
		line.Signal = ev.Signal
		line.MatchedSpan = ev.MatchedSpan
		line.ConfigSource = ev.ConfigSource
		line.Reason = ev.Reason
	case event.UsageUpdate:
		line.Kind = "usage"
		line.Turn = ev.TurnID
		line.InputTokens = ev.InputTokens
		line.OutputTokens = ev.OutputTokens
	default:
		return
	}

	// The routing key: TurnID's stable "<sessionID>-turn-%03d" format.
	line.Session, _, _ = strings.Cut(line.Turn, "-turn-")

	raw, err := json.Marshal(line)
	if err != nil {
		m.fail(fmt.Errorf("marshal mirror line: %w", err))

		return
	}

	// The ONE chokepoint (Pitfall 9): redact before any sink.
	red, rerr := redact.Redact(raw)
	if rerr != nil {
		//nolint:err113 // dynamic error message
		red = []byte(redact.ScrubError(fmt.Errorf("%s", string(raw))))
	}

	red = append(red, '\n')

	werr := m.write(line.Session, red)
	if werr != nil {
		m.fail(werr)
	}
}

// write routes the redacted bytes to the session's sink (per-session mode
// opens lazily and caches; single-file mode writes the one writer).
func (m *Mirror) write(session string, red []byte) error {
	if m.single != nil {
		_, err := m.single.Write(red)

		return err //nolint:wrapcheck // passthrough to fail()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.handles[session]
	if !ok {
		f, err := os.OpenFile(filepath.Join(m.dir, session+".jsonl"),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePermOwnerOnly)
		if err != nil {
			return fmt.Errorf("open mirror %s: %w", session, err)
		}

		w = f
		m.handles[session] = w
	}

	_, err := w.Write(red)

	return err //nolint:wrapcheck // passthrough to fail()
}

// fail records a sink failure: logged once per DISTINCT error string, counted
// always, never fatal.
func (m *Mirror) fail(err error) {
	m.dropped.Add(1)

	if m.log == nil {
		m.log = slog.Default()
	}

	es := err.Error()
	if prev, _ := m.lastErrStr.Load().(string); prev != es {
		m.log.Error("audit: mirror line dropped", "error", es)
		m.lastErrStr.Store(es)
	}
}
