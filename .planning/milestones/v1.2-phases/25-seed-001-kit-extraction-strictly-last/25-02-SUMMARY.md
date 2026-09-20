---
phase: 25-seed-001-kit-extraction-strictly-last
plan: 02
subsystem: infra
tags: [go, verbatim-move, kit, library-extraction, refactoring, equivalence-proof, goimports]

requires:
  - phase: 25-seed-001-kit-extraction-strictly-last
    provides: kit/{event,redact,checkpoint,profile} rank-0 homes, the D-20 test ledger baseline (1397), the eval detector kit/ classes, and the tracer-proven move recipe
provides:
  - kit/{audit,hookdag,shaper,toolcat,provider,mcp,toolexec,modelrouting} — ranks 1-3 of the promotion set, bodies verbatim, public import paths under the live D-02 commitment; 12 of 15 KIT-01 packages now under kit/
  - All three go:embed payloads resolved at their kit/ homes (hookdag/seeded.yaml, toolcat/coretools.json, modelrouting/defaults/config.yaml — package tests green at the new paths)
  - Pass-1 mid-point import inventory (the record later 25-xx plans and the D-19 gate check against drift): twelve packages, kit-internal edges only, ZERO app edges at ranks 0-3
  - Pre-move red baseline captured at 1002acc (599 lint findings, 15 race-suite failures) with the differential proof that these moves add nothing
affects: [25-03, 25-04, 25-05, 25-06, 25-07, 25-08, 25-09, KIT-02 frontend seam]

actuals:
  tokens: 25687   # chars/4 over the rendered diff 1002acc..581d20c (both move commits; includes rename headers both sides)
  tasks: 2
  commits: 2      # 7e2cfd9 (rank-1), 581d20c (rank-2+3) — measured: git rev-list --count 1002acc..HEAD; docs commit follows
  plan_head_before: 1002acc977138a14c3c6d7b36d16745965c67892

tech-stack:
  added: []        # goimports installed as a dev-time tool (golang.org/x/tools/cmd/goimports@latest, official) — go.mod untouched
  patterns:
    - "Cache-clean differential lint proof: a stale shared ~/.cache/golangci-lint under-reported 2 findings in a full-repo run; later 25-xx executors must run `golangci-lint cache clean` before comparing finding multisets"
    - "Import rewrite = sed on quoted paths THEN goimports -local regroup (goimports alone does not rename import paths)"
    - "Scoped goimports: run only on files grep-matching the old quoted paths — avoids reformatting unrelated pre-existing drift"

key-files:
  created:
    - kit/audit/ (audit logger, body store, mirror — moved verbatim from internal/audit)
    - kit/hookdag/ (hook-DAG executor + seeded.yaml embed — moved verbatim)
    - kit/shaper/ (request shaper + testdata — moved verbatim)
    - kit/toolcat/ (tool catalog + coretools.json embed — moved verbatim)
    - kit/provider/ (Provider iface + Anthropic/OpenAI adapters — moved verbatim)
    - kit/mcp/ (MCP subprocess host — moved verbatim)
    - kit/toolexec/ (tool executors + testdata — moved verbatim)
    - kit/modelrouting/ (resolver/scheduler + defaults/config.yaml embed — moved verbatim)
  modified:
    - 147 importer files (quoted import-prefix rewrites via sed + goimports -local)
    - cmd/ass-guard/modelrouting_test.go (2 testdata path literals repointed ../../internal/modelrouting/ -> ../../kit/modelrouting/)
    - internal/acpserve/config_surface.go (one gofmt directive-separation comment line — sanctioned formatter fallout)

