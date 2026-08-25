---
phase: 14-adoption-readiness-analysis-dispositions
plan: "06"
subsystem: testing
tags: [tool-contract, timeouts, is-error, retry-discipline, concurrency-flags, occ-census, toolcat, toolexec]

requires:
  - phase: 14-03
    provides: the tools/ clone-coordination precedent (.gitignore layout for read-only third-party clones)
provides:
  - Per-tool timeout backstop at the dispatch seam (DispatchBatch wraps EVERY call in its own ctx deadline; DefaultToolTimeoutMS=120000)
  - Contract annotations across all 19 coretools (timeout_ms / concurrency_safe / destructive) with accessors
  - IsConcurrencySafe-driven pool membership (declared value serializes a read-only tool; D-21 floor independent)
  - The is_error corpus evidence base (live-scan findings + committed fixture + executor comparison, all rows terminal)
  - The retry-site census + retry-only-transient pins (exactly one Kind-gated retry path; the classification table pinned)
  - The occ surface census fixture (pinned commit, 25 tools/63 env vars/6 modes/4 transports) + three-way cross-check accounting
  - docs/tool-contract-inventory.md — the EARLY-06 evidence base
affects: [12-06, 12-07, 12-08, v1.2-flags-knob]

actuals:
  tokens: 44000      # chars/4 over the realized diff (~177k diff chars across the plan's 5 commits)
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "Ass-guard-side contract annotations beside mutability in coretools.json (the captured schema never rewritten)"
    - "Per-call deadline wrapping at the dispatch seam — outer backstop, inner tool-specific deadlines authoritative"
    - "Mechanical corpus-evidence scans as committed tests (isError per-tool counts + payload-free form classes)"

key-files:
  created:
    - internal/toolexec/iserror_corpus_test.go
    - internal/toolexec/testdata/occ-census.json
    - internal/toolexec/testdata/iserror-rollout-fixture.jsonl
    - docs/tool-contract-inventory.md
  modified:
    - internal/toolcat/types.go
    - internal/toolcat/catalog.go
    - internal/toolcat/coretools.json
    - internal/toolcat/catalog_test.go
    - internal/toolexec/batch.go
    - internal/toolexec/batch_test.go
    - internal/coreexec/bash_test.go
    - internal/coreexec/files_test.go
    - internal/provider/errors_test.go
    - internal/scheduler/dispatch_test.go
    - .gitignore

key-decisions:
  - "Bash's timeout backstop annotated 600000 (the schema-declared max), not the plan's literal 120000 example — a 120s outer bound would pre-empt legitimate model-requested timeouts up to 600s, violating the plan's own precedence truth (the model-ms inner deadline stays authoritative)"
  - "EffectiveTimeoutMS() accessor naming (Go forbids a field and method sharing a name; follows the EffectiveMutability house vocabulary)"
  - "concurrency_safe declared false on the session-state writers (TodoWrite, Cron*, TaskStop, SendMessage, ExitPlanMode, AskUserQuestion, Agent) — read-only per the captured catalog but serialized for deterministic ordering"
  - "Read-tracking is_error forms (the DOMINANT live error class, 165 obs.) stay ROUTED: implementing them is a product-behavior change on the locked no-confirmation-tier model (the 08-08 operator disposition; 12-05's re-record carries the forms)"
  - "Engine gating on isDestructive stays ROUTED (unmatched⇒nothing unchanged; EARLY-01 checkpoint restore is the backstop)"

patterns-established:
  - "Names-only census fixtures with provenance {repo, commit, extracted_at} + determinism test over the re-clone (INPUT ONLY discipline made mechanical)"
  - "Retry-site census as a living doc section: every site named with its error-kind gate + companion pin test"

requirements-completed: [EARLY-06]

