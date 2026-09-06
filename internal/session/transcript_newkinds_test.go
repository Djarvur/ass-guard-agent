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

// Reused fixture strings (goconst).
const (
	fixtureModelSlug     = "glm-5.2"
	fixtureUserText      = "hello"
	fixtureProfileName   = "zcode"
	fixtureSourceBuiltin = "builtin"
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
var uuidV4Shape = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestTranscriptNewKinds locks the Phase-16 extended transcript schema
// (D-20..D-23): three additive kinds with a provably redaction-exempt
// thinking path, provenance-complete local-command and compaction records,
// and replay tolerance for unknown kinds/fields. All five behavior cases run
// in this one test (the -race gate runs it as a unit).
func TestTranscriptNewKinds(t *testing.T) {
	t.Parallel()

	t.Run("raw_thinking payload round-trips byte-identical", testRawThinkingByteIdentity)
	t.Run("raw_thinking never touches the redactor; redacted control does", testRawThinkingRedactorZeroCalls)
	t.Run("local_command records key, verbatim args, source chain, outcome", testLocalCommandInvocationRecord)
	t.Run("compaction records fresh boundary id, usage snapshot, pointers", testCompactionBoundaryRecord)
	t.Run("compaction summary payload rides the redacted path and round-trips", testCompactionSummaryPayload)
	t.Run("replay tolerates new kinds, unknown kinds, unknown fields", testReplayToleratesNewKinds)
}

// testRawThinkingByteIdentity proves the D-23 passthrough: the provider's
// thinking bytes survive a marshal→readTranscriptFile round-trip EXACTLY.
func testRawThinkingByteIdentity(t *testing.T) {
	t.Parallel()

	m, _ := newCountingManager(t)

	// Nested oddities chosen inside the two classes the pass-through
	// guarantee covers: whitespace INSIDE string tokens and EXISTING
	// unicode escapes — json.Marshal's compaction preserves both
	// byte-for-byte (it only rewrites whitespace between tokens).
	payload := json.RawMessage(`{"thinking":"line one\n  indented\tkept",` +
		`"unicode":"café ✓ \u00e9 kept","nested":{"arr":[1,2,3],"deep":{"ws":"  padded  "}}}`)

	err := m.AppendRawThinking("turn_1", fixtureModelSlug, payload)
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

	if got.Model != fixtureModelSlug {
		t.Errorf("provider attribution Model = %q; want %s", got.Model, fixtureModelSlug)
	}

	if got.TurnID != "turn_1" {
		t.Errorf("TurnID = %q; want turn_1", got.TurnID)
	}
}

// testRawThinkingRedactorZeroCalls is the D-23 counting proof: zero Redact
// calls on the thinking path, at least one on a redacted control kind.
func testRawThinkingRedactorZeroCalls(t *testing.T) {
	t.Parallel()

	m, red := newCountingManager(t)

	payload := json.RawMessage(`{"thinking":"bytes the redactor must not walk"}`)

	err := m.AppendRawThinking("turn_1", fixtureModelSlug, payload)
	if err != nil {
		t.Fatalf("AppendRawThinking: %v", err)
	}

	if got := red.calls(); got != 0 {
		t.Fatalf("redactor invoked %d time(s) on the raw_thinking path; want 0 (D-23)", got)
	}

	// Control: an existing redacted kind must observe at least one call —
	// proves the fake sits on the real path (not a dead counter).
	err = m.AppendUserMessage("turn_1", []ContentBlock{{Type: blockText, Text: fixtureUserText}})
	if err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}

	if red.calls() < 1 {
		t.Fatal("control redacted kind recorded zero redactor calls — counter not observing the path")
	}
}

// testLocalCommandInvocationRecord proves the D-22 full invocation record:
// key + verbatim args + resolution-source chain + expansion outcome.
func testLocalCommandInvocationRecord(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "sess-lc")

	// Verbatim: double spaces + mixed quoting must survive untouched —
	// no shell re-quoting, no normalization (D-22).
	args := `--flag="a b"   'c  d'`

	err := m.AppendLocalCommand("turn_2", "review", args, "expanded", []string{fixtureSourceBuiltin, "skills"})
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

	wantChain := []string{fixtureSourceBuiltin, "skills"}
	if !reflect.DeepEqual(got.SourceChain, wantChain) {
		t.Errorf("SourceChain = %v; want %v (resolution order preserved)", got.SourceChain, wantChain)
	}

	if got.Expansion != "expanded" {
		t.Errorf("Expansion = %q; want expanded", got.Expansion)
	}

	if got.TurnID != "turn_2" {
		t.Errorf("TurnID = %q; want turn_2", got.TurnID)
	}
}