key-decisions:
  - "25-02: lint differential comparisons MUST run on a cache-clean golangci-lint — the shared cache initially hid 2 canonicalheader findings in a full-repo run; the per-package rerun contradicted it and cache-clean restored the true 599-identical multiset"
  - "25-02: the pre-move baseline at 1002acc is 599 lint findings + 15 race failures (9 ModesMatrix fixture-missing legs + TestPermissionsE2E + 4 TestRunSuite_* + TestRescanConcurrency) — grown from 25-01's recorded 6; every later 25-xx move must re-capture its own baseline, not reuse a recorded one"
  - "25-02: goimports -w scoped to grep-matched importer files only (repo-wide runs risk reformatting unrelated pre-existing drift); the one file it did reformat beyond imports (config_surface.go) fixed a pre-existing gofmt finding — strictly improving"

patterns-established:
  - "Rank-ordered batch move with intra-batch edges: all four git mvs of a task land before the single goimports pass, so provider->shaper/toolexec->provider edges never dangle"
  - "Balanced-pair purity assertion: aggregate every +/- line across rename pairs AND importers; the whole 25-02 diff is 213 balanced quoted-import line pairs + 2 path literals + 1 formatter comment line"

requirements-completed: []   # KIT-01 intentionally NOT listed: 12 of 15 packages moved; shared-ID gate (requirements.ready-ids = 0/1) holds it until 25-03..25-09 finish

coverage:
  - id: D1
    description: "Rank-1 verbatim moves: audit, hookdag, shaper, toolcat under kit/ with embeds (seeded.yaml, coretools.json) file-adjacent and resolving"
    requirement: KIT-01
    verification:
      - kind: unit
        ref: "go test ./kit/hookdag/ ./kit/toolcat/ ./kit/audit/ ./kit/shaper/ -count=1 -> 4/4 ok (DefaultHooks + catalog exercised, embed resolved at kit/ paths)"
        status: pass
      - kind: other
        ref: "per-pair mechanical diff: all 43 rename pairs show ONLY quoted import-prefix rewrites; ! test -d internal/hookdag etc. — zero residue"
        status: pass
    human_judgment: false
  - id: D2
    description: "Rank-2+3 verbatim moves: provider, mcp, toolexec, modelrouting under kit/ with defaults/config.yaml embed resolving"
    requirement: KIT-01
    verification:
      - kind: unit
        ref: "go test ./kit/modelrouting/ ./kit/provider/ ./kit/mcp/ ./kit/toolexec/ -count=1 -> 4/4 ok (TestLoadEmbeddedDefault family green at kit home)"
        status: pass
      - kind: other
        ref: "test -f kit/modelrouting/defaults/config.yaml; internal/{provider,mcp,toolexec,modelrouting} gone; zero residue"
        status: pass
    human_judgment: false
  - id: D3
    description: "Pass-1 mid-point inventory: twelve kit packages, kit-internal edges only, zero app edges at ranks 0-3 (the D-19-drift record)"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "go list import enumeration over ./kit/... matches the planning-time graph exactly; grep 'ass-guard-agent/internal/|ass-guard-agent/cmd/' kit/ --include='*.go' -> 0 hits (tests included)"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-20 test-ledger sum equality after the final commit"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "grep -hE '^func Test' sums: kit 341 + internal 1023 + cmd 33 = 1397 == baseline total in test-ledger.txt"
        status: pass
    human_judgment: false
  - id: D5
    description: "Differential gate equivalence at both atomic commits (pre-existing red baseline provably untouched)"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "cache-clean golangci multiset: Task1 599/599 identical after moved-path normalization; Task2 598 = 599 minus one pre-existing gofmt finding FIXED by formatter fallout, nothing added; race failing set identical both commits (15 tests); vet+build green; go test ./kit/... 12/12 ok"
        status: pass
    human_judgment: false

duration: 18 min
completed: 2026-09-10
status: complete
---

# Phase 25 Plan 02: Pass-1 Continuation — Ranks 1-3 Summary

**Eight more packages re-homed verbatim under kit/ (audit, hookdag, shaper, toolcat, provider, mcp, toolexec, modelrouting) in two mise-ci-equivalent atomic commits — 12 of 15 KIT-01 packages now live at their permanent public paths, with all three go:embed payloads proven resolving and the move proven delta-free against a freshly captured pre-move red baseline**

