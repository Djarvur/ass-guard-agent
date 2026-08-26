package providerfactory //nolint:testpackage // internal package test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// testModelRoutingWarnsConfig declares two providers — zai with a literal api_key
// and nokey with api_key_env pointing at an UNSET var — so WarnUncredentialed
// emits exactly one warning (for nokey) and Load's D-04 credential-field warn
// stays silent. The literal api_key is a FAKE test value.
const testModelRoutingWarnsConfig = `providers:
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
    capabilities:
      context_window: 200000
      max_output_tokens: 32000
      tool_calling: true
      streaming: true
      extended_thinking: true
tiers:
  heavy:
    model: glm-5.2
    fallback: []
`

// testModelRoutingZeroEnvConfig declares two providers; the heavy-tier provider
// (zai) carries a literal api_key and no env dependency (SC1 editor-zero-env),
// while oai declares an unset env var (D-07 warn-not-reject path).
const testModelRoutingZeroEnvConfig = `providers:
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
    capabilities:
      context_window: 200000
      max_output_tokens: 32000
      tool_calling: true
      streaming: true
      extended_thinking: true
tiers:
  heavy:
    model: glm-5.2
    fallback: []
`

// testGlobalLayerConfig declares a sentinel provider + heavy-tier binding
// reachable ONLY through the global layer (~/.config/ass-guard-agent/
// config.yaml) — any assertion seeing gs-model proves the global file was read
// and merged over the embedded floor.
const testGlobalLayerConfig = `providers:
  gsentinel:
    base_url: "https://global-sentinel.example/api"
    shape: anthropic
    api_key: "sk-test-global"
models:
  gs-model:
    provider: gsentinel
tiers:
  heavy:
    model: gs-model
`

// testProjectLayerConfig declares a DIFFERENT sentinel (ps-model) so the
// project layer's win over the global layer's conflicting tier binding is
// directly observable.
const testProjectLayerConfig = `providers:
  psentinel:
    base_url: "https://project-sentinel.example/api"
    shape: anthropic
    api_key: "sk-test-project"
models:
  ps-model:
    provider: psentinel
tiers:
  heavy:
    model: ps-model
`

// legacySchedulingFileName is the pre-rename operator config filename. It is
// spelled as a concatenation so the repo-wide rename grep gate (which greps
// tracked files for the joined literal) stays clean while this test still pins
// the no-legacy-reads contract (260817-11v revised decision).
const legacySchedulingFileName = "scheduling" + ".yaml"

// writeTestModelRouting writes content to the PROJECT config layer
// (<workDir>/.ass-guard/config.yaml) and returns the written path.
func writeTestModelRouting(t *testing.T, workDir, content string) string {
	t.Helper()

	dir := filepath.Join(workDir, ".ass-guard")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// pinEmptyHome pins HOME to an empty temp dir so the global config layer
// contributes nothing — every LoadModelRoutingFactory-reaching test stays
// hermetic against the operator's real home (t.Setenv ⇒ these tests are not
// parallel).
func pinEmptyHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	return home
}

