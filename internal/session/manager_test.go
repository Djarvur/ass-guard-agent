package session

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/redact"
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

func (redactorAdapter) Redact(b []byte) ([]byte, error) {
	return redact.Redact(b) //nolint:wrapcheck // test adapter
}
func (redactorAdapter) ScrubError(err error) string { return redact.ScrubError(err) }

// TestAppendUserMessageWritesJSONLine verifies AppendUserMessage writes one
// well-formed JSON line with the type discriminator + turnID + content.
func TestAppendUserMessageWritesJSONLine(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, "sess-1")

	err := m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: "hello"}})
	if err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}

	lines := readTranscriptLines(t, m.path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines; want 1", len(lines))
	}

	if lines[0][keyType] != userMessageType {
		t.Errorf("type = %v; want user_message", lines[0][keyType])
	}

	if lines[0]["turnID"] != "turn_1" {
		t.Errorf("turnID = %v; want turn_1", lines[0]["turnID"])
	}
}

// TestAppendBoundary verifies the boundary line carries cause + commandRef +
// turnID (D-08 — boundary markers are explicit transcript lines).
// TestAppendEngineDecision verifies the Phase-4 engine_decision line carries
// the type discriminator, the action in `name`, the signal in `input`, and the
// reason in `text` (ENG-02 — the single provenance-tagged stream).
func TestAppendEngineDecision(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "sess-1")

	err := m.AppendEngineDecision("turn_7", "continue", "text:impl-complete", "text-pattern matched")
	if err != nil {
		t.Fatalf("AppendEngineDecision: %v", err)
	}

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(lines) != 1 {
		t.Fatalf("got %d lines; want 1", len(lines))
	}

	l := lines[0]
	if l.Type != TypeEngineDecision {
		t.Errorf("Type = %q; want %q", l.Type, TypeEngineDecision)
	}

	if l.TurnID != "turn_7" {
		t.Errorf("TurnID = %q; want turn_7", l.TurnID)
	}

	if l.Name != "continue" {
		t.Errorf("Name(action) = %q; want continue", l.Name)
	}

	if string(l.Input) != `"text:impl-complete"` {
		t.Errorf("Input(signal) = %s; want \"text:impl-complete\"", l.Input)
	}

	if l.Text != "text-pattern matched" {
		t.Errorf("Text(reason) = %q; want text-pattern matched", l.Text)
	}
}

// TestAppendEngineDecisionNothing verifies the structural-safety cell records
// an ActionNothing decision (unmatched output triggers nothing — D-03); the
// audit log must prove it so a human investigating can confirm nothing fired.
func TestAppendEngineDecisionNothing(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "sess-1")

	err := m.AppendEngineDecision("turn_1", "nothing", "unmatched",
		"no pattern or handoff tool matched")
	if err != nil {
		t.Fatalf("AppendEngineDecision: %v", err)
	}

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if lines[0].Name != "nothing" || lines[0].Text == "" {
		t.Errorf("engine_decision nothing line not recorded faithfully: %+v", lines[0])
	}
}

func TestAppendBoundary(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, "sess-1")

	err := m.AppendBoundary(mutatingCommandBash, "toolCallId_abc", "turn_042")
	if err != nil {
		t.Fatalf("AppendBoundary: %v", err)
	}

	lines := readTranscriptLines(t, m.path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines; want 1", len(lines))
	}

	if lines[0][keyType] != kindBoundary {
		t.Errorf("type = %v; want boundary", lines[0][keyType])
	}

	if lines[0]["cause"] != mutatingCommandBash {
		t.Errorf("cause = %v; want mutating-command:Bash", lines[0]["cause"])
	}

	if lines[0]["commandRef"] != "toolCallId_abc" {
		t.Errorf("commandRef = %v; want toolCallId_abc", lines[0]["commandRef"])
	}
}

// TestAppendErrorScrubsMessage verifies the error line's message is scrubbed
// (LOG-03) but the component + recoverable flag are preserved.
func TestAppendErrorScrubsMessage(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, "sess-1")

	err := m.AppendError("turn_1", "provider", "send failed: sk-live-xxx", nil, false, "")
	if err != nil {
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
	t.Parallel()
	m := newTestManager(t, "sess-1")

	body := json.RawMessage(`{"authorization":"Bearer sk-test","headers":{"x-request-id":"abc"}}`)

	err := m.AppendRequestShaped("turn_1", body, "zcode", time.Now())
	if err != nil {
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
	t.Parallel()
	m := newTestManager(t, "sess-1")

	const n = 100

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			_ = m.AppendUserMessage("turn_x", []ContentBlock{{Type: blockText, Text: "msg"}})
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
	t.Parallel()
	m := newTestManager(t, "sess-1")
	_ = m.AppendUserMessage("t1", []ContentBlock{{Type: blockText, Text: "first"}})
	_ = m.AppendAssistantMessage("t1", "second")
	_ = m.AppendBoundary(mutatingCommandBash, "tc1", "t1")
	_ = m.AppendUserMessage("t2", []ContentBlock{{Type: blockText, Text: "third"}})

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(lines) != 4 {
		t.Fatalf("got %d lines; want 4", len(lines))
	}

	if lines[0].Type != userMessageType || lines[1].Type != "assistant_message" {
		t.Errorf("order wrong: %s, %s", lines[0].Type, lines[1].Type)
	}

	if lines[2].Type != kindBoundary {
		t.Errorf("line 2 type = %s; want boundary", lines[2].Type)
	}
}

// TestReadLastBoundary verifies ReadLastBoundary returns the most recent
// boundary line (or nil if none).
func TestReadLastBoundary(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "sess-1")
	if b, _ := m.ReadLastBoundary(); b != nil {
		t.Errorf("ReadLastBoundary on empty transcript = %v; want nil", b)
	}

	_ = m.AppendBoundary(mutatingCommandBash, "tc1", "t1")
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
	t.Parallel()
	dir := t.TempDir()

	m, err := NewManager(dir, "sess-1", redactorAdapter{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	defer func() { _ = m.Close() }()

	gi := dir + "/.ass-guard/.gitignore"

	raw := readFile(t, gi)
	if raw != "*\n!.gitignore\n" {
		t.Errorf(".gitignore content = %q; want exactly \"*\\n!.gitignore\\n\"", raw)
	}
}

// readTranscriptLines parses the transcript file into a slice of generic maps
// (for asserting on raw field values).
func readTranscriptLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	raw := readTranscriptRaw(t, path)

	var out []map[string]any

	for line := range strings.SplitSeq(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			continue
		}

		var m map[string]any

		err := json.Unmarshal([]byte(line), &m)
		if err != nil {
			t.Fatalf("malformed transcript line: %v (line=%q)", err, line)
		}

		out = append(out, m)
	}

	return out
}

// readTranscriptRaw returns the raw transcript bytes.
func readTranscriptRaw(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript %s: %v", path, err)
	}

	return string(b)
}

// readFile returns a file's content, failing the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(b)
}
