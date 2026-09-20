---
phase: 18-session-family
plan: 03
subsystem: acp-sessions
tags: [acp, session-list, enumeration, cursor-pagination, tombstone, header-scan, filesystem, tdd]

# Dependency graph
requires:
  - "internal/session transcript family (transcript_<uuid>.jsonl under .ass-guard/, Line envelope, openTranscript naming) — the scan surface"
  - "18-02's reader discipline vocabulary (non-conforming lines skipped) and seed.go's isTurnSuffix — reused verbatim by the checkpoint ref probe"
  - "internal/coreexec/messaging.go sessIDPattern + SessionReader enumeration — the mirrored patterns (package-local copy per T-12-04-03)"
provides:
  - "session.ListSessions(dir, cursor, limit) ([]SessionHeader, string, error) — the ONE enumeration engine ACP-05's session/list (18-04), D-11's picker, and D-10's --continue all consume"
  - "session.SessionHeader — lean 6-field row (SessionID, Title, TitlePresent, CreatedAt, LastActivity, HasCheckpoints) with the TitlePresent flag downstream wire mapping branches on"
  - "session.ErrInvalidCursor — the typed malformed-cursor rejection 18-04 maps to a JSON-RPC invalid-params error"
  - "tombstoneSuffix ('.deleted') store-layout consts — 18-04's tombstone.go write side shares the spelling"
affects: [18-04-session-family-rpc, 18-06-resume-cli, ACP-05]

# Actuals (#2632) — chars/4 over the realized diff (33,458 diff chars), same scale as the plan's estimate
actuals:
  tokens: 8364
  tasks: 2
  commits: 2

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "io.LimitReader(64 KiB) UNDER the bufio reader — pins the TOTAL bytes ever read per transcript regardless of buffer fills; the bounded prefix is a hard byte ceiling, not a per-line one"
    - "Composite cursor comparison encoded exactly once (afterListCursor) and reused by the sort order — pagination and ordering cannot drift apart (D-05)"
    - "DirEntry.Type().IsRegular() from the ReadDir entry itself (lstat-shaped, zero extra syscalls) rejects planted symlinks/fifos before any open (T-18-07)"
    - "Contract proofs via chmod-000: an unreadable transcript proves stat-before-open ordering because open failures on structurally valid transcripts PROPAGATE (SessionReader.read precedent)"

key-files:
  created:
    - internal/session/list.go
    - internal/session/list_test.go
  modified: []

key-decisions:
  - "Tombstone spelling locked as <sessionID>.deleted (a zero-byte SIBLING named id+'.deleted', not transcript_<id>.jsonl.deleted): 18-04-PLAN's SweepTombstones enumerates markers by '.deleted suffix, id validates' — the sibling spelling is the one whose marker-name trim yields the id directly; tombstoneSuffix is the shared const"
  - "Open failures on structurally valid transcripts PROPAGATE as errors (not silent skips) — the messaging.go SessionReader.read precedent; this makes the plan's chmod-000 instruments airtight (tombstone filter proves stat-before-open; malformed cursor proves decode-before-scan) at the cost of one honest failure mode for genuinely unreadable store files"
  - "HasCheckpoints probes the checkpoint store's loose-ref layout (one ReadDir of .ass-guard/checkpoints/shadow.git/refs/checkpoints/, owner = name[:LastIndex('-turn-')] with isTurnSuffix-validated tail — seed.go's parser reused, not duplicated); deliberately NOT checkpoint.Open, which would CREATE the store — a list scan must never write. Validated against the real store API in TestSessionListHeaderShape"
  - "The pagination test asserts strictly-after-cursor BEHAVIORALLY (pages partition the single-page full order exactly, no row re-emitted, stale-cursor page stable) rather than decoding cursors in the test — the contract is the observable order, not the codec internals"
  - "Missing store dir (os.ErrNotExist) returns the empty page, not an error; every other ReadDir failure wraps and returns — empty-input edge honored without masking real I/O problems"
  - "Title = first text block of the user_message Content blocks (the AppendUserMessage shape) with the flat Line.Text as fallback, truncated to 80 runes via []rune; TitlePresent=false exactly when no prompt text was found in the bounded prefix — including when a user_message exists BEYOND the 64 KiB cap (pinned by the 70 KiB-filler fixture)"

