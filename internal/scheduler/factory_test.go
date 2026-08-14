package scheduler //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// TestProviderFactory_WireRoundTrip drives a factory-built AnthropicProvider
// through Stream against an httptest server and asserts the resolved key +
// configured base_url reach the wire: URL path /v1/messages, request Host ==
// the configured base_url host, X-Api-Key == the resolved key (PROV-01's
// configurable base URL + PCFG-02's resolved key, end-to-end).
func TestProviderFactory_WireRoundTrip(t *testing.T) {
	var gotPath, gotHost, gotKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHost = r.Host
		gotKey = r.Header.Get("X-Api-Key")

		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		for _, frame := range []string{
			`{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", frame)

			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	t.Setenv(testZAIEnv, "") // no ambient key interference

	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"anthropic": {
				BaseURL: srv.URL,
				Shape:   providerAnthropic,
				APIKey:  "sk-wire-secret",
			},
		},
	}

	f := NewProviderFactory(cfg, "", nil)

	p, err := f.Build("anthropic", shaper.New())
	require.NoError(t, err)

	ch, err := p.Stream(context.Background(),
		&profile.Profile{Model: "glm-5.2", MaxTokens: 100},
		[]shaper.Message{{Role: "user", Content: "hi"}})
	require.NoError(t, err)
	require.NotNil(t, ch)
	drainStreamChannel(t, ch)

	require.Equal(t, "/v1/messages", gotPath, "base_url must be honored")
	require.Equal(t, strings.TrimPrefix(srv.URL, "http://"), gotHost, "request Host must be the configured base_url host")
	require.Equal(t, "sk-wire-secret", gotKey, "X-Api-Key must carry the resolved key")
}

// drainStreamChannel drains a Stream channel until it closes, failing the test
// if the provider never closes it within the deadline.
func drainStreamChannel(t *testing.T, ch <-chan provider.StreamChunk) {
	t.Helper()

	deadline := time.After(3 * time.Second)

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream channel did not close within 3s")
		}
	}
}