## Performance

- **Duration:** 18 min (2026-09-10T20:07Z → 20:26Z)
- **Started:** 2026-09-10T20:07:18Z
- **Completed:** 2026-09-10T20:26Z
- **Tasks:** 2/2
- **Files modified:** 266 (118 moved files + 147 importer rewrites + literal/formatter fixups; counted across both commits)

## Accomplishments

- Task 1 (`7e2cfd9`): rank-1 moves — `internal/{audit,hookdag,shaper,toolcat}` → `kit/` as whole-directory git mvs (43 files; seeded.yaml + coretools.json travel file-adjacent) + 71 importer rewrites. The entire diff is exactly 91 balanced quoted-import line pairs; zero body edits.
- Task 2 (`581d20c`): rank-2+3 moves — `internal/{provider,mcp,toolexec,modelrouting}` → `kit/` (75 files; defaults/config.yaml travels) + 76 importer rewrites + the 2-line testdata-literal repoint in `cmd/ass-guard/modelrouting_test.go`. Diff: 122 balanced import pairs, 2 path literals, 1 formatter comment line.
- Pre-move red baseline captured at HEAD `1002acc` in a throwaway detached worktree (vet 0 / lint 1 / build 0 / test 1): **599 lint findings, 15 race-suite failures** — and both commits proven to add NOTHING (Task 1: multiset identical; Task 2: one pre-existing gofmt finding *fixed*, zero added; failing test set byte-identical both times).
- Pass-1 mid-point inventory recorded (below): twelve packages, kit-internal edges only, zero app edges — the record the D-19 gate and later plans check drift against.
- D-20 ledger asserted after the final commit: kit 341 + internal 1023 + cmd 33 = **1397 == baseline**.

## Task Commits

1. **Task 1: rank-1 moves (audit, hookdag, shaper, toolcat)** — `7e2cfd9` (feat; 114 files)
2. **Task 2: rank-2+3 moves (provider, mcp, toolexec, modelrouting)** — `581d20c` (feat; 152 files)

**Plan metadata:** this docs commit.

## Verification Evidence (re-run at HEAD 581d20c)

Task 1 acceptance criteria:
- kit/{audit,hookdag,shaper,toolcat} exist; internal/ counterparts gone — **PASS** (residue grep zero)
- seeded.yaml + coretools.json beside consumers; hookdag/toolcat tests pass at kit/ homes — **PASS** (4/4 packages ok)
- mise ci exits 0 at the task's final commit — **NOT MET as written; pre-existing red** (see Deviations #3) — equivalence proven differentially instead: build+vet green, lint multiset identical (599, after moved-path normalization), race failing set identical (15)
- color-moved diff review shows relocations, not rewrites — **PASS** (stronger mechanical form: per-pair diff aggregation shows the ONLY changed lines in all 43 moved files are quoted import-prefix rewrites; rename similarity 96-100%)

Task 2 acceptance criteria:
- kit/{provider,mcp,toolexec,modelrouting} exist; internal/ counterparts gone — **PASS**
- kit/modelrouting/defaults/config.yaml exists; defaults/loading tests pass at the new home — **PASS** (TestLoadEmbeddedDefault family green; test -f verified)
- mise ci green; twelve-package inventory recorded — **differential PASS + inventory PASS** (see §Pass-1 Mid-Point Inventory)
- 12 of 15 KIT-01 packages under kit/ — **PASS** (ls kit/: audit checkpoint event hookdag mcp modelrouting profile provider redact shaper toolcat toolexec)

