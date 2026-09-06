---
phase: 19-compaction-cache-control
plan: "01"
subsystem: api
tags: [anthropic-api, cache-control, prompt-caching, parity, profile, shaper, go]

# Dependency graph
requires:
  - phase: 01-mimicry-core
    provides: profile.TextBlock/Profile loader + the Shaper systemBlocks loop this plan extends
  - phase: 14-adoption-readiness-analysis-dispositions
    provides: the paritycli composeCacheProbeInput seam, the committed cache-control pin fixture, and the corpus census (910/910) this emission implements
provides:
  - cache_control {"type":"ephemeral"} emission on every flagged system block of every outgoing request (PAR-02, D-12 disposition 1)
  - the profile-level system_cache_control yaml declaration as the storage form; TextBlock.CacheControl as the runtime carrier
  - the keep-last-4 over-cap degrade policy at the Anthropic 4-breakpoint API cap, pinned 3/4/5/6
  - extractor capture of the declaration (re-captures cannot lose the emission; extract-profile writes the yaml key)
  - the parity cache-probe placement flip — WINDOWS ledger item 5 closed with an untouched baseline
affects: [19-compaction-cache-control sibling plans (PAR-01 compaction engine rides the same profile machinery), 20-commands-skills (the /compact command family), parity re-capture tooling]

actuals:
  tokens: 7500    # chars/4 over the realized diff (29,914 diff chars across internal/, profiles/, cmd/)
  tasks: 3
  commits: 7      # 6 task commits (3 RED + 3 GREEN) + this docs commit

tech-stack:
  added: []       # zero new dependencies — anthropic-sdk-go v1.63.0 already pinned
  patterns:
    - "profile-level presence flag for corpus-uniform captured values (system_cache_control applies to every block at Load; no per-block sidecar channel exists)"
    - "keep-last-4 breakpoint degrade at the provider API cap, documented at the emission site and pinned by position-asserting tests"
    - "probe composition mirrors the same profile field the shaper consumes — one source of truth for emission and verification"

key-files:
  created:
    - internal/profile/testdata/sample-sessions/cache-control.jsonl
  modified:
    - internal/profile/types.go
    - internal/profile/loader.go
    - internal/profile/extract.go
    - internal/profile/goconst_constants.go
    - internal/profile/extract_test.go
    - internal/shaper/shaper.go
    - internal/shaper/shaper_test.go
    - internal/shaper/midturn_test.go
    - internal/paritycli/parity.go
    - internal/paritycli/parity_test.go
    - profiles/zcode/profile.yaml
    - cmd/extract-profile/main.go
    - .planning/WINDOWS.md

key-decisions:
  - "Storage shape: a profile-level presence flag (system_cache_control in profile.yaml) applied to every block at Load — the corpus value is 910/910 uniform so a bool carries full captured fidelity; block-*.txt files carry no metadata channel, so a per-block sidecar was rejected"
  - "Over the Anthropic 4-breakpoint cap the shaper keeps the LAST four flagged blocks (deepest cache prefixes); the probe compares placement CLASSES so it stays green under either policy — degrade pinned by TestShape_CacheControlCapDegrade"
  - "The extractor derives the declaration by reusing ScanContextBehavior's system: placement class over the chosen rollout (the same detection that grounded docs/compaction-decision.md), and extract-profile writes the yaml key — re-captures cannot lose the emission"
  - "The parity probe baseline (AssertPlacementAgainstPin/PinClasses) stayed byte-stable per D-12 — only the composition side changed; verified via git diff against pre-phase ref 73440bd"

patterns-established:
  - "Presence-flag fidelity: corpus-uniform captured values store as one profile-level declaration; the loader fans it out to per-block runtime carriers"
  - "Emission and verification read the same field: the probe composition mirrors profile.TextBlock.CacheControl — no second truth to drift"

requirements-completed: [PAR-02]

