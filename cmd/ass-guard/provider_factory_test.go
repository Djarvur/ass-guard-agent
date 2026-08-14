package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/scheduler"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// testSchedulingWarnsConfig declares two providers — zai with a literal api_key
// and nokey with api_key_env pointing at an UNSET var — so WarnUncredentialed
// emits exactly one warning (for nokey) and Load's D-04 credential-field warn
// stays silent. The literal api_key is a FAKE test value.
const testSchedulingWarnsConfig = `providers:
  zai:
    base_url: "https://api.z.ai/api/anthropic"
    shape: anthropic
    api_key: "sk-test-literal"
  nokey:
    base_url: "https://api.z.ai/api/anthropic"
    shape: anthropic
    api_key_env: NOKEY_API_KEY
models:
  glm-5.2:
    provider: zai
    pricing: { input_per_mtoken: 0.60, output_per_mtoken: 2.20 }
    capabilities: { context_window: 200000, max_output_tokens: 32000, tool_calling: true, streaming: true, extended_thinking: true }
tiers:
  heavy:
    model: glm-5.2
    fallback: []
`

// testSchedulingZeroEnvConfig declares two providers; the heavy-tier provider
// (zai) carries a literal api_key and no env dependency (SC1 editor-zero-env),
// while oai declares an unset env var (D-07 warn-not-reject path).
const testSchedulingZeroEnvConfig = `providers:
  zai:
    base_url: "https://api.z.ai/api/anthropic"
    shape: anthropic
    api_key: "sk-test-literal"
  oai:
    base_url: "https://api.openai.com/v1"
    shape: openai
    api_key_env: OPENAI_API_KEY
models:
  glm-5.2:
    provider: zai
    pricing: { input_per_mtoken: 0.60, output_per_mtoken: 2.20 }
    capabilities: { context_window: 200000, max_output_tokens: 32000, tool_calling: true, streaming: true, extended_thinking: true }
tiers:
  heavy:
    model: glm-5.2
    fallback: []
`

// writeTestScheduling writes content to <workDir>/.ass-guard/scheduling.yaml
// and returns the written path.
func writeTestScheduling(t *testing.T, workDir, content string) string {
	t.Helper()

	dir := filepath.Join(workDir, ".ass-guard")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	path := filepath.Join(dir, "scheduling.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// TestLoadSchedulingFactory_WarnsUncredentialed proves D-07 at the wiring seam:
// a 2-provider config where one provider carries a literal api_key and the
// other has no resolvable credential produces exactly one stderr warning naming
// the uncredentialed provider — and the factory is still built (warn, never
// refuse to start).
func TestLoadSchedulingFactory_WarnsUncredentialed(t *testing.T) {
	workDir := t.TempDir()
	writeTestScheduling(t, workDir, testSchedulingWarnsConfig)

	var stderr bytes.Buffer

	cfg, factory, err := loadSchedulingFactory(workDir, "", &stderr)
	require.NoError(t, err, "an uncredentialed provider must not refuse the factory (D-07)")
	require.NotNil(t, cfg)
	require.NotNil(t, factory)
	require.Contains(t, stderr.String(), "nokey", "the D-07 warning names the uncredentialed provider")
	require.Contains(t, stderr.String(), "NOKEY_API_KEY", "the D-07 warning names the env var to set")
	require.NotContains(t, stderr.String(), "sk-test-literal", "a warning must never print a key (T-07-06)")
}

// TestLoadSchedulingFactory_ZeroConfigEnv proves SC4 backward-compat at the
// wiring seam: with no overlay file and $ZAI_API_KEY set, the embedded default
// resolves the anthropic provider's key from the env (Source=env) and Build
// returns a real adapter — today's exact behavior.
func TestLoadSchedulingFactory_ZeroConfigEnv(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "env-secret")

	var stderr bytes.Buffer

	cfg, factory, err := loadSchedulingFactory(t.TempDir(), "", &stderr)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, factory)

	prov, ok := cfg.Providers["anthropic"]
	require.True(t, ok, "the embedded default declares the anthropic provider")

	cred := scheduler.ResolveCredential(prov, "anthropic", "")
	require.Equal(t, "env", cred.Source, "the embedded api_key_env resolves from $ZAI_API_KEY")
	require.Equal(t, "env-secret", cred.Key)

	p, err := factory.Build("anthropic", shaper.New())
	require.NoError(t, err)
	require.IsType(t, &provider.AnthropicProvider{}, p, "a credentialed build returns the real adapter")
	require.Empty(t, stderr.String(), "a fully-credentialed config warns nothing")
}

