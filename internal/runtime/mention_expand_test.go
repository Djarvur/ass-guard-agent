package runtime //nolint:testpackage // internal package test

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// 21-04 (PAR-06/D-10) mention-expansion battery: expandUserBlocks' mention
// pass — file content sections, ONE-LEVEL dir listings, the Read-rule gate
// (injected evaluator; nil = implicit allow), fixed-form unresolvable notes,
// per-mention provenance, and the zero-mention byte-identity control. All
// fixtures live under a t.TempDir workspace; the tests drive expandUserBlocks
// directly (the exact seam slash-commands ride, invoked pre-turn from Run).

// Mention-expansion fixture markers (unique so Contains-asserts cannot pass
// on fixture plumbing by accident).
const (
	mentionFileBody   = "file-content-marker-alpha\nsecond line\n"
	mentionSpacedBody = "spaced-file-content-marker-beta\n"
	mentionDeepBody   = "deep-nested-content-marker-gamma\n"
	mentionSessionID  = "sess-mentions"
)

// mentionFixture plants the standard workspace tree:
//
//	dir/notes.md              — plain file (mentionFileBody)
//	dir/my file.txt           — spaced name (mentionSpacedBody)
//	dir/pkg/entry.md          — level-1 file
//	dir/pkg/nested/deep.txt   — level-2 file (must NEVER appear in a listing)
func mentionFixture(t *testing.T, dir string) {
	t.Helper()

	plants := map[string]string{
		"notes.md":            mentionFileBody,
		"my file.txt":         mentionSpacedBody,
		"pkg/entry.md":        "entry\n",
		"pkg/nested/deep.txt": mentionDeepBody,
	}

	for rel, body := range plants {
		p := filepath.Join(dir, rel)

		err := os.MkdirAll(filepath.Dir(p), 0o750)
		if err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}

		err = os.WriteFile(p, []byte(body), 0o600)
		if err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// newMentionRunner builds a Runner over the planted temp workspace plus a
// minimal session (Manager only — expandUserBlocks touches nothing else; no
// registry discovery runs, so no HOME pin is needed).
func newMentionRunner(t *testing.T, plant func(t *testing.T, dir string)) (r *Runner, sess *session.Session, dir string) {
	t.Helper()

	dir = t.TempDir()
	if plant != nil {
		plant(t, dir)
	}

	mgr, err := session.NewManager(dir, mentionSessionID, redactorAdapter{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	t.Cleanup(func() { _ = mgr.Close() })

	return &Runner{workDir: dir}, &session.Session{Manager: mgr}, dir
}

// mentionBlocks builds the one-text-block prompt over text.
func mentionBlocks(text string) []session.ContentBlock {
	return []session.ContentBlock{{Type: blockText, Text: text}}
}

// mentionProvenance returns the session's mention_provenance lines in
// append order.
func mentionProvenance(t *testing.T, sess *session.Session) []session.Line {
	t.Helper()

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var out []session.Line
	for _, l := range lines {
		if l.Type == session.TypeMentionProvenance {
			out = append(out, l)
		}
	}

	return out
}

// TestMentionExpand_FileSectionAndProvenance: an @file under the workspace
// gains a labeled section (raw-token header + content) in the first text
// block and one AppendMentionProvenance line lands with form=file, the raw
// token, and the resolved path (the AppendCommandProvenance discipline).
func TestMentionExpand_FileSectionAndProvenance(t *testing.T) {
	t.Parallel()

	r, sess, dir := newMentionRunner(t, mentionFixture)

	got := r.expandUserBlocks(sess, mentionBlocks("review @notes.md please"))

	if len(got) != 1 || got[0].Type != blockText {
		t.Fatalf("blocks shape changed: %+v", got)
	}

	text := got[0].Text
	if !strings.Contains(text, "[@notes.md]\n"+mentionFileBody) {
		t.Errorf("text missing the labeled file section (token header + content):\n%q", text)
	}

	if !strings.Contains(text, "review @notes.md please") {
		t.Errorf("text lost its original prose:\n%q", text)
	}

	prov := mentionProvenance(t, sess)
	if len(prov) != 1 {
		t.Fatalf("provenance lines = %d; want 1 (%+v)", len(prov), prov)
	}

	if prov[0].Name != "@notes.md" {
		t.Errorf("provenance Name = %q; want the raw token @notes.md", prov[0].Name)
	}

	if want := filepath.Join(dir, "notes.md"); prov[0].CommandRef != want {
		t.Errorf("provenance CommandRef = %q; want %q", prov[0].CommandRef, want)
	}

	if prov[0].Text != "file" {
		t.Errorf("provenance form = %q; want file", prov[0].Text)
	}
}

// TestMentionExpand_SpacedQuotedPath: @"my file.txt" resolves through the
// quote-capturing parse into the spaced-name file's content section.
func TestMentionExpand_SpacedQuotedPath(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	got := r.expandUserBlocks(sess, mentionBlocks(`open @"my file.txt" now`))

	if !strings.Contains(got[0].Text, "[@\"my file.txt\"]\n"+mentionSpacedBody) {
		t.Errorf("text missing the spaced-path file section:\n%q", got[0].Text)
	}

	if prov := mentionProvenance(t, sess); len(prov) != 1 || prov[0].Text != "file" {
		t.Errorf("provenance = %+v; want one file-form line", prov)
	}
}

// TestMentionExpand_DirListingOneLevel (D-10 pinned): @pkg expands to a
// ONE-LEVEL listing — level-1 entries appear (subdirectories marked with a
// trailing slash, sizes in bytes), and the child dir's FILES NEVER appear
// (no recursion — the two-level fixture is the proof).
func TestMentionExpand_DirListingOneLevel(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	got := r.expandUserBlocks(sess, mentionBlocks("look at @pkg"))

	text := got[0].Text
	if !strings.Contains(text, "[@pkg]") {
		t.Errorf("text missing the dir section header:\n%q", text)
	}

	if !strings.Contains(text, "entry.md (") || !strings.Contains(text, " bytes)") {
		t.Errorf("listing missing the level-1 file entry with size:\n%q", text)
	}

	if !strings.Contains(text, "nested/") {
		t.Errorf("listing missing the child dir with its trailing slash:\n%q", text)
	}

	if strings.Contains(text, "entry.md\n") && strings.Contains(text, mentionDeepBody) {
		t.Errorf("listing leaked nested file CONTENT (deep.txt body):\n%q", text)
	}

	if strings.Contains(text, "deep.txt") {
		t.Errorf("listing recursed — level-2 file deep.txt must NEVER appear (D-10):\n%q", text)
	}

	prov := mentionProvenance(t, sess)
	if len(prov) != 1 || prov[0].Text != "dir" || prov[0].Name != "@pkg" {
		t.Errorf("provenance = %+v; want one dir-form @pkg line", prov)
	}
}

// TestMentionExpand_ReadRuleDenial (the gate is observable, not implied): a
// denying injected evaluator replaces the mention with a loud denied note —
// no content section — and the provenance line records the denial form. The
// consult happens with tool="Read".
func TestMentionExpand_ReadRuleDenial(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	var consultedTool string
	r.readRuleEvaluator = func(tool, _ string) bool {
		consultedTool = tool
		return false
	}

	got := r.expandUserBlocks(sess, mentionBlocks("review @notes.md please"))

	text := got[0].Text
	if !strings.Contains(text, "[@notes.md] could not be expanded:") {
		t.Errorf("text missing the loud denied note:\n%q", text)
	}

	if strings.Contains(text, mentionFileBody) {
		t.Errorf("denied @file content leaked into the prompt:\n%q", text)
	}

	if consultedTool != "Read" {
		t.Errorf("evaluator consulted with tool %q; want Read", consultedTool)
	}

	prov := mentionProvenance(t, sess)
	if len(prov) != 1 {
		t.Fatalf("provenance lines = %d; want 1 (%+v)", len(prov), prov)
	}

	if prov[0].Text != "denied" || prov[0].CommandRef != "" {
		t.Errorf("denial provenance = %+v; want form=denied with no resolved path", prov[0])
	}
}

// TestMentionExpand_DefaultEvaluatorImplicitAllow: with NO evaluator wired
// (today's pre-21-06 default) the consult allows — the file section lands
// (current behavior preserved; 21-06 wires the perm rule set into the seam).
func TestMentionExpand_DefaultEvaluatorImplicitAllow(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	if r.readRuleEvaluator != nil {
		t.Fatal("fixture must leave the evaluator unwired")
	}

	got := r.expandUserBlocks(sess, mentionBlocks("review @notes.md"))

	if !strings.Contains(got[0].Text, mentionFileBody) {
		t.Errorf("implicit-allow default did not expand the file:\n%q", got[0].Text)
	}
}

// TestMentionExpand_UnresolvableNote (D-10 + the privacy prohibition): a
// missing @path yields ONE fixed-form loud note naming the token and the
// outcome class — never per-path filesystem detail beyond the class, never
// content — and the turn proceeds (blocks still expand, provenance records
// the unresolved form).
func TestMentionExpand_UnresolvableNote(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	got := r.expandUserBlocks(sess, mentionBlocks("check @nope/missing.md out"))

	text := got[0].Text
	if !strings.Contains(text, "[@nope/missing.md] could not be expanded:") {
		t.Errorf("text missing the loud unresolvable note:\n%q", text)
	}

	if strings.Count(text, "could not be expanded:") != 1 {
		t.Errorf("expected exactly ONE note, got:\n%q", text)
	}

	if !strings.Contains(text, "check @nope/missing.md out") {
		t.Errorf("text lost its original prose:\n%q", text)
	}

	prov := mentionProvenance(t, sess)
	if len(prov) != 1 || prov[0].Text != "unresolved" || prov[0].CommandRef != "" {
		t.Errorf("provenance = %+v; want one unresolved-form line", prov)
	}
}

// TestMentionExpand_OutsideAdmittedRoots (T-21-14): a relative @path
// escaping the workspace root and an ABSOLUTE @path with no evaluator wired
// both yield the fixed outside-roots note — ingress resolution is never a
// whole-filesystem existence oracle (even an existing absolute path stays
// unexpanded under the nil default).
func TestMentionExpand_OutsideAdmittedRoots(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	got := r.expandUserBlocks(sess, mentionBlocks("a @../escape.md b @/etc/hosts"))

	text := got[0].Text
	if strings.Count(text, "could not be expanded:") != 2 {
		t.Fatalf("expected one note per outside-roots mention, got:\n%q", text)
	}

	if !strings.Contains(text, "[@../escape.md] could not be expanded:") ||
		!strings.Contains(text, "[@/etc/hosts] could not be expanded:") {
		t.Errorf("notes missing their tokens:\n%q", text)
	}

	if strings.Contains(text, "localhost") {
		t.Errorf("absolute-path content leaked without any evaluator admission:\n%q", text)
	}

	prov := mentionProvenance(t, sess)
	if len(prov) != 2 {
		t.Fatalf("provenance lines = %d; want 2 (%+v)", len(prov), prov)
	}

	for _, p := range prov {
		if p.Text != "unresolved" || p.CommandRef != "" {
			t.Errorf("outside-roots provenance = %+v; want unresolved with no resolved path", p)
		}
	}
}

// TestMentionExpand_AbsolutePathAdmittedByEvaluator: an absolute path an
// explicitly-wired evaluator allows IS admitted (the "declared absolute
// paths the Read rules admit" half of the bounds) and expands normally.
func TestMentionExpand_AbsolutePathAdmittedByEvaluator(t *testing.T) {
	t.Parallel()

	r, sess, dir := newMentionRunner(t, mentionFixture)

	abs := filepath.Join(dir, "notes.md")
	r.readRuleEvaluator = func(_, p string) bool { return p == abs }

	got := r.expandUserBlocks(sess, mentionBlocks("read @"+abs))

	if !strings.Contains(got[0].Text, mentionFileBody) {
		t.Errorf("evaluator-admitted absolute path did not expand:\n%q", got[0].Text)
	}

	prov := mentionProvenance(t, sess)
	if len(prov) != 1 || prov[0].Text != "file" || prov[0].CommandRef != abs {
		t.Errorf("provenance = %+v; want one file-form line with the abs path", prov)
	}
}

// TestMentionExpand_MultipleMentionsInOrder: mentions expand in token order,
// each with its own section and provenance line.
func TestMentionExpand_MultipleMentionsInOrder(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	got := r.expandUserBlocks(sess, mentionBlocks("compare @notes.md with @pkg"))

	text := got[0].Text
	notesIdx := strings.Index(text, "[@notes.md]")
	pkgIdx := strings.Index(text, "[@pkg]")
	if notesIdx < 0 || pkgIdx < 0 || notesIdx > pkgIdx {
		t.Errorf("sections missing or out of token order (notes=%d pkg=%d):\n%q", notesIdx, pkgIdx, text)
	}

	prov := mentionProvenance(t, sess)
	if len(prov) != 2 {
		t.Fatalf("provenance lines = %d; want 2 (%+v)", len(prov), prov)
	}

	if prov[0].Name != "@notes.md" || prov[0].Text != "file" {
		t.Errorf("provenance[0] = %+v; want the @notes.md file form first", prov[0])
	}

	if prov[1].Name != "@pkg" || prov[1].Text != "dir" {
		t.Errorf("provenance[1] = %+v; want the @pkg dir form second", prov[1])
	}
}

// TestMentionExpand_ZeroMentionsByteIdentical (the existing seam control): a
// prompt with zero mentions passes through UNCHANGED — equal blocks, no
// provenance lines (expandUserBlocks' early-return shape preserved).
func TestMentionExpand_ZeroMentionsByteIdentical(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	in := []session.ContentBlock{
		{Type: blockText, Text: "email foo@bar.com — no mentions at all"},
		{Type: "resource_link", Text: ""},
	}

	got := r.expandUserBlocks(sess, in)

	if len(got) != len(in) {
		t.Fatalf("block count %d → %d", len(in), len(got))
	}

	for i := range in {
		if got[i] != in[i] {
			t.Errorf("block[%d] changed: %+v → %+v", i, in[i], got[i])
		}
	}

	if prov := mentionProvenance(t, sess); len(prov) != 0 {
		t.Errorf("provenance lines = %d; want 0 for a zero-mention prompt (%+v)", len(prov), prov)
	}
}

// TestMentionExpand_ProvenanceFailureContinues (the :426-434 discipline): a
// transcript write failure degrades to a LOUD stderr log — the expansion
// itself still lands and the turn proceeds (never an ACP error).
func TestMentionExpand_ProvenanceFailureContinues(t *testing.T) { //nolint:paralleltest // global logger pin
	r, sess, _ := newMentionRunner(t, mentionFixture)

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	// Close the Manager: every append now fails (the write-error class).
	if err := sess.Manager.Close(); err != nil {
		t.Fatalf("close manager: %v", err)
	}

	got := r.expandUserBlocks(sess, mentionBlocks("review @notes.md"))

	if !strings.Contains(got[0].Text, mentionFileBody) {
		t.Errorf("expansion suppressed by a provenance write failure:\n%q", got[0].Text)
	}

	if !strings.Contains(logBuf.String(), "mention provenance write failed") {
		t.Errorf("provenance failure was not LOUD on stderr; log = %q", logBuf.String())
	}
}

// TestMentionExpand_CommandAndMentionCompose: the mention pass runs AFTER
// the command-invocation logic on the same seam — an unregistered /foo
// (ordinary text) carrying a mention still expands the mention; the
// registered-command path is pinned by the TestExpansion_* battery.
func TestMentionExpand_CommandAndMentionCompose(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	got := r.expandUserBlocks(sess, mentionBlocks("/notaregistrykey with @notes.md"))

	if !strings.Contains(got[0].Text, "/notaregistrykey with @notes.md") {
		t.Errorf("text lost its original prose:\n%q", got[0].Text)
	}

	if !strings.Contains(got[0].Text, mentionFileBody) {
		t.Errorf("mention inside a registry-miss invocation did not expand:\n%q", got[0].Text)
	}
}

// TestMentionExpand_JSONRoundTripSanity: the expanded text survives a JSON
// marshal/unmarshal round trip (the user_message transcript line carries it
// as content — no control characters or framing the writer cannot persist).
func TestMentionExpand_JSONRoundTripSanity(t *testing.T) {
	t.Parallel()

	r, sess, _ := newMentionRunner(t, mentionFixture)

	got := r.expandUserBlocks(sess, mentionBlocks("review @notes.md and @pkg"))

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back []session.ContentBlock
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if back[0].Text != got[0].Text {
		t.Errorf("round trip drifted:\n%q\n%q", got[0].Text, back[0].Text)
	}
}