// testCompactionBoundaryRecord proves the D-21 rich boundary record: fresh
// UUID boundary id + token-usage snapshot + pre/post transcript pointers.
func testCompactionBoundaryRecord(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, "sess-cmp")

	const (
		inTokens    = 1200
		outTokens   = 340
		cacheTokens = 55
	)

	err := m.AppendCompaction("turn_3", "line:41", "line:42", "", inTokens, outTokens, cacheTokens)
	if err != nil {
		t.Fatalf("AppendCompaction: %v", err)
	}

	// Freshness: a second boundary gets its OWN id.
	err = m.AppendCompaction("turn_4", "line:42", "line:43", "", inTokens, outTokens, cacheTokens)
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

		ids = append(ids, l.BoundaryID)
	}

	if len(ids) != 2 {
		t.Fatalf("got %d compaction lines; want 2", len(ids))
	}

	if ids[0] == ids[1] {
		t.Errorf("boundary ids not fresh per append: %q repeated", ids[0])
	}

	// Field scoping: the FIRST boundary carries turn_3's pointers.
	first := lines[0]
	if first.TurnID != "turn_3" || first.PreRef != "line:41" || first.PostRef != "line:42" {
		t.Errorf("first boundary = turn:%q %q→%q; want turn_3 line:41→line:42",
			first.TurnID, first.PreRef, first.PostRef)
	}
}

// testCompactionSummaryPayload proves the Phase-19 summary extension of the
// D-21 marker (19-03 Task 1): the summary text rides the marker line through
// the REDACTED append path (16-D-23 scoping — only raw_thinking is exempt),
// round-trips exactly, and pre-field readers parse a marker WITHOUT the
// summary field with an empty Summary and no error (D-20 additive-only).
func testCompactionSummaryPayload(t *testing.T) {
	t.Parallel()

	// Round-trip + redacted-path case: one append on a counting manager, so
	// the call count is exact (not "at least one").
	m, red := newCountingManager(t)

	// Deliberately awkward text: newlines, quotes, unicode, JSON-significant
	// characters — the round-trip must preserve it EXACTLY.
	summary := "Session summary:\n- fixed the \"login\" flow\n- touched café/*.go ✓"

	if red.calls() != 0 {
		t.Fatalf("pre-append redactor calls = %d; want 0 (fixture broken)", red.calls())
	}

	err := m.AppendCompaction("turn_s", "line:7", "line:8", summary, 900, 120, 42)
	if err != nil {
		t.Fatalf("AppendCompaction: %v", err)
	}

	if got := red.calls(); got != 1 {
		t.Fatalf("redactor invoked %d time(s) on the compaction append; want exactly 1 "+
			"(the marker rides the REDACTED path — D-23's exemption is raw_thinking-only)", got)
	}

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var got *Line

	for i := range lines {
		if lines[i].Type == TypeCompaction {
			got = &lines[i]
		}
	}

	if got == nil {
		t.Fatal("no compaction line on disk")
	}

	if got.Summary != summary {
		t.Errorf("summary round-trip mutated:\n orig: %q\nround: %q", summary, got.Summary)
	}

	// The 16-02 field set rides along unchanged (additive extension).
	if got.TurnID != "turn_s" || got.PreRef != "line:7" || got.PostRef != "line:8" ||
		got.InputTokens != 900 || got.OutputTokens != 120 || got.CacheTokens != 42 {
		t.Errorf("16-02 field set drifted: turn:%q %q→%q in:%d out:%d cache:%d",
			got.TurnID, got.PreRef, got.PostRef, got.InputTokens, got.OutputTokens, got.CacheTokens)
	}

	if !uuidV4Shape.MatchString(got.BoundaryID) {
		t.Errorf("BoundaryID = %q; want an RFC 4122 v4 UUID", got.BoundaryID)
	}

	// On-disk field name is `summary` (omitempty — absent when empty): read
	// the raw file bytes, not the re-marshaled struct.
	raw, err := os.ReadFile(m.Path())
	if err != nil {
		t.Fatalf("read transcript file: %v", err)
	}

	if !strings.Contains(string(raw), `"summary":`) {
		t.Errorf("on-disk line carries no summary field; raw: %s", raw)
	}

	// Field-tolerant parse: a PRE-field marker fixture (16-02 shape, no
	// summary key) parses with an empty Summary and no error.
	path := filepath.Join(t.TempDir(), "prefixeld.jsonl")

	fixture := `{"type":"compaction","turnID":"t9","timestamp":"2026-08-27T00:00:02Z",` +
		`"boundaryID":"0b9e6c1d-7a42-4b8e-9c1d-2f3a4b5c6d7e","inputTokens":10,` +
		`"outputTokens":20,"cacheTokens":30,"preRef":"line:9","postRef":"line:10"}` + "\n"

	if werr := os.WriteFile(path, []byte(fixture), filePermOwner); werr != nil {
		t.Fatalf("write pre-field fixture: %v", werr)
	}

	preLines, rerr := readTranscriptFile(path)
	if rerr != nil {
		t.Fatalf("readTranscriptFile on pre-field compaction marker: %v", rerr)
	}

	if len(preLines) != 1 || preLines[0].Type != TypeCompaction {
		t.Fatalf("pre-field marker parse: %d line(s), type %q; want 1 compaction", len(preLines), preLines[0].Type)
	}

	if preLines[0].Summary != "" {
		t.Errorf("pre-field marker Summary = %q; want empty (field-tolerant parse)", preLines[0].Summary)
	}
}