patterns-established:
  - "Pattern: fixed-width-hex UUID tails in list fixtures (00000000-...-%012x) make lexicographic id order equal numeric order — tie-break assertions stay readable"
  - "Pattern: total-order pagination tested by full-order partition — fetch everything in one page as the reference, then prove pages concatenate to exactly that sequence"

requirements-completed: [ACP-05]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "Enumeration engine: stable total order (lastActivity desc, sessionId asc tie-break; equal-mtime pair appears exactly once each, ascending id, identical across identical calls), composite-cursor pagination (three pages partition the full order exactly once, final nextCursor empty, stale cursor pointing at a since-deleted session returns the same consistent page), stat-only tombstone filter (tombstoned session absent with zero transcript bytes read — chmod-000 proof), empty/missing/single stores (nil result + empty cursor; empty-string cursor is first page), corrupt-first-line skip without error, bounds (limit 0 -> 50 + cursor, limit 500 clamped to 100, exhaustion 100+20 then empty cursor)"
    requirement: ACP-05
    verification:
      - kind: unit
        ref: "internal/session/list_test.go#TestSessionListOrderingAndTiebreak"
        status: pass
      - kind: unit
        ref: "internal/session/list_test.go#TestSessionListCursorPagination"
        status: pass
      - kind: unit
        ref: "internal/session/list_test.go#TestSessionListTombstoneFilter"
        status: pass
      - kind: unit
        ref: "internal/session/list_test.go#TestSessionListEmptyAndSingle"
        status: pass
      - kind: unit
        ref: "internal/session/list_test.go#TestSessionListCorruptFirstLine"
        status: pass
      - kind: unit
        ref: "internal/session/list_test.go#TestSessionListBounds"
        status: pass
    human_judgment: false
  - id: D2
    description: "Lean header shape: title is the first user_message text truncated to 80 runes (unicode-safe, 200-rune multi-byte fixture) with TitlePresent true; no user_message in the bounded prefix (or one beyond the 64 KiB cap — 70 KiB filler fixture) yields the '(no prompt)' fallback with TitlePresent false; createdAt equals the session_start line's Timestamp; LastActivity equals mtime; HasCheckpoints true only via a real checkpoint-store snapshot (EARLY-01 loose-ref layout validated against checkpoint.Open+Snapshot)"
    requirement: ACP-05
    verification:
      - kind: unit
        ref: "internal/session/list_test.go#TestSessionListHeaderShape"
        status: pass
    human_judgment: false
  - id: D3
    description: "Opaque cursor codec + DoS guards: malformed cursors (non-base64url, no tuple separator, non-numeric nano, invalid id, empty id, >256 bytes) all typed-rejected with ErrInvalidCursor BEFORE any scan (proven by the unreadable-transcript store surfacing the cursor error, never EACCES); a well-formed cursor naming a nonexistent session is comparator input, never an error; codec unexported; limit clamped to 100"
    requirement: ACP-05
    verification:
      - kind: unit
        ref: "internal/session/list_test.go#TestSessionListMalformedCursor"
        status: pass
      - kind: other
        ref: "mise ci (vet + golangci-lint 0 issues + CGO_ENABLED=0 build + go test -race ./...)"
        status: pass
    human_judgment: false

# Metrics
duration: 19min
completed: 2026-09-03
status: complete
---

# Phase 18 Plan 03: Session List Engine Summary

**On-demand header-scan enumeration over .ass-guard/ transcripts — first-line + 64 KiB-bounded-prefix reads only, (lastActivity desc, id asc) stable total order with a composite opaque cursor, stat-only tombstone filter, lean 6-field headers, and a real-store checkpoint probe — the one engine session/list, the picker, and --continue all consume**

## Performance

- **Duration:** 19 min (12:11Z → 12:30Z)
- **Tasks:** 2/2
- **Files:** 2 (2 created)
- **Commits:** 2 (+ this metadata commit)

