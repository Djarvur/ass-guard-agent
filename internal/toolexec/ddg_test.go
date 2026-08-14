package toolexec //nolint:testpackage // internal package test (accesses the fetchHTML/resolve seams)

import (
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
	be := fixtureBackend(t, loadFixture(t, "ddg-anomaly.html"))

	raw, err := be.Search(context.Background(), "anything")
	require.NoError(t, err)

	var results []ddgResult
	require.NoError(t, json.Unmarshal(raw, &results))
	assert.Empty(t, results, "anomaly page must parse to zero results")
}

// TestFetchMarkdown (Test 3) verifies HTML converts to clean markdown.
func TestFetchMarkdown(t *testing.T) {
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

// TestFetchNonHTMLPassthrough (Test 4) verifies text/plain bodies return the
// raw text unconverted.
func TestFetchNonHTMLPassthrough(t *testing.T) {
	be := &DefaultBackend{
		fetchHTML: func(_ context.Context, _ string) ([]byte, string, error) {
			return []byte("plain text body"), "text/plain; charset=utf-8", nil
		},
		resolve: func(_ context.Context, _ string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		},
	}

	raw, err := be.Fetch(context.Background(), "https://example.com/robots.txt")
	require.NoError(t, err)
	assert.Equal(t, "plain text body", string(raw))
}

// TestSSRFGuard (Test 5) verifies Fetch refuses loopback/private/link-local
// hosts BEFORE any network attempt, and lets public hosts through.
func TestSSRFGuard(t *testing.T) {
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
