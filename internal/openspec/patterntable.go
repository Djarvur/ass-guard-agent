package openspec

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
)

var errFromconfigRequiresA = errors.New("openspec: FromConfig requires a non-nil config")

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

	// nextFor maps pattern id → the next /opsx:* command text (08-06 chaining,
	// D-12). Consumed by the dispatcher's ContinuePopulator — NOT part of the
	// engine.PatternTable interface (which stays unwidened).
	nextFor map[string]string
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
		return nil, errFromconfigRequiresA
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

		if p.Next != "" {
			if pt.nextFor == nil {
				pt.nextFor = map[string]string{}
			}

			pt.nextFor[p.ID] = p.Next
		}
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

// configSourcePatterns / configSourceHandoffTools are the ConfigSource
// prefixes the table supplies per match (09-02, AUD-04/D-03 — the engine is
// table-agnostic; the TABLE names its own entries). Identifiers only, never
// file contents.
const (
	configSourcePatterns     = "openspec.toml patterns/"
	configSourceHandoffTools = "openspec.toml handoff_tools/"
)

// MatchText scans the patterns in declared order; the FIRST match wins. The
// returned MatchDetail carries the LITERAL matched substring (FindString —
// not the whole text, not a boolean) + the config source naming the entry.
// No match ⇒ the zero value — the engine's structural-safety cell (D-03).
func (t *OpenSpecPatternTable) MatchText(text string) engine.MatchDetail {
	for _, p := range t.textPatterns {
		if span := p.regex.FindString(text); span != "" {
			return engine.MatchDetail{
				ID:           p.id,
				Action:       p.action,
				Span:         span,
				ConfigSource: configSourcePatterns + p.id,
			}
		}
	}

	return engine.MatchDetail{}
}

// NextPromptFor returns the next-command text configured for a pattern id
// (08-06 chaining); "" when the id is unknown or carries no next — the
// engine's generic continue fallback then applies.
func (t *OpenSpecPatternTable) NextPromptFor(patternID string) string {
	return t.nextFor[patternID]
}

// MatchTool reports whether name is a known handoff tool-call (exact-name
// match — D-02 second signal). The returned MatchDetail carries the tool NAME
// as the Span (TurnOutput carries names, not inputs — the name IS the matched
// signal text) + the handoff entry's config source. Unknown ⇒ zero value.
func (t *OpenSpecPatternTable) MatchTool(name string) engine.MatchDetail {
	if e, ok := t.handoffTools[name]; ok {
		return engine.MatchDetail{
			ID:           e.id,
			Action:       e.action,
			Span:         name,
			ConfigSource: configSourceHandoffTools + e.id,
		}
	}

	return engine.MatchDetail{}
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
		//nolint:err113 // dynamic error message
		return engine.ActionNothing, fmt.Errorf("unknown action %q (want continue/hook/ask/wait)", s)
	}
}

// Compile-time assertion: OpenSpecPatternTable satisfies engine.PatternTable
// (D-02 — the bridge to the engine's dual-signal detector).
var _ engine.PatternTable = (*OpenSpecPatternTable)(nil)
