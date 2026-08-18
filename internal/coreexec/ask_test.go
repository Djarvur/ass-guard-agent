package coreexec //nolint:testpackage // internal package test (shares the fixture loader)

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// interactiveFixturePath is the committed capture-grounded fixture for the
// interactive-tool forms (12-01 T1) — same convention as zcode-core-results.json.
const interactiveFixturePath = "testdata/zcode-interactive-results.json"

// interactiveFixture is the decoded shape of the interactive fixture (only the
// fields the conformance tests assert).
type interactiveFixture struct {
	Provenance struct {
		Policy       string   `json:"policy"`
		CorpusAbsent []string `json:"corpus_absent"`
	} `json:"_provenance"` //nolint:tagliatelle // fixture header key
	Tools map[string]struct {
		InputKeysObserved []string `json:"input_keys_observed"`
		Results           map[string]struct {
			IsError       bool   `json:"isError"` //nolint:tagliatelle // fixture mirrors the capture
			Template      string `json:"template"`
			LiteralPrefix string `json:"literal_prefix,omitempty"`
			CorpusAbsent  bool   `json:"corpus_absent,omitempty"`
		} `json:"results"`
	} `json:"tools"`
}

// TestAskForms_ConformToFixture (12-01 T1 Test 4): the answered and non-answer
// forms string-compare against the committed fixture templates; the non-answer
// family carries corpus_absent + the D-01 provenance note + the 12-05 upgrade
// pointer (the 08-08 fixtures_test.go pattern).
func TestAskForms_ConformToFixture(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(interactiveFixturePath)
	if err != nil {
		t.Fatalf("read %s: %v (the interactive fixture must be committed)", interactiveFixturePath, err)
	}

	var f interactiveFixture

	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse %s: %v", interactiveFixturePath, err)
	}

	entry, ok := f.Tools["AskUserQuestion"]
	if !ok {
		t.Fatal("fixture missing the AskUserQuestion entry")
	}

	// The ANSWERED form byte-matches the fixture template (prefix + verbatim
	// reply + closing quote).
	answered := session.RenderAskAnswered("ristretto, please")

	wantAnswered := strings.ReplaceAll(
		entry.Results["answered"].Template, "<reply verbatim>", "ristretto, please",
	)

	if answered != wantAnswered {
		t.Errorf("answered form = %q; want the fixture template %q", answered, wantAnswered)
	}

	if !strings.HasPrefix(answered, entry.Results["answered"].LiteralPrefix) {
		t.Errorf("answered form = %q; want the captured literal prefix %q",
			answered, entry.Results["answered"].LiteralPrefix)
	}

	// The NON-ANSWER form matches the D-01 corpus-absent default template with
	// the configured duration rendered.
	nonAnswer := session.RenderAskNonAnswer(90 * time.Second)

	wantNon := strings.ReplaceAll(
		entry.Results["non_answer_timeout"].Template, "<duration>", "1m30s",
	)

	if nonAnswer != wantNon {
		t.Errorf("non-answer form = %q; want the fixture template %q", nonAnswer, wantNon)
	}

	// The non-answer family is flagged corpus_absent with the D-01 note + the
	// 12-05 upgrade pointer.
	na := entry.Results["non_answer_timeout"]

	if !na.CorpusAbsent {
		t.Error("non_answer_timeout must carry corpus_absent:true (no pinned capture exists)")
	}

	if !strings.Contains(f.Provenance.Policy, "D-01") {
		t.Errorf("_provenance.policy = %q; want the D-01 policy note", f.Provenance.Policy)
	}

	foundAbsent, foundPointer := false, false

	for _, c := range f.Provenance.CorpusAbsent {
		if strings.Contains(c, "ask_non_answer_form") {
			foundAbsent = true
		}

		if strings.Contains(c, "12-05") {
			foundPointer = true
		}
	}

	if !foundAbsent {
		t.Error("corpus_absent missing the ask_non_answer_form entry")
	}

	if !foundPointer {
		t.Error("corpus_absent missing the 12-05 re-record upgrade pointer")
	}

	// Neither form is an error result: both are INFORMATION for the model to
	// proceed on (the D-01 hands-off contract).
	if entry.Results["answered"].IsError || na.IsError {
		t.Error("ask result forms must not be isError (the model proceeds on either)")
	}
}

// TestAskExecute_ParsesAndSuspends (12-01, executor unit): the executor parses
// the captured {questions:[…]} shape best-effort (unknown fields ignored), and
// returns ErrSuspended with the parsed questions riding the Output (the Stub
// seam carries no call identity — the session loop binds turnID+callID).
func TestAskExecute_ParsesAndSuspends(t *testing.T) {
	t.Parallel()

	b := session.NewAskBroker(time.Hour, nil)

	exec := AskUserQuestionExecute(b)

	out, err := exec(context.Background(), json.RawMessage(`{
		"questions":[{
			"question":"Which DB?",
			"header":"DB",
			"options":[{"label":"postgres","description":"boring and solid","preview":"x"}],
			"multiSelect":true,
			"unknownField":"ignored"
		}],
		"unknownTopLevel":true
	}`))

	if !errors.Is(err, session.ErrSuspended) {
		t.Fatalf("executor err = %v; want session.ErrSuspended", err)
	}

	var qs []session.AskQuestion

	if uerr := json.Unmarshal(out, &qs); uerr != nil {
		t.Fatalf("Output is not the parsed questions payload: %v (%s)", uerr, out)
	}

	if len(qs) != 1 {
		t.Fatalf("parsed %d questions; want 1", len(qs))
	}

	if qs[0].Question != "Which DB?" || qs[0].Header != "DB" || !qs[0].MultiSelect {
		t.Errorf("parsed question = %+v; want question+header+multiSelect carried", qs[0])
	}

	if len(qs[0].Options) != 1 || qs[0].Options[0].Label != "postgres" ||
		qs[0].Options[0].Description != "boring and solid" {
		t.Errorf("parsed options = %+v; want label+description", qs[0].Options)
	}
}

