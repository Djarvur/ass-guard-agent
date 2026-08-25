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
//
// Next (08-06 D-12 chaining) is the NEXT /opsx:* command text injected when
// this pattern matches (e.g. `next = "/opsx:apply"`); empty means the engine's
// generic continue prompt applies. Ids are stage-bearing
// ("post-<stage>-handoff") so hooks fire per stage.
type PatternEntry struct {
	ID     string `toml:"id"`
	Regex  string `toml:"regex"`
	Action string `toml:"action"`
	Next   string `toml:"next"`
}

// HandoffToolEntry is one [[handoff_tools]] row: a tool-call name + the action
// the engine takes when the model invokes it (D-02 tool signal).
type HandoffToolEntry struct {
	ID     string `toml:"id"`
	Tool   string `toml:"tool"`
	Action string `toml:"action"`
}

// CommandPatternEntry is one [[command_patterns]] row (hybrid chaining — the
// findings-6 disposition, 2026-08-15): a registry command key whose EXPANSION
// started a turn carries a configured action when the turn completes — the
// deterministic chaining mechanism for boundaries whose closing text is
// architecturally free-form (the /opsx:explore close; the toolkit's own command
// guardrails mandate it). Consulted by engine.Decide ONLY after the text/tool
// signals miss: the capture-seeded regex rows stay authoritative for the
// deterministic boundaries.
//
// Ids are stage-bearing ("post-<stage>-handoff") so hooks fire per stage, and
// Next is the /opsx:* command injected on match (consumed through the same
// NextPromptFor field the [[patterns]] rows use); empty means the engine's
// generic continue prompt applies.
type CommandPatternEntry struct {
	ID      string `toml:"id"`
	Command string `toml:"command"`
	Action  string `toml:"action"`
	Next    string `toml:"next"`
}

// CommandShape declares one OpenSpec command's classification + subprocess
// guards (D-15 / OPEN-03 — the single source of truth for context boundaries; a
// mutating command IS a boundary via the existing toolcat.IsBoundary floor).
//
// Argv is the exact subprocess argv prefix when it differs from the key
// (multi-word subcommands, e.g. key "new-change" → argv "new change"); empty
// means "use the key". TimeoutSecs bounds EACH subprocess run (distinct from
// the turn ctx); 0 at load time defaults to DefaultTimeoutSecs. ExitClass
// ("fixable" or empty) marks commands whose non-zero exits are actionable
// findings rather than hard errors (validate/doctor/archive — D-10).
type CommandShape struct {
	Mutability  string `toml:"mutability"`
	Argv        string `toml:"argv"`
	TimeoutSecs int    `toml:"timeout_secs"`
	ExitClass   string `toml:"exit_class"`
}

// Command-guard defaults applied at load time (Phase-8 CMD-03).
const (
	// DefaultTimeoutSecs bounds each openspec subprocess when an entry does not
	// declare its own.
	DefaultTimeoutSecs = 60

	// ExitClassFixable marks a non-zero exit as an actionable finding.
	ExitClassFixable = "fixable"

	// MutabilityMutating / MutabilityReadOnly are the mutability vocabulary for
	// BOTH the [commands] shapes and the [command_mutability] table.
	MutabilityMutating = "mutating"
	MutabilityReadOnly = "read-only"
)

// OpenSpecConfig is the parsed openspec.toml (D-14).
type OpenSpecConfig struct {
	Patterns     []PatternEntry          `toml:"patterns"`
	HandoffTools []HandoffToolEntry      `toml:"handoff_tools"`
	Commands     map[string]CommandShape `toml:"commands"`

	// CommandPatterns are the command-provenance chaining rows (hybrid
	// chaining, findings-6 disposition) — see CommandPatternEntry.
	CommandPatterns []CommandPatternEntry `toml:"command_patterns"`

	// CommandMutability classifies DISCOVERED slash-commands (08-04 D-11 —
	// the v1.0 mutability discipline extended from tools to commands): a
	// "mutating" command opens a context boundary at expansion time;
	// "read-only" (and unknown keys) never do. Keys are ecosystem command
	// keys ("opsx:apply"); an operator overlay flips classifications per the
	// normal loader precedence.
	CommandMutability map[string]string `toml:"command_mutability"`
}

// embeddedSeeded is the zero-config floor (OPEN-02), embedded into the binary.
//
//go:embed seeded.toml
var embeddedSeeded []byte

// ConfigError lists every validation violation found in one pass (collect-all —
// mirrors modelrouting.ConfigError / hookdag.ConfigError).
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

	for i := range cfg.CommandPatterns {
		v = append(v, validateCommandPattern(i, cfg.CommandPatterns[i])...)
	}

	for name, shape := range cfg.Commands {
		v = append(v, validateCommand(name, shape)...)
	}

	for key, mut := range cfg.CommandMutability {
		if mut != MutabilityMutating && mut != MutabilityReadOnly {
			v = append(v, fmt.Sprintf(
				"command_mutability.%s: unknown mutability %q (want mutating|read-only)", key, mut))
		}
	}

	if len(v) > 0 {
		return &ConfigError{Violations: v}
	}

	applyCommandDefaults(cfg)

	return nil
}

// validateCommand returns the violations for one [commands] entry.
func validateCommand(name string, shape CommandShape) []string {
	var v []string

	if shape.Mutability != MutabilityMutating && shape.Mutability != MutabilityReadOnly {
		v = append(v, fmt.Sprintf(
			"commands.%s: unknown mutability %q (want mutating|read-only)",
			name, shape.Mutability))
	}

	if shape.ExitClass != "" && shape.ExitClass != ExitClassFixable {
		v = append(v, fmt.Sprintf(
			"commands.%s: unknown exit_class %q (want %q or empty)",
			name, shape.ExitClass, ExitClassFixable))
	}

	if shape.TimeoutSecs < 0 {
		v = append(v, fmt.Sprintf(
			"commands.%s: negative timeout_secs %d", name, shape.TimeoutSecs))
	}

	return v
}

// validateCommandPattern returns the violations for one [[command_patterns]]
// entry (index for the collect-all message): a known action verb + a non-empty
// command key.
func validateCommandPattern(i int, c CommandPatternEntry) []string {
	var v []string

	if !validAction(c.Action) {
		v = append(v, fmt.Sprintf(
			"command_patterns[%d] %q: unknown action %q (want continue/hook/ask/wait)",
			i, c.ID, c.Action))
	}

	if c.Command == "" {
		v = append(v, fmt.Sprintf("command_patterns[%d] %q: empty command", i, c.ID))
	}

	return v
}

// applyCommandDefaults fills per-entry defaults: argv defaults to the key;
// timeout defaults to DefaultTimeoutSecs. Idempotent.
func applyCommandDefaults(cfg *OpenSpecConfig) {
	for name, shape := range cfg.Commands {
		if shape.Argv == "" {
			shape.Argv = name
		}

		if shape.TimeoutSecs == 0 {
			shape.TimeoutSecs = DefaultTimeoutSecs
		}

		cfg.Commands[name] = shape
	}
}

func validAction(a string) bool {
	return a == ActionContinue || a == ActionHook || a == ActionAsk || a == ActionWait
}
