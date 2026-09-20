---
phase: 25-seed-001-kit-extraction-strictly-last
plan: 01
subsystem: infra
tags: [go, verbatim-move, kit, library-extraction, refactoring, equivalence-proof]

requires:
  - phase: 15-internal-runtime-carve-step-0
    provides: verbatim-move discipline, test-ledger instrument precedent, runtime carve that anchors the kit core
  - phase: 16-24 (all)
    provides: the settled surfaces being extracted (strictly-last ordering precondition of Task 3)
provides:
  - kit/{event,redact,checkpoint,profile} — first four packages of the kit/ tree, bodies verbatim, permanent public import paths (D-02 one-way commitment in effect)
  - Pre-move test-function ledger baseline (whole-repo total 1397) for every later 25-xx move plan to assert sum equality against (D-20)
  - eval change-class detector recognition of kit/ paths (five new CLASSES entries, all legacy entries retained)
  - The proven pass-1 move recipe at tree scale: git mv -> goimports -w -> color-moved review -> differential gate comparison
affects: [25-02, 25-03, 25-04, 25-05, 25-06, 25-07, 25-08, 25-09, KIT-02 frontend seam]

actuals:
  tokens: 54900   # chars/4 over the rendered diffs of the three production commits (move diff includes both sides of the 40 renames)
  tasks: 3
  commits: 4      # 03db89d, 2005620, 018975c, 6dc7b62 (metadata docs commit follows; ledger base 5fdf598 is contaminated by the phases-19-24 interlude, so counted by scope-grep instead of rev-list)

tech-stack:
  added: []
  patterns:
    - "Differential equivalence proof for verbatim moves: run the failing gate (lint, race tests) at the pre-move HEAD in a throwaway detached worktree, normalize moved-path prefixes, and require the finding multiset to be byte-identical — stronger than a bare green/red check on a tree with a known-red baseline"
    - "Mechanical purity check for renames: for every staged rename pair, diff HEAD:old vs :new and assert the only +/- lines are quoted import-path rewrites of the four moved packages"

key-files:
  created:
    - kit/event/ (bus, events, doc — moved verbatim from internal/event)
    - kit/redact/ (redaction engine — moved verbatim)
    - kit/checkpoint/ (checkpoint store + testdata — moved verbatim)
    - kit/profile/ (profile model + testdata incl. embedded JSONL corpora — moved verbatim)
    - .planning/phases/25-seed-001-kit-extraction-strictly-last/test-ledger.txt
  modified:
    - scripts/eval-change-class.sh (CLASSES kit/ extension + selftest fixtures + header note)
    - 116 importer files (import-prefix rewrite via goimports -w)
    - 7 importer files with path literals pointing at moved testdata (cmd/ass-guard/parity.go, cli_contract_test.go, internal/audit, internal/parity, internal/paritycli, internal/provider, internal/shaper test files)

key-decisions:
  - "D-02 one-way commitment took effect at tracer commit 6dc7b62: ass-guard-agent/kit/{event,redact,checkpoint,profile} are now the library's permanent public import paths (option-a confirmed at the Task 1 blocking checkpoint, auto-selected under auto_advance, recorded at 2005620)"
  - "Move equivalence is proven differentially, not absolutely: the repo's lint and race-test baselines are pre-existing-red, so the proof is zero move-caused deltas vs pre-move HEAD 018975c — lint finding multisets identical after path normalization (the move FIXES one pre-existing gofmt finding, introduces one lll finding that was wrapped), race-suite failure set identical (6 tests / 3 packages)"
  - "Two importer-side mechanical edits were sanctioned as part of the move: a gofmt struct-literal realignment in internal/runtime/img_capability_test.go and an lll line-wrap in internal/parity/cacheprobe_test.go — both formatter fallout of the mandated goimports -w / path-lengthening, zero moved-file body edits"

