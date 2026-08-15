---
phase: 09-serve-path-audit-zcode-parity-re-capture
plan: 01
subsystem: factory-capturer-seam
tags: [audit, log-01, capturer, factory-seam, transcript-writer, serve-path, redaction]
status: complete

# Dependency graph
requires:
  - phase: v1.0 Phase 7 (factory wiring) + Phase 2 (D-03/D-20 transcript)
    provides: ProviderFactory + Endpoint accessor; TranscriptWriter (zero non-test callers until now)
provides:
  - "(*ProviderFactory).BuildWithCapturer — the ONE construction seam attaching RequestCapturer for BOTH shapes (anthropic Stream + openai Send)"
  - "Session.CurrentTurnID() — non-incrementing turn attribution for audit events"
  - "serve-path RequestShaped events landing in per-session transcripts via a per-session TranscriptWriter (serve-ctx lifetime, OnClose reaping)"
affects: [09-05 (body store consumes the events), 09-06 (audit mirror), Phase 10 runtime extraction (inherits ONE wiring site)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "single-sourced capturable construction: Build delegates to BuildWithCapturer(nil) — nil capturer is a no-op inside the adapters"
    - "late-bound closure for turn attribution: the capturer reads CurrentTurnID() at FIRE time via a sess pointer assigned after the Session literal"
    - "subscription-readiness barrier in tests: re-send a harmless event until it lands (the bus never replays; a single early publish can drop in the no-subscriber window)"

key-files:
  created: []
  modified:
    - internal/scheduler/factory.go
    - internal/scheduler/factory_test.go
    - internal/session/session.go
    - internal/session/session_test.go
    - cmd/ass-guard/main.go
    - cmd/ass-guard/provider_factory.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/acp_serve_test.go
    - cmd/ass-guard/acp_engine_e2e_test.go
    - cmd/ass-guard/integration_test.go
    - cmd/ass-guard/mcp_tracer_test.go
    - cmd/ass-guard/e2e_opsx_test.go
    - cmd/ass-guard/goconst_constants.go
    - internal/scheduler/goconst_constants.go

key-decisions:
  - "CurrentTurnID implemented (not the empty-TurnID fallback) per the CONTEXT discretion call — AUD-02 correlation from day one"
  - "BuildWithCapturer lives on scheduler.ProviderFactory (not a cmd/ helper) — the factory owns construction (Phase-7 D-08)"
  - "the E2E harness's 08-07-era startTranscriptWriter helper was REMOVED — superseded by the production wiring (two writers duplicate lines)"
  - "writer subscription is asynchronous (goroutine Run); the serve path's ACP round-trip makes the startup window a non-issue in practice, and drops are logged loudly — tests use a re-send barrier"

patterns-established:
  - "Pitfall-8 grep guard: the divergent construction path's NAME is purged from comments too (acceptance: grep returns nothing)"

requirements-completed: [AUD-01, AUD-02-transcript-leg]

# Metrics
duration: 85min
completed: 2026-08-15
---

# Phase 9 Plan 01: Factory capturer seam Summary

**The v1.0 LOG-01 serve gap is closed permanently: a real serve-path turn now leaves a redacted `request_shaped` line with real turn attribution in its transcript, produced through ONE factory-seam capturer shared by the tracer and the serve path — proven by an in-process integration test against the REAL factory (the Pitfall-8 closer).**

## Performance

- **Duration:** ~85 min (T1 + T2 + T3 + lint battle)
- **Tasks:** T1 complete (RED `41131c5` → GREEN `6e64e4d`), T2 complete (`a48221a` RED → wiring), T3 complete (integration test green under `-race`)

## Accomplishments

- **T1** — `BuildWithCapturer(providerName, sh, capturer)` on `scheduler.ProviderFactory`: anthropic attaches `WithAnthropicRequestCapture` (fires per Stream with body+headers), openai attaches `WithOpenAIRequestCapture` (fires per Send with the marshaled wire body); `Build` delegates with nil; lazy D-07 + undeclared semantics byte-identical. Five tests: both shapes fire exactly once with verbatim bodies, uncredentialed never fires, undeclared errors identically, delegation constructs indistinguishable providers.
- **T2** — `Session.CurrentTurnID()` (non-incrementing; fresh session → ""; next nextTurnID unaffected); `makeProvider` signature takes the capturer; `runACPServe` constructs through `BuildWithCapturer` with `serveCtx`; `sessionFor` publishes `event.RequestShaped{TurnID: sess.CurrentTurnID(), …}` via the late-bound closure (header VALUES dropped, Pitfall 9) and starts ONE TranscriptWriter per session on a serve-ctx-derived context, canceled in the OnClose chain ahead of the MCP reaper; `runTrace` builds through the seam; the divergent `tracerProvider` is DELETED (`grep -rn tracerProvider cmd/ass-guard/` → exit 1, name purged from comments too).
- **T3** — `TestServeAudit_RequestShapedThroughRealSeam`: in-process `runACPServe` over pipes, REAL temp `.ass-guard/scheduling.yaml` (anthropic → httptest SSE stub, synthetic `sk-test-canary-…` key, `ZAI_API_KEY` cleared so the literal wins), `setupProviderFactory` → `BuildWithCapturer` exactly as production. Asserts: `request_shaped` line in `<workdir>/.ass-guard/transcript_<sid>.jsonl` with non-empty TurnID + profile `zcode`; canary value absent from the WHOLE transcript (redaction chokepoint); stdout lines all valid JSON-RPC frames (transport discipline).
- Wiring tests: capturer publishes with the in-flight turn id (`sess-cap-turn-001`); writer one-per-session (readiness barrier + exactly-N-lines assertion, `-race -count=10` stable), Close reaps (post-close publish lands nothing).

## Task Commits

1. **T1 RED** `41131c5` → **GREEN** `6e64e4d`
2. **T2** `a48221a` (CurrentTurnID RED) → `e9d8880` (wiring + deletion + tests)
3. **T3** `e9d8880` + comment purge `4eff6ae`

**Plan metadata:** this commit

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2] E2E harness writer helper removed**
- **Issue:** 08-07 added a harness-only `startTranscriptWriter` (evidence wiring); this plan's production wiring makes it a DUPLICATE writer (Publish fan-outs to every subscriber → duplicated transcript lines)
- **Fix:** removed the helper + call sites; the production sessionFor wiring now provides the E2E's request_shaped evidence
- **Committed in:** `e9d8880`

