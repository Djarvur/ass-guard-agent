---
phase: 14-adoption-readiness-analysis-dispositions
plan: "05"
subsystem: session
tags: [token-economics, scheduler-tiers, subagent-routing, truncation, context-window]

requires:
  - phase: 07-scheduling
    provides: the scheduler tiers table + resolver (the EXISTING surface this plan rides)
  - phase: 08-08 core tool execution
    provides: the captured result forms + the 08-08 corpus measurements grounding the cap
provides:
  - Session.SubagentModel — the light-tier model slug for subagent dispatches (empty = parent model, exactly today's behavior)
  - sessionFor light-tier resolution wiring (same-provider check + loud cross-provider degrade)
  - DefaultToolResultCapBytes = 131072 + truncateToolResult + boundedToolResult — the single append-boundary truncation chokepoint
affects: [12-05 re-record (corpus-absent truncation marker), 14-06 tool contract, post-adoption cross-provider light routing]

actuals:
  tokens: 9743     # chars/4 over the realized code diff (886 insertions / 23 deletions, 9 files)
  tasks: 2
  commits: 4       # 2 RED + 2 GREEN (plus this docs commit)

tech-stack:
  added: []
  patterns:
    - "cfg-retaining wiring twin (setupScheduling): when a pinned call-site signature blocks returning a needed value, extract the semantics-complete twin and keep the legacy function as a thin wrapper"
    - "append-boundary truncation chokepoint: bound the JSON-STRING payload form only; under-cap payloads return the original raw bytes (no decode/re-encode round trip)"

key-files:
  created:
    - internal/session/truncate.go
    - internal/session/truncate_test.go
    - cmd/ass-guard/subagent_tier_wiring_test.go
  modified:
    - internal/session/session.go
    - internal/session/subagent.go
    - internal/session/subagent_test.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/provider_factory.go
    - cmd/ass-guard/goconst_constants.go

key-decisions:
  - "The truncation chokepoint decodes ONLY the JSON-string payload form: under-cap results are returned as the ORIGINAL raw bytes (byte-identical, no re-encode); object payloads (structured errors, openspec forms, TodoWrite echo, ask surfaces) never flow through the bound — structurally excluded, not size-excluded"
  - "The subagent Task-result append keeps the existing naive quote-concat encoding for under-cap results (byte-identical to today); only over-cap results get the properly re-encoded bounded form"
  - "The over-cap byte slice is rune-aligned (advances past a partial UTF-8 rune) so the marker's size math survives JSON re-encoding (json.Marshal would replace invalid bytes with U+FFFD)"
  - "setupScheduling extracted as setupProviderFactory's cfg-retaining twin: sessionFor needs the loaded config after startup, but setupProviderFactory's 3-return signature is pinned by parity.go (a 14-04 file, out of bounds) — the twin keeps parity/main call sites byte-identical"

patterns-established:
  - "Light-tier routing is a MODEL-level override on the per-dispatch profile copy — the shared session profile and the main turn loop's request shape are never touched (the economics lever cannot alter mimicry)"
  - "Cross-provider tier bindings degrade LOUDLY (one stderr warning naming both providers) and route to the post-adoption queue — never a silent wrong-wire"

requirements-completed: [EARLY-05]

coverage:
  - id: D1
    description: "Light-tier subagent model routing: config's tiers.light routes subagent dispatches to the cheap model on the per-dispatch profile copy; absent binding keeps the parent model; cross-provider binding degrades loudly (one warning, parent model kept)"
    requirement: EARLY-05
    verification:
      - kind: unit
        ref: internal/session/subagent_test.go#TestSubagentModel_OverrideApplied
        status: pass
      - kind: unit
        ref: internal/session/subagent_test.go#TestSubagentModel_EmptyKeepsParent
        status: pass
      - kind: integration
        ref: cmd/ass-guard/subagent_tier_wiring_test.go#TestSessionFor_ResolvesLightTier
        status: pass
      - kind: integration
        ref: cmd/ass-guard/subagent_tier_wiring_test.go#TestSessionFor_LightTierCrossProviderWarns
        status: pass
    human_judgment: false
  - id: D2
    description: "Tool-output truncation policy: oversized results (> 128 KiB) are tail-truncated with the explicit corpus-absent-flagged marker at the single append chokepoint covering parent tool results and subagent Task results; under-cap results are byte-unmodified"
    requirement: EARLY-05
    verification:
      - kind: unit
        ref: internal/session/truncate_test.go#TestTruncateToolResult_Cap
        status: pass
      - kind: unit
        ref: internal/session/truncate_test.go#TestTruncateToolResult_UnderCapUnmodified
        status: pass
      - kind: unit
        ref: internal/session/truncate_test.go#TestTruncateToolResult_Edges
        status: pass
      - kind: integration
        ref: internal/session/truncate_test.go#TestAppendToolResult_TruncatedBeforeTranscript
        status: pass
      - kind: integration
        ref: internal/session/truncate_test.go#TestSubagentResultAlsoBounded
        status: pass
      - kind: integration
        ref: internal/session/truncate_test.go#TestTruncation_UnderCapTranscriptByteIdentical
        status: pass
      - kind: command
        ref: "mise run ci (exit 0)"
        status: pass
    human_judgment: false

duration: 25min
completed: 2026-08-19
status: complete
---

# Phase 14 Plan 05: Token economics — light-tier subagent routing + tool-output truncation Summary

**Scheduler light-tier model override for subagent dispatches (existing tiers table, loud cross-provider degrade) plus a 128 KiB corpus-safe tail-truncation bound at the single AppendToolResult chokepoint**

## Performance

- **Duration:** ~25 min (started 2026-08-19T19:19:13Z)
- **Started:** 2026-08-19T19:19:13Z
- **Completed:** 2026-08-19T19:45:00Z (approx; docs close-out)
- **Tasks:** 2 (both TDD: RED → GREEN)
- **Files modified:** 9 (3 created, 6 modified)

## Accomplishments

- **Light-tier subagent routing (EARLY-05, economics half):** `Session.SubagentModel` (exported) carries the light-tier slug; `subagentProfile` applies the Model override on the per-dispatch COPY (value-copy semantics — the shared session profile and the main turn loop's request shape are never touched); `sessionFor` resolves `tierLight` through the EXISTING scheduler resolver (the same call shape `setupScheduling` uses for `tierHeavy`) with the same-provider check — a binding on the session's provider sets the field, absence leaves it empty silently (the documented parent-model default), a cross-provider binding degrades LOUDLY (exactly one stderr warning naming both providers, parent model kept — the second-provider-instance change routed to the post-adoption queue per the plan's Truths).
- **Tool-output truncation (EARLY-05, bounding half):** `DefaultToolResultCapBytes = 131072` (128 KiB — above the 08-08 corpus-observed ~70KB maximum, so no corpus-typical result is ever modified) and `truncateToolResult` (tail-keeping bound with the one-line marker `[ass-guard: tool output truncated; original N bytes, kept tail M bytes]`, rune-aligned byte slice). The chokepoint `boundedToolResult` applies at the AppendToolResult boundary — the parent loop's per-call result append AND the subagent Task-result append — before the transcript line exists, hence before both the mid-turn model view and the Projector's projected window fold it in.
- **Fidelity discipline held:** the truncation marker is flagged CORPUS-ABSENT in the helper's doc comment and routed to the 12-05 re-record (never to be added to a capture-pinning fixture as if observed); the marker does not collide with the captured `Exit code ` Bash error form or the no-output sentinel; under-cap payloads are byte-identical through the chokepoint (pinned by TestTruncation_UnderCapTranscriptByteIdentical at the transcript level).
- **Zero-dep prohibition held:** go.mod/go.sum byte-identical across all four plan commits (md5-verified before and after).

## Task Commits

Each task was committed atomically (TDD RED → GREEN):

1. **Task 1: Light-tier subagent routing** — `90b5bfc` (test, RED) → `ddb59ff` (feat, GREEN)
2. **Task 2: Tool-output truncation policy** — `c4a73d7` (test, RED) → `834c0d8` (feat, GREEN)

**Plan metadata:** this docs commit (SUMMARY + state updates).

## Files Created/Modified

- `internal/session/truncate.go` (created) — DefaultToolResultCapBytes, truncateToolResult (corpus-absent-flagged marker), boundedToolResult (the append-boundary chokepoint)
- `internal/session/truncate_test.go` (created) — tests 5-8 + TestSubagentResultAlsoBounded + the under-cap byte-identity pin
- `internal/session/subagent_test.go` (modified, additions only) — TestSubagentModel_OverrideApplied / EmptyKeepsParent (the fake provider's streamed-profiles capture seam)
- `internal/session/session.go` (modified) — Session.SubagentModel field; boundedToolResult applied at both append sites
- `internal/session/subagent.go` (modified) — subagentProfile's Model override on the copy
- `cmd/ass-guard/acp_serve.go` (modified) — runner fields (schedCfg/providerName/stderr), runACPServe handoff, sessionFor resolution + resolveSubagentModel + stderrOrDefault
- `cmd/ass-guard/provider_factory.go` (modified) — setupScheduling (cfg-retaining twin) + thin-wrapper setupProviderFactory
- `cmd/ass-guard/goconst_constants.go` (modified) — tierLight const (the tierHeavy constant family)
- `cmd/ass-guard/subagent_tier_wiring_test.go` (created) — sessionFor wiring tests 3-4 (both-ways + loud degrade)

## Decisions Made

- The chokepoint decodes only the JSON-STRING payload form (the captured plain-text result form). Object payloads — the structured {"error":…} convention, the openspec {stdout,stderr,exit_code,classification} forms, TodoWrite's JSON echo, the ask answered form — pass through untouched: only a tool Output payload is ever bounded, never transcript metadata or AskUserQuestion surfaces (which do not flow through the two bounded call sites at all).
- Under-cap JSON-string payloads return the ORIGINAL raw bytes (no decode/re-encode round trip) — byte-identity is structural, not incidental.
- The over-cap byte slice is rune-aligned so `json.Marshal` cannot replace invalid UTF-8 with U+FFFD (which would silently change the kept size the marker reports).
- `setupScheduling` is `setupProviderFactory`'s cfg-retaining twin rather than a signature change: sessionFor needs the loaded config after startup, but parity.go (a 14-04 file, out of bounds this plan) pins the 3-return call site. The twin carries the identical load + degrade + heavy-resolution semantics; parity/main call sites stay byte-identical.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] File set extended beyond files_modified**
- **Found during:** Task 1 (wiring)
- **Issue:** sessionFor needs the loaded scheduling config + the session provider name, but `setupProviderFactory`'s 3-return signature is pinned by `cmd/ass-guard/parity.go` (a 14-04 file, explicitly out of bounds) and discards the cfg.
- **Fix:** (a) `setupScheduling` extracted in provider_factory.go as the cfg-retaining twin — `setupProviderFactory` is now a thin wrapper over it, so parity.go/main.go call sites are byte-identical with zero behavior change; (b) `tierLight = "light"` added to goconst_constants.go (the acceptance criterion demands "the same constant family as provider_factory's tierHeavy use"); (c) new files internal/session/truncate.go (the plan's action explicitly allows "session.go or a focused file") and cmd/ass-guard/subagent_tier_wiring_test.go (tests 3-4 are package-main tests; the plan's artifact note "subagent_test.go additions (4)" cannot span two packages).
- **Files modified:** cmd/ass-guard/provider_factory.go, cmd/ass-guard/goconst_constants.go, plus the two new files
- **Verification:** full cmd/ass-guard suite green; parity path untouched (byte-identical call site); mise run ci green
- **Committed in:** ddb59ff (Task 1 GREEN)

**2. [Rule 1 - Bug] Lint-gate conformance in the new test files**
- **Found during:** Task 2 (mise run ci)
- **Issue:** 27 lint findings — funlen on the table test, goconst counts tipped over by the new literals ("glm-5.2" 5 new occurrences, "dispatch"/"run" cross-file counts), gosmopolitan Han-script literals, builtinShadow `max`, rangeValCopy on the 480-byte Line struct, lll lines, and paralleltest on the HOME-pinned wiring tests (the linter does not see t.Setenv through the pinEmptyHome helper).
- **Fix:** extracted the table rows into named helpers; test consts for the repeated literals; U+20AC (€) instead of Han runes for the multibyte tail; index loop over Line; shortened messages; the house `//nolint:paralleltest // HOME pinned` pattern on the two wiring tests.
- **Files modified:** internal/session/truncate_test.go, internal/session/subagent_test.go, cmd/ass-guard/subagent_tier_wiring_test.go
- **Verification:** mise run ci exit 0
- **Committed in:** 834c0d8 (Task 2 GREEN)

---

**Total deviations:** 2 auto-fixed (1 blocking file-set, 1 lint conformance)
**Impact on plan:** No scope creep — both mechanisms landed exactly as planned; the file-set extension is the minimal cfg-plumbing the wiring requires, and the lint fixes are test-code conformance.

## Issues Encountered

None beyond the deviations above. The tracer feedback gate (Task 1's verify re-run end-to-end after commit, auto mode) passed on the first re-run.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- EARLY-05 complete: both mechanisms test-proven, `mise run ci` green across all 26 packages, go.mod/go.sum untouched.
- One plan remains in Phase 14 (14-06 tool contract, EARLY-06); the phase then continues on the adoption path 12-05 → 12-04 → 12-06 → LINE.
- Routed residue (documented, deliberate): the truncation marker's captured form → 12-05 re-record; cross-provider light-tier routing (a second provider instance threaded through the subagent runner) → post-adoption queue.

## Self-Check: PASSED

- Created files exist: internal/session/truncate.go, internal/session/truncate_test.go, cmd/ass-guard/subagent_tier_wiring_test.go — FOUND
- Task commits exist: 90b5bfc, ddb59ff, c4a73d7, 834c0d8 — FOUND in git log
- Acceptance criteria re-verified post-commit: cap constant + exact marker shape grep-able; corpus_absent + 12-05 routing in the doc comment; both append paths bounded; no "Exit code " prefix collision; go.mod/go.sum md5-identical to plan start

---
*Phase: 14-adoption-readiness-analysis-dispositions*
*Completed: 2026-08-19*
