package openspec

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
)

// compiledPattern is one [[patterns]] row with its regex compiled once at load.
type compiledPattern struct {
	id     string
	regex  *regexp.Regexp
	action engine.Action
}

// OpenSpecPatternTable bridges the openspec.toml config to the engine's
// PatternTable interface (D-02). It compiles each pattern's regex once at
// construction (FromConfig) so MatchText is a cheap scan, never a recompile.
// It satisfies engine.PatternTable (the compile-time assertion below).
type OpenSpecPatternTable struct {
	textPatterns []compiledPattern // declared order; first match wins
	handoffTools map[string]toolEntry
}

type toolEntry struct {
	id     string
	action engine.Action
}

// FromConfig compiles cfg into an OpenSpecPatternTable. A malformed regex or
// unknown action string (the load-time validate should have caught these, but
// FromConfig is defensive) yields a structured error.
func FromConfig(cfg *OpenSpecConfig) (*OpenSpecPatternTable, error) {
	if cfg == nil {
		return nil, errors.New("openspec: FromConfig requires a non-nil config")
	}

	pt := &OpenSpecPatternTable{handoffTools: map[string]toolEntry{}}

	for _, p := range cfg.Patterns {
		act, err := parseAction(p.Action)
		if err != nil {
			return nil, fmt.Errorf("openspec: pattern %q: %w", p.ID, err)
		}

		re, err := regexp.Compile(p.Regex)
		if err != nil {
			return nil, fmt.Errorf("openspec: pattern %q regex %q: %w", p.ID, p.Regex, err)
		}

		pt.textPatterns = append(pt.textPatterns, compiledPattern{id: p.ID, regex: re, action: act})
	}

	for _, h := range cfg.HandoffTools {
		act, err := parseAction(h.Action)
		if err != nil {
			return nil, fmt.Errorf("openspec: handoff tool %q: %w", h.ID, err)
		}

		pt.handoffTools[h.Tool] = toolEntry{id: h.ID, action: act}
	}

	return pt, nil
}

// MatchText scans the patterns in declared order; the FIRST match wins (returns
// its id + action). No match ⇒ ("", engine.ActionNothing) — the engine's
// structural-safety cell (D-03).
func (t *OpenSpecPatternTable) MatchText(text string) (string, engine.Action) {
	for _, p := range t.textPatterns {
		if p.regex.MatchString(text) {
			return p.id, p.action
		}
	}

	return "", engine.ActionNothing
}

// MatchTool reports whether name is a known handoff tool-call + returns its id +
// action (exact-name match — D-02 second signal). Unknown ⇒ ActionNothing.
func (t *OpenSpecPatternTable) MatchTool(name string) (string, engine.Action) {
	if e, ok := t.handoffTools[name]; ok {
		return e.id, e.action
	}

	return "", engine.ActionNothing
}

// parseAction maps a TOML action string to the engine.Action enum. An unknown
// verb yields a structured error (the load-time validate catches these first,
// but FromConfig is defensive against programmatic configs).
func parseAction(s string) (engine.Action, error) {
	switch s {
	case ActionContinue, "":
		return engine.ActionContinue, nil
	case ActionHook:
		return engine.ActionHook, nil
	case ActionAsk:
		return engine.ActionAsk, nil
	case ActionWait:
		return engine.ActionWait, nil
	default:
		return engine.ActionNothing, fmt.Errorf("unknown action %q (want continue/hook/ask/wait)", s)
	}
}

// Compile-time assertion: OpenSpecPatternTable satisfies engine.PatternTable
// (D-02 — the bridge to the engine's dual-signal detector).
var _ engine.PatternTable = (*OpenSpecPatternTable)(nil)
