---
phase: 08-slash-command-kickoff
plan: 02
subsystem: web-tools
tags: [websearch, webfetch, duckduckgo, html-to-markdown, ssrf, zero-config]

# Dependency graph
requires:
  - phase: 04 tool-executor (v1.0)
    provides: toolexec.Backend swappable seam + RealExecutor dispatch
provides:
  - DefaultBackend: DDG-HTML WebSearch (no API key) + markdown WebFetch with SSRF guards
  - zero-config first run has WORKING web tools (RealExecutor falls back to the DDG default when Backends is nil)
  - offline fixture-pinned tests (ddg-results.html + real captured ddg-anomaly.html)
affects: [08-06 e2e (opsx flows needing web tools), any deployment relying on zero-config web tools]

# Tech tracking
tech-stack:
  added: ["github.com/JohannesKaufmann/html-to-markdown v1.6.0", "golang.org/x/net v0.58.0 (upgraded)"]
  patterns:
    - "injectable HTTP fetch + DNS resolve seams keep the web-tool suite offline"
    - "real-captured anomaly fixture pins benign zero-result degradation"

key-files:
  created:
    - internal/toolexec/ddg.go
    - internal/toolexec/ddg_test.go
    - internal/toolexec/testdata/ddg-results.html
    - internal/toolexec/testdata/ddg-anomaly.html
    - internal/toolexec/testdata/README.md
  modified:
    - internal/toolexec/backend.go
    - internal/toolexec/real.go
    - internal/toolexec/goconst_constants.go
    - go.mod
    - go.sum

key-decisions:
  - "DDG anomaly wall (202 bot-challenge on every egress from this environment) blocks whole-page capture; parse fixture = DDG-structure contract (corroborated across 4 independent DDG scrapers) with REAL result data; the REAL anomaly page is committed as the degradation fixture; regeneration command documented in testdata/README.md"
  - "Fetch result shape {"content": markdown} for HTML; raw bounded (512 KiB) passthrough otherwise — no prior shape was pinned by catalog schema or v1.0 tests"
  - "defaultWebBackend package var = injectable fallback seam; empty config value + empty selectBackend name resolve to 'ddg'; explicit http/firecrawl overrides untouched"

patterns-established:
  - "exact-host uddg redirect unwrap (duckduckgo.com / .duckduckgo.com suffix — never substring), non-http(s) unwrapped targets dropped"
  - "SSRF guard before any request: scheme + IP-literal/DNS-resolved public check with injectable resolver"

requirements-completed: [CMD-07]

# Metrics
duration: 50min
completed: 2026-08-14
---

# Phase 8 Plan 02: Real web-tool backends Summary

**Zero-config WebSearch (DDG-HTML scrape, no key) + WebFetch (html→markdown) on the existing swappable seam, with pre-request SSRF guards and offline fixture-pinned tests**

## Performance

- **Duration:** ~50 min
- **Started:** 2026-08-14T18:45Z
- **Completed:** 2026-08-14T19:35Z
- **Tasks:** 3
- **Files modified:** 11

## Accomplishments
- `DefaultBackend` ("ddg"): Search scrapes `https://html.duckduckgo.com/html/?q=<encoded>` with a browser UA, parses `result__a` anchors + `result__snippet` siblings via `x/net/html`, unwraps `/l/?uddg=` redirects (exact-host match), returns the pinned `[{title,url,snippet}]` shape
- Fetch: http/https-only scheme guard + `requirePublicHost` (loopback/private/link-local/metadata refused BEFORE any request, injectable DNS seam); HTML converts via `JohannesKaufmann/html-to-markdown` v1.6.0 into `{"content": markdown}`; other types pass through raw capped at 512 KiB; reads bounded at 2 MiB
- `selectBackend("")`/`("ddg")` → DefaultBackend; RealExecutor falls back to it when `Backends` has no entry — the acp_serve zero-config construction starts working with no wiring change; configured backends keep winning; unknown names still `ConfigError` (T-04-03b)
- Anomaly/bot-challenge pages (real captured response) degrade benignly to an empty result array (T-8-08)

