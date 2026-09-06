---
phase: 19-compaction-cache-control
plan: "04"
subsystem: session
tags: [compaction, threshold, summarizer, overflow-retry, tdd]

# Dependency graph
requires:
  - phase: 19-compaction-cache-control plan 19-02
    provides: provider.IsOverflow (the message-class matcher over *ProviderError)
  - phase: 19-compaction-cache-control plan 19-03
    provides: AppendCompaction(summary) + the Projector's CompactionTailBudget / SetCompactionTailBudget seam
provides:
  - The compaction engine: inclusive threshold math over the D-01 usage read model, estimateSinceLastRequest (transcript content since the last request_shaped line / 4), the blocking same-pipeline summarizer (D-07/D-08/D-10) with the D-09 warning-counter degrade, the loop-head pre-request check (D-02), overflow retry-once with a per-turn guard (criterion 3), and the public CompactNow() entry (D-11)
  - Session.SetCompactionSettings live-apply setter wiring the 19-03 projector budget from the same context window
affects: [19-05 (config keys + live-apply wiring), 20 (/compact class-B handler rides CompactNow), PAR-01 verification]

# Actuals (#2632) — pairs with the plan's estimate (40000) to calibrate future estimates.
actuals:
  tokens: 15000   # chars/4 over the realized diff (60126 diff chars)
  tasks: 3
  commits: 5

tech-stack:
  added: []   # stdlib only
  patterns:
    - "same-pipeline summarizer: Provider.Stream on a profile COPY with a private chunk consumer — zero bus publishes, capturer attribution and adapter classification come free"
    - "warning-counter degrade family: one structured stderr line + one atomic counter per failure (the counter IS the countable warning record the tests pin)"
    - "per-turn retry guard as a runTurn local: never reset across tool-loop iterations by construction, no cross-turn state to clean"

key-files:
  created:
    - internal/session/compaction.go
    - internal/session/compaction_test.go
  modified:
    - internal/session/session.go

key-decisions:
  - "The compacting note degraded to the warning-counter family per the plan's pre-authorized path: the landed session/update vocabulary has no status frame and agent_message_chunk is exactly what PAR-01's bus-isolation prohibition bars (gap recorded in deferred-items.md)"
  - "The overflow retry and CompactNow bypass BOTH the threshold and the enabled gate (the manual-intent class, D-11's letter); only the pre-request check honors enabled — disabled settings are byte-identical to pre-phase"
  - "The summarizer model rides Session.SubagentModel (the tiers-table light slug the runtime already stamps at sessionFor — the resolveSubagentModel precedent); empty keeps the parent model (A4's documented default); the 2048 max_tokens cap and the model override ride the per-call profile copy"
  - "E2E shaped to 19-03's pinned position rule: the marker serves turns that START after it — the compacting turn completes on its own (mechanical) window and the NEXT turn projects the summary seed; the producing-turn re-seed carve-out is recorded as a deferred architectural item, not taken unilaterally (Rule 4)"
  - "Marker pointers are line-count-indexed opaque strings (pre = last absorbed line, post = the marker itself); the usage snapshot records the trigger anchor (lastInputTokens) + the summarizer's own output"

requirements-completed: [PAR-01]

