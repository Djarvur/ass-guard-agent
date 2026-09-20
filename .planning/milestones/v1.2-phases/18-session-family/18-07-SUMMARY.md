---
phase: 18-session-family
plan: 07
subsystem: acp-sessions
tags: [session-transcript, session-start-opener, list-sessions, resume-picker, g-18-1, gap-closure, acp-05, jsonl, tdd]

# Dependency graph
requires:
  - "18-03 list engine — session.ListSessions header scan (readHeaderOpener/conformingOpener), the surface G-18-1 blinded"
  - "18-05 full resume — Runner.ResumeSession → sessionFor reopen path + the kill-9 harness (real serve path)"
  - "18-06 CLI trio — --resume picker + cwd-store resolver rendering what listing returns"
  - "18-UAT G-18-1 — the operator-diagnosed root cause (AppendSessionStart zero production callers) + fix_direction"
provides:
  - "Production session_start opener: sessionFor writes AppendSessionStart(sessionID) as the transcript's FIRST line behind a size-0 gate — covers primary AND temp-fallback managers, resume-safe (no second opener), kill-9 self-healing, loud-degrade on failure (AUD-03)"
  - "Two-tier opener tolerance in listing: preferred session_start + knownOpenerType whitelist legacy fallback — pre-fix transcripts (user_message first line) enumerate forever; unknown type / zero timestamp / corrupt / empty still skip (T-18-07-01 bounded whitelist)"
  - "Real-shape pins on both sides: TestSessionListLegacyShapeOpener (the ~/tmp/perm-uat shape) + TestSessionForWritesOpener (serve path, reopen, self-heal) — the fixture-fiction failure mode (G-17-1 class) closed for listing"
affects: [18-session-family, verify-work UAT Test 2 re-run, any future phase touching transcript shape, listing, or the resume picker]

# Actuals (#2632) — chars/4 over the realized diff (18,198 diff chars on internal/, +386/−15 over 4 files), same scale as the plan's estimate
actuals:
  tokens: 4549
  tasks: 2
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Creation-only line kinds: an opener that must be the transcript's FIRST line can only be written when the file has zero bytes — the size gate IS the resume-safety and the kill-9 self-heal, one check for both"
    - "Two-tier opener contract: preferred session_start + legacy known-type fallback over a whitelist mechanically mirrored from the Type* vocabulary (transcript.go) — bounded tolerance, never accept-anything"
    - "Fiction-fixture guard as documentation: the conformingOpener comment cites G-18-1 and the G-17-1 'tests modeled a fiction' lesson so the next planner does not re-tighten the tolerance"

key-files:
  created:
    - internal/runtime/session_opener_test.go
  modified:
    - internal/runtime/runtime.go
    - internal/session/list.go
    - internal/session/list_test.go

key-decisions:
  - "Size gate semantics: os.Stat succeeds + size 0 → append the opener; stat error → loud log, continue (never guess); size > 0 → nothing (resume reopens the same file through sessionFor). All under sessMu (race-free), covering whichever manager survived the temp-fallback chain"
  - "knownOpenerTypes mirrors ALL Type* constants (session_start included): the preferred tier is checked first so behavior is unchanged for conforming transcripts, and the whitelist stays an honest mirror of the reader vocabulary instead of a curated subset someone must remember to extend"
  - "The legacy title consequence is documented IN CODE, not fixed: a legacy single-prompt session shows '(no prompt)' and a multi-prompt legacy session titles from its SECOND prompt — the UAT's operator-approved 'title scan unchanged' direction; readHeaderTitle/userTitleLine/titleOfLine are byte-identical to pre-plan (verified)"
  - "Lint polish on the RED pins landed as its own style(18-07) commit between test and fix — zero assertion changes, so the RED proof stays honest for the committed test state and bisect stays clean"

patterns-established:
  - "Real-shape pinning discipline: every new transcript fixture family must include at least one pin built from the shape the PRODUCTION path actually writes (drive sessionFor, read the file), not only hand-written conforming lines"

requirements-completed: [ACP-05, ACP-06]

