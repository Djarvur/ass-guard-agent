---
phase: 16-acp-wire-foundation
plan: 02
subsystem: session-transcript
tags: [transcript, session, redaction, schema, tdd]
requires:
  - "internal/session transcript envelope (Line struct + kind-discriminated JSONL, 0600 discipline)"
  - "Manager sole-owner append API + Redactor interface (LOG-03)"
provides:
  - "TypeRawThinking / TypeLocalCommand / TypeCompaction kind constants + Line fields (Args, SourceChain, Expansion, BoundaryID, CacheTokens, PreRef, PostRef)"
  - "AppendRawThinking (unredacted, provider-attributed) / AppendLocalCommand (D-22 invocation record) / AppendCompaction (D-21 rich boundary record)"
  - "appendLineUnredacted — the raw_thinking-ONLY append path the Redactor never sees (D-23)"
  - "uuidV4 (local crypto/rand v4) for boundary ids; counting-Redactor fake as the reusable zero-call proof instrument"
affects:
  - "Phase 19 (compaction — consumes TypeCompaction as its reset-point class; marker carries the full D-21 record)"
  - "Phase 20 (commands — local_command producers; owns the source-chain/expansion vocabularies)"
  - "Phase 21 (PAR-05 — raw_thinking replay with provider attribution; bytes on disk are already byte-faithful)"
tech-stack:
  added: [] # stdlib only (crypto/rand joins encoding/json; no new deps)
  patterns:
    - "deliberate near-copy unredacted path — NO shared helper with the redacted path; the distinct code path IS the D-23 guarantee (Pitfall 5)"
    - "counting-Redactor fake injected via NewManager as the redaction proof instrument (zero-calls + redacted-control positive)"
    - "raw JSONL line injection in tests for unknown-future kinds (no appender exists by design — readers tolerate)"
key-files:
  created:
    - internal/session/transcript_newkinds_test.go
  modified:
    - internal/session/transcript.go
    - internal/session/manager.go
key-decisions:
  - "raw_thinking reuses the existing Line fields Content (payload) + Model (provider attribution) — no redundant new fields; payload stays provider-shaped json.RawMessage per D-20, and the CONTEXT discretion item (carry provider attribution) is answered YES via Model"
  - "compaction pre/post pointers are opaque strings (PreRef/PostRef) — D-21 pointers stay untyped until Phase 19 defines their concrete form (D-20 weak schema); boundary ids via a local uuidV4 (acp/shaper helpers unexported, same crypto/rand construction)"
  - "appendLineUnredacted is a deliberate near-copy of appendLine minus the redact block — no shared marshal/newline helper (Pitfall 5's warning sign); grep-verified sole caller is AppendRawThinking (T-16-04 structural scoping)"
  - "local_command args stored verbatim in a string field (no re-quoting/normalization); resolution-source chain as ordered []string; expansion outcome as free string — vocabularies owned by Phase 20 (CMDS-01/02)"
  - "local-command and compaction metadata route through the REDACTED path — D-23's exemption is type-scoped to thinking bytes only"
requirements-completed: [ACP-03]
duration: 16 min
completed: 2026-08-27
estimate:
  tokens: 26000
  raw_tokens: 26000
  tasks: 2
actuals:
  tokens: 5788 # chars/4 over the realized diff (23,152 chars, 3 files, 596 insertions)
  tasks: 2
  commits: 3
