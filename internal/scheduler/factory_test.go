package scheduler //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	testNoKeySlug  = "nokey"
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
			testNoKeySlug: {Shape: providerAnthropic},
		},
	}

	f := NewProviderFactory(cfg, "", nil)

	p, err := f.Build(testNoKeySlug, nil)
	require.NoError(t, err, "Build must not fail eagerly for an uncredentialed provider")
	require.NotNil(t, p)

	_, err = p.Send(context.Background(), &profile.Profile{}, nil)
	require.Error(t, err)

	var perr *provider.ProviderError
	require.ErrorAs(t, err, &perr)
	require.Equal(t, provider.KindStructural, perr.Kind)
	require.Equal(t, testNoKeySlug, perr.Provider)
	require.Contains(t, perr.Reason, testNoKeyEnv, "reason must name the env var to set")
}

// TestProviderFactory_WarnUncredentialed asserts the startup-warn helper emits
// one line per uncredentialed provider naming the provider + env var, and
// never prints a key value.
func TestProviderFactory_WarnUncredentialed(t *testing.T) {
	t.Setenv(testNoKeyEnv, "")

	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"haskey":      {Shape: providerAnthropic, APIKey: testLitKey},
			testNoKeySlug: {Shape: providerOpenAI},
		},
	}

	f := NewProviderFactory(cfg, "", nil)

	var buf bytes.Buffer
	f.WarnUncredentialed(&buf)

	out := buf.String()
	require.Contains(t, out, testNoKeySlug, "warning must name the uncredentialed provider")
	require.Contains(t, out, testNoKeyEnv, "warning must name the env var")
	require.NotContains(t, out, "haskey", "credentialed provider must not warn")
	require.NotContains(t, out, testLitKey, "warning must never print the key")
}

// TestProviderFactory_Endpoint asserts the Plan 07-02 cross-plan accessor: it
// returns the configured base_url + resolved key for a credentialed provider
// (from env or config), and ok=false for an undeclared or uncredentialed one
// (D-07 lazy semantics).
func TestProviderFactory_Endpoint(t *testing.T) {
	t.Setenv(testZAIEnv, "") // zai's configured env var forced empty -> literal wins
	t.Setenv("OTHER_API_KEY", "other-secret")

	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"zaienv": {BaseURL: "https://api.z.ai/api/anthropic", Shape: providerAnthropic, APIKeyEnv: "OTHER_API_KEY"},
			testZaiSlug: {
				BaseURL:   "https://api.z.ai/api/anthropic",
				Shape:     providerAnthropic,
				APIKeyEnv: testZAIEnv, // forced empty -> the config literal wins
				APIKey:    testLitKey,
			},
			testNoKeySlug: {
				Shape: providerAnthropic,
			},
		},
	}

	f := NewProviderFactory(cfg, "", nil)

	baseURL, key, ok := f.Endpoint("zaienv")
	require.True(t, ok)
	require.Equal(t, "https://api.z.ai/api/anthropic", baseURL)
	require.Equal(t, "other-secret", key)

	baseURL, key, ok = f.Endpoint(testZaiSlug)
	require.True(t, ok)
	require.Equal(t, "https://api.z.ai/api/anthropic", baseURL)
	require.Equal(t, testLitKey, key, "config literal resolves when the env var is empty")

	_, _, ok = f.Endpoint(testNoKeySlug)
	require.False(t, ok, "an uncredentialed provider has no endpoint credential")

	_, _, ok = f.Endpoint("undeclared")
	require.False(t, ok, "an undeclared provider has no endpoint")
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

		w.Header().Set("Content-Type", "text/event-stream")
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
	require.Equal(t, strings.TrimPrefix(srv.URL, "http://"), gotHost,
		"request Host must be the configured base_url host")
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

// --- 09-01 T1: BuildWithCapturer — the single factory-seam capturer (AUD-01) ---

// captureRecord is what a test capturer observed: the verbatim body bytes +
// the header map the adapter passed (nil for the OpenAI Send path).
type captureRecord struct {
	body    []byte
	headers map[string]string
}

// anthropicCaptureStub is an httptest server speaking the minimal Anthropic
// SSE stream (same shape as TestProviderFactory_WireRoundTrip's).
func anthropicCaptureStub(got *captureRecord) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		got.body = body
		got.headers = map[string]string{"X-Api-Key": r.Header.Get("X-Api-Key")}

		w.Header().Set("Content-Type", "text/event-stream")
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
}

