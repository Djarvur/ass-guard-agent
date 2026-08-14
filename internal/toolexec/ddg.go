package toolexec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"golang.org/x/net/html"
)

// DDG HTML-endpoint contract (predecessor-proven, STACK research). The lite
// endpoint (https://lite.duckduckgo.com/lite/?q=) is the documented fallback
// if html/ changes shape — the parser is fixture-pinned (testdata/README.md)
// so drift is caught by regenerating fixtures, not by CI network calls.
const (
	ddgSearchURL = "https://html.duckduckgo.com/html/?q="
	// ddgUserAgent: a browser-like UA — the endpoint serves a JS challenge to
	// plain HTTP clients.
	ddgUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36" +
		" (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

	// fetchReadBound caps ANY read response body (T-8-07 DoS mitigation).
	fetchReadBound = 2 << 20 // 2 MiB
	// fetchRawCap truncates non-HTML passthrough bodies (documented cap).
	fetchRawCap = 512 << 10 // 512 KiB
)

// DefaultBackend is the zero-config default Backend (D-07): WebSearch scrapes
// DDG's HTML endpoint (no API key — keyed APIs remain config-swappable entries
// on the same seam), WebFetch GETs the URL, converting HTML bodies to markdown
// via JohannesKaufmann/html-to-markdown (D-08) and passing other content types
// through bounded-raw. SSRF guards (http/https only, loopback/private/link-local
// refused BEFORE any request) protect the model-controlled URL path (T-8-05).
type DefaultBackend struct {
	Client *http.Client

	// fetchHTML is the HTTP seam (body, contentType, error). Production uses
	// net/http with a 30s-timeout client + browser-like UA; tests inject
	// committed fixtures so the suite stays offline.
	fetchHTML func(ctx context.Context, pageURL string) ([]byte, string, error)

	// resolve is the DNS seam for the SSRF guard (host → IPs). Production uses
	// net.DefaultResolver; tests inject fake resolutions.
	resolve func(ctx context.Context, host string) ([]net.IP, error)

	converter *md.Converter
}

// compile-time interface check.
var _ Backend = (*DefaultBackend)(nil)

// NewDefaultBackend returns the zero-config default backend (exported for
// wiring; the test seams stay unexported).
func NewDefaultBackend() *DefaultBackend {
	return &DefaultBackend{}
}

// Name returns the backend identifier.
func (*DefaultBackend) Name() string { return "ddg" }

// ddgResult is the model-facing WebSearch result shape: [{"title":…,"url":…,"snippet":…}].
type ddgResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Search scrapes DDG's HTML endpoint for the query, parsing result anchors
// (class result__a, uddg-redirect unwrapped) + sibling snippets
// (class result__snippet) into the pinned JSON array shape. A page with no
// parseable results (including DDG's bot-challenge anomaly page) degrades
// benignly to an empty array — never a crash (T-8-08).
func (d *DefaultBackend) Search(ctx context.Context, query string) (json.RawMessage, error) {
	body, _, err := d.fetch(ctx, ddgSearchURL+url.QueryEscape(query))
	if err != nil {
		return nil, fmt.Errorf("toolexec: ddg search fetch: %w", err)
	}

	results, err := parseDDGResults(body)
	if err != nil {
		return nil, fmt.Errorf("toolexec: ddg parse: %w", err)
	}

	out, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("toolexec: ddg marshal: %w", err) //nolint:wrapcheck-v2 // marshaling typed slice
	}

	return out, nil
}

// Fetch GETs the target. Loopback/private/link-local hosts are refused BEFORE
// any request (T-8-05); HTML bodies convert to markdown (D-08); other content
// types pass through raw, truncated at fetchRawCap.
func (d *DefaultBackend) Fetch(ctx context.Context, target string) (json.RawMessage, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("toolexec: fetch url %q: %w", target, err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("toolexec: fetch refused %q: scheme must be http/https", target) //nolint:err113 // guard error
	}

	if err := d.requirePublicHost(ctx, u.Hostname()); err != nil {
		return nil, fmt.Errorf("toolexec: fetch refused %q: %w", target, err)
	}

	body, contentType, err := d.fetch(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("toolexec: fetch %q: %w", target, err)
	}

	if isHTMLContentType(contentType) {
		markdown, err := d.markdownConverter().ConvertString(string(body))
		if err != nil {
			return nil, fmt.Errorf("toolexec: html→markdown %q: %w", target, err)
		}

		out, err := json.Marshal(map[string]string{"content": markdown}) //nolint:err113 // fixed shape
		if err != nil {
			return nil, fmt.Errorf("toolexec: fetch marshal: %w", err)
		}

		return out, nil
	}

	if len(body) > fetchRawCap {
		body = body[:fetchRawCap]
	}

	return body, nil
}

