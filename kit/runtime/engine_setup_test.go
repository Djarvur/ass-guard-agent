package runtime //nolint:testpackage // internal package test

// 25-08 kit-native rework of the 25-05 engine-setup twin: the production
// loader lives app-side (acpserve's loadEngineSetup — OQ4/D-17); this file
// now provides the KIT-side fake family instead of mirroring the app loads —
// a fake pattern table carrying the SAME seeded rows the embedded openspec
// floor compiles (regex-for-regex, so every chaining assertion that matched
// through the real table matches through the fake), over a catalog the
// batteries' tools register into directly. Zero app imports. The REAL
// openspec table + config loading keep their own subjects in
// internal/openspec; the REAL app-side assembly is pinned by acpserve's
// serve suites. Zero Test functions — the D-20 ledger holds.

import (
	"regexp"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/kit/engine"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// seededPatternRow is one text-matching row of the embedded openspec floor
// (internal/openspec/seeded.toml) — the regex strings and actions are
// verbatim; a kit-side test constant, not an app import.
type seededPatternRow struct {
	id     string
	regex  string
	action engine.Action
	next   string
}

var seededPatternRows = []seededPatternRow{
	// The terminal shield: the archive closing ends the chain.
	{id: "post-archive-terminal", regex: `(?i)archive complete`, action: engine.ActionWait},
	{id: "post-propose-handoff", regex: "(?i)run `?/opsx:apply`?[^.]*to (start|begin|implement)",
		action: engine.ActionContinue, next: "/opsx:apply"},
	{id: "post-apply-handoff", regex: "(?i)archiv(e|ed)( this change)? with `?/opsx:archive`?",
		action: engine.ActionContinue, next: "/opsx:archive"},
	{id: "impl-complete", regex: `Implementation Complete.*ready for review`, action: engine.ActionContinue},
	{id: "spec-done", regex: `(?i)specification.*finalized`, action: engine.ActionContinue},
	{id: "changes-proposed", regex: `(?i)change proposal.*ready`, action: engine.ActionHook},
	{id: "post-verify-handoff", regex: "(?is)Verification Report.*(cannot be [a-z ]{0,40}archived|before archiving|incomplete)",
		action: engine.ActionContinue, next: "/opsx:continue"},
	{id: "post-new-continue-handoff", regex: "(?i)run `?/opsx:continue`?",
		action: engine.ActionContinue, next: "/opsx:continue"},
}

// seededHandoffTools are the tool-signal rows of the same floor.
var seededHandoffTools = map[string]engine.Action{
	"openspec_handoff":    engine.ActionContinue,
	"openspec_stage_done": engine.ActionContinue,
}

// seededCommandRows are the command-provenance rows (hybrid chaining).
var seededCommandRows = map[string]struct {
	action engine.Action
	next   string
}{
	"opsx:explore": {action: engine.ActionContinue, next: "/opsx:propose"},
}

// fakePatternTable implements engine.PatternTable + the optional
// CommandMatcher and NextPromptFor capabilities over the seeded rows — the
// kit-native twin of openspec.FromConfig's table (same rows, same
// first-match-wins order, same zero-value convention).
type fakePatternTable struct {
	rows    []compiledFakePattern
	tools   map[string]engine.Action
	nextFor map[string]string
}

type compiledFakePattern struct {
	id     string
	regex  *regexp.Regexp
	action engine.Action
}

func newFakePatternTable() *fakePatternTable {
	pt := &fakePatternTable{
		tools:   map[string]engine.Action{},
		nextFor: map[string]string{},
	}

	for _, r := range seededPatternRows {
		pt.rows = append(pt.rows, compiledFakePattern{
			id: r.id, regex: regexp.MustCompile(r.regex), action: r.action,
		})

		if r.next != "" {
			pt.nextFor[r.id] = r.next
		}
	}

	for tool, action := range seededHandoffTools {
		pt.tools[tool] = action
	}

	for cmd, row := range seededCommandRows {
		_ = cmd

		pt.nextFor["post-explore-handoff"] = row.next
	}

	return pt
}

// MatchText scans in declared order; first match wins (the floor's rule).
func (p *fakePatternTable) MatchText(text string) engine.MatchDetail {
	for _, r := range p.rows {
		if m := r.regex.FindString(text); m != "" {
			return engine.MatchDetail{ID: r.id, Action: r.action, Span: m, ConfigSource: "fake-seeded/" + r.id}
		}
	}

	return engine.MatchDetail{}
}

// MatchTool reports the handoff-tool rows.
func (p *fakePatternTable) MatchTool(name string) engine.MatchDetail {
	if action, ok := p.tools[name]; ok {
		return engine.MatchDetail{ID: "fake-tool-" + name, Action: action, Span: name, ConfigSource: "fake-seeded/tools"}
	}

	return engine.MatchDetail{}
}

// MatchCommand reports the command-provenance rows (hybrid chaining).
func (p *fakePatternTable) MatchCommand(key string) engine.MatchDetail {
	if _, ok := seededCommandRows[key]; ok {
		return engine.MatchDetail{
			ID: "post-explore-handoff", Action: engine.ActionContinue,
			Span: key, ConfigSource: "fake-seeded/commands",
		}
	}

	return engine.MatchDetail{}
}

// NextPromptFor answers the chaining rows' next /opsx:* command (the
// patternNextPrompter capability the dispatcher's ContinuePopulator reads).
func (p *fakePatternTable) NextPromptFor(patternID string) string {
	return p.nextFor[patternID]
}

// testEngineSetup builds the kit-native engine setup: an empty catalog (the
// batteries' engines execute scripted turns; tools register through the
// toolkit seam) + the seeded fake pattern table. Learned stays nil (tests
// that pin learning inject a fakeLearned through the same EngineSetup
// field).
func testEngineSetup(t *testing.T) EngineSetup {
	t.Helper()

	return EngineSetup{Catalog: toolcat.NewCatalog(), PatternTable: newFakePatternTable()}
}

// fakeLearned is the kit-side Learned port fake: an in-memory map answering
// Lookup plus the record/count bookkeeping. The REAL store's threshold and
// persistence semantics keep their own subject in internal/learning's suite;
// the kit batteries pin the PORT contract (wired through EngineSetup,
// consulted by the engine ask path). Mutex-guarded: RecordCandidate fires on
// the ask-pump goroutine while the test goroutine polls Lookup/EntryCount —
// the unsynchronized map was one DATA RACE failing the whole ask-wiring
// cluster under -race (2026-09-27 nightly baseline).
type fakeLearned struct {
	mu       sync.Mutex
	entries  map[string]string
	recorded int
}

func newFakeLearned() *fakeLearned { return &fakeLearned{entries: map[string]string{}} }

func (f *fakeLearned) Lookup(situation string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	ans, ok := f.entries[situation]

	return ans, ok
}

func (f *fakeLearned) RecordCandidate(situation, answer, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.entries[situation] = answer
	f.recorded++

	return nil
}

func (f *fakeLearned) EntryCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.entries)
}
