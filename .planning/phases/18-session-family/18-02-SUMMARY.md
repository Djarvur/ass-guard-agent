---
phase: 18-session-family
plan: 02
subsystem: acp-sessions
tags: [acp, session-load, reconciliation, transcript, kill9, ghost-state, resume, provenance, tdd]

# Dependency graph
requires:
  - "18-01 replay spine + session.MaxTurnCounter/SeedResume — the load path this engine feeds (18-05 wires them)"
  - "internal/session transcript Line envelope + Manager append discipline (marshal→redact→mutex→single-write)"
  - "Phase 17 ask machinery vocabulary — the cancelled-normal result family the ask closure mirrors (permissionCancelledForm)"
provides:
  - "session.Reconcile(sessionID, lines): pure classifier over the ten-row kill -9 Live-State Inventory — provenance-marked synthetic closures + Seed{MaxTurns, PlanMode, PlanModePresent}"
  - "session.InterruptedCause ('interrupted') — the D-02 provenance marker for every synthetic closure"
  - "Manager.AppendSynthetic(line) — the 18-05 append seam through the same redaction-disciplined appendLine (no second write path)"
  - "Nine hand-written inventory fixtures (class01..class09) under testdata/reconcile/ — the row-by-row contract checklist"
affects: [18-05-load-reconciliation, 18-06-resume-cli, ACP-06]

# Actuals (#2632) — chars/4 over the realized diff (44,476 diff chars), same scale as the plan's estimate
actuals:
  tokens: 11119
  tasks: 2
  commits: 2

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Read-only classifier + append-only closures: reconciliation never rewrites a transcript line; every fix for a dangling expectation is a NEW line with Cause=InterruptedCause (D-01/D-02)"
    - "Keyed pair matching, never adjacency: tool_result by ToolCallID, subagent_result by SubagentTurnID, ask resolution by the suspended ask's toolCallID — crafted collisions resolve deterministically (T-18-04)"
    - "Engine-authored closure payloads: constants only, never transcript-copied text (T-18-05) — a crafted transcript cannot inject content into closures"
    - "Opener-ordered emission: closures sort by the transcript index of the expectation's dangling opener (the turn's user_message for row 2, the tool_call/dispatch/ask/chunk line for rows 1/3/4/5); the session-level session_end closure is always last"

key-files:
  created:
    - internal/session/reconcile.go
    - internal/session/reconcile_test.go
    - internal/session/testdata/reconcile/class01_dangling_tool_call.jsonl
    - internal/session/testdata/reconcile/class02_turn_without_terminal.jsonl
    - internal/session/testdata/reconcile/class03_ask_suspended_unresolved.jsonl
    - internal/session/testdata/reconcile/class04_subagent_unresolved.jsonl
    - internal/session/testdata/reconcile/class05_open_chunk_stream.jsonl
    - internal/session/testdata/reconcile/class06_plan_mode_state.jsonl
    - internal/session/testdata/reconcile/class07_turn_sequence.jsonl
    - internal/session/testdata/reconcile/class08_request_without_usage.jsonl
    - internal/session/testdata/reconcile/class09_missing_session_end.jsonl
  modified:
    - internal/session/manager.go

key-decisions:
  - "Ask-suspended turns do NOT also get a row-2 canceled closure: ask_suspended IS the turn's terminal (the turn ended at the ask, 12-01) — the only dangling expectation it leaves is the tool_result for its toolCallID, closed cancelled-NORMAL (isError=false, permissionCancelledForm's family). This is also the class-10 answer: pending permission asks are class-3 machinery"
  - "Row 1 (dangling tool_call) vs row 3 (unresolved ask) disambiguation: the moment ask_suspended is on disk, its callID leaves row-1 scope — one closure per callID, never two"
  - "Row 5 fires only for turns with no terminal of any kind: a live-cancelled turn's chunks legitimately never get an assistant_message (WR-04) — the canceled terminal closes the stream too, so clean transcripts with cancelled mid-stream turns classify as clean (pinned in TestReconcileClean)"
  - "subagent_result is the subagent turn's terminal: completed subagent turns (which never carry an assistant_message — the nested loop returns textBuf straight into the result line) must not dangle (pinned in TestReconcileClean)"
  - "Row 9 co-fires in every kill -9 fixture by construction — a killed session has no session_end; each fixture's exact closure set includes it (fixtures stay realistic kill -9 transcripts rather than synthetic clean ones)"
  - "The max-turn scan is a DELIBERATE local duplicate of seed.go's MaxTurnCounter (same-wave artifact isolation); 18-05 Task 1 unifies the scanners — Reconcile's scan delegates to MaxTurnCounter so one implementation survives"
  - "Closure timestamps are load-time now(): classification itself is clock-free and pure; the timestamp is the only non-derivable field and records when the closure was actually written"

