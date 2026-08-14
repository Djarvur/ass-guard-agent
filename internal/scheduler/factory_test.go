package scheduler //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// Test-only fixture constants (repeated literals stay in one place).
const (
	testZAIEnv     = "ZAI_API_KEY"
	testNoKeyEnv   = "NOKEY_API_KEY"
	testZaiSlug    = "zai"
	testEnvSecret  = "env-secret"
	testFlagSecret = "flag-secret"
	testLitKey     = "sk-lit"
)

// ambientKeyVars are the provider-key env vars that could leak from the host
// into a precedence test; each case clears them before applying its own envs.
//
//nolint:gochecknoglobals // test fixture
var ambientKeyVars = []string{testZAIEnv, "OPENAI_API_KEY", "ANTHROPIC_API_KEY", testNoKeyEnv}

// precedenceCase is one row of the D-05 precedence table.
type precedenceCase struct {
	name         string
	prov         ProviderConfig
	providerName string
	flagKey      string
	envs         map[string]string
	wantSource   string
	wantKey      string
}

// precedenceCases exercises flag > provider env var (explicit api_key_env or
// derived <PROVIDER>_API_KEY) > config literal, the ${VAR} inline form, and the
// uncredentialed cases (empty Source, never a panic).
//
//nolint:gochecknoglobals // test table
var precedenceCases = []precedenceCase{
	{
		name:         "env via explicit api_key_env",
		prov:         ProviderConfig{APIKeyEnv: testZAIEnv},
		providerName: providerAnthropic,
		envs:         map[string]string{testZAIEnv: testEnvSecret},
		wantSource:   credSourceEnv,
		wantKey:      testEnvSecret,
	},
	{
		name:         "flag beats env and config",
		prov:         ProviderConfig{APIKeyEnv: testZAIEnv, APIKey: testLitKey},
		providerName: providerAnthropic,
		flagKey:      testFlagSecret,
		envs:         map[string]string{testZAIEnv: testEnvSecret},
		wantSource:   credSourceFlag,
		wantKey:      testFlagSecret,
	},
	{
		name:         "config literal when no flag and no env",
		prov:         ProviderConfig{APIKey: testLitKey},
		providerName: testZaiSlug,
		wantSource:   credSourceConfig,
		wantKey:      testLitKey,
	},
	{
		name:         "dollar-brace inline expansion",
		prov:         ProviderConfig{APIKey: "${MY_KEY}"},
		providerName: testZaiSlug,
		envs:         map[string]string{"MY_KEY": "expanded-secret"},
		wantSource:   credSourceEnv,
		wantKey:      "expanded-secret",
	},
	{
		name:         "dollar-brace unset is uncredentialed, not a panic",
		prov:         ProviderConfig{APIKey: "${MY_KEY}"},
		providerName: testZaiSlug,
		wantSource:   "",
		wantKey:      "",
	},
	{
		name:         "env var derived from provider name",
		prov:         ProviderConfig{},
		providerName: "openai",
		envs:         map[string]string{"OPENAI_API_KEY": "derived-secret"},
		wantSource:   credSourceEnv,
		wantKey:      "derived-secret",
	},
	{
		name:         "malformed brace is treated as a literal",
		prov:         ProviderConfig{APIKey: "${NOPE"},
		providerName: testZaiSlug,
		wantSource:   credSourceConfig,
		wantKey:      "${NOPE",
	},
	{
		name:         "nothing configured is uncredentialed",
		prov:         ProviderConfig{},
		providerName: providerAnthropic,
		wantSource:   "",
		wantKey:      "",
	},
}

// TestResolveCredential_Precedence runs the D-05 precedence table.
func TestResolveCredential_Precedence(t *testing.T) {
	// NOT parallel: subtests use t.Setenv, which is forbidden in parallel tests.
	for _, tt := range precedenceCases {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range ambientKeyVars {
				t.Setenv(k, "")
			}

			for k, v := range tt.envs {
				t.Setenv(k, v)
			}

			got := ResolveCredential(tt.prov, tt.providerName, tt.flagKey)
			require.Equal(t, tt.wantSource, got.Source, "Source")
			require.Equal(t, tt.wantKey, got.Key, "Key")
		})
	}
}

// TestProviderFactory_BuildConstruction asserts Build returns the correct
// adapter type per declared provider shape, constructed from the loaded
// testdata/providers-cred.yaml (zai -> AnthropicProvider, oai -> OpenAIProvider).
func TestProviderFactory_BuildConstruction(t *testing.T) {
	t.Setenv(testZAIEnv, "env-key-for-zai")

	cfg, err := Load("testdata/providers-cred.yaml")
	require.NoError(t, err)

	f := NewProviderFactory(cfg, "", nil)

	p, err := f.Build(testZaiSlug, shaper.New())
	require.NoError(t, err)
	require.IsType(t, &provider.AnthropicProvider{}, p)

	p, err = f.Build("oai", nil)
	require.NoError(t, err)
	require.IsType(t, &provider.OpenAIProvider{}, p)
}

// TestProviderFactory_NoCredentialLazy asserts an unresolvable credential is
// NOT a Build-time failure: Build returns a provider whose first Send fails
// with a typed structural ProviderError naming the provider + the env var.
func TestProviderFactory_NoCredentialLazy(t *testing.T) {
	t.Setenv(testNoKeyEnv, "")

	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"nokey": {Shape: providerAnthropic},
		},
	}

	f := NewProviderFactory(cfg, "", nil)

	p, err := f.Build("nokey", nil)
	require.NoError(t, err, "Build must not fail eagerly for an uncredentialed provider")
	require.NotNil(t, p)

	_, err = p.Send(context.Background(), &profile.Profile{}, nil)
	require.Error(t, err)

	var perr *provider.ProviderError
	require.ErrorAs(t, err, &perr)
	require.Equal(t, provider.KindStructural, perr.Kind)
	require.Equal(t, "nokey", perr.Provider)
	require.Contains(t, perr.Reason, testNoKeyEnv, "reason must name the env var to set")
}

// TestProviderFactory_WarnUncredentialed asserts the startup-warn helper emits
// one line per uncredentialed provider naming the provider + env var, and
// never prints a key value.
func TestProviderFactory_WarnUncredentialed(t *testing.T) {
	t.Setenv(testNoKeyEnv, "")

	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"haskey": {Shape: providerAnthropic, APIKey: testLitKey},
			"nokey":  {Shape: providerOpenAI},
		},
	}

	f := NewProviderFactory(cfg, "", nil)

	var buf bytes.Buffer
	f.WarnUncredentialed(&buf)

	out := buf.String()
	require.Contains(t, out, "nokey", "warning must name the uncredentialed provider")
	require.Contains(t, out, testNoKeyEnv, "warning must name the env var")
	require.NotContains(t, out, "haskey", "credentialed provider must not warn")
	require.NotContains(t, out, testLitKey, "warning must never print the key")
}
