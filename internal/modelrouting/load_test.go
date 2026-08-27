package modelrouting //nolint:testpackage // internal package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestLoadValid loads the canonical valid fixture and asserts the populated
// shape: models carry capabilities, tiers carry primary + fallback lists.
func TestLoadValid(t *testing.T) {
	t.Parallel()

	cfg, err := Load("testdata/valid.yaml")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	require.True(t, cfg.Models[modelGLM52].Capabilities.ToolCalling, "glm-5.2 should have tool_calling")
	require.Equal(t, modelGLM52, cfg.Tiers[tierHeavy].Model)
	require.Equal(t, []string{modelMinimaxM3, modelGLM46}, cfg.Tiers[tierHeavy].Fallback)
}

// TestLoadInvalidCapMismatch asserts D-10 rejects a config where a primary has a
// capability (tool_calling) a fallback lacks. The error names BOTH models AND
// the capability — investigate-and-fix-ready.
func TestLoadInvalidCapMismatch(t *testing.T) {
	t.Parallel()

	_, err := Load("testdata/invalid_cap_mismatch.yaml")
	require.Error(t, err)

	var cerr *ConfigError

	require.ErrorAs(t, err, &cerr, "should be a *ConfigError")
	msg := err.Error()
	require.Contains(t, msg, modelGLM52, "error must name the primary model")
	require.Contains(t, msg, "haiku-cheap", "error must name the incompatible fallback")
	require.Contains(t, msg, "tool_calling", "error must name the unmet capability")
}

// TestLoadInvalidDanglingSlug asserts a tier whose model references an undeclared
// slug is rejected with the slug named.
func TestLoadInvalidDanglingSlug(t *testing.T) {
	t.Parallel()

	_, err := Load("testdata/invalid_dangling_slug.yaml")
	require.Error(t, err)

	var cerr *ConfigError

	require.ErrorAs(t, err, &cerr)
	require.Contains(t, err.Error(), "nonexistent-model", "error must name the dangling slug")
}

// TestLoadCollectAll asserts Validate reports EVERY violation in one pass
// (pitfall 10), not fail-fast-on-first. The fixture has two distinct violations
// (a dangling slug + a capability mismatch); both must appear in the message.
func TestLoadCollectAll(t *testing.T) {
	t.Parallel()

	_, err := Load("testdata/invalid_multiple.yaml")
	require.Error(t, err)

	var cerr *ConfigError

	require.ErrorAs(t, err, &cerr)
	msg := err.Error()
	require.Contains(t, msg, "ghost-fallback", "must name violation 1 (dangling slug)")
	require.Contains(t, msg, "haiku-cheap", "must name violation 2 (cap mismatch)")
	// At least two distinct violations joined.
	require.GreaterOrEqual(t, len(strings.Split(msg, "; ")), 2)
}

// TestLoadDefaults asserts the documented D-07 breaker defaults + the 24h cost
// window are applied when the config omits those blocks (zero-valued fields).
func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load("testdata/minimal.yaml")
	require.NoError(t, err)

	cb := cfg.CircuitBreaker
	require.Equal(t, 5, cb.ConsecutiveFailures)
	require.Equal(t, 20, cb.ErrorRateWindow)
	require.InDelta(t, 0.50, cb.ErrorRateThreshold, 1e-9)
	require.Equal(t, 60*time.Second, cb.Cooldown)
	require.Equal(t, 1, cb.HalfOpenProbes)
	require.Equal(t, 24*time.Hour, cfg.CostCeiling.Window)
}

// TestLoadLayering asserts viper's deep-merge layering: an overlay path wins for
// keys present in both, base values persist for base-only keys. The embedded
// default sits below both as the floor (RESEARCH §1.1, pitfall 1).
func TestLoadLayering(t *testing.T) {
	t.Parallel()

	cfg, err := Load("testdata/base_layering.yaml", "testdata/overlay_layering.yaml")
	require.NoError(t, err)
	// Overlay wins for shared keys.
	require.Equal(t, "Asia/Tokyo", cfg.Timezone, "overlay timezone should win")
	require.Equal(t, modelMinimaxM3, cfg.Tiers[tierHeavy].Model, "overlay heavy.model should win")
	// Base persists for base-only keys.
	require.Equal(t, modelGLM46, cfg.Tiers["good"].Model, "base good.model should persist")
	// Deep merge: base's heavy.fallback survives the overlay's heavy.model change.
	require.Equal(t, []string{modelMinimaxM3}, cfg.Tiers[tierHeavy].Fallback,
		"base heavy.fallback should persist (deep merge)")
}

