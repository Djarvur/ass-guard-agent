package session

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/djarvur/ass-guard-agent/internal/redact"
)

// newTestManager opens a Manager in a temp dir with the real redactor.
func newTestManager(t *testing.T, sessionID string) *Manager {
	t.Helper()
	m, err := NewManager(t.TempDir(), sessionID, redactorAdapter{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

// redactorAdapter adapts internal/redact to the Manager's Redactor interface.
type redactorAdapter struct{}

func (redactorAdapter) Redact(b []byte) ([]byte, error) { return redact.Redact(b) }
func (redactorAdapter) ScrubError(err error) string      { return redact.ScrubError(err) }

// TestAppendUserMessageWritesJSONLine verifies AppendUserMessage writes one
// well-formed JSON line with the type discriminator + turnID + content.
func TestAppendUserMessageWritesJSONLine(t *testing.T) {
	m := newTestManager(t, "sess-1")
	if err := m.AppendUserMessage("turn_1", []ContentBlock{{Type: "text", Text: "hello"}}); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}
	lines := readTranscriptLines(t, m.path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines; want 1", len(lines))
	}
	if lines[0]["type"] != "user_message" {
		t.Errorf("type = %v; want user_message", lines[0]["type"])
	}
	if lines[0]["turnID"] != "turn_1" {
		t.Errorf("turnID = %v; want turn_1", lines[0]["turnID"])
	}
}

// TestAppendBoundary verifies the boundary line carries cause + commandRef +
// turnID (D-08 — boundary markers are explicit transcript lines).
func TestAppendBoundary(t *testing.T) {
	m := newTestManager(t, "sess-1")
	if err := m.AppendBoundary("mutating-command:Bash", "toolCallId_abc", "turn_042"); err != nil {
		t.Fatalf("AppendBoundary: %v", err)
	}
	lines := readTranscriptLines(t, m.path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines; want 1", len(lines))
	}
	if lines[0]["type"] != "boundary" {
		t.Errorf("type = %v; want boundary", lines[0]["type"])
	}
	if lines[0]["cause"] != "mutating-command:Bash" {
		t.Errorf("cause = %v; want mutating-command:Bash", lines[0]["cause"])
	}
	if lines[0]["commandRef"] != "toolCallId_abc" {
		t.Errorf("commandRef = %v; want toolCallId_abc", lines[0]["commandRef"])
	}
}

// TestAppendErrorScrubsMessage verifies the error line's message is scrubbed
// (LOG-03) but the component + recoverable flag are preserved.
func TestAppendErrorScrubsMessage(t *testing.T) {
	m := newTestManager(t, "sess-1")
	if err := m.AppendError("turn_1", "provider", "send failed: sk-live-xxx", nil, false, ""); err != nil {
		t.Fatalf("AppendError: %v", err)
	}
	lines := readTranscriptLines(t, m.path)
	if lines[0]["component"] != "provider" {
		t.Errorf("component = %v; want provider", lines[0]["component"])
	}
	msg, _ := lines[0]["message"].(string)
	if strings.Contains(msg, "sk-live-xxx") {
		t.Errorf("error message leaked the secret: %q", msg)
	}
	if !strings.Contains(msg, "send failed") {
		t.Errorf("error message lost the non-secret context: %q", msg)
	}
}

// TestRedactionOnRequestShaped verifies a request_shaped line whose verbatim
// request carries an auth token is redacted on disk (LOG-03) — auth value
// becomes [REDACTED], the field NAME + structure preserved.
func TestRedactionOnRequestShaped(t *testing.T) {
	m := newTestManager(t, "sess-1")
	body := json.RawMessage(`{"authorization":"Bearer sk-test","headers":{"x-request-id":"abc"}}`)
	if err := m.AppendRequestShaped("turn_1", body, "zcode", time.Now()); err != nil {
		t.Fatalf("AppendRequestShaped: %v", err)
	}
	raw := readTranscriptRaw(t, m.path)
	if strings.Contains(raw, "sk-test") {
		t.Errorf("transcript leaked the secret token on disk:\n%s", raw)
	}
	if !strings.Contains(raw, "[REDACTED]") {
		t.Errorf("transcript did not redact the auth value:\n%s", raw)
	}
	if !strings.Contains(raw, "authorization") {
		t.Errorf("transcript dropped the field name (must preserve names):\n%s", raw)
	}
}

// TestConcurrency verifies 100 goroutines appending to one Manager produce 100
// well-formed JSONL lines (no interleaving, no truncation) — SESS-06 sole-owner.
func TestConcurrency(t *testing.T) {
	m := newTestManager(t, "sess-1")
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.AppendUserMessage("turn_x", []ContentBlock{{Type: "text", Text: "msg"}})
		}(i)
	}
	wg.Wait()
	lines := readTranscriptLines(t, m.path)
	if len(lines) != n {
		t.Errorf("got %d lines; want %d", len(lines), n)
	}
}

// TestReadAllOrder verifies ReadAll returns lines in append order.
func TestReadAllOrder(t *testing.T) {
	m := newTestManager(t, "sess-1")
	_ = m.AppendUserMessage("t1", []ContentBlock{{Type: "text", Text: "first"}})
	_ = m.AppendAssistantMessage("t1", "second")
	_ = m.AppendBoundary("mutating-command:Bash", "tc1", "t1")
	_ = m.AppendUserMessage("t2", []ContentBlock{{Type: "text", Text: "third"}})

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(lines) != 4 {
		t.Fatalf("got %d lines; want 4", len(lines))
	}
	if lines[0].Type != "user_message" || lines[1].Type != "assistant_message" {
		t.Errorf("order wrong: %s, %s", lines[0].Type, lines[1].Type)
	}
	if lines[2].Type != "boundary" {
		t.Errorf("line 2 type = %s; want boundary", lines[2].Type)
	}
}

// TestReadLastBoundary verifies ReadLastBoundary returns the most recent
// boundary line (or nil if none).
func TestReadLastBoundary(t *testing.T) {
	m := newTestManager(t, "sess-1")
	if b, _ := m.ReadLastBoundary(); b != nil {
		t.Errorf("ReadLastBoundary on empty transcript = %v; want nil", b)
	}
	_ = m.AppendBoundary("mutating-command:Bash", "tc1", "t1")
	_ = m.AppendBoundary("mutating-command:Write", "tc2", "t2")
	b, err := m.ReadLastBoundary()
	if err != nil {
		t.Fatalf("ReadLastBoundary: %v", err)
	}
	if b == nil || b.Cause != "mutating-command:Write" {
		t.Errorf("ReadLastBoundary = %+v; want cause mutating-command:Write", b)
	}
}

// TestSelfGitignore verifies .ass-guard/ is self-gitignoring: on first run it
// creates .ass-guard/.gitignore with exactly `*\n!.gitignore\n` (D-07).
func TestSelfGitignore(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir, "sess-1", redactorAdapter{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Close()
	gi := dir + "/.ass-guard/.gitignore"
	raw := readFile(t, gi)
	if raw != "*\n!.gitignore\n" {
		t.Errorf(".gitignore content = %q; want exactly \"*\\n!.gitignore\\n\"", raw)
	}
}
