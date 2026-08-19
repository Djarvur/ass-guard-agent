---
phase: 14-adoption-readiness-analysis-dispositions
plan: "03"
subsystem: shaper
tags: [mimicry, anthropropic-messages, audit, pi, cache-control, wire-fidelity, cross-validation]

requires:
  - phase: 14-02 compaction corpus scan
    provides: the corpus cache_control/window census (910/910 ephemeral system placements, zero elsewhere) that grounds the audit's capture-authority resolutions
  - phase: 08-07/08-08 shaper mid-turn seam
    provides: the captured zcode-normalized message shapes (ToolCall{ID,Name,Input}, tool-result pairing) the message-mapping dimension diffs against
provides:
  - docs/shaper-pi-audit.md — the EARLY-04 committed audit: pinned pi commit 496185f, 29 rows across 5 dimensions, every row classified + dispositioned, zero unclassified
  - The .gitignore-scoped tools/pi-audit/ clone procedure (gitignored, never committed, re-clonable at the pin)
  - 14-04's cache-probe placement baseline cross-check (corpus wins on conflict, stated in both docs)
  - Phase-close TDD evidence: the zero-fix walk record (no shaper source changes needed — the shaper already agrees with the captured target form on all five audited surfaces)
affects: [14-04 cache probe, 12-05 re-record, post-adoption cache_control emission queue, 14-06 (.gitignore adjacency)]

actuals:
  tokens: 6600
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "AUDIT-ROW: fixed-table cross-validation against a pinned foreign repo (dimension | pi file:line | ass-guard file:line | classification | disposition) with a locked 5-literal vocabulary and no silent skips"
    - "Capture-authority hierarchy as the audit resolver: zcode corpus > in-repo justification > pi behavior — a pi behavior the target never exhibits is a JUSTIFIED divergence"

key-files:
  created:
    - docs/shaper-pi-audit.md
    - .planning/phases/14-adoption-readiness-analysis-dispositions/deferred-items.md
  modified:
    - .gitignore

key-decisions:
  - "cache_control system-block emission stays ROUTED (post-adoption queue per 14-02 disposition #1), not fixed in 14-03: the fix requires the profile-bundle format change (TextBlock gains the captured value, TIER-1) — not shaper-local; 14-04 stays probe-only"
  - "Zero in-phase fixes is the audit's legitimate result: every pi divergence on the five audited surfaces is pi's N-provider generality (compat matrix, header layering, session repair, cross-provider id migration) that the single-capture shaper deliberately does not replicate"
  - "pi's test suite consulted as spec oracle (cache-retention.test.ts, anthropic-thinking-disable.test.ts, transform-messages-copilot-openai-to-anthropic.test.ts) — cited alongside source in the rows"
  - "Task 3's tdd=true contract resolved to the plan's zero-fix path: the audit walklist contained no divergence-fixed rows, so no implementation step existed; doc-only finalization (gate-exempt)"
patterns-established:
  - "AUDIT-ROW pattern: foreign-repo behavioral diffs are citation tables at a recorded pin, never pasted code (>5-line blocks prohibited), never vendored"
  - "absent-at-pin rows record searched locations for planning-time symbol guesses that do not exist at the pin (transformHeaders → mergeHeaders)"

requirements-completed: [EARLY-04]

coverage:
  - id: D1
    description: "The EARLY-04 committed audit: pi cloned at pinned commit 496185f, internal/shaper diffed on 5 dimensions, 29 rows each classified + dispositioned with both-side file:line citations"
    requirement: EARLY-04
    verification:
      - kind: other
        ref: "Task 1/2/3 automated verify greps (pin recorded, per-dimension row coverage, 29/29 rows carry exactly one locked classification literal, git check-ignore tools/pi-audit)"
        status: pass
    human_judgment: true
    rationale: "Structural acceptance criteria all passed mechanically, but the correctness of each row's classification (justified vs routed vs match) is analytical judgment over the cited evidence — a human sign-off on the dispositions is the appropriate UAT for an audit artifact."
  - id: D2
    description: "The pinned-clone procedure: tools/pi-audit/ shallow clone gitignored, never committed; pin + clone command + re-derivation recorded in the audit header"
    requirement: EARLY-04
    verification:
      - kind: other
        ref: "git check-ignore tools/pi-audit (exit 0) + git show --stat on all three plan commits (only .gitignore/docs/.planning files, zero pi content)"
        status: pass
    human_judgment: false
  - id: D3
    description: "The zero-fix disposition outcome: no divergence-fixed rows existed, so no shaper source changed; fidelity battery + full CI green unmodified"
    requirement: EARLY-04
    verification:
      - kind: unit
        ref: "go test ./internal/shaper/... -count=1 (ok)"
        status: pass
      - kind: other
        ref: "mise run ci (exit 0, 26 pkgs ok); git diff over go.mod/go.sum/shaper.go/shaper_test.go/fidelity_test.go across plan commits is empty"
        status: pass
    human_judgment: false

duration: 12min
completed: 2026-08-19
status: complete
---

# Phase 14 Plan 03: pi↔shaper cross-validation Summary

**29-row audit diffing internal/shaper against pi's anthropic-messages.ts at pinned commit 496185f across 5 wire dimensions — every row dispositioned (2 routed per 14-02, 24 justified by capture authority, 2 match, 1 absent-at-pin); zero in-phase fixes because the shaper already agrees with the captured zcode form on every audited surface**

## Performance

- **Duration:** 12 min (749s)
- **Started:** 2026-08-19T18:25:32Z
- **Completed:** 2026-08-19T18:41:01Z
- **Tasks:** 3/3
- **Files modified:** 3 (.gitignore, docs/shaper-pi-audit.md, phase deferred-items.md)

