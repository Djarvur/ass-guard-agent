---
phase: 25-seed-001-kit-extraction-strictly-last
plan: 03
subsystem: infra
tags: [go, verbatim-move, kit, library-extraction, refactoring, equivalence-proof, pass-1-closeout]

requires:
  - phase: 25-seed-001-kit-extraction-strictly-last
    provides: kit/ ranks 0-3 (12 packages), the 1397 test-ledger baseline, the eval detector kit/ classes, the proven move recipe
provides:
  - kit/session, kit/engine, kit/runtime, kit/internal/enginebridge — the final four relocations; kit/ now holds the complete promotion set (15 top-level packages + the D-04 kit-private seed), all verbatim
  - Pass-1 close-out records: the D-20 ledger sum proof (1397 == 1397), the D-06 app-side disposition record (25 internal/ packages), and the D-20 gate-family exit evidence (eval detector fires, exit 3)
  - The execution-time kit→app edge inventory with recorded drift (12 production edges vs the planning-time 8) — the drift record 25-04..25-07 sever and the 25-09 D-19 gate checks
affects: [25-04, 25-05, 25-06, 25-07, 25-08, 25-09, KIT-02 frontend seam]

actuals:
  tokens: 14242   # chars/4 over the rendered diff c38d6f5..HEAD (56,966 chars; rename headers counted both sides)
  tasks: 3
  commits: 2      # 1d0569d (ranks 4-5), 543afc8 (ranks 6-7) — measured: git rev-list --count c38d6f5..HEAD; docs commit follows; Task 3 is records-only (go mod tidy changed nothing)

tech-stack:
  added: []
  patterns:
    - "Latent-lint surfacing: this golangci-lint version reports some findings (e.g. canonicalheader) only in files whose content changed since the last analysis — an untouched file HIDES them. Touch-probe proof: append one comment line to the pristine baseline file and the findings appear. Differential lint proofs over moved trees must attribute +findings per-file against the mover's own diff: a finding on a line the diff never touched is pre-existing latent, not a regression"
    - "Escalation-battery flakes: internal/coreexec/background_test.go escalation ladder (TermImmuneChildKilledAfterGrace / TermRespectingChildDiesByTerm / ReapAllUsesLadder) fails nondeterministically under full-suite -race load — a DIFFERENT member each run, in both the baseline tree and the move tree. One member flaking in a full run is not move-caused unless it reproduces in isolation"

