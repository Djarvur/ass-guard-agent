---
phase: 08-slash-command-kickoff
plan: 04
subsystem: slash-command-expansion
tags: [expansion, zcode-substitution, provenance, context-boundary, turn-runner, prompt-injection-guard, ecosys]

# Dependency graph
requires:
  - phase: 08-01
    provides: ecosys namespaced command discovery (commands/<ns>/<name>.md → "<ns>:<name>" keys)
  - phase: 08-03
    provides: openspec config/loader (CommandShape table + seeded.toml home for new tables)
  - phase: 04 tool-executor / 02 session core (v1.0)
    provides: sessionTurnRunner two-path turn pipeline, transcript Manager, Projector lean window, engine Observe loop
provides:
  - ecosys.ParseInvocation + Command.Expand — the exact zcode substitution contract (table-pinned)
  - Expansion wired at the turn runner: BOTH engine paths + engine continue-injections expand identically
  - command_provenance transcript line (key + source file + typed args) next to the expanded user message; projector replays the expanded body, provenance is metadata
  - [command_mutability] seeded table + pre-turn boundaries for mutating commands (cause mutating-command:<key>)
  - graceful degradation: registry load failure → expansion off (registry dropped, never stale), turns proceed on plain text
  - TestAssistantRoleOnly_InjectionGuard — the CMD-05 regression pin (user-side handoff patterns decide nothing)
affects: [08-06 (E2E drives /opsx:* through this seam), Telegram peer (surface-agnostic — expansion lives below internal/acp), every discovered command (D-03)]

# Tech tracking
tech-stack:
  added: []  # stdlib only (regexp + strings.ReplaceAll)
  patterns:
    - "expansion at the TURN RUNNER (session layer), never internal/acp — Telegram inherits /opsx:* free"
    - "registry mirrors disk: load failure DROPS the registry rather than serving stale commands"
    - "provenance is a metadata line adjacent by append order (empty turnID pre-turn — same convention as hook boundaries)"

key-files:
  created:
    - internal/ecosys/expand.go
    - internal/ecosys/expand_test.go
  modified:
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/acp_serve_test.go
    - internal/session/manager.go
    - internal/session/transcript.go
    - internal/session/projector_test.go
    - internal/openspec/config.go
    - internal/openspec/register.go
    - internal/openspec/seeded.toml
    - internal/openspec/config_test.go

key-decisions:
  - "Append suppression keyed on placeholder EXISTENCE (research ground truth FEATURES §a / STACK: 'args appended when NO placeholder'), not the plan's 'any $N that matched' wording — a present-but-out-of-range $N suppresses the append (the plan cites the research as the authority to implement exactly)"
  - "loadCommandRegistry DROPS the registry on reload failure instead of keeping last-known-good: the registry mirrors the on-disk tree; an unreadable tree means expansion OFF, not stale shadows"
  - "Engine continue-injections expand in engineTurnRunnerAdapter.Run — the only re-entry point for injected turns; first prompts expanded in Run, so adapter re-entry is a no-op for prose-first bodies (real opsx bodies verified)"
  - "Test 11 driven at the adapter level: the current engine produces no table-driven NextPrompt (literal 'continue' via NextStagePrompt; real /opsx injections are 08-06 scope) — the adapter-level test pins the same property (injections entering adapter.Run arrive expanded)"
  - "Mutability vocabulary exported from openspec (MutabilityMutating/ReadOnly) and shared by the tool table + command table + the runner's boundary check"

patterns-established:
  - "firstTextBlockIndex → ParseInvocation → registry hit → Expand → boundary (mutating) → provenance → replaced blocks; every failure degrades to a stderr log"
  - "transcript adjacency: boundary then provenance then user_message — the projector's ReadLastBoundary reset treats the expanded stage exactly like a tool-call boundary"

requirements-completed: [CMD-02, CMD-05]

# Metrics
duration: 70min
completed: 2026-08-14
---

# Phase 8 Plan 04: Expand + turn wiring Summary

**Typing /opsx:explore fix-it now drives a turn whose user message IS the command's expanded body — invisibly (D-01), with durable provenance + mutating-command boundaries (D-02/D-11), on every path including engine injections, with the prompt-injection guard pinned by regression.**

## Performance

- **Duration:** ~70 min
- **Tasks:** 3
- **Files modified:** 11

## Accomplishments
- `ecosys.ParseInvocation` (compiled-once zcode token regex, leading-newline tolerance, verbatim args) + `Command.Expand` ($ARGUMENTS verbatim, $1..$9 whitespace-split with out-of-range/$0 → empty, `${ARGUMENTS}` + `` !`cmd` `` literal, single pass, fences substituted, "User arguments:" append only when NO placeholder exists) — pinned by a 24-row table written RED-first
- `sessionTurnRunner.loadCommandRegistry` at startup (engine-independent): ecosys.Discover + the openspec mutability table; failure logs to stderr and drops expansion (never serves stale commands)
- `expandUserBlocks` between `toContentBlocks` and `runOneTurn` (both engine paths) AND in `engineTurnRunnerAdapter.Run` (continue-injections): registry hit → expanded body replaces the first text block; mutating commands write the pre-turn boundary (`mutating-command:<key>`); provenance line written next to the user message; unknown `/foo` falls through untouched
- `session.Manager.AppendCommandProvenance` + `command_provenance` line type: key + source file + typed args as metadata; the projector replays the expanded body and never surfaces provenance as content
- `[command_mutability]` seeded table (explore read-only; propose/apply/sync/update/archive mutating — write-semantics classification) with operator-overlay flip + loader validation
- `TestAssistantRoleOnly_InjectionGuard`: a command body carrying "Implementation Complete — ready for review" as the USER message decides NOTHING (signal unmatched, 1 provider call) — the assistant-role-only matcher pinned end-to-end

