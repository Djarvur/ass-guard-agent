package openspec

import (
	_ "embed"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Valid action strings a [[patterns]] / [[handoff_tools]] entry may declare
// (mapped to engine.Action by parseAction in patterntable.go).
const (
	ActionContinue = "continue"
	ActionHook     = "hook"
	ActionAsk      = "ask"
	ActionWait     = "wait"
)

// PatternEntry is one [[patterns]] row: a regex matched against the assistant
// text + the action the engine takes on match (D-02 text signal).
type PatternEntry struct {
	ID     string `toml:"id"`
	Regex  string `toml:"regex"`
	Action string `toml:"action"`
}

// HandoffToolEntry is one [[handoff_tools]] row: a tool-call name + the action
// the engine takes when the model invokes it (D-02 tool signal).
type HandoffToolEntry struct {
	ID     string `toml:"id"`
	Tool   string `toml:"tool"`
	Action string `toml:"action"`
}

// CommandShape declares one OpenSpec command's mutability (D-15 / OPEN-03 — the
// single source of truth for context boundaries; a mutating command IS a
// boundary via the existing toolcat.IsBoundary floor).
type CommandShape struct {
	Mutability string `toml:"mutability"`
}

// OpenSpecConfig is the parsed openspec.toml (D-14).
type OpenSpecConfig struct {
	Patterns     []PatternEntry          `toml:"patterns"`
	HandoffTools []HandoffToolEntry      `toml:"handoff_tools"`
	Commands     map[string]CommandShape `toml:"commands"`
}

// embeddedSeeded is the zero-config floor (OPEN-02), embedded into the binary.
//
//go:embed seeded.toml
var embeddedSeeded []byte

// ConfigError lists every validation violation found in one pass (collect-all —
// mirrors scheduler.ConfigError / hookdag.ConfigError).
type ConfigError struct {
	Violations []string
}

// Error joins the violations with "; " — investigate-and-fix-ready.
func (e *ConfigError) Error() string {
	if len(e.Violations) == 0 {
		return "openspec config invalid"
	}

	cp := append([]string(nil), e.Violations...)
	sort.Strings(cp)

	return strings.Join(cp, "; ")
}

// LoadConfig parses path with BurntSushi/toml + validates it. The regex/action/
// mutability fields are checked collect-all; a malformed config yields a
// *ConfigError naming every offender. The regex is NOT compiled here (it is
// compiled once in patterntable.go's FromConfig, which is the only consumer).
func LoadConfig(path string) (*OpenSpecConfig, error) {
	var cfg OpenSpecConfig
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, fmt.Errorf("openspec: decode %q: %w", path, err)
	}

	err = validate(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DefaultConfig returns the embedded zero-config floor (OPEN-02).
func DefaultConfig() (*OpenSpecConfig, error) {
	var cfg OpenSpecConfig
	_, err := toml.Decode(string(embeddedSeeded), &cfg)
	if err != nil {
		return nil, fmt.Errorf("openspec: decode embedded seeded.toml: %w", err)
	}

	err = validate(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate checks the config collect-all: every action is a known verb, every
// mutability is mutating|read-only, every pattern regex compiles. Pattern/tool
// ids may be empty (a missing id is flagged).
func validate(cfg *OpenSpecConfig) error {
	var v []string

	for i := range cfg.Patterns {
		p := &cfg.Patterns[i]
		if !validAction(p.Action) {
			v = append(v, fmt.Sprintf(
				"patterns[%d] %q: unknown action %q (want continue/hook/ask/wait)",
				i, p.ID, p.Action))
		}

		if p.Regex == "" {
			v = append(v, fmt.Sprintf("patterns[%d] %q: empty regex", i, p.ID))

			continue
		}

		_, err := regexp.Compile(p.Regex)
		if err != nil {
			v = append(v, fmt.Sprintf("patterns[%d] %q: invalid regex %q: %v", i, p.ID, p.Regex, err))
		}
	}

	for i := range cfg.HandoffTools {
		h := &cfg.HandoffTools[i]
		if !validAction(h.Action) {
			v = append(v, fmt.Sprintf(
				"handoff_tools[%d] %q: unknown action %q (want continue/hook/ask/wait)",
				i, h.ID, h.Action))
		}

		if h.Tool == "" {
			v = append(v, fmt.Sprintf("handoff_tools[%d] %q: empty tool", i, h.ID))
		}
	}

	for name, shape := range cfg.Commands {
		if shape.Mutability != "mutating" && shape.Mutability != "read-only" {
			v = append(v, fmt.Sprintf(
				"commands.%s: unknown mutability %q (want mutating|read-only)",
				name, shape.Mutability))
		}
	}

	if len(v) > 0 {
		return &ConfigError{Violations: v}
	}

	return nil
}

func validAction(a string) bool {
	return a == ActionContinue || a == ActionHook || a == ActionAsk || a == ActionWait
}
