---
phase: 21-context-policy-parity-closures
plan: "02"
subsystem: context
tags: [memory-injection, agents-md, claude-md, system-context, mtime-cache, d-05-collision, d-08-caps, claude-code-parity, go]

requires:
  - phase: 12-built-in-tools-infra
    provides: the ecosys substrate this rides (readCapped/logPluginSkipf degrade discipline, the SkillListing render-then-skip shape, the sessionFor trailing-TextBlock merge vehicle from 08-05/12-02)
provides:
  - DiscoverMemoryFiles — the AGENTS.md/CLAUDE.md walker: cwd→git-root levels (.git FILE counts, worktree-safe; no-git fallback = cwd level only), CLAUDE.md-wins same-directory collision with an AGENTS.md skip-note (D-05), user global ~/.ass-guard/{CLAUDE,AGENTS}.md blocking ~/.claude/CLAUDE.md (D-07), pinned order user-global-first then root descending to cwd
  - MemoryInjection — the capped, note-annotated injection body ("" on zero files → no merge); per-file 24 KB rune-safe truncation with original-byte-count notes, 64 KB total budget skipping whole files with per-file notes (D-08)
  - the mtime+size-keyed package read cache (reads bounded at 24 KB+1 so giant files never fully load, T-21-07) + ResetMemoryCache/MemoryCacheReads test seams
  - the FOURTH trailing-TextBlock merge in sessionFor (after AgentListing — memory is the closest-to-conversation static block), one call site, shared profile unmutated
affects: [PAR-04 verification, 21-03+ (system-context consumers), future re-capture (framing header is ass-guard text, not captured ground truth)]

actuals:
  tokens: 10100   # chars/4 over the realized diff (4 files, 1255 insertions; estimate was 32000)
  tasks: 2
  commits: 4

tech-stack:
  added: []   # stdlib only — os, path/filepath, io, sync, unicode/utf8, slices
  patterns:
    - "Cache as pure optimization keyed path+mtime+size — a miss always re-reads; content never stale within the keyed guarantee"
    - "Notes-not-errors injection body — collision/truncation/budget/unreadable all degrade to inline [note: …] entries, nothing ever errors up through sessionFor"
    - "Budget applied at injection, per-file cap at discovery (whole-file skip over partial budget cuts)"

key-files:
  created:
    - internal/ecosys/memory.go
    - internal/ecosys/memory_test.go
    - internal/runtime/memory_wiring_test.go
  modified:
    - internal/runtime/runtime.go

key-decisions:
  - "Total budget = whole-file fits-or-skips (a file whose capped content no longer fits is skipped with a note, never partially cut by the budget) — deterministic, observably loud"
  - "~/.ass-guard existence-gate: ANY present candidate there (even an unreadable one) blocks ~/.claude/CLAUDE.md — D-07's win-on-conflict read conservatively"
  - "Injection framing header is operator text ('The following project memory files apply to this session:'), flagged for the Phase-9-style re-capture to pin against the mimicry target"
  - "Rune-safe truncation backs off incomplete trailing runes (byte cut + DecodeLastRuneInString trim) — the truncateSkillDesc idiom generalized to arbitrary file bytes"

patterns-established:
  - "Pattern: read-bounded cache entry — cache stores the raw 24 KB+1 prefix, caps are downstream policy, so cache and D-08 stay orthogonal"
  - "Pattern: skip-note pseudo-entries (MemoryFile.SkipNote != \"\") carry observability through discovery → rendering without a parallel channel"

requirements-completed: [PAR-04]

