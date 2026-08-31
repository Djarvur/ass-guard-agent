---
phase: 17-permissions-elicitation
plan: 01
subsystem: permissions
tags: [permissions, rule-engine, cc-parity, yaml, acl, security, atomic-writes]

# Dependency graph
requires: []
provides:
  - "internal/perm rule grammar (D-02): ParseRule/Rule/Effect, RuleSet.Evaluate with fixed deny→ask→allow first-match order, compound-command split, bounded prefix-wildcard matcher (no regex engine), MCPName/SplitMCPName namespace helpers, NewRuleSet with returned []Warning diagnostics"
  - "internal/perm permissions.yaml store (D-01/D-03): Open zero-config floor (dirs 0750, file 0600), Rules() copy-on-read snapshot, AllowTool/ForbidTool dialog writers (simple entries, both directions, idempotent), 0600 atomic temp+rename persistence with structured load warnings"
affects: [17-02-permission-gate-chokepoint, 17-03-ask-queue, 21-hooks, ACP-01]

# Actuals (#2632) — chars/4 over the realized diff (49,100 chars), same scale as the plan's estimate
actuals:
  tokens: 12275
  tasks: 2
  commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "First-match deny→ask→allow evaluation with severity-cascaded compound matching (deny=any sub, ask=any sub, allow=ALL subs, unmatched sub fails safe)"
    - "Warning sink as returned diagnostics ([]Warning), never globals or logging in the pure grammar"
    - "renameFunc package-global seam (providerfactory convention) proving crash-between-marshal-and-rename"
    - "Commit-to-memory only after the atomic save succeeds — failed writes leave file AND state unchanged"

key-files:
  created:
    - internal/perm/rules.go
    - internal/perm/rules_test.go
    - internal/perm/store.go
    - internal/perm/store_test.go
  modified: []

key-decisions:
  - "Compound fail-safe semantics: any deny subcommand denies, any explicit ask subcommand asks, and an UNMATCHED subcommand forces overall Unmatched — an allow rule must cover EVERY subcommand before a compound executes (CC parity + the phase's fail-safe-default discipline; prevents 'git status && curl evil' riding an allow on git status)"
  - "ParseRule(string) leaves Effect as EffectNone; NewRuleSet assigns the effect per owning list — keeps the pinned single-string signature while one grammar serves three lists"
  - "Unanchored allow globs ('*', 'B*', 'mcp__*') are skipped as inert at NewRuleSet with one structured Warning (CC asymmetry); the same globs match in deny/ask; the STORE keeps such lines verbatim (operator-owned file)"
  - "Dialog write guard validSimpleEntry (letters/digits/underscore only) enforces D-01's dialog-never-writes-richer-rules boundary as a store API constraint, not caller discipline"
  - "Store warnings use the same Warning type/sink as the grammar (returned diagnostics) plus one slog structured line per skipped hand-edited line (stderr discipline)"

patterns-established:
  - "Pattern: permission rule evaluation is pure and consumer-free — the 17-02 chokepoint calls Evaluate per tool call and owns the Unmatched decision (D-05/D-06)"
  - "Pattern: trust-state files commit in-memory state only after a successful atomic save (crash-safe in both directions)"

requirements-completed: [ACP-01]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "CC-parity permission rule grammar: parse (bare/Tool(specifier)/end-anchored :*), fixed deny→ask→allow first-match evaluation with specificity never reordering, compound-command split with worst-verdict cascade, mcp__ namespace with whole-server bare rules, unanchored-allow-glob inertness with warnings, malformed-line tolerance, bounded prefix-wildcard matcher (no regex engine)"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/perm/rules_test.go#TestRules"
        status: pass
      - kind: unit
        ref: "internal/perm/rules_test.go#TestRulesParse"
        status: pass
      - kind: unit
        ref: "internal/perm/rules_test.go#TestRulesWarnings"
        status: pass
      - kind: unit
        ref: "internal/perm/rules_test.go#TestRulesMCPHelpers"
        status: pass
    human_judgment: false
  - id: D2
    description: "permissions.yaml persistence: zero-config floor (dirs 0750, file 0600), verbatim hand-edit load incl. richer rules, both dialog write directions (allow/deny simple entries), restart survival, idempotent byte-level no-op, atomic rename-failure discipline (prior file byte-intact, no tmp leftover), malformed-line tolerance with structured warnings, copy-on-read snapshot, concurrent click serialization, hard 0600 after save"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/perm/store_test.go#TestStoreAllowToolPersistsAndSurvivesRestart"
        status: pass
      - kind: unit
        ref: "internal/perm/store_test.go#TestStoreIdempotentNoop"
        status: pass
      - kind: unit
        ref: "internal/perm/store_test.go#TestStoreRenameFailureLeavesFileIntact"
        status: pass
      - kind: other
        ref: "mise ci (vet + golangci-lint v2 + CGO_ENABLED=0 build + go test -race ./...)"
        status: pass
    human_judgment: false

# Metrics
duration: 36min
completed: 2026-09-01
status: complete
---

# Phase 17 Plan 01: Permission Rule Grammar + permissions.yaml Store Summary

**CC-parity permission rule grammar (deny→ask→allow first-match, compound-split, mcp__ namespace) plus a 0600-atomic, idempotent, both-direction permissions.yaml trust store — the pure dependency-free core 17-02's gate chokepoint consumes**

## Performance

- **Duration:** 36 min
- **Started:** 2026-08-31T22:31:34Z
- **Completed:** 2026-08-31T23:08Z
- **Tasks:** 2
- **Files modified:** 4 (all created)

