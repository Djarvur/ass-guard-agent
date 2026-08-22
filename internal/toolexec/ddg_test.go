package toolexec //nolint:testpackage // internal package test (accesses the fetchHTML/resolve seams)

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureBackend returns a DefaultBackend whose HTTP + DNS layers are faked:
// fetch returns the given fixture body (content type text/html), resolve
// returns a public IP. Tests stay offline.
func fixtureBackend(t *testing.T, body string) *DefaultBackend {
	t.Helper()

	return &DefaultBackend{
		fetchHTML: func(_ context.Context, _ string) ([]byte, string, error) {
			return []byte(body), "text/html; charset=UTF-8", nil
		},
		resolve: func(_ context.Context, _ string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil // example.com — public
		},
	}
}

// loadFixture reads a committed testdata fixture.
func loadFixture(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	return string(data)
}

// TestDDGSearchParsesFixture (Test 1) verifies Search parses the committed
// DDG-structure fixture offline: >= 3 results, each with non-empty title,
// uddg-unwrapped absolute URL, and snippet.
func TestDDGSearchParsesFixture(t *testing.T) {
	t.Parallel()

	be := fixtureBackend(t, loadFixture(t, "ddg-results.html"))

	raw, err := be.Search(context.Background(), "golang context tutorial")
	require.NoError(t, err)

	var results []ddgResult
	require.NoError(t, json.Unmarshal(raw, &results))
	require.GreaterOrEqual(t, len(results), 3, "expected at least 3 parsed results")

	for i, r := range results {
		assert.NotEmpty(t, r.Title, "result %d title", i)
		assert.NotEmpty(t, r.URL, "result %d url", i)
		assert.NotEmpty(t, r.Snippet, "result %d snippet", i)
		assert.True(t, strings.HasPrefix(r.URL, "https://"),
			"result %d url %q must be the unwrapped absolute target", i, r.URL)
		assert.NotContains(t, r.URL, "duckduckgo.com/l/", "redirect must be unwrapped")
	}
}

// TestDDGSearchShape (Test 2) pins the model-facing JSON shape:
// [{"title":…,"url":…,"snippet":…}, …].
func TestDDGSearchShape(t *testing.T) {
	t.Parallel()

	be := fixtureBackend(t, loadFixture(t, "ddg-results.html"))

	raw, err := be.Search(context.Background(), "golang context tutorial")
	require.NoError(t, err)

	var generic []map[string]any
	require.NoError(t, json.Unmarshal(raw, &generic))
	require.NotEmpty(t, generic)

	for i, entry := range generic {
		keys := make([]string, 0, len(entry))
		for k := range entry {
			keys = append(keys, k)
		}

		assert.ElementsMatch(t, []string{"title", "url", "snippet"}, keys,
			"result %d must carry exactly title/url/snippet (got %v)", i, keys)
	}
}

// TestDDGSearchAnomalyDegradesToEmpty pins the benign-degradation behavior on
// DDG's anomaly/bot-challenge page (real captured response): zero results, no
// crash (T-8-08).
func TestDDGSearchAnomalyDegradesToEmpty(t *testing.T) {
	t.Parallel()

	be := fixtureBackend(t, loadFixture(t, "ddg-anomaly.html"))

	raw, err := be.Search(context.Background(), "anything")
	require.NoError(t, err)

	var results []ddgResult
	require.NoError(t, json.Unmarshal(raw, &results))
	assert.Empty(t, results, "anomaly page must parse to zero results")
}

