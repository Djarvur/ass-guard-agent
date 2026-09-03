---
phase: 21-context-policy-parity-closures
plan: "04"
subsystem: api
tags: [mentions, at-mention, prompt-expansion, read-rule, provenance, transcript, expanduserblocks, par-06, d-10]

# Dependency graph
requires:
  - phase: 08-opsx-closure
    provides: expandUserBlocks ingress seam + the ParseInvocation parse/registry split precedent (08-04)
  - phase: 16-acp-wire-foundation
    provides: the additive transcript-kind family (16-D-20) TypeMentionProvenance joins
provides:
  - ecosys.ParseMentions — pure, table-pinned @-token extraction (whitespace-delimited token-initial @, quote-aware spaced paths, trailing-punct trim, mid-word @ exclusion; IsDir unknown at parse)
  - TypeMentionProvenance transcript kind + Manager.AppendMentionProvenance — the D-10 provenance line (raw token + resolved path + form) in the AppendCommandProvenance discipline
  - Runtime mention expansion inside expandUserBlocks — Read-rule-gated @file content sections, one-level @dir listings (no recursion), fixed-form unresolvable notes, per-mention provenance, zero-mention byte-identical passthrough
  - The readRuleEvaluator Runner seam — nil = implicit allow for in-workspace files; absolute paths admitted only by an explicit ruling; documented as the internal/perm join point 21-06 wires
affects: [21-05 image ingress (the sibling PAR-06 leg rides the same seam), 21-06 perm rule-set join (consumes readRuleEvaluator), session/load replay (mention_provenance is an audit marker)]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
# Same estimateTokens scale (chars/4 over the realized diff), never a harness token count.
actuals:
  tokens: 10000   # 40,189 diff chars / 4 over internal/ (6 files, +1037/-21)
  tasks: 2
  commits: 5      # 4 task commits (RED+GREEN per task) + this docs commit

# Tech tracking
tech-stack:
  added: []       # stdlib only (os, path/filepath, strconv, strings) — no new deps
  patterns:
    - "Parse/decide split, second instance: ParseMentions in ecosys mirrors ParseInvocation — pure token extraction, resolution/gating/provenance stay in the runtime layer"
    - "Copy-on-write block pipeline: the command pass borrows the input slice and copies only on first change; the mention pass appends into the first text block on its own copied slice"
    - "One-consult gate: every @file consults readRuleEvaluator exactly once (absolute paths fold admission+gate into the single consult); nil = implicit allow — the seam 21-06 wires"

key-files:
  created:
    - internal/ecosys/mentions.go
    - internal/ecosys/mentions_test.go
    - internal/runtime/mention_expand_test.go
  modified:
    - internal/session/transcript.go
    - internal/session/manager.go
    - internal/runtime/runtime.go

key-decisions:
  - "Absolute @paths are admitted ONLY by an explicit evaluator ruling: the nil default resolves nothing outside the workspace root, so ingress can never serve as a whole-FS existence oracle (T-21-14's letter); the evaluator-admitted absolute path is the declared-roots half of the bound"
  - "Relative-path bound is the workspace root (workDir) checked via filepath.Rel before any stat — a ../ escape yields the outside-roots class without touching the filesystem outside"
  - "Notes name the token + the outcome class and nothing else (missing / outside the admitted roots / denied by the Read rules / could not be read) — fixed-form templates, per the flagged privacy prohibition; existence-class detail is the allowed maximum"
  - "Provenance is written per mention ATTEMPT (forms file|dir|denied|unresolved, resolved path empty when nothing answered) — the per-mention duty holds even when expansion fails, and the denial form makes the gate observable in the transcript"
  - "Dir listings are never Read-gated (the plan's letter: every @FILE consults); a one-level listing inside the admitted root exposes names+sizes only, which the bound already admits"

