package parity_test

import (
	"encoding/json"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
)

// TestCompareSequence covers Layer 1 (ordered tool-name sequence equality).
func TestCompareSequence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		expected, observed []string
		want               bool
	}{
		{"identical", []string{toolRead, toolGrep, toolEdit}, []string{toolRead, toolGrep, toolEdit}, true},
		{"reorder", []string{toolRead, toolGrep, toolEdit}, []string{toolGrep, toolRead, toolEdit}, false},
		{"substitute", []string{toolRead, toolGrep, toolEdit}, []string{toolRead, toolBash, toolEdit}, false},
		{"insertion", []string{toolRead, toolEdit}, []string{toolRead, toolGrep, toolEdit}, false},
		{"deletion", []string{toolRead, toolGrep, toolEdit}, []string{toolRead, toolEdit}, false},
		{"empty-both", []string{}, []string{}, true},
		{"empty-vs-one", []string{}, []string{toolRead}, false},
		{"single-match", []string{toolBash}, []string{toolBash}, true},
	}
	for _, c := range cases {
		if got := parity.CompareSequence(c.expected, c.observed); got != c.want {
			t.Errorf("%s: CompareSequence(%v,%v) = %v, want %v", c.name, c.expected, c.observed, got, c.want)
		}
	}
}

// TestCompareArgs covers Layer 2 (per-tool argument structural equality).
func TestCompareArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		expected, observed string
		wantOK             bool
	}{
		{"identical", srcXGoPathJSON, srcXGoPathJSON, true},
		{"missing-key", `{"file_path":"x","offset":10}`, `{"file_path":"x"}`, false},
		{"extra-key", `{"file_path":"x"}`, `{"file_path":"x","offset":10}`, false},
		{"type-mismatch", `{"limit":100}`, `{"limit":"100"}`, false},
		{"path-normalized-abs-vs-rel", `{"file_path":"/abs/repo/src/x.go"}`, srcXGoPathJSON, true},
		{"enum-exact-differs", readCommandJSON, `{"command":"Grep"}`, false},
		{"enum-exact-same", readCommandJSON, readCommandJSON, true},
		{"freetext-type-only", `{"query":"foo bar"}`, `{"query":"baz qux"}`, true},
		{"number-type-only", `{"limit":100}`, `{"limit":50}`, true},
		{"array-order-sensitive", pathsABJSON, `{"paths":["b","a"]}`, false},
		{"array-same-order", pathsABJSON, pathsABJSON, true},
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
	t.Parallel()

	expected := []parity.ToolCall{
		{Name: toolRead, Input: json.RawMessage(srcXGoPathJSON)},
		{Name: toolBash, Input: json.RawMessage(commandGoTestJSON)},
	}
	// matching observed
	match := []parity.ToolCall{
		{Name: toolRead, Input: json.RawMessage(`{"file_path":"/abs/repo/src/x.go"}`)}, // path-normalized
		{Name: toolBash, Input: json.RawMessage(commandGoTestJSON)},
	}
	if c := parity.Compare(expected, match); !c.Match {
		t.Errorf("expected Match, got Layer1=%v Layer2=%v", c.Layer1SequenceMatch, c.Layer2ArgMismatches)
	}
	// wrong tool
	wrong := []parity.ToolCall{
		{Name: toolGrep, Input: json.RawMessage(`{"pattern":"x"}`)},
		{Name: toolBash, Input: json.RawMessage(commandGoTestJSON)},
	}
	if c := parity.Compare(expected, wrong); c.Match {
		t.Error("expected mismatch for substituted tool")
	}
}