coverage:
  - id: C1
    description: "Threshold engine: inclusive boundary math, estimate over transcript content since the last request_shaped line (never the request's own Bytes), D-01 in-memory usage read model, engine fire/no-fire"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/compaction_test.go#TestCompaction_Threshold"
        status: pass
    human_judgment: false
  - id: C2
    description: "Blocking same-pipeline summarizer: D-08 chaining over the previous marker's summary, light-tier profile copy (2048 cap, no write-back), private stream consumer, own AppendUsage + AppendCompaction marker, D-09 warning-counter degrade on error/timeout/empty-summary, zero client-visible bus publishes"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/compaction_test.go#TestCompaction_SummarizerFailure"
        status: pass
      - kind: unit
        ref: "internal/session/compaction_test.go#TestCompaction_Chaining"
        status: pass
    human_judgment: false
  - id: C3
    description: "Loop-head wiring (D-02): the check runs on every maxIterations iteration when enabled, skips entirely when disabled (zero checks, zero notes)"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/compaction_test.go#TestCompaction_LoopHead"
        status: pass
    human_judgment: false
  - id: C4
    description: "Overflow retry-once (criterion 3): force-compact + exactly one resend recovers the turn; a second overflow fails through the existing appendError path with no further retry (Pitfall 8 per-turn guard)"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/compaction_test.go#TestCompaction_OverflowRetryOnce"
        status: pass
    human_judgment: false
  - id: C5
    description: "CompactNow (D-11): manual entry compacts regardless of threshold/enabled gates, same marker shape (summary, turn attribution, anchor snapshot, pre/post pointers)"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/compaction_test.go#TestCompactNow"
        status: pass
    human_judgment: false
  - id: C6
    description: "Offline end-to-end: over-threshold session compacts at the next pre-request check, next turn projects the summary seed with a pair-safe tail, D-10 attribution on disk (request_shaped + own usage lines), headroom holds across subsequent turns, no-marker replay identity engine on vs off"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/compaction_test.go#TestCompaction_EndToEnd"
        status: pass
    human_judgment: false

# Metrics
duration: 37min
completed: 2026-09-06
status: complete
---

# Phase 19 Plan 04: Compaction Engine — Threshold, Summarizer, Retry-Once, CompactNow Summary

**PAR-01's engine assembled on the 19-03 semantics: threshold-triggered blocking compaction with real-usage measurement (D-01), a same-pipeline summarizer with loud degradation (D-07/D-08/D-09/D-10), loop-head wiring on every request boundary (D-02), overflow retry-once (criterion 3), and the public CompactNow() entry (D-11)**

## Performance

- **Duration:** 37 min
- **Started:** 2026-09-06T21:43:24Z
- **Completed:** 2026-09-06T22:20:18Z
- **Tasks:** 3 (Tasks 1-2 RED→GREEN under TDD mode; Task 3 the composition proof, committed green per its action spec)
- **Files:** 3 (2 created, 1 modified)

