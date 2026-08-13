package hookdag

import (
	_ "embed"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// OnFailure is the per-step + per-hook failure policy (HOOK-03).
type OnFailure string

const (
	// OnFailureHalt stops the chain immediately (default for steps that mutate).
	OnFailureHalt OnFailure = "halt"
	// OnFailureContinue logs the error via HookProgress + proceeds (advisory
	// steps like lint after test).
	OnFailureContinue OnFailure = "continue"
	// OnFailureAsk surfaces the failure to the user via HookProgress{ask} + the
	// chain SUSPENDS (the engine in 04-05 surfaces it as a session/update).
	OnFailureAsk OnFailure = "ask"
)

// StepKind discriminates the four step types (HOOK-01 / D-08).
type StepKind string

const (
	// StepRunCommand runs a shell command and checks its exit code.
	StepRunCommand StepKind = "run-command"
	// StepSendPrompt injects a prompt as a nested turn.
	StepSendPrompt StepKind = "send-prompt"
	// StepFreshContext opens a fresh-context boundary.
	StepFreshContext StepKind = "fresh-context"
	// StepWait sleeps for the configured duration.
	StepWait StepKind = "wait"
)

// Step is one node of a hook's DAG (declared order — D-07 walks-as-declared;
// parallel-within-a-rank is a v1.1 concern and is NOT required for the seeded
// set).
type Step struct {
	Name      string    `yaml:"name"`
	Kind      StepKind  `yaml:"kind"`
	OnFailure OnFailure `yaml:"on_failure"`         // defaults to the hook's when empty
	Command   string    `yaml:"command,omitempty"`  // run-command: the executable
	Args      []string  `yaml:"args,omitempty"`     // run-command: the argv tail
	Prompt    string    `yaml:"prompt,omitempty"`   // send-prompt: the turn prompt
	Duration  string    `yaml:"duration,omitempty"` // wait: Go duration string
}

// Hook is one declarative hook entry.
type Hook struct {
	Name           string    `yaml:"name"`
	Trigger        string    `yaml:"trigger"` // "post-implement" | "post-phase" | custom stage
	Steps          []Step    `yaml:"steps"`
	OnFailure      OnFailure `yaml:"on_failure"`      // default for steps that omit theirs
	AllowReentrant bool      `yaml:"allow_reentrant"` // D-11 opt-in for the rare re-entrant case
}

// Provenance identifies the stage + turn that triggered this hook (HOOK-04). It
// is carried through every HookProgress event + the in-flight dedup key.
type Provenance struct {
	HookName     string
	TriggerStage string
	SourceTurnID string
}

// configFile is the on-disk YAML envelope (a `hooks:` list).
type configFile struct {
	Hooks []Hook `yaml:"hooks"`
}

// embeddedSeeded is the zero-config floor (HOOK-02), embedded into the binary.
//
//go:embed seeded.yaml
var embeddedSeeded []byte

// Load decodes the embedded default, then each path in order (layered: later
// paths overlay earlier ones — operator over default — D-06 mirrors
// internal/scheduler's scheduling.yaml convention, using gopkg.in/yaml.v3
// directly so dotted keys are preserved). The merged config is re-decoded into
// typed Hooks + validated. A *ConfigError (collect-all) is returned if any
// violation is found; the config is NOT executed.
func Load(paths ...string) ([]Hook, error) {
	merged := make(map[string]any)
	err := yaml.Unmarshal(embeddedSeeded, &merged)
	if err != nil {
		return nil, fmt.Errorf("hookdag: decode embedded seeded default: %w", err)
	}

	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("hookdag: read config %q: %w", p, err)
		}

		overlay := make(map[string]any)
		err = yaml.Unmarshal(raw, &overlay)
		if err != nil {
			return nil, fmt.Errorf("hookdag: decode config %q: %w", p, err)
		}

		deepMerge(merged, overlay)
	}

	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("hookdag: re-encode merged config: %w", err)
	}

	var cfg configFile
	err = yaml.Unmarshal(out, &cfg)
	if err != nil {
		return nil, fmt.Errorf("hookdag: decode merged config: %w", err)
	}

	err = Validate(cfg.Hooks)
	if err != nil {
		return nil, err
	}

	return cfg.Hooks, nil
}

