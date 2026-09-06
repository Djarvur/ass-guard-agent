package modelrouting

import (
	"time"
)

// Config is the fully-merged scheduling configuration (RESEARCH §1.3). Viper
// loads it layered (embedded default → global → per-project; RESEARCH §1.1) and
// Validate cross-references the graph before any request is served (D-10). The
// file-merge layering is distinct from the D-02 resolution precedence
// (time-window → project → global) the resolver applies over the MERGED config
// (pitfall 1).
type Config struct {
	Timezone string `yaml:"timezone"`

	// SessionTier names the tier the interactive session runs on (ACP-08's
	// editor-facing option; 16-05 advertises and applies it). Additive key
	// (D-08 groundwork): either config layer may set it; an absent key gets
	// the tierHeavy default at load time. This plan only parses, defaults,
	// and round-trips it — tier consumption lands in 16-05's apply seam, so
	// Validate does NOT cross-reference it (an unknown tier name is 16-05's
	// typed-reject concern at the wire, per D-09).
	SessionTier string `yaml:"session_tier"`

	// Compaction is PAR-01's compaction policy (19-05/D-03): the same
	// layered-config identity model session_tier occupies. Absent keys load
	// the embedded floor's 80/true defaults; out-of-range thresholds are
	// deliberately NOT remapped here — the set path typed-rejects them at the
	// wire (1..100, D-09) and the session-side comparison clamps defensively
	// (the hand-edited-file net). The context limit itself is NEVER
	// configurable: it stays sourced from the capability table
	// (Capabilities.ContextWindow) — an override key would fork the
	// measurement baseline.
	Compaction     CompactionConfig           `yaml:"compaction"`
	Providers      map[string]ProviderConfig  `yaml:"providers"`
	Models         map[string]ModelConfig     `yaml:"models"`
	Tiers          map[string]TierBinding     `yaml:"tiers"`
	TimeWindows    []TimeWindow               `yaml:"time_windows"`
	Projects       map[string]ProjectOverride `yaml:"projects"`
	CircuitBreaker CircuitBreakerConfig       `yaml:"circuit_breaker"`
	CostCeiling    CostCeilingConfig          `yaml:"cost_ceiling"`
}

// CompactionConfig is the compaction policy block (19-05/D-03): ThresholdPct
// is the share of the resolved context window at which the pre-request check
// fires (inclusive); Enabled is the operator switch (false makes the
// pre-request check skip entirely — 19-04's disabled path). Parse/default/
// round-trip only: Validate does NOT cross-reference these values (the
// 16-04 session_tier discipline — value whitelisting lives at the wire where
// the menu ids are).
type CompactionConfig struct {
	ThresholdPct int  `yaml:"threshold_pct"`
	Enabled      bool `yaml:"enabled"`
}