coverage:
  - id: D1
    description: Per-tool timeout backstop at the dispatch seam — every dispatched call deadline-bounded per-tool, siblings isolated, inner deadlines authoritative, D-21 drain intact
    requirement: EARLY-06
    verification:
      - kind: unit
        ref: internal/toolexec/batch_test.go#TestDispatchBatch_PerToolTimeout_Bounds
        status: pass
      - kind: unit
        ref: internal/toolexec/batch_test.go#TestDispatchBatch_TimeoutDoesNotBreakD21
        status: pass
      - kind: unit
        ref: internal/toolexec/batch_test.go#TestDispatchBatch_MutatingTimeoutBounded
        status: pass
    human_judgment: false
  - id: D2
    description: Contract annotations on all 19 coretools (timeout_ms/concurrency_safe/destructive) parsed with accessors and documented defaults
    requirement: EARLY-06
    verification:
      - kind: unit
        ref: internal/toolcat/catalog_test.go#TestCatalogParsesContractAnnotations
        status: pass
      - kind: unit
        ref: internal/toolcat/catalog_test.go#TestCatalogRawCoretoolsAnnotations
        status: pass
      - kind: unit
        ref: internal/toolexec/batch_test.go#TestDispatchBatch_DefaultBackstop
        status: pass
    human_judgment: false
  - id: D3
    description: Corpus-grounded is_error — mechanical scan, committed fixture, executor comparison with every row terminal (Bash/Read/Edit-not-found match; read-tracking routed)
    requirement: EARLY-06
    verification:
      - kind: unit
        ref: internal/toolexec/iserror_corpus_test.go#TestIsErrorCorpusFixture
        status: pass
      - kind: unit
        ref: internal/toolexec/iserror_corpus_test.go#TestIsErrorCorpusLive
        status: pass
      - kind: unit
        ref: internal/coreexec/bash_test.go#TestIsErrorCorpusForm_BashExitCode
        status: pass
      - kind: unit
        ref: internal/coreexec/files_test.go#TestIsErrorCorpusForm_ReadMissingFile
        status: pass
    human_judgment: false
  - id: D4
    description: occ surface census as INPUT ONLY — pinned-commit fixture (25 tools, 63 env vars w/ defaults count, 6 modes, 4 transports) + three-way cross-check accounting + determinism
    requirement: EARLY-06
    verification:
      - kind: unit
        ref: internal/toolexec/iserror_corpus_test.go#TestOccCensusFixture
        status: pass
      - kind: unit
        ref: internal/toolexec/iserror_corpus_test.go#TestOccCensusCrossCheck
        status: pass
      - kind: unit
        ref: internal/toolexec/iserror_corpus_test.go#TestOccCensusDeterministic
        status: pass
    human_judgment: false
  - id: D5
    description: Flags consumption — IsConcurrencySafe drives DispatchBatch pool membership (declared value serializes; D-21 floor independent; T-14-19)
    requirement: EARLY-06
    verification:
      - kind: unit
        ref: internal/toolexec/batch_test.go#TestDispatchBatch_UsesConcurrencySafeFlag
        status: pass
    human_judgment: false
  - id: D6
    description: Retry-only-transient pinned at the seam — the literal classification table + the scheduler walk's both directions (Structural never retried)
    requirement: EARLY-06
    verification:
      - kind: unit
        ref: internal/provider/errors_test.go#TestRetryOnlyTransient_ClassificationTable
        status: pass
      - kind: unit
        ref: internal/scheduler/dispatch_test.go#TestRetrySites_NeverRetryStructural
        status: pass
    human_judgment: false
  - id: D7
    description: docs/tool-contract-inventory.md — the EARLY-06 evidence base (existing-partials census, is_error findings, retry-site census, occ cross-check, flags map) with every gap dispositioned terminal
    requirement: EARLY-06
    verification:
      - kind: other
        ref: "grep -c 'documented-absent|in shipped catalog|MCP-shaped' docs/tool-contract-inventory.md (non-zero; 25-name accounting) + commit f5f067f"
        status: pass
    human_judgment: false

duration: 45min
completed: 2026-08-19
status: complete
---

# Phase 14 Plan 06: The Uniform Tool Contract Summary

**Catalog-wide per-tool timeout backstop at DispatchBatch, corpus-grounded is_error (283 live observations, 5 form classes), retry-only-transient pins at the classification and scheduler-walk seams, concurrency/destructive annotations consumed for pool membership — plus the occ surface census cross-checked as input only (13 in-catalog + 12 documented-absent = 25 accounted).**

## Performance

- **Duration:** 45 min (2026-08-19T19:52Z → 20:37Z)
- **Tasks:** 3/3
- **Files modified:** 15 (4 created, 11 modified) + 2 testdata fixtures

## Accomplishments

