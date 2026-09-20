---
phase: 16-acp-wire-foundation
plan: 04
subsystem: config-persistence
tags: [yaml, config-layers, atomic-write, modelrouting, providerfactory, tdd]
requires:
  - "internal/modelrouting Load/deepMerge layered loader + embedded default floor (D-08 precedence chain)"
  - "internal/providerfactory GlobalConfigPath/ProjectConfigPath layer addressing + WarnLooseConfigPerm 0600 conventions"
provides:
  - "WriteLayerOption(layerPath, keyPath, value) — the D-07 persist-then-apply layer writer: read-as-generic-map, deep-merge via the loader's own semantics, atomic temp+rename at hard 0600, created dirs 0750 max"
  - "Typed LayerReadError / LayerWriteError — distinct read/parse vs persist failure classes for 16-05's distinct JSON-RPC error data"
  - "modelrouting.DeepMerge (exported) — the single source of layer-overlay semantics shared by Load and the writer"
  - "Config.SessionTier (yaml session_tier), defaulted heavy at the standard defaults-application site — additive, round-trip-safe in either layer"
affects:
  - "16-05 (wire surface — calls WriteLayerOption behind set_config_option; advertises + applies session_tier; owns key whitelisting and D-09 typed rejection)"
  - "16-06 (consumes the session_tier round-trip proof for its ACP-03+ACP-08 criterion)"
tech-stack:
  added: [] # yaml.v3 already in tree; no new deps
  patterns:
    - "writer reuses the loader's exported merge helper (DeepMerge) instead of forking overlay semantics — inverse-compatibility is structural, not asserted by convention"
    - "unexported renameFunc seam (Phase-15 parity-seam convention) for crash-window injection: temp+rename is observable without chmod gymnastics"
    - "typed error pair (read vs write) mapped to caller-distinct wire error classes"
key-files:
  created:
    - internal/providerfactory/config_write.go
    - internal/providerfactory/config_write_test.go
  modified:
    - internal/modelrouting/config.go
    - internal/modelrouting/load.go
    - internal/modelrouting/load_test.go
    - internal/providerfactory/goconst_constants.go
key-decisions:
  - "modelrouting.DeepMerge exported (was unexported deepMerge) and reused by the writer — a written layer is inverse-compatible with Load by construction; no forked merge semantics to drift"
  - "Atomic replace with hard 0600 on the staged temp (re-chmod'd after write so any umask yields exactly 0600), 0750-max dir creation; no fsync — the repo's artifact-write convention (session append path) carries none"
  - "Rename-failure proven via the renameFunc unexported seam rather than a chmod trick — deterministic, root-proof, and matches the same-package test-injection convention"
  - "session_tier is parse/default/round-trip only in this plan: Validate does NOT cross-reference it; tier consumption and D-09 typed rejection land in 16-05's apply seam (config packages stay ACP-free, T-16-12 whitelisting stays wire-side)"
  - "Round-trip test writes glm-5.2 (not the plan's literal glm-5.3): the floor's heavy primary is GLM-5.3 wire-exact casing, and glm-5.3 is undeclared — Load's D-10 validation would reject it. glm-5.2 is declared AND differs from the primary, so a pass proves the write won rather than the default echoing"
requirements-completed: [ACP-08]
duration: 13 min
completed: 2026-08-27
status: complete
estimate:
  tokens: 24000
  raw_tokens: 24000
  tasks: 2
actuals:
  tokens: 5753 # chars/4 over the realized diff (23,011 chars, 6 files, 509 insertions)
  tasks: 2
  commits: 4
