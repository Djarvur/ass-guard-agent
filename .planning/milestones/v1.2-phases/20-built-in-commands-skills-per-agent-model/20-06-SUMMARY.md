---
phase: 20-built-in-commands-skills-per-agent-model
plan: 06
subsystem: verification
tags: [e2e, simulator, criteria-matrix, operator-uat]

requires:
  - phase: 20-built-in-commands-skills-per-agent-model
    provides: plans 01-05 (the whole command surface)
provides:
  - TestSimulatorCommandSurface — four wire-level scenarios over the real Run composition
  - The criteria-to-evidence matrix (below) + the operator UAT items (pending)
affects: [phase close, retrospective]

tech-stack:
  added: []
  patterns:
    - "Simulator scenario shape: fixture-discovered entries (agents with DECLARED floor slugs), stub script sized per scenario, deadline polling for rescan timing"

key-files:
  created: []
  modified:
    - internal/acpserve/simulator_e2e_test.go

key-decisions:
  - "The agent fixture's frontmatter model is the floor-declared GLM-5.3 (routing applies for real over the wire — not a degrade)"
  - "Operator checkpoint recorded as PENDING (unattended execution): items persisted to 20-HUMAN-UAT.md per the Phase 15/16 WINDOWS precedent"

requirements-completed: [ACP-04, CMDS-01, CMDS-02, CMDS-03, CMDS-04, SKLS-01, SKLS-02, SKLS-03]

duration: 46 min
completed: 2026-09-08T00:20:00Z
---

# Phase 20 Plan 06: E2E Battery + Phase Close Summary

**The command surface is proven over the real wire composition (advertisement, class-B zero-model-turn, live rescan re-fire + invocation, agent dispatch with both resolvedModel channels), the full suite is race-green, and the live-Zed operator checkpoint is persisted as pending UAT items.**

## Performance

- **Duration:** 46 min
- **Tasks:** 3 (E2E + gate/matrix + operator checkpoint persisted)
- **Files:** 1 modified

## Task 1 — TestSimulatorCommandSurface (6e2d7e6)

Four scenarios, one Run over pipes, race-clean:

1. ADVERTISEMENT: session/new precedes its response with available_commands_update carrying status/init/help/cost/demo-agent (winner set, schema spellings).
2. CLASS-B: /status → user_message_chunk echo + agent_message_chunk output + end_turn, stub requests == 0.
3. LIVE RESCAN: commands/extra.md created under the watched root → advertisement re-fires WITHIN debounce+epsilon including "extra"; /extra fires (stub request 1 — the expanded turn).
4. AGENT DISPATCH: /demo-agent → subagent chunks stream into the turn, the resolvedModel note reaches the client, the stub's second request carries the frontmatter model (GLM-5.3), and the ON-DISK transcript's subagent_dispatch line carries resolvedModel + a local_command line for the slash invocation.

## Task 2 — Full gate + criteria-to-evidence matrix

Gate components executed 2026-09-08:

- `go vet ./...` — clean.
- `CGO_ENABLED=0 go build ./...` — clean (static binary preserved; fsnotify adds no cgo).
- `go test -race -count=1 ./...` — ALL 38 packages ok (incl. runtime 137s, acpserve 75s, cmd 56s).
- `golangci-lint` (mise) — **red at BASELINE, environmental**: the mise-managed golangci-lint auto-updated to 2.13.2 (built Aug 2026) which renamed linters (exhaustruct→exhaustruct_v5, wsl→wsl_v5, goconst→varnamelen drift); .golangci.yml's exclusion rules reference the pre-rename names, so ~1.6k pre-existing findings surface repo-wide. Verified pre-existing via stash-baseline runs BEFORE any Phase 20 commit. Every file Phase 20 touched or created passes the linters still matching the config (verified per-file throughout). RECOMMENDED FIX (one line-class config update, out of Phase 20's scope): update .golangci.yml exclusions to the _v5 linter names or pin golangci-lint = "2.12" in .mise.toml. Recorded in STATE.md.

