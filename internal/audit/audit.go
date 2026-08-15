package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/redact"
)

// AuditLogger subscribes to RequestShaped and writes one JSON line per event to
// its sink. The sink must NEVER be stdout (transport discipline).
type AuditLogger struct {
	mu   sync.Mutex
	sink io.Writer
}

// NewAuditLogger subscribes l to RequestShaped on bus and returns it. It panics
// if sink is os.Stdout (stdout is reserved for ACP frames — transport
// discipline). The subscriber drains its channel in a background goroutine; the
// goroutine exits when the bus is closed (bus.Close) or the process exits.
//
// NOTE: Plan 02-07 folds this AuditLogger into the unified TranscriptWriter
// (D-20 — one artifact). Until then this keeps the Phase-1 LOG-01 path working
// against the Phase-2 typed-channel bus.
// errSinkStdout is the locked AUD-02 guard's rejection error.
var errSinkStdout = errors.New("audit sink must not be stdout (transport discipline)")

// filePermOwnerOnly / dirPermOwnerOnly match the sink permission discipline
// (0600 files, 0700 dirs — owner-only, like the transcript + body store).
const (
	filePermOwnerOnly = 0o600
	dirPermOwnerOnly  = 0o700
)

// OpenFileSink resolves the audit sink (09-06, the SHARED opener — Pitfall 8:
// no os.OpenFile for audit anywhere else). Empty path or "-" → os.Stderr with
// a nil closer; the literal stdout TARGETS ("stdout", "/dev/stdout") are
// REJECTED (the locked AUD-02 guard, enforced for file targets too — stdout is
// reserved for ACP frames); any other path opens append-only at 0600.
func OpenFileSink(path string) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return os.Stderr, nil, nil
	}

	if path == "stdout" || path == "/dev/stdout" {
		return nil, nil, errSinkStdout
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePermOwnerOnly)
	if err != nil {
		return nil, nil, fmt.Errorf("open audit-log: %w", err)
	}

	return f, func() error { return f.Close() }, nil
}

// NewAuditLogger builds the Phase-1 tracer-path LOG-01 consumer: it
// subscribes to RequestShaped and writes each redacted verbatim body to sink.
// Panics when sink is os.Stdout (transport discipline).
func NewAuditLogger(bus *event.Bus, sink io.Writer) *AuditLogger {
	if sink == os.Stdout {
		panic("audit: sink must not be os.Stdout (transport discipline — stdout is reserved for ACP frames)")
	}

	l := &AuditLogger{sink: sink}

	ch := bus.Subscribe("RequestShaped", event.BufRequestShaped)

	go func() {
		for e := range ch {
			l.handle(e)
		}
	}()

	return l
}

func (l *AuditLogger) handle(e event.Event) {
	rs, ok := e.(event.RequestShaped)
	if !ok {
		return
	}

	redacted, err := redact.Redact(rs.VerbatimRequest)
	if err != nil {
		// Redaction failed (non-JSON / regex fallback). Best-effort: scrub errors
		// rather than dropping the audit line entirely.
		//nolint:err113 // dynamic error message
		redacted = []byte(redact.ScrubError(fmt.Errorf("%s", string(rs.VerbatimRequest))))
	}

	line := map[string]any{
		"timestamp": rs.Timestamp.UTC().Format(time.RFC3339Nano),
		"profile":   rs.Profile,
		"request":   json.RawMessage(redacted),
	}

	out, err := json.Marshal(line)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	_, _ = fmt.Fprintln(l.sink, string(out))
}
