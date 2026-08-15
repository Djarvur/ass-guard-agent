package coreexec

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// fixturePath is the committed capture-grounded result-shape fixture (08-08 T1).
// The live rollout corpus at ~/.zcode/cli/rollout/ rotates; this file is the
// pinned ground truth the executors' conformance tests compare against.
const fixturePath = "testdata/zcode-core-results.json"

// fixtureFixture is the decoded shape of testdata/zcode-core-results.json.
// Only the fields the tests assert are modeled (the fixture carries more).
type fixtureFixture struct {
	Provenance struct {
		Sessions    map[string]string `json:"sessions"`
		HarvestDate string            `json:"harvestDate"`
		Redaction   string            `json:"redaction"`
		CorpusAbsent []string         `json:"corpus_absent"`
	} `json:"_provenance"`
	Tools map[string]struct {
		InputKeysObserved []string `json:"input_keys_observed"`
		Results           map[string]struct {
			IsError        bool   `json:"isError"`
			Template       string `json:"template"`
			LiteralPrefix  string `json:"literal_prefix,omitempty"`
			SampleRedacted string `json:"sample_redacted,omitempty"`
		} `json:"results"`
	} `json:"tools"`
}

// loadFixture reads + decodes the committed fixture (fatal on any failure —
// every conformance test in this package depends on it).
func loadFixture(t *testing.T) fixtureFixture {
	t.Helper()

	raw, err := os.ReadFile(fixturePath) //nolint:gosec // test fixture path is constant
	if err != nil {
		t.Fatalf("read %s: %v (the capture fixture must be committed — see 08-08 T1)", fixturePath, err)
	}

	var f fixtureFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse %s: %v", fixturePath, err)
	}

	return f
}

// TestCoreResultsFixturePinned (08-08 T1 Test 1): the capture-grounded
// result-shape fixture is committed with a _provenance header naming BOTH
// source sessions (main 4440f5a7 + subagent 8a003655) + the harvest date +
// the D-03 redaction note + a corpus_absent list naming every form the corpus
// does NOT show, and one entry per core tool holding the observed result
// forms (exact literal templates) + the observed input-key shapes. An invented
// form passing as captured would erode the mimicry core value silently
// (T-8-34) — this test is the pin.
func TestCoreResultsFixturePinned(t *testing.T) {
	t.Parallel()

	f := loadFixture(t)

	// Provenance: both source sessions named, harvest date, redaction note.
	for _, want := range []string{"4440f5a7", "8a003655"} {
		found := false

		for _, v := range f.Provenance.Sessions {
			if strings.Contains(v, want) {
				found = true
			}
		}

		if !found {
			t.Errorf("_provenance.sessions missing source %q (have %v)", want, f.Provenance.Sessions)
		}
	}

	if f.Provenance.HarvestDate != "2026-08-15" {
		t.Errorf("_provenance.harvestDate = %q; want 2026-08-15", f.Provenance.HarvestDate)
	}

	if !strings.Contains(f.Provenance.Redaction, "D-03") {
		t.Errorf("_provenance.redaction = %q; want the D-03 redaction note", f.Provenance.Redaction)
	}

	// corpus_absent names every family the corpus does not show.
	wantAbsent := []string{
		"bash_output_truncation_marker",
		"bash_timeout_error_form",
		"bash_default_timeout_value",
		"todoRead_result_form",
	}

	for _, want := range wantAbsent {
		found := false

		for _, got := range f.Provenance.CorpusAbsent {
			if strings.Contains(got, want) {
				found = true
			}
		}

		if !found {
			t.Errorf("corpus_absent missing %q (have %v)", want, f.Provenance.CorpusAbsent)
		}
	}

	// One entry per tool with input shapes + result forms. TodoRead is the
	// corpus-absent exception: no call was observed, so its input_keys list
	// is legitimately empty (the entry exists to record the absence).
	for _, tool := range []string{"Bash", "Read", "Write", "Edit", "TodoWrite"} {
		entry, ok := f.Tools[tool]
		if !ok {
			t.Errorf("fixture missing a %q entry", tool)

			continue
		}

		if len(entry.InputKeysObserved) == 0 {
			t.Errorf("%s.input_keys_observed is empty", tool)
		}

		if len(entry.Results) == 0 {
			t.Errorf("%s.results is empty", tool)
		}
	}

	if _, ok := f.Tools["TodoRead"]; !ok {
		t.Error("fixture missing a TodoRead entry (corpus-absent record)")
	}

	// The exact literal forms the executors' conformance tests pin against.
	bash := f.Tools["Bash"].Results
	if got := bash["success_no_output"].Template; got != "(Bash completed with no output)" {
		t.Errorf("Bash sentinel template = %q; want the captured literal", got)
	}

	if got := bash["error"].LiteralPrefix; got != "Exit code " {
		t.Errorf("Bash error literal_prefix = %q; want %q", got, "Exit code ")
	}

	if !bash["error"].IsError {
		t.Error("Bash error form must carry isError:true (captured 137/137 errors)")
	}

	read := f.Tools["Read"].Results
	if !strings.Contains(read["success"].Template, "\\t") && !strings.Contains(read["success"].SampleRedacted, "\t") {
		t.Errorf("Read success template/sample must show the <line-number><TAB><content> form (template=%q sample=%q)",
			read["success"].Template, read["success"].SampleRedacted)
	}

	if got := read["error_file_missing"].LiteralPrefix; got != "File does not exist. Note: your current working directory is " {
		t.Errorf("Read error literal_prefix = %q; want the captured form", got)
	}

	write := f.Tools["Write"].Results
	if got := write["success_created"].Template; !strings.HasPrefix(got, "File created successfully at: ") {
		t.Errorf("Write created template = %q; want the captured literal", got)
	}

	if got := write["success_updated"].Template; !strings.HasPrefix(got, "The file ") {
		t.Errorf("Write updated template = %q; want the captured literal", got)
	}

	edit := f.Tools["Edit"].Results
	if got := edit["success"].Template; !strings.HasPrefix(got, "The file ") {
		t.Errorf("Edit success template = %q; want the captured literal", got)
	}

	if got := edit["error_not_found"].Template; !strings.HasPrefix(got, "<tool_use_error>String to replace not found in file.") {
		t.Errorf("Edit not-found template = %q; want the captured literal", got)
	}

	tw := f.Tools["TodoWrite"].Results
	if got := tw["echo"].SampleRedacted; !strings.Contains(got, `"inProgress"`) {
		t.Errorf("TodoWrite echo sample = %q; want the camelCase inProgress summary key", got)
	}
}
