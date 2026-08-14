package audit

import (
	"encoding/json"
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
