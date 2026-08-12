package scheduler

import (
	_ "embed"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// embeddedDefault is the zero-config floor (DIST-03), embedded into the binary.
// Load decodes it first; every caller-provided path overlays it (project over
// global over default). RESEARCH §1.1.
//
//go:embed defaults/scheduling.yaml
var embeddedDefault []byte

// Load decodes the embedded default, then each path in order (layered: later
// paths overlay earlier ones — project over global over default), applies
// documented defaults to zero-valued fields, and validates the graph (D-10). A
// *ConfigError is returned (collect-all) if any inconsistency is found; the
// config is NOT served. RESEARCH §1.1, §7.3.
//
// Implementation note (deviation from RESEARCH §1.1's "viper-loaded" wording,
// recorded investigate-and-fix-ready): the loader uses gopkg.in/yaml.v3
// directly with a manual deep-merge rather than spf13/viper. Viper's internal
// config flatten splits map keys on "." (its key-path delimiter), which
// mangles model slugs that contain dots ("glm-5.2" → "glm-5") — every model
// slug in the zcode/GLM ecosystem is dotted, so this is load-bearing. yaml.v3
// preserves dotted map keys verbatim. The OPERATOR-FACING contract from D-01
// (declarative YAML, layered global → per-project, embedded zero-config floor)
// is preserved exactly; only the parsing library changes.
func Load(paths ...string) (*Config, error) {
	merged := make(map[string]any)
	if err := yaml.Unmarshal(embeddedDefault, &merged); err != nil {
		return nil, fmt.Errorf("decode embedded scheduling default: %w", err)
	}

	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read scheduling config %q: %w", p, err)
		}

		overlay := make(map[string]any)
		if err := yaml.Unmarshal(raw, &overlay); err != nil {
			return nil, fmt.Errorf("decode scheduling config %q: %w", p, err)
		}

		deepMerge(merged, overlay)
	}

	// Re-marshal the merged map and decode into the typed Config so yaml tags +
	// the UnmarshalYAML methods (Duration parsing) drive the conversion.
	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("re-encode merged scheduling config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		return nil, fmt.Errorf("decode scheduling config: %w", err)
	}

	applyDefaults(&cfg)

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// deepMerge recursively merges src into dst. For shared keys whose values are
// both maps, it recurses (deep merge); otherwise src's value wins (overlay
// semantics). Mutates dst in place.
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

// applyDefaults fills zero-valued config fields with the documented D-07/D-08
// defaults (RESEARCH §1.3). Operators override any of these via scheduling.yaml.
func applyDefaults(cfg *Config) {
	if cfg.Timezone == "" {
		cfg.Timezone = "UTC"
	}

	cb := &cfg.CircuitBreaker
	if cb.ConsecutiveFailures == 0 {
		cb.ConsecutiveFailures = defaultBreaker.ConsecutiveFailures
	}

	if cb.ErrorRateWindow == 0 {
		cb.ErrorRateWindow = defaultBreaker.ErrorRateWindow
	}

	if cb.ErrorRateThreshold == 0 {
		cb.ErrorRateThreshold = defaultBreaker.ErrorRateThreshold
	}

	if cb.Cooldown == 0 {
		cb.Cooldown = defaultBreaker.Cooldown
	}

	if cb.HalfOpenProbes == 0 {
		cb.HalfOpenProbes = defaultBreaker.HalfOpenProbes
	}

	if cfg.CostCeiling.Window == 0 {
		cfg.CostCeiling.Window = defaultCost.Window
	}
}

// UnmarshalYAML parses the circuit_breaker block, tolerating the human-friendly
// "60s" duration spelling (yaml.v3 does not natively decode strings into
// time.Duration).
func (c *CircuitBreakerConfig) UnmarshalYAML(value *yaml.Node) error {
	type raw struct {
		ConsecutiveFailures int     `yaml:"consecutive_failures"`
		ErrorRateWindow     int     `yaml:"error_rate_window"`
		ErrorRateThreshold  float64 `yaml:"error_rate_threshold"`
		Cooldown            string  `yaml:"cooldown"`
		HalfOpenProbes      int     `yaml:"half_open_probes"`
	}

	var r raw

	err := value.Decode(&r)
	if err != nil {
		return err
	}

	c.ConsecutiveFailures = r.ConsecutiveFailures
	c.ErrorRateWindow = r.ErrorRateWindow
	c.ErrorRateThreshold = r.ErrorRateThreshold

	c.HalfOpenProbes = r.HalfOpenProbes

	if r.Cooldown != "" {
		d, err := time.ParseDuration(r.Cooldown)
		if err != nil {
			return fmt.Errorf("circuit_breaker.cooldown %q: %w", r.Cooldown, err)
		}

		c.Cooldown = d
	}

	return nil
}

// UnmarshalYAML parses the cost_ceiling block, tolerating the human-friendly
// "24h" duration spelling.
func (c *CostCeilingConfig) UnmarshalYAML(value *yaml.Node) error {
	type raw struct {
		AmountUSD float64 `yaml:"amount_usd"`
		Window    string  `yaml:"window"`
		DegradeTo string  `yaml:"degrade_to"`
	}

	var r raw

	err := value.Decode(&r)
	if err != nil {
		return err
	}

	c.AmountUSD = r.AmountUSD

	c.DegradeTo = r.DegradeTo

	if r.Window != "" {
		d, err := time.ParseDuration(r.Window)
		if err != nil {
			return fmt.Errorf("cost_ceiling.window %q: %w", r.Window, err)
		}

		c.Window = d
	}

	return nil
}