| # | Phase criterion (ROADMAP) | Evidence (command → artifact) |
|---|---------------------------|-------------------------------|
| 1 | Autocomplete: available_commands_update on session start + discovery change | `go test ./internal/acp/ -run TestAvailableCommands` → available_commands_test.go (golden, byte-for-byte); `go test ./internal/acpserve/ -run TestSimulatorCommandSurface` → simulator_e2e_test.go scenarios 1+3; `go test ./internal/acpserve/ -run TestCommandsNotifySeam` → commands_seam_test.go; Operator UAT item 1 (pending) |
| 2 | Live pickup: new/removed commands/skills/agents without restart | `go test ./internal/runtime/ -run TestRescan` → rescan_test.go (watcher add/remove, debounce, -race concurrency, mid-session agent dispatch); simulator scenario 3; Operator UAT item 3 (pending) |
| 3 | Class-B set answers instantly, NO model turn | `go test ./internal/runtime/ -run TestClassB` → commands_test.go (all 12 commands × zero Stream calls + local_command lines); simulator scenario 2 (stub == 0); /compact leg: Phase 19's CompactNow IS registered (both executed) — real machinery invoked under the turn-mutex contract, focus-args recorded verbatim (16-D-22) with a fixed-form note (signature admits no focus instructions); Operator UAT items 2+4 (pending) |
| 4 | /init prompt-expanding via the existing seam with provenance | `go test ./internal/runtime/ -run TestInitExpansion` → commands_test.go (engine-off + engine-on parity, builtin:init provenance, D-01 shadow) |
| 5 | /skill + /agent invocation | `go test ./internal/runtime/ -run 'TestSkillSlash|TestAgentSlash'` → commands_test.go (locked Expand semantics, user-invocable:false exclusion, dispatch with Prompt/Tools/model, D-02 collision); simulator scenario 4 |
| 6 | Per-agent model routing, resolvedModel reported back, mis-routing impossible | `go test ./internal/runtime/ -run TestDispatchModel` → subagent_tier_wiring_test.go (all precedence arms, cross-provider ROUTE + cache, unknown/uncredentialed one-warning degrades, reversal of 14-05); `go test ./internal/session/ -run TestSubagentDispatch_ResolvedModel` → subagent_test.go (durable field + D-20 tolerance); simulator scenario 4 (live note + on-disk line) |

Probe rows: the nine flagged PROBE-UNRESOLVED rows (20-01×3, 20-02, 20-04×3, 20-05×2 per plans) remain flagged per the fallback protocol. Prohibitions: P-20-01 (no execution/permission authority from discovered files — enforced by TestAgentSlash restricted-set + the unchanged gate pipeline; T-20-14 advisory-posture invariant), P-20-02 (cost source note — enforced by TestClassBCost on every variant). Decision coverage D-01..D-16: each lands in the plan battery named above (D-05/D-08 shape, D-06 clear, D-07 cost, D-13/D-14/D-15/D-16 dispatch).

## Task 3 — Operator checkpoint (PENDING)

Unattended execution: the seven live-Zed verification steps are persisted as `.planning/phases/20-built-in-commands-skills-per-agent-model/20-HUMAN-UAT.md` (items 1-7: autocomplete completeness, instant /status, live mid-session pickup, agent dispatch note, /cost source note, /compact output at this milestone, /help==autocomplete consistency). The automated wire-level equivalents all pass; the UX confirmation remains the operator's.

## Self-Check: PASSED

- TaskSimulatorCommandSurface green under -race; full `go test -race ./...` green; vet + static build clean; lint red is the documented pre-existing tooling drift (above).

## Deviations from Plan

- **[Rule 3 - environment] operator checkpoint executed as PERSISTED-PENDING, not blocking-wait**: background execution cannot hold a human gate open; items persisted per the Phase 15/16 WINDOWS precedent (verified live by the operator at the next session, tracked in STATE).
- **[Rule 3 - environment] mise ci decomposed**: the lint task's failure is the pre-existing golangci drift (verified at baseline); the other three gates (vet/build/test) ran green in full.

## Issues Encountered

- Pre-existing (carried): lint-baseline drift; TestAskPark load-flake did NOT recur across three full-suite runs this phase.