coverage:
  - id: D1
    description: "WriteLayerOption persists a nested key-path write into a layer file with atomic replace at exactly 0600, and the REAL loader (modelrouting.Load) resolves the written value at the written key path — round-trip through the loader, not a re-parse"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/providerfactory/config_write_test.go#TestConfigWrite_CreatesLayerAndRoundTripsThroughLoad"
        status: pass
    human_judgment: false
  - id: D2
    description: "Failure transparency (D-07 / T-16-10): corrupt-layer reads fail typed (*LayerReadError naming the path) leaving the file byte-identical; unwritable dirs and injected rename failures fail typed (*LayerWriteError) leaving the original intact with zero temp leftovers"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/providerfactory/config_write_test.go#TestConfigWrite_CorruptLayerNeverClobbered"
        status: pass
      - kind: unit
        ref: "tests/internal/providerfactory/config_write_test.go#TestConfigWrite_UnwritableDirectory"
        status: pass
      - kind: unit
        ref: "tests/internal/providerfactory/config_write_test.go#TestConfigWrite_RenameFailureKeepsOriginal"
        status: pass
    human_judgment: false
  - id: D3
    description: "Writing onto a layer carrying unrelated keys preserves every unrelated key — re-Load equals the original loaded Config plus the one changed value (struct comparison, not bytes)"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/providerfactory/config_write_test.go#TestConfigWrite_PreservesUnrelatedKeys"
        status: pass
    human_judgment: false
  - id: D4
    description: "session_tier is an additive config key defaulted to heavy: absent loads heavy, explicit value loads through, marshal→Load round-trip holds; the full persist→read path (write global session_tier light → Load sees light) passes, and a project-layer write leaves the global layer file byte-identical"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/modelrouting/load_test.go#TestSessionTier"
        status: pass
      - kind: unit
        ref: "tests/internal/providerfactory/config_write_test.go#TestConfigWrite_SessionTierRoundTrip"
        status: pass
    human_judgment: false
---

# Phase 16 Plan 04: Config Layer Writer + session_tier Summary

**Atomic 0600 config-layer writer (WriteLayerOption) with typed read/write errors, reusing the loader's exported DeepMerge for inverse-compatible round-trips, plus the additive session_tier key defaulted heavy**

## Performance

- **Duration:** 13 min
- **Started:** 2026-08-27T15:17:31Z
- **Completed:** 2026-08-27T15:30:50Z
- **Tasks:** 2 (both TDD — RED and GREEN gates committed separately)
- **Files modified:** 6

## Accomplishments
- WriteLayerOption: the D-07 write half of editor-driven configuration — read layer as generic map, deep-merge the key-path write with the loader's own overlay semantics, atomic sibling-temp+rename at hard 0600, created dirs 0750 max, no temp leftovers on any failure path
- Typed failure surface: *LayerReadError vs *LayerWriteError so 16-05 maps read/parse vs persist failures to distinct JSON-RPC error data; every failure leaves the existing file byte-identical
- modelrouting.DeepMerge exported — one merge-semantics source shared by Load and the writer (inverse-compatibility by construction)
- Config.SessionTier (yaml session_tier), defaulted heavy at the standard defaults site; parse/default/round-trip proven in both packages including the full persist→read path and global/project layer isolation

## Task Commits

Each task was committed atomically (TDD RED → GREEN per task):

1. **Task 1: Persist-then-apply layer writer** — RED `fd219c5` (test), GREEN `aa42628` (feat)
2. **Task 2: session_tier additive key** — RED `ed49dd3` (test), GREEN `516a03d` (feat)

**Plan metadata:** see git log (docs commit)

_Note: TDD plans carry two commits per task (failing test first, then implementation)._

## Files Created/Modified
- `internal/providerfactory/config_write.go` — WriteLayerOption + typed errors + atomicReplace + nestedMap
- `internal/providerfactory/config_write_test.go` — TestConfigWrite family: round-trip through the real loader, unrelated-key preservation, unwritable dir, corrupt layer, rename-failure seam, session_tier persist→read + layer isolation
- `internal/modelrouting/load.go` — deepMerge exported as DeepMerge (call site updated); SessionTier heavy default in applyDefaults
- `internal/modelrouting/config.go` — SessionTier field (yaml session_tier)
- `internal/modelrouting/load_test.go` — TestSessionTier (absent→heavy, explicit loads, marshal round-trip)
- `internal/providerfactory/goconst_constants.go` — filePermOwnerWrite 0600 / dirPermOwnerGroup 0750 constants