// ConfigError lists every validation violation found in one pass (collect-all,
// RESEARCH §7.3 pitfall 10). Violations are sorted for stable output. The
// operator sees the whole picture, not fail-fast-on-first.
type ConfigError struct {
	Violations []string
}

// Error joins all violations with "; " — investigate-and-fix-ready (C5).
func (e *ConfigError) Error() string {
	if len(e.Violations) == 0 {
		return "scheduling config invalid"
	}

	cp := append([]string(nil), e.Violations...)
	sort.Strings(cp)

	return strings.Join(cp, "; ")
}

// Validate cross-references the config graph and returns a *ConfigError listing
// every violation (collect-all, RESEARCH §7.3). It checks:
//   - every tier/window/project model + fallback slug exists in models;
//   - capability consistency (D-10): for tool_calling/streaming/extended_thinking,
//     each binding's primary and every fallback must be >= the primary (a primary
//     that supports a capability cannot fall back to one that lacks it);
//   - every model.provider exists in providers;
//   - every provider.shape is anthropic|openai;
//   - cost_ceiling.degrade_to references a declared tier.
//
// Overlapping time-windows are a soft WARN (RESEARCH §7.3) and are NOT rejected
// here — first-match-in-config-order is deterministic.
func Validate(cfg *Config) error {
	var v []string

	// checkBinding appends violations for dangling slugs + capability mismatch
	// for one TierBinding (used by tiers, windows, projects).
	checkBinding := func(label string, b TierBinding) {
		primary, pok := cfg.Models[b.Model]
		if !pok {
			v = append(v, fmt.Sprintf("%s: model %q is not declared in models", label, b.Model))
		}

		for _, fb := range b.Fallback {
			fbModel, ok := cfg.Models[fb]
			if !ok {
				v = append(v, fmt.Sprintf("%s: fallback %q is not declared in models", label, fb))

				continue
			}

			if !pok {
				continue
			}

			pcap, fcap := primary.Capabilities, fbModel.Capabilities
			if pcap.ToolCalling && !fcap.ToolCalling {
				v = append(v, fmt.Sprintf(
					"%s: primary %s supports tool_calling but fallback %s does not — incompatible chain",
					label, b.Model, fb))
			}

			if pcap.Streaming && !fcap.Streaming {
				v = append(v, fmt.Sprintf(
					"%s: primary %s supports streaming but fallback %s does not — incompatible chain",
					label, b.Model, fb))
			}

			if pcap.ExtendedThinking && !fcap.ExtendedThinking {
				v = append(v, fmt.Sprintf(
					"%s: primary %s supports extended_thinking but fallback %s does not — incompatible chain",
					label, b.Model, fb))
			}
		}
	}

	// Iterate tier tables in a deterministic order so violation messages are
	// stable across runs (map iteration is randomized in Go).
	for _, tier := range sortedKeys(cfg.Tiers) {
		checkBinding(fmt.Sprintf("tier %q", tier), cfg.Tiers[tier])
	}

	for _, name := range sortedWindowNames(cfg.TimeWindows) {
		w := windowByName(cfg.TimeWindows, name)
		for _, tier := range sortedKeys(w.Tiers) {
			checkBinding(fmt.Sprintf("time_window %q tier %q", name, tier), w.Tiers[tier])
		}
	}

	for _, proj := range sortedKeys(cfg.Projects) {
		po := cfg.Projects[proj]
		for _, tier := range sortedKeys(po.Tiers) {
			checkBinding(fmt.Sprintf("project %q tier %q", proj, tier), po.Tiers[tier])
		}
	}

	// model.provider exists + provider.shape valid.
	for _, slug := range sortedKeys(cfg.Models) {
		m := cfg.Models[slug]
		if _, ok := cfg.Providers[m.Provider]; !ok {
			v = append(v, fmt.Sprintf("model %q: provider %q is not declared in providers", slug, m.Provider))
		}
	}

	for _, slug := range sortedKeys(cfg.Providers) {
		p := cfg.Providers[slug]
		if p.Shape != "anthropic" && p.Shape != "openai" {
			v = append(v, fmt.Sprintf("provider %q: unknown shape %q (want \"anthropic\" or \"openai\")", slug, p.Shape))
		}
	}

	// cost_ceiling.degrade_to references a declared tier (only when set).
	if cfg.CostCeiling.DegradeTo != "" {
		if _, ok := cfg.Tiers[cfg.CostCeiling.DegradeTo]; !ok {
			v = append(v, fmt.Sprintf("cost_ceiling.degrade_to: tier %q is not declared in tiers", cfg.CostCeiling.DegradeTo))
		}
	}

	if len(v) > 0 {
		return &ConfigError{Violations: v}
	}

	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func sortedWindowNames(windows []TimeWindow) []string {
	names := make([]string, 0, len(windows))
	for _, w := range windows {
		names = append(names, w.Name)
	}

	sort.Strings(names)

	return names
}

func windowByName(windows []TimeWindow, name string) TimeWindow {
	for _, w := range windows {
		if w.Name == name {
			return w
		}
	}

	return TimeWindow{}
}