- **No tool call can hold a turn**: every dispatched call (pooled and serialized) is wrapped in its own context deadline from the tool's `timeout_ms` annotation (default `DefaultToolTimeoutMS` = 120000, matching the Bash schema default and the occ census's `CLAUDE_CODE_TOOL_TIMEOUT` default); a timing-out call returns an IsError result with the timeout-classified form and never cancels siblings; existing inner deadlines (Bash model-ms, openspec per-command, web 30s) fire first.
- **The contract is declared, once**: all 19 coretools carry `timeout_ms`; the 16 non-mutating carry explicit `concurrency_safe` (false serializes the session-state writers); `destructive=true` only on Bash. The captured `profiles/zcode/tools.json` is byte-unchanged (verified across all plan commits).
- **is_error is corpus-grounded, not invented**: the mechanical scan (committed to CI via a corpus-shaped fixture) found 283 isError observations in the live 2026-08-19 corpus across 5 form classes; the executor comparison shows every implemented form matches the corpus (Bash `Exit code <N>` anchor, Read missing-file, Edit not-found), with the read-tracking class (165 obs.) routed per the standing 08-08 operator disposition.
- **Retry-only-transient is pinned from both ends**: the classification table (408/425/429/5xx/net → Transient; 400/401/403/422 → Structural, statuses literal) and the scheduler walk (Structural → zero fallback steps; Transient → the walk advances). The census found exactly ONE retry path in the tree and zero hidden retry loops.
- **The occ census paid for nothing and proved the accounting**: pinned-commit fixture (5d007f09), determinism test (re-extraction reproduces it), and the three-way cross-check — 13 in-catalog + 0 MCP-shaped + 12 documented-absent with rationale = 25/25. No census content enters catalog/profile/config.

## Task Commits

1. **Task 1: per-tool timeout enforcement at the dispatch seam** — RED `edfd68c` → GREEN `a0c4644` (tracer; verified end-to-end post-commit, all four named tests green)
2. **Task 2: the contract inventory** — `f5f067f` (is_error grounding, retry census, occ cross-check)
3. **Task 3: close the inventoried gaps** — RED `6aabc8e` → GREEN `3e5006d` (flags consumption + retry/is_error pins)

**Plan metadata:** (this commit)

Note: two operator commits (`8c7e6e9` seed-sync, `3ef79ec` scheduler heavy-primary) landed concurrently mid-execution between commits edfd68c and a0c4644 — not part of this plan; see Issues Encountered.

## Files Created/Modified

- `internal/toolcat/types.go` — TimeoutMS/ConcurrencySafeOpt/Destructive fields + accessors + DefaultToolTimeoutMS
- `internal/toolcat/coretools.json` — the three annotations on all 19 coretools
- `internal/toolexec/batch.go` — executeBounded per-call deadline wrap + isAloneInSlot pool membership
- `internal/toolexec/iserror_corpus_test.go` — the corpus scan + occ census tests (NEW)
- `internal/toolexec/testdata/occ-census.json`, `testdata/iserror-rollout-fixture.jsonl` — committed evidence fixtures (NEW)
- `docs/tool-contract-inventory.md` — the EARLY-06 evidence base (NEW)
- `internal/coreexec/bash_test.go`, `files_test.go` — is_error corpus-form pins
- `internal/provider/errors_test.go`, `internal/scheduler/dispatch_test.go` — retry-only-transient pins
- `.gitignore` — `tools/occ-census-clone/` beside 14-03's pi-audit entry

## Decisions Made