patterns-established:
  - "Second-pass seam extension: a new expansion family rides expandUserBlocks AFTER the existing command logic, operating on the (possibly expanded) text, no-op without its tokens — zero change to the command battery"
  - "Admission-before-stat resolution: bounds and evaluator admission run BEFORE os.Stat so an unadmitted path never probes the filesystem at all"

requirements-completed: [PAR-06]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "ParseMentions: pure parse-only @-token extraction — ordered, quote-aware spaced paths, trailing-punct trim, mid-word @ exclusion, empty contract, IsDir unknown at parse"
    requirement: PAR-06
    verification:
      - kind: unit
        ref: "internal/ecosys/mentions_test.go#TestParseMentions"
        status: pass
    human_judgment: false
  - id: D2
    description: "TypeMentionProvenance additive kind + AppendMentionProvenance mirroring AppendCommandProvenance (redacted append, pre-turn turnID, raw token / resolved path / form fields)"
    requirement: PAR-06
    verification:
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_FileSectionAndProvenance"
        status: pass
    human_judgment: false
  - id: D3
    description: "@file expansion: labeled section (raw-token header + verbatim content) in the first text block plus the file-form provenance line; quoted spaced paths resolve"
    requirement: PAR-06
    verification:
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_FileSectionAndProvenance"
        status: pass
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_SpacedQuotedPath"
        status: pass
    human_judgment: false
  - id: D4
    description: "@dir expansion: ONE-LEVEL listing — name + size in bytes, subdirectories with trailing slash, NO recursion (two-level fixture: the child dir's files never appear)"
    requirement: PAR-06
    verification:
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_DirListingOneLevel"
        status: pass
    human_judgment: false
  - id: D5
    description: "Read-rule gate observable end-to-end: denying evaluator -> loud denied note, no content section, denial-form provenance, consulted with tool=Read; nil default implicitly allows; evaluator-allowed absolute path admitted"
    requirement: PAR-06
    verification:
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_ReadRuleDenial"
        status: pass
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_DefaultEvaluatorImplicitAllow"
        status: pass
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_AbsolutePathAdmittedByEvaluator"
        status: pass
    human_judgment: false
  - id: D6
    description: "Unresolvable + outside-roots mentions: ONE fixed-form note naming token and outcome class, turn proceeds, unresolved-form provenance; ../ escapes and nil-evaluator absolute paths never stat outside the workspace (no FS oracle)"
    requirement: PAR-06
    verification:
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_UnresolvableNote"
        status: pass
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_OutsideAdmittedRoots"
        status: pass
    human_judgment: false
  - id: D7
    description: "Composition and safety controls: multi-mention token order (sections + provenance), zero-mention byte-identical passthrough, provenance-write failure loud-continues, registry-miss command text with a mention still expands, JSON round-trip sanity"
    requirement: PAR-06
    verification:
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_MultipleMentionsInOrder"
        status: pass
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_ZeroMentionsByteIdentical"
        status: pass
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_ProvenanceFailureContinues"
        status: pass
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_CommandAndMentionCompose"
        status: pass
      - kind: integration
        ref: "internal/runtime/mention_expand_test.go#TestMentionExpand_JSONRoundTripSanity"
        status: pass
    human_judgment: false

# Metrics
duration: 25 min
completed: 2026-09-03
status: complete
---

# Phase 21 Plan 04: @-Mention Expansion (PAR-06) Summary

**Parse-only @-mention extraction in ecosys plus runtime expansion riding the exact expandUserBlocks seam slash-commands use — Read-rule-gated file content sections, one-level dir listings, fixed-form unresolvable notes, and a per-mention provenance transcript line**

## Performance

- **Duration:** 25 min
- **Started:** 2026-09-03T20:02:07Z
- **Completed:** 2026-09-03T20:27:00Z
- **Tasks:** 2 (each RED -> GREEN; 4 task commits)
- **Files modified:** 6 (3 created, 3 modified)

