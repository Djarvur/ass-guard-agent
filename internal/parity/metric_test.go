package parity_test

import (
	"encoding/json"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
)

// TestCompareSequence covers Layer 1 (ordered tool-name sequence equality).
func TestCompareSequence(t *testing.T) {
	cases := []struct {
		name               string
		expected, observed []string
		want               bool
	}{
		{"identical", []string{"Read", "Grep", "Edit"}, []string{"Read", "Grep", "Edit"}, true},
		{"reorder", []string{"Read", "Grep", "Edit"}, []string{"Grep", "Read", "Edit"}, false},
		{"substitute", []string{"Read", "Grep", "Edit"}, []string{"Read", "Bash", "Edit"}, false},
		{"insertion", []string{"Read", "Edit"}, []string{"Read", "Grep", "Edit"}, false},
		{"deletion", []string{"Read", "Grep", "Edit"}, []string{"Read", "Edit"}, false},
		{"empty-both", []string{}, []string{}, true},
		{"empty-vs-one", []string{}, []string{"Read"}, false},
		{"single-match", []string{"Bash"}, []string{"Bash"}, true},
	}
	for _, c := range cases {
		if got := parity.CompareSequence(c.expected, c.observed); got != c.want {
			t.Errorf("%s: CompareSequence(%v,%v) = %v, want %v", c.name, c.expected, c.observed, got, c.want)
		}
	}
}

// TestCompareArgs covers Layer 2 (per-tool argument structural equality).
func TestCompareArgs(t *testing.T) {
	cases := []struct {
		name               string
		expected, observed string
		wantOK             bool
	}{
		{"identical", `{"file_path":"src/x.go"}`, `{"file_path":"src/x.go"}`, true},
		{"missing-key", `{"file_path":"x","offset":10}`, `{"file_path":"x"}`, false},
		{"extra-key", `{"file_path":"x"}`, `{"file_path":"x","offset":10}`, false},
		{"type-mismatch", `{"limit":100}`, `{"limit":"100"}`, false},
		{"path-normalized-abs-vs-rel", `{"file_path":"/abs/repo/src/x.go"}`, `{"file_path":"src/x.go"}`, true},
		{"enum-exact-differs", `{"command":"Read"}`, `{"command":"Grep"}`, false},
		{"enum-exact-same", `{"command":"Read"}`, `{"command":"Read"}`, true},
		{"freetext-type-only", `{"query":"foo bar"}`, `{"query":"baz qux"}`, true},
		{"number-type-only", `{"limit":100}`, `{"limit":50}`, true},
		{"array-order-sensitive", `{"paths":["a","b"]}`, `{"paths":["b","a"]}`, false},
		{"array-same-order", `{"paths":["a","b"]}`, `{"paths":["a","b"]}`, true},
	}
	for _, c := range cases {
		_, ok := parity.CompareArgs(json.RawMessage(c.expected), json.RawMessage(c.observed))
		if ok != c.wantOK {
			t.Errorf("%s: CompareArgs ok=%v, want %v", c.name, ok, c.wantOK)
		}
	}
}

// TestCompare covers the combined two-layer result.
func TestCompare(t *testing.T) {
	expected := []parity.ToolCall{
		{Name: "Read", Input: json.RawMessage(`{"file_path":"src/x.go"}`)},
		{Name: "Bash", Input: json.RawMessage(`{"command":"go test"}`)},
	}
	// matching observed
	match := []parity.ToolCall{
		{Name: "Read", Input: json.RawMessage(`{"file_path":"/abs/repo/src/x.go"}`)}, // path-normalized
		{Name: "Bash", Input: json.RawMessage(`{"command":"go test"}`)},
	}
	if c := parity.Compare(expected, match); !c.Match {
		t.Errorf("expected Match, got Layer1=%v Layer2=%v", c.Layer1SequenceMatch, c.Layer2ArgMismatches)
	}
	// wrong tool
	wrong := []parity.ToolCall{
		{Name: "Grep", Input: json.RawMessage(`{"pattern":"x"}`)},
		{Name: "Bash", Input: json.RawMessage(`{"command":"go test"}`)},
	}
	if c := parity.Compare(expected, wrong); c.Match {
		t.Error("expected mismatch for substituted tool")
	}
}