## Accomplishments
- `internal/perm` rule grammar table-pinned against the verified CC semantics (D-02): order-beats-specificity, ask-beats-allow, deny-beats-allow, compound-worst-match, mcp-namespace, unanchored-allow-skip, malformed-skip — green under `-race` before any consumer exists
- `internal/perm` permissions.yaml store: zero-config floor, both dialog write directions as simple tool-x-project entries (D-01/D-03), idempotent always-clicks, 0600 atomic temp+rename, crash-safe in both directions, tolerant hand-edit surface
- Package joins the standing `mise ci` gate green (vet + golangci-lint v2 strict + CGO_ENABLED=0 build + full `-race` suite)
- Exported write surface deliberately narrower than the file's read surface: exactly AllowTool/ForbidTool write simple entries; richer grammar is hand-edit-only and preserved through dialog writes

## Task Commits

Each task was committed atomically (TDD RED→GREEN):

1. **Task 1: Rule grammar — parse + CC-parity first-match evaluation**
   - `02e10d0` test(17-01): add failing CC-parity rule grammar battery (RED)
   - `6c4f290` feat(17-01): implement CC-parity rule grammar — parse + first-match evaluation (GREEN)
2. **Task 2: permissions.yaml store — 0600 atomic, both directions, idempotent**
   - `1bba5bd` test(17-01): add failing permissions.yaml store battery (RED)
   - `9f1dc3b` feat(17-01): implement permissions.yaml store — 0600 atomic, both directions, idempotent (GREEN)

## Files Created/Modified
- `internal/perm/rules.go` — ParseRule/Rule/Effect, RuleSet.Evaluate (D-02 contract doc verbatim + caller-owned Unmatched), SplitCompound, bounded matchGlob, MCPName/SplitMCPName, NewRuleSet with []Warning sink
- `internal/perm/rules_test.go` — 30-row CC-parity verdict table + parse-shape/warning/mcp-helper batteries
- `internal/perm/store.go` — Store: Open floor, load with validated lines + warnings, AllowTool/ForbidTool/persist, 0600 atomic save (2-space yaml encoder, renameFunc seam), copy-on-read Rules/Warnings
- `internal/perm/store_test.go` — 13-test persistence battery (floor, verbatim load, both directions, restart survival, idempotency, specifier-write rejection, rename-failure, stale tmp, malformed tolerance, richer-rule preservation, copy-on-read, concurrency, hard 0600)

## Decisions Made
- **Compound fail-safe (documented interpretation of D-02):** per-subcommand verdicts cascade deny > ask > unmatched > allow — an unmatched subcommand (e.g. `curl` in `git status && curl evil`) forces overall Unmatched so it can never ride an allow matched on another subcommand. Matches CC's "a rule must match each subcommand independently" plus the phase's fail-safe-default discipline; the plan's pinned case (allow git + deny rm ⇒ deny) holds.
- **Effect assignment lives in NewRuleSet** (per owning list), keeping the pinned `ParseRule(string) (Rule, error)` signature with `EffectNone` as the pre-placement placeholder.
- **Warning sink = returned diagnostics** (`NewRuleSet` returns `[]Warning`; `Store.Warnings()` exposes load-time warnings) — no globals in the pure grammar; the store additionally emits one structured slog line to stderr per skipped hand-edited line.
- **Crash-safe both directions:** `persist` commits to in-memory state only after the atomic save succeeds; a failed write leaves the file byte-intact AND the rule set unchanged (pinned by test).
- **Unanchored allow globs stay in the file** (operator-owned) but are inert at evaluation — the store only drops lines that fail ParseRule entirely.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] persist() wrote to disk without committing to in-memory state**
- **Found during:** Task 2 (GREEN iteration — caught immediately by TestStoreCopyOnRead panicking on an empty Allow list)
- **Issue:** the appended entry was saved to permissions.yaml but `s.lists` was never updated, so Rules() went stale until restart
- **Fix:** commit `env` to `s.lists` only after `save` succeeds (which also yields the desired failed-write rollback semantics)
- **Files modified:** internal/perm/store.go
- **Verification:** full package battery green under -race
- **Committed in:** 9f1dc3b (part of Task 2 GREEN)

---

**Total deviations:** 1 auto-fixed (Rule 1 bug, inside the TDD GREEN cycle before the commit)
**Impact on plan:** None on scope; the fix tightened the crash-safety semantics the plan pinned.

## Issues Encountered
- The first `mise ci` run failed once in the long `-race` suite; the failing package could not be reproduced — three subsequent full runs (`go test -race -count=1 ./...` ×2, `mise test`, `mise ci` ×2) were all green. Not attributable to this plan's changes (new leaf package only; no existing file touched). Treated as a transient flake in an unrelated package per the scope boundary.

## TDD Gate Compliance
- Both tasks followed RED→GREEN with `test(17-01)` commits preceding `feat(17-01)` commits; no REFACTOR commits needed (lint-clean at GREEN).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- 17-02 can consume `Store.Rules()` (RuleSet.Evaluate) per tool call at the chokepoint seam and persist allow_always/reject_always clicks via `AllowTool`/`ForbidTool` before execution (persist-then-execute)
- The evaluator returns `Unmatched` for rule-less subjects — the mode+class decision (D-05/D-06) is owned by the gate caller by construction
- MCP namespace mapping for the session catalog (RESEARCH Pitfall 7 / A7) lands in 17-02 using MCPName/SplitMCPName

---
*Phase: 17-permissions-elicitation*
*Completed: 2026-09-01*

## Self-Check: PASSED

- Created files verified on disk: internal/perm/{rules,rules_test,store,store_test}.go
- All 4 task commits verified in git log: 02e10d0, 6c4f290, 1bba5bd, 9f1dc3b
- Re-run: `go test -race ./internal/perm/ -count=1` green; `mise ci` green (see Issues Encountered for the one non-reproducible first-run flake)