patterns-established:
  - "Pattern: inventory-row fixtures are the checklist — one hand-written JSONL per Live-State Inventory row, every line valid JSON (loader-enforced), ids embedding the fixture session's <sessionID>-turn-%03d shape"
  - "Pattern: idempotency is a first-class reconcile property — lines+closures fed back through Reconcile must classify as clean with an unchanged seed (second loads never duplicate closures)"

requirements-completed: [ACP-06]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "Pure reconciliation engine over the ten-row kill -9 inventory: nine table-driven fixture subtests asserting the exact ordered closure set (kinds, TurnID/ToolCallID/SubagentTurnID keying, IsError flags — failed row-1 results, cancelled-normal row-3 results — and Cause == InterruptedCause) plus seed values (MaxTurns max-scan with gaps, plan mode from the LAST plan_mode line); idempotent second pass (zero closures, stable seed); cleanly-closed transcripts (incl. live-cancelled chunk turns and completed subagent turns) return zero closures; torn garbage tail classifies over the conforming prefix via the reader contract"
    requirement: ACP-06
    verification:
      - kind: unit
        ref: "internal/session/reconcile_test.go#TestReconcile"
        status: pass
      - kind: unit
        ref: "internal/session/reconcile_test.go#TestReconcileIdempotent"
        status: pass
      - kind: unit
        ref: "internal/session/reconcile_test.go#TestReconcileClean"
        status: pass
      - kind: unit
        ref: "internal/session/reconcile_test.go#TestReconcileSkipsGarbageTail"
        status: pass
      - kind: other
        ref: "grep os./http. in internal/session/reconcile.go — none (zero I/O, D-01)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Manager.AppendSynthetic append seam for 18-05: routes through the same marshal→redact→mutex→single-write appendLine as every other append (redactor invoked exactly once — synthetic content crosses the redactor per 16-D-23; no O_TRUNC, no second write path), landing the closure on disk with its provenance Cause intact"
    requirement: ACP-06
    verification:
      - kind: unit
        ref: "internal/session/reconcile_test.go#TestAppendSynthetic"
        status: pass
      - kind: other
        ref: "mise ci (vet + golangci-lint v2 0 issues + CGO_ENABLED=0 build + go test -race ./...)"
        status: pass
    human_judgment: false

# Metrics
duration: 23min
completed: 2026-09-03
status: complete
---

# Phase 18 Plan 02: Reconciliation Engine Summary

**Pure transcript classifier for every dangling expectation a kill -9 mid-turn leaves on disk (ten-row inventory: failed tool_results, cancelled-normal ask closures, failed subagent_results, terminal assistant messages, canceled terminals, synthetic session_end) with D-02 provenance, keyed pair matching, and Seed{MaxTurns, PlanMode} — plus the redaction-disciplined AppendSynthetic seam 18-05 wires into the load path**

## Performance

- **Duration:** 23 min (11:38Z → 12:01Z)
- **Tasks:** 2/2
- **Files:** 12 (11 created, 1 modified)
- **Commits:** 2 (+ this metadata commit)

