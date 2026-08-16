package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// ResolvedCredential is the outcome of the D-05 precedence resolution.
// Source is one of "flag", "env", "config" — or "" (uncredentialed: nothing
// resolvable). The Key is the winning value; it must never be logged (T-07-01).
type ResolvedCredential struct {
	Source string
	Key    string
}

// Credential-source labels returned by ResolveCredential (D-05). An empty
// Source means uncredentialed.
const (
	credSourceFlag   = "flag"
	credSourceEnv    = "env"
	credSourceConfig = "config"
)

// ResolveCredential applies the locked D-05 precedence at provider-construction
// time (lazy, not at load — see 07-CONTEXT D-02/D-06/D-07 reconciliation):
//
//  1. flagKey != ""            -> {flag, flagKey}          (flag wins outright)
//  2. env var (api_key_env, or <PROVIDER>_API_KEY derived) -> {env, value}
//  3. api_key "${NAME}" form   -> expanded from $NAME       (unset -> {"",""})
//  4. api_key plain literal    -> {config, literal}
//  5. otherwise                -> {"", ""}                  (uncredentialed)
//
// Only `${NAME}` is recognized (D-02 minimal surface — no shell, no defaults
// syntax); a malformed `${` without a closing `}` is treated as a literal.
// The Key is never written to any log/stdout path here (T-07-01).
func ResolveCredential(prov ProviderConfig, providerName, flagKey string) ResolvedCredential {
	if flagKey != "" {
		return ResolvedCredential{Source: credSourceFlag, Key: flagKey}
	}

	if v := os.Getenv(credentialEnvName(prov, providerName)); v != "" {
		return ResolvedCredential{Source: credSourceEnv, Key: v}
	}

	if name, ok := envExpansionName(prov.APIKey); ok {
		if v := os.Getenv(name); v != "" {
			return ResolvedCredential{Source: credSourceEnv, Key: v}
		}

		return ResolvedCredential{}
	}

	if prov.APIKey != "" {
		return ResolvedCredential{Source: credSourceConfig, Key: prov.APIKey}
	}

	return ResolvedCredential{}
}

// credentialEnvName is the env var a provider's credential resolves from: the
// explicit api_key_env when set, else <PROVIDER>_API_KEY upper-cased (the
// anthropic provider keeps $ZAI_API_KEY via the embedded default's api_key_env).
func credentialEnvName(prov ProviderConfig, providerName string) string {
	if prov.APIKeyEnv != "" {
		return prov.APIKeyEnv
	}

	return strings.ToUpper(providerName) + "_API_KEY"
}

// envExpansionName parses the `${NAME}` inline form. The second return is false
// for anything that is not exactly `${NAME}` — a malformed `${` without a
// closing `}` falls through as a literal (D-02, T-07-03).
func envExpansionName(v string) (string, bool) {
	if !strings.HasPrefix(v, "${") || !strings.HasSuffix(v, "}") {
		return "", false
	}

	return v[2 : len(v)-1], true
}

// ProviderFactory builds a credentialed provider.Provider per declared provider
// (D-08): constructed ONCE at startup from the loaded config, keyed by provider
// name. Build resolves the credential (D-05), constructs the correct adapter
// for the shape with the configured base_url + resolved key, or returns the
// lazy noCredentialProvider when nothing resolves (D-07).
type ProviderFactory struct {
	cfg     *Config
	flagKey string
	log     *slog.Logger
}

// NewProviderFactory constructs a factory over the (validated) config. A nil
// log falls back to a stderr slog default (same pattern as NewScheduler).
func NewProviderFactory(cfg *Config, flagKey string, log *slog.Logger) *ProviderFactory {
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	return &ProviderFactory{cfg: cfg, flagKey: flagKey, log: log}
}

// Build returns the credentialed adapter for providerName — it DELEGATES to
// BuildWithCapturer with a nil capturer (the single construction seam, 09-01).
// An undeclared provider is an error; an uncredentialed provider is NOT a
// Build error — the noCredentialProvider wrapper fails lazily at first
// Send/Stream with a typed structural error (D-07: warn at startup, fail lazy
// at first use). sh may be nil for the openai shape; a nil shaper on the
// anthropic shape is the caller's error (NewAnthropicProvider rejects a nil
// Shaper at Stream time).
//
//nolint:ireturn // factory: abstraction over the concrete adapter types
func (f *ProviderFactory) Build(
	providerName string, sh *shaper.Shaper,
) (provider.Provider, error) {
	return f.BuildWithCapturer(providerName, sh, nil)
}