## Accomplishments
- ParseMentions lands as the second instance of the expand.go parse/decide split: a pure, table-pinned tokenizer (whitespace-delimited token-initial @, quote-capturing spaced segments with path tails, trailing `. , :` trim, mid-word/email @ exclusion) with IsDeliberately-unknown IsDir — no I/O, no resolution, no errors
- The runtime mention pass extends expandUserBlocks AFTER the command logic with zero behavior change for mention-free prompts (pinned byte-identical): each mention resolves bounded to the workspace root, gains a labeled content/listing section or a fixed-form loud note, and writes one AppendMentionProvenance line with the :426-434 loud-continue failure discipline
- The Read-rule gate is observable, not implied: every @file consults the injected readRuleEvaluator (tool="Read") before content enters the prompt — a denying evaluator yields a loud denied note, no content section, and a denial-form provenance line; nil implicitly allows in-workspace files while admitting NO absolute path (no whole-FS existence oracle — the 21-06 join wires internal/perm's rule set into the documented seam)
- One-level @dir listings pin D-10's no-recursion letter with a two-level fixture (the child dir's files never appear); unresolvable and outside-roots mentions each produce exactly ONE note naming the token and the outcome class, and the turn always proceeds

## Task Commits

Each task was committed atomically (TDD: failing test first):

1. **Task 1: ParseMentions — parse-only extraction** — `c0ace2f` (test) + `053f0d2` (feat)
2. **Task 2: Runtime expansion — gate, sections, provenance** — `0b4eec1` (test) + `25ef06d` (feat)

**Plan metadata:** this commit (docs)

## Files Created/Modified
- `internal/ecosys/mentions.go` — Mention + ParseMentions + the quote-aware scanner (parse-only; the ParseInvocation split precedent)
- `internal/ecosys/mentions_test.go` — the parse table (20 rows: order, quoting, punctuation, mid-word, empties)
- `internal/session/transcript.go` — TypeMentionProvenance in the additive-kind family (audit marker; Projector-inert)
- `internal/session/manager.go` — AppendMentionProvenance (redacted append; Name/CommandRef/Text = token/resolved/form)
- `internal/runtime/runtime.go` — readRuleEvaluator Runner field (the 21-06 seam) + expandMentions/resolveMention/readAllows/mentionNote/mentionFileSection/mentionDirSection; expandUserBlocks restructured copy-on-write with the command pass's behavior preserved verbatim
- `internal/runtime/mention_expand_test.go` — the 13-test expansion battery over t.TempDir trees