## Accomplishments

- Cloned pi (earendil-works, MIT) at pinned commit `496185f` under gitignored `tools/pi-audit/`; read the wire layer (anthropic-messages.ts 1,391 lines + transform-messages.ts 223 lines) plus its test suite as spec oracle — no pi code executed, imported, or committed (T-14-SC)
- Committed the EARLY-04 audit (`docs/shaper-pi-audit.md`, 143 lines): 29 rows across cache_control placement (4), thinking-config mapping (3), header merge order (3), compat-flag catalog (11), and transform-messages message-shape mapping (8) — every row cited file:line on BOTH sides, every row carries exactly one locked classification literal, zero silent skips (the plan's `transformHeaders` symbol guess resolved to `mergeHeaders` via an absent-at-pin row)
- Resolved the cache_control dimension jointly with 14-02's corpus: system-block emission stays ROUTED to the post-adoption queue (profile-bundle format change, not shaper-local; 14-04 probe-only); pi's tool-placement and last-user-message breakpoints are JUSTIFIED divergences (the target never exhibits them — 0 placements across 228 corpus records)
- Task 3 walk: zero divergence-fixed rows → the plan's legitimate empty-fix path; shaper battery + full `mise run ci` green with go.mod/go.sum/shaper sources/fidelity tests untouched across the plan

## Task Commits

1. **Task 1: Clone pi at a pin; audit cache_control end-to-end (tracer)** — `165ac43` (docs)
2. **Task 2: Complete the audit matrix (thinking/header/compat/message mapping)** — `a8224e9` (docs)
3. **Task 3: Apply fix-dispositions — zero-fix walk recorded** — `805800f` (docs)

**Plan metadata:** committed with STATE/ROADMAP updates (see below).

## Files Created/Modified

- `docs/shaper-pi-audit.md` — the EARLY-04 audit: provenance header (pin, clone command, MIT license, corpus authority), 5-dimension table (29 rows), fix list, reproducibility
- `.gitignore` — `tools/pi-audit/` entry with a scoped comment (14-06 extends beside it)
- `.planning/phases/14-adoption-readiness-analysis-dispositions/deferred-items.md` — out-of-scope discovery log (cmd/ass-guard live-serve flake)

## Decisions Made

- **cache_control: routed, not fixed.** The emission fix is not shaper-local (profile TextBlock must gain the captured value first — 14-02 disposition #1's mechanism). The 14-03 plan's route-to-14-04 option was superseded by 14-02's committed disposition (14-04 is probe-only); destination = post-adoption queue.
- **No mechanical porting, no content borrowing.** All citations are file:line references; the shaper's agreement with the target is the capture's doing, not pi's.
- **pi's compat matrix = pi's generality, not target behavior.** All 11 compat flags classified justified: none of their wire effects appear in the zcode corpus (no eager_input_streaming, no strict, no fallbacks, no tool_reference, no temperature, no ttl).

## Deviations from Plan

None - plan executed exactly as written. (The zero-fix outcome of Task 3 is the plan's own designed path, not a deviation.)

## TDD Gate Compliance

Task 3 carries `tdd="true"`, and the MVP+TDD runtime gate's static predicate
(`task.is-behavior-adding` → `true`) was evaluated before its implementation step.
At execution time the gate's subject dissolved: the task's `<behavior>` contract is
"filled at execution time from the audit's fix list", and the audit's fix list
contains ZERO `divergence-fixed` rows — so no implementation step existed to gate.
The task resolved to the plan's explicit zero-fix path ("an empty fix list is a
legitimate audit result, not a skipped task"): doc-only finalization + battery.
Consequently no `test(14-03)`/`feat(14-03)` commits exist — by design, not by
omission. The no-test escape hatch (the behavior block's corpus_absent flag) was
never needed because no behavior was added; shaper sources are byte-identical
across the plan's commits (verified by empty git diff).

## Issues Encountered

- First `mise run ci` of the session failed 2 `cmd/ass-guard` live-serve tests
  (`TestServeAudit_RequestShapedThroughRealSeam`, `TestServeMirror_Override`).
  Both passed in isolation and in two subsequent full runs (final: exit 0, 26 pkgs
  ok) — the documented load-dependent timing flake (STATE.md 14-01 entry). Out of
  scope (14-03 touches zero Go files); logged to the phase deferred-items.md.

## User Setup Required

None - no external service configuration required. (The pi clone is already on
disk under `tools/pi-audit/`; re-clone per the audit header if absent.)

## Next Phase Readiness

- 14-04 (EARLY-03 cache probe) can consume the audit's cache_control rows: expected
  placement = system blocks only, every block, value `{"type":"ephemeral"}` —
  corpus wins on conflict (stated in both docs)
- 14-06 can extend `.gitignore` beside the `tools/pi-audit/` entry (scoped comment marks the adjacency)
- The post-adoption cache_control emission queue inherits the routed mechanism:
  profile TextBlock gains the captured value (TIER-1) → shaper maps onto
  `anthropic.TextBlockParam.CacheControl` (SDK field verified present, v1.63.0)
- Blockers: none

## Self-Check: PASSED

All key files exist on disk (docs/shaper-pi-audit.md, 14-03-SUMMARY.md,
deferred-items.md); all four plan commits verified in git log (165ac43, a8224e9,
805800f, 956414a); working tree clean (only the pre-existing untracked
`.planning/.gsd-allow-shrink` remains, untouched); requirements-completed carries
EARLY-04 verbatim.

---
*Phase: 14-adoption-readiness-analysis-dispositions*
*Completed: 2026-08-19*