// BuildWithCapturer is the SINGLE provider-construction seam that can attach a
// RequestCapturer (09-01, AUD-01): same resolution path as Build (config
// lookup → ResolveCredential), then per shape it passes the existing capture
// options — anthropic: WithAnthropicRequestCapture (fires per Stream, body +
// headers); openai: WithOpenAIRequestCapture (fires per Send, marshaled wire
// body, nil headers). A nil capturer is a no-op inside the adapters (both
// capture sites nil-check), so Build delegates here with nil and the two
// constructors are indistinguishable without a capturer. The uncredentialed
// (lazy D-07) and undeclared semantics are identical to Build's — a capturer
// never fires for a provider that never shapes anything. The tracer path
// (cmd/ass-guard runTrace) and the serve path (sessionFor) BOTH construct
// through this method — no divergent copies (Pitfall 8).
//
//nolint:ireturn // factory: abstraction over the concrete adapter types
func (f *ProviderFactory) BuildWithCapturer(
	providerName string, sh *shaper.Shaper, capturer provider.RequestCapturer,
) (provider.Provider, error) {
	prov, ok := f.cfg.Providers[providerName]
	if !ok {
		//nolint:err113 // dynamic error message
		return nil, fmt.Errorf("provider %q: not declared in providers", providerName)
	}

	cred := ResolveCredential(prov, providerName, f.flagKey)
	if cred.Key == "" {
		return noCredentialProvider{name: providerName, envName: credentialEnvName(prov, providerName)}, nil
	}

	switch prov.Shape {
	case providerAnthropic:
		return provider.NewAnthropicProvider(sh,
			provider.WithAnthropicBaseURL(prov.BaseURL),
			provider.WithAnthropicAPIKey(cred.Key),
			provider.WithAnthropicRequestCapture(capturer)), nil
	case providerOpenAI:
		return provider.NewOpenAIProvider(
			provider.WithOpenAIBaseURL(prov.BaseURL),
			provider.WithOpenAIAPIKey(cred.Key),
			provider.WithOpenAIRequestCapture(capturer)), nil
	default:
		//nolint:err113 // dynamic error message
		return nil, fmt.Errorf("provider %q: unknown shape %q", providerName, prov.Shape)
	}
}

// Endpoint returns the configured base URL + the resolved credential key for a
// declared provider (Plan 07-02 cross-plan accessor: the LOG-01 tracer
// reconstructs the Anthropic adapter with factory-RESOLVED values so it can
// attach its RequestCapturer — never hardcoded defaults, D-08). ok is false
// for an undeclared provider or an uncredentialed one (D-07 lazy semantics —
// the caller falls back to Build's noCredentialProvider).
//
//nolint:nonamedreturns // (baseURL, key, ok) is the Plan 07-02 accessor contract
func (f *ProviderFactory) Endpoint(providerName string) (baseURL, key string, ok bool) {
	prov, declared := f.cfg.Providers[providerName]
	if !declared {
		return "", "", false
	}

	cred := ResolveCredential(prov, providerName, f.flagKey)
	if cred.Key == "" {
		return "", "", false
	}

	return prov.BaseURL, cred.Key, true
}

// WarnUncredentialed writes one stderr-style warning line per provider whose
// credential is unresolvable (neither flag, env, nor config), naming the
// provider + the env var to set — never the key (D-07, T-07-01). The startup
// wiring (Plan 07-02) calls this; here it is defined + unit-tested.
func (f *ProviderFactory) WarnUncredentialed(w io.Writer) {
	for _, name := range sortedKeys(f.cfg.Providers) {
		prov := f.cfg.Providers[name]
		if ResolveCredential(prov, name, f.flagKey).Key == "" {
			_, _ = fmt.Fprintf(w,
				"scheduler: provider %q has no resolvable credential — set api_key in config.yaml or $%s\n",
				name, credentialEnvName(prov, name))
		}
	}
}

// noCredentialProvider is the D-07 lazy failure wrapper: it implements
// provider.Provider but every call returns a typed structural ProviderError
// naming the provider + the env var — classified exactly like 401/403 (never
// retried, never an opaque 401). ToolResultMessage is unreachable in a real
// loop (a turn cannot get a tool call from a provider that failed to Send).
type noCredentialProvider struct {
	name    string
	envName string
}

func (p noCredentialProvider) Send(
	ctx context.Context, prof *profile.Profile, messages []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, p.noCredentialError()
}

func (p noCredentialProvider) Stream(
	ctx context.Context, prof *profile.Profile, messages []provider.Message,
) (<-chan provider.StreamChunk, error) {
	return nil, p.noCredentialError()
}

func (p noCredentialProvider) ToolResultMessage(toolCallID string, result json.RawMessage) (json.RawMessage, error) {
	return nil, p.noCredentialError()
}

func (p noCredentialProvider) noCredentialError() error {
	return &provider.ProviderError{
		Kind:     provider.KindStructural,
		Provider: p.name,
		Reason:   "no credential configured — set api_key in config.yaml or $" + p.envName,
	}
}
