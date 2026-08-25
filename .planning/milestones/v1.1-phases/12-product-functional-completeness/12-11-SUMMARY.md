---
phase: 12-product-functional-completeness
plan: "11"
subsystem: acp
tags: [acp, session-context, toolcat, gap-closure, catalog-parity, tdd]

requires:
  - phase: 12
    provides: "the 12-04 SessionReader/TaskOutput executors + UAT gaps G-12-4c/G-12-5a diagnoses"
provides:
  - "ReadSessionContext reads ass-guard's OWN UUID-keyed transcripts (relevant + handoff strategies) AND the captured sess_* vocabulary, traversal-safe for both"
  - "Sessions() enumerates every recorded transcript_<id>.jsonl regardless of id vocabulary"
  - "TaskOutput executes for real on live sessions — the model-visible zcode profile and the execution catalog agree"
  - "TestBackgroundWiring_ProfileCatalogParity: any future non-mcp declared-but-unexecutable (or executable-but-undeclared) tool fails CI with both directions named"
  - "RegisterInteractive warns loudly on stderr when a stub target has no catalog entry"
affects: [uat-retest-4-and-5]

actuals:
  tokens: 65000   # chars/4 over the realized diff (260 insertions + 15 deletions across 6 files)
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "Bidirectional profile-vs-catalog parity gate — completeness walking one side only cannot see an absent entry; diff BOTH surfaces instead"

key-files:
  created: []
  modified:
    - internal/coreexec/messaging.go
    - internal/coreexec/messaging_test.go
    - internal/toolcat/coretools.json
    - internal/toolcat/catalog_test.go
    - internal/coreexec/planmode.go
    - cmd/ass-guard/background_wiring_test.go

key-decisions:
  - "Dual-vocabulary validator in ONE anchored alternation — sess_ branch kept character-for-character (captured-schema provenance), UUID branch added for ass-guard's own newSessionID ids; both alternatives exclude path separators so traversal-safety holds by construction (T-12-11-01)"
  - "toolcat's ReadSessionContext input_schema (sess_* pattern) stays byte-identical per the captured-material never-rewritten discipline — the executor being more permissive than the model-facing pattern follows the TaskStop shell_id-acceptance precedent"
  - "TaskOutput timeout_ms deliberately 600000, NOT TaskStop's 30000: the schema lets the model block up to 600000 ms and the 14-06 Bash precedent sets the backstop at the schema max so the model-controlled inner deadline stays authoritative over DispatchBatch; concurrency_safe false pairs with TaskStop — a potentially long-BLOCKING retrieval must not occupy the parallel read-only pool"
  - "coreToolCount re-pinned 20→21 with a comment naming this plan (re-pin-never-delete discipline; the count pin exists to force exactly this conscious decision)"

patterns-established:
  - "Real-shaped fixtures: offline reader tests seed transcript_<uuid>.jsonl alongside sess_ fixtures so fixture-vocabulary blindness cannot recur"

requirements-completed: [ACP-03, ACP-06]

duration: ~40min
completed: 2026-08-25
status: complete
---

# Phase 12 Plan 11: Gap closure round 2 — session-id vocabulary + TaskOutput catalog entry Summary

**Both remaining major UAT gaps closed: ReadSessionContext now reads ass-guard's own UUID-keyed transcripts (G-12-4c) and TaskOutput reaches the execution catalog with a permanent bidirectional parity gate so no model-visible tool can silently lack an executor again (G-12-5a).**

## Performance

- **Duration:** ~40 min
- **Tasks:** 3 (RED battery → GREEN fix → RED parity gate → GREEN entry + warn)
- **Files:** 6 modified

## Accomplishments

- G-12-4c: `sessIDPattern` accepts BOTH vocabularies in one anchored alternation — the captured `sess_*` branch character-for-character plus ass-guard's own RFC-4122-v4 UUID form (`internal/acp/handlers.go` newSessionID → `transcript_<uuid>.jsonl`); `Sessions()` widened from `transcript_sess_*` to every `transcript_<id>.jsonl`. Both alternatives exclude path separators, so `read()`'s filepath.Join under `.ass-guard/` cannot escape the store — traversal-safe by construction.
- G-12-5a: coretools.json gains the TaskOutput entry between TaskStop and TodoRead — Name/Description/InputSchema copied VERBATIM from the seed zcode declaration (parse-equivalent bytes, single-space indent preserved); RegisterInteractive's existing `"TaskOutput": TaskOutputExecute(cfg.Tasks)` stub auto-wires it, completing the chain on every live session.
- New permanent gates: `TestBackgroundWiring_ProfileCatalogParity` (bidirectional set equality between seed-profile non-mcp decls and catalog Names(), failure naming each asymmetric tool per direction) extends the completeness gate past its one-sided blind spot; RegisterInteractive now warns loudly on stderr when a stub target has no catalog entry.