coverage:
  - deliverable: "raw_thinking appends through the dedicated unredacted path; payload (json.RawMessage with inside-string whitespace + existing unicode escapes) round-trips marshal→disk→read byte-identical; provider model attribution carried"
    verification:
      - kind: test
        ref: "tests/internal/session/transcript_newkinds_test.go#TestTranscriptNewKinds/raw_thinking_payload_round-trips_byte-identical"
        status: pass
    human_judgment: false
  - deliverable: "D-23 zero-call proof: the Redactor is invoked ZERO times on the raw_thinking path and at least once on a redacted control kind (AppendUserMessage)"
    verification:
      - kind: test
        ref: "tests/internal/session/transcript_newkinds_test.go#TestTranscriptNewKinds/raw_thinking_never_touches_the_redactor;_redacted_control_does"
        status: pass
    human_judgment: false
  - deliverable: "D-22 local_command full invocation record: command key + verbatim args (mixed quoting/double spaces preserved) + ordered resolution-source chain + expansion outcome"
    verification:
      - kind: test
        ref: "tests/internal/session/transcript_newkinds_test.go#TestTranscriptNewKinds/local_command_records_key,_verbatim_args,_source_chain,_outcome"
        status: pass
    human_judgment: false
  - deliverable: "D-21 compaction rich boundary record: fresh RFC-4122-v4 boundary id per append + token-usage snapshot (input/output/cache) + pre/post transcript pointers"
    verification:
      - kind: test
        ref: "tests/internal/session/transcript_newkinds_test.go#TestTranscriptNewKinds/compaction_records_fresh_boundary_id,_usage_snapshot,_pointers"
        status: pass
    human_judgment: false
  - deliverable: "D-20 replay tolerance: readTranscriptFile parses the three new kinds plus an unknown future kind plus a known kind with an unknown extra field — no error, discriminators intact, no lines dropped"
    verification:
      - kind: test
        ref: "tests/internal/session/transcript_newkinds_test.go#TestTranscriptNewKinds/replay_tolerates_new_kinds,_unknown_kinds,_unknown_fields"
        status: pass
    human_judgment: false
  - deliverable: "Additive inertness end-to-end: the Projector's projected window is deeply equal with and without the new kinds + unknown kind interleaved (incl. a compaction line where a boundary would reset); activation owners Phase 19/21 named at the test header"
    verification:
      - kind: test
        ref: "tests/internal/session/transcript_newkinds_test.go#TestProjectorToleratesNewKinds"
        status: pass
    human_judgment: false
  - deliverable: "T-16-04 prohibition holds structurally: the unredacted append path is reachable ONLY from raw_thinking lines"
    verification:
      - kind: command
        ref: "grep -rn appendLineUnredacted internal/session — sole non-test caller is AppendRawThinking (manager.go:350)"
        status: pass
    human_judgment: false
---

# Phase 16 Plan 02: Extended Transcript Schema — Additive Kinds + Unredacted Thinking Path Summary

Three additive transcript kinds (raw_thinking / local_command / compaction) locked on disk with a type-scoped redaction-exempt thinking path — proven by a counting Redactor fake (zero calls on thinking bytes, positive on a control kind), a byte-identical payload round-trip, and replay/projector tolerance pinned end-to-end.

## Accomplishments

- **Schema (Task 1):** `TypeRawThinking` / `TypeLocalCommand` / `TypeCompaction` join the kind const block, each doc-comment naming its decision (D-20 additive, D-21 compaction contract, D-22 local-command contract) and payload contract. New `Line` fields follow the field-grouping convention: `Args`, `SourceChain`, `Expansion` (local_command); `BoundaryID`, `CacheTokens`, `PreRef`, `PostRef` (compaction); raw_thinking rides the existing `Content` (payload) + `Model` (provider attribution). On-disk names camelCase with the tagliatelle marker.
- **Unredacted path (Task 1, D-23):** `appendLineUnredacted` is a deliberate near-copy of `appendLine` minus the redact block — no shared helper (Pitfall 5), same mutex/file-nil discipline. `AppendRawThinking` routes through it; `AppendLocalCommand` / `AppendCompaction` route through the REDACTED path like every non-thinking kind. The counting-fake test proves zero Redact calls on thinking bytes and ≥1 on a redacted control; grep proves `AppendRawThinking` is the path's sole caller (T-16-04).
- **Byte-identity:** the raw-thinking payload round-trips marshal→disk→read with `bytes.Equal` — fixture oddities chosen inside the two classes the passthrough covers (whitespace inside strings, existing unicode escapes; compaction only rewrites inter-token whitespace).
- **Provenance records:** local_command stores args verbatim (mixed quoting + double spaces survive untouched), the ordered resolution-source chain, and the expansion outcome; compaction stores a fresh crypto/rand UUID v4 boundary id per append, the input/output/cache token snapshot, and opaque pre/post transcript pointers (Phase 19 reconstructs the reset from the marker alone).
- **Tolerance (Task 2):** the replay reader parses new kinds, unknown future kinds (`quantum_teleport` fixture), and unknown fields on known kinds without dropping a line; `TestProjectorToleratesNewKinds` projects the same two-turn content with and without the new kinds interleaved and asserts deeply equal windows — including a compaction line sitting where a boundary would reset (inert until Phase 19). The Projector dispatch was verified inert by reading (`splitAtResetBoundary` resets only on `TypeBoundary`; `accumulateMidTurn`/`extractSummary` switch only on known kinds), so `projector.go` needed no change — the test pins it, with Phase 19/21 named as activation owners at the test header.