coverage:
  - id: D1
    description: "Memory walker + collision/span/precedence rules: D-05 CLAUDE.md-wins with exactly one skip-note, D-06 git-root stop (file or .git-file boundary; above-root content provably absent; no-git cwd-only fallback), D-07 four HOME combinations, pinned user-global-first ordering, accumulate-without-dedup"
    requirement: PAR-04
    verification:
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_Collision
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_RepoRootStop
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_GitFileBoundary
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_NoGitFallback
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_UserGlobalPrecedence
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_Ordering
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_AccumulateNoDedup
        status: pass
    human_judgment: false
  - id: D2
    description: "D-08 caps + degrade-softly + mtime cache: per-file 24 KB rune-safe truncation with original-byte-count notes, 64 KB total budget naming every skipped file, empty tree → \"\" (render-then-skip), unreadable/dir-named entries skipped with notes, cache hit/re-read/size-change key semantics"
    requirement: PAR-04
    verification:
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_PerFileCap
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_TotalBudget
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_EmptyTree
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_UnreadableEntries
        status: pass
      - kind: unit
        ref: internal/ecosys/memory_test.go#TestMemoryDiscovery_Cache
        status: pass
    human_judgment: false
  - id: D3
    description: "sessionFor wiring (criterion 3): a fresh session in a memory-bearing repo carries the injected block as the LAST system TextBlock (after skills + agent listings) automatically; empty tree adds nothing; shared r.profile byte-identical; truncation notes flow through end-to-end; exactly one MemoryInjection call site"
    requirement: PAR-04
    verification:
      - kind: integration
        ref: internal/runtime/memory_wiring_test.go#TestMemoryInjection_MemoryBlockIsLast
        status: pass
      - kind: integration
        ref: internal/runtime/memory_wiring_test.go#TestMemoryInjection_EmptyTreeAddsNothing
        status: pass
      - kind: integration
        ref: internal/runtime/memory_wiring_test.go#TestMemoryInjection_SharedProfileUntouched
        status: pass
      - kind: integration
        ref: internal/runtime/memory_wiring_test.go#TestMemoryInjection_TruncationFlowsThrough
        status: pass
      - kind: other
        ref: "grep -c 'MemoryInjection' internal/runtime/runtime.go == 1"
        status: pass
    human_judgment: false

duration: 34min
completed: 2026-09-03
status: complete
---

# Phase 21 Plan 02: AGENTS.md/CLAUDE.md Auto-Injection Summary

**Memory walker (D-05 collision, D-06 git-root stop, D-07 user-global precedence) with an mtime+size cache and D-08 caps, riding the fourth trailing-TextBlock merge into every fresh session's profile copy**

## Performance

- **Duration:** 34 min
- **Started:** 2026-09-03T18:17:07Z
- **Completed:** 2026-09-03T18:51:09Z
- **Tasks:** 2 (both TDD: RED → GREEN)
- **Files modified:** 4

## Accomplishments
- `internal/ecosys/memory.go`: DiscoverMemoryFiles + MemoryInjection + the package cache — every D-05/D-06/D-07/D-08 rule table-pinned, degrading softly on every malformed or unreadable input
- The fourth trailing-TextBlock merge in `sessionFor` (after AgentListing): one call site, empty body → no merge, shared profile never mutated — criterion 3 holds at the wiring level
- 16 new tests (12 walker + 4 wiring) green under `-race`; the plan's `<verify>` commands and `go vet` clean; full-repo `mise run test` green (37/37 packages)

## Task Commits

Each task was committed atomically (TDD — RED then GREEN):

1. **Task 1: Memory walker + mtime cache + capped injection body** - `53e5651` (test) → `ea40f33` (feat)
2. **Task 2: sessionFor wiring — the fourth trailing-TextBlock merge** - `ca261f5` (test) → `c3912b8` (feat)

**Plan metadata:** (see final docs commit)

## Files Created/Modified
- `internal/ecosys/memory.go` - the walker, cache, caps, and injection body (NEW)
- `internal/ecosys/memory_test.go` - the D-05..D-08 table + cache/counting cases (NEW)
- `internal/runtime/runtime.go` - the memory merge block in sessionFor after AgentListing (+13 lines)
- `internal/runtime/memory_wiring_test.go` - the sessionFor integration tests (NEW)

