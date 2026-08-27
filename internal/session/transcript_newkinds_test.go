package session //nolint:testpackage // internal package test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// countingRedactor is a Redactor fake that counts Redact invocations — the
// D-23 zero-call proof instrument: the raw-thinking append path must never
// reach the redactor, while every other kind must.
type countingRedactor struct {
	mu      sync.Mutex
	redacts int
}

func (c *countingRedactor) Redact(b []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.redacts++

	return b, nil
}

func (c *countingRedactor) ScrubError(err error) string { return err.Error() }

func (c *countingRedactor) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.redacts
}

// newCountingManager opens a Manager over a countingRedactor (the D-23
// observation seam).
func newCountingManager(t *testing.T) (*Manager, *countingRedactor) {
	t.Helper()

	red := &countingRedactor{}

	m, err := NewManager(t.TempDir(), "sess-nk", red)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	t.Cleanup(func() { _ = m.Close() })

	return m, red
}

// uuidV4Shape matches an RFC 4122 v4 UUID string (version nibble 4, variant
// 10xx) — the shape the in-repo crypto/rand construction produces.
var uuidV4Shape = regexp.MustCompile( //nolint:gochecknoglobals // test fixture
	`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestTranscriptNewKinds locks the Phase-16 extended transcript schema
// (D-20..D-23): three additive kinds with a provably redaction-exempt
// thinking path, provenance-complete local-command and compaction records,
// and replay tolerance for unknown kinds/fields.
func TestTranscriptNewKinds(t *testing.T) {
	t.Parallel()

	t.Run("raw_thinking payload round-trips byte-identical", func(t *testing.T) {
		t.Parallel()

		m, _ := newCountingManager(t)

		// Nested oddities chosen inside the two classes the pass-through
		// guarantee covers: whitespace INSIDE string tokens and EXISTING
		// unicode escapes — json.Marshal's compaction preserves both
		// byte-for-byte (it only rewrites whitespace between tokens).
		payload := json.RawMessage(`{"thinking":"line one\n  indented\tkept","unicode":"café ✓ \u00e9 kept","nested":{"arr":[1,2,3],"deep":{"ws":"  padded  "}}}`)

		err := m.AppendRawThinking("turn_1", "glm-5.2", payload)
		if err != nil {
			t.Fatalf("AppendRawThinking: %v", err)
		}

		lines, err := m.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		var got *Line

		for i := range lines {
			if lines[i].Type == TypeRawThinking {
				got = &lines[i]
			}
		}

		if got == nil {
			t.Fatal("no raw_thinking line on disk")
		}

		if !bytes.Equal(payload, got.Content) {
			t.Errorf("payload round-trip NOT byte-identical:\n orig: %s\nround: %s", payload, got.Content)
		}

		if got.Model != "glm-5.2" {
			t.Errorf("provider attribution Model = %q; want glm-5.2", got.Model)
		}

		if got.TurnID != "turn_1" {
			t.Errorf("TurnID = %q; want turn_1", got.TurnID)
		}
	})

	t.Run("raw_thinking never touches the redactor; redacted control does", func(t *testing.T) {
		t.Parallel()

		m, red := newCountingManager(t)

		err := m.AppendRawThinking("turn_1", "glm-5.2", json.RawMessage(`{"thinking":"bytes the redactor must not walk"}`))
		if err != nil {
			t.Fatalf("AppendRawThinking: %v", err)
		}

		if got := red.calls(); got != 0 {
			t.Fatalf("redactor invoked %d time(s) on the raw_thinking path; want 0 (D-23)", got)
		}

		// Control: an existing redacted kind must observe at least one call —
		// proves the fake sits on the real path (not a dead counter).
		err = m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: "hello"}})
		if err != nil {
			t.Fatalf("AppendUserMessage: %v", err)
		}

		if red.calls() < 1 {
			t.Fatal("control redacted kind recorded zero redactor calls — counter not observing the path")
		}
	})

	t.Run("local_command records key, verbatim args, source chain, outcome", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "sess-lc")

		// Verbatim: double spaces + mixed quoting must survive untouched —
		// no shell re-quoting, no normalization (D-22).
		args := `--flag="a b"   'c  d'`

		err := m.AppendLocalCommand("turn_2", "review", args, "expanded", []string{"builtin", "skills"})
		if err != nil {
			t.Fatalf("AppendLocalCommand: %v", err)
		}

		lines, err := m.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		var got *Line

		for i := range lines {
			if lines[i].Type == TypeLocalCommand {
				got = &lines[i]
			}
		}

		if got == nil {
			t.Fatal("no local_command line on disk")
		}

		if got.Name != "review" {
			t.Errorf("command key Name = %q; want review", got.Name)
		}

		if got.Args != args {
			t.Errorf("verbatim args mutated:\n orig: %q\nround: %q", args, got.Args)
		}

		wantChain := []string{"builtin", "skills"}
		if !reflect.DeepEqual(got.SourceChain, wantChain) {
			t.Errorf("SourceChain = %v; want %v (resolution order preserved)", got.SourceChain, wantChain)
		}

		if got.Expansion != "expanded" {
			t.Errorf("Expansion = %q; want expanded", got.Expansion)
		}

		if got.TurnID != "turn_2" {
			t.Errorf("TurnID = %q; want turn_2", got.TurnID)
		}
	})

	t.Run("compaction records fresh boundary id, usage snapshot, pointers", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "sess-cmp")

		const (
			inTokens    = 1200
			outTokens   = 340
			cacheTokens = 55
		)

		err := m.AppendCompaction("turn_3", "line:41", "line:42", inTokens, outTokens, cacheTokens)
		if err != nil {
			t.Fatalf("AppendCompaction: %v", err)
		}

		// Freshness: a second boundary gets its OWN id.
		err = m.AppendCompaction("turn_4", "line:42", "line:43", inTokens, outTokens, cacheTokens)
		if err != nil {
			t.Fatalf("AppendCompaction(2): %v", err)
		}

		lines, err := m.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		var ids []string

		for i := range lines {
			l := &lines[i]
			if l.Type != TypeCompaction {
				continue
			}

			if !uuidV4Shape.MatchString(l.BoundaryID) {
				t.Errorf("BoundaryID = %q; want an RFC 4122 v4 UUID", l.BoundaryID)
			}

			if l.InputTokens != inTokens || l.OutputTokens != outTokens || l.CacheTokens != cacheTokens {
				t.Errorf("usage snapshot = in:%d out:%d cache:%d; want %d/%d/%d",
					l.InputTokens, l.OutputTokens, l.CacheTokens, inTokens, outTokens, cacheTokens)
			}

			if l.PreRef != "line:41" || l.PostRef != "line:42" {
				t.Errorf("pre/post pointers = %q/%q; want line:41/line:42", l.PreRef, l.PostRef)
			}

			if l.TurnID != "turn_3" {
				t.Errorf("TurnID = %q; want turn_3", l.TurnID)
			}

			ids = append(ids, l.BoundaryID)
		}

		if len(ids) != 2 {
			t.Fatalf("got %d compaction lines; want 2", len(ids))
		}

		if ids[0] == ids[1] {
			t.Errorf("boundary ids not fresh per append: %q repeated", ids[0])
		}
	})

	t.Run("replay tolerates new kinds, unknown kinds, unknown fields", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "fixture.jsonl")

		// One line of each new kind + one UNKNOWN future kind + one known
		// kind carrying an unknown extra field (D-20 tolerance rule).
		fixture := strings.Join([]string{
			`{"type":"raw_thinking","turnID":"t1","timestamp":"2026-08-27T00:00:00Z","model":"glm-5.2","content":{"thinking":"  padded "}}`,
			`{"type":"local_command","turnID":"t1","timestamp":"2026-08-27T00:00:01Z","name":"review","args":"--x 1","sourceChain":["builtin"],"expansion":"expanded"}`,
			`{"type":"compaction","turnID":"t1","timestamp":"2026-08-27T00:00:02Z","boundaryID":"0b9e6c1d-7a42-4b8e-9c1d-2f3a4b5c6d7e","inputTokens":10,"outputTokens":20,"cacheTokens":30,"preRef":"line:9","postRef":"line:10"}`,
			`{"type":"quantum_teleport","turnID":"t1","timestamp":"2026-08-27T00:00:03Z","qubits":9}`,
			`{"type":"user_message","turnID":"t1","timestamp":"2026-08-27T00:00:04Z","content":[{"type":"text","text":"hi"}],"quantumField":"unknown-future"}`,
		}, "\n") + "\n"

		if err := os.WriteFile(path, []byte(fixture), filePermOwner); err != nil {
			t.Fatalf("write fixture: %v", err)
		}

		lines, err := readTranscriptFile(path)
		if err != nil {
			t.Fatalf("readTranscriptFile on fixture with new/unknown kinds: %v", err)
		}

		if len(lines) != 5 {
			t.Fatalf("got %d lines; want 5 (no line dropped)", len(lines))
		}

		wantTypes := []string{TypeRawThinking, TypeLocalCommand, TypeCompaction, "quantum_teleport", userMessageType}
		for i, want := range wantTypes {
			if lines[i].Type != want {
				t.Errorf("line %d type = %q; want %q (discriminator intact)", i, lines[i].Type, want)
			}
		}

		// The known kind's payload still parses despite the unknown sibling
		// field on the same line.
		if got := extractText(&lines[4]); got != "hi" {
			t.Errorf("user_message text = %q; want hi (payload intact)", got)
		}
	})
}