- **Bash backstop = 600000, not the plan's literal "120s"**: the model-ms inner deadline legitimately ranges to the schema max (600000); a 120s outer bound would truncate a model-requested 600s command — violating the plan's own precedence truth. The annotation makes the inner deadline authoritative in every case.
- **`EffectiveTimeoutMS()` naming**: Go forbids a field and a method sharing a name (the plan's "TimeoutMS() accessor" is unimplementable literally); named after the `EffectiveMutability` house precedent.
- **concurrency_safe=false on TodoWrite et al.**: the captured catalog marks them read-only (pool-eligible by default), but they mutate session/schedule state — concurrent duplicates would race; serialization keeps arrival order deterministic.
- **Agent's 600000 + dispatch exemption noted**: Agent/Task never reach DispatchBatch (the session loop dispatches subagents inline) — the annotation is the declared bound for inventory/audit and any future batch-path migration.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug avoidance] Bash timeout_ms annotated 600000 instead of the plan's "120s backstop" example**
- **Found during:** Task 1 (annotation values)
- **Issue:** the plan's example value would make the outer backstop fire before a legitimate model-requested inner timeout >120s (schema max 600000), contradicting the plan's own truths ("the model-ms inner timeout stays authoritative", "existing inner deadlines … fire first")
- **Fix:** annotate 600000 (the schema-declared max) — the inner deadline (always ≤600000 after clamping) fires first in every case; documented in the inventory §e
- **Files modified:** internal/toolcat/coretools.json, docs/tool-contract-inventory.md
- **Verification:** TestDispatchBatch_PerToolTimeout_Bounds + the inner-deadline tests (TestBash_Timeout*) green unchanged
- **Committed in:** a0c4644

**2. [Naming resolution] TimeoutMS() accessor renamed EffectiveTimeoutMS()**
- **Found during:** Task 1 (API realization)
- **Issue:** Go rejects a struct field and method with the same name — the plan's field `TimeoutMS` + accessor `TimeoutMS()` cannot coexist
- **Fix:** `EffectiveTimeoutMS()` (resolved value), following the EffectiveMutability vocabulary; field keeps the plan's name
- **Verification:** TestDispatchBatch_DefaultBackstop asserts the resolved default
- **Committed in:** a0c4644

**3. [Rule 3 - Test-infrastructure blocker] corpus-scan parser rejected mixed-content records**
- **Found during:** Task 2 (live scan returned near-zero findings)
- **Issue:** a strict `content string` field rejected rollout lines whose NON-tool (assistant) messages carry array content blocks — losing the tool messages in the same records (live scan showed 2/7892 messages)
- **Fix:** Content as json.RawMessage + textContent best-effort decode; the committed fixture now pins the mixed-content record
- **Files modified:** internal/toolexec/iserror_corpus_test.go
- **Verification:** live scan 7892/283 as observed directly; TestIsErrorCorpusFixture green
- **Committed in:** f5f067f

---

**Total deviations:** 3 auto-fixed (1 bug-avoidance, 1 naming necessity, 1 blocker)
**Impact on plan:** No scope growth; each preserves a plan truth the literal text would have broken.

## Issues Encountered

- **Concurrent operator commits mid-execution** (resolved): the operator landed `8c7e6e9` (embedded-seed sync to the 09-04 re-pin) + `3ef79ec` (scheduler heavy → GLM-5.3) at 20:08 UTC while Task 1's CI ran — one `go test -race ./...` invocation compiled the tree between their edits (new floor values + old test expectation) and failed `TestLoadSchedulingFactory_LegacyNameNeverRead`. Initially recorded as the 14-03 flake family in deferred-items.md; corrected at close-out to the true cause. All subsequent runs and the final `mise run ci` (exit 0) are green on the coherent tree. Zero interaction with this plan's files.
- **Acceptance-grep nuance**: the plan's `grep -c '"timeout_ms"' == 19` assumes multi-line JSON; coretools.json is single-line (the count is 1 line). The criterion's intent is pinned programmatically by `TestCatalogRawCoretoolsAnnotations` (19 entries, each with timeout_ms > 0) and `grep -o '"timeout_ms"' | wc -l` = 19.
- **Two fixture files vs the plan's one**: `testdata/iserror-rollout-fixture.jsonl` was added beside the planned `occ-census.json` — the inline-fixture alternative exceeded the 120-char line lint on JSONL content; a testdata file is the established house pattern.

## User Setup Required

None — no external service configuration required. (The occ clone is dev-time only, gitignored, re-clonable per the fixture's provenance header.)

## Next Phase Readiness

- EARLY-06 closed: the uniform contract holds catalog-wide with corpus-grounded evidence; every inventoried gap terminal (closed-here / routed / already-pinned).
- Phase 14 complete (6/6 plans): the adoption line's pre-milestone [12-05 → 12-04 → 12-06] is next per STATE's ordering.
- 12-06/12-07 gain the census fixture as a coverage-manifest input (the SEED-004 disposition); v1.2 gains the MCP/plugin annotation knob + the engine-gating decision.

---
*Phase: 14-adoption-readiness-analysis-dispositions*
*Completed: 2026-08-19*

## Self-Check: PASSED

All 5 created files exist on disk; all 6 commits (5 task + 1 summary) present in history; 19 `timeout_ms` occurrences in coretools.json; `mise run ci` exit 0 on the final tree.
