package openspec_test

import (
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/openspec"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// TestRegisterTools_Mutability verifies RegisterTools registers each command
// under "openspec:<name>" with its declared mutability.
func TestRegisterTools_Mutability(t *testing.T) {
	t.Parallel()

	cfg := &openspec.OpenSpecConfig{
		Commands: map[string]openspec.CommandShape{
			cmdList: {Mutability: "read-only"},
			"apply": {Mutability: classMutating},
		},
	}

	cat := toolcat.NewCatalog()

	err := openspec.RegisterTools(cat, cfg)
	if err != nil {
		t.Fatalf("RegisterTools: %v", err)
	}

	list, ok := cat.Get("openspec:" + cmdList)
	if !ok {
		t.Fatal("openspec:list not registered")
	}

	if list.IsMutating() {
		t.Error("openspec:list classified mutating; want read-only")
	}

	apply, ok := cat.Get("openspec:apply")
	if !ok {
		t.Fatal("openspec:apply not registered")
	}

	if !apply.IsMutating() {
		t.Error("openspec:apply classified read-only; want mutating")
	}
}

// TestRegisterTools_DrivesIsBoundary verifies the registered mutability drives
// toolcat.IsBoundary with NO boundary-engine change (OPEN-03).
func TestRegisterTools_DrivesIsBoundary(t *testing.T) {
	t.Parallel()

	cfg := &openspec.OpenSpecConfig{
		Commands: map[string]openspec.CommandShape{
			cmdList: {Mutability: "read-only"},
			"apply": {Mutability: classMutating},
		},
	}
	cat := toolcat.NewCatalog()

	_ = openspec.RegisterTools(cat, cfg)
	if toolcat.IsBoundary("openspec:apply", cat, nil) {
		// good
	} else {
		t.Error("IsBoundary(openspec:apply) = false; want true (mutating ⇒ boundary)")
	}

	if toolcat.IsBoundary("openspec:list", cat, nil) {
		t.Error("IsBoundary(openspec:list) = true; want false (read-only)")
	}
}

// TestPatternTable_MatchTextFirstWins verifies MatchText scans in declared order
// + the first match wins; unmatched ⇒ ActionNothing.
func TestPatternTable_MatchTextFirstWins(t *testing.T) {
	t.Parallel()

	cfg := &openspec.OpenSpecConfig{
		Patterns: []openspec.PatternEntry{
			{ID: statusImplComplete, Regex: "Implementation Complete.*ready for review", Action: stopContinue},
			{ID: "spec-done", Regex: "(?i)specification.*finalized", Action: stopContinue},
		},
	}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	if d := pt.MatchText("... Implementation Complete — ready for review ..."); d.ID != statusImplComplete ||
		d.Action != engine.ActionContinue {
		t.Errorf("MatchText(impl) = (%q,%v); want (impl-complete, continue)", d.ID, d.Action)
	}

	if d := pt.MatchText("the Specification is now finalized"); d.ID != "spec-done" ||
		d.Action != engine.ActionContinue {
		t.Errorf("MatchText(spec) = (%q,%v); want (spec-done, continue)", d.ID, d.Action)
	}

	if d := pt.MatchText("totally unrelated text"); d.ID != "" || d.Action != engine.ActionNothing {
		t.Errorf("MatchText(unmatched) = (%q,%v); want (\"\", Nothing)", d.ID, d.Action)
	}
}

// TestPatternTable_MatchTool verifies MatchTool does an exact-name match +
// unknown ⇒ ActionNothing.
func TestPatternTable_MatchTool(t *testing.T) {
	t.Parallel()

	cfg := &openspec.OpenSpecConfig{
		HandoffTools: []openspec.HandoffToolEntry{
			{ID: "os-handoff", Tool: "openspec_handoff", Action: stopContinue},
		},
	}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	if d := pt.MatchTool("openspec_handoff"); d.ID != "os-handoff" || d.Action != engine.ActionContinue {
		t.Errorf("MatchTool(handoff) = (%q,%v); want (os-handoff, continue)", d.ID, d.Action)
	}

	if d := pt.MatchTool("Read"); d.ID != "" || d.Action != engine.ActionNothing {
		t.Errorf("MatchTool(Read) = (%q,%v); want (\"\", Nothing)", d.ID, d.Action)
	}
}

// TestPatternTable_SatisfiesEngineInterface is the bridge assertion — the
// OpenSpecPatternTable is a valid engine.PatternTable (D-02).
func TestPatternTable_SatisfiesEngineInterface(t *testing.T) {
	t.Parallel()

	pt, err := openspec.FromConfig(&openspec.OpenSpecConfig{})
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	var _ engine.PatternTable = pt // compile-time check (also in patterntable.go)
}

// TestPatternTable_ActionMapping verifies the TOML action strings map to the
// engine.Action values (continue/hook/ask/wait).
func TestPatternTable_ActionMapping(t *testing.T) {
	t.Parallel()

	cfg := &openspec.OpenSpecConfig{
		Patterns: []openspec.PatternEntry{
			{ID: "c", Regex: "x", Action: stopContinue},
			{ID: "h", Regex: "y", Action: "hook"},
			{ID: "a", Regex: "z", Action: "ask"},
			{ID: "w", Regex: "q", Action: "wait"},
		},
	}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	cases := map[string]engine.Action{
		"x": engine.ActionContinue,
		"y": engine.ActionHook,
		"z": engine.ActionAsk,
		"q": engine.ActionWait,
	}
	for text, wantAct := range cases {
		if got := pt.MatchText(text).Action; got != wantAct {
			t.Errorf("MatchText(%q) action = %v; want %v", text, got, wantAct)
		}
	}
}

// TestRegisterTools_NilArgs verifies nil catalog/config yields a structured error
// (never a panic).
func TestRegisterTools_NilArgs(t *testing.T) {
	t.Parallel()

	err := openspec.RegisterTools(nil, &openspec.OpenSpecConfig{})
	if err == nil {
		t.Error("RegisterTools(nil catalog) = nil; want error")
	}

	err = openspec.RegisterTools(toolcat.NewCatalog(), nil)
	if err == nil {
		t.Error("RegisterTools(nil cfg) = nil; want error")
	}
}

// TestPatternTable_MatchDetailSpan (09-02 T1 Test 5): MatchText returns the
// LITERAL matched substring via FindString — not the whole text, not a boolean.
func TestPatternTable_MatchDetailSpan(t *testing.T) {
	t.Parallel()

	cfg := &openspec.OpenSpecConfig{
		Patterns: []openspec.PatternEntry{
			{ID: "ready-next", Regex: "ready to (implement|apply)", Action: stopContinue},
		},
	}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	text := "prefix noise … ready to implement … suffix noise"
	d := pt.MatchText(text)

	if d.Span != "ready to implement" {
		t.Errorf("Span = %q; want the literal matched substring (FindString), not %q", d.Span, text)
	}
}

// TestPatternTable_MatchDetailConfigSource (09-02 T1 Test 6): the table names
// its own entries — patterns/… for text matches, handoff_tools/… for tools.
func TestPatternTable_MatchDetailConfigSource(t *testing.T) {
	t.Parallel()

	cfg := &openspec.OpenSpecConfig{
		Patterns: []openspec.PatternEntry{
			{ID: "spec-done", Regex: "specification finalized", Action: stopContinue},
		},
		HandoffTools: []openspec.HandoffToolEntry{
			{ID: "os-handoff", Tool: "openspec_handoff", Action: stopContinue},
		},
	}

	pt, err := openspec.FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	if d := pt.MatchText("the specification finalized ok"); d.ConfigSource != "openspec.toml patterns/spec-done" {
		t.Errorf("MatchText ConfigSource = %q; want openspec.toml patterns/spec-done", d.ConfigSource)
	}

	d := pt.MatchTool("openspec_handoff")
	if d.ConfigSource != "openspec.toml handoff_tools/os-handoff" {
		t.Errorf("MatchTool ConfigSource = %q; want openspec.toml handoff_tools/os-handoff", d.ConfigSource)
	}

	if d.Span != "openspec_handoff" {
		t.Errorf("MatchTool Span = %q; want the tool NAME as the span", d.Span)
	}
}