## Accomplishments
- `internal/session/reconcile.go`: `Reconcile` implements rows 1-9 of the Live-State Inventory in one ordered pass — dangling tool_call → failed tool_result (row 1), unresolved ask_suspended → cancelled-NORMAL tool_result for its toolCallID (row 3, class-10 machinery included), dangling subagent_dispatch → failed subagent_result keyed by subagentTurnID (row 4), open chunk stream → terminal assistant_message (row 5), unterminated user-turn → synthetic canceled (row 2), missing session_end → synthetic session_end (row 9). Rows 6/7/8 append NOTHING: `Seed.PlanMode` from the LAST plan_mode line (Pitfall 3), `Seed.MaxTurns` via max-scan (Pitfall 2), request_shaped/usage stays the audit-only pair. Zero I/O, zero sidecar (D-01); every closure carries `Cause = InterruptedCause` (D-02)
- `Manager.AppendSynthetic` in manager.go beside the other Append* one-liners — delegates to the existing `appendLine`, so closures cross the Redactor like every non-thinking append (16-D-23) and there is no second write path
- Nine hand-written fixtures under `internal/session/testdata/reconcile/` (class01..class09, one per inventory row, exact on-disk camelCase spellings, loader-enforced valid JSON); class10 needs no fixture — a test-file comment records the class-3 resolution (18-RESEARCH Q2)
- The battery pins the interaction rules the inventory implies: ask_suspended IS a turn terminal; subagent_result IS the subagent turn's terminal; a canceled terminal closes a chunk stream (live-cancel contract WR-04); row 9 co-fires in every kill -9 fixture

## Task Commits

Each task committed atomically (TDD RED→GREEN):