coverage:
  - id: D1
    description: "Real-shape transcripts (user_message first line) enumerate through ListSessions with createdAt from that first line; single-prompt legacy fallback title pinned; unknown-type and zero-timestamp first lines still skip"
    requirement: ACP-05
    verification:
      - kind: unit
        ref: internal/session/list_test.go#TestSessionListLegacyShapeOpener
        status: pass
    human_judgment: false
  - id: D2
    description: "The serve path writes the session_start opener as the transcript's FIRST line (sessionID in Text), never duplicates it on reopen, and self-heals a zero-byte transcript"
    requirement: ACP-05
    verification:
      - kind: unit
        ref: internal/runtime/session_opener_test.go#TestSessionForWritesOpener
        status: pass
    human_judgment: false
  - id: D3
    description: "The existing battery stays green under -race with opener-carrying serve-path transcripts — kill-9 harness, reconcile/resume pins, acp session-family, cmd picker/resume-flag batteries, full repo suite"
    verification:
      - kind: integration
        ref: internal/acpserve/kill9_test.go#TestKill9Resume
        status: pass
      - kind: command
        ref: "go test -race ./internal/session/ ./internal/runtime/ ./internal/acp/ ./internal/acpserve/ ./cmd/ass-guard/ -count=1"
        status: pass
      - kind: command
        ref: "mise test (full repo -race: 38 ok / 0 FAIL) + mise vet + mise build (CGO_ENABLED=0)"
        status: pass
    human_judgment: false
  - id: D4
    description: "Operator leg: `ass-guard --resume` in ~/tmp/perm-uat renders the numbered picker listing the two live transcripts, and a fresh serve-path session shows a session_start first line on disk (UAT Test 2 re-runnable)"
    requirement: ACP-05
    verification: []
    human_judgment: true
    rationale: "Plan §verification item 5 assigns this leg to the orchestrator's verify-work / operator UAT re-run — it needs the real binary in the operator's terminal and on-disk store, which no in-plan test can substitute."

# Metrics
duration: 21 min
completed: 2026-09-05
status: complete
---

# Phase 18 Plan 07: G-18-1 Gap Closure Summary

**session_start opener wired into real session creation (size-gated, resume-safe) + known-type legacy fallback in ListSessions — `ass-guard --resume` enumerates real transcripts again instead of "no sessions to resume"**

## Performance

- **Duration:** 21 min
- **Started:** 2026-09-05T21:52:14Z
- **Completed:** 2026-09-05T22:13:15Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Root cause G-18-1 closed from both sides: the WRITE side (sessionFor calls `mgr.AppendSessionStart(sessionID)` — the production caller that was missing entirely) and the READ side (readHeaderOpener accepts any known-type first line with a valid timestamp as the legacy createdAt fallback).
- Resume safety and kill-9 self-heal come from the same size-0 gate: a transcript with bytes never gets a second opener; a zero-byte transcript (kill -9 between create and first append) gets the opener on the next sessionFor.
- The list battery now pins the shape real sessions actually produce — the G-17-1 fixture-fiction failure mode ("tests modeled a fiction") is closed for listing, with in-code documentation citing the lesson so the tolerance is not re-tightened.
- Threat-model mitigations implemented as planned: T-18-07-01 (bounded whitelist — unknown type / zero timestamp / corrupt / empty still skip, pinned) and T-18-07-03 (size gate preserves append-only; opener carries only the pattern-validated id and crosses the Redactor like every append).

## RED Proof (Task 1, verbatim)

Captured against the pre-fix tree at commit `61a18ea` — both runs FAIL exactly where the plan requires (real-shape enumeration subtests + the fresh-construction opener subtest; the still-skipped subtests already passed, as the plan expected):

```
=== go test ./internal/session/ -run 'TestSessionListLegacyShapeOpener' -count=1 ===
--- FAIL: TestSessionListLegacyShapeOpener (0.00s)
    --- FAIL: TestSessionListLegacyShapeOpener/real_shape_lists (0.00s)
        list_test.go:558: real shape: got [], want [00000000-0000-0000-0000-000000000002]
    --- FAIL: TestSessionListLegacyShapeOpener/single-prompt_legacy_title (0.00s)
        list_test.go:582: single-prompt legacy: got [], want [00000000-0000-0000-0000-000000000003]
FAIL
FAIL	github.com/Djarvur/ass-guard-agent/internal/session	0.527s
FAIL
exit=1
=== go test ./internal/runtime/ -run 'TestSessionForWritesOpener' -count=1 ===
--- FAIL: TestSessionForWritesOpener (0.00s)
    --- FAIL: TestSessionForWritesOpener/fresh_construction_writes_opener_first (0.15s)
        session_opener_test.go:98: transcript is empty — the session_start opener was not written at creation (G-18-1)
    --- FAIL: TestSessionForWritesOpener/zero-byte_transcript_self-heals (0.15s)
        session_opener_test.go:172: self-heal wrote 0 lines, want exactly the session_start opener first
    --- FAIL: TestSessionForWritesOpener/reopen_appends_no_second_opener (0.15s)
        session_opener_test.go:139: session_start lines = 0, want exactly 1 (append-only: reopen never duplicates the opener)
FAIL
FAIL	github.com/Djarvur/ass-guard-agent/internal/runtime	0.739s
FAIL
exit=1
```

## Task Commits

1. **Task 1: RED — pin the real transcript shape from both sides** - `61a18ea` (test)
2. **Lint polish on the RED pins (no assertion changes)** - `d56c20d` (style)
3. **Task 2: GREEN — write the opener at creation, tolerate it at listing** - `2a3cea7` (fix)

## Files Created/Modified