Plan-level verification:
- mise ci green at both atomic commits — **differential equivalence** (pre-existing baseline; Deviations #3)
- all three go:embed payloads resolve at kit/ paths — **PASS** (package tests exercise DefaultHooks/catalog/embedded defaults)
- execution-time import audits show zero NEW app edges at ranks 0-3 — **PASS** (re-audit before each task; mid-point inventory below)
- twelve of fifteen promotion-set packages under kit/ — **PASS**
- `go test ./kit/... -count=1` at final HEAD — **PASS** (12/12 ok); `go build ./...` — **PASS**; `go vet ./...` — **PASS**
- eval detector: `./scripts/eval-change-class.sh --selftest` — **PASS** ("class matches (8) + docs-only no-match verified"; model:kit/provider + model:kit/shaper pre-armed by 25-01, T-25-07 satisfied)

## Pass-1 Mid-Point Inventory (12 packages — the drift record)

Execution-time `go list` enumeration, taken after Task 2 at `581d20c`:

| Package | kit-internal deps (actual) | Planning-time expectation |
|---|---|---|
| kit/event, kit/redact, kit/checkpoint, kit/profile | — | — (rank 0, moved 25-01) |
| kit/audit | kit/event, kit/redact | event, redact ✓ |
| kit/hookdag | kit/event | event ✓ |
| kit/shaper | kit/profile | profile ✓ |
| kit/toolcat | kit/profile | profile ✓ |
| kit/provider | kit/profile, kit/redact, kit/shaper | profile, redact, shaper ✓ |
| kit/mcp | kit/toolcat | toolcat ✓ |
| kit/toolexec | kit/provider, kit/toolcat | provider, toolcat ✓ |
| kit/modelrouting | kit/event, kit/profile, kit/provider, kit/shaper | event, profile, provider, shaper ✓ |

Zero drift from the planning-time graph. `grep -rn 'ass-guard-agent/internal/\|ass-guard-agent/cmd/' kit/ --include='*.go'` → **0 hits, tests included**. No NEW kit→app edge appeared from phases-17-24 landed code (T-25-08 clear).

## Files Created/Modified

- `kit/{audit,hookdag,shaper,toolcat}` — 43 files moved verbatim (Task 1)
- `kit/{provider,mcp,toolexec,modelrouting}` — 75 files moved verbatim (Task 2)
- 147 importer files — quoted import-prefix rewrites (sed + `goimports -w -local github.com/Djarvur/ass-guard-agent`)
- `cmd/ass-guard/modelrouting_test.go` — testdata path literals repointed (2 lines)
- `internal/acpserve/config_surface.go` — gofmt directive-separation comment line (formatter fallout)

## Decisions Made

- Import rewrite mechanics: sed on quoted paths first, goimports -local second (goimports regroups but does not rename); scoped to grep-matched files to avoid reformatting unrelated drift.
- Rank-ordered batch moves within a task: all git mvs land before the single goimports pass, so intra-batch edges (toolexec→provider, modelrouting→provider) never dangle mid-rewrite.
- Comments/prose referencing old paths (in moved bodies and importers) deliberately left untouched — D-03 zero-body-edit discipline; 25-01 precedent (kit/event/events.go:158 still says "internal/hookdag").

## Deviations from Plan

### Auto-fixed Issues

**1. [Sanctioned mechanical - formatter fallout] gofmt directive-separation line in an importer**
- **Found during:** Task 2 gate (lint differential)
- **Issue:** `goimports -w` on `internal/acpserve/config_surface.go` inserted a bare `//` between a doc comment and the `//nolint:funcorder` directive (gofmt's directive-separation rule). The file carried a pre-existing "File is not properly formatted (gofmt)" finding at baseline.
- **Fix:** kept (formatter-mandated, comment-only, zero semantics) — the fallout FIXED that pre-existing finding, taking the multiset from 599 to 598 with nothing added.
- **Files modified:** internal/acpserve/config_surface.go
- **Verification:** cache-clean lint differential shows exactly this one finding removed and zero findings added.
- **Committed in:** `581d20c`

**2. [Sanctioned mechanical - path literal] testdata literals in cmd**
- **Found during:** Task 2 pre-move literal sweep (the 25-01 seven-file lesson)
- **Issue:** `cmd/ass-guard/modelrouting_test.go:17-18` pin `../../internal/modelrouting/testdata/{valid,invalid_cap_mismatch}.yaml` — would dangle after the move.
- **Fix:** repointed to `../../kit/modelrouting/testdata/...` (2 lines).
- **Files modified:** cmd/ass-guard/modelrouting_test.go
- **Verification:** `go test ./cmd/ass-guard/ -run TestModelRouting -count=1` green within the full-suite run; race failing set unchanged.
- **Committed in:** `581d20c`

### Recorded (not fixed — out of scope)

**3. [Pre-existing environmental] mise ci red at lint + race failures predates this plan**
- **Found during:** pre-move baseline capture (detached worktree at 1002acc)
- **Issue:** `mise ci` exits 1 at lint (599 findings) and 15 race failures exist at HEAD: TestModesMatrix{CronCommands,CronHooks,CronSkills,Interactive,InteractiveSurfaces,Subagent,WakeCommands,WakeHooks,WakeSkills}, TestPermissionsE2E, TestRunSuite_{KSemantics,ArtifactEmit,MatrixAssertKeys,FailureAttribution}, TestRescanConcurrency. The ModesMatrix legs fail on a missing modes-matrix fixture tree (environment-dependent); TestPermissionsE2E is the STATE-recorded Phase-23 cross-workstream regression.
- **Fix:** none applied — scope boundary; the plan's equivalence bar is met differentially (both commits add nothing; one pre-existing finding improved).
- **Committed in:** n/a

**4. [Note - verification methodology] stale golangci-lint cache under-reported findings**
- The first full-repo post-Task-1 lint run reported 597 findings (silently dropping 2 canonicalheader findings that per-package and worktree runs both show); `golangci-lint cache clean` restored the true 599-identical multiset. All recorded comparisons use cache-clean runs. Later 25-xx executors must do the same.

---

**Total deviations:** 2 auto-fixed/sanctioned (formatter fallout kept, path literal repointed), 2 recorded-not-fixed (pre-existing baseline, cache methodology note)
**Impact on plan:** Plan goal fully achieved — both moves provably delta-free; the only lint motion is strictly improving.

## Issues Encountered

- `goimports` alone does not rewrite import paths (it only adds/removes/groups) — the rename needed sed on quoted paths followed by goimports regrouping. Discovered on first attempt (residue count unchanged), corrected immediately.
- Baseline at 1002acc is larger than 25-01's recorded one (15 failing tests vs 6; 599 findings vs 596 at 018975c) — the tree evolved between the two captures. Handled by capturing a fresh baseline this session rather than reusing the recorded numbers.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Ready for 25-03 (session move, rank 4) — recipe unchanged: fresh baseline capture in a detached worktree, cache-clean lint comparison, whole-dir git mv, sed+goimports scoped rewrite, balanced-pair purity assertion, one atomic commit, ledger re-assert (1397).
- session→ecosys is the first kit→app edge (types only) — expected, D-19 gate does not exist yet in pass 1, do not "fix" mid-pass (D-03).
- Known pre-existing (not this plan's): lint baseline (STATE blocker), 15 race failures (ModesMatrix fixture availability + Phase-23 cross-workstream note).

## Self-Check: PASSED

- kit/{audit,hookdag,shaper,toolcat,provider,mcp,toolexec,modelrouting} exist on disk — FOUND
- internal/ counterparts of all eight — ABSENT (verified)
- kit/modelrouting/defaults/config.yaml, kit/hookdag/seeded.yaml, kit/toolcat/coretools.json — FOUND
- Commits 7e2cfd9, 581d20c present in git log — FOUND
- All acceptance criteria re-run with PASS evidence above (mise ci lint exception documented as Deviation #3, differential equivalence proven)

---
*Phase: 25-seed-001-kit-extraction-strictly-last*
*Completed: 2026-09-10*