## Task Commits

1. **Task 1 RED** — `640fc73` (test): UUID-vocabulary reader battery + real-shaped fixture; fails exactly as UAT test 4 reported ("invalid session id d9f98023-…"; silent enumeration omission); traversal-rejection pin green before AND after
2. **Task 2 GREEN** — `0519a23` (fix): dual-vocabulary traversal-safe validator + general enumeration; full `mise ci` green
3. **Task 3 RED** — `b6fba3b` (test): profile-vs-catalog parity gate failing naming exactly [TaskOutput] — the UAT test 5 finding mechanically
4. **Task 3 GREEN** — `7d79b53` (feat): verbatim catalog entry + count re-pin + loud skipped-stub warn; parity gate green; full `mise ci` green

RED evidence preserved: `/tmp/12-11-red-t1.txt`, `/tmp/12-11-red-t3.txt`.

## Files Created/Modified

- `internal/coreexec/messaging.go` — combined anchored-alternation id validator (dual provenance documented); Sessions() prefix widened to `transcript_`
- `internal/coreexec/messaging_test.go` — real-shaped transcript_<uuid> fixture joins sess_alpha; four new tests (ProductUUIDSession / UUIDHandoff / EnumeratesBothVocabularies / RejectsTraversalIds)
- `internal/toolcat/coretools.json` — TaskOutput entry (captured schema verbatim + assguard annotations)
- `internal/toolcat/catalog_test.go` — coreToolCount 20→21 re-pin; TaskOutput joins declaredConcurrencySafe
- `internal/coreexec/planmode.go` — loud stderr warn on skipped stub targets (skip stays skip)
- `cmd/ass-guard/background_wiring_test.go` — TestBackgroundWiring_ProfileCatalogParity

## Dispositions Required by Plan

- **ReadSessionContext captured schema untouched:** toolcat's input_schema keeps the sess_* pattern byte-identical (never-rewritten discipline); the executor is more permissive than the model-facing pattern per the TaskStop shell_id-acceptance precedent.
- **timeout_ms rationale:** 600000 per the Bash-precedent reasoning recorded in key-decisions.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] TaskOutput needed an explicit declaredConcurrencySafe listing**
- **Found during:** Task 3 GREEN run — TestCatalogParsesContractAnnotations failed
- **Issue:** the new entry carries explicit `concurrency_safe: false` (overriding the read-only default true); the test helper enumerating explicitly-declared tools didn't know it
- **Fix:** added TaskOutput to `declaredConcurrencySafe`
- **Commit:** 7d79b53

**2. [Rule 1 - Bug] Two lint failures in the new parity test**
- **Found during:** Task 3 mise ci
- **Issue:** noinlineerr (inline `if err :=`) and prealloc (unpreallocated catalogOnly slice)
- **Fix:** plain assignment style + make(...) with capacity — matching house test conventions
- **Commit:** 7d79b53

---

**Total deviations:** 2 auto-fixed. No scope creep. Zero new dependencies (T-12-SC held).

## Verification Evidence

- RED T1: `/tmp/12-11-red-t1.txt` — three new-vocabulary tests fail with the diagnosed signatures; RejectsTraversalIds ok
- GREEN T2: `go test ./internal/coreexec/ ./cmd/ass-guard/ -count=1` ok; `mise ci` green
- RED T3: `/tmp/12-11-red-t3.txt` — parity gate fails naming exactly `[TaskOutput]`; executable-but-not-declared empty
- GREEN T3: toolcat + coreexec + cmd/ass-guard batteries ok; `mise ci` green (vet + lint + build + race tests)

## Next Phase Readiness

- G-12-4c and G-12-5a closed at the code level; UAT tests 4 and 5 ready for re-test (stdio-driven live or operator).
- UAT test 7's outcome clause (no dead ends) is now enforced by the parity gate at CI time.