## Accomplishments
- `internal/session/list.go`: `ListSessions(dir, cursor, limit)` — ReadDir(.ass-guard) + per-entry classification (transcript-named, sessIDPattern-valid, regular DirEntry type, no `<id>.deleted` sibling by os.Stat) + first-line createdAt + bounded-prefix title read, mtime from the DirEntry info, one ReadDir checkpoint-ref probe shared across the scan; sort (lastActivity desc, id asc), cursor filter, page window, nextCursor when rows remain. No index file, nothing to drift (D-04); best-effort paging shift documented in the file header (D-05)
- `SessionHeader` exactly as the must-have truth lists — six lean fields, no token/cost (D-06); `TitlePresent` carries real-prompt-vs-fallback so 18-04/18-06 branch on the flag, never on string comparison
- Opaque cursor codec (`encodeCursor`/`decodeCursor`, unexported): base64url "unixNano|sessionId", 256-byte cap + full shape validation before ANY scan, typed `ErrInvalidCursor` rejection (T-18-06); the strictly-after comparison lives once in `afterListCursor`
- `internal/session/list_test.go`: the eight contract functions, all parallel, all green under -race — including the chmod-000 instruments (tombstone stat-before-open; malformed-cursor decode-before-scan) and the real-checkpoint-store HasCheckpoints validation

## Task Commits

Each task committed atomically (TDD RED→GREEN):