## TDD Gate Compliance

| Task | RED | GREEN | REFACTOR | Status |
|------|-----|-------|----------|--------|
| 16-02 T1 | ✓ `9faca21` (build-failure RED — the kinds/appenders did not exist) | ✓ `e08038c` | — (folded into GREEN: lint-gate restructure of the test file, behavior unchanged) | Pass |
| 16-02 T2 | ✓ `7b891c4` — passed at RED **by design** | — (no feat commit: projector.go correctly unchanged, per the plan's own action text) | — | Pass |

Note: Task 2's test is a pinning test — the plan explicitly routes "if the Projector's existing unknown-kind handling already satisfies this (verify by reading the dispatch logic, not by assumption), the test pins it and projector.go needs no change". The dispatch was read and confirmed inert before writing the test; the pass-at-RED is the sanctioned outcome, mirroring 16-01's documented precedent.

## Deviations from Plan

**1. [Rule 1 - Bug] Compaction subtest assertion scoping fixed**
- **Found during:** Task 1 GREEN (first test run)
- **Issue:** the subtest loop asserted the FIRST boundary's turn id and pre/post pointers against BOTH appended compaction lines (the second line carries turn_4/line:42→43 by design), producing two spurious failures.
- **Fix:** per-line assertions scoped to the usage snapshot + UUID shape; field scoping asserted once against the first boundary; freshness asserted via collected ids.
- **Files modified:** internal/session/transcript_newkinds_test.go
- **Verification:** all five subtests green under -race
- **Commit:** e08038c

**2. [Plan-structure note] Lint-gate restructure of the test file (folded into GREEN)**
The strict golangci gate (gocognit on the subtest parent, goconst across package test files, lll, noinlineerr) required extracting subtest bodies into helpers and a `mustAppend` fixture helper before the GREEN commit could land clean. Test semantics unchanged; RED/GREEN gate order preserved.

**Total deviations:** 1 auto-fixed (Rule 1, test-side) + 1 structural note. **Impact:** low — all within the plan's file set; production code landed exactly as planned.

## Issues Encountered

None.

## Authentication Gates

None.

## Known Stubs

None. The three kinds are intentionally producer-less until their consuming phases land (Phase 19 compaction, Phase 20 local_command, Phase 21 raw_thinking replay) — that sequencing IS the plan's objective ("lands the schema now so their wire work needs no on-disk-format change"); nothing is fabricated or placeholder in the meantime, and the appenders are fully implemented + tested.

## Threat Model Mitigations Applied

- **T-16-04 (Information Disclosure, raw_thinking on disk):** exclusion is type-scoped — `appendLineUnredacted`'s sole caller is `AppendRawThinking` (grep-verified); file perms unchanged (0600 via `filePermOwner`); the zero-calls counting test plus the SUMMARY coverage entry pin the boundary. No credential-bearing kind gains the exemption.
- **T-16-05 (Tampering, replay with unknown kinds):** tolerant parse pinned by the fixture test — unknown kinds/fields are preserved as inert discriminators, never executed.
- **T-16-SC (package installs):** none — stdlib only (`crypto/rand` joined; no new module deps).

## Verification Results

- `go test -race ./internal/session/ -run TestTranscriptNewKinds -count=1` — green (all five behavior cases in one run)
- `go test ./internal/session/ -run TestProjectorToleratesNewKinds -count=1` — green
- `go test -race ./internal/session/ -count=1` — green (full package incl. both new tests)
- `go test ./internal/session/ ./internal/runtime/ -count=1` — green (no projection/runtime regression)
- `go vet ./internal/session/ ./internal/runtime/` — clean; `golangci-lint run ./internal/session/` — 0 issues
- `mise ci` (vet + lint + build + `go test -race ./...`) — green, exit 0, 68s

## Self-Check: PASSED

- internal/session/transcript.go, internal/session/manager.go, internal/session/transcript_newkinds_test.go exist on disk ✓
- Commits 9faca21, e08038c, 7b891c4 present in git log ✓
- All task acceptance criteria re-run and passing; plan-level `<verification>` commands green ✓
- Post-commit deletion check: no tracked files deleted across the plan's commits ✓

## Next

Ready for 16-03 (background emitters / remaining wire plans — the transcript schema Phases 19–21 consume is now locked on disk).