## Decisions Made
- Budget = whole-file fits-or-skips; per-file cap at discovery, budget at injection (see key-decisions)
- `MemoryFile` grew `OrigBytes` + `SkipNote` beyond the artifact's 4-field sketch — OrigBytes carries the original size the truncation note must show; SkipNote carries collision/unreadable notes from discovery to rendering without a parallel channel
- User-global ordering: the `~/.claude/CLAUDE.md` entry renders under its own path header like every other source (auditable provenance)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Accumulate-no-dedup test passed the repo root instead of the deep workdir**
- **Found during:** Task 1 (first GREEN run)
- **Issue:** The RED test called `MemoryInjection(repo)` while the twin fixture's second level lived at `repo/deep` — walking from the root never sees the deeper level, so the no-dedup pin could never assert 2 occurrences
- **Fix:** Corrected the argument to `MemoryInjection(deep)`; the walker itself was correct
- **Files modified:** internal/ecosys/memory_test.go
- **Verification:** `go test -race ./internal/ecosys/ -run TestMemoryDiscovery -count=1` green (all 12 cases)
- **Committed in:** ea40f33 (Task 1 GREEN)

**2. [Rule 3 - Blocking] Lint findings on the new files (wsl_v5, noinlineerr, goconst, gochecknoglobals, staticcheck QF1012, wrapcheck, modernize, nonamedreturns/unnamedResult, gofmt)**
- **Found during:** Task 2 acceptance (`mise ci` lint step)
- **Issue:** The strict house config flagged 30+ findings on the new files (the lint gate is already red repo-wide from the ledgered exhaustruct_v5 drift, but the new files must add none)
- **Fix:** Applied the house idioms — plain-assignment error handling, blank-line rules, `slices.Backward`, `fmt.Fprintf(&b, …)` notes, wrapped read errors, marker consts, named-returns nolint on the helper; verified the filtered run reports zero non-exhaustruct findings on all four touched files
- **Files modified:** internal/ecosys/memory.go, internal/ecosys/memory_test.go, internal/runtime/memory_wiring_test.go
- **Verification:** filtered `golangci-lint run` output empty for the four files (exhaustruct_v5 aside — house drift); both test batteries green under -race
- **Committed in:** c3912b8 (Task 2 GREEN)

---

**Total deviations:** 2 auto-fixed (1 bug in a test fixture argument, 1 blocking lint conformance)
**Impact on plan:** No scope creep; both fixes land inside the plan's file set.

## Issues Encountered
- One timing flake observed mid-verification: `TestAskPark_PromptResponsePrecedesResolution` (internal/runtime) and `TestZedSimulatorE2E` (internal/acpserve) each failed once under full-suite machine load and passed on every re-run (isolation + full package, 2/2). Both live in files this plan does not touch — logged in `deferred-items.md`, not fixed (scope boundary).
- `mise ci` lint step remains red from the pre-existing ledgered drift (WINDOWS #16: exhaustruct_v5 repo-wide findings the renamed linter now surfaces). vet + build + full-repo `mise run test` are green; the four files this plan touched are lint-clean apart from that linter.

## TDD Gate Compliance
- Task 1: `test(21-02)` 53e5651 precedes `feat(21-02)` ea40f33 — RED failed for the right reason (undefined symbols), GREEN green under -race. PASS
- Task 2: `test(21-02)` ca261f5 precedes `feat(21-02)` c3912b8 — RED failed for the right reason (no memory block merged; the last block was the agent listing), GREEN green under -race. PASS

## User Setup Required
None - no external service configuration required.

## Self-Check: PASSED
- Created files exist on disk (memory.go, memory_test.go, memory_wiring_test.go)
- All four task commits present in git log (53e5651, ea40f33, ca261f5, c3912b8)
- `grep -c MemoryInjection internal/runtime/runtime.go` == 1 (single merge site)
- Plan `<verify>` commands green: `go test -race ./internal/ecosys/ -run TestMemoryDiscovery -count=1`, `go test ./internal/ecosys/ -count=1`, `go test -race ./internal/runtime/ -run TestMemoryInjection -count=1`, `go test ./internal/runtime/ -count=1`; `go vet` clean; `go build ./...` green; full-repo `mise run test` 37/37 packages ok

## Next Phase Readiness
- PAR-04 complete: fresh sessions in memory-bearing repos inject automatically, mtime-cached and capped; ready for 21-03 (thinking pipeline) and later plans unaffected
- The framing header is operator text pending a re-capture pin (flagged in `affects`); CLAUDE.local.md and @file import syntax remain out of scope per the CONTEXT deferred list

---
*Phase: 21-context-policy-parity-closures*
*Completed: 2026-09-03*
