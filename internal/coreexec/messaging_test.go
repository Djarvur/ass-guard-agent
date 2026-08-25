package coreexec //nolint:testpackage // internal package test (fixture helpers shared)

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// The 12-04 Task 2 battery: SendMessage over the agent mailbox +
// ReadSessionContext over the persisted-session reader.

// TestSendMessage_DeliversAndAcks (T2 Test 1): a registered live agent
// receives {to, summary, message} BYTE-FAITHFUL; the result is the ack form
// (corpus_absent source-informed default — the 12-05 hunt).
func TestSendMessage_DeliversAndAcks(t *testing.T) {
	t.Parallel()

	mb := NewAgentMailbox()

	var got AgentMessage

	const agentID = "agent_26954a13-f6f1-4028-bfa6-764d66cb7577"

	mb.Register(agentID, func(m AgentMessage) error {
		got = m

		return nil
	})

	input := `{"to":"` + agentID + `","summary":"thanks for the count","message":"great work"}`

	out, err := SendMessageExecute(mb)(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("err = %v; want nil (delivery succeeded)", err)
	}

	if got.Summary != "thanks for the count" || got.Message != "great work" {
		t.Errorf("delivered payload = %+v; want byte-faithful", got)
	}

	var ack string

	uerr := json.Unmarshal(out, &ack)
	if uerr != nil {
		t.Fatalf("ack not a JSON string: %v (%s)", uerr, out)
	}

	if !strings.HasPrefix(ack, "Message msg_") || !strings.Contains(ack, "was queued for local agent agent_26954a13") {
		t.Errorf("ack = %q; want the queued-ack form naming the agent", ack)
	}
}

// TestSendMessage_UnknownRecipient (T2 Test 2): an unregistered id returns the
// structured error form with the id echoed — no panic, no silent success.
func TestSendMessage_UnknownRecipient(t *testing.T) {
	t.Parallel()

	input := `{"to":"agent_deadbeef-0000-0000-0000-000000000000","summary":"s","message":"m"}`

	out, err := SendMessageExecute(NewAgentMailbox())(context.Background(), json.RawMessage(input))
	if err == nil {
		t.Fatal("err = nil; want non-nil (unknown recipient is an error result)")
	}

	var structured struct {
		Error string `json:"error"`
	}

	uerr := json.Unmarshal(out, &structured)
	if uerr != nil || !strings.Contains(structured.Error, "agent_deadbeef") {
		t.Errorf("Output = %s; want the structured error echoing the id", out)
	}
}

// writeFixtureTranscript seeds a two-session .ass-guard/ family for the reader:
// the captured-vocabulary fixture (transcript_sess_alpha.jsonl) AND a real-shaped
// product fixture (transcript_<uuid>.jsonl — 12-11/G-12-4c: ass-guard mints
// RFC-4122-v4 session ids via internal/acp/handlers.go newSessionID and writes
// transcript_<uuid>.jsonl via internal/session/transcript.go openTranscript; the
// sess_ shape alone hid the id-vocabulary mismatch from the 12-04 offline tests).
func writeFixtureTranscript(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	store := filepath.Join(dir, ".ass-guard")

	_ = os.MkdirAll(store, dirPermWrite)

	write := func(name string, lines []string) {
		t.Helper()

		werr := os.WriteFile(filepath.Join(store, name), []byte(strings.Join(lines, "\n")+"\n"), dirPermWrite)
		if werr != nil {
			t.Fatal(werr)
		}
	}

	const uuidSession = "d9f98023-1c3e-4b5a-9e2f-a1b2c3d4e5f6"

	write("transcript_sess_alpha.jsonl", []string{
		`{"type":"session_start","sessionID":"sess_alpha"}`,
		`{"type":"user_message","turnID":"sess_alpha-turn-001",` +
			`"content":[{"type":"text",` +
			`"text":"add a cache library to the project"}]}`,
		`{"type":"assistant_message","turnID":"sess_alpha-turn-001","text":"I will use ristretto for the cache."}`,
		`{"type":"user_message","turnID":"sess_alpha-turn-002",` +
			`"content":[{"type":"text","text":"also add tests for the cache"}]}`,
		`{"type":"assistant_message","turnID":"sess_alpha-turn-002","text":"tests added for ristretto."}`,
	})

	// The real-shaped fixture mirrors the line types the product writes (the
	// same shapes as above, keyed to the UUID id) with a distinctive marker so
	// query filtering has something to hit.
	write("transcript_"+uuidSession+".jsonl", []string{
		`{"type":"session_start","sessionID":"` + uuidSession + `"}`,
		`{"type":"user_message","turnID":"` + uuidSession + `-turn-001",` +
			`"content":[{"type":"text",` +
			`"text":"which cache library should we adopt?"}]}`,
		`{"type":"assistant_message","turnID":"` + uuidSession + `-turn-001",` +
			`"text":"We chose ristretto for the cache-library decision."}`,
		`{"type":"user_message","turnID":"` + uuidSession + `-turn-002",` +
			`"content":[{"type":"text","text":"wire it into the store"}]}`,
		`{"type":"assistant_message","turnID":"` + uuidSession + `-turn-002","text":"store wired to ristretto."}`,
	})

	return dir
}

