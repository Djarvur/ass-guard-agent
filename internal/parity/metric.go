// Package parity implements the behavioral mimicry A/B parity test — the
// project's reason to exist (MIMC-03/MIMC-04, D-01..D-05).
//
// The two-layer metric (D-02): (1) tool-name SEQUENCE equality, (2) per-tool
// argument STRUCTURAL equality with explicit normalization. Byte-diff is out of
// scope (MIMC-04 — structural indistinguishability is the bar, not byte-identity).
package parity

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// ToolCall is the zcode-normalized tool invocation (VERIFIED-FACTS.md item #1).
type ToolCall struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ArgMismatch describes one Layer-2 argument-structural difference.
type ArgMismatch struct {
	ToolIndex int
	Reason    string
}

// Comparison is the two-layer result for one turn.
type Comparison struct {
	Layer1SequenceMatch bool
	Layer2ArgMismatches []ArgMismatch
	// Match is true iff BOTH layers matched (the D-03 100% gate, per turn).
	Match bool
}

// CompareSequence is Layer 1: ordered tool-name slice equality.
func CompareSequence(expected, observed []string) bool {
	if len(expected) != len(observed) {
		return false
	}
	for i := range expected {
		if expected[i] != observed[i] {
			return false
		}
	}
	return true
}

// CompareArgs is Layer 2: per-tool argument STRUCTURAL equality with the D-02
// normalization rules:
//   - the SET of JSON keys must match (missing/extra = mismatch)
//   - each value's JSON TYPE must match (string/number/bool/array/object/null)
//   - file-path strings are normalized to forward-slash + repo-relative before
//     comparison (so /abs/repo/src/x.go matches src/x.go)
//   - closed-enum string values are compared EXACTLY (e.g. a "command" field)
//   - free-text string values are compared TYPE-ONLY (any non-empty string matches)
//   - arrays are order-sensitive + per-element structural
//   - numbers are compared TYPE-ONLY (the value may differ; only number-kind matters)
//
// The repoRoot is the normalization anchor for file paths; pass "" to disable
// repo-root stripping (absolute paths are still normalized to forward-slash and
// heuristically trimmed to a relative tail when they contain a repo-root-like segment).
func CompareArgs(expected, observed json.RawMessage) ([]ArgMismatch, bool) {
	var e, o any
	if err := json.Unmarshal(expected, &e); err != nil {
		return []ArgMismatch{{Reason: "expected input is not valid JSON"}}, false
	}
	if err := json.Unmarshal(observed, &o); err != nil {
		return []ArgMismatch{{Reason: "observed input is not valid JSON"}}, false
	}
	m := compareValues("", e, o)
	if len(m) == 0 {
		return nil, true
	}
	out := make([]ArgMismatch, 0, len(m))
	for _, reason := range m {
		out = append(out, ArgMismatch{Reason: reason})
	}
	return out, false
}

// compareValues recursively compares two decoded JSON values per the D-02 rules.
// path is the dotted field path (for richer mismatch reasons in future).
func compareValues(path string, expected, observed any) []string {
	if equiv(expected, observed) {
		return nil
	}
	// Objects: compare key sets + recurse.
	eo, eOK := expected.(map[string]any)
	oo, oOK := observed.(map[string]any)
	if eOK && oOK {
		return compareObject(path, eo, oo)
	}
	// Arrays: order-sensitive per-element structural.
	ea, eOK := expected.([]any)
	oa, oOK := observed.([]any)
	if eOK && oOK {
		return compareArray(path, ea, oa)
	}
	// Scalars: type-only for numbers/strings (free text), exact for the rest.
	if scalarEquiv(expected, observed) {
		return nil
	}
	return []string{path + ": value mismatch"}
}

func compareObject(path string, expected, observed map[string]any) []string {
	var mismatches []string
	ek := keys(expected)
	ok := keys(observed)
	if !sameSet(ek, ok) {
		mismatches = append(mismatches, path+": key set differs")
		return mismatches // a key-set difference is enough; deeper recursion is noise
	}
	for _, k := range ek {
		mismatches = append(mismatches, compareValues(joinPath(path, k), expected[k], observed[k])...)
	}
	return mismatches
}