## Decisions Made
- Exported modelrouting.DeepMerge rather than replicating overlay rules — round-trip fidelity is structural (plan action explicitly prefers this)
- No fsync before rename: the repo's artifact-write convention (session transcript append path) carries none; writer keeps parity
- renameFunc seam instead of a chmod trick for the crash-window test — deterministic under root and matches the same-package test-injection convention
- session_tier deliberately unvalidated: consumption is 16-05's apply seam; D-09 rejection stays wire-side (T-16-12 whitelisting too)
- Test fixture writes glm-5.2 instead of the plan's literal glm-5.3 (see Deviations)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Round-trip fixture slug was undeclared — substituted glm-5.2**
- **Found during:** Task 1 (RED test authoring)
- **Issue:** The plan's behavior case writes `tiers.heavy.model: glm-5.3`, but the embedded default declares models `GLM-5.3` (wire-exact casing) and `glm-5.2` — the lowercase `glm-5.3` is undeclared, so modelrouting.Load's D-10 validation would reject the written layer and the round-trip-through-the-real-loader case could never pass as specified
- **Fix:** Write `glm-5.2` (declared fallback slug) — a stronger proof anyway: the floor's heavy primary is GLM-5.3, so a pass proves the written value won rather than the default echoing back
- **Files modified:** internal/providerfactory/config_write_test.go
- **Verification:** TestConfigWrite_CreatesLayerAndRoundTripsThroughLoad passes via the real loader
- **Committed in:** fd219c5 (Task 1 RED commit)

**2. [Rule 3 - Blocking] files_modified frontmatter omitted the sanctioned DeepMerge export site**
- **Found during:** Task 1 (GREEN implementation)
- **Issue:** The plan's action text requires exporting modelrouting's merge helper ("prefer exporting the existing helper over duplication"), but internal/modelrouting/load.go was absent from the plan's files_modified list
- **Fix:** Exported deepMerge as DeepMerge in load.go and updated the single internal call site (no test callers existed; hookdag's deliberate mirror untouched)
- **Files modified:** internal/modelrouting/load.go
- **Verification:** Full modelrouting suite green — export is behavior-neutral
- **Committed in:** aa42628 (Task 1 GREEN commit)

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** Both fixes required for the round-trip contract to be real; no scope creep. The helper export is the plan's own preferred path.

## Issues Encountered
None beyond the deviations above — repo linters (wsl_v5, noinlineerr, funlen, err113, gochecknoglobals) required the usual convention passes, handled inline before each commit.

## TDD Gate Compliance
RED and GREEN gates committed separately for both tasks: `test(16-04)` fd219c5 → `feat(16-04)` aa42628 (Task 1), `test(16-04)` ed49dd3 → `feat(16-04)` 516a03d (Task 2). No refactor commits needed — no cleanup beyond lint-convention passes, which rode the GREEN commits.

## User Setup Required
None — no external service configuration required.

## Next Phase Readiness
- 16-05 can compose WriteLayerOption behind the wire immediately: typed error pair maps to JSON-RPC error data, caller-serializes contract is documented on the function, and session_tier round-trips — the full persist→read path for the tier option the menu exposes
- ACP-08 is NOT yet marked complete in REQUIREMENTS.md: the shared-ID gate (#2388) blocks it while 16-05/16-06 (also declaring ACP-08) lack summaries — the last declaring plan flips it
- Requirements traceability note: `requirements-completed: [ACP-08]` here records this plan's contribution; phase verification owns the final ACP-08 judgment

---
*Phase: 16-acp-wire-foundation*
*Completed: 2026-08-27*

## Self-Check: PASSED

- All 5 key files exist on disk (both created, three modified, SUMMARY present)
- All 5 commits found in history: fd219c5, aa42628, ed49dd3, 516a03d, d22e7bd
- TDD gate order verified: test → feat → test → feat → docs
- Plan-level verification re-run after close-out: `go test ./internal/providerfactory/ ./internal/modelrouting/ -count=1` green; `mise ci` green (vet + lint + build + race tests, full repo)