// TestStartupWarn_ConfigPermLoose proves the SC3 credential-on-disk hygiene
// warning: a group/world-readable scheduling.yaml warns at startup with the
// 0600 recommendation; a 0600-tight file stays silent. Never refuses to start.
func TestStartupWarn_ConfigPermLoose(t *testing.T) {
	workDir := t.TempDir()
	path := writeTestScheduling(t, workDir, testSchedulingZeroEnvConfig)

	require.NoError(t, os.Chmod(path, 0o644))

	var loose bytes.Buffer

	warnLooseConfigPerm(path, &loose)
	require.Contains(t, loose.String(), "0600", "a 0644 config warns with the 0600 recommendation")

	require.NoError(t, os.Chmod(path, 0o600))

	var tight bytes.Buffer

	warnLooseConfigPerm(path, &tight)
	require.Empty(t, tight.String(), "a 0600-tight config stays silent")
}

// TestBackwardCompat_ZAIEnvOnly is the SC4 proof: embedded default only,
// $ZAI_API_KEY set, no flag, no literal — the resolved anthropic credential
// Source is "env" and the key equals $ZAI_API_KEY (today's exact behavior).
func TestBackwardCompat_ZAIEnvOnly(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "env-secret")

	var stderr bytes.Buffer

	cfg, factory, err := loadSchedulingFactory(t.TempDir(), "", &stderr)
	require.NoError(t, err)

	prov, ok := cfg.Providers["anthropic"]
	require.True(t, ok, "the embedded default declares the anthropic provider")

	cred := scheduler.ResolveCredential(prov, "anthropic", "")
	require.Equal(t, "env", cred.Source, "zero-config resolves from the env (D-06)")
	require.Equal(t, "env-secret", cred.Key, "the key equals $ZAI_API_KEY")

	p, err := factory.Build("anthropic", shaper.New())
	require.NoError(t, err)
	require.IsType(t, &provider.AnthropicProvider{}, p, "a real adapter, not the noCredentialProvider wrapper")
}

// TestEditorZeroEnv_LiteralInConfig is the SC1 proof: a 2-provider config where
// the active (heavy-tier) provider carries a literal api_key authenticates with
// ZERO environment — the Zed-spawned `ass-guard acp serve` case (D-01).
func TestEditorZeroEnv_LiteralInConfig(t *testing.T) {
	// Zero environment: every provider-key var is empty.
	t.Setenv("ZAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("NOKEY_API_KEY", "")

	workDir := t.TempDir()
	writeTestScheduling(t, workDir, testSchedulingZeroEnvConfig)

	var stderr bytes.Buffer

	cfg, factory, err := loadSchedulingFactory(workDir, "", &stderr)
	require.NoError(t, err)
	require.NotNil(t, factory)

	prov, ok := cfg.Providers["zai"]
	require.True(t, ok, "the config declares the active provider")

	cred := scheduler.ResolveCredential(prov, "zai", "")
	require.Equal(t, "config", cred.Source, "the file credential is the floor (D-05)")
	require.Equal(t, "sk-test-literal", cred.Key)

	baseURL, key, ok := factory.Endpoint("zai")
	require.True(t, ok)
	require.Equal(t, "https://api.z.ai/api/anthropic", baseURL)
	require.Equal(t, "sk-test-literal", key)

	p, err := factory.Build("zai", shaper.New())
	require.NoError(t, err)
	require.IsType(t, &provider.AnthropicProvider{}, p, "a file credential authenticates with zero env (SC1)")
}