key-files:
  created:
    - kit/session/ (Session, Manager, Projector, AskBroker, transcript kinds + gate/askqueue/reconcile/list/tombstone/seed/compaction/steerqueue + testdata — moved verbatim, 61 files)
    - kit/engine/ (turn engine — moved verbatim, 15 files)
    - kit/runtime/ (Runner, RunnerConfig, sessionFor, cron wiring, park — the kit core; moved verbatim, 43 files; package doc's kit-ambition paragraph now simply true)
    - kit/internal/enginebridge/ (Engine↔ACP adapters — moved verbatim; kit-PRIVATE per D-04)
  modified:
    - 27 importer files (quoted import-prefix sed + scoped goimports regroup; whole pass-1 diff = 75/75 balanced import lines, zero body edits)

key-decisions:
  - "25-03: D-04 destination confirmed kit/internal/enginebridge — execution-time go list + repo-wide grep show the sole production importer is runtime; the only other importer is runner_battery_test.go (in-package white-box test that rides along inside kit/, legal under Go's internal rule)"
  - "25-03: execution-time drift recorded, not fixed (D-03): kit/session gained internal/perm (gate.go, 17-01/21-06 wiring) beyond the planning-known ecosys edge; kit/runtime gained internal/{perm,sandbox,tasks} beyond the planning six — all three predicted by RESEARCH §Future surfaces. Pass-1 production edge count: 12 (planning-time 8)"
  - "25-03: the 2 'added' canonicalheader findings (img_capability_test.go:245,343) are pre-existing latent content proven present at pristine HEAD by touch-probe — the import-rewrite touching the file SURFACED them; lines 245/343 are untouched by the diff. Recorded, not fixed (D-03 zero body edits; scope boundary)"
  - "25-03: Task 3 landed no production commit — go mod tidy was a no-op (all imports in-module) and the close-out deliverables are this SUMMARY's records; measured commits = 2"

patterns-established:
  - "Per-finding latent-lint attribution (see tech-stack) — the successor to 25-02's cache-clean rule; both are required for honest differential lint over moved trees"
  - "Aggregate purity assertion at plan scale: 121 rename pairs + 27 importers across the whole pass-1 diff contain ZERO +/- lines outside quoted import paths — mechanically stronger than the color-moved review the plan asked for (which this also satisfies)"

requirements-completed: [KIT-03]   # KIT-01 intentionally NOT listed: 6 of 9 sibling plans still declare it; shared-ID gate holds it until the last SUMMARY exists

coverage:
  - id: D1
    description: "Rank-4+5 verbatim moves: session and engine under kit/ with the app edges riding along unfixed (D-03)"
    requirement: KIT-01
    verification:
      - kind: unit
        ref: "go test ./kit/session/ ./kit/engine/ -count=1 -> 2/2 ok (projector/boundary/ask/subagent batteries green at the new home)"
        status: pass
      - kind: other
        ref: "76 rename pairs: 0 impure, only 32 balanced quoted-import lines in 10 pairs, 66 byte-identical; ! test -d internal/session && ! test -d internal/engine; kit/session app edges present: ecosys (planning) + perm (recorded drift)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Rank-6+7 verbatim moves: enginebridge to kit/internal/enginebridge (D-04) and runtime to kit/runtime with its app edges riding along"
    requirement: KIT-01
    verification:
      - kind: unit
        ref: "go test ./kit/runtime/... -count=1 -> ok (216 test functions; planning-time 87 grown by phases 17-24); sole enginebridge production importer = runtime, verified by go list + repo grep this session"
        status: pass
      - kind: other
        ref: "45 rename pairs: 0 impure, 4 balanced import lines; kit/runtime production app-import set = acp, coreexec, ecosys, learning, openspec, sched (planning six) + perm, sandbox, tasks (recorded drift); ! test -d internal/runtime; 15 promotion-set packages + kit/internal under kit/"
        status: pass
    human_judgment: false
  - id: D3
    description: "D-20 relocated-ledger sum equality at final commit"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "grep -hE '^func Test' sums: kit 807 + internal 557 + cmd 33 = 1397 == test-ledger.txt baseline total 1397"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-20 gate family as pass-1 exit proof (differential equivalence; eval detector fires)"
    requirement: KIT-03
    verification:
      - kind: other
        ref: "vet 0 / build 0 at final HEAD; cache-clean lint 598 == task-1 state byte-identical after moved-path normalization (baseline 596 + 2 latent pre-existing surfaced, touch-probe-proven); race failing set == baseline 6 exactly (escalation-battery flakes proven environmental at baseline too); TestCLIBinaryContract{RootAndACP,SupportCommands,Profile} + TestZeroConfigFirstRun green; MERGE_BASE=c38d6f5 mise eval-check-changed -> exit 3 (turn-behavior class fires via kit/ paths)"
        status: pass
    human_judgment: false
  - id: D5
    description: "Pass-1 diff is pure relocation: zero body edits in all of pass 1 (D-03 sanctions zero)"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "git diff c38d6f5..HEAD: 121 renames + 27 modified importers, 75 insertions / 75 deletions, aggregate non-import +/- lines = 0; go mod tidy no-op"
        status: pass
    human_judgment: false

duration: 21 min
completed: 2026-09-10
status: complete
---

# Phase 25 Plan 03: Pass-1 Completion — Ranks 4-7 + Close-Out Summary

**The final four packages re-homed verbatim (session, engine, runtime, enginebridge→kit/internal per D-04) in two mise-ci-equivalent atomic commits — kit/ now holds the complete promotion set, internal/ is app-only, and pass 1 closes with the 1397==1397 ledger proof, the D-06 disposition record, and the eval detector proven to fire on the extraction**

## Performance

- **Duration:** 21 min (2026-09-10T20:39Z → 21:00Z)
- **Started:** 2026-09-10T20:39:06Z
- **Completed:** 2026-09-10T20:59Z
- **Tasks:** 3/3 (Tasks 1-2 production; Task 3 records + gate proofs)
- **Files modified:** 148 (121 moved + 27 importer rewrites; whole-plan diff 75/75 balanced lines)

## Accomplishments

- Task 1 (`1d0569d`): rank-4+5 moves — `internal/session` → `kit/session` (61 files incl. reconcile/tombstone/seed/compaction/steerqueue/gate batteries + testdata) and `internal/engine` → `kit/engine` (15 files) + 50 importer rewrites. 76 rename pairs: 66 byte-identical (R100), 10 differing only in 32 balanced quoted-import lines.
- Task 2 (`543afc8`): rank-6+7 moves — `internal/runtime/enginebridge` → `kit/internal/enginebridge` (D-04 kit-private seed; sole production importer verified to remain runtime) and `internal/runtime` → `kit/runtime` (43 files, all wiring + batteries) + 5 importer rewrites. 45 rename pairs: 0 impure.
- Task 3: pass-1 close-out — ledger sum proof **1397 == 1397** (kit 807 + internal 557 + cmd 33); `go mod tidy` no-op; D-06 disposition record (below); D-20 gate family green/firing (evidence in §Verification Evidence); pure-relocation proof: the whole c38d6f5..HEAD diff contains ZERO changed lines outside quoted import paths.
- Fresh pre-move baseline captured at HEAD `c38d6f5` in a detached worktree (removed after): **596 lint findings, 6 stable race failures** (vet 0 / build 0) — drifted from 25-02's recorded 599/15 exactly as its SUMMARY warned; both moves proven delta-free against it.

## Task Commits

1. **Task 1: rank-4+5 moves (session, engine)** — `1d0569d` (feat; 126 files)
2. **Task 2: rank-6+7 moves (enginebridge, runtime)** — `543afc8` (feat; 48 files)
3. **Task 3: close-out records** — no production commit (tidy no-op; deliverables are this SUMMARY + the gate evidence below)

**Plan metadata:** this docs commit.

## Verification Evidence (re-run at HEAD 543afc8)

Task 1 acceptance criteria:
- kit/session and kit/engine exist; internal/ counterparts gone — **PASS**
- kit/session still compiles against internal/ecosys — **PASS** (and against internal/perm — recorded drift, see §Execution-Time Edge Inventory)
- relocated session suites pass at the new home — **PASS** (projector, boundary, ask, subagent batteries within ./kit/session ok, 5.3s)
- color-moved review shows pure relocation — **PASS** (stronger mechanical form: 76/76 pairs, only quoted import-prefix lines)

Task 2 acceptance criteria:
- kit/runtime and kit/internal/enginebridge exist; internal/runtime tree gone — **PASS**
- all 15 promotion-set packages under kit/ — **PASS** (12 prior + session, engine, runtime top-level + enginebridge as the kit/internal seed)
- kit/runtime's app import list = planning six + recorded drift — **PASS** (acp, coreexec, ecosys, learning, openspec, sched + perm, sandbox, tasks; no others in production files)
- relocated runtime suites pass — **PASS** (`go test ./kit/runtime/... -count=1` ok, 28s; 216 test functions — planning-time 87 grown by phases 17-24, the count this SUMMARY records as the execution-time ledger baseline for kit/runtime)

Task 3 acceptance criteria:
- whole-repo test-function sum equals baseline — **PASS** (1397 == 1397; kit 807 + internal 557 + cmd 33)
- CLI golden + zeroconfig smoke green — **PASS** (`go test ./cmd/ass-guard/ -run 'TestCLIBinaryContract|TestZeroConfigFirstRun'` ok)
- mise eval-check-changed exits 3 — **PASS** (`MERGE_BASE=c38d6f5` → matched class turn-behavior, exit 3 — the kit/runtime paths fire the detector; the silent-gate fix is live)
- SUMMARY contains the D-06 disposition and the zero-body-edits record — **PASS** (both below)

Plan-level verification:
- mise ci green at every commit — **differential equivalence** (pre-existing red, Deviation #3: vet 0 / build 0 / lint 596→598→598 with the +2 proven latent-pre-existing / race failing set identical)
- the six expected kit→app edges are the only app edges — **PASS with recorded drift**: 12 production edges at execution time (planning 8); every addition predicted by RESEARCH §Future surfaces; nothing beyond those (see inventory)
- eval detector fires on the pass-1 diff — **PASS** (exit 3)

## Execution-Time Edge Inventory (pass-1 final state — the D-19 drift record)

Production kit→app edges, verified by go list + grep at HEAD 543afc8:

| Edge | Status vs planning |
|---|---|
| kit/session → internal/ecosys | planning-known (types only: SubagentTypes, Hooks) |
| kit/session → internal/perm | **DRIFT** (gate.go, 17-01 gate / 21-06 readRuleEvaluator join) |
| kit/runtime → internal/acp | planning-known |
| kit/runtime → internal/coreexec | planning-known |
| kit/runtime → internal/ecosys | planning-known |
| kit/runtime → internal/learning | planning-known |
| kit/runtime → internal/openspec | planning-known |
| kit/runtime → internal/sched | planning-known |
| kit/runtime → internal/perm | **DRIFT** (17-01) |
| kit/runtime → internal/sandbox | **DRIFT** (22-05) |
| kit/runtime → internal/tasks | **DRIFT** (22-01/22-03) |
| kit/internal/enginebridge → internal/learning | planning-known (one call: d.learned.Lookup) |

Test-only mentions in kit/runtime additionally: evalharness, evalsuite, modesmatrix, providerfactory (E2E batteries; 28 test files import app packages — the documented ride-along set; gate-scope decision is 25-08/25-09's). All drift sources were enumerated in RESEARCH §Future surfaces before this phase executed; none is a surprise edge — pass 2 (25-04..25-07) must now sever 12 edges, not 8.

## D-06 App-Side Disposition Record (final internal/ set — 25 packages)

Fifteen D-06 app-only packages (CONTEXT decision, verbatim list):
`acpserve, providerfactory, checkpointcmd, learningcmd, modelroutingcmd, profilecheckcmd, paritycli, acp, ecosys, openspec, coreexec, learning, firstrun, evalsuite, parity`

Six unmapped-by-D-06 packages — stays-app-side per D-05 default (verified import-clean, RESEARCH §Reference-app disposition):
`defaults` (go:embed seed consumed by firstrun), `drift` (imports profile/kit only), `evalharness` (imports session/kit only), `loop` (parity helper), `sched` (implements the Scheduler port app-side per D-16), `version` (imports nothing)

Four packages added AFTER D-06 was written (RESEARCH §Future surfaces "new app-side packages") — app-side per D-05 default, recorded here so the disposition is complete:
`perm` (17-01 permission rules), `tasks` (22-01/22-03 task registry), `sandbox` (22-05 landlock/seatbelt), `modesmatrix` (24-05 matrix harness)

Zero packages unaccounted: 15 + 6 + 4 = 25 = `ls internal/ | wc -l`.

## Files Created/Modified

- `kit/session/` — 61 files moved verbatim (Task 1)
- `kit/engine/` — 15 files moved verbatim (Task 1)
- `kit/internal/enginebridge/`, `kit/runtime/` — 45 files moved verbatim (Task 2)
- 27 importer files — quoted import-prefix rewrites (sed + `goimports -w -local github.com/Djarvur/ass-guard-agent`)
- go.mod / go.sum — UNTOUCHED (`go mod tidy` no-op; external deps untouched per RESEARCH §Package Legitimacy Audit)

## Decisions Made

- D-04 discretion resolved for enginebridge: kit/internal/enginebridge (not kit/runtime/enginebridge) — the sole-production-importer precondition held at execution time; no escalation needed.
- Prose comments referencing old paths (in moved bodies, docs/*.md, .golangci.yml) deliberately left untouched — D-03 zero-body-edit discipline, 25-01/25-02 precedent; kit/ doc references are drift for later plans.
- eval-change-class.sh legacy internal/* class entries retained by design (25-01 header note) — they are pattern classes, not file references.

## Deviations from Plan

### Recorded (not fixed — out of scope)

**1. [Pre-existing environmental — methodology discovery] latent lint findings surface only when a file is touched**
- **Found during:** Task 1 lint differential (598 vs 596 baseline)
- **Issue:** the 2 "added" canonicalheader findings (img_capability_test.go:245/343) exist in pristine HEAD content but this golangci-lint version only reports them in files whose content changed. Touch-probe proof: appending one comment line to the pristine baseline file surfaces exactly these 2 findings. My diff touches that file (import swap) but never lines 245/343.
- **Fix:** none — pre-existing latent content; fixing would be a body edit outside move scope (D-03). Attributed and documented; Task 2's multiset is byte-identical to Task 1's (598), so the surface count stabilized.
- **Committed in:** n/a (evidence in /tmp logs summarized here)

**2. [Pre-existing environmental] escalation-battery flakes under full-suite -race load**
- **Found during:** Task 1 race differential
- **Issue:** one escalation-ladder test failed per full-suite run (a different member each time); each passes in isolation. The baseline tree flakes the same battery on re-run (TestEscalation_ReapAllUsesLadder at pristine HEAD) — the family is load-flaky, not move-caused (coreexec is not in the moved set; content changes are import swaps).
- **Fix:** none — environmental. The stable failing set (6 tests) is identical across all runs in both trees.

**3. [Pre-existing environmental] mise ci red at lint + race predates this plan**
- Same shape as 25-02 Deviation #3: 596 findings at c38d6f5 (fresh capture; drifted from 25-02's 599 — the tree evolved) and the 6 stable race failures (ModesMatrix family now passes; TestPermissionsE2E remains the STATE-recorded Phase-23 regression; 4 TestRunSuite_* fixture legs; TestRescanConcurrency). Equivalence proven differentially at both commits.

---

**Total deviations:** 0 auto-fixed (no move-caused defects), 3 recorded-not-fixed (all pre-existing/environmental)
**Impact on plan:** Plan goal fully achieved — both moves provably delta-free; pass 1 closed with all instruments green.

## Issues Encountered

- Task 1's first purity run reported all 76 pairs "impure" — a shell bug (empty `echo` line counted as a non-import change) and, separately, staged-vs-worktree drift (git mv stages original blobs; the sed/goimports rewrites must be `git add`ed before the index diff shows them). Both corrected in-session; the recipe note for 25-04+: **stage the rewrites before the pair assertion**.
- Import-count expectation for Task 2 was 5 files, not ~50 — only acpserve (3 files) imports runtime at repo scope; cmd reaches it through acpserve. Verified by grep + build, no residue.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Ready for 25-04 (pass-2 seam work begins) — recipe notes: fresh baseline capture per plan, cache-clean lint, per-finding latent attribution (new this plan), stage-then-assert purity, balanced-pair check.
- Pass 2 must sever **12** production edges (not the planning-time 8) — the four drift edges (perm ×2, sandbox, tasks) join the list; session→perm is new scope for 25-06 beyond its planning brief.
- Known pre-existing (not this plan's): lint baseline (STATE blocker), 6 stable race failures, escalation-battery flake family (this plan's finding #2 — worth a dedicated ticket).

## Self-Check: PASSED

- kit/session/, kit/engine/, kit/runtime/, kit/internal/enginebridge/ exist on disk — FOUND
- internal/{session,engine,runtime} absent — VERIFIED (internal/ = 25 packages, all dispositioned)
- Ledger 1397 == 1397 — VERIFIED (kit 807 + internal 557 + cmd 33)
- Commits 1d0569d, 543afc8 present in git log — FOUND
- All acceptance criteria re-run with PASS evidence above (mise ci lint/race exception documented as Deviation #3; differential equivalence proven)

---
*Phase: 25-seed-001-kit-extraction-strictly-last*
*Completed: 2026-09-10*
