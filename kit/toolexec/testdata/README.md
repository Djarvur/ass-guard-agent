# toolexec web-tool fixtures

## ddg-results.html

Parse fixture for `DefaultBackend.Search` (DuckDuckGo HTML endpoint scrape).

**Structure fidelity:** the SERP markup follows DDG's real html-endpoint
contract — `result__body` rows, `result__title > a.result__a` anchors,
`a.result__snippet` siblings, and `//duckduckgo.com/l/?uddg=<encoded>`
redirect hrefs — corroborated against four independent DDG-HTML scrapers
(oh-my-pi, odysseus, openclaw, pi-web-access). Result data (titles, URLs,
snippets) is real, captured from a live "golang context tutorial" search
(2026-08-14).

**Regenerate on a clean-egress machine** (datacenter/shared IPs receive DDG's
anomaly challenge instead of results — see ddg-anomaly.html):

```sh
curl -sS -A "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36" \
  "https://html.duckduckgo.com/html/?q=golang+context+tutorial" \
  -o internal/toolexec/testdata/ddg-results.html
```

If the response contains `anomaly-modal`, the egress IP is bot-flagged — try a
residential connection. The parser tests are structure-pinned: regenerating
with a fresh real response keeps them green as long as the class contract
holds; if DDG changes shape, regenerate + fix the parser together (the
`lite.duckduckgo.com/lite/` endpoint is the documented fallback — see ddg.go).

## ddg-anomaly.html

A REAL captured DDG response (HTTP 202 anomaly challenge) from this project's
build environment on 2026-08-14 — every egress path (direct curl + scrape
proxy) received this page. Committed to pin `DefaultBackend`'s
benign-degradation behavior: a challenge page parses to zero results (an
empty array, never a crash — T-8-08).