## Accomplishments
- `internal/session/compaction.go`: the engine — `SetCompactionSettings` live-apply holder (wires the 19-03 `SetCompactionTailBudget` seam from the same context window), inclusive `overThreshold` math, `estimateLinesSinceLastRequest` (folded line bytes / 4 since the last request_shaped line — envelope overhead nets conservative, never the request's own Bytes), the blocking `compact` seam (D-08 chained span render through `foldExchanges`, extractive prompt with the 2048 cap, light-tier profile copy, private stream consumer, own `AppendUsage` + `AppendCompaction` marker, D-09 degrade with a bounded 60s summarizer timeout), and `CompactNow`
- `internal/session/session.go`: the D-01 in-memory `lastInputTokens` read model at the stream loop's usage case (Pitfall 3 — the bus publish and transcript line stay the audit record), the D-02 loop-head check between the ctx check and `Projector.Project`, and the criterion-3 overflow intercept before the appendError return with a per-turn `overflowRetried` guard
- The offline proof battery: 6 test functions / 20 subtests covering threshold math (boundary, estimate sources, engine fire/no-fire), D-09 degrade (mid-stream error, sync error, timeout, empty summary), chaining (empty-span re-compact, previous summary preserved, prompt extractive, profile-copy discipline, A4 parent-model default, zero client-visible publishes), loop-head per-iteration counting + disabled skip, overflow recovery + fail-twice bounds (exactly 3 stream calls), CompactNow marker shape, and the end-to-end composed cycle
- Battery + package clean under golangci-lint 2.13.2 beyond the documented renamed-linter (`exhaustruct_v5`) noise; remaining session.go findings are the pre-plan baseline trio (verified by linting commit 4829451 in a temp worktree)

## Task Commits

Each behavior-adding task followed RED→GREEN with per-phase commits:

1. **Task 1: Threshold math, blocking summarizer, marker write, D-09 degrade** — `46a4906` (test, RED) + `efa598b` (feat, GREEN)
2. **Task 2: Loop-head wiring, overflow retry-once, CompactNow, compacting note** — `3c2c490` (test, RED) + `78a043f` (feat, GREEN)
3. **Task 3: Offline end-to-end proof** — `93d81ba` (single green commit per the task's action spec; the battery pins the composed behavior and drove the lint cleanups)

## Files Created/Modified
- `internal/session/compaction.go` — NEW: the engine (settings holder + setter, threshold helpers, estimate, compact, degrade, CompactNow, summarizer profile, extractive prompt, span renderer)
- `internal/session/compaction_test.go` — NEW: the scripted `compactionProvider` (usage chunks, mid-stream errors, hang leg, production-faithful RequestShaped turn attribution) + the six-test battery
- `internal/session/session.go` — compaction field group (settings, atomic read model + counters, test timeout), the usage-case read model assignment, the loop-head check, the overflow intercept

## Decisions Made
- **Compacting note degrade (documented gap):** the landed session/update vocabulary has no status frame; the only text frame (`agent_message_chunk`) is what PAR-01's bus-isolation prohibition bars from the compaction path. Degrade taken per the plan's pre-authorized path: one structured stderr line + one `compactionNotes` counter per compaction start (never a new wire frame). Gap recorded in `deferred-items.md`
- **Retry/manual bypass both gates:** the overflow retry and CompactNow call `compact` directly (the manual-intent class); only `maybeCompact` (the pre-request check) honors the enabled flag — the disabled path is structurally inert, proven by zero checks/notes/extra provider calls
- **Summarizer model source:** `Session.SubagentModel` (the tiers-table light slug the runtime already resolves at sessionFor via `resolveSubagentModel`); empty = parent model (A4). Keeps 19-04 inside its file scope; 19-05's config wiring can revisit
- **E2E shaped to 19-03's pinned position rule:** the marker serves turns that START after it (test-enforced), so the compacting turn completes on its own mechanical window and the NEXT turn carries the seed. The producing-turn re-seed carve-out (a same-turn, TurnID-keyed marker serving when no pre-user marker exists — subagent-safe by construction) was identified, analyzed against every 19-03 pin, and NOT taken: it is an architectural change against a pinned cross-plan contract (Rule 4). Recorded in `deferred-items.md` with the full analysis
- **Marker shape:** usage snapshot = the trigger anchor (`lastInputTokens`) as input + the summarizer's own output; pre/post pointers are line-count-indexed opaque strings (16-02 kept them opaque; the E2E asserts presence and turn attribution)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Task 2's TestCompactNow RED fixture starved the summarizer**
- **Found during:** Task 2 RED
- **Issue:** the fixture's single script entry was consumed by the Prompt turn's own send, so CompactNow's summarizer got the empty default script and degraded (empty summary) — the test failed for a fixture reason, not the missing wiring
- **Fix:** two script entries (the turn's send + the summarizer's); also made the fake's RequestShaped carry production-faithful turn attribution (a `turnIDOf` seam mirroring the capturer closure) so the D-10 on-disk attribution assertion is meaningful
- **Files modified:** internal/session/compaction_test.go
- **Verification:** RED then failed only on the missing loop-head/retry wiring (the right reasons); GREEN passed under -race
- **Committed in:** 3c2c490

**2. [Rule 3 - Blocking] lint gate: new files carried findings under golangci-lint 2.13.2**
- **Found during:** Task 3 verification (`mise ci` lint leg)
- **Issue:** per the 19-03 precedent, new code must stay clean under 2.13.2 beyond the renamed-linter noise; the first implementation had ~45 findings (noinlineerr, wsl_v5, goconst, err113, complexity directives, one unused helper, gofmt)
- **Fix:** plain-assignment error handling, blank-line placements, fixture string constants, static package-level fixture errors, precise per-function nolint directives (trimmed until nolintlint-clean), removed the unused helper
- **Files modified:** internal/session/compaction.go, internal/session/compaction_test.go, internal/session/session.go
- **Verification:** `golangci-lint 2.13.2 run internal/session/` reports zero findings in the new files beyond exhaustruct_v5; the remaining session.go trio (gocognit on streamAndEmit, rangeValCopy on firstTextOf, noinlineerr on AppendRawThinking) is the pre-plan baseline, verified by linting commit 4829451 in a temp worktree
- **Committed in:** 93d81ba

---

**Total deviations:** 2 auto-fixed (Rule 1 bug, Rule 3 blocker)
**Impact on plan:** No scope change — a test fixture repair and the standing lint-gate obligation.

## Issues Encountered
- **Producing-turn re-seed tension (architectural, NOT auto-taken — Rule 4):** D-07's "summarize → marker → build request" sequence and criterion 3's recovery value read as the current turn benefiting immediately, but 19-03's test-pinned position rule (the marker resets turns that START after it; a mid-flight marker of the projected turn is not its reset point) makes the producing turn's own window unchanged. The E2E was shaped to the pinned semantics (the next turn carries the seed; the compacting turn completes normally; the overflow retry recovers the session rather than guaranteed-shrinking the producing turn's window). The candidate carve-out and its Pitfall-5 safety analysis are recorded in `deferred-items.md` for architect sign-off.
- **Compacting note wire gap (pre-authorized degrade):** see Decisions; recorded in `deferred-items.md`.
- **mise ci lint leg red (pre-existing, out of scope):** the documented repo-wide golangci-lint 2.12↔2.13 config drift (19-03 issue, `deferred-items.md`). vet, CGO_ENABLED=0 build, and the full `-race` suite all green.
- **internal/runtime flakes (pre-existing, out of scope):** `TestAskPark_PromptResponsePrecedesResolution` (documented) and `TestCronWiring_AutomationTurnDeclinesGatedAsk` (a third member of the load-sensitive family — first observed here) each failed once under full-suite load; both passed in isolation and the full package passed on rerun. 19-04's disabled path is structurally inert (the check returns before any work when settings are unset). Noted in `deferred-items.md`.

## TDD Gate Compliance

Tasks 1-2 followed RED→GREEN with committed gates: `test(19-04)` commits 46a4906 and 3c2c490 each precede their `feat(19-04)` GREEN commits efa598b and 78a043f. RED failures verified for the right reasons (undefined engine symbols at Task 1; the missing loop-head wiring and overflow intercept at Task 2 — TestCompactNow passed at Task 2 RED because CompactNow is Task 1's artifact per the plan's own artifact list, pinned rather than driven by Task 2's test). Task 3 is the plan's composition-proof task with a single green commit per its action spec.

## Verification
- `go test -race ./internal/session/ -run 'TestCompaction' -count=1` — green (the full battery)
- `go test -race ./internal/session/ -count=1` — green (whole package, no regressions; 19-03's projector batteries untouched and passing)
- `go test -race ./internal/runtime/ -count=1` — green (after the documented load-flake verification in isolation)
- `go vet ./...` — clean; `CGO_ENABLED=0 go build ./...` — clean; `go test -race -count=1 ./...` — green except the runtime load-flake pair noted above
- `mise ci` — lint leg red on the pre-existing repo-wide 2.13.2 drift (documented); all other legs green; new files clean beyond the renamed-linter noise
- Bus isolation: zero client-visible publishes across compact invocations, asserted by the four-kind recording subscription
- Retry bounded: fail-twice ends in the turn's normal error path with exactly 3 stream calls (one retry)

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- 19-05 (config keys) wires `compaction.threshold_pct` / `compaction.enabled` onto `SetCompactionSettings` through the Phase 16 configOptions registry — the live-apply landing mirrors 16-05's SetTurnModel serialization discipline (the setter's doc comment pins the contract)
- Phase 20's /compact handler calls `CompactNow(ctx)` — one implementation, two triggers, per D-11
- Open architectural item for the operator: the producing-turn re-seed carve-out (deferred-items.md) — decides whether criterion 3's retry shrinks the producing turn's window or only recovers the session

## Self-Check: PASSED

- Files exist: internal/session/compaction.go, internal/session/compaction_test.go (created), internal/session/session.go (modified) — all present in the 19-04 commit range
- Commits 46a4906, efa598b, 3c2c490, 78a043f, 93d81ba all present on gsd/v1.2-claude-code-parity

---
*Phase: 19-compaction-cache-control*
*Completed: 2026-09-06*