**2. [Rule 2] TDD ordering on the wiring leg**
- **Issue:** CurrentTurnID was strict RED→GREEN; the T2 wiring tests (Test 7/8) and the T3 integration test were written against the freshly-landed wiring (co-developed), not pre-committed RED — the wiring itself has no pre-existing failing surface to pin without the signature change landing first
- **Fix:** behavior assertions are equivalent to the plan's tests; the RED evidence for this plan is T1 + CurrentTurnID; noted here for the verifier
- **Committed in:** `e9d8880`

## Issues Encountered

- Race in the first T3 draft (polling a live `bytes.Buffer`) → `syncBuffer`; writer-subscription startup race in Test 8 → re-send readiness barrier (both documented in patterns-established)

## TDD Gate Compliance

RED→GREEN: T1 (`41131c5`→`6e64e4d`), CurrentTurnID (`a48221a`→wiring). Full battery: `go test ./internal/scheduler/ ./internal/session/ ./cmd/ass-guard/ -race -count=1` green; `mise run ci` green (exit 0); lint 0 issues.

## Verification (re-runnable)

- `go test ./internal/scheduler/ -race -run TestBuildWithCapturer -v` — 5 tests
- `go test ./internal/session/ -race -run TestCurrentTurnID -v`
- `go test ./cmd/ass-guard/ -race -run 'TestServeCapturer|TestServeTranscriptWriter|TestServeAudit' -v`
- `grep -rn "tracerProvider" cmd/ass-guard/` → exit 1
- `grep -n "BuildWithCapturer" cmd/ass-guard/main.go cmd/ass-guard/acp_serve.go` → both call sites
- `grep -n "NewTranscriptWriter" cmd/ass-guard/acp_serve.go` → the wired writer

---
*Phase: 09-serve-path-audit-zcode-parity-re-capture*
*Completed: 2026-08-15*
