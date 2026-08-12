package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, 0.50, cb.ErrorRateThreshold)
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
	require.Equal(t, []string{modelMinimaxM3}, cfg.Tiers[tierHeavy].Fallback, "base heavy.fallback should persist (deep merge)")
}

// TestLoadEmbeddedDefault asserts Load() with no paths returns the embedded
// zero-config floor (DIST-03) — a valid config that resolves heavy.
func TestLoadEmbeddedDefault(t *testing.T) {
	t.Parallel()

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, modelGLM52, cfg.Tiers[tierHeavy].Model)
	require.Equal(t, providerAnthropic, cfg.Models[modelGLM52].Provider)
}