// TestReadSessionContext_Relevant (T2 Test 3): a query matching scattered
// lines returns a deterministic excerpt with turn context, bounded by
// maxTokens (chars/4) with a documented truncation tail.
func TestReadSessionContext_Relevant(t *testing.T) {
	t.Parallel()

	reader := NewSessionReader(writeFixtureTranscript(t))

	out, err := ReadSessionContextExecute(reader)(context.Background(), json.RawMessage(
		`{"sessionId":"sess_alpha","query":"cache"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	var text string

	uerr := json.Unmarshal(out, &text)
	if uerr != nil {
		t.Fatalf("Output not a JSON string: %v (%s)", uerr, out)
	}

	if !strings.HasPrefix(text, "ReadSessionContext returned lite context for sess_alpha.") {
		t.Errorf("head = %q; want the captured lite-context head", text[:40])
	}

	// Both cache-mentioning turns appear with their turn context.
	if !strings.Contains(text, "add a cache library") || !strings.Contains(text, "ristretto") {
		t.Errorf("relevant excerpt = %q; want both matching turns", text)
	}

	// maxTokens bounding: a tiny budget truncates with the documented tail.
	smallIn := `{"sessionId":"sess_alpha","query":"cache","maxTokens":8}`

	outSmall, err := ReadSessionContextExecute(reader)(context.Background(), json.RawMessage(smallIn))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	_ = json.Unmarshal(outSmall, &text)

	if !strings.Contains(text, "[truncated at maxTokens=8]") {
		t.Errorf("bounded output = %q; want the truncation tail", text)
	}
}

// TestReadSessionContext_HandoffAndUnknown (T2 Test 4): the handoff strategy
// returns the bounded TAIL of the session; an unknown sess_ id returns the
// structured error form.
func TestReadSessionContext_HandoffAndUnknown(t *testing.T) {
	t.Parallel()

	reader := NewSessionReader(writeFixtureTranscript(t))

	out, err := ReadSessionContextExecute(reader)(context.Background(), json.RawMessage(
		`{"sessionId":"sess_alpha","query":"handoff","strategy":"handoff"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	var text string

	_ = json.Unmarshal(out, &text)

	if !strings.Contains(text, "tests added for ristretto.") {
		t.Errorf("handoff excerpt = %q; want the session tail", text)
	}

	out2, err2 := ReadSessionContextExecute(reader)(context.Background(), json.RawMessage(
		`{"sessionId":"sess_nonexistent","query":"x"}`))
	if err2 == nil {
		t.Fatal("err = nil; want the unknown-session error")
	}

	var structured struct {
		Error string `json:"error"`
	}

	_ = json.Unmarshal(out2, &structured)

	if !strings.Contains(structured.Error, "sess_nonexistent") {
		t.Errorf("Output = %s; want the unknown-session error echoing the id", out2)
	}
}

// TestRegisterInteractive_MessagingPair: the pair registers Execute-only and a
// nil mailbox/reader is tolerated (missing catalog entries skipped, present
// ones registered).
func TestRegisterInteractive_MessagingPair(t *testing.T) {
	t.Parallel()

	catalog := toolcat.NewCatalog()

	RegisterInteractive(catalog, InteractiveConfig{
		Ask:      session.NewAskBroker(session.DefaultAskTimeout, nil),
		PlanMode: session.NewPlanModeState(),
		Mailbox:  NewAgentMailbox(),
		Sessions: NewSessionReader(t.TempDir()),
	})

	for _, name := range []string{"SendMessage", "ReadSessionContext"} {
		tool, ok := catalog.Get(name)
		if !ok {
			t.Fatalf("catalog missing %s", name)
		}

		if tool.Execute == nil {
			t.Errorf("%s Execute not set", name)
		}
	}
}

// --- 12-11 / G-12-4c battery: the product's own UUID id vocabulary ----------

// uuidSession is a REAL-shaped session id — the RFC 4122 v4 form
// internal/acp/handlers.go newSessionID mints (lowercase hex + dashes).
const uuidSession = "d9f98023-1c3e-4b5a-9e2f-a1b2c3d4e5f6"

// TestReadSessionContext_ProductUUIDSession (12-11 T1): the reader must serve
// ass-guard's OWN persisted transcripts keyed by the minted UUID ids, not only
// the captured sess_* vocabulary. RED signature today: the structured
// "readsessioncontext: invalid session id d9f98023-…" error — the UAT test 4
// failure mechanically.
func TestReadSessionContext_ProductUUIDSession(t *testing.T) {
	t.Parallel()

	reader := NewSessionReader(writeFixtureTranscript(t))

	out, err := ReadSessionContextExecute(reader)(context.Background(), json.RawMessage(
		`{"sessionId":"`+uuidSession+`","query":"cache-library"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (the reader must accept ass-guard's own UUID session ids)", err)
	}

	var text string

	uerr := json.Unmarshal(out, &text)
	if uerr != nil {
		t.Fatalf("Output not a JSON string: %v (%s)", uerr, out)
	}

	if !strings.Contains(text, "cache-library decision") {
		t.Errorf("excerpt = %q; want the fixture's matching assistant text", text)
	}
}

// TestReadSessionContext_UUIDHandoff (12-11 T1): the handoff strategy renders
// the bounded tail over the product vocabulary too.
func TestReadSessionContext_UUIDHandoff(t *testing.T) {
	t.Parallel()

	reader := NewSessionReader(writeFixtureTranscript(t))

	out, err := ReadSessionContextExecute(reader)(context.Background(), json.RawMessage(
		`{"sessionId":"`+uuidSession+`","query":"handoff","strategy":"handoff"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	var text string

	_ = json.Unmarshal(out, &text)

	if !strings.Contains(text, "store wired to ristretto.") {
		t.Errorf("handoff excerpt = %q; want the UUID session's tail", text)
	}
}

// TestSessions_EnumeratesBothVocabularies (12-11 T1): Sessions() must list every
// recorded transcript regardless of id vocabulary. RED signature today: the
// UUID id is SILENTLY absent — the transcript_sess_* glob skips its file.
func TestSessions_EnumeratesBothVocabularies(t *testing.T) {
	t.Parallel()

	got := map[string]bool{}
	for _, id := range NewSessionReader(writeFixtureTranscript(t)).Sessions() {
		got[id] = true
	}

	for _, want := range []string{"sess_alpha", uuidSession} {
		if !got[want] {
			t.Errorf("Sessions() missing %q; got %v", want, NewSessionReader(writeFixtureTranscript(t)).Sessions())
		}
	}
}

// TestReadSessionContext_RejectsTraversalIds pins the safety invariant that
// survives the vocabulary widening: an id carrying path separators (or any
// shape outside both vocabularies) is rejected structurally BEFORE any file
// open — the widened validator stays traversal-safe (T-12-11-01). Expected
// GREEN before AND after the fix.
func TestReadSessionContext_RejectsTraversalIds(t *testing.T) {
	t.Parallel()

	reader := NewSessionReader(writeFixtureTranscript(t))

	for _, bad := range []string{"../../etc", `sub\evil`, "..", "", ".gitignore", "d9f98023/../../etc"} {
		out, err := ReadSessionContextExecute(reader)(context.Background(), json.RawMessage(
			`{"sessionId":"`+bad+`","query":"x"}`))
		if err == nil {
			t.Errorf("sessionId %q: err = nil; want the structured invalid-id rejection", bad)

			continue
		}

		var structured struct {
			Error string `json:"error"`
		}

		_ = json.Unmarshal(out, &structured)

		if !strings.Contains(structured.Error, "invalid session id") &&
			!strings.Contains(structured.Error, "invalid input") {
			t.Errorf("sessionId %q: Output = %s; want the invalid-id error class", bad, out)
		}
	}
}
