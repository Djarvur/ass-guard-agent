package toolexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/toolexec"
)

// TestHTTPBackend_ImplementsInterface is the compile-time assertion that the
// default impl satisfies Backend (T-04-05).
func TestHTTPBackend_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var (
		_ toolexec.Backend = (*toolexec.HTTPBackend)(nil)
		_ toolexec.Backend = toolexec.FirecrawlBackend{}
	)
}

// TestHTTPBackend_Search verifies Search GETs the URL template with the
// {query} substituted (httptest-backed — no live network).
func TestHTTPBackend_Search(t *testing.T) {
	t.Parallel()

	var gotPath, gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"results":["a","b"]}`))
	}))
	defer srv.Close()

	b := &toolexec.HTTPBackend{
		SearchURLTemplate: srv.URL + "/search?q={query}",
	}

	out, err := b.Search(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if gotPath != "/search" {
		t.Errorf("path = %q; want /search", gotPath)
	}

	if gotQuery != "hello world" {
		t.Errorf("query = %q; want hello world", gotQuery)
	}

	var parsed map[string]any

	err = json.Unmarshal(out, &parsed)
	if err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
}

// TestHTTPBackend_Fetch verifies Fetch GETs the URL template with {url}
// substituted, or the URL directly when no template is set.
func TestHTTPBackend_Fetch(t *testing.T) {
	t.Parallel()
	t.Run("template", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"fetched":true}`))
		}))
		defer srv.Close()

		b := &toolexec.HTTPBackend{FetchURLTemplate: srv.URL + "/fetch?u={url}"}

		out, err := b.Fetch(context.Background(), "https://example.com/page")
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}

		if !strings.Contains(string(out), "fetched") {
			t.Errorf("body = %s; want fetched:true", out)
		}
	})
	t.Run("direct", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"direct":true}`))
		}))
		defer srv.Close()

		b := &toolexec.HTTPBackend{}

		out, err := b.Fetch(context.Background(), srv.URL+"/anything")
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}

		if !strings.Contains(string(out), "direct") {
			t.Errorf("body = %s; want direct:true", out)
		}
	})
}

// TestBackendsFromConfig_Select verifies the map selects the right concrete
// backend per tool; an unknown backend name yields a ConfigError (T-04-03b).
func TestBackendsFromConfig_Select(t *testing.T) {
	t.Parallel()
	t.Run("both http", func(t *testing.T) {
		t.Parallel()

		got, err := toolexec.BackendsFromConfig(map[string]string{toolWebsearch: schemeHTTP, "webfetch": schemeHTTP})
		if err != nil {
			t.Fatalf("err = %v", err)
		}

		if got["WebSearch"].Name() != schemeHTTP || got[toolWebFetch].Name() != schemeHTTP {
			t.Errorf("names = %q,%q; want http,http", got["WebSearch"].Name(), got[toolWebFetch].Name())
		}
	})
	t.Run("firecrawl websearch", func(t *testing.T) {
		t.Parallel()

		got, err := toolexec.BackendsFromConfig(map[string]string{toolWebsearch: toolFirecrawl})
		if err != nil {
			t.Fatalf("err = %v", err)
		}

		if got["WebSearch"].Name() != toolFirecrawl {
			t.Errorf("WebSearch = %q; want firecrawl", got["WebSearch"].Name())
		}

		if _, ok := got[toolWebFetch]; ok {
			t.Errorf("WebFetch unexpectedly set; want absent (not in config)")
		}
	})
	t.Run("unknown backend", func(t *testing.T) {
		t.Parallel()

		_, err := toolexec.BackendsFromConfig(map[string]string{toolWebsearch: "exfiltrate-evil"})
		if err == nil {
			t.Fatal("err = nil; want ConfigError for unknown backend")
		}

		var ce *toolexec.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("err type = %T; want *toolexec.ConfigError", err)
		}

		if !strings.Contains(err.Error(), "exfiltrate-evil") {
			t.Errorf("err = %v; want it to name the unknown backend", err)
		}
	})
}

// TestBackend_SwappableWithoutCodeChange verifies swapping the WebSearch backend
// by config alone (no edit to HTTPBackend/FirecrawlBackend) returns a different
// concrete type — the swap seam (D-22 / TOOL-05).
func TestBackend_SwappableWithoutCodeChange(t *testing.T) {
	t.Parallel()

	httpMap, _ := toolexec.BackendsFromConfig(map[string]string{toolWebsearch: schemeHTTP})
	fireMap, _ := toolexec.BackendsFromConfig(map[string]string{toolWebsearch: toolFirecrawl})
	httpName := httpMap["WebSearch"].Name()

	fireName := fireMap["WebSearch"].Name()
	if httpName == fireName {
		t.Errorf("config swap returned same backend (%s); want http != firecrawl (swap seam broken)", httpName)
	}
}

// TestFirecrawlBackend_NotConfigured verifies the stub returns a structured
// "not configured" error when the API key is empty (the swap seam is exercised;
// no firecrawl dependency added).
func TestFirecrawlBackend_NotConfigured(t *testing.T) {
	t.Parallel()

	f := toolexec.FirecrawlBackend{}

	_, err := f.Search(context.Background(), "x")
	if err == nil {
		t.Error("Search with empty API key = nil; want not-configured error")
	}

	_, err = f.Fetch(context.Background(), "x")
	if err == nil {
		t.Error("Fetch with empty API key = nil; want not-configured error")
	}
}
