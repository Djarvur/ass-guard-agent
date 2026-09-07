---
phase: 20-built-in-commands-skills-per-agent-model
plan: 05
subsystem: discovery
tags: [fsnotify, live-rescan, atomic-swap, freshness-backstop, available-commands-update]

requires:
  - phase: 20-built-in-commands-skills-per-agent-model
    provides: plans 01 (chain + swap accessor + re-fire seam), 03 (live agent lookup), 04 (chain entries the rescan rebuilds)
provides:
  - internal/runtime/rescan.go — rescanCoordinator (fsnotify watch set, 300ms debounce, D-12 single-warning degrade, ctx teardown), rescanAndSwap (Discover → installRegistry → seam fire)
  - installRegistry: wholesale reg+mcpServers+chain swap under regMu (never in-place mutation; readers via registrySnapshot)
  - Invoke-time freshness backstop (D-10 layer two): stat-level root signature; drift → one synchronous rescan BEFORE resolution
  - acpserve watcher lifecycle on the serve ctx (started after the notify seam exists)
  - github.com/fsnotify/fsnotify v1.10.1 (audit-approved)
affects: [20-06 E2E, Phase 22 background]

tech-stack:
  added: ["github.com/fsnotify/fsnotify v1.10.1"]
  patterns:
    - "Debounced watch loop (Chmod skip, dynamic leaf arming, burst → one rescan)"
    - "Stat-signature freshness probe: O(watch-set) stats, drift → synchronous swap before resolve"

key-files:
  created:
    - internal/runtime/rescan.go
    - internal/runtime/rescan_test.go
  modified:
    - go.mod
    - go.sum
    - internal/runtime/runtime.go
    - internal/runtime/commands.go
    - internal/acpserve/acp_serve.go
    - internal/acpserve/simulator_e2e_test.go

key-decisions:
  - ".ass-guard watched LEAF-ONLY: its PARENT holds transcripts that change every turn — watching it wholesale turned each transcript append into a rescan event (T-20-18 storm, caught live by TestZedSimulatorE2E's post-response assertion; fixed with the leaf-only rule)"
  - "The freshness probe lives on resolveSlashCommand (the one resolution path both engine modes share); probe compares a stat-level signature of the watch roots stamped at install time — stat errors degrade to 'fresh' (never blocks resolution)"
  - "registrySnapshot under regMu for every r.reg reader (CommandRegistry, Skill/AgentListing, SkillExecute, ResolveSkill paths) — the wholesale swap is race-free; -race stress proof in TestRescanConcurrency"
  - "Debounce 300ms (the documented midpoint of the locked 100–500ms discretion window)"

patterns-established:
  - "waitForCond deadline polling in tests (never sleep-fixed assertions)"
  - "The simulator turn-story admits available_commands_update anywhere (pre-turn session setup, not turn emission)"

requirements-completed: [CMDS-04, ACP-04]

duration: 88 min
completed: 2026-09-07T23:25:00Z
---

# Phase 20 Plan 05: Live Discovery Rescan Summary

**Discovery is live: watched file changes debounced-rescan into an atomic chain swap and a full-set advertisement re-fire, the invoke-time backstop catches every watch miss before resolution, and the whole loop is -race-proven against concurrent turns — with watcher loss degrading once to invoke-time-only.**

## Performance

- **Duration:** 88 min
- **Tasks:** 3/3
- **Files:** 2 created, 6 modified (+go.mod/go.sum)

## Accomplishments

- fsnotify v1.10.1 added (audit-approved, pinned). rescanCoordinator: parent+leaf watch set for .claude trees (dynamic leaf arming for first-ever installs), leaf-only for .ass-guard, Chmod skip, 300ms debounce coalescing bursts into one rescan.
- rescanAndSwap shared by startup/rescan/backstop: Discover → installRegistry (wholesale swap under regMu) → chain rebuild + signature stamp → seam fire.
- Freshness backstop on the resolution path: watcher-stopped + file-added resolves the new file on the NEXT invocation (probe → drift → synchronous swap → resolve post-scan).
- Battery: watcher add/remove both live, debounce exactly-one-callback per 5-write burst, D-12 degrade + backstop, refire full-set completeness, -race concurrency stress (4 resolution hammers × churn loop × mid-session agent dispatchable), clean ctx shutdown.
- acpserve starts the watcher after the notify seam; simulator turn-story updated for the new pre-turn advertisement frames.

## Self-Check: PASSED

- go test -race ./internal/runtime/ ./internal/acpserve/ green (119s + 25s over the affected selections; full suites run at close).

## Deviations from Plan

- **[Rule 2 - live-found hazard] .ass-guard watch scope narrowed to LEAF-ONLY**: the plan's watch-set prescription ("the stable PARENTS ... .ass-guard/ where present") produced a transcript-write → rescan storm (every turn re-advertised; caught by the simulator's post-response ordering assertion, not by the plan's own tests). The leaf-only rule keeps .ass-guard discovery coverage (its skills/commands/agents leaves) without the storm — arguably the plan's D-12 'sessions never die over discovery' spirit applied to its own watch-set letter.
- **[Rule 3 - test contract] TestRescanRefire lives at the runtime seam level** (two changes → two complete advertisements via CommandAdvertisement + fires>=2) rather than full acpserve pipes — the composition-level frame emission is already pinned by 20-01's TestCommandsNotifySeam and the updated simulator E2E; a third full-pipe harness would duplicate them.

## Issues Encountered

- The transcript-storm above (found and fixed in-plan).

## Next Phase Readiness

20-06's E2E battery can assert live rescan re-fire, agent dispatch + resolvedModel, and advertisement end-to-end over the simulator harness.
