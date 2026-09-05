---
title: Per-provider HTTP(S) proxy support
resolves_phase:
created: 2026-09-06
source: operator request (verify-work session, 2026-09-06)
priority: normal
---

# Per-provider HTTP(S) proxy support

## What

The provider description (alongside base URL, API key, etc.) may specify a
**proxy** through which ALL requests to that provider must be sent.

## Requirements (operator's words, expanded)

- Config surface: a `proxy` field per provider entry (`.ass-guard/config.yaml`
  providers block + the global layer), e.g.
  `proxy: "http://user:pass@proxy.example.com:3128"`.
- Proxy schemes supported: `http://` and `https://`.
- Proxy authentication: **basic** for starters (`user:pass`); design the field
  so additional auth kinds (token headers, etc.) can be added later without a
  config-format break.
- The proxy applies to every request the provider's client makes (chat,
  streaming, any aux calls) — no per-call bypass.

## Implementation sketch (for whoever picks it up)

- `internal/providerfactory` is the construction site: when a provider entry
  carries `proxy`, build the provider's `http.Client` with a custom
  `http.Transport` whose `Proxy` is the parsed proxy URL (Go's `net/http`
  honors userinfo in the proxy URL as basic auth automatically; https-scheme
  proxies need the TLS dial path considered).
- Both provider SDKs in use accept an injected `http.Client`
  (`anthropics/anthropic-sdk-go`, `sashabaranov/go-openai`) — no SDK forks.
- Validate the proxy URL at config load (typed error class, same family as
  the existing LayerReadError-style config errors); invalid proxy = loud
  load-time failure, never a silent direct connection.
- Tests: unit (transport wiring via httptest through a local proxy stub),
  config round-trip (write/read preserves the field), and a RED-first pin
  that a proxied provider NEVER dials the target directly.

## Non-goals (first cut)

- SOCKS proxies, auth beyond basic, per-model (vs per-provider) proxies,
  proxy PAC/auto-discovery.