// ProviderConfig declares one provider endpoint. Shape selects the Phase-1
// adapter ("anthropic" or "openai"). Credentials (D-01/D-03): APIKey is a
// literal or a `${NAME}` expansion; APIKeyEnv names the env var (e.g.
// ZAI_API_KEY) as a self-documenting alias for the `${...}` form. Either may be
// absent — a provider with neither resolves to no credential (D-07 lazy error).
type ProviderConfig struct {
	BaseURL   string `yaml:"base_url"`
	Shape     string `yaml:"shape"`
	APIKey    string `yaml:"api_key"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// ModelConfig is the per-(provider, model) declaration: the capability profile
// (D-09) + pricing (D-08). The map key in Config.Models is the model slug
// referenced by tiers/windows/projects/fallbacks.
type ModelConfig struct {
	Provider     string            `yaml:"provider"`
	Pricing      Pricing           `yaml:"pricing"`
	Capabilities CapabilityProfile `yaml:"capabilities"`
}

// Pricing is the per-model USD-per-1M-tokens rate (D-08). The cost tracker
// estimates per-request cost as (inTokens×Input + outTokens×Output)/1e6.
type Pricing struct {
	InputPerMToken  float64 `json:"input_per_mtoken"  yaml:"input_per_mtoken"`
	OutputPerMToken float64 `json:"output_per_mtoken" yaml:"output_per_mtoken"`
}

// CapabilityProfile is the structured D-09 declaration. This is what makes the
// tier abstraction honest: "heavy" on one model (200K context, tools, streaming)
// ≠ "heavy" on another. The resolver consults it when resolving fallbacks; the
// load-time validator (D-10) rejects configs where a primary has a capability a
// fallback lacks.
type CapabilityProfile struct {
	ContextWindow    int      `json:"context_window"    yaml:"context_window"`
	MaxOutputTokens  int      `json:"max_output_tokens" yaml:"max_output_tokens"`
	ToolCalling      bool     `json:"tool_calling"      yaml:"tool_calling"`
	Streaming        bool     `json:"streaming"         yaml:"streaming"`
	ExtendedThinking bool     `json:"extended_thinking" yaml:"extended_thinking"`
	Limitations      []string `json:"limitations"       yaml:"limitations"`
}

// TierBinding is the primary (model + ordered fallback list) for one tier. The
// explicit ordered fallback array is D-05 — operator-authored, auditable, no
// silent capability mismatch.
type TierBinding struct {
	Model    string   `yaml:"model"`
	Fallback []string `yaml:"fallback"`
}

// TimeWindow is a structural peak/off-peak substitution (SCHED-02). When active
// at request time, the window's tier bindings win outright (D-02 — the
// operational reality of peak hours is the floor projects build on).
type TimeWindow struct {
	Name     string                 `yaml:"name"`
	Zone     string                 `yaml:"zone"`
	Schedule Schedule               `yaml:"schedule"`
	Tiers    map[string]TierBinding `yaml:"tiers"`
}

// Schedule is the window's time-of-day + days-of-week pattern (RESEARCH §2.3).
// From/To are "HH:MM" in the window's zone. If To <= From the window wraps past
// midnight (e.g. 22:00→06:00). Empty Days means every day.
type Schedule struct {
	From string   `yaml:"from"`
	To   string   `yaml:"to"`
	Days []string `yaml:"days"`
}

// ProjectOverride is the per-project tier-table narrowing (SCHED-03). The
// project override fills the gaps the active time-window leaves — it cannot
// displace a window's structural pick (D-02).
type ProjectOverride struct {
	Tiers map[string]TierBinding `yaml:"tiers"`
}

// CircuitBreakerConfig holds the D-07 thresholds (both consecutive-failure AND
// error-rate trip mechanisms). Zero-valued fields are filled from defaultBreaker
// at load time.
type CircuitBreakerConfig struct {
	ConsecutiveFailures int           `yaml:"consecutive_failures"`
	ErrorRateWindow     int           `yaml:"error_rate_window"`
	ErrorRateThreshold  float64       `yaml:"error_rate_threshold"`
	Cooldown            time.Duration `yaml:"cooldown"`
	HalfOpenProbes      int           `yaml:"half_open_probes"`
}

// CostCeilingConfig holds the D-08 dollars-per-window ceiling + the tier to
// degrade to on first breach.
type CostCeilingConfig struct {
	AmountUSD float64       `yaml:"amount_usd"`
	Window    time.Duration `yaml:"window"`
	DegradeTo string        `yaml:"degrade_to"`
}

// Target is the resolved hand-off shape the dispatcher gives to a provider
// adapter: the concrete (provider, model), the base URL + shape (adapter
// selection), the capability profile (D-09), and pricing (D-08 cost tracking).
type Target struct {
	Provider     string
	Model        string
	BaseURL      string
	Shape        string
	Capabilities CapabilityProfile
	Pricing      Pricing
}

// CapabilityReq is what a turn declares it needs (D-09 request-time gate). A
// zero-valued CapabilityReq means "no specific needs" — the resolver returns the
// primary regardless of its capabilities (the gate is opt-in per turn).
type CapabilityReq struct {
	NeedsTools     bool
	NeedsStreaming bool
	NeedsThinking  bool
}