1. **Task 1 (RED): inventory fixture table + failing Reconcile tests**
   - `91622c7` test(18-02): add failing reconciliation inventory tests (compile-failure RED confirmed via the plan's verify command)
2. **Task 2 (GREEN): Reconcile engine + AppendSynthetic append seam**
   - `b7554e2` feat(18-02): implement reconciliation engine (lint-clean in the same commit — fixture builders + consts per the 18-01 c1472b5 pattern; no separate REFACTOR needed: the only duplication, the max-turn scan, is plan-mandated deliberate)

## Files Created/Modified
- `internal/session/reconcile.go` — InterruptedCause, Seed, Reconcile, scanTranscript/trackTurn/trackPair, opener-ordered closure emission, payload constants (T-18-05), local reconcileMaxTurn (deliberate seed.go duplicate, 18-05 unifies)
- `internal/session/reconcile_test.go` — TestReconcile (9 subtests), TestReconcileIdempotent, TestReconcileClean (inline clean fixtures incl. cancelled-chunk + subagent + plan-mode-enter/exit shapes), TestReconcileSkipsGarbageTail (reader contract), TestAppendSynthetic (redactor-crossing proof)
- `internal/session/manager.go` — AppendSynthetic beside the other Append* one-liners
- `internal/session/testdata/reconcile/class01..class09` — one fixture per inventory row

## Decisions Made
- **Ask turns and row 2:** ask_suspended is a turn terminal (12-01's "the turn ENDED at the ask") — the ask's ONLY dangling expectation is the tool_result, closed cancelled-normal (`isError=false`, the permissionCancelledForm sentence family: "Tool call cancelled: the session was interrupted while the ask was pending. Continue without this call's result."). No canceled closure for such turns (class03 pins: exactly `[tool_result, session_end]`)
- **Row-1 vs row-3 scope:** the ask's callID leaves row-1 scope when ask_suspended lands — one closure per callID, never two; map-keyed so crafted collisions resolve deterministically (T-18-04)
- **Terminals:** assistant_message (normal end), canceled, error, session_end (per the plan's additive wording), ask_suspended, and subagent_result (the subagent turn's terminal — sub turns never carry an assistant_message). Row 5 skips any terminal'd turn so live-cancelled chunk streams and completed subagent turns stay clean
- **Emission order:** closures sort by the transcript index of their dangling opener (turn's user_message for row 2; the tool_call/subagent_dispatch/ask_suspended/first-chunk lines for rows 1/4/3/5); the session_end closure is always last
- **TestAppendSynthetic added to the battery** (beyond the four named test functions): the plan's must_haves key_links make the redaction-disciplined append seam a shipped deliverable (T-18-05 mitigation) — the counting-redactor routing proof pins it; it rode the same RED/GREEN commits
- **REQUIREMENTS.md not flipped:** ACP-06 is declared by 18-01/02/05/06 — the shared-ID gate (#2388) correctly holds it until every declaring plan finishes; `requirements.ready-ids` returned 0/1 ready

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] countingRedactor redeclared**
- **Found during:** Task 2 GREEN (first compile of the battery)
- **Issue:** reconcile_test.go declared its own `countingRedactor` fake; transcript_newkinds_test.go already declares one (with a mutex-guarded `calls()` accessor) — duplicate symbol, build failure
- **Fix:** dropped the duplicate; reused the existing fake (the routing-proof semantics are identical)
- **Files modified:** internal/session/reconcile_test.go
- **Verification:** `go test -race -count=1 ./internal/session/` ok
- **Committed in:** b7554e2

**2. [Rule 3 - Blocking] golangci direct invocation panics (environment, not code)**
- **Found during:** Task 2 lint pass
- **Issue:** `golangci-lint run` invoked directly panics with "file requires newer Go version go1.27 (application built with go1.26)" — the system `go` on PATH is go1.27.1 while mise pins 1.26; the crash reproduces on untouched packages (./internal/acp/), pre-existing drift unrelated to this plan
- **Fix:** run lint through `mise lint` (the pinned toolchain) — exactly how `mise ci` invokes it; 0 issues after the battery cleanup
- **Files modified:** none (workflow, not code)
- **Verification:** `mise lint` → 0 issues; two consecutive `mise ci` runs green
- **Committed in:** n/a

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking/environment)
**Impact on plan:** No scope creep — both fixes were required to land GREEN at the standing gates.

## Verification Results

- RED (Task 1): `go test -race -count=1 ./internal/session/ -run TestReconcile` → build failure on the undefined engine symbols — RED-CONFIRMED per the plan's verify command; all 9 fixtures validated line-by-line as JSON
- GREEN (Task 2): `go test -race -count=1 ./internal/session/ && go vet ./internal/session/` — **ok / clean**
- `go test -race -count=1 ./internal/session/ -run TestReconcile` — **ok** (plan-level verification)
- `gofmt -l internal/session/` — clean; `mise lint` — **0 issues**
- `mise ci` — **green twice consecutively** (vet ~1s, lint 0 issues, CGO_ENABLED=0 build, full `-race` suite ~61s). One earlier invocation FAILed with no reproducible package (all packages `ok` in that log's tail); the identical suite then passed three times (direct `go test -race ./...`, and two full `mise ci` runs) — the same environmental under-load flake 18-01 documented, not reproduced and unrelated to this plan's code
- Acceptance greps: `InterruptedCause` / `Seed{MaxTurns, PlanMode, PlanModePresent}` / `func Reconcile(` present; no `os.`/`http.` in reconcile.go; `AppendSynthetic` body is exactly `return m.appendLine(line)`; no `O_TRUNC` anywhere in the package

## TDD Gate Compliance

RED (`91622c7` test) precedes GREEN (`b7554e2` feat) in git log — gate sequence compliant; RED failed for the right reason (undefined engine symbols, not a broken test).

## Known Stubs

None — no stubs, no skipped tests, every `<verify>` command ran.

## Threat Surface

No security-relevant surface beyond the plan's threat model. Both mitigated dispositions verified:
- **T-18-04** (replay poisoning): Reconcile is read-only classification; closures append-only; pair matching keyed (ToolCallID / SubagentTurnID / ask toolCallID), never adjacency
- **T-18-05** (closure content injection): payloads are engine-authored constants ("interrupted by process death", the cancelled-normal sentence) — never copied from transcript content; AppendSynthetic crosses the redactor (TestAppendSynthetic)
- **T-18-SC**: no new packages (stdlib only)

## Self-Check: PASSED

- Created files exist: reconcile.go, reconcile_test.go, manager.go, testdata/reconcile/class01..class09 (9 files) — FOUND
- Commits exist: 91622c7, b7554e2 — FOUND (git log)
- Acceptance criteria re-run post-commit: tests/vet/lint/ci green (see Verification Results)

## Next Phase Readiness

- The engine + append seam are ready for 18-05's wiring: `Runner.ResumeSession` calls `Reconcile` → `Manager.AppendSynthetic` per closure → `SeedResume(seed.MaxTurns)` + plan-mode seeding; 18-05 Task 1 unifies reconcileMaxTurn with seed.go's MaxTurnCounter
- BLOCKER: none. Note for 18-05: closures append BEFORE the D-03 map-insert (reconcile-then-accept ordering per 18-CONTEXT)

---
*Phase: 18-session-family*
*Completed: 2026-09-03*