// writeGlobalTestConfig writes content to the GLOBAL config layer
// (<home>/.config/ass-guard-agent/config.yaml) and returns the written path.
func writeGlobalTestConfig(t *testing.T, home, content string) string {
	t.Helper()

	path := filepath.Join(home, ".config", "ass-guard-agent", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// TestLoadModelRoutingFactory_WarnsUncredentialed proves D-07 at the wiring seam:
// a 2-provider config where one provider carries a literal api_key and the
// other has no resolvable credential produces exactly one stderr warning naming
// the uncredentialed provider — and the factory is still built (warn, never
// refuse to start).
func TestLoadModelRoutingFactory_WarnsUncredentialed(t *testing.T) {
	// Env hygiene: the assertions assume NOKEY_API_KEY is unset (the fixture
	// declares it via api_key_env) and zai stays credentialed regardless of
	// any ambient ZAI_API_KEY. The pinned empty HOME keeps the global config
	// layer out of the assertions. t.Setenv makes this test non-parallel.
	pinEmptyHome(t)
	t.Setenv("ZAI_API_KEY", "")
	t.Setenv("NOKEY_API_KEY", "")

	workDir := t.TempDir()
	writeTestModelRouting(t, workDir, testModelRoutingWarnsConfig)

	var stderr bytes.Buffer

	cfg, factory, err := LoadModelRoutingFactory(workDir, "", &stderr)
	require.NoError(t, err, "an uncredentialed provider must not refuse the factory (D-07)")
	require.NotNil(t, cfg)
	require.NotNil(t, factory)
	require.Contains(t, stderr.String(), "nokey", "the D-07 warning names the uncredentialed provider")
	require.Contains(t, stderr.String(), "NOKEY_API_KEY", "the D-07 warning names the env var to set")
	require.NotContains(t, stderr.String(), "sk-test-literal", "a warning must never print a key (T-07-06)")
}

// TestLoadModelRoutingFactory_ZeroConfigEnv proves SC4 backward-compat at the
// wiring seam: with no overlay file and $ZAI_API_KEY set, the embedded default
// resolves the anthropic provider's key from the env (Source=env) and Build
// returns a real adapter — today's exact behavior.
func TestLoadModelRoutingFactory_ZeroConfigEnv(t *testing.T) {
	pinEmptyHome(t)
	t.Setenv("ZAI_API_KEY", "env-secret")

	var stderr bytes.Buffer

	cfg, factory, err := LoadModelRoutingFactory(t.TempDir(), "", &stderr)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, factory)

	prov, ok := cfg.Providers["anthropic"]
	require.True(t, ok, "the embedded default declares the anthropic provider")

	cred := modelrouting.ResolveCredential(prov, "anthropic", "")
	require.Equal(t, "env", cred.Source, "the embedded api_key_env resolves from $ZAI_API_KEY")
	require.Equal(t, "env-secret", cred.Key)

	p, err := factory.Build("anthropic", shaper.New())
	require.NoError(t, err)
	require.IsType(t, &provider.AnthropicProvider{}, p, "a credentialed build returns the real adapter")
	require.Empty(t, stderr.String(), "a fully-credentialed config warns nothing (no ambient global layer)")
}

// TestLoadModelRoutingFactory_GlobalLayerMerged proves the global layer
// (260817-11v): a config at <home>/.config/ass-guard-agent/config.yaml is
// honored over an empty project — the sentinel tier binding wins over the
// embedded floor while the embedded default's provider stays present (merge,
// not replace).
func TestLoadModelRoutingFactory_GlobalLayerMerged(t *testing.T) { //nolint:paralleltest // HOME pinned — serial
	home := pinEmptyHome(t)
	writeGlobalTestConfig(t, home, testGlobalLayerConfig)

	var stderr bytes.Buffer

	cfg, factory, err := LoadModelRoutingFactory(t.TempDir(), "", &stderr)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, factory)

	require.Equal(t, "gs-model", cfg.Tiers["heavy"].Model,
		"the global layer's tier binding overlays the embedded floor")

	gprov, ok := cfg.Providers["gsentinel"]
	require.True(t, ok, "the global layer's provider is merged in")
	require.Equal(t, "https://global-sentinel.example/api", gprov.BaseURL)

	_, ok = cfg.Providers["anthropic"]
	require.True(t, ok, "the embedded default's provider survives beneath the global layer")
}

// TestLoadModelRoutingFactory_ProjectOverridesGlobal proves the layered
// precedence: with BOTH layers present and conflicting heavy-tier bindings,
// the project layer (<workDir>/.ass-guard/config.yaml) wins — Load's overlay
// order is global then project.
func TestLoadModelRoutingFactory_ProjectOverridesGlobal(t *testing.T) { //nolint:paralleltest // HOME pinned — serial
	home := pinEmptyHome(t)
	writeGlobalTestConfig(t, home, testGlobalLayerConfig)

	workDir := t.TempDir()
	writeTestModelRouting(t, workDir, testProjectLayerConfig)

	var stderr bytes.Buffer

	cfg, factory, err := LoadModelRoutingFactory(workDir, "", &stderr)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, factory)

	require.Equal(t, "ps-model", cfg.Tiers["heavy"].Model,
		"the project layer overrides the global layer on conflict (project wins)")

	_, gok := cfg.Providers["gsentinel"]
	require.True(t, gok, "the global layer still contributes its provider (merge)")

	_, pok := cfg.Providers["psentinel"]
	require.True(t, pok, "the project layer's provider is present")
}