// testReplayToleratesNewKinds pins the D-20 tolerance rule: the replay
// reader parses new kinds, UNKNOWN future kinds, and unknown fields on known
// kinds without error, discriminators intact.
func testReplayToleratesNewKinds(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fixture.jsonl")

	// One line of each new kind + one UNKNOWN future kind + one known
	// kind carrying an unknown extra field (D-20 tolerance rule).
	fixture := strings.Join([]string{
		`{"type":"raw_thinking","turnID":"t1","timestamp":"2026-08-27T00:00:00Z",` +
			`"model":"glm-5.2","content":{"thinking":"  padded "}}`,
		`{"type":"local_command","turnID":"t1","timestamp":"2026-08-27T00:00:01Z",` +
			`"name":"review","args":"--x 1","sourceChain":["builtin"],"expansion":"expanded"}`,
		`{"type":"compaction","turnID":"t1","timestamp":"2026-08-27T00:00:02Z",` +
			`"boundaryID":"0b9e6c1d-7a42-4b8e-9c1d-2f3a4b5c6d7e","inputTokens":10,` +
			`"outputTokens":20,"cacheTokens":30,"preRef":"line:9","postRef":"line:10"}`,
		`{"type":"quantum_teleport","turnID":"t1","timestamp":"2026-08-27T00:00:03Z","qubits":9}`,
		`{"type":"user_message","turnID":"t1","timestamp":"2026-08-27T00:00:04Z",` +
			`"content":[{"type":"text","text":"hi"}],"quantumField":"unknown-future"}`,
	}, "\n") + "\n"

	err := os.WriteFile(path, []byte(fixture), filePermOwner)
	if err != nil {
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
}

// TestProjectorToleratesNewKinds pins D-20's additive-only guarantee
// end-to-end: a transcript interleaved with the STILL-INERT Phase-16 kinds
// (local_command, compaction, foreign-turn raw_thinking) PLUS an unknown
// future kind projects a window IDENTICAL to the same transcript without
// them.
//
// Activation status: compaction stays inert until Phase 19 (reset-point
// class). raw_thinking within the PROJECTED turn is NO LONGER inert — Phase
// 21 (21-03, PAR-05) activated the thinking fold (see TestProjector_Thinking*
// + TestThinkingGolden); only foreign-turn thinking lines remain inert here
// (a subagent's or prior turn's thinking never leaks into the current turn's
// window). A future dispatch change that breaks the remaining inertness
// fails loudly here.
func TestProjectorToleratesNewKinds(t *testing.T) {
	t.Parallel()

	plain := newTestManager(t, "sess-plain")
	appendToleratedTurn(t, plain, false)

	mixed := newTestManager(t, "sess-mixed")
	appendToleratedTurn(t, mixed, true)

	winPlain, err := NewProjector(&profile.Profile{Name: fixtureProfileName}, plain).Project("turn_t")
	if err != nil {
		t.Fatalf("Project(plain): %v", err)
	}

	winMixed, err := NewProjector(&profile.Profile{Name: fixtureProfileName}, mixed).Project("turn_t")
	if err != nil {
		t.Fatalf("Project(mixed): %v", err)
	}

	if len(winPlain) == 0 {
		t.Fatal("empty plain projection — fixture broken")
	}

	if !reflect.DeepEqual(winPlain, winMixed) {
		t.Errorf("projected window changed by inert kinds:\nplain: %+v\nmixed: %+v", winPlain, winMixed)
	}
}

// appendToleratedTurn writes the same two-turn content into the transcript;
// when interleave is true the still-inert kinds + one unknown kind thread
// through the IDENTICAL content — including a compaction line sitting where
// a boundary would reset (proving compaction is NOT a reset point today) and
// a FOREIGN-TURN thinking line (21-03: raw_thinking inside the projected
// turn is active PAR-05 fold territory — covered by the thinking battery —
// so only the foreign-turn placement remains in this inertness pin).
func appendToleratedTurn(t *testing.T, m *Manager, interleave bool) {
	t.Helper()

	thinking := json.RawMessage(`{"type":"thinking","thinking":"scratch","signature":"sig-nk"}`)
	toolInput := json.RawMessage(`{"cmd":"ls"}`)
	toolOutput := json.RawMessage(`"done"`)

	if interleave {
		mustAppend(t, m.AppendRawThinking("turn_0", fixtureModelSlug, thinking), "AppendRawThinking")
	}

	mustAppend(t,
		m.AppendUserMessage("turn_0", []ContentBlock{{Type: blockText, Text: fixtureUserText}}),
		"AppendUserMessage")

	mustAppend(t, m.AppendAssistantMessage("turn_0", "prior answer"), "AppendAssistantMessage")

	if interleave {
		// Sits where a boundary WOULD reset the window; inert until Phase 19.
		mustAppend(t, m.AppendCompaction("turn_0", "line:2", "line:3", "", 10, 20, 30), "AppendCompaction")
	}

	if interleave {
		mustAppend(t,
			m.AppendLocalCommand("turn_t", "review", "--x 1", "expanded", []string{fixtureSourceBuiltin}),
			"AppendLocalCommand")
	}

	mustAppend(t,
		m.AppendUserMessage("turn_t", []ContentBlock{{Type: blockText, Text: "current intent"}}),
		"AppendUserMessage")

	mustAppend(t, m.AppendToolCall("turn_t", "tc1", "Bash", toolInput), "AppendToolCall")

	// 21-03 (PAR-05): NO turn_t raw_thinking interleave here anymore — the
	// kind is ACTIVE in projection (the thinking fold, see the TestProjector_
	// Thinking* battery); interleaving it would legitimately change the window.
	// The foreign-turn line above carries this kind's inertness pin.

	mustAppend(t, m.AppendToolResult("turn_t", "tc1", toolOutput, false), "AppendToolResult")

	mustAppend(t, m.AppendAssistantMessage("turn_t", "final text"), "AppendAssistantMessage")

	if interleave {
		appendRawUnknownLine(t, m, "turn_t")
	}
}

// mustAppend fails the test when a fixture append errors (keeps the fixture
// builders free of per-call error scaffolding).
func mustAppend(t *testing.T, err error, what string) {
	t.Helper()

	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// appendRawUnknownLine appends an unknown-future-kind line straight to the
// JSONL file (no appender exists for unknown kinds by design — readers must
// tolerate them, D-20).
func appendRawUnknownLine(t *testing.T, m *Manager, turnID string) {
	t.Helper()

	line := `{"type":"quantum_teleport","turnID":"` + turnID + `",` +
		`"timestamp":"2026-08-27T00:00:09Z","qubits":9}` + "\n"

	f, err := os.OpenFile(m.Path(), os.O_APPEND|os.O_WRONLY, filePermOwner)
	if err != nil {
		t.Fatalf("open transcript for unknown-kind line: %v", err)
	}

	defer func() { _ = f.Close() }()

	_, werr := f.WriteString(line)
	if werr != nil {
		t.Fatalf("write unknown-kind line: %v", werr)
	}
}

// --- 21-05 (PAR-06/D-09) image-variant ContentBlock battery ---

// Image-variant fixture values (unique markers).
const (
	imgRefPath   = "/ws/.ass-guard/images/abc123.png"
	imgMediaTyp  = "image/png"
	imgOrigW     = 9000
	imgOrigH     = 6000
	imgOrigSize  = 1234567
	imgScaledW   = 1568
	imgScaledH   = 1045
	imgTurnIDStr = "turn_img"
)

// TestContentBlockRoundTrip pins the omitempty discipline on the ContentBlock
// image-variant fields (21-05): a PRE-CHANGE-shaped text-only block and a
// pre-change-shaped user_message line marshal byte-identically after the
// field additions, while the image variant carries Ref + metadata + resize
// provenance through a marshal→read round-trip.
//
//nolint:gocognit,gocyclo,cyclop,funlen // three subtests, branch-dense assertions
func TestContentBlockRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("text-only block marshals byte-identically", func(t *testing.T) {
		t.Parallel()

		got, err := json.Marshal(ContentBlock{Type: blockText, Text: fixtureUserText})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}

		// The EXACT pre-change shape: type + text only — the image fields'
		// omitempty must not add a single byte.
		if want := `{"type":"text","text":"hello"}`; string(got) != want {
			t.Errorf("text-only marshal = %s; want %s (byte-identical)", got, want)
		}
	})

	t.Run("pre-change user_message line round-trips unchanged", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "prechange.jsonl")

		fixture := `{"type":"user_message","turnID":"t1","timestamp":"2026-09-01T00:00:00Z",` +
			`"content":[{"type":"text","text":"hi"}]}` + "\n"

		werr := os.WriteFile(path, []byte(fixture), filePermOwner)
		if werr != nil {
			t.Fatalf("write fixture: %v", werr)
		}

		lines, rerr := readTranscriptFile(path)
		if rerr != nil {
			t.Fatalf("readTranscriptFile: %v", rerr)
		}

		if len(lines) != 1 || lines[0].Type != userMessageType {
			t.Fatalf("round-trip lost the line: %+v", lines)
		}

		var blocks []ContentBlock

		uerr := json.Unmarshal(lines[0].Content, &blocks)
		if uerr != nil {
			t.Fatalf("unmarshal content: %v", uerr)
		}

		if len(blocks) != 1 || blocks[0].Type != blockText || blocks[0].Text != "hi" {
			t.Errorf("pre-change block drifted: %+v; want text hi", blocks[0])
		}

		remarshaled, merr := json.Marshal(blocks[0])
		if merr != nil {
			t.Fatalf("re-marshal: %v", merr)
		}

		if want := `{"type":"text","text":"hi"}`; string(remarshaled) != want {
			t.Errorf("re-marshal = %s; want %s", remarshaled, want)
		}
	})

	t.Run("image variant carries ref, metadata, provenance", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "sess-imgcb")

		blocks := []ContentBlock{{
			Type:       blockImage,
			DataRef:    imgRefPath,
			MediaType:  imgMediaTyp,
			Width:      imgScaledW,
			Height:     imgScaledH,
			OrigWidth:  imgOrigW,
			OrigHeight: imgOrigH,
			OrigSize:   imgOrigSize,
			Scaled:     true,
		}}

		aerr := m.AppendUserMessage(imgTurnIDStr, blocks)
		if aerr != nil {
			t.Fatalf("AppendUserMessage: %v", aerr)
		}

		lines, rerr := m.ReadAll()
		if rerr != nil {
			t.Fatalf("ReadAll: %v", rerr)
		}

		var got []ContentBlock

		uerr := json.Unmarshal(lines[len(lines)-1].Content, &got)
		if uerr != nil {
			t.Fatalf("unmarshal image content: %v", uerr)
		}

		if len(got) != 1 {
			t.Fatalf("blocks = %d; want 1", len(got))
		}

		b := got[0]
		switch {
		case b.DataRef != imgRefPath:
			t.Errorf("DataRef = %q; want %q", b.DataRef, imgRefPath)
		case b.MediaType != imgMediaTyp:
			t.Errorf("MediaType = %q; want %q", b.MediaType, imgMediaTyp)
		case b.Width != imgScaledW || b.Height != imgScaledH:
			t.Errorf("dims = (%d,%d); want (%d,%d)", b.Width, b.Height, imgScaledW, imgScaledH)
		case b.OrigWidth != imgOrigW || b.OrigHeight != imgOrigH:
			t.Errorf("orig dims = (%d,%d); want (%d,%d)", b.OrigWidth, b.OrigHeight, imgOrigW, imgOrigH)
		case b.OrigSize != imgOrigSize:
			t.Errorf("OrigSize = %d; want %d", b.OrigSize, imgOrigSize)
		case !b.Scaled:
			t.Error("Scaled = false; want true (the resize provenance flag)")
		}

		// NO base64 payload field on the lean image line (09-05 discipline).
		raw := string(lines[len(lines)-1].Content)
		if strings.Contains(raw, `"data"`) {
			t.Errorf("image line carries inline data: %s; want Ref-only", raw)
		}
	})
}