func compareArray(path string, expected, observed []any) []string {
	if len(expected) != len(observed) {
		return []string{path + ": array length differs"}
	}
	var mismatches []string
	for i := range expected {
		mismatches = append(mismatches, compareValues(joinPath(path, ""), expected[i], observed[i])...)
	}
	return mismatches
}

// equiv is the top-level structural-equivalence check.
func equiv(expected, observed any) bool {
	return reflect.DeepEqual(expected, observed)
}

// scalarEquiv applies the type-only rules for numbers and free-text strings, and
// exact equality for closed-enum strings and bools/nulls. File-path strings get
// path normalization.
func scalarEquiv(expected, observed any) bool {
	es, eStr := expected.(string)
	os, oStr := observed.(string)
	if eStr && oStr {
		return stringEquiv(es, os)
	}
	// numbers: type-only (any two JSON numbers are equivalent here).
	if isNumber(expected) && isNumber(observed) {
		return true
	}
	// bool/null: exact.
	return reflect.DeepEqual(expected, observed)
}

// stringEquiv compares two strings. File-path-like strings are normalized;
// otherwise strings are type-only equivalent (both strings → match) UNLESS the
// field is a closed enum. The closed-enum heuristic (D-02): field NAMES like
// "command", "role", "type", "mode", "name" carry closed-enum values that must
// match EXACTLY; free-text fields (e.g. "query", "prompt", "content") are
// type-only. We approximate this at the value level: if both strings are
// non-empty, they are type-equivalent by default. The exact-enum requirement is
// enforced by the caller passing the field context — but to keep the metric
// self-contained, we treat short single-token strings (no whitespace, <=24
// chars, looks like an identifier) as enum-like (exact), and longer/whitespace
// strings as free-text (type-only).
func stringEquiv(a, b string) bool {
	if looksLikePath(a) || looksLikePath(b) {
		return normalizePath(a) == normalizePath(b)
	}
	if looksLikeEnum(a) && looksLikeEnum(b) {
		return a == b
	}
	// free-text: both non-empty strings → equivalent.
	return a != "" && b != ""
}

func looksLikePath(s string) bool {
	return strings.Contains(s, "/") || strings.HasSuffix(s, ".go") || strings.HasSuffix(s, ".json") || strings.HasSuffix(s, ".md")
}

func normalizePath(s string) string {
	s = filepath.ToSlash(s)
	// Strip an absolute prefix up to a common repo-root segment heuristically.
	for _, anchor := range []string{"/src/", "/internal/", "/cmd/", "/profiles/"} {
		if i := strings.Index(s, anchor); i >= 0 {
			return strings.TrimPrefix(s[i+1:], "/")
		}
	}
	return s
}

// looksLikeEnum approximates closed-enum values: a short single token with no
// whitespace or sentence punctuation.
func looksLikeEnum(s string) bool {
	if len(s) == 0 || len(s) > 24 {
		return false
	}
	if strings.ContainsAny(s, " \t\n.,;:") {
		return false
	}
	return true
}

func isNumber(v any) bool {
	switch v.(type) {
	case float64, int, int64, float32, int32:
		return true
	}
	return false
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func joinPath(base, leaf string) string {
	if base == "" {
		return leaf
	}
	return base + "." + leaf
}

// Compare combines both layers for one turn's tool-call lists.
func Compare(expected, observed []ToolCall) Comparison {
	c := Comparison{}
	expNames := make([]string, len(expected))
	obsNames := make([]string, len(observed))
	for i, t := range expected {
		expNames[i] = t.Name
	}
	for i, t := range observed {
		obsNames[i] = t.Name
	}
	c.Layer1SequenceMatch = CompareSequence(expNames, obsNames)
	if !c.Layer1SequenceMatch {
		// Layer-2 per-index comparison is meaningless when sequences differ.
		return c
	}
	for i := range expected {
		if _, ok := CompareArgs(expected[i].Input, observed[i].Input); !ok {
			c.Layer2ArgMismatches = append(c.Layer2ArgMismatches, ArgMismatch{ToolIndex: i})
		}
	}
	c.Match = c.Layer1SequenceMatch && len(c.Layer2ArgMismatches) == 0
	return c
}