coverage:
  - id: D1
    description: "cache_control {\"type\":\"ephemeral\"} emitted on every flagged system block of the marshaled request; unflagged profiles byte-identical to pre-phase output; zero-block profiles shape cleanly"
    requirement: PAR-02
    verification:
      - kind: unit
        ref: internal/shaper/shaper_test.go#TestShape_CacheControlEmission
        status: pass
    human_judgment: false
  - id: D2
    description: "Parity cache probe placement flips green against the committed pin fixture for the flagged zcode profile (WINDOWS #5 closed); unflagged profiles keep reporting the gap; baseline untouched"
    requirement: PAR-02
    verification:
      - kind: unit
        ref: internal/paritycli/parity_test.go#TestCacheProbe_PlacementFlip
        status: pass
      - kind: command
        ref: "git diff --name-only 73440bd -- internal/parity/cacheprobe.go (empty)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Keep-last-4 degrade at the 4-breakpoint API cap (3/4/5/6-block cases, positions asserted, order preserved) + the regenerated golden pinning every system block carries the ephemeral form"
    requirement: PAR-02
    verification:
      - kind: unit
        ref: internal/shaper/shaper_test.go#TestShape_CacheControlCapDegrade
        status: pass
      - kind: unit
        ref: internal/shaper/midturn_test.go#TestMidTurnCapture_GoldenShape (goldenSystemCarriesEphemeral)
        status: pass
    human_judgment: false
  - id: D4
    description: "Extractor captures the profile-level declaration from cache_control-bearing rollouts and extract-profile persists it — future re-captures do not lose the emission"
    requirement: PAR-02
    verification:
      - kind: unit
        ref: internal/profile/extract_test.go#TestExtractFromRollout_CacheControlDeclaration
        status: pass
    human_judgment: false

duration: 21min
completed: 2026-09-06
status: complete
---

# Phase 19 Plan 01: cache_control emission Summary

