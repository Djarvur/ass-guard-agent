package toolexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const mnd30 = 30

var errFirecrawlBackendNot = errors.New("toolexec: firecrawl backend not configured (API key empty)")

// Backend is the swappable WebSearch/WebFetch implementation (D-22 / TOOL-05).
// The concrete impl is selected at startup from config (BackendsFromConfig),
// so swapping backends = config + inject a different Backend, never a code
// change to RealExecutor or the catalog.
//
// WebSearch and WebFetch are the two complex tools whose backend varies by
// deployment (default HTTP, optional firecrawl/brave/etc.). Every other tool
// is catalog-driven and does NOT use this interface.
type Backend interface {
	// Name is the backend identifier ("http", "firecrawl", ...) for diagnostics.
	Name() string
	// Search implements WebSearch.
	Search(ctx context.Context, query string) (json.RawMessage, error)
	// Fetch implements WebFetch.
	Fetch(ctx context.Context, target string) (json.RawMessage, error)
}

// HTTPBackend is the default Backend: a thin net/http GET client with a
// configurable URL template per operation. The template may contain the
// literal substring "{query}" (Search) or "{url}" (Fetch) which is URL-encoded
// and substituted; if absent, the query/url is appended.
type HTTPBackend struct {
	Client            *http.Client
	SearchURLTemplate string
	FetchURLTemplate  string
}

// Name returns the backend identifier.
func (*HTTPBackend) Name() string { return "http" }

// Search issues an HTTP GET to the configured search URL template with the
// query substituted, returning the raw body.
func (h *HTTPBackend) Search(ctx context.Context, query string) (json.RawMessage, error) {
	return h.do(ctx, h.SearchURLTemplate, "query", query)
}

// Fetch issues an HTTP GET to the configured fetch URL template (or the URL
// directly when no template is set), returning the raw body.
func (h *HTTPBackend) Fetch(ctx context.Context, target string) (json.RawMessage, error) {
	if h.FetchURLTemplate == "" {
		// No template: fetch the URL directly.
		return h.getRaw(ctx, target)
	}

	return h.do(ctx, h.FetchURLTemplate, "url", target)
}

// do substitutes {placeholder} in template with the encoded value and GETs it.
// placeholder is "query" or "url".
func (h *HTTPBackend) do(ctx context.Context, template, placeholder, value string) (json.RawMessage, error) {
	if template == "" {
		//nolint:err113 // dynamic config error
		return nil, errors.New("toolexec: HTTPBackend " + placeholder + " URL template not configured")
	}

	enc := url.QueryEscape(value)
	final := strings.ReplaceAll(template, "{"+placeholder+"}", enc)

	return h.getRaw(ctx, final)
}

// getRaw performs the HTTP GET + returns the body. The client defaults to a
// 30s-timeout transport if unset.
func (h *HTTPBackend) getRaw(ctx context.Context, full string) (json.RawMessage, error) {
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: mnd30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("toolexec: build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("toolexec: HTTP get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("toolexec: read response: %w", err)
	}

	return json.RawMessage(body), nil
}

// FirecrawlBackend is a minimal stub Backend (D-22 — the swap seam). Real
// firecrawl wiring is a v1.1 concern; this stub returns a structured
// "not configured" error when the API key is empty, PROVING the backend is
// swappable by config WITHOUT adding a firecrawl dependency. It is intentionally
// not registered as a direct require in go.mod.
type FirecrawlBackend struct {
	APIKey   string
	Endpoint string
}

// Name returns the backend identifier.
func (FirecrawlBackend) Name() string { return "firecrawl" }

// Search returns a not-configured error when the API key is unset (the swap seam
// is exercised by tests; a real impl would POST to the firecrawl endpoint).
func (f FirecrawlBackend) Search(ctx context.Context, query string) (json.RawMessage, error) {
	if f.APIKey == "" {
		return nil, errFirecrawlBackendNot
	}

	//nolint:err113 // dynamic error message
	return nil, fmt.Errorf("toolexec: firecrawl Search not implemented (endpoint=%s)", f.Endpoint)
}

// Fetch returns a not-configured error when the API key is unset.
func (f FirecrawlBackend) Fetch(ctx context.Context, target string) (json.RawMessage, error) {
	if f.APIKey == "" {
		return nil, errFirecrawlBackendNot
	}

	//nolint:err113 // dynamic error message
	return nil, fmt.Errorf("toolexec: firecrawl Fetch not implemented (endpoint=%s)", f.Endpoint)
}

// BackendsFromConfig selects the concrete Backend for each complex tool from a
// config map keyed by tool name ("websearch", "webfetch") with values naming
// the backend ("http" → HTTPBackend; "firecrawl" → FirecrawlBackend). An
// unknown backend name yields a structured ConfigError naming it. The returned
// map is keyed by the catalog tool names "WebSearch"/"WebFetch" (so RealExecutor
// looks up Backends[toolName]).
//
// T-04-03b mitigation: the backend names are operator-configured, NEVER
// model-controlled, so a misconfigured backend fails loudly (ConfigError)
// rather than silently defaulting to a wrong/exfiltrating endpoint.
func BackendsFromConfig(cfg map[string]string) (map[string]Backend, error) {
	out := make(map[string]Backend)
	// Normalize keys: accept the tool name in either case.
	norm := strings.ToLower
	for k, v := range cfg {
		switch norm(k) {
		case "websearch":
			b, err := selectBackend(v, "websearch")
			if err != nil {
				return nil, err
			}

			out["WebSearch"] = b
		case "webfetch":
			b, err := selectBackend(v, "webfetch")
			if err != nil {
				return nil, err
			}

			out[toolWebFetch] = b
		default:
			// Ignore unrelated keys (the config map may carry other entries).
		}
	}

	return out, nil
}

// selectBackend maps a single backend name to its concrete impl.
func selectBackend(name, tool string) (Backend, error) { //nolint:ireturn // one of several Backend impls
	switch strings.ToLower(name) {
	case "http", "":
		return &HTTPBackend{}, nil
	case "firecrawl":
		return FirecrawlBackend{}, nil
	default:
		msg := fmt.Sprintf("%s: unknown backend %q (want http or firecrawl)", tool, name)

		return nil, &ConfigError{Violations: []string{msg}}
	}
}

// ConfigError lists one or more config violations (mirrors scheduler.ConfigError
// for a consistent operator-facing report). Used by BackendsFromConfig.
type ConfigError struct {
	Violations []string
}

// Error joins the violations with "; ".
func (e *ConfigError) Error() string {
	if len(e.Violations) == 0 {
		return "toolexec config invalid"
	}

	return strings.Join(e.Violations, "; ")
}
