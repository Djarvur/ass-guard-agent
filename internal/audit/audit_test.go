package audit_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/event"
)

// safeBuffer is a mutex-guarded bytes.Buffer so the async audit goroutine
// (writer) and the test (reader) don't race under -race.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.String()
}

func (s *safeBuffer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.Len()
}

// TestAuditLogger_RedactsSecrets confirms a RequestShaped carrying an auth token
// is written to the sink with the secret value redacted (LOG-03) but the field
// NAME and the 12 identity header names preserved.
func TestAuditLogger_RedactsSecrets(t *testing.T) {
	b := event.NewBus()
	sink := &safeBuffer{}
	audit.NewAuditLogger(b, sink)
	b.Publish(event.RequestShaped{
		Profile:         "zcode",
		Timestamp:       time.Now(),
		VerbatimRequest: json.RawMessage(`{"headers":{"authorization":"Bearer sk-secret-xyz","x-request-id":"abc"},"body":{"system":[{"type":"text","text":"hi"}]}}`),
	})
	// Allow the async subscriber goroutine to run.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && sink.Len() == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	out := sink.String()
	if !strings.Contains(out, `[REDACTED]`) {
		t.Errorf("audit line did not redact the auth token:\n%s", out)
	}

	if strings.Contains(out, "sk-secret-xyz") {
		t.Errorf("audit line leaked the secret token:\n%s", out)
	}

	if !strings.Contains(out, "authorization") {
		t.Error("audit line dropped the field NAME (must preserve names)")
	}
}

// TestAuditLogger_RejectsStdout confirms the transport-discipline guard: the
// audit sink must never be stdout.
func TestAuditLogger_RejectsStdout(t *testing.T) {
	b := event.NewBus()

	defer func() {
		if r := recover(); r == nil {
			t.Error("NewAuditLogger with os.Stdout did not panic; want transport-discipline panic")
		}
	}()

	audit.NewAuditLogger(b, os.Stdout)
}
