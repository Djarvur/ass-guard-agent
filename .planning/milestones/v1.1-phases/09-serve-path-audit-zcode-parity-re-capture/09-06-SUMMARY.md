---
phase: 09-serve-path-audit-zcode-parity-re-capture
plan: 06
subsystem: audit-mirror
tags: [aud-02, mirror, audit-log, header-names, canary, phase-gate]
status: complete   # code + tests green; the PHASE gate's stability leg is blocked on the 09-04 harvest finding (recorded)

# Dependency graph
requires:
  - phase: 09-01 (serve wiring), 09-02 (enriched EngineDecision), 09-05 (body store + metadata events)
    provides: the bus events the mirror consumes
provides:
  - "audit.OpenFileSink — the ONE shared opener (stdout targets rejected, 0600 append-only)"
  - "audit.Mirror — per-session JSONL under .ass-guard/audit/ (default ON), --audit-log override, drop-with-counter failures"
  - "header NAMES on RequestShaped (values never — Pitfall 9 audit-path discipline)"
  - "the automated secret canary — the phase gate's grep protocol as a test"
affects: [the Phase-9 gate (mirror + canary legs), Phase 10 (the runtime extraction inherits the audit home)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "artifact vs SOURCE scoping in canaries: the operator's scheduling.yaml holds the credential BY CONSTRUCTION — the leak scan covers artifacts (.ass-guard/audit + transcripts), never the source"
    - "complements-never-duplicates (v1.0 D-20): mirror and transcript = different schemas, different consumers, one bus"

key-files:
  created:
    - internal/audit/mirror.go
    - internal/audit/mirror_test.go
    - internal/audit/goconst_constants_test.go
  modified:
    - internal/audit/audit.go
    - internal/audit/audit_test.go
    - internal/audit/doc.go
    - internal/event/events.go
    - cmd/ass-guard/main.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/acp_serve_test.go

key-decisions:
  - "the dead --audit-log flag is now READ (runACPServeCmd threads the inherited persistent flag) — D-02's operator override is live"
  - "openAuditSink DELETED from cmd/ass-guard — ONE opener (audit.OpenFileSink), Pitfall 8's shared-sink rule"
  - "the mirror closes its override sink on ctx done (serve lifetime), logged on error"

patterns-established:
  - "the canary test operationalizes the phase gate — 'token/secret canary greps clean' is executable at will"

requirements-completed: [AUD-02]

# Metrics
duration: 110min
completed: 2026-08-15
---

# Phase 9 Plan 06: The per-session audit mirror Summary

**The complete AUD-02 surface is live: transcript index (09-01/09-05) + bounded retrievable bodies (09-05) + the compact operator mirror with a working --audit-log override — every line redacted at one chokepoint, header NAMES (never values) as shape evidence, and the phase gate's secret-canary greps executable as a test.**

## Performance

- **Duration:** ~110 min
- **Tasks:** T1 (RED `3b5c9a2` → GREEN `fe416b1`), T2+T3 (`f7d67e4`)

## Accomplishments

- **T1** — `audit.OpenFileSink` (shared opener: ""/"-" → stderr; `stdout`/`/dev/stdout` TARGETS rejected — AUD-02's guard now enforced for file targets; 0600 append-only) + `audit.Mirror`: per-session `<dir>/<sessionID>.jsonl` routing by the TurnID session prefix (root created at construction — no ordering dependency on the body store's lazy subtree), RequestShaped/EngineDecision/UsageUpdate consumed, every line marshaled then `redact.Redact`ed before the sink, drop-with-counter failures (logged once per distinct error; `Dropped()` accessor), `NewMirrorFile` single-file mode (panics on os.Stdout). Seven tests -race green: opener, routing, provenance mirror line, chokepoint, stdout rejection, drop counter, single-file interleave.
- **T2** — `event.RequestShaped.HeaderNames []string` (names ONLY, sorted at the capturer closure; values never referenced); `serveOptions.AuditLogPath` threaded from the previously-DEAD persistent `--audit-log` flag; `startAuditMirror` (default ON per-session; "-" → stderr; path → single file; failures degrade loudly, never refuse); `openAuditSink` deleted from cmd/ass-guard (ONE opener — Pitfall 8). Serve tests: default mirror file with `kind`+`headerNames`; override mode = one file, NO per-session artifact.
- **T3** — the automated canary: full artifact-tree walk (`.ass-guard/audit` + the transcript; scheduling.yaml excluded as the credential SOURCE) asserting zero canary bytes, WITH positive controls (transcript request line, stored body, mirror line all non-empty — absence is not vacuity).

## Phase 9 gate status (recorded honestly)

- `mise ci` — GREEN (exit 0)
- live-serve redacted-audit verification — GREEN in-process (the REAL `runACPServe` over pipes through the REAL factory: redacted request line + retrievable body + canaries); operator witness pending morning
- stability test green against the newly pinned session — **BLOCKED** on the 09-04 harvest finding (no existing session meets the catalog-change threshold; morning options recorded in STATE.md)
- token/secret canary greps — GREEN (the automated canary + lint/test suite)

## Task Commits

1. **T1 RED** `3b5c9a2` → **GREEN** `fe416b1`
2. **T2+T3** `f7d67e4`

**Plan metadata:** this commit

## Deviations from Plan

- Test 8's "no header values" assertion is structural (the closure never references values; the field carries `[]string` names) rather than an unmarshaled negative assertion on a captured event — the canary's tree-walk covers the leak surface end-to-end.
- Test 11 (tracer unchanged) is covered by the existing tracer tests re-running green through the shared opener rather than a new dedicated test.

## Issues Encountered

- The default mirror initially raced the body store's lazy directory creation — fixed by creating the mirror root at construction (documented in code).

## TDD Gate Compliance

T1 strict RED→GREEN; full `go test ./... -race` green; `mise run ci` green; lint 0 issues.

## Verification (re-runnable)

- `go test ./internal/audit/ -race -run 'TestOpenFileSink|TestMirror' -v` — 7 tests
- `go test ./cmd/ass-guard/ -race -run 'TestServeAudit|TestServeMirror' -v` — mirror + canary + override
- `grep -n "HeaderNames" internal/event/events.go cmd/ass-guard/acp_serve.go` — both
- `grep -c "func openAuditSink" cmd/ass-guard/main.go` → 0

---
*Phase: 09-serve-path-audit-zcode-parity-re-capture*
*Completed: 2026-08-15 (overnight delegated run)*