patterns-established:
  - "Verbatim-move hygiene at tree scale (D-03 pass 1): whole-dir git mv, goimports -w with local-prefix grouping, color-moved=dimmed-zebra review, per-pair mechanical diff assertion, one atomic commit per move wave"
  - "Shared-ID gate honored: KIT-01 declared by all 9 phase-25 plans stays open until the last sibling SUMMARY exists"

requirements-completed: []   # KIT-01 intentionally NOT listed: 4 of 15 packages moved; shared-ID gate (requirements.ready-ids = 0/1) holds it until 25-02..25-09 finish

coverage:
  - id: D1
    description: "D-02 one-way kit/ boundary commitment confirmed (option-a) before the first permanent import path existed"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "git:2005620 (docs commit recording the auto-selected decision + ordering blocker) and STATE.md decision entry"
        status: pass
    human_judgment: false
  - id: D2
    description: "Pre-move test-function ledger baseline with whole-repo total 1397 (D-20), re-captured post phases 22-24"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "test -f .planning/phases/25-seed-001-kit-extraction-strictly-last/test-ledger.txt && grep -hE '^func Test' across kit/+internal/+cmd/ == 1397 (re-verified post-move: kit 66 + internal 1298 + cmd 33)"
        status: pass
    human_judgment: false
  - id: D3
    description: "eval change-class detector recognizes kit/ paths with all legacy classes retained (Pitfall 1 silent-gate fix)"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "./scripts/eval-change-class.sh --selftest -> 'selftest ok: class matches (8)' (was 4 pre-extension, +4 >= +1); all 15 CLASSES entries grep-verified present"
        status: pass
    human_judgment: false
  - id: D4
    description: "Rank-0 verbatim move: internal/{event,redact,checkpoint,profile} -> kit/ as one atomic commit, bodies verbatim, whole repo building and kit suites green"
    requirement: KIT-01
    verification:
      - kind: unit
        ref: "go test ./kit/... -count=1 -> ok kit/checkpoint, kit/event, kit/profile, kit/redact"
        status: pass
      - kind: other
        ref: "go vet ./... pass; CGO_ENABLED=0 go build ./... pass; exact-quoted import-path residue grep = 0 hits outside .planning/"
        status: pass
    human_judgment: false

duration: 2 interrupted sessions (2026-08-28 start; 2026-09-10 continuation ~9 min active)
completed: 2026-09-10
status: complete
---

# Phase 25 Plan 01: SEED-001 Kit Extraction — Tracer Summary

**First four kit/ packages (event, redact, checkpoint, profile) re-homed verbatim behind the now-effective D-02 one-way import-path commitment, with the phase's equivalence instruments armed and the move proven delta-free against the pre-move tree**

## Performance

- **Duration:** 2 sessions — 2026-08-28 (Tasks 1-2 + interruption mid-Task-3) and 2026-09-10 (continuation: move verification + atomic commit + close-out, ~9 min active)
- **Started:** 2026-08-28 (original session; Task 3 precondition halted the move until phases 19-24 landed)
- **Completed:** 2026-09-10T20:05Z
- **Tasks:** 3/3
- **Files modified:** 160 (40 moved files + 116 importers + 7 path-literal fixups... overlap collapses to 160 unique paths; plus instruments)

## Accomplishments

- Task 1 (checkpoint:decision, gate=blocking): D-02 one-way commitment resolved option-a — auto-selected under auto_advance, recorded at `2005620` with the phase-ordering blocker that then halted the tracer.
- Task 2 (auto): wave-0 instruments landed — test-ledger baseline (whole-repo total **1397**, re-captured at `018975c` after phases 22-24 resolved the ordering blocker) and eval change-class kit/ extension (selftest hits 4 -> 8; all legacy classes retained; additions-only diff).
- Task 3 (tracer): the rank-0 verbatim move executed and committed atomically as `6dc7b62` — 40 files under kit/{event,redact,checkpoint,profile}, 33 byte-identical (R100), 7 differing only in their own quoted import prefix; testdata/embed corpora travelled with their packages.

## Task Commits

