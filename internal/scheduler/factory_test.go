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

// ambientKeyVars are the provider-key env vars that could leak from the host
// into a precedence test; each case clears them before applying its own envs.
var ambientKeyVars = []string{"ZAI_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "NOKEY_API_KEY"}

// TestResolveCredential_Precedence is the D-05 table: flag > provider env var
// (explicit api_key_env or derived <PROVIDER>_API_KEY) > config literal; the
// ${VAR} inline form expands from the environment; an unresolvable provider is
// uncredentialed (Source == ""), never a panic.
func TestResolveCredential_Precedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		prov         ProviderConfig
		providerName string
		flagKey      string
		envs         map[string]string
		wantSource   string
		wantKey      string
	}{
		{
			name:         "env via explicit api_key_env",
			prov:         ProviderConfig{APIKeyEnv: "ZAI_API_KEY"},
			providerName: "anthropic",
			envs:         map[string]string{"ZAI_API_KEY": "env-secret"},
			wantSource:   "env",
			wantKey:      "env-secret",
		},
		{
			name:         "flag beats env and config",
			prov:         ProviderConfig{APIKeyEnv: "ZAI_API_KEY", APIKey: "sk-lit"},
			providerName: "anthropic",
			flagKey:      "flag-secret",
			envs:         map[string]string{"ZAI_API_KEY": "env-secret"},
			wantSource:   "flag",
			wantKey:      "flag-secret",
		},
		{
			name:         "config literal when no flag and no env",
			prov:         ProviderConfig{APIKey: "sk-lit"},
			providerName: "zai",
			wantSource:   "config",
			wantKey:      "sk-lit",
		},
		{
			name:         "dollar-brace inline expansion",
			prov:         ProviderConfig{APIKey: "${MY_KEY}"},
			providerName: "zai",
			envs:         map[string]string{"MY_KEY": "expanded-secret"},
			wantSource:   "env",
			wantKey:      "expanded-secret",
		},
		{
			name:         "dollar-brace unset is uncredentialed, not a panic",
			prov:         ProviderConfig{APIKey: "${MY_KEY}"},
			providerName: "zai",
			wantSource:   "",
			wantKey:      "",
		},
		{
			name:         "env var derived from provider name",
			prov:         ProviderConfig{},
			providerName: "openai",
			envs:         map[string]string{"OPENAI_API_KEY": "derived-secret"},
			wantSource:   "env",
			wantKey:      "derived-secret",
		},
		{
			name:         "malformed brace is treated as a literal",
			prov:         ProviderConfig{APIKey: "${NOPE"},
			providerName: "zai",
			wantSource:   "config",
			wantKey:      "${NOPE",
		},
		{
			name:         "nothing configured is uncredentialed",
			prov:         ProviderConfig{},
			providerName: "anthropic",
			wantSource:   "",
			wantKey:      "",
		},
	}

	for _, tt := range tests {
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
	t.Setenv("ZAI_API_KEY", "env-key-for-zai")

	cfg, err := Load("testdata/providers-cred.yaml")
	require.NoError(t, err)

	f := NewProviderFactory(cfg, "", nil)

	p, err := f.Build("zai", shaper.New())
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
	t.Setenv("NOKEY_API_KEY", "")

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
	require.Contains(t, perr.Reason, "NOKEY_API_KEY", "reason must name the env var to set")
}

// TestProviderFactory_WarnUncredentialed asserts the startup-warn helper emits
// one line per uncredentialed provider naming the provider + env var, and
// never prints a key value.
func TestProviderFactory_WarnUncredentialed(t *testing.T) {
	t.Setenv("NOKEY_API_KEY", "")

	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"haskey": {Shape: providerAnthropic, APIKey: "sk-lit"},
			"nokey":  {Shape: providerOpenAI},
		},
	}

	f := NewProviderFactory(cfg, "", nil)

	var buf bytes.Buffer
	f.WarnUncredentialed(&buf)

	out := buf.String()
	require.Contains(t, out, "nokey", "warning must name the uncredentialed provider")
	require.Contains(t, out, "NOKEY_API_KEY", "warning must name the env var")
	require.NotContains(t, out, "haskey", "credentialed provider must not warn")
	require.NotContains(t, out, "sk-lit", "warning must never print the key")
}
