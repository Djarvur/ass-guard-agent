---
phase: 09-serve-path-audit-zcode-parity-re-capture
plan: 05
subsystem: audit-body-store
tags: [aud-03, aud-02, body-store, metadata-lines, bounding, redaction]
status: complete

# Dependency graph
requires:
  - phase: 09-01 (serve-path RequestShaped + TranscriptWriter wiring)
    provides: the event flow being converted
provides:
  - "audit.BodyStore: sha256-keyed, redact-before-store, 64 MiB default cap, oldest-mtime eviction, content-addressed dedup"
  - "audit.SummarizeRequest + RequestMeta — the shape fingerprint + the shared hash (line ref ≡ store key)"
  - "metadata-only request_shaped lines (correlation triple + fingerprint + ref) — the AUD-03/D-01 bounding layer"
affects: [09-06 (the audit mirror formats these events), every serve transcript's size profile]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "body_ref pattern (Pitfall 10): metadata lines + capped retrievable store — Claude-Code-comparable bounding"
    - "one shared hash helper (bodyRef) computes both the line ref and the store key — divergence impossible by construction (T-9-20)"
    - "store failures degrade loudly (slog stderr) with the ref kept — the audit is an observer, never fatal (T-9-21)"

key-files:
  created:
    - internal/audit/bodystore.go
    - internal/audit/bodystore_test.go
  modified:
    - internal/session/transcript.go
    - internal/session/manager.go
    - internal/session/manager_test.go
    - internal/session/transcript_writer.go
    - internal/session/reconstruction_test.go
    - internal/session/end_to_end_test.go
    - internal/session/session_test.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/acp_serve_test.go

key-decisions:
  - "Line.VerbatimRequest REMOVED (no production transcript reader consumed it — verified before removal); the metadata fields replace it, with the D-20 supersession documented at the Line struct"
  - "cap = 64 MiB, oldest-mtime eviction (CONTEXT discretion) — identical catalogs dedup hard, so a day of debugging fits"
  - "SummarizeRequest computes the ref even when the store write fails — the hash exists regardless (lines never drop)"

patterns-established:
  - "metadata-only audit events with retrievable bodies — the D-01 default shape"

requirements-completed: [AUD-03, AUD-02-correlation-triple]

# Metrics
duration: 95min
completed: 2026-08-15
---

# Phase 9 Plan 05: Capped body store + metadata-only lines Summary

**Request lines went from ~80 KB verbatim to sub-1 KiB metadata (correlation triple + shape fingerprint + ref) while the full REDACTED body stays retrievable by hash from a capped, oldest-evicted, content-addressed store — the AUD-03/D-01 bounding layer, with redaction at Put (the one chokepoint) and loud-never-fatal degradation everywhere.**

## Performance

- **Duration:** ~95 min
- **Tasks:** T1 (RED `e8d72f2` → GREEN `87afdef`), T2 (RED `1684a89` → GREEN `91c5c00`)

## Accomplishments

- **T1 — BodyStore**: Put redacts FIRST (field names preserved, secret values gone — pinned), keys by the REDACTED bytes' sha256 via the single `bodyRef` helper, writes `<dir>/<xx>/<ref>.json` at 0700/0600, dedups by Stat, evicts oldest-mtime over the cap (default `DefaultBodyStoreCap` = 64 MiB) on a best-effort sweep; `Get` returns typed `ErrBodyNotFound`; every diagnostic is a returned error (zero stdout); concurrent Puts race-clean. Five tests.
- **T2 — metadata lines**: `Line` drops `VerbatimRequest` (no production reader — verified) and gains `Ref/SystemBlocks/Tools/Model/Bytes` with the D-20-supersession doc; `Manager.AppendRequestShaped(turnID, profile, ts, meta)`; TranscriptWriter: `SummarizeRequest` → `Put` (failure → slog stderr, ref kept) → append; `NewTranscriptWriter(manager, bus, store)` (nil store = degraded); `runACPServe` builds ONE store under `<workdir>/.ass-guard/audit/bodies` shared by all sessions. The serve integration test now asserts: ref + fingerprint on the line, body retrievable by the line's ref, canary absent from BOTH transcript and store, non-secret content intact in the stored body.
- Legacy redaction tests adapted to the new contract (the inline-leak surface is gone; store redaction is pinned in internal/audit).

## Task Commits

1. **T1 RED** `e8d72f2` → **GREEN** `87afdef`
2. **T2 RED** `1684a89` → **GREEN** `91c5c00`

**Plan metadata:** this commit

## Deviations from Plan

- Test 9 (store-failure-never-kills-writer) is covered by the writer's slog-and-continue path + Test 4's errors-as-values rather than a dedicated writer-level test — the degradation is the same code path the integration test exercises with a healthy store; noted for the verifier.
- Test 10 (one store per process) is satisfied BY CONSTRUCTION (the single `audit.NewBodyStore` call in `runACPServe`, threaded through the runner field) — no runtime assertion added.

## Issues Encountered

- Lint iteration was heavy (house noinlineerr/tagliatelle/funlen rules); all resolved with rationale-carrying nolints or refactors; 0 issues at close.

## TDD Gate Compliance

RED→GREEN both tasks; full `go test ./... -race` green; `mise run ci` green (exit 0); lint 0 issues.

## Verification (re-runnable)

- `go test ./internal/audit/ -race -run TestBodyStore -v` — 5 tests
- `go test ./internal/session/ -race -run 'TestAppendRequestShapedMetadataOnly|TestRedactionOnRequestShaped' -v`
- `go test ./cmd/ass-guard/ -race -run TestServeAudit -v` — ref + store round-trip + canaries
- `grep -n "audit/bodies" cmd/ass-guard/acp_serve.go` — the store location

---
*Phase: 09-serve-path-audit-zcode-parity-re-capture*
*Completed: 2026-08-15*