// TestLoadEmbeddedDefault asserts Load() with no paths returns the embedded
// zero-config floor (DIST-03) — a valid config that resolves heavy.
func TestLoadEmbeddedDefault(t *testing.T) {
	t.Parallel()

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, modelGLM53, cfg.Tiers[tierHeavy].Model,
		"heavy primary must be the pinned capture's wire slug")
	require.Equal(t, []string{modelGLM52}, cfg.Tiers[tierHeavy].Fallback,
		"glm-5.2 stays as the declared fallback")
	require.Equal(t, providerAnthropic, cfg.Models[modelGLM53].Provider)
	require.Equal(t, 128000, cfg.Models[modelGLM53].Capabilities.MaxOutputTokens,
		"max_output_tokens mirrors the pinned capture's request.body.max_tokens")
	require.True(t, cfg.Models[modelGLM53].Capabilities.ToolCalling)
}

// TestSessionTier asserts the additive session_tier key (ACP-08 groundwork,
// 16-04): absent → tierHeavy default at the same defaults-application site as
// the other absent-key defaults; an explicit value loads through; the key
// survives a marshal→Load round-trip. No resolver behavior here — tier
// consumption lands in 16-05's apply seam.
func TestSessionTier(t *testing.T) {
	t.Parallel()

	t.Run("absent key defaults to heavy", func(t *testing.T) {
		t.Parallel()

		cfg, err := Load("testdata/minimal.yaml")
		require.NoError(t, err)
		require.Equal(t, tierHeavy, cfg.SessionTier)
	})

	t.Run("explicit value loads through", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "session_tier.yaml")
		require.NoError(t, os.WriteFile(path, []byte("session_tier: light\n"), 0o600))

		cfg, err := Load(path)
		require.NoError(t, err)
		require.Equal(t, tierLight, cfg.SessionTier)
	})

	t.Run("marshal round-trip preserves the key", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "session_tier.yaml")
		require.NoError(t, os.WriteFile(path, []byte("session_tier: light\n"), 0o600))

		cfg, err := Load(path)
		require.NoError(t, err)

		out, err := yaml.Marshal(cfg)
		require.NoError(t, err)

		round := filepath.Join(t.TempDir(), "round.yaml")
		require.NoError(t, os.WriteFile(round, out, 0o600))

		reloaded, err := Load(round)
		require.NoError(t, err)
		require.Equal(t, tierLight, reloaded.SessionTier,
			"session_tier must survive marshal→Load byte-for-byte in value")
	})
}

// TestValidateWarns_NoCredentialField asserts D-04's warn-not-reject: a
// provider declared with neither api_key nor api_key_env yields a warning
// naming the provider, and validation still succeeds (no *ConfigError).
func TestValidateWarns_NoCredentialField(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Providers: map[string]ProviderConfig{
			testNoKeySlug: {Shape: providerAnthropic},
		},
	}

	warnings, err := ValidateWithWarnings(cfg)
	require.NoError(t, err, "missing credential fields must warn, not reject")
	require.NotEmpty(t, warnings)
	require.Contains(t, strings.Join(warnings, " "), testNoKeySlug)
	require.Contains(t, strings.Join(warnings, " "), "api_key")
}

// TestValidate_NoWarnWhenAPIKeyEnvDeclared asserts a provider that declares
// api_key_env (even with the env var currently unset) produces NO load-time
// warning — load-time cannot know runtime env state; the startup warn (Plan
// 07-02) handles that.
func TestValidate_NoWarnWhenAPIKeyEnvDeclared(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Providers: map[string]ProviderConfig{
			testZaiSlug: {Shape: providerAnthropic, APIKeyEnv: testZAIEnv},
		},
	}

	warnings, err := ValidateWithWarnings(cfg)
	require.NoError(t, err)
	require.Empty(t, warnings)
}

// TestValidate_StillRejectsBadShape guards Phase-3 D-10: an unknown provider
// shape remains a hard violation even after the credential warn was added.
func TestValidate_StillRejectsBadShape(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"weird": {Shape: "weird", APIKey: "sk-lit"},
		},
	}

	_, err := ValidateWithWarnings(cfg)
	require.Error(t, err)

	var cerr *ConfigError

	require.ErrorAs(t, err, &cerr, "unknown shape must still be a *ConfigError")
	require.Contains(t, err.Error(), "weird")
}