## Task Commits

1. **Task 1: deps pinned** — realized inside the T2/T3 commits (tidy prunes an unimported require; pinned the moment ddg.go imports landed): html-to-markdown v1.6.0, x/net v0.58.0
2. **Task 2: DefaultBackend RED→GREEN** - `0a31381` (test: fixtures + failing tests) → `1be8e89` (feat: ddg.go)
3. **Task 3: selection + fallback RED→GREEN** - `c53612a` (test) → `95690e8` (feat)

**Plan metadata:** (this commit)

## Files Created/Modified
- `internal/toolexec/ddg.go` - DefaultBackend (search parse, redirect unwrap, SSRF guard, markdown conversion, seams)
- `internal/toolexec/backend.go` - selectBackend ddg/empty case
- `internal/toolexec/real.go` - zero-config fallback via defaultWebBackend
- `internal/toolexec/testdata/` - fixtures + regeneration README
- `go.mod`/`go.sum` - two STACK-vetted deps pinned

## Decisions Made
- Fixture strategy under DDG's anomaly wall (see key-decisions): structure-contract fixture with real data + real anomaly fixture + documented regeneration on clean egress
- Fetch HTML result shape `{"content": ...}` (nothing previously pinned; self-describing for the model)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Real DDG capture impossible from this environment**
- **Found during:** Task 2 (fixture capture)
- **Issue:** DDG serves a 202 anomaly bot-challenge to every available egress (direct curl AND scrape proxy) — a real RESULTS page cannot be captured here
- **Fix:** parse fixture follows DDG's real markup contract (corroborated across four independent DDG-HTML scrapers: oh-my-pi, odysseus, openclaw, pi-web-access) populated with real live-search result data; the REAL anomaly response is committed as `ddg-anomaly.html` pinning benign degradation; regeneration command recorded in `testdata/README.md`
- **Files modified:** internal/toolexec/testdata/*
- **Verification:** TestDDGSearchParsesFixture + TestDDGSearchAnomalyDegradesToEmpty green offline
- **Committed in:** 0a31381

**2. [Rule 3 - Blocking] `go mod tidy` pruned the dep before any import existed**
- **Found during:** Task 1
- **Issue:** tidy removes unimported requires, so the T1 pin could not survive its own commit
- **Fix:** dep pinning realized in the T2/T3 commits where the imports land (documented above)
- **Committed in:** 1be8e89

---

**Total deviations:** 2 auto-fixed (2 blocking)
**Impact on plan:** No scope creep; fixture provenance documented honestly (structure contract + real data + real anomaly capture).

## Issues Encountered
- `TestStability_WithinSessionExtractionSource` (internal/profile) failed once in the full-suite run but passes standalone. Environmental, pre-existing: it reads the live `~/.zcode/cli/rollout` store and picks the "richest main session", which shifts while concurrent zcode sessions (this execution + a parallel planning session) are writing. internal/profile has zero dependency on the packages this plan touched. Matches the already-dispositioned deferred item (pinned session eea3dc48 → Phase 9 AUD-05).

## TDD Gate Compliance

RED commits: `0a31381` (build-fail on missing DefaultBackend), `c53612a` (missing defaultWebBackend/selection). GREEN commits: `1be8e89`, `95690e8`. Full package `-race` green after each.

## Next Phase Readiness
- Zero-config web tools work end-to-end in tests; 08-06's E2E can rely on WebSearch/WebFetch without config
- `go test ./internal/toolexec/ -race` + lint clean; `CGO_ENABLED=0 go build ./...` green (both deps pure Go — static binary intact)

## Self-Check: PASSED

- FOUND: internal/toolexec/ddg.go (Name() "ddg", uddg unwrap, SSRF guard)
- FOUND: internal/toolexec/testdata/ddg-results.html + ddg-anomaly.html
- Commits 0a31381/1be8e89/c53612a/95690e8 present on master

---
*Phase: 08-slash-command-kickoff*
*Completed: 2026-08-14*