- `internal/runtime/runtime.go` - sessionFor: size-gated `AppendSessionStart(sessionID)` after the NewManager fallback chain, loud-degrade on stat/append failure (AUD-03), under sessMu
- `internal/session/list.go` - `knownOpenerTypes`/`knownOpenerType` + two-tier `conformingOpener`; doc comments on the file header, `readTranscriptHeader`, `readHeaderOpener`, `conformingOpener` describe preferred-plus-fallback semantics and cite G-18-1
- `internal/session/list_test.go` - `TestSessionListLegacyShapeOpener`: real-shape listing (createdAt from line 1 exactly), single-prompt legacy title, still-skipped unknown-type and zero-timestamp pins with conforming tails
- `internal/runtime/session_opener_test.go` - `TestSessionForWritesOpener`: fresh-construction first-line assertion, reopen-no-second-opener (count == 1 across the whole file), zero-byte self-heal — built on the session_fallback_test.go minimal-Runner template

## Verification Log

- Plan verify battery: `go test -race ./internal/session/ ./internal/runtime/ ./internal/acp/ ./internal/acpserve/ ./cmd/ass-guard/ -count=1` — all five packages ok (re-run after lint polish, and again on HEAD).
- `TestKill9Resume` explicitly green (`-run TestKill9Resume -count=1 -race`: PASS, all subtests) — the harness builds sessions through the real serve path, so its transcripts now carry the opener; its pairing/substring assertions are unaffected (knock-on 3 confirmed).
- Full repo suite via `mise test`: 38 ok / 0 FAIL. `mise vet`: clean. `mise build` (CGO_ENABLED=0): clean.
- Shape gates: `grep -c knownOpenerType internal/session/list.go` = 7 (≥ 2); `grep -c AppendSessionStart internal/runtime/runtime.go` = 1 (≥ 1 — the production caller that was missing); `readHeaderTitle`/`userTitleLine`/`titleOfLine` byte-identical to pre-plan (diff-verified against `61a18ea^`).
- Lint: `mise lint` remains red on the documented pre-existing drift (2925 issues, dominated by 2722 exhaustruct_v5). Touched-file diff against the pre-plan baseline: 0 new non-endemic issues; the only new entries are exhaustruct_v5 on test composite literals — identical warnings already sit on the file-local helpers and the session_fallback_test.go template this plan reused (repo convention: no exhaustruct nolints, 0 uses repo-wide).

## Decisions Made

See key-decisions in the frontmatter; nothing beyond the plan's own direction was decided.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Lint polish commit between RED and GREEN**
- **Found during:** Task 2 GREEN ladder (strict-lint conformance of the new code)
- **Issue:** The Task 1 pins as first written carried six new lint findings under the strict config (lll x2, SplitSeq modernize, emptyStringTest, gocognit/cyclop/funlen on the parent test, gochecknoglobals on the whitelist, builtinShadow `real`).
- **Fix:** Pure formatting/naming polish (SplitSeq, `b == ""`, `real`→`live`, nolint directives in the repo's established spelling, shortened messages); zero assertion changes. Landed as `d56c20d` (style) before the fix commit so the TDD sequence stays bisectable.
- **Files modified:** internal/runtime/session_opener_test.go, internal/session/list_test.go (+ the knownOpenerTypes nolint in internal/session/list.go, committed with the fix)
- **Verification:** Normalized lint diff vs pre-plan baseline shows 0 new non-endemic issues; both pin batteries re-run green after the polish.
- **Committed in:** d56c20d (test files), 2a3cea7 (list.go nolint)

---

**Total deviations:** 1 auto-fixed (1 blocking/lint-conformance)
**Impact on plan:** None on behavior or scope — the RED proof was captured from the committed RED state before any polish; all assertions unchanged.

## Issues Encountered

None beyond the deviation above. One compile fix during RED authoring (`json.Unmarshal([]byte(b), ...)` — `strings.Split` yields strings) landed before the RED commit and RED output capture.

## Known Stubs

None — no stubs, no skipped tests, no unrun verify commands.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- G-18-1 is code-closed; the operator leg (re-run UAT Test 2 in ~/tmp/perm-uat: picker must list the two live transcripts; a fresh serve-path session must show a session_start first line on disk) is owned by the orchestrator's verify-work per plan §verification item 5.
- Out-of-scope observation recorded in the plan's knock-on 6 (NOT fixed here, by design): `AppendSessionEnd` also has zero production callers — Reconcile's synthetic session_end closure already covers the resume story (18-02 shipped + tested).
- AppendSessionEnd remains available for any future clean-shutdown work; no action needed for listing.

## Self-Check: PASSED

- Created file exists: internal/runtime/session_opener_test.go — FOUND
- Commits found: 61a18ea (test), d56c20d (style), 2a3cea7 (fix) — FOUND in `git log`
- TDD gates: RED (`test(18-07)`) precedes GREEN (`fix(18-07)`) — sequence verified
- All plan verify commands green; grep gates pass; title trio byte-identical

---
*Phase: 18-session-family*
*Completed: 2026-09-05*