// TestFetchMarkdown (Test 3) verifies HTML converts to clean markdown.
func TestFetchMarkdown(t *testing.T) {
	t.Parallel()

	be := fixtureBackend(t, "<html><body><h1>Title</h1><p>Some <b>bold</b> text</p></body></html>")

	raw, err := be.Fetch(context.Background(), "https://example.com/doc")
	require.NoError(t, err)

	var out struct {
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Contains(t, out.Content, "# Title")
	assert.Contains(t, out.Content, "**bold**")
}

// newPlainPassthroughBackend returns a DefaultBackend whose HTTP seam serves
// the given non-HTML body/content type (the passthrough legs' shared fixture).
func newPlainPassthroughBackend(body []byte, contentType string) *DefaultBackend {
	return &DefaultBackend{
		fetchHTML: func(_ context.Context, _ string) ([]byte, string, error) {
			return body, contentType, nil
		},
		resolve: func(_ context.Context, _ string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil // example.com — public
		},
	}
}

// fetchContent runs Fetch and decodes the wrapped {"content": …} form.
func fetchContent(t *testing.T, be *DefaultBackend, target string) string {
	t.Helper()

	raw, err := be.Fetch(context.Background(), target)
	require.NoError(t, err)

	var out struct {
		Content string `json:"content"`
	}

	require.NoErrorf(t, json.Unmarshal(raw, &out),
		"non-HTML Fetch output must be valid JSON (G-12-3b); got: %s", raw)

	return out.Content
}

// TestFetchNonHTMLPassthrough (Test 4) re-pinned by 12-10 (G-12-3b): a
// non-HTML body must return VALID JSON — wrapped as {"content": <body>} like
// the HTML branch — never raw bytes. The old unwrapped shape produced invalid
// JSON for any text/plain body ("invalid character p looking for beginning of
// value"), appendLine's Marshal failed, the '_ =' caller swallowed it, and
// every WebFetch tool_result silently vanished from the transcript (the UAT
// G-12-3b retry storm: 33 identical calls, zero visible results).
//
// The over-cap leg pins truncation-BEFORE-wrap (fetchRawCap); the JSON
// content-type leg pins that original JSON text travels as a STRING inside
// content (never re-encoded as an object).
func TestFetchNonHTMLPassthrough(t *testing.T) {
	t.Parallel()

	t.Run("text_plain_wrapped_as_content", func(t *testing.T) {
		t.Parallel()

		be := newPlainPassthroughBackend([]byte("plain text body"), "text/plain; charset=utf-8")

		content := fetchContent(t, be, "https://example.com/robots.txt")
		assert.Equal(t, "plain text body", content)
	})

	t.Run("over_cap_truncates_before_wrap", func(t *testing.T) {
		t.Parallel()

		body := bytes.Repeat([]byte("a"), fetchRawCap+1024)
		be := newPlainPassthroughBackend(body, "text/plain")

		raw, err := be.Fetch(context.Background(), "https://example.com/big.txt")
		require.NoError(t, err)
		require.LessOrEqual(t, len(raw), jsonMaxWrappedLen(fetchRawCap))

		content := fetchContent(t, be, "https://example.com/big.txt")
		assert.Len(t, content, fetchRawCap)
	})

	t.Run("json_content_type_travels_as_string", func(t *testing.T) {
		t.Parallel()

		originalJSON := `{"key":"value","n":3}`
		be := newPlainPassthroughBackend([]byte(originalJSON), "application/json")

		content := fetchContent(t, be, "https://example.com/data.json")
		assert.JSONEq(t, originalJSON, content,
			"content must carry the ORIGINAL JSON text, not a re-encoded object")
	})
}

// jsonMaxWrappedLen bounds the wrapped form's wire size: the {"content":…}
// envelope overhead plus JSON escaping headroom for an all-escaped payload
// (\u00XX = 6 bytes per source byte worst case).
func jsonMaxWrappedLen(sizeCap int) int {
	return sizeCap*6 + len(`{"content":""}`) + 64
}

// TestSSRFGuard (Test 5) verifies Fetch refuses loopback/private/link-local
// hosts BEFORE any network attempt, and lets public hosts through.
func TestSSRFGuard(t *testing.T) {
	t.Parallel()

	fetchCalled := false

	be := &DefaultBackend{
		fetchHTML: func(_ context.Context, u string) ([]byte, string, error) {
			fetchCalled = true

			return []byte("<html><body>ok</body></html>"), "text/html", nil
		},
		resolve: func(_ context.Context, host string) ([]net.IP, error) {
			switch host {
			case "localhost":
				return []net.IP{net.ParseIP("127.0.0.1")}, nil
			case "metadata.example":
				return []net.IP{net.ParseIP("169.254.169.254")}, nil
			case "internal.example":
				return []net.IP{net.ParseIP("10.0.0.1")}, nil
			default:
				return []net.IP{net.ParseIP("93.184.216.34")}, nil
			}
		},
	}

	for _, target := range []string{
		"http://127.0.0.1/x",
		"http://localhost/x",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.1/x",
		"file:///etc/passwd",
	} {
		fetchCalled = false

		_, err := be.Fetch(context.Background(), target)
		require.Error(t, err, "Fetch(%q) must be refused", target)
		assert.False(t, fetchCalled, "guard must fire BEFORE any request for %q", target)
	}

	_, err := be.Fetch(context.Background(), "https://example.com/")
	require.NoError(t, err, "public host must pass the guard")
	assert.True(t, fetchCalled, "public host must reach the fetch layer")
}

// TestDDGQueryEncoding (Test 6) verifies the query is URL-encoded into q=.
func TestDDGQueryEncoding(t *testing.T) {
	t.Parallel()

	var gotURL string

	be := &DefaultBackend{
		fetchHTML: func(_ context.Context, u string) ([]byte, string, error) {
			gotURL = u

			return []byte("<html></html>"), "text/html", nil
		},
	}

	_, err := be.Search(context.Background(), "go routines & context (tutorial)")
	require.NoError(t, err)

	assert.Contains(t, gotURL, "https://html.duckduckgo.com/html/?q=")
	assert.Contains(t, gotURL, urlEncodedQuery, "query must be url-encoded into q=")
}

// urlEncodedQuery is "go routines & context (tutorial)" after QueryEscape.
const urlEncodedQuery = "go+routines+%26+context+%28tutorial%29"

// TestDefaultSelection (Test 7) verifies the zero-config default: an
// empty/unset backend name resolves to the DDG default; an explicit "http"
// override still selects HTTPBackend.
func TestDefaultSelection(t *testing.T) { //nolint:paralleltest // asserts package helpers
	be, err := selectBackend("", "websearch")
	require.NoError(t, err)
	assert.Equal(t, "ddg", be.Name(), "empty name must resolve to the DDG default")

	be, err = selectBackend("ddg", "websearch")
	require.NoError(t, err)
	assert.Equal(t, "ddg", be.Name(), "explicit ddg name selects the default backend")

	be, err = selectBackend("http", "websearch")
	require.NoError(t, err)
	assert.Equal(t, "http", be.Name(), "explicit http override preserved")

	got, err := BackendsFromConfig(map[string]string{toolWebsearchCfg: ""})
	require.NoError(t, err)
	assert.Equal(t, "ddg", got["WebSearch"].Name(), "empty config value resolves to the DDG default")
}

// toolWebsearchCfg is the config-map key for WebSearch ("websearch").
const toolWebsearchCfg = "websearch"

// TestRealExecutorFallbackDefault (Test 8) verifies the zero-config fix: a
// RealExecutor with nil Backends executing WebSearch hits the injected default
// backend (DDG shape), while a configured backend still wins.
func TestRealExecutorFallbackDefault(t *testing.T) { //nolint:paralleltest // swaps package default seam
	prev := defaultWebBackend

	t.Cleanup(func() { defaultWebBackend = prev })

	defaultWebBackend = fixtureBackend(t, loadFixture(t, "ddg-results.html"))

	// Nil Backends: the fallback answers (no more "no implementation yet").
	re := &RealExecutor{}
	out, err := re.Execute(context.Background(), "WebSearch", json.RawMessage(`{"query":"mimicry"}`))
	require.NoError(t, err)

	var results []map[string]any
	require.NoError(t, json.Unmarshal(out, &results))
	assert.NotEmpty(t, results, "zero-config WebSearch must return DDG-shaped results")

	// Configured backend wins over the default (D-07 override preserved).
	fake := &fakeBackend{searchOut: json.RawMessage(`[{"title":"fake","url":"https://fake","snippet":"s"}]`)}
	re = &RealExecutor{Backends: map[string]Backend{"WebSearch": fake}}

	out, err = re.Execute(context.Background(), "WebSearch", json.RawMessage(`{"query":"mimicry"}`))
	require.NoError(t, err)
	assert.Contains(t, string(out), "fake", "configured backend must beat the default")
	assert.True(t, fake.searchCalled, "configured backend must be the one called")
}

// fakeBackend records calls for override assertions.
type fakeBackend struct {
	searchOut    json.RawMessage
	searchCalled bool
	fetchCalled  bool
}

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Search(_ context.Context, _ string) (json.RawMessage, error) {
	f.searchCalled = true

	return f.searchOut, nil
}

func (f *fakeBackend) Fetch(_ context.Context, _ string) (json.RawMessage, error) {
	f.fetchCalled = true

	return f.searchOut, nil
}

// TestUnknownBackendStillConfigError (Test 9) verifies loud misconfiguration
// is preserved: an unknown backend name yields the structured ConfigError.
func TestUnknownBackendStillConfigError(t *testing.T) {
	t.Parallel()

	_, err := BackendsFromConfig(map[string]string{toolWebsearchCfg: "bogus"})
	require.Error(t, err)

	var cfgErr *ConfigError
	require.ErrorAs(t, err, &cfgErr)
	assert.NotEmpty(t, cfgErr.Violations)
}