## Decisions Made
- Absolute @paths require an explicit evaluator allow to be admitted at all (nil admits nothing outside the workspace); admission runs BEFORE os.Stat, so an unadmitted path never probes the filesystem — the strictest honest reading of the privacy prohibition, and the shape 21-06's rule set completes
- Provenance is per mention attempt, not per success: unresolved and denied mentions also write their line (form carries the outcome), so the transcript answers "what did this @token turn into" for every token
- Dir listings are not Read-gated (the plan's letter consults on @file); the workspace bound already governs what a listing can name
- The section/note rendering (bracketed raw-token headers, `(N bytes)` listing lines, note class wording) is test-pinned discretion — 21-CONTEXT assigns @-parse details to Claude's discretion and the corpus carries no @-mention ground truth to mirror

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Go 1.27 forbids t.Setenv in parallel tests**
- **Found during:** Task 2 (first GREEN run)
- **Issue:** The battery fixture pinned HOME via t.Setenv while the tests called t.Parallel() — Go 1.27.1 panics on that combination, failing the whole run before any assertion.
- **Fix:** Dropped the HOME pin — the mention pass never consults HOME (no registry discovery runs on the bare Runner fixture), so the pin was dead weight; parallelism stays.
- **Files modified:** internal/runtime/mention_expand_test.go
- **Verification:** `go test -race ./internal/runtime/ -run TestMentionExpand -count=1` green
- **Committed in:** 25ef06d (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (test-infrastructure blocker)
**Impact on plan:** None on scope or contracts — the plan's truths all hold as written.

## Known Limitations
- **No content-size cap on @file expansion:** D-08's per-file/total caps govern memory injection only; the plan pinned no cap for mentions, so an @mentioned file enters whole (Read-rule gate applies, size does not). If context blowups surface in practice, a cap belongs in a follow-up alongside 21-05's image size discipline.
- **First-text-block scope only:** mentions parse from the first text block (the seam's scope, matching slash-commands); @tokens in later text blocks pass through unexpanded.

## Issues Encountered
- `TestAdvisoryWiring_QuestionEndingNote` (internal/runtime) flaked ONCE under full-package `go test -race -count=2` load (34.69s wall); 5/5 green in isolation and green in every -count=1 full run. Pre-existing load/timing sensitivity in the real-acp.Server pipe battery (same family 21-03 ledgered), provably unrelated to this plan — the test's prompts carry no @ tokens, so the mention pass is a no-op for them. Logged in deferred-items.md under 21-04.
- `golangci-lint run` red on the pre-existing toolchain drift: v2.12.2 cannot typecheck the Go 1.27.1 stdlib itself (crypto/internal/randutil / math/rand/v2 typecheck errors) — the only 2 reported issues are stdlib-internal; ZERO findings in any repo file, including everything this plan touched.

## TDD Gate Compliance

Both tasks executed RED -> GREEN with per-phase commits:
- RED: `c0ace2f` (Task 1), `0b4eec1` (Task 2) — each verified failing first (build failures on the undefined new symbols: ecosys.Mention / session.TypeMentionProvenance / Runner.readRuleEvaluator)
- GREEN: `053f0d2` (Task 1), `25ef06d` (Task 2) — each verified passing after (Task 2's fixture blocker fixed inside the GREEN iteration, see Deviations)
- No REFACTOR phase needed (the GREEN shapes landed clean; no post-green cleanup commits)

## Self-Check: PASSED

- Created files exist: internal/ecosys/mentions.go, internal/ecosys/mentions_test.go, internal/runtime/mention_expand_test.go — FOUND
- All task commits exist in git log: c0ace2f, 053f0d2, 0b4eec1, 25ef06d — FOUND
- Task verifies: `go test -race ./internal/ecosys/ -run TestParseMentions -count=1` + `go vet ./internal/ecosys/` — PASS; `go test -race ./internal/runtime/ -run TestMentionExpand -count=1` + `go test ./internal/session/ ./internal/runtime/ -count=1` + `go vet ./internal/runtime/ ./internal/session/` — PASS
- Plan `<verification>`: `go test -race ./internal/ecosys/ ./internal/runtime/ ./internal/session/ -count=1` — PASS (green on 3 consecutive runs); full-repo `go test -race -count=1 ./...` — PASS (all 38 packages)
- `mise ci` — vet green, CGO_ENABLED=0 build green, test green; lint red = pre-existing golangci-lint/Go-1.27.1 stdlib typecheck drift with zero repo-file findings (documented above)
- Acceptance grep: `grep -c 'TypeMentionProvenance' internal/session/transcript.go` = 2 (>= 1)

## Next Phase Readiness
- The @-mention leg of PAR-06 is live end-to-end on both turn paths (engine-off via Run, engine-on via the adapter's Expand — the same expandUserBlocks)
- 21-05 (image leg) rides the same ingress seam with the D-09 pure-Go downscale; 21-06 wires internal/perm's rule set into readRuleEvaluator — the seam comment names the contract (RuleSet.Evaluate: VerdictDeny -> deny, else allow; one consumption site, never a second gate pipeline)
- mention_provenance joins the additive-kind family as an audit marker: session/load replay shows it as metadata beside the expanded user message, exactly like command_provenance

---
*Phase: 21-context-policy-parity-closures*
*Completed: 2026-09-03*