1. **Task 1 (RED): list engine contract tests**
   - `6a27601` test(18-03): add failing session list engine tests (compile-failure RED confirmed via the plan's verify command — undefined ListSessions/SessionHeader/ErrInvalidCursor)
2. **Task 2 (GREEN): ListSessions engine**
   - `f8a8539` feat(18-03): implement session list engine (lint-clean in the same commit — scan helpers, parallel tests, no-inline-err, the repo-precedent nonamedreturns nolint for the two multi-result helpers; no separate REFACTOR needed: zero duplication emerged, the turn-suffix parsing REUSES seed.go's isTurnSuffix as the plan directed)

## Files Created/Modified
- `internal/session/list.go` — ListSessions, SessionHeader, ErrInvalidCursor, listCursor + unexported codec, scanStoreHeaders/scanStoreEntry, readTranscriptHeader/readHeaderOpener/readHeaderTitle/titleOfLine, checkpointOwners (loose-ref probe), filterAfterCursor/afterListCursor, package-local sessIDPattern copy (T-12-04-03 provenance comment), store-layout consts (tombstoneSuffix shared with 18-04)
- `internal/session/list_test.go` — the eight TestSessionList* functions + fixture helpers (real Line envelope transcripts, Chtimes-forced mtimes, fixed-width-hex UUID ids, real checkpoint store)

## Decisions Made
- **Tombstone spelling (18-RESEARCH A4 resolved):** `<sessionID>.deleted` — the sibling whose marker-name trim yields the validating id directly, matching 18-04's SweepTombstones contract ("suffix .deleted, id validates"); the spelling lives in `tombstoneSuffix` where 18-04's tombstone.go will reuse it
- **Open-failure propagation:** a transcript that passes every structural check but cannot be opened errors the call (SessionReader.read precedent) rather than vanishing silently — this is what makes the plan's chmod-000 proofs meaningful, and it fails loud (never masks fs anomalies) at the cost of an honest failure mode for out-of-contract stores
- **Checkpoint probe:** loose-ref layout ReadDir (validated against the REAL store in the test) instead of checkpoint.Open — Open creates the store; a list scan must never write
- **Strict 64 KiB cap:** io.LimitReader under bufio pins TOTAL bytes read per transcript (buffer fills included), honoring "<= 64 KiB" at the byte level
- **REQUIREMENTS.md not flipped:** ACP-05 is also declared by 18-04 (no SUMMARY yet) — the shared-ID gate (#2388) correctly holds it; `requirements.ready-ids` returned 0/1 ready

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] lint gate vs the artifact's literal signature spelling**
- **Found during:** Task 2 GREEN (mise lint)
- **Issue:** gocritic paramTypeCombine rejects the artifact spec's literal `func ListSessions(dir string, cursor string, limit int)` spelling; lint also demanded restructuring (scan helpers, parallel tests, no-inline-err, named-return conflicts between gocritic unnamedResult and nonamedreturns)
- **Fix:** signature compacted to `func ListSessions(dir, cursor string, limit int)` — the identical type; the scan split into scanStoreHeaders/scanStoreEntry/readHeaderOpener/readHeaderTitle helpers; the two multi-result helpers carry the repo-precedent `//nolint:nonamedreturns` (ecosys/expand.go pattern)
- **Files modified:** internal/session/list.go, internal/session/list_test.go
- **Verification:** `mise lint` — 0 issues; full suite green
- **Committed in:** f8a8539

---

**Total deviations:** 1 auto-fixed (blocking/lint)
**Impact on plan:** None on behavior — signature is semantically identical; all eight contract tests and every acceptance criterion pass.

## Verification Results

- RED (Task 1): `go test -race -count=1 ./internal/session/ -run TestSessionList` → build failure on undefined engine symbols — RED-CONFIRMED via the plan's verify command
- GREEN (Task 2): `go test -race -count=1 ./internal/session/ && go vet ./internal/session/` — **ok / clean**
- `-run TestSessionList -v` — **8/8 PASS** (0.01–0.69s each, checkpoint-backed shape test the slowest at 0.69s)
- `gofmt -l internal/session/` — clean; `mise lint` — **0 issues**
- `mise ci` — **green** (vet, lint 0 issues, CGO_ENABLED=0 build, full `-race` suite ~88s)
- Acceptance greps: `func ListSessions(` present; `SessionHeader` has exactly the six lean fields; **no ReadAll/ReadFile/ioutil** in the scan path; the only reader is `bufio.NewReaderSize(io.LimitReader(f, 64 KiB), …)`; `encodeCursor`/`decodeCursor` unexported; malformed cursor → typed error (test-pinned)
- One non-reproducing failure: a single full-suite run failed `TestGateOutcomeMatrix/allow_once_executes_without_persisting` (gate_test.go — untouched by this plan); the same test passed alone, the full package then passed FIVE consecutive times (three plain, one paired with the list battery, one full `mise ci`) — the same environmental under-load flake family 18-02 documented, not code-related

## TDD Gate Compliance

RED (`6a27601` test) precedes GREEN (`f8a8539` feat) in git log — gate sequence compliant; RED failed for the right reason (undefined engine symbols, not a broken test).

## Known Stubs

None — no stubs, no skipped tests, every `<verify>` command ran.

## Threat Surface

No security-relevant surface beyond the plan's threat model. All mitigated dispositions verified:
- **T-18-06** (DoS): limit clamped to 100; cursor length-capped at 256 bytes BEFORE any parsing; malformed input is a typed reject that performs zero file reads (chmod-000 proof)
- **T-18-07** (tampering/planted entries): DirEntry type check rejects symlinks/dirs/fifos structurally; sessIDPattern-valid ids only (traversal-safe); non-conforming openers/lines skipped
- **T-18-08** (info disclosure): title derives only from the first user_message inside the 64 KiB prefix, truncated to 80 runes — later prompts, assistant output, and tool results are never header material (the beyond-prefix fixture pins it)
- **T-18-SC**: no new packages (stdlib only)

## Self-Check: PASSED

- Created files exist: internal/session/list.go, internal/session/list_test.go — FOUND
- Commits exist: 6a27601, f8a8539 — FOUND (git log)
- Acceptance criteria re-run post-commit: tests/vet/lint/ci green (see Verification Results)

## Next Phase Readiness

- The engine is ready for 18-04's `session/list` handler: call `ListSessions(workDir, request cursor, page size)`, map `updatedAt = LastActivity`, and derive the wire title field from `TitlePresent` (never from comparison against the fallback literal); `ErrInvalidCursor` maps to invalid-params
- The store-layout consts (`tombstoneSuffix`, `storeDirName`) are the shared spellings 18-04's tombstone.go write side should reuse
- BLOCKER: none

---
*Phase: 18-session-family*
*Completed: 2026-09-03*