## Task Commits

1. **Task 1: substitution contract (RED→GREEN)** — `b302e6a` (test: 24-row table, build-fail RED) → `6b98319` (feat: expand.go)
2. **Task 2: turn wiring (RED→GREEN)** — `6b722ba` (test: Tests 9-14, build-fail RED on loadCommandRegistry) → `6b48107` (feat: registry load + expandUserBlocks + both call sites + the compile-dep slices of T3)
3. **Task 3: provenance/boundary/table/regression (pinning commit)** — `dd63f1a` (Tests 15-19; implementation slices had landed in T2 GREEN as compile deps — see Deviations)

**Plan metadata:** (this commit)

## Files Created/Modified
- `internal/ecosys/expand.go` — ParseInvocation + Expand (the zcode contract)
- `cmd/ass-guard/acp_serve.go` — loadCommandRegistry, expandUserBlocks, Run/adapter wiring, mutatingCommandCause
- `internal/session/{manager,transcript}.go` — AppendCommandProvenance + line type
- `internal/session/projector_test.go` — provenance/replay pins
- `internal/openspec/{config,register}.go` + `seeded.toml` — [command_mutability] + Mutability* vocabulary + validation
- test files: ecosys contract table, acp_serve expansion suite, openspec table test

## Decisions Made
- See key-decisions: existence-keyed append suppression, registry-drop degradation, adapter-level injection test, shared mutability vocabulary

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Contract wording conflict: append suppression**
- **Found during:** Task 1 GREEN
- **Issue:** plan action said append fires when no `$N` "matched"; the research it cites as ground truth (FEATURES §a, STACK) says append fires when NO placeholder EXISTS — the two readings diverge exactly when a body carries an out-of-range `$N`
- **Fix:** implemented the research reading (a present placeholder consumed the args slot — zcode does not re-surface args); the divergent table row corrected with the reasoning documented in-table
- **Committed in:** `6b98319`

**2. [Rule 2] T3 implementation landed inside T2 GREEN**
- **Issue:** expandUserBlocks (T2) references AppendCommandProvenance + the mutability table (T3 artifacts) — they are compile dependencies, so they landed in T2's GREEN commit
- **Fix:** T3's commit carries the full test battery (15-19) pinning those slices against current code — exactly the plan's own instruction for Test 19 ("the guard exists today by construction — the test PINS it"); no implementation escaped testing
- **Committed in:** `6b48107` + `dd63f1a`

**3. [Rule 2] Test 11 driven at the adapter level, not via a fake NextPrompt table**
- **Issue:** the plan sketched a fake pattern table producing NextPrompt "/opsx:propose name-x", but the current engine sets no table-driven NextPrompt (continue-injections are the literal "continue"; real /opsx injections are 08-06 scope)
- **Fix:** the test drives engineTurnRunnerAdapter.Run (the single re-entry point for injections) with a "/opsx:propose name-x" prompt and asserts the expanded body reaches sess.Prompt — the same property at the seam that will carry 08-06's injections
- **Committed in:** `6b722ba` + `6b48107`

---

**Total deviations:** 3 auto-fixed (1 blocking, 2 sequencing)
**Impact on plan:** No scope creep; contract fidelity pinned to research ground truth over plan prose.

## Issues Encountered
- None. Full `go test ./... -race` suite green (including the historically flaky internal/profile stability test, which passed this run — pre-existing, dispositioned to Phase 9 AUD-05).

## TDD Gate Compliance
RED commits: `b302e6a` (contract table, build-fail), `6b722ba` (wiring tests, build-fail on loadCommandRegistry). GREEN commits: `6b98319`, `6b48107`. Pinning commit: `dd63f1a` (Tests 15-19 green by construction, per the plan's pinning instruction). Full `-race` suite green after each.

## Next Phase Readiness
- 08-05 (skills) builds on the same registry; 08-06's E2E types /opsx:* commands through this seam — expansion, provenance, boundaries all live
- Telegram inherits expansion free (the seam is below internal/acp)
- `go vet ./...` + `golangci-lint run ./...` (0 issues) + `CGO_ENABLED=0 go build ./...` clean

## Self-Check: PASSED

- FOUND: internal/ecosys/expand.go (ParseInvocation + Expand; zero exec surface)
- FOUND: cmd/ass-guard/acp_serve.go (ecosys.Discover in loadCommandRegistry; expandUserBlocks in Run + engineTurnRunnerAdapter)
- FOUND: command_mutability table in seeded.toml with opsx:apply mutating / opsx:explore read-only
- Commits b302e6a/6b98319/6b722ba/6b48107/dd63f1a present on master

---
*Phase: 08-slash-command-kickoff*
*Completed: 2026-08-14*