1. **Task 1: D-02 blocking decision** — `2005620` (docs: auto-selected option-a + ordering blocker)
2. **Task 2: wave-0 instruments** — `03db89d` (chore: ledger + eval detector), re-capture `018975c`
3. **Task 3: rank-0 verbatim tracer move** — `6dc7b62` (feat: 158 files, one atomic commit)

**Plan metadata:** this docs commit.

## Verification Evidence (re-run 2026-09-10 post-move)

Task 2 acceptance criteria:
- selftest exits 0 printing "selftest ok: class matches (8)" — **PASS** (prior count 4 at 5fdf598, +4 >= +1)
- all five kit/ CLASSES entries present AND all ten legacy entries retained — **PASS** (15/15 grep-verified)
- test-ledger.txt records HEAD sha (a6f5a61) and numeric whole-repo total 1397 — **PASS**
- no legacy CLASSES line deleted (diff additions-only within CLASSES + header) — **PASS**

Task 3 acceptance criteria:
- kit/{event,redact,checkpoint,profile} exist; internal/ originals gone — **PASS**
- go build ./... **PASS**; go vet ./... **PASS**; mise ci — **FAIL at lint only, 100% pre-existing** (see Deviations #3: 596 findings byte-identical at pre-move HEAD 018975c after path normalization; vet/build/test components green; race-suite failures byte-identical to HEAD)
- color-moved review: all 40 moved bodies show as relocations; mechanical per-pair diff asserts the only changed lines are quoted import prefixes — **PASS**
- exact-quoted old import paths: 0 hits outside .planning/ and git history; no string-literal residue in scripts/configs — **PASS**
- go test ./kit/... -count=1 — **PASS** (4/4 packages ok)

Plan-level verification:
- mise ci green at every commit — **NOT MET as written** (pre-existing lint baseline; deviation #3) — equivalence instead proven differentially at every commit touched by this plan
- Ledger baseline with recorded total; post-move sum equality asserted — **PASS** (1397 = 1397: kit 66 + internal 1298 + cmd 33)
- eval-change-class.sh --selftest green with kit/ fixtures — **PASS**
- four rank-0 packages compile, test, and lint at kit/ homes with zero body edits — **PASS** (kit/ lint findings = the 15 that moved verbatim from internal/; zero new)

## Files Created/Modified

- `kit/event/`, `kit/redact/`, `kit/checkpoint/`, `kit/profile/` — moved verbatim (public library paths, D-02)
- `.planning/phases/25-seed-001-kit-extraction-strictly-last/test-ledger.txt` — D-20 baseline
- `scripts/eval-change-class.sh` — kit/ class extension
- 116 importer files — import-prefix rewrites (goimports -w)
- 7 importer files — path-literal rewrites to moved testdata (+1 lll wrap, see Deviations #2)

## Decisions Made

- Continuation protocol: the interrupted mid-Task-3 tree was NOT redone — the orchestrator-verified staged state (40 renames + importer rewrites, build passing) was resumed, the 7 unstaged path-literal fixups diff-verified as move-scope before staging, then the gate ran.
- KIT-01 left open by the shared-ID gate (`requirements.ready-ids` = 0/1 ready): all 9 phase-25 plans declare it; it flips complete only when the last sibling SUMMARY exists.

## Deviations from Plan

### Recorded Deviations

**1. [Process - Interruption/Continuation] Prior executor session died mid-Task-3**
- **Found during:** continuation start
- **Issue:** Tasks 1-2 committed; Task 3's move mechanics executed (git mv + import rewrites + build green) but verification and the atomic commit never ran; 7 importer files with path literals were left unstaged.
- **Fix:** Verified the staged state (40 renames R098-R100, build passing, zero import residue), diff-verified each of the 7 unstaged files as pure path-literal rewrites, staged them by explicit path, then ran the full review + gate + commit. No completed work redone.
- **Files modified:** none beyond the plan's own scope
- **Verification:** this SUMMARY's evidence blocks
- **Committed in:** `6dc7b62`

**2. [Rule 1 - Move regression] lll line-length finding introduced by the path rewrite**
- **Found during:** Task 3 gate (mise ci lint)
- **Issue:** `internal/parity/cacheprobe_test.go:228` grew to 121 chars (>120) when the testdata path literal lengthened (`internal/profile` -> `..`, `..`, `kit`, `profile`).
- **Fix:** Wrapped the `filepath.Join` call across two lines (gofmt-clean); verified `golangci-lint run internal/parity/...` -> 0 issues.
- **Files modified:** internal/parity/cacheprobe_test.go
- **Verification:** lint delta vs pre-move HEAD reduced to exactly one pre-existing gofmt finding FIXED by the move; zero move-caused findings remain.
- **Committed in:** `6dc7b62` (part of the atomic move commit — it is move fallout, not scope creep)

**3. [Category (a) - Pre-existing environmental] mise ci lint task fails on the pre-existing baseline**
- **Found during:** Task 3 gate
- **Issue:** `mise ci` exits 1 at the lint task (596 findings). Rigorous attribution: golangci-lint run at pre-move HEAD 018975c (detached worktree) fails identically; the normalized finding multiset differs only by one pre-existing gofmt finding the move FIXED (`internal/runtime/img_capability_test.go:462`). The 15 kit/ findings are the verbatim-moved twins of their internal/ originals. The race-suite failures (6 tests: TestPermissionsE2E, TestRunSuite_{KSemantics,ArtifactEmit,MatrixAssertKeys,FailureAttribution}, TestRescanConcurrency across acpserve/evalsuite/runtime) also fail byte-identically at HEAD — TestPermissionsE2E is the STATE-recorded Phase-23 cross-workstream regression.
- **Fix:** none applied — per executor instructions, pre-existing repo-wide/config issues are recorded, not fixed in this plan's scope (STATE's LINT BASELINE blocker already tracks the repair ticket).
- **Files modified:** none
- **Verification:** differential proof above; vet, build, kit tests, and all non-pre-existing-failing packages green
- **Committed in:** n/a (documentation only)

**4. [Note - Sanctioned mechanical] gofmt realignment in an importer**
- `internal/runtime/img_capability_test.go` struct-literal key alignment (`Type:`/`Data:` spacing) shifted as mechanical fallout of the mandated `goimports -w` pass. Importer file, not a moved body; zero semantic change; strictly improves the lint baseline.

---

**Total deviations:** 2 auto-fixed/in-plan (1 move regression, 1 continuation recovery), 2 recorded-not-fixed (pre-existing baseline, mechanical note)
**Impact on plan:** The plan's goal is fully achieved — the move itself is provably delta-free. The un-green `mise ci` is a pre-existing repo-wide condition with a tracked repair path, not a defect of this plan.

## Issues Encountered

- Task 3's original precondition (phases 16-24 landed) was unmet at the 2026-08-28 session — the prior executor correctly halted before creating any kit/ path; resolved by executing phases 19-24 and re-capturing the ledger baseline (`018975c`).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Ready for 25-02 (next move rank) — the recipe is proven: whole-dir git mv, goimports -w, per-pair mechanical diff assertion, color-moved review, differential gate comparison, one atomic commit, ledger sum assertion (1397).
- The D-02 one-way commitment is live: kit/ import paths are permanent public contract from `6dc7b62`.
- Known pre-existing (not this plan's): lint baseline (596 findings, STATE blocker) and 6 race-suite failures (STATE cross-workstream note) — both predate the move and block a clean bare `mise ci` exit until repaired by a dedicated touching commit.

## Self-Check: PASSED

- kit/event/, kit/redact/, kit/checkpoint/, kit/profile/ exist on disk — FOUND
- internal/{event,redact,checkpoint,profile} absent — VERIFIED
- test-ledger.txt + eval-change-class.sh — FOUND
- Commits 03db89d, 2005620, 018975c, 6dc7b62 present in git log — FOUND
- All acceptance criteria re-run with PASS evidence above (mise ci lint exception documented as Deviation #3)

---
*Phase: 25-seed-001-kit-extraction-strictly-last*
*Completed: 2026-09-10*