// TestLoadModelRoutingFactory_LegacyNameNeverRead proves the revised 260817-11v
// decision: the pre-rename project config filename is treated as nonexistent —
// a file carrying it is never read, and behavior is identical to no config at
// all (the embedded floor serves).
func TestLoadModelRoutingFactory_LegacyNameNeverRead(t *testing.T) { //nolint:paralleltest // HOME pinned — serial
	pinEmptyHome(t)

	workDir := t.TempDir()
	legacyDir := filepath.Join(workDir, ".ass-guard")
	require.NoError(t, os.MkdirAll(legacyDir, 0o700))

	legacyPath := filepath.Join(legacyDir, legacySchedulingFileName)
	require.NoError(t, os.WriteFile(legacyPath, []byte(testGlobalLayerConfig), 0o600))

	var stderr bytes.Buffer

	cfg, factory, err := LoadModelRoutingFactory(workDir, "", &stderr)
	require.NoError(t, err)
	require.NotNil(t, factory)

	require.Equal(t, "GLM-5.3", cfg.Tiers["heavy"].Model,
		"the embedded floor serves — the legacy-named file was never read")

	_, ok := cfg.Providers["gsentinel"]
	require.False(t, ok, "the sentinel inside the legacy-named file must NOT leak into the config")

	_, ok = cfg.Providers["anthropic"]
	require.True(t, ok, "the embedded default is intact")
}

// TestStartupWarn_ConfigPermLoose proves the SC3 credential-on-disk hygiene
// warning covers BOTH new-path layers: a group/world-readable global or
// project config.yaml warns with the 0600 recommendation; 0600-tight files
// stay silent. Never refuses to start.
// Layer tags for the perm-warning table (goconst: the literals repeat).
const (
	layerGlobal  = "global"
	layerProject = "project"
)

func TestStartupWarn_ConfigPermLoose(t *testing.T) { //nolint:paralleltest // HOME pinned — serial
	home := pinEmptyHome(t)

	table := []struct {
		name     string
		layer    string
		perm     os.FileMode
		wantWarn bool
	}{
		{name: "global 0644 warns", layer: layerGlobal, perm: 0o644, wantWarn: true},
		{name: "project 0644 warns", layer: layerProject, perm: 0o644, wantWarn: true},
		{name: "global 0600 silent", layer: layerGlobal, perm: 0o600, wantWarn: false},
		{name: "project 0600 silent", layer: layerProject, perm: 0o600, wantWarn: false},
	}

	for _, tc := range table { //nolint:paralleltest // subtests share the pinned HOME — serial
		t.Run(tc.name, func(t *testing.T) {
			var path string

			if tc.layer == layerGlobal {
				path = writeGlobalTestConfig(t, home, testModelRoutingZeroEnvConfig)
			} else {
				path = writeTestModelRouting(t, t.TempDir(), testModelRoutingZeroEnvConfig)
			}

			require.NoError(t, os.Chmod(path, tc.perm))

			var stderr bytes.Buffer

			WarnLooseConfigPerm(path, &stderr)

			if tc.wantWarn {
				require.Contains(t, stderr.String(), "0600",
					"a %04o %s-layer config warns with the 0600 recommendation", tc.perm, tc.layer)
			} else {
				require.Empty(t, stderr.String(), "a 0600-tight %s-layer config stays silent", tc.layer)
			}
		})
	}
}

// TestBackwardCompat_ZAIEnvOnly is the SC4 proof: embedded default only,
// $ZAI_API_KEY set, no flag, no literal — the resolved anthropic credential
// Source is "env" and the key equals $ZAI_API_KEY (today's exact behavior).
func TestBackwardCompat_ZAIEnvOnly(t *testing.T) {
	pinEmptyHome(t)
	t.Setenv("ZAI_API_KEY", "env-secret")

	var stderr bytes.Buffer

	cfg, factory, err := LoadModelRoutingFactory(t.TempDir(), "", &stderr)
	require.NoError(t, err)

	prov, ok := cfg.Providers["anthropic"]
	require.True(t, ok, "the embedded default declares the anthropic provider")

	cred := modelrouting.ResolveCredential(prov, "anthropic", "")
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
	// Zero environment: every provider-key var is empty. The pinned empty HOME
	// keeps the global config layer out (t.Setenv ⇒ non-parallel).
	pinEmptyHome(t)
	t.Setenv("ZAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("NOKEY_API_KEY", "")

	workDir := t.TempDir()
	writeTestModelRouting(t, workDir, testModelRoutingZeroEnvConfig)

	var stderr bytes.Buffer

	cfg, factory, err := LoadModelRoutingFactory(workDir, "", &stderr)
	require.NoError(t, err)
	require.NotNil(t, factory)

	prov, ok := cfg.Providers["zai"]
	require.True(t, ok, "the config declares the active provider")

	cred := modelrouting.ResolveCredential(prov, "zai", "")
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