// deepMerge recursively merges src into dst (mirrors scheduler.deepMerge). For
// shared keys whose values are both maps, recurse; otherwise src's value wins
// (overlay semantics — an operator path replaces a default list entry).
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dv, ok := dst[k]; ok {
				if dm, ok := dv.(map[string]any); ok {
					deepMerge(dm, sm)

					continue
				}
			}
		}

		dst[k] = sv
	}
}

// ConfigError lists every validation violation found in one pass (collect-all).
type ConfigError struct {
	Violations []string
}

// Error joins the violations with "; " — investigate-and-fix-ready.
func (e *ConfigError) Error() string {
	if len(e.Violations) == 0 {
		return "hookdag config invalid"
	}

	cp := append([]string(nil), e.Violations...)
	sort.Strings(cp)

	return strings.Join(cp, "; ")
}

// Validate cross-references the hook config + returns a *ConfigError listing
// every violation (collect-all). It checks: every Step.Kind is a known kind;
// every OnFailure (hook + step) is halt/continue/ask; every run-command has a
// non-empty Command; every send-prompt has a non-empty Prompt; every wait has a
// parseable Duration; hook Name is unique. The default OnFailure (when a step
// omits its policy) inherits the hook's at execution time, NOT here.
func Validate(hooks []Hook) error {
	var v []string

	seenNames := map[string]struct{}{}
	validKinds := map[StepKind]bool{StepRunCommand: true, StepSendPrompt: true, StepFreshContext: true, StepWait: true}

	for i := range hooks {
		h := &hooks[i]
		if h.Name == "" {
			v = append(v, fmt.Sprintf("hook[%d]: empty name", i))
		} else if _, dup := seenNames[h.Name]; dup {
			v = append(v, fmt.Sprintf("hook %q: duplicate name", h.Name))
		}

		seenNames[h.Name] = struct{}{}
		if h.OnFailure != "" && !validOnFailure(h.OnFailure) {
			v = append(v, fmt.Sprintf("hook %q: unknown on_failure %q (want halt/continue/ask)", h.Name, h.OnFailure))
		}

		if h.Trigger == "" {
			v = append(v, fmt.Sprintf("hook %q: empty trigger", h.Name))
		}

		for j := range h.Steps {
			s := &h.Steps[j]
			if !validKinds[s.Kind] {
				v = append(v, fmt.Sprintf(
					"hook %q step %q: unknown kind %q (want run-command/send-prompt/fresh-context/wait)",
					h.Name, s.Name, s.Kind))
			}

			if s.OnFailure != "" && !validOnFailure(s.OnFailure) {
				v = append(v, fmt.Sprintf(
					"hook %q step %q: unknown on_failure %q (want halt/continue/ask)",
					h.Name, s.Name, s.OnFailure))
			}

			if s.Name == "" {
				v = append(v, fmt.Sprintf("hook %q step[%d]: empty name", h.Name, j))
			}

			switch s.Kind {
			case StepRunCommand:
				if s.Command == "" {
					v = append(v, fmt.Sprintf(
						"hook %q step %q: run-command requires a non-empty command",
						h.Name, s.Name))
				}
			case StepSendPrompt:
				if s.Prompt == "" {
					v = append(v, fmt.Sprintf(
						"hook %q step %q: send-prompt requires a non-empty prompt",
						h.Name, s.Name))
				}
			case StepWait:
				if s.Duration == "" {
					v = append(v, fmt.Sprintf("hook %q step %q: wait requires a duration", h.Name, s.Name))
				} else {
					_, derr := time.ParseDuration(s.Duration)
					if derr != nil {
						v = append(v, fmt.Sprintf(
							"hook %q step %q: unparseable duration %q: %v",
							h.Name, s.Name, s.Duration, derr))
					}
				}
			case StepFreshContext:
				// no user-supplied fields to validate.
			}
		}
	}

	if len(v) > 0 {
		return &ConfigError{Violations: v}
	}

	return nil
}

func validOnFailure(o OnFailure) bool {
	return o == OnFailureHalt || o == OnFailureContinue || o == OnFailureAsk
}