// markdownConverter lazily builds the html-to-markdown converter (v1 API:
// NewConverter(domain, enableCommonmark, options) — verified against v1.6.0).
func (d *DefaultBackend) markdownConverter() *md.Converter {
	if d.converter == nil {
		d.converter = md.NewConverter("", true, nil)
	}

	return d.converter
}

// fetch routes through the injectable HTTP seam (offline tests) or the
// production net/http client with a browser-like UA and a 2 MiB read bound.
func (d *DefaultBackend) fetch(ctx context.Context, pageURL string) ([]byte, string, error) {
	if d.fetchHTML != nil {
		return d.fetchHTML(ctx, pageURL)
	}

	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: mnd30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, http.NoBody)
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("User-Agent", ddgUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http get: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchReadBound))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	return body, resp.Header.Get("Content-Type"), nil
}

// requirePublicHost enforces the SSRF guard: the host (IP literal or resolved
// name) must not resolve to loopback/private/link-local/unspecified ranges
// (cloud metadata endpoints included) — checked BEFORE any request.
func (d *DefaultBackend) requirePublicHost(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("host %s is not a public address", host) //nolint:err113 // guard error
		}

		return nil
	}

	ips, err := d.resolveHost(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", host, err)
	}

	for _, ip := range ips {
		if !isPublicIP(ip) {
			return fmt.Errorf("host %s resolves to non-public %s", host, ip) //nolint:err113 // guard error
		}
	}

	return nil
}

// resolveHost routes DNS through the injectable seam.
func (d *DefaultBackend) resolveHost(ctx context.Context, host string) ([]net.IP, error) {
	if d.resolve != nil {
		return d.resolve(ctx, host)
	}

	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

// isPublicIP reports whether ip is outside every refused range: loopback,
// private (RFC 1918 + fc00::/7), link-local (169.254.0.0/16 — cloud metadata),
// unspecified, multicast.
func isPublicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast())
}

// isHTMLContentType matches text/html and application/xhtml content types.
func isHTMLContentType(contentType string) bool {
	ct := strings.ToLower(contentType)

	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml")
}

// parseDDGResults walks the DDG html SERP: anchors with class result__a
// (title + href) paired by document order with class result__snippet elements.
func parseDDGResults(body []byte) ([]ddgResult, error) {
	root, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	var hrefs []string

	var titles []string

	var snippets []string

	var walk func(n *html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			classes := nodeClasses(n)

			switch {
			case n.Data == "a" && hasClass(classes, "result__a"):
				hrefs = append(hrefs, attrValue(n, "href"))
				titles = append(titles, nodeText(n))
			case hasClass(classes, "result__snippet"):
				snippets = append(snippets, strings.TrimSpace(nodeText(n)))
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(root)

	out := make([]ddgResult, 0, len(hrefs))

	for i, href := range hrefs {
		resolved := resolveDDGHref(href)
		if resolved == "" {
			continue
		}

		snippet := ""
		if i < len(snippets) {
			snippet = snippets[i]
		}

		out = append(out, ddgResult{Title: strings.TrimSpace(titles[i]), URL: resolved, Snippet: snippet})
	}

	return out, nil
}

// resolveDDGHref unwraps DDG's /l/?uddg=<encoded> redirect links to the real
// target. Host matching is exact (duckduckgo.com or a .duckduckgo.com
// subdomain) — never a substring match, which look-alike hosts would defeat.
// Protocol-relative URLs are absolutized to https. Unwrapped targets with a
// non-http(s) scheme (e.g. javascript:) are dropped.
func resolveDDGHref(href string) string {
	candidate := href
	if strings.HasPrefix(candidate, "//") {
		candidate = "https:" + candidate
	}

	u, err := url.Parse(candidate)
	if err != nil {
		return ""
	}

	host := strings.ToLower(u.Hostname())
	isDDG := host == "duckduckgo.com" || strings.HasSuffix(host, ".duckduckgo.com")
	if isDDG && strings.TrimSuffix(u.Path, "/") == "/l" {
		if target := u.Query().Get("uddg"); target != "" {
			tu, terr := url.Parse(target)
			if terr != nil || (tu.Scheme != "http" && tu.Scheme != "https") {
				return ""
			}

			return target
		}
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}

	return candidate
}

// nodeClasses splits an element's class attribute value.
func nodeClasses(n *html.Node) []string {
	return strings.Fields(attrValue(n, "class"))
}

// hasClass reports whether the class list contains the exact token.
func hasClass(classes []string, want string) bool {
	for _, c := range classes {
		if c == want {
			return true
		}
	}

	return false
}

// attrValue returns an element attribute's value ("" when absent).
func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}

	return ""
}

// nodeText concatenates all descendant text of a node.
func nodeText(n *html.Node) string {
	var sb strings.Builder

	var collect func(node *html.Node)

	collect = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}

		for c := node.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}

	collect(n)

	return sb.String()
}