// TestAskExecute_EmptyAndMalformed (best-effort parse): empty/absent questions
// still suspend (the model asked SOMETHING — an empty surface is rendered) and
// never panic; malformed JSON degrades to zero questions.
func TestAskExecute_EmptyAndMalformed(t *testing.T) {
	t.Parallel()

	b := session.NewAskBroker(time.Hour, nil)

	exec := AskUserQuestionExecute(b)

	for _, in := range []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"questions":[]}`),
		json.RawMessage(`not json at all`),
	} {
		out, err := exec(context.Background(), in)
		if !errors.Is(err, session.ErrSuspended) {
			t.Errorf("input %s: err = %v; want ErrSuspended", in, err)
		}

		if out == nil {
			t.Errorf("input %s: Output must carry the (possibly empty) questions payload", in)
		}
	}
}

// TestAskExecute_NilBroker (defensive): an executor built without a broker
// returns the structured-error convention, NOT a suspension (a suspension
// nobody can resolve is a dead end — the exact thing this plan removes).
func TestAskExecute_NilBroker(t *testing.T) {
	t.Parallel()

	exec := AskUserQuestionExecute(nil)

	out, err := exec(context.Background(), json.RawMessage(`{"questions":[]}`))

	if errors.Is(err, session.ErrSuspended) {
		t.Fatal("nil-broker executor must not suspend")
	}

	if err == nil {
		t.Fatal("nil-broker executor must return an error")
	}

	var m map[string]string

	if uerr := json.Unmarshal(out, &m); uerr != nil || m["error"] == "" {
		t.Errorf("nil-broker output = %s; want the structured {\"error\":…} convention", out)
	}
}

// TestRegisterAsk_SchemaNeverRewritten (12-01 T1 Test 5): registration is
// Execute-ONLY — Name, Description, InputSchema, and Mutability stay
// byte-identical to the captured entry (the 08-05/08-08 discipline).
func TestRegisterAsk_SchemaNeverRewritten(t *testing.T) {
	t.Parallel()

	capturedDesc := "Use this tool only when you are blocked on a decision"
	capturedSchema := json.RawMessage(`{"type":"object","properties":{"questions":{"type":"array"}}}`)

	catalog := toolcat.NewCatalog()
	catalog.Register(toolcat.Tool{
		Name:        "AskUserQuestion",
		Description: capturedDesc,
		InputSchema: capturedSchema,
		Mutability:  toolcat.MutabilityReadOnly, // the captured entry is non-mutating (a question reads nothing)
	})

	before, _ := catalog.Get("AskUserQuestion")

	RegisterAsk(catalog, session.NewAskBroker(time.Hour, nil))

	after, ok := catalog.Get("AskUserQuestion")
	if !ok {
		t.Fatal("AskUserQuestion missing after RegisterAsk")
	}

	if after.Name != before.Name {
		t.Errorf("Name rewritten: %q → %q", before.Name, after.Name)
	}

	if after.Description != before.Description {
		t.Errorf("Description rewritten (%d → %d bytes)", len(before.Description), len(after.Description))
	}

	if string(after.InputSchema) != string(before.InputSchema) {
		t.Errorf("InputSchema rewritten: %s → %s", before.InputSchema, after.InputSchema)
	}

	if after.Mutability != before.Mutability {
		t.Errorf("Mutability rewritten: %v → %v", before.Mutability, after.Mutability)
	}

	if after.Execute == nil {
		t.Error("Execute not set — the dead end this plan removes is still there")
	}
}

// TestRegisterAsk_MissingEntrySkipped (forward-compat, the RegisterCore rule):
// a catalog without the entry is skipped, not fatal.
func TestRegisterAsk_MissingEntrySkipped(t *testing.T) {
	t.Parallel()

	catalog := toolcat.NewCatalog()

	RegisterAsk(catalog, session.NewAskBroker(time.Hour, nil)) // must not panic

	if _, ok := catalog.Get("AskUserQuestion"); ok {
		t.Error("RegisterAsk invented an entry")
	}
}

// TestRenderAskSurface (12-01 T2 Test 2's renderer, unit level): the client
// surface renders STRUCTURE — header, question, option labels with
// descriptions, multiSelect noted — never judgment (no "(Recommended)" is
// added; the model authors that convention itself per the schema).
func TestRenderAskSurface(t *testing.T) {
	t.Parallel()

	got := RenderAskSurface([]session.AskQuestion{{
		Question:    "Which cache library?",
		Header:      "Cache",
		Options:     []session.AskOption{{Label: "ristretto", Description: "fast"}},
		MultiSelect: true,
	}})

	for _, want := range []string{
		"Cache",                // the header chip
		"Which cache library?", // the question
		"ristretto",            // the option label
		"fast",                 // the option description
		"multiple",             // the multiSelect note
	} {
		if !strings.Contains(got, want) {
			t.Errorf("surface render = %q; missing %q", got, want)
		}
	}

	if strings.Contains(got, "Recommended") {
		t.Errorf("surface render = %q; the renderer must NOT add (Recommended) — that is the model's convention", got)
	}
}