**cache_control {"type":"ephemeral"} ships on every system block via a profile-level declaration through the loader and Shaper onto the marshaled wire body; the standing parity cache-probe FAIL flips green against an untouched pin baseline (WINDOWS #5 closed), the 4-breakpoint API cap degrades keep-last-4, and the extractor preserves the lever on re-capture.**

## Performance

- **Duration:** 21 min
- **Started:** 2026-09-06T20:14:53Z
- **Completed:** 2026-09-06T20:36:00Z
- **Tasks:** 3 (tracer + 2 auto, all TDD RED→GREEN)
- **Files modified:** 14 (13 code/config + the WINDOWS ledger)

## Accomplishments

- The full emission chain landed: `profiles/zcode/profile.yaml` declares `system_cache_control: true` → `Load` applies the flag to every system TextBlock → `Shaper.Shape` maps it onto `anthropic.TextBlockParam.CacheControl` via `NewCacheControlEphemeralParam()` → the marshaled request body carries exactly `{"type":"ephemeral"}` per block (no ttl key — the SDK field is omitzero).
- Backward compat is proven at the byte level: an unflagged profile's system array marshals byte-identically to the pre-phase construction; zero-block profiles shape without error and emit no breakpoints.
- The parity cache probe (WINDOWS #5, open since 2026-08-19) flipped green: `composeCacheProbeInput` now derives its flags from `profile.TextBlock.CacheControl` — the same field the shaper consumes — while `AssertPlacementAgainstPin`/`PinClasses` stayed byte-stable (verified against pre-phase ref 73440bd).
- The over-cap degrade is pinned: 3/4 flagged blocks all carry the breakpoint; 5/6 degrade to exactly the LAST four, original order preserved — the Anthropic API's 4-breakpoint cap (a 5th returns 400) can never 400 a request once dynamic merges stack system blocks.
- The golden regenerated once and visibly (`midTurnProfile` carries the captured flag) with a NEW assertion pinning every golden system block carries the ephemeral form; the fidelity test (text+type decode) stayed green unmodified.
- The extractor captures the declaration by reusing the corpus-scan detection (`ScanContextBehavior` system: placement class) and `cmd/extract-profile` writes the yaml key — future re-captures cannot lose the emission.

## Task Commits

Each task was committed atomically (TDD: RED test first, then GREEN implementation):

1. **Task 1: Tracer — profile flag to wire emission** — `2b6873d` (test RED) + `9a40325` (feat GREEN)
2. **Task 2: Probe flip — composition derives from the profile, WINDOWS 5 closes** — `6f02f8a` (test RED) + `a63bdac` (feat GREEN, includes the ledger closure)
3. **Task 3: Over-cap degrade, golden regeneration, extractor capture** — `b00877c` (test RED) + `e3e0859` (feat GREEN)

**Plan metadata:** docs commit (this file + STATE/ROADMAP/REQUIREMENTS).

## Files Created/Modified

- `internal/profile/types.go` — TextBlock.CacheControl (json/yaml cache_control, omitempty) + Profile.SystemCacheControl declaration
- `internal/profile/loader.go` — the declaration applies to every system block after readSystemBlocks
- `internal/profile/extract.go` — ExtractResult.SystemCacheControl set via systemCacheDeclared (ScanContextBehavior reuse); explicit field copy replaces the broken TextBlock(b) conversion
- `internal/profile/goconst_constants.go` — classSystemPrefix constant
- `internal/profile/testdata/sample-sessions/cache-control.jsonl` — NEW cache_control-bearing rollout fixture (2 stable main-turn lines)
- `internal/shaper/shaper.go` — gated emission + keep-last-4 degrade with the cap rationale at the site
- `internal/shaper/shaper_test.go` — TestShape_CacheControlEmission + TestShape_CacheControlCapDegrade + temp-profile loader-path helper
- `internal/shaper/midturn_test.go` — golden regeneration (flag on midTurnProfile) + goldenSystemCarriesEphemeral pin
- `internal/paritycli/parity.go` — composeCacheProbeInput derives CacheControl from the profile field; seam comment states the landed truth
- `internal/paritycli/parity_test.go` — TestCacheProbe_PlacementFlip + truthed-up DefaultGap comment
- `profiles/zcode/profile.yaml` — system_cache_control: true
- `cmd/extract-profile/main.go` — writes system_cache_control into profile.yaml when the declaration is set
- `.planning/WINDOWS.md` — item 5 → fixed with the 19-01 reason recorded

## Decisions Made

- Presence-flag storage (see key-decisions): the corpus value is 910/910 uniform, so one profile-level bool carries full fidelity; no per-block sidecar.
- Keep-last-4 over the cap (RESEARCH Open Question 1's resolved policy) — implemented exactly as pinned in this plan's Task 3.
- Extractor detection reuses the committed corpus scanner rather than modeling cache_control in the rollout-side SystemBlock — one detection implementation, no drift between the census that grounded the decision and the extraction that preserves it.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] extract-profile writes the declaration into profile.yaml**
- **Found during:** Task 3 (extractor capture)
- **Issue:** The plan's file list stops at `internal/profile/extract.go`, but the must_haves truth "future rollout extractions set the profile-level cache_control declaration" requires the `cmd/extract-profile` artifact writer to emit the key — without it, `ExtractResult.SystemCacheControl` would be computed and then silently dropped at the profile.yaml boundary, losing the emission on every re-capture.
- **Fix:** Added the 3-line `profileMap["system_cache_control"] = true` branch in `writeArtifact` with a PAR-02 comment.
- **Files modified:** cmd/extract-profile/main.go
- **Verification:** `go build ./...` + `go vet ./cmd/extract-profile/` clean; the extractor unit test pins the ExtractResult side.
- **Committed in:** e3e0859 (Task 3 GREEN)

**2. [Rule 3 - Blocking] windows-fixed reason recorded manually**
- **Found during:** Task 2 (ledger closure)
- **Issue:** The plan's `windows fixed 5 "<reason>"` invocation assumes the tool records the reason, but both installed gsd-tools versions' `fixed` subcommand takes exactly one positional (the id) — the reason field is waive-only in the tool schema.
- **Fix:** Ran `windows fixed 5` (status flipped, resolved_at stamped, counts recomputed), then recorded the 19-01 reason surgically in both the table row and the JSON entry; verified the ledger still parses via `windows status` (ok: true).
- **Files modified:** .planning/WINDOWS.md
- **Verification:** `node .zcode/gsd-core/bin/gsd-tools.cjs windows status` → ok, fixed_count 10 / open 9.
- **Committed in:** a63bdac (Task 2 GREEN)

**3. [Rule 1 - Bug] Task 3 RED test-authoring fixes before the RED commit**
- **Found during:** Task 3 RED
- **Issue:** The cap test first decoded `e["text"]` (a JSON string) into a struct (wrong target), and the first cache-control.jsonl draft had `response` outside the top-level object (invalid JSONL).
- **Fix:** Decode into `string`; regenerate the fixture programmatically with json.dumps.
- **Files modified:** internal/shaper/shaper_test.go, internal/profile/testdata/sample-sessions/cache-control.jsonl
- **Verification:** RED then failed for the designed reasons only (no-cap placement, unflagged golden, missing ExtractResult field).
- **Committed in:** b00877c (Task 3 RED)

---

**Total deviations:** 3 auto-fixed (1 missing-critical Rule 2, 1 blocking Rule 3, 1 bug Rule 1)
**Impact on plan:** All fixes are within the plan's own objective; the extract-profile wiring is the only scope addition (3 lines, required by the plan's must_haves truth). No architectural change, no scope creep beyond the declared truth.

## Issues Encountered

- One-off flake: the first post-Task-1 neighborhood run (`internal/session + runtime + paritycli + parity` under parallel load) showed a single `internal/runtime` FAIL; the exact combination re-ran green twice consecutively (and runtime alone green). Deterministic emission cannot flake — treated as machine warm-up timing, unrelated to this change. No action.

## Authentication Gates

None — no operator-gated commands ran (the parity A/B arms stay faked offline; the probe flip is proven by the committed pin fixture per plan).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- PAR-02 is complete and drift-proof: emission honors the API cap with a pinned policy, goldens prove the wire form, the probe guards the placement classes, and re-captures preserve the lever.
- Ready for 19-02..19-05 (PAR-01 compaction engine): the profile/loader/shaper machinery they ride is unchanged apart from the additive flag, and `profiles/zcode` now shapes with cache_control on every system block.
- The standing `mise ci` lint-leg environment issue (WINDOWS #16/#19, pre-existing) is untouched by this plan — vet/build/test legs green on every touched package.

## TDD Gate Compliance

All three tasks followed RED→GREEN with per-gate commits: `test(19-01)` commits (2b6873d, 6f02f8a, b00877c) each precede their `feat(19-01)` GREEN commits (9a40325, a63bdac, e3e0859), and each RED was verified failing for the designed reason before implementation. No REFACTOR commits needed — the GREEN implementations landed in the shape the plan specified.

## Self-Check: PASSED

- Created/modified files exist on disk: internal/profile/testdata/sample-sessions/cache-control.jsonl FOUND; all 13 modified key-files FOUND.
- Task commits exist: 2b6873d, 9a40325, 6f02f8a, a63bdac, b00877c, e3e0859 all FOUND in git log.
- Plan verification re-run green: `go test -race ./internal/shaper/ ./internal/profile/ ./internal/paritycli/ -count=1` → ok x3.
- `git diff --name-only 73440bd -- internal/parity/cacheprobe.go` → empty (baseline untouched across the whole plan).
- WINDOWS ledger item 5 → fixed (open_count 9, fixed_count 10).

---
*Phase: 19-compaction-cache-control*
*Completed: 2026-09-06*