// TestBuildWithCapturer_AnthropicShape (09-01 T1 Test 1, AUD-01): a
// credentialed anthropic-shape provider built through BuildWithCapturer fires
// the capturer EXACTLY ONCE with the verbatim wire body + headers during a
// Stream — today only the tracer's hand-rebuilt path could do this.
func TestBuildWithCapturer_AnthropicShape(t *testing.T) {
	t.Setenv(testZAIEnv, "") // no ambient key interference

	var got captureRecord

	srv := anthropicCaptureStub(&got)
	defer srv.Close()

	cfg := &Config{Providers: map[string]ProviderConfig{
		"anthropic": {BaseURL: srv.URL, Shape: providerAnthropic, APIKey: "sk-cap-anthropic"},
	}}

	f := NewProviderFactory(cfg, "", nil)

	var fired int

	p, err := f.BuildWithCapturer("anthropic", shaper.New(), func(body []byte, headers map[string]string) {
		fired++
		got.body = body
		got.headers = headers
	})
	require.NoError(t, err)

	ch, err := p.Stream(context.Background(),
		&profile.Profile{Model: "glm-5.2", MaxTokens: 100},
		[]shaper.Message{{Role: "user", Content: "hi"}})
	require.NoError(t, err)

	drainStreamChannel(t, ch)

	require.Equal(t, 1, fired, "capturer must fire exactly once per Stream")
	require.NotEmpty(t, got.body, "captured body must be the verbatim wire bytes")
	require.Contains(t, string(got.body), `"model":"glm-5.2"`, "body is the shaped request")
	require.Contains(t, got.headers["X-Api-Key"], "sk-cap-anthropic", "headers carry the resolved key")
}

// TestBuildWithCapturer_OpenAIShape (09-01 T1 Test 2, AUD-01): the SAME seam
// attaches the capturer on the openai shape — a Send against a chat-completions
// stub fires the capturer with the marshaled OpenAI wire body.
func TestBuildWithCapturer_OpenAIShape(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	var got captureRecord

	var fired int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,` +
			`"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},` +
			`"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	cfg := &Config{Providers: map[string]ProviderConfig{
		"oai": {BaseURL: srv.URL, Shape: providerOpenAI, APIKey: "sk-cap-openai"},
	}}

	f := NewProviderFactory(cfg, "", nil)

	p, err := f.BuildWithCapturer("oai", nil, func(body []byte, headers map[string]string) {
		fired++
		got.body = body
		got.headers = headers
	})
	require.NoError(t, err)

	_, err = p.Send(context.Background(),
		&profile.Profile{Model: "gpt-test", MaxTokens: 100},
		[]shaper.Message{{Role: "user", Content: "hi"}})
	require.NoError(t, err)

	require.Equal(t, 1, fired, "capturer must fire exactly once per Send")
	require.Contains(t, string(got.body), `"model":"gpt-test"`, "captured body is the OpenAI wire request")
}

// TestBuildWithCapturer_Uncredentialed (09-01 T1 Test 3, D-07): the lazy
// noCredentialProvider semantics are unchanged — the capturer NEVER fires
// because nothing is ever shaped.
func TestBuildWithCapturer_Uncredentialed(t *testing.T) {
	t.Setenv(testNoKeyEnv, "")

	cfg := &Config{Providers: map[string]ProviderConfig{
		testNoKeySlug: {Shape: providerAnthropic},
	}}

	f := NewProviderFactory(cfg, "", nil)

	fired := false

	p, err := f.BuildWithCapturer(testNoKeySlug, nil, func([]byte, map[string]string) { fired = true })
	require.NoError(t, err, "uncredentialed must stay lazy, not a Build error")

	_, err = p.Send(context.Background(), &profile.Profile{}, nil)
	require.Error(t, err)

	require.False(t, fired, "nothing is shaped — the capturer must never fire")
}

// TestBuildWithCapturer_Undeclared (09-01 T1 Test 4): the undeclared-provider
// error is identical to Build's.
func TestBuildWithCapturer_Undeclared(t *testing.T) {
	f := NewProviderFactory(&Config{Providers: map[string]ProviderConfig{}}, "", nil)

	_, err := f.BuildWithCapturer("ghost", nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not declared in providers")
}

// TestBuildWithCapturer_Delegation (09-01 T1 Test 5): Build(name, sh) and
// BuildWithCapturer(name, sh, nil) construct indistinguishable providers — the
// nil capturer is a no-op inside the adapters (verified at their capture sites).
func TestBuildWithCapturer_Delegation(t *testing.T) {
	t.Setenv(testZAIEnv, "env-key-for-zai")

	cfg, err := Load("testdata/providers-cred.yaml")
	require.NoError(t, err)

	f := NewProviderFactory(cfg, "", nil)

	viaBuild, err := f.Build(testZaiSlug, shaper.New())
	require.NoError(t, err)

	viaSeam, err := f.BuildWithCapturer(testZaiSlug, shaper.New(), nil)
	require.NoError(t, err)

	require.IsType(t, viaBuild, viaSeam)
	require.Equal(t, fmt.Sprintf("%T", viaBuild), fmt.Sprintf("%T", viaSeam))
}
